package macie2_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/macie2"
)

// TestBuckets_DescribeBuckets_MaxResultsHonoured proves DescribeBuckets
// applies maxResults/nextToken, which the handler used to not even parse
// from the request body before the fix -- every bucket always came back on
// one page regardless of what a real client sent.
func TestBuckets_DescribeBuckets_MaxResultsHonoured(t *testing.T) {
	t.Parallel()

	h, b := newBucketHandlerAndBackend(t)

	seedBucket(t, b, "bucket-a", "us-east-1", "NOT_PUBLIC", "AES256", 0, 0)
	seedBucket(t, b, "bucket-b", "us-east-1", "NOT_PUBLIC", "AES256", 0, 0)
	seedBucket(t, b, "bucket-c", "us-east-1", "NOT_PUBLIC", "AES256", 0, 0)

	rec := doRequest(t, h, http.MethodPost, "/datasources/s3", map[string]any{"maxResults": 1})
	require.Equal(t, http.StatusOK, rec.Code)

	var page1 struct {
		NextToken string           `json:"nextToken"`
		Buckets   []map[string]any `json:"buckets"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page1))
	require.Len(t, page1.Buckets, 1, "maxResults=1 must limit the page to 1 item")
	require.NotEmpty(t, page1.NextToken, "a partial page must return a nextToken")

	rec2 := doRequest(t, h, http.MethodPost, "/datasources/s3", map[string]any{
		"maxResults": 3, "nextToken": page1.NextToken,
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var page2 struct {
		NextToken string           `json:"nextToken"`
		Buckets   []map[string]any `json:"buckets"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &page2))
	assert.Len(t, page2.Buckets, 2, "second page must return the remainder")
}

