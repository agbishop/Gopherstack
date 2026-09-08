package mediatailor_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureLogsForChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
		body     any
		name     string
		wantCode int
	}{
		{
			name: "configure logs returns channel name and log types",
			body: map[string]any{
				"ChannelName": "ch1",
				"LogTypes":    []any{"AS_RUN"},
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, "ch1", resp["ChannelName"])
				logTypes, _ := resp["LogTypes"].([]any)
				assert.Len(t, logTypes, 1)
			},
		},
		{
			name: "configure logs for missing channel returns 404",
			body: map[string]any{
				"ChannelName": "nope",
				"LogTypes":    []any{"AS_RUN"},
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createTestChannel(t, h)

			rec := doRequest(t, h, http.MethodPut, "/configureLogs/channel", tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)

			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}

func TestConfigureLogsForPlaybackConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
		body     any
		name     string
		wantCode int
	}{
		{
			name: "configure logs returns config name and percent",
			body: map[string]any{
				"PlaybackConfigurationName": "pc1",
				"PercentEnabled":            float64(60),
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, "pc1", resp["PlaybackConfigurationName"])
				assert.InDelta(t, float64(60), resp["PercentEnabled"], 0.0001)
			},
		},
		{
			name: "configure logs for missing config returns 404",
			body: map[string]any{
				"PlaybackConfigurationName": "nope",
				"PercentEnabled":            float64(0),
			},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createTestPlaybackConfig(t, h, "pc1")

			rec := doRequest(t, h, http.MethodPut, "/configureLogs/playbackConfiguration", tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)

			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}

