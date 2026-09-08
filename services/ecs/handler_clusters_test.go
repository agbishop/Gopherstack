package ecs_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecs"
)

func TestECS_CreateCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    map[string]any
		name     string
		wantName string
		wantCode int
	}{
		{
			name:     "with explicit name",
			input:    map[string]any{"clusterName": "my-cluster"},
			wantCode: http.StatusOK,
			wantName: "my-cluster",
		},
		{
			name:     "with empty name defaults to 'default'",
			input:    map[string]any{},
			wantCode: http.StatusOK,
			wantName: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doECSRequest(t, h, "CreateCluster", tt.input)

			require.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			cluster, ok := resp["cluster"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.wantName, cluster["clusterName"])
			assert.NotEmpty(t, cluster["clusterArn"])
			assert.Equal(t, "ACTIVE", cluster["status"])
		})
	}
}

// TestECS_CreateCluster_Idempotent covers real ECS's documented idempotent
// behavior: calling CreateCluster again with an existing ClusterName returns
// the existing cluster (HTTP 200), not an error. "ClusterAlreadyExistsException"
// is not a real ECS exception type -- it appears in no per-op
// deserializeOpError switch and has no shape in ecs@v1.90.0/types/errors.go.
func TestECS_CreateCluster_Idempotent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "dupe"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "dupe"})
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "\"clusterName\":\"dupe\"")
}

func TestECS_DescribeClusters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		clusters     []string
		filter       []string
		wantCode     int
		wantCount    int
		wantFailures int
	}{
		{
			// DescribeClustersInput.Clusters doc: "If you do not specify a
			// cluster, the default cluster is assumed." Omitting the filter
			// describes the "default" cluster, not every cluster in the
			// account -- ListClusters (a different operation, no such
			// default-substitution language) is the one that returns
			// everything.
			name:      "empty describes default cluster only",
			clusters:  []string{"cluster-a", "cluster-b"},
			filter:    nil,
			wantCode:  http.StatusOK,
			wantCount: 1,
		},
		{
			name:      "filter by name",
			clusters:  []string{"cluster-a", "cluster-b"},
			filter:    []string{"cluster-a"},
			wantCode:  http.StatusOK,
			wantCount: 1,
		},
		{
			name:         "not found returns failure not error",
			clusters:     []string{},
			filter:       []string{"nonexistent"},
			wantCode:     http.StatusOK,
			wantFailures: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for _, name := range tt.clusters {
				rec := doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": name})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			input := map[string]any{}
			if tt.filter != nil {
				input["clusters"] = tt.filter
			}

			rec := doECSRequest(t, h, "DescribeClusters", input)
			require.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			if tt.wantCount > 0 {
				clusters, ok := resp["clusters"].([]any)
				require.True(t, ok)
				assert.Len(t, clusters, tt.wantCount)
			}

			if tt.wantFailures > 0 {
				failures, _ := resp["failures"].([]any)
				assert.Len(t, failures, tt.wantFailures)
			}
		})
	}
}

