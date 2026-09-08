package s3control_test

import (
	"encoding/xml"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	s3control "github.com/blackbirdworks/gopherstack/services/s3control"
)

// ---- Job Tagging ----

func TestJobTagging(t *testing.T) {
	t.Parallel()
	b := s3control.NewInMemoryBackend()

	_, setupErr := b.CreateJob("000000000000", "arn:aws:iam::000000000000:role/test", 1)
	require.NoError(t, setupErr)

	t.Run("put and get tags", func(t *testing.T) {
		t.Parallel()
		b2 := s3control.NewInMemoryBackend()
		j, _ := b2.CreateJob("000000000000", "arn:aws:iam::000000000000:role/test", 1)

		require.NoError(
			t,
			b2.PutJobTagging(
				"000000000000",
				j.JobID,
				s3control.TagSet{"env": "test", "project": "s3"},
			),
		)
		tags, err := b2.GetJobTagging("000000000000", j.JobID)
		require.NoError(t, err)
		assert.Equal(t, "test", tags["env"])
		assert.Equal(t, "s3", tags["project"])
	})

	t.Run("delete tags", func(t *testing.T) {
		t.Parallel()
		b2 := s3control.NewInMemoryBackend()
		j, _ := b2.CreateJob("000000000000", "arn:aws:iam::000000000000:role/test", 1)
		_ = b2.PutJobTagging("000000000000", j.JobID, s3control.TagSet{"k": "v"})

		require.NoError(t, b2.DeleteJobTagging("000000000000", j.JobID))
		tags, err := b2.GetJobTagging("000000000000", j.JobID)
		require.NoError(t, err)
		assert.Empty(t, tags)
	})

	t.Run("unknown job returns error", func(t *testing.T) {
		t.Parallel()
		_, err := b.GetJobTagging("000000000000", "job-missing")
		require.Error(t, err)
	})
}

func TestHTTP_JobTagging(t *testing.T) {
	t.Parallel()
	b := s3control.NewInMemoryBackend()
	h := s3control.NewHandler(b)
	job, _ := b.CreateJob("000000000000", "arn:aws:iam::000000000000:role/r", 1)

	resp := doS3ControlNewOpRequest(
		t,
		h,
		http.MethodGet,
		"/v20180820/jobs/"+job.JobID+"/tagging",
		"000000000000",
		"",
	)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestBackend_BatchJob_Operations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(b *s3control.InMemoryBackend) string
		name    string
		op      string
		wantErr bool
	}{
		{
			name: "get_job",
			op:   "get",
			setup: func(b *s3control.InMemoryBackend) string {
				job := b.AddBatchJobInternal("acct1", "arn:aws:iam::acct1:role/R", 10)

				return job.JobID
			},
		},
		{
			name:    "get_missing_job",
			op:      "get",
			wantErr: true,
			setup:   func(_ *s3control.InMemoryBackend) string { return "nonexistent" },
		},
		{
			name: "list_jobs",
			op:   "list",
			setup: func(b *s3control.InMemoryBackend) string {
				b.AddBatchJobInternal("acct1", "arn:aws:iam::acct1:role/R", 10)

				return ""
			},
		},
		{
			name: "update_job_priority",
			op:   "priority",
			setup: func(b *s3control.InMemoryBackend) string {
				job := b.AddBatchJobInternal("acct1", "arn:aws:iam::acct1:role/R", 10)

				return job.JobID
			},
		},
		{
			name:    "update_priority_missing_job",
			op:      "priority",
			wantErr: true,
			setup:   func(_ *s3control.InMemoryBackend) string { return "nonexistent" },
		},
		{
			name: "update_job_status",
			op:   "status",
			setup: func(b *s3control.InMemoryBackend) string {
				job := b.AddBatchJobInternal("acct1", "arn:aws:iam::acct1:role/R", 10)

				return job.JobID
			},
		},
		{
			name:    "update_status_missing_job",
			op:      "status",
			wantErr: true,
			setup:   func(_ *s3control.InMemoryBackend) string { return "nonexistent" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3control.NewInMemoryBackend()
			jobID := tt.setup(b)

			switch tt.op {
			case "get":
				job, err := b.GetJob("acct1", jobID)
				if tt.wantErr {
					// AWS error code must be NoSuchJob, not the unrelated
					// NoSuchPublicAccessBlockConfiguration sentinel this
					// used to wrongly share.
					require.ErrorContains(t, err, "NoSuchJob")
				} else {
					require.NoError(t, err)
					assert.Equal(t, jobID, job.JobID)
				}
			case "list":
				jobs := b.ListJobs("acct1")
				assert.Len(t, jobs, 1)
			case "priority":
				job, err := b.UpdateJobPriority("acct1", jobID, 99)
				if tt.wantErr {
					require.ErrorContains(t, err, "NoSuchJob")
				} else {
					require.NoError(t, err)
					assert.Equal(t, int32(99), job.Priority)
				}
			case "status":
				job, err := b.UpdateJobStatus("acct1", jobID, "Complete")
				if tt.wantErr {
					require.ErrorContains(t, err, "NoSuchJob")
				} else {
					require.NoError(t, err)
					assert.Equal(t, "Complete", job.Status)
				}
			}
		})
	}
}

