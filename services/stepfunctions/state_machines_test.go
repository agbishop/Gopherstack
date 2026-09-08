package stepfunctions_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/stepfunctions"
)

const passDefinition = `{
"StartAt": "P",
"States": {
"P": {"Type": "Pass", "End": true}
}
}`

func TestCreateStateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr             error
		name                string
		smName              string
		definition          string
		preCreateDefinition string // if set, used for the preCreate call instead of definition
		roleArn             string
		smType              string
		wantName            string
		wantStatus          string
		wantType            string
		preCreate           bool
	}{
		{
			name:       "basic",
			smName:     "my-sm",
			definition: passDefinition,
			roleArn:    "arn:aws:iam::123456789012:role/role",
			smType:     "STANDARD",
			wantName:   "my-sm",
			wantStatus: "ACTIVE",
			wantType:   "STANDARD",
		},
		{
			name:       "DefaultType",
			smName:     "typed-sm",
			definition: passDefinition,
			roleArn:    "arn:role",
			smType:     "",
			wantType:   "STANDARD",
		},
		{
			name:                "AlreadyExists",
			smName:              "dup-sm",
			preCreateDefinition: passDefinition,
			definition:          `{"StartAt":"T","States":{"T":{"Type":"Succeed"}}}`,
			roleArn:             "arn:role",
			smType:              "STANDARD",
			preCreate:           true,
			wantErr:             stepfunctions.ErrStateMachineAlreadyExists,
		},
		{
			name:       "InvalidDefinition",
			smName:     "invalid-sm",
			definition: `{}`,
			roleArn:    "arn:role",
			smType:     "STANDARD",
			wantErr:    stepfunctions.ErrInvalidDefinition,
		},
		{
			name:       "InvalidRoleArn",
			smName:     "invalid-role",
			definition: passDefinition,
			roleArn:    "not-an-arn",
			smType:     "STANDARD",
			wantErr:    stepfunctions.ErrInvalidRoleArn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

			if tt.preCreate {
				preCreateDef := tt.definition
				if tt.preCreateDefinition != "" {
					preCreateDef = tt.preCreateDefinition
				}
				_, err := b.CreateStateMachine(context.Background(), tt.smName, preCreateDef, tt.roleArn, tt.smType)
				require.NoError(t, err)
			}

			sm, err := b.CreateStateMachine(context.Background(), tt.smName, tt.definition, tt.roleArn, tt.smType)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)

			if tt.wantName != "" {
				assert.Equal(t, tt.wantName, sm.Name)
				assert.Contains(t, sm.StateMachineArn, tt.wantName)
			}
			if tt.wantStatus != "" {
				assert.Equal(t, tt.wantStatus, sm.Status)
			}
			if tt.wantType != "" {
				assert.Equal(t, tt.wantType, sm.Type)
			}
		})
	}
}

func TestDescribeStateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr    error
		name       string
		createName string
		createDef  string
		createType string
		descArn    string
		wantName   string
		wantType   string
		wantDef    string
	}{
		{
			name:       "success",
			createName: "desc-sm",
			createDef:  passDefinition,
			createType: "EXPRESS",
			wantName:   "desc-sm",
			wantType:   "EXPRESS",
			wantDef:    passDefinition,
		},
		{
			name:    "NotFound",
			descArn: "arn:aws:states:us-east-1:123:stateMachine:nonexistent",
			wantErr: stepfunctions.ErrStateMachineDoesNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

			arn := tt.descArn
			if tt.createName != "" {
				sm, err := b.CreateStateMachine(
					context.Background(),
					tt.createName,
					tt.createDef,
					"arn:role",
					tt.createType,
				)
				require.NoError(t, err)
				arn = sm.StateMachineArn
			}

			got, err := b.DescribeStateMachine(arn)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, got.Name)
			assert.Equal(t, tt.wantType, got.Type)
			assert.JSONEq(t, tt.wantDef, got.Definition)
		})
	}
}

