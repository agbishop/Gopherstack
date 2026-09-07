package organizations

import (
	"cmp"
	"encoding/json"
	"slices"
)

const policyStatusEnabled = "ENABLED"

// Policy type names, shared between validPolicyTypes, policyContentMaxSize, and
// effective_policy.go's merge-style switch (avoids goconst violations from
// repeating these literals across the package).
const (
	policyTypeSCP      = "SERVICE_CONTROL_POLICY"
	policyTypeRCP      = "RESOURCE_CONTROL_POLICY"
	policyTypeTag      = "TAG_POLICY"
	policyTypeBackup   = "BACKUP_POLICY"
	policyTypeAIOptOut = "AISERVICES_OPT_OUT_POLICY"
	policyTypeChatbot  = "CHATBOT_POLICY"
	policyTypeDeclEC2  = "DECLARATIVE_POLICY_EC2"
	policyTypeSecHub   = "SECURITYHUB_POLICY"
)

// fullAWSAccessPolicyID, Name and Description match the id/name/description
// AWS assigns the default SCP every organization is created with (verified
// against live `aws organizations describe-policy --policy-id
// p-FullAWSAccess` output; the pinned SDK ships no policy id/name/content
// constants of its own -- see types.PolicySummary's doc comment, which only
// describes the shape). Content is the well-known full-access SCP body AWS
// documents for it (docs.aws.amazon.com/organizations/latest/userguide
// /orgs_manage_policies_scps_example-scps.html); not SDK-verified either,
// since PolicyContent carries no default in the model.
const (
	fullAWSAccessPolicyID          = "p-FullAWSAccess"
	fullAWSAccessPolicyName        = "FullAWSAccess"
	fullAWSAccessPolicyDescription = "Allows access to every operation"
	fullAWSAccessPolicyContent     = `{"Version":"2012-10-17","Statement":` +
		`[{"Effect":"Allow","Action":"*","Resource":"*"}]}`
)

// validPolicyTypes returns the policy types supported by AWS Organizations.
func validPolicyTypes() []string {
	return []string{
		policyTypeSCP,
		policyTypeRCP,
		policyTypeTag,
		policyTypeBackup,
		policyTypeAIOptOut,
		policyTypeChatbot,
		policyTypeDeclEC2,
		policyTypeSecHub,
	}
}

// Maximum policy document sizes (characters), the account-level DEFAULT quota
// only -- not the quota-increase path (no service-quota-increase API is
// emulated here). Verified against the "Maximum size of a policy document"
// row of docs.aws.amazon.com/organizations/latest/userguide/orgs_reference_limits.html
// (fetched live; the PolicyContent shape in botocore's organizations model has
// no `max`, i.e. this limit is account-quota state, not a static shape
// constraint). CHATBOT_POLICY ("Chat applications policies") and
// SECURITYHUB_POLICY both independently confirmed at 10,000 on that same
// page -- no longer an unverified guess.
const (
	policyContentLimitSCP      = 10240
	policyContentLimitRCP      = 5120
	policyContentLimitTag      = 10000
	policyContentLimitBackup   = 10000
	policyContentLimitAIOptOut = 2500
	policyContentLimitChatbot  = 10000
	policyContentLimitDeclEC2  = 10000
	policyContentLimitSecHub   = 10000
)

// policyContentMaxSize returns the maximum content size for policyType and
// whether policyType is a recognized type with a modeled limit.
func policyContentMaxSize(policyType string) (int, bool) {
	switch policyType {
	case policyTypeSCP:
		return policyContentLimitSCP, true
	case policyTypeRCP:
		return policyContentLimitRCP, true
	case policyTypeTag:
		return policyContentLimitTag, true
	case policyTypeBackup:
		return policyContentLimitBackup, true
	case policyTypeAIOptOut:
		return policyContentLimitAIOptOut, true
	case policyTypeChatbot:
		return policyContentLimitChatbot, true
	case policyTypeDeclEC2:
		return policyContentLimitDeclEC2, true
	case policyTypeSecHub:
		return policyContentLimitSecHub, true
	default:
		return 0, false
	}
}

