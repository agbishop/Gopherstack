package apprunner_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoScalingConfigurationCRUD(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:     "create returns ARN and defaults",
			action:   "CreateAutoScalingConfiguration",
			body:     map[string]any{"AutoScalingConfigurationName": "my-asg"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				cfg := resp["AutoScalingConfiguration"].(map[string]any)
				assert.Contains(t, cfg["AutoScalingConfigurationArn"], "autoscalingconfiguration/my-asg/1/")
				assert.Equal(t, "ACTIVE", cfg["Status"])
				assert.InDelta(t, float64(1), cfg["AutoScalingConfigurationRevision"], 0.0001)
				assert.InDelta(t, float64(100), cfg["MaxConcurrency"], 0.0001)
				assert.InDelta(t, float64(25), cfg["MaxSize"], 0.0001)
				assert.InDelta(t, float64(1), cfg["MinSize"], 0.0001)
			},
		},
		{
			name:     "create missing name returns 400",
			action:   "CreateAutoScalingConfiguration",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestAutoScalingConfigurationDescribeDeleteList(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateAutoScalingConfiguration", map[string]any{
		"AutoScalingConfigurationName": "cfg1",
		"MaxConcurrency":               50,
		"MaxSize":                      10,
		"MinSize":                      2,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	asgArn := createResp["AutoScalingConfiguration"].(map[string]any)["AutoScalingConfigurationArn"].(string)

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:     "describe returns config",
			action:   "DescribeAutoScalingConfiguration",
			body:     map[string]any{"AutoScalingConfigurationArn": asgArn},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				cfg := resp["AutoScalingConfiguration"].(map[string]any)
				assert.Equal(t, "cfg1", cfg["AutoScalingConfigurationName"])
				assert.InDelta(t, float64(50), cfg["MaxConcurrency"], 0.0001)
			},
		},
		{
			name:     "describe missing ARN returns 400",
			action:   "DescribeAutoScalingConfiguration",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "describe unknown ARN returns 400",
			action: "DescribeAutoScalingConfiguration",
			body: map[string]any{
				"AutoScalingConfigurationArn": "arn:aws:apprunner:us-east-1:000000000000:autoscalingconfiguration/notexist/1/abc",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "list returns config",
			action:   "ListAutoScalingConfigurations",
			body:     map[string]any{},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				list := resp["AutoScalingConfigurationSummaryList"].([]any)
				// cfg1 plus the account's always-present DefaultConfiguration
				// (see ensureDefaultAutoScalingConfiguration).
				assert.Len(t, list, 2)
			},
		},
		{
			name:     "delete returns config",
			action:   "DeleteAutoScalingConfiguration",
			body:     map[string]any{"AutoScalingConfigurationArn": asgArn},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				cfg := resp["AutoScalingConfiguration"].(map[string]any)
				assert.Equal(t, "cfg1", cfg["AutoScalingConfigurationName"])
			},
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, tc.action, tc.body) //nolint:govet // existing issue.
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestAutoScalingConfigurationRevisions(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateAutoScalingConfiguration", map[string]any{"AutoScalingConfigurationName": "my-asg"})
	require.Equal(t, http.StatusOK, rec.Code)
	var r1 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &r1))
	asgArn1 := r1["AutoScalingConfiguration"].(map[string]any)["AutoScalingConfigurationArn"].(string)

	rec = doRequest(t, h, "CreateAutoScalingConfiguration", map[string]any{"AutoScalingConfigurationName": "my-asg"})
	require.Equal(t, http.StatusOK, rec.Code)
	var r2 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &r2))
	rev2 := r2["AutoScalingConfiguration"].(map[string]any)["AutoScalingConfigurationRevision"].(float64)
	assert.InDelta(t, float64(2), rev2, 0.0001)

	// LatestOnly's doc (aws-sdk-go-v2/service/apprunner@v1.42.4
	// api_op_ListAutoScalingConfigurations.go): "Default: true" -- an omitted
	// LatestOnly must behave the same as an explicit true, not the same as
	// an explicit false.
	rec = doRequest(t, h, "ListAutoScalingConfigurations", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	list := listResp["AutoScalingConfigurationSummaryList"].([]any)
	// my-asg's latest revision plus DefaultConfiguration.
	assert.Len(t, list, 2)

	rec = doRequest(t, h, "ListAutoScalingConfigurations", map[string]any{"LatestOnly": true})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	list = listResp["AutoScalingConfigurationSummaryList"].([]any)
	// my-asg's latest revision plus DefaultConfiguration.
	assert.Len(t, list, 2)

	rec = doRequest(t, h, "ListAutoScalingConfigurations", map[string]any{"LatestOnly": false})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	list = listResp["AutoScalingConfigurationSummaryList"].([]any)
	// my-asg's 2 revisions plus the account's always-present
	// DefaultConfiguration (see ensureDefaultAutoScalingConfiguration).
	assert.Len(t, list, 3)

	rec = doRequest(t, h, "UpdateDefaultAutoScalingConfiguration", map[string]any{
		"AutoScalingConfigurationArn": asgArn1,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var updateResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updateResp))
	cfg := updateResp["AutoScalingConfiguration"].(map[string]any)
	assert.Equal(t, true, cfg["IsDefault"])
}

