package cognitoidp

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	attrEmail = "email"
)

// UpdateUserAttributes updates the attributes of an authenticated user.
func (b *InMemoryBackend) UpdateUserAttributes(accessToken string, attributes map[string]string) error {
	b.mu.Lock("UpdateUserAttributes")
	defer b.mu.Unlock()

	u, err := b.findUserByAccessTokenLocked(accessToken)
	if err != nil {
		return err
	}

	if u.Attributes == nil {
		u.Attributes = make(map[string]string)
	}

	maps.Copy(u.Attributes, attributes)
	u.UpdatedAt = time.Now()

	return nil
}

// AdminUpdateUserAttributes updates attributes for a user in a pool.
func (b *InMemoryBackend) AdminUpdateUserAttributes(userPoolID, username string, attributes map[string]string) error {
	b.mu.Lock("AdminUpdateUserAttributes")
	defer b.mu.Unlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	u, ok := b.users.Get(userKey(userPoolID, username))
	if !ok {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, username)
	}

	if u.Attributes == nil {
		u.Attributes = make(map[string]string)
	}

	maps.Copy(u.Attributes, attributes)
	u.UpdatedAt = time.Now()

	return nil
}

// AddCustomAttributes adds custom attribute definitions to a user pool schema.
// All attribute names must start with the "custom:" prefix as required by AWS Cognito.
func (b *InMemoryBackend) AddCustomAttributes(userPoolID string, attrs []SchemaAttribute) error {
	b.mu.Lock("AddCustomAttributes")
	defer b.mu.Unlock()

	pool, ok := b.pools.Get(userPoolID)
	if !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	for _, a := range attrs {
		if !strings.HasPrefix(a.Name, "custom:") {
			// AddCustomAttributes's own deserializer models
			// InvalidParameterException, not InvalidUserPoolConfigurationException
			// (aws-sdk-go-v2/service/cognitoidentityprovider@v1.67.4
			// deserializers.go) — unlike InitiateAuth/AdminInitiateAuth, which
			// genuinely do model the latter for auth-flow misconfiguration.
			return fmt.Errorf(
				"%w: attribute name %q must start with 'custom:' prefix",
				ErrInvalidParameter,
				a.Name,
			)
		}
	}

	pool.CustomAttributes = append(pool.CustomAttributes, attrs...)

	return nil
}

// AdminDeleteUserAttributes removes specific attributes from a user in a pool.
func (b *InMemoryBackend) AdminDeleteUserAttributes(userPoolID, username string, attrNames []string) error {
	b.mu.Lock("AdminDeleteUserAttributes")
	defer b.mu.Unlock()

	if _, ok := b.pools.Get(userPoolID); !ok {
		return fmt.Errorf("%w: pool %q not found", ErrUserPoolNotFound, userPoolID)
	}

	u, ok := b.users.Get(userKey(userPoolID, username))
	if !ok {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, username)
	}

	for _, name := range attrNames {
		delete(u.Attributes, name)
	}

	return nil
}

// DeleteUserAttributes removes specific attributes from the authenticated user (self-service).
func (b *InMemoryBackend) DeleteUserAttributes(accessToken string, attrNames []string) error {
	b.mu.Lock("DeleteUserAttributes")
	defer b.mu.Unlock()

	u, err := b.findUserByAccessTokenLocked(accessToken)
	if err != nil {
		return err
	}

	for _, name := range attrNames {
		delete(u.Attributes, name)
	}

	u.UpdatedAt = time.Now()

	return nil
}

// VerifyUserAttribute is a no-op stub: the mock does not send verification codes so
// all attributes are considered already verified. Returns success for any code.
func (b *InMemoryBackend) VerifyUserAttribute(accessToken, attributeName, _ string) error {
	b.mu.RLock("VerifyUserAttribute")
	defer b.mu.RUnlock()

	if _, err := b.findUserByAccessTokenLocked(accessToken); err != nil {
		return err
	}

	// Attribute name validation: must be a known verifiable attribute.
	switch attributeName {
	case attrEmail, attrPhoneNumber:
		// valid
	default:
		// VerifyUserAttribute's own deserializer models InvalidParameterException
		// for this, not InvalidUserPoolConfigurationException.
		return fmt.Errorf("%w: attribute %q is not verifiable", ErrInvalidParameter, attributeName)
	}

	return nil
}

// sortedCustomAttributes returns a copy of the pool's custom attributes sorted by name.
func sortedCustomAttributes(attrs []SchemaAttribute) []SchemaAttribute {
	if len(attrs) == 0 {
		return nil
	}

	cp := slices.Clone(attrs)
	sort.Slice(cp, func(i, j int) bool { return cp[i].Name < cp[j].Name })

	return cp
}

// attrVerificationTTL is how long attribute verification codes remain valid.
const attrVerificationTTL = 24 * time.Hour

