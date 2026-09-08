package iam

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// ---- SAML Provider operations ----

// CreateSAMLProvider creates a new IAM SAML identity provider.
// The provider name is used to build the ARN; it must be unique.
func (b *InMemoryBackend) CreateSAMLProvider(name, samlMetadataDocument string) (*SAMLProvider, error) {
	b.mu.Lock("CreateSAMLProvider")
	defer b.mu.Unlock()

	providerArn := arn.Build("iam", "", b.accountID, "saml-provider/"+name)

	if err := validateSAMLMetadata(samlMetadataDocument); err != nil {
		return nil, err
	}

	if _, exists := b.samlProviders.Get(providerArn); exists {
		return nil, fmt.Errorf("%w: SAML provider %q already exists", ErrSAMLProviderAlreadyExists, name)
	}

	p := SAMLProvider{
		Arn:                  providerArn,
		SAMLMetadataDocument: samlMetadataDocument,
		CreateDate:           time.Now().UTC(),
	}
	b.samlProviders.Put(&p)

	return &p, nil
}

// UpdateSAMLProvider replaces the SAML metadata document for an existing provider.
func (b *InMemoryBackend) UpdateSAMLProvider(providerArn, samlMetadataDocument string) (*SAMLProvider, error) {
	b.mu.Lock("UpdateSAMLProvider")
	defer b.mu.Unlock()

	p, exists := b.samlProviders.Get(providerArn)
	if !exists {
		return nil, fmt.Errorf("%w: SAML provider %q not found", ErrSAMLProviderNotFound, providerArn)
	}

	if err := validateSAMLMetadata(samlMetadataDocument); err != nil {
		return nil, err
	}

	p.SAMLMetadataDocument = samlMetadataDocument
	b.samlProviders.Put(p)

	return p, nil
}

// DeleteSAMLProvider removes a SAML provider by ARN.
func (b *InMemoryBackend) DeleteSAMLProvider(providerArn string) error {
	b.mu.Lock("DeleteSAMLProvider")
	defer b.mu.Unlock()

	if _, exists := b.samlProviders.Get(providerArn); !exists {
		return fmt.Errorf("%w: SAML provider %q not found", ErrSAMLProviderNotFound, providerArn)
	}

	b.samlProviders.Delete(providerArn)

	return nil
}

// GetSAMLProvider retrieves a SAML provider by ARN.
func (b *InMemoryBackend) GetSAMLProvider(providerArn string) (*SAMLProvider, error) {
	b.mu.RLock("GetSAMLProvider")
	defer b.mu.RUnlock()

	p, exists := b.samlProviders.Get(providerArn)
	if !exists {
		return nil, fmt.Errorf("%w: SAML provider %q not found", ErrSAMLProviderNotFound, providerArn)
	}

	return p, nil
}

// ListSAMLProviders returns all SAML providers sorted by ARN.
func (b *InMemoryBackend) ListSAMLProviders() ([]SAMLProvider, error) {
	b.mu.RLock("ListSAMLProviders")
	defer b.mu.RUnlock()

	result := make([]SAMLProvider, 0, b.samlProviders.Len())
	for _, p := range b.samlProviders.All() {
		result = append(result, *p)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Arn < result[j].Arn })

	return result, nil
}

// ---- OIDC Provider operations ----

// oidcProviderHostFromURL extracts the host portion from an OIDC provider URL.
// For example, "https://token.actions.githubusercontent.com" → "token.actions.githubusercontent.com".
func oidcProviderHostFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err == nil && u.Host != "" {
		return u.Host, nil
	}

	// Treat a URL without a scheme as a bare hostname (strip any path/query).
	bare := strings.TrimPrefix(rawURL, "https://")
	bare = strings.TrimPrefix(bare, "http://")

	// Retain only the host portion (before the first '/').
	if idx := strings.IndexByte(bare, '/'); idx >= 0 {
		bare = bare[:idx]
	}

	bare = strings.TrimSpace(bare)

	if bare == "" {
		return "", fmt.Errorf("%w: %q", ErrInvalidOIDCProviderURL, rawURL)
	}

	return bare, nil
}

// CreateOpenIDConnectProvider creates a new IAM OIDC identity provider.
func (b *InMemoryBackend) CreateOpenIDConnectProvider(
	rawURL string, clientIDs, thumbprints []string,
) (*OIDCProvider, error) {
	b.mu.Lock("CreateOpenIDConnectProvider")
	defer b.mu.Unlock()

	host, err := oidcProviderHostFromURL(rawURL)
	if err != nil {
		return nil, err
	}

	providerArn := arn.Build("iam", "", b.accountID, "oidc-provider/"+host)

	if vErr := validateThumbprints(thumbprints); vErr != nil {
		return nil, vErr
	}

	if _, exists := b.oidcProviders.Get(providerArn); exists {
		return nil, fmt.Errorf("%w: OIDC provider for URL %q already exists", ErrOIDCProviderAlreadyExists, rawURL)
	}

	p := OIDCProvider{
		Arn:            providerArn,
		URL:            rawURL,
		ClientIDList:   append([]string(nil), clientIDs...),
		ThumbprintList: append([]string(nil), thumbprints...),
		CreateDate:     time.Now().UTC(),
	}
	b.oidcProviders.Put(&p)

	return &p, nil
}

