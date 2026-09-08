package iot

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreatePolicy creates a new IoT Policy.
func (b *InMemoryBackend) CreatePolicy(input *CreatePolicyInput) (*CreatePolicyOutput, error) {
	if input.PolicyName == "" {
		return nil, fmt.Errorf("%w: PolicyName is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.policies.Has(input.PolicyName) {
		return nil, fmt.Errorf("%w: policy %q already exists", ErrAlreadyExists, input.PolicyName)
	}

	arn := arn.Build("iot", b.region, b.accountID, fmt.Sprintf("policy/%s", input.PolicyName))
	now := time.Now()

	b.policies.Put(&Policy{
		PolicyName:     input.PolicyName,
		PolicyDocument: input.PolicyDocument,
		ARN:            arn,
		CreatedAt:      now,
		LastModifiedAt: now,
	})

	// AWS automatically creates version "1" as the default on CreatePolicy.
	b.policyVersions[input.PolicyName] = []*PolicyVersion{
		{
			VersionID:        "1",
			PolicyDocument:   input.PolicyDocument,
			IsDefaultVersion: true,
			CreatedAt:        now,
		},
	}
	b.putResourceTagsLocked(arn, input.Tags)

	return &CreatePolicyOutput{
		PolicyName:      input.PolicyName,
		PolicyARN:       arn,
		PolicyDocument:  input.PolicyDocument,
		PolicyVersionID: "1",
	}, nil
}

// AttachPrincipalPolicy attaches a policy to a principal. The attachment is
// recorded in the same policyTargets index used by AttachPolicy/DetachPolicy
// so that ListPrincipalPolicies, ListPolicyPrincipals, and
// DetachPrincipalPolicy observe it.
func (b *InMemoryBackend) AttachPrincipalPolicy(input *AttachPrincipalPolicyInput) error {
	if input.PolicyName == "" || input.Principal == "" {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.policyTargets[input.PolicyName] = appendUnique(b.policyTargets[input.PolicyName], input.Principal)

	return nil
}

// AttachPolicy attaches a policy to a target (thing group or certificate).
func (b *InMemoryBackend) AttachPolicy(input *AttachPolicyInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.policyTargets[input.PolicyName] = appendUnique(b.policyTargets[input.PolicyName], input.Target)

	return nil
}

// GetPolicy retrieves an existing Policy by name.
func (b *InMemoryBackend) GetPolicy(policyName string) (*GetPolicyOutput, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	p, ok := b.policies.Get(policyName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPolicyNotFound, policyName)
	}

	defaultVersionID := "1"
	for _, v := range b.policyVersions[policyName] {
		if v.IsDefaultVersion {
			defaultVersionID = v.VersionID

			break
		}
	}

	return &GetPolicyOutput{
		PolicyName:       p.PolicyName,
		PolicyARN:        p.ARN,
		PolicyDocument:   p.PolicyDocument,
		CreatedAt:        p.CreatedAt,
		LastModifiedAt:   p.LastModifiedAt,
		DefaultVersionID: defaultVersionID,
	}, nil
}

// DeletePolicy removes a Policy by name.
func (b *InMemoryBackend) DeletePolicy(policyName string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.policies.Has(policyName) {
		return fmt.Errorf("%w: %s", ErrPolicyNotFound, policyName)
	}

	if targets := b.policyTargets[policyName]; len(targets) > 0 {
		return fmt.Errorf("%w: policy %q has attached targets", ErrDeleteConflict, policyName)
	}

	p, _ := b.policies.Get(policyName)

	b.policies.Delete(policyName)
	delete(b.resourceTags, p.ARN)
	delete(b.policyVersions, policyName)

	return nil
}

// ListPolicies returns all policies sorted by name.
func (b *InMemoryBackend) ListPolicies() []*Policy {
	b.mu.RLock()
	defer b.mu.RUnlock()

	items := b.policies.Snapshot()
	out := make([]*Policy, 0, len(items))

	for _, v := range items {
		cp := *v
		out = append(out, &cp)
	}

	return out
}

// AddPolicyInternal seeds a Policy directly into the backend for testing.
func (b *InMemoryBackend) AddPolicyInternal(p Policy) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if p.ARN == "" {
		p.ARN = arn.Build("iot", b.region, b.accountID, fmt.Sprintf("policy/%s", p.PolicyName))
	}

	b.policies.Put(&p)
}

// DetachPolicy detaches a policy from a target.
func (b *InMemoryBackend) DetachPolicy(input *DetachPolicyInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	targets := b.policyTargets[input.PolicyName]
	filtered := make([]string, 0, len(targets))

	for _, t := range targets {
		if t != input.Target {
			filtered = append(filtered, t)
		}
	}

	b.policyTargets[input.PolicyName] = filtered

	return nil
}

// ListAttachedPolicies returns all policies attached to a target.
func (b *InMemoryBackend) ListAttachedPolicies(input *ListAttachedPoliciesInput) ([]*Policy, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []*Policy

	for policyName, targets := range b.policyTargets {
		if slices.Contains(targets, input.Target) {
			if p, ok := b.policies.Get(policyName); ok {
				cp := *p
				out = append(out, &cp)
			}
		}
	}

	return out, nil
}

// maxPolicyVersions is the maximum number of versions allowed per policy (AWS limit).
const maxPolicyVersions = 5