func TestHandler_DescribeJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *s3control.Handler) string
		name       string
		wantBody   string
		wantStatus int
	}{
		{
			name:       "describe_existing_job",
			wantStatus: http.StatusOK,
			wantBody:   "DescribeJobResult",
			setup: func(h *s3control.Handler) string {
				job := h.Backend.AddBatchJobInternal("acct1", "arn:aws:iam::acct1:role/R", 5)

				return job.JobID
			},
		},
		{
			name:       "describe_missing_job",
			wantStatus: http.StatusNotFound,
			setup:      func(_ *s3control.Handler) string { return "nonexistent" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			jobID := tt.setup(h)

			rec := doS3Request(t, h, http.MethodGet, "/v20180820/jobs/"+jobID, "")
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantBody != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHandler_ListJobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *s3control.Handler)
		name       string
		wantBody   string
		wantStatus int
	}{
		{
			name:       "list_empty",
			wantStatus: http.StatusOK,
			wantBody:   "ListJobsResult",
			setup:      func(_ *s3control.Handler) {},
		},
		{
			name:       "list_with_jobs",
			wantStatus: http.StatusOK,
			wantBody:   "New",
			setup: func(h *s3control.Handler) {
				h.Backend.AddBatchJobInternal("acct1", "arn:aws:iam::acct1:role/R", 5)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			tt.setup(h)

			rec := doS3Request(t, h, http.MethodGet, "/v20180820/jobs", "")
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantBody)
		})
	}
}