// UpdateOpenIDConnectProviderThumbprint replaces the thumbprint list for an existing OIDC provider.
func (b *InMemoryBackend) UpdateOpenIDConnectProviderThumbprint(providerArn string, thumbprints []string) error {
	b.mu.Lock("UpdateOpenIDConnectProviderThumbprint")
	defer b.mu.Unlock()

	p, exists := b.oidcProviders.Get(providerArn)
	if !exists {
		return fmt.Errorf("%w: OIDC provider %q not found", ErrOIDCProviderNotFound, providerArn)
	}

	if vErr := validateThumbprints(thumbprints); vErr != nil {
		return vErr
	}

	p.ThumbprintList = append([]string(nil), thumbprints...)
	b.oidcProviders.Put(p)

	return nil
}

// DeleteOpenIDConnectProvider removes an OIDC provider by ARN.
func (b *InMemoryBackend) DeleteOpenIDConnectProvider(providerArn string) error {
	b.mu.Lock("DeleteOpenIDConnectProvider")
	defer b.mu.Unlock()

	if _, exists := b.oidcProviders.Get(providerArn); !exists {
		return fmt.Errorf("%w: OIDC provider %q not found", ErrOIDCProviderNotFound, providerArn)
	}

	b.oidcProviders.Delete(providerArn)

	return nil
}

// GetOpenIDConnectProvider retrieves an OIDC provider by ARN.
func (b *InMemoryBackend) GetOpenIDConnectProvider(providerArn string) (*OIDCProvider, error) {
	b.mu.RLock("GetOpenIDConnectProvider")
	defer b.mu.RUnlock()

	p, exists := b.oidcProviders.Get(providerArn)
	if !exists {
		return nil, fmt.Errorf("%w: OIDC provider %q not found", ErrOIDCProviderNotFound, providerArn)
	}

	return p, nil
}

// ListOpenIDConnectProviders returns all OIDC providers sorted by ARN.
func (b *InMemoryBackend) ListOpenIDConnectProviders() ([]OIDCProvider, error) {
	b.mu.RLock("ListOpenIDConnectProviders")
	defer b.mu.RUnlock()

	result := make([]OIDCProvider, 0, b.oidcProviders.Len())
	for _, p := range b.oidcProviders.All() {
		result = append(result, *p)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Arn < result[j].Arn })

	return result, nil
}

// ---- Login Profile operations ----

