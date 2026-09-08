package batch

import "time"

// --- ComputeResources sub-types ---

// LaunchTemplateOverride specifies a launch template override for specific instance types.
type LaunchTemplateOverride struct {
	LaunchTemplateName  string   `json:"launchTemplateName,omitempty"`
	LaunchTemplateID    string   `json:"launchTemplateId,omitempty"`
	Version             string   `json:"version,omitempty"`
	TargetInstanceTypes []string `json:"targetInstanceTypes,omitempty"`
}

// LaunchTemplate specifies an EC2 launch template.
type LaunchTemplate struct {
	LaunchTemplateName string                   `json:"launchTemplateName,omitempty"`
	LaunchTemplateID   string                   `json:"launchTemplateId,omitempty"`
	Version            string                   `json:"version,omitempty"`
	Overrides          []LaunchTemplateOverride `json:"overrides,omitempty"`
}

// Ec2Configuration specifies AMI matching configuration for EC2 compute environments.
type Ec2Configuration struct {
	ImageType              string `json:"imageType"`
	ImageIDOverride        string `json:"imageIdOverride,omitempty"`
	ImageKubernetesVersion string `json:"imageKubernetesVersion,omitempty"`
}

// ComputeResources holds compute resource configuration for a managed CE.
type ComputeResources struct {
	Type               string             `json:"type,omitempty"`
	AllocationStrategy string             `json:"allocationStrategy,omitempty"`
	InstanceRole       string             `json:"instanceRole,omitempty"`
	Ec2KeyPair         string             `json:"ec2KeyPair,omitempty"`
	ImageID            string             `json:"imageId,omitempty"`
	PlacementGroup     string             `json:"placementGroup,omitempty"`
	SpotIamFleetRole   string             `json:"spotIamFleetRole,omitempty"`
	InstanceTypes      []string           `json:"instanceTypes,omitempty"`
	Subnets            []string           `json:"subnets,omitempty"`
	SecurityGroupIDs   []string           `json:"securityGroupIds,omitempty"`
	Tags               map[string]string  `json:"tags,omitempty"`
	LaunchTemplate     *LaunchTemplate    `json:"launchTemplate,omitempty"`
	Ec2Configuration   []Ec2Configuration `json:"ec2Configuration,omitempty"`
	MinvCpus           int32              `json:"minvCpus,omitempty"`
	// MaxvCpus is required whenever ComputeResources is present (the real
	// SDK client only rejects a nil pointer, not a zero value), so it must
	// never be dropped even when explicitly 0.
	MaxvCpus      int32 `json:"maxvCpus"`
	DesiredvCpus  int32 `json:"desiredvCpus,omitempty"`
	BidPercentage int32 `json:"bidPercentage,omitempty"`
}

// EksConfiguration specifies EKS cluster configuration for a CE.
type EksConfiguration struct {
	EksClusterArn       string `json:"eksClusterArn"`
	KubernetesNamespace string `json:"kubernetesNamespace"`
}

// UpdatePolicy controls behaviour during in-place CE updates.
type UpdatePolicy struct {
	TerminateJobsOnUpdate      bool  `json:"terminateJobsOnUpdate,omitempty"`
	JobExecutionTimeoutMinutes int64 `json:"jobExecutionTimeoutMinutes,omitempty"`
}

// ComputeEnvironment represents a Batch compute environment.
//
// UnmanagedvCpus (CreateComputeEnvironmentInput/UpdateComputeEnvironmentInput/
// types.ComputeEnvironmentDetail) is only meaningful for UNMANAGED compute
// environments; the real SDK client only rejects a nil pointer, not zero, so
// this must round-trip a real 0 too -- kept as *int32 (not plain int32)
// since real AWS omits this field entirely for MANAGED compute environments
// rather than emitting zero.
//
// ContainerOrchestrationType ("ECS (default) or EKS",
// types.ComputeEnvironmentDetail) is deterministic from whether
// EksConfiguration was set at creation -- computed once and stored, not
// re-derived, since it cannot change after creation either.
//
// UUID ("Unique identifier for the compute environment",
// types.ComputeEnvironmentDetail.Uuid) is an opaque AWS-generated
// identifier, generated once at creation like every other resource's Id in
// this service.
type ComputeEnvironment struct {
	Tags                       map[string]string `json:"tags"`
	ComputeResources           *ComputeResources `json:"computeResources,omitempty"`
	EksConfiguration           *EksConfiguration `json:"eksConfiguration,omitempty"`
	UpdatePolicy               *UpdatePolicy     `json:"updatePolicy,omitempty"`
	UnmanagedvCpus             *int32            `json:"unmanagedvCpus,omitempty"`
	ComputeEnvironmentArn      string            `json:"computeEnvironmentArn"`
	ServiceRole                string            `json:"serviceRole,omitempty"`
	Type                       string            `json:"type"`
	State                      string            `json:"state"`
	Status                     string            `json:"status"`
	StatusReason               string            `json:"statusReason,omitempty"`
	ComputeEnvironmentName     string            `json:"computeEnvironmentName"`
	ContainerOrchestrationType string            `json:"containerOrchestrationType,omitempty"`
	UUID                       string            `json:"uuid,omitempty"`
	region                     string
}

