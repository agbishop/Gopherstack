package cloudformation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
)

// ---- Backend: DetectStackDrift ----------------------------------------------

func TestBackend_DetectStackDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		name      string
		stackName string
		setup     bool
	}{
		{
			name:      "success",
			stackName: "my-stack",
			setup:     true,
		},
		{
			name:      "stack_not_found",
			stackName: "missing-stack",
			wantErr:   cloudformation.ErrStackNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			if tt.setup {
				_, err := b.CreateStack(t.Context(), tt.stackName, simpleTemplate, nil, cloudformation.StackOptions{})
				require.NoError(t, err)
			}

			detectionID, err := b.DetectStackDrift(tt.stackName)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, detectionID)
		})
	}
}

// ---- Backend: DescribeStackDriftDetectionStatus -----------------------------

func TestBackend_DescribeStackDriftDetectionStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr         error
		name            string
		stackName       string
		detectionIDFn   func(b *cloudformation.InMemoryBackend) string
		wantStatus      string
		wantDriftStatus string
	}{
		{
			name:      "success",
			stackName: "my-stack",
			detectionIDFn: func(b *cloudformation.InMemoryBackend) string {
				id, err := b.DetectStackDrift("my-stack")
				if err != nil {
					return ""
				}

				return id
			},
			wantStatus:      "DETECTION_COMPLETE",
			wantDriftStatus: "IN_SYNC",
		},
		{
			name:      "not_found",
			stackName: "my-stack",
			detectionIDFn: func(_ *cloudformation.InMemoryBackend) string {
				return "nonexistent-detection-id"
			},
			wantErr: cloudformation.ErrDriftDetectionNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			if tt.stackName != "" {
				_, err := b.CreateStack(t.Context(), tt.stackName, simpleTemplate, nil, cloudformation.StackOptions{})
				require.NoError(t, err)
			}

			detectionID := tt.detectionIDFn(b)
			status, err := b.DescribeStackDriftDetectionStatus(detectionID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, status.DetectionStatus)
			assert.Equal(t, tt.wantDriftStatus, status.StackDriftStatus)
		})
	}
}

// ---- Backend: DetectStackResourceDrift -------------------------------------

func TestBackend_DetectStackResourceDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr   error
		name      string
		stackName string
		logicalID string
		setup     bool
	}{
		{
			name:      "success",
			stackName: "my-stack",
			logicalID: "MyBucket",
			setup:     true,
		},
		{
			name:      "stack_not_found",
			stackName: "missing",
			logicalID: "MyBucket",
			wantErr:   cloudformation.ErrStackNotFound,
		},
		{
			name:      "resource_not_found",
			stackName: "my-stack",
			logicalID: "NoSuchResource",
			setup:     true,
			wantErr:   cloudformation.ErrResourceNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			if tt.setup {
				_, err := b.CreateStack(t.Context(), tt.stackName, simpleTemplate, nil, cloudformation.StackOptions{})
				require.NoError(t, err)
			}

			drift, err := b.DetectStackResourceDrift(tt.stackName, tt.logicalID)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.logicalID, drift.LogicalResourceID)
			assert.NotEmpty(t, drift.ResourceType)
			assert.NotEmpty(t, drift.StackID)
			assert.Equal(t, "IN_SYNC", drift.StackResourceDriftStatus)
		})
	}
}

// ---- Backend: DescribeStackResourceDrifts ----------------------------------

func TestBackend_DescribeStackResourceDrifts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr      error
		name         string
		stackName    string
		template     string
		setup        bool
		wantDriftLen int
	}{
		{
			name:         "stack_with_resources",
			stackName:    "my-stack",
			template:     simpleTemplate,
			setup:        true,
			wantDriftLen: 1,
		},
		{
			name:      "stack_not_found",
			stackName: "missing",
			wantErr:   cloudformation.ErrStackNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			if tt.setup {
				_, err := b.CreateStack(t.Context(), tt.stackName, tt.template, nil, cloudformation.StackOptions{})
				require.NoError(t, err)
			}

			drifts, err := b.DescribeStackResourceDrifts(tt.stackName)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Len(t, drifts, tt.wantDriftLen)

			for _, d := range drifts {
				assert.Equal(t, "IN_SYNC", d.StackResourceDriftStatus)
			}
		})
	}
}

