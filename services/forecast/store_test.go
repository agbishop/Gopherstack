package forecast_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/forecast"
)

// TestStatusTransitions_PendingActiveDelete verifies the generic
// CREATE_PENDING -> ACTIVE -> (deleted) lifecycle implemented by
// InMemoryBackend.describe/delete, using Forecast as the vehicle resource.
func TestStatusTransitions_PendingActiveDelete(t *testing.T) {
	t.Parallel()

	h := newHandler()
	predictorARN := createPredictor(t, h)

	code, created := request(t, h, "CreateForecast", map[string]any{
		"ForecastName": "status-forecast", "PredictorArn": predictorARN,
	})
	require.Equal(t, http.StatusOK, code)
	arn := created["ForecastArn"].(string)

	// First describe: CREATE_PENDING
	code, m := request(t, h, "DescribeForecast", map[string]any{"ForecastArn": arn})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "CREATE_PENDING", m["Status"])

	// Second describe: ACTIVE
	code, m = request(t, h, "DescribeForecast", map[string]any{"ForecastArn": arn})
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ACTIVE", m["Status"])

	// Delete
	code, _ = request(t, h, "DeleteForecast", map[string]any{"ForecastArn": arn})
	require.Equal(t, http.StatusOK, code)

	// After delete: resource is removed; describe returns ResourceNotFoundException.
	code, m = request(t, h, "DescribeForecast", map[string]any{"ForecastArn": arn})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "ResourceNotFoundException", m["__type"])
}

// TestARNFormat_AcrossResourceKinds verifies that every resource kind's
// generated ARN follows the arn:aws:forecast:<region>:<account>: prefix built
// by InMemoryBackend.create.
func TestARNFormat_AcrossResourceKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     func(t *testing.T, h *forecast.Handler) map[string]any
		name     string
		action   string
		arnField string
	}{
		{
			name:   "dataset_group_arn",
			action: "CreateDatasetGroup",
			body: func(*testing.T, *forecast.Handler) map[string]any {
				return map[string]any{"DatasetGroupName": "arn-group", "Domain": "RETAIL"}
			},
			arnField: "DatasetGroupArn",
		},
		{
			name:   "dataset_arn",
			action: "CreateDataset",
			body: func(*testing.T, *forecast.Handler) map[string]any {
				return map[string]any{
					"DatasetName": "arn-dataset", "Domain": "RETAIL",
					"DatasetType": "TARGET_TIME_SERIES", "DataFrequency": "D",
					"Schema": map[string]any{"Attributes": []any{}},
				}
			},
			arnField: "DatasetArn",
		},
		{
			name:   "predictor_arn",
			action: "CreatePredictor",
			body: func(t *testing.T, h *forecast.Handler) map[string]any {
				t.Helper()

				return map[string]any{
					"PredictorName": "arn-predictor", "ForecastHorizon": 10,
					"InputDataConfig":     map[string]any{"DatasetGroupArn": createDatasetGroup(t, h)},
					"FeaturizationConfig": map[string]any{},
				}
			},
			arnField: "PredictorArn",
		},
		{
			name:   "forecast_arn",
			action: "CreateForecast",
			body: func(t *testing.T, h *forecast.Handler) map[string]any {
				t.Helper()

				return map[string]any{"ForecastName": "arn-forecast", "PredictorArn": createPredictor(t, h)}
			},
			arnField: "ForecastArn",
		},
		{
			name:   "monitor_arn",
			action: "CreateMonitor",
			body: func(t *testing.T, h *forecast.Handler) map[string]any {
				t.Helper()

				return map[string]any{"MonitorName": "arn-monitor", "ResourceArn": createPredictor(t, h)}
			},
			arnField: "MonitorArn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			code, m := request(t, h, tt.action, tt.body(t, h))
			require.Equal(t, http.StatusOK, code)

			arn, ok := m[tt.arnField].(string)
			require.True(t, ok, "ARN field %s must be a string", tt.arnField)
			assert.Contains(t, arn, "arn:aws:forecast:us-east-1:000000000000:")
		})
	}
}

