package scheduler

import (
	"regexp"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	defaultGroupName         = "default"
	scheduleGroupStateActive = "ACTIVE"
	scheduleStateEnabled     = "ENABLED"
	scheduleStateDisabled    = "DISABLED"

	// flexibleTimeWindowModeOff means no flexible time window is used.
	flexibleTimeWindowModeOff = "OFF"
	// flexibleTimeWindowModeFlexible means a flexible time window is applied.
	flexibleTimeWindowModeFlexible = "FLEXIBLE"

	// actionAfterCompletionNone means no action is taken after the schedule completes.
	actionAfterCompletionNone = "NONE"
	// actionAfterCompletionDelete means the schedule is deleted after it completes.
	actionAfterCompletionDelete = "DELETE"

	// cronFieldCount is the number of space-separated fields a valid EventBridge
	// Scheduler cron() expression must contain:
	// minutes hours day-of-month month day-of-week year.
	cronFieldCount = 6

	// Name validation limits.
	scheduleNameMaxLen = 64
	// RetryPolicy field limits per AWS spec.
	retryPolicyMinEventAge = 60
	retryPolicyMaxEventAge = 86400
	retryPolicyMaxAttempts = 185
)

// validNameRE matches valid schedule/group names: 1-64 chars, [0-9a-zA-Z-_.].
var validNameRE = regexp.MustCompile(`^[0-9a-zA-Z\-_.]+$`)

type FlexibleTimeWindow struct {
	Mode                   string `json:"mode"`
	MaximumWindowInMinutes int    `json:"maximumWindowInMinutes,omitempty"`
}

// RetryPolicy configures retry behaviour for a schedule target.
// MaximumEventAgeInSeconds: 60-86400 (default 86400).
// MaximumRetryAttempts: 0-185 (default 185).
type RetryPolicy struct {
	MaximumEventAgeInSeconds int `json:"maximumEventAgeInSeconds"`
	MaximumRetryAttempts     int `json:"maximumRetryAttempts"`
}

// DeadLetterConfig holds the ARN of an SQS queue used as a dead-letter queue.
type DeadLetterConfig struct {
	Arn string `json:"arn"`
}

// EventBridgeParameters holds parameters for EventBridge bus targets.
type EventBridgeParameters struct {
	DetailType string `json:"detailType"`
	Source     string `json:"source"`
}

// KinesisParameters holds parameters for Kinesis stream targets.
type KinesisParameters struct {
	PartitionKey string `json:"partitionKey"`
}

// SqsParameters holds parameters for SQS targets.
type SqsParameters struct {
	MessageGroupID string `json:"messageGroupId,omitempty"`
}

// SageMakerPipelineParameter is a name/value pair for SageMaker pipeline execution.
type SageMakerPipelineParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SageMakerPipelineParameters holds parameters for SageMaker pipeline targets.
type SageMakerPipelineParameters struct {
	PipelineParameterList []SageMakerPipelineParameter `json:"pipelineParameterList,omitempty"`
}

// EcsAwsvpcConfiguration holds VPC networking options for ECS tasks.
type EcsAwsvpcConfiguration struct {
	AssignPublicIP string   `json:"assignPublicIp,omitempty"`
	SecurityGroups []string `json:"securityGroups,omitempty"`
	Subnets        []string `json:"subnets"`
}

// EcsNetworkConfiguration holds the awsvpc network configuration for ECS tasks.
type EcsNetworkConfiguration struct {
	AwsvpcConfiguration *EcsAwsvpcConfiguration `json:"awsvpcConfiguration,omitempty"`
}

// EcsCapacityProviderStrategyItem is one entry in an ECS capacity provider strategy.
type EcsCapacityProviderStrategyItem struct {
	CapacityProvider string `json:"capacityProvider"`
	Base             int    `json:"base,omitempty"`
	Weight           int    `json:"weight,omitempty"`
}

// EcsPlacementConstraint constrains ECS task placement.
type EcsPlacementConstraint struct {
	Expression string `json:"expression,omitempty"`
	Type       string `json:"type,omitempty"`
}

// EcsPlacementStrategy defines ECS task placement strategy.
type EcsPlacementStrategy struct {
	Field string `json:"field,omitempty"`
	Type  string `json:"type,omitempty"`
}

