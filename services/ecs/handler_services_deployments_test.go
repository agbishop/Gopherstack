package ecs_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecs"
)

func TestECS_ListServices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*ecs.Handler)
		input    map[string]any
		name     string
		wantCode int
		wantLen  int
	}{
		{
			name: "empty list",
			setup: func(h *ecs.Handler) {
				doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "list-svc-empty"})
			},
			input:    map[string]any{"cluster": "list-svc-empty"},
			wantCode: http.StatusOK,
			wantLen:  0,
		},
		{
			name: "two services",
			setup: func(h *ecs.Handler) {
				tdArn := registerTestTaskDef(t, h, "list-svc-td")
				doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "list-svc-two"})
				doECSRequest(t, h, "CreateService", map[string]any{
					"cluster": "list-svc-two", "serviceName": "svc-a", "taskDefinition": tdArn, "desiredCount": 1,
				})
				doECSRequest(t, h, "CreateService", map[string]any{
					"cluster": "list-svc-two", "serviceName": "svc-b", "taskDefinition": tdArn, "desiredCount": 1,
				})
			},
			input:    map[string]any{"cluster": "list-svc-two"},
			wantCode: http.StatusOK,
			wantLen:  2,
		},
		{
			name: "filter by launchType returns subset",
			setup: func(h *ecs.Handler) {
				tdArn := registerTestTaskDef(t, h, "list-svc-lt-td")
				doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "list-svc-lt"})
				doECSRequest(t, h, "CreateService", map[string]any{
					"cluster": "list-svc-lt", "serviceName": "svc-fargate", "taskDefinition": tdArn,
					"desiredCount": 1, "launchType": "FARGATE",
				})
				doECSRequest(t, h, "CreateService", map[string]any{
					"cluster": "list-svc-lt", "serviceName": "svc-ec2", "taskDefinition": tdArn,
					"desiredCount": 1, "launchType": "EC2",
				})
			},
			input:    map[string]any{"cluster": "list-svc-lt", "launchType": "FARGATE"},
			wantCode: http.StatusOK,
			wantLen:  1,
		},
		{
			name: "filter by schedulingStrategy returns subset",
			setup: func(h *ecs.Handler) {
				tdArn := registerTestTaskDef(t, h, "list-svc-ss-td")
				doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "list-svc-ss"})
				doECSRequest(t, h, "CreateService", map[string]any{
					"cluster": "list-svc-ss", "serviceName": "svc-replica", "taskDefinition": tdArn,
					"desiredCount": 1, "schedulingStrategy": "REPLICA",
				})
				doECSRequest(t, h, "CreateService", map[string]any{
					"cluster": "list-svc-ss", "serviceName": "svc-daemon", "taskDefinition": tdArn,
					"desiredCount": 0, "schedulingStrategy": "DAEMON",
				})
			},
			input:    map[string]any{"cluster": "list-svc-ss", "schedulingStrategy": "DAEMON"},
			wantCode: http.StatusOK,
			wantLen:  1,
		},
		{
			name:     "unknown cluster",
			setup:    func(_ *ecs.Handler) {},
			input:    map[string]any{"cluster": "nonexistent"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			tt.setup(h)
			rec := doECSRequest(t, h, "ListServices", tt.input)

			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				arns := resp["serviceArns"].([]any)
				assert.Len(t, arns, tt.wantLen)
			}
		})
	}
}

func TestECS_DeleteService_CleansUpTaskSets(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	_, err := backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "svccleanup-cluster"})
	require.NoError(t, err)

	td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "svccleanup-td",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	svc, err := backend.CreateService(ecs.CreateServiceInput{
		Cluster:        "svccleanup-cluster",
		ServiceName:    "svccleanup-svc",
		TaskDefinition: td.TaskDefinitionArn,
		DesiredCount:   1,
	})
	require.NoError(t, err)

	_, err = backend.CreateTaskSet(ecs.CreateTaskSetInput{
		Cluster:        "svccleanup-cluster",
		Service:        svc.ServiceArn,
		TaskDefinition: td.TaskDefinitionArn,
	})
	require.NoError(t, err)

	// Delete the service — task sets should be cleaned up. Force bypasses
	// the desiredCount/runningCount guard since this test isn't exercising it.
	_, err = backend.DeleteService("svccleanup-cluster", "svccleanup-svc", true)
	require.NoError(t, err)

	// Recreate the service with the same name — no stale task sets.
	svc2, err := backend.CreateService(ecs.CreateServiceInput{
		Cluster:        "svccleanup-cluster",
		ServiceName:    "svccleanup-svc",
		TaskDefinition: td.TaskDefinitionArn,
		DesiredCount:   1,
	})
	require.NoError(t, err)

	sets, _, err := backend.DescribeTaskSets("svccleanup-cluster", svc2.ServiceArn, nil)
	require.NoError(t, err)
	assert.Empty(t, sets, "no stale task sets after service delete+recreate")
}