// ---- Backend: SetStackPolicy / GetStackPolicy ------------------------------

func TestBackend_StackPolicy(t *testing.T) {
	t.Parallel()

	policy := `{"Statement":[{"Effect":"Allow","Action":"Update:*","Principal":"*","Resource":"*"}]}`

	tests := []struct {
		setErr    error
		getErr    error
		name      string
		stackName string
		policy    string
		setup     bool
		wantEmpty bool
	}{
		{
			name:      "set_and_get",
			stackName: "my-stack",
			policy:    policy,
			setup:     true,
		},
		{
			name:      "get_empty_policy",
			stackName: "my-stack",
			setup:     true,
			wantEmpty: true,
		},
		{
			name:      "set_stack_not_found",
			stackName: "missing",
			setErr:    cloudformation.ErrStackNotFound,
		},
		{
			name:      "get_stack_not_found",
			stackName: "missing",
			getErr:    cloudformation.ErrStackNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			if tt.setup {
				_, err := b.CreateStack(t.Context(), tt.stackName, simpleTemplate, nil, cloudformation.StackOptions{})
				require.NoError(t, err)
			}

			if tt.setErr != nil {
				err := b.SetStackPolicy(tt.stackName, tt.policy)
				require.ErrorIs(t, err, tt.setErr)

				return
			}

			if tt.getErr != nil {
				_, err := b.GetStackPolicy(tt.stackName)
				require.ErrorIs(t, err, tt.getErr)

				return
			}

			if !tt.wantEmpty {
				err := b.SetStackPolicy(tt.stackName, tt.policy)
				require.NoError(t, err)
			}

			got, err := b.GetStackPolicy(tt.stackName)
			require.NoError(t, err)

			if tt.wantEmpty {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tt.policy, got)
			}
		})
	}
}

// ---- Backend: GetTemplateSummary -------------------------------------------

func TestBackend_GetTemplateSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr      error
		name         string
		templateBody string
		stackName    string
		wantDesc     string
		wantParamLen int
		wantResTypes int
		setupStack   bool
	}{
		{
			name:         "from_template_body",
			templateBody: simpleTemplate,
			wantResTypes: 1,
		},
		{
			name:         "from_template_body_with_params",
			templateBody: templateWithParams,
			wantParamLen: 1,
			wantResTypes: 1,
		},
		{
			name:         "from_stack_name",
			stackName:    "my-stack",
			setupStack:   true,
			wantResTypes: 1,
		},
		{
			name:         "empty_body",
			wantResTypes: 0,
		},
		{
			name:      "stack_not_found",
			stackName: "missing",
			wantErr:   cloudformation.ErrStackNotFound,
		},
		{
			name:         "yaml_template",
			templateBody: yamlTemplate,
			wantDesc:     "YAML template",
			wantResTypes: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			if tt.setupStack {
				_, err := b.CreateStack(t.Context(), tt.stackName, simpleTemplate, nil, cloudformation.StackOptions{})
				require.NoError(t, err)
			}

			summary, err := b.GetTemplateSummary(tt.templateBody, tt.stackName)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.NotNil(t, summary)
			assert.Len(t, summary.Parameters, tt.wantParamLen)
			assert.Len(t, summary.ResourceTypes, tt.wantResTypes)

			if tt.wantDesc != "" {
				assert.Equal(t, tt.wantDesc, summary.Description)
			}
		})
	}
}

// ---- Backend: EstimateTemplateCost -----------------------------------------

func TestBackend_EstimateTemplateCost(t *testing.T) {
	t.Parallel()

	b := newBackend()
	url, err := b.EstimateTemplateCost(simpleTemplate, nil)

	require.NoError(t, err)
	assert.NotEmpty(t, url)
	assert.Contains(t, url, "calculator")
}

// ---- Backend: ContinueUpdateRollback ---------------------------------------

