package xray

import (
	"fmt"
	"maps"
	"sort"
	"time"
)

// defaultSamplingRule returns the built-in X-Ray sampling rule that is always
// present. The "Default" rule matches all requests and has the lowest
// priority (10000).
func (b *InMemoryBackend) defaultSamplingRule() *SamplingRule {
	now := time.Now()

	return &SamplingRule{
		RuleName:      defaultSamplingRuleName,
		RuleARN:       b.samplingRuleARN(defaultSamplingRuleName),
		ResourceARN:   "*",
		ServiceName:   "*",
		ServiceType:   "*",
		Host:          "*",
		HTTPMethod:    "*",
		URLPath:       "*",
		FixedRate:     defaultFixedRate,
		Priority:      defaultSamplingPriority,
		ReservoirSize: 1,
		CreatedAt:     now,
		ModifiedAt:    now,
	}
}

func cloneRule(r *SamplingRule) *SamplingRule {
	cp := *r

	if len(r.Attributes) > 0 {
		cp.Attributes = make(map[string]string, len(r.Attributes))
		maps.Copy(cp.Attributes, r.Attributes)
	}

	return &cp
}

// ValidateSamplingRule validates sampling rule fields per AWS constraints.
func ValidateSamplingRule(rule SamplingRule) error {
	if rule.RuleName == "" || len(rule.RuleName) > 32 {
		return fmt.Errorf("%w: RuleName must be 1-32 characters", ErrInvalidSamplingRule)
	}

	if len(rule.ServiceName) > maxServiceNameLen {
		return fmt.Errorf("%w: ServiceName must be at most %d characters", ErrInvalidSamplingRule, maxServiceNameLen)
	}

	if rule.Priority < 1 || rule.Priority > 9999 {
		return fmt.Errorf("%w: Priority must be between 1 and 9999", ErrInvalidSamplingRule)
	}

	if rule.FixedRate < 0 || rule.FixedRate > 1.0 {
		return fmt.Errorf("%w: FixedRate must be between 0.0 and 1.0", ErrInvalidSamplingRule)
	}

	if rule.ReservoirSize < 0 {
		return fmt.Errorf("%w: ReservoirSize must be >= 0", ErrInvalidSamplingRule)
	}

	return nil
}

// ValidateSamplingRuleUpdate validates the pointer-optional fields of a
// SamplingRuleUpdate against the same constraints ValidateSamplingRule enforces on
// create: Priority, FixedRate, ReservoirSize, and ServiceName length are properties
// of the sampling rule resource, not create-time-only, so UpdateSamplingRule must
// not be usable to push a rule into a state CreateSamplingRule would have rejected.
// Only fields the caller actually set (non-nil) are checked.
func ValidateSamplingRuleUpdate(u SamplingRuleUpdate) error {
	if u.ServiceName != nil && len(*u.ServiceName) > maxServiceNameLen {
		return fmt.Errorf("%w: ServiceName must be at most %d characters", ErrInvalidSamplingRule, maxServiceNameLen)
	}

	if u.Priority != nil && (*u.Priority < 1 || *u.Priority > 9999) {
		return fmt.Errorf("%w: Priority must be between 1 and 9999", ErrInvalidSamplingRule)
	}

	if u.FixedRate != nil && (*u.FixedRate < 0 || *u.FixedRate > 1.0) {
		return fmt.Errorf("%w: FixedRate must be between 0.0 and 1.0", ErrInvalidSamplingRule)
	}

	if u.ReservoirSize != nil && *u.ReservoirSize < 0 {
		return fmt.Errorf("%w: ReservoirSize must be >= 0", ErrInvalidSamplingRule)
	}

	return nil
}

// CreateSamplingRule creates a new sampling rule.
// Returns ErrRuleLimitExceeded if the account already has maxSamplingRules rules.
func (b *InMemoryBackend) CreateSamplingRule(rule SamplingRule) (*SamplingRule, error) {
	b.mu.Lock("CreateSamplingRule")
	defer b.mu.Unlock()

	if b.samplingRules.Has(rule.RuleName) {
		return nil, fmt.Errorf("%w: sampling rule %s already exists", ErrSamplingRuleAlreadyExists, rule.RuleName)
	}

	if b.samplingRules.Len() >= maxSamplingRules {
		return nil, fmt.Errorf("%w: maximum of %d sampling rules per account", ErrRuleLimitExceeded, maxSamplingRules)
	}

	rule.RuleARN = b.samplingRuleARN(rule.RuleName)
	now := time.Now()
	rule.CreatedAt = now
	rule.ModifiedAt = now
	b.samplingRules.Put(&rule)
	b.lastRuleModification = now

	return cloneRule(&rule), nil
}

// GetSamplingRules returns all sampling rules sorted by priority (ascending), then by name for stability.
func (b *InMemoryBackend) GetSamplingRules() []SamplingRule {
	b.mu.RLock("GetSamplingRules")
	defer b.mu.RUnlock()

	all := b.samplingRules.All()
	out := make([]SamplingRule, 0, len(all))

	for _, r := range all {
		out = append(out, *cloneRule(r))
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}

		return out[i].RuleName < out[j].RuleName
	})

	return out
}

