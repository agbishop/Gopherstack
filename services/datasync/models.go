package datasync

import (
	"maps"
	"time"
)

// storedAgent holds an agent with all fields.
// CreationTime is first so its non-pointer prefix (wall, ext) reduces GC pointer bytes.
type storedAgent struct {
	CreationTime time.Time         `json:"creationTime"`
	Tags         map[string]string `json:"tags"`
	AgentArn     string            `json:"agentArn"`
	Name         string            `json:"name"`
	Status       string            `json:"status"`
	EndpointType string            `json:"endpointType"`
}

func (a *storedAgent) toAgent() Agent {
	return Agent{
		AgentArn:     a.AgentArn,
		Name:         a.Name,
		Status:       a.Status,
		EndpointType: a.EndpointType,
		CreationTime: a.CreationTime,
		Tags:         a.Tags,
	}
}

// storedLocation holds a location with all fields.
// CreationTime is first so its non-pointer prefix (wall, ext) reduces GC pointer bytes.
type storedLocation struct {
	CreationTime   time.Time                  `json:"creationTime"`
	S3Config       *storedS3Config            `json:"s3Config,omitempty"`
	AzureBlob      *storedAzureBlobConfig     `json:"azureBlob,omitempty"`
	Efs            *storedEfsConfig           `json:"efs,omitempty"`
	FsxLustre      *storedFsxLustreConfig     `json:"fsxLustre,omitempty"`
	FsxOntap       *storedFsxOntapConfig      `json:"fsxOntap,omitempty"`
	FsxOpenZfs     *storedFsxOpenZfsConfig    `json:"fsxOpenZfs,omitempty"`
	FsxWindows     *storedFsxWindowsConfig    `json:"fsxWindows,omitempty"`
	Hdfs           *storedHdfsConfig          `json:"hdfs,omitempty"`
	Nfs            *storedNfsConfig           `json:"nfs,omitempty"`
	ObjectStorage  *storedObjectStorageConfig `json:"objectStorage,omitempty"`
	Smb            *storedSmbConfig           `json:"smb,omitempty"`
	Tags           map[string]string          `json:"tags"`
	LocationArn    string                     `json:"locationArn"`
	LocationURI    string                     `json:"locationUri"`
	S3BucketArn    string                     `json:"s3BucketArn,omitempty"`
	Subdirectory   string                     `json:"subdirectory,omitempty"`
	S3StorageClass string                     `json:"s3StorageClass,omitempty"`
	LocationType   string                     `json:"locationType"`
}

type storedS3Config struct {
	BucketAccessRoleArn string   `json:"bucketAccessRoleArn"`
	AgentArns           []string `json:"agentArns,omitempty"`
}

// storedCmkSecretConfig mirrors CmkSecretConfig for JSON persistence.
type storedCmkSecretConfig struct {
	SecretArn string `json:"secretArn,omitempty"`
	KmsKeyArn string `json:"kmsKeyArn,omitempty"`
}

// storedCustomSecretConfig mirrors CustomSecretConfig for JSON persistence.
type storedCustomSecretConfig struct {
	SecretArn           string `json:"secretArn,omitempty"`
	SecretAccessRoleArn string `json:"secretAccessRoleArn,omitempty"`
}

func toStoredCmkSecretConfig(c *CmkSecretConfig) *storedCmkSecretConfig {
	if c == nil {
		return nil
	}

	return &storedCmkSecretConfig{SecretArn: c.SecretArn, KmsKeyArn: c.KmsKeyArn}
}

func fromStoredCmkSecretConfig(c *storedCmkSecretConfig) *CmkSecretConfig {
	if c == nil {
		return nil
	}

	return &CmkSecretConfig{SecretArn: c.SecretArn, KmsKeyArn: c.KmsKeyArn}
}

func toStoredCustomSecretConfig(c *CustomSecretConfig) *storedCustomSecretConfig {
	if c == nil {
		return nil
	}

	return &storedCustomSecretConfig{SecretArn: c.SecretArn, SecretAccessRoleArn: c.SecretAccessRoleArn}
}

func fromStoredCustomSecretConfig(c *storedCustomSecretConfig) *CustomSecretConfig {
	if c == nil {
		return nil
	}

	return &CustomSecretConfig{SecretArn: c.SecretArn, SecretAccessRoleArn: c.SecretAccessRoleArn}
}