func TestListStateMachines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		token      string
		setupNames []string
		maxResults int
		wantCount  int
		wantNext   bool
	}{
		{
			name:       "basic",
			setupNames: []string{"alpha-sm", "beta-sm"},
			wantCount:  2,
		},
		{
			// nextToken beyond size returns empty
			name:      "EmptyToken",
			token:     "999",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

			for _, name := range tt.setupNames {
				_, _ = b.CreateStateMachine(context.Background(), name, passDefinition, "arn:role", "STANDARD")
			}

			sms, next, err := b.ListStateMachines(context.Background(), tt.token, tt.maxResults)
			require.NoError(t, err)
			assert.Len(t, sms, tt.wantCount)
			if tt.wantNext {
				assert.NotEmpty(t, next)
			} else {
				assert.Empty(t, next)
			}
		})
	}

	t.Run("Pagination", func(t *testing.T) {
		t.Parallel()
		b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

		for i := range 5 {
			_, _ = b.CreateStateMachine(
				context.Background(),
				"sm-"+string(rune('a'+i)), passDefinition, "arn:role", "STANDARD",
			)
		}

		page1, next, err := b.ListStateMachines(context.Background(), "", 2)
		require.NoError(t, err)
		assert.Len(t, page1, 2)
		assert.NotEmpty(t, next)

		page2, next2, err := b.ListStateMachines(context.Background(), next, 2)
		require.NoError(t, err)
		assert.Len(t, page2, 2)
		assert.NotEmpty(t, next2)

		page3, next3, err := b.ListStateMachines(context.Background(), next2, 2)
		require.NoError(t, err)
		assert.Len(t, page3, 1)
		assert.Empty(t, next3)
	})
}

func TestDeleteStateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		name      string
		deleteArn string
		createSM  bool
	}{
		{
			name:     "success",
			createSM: true,
		},
		{
			// AWS: DeleteStateMachine's own error switch models only InvalidArn
			// and ValidationException -- no StateMachineDoesNotExist -- so it is
			// idempotent on a missing state machine.
			name:      "NotFoundIsIdempotent",
			deleteArn: "arn:aws:states:us-east-1:123:stateMachine:nonexistent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

			arn := tt.deleteArn
			if tt.createSM {
				sm, err := b.CreateStateMachine(
					context.Background(),
					"to-delete",
					passDefinition,
					"arn:role",
					"STANDARD",
				)
				require.NoError(t, err)
				arn = sm.StateMachineArn
			}

			err := b.DeleteStateMachine(arn)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)

			_, err = b.DescribeStateMachine(arn)
			require.ErrorIs(t, err, stepfunctions.ErrStateMachineDoesNotExist)
		})
	}
}

// TestDeleteStateMachine_DeletingObservableWhileExecutionRunning is the
// gopherstack-kx95 regression test: AWS's DeleteStateMachine is documented
// as asynchronous -- "It sets the state machine's status to DELETING ...
// A state machine is deleted only when all its executions are completed"
// (sfn service-2.json). While a state machine has a running execution, its
// DELETING status must be observable via DescribeStateMachine, and once
// there are no running executions left, the sweep must complete the
// physical removal.
func TestDeleteStateMachine_DeletingObservableWhileExecutionRunning(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	sm, err := b.CreateStateMachine(context.Background(), "deleting-sm", waitDefinition, "arn:role", "STANDARD")
	require.NoError(t, err)

	smARN := sm.StateMachineArn

	exec, err := b.StartExecution(smARN, "keep-alive", "")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = b.StopExecution(exec.ExecutionArn, "Test", "cleanup")
	})

	require.Eventually(t, func() bool {
		d, dErr := b.DescribeExecution(exec.ExecutionArn)

		return dErr == nil && d.Status == "RUNNING"
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, b.DeleteStateMachine(smARN))

	desc, err := b.DescribeStateMachine(smARN)
	require.NoError(t, err, "DELETING state machine must still be describable")
	assert.Equal(t, "DELETING", desc.Status)

	_, err = b.CreateStateMachine(context.Background(), "deleting-sm", waitDefinition, "arn:role", "STANDARD")
	require.ErrorIs(t, err, stepfunctions.ErrStateMachineDeleting)

	require.NoError(t, b.StopExecution(exec.ExecutionArn, "Test", "cleanup"))
	require.Eventually(t, func() bool {
		d, dErr := b.DescribeExecution(exec.ExecutionArn)

		return dErr == nil && d.Status != "RUNNING"
	}, 5*time.Second, 10*time.Millisecond)

	swept := b.SweepDeletingStateMachines(context.Background())
	assert.Equal(t, 1, swept)

	_, err = b.DescribeStateMachine(smARN)
	require.ErrorIs(t, err, stepfunctions.ErrStateMachineDoesNotExist)

	freshSM, err := b.CreateStateMachine(context.Background(), "deleting-sm", waitDefinition, "arn:role", "STANDARD")
	require.NoError(t, err, "name must be reusable once the sweep completes deletion")

	t.Cleanup(func() {
		_ = b.DeleteStateMachine(freshSM.StateMachineArn)
	})
}

