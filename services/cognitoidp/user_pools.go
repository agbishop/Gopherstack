package cognitoidp

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// deletionProtectionActive is the UserPool.DeletionProtection value that makes
// DeleteUserPool refuse to delete the pool.
const deletionProtectionActive = "ACTIVE"

// CreateUserPool creates a new user pool with the given name.
func (b *InMemoryBackend) CreateUserPool(name string) (*UserPool, error) {
	b.mu.Lock("CreateUserPool")
	defer b.mu.Unlock()

	poolID := b.region + "_" + randomAlphanumeric(poolIDSuffixLen)
	issuerURL := fmt.Sprintf("%s/%s", b.endpoint, poolID)

	issuer, err := newTokenIssuer(issuerURL)
	if err != nil {
		return nil, fmt.Errorf("creating token issuer: %w", err)
	}

	pool := &UserPool{
		ID:        poolID,
		Name:      name,
		ARN:       arn.Build("cognito-idp", b.region, b.accountID, fmt.Sprintf("userpool/%s", poolID)),
		CreatedAt: time.Now(),
		issuer:    issuer,
	}

	b.pools.Put(pool)

	cp := *pool

	return &cp, nil
}

// DescribeUserPool returns the user pool with the given ID.
func (b *InMemoryBackend) DescribeUserPool(userPoolID string) (*UserPool, error) {
	b.mu.RLock("DescribeUserPool")
	defer b.mu.RUnlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	cp := *pool

	return &cp, nil
}

// DeleteUserPool removes the user pool with the given ID and all of its associated clients.
func (b *InMemoryBackend) DeleteUserPool(userPoolID string) error {
	b.mu.Lock("DeleteUserPool")
	defer b.mu.Unlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	// AWS refuses to delete a user pool with deletion protection ACTIVE; the caller must
	// first UpdateUserPool to set DeletionProtection back to INACTIVE.
	if pool.DeletionProtection == deletionProtectionActive {
		return fmt.Errorf(
			"%w: User pool cannot be deleted because deletion protection is activated, "+
				"set deletion protection to INACTIVE and retry to delete the user pool",
			ErrInvalidParameter,
		)
	}

	// Real AWS refuses DeleteUserPool outright when a domain is still attached
	// (InvalidParameterException: "User pool cannot be deleted. It has a domain
	// configured that should be deleted first.") rather than cascading the
	// domain delete -- confirmed via the AWS API's own documented error
	// response (github.com/hashicorp/terraform-provider-aws#16479). The caller
	// must DeleteUserPoolDomain first.
	if b.findPoolDomainLocked(userPoolID) != nil {
		return fmt.Errorf(
			"%w: User pool cannot be deleted, it has a domain configured that should be deleted first",
			ErrInvalidParameter,
		)
	}

	b.pools.Delete(userPoolID)
	delete(b.resourceTags, pool.ARN)

	// Copy the index result before deleting: Delete mutates the byPool index's
	// backing slice in place, so ranging directly over it while deleting would
	// skip entries.
	for _, u := range slices.Clone(b.usersByPool.Get(userPoolID)) {
		b.deleteUserStateLocked(userPoolID, u.Username)
	}

	for _, client := range slices.Clone(b.clientsByPool.Get(userPoolID)) {
		b.clients.Delete(client.ClientID)
		b.deleteRefreshTokensForClientAndUserIndexLocked(client.ClientID)
	}

	// Clean up groups and group memberships for this pool.
	for _, g := range slices.Clone(b.groupsByPool.Get(userPoolID)) {
		b.groups.Delete(groupKey(userPoolID, g.GroupName))
	}

	delete(b.groupMembers, userPoolID)

	return nil
}

// ListUserPools returns all user pools sorted by name, tiebroken by ID.
// PoolName is not unique -- CreateUserPool has no "already exists" exception
// (real AWS Cognito allows multiple pools with the same name), so a Name-only
// sort admits ties; handleListUserPools' marker-based pagination (which
// resumes by pool ID) needs the complete order, not just the marker, to be
// reproducible across calls.
func (b *InMemoryBackend) ListUserPools() []*UserPool {
	b.mu.RLock("ListUserPools")
	defer b.mu.RUnlock()

	pools := b.pools.All()
	out := make([]*UserPool, 0, len(pools))

	for _, p := range pools {
		cp := *p
		out = append(out, &cp)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}

		return out[i].ID < out[j].ID
	})

	return out
}