// TestCreateResource_NameFormatValidation verifies that Forecast Create*
// operations enforce AWS name format rules: only alphanumeric characters,
// underscores, and hyphens are allowed; max 256 characters.
// Real AWS returns InvalidInputException for names that violate these rules;
// the emulator previously accepted any non-empty string.
func TestCreateResource_NameFormatValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resName  string
		wantCode int
	}{
		{
			name:     "space_in_name_rejected",
			resName:  "my dataset",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "dot_in_name_rejected",
			resName:  "my.dataset",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "slash_in_name_rejected",
			resName:  "my/dataset",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "at_sign_rejected",
			resName:  "my@dataset",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "simple_name_accepted",
			resName:  "mydataset",
			wantCode: http.StatusOK,
		},
		{
			name:     "underscores_accepted",
			resName:  "my_dataset_v2",
			wantCode: http.StatusOK,
		},
		{
			name:     "hyphens_accepted",
			resName:  "my-dataset-v2",
			wantCode: http.StatusOK,
		},
		{
			name:     "mixed_case_accepted",
			resName:  "MyDataset123",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			code, resp := request(t, h, "CreateDatasetGroup", map[string]any{
				"DatasetGroupName": tt.resName,
				"Domain":           "RETAIL",
			})

			assert.Equal(t, tt.wantCode, code, "DatasetGroupName=%q", tt.resName)

			if tt.wantCode == http.StatusBadRequest {
				assert.Equal(t, "InvalidInputException", resp["__type"],
					"expected InvalidInputException for DatasetGroupName=%q", tt.resName)
			}
		})
	}
}

// TestCreateResource_NameMaxLength verifies that a 256-character name is
// accepted and a 257-character name is rejected with InvalidInputException.
// Real AWS Forecast enforces a 256-character maximum across all resource types.
func TestCreateResource_NameMaxLength(t *testing.T) {
	t.Parallel()

	h := newHandler()

	validName := strings.Repeat("a", 256)
	code, _ := request(t, h, "CreateDatasetGroup", map[string]any{
		"DatasetGroupName": validName,
		"Domain":           "RETAIL",
	})
	require.Equal(t, http.StatusOK, code, "256-char name should be accepted")

	h2 := newHandler()
	tooLongName := strings.Repeat("a", 257)
	code2, resp2 := request(t, h2, "CreateDatasetGroup", map[string]any{
		"DatasetGroupName": tooLongName,
		"Domain":           "RETAIL",
	})
	assert.Equal(t, http.StatusBadRequest, code2, "257-char name should be rejected")
	assert.Equal(t, "InvalidInputException", resp2["__type"])
}

// TestDeleteResourceTree_RemovesTarget verifies that DeleteResourceTree
// removes the target resource and returns ResourceNotFoundException for a
// missing target ARN.
func TestDeleteResourceTree_RemovesTarget(t *testing.T) {
	t.Parallel()

	h := newHandler()
	predictorARN := createPredictor(t, h)

	code, created := request(t, h, "CreateForecast", map[string]any{
		"ForecastName": "tree-forecast", "PredictorArn": predictorARN,
	})
	require.Equal(t, http.StatusOK, code)
	arn := created["ForecastArn"].(string)

	code, _ = request(t, h, "DeleteResourceTree", map[string]any{"ResourceArn": arn})
	require.Equal(t, http.StatusOK, code)

	// After tree delete: resource is removed; describe returns ResourceNotFoundException.
	code, m := request(t, h, "DescribeForecast", map[string]any{"ForecastArn": arn})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "ResourceNotFoundException", m["__type"])

	// Not found
	code, _ = request(t, h, "DeleteResourceTree", map[string]any{"ResourceArn": "missing"})
	assert.Equal(t, http.StatusBadRequest, code)
}

