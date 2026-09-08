package rekognition_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// extractLabels extracts the label list from a ListDatasetLabels response
// body under the real wire key, "DatasetLabelDescriptions" (deserializers.go's
// ListDatasetLabelsOutput switch -- confirmed no "DatasetLabelStats" case
// exists).
func extractLabels(t *testing.T, body []byte) []any {
	t.Helper()

	var resp map[string]any
	require.NoError(t, json.Unmarshal(body, &resp))

	v, _ := resp["DatasetLabelDescriptions"].([]any)

	return v
}

// =============================================================================
// ListDatasetLabels
// =============================================================================

func TestListDatasetLabels_SingleLabel(t *testing.T) { //nolint:paralleltest // stateful sequential
	h := newTestHandler(t)

	// Create project + dataset
	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "lbl-proj"})
	require.Equal(t, http.StatusOK, rec.Code)
	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projARN := projResp["ProjectArn"].(string)

	rec = doRequest(t, h, "CreateDataset", map[string]any{
		"ProjectArn":  projARN,
		"DatasetType": "TRAIN",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var dsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dsResp))
	dsARN := dsResp["DatasetArn"].(string)

	// Add two entries: cat appears twice, dog once
	entry1 := []byte(`{"source-ref":"s3://b/img1.jpg","labels-metadata":{"class-name":"cat"}}`)
	entry2 := []byte(`{"source-ref":"s3://b/img2.jpg","labels-metadata":{"class-name":"dog"}}`)
	entry3 := []byte(`{"source-ref":"s3://b/img3.jpg","labels-metadata":{"class-name":"cat"}}`)

	for _, e := range [][]byte{entry1, entry2, entry3} {
		rec = doRequest(t, h, "UpdateDatasetEntries", map[string]any{
			"DatasetArn": dsARN,
			"Changes":    map[string]any{"GroundTruth": e},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec = doRequest(t, h, "ListDatasetLabels", map[string]any{"DatasetArn": dsARN})
	require.Equal(t, http.StatusOK, rec.Code)

	labels := extractLabels(t, rec.Body.Bytes())
	require.NotNil(t, labels, "expected DatasetLabelStats or DatasetLabels key")
	require.Len(t, labels, 2)

	// Sorted by name: cat, dog
	label0 := labels[0].(map[string]any)
	label1 := labels[1].(map[string]any)
	assert.Equal(t, "cat", label0["LabelName"])
	assert.InDelta(t, float64(2), label0["LabelStats"].(map[string]any)["EntryCount"], 0)
	assert.Equal(t, "dog", label1["LabelName"])
	assert.InDelta(t, float64(1), label1["LabelStats"].(map[string]any)["EntryCount"], 0)
}

func TestListDatasetLabels_MultiLabel(t *testing.T) { //nolint:paralleltest // stateful sequential
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "ml-proj"})
	require.Equal(t, http.StatusOK, rec.Code)
	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))

	rec = doRequest(t, h, "CreateDataset", map[string]any{
		"ProjectArn":  projResp["ProjectArn"].(string),
		"DatasetType": "TRAIN",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var dsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dsResp))
	dsARN := dsResp["DatasetArn"].(string)

	// Multi-label entry: sunglasses + hat
	entry := []byte(`{"source-ref":"s3://b/img.jpg","labels-metadata":{"class-map":{"sunglasses":1,"hat":1}}}`)
	rec = doRequest(t, h, "UpdateDatasetEntries", map[string]any{
		"DatasetArn": dsARN,
		"Changes":    map[string]any{"GroundTruth": entry},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "ListDatasetLabels", map[string]any{"DatasetArn": dsARN})
	require.Equal(t, http.StatusOK, rec.Code)

	labels := extractLabels(t, rec.Body.Bytes())
	require.Len(t, labels, 2)

	names := []string{
		labels[0].(map[string]any)["LabelName"].(string),
		labels[1].(map[string]any)["LabelName"].(string),
	}
	assert.ElementsMatch(t, []string{"hat", "sunglasses"}, names)
}