func TestECS_DeleteCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		create   string
		delete   string
		wantCode int
	}{
		{
			name:     "success",
			create:   "my-cluster",
			delete:   "my-cluster",
			wantCode: http.StatusOK,
		},
		{
			name:     "not found",
			create:   "",
			delete:   "nonexistent",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.create != "" {
				rec := doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": tt.create})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doECSRequest(t, h, "DeleteCluster", map[string]any{"cluster": tt.delete})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestECS_DeleteCluster_DependencyViolation verifies AWS's DeleteCluster
// guard: a cluster still holding services, active tasks, or registered
// container instances can't be deleted -- it does not cascade.
func TestECS_DeleteCluster_DependencyViolation(t *testing.T) {
	t.Parallel()

	t.Run("has_service", func(t *testing.T) {
		t.Parallel()

		backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

		td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
			Family:               "dcg-svc-td",
			ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
		})
		require.NoError(t, err)

		_, err = backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "dcg-svc-cluster"})
		require.NoError(t, err)

		_, err = backend.CreateService(ecs.CreateServiceInput{
			Cluster:        "dcg-svc-cluster",
			ServiceName:    "dcg-svc",
			TaskDefinition: td.TaskDefinitionArn,
			DesiredCount:   0,
		})
		require.NoError(t, err)

		_, err = backend.DeleteCluster("dcg-svc-cluster")
		require.Error(t, err, "DeleteCluster must fail while the cluster has a service")

		clusters, _, err := backend.DescribeClusters([]string{"dcg-svc-cluster"})
		require.NoError(t, err)
		require.Len(t, clusters, 1, "cluster must survive a failed DeleteCluster")

		_, err = backend.DeleteService("dcg-svc-cluster", "dcg-svc")
		require.NoError(t, err)

		_, err = backend.DeleteCluster("dcg-svc-cluster")
		require.NoError(t, err, "DeleteCluster must succeed once the service is gone")
	})

	t.Run("has_active_task", func(t *testing.T) {
		t.Parallel()

		backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

		td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
			Family:               "dcg-task-td",
			ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
		})
		require.NoError(t, err)

		_, err = backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "dcg-task-cluster"})
		require.NoError(t, err)

		tasks, err := backend.RunTask(ecs.RunTaskInput{
			Cluster:        "dcg-task-cluster",
			TaskDefinition: td.TaskDefinitionArn,
		})
		require.NoError(t, err)
		require.Len(t, tasks, 1)

		_, err = backend.DeleteCluster("dcg-task-cluster")
		require.Error(t, err, "DeleteCluster must fail while the cluster has an active task")

		_, err = backend.StopTask("dcg-task-cluster", tasks[0].TaskArn, "test cleanup")
		require.NoError(t, err)

		_, err = backend.DeleteCluster("dcg-task-cluster")
		require.NoError(t, err, "DeleteCluster must succeed once the task is stopped")
	})

	t.Run("has_container_instance", func(t *testing.T) {
		t.Parallel()

		backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

		_, err := backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "dcg-ci-cluster"})
		require.NoError(t, err)

		ci, err := backend.RegisterContainerInstance("dcg-ci-cluster", "i-dcgci123")
		require.NoError(t, err)

		_, err = backend.DeleteCluster("dcg-ci-cluster")
		require.Error(t, err, "DeleteCluster must fail while the cluster has a registered container instance")

		_, err = backend.DeregisterContainerInstance("dcg-ci-cluster", ci.ContainerInstanceArn, true)
		require.NoError(t, err)

		_, err = backend.DeleteCluster("dcg-ci-cluster")
		require.NoError(t, err, "DeleteCluster must succeed once the container instance is deregistered")
	})
}

func TestECS_Backend_DefaultClusterAutoCreated(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	// Register a task definition.
	td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "auto-cluster-task",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	// Run a task - default cluster should be auto-created.
	tasks, err := backend.RunTask(ecs.RunTaskInput{
		TaskDefinition: td.TaskDefinitionArn,
		Count:          1,
	})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "RUNNING", tasks[0].LastStatus)
}

func TestECS_Backend_ClusterKey_ARN(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	// CreateCluster, then describe using ARN.
	c, err := backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "arn-test"})
	require.NoError(t, err)

	clusters, _, err := backend.DescribeClusters([]string{c.ClusterArn})
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.Equal(t, "arn-test", clusters[0].ClusterName)
}

func TestECS_Backend_EnrichCluster_PendingTasks(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	_, err := backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "enrich-cluster"})
	require.NoError(t, err)

	td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "enrich-task",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	// Run tasks so cluster has running count.
	_, err = backend.RunTask(ecs.RunTaskInput{
		Cluster:        "enrich-cluster",
		TaskDefinition: td.TaskDefinitionArn,
		Count:          2,
	})
	require.NoError(t, err)

	clusters, _, err := backend.DescribeClusters([]string{"enrich-cluster"})
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.Equal(t, 2, clusters[0].RunningTasksCount)
}

func TestDeleteCluster_CleansTaskSets_HandlerLevel(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "del-ts-cluster"})
	doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "del-ts-task",
		"containerDefinitions": []any{map[string]any{"name": "app", "image": "nginx"}},
	})
	doECSRequest(t, h, "CreateService", map[string]any{
		"cluster":              "del-ts-cluster",
		"serviceName":          "del-ts-svc",
		"taskDefinition":       "del-ts-task",
		"desiredCount":         0,
		"deploymentController": map[string]any{"type": "EXTERNAL"},
	})
	doECSRequest(t, h, "CreateTaskSet", map[string]any{
		"cluster":        "del-ts-cluster",
		"service":        "del-ts-svc",
		"taskDefinition": "del-ts-task",
		"scale":          map[string]any{"unit": "PERCENT", "value": 100},
	})

	doECSRequest(t, h, "DeleteService", map[string]any{
		"cluster": "del-ts-cluster",
		"service": "del-ts-svc",
		"force":   true,
	})
	deleteResp := doECSRequest(t, h, "DeleteCluster", map[string]any{
		"cluster": "del-ts-cluster",
	})
	require.Equal(t, http.StatusOK, deleteResp.Code)
}