// TestDelete_ResourceRemovedFromMap verifies that calling Delete* on a
// Forecast resource actually removes it from the backend (AWS behavior), rather
// than leaving a DELETING tombstone. Subsequent Describe and List calls must
// return ResourceNotFoundException / empty list.
func TestDelete_ResourceRemovedFromMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createBody func(t *testing.T, h *forecast.Handler) map[string]any
		name       string
		create     string
		arnField   string
		describe   string
		deleteOp   string
		list       string
		listField  string
	}{
		{
			name:   "dataset_group",
			create: "CreateDatasetGroup",
			createBody: func(*testing.T, *forecast.Handler) map[string]any {
				return map[string]any{"DatasetGroupName": "del-dg", "Domain": "RETAIL"}
			},
			arnField:  "DatasetGroupArn",
			describe:  "DescribeDatasetGroup",
			deleteOp:  "DeleteDatasetGroup",
			list:      "ListDatasetGroups",
			listField: "DatasetGroups",
		},
		{
			name:   "dataset",
			create: "CreateDataset",
			createBody: func(*testing.T, *forecast.Handler) map[string]any {
				return map[string]any{
					"DatasetName": "del-ds", "DatasetType": "TARGET_TIME_SERIES", "Domain": "RETAIL",
					"Schema": map[string]any{"Attributes": []any{}},
				}
			},
			arnField:  "DatasetArn",
			describe:  "DescribeDataset",
			deleteOp:  "DeleteDataset",
			list:      "ListDatasets",
			listField: "Datasets",
		},
		{
			name:   "predictor",
			create: "CreatePredictor",
			createBody: func(t *testing.T, h *forecast.Handler) map[string]any {
				t.Helper()

				return map[string]any{
					"PredictorName": "del-pred", "ForecastHorizon": 5,
					"InputDataConfig":     map[string]any{"DatasetGroupArn": createDatasetGroup(t, h)},
					"FeaturizationConfig": map[string]any{},
				}
			},
			arnField:  "PredictorArn",
			describe:  "DescribePredictor",
			deleteOp:  "DeletePredictor",
			list:      "ListPredictors",
			listField: "Predictors",
		},
		{
			name:   "forecast",
			create: "CreateForecast",
			createBody: func(t *testing.T, h *forecast.Handler) map[string]any {
				t.Helper()

				return map[string]any{"ForecastName": "del-fc", "PredictorArn": createPredictor(t, h)}
			},
			arnField:  "ForecastArn",
			describe:  "DescribeForecast",
			deleteOp:  "DeleteForecast",
			list:      "ListForecasts",
			listField: "Forecasts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			code, created := request(t, h, tt.create, tt.createBody(t, h))
			require.Equal(t, http.StatusOK, code)

			resARN, ok := created[tt.arnField].(string)
			require.True(t, ok)

			// Advance to ACTIVE so delete is valid.
			request(t, h, tt.describe, map[string]any{tt.arnField: resARN})
			request(t, h, tt.describe, map[string]any{tt.arnField: resARN})

			code, _ = request(t, h, tt.deleteOp, map[string]any{tt.arnField: resARN})
			require.Equal(t, http.StatusOK, code)

			// Describe must return 400 ResourceNotFoundException — not 200 with DELETING.
			code, resp := request(t, h, tt.describe, map[string]any{tt.arnField: resARN})
			assert.Equal(t, http.StatusBadRequest, code)
			assert.Equal(t, "ResourceNotFoundException", resp["__type"])

			// List must return empty.
			_, listResp := request(t, h, tt.list, map[string]any{})
			items, _ := listResp[tt.listField].([]any)
			assert.Empty(t, items, "deleted resource must not appear in list")
		})
	}
}

// TestDelete_IdempotencyFails verifies that deleting the same resource
// twice returns ResourceNotFoundException on the second call.
func TestDelete_IdempotencyFails(t *testing.T) {
	t.Parallel()

	h := newHandler()
	code, created := request(t, h, "CreateDatasetGroup", map[string]any{
		"DatasetGroupName": "idem-dg", "Domain": "RETAIL",
	})
	require.Equal(t, http.StatusOK, code)
	arn := created["DatasetGroupArn"].(string)

	// Advance past CREATE_PENDING: Delete* now requires a deletable status
	// (ACTIVE/CREATE_FAILED/...), matching real Amazon Forecast.
	request(t, h, "DescribeDatasetGroup", map[string]any{"DatasetGroupArn": arn})

	code, _ = request(t, h, "DeleteDatasetGroup", map[string]any{"DatasetGroupArn": arn})
	require.Equal(t, http.StatusOK, code)

	code, resp := request(t, h, "DeleteDatasetGroup", map[string]any{"DatasetGroupArn": arn})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])
}

// TestDeleteResourceTree_TransitiveDelete verifies that DeleteResourceTree
// removes the root resource AND all dependent children, mirroring AWS
// Forecast behavior.
func TestDeleteResourceTree_TransitiveDelete(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Create predictor (root).
	_, cp := request(t, h, "CreatePredictor", map[string]any{
		"PredictorName": "tree-pred", "ForecastHorizon": 5,
		"InputDataConfig":     map[string]any{"DatasetGroupArn": createDatasetGroup(t, h)},
		"FeaturizationConfig": map[string]any{},
	})
	predARN := cp["PredictorArn"].(string)

	// Create forecast referencing predictor.
	_, cf := request(t, h, "CreateForecast", map[string]any{
		"ForecastName": "tree-fc", "PredictorArn": predARN,
	})
	fcARN := cf["ForecastArn"].(string)

	// DeleteResourceTree on predictor → must delete predictor AND forecast.
	code, _ := request(t, h, "DeleteResourceTree", map[string]any{"ResourceArn": predARN})
	require.Equal(t, http.StatusOK, code)

	// Predictor gone.
	code, resp := request(t, h, "DescribePredictor", map[string]any{"PredictorArn": predARN})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])

	// Forecast (child) also gone.
	code, resp = request(t, h, "DescribeForecast", map[string]any{"ForecastArn": fcARN})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])
}