// ComputeEnvironmentOrder pairs a compute environment with its ordering in a job queue.
type ComputeEnvironmentOrder struct {
	ComputeEnvironment string `json:"computeEnvironment"`
	Order              int32  `json:"order"`
}

// JobStateTimeLimitAction cancels jobs stuck in a given state beyond a time limit.
type JobStateTimeLimitAction struct {
	Reason         string `json:"reason"`
	State          string `json:"state"`
	Action         string `json:"action"`
	MaxTimeSeconds int32  `json:"maxTimeSeconds"`
}

// ServiceEnvironmentOrder pairs a service environment with its ordering in a job queue.
type ServiceEnvironmentOrder struct {
	ServiceEnvironment string `json:"serviceEnvironment"`
	Order              int32  `json:"order"`
}

// JobQueue represents a Batch job queue.
type JobQueue struct {
	Tags map[string]string `json:"tags"`
	// region is the store.Table composite-key qualifier (see regionKey); see
	// ComputeEnvironment.region for why it is unexported.
	region              string
	JobQueueName        string `json:"jobQueueName"`
	JobQueueArn         string `json:"jobQueueArn"`
	State               string `json:"state"`
	Status              string `json:"status"`
	StatusReason        string `json:"statusReason,omitempty"`
	SchedulingPolicyArn string `json:"schedulingPolicyArn,omitempty"`
	JobQueueType        string `json:"jobQueueType,omitempty"`
	// ComputeEnvironmentOrder is required on JobQueueDetail even when the
	// queue was built purely from ServiceEnvironmentOrder instead (the two
	// are mutually exclusive on input) -- must serialize as [] not be
	// omitted; see cloneJobQueueWithTags.
	ComputeEnvironmentOrder  []ComputeEnvironmentOrder `json:"computeEnvironmentOrder"`
	ServiceEnvironmentOrder  []ServiceEnvironmentOrder `json:"serviceEnvironmentOrder,omitempty"`
	JobStateTimeLimitActions []JobStateTimeLimitAction `json:"jobStateTimeLimitActions,omitempty"`
	Priority                 int32                     `json:"priority"`
}

// --- ContainerProperties sub-types ---

// KeyValuePair is a name/value environment variable pair.
type KeyValuePair struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// ResourceRequirement specifies a compute resource requirement (VCPU, MEMORY, or GPU).
type ResourceRequirement struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// HostVolume specifies a host-path volume binding.
type HostVolume struct {
	SourcePath string `json:"sourcePath,omitempty"`
}

// Volume specifies a volume available to containers.
type Volume struct {
	Host *HostVolume `json:"host,omitempty"`
	Name string      `json:"name"`
}

// MountPoint maps a volume into a container.
type MountPoint struct {
	ContainerPath string `json:"containerPath,omitempty"`
	SourceVolume  string `json:"sourceVolume,omitempty"`
	ReadOnly      bool   `json:"readOnly,omitempty"`
}

// Ulimit specifies a ulimit for a container.
type Ulimit struct {
	Name      string `json:"name"`
	SoftLimit int32  `json:"softLimit"`
	HardLimit int32  `json:"hardLimit"`
}

// LogConfiguration specifies the log driver configuration for a container.
type LogConfiguration struct {
	Options   map[string]string `json:"options,omitempty"`
	LogDriver string            `json:"logDriver"`
}

// NetworkConfiguration specifies network settings for Fargate containers.
type NetworkConfiguration struct {
	AssignPublicIP string `json:"assignPublicIp,omitempty"`
}

// FargatePlatformConfiguration specifies the Fargate platform version.
type FargatePlatformConfiguration struct {
	PlatformVersion string `json:"platformVersion,omitempty"`
}

// EphemeralStorage specifies ephemeral storage capacity for Fargate tasks.
type EphemeralStorage struct {
	SizeInGiB int32 `json:"sizeInGiB"`
}

// RuntimePlatform specifies the OS family and CPU architecture for a job.
type RuntimePlatform struct {
	OperatingSystemFamily string `json:"operatingSystemFamily,omitempty"`
	CPUArchitecture       string `json:"cpuArchitecture,omitempty"`
}

// RepositoryCredentials specifies credentials for a private container registry.
type RepositoryCredentials struct {
	CredentialsParameter string `json:"credentialsParameter"`
}

// Secret specifies a secret to expose to a container via environment variable.
type Secret struct {
	Name      string `json:"name"`
	ValueFrom string `json:"valueFrom"`
}

// Device specifies a device to expose to a container.
type Device struct {
	HostPath      string   `json:"hostPath"`
	ContainerPath string   `json:"containerPath,omitempty"`
	Permissions   []string `json:"permissions,omitempty"`
}

