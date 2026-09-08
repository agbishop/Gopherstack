package rekognition_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rekognition"
)

func TestProjectVersions(t *testing.T) { //nolint:paralleltest // existing issue.
	tests := []struct {
		body     any
		setup    func(h *rekognition.Handler) string // returns projectARN
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "CreateProjectVersion returns ARN",
			action: "CreateProjectVersion",
			setup: func(h *rekognition.Handler) string {
				rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "ver-proj"})
				require.Equal(t, http.StatusOK, rec.Code)
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				return resp["ProjectArn"].(string)
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Contains(t, resp["ProjectVersionArn"], "arn:aws:rekognition:")
			},
		},
		{
			name:     "CreateProjectVersion missing ProjectArn returns error",
			action:   "CreateProjectVersion",
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "CreateProjectVersion missing OutputConfig returns error",
			action: "CreateProjectVersion",
			setup: func(h *rekognition.Handler) string {
				rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "no-output-config-proj"})
				require.Equal(t, http.StatusOK, rec.Code)
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				return resp["ProjectArn"].(string)
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			var body any

			if tc.setup != nil {
				arn := tc.setup(h)
				body = map[string]any{
					"ProjectArn":  arn,
					"VersionName": "v1",
				}
				if tc.wantCode == http.StatusOK {
					body.(map[string]any)["OutputConfig"] = map[string]any{"S3Bucket": "my-bucket"}
				}
			}

			rec := doRequest(t, h, tc.action, body)
			assert.Equal(t, tc.wantCode, rec.Code, tc.name)

			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestProjectVersion_Lifecycle(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	// Create project
	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "life-proj"})
	require.Equal(t, http.StatusOK, rec.Code)

	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projectARN := projResp["ProjectArn"].(string)

	// Create version
	rec = doRequest(t, h, "CreateProjectVersion", map[string]any{
		"ProjectArn":   projectARN,
		"VersionName":  "v1",
		"OutputConfig": map[string]any{"S3Bucket": "my-bucket"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var verResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &verResp))
	versionARN := verResp["ProjectVersionArn"].(string)

	// DescribeProjectVersions
	rec = doRequest(t, h, "DescribeProjectVersions", map[string]any{"ProjectArn": projectARN})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	versions := descResp["ProjectVersionDescriptions"].([]any)
	assert.Len(t, versions, 1)

	// StartProjectVersion
	rec = doRequest(t, h, "StartProjectVersion", map[string]any{
		"ProjectVersionArn": versionARN,
		"MinInferenceUnits": 1,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var startResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
	assert.Equal(t, "RUNNING", startResp["Status"])

	// StopProjectVersion
	rec = doRequest(t, h, "StopProjectVersion", map[string]any{
		"ProjectVersionArn": versionARN,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var stopResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stopResp))
	assert.Equal(t, "STOPPED", stopResp["Status"])

	// DeleteProjectVersion
	rec = doRequest(t, h, "DeleteProjectVersion", map[string]any{
		"ProjectVersionArn": versionARN,
	})
	require.Equal(t, http.StatusOK, rec.Code)
}

// DeleteProjectVersion rejects a version that is training or running with
// ResourceInUseException -- DeleteProjectVersionInput's own doc comment
// (api_op_DeleteProjectVersion.go): "You can't delete a project version if
// it is running or if it is training.".
func TestProjectVersion_DeleteWhileTrainingOrRunning_Rejected(t *testing.T) { //nolint:paralleltest // stateful
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "in-use-proj"})
	require.Equal(t, http.StatusOK, rec.Code)

	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projectARN := projResp["ProjectArn"].(string)

	rec = doRequest(t, h, "CreateProjectVersion", map[string]any{
		"ProjectArn":   projectARN,
		"VersionName":  "v1",
		"OutputConfig": map[string]any{"S3Bucket": "my-bucket"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var verResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &verResp))
	versionARN := verResp["ProjectVersionArn"].(string)

	// Freshly created -> TRAINING_IN_PROGRESS -> delete rejected.
	rec = doRequest(t, h, "DeleteProjectVersion", map[string]any{"ProjectVersionArn": versionARN})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ResourceInUseException", errResp["__type"])

	// RUNNING -> delete rejected.
	rec = doRequest(t, h, "StartProjectVersion", map[string]any{
		"ProjectVersionArn": versionARN,
		"MinInferenceUnits": 1,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DeleteProjectVersion", map[string]any{"ProjectVersionArn": versionARN})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ResourceInUseException", errResp["__type"])

	// STOPPED -> delete allowed.
	rec = doRequest(t, h, "StopProjectVersion", map[string]any{"ProjectVersionArn": versionARN})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DeleteProjectVersion", map[string]any{"ProjectVersionArn": versionARN})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---------------------------------------------------------------------------
// CreateProjectVersion persists and echoes OutputConfig/KmsKeyId/
// VersionDescription/Tags back through DescribeProjectVersions and
// ListTagsForResource. These were previously accepted by the request but
// silently dropped -- see PARITY.md gaps.
// ---------------------------------------------------------------------------

func TestCreateProjectVersion_FullFieldsRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "full-ver-proj"})
	require.Equal(t, http.StatusOK, rec.Code)

	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projectARN := projResp["ProjectArn"].(string)

	rec = doRequest(t, h, "CreateProjectVersion", map[string]any{
		"ProjectArn":  projectARN,
		"VersionName": "v-full",
		"OutputConfig": map[string]any{
			"S3Bucket":    "my-output-bucket",
			"S3KeyPrefix": "training-output/",
		},
		"KmsKeyId":           "arn:aws:kms:us-east-1:000000000000:key/abc",
		"VersionDescription": "a test model version",
		"Tags":               map[string]any{"team": "vision"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var verResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &verResp))
	versionARN := verResp["ProjectVersionArn"].(string)

	rec = doRequest(t, h, "DescribeProjectVersions", map[string]any{"ProjectArn": projectARN})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	versions := descResp["ProjectVersionDescriptions"].([]any)
	require.Len(t, versions, 1)

	desc := versions[0].(map[string]any)
	assert.Equal(t, "arn:aws:kms:us-east-1:000000000000:key/abc", desc["KmsKeyId"])
	assert.Equal(t, "a test model version", desc["VersionDescription"])

	outputConfig, _ := desc["OutputConfig"].(map[string]any)
	require.NotNil(t, outputConfig)
	assert.Equal(t, "my-output-bucket", outputConfig["S3Bucket"])
	assert.Equal(t, "training-output/", outputConfig["S3KeyPrefix"])

	// ProjectVersion ARNs are taggable (see PARITY.md Notes #3) -- confirm
	// the initial Tags made it into the tag store.
	rec = doRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": versionARN})
	require.Equal(t, http.StatusOK, rec.Code)

	var tagsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tagsResp))
	tags, _ := tagsResp["Tags"].(map[string]any)
	assert.Equal(t, "vision", tags["team"])
}

