package iot

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// cloneTopicRule creates a deep copy of a TopicRule.
func cloneTopicRule(r *TopicRule) *TopicRule {
	actions := make([]RuleAction, len(r.Actions))
	for i, action := range r.Actions {
		actions[i] = RuleAction{}
		if action.SQS != nil {
			actions[i].SQS = &SQSAction{
				QueueURL: action.SQS.QueueURL,
				RoleARN:  action.SQS.RoleARN,
			}
		}
		if action.Lambda != nil {
			actions[i].Lambda = &LambdaAction{
				FunctionARN: action.Lambda.FunctionARN,
			}
		}
	}

	return &TopicRule{
		RuleName:         r.RuleName,
		ARN:              r.ARN,
		SQL:              r.SQL,
		AWSIoTSQLVersion: r.AWSIoTSQLVersion,
		Description:      r.Description,
		Enabled:          r.Enabled,
		CreatedAt:        r.CreatedAt,
		Actions:          actions,
	}
}

// CreateTopicRule creates a new IoT Topic Rule.
func (b *InMemoryBackend) CreateTopicRule(input *CreateTopicRuleInput) error {
	if input.RuleName == "" {
		return fmt.Errorf("%w: RuleName is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.rules.Has(input.RuleName) {
		return fmt.Errorf("%w: rule %q already exists", ErrAlreadyExists, input.RuleName)
	}

	payload := input.TopicRulePayload
	if payload == nil {
		payload = &TopicRulePayload{}
	}

	actions := payload.Actions
	if actions == nil {
		actions = []RuleAction{}
	}

	arn := arn.Build("iot", b.region, b.accountID, fmt.Sprintf("rule/%s", input.RuleName))

	sqlVersion := payload.AWSIoTSQLVersion
	if sqlVersion == "" {
		sqlVersion = "2015-10-08"
	}

	b.rules.Put(&TopicRule{
		RuleName:         input.RuleName,
		ARN:              arn,
		SQL:              payload.SQL,
		AWSIoTSQLVersion: sqlVersion,
		Description:      payload.Description,
		Actions:          actions,
		Enabled:          !payload.RuleDisabled,
		CreatedAt:        time.Now(),
	})
	b.putResourceTagsLocked(arn, input.Tags)

	return nil
}

// GetTopicRule returns a deep copy of an existing Topic Rule.
func (b *InMemoryBackend) GetTopicRule(ruleName string) (*TopicRule, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	r, ok := b.rules.Get(ruleName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRuleNotFound, ruleName)
	}

	return cloneTopicRule(r), nil
}

// ListTopicRules returns all Topic Rules sorted by name.
func (b *InMemoryBackend) ListTopicRules() []*TopicRule {
	b.mu.RLock()
	defer b.mu.RUnlock()

	items := b.rules.Snapshot()
	out := make([]*TopicRule, 0, len(items))

	for _, v := range items {
		out = append(out, cloneTopicRule(v))
	}

	return out
}

// DeleteTopicRule deletes a Topic Rule by name.
func (b *InMemoryBackend) DeleteTopicRule(ruleName string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	r, ok := b.rules.Get(ruleName)
	if !ok {
		return fmt.Errorf("%w: %s", ErrRuleNotFound, ruleName)
	}

	b.rules.Delete(ruleName)
	delete(b.resourceTags, r.ARN)

	return nil
}

// DisableTopicRule disables an existing topic rule.
func (b *InMemoryBackend) DisableTopicRule(ruleName string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	r, ok := b.rules.Get(ruleName)
	if !ok {
		return fmt.Errorf("%w: %s", ErrRuleNotFound, ruleName)
	}

	r.Enabled = false

	return nil
}

// EnableTopicRule enables an existing topic rule.
func (b *InMemoryBackend) EnableTopicRule(ruleName string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	r, ok := b.rules.Get(ruleName)
	if !ok {
		return fmt.Errorf("%w: %s", ErrRuleNotFound, ruleName)
	}

	r.Enabled = true

	return nil
}