func TestCluster_PutCapacityProviders_WithStrategy(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "pcps-cluster"})

	resp := doECSRequest(t, h, "PutClusterCapacityProviders", map[string]any{
		"cluster":           "pcps-cluster",
		"capacityProviders": []string{"FARGATE", "FARGATE_SPOT"},
		"defaultCapacityProviderStrategy": []any{
			map[string]any{"capacityProvider": "FARGATE", "weight": 1, "base": 1},
			map[string]any{"capacityProvider": "FARGATE_SPOT", "weight": 4},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	cluster := out["cluster"].(map[string]any)
	providers := cluster["capacityProviders"].([]any)
	assert.Len(t, providers, 2)

	strategy := cluster["defaultCapacityProviderStrategy"].([]any)
	assert.Len(t, strategy, 2)
}

func TestClusterSetting_ContainerInsights_Enable(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "insights-cluster"})

	resp := doECSRequest(t, h, "UpdateClusterSettings", map[string]any{
		"cluster": "insights-cluster",
		"settings": []any{
			map[string]any{"name": "containerInsights", "value": "enabled"},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	cluster := out["cluster"].(map[string]any)
	settings := cluster["settings"].([]any)
	require.Len(t, settings, 1)
	assert.Equal(t, "containerInsights", settings[0].(map[string]any)["name"])
	assert.Equal(t, "enabled", settings[0].(map[string]any)["value"])
}

func TestClusterSetting_ContainerInsights_Disable(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{
		"clusterName": "insights-dis-cluster",
		"settings": []any{
			map[string]any{"name": "containerInsights", "value": "enabled"},
		},
	})

	resp := doECSRequest(t, h, "UpdateClusterSettings", map[string]any{
		"cluster": "insights-dis-cluster",
		"settings": []any{
			map[string]any{"name": "containerInsights", "value": "disabled"},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	cluster := out["cluster"].(map[string]any)
	settings := cluster["settings"].([]any)
	assert.Equal(t, "disabled", settings[0].(map[string]any)["value"])
}

func TestCreateCluster_WithSettings(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	resp := doECSRequest(t, h, "CreateCluster", map[string]any{
		"clusterName": "with-settings-cluster",
		"settings": []any{
			map[string]any{"name": "containerInsights", "value": "enabled"},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	cluster := out["cluster"].(map[string]any)
	rawSettings := cluster["settings"]
	require.NotNil(t, rawSettings, "settings should be present in response")
	settings := rawSettings.([]any)
	require.Len(t, settings, 1)
	assert.Equal(t, "containerInsights", settings[0].(map[string]any)["name"])
	assert.Equal(t, "enabled", settings[0].(map[string]any)["value"])
}

// TestUpdateCluster_DoesNotAcceptCapacityProviders proves UpdateCluster doesn't
// wire up capacityProviders/defaultCapacityProviderStrategy: the real
// UpdateClusterRequest has neither field, so even if a caller sends them anyway
// they must be silently ignored rather than applied.
func TestUpdateCluster_DoesNotAcceptCapacityProviders(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "upd-cp-cluster"})
	doECSRequest(t, h, "CreateCapacityProvider", map[string]any{"name": "custom-cp"})

	resp := doECSRequest(t, h, "UpdateCluster", map[string]any{
		"cluster":           "upd-cp-cluster",
		"capacityProviders": []string{"FARGATE", "custom-cp"},
		"defaultCapacityProviderStrategy": []any{
			map[string]any{"capacityProvider": "FARGATE", "weight": 1},
		},
	})
	require.Equal(t, http.StatusOK, resp.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	cluster := out["cluster"].(map[string]any)
	providers, ok := cluster["capacityProviders"].([]any)
	require.True(t, ok)
	assert.Empty(
		t,
		providers,
		"UpdateCluster must not apply capacityProviders -- only PutClusterCapacityProviders can",
	)
}

func TestDescribeClusters_MultipleClusters(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, name := range []string{"cluster-x", "cluster-y", "cluster-z"} {
		doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": name})
	}

	descResp := doECSRequest(t, h, "DescribeClusters", map[string]any{
		"clusters": []string{"cluster-x", "cluster-y", "cluster-z"},
	})
	require.Equal(t, http.StatusOK, descResp.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(descResp.Body.Bytes(), &out))
	clusters := out["clusters"].([]any)
	assert.Len(t, clusters, 3)
}

func TestListClusters_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 5 {
		doECSRequest(t, h, "CreateCluster", map[string]any{
			"clusterName": "page-cluster-" + string(rune('a'+i)),
		})
	}

	resp := doECSRequest(t, h, "ListClusters", map[string]any{
		"maxResults": 3,
	})
	require.Equal(t, http.StatusOK, resp.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))
	arns := out["clusterArns"].([]any)
	assert.Len(t, arns, 3)
	assert.NotEmpty(t, out["nextToken"])
}

func TestECS_DeleteCluster_CleansUpTaskSets(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	_, err := backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "cleanup-cluster"})
	require.NoError(t, err)

	td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "cleanup-td",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	svc, err := backend.CreateService(ecs.CreateServiceInput{
		Cluster:        "cleanup-cluster",
		ServiceName:    "cleanup-svc",
		TaskDefinition: td.TaskDefinitionArn,
		DesiredCount:   1,
	})
	require.NoError(t, err)

	_, err = backend.CreateTaskSet(ecs.CreateTaskSetInput{
		Cluster:        "cleanup-cluster",
		Service:        svc.ServiceArn,
		TaskDefinition: td.TaskDefinitionArn,
	})
	require.NoError(t, err)

	// DeleteCluster now refuses while the cluster still has a service, so
	// delete the service first (force bypasses the desiredCount guard --
	// irrelevant to what this test covers) -- this is also what cleans up
	// the task set (see deleteTaskSetsForServiceLocked in DeleteService).
	_, err = backend.DeleteService("cleanup-cluster", "cleanup-svc", true)
	require.NoError(t, err)

	_, err = backend.DeleteCluster("cleanup-cluster")
	require.NoError(t, err)

	// Recreate the cluster and service with the same names — no stale task sets.
	_, err = backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "cleanup-cluster"})
	require.NoError(t, err)

	svc2, err := backend.CreateService(ecs.CreateServiceInput{
		Cluster:        "cleanup-cluster",
		ServiceName:    "cleanup-svc",
		TaskDefinition: td.TaskDefinitionArn,
		DesiredCount:   1,
	})
	require.NoError(t, err)

	sets, _, err := backend.DescribeTaskSets("cleanup-cluster", svc2.ServiceArn, nil)
	require.NoError(t, err)
	assert.Empty(t, sets, "no stale task sets after cluster delete+recreate")
}

func TestECS_PutClusterCapacityProviders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		capacityProviders []string
		wantCode          int
	}{
		{
			name:              "sets capacity providers",
			capacityProviders: []string{"FARGATE", "FARGATE_SPOT"},
			wantCode:          http.StatusOK,
		},
		{
			name:              "empty capacity providers",
			capacityProviders: []string{},
			wantCode:          http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "cp-cluster"})

			rec := doECSRequest(t, h, "PutClusterCapacityProviders", map[string]any{
				"cluster":           "cp-cluster",
				"capacityProviders": tt.capacityProviders,
			})
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			cluster, ok := resp["cluster"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "cp-cluster", cluster["clusterName"])
		})
	}
}

