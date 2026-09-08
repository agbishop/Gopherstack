package pipes_test

// Covers CreatePipe/UpdatePipe required-field validation for the nested
// target union types not covered by targets_test.go's
// TestTargetPartitionKey_Required: ECS TaskDefinitionArn, Batch
// JobDefinition/JobName, Redshift Database/Sqls, SageMaker pipeline
// parameter Name/Value, and Timestream TimeValue/VersionValue/
// DimensionMappings. Unlike source parameters, target parameters route
// through the same validator on both Create and Update, so each case is
// exercised on both paths.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/pipes"
)

func createWithTarget(t *testing.T, name string, tp *pipes.TargetParameters) error {
	t.Helper()

	b := b2Backend()
	_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
		Name:             name,
		RoleARN:          "arn:aws:iam::123456789012:role/r",
		Source:           "arn:aws:sqs:us-east-1:123456789012:q",
		Target:           "arn:aws:ecs:us-east-1:123456789012:cluster/c",
		DesiredState:     "RUNNING",
		TargetParameters: tp,
	})

	return err
}

func updateWithTarget(t *testing.T, name string, tp *pipes.TargetParameters) error {
	t.Helper()

	b := b2Backend()
	_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
		Name:         name,
		RoleARN:      "arn:aws:iam::123456789012:role/r",
		Source:       "arn:aws:sqs:us-east-1:123456789012:q",
		Target:       "arn:aws:lambda:us-east-1:123456789012:function:fn",
		DesiredState: "RUNNING",
	})
	require.NoError(t, err)

	_, err = b.UpdatePipe(context.Background(), name, pipes.UpdatePipeInput{
		RoleARN:          "arn:aws:iam::123456789012:role/r",
		TargetParameters: tp,
	})

	return err
}

