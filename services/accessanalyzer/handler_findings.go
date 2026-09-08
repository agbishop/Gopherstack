package accessanalyzer

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

const (
	opGetFinding                    = "GetFinding"
	opListFindings                  = "ListFindings"
	opUpdateFindings                = "UpdateFindings"
	opGetFindingV2                  = "GetFindingV2"
	opListFindingsV2                = "ListFindingsV2"
	opGetFindingsStatistics         = "GetFindingsStatistics"
	opGenerateFindingRecommendation = "GenerateFindingRecommendation"
	opGetFindingRecommendation      = "GetFindingRecommendation"

	pathFindingV2      = "findingv2"
	pathRecommendation = "recommendation"

	keyFinding           = "finding"
	keyFindings          = "findings"
	keyResource          = "resource"
	keyResourceOwnerAcct = "resourceOwnerAccount"
	keyFindingType       = "findingType"

	// findingTypeExternalAccess is the only types.FindingType value
	// InMemoryBackend ever produces: every finding created via AddFinding
	// carries the Principal/Condition/Action/IsPublic shape of an external
	// access finding (types.ExternalAccessDetails), never an
	// unused-access/internal-access one, so GetFindingV2/ListFindingsV2 can
	// always report it truthfully rather than leaving findingType empty.
	findingTypeExternalAccess = "ExternalAccess"
)

// dispatchFindingOps routes finding operations (v1, v2, statistics, and recommendations).
func (h *Handler) dispatchFindingOps(op, path, query string, body []byte) (any, int, bool, error) {
	switch op {
	case opGetFinding:
		r, c, e := h.handleGetFinding(path, query)

		return r, c, true, e
	case opListFindings:
		r, c, e := h.handleListFindings(body)

		return r, c, true, e
	case opUpdateFindings:
		c, e := h.handleUpdateFindings(body)

		return nil, c, true, e
	case opGetFindingV2:
		r, c, e := h.handleGetFindingV2(path, query)

		return r, c, true, e
	case opListFindingsV2:
		r, c, e := h.handleListFindingsV2(body)

		return r, c, true, e
	case opGetFindingsStatistics:
		r, c, e := h.handleGetFindingsStatistics(body)

		return r, c, true, e
	case opGenerateFindingRecommendation:
		c, e := h.handleGenerateFindingRecommendation(path, query)

		return nil, c, true, e
	case opGetFindingRecommendation:
		r, c, e := h.handleGetFindingRecommendation(path, query)

		return r, c, true, e
	}

	return nil, 0, false, nil
}

// ---- operation handlers ----

// handleGetFinding serves GET /finding/{id}?analyzerArn=... . Unlike the
// analyzer/archive-rule/finding-family ops nested under /analyzer/{name}/...,
// the real GetFinding endpoint carries the owning analyzer as an ARN query
// parameter, not a path segment (see aws-sdk-go-v2's
// awsRestjson1_serializeOpHttpBindingsGetFindingInput).
func (h *Handler) handleGetFinding(path, query string) (any, int, error) {
	findingID := extractLastSegment(path, pathFinding)
	analyzerArn := queryParamValue(query, keyAnalyzerArn)
	analyzerName := analyzerNameFromArn(analyzerArn)

	f, err := h.Backend.GetFinding(analyzerName, findingID)
	if err != nil {
		return nil, 0, err
	}

	return map[string]any{keyFinding: findingToJSON(f, h.Backend.AccountID())}, http.StatusOK, nil
}