func TestDescribeServices_FailureSemantics(t *testing.T) {
	t.Parallel()

	t.Run("unknown returns failure", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "acc-ops2-svc-notfound"})

		rec := doECSRequest(t, h, "DescribeServices", map[string]any{
			"cluster":  "acc-ops2-svc-notfound",
			"services": []string{"ghost-service"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		services, _ := resp["services"].([]any)
		assert.Empty(t, services)

		failures, _ := resp["failures"].([]any)
		require.Len(t, failures, 1)

		f := failures[0].(map[string]any)
		assert.Equal(t, "ghost-service", f["arn"])
		assert.Equal(t, "MISSING", f["reason"])
	})

	t.Run("mix found and missing", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		tdArn := registerTestTaskDef(t, h, "acc-ops2-svc-task")
		doECSRequest(t, h, "CreateService", map[string]any{
			"serviceName":    "real-svc",
			"taskDefinition": tdArn,
			"desiredCount":   0,
		})

		rec := doECSRequest(t, h, "DescribeServices", map[string]any{
			"services": []string{"real-svc", "ghost-svc"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		services, _ := resp["services"].([]any)
		assert.Len(t, services, 1)

		svc := services[0].(map[string]any)
		assert.Equal(t, "real-svc", svc["serviceName"])

		failures, _ := resp["failures"].([]any)
		require.Len(t, failures, 1)

		f := failures[0].(map[string]any)
		assert.Equal(t, "ghost-svc", f["arn"])
	})

	t.Run("cluster not found still errors", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doECSRequest(t, h, "DescribeServices", map[string]any{
			"cluster":  "nonexistent-cluster",
			"services": []string{"any-svc"},
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("empty list returns all", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		tdArn := registerTestTaskDef(t, h, "acc-ops2-list-task")
		doECSRequest(t, h, "CreateService", map[string]any{
			"serviceName":    "svc-a",
			"taskDefinition": tdArn,
			"desiredCount":   0,
		})
		doECSRequest(t, h, "CreateService", map[string]any{
			"serviceName":    "svc-b",
			"taskDefinition": tdArn,
			"desiredCount":   0,
		})

		rec := doECSRequest(t, h, "DescribeServices", map[string]any{
			"services": []string{},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		services, _ := resp["services"].([]any)
		assert.Len(t, services, 2)

		failures, _ := resp["failures"].([]any)
		assert.Empty(t, failures)
	})
}

func TestHandler_CreateService_PropagateTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_ = doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "myapp",
		"containerDefinitions": []map[string]any{{"name": "app", "image": "nginx"}},
	})

	rec := doECSRequest(t, h, "CreateService", map[string]any{
		"serviceName":    "my-svc",
		"taskDefinition": "myapp",
		"desiredCount":   0,
		"propagateTags":  "SERVICE",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	svc := out["service"].(map[string]any)
	assert.Equal(t, "SERVICE", svc["propagateTags"])
}

func TestHandler_CreateService_PropagateTags_DefaultNone(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_ = doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "myapp",
		"containerDefinitions": []map[string]any{{"name": "app", "image": "nginx"}},
	})

	rec := doECSRequest(t, h, "CreateService", map[string]any{
		"serviceName":    "my-svc",
		"taskDefinition": "myapp",
		"desiredCount":   0,
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	svc := out["service"].(map[string]any)
	assert.Equal(t, "NONE", svc["propagateTags"])
}

func TestCreateService_DeploymentsField_Populated(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Register task def.
	tdRec := doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "webapp",
		"containerDefinitions": []map[string]any{{"name": "app", "image": "nginx"}},
	})
	require.Equal(t, http.StatusOK, tdRec.Code)
	var tdOut map[string]any
	require.NoError(t, json.Unmarshal(tdRec.Body.Bytes(), &tdOut))
	tdArn := tdOut["taskDefinition"].(map[string]any)["taskDefinitionArn"].(string)

	// Create cluster + service.
	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "prod"})

	svcRec := doECSRequest(t, h, "CreateService", map[string]any{
		"cluster":        "prod",
		"serviceName":    "myapp",
		"taskDefinition": tdArn,
		"desiredCount":   2,
	})
	require.Equal(t, http.StatusOK, svcRec.Code, svcRec.Body.String())

	var svcOut map[string]any
	require.NoError(t, json.Unmarshal(svcRec.Body.Bytes(), &svcOut))
	svc := svcOut["service"].(map[string]any)

	require.Contains(t, svc, "deployments", "service response must include deployments")
	deployments := svc["deployments"].([]any)
	require.Len(t, deployments, 1, "CreateService must produce exactly one deployment")

	d := deployments[0].(map[string]any)
	assert.Equal(t, "PRIMARY", d["status"])
	assert.Equal(t, tdArn, d["taskDefinition"])
	assert.NotEmpty(t, d["id"])
	assert.InDelta(t, 2.0, d["desiredCount"], 0)
}

func TestUpdateService_DeploymentRotation_OnTaskDefChange(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Register two task definitions.
	tdRec1 := doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "api",
		"containerDefinitions": []map[string]any{{"name": "app", "image": "api:1"}},
	})
	require.Equal(t, http.StatusOK, tdRec1.Code)
	var out1 map[string]any
	require.NoError(t, json.Unmarshal(tdRec1.Body.Bytes(), &out1))
	tdArn1 := out1["taskDefinition"].(map[string]any)["taskDefinitionArn"].(string)

	tdRec2 := doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "api",
		"containerDefinitions": []map[string]any{{"name": "app", "image": "api:2"}},
	})
	require.Equal(t, http.StatusOK, tdRec2.Code)
	var out2 map[string]any
	require.NoError(t, json.Unmarshal(tdRec2.Body.Bytes(), &out2))
	tdArn2 := out2["taskDefinition"].(map[string]any)["taskDefinitionArn"].(string)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "staging"})

	svcRec := doECSRequest(t, h, "CreateService", map[string]any{
		"cluster":        "staging",
		"serviceName":    "api",
		"taskDefinition": tdArn1,
		"desiredCount":   1,
	})
	require.Equal(t, http.StatusOK, svcRec.Code)

	// Update service to new task definition.
	updRec := doECSRequest(t, h, "UpdateService", map[string]any{
		"cluster":        "staging",
		"service":        "api",
		"taskDefinition": tdArn2,
	})
	require.Equal(t, http.StatusOK, updRec.Code, updRec.Body.String())

	var updOut map[string]any
	require.NoError(t, json.Unmarshal(updRec.Body.Bytes(), &updOut))
	svc := updOut["service"].(map[string]any)

	deployments := svc["deployments"].([]any)
	require.Len(t, deployments, 2, "UpdateService must produce two deployments")

	// First deployment should be the new PRIMARY.
	d0 := deployments[0].(map[string]any)
	assert.Equal(t, "PRIMARY", d0["status"])
	assert.Equal(t, tdArn2, d0["taskDefinition"])

	// Second deployment should be the old PRIMARY demoted to ACTIVE.
	d1 := deployments[1].(map[string]any)
	assert.Equal(t, "ACTIVE", d1["status"])
	assert.Equal(t, tdArn1, d1["taskDefinition"])
}

