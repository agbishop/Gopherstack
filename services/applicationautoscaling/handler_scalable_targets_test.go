package applicationautoscaling_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_RegisterScalableTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantKey  string
		wantCode int
	}{
		{
			name: "create",
			body: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/my-svc",
				"ScalableDimension": "ecs:service:DesiredCount",
				"MinCapacity":       int32(1),
				"MaxCapacity":       int32(10),
			},
			wantCode: http.StatusOK,
			wantKey:  "ScalableTargetARN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "RegisterScalableTarget", tt.body)
			require.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Contains(t, resp, tt.wantKey)
			assert.Contains(t, resp[tt.wantKey].(string), "arn:aws:application-autoscaling:")
		})
	}
}

func TestHandler_RegisterScalableTarget_Upsert(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	body := map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(10),
	}

	// Create
	rec1 := doRequest(t, h, "RegisterScalableTarget", body)
	require.Equal(t, http.StatusOK, rec1.Code)
	var resp1 map[string]string
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &resp1))

	// Update (upsert) - should update, not error
	body["MaxCapacity"] = int32(20)
	rec2 := doRequest(t, h, "RegisterScalableTarget", body)
	require.Equal(t, http.StatusOK, rec2.Code)

	// Verify the updated capacity
	descRec := doRequest(t, h, "DescribeScalableTargets", map[string]any{"ServiceNamespace": "ecs"})
	require.Equal(t, http.StatusOK, descRec.Code)
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
	targets := descResp["ScalableTargets"].([]any)
	require.Len(t, targets, 1)
	target := targets[0].(map[string]any)
	assert.InDelta(t, float64(20), target["MaxCapacity"], 0)
}

