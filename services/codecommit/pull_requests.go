package codecommit

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// CreatePullRequest creates a new pull request.
func (b *InMemoryBackend) CreatePullRequest(
	title, description, clientRequestToken string,
	targets []PullRequestTarget,
) (*PullRequest, error) {
	b.mu.Lock("CreatePullRequest")
	defer b.mu.Unlock()

	b.nextPRCounter++
	prID := strconv.Itoa(b.nextPRCounter)
	now := time.Now().UTC()

	pr := &PullRequest{
		PullRequestID:      prID,
		Title:              title,
		Description:        description,
		PullRequestStatus:  prStatusOpen,
		CreationDate:       now,
		LastActivityDate:   now,
		ClientRequestToken: clientRequestToken,
		PullRequestTargets: targets,
		RevisionID:         uuid.NewString(),
	}
	b.pullRequests.Put(pr)
	cp := *pr

	// deep copy targets slice
	cp.PullRequestTargets = make([]PullRequestTarget, len(targets))
	copy(cp.PullRequestTargets, targets)

	return &cp, nil
}

// GetPullRequest returns a pull request by ID.
func (b *InMemoryBackend) GetPullRequest(prID string) (*PullRequest, error) {
	b.mu.RLock("GetPullRequest")
	defer b.mu.RUnlock()

	pr, ok := b.pullRequests.Get(prID)
	if !ok {
		return nil, fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}

	cp := *pr

	return &cp, nil
}

// ListPullRequests returns pull request IDs for a repository, optionally filtered by status.
// IDs are returned in numeric descending order (newest first), matching AWS behaviour.
// pullRequestStatus accepts "OPEN" or "CLOSED" (empty means return all); a merged
// pull request's status is "CLOSED", matching types.PullRequestStatusEnum, which has
// no MERGED member.
func (b *InMemoryBackend) ListPullRequests(repositoryName, pullRequestStatus, authorARN string) ([]string, error) {
	b.mu.RLock("ListPullRequests")
	defer b.mu.RUnlock()

	if !b.repositories.Has(repositoryName) {
		return nil, fmt.Errorf("%w: repository %s not found", ErrNotFound, repositoryName)
	}

	all := b.pullRequests.All()
	ids := make([]string, 0, len(all))

	for _, pr := range all {
		if pullRequestStatus != "" && pr.PullRequestStatus != pullRequestStatus {
			continue
		}
		if authorARN != "" && pr.AuthorARN != authorARN {
			continue
		}

		for _, t := range pr.PullRequestTargets {
			if t.RepositoryName == repositoryName {
				ids = append(ids, pr.PullRequestID)

				break
			}
		}
	}

	// Sort numerically descending (newest first) — AWS returns highest IDs first.
	sort.Slice(ids, func(i, j int) bool {
		ni, ei := strconv.Atoi(ids[i])
		nj, ej := strconv.Atoi(ids[j])
		if ei == nil && ej == nil {
			return ni > nj
		}

		return ids[i] > ids[j]
	})

	return ids, nil
}

// GetPullRequestApprovalStates returns the approval states for a pull request.
func (b *InMemoryBackend) GetPullRequestApprovalStates(prID string) ([]PullRequestApproval, error) {
	b.mu.RLock("GetPullRequestApprovalStates")
	defer b.mu.RUnlock()

	if !b.pullRequests.Has(prID) {
		return nil, fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}

	approvals := b.prApprovals[prID]
	result := make([]PullRequestApproval, 0, len(approvals))
	for userARN, state := range approvals {
		result = append(result, PullRequestApproval{UserARN: userARN, ApprovalState: state})
	}

	return result, nil
}

// GetPullRequestOverrideState returns whether a PR has been overridden and by whom.
func (b *InMemoryBackend) GetPullRequestOverrideState(prID string) (bool, string, error) {
	b.mu.RLock("GetPullRequestOverrideState")
	defer b.mu.RUnlock()

	if !b.pullRequests.Has(prID) {
		return false, "", fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}

	overridden := b.prOverrides[prID]
	overrider := b.prOverriders[prID]

	return overridden, overrider, nil
}

