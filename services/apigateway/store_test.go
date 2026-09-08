package apigateway_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigateway"
)

// TestAPIGW_BackendReset covers Reset.
func TestAPIGW_BackendReset(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	b.Reset()
}

// TestAPIGW_BackendReset_Account covers Reset restoring the account settings
// mutated by UpdateAccount back to their construction-time defaults.
func TestAPIGW_BackendReset_Account(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()

	before, err := b.GetAccount()
	require.NoError(t, err)

	_, err = b.UpdateAccount(apigateway.UpdateAccountInput{
		ThrottleSettings:  &apigateway.ThrottleSettings{BurstLimit: 1, RateLimit: 1},
		CloudwatchRoleARN: "arn:aws:iam::000000000000:role/mutated",
		Features:          []string{"UsagePlans", "ExtraFeature"},
	})
	require.NoError(t, err)

	b.Reset()

	after, err := b.GetAccount()
	require.NoError(t, err)

	assert.Equal(t, before.ThrottleSettings, after.ThrottleSettings, "throttle settings")
	assert.Equal(t, before.CloudwatchRoleARN, after.CloudwatchRoleARN, "cloudwatch role arn")
	assert.Equal(t, before.Features, after.Features, "features")
	assert.Empty(t, after.CloudwatchRoleARN, "cloudwatch role arn should reset to empty")
}

// TestPaginatedListing tests GetAPIKeysPage, GetDomainNamesPage, GetUsagePlansPage (via handler).
func TestPaginatedListing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		action       string
		setupAction  string
		setupBodies  []string
		wantCode     int
		wantMinItems int
	}{
		{
			name:        "api_keys_paginated",
			action:      "GetApiKeys",
			setupAction: "CreateApiKey",
			setupBodies: []string{
				`{"name":"key-a","enabled":true}`,
				`{"name":"key-b","enabled":true}`,
			},
			wantCode:     http.StatusOK,
			wantMinItems: 2,
		},
		{
			name:        "domain_names_paginated",
			action:      "GetDomainNames",
			setupAction: "CreateDomainName",
			setupBodies: []string{
				`{"domainName":"a.example.com"}`,
				`{"domainName":"b.example.com"}`,
			},
			wantCode:     http.StatusOK,
			wantMinItems: 2,
		},
		{
			name:        "usage_plans_paginated",
			action:      "GetUsagePlans",
			setupAction: "CreateUsagePlan",
			setupBodies: []string{
				`{"name":"plan-a"}`,
				`{"name":"plan-b"}`,
			},
			wantCode:     http.StatusOK,
			wantMinItems: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, e := boostSetup()
			for _, body := range tt.setupBodies {
				postWithHandler(t, handler, e, tt.setupAction, body)
			}

			// Request with limit=1 to exercise pagination logic
			rec := postWithHandler(t, handler, e, tt.action, `{"limit":1}`)
			assert.Equal(t, tt.wantCode, rec.Code)

			// Also request all to verify total count
			rec2 := postWithHandler(t, handler, e, tt.action, `{}`)
			assert.Equal(t, tt.wantCode, rec2.Code)
			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
			items, _ := resp["item"].([]any)
			assert.GreaterOrEqual(t, len(items), tt.wantMinItems)
		})
	}
}

// TestReset tests the Handler Reset method.
func TestReset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "reset_clears_state"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, e := boostSetup()
			apiID := boostAPI(t, handler, e)

			// Confirm API exists
			rec := postWithHandler(t, handler, e, "GetRestApi",
				fmt.Sprintf(`{"restApiId":%q}`, apiID))
			assert.Equal(t, http.StatusOK, rec.Code)

			// Reset the handler
			handler.Reset()

			// After reset, the API should be gone
			rec2 := postWithHandler(t, handler, e, "GetRestApi",
				fmt.Sprintf(`{"restApiId":%q}`, apiID))
			assert.Equal(t, http.StatusNotFound, rec2.Code)
		})
	}
}

// TestOpaquePagination_EdgeCases verifies that GetRestAPIs treats malformed/legacy
// (numeric) position tokens as "start from the beginning" — the opaque cursor is not a
// numeric offset — and that a real cursor round-trips to the next page.
func TestOpaquePagination_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		position string
		wantLen  int
	}{
		{
			name:     "invalid_position_string_treated_as_start",
			position: "not-a-number",
			wantLen:  2,
		},
		{
			name:     "legacy_numeric_position_treated_as_start",
			position: "1",
			wantLen:  2,
		},
		{
			name:     "empty_position_returns_all",
			position: "",
			wantLen:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := apigateway.NewInMemoryBackend()
			_, _ = b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "api-a"})
			_, _ = b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "api-b"})

			apis, _, err := b.GetRestAPIs(0, tt.position)
			require.NoError(t, err)
			assert.Len(t, apis, tt.wantLen)
		})
	}
}

// TestOpaquePagination_RoundTrip verifies that the opaque cursor returned by a limited
// page resumes at the correct item on the next call and is not a numeric offset.
func TestOpaquePagination_RoundTrip(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	for _, name := range []string{"api-a", "api-b", "api-c"} {
		_, err := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: name})
		require.NoError(t, err)
	}

	first, token, err := b.GetRestAPIs(1, "")
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.NotEmpty(t, token, "cursor must be present when more pages remain")
	assert.NotEqual(t, "1", token, "cursor must be opaque, not a numeric offset")

	// Walk the remaining pages using the opaque cursor.
	seen := map[string]bool{first[0].ID: true}
	for token != "" {
		var page []apigateway.RestAPI
		page, token, err = b.GetRestAPIs(1, token)
		require.NoError(t, err)
		for _, api := range page {
			assert.False(t, seen[api.ID], "cursor must not repeat an item")
			seen[api.ID] = true
		}
	}
	assert.Len(t, seen, 3, "cursor pagination must cover every item exactly once")
}