func TestCopyProjectVersion(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	// Create source project + version
	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "src-proj"})
	require.Equal(t, http.StatusOK, rec.Code)

	var srcProjResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &srcProjResp))
	srcProjectARN := srcProjResp["ProjectArn"].(string)

	rec = doRequest(t, h, "CreateProjectVersion", map[string]any{
		"ProjectArn":   srcProjectARN,
		"VersionName":  "v1",
		"OutputConfig": map[string]any{"S3Bucket": "my-bucket"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var srcVerResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &srcVerResp))
	sourceVersionARN := srcVerResp["ProjectVersionArn"].(string)

	// Create destination project
	rec = doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "dst-proj"})
	require.Equal(t, http.StatusOK, rec.Code)

	var dstProjResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dstProjResp))
	dstProjectARN := dstProjResp["ProjectArn"].(string)

	// Copy version fails when SourceProjectArn doesn't match the project
	// that actually owns SourceProjectVersionArn.
	rec = doRequest(t, h, "CopyProjectVersion", map[string]any{
		"SourceProjectArn":        dstProjectARN,
		"SourceProjectVersionArn": sourceVersionARN,
		"DestinationProjectArn":   dstProjectARN,
		"VersionName":             "v1-copy",
		"OutputConfig":            map[string]any{"S3Bucket": "copy-bucket"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Copy version
	rec = doRequest(t, h, "CopyProjectVersion", map[string]any{
		"SourceProjectArn":        srcProjectARN,
		"SourceProjectVersionArn": sourceVersionARN,
		"DestinationProjectArn":   dstProjectARN,
		"VersionName":             "v1-copy",
		"OutputConfig":            map[string]any{"S3Bucket": "copy-bucket", "S3KeyPrefix": "copy-prefix"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var copyResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &copyResp))
	assert.Contains(t, copyResp["ProjectVersionArn"], "dst-proj")

	// SourceProjectVersionArn and OutputConfig are echoed back on the
	// destination version.
	rec = doRequest(t, h, "DescribeProjectVersions", map[string]any{"ProjectArn": dstProjectARN})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	versions := descResp["ProjectVersionDescriptions"].([]any)
	require.Len(t, versions, 1)
	copied := versions[0].(map[string]any)
	assert.Equal(t, sourceVersionARN, copied["SourceProjectVersionArn"])
	outputConfig, ok := copied["OutputConfig"].(map[string]any)
	require.True(t, ok, "OutputConfig must be echoed back on the copied version")
	assert.Equal(t, "copy-bucket", outputConfig["S3Bucket"])
	assert.Equal(t, "copy-prefix", outputConfig["S3KeyPrefix"])
}

func TestCopyProjectVersion_MissingRequiredFields_ReturnsError(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]any{
		"missing source project arn": {
			"SourceProjectVersionArn": "arn:aws:rekognition:us-east-1:000000000000:project/p/version/v/1",
			"DestinationProjectArn":   "arn:aws:rekognition:us-east-1:000000000000:project/d",
			"VersionName":             "v1",
			"OutputConfig":            map[string]any{"S3Bucket": "b"},
		},
		"missing source project version arn": {
			"SourceProjectArn":      "arn:aws:rekognition:us-east-1:000000000000:project/p",
			"DestinationProjectArn": "arn:aws:rekognition:us-east-1:000000000000:project/d",
			"VersionName":           "v1",
			"OutputConfig":          map[string]any{"S3Bucket": "b"},
		},
		"missing destination project arn": {
			"SourceProjectArn":        "arn:aws:rekognition:us-east-1:000000000000:project/p",
			"SourceProjectVersionArn": "arn:aws:rekognition:us-east-1:000000000000:project/p/version/v/1",
			"VersionName":             "v1",
			"OutputConfig":            map[string]any{"S3Bucket": "b"},
		},
		"missing version name": {
			"SourceProjectArn":        "arn:aws:rekognition:us-east-1:000000000000:project/p",
			"SourceProjectVersionArn": "arn:aws:rekognition:us-east-1:000000000000:project/p/version/v/1",
			"DestinationProjectArn":   "arn:aws:rekognition:us-east-1:000000000000:project/d",
			"OutputConfig":            map[string]any{"S3Bucket": "b"},
		},
		"missing output config": {
			"SourceProjectArn":        "arn:aws:rekognition:us-east-1:000000000000:project/p",
			"SourceProjectVersionArn": "arn:aws:rekognition:us-east-1:000000000000:project/p/version/v/1",
			"DestinationProjectArn":   "arn:aws:rekognition:us-east-1:000000000000:project/d",
			"VersionName":             "v1",
		},
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CopyProjectVersion", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// ---------------------------------------------------------------------------
// FeatureConfig.ContentModeration.ConfidenceThreshold round-trips through
// DescribeProjectVersions -- a shallow (2-level, no unions) shape, unlike
// TrainingData/TestingData which stay opaque (see PARITY.md deferred).
// ---------------------------------------------------------------------------

func TestCreateProjectVersion_FeatureConfigRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "feature-config-proj"})
	require.Equal(t, http.StatusOK, rec.Code)

	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projectARN := projResp["ProjectArn"].(string)

	rec = doRequest(t, h, "CreateProjectVersion", map[string]any{
		"ProjectArn":   projectARN,
		"VersionName":  "v1",
		"OutputConfig": map[string]any{"S3Bucket": "my-bucket"},
		"FeatureConfig": map[string]any{
			"ContentModeration": map[string]any{"ConfidenceThreshold": 75.5},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DescribeProjectVersions", map[string]any{"ProjectArn": projectARN})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	versions := descResp["ProjectVersionDescriptions"].([]any)
	require.Len(t, versions, 1)

	featureConfig, _ := versions[0].(map[string]any)["FeatureConfig"].(map[string]any)
	require.NotNil(t, featureConfig)
	contentMod, _ := featureConfig["ContentModeration"].(map[string]any)
	require.NotNil(t, contentMod)
	assert.InDelta(t, 75.5, contentMod["ConfidenceThreshold"], 0.001)
}

// ---------------------------------------------------------------------------
// "If you specify TrainingData you must also specify TestingData"
// (api_op_CreateProjectVersion.go) -- gopherstack previously accepted either
// one alone, more permissive than the real API.
// ---------------------------------------------------------------------------

func TestCreateProjectVersion_TrainingTestingDataMustBothBeSpecified(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "training data only rejected",
			body: map[string]any{
				"TrainingData": map[string]any{"Assets": []any{}},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "testing data only rejected",
			body: map[string]any{
				"TestingData": map[string]any{"Assets": []any{}},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "both specified accepted",
			body: map[string]any{
				"TrainingData": map[string]any{"Assets": []any{}},
				"TestingData":  map[string]any{"Assets": []any{}},
			},
			wantCode: http.StatusOK,
		},
		{
			name:     "neither specified accepted",
			body:     map[string]any{},
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "cross-validate-proj-" + tc.name})
			require.Equal(t, http.StatusOK, rec.Code)

			var projResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
			projectARN := projResp["ProjectArn"].(string)

			body := map[string]any{
				"ProjectArn":   projectARN,
				"VersionName":  "v1",
				"OutputConfig": map[string]any{"S3Bucket": "my-bucket"},
			}
			maps.Copy(body, tc.body)

			rec = doRequest(t, h, "CreateProjectVersion", body)
			assert.Equal(t, tc.wantCode, rec.Code, tc.name)
		})
	}
}

// ---------------------------------------------------------------------------
// StartProjectVersion's optional MaxInferenceUnits round-trips through
// DescribeProjectVersions -- previously accepted (per StartProjectVersionInput,
// validators.go) but never stored.
// ---------------------------------------------------------------------------

func TestStartProjectVersion_MaxInferenceUnitsRoundTrip(t *testing.T) { //nolint:paralleltest // stateful sequential
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "max-inference-proj"})
	require.Equal(t, http.StatusOK, rec.Code)

	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projectARN := projResp["ProjectArn"].(string)

	rec = doRequest(t, h, "CreateProjectVersion", map[string]any{
		"ProjectArn":   projectARN,
		"VersionName":  "v1",
		"OutputConfig": map[string]any{"S3Bucket": "my-bucket"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var verResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &verResp))
	versionARN := verResp["ProjectVersionArn"].(string)

	rec = doRequest(t, h, "StartProjectVersion", map[string]any{
		"ProjectVersionArn": versionARN,
		"MinInferenceUnits": 1,
		"MaxInferenceUnits": 5,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DescribeProjectVersions", map[string]any{"ProjectArn": projectARN})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	versions := descResp["ProjectVersionDescriptions"].([]any)
	require.Len(t, versions, 1)
	desc := versions[0].(map[string]any)
	assert.InDelta(t, 1, desc["MinInferenceUnits"], 0.001)
	assert.InDelta(t, 5, desc["MaxInferenceUnits"], 0.001)
}
