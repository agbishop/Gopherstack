package rekognition_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rekognition"
)

func TestRekognition_Faces(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		setup    func(h *rekognition.Handler)
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "IndexFaces returns face record",
			action: "IndexFaces",
			setup: func(h *rekognition.Handler) {
				doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "face-coll"})
			},
			body:     map[string]any{"CollectionId": "face-coll"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				records, _ := resp["FaceRecords"].([]any)
				assert.Len(t, records, 1)
			},
		},
		{
			name:     "IndexFaces unknown collection returns error",
			action:   "IndexFaces",
			body:     map[string]any{"CollectionId": "no-coll"},
			wantCode: http.StatusBadRequest,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "ResourceNotFoundException", resp["__type"])
			},
		},
		{
			name:   "ListFaces returns indexed face",
			action: "ListFaces",
			setup: func(h *rekognition.Handler) {
				doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "lf-coll"})
				doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "lf-coll"})
			},
			body:     map[string]any{"CollectionId": "lf-coll"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				faces, _ := resp["Faces"].([]any)
				assert.Len(t, faces, 1)
			},
		},
		{
			name:   "DeleteFaces removes face",
			action: "DeleteFaces",
			setup: func(h *rekognition.Handler) {
				doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "df-coll"})
				doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "df-coll"})
			},
			body: map[string]any{
				"CollectionId": "df-coll",
				"FaceIds":      []string{},
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.NotNil(t, resp["DeletedFaces"])
			},
		},
		{
			name:   "SearchFaces returns matches",
			action: "SearchFaces",
			setup: func(h *rekognition.Handler) {
				doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "sf-coll"})
				doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "sf-coll"})
				doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "sf-coll"})
			},
			body: map[string]any{"CollectionId": "sf-coll"},
			// FaceId will be empty, triggers validation
			wantCode: http.StatusBadRequest,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "InvalidParameterException", resp["__type"])
			},
		},
		{
			name:   "SearchFacesByImage returns matches",
			action: "SearchFacesByImage",
			setup: func(h *rekognition.Handler) {
				doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "sfbi-coll"})
				doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "sfbi-coll"})
			},
			body:     map[string]any{"CollectionId": "sfbi-coll"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				matches, _ := resp["FaceMatches"].([]any)
				assert.Len(t, matches, 1)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			if tc.setup != nil {
				tc.setup(h)
			}

			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)

			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pagination: ListFaces
// ---------------------------------------------------------------------------

func TestListFaces_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "page-faces-coll"})
	require.Equal(t, http.StatusOK, rec.Code)

	// Index 3 faces.
	for range 3 {
		doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "page-faces-coll"})
	}

	// Page 1: MaxResults=2.
	rec = doRequest(t, h, "ListFaces", map[string]any{
		"CollectionId": "page-faces-coll",
		"MaxResults":   2,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var page1 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page1))

	faces1, _ := page1["Faces"].([]any)
	require.Len(t, faces1, 2)

	nextToken1, _ := page1["NextToken"].(string)
	require.NotEmpty(t, nextToken1)

	// Page 2: remaining face.
	rec = doRequest(t, h, "ListFaces", map[string]any{
		"CollectionId": "page-faces-coll",
		"NextToken":    nextToken1,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var page2 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page2))

	faces2, _ := page2["Faces"].([]any)
	require.Len(t, faces2, 1)
	assert.Empty(t, page2["NextToken"])
}

// ---------------------------------------------------------------------------
// SearchFaces with real FaceId returns matches
// ---------------------------------------------------------------------------

func TestSearchFaces_RealFaceId_ReturnsMatches(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "search-real-coll"})

	// Index 3 faces, capture the first face ID.
	rec := doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "search-real-coll"})
	require.Equal(t, http.StatusOK, rec.Code)

	var idx1 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &idx1))
	records, _ := idx1["FaceRecords"].([]any)
	require.Len(t, records, 1)
	rec1, _ := records[0].(map[string]any)
	face1, _ := rec1["Face"].(map[string]any)
	faceID := face1["FaceId"].(string)

	// Index 2 more faces to match against.
	doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "search-real-coll"})
	doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "search-real-coll"})

	// SearchFaces returns the other 2 faces.
	rec = doRequest(t, h, "SearchFaces", map[string]any{
		"CollectionId": "search-real-coll",
		"FaceId":       faceID,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var sf map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sf))
	matches, _ := sf["FaceMatches"].([]any)
	assert.Len(t, matches, 2)
	assert.Equal(t, faceID, sf["SearchedFaceId"])
}

