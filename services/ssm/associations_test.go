package ssm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ssm"
)

func TestAssociationOps(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	// Create association
	assoc, err := b.CreateAssociation(context.TODO(), &ssm.CreateAssociationInput{
		Name:       "AWS-RunShellScript",
		InstanceID: "i-001",
	})
	require.NoError(t, err)

	assocID := assoc.AssociationDescription.AssociationID

	// ListAssociationVersions
	rec := doRequest(t, h, "ListAssociationVersions", `{"AssociationId":"`+assocID+`"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assertBodyContains(t, rec, "AssociationVersions")

	// DescribeAssociationExecutions
	rec = doRequest(t, h, "DescribeAssociationExecutions", `{"AssociationId":"`+assocID+`"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	// DescribeEffectiveInstanceAssociations
	rec = doRequest(t, h, "DescribeEffectiveInstanceAssociations", `{"InstanceId":"i-001"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assertBodyContains(t, rec, "Associations")

	// DescribeInstanceAssociationsStatus
	rec = doRequest(t, h, "DescribeInstanceAssociationsStatus", `{"InstanceId":"i-001"}`)
	require.Equal(t, http.StatusOK, rec.Code)
}
func TestUpdateAssociationStatus_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		instanceID string
		assocName  string
	}{
		{
			name:       "no_matching_association_returns_error",
			instanceID: "i-does-not-exist",
			assocName:  "AWS-RunShellScript",
		},
		{
			name:       "empty_ids_return_error",
			instanceID: "",
			assocName:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			_, err := b.UpdateAssociationStatus(context.TODO(), &ssm.UpdateAssociationStatusInput{
				InstanceID: tt.instanceID,
				Name:       tt.assocName,
				AssociationStatus: ssm.AssociationStatusValue{
					Name:    "Success",
					Date:    1700000000,
					Message: "association is compliant",
				},
			})
			require.ErrorIs(t, err, ssm.ErrAssociationNotFound,
				"no-match UpdateAssociationStatus must return ErrAssociationNotFound")
		})
	}
}
func TestUpdateAssociationStatus_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusName string
	}{
		{
			name:       "status_updated_to_success",
			statusName: "Success",
		},
		{
			name:       "status_updated_to_failed",
			statusName: "Failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)

			assocOut, err := b.CreateAssociation(context.TODO(), &ssm.CreateAssociationInput{
				Name:       "AWS-RunShellScript",
				InstanceID: "i-abc123",
			})
			require.NoError(t, err)

			out, err := b.UpdateAssociationStatus(context.TODO(), &ssm.UpdateAssociationStatusInput{
				InstanceID: "i-abc123",
				Name:       "AWS-RunShellScript",
				AssociationStatus: ssm.AssociationStatusValue{
					Name:           tt.statusName,
					Date:           1700000000,
					Message:        "status changed by agent",
					AdditionalInfo: "agent-reported",
				},
			})
			require.NoError(t, err)
			require.NotNil(t, out)
			require.NotNil(t, out.AssociationDescription.Overview)
			assert.Equal(t, tt.statusName, out.AssociationDescription.Overview.Status)
			assert.Equal(t, assocOut.AssociationDescription.AssociationID, out.AssociationDescription.AssociationID)

			require.NotNil(t, out.AssociationDescription.Status,
				"AssociationDescription.Status has no Go member without this fix -- "+
					"UpdateAssociationStatus silently dropped it")
			assert.Equal(t, tt.statusName, out.AssociationDescription.Status.Name)
			assert.InDelta(t, 1700000000, out.AssociationDescription.Status.Date, 0)
			assert.Equal(t, "status changed by agent", out.AssociationDescription.Status.Message)
			assert.Equal(t, "agent-reported", out.AssociationDescription.Status.AdditionalInfo)
		})
	}
}
func TestUpdateAssociationStatus_Handler_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{
			name: "no_matching_association_returns_400",
			body: `{"InstanceId":"i-ghost","Name":"AWS-RunShellScript",` +
				`"AssociationStatus":{"Name":"Success","Date":1700000000,"Message":"m"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			rec := doRequest(t, h, "UpdateAssociationStatus", tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "AssociationDoesNotExist")
		})
	}
}
func TestAssociationExecutions_Stable(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	ctx := context.TODO()

	create, err := b.CreateAssociation(ctx, &ssm.CreateAssociationInput{
		Name:       "AWS-RunShellScript",
		InstanceID: "i-0123456789abcdef0",
	})
	require.NoError(t, err)
	assocID := create.AssociationDescription.AssociationID

	first, err := b.DescribeAssociationExecutions(ctx, &ssm.DescribeAssociationExecutionsInput{
		AssociationID: assocID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, first.AssociationExecutions)
	execID := first.AssociationExecutions[0].ExecutionID
	require.NotEmpty(t, execID)

	// A second call must return the SAME execution ID (not a fresh UUID).
	second, err := b.DescribeAssociationExecutions(ctx, &ssm.DescribeAssociationExecutionsInput{
		AssociationID: assocID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, second.AssociationExecutions)
	assert.Equal(t, execID, second.AssociationExecutions[0].ExecutionID,
		"association execution ID must be stable across calls")

	// Targets for that execution must be stable and reference the instance.
	targetsA, err := b.DescribeAssociationExecutionTargets(
		ctx,
		&ssm.DescribeAssociationExecutionTargetsInput{
			AssociationID: assocID,
			ExecutionID:   execID,
		},
	)
	require.NoError(t, err)
	require.Len(t, targetsA.AssociationExecutionTargets, 1)
	assert.Equal(t, "i-0123456789abcdef0", targetsA.AssociationExecutionTargets[0].ResourceID)
	assert.Equal(t, execID, targetsA.AssociationExecutionTargets[0].ExecutionID)

	targetsB, err := b.DescribeAssociationExecutionTargets(
		ctx,
		&ssm.DescribeAssociationExecutionTargetsInput{
			AssociationID: assocID,
			ExecutionID:   execID,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, targetsA.AssociationExecutionTargets, targetsB.AssociationExecutionTargets,
		"association execution targets must be stable across calls")

	// StartAssociationsOnce appends a new stable execution record.
	_, err = b.StartAssociationsOnce(ctx, &ssm.StartAssociationsOnceInput{
		AssociationIDs: []string{assocID},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, b.AssociationExecutionCount(assocID),
		"a one-time run must append a new execution record")
}

// TestStubOps_DeleteAssociation exercises the association-backed delete stub.
func TestStubOps_DeleteAssociation(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	// Create an association so we have a valid ID.
	assoc, err := b.CreateAssociation(context.TODO(), &ssm.CreateAssociationInput{
		Name:       "AWS-RunShellScript",
		InstanceID: "i-1234567890abcdef0",
	})
	require.NoError(t, err)

	body, _ := json.Marshal(
		map[string]any{"AssociationId": assoc.AssociationDescription.AssociationID},
	)
	rec := doRequest(t, h, "DeleteAssociation", string(body))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStubOps_DescribeAssociation exercises that stub.
func TestStubOps_DescribeAssociation(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	assoc, err := b.CreateAssociation(context.TODO(), &ssm.CreateAssociationInput{
		Name:       "AWS-RunShellScript",
		InstanceID: "i-1234567890abcdef0",
	})
	require.NoError(t, err)

	body, _ := json.Marshal(
		map[string]any{"AssociationId": assoc.AssociationDescription.AssociationID},
	)
	rec := doRequest(t, h, "DescribeAssociation", string(body))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStubOps_UpdateAssociation exercises the update stub.
func TestStubOps_UpdateAssociation(t *testing.T) {
	t.Parallel()

	h, b := newTestHandler(t)

	assoc, err := b.CreateAssociation(context.TODO(), &ssm.CreateAssociationInput{
		Name:       "AWS-RunShellScript",
		InstanceID: "i-1234567890abcdef0",
	})
	require.NoError(t, err)

	body, _ := json.Marshal(
		map[string]any{"AssociationId": assoc.AssociationDescription.AssociationID},
	)
	rec := doRequest(t, h, "UpdateAssociation", string(body))
	assert.Equal(t, http.StatusOK, rec.Code)
}
func TestCreateAssociation_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check      func(t *testing.T, out ssm.Association)
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "associates_existing_document",
			body:       `{"Name":"MyDoc","InstanceId":"i-abc123"}`,
			wantStatus: http.StatusOK,
		},
		{
			// Locks in the previously-missing CreateAssociationInput fields
			// (bd gopherstack-ouvq), confirmed present in aws-sdk-go-v2
			// v1.73.4's api_op_CreateAssociation.go.
			name: "round_trips_extended_fields",
			body: `{
				"Name": "MyDoc",
				"InstanceId": "i-abc123",
				"ApplyOnlyAtCronInterval": true,
				"AssociationDispatchAssumeRole": "arn:aws:iam::123456789012:role/dispatch",
				"AutomationTargetParameterName": "InstanceId",
				"CalendarNames": ["cal-1"],
				"ComplianceSeverity": "HIGH",
				"Duration": 4,
				"MaxConcurrency": "50%",
				"MaxErrors": "1",
				"OutputLocation": {"S3Location": {"OutputS3BucketName": "assoc-bucket", "OutputS3KeyPrefix": "out"}},
				"ScheduleExpression": "rate(30 minutes)",
				"SyncCompliance": "AUTO"
			}`,
			wantStatus: http.StatusOK,
			check: func(t *testing.T, out ssm.Association) {
				t.Helper()
				assert.True(t, out.ApplyOnlyAtCronInterval)
				assert.Equal(t, "arn:aws:iam::123456789012:role/dispatch", out.AssociationDispatchAssumeRole)
				assert.Equal(t, "InstanceId", out.AutomationTargetParameterName)
				assert.Equal(t, []string{"cal-1"}, out.CalendarNames)
				assert.Equal(t, "HIGH", out.ComplianceSeverity)
				require.NotNil(t, out.Duration)
				assert.EqualValues(t, 4, *out.Duration)
				assert.Equal(t, "50%", out.MaxConcurrency)
				assert.Equal(t, "1", out.MaxErrors)
				require.NotNil(t, out.OutputLocation)
				require.NotNil(t, out.OutputLocation.S3Location)
				assert.Equal(t, "assoc-bucket", out.OutputLocation.S3Location.OutputS3BucketName)
				assert.Equal(t, "rate(30 minutes)", out.ScheduleExpression)
				assert.Equal(t, "AUTO", out.SyncCompliance)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, backend := newTestHandler(t)

			doRequest(
				t,
				h,
				"CreateDocument",
				`{"Name":"MyDoc","Content":"{\"schemaVersion\":\"2.2\"}","DocumentType":"Command"}`,
			)

			rec := doRequest(t, h, "CreateAssociation", tt.body)
			require.Equal(t, tt.wantStatus, rec.Code)

			var resp ssm.CreateAssociationOutput
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.NotEmpty(t, resp.AssociationDescription.AssociationID)
			assert.Equal(t, "MyDoc", resp.AssociationDescription.Name)
			assert.Equal(t, 1, backend.AssociationCount())

			if tt.check != nil {
				tt.check(t, resp.AssociationDescription)
			}
		})
	}
}
func TestCreateAssociationBatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		wantStatus     int
		wantSuccessful int
		wantFailed     int
		setupDoc       bool
	}{
		{
			name:           "all_succeed",
			setupDoc:       true,
			body:           `{"Entries":[{"Name":"MyDoc","InstanceId":"i-1"},{"Name":"MyDoc","InstanceId":"i-2"}]}`,
			wantStatus:     http.StatusOK,
			wantSuccessful: 2,
			wantFailed:     0,
		},
		{
			name:           "partial_failure",
			setupDoc:       true,
			body:           `{"Entries":[{"Name":"MyDoc","InstanceId":"i-1"},{"Name":"NoSuchDoc","InstanceId":"i-2"}]}`,
			wantStatus:     http.StatusOK,
			wantSuccessful: 1,
			wantFailed:     1,
		},
		{
			name:           "all_fail",
			setupDoc:       false,
			body:           `{"Entries":[{"Name":"BadDoc1"},{"Name":"BadDoc2"}]}`,
			wantStatus:     http.StatusOK,
			wantSuccessful: 0,
			wantFailed:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, backend := newTestHandler(t)

			if tt.setupDoc {
				doRequest(
					t,
					h,
					"CreateDocument",
					`{"Name":"MyDoc","Content":"{\"schemaVersion\":\"2.2\"}","DocumentType":"Command"}`,
				)
			}

			rec := doRequest(t, h, "CreateAssociationBatch", tt.body)
			require.Equal(t, tt.wantStatus, rec.Code)

			var resp ssm.CreateAssociationBatchOutput
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Len(t, resp.Successful, tt.wantSuccessful)
			assert.Len(t, resp.Failed, tt.wantFailed)
			assert.Equal(t, tt.wantSuccessful, backend.AssociationCount())
		})
	}
}

// TestCreateAssociation_NameValidation verifies Name is required.
func TestCreateAssociation_NameValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantErr    string
		wantStatus int
	}{
		{
			name:       "missing_name",
			body:       `{"InstanceId":"i-abc123"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "ValidationException",
		},
		{
			name:       "empty_name",
			body:       `{"Name":"","InstanceId":"i-abc123"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "ValidationException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			rec := doRequest(t, h, "CreateAssociation", tt.body)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantErr)
		})
	}
}
func TestFull_Association_CreateDescribeUpdate(t *testing.T) {
	t.Parallel()
	h := newHandler()

	// Need a document
	postJSON(t, h, "CreateDocument", map[string]any{
		"Name":    "AssocDoc",
		"Content": `{"schemaVersion":"2.2"}`,
	})

	// Create association
	code, out := postJSON(t, h, "CreateAssociation", map[string]any{
		"Name":       "AssocDoc",
		"InstanceId": "i-assoc001",
		"Parameters": map[string]any{"cmd": []string{"echo test"}},
	})
	assert.Equal(t, http.StatusOK, code)
	assocID := out["AssociationDescription"].(map[string]any)["AssociationId"].(string)
	assert.NotEmpty(t, assocID)

	// Describe
	code, out = postJSON(t, h, "DescribeAssociation", map[string]any{"AssociationId": assocID})
	assert.Equal(t, http.StatusOK, code)
	assert.NotNil(t, out["AssociationDescription"])

	// Update
	code, _ = postJSON(t, h, "UpdateAssociation", map[string]any{
		"AssociationId":   assocID,
		"AssociationName": "UpdatedAssoc",
	})
	assert.Equal(t, http.StatusOK, code)
}

