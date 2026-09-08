package integration_test

// This file deliberately does NOT use an azure-sdk-for-go ARM SDK client
// (e.g. sdk/resourcemanager/resources/armresources or
// sdk/resourcemanager/storage/armstorage): those clients' auth pipeline is
// built around azidentity's credential chain, which -- to be pointed at a
// custom ARM endpoint at all -- requires either a custom cloud.Configuration
// (itself requiring the exact environment-descriptor shape
// services/azurearm/metadata.go serves, plus a client transport that trusts
// the self-signed certificate) or bypassing auth entirely via a
// policy.ClientOptions{Cloud: ...} override that still expects TLS trust to
// be configured at the http.Client level. Standing up that pipeline just for
// an integration test duplicates most of what test/terraform/azure's
// hashicorp/azurerm run already proves end-to-end against a real Terraform
// provider. Mirroring test/integration/azureservicebus_test.go's own
// documented choice (that package is AMQP-only and has no REST transport),
// this file exercises the ARM REST surface directly via net/http with
// InsecureSkipVerify -- a real, if less turnkey, integration test of the
// exact wire behavior azurerm and other REST-based ARM clients depend on.

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// armInsecureClient is an *http.Client that trusts azureARMEndpoint's
// self-signed certificate (AZURE.md section 10.8) -- production clients
// instead rely on the system trust store plus SSL_CERT_FILE, as
// test/terraform/azure's harness does for the real hashicorp/azurerm
// provider.
func armInsecureClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
}

// armRequest performs one ARM REST call against azureARMEndpoint and returns
// the response, skipping the test if the endpoint isn't available (mirroring
// sbRequest's t.Skip pattern in azureservicebus_test.go).
func armRequest(t *testing.T, method, path string, body []byte) *http.Response {
	t.Helper()

	if azureARMEndpoint == "" {
		t.Skip("Azure ARM endpoint not available (mapped port could not be determined)")
	}

	var bodyReader *strings.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	} else {
		bodyReader = strings.NewReader("")
	}

	req, err := http.NewRequestWithContext(t.Context(), method, azureARMEndpoint+path, bodyReader)
	require.NoError(t, err)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := armInsecureClient().Do(req)
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp
}

func armDecodeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	return body
}

// TestIntegration_AzureARM_MetadataAndToken proves the discovery documents
// that terraform-provider-azurerm's provider initialization depends on are
// reachable over the real HTTPS listener, and that the client-credentials
// token endpoint issues a usable bearer token.
func TestIntegration_AzureARM_MetadataAndToken(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	resp := armRequest(t, http.MethodGet, "/metadata/endpoints?api-version=2022-09-01", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var docs []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&docs))
	require.Len(t, docs, 1)
	assert.Equal(t, "gopherstack", docs[0]["name"])
	assert.NotEmpty(t, docs[0]["resourceManagerEndpoint"])

	tenant := "00000000-0000-0000-0000-000000000000"

	resp = armRequest(t, http.MethodGet, "/"+tenant+"/v2.0/.well-known/openid-configuration", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	form := "grant_type=client_credentials&client_id=" + tenant +
		"&client_secret=gopherstack&scope=https://management.azure.com/.default"

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		azureARMEndpoint+"/"+tenant+"/oauth2/v2.0/token", strings.NewReader(form))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	tokenResp, err := armInsecureClient().Do(req)
	require.NoError(t, err)

	defer tokenResp.Body.Close()
	require.Equal(t, http.StatusOK, tokenResp.StatusCode)

	tokenBody := armDecodeJSON(t, tokenResp)
	assert.Equal(t, "Bearer", tokenBody["token_type"])
	assert.NotEmpty(t, tokenBody["access_token"])
}

// TestIntegration_AzureARM_ResourceGroupAndStorageAccountLifecycle drives
// the exact CRUD sequence terraform-provider-azurerm's azurerm_resource_group
// + azurerm_storage_account resources perform, end to end against a running
// gopherstack instance: create the resource group, create a storage account
// in it, fetch listKeys, then delete both.
func TestIntegration_AzureARM_ResourceGroupAndStorageAccountLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	sub := "00000000-0000-0000-0000-000000000000"
	rg := "test-rg-" + uuid.NewString()[:8]
	acct := "acct" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

	base := "/subscriptions/" + sub

	// Create resource group.
	resp := armRequest(t, http.MethodPut, base+"/resourcegroups/"+rg, []byte(`{"location":"local"}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	rgBody := armDecodeJSON(t, resp)
	assert.Equal(t, rg, rgBody["name"])

	// Create storage account.
	resourcePath := base + "/resourceGroups/" + rg + "/providers/Microsoft.Storage/storageAccounts/" + acct

	resp = armRequest(t, http.MethodPut, resourcePath,
		[]byte(`{"location":"local","sku":{"name":"Standard_LRS"},"kind":"StorageV2"}`))
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	acctBody := armDecodeJSON(t, resp)
	assert.Equal(t, acct, acctBody["name"])

	props, ok := acctBody["properties"].(map[string]any)
	require.True(t, ok)

	endpoints, ok := props["primaryEndpoints"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, endpoints["blob"], acct)

	// Get storage account.
	resp = armRequest(t, http.MethodGet, resourcePath, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// listKeys.
	resp = armRequest(t, http.MethodPost, resourcePath+"/listKeys", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	keysBody := armDecodeJSON(t, resp)
	keys, ok := keysBody["keys"].([]any)
	require.True(t, ok)
	assert.Len(t, keys, 2)

	// Delete storage account, then resource group.
	resp = armRequest(t, http.MethodDelete, resourcePath, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp = armRequest(t, http.MethodDelete, base+"/resourcegroups/"+rg, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