// ---------------------------------------------------------------------------
// SearchFaces with FaceId not in collection returns ResourceNotFoundException
// ---------------------------------------------------------------------------

func TestSearchFaces_UnknownFaceId_Returns404(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "sf-notfound-coll"})

	rec := doRequest(t, h, "SearchFaces", map[string]any{
		"CollectionId": "sf-notfound-coll",
		"FaceId":       "00000000-0000-0000-0000-000000000000",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])
}

// ---------------------------------------------------------------------------
// DeleteFaces by specific IDs removes only those faces
// ---------------------------------------------------------------------------

func TestDeleteFaces_BySpecificIDs(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "delf-coll"})

	// Index 3 faces, capture all face IDs.
	faceIDs := make([]string, 3)

	for i := range 3 {
		rec := doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "delf-coll"})
		require.Equal(t, http.StatusOK, rec.Code)

		var idx map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &idx))
		records, _ := idx["FaceRecords"].([]any)
		rec0, _ := records[0].(map[string]any)
		face, _ := rec0["Face"].(map[string]any)
		faceIDs[i] = face["FaceId"].(string)
	}

	// Delete only the first two faces.
	rec := doRequest(t, h, "DeleteFaces", map[string]any{
		"CollectionId": "delf-coll",
		"FaceIds":      faceIDs[:2],
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var delResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &delResp))
	deleted, _ := delResp["DeletedFaces"].([]any)
	assert.Len(t, deleted, 2)

	// Verify only third face remains.
	rec = doRequest(t, h, "ListFaces", map[string]any{"CollectionId": "delf-coll"})
	require.Equal(t, http.StatusOK, rec.Code)

	var lf map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &lf))
	remaining, _ := lf["Faces"].([]any)
	require.Len(t, remaining, 1)
	remainFace, _ := remaining[0].(map[string]any)
	assert.Equal(t, faceIDs[2], remainFace["FaceId"])
}

// ---------------------------------------------------------------------------
// ListFaces unknown collection returns ResourceNotFoundException
// ---------------------------------------------------------------------------

func TestListFaces_UnknownCollection_Returns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "ListFaces", map[string]any{"CollectionId": "no-such-coll"})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])
}

// ---------------------------------------------------------------------------
// ListFaces empty collection returns non-null Faces slice
// ---------------------------------------------------------------------------

func TestListFaces_EmptyCollection_ReturnsNonNullSlice(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "empty-faces-coll"})

	rec := doRequest(t, h, "ListFaces", map[string]any{"CollectionId": "empty-faces-coll"})
	require.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

	_, hasFaces := raw["Faces"]
	assert.True(t, hasFaces, "Faces field must be present")

	var faces []any
	require.NoError(t, json.Unmarshal(raw["Faces"], &faces))
	assert.NotNil(t, faces, "Faces must not be null")
}

// ---------------------------------------------------------------------------
// IndexFaces ExternalImageId round-trips via ListFaces
// ---------------------------------------------------------------------------

func TestIndexFaces_ExternalImageId_RoundTrips(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "extid-coll"})

	rec := doRequest(t, h, "IndexFaces", map[string]any{
		"CollectionId":    "extid-coll",
		"ExternalImageId": "my-image-123",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var idx map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &idx))
	records, _ := idx["FaceRecords"].([]any)
	require.Len(t, records, 1)
	rec0, _ := records[0].(map[string]any)
	face0, _ := rec0["Face"].(map[string]any)
	assert.Equal(t, "my-image-123", face0["ExternalImageId"])

	// Verify via ListFaces.
	rec2 := doRequest(t, h, "ListFaces", map[string]any{"CollectionId": "extid-coll"})
	require.Equal(t, http.StatusOK, rec2.Code)

	var lf map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &lf))
	faces, _ := lf["Faces"].([]any)
	require.Len(t, faces, 1)
	f0, _ := faces[0].(map[string]any)
	assert.Equal(t, "my-image-123", f0["ExternalImageId"])
}