// SetUserPoolMfaConfig sets the MFA configuration for a user pool.
func (b *InMemoryBackend) SetUserPoolMfaConfig(userPoolID, mfaConfig string) error {
	b.mu.Lock("SetUserPoolMfaConfig")
	defer b.mu.Unlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	pool.MfaConfiguration = mfaConfig

	return nil
}

// UpdateUserPool updates mutable properties of an existing user pool.
func (b *InMemoryBackend) UpdateUserPool(userPoolID, mfaConfiguration string) error {
	b.mu.Lock("UpdateUserPool")
	defer b.mu.Unlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	if mfaConfiguration != "" {
		pool.MfaConfiguration = mfaConfiguration
	}

	pool.UpdatedAt = time.Now()

	return nil
}

// AddUserPoolInternal seeds a user pool directly into the backend, bypassing normal
// creation logic. Intended for use in tests only.
func (b *InMemoryBackend) AddUserPoolInternal(pool *UserPool) {
	b.mu.Lock("AddUserPoolInternal")
	defer b.mu.Unlock()

	b.pools.Put(pool)
}

// GetPoolMetrics returns aggregate statistics for a user pool.
func (b *InMemoryBackend) GetPoolMetrics(userPoolID string) (*PoolMetrics, error) {
	b.mu.RLock("GetPoolMetrics")
	defer b.mu.RUnlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	tokenCount := 0
	now := time.Now().UTC()
	for _, entry := range b.refreshTokens {
		if entry.PoolID == userPoolID && (entry.ExpiresAt.IsZero() || entry.ExpiresAt.After(now)) {
			tokenCount++
		}
	}

	return &PoolMetrics{
		UserCount:        len(b.usersByPool.Get(userPoolID)),
		ClientCount:      len(b.clientsByPool.Get(userPoolID)),
		GroupCount:       len(b.groupsByPool.Get(userPoolID)),
		ActiveTokenCount: tokenCount,
	}, nil
}

// validatePasswordPolicy checks AWS-imposed bounds on a PasswordPolicy at
// pool create / update time. AWS rejects MinimumLength outside [6, 99] and
// TemporaryPasswordValidityDays outside [0, 365] with InvalidParameterException.
func validatePasswordPolicy(pp *PasswordPolicy) error {
	if pp == nil {
		return nil
	}

	const (
		minPasswordLength   = 6
		maxPasswordLength   = 99
		maxTempPwdValidDays = 365
	)

	if pp.MinimumLength != 0 && (pp.MinimumLength < minPasswordLength || pp.MinimumLength > maxPasswordLength) {
		return fmt.Errorf("%w: MinimumLength must be in [%d, %d]",
			ErrInvalidParameter, minPasswordLength, maxPasswordLength)
	}

	if pp.TemporaryPasswordValidityDays < 0 || pp.TemporaryPasswordValidityDays > maxTempPwdValidDays {
		return fmt.Errorf("%w: TemporaryPasswordValidityDays must be in [0, %d]",
			ErrInvalidParameter, maxTempPwdValidDays)
	}

	return nil
}

// validatePassword checks the proposed password against the pool's PasswordPolicy.
// Returns ErrInvalidPassword if the policy is violated.
func validatePassword(policy *PasswordPolicy, password string) error {
	if policy == nil {
		return nil
	}

	minLen := policy.MinimumLength
	if minLen <= 0 {
		minLen = 8
	}

	if len(password) < minLen {
		return fmt.Errorf("%w: password must be at least %d characters", ErrInvalidPassword, minLen)
	}

	if policy.RequireUppercase && !strings.ContainsFunc(password, unicode.IsUpper) {
		return fmt.Errorf("%w: password must contain at least one uppercase letter", ErrInvalidPassword)
	}

	if policy.RequireLowercase && !strings.ContainsFunc(password, unicode.IsLower) {
		return fmt.Errorf("%w: password must contain at least one lowercase letter", ErrInvalidPassword)
	}

	if policy.RequireNumbers && !strings.ContainsFunc(password, unicode.IsDigit) {
		return fmt.Errorf("%w: password must contain at least one number", ErrInvalidPassword)
	}

	if policy.RequireSymbols {
		isSymbol := func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsSpace(r) }
		if !strings.ContainsFunc(password, isSymbol) {
			return fmt.Errorf("%w: password must contain at least one symbol", ErrInvalidPassword)
		}
	}

	return nil
}

