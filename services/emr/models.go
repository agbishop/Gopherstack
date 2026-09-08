package emr

import (
	"time"
)

// regionContextKey is the context key under which the per-request AWS region is stored.
type regionContextKey struct{}

const (
	StateWaiting              = "WAITING"
	StateTerminated           = "TERMINATED"
	StateTerminatedWithErrors = "TERMINATED_WITH_ERRORS"

	// StateRunning is the real ClusterState value StartSession's doc allows
	// sessions to start against, alongside StateWaiting. This backend's own
	// cluster state machine never produces RUNNING -- clusters are created
	// directly in WAITING (see buildNewCluster) and go straight from WAITING
	// to TERMINATED (see terminateSingle), with no simulated
	// STARTING/BOOTSTRAPPING/RUNNING/TERMINATING in between -- so in
	// practice only StateWaiting ever passes the sessionCanStart check
	// below. StateRunning is included for correctness against the real API
	// and forward-compatibility (e.g. a hand-seeded test cluster).
	StateRunning = "RUNNING"

	// StateStarting and StateBootstrapping are the two remaining real
	// ClusterState values AddJobFlowSteps' doc allows steps to be added
	// against ("STARTING, BOOTSTRAPPING, RUNNING, or WAITING"), alongside
	// StateWaiting/StateRunning above. Unreachable in this backend for the
	// same reason StateRunning is (see its comment) -- included so
	// clusterAcceptsSteps (clusters.go) checks the real allow-list, not a
	// backend-specific approximation of it.
	StateStarting      = "STARTING"
	StateBootstrapping = "BOOTSTRAPPING"

	StepStatePending   = "PENDING"
	StepStateCompleted = "COMPLETED"
	StepStateCancelled = "CANCELLED"

	// SessionStateSubmitted and its siblings below mirror the real
	// SessionState enum (aws-sdk-go-v2/service/emr/types/enums.go) verbatim:
	// SUBMITTED, STARTING, STARTED, IDLE, BUSY, TERMINATING, TERMINATED,
	// FAILED. This backend only ever drives two of them -- see sessions.go's
	// package doc comment for the full state-model rationale.
	SessionStateSubmitted   = "SUBMITTED"
	SessionStateStarting    = "STARTING"
	SessionStateStarted     = "STARTED"
	SessionStateIdle        = "IDLE"
	SessionStateBusy        = "BUSY"
	SessionStateTerminating = "TERMINATING"
	SessionStateTerminated  = "TERMINATED"
	SessionStateFailed      = "FAILED"

	// cancelStepsStatusSubmitted/Failed are the only two values of the real
	// CancelStepsRequestStatus enum (SUBMITTED | FAILED) -- not the ad hoc
	// "SUCCESS"/"QUEUED" strings this backend used to return.
	cancelStepsStatusSubmitted = "SUBMITTED"
	cancelStepsStatusFailed    = "FAILED"

	defaultReleaseLabel    = "emr-7.3.0"
	defaultStepConcurrency = 1

	minIdleTimeout = 60
	maxIdleTimeout = 604800

	minStepConcurrency = 1
	maxStepConcurrency = 256

	timelineKeyCreation = "CreationDateTime"
	timelineKeyReady    = "ReadyDateTime"
	timelineKeyEnd      = "EndDateTime"

	// stepCompletionDelay is how long a step stays PENDING before gopherstack
	// promotes it to COMPLETED on read. AWS steps run asynchronously and may
	// stay PENDING/RUNNING for as long as the underlying Hadoop job takes;
	// gopherstack has no real workload to run, so it simulates near-instant
	// completion instead of leaving steps parked in PENDING forever, which
	// would hang a real client's StepComplete waiter (min poll interval 30s).
	stepCompletionDelay = 3 * time.Second

	listClustersPageSize         = 50
	listSecConfigsPageSize       = 50
	listReleaseLabelsPage        = 50
	listInstanceTypesPage        = 50
	listStepsPageSize            = 50
	listInstancesPageSize        = 500
	listStudiosPageSize          = 50
	listNotebookExecPageSize     = 50
	listBootstrapActionsPageSize = 50
	listSessionsPageSize         = 50
	listStudioMappingsPageSize   = 50

	instanceGroupStateRunning = "RUNNING"

	defaultSSHPort = 22

	sessionCredentialExpiry = 12 * time.Hour

	archX86   = "X86_64"
	archARM64 = "ARM64"

	// EC2 instance size constants used in the supportedInstanceTypes catalog.
	vcpu4  = 4
	vcpu8  = 8
	vcpu16 = 16
	vcpu32 = 32

	gb8   = float64(8)
	gb16  = float64(16)
	gb30  = float64(30)
	gb32  = float64(32)
	gb61  = float64(61)
	gb64  = float64(64)
	gb128 = float64(128)

	ndisk1 = 1
	ndisk2 = 2

	appHadoop = "Hadoop"
	appHive   = "Hive"
	appHue    = "Hue"
	appLivy   = "Livy"
	appMXNet  = "MXNet"
	appOozie  = "Oozie"
	appPig    = "Pig"
	appPresto = "Presto"
	appSpark  = "Spark"
	appTez    = "Tez"
	appFlink  = "Flink"
	appHBase  = "HBase"
	appTF     = "TensorFlow"
	appTrino  = "Trino"
)

// --- Domain types ---

// Tag is an EMR resource tag.
type Tag struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

// Configuration is a recursive EMR classification configuration entry.
type Configuration struct {
	Classification string            `json:"Classification,omitempty"`
	Properties     map[string]string `json:"Properties,omitempty"`
	Configurations []Configuration   `json:"Configurations,omitempty"`
}

// Application represents an EMR application bundled in a cluster.
type Application struct {
	Name    string `json:"Name"`
	Version string `json:"Version,omitempty"`
}

// BootstrapActionScript holds the script path and arguments for a bootstrap action.
type BootstrapActionScript struct {
	Path string   `json:"Path"`
	Args []string `json:"Args,omitempty"`
}

// BootstrapActionConfig is the full bootstrap action specification used in RunJobFlow input.
type BootstrapActionConfig struct {
	Name                  string                `json:"Name"`
	ScriptBootstrapAction BootstrapActionScript `json:"ScriptBootstrapAction"`
}

