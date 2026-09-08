package ecs_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecssdk "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecs"
)

func TestECS_CreateService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*ecs.Handler) map[string]any
		name     string
		wantCode int
	}{
		{
			name: "success with default cluster",
			setup: func(h *ecs.Handler) map[string]any {
				tdArn := registerTestTaskDef(t, h, "svc-task")

				return map[string]any{
					"serviceName":    "my-service",
					"taskDefinition": tdArn,
					"desiredCount":   2,
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name: "success with explicit cluster",
			setup: func(h *ecs.Handler) map[string]any {
				doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "prod"})
				tdArn := registerTestTaskDef(t, h, "svc-task2")

				return map[string]any{
					"serviceName":    "prod-service",
					"cluster":        "prod",
					"taskDefinition": tdArn,
					"desiredCount":   1,
				}
			},
			wantCode: http.StatusOK,
		},
		{
			name: "missing service name",
			setup: func(_ *ecs.Handler) map[string]any {
				return map[string]any{"taskDefinition": "some-task"}
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing task definition",
			setup: func(_ *ecs.Handler) map[string]any {
				return map[string]any{"serviceName": "svc"}
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			input := tt.setup(h)
			rec := doECSRequest(t, h, "CreateService", input)

			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				svc := resp["service"].(map[string]any)
				assert.NotEmpty(t, svc["serviceArn"])
				assert.Equal(t, "ACTIVE", svc["status"])
			}
		})
	}
}

// TestECS_CreateService_AlreadyExists asserts the real code:
// "ServiceAlreadyExistsException" is not a real ECS exception type (absent
// from ecs@v1.90.0/types/errors.go and every per-op deserializeOpError
// switch); CreateService's own deserializer models InvalidParameterException,
// which is what real AWS returns for a duplicate active service name.
func TestECS_CreateService_AlreadyExists(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	tdArn := registerTestTaskDef(t, h, "dup-task")

	input := map[string]any{
		"serviceName":    "dup-svc",
		"taskDefinition": tdArn,
		"desiredCount":   1,
	}

	rec := doECSRequest(t, h, "CreateService", input)
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doECSRequest(t, h, "CreateService", input)
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "InvalidParameterException")
}

func TestECS_DescribeServices(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	tdArn := registerTestTaskDef(t, h, "desc-task")

	rec := doECSRequest(t, h, "CreateService", map[string]any{
		"serviceName":    "desc-svc",
		"taskDefinition": tdArn,
		"desiredCount":   1,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Describe all services.
	rec2 := doECSRequest(t, h, "DescribeServices", map[string]any{"services": []string{}})
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))

	svcs := resp["services"].([]any)
	assert.GreaterOrEqual(t, len(svcs), 1)

	// Describe by name.
	rec3 := doECSRequest(t, h, "DescribeServices", map[string]any{"services": []string{"desc-svc"}})
	require.Equal(t, http.StatusOK, rec3.Code)

	var resp3 map[string]any
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &resp3))

	svcs3 := resp3["services"].([]any)
	require.Len(t, svcs3, 1)
	assert.Equal(t, "desc-svc", svcs3[0].(map[string]any)["serviceName"])
}

func TestECS_UpdateService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*ecs.Handler) (string, map[string]any)
		name     string
		wantCode int
		wantDC   int
	}{
		{
			name: "update desiredCount",
			setup: func(h *ecs.Handler) (string, map[string]any) {
				tdArn := registerTestTaskDef(t, h, "upd-task")
				doECSRequest(t, h, "CreateService", map[string]any{
					"serviceName":    "upd-svc",
					"taskDefinition": tdArn,
					"desiredCount":   1,
				})

				count := 5

				return "upd-svc", map[string]any{
					"service":      "upd-svc",
					"desiredCount": count,
				}
			},
			wantCode: http.StatusOK,
			wantDC:   5,
		},
		{
			name: "service not found",
			setup: func(_ *ecs.Handler) (string, map[string]any) {
				return "", map[string]any{"service": "nonexistent"}
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			_, input := tt.setup(h)
			rec := doECSRequest(t, h, "UpdateService", input)

			require.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				svc := resp["service"].(map[string]any)
				assert.Equal(t, tt.wantDC, int(svc["desiredCount"].(float64)))
			}
		})
	}
}

