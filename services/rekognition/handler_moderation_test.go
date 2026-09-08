package rekognition_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModeration_ImageAnalysis(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:     "DetectModerationLabels returns empty list",
			action:   "DetectModerationLabels",
			body:     map[string]any{"Image": map[string]any{}},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.NotNil(t, resp["ModerationLabels"])
			},
		},
		{
			name:     "DetectProtectiveEquipment returns empty list",
			action:   "DetectProtectiveEquipment",
			body:     map[string]any{"Image": map[string]any{}},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.NotNil(t, resp["Persons"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code, tc.name)

			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

// gopherstack-duj0: DetectProtectiveEquipment's SummarizationAttributes has two
// required members (MinConfidence, RequiredEquipmentTypes -- both "This member
// is required" in validateProtectiveEquipmentSummarizationAttributes,
// rekognition@v1.54.4 validators.go), enforced only when SummarizationAttributes
// itself is present (it's an optional top-level field).
func TestDetectProtectiveEquipment_SummarizationAttributesValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		summarizationAttributes any
		name                    string
		wantType                string
		wantCode                int
	}{
		{
			name:                    "missing MinConfidence is rejected",
			summarizationAttributes: map[string]any{"RequiredEquipmentTypes": []string{"FACE_COVER"}},
			wantCode:                http.StatusBadRequest,
			wantType:                "InvalidParameterException",
		},
		{
			name:                    "missing RequiredEquipmentTypes is rejected",
			summarizationAttributes: map[string]any{"MinConfidence": 50},
			wantCode:                http.StatusBadRequest,
			wantType:                "InvalidParameterException",
		},
		{
			name: "complete SummarizationAttributes succeeds",
			summarizationAttributes: map[string]any{
				"MinConfidence":          50,
				"RequiredEquipmentTypes": []string{"FACE_COVER"},
			},
			wantCode: http.StatusOK,
		},
		{
			name: "explicit empty RequiredEquipmentTypes array is not the same as missing",
			summarizationAttributes: map[string]any{
				"MinConfidence":          50,
				"RequiredEquipmentTypes": []string{},
			},
			wantCode: http.StatusOK,
		},
		{
			name:                    "omitted SummarizationAttributes stays legal",
			summarizationAttributes: nil,
			wantCode:                http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			body := map[string]any{"Image": map[string]any{}}
			if tc.summarizationAttributes != nil {
				body["SummarizationAttributes"] = tc.summarizationAttributes
			}

			rec := doRequest(t, h, "DetectProtectiveEquipment", body)
			require.Equal(t, tc.wantCode, rec.Code)

			if tc.wantType != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tc.wantType, resp["__type"])
			}
		})
	}
}

func TestAsyncVideoJobs_ContentModeration(t *testing.T) { //nolint:paralleltest // stateful sequential
	h := newTestHandler(t)

	// Start job
	rec := doRequest(t, h, "StartContentModeration", map[string]any{"Video": map[string]any{}})
	require.Equal(t, http.StatusOK, rec.Code, "StartContentModeration")

	var startResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
	jobID, ok := startResp["JobId"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, jobID)

	// First poll returns IN_PROGRESS
	rec = doRequest(t, h, "GetContentModeration", map[string]any{"JobId": jobID})
	require.Equal(t, http.StatusOK, rec.Code, "GetContentModeration")

	var firstResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &firstResp))
	assert.Equal(t, "IN_PROGRESS", firstResp["JobStatus"])

	// Second poll returns SUCCEEDED
	rec = doRequest(t, h, "GetContentModeration", map[string]any{"JobId": jobID})
	require.Equal(t, http.StatusOK, rec.Code, "GetContentModeration")

	var secondResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &secondResp))
	assert.Equal(t, "SUCCEEDED", secondResp["JobStatus"])
	assert.NotNil(t, secondResp["ModerationLabels"])
}

func TestAsyncVideoJobs_ContentModeration_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "GetContentModeration", map[string]any{"JobId": "00000000-0000-0000-0000-000000000000"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAsyncVideoJobs_ContentModeration_MissingJobId(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "GetContentModeration", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
