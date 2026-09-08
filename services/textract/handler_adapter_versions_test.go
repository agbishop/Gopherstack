package textract_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/textract"
)

func TestHandler_AdapterVersionLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantStatus int
	}{
		{name: "full adapter version lifecycle", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// First create an adapter
			createAdapterBody := map[string]any{
				"AdapterName":  "version-test-adapter",
				"FeatureTypes": []string{"QUERIES"},
			}
			createAdapterRec := doTextractRequest(t, h, "CreateAdapter", createAdapterBody)
			require.Equal(t, tt.wantStatus, createAdapterRec.Code)

			var createAdapterResp map[string]string
			require.NoError(t, json.Unmarshal(createAdapterRec.Body.Bytes(), &createAdapterResp))
			adapterID := createAdapterResp["AdapterId"]

			// CreateAdapterVersion
			createVersionBody := map[string]any{
				"AdapterId": adapterID,
				"Tags":      map[string]string{"env": "test"},
				"DatasetConfig": map[string]any{
					"ManifestS3Object": map[string]any{
						"Bucket": "test-dataset-bucket",
						"Name":   "manifest.json",
					},
				},
				"OutputConfig": map[string]any{
					"S3Bucket": "test-output-bucket",
				},
			}
			createVersionRec := doTextractRequest(t, h, "CreateAdapterVersion", createVersionBody)
			assert.Equal(t, http.StatusOK, createVersionRec.Code)

			var createVersionResp map[string]string
			require.NoError(t, json.Unmarshal(createVersionRec.Body.Bytes(), &createVersionResp))
			adapterVersion := createVersionResp["AdapterVersion"]
			assert.NotEmpty(t, adapterVersion)
			assert.Equal(t, adapterID, createVersionResp["AdapterId"])

			// GetAdapterVersion
			getVersionBody := map[string]string{
				"AdapterId":      adapterID,
				"AdapterVersion": adapterVersion,
			}
			getVersionRec := doTextractRequest(t, h, "GetAdapterVersion", getVersionBody)
			assert.Equal(t, http.StatusOK, getVersionRec.Code)

			var getVersionResp map[string]any
			require.NoError(t, json.Unmarshal(getVersionRec.Body.Bytes(), &getVersionResp))
			assert.Equal(t, adapterID, getVersionResp["AdapterId"])
			assert.Equal(t, adapterVersion, getVersionResp["AdapterVersion"])
			assert.Equal(t, "ACTIVE", getVersionResp["Status"])

			// DeleteAdapterVersion
			deleteVersionRec := doTextractRequest(t, h, "DeleteAdapterVersion", getVersionBody)
			assert.Equal(t, http.StatusOK, deleteVersionRec.Code)

			// GetAdapterVersion after delete returns error
			getVersionRec2 := doTextractRequest(t, h, "GetAdapterVersion", getVersionBody)
			assert.Equal(t, http.StatusBadRequest, getVersionRec2.Code)
		})
	}
}