// decodeFaceIDs pulls FaceId values out of an IndexFaces response.
func decodeFaceIDs(t *testing.T, body []byte) []string {
	t.Helper()

	var resp struct {
		FaceRecords []struct {
			Face struct {
				FaceID string `json:"FaceId"`
			} `json:"Face"`
		} `json:"FaceRecords"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))

	ids := make([]string, 0, len(resp.FaceRecords))
	for _, r := range resp.FaceRecords {
		ids = append(ids, r.Face.FaceID)
	}

	return ids
}

func TestIndexFaces_ConfidenceDeterministicAndPlausible(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.Equal(t, http.StatusOK,
		doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "conf-coll"}).Code)

	confidences := make([]float64, 0, 5)

	for i := range 5 {
		rec := doRequest(t, h, "IndexFaces", map[string]any{
			"CollectionId":    "conf-coll",
			"ExternalImageId": map[bool]string{true: "", false: "subject"}[i%2 == 0],
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			FaceRecords []struct {
				Face struct {
					Confidence float64 `json:"Confidence"`
				} `json:"Face"`
			} `json:"FaceRecords"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Len(t, resp.FaceRecords, 1)

		conf := resp.FaceRecords[0].Face.Confidence
		assert.GreaterOrEqual(t, conf, 90.0, "confidence must be a plausible detection value")
		assert.Less(t, conf, 100.0, "confidence must stay below 100")
		confidences = append(confidences, conf)
	}

	// Distinct faces should not all collapse to a single canned constant.
	distinct := map[float64]struct{}{}
	for _, c := range confidences {
		distinct[c] = struct{}{}
	}

	assert.Greater(t, len(distinct), 1, "confidence must vary per face, not be a canned constant")
}

func TestSearchFaces_SimilarityDeterministic(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.Equal(t, http.StatusOK,
		doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "sim-coll"}).Code)

	// Two faces with distinct external image IDs (different subjects).
	rec1 := doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "sim-coll", "ExternalImageId": "alice"})
	rec2 := doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "sim-coll", "ExternalImageId": "bob"})
	queryID := decodeFaceIDs(t, rec1.Body.Bytes())[0]
	require.NotEmpty(t, queryID)
	require.NotEmpty(t, decodeFaceIDs(t, rec2.Body.Bytes())[0])

	getSim := func() float64 {
		rec := doRequest(t, h, "SearchFaces", map[string]any{"CollectionId": "sim-coll", "FaceId": queryID})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			FaceMatches []struct {
				Similarity float64 `json:"Similarity"`
			} `json:"FaceMatches"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Len(t, resp.FaceMatches, 1)

		return resp.FaceMatches[0].Similarity
	}

	first := getSim()
	second := getSim()

	assert.InDelta(t, first, second, 1e-9, "similarity must be deterministic across calls")
	assert.GreaterOrEqual(t, first, 75.0)
	assert.Less(t, first, 100.0, "different subjects must score below an exact match")
}

func TestSearchFaces_SameExternalImageId_ScoresExactMatch(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.Equal(t, http.StatusOK,
		doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "exact-coll"}).Code)

	// Two faces sharing the same ExternalImageId model the same subject.
	same := map[string]any{"CollectionId": "exact-coll", "ExternalImageId": "same-person"}
	rec1 := doRequest(t, h, "IndexFaces", same)
	doRequest(t, h, "IndexFaces", same)
	queryID := decodeFaceIDs(t, rec1.Body.Bytes())[0]

	rec := doRequest(t, h, "SearchFaces", map[string]any{"CollectionId": "exact-coll", "FaceId": queryID})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		FaceMatches []struct {
			Similarity float64 `json:"Similarity"`
		} `json:"FaceMatches"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.FaceMatches, 1)
	assert.InDelta(t, 100.0, resp.FaceMatches[0].Similarity, 1e-9,
		"faces sharing an ExternalImageId must score an exact match")
}