// CreateLoginProfile creates a console login profile for an IAM user.
// The password is validated but not stored; this is an in-memory mock.
func (b *InMemoryBackend) CreateLoginProfile(
	userName, password string, passwordResetRequired bool,
) (*LoginProfile, error) {
	b.mu.Lock("CreateLoginProfile")
	defer b.mu.Unlock()

	if password == "" {
		return nil, fmt.Errorf("%w: password must not be empty", ErrInvalidPassword)
	}

	// Check resource existence before password policy so callers receive
	// the correct entity error (NoSuchEntity / EntityAlreadyExists) even if
	// the password also violates policy.
	if _, exists := b.users.Get(userName); !exists {
		return nil, fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	if _, exists := b.loginProfiles.Get(userName); exists {
		return nil, fmt.Errorf("%w: login profile for user %q already exists", ErrLoginProfileAlreadyExists, userName)
	}

	if err := validatePasswordAgainstPolicy(password, b.passwordPolicy, nil); err != nil {
		return nil, err
	}

	lp := LoginProfile{
		UserName:              userName,
		Password:              password,
		PasswordHistory:       recordPasswordHistory(nil, password, reusePreventionLimit(b.passwordPolicy)),
		CreateDate:            time.Now().UTC(),
		PasswordResetRequired: passwordResetRequired,
	}
	b.loginProfiles.Put(&lp)

	return &lp, nil
}

// UpdateLoginProfile updates the console login profile for an IAM user.
func (b *InMemoryBackend) UpdateLoginProfile(
	userName, password string, passwordResetRequired bool,
) error {
	b.mu.Lock("UpdateLoginProfile")
	defer b.mu.Unlock()

	if password == "" {
		return fmt.Errorf("%w: password must not be empty", ErrInvalidPassword)
	}

	lp, exists := b.loginProfiles.Get(userName)
	if !exists {
		return fmt.Errorf("%w: login profile for user %q not found", ErrLoginProfileNotFound, userName)
	}

	if err := validatePasswordAgainstPolicy(password, b.passwordPolicy, lp.PasswordHistory); err != nil {
		return err
	}

	lp.Password = password
	lp.PasswordHistory = recordPasswordHistory(lp.PasswordHistory, password, reusePreventionLimit(b.passwordPolicy))
	lp.PasswordResetRequired = passwordResetRequired
	b.loginProfiles.Put(lp)

	return nil
}

// DeleteLoginProfile removes the console login profile for an IAM user.
func (b *InMemoryBackend) DeleteLoginProfile(userName string) error {
	b.mu.Lock("DeleteLoginProfile")
	defer b.mu.Unlock()

	if _, exists := b.loginProfiles.Get(userName); !exists {
		return fmt.Errorf("%w: login profile for user %q not found", ErrLoginProfileNotFound, userName)
	}

	b.loginProfiles.Delete(userName)

	return nil
}

func validateSAMLMetadata(doc string) error {
	var v any
	if err := xml.Unmarshal([]byte(doc), &v); err != nil {
		return fmt.Errorf("%w: invalid XML", ErrInvalidInput)
	}

	return nil
}

const thumbprintLen = 40

func validateThumbprints(thumbprints []string) error {
	for _, t := range thumbprints {
		if len(t) != thumbprintLen {
			return fmt.Errorf("%w: thumbprint must be 40 characters long", ErrInvalidInput)
		}
		for _, c := range t {
			isDigit := c >= '0' && c <= '9'
			isLowerHex := c >= 'a' && c <= 'f'
			isUpperHex := c >= 'A' && c <= 'F'
			if !isDigit && !isLowerHex && !isUpperHex {
				return fmt.Errorf("%w: thumbprint must be hex", ErrInvalidInput)
			}
		}
	}

	return nil
}

// GetLoginProfile retrieves the console login profile for an IAM user.
func (b *InMemoryBackend) GetLoginProfile(userName string) (*LoginProfile, error) {
	b.mu.RLock("GetLoginProfile")
	defer b.mu.RUnlock()

	lp, exists := b.loginProfiles.Get(userName)
	if !exists {
		return nil, fmt.Errorf("%w: login profile for user %q not found", ErrLoginProfileNotFound, userName)
	}

	return lp, nil
}

// purgeSAMLProvidersLocked removes SAML providers created before cutoff.
// Caller must hold b.mu.
func (b *InMemoryBackend) purgeSAMLProvidersLocked(cutoff time.Time) {
	for _, p := range b.samlProviders.All() {
		if p.CreateDate.Before(cutoff) {
			b.samlProviders.Delete(p.Arn)
		}
	}
}

// purgeOIDCProvidersLocked removes OIDC providers created before cutoff.
// Caller must hold b.mu.
func (b *InMemoryBackend) purgeOIDCProvidersLocked(cutoff time.Time) {
	for _, p := range b.oidcProviders.All() {
		if p.CreateDate.Before(cutoff) {
			b.oidcProviders.Delete(p.Arn)
		}
	}
}

// OIDCProviderExists reports whether an OIDC provider with the given issuer URL exists.
// The issuer URL may or may not have a trailing slash; both forms are checked.
// This method implements the sts.OIDCLookup interface.
func (b *InMemoryBackend) OIDCProviderExists(issuerURL string) bool {
	b.mu.RLock("OIDCProviderExists")
	defer b.mu.RUnlock()

	// Normalise the issuer URL to strip trailing slashes for comparison.
	normalised := strings.TrimRight(issuerURL, "/")

	for _, p := range b.oidcProviders.All() {
		providerURL := strings.TrimRight(p.URL, "/")
		if providerURL == normalised {
			return true
		}
	}

	return false
}

// RemoveClientIDFromOpenIDConnectProvider removes a client ID from an OIDC provider.
func (b *InMemoryBackend) RemoveClientIDFromOpenIDConnectProvider(providerArn, clientID string) error {
	b.mu.Lock("RemoveClientIDFromOpenIDConnectProvider")
	defer b.mu.Unlock()

	p, exists := b.oidcProviders.Get(providerArn)
	if !exists {
		return fmt.Errorf("%w: OIDC provider %q not found", ErrOIDCProviderNotFound, providerArn)
	}

	for i, id := range p.ClientIDList {
		if id == clientID {
			p.ClientIDList = append(p.ClientIDList[:i], p.ClientIDList[i+1:]...)
			b.oidcProviders.Put(p)

			return nil
		}
	}

	// RemoveClientIDFromOpenIDConnectProvider is documented as idempotent: "it
	// does not fail or return an error if you try to remove a client ID that
	// does not exist" (iam@v1.58.1 api_op_RemoveClientIDFromOpenIDConnectProvider.go:15).
	return nil
}

// AddClientIDToOpenIDConnectProvider appends a client ID to an existing OIDC provider.
// If the client ID is already present, the call is idempotent.
func (b *InMemoryBackend) AddClientIDToOpenIDConnectProvider(providerArn, clientID string) error {
	if clientID == "" {
		return fmt.Errorf("%w: ClientID must not be empty", ErrInvalidInput)
	}

	b.mu.Lock("AddClientIDToOpenIDConnectProvider")
	defer b.mu.Unlock()

	p, exists := b.oidcProviders.Get(providerArn)
	if !exists {
		return fmt.Errorf("%w: OIDC provider %q not found", ErrOIDCProviderNotFound, providerArn)
	}

	if !slices.Contains(p.ClientIDList, clientID) {
		p.ClientIDList = append(p.ClientIDList, clientID)
		b.oidcProviders.Put(p)
	}

	return nil
}
