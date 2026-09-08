package appsync_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/appsync"
)

func TestPutGraphqlAPIEnvironmentVariables_ReplacesAll(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.PutGraphqlAPIEnvironmentVariables(api.APIID, map[string]string{
		"OLD_VAR": "old_value",
	})
	require.NoError(t, err)

	_, err = b.PutGraphqlAPIEnvironmentVariables(api.APIID, map[string]string{
		"NEW_VAR": "new_value",
	})
	require.NoError(t, err)

	got, err := b.GetGraphqlAPIEnvironmentVariables(api.APIID)
	require.NoError(t, err)
	_, hasOld := got["OLD_VAR"]
	assert.False(t, hasOld)
	assert.Equal(t, "new_value", got["NEW_VAR"])
}

func TestDeleteGraphqlAPI_PrivateAPI(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("PrivateAPI", appsync.AuthTypeAPIKey, false, "", "PRIVATE", nil, nil, nil)
	require.NoError(t, err)

	err = b.DeleteGraphqlAPI(api.APIID)
	require.NoError(t, err)

	_, err = b.GetGraphqlAPI(api.APIID)
	require.Error(t, err)
	assert.ErrorIs(t, err, appsync.ErrNotFound)
}

func TestInMemoryBackend_CreateGraphqlAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		apiName  string
		authType appsync.AuthenticationType
		wantName string
		wantAuth appsync.AuthenticationType
		wantErr  bool
	}{
		{
			name:     "creates_api_with_api_key_auth",
			apiName:  "MyAPI",
			authType: appsync.AuthTypeAPIKey,
			wantName: "MyAPI",
			wantAuth: appsync.AuthTypeAPIKey,
		},
		{
			name:     "creates_api_with_iam_auth",
			apiName:  "IAMApi",
			authType: appsync.AuthTypeIAM,
			wantName: "IAMApi",
			wantAuth: appsync.AuthTypeIAM,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			api, err := b.CreateGraphqlAPI(tt.apiName, tt.authType, false, "", "", nil, nil, nil)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, api.Name)
			assert.Equal(t, tt.wantAuth, api.AuthenticationType)
			assert.NotEmpty(t, api.APIID)
			assert.NotEmpty(t, api.ARN)
			assert.Contains(t, api.URIs["GRAPHQL"], api.APIID)
		})
	}
}

func TestInMemoryBackend_GetGraphqlAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(*appsync.InMemoryBackend) string
		apiID   string
		wantErr bool
	}{
		{
			name: "returns_existing_api",
			setup: func(b *appsync.InMemoryBackend) string {
				api, _ := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)

				return api.APIID
			},
			wantErr: false,
		},
		{
			name:    "returns_not_found_for_missing_api",
			setup:   func(_ *appsync.InMemoryBackend) string { return "nonexistent" },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			apiID := tt.setup(b)
			api, err := b.GetGraphqlAPI(apiID)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, awserr.ErrNotFound)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, apiID, api.APIID)
		})
	}
}

func TestInMemoryBackend_ListGraphqlAPIs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(*appsync.InMemoryBackend)
		name      string
		wantCount int
	}{
		{
			name:      "empty_list",
			setup:     func(_ *appsync.InMemoryBackend) {},
			wantCount: 0,
		},
		{
			name: "returns_all_apis",
			setup: func(b *appsync.InMemoryBackend) {
				_, _ = b.CreateGraphqlAPI("API1", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
				_, _ = b.CreateGraphqlAPI("API2", appsync.AuthTypeIAM, false, "", "", nil, nil, nil)
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			tt.setup(b)
			apis, err := b.ListGraphqlAPIs("")
			require.NoError(t, err)
			assert.Len(t, apis, tt.wantCount)
		})
	}
}

func TestInMemoryBackend_DeleteGraphqlAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		apiID   string
		wantErr bool
	}{
		{
			name:    "deletes_existing_api",
			wantErr: false,
		},
		{
			name:    "error_for_missing_api",
			apiID:   "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			apiID := tt.apiID

			if apiID == "" {
				api, _ := b.CreateGraphqlAPI("ToDelete", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
				apiID = api.APIID
			}

			err := b.DeleteGraphqlAPI(apiID)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, awserr.ErrNotFound)

				return
			}

			require.NoError(t, err)

			_, getErr := b.GetGraphqlAPI(apiID)
			require.Error(t, getErr)
		})
	}
}

func TestInMemoryBackend_CreateGraphqlAPI_InvalidAuthType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		authType appsync.AuthenticationType
		wantErr  bool
	}{
		{name: "valid_api_key", authType: appsync.AuthTypeAPIKey},
		{name: "valid_iam", authType: appsync.AuthTypeIAM},
		{name: "valid_cognito", authType: appsync.AuthTypeCognito},
		{name: "valid_oidc", authType: appsync.AuthTypeOIDC},
		{name: "valid_lambda", authType: appsync.AuthTypeLambda},
		{name: "empty_defaults_to_api_key", authType: ""},
		{name: "invalid_type_rejected", authType: "INVALID_AUTH", wantErr: true},
		{name: "invalid_type_basic", authType: "BASIC", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			api, err := b.CreateGraphqlAPI("TestAPI", tt.authType, false, "", "", nil, nil, nil)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, api)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, api)
		})
	}
}