func TestHandleConfigureLogs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		path     string
		wantCode int
	}{
		{
			name:     "configure logs for channel that exists",
			path:     "/configureLogs/channel",
			body:     map[string]any{"ChannelName": "ch1", "LogTypes": []any{"AS_RUN"}},
			wantCode: http.StatusOK,
		},
		{
			name:     "configure logs for channel not found",
			path:     "/configureLogs/channel",
			body:     map[string]any{"ChannelName": "missing", "LogTypes": []any{"AS_RUN"}},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "configure logs for playback config that exists",
			path:     "/configureLogs/playbackConfiguration",
			body:     map[string]any{"PlaybackConfigurationName": "pc1", "PercentEnabled": float64(50)},
			wantCode: http.StatusOK,
		},
		{
			name:     "configure logs for playback config not found",
			path:     "/configureLogs/playbackConfiguration",
			body:     map[string]any{"PlaybackConfigurationName": "missing", "PercentEnabled": float64(50)},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			// Set up dependencies for success cases
			if tt.wantCode == http.StatusOK {
				if tt.path == "/configureLogs/channel" {
					doRequest(t, h, http.MethodPost, "/channel/ch1", map[string]any{
						"PlaybackMode": "LINEAR",
					})
				} else {
					doRequest(t, h, http.MethodPut, "/playbackConfiguration", map[string]any{
						"Name":                  "pc1",
						"AdDecisionServerUrl":   "https://ads.com",
						"VideoContentSourceUrl": "https://video.com",
					})
				}
			}

			rec := doRequest(t, h, http.MethodPut, tt.path, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestConfigureLogsForChannel_PersistsToDescribe verifies ConfigureLogsForChannel's
// LogTypes are queryable back from DescribeChannel's LogConfiguration, not just
// echoed by the configure call itself (the gap PARITY.md flagged: the prior
// implementation validated + echoed but never wrote anywhere queryable).
func TestConfigureLogsForChannel_PersistsToDescribe(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestChannel(t, h)

	rec := doRequest(t, h, http.MethodPut, "/configureLogs/channel", map[string]any{
		"ChannelName": "ch1",
		"LogTypes":    []any{"AS_RUN"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/channel/ch1", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	logCfg, ok := resp["LogConfiguration"].(map[string]any)
	require.True(t, ok, "DescribeChannel must include LogConfiguration")
	logTypes, _ := logCfg["LogTypes"].([]any)
	require.Len(t, logTypes, 1)
	assert.Equal(t, "AS_RUN", logTypes[0])
}

// TestConfigureLogsForPlaybackConfiguration_PersistsToGet verifies
// ConfigureLogsForPlaybackConfiguration's settings are queryable back from
// GetPlaybackConfiguration's LogConfiguration.
func TestConfigureLogsForPlaybackConfiguration_PersistsToGet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestPlaybackConfig(t, h, "pc1")

	rec := doRequest(t, h, http.MethodPut, "/configureLogs/playbackConfiguration", map[string]any{
		"PlaybackConfigurationName": "pc1",
		"PercentEnabled":            float64(75),
		"EnabledLoggingStrategies":  []any{"VENDED_LOGS"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/playbackConfiguration/pc1", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	logCfg, ok := resp["LogConfiguration"].(map[string]any)
	require.True(t, ok, "GetPlaybackConfiguration must include LogConfiguration")
	assert.InDelta(t, float64(75), logCfg["PercentEnabled"], 0.0001)
	strategies, _ := logCfg["EnabledLoggingStrategies"].([]any)
	require.Len(t, strategies, 1)
	assert.Equal(t, "VENDED_LOGS", strategies[0])
}

// TestConfigureLogsForChannel_RejectsInvalidLogTypes verifies LogTypes is
// required and constrained to the real single-value enum (AS_RUN) --
// aws-sdk-go-v2/service/mediatailor@v1.63.4 types/enums.go:348-353.
func TestConfigureLogsForChannel_RejectsInvalidLogTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
	}{
		{name: "empty log types", body: map[string]any{"ChannelName": "ch1", "LogTypes": []any{}}},
		{name: "unknown log type", body: map[string]any{"ChannelName": "ch1", "LogTypes": []any{"FULL_MANIFEST"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createTestChannel(t, h)

			rec := doRequest(t, h, http.MethodPut, "/configureLogs/channel", tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// TestConfigureLogsForPlaybackConfiguration_RejectsInvalidLogSettings verifies
// PercentEnabled's documented "Valid values: 0 - 100" range
// (aws-sdk-go-v2/service/mediatailor@v1.63.4
// api_op_ConfigureLogsForPlaybackConfiguration.go:39) and
// EnabledLoggingStrategies' real 2-value enum (types/enums.go:329-335) are
// both enforced.
func TestConfigureLogsForPlaybackConfiguration_RejectsInvalidLogSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
	}{
		{
			name: "percent enabled below range",
			body: map[string]any{"PlaybackConfigurationName": "pc1", "PercentEnabled": float64(-1)},
		},
		{
			name: "percent enabled above range",
			body: map[string]any{"PlaybackConfigurationName": "pc1", "PercentEnabled": float64(101)},
		},
		{
			name: "unknown logging strategy",
			body: map[string]any{
				"PlaybackConfigurationName": "pc1",
				"PercentEnabled":            float64(50),
				"EnabledLoggingStrategies":  []any{"NOT_A_STRATEGY"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createTestPlaybackConfig(t, h, "pc1")

			rec := doRequest(t, h, http.MethodPut, "/configureLogs/playbackConfiguration", tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// TestConfigureLogsForPlaybackConfiguration_SurvivesRePut verifies a
// logging configuration set via ConfigureLogsForPlaybackConfiguration is not
// reset by a subsequent PutPlaybackConfiguration on the same name, matching
// real MediaTailor (logging config is managed by its own operation).
func TestConfigureLogsForPlaybackConfiguration_SurvivesRePut(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestPlaybackConfig(t, h, "pc1")

	rec := doRequest(t, h, http.MethodPut, "/configureLogs/playbackConfiguration", map[string]any{
		"PlaybackConfigurationName": "pc1",
		"PercentEnabled":            float64(40),
	})
	require.Equal(t, http.StatusOK, rec.Code)

	createTestPlaybackConfig(t, h, "pc1")

	rec = doRequest(t, h, http.MethodGet, "/playbackConfiguration/pc1", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	logCfg, ok := resp["LogConfiguration"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, float64(40), logCfg["PercentEnabled"], 0.0001)
}
