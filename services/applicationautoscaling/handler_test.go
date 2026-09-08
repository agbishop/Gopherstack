package applicationautoscaling_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/applicationautoscaling"
)

func newTestHandler(t *testing.T) *applicationautoscaling.Handler {
	t.Helper()

	return applicationautoscaling.NewHandler(applicationautoscaling.NewInMemoryBackend("000000000000", "us-east-1"))
}

// int32p is a test helper for RegisterScalableTarget's *int32
// MinCapacity/MaxCapacity parameters, which distinguish "not specified" (nil,
// leaves the existing value unchanged on update) from an explicit value.
func int32p(v int32) *int32 { return new(v) }

func doRequest(t *testing.T, h *applicationautoscaling.Handler, action string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	} else {
		bodyBytes = []byte("{}")
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AnyScaleFrontendService."+action)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func doInvalidRequest(t *testing.T, h *applicationautoscaling.Handler, action string) *httptest.ResponseRecorder {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AnyScaleFrontendService."+action)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

// seedTarget is a helper that registers an ECS scalable target and asserts success.
func seedTarget(t *testing.T, h *applicationautoscaling.Handler, resID string, minCap, maxCap int) string {
	t.Helper()

	return seedTargetNS(t, h, "ecs", resID, "ecs:service:DesiredCount", minCap, maxCap)
}

// seedTargetNS is a helper that registers a scalable target for an arbitrary
// (serviceNamespace, scalableDimension) pair and asserts success. Real AWS
// requires a scalable target to already be registered before PutScalingPolicy/
// PutScheduledAction will succeed (both model ObjectNotFoundException), so
// every test exercising those two ops must seed a target first.
func seedTargetNS(
	t *testing.T, h *applicationautoscaling.Handler,
	serviceNamespace, resID, scalableDimension string,
	minCap, maxCap int,
) string {
	t.Helper()

	rec := doRequest(t, h, "RegisterScalableTarget", map[string]any{
		"ServiceNamespace":  serviceNamespace,
		"ResourceId":        resID,
		"ScalableDimension": scalableDimension,
		"MinCapacity":       minCap,
		"MaxCapacity":       maxCap,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	return out["ScalableTargetARN"].(string)
}

func TestHandler_Name(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, "ApplicationAutoscaling", h.Name())
}

func TestHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.GetSupportedOperations()
	assert.Contains(t, ops, "RegisterScalableTarget")
	assert.Contains(t, ops, "DeregisterScalableTarget")
	assert.Contains(t, ops, "DescribeScalableTargets")
	assert.Contains(t, ops, "PutScalingPolicy")
	assert.Contains(t, ops, "DeleteScalingPolicy")
	assert.Contains(t, ops, "DescribeScalingPolicies")
	assert.Contains(t, ops, "DescribeScalingActivities")
	assert.Contains(t, ops, "PutScheduledAction")
	assert.Contains(t, ops, "DeleteScheduledAction")
	assert.Contains(t, ops, "DescribeScheduledActions")
	assert.Contains(t, ops, "ListTagsForResource")
	assert.Contains(t, ops, "TagResource")
	assert.Contains(t, ops, "UntagResource")
}

func TestHandler_MatchPriority(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, 100, h.MatchPriority())
}

func TestHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    string
		wantMatch bool
	}{
		{name: "match", target: "AnyScaleFrontendService.RegisterScalableTarget", wantMatch: true},
		{name: "no_match", target: "AWSScheduler.CreateSchedule", wantMatch: false},
		{name: "empty", target: "", wantMatch: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			matcher := h.RouteMatcher()

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			c := e.NewContext(req, httptest.NewRecorder())

			assert.Equal(t, tt.wantMatch, matcher(c))
		})
	}
}

func TestHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	e := echo.New()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Amz-Target", "AnyScaleFrontendService.RegisterScalableTarget")
	c := e.NewContext(req, httptest.NewRecorder())
	assert.Equal(t, "RegisterScalableTarget", h.ExtractOperation(c))
}