// --- Type-specific location config stored types ---

type storedAzureBlobConfig struct {
	CmkSecretConfig    *storedCmkSecretConfig    `json:"cmkSecretConfig,omitempty"`
	CustomSecretConfig *storedCustomSecretConfig `json:"customSecretConfig,omitempty"`
	SasToken           string                    `json:"sasToken,omitempty"`
	ContainerURL       string                    `json:"containerUrl"`
	BlobType           string                    `json:"blobType,omitempty"`
	AccessTier         string                    `json:"accessTier,omitempty"`
	AuthenticationType string                    `json:"authenticationType,omitempty"`
	AgentArns          []string                  `json:"agentArns,omitempty"`
}

type storedEfsEc2Config struct {
	SubnetArn         string   `json:"subnetArn"`
	SecurityGroupArns []string `json:"securityGroupArns"`
}

type storedEfsConfig struct {
	Ec2Config               *storedEfsEc2Config `json:"ec2Config,omitempty"`
	EfsFilesystemArn        string              `json:"efsFilesystemArn"`
	AccessPointArn          string              `json:"accessPointArn,omitempty"`
	FileSystemAccessRoleArn string              `json:"fileSystemAccessRoleArn,omitempty"`
	InTransitEncryption     string              `json:"inTransitEncryption,omitempty"`
}

type storedFsxLustreConfig struct {
	FsxFilesystemArn  string   `json:"fsxFilesystemArn"`
	SecurityGroupArns []string `json:"securityGroupArns,omitempty"`
}

type storedFsxMountOptions struct {
	Version string `json:"version,omitempty"`
}

type storedFsxNfsProtocol struct {
	MountOptions *storedFsxMountOptions `json:"mountOptions,omitempty"`
}

type storedFsxSmbProtocol struct {
	MountOptions *storedFsxMountOptions `json:"mountOptions,omitempty"`
	Domain       string                 `json:"domain,omitempty"`
	Password     string                 `json:"password,omitempty"`
	User         string                 `json:"user,omitempty"`
}

type storedFsxProtocol struct {
	NFS *storedFsxNfsProtocol `json:"nfs,omitempty"`
	SMB *storedFsxSmbProtocol `json:"smb,omitempty"`
}

type storedFsxOntapConfig struct {
	Protocol                 *storedFsxProtocol `json:"protocol,omitempty"`
	StorageVirtualMachineArn string             `json:"storageVirtualMachineArn"`
	FsxFilesystemArn         string             `json:"fsxFilesystemArn,omitempty"`
	SecurityGroupArns        []string           `json:"securityGroupArns,omitempty"`
}

type storedFsxOpenZfsConfig struct {
	Protocol          *storedFsxProtocol `json:"protocol,omitempty"`
	FsxFilesystemArn  string             `json:"fsxFilesystemArn"`
	SecurityGroupArns []string           `json:"securityGroupArns,omitempty"`
}

type storedFsxWindowsConfig struct {
	CmkSecretConfig    *storedCmkSecretConfig    `json:"cmkSecretConfig,omitempty"`
	CustomSecretConfig *storedCustomSecretConfig `json:"customSecretConfig,omitempty"`
	FsxFilesystemArn   string                    `json:"fsxFilesystemArn"`
	Domain             string                    `json:"domain,omitempty"`
	User               string                    `json:"user,omitempty"`
	Password           string                    `json:"password,omitempty"`
	SecurityGroupArns  []string                  `json:"securityGroupArns,omitempty"`
}

type storedHdfsNameNode struct {
	Hostname string `json:"hostname"`
	Port     int32  `json:"port"`
}

type storedQopConfig struct {
	DataTransferProtection string `json:"dataTransferProtection,omitempty"`
	RPCProtection          string `json:"rpcProtection,omitempty"`
}

type storedHdfsConfig struct {
	QopConfiguration   *storedQopConfig          `json:"qopConfiguration,omitempty"`
	CmkSecretConfig    *storedCmkSecretConfig    `json:"cmkSecretConfig,omitempty"`
	CustomSecretConfig *storedCustomSecretConfig `json:"customSecretConfig,omitempty"`
	KerberosPrincipal  string                    `json:"kerberosPrincipal,omitempty"`
	KerberosKeytab     string                    `json:"kerberosKeytab,omitempty"`
	KerberosKrb5Conf   string                    `json:"kerberosKrb5Conf,omitempty"`
	KmsKeyProviderURI  string                    `json:"kmsKeyProviderUri,omitempty"`
	AuthenticationType string                    `json:"authenticationType,omitempty"`
	SimpleUser         string                    `json:"simpleUser,omitempty"`
	NameNodes          []storedHdfsNameNode      `json:"nameNodes"`
	AgentArns          []string                  `json:"agentArns,omitempty"`
	BlockSize          int64                     `json:"blockSize,omitempty"`
	ReplicationFactor  int32                     `json:"replicationFactor,omitempty"`
}

