package codedeploy_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/codedeploy"
)

func TestHandler_CreateDeploymentGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input      map[string]any
		setup      func(h *codedeploy.Handler)
		name       string
		wantStatus int
		wantID     bool
	}{
		{
			name: "success",
			setup: func(h *codedeploy.Handler) {
				_, err := h.Backend.CreateApplication("my-app", "Server", nil)
				if err != nil {
					panic(err)
				}
			},
			input: map[string]any{
				"applicationName":     "my-app",
				"deploymentGroupName": "my-dg",
				"serviceRoleArn":      "arn:aws:iam::123:role/my-role",
			},
			wantStatus: http.StatusOK,
			wantID:     true,
		},
		{
			name: "app_not_found",
			input: map[string]any{
				"applicationName":     "nonexistent-app",
				"deploymentGroupName": "my-dg",
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "missing_fields",
			input:      map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "CreateDeploymentGroup", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantID {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["deploymentGroupId"])
			}
		})
	}
}

func TestDeploymentGroups_AlreadyExistsError(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateApplication("dup-app", "Server", nil)
	_, _ = createDG(h.Backend, "dup-app", "dup-dg", "", "", nil)

	rec := doRequest(t, h, "CreateDeploymentGroup", map[string]any{
		"applicationName":     "dup-app",
		"deploymentGroupName": "dup-dg",
	})
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestDeploymentGroups_RichFieldsRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, err := h.Backend.CreateApplication("my-app", "Server", nil)
	require.NoError(t, err)

	rec := doRequest(t, h, "CreateDeploymentGroup", map[string]any{
		"applicationName":      "my-app",
		"deploymentGroupName":  "rich-dg",
		"serviceRoleArn":       "arn:aws:iam::123:role/role",
		"deploymentConfigName": "CodeDeployDefault.AllAtOnce",
		"ec2TagFilters": []map[string]string{
			{"Key": "env", "Value": "prod", "Type": "KEY_AND_VALUE"},
		},
		"autoScalingGroups": []map[string]string{
			{"name": "my-asg"},
		},
		"deploymentStyle": map[string]string{
			"deploymentType":   "BLUE_GREEN",
			"deploymentOption": "WITH_TRAFFIC_CONTROL",
		},
		"alarmConfiguration": map[string]any{
			"enabled": true,
			"alarms":  []map[string]string{{"name": "cpu-alarm"}},
		},
		"autoRollbackConfiguration": map[string]any{
			"enabled": true,
			"events":  []string{"DEPLOYMENT_FAILURE"},
		},
		"triggerConfigurations": []map[string]any{
			{
				"triggerName":      "deploy-trigger",
				"triggerTargetArn": "arn:aws:sns:us-east-1:123:topic",
				"triggerEvents":    []string{"DeploymentSuccess"},
			},
		},
		"outdatedInstancesStrategy": "UPDATE",
		"terminationHookEnabled":    true,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Retrieve via GetDeploymentGroup and verify round-trip.
	rec2 := doRequest(t, h, "GetDeploymentGroup", map[string]any{
		"applicationName":     "my-app",
		"deploymentGroupName": "rich-dg",
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp struct {
		DeploymentGroupInfo struct {
			DeploymentStyle *struct {
				DeploymentType   string `json:"deploymentType"`
				DeploymentOption string `json:"deploymentOption"`
			} `json:"deploymentStyle"`
			AlarmConfiguration *struct {
				Alarms []struct {
					Name string `json:"name"`
				} `json:"alarms"`
				Enabled bool `json:"enabled"`
			} `json:"alarmConfiguration"`
			AutoRollbackConfiguration *struct {
				Events  []string `json:"events"`
				Enabled bool     `json:"enabled"`
			} `json:"autoRollbackConfiguration"`
			ServiceRoleArn            string `json:"serviceRoleArn"`
			ComputePlatform           string `json:"computePlatform"`
			OutdatedInstancesStrategy string `json:"outdatedInstancesStrategy"`
			Ec2TagFilters             []struct {
				Key   string `json:"Key"`
				Value string `json:"Value"`
				Type  string `json:"Type"`
			} `json:"ec2TagFilters"`
			TriggerConfigurations []struct {
				TriggerName   string   `json:"triggerName"`
				TriggerEvents []string `json:"triggerEvents"`
			} `json:"triggerConfigurations"`
			TerminationHookEnabled bool `json:"terminationHookEnabled"`
		} `json:"deploymentGroupInfo"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))

	info := resp.DeploymentGroupInfo
	assert.Equal(t, "arn:aws:iam::123:role/role", info.ServiceRoleArn)
	assert.Equal(t, "Server", info.ComputePlatform)
	require.NotNil(t, info.DeploymentStyle)
	assert.Equal(t, "BLUE_GREEN", info.DeploymentStyle.DeploymentType)
	assert.Equal(t, "WITH_TRAFFIC_CONTROL", info.DeploymentStyle.DeploymentOption)
	require.NotNil(t, info.AlarmConfiguration)
	assert.True(t, info.AlarmConfiguration.Enabled)
	require.Len(t, info.AlarmConfiguration.Alarms, 1)
	assert.Equal(t, "cpu-alarm", info.AlarmConfiguration.Alarms[0].Name)
	require.NotNil(t, info.AutoRollbackConfiguration)
	assert.True(t, info.AutoRollbackConfiguration.Enabled)
	assert.Equal(t, []string{"DEPLOYMENT_FAILURE"}, info.AutoRollbackConfiguration.Events)
	require.Len(t, info.Ec2TagFilters, 1)
	assert.Equal(t, "env", info.Ec2TagFilters[0].Key)
	assert.Equal(t, "prod", info.Ec2TagFilters[0].Value)
	assert.Equal(t, "KEY_AND_VALUE", info.Ec2TagFilters[0].Type)
	require.Len(t, info.TriggerConfigurations, 1)
	assert.Equal(t, "deploy-trigger", info.TriggerConfigurations[0].TriggerName)
	assert.Equal(t, "UPDATE", info.OutdatedInstancesStrategy)
	assert.True(t, info.TerminationHookEnabled)
}

func TestHandler_GetDeploymentGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codedeploy.Handler)
		input      map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *codedeploy.Handler) {
				_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
				_, _ = createDG(h.Backend, "my-app", "my-dg", "arn:aws:iam::123:role/role", "", nil)
			},
			input: map[string]any{
				"applicationName":     "my-app",
				"deploymentGroupName": "my-dg",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not_found",
			input:      map[string]any{"applicationName": "missing", "deploymentGroupName": "dg"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "GetDeploymentGroup", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestDeploymentGroups_GetRequiresAppName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "GetDeploymentGroup", map[string]any{
		"deploymentGroupName": "my-dg",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_ListDeploymentGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codedeploy.Handler)
		input      map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *codedeploy.Handler) {
				_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
			},
			input:      map[string]any{"applicationName": "my-app"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_name",
			input:      map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "app_not_found",
			input:      map[string]any{"applicationName": "missing"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "ListDeploymentGroups", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestDeploymentGroups_SortedList(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
	_, _ = createDG(h.Backend, "my-app", "z-group", "", "", nil)
	_, _ = createDG(h.Backend, "my-app", "a-group", "", "", nil)
	_, _ = createDG(h.Backend, "my-app", "m-group", "", "", nil)

	rec := doRequest(t, h, "ListDeploymentGroups", map[string]any{"applicationName": "my-app"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		DeploymentGroups []string `json:"deploymentGroups"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, []string{"a-group", "m-group", "z-group"}, resp.DeploymentGroups)
}

func TestBackend_ListDeploymentGroupDetails(t *testing.T) {
	t.Parallel()

	b := codedeploy.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)

	_, err := b.CreateApplication("my-app", "Server", nil)
	require.NoError(t, err)

	_, err = createDG(b, "my-app", "dg1", "", "", nil)
	require.NoError(t, err)

	_, err = createDG(b, "my-app", "dg2", "", "", nil)
	require.NoError(t, err)

	dgs, err := b.ListDeploymentGroupDetails("my-app")
	require.NoError(t, err)
	assert.Len(t, dgs, 2)

	_, err = b.ListDeploymentGroupDetails("missing")
	require.Error(t, err)
}

func TestHandler_DeleteDeploymentGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codedeploy.Handler)
		input      map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *codedeploy.Handler) {
				_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
				_, _ = createDG(h.Backend, "my-app", "my-dg", "", "", nil)
			},
			input:      map[string]any{"applicationName": "my-app", "deploymentGroupName": "my-dg"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "not_found",
			input:      map[string]any{"applicationName": "missing", "deploymentGroupName": "dg"},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "DeleteDeploymentGroup", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_UpdateDeploymentGroup(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createCDApp(t, h, "update-dg-app")
	doRequest(t, h, "CreateDeploymentGroup", map[string]any{
		"applicationName":     "update-dg-app",
		"deploymentGroupName": "my-dg",
		"serviceRoleArn":      "arn:aws:iam::123456789012:role/CodeDeployRole",
	})

	rec := doRequest(t, h, "UpdateDeploymentGroup", map[string]any{
		"applicationName":            "update-dg-app",
		"currentDeploymentGroupName": "my-dg",
		"newDeploymentGroupName":     "my-dg-v2",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDeploymentGroups_Update(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup          func(h *codedeploy.Handler)
		input          map[string]any
		name           string
		wantStatus     int
		wantHooksClean bool
	}{
		{
			name: "rename",
			setup: func(h *codedeploy.Handler) {
				_, _ = h.Backend.CreateApplication("app", "Server", nil)
				_, _ = createDG(h.Backend, "app", "old-name", "", "", nil)
			},
			input: map[string]any{
				"applicationName":            "app",
				"currentDeploymentGroupName": "old-name",
				"newDeploymentGroupName":     "new-name",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "update_fields",
			setup: func(h *codedeploy.Handler) {
				_, _ = h.Backend.CreateApplication("app2", "Server", nil)
				_, _ = createDG(h.Backend, "app2", "dg", "", "", nil)
			},
			input: map[string]any{
				"applicationName":            "app2",
				"currentDeploymentGroupName": "dg",
				"serviceRoleArn":             "arn:aws:iam::123:role/new-role",
				"deploymentConfigName":       "CodeDeployDefault.OneAtATime",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "hooks_cleaned_on_alarm_removal",
			setup: func(h *codedeploy.Handler) {
				_, _ = h.Backend.CreateApplication("app3", "Server", nil)
				// Create DG with alarms enabled.
				_, _ = h.Backend.CreateDeploymentGroup("app3", "dg", codedeploy.DeploymentGroupInput{
					AlarmConfiguration: &codedeploy.AlarmConfiguration{
						Enabled: true,
						Alarms:  []codedeploy.Alarm{{Name: "cpu"}},
					},
				}, nil)
			},
			input: map[string]any{
				"applicationName":            "app3",
				"currentDeploymentGroupName": "dg",
				// alarmConfiguration omitted → removing alarms
			},
			wantStatus:     http.StatusOK,
			wantHooksClean: true,
		},
		{
			name: "not_found",
			input: map[string]any{
				"applicationName":            "missing-app",
				"currentDeploymentGroupName": "dg",
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "missing_required_fields",
			input: map[string]any{
				"applicationName": "app",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "UpdateDeploymentGroup", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK && tt.wantHooksClean {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				hooksNotCleanedUp, _ := resp["hooksNotCleanedUp"].(bool)
				assert.True(t, hooksNotCleanedUp, "expected hooksNotCleanedUp=true when alarms were removed")
			}
		})
	}
}

func TestDeploymentGroups_UpdateRenameVerifiable(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateApplication("app", "Server", nil)
	_, _ = createDG(h.Backend, "app", "old-name", "", "", nil)

	rec := doRequest(t, h, "UpdateDeploymentGroup", map[string]any{
		"applicationName":            "app",
		"currentDeploymentGroupName": "old-name",
		"newDeploymentGroupName":     "new-name",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Old name should no longer exist.
	rec2 := doRequest(t, h, "GetDeploymentGroup", map[string]any{
		"applicationName":     "app",
		"deploymentGroupName": "old-name",
	})
	assert.Equal(t, http.StatusNotFound, rec2.Code)

	// New name should exist.
	rec3 := doRequest(t, h, "GetDeploymentGroup", map[string]any{
		"applicationName":     "app",
		"deploymentGroupName": "new-name",
	})
	assert.Equal(t, http.StatusOK, rec3.Code)
}

func TestDeploymentGroups_InheritsComputePlatform(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		platform string
	}{
		{name: "server", platform: "Server"},
		{name: "lambda", platform: "Lambda"},
		{name: "ecs", platform: "ECS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, _ = h.Backend.CreateApplication("app-"+tt.name, tt.platform, nil)
			_, _ = createDG(h.Backend, "app-"+tt.name, "dg", "", "", nil)

			rec := doRequest(t, h, "GetDeploymentGroup", map[string]any{
				"applicationName":     "app-" + tt.name,
				"deploymentGroupName": "dg",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				DeploymentGroupInfo struct {
					ComputePlatform string `json:"computePlatform"`
				} `json:"deploymentGroupInfo"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.platform, resp.DeploymentGroupInfo.ComputePlatform)
		})
	}
}

// TestDeploymentGroups_ComputePlatformInheritedViaHandler covers the same
// inherited-computePlatform behavior as TestDeploymentGroups_InheritsComputePlatform
// but drives creation entirely through the handler (rather than seeding the
// backend directly), guarding the wire-level CreateDeploymentGroup path too.
func TestDeploymentGroups_ComputePlatformInheritedViaHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		platform string
	}{
		{"Server"},
		{"Lambda"},
		{"ECS"},
	}

	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateApplication", map[string]any{
				"applicationName": "cp-app-" + tt.platform,
				"computePlatform": tt.platform,
			})
			doRequest(t, h, "CreateDeploymentGroup", map[string]any{
				"applicationName":     "cp-app-" + tt.platform,
				"deploymentGroupName": "dg",
				"serviceRoleArn":      "arn:aws:iam::000000000000:role/role",
			})

			rec := doRequest(t, h, "GetDeploymentGroup", map[string]any{
				"applicationName":     "cp-app-" + tt.platform,
				"deploymentGroupName": "dg",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				DeploymentGroupInfo struct {
					ComputePlatform string `json:"computePlatform"`
				} `json:"deploymentGroupInfo"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, tt.platform, out.DeploymentGroupInfo.ComputePlatform,
				"deployment group must inherit computePlatform from application")
		})
	}
}

func TestHandler_BatchGetDeploymentGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *codedeploy.Handler)
		input      map[string]any
		name       string
		wantCount  int
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *codedeploy.Handler) {
				_, _ = h.Backend.CreateApplication("my-app", "Server", nil)
				_, _ = createDG(h.Backend, "my-app", "dg1", "", "", nil)
				_, _ = createDG(h.Backend, "my-app", "dg2", "", "", nil)
			},
			input: map[string]any{
				"applicationName":      "my-app",
				"deploymentGroupNames": []string{"dg1", "dg2", "missing"},
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "missing_app_name",
			input:      map[string]any{"deploymentGroupNames": []string{"dg1"}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "app_not_found",
			input:      map[string]any{"applicationName": "no-such-app", "deploymentGroupNames": []string{"dg1"}},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "BatchGetDeploymentGroups", tt.input)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				infos, ok := resp["deploymentGroupsInfo"].([]any)
				require.True(t, ok)
				assert.Len(t, infos, tt.wantCount)
			}
		})
	}
}

func TestDeploymentGroups_BatchGetEmptySlice(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, _ = h.Backend.CreateApplication("my-app", "Server", nil)

	rec := doRequest(t, h, "BatchGetDeploymentGroups", map[string]any{
		"applicationName":      "my-app",
		"deploymentGroupNames": []string{"nonexistent"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		DeploymentGroupsInfo []any `json:"deploymentGroupsInfo"`
	}

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	// Should be an empty array not null
	assert.NotNil(t, resp.DeploymentGroupsInfo)
	assert.Empty(t, resp.DeploymentGroupsInfo)
}

func TestDeploymentGroups_BatchGetRichFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateApplication("app", "ECS", nil)
	_, _ = h.Backend.CreateDeploymentGroup("app", "dg1", codedeploy.DeploymentGroupInput{
		ECSServices: []codedeploy.ECSService{{ServiceName: "my-svc", ClusterName: "my-cluster"}},
		DeploymentStyle: &codedeploy.DeploymentStyle{
			DeploymentType:   "BLUE_GREEN",
			DeploymentOption: "WITH_TRAFFIC_CONTROL",
		},
	}, nil)

	rec := doRequest(t, h, "BatchGetDeploymentGroups", map[string]any{
		"applicationName":      "app",
		"deploymentGroupNames": []string{"dg1"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		DeploymentGroupsInfo []struct {
			ComputePlatform string `json:"computePlatform"`
			DeploymentStyle *struct {
				DeploymentType string `json:"deploymentType"`
			} `json:"deploymentStyle"`
			ECSServices []struct {
				ServiceName string `json:"serviceName"`
			} `json:"ecsServices"`
		} `json:"deploymentGroupsInfo"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.DeploymentGroupsInfo, 1)

	info := resp.DeploymentGroupsInfo[0]
	assert.Equal(t, "ECS", info.ComputePlatform)
	require.NotNil(t, info.DeploymentStyle)
	assert.Equal(t, "BLUE_GREEN", info.DeploymentStyle.DeploymentType)
	require.Len(t, info.ECSServices, 1)
	assert.Equal(t, "my-svc", info.ECSServices[0].ServiceName)
}

func TestDeploymentGroups_ARN(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	dgARN := h.Backend.DeploymentGroupARN("my-app", "my-dg")

	assert.Contains(t, dgARN, "deploymentgroup:my-app/my-dg")
}