// TestStateMachineDeleting_BlocksClientCallableOps is the gopherstack-kx95
// regression test for the error side: AWS declares StateMachineDeleting on
// CreateStateMachine, CreateStateMachineAlias, ListStateMachineAliases,
// PublishStateMachineVersion, StartExecution, StartSyncExecution,
// UpdateStateMachine, and UpdateStateMachineAlias (aws-sdk-go-v2/service/sfn
// deserializers.go; botocore stepfunctions service-2.json). Every one of
// those must surface it -- not DoesNotExist, not silent success -- while the
// target state machine is mid-deletion.
func TestStateMachineDeleting_BlocksClientCallableOps(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	sm, err := b.CreateStateMachine(context.Background(), "blocked-sm", waitDefinition, "arn:role", "EXPRESS")
	require.NoError(t, err)

	smARN := sm.StateMachineArn

	version, err := b.PublishStateMachineVersion(smARN, "v1", "")
	require.NoError(t, err)

	alias, err := b.CreateStateMachineAlias(smARN, "live", "", []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: version.StateMachineVersionArn, Weight: 100},
	})
	require.NoError(t, err)

	exec, err := b.StartExecution(smARN, "keep-alive", "")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = b.StopExecution(exec.ExecutionArn, "Test", "cleanup")
	})

	require.Eventually(t, func() bool {
		d, dErr := b.DescribeExecution(exec.ExecutionArn)

		return dErr == nil && d.Status == "RUNNING"
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, b.DeleteStateMachine(smARN))

	tests := []struct {
		call func() error
		name string
	}{
		{
			name: "create_state_machine_same_name",
			call: func() error {
				_, callErr := b.CreateStateMachine(
					context.Background(), "blocked-sm", waitDefinition, "arn:role", "EXPRESS",
				)

				return callErr
			},
		},
		{
			name: "create_alias",
			call: func() error {
				_, callErr := b.CreateStateMachineAlias(smARN, "second", "", []stepfunctions.AliasRoutingConfig{
					{StateMachineVersionArn: version.StateMachineVersionArn, Weight: 100},
				})

				return callErr
			},
		},
		{
			name: "update_alias",
			call: func() error {
				_, callErr := b.UpdateStateMachineAlias(alias.StateMachineAliasArn, "updated", nil)

				return callErr
			},
		},
		{
			name: "list_aliases",
			call: func() error {
				_, _, callErr := b.ListStateMachineAliases(smARN, "", 100)

				return callErr
			},
		},
		{
			name: "publish_version",
			call: func() error {
				_, callErr := b.PublishStateMachineVersion(smARN, "v2", "")

				return callErr
			},
		},
		{
			name: "start_execution",
			call: func() error {
				_, callErr := b.StartExecution(smARN, "second-exec", "")

				return callErr
			},
		},
		{
			name: "start_sync_execution",
			call: func() error {
				_, callErr := b.StartSyncExecution(smARN, "sync-exec", "")

				return callErr
			},
		},
		{
			name: "update_state_machine",
			call: func() error {
				_, _, callErr := b.UpdateStateMachine(smARN, "", "")

				return callErr
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotErr := tt.call()
			require.ErrorIs(t, gotErr, stepfunctions.ErrStateMachineDeleting)
		})
	}
}

