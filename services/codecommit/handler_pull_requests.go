package codecommit

import (
	"encoding/json"
	"fmt"
)

// isValidPullRequestEventType reports whether v is one of the nine
// PullRequestEventType enum values (codecommit@v1.36.4 types/enums.go).
func isValidPullRequestEventType(v string) bool {
	switch v {
	case "PULL_REQUEST_CREATED",
		"PULL_REQUEST_STATUS_CHANGED",
		"PULL_REQUEST_SOURCE_REFERENCE_UPDATED",
		"PULL_REQUEST_MERGE_STATE_CHANGED",
		"PULL_REQUEST_APPROVAL_RULE_CREATED",
		"PULL_REQUEST_APPROVAL_RULE_UPDATED",
		"PULL_REQUEST_APPROVAL_RULE_DELETED",
		"PULL_REQUEST_APPROVAL_RULE_OVERRIDDEN",
		"PULL_REQUEST_APPROVAL_STATE_CHANGED":
		return true
	}

	return false
}

type pullRequestTargetInput struct {
	RepositoryName       string `json:"repositoryName"`
	SourceReference      string `json:"sourceReference"`
	DestinationReference string `json:"destinationReference"`
}

type createPullRequestInput struct {
	Title              string                   `json:"title"`
	Description        string                   `json:"description"`
	ClientRequestToken string                   `json:"clientRequestToken"`
	Targets            []pullRequestTargetInput `json:"targets"`
}

func pullRequestToMap(pr *PullRequest) map[string]any {
	targets := make([]map[string]any, 0, len(pr.PullRequestTargets))
	for _, t := range pr.PullRequestTargets {
		targets = append(targets, map[string]any{
			keyRepositoryName:      t.RepositoryName,
			"sourceReference":      t.SourceReference,
			"destinationReference": t.DestinationReference,
			"sourceCommit":         t.SourceCommit,
			"destinationCommit":    t.DestinationCommit,
			"mergeBase":            t.MergeBase,
		})
	}

	return map[string]any{
		keyPullRequestID:     pr.PullRequestID,
		"title":              pr.Title,
		"description":        pr.Description,
		"authorArn":          pr.AuthorARN,
		"pullRequestStatus":  pr.PullRequestStatus,
		keyCreationDate:      pr.CreationDate.Unix(),
		"lastActivityDate":   pr.LastActivityDate.Unix(),
		"revisionId":         pr.RevisionID,
		"pullRequestTargets": targets,
		"clientRequestToken": pr.ClientRequestToken,
	}
}

func (h *Handler) handleCreatePullRequest(body []byte) (any, error) {
	var in createPullRequestInput
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if in.Title == "" {
		return nil, fmt.Errorf("%w: title is required", errInvalidRequest)
	}

	if len(in.Targets) == 0 {
		return nil, fmt.Errorf("%w: at least one target is required", errInvalidRequest)
	}

	targets := make([]PullRequestTarget, 0, len(in.Targets))
	for i, t := range in.Targets {
		if t.RepositoryName == "" {
			return nil, fmt.Errorf("%w: targets[%d].repositoryName is required", errInvalidRequest, i)
		}

		if t.SourceReference == "" {
			return nil, fmt.Errorf("%w: targets[%d].sourceReference is required", errInvalidRequest, i)
		}

		targets = append(targets, PullRequestTarget{
			RepositoryName:       t.RepositoryName,
			SourceReference:      t.SourceReference,
			DestinationReference: t.DestinationReference,
		})
	}

	pr, err := h.Backend.CreatePullRequest(in.Title, in.Description, in.ClientRequestToken, targets)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyPullRequest: pullRequestToMap(pr),
	}, nil
}

