package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/aadauth"
	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

func TestIssueToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		scope         string
		wantAudSuffix string
	}{
		{name: "v2 scope with .default suffix", scope: "https://management.azure.com/.default",
			wantAudSuffix: "https://management.azure.com/"},
		{name: "v1 resource without .default", scope: "https://management.azure.com/",
			wantAudSuffix: "https://management.azure.com/"},
		{name: "empty scope uses default audience", scope: "", wantAudSuffix: "https://management.azure.com/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			issuer, err := aadauth.NewIssuer()
			require.NoError(t, err)

			resp, err := azurearm.IssueToken(issuer, "https://host:10006", "tenant1", "client1", tt.scope)
			require.NoError(t, err)

			assert.Equal(t, "Bearer", resp.TokenType)
			assert.Equal(t, aadauth.DefaultTokenExpirySeconds, resp.ExpiresIn)
			assert.NotEmpty(t, resp.AccessToken)

			claims, err := issuer.Parse(resp.AccessToken)
			require.NoError(t, err)
			assert.Equal(t, tt.wantAudSuffix, claims["aud"])
			assert.Equal(t, "tenant1", claims["tid"])
			assert.Equal(t, "client1", claims["appid"])
			assert.Equal(t, "2.0", claims["ver"])
		})
	}
}
