package translate_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/translate"
)

func createTestJob(t *testing.T, h *translate.Handler) string {
	t.Helper()

	rec := doRequest(t, h, "StartTextTranslationJob", map[string]any{
		"JobName":             "state-guard-job",
		"SourceLanguageCode":  "en",
		"TargetLanguageCodes": []string{"fr"},
		"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/TranslateRole",
		"InputDataConfig":     map[string]any{"S3Uri": "s3://b/i/", "ContentType": "text/plain"},
		"OutputDataConfig":    map[string]any{"S3Uri": "s3://b/o/"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	return unmarshalJSON(t, rec.Body.Bytes())["JobId"].(string)
}

func TestStartTextTranslationJob(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "StartTextTranslationJob", map[string]any{
		"JobName":             "batch-job-1",
		"SourceLanguageCode":  "en",
		"TargetLanguageCodes": []string{"fr", "de"},
		"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/TranslateRole",
		"InputDataConfig": map[string]any{
			"S3Uri":       "s3://bucket/input/",
			"ContentType": "text/plain",
		},
		"OutputDataConfig": map[string]any{
			"S3Uri": "s3://bucket/output/",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := unmarshalJSON(t, rec.Body.Bytes())
	assert.NotEmpty(t, m["JobId"])
	assert.Equal(t, "SUBMITTED", m["JobStatus"])
}

func TestDescribeTextTranslationJob(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "StartTextTranslationJob", map[string]any{
		"JobName":             "describe-test",
		"SourceLanguageCode":  "en",
		"TargetLanguageCodes": []string{"es"},
		"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/TranslateRole",
		"InputDataConfig":     map[string]any{"S3Uri": "s3://b/i/", "ContentType": "text/plain"},
		"OutputDataConfig":    map[string]any{"S3Uri": "s3://b/o/"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	jobID := unmarshalJSON(t, rec.Body.Bytes())["JobId"].(string)

	rec = doRequest(t, h, "DescribeTextTranslationJob", map[string]any{"JobId": jobID})
	assert.Equal(t, http.StatusOK, rec.Code)

	m := unmarshalJSON(t, rec.Body.Bytes())
	props, ok := m["TextTranslationJobProperties"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, jobID, props["JobId"])
	assert.Equal(t, "describe-test", props["JobName"])
}

func TestStopTextTranslationJob(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "StartTextTranslationJob", map[string]any{
		"JobName":             "stop-test",
		"SourceLanguageCode":  "en",
		"TargetLanguageCodes": []string{"fr"},
		"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/TranslateRole",
		"InputDataConfig":     map[string]any{"S3Uri": "s3://b/i/", "ContentType": "text/plain"},
		"OutputDataConfig":    map[string]any{"S3Uri": "s3://b/o/"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	jobID := unmarshalJSON(t, rec.Body.Bytes())["JobId"].(string)

	rec = doRequest(t, h, "StopTextTranslationJob", map[string]any{"JobId": jobID})
	assert.Equal(t, http.StatusOK, rec.Code)

	m := unmarshalJSON(t, rec.Body.Bytes())
	assert.Equal(t, "STOP_REQUESTED", m["JobStatus"])
}

func TestListTextTranslationJobs(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 3 {
		rec := doRequest(t, h, "StartTextTranslationJob", map[string]any{
			"JobName":             "job-" + string(rune('a'+i)),
			"SourceLanguageCode":  "en",
			"TargetLanguageCodes": []string{"fr"},
			"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/TranslateRole",
			"InputDataConfig":     map[string]any{"S3Uri": "s3://b/i/", "ContentType": "text/plain"},
			"OutputDataConfig":    map[string]any{"S3Uri": "s3://b/o/"},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec := doRequest(t, h, "ListTextTranslationJobs", map[string]any{})
	assert.Equal(t, http.StatusOK, rec.Code)

	m := unmarshalJSON(t, rec.Body.Bytes())
	jobs, _ := m["TextTranslationJobPropertiesList"].([]any)
	assert.Len(t, jobs, 3)
}

// TestStopTextTranslationJob_Idempotent replaces the former
// TestStopTextTranslationJob_StateGuard, which asserted that stopping an
// already STOP_REQUESTED job returns InvalidRequestException. That was
// wrong: StopTextTranslationJob's modeled error set is
// ResourceNotFoundException, TooManyRequestsException, and
// InternalServerException only -- no InvalidRequestException or
// InvalidParameterValueException at all
// (aws-sdk-go-v2/service/translate@v1.36.4's
// awsAwsjson11_deserializeOpErrorStopTextTranslationJob) -- so a job that
// isn't stoppable is not a client error the real operation can raise. Stop
// is idempotent: calling it on a job that is not IN_PROGRESS or SUBMITTED
// just reports the job's current status back, unchanged.
func TestStopTextTranslationJob_Idempotent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *translate.Handler, jobID string)
		name       string
		wantStatus string
	}{
		{
			name:       "already_stop_requested",
			wantStatus: "STOP_REQUESTED",
			setup: func(t *testing.T, h *translate.Handler, jobID string) {
				t.Helper()
				rec := doRequest(t, h, "StopTextTranslationJob", map[string]any{"JobId": jobID})
				require.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name:       "already_completed",
			wantStatus: "COMPLETED",
			setup: func(t *testing.T, h *translate.Handler, jobID string) {
				t.Helper()
				// SUBMITTED -> IN_PROGRESS -> COMPLETED (advanceJob moves one
				// step per DescribeTextTranslationJob poll).
				doRequest(t, h, "DescribeTextTranslationJob", map[string]any{"JobId": jobID})
				rec := doRequest(t, h, "DescribeTextTranslationJob", map[string]any{"JobId": jobID})
				require.Equal(t, http.StatusOK, rec.Code)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			jobID := createTestJob(t, h)
			tc.setup(t, h, jobID)

			rec := doRequest(t, h, "StopTextTranslationJob", map[string]any{"JobId": jobID})
			require.Equal(t, http.StatusOK, rec.Code)

			m := unmarshalJSON(t, rec.Body.Bytes())
			assert.Equal(t, tc.wantStatus, m["JobStatus"])
		})
	}
}

// TestStopTextTranslationJob_NotFound verifies that stopping a nonexistent
// job returns ResourceNotFoundException.
func TestStopTextTranslationJob_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "StopTextTranslationJob", map[string]any{"JobId": "nonexistent-job-id"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ResourceNotFoundException", body["__type"])
}

// TestDescribeTextTranslationJob_NotFound verifies that describing a
// nonexistent job returns ResourceNotFoundException.
func TestDescribeTextTranslationJob_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "DescribeTextTranslationJob", map[string]any{"JobId": "no-such-job"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ResourceNotFoundException", body["__type"])
}

// TestDescribeTextTranslationJob_TerminologyAndParallelDataFields verifies
// that TerminologyNames and ParallelDataNames are preserved and returned in
// job details. StartTextTranslationJob validates that referenced
// TerminologyNames/ParallelDataNames exist (real AWS models
// ResourceNotFoundException for exactly this -- the only named-resource
// lookup the operation performs), so both must be created first.
func TestDescribeTextTranslationJob_TerminologyAndParallelDataFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	termRec := doRequest(t, h, "ImportTerminology", map[string]any{
		"Name": "my-term", "MergeStrategy": "OVERWRITE",
		"TerminologyData": map[string]any{"Format": "CSV"},
	})
	require.Equal(t, http.StatusOK, termRec.Code)

	pdRec := doRequest(t, h, "CreateParallelData", map[string]any{
		"Name":               "my-pd",
		"ParallelDataConfig": map[string]any{"S3Uri": "s3://b/f.tmx", "Format": "TMX"},
	})
	require.Equal(t, http.StatusOK, pdRec.Code)

	rec := doRequest(t, h, "StartTextTranslationJob", map[string]any{
		"JobName":             "fields-test",
		"SourceLanguageCode":  "en",
		"TargetLanguageCodes": []string{"fr"},
		"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/TranslateRole",
		"InputDataConfig":     map[string]any{"S3Uri": "s3://b/i/", "ContentType": "text/plain"},
		"OutputDataConfig":    map[string]any{"S3Uri": "s3://b/o/"},
		"TerminologyNames":    []string{"my-term"},
		"ParallelDataNames":   []string{"my-pd"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	jobID := unmarshalJSON(t, rec.Body.Bytes())["JobId"].(string)

	rec = doRequest(t, h, "DescribeTextTranslationJob", map[string]any{"JobId": jobID})
	require.Equal(t, http.StatusOK, rec.Code)

	props := unmarshalJSON(t, rec.Body.Bytes())["TextTranslationJobProperties"].(map[string]any)
	termNames, _ := props["TerminologyNames"].([]any)
	pdNames, _ := props["ParallelDataNames"].([]any)
	assert.Equal(t, []any{"my-term"}, termNames)
	assert.Equal(t, []any{"my-pd"}, pdNames)
}

// TestStartTextTranslationJob_UnknownTerminologyRejected verifies that
// referencing a TerminologyNames or ParallelDataNames entry that does not
// exist returns ResourceNotFoundException, matching real AWS: "Use the
// ListTerminologies/ListParallelData operation to get the available
// resources" implies StartTextTranslationJob validates the reference.
func TestStartTextTranslationJob_UnknownTerminologyRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body map[string]any
		name string
	}{
		{
			name: "unknown_terminology_name",
			body: map[string]any{"TerminologyNames": []string{"no-such-term"}},
		},
		{
			name: "unknown_parallel_data_name",
			body: map[string]any{"ParallelDataNames": []string{"no-such-pd"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			body := map[string]any{
				"JobName":             "bad-ref-job",
				"SourceLanguageCode":  "en",
				"TargetLanguageCodes": []string{"fr"},
				"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/r",
				"InputDataConfig":     map[string]any{"S3Uri": "s3://b/i/", "ContentType": "text/plain"},
				"OutputDataConfig":    map[string]any{"S3Uri": "s3://b/o/"},
			}
			maps.Copy(body, tt.body)

			rec := doRequest(t, h, "StartTextTranslationJob", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var respBody map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &respBody))
			assert.Equal(t, "ResourceNotFoundException", respBody["__type"])
		})
	}
}

// TestStartTextTranslationJob_RequiredFieldsValidated verifies that omitting
// any of StartTextTranslationJobRequest's required members (DataAccessRoleArn,
// InputDataConfig, OutputDataConfig, SourceLanguageCode, TargetLanguageCodes)
// is rejected as InvalidRequestException instead of silently creating a job
// with a hole in its configuration.
func TestStartTextTranslationJob_RequiredFieldsValidated(t *testing.T) {
	t.Parallel()

	full := func() map[string]any {
		return map[string]any{
			"JobName":             "req-fields-job",
			"SourceLanguageCode":  "en",
			"TargetLanguageCodes": []string{"fr"},
			"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/r",
			"InputDataConfig":     map[string]any{"S3Uri": "s3://b/i/", "ContentType": "text/plain"},
			"OutputDataConfig":    map[string]any{"S3Uri": "s3://b/o/"},
		}
	}

	tests := []struct {
		name   string
		remove string
	}{
		{name: "missing_data_access_role_arn", remove: "DataAccessRoleArn"},
		{name: "missing_input_data_config", remove: "InputDataConfig"},
		{name: "missing_output_data_config", remove: "OutputDataConfig"},
		{name: "missing_source_language_code", remove: "SourceLanguageCode"},
		{name: "missing_target_language_codes", remove: "TargetLanguageCodes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := full()
			delete(body, tt.remove)

			rec := doRequest(t, h, "StartTextTranslationJob", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var respBody map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &respBody))
			assert.Equal(t, "InvalidRequestException", respBody["__type"])
		})
	}
}

// TestStartTextTranslationJob_UnsupportedLanguagePairRejected verifies that
// an unrecognized source or target language code is rejected as
// UnsupportedLanguagePairException, and that "auto" is always accepted as a
// source language.
func TestStartTextTranslationJob_UnsupportedLanguagePairRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name:     "unknown_source_language",
			body:     map[string]any{"SourceLanguageCode": "xx"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "unknown_target_language",
			body:     map[string]any{"TargetLanguageCodes": []string{"xx"}},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "auto_source_accepted",
			body:     map[string]any{"SourceLanguageCode": "auto"},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := map[string]any{
				"JobName":             "lang-pair-job",
				"SourceLanguageCode":  "en",
				"TargetLanguageCodes": []string{"fr"},
				"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/r",
				"InputDataConfig":     map[string]any{"S3Uri": "s3://b/i/", "ContentType": "text/plain"},
				"OutputDataConfig":    map[string]any{"S3Uri": "s3://b/o/"},
			}
			maps.Copy(body, tt.body)

			rec := doRequest(t, h, "StartTextTranslationJob", body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusBadRequest {
				var respBody map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &respBody))
				assert.Equal(t, "UnsupportedLanguagePairException", respBody["__type"])
			}
		})
	}
}

// TestListTextTranslationJobs_StatusFilter verifies that filtering by
// JobStatus returns only jobs with the matching status.
func TestListTextTranslationJobs_StatusFilter(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 3 {
		rec := doRequest(t, h, "StartTextTranslationJob", map[string]any{
			"JobName":             "filter-job-" + string(rune('a'+i)),
			"SourceLanguageCode":  "en",
			"TargetLanguageCodes": []string{"fr"},
			"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/TranslateRole",
			"InputDataConfig":     map[string]any{"S3Uri": "s3://b/i/", "ContentType": "text/plain"},
			"OutputDataConfig":    map[string]any{"S3Uri": "s3://b/o/"},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec := doRequest(t, h, "ListTextTranslationJobs", map[string]any{
		"Filter": map[string]any{"JobStatus": "SUBMITTED"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := unmarshalJSON(t, rec.Body.Bytes())
	jobs, _ := m["TextTranslationJobPropertiesList"].([]any)
	assert.Len(t, jobs, 3)

	rec = doRequest(t, h, "ListTextTranslationJobs", map[string]any{
		"Filter": map[string]any{"JobStatus": "COMPLETED"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m = unmarshalJSON(t, rec.Body.Bytes())
	jobs, _ = m["TextTranslationJobPropertiesList"].([]any)
	assert.Empty(t, jobs)
}

// TestStopTextTranslationJob_SetsEndTime verifies StopTextTranslationJob
// sets an EndTime on the job.
func TestStopTextTranslationJob_SetsEndTime(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	startRec := doRequest(t, h, "StartTextTranslationJob", map[string]any{
		"JobName":             "stop-test",
		"SourceLanguageCode":  "en",
		"TargetLanguageCodes": []string{"es"},
		"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/r",
		"InputDataConfig": map[string]any{
			"S3Uri":       "s3://bucket/input/",
			"ContentType": "text/plain",
		},
		"OutputDataConfig": map[string]any{
			"S3Uri": "s3://bucket/output/",
		},
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	startResp := unmarshalJSON(t, startRec.Body.Bytes())
	jobID := startResp["JobId"].(string)

	stopRec := doRequest(t, h, "StopTextTranslationJob", map[string]any{"JobId": jobID})
	require.Equal(t, http.StatusOK, stopRec.Code)

	descRec := doRequest(t, h, "DescribeTextTranslationJob", map[string]any{"JobId": jobID})
	require.Equal(t, http.StatusOK, descRec.Code)

	descResp := unmarshalJSON(t, descRec.Body.Bytes())
	job := descResp["TextTranslationJobProperties"].(map[string]any)
	endTime, hasEndTime := job["EndTime"]
	assert.True(t, hasEndTime, "EndTime must be present after StopTextTranslationJob")
	assert.NotNil(t, endTime)
}

// TestJobToMap_IncludesJobDetails verifies DescribeTextTranslationJob
// response includes the JobDetails field with document counts.
func TestJobToMap_IncludesJobDetails(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	startRec := doRequest(t, h, "StartTextTranslationJob", map[string]any{
		"JobName":             "details-test",
		"SourceLanguageCode":  "en",
		"TargetLanguageCodes": []string{"fr"},
		"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/r",
		"InputDataConfig": map[string]any{
			"S3Uri":       "s3://bucket/input/",
			"ContentType": "text/plain",
		},
		"OutputDataConfig": map[string]any{
			"S3Uri": "s3://bucket/output/",
		},
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	startResp := unmarshalJSON(t, startRec.Body.Bytes())
	jobID := startResp["JobId"].(string)

	descRec := doRequest(t, h, "DescribeTextTranslationJob", map[string]any{"JobId": jobID})
	require.Equal(t, http.StatusOK, descRec.Code)

	descResp := unmarshalJSON(t, descRec.Body.Bytes())
	job := descResp["TextTranslationJobProperties"].(map[string]any)
	details, hasDetails := job["JobDetails"]
	require.True(t, hasDetails, "JobDetails must be present in job response")
	d := details.(map[string]any)
	assert.Contains(t, d, "TranslatedDocumentsCount")
	assert.Contains(t, d, "DocumentsWithErrorsCount")
	assert.Contains(t, d, "InputDocumentsCount")
}

// TestListTextTranslationJobs_InvalidFilterRejected verifies that an
// unrecognized Filter.JobStatus value is rejected as InvalidFilterException
// rather than silently matching zero jobs.
func TestListTextTranslationJobs_InvalidFilterRejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "ListTextTranslationJobs", map[string]any{
		"Filter": map[string]any{"JobStatus": "NOT_A_REAL_STATUS"},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "InvalidFilterException", body["__type"])
}

// TestListTextTranslationJobs_StatusFilterMinCount verifies filtering by
// JobStatus using a threshold check (GreaterOrEqual) rather than an exact
// count, complementing TestListTextTranslationJobs_StatusFilter's exact-count
// assertions with a scenario using fewer seeded jobs and named subtests.
func TestListTextTranslationJobs_StatusFilterMinCount(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	startBody := func(name string) map[string]any {
		return map[string]any{
			"JobName":             name,
			"SourceLanguageCode":  "en",
			"TargetLanguageCodes": []string{"es"},
			"DataAccessRoleArn":   "arn:aws:iam::000000000000:role/r",
			"InputDataConfig": map[string]any{
				"S3Uri":       "s3://b/i/",
				"ContentType": "text/plain",
			},
			"OutputDataConfig": map[string]any{"S3Uri": "s3://b/o/"},
		}
	}

	for _, name := range []string{"job-a", "job-b"} {
		rec := doRequest(t, h, "StartTextTranslationJob", startBody(name))
		require.Equal(t, http.StatusOK, rec.Code)
	}

	tests := []struct {
		name    string
		status  string
		wantMin int
	}{
		{name: "submitted_filter", status: "SUBMITTED", wantMin: 2},
		{name: "completed_filter", status: "COMPLETED", wantMin: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := map[string]any{}
			if tt.status != "" {
				body["Filter"] = map[string]any{"JobStatus": tt.status}
			}

			rec := doRequest(t, h, "ListTextTranslationJobs", body)
			require.Equal(t, http.StatusOK, rec.Code)

			resp := unmarshalJSON(t, rec.Body.Bytes())
			jobs, ok := resp["TextTranslationJobPropertiesList"].([]any)
			require.True(t, ok)
			assert.GreaterOrEqual(t, len(jobs), tt.wantMin)
		})
	}
}
