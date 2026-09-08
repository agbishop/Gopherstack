package iam

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/pkgs/collections"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// CreateUser creates a new IAM user.
func (b *InMemoryBackend) CreateUser(userName, path, permissionsBoundary string) (*User, error) {
	b.mu.Lock("CreateUser")
	defer b.mu.Unlock()

	if _, exists := b.users.Get(userName); exists {
		return nil, fmt.Errorf("%w: user %q already exists", ErrUserAlreadyExists, userName)
	}

	p := normPath(path)
	u := User{
		UserName:            userName,
		UserID:              newID("AIDA"),
		Arn:                 arn.Build("iam", "", b.accountID, "user"+p+userName),
		Path:                p,
		CreateDate:          time.Now().UTC(),
		PermissionsBoundary: permissionsBoundary,
	}
	b.users.Put(&u)
	b.sortedUserNames = insertSorted(b.sortedUserNames, userName)

	return &u, nil
}

// userComprehensiveDepsLocked reports whether userName has an SSH public key
// and/or a linked MFA device, per the comprehensiveBackend state. Split out of
// DeleteUser to keep it below the cognitive-complexity threshold. Caller must
// hold b.mu (comprehensiveBackend's fields are guarded by the same lock).
func (b *InMemoryBackend) userComprehensiveDepsLocked(userName string) (bool, bool) {
	c := b.comp()

	hasSSHKey := false

	for _, k := range c.sshPublicKeys {
		if k.UserName == userName {
			hasSSHKey = true

			break
		}
	}

	hasMFADevice := false

	for _, linkedUser := range c.mfaUserLinks {
		if linkedUser == userName {
			hasMFADevice = true

			break
		}
	}

	return hasSSHKey, hasMFADevice
}

// deleteUserConflictLocked returns the DeleteConflict error for the first
// dependent of userName found, in the order AWS documents for DeleteUser, or
// nil if the user has none. Must be called with b.mu held. Split out of
// DeleteUser to keep it below the cognitive-complexity threshold.
func (b *InMemoryBackend) deleteUserConflictLocked(userName string, hasSSHKey, hasMFADevice bool) error {
	if _, exists := b.loginProfiles.Get(userName); exists {
		return fmt.Errorf("%w: user %q has a login profile (password)", ErrDeleteConflict, userName)
	}

	if len(b.userAccessKeys[userName]) > 0 {
		return fmt.Errorf("%w: user %q has access keys", ErrDeleteConflict, userName)
	}

	for _, cert := range b.signingCertificates.All() {
		if cert.UserName == userName {
			return fmt.Errorf("%w: user %q has a signing certificate", ErrDeleteConflict, userName)
		}
	}

	if hasSSHKey {
		return fmt.Errorf("%w: user %q has an SSH public key", ErrDeleteConflict, userName)
	}

	for _, cred := range b.serviceSpecificCreds.All() {
		if cred.UserName == userName {
			return fmt.Errorf("%w: user %q has a service-specific credential", ErrDeleteConflict, userName)
		}
	}

	if hasMFADevice {
		return fmt.Errorf("%w: user %q has an MFA device", ErrDeleteConflict, userName)
	}

	if len(b.userInlinePolicies[userName]) > 0 {
		return fmt.Errorf("%w: user %q has inline policies", ErrDeleteConflict, userName)
	}

	if len(b.userPolicies[userName]) > 0 {
		return fmt.Errorf("%w: user %q has attached policies", ErrDeleteConflict, userName)
	}

	for _, members := range b.groupMembers {
		if slices.Contains(members, userName) {
			return fmt.Errorf("%w: user %q is a member of a group", ErrDeleteConflict, userName)
		}
	}

	return nil
}

// DeleteUser deletes an IAM user by name.
//
// Matching real AWS: DeleteUser does NOT cascade-remove any of the user's
// dependents. The caller must first remove the login profile (password),
// access keys, signing certificate, SSH public key, service-specific
// credentials, MFA device, inline policies, attached managed policies, and
// group memberships — otherwise the request is rejected with
// DeleteConflictException. See
// https://docs.aws.amazon.com/IAM/latest/APIReference/API_DeleteUser.html.
func (b *InMemoryBackend) DeleteUser(userName string) error {
	b.mu.Lock("DeleteUser")
	defer b.mu.Unlock()

	if _, exists := b.users.Get(userName); !exists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	hasSSHKey, hasMFADevice := b.userComprehensiveDepsLocked(userName)
	if err := b.deleteUserConflictLocked(userName, hasSSHKey, hasMFADevice); err != nil {
		return err
	}

	b.users.Delete(userName)
	b.sortedUserNames = deleteSorted(b.sortedUserNames, userName)

	return nil
}

