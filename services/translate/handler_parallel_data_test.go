package translate_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/translate"
)

func TestCreateParallelData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		wantCode int
	}{
		{
			name: "creates parallel data",
			body: map[string]any{
				"Name":        "my-pd",
				"Description": "test data",
				"ParallelDataConfig": map[string]any{
					"S3Uri":  "s3://bucket/key.tmx",
					"Format": "TMX",
				},
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()

				m := unmarshalJSON(t, body)
				assert.Equal(t, "my-pd", m["Name"])
				// Real CreateParallelData starts the resource at CREATING;
				// it only becomes ACTIVE once a caller polls GetParallelData
				// (see TestGetParallelData_AdvancesCreatingToActive).
				assert.Equal(t, "CREATING", m["Status"])
			},
		},
		{
			name:     "missing Name returns error",
			body:     map[string]any{"Description": "no name"},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "duplicate name returns conflict",
			body: map[string]any{
				"Name": "dup-pd",
				"ParallelDataConfig": map[string]any{
					"S3Uri":  "s3://bucket/a.tmx",
					"Format": "TMX",
				},
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tc.name == "duplicate name returns conflict" {
				rec := doRequest(t, h, "CreateParallelData", tc.body)
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doRequest(t, h, "CreateParallelData", tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)

			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestGetParallelData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
		preload  bool
	}{
		{
			name:     "returns existing parallel data",
			wantCode: http.StatusOK,
			preload:  true,
		},
		{
			name:     "error when not found",
			wantCode: http.StatusBadRequest,
			preload:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tc.preload {
				rec := doRequest(t, h, "CreateParallelData", map[string]any{
					"Name":               "pd-1",
					"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
				})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doRequest(t, h, "GetParallelData", map[string]any{"Name": "pd-1"})
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}

func TestDeleteParallelData(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateParallelData", map[string]any{
		"Name":               "pd-del",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DeleteParallelData", map[string]any{"Name": "pd-del"})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "GetParallelData", map[string]any{"Name": "pd-del"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListParallelData(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, name := range []string{"pd-a", "pd-b", "pd-c"} {
		rec := doRequest(t, h, "CreateParallelData", map[string]any{
			"Name":               name,
			"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec := doRequest(t, h, "ListParallelData", map[string]any{})
	assert.Equal(t, http.StatusOK, rec.Code)

	m := unmarshalJSON(t, rec.Body.Bytes())
	list, _ := m["ParallelDataPropertiesList"].([]any)
	assert.Len(t, list, 3)
}

func TestUpdateParallelData(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateParallelData", map[string]any{
		"Name":               "pd-update",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Advance CREATING -> ACTIVE: updating a still-CREATING resource is a
	// ConcurrentModificationException (see TestUpdateParallelData_ConcurrentModification).
	doRequest(t, h, "GetParallelData", map[string]any{"Name": "pd-update"})

	rec = doRequest(t, h, "UpdateParallelData", map[string]any{
		"Name":               "pd-update",
		"Description":        "updated description",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f2.tmx", "Format": "TMX"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	m := unmarshalJSON(t, rec.Body.Bytes())
	assert.Equal(t, "pd-update", m["Name"])
}

// TestListParallelData_Pagination verifies that MaxResults and NextToken
// paginate correctly through all parallel data resources.
func TestListParallelData_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 4 {
		rec := doRequest(t, h, "CreateParallelData", map[string]any{
			"Name":               "pd-" + string(rune('a'+i)),
			"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec := doRequest(t, h, "ListParallelData", map[string]any{"MaxResults": 2})
	require.Equal(t, http.StatusOK, rec.Code)

	m := unmarshalJSON(t, rec.Body.Bytes())
	page1, _ := m["ParallelDataPropertiesList"].([]any)
	assert.Len(t, page1, 2)
	nextToken, _ := m["NextToken"].(string)
	assert.NotEmpty(t, nextToken)

	rec = doRequest(t, h, "ListParallelData", map[string]any{"MaxResults": 10, "NextToken": nextToken})
	require.Equal(t, http.StatusOK, rec.Code)

	m = unmarshalJSON(t, rec.Body.Bytes())
	page2, _ := m["ParallelDataPropertiesList"].([]any)
	assert.Len(t, page2, 2)
	assert.Nil(t, m["NextToken"])
}

// TestGetParallelData_DataLocationField verifies that GetParallelData returns
// a DataLocation with RepositoryType and Location fields, matching AWS behavior.
func TestGetParallelData_DataLocationField(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateParallelData", map[string]any{
		"Name":               "loc-pd",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "GetParallelData", map[string]any{"Name": "loc-pd"})
	require.Equal(t, http.StatusOK, rec.Code)

	m := unmarshalJSON(t, rec.Body.Bytes())
	loc, ok := m["DataLocation"].(map[string]any)
	require.True(t, ok, "DataLocation must be present")
	assert.Equal(t, "S3", loc["RepositoryType"])
	assert.NotEmpty(t, loc["Location"])
}

// TestUpdateParallelData_NotFound verifies that updating a nonexistent
// parallel data resource returns ResourceNotFoundException.
func TestUpdateParallelData_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "UpdateParallelData", map[string]any{
		"Name":               "nonexistent-pd",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ResourceNotFoundException", body["__type"])
}

// TestCreateParallelData_DuplicateNameReturnsConflictException verifies that
// a name conflict on CreateParallelData reports the real Amazon Translate
// wire type "ConflictException". Real Translate has no "ResourceInUseException"
// exception shape at all (confirmed absent from
// aws-sdk-go-v2/service/translate/types/errors.go and the smithy model) --
// CreateParallelData/UpdateParallelData model ConflictException for exactly
// this case.
func TestCreateParallelData_DuplicateNameReturnsConflictException(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"Name":               "conflict-check-pd",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
	}

	rec := doRequest(t, h, "CreateParallelData", body)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "CreateParallelData", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var respBody map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &respBody))
	assert.Equal(t, "ConflictException", respBody["__type"])
}

// TestGetParallelData_AdvancesCreatingToActive verifies that a parallel data
// resource starts at CREATING (matching real CreateParallelData's documented
// "When the resource is ready for you to use, the status is ACTIVE" -- i.e.
// not immediately) and advances to ACTIVE the next time it is polled via
// GetParallelData, mirroring DescribeTextTranslationJob's advance-on-poll
// convention for translation jobs. Previously the resource was ACTIVE the
// instant it was created, skipping the async CREATING state real AWS goes
// through.
func TestGetParallelData_AdvancesCreatingToActive(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doRequest(t, h, "CreateParallelData", map[string]any{
		"Name":               "advance-pd",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)
	assert.Equal(t, "CREATING", unmarshalJSON(t, createRec.Body.Bytes())["Status"])

	getRec := doRequest(t, h, "GetParallelData", map[string]any{"Name": "advance-pd"})
	require.Equal(t, http.StatusOK, getRec.Code)

	props := unmarshalJSON(t, getRec.Body.Bytes())["ParallelDataProperties"].(map[string]any)
	assert.Equal(t, "ACTIVE", props["Status"], "GetParallelData must advance CREATING to ACTIVE")

	// Further polling must not regress past the terminal state.
	getRec2 := doRequest(t, h, "GetParallelData", map[string]any{"Name": "advance-pd"})
	require.Equal(t, http.StatusOK, getRec2.Code)
	props2 := unmarshalJSON(t, getRec2.Body.Bytes())["ParallelDataProperties"].(map[string]any)
	assert.Equal(t, "ACTIVE", props2["Status"])
}

// TestListParallelData_DoesNotAdvance verifies that listing parallel data is
// a pure read: it must not itself advance CREATING -> ACTIVE the way
// GetParallelData does, matching ListTextTranslationJobs's precedent that
// List operations never mutate state.
func TestListParallelData_DoesNotAdvance(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateParallelData", map[string]any{
		"Name":               "list-no-advance-pd",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
	})

	for range 3 {
		rec := doRequest(t, h, "ListParallelData", map[string]any{})
		require.Equal(t, http.StatusOK, rec.Code)

		list := unmarshalJSON(t, rec.Body.Bytes())["ParallelDataPropertiesList"].([]any)
		require.Len(t, list, 1)
		assert.Equal(t, "CREATING", list[0].(map[string]any)["Status"])
	}
}

// TestUpdateParallelData_AdvancesUpdatingToActive verifies that
// UpdateParallelData puts an ACTIVE resource into UPDATING (tracking a real,
// non-hardcoded LatestUpdateAttemptStatus) and that GetParallelData advances
// both Status and LatestUpdateAttemptStatus back to ACTIVE on the next poll.
// Previously LatestUpdateAttemptStatus was hardcoded to "ACTIVE" in the
// handler regardless of actual state, and the resource never left ACTIVE at
// all.
func TestUpdateParallelData_AdvancesUpdatingToActive(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateParallelData", map[string]any{
		"Name":               "update-advance-pd",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
	})
	// Advance CREATING -> ACTIVE before updating.
	doRequest(t, h, "GetParallelData", map[string]any{"Name": "update-advance-pd"})

	updateRec := doRequest(t, h, "UpdateParallelData", map[string]any{
		"Name":               "update-advance-pd",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f2.tmx", "Format": "TMX"},
	})
	require.Equal(t, http.StatusOK, updateRec.Code)

	updateResp := unmarshalJSON(t, updateRec.Body.Bytes())
	assert.Equal(t, "UPDATING", updateResp["Status"])
	assert.Equal(t, "UPDATING", updateResp["LatestUpdateAttemptStatus"])

	getRec := doRequest(t, h, "GetParallelData", map[string]any{"Name": "update-advance-pd"})
	require.Equal(t, http.StatusOK, getRec.Code)

	props := unmarshalJSON(t, getRec.Body.Bytes())["ParallelDataProperties"].(map[string]any)
	assert.Equal(t, "ACTIVE", props["Status"])
	assert.Equal(t, "ACTIVE", props["LatestUpdateAttemptStatus"])
}

// TestUpdateParallelData_ConcurrentModification verifies that updating a
// parallel data resource that is still CREATING or UPDATING from a prior
// call reports ConcurrentModificationException, matching
// types/errors.go's doc ("Another modification is being made. That
// modification must complete before you can make your change.").
func TestUpdateParallelData_ConcurrentModification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(t *testing.T, h *translate.Handler, name string)
		name  string
	}{
		{
			name:  "still_creating",
			setup: func(_ *testing.T, _ *translate.Handler, _ string) {},
		},
		{
			name: "still_updating",
			setup: func(t *testing.T, h *translate.Handler, name string) {
				t.Helper()
				// Advance CREATING -> ACTIVE, then start (but don't observe
				// the completion of) a second update.
				doRequest(t, h, "GetParallelData", map[string]any{"Name": name})
				rec := doRequest(t, h, "UpdateParallelData", map[string]any{
					"Name":               name,
					"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/mid.tmx", "Format": "TMX"},
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			name := "concurrent-mod-" + tc.name

			createRec := doRequest(t, h, "CreateParallelData", map[string]any{
				"Name":               name,
				"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
			})
			require.Equal(t, http.StatusOK, createRec.Code)

			tc.setup(t, h, name)

			rec := doRequest(t, h, "UpdateParallelData", map[string]any{
				"Name":               name,
				"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f2.tmx", "Format": "TMX"},
			})
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var body map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "ConcurrentModificationException", body["__type"])
		})
	}
}

// TestCreateParallelData_FormatValidation verifies that
// ParallelDataConfig.Format is restricted to the modeled CSV|TMX|TSV enum.
func TestCreateParallelData_FormatValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		format   string
		wantCode int
	}{
		{name: "csv_accepted", format: "CSV", wantCode: http.StatusOK},
		{name: "tmx_accepted", format: "TMX", wantCode: http.StatusOK},
		{name: "tsv_accepted", format: "TSV", wantCode: http.StatusOK},
		{name: "invalid_rejected", format: "XML", wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateParallelData", map[string]any{
				"Name":               "format-test-pd-" + tt.name,
				"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f", "Format": tt.format},
			})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestCreateParallelData_TooManyTagsRejected verifies that creating a
