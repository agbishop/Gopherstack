package quicksight

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// ---- Users ----

func (b *InMemoryBackend) RegisterUser(
	accountID, namespace, userName, email, role, identityType, sessionName string,
	tags map[string]string,
) (*User, error) {
	if userName == "" || email == "" {
		return nil, ErrValidation
	}

	b.mu.Lock("RegisterUser")
	defer b.mu.Unlock()

	if !b.namespaces.Has(nsKey(accountID, namespace)) {
		return nil, ErrNamespaceNotFound
	}

	key := userKey(accountID, namespace, userName)
	if b.users.Has(key) {
		return nil, ErrUserAlreadyExists
	}

	if role == "" {
		role = "READER"
	}
	if identityType == "" {
		identityType = identityStoreQuickSight
	}

	u := &storedUser{
		UserName:     userName,
		Arn:          arn.Build("quicksight", b.region, accountID, fmt.Sprintf("user/%s/%s", namespace, userName)),
		Email:        email,
		Role:         role,
		IdentityType: identityType,
		Namespace:    namespace,
		PrincipalID:  uuid.New().String(),
		SessionName:  sessionName,
		Active:       true,
	}
	b.users.Put(u)

	if len(tags) > 0 {
		b.tags[u.Arn] = maps.Clone(tags)
	}

	return u.toUser(), nil
}

func (b *InMemoryBackend) DescribeUser(accountID, namespace, userName string) (*User, error) {
	b.mu.RLock("DescribeUser")
	defer b.mu.RUnlock()

	u, ok := b.users.Get(userKey(accountID, namespace, userName))
	if !ok {
		return nil, ErrUserNotFound
	}

	out := u.toUser()
	out.CustomPermissionsName = b.userCustomPermissions[userCustomPermissionKey(accountID, namespace, userName)]

	return out, nil
}

func (b *InMemoryBackend) UpdateUser(accountID, namespace, userName, email, role string) (*User, error) {
	b.mu.Lock("UpdateUser")
	defer b.mu.Unlock()

	key := userKey(accountID, namespace, userName)
	u, ok := b.users.Get(key)
	if !ok {
		return nil, ErrUserNotFound
	}

	if email != "" {
		u.Email = email
	}
	if role != "" {
		u.Role = role
	}

	return u.toUser(), nil
}

func (b *InMemoryBackend) DeleteUser(accountID, namespace, userName string) error {
	b.mu.Lock("DeleteUser")
	defer b.mu.Unlock()

	key := userKey(accountID, namespace, userName)
	if !b.users.Delete(key) {
		return ErrUserNotFound
	}

	b.removeUserFromAllGroups(accountID, namespace, userName)
	delete(b.userCustomPermissions, userCustomPermissionKey(accountID, namespace, userName))

	return nil
}

func (b *InMemoryBackend) DeleteUserByPrincipalID(accountID, namespace, principalID string) error {
	b.mu.Lock("DeleteUserByPrincipalID")
	defer b.mu.Unlock()

	for _, u := range b.users.All() {
		if u.Namespace == namespace && u.PrincipalID == principalID {
			b.users.Delete(userKey(accountID, namespace, u.UserName))
			b.removeUserFromAllGroups(accountID, namespace, u.UserName)
			delete(b.userCustomPermissions, userCustomPermissionKey(accountID, namespace, u.UserName))

			return nil
		}
	}

	return ErrUserNotFound
}

// removeUserFromAllGroups deletes every group-membership entry for userName
// in namespace. Called on user deletion so DescribeGroupMembership,
// ListGroupMemberships, and ListUserGroups don't keep surfacing a deleted
// user as a live group member. Caller must hold b.mu.
func (b *InMemoryBackend) removeUserFromAllGroups(accountID, namespace, userName string) {
	suffix := "/" + userName
	prefix := accountID + "/" + namespace + "/"
	for key := range b.groupMembers {
		if strings.HasPrefix(key, prefix) && strings.HasSuffix(key, suffix) {
			delete(b.groupMembers, key)
		}
	}
}

func (b *InMemoryBackend) ListUsers(
	accountID, namespace string,
	maxResults int32,
	nextToken string,
) ([]*User, string, error) {
	b.mu.RLock("ListUsers")
	defer b.mu.RUnlock()

	var all []*storedUser
	for _, u := range b.users.All() {
		if u.Namespace == namespace {
			all = append(all, u)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].UserName < all[j].UserName })

	if maxResults <= 0 || maxResults > defaultMaxResults {
		maxResults = defaultMaxResults
	}

	start := 0
	if nextToken != "" {
		start = len(all)
		for i, u := range all {
			if u.UserName == nextToken {
				start = i

				break
			}
		}
	}

	end := start + int(maxResults)
	var next string
	if end < len(all) {
		next = all[end].UserName
	} else {
		end = len(all)
	}

	result := make([]*User, 0, end-start)
	for _, u := range all[start:end] {
		out := u.toUser()
		out.CustomPermissionsName = b.userCustomPermissions[userCustomPermissionKey(accountID, namespace, u.UserName)]
		result = append(result, out)
	}

	return result, next, nil
}

func (b *InMemoryBackend) ListUserGroups(
	accountID, namespace, userName string,
	maxResults int32,
	nextToken string,
) ([]*Group, string, error) {
	b.mu.RLock("ListUserGroups")
	defer b.mu.RUnlock()

	if !b.users.Has(userKey(accountID, namespace, userName)) {
		return nil, "", ErrUserNotFound
	}

	var all []*storedGroup
	for _, g := range b.groups.All() {
		if g.Namespace != namespace {
			continue
		}
		memberKey := groupMemberKey(accountID, namespace, g.GroupName, userName)
		if b.groupMembers[memberKey] {
			all = append(all, g)
		}
	}

	result, next := paginateGroups(all, maxResults, nextToken)

	return result, next, nil
}