func (h *Handler) handleGetPullRequest(body []byte) (any, error) {
	var in struct {
		PullRequestID string `json:"pullRequestId"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if in.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}

	pr, err := h.Backend.GetPullRequest(in.PullRequestID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyPullRequest: pullRequestToMap(pr),
	}, nil
}

func (h *Handler) handleListPullRequests(body []byte) (any, error) {
	var in struct {
		RepositoryName    string `json:"repositoryName"`
		PullRequestStatus string `json:"pullRequestStatus"`
		AuthorARN         string `json:"authorArn"`
		NextToken         string `json:"nextToken"`
		MaxResults        int    `json:"maxResults"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if in.RepositoryName == "" {
		return nil, fmt.Errorf("%w: repositoryName is required", errInvalidRequest)
	}

	if in.PullRequestStatus != "" &&
		in.PullRequestStatus != prStatusOpen &&
		in.PullRequestStatus != prStatusClosed {
		return nil, fmt.Errorf("%w: pullRequestStatus must be OPEN or CLOSED", ErrInvalidPullRequestStatus)
	}

	ids, err := h.Backend.ListPullRequests(in.RepositoryName, in.PullRequestStatus, in.AuthorARN)
	if err != nil {
		return nil, err
	}

	// Apply pagination.
	ids, nextToken := paginateStrings(ids, in.NextToken, in.MaxResults)

	resp := map[string]any{
		"pullRequestIds": ids,
	}
	if nextToken != "" {
		resp["nextToken"] = nextToken
	}

	return resp, nil
}

func (h *Handler) handleGetPullRequestApprovalStates(body []byte) (any, error) {
	var req struct {
		PullRequestID string `json:"pullRequestId"`
		RevisionID    string `json:"revisionId"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}

	if req.RevisionID == "" {
		return nil, fmt.Errorf("%w: revisionId is required", errInvalidRequest)
	}

	approvals, err := h.Backend.GetPullRequestApprovalStates(req.PullRequestID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"approvals": approvals,
	}, nil
}

func (h *Handler) handleGetPullRequestOverrideState(body []byte) (any, error) {
	var req struct {
		PullRequestID string `json:"pullRequestId"`
		RevisionID    string `json:"revisionId"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}

	if req.RevisionID == "" {
		return nil, fmt.Errorf("%w: revisionId is required", errInvalidRequest)
	}

	overridden, overrider, err := h.Backend.GetPullRequestOverrideState(req.PullRequestID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"overridden": overridden,
		"overrider":  overrider,
	}, nil
}

func (h *Handler) handleOverridePullRequestApprovalRules(body []byte) (any, error) {
	var req struct {
		PullRequestID  string `json:"pullRequestId"`
		RevisionID     string `json:"revisionId"`
		OverrideStatus string `json:"overrideStatus"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}

	if req.RevisionID == "" {
		return nil, fmt.Errorf("%w: revisionId is required", errInvalidRequest)
	}

	return map[string]any{}, h.Backend.OverridePullRequestApprovalRules(req.PullRequestID, req.OverrideStatus, "")
}

func (h *Handler) handleUpdatePullRequestApprovalState(body []byte) (any, error) {
	var req struct {
		PullRequestID string `json:"pullRequestId"`
		RevisionID    string `json:"revisionId"`
		ApprovalState string `json:"approvalState"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}

	if req.RevisionID == "" {
		return nil, fmt.Errorf("%w: revisionId is required", errInvalidRequest)
	}

	return map[string]any{}, h.Backend.UpdatePullRequestApprovalState(req.PullRequestID, "", req.ApprovalState)
}

func (h *Handler) handleUpdatePullRequestDescription(body []byte) (any, error) {
	var req struct {
		PullRequestID string `json:"pullRequestId"`
		Description   string `json:"description"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}

	if err := h.Backend.UpdatePullRequestDescription(req.PullRequestID, req.Description); err != nil {
		return nil, err
	}

	pr, err := h.Backend.GetPullRequest(req.PullRequestID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyPullRequest: pullRequestToMap(pr),
	}, nil
}

func (h *Handler) handleUpdatePullRequestStatus(body []byte) (any, error) {
	var req struct {
		PullRequestID     string `json:"pullRequestId"`
		PullRequestStatus string `json:"pullRequestStatus"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}
	if req.PullRequestStatus != prStatusOpen && req.PullRequestStatus != prStatusClosed {
		return nil, fmt.Errorf("%w: pullRequestStatus must be OPEN or CLOSED", ErrInvalidPullRequestStatus)
	}

	if err := h.Backend.UpdatePullRequestStatus(req.PullRequestID, req.PullRequestStatus); err != nil {
		return nil, err
	}

	pr, err := h.Backend.GetPullRequest(req.PullRequestID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyPullRequest: pullRequestToMap(pr),
	}, nil
}

func (h *Handler) handleUpdatePullRequestTitle(body []byte) (any, error) {
	var req struct {
		PullRequestID string `json:"pullRequestId"`
		Title         string `json:"title"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}

	if err := h.Backend.UpdatePullRequestTitle(req.PullRequestID, req.Title); err != nil {
		return nil, err
	}

	pr, err := h.Backend.GetPullRequest(req.PullRequestID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		keyPullRequest: pullRequestToMap(pr),
	}, nil
}

func (h *Handler) handleCreatePullRequestApprovalRule(body []byte) (any, error) {
	var req struct {
		PullRequestID       string `json:"pullRequestId"`
		ApprovalRuleName    string `json:"approvalRuleName"`
		ApprovalRuleContent string `json:"approvalRuleContent"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" || req.ApprovalRuleName == "" {
		return nil, fmt.Errorf("%w: pullRequestId and approvalRuleName are required", errInvalidRequest)
	}

	rule, err := h.Backend.CreatePullRequestApprovalRule(
		req.PullRequestID,
		req.ApprovalRuleName,
		req.ApprovalRuleContent,
	)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"approvalRule": map[string]any{
			keyApprovalRuleID:     rule.RuleID,
			"approvalRuleName":    rule.RuleName,
			"approvalRuleContent": rule.ApprovalRuleContent,
		},
	}, nil
}