// TestHandler_CreateAdapterVersion_RequiresDatasetAndOutputConfig verifies
// that DatasetConfig and OutputConfig.S3Bucket are enforced as required:
// real AWS's CreateAdapterVersionInput marks both "This member is required"
// (api_op_CreateAdapterVersion.go), mirrored by the client-side
// validateOpCreateAdapterVersionInput/validateOutputConfig in validators.go.
func TestHandler_CreateAdapterVersion_RequiresDatasetAndOutputConfig(t *testing.T) {
	t.Parallel()

	validDatasetConfig := map[string]any{
		"ManifestS3Object": map[string]any{
			"Bucket": "test-dataset-bucket",
			"Name":   "manifest.json",
		},
	}
	validOutputConfig := map[string]any{"S3Bucket": "test-output-bucket"}

	tests := []struct {
		datasetConfig map[string]any
		outputConfig  map[string]any
		name          string
		wantStatus    int
	}{
		{
			name:       "missing both is rejected",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:          "missing OutputConfig is rejected",
			datasetConfig: validDatasetConfig,
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:         "missing DatasetConfig is rejected",
			outputConfig: validOutputConfig,
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:          "OutputConfig without S3Bucket is rejected",
			datasetConfig: validDatasetConfig,
			outputConfig:  map[string]any{},
			wantStatus:    http.StatusBadRequest,
		},
		{
			name:          "both present succeeds",
			datasetConfig: validDatasetConfig,
			outputConfig:  validOutputConfig,
			wantStatus:    http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			createAdapterRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
				"AdapterName":  "required-config-adapter",
				"FeatureTypes": []string{"FORMS"},
			})
			require.Equal(t, http.StatusOK, createAdapterRec.Code)

			var createAdapterResp map[string]string
			require.NoError(t, json.Unmarshal(createAdapterRec.Body.Bytes(), &createAdapterResp))

			body := map[string]any{"AdapterId": createAdapterResp["AdapterId"]}
			if tt.datasetConfig != nil {
				body["DatasetConfig"] = tt.datasetConfig
			}

			if tt.outputConfig != nil {
				body["OutputConfig"] = tt.outputConfig
			}

			rec := doTextractRequest(t, h, "CreateAdapterVersion", body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_CreateAdapterVersion_MissingAdapterId(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	body := map[string]any{"AdapterId": ""}
	rec := doTextractRequest(t, h, "CreateAdapterVersion", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_CreateAdapterVersion_AdapterNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	body := map[string]any{"AdapterId": "nonexistent-adapter"}
	rec := doTextractRequest(t, h, "CreateAdapterVersion", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_GetAdapterVersion_MissingFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]string
		name string
	}{
		{
			name: "missing adapter id",
			body: map[string]string{"AdapterId": "", "AdapterVersion": "v1"},
		},
		{
			name: "missing adapter version",
			body: map[string]string{"AdapterId": "adapter-id", "AdapterVersion": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doTextractRequest(t, h, "GetAdapterVersion", tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// TestHandler_CreateAdapterVersion_DatasetConfig verifies GetAdapterVersion
// includes the DatasetConfig and KMSKeyId supplied at creation.
func TestHandler_CreateAdapterVersion_DatasetConfig(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createAdapterRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "dataset-adapter",
		"FeatureTypes": []string{"FORMS"},
	})
	require.Equal(t, http.StatusOK, createAdapterRec.Code)

	var createAdapterResp map[string]string
	require.NoError(t, json.Unmarshal(createAdapterRec.Body.Bytes(), &createAdapterResp))
	adapterID := createAdapterResp["AdapterId"]

	createVersionRec := doTextractRequest(t, h, "CreateAdapterVersion", map[string]any{
		"AdapterId": adapterID,
		"DatasetConfig": map[string]any{
			"ManifestS3Object": map[string]any{
				"Bucket": "my-dataset-bucket",
				"Name":   "manifest.json",
			},
		},
		"OutputConfig": map[string]any{
			"S3Bucket": "my-output-bucket",
		},
		"KMSKeyId": "arn:aws:kms:us-east-1:123456789012:key/abcd1234",
	})
	require.Equal(t, http.StatusOK, createVersionRec.Code)

	var createVersionResp map[string]string
	require.NoError(t, json.Unmarshal(createVersionRec.Body.Bytes(), &createVersionResp))
	adapterVersion := createVersionResp["AdapterVersion"]

	getVersionRec := doTextractRequest(t, h, "GetAdapterVersion", map[string]any{
		"AdapterId":      adapterID,
		"AdapterVersion": adapterVersion,
	})
	require.Equal(t, http.StatusOK, getVersionRec.Code)

	var getVersionResp map[string]any
	require.NoError(t, json.Unmarshal(getVersionRec.Body.Bytes(), &getVersionResp))

	datasetConfig, ok := getVersionResp["DatasetConfig"].(map[string]any)
	require.True(t, ok, "GetAdapterVersion should include DatasetConfig")
	manifest, ok2 := datasetConfig["ManifestS3Object"].(map[string]any)
	require.True(t, ok2)
	assert.Equal(t, "my-dataset-bucket", manifest["Bucket"])

	assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/abcd1234", getVersionResp["KMSKeyId"])
}

// TestHandler_AdapterVersion_InProgressThenActive verifies an adapter version
// starts CREATION_IN_PROGRESS then becomes ACTIVE after the async delay.
func TestHandler_AdapterVersion_InProgressThenActive(t *testing.T) {
	t.Parallel()

	// Use a backend with a short async delay.
	b := textract.NewInMemoryBackendSync("123456789012", "us-east-1")
	textract.SetBackendAsyncDelay(b, 50*time.Millisecond)
	h := textract.NewHandler(b)

	createAdapterRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "lifecycle-adapter",
		"FeatureTypes": []string{"FORMS"},
	})
	require.Equal(t, http.StatusOK, createAdapterRec.Code)

	var createAdapterResp map[string]string
	require.NoError(t, json.Unmarshal(createAdapterRec.Body.Bytes(), &createAdapterResp))
	adapterID := createAdapterResp["AdapterId"]

	createVersionRec := doTextractRequest(t, h, "CreateAdapterVersion", map[string]any{
		"AdapterId": adapterID,
		"DatasetConfig": map[string]any{
			"ManifestS3Object": map[string]any{
				"Bucket": "test-dataset-bucket",
				"Name":   "manifest.json",
			},
		},
		"OutputConfig": map[string]any{
			"S3Bucket": "test-output-bucket",
		},
	})
	require.Equal(t, http.StatusOK, createVersionRec.Code)

	var createVersionResp map[string]string
	require.NoError(t, json.Unmarshal(createVersionRec.Body.Bytes(), &createVersionResp))
	adapterVersion := createVersionResp["AdapterVersion"]

	// Immediately check: should be CREATION_IN_PROGRESS.
	getRec1 := doTextractRequest(t, h, "GetAdapterVersion", map[string]any{
		"AdapterId":      adapterID,
		"AdapterVersion": adapterVersion,
	})
	require.Equal(t, http.StatusOK, getRec1.Code)

	var getResp1 map[string]any
	require.NoError(t, json.Unmarshal(getRec1.Body.Bytes(), &getResp1))
	assert.Equal(t, "CREATION_IN_PROGRESS", getResp1["Status"])

	// After delay, should be ACTIVE.
	time.Sleep(200 * time.Millisecond)

	getRec2 := doTextractRequest(t, h, "GetAdapterVersion", map[string]any{
		"AdapterId":      adapterID,
		"AdapterVersion": adapterVersion,
	})
	require.Equal(t, http.StatusOK, getRec2.Code)

	var getResp2 map[string]any
	require.NoError(t, json.Unmarshal(getRec2.Body.Bytes(), &getResp2))
	assert.Equal(t, "ACTIVE", getResp2["Status"])
}

// TestHandler_AdapterVersion_EvaluationMetrics verifies GetAdapterVersion
// returns the deterministic EvaluationMetrics as a list of
// AdapterVersionEvaluationMetric entries (one per FeatureType), each with
// Baseline and AdapterVersion sub-scores -- matching the real SDK's
// GetAdapterVersionOutput.EvaluationMetrics shape, not a single flat struct.
func TestHandler_AdapterVersion_EvaluationMetrics(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createAdapterRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "metrics-adapter",
		"FeatureTypes": []string{"QUERIES"},
	})
	require.Equal(t, http.StatusOK, createAdapterRec.Code)

	var createAdapterResp map[string]string
	require.NoError(t, json.Unmarshal(createAdapterRec.Body.Bytes(), &createAdapterResp))
	adapterID := createAdapterResp["AdapterId"]

	createVersionRec := doTextractRequest(t, h, "CreateAdapterVersion", map[string]any{
		"AdapterId": adapterID,
		"DatasetConfig": map[string]any{
			"ManifestS3Object": map[string]any{
				"Bucket": "test-dataset-bucket",
				"Name":   "manifest.json",
			},
		},
		"OutputConfig": map[string]any{
			"S3Bucket": "test-output-bucket",
		},
	})
	require.Equal(t, http.StatusOK, createVersionRec.Code)

	var createVersionResp map[string]string
	require.NoError(t, json.Unmarshal(createVersionRec.Body.Bytes(), &createVersionResp))
	adapterVersion := createVersionResp["AdapterVersion"]

	getVersionRec := doTextractRequest(t, h, "GetAdapterVersion", map[string]any{
		"AdapterId":      adapterID,
		"AdapterVersion": adapterVersion,
	})
	require.Equal(t, http.StatusOK, getVersionRec.Code)

	var getVersionResp map[string]any
	require.NoError(t, json.Unmarshal(getVersionRec.Body.Bytes(), &getVersionResp))

	evalMetricsList, ok := getVersionResp["EvaluationMetrics"].([]any)
	require.True(t, ok, "GetAdapterVersion should include EvaluationMetrics as a list")
	require.Len(t, evalMetricsList, 1, "one entry per adapter FeatureType (QUERIES)")

	entry, ok := evalMetricsList[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "QUERIES", entry["FeatureType"])

	adapterVersionScore, ok := entry["AdapterVersion"].(map[string]any)
	require.True(t, ok, "entry should include AdapterVersion sub-scores")
	assert.InDelta(t, 0.85, adapterVersionScore["F1Score"], 0.001)
	assert.InDelta(t, 0.88, adapterVersionScore["Precision"], 0.001)
	assert.InDelta(t, 0.82, adapterVersionScore["Recall"], 0.001)

	baseline, ok := entry["Baseline"].(map[string]any)
	require.True(t, ok, "entry should include Baseline sub-scores")
	assert.InDelta(t, 0.80, baseline["F1Score"], 0.001)
	assert.InDelta(t, 0.83, baseline["Precision"], 0.001)
	assert.InDelta(t, 0.78, baseline["Recall"], 0.001)
}

// TestHandler_DeleteAdapterVersion_GetReturnsErrorAfterDelete verifies that
// after DeleteAdapterVersion, calling GetAdapterVersion on the deleted version
// returns ResourceNotFoundException (400).
func TestHandler_DeleteAdapterVersion_GetReturnsErrorAfterDelete(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createAdapterRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "versioned",
		"FeatureTypes": []string{"FORMS"},
	})
	require.Equal(t, http.StatusOK, createAdapterRec.Code)
	var adapterResp map[string]string
	require.NoError(t, json.Unmarshal(createAdapterRec.Body.Bytes(), &adapterResp))
	adapterID := adapterResp["AdapterId"]

	createVerRec := doTextractRequest(t, h, "CreateAdapterVersion", map[string]any{
		"AdapterId": adapterID,
		"DatasetConfig": map[string]any{
			"ManifestS3Object": map[string]any{
				"Bucket": "test-dataset-bucket",
				"Name":   "manifest.json",
			},
		},
		"OutputConfig": map[string]any{
			"S3Bucket": "test-output-bucket",
		},
	})
	require.Equal(t, http.StatusOK, createVerRec.Code)
	var verResp map[string]string
	require.NoError(t, json.Unmarshal(createVerRec.Body.Bytes(), &verResp))
	version := verResp["AdapterVersion"]
	require.NotEmpty(t, version)

	getBeforeRec := doTextractRequest(t, h, "GetAdapterVersion", map[string]any{
		"AdapterId":      adapterID,
		"AdapterVersion": version,
	})
	require.Equal(t, http.StatusOK, getBeforeRec.Code, "version must exist before deletion")

	deleteRec := doTextractRequest(t, h, "DeleteAdapterVersion", map[string]any{
		"AdapterId":      adapterID,
		"AdapterVersion": version,
	})
	require.Equal(t, http.StatusOK, deleteRec.Code, "delete must succeed")

	getAfterRec := doTextractRequest(t, h, "GetAdapterVersion", map[string]any{
		"AdapterId":      adapterID,
		"AdapterVersion": version,
	})
	assert.Equal(t, http.StatusBadRequest, getAfterRec.Code)
	var errResp map[string]any
	require.NoError(t, json.Unmarshal(getAfterRec.Body.Bytes(), &errResp))
	assert.Equal(t, "ResourceNotFoundException", errResp["__type"],
		"deleted version must return ResourceNotFoundException")
}