// validatePolicyContent checks content against AWS's syntax and size rules for
// policyType. Real AWS rejects non-JSON policy documents with
// MalformedPolicyDocumentException and documents exceeding the per-type size
// quota with ConstraintViolationException(POLICY_CONTENT_LIMIT_EXCEEDED).
func validatePolicyContent(content, policyType string) error {
	if !json.Valid([]byte(content)) {
		return ErrMalformedPolicyDocument
	}

	if limit, ok := policyContentMaxSize(policyType); ok && len(content) > limit {
		return ErrPolicyContentLimitExceeded
	}

	return nil
}

// seedFullAWSAccessPolicyLocked creates the default FullAWSAccess SCP that
// real AWS Organizations creates with every organization, and attaches it to
// root when SCPs are enabled by default (featureSet == ALL; CreateOrganization's
// doc comment: "By default (or if you set FeatureSet to ALL) ... service
// control policies automatically enabled in the root. If you instead choose
// ... CONSOLIDATED_BILLING ... no policy types are enabled by default").
// Must be called with the write lock held, after b.org and b.root are set.
func (b *InMemoryBackend) seedFullAWSAccessPolicyLocked(featureSet, orgID, rootID string) {
	p := &Policy{
		PolicySummary: PolicySummary{
			ID:          fullAWSAccessPolicyID,
			ARN:         b.policyARN(orgID, policyTypeSCP, fullAWSAccessPolicyID),
			Name:        fullAWSAccessPolicyName,
			Description: fullAWSAccessPolicyDescription,
			Type:        policyTypeSCP,
			AwsManaged:  true,
		},
		Content: fullAWSAccessPolicyContent,
	}

	b.policies.Put(p)
	b.policyTargets[fullAWSAccessPolicyID] = []string{}

	if featureSet == featureSetAll {
		b.policyTargets[fullAWSAccessPolicyID] = append(b.policyTargets[fullAWSAccessPolicyID], rootID)
		b.targetPolicies[rootID] = append(b.targetPolicies[rootID], fullAWSAccessPolicyID)
	}
}

// CreatePolicy creates a new policy.
func (b *InMemoryBackend) CreatePolicy(
	name, description, content, policyType string,
	tags []Tag,
) (*Policy, error) {
	b.mu.Lock("CreatePolicy")
	defer b.mu.Unlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if !slices.Contains(validPolicyTypes(), policyType) {
		return nil, ErrInvalidInput
	}

	if err := validatePolicyContent(content, policyType); err != nil {
		return nil, err
	}

	if err := validateNewTags(nil, tags); err != nil {
		return nil, err
	}

	policyID := newPolicyID()
	p := &Policy{
		PolicySummary: PolicySummary{
			ID:          policyID,
			ARN:         b.policyARN(b.org.ID, policyType, policyID),
			Name:        name,
			Description: description,
			Type:        policyType,
			AwsManaged:  false,
		},
		Content: content,
	}

	b.policies.Put(p)
	b.policyTargets[policyID] = []string{}
	b.setTagsLocked(policyID, tags)

	return p, nil
}

// DescribePolicy returns a policy by ID.
func (b *InMemoryBackend) DescribePolicy(policyID string) (*Policy, error) {
	b.mu.RLock("DescribePolicy")
	defer b.mu.RUnlock()

	p, ok := b.policies.Get(policyID)
	if !ok {
		return nil, ErrPolicyNotFound
	}

	return copyPolicy(p), nil
}

// UpdatePolicy updates a policy.
func (b *InMemoryBackend) UpdatePolicy(
	policyID, name, description, content string,
) (*Policy, error) {
	b.mu.Lock("UpdatePolicy")
	defer b.mu.Unlock()

	p, ok := b.policies.Get(policyID)
	if !ok {
		return nil, ErrPolicyNotFound
	}

	if p.PolicySummary.AwsManaged {
		return nil, ErrAccessDeniedManagedPolicy
	}

	if content != "" {
		if err := validatePolicyContent(content, p.PolicySummary.Type); err != nil {
			return nil, err
		}
	}

	if name != "" {
		p.PolicySummary.Name = name
	}

	if description != "" {
		p.PolicySummary.Description = description
	}

	if content != "" {
		p.Content = content
	}

	return copyPolicy(p), nil
}

