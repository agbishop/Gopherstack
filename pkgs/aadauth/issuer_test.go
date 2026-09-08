package aadauth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/aadauth"
)

func TestNewIssuer(t *testing.T) {
	t.Parallel()

	iss, err := aadauth.NewIssuer()
	require.NoError(t, err)
	assert.NotEmpty(t, iss.KeyID())

	iss2, err := aadauth.NewIssuer()
	require.NoError(t, err)
	assert.NotEqual(t, iss.KeyID(), iss2.KeyID(), "each issuer should get a distinct key ID")
}

func TestIssuer_IssueClientCredentialsToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		expirySeconds int
		wantExpiry    int64
	}{
		{name: "explicit expiry", expirySeconds: 120, wantExpiry: 120},
		{name: "zero uses default", expirySeconds: 0, wantExpiry: aadauth.DefaultTokenExpirySeconds},
		{name: "negative uses default", expirySeconds: -5, wantExpiry: aadauth.DefaultTokenExpirySeconds},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			iss, err := aadauth.NewIssuer()
			require.NoError(t, err)

			claims := aadauth.ClientCredentialsClaims{
				Issuer:   "https://host:10006/00000000-0000-0000-0000-000000000000/v2.0",
				Audience: "https://management.azure.com/.default",
				TenantID: "00000000-0000-0000-0000-000000000000",
				ClientID: "00000000-0000-0000-0000-000000000000",
			}

			tokenString, err := iss.IssueClientCredentialsToken(claims, tt.expirySeconds)
			require.NoError(t, err)
			assert.NotEmpty(t, tokenString)

			parsed, err := iss.Parse(tokenString)
			require.NoError(t, err)

			assert.Equal(t, claims.Issuer, parsed["iss"])
			assert.Equal(t, claims.Audience, parsed["aud"])
			assert.Equal(t, claims.TenantID, parsed["tid"])
			assert.Equal(t, claims.ClientID, parsed["appid"])
			assert.Equal(t, claims.ClientID, parsed["oid"])
			assert.Equal(t, claims.ClientID, parsed["sub"])
			assert.Equal(t, "2.0", parsed["ver"])

			exp, ok := parsed["exp"].(float64)
			require.True(t, ok)
			iat, ok := parsed["iat"].(float64)
			require.True(t, ok)
			assert.InDelta(t, tt.wantExpiry, exp-iat, 1)
		})
	}
}

func TestIssuer_Parse_RejectsWrongMethodOrTamperedToken(t *testing.T) {
	t.Parallel()

	iss, err := aadauth.NewIssuer()
	require.NoError(t, err)

	claims := aadauth.ClientCredentialsClaims{
		Issuer:   "https://host/tenant/v2.0",
		Audience: "aud",
		TenantID: "tenant",
		ClientID: "client",
	}

	tokenString, err := iss.IssueClientCredentialsToken(claims, 60)
	require.NoError(t, err)

	// Tamper: flip a character in the middle of the signature segment. (Flipping
	// the very last character of a base64url string can leave the decoded bytes
	// unchanged, since the trailing character's low bits may be padding-only.)
	mid := len(tokenString) / 2
	flip := byte('x')

	if tokenString[mid] == 'x' {
		flip = 'y'
	}

	tampered := tokenString[:mid] + string(flip) + tokenString[mid+1:]

	_, err = iss.Parse(tampered)
	require.Error(t, err)

	// A different issuer's key must not validate this issuer's tokens.
	other, err := aadauth.NewIssuer()
	require.NoError(t, err)

	_, err = other.Parse(tokenString)
	require.Error(t, err)

	// HS256-signed token must be rejected (unexpected signing method).
	hsToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()})

	hsSigned, err := hsToken.SignedString([]byte("secret"))
	require.NoError(t, err)

	_, err = iss.Parse(hsSigned)
	require.Error(t, err)
}

func TestIssuer_Validate(t *testing.T) {
	t.Parallel()

	iss, err := aadauth.NewIssuer()
	require.NoError(t, err)

	claims := aadauth.ClientCredentialsClaims{
		Issuer:   "https://host/tenant/v2.0",
		Audience: "https://management.azure.com/.default",
		TenantID: "tenant-a",
		ClientID: "client",
	}

	tokenString, err := iss.IssueClientCredentialsToken(claims, 60)
	require.NoError(t, err)

	tests := []struct {
		name        string
		wantAud     string
		wantTenant  string
		expectError bool
	}{
		{name: "matches", wantAud: claims.Audience, wantTenant: claims.TenantID, expectError: false},
		{name: "audience mismatch", wantAud: "wrong", wantTenant: claims.TenantID, expectError: true},
		{name: "tenant mismatch", wantAud: claims.Audience, wantTenant: "wrong", expectError: true},
		{name: "empty wants skip checks", wantAud: "", wantTenant: "", expectError: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := iss.Validate(tokenString, tt.wantAud, tt.wantTenant)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIssuer_PublicKeyForKID(t *testing.T) {
	t.Parallel()

	iss, err := aadauth.NewIssuer()
	require.NoError(t, err)

	_, ok := iss.PublicKeyForKID(iss.KeyID())
	assert.True(t, ok)

	_, ok = iss.PublicKeyForKID("wrong-kid")
	assert.False(t, ok)
}

func TestIssuer_JWKS(t *testing.T) {
	t.Parallel()

	iss, err := aadauth.NewIssuer()
	require.NoError(t, err)

	jwks := iss.JWKS()
	require.Len(t, jwks.Keys, 1)

	key := jwks.Keys[0]
	assert.Equal(t, "RSA", key.Kty)
	assert.Equal(t, iss.KeyID(), key.Kid)
	assert.Equal(t, "sig", key.Use)
	assert.Equal(t, "RS256", key.Alg)
	assert.NotEmpty(t, key.N)
	assert.NotEmpty(t, key.E)
}
