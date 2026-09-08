package iam

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// AWS EntityType values accepted by GetAccountAuthorizationDetails' Filter parameter.
const (
	entityTypeUser               = "User"
	entityTypeGroup              = "Group"
	entityTypeRole               = "Role"
	entityTypeLocalManagedPolicy = "LocalManagedPolicy"
	entityTypeAWSManagedPolicy   = "AWSManagedPolicy"
)

// GetAccountAuthorizationDetails returns a page of IAM entities and their
// policies. filter restricts which entity categories are populated (AWS
// EntityType values: User, Role, Group, LocalManagedPolicy,
// AWSManagedPolicy); an empty filter includes every category. This mock
// tracks only customer-managed policies, so LocalManagedPolicy matches every
// policy and AWSManagedPolicy always yields none (no AWS-managed policy
// catalog is emulated). marker/maxItems paginate over the combined
// Users+Groups+Roles+Policies sequence in that order (matching the XML field
// order), mirroring AWS's single Marker/MaxItems pair spanning all four
// lists. Returns the next marker ("" when not truncated).
func (b *InMemoryBackend) GetAccountAuthorizationDetails(
	marker string, maxItems int, filter []string,
) (AccountAuthorizationDetails, string) {
	b.mu.RLock("GetAccountAuthorizationDetails")
	defer b.mu.RUnlock()

	includeUsers, includeGroups, includeRoles, includePolicies := authDetailsFilterSets(filter)

	users := b.buildUserDetails()
	groups := b.buildGroupDetails()
	roles := b.buildRoleDetails()
	policies := b.buildPolicyDetails()

	if !includeUsers {
		users = nil
	}

	if !includeGroups {
		groups = nil
	}

	if !includeRoles {
		roles = nil
	}

	if !includePolicies {
		policies = nil
	}

	if maxItems <= 0 {
		maxItems = iamDefaultMaxItems
	}

	users, groups, roles, policies, nextMarker := paginateAuthDetails(
		users, groups, roles, policies, page.DecodeToken(marker), maxItems,
	)

	return AccountAuthorizationDetails{
		Users:    users,
		Groups:   groups,
		Roles:    roles,
		Policies: policies,
	}, nextMarker
}

// buildUserGroupMap builds the reverse group-membership map: userName → []groupName.
// Caller must hold b.mu.
func (b *InMemoryBackend) buildUserGroupMap() map[string][]string {
	userGroupMap := make(map[string][]string, b.users.Len())
	for groupName, members := range b.groupMembers {
		for _, member := range members {
			userGroupMap[member] = append(userGroupMap[member], groupName)
		}
	}

	return userGroupMap
}

// buildRoleInstanceProfiles builds the reverse instance-profile map:
// roleName → []InstanceProfile, mirroring ListInstanceProfilesForRole (same
// real backend now feeds both), so RoleDetail.InstanceProfiles is populated
// instead of always empty. Caller must hold b.mu.
func (b *InMemoryBackend) buildRoleInstanceProfiles() map[string][]InstanceProfile {
	roleInstanceProfiles := make(map[string][]InstanceProfile)
	for _, ip := range b.instanceProfiles.All() {
		for _, roleName := range ip.Roles {
			roleInstanceProfiles[roleName] = append(roleInstanceProfiles[roleName], *ip)
		}
	}

	for roleName, profiles := range roleInstanceProfiles {
		sort.Slice(profiles, func(i, j int) bool {
			return profiles[i].InstanceProfileName < profiles[j].InstanceProfileName
		})
		roleInstanceProfiles[roleName] = profiles
	}

	return roleInstanceProfiles
}

// buildUserDetails builds the sorted UserDetail list. Caller must hold b.mu.
func (b *InMemoryBackend) buildUserDetails() []UserDetail {
	userGroupMap := b.buildUserGroupMap()

	users := make([]UserDetail, 0, b.users.Len())
	for _, u := range b.users.All() {
		user := *u
		attached := attachedFromARNs(b.userPolicies[u.UserName])
		inline := inlineEntries(b.userInlinePolicies[u.UserName])
		groupNames := userGroupMap[u.UserName]
		sort.Strings(groupNames)
		users = append(
			users,
			UserDetail{User: user, AttachedPolicies: attached, InlinePolicies: inline, GroupNames: groupNames},
		)
	}

	sort.Slice(users, func(i, j int) bool { return users[i].UserName < users[j].UserName })

	return users
}

