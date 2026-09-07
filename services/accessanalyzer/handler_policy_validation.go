package accessanalyzer

import (
	"encoding/json"
	"net/http"
)

const (
	opCheckAccessNotGranted = "CheckAccessNotGranted"
	opCheckNoNewAccess      = "CheckNoNewAccess"
	opCheckNoPublicAccess   = "CheckNoPublicAccess"
	opValidatePolicy        = "ValidatePolicy"

	pathPolicy                = "policy"
	pathValidation            = "validation"
	pathCheckAccessNotGranted = "check-access-not-granted"
	pathCheckNoNewAccess      = "check-no-new-access"
	pathCheckNoPublicAccess   = "check-no-public-access"

	keyMessage = "message"
	keyResult  = "result"
	keyReasons = "reasons"
)

// dispatchPolicyValidationOps routes policy-validation operations
// (Check*/ValidatePolicy). These take their input entirely from the request
// body, so path and query are unused here.
func (h *Handler) dispatchPolicyValidationOps(op, _, _ string, body []byte) (any, int, bool, error) {
	switch op {
	case opCheckAccessNotGranted:
		r, c, err := h.handleCheckAccessNotGranted(body)

		return r, c, true, err
	case opCheckNoNewAccess:
		r, c, err := h.handleCheckNoNewAccess(body)

		return r, c, true, err
	case opCheckNoPublicAccess:
		r, c, err := h.handleCheckNoPublicAccess(body)

		return r, c, true, err
	case opValidatePolicy:
		r, c, err := h.handleValidatePolicy(body)

		return r, c, true, err
	}

	return nil, 0, false, nil
}

// ---- operation handlers ----

func (h *Handler) handleCheckAccessNotGranted(body []byte) (any, int, error) {
	var req struct {
		PolicyDocument string       `json:"policyDocument"`
		PolicyType     string       `json:"policyType"`
		Access         []AccessSpec `json:"access"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, 0, ErrValidation
	}

	if req.PolicyType == "" {
		return nil, 0, ErrValidation
	}

	res, err := CheckAccessNotGranted(req.PolicyDocument, req.Access)
	if err != nil {
		return nil, 0, err
	}

	out := map[string]any{keyResult: res.Result, keyMessage: res.Message}

	if len(res.Reasons) > 0 {
		out[keyReasons] = res.Reasons
	}

	return out, http.StatusOK, nil
}

func (h *Handler) handleCheckNoNewAccess(body []byte) (any, int, error) {
	var req struct {
		ExistingPolicyDocument string `json:"existingPolicyDocument"`
		NewPolicyDocument      string `json:"newPolicyDocument"`
		PolicyType             string `json:"policyType"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, 0, ErrValidation
	}

	if req.PolicyType == "" {
		return nil, 0, ErrValidation
	}

	res, err := CheckNoNewAccess(req.ExistingPolicyDocument, req.NewPolicyDocument)
	if err != nil {
		return nil, 0, err
	}

	out := map[string]any{keyResult: res.Result, keyMessage: res.Message}

	if len(res.Reasons) > 0 {
		out[keyReasons] = res.Reasons
	}

	return out, http.StatusOK, nil
}

func (h *Handler) handleCheckNoPublicAccess(body []byte) (any, int, error) {
	var req struct {
		PolicyDocument string `json:"policyDocument"`
		ResourceType   string `json:"resourceType"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, 0, ErrValidation
	}

	if req.ResourceType == "" {
		return nil, 0, ErrValidation
	}

	res, err := CheckNoPublicAccess(req.PolicyDocument)
	if err != nil {
		return nil, 0, err
	}

	reasons := make([]any, 0, len(res.Reasons))
	for _, r := range res.Reasons {
		reasons = append(reasons, r)
	}

	return map[string]any{keyResult: res.Result, keyMessage: res.Message, keyReasons: reasons}, http.StatusOK, nil
}

func (h *Handler) handleValidatePolicy(body []byte) (any, int, error) {
	var req struct {
		PolicyDocument string `json:"policyDocument"`
		PolicyType     string `json:"policyType"`
		NextToken      string `json:"nextToken"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return nil, 0, ErrValidation
	}

	policyType := req.PolicyType
	if policyType == "" {
		policyType = "IDENTITY_POLICY"
	}

	raw := ValidatePolicy(req.PolicyDocument, policyType)
	findings := make([]any, 0, len(raw))

	for _, f := range raw {
		findings = append(findings, map[string]any{
			"findingType":    f.FindingType,
			"issueCode":      f.IssueCode,
			"findingDetails": f.FindingDetails,
			"learnMoreLink":  f.LearnMoreLink,
			"locations":      f.Locations,
		})
	}

	return map[string]any{keyFindings: findings}, http.StatusOK, nil
}

// ---- URL path parsing ----

// parsePolicyPath parses paths under /policy/...: the four Check*/Validate
// policy-validation endpoints handled directly here, plus /policy/generation
// (policy generation jobs), which delegates to parsePolicyGenerationPath in
// handler_generated_policies.go.
func parsePolicyPath(method string, segments []string) (string, string, bool) {
	if len(segments) < segmentDepthResource {
		return "", "", false
	}

	switch segments[1] {
	case pathCheckAccessNotGranted:
		if method == http.MethodPost {
			return opCheckAccessNotGranted, "", true
		}
	case pathCheckNoNewAccess:
		if method == http.MethodPost {
			return opCheckNoNewAccess, "", true
		}
	case pathCheckNoPublicAccess:
		if method == http.MethodPost {
			return opCheckNoPublicAccess, "", true
		}
	case pathValidation:
		if method == http.MethodPost {
			return opValidatePolicy, "", true
		}
	case pathGeneration:
		return parsePolicyGenerationPath(method, segments)
	}

	return "", "", false
}