// TestDeleteResourceTree_DatasetGroupCascadesToPredictor verifies that
// DeleteResourceTree on a DatasetGroup also removes a Predictor that
// references it, per the api_op_DeleteResourceTree.go doc's "Dataset Group:
// predictors, ..." hierarchy. The reference lives nested under
// CreatePredictorInput.InputDataConfig.DatasetGroupArn, not top-level, so
// this exercises the recursive scan rather than the top-level-only case
// TestDeleteResourceTree_TransitiveDelete already covers via Forecast's
// top-level PredictorArn.
func TestDeleteResourceTree_DatasetGroupCascadesToPredictor(t *testing.T) {
	t.Parallel()

	h := newHandler()
	dgARN := createDatasetGroup(t, h)

	_, cp := request(t, h, "CreatePredictor", map[string]any{
		"PredictorName": "casc-pred", "ForecastHorizon": 5,
		"InputDataConfig":     map[string]any{"DatasetGroupArn": dgARN},
		"FeaturizationConfig": map[string]any{},
	})
	predARN := cp["PredictorArn"].(string)

	code, _ := request(t, h, "DeleteResourceTree", map[string]any{"ResourceArn": dgARN})
	require.Equal(t, http.StatusOK, code)

	code, resp := request(t, h, "DescribePredictor", map[string]any{"PredictorArn": predARN})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])
}

// TestDeleteResourceTree_NotFound verifies that DeleteResourceTree
// returns ResourceNotFoundException when the target ARN does not exist.
func TestDeleteResourceTree_NotFound(t *testing.T) {
	t.Parallel()

	h := newHandler()
	code, resp := request(t, h, "DeleteResourceTree", map[string]any{
		"ResourceArn": "arn:aws:forecast:us-east-1:000000000000:forecast/nonexistent",
	})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])
}

// TestUpdateResourceStatus_ARNIndex verifies that StopResource and
// ResumeResource use the O(1) ARN index and correctly update resource status.
func TestUpdateResourceStatus_ARNIndex(t *testing.T) {
	t.Parallel()

	h := newHandler()
	_, cp := request(t, h, "CreatePredictor", map[string]any{
		"PredictorName": "arn-pred", "ForecastHorizon": 5,
		"InputDataConfig":     map[string]any{"DatasetGroupArn": createDatasetGroup(t, h)},
		"FeaturizationConfig": map[string]any{},
	})
	predARN := cp["PredictorArn"].(string)

	// Advance past CREATE_PENDING.
	request(t, h, "DescribePredictor", map[string]any{"PredictorArn": predARN})
	request(t, h, "DescribePredictor", map[string]any{"PredictorArn": predARN})

	// Stop.
	code, _ := request(t, h, "StopResource", map[string]any{"ResourceArn": predARN})
	require.Equal(t, http.StatusOK, code)

	_, desc := request(t, h, "DescribePredictor", map[string]any{"PredictorArn": predARN})
	assert.Equal(t, "STOPPED", desc["Status"])

	// Resume.
	code, _ = request(t, h, "ResumeResource", map[string]any{"ResourceArn": predARN})
	require.Equal(t, http.StatusOK, code)

	_, desc = request(t, h, "DescribePredictor", map[string]any{"PredictorArn": predARN})
	assert.Equal(t, "ACTIVE", desc["Status"])
}

// TestUpdateResourceStatus_NotFound verifies that StopResource on a
// missing ARN returns ResourceNotFoundException.
func TestUpdateResourceStatus_NotFound(t *testing.T) {
	t.Parallel()

	h := newHandler()
	code, resp := request(t, h, "StopResource", map[string]any{
		"ResourceArn": "arn:aws:forecast:us-east-1:000000000000:predictor/ghost",
	})
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])
}

// TestARNIndex_RebuildAfterReset verifies that creating resources after
// Reset works correctly — the ARN index must be cleared on Reset so stale
// entries don't cause incorrect lookups.
func TestARNIndex_RebuildAfterReset(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Create + delete a resource.
	_, c1 := request(t, h, "CreateDatasetGroup", map[string]any{
		"DatasetGroupName": "pre-reset-dg", "Domain": "RETAIL",
	})
	arn1 := c1["DatasetGroupArn"].(string)
	request(t, h, "DescribeDatasetGroup", map[string]any{"DatasetGroupArn": arn1}) // advance past CREATE_PENDING
	request(t, h, "DeleteDatasetGroup", map[string]any{"DatasetGroupArn": arn1})

	// Reset backend.
	h.Reset()

	// Create a new resource with the same name — must succeed (no stale conflict).
	code, c2 := request(t, h, "CreateDatasetGroup", map[string]any{
		"DatasetGroupName": "pre-reset-dg", "Domain": "CUSTOM",
	})
	require.Equal(t, http.StatusOK, code)
	arn2 := c2["DatasetGroupArn"].(string)
	assert.NotEmpty(t, arn2)

	// ARN lookup must work (StopResource is cross-kind lookup via index).
	// Use a predictor for stop/resume since dataset groups don't support it,
	// but verify the new resource is listable to confirm index integrity.
	_, listResp := request(t, h, "ListDatasetGroups", map[string]any{})
	items := listResp["DatasetGroups"].([]any)
	require.Len(t, items, 1)
}
