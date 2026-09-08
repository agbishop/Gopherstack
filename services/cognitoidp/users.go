package cognitoidp

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// AdminCreateUser creates a new user in the pool with FORCE_CHANGE_PASSWORD status.
func (b *InMemoryBackend) AdminCreateUser(
	userPoolID, username, tempPassword string,
	userAttributes map[string]string,
) (*User, error) {
	b.mu.Lock("AdminCreateUser")
	defer b.mu.Unlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	if _, exists := b.users.Get(userKey(userPoolID, username)); exists {
		return nil, fmt.Errorf("%w: user %q already exists", ErrUsernameExists, username)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	return b.buildAndStoreUserLocked(userPoolID, username, tempPassword, string(hash), userAttributes)
}

// AdminSetUserPassword sets the password for a user in a pool.
func (b *InMemoryBackend) AdminSetUserPassword(userPoolID, username, password string, permanent bool) error {
	b.mu.Lock("AdminSetUserPassword")
	defer b.mu.Unlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	user, ok := b.users.Get(userKey(userPoolID, username))
	if !ok {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, username)
	}

	// AWS enforces the pool's password policy on AdminSetUserPassword, just as
	// it does on ConfirmForgotPassword. An invalid password is rejected with
	// InvalidPasswordException.
	if err := validatePassword(pool.PasswordPolicy, password); err != nil {
		return err
	}

	hash, saltHex, verifierHex, err := hashAndSRP(userPoolID, username, password)
	if err != nil {
		return err
	}

	user.PasswordHash = hash
	user.SRPSalt = saltHex
	user.SRPVerifier = verifierHex
	user.UpdatedAt = time.Now()

	if permanent {
		user.Status = UserStatusConfirmed
	}

	return nil
}

// AdminGetUser returns a user from a pool by username.
func (b *InMemoryBackend) AdminGetUser(userPoolID, username string) (*User, error) {
	b.mu.RLock("AdminGetUser")
	defer b.mu.RUnlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	user, ok := b.users.Get(userKey(userPoolID, username))
	if !ok {
		return nil, fmt.Errorf("%w: user %q not found", ErrUserNotFound, username)
	}

	cp := *user

	return &cp, nil
}

// AdminDeleteUser deletes a user from a pool by username.
func (b *InMemoryBackend) AdminDeleteUser(userPoolID, username string) error {
	b.mu.Lock("AdminDeleteUser")
	defer b.mu.Unlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	if _, ok := b.users.Get(userKey(userPoolID, username)); !ok {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, username)
	}

	b.deleteUserStateLocked(userPoolID, username)

	return nil
}

// deleteUserStateLocked removes the user record for poolID:username and every
// piece of per-user state that would otherwise outlive it: refresh tokens,
// devices, auth events, WebAuthn credentials, and group memberships. Shared
// by AdminDeleteUser, DeleteUser, and DeleteUserPool's cascade so a cleanup
// added to one path can't drift from the others -- DeleteUserPool's cascade
// was already fixed once to repeat this list by hand and missed groupMembers
// and webauthnCredentials in the repeat (gopherstack-tq5q/-ljak). Caller must
// hold b.mu in write mode.
func (b *InMemoryBackend) deleteUserStateLocked(poolID, username string) {
	b.users.Delete(userKey(poolID, username))
	b.deleteRefreshTokensForUserLocked(poolID, username)

	key := userStateKey(poolID, username)
	delete(b.devices, key)
	delete(b.authEvents, key)
	delete(b.webauthnCredentials, key)

	for _, members := range b.groupMembers[poolID] {
		delete(members, username)
	}
}

// ListUsers returns all users in a pool sorted by username.
func (b *InMemoryBackend) ListUsers(userPoolID string) ([]*User, error) {
	b.mu.RLock("ListUsers")
	defer b.mu.RUnlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	poolUsers := b.usersByPool.Get(userPoolID)
	out := make([]*User, 0, len(poolUsers))

	for _, u := range poolUsers {
		cp := *u
		cp.Attributes = maps.Clone(u.Attributes)
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })

	return out, nil
}

// GetUser returns user attributes for an authenticated user (via access token).
func (b *InMemoryBackend) GetUser(accessToken string) (*User, error) {
	b.mu.RLock("GetUser")
	defer b.mu.RUnlock()

	u, err := b.findUserByAccessTokenLocked(accessToken)
	if err != nil {
		return nil, err
	}

	cp := *u
	cp.Attributes = maps.Clone(u.Attributes)

	return &cp, nil
}