// ReplaceTopicRule replaces the payload of an existing topic rule.
func (b *InMemoryBackend) ReplaceTopicRule(input *ReplaceTopicRuleInput) error {
	if input.RuleName == "" {
		return fmt.Errorf("%w: RuleName is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	r, ok := b.rules.Get(input.RuleName)
	if !ok {
		return fmt.Errorf("%w: %s", ErrRuleNotFound, input.RuleName)
	}

	payload := input.TopicRulePayload
	if payload == nil {
		payload = &TopicRulePayload{}
	}

	actions := payload.Actions
	if actions == nil {
		actions = []RuleAction{}
	}

	sqlVersion := payload.AWSIoTSQLVersion
	if sqlVersion == "" {
		sqlVersion = "2015-10-08"
	}

	r.SQL = payload.SQL
	r.Description = payload.Description
	r.Actions = actions
	r.AWSIoTSQLVersion = sqlVersion
	r.Enabled = !payload.RuleDisabled

	return nil
}

// AddRuleInternal seeds a TopicRule directly into the backend for testing.
func (b *InMemoryBackend) AddRuleInternal(r TopicRule) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if r.ARN == "" {
		r.ARN = arn.Build("iot", b.region, b.accountID, fmt.Sprintf("rule/%s", r.RuleName))
	}

	if r.Actions == nil {
		r.Actions = []RuleAction{}
	}

	b.rules.Put(&r)
}

// CreateTopicRuleDestination creates a new topic rule destination.
func (b *InMemoryBackend) CreateTopicRuleDestination(
	input *CreateTopicRuleDestinationInput,
) (*TopicRuleDestination, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	arn := arn.Build("iot", b.region, b.accountID,
		fmt.Sprintf("ruledestination/http/%s", uuid.NewString()))

	dest := &TopicRuleDestination{
		ARN: arn,
	}

	if input.DestinationConfiguration != nil && input.DestinationConfiguration.HTTPURLConfiguration != nil {
		dest.HTTPURLProperties = &HTTPURLDestinationProperties{
			ConfirmationURL: input.DestinationConfiguration.HTTPURLConfiguration.ConfirmationURL,
		}
		// HTTP destinations require confirmation before they can be used,
		// matching AWS's real IN_PROGRESS -> ENABLED lifecycle.
		dest.Status = statusInProgress
		dest.ConfirmationToken = randomHex(certIDHexLen)
	} else {
		dest.Status = statusEnabled
	}

	b.topicRuleDestinations.Put(dest)

	return dest, nil
}

// GetTopicRuleDestination returns a topic rule destination by ARN.
func (b *InMemoryBackend) GetTopicRuleDestination(arn string) (*TopicRuleDestination, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	dest, ok := b.topicRuleDestinations.Get(arn)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTopicRuleDestinationNotFound, arn)
	}

	cp := *dest

	return &cp, nil
}

// ListTopicRuleDestinations returns all topic rule destinations.
func (b *InMemoryBackend) ListTopicRuleDestinations() []*TopicRuleDestination {
	b.mu.RLock()
	defer b.mu.RUnlock()

	items := b.topicRuleDestinations.Snapshot()
	out := make([]*TopicRuleDestination, 0, len(items))

	for _, v := range items {
		cp := *v
		out = append(out, &cp)
	}

	return out
}

// UpdateTopicRuleDestination updates the status of a topic rule destination.
func (b *InMemoryBackend) UpdateTopicRuleDestination(input *UpdateTopicRuleDestinationInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	dest, ok := b.topicRuleDestinations.Get(input.ARN)
	if !ok {
		return fmt.Errorf("%w: %s", ErrTopicRuleDestinationNotFound, input.ARN)
	}

	dest.Status = input.Status

	return nil
}

// DeleteTopicRuleDestination deletes a topic rule destination by ARN.
func (b *InMemoryBackend) DeleteTopicRuleDestination(arn string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.topicRuleDestinations.Has(arn) {
		return fmt.Errorf("%w: %s", ErrTopicRuleDestinationNotFound, arn)
	}

	b.topicRuleDestinations.Delete(arn)

	return nil
}

// ConfirmTopicRuleDestination transitions a topic rule destination created
// with an HTTP URL configuration from IN_PROGRESS to ENABLED, given the
// confirmation token that was generated at creation time.
func (b *InMemoryBackend) ConfirmTopicRuleDestination(token string) error {
	if token == "" {
		return fmt.Errorf("%w: confirmationToken is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for _, dest := range b.topicRuleDestinations.All() {
		if dest.ConfirmationToken != "" && dest.ConfirmationToken == token {
			dest.Status = statusEnabled
			dest.ConfirmationToken = ""

			return nil
		}
	}

	return fmt.Errorf("%w: invalid or expired confirmation token", ErrValidation)
}