// ListUsers returns a paginated list of IAM users sorted by name.
func (b *InMemoryBackend) ListUsers(marker string, maxItems int) (page.Page[User], error) {
	b.mu.RLock("ListUsers")
	defer b.mu.RUnlock()

	return pageFromSortedNames(
		b.sortedUserNames,
		b.users.Get,
		marker,
		maxItems,
		iamDefaultMaxItems,
	), nil
}

// GetUser retrieves a single IAM user by name.
func (b *InMemoryBackend) GetUser(userName string) (*User, error) {
	b.mu.RLock("GetUser")
	defer b.mu.RUnlock()

	u, exists := b.users.Get(userName)
	if !exists {
		return nil, fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	return u, nil
}

// ListAllUsers returns all users (for dashboard).
func (b *InMemoryBackend) ListAllUsers() []User {
	b.mu.RLock("ListAllUsers")
	defer b.mu.RUnlock()

	return sortedUsers(b.users)
}

// ListAttachedUserPolicies returns all policy ARNs attached to the named user.
func (b *InMemoryBackend) ListAttachedUserPolicies(userName string) ([]AttachedPolicy, error) {
	b.mu.RLock("ListAttachedUserPolicies")
	defer b.mu.RUnlock()

	if _, exists := b.users.Get(userName); !exists {
		return nil, fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	arns := b.userPolicies[userName]
	result := make([]AttachedPolicy, 0, len(arns))

	for _, arn := range arns {
		name := policyNameFromARN(arn)
		result = append(result, AttachedPolicy{PolicyName: name, PolicyArn: arn})
	}

	return result, nil
}

// GetUserByAccessKeyID returns the User associated with the given access key ID.
// Returns ErrAccessKeyNotFound if no key with that ID exists.
func (b *InMemoryBackend) GetUserByAccessKeyID(accessKeyID string) (*User, error) {
	b.mu.RLock("GetUserByAccessKeyID")
	defer b.mu.RUnlock()

	ak, exists := b.accessKeys.Get(accessKeyID)
	if !exists {
		return nil, fmt.Errorf("%w: access key %q not found", ErrAccessKeyNotFound, accessKeyID)
	}

	u, exists := b.users.Get(ak.UserName)
	if !exists {
		return nil, fmt.Errorf("%w: user %q not found for access key", ErrUserNotFound, ak.UserName)
	}

	return u, nil
}

// ResolvePrincipal resolves an access key ID to an awsmeta.Principal representing an IAM User.
func (b *InMemoryBackend) ResolvePrincipal(
	_ context.Context,
	accessKeyID, _ string,
) (*awsmeta.Principal, bool) {
	u, err := b.GetUserByAccessKeyID(accessKeyID)
	if err != nil || u == nil {
		return nil, false
	}

	return &awsmeta.Principal{
		Kind:      awsmeta.PrincipalKindUser,
		Arn:       u.Arn,
		UserName:  u.UserName,
		AccountID: b.accountID,
		UserID:    u.UserID,
	}, true
}

// GetPoliciesForUser returns the policy documents for all policies that apply to the
// named user: its own managed and inline policies, plus managed and inline policies
// inherited from every group the user belongs to. Policies that are referenced but not
// found in the backend are silently skipped.
func (b *InMemoryBackend) GetPoliciesForUser(userName string) ([]string, error) {
	b.mu.RLock("GetPoliciesForUser")
	defer b.mu.RUnlock()

	if _, exists := b.users.Get(userName); !exists {
		return nil, fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	arns := b.userPolicies[userName]
	inline := b.userInlinePolicies[userName]
	docs := make([]string, 0, len(arns)+len(inline))

	docs = b.appendManagedPolicyDocsLocked(docs, arns)
	docs = appendInlinePolicyDocs(docs, inline)

	for groupName, members := range b.groupMembers {
		if !slices.Contains(members, userName) {
			continue
		}

		docs = b.appendManagedPolicyDocsLocked(docs, b.groupPolicies[groupName])
		docs = appendInlinePolicyDocs(docs, b.groupInlinePolicies[groupName])
	}

	return docs, nil
}

// appendManagedPolicyDocsLocked appends the policy documents for each managed
// policy ARN to docs, skipping ARNs the backend no longer has a document for.
// Caller must hold b.mu (read-locked is sufficient).
func (b *InMemoryBackend) appendManagedPolicyDocsLocked(docs []string, arns []string) []string {
	for _, policyArn := range arns {
		policy, exists := b.getPolicyByARNLocked(policyArn)
		if !exists || policy.PolicyDocument == "" {
			continue
		}

		docs = append(docs, policy.PolicyDocument)
	}

	return docs
}

// appendInlinePolicyDocs appends each non-empty inline policy document to docs.
func appendInlinePolicyDocs(docs []string, inline map[string]string) []string {
	for _, doc := range inline {
		if doc != "" {
			docs = append(docs, doc)
		}
	}

	return docs
}

// PutUserPolicy creates or replaces an inline policy on a user.
func (b *InMemoryBackend) PutUserPolicy(userName, policyName, policyDocument string) error {
	b.mu.Lock("PutUserPolicy")
	defer b.mu.Unlock()

	if _, exists := b.users.Get(userName); !exists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	if err := validateIdentityPolicyDocument(policyDocument); err != nil {
		return err
	}

	if len(policyDocument) > maxUserPolicySize {
		return fmt.Errorf("%w: inline policy for user %q exceeds %d bytes",
			ErrLimitExceeded, userName, maxUserPolicySize)
	}

	if b.userInlinePolicies[userName] == nil {
		b.userInlinePolicies[userName] = make(map[string]string)
	}

	b.userInlinePolicies[userName][policyName] = policyDocument

	return nil
}

// GetUserPolicy retrieves an inline policy document from a user.
func (b *InMemoryBackend) GetUserPolicy(userName, policyName string) (string, error) {
	b.mu.RLock("GetUserPolicy")
	defer b.mu.RUnlock()

	if _, exists := b.users.Get(userName); !exists {
		return "", fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	doc, exists := b.userInlinePolicies[userName][policyName]
	if !exists {
		return "", fmt.Errorf(
			"%w: inline policy %q not found on user %q",
			ErrInlinePolicyNotFound,
			policyName,
			userName,
		)
	}

	return doc, nil
}

// DeleteUserPolicy removes an inline policy from a user.
func (b *InMemoryBackend) DeleteUserPolicy(userName, policyName string) error {
	b.mu.Lock("DeleteUserPolicy")
	defer b.mu.Unlock()

	if _, exists := b.users.Get(userName); !exists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	if _, exists := b.userInlinePolicies[userName][policyName]; !exists {
		return fmt.Errorf(
			"%w: inline policy %q not found on user %q",
			ErrInlinePolicyNotFound,
			policyName,
			userName,
		)
	}

	delete(b.userInlinePolicies[userName], policyName)

	return nil
}

// ListUserPolicies returns sorted inline policy names for a user.
func (b *InMemoryBackend) ListUserPolicies(userName string) ([]string, error) {
	b.mu.RLock("ListUserPolicies")
	defer b.mu.RUnlock()

	if _, exists := b.users.Get(userName); !exists {
		return nil, fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	names := collections.SortedKeys(b.userInlinePolicies[userName])

	return names, nil
}

// PutUserPermissionsBoundary sets the permissions boundary on a user.
func (b *InMemoryBackend) PutUserPermissionsBoundary(userName, policyArn string) error {
	b.mu.Lock("PutUserPermissionsBoundary")
	defer b.mu.Unlock()

	u, exists := b.users.Get(userName)
	if !exists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	u.PermissionsBoundary = policyArn
	b.users.Put(u)

	return nil
}

// DeleteUserPermissionsBoundary clears the permissions boundary on a user.
func (b *InMemoryBackend) DeleteUserPermissionsBoundary(userName string) error {
	b.mu.Lock("DeleteUserPermissionsBoundary")
	defer b.mu.Unlock()

	u, exists := b.users.Get(userName)
	if !exists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	u.PermissionsBoundary = ""
	b.users.Put(u)

	return nil
}

// purgeUsersLocked removes users created before cutoff and cleans up associated data.
// Caller must hold b.mu.
func (b *InMemoryBackend) purgeUsersLocked(cutoff time.Time) {
	for _, u := range b.users.All() {
		if !u.CreateDate.Before(cutoff) {
			continue
		}
		name := u.UserName
		b.users.Delete(name)
		b.loginProfiles.Delete(name)
		delete(b.userPolicies, name)
		delete(b.userInlinePolicies, name)
		b.removeUserFromGroupsLocked(name)
	}
}

// removeUserFromGroupsLocked removes a user from all group membership lists.
// Caller must hold b.mu.
func (b *InMemoryBackend) removeUserFromGroupsLocked(userName string) {
	for g, members := range b.groupMembers {
		for i, m := range members {
			if m == userName {
				b.groupMembers[g] = append(members[:i], members[i+1:]...)

				break
			}
		}
	}
}

// UpdateUser renames a user and/or updates their path.
// If newUserName is non-empty the user is renamed; if newPath is non-empty the path is updated.
func (b *InMemoryBackend) UpdateUser(userName, newPath, newUserName string) error {
	b.mu.Lock("UpdateUser")
	defer b.mu.Unlock()

	u, ok := b.users.Get(userName)
	if !ok {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	if newUserName != "" && newUserName != userName {
		if _, taken := b.users.Get(newUserName); taken {
			return fmt.Errorf("%w: user %q already exists", ErrUserAlreadyExists, newUserName)
		}
	}

	if newPath != "" {
		u.Path = normPath(newPath)
	}

	if newUserName != "" && newUserName != userName {
		b.migrateUserData(userName, newUserName)
		u.UserName = newUserName
		b.users.Delete(userName)
	}

	b.users.Put(u)

	return nil
}

// migrateUserData moves per-user maps to the new name (called with lock held).
func (b *InMemoryBackend) migrateUserData(oldName, newName string) {
	for _, ak := range b.accessKeys.All() {
		if ak.UserName == oldName {
			ak.UserName = newName
			b.accessKeys.Put(ak)
		}
	}

	// loginProfiles is keyed by UserName (see loginProfilesKeyFn in
	// store_setup.go), so the rename must update the value's own UserName
	// field before re-inserting -- store.Table always derives the key from
	// the value, unlike the raw map this replaces which allowed the map key
	// and the value's UserName field to diverge after a rename.
	if lp, found := b.loginProfiles.Get(oldName); found {
		b.loginProfiles.Delete(oldName)
		lp.UserName = newName
		b.loginProfiles.Put(lp)
	}

	if policies, found := b.userPolicies[oldName]; found {
		b.userPolicies[newName] = policies
		delete(b.userPolicies, oldName)
		// Keep the reverse policyAttachments index (used by DeletePolicy's
		// conflict check, ListEntitiesForPolicy, and DetachUserPolicy) in
		// sync so it doesn't keep pointing at the pre-rename user name.
		b.renamePolicyAttachmentsLocked("user", oldName, newName, policies)
	}

	if inline, found := b.userInlinePolicies[oldName]; found {
		b.userInlinePolicies[newName] = inline
		delete(b.userInlinePolicies, oldName)
	}

	for groupName, members := range b.groupMembers {
		for i, m := range members {
			if m == oldName {
				b.groupMembers[groupName][i] = newName
			}
		}
	}
}

// TagUser merges the given key-value pairs into the user's Tags field.
func (b *InMemoryBackend) TagUser(userName string, tags map[string]string) error {
	b.mu.Lock("TagUser")
	defer b.mu.Unlock()

	u, exists := b.users.Get(userName)
	if !exists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	if u.Tags == nil {
		u.Tags = make(map[string]string, len(tags))
	}

	maps.Copy(u.Tags, tags)
	b.users.Put(u)

	return nil
}

// UntagUser removes the given keys from the user's Tags field.
func (b *InMemoryBackend) UntagUser(userName string, keys []string) error {
	b.mu.Lock("UntagUser")
	defer b.mu.Unlock()

	u, exists := b.users.Get(userName)
	if !exists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	for _, k := range keys {
		delete(u.Tags, k)
	}

	b.users.Put(u)

	return nil
}