// Tmpfs specifies a tmpfs mount for a container.
type Tmpfs struct {
	ContainerPath string   `json:"containerPath"`
	MountOptions  []string `json:"mountOptions,omitempty"`
	Size          int32    `json:"size"`
}

// LinuxParameters configures Linux-specific container settings.
type LinuxParameters struct {
	Devices            []Device `json:"devices,omitempty"`
	Tmpfs              []Tmpfs  `json:"tmpfs,omitempty"`
	InitProcessEnabled bool     `json:"initProcessEnabled,omitempty"`
	SharedMemorySize   int32    `json:"sharedMemorySize,omitempty"`
	MaxSwap            int32    `json:"maxSwap,omitempty"`
	Swappiness         int32    `json:"swappiness,omitempty"`
}

// ContainerProperties stores container configuration for a job definition.
type ContainerProperties struct {
	LinuxParameters              *LinuxParameters              `json:"linuxParameters,omitempty"`
	RepositoryCredentials        *RepositoryCredentials        `json:"repositoryCredentials,omitempty"`
	RuntimePlatform              *RuntimePlatform              `json:"runtimePlatform,omitempty"`
	EphemeralStorage             *EphemeralStorage             `json:"ephemeralStorage,omitempty"`
	FargatePlatformConfiguration *FargatePlatformConfiguration `json:"fargatePlatformConfiguration,omitempty"`
	NetworkConfiguration         *NetworkConfiguration         `json:"networkConfiguration,omitempty"`
	LogConfiguration             *LogConfiguration             `json:"logConfiguration,omitempty"`
	JobRoleArn                   string                        `json:"jobRoleArn,omitempty"`
	ExecutionRoleArn             string                        `json:"executionRoleArn,omitempty"`
	User                         string                        `json:"user,omitempty"`
	InstanceType                 string                        `json:"instanceType,omitempty"`
	Image                        string                        `json:"image,omitempty"`
	Command                      []string                      `json:"command,omitempty"`
	Secrets                      []Secret                      `json:"secrets,omitempty"`
	ResourceRequirements         []ResourceRequirement         `json:"resourceRequirements,omitempty"`
	Ulimits                      []Ulimit                      `json:"ulimits,omitempty"`
	MountPoints                  []MountPoint                  `json:"mountPoints,omitempty"`
	Volumes                      []Volume                      `json:"volumes,omitempty"`
	Environment                  []KeyValuePair                `json:"environment,omitempty"`
	Vcpus                        int32                         `json:"vcpus,omitempty"`
	Memory                       int32                         `json:"memory,omitempty"`
	ReadonlyRootFilesystem       bool                          `json:"readonlyRootFilesystem,omitempty"`
	Privileged                   bool                          `json:"privileged,omitempty"`
}

// --- JobDefinition sub-types ---

// NodeRangeProperty specifies container properties for a range of multi-node job nodes.
type NodeRangeProperty struct {
	ContainerProperties *ContainerProperties `json:"containerProperties,omitempty"`
	TargetNodes         string               `json:"targetNodes"`
}

// NodeProperties specifies multi-node parallel job configuration.
type NodeProperties struct {
	NodeRangeProperties []NodeRangeProperty `json:"nodeRangeProperties"`
	NumNodes            int32               `json:"numNodes"`
	MainNode            int32               `json:"mainNode"`
}

// EksContainerEnv is a name/value env var for EKS containers.
type EksContainerEnv struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

// EksContainerResources specifies resource limits and requests for EKS containers.
type EksContainerResources struct {
	Limits   map[string]string `json:"limits,omitempty"`
	Requests map[string]string `json:"requests,omitempty"`
}

// EksVolumeMount mounts a volume into an EKS container.
type EksVolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

// EksSecurityContext specifies security settings for an EKS container.
type EksSecurityContext struct {
	RunAsUser                *int64 `json:"runAsUser,omitempty"`
	RunAsGroup               *int64 `json:"runAsGroup,omitempty"`
	Privileged               bool   `json:"privileged,omitempty"`
	ReadOnlyRootFilesystem   bool   `json:"readOnlyRootFilesystem,omitempty"`
	RunAsNonRoot             bool   `json:"runAsNonRoot,omitempty"`
	AllowPrivilegeEscalation bool   `json:"allowPrivilegeEscalation,omitempty"`
}

// EksContainer specifies an EKS pod container.
type EksContainer struct {
	Resources       *EksContainerResources `json:"resources,omitempty"`
	SecurityContext *EksSecurityContext    `json:"securityContext,omitempty"`
	Name            string                 `json:"name"`
	Image           string                 `json:"image"`
	ImagePullPolicy string                 `json:"imagePullPolicy,omitempty"`
	Command         []string               `json:"command,omitempty"`
	Args            []string               `json:"args,omitempty"`
	Env             []EksContainerEnv      `json:"env,omitempty"`
	VolumeMounts    []EksVolumeMount       `json:"volumeMounts,omitempty"`
}

