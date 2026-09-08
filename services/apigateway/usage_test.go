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

// TestUpdateUsage_ValidatesUsagePlanExists verifies that UpdateUsage returns 404
// for an unknown usage plan. Real AWS returns NotFoundException.
func TestUpdateUsage_ValidatesUsagePlanExists(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()

	rec := restRequest(t, h, http.MethodPatch,
		"/usageplans/nonexistent-plan/keys/somekey/usage",
		`{"patchOperations":[{"op":"replace","path":"/remaining","value":"100"}]}`,
	)

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"UpdateUsage with unknown planId must return 404; body: %s", rec.Body.String())
}

// TestUpdateUsage_ValidatesKeyExists verifies that UpdateUsage returns 404 for an
// unknown key. Real AWS returns NotFoundException.
func TestUpdateUsage_ValidatesKeyExists(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()

	// Create a usage plan.
	rec := restRequest(t, h, http.MethodPost, "/usageplans",
		`{"name":"test-plan","throttle":{"rateLimit":100,"burstLimit":50}}`)
	require.True(t, rec.Code >= 200 && rec.Code < 300, "create plan: %s", rec.Body.String())

	var plan map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &plan))
	planID, _ := plan["id"].(string)
	require.NotEmpty(t, planID)

	rec = restRequest(t, h, http.MethodPatch,
		fmt.Sprintf("/usageplans/%s/keys/nonexistent-key/usage", planID),
		`{"patchOperations":[{"op":"replace","path":"/remaining","value":"100"}]}`,
	)

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"UpdateUsage with unknown keyId must return 404; body: %s", rec.Body.String())
}

func TestAPIGateway_UpdateUsage_RESTRoute(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)

	plan, err := backend.CreateUsagePlan(apigateway.CreateUsagePlanInput{Name: "plan"})
	require.NoError(t, err)

	key, err := backend.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "usage-key"})
	require.NoError(t, err)

	_, err = backend.CreateUsagePlanKey(apigateway.CreateUsagePlanKeyInput{
		UsagePlanID: plan.ID, KeyID: key.ID, KeyType: "API_KEY",
	})
	require.NoError(t, err)

	// "/remaining" is the real (and only) AWS-documented UpdateUsage patch
	// path (patch-operations.html's UpdateUsage table lists just this single
	// scalar field — there is no per-date path segment).
	rec := restCall(
		t, h, http.MethodPatch, "/usageplans/"+plan.ID+"/keys/"+key.ID+"/usage", "application/json",
		`[{"op":"replace","path":"/remaining","value":"42"}]`,
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	values, _ := resp["values"].(map[string]any)
	require.Contains(t, values, key.ID, "real wire key is \"values\", not \"items\"")

	// GetUsage must reflect the same override afterward.
	usage, err := backend.GetUsage(apigateway.GetUsageInput{UsagePlanID: plan.ID})
	require.NoError(t, err)
	require.Contains(t, usage.Items, key.ID)

	pair, ok := usage.Items[key.ID][0].([]int)
	require.True(t, ok, "usage.Items[keyID][0] must be a [used, remaining] pair")
	require.Equal(t, 42, pair[1], "PATCH /remaining must set the overridden remaining quota")
}

func TestAPIGateway_UpdateUsage_UnknownPlan(t *testing.T) {
	t.Parallel()

	backend := apigateway.NewInMemoryBackend()
	h := apigateway.NewHandler(backend)

	rec := restCall(
		t, h, http.MethodPatch, "/usageplans/nope/keys/also-nope/usage", "application/json",
		`[{"op":"replace","path":"/remaining","value":"42"}]`,
	)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestBackend_GetUsage_RealItems(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, err := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "a"})
	require.NoError(t, err)
	key, err := b.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "k", Enabled: true})
	require.NoError(t, err)

	plan, err := b.CreateUsagePlan(apigateway.CreateUsagePlanInput{
		Name:      "plan",
		Quota:     &apigateway.QuotaSettings{Limit: 100, Period: "DAY"},
		APIStages: []apigateway.APIStageAssociation{{RestAPIID: api.ID, Stage: "prod"}},
	})
	require.NoError(t, err)
	_, err = b.CreateUsagePlanKey(apigateway.CreateUsagePlanKeyInput{
		UsagePlanID: plan.ID, KeyID: key.ID, KeyType: "API_KEY",
	})
	require.NoError(t, err)

	// Meter three requests.
	for range 3 {
		require.NoError(t, b.EnforceUsagePlan(api.ID, "prod", key.ID))
	}

	usage, err := b.GetUsage(apigateway.GetUsageInput{UsagePlanID: plan.ID})
	require.NoError(t, err)
	require.Contains(t, usage.Items, key.ID)

	pair, ok := usage.Items[key.ID][0].([]int)
	require.True(t, ok, "usage item should be a [used, remaining] pair")
	assert.Equal(t, 3, pair[0], "used count should reflect metered requests")
	assert.Equal(t, 97, pair[1], "remaining should be limit minus used")
}