// Command is the flattened representation of a bootstrap action returned by ListBootstrapActions.
type Command struct {
	Name       string   `json:"Name"`
	ScriptPath string   `json:"ScriptPath"`
	Args       []string `json:"Args,omitempty"`
}

// KeyValue is a Hadoop job property pair, the real REQUEST-side wire shape
// for step Properties (types.KeyValue, serializers.go's
// awsAwsjson11_serializeDocumentKeyValue: {"Key":..., "Value":...}).
type KeyValue struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

// StepHadoopJarStepInput is the REQUEST-side shape (types.HadoopJarStepConfig)
// for a step's Hadoop JAR execution, used by StepSpec (RunJobFlow/
// AddJobFlowSteps' Steps input). Properties is genuinely a JSON ARRAY of
// {Key,Value} objects on this side (serializers.go's
// awsAwsjson11_serializeDocumentKeyValueList) -- asymmetric with the
// RESPONSE side (StepHadoopJarStep.Properties below), which the real API
// represents as a plain string map (types.HadoopStepConfig, StringMap
// shape). A real EMR wire quirk, confirmed independently against both
// serializers.go and deserializers.go, not a gopherstack inconsistency.
type StepHadoopJarStepInput struct {
	Jar        string     `json:"Jar"`
	MainClass  string     `json:"MainClass,omitempty"`
	Args       []string   `json:"Args,omitempty"`
	Properties []KeyValue `json:"Properties,omitempty"`
}

// StepHadoopJarStep is the RESPONSE-side shape (types.HadoopStepConfig) for
// a step's Hadoop JAR execution -- see StepHadoopJarStepInput's doc comment
// for why Properties differs in shape from the request side. Previously had
// no Properties member at all, so a real client's per-step Hadoop job
// properties were silently dropped on input and never echoed back.
type StepHadoopJarStep struct {
	Properties map[string]string `json:"Properties,omitempty"`
	Jar        string            `json:"Jar"`
	MainClass  string            `json:"MainClass,omitempty"`
	Args       []string          `json:"Args,omitempty"`
}

// StepTimeline tracks creation and completion times of a step.
type StepTimeline struct {
	CreationDateTime float64 `json:"CreationDateTime"`
	StartDateTime    float64 `json:"StartDateTime,omitempty"`
	EndDateTime      float64 `json:"EndDateTime,omitempty"`
}

// StepStatus holds the lifecycle state of an EMR step.
type StepStatus struct {
	State    string       `json:"State"`
	Timeline StepTimeline `json:"Timeline"`
}

// CancelStepsInfo represents the result of cancelling a single step.
type CancelStepsInfo struct {
	StepID string `json:"StepId"`
	Status string `json:"Status"`
	Reason string `json:"Reason,omitempty"`
}

// Step represents an EMR step attached to a cluster, shared by both
// DescribeStep (real shape types.Step) and ListSteps' per-item shape (real
// shape types.StepSummary).
//
// HadoopJarStep is wire-keyed "Config", not "HadoopJarStep": that name is
// correct for the request-side StepConfig (types.StepConfig, real key
// "HadoopJarStep", serializers.go's awsAwsjson11_serializeDocumentStepConfig)
// but the RESPONSE types (types.Step/types.StepSummary) nest the same shape
// under "Config" (types.HadoopStepConfig, deserializers.go's "Config" case in
// awsAwsjson11_deserializeDocumentStep/...StepSummary) -- a real client's
// typed Step.Config/StepSummary.Config was always nil regardless of backend
// state before this fix.
//
// ExecutionRoleArn is real and non-required, but ONLY on types.Step
// (deserializers.go's awsAwsjson11_deserializeDocumentStep "ExecutionRoleArn"
// case) -- types.StepSummary genuinely has no such member
// (awsAwsjson11_deserializeDocumentStepSummary's case list has none). Since
// this type is shared by both responses and DescribeStep is where the field
// matters (sourced from the call-level AddJobFlowStepsInput.ExecutionRoleArn/
// RunJobFlowInput.StepExecutionRoleArn, not from the per-step StepConfig),
// ListSteps also emits it when set: a harmless extra field a real typed
// client for that op has no slot to decode into, same non-bug class as
// rds's DBInstance.StorageOptimized.
type Step struct {
	ID               string            `json:"Id"`
	Name             string            `json:"Name"`
	HadoopJarStep    StepHadoopJarStep `json:"Config"`
	ActionOnFailure  string            `json:"ActionOnFailure"`
	ExecutionRoleArn string            `json:"ExecutionRoleArn,omitempty"`
	Status           StepStatus        `json:"Status"`
}

// StepSpec is the input for adding a new step.
type StepSpec struct {
	Name            string                 `json:"Name"`
	ActionOnFailure string                 `json:"ActionOnFailure"`
	HadoopJarStep   StepHadoopJarStepInput `json:"HadoopJarStep"`
}

// ComputeLimits defines compute bounds for managed scaling.
type ComputeLimits struct {
	UnitType                     string `json:"UnitType"`
	MinimumCapacityUnits         int    `json:"MinimumCapacityUnits"`
	MaximumCapacityUnits         int    `json:"MaximumCapacityUnits"`
	MaximumOnDemandCapacityUnits int    `json:"MaximumOnDemandCapacityUnits,omitempty"`
	MaximumCoreCapacityUnits     int    `json:"MaximumCoreCapacityUnits,omitempty"`
}

// ManagedScalingPolicy defines managed scaling for a cluster.
type ManagedScalingPolicy struct {
	ComputeLimits ComputeLimits `json:"ComputeLimits"`
}

// AutoTerminationPolicy defines the auto-termination idle timeout for a cluster.
type AutoTerminationPolicy struct {
	IdleTimeout int64 `json:"IdleTimeout"`
}

// PortRange defines an inclusive range of port numbers.
type PortRange struct {
	MinRange int `json:"MinRange"`
	MaxRange int `json:"MaxRange"`
}

// BlockPublicAccessConfiguration is the account-level block-public-access config.
type BlockPublicAccessConfiguration struct {
	region                                 string
	PermittedPublicSecurityGroupRuleRanges []PortRange `json:"PermittedPublicSecurityGroupRuleRanges,omitempty"`
	BlockPublicSecurityGroupRules          bool        `json:"BlockPublicSecurityGroupRules"`
}

