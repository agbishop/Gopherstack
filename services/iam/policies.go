package iam

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// AWS IAM managed policy document quota (UTF-8 bytes).
const maxManagedPolicySize = 6144

// CreatePolicy creates a new IAM managed policy.
func (b *InMemoryBackend) CreatePolicy(policyName, path, policyDocument string) (*Policy, error) {
	b.mu.Lock("CreatePolicy")
	defer b.mu.Unlock()

	if _, exists := b.policies.Get(policyName); exists {
		return nil, fmt.Errorf("%w: policy %q already exists", ErrPolicyAlreadyExists, policyName)
	}

	if err := validateIdentityPolicyDocument(policyDocument); err != nil {
		return nil, err
	}

	if len(policyDocument) > maxManagedPolicySize {
		return nil, fmt.Errorf("%w: managed policy %q exceeds %d bytes",
			ErrLimitExceeded, policyName, maxManagedPolicySize)
	}

	p := normPath(path)
	now := time.Now().UTC()
	pol := Policy{
		PolicyName:       policyName,
		PolicyID:         newID("ANPA"),
		Arn:              arn.Build("iam", "", b.accountID, "policy"+p+policyName),
		Path:             p,
		PolicyDocument:   policyDocument,
		CreateDate:       now,
		UpdateDate:       now,
		DefaultVersionID: "v1",
		IsAttachable:     true,
	}
	b.policies.Put(&pol)
	b.policyByARN[pol.Arn] = policyName
	b.sortedPolicyNames = insertSorted(b.sortedPolicyNames, policyName)

	return &pol, nil
}