// parallel data resource with more than 50 tags is rejected as
// TooManyTagsException.
func TestCreateParallelData_TooManyTagsRejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	const tooMany = 51

	tags := make([]map[string]any, 0, tooMany)
	for i := range tooMany {
		tags = append(tags, map[string]any{"Key": "k" + string(rune('a'+i)), "Value": "v"})
	}

	rec := doRequest(t, h, "CreateParallelData", map[string]any{
		"Name":               "many-tags-pd",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
		"Tags":               tags,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "TooManyTagsException", body["__type"])
}

// TestUpdateParallelData_IncludesLatestUpdateAttemptAt verifies
// UpdateParallelData response includes LatestUpdateAttemptAt.
func TestUpdateParallelData_IncludesLatestUpdateAttemptAt(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateParallelData", map[string]any{
		"Name": "pd-update-test",
		"ParallelDataConfig": map[string]any{
			"S3Uri":  "s3://bucket/pd/",
			"Format": "TSV",
		},
	})
	// Advance CREATING -> ACTIVE before updating (see
	// TestUpdateParallelData_ConcurrentModification).
	doRequest(t, h, "GetParallelData", map[string]any{"Name": "pd-update-test"})

	rec := doRequest(t, h, "UpdateParallelData", map[string]any{
		"Name": "pd-update-test",
		"ParallelDataConfig": map[string]any{
			"S3Uri":  "s3://bucket/pd-v2/",
			"Format": "TSV",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := unmarshalJSON(t, rec.Body.Bytes())
	_, hasAt := resp["LatestUpdateAttemptAt"]
	assert.True(t, hasAt, "LatestUpdateAttemptAt must be present in UpdateParallelData response")
}