// TestCopyAssocTargets exercises the copyAssocTargets nil/non-nil paths.
func TestCopyAssocTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		targets []ssm.AssociationTarget
	}{
		{
			name:    "nil_targets",
			targets: nil,
		},
		{
			name: "non_nil_targets",
			targets: []ssm.AssociationTarget{
				{Key: "tag:Env", Values: []string{"prod"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			// CreateAssociationBatch exercises copyAssocTargets internally
			out, err := b.CreateAssociationBatch(context.TODO(), &ssm.CreateAssociationBatchInput{
				Entries: []ssm.CreateAssociationBatchRequestEntry{
					{
						Name:    "AWS-RunShellScript",
						Targets: tt.targets,
					},
				},
			})
			require.NoError(t, err)
			assert.Len(t, out.Successful, 1)
		})
	}
}

// TestDeleteAssociation_NotFound exercises the error path.
func TestDeleteAssociation_NotFound(t *testing.T) {
	t.Parallel()

	b := ssm.NewInMemoryBackend()
	_, err := b.DeleteAssociation(context.TODO(), &ssm.DeleteAssociationInput{AssociationID: "nonexistent"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ssm.ErrAssociationNotFound)
}

// TestUpdateAssociation covers update branches including nil vs non-nil targets.
func TestUpdateAssociation(t *testing.T) {
	t.Parallel()

	durationHours := int32(6)

	tests := []struct {
		check   func(t *testing.T, out ssm.Association)
		name    string
		update  ssm.UpdateAssociationInput
		wantErr bool
	}{
		{
			name: "not_found",
			update: ssm.UpdateAssociationInput{
				AssociationID: "does-not-exist",
			},
			wantErr: true,
		},
		{
			name: "update_name_and_version",
			update: ssm.UpdateAssociationInput{
				AssociationName: "updated-name",
				DocumentVersion: "$LATEST",
			},
			wantErr: false,
		},
		{
			name: "update_with_targets",
			update: ssm.UpdateAssociationInput{
				Targets: []ssm.AssociationTarget{
					{Key: "tag:Env", Values: []string{"staging"}},
				},
			},
			wantErr: false,
		},
		{
			// Locks in that UpdateAssociationInput carries the same
			// previously-missing fields as CreateAssociationInput
			// (ApplyOnlyAtCronInterval/ComplianceSeverity/MaxConcurrency/
			// MaxErrors/OutputLocation/ScheduleExpression/SyncCompliance/
			// CalendarNames/AssociationDispatchAssumeRole/
			// AutomationTargetParameterName/Duration), confirmed present on
			// both api_op_CreateAssociation.go and api_op_UpdateAssociation.go.
			name: "update_extended_fields",
			update: ssm.UpdateAssociationInput{
				ApplyOnlyAtCronInterval:       true,
				AssociationDispatchAssumeRole: "arn:aws:iam::123456789012:role/dispatch",
				AutomationTargetParameterName: "InstanceId",
				CalendarNames:                 []string{"cal-1", "cal-2"},
				ComplianceSeverity:            "CRITICAL",
				Duration:                      &durationHours,
				MaxConcurrency:                "10%",
				MaxErrors:                     "5%",
				OutputLocation: &ssm.InstanceAssociationOutputLocation{
					S3Location: &ssm.S3OutputLocation{
						OutputS3BucketName: "my-bucket",
						OutputS3KeyPrefix:  "assoc-output",
					},
				},
				ScheduleExpression: "cron(0 2 ? * SUN *)",
				SyncCompliance:     "MANUAL",
			},
			wantErr: false,
			check: func(t *testing.T, out ssm.Association) {
				t.Helper()
				assert.True(t, out.ApplyOnlyAtCronInterval)
				assert.Equal(t, "arn:aws:iam::123456789012:role/dispatch", out.AssociationDispatchAssumeRole)
				assert.Equal(t, "InstanceId", out.AutomationTargetParameterName)
				assert.Equal(t, []string{"cal-1", "cal-2"}, out.CalendarNames)
				assert.Equal(t, "CRITICAL", out.ComplianceSeverity)
				require.NotNil(t, out.Duration)
				assert.EqualValues(t, 6, *out.Duration)
				assert.Equal(t, "10%", out.MaxConcurrency)
				assert.Equal(t, "5%", out.MaxErrors)
				require.NotNil(t, out.OutputLocation)
				require.NotNil(t, out.OutputLocation.S3Location)
				assert.Equal(t, "my-bucket", out.OutputLocation.S3Location.OutputS3BucketName)
				assert.Equal(t, "cron(0 2 ? * SUN *)", out.ScheduleExpression)
				assert.Equal(t, "MANUAL", out.SyncCompliance)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()

			if !tt.wantErr {
				// Create an association first
				out, err := b.CreateAssociation(context.TODO(), &ssm.CreateAssociationInput{
					Name: "AWS-RunShellScript",
				})
				require.NoError(t, err)
				tt.update.AssociationID = out.AssociationDescription.AssociationID
			}

			updated, err := b.UpdateAssociation(context.TODO(), &tt.update)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ssm.ErrAssociationNotFound)
			} else {
				require.NoError(t, err)

				if tt.check != nil {
					tt.check(t, updated.AssociationDescription)
				}
			}
		})
	}
}

