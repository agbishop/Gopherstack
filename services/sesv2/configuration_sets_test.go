package sesv2_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sesv2"
)

// ─── configuration set option round-trips (HTTP) ────────────────────────────

// TestConfigSetTrackingOptions verifies round-trip for tracking options.
func TestConfigSetTrackingOptions(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateConfigurationSet("track-cs", nil)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodPut, "/v2/email/configuration-sets/track-cs/tracking-options",
		map[string]any{
			"CustomRedirectDomain": "click.example.com",
			"HttpsPolicy":          "REQUIRE",
		})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(t, h, http.MethodGet, "/v2/email/configuration-sets/track-cs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	tracking, ok := out["TrackingOptions"].(map[string]any)
	require.True(t, ok, "TrackingOptions should be present in GetConfigurationSet response")
	assert.Equal(t, "click.example.com", tracking["CustomRedirectDomain"])
	assert.Equal(t, "REQUIRE", tracking["HttpsPolicy"])
}

// TestConfigSetTrackingOptionsNotFound returns 404 for missing config set.
func TestConfigSetTrackingOptionsNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/configuration-sets/no-such-cs/tracking-options",
		map[string]any{"CustomRedirectDomain": "x.com"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestConfigSetDeliveryOptions verifies round-trip for delivery options.
func TestConfigSetDeliveryOptions(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateConfigurationSet("delivery-cs", nil)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/configuration-sets/delivery-cs/delivery-options",
		map[string]any{
			"TlsPolicy":       "REQUIRE",
			"SendingPoolName": "my-pool",
		})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(t, h, http.MethodGet, "/v2/email/configuration-sets/delivery-cs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	delivery, ok := out["DeliveryOptions"].(map[string]any)
	require.True(t, ok, "DeliveryOptions should be present")
	assert.Equal(t, "REQUIRE", delivery["TlsPolicy"])
	assert.Equal(t, "my-pool", delivery["SendingPoolName"])
}

// TestConfigSetDeliveryOptionsNotFound returns 404 for missing config set.
func TestConfigSetDeliveryOptionsNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/configuration-sets/no-such-cs/delivery-options",
		map[string]any{"TlsPolicy": "REQUIRE"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestConfigSetSendingOptions verifies round-trip for sending options.
func TestConfigSetSendingOptions(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateConfigurationSet("send-cs", nil)
	require.NoError(t, err)

	// Disable sending.
	rec := doReq(t, h, http.MethodPut,
		"/v2/email/configuration-sets/send-cs/sending",
		map[string]any{"SendingEnabled": false})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(t, h, http.MethodGet, "/v2/email/configuration-sets/send-cs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	sending, ok := out["SendingOptions"].(map[string]any)
	require.True(t, ok, "SendingOptions should be present")
	assert.Equal(t, false, sending["SendingEnabled"])
}

// TestConfigSetSendingOptionsNotFound returns 404 for missing config set.
func TestConfigSetSendingOptionsNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/configuration-sets/no-such-cs/sending",
		map[string]any{"SendingEnabled": true})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestConfigSetReputationOptions verifies round-trip for reputation options.
func TestConfigSetReputationOptions(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateConfigurationSet("rep-cs", nil)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/configuration-sets/rep-cs/reputation-options",
		map[string]any{"ReputationMetricsEnabled": true})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(t, h, http.MethodGet, "/v2/email/configuration-sets/rep-cs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	rep, ok := out["ReputationOptions"].(map[string]any)
	require.True(t, ok, "ReputationOptions should be present")
	assert.Equal(t, true, rep["ReputationMetricsEnabled"])
}

// TestConfigSetReputationOptionsNotFound returns 404 for missing config set.
func TestConfigSetReputationOptionsNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/configuration-sets/no-such-cs/reputation-options",
		map[string]any{"ReputationMetricsEnabled": true})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestConfigSetSuppressionOptions verifies round-trip for suppression options.
func TestConfigSetSuppressionOptions(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateConfigurationSet("supp-cs", nil)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/configuration-sets/supp-cs/suppression-options",
		map[string]any{"SuppressedReasons": []string{"BOUNCE", "COMPLAINT"}})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doReq(t, h, http.MethodGet, "/v2/email/configuration-sets/supp-cs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	supp, ok := out["SuppressionOptions"].(map[string]any)
	require.True(t, ok, "SuppressionOptions should be present")

	reasons, ok := supp["SuppressedReasons"].([]any)
	require.True(t, ok, "SuppressedReasons should be a list")
	assert.Len(t, reasons, 2)
}

// TestConfigSetSuppressionOptionsNotFound returns 404 for missing config set.
func TestConfigSetSuppressionOptionsNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/configuration-sets/no-such-cs/suppression-options",
		map[string]any{"SuppressedReasons": []string{"BOUNCE"}})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestConfigSetArchivingOptionsNotFound returns 404 for missing config set.