func TestBackend_ValidateName_StateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		name    string
		smName  string
	}{
		{
			name:    "valid_name",
			smName:  "good-sm",
			wantErr: nil,
		},
		{
			name:    "name_with_space_allowed",
			smName:  "my sm",
			wantErr: nil,
		},
		{
			name:    "empty_name_invalid",
			smName:  "",
			wantErr: stepfunctions.ErrInvalidName,
		},
		{
			name:    "name_too_long_invalid",
			smName:  strings.Repeat("x", 81),
			wantErr: stepfunctions.ErrInvalidName,
		},
		{
			name:    "name_with_dollar_invalid",
			smName:  "my$sm",
			wantErr: stepfunctions.ErrInvalidName,
		},
		{
			name:    "name_with_bracket_invalid",
			smName:  "my[sm]",
			wantErr: stepfunctions.ErrInvalidName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := stepfunctions.NewInMemoryBackend()
			_, err := b.CreateStateMachine(
				context.Background(),
				tt.smName,
				sfnPassDefinition,
				"arn:aws:iam::123456789012:role/role",
				"STANDARD",
			)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestARN_StateMachine(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	sm, err := b.CreateStateMachine(
		context.Background(),
		"my-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:states:us-east-1:123456789012:stateMachine:my-sm", sm.StateMachineArn)
}

func TestCreateStateMachine_DefaultTypeIsSTANDARD(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(context.Background(), "t1", minimalDefinition, validRoleARN, "")
	require.NoError(t, err)
	assert.Equal(t, "STANDARD", sm.Type)
}

func TestCreateStateMachine_Express(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"exp1",
		minimalDefinition,
		validRoleARN,
		"EXPRESS",
	)
	require.NoError(t, err)
	assert.Equal(t, "EXPRESS", sm.Type)
}

func TestCreateStateMachine_StatusIsActive(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"status-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)
	assert.Equal(t, "ACTIVE", sm.Status)
}

func TestCreateStateMachine_CreationDateSet(t *testing.T) {
	t.Parallel()

	before := float64(time.Now().Unix())
	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"date-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, sm.CreationDate, before)
}

func TestCreateStateMachine_DuplicateNameDiffDefReturnsError(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	_, err := b.CreateStateMachine(
		context.Background(),
		"dup-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	altDef := `{"StartAt":"T","States":{"T":{"Type":"Succeed"}}}`
	_, err = b.CreateStateMachine(context.Background(), "dup-sm", altDef, validRoleARN, "STANDARD")
	require.Error(t, err)
	assert.ErrorIs(t, err, stepfunctions.ErrStateMachineAlreadyExists)
}

func TestCreateStateMachine_InvalidDefinition(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	_, err := b.CreateStateMachine(
		context.Background(),
		"bad-def",
		`{"not":"valid-asl"}`,
		validRoleARN,
		"STANDARD",
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, stepfunctions.ErrInvalidDefinition)
}

func TestDescribeStateMachine_NotFound(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	_, err := b.DescribeStateMachine("arn:aws:states:us-east-1:123:stateMachine:none")
	require.Error(t, err)
	assert.ErrorIs(t, err, stepfunctions.ErrStateMachineDoesNotExist)
}

func TestDeleteStateMachine_RemovesStateMachine(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"del-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	require.NoError(t, b.DeleteStateMachine(sm.StateMachineArn))

	_, err = b.DescribeStateMachine(sm.StateMachineArn)
	assert.ErrorIs(t, err, stepfunctions.ErrStateMachineDoesNotExist)
}

// AWS: DeleteStateMachine's own error switch models only InvalidArn and
// ValidationException -- no StateMachineDoesNotExist -- so it is idempotent
// on a missing state machine.
func TestDeleteStateMachine_NotFound(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	err := b.DeleteStateMachine("arn:aws:states:us-east-1:123:stateMachine:ghost")
	require.NoError(t, err)
}

func TestUpdateStateMachine_UpdatesDefinition(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"upd-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	newDef := `{"StartAt":"S2","States":{"S2":{"Type":"Pass","End":true}}}`
	_, _, err = b.UpdateStateMachine(sm.StateMachineArn, newDef, "")
	require.NoError(t, err)

	updated, err := b.DescribeStateMachine(sm.StateMachineArn)
	require.NoError(t, err)
	assert.Equal(t, newDef, updated.Definition)
}

func TestListStateMachines_ReturnsSorted(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	for _, name := range []string{"zz-sm", "aa-sm", "mm-sm"} {
		_, err := b.CreateStateMachine(
			context.Background(),
			name,
			minimalDefinition,
			validRoleARN,
			"STANDARD",
		)
		require.NoError(t, err)
	}

	sms, next, err := b.ListStateMachines(context.Background(), "", 100)
	require.NoError(t, err)
	assert.Empty(t, next)
	require.Len(t, sms, 3)
	assert.Equal(t, "aa-sm", sms[0].Name)
	assert.Equal(t, "mm-sm", sms[1].Name)
	assert.Equal(t, "zz-sm", sms[2].Name)
}

func TestListStateMachines_Pagination(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	for i := range 5 {
		name := fmt.Sprintf("pag-sm-%02d", i)
		_, err := b.CreateStateMachine(
			context.Background(),
			name,
			minimalDefinition,
			validRoleARN,
			"STANDARD",
		)
		require.NoError(t, err)
	}

	page1, next, err := b.ListStateMachines(context.Background(), "", 2)
	require.NoError(t, err)
	require.NotEmpty(t, next)
	assert.Len(t, page1, 2)

	page2, next2, err := b.ListStateMachines(context.Background(), next, 2)
	require.NoError(t, err)
	require.NotEmpty(t, next2)
	assert.Len(t, page2, 2)

	page3, next3, err := b.ListStateMachines(context.Background(), next2, 2)
	require.NoError(t, err)
	assert.Empty(t, next3)
	assert.Len(t, page3, 1)
}

// ─── Name Validation ──────────────────────────────────────────────────────────

func TestStateMachineName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		smName  string
		wantErr bool
	}{
		{name: "too_long", smName: strings.Repeat("a", 81), wantErr: true},
		{name: "exact_max_length", smName: strings.Repeat("a", 80)},
		{name: "empty", smName: "", wantErr: true},
		{name: "invalid_chars_angle_open", smName: "has<angle", wantErr: true},
		{name: "invalid_chars_angle_close", smName: "has>angle", wantErr: true},
		{name: "invalid_chars_brace_open", smName: "has{brace", wantErr: true},
		{name: "invalid_chars_brace_close", smName: "has}brace", wantErr: true},
		{name: "valid_dash", smName: "has-dash"},
		{name: "valid_underscore", smName: "has_underscore"},
		{name: "valid_dot", smName: "has.dot"},
		{name: "valid_plus", smName: "has+plus"},
		{name: "valid_eq", smName: "has=eq"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := stepfunctions.NewInMemoryBackend()
			sm, err := b.CreateStateMachine(
				context.Background(),
				tt.smName,
				minimalDefinition,
				validRoleARN,
				"STANDARD",
			)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, stepfunctions.ErrInvalidName)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.smName, sm.Name)
		})
	}
}