func TestSearchFacesByImage_SimilarityDependsOnImage(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.Equal(t, http.StatusOK,
		doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "img-coll"}).Code)
	require.Equal(t, http.StatusOK,
		doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "img-coll", "ExternalImageId": "x"}).Code)

	search := func(bucket, key string) float64 {
		rec := doRequest(t, h, "SearchFacesByImage", map[string]any{
			"CollectionId": "img-coll",
			"Image":        map[string]any{"S3Object": map[string]any{"Bucket": bucket, "Name": key}},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp struct {
			FaceMatches []struct {
				Similarity float64 `json:"Similarity"`
			} `json:"FaceMatches"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Len(t, resp.FaceMatches, 1)

		return resp.FaceMatches[0].Similarity
	}

	a := search("b1", "img-a.jpg")
	aAgain := search("b1", "img-a.jpg")
	b := search("b1", "img-b.jpg")

	assert.InDelta(t, a, aAgain, 1e-9, "same image must yield the same similarity")
	assert.NotEqual(t, a, b, "different images must yield different similarities (not canned)")
}

func TestFaceRoundTrip_IndexSearchListDelete(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.Equal(t, http.StatusOK,
		doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "rt-coll"}).Code)

	// Index two faces.
	rec1 := doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "rt-coll", "ExternalImageId": "a"})
	rec2 := doRequest(t, h, "IndexFaces", map[string]any{"CollectionId": "rt-coll", "ExternalImageId": "b"})
	id1 := decodeFaceIDs(t, rec1.Body.Bytes())[0]
	id2 := decodeFaceIDs(t, rec2.Body.Bytes())[0]
	require.NotEmpty(t, id1)
	require.NotEmpty(t, id2)

	// ListFaces reflects both.
	listRec := doRequest(t, h, "ListFaces", map[string]any{"CollectionId": "rt-coll"})
	require.Equal(t, http.StatusOK, listRec.Code)

	var list struct {
		Faces []struct {
			FaceID string `json:"FaceId"`
		} `json:"Faces"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &list))
	require.Len(t, list.Faces, 2)

	// SearchFaces from id1 finds id2.
	searchRec := doRequest(t, h, "SearchFaces", map[string]any{"CollectionId": "rt-coll", "FaceId": id1})
	require.Equal(t, http.StatusOK, searchRec.Code)

	var search struct {
		FaceMatches []struct {
			Face struct {
				FaceID string `json:"FaceId"`
			} `json:"Face"`
		} `json:"FaceMatches"`
	}
	require.NoError(t, json.Unmarshal(searchRec.Body.Bytes(), &search))
	require.Len(t, search.FaceMatches, 1)
	assert.Equal(t, id2, search.FaceMatches[0].Face.FaceID)

	// Delete id2.
	delRec := doRequest(t, h, "DeleteFaces", map[string]any{"CollectionId": "rt-coll", "FaceIds": []string{id2}})
	require.Equal(t, http.StatusOK, delRec.Code)

	// SearchFaces from id1 now finds nothing.
	searchRec2 := doRequest(t, h, "SearchFaces", map[string]any{"CollectionId": "rt-coll", "FaceId": id1})
	require.Equal(t, http.StatusOK, searchRec2.Code)

	var search2 struct {
		FaceMatches []json.RawMessage `json:"FaceMatches"`
	}
	require.NoError(t, json.Unmarshal(searchRec2.Body.Bytes(), &search2))
	assert.Empty(t, search2.FaceMatches)

	// SearchFaces for the deleted id returns ResourceNotFoundException.
	nfRec := doRequest(t, h, "SearchFaces", map[string]any{"CollectionId": "rt-coll", "FaceId": id2})
	require.Equal(t, http.StatusBadRequest, nfRec.Code)
	assert.Contains(t, nfRec.Body.String(), "ResourceNotFoundException")
}

// =============================================================================
// Image Analysis (stateless mock results)
// =============================================================================

func TestImageAnalysis_CompareFaces(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CompareFaces", map[string]any{
		"SourceImage": map[string]any{},
		"TargetImage": map[string]any{},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp["FaceMatches"])
	assert.NotNil(t, resp["SourceImageFace"])
}

func TestImageAnalysis_DetectFaces(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "DetectFaces", map[string]any{"Image": map[string]any{}})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotNil(t, resp["FaceDetails"])
}

// =============================================================================
// gopherstack-qlqz: MinConfidence / QualityFilter / Attributes enum validation
// =============================================================================