// TestTargetEcsTaskDefinitionArnRequired verifies CreatePipe and UpdatePipe
// reject an ECS target with no TaskDefinitionArn, matching aws-sdk-go-v2
// pipes validators.go's validatePipeTargetEcsTaskParameters.
func TestTargetEcsTaskDefinitionArnRequired(t *testing.T) {
	t.Parallel()

	t.Run("create_missing_rejected", func(t *testing.T) {
		t.Parallel()
		err := createWithTarget(t, "ecs-create-missing", &pipes.TargetParameters{
			EcsTaskParameters: &pipes.ECSTaskTargetParameters{},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, pipes.ErrValidation)
	})

	t.Run("create_present_accepted", func(t *testing.T) {
		t.Parallel()
		err := createWithTarget(t, "ecs-create-present", &pipes.TargetParameters{
			EcsTaskParameters: &pipes.ECSTaskTargetParameters{
				TaskDefinitionArn: "arn:aws:ecs:us-east-1:123456789012:task-definition/td",
			},
		})
		assert.NoError(t, err)
	})

	t.Run("update_missing_rejected", func(t *testing.T) {
		t.Parallel()
		err := updateWithTarget(t, "ecs-update-missing", &pipes.TargetParameters{
			EcsTaskParameters: &pipes.ECSTaskTargetParameters{},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, pipes.ErrValidation)
	})
}

// TestTargetBatchJobRequiredFields verifies CreatePipe and UpdatePipe reject
// a Batch target missing JobDefinition or JobName, matching aws-sdk-go-v2
// pipes validators.go's validatePipeTargetBatchJobParameters.
func TestTargetBatchJobRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		bp        *pipes.BatchJobTargetParameters
		name      string
		wantError bool
	}{
		{
			name:      "missing_job_definition",
			bp:        &pipes.BatchJobTargetParameters{JobName: "jn"},
			wantError: true,
		},
		{
			name:      "missing_job_name",
			bp:        &pipes.BatchJobTargetParameters{JobDefinition: "jd"},
			wantError: true,
		},
		{
			name:      "complete",
			bp:        &pipes.BatchJobTargetParameters{JobDefinition: "jd", JobName: "jn"},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run("create_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := createWithTarget(
				t,
				"batch-create-"+tt.name,
				&pipes.TargetParameters{BatchJobParameters: tt.bp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})

		t.Run("update_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := updateWithTarget(
				t,
				"batch-update-"+tt.name,
				&pipes.TargetParameters{BatchJobParameters: tt.bp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTargetRedshiftRequiredFields verifies CreatePipe and UpdatePipe reject
// a Redshift Data target missing Database or Sqls, matching aws-sdk-go-v2
// pipes validators.go's validatePipeTargetRedshiftDataParameters.
func TestTargetRedshiftRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		rp        *pipes.RedshiftDataTargetParameters
		name      string
		wantError bool
	}{
		{
			name:      "missing_database",
			rp:        &pipes.RedshiftDataTargetParameters{Sqls: []string{"select 1"}},
			wantError: true,
		},
		{
			name:      "missing_sqls",
			rp:        &pipes.RedshiftDataTargetParameters{Database: "db"},
			wantError: true,
		},
		{
			name: "complete",
			rp: &pipes.RedshiftDataTargetParameters{
				Database: "db",
				Sqls:     []string{"select 1"},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run("create_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := createWithTarget(
				t,
				"redshift-create-"+tt.name,
				&pipes.TargetParameters{RedshiftDataParameters: tt.rp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})

		t.Run("update_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := updateWithTarget(
				t,
				"redshift-update-"+tt.name,
				&pipes.TargetParameters{RedshiftDataParameters: tt.rp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTargetSageMakerPipelineParameterRequiredFields verifies CreatePipe and
// UpdatePipe reject a SageMaker pipeline target whose PipelineParameterList
// entries are missing Name or Value, matching aws-sdk-go-v2 pipes
// validators.go's validateSageMakerPipelineParameter (nested under
// validatePipeTargetSageMakerPipelineParameters).
func TestTargetSageMakerPipelineParameterRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sp        *pipes.SageMakerPipelineTargetParameters
		name      string
		wantError bool
	}{
		{
			name: "missing_name",
			sp: &pipes.SageMakerPipelineTargetParameters{
				PipelineParameterList: []pipes.SageMakerPipelineParameter{{Value: "v"}},
			},
			wantError: true,
		},
		{
			name: "missing_value",
			sp: &pipes.SageMakerPipelineTargetParameters{
				PipelineParameterList: []pipes.SageMakerPipelineParameter{{Name: "n"}},
			},
			wantError: true,
		},
		{
			name: "complete",
			sp: &pipes.SageMakerPipelineTargetParameters{
				PipelineParameterList: []pipes.SageMakerPipelineParameter{{Name: "n", Value: "v"}},
			},
			wantError: false,
		},
		{
			name:      "empty_list_accepted",
			sp:        &pipes.SageMakerPipelineTargetParameters{},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run("create_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := createWithTarget(
				t,
				"sagemaker-create-"+tt.name,
				&pipes.TargetParameters{SageMakerPipelineParameters: tt.sp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})

		t.Run("update_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := updateWithTarget(
				t,
				"sagemaker-update-"+tt.name,
				&pipes.TargetParameters{SageMakerPipelineParameters: tt.sp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTargetTimestreamRequiredFields verifies CreatePipe and UpdatePipe
// reject a Timestream target missing TimeValue, VersionValue, or
// DimensionMappings (and each DimensionMappings entry's DimensionName/
// DimensionValue/DimensionValueType), matching aws-sdk-go-v2 pipes
// validators.go's validatePipeTargetTimestreamParameters and its nested
// validateDimensionMapping.
func TestTargetTimestreamRequiredFields(t *testing.T) {
	t.Parallel()

	completeDimensions := []pipes.TimestreamDimensionMapping{
		{DimensionName: "dn", DimensionValue: "dv", DimensionValueType: "VARCHAR"},
	}

	tests := []struct {
		tsp       *pipes.TimestreamParameters
		name      string
		wantError bool
	}{
		{
			name: "missing_time_value",
			tsp: &pipes.TimestreamParameters{
				VersionValue:      "v",
				DimensionMappings: completeDimensions,
			},
			wantError: true,
		},
		{
			name: "missing_version_value",
			tsp: &pipes.TimestreamParameters{
				TimeValue:         "t",
				DimensionMappings: completeDimensions,
			},
			wantError: true,
		},
		{
			name:      "missing_dimension_mappings",
			tsp:       &pipes.TimestreamParameters{TimeValue: "t", VersionValue: "v"},
			wantError: true,
		},
		{
			name: "dimension_mapping_missing_name",
			tsp: &pipes.TimestreamParameters{
				TimeValue: "t", VersionValue: "v",
				DimensionMappings: []pipes.TimestreamDimensionMapping{
					{DimensionValue: "dv", DimensionValueType: "VARCHAR"},
				},
			},
			wantError: true,
		},
		{
			name: "dimension_mapping_missing_value",
			tsp: &pipes.TimestreamParameters{
				TimeValue: "t", VersionValue: "v",
				DimensionMappings: []pipes.TimestreamDimensionMapping{
					{DimensionName: "dn", DimensionValueType: "VARCHAR"},
				},
			},
			wantError: true,
		},
		{
			name: "dimension_mapping_missing_value_type",
			tsp: &pipes.TimestreamParameters{
				TimeValue: "t", VersionValue: "v",
				DimensionMappings: []pipes.TimestreamDimensionMapping{
					{DimensionName: "dn", DimensionValue: "dv"},
				},
			},
			wantError: true,
		},
		{
			name: "complete",
			tsp: &pipes.TimestreamParameters{
				TimeValue:         "t",
				VersionValue:      "v",
				DimensionMappings: completeDimensions,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run("create_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := createWithTarget(
				t,
				"timestream-create-"+tt.name,
				&pipes.TargetParameters{TimestreamParameters: tt.tsp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})

		t.Run("update_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := updateWithTarget(
				t,
				"timestream-update-"+tt.name,
				&pipes.TargetParameters{TimestreamParameters: tt.tsp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTargetTimestreamSingleMeasureMappingsRequiredFields verifies CreatePipe
// and UpdatePipe reject a Timestream target whose SingleMeasureMappings
// entries are missing MeasureName, MeasureValue, or MeasureValueType,
// matching aws-sdk-go-v2 pipes validators.go's validateSingleMeasureMapping
// (nested under validatePipeTargetTimestreamParameters).
func TestTargetTimestreamSingleMeasureMappingsRequiredFields(t *testing.T) {
	t.Parallel()

	base := func(sm []pipes.TimestreamSingleMeasureMapping) *pipes.TimestreamParameters {
		return &pipes.TimestreamParameters{
			TimeValue:    "t",
			VersionValue: "v",
			DimensionMappings: []pipes.TimestreamDimensionMapping{
				{DimensionName: "dn", DimensionValue: "dv", DimensionValueType: "VARCHAR"},
			},
			SingleMeasureMappings: sm,
		}
	}

	tests := []struct {
		tsp       *pipes.TimestreamParameters
		name      string
		wantError bool
	}{
		{
			name: "missing_measure_name",
			tsp: base([]pipes.TimestreamSingleMeasureMapping{
				{MeasureValue: "mv", MeasureValueType: "DOUBLE"},
			}),
			wantError: true,
		},
		{
			name: "missing_measure_value",
			tsp: base([]pipes.TimestreamSingleMeasureMapping{
				{MeasureName: "mn", MeasureValueType: "DOUBLE"},
			}),
			wantError: true,
		},
		{
			name: "missing_measure_value_type",
			tsp: base([]pipes.TimestreamSingleMeasureMapping{
				{MeasureName: "mn", MeasureValue: "mv"},
			}),
			wantError: true,
		},
		{
			name: "complete",
			tsp: base([]pipes.TimestreamSingleMeasureMapping{
				{MeasureName: "mn", MeasureValue: "mv", MeasureValueType: "DOUBLE"},
			}),
			wantError: false,
		},
		{
			name:      "omitted_accepted",
			tsp:       base(nil),
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run("create_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := createWithTarget(
				t,
				"ts-single-create-"+tt.name,
				&pipes.TargetParameters{TimestreamParameters: tt.tsp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})

		t.Run("update_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := updateWithTarget(
				t,
				"ts-single-update-"+tt.name,
				&pipes.TargetParameters{TimestreamParameters: tt.tsp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTargetTimestreamMultiMeasureMappingsRequiredFields verifies CreatePipe
// and UpdatePipe reject a Timestream target whose MultiMeasureMappings
// entries are missing MultiMeasureName or MultiMeasureAttributeMappings, and
// that each attribute mapping entry requires MeasureValue, MeasureValueType,
// and MultiMeasureAttributeName, matching aws-sdk-go-v2 pipes validators.go's
// validateMultiMeasureMapping and validateMultiMeasureAttributeMapping
// (nested under validatePipeTargetTimestreamParameters).
func TestTargetTimestreamMultiMeasureMappingsRequiredFields(t *testing.T) {
	t.Parallel()

	completeAttr := []pipes.TimestreamMultiMeasureAttributeMapping{
		{MeasureValue: "mv", MeasureValueType: "DOUBLE", MultiMeasureAttributeName: "an"},
	}

	base := func(mm []pipes.TimestreamMultiMeasureMapping) *pipes.TimestreamParameters {
		return &pipes.TimestreamParameters{
			TimeValue:    "t",
			VersionValue: "v",
			DimensionMappings: []pipes.TimestreamDimensionMapping{
				{DimensionName: "dn", DimensionValue: "dv", DimensionValueType: "VARCHAR"},
			},
			MultiMeasureMappings: mm,
		}
	}

	tests := []struct {
		tsp       *pipes.TimestreamParameters
		name      string
		wantError bool
	}{
		{
			name: "missing_multi_measure_name",
			tsp: base([]pipes.TimestreamMultiMeasureMapping{
				{MultiMeasureAttributeMappings: completeAttr},
			}),
			wantError: true,
		},
		{
			name: "missing_attribute_mappings",
			tsp: base([]pipes.TimestreamMultiMeasureMapping{
				{MultiMeasureName: "mn"},
			}),
			wantError: true,
		},
		{
			name: "attribute_missing_measure_value",
			tsp: base([]pipes.TimestreamMultiMeasureMapping{
				{
					MultiMeasureName: "mn",
					MultiMeasureAttributeMappings: []pipes.TimestreamMultiMeasureAttributeMapping{
						{MeasureValueType: "DOUBLE", MultiMeasureAttributeName: "an"},
					},
				},
			}),
			wantError: true,
		},
		{
			name: "attribute_missing_measure_value_type",
			tsp: base([]pipes.TimestreamMultiMeasureMapping{
				{
					MultiMeasureName: "mn",
					MultiMeasureAttributeMappings: []pipes.TimestreamMultiMeasureAttributeMapping{
						{MeasureValue: "mv", MultiMeasureAttributeName: "an"},
					},
				},
			}),
			wantError: true,
		},
		{
			name: "attribute_missing_name",
			tsp: base([]pipes.TimestreamMultiMeasureMapping{
				{
					MultiMeasureName: "mn",
					MultiMeasureAttributeMappings: []pipes.TimestreamMultiMeasureAttributeMapping{
						{MeasureValue: "mv", MeasureValueType: "DOUBLE"},
					},
				},
			}),
			wantError: true,
		},
		{
			name: "complete",
			tsp: base([]pipes.TimestreamMultiMeasureMapping{
				{MultiMeasureName: "mn", MultiMeasureAttributeMappings: completeAttr},
			}),
			wantError: false,
		},
		{
			name:      "omitted_accepted",
			tsp:       base(nil),
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run("create_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := createWithTarget(
				t,
				"ts-multi-create-"+tt.name,
				&pipes.TargetParameters{TimestreamParameters: tt.tsp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})

		t.Run("update_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := updateWithTarget(
				t,
				"ts-multi-update-"+tt.name,
				&pipes.TargetParameters{TimestreamParameters: tt.tsp},
			)
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTargetECSNetworkConfigurationRequiredFields verifies CreatePipe and
// UpdatePipe reject an ECS target whose NetworkConfiguration.
// AwsvpcConfiguration is set with no Subnets, matching aws-sdk-go-v2 pipes
// validators.go's validateNetworkConfiguration/validateAwsVpcConfiguration
// (nested under validatePipeTargetEcsTaskParameters).
func TestTargetECSNetworkConfigurationRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		nc        *pipes.NetworkConfiguration
		name      string
		wantError bool
	}{
		{
			name:      "awsvpc_missing_subnets",
			nc:        &pipes.NetworkConfiguration{AwsvpcConfiguration: &pipes.AwsVpcConfiguration{}},
			wantError: true,
		},
		{
			name: "awsvpc_with_subnets",
			nc: &pipes.NetworkConfiguration{
				AwsvpcConfiguration: &pipes.AwsVpcConfiguration{Subnets: []string{"subnet-1"}},
			},
			wantError: false,
		},
		{
			name:      "omitted_accepted",
			nc:        nil,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run("create_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := createWithTarget(t, "ecs-nc-create-"+tt.name, &pipes.TargetParameters{
				EcsTaskParameters: &pipes.ECSTaskTargetParameters{
					TaskDefinitionArn:    "arn:aws:ecs:us-east-1:123456789012:task-definition/td",
					NetworkConfiguration: tt.nc,
				},
			})
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})

		t.Run("update_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := updateWithTarget(t, "ecs-nc-update-"+tt.name, &pipes.TargetParameters{
				EcsTaskParameters: &pipes.ECSTaskTargetParameters{
					TaskDefinitionArn:    "arn:aws:ecs:us-east-1:123456789012:task-definition/td",
					NetworkConfiguration: tt.nc,
				},
			})
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTargetEcsTaskOverrideRequiredFields verifies CreatePipe and UpdatePipe
// reject an ECS target whose Overrides.ContainerOverrides entries are missing
// EnvironmentFiles/ResourceRequirements Type or Value, or whose
// EphemeralStorage is set with no SizeInGiB, matching aws-sdk-go-v2 pipes
// validators.go's validateEcsTaskOverride (nested under
// validatePipeTargetEcsTaskParameters).
func TestTargetEcsTaskOverrideRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ov        *pipes.EcsTaskOverride
		name      string
		wantError bool
	}{
		{
			name: "environment_file_missing_type",
			ov: &pipes.EcsTaskOverride{
				ContainerOverrides: []pipes.EcsContainerOverride{
					{EnvironmentFiles: []pipes.EcsEnvironmentFile{{Value: "arn:aws:s3:::b/env"}}},
				},
			},
			wantError: true,
		},
		{
			name: "environment_file_missing_value",
			ov: &pipes.EcsTaskOverride{
				ContainerOverrides: []pipes.EcsContainerOverride{
					{EnvironmentFiles: []pipes.EcsEnvironmentFile{{Type: "s3"}}},
				},
			},
			wantError: true,
		},
		{
			name: "resource_requirement_missing_type",
			ov: &pipes.EcsTaskOverride{
				ContainerOverrides: []pipes.EcsContainerOverride{
					{ResourceRequirements: []pipes.EcsResourceRequirement{{Value: "1"}}},
				},
			},
			wantError: true,
		},
		{
			name: "resource_requirement_missing_value",
			ov: &pipes.EcsTaskOverride{
				ContainerOverrides: []pipes.EcsContainerOverride{
					{ResourceRequirements: []pipes.EcsResourceRequirement{{Type: "GPU"}}},
				},
			},
			wantError: true,
		},
		{
			name:      "ephemeral_storage_missing_size",
			ov:        &pipes.EcsTaskOverride{EphemeralStorage: &pipes.EcsEphemeralStorage{}},
			wantError: true,
		},
		{
			name: "complete",
			ov: &pipes.EcsTaskOverride{
				EphemeralStorage: &pipes.EcsEphemeralStorage{SizeInGiB: 21},
				ContainerOverrides: []pipes.EcsContainerOverride{
					{
						EnvironmentFiles:     []pipes.EcsEnvironmentFile{{Type: "s3", Value: "arn:aws:s3:::b/env"}},
						ResourceRequirements: []pipes.EcsResourceRequirement{{Type: "GPU", Value: "1"}},
					},
				},
			},
			wantError: false,
		},
		{
			name:      "omitted_accepted",
			ov:        nil,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run("create_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := createWithTarget(t, "ecs-override-create-"+tt.name, &pipes.TargetParameters{
				EcsTaskParameters: &pipes.ECSTaskTargetParameters{
					TaskDefinitionArn: "arn:aws:ecs:us-east-1:123456789012:task-definition/td",
					Overrides:         tt.ov,
				},
			})
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})

		t.Run("update_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := updateWithTarget(t, "ecs-override-update-"+tt.name, &pipes.TargetParameters{
				EcsTaskParameters: &pipes.ECSTaskTargetParameters{
					TaskDefinitionArn: "arn:aws:ecs:us-east-1:123456789012:task-definition/td",
					Overrides:         tt.ov,
				},
			})
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTargetBatchContainerOverridesRequiredFields verifies CreatePipe and
// UpdatePipe reject a Batch target whose ContainerOverrides.
// ResourceRequirements entries are missing Type or Value, matching
// aws-sdk-go-v2 pipes validators.go's validateBatchContainerOverrides
// (nested under validatePipeTargetBatchJobParameters).
func TestTargetBatchContainerOverridesRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		co        *pipes.BatchContainerOverrides
		name      string
		wantError bool
	}{
		{
			name: "resource_requirement_missing_type",
			co: &pipes.BatchContainerOverrides{
				ResourceRequirements: []pipes.BatchResourceRequirement{{Value: "1"}},
			},
			wantError: true,
		},
		{
			name: "resource_requirement_missing_value",
			co: &pipes.BatchContainerOverrides{
				ResourceRequirements: []pipes.BatchResourceRequirement{{Type: "VCPU"}},
			},
			wantError: true,
		},
		{
			name: "complete",
			co: &pipes.BatchContainerOverrides{
				ResourceRequirements: []pipes.BatchResourceRequirement{{Type: "VCPU", Value: "1"}},
			},
			wantError: false,
		},
		{
			name:      "omitted_accepted",
			co:        nil,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run("create_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := createWithTarget(t, "batch-override-create-"+tt.name, &pipes.TargetParameters{
				BatchJobParameters: &pipes.BatchJobTargetParameters{
					JobDefinition:      "jd",
					JobName:            "jn",
					ContainerOverrides: tt.co,
				},
			})
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})

		t.Run("update_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := updateWithTarget(t, "batch-override-update-"+tt.name, &pipes.TargetParameters{
				BatchJobParameters: &pipes.BatchJobTargetParameters{
					JobDefinition:      "jd",
					JobName:            "jn",
					ContainerOverrides: tt.co,
				},
			})
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestTargetECSCapacityProviderStrategyRequiredFields verifies CreatePipe
// and UpdatePipe reject an ECS target whose CapacityProviderStrategy entries
// are missing CapacityProvider, matching aws-sdk-go-v2 pipes validators.go's
// validateCapacityProviderStrategyItem (nested under
// validatePipeTargetEcsTaskParameters).
func TestTargetECSCapacityProviderStrategyRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cps       []pipes.CapacityProviderStrategyItem
		wantError bool
	}{
		{
			name:      "missing_capacity_provider",
			cps:       []pipes.CapacityProviderStrategyItem{{Weight: 1}},
			wantError: true,
		},
		{
			name:      "complete",
			cps:       []pipes.CapacityProviderStrategyItem{{CapacityProvider: "FARGATE", Weight: 1}},
			wantError: false,
		},
		{
			name:      "omitted_accepted",
			cps:       nil,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run("create_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := createWithTarget(t, "ecs-cps-create-"+tt.name, &pipes.TargetParameters{
				EcsTaskParameters: &pipes.ECSTaskTargetParameters{
					TaskDefinitionArn:        "arn:aws:ecs:us-east-1:123456789012:task-definition/td",
					CapacityProviderStrategy: tt.cps,
				},
			})
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})

		t.Run("update_"+tt.name, func(t *testing.T) {
			t.Parallel()
			err := updateWithTarget(t, "ecs-cps-update-"+tt.name, &pipes.TargetParameters{
				EcsTaskParameters: &pipes.ECSTaskTargetParameters{
					TaskDefinitionArn:        "arn:aws:ecs:us-east-1:123456789012:task-definition/td",
					CapacityProviderStrategy: tt.cps,
				},
			})
			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