func TestECS_DeleteService(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	tdArn := registerTestTaskDef(t, h, "del-task")

	rec := doECSRequest(t, h, "CreateService", map[string]any{
		"serviceName":    "del-svc",
		"taskDefinition": tdArn,
		"desiredCount":   1,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doECSRequest(t, h, "DeleteService", map[string]any{"service": "del-svc", "force": true})
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))

	svc := resp["service"].(map[string]any)
	assert.Equal(t, "del-svc", svc["serviceName"])

	// Confirm deletion: AWS returns 200 with failures list, not 404.
	rec3 := doECSRequest(t, h, "DescribeServices", map[string]any{"services": []string{"del-svc"}})
	require.Equal(t, http.StatusOK, rec3.Code)

	var resp3 map[string]any
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &resp3))

	svcs3, _ := resp3["services"].([]any)
	assert.Empty(t, svcs3)

	failures3, _ := resp3["failures"].([]any)
	assert.Len(t, failures3, 1)
}

// TestECS_DeleteService_ActiveGuard verifies AWS's DeleteService guard: a
// service with a non-zero desired count can't be deleted without Force.
func TestECS_DeleteService_ActiveGuard(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "active-guard-td",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	_, err = backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "active-guard-cluster"})
	require.NoError(t, err)

	svc, err := backend.CreateService(ecs.CreateServiceInput{
		Cluster:        "active-guard-cluster",
		ServiceName:    "active-guard-svc",
		TaskDefinition: td.TaskDefinitionArn,
		DesiredCount:   2,
	})
	require.NoError(t, err)

	_, err = backend.DeleteService("active-guard-cluster", "active-guard-svc")
	require.Error(t, err, "DeleteService must fail while desiredCount is non-zero")

	services, failures, err := backend.DescribeServices(
		"active-guard-cluster", []string{"active-guard-svc"},
	)
	require.NoError(t, err)
	assert.Empty(t, failures, "service must survive a failed DeleteService")
	require.Len(t, services, 1)

	// Scale down, then deletion succeeds without Force.
	zero := 0
	_, err = backend.UpdateService(ecs.UpdateServiceInput{
		Cluster:      "active-guard-cluster",
		Service:      svc.ServiceArn,
		DesiredCount: &zero,
	})
	require.NoError(t, err)

	_, err = backend.DeleteService("active-guard-cluster", "active-guard-svc")
	require.NoError(t, err)
}

func TestECS_Backend_CountRunningTasksForService(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "svc-task-count",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	// Create a cluster and service.
	_, err = backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "test-cluster"})
	require.NoError(t, err)

	_, err = backend.CreateService(ecs.CreateServiceInput{
		ServiceName:    "count-svc",
		Cluster:        "test-cluster",
		TaskDefinition: td.TaskDefinitionArn,
		DesiredCount:   2,
	})
	require.NoError(t, err)

	// Run tasks with the service group.
	_, err = backend.RunTask(ecs.RunTaskInput{
		Cluster:        "test-cluster",
		TaskDefinition: td.TaskDefinitionArn,
		Count:          2,
		Group:          "service:count-svc",
	})
	require.NoError(t, err)

	count := backend.CountRunningTasksForService("test-cluster", "count-svc")
	assert.Equal(t, 2, count)
}

func TestECS_Backend_StopOldestServiceTask(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "oldest-task",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	_, err = backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "oldest-cluster"})
	require.NoError(t, err)

	// Start 3 tasks for the service.
	for range 3 {
		err = backend.StartTaskForService("oldest-cluster", "oldest-svc", td.TaskDefinitionArn)
		require.NoError(t, err)
	}

	// Stop the oldest one.
	err = backend.StopOldestServiceTask("oldest-cluster", "oldest-svc")
	require.NoError(t, err)

	count := backend.CountRunningTasksForService("oldest-cluster", "oldest-svc")
	assert.Equal(t, 2, count)
}