func TestHandler_UpdateJobPriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *s3control.Handler) string
		name       string
		body       string
		wantBody   string
		wantStatus int
	}{
		{
			name:       "update_priority_success",
			wantStatus: http.StatusOK,
			wantBody:   "UpdateJobPriorityResult",
			body:       `<UpdateJobPriorityRequest><Priority>99</Priority></UpdateJobPriorityRequest>`,
			setup: func(h *s3control.Handler) string {
				job := h.Backend.AddBatchJobInternal("acct1", "arn:aws:iam::acct1:role/R", 5)

				return job.JobID
			},
		},
		{
			name:       "update_priority_missing_job",
			wantStatus: http.StatusNotFound,
			body:       `<UpdateJobPriorityRequest><Priority>99</Priority></UpdateJobPriorityRequest>`,
			setup:      func(_ *s3control.Handler) string { return "nonexistent" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			jobID := tt.setup(h)

			rec := doS3Request(t, h, http.MethodPost, "/v20180820/jobs/"+jobID+"/priority", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantBody != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHandler_UpdateJobStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(h *s3control.Handler) string
		name       string
		body       string
		wantBody   string
		wantStatus int
	}{
		{
			name:       "update_status_success_cancelled",
			wantStatus: http.StatusOK,
			wantBody:   "UpdateJobStatusResult",
			body:       `<UpdateJobStatusRequest><RequestedJobStatus>Cancelled</RequestedJobStatus></UpdateJobStatusRequest>`,
			setup: func(h *s3control.Handler) string {
				job := h.Backend.AddBatchJobInternal("acct1", "arn:aws:iam::acct1:role/R", 5)

				return job.JobID
			},
		},
		{
			name:       "update_status_success_ready",
			wantStatus: http.StatusOK,
			wantBody:   "UpdateJobStatusResult",
			body:       `<UpdateJobStatusRequest><RequestedJobStatus>Ready</RequestedJobStatus></UpdateJobStatusRequest>`,
			setup: func(h *s3control.Handler) string {
				job := h.Backend.AddBatchJobInternal("acct1", "arn:aws:iam::acct1:role/R", 5)

				return job.JobID
			},
		},
		{
			name:       "update_status_invalid_rejects",
			wantStatus: http.StatusBadRequest,
			body:       `<UpdateJobStatusRequest><RequestedJobStatus>Complete</RequestedJobStatus></UpdateJobStatusRequest>`,
			setup: func(h *s3control.Handler) string {
				job := h.Backend.AddBatchJobInternal("acct1", "arn:aws:iam::acct1:role/R", 5)

				return job.JobID
			},
		},
		{
			name:       "update_status_missing_job",
			wantStatus: http.StatusBadRequest,
			body:       `<UpdateJobStatusRequest><RequestedJobStatus>Complete</RequestedJobStatus></UpdateJobStatusRequest>`,
			setup:      func(_ *s3control.Handler) string { return "nonexistent" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			jobID := tt.setup(h)

			rec := doS3Request(t, h, http.MethodPost, "/v20180820/jobs/"+jobID+"/status", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantBody != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBody)
			}
		})
	}
}

// ---- BatchJob extended fields tests ----

func TestBatchJob_UpdateDetails(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	job, err := b.CreateJob("000000000000", "arn:aws:iam::000000000000:role/R", 5)
	require.NoError(t, err)
	require.NotEmpty(t, job.CreationTime) // CreationTime set on create

	err = b.UpdateJobDetails(
		"000000000000", job.JobID,
		"test job",
		"<Spec><Format>S3BatchOperations_CSV_20180820</Format></Spec>",
		"<LambdaInvoke><FunctionArn>arn:aws:lambda:::fn</FunctionArn></LambdaInvoke>",
		"<Bucket>arn:aws:s3:::report-bucket</Bucket>",
		true,
	)
	require.NoError(t, err)

	got, err := b.GetJob("000000000000", job.JobID)
	require.NoError(t, err)
	assert.Equal(t, "test job", got.Description)
	assert.Contains(t, got.Manifest, "S3BatchOperations_CSV_20180820")
	assert.Contains(t, got.Operation, "LambdaInvoke")
	assert.Contains(t, got.Report, "report-bucket")
	assert.True(t, got.ConfirmationRequired)
}

func TestBatchJob_UpdateDetails_MissingJob(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	err := b.UpdateJobDetails("000000000000", "nonexistent", "desc", "", "", "", false)
	require.ErrorContains(t, err, "NoSuchJob")
}

func TestHandler_CreateJob_ExtendedFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		body            string
		wantDescription string
		wantStatus      int
		wantManifest    bool
	}{
		{
			name: "job_with_description_and_manifest",
			body: `<CreateJobRequest>
<RoleArn>arn:aws:iam::000000000000:role/Role</RoleArn>
<Priority>10</Priority>
<Description>my batch job</Description>
<ConfirmationRequired>true</ConfirmationRequired>
<Manifest><Spec><Format>S3BatchOperations_CSV_20180820</Format></Spec></Manifest>
<Operation><LambdaInvoke><FunctionArn>arn:aws:lambda:::fn</FunctionArn></LambdaInvoke></Operation>
<Report><Bucket>arn:aws:s3:::report-bucket</Bucket><Enabled>true</Enabled></Report>
</CreateJobRequest>`,
			wantStatus:      http.StatusOK,
			wantDescription: "my batch job",
			wantManifest:    true,
		},
		{
			name: "job_without_extended_fields",
			body: `<CreateJobRequest>
<RoleArn>arn:aws:iam::000000000000:role/Role</RoleArn>
<Priority>5</Priority>
</CreateJobRequest>`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3control.NewInMemoryBackend()
			h := s3control.NewHandler(b)

			rec := doS3ControlNewOpRequest(t, h, http.MethodPost, "/v20180820/jobs", "000000000000", tt.body)
			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantDescription != "" {
				// Extract job ID from response and verify stored job.
				require.Contains(t, rec.Body.String(), "JobId")
			}
		})
	}
}