func TestCreateStateMachine_Idempotent_SameParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		smType string
	}{
		{name: "standard_type", smType: "STANDARD"},
		{name: "express_type", smType: "EXPRESS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			sm1, err := b.CreateStateMachine(
				context.Background(),
				"idem-sm-"+tt.name,
				minimalDefinition,
				validRoleARN,
				tt.smType,
			)
			require.NoError(t, err)

			// Same name+def+roleArn+type → idempotent, returns same ARN.
			sm2, err := b.CreateStateMachine(
				context.Background(),
				"idem-sm-"+tt.name,
				minimalDefinition,
				validRoleARN,
				tt.smType,
			)
			require.NoError(t, err)
			assert.Equal(t, sm1.StateMachineArn, sm2.StateMachineArn)

			// Only one state machine stored.
			assert.Equal(t, 1, b.StateMachineCount())
		})
	}
}

func TestCreateStateMachine_SameName_DiffDef_Errors(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	_, err := b.CreateStateMachine(context.Background(), "conflict-sm", minimalDefinition, validRoleARN, "STANDARD")
	require.NoError(t, err)

	altDef := `{"StartAt":"T","States":{"T":{"Type":"Succeed"}}}`
	_, err = b.CreateStateMachine(context.Background(), "conflict-sm", altDef, validRoleARN, "STANDARD")
	require.ErrorIs(t, err, stepfunctions.ErrStateMachineAlreadyExists)
}

