package codedeploy_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_CreateDeploymentConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      map[string]any
		name       string
		wantStatus int
		wantID     bool
	}{
		{
			name: "success",
			input: map[string]any{
				"deploymentConfigName": "my-config",
				"computePlatform":      "Server",
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
		{
			name:       "missing_name",
			input:      map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "duplicate",
			input: map[string]any{
				"deploymentConfigName": "dup-config",
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.name == "duplicate" {
				rec := doRequest(t, h, "CreateDeploymentConfig", map[string]any{"deploymentConfigName": "dup-config"})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doRequest(t, h, "CreateDeploymentConfig", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantID {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["deploymentConfigId"])
			}
		})
	}
}

func TestDeploymentConfigs_ComputePlatformValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		platform    string
		wantStatus  int
		wantSuccess bool
	}{
		{name: "server", platform: "Server", wantStatus: http.StatusOK, wantSuccess: true},
		{name: "lambda", platform: "Lambda", wantStatus: http.StatusOK, wantSuccess: true},
		{name: "ecs", platform: "ECS", wantStatus: http.StatusOK, wantSuccess: true},
		{name: "invalid", platform: "Docker", wantStatus: http.StatusBadRequest, wantSuccess: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateDeploymentConfig", map[string]any{
				"deploymentConfigName": "cfg-" + tt.name,
				"computePlatform":      tt.platform,
			})

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_DeploymentConfig(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// CreateDeploymentConfig
	rec := doRequest(t, h, "CreateDeploymentConfig", map[string]any{
		"deploymentConfigName": "my-config",
		"minimumHealthyHosts": map[string]any{
			"type":  "HOST_COUNT",
			"value": 1,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// GetDeploymentConfig
	rec = doRequest(t, h, "GetDeploymentConfig", map[string]any{
		"deploymentConfigName": "my-config",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// ListDeploymentConfigs
	rec = doRequest(t, h, "ListDeploymentConfigs", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// DeleteDeploymentConfig
	rec = doRequest(t, h, "DeleteDeploymentConfig", map[string]any{
		"deploymentConfigName": "my-config",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDeploymentConfigs_SubStructs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      map[string]any
		checkFn    func(t *testing.T, info map[string]any)
		name       string
		wantStatus int
	}{
		{
			name: "minimum_healthy_hosts",
			input: map[string]any{
				"deploymentConfigName": "cfg-mhh",
				"computePlatform":      "Server",
				"minimumHealthyHosts": map[string]any{
					"type":  "FLEET_PERCENT",
					"value": 75,
				},
			},
			wantStatus: http.StatusOK,
			checkFn: func(t *testing.T, info map[string]any) {
				t.Helper()
				mhh, ok := info["minimumHealthyHosts"].(map[string]any)
				require.True(t, ok, "minimumHealthyHosts should be present")
				assert.Equal(t, "FLEET_PERCENT", mhh["type"])
				assert.InDelta(t, 75, mhh["value"], 0.1)
			},
		},
		{
			name: "canary_traffic_routing",
			input: map[string]any{
				"deploymentConfigName": "cfg-canary",
				"computePlatform":      "Lambda",
				"trafficRoutingConfig": map[string]any{
					"type": "TimeBasedCanary",
					"timeBasedCanary": map[string]any{
						"canaryPercentage": 10,
						"canaryInterval":   5,
					},
				},
			},
			wantStatus: http.StatusOK,
			checkFn: func(t *testing.T, info map[string]any) {
				t.Helper()
				trc, ok := info["trafficRoutingConfig"].(map[string]any)
				require.True(t, ok, "trafficRoutingConfig should be present")
				assert.Equal(t, "TimeBasedCanary", trc["type"])
				canary, ok := trc["timeBasedCanary"].(map[string]any)
				require.True(t, ok)
				assert.InDelta(t, 10, canary["canaryPercentage"], 0.1)
				assert.InDelta(t, 5, canary["canaryInterval"], 0.1)
			},
		},
		{
			name: "linear_traffic_routing",
			input: map[string]any{
				"deploymentConfigName": "cfg-linear",
				"computePlatform":      "Lambda",
				"trafficRoutingConfig": map[string]any{
					"type": "TimeBasedLinear",
					"timeBasedLinear": map[string]any{
						"linearPercentage": 10,
						"linearInterval":   1,
					},
				},
			},
			wantStatus: http.StatusOK,
			checkFn: func(t *testing.T, info map[string]any) {
				t.Helper()
				trc, ok := info["trafficRoutingConfig"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "TimeBasedLinear", trc["type"])
			},
		},
		{
			name: "zonal_config",
			input: map[string]any{
				"deploymentConfigName": "cfg-zonal",
				"computePlatform":      "Server",
				"zonalConfig": map[string]any{
					"firstZoneMonitorDurationInSeconds": 60,
					"monitorDurationInSeconds":          30,
					"minimumHealthyHostsPerZone": map[string]any{
						"type":  "HOST_COUNT",
						"value": 2,
					},
				},
			},
			wantStatus: http.StatusOK,
			checkFn: func(t *testing.T, info map[string]any) {
				t.Helper()
				zc, ok := info["zonalConfig"].(map[string]any)
				require.True(t, ok, "zonalConfig should be present")
				assert.InDelta(t, 60, zc["firstZoneMonitorDurationInSeconds"], 0.1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateDeploymentConfig", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.checkFn != nil && tt.wantStatus == http.StatusOK {
				name, _ := tt.input["deploymentConfigName"].(string)
				rec2 := doRequest(t, h, "GetDeploymentConfig", map[string]any{
					"deploymentConfigName": name,
				})
				require.Equal(t, http.StatusOK, rec2.Code)

				var resp map[string]map[string]any
				require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
				tt.checkFn(t, resp["deploymentConfigInfo"])
			}
		})
	}
}

func TestDeploymentConfigs_DefaultsSeeded(t *testing.T) {
	t.Parallel()

	expectedDefaults := []string{
		"CodeDeployDefault.AllAtOnce",
		"CodeDeployDefault.OneAtATime",
		"CodeDeployDefault.HalfAtATime",
		"CodeDeployDefault.LambdaAllAtOnce",
		"CodeDeployDefault.LambdaCanary10Percent5Minutes",
		"CodeDeployDefault.LambdaLinear10PercentEvery1Minute",
		"CodeDeployDefault.ECSAllAtOnce",
		"CodeDeployDefault.ECSCanary10Percent5Minutes",
		"CodeDeployDefault.ECSLinear10PercentEvery1Minute",
	}

	h := newTestHandler(t)

	rec := doRequest(t, h, "ListDeploymentConfigs", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		DeploymentConfigsList []string `json:"deploymentConfigsList"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	for _, name := range expectedDefaults {
		assert.Contains(t, resp.DeploymentConfigsList, name, "missing default config: %s", name)
	}
}

func TestDeploymentConfigs_DefaultsReturnSubStructs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		configName      string
		wantPlatform    string
		wantRoutingType string
	}{
		{
			name:            "AllAtOnce",
			configName:      "CodeDeployDefault.AllAtOnce",
			wantPlatform:    "Server",
			wantRoutingType: "AllAtOnce",
		},
		{
			name:            "LambdaCanary10Percent5Minutes",
			configName:      "CodeDeployDefault.LambdaCanary10Percent5Minutes",
			wantPlatform:    "Lambda",
			wantRoutingType: "TimeBasedCanary",
		},
		{
			name:            "ECSLinear",
			configName:      "CodeDeployDefault.ECSLinear10PercentEvery1Minute",
			wantPlatform:    "ECS",
			wantRoutingType: "TimeBasedLinear",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "GetDeploymentConfig", map[string]any{
				"deploymentConfigName": tt.configName,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				DeploymentConfigInfo struct {
					TrafficRoutingConfig *struct {
						Type string `json:"type"`
					} `json:"trafficRoutingConfig"`
					ComputePlatform string `json:"computePlatform"`
				} `json:"deploymentConfigInfo"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantPlatform, resp.DeploymentConfigInfo.ComputePlatform)
			require.NotNil(t, resp.DeploymentConfigInfo.TrafficRoutingConfig)
			assert.Equal(t, tt.wantRoutingType, resp.DeploymentConfigInfo.TrafficRoutingConfig.Type)
		})
	}
}

func TestDeploymentConfigs_DefaultsCannotDelete(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "DeleteDeploymentConfig", map[string]any{
		"deploymentConfigName": "CodeDeployDefault.AllAtOnce",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	// DeleteDeploymentConfig's own deserializer models InvalidOperationException
	// for this case, not DeploymentConfigInUseException (verified against
	// aws-sdk-go-v2/service/codedeploy deserializers.go).
	assert.Equal(t, "InvalidOperationException", resp["__type"])
}

// TestDeploymentConfigs_InUseCannotDelete proves a deployment config still
// referenced by a deployment group cannot be deleted (api_op_DeleteDeploymentConfig.go:12-13,
// "A deployment configuration cannot be deleted if it is currently in use."),
// mapped to DeleteDeploymentConfig's own DeploymentConfigInUseException.
func TestDeploymentConfigs_InUseCannotDelete(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doRequest(t, h, "CreateDeploymentConfig", map[string]any{
		"deploymentConfigName": "in-use-cfg",
		"computePlatform":      "Server",
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	doRequest(t, h, "CreateApplication", map[string]any{
		"applicationName": "cfg-app",
		"computePlatform": "Server",
	})
	dgRec := doRequest(t, h, "CreateDeploymentGroup", map[string]any{
		"applicationName":      "cfg-app",
		"deploymentGroupName":  "cfg-dg",
		"serviceRoleArn":       "arn:aws:iam::000000000000:role/role",
		"deploymentConfigName": "in-use-cfg",
	})
	require.Equal(t, http.StatusOK, dgRec.Code)

	delRec := doRequest(t, h, "DeleteDeploymentConfig", map[string]any{
		"deploymentConfigName": "in-use-cfg",
	})
	assert.Equal(t, http.StatusConflict, delRec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(delRec.Body.Bytes(), &resp))
	assert.Equal(t, "DeploymentConfigInUseException", resp["__type"])
}

func TestDeploymentConfigs_ErrInvalidComputePlatformMapping(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Invalid compute platform triggers ErrInvalidComputePlatform → 400
	rec := doRequest(t, h, "CreateDeploymentConfig", map[string]any{
		"deploymentConfigName": "bad-cfg",
		"computePlatform":      "InvalidPlatform",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "InvalidComputePlatformException", resp["__type"])
}

func TestDeploymentConfigs_ARN(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	cfgARN := h.Backend.DeploymentConfigARN("my-config")

	assert.Contains(t, cfgARN, "deploymentconfig:my-config")
}
