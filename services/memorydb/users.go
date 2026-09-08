package memorydb

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// normalizeAuthType validates req.AuthenticationMode.Type (raw as sent on the
// wire) and normalizes it to its canonical stored/output form. Real AWS's
// request-side InputAuthenticationType enum only allows "password" and "iam"
// (Type omitted means no password required); this backend additionally
// accepts "no-password" and the "no-password-required" alias as explicit
// input, normalizing all of them to authTypeNoPassword ("no-password") --
// the only value real AWS's Authentication.Type output enum defines for a
// passwordless user. Callers must never store/emit the raw input string.
func normalizeAuthType(raw string) (string, error) {
	authType := strings.ToLower(raw)
	if authType == "" || authType == authTypeNoPasswordRequired {
		authType = authTypeNoPassword
	}

	if authType != authTypePassword && authType != authTypeIAM && authType != authTypeNoPassword {
		return "", fmt.Errorf(
			"AuthenticationMode.Type must be password, iam, or no-password-required: %w",
			ErrValidation,
		)
	}

	return authType, nil
}

// validateAuthPasswordCombo rejects passwords supplied alongside iam auth,
// matching real AWS's CreateUser/UpdateUser validation.
func validateAuthPasswordCombo(authType string, passwords []string) error {
	if authType == authTypeIAM && len(passwords) > 0 {
		return fmt.Errorf("passwords cannot be set when AuthenticationMode.Type is iam: %w", ErrValidation)
	}

	return nil
}

// applyUserAuthModeUpdate validates and applies an UpdateUser AuthenticationMode
// request onto u, normalizing/validating Type exactly like CreateUser.
func applyUserAuthModeUpdate(u *User, mode *authenticationModeReq) error {
	if mode.Type != "" {
		authType, err := normalizeAuthType(mode.Type)
		if err != nil {
			return err
		}

		if err = validateAuthPasswordCombo(authType, mode.Passwords); err != nil {
			return err
		}

		u.AuthType = authType
	}

	if len(mode.Passwords) > 0 {
		u.Passwords = mode.Passwords
	}

	return nil
}

// CreateUser creates a new MemoryDB user.
func (b *InMemoryBackend) CreateUser(ctx context.Context, req *createUserRequest) (*User, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	if err := validateResourceName(req.UserName, "user"); err != nil {
		return nil, err
	}

	if _, exists := b.usersStore(region).Get(req.UserName); exists {
		return nil, ErrUserAlreadyExists
	}

	authType, err := normalizeAuthType(req.AuthenticationMode.Type)
	if err != nil {
		return nil, err
	}

	if err = validateAuthPasswordCombo(authType, req.AuthenticationMode.Passwords); err != nil {
		return nil, err
	}

	userARN := arn.Build("memorydb", region, b.accountID, "user/"+req.UserName)

	u := &User{
		Name:         req.UserName,
		ARN:          userARN,
		AccessString: req.AccessString,
		Status:       userStatusActive,
		AuthType:     authType,
		Passwords:    req.AuthenticationMode.Passwords,
		Tags:         tagsFromSlice(req.Tags),
		CreatedAt:    time.Now(),
	}

	b.usersStore(region).Put(u)
	b.arnToResourceStore(region)[userARN] = resourceRef{Kind: resourceKindUser, Name: req.UserName}

	b.appendEventLocked(region, &Event{
		Date:       time.Now(),
		SourceName: req.UserName,
		SourceType: "user",
		Message:    "User " + req.UserName + " created",
	})

	return cloneUser(u), nil
}

// DescribeUsers returns users, optionally filtered by name.
func (b *InMemoryBackend) DescribeUsers(ctx context.Context, name string) ([]*User, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	t := b.users[region]

	if name != "" {
		u, ok := tableGet(t, name)
		if !ok {
			return nil, ErrUserNotFound
		}

		return []*User{cloneUser(u)}, nil
	}

	all := tableAll(t)
	result := make([]*User, 0, len(all))
	for _, u := range all {
		result = append(result, cloneUser(u))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// DeleteUser removes a user.
func (b *InMemoryBackend) DeleteUser(ctx context.Context, name string) (*User, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	u, ok := b.usersStore(region).Get(name)
	if !ok {
		return nil, ErrUserNotFound
	}

	// DeleteUser cascades: "The user will be removed from all ACLs and in
	// turn removed from all clusters" (api_op_DeleteUser.go doc comment).
	// Unlike DeleteACL, membership is not a blocker here.
	for _, a := range tableAll(b.acls[region]) {
		if idx := slices.Index(a.UserNames, name); idx != -1 {
			a.UserNames = slices.Delete(a.UserNames, idx, idx+1)
		}
	}

	b.usersStore(region).Delete(name)
	delete(b.arnToResourceStore(region), u.ARN)

	return u, nil
}

// UpdateUser modifies an existing user.
func (b *InMemoryBackend) UpdateUser(ctx context.Context, req *updateUserRequest) (*User, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	u, ok := b.usersStore(region).Get(req.UserName)
	if !ok {
		return nil, ErrUserNotFound
	}

	if req.AccessString != "" {
		u.AccessString = req.AccessString
	}

	if req.AuthenticationMode != nil {
		if err := applyUserAuthModeUpdate(u, req.AuthenticationMode); err != nil {
			return nil, err
		}
	}

	return cloneUser(u), nil
}

// -- ParameterGroup operations ---------------------------------------------------

// cloneUser returns a shallow copy of the user with separate tag and password slices.
func cloneUser(u *User) *User {
	if u == nil {
		return nil
	}

	cp := *u
	cp.Tags = maps.Clone(u.Tags)
	cp.Passwords = append([]string(nil), u.Passwords...)

	return &cp
}

// AddUserInternal inserts a user directly into the backend for testing.
func (b *InMemoryBackend) AddUserInternal(name, accessString string) *User {
	b.mu.Lock()
	defer b.mu.Unlock()

	userARN := arn.Build("memorydb", b.defaultRegion, b.accountID, "user/"+name)
	u := &User{
		Name:         name,
		ARN:          userARN,
		AccessString: accessString,
		Status:       userStatusActive,
		Tags:         make(map[string]string),
		CreatedAt:    time.Now(),
	}
	b.usersStore(b.defaultRegion).Put(u)
	b.arnToResourceStore(b.defaultRegion)[userARN] = resourceRef{Kind: resourceKindUser, Name: name}

	return u
}
