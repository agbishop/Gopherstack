package applicationautoscaling_test

// Tests for NextToken pagination on Application Auto Scaling Describe* ops.
// Prior to this the ops accepted MaxResults but never emitted NextToken, so a
// client could not page past the first MaxResults items.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/applicationautoscaling"
)

func registerN(t *testing.T, b *applicationautoscaling.InMemoryBackend, n int) {
	t.Helper()

	for i := range n {
		_, err := b.RegisterScalableTarget(
			"ecs",
			"service/cluster/svc-"+string(rune('a'+i)),
			"ecs:service:DesiredCount",
			int32p(1), int32p(10), nil, "", nil,
		)
		require.NoError(t, err)
	}
}

func TestDescribeScalableTargets_Pagination(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
	registerN(t, b, 5)

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
	require.Len(t, page2, 2)
	require.NotEmpty(t, next2)

	page3, next3, err := b.DescribeScalableTargets(applicationautoscaling.DescribeScalableTargetsFilter{
		ServiceNamespace: "ecs",
		MaxResults:       2,
		NextToken:        next2,
	})
	require.NoError(t, err)
	require.Len(t, page3, 1)
	assert.Empty(t, next3)

	// No resource appears on more than one page.
	seen := map[string]bool{}
	for _, page := range [][]*applicationautoscaling.ScalableTarget{page1, page2, page3} {
		for _, tgt := range page {
			assert.False(t, seen[tgt.ResourceID], "duplicate %s across pages", tgt.ResourceID)
			seen[tgt.ResourceID] = true
		}
	}

	assert.Len(t, seen, 5)
}

func TestDescribeScalingPolicies_Pagination(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
	registerN(t, b, 3)

	for i := range 3 {
		_, err := b.PutScalingPolicy(
			"ecs",
			"service/cluster/svc-"+string(rune('a'+i)),
			"ecs:service:DesiredCount",
			"pol-"+string(rune('a'+i)),
			"TargetTrackingScaling",
			map[string]any{"TargetValue": 50.0},
			nil,
			nil,
		)
		require.NoError(t, err)
	}

	page1, next, err := b.DescribeScalingPolicies(applicationautoscaling.DescribeScalingPoliciesFilter{
		ServiceNamespace: "ecs",
		MaxResults:       2,
	})
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, next)

	page2, next2, err := b.DescribeScalingPolicies(applicationautoscaling.DescribeScalingPoliciesFilter{
		ServiceNamespace: "ecs",
		MaxResults:       2,
		NextToken:        next,
	})
	require.NoError(t, err)
	require.Len(t, page2, 1)
	assert.Empty(t, next2)
}

func TestDescribeScheduledActions_Pagination(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")

	_, err := b.RegisterScalableTarget(
		"ecs", "service/cluster/svc", "ecs:service:DesiredCount", int32p(1), int32p(10), nil, "", nil,
	)
	require.NoError(t, err)

	for i := range 3 {
		_, err = b.PutScheduledAction(
			"ecs",
			"service/cluster/svc",
			"ecs:service:DesiredCount",
			"action-"+string(rune('a'+i)),
			"rate(1 hour)",
			"",
			nil,
			nil,
			nil,
		)
		require.NoError(t, err)
	}

	page1, next, err := b.DescribeScheduledActions(applicationautoscaling.DescribeScheduledActionsFilter{
		ServiceNamespace: "ecs",
		MaxResults:       2,
	})
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotEmpty(t, next)

	page2, next2, err := b.DescribeScheduledActions(applicationautoscaling.DescribeScheduledActionsFilter{
		ServiceNamespace: "ecs",
		MaxResults:       2,
		NextToken:        next,
	})
	require.NoError(t, err)
	require.Len(t, page2, 1)
	assert.Empty(t, next2)
}