// TestBuckets_DescribeBuckets_SortCriteriaHonoured proves DescribeBuckets
// applies sortCriteria, which was always ignored (hardcoded ascending by
// bucketName) before the fix.
func TestBuckets_DescribeBuckets_SortCriteriaHonoured(t *testing.T) {
	t.Parallel()

	h, b := newBucketHandlerAndBackend(t)

	// Names are alphabetically ascending (aaa < bbb < ccc) but objectCount
	// DESC order is bbb, ccc, aaa -- deliberately not the same order as
	// bucketName-ascending, so this test can't pass by coincidence against
	// the old hardcoded "always sort by bucketName ascending" behavior.
	seedBucket(t, b, "aaa-bucket", "us-east-1", "NOT_PUBLIC", "AES256", 1, 0)
	seedBucket(t, b, "bbb-bucket", "us-east-1", "NOT_PUBLIC", "AES256", 100, 0)
	seedBucket(t, b, "ccc-bucket", "us-east-1", "NOT_PUBLIC", "AES256", 50, 0)

	rec := doRequest(t, h, http.MethodPost, "/datasources/s3", map[string]any{
		"sortCriteria": map[string]any{"attributeName": "objectCount", "orderBy": "DESC"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Buckets []map[string]any `json:"buckets"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Buckets, 3)

	assert.Equal(t, "bbb-bucket", resp.Buckets[0]["bucketName"])
	assert.Equal(t, "ccc-bucket", resp.Buckets[1]["bucketName"])
	assert.Equal(t, "aaa-bucket", resp.Buckets[2]["bucketName"])
}

// TestGetFindingStatistics_FindingCriteriaHonoured proves GetFindingStatistics
// applies its FindingCriteria parameter, which was discarded into `_` before
// the fix -- every finding was counted regardless of the filter.
func TestGetFindingStatistics_FindingCriteriaHonoured(t *testing.T) {
	t.Parallel()

	h, _ := newBucketHandlerAndBackend(t)

	rec := doRequest(t, h, http.MethodPost, "/findings/sample", map[string]any{
		"findingTypes": []string{"Policy:IAMUser/TypeA", "Policy:IAMUser/TypeB"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doRequest(t, h, http.MethodPost, "/findings/statistics", map[string]any{
		"groupBy": "type",
		"findingCriteria": map[string]any{
			"criterion": map[string]any{
				"type": map[string]any{"eq": []any{"Policy:IAMUser/TypeA"}},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var stats struct {
		CountsByGroup []map[string]any `json:"countsByGroup"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &stats))
	require.Len(t, stats.CountsByGroup, 1, "FindingCriteria must exclude TypeB's group")
	assert.Equal(t, "Policy:IAMUser/TypeA", stats.CountsByGroup[0]["groupKey"])
}

// TestFindingsFilter_ArchiveActionHonoured proves an ARCHIVE-action findings filter is
// applied to matching findings created by CreateSampleFindings, mirroring guardduty's
// identical fix for its own filter Action (b.findingsFilters was previously stored and
// echoed back but never consulted by any finding-creation path).
func TestFindingsFilter_ArchiveActionHonoured(t *testing.T) {
	t.Parallel()

	h, _ := newBucketHandlerAndBackend(t)

	rec := doRequest(t, h, http.MethodPost, "/findingsfilters", map[string]any{
		"name":   "archive-type-a",
		"action": "ARCHIVE",
		"findingCriteria": map[string]any{
			"criterion": map[string]any{
				"type": map[string]any{"eq": []any{"Policy:IAMUser/TypeA"}},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doRequest(t, h, http.MethodPost, "/findings/sample", map[string]any{
		"findingTypes": []string{"Policy:IAMUser/TypeA", "Policy:IAMUser/TypeB"},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	rec3 := doRequest(t, h, http.MethodPost, "/findings", map[string]any{})
	require.Equal(t, http.StatusOK, rec3.Code)

	var listResp struct {
		FindingIDs []string `json:"findingIds"`
	}
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &listResp))
	require.Len(t, listResp.FindingIDs, 2)

	rec4 := doRequest(t, h, http.MethodPost, "/findings/describe", map[string]any{
		"findingIds": listResp.FindingIDs,
	})
	require.Equal(t, http.StatusOK, rec4.Code)

	var describeResp struct {
		Findings []map[string]any `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(rec4.Body.Bytes(), &describeResp))
	require.Len(t, describeResp.Findings, 2)

	for _, f := range describeResp.Findings {
		switch f["type"] {
		case "Policy:IAMUser/TypeA":
			assert.Equal(t, true, f["archived"], "TypeA matches the ARCHIVE filter and must be archived")
		case "Policy:IAMUser/TypeB":
			assert.Equal(t, false, f["archived"], "TypeB does not match the filter and must not be archived")
		default:
			t.Fatalf("unexpected finding type %v", f["type"])
		}
	}
}

// TestListFindings_SortCriteriaHonoured proves ListFindings applies its
// SortCriteria parameter, which was parsed but never passed to the backend
// before the fix (findings always came back in ID order).
func TestListFindings_SortCriteriaHonoured(t *testing.T) {
	t.Parallel()

	h, _ := newBucketHandlerAndBackend(t)

	rec := doRequest(t, h, http.MethodPost, "/findings/sample", map[string]any{
		"findingTypes": []string{"Policy:IAMUser/AAA", "Policy:IAMUser/ZZZ"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doRequest(t, h, http.MethodPost, "/findings", map[string]any{
		"sortCriteria": map[string]any{"attributeName": "type", "orderBy": "DESC"},
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var listResp struct {
		FindingIDs []string `json:"findingIds"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &listResp))
	require.Len(t, listResp.FindingIDs, 2)

	rec3 := doRequest(t, h, http.MethodPost, "/findings/describe", map[string]any{
		"findingIds": listResp.FindingIDs,
	})
	require.Equal(t, http.StatusOK, rec3.Code)

	var describeResp struct {
		Findings []macie2.Finding `json:"findings"`
	}
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &describeResp))

	byID := make(map[string]string, len(describeResp.Findings))
	for _, f := range describeResp.Findings {
		byID[f.ID] = f.Type
	}

	assert.Equal(t, "Policy:IAMUser/ZZZ", byID[listResp.FindingIDs[0]], "DESC by type must put ZZZ first")
	assert.Equal(t, "Policy:IAMUser/AAA", byID[listResp.FindingIDs[1]], "DESC by type must put AAA last")
}

// TestListClassificationJobs_SortCriteriaHonoured proves ListClassificationJobs
// applies its SortCriteria parameter, which was parsed but never passed to
// the backend before the fix (jobs always came back in createdAt order).
func TestListClassificationJobs_SortCriteriaHonoured(t *testing.T) {
	t.Parallel()

	h, _ := newBucketHandlerAndBackend(t)

	for _, name := range []string{"aaa-job", "zzz-job"} {
		rec := doRequest(t, h, http.MethodPost, "/jobs", map[string]any{
			"name":            name,
			"jobType":         "ONE_TIME",
			"s3JobDefinition": map[string]any{"bucketDefinitions": []any{}},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	rec := doRequest(t, h, http.MethodPost, "/jobs/list", map[string]any{
		"sortCriteria": map[string]any{"attributeName": "name", "orderBy": "DESC"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 2)

	assert.Equal(t, "zzz-job", resp.Items[0]["name"])
	assert.Equal(t, "aaa-job", resp.Items[1]["name"])
}
