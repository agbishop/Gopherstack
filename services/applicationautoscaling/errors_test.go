package applicationautoscaling_test

// Tests for the AWS exception types/HTTP-status mapping introduced/fixed in
// errors.go and handler.go's handleError: ObjectNotFoundException is a
// client fault (HTTP 400, not 404) on this awsjson1.1 service,
// PutScalingPolicy/PutScheduledAction require a pre-registered scalable
// target, LimitExceededException covers the real, documented Application
// Auto Scaling quotas, and Describe* ops reject a malformed NextToken with
// InvalidNextTokenException.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/applicationautoscaling"
)

// TestHandler_PutScalingPolicy_RequiresRegisteredTarget verifies real AWS's
// documented ObjectNotFoundException behavior for PutScalingPolicy ("For any
// operation that depends on the existence of a scalable target, this
// exception is thrown if the scalable target ... does not exist"),
// confirmed against PutScalingPolicy's modeled error set in the vendored SDK.
func TestHandler_PutScalingPolicy_RequiresRegisteredTarget(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "PutScalingPolicy", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/unregistered",
		"ScalableDimension": "ecs:service:DesiredCount",
		"PolicyName":        "my-policy",
		"PolicyType":        "TargetTrackingScaling",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	assert.Equal(t, "ObjectNotFoundException", errBody["__type"])
}