func TestECS_UpdateClusterSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		settings []map[string]any
		wantCode int
	}{
		{
			name:     "updates cluster settings",
			settings: []map[string]any{{"name": "containerInsights", "value": "enabled"}},
			wantCode: http.StatusOK,
		},
		{
			name:     "not found cluster",
			settings: []map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			cluster := "settings-cluster"
			if tt.wantCode == http.StatusOK {
				doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": cluster})
			} else {
				cluster = "nonexistent-cluster"
			}

			rec := doECSRequest(t, h, "UpdateClusterSettings", map[string]any{
				"cluster":  cluster,
				"settings": tt.settings,
			})
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			c, ok := resp["cluster"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, cluster, c["clusterName"])
		})
	}
}

func TestDescribeClusters_FailureSemantics(t *testing.T) {
	t.Parallel()

	t.Run("unknown returns failure", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doECSRequest(t, h, "DescribeClusters", map[string]any{
			"clusters": []string{"does-not-exist"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		clusters, _ := resp["clusters"].([]any)
		assert.Empty(t, clusters)

		failures, _ := resp["failures"].([]any)
		require.Len(t, failures, 1)

		f := failures[0].(map[string]any)
		assert.Equal(t, "does-not-exist", f["arn"])
		assert.Equal(t, "MISSING", f["reason"])
	})

	t.Run("mix found and missing", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "real-cluster"})

		rec := doECSRequest(t, h, "DescribeClusters", map[string]any{
			"clusters": []string{"real-cluster", "ghost-cluster"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		clusters, _ := resp["clusters"].([]any)
		assert.Len(t, clusters, 1)

		failures, _ := resp["failures"].([]any)
		assert.Len(t, failures, 1)

		f := failures[0].(map[string]any)
		assert.Equal(t, "ghost-cluster", f["arn"])
	})

	t.Run("all missing", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doECSRequest(t, h, "DescribeClusters", map[string]any{
			"clusters": []string{"x", "y", "z"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		clusters, _ := resp["clusters"].([]any)
		assert.Empty(t, clusters)

		failures, _ := resp["failures"].([]any)
		assert.Len(t, failures, 3)
	})

	t.Run("empty describes default cluster, not every cluster", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "c1"})
		doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "c2"})

		rec := doECSRequest(t, h, "DescribeClusters", map[string]any{})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		clusters, _ := resp["clusters"].([]any)
		require.Len(t, clusters, 1)
		c := clusters[0].(map[string]any)
		assert.Equal(t, "default", c["clusterName"])

		failures, _ := resp["failures"].([]any)
		assert.Empty(t, failures)
	})
}

