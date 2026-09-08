package asl_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/stepfunctions/asl"
)

var (
	errClusterNotFound = errors.New("cluster not found")
	errJobNotFound     = errors.New("job not found")
)

// --- mock implementations ---

type mockECS struct {
	returnOutput any
	returnErr    error
	calledInput  map[string]any
}

func (m *mockECS) SFNRunTask(_ context.Context, input map[string]any) (any, error) {
	m.calledInput = input

	return m.returnOutput, m.returnErr
}

type mockGlue struct {
	returnErr      error
	capturedArgs   map[string]string
	returnJobRunID string
	capturedJob    string
}

func (m *mockGlue) SFNStartJobRun(
	_ context.Context,
	jobName string,
	arguments map[string]string,
) (string, error) {
	m.capturedJob = jobName
	m.capturedArgs = arguments

	return m.returnJobRunID, m.returnErr
}

type mockEventBridge struct {
	returnErr       error
	capturedEntries []map[string]any
	returnFailCount int
}

func (m *mockEventBridge) SFNPutEvents(_ context.Context, entries []map[string]any) (int, error) {
	m.capturedEntries = entries

	return m.returnFailCount, m.returnErr
}

// --- ECS RunTask tests ---

func TestSFN_ECSRunTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		def               string
		mock              *mockECS
		input             string
		wantError         string
		wantCauseContains string
		wantTaskDef       string
	}{
		{
			name: "runs_ecs_task_optimistic",
			def: `{
				"StartAt": "Run",
				"States": {
					"Run": {
						"Type": "Task",
						"Resource": "arn:aws:states:::ecs:runTask",
						"End": true
					}
				}
			}`,
			mock: &mockECS{
				returnOutput: map[string]any{"Tasks": []any{}, "Failures": []any{}},
			},
			input:       `{"TaskDefinition": "my-task", "Cluster": "my-cluster"}`,
			wantTaskDef: "my-task",
		},
		{
			name: "runs_ecs_task_sync",
			def: `{
				"StartAt": "Run",
				"States": {
					"Run": {
						"Type": "Task",
						"Resource": "arn:aws:states:::ecs:runTask.sync",
						"End": true
					}
				}
			}`,
			mock: &mockECS{
				returnOutput: map[string]any{"Tasks": []any{}, "Failures": []any{}},
			},
			input:       `{"TaskDefinition": "sync-task"}`,
			wantTaskDef: "sync-task",
		},
		{
			name: "ecs_integration_not_configured",
			def: `{
				"StartAt": "Run",
				"States": {
					"Run": {
						"Type": "Task",
						"Resource": "arn:aws:states:::ecs:runTask",
						"End": true
					}
				}
			}`,
			mock:              nil,
			wantError:         "TaskFailed",
			wantCauseContains: asl.ErrECSIntegrationNotConfigured.Error(),
		},
		{
			name: "ecs_unsupported_action",
			def: `{
				"StartAt": "Describe",
				"States": {
					"Describe": {
						"Type": "Task",
						"Resource": "arn:aws:states:::ecs:describeTasks",
						"End": true
					}
				}
			}`,
			mock:              &mockECS{},
			wantError:         "TaskFailed",
			wantCauseContains: "unsupported ECS action",
		},
		{
			name: "ecs_backend_error_propagated",
			def: `{
				"StartAt": "Run",
				"States": {
					"Run": {
						"Type": "Task",
						"Resource": "arn:aws:states:::ecs:runTask",
						"End": true
					}
				}
			}`,
			mock:              &mockECS{returnErr: errClusterNotFound},
			wantError:         "TaskFailed",
			wantCauseContains: "cluster not found",
		},
		{
			name: "aws_sdk_ecs_resource",
			def: `{
				"StartAt": "Run",
				"States": {
					"Run": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:ecs:runTask",
						"End": true
					}
				}
			}`,
			mock:  &mockECS{returnOutput: map[string]any{"Tasks": []any{}, "Failures": []any{}}},
			input: `{"TaskDefinition": "sdk-task"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm, err := asl.Parse(tt.def)
			require.NoError(t, err)

			exec := asl.NewExecutor(sm, nil, nil)
			if tt.mock != nil {
				exec.SetECSIntegration(tt.mock)
			}

			input := tt.input
			if input == "" {
				input = `{}`
			}

			result, err := exec.Execute(t.Context(), "exec-1", input)
			require.NoError(t, err)

			assert.Equal(t, tt.wantError, result.Error)

			if tt.wantCauseContains != "" {
				assert.Contains(t, result.Cause, tt.wantCauseContains)
			}

			if tt.mock != nil && tt.wantError == "" && tt.wantTaskDef != "" {
				assert.Equal(t, tt.wantTaskDef, tt.mock.calledInput["TaskDefinition"])
			}
		})
	}
}

// --- Glue StartJobRun tests ---

func TestSFN_GlueStartJobRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		def               string
		mock              *mockGlue
		input             string
		wantError         string
		wantCauseContains string
		wantJobRunID      string
		wantJobName       string
	}{
		{
			name: "starts_glue_job",
			def: `{
				"StartAt": "StartJob",
				"States": {
					"StartJob": {
						"Type": "Task",
						"Resource": "arn:aws:states:::glue:startJobRun",
						"End": true
					}
				}
			}`,
			mock:         &mockGlue{returnJobRunID: "jr-abc123"},
			input:        `{"JobName": "my-etl-job", "Arguments": {"--input": "s3://bucket/key"}}`,
			wantJobRunID: "jr-abc123",
			wantJobName:  "my-etl-job",
		},
		{
			name: "glue_integration_not_configured",
			def: `{
				"StartAt": "StartJob",
				"States": {
					"StartJob": {
						"Type": "Task",
						"Resource": "arn:aws:states:::glue:startJobRun",
						"End": true
					}
				}
			}`,
			mock:              nil,
			wantError:         "TaskFailed",
			wantCauseContains: asl.ErrGlueIntegrationNotConfigured.Error(),
		},
		{
			name: "glue_unsupported_action",
			def: `{
				"StartAt": "Get",
				"States": {
					"Get": {
						"Type": "Task",
						"Resource": "arn:aws:states:::glue:getJob",
						"End": true
					}
				}
			}`,
			mock:              &mockGlue{},
			wantError:         "TaskFailed",
			wantCauseContains: "unsupported Glue action",
		},
		{
			name: "glue_backend_error",
			def: `{
				"StartAt": "StartJob",
				"States": {
					"StartJob": {
						"Type": "Task",
						"Resource": "arn:aws:states:::glue:startJobRun.sync",
						"End": true
					}
				}
			}`,
			mock:              &mockGlue{returnErr: errJobNotFound},
			input:             `{"JobName": "missing-job"}`,
			wantError:         "TaskFailed",
			wantCauseContains: "job not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm, err := asl.Parse(tt.def)
			require.NoError(t, err)

			exec := asl.NewExecutor(sm, nil, nil)
			if tt.mock != nil {
				exec.SetGlueIntegration(tt.mock)
			}

			input := tt.input
			if input == "" {
				input = `{}`
			}

			result, err := exec.Execute(t.Context(), "exec-2", input)
			require.NoError(t, err)

			assert.Equal(t, tt.wantError, result.Error)

			if tt.wantCauseContains != "" {
				assert.Contains(t, result.Cause, tt.wantCauseContains)
			}

			if tt.mock != nil && tt.wantError == "" {
				if tt.wantJobName != "" {
					assert.Equal(t, tt.wantJobName, tt.mock.capturedJob)
				}

				if tt.wantJobRunID != "" {
					out, ok := result.Output.(map[string]any)
					require.True(t, ok, "expected map output")
					assert.Equal(t, tt.wantJobRunID, out["JobRunId"])
				}
			}
		})
	}
}

// --- EventBridge PutEvents tests ---

func TestSFN_EventBridgePutEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		def               string
		mock              *mockEventBridge
		input             string
		wantError         string
		wantCauseContains string
		wantEntriesCount  int
	}{
		{
			name: "puts_events",
			def: `{
				"StartAt": "Put",
				"States": {
					"Put": {
						"Type": "Task",
						"Resource": "arn:aws:states:::events:putEvents",
						"End": true
					}
				}
			}`,
			mock: &mockEventBridge{returnFailCount: 0},
			input: `{"Entries": [
				{"Source": "my.app", "DetailType": "Order", "Detail": "{\"id\":1}"},
				{"Source": "my.app", "DetailType": "Order", "Detail": "{\"id\":2}"}
			]}`,
			wantEntriesCount: 2,
		},
		{
			name: "eventbridge_integration_not_configured",
			def: `{
				"StartAt": "Put",
				"States": {
					"Put": {
						"Type": "Task",
						"Resource": "arn:aws:states:::events:putEvents",
						"End": true
					}
				}
			}`,
			mock:              nil,
			wantError:         "TaskFailed",
			wantCauseContains: asl.ErrEventBridgeIntegrationNotConfigured.Error(),
		},
		{
			name: "eventbridge_unsupported_action",
			def: `{
				"StartAt": "Desc",
				"States": {
					"Desc": {
						"Type": "Task",
						"Resource": "arn:aws:states:::events:describeRule",
						"End": true
					}
				}
			}`,
			mock:              &mockEventBridge{},
			wantError:         "TaskFailed",
			wantCauseContains: "unsupported EventBridge action",
		},
		{
			name: "aws_sdk_events_resource",
			def: `{
				"StartAt": "Put",
				"States": {
					"Put": {
						"Type": "Task",
						"Resource": "arn:aws:states:::aws-sdk:events:putEvents",
						"End": true
					}
				}
			}`,
			mock:             &mockEventBridge{},
			input:            `{"Entries": [{"Source": "my.app", "DetailType": "Test", "Detail": "{}"}]}`,
			wantEntriesCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sm, err := asl.Parse(tt.def)
			require.NoError(t, err)

			exec := asl.NewExecutor(sm, nil, nil)
			if tt.mock != nil {
				exec.SetEventBridgeIntegration(tt.mock)
			}

			input := tt.input
			if input == "" {
				input = `{}`
			}

			result, err := exec.Execute(t.Context(), "exec-3", input)
			require.NoError(t, err)

			assert.Equal(t, tt.wantError, result.Error)

			if tt.wantCauseContains != "" {
				assert.Contains(t, result.Cause, tt.wantCauseContains)
			}

			if tt.mock != nil && tt.wantError == "" && tt.wantEntriesCount > 0 {
				assert.Len(t, tt.mock.capturedEntries, tt.wantEntriesCount)
			}
		})
	}
}

// --- Unsupported integration (API GW / EMR) returns TaskFailed ---

func TestSFN_UnsupportedIntegrationTaskFailed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resource string
	}{
		{"apigateway", "arn:aws:states:::apigateway:invoke"},
		{"emr", "arn:aws:states:::elasticmapreduce:startJobRun"},
		{"emr_serverless", "arn:aws:states:::emr-serverless:startJobRun"},
		{"unknown_service", "arn:aws:states:::unknownservice:doThing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			def := `{
				"StartAt": "Task",
				"States": {
					"Task": {
						"Type": "Task",
						"Resource": "` + tt.resource + `",
						"End": true
					}
				}
			}`

			sm, err := asl.Parse(def)
			require.NoError(t, err)

			exec := asl.NewExecutor(sm, nil, nil)
			result, err := exec.Execute(t.Context(), "exec-unsupported", `{}`)
			require.NoError(t, err)

			assert.Equal(
				t,
				"TaskFailed",
				result.Error,
				"expected States.TaskFailed for %s",
				tt.resource,
			)
			assert.NotEmpty(t, result.Cause)
		})
	}
}

// --- .sync pattern support (gopherstack-tdp6) ---

// TestSFN_SyncPatternUnsupportedForRequestResponseOnlyServices verifies that a
// ".sync" Resource suffix is rejected with States.TaskFailed for services AWS
// documents as Request-Response only (the "Run a Job - .sync" column reads
// "Not supported" for these rows in the "AWS SDK integrations in Step
// Functions" and "Optimized integrations" tables at
// https://docs.aws.amazon.com/step-functions/latest/dg/connect-to-resource.html).
// Before this check existed, gopherstack's serviceAction() stripped ".sync"
// unconditionally and ran these as plain request/response, silently accepting
// a pattern real Step Functions would reject.
func TestSFN_SyncPatternUnsupportedForRequestResponseOnlyServices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resource string
	}{
		{"lambda", "arn:aws:states:::lambda:invoke.sync"},
		{"sqs", "arn:aws:states:::sqs:sendMessage.sync"},
		{"sns", "arn:aws:states:::sns:publish.sync"},
		{"dynamodb", "arn:aws:states:::dynamodb:putItem.sync"},
		{"eventbridge", "arn:aws:states:::events:putEvents.sync"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			def := `{
				"StartAt": "Task",
				"States": {
					"Task": {
						"Type": "Task",
						"Resource": "` + tt.resource + `",
						"End": true
					}
				}
			}`

			sm, err := asl.Parse(def)
			require.NoError(t, err)

			exec := asl.NewExecutor(sm, nil, nil)

			result, err := exec.Execute(t.Context(), "exec-sync-unsupported", `{}`)
			require.NoError(t, err)

			assert.Equal(t, "TaskFailed", result.Error, "expected States.TaskFailed for %s", tt.resource)
			assert.Contains(t, result.Cause, asl.ErrSyncPatternUnsupported.Error())
		})
	}
}

// TestSFN_SyncPatternSupportedForJobServices verifies ECS and Glue -- the two
// services AWS documents as supporting "Run a Job (.sync)" -- are unaffected
// by the pattern-support check and still dispatch normally.
func TestSFN_SyncPatternSupportedForJobServices(t *testing.T) {
	t.Parallel()

	def := `{
		"StartAt": "Run",
		"States": {
			"Run": {
				"Type": "Task",
				"Resource": "arn:aws:states:::ecs:runTask.sync",
				"End": true
			}
		}
	}`

	sm, err := asl.Parse(def)
	require.NoError(t, err)

	exec := asl.NewExecutor(sm, nil, nil)
	exec.SetECSIntegration(&mockECS{returnOutput: map[string]any{"Tasks": []any{}, "Failures": []any{}}})

	result, err := exec.Execute(t.Context(), "exec-sync-supported", `{"TaskDefinition": "td"}`)
	require.NoError(t, err)

	assert.Empty(t, result.Error)
}