// AdminDisableUser disables a user account in a pool.
func (b *InMemoryBackend) AdminDisableUser(userPoolID, username string) error {
	b.mu.Lock("AdminDisableUser")
	defer b.mu.Unlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	u, ok := b.users.Get(userKey(userPoolID, username))
	if !ok {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, username)
	}

	u.Enabled = false

	return nil
}

// AdminEnableUser re-enables a previously disabled user account in a pool.
func (b *InMemoryBackend) AdminEnableUser(userPoolID, username string) error {
	b.mu.Lock("AdminEnableUser")
	defer b.mu.Unlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	u, ok := b.users.Get(userKey(userPoolID, username))
	if !ok {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, username)
	}

	u.Enabled = true

	return nil
}

// ValidatePoolUser validates that a pool and a user within it both exist. It is used by
// operations that have nothing to mutate but must still reject unknown pools/users with
// the AWS-accurate error shape.
func (b *InMemoryBackend) ValidatePoolUser(userPoolID, username string) error {
	b.mu.RLock("ValidatePoolUser")
	defer b.mu.RUnlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	if _, ok := b.users.Get(userKey(userPoolID, username)); !ok {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, username)
	}

	return nil
}

// DeleteUser deletes the currently authenticated user (self-service).
func (b *InMemoryBackend) DeleteUser(accessToken string) error {
	b.mu.Lock("DeleteUser")
	defer b.mu.Unlock()

	u, err := b.findUserByAccessTokenLocked(accessToken)
	if err != nil {
		return err
	}

	b.deleteUserStateLocked(u.UserPoolID, u.Username)

	return nil
}

// ListUsersFiltered returns users matching an optional AWS-style filter string.
// Supported filter form: "username = \"prefix*\"" or "username ^= \"prefix\"".
// If filter is empty all users are returned (same as ListUsers).
func (b *InMemoryBackend) ListUsersFiltered(userPoolID, filter string) ([]*User, error) {
	b.mu.RLock("ListUsersFiltered")
	defer b.mu.RUnlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	poolUsers := b.usersByPool.Get(userPoolID)
	prefix, attrFilter := parseListUsersFilter(filter)

	out := make([]*User, 0, len(poolUsers))

	for _, u := range poolUsers {
		if !userMatchesFilter(u, prefix, attrFilter) {
			continue
		}

		cp := *u
		cp.Attributes = maps.Clone(u.Attributes)
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })

	return out, nil
}

// parseListUsersFilter parses a simplified Cognito filter expression.
// Returns a username prefix (may be "") and an optional attribute=value pair.
func parseListUsersFilter(filter string) (string, [2]string) {
	if filter == "" {
		return "", [2]string{}
	}

	// Trim whitespace and quotes.
	f := strings.TrimSpace(filter)

	// Common form: `username ^= "prefix"` or `username = "value"`
	for _, sep := range []string{" ^= ", " = "} {
		before, after, ok := strings.Cut(f, sep)
		if !ok {
			continue
		}

		attr := strings.TrimSpace(before)
		val := strings.Trim(strings.TrimSpace(after), `"`)

		if attr == "username" {
			val = strings.TrimSuffix(val, "*")

			return val, [2]string{}
		}

		return "", [2]string{attr, val}
	}

	return "", [2]string{}
}

// userMatchesFilter returns true if the user satisfies the filter criteria.
// AttributeName cognito:user_status, status, and sub are not stored in
// u.Attributes (they're dedicated User fields), so they need their own cases;
// AWS documents all three as searchable ListUsers attributes alongside the
// generic standard/custom ones (api_op_ListUsers.go).
func userMatchesFilter(u *User, usernamePrefix string, attrFilter [2]string) bool {
	if usernamePrefix != "" && !strings.HasPrefix(u.Username, usernamePrefix) {
		return false
	}

	switch attrFilter[0] {
	case "":
		return true
	case "cognito:user_status":
		return strings.EqualFold(u.Status, attrFilter[1])
	case "status":
		return u.Enabled == (attrFilter[1] == "Enabled")
	case "sub":
		return u.Sub == attrFilter[1]
	default:
		attrVal, exists := u.Attributes[attrFilter[0]]

		return exists && attrVal == attrFilter[1]
	}
}

