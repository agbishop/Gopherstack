package cognitoidp

import "time"

// UserPoolCount returns the number of user pools in the backend. For testing only.
func (b *InMemoryBackend) UserPoolCount() int {
	b.mu.RLock("UserPoolCount")
	defer b.mu.RUnlock()

	return b.pools.Len()
}

// UserCount returns the total number of users across all pools. For testing only.
func (b *InMemoryBackend) UserCount() int {
	b.mu.RLock("UserCount")
	defer b.mu.RUnlock()

	return b.users.Len()
}

// ClientCount returns the number of user pool clients. For testing only.
func (b *InMemoryBackend) ClientCount() int {
	b.mu.RLock("ClientCount")
	defer b.mu.RUnlock()

	return b.clients.Len()
}

// RefreshTokenCount returns the number of refresh tokens in the backend. For testing only.
func (b *InMemoryBackend) RefreshTokenCount() int {
	b.mu.RLock("RefreshTokenCount")
	defer b.mu.RUnlock()

	return len(b.refreshTokens)
}

// MFASessionCount returns the number of pending MFA challenge sessions. For testing only.
func (b *InMemoryBackend) MFASessionCount() int {
	b.mu.RLock("MFASessionCount")
	defer b.mu.RUnlock()

	return len(b.mfaSessions)
}

// PoolMfaConfigCount returns the number of pools with a stored full MFA config. For testing only.
func (b *InMemoryBackend) PoolMfaConfigCount() int {
	b.mu.RLock("PoolMfaConfigCount")
	defer b.mu.RUnlock()

	return len(b.poolMfaConfigs)
}

// AttrVerificationCodeCount returns the number of pending attribute verification codes. For testing only.
func (b *InMemoryBackend) AttrVerificationCodeCount() int {
	b.mu.RLock("AttrVerificationCodeCount")
	defer b.mu.RUnlock()

	return len(b.attrVerificationCodes)
}

// SetRefreshTokenExpiry sets expiry for a refresh token. For testing only.
func (b *InMemoryBackend) SetRefreshTokenExpiry(token string, expiresAt time.Time) bool {
	b.mu.Lock("SetRefreshTokenExpiry")
	defer b.mu.Unlock()

	entry, ok := b.refreshTokens[token]
	if !ok {
		return false
	}

	entry.ExpiresAt = expiresAt

	return true
}

// ExpireMFASessionForTest forces a session's ExpiresAt to a past time. For testing only.
func (b *InMemoryBackend) ExpireMFASessionForTest(session string) {
	b.mu.Lock("ExpireMFASessionForTest")
	defer b.mu.Unlock()

	if entry, ok := b.mfaSessions[session]; ok {
		entry.ExpiresAt = time.Now().Add(-time.Hour)
	}
}

// ClearConfirmCodeForTest clears a user's stored confirmation code. For testing only.
func (b *InMemoryBackend) ClearConfirmCodeForTest(poolID, username string) {
	b.mu.Lock("ClearConfirmCodeForTest")
	defer b.mu.Unlock()

	if u, ok := b.users.Get(userKey(poolID, username)); ok {
		u.ConfirmCode = ""
		u.ConfirmCodeExpiresAt = time.Time{}
	}
}

// ExpireConfirmCodeForTest sets a user's confirmation code expiry to the past. For testing only.
func (b *InMemoryBackend) ExpireConfirmCodeForTest(poolID, username string) {
	b.mu.Lock("ExpireConfirmCodeForTest")
	defer b.mu.Unlock()

	if u, ok := b.users.Get(userKey(poolID, username)); ok {
		u.ConfirmCodeExpiresAt = time.Now().Add(-time.Hour)
	}
}

// UserPoolID returns the pool ID for a client. For testing only.
func (b *InMemoryBackend) UserPoolID(clientID string) string {
	b.mu.RLock("UserPoolID")
	defer b.mu.RUnlock()

	if c, ok := b.clients.Get(clientID); ok {
		return c.UserPoolID
	}

	return ""
}

