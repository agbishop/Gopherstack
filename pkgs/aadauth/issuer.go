// Package aadauth provides a shared, dependency-light emulation of Azure AD
// (Microsoft Entra ID)'s client-credentials OAuth2 flow: an RSA-signed JWT
// issuer, JWKS publication, and the two AAD instance/authority discovery
// documents (openid-configuration and /common/discovery/instance).
//
// It is modeled on services/cognitoidp/tokens.go -- the existing precedent
// in this repo for "issue structurally-valid OAuth2/JWT tokens without real
// cryptographic trust" -- but deliberately NOT imported from there: Cognito's
// tokenIssuer is unexported and coupled to Cognito's own
// UserPoolClient/clientTokenSettings types, so it can't be reused as-is, and
// this package's shape (AAD claims: iss/aud/appid/tid/oid/sub/ver, not
// Cognito's cognito:username/token_use) is different enough that sharing
// code would mean threading AAD-specific special cases through Cognito's
// file instead of owning a small, purpose-built package. See AZURE.md
// section 10.5.
//
// services/azurearm is the first consumer (M7); services/keyvault (M11) is
// expected to reuse it for its own dev-mode bearer-token auth.
package aadauth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// rsaKeyBits is the size of the RSA key generated for JWT signing.
const rsaKeyBits = 2048

// keyIDLen is the byte length of the random key ID.
const keyIDLen = 8

// aadVersion is the "ver" claim value stamped on every issued token,
// matching the v2.0 endpoint shape this package emulates exclusively.
const aadVersion = "2.0"

// DefaultTokenExpirySeconds is the lifetime, in seconds, of an issued access
// token -- matching the real AAD v2.0 client-credentials response's typical
// expires_in of 3599/3600 closely enough for SDK token-cache logic that only
// checks "is this expired soon."
const DefaultTokenExpirySeconds = 3599

// Issuer generates and signs AAD-shaped client-credentials JWTs for a single
// tenant, and publishes the corresponding JWKS. One Issuer is created per
// running service instance (see services/azurearm/token.go); its RSA keypair
// is generated once at startup and is not persisted across restarts --
// tokens issued before a restart no longer validate against the new key,
// which is acceptable because validation itself is opt-in and off by default
// (AZURE.md section 10.5).
type Issuer struct {
	privateKey *rsa.PrivateKey
	keyID      string
}

// NewIssuer generates a fresh RSA-2048 keypair and a random key ID.
func NewIssuer() (*Issuer, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("aadauth: generating RSA key: %w", err)
	}

	kidBytes := make([]byte, keyIDLen)
	if _, err = rand.Read(kidBytes); err != nil {
		return nil, fmt.Errorf("aadauth: generating key ID: %w", err)
	}

	return &Issuer{
		privateKey: privateKey,
		keyID:      base64.RawURLEncoding.EncodeToString(kidBytes),
	}, nil
}

// ClientCredentialsClaims are the AAD v2.0-shaped claims this package
// stamps into every issued token (AZURE.md section 10.5's claim list).
type ClientCredentialsClaims struct {
	Issuer   string // https://<host>/<tenant>/v2.0
	Audience string // requested scope's resource, e.g. https://management.azure.com
	TenantID string
	ClientID string // becomes both "appid" and "oid"/"sub" (a service principal has no separate user)
}

// IssueClientCredentialsToken signs and returns an AAD-shaped access token
// for the client-credentials grant. expirySeconds <= 0 uses
// DefaultTokenExpirySeconds.
func (iss *Issuer) IssueClientCredentialsToken(claims ClientCredentialsClaims, expirySeconds int) (string, error) {
	if expirySeconds <= 0 {
		expirySeconds = DefaultTokenExpirySeconds
	}

	now := time.Now().Unix()
	expiry := int64(expirySeconds)

	jwtClaims := jwt.MapClaims{
		"iss":   claims.Issuer,
		"aud":   claims.Audience,
		"tid":   claims.TenantID,
		"appid": claims.ClientID,
		"oid":   claims.ClientID,
		"sub":   claims.ClientID,
		"ver":   aadVersion,
		"iat":   now,
		"nbf":   now,
		"exp":   now + expiry,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtClaims)
	token.Header["kid"] = iss.keyID

	signed, err := token.SignedString(iss.privateKey)
	if err != nil {
		return "", fmt.Errorf("aadauth: signing token: %w", err)
	}

	return signed, nil
}

// KeyID returns the issuer's JWKS key ID.
func (iss *Issuer) KeyID() string { return iss.keyID }

// PublicKeyForKID returns the RSA public key if kid matches this issuer's
// key ID.
func (iss *Issuer) PublicKeyForKID(kid string) (*rsa.PublicKey, bool) {
	if iss.keyID != kid {
		return nil, false
	}

	return &iss.privateKey.PublicKey, true
}