// AddUserInternal seeds a user directly into the backend, bypassing normal sign-up.
// Intended for use in tests only. The pool must already exist.
func (b *InMemoryBackend) AddUserInternal(user *User) {
	b.mu.Lock("AddUserInternal")
	defer b.mu.Unlock()

	b.users.Put(user)
}

// Valid values for UserPoolClient.PreventUserExistenceErrors.
const (
	preventUserExistenceEnabled = "ENABLED"
	preventUserExistenceLegacy  = "LEGACY"
)

// normalizePreventUserExistenceErrors validates and defaults the PreventUserExistenceErrors
// app client setting to AWS's documented values. An unset value defaults to "LEGACY" (the
// AWS default for app clients that don't specify it); anything other than "ENABLED" or
// "LEGACY" is an InvalidParameterException.
func normalizePreventUserExistenceErrors(v string) (string, error) {
	switch v {
	case "":
		return preventUserExistenceLegacy, nil
	case preventUserExistenceEnabled, preventUserExistenceLegacy:
		return v, nil
	default:
		return "", fmt.Errorf(
			"%w: PreventUserExistenceErrors must be ENABLED or LEGACY, got %q",
			ErrInvalidParameter, v,
		)
	}
}

// applyPreventUserExistenceErrorsUpdate updates client's PreventUserExistenceErrors when
// value is non-empty (an UpdateUserPoolClient caller omitting the field leaves the existing
// setting untouched, matching the update-only-what-was-sent semantics of the other opts
// fields in UpdateUserPoolClientWithOpts).
func applyPreventUserExistenceErrorsUpdate(client *UserPoolClient, value string) error {
	if value == "" {
		return nil
	}

	normalized, err := normalizePreventUserExistenceErrors(value)
	if err != nil {
		return err
	}

	client.PreventUserExistenceErrors = normalized

	return nil
}