// EcsParameters holds parameters for ECS task targets.
//
// Tags is a list of free-form single-entry maps (e.g. [{"env":"prod"}]), not a
// list of {Key,Value} objects -- see aws-sdk-go-v2/service/scheduler/types.
// EcsParameters.Tags and its TagMap (de)serializer.
type EcsParameters struct {
	NetworkConfiguration     *EcsNetworkConfiguration          `json:"networkConfiguration,omitempty"`
	PropagateTags            string                            `json:"propagateTags,omitempty"`
	TaskDefinitionArn        string                            `json:"taskDefinitionArn"`
	LaunchType               string                            `json:"launchType,omitempty"`
	PlatformVersion          string                            `json:"platformVersion,omitempty"`
	Group                    string                            `json:"group,omitempty"`
	ReferenceID              string                            `json:"referenceId,omitempty"`
	PlacementConstraints     []EcsPlacementConstraint          `json:"placementConstraints,omitempty"`
	PlacementStrategy        []EcsPlacementStrategy            `json:"placementStrategy,omitempty"`
	Tags                     []map[string]string               `json:"tags,omitempty"`
	CapacityProviderStrategy []EcsCapacityProviderStrategyItem `json:"capacityProviderStrategy,omitempty"`
	TaskCount                int                               `json:"taskCount,omitempty"`
	EnableECSManagedTags     bool                              `json:"enableECSManagedTags,omitempty"`
	EnableExecuteCommand     bool                              `json:"enableExecuteCommand,omitempty"`
}

type Target struct {
	RetryPolicy                 *RetryPolicy                 `json:"retryPolicy,omitempty"`
	DeadLetterConfig            *DeadLetterConfig            `json:"deadLetterConfig,omitempty"`
	EventBridgeParameters       *EventBridgeParameters       `json:"eventBridgeParameters,omitempty"`
	KinesisParameters           *KinesisParameters           `json:"kinesisParameters,omitempty"`
	SqsParameters               *SqsParameters               `json:"sqsParameters,omitempty"`
	SageMakerPipelineParameters *SageMakerPipelineParameters `json:"sageMakerPipelineParameters,omitempty"`
	EcsParameters               *EcsParameters               `json:"ecsParameters,omitempty"`
	// Input is an optional custom event payload sent to the target instead of the default
	// scheduler event. When empty the runner constructs a default EventBridge Scheduler event.
	Input   string `json:"input,omitempty"`
	ARN     string `json:"arn"`
	RoleARN string `json:"roleARN"`
}

// Schedule represents an EventBridge Scheduler schedule.
type Schedule struct {
	Tags                       *tags.Tags         `json:"tags,omitempty"`
	CreationDate               time.Time          `json:"creationDate"`
	LastModificationDate       time.Time          `json:"lastModificationDate"`
	StartDate                  *time.Time         `json:"startDate,omitempty"`
	EndDate                    *time.Time         `json:"endDate,omitempty"`
	Target                     Target             `json:"target"`
	Name                       string             `json:"name"`
	ARN                        string             `json:"arn"`
	ScheduleExpression         string             `json:"scheduleExpression"`
	ScheduleExpressionTimezone string             `json:"scheduleExpressionTimezone,omitempty"`
	Description                string             `json:"description,omitempty"`
	GroupName                  string             `json:"groupName"`
	State                      string             `json:"state"`
	ActionAfterCompletion      string             `json:"actionAfterCompletion,omitempty"`
	KmsKeyArn                  string             `json:"kmsKeyArn,omitempty"`
	AccountID                  string             `json:"accountID"`
	Region                     string             `json:"region"`
	FlexibleTimeWindow         FlexibleTimeWindow `json:"flexibleTimeWindow"`
}

// ScheduleGroup represents an EventBridge Scheduler schedule group.
type ScheduleGroup struct {
	CreationDate         time.Time  `json:"creationDate"`
	LastModificationDate time.Time  `json:"lastModificationDate"`
	Tags                 *tags.Tags `json:"tags,omitempty"`
	Name                 string     `json:"name"`
	ARN                  string     `json:"arn"`
	State                string     `json:"state"`
}
