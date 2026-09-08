package securityhub_test

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/securityhub"
)

// Batch-1 accuracy gap: EnableSecurityHub is POST /accounts (not POST /hub or POST /securityhub).
func TestEnableSecurityHubPath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/accounts", map[string]any{
		"EnableDefaultStandards": true,
	})

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	// EnableSecurityHub returns an empty object, not a hub object
	assert.NotContains(t, resp, "HubArn", "EnableSecurityHub must return empty body")
}

// Batch-1 accuracy gap: DescribeHub is GET /accounts (not GET /hub).
func TestDescribeHubIsGETAccounts(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	// First enable
	doRequest(t, h, http.MethodPost, "/accounts", nil)

	rec := doRequest(t, h, http.MethodGet, "/accounts", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Contains(t, resp, "HubArn")
	assert.Contains(t, resp, "SubscribedAt")
	assert.Contains(t, resp, "AutoEnableControls")
	assert.Contains(t, resp, "AutoEnableStandards")
	assert.Contains(t, resp, "ControlFindingGenerator")

	hubArn, _ := resp["HubArn"].(string)
	assert.Contains(t, hubArn, "arn:aws:securityhub:")
	assert.Contains(t, hubArn, ":hub/default")
}

// Batch-1 accuracy gap: DescribeHub when not enabled returns 400 (not 404).
func TestDescribeHubNotEnabledReturns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/accounts", nil)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// Batch-1 accuracy gap: DisableSecurityHub is DELETE /accounts.
func TestDisableSecurityHubPath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/accounts", nil)

	rec := doRequest(t, h, http.MethodDelete, "/accounts", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// After disable, DescribeHub should return 400
	rec2 := doRequest(t, h, http.MethodGet, "/accounts", nil)
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

// Batch-1 accuracy gap: UpdateSecurityHubConfiguration is PATCH /accounts.
func TestUpdateSecurityHubConfigurationIsPATCHAccounts(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, http.MethodPost, "/accounts", nil)

	rec := doRequest(t, h, http.MethodPatch, "/accounts", map[string]any{
		"AutoEnableControls":      false,
		"AutoEnableStandards":     "NONE",
		"ControlFindingGenerator": "STANDARD_CONTROL",
	})

	require.Equal(t, http.StatusOK, rec.Code)

	// Verify the change
	rec2 := doRequest(t, h, http.MethodGet, "/accounts", nil)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))

	assert.Equal(t, false, resp["AutoEnableControls"])
	assert.Equal(t, "NONE", resp["AutoEnableStandards"])
	assert.Equal(t, "STANDARD_CONTROL", resp["ControlFindingGenerator"])
}

// Batch-1 accuracy gap: HubArn format is arn:aws:securityhub:{region}:{account}:hub/default.
func TestHubArnFormat(t *testing.T) {
	t.Parallel()

	b := securityhub.NewInMemoryBackend("123456789012", "eu-west-1")
	h := securityhub.NewHandler(b)

	doRequest(t, h, http.MethodPost, "/accounts", nil)

	rec := doRequest(t, h, http.MethodGet, "/accounts", nil)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	hubArn, _ := resp["HubArn"].(string)
	assert.Equal(t, "arn:aws:securityhub:eu-west-1:123456789012:hub/default", hubArn)
}

func TestHubV2(t *testing.T) {
	t.Parallel()

	type step struct {
		body   any
		check  func(t *testing.T, code int, resp map[string]any)
		name   string
		method string
		path   string
	}

	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "EnableDescribeDisableSecurityHubV2",
			steps: []step{
				{
					name:   "describe before enable returns 404",
					method: http.MethodGet,
					path:   "/hubv2",
					body:   nil,
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusNotFound, code)
					},
				},
				{
					name:   "enable",
					method: http.MethodPost,
					path:   "/hubv2",
					body:   map[string]any{},
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
					},
				},
				{
					name:   "enable again returns conflict",
					method: http.MethodPost,
					path:   "/hubv2",
					body:   map[string]any{},
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusConflict, code)
					},
				},
				{
					name:   "describe returns hub",
					method: http.MethodGet,
					path:   "/hubv2",
					body:   nil,
					check: func(t *testing.T, code int, resp map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
						assert.NotEmpty(t, resp["HubV2Arn"])
						assert.Contains(t, resp["HubV2Arn"].(string), "hub-v2/default")
					},
				},
				{
					name:   "disable",
					method: http.MethodDelete,
					path:   "/hubv2",
					body:   nil,
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
					},
				},
				{
					name:   "describe after disable returns 404",
					method: http.MethodGet,
					path:   "/hubv2",
					body:   nil,
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusNotFound, code)
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			for _, s := range tc.steps {
				rec := doRequest(t, h, s.method, s.path, s.body)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				s.check(t, rec.Code, resp)
			}
		})
	}
}