func TestBackend_ContinueUpdateRollback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr     error
		name        string
		stackName   string
		forceStatus string
		wantStatus  string
		setup       bool
	}{
		{
			name:        "rollback_in_progress",
			stackName:   "my-stack",
			setup:       true,
			forceStatus: "ROLLBACK_IN_PROGRESS",
			wantStatus:  "ROLLBACK_COMPLETE",
		},
		{
			name:        "update_rollback_in_progress",
			stackName:   "my-stack",
			setup:       true,
			forceStatus: "UPDATE_ROLLBACK_IN_PROGRESS",
			wantStatus:  "UPDATE_ROLLBACK_COMPLETE",
		},
		{
			name:       "no_op_when_create_complete",
			stackName:  "my-stack",
			setup:      true,
			wantStatus: "CREATE_COMPLETE",
		},
		{
			name:      "stack_not_found",
			stackName: "missing",
			wantErr:   cloudformation.ErrStackNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()

			if tt.setup {
				_, err := b.CreateStack(t.Context(), tt.stackName, simpleTemplate, nil, cloudformation.StackOptions{})
				require.NoError(t, err)
			}

			if tt.forceStatus != "" {
				b.ForceStackStatus(tt.stackName, tt.forceStatus)
			}

			err := b.ContinueUpdateRollback(t.Context(), tt.stackName)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			if tt.wantStatus != "" {
				stack, descErr := b.DescribeStack(tt.stackName)
				require.NoError(t, descErr)
				assert.Equal(t, tt.wantStatus, stack.StackStatus)
			}
		})
	}
}

// ---- Backend: CancelUpdateStack --------------------------------------------

func TestBackend_CancelUpdateStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr     error
		name        string
		stackName   string
		forceStatus string
		wantStatus  string
		setup       bool
	}{
		{
			name:        "update_in_progress",
			stackName:   "my-stack",
			setup:       true,
			forceStatus: "UPDATE_IN_PROGRESS",
			wantStatus:  "UPDATE_ROLLBACK_COMPLETE",
		},
		{
			// Real AWS: "You can cancel only stacks that are in the
			// UPDATE_IN_PROGRESS state."
			name:       "rejected_when_create_complete",
			stackName:  "my-stack",
			setup:      true,
			wantErr:    cloudformation.ErrCancelUpdateStackInvalidState,
			wantStatus: "CREATE_COMPLETE",
		},
		{
			name:      "stack_not_found",
			stackName: "missing",
			wantErr:   cloudformation.ErrStackNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			if tt.setup {
				_, err := b.CreateStack(t.Context(), tt.stackName, simpleTemplate, nil, cloudformation.StackOptions{})
				require.NoError(t, err)
			}

			if tt.forceStatus != "" {
				b.ForceStackStatus(tt.stackName, tt.forceStatus)
			}

			err := b.CancelUpdateStack(t.Context(), tt.stackName)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				if tt.wantStatus != "" {
					stack, descErr := b.DescribeStack(tt.stackName)
					require.NoError(t, descErr)
					assert.Equal(t, tt.wantStatus, stack.StackStatus, "status must be unchanged by a rejected cancel")
				}

				return
			}

			require.NoError(t, err)

			if tt.wantStatus != "" {
				stack, descErr := b.DescribeStack(tt.stackName)
				require.NoError(t, descErr)
				assert.Equal(t, tt.wantStatus, stack.StackStatus)
			}
		})
	}
}

// ---- Backend: DescribeAccountLimits ----------------------------------------

func TestBackend_DescribeAccountLimits(t *testing.T) {
	t.Parallel()

	b := newBackend()
	limits := b.DescribeAccountLimits()

	assert.NotEmpty(t, limits)

	for _, l := range limits {
		assert.NotEmpty(t, l.Name)
		assert.Positive(t, l.Value)
	}
}

// ---- Persistence: snapshot/restore includes new state ----------------------

func TestPersistence_SnapshotRestoreWithExtState(t *testing.T) {
	t.Parallel()

	policy := `{"Statement":[{"Effect":"Allow","Action":"Update:*","Principal":"*","Resource":"*"}]}`

	b := newBackend()

	_, err := b.CreateStack(t.Context(), "my-stack", simpleTemplate, nil, cloudformation.StackOptions{})
	require.NoError(t, err)

	err = b.SetStackPolicy("my-stack", policy)
	require.NoError(t, err)

	detectionID, err := b.DetectStackDrift("my-stack")
	require.NoError(t, err)
	assert.NotEmpty(t, detectionID)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := cloudformation.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	gotPolicy, err := fresh.GetStackPolicy("my-stack")
	require.NoError(t, err)
	assert.Equal(t, policy, gotPolicy)

	status, err := fresh.DescribeStackDriftDetectionStatus(detectionID)
	require.NoError(t, err)
	assert.Equal(t, "DETECTION_COMPLETE", status.DetectionStatus)
}