type storedMountOptions struct {
	Version string `json:"version,omitempty"`
}

type storedNfsConfig struct {
	MountOptions   *storedMountOptions `json:"mountOptions,omitempty"`
	ServerHostname string              `json:"serverHostname"`
	AgentArns      []string            `json:"agentArns,omitempty"`
}

type storedObjectStorageConfig struct {
	CmkSecretConfig    *storedCmkSecretConfig    `json:"cmkSecretConfig,omitempty"`
	CustomSecretConfig *storedCustomSecretConfig `json:"customSecretConfig,omitempty"`
	ServerHostname     string                    `json:"serverHostname"`
	BucketName         string                    `json:"bucketName"`
	AccessKey          string                    `json:"accessKey,omitempty"`
	SecretKey          string                    `json:"secretKey,omitempty"`
	ServerProtocol     string                    `json:"serverProtocol,omitempty"`
	AgentArns          []string                  `json:"agentArns,omitempty"`
	ServerPort         int32                     `json:"serverPort,omitempty"`
}

type storedSmbConfig struct {
	MountOptions       *storedMountOptions       `json:"mountOptions,omitempty"`
	CmkSecretConfig    *storedCmkSecretConfig    `json:"cmkSecretConfig,omitempty"`
	CustomSecretConfig *storedCustomSecretConfig `json:"customSecretConfig,omitempty"`
	ServerHostname     string                    `json:"serverHostname"`
	Domain             string                    `json:"domain,omitempty"`
	User               string                    `json:"user,omitempty"`
	Password           string                    `json:"password,omitempty"`
	AuthenticationType string                    `json:"authenticationType,omitempty"`
	KerberosPrincipal  string                    `json:"kerberosPrincipal,omitempty"`
	KerberosKeytab     string                    `json:"kerberosKeytab,omitempty"`
	KerberosKrb5Conf   string                    `json:"kerberosKrb5Conf,omitempty"`
	AgentArns          []string                  `json:"agentArns,omitempty"`
	DNSIPAddresses     []string                  `json:"dnsIpAddresses,omitempty"`
}

func (l *storedLocation) toLocation() Location {
	return Location{
		LocationArn:  l.LocationArn,
		LocationURI:  l.LocationURI,
		CreationTime: l.CreationTime,
	}
}

func (l *storedLocation) toLocationS3() LocationS3 {
	loc := LocationS3{
		LocationArn:    l.LocationArn,
		LocationURI:    l.LocationURI,
		S3BucketArn:    l.S3BucketArn,
		Subdirectory:   l.Subdirectory,
		S3StorageClass: l.S3StorageClass,
		CreationTime:   l.CreationTime,
	}
	if l.S3Config != nil {
		loc.S3Config = S3Config{BucketAccessRoleArn: l.S3Config.BucketAccessRoleArn}
		loc.AgentArns = l.S3Config.AgentArns
	}

	return loc
}

// storedFilterRule mirrors FilterRule for JSON persistence.
type storedFilterRule struct {
	FilterType string `json:"filterType,omitempty"`
	Value      string `json:"value,omitempty"`
}

func toStoredFilterRules(rules []FilterRule) []storedFilterRule {
	if rules == nil {
		return nil
	}

	out := make([]storedFilterRule, len(rules))
	for i, r := range rules {
		out[i] = storedFilterRule(r)
	}

	return out
}

func fromStoredFilterRules(rules []storedFilterRule) []FilterRule {
	if rules == nil {
		return nil
	}

	out := make([]FilterRule, len(rules))
	for i, r := range rules {
		out[i] = FilterRule(r)
	}

	return out
}

// storedTaskSchedule mirrors TaskSchedule for JSON persistence.
type storedTaskSchedule struct {
	ScheduleExpression string `json:"scheduleExpression"`
	Status             string `json:"status,omitempty"`
}