// EksHostPath specifies a host path volume for EKS.
type EksHostPath struct {
	Path string `json:"path,omitempty"`
}

// EksEmptyDir specifies an emptyDir volume for EKS.
type EksEmptyDir struct {
	Medium    string `json:"medium,omitempty"`
	SizeLimit string `json:"sizeLimit,omitempty"`
}

// EksSecret specifies a Kubernetes secret volume for EKS.
type EksSecret struct {
	SecretName string `json:"secretName"`
	Optional   bool   `json:"optional,omitempty"`
}

// ImagePullSecret specifies an image pull secret for an EKS pod.
type ImagePullSecret struct {
	Name string `json:"name"`
}

// EksVolume specifies a volume available to EKS pod containers.
type EksVolume struct {
	HostPath *EksHostPath `json:"hostPath,omitempty"`
	EmptyDir *EksEmptyDir `json:"emptyDir,omitempty"`
	Secret   *EksSecret   `json:"secret,omitempty"`
	Name     string       `json:"name"`
}

// EksMetadata holds labels and annotations for an EKS pod.
type EksMetadata struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// EksPodProperties specifies the Kubernetes pod spec for an EKS job.
type EksPodProperties struct {
	Metadata           *EksMetadata      `json:"metadata,omitempty"`
	ServiceAccountName string            `json:"serviceAccountName,omitempty"`
	DNSPolicy          string            `json:"dnsPolicy,omitempty"`
	Containers         []EksContainer    `json:"containers,omitempty"`
	InitContainers     []EksContainer    `json:"initContainers,omitempty"`
	ImagePullSecrets   []ImagePullSecret `json:"imagePullSecrets,omitempty"`
	Volumes            []EksVolume       `json:"volumes,omitempty"`
	HostNetwork        bool              `json:"hostNetwork,omitempty"`
}

// EksProperties specifies EKS-specific job definition properties.
type EksProperties struct {
	PodProperties *EksPodProperties `json:"podProperties,omitempty"`
}

// ConsumableResourceProperty specifies a single consumable resource requirement.
// Quantity is int64 (Long) to match aws-sdk-go-v2/service/batch/types.
// ConsumableResourceRequirement.Quantity exactly; it was previously float64,
// which is wrong for the real API (see PARITY.md gaps).
type ConsumableResourceProperty struct {
	ConsumableResource string `json:"consumableResource"`
	Quantity           int64  `json:"quantity"`
}

// ConsumableResourceProperties holds the consumable resources required by a
// job or job definition. The real Batch API nests the requirement list under
// "consumableResourceList" rather than serialising it as a bare array; wrap
// it here so the wire shape matches (see aws-sdk-go-v2/service/batch/types.
// ConsumableResourceProperties).
type ConsumableResourceProperties struct {
	ConsumableResourceList []ConsumableResourceProperty `json:"consumableResourceList,omitempty"`
}

// JobDefinition represents a Batch job definition.
type JobDefinition struct {
	DeregisteredAt      *time.Time           `json:"deregisteredAt,omitempty"`
	Tags                map[string]string    `json:"tags"`
	Parameters          map[string]string    `json:"parameters,omitempty"`
	ContainerProperties *ContainerProperties `json:"containerProperties,omitempty"`
	NodeProperties      *NodeProperties      `json:"nodeProperties,omitempty"`
	EksProperties       *EksProperties       `json:"eksProperties,omitempty"`
	RuntimePlatform     *RuntimePlatform     `json:"runtimePlatform,omitempty"`
	// RetryStrategy is the job-definition-level default retry strategy (real
	// AWS Batch supports this in addition to the job-level RetryStrategy
	// passed to SubmitJob; see aws-sdk-go-v2/service/batch/types.
	// RegisterJobDefinitionInput.RetryStrategy).
	RetryStrategy *RetryStrategy `json:"retryStrategy,omitempty"`
	// Timeout is nested (wire key "timeout": {"attemptDurationSeconds": N}) to
	// match aws-sdk-go-v2/service/batch/types.JobDefinition.Timeout; it must
	// NOT be a flat "timeoutSeconds" integer.
	Timeout *JobTimeout `json:"timeout,omitempty"`
	// region is the store.Table composite-key qualifier (see regionKey); see
	// ComputeEnvironment.region for why it is unexported.
	region                       string
	ConsumableResourceProperties *ConsumableResourceProperties `json:"consumableResourceProperties,omitempty"`
	JobDefinitionName            string                        `json:"jobDefinitionName"`
	JobDefinitionArn             string                        `json:"jobDefinitionArn"`
	Type                         string                        `json:"type"`
	Status                       string                        `json:"status"`
	PlatformCapabilities         []string                      `json:"platformCapabilities,omitempty"`
	Revision                     int32                         `json:"revision"`
	SchedulingPriority           int32                         `json:"schedulingPriority,omitempty"`
	PropagateTags                bool                          `json:"propagateTags,omitempty"`
}