// buildGroupDetails builds the sorted GroupDetail list. Caller must hold b.mu.
func (b *InMemoryBackend) buildGroupDetails() []GroupDetail {
	groups := make([]GroupDetail, 0, b.groups.Len())
	for _, g := range b.groups.All() {
		group := *g
		attached := attachedFromARNs(b.groupPolicies[g.GroupName])
		inline := inlineEntries(b.groupInlinePolicies[g.GroupName])
		groups = append(
			groups,
			GroupDetail{Group: group, AttachedPolicies: attached, InlinePolicies: inline},
		)
	}

	sort.Slice(groups, func(i, j int) bool { return groups[i].GroupName < groups[j].GroupName })

	return groups
}

// buildRoleDetails builds the sorted RoleDetail list. Caller must hold b.mu.
func (b *InMemoryBackend) buildRoleDetails() []RoleDetail {
	roleInstanceProfiles := b.buildRoleInstanceProfiles()

	roles := make([]RoleDetail, 0, b.roles.Len())
	for _, r := range b.roles.All() {
		role := *r
		attached := attachedFromARNs(b.rolePolicies[r.RoleName])
		inline := inlineEntries(b.roleInlinePolicies[r.RoleName])
		profiles := roleInstanceProfiles[r.RoleName]
		roles = append(
			roles,
			RoleDetail{Role: role, AttachedPolicies: attached, InlinePolicies: inline, InstanceProfiles: profiles},
		)
	}

	sort.Slice(roles, func(i, j int) bool { return roles[i].RoleName < roles[j].RoleName })

	return roles
}

// buildPolicyDetails builds the sorted managed-policy list. Caller must hold b.mu.
func (b *InMemoryBackend) buildPolicyDetails() []Policy {
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

// authDetailsFilterSets translates GetAccountAuthorizationDetails' Filter
// (AWS EntityType values) into per-category inclusion flags. An empty filter
// includes every category, matching AWS's "no filter means everything" default.
func authDetailsFilterSets(filter []string) (bool, bool, bool, bool) {
	if len(filter) == 0 {
		return true, true, true, true
	}

	var users, groups, roles, policies bool

	for _, f := range filter {
		switch f {
		case entityTypeUser:
			users = true
		case entityTypeGroup:
			groups = true
		case entityTypeRole:
			roles = true
		case entityTypeLocalManagedPolicy:
			policies = true
		case entityTypeAWSManagedPolicy:
			// No AWS-managed policy catalog is emulated; matches nothing.
		}
	}

	return users, groups, roles, policies
}

// paginateAuthDetails slices the four already-filtered detail lists as one
// combined virtual sequence (Users, Groups, Roles, Policies, in that order --
// matching AWS's single Marker/MaxItems pair spanning all four
// GetAccountAuthorizationDetails lists). Returns the next marker ("" once the
// combined sequence is exhausted).
func paginateAuthDetails(
	users []UserDetail, groups []GroupDetail, roles []RoleDetail, policies []Policy,
	start, limit int,
) ([]UserDetail, []GroupDetail, []RoleDetail, []Policy, string) {
	total := len(users) + len(groups) + len(roles) + len(policies)
	if start >= total {
		return nil, nil, nil, nil, ""
	}

	end := start + limit
	truncated := end < total

	if !truncated {
		end = total
	}

	outUsers := windowSlice(users, 0, start, end)
	outGroups := windowSlice(groups, len(users), start, end)
	outRoles := windowSlice(roles, len(users)+len(groups), start, end)
	outPolicies := windowSlice(policies, len(users)+len(groups)+len(roles), start, end)

	if !truncated {
		return outUsers, outGroups, outRoles, outPolicies, ""
	}

	return outUsers, outGroups, outRoles, outPolicies, page.EncodeToken(end)
}

// windowSlice returns the portion of items -- whose combined-sequence offset
// range starts at base -- that overlaps [start, end).
func windowSlice[T any](items []T, base, start, end int) []T {
	lo := max(start-base, 0)
	hi := min(end-base, len(items))

	if lo >= hi {
		return nil
	}

	return items[lo:hi]
}

// attachedFromARNs converts a slice of policy ARNs to AttachedPolicy entries.
func attachedFromARNs(arns []string) []AttachedPolicy {
	result := make([]AttachedPolicy, 0, len(arns))

	for _, a := range arns {
		result = append(result, AttachedPolicy{PolicyName: policyNameFromARN(a), PolicyArn: a})
	}

	return result
}

// inlineEntries converts a policyName→document map to sorted InlinePolicyEntry slices.
func inlineEntries(m map[string]string) []InlinePolicyEntry {
	result := make([]InlinePolicyEntry, 0, len(m))

	for name, doc := range m {
		result = append(result, InlinePolicyEntry{PolicyName: name, PolicyDocument: doc})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].PolicyName < result[j].PolicyName })

	return result
}