// OverridePullRequestApprovalRules sets the override status for a pull request.
func (b *InMemoryBackend) OverridePullRequestApprovalRules(prID, overrideStatus, overriderARN string) error {
	b.mu.Lock("OverridePullRequestApprovalRules")
	defer b.mu.Unlock()

	if !b.pullRequests.Has(prID) {
		return fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}

	b.prOverrides[prID] = overrideStatus == "OVERRIDE"
	b.prOverriders[prID] = overriderARN
	b.prEvents[prID] = append(b.prEvents[prID], PullRequestEvent{
		PullRequestEventType: "PULL_REQUEST_APPROVAL_RULE_OVERRIDDEN",
		EventDate:            time.Now().UTC(),
		ActorARN:             overriderARN,
	})

	return nil
}

// UpdatePullRequestApprovalState sets the approval state for a user on a pull request.
// AWS rejects this operation on closed or merged pull requests.
func (b *InMemoryBackend) UpdatePullRequestApprovalState(prID, userARN, approvalState string) error {
	b.mu.Lock("UpdatePullRequestApprovalState")
	defer b.mu.Unlock()

	pr, ok := b.pullRequests.Get(prID)
	if !ok {
		return fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}
	if pr.PullRequestStatus == prStatusClosed {
		return fmt.Errorf("%w: pull request %s is already closed", ErrPullRequestAlreadyMerged, prID)
	}

	if b.prApprovals[prID] == nil {
		b.prApprovals[prID] = make(map[string]string)
	}
	b.prApprovals[prID][userARN] = approvalState

	return nil
}

// UpdatePullRequestDescription updates the description of a pull request.
// AWS rejects this operation on closed or merged pull requests.
func (b *InMemoryBackend) UpdatePullRequestDescription(prID, desc string) error {
	b.mu.Lock("UpdatePullRequestDescription")
	defer b.mu.Unlock()

	pr, ok := b.pullRequests.Get(prID)
	if !ok {
		return fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}
	if pr.PullRequestStatus == prStatusClosed {
		return fmt.Errorf("%w: pull request %s is already closed", ErrPullRequestAlreadyMerged, prID)
	}
	pr.Description = desc
	pr.LastActivityDate = time.Now().UTC()

	return nil
}

// UpdatePullRequestStatus updates the status of a pull request.
func (b *InMemoryBackend) UpdatePullRequestStatus(prID, status string) error {
	b.mu.Lock("UpdatePullRequestStatus")
	defer b.mu.Unlock()

	pr, ok := b.pullRequests.Get(prID)
	if !ok {
		return fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}
	pr.PullRequestStatus = status
	pr.LastActivityDate = time.Now().UTC()

	return nil
}

// UpdatePullRequestTitle updates the title of a pull request.
// AWS rejects this operation on closed or merged pull requests.
func (b *InMemoryBackend) UpdatePullRequestTitle(prID, title string) error {
	b.mu.Lock("UpdatePullRequestTitle")
	defer b.mu.Unlock()

	pr, ok := b.pullRequests.Get(prID)
	if !ok {
		return fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}
	if pr.PullRequestStatus == prStatusClosed {
		return fmt.Errorf("%w: pull request %s is already closed", ErrPullRequestAlreadyMerged, prID)
	}
	pr.Title = title
	pr.LastActivityDate = time.Now().UTC()

	return nil
}

// CreatePullRequestApprovalRule creates an approval rule on a pull request.
func (b *InMemoryBackend) CreatePullRequestApprovalRule(
	prID, ruleName, content string,
) (*PullRequestApprovalRule, error) {
	b.mu.Lock("CreatePullRequestApprovalRule")
	defer b.mu.Unlock()

	if !b.pullRequests.Has(prID) {
		return nil, fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}

	rule := &PullRequestApprovalRule{
		RuleID:              uuid.NewString(),
		RuleName:            ruleName,
		ApprovalRuleContent: content,
		PRID:                prID,
	}
	b.prApprovalRules.Put(rule)
	cp := *rule

	return &cp, nil
}