// handleListFindings serves POST /finding. The owning analyzer is carried as
// an ARN in the JSON body (analyzerArn), not a path segment.
func (h *Handler) handleListFindings(body []byte) (any, int, error) {
	var req struct {
		Filter      map[string]FilterCriterion `json:"filter"`
		Sort        *FindingSortCriteria       `json:"sort"`
		AnalyzerArn string                     `json:"analyzerArn"`
		NextToken   string                     `json:"nextToken"`
		Status      string                     `json:"status"`
		MaxResults  int                        `json:"maxResults"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, 0, ErrValidation
	}

	if req.AnalyzerArn == "" {
		return nil, 0, ErrValidation
	}

	analyzerName := analyzerNameFromArn(req.AnalyzerArn)

	findings, nextToken, err := h.Backend.ListFindings(
		analyzerName, req.Filter, req.Status, req.Sort, req.MaxResults, req.NextToken,
	)
	if err != nil {
		return nil, 0, err
	}

	accountID := h.Backend.AccountID()
	list := make([]any, 0, len(findings))

	for _, f := range findings {
		list = append(list, findingToJSON(f, accountID))
	}

	resp := map[string]any{keyFindings: list}

	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return resp, http.StatusOK, nil
}

// handleUpdateFindings serves PUT /finding. The owning analyzer is carried as
// an ARN in the JSON body (analyzerArn), not a path segment.
func (h *Handler) handleUpdateFindings(body []byte) (int, error) {
	var req struct {
		AnalyzerArn string   `json:"analyzerArn"`
		Status      string   `json:"status"`
		ResourceArn string   `json:"resourceArn"`
		IDs         []string `json:"ids"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return 0, ErrValidation
	}

	if req.AnalyzerArn == "" {
		return 0, ErrValidation
	}

	analyzerName := analyzerNameFromArn(req.AnalyzerArn)

	if err := h.Backend.UpdateFindings(analyzerName, req.IDs, req.ResourceArn, FindingStatus(req.Status)); err != nil {
		return 0, err
	}

	return http.StatusOK, nil
}

func (h *Handler) handleGetFindingV2(path, query string) (any, int, error) {
	findingID := extractLastSegment(path, pathFindingV2)
	analyzerArn := queryParamValue(query, keyAnalyzerArn)

	f, err := h.Backend.GetFindingV2(analyzerArn, findingID)
	if err != nil {
		return nil, 0, err
	}

	return map[string]any{
		"id":                 f.ID,
		keyAnalyzerArn:       f.AnalyzerArn,
		keyStatus:            string(f.Status),
		keyResourceType:      f.ResourceType,
		keyResource:          f.ResourceArn,
		keyResourceOwnerAcct: h.Backend.AccountID(),
		keyAnalyzedAt:        f.UpdatedAt.Format(time.RFC3339),
		keyCreatedAt:         f.CreatedAt.Format(time.RFC3339),
		keyUpdatedAt:         f.UpdatedAt.Format(time.RFC3339),
		keyFindingType:       findingTypeExternalAccess,
		"findingDetails":     findingDetailsV2JSON(f),
	}, http.StatusOK, nil
}

func (h *Handler) handleListFindingsV2(body []byte) (any, int, error) {
	var req struct {
		Filter      map[string]FilterCriterion `json:"filter"`
		Sort        *FindingSortCriteria       `json:"sort"`
		AnalyzerArn string                     `json:"analyzerArn"`
		NextToken   string                     `json:"nextToken"`
		Status      string                     `json:"status"`
		MaxResults  int                        `json:"maxResults"`
	}

	_ = json.Unmarshal(body, &req)

	findings, nextToken, err := h.Backend.ListFindingsV2(
		req.AnalyzerArn, req.Status, req.Filter, req.Sort, req.MaxResults, req.NextToken,
	)
	if err != nil {
		return nil, 0, err
	}

	accountID := h.Backend.AccountID()
	list := make([]any, 0, len(findings))

	for _, f := range findings {
		list = append(list, map[string]any{
			"id":                 f.ID,
			keyAnalyzerArn:       f.AnalyzerArn,
			keyStatus:            string(f.Status),
			keyResourceType:      f.ResourceType,
			keyResource:          f.ResourceArn,
			keyResourceOwnerAcct: accountID,
			keyAnalyzedAt:        f.UpdatedAt.Format(time.RFC3339),
			keyUpdatedAt:         f.UpdatedAt.Format(time.RFC3339),
			keyCreatedAt:         f.CreatedAt.Format(time.RFC3339),
			keyFindingType:       findingTypeExternalAccess,
		})
	}

	resp := map[string]any{keyFindings: list}

	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return resp, http.StatusOK, nil
}

