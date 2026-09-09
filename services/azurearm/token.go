package azurearm

import (
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/aadauth"
)

// defaultTokenAudience is used when a client-credentials request's "scope"
// form field is empty or doesn't carry a resource prefix.
const defaultTokenAudience = "https://management.azure.com/" //nolint:gosec // URL, not a credential

// TokenResponse is the client-credentials grant response body, for both the
// v1 (/{tenant}/oauth2/token) and v2 (/{tenant}/oauth2/v2.0/token) endpoints
// -- AZURE.md section 10.1 specifies the identical body shape for both.
type TokenResponse struct {
	TokenType   string `json:"token_type"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// audienceFromScope extracts the resource audience from an OAuth2 "scope"
// (v2 shape, e.g. "https://management.azure.com/.default") or "resource"
// (v1 shape) form value, stripping a trailing "/.default" if present.
func audienceFromScope(scopeOrResource string) string {
	s := strings.TrimSpace(scopeOrResource)
	if s == "" {
		return defaultTokenAudience
	}

	s = strings.TrimSuffix(s, "/.default")
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}

	return s
}

// IssueToken issues a client-credentials access token for tenant, using
// issuer, returning the wire response body.
func IssueToken(issuer *aadauth.Issuer, baseURL, tenant, clientID, scopeOrResource string) (TokenResponse, error) {
	audience := audienceFromScope(scopeOrResource)

	claims := aadauth.ClientCredentialsClaims{
		Issuer:   baseURL + "/" + tenant + "/v2.0",
		Audience: audience,
		TenantID: tenant,
		ClientID: clientID,
	}

	tokenString, err := issuer.IssueClientCredentialsToken(claims, aadauth.DefaultTokenExpirySeconds)
	if err != nil {
		return TokenResponse{}, err
	}

	return TokenResponse{
		TokenType:   "Bearer",
		ExpiresIn:   aadauth.DefaultTokenExpirySeconds,
		AccessToken: tokenString,
	}, nil
}