// blockPublicAccessMeta holds metadata for the block-public-access configuration.
// CreationDateTime is epoch seconds (float64); see SecurityConfiguration for
// why (EMR's awsjson1.1 unixTimestamp wire format).
type blockPublicAccessMeta struct {
	CreatedByArn string `json:"CreatedByArn,omitempty"`
	// region is the store.Table primary key (one meta record per region).
	region           string
	CreationDateTime float64 `json:"CreationDateTime"`
}

// AutoScalingConstraints defines capacity bounds for an auto-scaling policy.
type AutoScalingConstraints struct {
	MinCapacity int `json:"MinCapacity"`
	MaxCapacity int `json:"MaxCapacity"`
}

// SimpleScalingPolicyConfiguration defines scaling adjustment details.
type SimpleScalingPolicyConfiguration struct {
	AdjustmentType    string `json:"AdjustmentType"`
	ScalingAdjustment int    `json:"ScalingAdjustment"`
	CoolDown          int    `json:"CoolDown,omitempty"`
}

// ScalingAction defines what to do when a scaling rule fires.
type ScalingAction struct {
	SimpleScalingPolicyConfiguration SimpleScalingPolicyConfiguration `json:"SimpleScalingPolicyConfiguration"`
}

// CloudWatchAlarmDefinition is the CloudWatch alarm that triggers scaling.
type CloudWatchAlarmDefinition struct {
	ComparisonOperator string  `json:"ComparisonOperator"`
	MetricName         string  `json:"MetricName"`
	Namespace          string  `json:"Namespace,omitempty"`
	Statistic          string  `json:"Statistic,omitempty"`
	Unit               string  `json:"Unit,omitempty"`
	EvaluationPeriods  int     `json:"EvaluationPeriods"`
	Period             int     `json:"Period"`
	Threshold          float64 `json:"Threshold"`
}

// ScalingTrigger wraps a CloudWatch alarm definition.
type ScalingTrigger struct {
	CloudWatchAlarmDefinition CloudWatchAlarmDefinition `json:"CloudWatchAlarmDefinition"`
}

// ScalingRule is a named auto-scaling rule combining action and trigger.
type ScalingRule struct {
	Name        string         `json:"Name"`
	Description string         `json:"Description,omitempty"`
	Action      ScalingAction  `json:"Action"`
	Trigger     ScalingTrigger `json:"Trigger"`
}

// AutoScalingPolicySpec is used as input to PutAutoScalingPolicy.
type AutoScalingPolicySpec struct {
	Rules       []ScalingRule          `json:"Rules,omitempty"`
	Constraints AutoScalingConstraints `json:"Constraints"`
}

// AutoScalingPolicyDetail is stored on an instance group after PutAutoScalingPolicy.
type AutoScalingPolicyDetail struct {
	Status      map[string]string      `json:"Status"`
	Rules       []ScalingRule          `json:"Rules,omitempty"`
	Constraints AutoScalingConstraints `json:"Constraints"`
}

// ClusterInstance represents a single EC2 instance in a cluster.
type ClusterInstance struct {
	ID               string                `json:"Id"`
	Ec2InstanceID    string                `json:"Ec2InstanceId"`
	PrivateDNSName   string                `json:"PrivateDnsName"`
	PublicDNSName    string                `json:"PublicDnsName,omitempty"`
	PrivateIPAddress string                `json:"PrivateIpAddress,omitempty"`
	InstanceGroupID  string                `json:"InstanceGroupId,omitempty"`
	InstanceFleetID  string                `json:"InstanceFleetId,omitempty"`
	Market           string                `json:"Market"`
	InstanceType     string                `json:"InstanceType"`
	Status           ClusterInstanceStatus `json:"Status"`
}

// ClusterInstanceStatus holds the state of a ClusterInstance.
type ClusterInstanceStatus struct {
	State string `json:"State"`
}

// SupportedInstanceType describes an EC2 instance type supported by EMR.
type SupportedInstanceType struct {
	Type          string  `json:"Type"`
	Architecture  string  `json:"Architecture"`
	MemoryGB      float64 `json:"MemoryGB"`
	VCPU          int     `json:"VCPU"`
	NumberOfDisks int     `json:"NumberOfDisks,omitempty"`
	Is64BitsOnly  bool    `json:"Is64BitsOnly"`
}

// ReleaseLabelApplication is an application listed for a release label.
type ReleaseLabelApplication struct {
	Name    string `json:"Name"`
	Version string `json:"Version"`
}

// ReleaseLabel holds details about an EMR release label.
type ReleaseLabel struct {
	ReleaseLabel string                    `json:"ReleaseLabel"`
	Applications []ReleaseLabelApplication `json:"Applications,omitempty"`
}

// InstanceFleetStatus tracks the provisioning state of an EMR instance fleet.
type InstanceFleetStatus struct {
	State string `json:"State"`
}

// NotebookExecutionStatus values for a notebook execution.
const (
	NotebookStatusRunning  = "RUNNING"
	NotebookStatusStopping = "STOPPING"
	NotebookStatusStopped  = "STOPPED"
	NotebookStatusFinished = "FINISHED"
)

// NotebookExecution represents an EMR Studio notebook execution.
//
// StartTime/EndTime are epoch seconds (float64), matching the EMR awsjson1.1
// wire format -- the real SDK deserializer parses these with
// smithytime.ParseEpochSeconds and rejects RFC3339 strings. A zero value
// (unset) is omitted via omitempty, matching the "not yet ended" case where
// AWS omits EndTime entirely.
// ExecutionEngineID's json tag is persistence-only (regionalDTO's plain
// json.Marshal round-trip, see persistence.go): NotebookExecution itself is
// never marshaled directly for an HTTP response any more. The real
// DescribeNotebookExecutionOutput.NotebookExecution nests it under an
// "ExecutionEngine" object (types.ExecutionEngineConfig{Id,...},
// emr@v1.64.4 deserializers.go's "ExecutionEngine" case in
// awsAwsjson11_deserializeDocumentNotebookExecution) rather than the flat
// "ExecutionEngineId" this type used to emit directly -- that flat key is
// only correct for the DIFFERENT, trimmed NotebookExecutionSummary type
// ListNotebookExecutions returns (deserializers.go's
// awsAwsjson11_deserializeDocumentNotebookExecutionSummary, which genuinely
// has "ExecutionEngineId" flat). handler_notebook_executions.go's
// newNotebookExecutionDetail builds the correctly-nested Describe wire
// shape from this field instead.
type NotebookExecution struct {
	NotebookExecutionID   string `json:"NotebookExecutionId"`
	EditorID              string `json:"EditorId,omitempty"`
	NotebookExecutionName string `json:"NotebookExecutionName,omitempty"`
	NotebookParams        string `json:"NotebookParams,omitempty"`
	ExecutionEngineID     string `json:"executionEngineId,omitempty"`
	Status                string `json:"Status"`
	region                string
	Tags                  []Tag   `json:"Tags"`
	StartTime             float64 `json:"StartTime,omitempty"`
	EndTime               float64 `json:"EndTime,omitempty"`
}