// UpdateSamplingRule updates the mutable fields of an existing sampling rule.
// It accepts a SamplingRule struct where non-zero values are applied (legacy API).
func (b *InMemoryBackend) UpdateSamplingRule(ruleName string, updates SamplingRule) (*SamplingRule, error) {
	b.mu.Lock("UpdateSamplingRule")
	defer b.mu.Unlock()

	r, ok := b.samplingRules.Get(ruleName)
	if !ok {
		return nil, fmt.Errorf("%w: sampling rule %s not found", ErrSamplingRuleNotFound, ruleName)
	}

	if updates.ResourceARN != "" {
		r.ResourceARN = updates.ResourceARN
	}

	if updates.ServiceName != "" {
		r.ServiceName = updates.ServiceName
	}

	if updates.ServiceType != "" {
		r.ServiceType = updates.ServiceType
	}

	if updates.Host != "" {
		r.Host = updates.Host
	}

	if updates.HTTPMethod != "" {
		r.HTTPMethod = updates.HTTPMethod
	}

	if updates.URLPath != "" {
		r.URLPath = updates.URLPath
	}

	if updates.Priority > 0 {
		r.Priority = updates.Priority
	}

	r.ModifiedAt = time.Now()
	b.lastRuleModification = r.ModifiedAt

	return cloneRule(r), nil
}

// resolveSamplingRule finds a sampling rule by name (if given), falling back to ARN.
// Must be called with b.mu held (read or write lock).
func (b *InMemoryBackend) resolveSamplingRule(ruleName, ruleARN string) (*SamplingRule, error) {
	if ruleName != "" {
		if r, ok := b.samplingRules.Get(ruleName); ok {
			return r, nil
		}
	} else if ruleARN != "" {
		if list := b.samplingRulesByARN.Get(ruleARN); len(list) > 0 {
			return list[0], nil
		}
	}

	key := ruleName
	if key == "" {
		key = ruleARN
	}

	return nil, fmt.Errorf("%w: sampling rule %s not found", ErrSamplingRuleNotFound, key)
}

// UpdateSamplingRuleWithPointers applies pointer-semantic updates so zero values apply.
// The rule is identified by ruleName if non-empty, otherwise by ruleARN (matching the
// real SamplingRuleUpdate shape, which allows specifying either but not both).
func (b *InMemoryBackend) UpdateSamplingRuleWithPointers(
	ruleName, ruleARN string,
	updates SamplingRuleUpdate,
) (*SamplingRule, error) {
	b.mu.Lock("UpdateSamplingRuleWithPointers")
	defer b.mu.Unlock()

	r, err := b.resolveSamplingRule(ruleName, ruleARN)
	if err != nil {
		return nil, err
	}

	if updates.SamplingRateBoost != nil {
		boost := *updates.SamplingRateBoost
		r.SamplingRateBoost = &boost
	}

	if updates.FixedRate != nil {
		r.FixedRate = *updates.FixedRate
	}

	if updates.ReservoirSize != nil {
		r.ReservoirSize = *updates.ReservoirSize
	}

	if updates.ResourceARN != nil {
		r.ResourceARN = *updates.ResourceARN
	}

	if updates.ServiceName != nil {
		r.ServiceName = *updates.ServiceName
	}

	if updates.ServiceType != nil {
		r.ServiceType = *updates.ServiceType
	}

	if updates.Host != nil {
		r.Host = *updates.Host
	}

	if updates.HTTPMethod != nil {
		r.HTTPMethod = *updates.HTTPMethod
	}

	if updates.URLPath != nil {
		r.URLPath = *updates.URLPath
	}

	if updates.Priority != nil {
		r.Priority = *updates.Priority
	}

	if updates.Attributes != nil {
		r.Attributes = maps.Clone(updates.Attributes)
	}

	r.ModifiedAt = time.Now()
	b.lastRuleModification = r.ModifiedAt

	return cloneRule(r), nil
}

// DeleteSamplingRule removes the sampling rule identified by ruleName (if non-empty,
// else ruleARN) and returns it. The built-in "Default" rule cannot be deleted, whether
// identified by name or ARN; attempting to do so returns ErrDefaultRuleUndeletable.
func (b *InMemoryBackend) DeleteSamplingRule(ruleName, ruleARN string) (*SamplingRule, error) {
	b.mu.Lock("DeleteSamplingRule")
	defer b.mu.Unlock()

	r, err := b.resolveSamplingRule(ruleName, ruleARN)
	if err != nil {
		return nil, err
	}

	if r.RuleName == defaultSamplingRuleName {
		return nil, fmt.Errorf(
			"%w: the %s sampling rule cannot be deleted",
			ErrDefaultRuleUndeletable,
			defaultSamplingRuleName,
		)
	}

	deleted := cloneRule(r)
	b.samplingRules.Delete(r.RuleName)
	delete(b.resourceTags, r.RuleARN)
	b.lastRuleModification = time.Now()

	return deleted, nil
}

const (
	// maxServiceNameLen is the maximum length of a sampling rule ServiceName.
	maxServiceNameLen = 64
)

const (
	// maxSamplingRules is the AWS default Service Quota "Custom sampling rules
	// per region" (docs.aws.amazon.com/general/latest/gr/xray.html); previously
	// wrongly assumed to be 2000.
	maxSamplingRules = 25
)

const (
	// defaultFixedRate is the FixedRate of the built-in Default sampling rule.
	defaultFixedRate = 0.05
)

const (
	// defaultSamplingPriority is the priority of the built-in Default sampling rule.
	defaultSamplingPriority = int32(10000)
)