// --- RetryStrategy ---

// EvaluateOnExit specifies a conditional retry rule evaluated against exit information.
type EvaluateOnExit struct {
	Action         string `json:"action"`
	OnStatusReason string `json:"onStatusReason,omitempty"`
	OnReason       string `json:"onReason,omitempty"`
	OnExitCode     string `json:"onExitCode,omitempty"`
}

// RetryStrategy configures automatic retry behavior for a job.
type RetryStrategy struct {
	EvaluateOnExit []EvaluateOnExit `json:"evaluateOnExit,omitempty"`
	Attempts       int32            `json:"attempts,omitempty"`
}

// JobTimeout configures the maximum duration for a job attempt.
type JobTimeout struct {
	AttemptDurationSeconds int32 `json:"attemptDurationSeconds,omitempty"`
}

// JobDependency represents a dependency between jobs.
type JobDependency struct {
	JobID string `json:"jobId,omitempty"`
	Type  string `json:"type,omitempty"`
}

// ArrayProperties specifies array job fan-out configuration.
type ArrayProperties struct {
	StatusSummary map[string]int32 `json:"statusSummary,omitempty"`
	Size          int32            `json:"size,omitempty"`
	Index         int32            `json:"index,omitempty"`
}

// ContainerOverrides overrides container properties at job submission time.
type ContainerOverrides struct {
	InstanceType         string                `json:"instanceType,omitempty"`
	Command              []string              `json:"command,omitempty"`
	Environment          []KeyValuePair        `json:"environment,omitempty"`
	ResourceRequirements []ResourceRequirement `json:"resourceRequirements,omitempty"`
}

// JobAttemptContainer holds per-attempt container execution details.
type JobAttemptContainer struct {
	LogStreamName string `json:"logStreamName,omitempty"`
	Reason        string `json:"reason,omitempty"`
	ExitCode      int32  `json:"exitCode,omitempty"`
}

// JobAttempt holds per-attempt lifecycle and result information.
type JobAttempt struct {
	Container    *JobAttemptContainer `json:"container,omitempty"`
	StartedAt    *int64               `json:"startedAt,omitempty"`
	StoppedAt    *int64               `json:"stoppedAt,omitempty"`
	StatusReason string               `json:"statusReason,omitempty"`
}

// ContainerDetail mirrors aws-sdk-go-v2/service/batch/types.ContainerDetail:
// the describe-side view of a job's container, which is ContainerProperties
// plus a handful of runtime-only fields (Reason, ExitCode, LogStreamName).
// Placement fields real AWS populates from live ECS/EC2 state
// (ContainerInstanceArn, TaskArn, NetworkInterfaces, EnableExecuteCommand)
// aren't modeled since this emulator doesn't simulate container placement.
type ContainerDetail struct {
	LinuxParameters              *LinuxParameters              `json:"linuxParameters,omitempty"`
	RepositoryCredentials        *RepositoryCredentials        `json:"repositoryCredentials,omitempty"`
	RuntimePlatform              *RuntimePlatform              `json:"runtimePlatform,omitempty"`
	EphemeralStorage             *EphemeralStorage             `json:"ephemeralStorage,omitempty"`
	FargatePlatformConfiguration *FargatePlatformConfiguration `json:"fargatePlatformConfiguration,omitempty"`
	NetworkConfiguration         *NetworkConfiguration         `json:"networkConfiguration,omitempty"`
	LogConfiguration             *LogConfiguration             `json:"logConfiguration,omitempty"`
	ExitCode                     *int32                        `json:"exitCode,omitempty"`
	JobRoleArn                   string                        `json:"jobRoleArn,omitempty"`
	ExecutionRoleArn             string                        `json:"executionRoleArn,omitempty"`
	User                         string                        `json:"user,omitempty"`
	InstanceType                 string                        `json:"instanceType,omitempty"`
	Image                        string                        `json:"image,omitempty"`
	Reason                       string                        `json:"reason,omitempty"`
	LogStreamName                string                        `json:"logStreamName,omitempty"`
	Command                      []string                      `json:"command,omitempty"`
	Secrets                      []Secret                      `json:"secrets,omitempty"`
	ResourceRequirements         []ResourceRequirement         `json:"resourceRequirements,omitempty"`
	Ulimits                      []Ulimit                      `json:"ulimits,omitempty"`
	MountPoints                  []MountPoint                  `json:"mountPoints,omitempty"`
	Volumes                      []Volume                      `json:"volumes,omitempty"`
	Environment                  []KeyValuePair                `json:"environment,omitempty"`
	Vcpus                        int32                         `json:"vcpus,omitempty"`
	Memory                       int32                         `json:"memory,omitempty"`
	ReadonlyRootFilesystem       bool                          `json:"readonlyRootFilesystem,omitempty"`
	Privileged                   bool                          `json:"privileged,omitempty"`
}