// TestHandler_ListAdapterVersions_HappyPath tests listing versions for an adapter.
func TestHandler_ListAdapterVersions_HappyPath(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "versioned-adapter",
		"FeatureTypes": []string{"FORMS"},
	})
	var createResp map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	adapterID := createResp["AdapterId"]

	for range 3 {
		doTextractRequest(t, h, "CreateAdapterVersion", map[string]any{
			"AdapterId": adapterID,
			"DatasetConfig": map[string]any{
				"ManifestS3Object": map[string]any{
					"Bucket": "test-dataset-bucket",
					"Name":   "manifest.json",
				},
			},
			"OutputConfig": map[string]any{
				"S3Bucket": "test-output-bucket",
			},
		})
	}

	listRec := doTextractRequest(t, h, "ListAdapterVersions", map[string]any{
		"AdapterId": adapterID,
	})
	require.Equal(t, http.StatusOK, listRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &resp))
	versions, ok := resp["AdapterVersions"].([]any)
	assert.True(t, ok)
	assert.Len(t, versions, 3)

	// ListAdapterVersionsOutput has no top-level AdapterId in the real SDK --
	// only per-entry AdapterId inside each AdapterVersionOverview.
	_, hasTopLevelAdapterID := resp["AdapterId"]
	assert.False(t, hasTopLevelAdapterID, "ListAdapterVersions response must not have a top-level AdapterId")

	for _, v := range versions {
		vm, ok2 := v.(map[string]any)
		require.True(t, ok2)
		assert.Equal(t, adapterID, vm["AdapterId"], "each AdapterVersionOverview entry must carry its own AdapterId")
	}
}