// SimulatePrincipalPolicy evaluates a set of actions against a set of resources
// for the given principal ARN, returning a result per action×resource pair.
//
// Supported principal ARN formats:
//   - arn:aws:iam::<account>:user/<name>
//   - arn:aws:iam::<account>:role/<name>
//
// Permission boundaries narrow only the identity-policy result (effective
// identity permissions = identity policies ∩ boundary) before it is combined
// with any resource-based policy, mirroring the live enforcement path in
// middleware.go. A resource-policy Allow can still return "allowed" even
// when the boundary does not cover the action, per the AWS IAM User Guide.
// An explicit Deny from either the boundary, identity policy, or resource
// policy always wins.
func (b *InMemoryBackend) SimulatePrincipalPolicy(
	principalArn, callerArn, resourceOwner string,
	resourcePolicyList, actionNames, resourceArns []string, ctx ConditionContext,
) ([]SimulationResult, error) {
	b.mu.RLock("SimulatePrincipalPolicy")
	defer b.mu.RUnlock()

	namedPolicies, err := b.collectNamedPrincipalPolicies(principalArn)
	if err != nil {
		return nil, err
	}

	// Collect permission boundary document (if any).
	boundaryDoc := b.collectBoundaryDoc(principalArn)
	hasBoundary := boundaryDoc != ""

	if len(resourceArns) == 0 {
		resourceArns = []string{"*"}
	}

	// Build plain docs slice for combined evaluation.
	docs := make([]string, 0, len(namedPolicies))
	for _, np := range namedPolicies {
		docs = append(docs, np.Doc)
	}

	results := make([]SimulationResult, 0, len(actionNames)*len(resourceArns))

	principalAccount := parseAccountFromArn(principalArn)
	resourceAccount := resourceOwner
	if resourceAccount == "" {
		if callerArn != "" {
			resourceAccount = parseAccountFromArn(callerArn)
		} else {
			resourceAccount = principalAccount
		}
	}

	isCrossAccount := resourceAccount != principalAccount && resourceAccount != "" && principalAccount != ""

	for _, action := range actionNames {
		for _, resource := range resourceArns {
			results = append(results, b.evaluateSingleSimulation(
				action, resource, docs, resourcePolicyList,
				ctx, hasBoundary, boundaryDoc, namedPolicies, isCrossAccount,
			))
		}
	}

	return results, nil
}

func (b *InMemoryBackend) evaluateSingleSimulation(
	action, resource string,
	docs, resourcePolicyList []string,
	ctx ConditionContext,
	hasBoundary bool, boundaryDoc string,
	namedPolicies []namedPolicyDoc,
	isCrossAccount bool,
) SimulationResult {
	// Identity Policies evaluation, boundary-limited the same way the live
	// enforcement path does (applyPermissionsBoundary in middleware.go):
	// the boundary narrows only the identity-policy result, before it is
	// combined with any resource-based policy below. Per the AWS IAM User
	// Guide, a resource-based policy Allow for an IAM user is not limited by
	// an implicit deny in an identity-based policy or permissions boundary.
	idResult := EvaluatePolicies(docs, action, resource, ctx)

	var boundaryDocs []string
	if boundaryDoc != "" {
		boundaryDocs = []string{boundaryDoc}
	}

	idResult, boundaryExplicitDeny, _ := applyPermissionsBoundary(boundaryDocs, action, resource, ctx, idResult)

	// Resource Policies evaluation
	var resDocs []string
	resDocs = append(resDocs, resourcePolicyList...)

	// Auto-inject role trust policy for sts:AssumeRole
	if action == "sts:AssumeRole" {
		if r, errGet := b.GetRoleByArn(resource); errGet == nil && r.AssumeRolePolicyDocument != "" {
			resDocs = append(resDocs, r.AssumeRolePolicyDocument)
		}
	}

	resResult := EvalImplicitDeny
	if len(resDocs) > 0 {
		resResult = EvaluatePolicies(resDocs, action, resource, ctx)
	}

	// Combine logic (Intra-account vs Cross-account)
	evalResult := combineSimulationResults(idResult, resResult, isCrossAccount)
	if boundaryExplicitDeny {
		evalResult = EvalExplicitDeny
	}

	// Per-policy detail map.
	detail := make(map[string]string, len(namedPolicies))
	for _, np := range namedPolicies {
		r := EvaluatePolicies([]string{np.Doc}, action, resource, ctx)
		detail[np.SourceID] = evalDecisionStr(r)
	}

	var allowedByBoundary *bool

	if hasBoundary {
		allowed := EvaluatePolicies([]string{boundaryDoc}, action, resource, ctx) == EvalAllow
		allowedByBoundary = &allowed
	}

	return SimulationResult{
		ActionName:                   action,
		ResourceName:                 resource,
		Decision:                     evalDecisionStr(evalResult),
		EvalDecisionDetails:          detail,
		AllowedByPermissionsBoundary: allowedByBoundary,
	}
}