// NotebookExecutionSummary is the wire shape for ListNotebookExecutions
// items (types.NotebookExecutionSummary, emr@v1.64.4 types.go:2161): no
// NotebookParams, no Tags -- both present on the full NotebookExecution that
// DescribeNotebookExecution returns.
type NotebookExecutionSummary struct {
	NotebookExecutionID   string  `json:"NotebookExecutionId"`
	EditorID              string  `json:"EditorId,omitempty"`
	NotebookExecutionName string  `json:"NotebookExecutionName,omitempty"`
	ExecutionEngineID     string  `json:"ExecutionEngineId,omitempty"`
	Status                string  `json:"Status"`
	StartTime             float64 `json:"StartTime,omitempty"`
	EndTime               float64 `json:"EndTime,omitempty"`
}

// newNotebookExecutionSummary projects a NotebookExecution into
// ListNotebookExecutions' real per-item shape.
func newNotebookExecutionSummary(ne NotebookExecution) NotebookExecutionSummary {
	return NotebookExecutionSummary{
		NotebookExecutionID:   ne.NotebookExecutionID,
		EditorID:              ne.EditorID,
		NotebookExecutionName: ne.NotebookExecutionName,
		ExecutionEngineID:     ne.ExecutionEngineID,
		Status:                ne.Status,
		StartTime:             ne.StartTime,
		EndTime:               ne.EndTime,
	}
}

// InstanceGroupStatus is the status of an EMR instance group.
type InstanceGroupStatus struct {
	State string `json:"State"`
}

// InstanceGroupSpec is the input specification for an instance group from RunJobFlow.
type InstanceGroupSpec struct {
	Name           string          `json:"Name"`
	Market         string          `json:"Market"`
	InstanceRole   string          `json:"InstanceRole"`
	InstanceType   string          `json:"InstanceType"`
	BidPrice       string          `json:"BidPrice,omitempty"`
	Configurations []Configuration `json:"Configurations,omitempty"`
	InstanceCount  int             `json:"InstanceCount"`
}

// InstanceGroup represents an EMR instance group returned by ListInstanceGroups.
type InstanceGroup struct {
	AutoScalingPolicy      *AutoScalingPolicyDetail `json:"AutoScalingPolicy,omitempty"`
	Status                 InstanceGroupStatus      `json:"Status"`
	ID                     string                   `json:"Id"`
	Name                   string                   `json:"Name"`
	Market                 string                   `json:"Market"`
	BidPrice               string                   `json:"BidPrice,omitempty"`
	InstanceGroupType      string                   `json:"InstanceGroupType"`
	InstanceType           string                   `json:"InstanceType"`
	Configurations         []Configuration          `json:"Configurations,omitempty"`
	RequestedInstanceCount int                      `json:"RequestedInstanceCount"`
	RunningInstanceCount   int                      `json:"RunningInstanceCount"`
}

// EC2InstanceAttributes represents EC2 instance attributes for an EMR cluster.
type EC2InstanceAttributes struct {
	Ec2KeyName                     string   `json:"Ec2KeyName,omitempty"`
	Ec2SubnetID                    string   `json:"Ec2SubnetId,omitempty"`
	Ec2AvailabilityZone            string   `json:"Ec2AvailabilityZone,omitempty"`
	EmrManagedMasterSecurityGroup  string   `json:"EmrManagedMasterSecurityGroup,omitempty"`
	EmrManagedSlaveSecurityGroup   string   `json:"EmrManagedSlaveSecurityGroup,omitempty"`
	ServiceAccessSecurityGroup     string   `json:"ServiceAccessSecurityGroup,omitempty"`
	IamInstanceProfile             string   `json:"IamInstanceProfile,omitempty"`
	AdditionalMasterSecurityGroups []string `json:"AdditionalMasterSecurityGroups,omitempty"`
	AdditionalSlaveSecurityGroups  []string `json:"AdditionalSlaveSecurityGroups,omitempty"`
	RequestedEc2SubnetIDs          []string `json:"RequestedEc2SubnetIds,omitempty"`
}

// KerberosAttributes holds Kerberos configuration for a cluster, set via
// RunJobFlow and echoed back on Cluster when Kerberos authentication is
// enabled using a security configuration.
type KerberosAttributes struct {
	Realm                            string `json:"Realm"`
	KdcAdminPassword                 string `json:"KdcAdminPassword"`
	ADDomainJoinUser                 string `json:"ADDomainJoinUser,omitempty"`
	ADDomainJoinPassword             string `json:"ADDomainJoinPassword,omitempty"`
	CrossRealmTrustPrincipalPassword string `json:"CrossRealmTrustPrincipalPassword,omitempty"`
}

// PlacementGroupConfig is the placement group configuration for a single
// instance role, part of RunJobFlow's Instances.Placement and echoed back on
// Cluster.PlacementGroups.
type PlacementGroupConfig struct {
	InstanceRole      string `json:"InstanceRole"`
	PlacementStrategy string `json:"PlacementStrategy,omitempty"`
}

// CloudWatchLogConfiguration controls CloudWatch log publishing, part of
// RunJobFlow's MonitoringConfiguration and echoed back on Cluster.
type CloudWatchLogConfiguration struct {
	LogTypes            map[string][]string `json:"LogTypes,omitempty"`
	LogGroupName        string              `json:"LogGroupName,omitempty"`
	LogStreamNamePrefix string              `json:"LogStreamNamePrefix,omitempty"`
	EncryptionKeyArn    string              `json:"EncryptionKeyArn,omitempty"`
	Enabled             bool                `json:"Enabled"`
}

// S3LoggingConfiguration controls per-log-type S3 upload policy, part of
// RunJobFlow's MonitoringConfiguration and echoed back on Cluster.
type S3LoggingConfiguration struct {
	LogTypeUploadPolicy map[string]string `json:"LogTypeUploadPolicy,omitempty"`
}