// CreatePolicyVersion creates a new version of an existing policy.
func (b *InMemoryBackend) CreatePolicyVersion(input *CreatePolicyVersionInput) (*PolicyVersion, error) {
	if input.PolicyName == "" {
		return nil, fmt.Errorf("%w: PolicyName is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	p, ok := b.policies.Get(input.PolicyName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPolicyNotFound, input.PolicyName)
	}

	versions := b.policyVersions[input.PolicyName]

	if len(versions) >= maxPolicyVersions {
		return nil, fmt.Errorf(
			"%w: policy %q already has %d versions",
			ErrVersionsLimitExceeded,
			input.PolicyName,
			maxPolicyVersions,
		)
	}

	versionID := strconv.Itoa(len(versions) + 1)

	if input.SetAsDefault {
		for _, v := range versions {
			v.IsDefaultVersion = false
		}
	}

	now := time.Now()
	pv := &PolicyVersion{
		VersionID:        versionID,
		PolicyDocument:   input.PolicyDocument,
		IsDefaultVersion: input.SetAsDefault,
		CreatedAt:        now,
		GenerationID:     uuid.NewString(),
	}

	b.policyVersions[input.PolicyName] = append(versions, pv)
	p.LastModifiedAt = now

	return pv, nil
}

// GetPolicyVersion retrieves a specific version of a policy.
func (b *InMemoryBackend) GetPolicyVersion(policyName, versionID string) (*PolicyVersion, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	versions := b.policyVersions[policyName]

	for _, v := range versions {
		if v.VersionID == versionID {
			cp := *v

			return &cp, nil
		}
	}

	return nil, fmt.Errorf("%w: %s/%s", ErrPolicyVersionNotFound, policyName, versionID)
}

// ListPolicyVersions returns all versions of a policy.
func (b *InMemoryBackend) ListPolicyVersions(policyName string) ([]*PolicyVersion, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.policies.Has(policyName) {
		return nil, fmt.Errorf("%w: %s", ErrPolicyNotFound, policyName)
	}

	versions := b.policyVersions[policyName]
	out := make([]*PolicyVersion, len(versions))

	for i, v := range versions {
		cp := *v
		out[i] = &cp
	}

	return out, nil
}

// DeletePolicyVersion deletes a specific version of a policy.
// The default version cannot be deleted.
func (b *InMemoryBackend) DeletePolicyVersion(policyName, versionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	versions := b.policyVersions[policyName]
	filtered := make([]*PolicyVersion, 0, len(versions))
	found := false

	for _, v := range versions {
		if v.VersionID == versionID {
			found = true
			if v.IsDefaultVersion {
				return fmt.Errorf("%w: cannot delete default policy version %s", ErrDeleteConflict, versionID)
			}
		} else {
			filtered = append(filtered, v)
		}
	}

	if !found {
		return fmt.Errorf("%w: %s/%s", ErrPolicyVersionNotFound, policyName, versionID)
	}

	b.policyVersions[policyName] = filtered

	return nil
}

// SetDefaultPolicyVersion sets the default version of a policy.
func (b *InMemoryBackend) SetDefaultPolicyVersion(policyName, versionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	versions := b.policyVersions[policyName]
	found := false

	for _, v := range versions {
		if v.VersionID == versionID {
			v.IsDefaultVersion = true
			found = true
		} else {
			v.IsDefaultVersion = false
		}
	}

	if !found {
		return fmt.Errorf("%w: %s/%s", ErrPolicyVersionNotFound, policyName, versionID)
	}

	return nil
}

func (b *InMemoryBackend) ListPrincipalPolicies(principal string) []*Policy {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []*Policy
	for policyName, targets := range b.policyTargets {
		if slices.Contains(targets, principal) {
			if p, ok := b.policies.Get(policyName); ok {
				cp := *p
				out = append(out, &cp)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PolicyName < out[j].PolicyName })

	return out
}

func (b *InMemoryBackend) ListPolicyPrincipals(policyName string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := append([]string(nil), b.policyTargets[policyName]...)
	sort.Strings(out)

	return out
}

func (b *InMemoryBackend) ListTargetsForPolicy(policyName string) []string {
	return b.ListPolicyPrincipals(policyName)
}

func (b *InMemoryBackend) ListPrincipalThings(principal string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []string
	for thingName, principals := range b.thingPrincipals {
		if slices.Contains(principals, principal) {
			out = append(out, thingName)
		}
	}
	sort.Strings(out)

	return out
}

func (b *InMemoryBackend) GetEffectivePolicies(thingName, principal string) []*Policy {
	b.mu.RLock()
	defer b.mu.RUnlock()

	seen := map[string]bool{}
	var out []*Policy

	// Collect all principals for this thing.
	var principals []string
	if principal != "" {
		principals = append(principals, principal)
	}
	principals = append(principals, b.thingPrincipals[thingName]...)

	for policyName, targets := range b.policyTargets {
		for _, target := range targets {
			if slices.Contains(principals, target) && !seen[policyName] {
				if pol, ok := b.policies.Get(policyName); ok {
					cp := *pol
					out = append(out, &cp)
					seen[policyName] = true
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PolicyName < out[j].PolicyName })

	return out
}

// DetachPrincipalPolicy removes a principal from a policy's target list,
// mirroring AttachPrincipalPolicy/DetachPolicy's use of policyTargets.
func (b *InMemoryBackend) DetachPrincipalPolicy(policyName, principal string) error {
	if policyName == "" || principal == "" {
		return fmt.Errorf("%w: policyName and principal are required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.policies.Has(policyName) {
		return fmt.Errorf("%w: %s", ErrPolicyNotFound, policyName)
	}

	targets := b.policyTargets[policyName]
	filtered := make([]string, 0, len(targets))

	for _, t := range targets {
		if t != principal {
			filtered = append(filtered, t)
		}
	}

	b.policyTargets[policyName] = filtered

	return nil
}
