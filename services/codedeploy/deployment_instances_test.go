package codedeploy_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codedeploy"
)

// serverDeployWithMatchedInstances creates a Server-platform app + deployment
// group whose OnPremisesInstanceTagFilters match two registered on-premises
// instances (and a third, deliberately non-matching instance), then creates a
// deployment against it. Returns the deployment ID and the two instance names
// that should resolve as real targets.
func serverDeployWithMatchedInstances(t *testing.T, h *codedeploy.Handler) (string, []string) {
	t.Helper()

	b := h.Backend
	_, err := b.CreateApplication("my-app", "Server", nil)
	require.NoError(t, err)

	_, err = b.CreateDeploymentGroup("my-app", "my-dg", codedeploy.DeploymentGroupInput{
		ServiceRoleArn: "arn:aws:iam::000000000000:role/role",
		OnPremisesInstanceTagFilters: []codedeploy.TagFilter{
			{Key: "env", Value: "prod", Type: "EQUALS"},
		},
	}, nil)
	require.NoError(t, err)

	err = b.RegisterOnPremisesInstance("i-match-1", "", "arn:aws:iam::000000000000:user/u1")
	require.NoError(t, err)
	require.NoError(t, b.AddTagsToOnPremisesInstances([]string{"i-match-1"}, map[string]string{"env": "prod"}))

	err = b.RegisterOnPremisesInstance("i-match-2", "", "arn:aws:iam::000000000000:user/u2")
	require.NoError(t, err)
	require.NoError(t, b.AddTagsToOnPremisesInstances([]string{"i-match-2"}, map[string]string{"env": "prod"}))

	err = b.RegisterOnPremisesInstance("i-nomatch", "", "arn:aws:iam::000000000000:user/u3")
	require.NoError(t, err)
	require.NoError(t, b.AddTagsToOnPremisesInstances([]string{"i-nomatch"}, map[string]string{"env": "dev"}))

	d, err := b.CreateDeployment("my-app", "my-dg", codedeploy.DeploymentOptions{Creator: "user"})
	require.NoError(t, err)

	return d.DeploymentID, []string{"i-match-1", "i-match-2"}
}

func TestDeploymentInstances_BatchGetMissingDeployment(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "BatchGetDeploymentInstances", map[string]any{
		"deploymentId": "d-NOTEXIST1",
		"instanceIds":  []string{"i-abc"},
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "DeploymentDoesNotExistException", resp["__type"])
}