func TestDescribeServices_DeploymentsField(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tdRec := doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "dstest",
		"containerDefinitions": []map[string]any{{"name": "app", "image": "nginx"}},
	})
	require.Equal(t, http.StatusOK, tdRec.Code)
	var tdOut map[string]any
	require.NoError(t, json.Unmarshal(tdRec.Body.Bytes(), &tdOut))
	tdArn := tdOut["taskDefinition"].(map[string]any)["taskDefinitionArn"].(string)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "ds-cluster"})

	doECSRequest(t, h, "CreateService", map[string]any{
		"cluster":        "ds-cluster",
		"serviceName":    "ds-svc",
		"taskDefinition": tdArn,
		"desiredCount":   1,
	})

	descRec := doECSRequest(t, h, "DescribeServices", map[string]any{
		"cluster":  "ds-cluster",
		"services": []string{"ds-svc"},
	})
	require.Equal(t, http.StatusOK, descRec.Code, descRec.Body.String())

	var descOut map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))

	services := descOut["services"].([]any)
	require.Len(t, services, 1)

	svc := services[0].(map[string]any)
	deployments, ok := svc["deployments"].([]any)
	require.True(t, ok, "DescribeServices must include deployments")
	require.NotEmpty(t, deployments)

	d := deployments[0].(map[string]any)
	assert.Equal(t, "PRIMARY", d["status"])
}