func (h *Handler) handleDeletePullRequestApprovalRule(body []byte) (any, error) {
	var req struct {
		PullRequestID    string `json:"pullRequestId"`
		ApprovalRuleName string `json:"approvalRuleName"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" || req.ApprovalRuleName == "" {
		return nil, fmt.Errorf("%w: pullRequestId and approvalRuleName are required", errInvalidRequest)
	}

	ruleID, err := h.Backend.DeletePullRequestApprovalRule(req.PullRequestID, req.ApprovalRuleName)
	if err != nil {
		return nil, err
	}
	if ruleID == "" {
		return map[string]any{}, nil
	}

	return map[string]any{
		keyApprovalRuleID: ruleID,
	}, nil
}

func (h *Handler) handleUpdatePullRequestApprovalRuleContent(body []byte) (any, error) {
	var req struct {
		PullRequestID    string `json:"pullRequestId"`
		ApprovalRuleName string `json:"approvalRuleName"`
		NewRuleContent   string `json:"newRuleContent"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" || req.ApprovalRuleName == "" {
		return nil, fmt.Errorf("%w: pullRequestId and approvalRuleName are required", errInvalidRequest)
	}

	rule, err := h.Backend.UpdatePullRequestApprovalRuleContent(
		req.PullRequestID, req.ApprovalRuleName, req.NewRuleContent,
	)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"approvalRule": map[string]any{
			keyApprovalRuleID:     rule.RuleID,
			"approvalRuleName":    rule.RuleName,
			"approvalRuleContent": rule.ApprovalRuleContent,
		},
	}, nil
}

func (h *Handler) handleDescribePullRequestEvents(body []byte) (any, error) {
	var req struct {
		PullRequestID        string `json:"pullRequestId"`
		PullRequestEventType string `json:"pullRequestEventType"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}
	if req.PullRequestEventType != "" && !isValidPullRequestEventType(req.PullRequestEventType) {
		return nil, fmt.Errorf("%w: pullRequestEventType %q is not a valid value",
			ErrInvalidPullRequestEventType, req.PullRequestEventType)
	}

	events, err := h.Backend.DescribePullRequestEvents(req.PullRequestID, req.PullRequestEventType)
	if err != nil {
		return nil, err
	}

	wireEvents := make([]map[string]any, len(events))
	for i, e := range events {
		wireEvents[i] = map[string]any{
			"pullRequestEventType": e.PullRequestEventType,
			"eventDate":            e.EventDate.Unix(),
		}
	}

	return map[string]any{
		"pullRequestEvents": wireEvents,
	}, nil
}

func (h *Handler) handleEvaluatePullRequestApprovalRules(body []byte) (any, error) {
	var req struct {
		PullRequestID string `json:"pullRequestId"`
		RevisionID    string `json:"revisionId"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	if req.PullRequestID == "" {
		return nil, fmt.Errorf("%w: pullRequestId is required", errInvalidRequest)
	}

	if req.RevisionID == "" {
		return nil, fmt.Errorf("%w: revisionId is required", errInvalidRequest)
	}

	evals, err := h.Backend.EvaluatePullRequestApprovalRules(req.PullRequestID)
	if err != nil {
		return nil, err
	}

	overridden, _, err := h.Backend.GetPullRequestOverrideState(req.PullRequestID)
	if err != nil {
		return nil, err
	}

	satisfied := make([]string, 0, len(evals))
	notSatisfied := make([]string, 0, len(evals))

	for _, e := range evals {
		if e.Satisfied {
			satisfied = append(satisfied, e.RuleName)
		} else {
			notSatisfied = append(notSatisfied, e.RuleName)
		}
	}

	return map[string]any{
		"evaluation": map[string]any{
			"approved":                  overridden || len(notSatisfied) == 0,
			"overridden":                overridden,
			"approvalRulesSatisfied":    satisfied,
			"approvalRulesNotSatisfied": notSatisfied,
		},
	}, nil
}