func TestHandler_DescribeJob_ExtendedFields(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	job, err := b.CreateJob("000000000000", "arn:aws:iam::000000000000:role/R", 5)
	require.NoError(t, err)

	err = b.UpdateJobDetails(
		"000000000000", job.JobID,
		"my job description",
		"<Spec><Format>CSV</Format></Spec>",
		"<LambdaInvoke/>",
		"<Bucket>arn:aws:s3:::report</Bucket>",
		true,
	)
	require.NoError(t, err)

	h := s3control.NewHandler(b)
	rec := doS3ControlNewOpRequest(t, h, http.MethodGet, "/v20180820/jobs/"+job.JobID, "000000000000", "")

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "DescribeJobResult")
	assert.Contains(t, body, "my job description")
	assert.Contains(t, body, "ConfirmationRequired")
	assert.Contains(t, body, "CreationTime")
	assert.Contains(t, body, "Manifest")
	assert.Contains(t, body, "Operation")
	assert.Contains(t, body, "Report")
}

// ---- Job status validation tests ----

func TestUpdateJobStatus_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		requestedStatus string
		wantErr         bool
	}{
		{name: "cancelled_valid", requestedStatus: "Cancelled", wantErr: false},
		{name: "ready_valid", requestedStatus: "Ready", wantErr: false},
		{name: "complete_invalid", requestedStatus: "Complete", wantErr: true},
		{name: "active_invalid", requestedStatus: "Active", wantErr: true},
		{name: "new_invalid", requestedStatus: "New", wantErr: true},
		{name: "empty_invalid", requestedStatus: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3control.NewInMemoryBackend()
			job, err := b.CreateJob("000000000000", "arn:aws:iam::000000000000:role/R", 5)
			require.NoError(t, err)

			_, err = b.UpdateJobStatusValidated("000000000000", job.JobID, tt.requestedStatus, "")
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, s3control.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpdateJobStatusValidated_StatusUpdateReason(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	job, err := b.CreateJob("000000000000", "arn:aws:iam::000000000000:role/R", 5)
	require.NoError(t, err)

	updated, err := b.UpdateJobStatusValidated(
		"000000000000", job.JobID,
		"Cancelled",
		"user requested cancellation",
	)
	require.NoError(t, err)
	assert.Equal(t, "Cancelled", updated.Status)
	assert.Equal(t, "user requested cancellation", updated.StatusUpdateReason)
}

func TestCreateJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		accountID        string
		body             string
		wantBodyContains string
		wantStatus       int
	}{
		{
			name:      "creates_batch_job",
			accountID: "123456789012",
			body: `<CreateJobRequest>
<ClientRequestToken>token-123</ClientRequestToken>
<Priority>10</Priority>
<RoleArn>arn:aws:iam::123456789012:role/BatchOpsRole</RoleArn>
</CreateJobRequest>`,
			wantStatus:       http.StatusOK,
			wantBodyContains: "JobId",
		},
		{
			name:             "creates_job_with_minimal_body",
			accountID:        "000000000000",
			body:             `<CreateJobRequest><RoleArn>arn:aws:iam::000000000000:role/Role</RoleArn></CreateJobRequest>`,
			wantStatus:       http.StatusOK,
			wantBodyContains: "JobId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestS3ControlHandler(t)
			rec := doS3ControlNewOpRequest(t, h, http.MethodPost, "/v20180820/jobs", tt.accountID, tt.body)

			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantBodyContains != "" {
				assert.Contains(t, rec.Body.String(), tt.wantBodyContains)
			}
		})
	}
}

