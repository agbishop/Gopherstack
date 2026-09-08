package pipes

import (
	"fmt"
	"maps"
)

// AwsVpcConfiguration is the VPC network configuration for ECS tasks.
type AwsVpcConfiguration struct {
	AssignPublicIP string   `json:"AssignPublicIp,omitempty"`
	Subnets        []string `json:"Subnets,omitempty"`
	SecurityGroups []string `json:"SecurityGroups,omitempty"`
}

// NetworkConfiguration wraps VPC configuration for ECS task targets.
type NetworkConfiguration struct {
	AwsvpcConfiguration *AwsVpcConfiguration `json:"AwsvpcConfiguration,omitempty"`
}

// CapacityProviderStrategyItem is a single entry in an ECS capacity provider strategy.
type CapacityProviderStrategyItem struct {
	CapacityProvider string `json:"CapacityProvider,omitempty"`
	Weight           int    `json:"Weight,omitempty"`
	Base             int    `json:"Base,omitempty"`
}

// PlacementConstraint is a constraint for ECS task placement.
type PlacementConstraint struct {
	Expression string `json:"Expression,omitempty"`
	Type       string `json:"Type,omitempty"`
}

// PlacementStrategy is a placement strategy rule for ECS tasks.
type PlacementStrategy struct {
	Field string `json:"Field,omitempty"`
	Type  string `json:"Type,omitempty"`
}

