package awsconfig

import "fmt"

const orgRuleStatusCreateSuccessful = "CREATE_SUCCESSFUL"

// PutOrganizationConfigRule creates or updates an organization config rule,
// returning its ARN. The ARN is generated once on create and preserved on
// update, mirroring putConfigRuleLocked's config-rule ARN convention
// (config_rules.go) with the "organization-config-rule" resource type.
func (b *InMemoryBackend) PutOrganizationConfigRule(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: OrganizationConfigRuleName is required", ErrValidation)
	}

	b.mu.Lock("PutOrganizationConfigRule")
	defer b.mu.Unlock()

	arnStr := b.organizationConfigRuleArnLocked(name)

	b.orgConfigRules.Put(&OrganizationConfigRule{
		OrganizationConfigRuleName: name,
		OrganizationConfigRuleArn:  arnStr,
	})

	return arnStr, nil
}

// organizationConfigRuleArnLocked returns name's ARN, generating and
// counting a new one only if this is the first time name has been put.
// Callers must already hold the write lock.
func (b *InMemoryBackend) organizationConfigRuleArnLocked(name string) string {
	if existing, ok := b.orgConfigRules.Get(name); ok {
		return existing.OrganizationConfigRuleArn
	}

	b.orgRuleCounter++

	return fmt.Sprintf(
		"arn:aws:config:%s:%s:organization-config-rule/organization-config-rule-%08d",
		b.region, b.accountID, b.orgRuleCounter,
	)
}

// DeleteOrganizationConfigRule deletes an organization config rule by name.
func (b *InMemoryBackend) DeleteOrganizationConfigRule(name string) error {
	if name == "" {
		// Declared set is NoSuchOrganizationConfigRuleException/
		// OrganizationAccessDeniedException/ResourceInUseException only -- no
		// validation-shaped code fits an empty name (configservice@v1.68.4 deserializers.go).
		return fmt.Errorf("%w: OrganizationConfigRuleName is required", ErrValidation)
	}

	b.mu.Lock("DeleteOrganizationConfigRule")
	defer b.mu.Unlock()

	if !b.orgConfigRules.Has(name) {
		return fmt.Errorf("%w: %s", ErrNoSuchOrganizationConfigRule, name)
	}

	b.orgConfigRules.Delete(name)

	return nil
}

// PutOrganizationConformancePack creates or updates an organization conformance pack.
func (b *InMemoryBackend) PutOrganizationConformancePack(name string) error {
	if name == "" {
		return fmt.Errorf("%w: OrganizationConformancePackName is required", ErrValidation)
	}

	b.mu.Lock("PutOrganizationConformancePack")
	defer b.mu.Unlock()

	b.orgConformancePacks.Put(&OrganizationConformancePack{OrganizationConformancePackName: name})

	return nil
}

// DeleteOrganizationConformancePack deletes an organization conformance pack by name.
func (b *InMemoryBackend) DeleteOrganizationConformancePack(name string) error {
	if name == "" {
		// Same shape as DeleteOrganizationConfigRule -- declared set is
		// NoSuchOrganizationConformancePackException/OrganizationAccessDeniedException/
		// ResourceInUseException only, no fitting validation code.
		return fmt.Errorf("%w: OrganizationConformancePackName is required", ErrValidation)
	}

	b.mu.Lock("DeleteOrganizationConformancePack")
	defer b.mu.Unlock()

	if !b.orgConformancePacks.Has(name) {
		return fmt.Errorf("%w: %s", ErrNoSuchOrganizationConformancePack, name)
	}

	b.orgConformancePacks.Delete(name)

	return nil
}

// DescribeOrganizationConfigRules returns all organization config rules.
func (b *InMemoryBackend) DescribeOrganizationConfigRules() []OrganizationConfigRule {
	b.mu.RLock("DescribeOrganizationConfigRules")
	defer b.mu.RUnlock()

	all := b.orgConfigRules.All()
	out := make([]OrganizationConfigRule, 0, len(all))

	for _, r := range all {
		out = append(out, *r)
	}

	return out
}

// DescribeOrganizationConformancePacks returns all organization conformance packs.
func (b *InMemoryBackend) DescribeOrganizationConformancePacks() []OrganizationConformancePack {
	b.mu.RLock("DescribeOrganizationConformancePacks")
	defer b.mu.RUnlock()

	all := b.orgConformancePacks.All()
	out := make([]OrganizationConformancePack, 0, len(all))

	for _, p := range all {
		out = append(out, *p)
	}

	return out
}