// TestHandler_ListAdapterVersions_NotFound returns 400 for unknown adapter.
func TestHandler_ListAdapterVersions_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doTextractRequest(t, h, "ListAdapterVersions", map[string]any{
		"AdapterId": "nonexistent",
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_ListAdapterVersions_SummaryShape verifies that
// ListAdapterVersions returns FeatureTypes in each version summary and omits
// Tags: real AWS returns AdapterVersionOverview items with
// FeatureTypes/Status but without Tags.
func TestHandler_ListAdapterVersions_SummaryShape(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "version-shape-adapter",
		"FeatureTypes": []string{"QUERIES"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	adapterID := createResp["AdapterId"]

	verRec := doTextractRequest(t, h, "CreateAdapterVersion", map[string]any{
		"AdapterId": adapterID,
		"Tags":      map[string]string{"phase": "alpha"},
		"DatasetConfig": map[string]any{
			"ManifestS3Object": map[string]any{
				"Bucket": "test-dataset-bucket",
				"Name":   "manifest.json",
			},
		},
		"OutputConfig": map[string]any{
			"S3Bucket": "test-output-bucket",
		},
	})
	require.Equal(t, http.StatusOK, verRec.Code)

	listRec := doTextractRequest(t, h, "ListAdapterVersions", map[string]any{
		"AdapterId": adapterID,
	})
	require.Equal(t, http.StatusOK, listRec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &resp))

	versions, ok := resp["AdapterVersions"].([]any)
	require.True(t, ok)
	require.Len(t, versions, 1)

	vSummary, ok := versions[0].(map[string]any)
	require.True(t, ok)

	fts, hasFT := vSummary["FeatureTypes"].([]any)
	assert.True(t, hasFT, "ListAdapterVersions summary must include FeatureTypes")
	assert.Len(t, fts, 1)

	_, hasTags := vSummary["Tags"]
	assert.False(t, hasTags, "ListAdapterVersions summary must not include Tags")

	status, hasStatus := vSummary["Status"].(string)
	assert.True(t, hasStatus, "ListAdapterVersions summary must include Status")
	assert.NotEmpty(t, status)
}

// TestHandler_AdapterVersion_CreationTimeIsEpochSeconds locks in that
// CreationTime is serialized as a JSON number of epoch seconds (the
// awsjson1.1 unixTimestamp wire format the real SDK deserializer requires),
// not an RFC3339 string. GetAdapterVersion/ListAdapterVersions previously
// formatted CreationTime as "2006-01-02T15:04:05Z", which a real Textract
// SDK client would reject.
func TestHandler_AdapterVersion_CreationTimeIsEpochSeconds(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createAdapterRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "epoch-version-adapter",
		"FeatureTypes": []string{"FORMS"},
	})
	require.Equal(t, http.StatusOK, createAdapterRec.Code)

	var createAdapterResp map[string]string
	require.NoError(t, json.Unmarshal(createAdapterRec.Body.Bytes(), &createAdapterResp))
	adapterID := createAdapterResp["AdapterId"]

	createVersionRec := doTextractRequest(t, h, "CreateAdapterVersion", map[string]any{
		"AdapterId": adapterID,
		"DatasetConfig": map[string]any{
			"ManifestS3Object": map[string]any{
				"Bucket": "test-dataset-bucket",
				"Name":   "manifest.json",
			},
		},
		"OutputConfig": map[string]any{
			"S3Bucket": "test-output-bucket",
		},
	})
	require.Equal(t, http.StatusOK, createVersionRec.Code)

	var createVersionResp map[string]string
	require.NoError(t, json.Unmarshal(createVersionRec.Body.Bytes(), &createVersionResp))
	adapterVersion := createVersionResp["AdapterVersion"]

	getRec := doTextractRequest(t, h, "GetAdapterVersion", map[string]any{
		"AdapterId":      adapterID,
		"AdapterVersion": adapterVersion,
	})
	require.Equal(t, http.StatusOK, getRec.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	ct, ok := getResp["CreationTime"].(float64)
	assert.True(t, ok, "GetAdapterVersion CreationTime must be a JSON number")
	assert.Positive(t, ct)

	listRec := doTextractRequest(t, h, "ListAdapterVersions", map[string]any{"AdapterId": adapterID})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
	versions, ok := listResp["AdapterVersions"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, versions)

	summary, ok := versions[0].(map[string]any)
	require.True(t, ok)
	lct, ok := summary["CreationTime"].(float64)
	assert.True(t, ok, "ListAdapterVersions summary CreationTime must be a JSON number")
	assert.Positive(t, lct)
}

// TestHandler_ListAdapterVersions_Pagination verifies NextToken/MaxResults
// pagination. Real AWS's ListAdapterVersionsInput accepts
// MaxResults/NextToken and ListAdapterVersionsOutput echoes NextToken --
// this was previously accepted as AdapterId-only, dropping every other
// field silently.
func TestHandler_ListAdapterVersions_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "pagination-versions-adapter",
		"FeatureTypes": []string{"FORMS"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	adapterID := createResp["AdapterId"]

	for range 3 {
		rec := doTextractRequest(t, h, "CreateAdapterVersion", map[string]any{
			"AdapterId": adapterID,
			"DatasetConfig": map[string]any{
				"ManifestS3Object": map[string]any{
					"Bucket": "test-dataset-bucket",
					"Name":   "manifest.json",
				},
			},
			"OutputConfig": map[string]any{
				"S3Bucket": "test-output-bucket",
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec1 := doTextractRequest(t, h, "ListAdapterVersions", map[string]any{
		"AdapterId":  adapterID,
		"MaxResults": 2,
	})
	require.Equal(t, http.StatusOK, rec1.Code)

	var resp1 struct {
		NextToken       string           `json:"NextToken"`
		AdapterVersions []map[string]any `json:"AdapterVersions"`
	}
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &resp1))
	assert.Len(t, resp1.AdapterVersions, 2)
	require.NotEmpty(t, resp1.NextToken, "first page must return a NextToken")

	rec2 := doTextractRequest(t, h, "ListAdapterVersions", map[string]any{
		"AdapterId": adapterID,
		"NextToken": resp1.NextToken,
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp2 struct {
		NextToken       string           `json:"NextToken"`
		AdapterVersions []map[string]any `json:"AdapterVersions"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))
	assert.Len(t, resp2.AdapterVersions, 1, "remaining 1 version on the second page")
	assert.Empty(t, resp2.NextToken, "no more pages")
}