// MonitoringConfiguration is RunJobFlowInput's MonitoringConfiguration,
// threaded through unchanged and echoed back on Cluster -- gopherstack does
// not itself publish any logs, so this is stored/returned configuration, not
// simulated behavior.
type MonitoringConfiguration struct {
	CloudWatchLogConfiguration *CloudWatchLogConfiguration `json:"CloudWatchLogConfiguration,omitempty"`
	S3LoggingConfiguration     *S3LoggingConfiguration     `json:"S3LoggingConfiguration,omitempty"`
}

// Cluster represents an EMR cluster.
type Cluster struct {
	// terminatedAt is internal-only (janitor.go's TTL cleanup): real
	// types.Cluster has no such member (emr@v1.64.4 deserializers.go's
	// awsAwsjson11_deserializeDocumentCluster case list), so it must not
	// reach the wire -- unexported like instanceGroups/steps/etc. below, and
	// carried through persistence via clusterDTO.TerminatedAt the same way
	// (see persistence.go).
	terminatedAt            time.Time
	Ec2InstanceAttributes   *EC2InstanceAttributes   `json:"Ec2InstanceAttributes"`
	KerberosAttributes      *KerberosAttributes      `json:"KerberosAttributes,omitempty"`
	MonitoringConfiguration *MonitoringConfiguration `json:"MonitoringConfiguration,omitempty"`
	autoTerminationPolicy   *AutoTerminationPolicy
	managedScalingPolicy    *ManagedScalingPolicy
	// region is the store.Table composite-key qualifier (see regionKey in
	// backend.go); it is unexported so it is never marshaled by a plain
	// json.Marshal(Cluster) and is instead carried through persistence via
	// clusterDTO (see persistence.go).
	region                 string
	Status                 ClusterStatus `json:"Status"`
	ScaleDownBehavior      string        `json:"ScaleDownBehavior,omitempty"`
	ID                     string        `json:"Id"`
	ARN                    string        `json:"ClusterArn"`
	ReleaseLabel           string        `json:"ReleaseLabel"`
	OSReleaseLabel         string        `json:"OSReleaseLabel,omitempty"`
	LogURI                 string        `json:"LogUri,omitempty"`
	LogEncryptionKmsKeyID  string        `json:"LogEncryptionKmsKeyId,omitempty"`
	ServiceRole            string        `json:"ServiceRole,omitempty"`
	AutoScalingRole        string        `json:"AutoScalingRole,omitempty"`
	Name                   string        `json:"Name"`
	SecurityConfiguration  string        `json:"SecurityConfiguration,omitempty"`
	CustomAmiID            string        `json:"CustomAmiId,omitempty"`
	InstanceCollectionType string        `json:"InstanceCollectionType,omitempty"`
	RepoUpgradeOnBoot      string        `json:"RepoUpgradeOnBoot,omitempty"`
	RequestedAmiVersion    string        `json:"RequestedAmiVersion,omitempty"`
	RunningAmiVersion      string        `json:"RunningAmiVersion,omitempty"`
	instanceGroups         []InstanceGroup
	bootstrapActions       []BootstrapActionConfig
	Tags                   []Tag                  `json:"Tags"`
	Applications           []Application          `json:"Applications,omitempty"`
	Configurations         []Configuration        `json:"Configurations,omitempty"`
	PlacementGroups        []PlacementGroupConfig `json:"PlacementGroups,omitempty"`
	steps                  []Step
	instanceFleets         []InstanceFleet
	// sessions holds the interactive (Spark Connect) sessions started on
	// this cluster via StartSession -- like steps/instanceGroups/
	// instanceFleets above, real EMR has no ListSessions-across-clusters
	// operation, only ListSessions(ClusterId), so sessions are modeled as a
	// child collection embedded directly on the owning Cluster rather than a
	// separate store.Table. This also gives cascade-delete for free: when
	// the janitor sweeps a TERMINATED cluster's row (see janitor.go), every
	// session on it is removed in the same operation -- no separate sweep
	// needed to avoid orphaned sessions.
	sessions                    []Session
	StepConcurrencyLevel        int  `json:"StepConcurrencyLevel,omitempty"`
	EbsRootVolumeSize           int  `json:"EbsRootVolumeSize,omitempty"`
	EbsRootVolumeIops           int  `json:"EbsRootVolumeIops,omitempty"`
	EbsRootVolumeThroughput     int  `json:"EbsRootVolumeThroughput,omitempty"`
	UnhealthyNodeReplacement    bool `json:"UnhealthyNodeReplacement"`
	KeepJobFlowAliveWhenNoSteps bool `json:"KeepJobFlowAliveWhenNoSteps"`
	TerminationProtected        bool `json:"TerminationProtected"`
	VisibleToAllUsers           bool `json:"VisibleToAllUsers"`
	// AutoTerminate is the real API's inverse of KeepJobFlowAliveWhenNoSteps:
	// true means the cluster terminates after completing all steps.
	AutoTerminate bool `json:"AutoTerminate"`
	// SessionEnabled indicates whether Spark Connect sessions (StartSession
	// et al.) are enabled on this cluster (emr@v1.64.4 types.go:447-448).
	SessionEnabled bool `json:"SessionEnabled"`
}

// ClusterStatus holds the status fields for a Cluster.
type ClusterStatus struct {
	StateChangeReason map[string]any `json:"StateChangeReason,omitempty"`
	Timeline          map[string]any `json:"Timeline,omitempty"`
	State             string         `json:"State"`
}

// ClusterSummary is a trimmed-down view used for ListClusters.
//
// NormalizedInstanceHours is a real ClusterSummary member
// (aws-sdk-go-v2/service/emr/types.ClusterSummary) this backend never
// populates -- an honest omission (a real client sees it as nil/zero), not
// fabricated. OutpostArn is likewise real and omitted for the same reason.
// ReleaseLabel used to live here but was deleted: the real ClusterSummary
// has no such member at all (only Id, Name, Status, ClusterArn,
// NormalizedInstanceHours, OutpostArn) -- it was an invented field, not an
// omission.
type ClusterSummary struct {
	ID         string        `json:"Id"`
	Name       string        `json:"Name"`
	Status     ClusterStatus `json:"Status"`
	ClusterArn string        `json:"ClusterArn"`
}