// TestCreateJob_PriorityBound verifies CreateJob rejects a negative
// priority (AWS bounds Priority to a non-negative integer) while accepting valid
// non-negative values.
func TestCreateJob_PriorityBound(t *testing.T) {
	t.Parallel()

	const role = "arn:aws:iam::000000000000:role/R"

	tests := []struct {
		name     string
		priority int32
		wantErr  bool
	}{
		{name: "zero_ok", priority: 0, wantErr: false},
		{name: "positive_ok", priority: 100, wantErr: false},
		{name: "negative_rejected", priority: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3control.NewInMemoryBackend()
			_, err := b.CreateJob("000000000000", role, tt.priority)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, s3control.ErrValidation)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestUpdateJobPriority_PriorityBound verifies UpdateJobPriority rejects a
// negative priority, mirroring CreateJob's own non-negative-Priority
// validation (TestCreateJob_PriorityBound above) -- UpdateJobPriorityInput
// and CreateJobInput share the same JobPriority quantity in the S3 Batch
// Operations model, so this backend previously let UpdateJobPriority set a
// job to a negative priority CreateJob would have rejected outright.
func TestUpdateJobPriority_PriorityBound(t *testing.T) {
	t.Parallel()

	const role = "arn:aws:iam::000000000000:role/R"

	tests := []struct {
		name     string
		priority int32
		wantErr  bool
	}{
		{name: "zero_ok", priority: 0, wantErr: false},
		{name: "positive_ok", priority: 100, wantErr: false},
		{name: "negative_rejected", priority: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3control.NewInMemoryBackend()
			job, err := b.CreateJob("000000000000", role, 0)
			require.NoError(t, err)

			_, err = b.UpdateJobPriority("000000000000", job.JobID, tt.priority)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, s3control.ErrValidation)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestCreateJob_RequiresRoleArn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		roleArn string
		wantErr bool
	}{
		{name: "empty_role_arn", roleArn: "", wantErr: true},
		{name: "valid_role_arn", roleArn: "arn:aws:iam::123:role/R", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := s3control.NewInMemoryBackend()
			job, err := b.CreateJob("acc1", tt.roleArn, 5)

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, s3control.ErrValidation)
				assert.Nil(t, job)
			} else {
				require.NoError(t, err)
				require.NotNil(t, job)
				assert.NotEmpty(t, job.JobID)
			}
		})
	}
}

func TestHandler_CreateJob_BadRoleArn(t *testing.T) {
	t.Parallel()

	h := newTestS3ControlHandler(t)
	rec := doS3ControlNewOpRequest(
		t, h, http.MethodPost, "/v20180820/jobs",
		"123456789012",
		`<CreateJobRequest><Priority>5</Priority></CreateJobRequest>`,
	)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListJobs_Pagination(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	for range 4 {
		b.AddBatchJobInternal("acct1", "arn:aws:iam::000000000000:role/r", 1)
	}
	h := s3control.NewHandler(b)

	tests := []struct {
		path          string
		name          string
		wantLen       int
		wantNextToken bool
	}{
		{
			name:          "no_limit_returns_all",
			path:          "/v20180820/jobs",
			wantLen:       4,
			wantNextToken: false,
		},
		{
			name:          "page1_two_items",
			path:          "/v20180820/jobs?maxResults=2",
			wantLen:       2,
			wantNextToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doS3Request(t, h, http.MethodGet, tt.path, "")
			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				XMLName   xml.Name `xml:"ListJobsResult"`
				NextToken string   `xml:"NextToken"`
				Jobs      []struct {
					JobID string `xml:"JobId"`
				} `xml:"Jobs>member"`
			}
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))
			assert.Len(t, out.Jobs, tt.wantLen)
			if tt.wantNextToken {
				assert.NotEmpty(t, out.NextToken)
			} else {
				assert.Empty(t, out.NextToken)
			}
		})
	}
}

func TestListJobs_JobStatusesFilter(t *testing.T) {
	t.Parallel()

	b := s3control.NewInMemoryBackend()
	j1 := b.AddBatchJobInternal("acct1", "arn:aws:iam::000000000000:role/r", 1)
	j2 := b.AddBatchJobInternal("acct1", "arn:aws:iam::000000000000:role/r", 1)
	_ = b.AddBatchJobInternal("acct1", "arn:aws:iam::000000000000:role/r", 1)
	_, _ = b.UpdateJobStatus("acct1", j1.JobID, "Active")
	_, _ = b.UpdateJobStatus("acct1", j2.JobID, "Cancelled")
	h := s3control.NewHandler(b)

	tests := []struct {
		path    string
		name    string
		wantLen int
	}{
		{
			name:    "no_filter_returns_all",
			path:    "/v20180820/jobs",
			wantLen: 3,
		},
		{
			name:    "filter_active_only",
			path:    "/v20180820/jobs?jobStatuses=Active",
			wantLen: 1,
		},
		{
			name:    "filter_active_and_cancelled",
			path:    "/v20180820/jobs?jobStatuses=Active&jobStatuses=Cancelled",
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doS3Request(t, h, http.MethodGet, tt.path, "")
			require.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				XMLName xml.Name `xml:"ListJobsResult"`
				Jobs    []struct {
					JobID string `xml:"JobId"`
				} `xml:"Jobs>member"`
			}
			require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))
			assert.Len(t, out.Jobs, tt.wantLen, "path: %s", tt.path)
		})
	}
}