func TestCreateStateMachine_SameName_DiffRole_Errors(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	_, err := b.CreateStateMachine(
		context.Background(),
		"role-conflict-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	altRole := "arn:aws:iam::000000000000:role/other-role"
	_, err = b.CreateStateMachine(context.Background(), "role-conflict-sm", minimalDefinition, altRole, "STANDARD")
	require.ErrorIs(t, err, stepfunctions.ErrStateMachineAlreadyExists)
}

func TestCreateStateMachine_SameName_DiffType_Errors(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	_, err := b.CreateStateMachine(
		context.Background(),
		"type-conflict-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	_, err = b.CreateStateMachine(context.Background(), "type-conflict-sm", minimalDefinition, validRoleARN, "EXPRESS")
	require.ErrorIs(t, err, stepfunctions.ErrStateMachineAlreadyExists)
}

// ─── Alias Routing Weight Validation ─────────────────────────────────────────

// TestDeleteStateMachine_TombstoneOnlyRunning verifies that completed executions
// do not accumulate tombstones when a state machine is deleted.
func TestDeleteStateMachine_TombstoneOnlyRunning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		waitForCompletion bool
		wantTombstone     bool
	}{
		{
			name:              "completed_execution_no_tombstone",
			waitForCompletion: true,
			wantTombstone:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newSFBackend()
			sm, err := b.CreateStateMachine(context.Background(), "tomb-sm", exprPassDef, "arn:role", "STANDARD")
			require.NoError(t, err)

			exec, err := b.StartExecution(sm.StateMachineArn, "tomb-exec", "{}")
			require.NoError(t, err)

			execARN := exec.ExecutionArn

			if tt.waitForCompletion {
				require.Eventually(t, func() bool {
					d, e := b.DescribeExecution(execARN)

					return e == nil && d.Status != "RUNNING"
				}, 5*time.Second, 50*time.Millisecond)
			}

			err = b.DeleteStateMachine(sm.StateMachineArn)
			require.NoError(t, err)

			hasTombstone := b.HasTombstoneForTest(execARN)
			assert.Equal(t, tt.wantTombstone, hasTombstone)
		})
	}
}

// TestUpdateStateMachine verifies state machine definition and roleArn updates.
func TestUpdateStateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errIs         error
		name          string
		newDefinition string
		newRoleArn    string
		checkDef      string
		checkRole     string
		wantErr       bool
	}{
		{
			name: "update_definition",
			newDefinition: `{
"StartAt": "S",
"States": {"S": {"Type": "Succeed"}}}`,
			newRoleArn: "",
			checkDef:   "Succeed",
			checkRole:  "arn:aws:iam::123456789012:role/original",
		},
		{
			name:          "update_role_only",
			newDefinition: "",
			newRoleArn:    "arn:aws:iam::123456789012:role/new-role",
			checkRole:     "arn:aws:iam::123456789012:role/new-role",
		},
		{
			name:          "invalid_definition",
			newDefinition: `{}`,
			wantErr:       true,
			errIs:         stepfunctions.ErrInvalidDefinition,
		},
		{
			name:       "invalid_role_arn",
			newRoleArn: "invalid-role",
			wantErr:    true,
			errIs:      stepfunctions.ErrInvalidRoleArn,
		},
		{
			name:    "nonexistent_sm",
			wantErr: true,
			errIs:   stepfunctions.ErrStateMachineDoesNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newSFBackend()

			var smARN string
			if !errors.Is(tt.errIs, stepfunctions.ErrStateMachineDoesNotExist) {
				sm, smErr := b.CreateStateMachine(
					context.Background(),
					"update-sm", exprPassDef,
					"arn:aws:iam::123456789012:role/original", "STANDARD",
				)
				require.NoError(t, smErr)
				smARN = sm.StateMachineArn
			} else {
				smARN = "arn:aws:states:us-east-1:123456789012:stateMachine:nonexistent"
			}

			updateDate, revisionID, err := b.UpdateStateMachine(smARN, tt.newDefinition, tt.newRoleArn)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					require.ErrorIs(t, err, tt.errIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Greater(t, updateDate, float64(0))
			assert.NotEmpty(t, revisionID)

			sm, err := b.DescribeStateMachine(smARN)
			require.NoError(t, err)
			assert.Greater(t, sm.UpdatedDate, float64(0))

			if tt.checkDef != "" {
				assert.Contains(t, sm.Definition, tt.checkDef)
			}
			if tt.checkRole != "" {
				assert.Equal(t, tt.checkRole, sm.RoleArn)
			}
		})
	}
}