func TestListDatasetLabels_OpaqueToken(t *testing.T) { //nolint:paralleltest // stateful sequential
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "page-proj"})
	require.Equal(t, http.StatusOK, rec.Code)
	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))

	rec = doRequest(t, h, "CreateDataset", map[string]any{
		"ProjectArn":  projResp["ProjectArn"].(string),
		"DatasetType": "TRAIN",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var dsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dsResp))
	dsARN := dsResp["DatasetArn"].(string)

	// Add 3 entries with distinct labels
	for i, lbl := range []string{"apple", "banana", "cherry"} {
		entry, _ := json.Marshal(map[string]any{
			"source-ref":      "s3://b/img" + string(rune('0'+i)) + ".jpg",
			"labels-metadata": map[string]any{"class-name": lbl},
		})
		rec = doRequest(t, h, "UpdateDatasetEntries", map[string]any{
			"DatasetArn": dsARN,
			"Changes":    map[string]any{"GroundTruth": entry},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// First page: limit 2
	rec = doRequest(t, h, "ListDatasetLabels", map[string]any{
		"DatasetArn": dsARN,
		"MaxResults": 2,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	firstPageLabels := extractLabels(t, rec.Body.Bytes())
	require.Len(t, firstPageLabels, 2)

	var page1 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page1))

	nextToken, hasToken := page1["NextToken"].(string)
	require.True(t, hasToken, "expected NextToken for more pages")
	assert.NotEmpty(t, nextToken)

	// Verify token is base64url-encoded JSON
	decoded, err := base64.RawURLEncoding.DecodeString(nextToken)
	require.NoError(t, err)
	var tok map[string]any
	require.NoError(t, json.Unmarshal(decoded, &tok))
	assert.InDelta(t, float64(2), tok["o"], 0)

	// Second page
	rec = doRequest(t, h, "ListDatasetLabels", map[string]any{
		"DatasetArn": dsARN,
		"MaxResults": 2,
		"NextToken":  nextToken,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	secondPageLabels := extractLabels(t, rec.Body.Bytes())
	require.Len(t, secondPageLabels, 1, "expected 1 label on second page")

	var page2 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page2))
	_, hasMore := page2["NextToken"].(string)
	assert.False(t, hasMore, "should not have next token on last page")
}

func TestListDatasetLabels_InvalidToken(t *testing.T) { //nolint:paralleltest // stateful sequential
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "inv-proj"})
	require.Equal(t, http.StatusOK, rec.Code)
	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))

	rec = doRequest(t, h, "CreateDataset", map[string]any{
		"ProjectArn":  projResp["ProjectArn"].(string),
		"DatasetType": "TRAIN",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var dsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dsResp))

	// Invalid token treated as offset 0 (returns from start, no error)
	rec = doRequest(t, h, "ListDatasetLabels", map[string]any{
		"DatasetArn": dsResp["DatasetArn"].(string),
		"NextToken":  "!!!invalid!!!",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// =============================================================================
// DistributeDatasetEntries
// =============================================================================

func TestDistributeDatasetEntries_SetsInProgress(t *testing.T) { //nolint:paralleltest // stateful
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "dist-proj"})
	require.Equal(t, http.StatusOK, rec.Code)
	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projARN := projResp["ProjectArn"].(string)

	rec = doRequest(t, h, "CreateDataset", map[string]any{
		"ProjectArn":  projARN,
		"DatasetType": "TRAIN",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var dsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dsResp))
	dsARN := dsResp["DatasetArn"].(string)

	rec = doRequest(t, h, "DistributeDatasetEntries", map[string]any{
		"Datasets": []any{
			map[string]any{"DatasetArn": dsARN},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// DescribeDataset should show UPDATE_IN_PROGRESS
	rec = doRequest(t, h, "DescribeDataset", map[string]any{"DatasetArn": dsARN})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	desc := descResp["DatasetDescription"].(map[string]any)
	assert.Equal(t, "UPDATE_IN_PROGRESS", desc["Status"])
}

// DeleteDataset rejects a dataset that is updating with
// ResourceInUseException -- DeleteDatasetInput's own doc comment
// (api_op_DeleteDataset.go): "You can't delete a dataset while it is
// creating (Status = CREATE_IN_PROGRESS) or if the dataset is updating
// (Status = UPDATE_IN_PROGRESS).".
func TestDeleteDataset_WhileUpdating_Rejected(t *testing.T) { //nolint:paralleltest // stateful sequential
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "del-ds-proj"})
	require.Equal(t, http.StatusOK, rec.Code)
	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projARN := projResp["ProjectArn"].(string)

	rec = doRequest(t, h, "CreateDataset", map[string]any{
		"ProjectArn":  projARN,
		"DatasetType": "TRAIN",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var dsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dsResp))
	dsARN := dsResp["DatasetArn"].(string)

	rec = doRequest(t, h, "DistributeDatasetEntries", map[string]any{
		"Datasets": []any{
			map[string]any{"DatasetArn": dsARN},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DeleteDataset", map[string]any{"DatasetArn": dsARN})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ResourceInUseException", errResp["__type"])
}

func TestDistributeDatasetEntries_UnknownDataset(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "DistributeDatasetEntries", map[string]any{
		"Datasets": []any{
			map[string]any{"DatasetArn": "arn:aws:rekognition:us-east-1:000000000000:project/x/dataset/train/1"},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDatasets(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	// Create project first
	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "ds-proj"})
	require.Equal(t, http.StatusOK, rec.Code)

	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projectARN := projResp["ProjectArn"].(string)

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte) string
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "CreateDataset returns ARN",
			action: "CreateDataset",
			body: map[string]any{
				"ProjectArn":  projectARN,
				"DatasetType": "TRAIN",
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) string {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				arn := resp["DatasetArn"].(string)
				assert.Contains(t, arn, "arn:aws:rekognition:")

				return arn
			},
		},
	}

	var datasetARN string

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, tc.action, tc.body) //nolint:govet // existing issue.
			assert.Equal(t, tc.wantCode, rec.Code, tc.name)

			if tc.check != nil {
				datasetARN = tc.check(t, rec.Body.Bytes())
			}
		})
	}

	// DescribeDataset
	t.Run("DescribeDataset returns details", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest( //nolint:govet // existing issue.
			t,
			h,
			"DescribeDataset",
			map[string]any{"DatasetArn": datasetARN},
		)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		desc := resp["DatasetDescription"].(map[string]any)
		assert.Equal(t, "CREATE_COMPLETE", desc["Status"])
		assert.Equal(t, "TRAIN", desc["DatasetType"])
	})

	// UpdateDatasetEntries
	t.Run("UpdateDatasetEntries succeeds", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest(t, h, "UpdateDatasetEntries", map[string]any{ //nolint:govet // existing issue.
			"DatasetArn": datasetARN,
			"Changes":    map[string]any{"GroundTruth": []byte(`{"source-ref": "s3://bucket/img.jpg"}`)},
		})
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	// ListDatasetEntries
	t.Run("ListDatasetEntries returns entries", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest( //nolint:govet // existing issue.
			t,
			h,
			"ListDatasetEntries",
			map[string]any{"DatasetArn": datasetARN},
		)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.NotNil(t, resp["DatasetEntries"])
	})

	// ListDatasetLabels
	t.Run("ListDatasetLabels returns empty list", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest( //nolint:govet // existing issue.
			t,
			h,
			"ListDatasetLabels",
			map[string]any{"DatasetArn": datasetARN},
		)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.NotNil(t, resp["DatasetLabelDescriptions"])
	})

	// DistributeDatasetEntries
	t.Run("DistributeDatasetEntries succeeds", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest(t, h, "DistributeDatasetEntries", map[string]any{ //nolint:govet // existing issue.
			"Datasets": []any{
				map[string]any{"DatasetArn": datasetARN},
			},
		})
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	// DeleteDataset. datasetARN is UPDATE_IN_PROGRESS after the
	// DistributeDatasetEntries subtest above and can no longer be deleted
	// (see TestDeleteDataset_WhileUpdating_Rejected) -- exercise the
	// success path on a freshly created, CREATE_COMPLETE dataset instead.
	t.Run("DeleteDataset removes dataset", func(t *testing.T) { //nolint:paralleltest // existing issue.
		rec := doRequest(t, h, "CreateDataset", map[string]any{ //nolint:govet // existing issue.
			"ProjectArn":  projectARN,
			"DatasetType": "TEST",
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var dsResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dsResp))
		freshARN := dsResp["DatasetArn"].(string)

		rec = doRequest(t, h, "DeleteDataset", map[string]any{"DatasetArn": freshARN})
		assert.Equal(t, http.StatusOK, rec.Code)

		// DescribeDataset should now return not found
		rec = doRequest(t, h, "DescribeDataset", map[string]any{"DatasetArn": freshARN})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// ---------------------------------------------------------------------------
// CreateDataset rejects a duplicate (ProjectArn, DatasetType) pair with
// ResourceAlreadyExistsException. Previously datasetARN was always
// uuid-suffixed, so this check never fired -- see PARITY.md gaps.
// ---------------------------------------------------------------------------

func TestCreateDataset_DuplicateProjectAndType_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateProject", map[string]any{"ProjectName": "dup-ds-proj"})
	require.Equal(t, http.StatusOK, rec.Code)

	var projResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &projResp))
	projARN := projResp["ProjectArn"].(string)

	rec = doRequest(t, h, "CreateDataset", map[string]any{
		"ProjectArn":  projARN,
		"DatasetType": "TRAIN",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Same project + same DatasetType again -> rejected.
	rec = doRequest(t, h, "CreateDataset", map[string]any{
		"ProjectArn":  projARN,
		"DatasetType": "TRAIN",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ResourceAlreadyExistsException", errResp["__type"])

	// Same project, different DatasetType -> allowed.
	rec = doRequest(t, h, "CreateDataset", map[string]any{
		"ProjectArn":  projARN,
		"DatasetType": "TEST",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDataset_MissingProjectArn(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	tests := []struct {
		body   any
		name   string
		action string
	}{
		{
			name:   "CreateDataset missing ProjectArn",
			action: "CreateDataset",
			body:   map[string]any{"DatasetType": "TRAIN"},
		},
		{
			name:   "CreateDataset missing DatasetType",
			action: "CreateDataset",
			body:   map[string]any{"ProjectArn": "arn:xxx"},
		},
		{
			name:   "DescribeDataset missing DatasetArn",
			action: "DescribeDataset",
			body:   map[string]any{},
		},
		{
			name:   "DeleteDataset missing DatasetArn",
			action: "DeleteDataset",
			body:   map[string]any{},
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code, tc.name)
		})
	}
}
