package codeartifact

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"
)

type upstreamRepoEntry struct {
	RepositoryName string `json:"repositoryName"`
}

// createRepositoryBody's Upstreams field uses the wire key "upstreams" (verified against
// aws-sdk-go-v2 serializers.go's awsRestjson1_serializeOpDocumentCreateRepositoryInput) --
// NOT "upstreamRepositories" as the RepositoryDescription.Upstreams Go field name might
// suggest.

type createRepositoryBody struct {
	Description string              `json:"description"`
	Tags        []map[string]any    `json:"tags"`
	Upstreams   []upstreamRepoEntry `json:"upstreams"`
}

func repoToMap(r *Repository, connections []ExternalConnection) map[string]any {
	m := map[string]any{
		keyArn:                 r.ARN,
		keyName:                r.Name,
		keyDomainName:          r.DomainName,
		keyDomainOwner:         r.DomainOwner,
		"administratorAccount": r.AdministratorAccount,
		// createdTime is a real, always-present RepositoryDescription member
		// (deserializers.go's awsRestjson1_deserializeDocumentRepositoryDescription)
		// -- the backend already tracks r.CreatedTime, it was just never emitted.
		keyCreatedTime: epochSeconds(r.CreatedTime),
	}
	if r.Description != "" {
		m["description"] = r.Description
	}

	extConns := make([]map[string]any, 0, len(connections))
	for _, ec := range connections {
		extConns = append(extConns, map[string]any{
			"externalConnectionName": ec.ExternalConnectionName,
			"packageFormat":          ec.PackageFormat,
			keyStatusField:           ec.Status,
		})
	}
	m["externalConnections"] = extConns

	upstreams := make([]map[string]string, 0, len(r.UpstreamRepositories))
	for _, name := range r.UpstreamRepositories {
		upstreams = append(upstreams, map[string]string{"repositoryName": name})
	}
	// Wire key is "upstreams", not "upstreamRepositories" -- verified against
	// aws-sdk-go-v2 deserializers.go's awsRestjson1_deserializeDocumentRepositoryDescription.
	m["upstreams"] = upstreams

	return m
}

func (h *Handler) handleCreateRepository(c *echo.Context, domainName, repoName string, body []byte) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}

	var in createRepositoryBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("ValidationException", "invalid request body"))
		}
	}

	upstreams := make([]string, 0, len(in.Upstreams))
	for _, u := range in.Upstreams {
		upstreams = append(upstreams, u.RepositoryName)
	}

	r, err := h.Backend.CreateRepository(
		c.Request().Context(),
		domainName,
		repoName,
		in.Description,
		tagsFromSlice(in.Tags),
		upstreams,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyRepository: repoToMap(r, h.Backend.GetExternalConnections(c.Request().Context(), domainName, repoName)),
	})
}

func (h *Handler) handleDescribeRepository(c *echo.Context, domainName, repoName string) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}

	r, err := h.Backend.DescribeRepository(c.Request().Context(), domainName, repoName)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyRepository: repoToMap(r, h.Backend.GetExternalConnections(c.Request().Context(), domainName, repoName)),
	})
}

func (h *Handler) handleDeleteRepository(c *echo.Context, domainName, repoName string) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}

	conns := h.Backend.GetExternalConnections(c.Request().Context(), domainName, repoName)

	r, err := h.Backend.DeleteRepository(c.Request().Context(), domainName, repoName)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyRepository: repoToMap(r, conns),
	})
}

// repositorySummaryToMap builds the types.RepositorySummary shape -- verified
// against aws-sdk-go-v2 deserializers.go's
// awsRestjson1_deserializeDocumentRepositorySummary (arn/name/domainName/
// domainOwner/administratorAccount/createdTime/description). Both List ops
// below previously emitted only 4 of these 7 real fields, silently dropping
// administratorAccount/createdTime/description even though the backend
// already tracks all three on Repository.
func repositorySummaryToMap(r *Repository) map[string]any {
	m := map[string]any{
		keyArn:                 r.ARN,
		keyName:                r.Name,
		keyDomainName:          r.DomainName,
		keyDomainOwner:         r.DomainOwner,
		"administratorAccount": r.AdministratorAccount,
		keyCreatedTime:         epochSeconds(r.CreatedTime),
	}
	if r.Description != "" {
		m["description"] = r.Description
	}

	return m
}

