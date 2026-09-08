package codedeploy

import (
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// TagFilter represents a key/value tag filter for on-premises or EC2 instances.
type TagFilter struct {
	Key   string `json:"Key,omitempty"`
	Value string `json:"Value,omitempty"`
	Type  string `json:"Type,omitempty"` // KEY_ONLY | VALUE_ONLY | KEY_AND_VALUE
}

// TagSet is a list of tag filters combined with AND logic.
type TagSet struct {
	OnPremisesTagSetList [][]TagFilter `json:"onPremisesTagSetList,omitempty"`
}

// Ec2TagSet is a list of EC2 tag filter groups combined with AND logic.
type Ec2TagSet struct {
	Ec2TagSetList [][]TagFilter `json:"ec2TagSetList,omitempty"`
}

// AutoScalingGroup references an Auto Scaling group for a deployment group.
type AutoScalingGroup struct {
	Name string `json:"name,omitempty"`
	Hook string `json:"hook,omitempty"`
}

// ElbInfo references an ELB for load-balancer-based routing.
type ElbInfo struct {
	Name string `json:"name,omitempty"`
}

// TargetGroupInfo references a target group for ALB/NLB routing.
type TargetGroupInfo struct {
	Name string `json:"name,omitempty"`
}

// TargetGroupPairInfo is an ALB/NLB target group pair for blue/green deployments.
type TargetGroupPairInfo struct {
	ProdTrafficRoute *TrafficRoute     `json:"prodTrafficRoute,omitempty"`
	TestTrafficRoute *TrafficRoute     `json:"testTrafficRoute,omitempty"`
	TargetGroups     []TargetGroupInfo `json:"targetGroups,omitempty"`
}

// TrafficRoute specifies a listener ARN for traffic routing.
type TrafficRoute struct {
	ListenerArns []string `json:"listenerArns,omitempty"`
}

// LoadBalancerInfo holds load balancer configuration for a deployment group.
type LoadBalancerInfo struct {
	ElbInfoList             []ElbInfo             `json:"elbInfoList,omitempty"`
	TargetGroupInfoList     []TargetGroupInfo     `json:"targetGroupInfoList,omitempty"`
	TargetGroupPairInfoList []TargetGroupPairInfo `json:"targetGroupPairInfoList,omitempty"`
}

// DeploymentStyle describes the type and traffic control option for a deployment.
type DeploymentStyle struct {
	DeploymentType   string `json:"deploymentType,omitempty"`   // IN_PLACE | BLUE_GREEN
	DeploymentOption string `json:"deploymentOption,omitempty"` // WITH_TRAFFIC_CONTROL | WITHOUT_TRAFFIC_CONTROL
}

// TerminateBlueInstancesOnDeploymentSuccess holds blue-instance termination config.
type TerminateBlueInstancesOnDeploymentSuccess struct {
	Action                       string `json:"action,omitempty"` // TERMINATE | KEEP_ALIVE
	TerminationWaitTimeInMinutes int    `json:"terminationWaitTimeInMinutes,omitempty"`
}

// DeploymentReadyOption configures behavior when blue/green instances are ready.
type DeploymentReadyOption struct {
	ActionOnTimeout   string `json:"actionOnTimeout,omitempty"` // CONTINUE_DEPLOYMENT | STOP_DEPLOYMENT
	WaitTimeInMinutes int    `json:"waitTimeInMinutes,omitempty"`
}

// GreenFleetProvisioningOption configures how replacement instances are provisioned.
type GreenFleetProvisioningOption struct {
	Action string `json:"action,omitempty"` // DISCOVER_EXISTING | COPY_AUTO_SCALING_GROUP
}

// BlueGreenDeploymentConfiguration holds blue/green deployment configuration.
type BlueGreenDeploymentConfiguration struct {
	TerminateBlueInstancesOnDeploymentSuccess *TerminateBlueInstancesOnDeploymentSuccess `json:"terminateBlueInstancesOnDeploymentSuccess,omitempty"` //nolint:lll // long AWS name
	DeploymentReadyOption                     *DeploymentReadyOption                     `json:"deploymentReadyOption,omitempty"`                     //nolint:lll // aligned with AWS field above
	GreenFleetProvisioningOption              *GreenFleetProvisioningOption              `json:"greenFleetProvisioningOption,omitempty"`              //nolint:lll // aligned with AWS field above
}

// Alarm references a CloudWatch alarm.
type Alarm struct {
	Name string `json:"name,omitempty"`
}

// AlarmConfiguration holds alarm-based stop configuration.
type AlarmConfiguration struct {
	Alarms                 []Alarm `json:"alarms,omitempty"`
	Enabled                bool    `json:"enabled,omitempty"`
	IgnorePollAlarmFailure bool    `json:"ignorePollAlarmFailure,omitempty"`
}

// AutoRollbackConfiguration holds auto-rollback event configuration.
type AutoRollbackConfiguration struct {
	Events  []string `json:"events,omitempty"` // rollback event types
	Enabled bool     `json:"enabled,omitempty"`
}

// TriggerConfiguration holds SNS trigger configuration for deployment events.
type TriggerConfiguration struct {
	TriggerName      string   `json:"triggerName,omitempty"`
	TriggerTargetArn string   `json:"triggerTargetArn,omitempty"`
	TriggerEvents    []string `json:"triggerEvents,omitempty"`
}

// ECSService references an ECS service for ECS-platform deployments.
type ECSService struct {
	ServiceName string `json:"serviceName,omitempty"`
	ClusterName string `json:"clusterName,omitempty"`
}

// Application represents an AWS CodeDeploy application.
type Application struct {
	CreationTime    time.Time         `json:"createTime"`
	Tags            *tags.Tags        `json:"-"`
	TagsMap         map[string]string `json:"tagsMap,omitempty"`
	ApplicationName string            `json:"applicationName"`
	ApplicationID   string            `json:"applicationId"`
	ComputePlatform string            `json:"computePlatform"`
	AccountID       string            `json:"-"`
	Region          string            `json:"-"`
}

// DeploymentGroup represents a CodeDeploy deployment group.
type DeploymentGroup struct {
	Tags                             *tags.Tags                        `json:"-"`
	TagsMap                          map[string]string                 `json:"tagsMap,omitempty"`
	BlueGreenDeploymentConfiguration *BlueGreenDeploymentConfiguration `json:"blueGreenDeploymentConfiguration,omitempty"`
	AlarmConfiguration               *AlarmConfiguration               `json:"alarmConfiguration,omitempty"`
	AutoRollbackConfiguration        *AutoRollbackConfiguration        `json:"autoRollbackConfiguration,omitempty"`
	LoadBalancerInfo                 *LoadBalancerInfo                 `json:"loadBalancerInfo,omitempty"`
	DeploymentStyle                  *DeploymentStyle                  `json:"deploymentStyle,omitempty"`
	Ec2TagSet                        *Ec2TagSet                        `json:"ec2TagSet,omitempty"`
	OnPremisesTagSet                 *TagSet                           `json:"onPremisesTagSet,omitempty"`
	ApplicationName                  string                            `json:"applicationName"`
	DeploymentGroupName              string                            `json:"deploymentGroupName"`
	DeploymentGroupID                string                            `json:"deploymentGroupId"`
	ServiceRoleArn                   string                            `json:"serviceRoleArn"`
	DeploymentConfigName             string                            `json:"deploymentConfigName"`
	ComputePlatform                  string                            `json:"computePlatform"`
	OutdatedInstancesStrategy        string                            `json:"outdatedInstancesStrategy,omitempty"`
	AccountID                        string                            `json:"-"`
	Region                           string                            `json:"-"`
	Ec2TagFilters                    []TagFilter                       `json:"ec2TagFilters,omitempty"`
	OnPremisesInstanceTagFilters     []TagFilter                       `json:"onPremisesInstanceTagFilters,omitempty"`
	AutoScalingGroups                []AutoScalingGroup                `json:"autoScalingGroups,omitempty"`
	TriggerConfigurations            []TriggerConfiguration            `json:"triggerConfigurations,omitempty"`
	ECSServices                      []ECSService                      `json:"ecsServices,omitempty"`
	TerminationHookEnabled           bool                              `json:"terminationHookEnabled,omitempty"`
}

// RevisionS3Location holds the S3 location fields for a deployment revision.
type RevisionS3Location struct {
	Bucket     string `json:"bucket,omitempty"`
	Key        string `json:"key,omitempty"`
	BundleType string `json:"bundleType,omitempty"`
	ETag       string `json:"eTag,omitempty"`
	Version    string `json:"version,omitempty"`
}

// RevisionGitHubLocation holds the GitHub location fields for a deployment revision.
type RevisionGitHubLocation struct {
	Repository string `json:"repository,omitempty"`
	CommitID   string `json:"commitId,omitempty"`
}

// RevisionAppSpecContent holds the AppSpec content for a string/AppSpecContent revision.
type RevisionAppSpecContent struct {
	Content string `json:"content,omitempty"`
	Sha256  string `json:"sha256,omitempty"`
}

// RevisionLocation represents a deployment revision source location.
type RevisionLocation struct {
	S3Location     *RevisionS3Location     `json:"s3Location,omitempty"`
	GitHubLocation *RevisionGitHubLocation `json:"gitHubLocation,omitempty"`
	AppSpecContent *RevisionAppSpecContent `json:"appSpecContent,omitempty"`
	RevisionType   string                  `json:"revisionType,omitempty"`
}

// Deployment represents a CodeDeploy deployment.
type Deployment struct {
	CreateTime           time.Time         `json:"createTime"`
	CompleteTime         *time.Time        `json:"completeTime,omitempty"`
	Revision             *RevisionLocation `json:"revision,omitempty"`
	Status               string            `json:"status"`
	ApplicationName      string            `json:"applicationName"`
	DeploymentGroupName  string            `json:"deploymentGroupName"`
	DeploymentConfigName string            `json:"deploymentConfigName"`
	DeploymentID         string            `json:"deploymentId"`
	Creator              string            `json:"creator"`
	Description          string            `json:"description,omitempty"`
	FileExistsBehavior   string            `json:"fileExistsBehavior,omitempty"`
	// ExternalID is never populated: CreateDeploymentInput has no field for
	// it (api_op_CreateDeployment.go), so it stays empty so ListDeployments'
	// externalId filter matches zero deployments instead of being ignored.
	ExternalID                    string `json:"externalId,omitempty"`
	AccountID                     string `json:"-"`
	Region                        string `json:"-"`
	UpdateOutdatedInstancesOnly   bool   `json:"updateOutdatedInstancesOnly,omitempty"`
	IgnoreApplicationStopFailures bool   `json:"ignoreApplicationStopFailures,omitempty"`
}

// OnPremisesInstance represents an on-premises instance registered with CodeDeploy.
type OnPremisesInstance struct {
	RegisterTime   time.Time         `json:"registerTime"`
	DeregisterTime *time.Time        `json:"deregisterTime,omitempty"`
	Tags           *tags.Tags        `json:"-"`
	TagsMap        map[string]string `json:"tagsMap,omitempty"`
	InstanceName   string            `json:"instanceName"`
	IamSessionArn  string            `json:"iamSessionArn,omitempty"`
	IamUserArn     string            `json:"iamUserArn,omitempty"`
}

// MinimumHealthyHosts specifies the minimum number/percentage of healthy instances.
type MinimumHealthyHosts struct {
	Type  string `json:"type,omitempty"` // HOST_COUNT | FLEET_PERCENT
	Value int    `json:"value,omitempty"`
}

// TimeBasedCanary holds canary traffic shifting configuration.
type TimeBasedCanary struct {
	CanaryPercentage int `json:"canaryPercentage,omitempty"`
	CanaryInterval   int `json:"canaryInterval,omitempty"`
}

// TimeBasedLinear holds linear traffic shifting configuration.
type TimeBasedLinear struct {
	LinearPercentage int `json:"linearPercentage,omitempty"`
	LinearInterval   int `json:"linearInterval,omitempty"`
}

// TrafficRoutingConfig holds traffic routing configuration for a deployment config.
type TrafficRoutingConfig struct {
	TimeBasedCanary *TimeBasedCanary `json:"timeBasedCanary,omitempty"`
	TimeBasedLinear *TimeBasedLinear `json:"timeBasedLinear,omitempty"`
	Type            string           `json:"type,omitempty"`
}

// ZonalConfig holds availability-zone-based deployment configuration.
type ZonalConfig struct {
	MinimumHealthyHostsPerZone        *MinimumHealthyHosts `json:"minimumHealthyHostsPerZone,omitempty"`
	FirstZoneMonitorDurationInSeconds int                  `json:"firstZoneMonitorDurationInSeconds,omitempty"`
	MonitorDurationInSeconds          int                  `json:"monitorDurationInSeconds,omitempty"`
}

// DeploymentConfig represents a CodeDeploy deployment configuration.
type DeploymentConfig struct {
	CreateTime           time.Time             `json:"createTime"`
	MinimumHealthyHosts  *MinimumHealthyHosts  `json:"minimumHealthyHosts,omitempty"`
	TrafficRoutingConfig *TrafficRoutingConfig `json:"trafficRoutingConfig,omitempty"`
	ZonalConfig          *ZonalConfig          `json:"zonalConfig,omitempty"`
	DeploymentConfigName string                `json:"deploymentConfigName"`
	DeploymentConfigID   string                `json:"deploymentConfigId"`
	ComputePlatform      string                `json:"computePlatform"`
	IsDefault            bool                  `json:"isDefault,omitempty"`
}

// DeploymentGroupInput holds all the optional rich fields for CreateDeploymentGroup and UpdateDeploymentGroup.
type DeploymentGroupInput struct {
	BlueGreenDeploymentConfiguration *BlueGreenDeploymentConfiguration
	AlarmConfiguration               *AlarmConfiguration
	AutoRollbackConfiguration        *AutoRollbackConfiguration
	LoadBalancerInfo                 *LoadBalancerInfo
	DeploymentStyle                  *DeploymentStyle
	Ec2TagSet                        *Ec2TagSet
	OnPremisesTagSet                 *TagSet
	ServiceRoleArn                   string
	DeploymentConfigName             string
	OutdatedInstancesStrategy        string
	OnPremisesInstanceTagFilters     []TagFilter
	AutoScalingGroups                []AutoScalingGroup
	TriggerConfigurations            []TriggerConfiguration
	ECSServices                      []ECSService
	Ec2TagFilters                    []TagFilter
	TerminationHookEnabled           bool
}

// DeploymentOptions holds optional per-deployment settings.
type DeploymentOptions struct {
	Revision                      *RevisionLocation
	FileExistsBehavior            string
	Description                   string
	Creator                       string
	UpdateOutdatedInstancesOnly   bool
	IgnoreApplicationStopFailures bool
}

// DeploymentFilter holds optional filters for ListDeployments.
type DeploymentFilter struct {
	CreateTimeStart     *time.Time
	CreateTimeEnd       *time.Time
	ApplicationName     string
	DeploymentGroupName string
	ExternalID          string
	Statuses            []string
}

// ApplicationRevision represents a registered CodeDeploy application revision,
// keyed by (ApplicationName, Revision) identity. FirstUsedTime/LastUsedTime are
// nil until the revision is actually referenced by a CreateDeployment call;
// DeploymentGroups lists the deployment groups this revision is the current
// target revision for.
type ApplicationRevision struct {
	RegisterTime     time.Time
	FirstUsedTime    *time.Time
	LastUsedTime     *time.Time
	ApplicationName  string
	Description      string
	Revision         RevisionLocation
	DeploymentGroups []string
}

// RevisionListFilter holds the optional filter/sort fields for ListApplicationRevisions.
type RevisionListFilter struct {
	Deployed    string // include | exclude | ignore
	S3Bucket    string
	S3KeyPrefix string
	SortBy      string // registerTime | firstUsedTime | lastUsedTime
	SortOrder   string // ascending | descending
}

// InstanceListFilter holds ListDeploymentInstances' optional instanceStatusFilter
// (TargetStatus values) and instanceTypeFilter (Blue/Green, case-insensitive
// against this backend's BLUE/GREEN InstanceLabel values).
type InstanceListFilter struct {
	StatusFilter []string
	TypeFilter   []string
}

// TargetListFilter holds ListDeploymentTargets' optional targetFilters map,
// keyed by "TargetStatus" or "ServerInstanceLabel" (the two SDK-defined
// TargetFilterName values).
type TargetListFilter struct {
	TargetStatus        []string
	ServerInstanceLabel []string
}

// DeploymentTargetRecord describes one participant (instance/ECS service/Lambda
// function) in a deployment. It is derived on read from the deployment's real
// backend state (the owning deployment group's compute platform, on-premises
// instance tag matching, or configured ECS services) rather than fabricated
// per-request, and its Status is always mapped from the deployment's own
// current Status (see targetStatusForDeployment) instead of being hardcoded.
type DeploymentTargetRecord struct {
	LastUpdatedAt time.Time
	DeploymentID  string
	TargetID      string
	TargetType    string // instanceTarget | lambdaTarget | ecsTarget
	Status        string // TargetStatus enum value
	TargetArn     string
	InstanceLabel string // BLUE | GREEN (instanceTarget only)
	ClusterName   string // ecsTarget only
	ServiceName   string // ecsTarget only
}
