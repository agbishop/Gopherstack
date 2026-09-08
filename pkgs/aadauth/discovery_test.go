package aadauth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/blackbirdworks/gopherstack/pkgs/aadauth"
)

func TestBuildOpenIDConfiguration(t *testing.T) {
	t.Parallel()

	cfg := aadauth.BuildOpenIDConfiguration("https://host:10006", "00000000-0000-0000-0000-000000000000")

	assert.Equal(t, "https://host:10006/00000000-0000-0000-0000-000000000000/v2.0", cfg.Issuer)
	assert.Equal(t,
		"https://host:10006/00000000-0000-0000-0000-000000000000/oauth2/v2.0/token", cfg.TokenEndpoint)
	assert.Equal(t,
		"https://host:10006/00000000-0000-0000-0000-000000000000/discovery/v2.0/keys", cfg.JWKSURI)
	assert.Contains(t, cfg.IDTokenSigningAlgs, "RS256")
	assert.NotEmpty(t, cfg.ResponseTypesSupp)
	assert.NotEmpty(t, cfg.SubjectTypesSupp)
	assert.NotEmpty(t, cfg.TokenEndpointAuthMeth)
}

func TestBuildInstanceDiscoveryResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		baseURL  string
		wantHost string
	}{
		{name: "https scheme stripped", baseURL: "https://host:10006", wantHost: "host:10006"},
		{name: "http scheme stripped", baseURL: "http://host:10006", wantHost: "host:10006"},
		{name: "no scheme unchanged", baseURL: "host:10006", wantHost: "host:10006"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := aadauth.BuildInstanceDiscoveryResponse(tt.baseURL, "tenant1")

			assert.Equal(t, tt.baseURL+"/tenant1/v2.0/.well-known/openid-configuration",
				resp.TenantDiscoveryEndpoint)
			assert.Len(t, resp.Metadata, 1)
			assert.Equal(t, tt.wantHost, resp.Metadata[0].PreferredNetwork)
			assert.Equal(t, tt.wantHost, resp.Metadata[0].PreferredCache)
			assert.Contains(t, resp.Metadata[0].Aliases, tt.wantHost)
		})
	}
}