func TestHandler_Snapshot_Restore_Delegation(t *testing.T) {
	t.Parallel()

	h := newHandler()

	_, err := h.Backend.(*cloudformation.InMemoryBackend).CreateStack(
		t.Context(), "snap-stack", simpleTemplate, nil, cloudformation.StackOptions{},
	)
	require.NoError(t, err)

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := newHandler()
	err = h2.Restore(t.Context(), snap)
	require.NoError(t, err)

	stack, err := h2.Backend.(*cloudformation.InMemoryBackend).DescribeStack("snap-stack")
	require.NoError(t, err)
	assert.Equal(t, "snap-stack", stack.StackName)
}

// ---- Backend: UpdateStack with invalid template (covers applyTemplateToStack error) ----

func TestBackend_UpdateStack_InvalidTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		updateBody string
		wantStatus string
	}{
		{
			name:       "invalid_template_body_on_update",
			updateBody: "{bad json}",
			wantStatus: "UPDATE_FAILED",
		},
		{
			name: "import_value_missing_on_update",
			updateBody: `{
				"AWSTemplateFormatVersion": "2010-09-09",
				"Resources": {
					"MyBucket": {
						"Type": "AWS::S3::Bucket",
						"Properties": {
							"BucketName": {"Fn::ImportValue": "NonExistentExport"}
						}
					}
				}
			}`,
			// AWS rolls back to UPDATE_ROLLBACK_COMPLETE on pre-flight import validation failure.
			wantStatus: "UPDATE_ROLLBACK_COMPLETE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			_, err := b.CreateStack(t.Context(), "upd-stack", simpleTemplate, nil, cloudformation.StackOptions{})
			require.NoError(t, err)

			updated, err := b.UpdateStack(t.Context(), "upd-stack", tt.updateBody, nil, cloudformation.StackOptions{})
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, updated.StackStatus)

			// A failed update must record a stack-level event carrying the
			// failure status, exactly like the CreateStack failure paths do —
			// callers polling DescribeStackEvents rely on this.
			evtPage, evErr := b.DescribeStackEvents("upd-stack", "")
			require.NoError(t, evErr)
			events := evtPage.Data
			foundFailureEvent := false
			for _, ev := range events {
				if ev.ResourceStatus == tt.wantStatus && ev.LogicalResourceID == "upd-stack" {
					foundFailureEvent = true

					break
				}
			}
			assert.True(t, foundFailureEvent, "expected a stack-level %s event", tt.wantStatus)
		})
	}
}

// ---- Template: evalAndExpr and evalOrExpr coverage -------------------------