func TestECS_Backend_DescribeServices_ClusterNotFound(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	_, _, err := backend.DescribeServices("nonexistent-cluster", nil)
	require.Error(t, err)
}

func TestECS_Backend_UpdateService_ClusterNotFound(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	count := 1
	_, err := backend.UpdateService(ecs.UpdateServiceInput{
		Cluster:      "nonexistent-cluster",
		Service:      "any-service",
		DesiredCount: &count,
	})
	require.Error(t, err)
}

func TestECS_Backend_DeleteService_ClusterNotFound(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	_, err := backend.DeleteService("nonexistent-cluster", "any-service")
	require.Error(t, err)
}

func TestECS_Handler_DescribeServices_ClusterNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doECSRequest(t, h, "DescribeServices", map[string]any{
		"cluster":  "nonexistent-cluster",
		"services": []string{},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestECS_Handler_DeleteService_ClusterNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doECSRequest(t, h, "DeleteService", map[string]any{
		"cluster": "nonexistent-cluster",
		"service": "any-service",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestECS_Backend_ServiceKey_ARN(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "arn-svc-task",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	_, err = backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "arn-svc-cluster"})
	require.NoError(t, err)

	svc, err := backend.CreateService(ecs.CreateServiceInput{
		ServiceName:    "arn-svc",
		Cluster:        "arn-svc-cluster",
		TaskDefinition: td.TaskDefinitionArn,
		DesiredCount:   1,
	})
	require.NoError(t, err)

	// Describe using the full service ARN.
	svcs, _, err := backend.DescribeServices("arn-svc-cluster", []string{svc.ServiceArn})
	require.NoError(t, err)
	require.Len(t, svcs, 1)
	assert.Equal(t, "arn-svc", svcs[0].ServiceName)
}

func TestECS_Backend_UpdateService_TaskDefinition(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	td1, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "td-update-family",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "v1"}},
	})
	require.NoError(t, err)

	td2, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "td-update-family",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "v2"}},
	})
	require.NoError(t, err)

	_, err = backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "td-update-cluster"})
	require.NoError(t, err)

	_, err = backend.CreateService(ecs.CreateServiceInput{
		ServiceName:    "td-update-svc",
		Cluster:        "td-update-cluster",
		TaskDefinition: td1.TaskDefinitionArn,
		DesiredCount:   1,
	})
	require.NoError(t, err)

	updated, err := backend.UpdateService(ecs.UpdateServiceInput{
		Cluster:        "td-update-cluster",
		Service:        "td-update-svc",
		TaskDefinition: td2.TaskDefinitionArn,
	})
	require.NoError(t, err)
	assert.Equal(t, td2.TaskDefinitionArn, updated.TaskDefinition)
}

func TestECS_Backend_EnrichService_PendingTasks(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "enrich-svc-task",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	_, err = backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "enrich-svc-cluster"})
	require.NoError(t, err)

	_, err = backend.CreateService(ecs.CreateServiceInput{
		ServiceName:    "enrich-svc",
		Cluster:        "enrich-svc-cluster",
		TaskDefinition: td.TaskDefinitionArn,
		DesiredCount:   2,
	})
	require.NoError(t, err)

	// Run tasks to populate service running count.
	_, err = backend.RunTask(ecs.RunTaskInput{
		Cluster:        "enrich-svc-cluster",
		TaskDefinition: td.TaskDefinitionArn,
		Count:          2,
		Group:          "service:enrich-svc",
	})
	require.NoError(t, err)

	svcs, _, err := backend.DescribeServices("enrich-svc-cluster", []string{"enrich-svc"})
	require.NoError(t, err)
	require.Len(t, svcs, 1)
	assert.Equal(t, 2, svcs[0].RunningCount)
}

func TestECS_Backend_CreateService_LaunchTypeDefault(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "default-lt-task",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	svc, err := backend.CreateService(ecs.CreateServiceInput{
		ServiceName:    "default-lt-svc",
		TaskDefinition: td.TaskDefinitionArn,
		DesiredCount:   1,
	})
	require.NoError(t, err)
	// Default launch type is FARGATE.
	assert.Equal(t, "FARGATE", svc.LaunchType)
}