const sfnPassDefinition = `{
"StartAt": "Step1",
"States": {
  "Step1": {"Type": "Pass", "End": true}
}}`

const sfnFailDefinition = `{
"StartAt": "Step1",
"States": {
  "Step1": {"Type": "Fail", "Error": "MyErr", "Cause": "test reason"}
}}`

// ---- getTags: resource with no tags vs. with tags ----

// TestRefinement1_CreateStateMachineAlreadyExists verifies duplicate state machine creation with
// different definition returns error; same definition is idempotent.
func TestCreateStateMachineAlreadyExists(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	_, err := b.CreateStateMachine(context.Background(), "sm1", validPassDef, "arn:role", "STANDARD")
	require.NoError(t, err)

	altDef := `{"StartAt":"T","States":{"T":{"Type":"Succeed"}}}`
	_, err = b.CreateStateMachine(context.Background(), "sm1", altDef, "arn:role", "STANDARD")
	require.Error(t, err)
	assert.ErrorIs(t, err, stepfunctions.ErrStateMachineAlreadyExists)
}

// TestRefinement1_DeleteStateMachineNotFound verifies deleting nonexistent SM returns error.
// AWS: DeleteStateMachine's own error switch models only InvalidArn and
// ValidationException -- no StateMachineDoesNotExist -- so it is idempotent
// on a missing state machine.
func TestDeleteStateMachineNotFound(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	err := b.DeleteStateMachine("arn:aws:states:us-east-1:123:stateMachine:nonexistent")
	require.NoError(t, err)
}

// TestRefinement1_DescribeStateMachineNotFound verifies describing nonexistent SM returns error.
func TestDescribeStateMachineNotFound(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	_, err := b.DescribeStateMachine("arn:aws:states:us-east-1:123:stateMachine:nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, stepfunctions.ErrStateMachineDoesNotExist)
}

// TestRefinement1_UpdateStateMachineInvalidDefinition verifies bad definition is rejected.
func TestUpdateStateMachineInvalidDefinition(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(context.Background(), "sm1", validPassDef, "arn:role", "STANDARD")
	require.NoError(t, err)

	_, _, err = b.UpdateStateMachine(sm.StateMachineArn, `{"StartAt":"Missing"}`, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, stepfunctions.ErrInvalidDefinition)
}

// TestCreateStateMachine_InvalidJitterStrategyRejected verifies AWS's
// documented Retry.JitterStrategy enum ("FULL" or "NONE" only) is enforced
// at CreateStateMachine time, not silently accepted and treated as "NONE".
func TestCreateStateMachine_InvalidJitterStrategyRejected(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	badDef := `{"StartAt":"T","States":{"T":{"Type":"Task",
"Resource":"arn:aws:lambda:us-east-1:123:function:f",
"Retry":[{"ErrorEquals":["States.ALL"],"JitterStrategy":"EXPONENTIAL"}],"End":true}}}`

	_, err := b.CreateStateMachine(context.Background(), "bad-jitter-sm", badDef, validRoleARN, "STANDARD")
	require.Error(t, err)
	assert.ErrorIs(t, err, stepfunctions.ErrInvalidDefinition)
}

// TestRefinement1_CreateStateMachineDefaultType verifies empty type defaults to STANDARD.
func TestCreateStateMachineDefaultType(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(context.Background(), "sm1", validPassDef, "arn:role", "")
	require.NoError(t, err)
	assert.Equal(t, "STANDARD", sm.Type)
}
