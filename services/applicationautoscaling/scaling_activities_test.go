package applicationautoscaling_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/applicationautoscaling"
)

// TestDescribeScalingActivities_TracksRegistrations verifies that registering and
// updating a scalable target records scaling activities and that
// DescribeScalingActivities returns and filters them correctly.
func TestDescribeScalingActivities_TracksRegistrations(t *testing.T) {
	t.Parallel()

	const (
		ns        = "ecs"
		resA      = "service/default/svc-a"
		resB      = "service/default/svc-b"
		dimension = "ecs:service:DesiredCount"
	)

	tests := []struct {
		name          string
		filterRes     string
		filterDim     string
		wantNamespace string
		wantCount     int
	}{
		{
			name:          "all_in_namespace",
			wantCount:     3, // svc-a registered+updated (2) + svc-b registered (1)
			wantNamespace: ns,
		},
		{
			name:      "filter_by_resource_a",
			filterRes: resA,
			wantCount: 2,
		},
		{
			name:      "filter_by_resource_b",
			filterRes: resB,
			wantCount: 1,
		},
		{
			name:      "filter_by_resource_and_dimension",
			filterRes: resA,
			filterDim: dimension,
			wantCount: 2,
		},
		{
			name:      "filter_unknown_resource",
			filterRes: "service/default/missing",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")

			// Register svc-a, then update it (RegisterScalableTarget upserts).
			_, err := b.RegisterScalableTarget(ns, resA, dimension, int32p(1), int32p(5), nil, "", nil)
			require.NoError(t, err)
			_, err = b.RegisterScalableTarget(ns, resA, dimension, int32p(2), int32p(10), nil, "", nil)
			require.NoError(t, err)

			// Register svc-b once.
			_, err = b.RegisterScalableTarget(ns, resB, dimension, int32p(1), int32p(3), nil, "", nil)
			require.NoError(t, err)

			activities, _, _ := b.DescribeScalingActivities(applicationautoscaling.DescribeScalingActivitiesFilter{
				ServiceNamespace:  ns,
				ResourceID:        tt.filterRes,
				ScalableDimension: tt.filterDim,
			})
			assert.Len(t, activities, tt.wantCount)

			for _, a := range activities {
				assert.Equal(t, ns, a.ServiceNamespace)
				assert.Equal(t, "Successful", a.StatusCode)
				assert.NotEmpty(t, a.ActivityID)
				assert.NotEmpty(t, a.Description)

				if tt.filterRes != "" {
					assert.Equal(t, tt.filterRes, a.ResourceID)
				}
			}
		})
	}
}

// TestDescribeScalingActivities_ResetClears verifies Reset clears recorded activities.
func TestDescribeScalingActivities_ResetClears(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.RegisterScalableTarget(
		"ecs", "service/default/svc", "ecs:service:DesiredCount", int32p(1), int32p(5), nil, "", nil,
	)
	require.NoError(t, err)

	got, _, _ := b.DescribeScalingActivities(
		applicationautoscaling.DescribeScalingActivitiesFilter{ServiceNamespace: "ecs"},
	)
	require.Len(t, got, 1)

	b.Reset()
	after, _, _ := b.DescribeScalingActivities(
		applicationautoscaling.DescribeScalingActivitiesFilter{ServiceNamespace: "ecs"},
	)
	assert.Empty(t, after)
}

func TestHandler_DescribeScalingActivities(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "DescribeScalingActivities", map[string]any{"ServiceNamespace": "ecs"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	activities, ok := resp["ScalingActivities"].([]any)
	require.True(t, ok)
	assert.Empty(t, activities)
}

func TestHandler_DescribeScalingActivities_AfterRegister(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	regRec := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
		"MinCapacity":       int32(1),
		"MaxCapacity":       int32(5),
	})
	require.Equal(t, http.StatusOK, regRec.Code)

	rec := doRequest(t, h, "DescribeScalingActivities", map[string]any{
		"ServiceNamespace":  "ecs",
		"ResourceId":        "service/default/my-svc",
		"ScalableDimension": "ecs:service:DesiredCount",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	activities, ok := resp["ScalingActivities"].([]any)
	require.True(t, ok)
	require.Len(t, activities, 1)

	act, ok := activities[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ecs", act["ServiceNamespace"])
	assert.Equal(t, "service/default/my-svc", act["ResourceId"])
	assert.Equal(t, "Successful", act["StatusCode"])
}

func TestHandler_DescribeScalingActivities_MissingNamespace(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "DescribeScalingActivities", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_DescribeScalingActivities_WithInput(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "DescribeScalingActivities", map[string]any{
		"ServiceNamespace":           "ecs",
		"ResourceId":                 "service/default/my-svc",
		"ScalableDimension":          "ecs:service:DesiredCount",
		"MaxResults":                 int32(10),
		"IncludeNotScaledActivities": true,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	acts, ok := resp["ScalingActivities"].([]any)
	require.True(t, ok, "ScalingActivities should be an array")
	assert.Empty(t, acts, "expected empty array not null")
}

func TestHandler_DescribeScalingActivities_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	const (
		ns  = "ecs"
		dim = "ecs:service:DesiredCount"
	)

	for i := range 5 {
		seedTarget(t, h, "service/default/svc"+string(rune('a'+i)), 1, 10)
	}

	rec1 := doRequest(t, h, "DescribeScalingActivities", map[string]any{
		"ServiceNamespace": ns,
		"MaxResults":       2,
	})
	require.Equal(t, http.StatusOK, rec1.Code)
	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &out1))
	page1 := out1["ScalingActivities"].([]any)
	assert.Len(t, page1, 2)
	nextToken, ok := out1["NextToken"].(string)
	assert.True(t, ok && nextToken != "", "NextToken must be present after partial page")

	rec2 := doRequest(t, h, "DescribeScalingActivities", map[string]any{
		"ServiceNamespace": ns,
		"MaxResults":       2,
		"NextToken":        nextToken,
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))
	page2 := out2["ScalingActivities"].([]any)
	assert.Len(t, page2, 2)
}

func TestHandler_DescribeScalingActivities_TypedFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	seedTarget(t, h, "service/default/typed", 1, 5)

	rec := doRequest(t, h, "DescribeScalingActivities", map[string]any{
		"ServiceNamespace": "ecs",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	activities := out["ScalingActivities"].([]any)
	require.Len(t, activities, 1)

	a := activities[0].(map[string]any)
	assert.NotEmpty(t, a["ActivityId"], "ActivityId must be present")
	assert.NotEmpty(t, a["ResourceId"], "ResourceId must be present")
	assert.NotEmpty(t, a["StatusCode"], "StatusCode must be present")
	assert.NotNil(t, a["StartTime"], "StartTime must be present as epoch seconds")
}