// attrVerificationCodeLen is the length of attribute verification codes.
const attrVerificationCodeLen = 6

// attrPhoneNumber is the standard Cognito phone_number attribute name.
const attrPhoneNumber = "phone_number"

// attrVerifiedTrue is the string value for a verified attribute.
const attrVerifiedTrue = "true"

// maskEmailPrefixLen is the number of chars shown unmasked in an email prefix.
const maskEmailPrefixLen = 2

// maskPhoneVisibleSuffix is the number of phone digits shown unmasked at the end.
const maskPhoneVisibleSuffix = 4

// GetUserAttributeVerificationCode generates a verification code for a user attribute
// and stores it for later validation via VerifyUserAttributeWithCode.
func (b *InMemoryBackend) GetUserAttributeVerificationCode(
	accessToken, attributeName string,
) (string, string, string, error) {
	b.mu.Lock("GetUserAttributeVerificationCode")
	defer b.mu.Unlock()

	user, verr := b.findUserByAccessTokenLocked(accessToken)
	if verr != nil {
		return "", "", "", verr
	}

	switch attributeName {
	case attrEmail, attrPhoneNumber:
		// valid verifiable attributes
	default:
		return "", "", "", fmt.Errorf("%w: attribute %q is not verifiable", ErrInvalidParameter, attributeName)
	}

	code := randomAlphanumeric(attrVerificationCodeLen)
	key := user.UserPoolID + ":" + user.Username + ":" + attributeName

	b.attrVerificationCodes[key] = &attrVerificationEntry{
		Code:      code,
		ExpiresAt: time.Now().Add(attrVerificationTTL),
	}

	// Determine delivery destination and medium.
	if attributeName == attrEmail {
		dest := user.Attributes[attrEmail]
		if dest == "" {
			dest = "unknown@example.com"
		}
		// Mask email: show first 2 chars + *** + domain.
		dest = maskEmail(dest)

		return code, dest, "EMAIL", nil
	}

	// phone_number
	dest := user.Attributes[attrPhoneNumber]
	if dest == "" {
		dest = "+*******1234"
	}

	dest = maskPhone(dest)

	return code, dest, "SMS", nil
}

// VerifyUserAttributeWithCode validates the code stored by GetUserAttributeVerificationCode
// and marks the attribute as verified in the user record.
func (b *InMemoryBackend) VerifyUserAttributeWithCode(accessToken, attributeName, code string) error {
	b.mu.Lock("VerifyUserAttributeWithCode")
	defer b.mu.Unlock()

	user, err := b.findUserByAccessTokenLocked(accessToken)
	if err != nil {
		return err
	}

	switch attributeName {
	case attrEmail, attrPhoneNumber:
	default:
		return fmt.Errorf("%w: attribute %q is not verifiable", ErrInvalidParameter, attributeName)
	}

	key := user.UserPoolID + ":" + user.Username + ":" + attributeName

	entry, ok := b.attrVerificationCodes[key]
	if !ok {
		return fmt.Errorf("%w: no verification code found for attribute %q", ErrCodeMismatch, attributeName)
	}

	if time.Now().After(entry.ExpiresAt) {
		delete(b.attrVerificationCodes, key)

		return fmt.Errorf("%w: verification code for attribute %q has expired", ErrExpiredCode, attributeName)
	}

	if entry.Code != code {
		return fmt.Errorf("%w: incorrect verification code for attribute %q", ErrCodeMismatch, attributeName)
	}

	delete(b.attrVerificationCodes, key)

	// Mark attribute as verified.
	user.Attributes[attributeName+"_verified"] = attrVerifiedTrue
	user.UpdatedAt = time.Now()

	return nil
}

// maskEmail masks most of an email address for privacy.
func maskEmail(email string) string {
	at := -1
	for i, c := range email {
		if c == '@' {
			at = i

			break
		}
	}

	if at <= 0 {
		return "***@***"
	}

	prefix := email[:at]
	domain := email[at:]

	if len(prefix) <= maskEmailPrefixLen {
		return prefix + "***" + domain
	}

	return prefix[:maskEmailPrefixLen] + "***" + domain
}

// maskPhone masks most of a phone number for privacy.
func maskPhone(phone string) string {
	if len(phone) <= maskPhoneVisibleSuffix {
		return "+*******"
	}

	return "+*******" + phone[len(phone)-maskPhoneVisibleSuffix:]
}

// EvictExpiredAttrVerificationCodes deletes expired codes and returns the count evicted.
func (b *InMemoryBackend) EvictExpiredAttrVerificationCodes() int {
	b.mu.Lock("EvictExpiredAttrVerificationCodes")
	defer b.mu.Unlock()

	now := time.Now()
	evicted := 0

	for key, entry := range b.attrVerificationCodes {
		if now.After(entry.ExpiresAt) {
			delete(b.attrVerificationCodes, key)
			evicted++
		}
	}

	return evicted
}