// TestHandler_BatchGetDeploymentInstances_RealTargets proves
// BatchGetDeploymentInstances resolves real on-premises instances matched by
// the deployment group's tag filters -- not a fabricated summary for every
// requested ID -- by requesting the two matched instances plus one that
// exists but doesn't match the filter and one that was never registered.
func TestHandler_BatchGetDeploymentInstances_RealTargets(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	deployID, matched := serverDeployWithMatchedInstances(t, h)

	rec := doRequest(t, h, "BatchGetDeploymentInstances", map[string]any{
		"deploymentId": deployID,
		"instanceIds":  []string{matched[0], matched[1], "i-nomatch", "i-never-registered"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	summaries, ok := resp["instancesSummary"].([]any)
	require.True(t, ok)
	// Only the two tag-filter-matched instances resolve as real targets;
	// the non-matching and never-registered IDs are silently omitted.
	require.Len(t, summaries, 2)

	for _, s := range summaries {
		m, isMap := s.(map[string]any)
		require.True(t, isMap)
		assert.Equal(t, deployID, m["deploymentId"])
		assert.Equal(t, "Succeeded", m["status"])
		assert.Contains(t, matched, m["instanceId"])
	}
}

func TestHandler_BatchGetDeploymentInstances_MissingDeploymentID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "BatchGetDeploymentInstances", map[string]any{
		"instanceIds": []string{"i-abc"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_BatchGetDeploymentTargets_ECS proves ECS-platform deployments
// resolve one ecsTarget per configured ECS service (real, already-tracked
// deployment group data), rather than fabricating a target for whatever
// targetId the caller supplies.
func TestHandler_BatchGetDeploymentTargets_ECS(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := h.Backend

	_, err := b.CreateApplication("ecs-app", "ECS", nil)
	require.NoError(t, err)

	_, err = b.CreateDeploymentGroup("ecs-app", "ecs-dg", codedeploy.DeploymentGroupInput{
		ServiceRoleArn: "arn:aws:iam::000000000000:role/role",
		ECSServices: []codedeploy.ECSService{
			{ClusterName: "cluster-1", ServiceName: "service-1"},
		},
	}, nil)
	require.NoError(t, err)

	d, err := b.CreateDeployment("ecs-app", "ecs-dg", codedeploy.DeploymentOptions{Creator: "user"})
	require.NoError(t, err)

	rec := doRequest(t, h, "BatchGetDeploymentTargets", map[string]any{
		"deploymentId": d.DeploymentID,
		"targetIds":    []string{"cluster-1/service-1", "cluster-x/service-x"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	targets, ok := resp["deploymentTargets"].([]any)
	require.True(t, ok)
	require.Len(t, targets, 1)

	target, ok := targets[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ECSTarget", target["deploymentTargetType"])
	ecsTarget, ok := target["ecsTarget"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "cluster-1/service-1", ecsTarget["targetId"])
	assert.Equal(t, "Succeeded", ecsTarget["status"])
}

func TestHandler_BatchGetDeploymentTargets_MissingDeploymentID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "BatchGetDeploymentTargets", map[string]any{
		"targetIds": []string{"t-1"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_BatchGetDeploymentTargets_DeploymentNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "BatchGetDeploymentTargets", map[string]any{
		"deploymentId": "d-nonexistent",
		"targetIds":    []string{"t-1"},
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestHandler_GetDeploymentTarget_Lambda proves Lambda-platform deployments
// resolve exactly one lambdaTarget keyed by the deployment ID itself, and
// that an unknown target ID for that deployment 404s as
// DeploymentTargetDoesNotExistException instead of fabricating a match.
func TestHandler_GetDeploymentTarget_Lambda(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := h.Backend

	_, err := b.CreateApplication("lambda-app", "Lambda", nil)
	require.NoError(t, err)

	_, err = b.CreateDeploymentGroup("lambda-app", "lambda-dg", codedeploy.DeploymentGroupInput{
		ServiceRoleArn: "arn:aws:iam::000000000000:role/role",
	}, nil)
	require.NoError(t, err)

	d, err := b.CreateDeployment("lambda-app", "lambda-dg", codedeploy.DeploymentOptions{Creator: "user"})
	require.NoError(t, err)

	rec := doRequest(t, h, "GetDeploymentTarget", map[string]any{
		"deploymentId": d.DeploymentID,
		"targetId":     d.DeploymentID,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	target, ok := resp["deploymentTarget"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "LambdaTarget", target["deploymentTargetType"])
	lambdaTarget, ok := target["lambdaTarget"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, d.DeploymentID, lambdaTarget["targetId"])

	rec = doRequest(t, h, "GetDeploymentTarget", map[string]any{
		"deploymentId": d.DeploymentID,
		"targetId":     "not-a-real-target",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "DeploymentTargetDoesNotExistException", errResp["__type"])
}

func TestHandler_GetDeploymentTarget_MissingParams(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "GetDeploymentTarget", map[string]any{"deploymentId": "d-x"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_GetDeploymentInstance_RealMatch proves GetDeploymentInstance
// resolves a real matched on-premises instance and 404s for an instance that
// exists but isn't a target of this deployment.
func TestHandler_GetDeploymentInstance_RealMatch(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	deployID, matched := serverDeployWithMatchedInstances(t, h)

	rec := doRequest(t, h, "GetDeploymentInstance", map[string]any{
		"deploymentId": deployID,
		"instanceId":   matched[0],
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	summary, ok := resp["instanceSummary"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, matched[0], summary["instanceId"])
	assert.Equal(t, "Succeeded", summary["status"])
	assert.Equal(t, "BLUE", summary["instanceType"])

	rec = doRequest(t, h, "GetDeploymentInstance", map[string]any{
		"deploymentId": deployID,
		"instanceId":   "i-nomatch",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestHandler_ListDeploymentTargets_ListDeploymentInstances_RealTargets
// proves both list operations return the real matched instance set instead
// of an unconditional empty list.
func TestHandler_ListDeploymentTargets_ListDeploymentInstances_RealTargets(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	deployID, matched := serverDeployWithMatchedInstances(t, h)

	rec := doRequest(t, h, "ListDeploymentTargets", map[string]any{"deploymentId": deployID})
	require.Equal(t, http.StatusOK, rec.Code)

	var targetsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &targetsResp))
	targetIDs, ok := targetsResp["targetIds"].([]any)
	require.True(t, ok)
	require.Len(t, targetIDs, 2)
	assert.ElementsMatch(t, []any{matched[0], matched[1]}, targetIDs)

	rec = doRequest(t, h, "ListDeploymentInstances", map[string]any{"deploymentId": deployID})
	require.Equal(t, http.StatusOK, rec.Code)

	var instResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &instResp))
	instances, ok := instResp["instancesList"].([]any)
	require.True(t, ok)
	assert.ElementsMatch(t, []any{matched[0], matched[1]}, instances)
}

// TestHandler_ListDeploymentInstances_StatusAndTypeFilter proves
// instanceStatusFilter/instanceTypeFilter are actually applied instead of
// being silently ignored (ListDeploymentInstancesInput models both --
// aws-sdk-go-v2/service/codedeploy/api_op_ListDeploymentInstances.go).
func TestHandler_ListDeploymentInstances_StatusAndTypeFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	deployID, _ := serverDeployWithMatchedInstances(t, h)

	tests := []struct {
		body    map[string]any
		name    string
		wantLen int
	}{
		{name: "no_filter", body: map[string]any{"deploymentId": deployID}, wantLen: 2},
		{
			name:    "matching_status",
			body:    map[string]any{"deploymentId": deployID, "instanceStatusFilter": []string{"Succeeded"}},
			wantLen: 2,
		},
		{
			name:    "non_matching_status",
			body:    map[string]any{"deploymentId": deployID, "instanceStatusFilter": []string{"Failed"}},
			wantLen: 0,
		},
		{
			name:    "matching_type",
			body:    map[string]any{"deploymentId": deployID, "instanceTypeFilter": []string{"Blue"}},
			wantLen: 2,
		},
		{
			name:    "non_matching_type",
			body:    map[string]any{"deploymentId": deployID, "instanceTypeFilter": []string{"Green"}},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, "ListDeploymentInstances", tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			instances, ok := resp["instancesList"].([]any)
			require.True(t, ok)
			assert.Len(t, instances, tt.wantLen)
		})
	}
}

// TestHandler_ListDeploymentTargets_Filters proves targetFilters
// (TargetStatus/ServerInstanceLabel) are actually applied instead of being
// silently ignored (ListDeploymentTargetsInput models targetFilters --
// aws-sdk-go-v2/service/codedeploy/api_op_ListDeploymentTargets.go).
func TestHandler_ListDeploymentTargets_Filters(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	deployID, _ := serverDeployWithMatchedInstances(t, h)

	tests := []struct {
		targetFilters map[string][]string
		name          string
		wantLen       int
	}{
		{name: "no_filter", wantLen: 2},
		{name: "matching_status", targetFilters: map[string][]string{"TargetStatus": {"Succeeded"}}, wantLen: 2},
		{name: "non_matching_status", targetFilters: map[string][]string{"TargetStatus": {"Failed"}}, wantLen: 0},
		{
			name:          "matching_label",
			targetFilters: map[string][]string{"ServerInstanceLabel": {"Blue"}},
			wantLen:       2,
		},
		{
			name:          "non_matching_label",
			targetFilters: map[string][]string{"ServerInstanceLabel": {"Green"}},
			wantLen:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := map[string]any{"deploymentId": deployID}
			if tt.targetFilters != nil {
				body["targetFilters"] = tt.targetFilters
			}

			rec := doRequest(t, h, "ListDeploymentTargets", body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			targetIDs, ok := resp["targetIds"].([]any)
			require.True(t, ok)
			assert.Len(t, targetIDs, tt.wantLen)
		})
	}
}

// TestHandler_ListDeploymentInstances_ECSIsEmpty proves ListDeploymentInstances
// resolves to an empty list for an ECS-platform deployment (it has no
// per-instance concept), rather than fabricating instance IDs.
func TestHandler_ListDeploymentInstances_ECSIsEmpty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := h.Backend

	_, err := b.CreateApplication("ecs-app2", "ECS", nil)
	require.NoError(t, err)

	_, err = b.CreateDeploymentGroup("ecs-app2", "ecs-dg2", codedeploy.DeploymentGroupInput{
		ServiceRoleArn: "arn:aws:iam::000000000000:role/role",
		ECSServices: []codedeploy.ECSService{
			{ClusterName: "cluster-1", ServiceName: "service-1"},
		},
	}, nil)
	require.NoError(t, err)

	d, err := b.CreateDeployment("ecs-app2", "ecs-dg2", codedeploy.DeploymentOptions{Creator: "user"})
	require.NoError(t, err)

	rec := doRequest(t, h, "ListDeploymentInstances", map[string]any{"deploymentId": d.DeploymentID})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	instances, ok := resp["instancesList"].([]any)
	require.True(t, ok)
	assert.Empty(t, instances)
}

// TestDeploymentTargets_StatusTracksDeploymentStatus proves target status is
// derived from the deployment's own current Status (via
// targetStatusForDeployment) instead of being hardcoded to "Succeeded"
// regardless of what actually happened to the deployment.
func TestDeploymentTargets_StatusTracksDeploymentStatus(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	deployID, matched := serverDeployWithMatchedInstances(t, h)

	require.NoError(t, h.Backend.StopDeployment(deployID))

	rec := doRequest(t, h, "GetDeploymentTarget", map[string]any{
		"deploymentId": deployID,
		"targetId":     matched[0],
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	target, ok := resp["deploymentTarget"].(map[string]any)
	require.True(t, ok)
	instanceTarget, ok := target["instanceTarget"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Skipped", instanceTarget["status"])
}