// TestDeleteAssociation_TableDriven verifies success and not-found for DeleteAssociation.
func TestDeleteAssociation_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantErrMsg string
		wantStatus int
		setupFirst bool
	}{
		{
			name:       "deletes_existing_association",
			setupFirst: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "nonexistent_association_returns_error",
			setupFirst: false,
			wantStatus: http.StatusBadRequest,
			wantErrMsg: "AssociationDoesNotExist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler(t)

			assocID := "assoc-nonexistent"
			if tt.setupFirst {
				assoc, err := b.CreateAssociation(context.Background(), &ssm.CreateAssociationInput{
					Name:       "AWS-RunShellScript",
					InstanceID: "i-1234567890abcdef0",
				})
				require.NoError(t, err)
				assocID = assoc.AssociationDescription.AssociationID
			}

			body, _ := json.Marshal(map[string]any{"AssociationId": assocID})
			rec := doRequest(t, h, "DeleteAssociation", string(body))
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantErrMsg != "" {
				assert.Contains(t, rec.Body.String(), tt.wantErrMsg)
			}
		})
	}
}
func TestDescribeAssociationExecutionTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input         ssm.DescribeAssociationExecutionTargetsInput
		name          string
		assocInstance string
		targets       []ssm.AssociationTarget
		wantCount     int
	}{
		{
			name:      "unknown_assoc_returns_empty",
			input:     ssm.DescribeAssociationExecutionTargetsInput{AssociationID: "assoc-does-not-exist"},
			wantCount: 0,
		},
		{
			name:          "instance_only_returns_one_target",
			assocInstance: "i-1234567890abcdef0",
			wantCount:     1,
		},
		{
			name:      "explicit_targets_returned",
			targets:   []ssm.AssociationTarget{{Key: "InstanceIds", Values: []string{"i-aaa", "i-bbb"}}},
			wantCount: 2,
		},
		{
			name:          "instance_plus_targets",
			assocInstance: "i-base",
			targets:       []ssm.AssociationTarget{{Key: "InstanceIds", Values: []string{"i-extra"}}},
			wantCount:     2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			ctx := context.Background()

			var assocID string
			if tc.assocInstance != "" || len(tc.targets) > 0 {
				out, err := b.CreateAssociation(ctx, &ssm.CreateAssociationInput{
					Name:       "AWS-RunShellScript",
					InstanceID: tc.assocInstance,
					Targets:    tc.targets,
				})
				require.NoError(t, err)
				assocID = out.AssociationDescription.AssociationID
			}

			input := tc.input
			if assocID != "" {
				input.AssociationID = assocID
			}

			out, err := b.DescribeAssociationExecutionTargets(ctx, &input)
			require.NoError(t, err)
			assert.Len(t, out.AssociationExecutionTargets, tc.wantCount)

			// All returned targets must carry the association ID.
			for _, tgt := range out.AssociationExecutionTargets {
				assert.Equal(t, assocID, tgt.AssociationID)
				assert.NotEmpty(t, tgt.ExecutionID)
				assert.NotEmpty(t, tgt.Status)
			}
		})
	}
}