// TestGetUsage tests GetUsage.
func TestGetUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
		useValid bool
	}{
		{
			name:     "success",
			wantCode: http.StatusOK,
			useValid: true,
		},
		{
			name:     "plan_not_found",
			wantCode: http.StatusNotFound,
			useValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, e := boostSetup()

			// Create a usage plan
			planRec := postWithHandler(t, handler, e, "CreateUsagePlan", `{"name":"my-plan"}`)
			require.Equal(t, http.StatusCreated, planRec.Code)
			var planResp map[string]any
			require.NoError(t, json.Unmarshal(planRec.Body.Bytes(), &planResp))
			planID := planResp["id"].(string)

			lookupID := planID
			if !tt.useValid {
				lookupID = "notexist"
			}

			rec := postWithHandler(t, handler, e, "GetUsage",
				fmt.Sprintf(`{"usagePlanId":%q,"startDate":"2024-01-01","endDate":"2024-01-31"}`, lookupID))
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestDeleteUsagePlanKey_ClearsUsageOverride verifies that detaching an API
// key from a usage plan clears any UpdateUsage override recorded for that
// (plan, key) pair -- otherwise re-attaching the same key later would
// silently inherit the stale override, since neither ID is deleted.
func TestDeleteUsagePlanKey_ClearsUsageOverride(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()

	plan, err := b.CreateUsagePlan(apigateway.CreateUsagePlanInput{Name: "plan"})
	require.NoError(t, err)
	key, err := b.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "key", Enabled: true})
	require.NoError(t, err)
	_, err = b.CreateUsagePlanKey(apigateway.CreateUsagePlanKeyInput{
		UsagePlanID: plan.ID, KeyID: key.ID, KeyType: "API_KEY",
	})
	require.NoError(t, err)

	_, err = b.UpdateUsage(plan.ID, key.ID, map[string]string{"remaining": "5"})
	require.NoError(t, err)

	usage, err := b.GetUsage(apigateway.GetUsageInput{UsagePlanID: plan.ID})
	require.NoError(t, err)
	require.Equal(t, 5, usage.Items[key.ID][0].([]int)[1])

	require.NoError(t, b.DeleteUsagePlanKey(plan.ID, key.ID))
	_, err = b.CreateUsagePlanKey(apigateway.CreateUsagePlanKeyInput{
		UsagePlanID: plan.ID, KeyID: key.ID, KeyType: "API_KEY",
	})
	require.NoError(t, err)

	usage, err = b.GetUsage(apigateway.GetUsageInput{UsagePlanID: plan.ID})
	require.NoError(t, err)
	assert.NotEqual(t, 5, usage.Items[key.ID][0].([]int)[1],
		"re-attaching the same key must not inherit the detached association's stale usage override")
}

// TestDeleteUsagePlan_ClearsUsageOverrides verifies DeleteUsagePlan removes
// the plan's usageOverrides entry rather than leaking it in the persisted
// snapshot forever.
func TestDeleteUsagePlan_ClearsUsageOverrides(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()

	plan, err := b.CreateUsagePlan(apigateway.CreateUsagePlanInput{Name: "plan"})
	require.NoError(t, err)
	key, err := b.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "key", Enabled: true})
	require.NoError(t, err)
	_, err = b.CreateUsagePlanKey(apigateway.CreateUsagePlanKeyInput{
		UsagePlanID: plan.ID, KeyID: key.ID, KeyType: "API_KEY",
	})
	require.NoError(t, err)
	_, err = b.UpdateUsage(plan.ID, key.ID, map[string]string{"remaining": "5"})
	require.NoError(t, err)

	require.NoError(t, b.DeleteUsagePlan(plan.ID))

	var decoded struct {
		UsageOverrides map[string]map[string]int64 `json:"usageOverrides"`
	}
	require.NoError(t, json.Unmarshal(b.Snapshot(t.Context()), &decoded))
	assert.NotContains(t, decoded.UsageOverrides, plan.ID,
		"a deleted usage plan's override entry must not survive in the persisted snapshot")
}