// TestECS_ListServicesByNamespace verifies ListServicesByNamespace.
func TestECS_ListServicesByNamespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    map[string]any
		name     string
		wantCode int
	}{
		{
			name:     "empty cluster",
			input:    map[string]any{"cluster": "default"},
			wantCode: http.StatusOK,
		},
		{
			name:     "namespace filter",
			input:    map[string]any{"cluster": "default", "namespace": "frontend"},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doECSRequest(t, h, "ListServicesByNamespace", tt.input)
			require.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestECS_CreateService_Tags verifies service tags are stored and returned.
func TestECS_CreateService_Tags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "default"})
	doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "tag-svc-td",
		"containerDefinitions": []map[string]any{{"name": "c1", "image": "nginx"}},
	})

	rec := doECSRequest(t, h, "CreateService", map[string]any{
		"serviceName":    "tag-svc",
		"taskDefinition": "tag-svc-td",
		"desiredCount":   1,
		"tags":           []map[string]any{{"key": "env", "value": "staging"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	svc, ok := resp["service"].(map[string]any)
	require.True(t, ok)

	tags, ok := svc["tags"].([]any)
	require.True(t, ok)
	require.Len(t, tags, 1)

	tag := tags[0].(map[string]any)
	assert.Equal(t, "env", tag["key"])
	assert.Equal(t, "staging", tag["value"])
}

func TestPlacementStrategy_ForwardedFromService(t *testing.T) {
	t.Parallel()

	b := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	// Register two container instances so the strategy has something to choose between.
	ci1, err := b.RegisterContainerInstance("default", "i-aaaa0001")
	require.NoError(t, err)
	ci2, err := b.RegisterContainerInstance("default", "i-aaaa0002")
	require.NoError(t, err)

	tdArn := makeTaskDef(t, b, "spread-td")

	svc, err := b.CreateService(ecs.CreateServiceInput{
		ServiceName:    "spread-svc",
		TaskDefinition: tdArn,
		DesiredCount:   2,
		LaunchType:     "EC2",
		PlacementStrategy: []ecs.PlacementStrategy{
			{Type: "spread", Field: "instanceId"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, svc)

	// Two tasks via StartTaskForService — spread picks the least-loaded instance each time.
	require.NoError(t, b.StartTaskForService("default", "spread-svc", tdArn))
	require.NoError(t, b.StartTaskForService("default", "spread-svc", tdArn))

	tasks, _, err := b.DescribeTasks("default", nil)
	require.NoError(t, err)

	instanceCounts := make(map[string]int)
	for _, task := range tasks {
		if task.ContainerInstanceArn != "" {
			instanceCounts[task.ContainerInstanceArn]++
		}
	}

	assert.Equal(t, 1, instanceCounts[ci1.ContainerInstanceArn],
		"ci1 should host exactly one task with spread strategy")
	assert.Equal(t, 1, instanceCounts[ci2.ContainerInstanceArn],
		"ci2 should host exactly one task with spread strategy")
}

func TestDeploymentRolloutState_CompletedWhenDesiredMet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "rollout-cluster"})
	doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "rollout-td",
		"containerDefinitions": []any{map[string]any{"name": "app", "image": "nginx"}},
	})

	createResp := doECSRequest(t, h, "CreateService", map[string]any{
		"cluster":        "rollout-cluster",
		"serviceName":    "rollout-svc",
		"taskDefinition": "rollout-td",
		"desiredCount":   2,
	})
	require.Equal(t, http.StatusOK, createResp.Code)

	// Run 2 tasks under this service's group so enrichService counts them.
	for range 2 {
		runResp := doECSRequest(t, h, "RunTask", map[string]any{
			"cluster":        "rollout-cluster",
			"taskDefinition": "rollout-td",
			"group":          "service:rollout-svc",
		})
		require.Equal(t, http.StatusOK, runResp.Code)
	}

	descResp := doECSRequest(t, h, "DescribeServices", map[string]any{
		"cluster":  "rollout-cluster",
		"services": []any{"rollout-svc"},
	})
	require.Equal(t, http.StatusOK, descResp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(descResp.Body.Bytes(), &out))

	svcs, ok := out["services"].([]any)
	require.True(t, ok)
	require.Len(t, svcs, 1)

	svc := svcs[0].(map[string]any)
	deployments, ok := svc["deployments"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, deployments)

	primary := deployments[0].(map[string]any)
	assert.Equal(t, "PRIMARY", primary["status"])
	assert.Equal(t, "COMPLETED", primary["rolloutState"],
		"PRIMARY deployment must advance to COMPLETED when running >= desired")
	assert.Equal(t, 2, int(primary["runningCount"].(float64)),
		"per-deployment runningCount must reflect live tasks")
}