// GetMFASessionCodeForTest returns the one-time SMS_MFA/EMAIL_OTP code generated for a
// pending MFA session (SOFTWARE_TOKEN_MFA sessions have no stored code — it's verified
// cryptographically against the user's TOTP secret instead). For testing only.
func (b *InMemoryBackend) GetMFASessionCodeForTest(session string) string {
	b.mu.RLock("GetMFASessionCodeForTest")
	defer b.mu.RUnlock()

	if entry, ok := b.mfaSessions[session]; ok {
		return entry.Code
	}

	return ""
}

// SeedAuthEventForTest directly inserts an AuthEvent for a user, bypassing
// the normal (currently unimplemented) sign-in event hooks. For testing only.
func (b *InMemoryBackend) SeedAuthEventForTest(poolID, username string, ev *AuthEvent) {
	b.mu.Lock("SeedAuthEventForTest")
	defer b.mu.Unlock()

	key := userStateKey(poolID, username)
	if b.authEvents[key] == nil {
		b.authEvents[key] = make(map[string]*AuthEvent)
	}

	b.authEvents[key][ev.EventID] = ev
}

// SeedDeviceForTest directly inserts a Device for a user, bypassing the
// normal ConfirmDevice flow. For testing only.
func (b *InMemoryBackend) SeedDeviceForTest(poolID, username string, dev *Device) {
	b.mu.Lock("SeedDeviceForTest")
	defer b.mu.Unlock()

	key := userStateKey(poolID, username)
	if b.devices[key] == nil {
		b.devices[key] = make(map[string]*Device)
	}

	b.devices[key][dev.DeviceKey] = dev
}

// HasDeviceStateForTest reports whether any device or auth-event state is
// stored under a user's state key. For testing only.
func (b *InMemoryBackend) HasDeviceStateForTest(poolID, username string) bool {
	b.mu.RLock("HasDeviceStateForTest")
	defer b.mu.RUnlock()

	key := userStateKey(poolID, username)

	return len(b.devices[key]) > 0 || len(b.authEvents[key]) > 0
}

// SeedWebAuthnCredentialForTest directly inserts a WebAuthn credential for a
// user, bypassing the normal CompleteWebAuthnRegistration flow (which
// requires an access token). For testing only.
func (b *InMemoryBackend) SeedWebAuthnCredentialForTest(poolID, username string, cred *WebAuthnCredential) {
	b.mu.Lock("SeedWebAuthnCredentialForTest")
	defer b.mu.Unlock()

	key := userStateKey(poolID, username)
	if b.webauthnCredentials[key] == nil {
		b.webauthnCredentials[key] = make(map[string]*WebAuthnCredential)
	}

	b.webauthnCredentials[key][cred.CredentialID] = cred
}

// AddUserPoolDomainInternal seeds a domain directly into the backend,
// bypassing CreateUserPoolDomain's pool-existence check. For testing only --
// simulates a domain orphaned by data that predates DeleteUserPool's guard
// against deleting a pool with an attached domain (gopherstack-tq5q).
func (b *InMemoryBackend) AddUserPoolDomainInternal(domain *UserPoolDomain) {
	b.mu.Lock("AddUserPoolDomainInternal")
	defer b.mu.Unlock()

	b.domains.Put(domain)
}

// ExpireAttrVerificationCodeForTest forces an attribute verification code's ExpiresAt to a past time. For testing only.
func (b *InMemoryBackend) ExpireAttrVerificationCodeForTest(poolID, username, attrName string) {
	b.mu.Lock("ExpireAttrVerificationCodeForTest")
	defer b.mu.Unlock()

	key := poolID + ":" + username + ":" + attrName
	if entry, ok := b.attrVerificationCodes[key]; ok {
		entry.ExpiresAt = time.Now().Add(-time.Hour)
	}
}

// GetAttrVerificationCodeForTest returns the pending verification code for a user attribute. For testing only.
func (b *InMemoryBackend) GetAttrVerificationCodeForTest(poolID, username, attrName string) string {
	b.mu.RLock("GetAttrVerificationCodeForTest")
	defer b.mu.RUnlock()

	key := poolID + ":" + username + ":" + attrName
	if entry, ok := b.attrVerificationCodes[key]; ok {
		return entry.Code
	}

	return ""
}