// storedTask holds a task with all fields.
// CreationTime is first so its non-pointer prefix (wall, ext) reduces GC pointer bytes.
type storedTask struct {
	CreationTime            time.Time           `json:"creationTime"`
	Tags                    map[string]string   `json:"tags"`
	Options                 map[string]any      `json:"options,omitempty"`
	Schedule                *storedTaskSchedule `json:"schedule,omitempty"`
	ManifestConfig          map[string]any      `json:"manifestConfig,omitempty"`
	TaskReportConfig        map[string]any      `json:"taskReportConfig,omitempty"`
	TaskArn                 string              `json:"taskArn"`
	Name                    string              `json:"name"`
	Status                  string              `json:"status"`
	SourceLocationArn       string              `json:"sourceLocationArn"`
	DestinationLocationArn  string              `json:"destinationLocationArn"`
	CloudWatchLogGroupArn   string              `json:"cloudWatchLogGroupArn,omitempty"`
	CurrentTaskExecutionArn string              `json:"currentTaskExecutionArn,omitempty"`
	TaskMode                string              `json:"taskMode,omitempty"`
	Includes                []storedFilterRule  `json:"includes,omitempty"`
	Excludes                []storedFilterRule  `json:"excludes,omitempty"`
}

func (t *storedTask) toTask() Task {
	task := Task{
		TaskArn:                 t.TaskArn,
		Name:                    t.Name,
		Status:                  t.Status,
		SourceLocationArn:       t.SourceLocationArn,
		DestinationLocationArn:  t.DestinationLocationArn,
		CloudWatchLogGroupArn:   t.CloudWatchLogGroupArn,
		CurrentTaskExecutionArn: t.CurrentTaskExecutionArn,
		CreationTime:            t.CreationTime,
		Tags:                    t.Tags,
		Options:                 maps.Clone(t.Options),
		ManifestConfig:          maps.Clone(t.ManifestConfig),
		TaskReportConfig:        maps.Clone(t.TaskReportConfig),
		Excludes:                fromStoredFilterRules(t.Excludes),
		Includes:                fromStoredFilterRules(t.Includes),
		TaskMode:                t.TaskMode,
	}

	if t.Schedule != nil {
		task.Schedule = &TaskSchedule{
			ScheduleExpression: t.Schedule.ScheduleExpression,
			Status:             t.Schedule.Status,
		}
	}

	return task
}

// storedTaskExecution holds a task execution with all fields.
// StartTime is first so its non-pointer prefix (wall, ext) reduces GC pointer bytes.
type storedTaskExecution struct {
	StartTime                time.Time          `json:"startTime"`
	Options                  map[string]any     `json:"options,omitempty"`
	ManifestConfig           map[string]any     `json:"manifestConfig,omitempty"`
	TaskReportConfig         map[string]any     `json:"taskReportConfig,omitempty"`
	TaskExecutionArn         string             `json:"taskExecutionArn"`
	Status                   string             `json:"status"`
	TaskMode                 string             `json:"taskMode,omitempty"`
	Excludes                 []storedFilterRule `json:"excludes,omitempty"`
	Includes                 []storedFilterRule `json:"includes,omitempty"`
	EstimatedFilesToTransfer int64              `json:"estimatedFilesToTransfer"`
	EstimatedBytesToTransfer int64              `json:"estimatedBytesToTransfer"`
	FilesTransferred         int64              `json:"filesTransferred"`
	BytesTransferred         int64              `json:"bytesTransferred"`
}

func (e *storedTaskExecution) toTaskExecution() TaskExecution {
	return TaskExecution{
		TaskExecutionArn:         e.TaskExecutionArn,
		Status:                   e.Status,
		TaskMode:                 e.TaskMode,
		StartTime:                e.StartTime,
		Options:                  maps.Clone(e.Options),
		ManifestConfig:           maps.Clone(e.ManifestConfig),
		TaskReportConfig:         maps.Clone(e.TaskReportConfig),
		Excludes:                 fromStoredFilterRules(e.Excludes),
		Includes:                 fromStoredFilterRules(e.Includes),
		EstimatedFilesToTransfer: e.EstimatedFilesToTransfer,
		EstimatedBytesToTransfer: e.EstimatedBytesToTransfer,
		FilesTransferred:         e.FilesTransferred,
		BytesTransferred:         e.BytesTransferred,
	}
}