func TestListServicesForAutoScalingConfiguration(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateAutoScalingConfiguration", map[string]any{"AutoScalingConfigurationName": "my-asg"})
	require.Equal(t, http.StatusOK, rec.Code)
	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	asgArn := createResp["AutoScalingConfiguration"].(map[string]any)["AutoScalingConfigurationArn"].(string)

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		wantCode int
	}{
		{
			name:     "list services returns empty list",
			body:     map[string]any{"AutoScalingConfigurationArn": asgArn},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				list := resp["ServiceArnList"].([]any)
				assert.Empty(t, list)
			},
		},
		{
			name:     "missing ARN returns 400",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "unknown ARN returns 400",
			body: map[string]any{
				"AutoScalingConfigurationArn": "arn:aws:apprunner:us-east-1:000000000000:autoscalingconfiguration/notexist/1/abc",
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, "ListServicesForAutoScalingConfiguration", tc.body) //nolint:govet // existing issue.
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

// TestDeleteAutoScalingConfiguration_RejectsDefaultAndInUse verifies
// DeleteAutoScalingConfiguration fails for the account's default
// configuration and for one still associated with a service
// (api_op_DeleteAutoScalingConfiguration.go: "You can't delete the default
// auto scaling configuration or a configuration that's used by one or more
// App Runner services."), and succeeds once neither condition holds.
func TestDeleteAutoScalingConfiguration_RejectsDefaultAndInUse(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	t.Run("default configuration cannot be deleted", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest(t, h, "ListAutoScalingConfigurations", map[string]any{})
		require.Equal(t, http.StatusOK, rec.Code)
		var listResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
		list := listResp["AutoScalingConfigurationSummaryList"].([]any)
		require.Len(t, list, 1, "only the seeded default configuration should exist yet")
		defaultArn := list[0].(map[string]any)["AutoScalingConfigurationArn"].(string)

		rec = doRequest(
			t, h, "DeleteAutoScalingConfiguration", map[string]any{"AutoScalingConfigurationArn": defaultArn},
		)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("configuration in use cannot be deleted", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest(
			t, h, "CreateAutoScalingConfiguration", map[string]any{"AutoScalingConfigurationName": "in-use-asg"},
		)
		require.Equal(t, http.StatusOK, rec.Code)
		var createResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
		asgArn := createResp["AutoScalingConfiguration"].(map[string]any)["AutoScalingConfigurationArn"].(string)

		rec = doRequest(t, h, "CreateService", map[string]any{
			"ServiceName": "asg-svc",
			"SourceConfiguration": map[string]any{
				"ImageRepository": map[string]any{"ImageIdentifier": "img", "ImageRepositoryType": "ECR_PUBLIC"},
			},
			"AutoScalingConfigurationArn": asgArn,
		})
		require.Equal(t, http.StatusOK, rec.Code)
		var svcResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &svcResp))
		svcArn := svcResp["Service"].(map[string]any)["ServiceArn"].(string)

		rec = doRequest(t, h, "DeleteAutoScalingConfiguration", map[string]any{"AutoScalingConfigurationArn": asgArn})
		assert.Equal(t, http.StatusBadRequest, rec.Code, "asg still associated with asg-svc must not be deletable")

		rec = doRequest(t, h, "DeleteService", map[string]any{"ServiceArn": svcArn})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = doRequest(t, h, "DeleteAutoScalingConfiguration", map[string]any{"AutoScalingConfigurationArn": asgArn})
		assert.Equal(t, http.StatusOK, rec.Code, "asg must be deletable once no service references it")
	})
}