func TestImageAnalysis_EnumValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{
			name:   "DetectFaces rejects unknown Attributes value",
			action: "DetectFaces",
			body: map[string]any{
				"Image":      map[string]any{},
				"Attributes": []string{"NOT_A_REAL_ATTRIBUTE"},
			},
		},
		{
			name:   "CompareFaces rejects unknown QualityFilter value",
			action: "CompareFaces",
			body: map[string]any{
				"SourceImage":   map[string]any{},
				"TargetImage":   map[string]any{},
				"QualityFilter": "NOT_A_REAL_FILTER",
			},
		},
		{
			name:   "SearchFacesByImage rejects unknown QualityFilter value",
			action: "SearchFacesByImage",
			body: map[string]any{
				"CollectionId":  "sfbi-enum-coll",
				"Image":         map[string]any{},
				"QualityFilter": "NOT_A_REAL_FILTER",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			rec := doRequest(t, h, tc.action, tc.body)
			require.Equal(t, http.StatusBadRequest, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "InvalidParameterException", resp["__type"])
		})
	}
}

func TestImageAnalysis_EnumValidation_AcceptsValidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{
			name:   "DetectFaces accepts ALL attribute",
			action: "DetectFaces",
			body: map[string]any{
				"Image":      map[string]any{},
				"Attributes": []string{"ALL"},
			},
		},
		{
			name:   "CompareFaces accepts HIGH quality filter",
			action: "CompareFaces",
			body: map[string]any{
				"SourceImage":   map[string]any{},
				"TargetImage":   map[string]any{},
				"QualityFilter": "HIGH",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

// =============================================================================
// Async Video Jobs: face detection / face search
// =============================================================================

func TestAsyncVideoJobs_Faces(t *testing.T) { //nolint:paralleltest // stateful sequential
	h := newTestHandler(t)

	type jobFlow struct {
		startBody   any
		checkGet    func(t *testing.T, body []byte)
		startAction string
		getAction   string
	}

	flows := []jobFlow{
		{
			startAction: "StartFaceDetection",
			startBody:   map[string]any{"Video": map[string]any{}},
			getAction:   "GetFaceDetection",
			checkGet: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "SUCCEEDED", resp["JobStatus"])
				assert.NotNil(t, resp["Faces"])
			},
		},
		{
			startAction: "StartFaceSearch",
			startBody:   map[string]any{"Video": map[string]any{}, "CollectionId": ""},
			getAction:   "GetFaceSearch",
			checkGet: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "SUCCEEDED", resp["JobStatus"])
				assert.NotNil(t, resp["Persons"])
			},
		},
	}

	for _, flow := range flows { //nolint:paralleltest // existing issue.
		t.Run(flow.startAction+"/"+flow.getAction, func(t *testing.T) {
			// Start job
			rec := doRequest(t, h, flow.startAction, flow.startBody)
			require.Equal(t, http.StatusOK, rec.Code, flow.startAction)

			var startResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &startResp))
			jobID, ok := startResp["JobId"].(string)
			require.True(t, ok)
			assert.NotEmpty(t, jobID)

			// First poll returns IN_PROGRESS
			rec = doRequest(t, h, flow.getAction, map[string]any{"JobId": jobID})
			require.Equal(t, http.StatusOK, rec.Code, flow.getAction)

			var firstResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &firstResp))
			assert.Equal(t, "IN_PROGRESS", firstResp["JobStatus"])

			// Second poll returns SUCCEEDED
			rec = doRequest(t, h, flow.getAction, map[string]any{"JobId": jobID})
			require.Equal(t, http.StatusOK, rec.Code, flow.getAction)

			if flow.checkGet != nil {
				flow.checkGet(t, rec.Body.Bytes())
			}
		})
	}
}

func TestAsyncVideoJobs_Faces_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	getActions := []string{"GetFaceDetection", "GetFaceSearch"}

	for _, action := range getActions {
		t.Run(action+"_unknown_job", func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, action, map[string]any{"JobId": "00000000-0000-0000-0000-000000000000"})
			assert.Equal(t, http.StatusBadRequest, rec.Code, action)
		})
	}
}

func TestAsyncVideoJobs_Faces_MissingJobId(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	getActions := []string{"GetFaceDetection", "GetFaceSearch"}

	for _, action := range getActions {
		t.Run(action+"_missing_job_id", func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, action, map[string]any{})
			assert.Equal(t, http.StatusBadRequest, rec.Code, action)
		})
	}
}
