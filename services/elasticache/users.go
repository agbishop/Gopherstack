package elasticache

import (
	"context"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

func (b *InMemoryBackend) userARN(region, userID string) string {
	return arn.Build("elasticache", region, b.accountID, "user:"+userID)
}

// userGroupIDsLocked returns the sorted IDs of every user group userID
// belongs to -- the reverse of UserGroup.UserIDs, computed fresh on every
// call rather than persisted on User (matches AWS: DescribeUsers/Create
// UserResult/ModifyUserResult all echo back UserGroupIds). Must hold at
// least b.mu.RLock.
func (b *InMemoryBackend) userGroupIDsLocked(region, userID string) []string {
	var ids []string
	for _, ug := range b.userGroupsStoreRO(region).All() {
		if slices.Contains(ug.UserIDs, userID) {
			ids = append(ids, ug.UserGroupID)
		}
	}
	sort.Strings(ids)

	return ids
}

// withUserGroupIDs returns a copy of u with UserGroupIDs populated. Must hold
// at least b.mu.RLock.
func (b *InMemoryBackend) withUserGroupIDs(region string, u *User) *User {
	result := *u
	result.UserGroupIDs = b.userGroupIDsLocked(region, u.UserID)

	return &result
}

// ----------------------------------------
// CreateCacheSecurityGroup
// ----------------------------------------

// CreateUserWithAuth creates a new ElastiCache user with the full
// authentication model reported on the wire: authType is one of the output
// values ("password", "no-password", "iam" -- see authType* constants) and
// passwordCount is the number of passwords on file (0 for iam/no-password,
// 1-2 for password auth; validated by the caller before this is invoked).
func (b *InMemoryBackend) CreateUserWithAuth(
	ctx context.Context,
	userID, userName, accessString, engine, authType string,
	passwordCount int,
) (*User, error) {
	b.mu.Lock("CreateUserWithAuth")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	tbl := b.usersStore(region)
	if _, exists := tbl.Get(userID); exists {
		return nil, ErrUserAlreadyExists
	}

	if engine == "" {
		engine = engineRedis
	}
	if authType == "" {
		authType = authTypeNoPasswordOutput
	}

	u := &User{
		UserID:        userID,
		UserName:      userName,
		Status:        statusActive,
		ARN:           b.userARN(region, userID),
		Engine:        engine,
		AccessString:  accessString,
		AuthType:      authType,
		PasswordCount: passwordCount,
		CreatedAt:     time.Now(),
		Tags:          tags.New("elasticache.user." + userID + ".tags"),
	}
	tbl.Put(u)
	b.appendEventLocked(userID, "user", "user created")

	return b.withUserGroupIDs(region, u), nil
}

// CreateUser creates a new ElastiCache user using the legacy
// NoPasswordRequired boolean (no explicit AuthenticationMode/Passwords).
// Kept for callers that only need password-vs-no-password, delegating to
// [InMemoryBackend.CreateUserWithAuth] for the actual insert.
func (b *InMemoryBackend) CreateUser(
	ctx context.Context,
	userID, userName, accessString, engine string,
	noPasswordRequired bool,
) (*User, error) {
	authType := authTypePassword

	passwordCount := 1
	if noPasswordRequired {
		authType = authTypeNoPasswordOutput
		passwordCount = 0
	}

	return b.CreateUserWithAuth(ctx, userID, userName, accessString, engine, authType, passwordCount)
}

// AddUserInternal seeds a user for testing.
func (b *InMemoryBackend) AddUserInternal(u *User) {
	b.mu.Lock("AddUserInternal")
	defer b.mu.Unlock()
	b.usersStore(b.region).Put(u)
}

// DeleteUser deletes a user by ID. Per api_op_DeleteUser.go's own doc ("The
// user will be removed from all user groups and in turn removed from all
// replication groups"), membership in a user group does NOT block the
// delete -- it cascades. Only DeleteUser's real modeled error set
// (DefaultUserAssociatedToUserGroup, InvalidParameterValue, InvalidUserState,
// ServiceLinkedRoleNotFoundFault, UserNotFound -- deserializers.go's
// awsAwsquery_deserializeOpErrorDeleteUser) has no generic "user still in a
// group" fault; DefaultUserAssociatedToUserGroup is scoped to the special
// "default" user, which this emulator does not model.
func (b *InMemoryBackend) DeleteUser(ctx context.Context, userID string) (*User, error) {
	b.mu.Lock("DeleteUser")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	tbl := b.usersStore(region)
	u, ok := tbl.Get(userID)
	if !ok {
		return nil, ErrUserNotFound
	}

	result := b.withUserGroupIDs(region, u)

	for _, ug := range b.userGroupsStore(region).All() {
		if idx := slices.Index(ug.UserIDs, userID); idx >= 0 {
			ug.UserIDs = slices.Delete(ug.UserIDs, idx, idx+1)
		}
	}

	tbl.Delete(userID)
	b.appendEventLocked(userID, "user", "user deleted")

	return result, nil
}

// DescribeUsers returns a paginated list of users, optionally filtered by userID.
// DescribeUsers filters by engine and, when the request carries a Filters
// entry named "UserId" (elasticache@v1.56.4 api_op_DescribeUsers.go -- the
// only documented Filters[].Name), by that filter's Values.
func (b *InMemoryBackend) DescribeUsers(
	ctx context.Context,
	userID, marker, engine string,
	maxRecords int,
	filterUserIDs []string,
) (page.Page[User], error) {
	b.mu.RLock("DescribeUsers")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	idFilter := make(map[string]bool, len(filterUserIDs))
	for _, id := range filterUserIDs {
		idFilter[id] = true
	}

	p, err := describePaged(b.usersStoreRO(region), userID, ErrUserNotFound,
		func(u User) bool {
			if engine != "" && u.Engine != engine {
				return false
			}
			if len(idFilter) > 0 && !idFilter[u.UserID] {
				return false
			}

			return true
		},
		func(u User) string { return u.UserID }, marker, maxRecords)
	if err != nil {
		return p, err
	}

	for i := range p.Data {
		p.Data[i].UserGroupIDs = b.userGroupIDsLocked(region, p.Data[i].UserID)
	}

	return p, nil
}

// ModifyUserWithAuth modifies a user's access string (overwrite or append),
// engine, and/or full authentication model. Empty/nil arguments leave the
// corresponding field unchanged; accessString and appendAccessString are
// mutually exclusive per AWS's ModifyUser contract (the caller is expected to
// only set one).
func (b *InMemoryBackend) ModifyUserWithAuth(
	ctx context.Context,
	userID, accessString, appendAccessString, engine, authType string,
	passwordCount *int,
) (*User, error) {
	b.mu.Lock("ModifyUserWithAuth")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	u, ok := b.usersStore(region).Get(userID)
	if !ok {
		return nil, ErrUserNotFound
	}

	switch {
	case accessString != "":
		u.AccessString = accessString
	case appendAccessString != "":
		if u.AccessString == "" {
			u.AccessString = appendAccessString
		} else {
			u.AccessString += " " + appendAccessString
		}
	}

	if engine != "" {
		u.Engine = engine
	}

	if authType != "" {
		u.AuthType = authType
	}

	if passwordCount != nil {
		u.PasswordCount = *passwordCount
	}

	return b.withUserGroupIDs(region, u), nil
}

// ModifyUser modifies a user's access string and/or password settings using
// the legacy NoPasswordRequired boolean. Kept for callers that only need
// password-vs-no-password, delegating to [InMemoryBackend.ModifyUserWithAuth].
func (b *InMemoryBackend) ModifyUser(
	ctx context.Context,
	userID, accessString string,
	noPasswordRequired bool,
) (*User, error) {
	authType := authTypePassword

	passwordCount := 1
	if noPasswordRequired {
		authType = authTypeNoPasswordOutput
		passwordCount = 0
	}

	return b.ModifyUserWithAuth(ctx, userID, accessString, "", "", authType, &passwordCount)
}
