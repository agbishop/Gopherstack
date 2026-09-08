package securityhub

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
)

// configPolicyIdentifierFromPath returns the trailing identifier of a
// /configurationPolicy/{id} path, excluding the create/get/list sub-paths
// which are handled as their own exact-match routes.
func configPolicyIdentifierFromPath(path string) (string, bool) {
	const prefix = "/configurationPolicy/"

	if !strings.HasPrefix(path, prefix) ||
		strings.HasPrefix(path, prefix+"create") ||
		strings.HasPrefix(path, prefix+"get") ||
		strings.HasPrefix(path, prefix+"list") {
		return "", false
	}

	return strings.TrimPrefix(path, prefix), true
}

func classifyConfigPolicyPath(method, path string) (string, string) {
	switch {
	case method == http.MethodPost && path == "/configurationPolicy/create":
		return opCreateConfigurationPolicy, ""
	case method == http.MethodGet && strings.HasPrefix(path, "/configurationPolicy/get/"):
		return opGetConfigurationPolicy, strings.TrimPrefix(path, "/configurationPolicy/get/")
	case method == http.MethodGet && path == "/configurationPolicy/list":
		return opListConfigurationPolicies, ""
	}

	id, ok := configPolicyIdentifierFromPath(path)
	if !ok {
		return opUnknown, ""
	}

	switch method {
	case http.MethodPatch:
		return opUpdateConfigurationPolicy, id
	case http.MethodDelete:
		return opDeleteConfigurationPolicy, id
	default:
		return opUnknown, ""
	}
}

func classifyConfigPolicyAssocPath(method, path string) (string, string) {
	switch {
	case method == http.MethodPost && path == "/configurationPolicyAssociation/batchget":
		return opBatchGetConfigurationPolicyAssociations, ""
	case method == http.MethodPost && path == "/configurationPolicyAssociation/get":
		return opGetConfigurationPolicyAssociation, ""
	case method == http.MethodPost && path == "/configurationPolicyAssociation/list":
		return opListConfigurationPolicyAssociations, ""
	case method == http.MethodPost && path == "/configurationPolicyAssociation/associate":
		return opStartConfigurationPolicyAssociation, ""
	case method == http.MethodPost && path == "/configurationPolicyAssociation/disassociate":
		return opStartConfigurationPolicyDisassociation, ""
	}

	return opUnknown, ""
}

func (h *Handler) handleCreateConfigurationPolicy(
	c *echo.Context,
	body map[string]any,
) error {
	name, _ := body["Name"].(string)
	description, _ := body["Description"].(string)

	var policy map[string]any

	if p, ok := body["ConfigurationPolicy"].(map[string]any); ok {
		policy = p
	}

	var tags map[string]string

	if t, ok := body["Tags"].(map[string]any); ok {
		tags = make(map[string]string, len(t))

		for k, v := range t {
			tags[k], _ = v.(string)
		}
	}

	cp, err := h.Backend.CreateConfigurationPolicy(name, description, policy, tags)
	if err != nil {
		return typedErrorResponse(c, http.StatusInternalServerError, "InternalException", err.Error())
	}

	return c.JSON(http.StatusOK, configPolicyToResponse(cp))
}

func (h *Handler) handleGetConfigurationPolicy(c *echo.Context, identifier string) error {
	cp, err := h.Backend.GetConfigurationPolicy(identifier)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return typedErrorResponse(
				c, http.StatusNotFound, "ResourceNotFoundException",
				"Configuration policy not found",
			)
		}

		return typedErrorResponse(c, http.StatusInternalServerError, "InternalException", err.Error())
	}

	return c.JSON(http.StatusOK, configPolicyToResponse(cp))
}

func (h *Handler) handleUpdateConfigurationPolicy(c *echo.Context, identifier string, body map[string]any) error {
	name, _ := body["Name"].(string)
	description, _ := body["Description"].(string)

	var policy map[string]any

	if p, ok := body["ConfigurationPolicy"].(map[string]any); ok {
		policy = p
	}

	cp, err := h.Backend.UpdateConfigurationPolicy(identifier, name, description, policy)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return typedErrorResponse(
				c,
				http.StatusNotFound,
				"ResourceNotFoundException",
				"Configuration policy not found",
			)
		}

		return typedErrorResponse(c, http.StatusInternalServerError, "InternalException", err.Error())
	}

	return c.JSON(http.StatusOK, configPolicyToResponse(cp))
}