func TestConfigSetArchivingOptionsNotFound(t *testing.T) {
	t.Parallel()

	h, _ := newSESv2TestHandler(t)

	rec := doReq(t, h, http.MethodPut,
		"/v2/email/configuration-sets/no-such-cs/archiving-options",
		map[string]any{})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGetConfigurationSetDefaultSendingEnabled verifies SendingEnabled defaults to true.
func TestGetConfigurationSetDefaultSendingEnabled(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateConfigurationSet("default-cs", nil)
	require.NoError(t, err)

	rec := doReq(t, h, http.MethodGet, "/v2/email/configuration-sets/default-cs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	sending, ok := out["SendingOptions"].(map[string]any)
	require.True(t, ok, "SendingOptions always present in GetConfigurationSet")
	assert.Equal(t, true, sending["SendingEnabled"], "SendingEnabled defaults to true")
}

// TestConfigSetMultipleOptions verifies all options can be set and are returned together.
func TestConfigSetMultipleOptions(t *testing.T) {
	t.Parallel()

	h, backend := newSESv2TestHandler(t)

	_, err := backend.CreateConfigurationSet("multi-cs", nil)
	require.NoError(t, err)

	doReq(t, h, http.MethodPut, "/v2/email/configuration-sets/multi-cs/tracking-options",
		map[string]any{"CustomRedirectDomain": "track.example.com"})
	doReq(t, h, http.MethodPut, "/v2/email/configuration-sets/multi-cs/delivery-options",
		map[string]any{"TlsPolicy": "OPTIONAL"})
	doReq(t, h, http.MethodPut, "/v2/email/configuration-sets/multi-cs/reputation-options",
		map[string]any{"ReputationMetricsEnabled": true})
	doReq(t, h, http.MethodPut, "/v2/email/configuration-sets/multi-cs/sending",
		map[string]any{"SendingEnabled": false})
	doReq(t, h, http.MethodPut, "/v2/email/configuration-sets/multi-cs/suppression-options",
		map[string]any{"SuppressedReasons": []string{"BOUNCE"}})

	rec := doReq(t, h, http.MethodGet, "/v2/email/configuration-sets/multi-cs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	assert.Equal(t, "multi-cs", out["ConfigurationSetName"])
	assert.NotNil(t, out["TrackingOptions"])
	assert.NotNil(t, out["DeliveryOptions"])
	assert.NotNil(t, out["ReputationOptions"])
	assert.NotNil(t, out["SendingOptions"])
	assert.NotNil(t, out["SuppressionOptions"])
}

// ─── configuration set backend unit tests ───────────────────────────────────

// TestBackendPutConfigSetTrackingOptions tests backend method directly.
func TestBackendPutConfigSetTrackingOptions(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	_, err := backend.CreateConfigurationSet("track-test", nil)
	require.NoError(t, err)

	err = backend.PutConfigurationSetTrackingOptions("track-test", "click.example.com", "REQUIRE")
	require.NoError(t, err)

	cs, err := backend.GetConfigurationSet("track-test")
	require.NoError(t, err)
	assert.Equal(t, "click.example.com", cs.TrackingCustomRedirectDomain)
	assert.Equal(t, "REQUIRE", cs.TrackingHTTPSPolicy)
}

// TestBackendPutConfigSetTrackingOptionsNotFound errors for missing set.
func TestBackendPutConfigSetTrackingOptionsNotFound(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	err := backend.PutConfigurationSetTrackingOptions("no-such", "x.com", "")
	require.Error(t, err)
}

// TestBackendPutConfigSetDeliveryOptions tests backend method directly.
func TestBackendPutConfigSetDeliveryOptions(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	_, err := backend.CreateConfigurationSet("delivery-test", nil)
	require.NoError(t, err)

	err = backend.PutConfigurationSetDeliveryOptions("delivery-test", "REQUIRE", "my-pool")
	require.NoError(t, err)

	cs, err := backend.GetConfigurationSet("delivery-test")
	require.NoError(t, err)
	assert.Equal(t, "REQUIRE", cs.DeliveryTLSPolicy)
	assert.Equal(t, "my-pool", cs.DeliverySendingPoolName)
}

// TestBackendPutConfigSetSendingOptions tests backend method directly.
func TestBackendPutConfigSetSendingOptions(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	_, err := backend.CreateConfigurationSet("send-test", nil)
	require.NoError(t, err)

	// Default is true; disable.
	err = backend.PutConfigurationSetSendingOptions("send-test", false)
	require.NoError(t, err)

	cs, err := backend.GetConfigurationSet("send-test")
	require.NoError(t, err)
	assert.False(t, cs.SendingEnabled)
}

// TestBackendPutConfigSetReputationOptions tests backend method directly.
func TestBackendPutConfigSetReputationOptions(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	_, err := backend.CreateConfigurationSet("rep-test", nil)
	require.NoError(t, err)

	err = backend.PutConfigurationSetReputationOptions("rep-test", true)
	require.NoError(t, err)

	cs, err := backend.GetConfigurationSet("rep-test")
	require.NoError(t, err)
	assert.True(t, cs.ReputationMetricsEnabled)
}

// TestBackendPutConfigSetSuppressionOptions tests backend method directly.
func TestBackendPutConfigSetSuppressionOptions(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	_, err := backend.CreateConfigurationSet("supp-test", nil)
	require.NoError(t, err)

	err = backend.PutConfigurationSetSuppressionOptions("supp-test", []string{"BOUNCE", "COMPLAINT"})
	require.NoError(t, err)

	cs, err := backend.GetConfigurationSet("supp-test")
	require.NoError(t, err)
	assert.Equal(t, []string{"BOUNCE", "COMPLAINT"}, cs.SuppressionReasons)
}

// TestBackendConfigSetDefaultSendingEnabled verifies CreateConfigurationSet default.
func TestBackendConfigSetDefaultSendingEnabled(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	cs, err := backend.CreateConfigurationSet("default-send", nil)
	require.NoError(t, err)
	assert.True(t, cs.SendingEnabled, "SendingEnabled should default to true")
}

// TestBackendPutConfigSetArchivingOptionsNotFound errors for missing set.
func TestBackendPutConfigSetArchivingOptionsNotFound(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()

	err := backend.PutConfigurationSetArchivingOptions("no-such", "")
	require.Error(t, err)
}

// TestConfigSetCount verifies ConfigSetCount helper.
func TestConfigSetCount(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()
	assert.Equal(t, 0, sesv2.ConfigSetCount(backend))

	_, err := backend.CreateConfigurationSet("set-1", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, sesv2.ConfigSetCount(backend))
}

// TestGetConfigurationSetDeepCopy verifies deep copy of configuration set and its nested fields.
func TestGetConfigurationSetDeepCopy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mutate func(cs *sesv2.ConfigurationSet)
		check  func(t *testing.T, cs *sesv2.ConfigurationSet)
		name   string
	}{
		{
			name: "mutate_name",
			mutate: func(cs *sesv2.ConfigurationSet) {
				cs.Name = "mutated"
			},
			check: func(t *testing.T, cs *sesv2.ConfigurationSet) {
				t.Helper()
				assert.Equal(t, "copy-set", cs.Name)
			},
		},
		{
			name: "mutate_tags",
			mutate: func(cs *sesv2.ConfigurationSet) {
				if cs.Tags != nil {
					cs.Tags["k1"] = "mutated_value"
					cs.Tags["k_new"] = "new_value"
				}
			},
			check: func(t *testing.T, cs *sesv2.ConfigurationSet) {
				t.Helper()
				assert.Equal(t, "v1", cs.Tags["k1"])
				assert.NotContains(t, cs.Tags, "k_new")
			},
		},
		{
			name: "mutate_suppression_reasons",
			mutate: func(cs *sesv2.ConfigurationSet) {
				if len(cs.SuppressionReasons) > 0 {
					cs.SuppressionReasons[0] = "COMPLAINT"
				}
			},
			check: func(t *testing.T, cs *sesv2.ConfigurationSet) {
				t.Helper()
				assert.Equal(t, []string{"BOUNCE"}, cs.SuppressionReasons)
			},
		},
		{
			name: "mutate_vdm_options",
			mutate: func(cs *sesv2.ConfigurationSet) {
				if cs.VdmOptions != nil && cs.VdmOptions.DashboardOptions != nil {
					cs.VdmOptions.DashboardOptions["EngagementMetrics"] = "DISABLED"
				}
			},
			check: func(t *testing.T, cs *sesv2.ConfigurationSet) {
				t.Helper()
				assert.Equal(t, "ENABLED", cs.VdmOptions.DashboardOptions["EngagementMetrics"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sesv2.NewInMemoryBackend()
			_, err := backend.CreateConfigurationSet("copy-set", map[string]string{"k1": "v1"})
			require.NoError(t, err)

			err = backend.PutConfigurationSetSuppressionOptions("copy-set", []string{"BOUNCE"})
			require.NoError(t, err)

			err = backend.PutConfigurationSetVdmOptions("copy-set", map[string]any{"EngagementMetrics": "ENABLED"}, nil)
			require.NoError(t, err)

			cs, err := backend.GetConfigurationSet("copy-set")
			require.NoError(t, err)

			tt.mutate(cs)

			cs2, err := backend.GetConfigurationSet("copy-set")
			require.NoError(t, err)
			tt.check(t, cs2)
		})
	}
}

// TestDeleteConfigurationSetCascade verifies cascade delete of event destinations.
func TestDeleteConfigurationSetCascade(t *testing.T) {
	t.Parallel()

	backend := sesv2.NewInMemoryBackend()
	_, err := backend.CreateConfigurationSet("my-set", nil)
	require.NoError(t, err)

	_, err = backend.CreateConfigurationSetEventDestination("my-set", "dest-1", true, nil)
	require.NoError(t, err)

	err = backend.DeleteConfigurationSet("my-set")
	require.NoError(t, err)

	assert.Equal(t, 0, sesv2.ConfigSetCount(backend))
}

// ─── configuration set attribute Put operations (HTTP smoke test) ──────────

// TestPutConfigurationSetAttributes smoke-tests every PUT attribute-writing endpoint
// under /v2/email/configuration-sets/{name}/... in one pass.
func TestPutConfigurationSetAttributes(t *testing.T) {
	t.Parallel()

	h := newHandler()

	doRequest(t, h, http.MethodPost, "/v2/email/configuration-sets", map[string]any{
		"ConfigurationSetName": "AttrCS",
	})

	attrPaths := []string{
		"/v2/email/configuration-sets/AttrCS/archiving-options",
		"/v2/email/configuration-sets/AttrCS/delivery-options",
		"/v2/email/configuration-sets/AttrCS/reputation-options",
		"/v2/email/configuration-sets/AttrCS/sending",
		"/v2/email/configuration-sets/AttrCS/suppression-options",
		"/v2/email/configuration-sets/AttrCS/tracking-options",
		"/v2/email/configuration-sets/AttrCS/vdm-options",
	}

	for _, path := range attrPaths {
		rec := doRequest(t, h, http.MethodPut, path, map[string]any{})
		assert.Equal(t, http.StatusOK, rec.Code, "path=%s", path)
	}
}

// ─── core configuration set CRUD (HTTP) ─────────────────────────────────────

// TestCreateConfigurationSet tests the CreateConfigurationSet operation via HTTP.
func TestCreateConfigurationSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name:     "creates config set",
			body:     map[string]any{"ConfigurationSetName": "my-config"},
			wantCode: http.StatusOK,
		},
		{
			name:     "duplicate returns conflict",
			body:     map[string]any{"ConfigurationSetName": "dup-config"},
			wantCode: http.StatusConflict,
		},
		{
			name:     "empty name returns bad request",
			body:     map[string]any{"ConfigurationSetName": ""},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.name == "duplicate returns conflict" {
				doRequest(
					t,
					h,
					http.MethodPost,
					"/v2/email/configuration-sets",
					map[string]any{"ConfigurationSetName": "dup-config"},
				)
			}

			rec := doRequest(t, h, http.MethodPost, "/v2/email/configuration-sets", tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestGetConfigurationSet tests the GetConfigurationSet operation via HTTP.
func TestGetConfigurationSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		csName   string
		wantCode int
	}{
		{
			name:     "gets existing config set",
			csName:   "my-config",
			wantCode: http.StatusOK,
		},
		{
			name:     "not found returns 404",
			csName:   "notfound",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.wantCode == http.StatusOK {
				doRequest(
					t,
					h,
					http.MethodPost,
					"/v2/email/configuration-sets",
					map[string]any{"ConfigurationSetName": tt.csName},
				)
			}

			rec := doRequest(t, h, http.MethodGet, "/v2/email/configuration-sets/"+tt.csName, nil)

			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Equal(t, tt.csName, out["ConfigurationSetName"])
			}
		})
	}
}