func combineSimulationResults(idResult, resResult EvaluationResult, isCrossAccount bool) EvaluationResult {
	if idResult == EvalExplicitDeny || resResult == EvalExplicitDeny {
		return EvalExplicitDeny
	}

	if isCrossAccount {
		if idResult == EvalAllow && resResult == EvalAllow {
			return EvalAllow
		}

		return EvalImplicitDeny
	}

	if idResult == EvalAllow || resResult == EvalAllow {
		return EvalAllow
	}

	return EvalImplicitDeny
}

func parseAccountFromArn(arnStr string) string {
	const minArnParts = 5
	const arnAccountIndex = 4

	parts := strings.Split(arnStr, ":")
	if len(parts) >= minArnParts {
		return parts[arnAccountIndex]
	}

	return ""
}

// evalDecisionStr converts an EvalResult to the AWS-compatible decision string.
func evalDecisionStr(r EvaluationResult) string {
	switch r {
	case EvalAllow:
		return "allowed"
	case EvalExplicitDeny:
		return "explicitDeny"
	default:
		return "implicitDeny"
	}
}

// collectBoundaryDoc returns the policy document for the principal's permission boundary, or "".
// Caller must hold b.mu read-locked.
func (b *InMemoryBackend) collectBoundaryDoc(principalArn string) string {
	const (
		userPrefix = ":user/"
		rolePrefix = ":role/"
	)

	switch {
	case strings.Contains(principalArn, userPrefix):
		idx := strings.LastIndex(principalArn, userPrefix)

		return b.boundaryDocForUser(principalArn[idx+len(userPrefix):])
	case strings.Contains(principalArn, rolePrefix):
		idx := strings.LastIndex(principalArn, rolePrefix)

		return b.boundaryDocForRole(principalArn[idx+len(rolePrefix):])
	}

	return ""
}

// collectNamedPrincipalPolicies returns named policy documents for the given principal ARN.
// Each entry contains the policy source ID (ARN for managed, name for inline) and document.
// Caller must hold b.mu read-locked.
func (b *InMemoryBackend) collectNamedPrincipalPolicies(
	principalArn string,
) ([]namedPolicyDoc, error) {
	const (
		userPrefix = ":user/"
		rolePrefix = ":role/"
	)

	switch {
	case strings.Contains(principalArn, userPrefix):
		idx := strings.LastIndex(principalArn, userPrefix)
		userName := principalArn[idx+len(userPrefix):]

		if _, exists := b.users.Get(userName); !exists {
			return nil, fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
		}

		named := b.collectNamedEntityPolicies(
			b.userPolicies[userName],
			b.userInlinePolicies[userName],
		)

		// Add group-inherited policies.
		for groupName, members := range b.groupMembers {
			if !slices.Contains(members, userName) {
				continue
			}

			named = append(
				named,
				b.collectNamedEntityPolicies(
					b.groupPolicies[groupName],
					b.groupInlinePolicies[groupName],
				)...)
		}

		return named, nil

	case strings.Contains(principalArn, rolePrefix):
		idx := strings.LastIndex(principalArn, rolePrefix)
		roleName := principalArn[idx+len(rolePrefix):]

		if _, exists := b.roles.Get(roleName); !exists {
			return nil, fmt.Errorf("%w: role %q not found", ErrRoleNotFound, roleName)
		}

		return b.collectNamedEntityPolicies(
			b.rolePolicies[roleName],
			b.roleInlinePolicies[roleName],
		), nil

	default:
		return nil, fmt.Errorf(
			"%w: unsupported principal ARN format %q",
			ErrUserNotFound,
			principalArn,
		)
	}
}

// collectNamedEntityPolicies collects named policy docs from attached ARNs and inline policies.
// Uses policyByARN for O(1) ARN-to-name resolution instead of O(n) map scan.
// Caller must hold b.mu read-locked.
func (b *InMemoryBackend) collectNamedEntityPolicies(
	attachedARNs []string, inlinePols map[string]string,
) []namedPolicyDoc {
	var named []namedPolicyDoc

	for _, policyArn := range attachedARNs {
		polName, ok := b.policyByARN[policyArn]
		if !ok {
			continue
		}

		p, ok := b.policies.Get(polName)
		if ok && p.PolicyDocument != "" {
			named = append(named, namedPolicyDoc{SourceID: p.Arn, Doc: p.PolicyDocument})
		}
	}

	for name, doc := range inlinePols {
		if doc != "" {
			named = append(named, namedPolicyDoc{SourceID: name, Doc: doc})
		}
	}

	return named
}
