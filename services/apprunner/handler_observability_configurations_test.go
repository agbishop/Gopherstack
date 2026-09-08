package apprunner_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObservabilityConfigurationCRUD(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "create returns ARN",
			action: "CreateObservabilityConfiguration",
			body: map[string]any{
				"ObservabilityConfigurationName": "my-obs",
				"TraceConfiguration":             map[string]any{"Vendor": "AWSXRAY"},
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				cfg := resp["ObservabilityConfiguration"].(map[string]any)
				assert.Contains(t, cfg["ObservabilityConfigurationArn"], "observabilityconfiguration/my-obs/1/")
				assert.Equal(t, "ACTIVE", cfg["Status"])
				assert.Equal(t, true, cfg["Latest"])
				assert.InDelta(t, float64(1), cfg["ObservabilityConfigurationRevision"], 0.0001)
			},
		},
		{
			name:     "create missing name returns 400",
			action:   "CreateObservabilityConfiguration",
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

func TestObservabilityConfigurationDescribeDeleteList(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateObservabilityConfiguration", map[string]any{
		"ObservabilityConfigurationName": "obs1",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	obsArn := createResp["ObservabilityConfiguration"].(map[string]any)["ObservabilityConfigurationArn"].(string)

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:     "describe returns config",
			action:   "DescribeObservabilityConfiguration",
			body:     map[string]any{"ObservabilityConfigurationArn": obsArn},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				cfg := resp["ObservabilityConfiguration"].(map[string]any)
				assert.Equal(t, "obs1", cfg["ObservabilityConfigurationName"])
			},
		},
		{
			name:     "describe missing ARN returns 400",
			action:   "DescribeObservabilityConfiguration",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "describe unknown ARN returns 400",
			action: "DescribeObservabilityConfiguration",
			body: map[string]any{
				"ObservabilityConfigurationArn": "arn:aws:apprunner:us-east-1:000000000000:observabilityconfiguration/notexist/1/abc", //nolint:lll // existing issue.
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "list returns config",
			action:   "ListObservabilityConfigurations",
			body:     map[string]any{},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				list := resp["ObservabilityConfigurationSummaryList"].([]any)
				assert.Len(t, list, 1)
			},
		},
		{
			name:     "delete returns config",
			action:   "DeleteObservabilityConfiguration",
			body:     map[string]any{"ObservabilityConfigurationArn": obsArn},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				cfg := resp["ObservabilityConfiguration"].(map[string]any)
				assert.Equal(t, "obs1", cfg["ObservabilityConfigurationName"])
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

// TestDeleteObservabilityConfiguration_RejectsWhenServiceUsesIt verifies
// DeleteObservabilityConfiguration fails while a service still has it
// enabled (api_op_DeleteObservabilityConfiguration.go: "You can't delete a
// configuration that's used by one or more App Runner services."), and
// succeeds once that service no longer references it.
func TestDeleteObservabilityConfiguration_RejectsWhenServiceUsesIt(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateObservabilityConfiguration", map[string]any{
		"ObservabilityConfigurationName": "obs-in-use",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var obsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &obsResp))
	obsArn := obsResp["ObservabilityConfiguration"].(map[string]any)["ObservabilityConfigurationArn"].(string)

	rec = doRequest(t, h, "CreateService", map[string]any{
		"ServiceName": "obs-svc",
		"SourceConfiguration": map[string]any{
			"ImageRepository": map[string]any{"ImageIdentifier": "img", "ImageRepositoryType": "ECR_PUBLIC"},
		},
		"ObservabilityConfiguration": map[string]any{
			"ObservabilityEnabled":          true,
			"ObservabilityConfigurationArn": obsArn,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var svcResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &svcResp))
	svcArn := svcResp["Service"].(map[string]any)["ServiceArn"].(string)

	rec = doRequest(t, h, "DeleteObservabilityConfiguration", map[string]any{"ObservabilityConfigurationArn": obsArn})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "config still referenced by obs-svc must not be deletable")

	rec = doRequest(t, h, "DeleteService", map[string]any{"ServiceArn": svcArn})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DeleteObservabilityConfiguration", map[string]any{"ObservabilityConfigurationArn": obsArn})
	assert.Equal(t, http.StatusOK, rec.Code, "config must be deletable once no service references it")
}

// LatestOnly's doc (aws-sdk-go-v2/service/apprunner@v1.42.4
// api_op_ListObservabilityConfigurations.go): "Default: true" -- an omitted
// LatestOnly must behave the same as an explicit true, not the same as an
// explicit false.
func TestObservabilityConfigurationRevisionsLatestOnlyDefault(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	rec := doRequest(
		t, h, "CreateObservabilityConfiguration", map[string]any{"ObservabilityConfigurationName": "my-obs"},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(
		t, h, "CreateObservabilityConfiguration", map[string]any{"ObservabilityConfigurationName": "my-obs"},
	)
	require.Equal(t, http.StatusOK, rec.Code)
	var r2 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &r2))
	rev2 := r2["ObservabilityConfiguration"].(map[string]any)["ObservabilityConfigurationRevision"].(float64)
	assert.InDelta(t, float64(2), rev2, 0.0001)

	rec = doRequest(t, h, "ListObservabilityConfigurations", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	list := listResp["ObservabilityConfigurationSummaryList"].([]any)
	assert.Len(t, list, 1)

	rec = doRequest(t, h, "ListObservabilityConfigurations", map[string]any{"LatestOnly": true})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	list = listResp["ObservabilityConfigurationSummaryList"].([]any)
	assert.Len(t, list, 1)

	rec = doRequest(t, h, "ListObservabilityConfigurations", map[string]any{"LatestOnly": false})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	list = listResp["ObservabilityConfigurationSummaryList"].([]any)
	assert.Len(t, list, 2)
}