func TestInMemoryBackend_DeleteGraphqlAPI_CascadeDelete(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	apiID := api.APIID

	// Create sub-resources.
	_, err = b.CreateAPIKey(apiID, "test key", 0)
	require.NoError(t, err)

	_, err = b.CreateAPICache(apiID, &appsync.APICache{
		TTL:                60,
		Type:               "SMALL",
		APICachingBehavior: "FULL_REQUEST_CACHING",
	})
	require.NoError(t, err)

	_, err = b.CreateFunction(apiID, &appsync.Function{
		Name:           "TestFn",
		DataSourceName: "MyDS",
	})
	require.NoError(t, err)

	// Delete the API.
	err = b.DeleteGraphqlAPI(apiID)
	require.NoError(t, err)

	// Verify sub-resources were cascade deleted by trying to create a new API key
	// (which would need the api to exist) — and verifying it fails.
	_, err = b.CreateAPIKey(apiID, "test", 0)
	require.Error(t, err)
}

func TestInMemoryBackend_DeleteGraphqlAPI_CascadeDelete_SourceAPIAssociations(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	src, err := b.CreateGraphqlAPI("Src", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	merged, err := b.CreateGraphqlAPI("Merged", appsync.AuthTypeAPIKey, false, "MERGED", "", nil, nil, nil)
	require.NoError(t, err)

	assoc, err := b.AssociateSourceGraphqlAPI(merged.APIID, src.APIID, "desc", "")
	require.NoError(t, err)

	require.NoError(t, b.DeleteGraphqlAPI(src.APIID))

	_, err = b.GetSourceAPIAssociation(merged.APIID, assoc.AssociationID)
	require.ErrorIs(t, err, awserr.ErrNotFound)

	assocs, err := b.ListSourceAPIAssociations(merged.APIID)
	require.NoError(t, err)
	assert.Empty(t, assocs)
}

func TestInMemoryBackend_DeleteGraphqlAPI_CascadeDelete_DomainNameAssociation(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateDomainName("api.example.com", "cert-arn", "", nil)
	require.NoError(t, err)

	_, err = b.AssociateAPI("api.example.com", api.APIID)
	require.NoError(t, err)

	require.NoError(t, b.DeleteGraphqlAPI(api.APIID))

	_, err = b.GetAPIAssociation("api.example.com")
	require.ErrorIs(t, err, awserr.ErrNotFound)

	dn, err := b.GetDomainName("api.example.com")
	require.NoError(t, err)
	assert.Empty(t, dn.APIID)
}

func TestInMemoryBackend_UpdateGraphqlAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		newName  string
		authType appsync.AuthenticationType
		wantErr  bool
	}{
		{name: "update_name", newName: "NewName", authType: ""},
		{name: "update_auth_type", newName: "", authType: appsync.AuthTypeIAM},
		{name: "update_both", newName: "Updated", authType: appsync.AuthTypeCognito},
		{name: "invalid_auth_type", newName: "", authType: "INVALID", wantErr: true},
		{name: "no_change", newName: "", authType: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			api, err := b.CreateGraphqlAPI("OriginalName", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
			require.NoError(t, err)

			updated, err := b.UpdateGraphqlAPI(api.APIID, tt.newName, tt.authType, nil, "", nil, nil)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			if tt.newName != "" {
				assert.Equal(t, tt.newName, updated.Name)
			} else {
				assert.Equal(t, "OriginalName", updated.Name)
			}
		})
	}
}

func TestInMemoryBackend_CreateAndUpdateGraphqlAPI_OwnerContact(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	created, err := b.CreateGraphqlAPI(
		"OwnerContactAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil,
		&appsync.GraphqlAPIConfig{OwnerContact: "team-a@example.com"},
	)
	require.NoError(t, err)
	assert.Equal(t, "team-a@example.com", created.OwnerContact)

	fetched, err := b.GetGraphqlAPI(created.APIID)
	require.NoError(t, err)
	assert.Equal(t, "team-a@example.com", fetched.OwnerContact)

	updated, err := b.UpdateGraphqlAPI(
		created.APIID, "", "", nil, "", nil,
		&appsync.GraphqlAPIConfig{OwnerContact: "team-b@example.com"},
	)
	require.NoError(t, err)
	assert.Equal(t, "team-b@example.com", updated.OwnerContact)
}