// handleGetFindingsStatistics serves POST /analyzer/findings/statistics.
// types.ExternalAccessFindingsStatistics wire-serializes its counters as
// three flat integers -- totalActiveFindings/totalArchivedFindings/
// totalResolvedFindings -- not the {"total": N} nested-object shape this
// used to emit (which no real deserializer recognizes: see
// awsRestjson1_deserializeDocumentExternalAccessFindingsStatistics in the
// SDK's deserializers.go).
//
// types.FindingsStatistics is a union keyed by wire name
// (awsRestjson1_deserializeDocumentFindingsStatistics, deserializers.go
// ~L9169): "externalAccessFindingsStatistics" for ACCOUNT/ORGANIZATION
// analyzers, "unusedAccessFindingsStatistics" for
// ACCOUNT_UNUSED_ACCESS/ORGANIZATION_UNUSED_ACCESS ones -- a real client's
// typed union switch would land on the wrong branch (and decode into the
// wrong Go type entirely) if this always emitted the external-access key.
// Select the wire key from the target analyzer's own Type.
// UnusedAccessFindingsStatistics.TopAccounts/UnusedAccessTypeStatistics are
// left unset: neither is backed by any state InMemoryBackend tracks (no
// per-principal-account aggregation, no unused-access-type categorization),
// so there is no honest non-empty value to synthesize for them.
func (h *Handler) handleGetFindingsStatistics(body []byte) (any, int, error) {
	var req struct {
		AnalyzerArn string `json:"analyzerArn"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, 0, ErrValidation
	}

	if req.AnalyzerArn == "" {
		return nil, 0, ErrValidation
	}

	analyzer, err := h.Backend.GetAnalyzer(analyzerNameFromArn(req.AnalyzerArn))
	if err != nil {
		return nil, 0, err
	}

	counts, err := h.Backend.GetFindingsStatistics(req.AnalyzerArn)
	if err != nil {
		return nil, 0, err
	}

	countsJSON := map[string]any{
		"totalActiveFindings":   counts[string(FindingStatusActive)],
		"totalArchivedFindings": counts[string(FindingStatusArchived)],
		"totalResolvedFindings": counts[string(FindingStatusResolved)],
	}

	statsKey := "externalAccessFindingsStatistics"
	if analyzer.Type == AnalyzerTypeAccountUnusedAccess || analyzer.Type == AnalyzerTypeOrganizationUnusedAccess {
		statsKey = "unusedAccessFindingsStatistics"
	}

	return map[string]any{"findingsStatistics": []any{map[string]any{statsKey: countsJSON}}}, http.StatusOK, nil
}

// handleGenerateFindingRecommendation serves POST /recommendation/{id}.
// AnalyzerArn is a query parameter, not a body member --
// awsRestjson1_serializeOpHttpBindingsGenerateFindingRecommendationInput
// (aws-sdk-go-v2/service/accessanalyzer@v1.51.4 serializers.go) sends no
// request body at all for this operation, so decoding AnalyzerArn from the
// body meant it was always empty and (since the SDK sends zero body bytes)
// json.Unmarshal failed on every real call, returning ValidationException
// regardless of whether the finding existed.
func (h *Handler) handleGenerateFindingRecommendation(path, query string) (int, error) {
	findingID := extractLastSegment(path, pathRecommendation)
	analyzerArn := queryParamValue(query, keyAnalyzerArn)

	if err := h.Backend.GenerateFindingRecommendation(analyzerArn, findingID); err != nil {
		if errors.Is(err, ErrFindingNotFound) {
			// GenerateFindingRecommendation's own deserializeOpError switch
			// (aws-sdk-go-v2/service/accessanalyzer@v1.51.4 deserializers.go) does
			// not type ResourceNotFoundException -- only AccessDeniedException,
			// InternalServerException, ThrottlingException, ValidationException. An
			// unrecognized findingId is reported as invalid input, matching
			// account's errRegionNotFound precedent.
			return 0, ErrValidation
		}

		return 0, err
	}

	return http.StatusOK, nil
}

func (h *Handler) handleGetFindingRecommendation(path, query string) (any, int, error) {
	findingID := extractLastSegment(path, pathRecommendation)
	analyzerArn := queryParamValue(query, keyAnalyzerArn)

	rec, err := h.Backend.GetFindingRecommendation(analyzerArn, findingID)
	if err != nil {
		return nil, 0, err
	}

	resp := map[string]any{
		"recommendedSteps":   []any{},
		"recommendationType": rec.RecommendationType,
		keyResourceArn:       rec.ResourceArn,
		"startedAt":          rec.StartedAt.Format(time.RFC3339),
		keyStatus:            rec.Status,
	}

	if rec.CompletedAt != nil {
		resp["completedAt"] = rec.CompletedAt.Format(time.RFC3339)
	}

	return resp, http.StatusOK, nil
}

// ---- URL path parsing ----

// parseFindingPath parses GetFinding/ListFindings/UpdateFindings paths.
// Unlike the analyzer/archive-rule family, these live at the top-level
// /finding and /finding/{id} -- the owning analyzer is carried as an ARN in
// the query string (Get) or JSON body (List/Update), never as a path
// segment. See aws-sdk-go-v2's serializers.go for GetFinding ("/finding/{id}"
// GET), ListFindings ("/finding" POST), UpdateFindings ("/finding" PUT).
func parseFindingPath(method string, segments []string) (string, string) {
	switch len(segments) {
	case 1:
		switch method {
		case http.MethodPost:
			return opListFindings, ""
		case http.MethodPut:
			return opUpdateFindings, ""
		}
	case segmentDepthResource:
		if method == http.MethodGet {
			return opGetFinding, segments[1]
		}
	}

	return opUnknown, ""
}

func parseFindingV2Path(method string, segments []string) (string, string, bool) {
	switch len(segments) {
	case 1:
		if method == http.MethodPost {
			return opListFindingsV2, "", true
		}
	case segmentDepthResource:
		if method == http.MethodGet {
			return opGetFindingV2, segments[1], true
		}
	}

	return "", "", false
}

func parseRecommendationPath(method string, segments []string) (string, string, bool) {
	if len(segments) != segmentDepthResource {
		return "", "", false
	}

	switch method {
	case http.MethodPost:
		return opGenerateFindingRecommendation, segments[1], true
	case http.MethodGet:
		return opGetFindingRecommendation, segments[1], true
	}

	return "", "", false
}

// ---- JSON serialization ----

// findingToJSON builds the wire shape for the (v1) Finding type. The real API
// serializes the owning resource under "resource" (not "resourceArn" --
// that's only correct for AnalyzedResource) and requires
// "resourceOwnerAccount"/"analyzedAt", neither of which InMemoryBackend
// tracks per-finding; resourceOwnerAccount defaults to the backend's own
// account (emulated resources belong to the same test account) and
// analyzedAt mirrors updatedAt, matching the GetFindingV2/ListFindingsV2
// convention already used for the same data. "condition" is a required
// member of Finding/FindingSummary (unlike "action"/"principal"/"isPublic",
// which are optional), so it is always emitted -- as {} when the finding
// has none -- rather than omitted.
func findingToJSON(f *Finding, accountID string) map[string]any {
	m := map[string]any{
		"id":                 f.ID,
		keyAnalyzerArn:       f.AnalyzerArn,
		keyStatus:            string(f.Status),
		keyResourceType:      f.ResourceType,
		keyResource:          f.ResourceArn,
		keyResourceOwnerAcct: accountID,
		keyAnalyzedAt:        f.UpdatedAt.Format(time.RFC3339),
		keyUpdatedAt:         f.UpdatedAt.Format(time.RFC3339),
		keyCreatedAt:         f.CreatedAt.Format(time.RFC3339),
		"condition":          conditionOrEmpty(f.Condition),
	}

	if len(f.Action) > 0 {
		m["action"] = f.Action
	}

	if len(f.Principal) > 0 {
		m["principal"] = f.Principal
	}

	if f.IsPublic != nil {
		m["isPublic"] = *f.IsPublic
	}

	return m
}

// conditionOrEmpty returns c, or an empty (non-nil) map if c is nil, so
// callers that must always emit the "condition" wire key (it is a required
// Finding/ExternalAccessDetails member) get {} instead of a bare "null".
func conditionOrEmpty(c map[string]string) map[string]string {
	if c == nil {
		return map[string]string{}
	}

	return c
}

// externalAccessDetailsJSON builds the wire shape of one
// types.ExternalAccessDetails value from a Finding's external-access fields.
// "condition" is a required member; "action"/"principal"/"isPublic" are
// optional and only included when set.
func externalAccessDetailsJSON(f *Finding) map[string]any {
	d := map[string]any{"condition": conditionOrEmpty(f.Condition)}

	if len(f.Action) > 0 {
		d["action"] = f.Action
	}

	if len(f.Principal) > 0 {
		d["principal"] = f.Principal
	}

	if f.IsPublic != nil {
		d["isPublic"] = *f.IsPublic
	}

	return d
}

// findingDetailsV2JSON builds GetFindingV2Output's required "findingDetails"
// array. InMemoryBackend only ever produces external-access-shaped findings
// (see findingTypeExternalAccess), so the array always holds exactly one
// FindingDetails union value keyed "externalAccessDetails" -- never a
// fabricated placeholder for the other four union members
// (internalAccessDetails/unusedIamRoleDetails/unusedIamUserAccessKeyDetails/
// unusedIamUserPasswordDetails), which InMemoryBackend has no state to back.
func findingDetailsV2JSON(f *Finding) []any {
	return []any{map[string]any{"externalAccessDetails": externalAccessDetailsJSON(f)}}
}