func TestECS_Backend_StopOldestServiceTask_NoTasks(t *testing.T) {
	t.Parallel()

	backend := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())

	_, err := backend.CreateCluster(ecs.CreateClusterInput{ClusterName: "empty-cluster"})
	require.NoError(t, err)

	// Should not error when no tasks exist.
	err = backend.StopOldestServiceTask("empty-cluster", "nonexistent-svc")
	require.NoError(t, err)
}

func TestCreateService_Tags_PropagateToResource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "tag-prop-cluster"})
	doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "tag-prop-task",
		"containerDefinitions": []any{map[string]any{"name": "app", "image": "nginx"}},
	})
	svcResp := doECSRequest(t, h, "CreateService", map[string]any{
		"cluster":        "tag-prop-cluster",
		"serviceName":    "tag-prop-svc",
		"taskDefinition": "tag-prop-task",
		"desiredCount":   1,
		"tags": []any{
			map[string]any{"key": "app", "value": "myapp"},
			map[string]any{"key": "env", "value": "staging"},
		},
	})
	require.Equal(t, http.StatusOK, svcResp.Code)
	var svcOut map[string]any
	require.NoError(t, json.Unmarshal(svcResp.Body.Bytes(), &svcOut))
	tags := svcOut["service"].(map[string]any)["tags"].([]any)
	assert.Len(t, tags, 2)
}

func TestListServices_LaunchTypeFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "lt-filter-cluster"})
	doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "lt-filter-task",
		"containerDefinitions": []any{map[string]any{"name": "app", "image": "nginx"}},
	})

	for i := range 3 {
		doECSRequest(t, h, "CreateService", map[string]any{
			"cluster":        "lt-filter-cluster",
			"serviceName":    "lt-filter-svc-" + string(rune('a'+i)),
			"taskDefinition": "lt-filter-task",
			"desiredCount":   1,
			"launchType":     "FARGATE",
		})
	}

	listResp := doECSRequest(t, h, "ListServices", map[string]any{
		"cluster":    "lt-filter-cluster",
		"launchType": "FARGATE",
	})
	require.Equal(t, http.StatusOK, listResp.Code)
	var listOut map[string]any
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listOut))
	arns := listOut["serviceArns"].([]any)
	assert.Len(t, arns, 3)
}

func TestDescribeServices_MultipleServices(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "multi-svc-cluster"})
	doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "multi-svc-task",
		"containerDefinitions": []any{map[string]any{"name": "app", "image": "nginx"}},
	})

	for i := range 4 {
		doECSRequest(t, h, "CreateService", map[string]any{
			"cluster":        "multi-svc-cluster",
			"serviceName":    "multi-svc-" + string(rune('a'+i)),
			"taskDefinition": "multi-svc-task",
			"desiredCount":   1,
		})
	}

	descResp := doECSRequest(t, h, "DescribeServices", map[string]any{
		"cluster":  "multi-svc-cluster",
		"services": []string{"multi-svc-a", "multi-svc-b", "multi-svc-c", "multi-svc-d"},
	})
	require.Equal(t, http.StatusOK, descResp.Code)
	var descOut map[string]any
	require.NoError(t, json.Unmarshal(descResp.Body.Bytes(), &descOut))
	services := descOut["services"].([]any)
	assert.Len(t, services, 4)
}

func TestDeleteService_Force_WithRunningTasks(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "del-force-cluster"})
	doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "del-force-task",
		"containerDefinitions": []any{map[string]any{"name": "app", "image": "nginx"}},
	})
	doECSRequest(t, h, "CreateService", map[string]any{
		"cluster":        "del-force-cluster",
		"serviceName":    "del-force-svc",
		"taskDefinition": "del-force-task",
		"desiredCount":   2,
	})

	deleteResp := doECSRequest(t, h, "DeleteService", map[string]any{
		"cluster": "del-force-cluster",
		"service": "del-force-svc",
		"force":   true,
	})
	require.Equal(t, http.StatusOK, deleteResp.Code)
}