// TestHandler_PutScheduledAction_RequiresRegisteredTarget mirrors
// TestHandler_PutScalingPolicy_RequiresRegisteredTarget for PutScheduledAction,
// which is also modeled with ObjectNotFoundException.
func TestHandler_PutScheduledAction_RequiresRegisteredTarget(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "PutScheduledAction", map[string]any{
		"ServiceNamespace":    "ecs",
		"ResourceId":          "service/default/unregistered",
		"ScalableDimension":   "ecs:service:DesiredCount",
		"ScheduledActionName": "my-action",
		"Schedule":            "rate(1 hour)",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	assert.Equal(t, "ObjectNotFoundException", errBody["__type"])
}

// TestHandler_ObjectNotFoundException_Is400Not404 pins the real AWS status
// code for ObjectNotFoundException on this json-protocol service: every
// modeled exception's HTTP status follows its ErrorFault() classification
// (client fault -> 400), not resource-not-found-implies-404 REST convention.
func TestHandler_ObjectNotFoundException_Is400Not404(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "DeregisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/nonexistent",
		"ScalableDimension": "ecs:service:DesiredCount",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NotEqual(t, http.StatusNotFound, rec.Code)

	var errBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	assert.Equal(t, "ObjectNotFoundException", errBody["__type"])
}

// TestBackend_PutScalingPolicy_LimitExceeded_PoliciesPerTarget verifies the
// real, non-adjustable AWS quota of 50 scaling policies per scalable target
// (see "Quotas for Application Auto Scaling"). Updates to an existing policy
// must not count against the quota.
func TestBackend_PutScalingPolicy_LimitExceeded_PoliciesPerTarget(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.RegisterScalableTarget(
		"ecs", "service/default/svc", "ecs:service:DesiredCount", int32p(1), int32p(10), nil, "", nil,
	)
	require.NoError(t, err)

	for i := range 50 {
		_, err = b.PutScalingPolicy(
			"ecs", "service/default/svc", "ecs:service:DesiredCount",
			fmt.Sprintf("policy-%d", i), "StepScaling", nil, nil, nil,
		)
		require.NoError(t, err)
	}

	// The 51st distinct policy must be rejected.
	_, err = b.PutScalingPolicy(
		"ecs", "service/default/svc", "ecs:service:DesiredCount",
		"policy-50", "StepScaling", nil, nil, nil,
	)
	require.ErrorIs(t, err, applicationautoscaling.ErrLimitExceeded)

	// Updating one of the existing 50 must still succeed (updates don't
	// count against the quota).
	_, err = b.PutScalingPolicy(
		"ecs", "service/default/svc", "ecs:service:DesiredCount",
		"policy-0", "TargetTrackingScaling", map[string]any{"TargetValue": 50.0}, nil, nil,
	)
	require.NoError(t, err)
}

// TestBackend_PutScalingPolicy_LimitExceeded_StepAdjustments verifies the
// real AWS quota of 20 step adjustments per step scaling policy.
func TestBackend_PutScalingPolicy_LimitExceeded_StepAdjustments(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.RegisterScalableTarget(
		"ecs", "service/default/svc", "ecs:service:DesiredCount", int32p(1), int32p(10), nil, "", nil,
	)
	require.NoError(t, err)

	steps := make([]any, 21)
	for i := range steps {
		steps[i] = map[string]any{"ScalingAdjustment": 1}
	}

	_, err = b.PutScalingPolicy(
		"ecs", "service/default/svc", "ecs:service:DesiredCount",
		"too-many-steps", "StepScaling",
		nil, map[string]any{"StepAdjustments": steps}, nil,
	)
	require.ErrorIs(t, err, applicationautoscaling.ErrLimitExceeded)
}

// TestBackend_PutScheduledAction_LimitExceeded_ActionsPerTarget verifies the
// real, non-adjustable AWS quota of 200 scheduled actions per scalable
// target.
func TestBackend_PutScheduledAction_LimitExceeded_ActionsPerTarget(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.RegisterScalableTarget(
		"ecs", "service/default/svc", "ecs:service:DesiredCount", int32p(1), int32p(10), nil, "", nil,
	)
	require.NoError(t, err)

	for i := range 200 {
		_, err = b.PutScheduledAction(
			"ecs", "service/default/svc", "ecs:service:DesiredCount",
			fmt.Sprintf("action-%d", i), "rate(1 hour)", "", nil, nil, nil,
		)
		require.NoError(t, err)
	}

	_, err = b.PutScheduledAction(
		"ecs", "service/default/svc", "ecs:service:DesiredCount",
		"action-200", "rate(1 hour)", "", nil, nil, nil,
	)
	require.ErrorIs(t, err, applicationautoscaling.ErrLimitExceeded)
}

// TestHandler_DescribeOps_InvalidNextToken verifies every Describe* op
// rejects a malformed NextToken with InvalidNextTokenException/400, matching
// each op's modeled error set in the vendored SDK.
func TestHandler_DescribeOps_InvalidNextToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{
			name:   "DescribeScalableTargets",
			action: "DescribeScalableTargets",
			body:   map[string]any{"ServiceNamespace": "ecs", "NextToken": "not-valid-base64!!"},
		},
		{
			name:   "DescribeScalingPolicies",
			action: "DescribeScalingPolicies",
			body:   map[string]any{"ServiceNamespace": "ecs", "NextToken": "not-valid-base64!!"},
		},
		{
			name:   "DescribeScheduledActions",
			action: "DescribeScheduledActions",
			body:   map[string]any{"ServiceNamespace": "ecs", "NextToken": "not-valid-base64!!"},
		},
		{
			name:   "DescribeScalingActivities",
			action: "DescribeScalingActivities",
			body:   map[string]any{"ServiceNamespace": "ecs", "NextToken": "not-valid-base64!!"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.action, tt.body)
			require.Equal(t, http.StatusBadRequest, rec.Code)

			var errBody map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
			assert.Equal(t, "InvalidNextTokenException", errBody["__type"])
		})
	}
}

// TestBackend_DescribeScalableTargets_ValidNextTokenRoundTrips verifies the
// opaque NextToken produced by one call is accepted back by the next --
// guards against the InvalidNextTokenException check rejecting gopherstack's
// own valid tokens.
func TestBackend_DescribeScalableTargets_ValidNextTokenRoundTrips(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
	for i := range 3 {
		_, err := b.RegisterScalableTarget(
			"ecs", fmt.Sprintf("service/default/svc-%d", i), "ecs:service:DesiredCount",
			int32p(1), int32p(10), nil, "", nil,
		)
		require.NoError(t, err)
	}

	page1, next, err := b.DescribeScalableTargets(applicationautoscaling.DescribeScalableTargetsFilter{
		ServiceNamespace: "ecs",
		MaxResults:       2,
	})
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, next)

	page2, next2, err := b.DescribeScalableTargets(applicationautoscaling.DescribeScalableTargetsFilter{
		ServiceNamespace: "ecs",
		MaxResults:       2,
		NextToken:        next,
	})
	require.NoError(t, err)
	require.Len(t, page2, 1)
	assert.Empty(t, next2)
}