// InstanceFleet represents an EMR instance fleet returned by AddInstanceFleet.
type InstanceFleet struct {
	Status                      InstanceFleetStatus `json:"Status"`
	ID                          string              `json:"Id"`
	Name                        string              `json:"Name"`
	InstanceFleetType           string              `json:"InstanceFleetType"`
	TargetOnDemandCapacity      int                 `json:"TargetOnDemandCapacity"`
	TargetSpotCapacity          int                 `json:"TargetSpotCapacity"`
	ProvisionedOnDemandCapacity int                 `json:"ProvisionedOnDemandCapacity"`
	ProvisionedSpotCapacity     int                 `json:"ProvisionedSpotCapacity"`
}

// InstanceFleetSpec is the input specification for an instance fleet.
type InstanceFleetSpec struct {
	Name                   string          `json:"Name"`
	InstanceFleetType      string          `json:"InstanceFleetType"`
	Configurations         []Configuration `json:"Configurations,omitempty"`
	TargetOnDemandCapacity int             `json:"TargetOnDemandCapacity"`
	TargetSpotCapacity     int             `json:"TargetSpotCapacity"`
}

// SecurityConfiguration stores an EMR security configuration.
//
// CreationDateTime is epoch seconds (float64), matching the EMR awsjson1.1
// wire format -- the real SDK deserializer parses CreationDateTime fields
// with smithytime.ParseEpochSeconds and rejects RFC3339 strings.
type SecurityConfiguration struct {
	Name           string `json:"Name"`
	SecurityConfig string `json:"SecurityConfiguration"`
	// region is the store.Table composite-key qualifier (see regionKey).
	region           string
	CreationDateTime float64 `json:"CreationDateTime"`
}

// Studio represents an EMR Studio.
//
// CreationTime is epoch seconds (float64), matching the EMR awsjson1.1 wire
// format -- the real SDK deserializer parses CreationTime with
// smithytime.ParseEpochSeconds and rejects RFC3339 strings.
type Studio struct {
	EngineSecurityGroupID             string `json:"EngineSecurityGroupId"`
	VpcID                             string `json:"VpcId"`
	StudioID                          string `json:"StudioId"`
	EncryptionKeyArn                  string `json:"EncryptionKeyArn,omitempty"`
	Name                              string `json:"Name"`
	Description                       string `json:"Description,omitempty"`
	AuthMode                          string `json:"AuthMode"`
	DefaultS3Location                 string `json:"DefaultS3Location"`
	ServiceRole                       string `json:"ServiceRole"`
	IdcInstanceArn                    string `json:"IdcInstanceArn,omitempty"`
	IdcUserAssignment                 string `json:"IdcUserAssignment,omitempty"`
	URL                               string `json:"Url"`
	WorkspaceSecurityGroupID          string `json:"WorkspaceSecurityGroupId"`
	StudioArn                         string `json:"StudioArn"`
	UserRole                          string `json:"UserRole,omitempty"`
	IdpAuthURL                        string `json:"IdpAuthUrl,omitempty"`
	IdpRelayStateParameterName        string `json:"IdpRelayStateParameterName,omitempty"`
	region                            string
	Tags                              []Tag    `json:"Tags"`
	SubnetIDs                         []string `json:"SubnetIds"`
	CreationTime                      float64  `json:"CreationTime,omitempty"`
	TrustedIdentityPropagationEnabled bool     `json:"TrustedIdentityPropagationEnabled"`
}

// StudioSummary is a trimmed view of Studio for ListStudios.
// CreationTime is epoch seconds (float64); see Studio for why.
//
// StudioArn/DefaultS3Location were previously listed here but deleted: the
// real types.StudioSummary (emr@v1.64.4 deserializers.go's
// awsAwsjson11_deserializeDocumentStudioSummary case list) has no such
// members at all (only AuthMode/CreationTime/Description/Name/StudioId/Url/
// VpcId) -- both were invented fields, not omissions. Harmless (a real
// client's typed StudioSummary has no field to decode either into), but
// incorrect.
type StudioSummary struct {
	StudioID     string  `json:"StudioId"`
	Name         string  `json:"Name"`
	VpcID        string  `json:"VpcId"`
	AuthMode     string  `json:"AuthMode"`
	URL          string  `json:"Url"`
	Description  string  `json:"Description,omitempty"`
	CreationTime float64 `json:"CreationTime,omitempty"`
}

// StudioSessionMapping maps a user or group to an EMR Studio.
// CreationTime/LastModifiedTime are epoch seconds (float64); see Studio for why.
type StudioSessionMapping struct {
	StudioID         string `json:"StudioId"`
	IdentityType     string `json:"IdentityType"`
	IdentityID       string `json:"IdentityId,omitempty"`
	IdentityName     string `json:"IdentityName,omitempty"`
	SessionPolicyArn string `json:"SessionPolicyArn"`
	// region is the store.Table composite-key qualifier (see regionKey).
	region           string
	LastModifiedTime float64 `json:"LastModifiedTime,omitempty"`
	CreationTime     float64 `json:"CreationTime,omitempty"`
}

// PersistentAppUI represents an EMR persistent application user interface.
// PersistentAppUI is this backend's internal model, deliberately not
// marshaled directly: it mixes CreatePersistentAppUIOutput's real shape
// (PersistentAppUIId/RuntimeRoleEnabledCluster, correct there) with
// TargetResourceArn, which is a CreatePersistentAppUIInput-only concept --
// the real DescribePersistentAppUIOutput.PersistentAppUI (types.PersistentAppUI,
// emr@v1.64.4 deserializers.go's awsAwsjson11_deserializeDocumentPersistentAppUI
// case list) has an entirely different field set (AuthorId/CreationTime/
// LastModifiedTime/LastStateChangeReason/PersistentAppUIId/
// PersistentAppUIStatus/PersistentAppUITypeList/Tags) with neither
// TargetResourceArn nor RuntimeRoleEnabledCluster at all. See
// handler_persistent_app_ui.go's newPersistentAppUIDetail for the correctly
// separated Describe wire shape; handleCreatePersistentAppUI already built
// its own separate, correct DTO and never used this type's JSON tags.
type PersistentAppUI struct {
	CreatedAt                 time.Time
	ID                        string `json:"PersistentAppUIId"`
	TargetResourceArn         string `json:"TargetResourceArn"`
	region                    string
	RuntimeRoleEnabledCluster bool `json:"RuntimeRoleEnabledCluster"`
}