// TestAssociationOps_RequireAssociationID locks in that
// DescribeAssociationExecutionTargets, DescribeAssociationExecutions and
// ListAssociationVersions all require AssociationId
// (api_op_DescribeAssociationExecutionTargets.go,
// api_op_DescribeAssociationExecutions.go, api_op_ListAssociationVersions.go
// all mark it "This member is required."), and StartAssociationsOnce
// requires a non-empty AssociationIds (api_op_StartAssociationsOnce.go).
// Previously all four silently accepted an empty body and returned 200 --
// DescribeAssociationExecutionTargets's own table test asserted this as
// "empty_assoc_id_returns_empty" before this fix.
func TestAssociationOps_RequireAssociationID(t *testing.T) {
	t.Parallel()

	t.Run("describe_association_execution_targets", func(t *testing.T) {
		t.Parallel()

		b := newBackend(t)
		_, err := b.DescribeAssociationExecutionTargets(
			context.Background(),
			&ssm.DescribeAssociationExecutionTargetsInput{},
		)
		require.ErrorIs(t, err, ssm.ErrValidationException)
	})

	t.Run("describe_association_executions", func(t *testing.T) {
		t.Parallel()

		b := newBackend(t)
		_, err := b.DescribeAssociationExecutions(
			context.Background(),
			&ssm.DescribeAssociationExecutionsInput{},
		)
		require.ErrorIs(t, err, ssm.ErrValidationException)
	})

	t.Run("list_association_versions", func(t *testing.T) {
		t.Parallel()

		b := newBackend(t)
		_, err := b.ListAssociationVersions(
			context.Background(),
			&ssm.ListAssociationVersionsInput{},
		)
		require.ErrorIs(t, err, ssm.ErrValidationException)
	})

	t.Run("start_associations_once", func(t *testing.T) {
		t.Parallel()

		b := newBackend(t)
		_, err := b.StartAssociationsOnce(
			context.Background(),
			&ssm.StartAssociationsOnceInput{},
		)
		require.ErrorIs(t, err, ssm.ErrValidationException)
	})
}