// Job represents a submitted Batch job.
type Job struct {
	ContainerOverrides *ContainerOverrides `json:"containerOverrides,omitempty"`
	Tags               map[string]string   `json:"tags"`
	Parameters         map[string]string   `json:"parameters,omitempty"`
	StartedAt          *int64              `json:"startedAt,omitempty"`
	StoppedAt          *int64              `json:"stoppedAt,omitempty"`
	RetryStrategy      *RetryStrategy      `json:"retryStrategy,omitempty"`
	Timeout            *JobTimeout         `json:"timeout,omitempty"`
	ArrayProperties    *ArrayProperties    `json:"arrayProperties,omitempty"`
	// Container is derived (not stored directly by callers) from the resolved
	// job definition's ContainerProperties merged with ContainerOverrides; it
	// is populated by DescribeJobs. Left nil for multi-node jobs, matching
	// AWS's "for a multiple-container job, this object will be empty" note.
	Container *ContainerDetail `json:"container,omitempty"`
	// region is the store.Table composite-key qualifier (see regionKey); see
	// ComputeEnvironment.region for why it is unexported.
	region                       string
	JobDefinition                string                        `json:"jobDefinition"`
	ShareIdentifier              string                        `json:"shareIdentifier,omitempty"`
	StatusReason                 string                        `json:"statusReason,omitempty"`
	JobID                        string                        `json:"jobId"`
	JobARN                       string                        `json:"jobArn"`
	JobName                      string                        `json:"jobName"`
	JobQueue                     string                        `json:"jobQueue"`
	Status                       string                        `json:"status"`
	DependsOn                    []JobDependency               `json:"dependsOn,omitempty"`
	ConsumableResourceProperties *ConsumableResourceProperties `json:"consumableResourceProperties,omitempty"`
	Attempts                     []JobAttempt                  `json:"attempts,omitempty"`
	// PlatformCapabilities is copied from the resolved job definition at
	// SubmitJob time (real AWS defaults to ["EC2"] when unspecified).
	PlatformCapabilities       []string `json:"platformCapabilities,omitempty"`
	CreatedAt                  int64    `json:"createdAt"`
	SchedulingPriorityOverride int32    `json:"schedulingPriorityOverride,omitempty"`
	// attemptCount is the number of attempts that have run so far, used by the
	// janitor to decide whether a timed-out attempt is retried (RetryStrategy.
	// Attempts) or the job is failed for good. Unexported: internal bookkeeping,
	// not part of the wire shape.
	attemptCount  int32
	PropagateTags bool `json:"propagateTags,omitempty"`
	// IsCancelled/IsTerminated are set by CancelJob/TerminateJob respectively;
	// see aws-sdk-go-v2/service/batch/types.JobDetail.IsCancelled/IsTerminated.
	IsCancelled  bool `json:"isCancelled"`
	IsTerminated bool `json:"isTerminated"`
}

// ConsumableResource represents a Batch consumable resource.
type ConsumableResource struct {
	Tags map[string]string `json:"tags"`
	// region is the store.Table composite-key qualifier (see regionKey); see
	// ComputeEnvironment.region for why it is unexported.
	region                 string
	ConsumableResourceName string `json:"consumableResourceName"`
	ConsumableResourceArn  string `json:"consumableResourceArn"`
	ResourceType           string `json:"resourceType,omitempty"`
	CreatedAt              int64  `json:"createdAt"`
	TotalQuantity          int64  `json:"totalQuantity"`
	AvailableQuantity      int64  `json:"availableQuantity"`
	InUseQuantity          int64  `json:"inUseQuantity"`
}

// ShareDistribution specifies a fair-share weight for a share identifier.
type ShareDistribution struct {
	ShareIdentifier string  `json:"shareIdentifier"`
	WeightFactor    float32 `json:"weightFactor,omitempty"`
}

// FairsharePolicy configures fair-share scheduling for a scheduling policy.
type FairsharePolicy struct {
	ShareDistribution  []ShareDistribution `json:"shareDistribution,omitempty"`
	ShareDecaySeconds  int32               `json:"shareDecaySeconds,omitempty"`
	ComputeReservation int32               `json:"computeReservation,omitempty"`
}

// QuotaSharePolicy configures quota-share scheduling for a scheduling
// policy -- an alternative to FairsharePolicy, distinct from the separate
// top-level QuotaShare resource family (CreateQuotaShare etc., which
// associates a quota share with a job queue directly). See
// aws-sdk-go-v2/service/batch/types.QuotaSharePolicy.
type QuotaSharePolicy struct {
	// Real AWS docs: "Currently, only FIFO is supported." Accepted and
	// stored as given, not validated against that single value, matching
	// this file's existing FairsharePolicy precedent of not validating
	// enum-shaped sibling fields.
	IdleResourceAssignmentStrategy string `json:"idleResourceAssignmentStrategy,omitempty"`
}