func (h *Handler) handleDeleteConfigurationPolicy(c *echo.Context, identifier string) error {
	if err := h.Backend.DeleteConfigurationPolicy(identifier); err != nil {
		if errors.Is(err, ErrNotFound) {
			return typedErrorResponse(
				c,
				http.StatusNotFound,
				"ResourceNotFoundException",
				"Configuration policy not found",
			)
		}

		return typedErrorResponse(c, http.StatusInternalServerError, "InternalException", err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *Handler) handleListConfigurationPolicies(c *echo.Context) error {
	nextToken := c.QueryParam("NextToken")
	maxResults := 0

	if v := c.QueryParam("MaxResults"); v != "" {
		maxResults, _ = strconv.Atoi(v)
	}

	policies, next := h.Backend.ListConfigurationPolicies(nextToken, maxResults)

	var out []map[string]any //nolint:prealloc // existing issue.

	for _, p := range policies {
		out = append(out, map[string]any{
			"Arn":            p.Arn,
			"Id":             p.Id,
			"Name":           p.Name,
			"Description":    p.Description,
			"UpdatedAt":      p.UpdatedAt, //nolint:goconst // existing issue.
			"ServiceEnabled": configPolicyServiceEnabled(p),
		})
	}

	if out == nil {
		out = []map[string]any{}
	}

	// Real key is "ConfigurationPolicySummaries" (securityhub@v1.75.4
	// deserializers.go, awsRestjson1_deserializeOpDocumentListConfigurationPoliciesOutput)
	// -- a real client's typed field was always empty regardless of backend
	// state.
	resp := map[string]any{"ConfigurationPolicySummaries": out}

	if next != "" {
		resp["NextToken"] = next
	}

	return c.JSON(http.StatusOK, resp)
}

// Configuration policy association target-type wire values (TargetType enum).
const (
	targetTypeAccount            = "ACCOUNT"
	targetTypeOrganizationalUnit = "ORGANIZATIONAL_UNIT"
	targetTypeRoot               = "ROOT"
)

// extractConfigPolicyTarget reads the Target union from a request body and
// derives (targetID, targetType). The wire shape is a tagged union --
// {"AccountId": ...} | {"OrganizationalUnitId": ...} | {"RootId": ...} --
// with no client-supplied "TargetType" field; TargetType must be derived from
// which key is present, matching aws-sdk-go-v2's types.Target variants.
func extractConfigPolicyTarget(body map[string]any) (string, string) {
	t, isMap := body["Target"].(map[string]any)
	if !isMap {
		return "", ""
	}

	if v, isStr := t["AccountId"].(string); isStr && v != "" {
		return v, targetTypeAccount
	}

	if v, isStr := t["OrganizationalUnitId"].(string); isStr && v != "" {
		return v, targetTypeOrganizationalUnit
	}

	if v, isStr := t["RootId"].(string); isStr && v != "" {
		return v, targetTypeRoot
	}

	return "", ""
}

func (h *Handler) handleGetConfigurationPolicyAssociation(c *echo.Context, body map[string]any) error {
	targetID, targetType := extractConfigPolicyTarget(body)

	assoc, err := h.Backend.GetConfigurationPolicyAssociation(targetID, targetType)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return typedErrorResponse(c, http.StatusNotFound, "ResourceNotFoundException", "Association not found")
		}

		return typedErrorResponse(c, http.StatusInternalServerError, "InternalException", err.Error())
	}

	return c.JSON(http.StatusOK, configPolicyAssocToResponse(assoc))
}

func (h *Handler) handleListConfigurationPolicyAssociations(c *echo.Context, body map[string]any) error {
	filterPolicyID := ""
	filterType := ""
	nextToken := ""
	maxResults := 0

	if f, ok := body["Filters"].(map[string]any); ok {
		filterPolicyID, _ = f["ConfigurationPolicyId"].(string)
		filterType, _ = f["AssociationType"].(string)
	}

	if v, ok := body["NextToken"].(string); ok {
		nextToken = v
	}

	if v, ok := body[keyMaxResults].(float64); ok {
		maxResults = int(v)
	}

	assocs, next := h.Backend.ListConfigurationPolicyAssociations(filterPolicyID, filterType, nextToken, maxResults)

	var out []map[string]any //nolint:prealloc // existing issue.

	for _, a := range assocs {
		out = append(out, configPolicyAssocToResponse(a))
	}

	if out == nil {
		out = []map[string]any{}
	}

	// Real key is "ConfigurationPolicyAssociationSummaries" (securityhub@v1.75.4
	// deserializers.go), same wrapper-key bug as ListConfigurationPolicies above.
	resp := map[string]any{"ConfigurationPolicyAssociationSummaries": out}

	if next != "" {
		resp["NextToken"] = next
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleStartConfigurationPolicyAssociation(c *echo.Context, body map[string]any) error {
	policyID := ""

	if t, ok := body["ConfigurationPolicyIdentifier"].(string); ok {
		policyID = t
	}

	targetID, targetType := extractConfigPolicyTarget(body)

	assoc, err := h.Backend.StartConfigurationPolicyAssociation(policyID, targetID, targetType)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return typedErrorResponse(
				c,
				http.StatusNotFound,
				"ResourceNotFoundException",
				"Configuration policy not found",
			)
		}

		return typedErrorResponse(c, http.StatusInternalServerError, "InternalException", err.Error())
	}

	return c.JSON(http.StatusOK, configPolicyAssocToResponse(assoc))
}