// RunJobFlowInstances holds the Instances block from a RunJobFlow call.
//
// NOTE: real EMR's JobFlowInstancesConfig has no IamInstanceProfile member --
// that attribute is set via the top-level RunJobFlowInput.JobFlowRole field
// instead (see RunJobFlowParams.JobFlowRole) and echoed back on
// Cluster.Ec2InstanceAttributes.IamInstanceProfile. An IamInstanceProfile
// field used to live here, but no real client ever populates it at this
// nesting level, so it was deleted.
type RunJobFlowInstances struct {
	Ec2KeyName                     string              `json:"Ec2KeyName,omitempty"`
	Ec2SubnetID                    string              `json:"Ec2SubnetId,omitempty"`
	EmrManagedMasterSecurityGroup  string              `json:"EmrManagedMasterSecurityGroup,omitempty"`
	EmrManagedSlaveSecurityGroup   string              `json:"EmrManagedSlaveSecurityGroup,omitempty"`
	ServiceAccessSecurityGroup     string              `json:"ServiceAccessSecurityGroup,omitempty"`
	InstanceGroups                 []InstanceGroupSpec `json:"InstanceGroups,omitempty"`
	InstanceFleets                 []InstanceFleetSpec `json:"InstanceFleets,omitempty"`
	Ec2SubnetIDs                   []string            `json:"Ec2SubnetIds,omitempty"`
	AdditionalMasterSecurityGroups []string            `json:"AdditionalMasterSecurityGroups,omitempty"`
	AdditionalSlaveSecurityGroups  []string            `json:"AdditionalSlaveSecurityGroups,omitempty"`
	KeepJobFlowAliveWhenNoSteps    bool                `json:"KeepJobFlowAliveWhenNoSteps"`
	TerminationProtected           bool                `json:"TerminationProtected"`
}

// RunJobFlowParams is the full input for creating a new cluster.
type RunJobFlowParams struct {
	KerberosAttributes      *KerberosAttributes      `json:"KerberosAttributes,omitempty"`
	MonitoringConfiguration *MonitoringConfiguration `json:"MonitoringConfiguration,omitempty"`
	AutoTerminationPolicy   *AutoTerminationPolicy   `json:"AutoTerminationPolicy,omitempty"`
	ManagedScalingPolicy    *ManagedScalingPolicy    `json:"ManagedScalingPolicy,omitempty"`
	LogEncryptionKmsKeyID   string                   `json:"LogEncryptionKmsKeyId,omitempty"`
	AmiVersion              string                   `json:"AmiVersion,omitempty"`
	AutoScalingRole         string                   `json:"AutoScalingRole,omitempty"`
	Name                    string                   `json:"Name"`
	ScaleDownBehavior       string                   `json:"ScaleDownBehavior,omitempty"`
	CustomAmiID             string                   `json:"CustomAmiId,omitempty"`
	JobFlowRole             string                   `json:"JobFlowRole,omitempty"`
	RepoUpgradeOnBoot       string                   `json:"RepoUpgradeOnBoot,omitempty"`
	SecurityConfiguration   string                   `json:"SecurityConfiguration,omitempty"`
	ReleaseLabel            string                   `json:"ReleaseLabel"`
	OSReleaseLabel          string                   `json:"OSReleaseLabel,omitempty"`
	ServiceRole             string                   `json:"ServiceRole,omitempty"`
	LogURI                  string                   `json:"LogUri,omitempty"`
	PlacementGroupConfigs   []PlacementGroupConfig   `json:"PlacementGroupConfigs,omitempty"`
	BootstrapActions        []BootstrapActionConfig  `json:"BootstrapActions,omitempty"`
	Steps                   []StepSpec               `json:"Steps,omitempty"`
	StepExecutionRoleArn    string                   `json:"StepExecutionRoleArn,omitempty"`
	Configurations          []Configuration          `json:"Configurations,omitempty"`
	Applications            []Application            `json:"Applications,omitempty"`
	Tags                    []Tag                    `json:"Tags,omitempty"`
	Instances               RunJobFlowInstances      `json:"Instances"`
	StepConcurrencyLevel    int                      `json:"StepConcurrencyLevel,omitempty"`
	EbsRootVolumeSize       int                      `json:"EbsRootVolumeSize,omitempty"`
	EbsRootVolumeIops       int                      `json:"EbsRootVolumeIops,omitempty"`
	EbsRootVolumeThroughput int                      `json:"EbsRootVolumeThroughput,omitempty"`
	VisibleToAllUsers       bool                     `json:"VisibleToAllUsers"`
	SessionEnabled          bool                     `json:"SessionEnabled,omitempty"`
}

// ListClustersParams holds filter and pagination params for ListClusters.
type ListClustersParams struct {
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Marker        string
	ClusterStates []string
}

// ListInstancesParams holds filter params for ListInstances.
type ListInstancesParams struct {
	InstanceGroupID    string
	InstanceFleetID    string
	InstanceFleetType  string
	Marker             string
	InstanceGroupTypes []string
	InstanceStates     []string
}

// InstanceGroupModification describes a single instance group count change.
type InstanceGroupModification struct {
	InstanceGroupID string `json:"InstanceGroupId"`
	InstanceCount   int    `json:"InstanceCount"`
}

// InstanceFleetModification describes a fleet target capacity change.
type InstanceFleetModification struct {
	InstanceFleetID        string `json:"InstanceFleetId"`
	TargetOnDemandCapacity int    `json:"TargetOnDemandCapacity,omitempty"`
	TargetSpotCapacity     int    `json:"TargetSpotCapacity,omitempty"`
}

// SecurityConfigSummary is returned by ListSecurityConfigurations.
// CreationDateTime is epoch seconds (float64); see SecurityConfiguration for why.
type SecurityConfigSummary struct {
	Name             string  `json:"Name"`
	CreationDateTime float64 `json:"CreationDateTime"`
}

// JobFlow is the legacy format returned by DescribeJobFlows.
type JobFlow struct {
	JobFlowID             string                       `json:"JobFlowId"`
	Name                  string                       `json:"Name"`
	ReleaseLabel          string                       `json:"ReleaseLabel,omitempty"`
	LogURI                string                       `json:"LogUri,omitempty"`
	ServiceRole           string                       `json:"ServiceRole,omitempty"`
	Instances             JobFlowInstancesDetail       `json:"Instances"`
	ExecutionStatusDetail JobFlowExecutionStatusDetail `json:"ExecutionStatusDetail"`
}