func TestBackend_TemplateConditions_AndOr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		template   string
		wantOutput string
	}{
		{
			name: "fn_and_both_true",
			template: `{
				"AWSTemplateFormatVersion": "2010-09-09",
				"Parameters": {
					"Env": {"Type": "String", "Default": "prod"}
				},
				"Conditions": {
					"IsEnvProd": {"Fn::Equals": [{"Ref": "Env"}, "prod"]},
					"AlwaysTrue": {"Fn::Equals": ["x", "x"]},
					"BothTrue": {"Fn::And": [{"Condition": "IsEnvProd"}, {"Condition": "AlwaysTrue"}]}
				},
				"Resources": {
					"Placeholder": {"Type": "AWS::CloudFormation::WaitConditionHandle", "Properties": {}}
				},
				"Outputs": {
					"Result": {"Value": {"Fn::If": ["BothTrue", "yes", "no"]}}
				}
			}`,
			wantOutput: "yes",
		},
		{
			name: "fn_and_one_false",
			template: `{
				"AWSTemplateFormatVersion": "2010-09-09",
				"Parameters": {
					"Env": {"Type": "String", "Default": "dev"}
				},
				"Conditions": {
					"IsEnvProd": {"Fn::Equals": [{"Ref": "Env"}, "prod"]},
					"AlwaysTrue": {"Fn::Equals": ["x", "x"]},
					"BothTrue": {"Fn::And": [{"Condition": "IsEnvProd"}, {"Condition": "AlwaysTrue"}]}
				},
				"Resources": {
					"Placeholder": {"Type": "AWS::CloudFormation::WaitConditionHandle", "Properties": {}}
				},
				"Outputs": {
					"Result": {"Value": {"Fn::If": ["BothTrue", "yes", "no"]}}
				}
			}`,
			wantOutput: "no",
		},
		{
			name: "fn_or_one_true",
			template: `{
				"AWSTemplateFormatVersion": "2010-09-09",
				"Parameters": {
					"Env": {"Type": "String", "Default": "dev"}
				},
				"Conditions": {
					"IsEnvProd": {"Fn::Equals": [{"Ref": "Env"}, "prod"]},
					"IsEnvDev": {"Fn::Equals": [{"Ref": "Env"}, "dev"]},
					"EitherEnv": {"Fn::Or": [{"Condition": "IsEnvProd"}, {"Condition": "IsEnvDev"}]}
				},
				"Resources": {
					"Placeholder": {"Type": "AWS::CloudFormation::WaitConditionHandle", "Properties": {}}
				},
				"Outputs": {
					"Result": {"Value": {"Fn::If": ["EitherEnv", "yes", "no"]}}
				}
			}`,
			wantOutput: "yes",
		},
		{
			name: "fn_or_all_false",
			template: `{
				"AWSTemplateFormatVersion": "2010-09-09",
				"Parameters": {
					"Env": {"Type": "String", "Default": "staging"}
				},
				"Conditions": {
					"IsEnvProd": {"Fn::Equals": [{"Ref": "Env"}, "prod"]},
					"IsEnvDev": {"Fn::Equals": [{"Ref": "Env"}, "dev"]},
					"EitherEnv": {"Fn::Or": [{"Condition": "IsEnvProd"}, {"Condition": "IsEnvDev"}]}
				},
				"Resources": {
					"Placeholder": {"Type": "AWS::CloudFormation::WaitConditionHandle", "Properties": {}}
				},
				"Outputs": {
					"Result": {"Value": {"Fn::If": ["EitherEnv", "yes", "no"]}}
				}
			}`,
			wantOutput: "no",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			stack, err := b.CreateStack(t.Context(), tt.name, tt.template, nil, cloudformation.StackOptions{})
			require.NoError(t, err)
			require.NotNil(t, stack)

			var gotOutput string
			for _, out := range stack.Outputs {
				if out.OutputKey == "Result" {
					gotOutput = out.OutputValue

					break
				}
			}
			assert.Equal(t, tt.wantOutput, gotOutput)
		})
	}
}

// ---- Backend: resolveStack by StackID (ARN lookup) -------------------------

func TestBackend_ResolveStackByID(t *testing.T) {
	t.Parallel()

	b := newBackend()
	stack, err := b.CreateStack(t.Context(), "my-stack", simpleTemplate, nil, cloudformation.StackOptions{})
	require.NoError(t, err)

	// Look up by StackID (ARN) rather than name
	found, err := b.DescribeStack(stack.StackID)
	require.NoError(t, err)
	assert.Equal(t, "my-stack", found.StackName)
}

// ---- Template: collectImportValuesFromValue with array ----------------------

func TestBackend_ListImports_WithArrayProperty(t *testing.T) {
	t.Parallel()

	// Template that exports a value
	exporterTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"MyBucket": {"Type": "AWS::S3::Bucket", "Properties": {}}
		},
		"Outputs": {
			"BucketOut": {
				"Value": "my-bucket",
				"Export": {"Name": "shared-bucket"}
			}
		}
	}`

	// Template that imports the exported value inside a list property
	importerTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"MyQueue": {
				"Type": "AWS::SQS::Queue",
				"Properties": {
					"Tags": [
						{"Key": "bucket", "Value": {"Fn::ImportValue": "shared-bucket"}}
					]
				}
			}
		}
	}`

	b := newBackend()

	_, err := b.CreateStack(t.Context(), "exporter", exporterTemplate, nil, cloudformation.StackOptions{})
	require.NoError(t, err)

	_, err = b.CreateStack(t.Context(), "importer", importerTemplate, nil, cloudformation.StackOptions{})
	require.NoError(t, err)

	imports, err := b.ListImports("shared-bucket", "")
	require.NoError(t, err)
	assert.Contains(t, imports.Data, "importer")
}
