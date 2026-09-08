package ecs

// CWLogsBackend is the subset of CloudWatch Logs operations that ECS needs to make an
// awslogs-driver container's log group/stream discoverable, wired via SetCWLogsBackend.
// Identical in shape to lambda.CWLogsBackend and firehose.CWLogsBackend (see
// services/lambda/store.go), this repo's convention for a cross-service log sink.
type CWLogsBackend interface {
	EnsureLogGroupAndStream(groupName, streamName string) error
	PutLogLines(groupName, streamName string, messages []string) error
}

// Backend defines the interface for ECS control-plane operations.
// InMemoryBackend implements this interface; alternative backends (e.g. test
// doubles or future persistence-aware backends) can implement it too.
type Backend interface {
	// Clusters

	CreateCluster(input CreateClusterInput) (*Cluster, error)
	ListClusters() ([]Cluster, error)
	DescribeClusters(clusterNames []string) ([]Cluster, []Failure, error)
	DeleteCluster(clusterName string) (*Cluster, error)

	// Task definitions

	RegisterTaskDefinition(input RegisterTaskDefinitionInput) (*TaskDefinition, error)
	DescribeTaskDefinition(family string) (*TaskDefinition, error)
	DeregisterTaskDefinition(taskDefinitionArn string) (*TaskDefinition, error)
	ListTaskDefinitions(familyPrefix string) ([]string, error)
	ListTaskDefinitionsFiltered(input ListTaskDefinitionsInput) ([]string, error)
	DeleteTaskDefinitions(taskDefinitionArns []string) ([]TaskDefinition, []Failure, error)

	// Services

	CreateService(input CreateServiceInput) (*Service, error)
	DescribeServices(cluster string, serviceNames []string) ([]Service, []Failure, error)
	UpdateService(input UpdateServiceInput) (*Service, error)
	DeleteService(cluster, serviceName string, force ...bool) (*Service, error)
	ListServices(cluster, launchType, schedulingStrategy string) ([]string, error)

	// Tasks

	RunTask(input RunTaskInput) ([]Task, error)
	DescribeTasks(cluster string, taskArns []string) ([]Task, []Failure, error)
	StopTask(cluster, taskArn, reason string) (*Task, error)
	ListTasks(cluster string) ([]string, error)
	ListTasksFiltered(input ListTasksInput) ([]string, error)

	// Container instances

	RegisterContainerInstance(cluster, ec2InstanceID string) (*ContainerInstance, error)
	DeregisterContainerInstance(
		cluster, containerInstance string,
		force bool,
	) (*ContainerInstance, error)
	DescribeContainerInstances(
		cluster string,
		containerInstances []string,
	) ([]ContainerInstance, []Failure, error)
	ListContainerInstances(cluster, status string) ([]string, error)
	UpdateContainerInstancesState(
		cluster string,
		containerInstances []string,
		status string,
	) ([]ContainerInstance, []Failure, error)

	// Task sets

	CreateTaskSet(input CreateTaskSetInput) (*TaskSet, error)
	DeleteTaskSet(cluster, service, taskSet string) (*TaskSet, error)
	DescribeTaskSets(cluster, service string, taskSets []string) ([]TaskSet, []Failure, error)
	UpdateTaskSet(cluster, service, taskSet string, scale TaskSetScale) (*TaskSet, error)
	UpdateServicePrimaryTaskSet(cluster, service, primaryTaskSet string) (*TaskSet, error)

	// ECS Exec

	ExecuteCommand(
		cluster, task, container, command string,
		interactive bool,
	) (*ExecuteCommandOutput, error)

	// Capacity providers

	CreateCapacityProvider(input CreateCapacityProviderInput) (*CapacityProvider, error)
	DeleteCapacityProvider(nameOrArn string) (*CapacityProvider, error)
	DescribeCapacityProviders(nameOrArns []string, cluster string) ([]CapacityProvider, []Failure, error)

	// Account settings

	DeleteAccountSetting(name, principalArn string) (*AccountSetting, error)
	ListAccountSettings(name, principalArn string, effectiveSettings bool) ([]AccountSetting, error)
	PutAccountSetting(name, value, principalArn string) (*AccountSetting, error)
	PutAccountSettingDefault(name, value string) (*AccountSetting, error)

	// Attributes

	DeleteAttributes(cluster string, attrs []Attribute) ([]Attribute, error)
	ListAttributes(cluster, targetType, attributeName, targetID string) ([]Attribute, error)
	PutAttributes(cluster string, attrs []Attribute) ([]Attribute, error)

	// Cluster capacity providers and settings

	PutClusterCapacityProviders(
		cluster string,
		capacityProviders []string,
		defaultCapacityProviderStrategy []CapacityProviderStrategyItem,
	) (*Cluster, error)
	UpdateClusterSettings(cluster string, settings []ClusterSetting) (*Cluster, error)

	// Container instance management

	UpdateContainerAgent(cluster, containerInstance string) (*ContainerInstance, error)

	// Cluster management

	UpdateCluster(input UpdateClusterInput) (*Cluster, error)

	// Capacity provider management

	UpdateCapacityProvider(input UpdateCapacityProviderInput) (*CapacityProvider, error)

	// Task definition families

	ListTaskDefinitionFamilies(familyPrefix, status string) ([]string, error)

	// Task placement

	StartTask(input StartTaskInput) ([]Task, []Failure, error)

	// Namespace-scoped service listing

	ListServicesByNamespace(cluster, namespace string) ([]string, error)

	// Tagging

	TagResource(resourceArn string, tags []Tag) error
	UntagResource(resourceArn string, tagKeys []string) error
	ListTagsForResource(resourceArn string) ([]Tag, error)

	// Service deployments

	DescribeServiceDeployments(
		serviceDeploymentArns []string,
	) ([]ServiceDeployment, []Failure, error)
	ListServiceDeployments(cluster, service string) ([]ServiceDeployment, error)
	StopServiceDeployment(serviceDeploymentArn string) (*ServiceDeployment, error)
	ContinueServiceDeployment(serviceDeploymentArn, hookID, action string) (*ServiceDeployment, error)

	// Express gateway services

	CreateExpressGatewayService(
		input CreateExpressGatewayServiceInput,
	) (*ExpressGatewayService, error)
	DeleteExpressGatewayService(serviceArn string) (*ExpressGatewayService, error)
	DescribeExpressGatewayService(serviceArn string) (*ExpressGatewayService, error)
	UpdateExpressGatewayService(
		input UpdateExpressGatewayServiceInput,
	) (*ExpressGatewayService, error)

	// Task protection

	GetTaskProtection(cluster string, taskArns []string) ([]TaskProtection, []Failure, error)
	UpdateTaskProtection(
		cluster string,
		taskArns []string,
		protectionEnabled bool,
		expiresInMinutes *int,
	) ([]TaskProtection, []Failure, error)

	// Daemons

	CreateDaemon(input CreateDaemonInput) (*Daemon, error)
	DeleteDaemon(daemonArn string) (*Daemon, error)
	DescribeDaemon(daemonArn string) (*Daemon, error)
	UpdateDaemon(input UpdateDaemonInput) (*Daemon, error)
	ListDaemons(input ListDaemonsInput) ([]Daemon, error)
	DescribeDaemonDeployments(arns []string) ([]DaemonDeployment, []Failure, error)
	ListDaemonDeployments(input ListDaemonDeploymentsInput) ([]DaemonDeployment, error)
	DescribeDaemonRevisions(arns []string) ([]DaemonRevision, []Failure, error)

	// Daemon task definitions

	RegisterDaemonTaskDefinition(input RegisterDaemonTaskDefinitionInput) (*DaemonTaskDefinition, error)
	DescribeDaemonTaskDefinition(familyOrArn string) (*DaemonTaskDefinition, error)
	DeleteDaemonTaskDefinition(familyOrArn string) (*DaemonTaskDefinition, error)
	ListDaemonTaskDefinitions(input ListDaemonTaskDefinitionsInput) ([]DaemonTaskDefinition, error)

	// Service revisions

	DescribeServiceRevisions(arns []string) ([]ServiceRevision, []Failure)

	// Agent discovery

	DiscoverPollEndpoint() (endpoint, serviceConnectEndpoint, telemetryEndpoint string)

	// Agent state-change submissions

	SubmitTaskStateChange(input SubmitTaskStateChangeInput) error
	SubmitContainerStateChange(input SubmitContainerStateChangeInput) error
	SubmitAttachmentStateChanges(cluster string, attachments []AttachmentStateChange) error

	// Region metadata

	GetRegion() string
}