func TestHandler_ErrorCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "DeregisterScalableTarget_NotFound",
			action: "DeregisterScalableTarget",
			body: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/nonexistent",
				"ScalableDimension": "ecs:service:DesiredCount",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "DeleteScalingPolicy_NotFound",
			action: "DeleteScalingPolicy",
			body: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/my-svc",
				"ScalableDimension": "ecs:service:DesiredCount",
				"PolicyName":        "nonexistent",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "DeleteScheduledAction_NotFound",
			action: "DeleteScheduledAction",
			body: map[string]any{
				"ServiceNamespace":    "ecs",
				"ResourceId":          "service/default/my-svc",
				"ScalableDimension":   "ecs:service:DesiredCount",
				"ScheduledActionName": "nonexistent",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "TagResource_NotFound",
			action: "TagResource",
			body: map[string]any{
				"ResourceARN": "arn:aws:application-autoscaling:us-east-1:000000000000:scalable-target/nonexistent",
				"Tags":        map[string]string{"env": "test"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "ListTagsForResource_NotFound",
			action: "ListTagsForResource",
			body: map[string]any{
				"ResourceARN": "arn:aws:application-autoscaling:us-east-1:000000000000:scalable-target/nonexistent",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "UntagResource_NotFound",
			action: "UntagResource",
			body: map[string]any{
				"ResourceARN": "arn:aws:application-autoscaling:us-east-1:000000000000:scalable-target/nonexistent",
				"TagKeys":     []string{"env"},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "UnknownAction",
			action:   "UnknownAction",
			body:     nil,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		action   string
		wantCode int
	}{
		{name: "RegisterScalableTarget", action: "RegisterScalableTarget", wantCode: http.StatusBadRequest},
		{name: "DeregisterScalableTarget", action: "DeregisterScalableTarget", wantCode: http.StatusBadRequest},
		{name: "PutScalingPolicy", action: "PutScalingPolicy", wantCode: http.StatusBadRequest},
		{name: "DeleteScalingPolicy", action: "DeleteScalingPolicy", wantCode: http.StatusBadRequest},
		{name: "TagResource", action: "TagResource", wantCode: http.StatusBadRequest},
		{name: "UntagResource", action: "UntagResource", wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doInvalidRequest(t, h, tt.action)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestHandlerErrorCodes verifies correct HTTP status codes for error cases
// spanning several operations.
func TestHandlerErrorCodes(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		body     map[string]any
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "DeregisterScalableTarget notfound",
			action: "DeregisterScalableTarget",
			body: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/nonexistent",
				"ScalableDimension": "ecs:service:DesiredCount",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "DeleteScalingPolicy notfound",
			action: "DeleteScalingPolicy",
			body: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/x",
				"ScalableDimension": "ecs:service:DesiredCount",
				"PolicyName":        "no-such-policy",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "DeleteScheduledAction notfound",
			action: "DeleteScheduledAction",
			body: map[string]any{
				"ServiceNamespace":    "ecs",
				"ResourceId":          "service/default/x",
				"ScalableDimension":   "ecs:service:DesiredCount",
				"ScheduledActionName": "no-such-action",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "RegisterScalableTarget min>max",
			action: "RegisterScalableTarget",
			body: map[string]any{
				"ServiceNamespace":  "ecs",
				"ResourceId":        "service/default/x",
				"ScalableDimension": "ecs:service:DesiredCount",
				"MinCapacity":       10,
				"MaxCapacity":       5,
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "DescribeScalingActivities missing namespace",
			action:   "DescribeScalingActivities",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code, "action=%s", tc.action)
		})
	}
}

func TestProvider(t *testing.T) {
	t.Parallel()

	p := &applicationautoscaling.Provider{}
	assert.Equal(t, "ApplicationAutoscaling", p.Name())
}

func TestProviderInit(t *testing.T) {
	t.Parallel()

	p := &applicationautoscaling.Provider{}
	ctx := &service.AppContext{Logger: slog.Default()}
	svc, err := p.Init(ctx)
	require.NoError(t, err)
	assert.NotNil(t, svc)
	assert.Equal(t, "ApplicationAutoscaling", svc.Name())
}

func TestApplicationAutoScaling_Handler_Reset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		createTargets int
		wantAfter     int
	}{
		{
			name:          "reset clears all scalable targets",
			createTargets: 2,
			wantAfter:     0,
		},
		{
			name:          "reset on empty backend is a no-op",
			createTargets: 0,
			wantAfter:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for i := range tt.createTargets {
				rec := doRequest(t, h, "RegisterScalableTarget", map[string]any{
					"ServiceNamespace":  "ecs",
					"ResourceId":        fmt.Sprintf("service/cluster/svc-%d", i),
					"ScalableDimension": "ecs:service:DesiredCount",
					"MinCapacity":       1,
					"MaxCapacity":       10,
				})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			h.Reset()

			rec := doRequest(t, h, "DescribeScalableTargets", map[string]any{
				"ServiceNamespace": "ecs",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

			targets, _ := out["ScalableTargets"].([]any)
			assert.Len(t, targets, tt.wantAfter)
		})
	}
}

func TestHandler_Backend_Purge(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.RegisterScalableTarget(
		"ecs", "service/default/my-svc", "ecs:service:DesiredCount", int32p(1), int32p(5), nil, "", nil,
	)
	require.NoError(t, err)

	b.Purge()
	targets, _, _ := b.DescribeScalableTargets(applicationautoscaling.DescribeScalableTargetsFilter{})
	assert.Empty(t, targets, "Purge should clear all scalable targets")
}

func TestHandler_ChaosAndRegion(t *testing.T) {
	t.Parallel()

	b := applicationautoscaling.NewInMemoryBackend("123456789012", "us-east-1")
	h := applicationautoscaling.NewHandler(b)

	tests := []struct {
		want any
		fn   func() any
		name string
	}{
		{
			name: "ChaosServiceName",
			fn:   func() any { return h.ChaosServiceName() },
			want: "applicationautoscaling",
		},
		{
			name: "ChaosOperations",
			fn:   func() any { return len(h.ChaosOperations()) > 0 },
			want: true,
		},
		{
			name: "ChaosRegions",
			fn:   func() any { return h.ChaosRegions() },
			want: []string{"us-east-1"},
		},
		{
			name: "Region",
			fn:   func() any { return b.Region() },
			want: "us-east-1",
		},
		{
			name: "ExtractResource",
			fn:   func() any { return h.ExtractResource(nil) },
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.fn())
		})
	}
}