// AdminCreateUserWithPolicy creates a new user in the pool with FORCE_CHANGE_PASSWORD status,
// enforcing the pool's PasswordPolicy on the temporary password.
func (b *InMemoryBackend) AdminCreateUserWithPolicy(
	userPoolID, username, tempPassword string,
	userAttributes map[string]string,
) (*User, error) {
	b.mu.Lock("AdminCreateUserWithPolicy")
	defer b.mu.Unlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	if _, exists := b.users.Get(userKey(userPoolID, username)); exists {
		return nil, fmt.Errorf("%w: user %q already exists", ErrUsernameExists, username)
	}

	if tempPassword != "" {
		if err := validatePassword(pool.PasswordPolicy, tempPassword); err != nil {
			return nil, err
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	return b.buildAndStoreUserLocked(userPoolID, username, tempPassword, string(hash), userAttributes)
}

func (b *InMemoryBackend) buildAndStoreUserLocked(
	userPoolID, username, tempPassword, hash string,
	userAttributes map[string]string,
) (*User, error) {
	attrs := make(map[string]string, len(userAttributes))
	maps.Copy(attrs, userAttributes)

	if tempPassword != "" {
		attrs["custom:temporaryPassword"] = tempPassword
	}

	saltHex, verifierHex, err := computeSRPVerifier(userPoolID, username, tempPassword)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	user := &User{
		Sub:                  uuid.New().String(),
		Username:             username,
		UserPoolID:           userPoolID,
		PasswordHash:         hash,
		SRPSalt:              saltHex,
		SRPVerifier:          verifierHex,
		Status:               UserStatusForceChangePassword,
		Attributes:           attrs,
		CreatedAt:            now,
		UpdatedAt:            now,
		TempPasswordIssuedAt: now,
		Enabled:              true,
	}

	b.users.Put(user)
	cp := *user

	return &cp, nil
}

// defaultTempPasswordSuffix is appended to ensure temp password meets basic complexity.
const defaultTempPasswordSuffix = "Aa1!"

// defaultTempPasswordPrefixLen is the length of the random prefix in generated temp passwords.
const defaultTempPasswordPrefixLen = 12

// AdminCreateUserFull creates a user with optional MessageAction (SUPPRESS/RESEND) and delivery mediums.
func (b *InMemoryBackend) AdminCreateUserFull(
	userPoolID, username, tempPassword string,
	userAttributes map[string]string,
	messageAction string,
	desiredDeliveryMediums []string,
	forceAliasCreation bool,
) (*User, error) {
	b.mu.Lock("AdminCreateUserFull")
	defer b.mu.Unlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	if existing, exists := b.users.Get(userKey(userPoolID, username)); exists {
		if messageAction == "RESEND" {
			// Re-send temp password to existing FORCE_CHANGE_PASSWORD user.
			if existing.Status != UserStatusForceChangePassword {
				return nil, fmt.Errorf(
					"%w: user %q is not in FORCE_CHANGE_PASSWORD state",
					ErrInvalidParameter,
					username,
				)
			}

			cp := *existing

			return &cp, nil
		}

		return nil, fmt.Errorf("%w: user %q already exists", ErrUsernameExists, username)
	}

	if tempPassword != "" {
		if err := validatePassword(pool.PasswordPolicy, tempPassword); err != nil {
			return nil, err
		}
	} else {
		// Generate a temporary password.
		tempPassword = randomAlphanumeric(defaultTempPasswordPrefixLen) + defaultTempPasswordSuffix
	}

	importHash, srpSaltHex, srpVerifierHex, err := hashAndSRP(userPoolID, username, tempPassword)
	if err != nil {
		return nil, err
	}

	attrs := make(map[string]string, len(userAttributes))
	maps.Copy(attrs, userAttributes)

	if messageAction != "SUPPRESS" && tempPassword != "" {
		attrs["custom:temporaryPassword"] = tempPassword
	}

	if verifyErr := b.applyAdminCreateUserAutoVerifyLocked(pool, username, attrs); verifyErr != nil {
		return nil, verifyErr
	}

	_ = desiredDeliveryMediums
	_ = forceAliasCreation

	user := &User{
		Sub:          uuid.New().String(),
		Username:     username,
		UserPoolID:   userPoolID,
		PasswordHash: importHash,
		SRPSalt:      srpSaltHex,
		SRPVerifier:  srpVerifierHex,
		Status:       UserStatusForceChangePassword,
		Attributes:   attrs,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Enabled:      true,
	}

	b.users.Put(user)

	cp := *user

	return &cp, nil
}

// applyAdminCreateUserAutoVerifyLocked auto-verifies attributes configured on the pool,
// then invokes PreSignUp (whose autoVerifyEmail/autoVerifyPhone response can also
// request verification) and applies its answer, mutating attrs in place. PreSignUp also
// fires for AdminCreateUser: unlike the self-service SignUp flow, autoConfirmUser has no
// meaningful target state here (admin-created users start in FORCE_CHANGE_PASSWORD, not
// UNCONFIRMED, and are never in the SignUp confirmation flow), so only
// autoVerifyEmail/autoVerifyPhone are applied. Caller must hold b.mu.
func (b *InMemoryBackend) applyAdminCreateUserAutoVerifyLocked(
	pool *UserPool, username string, attrs map[string]string,
) error {
	for _, attr := range pool.AutoVerifiedAttributes {
		if _, hasAttr := attrs[attr]; hasAttr {
			attrs[attr+"_verified"] = attrVerifiedTrue
		}
	}

	preSignUpResp, err := b.invokeLambdaTrigger(
		pool, triggerKeyPreSignUp, triggerSourcePreSignUpAdminCreateUser, "", username,
		map[string]any{
			eventKeyUserAttributes: stringMapToAny(attrs),
			eventKeyValidationData: map[string]any{},
			eventKeyClientMetadata: map[string]any{},
		},
		map[string]any{"autoConfirmUser": false, "autoVerifyEmail": false, "autoVerifyPhone": false},
	)
	if err != nil {
		return err
	}

	_, lambdaAutoVerifyEmail, lambdaAutoVerifyPhone := parsePreSignUpResponse(preSignUpResp)
	if lambdaAutoVerifyEmail {
		attrs[attrEmail+"_verified"] = attrVerifiedTrue
	}

	if lambdaAutoVerifyPhone {
		attrs["phone_number_verified"] = attrVerifiedTrue
	}

	return nil
}

// AdminSetUserPasswordFull sets password with proper status handling.
// If permanent=true, status becomes CONFIRMED. If permanent=false, status remains FORCE_CHANGE_PASSWORD.
func (b *InMemoryBackend) AdminSetUserPasswordFull(userPoolID, username, password string, permanent bool) error {
	b.mu.Lock("AdminSetUserPasswordFull")
	defer b.mu.Unlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	if err := validatePassword(pool.PasswordPolicy, password); err != nil {
		return err
	}

	user, ok := b.users.Get(userKey(userPoolID, username))
	if !ok {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, username)
	}

	hash, saltHex, verifierHex, err := hashAndSRP(userPoolID, username, password)
	if err != nil {
		return err
	}

	user.PasswordHash = hash
	user.SRPSalt = saltHex
	user.SRPVerifier = verifierHex
	user.UpdatedAt = time.Now()

	if permanent {
		user.Status = UserStatusConfirmed
	} else {
		user.Status = UserStatusForceChangePassword
		user.TempPasswordIssuedAt = user.UpdatedAt
		user.Attributes["custom:temporaryPassword"] = password
	}

	return nil
}

// userStateKey builds the composite key used by the per-user device, WebAuthn
// credential, and auth-event stores.
func userStateKey(userPoolID, username string) string {
	return userPoolID + ":" + username
}

// Auth factor values (AuthFactorType) recognized by GetUserAuthFactors /
// AdminGetUserAuthFactors.
const (
	authFactorPassword      = "PASSWORD"
	authFactorSMSOTP        = "SMS_OTP"
	authFactorWebAuthn      = "WEB_AUTHN"
	authFactorSoftwareToken = "SOFTWARE_TOKEN"
)

// commonAuthFactorSetLocked computes the PASSWORD/SMS_OTP/WEB_AUTHN/
// SOFTWARE_TOKEN factors shared by GetUserAuthFactors and
// AdminGetUserAuthFactors, derived from the user's real password/MFA/WebAuthn
// state. SOFTWARE_TOKEN comes from user.TOTPVerified (the user completed
// AssociateSoftwareToken + VerifySoftwareToken) or "SOFTWARE_TOKEN_MFA" being
// present in user.UserMFASettingList (enabled as an active MFA method).
// Caller must hold at least a read lock.
func (b *InMemoryBackend) commonAuthFactorSetLocked(user *User) map[string]struct{} {
	factorSet := map[string]struct{}{}

	if user.PasswordHash != "" {
		factorSet[authFactorPassword] = struct{}{}
	}

	if slices.Contains(user.UserMFASettingList, "SMS_MFA") {
		factorSet[authFactorSMSOTP] = struct{}{}
	}

	for _, opt := range user.MFAOptions {
		if opt.DeliveryMedium == "SMS" {
			factorSet[authFactorSMSOTP] = struct{}{}
		}
	}

	if len(b.webauthnCredentials[userStateKey(user.UserPoolID, user.Username)]) > 0 {
		factorSet[authFactorWebAuthn] = struct{}{}
	}

	if user.TOTPVerified || slices.Contains(user.UserMFASettingList, "SOFTWARE_TOKEN_MFA") {
		factorSet[authFactorSoftwareToken] = struct{}{}
	}

	return factorSet
}

// GetUserAuthFactors returns the authenticated user and the sign-in factors
// currently configured for their account, derived from stored MFA/WebAuthn state.
func (b *InMemoryBackend) GetUserAuthFactors(accessToken string) (*User, []string, error) {
	b.mu.RLock("GetUserAuthFactors")
	defer b.mu.RUnlock()

	user, err := b.findUserByAccessTokenLocked(accessToken)
	if err != nil {
		return nil, nil, err
	}

	factorSet := b.commonAuthFactorSetLocked(user)

	factors := make([]string, 0, len(factorSet))
	for f := range factorSet {
		factors = append(factors, f)
	}

	sort.Strings(factors)

	cp := *user

	return &cp, factors, nil
}

// AdminGetUserAuthFactors returns the authentication factors currently
// configured for a user, derived from the same real password/MFA/WebAuthn/
// SOFTWARE_TOKEN state as GetUserAuthFactors (commonAuthFactorSetLocked).
// AdminGetUserAuthFactorsOutput additionally carries PreferredMfaSetting and
// UserMFASettingList verbatim from the user record (fields the self-service
// GetUserAuthFactorsOutput does not have).
func (b *InMemoryBackend) AdminGetUserAuthFactors(userPoolID, username string) (*User, []string, error) {
	b.mu.RLock("AdminGetUserAuthFactors")
	defer b.mu.RUnlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return nil, nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	user, ok := b.users.Get(userKey(userPoolID, username))
	if !ok {
		return nil, nil, fmt.Errorf("%w: user %q not found", ErrUserNotFound, username)
	}

	factorSet := b.commonAuthFactorSetLocked(user)

	factors := make([]string, 0, len(factorSet))
	for f := range factorSet {
		factors = append(factors, f)
	}

	sort.Strings(factors)

	cp := *user

	return &cp, factors, nil
}