func TestListServicesByNamespace_Filter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doECSRequest(t, h, "CreateCluster", map[string]any{"clusterName": "ns-filter-cluster"})
	doECSRequest(t, h, "RegisterTaskDefinition", map[string]any{
		"family":               "ns-filter-task",
		"containerDefinitions": []any{map[string]any{"name": "app", "image": "nginx"}},
	})

	// Create services with different namespace prefixes
	for _, svcName := range []string{"payments-api", "payments-worker", "inventory-api"} {
		doECSRequest(t, h, "CreateService", map[string]any{
			"cluster":        "ns-filter-cluster",
			"serviceName":    svcName,
			"taskDefinition": "ns-filter-task",
			"desiredCount":   1,
		})
	}

	listResp := doECSRequest(t, h, "ListServicesByNamespace", map[string]any{
		"cluster":   "ns-filter-cluster",
		"namespace": "payments",
	})
	require.Equal(t, http.StatusOK, listResp.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &out))
	arns := out["serviceArns"].([]any)
	assert.Len(t, arns, 2)
}

// TestService_Tags_ResourceTagSync proves Service.Tags is synchronized with the
// shared resourceTags map (TagResource/UntagResource/ListTagsForResource), and
// that DescribeServices only returns tags when Include=[TAGS] is requested.
func TestService_Tags_ResourceTagSync(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T)
		name string
	}{
		{
			name: "tags_supplied_at_create_are_immediately_visible_to_ListTagsForResource",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				tdArn := registerTestTaskDef(t, h, "tag-sync-create-td")

				createRec := doECSRequest(t, h, "CreateService", map[string]any{
					"serviceName":    "tag-sync-create-svc",
					"taskDefinition": tdArn,
					"desiredCount":   1,
					"tags":           []any{map[string]any{"key": "team", "value": "platform"}},
				})
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut map[string]any
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
				serviceArn := createOut["service"].(map[string]any)["serviceArn"].(string)

				listRec := doECSRequest(
					t, h, "ListTagsForResource", map[string]any{"resourceArn": serviceArn},
				)
				require.Equal(t, http.StatusOK, listRec.Code)

				var listOut map[string]any
				require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listOut))
				tags := listOut["tags"].([]any)
				require.Len(t, tags, 1)
				assert.Equal(t, "team", tags[0].(map[string]any)["key"])
			},
		},
		{
			name: "tagresource_after_create_visible_on_describe_only_with_include_tags",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				client := newTestECSClient(t, h)
				tdArn := registerTestTaskDef(t, h, "tag-sync-describe-td")

				createOut, err := client.CreateService(
					t.Context(), &ecssdk.CreateServiceInput{
						ServiceName:    aws.String("tag-sync-describe-svc"),
						TaskDefinition: aws.String(tdArn),
						DesiredCount:   aws.Int32(1),
					},
				)
				require.NoError(t, err)
				serviceArn := aws.ToString(createOut.Service.ServiceArn)

				_, err = client.TagResource(t.Context(), &ecssdk.TagResourceInput{
					ResourceArn: aws.String(serviceArn),
					Tags:        []ecstypes.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
				})
				require.NoError(t, err)

				withoutInclude, err := client.DescribeServices(
					t.Context(), &ecssdk.DescribeServicesInput{Services: []string{serviceArn}},
				)
				require.NoError(t, err)
				require.Len(t, withoutInclude.Services, 1)
				assert.Empty(
					t, withoutInclude.Services[0].Tags,
					"tags must be omitted when Include=[TAGS] is not requested",
				)

				withInclude, err := client.DescribeServices(
					t.Context(), &ecssdk.DescribeServicesInput{
						Services: []string{serviceArn},
						Include:  []ecstypes.ServiceField{ecstypes.ServiceFieldTags},
					},
				)
				require.NoError(t, err)
				require.Len(t, withInclude.Services, 1)
				require.Len(t, withInclude.Services[0].Tags, 1)
				assert.Equal(t, "env", aws.ToString(withInclude.Services[0].Tags[0].Key))
			},
		},
		{
			// PropagateTags=SERVICE task tag propagation only runs through the
			// reconciler's StartTaskForService (RunTask called directly, without
			// going through a service, has no service to propagate tags from --
			// see resolveTaskTags/serviceTagsForPropagate in tasks.go), so this
			// drives StartTaskForService directly against a concrete
			// *InMemoryBackend and reads the result back through the real SDK
			// client to confirm the wire shape.
			name: "propagate_tags_service_inherits_tags_added_after_creation",
			run: func(t *testing.T) {
				t.Helper()

				b := ecs.NewInMemoryBackend(testAccountID, testRegion, ecs.NewNoopRunner())
				h := ecs.NewHandler(b)
				client := newTestECSClient(t, h)
				tdArn := registerTestTaskDef(t, h, "tag-sync-propagate-td")

				createOut, err := client.CreateService(
					t.Context(), &ecssdk.CreateServiceInput{
						ServiceName:    aws.String("tag-sync-propagate-svc"),
						TaskDefinition: aws.String(tdArn),
						DesiredCount:   aws.Int32(1),
						PropagateTags:  ecstypes.PropagateTagsService,
					},
				)
				require.NoError(t, err)
				serviceArn := aws.ToString(createOut.Service.ServiceArn)

				// Tag the service AFTER creation -- StartTaskForService must pick
				// this up from the resourceTags side map, not a stale
				// creation-time svc.Tags snapshot (see StartTaskForService in
				// services.go).
				_, err = client.TagResource(t.Context(), &ecssdk.TagResourceInput{
					ResourceArn: aws.String(serviceArn),
					Tags:        []ecstypes.Tag{{Key: aws.String("cost-center"), Value: aws.String("42")}},
				})
				require.NoError(t, err)

				require.NoError(
					t, b.StartTaskForService("default", "tag-sync-propagate-svc", tdArn),
				)

				listOut, err := client.ListTasks(t.Context(), &ecssdk.ListTasksInput{
					ServiceName: aws.String("tag-sync-propagate-svc"),
				})
				require.NoError(t, err)
				require.Len(t, listOut.TaskArns, 1)

				descOut, err := client.DescribeTasks(t.Context(), &ecssdk.DescribeTasksInput{
					Tasks:   listOut.TaskArns,
					Include: []ecstypes.TaskField{ecstypes.TaskFieldTags},
				})
				require.NoError(t, err)
				require.Len(t, descOut.Tasks, 1)
				require.Len(t, descOut.Tasks[0].Tags, 1)
				assert.Equal(t, "cost-center", aws.ToString(descOut.Tasks[0].Tags[0].Key))
			},
		},
		{
			name: "delete_echoes_final_tags_and_clears_resourceTags",
			run: func(t *testing.T) {
				t.Helper()

				h := newTestHandler(t)
				tdArn := registerTestTaskDef(t, h, "tag-sync-delete-td")

				createRec := doECSRequest(t, h, "CreateService", map[string]any{
					"serviceName":    "tag-sync-delete-svc",
					"taskDefinition": tdArn,
					"desiredCount":   1,
					"tags":           []any{map[string]any{"key": "team", "value": "platform"}},
				})
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut map[string]any
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
				serviceArn := createOut["service"].(map[string]any)["serviceArn"].(string)

				deleteRec := doECSRequest(t, h, "DeleteService", map[string]any{
					"service": "tag-sync-delete-svc",
					"force":   true,
				})
				require.Equal(t, http.StatusOK, deleteRec.Code)

				var deleteOut map[string]any
				require.NoError(t, json.Unmarshal(deleteRec.Body.Bytes(), &deleteOut))
				deletedTags := deleteOut["service"].(map[string]any)["tags"].([]any)
				require.Len(t, deletedTags, 1)
				assert.Equal(t, "team", deletedTags[0].(map[string]any)["key"])

				listRec := doECSRequest(
					t, h, "ListTagsForResource", map[string]any{"resourceArn": serviceArn},
				)
				require.Equal(t, http.StatusOK, listRec.Code)

				var listOut map[string]any
				require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listOut))
				assert.Empty(t, listOut["tags"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}