func TestCluster_EmptyArrays_NotNull(t *testing.T) {
	t.Parallel()

	t.Run("create capacityProviders empty not null", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doECSRequest(t, h, "CreateCluster", map[string]any{
			"clusterName": "bare-cluster",
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		cluster, ok := resp["cluster"].(map[string]any)
		require.True(t, ok, "cluster key must be present")

		// AWS always returns capacityProviders as an array, never absent.
		raw, exists := cluster["capacityProviders"]
		assert.True(t, exists, "capacityProviders must be present in cluster response")
		assert.NotNil(t, raw, "capacityProviders must not be null")

		providers, ok := raw.([]any)
		assert.True(t, ok, "capacityProviders must be an array")
		assert.Empty(t, providers, "capacityProviders must be [] when none set")
	})

	t.Run("create defaultCapacityProviderStrategy empty not null", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doECSRequest(t, h, "CreateCluster", map[string]any{
			"clusterName": "bare-cluster2",
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		cluster := resp["cluster"].(map[string]any)

		// AWS always returns defaultCapacityProviderStrategy as an array.
		raw, exists := cluster["defaultCapacityProviderStrategy"]
		assert.True(t, exists, "defaultCapacityProviderStrategy must be present")
		assert.NotNil(t, raw, "defaultCapacityProviderStrategy must not be null")

		strategy, ok := raw.([]any)
		assert.True(t, ok, "defaultCapacityProviderStrategy must be an array")
		assert.Empty(t, strategy, "must be [] when no default strategy set")
	})

	t.Run("describe capacityProviders empty not null", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "dc-cluster"})

		rec := doECSRequest(t, h, "DescribeClusters", map[string]any{
			"clusters": []string{"dc-cluster"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

		clusters, _ := resp["clusters"].([]any)
		require.Len(t, clusters, 1)

		cluster := clusters[0].(map[string]any)

		raw, exists := cluster["capacityProviders"]
		assert.True(t, exists, "capacityProviders present in DescribeClusters response")
		providers, _ := raw.([]any)
		assert.NotNil(t, providers)
		assert.Empty(t, providers)

		rawDcps, exists2 := cluster["defaultCapacityProviderStrategy"]
		assert.True(t, exists2, "defaultCapacityProviderStrategy present in DescribeClusters response")
		dcps, _ := rawDcps.([]any)
		assert.NotNil(t, dcps)
		assert.Empty(t, dcps)
	})
}

// TestECS_UpdateCluster verifies UpdateCluster updates capacity providers and settings.
func TestECS_UpdateCluster(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    map[string]any
		name     string
		wantCode int
	}{
		{
			name: "update capacity providers",
			input: map[string]any{
				"cluster":           "test-cluster",
				"capacityProviders": []string{"FARGATE", "FARGATE_SPOT"},
			},
			wantCode: http.StatusOK,
		},
		{
			name:     "cluster not found",
			input:    map[string]any{"cluster": "missing-cluster"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.wantCode == http.StatusOK {
				doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "test-cluster"})
			}

			rec := doECSRequest(t, h, "UpdateCluster", tt.input)
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			cluster, ok := resp["cluster"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "test-cluster", cluster["clusterName"])
		})
	}
}

