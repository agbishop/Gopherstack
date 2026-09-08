package sagemaker_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// LabelingJob
// ---------------------------------------------------------------------------

func createLabelingJobRequestBody(name, workteamArn string) map[string]any {
	return map[string]any{
		"LabelingJobName":    name,
		"LabelAttributeName": "label",
		"RoleArn":            "arn:aws:iam::000000000000:role/LabelingRole",
		"InputConfig": map[string]any{
			"DataSource": map[string]any{
				"S3DataSource": map[string]any{"ManifestS3Uri": "s3://bucket/manifest.json"},
			},
		},
		"OutputConfig": map[string]any{"S3OutputPath": "s3://bucket/output/"},
		"HumanTaskConfig": map[string]any{
			"WorkteamArn":                       workteamArn,
			"UiConfig":                          map[string]any{"UiTemplateS3Uri": "s3://bucket/template.html"},
			"TaskTitle":                         "Label it",
			"TaskDescription":                   "Please label",
			"NumberOfHumanWorkersPerDataObject": 1,
			"TaskTimeLimitInSeconds":            3600,
		},
	}
}

func TestHandler_CreateLabelingJob(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(
		t,
		h,
		"CreateLabelingJob",
		createLabelingJobRequestBody("job-1", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1"),
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["LabelingJobArn"], "labeling-job/job-1")
}

func TestHandler_CreateLabelingJob_TagsRoundTripThroughDescribe(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := createLabelingJobRequestBody("job-tags", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1")
	body["Tags"] = []any{
		map[string]any{"Key": "env", "Value": "prod"},
	}

	rec := doSageMakerRequest(t, h, "CreateLabelingJob", body)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h, "DescribeLabelingJob", map[string]any{"LabelingJobName": "job-tags"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	tags, ok := resp["Tags"].([]any)
	require.True(t, ok, "DescribeLabelingJob must return Tags")
	require.Len(t, tags, 1)
	assert.Equal(t, "env", tags[0].(map[string]any)["Key"])
	assert.Equal(t, "prod", tags[0].(map[string]any)["Value"])
}

func TestHandler_CreateLabelingJob_MissingRequiredFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		body map[string]any
		name string
	}{
		{name: "missing name", body: map[string]any{}},
		{name: "missing InputConfig", body: map[string]any{"LabelingJobName": "x", "RoleArn": "r"}},
		{
			name: "missing LabelAttributeName",
			body: func() map[string]any {
				b := createLabelingJobRequestBody(
					"no-label-attr",
					"arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1",
				)
				delete(b, "LabelAttributeName")

				return b
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doSageMakerRequest(t, h, "CreateLabelingJob", tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHandler_DescribeLabelingJob_Lifecycle(t *testing.T) {
	t.Parallel()

	// CreateLabelingJob schedules its Initializing->InProgress->Completed
	// transitions immediately, so the whole body stays in one bubble.
	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		doSageMakerRequest(
			t,
			h,
			"CreateLabelingJob",
			createLabelingJobRequestBody("job-lifecycle", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1"),
		)

		rec := doSageMakerRequest(t, h, "DescribeLabelingJob", map[string]any{"LabelingJobName": "job-lifecycle"})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "job-lifecycle", resp["LabelingJobName"])
		assert.Equal(t, "Initializing", resp["LabelingJobStatus"])
		assert.NotEmpty(t, resp["JobReferenceCode"])

		humanTaskConfig, ok := resp["HumanTaskConfig"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1", humanTaskConfig["WorkteamArn"])

		time.Sleep(time.Second)
		synctest.Wait()

		rec = doSageMakerRequest(t, h, "DescribeLabelingJob", map[string]any{"LabelingJobName": "job-lifecycle"})
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "Completed", resp["LabelingJobStatus"])
	})
}

func TestHandler_DescribeLabelingJob_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "DescribeLabelingJob", map[string]any{"LabelingJobName": "missing"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ValidationException", resp["__type"])
}

func TestHandler_StopLabelingJob(t *testing.T) {
	t.Parallel()

	// CreateLabelingJob schedules its own transitions immediately, so the
	// whole body stays in one bubble.
	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		doSageMakerRequest(
			t,
			h,
			"CreateLabelingJob",
			createLabelingJobRequestBody("job-stop", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1"),
		)

		rec := doSageMakerRequest(t, h, "StopLabelingJob", map[string]any{"LabelingJobName": "job-stop"})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = doSageMakerRequest(t, h, "DescribeLabelingJob", map[string]any{"LabelingJobName": "job-stop"})
		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Contains(t, []any{"Stopping", "Stopped"}, resp["LabelingJobStatus"])

		time.Sleep(time.Second)
		synctest.Wait()

		rec = doSageMakerRequest(t, h, "DescribeLabelingJob", map[string]any{"LabelingJobName": "job-stop"})
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "Stopped", resp["LabelingJobStatus"])
	})
}

func TestHandler_StopLabelingJob_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "StopLabelingJob", map[string]any{"LabelingJobName": "missing"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_ListLabelingJobs(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(
		t,
		h,
		"CreateLabelingJob",
		createLabelingJobRequestBody("list-job-a", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1"),
	)
	doSageMakerRequest(
		t,
		h,
		"CreateLabelingJob",
		createLabelingJobRequestBody("list-job-b", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1"),
	)

	rec := doSageMakerRequest(t, h, "ListLabelingJobs", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	items, ok := resp["LabelingJobSummaryList"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 2)

	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1", first["WorkteamArn"])

	// NameContains filter.
	rec = doSageMakerRequest(t, h, "ListLabelingJobs", map[string]any{"NameContains": "list-job-a"})
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	items, _ = resp["LabelingJobSummaryList"].([]any)
	assert.Len(t, items, 1)
}

func TestHandler_ListLabelingJobsForWorkteam(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(
		t,
		h,
		"CreateLabelingJob",
		createLabelingJobRequestBody("wt-job-a", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-x"),
	)
	doSageMakerRequest(
		t,
		h,
		"CreateLabelingJob",
		createLabelingJobRequestBody("wt-job-b", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-y"),
	)

	rec := doSageMakerRequest(t, h, "ListLabelingJobsForWorkteam", map[string]any{
		"WorkteamArn": "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-x",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	items, ok := resp["LabelingJobSummaryList"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)

	item, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "wt-job-a", item["LabelingJobName"])
	assert.NotEmpty(t, item["JobReferenceCode"])
}

func TestHandler_ListLabelingJobsForWorkteam_MissingArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "ListLabelingJobsForWorkteam", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_ListLabelingJobs_ResponseIncludesInputConfigAndOutput(t *testing.T) {
	t.Parallel()

	// CreateLabelingJob schedules its own transitions immediately, so the
	// whole body stays in one bubble.
	synctest.Test(t, func(t *testing.T) {
		h := newTestHandler(t)

		doSageMakerRequest(
			t,
			h,
			"CreateLabelingJob",
			createLabelingJobRequestBody("job-fields", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1"),
		)

		rec := doSageMakerRequest(t, h, "ListLabelingJobs", map[string]any{})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		items, ok := resp["LabelingJobSummaryList"].([]any)
		require.True(t, ok)
		require.Len(t, items, 1)

		item, ok := items[0].(map[string]any)
		require.True(t, ok)
		assert.NotNil(t, item["InputConfig"], "LabelingJobSummary.InputConfig is a real member of the wire type")

		// LabelingJobOutput is only populated once the job completes.
		time.Sleep(time.Second)
		synctest.Wait()

		rec = doSageMakerRequest(t, h, "ListLabelingJobs", map[string]any{})
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		items, ok = resp["LabelingJobSummaryList"].([]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.NotNil(t, items[0].(map[string]any)["LabelingJobOutput"])
	})
}

func TestHandler_ListLabelingJobs_TimeFilters(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(
		t,
		h,
		"CreateLabelingJob",
		createLabelingJobRequestBody("time-job", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1"),
	)

	future := float64(time.Now().Add(time.Hour).Unix())
	past := float64(time.Now().Add(-time.Hour).Unix())

	tests := []struct {
		body      map[string]any
		name      string
		wantCount int
	}{
		{name: "creation time after future excludes", body: map[string]any{"CreationTimeAfter": future}, wantCount: 0},
		{name: "creation time after past includes", body: map[string]any{"CreationTimeAfter": past}, wantCount: 1},
		{name: "creation time before past excludes", body: map[string]any{"CreationTimeBefore": past}, wantCount: 0},
		{
			name:      "creation time before future includes",
			body:      map[string]any{"CreationTimeBefore": future},
			wantCount: 1,
		},
		{
			name:      "last modified after future excludes",
			body:      map[string]any{"LastModifiedTimeAfter": future},
			wantCount: 0,
		},
		{
			name:      "last modified before past excludes",
			body:      map[string]any{"LastModifiedTimeBefore": past},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doSageMakerRequest(t, h, "ListLabelingJobs", tc.body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			items, _ := resp["LabelingJobSummaryList"].([]any)
			assert.Len(t, items, tc.wantCount)
		})
	}
}

func TestHandler_ListLabelingJobs_SortBy(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(
		t,
		h,
		"CreateLabelingJob",
		createLabelingJobRequestBody("sort-b", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1"),
	)
	doSageMakerRequest(
		t,
		h,
		"CreateLabelingJob",
		createLabelingJobRequestBody("sort-a", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-1"),
	)

	tests := []struct {
		body      map[string]any
		name      string
		wantFirst string
	}{
		{
			name:      "sort by name ascending",
			body:      map[string]any{"SortBy": "Name", "SortOrder": "Ascending"},
			wantFirst: "sort-a",
		},
		{
			name:      "sort by name descending",
			body:      map[string]any{"SortBy": "Name", "SortOrder": "Descending"},
			wantFirst: "sort-b",
		},
		{name: "default sort is creation time ascending", body: map[string]any{}, wantFirst: "sort-b"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doSageMakerRequest(t, h, "ListLabelingJobs", tc.body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			items, ok := resp["LabelingJobSummaryList"].([]any)
			require.True(t, ok)
			require.Len(t, items, 2)
			assert.Equal(t, tc.wantFirst, items[0].(map[string]any)["LabelingJobName"])
		})
	}
}

func TestHandler_ListLabelingJobsForWorkteam_Filters(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(
		t,
		h,
		"CreateLabelingJob",
		createLabelingJobRequestBody("wtf-job", "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-filter"),
	)

	describeRec := doSageMakerRequest(t, h, "DescribeLabelingJob", map[string]any{"LabelingJobName": "wtf-job"})
	require.Equal(t, http.StatusOK, describeRec.Code)

	var describeResp map[string]any
	require.NoError(t, json.Unmarshal(describeRec.Body.Bytes(), &describeResp))
	jobReferenceCode, ok := describeResp["JobReferenceCode"].(string)
	require.True(t, ok)
	require.NotEmpty(t, jobReferenceCode)

	future := float64(time.Now().Add(time.Hour).Unix())

	tests := []struct {
		extra     map[string]any
		name      string
		wantCount int
	}{
		{
			name:      "matching job reference code contains",
			extra:     map[string]any{"JobReferenceCodeContains": jobReferenceCode[:4]},
			wantCount: 1,
		},
		{
			name:      "non-matching job reference code contains",
			extra:     map[string]any{"JobReferenceCodeContains": "does-not-exist"},
			wantCount: 0,
		},
		{name: "creation time after future excludes", extra: map[string]any{"CreationTimeAfter": future}, wantCount: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := map[string]any{"WorkteamArn": "arn:aws:sagemaker:us-east-1:000000000000:workteam/team-filter"}
			maps.Copy(body, tc.extra)

			rec := doSageMakerRequest(t, h, "ListLabelingJobsForWorkteam", body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			items, _ := resp["LabelingJobSummaryList"].([]any)
			assert.Len(t, items, tc.wantCount)
		})
	}
}

// ---------------------------------------------------------------------------
// Workforce (Delete/List; Create/Describe/Update covered in handler_batch3_test.go)
// ---------------------------------------------------------------------------

func TestHandler_ListWorkforces(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateWorkforce", map[string]any{"WorkforceName": "wf-list"})

	rec := doSageMakerRequest(t, h, "ListWorkforces", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	items, ok := resp["Workforces"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)

	wf, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "wf-list", wf["WorkforceName"])
	assert.Equal(t, "Active", wf["Status"])
	assert.NotEmpty(t, wf["CreateDate"])
}

func TestHandler_DeleteWorkforce(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateWorkforce", map[string]any{"WorkforceName": "wf-del"})
	rec := doSageMakerRequest(t, h, "DeleteWorkforce", map[string]any{"WorkforceName": "wf-del"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h, "DescribeWorkforce", map[string]any{"WorkforceName": "wf-del"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_DeleteWorkforce_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "DeleteWorkforce", map[string]any{"WorkforceName": "missing"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_DeleteWorkforce_ResourceInUse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateWorkforce", map[string]any{"WorkforceName": "wf-inuse"})
	doSageMakerRequest(t, h, "CreateWorkteam", map[string]any{
		"WorkteamName":  "team-blocking",
		"Description":   "blocks deletion",
		"WorkforceName": "wf-inuse",
	})

	rec := doSageMakerRequest(t, h, "DeleteWorkforce", map[string]any{"WorkforceName": "wf-inuse"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ResourceInUse", resp["__type"])
}

func TestHandler_CreateWorkforce_OnlyOnePerAccount(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateWorkforce", map[string]any{"WorkforceName": "wf-first"})
	rec := doSageMakerRequest(t, h, "CreateWorkforce", map[string]any{"WorkforceName": "wf-second"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_UpdateWorkforce_WithSourceIPConfig(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateWorkforce", map[string]any{"WorkforceName": "wf-update"})

	rec := doSageMakerRequest(t, h, "UpdateWorkforce", map[string]any{
		"WorkforceName":  "wf-update",
		"SourceIpConfig": map[string]any{"Cidrs": []string{"10.0.0.0/24"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wf, ok := resp["Workforce"].(map[string]any)
	require.True(t, ok)

	sourceIPConfig, ok := wf["SourceIpConfig"].(map[string]any)
	require.True(t, ok)
	cidrs, ok := sourceIPConfig["Cidrs"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"10.0.0.0/24"}, cidrs)
}

// ---------------------------------------------------------------------------
// Workteam (Update/WorkforceName linkage; Create/Describe/Delete/List covered
// in handler_batch2_test.go)
// ---------------------------------------------------------------------------

func TestHandler_CreateWorkteam_WithWorkforceName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateWorkforce", map[string]any{"WorkforceName": "wf-link"})
	rec := doSageMakerRequest(t, h, "CreateWorkteam", map[string]any{
		"WorkteamName":  "team-link",
		"WorkforceName": "wf-link",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h, "DescribeWorkteam", map[string]any{"WorkteamName": "team-link"})
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wt, ok := resp["Workteam"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, wt["WorkforceArn"], "workforce/wf-link")
	assert.NotEmpty(t, wt["SubDomain"])
}

func TestHandler_CreateWorkteam_UnknownWorkforceName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "CreateWorkteam", map[string]any{
		"WorkteamName":  "team-orphan",
		"WorkforceName": "does-not-exist",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_UpdateWorkteam(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateWorkteam", map[string]any{
		"WorkteamName": "team-update",
		"Description":  "before",
	})

	rec := doSageMakerRequest(t, h, "UpdateWorkteam", map[string]any{
		"WorkteamName": "team-update",
		"Description":  "after",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	wt, ok := resp["Workteam"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "after", wt["Description"])
}

func TestHandler_UpdateWorkteam_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "UpdateWorkteam", map[string]any{"WorkteamName": "missing"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_DeleteWorkteam_ReturnsSuccess(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateWorkteam", map[string]any{"WorkteamName": "team-success"})
	rec := doSageMakerRequest(t, h, "DeleteWorkteam", map[string]any{"WorkteamName": "team-success"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["Success"])
}

// ---------------------------------------------------------------------------
// Subscribed workteams (unmodeled, no-create resource)
// ---------------------------------------------------------------------------

func TestHandler_ListSubscribedWorkteams_AlwaysEmpty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "ListSubscribedWorkteams", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	items, ok := resp["SubscribedWorkteams"].([]any)
	require.True(t, ok)
	assert.Empty(t, items)
	assert.NotContains(t, resp, "NextToken")
}

func TestHandler_DescribeSubscribedWorkteam_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "DescribeSubscribedWorkteam", map[string]any{
		"WorkteamArn": "arn:aws:sagemaker:us-east-1:000000000000:workteam/vendor-team",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ValidationException", resp["__type"])
}

func TestHandler_DescribeSubscribedWorkteam_MissingArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doSageMakerRequest(t, h, "DescribeSubscribedWorkteam", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ---------------------------------------------------------------------------
// Persistence round-trip
// ---------------------------------------------------------------------------

func TestLabelingFamily_SnapshotRestore(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	require.Equal(t, http.StatusOK,
		doSageMakerRequest(t, h, "CreateWorkforce", map[string]any{"WorkforceName": "persist-wf"}).Code,
	)
	require.Equal(t, http.StatusOK,
		doSageMakerRequest(t, h, "CreateWorkteam", map[string]any{
			"WorkteamName":  "persist-team",
			"WorkforceName": "persist-wf",
		}).Code,
	)
	require.Equal(t, http.StatusOK,
		doSageMakerRequest(
			t,
			h,
			"CreateLabelingJob",
			createLabelingJobRequestBody(
				"persist-job",
				"arn:aws:sagemaker:us-east-1:000000000000:workteam/persist-team",
			),
		).Code,
	)

	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	h2 := newTestHandler(t)
	require.NoError(t, h2.Restore(t.Context(), snap))

	rec := doSageMakerRequest(t, h2, "DescribeWorkforce", map[string]any{"WorkforceName": "persist-wf"})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doSageMakerRequest(t, h2, "DescribeWorkteam", map[string]any{"WorkteamName": "persist-team"})
	require.Equal(t, http.StatusOK, rec.Code)

	var wtResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &wtResp))
	wt, ok := wtResp["Workteam"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, wt["WorkforceArn"], "workforce/persist-wf")

	rec = doSageMakerRequest(t, h2, "DescribeLabelingJob", map[string]any{"LabelingJobName": "persist-job"})
	require.Equal(t, http.StatusOK, rec.Code)

	var jobResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &jobResp))
	assert.Equal(t, "persist-job", jobResp["LabelingJobName"])
}