// EcsEnvironmentVariable is a name/value pair overriding an ECS container's environment.
// serializers.go/deserializers.go lowercase these keys ("name"/"value"), unlike every other
// field in this ECS-override family -- pipes passes ECS's own RunTask override casing through.
type EcsEnvironmentVariable struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// EcsEnvironmentFile references an S3 object containing environment variables for an ECS container.
type EcsEnvironmentFile struct {
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

// EcsResourceRequirement is a resource type/value pair for an ECS container override.
type EcsResourceRequirement struct {
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

// EcsContainerOverride holds per-container override values for an ECS task execution.
type EcsContainerOverride struct {
	Name                 string                   `json:"Name,omitempty"`
	Command              []string                 `json:"Command,omitempty"`
	Environment          []EcsEnvironmentVariable `json:"Environment,omitempty"`
	EnvironmentFiles     []EcsEnvironmentFile     `json:"EnvironmentFiles,omitempty"`
	ResourceRequirements []EcsResourceRequirement `json:"ResourceRequirements,omitempty"`
	CPU                  int                      `json:"Cpu,omitempty"`
	Memory               int                      `json:"Memory,omitempty"`
	MemoryReservation    int                      `json:"MemoryReservation,omitempty"`
}

// EcsEphemeralStorage overrides the ephemeral storage size for an ECS task.
// SizeInGiB serializes as lowercase "sizeInGiB" (deserializers.go), the same
// ECS-casing quirk as EcsEnvironmentVariable/EcsEnvironmentFile/EcsResourceRequirement.
type EcsEphemeralStorage struct {
	SizeInGiB int `json:"sizeInGiB,omitempty"`
}

// EcsInferenceAcceleratorOverride overrides an Elastic Inference accelerator for an ECS task.
type EcsInferenceAcceleratorOverride struct {
	DeviceName string `json:"deviceName,omitempty"`
	DeviceType string `json:"deviceType,omitempty"`
}

// EcsTaskOverride holds override values for an ECS task execution.
type EcsTaskOverride struct {
	EphemeralStorage              *EcsEphemeralStorage              `json:"EphemeralStorage,omitempty"`
	TaskRoleArn                   string                            `json:"TaskRoleArn,omitempty"`
	ExecutionRoleArn              string                            `json:"ExecutionRoleArn,omitempty"`
	CPU                           string                            `json:"Cpu,omitempty"`
	Memory                        string                            `json:"Memory,omitempty"`
	ContainerOverrides            []EcsContainerOverride            `json:"ContainerOverrides,omitempty"`
	InferenceAcceleratorOverrides []EcsInferenceAcceleratorOverride `json:"InferenceAcceleratorOverrides,omitempty"`
}

// BatchJobDependency represents a dependency between Batch jobs.
type BatchJobDependency struct {
	JobID string `json:"JobId,omitempty"`
	Type  string `json:"Type,omitempty"`
}

// BatchEnvironmentVariable is a single name/value pair in
// BatchContainerOverrides.Environment. The real Batch shape (types.go) is a
// list of these, not a bare map -- both serializers.go (request) and
// deserializers.go (response) reuse the same BatchContainerOverrides type,
// so a map here breaks CreatePipe's request decode for any real client
// setting environment variables, and would equally break DescribePipe's
// response decode once fixed on the input side alone.
type BatchEnvironmentVariable struct {
	Name  string `json:"Name,omitempty"`
	Value string `json:"Value,omitempty"`
}

// BatchResourceRequirement is a resource type/value pair for a Batch container override.
type BatchResourceRequirement struct {
	Type  string `json:"Type,omitempty"`
	Value string `json:"Value,omitempty"`
}

// BatchContainerOverrides holds container override values for a Batch job.
type BatchContainerOverrides struct {
	InstanceType         string                     `json:"InstanceType,omitempty"`
	Environment          []BatchEnvironmentVariable `json:"Environment,omitempty"`
	Command              []string                   `json:"Command,omitempty"`
	ResourceRequirements []BatchResourceRequirement `json:"ResourceRequirements,omitempty"`
}

// LambdaFunctionParameters holds Lambda-specific target configuration.
type LambdaFunctionParameters struct {
	InvocationType string `json:"InvocationType,omitempty"`
}

// StepFunctionTargetParameters holds Step Functions target configuration.
type StepFunctionTargetParameters struct {
	InvocationType string `json:"InvocationType,omitempty"`
}

// SQSTargetParameters holds SQS-specific target configuration.
type SQSTargetParameters struct {
	MessageGroupID         string `json:"MessageGroupId,omitempty"`
	MessageDeduplicationID string `json:"MessageDeduplicationId,omitempty"`
}

// KinesisStreamTargetParameters holds Kinesis stream target configuration.
type KinesisStreamTargetParameters struct {
	PartitionKey string `json:"PartitionKey,omitempty"`
}

// CloudWatchLogsTargetParameters holds CloudWatch Logs target configuration.
type CloudWatchLogsTargetParameters struct {
	LogStreamName string `json:"LogStreamName,omitempty"`
	Timestamp     string `json:"Timestamp,omitempty"`
}

// EBEventBusTargetParameters holds EventBridge event bus target configuration.
type EBEventBusTargetParameters struct {
	DetailType string   `json:"DetailType,omitempty"`
	EndpointID string   `json:"EndpointId,omitempty"`
	Source     string   `json:"Source,omitempty"`
	Time       string   `json:"Time,omitempty"`
	Resources  []string `json:"Resources,omitempty"`
}

// RedshiftDataTargetParameters holds Redshift Data API target configuration.
type RedshiftDataTargetParameters struct {
	Database         string   `json:"Database,omitempty"`
	DBUser           string   `json:"DbUser,omitempty"`
	SecretManagerArn string   `json:"SecretManagerArn,omitempty"`
	StatementName    string   `json:"StatementName,omitempty"`
	Sqls             []string `json:"Sqls,omitempty"`
	WithEvent        bool     `json:"WithEvent,omitempty"`
}

// SageMakerPipelineParameter is a name/value pair for a SageMaker pipeline.
type SageMakerPipelineParameter struct {
	Name  string `json:"Name,omitempty"`
	Value string `json:"Value,omitempty"`
}

// SageMakerPipelineTargetParameters holds SageMaker pipeline target configuration.
type SageMakerPipelineTargetParameters struct {
	PipelineParameterList []SageMakerPipelineParameter `json:"PipelineParameterList,omitempty"`
}

// BatchArrayProperties holds Batch array job properties.
type BatchArrayProperties struct {
	Size int `json:"Size,omitempty"`
}

// BatchRetryStrategy holds Batch retry configuration.
type BatchRetryStrategy struct {
	Attempts int `json:"Attempts,omitempty"`
}

// BatchJobTargetParameters holds Batch job target configuration.
type BatchJobTargetParameters struct {
	ArrayProperties    *BatchArrayProperties    `json:"ArrayProperties,omitempty"`
	RetryStrategy      *BatchRetryStrategy      `json:"RetryStrategy,omitempty"`
	ContainerOverrides *BatchContainerOverrides `json:"ContainerOverrides,omitempty"`
	Parameters         map[string]string        `json:"Parameters,omitempty"`
	JobDefinition      string                   `json:"JobDefinition,omitempty"`
	JobName            string                   `json:"JobName,omitempty"`
	DependsOn          []BatchJobDependency     `json:"DependsOn,omitempty"`
}

// Tag is a key/value tag applied to an ECS RunTask call.
type Tag struct {
	Key   string `json:"Key,omitempty"`
	Value string `json:"Value,omitempty"`
}

// ECSTaskTargetParameters holds ECS task target configuration.
type ECSTaskTargetParameters struct {
	NetworkConfiguration     *NetworkConfiguration          `json:"NetworkConfiguration,omitempty"`
	Overrides                *EcsTaskOverride               `json:"Overrides,omitempty"`
	TaskDefinitionArn        string                         `json:"TaskDefinitionArn,omitempty"`
	LaunchType               string                         `json:"LaunchType,omitempty"`
	Group                    string                         `json:"Group,omitempty"`
	PlatformVersion          string                         `json:"PlatformVersion,omitempty"`
	PropagateTags            string                         `json:"PropagateTags,omitempty"`
	ReferenceID              string                         `json:"ReferenceId,omitempty"`
	CapacityProviderStrategy []CapacityProviderStrategyItem `json:"CapacityProviderStrategy,omitempty"`
	PlacementConstraints     []PlacementConstraint          `json:"PlacementConstraints,omitempty"`
	PlacementStrategy        []PlacementStrategy            `json:"PlacementStrategy,omitempty"`
	Tags                     []Tag                          `json:"Tags,omitempty"`
	TaskCount                int                            `json:"TaskCount,omitempty"`
	EnableECSManagedTags     bool                           `json:"EnableECSManagedTags,omitempty"`
	EnableExecuteCommand     bool                           `json:"EnableExecuteCommand,omitempty"`
}

// TargetHTTPParameters holds HTTP-specific parameters for API Gateway and API destination targets.
type TargetHTTPParameters struct {
	HeaderParameters      map[string]string `json:"HeaderParameters,omitempty"`
	QueryStringParameters map[string]string `json:"QueryStringParameters,omitempty"`
	PathParameterValues   []string          `json:"PathParameterValues,omitempty"`
}

// TimestreamDimensionMapping maps an event field to a Timestream dimension.
type TimestreamDimensionMapping struct {
	DimensionName      string `json:"DimensionName,omitempty"`
	DimensionValue     string `json:"DimensionValue,omitempty"`
	DimensionValueType string `json:"DimensionValueType,omitempty"`
}

// TimestreamSingleMeasureMapping maps an event field to a single Timestream measure.
type TimestreamSingleMeasureMapping struct {
	MeasureName      string `json:"MeasureName,omitempty"`
	MeasureValue     string `json:"MeasureValue,omitempty"`
	MeasureValueType string `json:"MeasureValueType,omitempty"`
}

// TimestreamMultiMeasureAttributeMapping maps an event field to a multi-measure attribute.
type TimestreamMultiMeasureAttributeMapping struct {
	MeasureValue              string `json:"MeasureValue,omitempty"`
	MeasureValueType          string `json:"MeasureValueType,omitempty"`
	MultiMeasureAttributeName string `json:"MultiMeasureAttributeName,omitempty"`
}

// TimestreamMultiMeasureMapping maps event fields to a Timestream multi-measure record.
type TimestreamMultiMeasureMapping struct {
	MultiMeasureName              string                                   `json:"MultiMeasureName,omitempty"`
	MultiMeasureAttributeMappings []TimestreamMultiMeasureAttributeMapping `json:"MultiMeasureAttributeMappings,omitempty"`
}

// TimestreamParameters holds Timestream target configuration.
type TimestreamParameters struct {
	TimeValue             string                           `json:"TimeValue,omitempty"`
	TimeFieldType         string                           `json:"TimeFieldType,omitempty"`
	TimestampFormat       string                           `json:"TimestampFormat,omitempty"`
	EpochTimeUnit         string                           `json:"EpochTimeUnit,omitempty"`
	VersionValue          string                           `json:"VersionValue,omitempty"`
	DimensionMappings     []TimestreamDimensionMapping     `json:"DimensionMappings,omitempty"`
	SingleMeasureMappings []TimestreamSingleMeasureMapping `json:"SingleMeasureMappings,omitempty"`
	MultiMeasureMappings  []TimestreamMultiMeasureMapping  `json:"MultiMeasureMappings,omitempty"`
}

// TargetParameters holds target-specific configuration.
type TargetParameters struct {
	LambdaFunctionParameters      *LambdaFunctionParameters          `json:"LambdaFunctionParameters,omitempty"`
	SFNStateMachineParameters     *StepFunctionTargetParameters      `json:"StepFunctionStateMachineParameters,omitempty"`
	SqsQueueParameters            *SQSTargetParameters               `json:"SqsQueueParameters,omitempty"`
	KinesisStreamParameters       *KinesisStreamTargetParameters     `json:"KinesisStreamParameters,omitempty"`
	CloudWatchLogsParameters      *CloudWatchLogsTargetParameters    `json:"CloudWatchLogsParameters,omitempty"`
	EventBridgeEventBusParameters *EBEventBusTargetParameters        `json:"EventBridgeEventBusParameters,omitempty"`
	RedshiftDataParameters        *RedshiftDataTargetParameters      `json:"RedshiftDataParameters,omitempty"`
	SageMakerPipelineParameters   *SageMakerPipelineTargetParameters `json:"SageMakerPipelineParameters,omitempty"`
	BatchJobParameters            *BatchJobTargetParameters          `json:"BatchJobParameters,omitempty"`
	EcsTaskParameters             *ECSTaskTargetParameters           `json:"EcsTaskParameters,omitempty"`
	TimestreamParameters          *TimestreamParameters              `json:"TimestreamParameters,omitempty"`
	HTTPParameters                *TargetHTTPParameters              `json:"HttpParameters,omitempty"`
	InputTemplate                 string                             `json:"InputTemplate,omitempty"`
}

func cloneAwsVpcConfiguration(src *AwsVpcConfiguration) *AwsVpcConfiguration {
	if src == nil {
		return nil
	}
	vpc := *src
	vpc.Subnets = append([]string(nil), src.Subnets...)
	vpc.SecurityGroups = append([]string(nil), src.SecurityGroups...)

	return &vpc
}

func cloneBatchJobParameters(src *BatchJobTargetParameters) *BatchJobTargetParameters {
	v := *src
	if v.ArrayProperties != nil {
		ap := *v.ArrayProperties
		v.ArrayProperties = &ap
	}
	if v.RetryStrategy != nil {
		rs := *v.RetryStrategy
		v.RetryStrategy = &rs
	}
	if v.ContainerOverrides != nil {
		co := *v.ContainerOverrides
		co.Command = append([]string(nil), v.ContainerOverrides.Command...)
		co.Environment = append(
			[]BatchEnvironmentVariable(nil),
			v.ContainerOverrides.Environment...)
		co.ResourceRequirements = append(
			[]BatchResourceRequirement(nil),
			v.ContainerOverrides.ResourceRequirements...)
		v.ContainerOverrides = &co
	}
	v.DependsOn = append([]BatchJobDependency(nil), src.DependsOn...)
	v.Parameters = maps.Clone(src.Parameters)

	return &v
}

func cloneEcsContainerOverrides(src []EcsContainerOverride) []EcsContainerOverride {
	if src == nil {
		return nil
	}
	out := make([]EcsContainerOverride, len(src))
	for i, co := range src {
		co.Command = append([]string(nil), co.Command...)
		co.Environment = append([]EcsEnvironmentVariable(nil), co.Environment...)
		co.EnvironmentFiles = append([]EcsEnvironmentFile(nil), co.EnvironmentFiles...)
		co.ResourceRequirements = append([]EcsResourceRequirement(nil), co.ResourceRequirements...)
		out[i] = co
	}

	return out
}

func cloneEcsTaskOverride(src *EcsTaskOverride) *EcsTaskOverride {
	if src == nil {
		return nil
	}
	ov := *src
	if src.EphemeralStorage != nil {
		es := *src.EphemeralStorage
		ov.EphemeralStorage = &es
	}
	ov.ContainerOverrides = cloneEcsContainerOverrides(src.ContainerOverrides)
	ov.InferenceAcceleratorOverrides = append(
		[]EcsInferenceAcceleratorOverride(nil), src.InferenceAcceleratorOverrides...,
	)

	return &ov
}

func cloneECSTaskParameters(src *ECSTaskTargetParameters) *ECSTaskTargetParameters {
	v := *src
	if v.NetworkConfiguration != nil {
		nc := *v.NetworkConfiguration
		nc.AwsvpcConfiguration = cloneAwsVpcConfiguration(
			v.NetworkConfiguration.AwsvpcConfiguration,
		)
		v.NetworkConfiguration = &nc
	}
	v.Overrides = cloneEcsTaskOverride(src.Overrides)
	v.CapacityProviderStrategy = append(
		[]CapacityProviderStrategyItem(nil), src.CapacityProviderStrategy...,
	)
	v.PlacementConstraints = append([]PlacementConstraint(nil), src.PlacementConstraints...)
	v.PlacementStrategy = append([]PlacementStrategy(nil), src.PlacementStrategy...)
	v.Tags = append([]Tag(nil), src.Tags...)

	return &v
}

func cloneTargetParameters(src *TargetParameters) *TargetParameters {
	tp := *src
	if src.LambdaFunctionParameters != nil {
		v := *src.LambdaFunctionParameters
		tp.LambdaFunctionParameters = &v
	}
	if src.SFNStateMachineParameters != nil {
		v := *src.SFNStateMachineParameters
		tp.SFNStateMachineParameters = &v
	}
	if src.SqsQueueParameters != nil {
		v := *src.SqsQueueParameters
		tp.SqsQueueParameters = &v
	}
	if src.KinesisStreamParameters != nil {
		v := *src.KinesisStreamParameters
		tp.KinesisStreamParameters = &v
	}
	if src.CloudWatchLogsParameters != nil {
		v := *src.CloudWatchLogsParameters
		tp.CloudWatchLogsParameters = &v
	}
	if src.EventBridgeEventBusParameters != nil {
		v := *src.EventBridgeEventBusParameters
		v.Resources = append([]string(nil), src.EventBridgeEventBusParameters.Resources...)
		tp.EventBridgeEventBusParameters = &v
	}
	if src.RedshiftDataParameters != nil {
		v := *src.RedshiftDataParameters
		v.Sqls = append([]string(nil), src.RedshiftDataParameters.Sqls...)
		tp.RedshiftDataParameters = &v
	}
	if src.SageMakerPipelineParameters != nil {
		v := *src.SageMakerPipelineParameters
		v.PipelineParameterList = append(
			[]SageMakerPipelineParameter(nil),
			src.SageMakerPipelineParameters.PipelineParameterList...,
		)
		tp.SageMakerPipelineParameters = &v
	}
	if src.BatchJobParameters != nil {
		tp.BatchJobParameters = cloneBatchJobParameters(src.BatchJobParameters)
	}
	if src.EcsTaskParameters != nil {
		tp.EcsTaskParameters = cloneECSTaskParameters(src.EcsTaskParameters)
	}
	if src.TimestreamParameters != nil {
		v := *src.TimestreamParameters
		v.DimensionMappings = append(
			[]TimestreamDimensionMapping(nil),
			src.TimestreamParameters.DimensionMappings...)
		v.SingleMeasureMappings = append(
			[]TimestreamSingleMeasureMapping(nil),
			src.TimestreamParameters.SingleMeasureMappings...,
		)
		v.MultiMeasureMappings = append(
			[]TimestreamMultiMeasureMapping(nil),
			src.TimestreamParameters.MultiMeasureMappings...,
		)
		tp.TimestreamParameters = &v
	}
	if src.HTTPParameters != nil {
		v := *src.HTTPParameters
		v.HeaderParameters = maps.Clone(src.HTTPParameters.HeaderParameters)
		v.QueryStringParameters = maps.Clone(src.HTTPParameters.QueryStringParameters)
		v.PathParameterValues = append([]string(nil), src.HTTPParameters.PathParameterValues...)
		tp.HTTPParameters = &v
	}

	return &tp
}

// validateTargetRequiredFields enforces required nested target fields, matching
// aws-sdk-go-v2 pipes validators.go's validatePipeTargetKinesisStreamParameters
// (PartitionKey required), validatePipeTargetEcsTaskParameters (TaskDefinitionArn
// required, plus nested validateNetworkConfiguration/validateAwsVpcConfiguration:
// Subnets required when AwsvpcConfiguration is set, and nested
// validateCapacityProviderStrategyItem: CapacityProvider required per entry,
// and nested validateEcsTaskOverride: EnvironmentFiles/ResourceRequirements
// Type/Value required per ContainerOverrides entry, EphemeralStorage.SizeInGiB
// required when set), validatePipeTargetBatchJobParameters (JobDefinition and
// JobName required, plus nested validateBatchContainerOverrides:
// ResourceRequirements Type/Value required per entry),
// validatePipeTargetRedshiftDataParameters (Database and Sqls
// required), validatePipeTargetSageMakerPipelineParameters's nested
// validateSageMakerPipelineParameter (Name and Value required per list entry),
// and validatePipeTargetTimestreamParameters (TimeValue, VersionValue, and
// DimensionMappings required, plus its nested validateDimensionMapping:
// DimensionName, DimensionValue, DimensionValueType required per entry;
// validateSingleMeasureMapping: MeasureName, MeasureValue, MeasureValueType
// required per SingleMeasureMappings entry; validateMultiMeasureMapping:
// MultiMeasureName and MultiMeasureAttributeMappings required per
// MultiMeasureMappings entry, plus nested
// validateMultiMeasureAttributeMapping: MeasureValue, MeasureValueType,
// MultiMeasureAttributeName required per attribute mapping entry).
// Unlike source-side StartingPosition, this applies on both CreatePipe and
// UpdatePipe: both ops route TargetParameters through the same validator.
func validateTargetRequiredFields(tp *TargetParameters) error {
	if tp == nil {
		return nil
	}
	if kp := tp.KinesisStreamParameters; kp != nil && kp.PartitionKey == "" {
		return fmt.Errorf("%w: KinesisStreamParameters.PartitionKey is required", ErrValidation)
	}
	if err := validateECSTargetRequiredFields(tp.EcsTaskParameters); err != nil {
		return err
	}
	if err := validateBatchJobRequiredFields(tp.BatchJobParameters); err != nil {
		return err
	}
	if err := validateRedshiftRequiredFields(tp.RedshiftDataParameters); err != nil {
		return err
	}
	if err := validateSageMakerPipelineRequiredFields(tp.SageMakerPipelineParameters); err != nil {
		return err
	}
	if err := validateTimestreamRequiredFields(tp.TimestreamParameters); err != nil {
		return err
	}

	return nil
}

func validateECSTargetRequiredFields(ep *ECSTaskTargetParameters) error {
	if ep == nil {
		return nil
	}
	if ep.TaskDefinitionArn == "" {
		return fmt.Errorf("%w: EcsTaskParameters.TaskDefinitionArn is required", ErrValidation)
	}
	if nc := ep.NetworkConfiguration; nc != nil && nc.AwsvpcConfiguration != nil &&
		len(nc.AwsvpcConfiguration.Subnets) == 0 {
		return fmt.Errorf(
			"%w: EcsTaskParameters.NetworkConfiguration.AwsvpcConfiguration.Subnets is required",
			ErrValidation,
		)
	}
	for i, cps := range ep.CapacityProviderStrategy {
		if cps.CapacityProvider == "" {
			return fmt.Errorf(
				"%w: EcsTaskParameters.CapacityProviderStrategy[%d].CapacityProvider is required",
				ErrValidation,
				i,
			)
		}
	}

	return validateEcsTaskOverrideRequiredFields(ep.Overrides)
}

// validateEcsTaskOverrideRequiredFields matches aws-sdk-go-v2 pipes
// validators.go's validateEcsTaskOverride: nested validation on ContainerOverrides
// (validateEcsContainerOverrideList/validateEcsContainerOverride, itself nested on
// EnvironmentFiles and ResourceRequirements) and on EphemeralStorage
// (validateEcsEphemeralStorage: SizeInGiB required).
func validateEcsTaskOverrideRequiredFields(ov *EcsTaskOverride) error {
	if ov == nil {
		return nil
	}
	for i, co := range ov.ContainerOverrides {
		for j, ef := range co.EnvironmentFiles {
			if ef.Type == "" {
				return fmt.Errorf(
					"%w: EcsTaskParameters.Overrides.ContainerOverrides[%d].EnvironmentFiles[%d].Type is required",
					ErrValidation,
					i,
					j,
				)
			}
			if ef.Value == "" {
				return fmt.Errorf(
					"%w: EcsTaskParameters.Overrides.ContainerOverrides[%d].EnvironmentFiles[%d].Value is required",
					ErrValidation,
					i,
					j,
				)
			}
		}
		for j, rr := range co.ResourceRequirements {
			if rr.Type == "" {
				return fmt.Errorf(
					"%w: EcsTaskParameters.Overrides.ContainerOverrides[%d].ResourceRequirements[%d].Type is required",
					ErrValidation,
					i,
					j,
				)
			}
			if rr.Value == "" {
				return fmt.Errorf(
					"%w: EcsTaskParameters.Overrides.ContainerOverrides[%d].ResourceRequirements[%d].Value is required",
					ErrValidation,
					i,
					j,
				)
			}
		}
	}
	if ov.EphemeralStorage != nil && ov.EphemeralStorage.SizeInGiB == 0 {
		return fmt.Errorf(
			"%w: EcsTaskParameters.Overrides.EphemeralStorage.SizeInGiB is required",
			ErrValidation,
		)
	}

	return nil
}

func validateBatchJobRequiredFields(bp *BatchJobTargetParameters) error {
	if bp == nil {
		return nil
	}
	if bp.JobDefinition == "" {
		return fmt.Errorf("%w: BatchJobParameters.JobDefinition is required", ErrValidation)
	}
	if bp.JobName == "" {
		return fmt.Errorf("%w: BatchJobParameters.JobName is required", ErrValidation)
	}

	return validateBatchContainerOverridesRequiredFields(bp.ContainerOverrides)
}

// validateBatchContainerOverridesRequiredFields matches aws-sdk-go-v2 pipes
// validators.go's validateBatchContainerOverrides: nested per-entry Type/Value
// on ResourceRequirements (validateBatchResourceRequirementsList/validateBatchResourceRequirement).
func validateBatchContainerOverridesRequiredFields(co *BatchContainerOverrides) error {
	if co == nil {
		return nil
	}
	for i, rr := range co.ResourceRequirements {
		if rr.Type == "" {
			return fmt.Errorf(
				"%w: BatchJobParameters.ContainerOverrides.ResourceRequirements[%d].Type is required",
				ErrValidation,
				i,
			)
		}
		if rr.Value == "" {
			return fmt.Errorf(
				"%w: BatchJobParameters.ContainerOverrides.ResourceRequirements[%d].Value is required",
				ErrValidation,
				i,
			)
		}
	}

	return nil
}

func validateRedshiftRequiredFields(rp *RedshiftDataTargetParameters) error {
	if rp == nil {
		return nil
	}
	if rp.Database == "" {
		return fmt.Errorf("%w: RedshiftDataParameters.Database is required", ErrValidation)
	}
	if len(rp.Sqls) == 0 {
		return fmt.Errorf("%w: RedshiftDataParameters.Sqls is required", ErrValidation)
	}

	return nil
}

func validateSageMakerPipelineRequiredFields(sp *SageMakerPipelineTargetParameters) error {
	if sp == nil {
		return nil
	}
	for i, param := range sp.PipelineParameterList {
		if param.Name == "" {
			return fmt.Errorf(
				"%w: SageMakerPipelineParameters.PipelineParameterList[%d].Name is required",
				ErrValidation,
				i,
			)
		}
		if param.Value == "" {
			return fmt.Errorf(
				"%w: SageMakerPipelineParameters.PipelineParameterList[%d].Value is required",
				ErrValidation,
				i,
			)
		}
	}

	return nil
}

func validateTimestreamRequiredFields(tsp *TimestreamParameters) error {
	if tsp == nil {
		return nil
	}
	if tsp.TimeValue == "" {
		return fmt.Errorf("%w: TimestreamParameters.TimeValue is required", ErrValidation)
	}
	if tsp.VersionValue == "" {
		return fmt.Errorf("%w: TimestreamParameters.VersionValue is required", ErrValidation)
	}
	if len(tsp.DimensionMappings) == 0 {
		return fmt.Errorf("%w: TimestreamParameters.DimensionMappings is required", ErrValidation)
	}
	for i, dm := range tsp.DimensionMappings {
		if dm.DimensionName == "" {
			return fmt.Errorf(
				"%w: TimestreamParameters.DimensionMappings[%d].DimensionName is required",
				ErrValidation,
				i,
			)
		}
		if dm.DimensionValue == "" {
			return fmt.Errorf(
				"%w: TimestreamParameters.DimensionMappings[%d].DimensionValue is required",
				ErrValidation,
				i,
			)
		}
		if dm.DimensionValueType == "" {
			return fmt.Errorf(
				"%w: TimestreamParameters.DimensionMappings[%d].DimensionValueType is required",
				ErrValidation,
				i,
			)
		}
	}
	if err := validateSingleMeasureMappingsRequiredFields(tsp.SingleMeasureMappings); err != nil {
		return err
	}

	return validateMultiMeasureMappingsRequiredFields(tsp.MultiMeasureMappings)
}

func validateSingleMeasureMappingsRequiredFields(sms []TimestreamSingleMeasureMapping) error {
	for i, sm := range sms {
		if sm.MeasureName == "" {
			return fmt.Errorf(
				"%w: TimestreamParameters.SingleMeasureMappings[%d].MeasureName is required",
				ErrValidation,
				i,
			)
		}
		if sm.MeasureValue == "" {
			return fmt.Errorf(
				"%w: TimestreamParameters.SingleMeasureMappings[%d].MeasureValue is required",
				ErrValidation,
				i,
			)
		}
		if sm.MeasureValueType == "" {
			return fmt.Errorf(
				"%w: TimestreamParameters.SingleMeasureMappings[%d].MeasureValueType is required",
				ErrValidation,
				i,
			)
		}
	}

	return nil
}

func validateMultiMeasureMappingsRequiredFields(mms []TimestreamMultiMeasureMapping) error {
	for i, mm := range mms {
		if mm.MultiMeasureName == "" {
			return fmt.Errorf(
				"%w: TimestreamParameters.MultiMeasureMappings[%d].MultiMeasureName is required",
				ErrValidation,
				i,
			)
		}
		if len(mm.MultiMeasureAttributeMappings) == 0 {
			return fmt.Errorf(
				"%w: TimestreamParameters.MultiMeasureMappings[%d].MultiMeasureAttributeMappings is required",
				ErrValidation,
				i,
			)
		}
		if err := validateMultiMeasureAttrMappingsRequiredFields(i, mm.MultiMeasureAttributeMappings); err != nil {
			return err
		}
	}

	return nil
}

func validateMultiMeasureAttrMappingsRequiredFields(
	i int,
	ams []TimestreamMultiMeasureAttributeMapping,
) error {
	for j, am := range ams {
		if am.MeasureValue == "" {
			return fmt.Errorf(
				"%w: TimestreamParameters.MultiMeasureMappings[%d].AttributeMappings[%d].MeasureValue is required",
				ErrValidation,
				i,
				j,
			)
		}
		if am.MeasureValueType == "" {
			return fmt.Errorf(
				"%w: TimestreamParameters.MultiMeasureMappings[%d].AttributeMappings[%d].MeasureValueType is required",
				ErrValidation,
				i,
				j,
			)
		}
		if am.MultiMeasureAttributeName == "" {
			return fmt.Errorf(
				"%w: TimestreamParameters.MultiMeasureMappings[%d].AttributeMappings[%d].MultiMeasureAttributeName is required",
				ErrValidation,
				i,
				j,
			)
		}
	}

	return nil
}