// JobFlowExecutionStatusDetail holds the legacy execution status.
//
// StateChangeReason is tagged LastStateChangeReason: the real
// types.JobFlowExecutionStatusDetail has no StateChangeReason member at all
// (deserializers.go's awsAwsjson11_deserializeDocumentJobFlowExecutionStatusDetail
// case list has only LastStateChangeReason) -- every real client's
// LastStateChangeReason decoded empty regardless of backend state. This is a
// different type from Cluster's ClusterStateChangeReason (Code/Message) and
// from Session's StateChangeReason (itself correctly named) -- do not
// generalise the tag across types.
type JobFlowExecutionStatusDetail struct {
	State             string  `json:"State"`
	StateChangeReason string  `json:"LastStateChangeReason,omitempty"`
	CreationDateTime  float64 `json:"CreationDateTime"`
	EndDateTime       float64 `json:"EndDateTime,omitempty"`
}

// JobFlowInstancesDetail holds the legacy instances detail.
type JobFlowInstancesDetail struct {
	MasterInstanceType string `json:"MasterInstanceType,omitempty"`
	SlaveInstanceType  string `json:"SlaveInstanceType,omitempty"`
	InstanceCount      int    `json:"InstanceCount"`
}

// ListNotebookExecutionsParams holds filters for ListNotebookExecutions.
type ListNotebookExecutionsParams struct {
	From              *time.Time
	To                *time.Time
	EditorID          string
	ExecutionEngineID string
	Status            string
	Marker            string
}

// SessionCloudWatchLoggingConfiguration is the CloudWatch Logs configuration
// for a session (types.SessionCloudWatchLoggingConfiguration).
type SessionCloudWatchLoggingConfiguration struct {
	LogTypes            map[string][]string `json:"LogTypes,omitempty"`
	EncryptionKeyArn    string              `json:"EncryptionKeyArn,omitempty"`
	LogGroup            string              `json:"LogGroup,omitempty"`
	LogStreamNamePrefix string              `json:"LogStreamNamePrefix,omitempty"`
	Enabled             bool                `json:"Enabled,omitempty"`
}

// SessionManagedLoggingConfiguration is the Amazon EMR-managed logging
// configuration for a session (types.SessionManagedLoggingConfiguration).
type SessionManagedLoggingConfiguration struct {
	EncryptionKeyArn string `json:"EncryptionKeyArn,omitempty"`
	Enabled          bool   `json:"Enabled,omitempty"`
}

// SessionS3LoggingConfiguration is the Amazon S3 logging configuration for a
// session (types.SessionS3LoggingConfiguration).
type SessionS3LoggingConfiguration struct {
	LogTypes         map[string][]string `json:"LogTypes,omitempty"`
	EncryptionKeyArn string              `json:"EncryptionKeyArn,omitempty"`
	LogURI           string              `json:"LogUri,omitempty"`
	Enabled          bool                `json:"Enabled,omitempty"`
}

// SessionMonitoringConfiguration controls where a session's logs are
// published (types.SessionMonitoringConfiguration).
type SessionMonitoringConfiguration struct {
	CloudWatchLoggingConfiguration *SessionCloudWatchLoggingConfiguration `json:"CloudWatchLoggingConfiguration,omitempty"`
	ManagedLoggingConfiguration    *SessionManagedLoggingConfiguration    `json:"ManagedLoggingConfiguration,omitempty"`
	S3LoggingConfiguration         *SessionS3LoggingConfiguration         `json:"S3LoggingConfiguration,omitempty"`
}

// Session represents an interactive (Spark Connect) session running on an
// EMR cluster (types.Session). See sessions.go's package doc comment for
// the state-model rationale.
//
// CreatedAt/UpdatedAt are always populated once a session exists, so they
// carry no omitempty (matching Cluster/SecurityConfiguration's convention
// for "always set" epoch-seconds fields elsewhere in this package).
// EndedAt/IdleSince/StartedAt are genuine real Timestamp members this
// backend only sometimes populates (IdleSince/StartedAt are never
// populated at all -- see sessions.go), so they use omitempty, matching the
// real optional *time.Time members on types.Session. All are epoch seconds
// (float64), matching EMR's awsjson1.1 wire format -- see
// SecurityConfiguration for why.
type Session struct {
	MonitoringConfiguration     *SessionMonitoringConfiguration `json:"MonitoringConfiguration,omitempty"`
	ID                          string                          `json:"Id"`
	ClusterID                   string                          `json:"ClusterId"`
	ARN                         string                          `json:"Arn"`
	State                       string                          `json:"State"`
	AccountID                   string                          `json:"AccountId,omitempty"`
	Name                        string                          `json:"Name,omitempty"`
	ExecutionRoleArn            string                          `json:"ExecutionRoleArn,omitempty"`
	ReleaseLabel                string                          `json:"ReleaseLabel,omitempty"`
	ServerURL                   string                          `json:"ServerUrl,omitempty"`
	StateChangeReason           string                          `json:"StateChangeReason,omitempty"`
	EngineConfigurations        []Configuration                 `json:"EngineConfigurations,omitempty"`
	Tags                        []Tag                           `json:"Tags"`
	SessionIdleTimeoutInMinutes int64                           `json:"SessionIdleTimeoutInMinutes,omitempty"`
	CreatedAt                   float64                         `json:"CreatedAt"`
	UpdatedAt                   float64                         `json:"UpdatedAt"`
	EndedAt                     float64                         `json:"EndedAt,omitempty"`
	IdleSince                   float64                         `json:"IdleSince,omitempty"`
	StartedAt                   float64                         `json:"StartedAt,omitempty"`
}

// StartSessionParams is the input for creating a new session.
type StartSessionParams struct {
	MonitoringConfiguration     *SessionMonitoringConfiguration
	ClusterID                   string
	Name                        string
	ExecutionRoleArn            string
	EngineConfigurations        []Configuration
	Tags                        []Tag
	SessionIdleTimeoutInMinutes int64
}

// SessionEndpointResult is the output of GetSessionEndpoint.
type SessionEndpointResult struct {
	Expiry      time.Time
	Credentials map[string]any
	Endpoint    string
	AuthToken   string
}