func (h *Handler) handleListRepositoriesInDomain(c *echo.Context, domainName string) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}

	q := c.Request().URL.Query()
	maxResults := parseMaxResults(q.Get("max-results"))
	nextToken := q.Get("next-token")
	// repository-prefix and administrator-account are real
	// ListRepositoriesInDomainInput filter members (serializers.go's
	// SetQuery("repository-prefix")/SetQuery("administrator-account")) that
	// were silently discarded -- every call returned every repository in the
	// domain regardless of either filter.
	repositoryPrefix := q.Get("repository-prefix")
	administratorAccount := q.Get("administrator-account")

	all, err := h.Backend.ListRepositoriesInDomain(
		c.Request().Context(), domainName, repositoryPrefix, administratorAccount,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	page, next := paginateSlice(all, maxResults, nextToken, func(r *Repository) string { return r.Name })

	items := make([]map[string]any, 0, len(page))
	for _, r := range page {
		items = append(items, repositorySummaryToMap(r))
	}

	resp := map[string]any{"repositories": items}
	if next != "" {
		resp["nextToken"] = next
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleListRepositories(c *echo.Context) error {
	q := c.Request().URL.Query()
	maxResults := parseMaxResults(q.Get("max-results"))
	nextToken := q.Get("next-token")
	// repository-prefix is a real ListRepositoriesInput filter member
	// (serializers.go's SetQuery("repository-prefix")) that was silently
	// discarded -- every call returned every repository account-wide
	// regardless of the filter.
	repositoryPrefix := q.Get("repository-prefix")

	all := h.Backend.ListRepositories(c.Request().Context(), repositoryPrefix)
	page, next := paginateSlice(all, maxResults, nextToken, func(r *Repository) string { return r.Name })

	items := make([]map[string]any, 0, len(page))
	for _, r := range page {
		items = append(items, repositorySummaryToMap(r))
	}

	resp := map[string]any{"repositories": items}
	if next != "" {
		resp["nextToken"] = next
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleGetRepositoryEndpoint(c *echo.Context, domainName, repoName, format string) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}
	if format == "" {
		format = "generic"
	}

	_, err := h.Backend.DescribeRepository(c.Request().Context(), domainName, repoName)
	if err != nil {
		return h.handleError(c, err)
	}

	endpoint := fmt.Sprintf(
		"https://%s-%s.d.codeartifact.%s.amazonaws.com/%s/%s/",
		domainName, h.Backend.accountID, h.Backend.region, format, repoName,
	)

	return c.JSON(http.StatusOK, map[string]any{
		"repositoryEndpoint": endpoint,
	})
}

func (h *Handler) handleAssociateExternalConnection(
	c *echo.Context,
	domainName, repoName, connectionName string,
) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}
	if connectionName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "externalConnection is required"))
	}

	r, err := h.Backend.AssociateExternalConnection(c.Request().Context(), domainName, repoName, connectionName)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyRepository: repoToMap(r, h.Backend.GetExternalConnections(c.Request().Context(), domainName, repoName)),
	})
}

func (h *Handler) handleGetRepositoryPermissionsPolicy(c *echo.Context, domainName, repoName string) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}

	pol, err := h.Backend.GetRepositoryPermissionsPolicy(c.Request().Context(), domainName, repoName)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyPolicy: map[string]any{
			keyDocument:    pol.Document,
			keyRevision:    pol.Revision,
			keyResourceArn: pol.ResourceARN,
		},
	})
}

type putRepositoryPermissionsPolicyBody struct {
	PolicyDocument string `json:"policyDocument"`
	PolicyRevision string `json:"policyRevision"`
}

func (h *Handler) handlePutRepositoryPermissionsPolicy(
	c *echo.Context,
	domainName, repoName string,
	body []byte,
) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}

	var in putRepositoryPermissionsPolicyBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("ValidationException", "invalid request body"))
		}
	}

	// PolicyDocument is "This member is required." on the real
	// PutRepositoryPermissionsPolicyInput
	// (api_op_PutRepositoryPermissionsPolicy.go) -- was silently defaulted to
	// an empty-statement policy instead of rejected, accepting a request
	// real AWS would 400 on.
	if in.PolicyDocument == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "policyDocument is required"))
	}

	pol, err := h.Backend.PutRepositoryPermissionsPolicy(
		c.Request().Context(), domainName, repoName, in.PolicyDocument, in.PolicyRevision,
	)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyPolicy: map[string]any{
			keyDocument:    pol.Document,
			keyRevision:    pol.Revision,
			keyResourceArn: pol.ResourceARN,
		},
	})
}

func (h *Handler) handleDeleteRepositoryPermissionsPolicy(c *echo.Context, domainName, repoName string) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}

	revision := c.Request().URL.Query().Get("policy-revision")

	pol, err := h.Backend.DeleteRepositoryPermissionsPolicy(c.Request().Context(), domainName, repoName, revision)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyPolicy: map[string]any{
			keyDocument:    pol.Document,
			keyRevision:    pol.Revision,
			keyResourceArn: pol.ResourceARN,
		},
	})
}

func (h *Handler) handleDisassociateExternalConnection(
	c *echo.Context, domainName, repoName, connectionName string,
) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}
	if connectionName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "externalConnection is required"))
	}

	r, err := h.Backend.DisassociateExternalConnection(c.Request().Context(), domainName, repoName, connectionName)
	if err != nil {
		return h.handleError(c, err)
	}

	extConns := h.Backend.GetExternalConnections(c.Request().Context(), domainName, repoName)

	return c.JSON(http.StatusOK, map[string]any{keyRepository: repoToMap(r, extConns)})
}

type updateRepositoryBody struct {
	Description string              `json:"description"`
	Upstreams   []upstreamRepoEntry `json:"upstreams"`
}

func (h *Handler) handleUpdateRepository(c *echo.Context, domainName, repoName string, body []byte) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}
	if repoName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "repository is required"))
	}

	var in updateRepositoryBody
	if len(body) > 0 {
		_ = json.Unmarshal(body, &in)
	}

	var upstreams []string
	if in.Upstreams != nil {
		upstreams = make([]string, 0, len(in.Upstreams))
		for _, u := range in.Upstreams {
			upstreams = append(upstreams, u.RepositoryName)
		}
	}

	r, err := h.Backend.UpdateRepository(c.Request().Context(), domainName, repoName, in.Description, upstreams)
	if err != nil {
		return h.handleError(c, err)
	}

	extConns := h.Backend.GetExternalConnections(c.Request().Context(), domainName, repoName)

	return c.JSON(http.StatusOK, map[string]any{keyRepository: repoToMap(r, extConns)})
}