// CreateUserPoolWithOpts creates a user pool with optional PasswordPolicy and AutoVerifiedAttributes.
func (b *InMemoryBackend) CreateUserPoolWithOpts(name string, opts UserPoolOptions) (*UserPool, error) {
	b.mu.Lock("CreateUserPoolWithOpts")
	defer b.mu.Unlock()

	poolID := b.region + "_" + randomAlphanumeric(poolIDSuffixLen)
	issuerURL := fmt.Sprintf("%s/%s", b.endpoint, poolID)

	issuer, err := newTokenIssuer(issuerURL)
	if err != nil {
		return nil, fmt.Errorf("creating token issuer: %w", err)
	}

	autoVerified := make([]string, len(opts.AutoVerifiedAttributes))
	copy(autoVerified, opts.AutoVerifiedAttributes)

	pool := &UserPool{
		ID:                     poolID,
		Name:                   name,
		ARN:                    arn.Build("cognito-idp", b.region, b.accountID, fmt.Sprintf("userpool/%s", poolID)),
		CreatedAt:              time.Now(),
		issuer:                 issuer,
		AutoVerifiedAttributes: autoVerified,
		PasswordPolicy:         opts.PasswordPolicy,
		LambdaConfig:           opts.LambdaConfig,
		EmailConfiguration:     opts.EmailConfiguration,
		AccountRecoverySetting: opts.AccountRecoverySetting,
		DeletionProtection:     opts.DeletionProtection,
		MfaConfiguration:       opts.MfaConfiguration,
	}

	b.pools.Put(pool)

	cp := *pool

	return &cp, nil
}

// UpdateUserPoolWithOpts updates user pool PasswordPolicy, AutoVerifiedAttributes, and MFA config.
func (b *InMemoryBackend) UpdateUserPoolWithOpts(
	userPoolID, mfaConfiguration string,
	opts UserPoolOptions,
) error {
	b.mu.Lock("UpdateUserPoolWithOpts")
	defer b.mu.Unlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	if mfaConfiguration != "" {
		pool.MfaConfiguration = mfaConfiguration
	}

	if opts.PasswordPolicy != nil {
		pp := *opts.PasswordPolicy
		pool.PasswordPolicy = &pp
	}

	if opts.AutoVerifiedAttributes != nil {
		av := make([]string, len(opts.AutoVerifiedAttributes))
		copy(av, opts.AutoVerifiedAttributes)
		pool.AutoVerifiedAttributes = av
	}

	if opts.LambdaConfig != nil {
		pool.LambdaConfig = opts.LambdaConfig
	}

	if opts.EmailConfiguration != nil {
		pool.EmailConfiguration = opts.EmailConfiguration
	}

	if opts.AccountRecoverySetting != nil {
		pool.AccountRecoverySetting = opts.AccountRecoverySetting
	}

	if opts.DeletionProtection != "" {
		pool.DeletionProtection = opts.DeletionProtection
	}

	pool.UpdatedAt = time.Now()

	return nil
}

// cloudFrontDistIDLen is the length of the random part in generated CloudFront distribution IDs.
const cloudFrontDistIDLen = 13

// SetUserPoolMfaConfigFull stores the full MFA configuration for a pool.
func (b *InMemoryBackend) SetUserPoolMfaConfigFull(userPoolID string, cfg UserPoolMfaFullConfig) error {
	b.mu.Lock("SetUserPoolMfaConfigFull")
	defer b.mu.Unlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	if cfg.MfaConfiguration != "" {
		pool.MfaConfiguration = cfg.MfaConfiguration
	}

	b.poolMfaConfigs[userPoolID] = &cfg

	return nil
}

// GetUserPoolMfaConfigFull returns the full MFA configuration for a pool.
func (b *InMemoryBackend) GetUserPoolMfaConfigFull(userPoolID string) (*UserPoolMfaFullConfig, error) {
	b.mu.RLock("GetUserPoolMfaConfigFull")
	defer b.mu.RUnlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return nil, fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	cfg, ok := b.poolMfaConfigs[userPoolID]
	if !ok {
		mfaVal := pool.MfaConfiguration
		if mfaVal == "" {
			mfaVal = mfaConfigOFF
		}

		return &UserPoolMfaFullConfig{MfaConfiguration: mfaVal}, nil
	}

	cp := *cfg

	return &cp, nil
}