// DeletePullRequestApprovalRule deletes an approval rule from a pull
// request, returning its ID. The real DeletePullRequestApprovalRuleOutput
// echoes ApprovalRuleId as a required field
// (api_op_DeletePullRequestApprovalRule.go:48).
func (b *InMemoryBackend) DeletePullRequestApprovalRule(prID, ruleName string) (string, error) {
	b.mu.Lock("DeletePullRequestApprovalRule")
	defer b.mu.Unlock()

	if !b.pullRequests.Has(prID) {
		return "", fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}

	rule, ok := b.prApprovalRules.Get(prApprovalRuleKey(prID, ruleName))
	if !ok {
		// DeletePullRequestApprovalRule is idempotent in real AWS: "If the
		// approval rule was deleted in an earlier API call, the response is
		// 200 OK without content" (codecommit@v1.36.4
		// api_op_DeletePullRequestApprovalRule.go:49) -- its own error
		// switch has no ApprovalRuleDoesNotExistException case, unlike
		// UpdatePullRequestApprovalRuleContent.
		return "", nil
	}
	b.prApprovalRules.Delete(prApprovalRuleKey(prID, ruleName))

	return rule.RuleID, nil
}

// UpdatePullRequestApprovalRuleContent updates the content of an approval
// rule on a pull request, returning the updated rule. The real
// UpdatePullRequestApprovalRuleContentOutput echoes the full ApprovalRule as
// a required field (api_op_UpdatePullRequestApprovalRuleContent.go:82).
func (b *InMemoryBackend) UpdatePullRequestApprovalRuleContent(
	prID, ruleName, content string,
) (*PullRequestApprovalRule, error) {
	b.mu.Lock("UpdatePullRequestApprovalRuleContent")
	defer b.mu.Unlock()

	if !b.pullRequests.Has(prID) {
		return nil, fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}

	rule, ok := b.prApprovalRules.Get(prApprovalRuleKey(prID, ruleName))
	if !ok {
		return nil, fmt.Errorf(
			"%w: approval rule %s not found on pull request %s", ErrApprovalRuleNotFound, ruleName, prID,
		)
	}
	rule.ApprovalRuleContent = content
	cp := *rule

	return &cp, nil
}

// DescribePullRequestEvents returns events for a pull request, optionally
// filtered to a single pullRequestEventType (DescribePullRequestEventsInput.
// PullRequestEventType, api_op_DescribePullRequestEvents.go: "Optional. The
// pull request event type about which you want to return information.") and/or
// a single actorARN (DescribePullRequestEventsInput.ActorArn: "The Amazon
// Resource Name (ARN) of the user whose actions resulted in the event.").
func (b *InMemoryBackend) DescribePullRequestEvents(prID, eventType, actorARN string) ([]PullRequestEvent, error) {
	b.mu.RLock("DescribePullRequestEvents")
	defer b.mu.RUnlock()

	if !b.pullRequests.Has(prID) {
		return nil, fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}

	events := b.prEvents[prID]
	result := make([]PullRequestEvent, 0, len(events))
	for _, e := range events {
		if eventType != "" && e.PullRequestEventType != eventType {
			continue
		}
		if actorARN != "" && e.ActorARN != actorARN {
			continue
		}
		result = append(result, e)
	}

	return result, nil
}

// EvaluatePullRequestApprovalRules evaluates all approval rules for a pull request.
func (b *InMemoryBackend) EvaluatePullRequestApprovalRules(prID string) ([]RuleEvaluation, error) {
	b.mu.RLock("EvaluatePullRequestApprovalRules")
	defer b.mu.RUnlock()

	if !b.pullRequests.Has(prID) {
		return nil, fmt.Errorf("%w: pull request %s not found", ErrPullRequestNotFound, prID)
	}

	rules := b.prApprovalRulesByPR.Get(prID)
	result := make([]RuleEvaluation, 0, len(rules))
	for _, rule := range rules {
		result = append(result, RuleEvaluation{RuleName: rule.RuleName, Satisfied: true})
	}

	return result, nil
}