// DeletePolicy deletes an IAM policy by ARN.
func (b *InMemoryBackend) DeletePolicy(policyArn string) error {
	b.mu.Lock("DeletePolicy")
	defer b.mu.Unlock()

	if refs, exists := b.policyAttachments[policyArn]; exists {
		if userName := firstKey(refs.users); userName != "" {
			return fmt.Errorf(
				"%w: policy %q is attached to user %q",
				ErrDeleteConflict,
				policyArn,
				userName,
			)
		}

		if roleName := firstKey(refs.roles); roleName != "" {
			return fmt.Errorf(
				"%w: policy %q is attached to role %q",
				ErrDeleteConflict,
				policyArn,
				roleName,
			)
		}

		if groupName := firstKey(refs.groups); groupName != "" {
			return fmt.Errorf(
				"%w: policy %q is attached to group %q",
				ErrDeleteConflict,
				policyArn,
				groupName,
			)
		}
	}

	policyName, exists := b.policyByARN[policyArn]
	if !exists {
		return fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	b.policies.Delete(policyName)
	delete(b.policyByARN, policyArn)
	delete(b.policyAttachments, policyArn)
	delete(b.policyVersions, policyArn)
	delete(b.policyVersionCounters, policyArn)
	delete(b.deletedV1Policies, policyArn)
	b.sortedPolicyNames = deleteSorted(b.sortedPolicyNames, policyName)

	return nil
}

// PermissionsBoundaryARNs returns the set of policy ARNs currently used as a
// permissions boundary by at least one user or role (ListPolicies'
// PolicyUsageFilter=PermissionsBoundary).
func (b *InMemoryBackend) PermissionsBoundaryARNs() map[string]bool {
	b.mu.RLock("PermissionsBoundaryARNs")
	defer b.mu.RUnlock()

	arns := make(map[string]bool)

	for _, u := range b.users.All() {
		if u.PermissionsBoundary != "" {
			arns[u.PermissionsBoundary] = true
		}
	}

	for _, r := range b.roles.All() {
		if r.PermissionsBoundary != "" {
			arns[r.PermissionsBoundary] = true
		}
	}

	return arns
}

// PermissionsBoundaryEntities returns the names of users and roles that
// currently use policyArn as their permissions boundary
// (ListEntitiesForPolicy's PolicyUsageFilter=PermissionsBoundary side).
// Groups have no permissions boundary in real IAM, so only users and roles
// are checked.
func (b *InMemoryBackend) PermissionsBoundaryEntities(policyArn string) ([]string, []string) {
	b.mu.RLock("PermissionsBoundaryEntities")
	defer b.mu.RUnlock()

	var userNames, roleNames []string

	for _, u := range b.users.All() {
		if u.PermissionsBoundary == policyArn {
			userNames = append(userNames, u.UserName)
		}
	}

	for _, r := range b.roles.All() {
		if r.PermissionsBoundary == policyArn {
			roleNames = append(roleNames, r.RoleName)
		}
	}

	return userNames, roleNames
}

// ListPolicies returns a paginated list of IAM policies sorted by name.
func (b *InMemoryBackend) ListPolicies(marker string, maxItems int) (page.Page[Policy], error) {
	b.mu.RLock("ListPolicies")
	defer b.mu.RUnlock()

	return pageFromSortedNames(
		b.sortedPolicyNames,
		b.policies.Get,
		marker,
		maxItems,
		iamDefaultMaxItems,
	), nil
}

// AttachUserPolicy attaches a policy to a user.
func (b *InMemoryBackend) AttachUserPolicy(userName, policyArn string) error {
	b.mu.Lock("AttachUserPolicy")
	defer b.mu.Unlock()

	if _, exists := b.users.Get(userName); !exists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	if slices.Contains(b.userPolicies[userName], policyArn) {
		return nil // already attached
	}

	b.userPolicies[userName] = append(b.userPolicies[userName], policyArn)
	b.addPolicyAttachmentLocked(policyArn, userName, "user")

	return nil
}

// DetachUserPolicy detaches a policy from a user.
func (b *InMemoryBackend) DetachUserPolicy(userName, policyArn string) error {
	b.mu.Lock("DetachUserPolicy")
	defer b.mu.Unlock()

	if _, exists := b.users.Get(userName); !exists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	policies := b.userPolicies[userName]
	for i, p := range policies {
		if p == policyArn {
			b.userPolicies[userName] = append(policies[:i], policies[i+1:]...)
			b.removePolicyAttachmentLocked(policyArn, userName, "user")

			return nil
		}
	}

	return nil
}

// AttachRolePolicy attaches a policy to a role.
func (b *InMemoryBackend) AttachRolePolicy(roleName, policyArn string) error {
	b.mu.Lock("AttachRolePolicy")
	defer b.mu.Unlock()

	if _, exists := b.roles.Get(roleName); !exists {
		return fmt.Errorf("%w: role %q not found", ErrRoleNotFound, roleName)
	}

	if slices.Contains(b.rolePolicies[roleName], policyArn) {
		return nil // already attached
	}

	b.rolePolicies[roleName] = append(b.rolePolicies[roleName], policyArn)
	b.addPolicyAttachmentLocked(policyArn, roleName, "role")

	return nil
}

// DetachRolePolicy detaches a policy from a role.
func (b *InMemoryBackend) DetachRolePolicy(roleName, policyArn string) error {
	b.mu.Lock("DetachRolePolicy")
	defer b.mu.Unlock()

	if _, exists := b.roles.Get(roleName); !exists {
		return fmt.Errorf("%w: role %q not found", ErrRoleNotFound, roleName)
	}

	policies := b.rolePolicies[roleName]
	for i, p := range policies {
		if p == policyArn {
			b.rolePolicies[roleName] = append(policies[:i], policies[i+1:]...)
			b.removePolicyAttachmentLocked(policyArn, roleName, "role")

			return nil
		}
	}

	return nil
}

// ListAllPolicies returns all policies (for dashboard).
func (b *InMemoryBackend) ListAllPolicies() []Policy {
	b.mu.RLock("ListAllPolicies")
	defer b.mu.RUnlock()

	policies := make([]Policy, 0, b.policies.Len())
	for _, p := range b.policies.All() {
		policies = append(policies, *p)
	}

	sort.Slice(
		policies,
		func(i, j int) bool { return policies[i].PolicyName < policies[j].PolicyName },
	)

	return policies
}

// GetPolicy returns the policy metadata for the given ARN.
func (b *InMemoryBackend) GetPolicy(policyArn string) (*Policy, error) {
	b.mu.RLock("GetPolicy")
	defer b.mu.RUnlock()

	pol, exists := b.getPolicyByARNLocked(policyArn)
	if !exists {
		return nil, fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	return &pol, nil
}

// GetPolicyVersion returns the requested version of a managed policy.
// If versionID is empty or "v1", the v1 (original) version info is returned.
func (b *InMemoryBackend) GetPolicyVersion(
	policyArn, versionID string,
) (*StoredPolicyVersion, error) {
	b.mu.RLock("GetPolicyVersion")
	defer b.mu.RUnlock()

	policy, exists := b.getPolicyByARNLocked(policyArn)
	if !exists {
		return nil, fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	for _, version := range b.policyVersions[policyArn] {
		if version.VersionID != versionID {
			continue
		}

		v := version
		v.IsDefaultVersion = (policy.DefaultVersionID == versionID)

		return &v, nil
	}

	if versionID == "" || versionID == "v1" {
		isDefault := policy.DefaultVersionID == "v1" || policy.DefaultVersionID == ""

		return &StoredPolicyVersion{
			VersionID:        "v1",
			PolicyDocument:   policy.PolicyDocument,
			IsDefaultVersion: isDefault,
			CreateDate:       policy.CreateDate,
		}, nil
	}

	return nil, fmt.Errorf("%w: policy version %q not found", ErrPolicyNotFound, versionID)
}

// policyNameFromARN extracts the policy name from an ARN.
// arn:aws:iam::<account>:policy/<name> → <name>
// policyNameFromARN extracts the bare policy name from an ARN. The ARN
// resource is "policy" + Path + PolicyName, and Path always starts and ends
// with "/" (arn.Build via CreatePolicy), so the name is always the text
// after the final "/" -- not everything after "policy/", which for a
// non-default Path (e.g. "/team/") wrongly includes the path segments too.
func policyNameFromARN(arn string) string {
	if i := strings.LastIndex(arn, "/"); i >= 0 {
		return arn[i+1:]
	}

	return arn
}

// purgePoliciesLocked removes policies created before cutoff.
// Caller must hold b.mu.
func (b *InMemoryBackend) purgePoliciesLocked(cutoff time.Time) {
	for _, p := range b.policies.All() {
		if p.CreateDate.Before(cutoff) {
			b.policies.Delete(p.PolicyName)
		}
	}
}

// SetDefaultPolicyVersion sets the specified version as the default for the managed policy.
func (b *InMemoryBackend) SetDefaultPolicyVersion(policyArn, versionID string) error {
	b.mu.Lock("SetDefaultPolicyVersion")
	defer b.mu.Unlock()

	polName, exists := b.policyByARN[policyArn]
	if !exists {
		return fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	pol, polExists := b.policies.Get(polName)
	if !polExists {
		return fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	now := time.Now().UTC()

	if versionID == "v1" {
		// v1 is default when no stored version overrides it.
		versions := b.policyVersions[policyArn]
		for i := range versions {
			versions[i].IsDefaultVersion = false
		}
		b.policyVersions[policyArn] = versions

		pol.DefaultVersionID = "v1"
		pol.UpdateDate = now
		b.policies.Put(pol)

		return nil
	}

	versions := b.policyVersions[policyArn]
	found := false

	for i := range versions {
		if versions[i].VersionID == versionID {
			found = true
			versions[i].IsDefaultVersion = true

			// Update the policy's document, default version, and update timestamp.
			pol.PolicyDocument = versions[i].PolicyDocument
			pol.DefaultVersionID = versionID
			pol.UpdateDate = now
			b.policies.Put(pol)
		} else {
			versions[i].IsDefaultVersion = false
		}
	}

	if !found {
		return fmt.Errorf("%w: policy version %q not found for policy %q", ErrPolicyNotFound, versionID, policyArn)
	}

	b.policyVersions[policyArn] = versions

	return nil
}

// DeletePolicyVersion deletes a non-default version of the managed policy.
func (b *InMemoryBackend) DeletePolicyVersion(policyArn, versionID string) error {
	b.mu.Lock("DeletePolicyVersion")
	defer b.mu.Unlock()

	if _, exists := b.policyByARN[policyArn]; !exists {
		return fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	// v1 is stored implicitly. If being deleted, verify it is not the current default.
	if versionID == "v1" {
		nonV1IsDefault := false

		for _, v := range b.policyVersions[policyArn] {
			if v.IsDefaultVersion {
				nonV1IsDefault = true

				break
			}
		}

		if !nonV1IsDefault {
			// v1 is still the default; cannot delete.
			return fmt.Errorf("%w: cannot delete the default version v1", ErrDeleteConflict)
		}

		// Mark v1 as deleted for this policy.
		b.deletedV1Policies[policyArn] = true

		return nil
	}

	versions := b.policyVersions[policyArn]
	for i, v := range versions {
		if v.VersionID != versionID {
			continue
		}

		if v.IsDefaultVersion {
			return fmt.Errorf("%w: cannot delete the default policy version %q", ErrDeleteConflict, versionID)
		}

		b.policyVersions[policyArn] = append(versions[:i], versions[i+1:]...)

		return nil
	}

	return fmt.Errorf("%w: policy version %q not found for policy %q", ErrPolicyNotFound, versionID, policyArn)
}

// ListEntitiesForPolicy returns the users, groups, and roles that have the specified policy attached.
func (b *InMemoryBackend) ListEntitiesForPolicy(policyArn, entityFilter string) (*PolicyEntities, error) {
	b.mu.RLock("ListEntitiesForPolicy")
	defer b.mu.RUnlock()

	if _, exists := b.getPolicyByARNLocked(policyArn); !exists {
		return nil, fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	refs := b.policyAttachments[policyArn]
	result := &PolicyEntities{}

	if entityFilter == "" || entityFilter == entityTypeUser {
		for userName := range refs.users {
			result.PolicyUsers = append(result.PolicyUsers, PolicyEntityUser{UserName: userName})
		}

		sort.Slice(result.PolicyUsers, func(i, j int) bool {
			return result.PolicyUsers[i].UserName < result.PolicyUsers[j].UserName
		})
	}

	if entityFilter == "" || entityFilter == entityTypeGroup {
		for groupName := range refs.groups {
			result.PolicyGroups = append(result.PolicyGroups, PolicyEntityGroup{GroupName: groupName})
		}

		sort.Slice(result.PolicyGroups, func(i, j int) bool {
			return result.PolicyGroups[i].GroupName < result.PolicyGroups[j].GroupName
		})
	}

	if entityFilter == "" || entityFilter == entityTypeRole {
		for roleName := range refs.roles {
			result.PolicyRoles = append(result.PolicyRoles, PolicyEntityRole{RoleName: roleName})
		}

		sort.Slice(result.PolicyRoles, func(i, j int) bool {
			return result.PolicyRoles[i].RoleName < result.PolicyRoles[j].RoleName
		})
	}

	return result, nil
}

// SimulateCustomPolicy simulates the effect of one or more custom IAM policies against a set of actions and resources.
// This is a best-effort simulation — results are authoritative only for policies provided directly.
func (b *InMemoryBackend) SimulateCustomPolicy(
	policyInputList, permissionsBoundaryPolicyInputList, actionNames, resourceArns []string,
	ctx ConditionContext,
) ([]SimulationResult, error) {
	if len(actionNames) == 0 {
		return nil, fmt.Errorf("%w: at least one action name is required", ErrInvalidInput)
	}

	b.mu.RLock("SimulateCustomPolicy")
	defer b.mu.RUnlock()

	if len(resourceArns) == 0 {
		resourceArns = []string{"*"}
	}

	results := make([]SimulationResult, 0, len(actionNames)*len(resourceArns))

	for _, action := range actionNames {
		for _, resource := range resourceArns {
			results = append(results, simulateCustomPolicyOne(
				policyInputList, permissionsBoundaryPolicyInputList, action, resource, ctx,
			))
		}
	}

	return results, nil
}

func simulateCustomPolicyOne(
	policyInputList, permissionsBoundaryPolicyInputList []string,
	action, resource string,
	ctx ConditionContext,
) SimulationResult {
	evalResult := EvaluatePolicies(policyInputList, action, resource, ctx)

	// Per-policy detail: label each input policy by its index.
	detail := make(map[string]string, len(policyInputList))

	for i, doc := range policyInputList {
		r := EvaluatePolicies([]string{doc}, action, resource, ctx)
		key := fmt.Sprintf("InputPolicy%d", i+1)
		detail[key] = evalDecisionStr(r)
	}

	if len(permissionsBoundaryPolicyInputList) > 0 {
		var explicitDeny, allowed bool

		evalResult, explicitDeny, allowed = applyPermissionsBoundary(
			permissionsBoundaryPolicyInputList, action, resource, ctx, evalResult,
		)
		if explicitDeny {
			evalResult = EvalExplicitDeny
		}

		if allowed {
			detail["PermissionsBoundaryPolicy"] = "allowed"
		} else {
			detail["PermissionsBoundaryPolicy"] = "explicitDeny"
		}
	}

	return SimulationResult{
		ActionName:          action,
		ResourceName:        resource,
		Decision:            evalDecisionStr(evalResult),
		EvalDecisionDetails: detail,
	}
}

// TagPolicy merges the given key-value pairs into the policy's Tags field.
func (b *InMemoryBackend) TagPolicy(policyArn string, tags map[string]string) error {
	b.mu.Lock("TagPolicy")
	defer b.mu.Unlock()

	polName, exists := b.policyByARN[policyArn]
	if !exists {
		return fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	p, exists := b.policies.Get(polName)
	if !exists {
		return fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	if p.Tags == nil {
		p.Tags = make(map[string]string, len(tags))
	}

	maps.Copy(p.Tags, tags)
	b.policies.Put(p)

	return nil
}

// UntagPolicy removes the given keys from the policy's Tags field.
func (b *InMemoryBackend) UntagPolicy(policyArn string, keys []string) error {
	b.mu.Lock("UntagPolicy")
	defer b.mu.Unlock()

	polName, exists := b.policyByARN[policyArn]
	if !exists {
		return fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	p, exists := b.policies.Get(polName)
	if !exists {
		return fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	for _, k := range keys {
		delete(p.Tags, k)
	}

	b.policies.Put(p)

	return nil
}

// PermissionsBoundaryDocForUser returns the policy document for the user's
// permissions boundary, or "" if none is set. It implements the
// enforcement middleware's optional permissionsBoundaryLookup capability
// (services/iam/middleware.go).
func (b *InMemoryBackend) PermissionsBoundaryDocForUser(userName string) string {
	b.mu.RLock("PermissionsBoundaryDocForUser")
	defer b.mu.RUnlock()

	return b.boundaryDocForUser(userName)
}

// PermissionsBoundaryDocForRole returns the policy document for the role's
// permissions boundary, or "" if none is set. It implements the
// enforcement middleware's optional permissionsBoundaryLookup capability
// (services/iam/middleware.go).
func (b *InMemoryBackend) PermissionsBoundaryDocForRole(roleName string) string {
	b.mu.RLock("PermissionsBoundaryDocForRole")
	defer b.mu.RUnlock()

	return b.boundaryDocForRole(roleName)
}

// boundaryDocForUser returns the policy document for the user's permissions boundary, or "".
// Caller must hold b.mu read-locked.
func (b *InMemoryBackend) boundaryDocForUser(userName string) string {
	u, exists := b.users.Get(userName)
	if !exists || u.PermissionsBoundary == "" {
		return ""
	}

	return b.policyDocByARNLocked(u.PermissionsBoundary)
}

// boundaryDocForRole returns the policy document for the role's permissions boundary, or "".
// Caller must hold b.mu read-locked.
func (b *InMemoryBackend) boundaryDocForRole(roleName string) string {
	r, exists := b.roles.Get(roleName)
	if !exists || r.PermissionsBoundary == "" {
		return ""
	}

	return b.policyDocByARNLocked(r.PermissionsBoundary)
}

// policyDocByARNLocked returns the current policy document for a managed policy ARN.
// Caller must hold b.mu read-locked.
func (b *InMemoryBackend) policyDocByARNLocked(policyArn string) string {
	polName, exists := b.policyByARN[policyArn]
	if !exists {
		return ""
	}

	p, exists := b.policies.Get(polName)
	if !exists {
		return ""
	}

	return p.PolicyDocument
}

// CreatePolicyVersion creates a new version of an existing managed policy.
// Up to five versions can exist at once (AWS limit); this implementation enforces it.
// If setAsDefault is true, the new version becomes the default (and the policy document is updated).
func (b *InMemoryBackend) CreatePolicyVersion(
	policyArn, policyDocument string, setAsDefault bool,
) (*StoredPolicyVersion, error) {
	if policyDocument == "" {
		return nil, fmt.Errorf("%w: policy document must not be empty", ErrMalformedPolicyDocument)
	}

	if err := validateIdentityPolicyDocument(policyDocument); err != nil {
		return nil, err
	}

	b.mu.Lock("CreatePolicyVersion")
	defer b.mu.Unlock()

	polName, exists := b.policyByARN[policyArn]
	if !exists {
		return nil, fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	pol, exists := b.policies.Get(polName)
	if !exists {
		return nil, fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	versions := b.policyVersions[policyArn]

	// AWS allows at most 5 versions per policy (v1 is the original, so
	// the stored extra versions list can hold at most 4 additional versions).
	// Count v1 unless it was explicitly deleted.
	totalVersions := len(versions)
	if !b.deletedV1Policies[policyArn] {
		totalVersions++
	}

	const maxPolicyVersions = 5
	if totalVersions >= maxPolicyVersions {
		return nil, fmt.Errorf(
			"%w: policy %q already has %d versions; delete one before creating another",
			ErrLimitExceeded, policyArn, maxPolicyVersions,
		)
	}

	// Use a monotonic counter to generate version IDs that never collide,
	// even after intermediate versions are deleted.
	b.policyVersionCounters[policyArn]++
	versionID := fmt.Sprintf("v%d", b.policyVersionCounters[policyArn]+1)

	newVersion := StoredPolicyVersion{
		VersionID:        versionID,
		PolicyDocument:   policyDocument,
		IsDefaultVersion: setAsDefault,
		CreateDate:       time.Now().UTC(),
	}

	if setAsDefault {
		// Clear default flag on all existing versions.
		for i := range versions {
			versions[i].IsDefaultVersion = false
		}

		// Update the policy document, default version ID, and update timestamp.
		pol.PolicyDocument = policyDocument
		pol.DefaultVersionID = versionID
		pol.UpdateDate = newVersion.CreateDate
		b.policies.Put(pol)
	}

	versions = append(versions, newVersion)
	b.policyVersions[policyArn] = versions

	return &newVersion, nil
}

// ListPolicyVersions returns all stored versions for a managed policy, including v1.
func (b *InMemoryBackend) ListPolicyVersions(policyArn string) ([]StoredPolicyVersion, error) {
	b.mu.RLock("ListPolicyVersions")
	defer b.mu.RUnlock()

	policy, exists := b.getPolicyByARNLocked(policyArn)
	if !exists {
		return nil, fmt.Errorf("%w: policy %q not found", ErrPolicyNotFound, policyArn)
	}

	storedVersions := b.policyVersions[policyArn]
	isV1Default := true
	for _, storedVersion := range storedVersions {
		if storedVersion.IsDefaultVersion {
			isV1Default = false

			break
		}
	}

	versions := make([]StoredPolicyVersion, 0, len(storedVersions)+1)

	if !b.deletedV1Policies[policyArn] {
		versions = append(versions, StoredPolicyVersion{
			VersionID:        "v1",
			PolicyDocument:   policy.PolicyDocument,
			CreateDate:       policy.CreateDate,
			IsDefaultVersion: isV1Default,
		})
	}

	return append(versions, storedVersions...), nil
}
