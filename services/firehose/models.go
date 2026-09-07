package firehose

import (
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// maxRecordBytes is the maximum size of a single Firehose record (1,000 KB).
const maxRecordBytes = 1_000 * 1024

// maxBatchRecords is the maximum number of records allowed in a single PutRecordBatch call.
const maxBatchRecords = 500

// maxBatchBytes is the AWS Firehose limit on the combined payload of a
// PutRecordBatch request (4 MiB).
const maxBatchBytes = 4 * 1024 * 1024

const (
	// deliveryStreamTypeDirectPut is the default stream type for direct-put streams.
	deliveryStreamTypeDirectPut     = "DirectPut"
	deliveryStreamTypeKinesisSource = "KinesisStreamAsSource"
)

// BufferingHints controls when buffered records are delivered to S3.
type BufferingHints struct {
	SizeInMBs         int `json:"SizeInMBs"`
	IntervalInSeconds int `json:"IntervalInSeconds"`
}

// ProcessorParameter is a key-value parameter for a processor.
type ProcessorParameter struct {
	ParameterName  string `json:"ParameterName"`
	ParameterValue string `json:"ParameterValue"`
}

// Processor describes a single transformation step.
type Processor struct {
	Type       string               `json:"Type"`
	Parameters []ProcessorParameter `json:"Parameters,omitempty"`
}

// ProcessingConfiguration describes Lambda-based transformation.
type ProcessingConfiguration struct {
	Processors []Processor `json:"Processors,omitempty"`
	Enabled    bool        `json:"Enabled"`
}

// CloudWatchLoggingOptions configures CloudWatch logging for a destination.
type CloudWatchLoggingOptions struct {
	LogGroupName  string `json:"LogGroupName,omitempty"`
	LogStreamName string `json:"LogStreamName,omitempty"`
	Enabled       bool   `json:"Enabled"`
}

// KMSEncryptionConfig holds a KMS key ARN for S3 encryption.
type KMSEncryptionConfig struct {
	AWSKMSKeyARN string `json:"AWSKMSKeyARN"`
}

// S3EncryptionConfiguration holds the S3 object encryption config.
type S3EncryptionConfiguration struct {
	KMSEncryptionConfig *KMSEncryptionConfig `json:"KMSEncryptionConfig,omitempty"`
	NoEncryptionConfig  string               `json:"NoEncryptionConfig,omitempty"`
}

// DynamicPartitioningConfiguration controls dynamic partitioning.
type DynamicPartitioningConfiguration struct {
	RetryOptions *RetryOptions `json:"RetryOptions,omitempty"`
	Enabled      bool          `json:"Enabled"`
}

// RetryOptions holds a retry duration.
type RetryOptions struct {
	DurationInSeconds int `json:"DurationInSeconds"`
}

// EncryptionConfigInput holds the optional SSE configuration for a delivery stream.
type EncryptionConfigInput struct {
	KeyARN  string `json:"KeyARN,omitempty"`
	KeyType string `json:"KeyType"`
}

// EncryptionConfig holds the effective SSE configuration for a delivery stream.
type EncryptionConfig struct {
	FailureDescription *FailureDescription `json:"FailureDescription,omitempty"`
	KeyARN             string              `json:"KeyARN,omitempty"`
	KeyType            string              `json:"KeyType"`
	Status             string              `json:"Status"`
}

// FailureDescription holds error context for SSE failures.
type FailureDescription struct {
	Details string `json:"Details,omitempty"`
	Type    string `json:"Type,omitempty"`
}

// S3DestinationDescription holds the effective S3 destination config stored on the stream.
type S3DestinationDescription struct {
	BufferingHints                   *BufferingHints                   `json:"BufferingHints,omitempty"`
	ProcessingConfiguration          *ProcessingConfiguration          `json:"ProcessingConfiguration,omitempty"`
	S3BackupDescription              *S3BackupDescription              `json:"S3BackupDescription,omitempty"`
	EncryptionConfiguration          *S3EncryptionConfiguration        `json:"EncryptionConfiguration,omitempty"`
	CloudWatchLoggingOptions         *CloudWatchLoggingOptions         `json:"CloudWatchLoggingOptions,omitempty"`
	DynamicPartitioningConfiguration *DynamicPartitioningConfiguration `json:"DynamicPartitioningConfiguration,omitempty"`
	DataFormatConversion             *DataFormatConversionConfig       `json:"DataFormatConversionConfiguration,omitempty"`
	BucketARN                        string                            `json:"BucketARN"`
	RoleARN                          string                            `json:"RoleARN"`
	Prefix                           string                            `json:"Prefix,omitempty"`
	ErrorOutputPrefix                string                            `json:"ErrorOutputPrefix,omitempty"`
	CompressionFormat                string                            `json:"CompressionFormat,omitempty"`
	FileExtension                    string                            `json:"FileExtension,omitempty"`
	CustomTimeZone                   string                            `json:"CustomTimeZone,omitempty"`
	DestinationID                    string                            `json:"DestinationId,omitempty"`
	S3BackupMode                     string                            `json:"S3BackupMode,omitempty"`
}

// S3BackupDescription holds the S3 backup destination configuration. The real SDK
// reuses S3DestinationDescription itself for this field (types.go:1575,2621), so its
// required set -- BucketARN/BufferingHints/CompressionFormat/EncryptionConfiguration/
// RoleARN -- applies here too.
type S3BackupDescription struct {
	BufferingHints           *BufferingHints            `json:"BufferingHints,omitempty"`
	EncryptionConfiguration  *S3EncryptionConfiguration `json:"EncryptionConfiguration,omitempty"`
	CloudWatchLoggingOptions *CloudWatchLoggingOptions  `json:"CloudWatchLoggingOptions,omitempty"`
	BucketARN                string                     `json:"BucketARN"`
	RoleARN                  string                     `json:"RoleARN"`
	Prefix                   string                     `json:"Prefix,omitempty"`
	ErrorOutputPrefix        string                     `json:"ErrorOutputPrefix,omitempty"`
	CompressionFormat        string                     `json:"CompressionFormat,omitempty"`
}

// HTTPEndpointRequestConfiguration holds the content-encoding and attributes for HTTP requests.
type HTTPEndpointRequestConfiguration struct {
	ContentEncoding  string                        `json:"ContentEncoding,omitempty"`
	CommonAttributes []HTTPEndpointCommonAttribute `json:"CommonAttributes,omitempty"`
}

// HTTPEndpointCommonAttribute is a key-value attribute sent with HTTP requests.
type HTTPEndpointCommonAttribute struct {
	AttributeName  string `json:"AttributeName"`
	AttributeValue string `json:"AttributeValue"`
}

// HTTPEndpointDestinationDescription holds the HTTP endpoint destination config.
type HTTPEndpointDestinationDescription struct {
	ProcessingConfiguration  *ProcessingConfiguration          `json:"ProcessingConfiguration,omitempty"`
	EndpointConfiguration    *HTTPEndpointConfiguration        `json:"EndpointConfiguration,omitempty"`
	RequestConfiguration     *HTTPEndpointRequestConfiguration `json:"RequestConfiguration,omitempty"`
	BufferingHints           *BufferingHints                   `json:"BufferingHints,omitempty"`
	RetryOptions             *RetryOptions                     `json:"RetryOptions,omitempty"`
	CloudWatchLoggingOptions *CloudWatchLoggingOptions         `json:"CloudWatchLoggingOptions,omitempty"`
	S3BackupMode             string                            `json:"S3BackupMode,omitempty"`
	// S3BackupDescription is HttpEndpoint's single S3 bucket (used only as the
	// backup/failed-data sink); its wire key is "S3DestinationDescription", not
	// "S3BackupDescription", confirmed via
	// awsAwsjson11_deserializeDocumentHttpEndpointDestinationDescription.
	S3BackupDescription *S3BackupDescription `json:"S3DestinationDescription,omitempty"`
	DestinationID       string               `json:"DestinationId,omitempty"`
}

// HTTPEndpointConfiguration holds the HTTP endpoint URL and name.
type HTTPEndpointConfiguration struct {
	URL       string `json:"Url,omitempty"`
	Name      string `json:"Name,omitempty"`
	AccessKey string `json:"AccessKey,omitempty"`
}

// KinesisStreamSourceDescription describes a Kinesis stream source.
type KinesisStreamSourceDescription struct {
	DeliveryStartTimestamp string `json:"DeliveryStartTimestamp,omitempty"`
	KinesisStreamARN       string `json:"KinesisStreamARN,omitempty"`
	RoleARN                string `json:"RoleARN,omitempty"`
}

// MSKSourceDescription describes an MSK cluster source. ReadFromTimestamp is
// float64 (epoch seconds), not string: both serializers.go (request) and
// deserializers.go (response) encode/decode it as a JSON number via
// FormatEpochSeconds/ParseEpochSeconds, so a string here breaks CreateDeliveryStream's
// request decode outright for any real client that sets it, and would
// equally break DescribeDeliveryStream's response decode once only the
// request side were fixed.
type MSKSourceDescription struct {
	AuthenticationConfiguration *MSKAuthenticationConfiguration `json:"AuthenticationConfiguration,omitempty"`
	MSKClusterARN               string                          `json:"MSKClusterARN,omitempty"`
	TopicName                   string                          `json:"TopicName,omitempty"`
	ReadFromTimestamp           float64                         `json:"ReadFromTimestamp,omitempty"`
}

// MSKAuthenticationConfiguration holds MSK connectivity and role config.
type MSKAuthenticationConfiguration struct {
	Connectivity string `json:"Connectivity,omitempty"`
	RoleARN      string `json:"RoleARN,omitempty"`
}

// SourceDescription holds source details for non-DirectPut streams.
type SourceDescription struct {
	KinesisStreamSourceDescription *KinesisStreamSourceDescription `json:"KinesisStreamSourceDescription,omitempty"`
	MSKSourceDescription           *MSKSourceDescription           `json:"MSKSourceDescription,omitempty"`
	DatabaseSourceDescription      *DatabaseSourceDescription      `json:"DatabaseSourceDescription,omitempty"`  //nolint:lll // AWS field name
	DirectPutSourceDescription     *DirectPutSourceDescription     `json:"DirectPutSourceDescription,omitempty"` //nolint:lll // AWS field name
}

// DatabaseSourceAuthenticationConfiguration holds how Firehose authenticates to a
// database source's secret.
type DatabaseSourceAuthenticationConfiguration struct {
	SecretsManagerConfiguration *SecretsManagerConfiguration `json:"SecretsManagerConfiguration,omitempty"`
}

// DatabaseSourceVPCConfiguration holds the VPC endpoint service used to reach a
// database source.
type DatabaseSourceVPCConfiguration struct {
	VPCEndpointServiceName string `json:"VpcEndpointServiceName,omitempty"`
}

// DatabaseIncludeExcludeList is the shared Include/Exclude pattern-list shape used by
// DatabaseSourceDescription's Databases/Tables/Columns members.
type DatabaseIncludeExcludeList struct {
	Include []string `json:"Include,omitempty"`
	Exclude []string `json:"Exclude,omitempty"`
}

// DatabaseSnapshotInfo describes one table's snapshot progress. Always empty in this
// backend: no database-source snapshot mechanics are modeled (same documented-
// simplification pattern as the MSK/Redshift mechanics gaps).
type DatabaseSnapshotInfo struct {
	FailureDescription *FailureDescription `json:"FailureDescription,omitempty"`
	ID                 string              `json:"Id,omitempty"`
	Table              string              `json:"Table,omitempty"`
	RequestedBy        string              `json:"RequestedBy,omitempty"`
	Status             string              `json:"Status,omitempty"`
	RequestTimestamp   int64               `json:"RequestTimestamp,omitempty"`
}

// DatabaseSourceDescription describes a database source
// (aws-sdk-go-v2/service/firehose types.DatabaseSourceDescription -- preview API).
type DatabaseSourceDescription struct {
	DatabaseSourceAuthenticationConfiguration *DatabaseSourceAuthenticationConfiguration `json:"DatabaseSourceAuthenticationConfiguration,omitempty"` //nolint:lll // AWS field name
	DatabaseSourceVPCConfiguration            *DatabaseSourceVPCConfiguration            `json:"DatabaseSourceVPCConfiguration,omitempty"`            //nolint:lll // AWS field name
	Databases                                 *DatabaseIncludeExcludeList                `json:"Databases,omitempty"`
	Tables                                    *DatabaseIncludeExcludeList                `json:"Tables,omitempty"`
	Columns                                   *DatabaseIncludeExcludeList                `json:"Columns,omitempty"`
	Endpoint                                  string                                     `json:"Endpoint,omitempty"`
	SnapshotWatermarkTable                    string                                     `json:"SnapshotWatermarkTable,omitempty"` //nolint:lll // AWS field name
	SSLMode                                   string                                     `json:"SSLMode,omitempty"`
	Type                                      string                                     `json:"Type,omitempty"`
	SnapshotInfo                              []DatabaseSnapshotInfo                     `json:"SnapshotInfo,omitempty"`
	SurrogateKeys                             []string                                   `json:"SurrogateKeys,omitempty"`
	Port                                      int32                                      `json:"Port,omitempty"`
}

// DirectPutSourceDescription describes a Direct PUT source.
type DirectPutSourceDescription struct {
	ThroughputHintInMBs int32 `json:"ThroughputHintInMBs,omitempty"`
}

// RedshiftCopyCommand holds the Redshift COPY command configuration. On the wire this
// nests under RedshiftDestinationDescription.CopyCommand (and, on the request side,
// RedshiftDestinationConfiguration.CopyCommand) rather than as flat fields.
type RedshiftCopyCommand struct {
	DataTableName    string `json:"DataTableName"`
	DataTableColumns string `json:"DataTableColumns,omitempty"`
	CopyOptions      string `json:"CopyOptions,omitempty"`
}

// RedshiftDestinationDescription holds a Redshift destination config.
type RedshiftDestinationDescription struct {
	ProcessingConfiguration  *ProcessingConfiguration  `json:"ProcessingConfiguration,omitempty"`
	RetryOptions             *RetryOptions             `json:"RetryOptions,omitempty"`
	S3BackupDescription      *S3BackupDescription      `json:"S3BackupDescription,omitempty"`
	CloudWatchLoggingOptions *CloudWatchLoggingOptions `json:"CloudWatchLoggingOptions,omitempty"`
	// S3Destination is the required intermediate S3 staging location that Amazon
	// Redshift's COPY command reads from (wire field "S3DestinationDescription").
	S3Destination               *S3DestinationDescription    `json:"S3DestinationDescription,omitempty"`
	CopyCommand                 *RedshiftCopyCommand         `json:"CopyCommand,omitempty"`
	SecretsManagerConfiguration *SecretsManagerConfiguration `json:"SecretsManagerConfiguration,omitempty"`
	ClusterJDBCURL              string                       `json:"ClusterJDBCURL,omitempty"`
	Username                    string                       `json:"Username,omitempty"`
	RoleARN                     string                       `json:"RoleARN,omitempty"`
	S3BackupMode                string                       `json:"S3BackupMode,omitempty"`
	DestinationID               string                       `json:"DestinationId,omitempty"`
}

// OpenSearchDestinationDescription holds an OpenSearch (Elasticsearch) destination config.
type OpenSearchDestinationDescription struct {
	ProcessingConfiguration  *ProcessingConfiguration  `json:"ProcessingConfiguration,omitempty"`
	BufferingHints           *BufferingHints           `json:"BufferingHints,omitempty"`
	RetryOptions             *RetryOptions             `json:"RetryOptions,omitempty"`
	CloudWatchLoggingOptions *CloudWatchLoggingOptions `json:"CloudWatchLoggingOptions,omitempty"`
	// S3BackupDescription is OpenSearch's single S3 bucket (used only as the
	// backup/failed-document sink); its wire key is "S3DestinationDescription", not
	// "S3BackupDescription", confirmed via
	// awsAwsjson11_deserializeDocumentAmazonopensearchserviceDestinationDescription.
	S3BackupDescription *S3BackupDescription `json:"S3DestinationDescription,omitempty"`
	DomainARN           string               `json:"DomainARN,omitempty"`
	ClusterEndpoint     string               `json:"ClusterEndpoint,omitempty"`
	IndexName           string               `json:"IndexName,omitempty"`
	TypeName            string               `json:"TypeName,omitempty"`
	IndexRotationPeriod string               `json:"IndexRotationPeriod,omitempty"`
	S3BackupMode        string               `json:"S3BackupMode,omitempty"`
	RoleARN             string               `json:"RoleARN,omitempty"`
	DestinationID       string               `json:"DestinationId,omitempty"`
}

// ElasticsearchDestinationDescription holds a legacy (pre-OpenSearch-rename) Elasticsearch
// destination config. AWS still exposes this as a distinct API shape
// (ElasticsearchDestinationConfiguration/-Update/-Description) alongside the newer
// AmazonopensearchserviceDestinationConfiguration family; the two are wire-distinct even
// though the field sets are nearly identical.
type ElasticsearchDestinationDescription struct {
	ProcessingConfiguration  *ProcessingConfiguration  `json:"ProcessingConfiguration,omitempty"`
	BufferingHints           *BufferingHints           `json:"BufferingHints,omitempty"`
	RetryOptions             *RetryOptions             `json:"RetryOptions,omitempty"`
	CloudWatchLoggingOptions *CloudWatchLoggingOptions `json:"CloudWatchLoggingOptions,omitempty"`
	// S3BackupDescription is Elasticsearch's single S3 bucket (used only as the
	// backup/failed-document sink); its wire key is "S3DestinationDescription", not
	// "S3BackupDescription", confirmed via
	// awsAwsjson11_deserializeDocumentElasticsearchDestinationDescription.
	S3BackupDescription *S3BackupDescription `json:"S3DestinationDescription,omitempty"`
	DomainARN           string               `json:"DomainARN,omitempty"`
	ClusterEndpoint     string               `json:"ClusterEndpoint,omitempty"`
	IndexName           string               `json:"IndexName,omitempty"`
	TypeName            string               `json:"TypeName,omitempty"`
	IndexRotationPeriod string               `json:"IndexRotationPeriod,omitempty"`
	S3BackupMode        string               `json:"S3BackupMode,omitempty"`
	RoleARN             string               `json:"RoleARN,omitempty"`
	DestinationID       string               `json:"DestinationId,omitempty"`
}

// SplunkDestinationDescription holds a Splunk HEC destination config.
type SplunkDestinationDescription struct {
	ProcessingConfiguration  *ProcessingConfiguration  `json:"ProcessingConfiguration,omitempty"`
	RetryOptions             *RetryOptions             `json:"RetryOptions,omitempty"`
	CloudWatchLoggingOptions *CloudWatchLoggingOptions `json:"CloudWatchLoggingOptions,omitempty"`
	// S3BackupDescription is Splunk's single S3 bucket (used only as the
	// backup/failed-event sink); its wire key is "S3DestinationDescription", not
	// "S3BackupDescription", confirmed via
	// awsAwsjson11_deserializeDocumentSplunkDestinationDescription.
	S3BackupDescription               *S3BackupDescription `json:"S3DestinationDescription,omitempty"`
	HECEndpoint                       string               `json:"HECEndpoint,omitempty"`
	HECEndpointType                   string               `json:"HECEndpointType,omitempty"`
	HECToken                          string               `json:"HECToken,omitempty"`
	S3BackupMode                      string               `json:"S3BackupMode,omitempty"`
	DestinationID                     string               `json:"DestinationId,omitempty"`
	HECAcknowledgmentTimeoutInSeconds int                  `json:"HECAcknowledgmentTimeoutInSeconds,omitempty"`
}

// CatalogConfiguration describes where destination Apache Iceberg tables are persisted.
type CatalogConfiguration struct {
	CatalogARN        string `json:"CatalogARN,omitempty"`
	WarehouseLocation string `json:"WarehouseLocation,omitempty"`
}

// PartitionField is a single identity-transform partition column for an Iceberg table.
type PartitionField struct {
	SourceName string `json:"SourceName"`
}

// PartitionSpec holds the partition-spec configuration used by automatic table creation.
type PartitionSpec struct {
	Identity []PartitionField `json:"Identity,omitempty"`
}

// DestinationTableConfiguration configures delivery to a single Apache Iceberg table.
type DestinationTableConfiguration struct {
	PartitionSpec           *PartitionSpec `json:"PartitionSpec,omitempty"`
	DestinationDatabaseName string         `json:"DestinationDatabaseName"`
	DestinationTableName    string         `json:"DestinationTableName"`
	S3ErrorOutputPrefix     string         `json:"S3ErrorOutputPrefix,omitempty"`
	UniqueKeys              []string       `json:"UniqueKeys,omitempty"`
}

// SchemaEvolutionConfiguration toggles automatic schema evolution for Iceberg delivery.
type SchemaEvolutionConfiguration struct {
	Enabled bool `json:"Enabled"`
}

// TableCreationConfiguration toggles automatic table creation for Iceberg delivery.
type TableCreationConfiguration struct {
	Enabled bool `json:"Enabled"`
}

// IcebergDestinationDescription holds an Apache Iceberg Tables destination config.
type IcebergDestinationDescription struct {
	SchemaEvolutionConfiguration      *SchemaEvolutionConfiguration   `json:"SchemaEvolutionConfiguration,omitempty"`
	CatalogConfiguration              *CatalogConfiguration           `json:"CatalogConfiguration,omitempty"`
	CloudWatchLoggingOptions          *CloudWatchLoggingOptions       `json:"CloudWatchLoggingOptions,omitempty"`
	ProcessingConfiguration           *ProcessingConfiguration        `json:"ProcessingConfiguration,omitempty"`
	RetryOptions                      *RetryOptions                   `json:"RetryOptions,omitempty"`
	S3Destination                     *S3DestinationDescription       `json:"S3DestinationDescription,omitempty"`
	BufferingHints                    *BufferingHints                 `json:"BufferingHints,omitempty"`
	TableCreationConfiguration        *TableCreationConfiguration     `json:"TableCreationConfiguration,omitempty"`
	RoleARN                           string                          `json:"RoleARN,omitempty"`
	S3BackupMode                      string                          `json:"S3BackupMode,omitempty"`
	DestinationID                     string                          `json:"DestinationId,omitempty"`
	DestinationTableConfigurationList []DestinationTableConfiguration `json:"DestinationTableConfigurationList,omitempty"`
	AppendOnly                        bool                            `json:"AppendOnly,omitempty"`
}

// SnowflakeBufferingHints controls when buffered records are delivered to Snowflake.
type SnowflakeBufferingHints struct {
	IntervalInSeconds int `json:"IntervalInSeconds,omitempty"`
	SizeInMBs         int `json:"SizeInMBs,omitempty"`
}

// SnowflakeRetryOptions holds a retry duration for Snowflake delivery.
type SnowflakeRetryOptions struct {
	DurationInSeconds int `json:"DurationInSeconds,omitempty"`
}

// SecretsManagerConfiguration describes how Firehose accesses secrets for a destination.
type SecretsManagerConfiguration struct {
	RoleARN   string `json:"RoleARN,omitempty"`
	SecretARN string `json:"SecretARN,omitempty"`
	Enabled   bool   `json:"Enabled"`
}

// SnowflakeRoleConfiguration optionally configures a Snowflake role.
type SnowflakeRoleConfiguration struct {
	SnowflakeRole string `json:"SnowflakeRole,omitempty"`
	Enabled       bool   `json:"Enabled"`
}

// SnowflakeVpcConfiguration holds the PrivateLink VPCE ID for private Snowflake connectivity.
type SnowflakeVpcConfiguration struct {
	PrivateLinkVpceID string `json:"PrivateLinkVpceId"`
}

// SnowflakeDestinationDescription holds a Snowflake destination config.
type SnowflakeDestinationDescription struct {
	BufferingHints           *SnowflakeBufferingHints  `json:"BufferingHints,omitempty"`
	CloudWatchLoggingOptions *CloudWatchLoggingOptions `json:"CloudWatchLoggingOptions,omitempty"`
	ProcessingConfiguration  *ProcessingConfiguration  `json:"ProcessingConfiguration,omitempty"`
	RetryOptions             *SnowflakeRetryOptions    `json:"RetryOptions,omitempty"`
	// S3Destination is the required S3 location Snowflake delivery stages through
	// (wire field "S3DestinationDescription").
	S3Destination               *S3DestinationDescription    `json:"S3DestinationDescription,omitempty"`
	SecretsManagerConfiguration *SecretsManagerConfiguration `json:"SecretsManagerConfiguration,omitempty"`
	SnowflakeRoleConfiguration  *SnowflakeRoleConfiguration  `json:"SnowflakeRoleConfiguration,omitempty"`
	SnowflakeVpcConfiguration   *SnowflakeVpcConfiguration   `json:"SnowflakeVpcConfiguration,omitempty"`
	AccountURL                  string                       `json:"AccountUrl,omitempty"`
	ContentColumnName           string                       `json:"ContentColumnName,omitempty"`
	DataLoadingOption           string                       `json:"DataLoadingOption,omitempty"`
	Database                    string                       `json:"Database,omitempty"`
	MetaDataColumnName          string                       `json:"MetaDataColumnName,omitempty"`
	RoleARN                     string                       `json:"RoleARN,omitempty"`
	S3BackupMode                string                       `json:"S3BackupMode,omitempty"`
	Schema                      string                       `json:"Schema,omitempty"`
	Table                       string                       `json:"Table,omitempty"`
	User                        string                       `json:"User,omitempty"`
	DestinationID               string                       `json:"DestinationId,omitempty"`
}

// DeliveryMetrics tracks delivery statistics for a stream.
type DeliveryMetrics struct {
	TotalRecords  int64 `json:"TotalRecords"`
	FailedRecords int64 `json:"FailedRecords"`
	TotalBytes    int64 `json:"TotalBytes"`
}

// DeliveryStream represents a Kinesis Firehose delivery stream.
type DeliveryStream struct {
	lastFlush                time.Time
	CreateTimestamp          time.Time                            `json:"createTimestamp"`
	LastUpdateTimestamp      time.Time                            `json:"lastUpdateTimestamp"`
	Tags                     *tags.Tags                           `json:"tags,omitempty"`
	S3Destination            *S3DestinationDescription            `json:"s3Destination,omitempty"`
	HTTPEndpointDestination  *HTTPEndpointDestinationDescription  `json:"httpEndpointDestination,omitempty"`
	RedshiftDestination      *RedshiftDestinationDescription      `json:"redshiftDestination,omitempty"`
	OpenSearchDestination    *OpenSearchDestinationDescription    `json:"openSearchDestination,omitempty"`
	ElasticsearchDestination *ElasticsearchDestinationDescription `json:"elasticsearchDestination,omitempty"`
	SplunkDestination        *SplunkDestinationDescription        `json:"splunkDestination,omitempty"`
	SnowflakeDestination     *SnowflakeDestinationDescription     `json:"snowflakeDestination,omitempty"`
	IcebergDestination       *IcebergDestinationDescription       `json:"icebergDestination,omitempty"`
	Encryption               *EncryptionConfig                    `json:"encryption,omitempty"`
	Source                   *SourceDescription                   `json:"source,omitempty"`
	DeliveryStreamType       string                               `json:"deliveryStreamType,omitempty"`
	Name                     string                               `json:"name"`
	ARN                      string                               `json:"arn"`
	VersionID                string                               `json:"versionID,omitempty"`
	Status                   string                               `json:"status"`
	AccountID                string                               `json:"accountID"`
	Region                   string                               `json:"region"`
	Records                  [][]byte                             `json:"records,omitempty"`
	BackupRecords            [][]byte                             `json:"backupRecords,omitempty"`
	Metrics                  DeliveryMetrics                      `json:"metrics"`
	bufferSizeBytes          int
}