// DeletePolicy removes a policy.
func (b *InMemoryBackend) DeletePolicy(policyID string) error {
	b.mu.Lock("DeletePolicy")
	defer b.mu.Unlock()

	p, ok := b.policies.Get(policyID)
	if !ok {
		return ErrPolicyNotFound
	}

	if p.PolicySummary.AwsManaged {
		return ErrAccessDeniedManagedPolicy
	}

	// AWS rejects deletion of policies that are still attached to targets.
	if len(b.policyTargets[policyID]) > 0 {
		return ErrPolicyInUse
	}

	delete(b.policyTargets, policyID)
	b.policies.Delete(policyID)
	delete(b.tags, policyID)

	return nil
}

// ListPolicies returns all policies of a given type.
// AWS requires a non-empty Filter; empty filter returns InvalidInputException.
func (b *InMemoryBackend) ListPolicies(filter string) ([]*Policy, error) {
	b.mu.RLock("ListPolicies")
	defer b.mu.RUnlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if filter == "" {
		return nil, ErrInvalidInput
	}

	var out []*Policy

	for _, p := range b.policies.All() {
		if p.PolicySummary.Type == filter {
			out = append(out, copyPolicy(p))
		}
	}

	slices.SortFunc(out, func(a, b *Policy) int {
		if c := cmp.Compare(a.PolicySummary.Name, b.PolicySummary.Name); c != 0 {
			return c
		}

		return cmp.Compare(a.PolicySummary.ID, b.PolicySummary.ID)
	})

	return out, nil
}

// EnablePolicyType enables a policy type on the root.
func (b *InMemoryBackend) EnablePolicyType(rootID, policyType string) (*Root, error) {
	b.mu.Lock("EnablePolicyType")
	defer b.mu.Unlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if b.root == nil || b.root.ID != rootID {
		return nil, ErrInvalidInput
	}

	if !slices.Contains(validPolicyTypes(), policyType) {
		return nil, ErrInvalidInput
	}

	for _, pt := range b.root.PolicyTypes {
		if pt.Type == policyType && pt.Status == policyStatusEnabled {
			return nil, ErrPolicyTypeAlreadyEnabled
		}
	}

	b.root.PolicyTypes = append(b.root.PolicyTypes, PolicyTypeSummary{
		Type:   policyType,
		Status: policyStatusEnabled,
	})

	return copyRoot(b.root), nil
}

// DisablePolicyType disables a policy type on the root.
func (b *InMemoryBackend) DisablePolicyType(rootID, policyType string) (*Root, error) {
	b.mu.Lock("DisablePolicyType")
	defer b.mu.Unlock()

	if b.org == nil {
		return nil, ErrOrgNotFound
	}

	if b.root == nil || b.root.ID != rootID {
		return nil, ErrInvalidInput
	}

	if !slices.Contains(validPolicyTypes(), policyType) {
		return nil, ErrInvalidInput
	}

	newTypes := make([]PolicyTypeSummary, 0, len(b.root.PolicyTypes))

	found := false

	for _, pt := range b.root.PolicyTypes {
		if pt.Type == policyType {
			found = true

			continue
		}

		newTypes = append(newTypes, pt)
	}

	if !found {
		return nil, ErrPolicyTypeNotEnabled
	}

	// AWS rejects disabling a policy type when policies of that type are still attached to any target.
	for policyID, targets := range b.policyTargets {
		if len(targets) > 0 {
			if p, ok := b.policies.Get(policyID); ok && p.PolicySummary.Type == policyType {
				return nil, ErrPolicyTypeAttached
			}
		}
	}

	b.root.PolicyTypes = newTypes

	return copyRoot(b.root), nil
}

// AddPolicyInternal seeds a policy directly for testing.
func (b *InMemoryBackend) AddPolicyInternal(p *Policy) {
	b.mu.Lock("AddPolicyInternal")
	defer b.mu.Unlock()

	b.policies.Put(p)

	if b.policyTargets[p.PolicySummary.ID] == nil {
		b.policyTargets[p.PolicySummary.ID] = []string{}
	}
}

// copyPolicy returns a value copy of a Policy (all fields are scalars).
func copyPolicy(p *Policy) *Policy {
	cp := *p

	return &cp
}
