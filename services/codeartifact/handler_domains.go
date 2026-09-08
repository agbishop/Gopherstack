package codeartifact

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

type createDomainBody struct {
	EncryptionKey string           `json:"encryptionKey"`
	Tags          []map[string]any `json:"tags"`
}

func domainToMap(d *Domain, repoCount int) map[string]any {
	m := map[string]any{
		keyArn:            d.ARN,
		keyName:           d.Name,
		"owner":           d.Owner,
		keyStatusField:    d.Status,
		keyCreatedTime:    epochSeconds(d.CreatedTime),
		"assetSizeBytes":  d.AssetSizeBytes,
		"repositoryCount": repoCount,
	}
	if d.EncryptionKey != "" {
		m["encryptionKey"] = d.EncryptionKey
	}
	if d.S3BucketARN != "" {
		m["s3BucketArn"] = d.S3BucketARN
	}

	return m
}

func domainSummaryToMap(d *Domain) map[string]any {
	m := map[string]any{
		keyArn:         d.ARN,
		keyName:        d.Name,
		"owner":        d.Owner,
		keyStatusField: d.Status,
		keyCreatedTime: epochSeconds(d.CreatedTime),
	}
	if d.EncryptionKey != "" {
		m["encryptionKey"] = d.EncryptionKey
	}

	return m
}

func (h *Handler) handleCreateDomain(c *echo.Context, name string, body []byte) error {
	if name == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain name is required"))
	}

	var in createDomainBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("ValidationException", "invalid request body"))
		}
	}

	d, err := h.Backend.CreateDomain(c.Request().Context(), name, in.EncryptionKey, tagsFromSlice(in.Tags))
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyDomain: domainToMap(d, 0),
	})
}

func (h *Handler) handleDescribeDomain(c *echo.Context, name string) error {
	if name == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain name is required"))
	}

	d, err := h.Backend.DescribeDomain(c.Request().Context(), name)
	if err != nil {
		return h.handleError(c, err)
	}

	repoCount := h.Backend.CountRepositoriesInDomain(c.Request().Context(), name)

	return c.JSON(http.StatusOK, map[string]any{
		keyDomain: domainToMap(d, repoCount),
	})
}

// listDomainsBody is ListDomains' request shape. Unlike every other List op in this
// service, ListDomains sends maxResults/nextToken as JSON body fields (it is the only
// List op whose Smithy model has no httpQuery bindings at all) rather than as
// "max-results"/"next-token" query params -- verified against aws-sdk-go-v2 serializers.go.

type listDomainsBody struct {
	NextToken  string `json:"nextToken"`
	MaxResults int    `json:"maxResults"`
}

func (h *Handler) handleListDomains(c *echo.Context, body []byte) error {
	var in listDomainsBody
	if len(body) > 0 {
		_ = json.Unmarshal(body, &in)
	}

	all := h.Backend.ListDomains(c.Request().Context())
	page, next := paginateSlice(all, in.MaxResults, in.NextToken, func(d *Domain) string { return d.Name })

	items := make([]map[string]any, 0, len(page))
	for _, d := range page {
		items = append(items, domainSummaryToMap(d))
	}

	resp := map[string]any{"domains": items}
	if next != "" {
		resp["nextToken"] = next
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleDeleteDomain(c *echo.Context, name string) error {
	if name == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain name is required"))
	}

	repoCount := h.Backend.CountRepositoriesInDomain(c.Request().Context(), name)

	d, err := h.Backend.DeleteDomain(c.Request().Context(), name)
	if err != nil {
		return h.handleError(c, err)
	}

	if d == nil {
		// Idempotent: no domain existed to describe. DeleteDomainOutput.Domain
		// is a nilable pointer on the wire, so omitting it is not a fabrication.
		return c.JSON(http.StatusOK, map[string]any{})
	}

	return c.JSON(http.StatusOK, map[string]any{
		keyDomain: domainToMap(d, repoCount),
	})
}

func (h *Handler) handleGetAuthorizationToken(c *echo.Context, domainName string) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}

	_, err := h.Backend.DescribeDomain(c.Request().Context(), domainName)
	if err != nil {
		return h.handleError(c, err)
	}

	// Return a plausible stub token.
	return c.JSON(http.StatusOK, map[string]any{
		"authorizationToken": "codeartifact-stub-token-" + domainName,
		"expiration":         epochSeconds(time.Now().Add(stubTokenExpireHours * time.Hour)),
	})
}

func (h *Handler) handleGetDomainPermissionsPolicy(c *echo.Context, domainName string) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}

	pol, err := h.Backend.GetDomainPermissionsPolicy(c.Request().Context(), domainName)
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

type putDomainPermissionsPolicyBody struct {
	PolicyDocument string `json:"policyDocument"`
	PolicyRevision string `json:"policyRevision"`
}

func (h *Handler) handlePutDomainPermissionsPolicy(c *echo.Context, domainName string, body []byte) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}

	var in putDomainPermissionsPolicyBody
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return c.JSON(http.StatusBadRequest, errResp("ValidationException", "invalid request body"))
		}
	}

	// PolicyDocument is "This member is required." on the real
	// PutDomainPermissionsPolicyInput (api_op_PutDomainPermissionsPolicy.go)
	// -- was silently defaulted to an empty-statement policy instead of
	// rejected, accepting a request real AWS would 400 on.
	if in.PolicyDocument == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "policyDocument is required"))
	}

	pol, err := h.Backend.PutDomainPermissionsPolicy(
		c.Request().Context(), domainName, in.PolicyDocument, in.PolicyRevision,
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

func (h *Handler) handleDeleteDomainPermissionsPolicy(c *echo.Context, domainName string) error {
	if domainName == "" {
		return c.JSON(http.StatusBadRequest, errResp("ValidationException", "domain is required"))
	}

	revision := c.Request().URL.Query().Get("policy-revision")

	pol, err := h.Backend.DeleteDomainPermissionsPolicy(c.Request().Context(), domainName, revision)
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