func TestBackend_EnrichCluster_CachedCounters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		desiredCount int
		stopCount    int
		wantRunning  float64
		wantPending  float64
	}{
		{
			name:         "running count reflects launched tasks",
			desiredCount: 0,
			stopCount:    0,
			wantRunning:  0,
			wantPending:  0,
		},
		{
			name:         "stop reduces running count",
			desiredCount: 0,
			stopCount:    0,
			wantRunning:  0,
			wantPending:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "perf-cluster"})
			registerTaskDef(t, h, "td", "nginx")

			// Run tasks and verify DescribeClusters reflects the count.
			taskArns := make([]string, 0)
			if tt.desiredCount > 0 {
				runRec := doECSRequest(t, h, "RunTask", map[string]any{
					"cluster":        "perf-cluster",
					"taskDefinition": "td",
					"count":          tt.desiredCount,
				})
				require.Equal(t, http.StatusOK, runRec.Code)

				var runOut map[string]any
				require.NoError(t, json.Unmarshal(runRec.Body.Bytes(), &runOut))
				for _, task := range runOut["tasks"].([]any) {
					taskArns = append(taskArns, task.(map[string]any)["taskArn"].(string))
				}
			}

			// Stop some tasks.
			for i := range tt.stopCount {
				doECSRequest(t, h, "StopTask", map[string]any{
					"cluster": "perf-cluster",
					"task":    taskArns[i],
				})
			}

			descRec := doECSRequest(t, h, "DescribeClusters", map[string]any{
				"clusters": []string{"perf-cluster"},
			})
			require.Equal(t, http.StatusOK, descRec.Code)

			var descOut map[string]any
			require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))
			clusters := descOut["clusters"].([]any)
			require.Len(t, clusters, 1)
			c := clusters[0].(map[string]any)
			assert.InDelta(t, tt.wantRunning, c["runningTasksCount"], 1e-9)
			assert.InDelta(t, tt.wantPending, c["pendingTasksCount"], 1e-9)
		})
	}
}

func TestBackend_EnrichCluster_RunAndStopUpdatesCounters(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "cnt-cluster"})
	registerTaskDef(t, h, "countable", "nginx")

	clusterCounters := func() (float64, float64) {
		rec := doECSRequest(
			t,
			h,
			"DescribeClusters",
			map[string]any{"clusters": []string{"cnt-cluster"}},
		)
		require.Equal(t, http.StatusOK, rec.Code)
		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		c := out["clusters"].([]any)[0].(map[string]any)

		return c["runningTasksCount"].(float64), c["pendingTasksCount"].(float64)
	}

	running, pending := clusterCounters()
	assert.InDelta(t, float64(0), running, 1e-9)
	assert.InDelta(t, float64(0), pending, 1e-9)

	// Run 2 tasks.
	runRec := doECSRequest(t, h, "RunTask", map[string]any{
		"cluster":        "cnt-cluster",
		"taskDefinition": "countable",
		"count":          2,
	})
	require.Equal(t, http.StatusOK, runRec.Code)

	var runOut map[string]any
	require.NoError(t, json.Unmarshal(runRec.Body.Bytes(), &runOut))
	tasks := runOut["tasks"].([]any)
	require.Len(t, tasks, 2)

	running, pending = clusterCounters()
	// With no runtime, tasks go immediately to RUNNING.
	assert.InDelta(t, float64(2), running, 1e-9, "two tasks should be running")
	assert.InDelta(t, float64(0), pending, 1e-9, "no pending tasks with noop runner")

	// Stop one task.
	taskArn := tasks[0].(map[string]any)["taskArn"].(string)
	doECSRequest(t, h, "StopTask", map[string]any{
		"cluster": "cnt-cluster",
		"task":    taskArn,
	})

	running, pending = clusterCounters()
	assert.InDelta(t, float64(1), running, 1e-9, "one task should remain running")
	assert.InDelta(t, float64(0), pending, 1e-9)
}