// SchedulingPolicy represents a Batch scheduling policy.
type SchedulingPolicy struct {
	Tags             map[string]string `json:"tags"`
	FairsharePolicy  *FairsharePolicy  `json:"fairsharePolicy,omitempty"`
	QuotaSharePolicy *QuotaSharePolicy `json:"quotaSharePolicy,omitempty"`
	// region is the store.Table composite-key qualifier (see regionKey); see
	// ComputeEnvironment.region for why it is unexported.
	region string
	Arn    string `json:"arn"`
	Name   string `json:"name"`
}

// QuotaShareCapacityLimit specifies the quantity and type of compute capacity
// allocated to a quota share. See
// aws-sdk-go-v2/service/batch/types.QuotaShareCapacityLimit -- both fields
// are required on the real API. CapacityUnit keeps omitempty: this
// backend's own CreateQuotaShare/UpdateQuotaShare validation already rejects
// an empty capacityUnit (see quota_shares.go), so unlike MaxCapacity, no
// client can ever store one empty here. MaxCapacity has no such guard --
// the real SDK client only rejects a nil pointer, not zero -- so it must
// never be dropped.
type QuotaShareCapacityLimit struct {
	CapacityUnit string `json:"capacityUnit,omitempty"`
	MaxCapacity  int32  `json:"maxCapacity"`
}

// QuotaSharePreemptionConfiguration specifies the preemption behavior for
// jobs in a quota share. See
// aws-sdk-go-v2/service/batch/types.QuotaSharePreemptionConfiguration.
type QuotaSharePreemptionConfiguration struct {
	InSharePreemption string `json:"inSharePreemption,omitempty"`
}

// QuotaShareResourceSharingConfiguration specifies whether a quota share
// reserves, lends, or both lends and borrows idle compute capacity. See
// aws-sdk-go-v2/service/batch/types.QuotaShareResourceSharingConfiguration.
type QuotaShareResourceSharingConfiguration struct {
	Strategy    string `json:"strategy,omitempty"`
	BorrowLimit int32  `json:"borrowLimit,omitempty"`
}

// QuotaShare represents a Batch quota share: a virtual queue with a
// configured compute capacity, resource sharing strategy, and borrow limits,
// associated with an existing JobQueue (see CreateQuotaShareInput's
// required jobQueue field in aws-sdk-go-v2/service/batch). This is a
// distinct top-level resource family from SchedulingPolicy/FairsharePolicy/
// ShareIdentifier -- CreateQuotaShareInput has no schedulingPolicyArn or
// shareIdentifier field at all, and QuotaShareDetail's ARN shape
// (job-queue/{queueName}/quota-share/{quotaShareName}, confirmed against the
// AWS API reference's CreateQuotaShare example) nests under the job queue's
// own ARN rather than referencing a SchedulingPolicy resource.
type QuotaShare struct {
	Tags                         map[string]string                       `json:"tags"`
	PreemptionConfiguration      *QuotaSharePreemptionConfiguration      `json:"preemptionConfiguration,omitempty"`
	ResourceSharingConfiguration *QuotaShareResourceSharingConfiguration `json:"resourceSharingConfiguration,omitempty"`
	region                       string
	QuotaShareArn                string                    `json:"quotaShareArn"`
	QuotaShareName               string                    `json:"quotaShareName"`
	JobQueueArn                  string                    `json:"jobQueueArn,omitempty"`
	State                        string                    `json:"state,omitempty"`
	Status                       string                    `json:"status,omitempty"`
	CapacityLimits               []QuotaShareCapacityLimit `json:"capacityLimits,omitempty"`
}

// CapacityLimit specifies the maximum capacity available for a service environment.
type CapacityLimit struct {
	CapacityUnit string `json:"capacityUnit,omitempty"`
	MaxCapacity  int32  `json:"maxCapacity,omitempty"`
}

// ServiceEnvironment represents a Batch service environment.
type ServiceEnvironment struct {
	Tags map[string]string `json:"tags"`
	// region is the store.Table composite-key qualifier (see regionKey); see
	// ComputeEnvironment.region for why it is unexported.
	region                 string
	ServiceEnvironmentName string `json:"serviceEnvironmentName"`
	ServiceEnvironmentArn  string `json:"serviceEnvironmentArn"`
	ServiceEnvironmentType string `json:"serviceEnvironmentType"`
	State                  string `json:"state"`
	Status                 string `json:"status"`
	// CapacityLimits is required by the real API on Create and in the
	// ServiceEnvironmentDetail response (see aws-sdk-go-v2/service/batch/
	// types.ServiceEnvironmentDetail.CapacityLimits); it was previously
	// missing entirely from this model.
	CapacityLimits []CapacityLimit `json:"capacityLimits"`
}