func (h *Handler) handleStartConfigurationPolicyDisassociation(c *echo.Context, body map[string]any) error {
	policyID := ""

	if t, ok := body["ConfigurationPolicyIdentifier"].(string); ok {
		policyID = t
	}

	targetID, targetType := extractConfigPolicyTarget(body)

	if err := h.Backend.StartConfigurationPolicyDisassociation(policyID, targetID, targetType); err != nil {
		return typedErrorResponse(c, http.StatusInternalServerError, "InternalException", err.Error())
	}

	return c.JSON(http.StatusOK, map[string]any{})
}

func (h *Handler) handleBatchGetConfigurationPolicyAssociations(c *echo.Context, body map[string]any) error {
	var requests []map[string]any

	if raw, ok := body["ConfigurationPolicyAssociationIdentifiers"].([]any); ok {
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok { //nolint:govet // existing issue.
				requests = append(requests, m)
			}
		}
	}

	found, unprocessed := h.Backend.BatchGetConfigurationPolicyAssociations(requests)

	var out []map[string]any //nolint:prealloc // existing issue.

	for _, a := range found {
		out = append(out, configPolicyAssocToResponse(a))
	}

	if out == nil {
		out = []map[string]any{}
	}

	resp := map[string]any{
		"ConfigurationPolicyAssociations": out,
	}

	if len(unprocessed) > 0 {
		resp["UnprocessedConfigurationPolicyAssociations"] = unprocessed
	} else {
		resp["UnprocessedConfigurationPolicyAssociations"] = []any{}
	}

	return c.JSON(http.StatusOK, resp)
}

// configPolicyServiceEnabled extracts ServiceEnabled from the opaque
// ConfigurationPolicy document -- a real ConfigurationPolicySummary member
// (securityhub@v1.75.4 deserializers.go's ConfigurationPolicySummary case
// list). The policy document is a Smithy tagged union whose only real
// variant is SecurityHub (types.PolicyMemberSecurityHub), so the value is
// already on hand under p.ConfigurationPolicy["SecurityHub"]["ServiceEnabled"]
// -- this was never read into the List summary before.
func configPolicyServiceEnabled(p *ConfigurationPolicy) bool {
	sh, ok := p.ConfigurationPolicy["SecurityHub"].(map[string]any)
	if !ok {
		return false
	}

	enabled, _ := sh["ServiceEnabled"].(bool)

	return enabled
}

func configPolicyToResponse(p *ConfigurationPolicy) map[string]any {
	return map[string]any{
		"Arn":                 p.Arn,
		"Id":                  p.Id,
		"Name":                p.Name,
		"Description":         p.Description,
		"CreatedAt":           p.CreatedAt,
		"UpdatedAt":           p.UpdatedAt,
		"ConfigurationPolicy": p.ConfigurationPolicy,
	}
}

func configPolicyAssocToResponse(a *ConfigurationPolicyAssociation) map[string]any {
	return map[string]any{
		"ConfigurationPolicyId":    a.ConfigurationPolicyId,
		"TargetId":                 a.TargetId,
		"TargetType":               a.TargetType,
		"AssociationType":          a.AssociationType,
		"UpdatedAt":                a.UpdatedAt,
		"AssociationStatus":        a.AssociationStatus,
		"AssociationStatusMessage": a.AssociationStatusMessage,
	}
}

// configPolicyOpHandlers returns the Configuration Policy operation dispatch
// table for handleREST.
func (h *Handler) configPolicyOpHandlers(
	c *echo.Context,
	resource string,
	body map[string]any,
) map[string]func() error {
	return map[string]func() error{
		opCreateConfigurationPolicy:         func() error { return h.handleCreateConfigurationPolicy(c, body) },
		opGetConfigurationPolicy:            func() error { return h.handleGetConfigurationPolicy(c, resource) },
		opDeleteConfigurationPolicy:         func() error { return h.handleDeleteConfigurationPolicy(c, resource) },
		opListConfigurationPolicies:         func() error { return h.handleListConfigurationPolicies(c) },
		opGetConfigurationPolicyAssociation: func() error { return h.handleGetConfigurationPolicyAssociation(c, body) },
		opUpdateConfigurationPolicy: func() error {
			return h.handleUpdateConfigurationPolicy(c, resource, body)
		},
		opListConfigurationPolicyAssociations: func() error {
			return h.handleListConfigurationPolicyAssociations(c, body)
		},
		opStartConfigurationPolicyAssociation: func() error {
			return h.handleStartConfigurationPolicyAssociation(c, body)
		},
		opStartConfigurationPolicyDisassociation: func() error {
			return h.handleStartConfigurationPolicyDisassociation(c, body)
		},
		opBatchGetConfigurationPolicyAssociations: func() error {
			return h.handleBatchGetConfigurationPolicyAssociations(c, body)
		},
	}
}