// TestListConfigurationSets tests the ListConfigurationSets operation via HTTP.
func TestListConfigurationSets(t *testing.T) {
	t.Parallel()

	h := newHandler()

	doRequest(
		t,
		h,
		http.MethodPost,
		"/v2/email/configuration-sets",
		map[string]any{"ConfigurationSetName": "config-a"},
	)
	doRequest(
		t,
		h,
		http.MethodPost,
		"/v2/email/configuration-sets",
		map[string]any{"ConfigurationSetName": "config-b"},
	)

	rec := doRequest(t, h, http.MethodGet, "/v2/email/configuration-sets", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	sets, ok := out["ConfigurationSets"].([]any)
	require.True(t, ok)
	assert.Len(t, sets, 2)
}

// TestDeleteConfigurationSet tests the DeleteConfigurationSet operation via HTTP.
func TestDeleteConfigurationSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		csName   string
		wantCode int
	}{
		{
			name:     "deletes existing config set",
			csName:   "del-config",
			wantCode: http.StatusOK,
		},
		{
			name:     "not found returns 404",
			csName:   "notfound",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.wantCode == http.StatusOK {
				doRequest(
					t,
					h,
					http.MethodPost,
					"/v2/email/configuration-sets",
					map[string]any{"ConfigurationSetName": tt.csName},
				)
			}

			rec := doRequest(
				t,
				h,
				http.MethodDelete,
				"/v2/email/configuration-sets/"+tt.csName,
				nil,
			)

			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestDeleteConfigurationSet_ClearsResourceTagsOnRecreate(t *testing.T) {
	t.Parallel()

	h := newHandler()

	_, err := h.Backend.CreateConfigurationSet("reused-config", map[string]string{"env": "prod"})
	require.NoError(t, err)

	require.NoError(t, h.Backend.DeleteConfigurationSet("reused-config"))

	_, err = h.Backend.CreateConfigurationSet("reused-config", nil)
	require.NoError(t, err)

	recreated, err := h.Backend.GetConfigurationSet("reused-config")
	require.NoError(t, err)
	assert.Empty(t, recreated.Tags)
}