// GetOrganizationConfigRuleDetailedStatus returns one MemberAccountStatus per
// member account in the organization for ruleName. This emulator models only
// the local account as the organization's single member (it has no real
// multi-account membership to enumerate), so the result is a single
// CREATE_SUCCESSFUL entry for the local account unless accountIDFilter
// excludes it. Errors with ErrNoSuchOrganizationConfigRule when ruleName is
// unknown, matching real AWS Config's declared error model (verified against
// aws-sdk-go-v2/service/configservice's GetOrganizationConfigRuleDetailedStatus
// deserializer).
func (b *InMemoryBackend) GetOrganizationConfigRuleDetailedStatus(
	ruleName, accountIDFilter string,
) ([]MemberAccountStatus, error) {
	b.mu.RLock("GetOrganizationConfigRuleDetailedStatus")
	defer b.mu.RUnlock()

	if !b.orgConfigRules.Has(ruleName) {
		return nil, fmt.Errorf("%w: %s", ErrNoSuchOrganizationConfigRule, ruleName)
	}

	if accountIDFilter != "" && accountIDFilter != b.accountID {
		return []MemberAccountStatus{}, nil
	}

	return []MemberAccountStatus{{
		AccountID:               b.accountID,
		ConfigRuleName:          ruleName,
		MemberAccountRuleStatus: orgRuleStatusCreateSuccessful,
	}}, nil
}

// GetOrganizationConformancePackDetailedStatus returns one
// OrganizationConformancePackDetailedStatus per member account in the
// organization for packName, mirroring
// GetOrganizationConfigRuleDetailedStatus's single-local-account model.
// Errors with ErrNoSuchOrganizationConformancePack when packName is unknown,
// matching real AWS Config's declared error model (verified against
// aws-sdk-go-v2/service/configservice's
// GetOrganizationConformancePackDetailedStatus deserializer).
func (b *InMemoryBackend) GetOrganizationConformancePackDetailedStatus(
	packName, accountIDFilter string,
) ([]OrganizationConformancePackDetailedStatus, error) {
	b.mu.RLock("GetOrganizationConformancePackDetailedStatus")
	defer b.mu.RUnlock()

	if !b.orgConformancePacks.Has(packName) {
		return nil, fmt.Errorf("%w: %s", ErrNoSuchOrganizationConformancePack, packName)
	}

	if accountIDFilter != "" && accountIDFilter != b.accountID {
		return []OrganizationConformancePackDetailedStatus{}, nil
	}

	return []OrganizationConformancePackDetailedStatus{{
		AccountID:           b.accountID,
		ConformancePackName: packName,
		Status:              orgRuleStatusCreateSuccessful,
	}}, nil
}

// DescribeOrganizationConfigRuleStatuses returns statuses for organization config rules.
// If names is empty, all rules are returned.
func (b *InMemoryBackend) DescribeOrganizationConfigRuleStatuses(names []string) []OrganizationConfigRuleStatus {
	b.mu.RLock("DescribeOrganizationConfigRuleStatuses")
	defer b.mu.RUnlock()

	if len(names) == 0 {
		all := b.orgConfigRules.All()
		out := make([]OrganizationConfigRuleStatus, 0, len(all))

		for _, r := range all {
			out = append(out, OrganizationConfigRuleStatus{
				OrganizationConfigRuleName: r.OrganizationConfigRuleName,
				OrganizationRuleStatus:     orgRuleStatusCreateSuccessful,
			})
		}

		return out
	}

	out := make([]OrganizationConfigRuleStatus, 0, len(names))

	for _, name := range names {
		if r, ok := b.orgConfigRules.Get(name); ok {
			out = append(out, OrganizationConfigRuleStatus{
				OrganizationConfigRuleName: r.OrganizationConfigRuleName,
				OrganizationRuleStatus:     orgRuleStatusCreateSuccessful,
			})
		}
	}

	return out
}

// DescribeOrganizationConformancePackStatuses returns statuses for organization conformance packs.
// If names is empty, all packs are returned.
func (b *InMemoryBackend) DescribeOrganizationConformancePackStatuses(
	names []string,
) []OrganizationConformancePackStatus {
	b.mu.RLock("DescribeOrganizationConformancePackStatuses")
	defer b.mu.RUnlock()

	if len(names) == 0 {
		all := b.orgConformancePacks.All()
		out := make([]OrganizationConformancePackStatus, 0, len(all))

		for _, p := range all {
			out = append(out, OrganizationConformancePackStatus{
				OrganizationConformancePackName: p.OrganizationConformancePackName,
				Status:                          orgRuleStatusCreateSuccessful,
			})
		}

		return out
	}

	out := make([]OrganizationConformancePackStatus, 0, len(names))

	for _, name := range names {
		if p, ok := b.orgConformancePacks.Get(name); ok {
			out = append(out, OrganizationConformancePackStatus{
				OrganizationConformancePackName: p.OrganizationConformancePackName,
				Status:                          orgRuleStatusCreateSuccessful,
			})
		}
	}

	return out
}

// GetOrganizationCustomRulePolicy returns the policy text for the given org custom rule.
func (b *InMemoryBackend) GetOrganizationCustomRulePolicy(ruleName string) string {
	b.mu.RLock("GetOrganizationCustomRulePolicy")
	defer b.mu.RUnlock()

	return b.orgCustomRulePolicies[ruleName]
}