func TestHandler_RegisterScalableTarget_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "missing_namespace",
			body: map[string]any{
				"ResourceId":        "service/default/my-svc",
				"ScalableDimension": "ecs:service:DesiredCount",
				"MinCapacity":       int32(1),
				"MaxCapacity":       int32(10),
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_resource_id",
			body: map[string]any{
				"ServiceNamespace":  "ecs",
				"ScalableDimension": "ecs:service:DesiredCount",
				"MinCapacity":       int32(1),
				"MaxCapacity":       int32(10),
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_scalable_dimension",
			body: map[string]any{
				"ServiceNamespace": "ecs",
				"ResourceId":       "service/default/my-svc",
				"MinCapacity":      int32(1),
				"MaxCapacity":      int32(10),
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "min_exceeds_max",
			body: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/my-svc",
				"ScalableDimension": "ecs:service:DesiredCount",
				"MinCapacity":       int32(20),
				"MaxCapacity":       int32(5),
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "equal_min_max",
			body: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/my-svc",
				"ScalableDimension": "ecs:service:DesiredCount",
				"MinCapacity":       int32(5),
				"MaxCapacity":       int32(5),
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "RegisterScalableTarget", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_RegisterScalableTarget_WithTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/tagged-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(5),
		"Tags":              map[string]string{"env": "prod", "team": "infra"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var regResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &regResp))
	targetARN, _ := regResp["ScalableTargetARN"].(string)
	require.NotEmpty(t, targetARN)

	// Tags should be visible via ListTagsForResource
	tagRec := doRequest(t, h, "ListTagsForResource", map[string]any{"ResourceARN": targetARN})
	require.Equal(t, http.StatusOK, tagRec.Code)

	var tagResp map[string]any
	require.NoError(t, json.Unmarshal(tagRec.Body.Bytes(), &tagResp))
	tags, ok := tagResp["Tags"].(map[string]any)
	require.True(t, ok, "expected Tags in response")
	assert.Equal(t, "prod", tags["env"])
	assert.Equal(t, "infra", tags["team"])
}

func TestHandler_RegisterScalableTarget_WithRoleARN(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	roleARN := "arn:aws:iam::123456789012:role/ApplicationAutoScalingRole"
	rec := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(5),
		"RoleARN":           roleARN,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// RoleARN should be visible in DescribeScalableTargets
	descRec := doRequest(t, h, "DescribeScalableTargets", map[string]any{"ServiceNamespace": "ecs"})
	require.Equal(t, http.StatusOK, descRec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
	targets, ok := descResp["ScalableTargets"].([]any)
	require.True(t, ok)
	require.Len(t, targets, 1)
	target := targets[0].(map[string]any)
	assert.Equal(t, roleARN, target["RoleARN"])
}

func TestHandler_RegisterScalableTarget_WithSuspendedState(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(5),
		"SuspendedState": map[string]any{
			"DynamicScalingInSuspended":  true,
			"DynamicScalingOutSuspended": false,
			"ScheduledScalingSuspended":  true,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	descRec := doRequest(t, h, "DescribeScalableTargets", map[string]any{
		"ServiceNamespace": "ecs",
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
	targets, ok := resp["ScalableTargets"].([]any)
	require.True(t, ok)
	require.Len(t, targets, 1)

	ss, ok := targets[0].(map[string]any)["SuspendedState"].(map[string]any)
	require.True(t, ok, "expected SuspendedState in response")
	assert.Equal(t, true, ss["DynamicScalingInSuspended"])
	assert.Equal(t, false, ss["DynamicScalingOutSuspended"])
	assert.Equal(t, true, ss["ScheduledScalingSuspended"])
}

func TestHandler_RegisterScalableTarget_UpdateSuspendedState(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create target without SuspendedState
	doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(5),
	})

	// Update with SuspendedState
	doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(2),
		"MaxCapacity":       int32(10),
		"SuspendedState": map[string]any{
			"DynamicScalingInSuspended": true,
		},
	})

	rec := doRequest(t, h, "DescribeScalableTargets", map[string]any{
		"ServiceNamespace": "ecs",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	targets, _ := resp["ScalableTargets"].([]any)
	require.Len(t, targets, 1)

	ss, ok := targets[0].(map[string]any)["SuspendedState"].(map[string]any)
	require.True(t, ok, "expected SuspendedState after update")
	assert.Equal(t, true, ss["DynamicScalingInSuspended"])
	// Also verify capacity was updated
	assert.InDelta(t, float64(2), targets[0].(map[string]any)["MinCapacity"], 0.001)
}

func TestHandler_RegisterScalableTarget_TagLimitEnforced(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Build exactly 51 tags (one over the limit)
	tags := make(map[string]string, 51)
	for i := range 51 {
		tags[fmt.Sprintf("key-%d", i)] = "value"
	}

	rec := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(5),
		"Tags":              tags,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "expected 400 when tag count exceeds 50")
}

func TestHandler_RegisterScalableTarget_UpdateTagsMergeLimit(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Register with 45 tags
	initial := make(map[string]string, 45)
	for i := range 45 {
		initial[fmt.Sprintf("k%d", i)] = "v"
	}

	doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(5),
		"Tags":              initial,
	})

	// Upsert with 10 new tags - total would be 55, exceeds limit
	extra := make(map[string]string, 10)
	for i := range 10 {
		extra[fmt.Sprintf("new-%d", i)] = "v"
	}

	rec := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(5),
		"Tags":              extra,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "expected 400 when tag limit exceeded on upsert")
}

// TestHandler_RegisterScalableTarget_NamespaceQuotaEnforced verifies the
// real, documented per-account/per-region "scalable targets per resource
// type" AWS quota (source: "Quotas for Application Auto Scaling"):
// 5,000 for dynamodb, 3,000 for ecs, 1,500 for cassandra (Keyspaces), and
// 500 for every other ServiceNamespace. Uses the smallest (default) bucket
// to keep the test bounded. Upserting an already-registered target must not
// consume additional quota.
func TestHandler_RegisterScalableTarget_NamespaceQuotaEnforced(t *testing.T) {
	t.Parallel()

	const (
		namespace     = "custom-resource"
		defaultQuota  = 500
		dimension     = "custom-resource:ResourceType:Property"
		roleARNSuffix = "-role"
	)

	h := newTestHandler(t)

	for i := range defaultQuota {
		rec := doRequest(t, h, "RegisterScalableTarget", map[string]any{
			"ServiceNamespace":  namespace,
			"ResourceId":        fmt.Sprintf("resource-%d", i),
			"ScalableDimension": dimension,
			"MinCapacity":       int32(1),
			"MaxCapacity":       int32(5),
		})
		require.Equal(t, http.StatusOK, rec.Code, "target %d should register within quota", i)
	}

	// The 501st distinct target for this namespace exceeds the default quota.
	overLimit := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  namespace,
		"ResourceId":        "resource-over-limit",
		"ScalableDimension": dimension,
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(5),
	})
	assert.Equal(t, http.StatusBadRequest, overLimit.Code,
		"expected 400 LimitExceededException once the namespace's scalable-target quota is exhausted")

	// Upserting an already-registered target must not be blocked by the
	// quota (it doesn't grow the namespace's target count).
	upsert := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  namespace,
		"ResourceId":        "resource-0",
		"ScalableDimension": dimension,
		"MinCapacity":       int32(2),
		"MaxCapacity":       int32(6),
		"RoleARN":           namespace + roleARNSuffix,
	})
	assert.Equal(t, http.StatusOK, upsert.Code, "upsert of an existing target must not be blocked by the quota")

	// A different, unrelated namespace has its own independent quota bucket.
	otherNS := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/other-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(5),
	})
	assert.Equal(t, http.StatusOK, otherNS.Code, "a different ServiceNamespace has its own quota bucket")
}

func TestRegisterDeregisterLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	const (
		ns  = "ecs"
		res = "service/default/web"
		dim = "ecs:service:DesiredCount"
	)

	arn := seedTarget(t, h, res, 1, 10)
	assert.NotEmpty(t, arn)

	descRec := doRequest(t, h, "DescribeScalableTargets", map[string]any{
		"ServiceNamespace": ns,
	})
	require.Equal(t, http.StatusOK, descRec.Code)
	var descOut map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))
	targets := descOut["ScalableTargets"].([]any)
	require.Len(t, targets, 1)
	t0 := targets[0].(map[string]any)
	assert.Equal(t, res, t0["ResourceId"])
	assert.Equal(t, arn, t0["ScalableTargetARN"])
	assert.NotNil(t, t0["CreationTime"], "CreationTime must be present")

	deregRec := doRequest(t, h, "DeregisterScalableTarget", map[string]any{
		"ServiceNamespace":  ns,
		"ResourceId":        res,
		"ScalableDimension": dim,
	})
	assert.Equal(t, http.StatusOK, deregRec.Code)

	afterRec := doRequest(t, h, "DescribeScalableTargets", map[string]any{
		"ServiceNamespace": ns,
	})
	var afterOut map[string]any
	require.NoError(t, json.Unmarshal(afterRec.Body.Bytes(), &afterOut))
	assert.Empty(t, afterOut["ScalableTargets"])
}

// TestHandler_RegisterScalableTarget_UpdateOmittedCapacityPreserved verifies
// that updating an existing scalable target without resending
// MinCapacity/MaxCapacity leaves the stored capacity unchanged, rather than
// resetting it to zero. Real AWS's RegisterScalableTargetInput models both as
// optional (*int32, "required when registering a new scalable target" --
// api_op_RegisterScalableTarget.go), and the operation doc states: "To update
// a scalable target, specify the parameters that you want to change ... Any
// parameters that you don't specify are not changed by this update request".
func TestHandler_RegisterScalableTarget_UpdateOmittedCapacityPreserved(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(2),
		"MaxCapacity":       int32(10),
	})

	// Update RoleARN only; MinCapacity/MaxCapacity are omitted entirely.
	roleARN := "arn:aws:iam::123456789012:role/ApplicationAutoScalingRole"
	rec := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"RoleARN":           roleARN,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	descRec := doRequest(t, h, "DescribeScalableTargets", map[string]any{"ServiceNamespace": "ecs"})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
	targets, _ := resp["ScalableTargets"].([]any)
	require.Len(t, targets, 1)
	target := targets[0].(map[string]any)

	assert.InDelta(t, float64(2), target["MinCapacity"], 0, "MinCapacity must be preserved when omitted on update")
	assert.InDelta(t, float64(10), target["MaxCapacity"], 0, "MaxCapacity must be preserved when omitted on update")
	assert.Equal(t, roleARN, target["RoleARN"])
}

// TestHandler_RegisterScalableTarget_MissingCapacityOnCreate verifies
// MinCapacity/MaxCapacity are still required when registering a brand-new
// scalable target (api_op_RegisterScalableTarget.go: "This property is
// required when registering a new scalable target").
func TestHandler_RegisterScalableTarget_MissingCapacityOnCreate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/new-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MaxCapacity":       int32(10),
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "expected 400 when MinCapacity is missing on create")
}