// ServiceJobEvaluateOnExit specifies a conditional retry rule for a service job.
type ServiceJobEvaluateOnExit struct {
	Action         string `json:"action"`
	OnStatusReason string `json:"onStatusReason,omitempty"`
}

// ServiceJobRetryStrategy configures automatic retry behavior for a service job.
// See aws-sdk-go-v2/service/batch/types.ServiceJobRetryStrategy; this is a
// distinct (and structurally narrower) type from the regular Job's
// RetryStrategy -- it has no OnReason/OnExitCode matching.
type ServiceJobRetryStrategy struct {
	EvaluateOnExit []ServiceJobEvaluateOnExit `json:"evaluateOnExit,omitempty"`
	// Attempts is required whenever RetryStrategy is present -- the real SDK
	// client only rejects a nil pointer, not zero -- so it must never be
	// dropped even when explicitly 0.
	Attempts int32 `json:"attempts"`
}

// ServiceJobTimeout configures the maximum duration for a service job attempt.
type ServiceJobTimeout struct {
	AttemptDurationSeconds int32 `json:"attemptDurationSeconds,omitempty"`
}

// ServiceJobPreemptionConfiguration configures whether/how many times a
// preempted service job is retried before termination. See
// aws-sdk-go-v2/service/batch/types.ServiceJobPreemptionConfiguration.
// Request-settable and stored verbatim; distinct from
// ServiceJobPreemptionSummary (response-only, actual preemption history --
// this backend never preempts service jobs, so that summary is never
// populated; see DescribeServiceJob's disclosed gap).
type ServiceJobPreemptionConfiguration struct {
	// nil means "unset" (real AWS: "preempted jobs will be requeued an
	// unlimited number of times"), distinct from a present 0.
	PreemptionRetriesBeforeTermination *int32 `json:"preemptionRetriesBeforeTermination,omitempty"`
}

// ServiceJob represents a Batch service job. Service jobs are submitted
// directly to a job queue (of type SAGEMAKER_TRAINING), not to a
// "ServiceEnvironment" reference on the job itself -- the service environment
// association lives on the JobQueue's ServiceEnvironmentOrder instead (see
// aws-sdk-go-v2/service/batch's SubmitServiceJobInput, which has no
// ServiceEnvironment field at all).
type ServiceJob struct {
	Tags                    map[string]string                  `json:"tags"`
	RetryStrategy           *ServiceJobRetryStrategy           `json:"retryStrategy,omitempty"`
	TimeoutConfig           *ServiceJobTimeout                 `json:"timeoutConfig,omitempty"`
	PreemptionConfiguration *ServiceJobPreemptionConfiguration `json:"preemptionConfiguration,omitempty"`
	StartedAt               *int64                             `json:"startedAt,omitempty"`
	StoppedAt               *int64                             `json:"stoppedAt,omitempty"`
	ScheduledAt             *int64                             `json:"scheduledAt,omitempty"`
	// region is the store.Table composite-key qualifier (see regionKey); see
	// ComputeEnvironment.region for why it is unexported.
	region                string
	JobID                 string `json:"jobId"`
	JobArn                string `json:"jobArn"`
	JobName               string `json:"jobName"`
	JobQueue              string `json:"jobQueue"`
	ServiceJobType        string `json:"serviceJobType"`
	Status                string `json:"status"`
	StatusReason          string `json:"statusReason,omitempty"`
	ServiceRequestPayload string `json:"serviceRequestPayload,omitempty"`
	ShareIdentifier       string `json:"shareIdentifier,omitempty"`
	QuotaShareName        string `json:"quotaShareName,omitempty"`
	CreatedAt             int64  `json:"createdAt"`
	SchedulingPriority    int32  `json:"schedulingPriority,omitempty"`
	IsTerminated          bool   `json:"isTerminated"`
}

// JobQueueSnapshot represents the front-of-queue state for a job queue.
type JobQueueSnapshot struct {
	FrontOfQueue *FrontOfQueue `json:"frontOfQueue,omitempty"`
}

// FrontOfQueue holds jobs at the front of a job queue. Field names and types
// mirror aws-sdk-go-v2/service/batch/types.FrontOfQueueDetail exactly:
// LastUpdatedAt (not "timestamp") as an epoch-millisecond int64 (not a
// seconds-based float64) -- a real SDK client parsing the previous shape
// would have silently dropped both fields.
type FrontOfQueue struct {
	Jobs          []FrontOfQueueJob `json:"jobs,omitempty"`
	LastUpdatedAt int64             `json:"lastUpdatedAt,omitempty"`
}

// FrontOfQueueJob represents a single job at the front of a queue.
// EarliestTimeAtPosition is an epoch-millisecond int64, matching
// aws-sdk-go-v2/service/batch/types.FrontOfQueueJobSummary (not a
// seconds-based float64).
type FrontOfQueueJob struct {
	JobArn                 string `json:"jobArn"`
	EarliestTimeAtPosition int64  `json:"earliestTimeAtPosition,omitempty"`
}