// TestCreateAssociation_MaxConcurrencyMaxErrorsValidation locks in that
// CreateAssociationInput.MaxConcurrency/MaxErrors are checked against the
// wire model's pattern (ssm/2014-11-06/service-2.json: MaxConcurrency
// "^([1-9][0-9]*|[1-9][0-9]%|[1-9]%|100%)$", MaxErrors
// "^([1-9][0-9]*|[0]|[1-9][0-9]%|[0-9]%|100%)$") rather than accepted verbatim.
func TestCreateAssociation_MaxConcurrencyMaxErrorsValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		maxConcurrency string
		maxErrors      string
		wantErr        bool
	}{
		{name: "absolute counts", maxConcurrency: "10", maxErrors: "0"},
		{name: "percentages", maxConcurrency: "50%", maxErrors: "10%"},
		{name: "unset is allowed"},
		{name: "maxConcurrency zero", maxConcurrency: "0", wantErr: true},
		{name: "maxConcurrency leading zero", maxConcurrency: "05", wantErr: true},
		{name: "maxConcurrency over 100 percent", maxConcurrency: "150%", wantErr: true},
		{name: "maxConcurrency non-numeric", maxConcurrency: "abc", wantErr: true},
		{name: "maxErrors negative", maxErrors: "-1", wantErr: true},
		{name: "maxErrors non-numeric", maxErrors: "abc", wantErr: true},
		{name: "maxConcurrency over length bound", maxConcurrency: "12345678", wantErr: true},
		{name: "maxErrors over length bound", maxErrors: "12345678", wantErr: true},
		{name: "maxConcurrency at length bound", maxConcurrency: "1234567"},
		{name: "maxErrors at length bound", maxErrors: "1234567"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)

			_, err := b.CreateAssociation(context.Background(), &ssm.CreateAssociationInput{
				Name:           "AWS-RunShellScript",
				InstanceID:     "i-001",
				MaxConcurrency: tc.maxConcurrency,
				MaxErrors:      tc.maxErrors,
			})

			if tc.wantErr {
				require.ErrorIs(t, err, ssm.ErrValidationException)

				return
			}

			require.NoError(t, err)
		})
	}
}
