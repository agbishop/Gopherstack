package azurearm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azurearm"
)

// TestBuildMetadataEndpoints asserts the presence of every field
// hashicorp/go-azure-sdk's environments.FromEndpoint requires or reads --
// AZURE.md section 10.8 is explicit that missing name, resourceManagerEndpoint,
// or resourceIdentifiers.microsoftGraphResourceId is a hard failure in the
// real provider, and that graph/graphAudience/suffixes/authentication are
// all part of the schema FromEndpoint parses.
func TestBuildMetadataEndpoints(t *testing.T) {
	t.Parallel()

	settings := azurearm.DefaultSettings()
	docs := azurearm.BuildMetadataEndpoints("https://host:10006", settings)

	require.Len(t, docs, 1)

	doc := docs[0]

	// Every one of these must be non-empty: FromEndpoint hard-fails without them.
	assert.Equal(t, settings.Environment, doc.Name, "name must equal the configured environment")
	assert.NotEmpty(t, doc.ResourceManager)
	assert.NotEmpty(t, doc.ResourceManagerEndpoint)
	assert.NotEmpty(t, doc.ResourceIdentifiers.MicrosoftGraphResourceID)

	assert.NotEmpty(t, doc.Authentication.LoginEndpoint)
	assert.NotEmpty(t, doc.Authentication.Audiences)
	assert.Equal(t, settings.TenantID, doc.Authentication.Tenant)

	assert.NotEmpty(t, doc.Graph)
	assert.NotEmpty(t, doc.GraphAudience)
	assert.NotEmpty(t, doc.Gallery)
	assert.NotEmpty(t, doc.Portal)

	assert.NotEmpty(t, doc.Suffixes.Storage)
	assert.NotEmpty(t, doc.Suffixes.KeyVaultDNS)
	assert.NotEmpty(t, doc.Suffixes.SQLServerHostname)
	assert.NotEmpty(t, doc.Suffixes.ACRLoginServer)

	// Every URL should point back at gopherstack's own base URL.
	assert.Contains(t, doc.ResourceManager, "host:10006")
	assert.Contains(t, doc.Graph, "host:10006")
	assert.Contains(t, doc.Portal, "host:10006")
}

// TestBuildMetadataEndpoints_IPv6Host proves hostnameOnly correctly strips
// an IPv6 host's brackets (baseURLFor can produce "https://[::1]:10006"),
// rather than a naive first-colon scan returning just "[" (CodeRabbit-flagged).
func TestBuildMetadataEndpoints_IPv6Host(t *testing.T) {
	t.Parallel()

	settings := azurearm.DefaultSettings()
	docs := azurearm.BuildMetadataEndpoints("https://[::1]:10006", settings)

	require.Len(t, docs, 1)
	doc := docs[0]

	assert.NotContains(t, doc.Suffixes.KeyVaultDNS, "[")
	assert.NotContains(t, doc.Suffixes.SQLServerHostname, "[")
	assert.NotContains(t, doc.Suffixes.ACRLoginServer, "[")
	assert.Contains(t, doc.Suffixes.KeyVaultDNS, "::1")
}

func TestBuildMetadataEndpoints_CustomEnvironmentName(t *testing.T) {
	t.Parallel()

	settings := azurearm.DefaultSettings()
	settings.Environment = "my-custom-cloud"

	docs := azurearm.BuildMetadataEndpoints("https://host:10006", settings)

	require.Len(t, docs, 1)
	assert.Equal(t, "my-custom-cloud", docs[0].Name)
}