func TestHandler_DisableSecurityHubV2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		preEnable bool
		wantCode  int
	}{
		{
			name:      "disable after enable succeeds",
			preEnable: true,
			wantCode:  http.StatusOK,
		},
		{
			name:      "disable without enable returns 400",
			preEnable: false,
			wantCode:  http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			if tc.preEnable {
				doRequest(t, h, http.MethodPost, "/hubv2", map[string]any{})
			}

			rec := doRequest(t, h, http.MethodDelete, "/hubv2", nil)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

// TestHubV2Features covers EnableSecurityHubFeatureV2/
// DisableSecurityHubFeatureV2 (/hubv2/feature/{FeatureName}) and their
// effect on DescribeSecurityHubV2's Features map. Per the real API's
// documented behavior ("The service must be enabled before you can enable a
// feature"), these require SecurityHub V2 to already be enabled -- there is
// no independent feature-enablement state.
func TestHubV2Features(t *testing.T) {
	t.Parallel()

	type step struct {
		body   any
		check  func(t *testing.T, code int, resp map[string]any)
		name   string
		method string
		path   string
	}

	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "EnableDisableSecurityHubFeatureV2",
			steps: []step{
				{
					name:   "enable feature before hub v2 enabled returns 404",
					method: http.MethodPost,
					path:   "/hubv2/feature/NETWORK_SCANNING",
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusNotFound, code)
					},
				},
				{
					name:   "enable hub v2",
					method: http.MethodPost,
					path:   "/hubv2",
					body:   map[string]any{},
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
					},
				},
				{
					name:   "describe hub v2 before any feature enabled has empty Features",
					method: http.MethodGet,
					path:   "/hubv2",
					check: func(t *testing.T, code int, resp map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
						assert.NotEmpty(t, resp["SubscribedAt"])
						features, _ := resp["Features"].(map[string]any)
						assert.Empty(t, features)
					},
				},
				{
					name:   "enable feature",
					method: http.MethodPost,
					path:   "/hubv2/feature/NETWORK_SCANNING",
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
					},
				},
				{
					name:   "describe hub v2 reports feature enabled",
					method: http.MethodGet,
					path:   "/hubv2",
					check: func(t *testing.T, code int, resp map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
						features, _ := resp["Features"].(map[string]any)
						require.Contains(t, features, "NETWORK_SCANNING")
						f, _ := features["NETWORK_SCANNING"].(map[string]any)
						assert.Equal(t, "ENABLED", f["FeatureStatus"])
						assert.NotEmpty(t, f["UpdatedAt"])
					},
				},
				{
					name:   "enable feature again is idempotent",
					method: http.MethodPost,
					path:   "/hubv2/feature/NETWORK_SCANNING",
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
					},
				},
				{
					name:   "disable feature",
					method: http.MethodDelete,
					path:   "/hubv2/feature/NETWORK_SCANNING",
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
					},
				},
				{
					name:   "describe hub v2 reports feature disabled",
					method: http.MethodGet,
					path:   "/hubv2",
					check: func(t *testing.T, code int, resp map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
						features, _ := resp["Features"].(map[string]any)
						f, _ := features["NETWORK_SCANNING"].(map[string]any)
						assert.Equal(t, "DISABLED", f["FeatureStatus"])
					},
				},
				{
					name:   "disable feature again is idempotent",
					method: http.MethodDelete,
					path:   "/hubv2/feature/NETWORK_SCANNING",
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
					},
				},
				{
					name:   "disable hub v2",
					method: http.MethodDelete,
					path:   "/hubv2",
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusOK, code)
					},
				},
				{
					name:   "enable feature after hub v2 disabled returns 404",
					method: http.MethodPost,
					path:   "/hubv2/feature/NETWORK_SCANNING",
					check: func(t *testing.T, code int, _ map[string]any) {
						t.Helper()
						assert.Equal(t, http.StatusNotFound, code)
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			for _, s := range tc.steps {
				rec := doRequest(t, h, s.method, s.path, s.body)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				s.check(t, rec.Code, resp)
			}
		})
	}
}

func TestHandler_HubV2_EnableWithTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name:     "enable with tags",
			body:     map[string]any{"Tags": map[string]any{"env": "test"}},
			wantCode: http.StatusOK,
		},
		{
			name:     "enable without tags",
			body:     map[string]any{},
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPost, "/hubv2", tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

// TestSecurityHubV2FeatureDescribeRace exercises DescribeSecurityHubV2
// concurrently with EnableSecurityHubFeatureV2/DisableSecurityHubFeatureV2.
// DescribeSecurityHubV2 used to shallow-copy *HubV2 (cp := *b.hubV2), which
// only copies the Features map header -- the returned copy's Features map is
// the same map as the live b.hubV2.Features that Enable/DisableFeature write
// into under lock. Ranging over the returned map after RUnlock races with
// those writes.
func TestSecurityHubV2FeatureDescribeRace(t *testing.T) {
	t.Parallel()

	b := securityhub.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b.EnableSecurityHubV2(nil))

	const iterations = 2000

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()

		for range iterations {
			hub, err := b.DescribeSecurityHubV2()
			if err != nil {
				continue
			}

			for range hub.Features { //nolint:revive // exercising the map read deliberately.
			}
		}
	}()

	go func() {
		defer wg.Done()

		for range iterations {
			_ = b.EnableSecurityHubFeatureV2("NETWORK_SCANNING")
		}
	}()

	go func() {
		defer wg.Done()

		for range iterations {
			_ = b.DisableSecurityHubFeatureV2("NETWORK_SCANNING")
		}
	}()

	wg.Wait()
}

// gopherstack-1qf: DisableSecurityHub's doc comment
// (api_op_DisableSecurityHub.go) states "You can't disable Security Hub CSPM
// in an account that is currently the Security Hub CSPM administrator" --
// DisableHub never checked this precondition.
func TestDisableSecurityHub_RefusedWhileAdministrator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		disassociate bool
		wantCode     int
	}{
		{
			name:     "administrator_with_active_member_refused",
			wantCode: http.StatusBadRequest,
		},
		{
			name:         "after_disassociating_all_members_allowed",
			disassociate: true,
			wantCode:     http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, http.MethodPost, "/accounts", nil)
			doRequest(t, h, http.MethodPost, "/members", map[string]any{
				"AccountDetails": []any{
					map[string]any{"AccountId": "111111111111", "Email": "a@example.com"},
				},
			})

			if tc.disassociate {
				doRequest(t, h, http.MethodPost, "/members/disassociate", map[string]any{
					"AccountIds": []any{"111111111111"},
				})
			}

			rec := doRequest(t, h, http.MethodDelete, "/accounts", nil)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}