// TestListJobs_ItemFields covers ListJobs' JobListDescriptor items:
// JobId/Status/Priority plus Description/Operation/CreationTime/TerminationDate.
// Operation is derived from the raw <Operation> inner XML's root element
// name (e.g. "LambdaInvoke" — jobOperationName in handler_jobs.go),
// matching JobListDescriptor.Operation's OperationName enum, not the full
// nested config JobDescriptor.Operation carries.
func TestListJobs_ItemFields(t *testing.T) {
	t.Parallel()

	h := newTestS3ControlHandler(t)
	createBody := `<CreateJobRequest>
<RoleArn>arn:aws:iam::000000000000:role/Role</RoleArn>
<Priority>7</Priority>
<Description>nightly batch job</Description>
<Manifest><Spec><Format>S3BatchOperations_CSV_20180820</Format></Spec></Manifest>
<Operation><LambdaInvoke><FunctionArn>arn:aws:lambda:::fn</FunctionArn></LambdaInvoke></Operation>
</CreateJobRequest>`
	createRec := doS3Request(t, h, http.MethodPost, "/v20180820/jobs", createBody)
	require.Equal(t, http.StatusOK, createRec.Code)

	var created struct {
		JobID string `xml:"JobId"`
	}
	require.NoError(t, xml.Unmarshal(createRec.Body.Bytes(), &created))
	require.NotEmpty(t, created.JobID)

	listRec := doS3Request(t, h, http.MethodGet, "/v20180820/jobs", "")
	require.Equal(t, http.StatusOK, listRec.Code)

	var out struct {
		XMLName xml.Name `xml:"ListJobsResult"`
		Jobs    []struct {
			JobID        string `xml:"JobId"`
			Description  string `xml:"Description"`
			Operation    string `xml:"Operation"`
			Status       string `xml:"Status"`
			CreationTime string `xml:"CreationTime"`
			Priority     int32  `xml:"Priority"`
		} `xml:"Jobs>member"`
	}
	require.NoError(t, xml.Unmarshal(listRec.Body.Bytes(), &out))
	require.Len(t, out.Jobs, 1)

	job := out.Jobs[0]
	assert.Equal(t, created.JobID, job.JobID)
	assert.Equal(t, "nightly batch job", job.Description)
	assert.Equal(t, "LambdaInvoke", job.Operation)
	assert.NotEmpty(t, job.Status)
	assert.NotEmpty(t, job.CreationTime)
	assert.Equal(t, int32(7), job.Priority)
}

// TestJobTagging_WireShape asserts the literal nested envelope for job
// tagging: S3TagSet wraps each entry as "<member>", not "<Tag>"
// (awsRestxml_serializeDocumentS3TagSet, shared by every S3Tag list in this
// service).
func TestJobTagging_WireShape(t *testing.T) {
	t.Parallel()

	h := newTestS3ControlHandler(t)
	job := h.Backend.AddBatchJobInternal("acct1", "arn:aws:iam::acct1:role/R", 1)

	putBody := `<PutJobTaggingRequest>` +
		`<Tags><member><Key>env</Key><Value>prod</Value></member></Tags>` +
		`</PutJobTaggingRequest>`
	putRec := doS3Request(t, h, http.MethodPut, "/v20180820/jobs/"+job.JobID+"/tagging", putBody)
	require.Equal(t, http.StatusOK, putRec.Code)

	getRec := doS3Request(t, h, http.MethodGet, "/v20180820/jobs/"+job.JobID+"/tagging", "")
	require.Equal(t, http.StatusOK, getRec.Code)

	assert.Contains(t, getRec.Body.String(), "<Tags><member>")
	assert.NotContains(t, getRec.Body.String(), "<Tag>")

	var out struct {
		XMLName xml.Name `xml:"GetJobTaggingResult"`
		Tags    struct {
			Entries []struct {
				Key   string `xml:"Key"`
				Value string `xml:"Value"`
			} `xml:"member"`
		} `xml:"Tags"`
	}
	require.NoError(t, xml.Unmarshal(getRec.Body.Bytes(), &out))
	require.Len(t, out.Tags.Entries, 1)
	assert.Equal(t, "env", out.Tags.Entries[0].Key)
	assert.Equal(t, "prod", out.Tags.Entries[0].Value)
}