// TestDeleteAPIKey_ClearsUsageOverrides verifies DeleteAPIKey removes any
// usageOverrides entries recorded for that key across all usage plans.
func TestDeleteAPIKey_ClearsUsageOverrides(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()

	plan, err := b.CreateUsagePlan(apigateway.CreateUsagePlanInput{Name: "plan"})
	require.NoError(t, err)
	key, err := b.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "key", Enabled: true})
	require.NoError(t, err)
	_, err = b.CreateUsagePlanKey(apigateway.CreateUsagePlanKeyInput{
		UsagePlanID: plan.ID, KeyID: key.ID, KeyType: "API_KEY",
	})
	require.NoError(t, err)
	_, err = b.UpdateUsage(plan.ID, key.ID, map[string]string{"remaining": "5"})
	require.NoError(t, err)

	require.NoError(t, b.DeleteAPIKey(key.ID))

	var decoded struct {
		UsageOverrides map[string]map[string]int64 `json:"usageOverrides"`
	}
	require.NoError(t, json.Unmarshal(b.Snapshot(t.Context()), &decoded))
	assert.NotContains(t, decoded.UsageOverrides[plan.ID], key.ID,
		"a deleted API key's usage override must not survive in the persisted snapshot")
}

// TestDeleteAPIKey_ClearsUsagePlanKeys verifies DeleteAPIKey cascades to the
// key's usagePlanKeys associations, so a deleted key no longer shows up in
// GetUsagePlanKeys or GetUsage (gopherstack-m7mb). A second, undeleted key on
// the same plan must be unaffected.
func TestDeleteAPIKey_ClearsUsagePlanKeys(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()

	plan, err := b.CreateUsagePlan(apigateway.CreateUsagePlanInput{Name: "plan"})
	require.NoError(t, err)
	deletedKey, err := b.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "deleted", Enabled: true})
	require.NoError(t, err)
	survivingKey, err := b.CreateAPIKey(apigateway.CreateAPIKeyInput{Name: "surviving", Enabled: true})
	require.NoError(t, err)
	_, err = b.CreateUsagePlanKey(apigateway.CreateUsagePlanKeyInput{
		UsagePlanID: plan.ID, KeyID: deletedKey.ID, KeyType: "API_KEY",
	})
	require.NoError(t, err)
	_, err = b.CreateUsagePlanKey(apigateway.CreateUsagePlanKeyInput{
		UsagePlanID: plan.ID, KeyID: survivingKey.ID, KeyType: "API_KEY",
	})
	require.NoError(t, err)

	require.NoError(t, b.DeleteAPIKey(deletedKey.ID))

	keys, err := b.GetUsagePlanKeys(plan.ID)
	require.NoError(t, err)
	ids := make([]string, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, k.ID)
	}
	assert.NotContains(t, ids, deletedKey.ID,
		"a deleted API key must not survive as a ghost row in GetUsagePlanKeys")
	assert.Contains(t, ids, survivingKey.ID,
		"deleting one key must not disturb another key's usage-plan association")

	usage, err := b.GetUsage(apigateway.GetUsageInput{UsagePlanID: plan.ID})
	require.NoError(t, err)
	assert.NotContains(t, usage.Items, deletedKey.ID,
		"a deleted API key must not survive as a ghost row in GetUsage")
	assert.Contains(t, usage.Items, survivingKey.ID,
		"deleting one key must not disturb another key's usage entry")

	_, err = b.GetUsagePlanKey(plan.ID, deletedKey.ID)
	require.Error(t, err, "the deleted key's usage-plan association must be gone")
}

// TestDeleteStage_ClearsMethodThrottleBuckets verifies DeleteStage evicts the stage's
// MethodSettings token buckets (gopherstack-91f2), so a stage name reused after deletion
// starts with a fresh bucket instead of inheriting an already-exhausted one -- the same
// ghost-row class as an orphaned usage-plan association, but for the new per-stage
// throttle state introduced by this fix.
func TestDeleteStage_ClearsMethodThrottleBuckets(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, err := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "a"})
	require.NoError(t, err)
	_, err = b.CreateDeployment(api.ID, "prod", "v1")
	require.NoError(t, err)
	_, err = b.UpdateStage(api.ID, "prod", apigateway.UpdateStageInput{
		MethodSettings: map[string]apigateway.MethodSetting{
			"*/*": {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 1},
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.EnforceMethodThrottle(api.ID, "prod", "/items", "GET"))
	require.ErrorIs(t, b.EnforceMethodThrottle(api.ID, "prod", "/items", "GET"), apigateway.ErrThrottled,
		"the burst-1 bucket must be exhausted after one request")

	require.NoError(t, b.DeleteStage(api.ID, "prod"))

	_, err = b.CreateDeployment(api.ID, "prod", "v2")
	require.NoError(t, err)
	_, err = b.UpdateStage(api.ID, "prod", apigateway.UpdateStageInput{
		MethodSettings: map[string]apigateway.MethodSetting{
			"*/*": {ThrottlingRateLimit: 1, ThrottlingBurstLimit: 1},
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.EnforceMethodThrottle(api.ID, "prod", "/items", "GET"),
		"a recreated stage must not inherit a deleted stage's exhausted throttle bucket")
}