func TestInMemoryBackend_EnvironmentVariables(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	// Get empty env vars.
	envVars, err := b.GetGraphqlAPIEnvironmentVariables(api.APIID)
	require.NoError(t, err)
	assert.Empty(t, envVars)

	// Put env vars.
	envVars2, err := b.PutGraphqlAPIEnvironmentVariables(api.APIID, map[string]string{"K1": "V1", "K2": "V2"})
	require.NoError(t, err)
	assert.Equal(t, "V1", envVars2["K1"])

	// Get returns updated vars.
	envVars3, err := b.GetGraphqlAPIEnvironmentVariables(api.APIID)
	require.NoError(t, err)
	assert.Equal(t, "V2", envVars3["K2"])

	// Put replaces all vars.
	_, err = b.PutGraphqlAPIEnvironmentVariables(api.APIID, map[string]string{"NEW_KEY": "new_val"})
	require.NoError(t, err)

	envVars4, err := b.GetGraphqlAPIEnvironmentVariables(api.APIID)
	require.NoError(t, err)
	assert.NotContains(t, envVars4, "K1")
	assert.Equal(t, "new_val", envVars4["NEW_KEY"])

	// Not found API returns error.
	_, err = b.GetGraphqlAPIEnvironmentVariables("nonexistent")
	require.ErrorIs(t, err, awserr.ErrNotFound)

	_, err = b.PutGraphqlAPIEnvironmentVariables("nonexistent", map[string]string{})
	require.ErrorIs(t, err, awserr.ErrNotFound)
}

func TestInMemoryBackend_CreateGraphqlAPI_XrayEnabled(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, true, "GRAPHQL", "", nil, nil, nil)
	require.NoError(t, err)
	assert.True(t, api.XrayEnabled)
	assert.Equal(t, "GRAPHQL", api.APIType)
	assert.NotZero(t, api.CreatedAt)
	assert.NotZero(t, api.UpdatedAt)
	assert.Equal(t, api.CreatedAt, api.UpdatedAt)
}

func TestInMemoryBackend_CreateGraphqlAPI_APIType_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		apiType string
		wantErr bool
	}{
		{name: "default_graphql", apiType: "", wantErr: false},
		{name: "explicit_graphql", apiType: "GRAPHQL", wantErr: false},
		{name: "merged_type", apiType: "MERGED", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, tt.apiType, "", nil, nil, nil)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.NotNil(t, api)
		})
	}
}

func TestInMemoryBackend_UpdateGraphqlAPI_XrayEnabled(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("TestAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)
	assert.False(t, api.XrayEnabled)

	xrayEnabled := true
	updated, err := b.UpdateGraphqlAPI(api.APIID, "", "", &xrayEnabled, "", nil, nil)
	require.NoError(t, err)
	assert.True(t, updated.XrayEnabled)
	assert.GreaterOrEqual(t, updated.UpdatedAt, api.UpdatedAt)
}

func TestInMemoryBackend_ListGraphqlAPIs_TypeFilter(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	_, err := b.CreateGraphqlAPI("GraphQL1", appsync.AuthTypeAPIKey, false, "GRAPHQL", "", nil, nil, nil)
	require.NoError(t, err)
	_, err = b.CreateGraphqlAPI("Merged1", appsync.AuthTypeAPIKey, false, "MERGED", "", nil, nil, nil)
	require.NoError(t, err)

	// Filter by GRAPHQL.
	graphqlAPIs, err := b.ListGraphqlAPIs("GRAPHQL")
	require.NoError(t, err)
	assert.Len(t, graphqlAPIs, 1)
	assert.Equal(t, "GraphQL1", graphqlAPIs[0].Name)

	// Filter by MERGED.
	mergedAPIs, err := b.ListGraphqlAPIs("MERGED")
	require.NoError(t, err)
	assert.Len(t, mergedAPIs, 1)
	assert.Equal(t, "Merged1", mergedAPIs[0].Name)

	// No filter returns all.
	allAPIs, err := b.ListGraphqlAPIs("")
	require.NoError(t, err)
	assert.Len(t, allAPIs, 2)
}

func TestInMemoryBackend_ListGraphqlAPIs_Sorted(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	for _, name := range []string{"Zebra", "Alpha", "Mango"} {
		_, err := b.CreateGraphqlAPI(name, appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
		require.NoError(t, err)
	}

	apis, err := b.ListGraphqlAPIs("")
	require.NoError(t, err)
	require.Len(t, apis, 3)
	assert.Equal(t, "Alpha", apis[0].Name)
	assert.Equal(t, "Mango", apis[1].Name)
	assert.Equal(t, "Zebra", apis[2].Name)
}

func TestInMemoryBackend_PutEnvVars_ExceedsLimit(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("MyAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	// Build a map with 26 entries (exceeds max of 25).
	envVars := make(map[string]string)
	for i := range 26 {
		envVars[fmt.Sprintf("KEY_%d", i)] = "value"
	}

	_, err = b.PutGraphqlAPIEnvironmentVariables(api.APIID, envVars)
	require.Error(t, err)
	assert.ErrorIs(t, err, appsync.ErrValidation)
}

func TestInMemoryBackend_PutEnvVars_MaxAllowed(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	api, err := b.CreateGraphqlAPI("MyAPI", appsync.AuthTypeAPIKey, false, "", "", nil, nil, nil)
	require.NoError(t, err)

	// Build a map with exactly 25 entries.
	envVars := make(map[string]string)
	for i := range 25 {
		envVars[fmt.Sprintf("KEY_%d", i)] = "value"
	}

	out, err := b.PutGraphqlAPIEnvironmentVariables(api.APIID, envVars)
	require.NoError(t, err)
	assert.Len(t, out, 25)
}
