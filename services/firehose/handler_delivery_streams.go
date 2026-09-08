package firehose

import (
	"context"
	"encoding/json"
	"fmt"

	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// compressionFormatUncompressed is the real SDK's documented default CompressionFormat
// ("If no value is specified, the default is UNCOMPRESSED.").
const compressionFormatUncompressed = "UNCOMPRESSED"

// defaultS3BufferingSizeMBs and defaultS3BufferingIntervalSeconds are the real SDK's
// documented BufferingHints defaults (types.go: "The default value is 5"/"300").
const (
	defaultS3BufferingSizeMBs         = 5
	defaultS3BufferingIntervalSeconds = 300
)

// s3DestinationInput holds the S3 destination configuration from the API request.
// It maps both S3DestinationConfiguration and ExtendedS3DestinationConfiguration fields.

const maxListLimit = 10000

// s3DestinationInput holds the S3 destination configuration from the API request.
// It maps both S3DestinationConfiguration and ExtendedS3DestinationConfiguration fields.
type s3DestinationInput struct {
	BufferingHints          *BufferingHints          `json:"BufferingHints"`
	ProcessingConfiguration *ProcessingConfiguration `json:"ProcessingConfiguration"`
	// S3BackupConfiguration is the wire key on ExtendedS3DestinationConfiguration
	// (Create); S3BackupUpdate is the same field's wire key on
	// ExtendedS3DestinationUpdate (Update) -- confirmed distinct via
	// awsAwsjson11_serializeDocumentExtendedS3DestinationUpdate. A real client sends
	// exactly one, never both.
	S3BackupConfiguration             *s3BackupInput                    `json:"S3BackupConfiguration"`
	S3BackupUpdate                    *s3BackupInput                    `json:"S3BackupUpdate"`
	EncryptionConfiguration           *S3EncryptionConfiguration        `json:"EncryptionConfiguration"`
	CloudWatchLoggingOptions          *CloudWatchLoggingOptions         `json:"CloudWatchLoggingOptions"`
	DynamicPartitioningConfiguration  *DynamicPartitioningConfiguration `json:"DynamicPartitioningConfiguration"`
	DataFormatConversionConfiguration *DataFormatConversionConfig       `json:"DataFormatConversionConfiguration"`
	BucketARN                         string                            `json:"BucketARN"`
	RoleARN                           string                            `json:"RoleARN"`
	Prefix                            string                            `json:"Prefix"`
	ErrorOutputPrefix                 string                            `json:"ErrorOutputPrefix"`
	CompressionFormat                 string                            `json:"CompressionFormat"`
	FileExtension                     string                            `json:"FileExtension"`
	CustomTimeZone                    string                            `json:"CustomTimeZone"`
	S3BackupMode                      string                            `json:"S3BackupMode"`
}

// s3BackupInput holds the S3 backup destination configuration. On the wire this is a
// plain S3DestinationConfiguration/S3DestinationUpdate (aws-sdk-go-v2/service/firehose
// types.go), which carries CloudWatchLoggingOptions/EncryptionConfiguration/
// ErrorOutputPrefix in addition to the basics.
type s3BackupInput struct {
	BufferingHints           *BufferingHints            `json:"BufferingHints"`
	EncryptionConfiguration  *S3EncryptionConfiguration `json:"EncryptionConfiguration"`
	CloudWatchLoggingOptions *CloudWatchLoggingOptions  `json:"CloudWatchLoggingOptions"`
	BucketARN                string                     `json:"BucketARN"`
	RoleARN                  string                     `json:"RoleARN"`
	Prefix                   string                     `json:"Prefix"`
	ErrorOutputPrefix        string                     `json:"ErrorOutputPrefix"`
	CompressionFormat        string                     `json:"CompressionFormat"`
}

// httpEndpointDestinationInput holds HTTP endpoint destination configuration. Firehose
// only ever writes S3 for this destination as its backup sink (there is no separate
// primary S3 destination); the wire key for that single S3 bucket is "S3Configuration"
// on HttpEndpointDestinationConfiguration (Create) and "S3Update" on
// HttpEndpointDestinationUpdate (Update) -- confirmed via
// awsAwsjson11_serializeDocumentHttpEndpointDestinationConfiguration/-Update. A real
// client sends exactly one, never both.
type httpEndpointDestinationInput struct {
	EndpointConfiguration    *httpEndpointConfigurationInput   `json:"EndpointConfiguration"`
	ProcessingConfiguration  *ProcessingConfiguration          `json:"ProcessingConfiguration"`
	S3Configuration          *s3BackupInput                    `json:"S3Configuration"`
	S3Update                 *s3BackupInput                    `json:"S3Update"`
	RequestConfiguration     *HTTPEndpointRequestConfiguration `json:"RequestConfiguration"`
	BufferingHints           *BufferingHints                   `json:"BufferingHints"`
	RetryOptions             *RetryOptions                     `json:"RetryOptions"`
	CloudWatchLoggingOptions *CloudWatchLoggingOptions         `json:"CloudWatchLoggingOptions"`
	S3BackupMode             string                            `json:"S3BackupMode"`
}

// httpEndpointConfigurationInput holds the HTTP endpoint URL and name.
type httpEndpointConfigurationInput struct {
	URL       string `json:"Url"`
	Name      string `json:"Name"`
	AccessKey string `json:"AccessKey"`
}

// kinesisStreamSrcInput holds Kinesis stream source config.
type kinesisStreamSrcInput struct {
	KinesisStreamARN string `json:"KinesisStreamARN"`
	RoleARN          string `json:"RoleARN"`
}

// mskSourceConfigurationInput holds MSK cluster source config.
type mskSourceConfigurationInput struct {
	AuthenticationConfiguration *MSKAuthenticationConfiguration `json:"AuthenticationConfiguration"`
	MSKClusterARN               string                          `json:"MSKClusterARN"`
	TopicName                   string                          `json:"TopicName"`
	ReadFromTimestamp           float64                         `json:"ReadFromTimestamp,omitempty"`
}

// directPutSourceConfigurationInput holds Direct PUT source config.
type directPutSourceConfigurationInput struct {
	ThroughputHintInMBs int32 `json:"ThroughputHintInMBs"`
}

// databaseSourceAuthConfigInput holds database source authentication config.
type databaseSourceAuthConfigInput struct {
	SecretsManagerConfiguration *SecretsManagerConfiguration `json:"SecretsManagerConfiguration"`
}

// databaseSourceVPCConfigInput holds the VPC endpoint service used to reach a
// database source.
type databaseSourceVPCConfigInput struct {
	VPCEndpointServiceName string `json:"VpcEndpointServiceName"`
}

// databaseIncludeExcludeListInput is the shared Include/Exclude pattern list shape
// used by DatabaseSourceConfiguration's Databases/Tables/Columns members.
type databaseIncludeExcludeListInput struct {
	Include []string `json:"Include"`
	Exclude []string `json:"Exclude"`
}

// databaseSourceConfigurationInput holds database source config (preview API,
// types.DatabaseSourceConfiguration).
type databaseSourceConfigurationInput struct {
	DatabaseSourceAuthenticationConfiguration *databaseSourceAuthConfigInput   `json:"DatabaseSourceAuthenticationConfiguration"` //nolint:lll // AWS field name
	DatabaseSourceVPCConfiguration            *databaseSourceVPCConfigInput    `json:"DatabaseSourceVPCConfiguration"`
	Databases                                 *databaseIncludeExcludeListInput `json:"Databases"`
	Tables                                    *databaseIncludeExcludeListInput `json:"Tables"`
	Columns                                   *databaseIncludeExcludeListInput `json:"Columns"`
	Endpoint                                  string                           `json:"Endpoint"`
	SnapshotWatermarkTable                    string                           `json:"SnapshotWatermarkTable"`
	SSLMode                                   string                           `json:"SSLMode"`
	Type                                      string                           `json:"Type"`
	SurrogateKeys                             []string                         `json:"SurrogateKeys"`
	Port                                      int32                            `json:"Port"`
}

// redshiftDestinationInput holds the Redshift destination configuration.
// redshiftCopyCommandInput holds the Redshift COPY command configuration. AWS nests
// these fields under RedshiftDestinationConfiguration.CopyCommand on the wire, not as
// flat fields on the destination configuration itself.
type redshiftCopyCommandInput struct {
	DataTableName    string `json:"DataTableName"`
	DataTableColumns string `json:"DataTableColumns"`
	CopyOptions      string `json:"CopyOptions"`
}

// redshiftDestinationInput holds the Redshift destination configuration. S3Configuration/
// S3BackupConfiguration are the Create wire keys (RedshiftDestinationConfiguration);
// S3Update/S3BackupUpdate are the Update wire keys (RedshiftDestinationUpdate) for the
// same two fields -- confirmed via awsAwsjson11_serializeDocumentRedshiftDestinationUpdate.
// A real client sends exactly one of each pair, never both.
type redshiftDestinationInput struct {
	ProcessingConfiguration     *ProcessingConfiguration     `json:"ProcessingConfiguration"`
	RetryOptions                *RetryOptions                `json:"RetryOptions"`
	S3Configuration             *s3DestinationInput          `json:"S3Configuration"`
	S3Update                    *s3DestinationInput          `json:"S3Update"`
	S3BackupConfiguration       *s3BackupInput               `json:"S3BackupConfiguration"`
	S3BackupUpdate              *s3BackupInput               `json:"S3BackupUpdate"`
	CopyCommand                 *redshiftCopyCommandInput    `json:"CopyCommand"`
	CloudWatchLoggingOptions    *CloudWatchLoggingOptions    `json:"CloudWatchLoggingOptions"`
	SecretsManagerConfiguration *SecretsManagerConfiguration `json:"SecretsManagerConfiguration"`
	ClusterJDBCURL              string                       `json:"ClusterJDBCURL"`
	RoleARN                     string                       `json:"RoleARN"`
	S3BackupMode                string                       `json:"S3BackupMode"`
	Username                    string                       `json:"Username"`
}

// openSearchDestinationInput holds the OpenSearch destination configuration.
// S3Configuration is the Create wire key (AmazonopensearchserviceDestinationConfiguration);
// S3Update is the Update wire key (AmazonopensearchserviceDestinationUpdate) for the same
// single S3 bucket -- confirmed via
// awsAwsjson11_serializeDocumentAmazonopensearchserviceDestinationConfiguration/-Update.
// A real client sends exactly one, never both.
type openSearchDestinationInput struct {
	ProcessingConfiguration  *ProcessingConfiguration  `json:"ProcessingConfiguration"`
	BufferingHints           *BufferingHints           `json:"BufferingHints"`
	RetryOptions             *RetryOptions             `json:"RetryOptions"`
	CloudWatchLoggingOptions *CloudWatchLoggingOptions `json:"CloudWatchLoggingOptions"`
	S3Configuration          *s3BackupInput            `json:"S3Configuration"`
	S3Update                 *s3BackupInput            `json:"S3Update"`
	DomainARN                string                    `json:"DomainARN"`
	ClusterEndpoint          string                    `json:"ClusterEndpoint"`
	IndexName                string                    `json:"IndexName"`
	TypeName                 string                    `json:"TypeName"`
	IndexRotationPeriod      string                    `json:"IndexRotationPeriod"`
	S3BackupMode             string                    `json:"S3BackupMode"`
	RoleARN                  string                    `json:"RoleARN"`
}

// elasticsearchDestinationInput holds the legacy Elasticsearch destination configuration.
// S3Configuration is the Create wire key (ElasticsearchDestinationConfiguration);
// S3Update is the Update wire key (ElasticsearchDestinationUpdate) for the same single S3
// bucket -- confirmed via awsAwsjson11_serializeDocumentElasticsearchDestinationUpdate. A
// real client sends exactly one, never both.
type elasticsearchDestinationInput struct {
	ProcessingConfiguration  *ProcessingConfiguration  `json:"ProcessingConfiguration"`
	BufferingHints           *BufferingHints           `json:"BufferingHints"`
	RetryOptions             *RetryOptions             `json:"RetryOptions"`
	CloudWatchLoggingOptions *CloudWatchLoggingOptions `json:"CloudWatchLoggingOptions"`
	S3Configuration          *s3DestinationInput       `json:"S3Configuration"`
	S3Update                 *s3DestinationInput       `json:"S3Update"`
	DomainARN                string                    `json:"DomainARN"`
	ClusterEndpoint          string                    `json:"ClusterEndpoint"`
	IndexName                string                    `json:"IndexName"`
	TypeName                 string                    `json:"TypeName"`
	IndexRotationPeriod      string                    `json:"IndexRotationPeriod"`
	S3BackupMode             string                    `json:"S3BackupMode"`
	RoleARN                  string                    `json:"RoleARN"`
}

// splunkDestinationInput holds the Splunk HEC destination configuration.
// S3Configuration is the Create wire key (SplunkDestinationConfiguration); S3Update is
// the Update wire key (SplunkDestinationUpdate) for the same single S3 bucket --
// confirmed via awsAwsjson11_serializeDocumentSplunkDestinationUpdate. A real client
// sends exactly one, never both.
type splunkDestinationInput struct {
	ProcessingConfiguration           *ProcessingConfiguration  `json:"ProcessingConfiguration"`
	RetryOptions                      *RetryOptions             `json:"RetryOptions"`
	CloudWatchLoggingOptions          *CloudWatchLoggingOptions `json:"CloudWatchLoggingOptions"`
	S3Configuration                   *s3BackupInput            `json:"S3Configuration"`
	S3Update                          *s3BackupInput            `json:"S3Update"`
	HECEndpoint                       string                    `json:"HECEndpoint"`
	HECEndpointType                   string                    `json:"HECEndpointType"`
	HECToken                          string                    `json:"HECToken"`
	S3BackupMode                      string                    `json:"S3BackupMode"`
	HECAcknowledgmentTimeoutInSeconds int                       `json:"HECAcknowledgmentTimeoutInSeconds"`
}

// aosDeliveryField holds the AmazonOpenSearch field separately so its long name
// does not drive gofmt alignment in createDeliveryStreamInput. Embedding keeps
// JSON marshaling transparent.
type aosDeliveryField struct {
	AmazonOpenSearchServiceDestinationConfiguration *openSearchDestinationInput `json:"AmazonOpenSearchServiceDestinationConfiguration"` //nolint:lll // AWS field name
}

// icebergDestinationInput holds the Apache Iceberg Tables destination configuration.
type icebergDestinationInput struct {
	BufferingHints                    *BufferingHints                 `json:"BufferingHints"`
	CatalogConfiguration              *CatalogConfiguration           `json:"CatalogConfiguration"`
	CloudWatchLoggingOptions          *CloudWatchLoggingOptions       `json:"CloudWatchLoggingOptions"`
	ProcessingConfiguration           *ProcessingConfiguration        `json:"ProcessingConfiguration"`
	RetryOptions                      *RetryOptions                   `json:"RetryOptions"`
	S3Configuration                   *s3DestinationInput             `json:"S3Configuration"`
	SchemaEvolutionConfiguration      *SchemaEvolutionConfiguration   `json:"SchemaEvolutionConfiguration"`
	TableCreationConfiguration        *TableCreationConfiguration     `json:"TableCreationConfiguration"`
	RoleARN                           string                          `json:"RoleARN"`
	S3BackupMode                      string                          `json:"S3BackupMode"`
	DestinationTableConfigurationList []DestinationTableConfiguration `json:"DestinationTableConfigurationList"`
	AppendOnly                        bool                            `json:"AppendOnly"`
}

// snowflakeDestinationInput holds the Snowflake destination configuration.
// S3Configuration is the Create wire key (SnowflakeDestinationConfiguration); S3Update is
// the Update wire key (SnowflakeDestinationUpdate) for the same single S3 bucket --
// confirmed via awsAwsjson11_serializeDocumentSnowflakeDestinationUpdate. A real client
// sends exactly one, never both.
type snowflakeDestinationInput struct {
	BufferingHints              *SnowflakeBufferingHints     `json:"BufferingHints"`
	CloudWatchLoggingOptions    *CloudWatchLoggingOptions    `json:"CloudWatchLoggingOptions"`
	ProcessingConfiguration     *ProcessingConfiguration     `json:"ProcessingConfiguration"`
	RetryOptions                *SnowflakeRetryOptions       `json:"RetryOptions"`
	S3Configuration             *s3DestinationInput          `json:"S3Configuration"`
	S3Update                    *s3DestinationInput          `json:"S3Update"`
	SecretsManagerConfiguration *SecretsManagerConfiguration `json:"SecretsManagerConfiguration"`
	SnowflakeRoleConfiguration  *SnowflakeRoleConfiguration  `json:"SnowflakeRoleConfiguration"`
	SnowflakeVpcConfiguration   *SnowflakeVpcConfiguration   `json:"SnowflakeVpcConfiguration"`
	AccountURL                  string                       `json:"AccountUrl"`
	ContentColumnName           string                       `json:"ContentColumnName"`
	DataLoadingOption           string                       `json:"DataLoadingOption"`
	Database                    string                       `json:"Database"`
	KeyPassphrase               string                       `json:"KeyPassphrase"`
	MetaDataColumnName          string                       `json:"MetaDataColumnName"`
	PrivateKey                  string                       `json:"PrivateKey"`
	RoleARN                     string                       `json:"RoleARN"`
	S3BackupMode                string                       `json:"S3BackupMode"`
	Schema                      string                       `json:"Schema"`
	Table                       string                       `json:"Table"`
	User                        string                       `json:"User"`
}

type createDeliveryStreamInput struct {
	aosDeliveryField
	S3DestinationConfiguration            *s3DestinationInput            `json:"S3DestinationConfiguration"`
	ExtendedS3DestinationConfiguration    *s3DestinationInput            `json:"ExtendedS3DestinationConfiguration"`
	HTTPEndpointDestinationConfiguration  *httpEndpointDestinationInput  `json:"HTTPEndpointDestinationConfiguration"`
	KinesisStreamSourceConfiguration      *kinesisStreamSrcInput         `json:"KinesisStreamSourceConfiguration"`
	MSKSourceConfiguration                *mskSourceConfigurationInput   `json:"MSKSourceConfiguration"`
	RedshiftDestinationConfiguration      *redshiftDestinationInput      `json:"RedshiftDestinationConfiguration"`
	ElasticsearchDestinationConfiguration *elasticsearchDestinationInput `json:"ElasticsearchDestinationConfiguration"` //nolint:lll // AWS field name
	SplunkDestinationConfiguration        *splunkDestinationInput        `json:"SplunkDestinationConfiguration"`
	IcebergDestinationConfiguration       *icebergDestinationInput       `json:"IcebergDestinationConfiguration"`
	SnowflakeDestinationConfiguration     *snowflakeDestinationInput     `json:"SnowflakeDestinationConfiguration"`
	// AmazonOpenSearchServerlessDestinationConfiguration is a real, distinct 11th
	// destination type (types.AmazonOpenSearchServerlessDestinationConfiguration) this
	// backend does not implement. Captured only as a presence marker (json.RawMessage,
	// not a typed struct) so handleCreateDeliveryStream can reject it explicitly instead
	// of silently dropping it and creating a stream with zero destinations -- see
	// validateSingleDestination.
	AmazonOpenSearchServerlessDestinationConfiguration json.RawMessage                    `json:"AmazonOpenSearchServerlessDestinationConfiguration,omitempty"` //nolint:lll // AWS field name
	DatabaseSourceConfiguration                        *databaseSourceConfigurationInput  `json:"DatabaseSourceConfiguration"`                                  //nolint:lll // AWS field name
	DirectPutSourceConfiguration                       *directPutSourceConfigurationInput `json:"DirectPutSourceConfiguration"`                                 //nolint:lll // AWS field name
	DeliveryStreamEncryptionConfigurationInput         *EncryptionConfigInput             `json:"DeliveryStreamEncryptionConfigurationInput"`                   //nolint:lll // AWS field name
	DeliveryStreamName                                 string                             `json:"DeliveryStreamName"`
	DeliveryStreamType                                 string                             `json:"DeliveryStreamType"`
	Tags                                               []svcTags.KV                       `json:"Tags"`
}

type createDeliveryStreamOutput struct {
	DeliveryStreamARN string `json:"DeliveryStreamARN"`
}

// buildS3DestinationDescription converts an s3DestinationInput to the backend type.
func buildS3DestinationDescription(raw *s3DestinationInput) *S3DestinationDescription {
	if raw == nil {
		return nil
	}

	// BufferingHints, EncryptionConfiguration and CompressionFormat are
	// optional on the request (validateS3DestinationConfiguration only
	// null-checks RoleARN/BucketARN) but REQUIRED on the response, so a client
	// that omits them must still get them back. gopherstack-r80d batch 28.
	bufferingHints := raw.BufferingHints
	if bufferingHints == nil {
		bufferingHints = defaultS3BufferingHints()
	}

	encryption := raw.EncryptionConfiguration
	if encryption == nil {
		encryption = defaultS3EncryptionConfiguration()
	}

	compression := raw.CompressionFormat
	if compression == "" {
		compression = compressionFormatUncompressed
	}

	dest := &S3DestinationDescription{
		BucketARN:                        raw.BucketARN,
		RoleARN:                          raw.RoleARN,
		Prefix:                           raw.Prefix,
		ErrorOutputPrefix:                raw.ErrorOutputPrefix,
		CompressionFormat:                compression,
		FileExtension:                    raw.FileExtension,
		CustomTimeZone:                   raw.CustomTimeZone,
		BufferingHints:                   bufferingHints,
		ProcessingConfiguration:          raw.ProcessingConfiguration,
		S3BackupMode:                     raw.S3BackupMode,
		EncryptionConfiguration:          encryption,
		CloudWatchLoggingOptions:         raw.CloudWatchLoggingOptions,
		DynamicPartitioningConfiguration: raw.DynamicPartitioningConfiguration,
		DataFormatConversion:             raw.DataFormatConversionConfiguration,
	}

	backup := raw.S3BackupConfiguration
	if backup == nil {
		backup = raw.S3BackupUpdate
	}

	dest.S3BackupDescription = buildS3BackupDescription(backup)

	return dest
}

// buildHTTPEndpointDestination converts httpEndpointDestinationInput to the backend type.
func buildHTTPEndpointDestination(
	ep *httpEndpointDestinationInput,
) *HTTPEndpointDestinationDescription {
	if ep == nil {
		return nil
	}

	dest := &HTTPEndpointDestinationDescription{
		ProcessingConfiguration:  ep.ProcessingConfiguration,
		S3BackupMode:             ep.S3BackupMode,
		RequestConfiguration:     ep.RequestConfiguration,
		BufferingHints:           ep.BufferingHints,
		RetryOptions:             ep.RetryOptions,
		CloudWatchLoggingOptions: ep.CloudWatchLoggingOptions,
	}

	if ep.EndpointConfiguration != nil {
		dest.EndpointConfiguration = &HTTPEndpointConfiguration{
			URL:       ep.EndpointConfiguration.URL,
			Name:      ep.EndpointConfiguration.Name,
			AccessKey: ep.EndpointConfiguration.AccessKey,
		}
	}

	backup := ep.S3Configuration
	if backup == nil {
		backup = ep.S3Update
	}

	if backup != nil {
		dest.S3BackupDescription = buildS3BackupDescription(backup)
	}

	return dest
}

// buildRedshiftDestination converts redshiftDestinationInput to the backend type.
func buildRedshiftDestination(rs *redshiftDestinationInput) *RedshiftDestinationDescription {
	if rs == nil {
		return nil
	}

	s3Config := rs.S3Configuration
	if s3Config == nil {
		s3Config = rs.S3Update
	}

	dest := &RedshiftDestinationDescription{
		ClusterJDBCURL:              rs.ClusterJDBCURL,
		RoleARN:                     rs.RoleARN,
		S3BackupMode:                rs.S3BackupMode,
		ProcessingConfiguration:     rs.ProcessingConfiguration,
		RetryOptions:                rs.RetryOptions,
		Username:                    rs.Username,
		CloudWatchLoggingOptions:    rs.CloudWatchLoggingOptions,
		SecretsManagerConfiguration: rs.SecretsManagerConfiguration,
		S3Destination:               buildS3DestinationDescription(s3Config),
	}

	if rs.CopyCommand != nil {
		dest.CopyCommand = &RedshiftCopyCommand{
			DataTableName:    rs.CopyCommand.DataTableName,
			DataTableColumns: rs.CopyCommand.DataTableColumns,
			CopyOptions:      rs.CopyCommand.CopyOptions,
		}
	}

	backup := rs.S3BackupConfiguration
	if backup == nil {
		backup = rs.S3BackupUpdate
	}

	if backup != nil {
		dest.S3BackupDescription = buildS3BackupDescription(backup)
	}

	return dest
}

// buildOpenSearchDestination converts openSearchDestinationInput to the backend type.
func buildOpenSearchDestination(os *openSearchDestinationInput) *OpenSearchDestinationDescription {
	if os == nil {
		return nil
	}

	dest := &OpenSearchDestinationDescription{
		DomainARN:                os.DomainARN,
		ClusterEndpoint:          os.ClusterEndpoint,
		IndexName:                os.IndexName,
		TypeName:                 os.TypeName,
		IndexRotationPeriod:      os.IndexRotationPeriod,
		S3BackupMode:             os.S3BackupMode,
		RoleARN:                  os.RoleARN,
		ProcessingConfiguration:  os.ProcessingConfiguration,
		BufferingHints:           os.BufferingHints,
		RetryOptions:             os.RetryOptions,
		CloudWatchLoggingOptions: os.CloudWatchLoggingOptions,
	}

	backup := os.S3Configuration
	if backup == nil {
		backup = os.S3Update
	}

	if backup != nil {
		dest.S3BackupDescription = buildS3BackupDescription(backup)
	}

	return dest
}

// buildElasticsearchDestination converts elasticsearchDestinationInput to the backend type.
// This is the legacy Elasticsearch destination shape, wire-distinct from the newer
// AmazonopensearchserviceDestinationConfiguration family (see ElasticsearchDestinationDescription).
func buildElasticsearchDestination(
	es *elasticsearchDestinationInput,
) *ElasticsearchDestinationDescription {
	if es == nil {
		return nil
	}

	dest := &ElasticsearchDestinationDescription{
		DomainARN:                es.DomainARN,
		ClusterEndpoint:          es.ClusterEndpoint,
		IndexName:                es.IndexName,
		TypeName:                 es.TypeName,
		IndexRotationPeriod:      es.IndexRotationPeriod,
		S3BackupMode:             es.S3BackupMode,
		RoleARN:                  es.RoleARN,
		ProcessingConfiguration:  es.ProcessingConfiguration,
		BufferingHints:           es.BufferingHints,
		RetryOptions:             es.RetryOptions,
		CloudWatchLoggingOptions: es.CloudWatchLoggingOptions,
	}

	// AWS models S3Configuration as the required backup destination for legacy
	// Elasticsearch (distinct from the optional-backup pattern used elsewhere).
	s3 := es.S3Configuration
	if s3 == nil {
		s3 = es.S3Update
	}

	if s3 != nil {
		dest.S3BackupDescription = &S3BackupDescription{
			BucketARN:                s3.BucketARN,
			RoleARN:                  s3.RoleARN,
			Prefix:                   s3.Prefix,
			ErrorOutputPrefix:        s3.ErrorOutputPrefix,
			CompressionFormat:        s3.CompressionFormat,
			BufferingHints:           s3.BufferingHints,
			EncryptionConfiguration:  s3.EncryptionConfiguration,
			CloudWatchLoggingOptions: s3.CloudWatchLoggingOptions,
		}
	}

	return dest
}

// buildSplunkDestination converts splunkDestinationInput to the backend type.
func buildSplunkDestination(sp *splunkDestinationInput) *SplunkDestinationDescription {
	if sp == nil {
		return nil
	}

	dest := &SplunkDestinationDescription{
		HECEndpoint:                       sp.HECEndpoint,
		HECEndpointType:                   sp.HECEndpointType,
		HECToken:                          sp.HECToken,
		S3BackupMode:                      sp.S3BackupMode,
		HECAcknowledgmentTimeoutInSeconds: sp.HECAcknowledgmentTimeoutInSeconds,
		ProcessingConfiguration:           sp.ProcessingConfiguration,
		RetryOptions:                      sp.RetryOptions,
		CloudWatchLoggingOptions:          sp.CloudWatchLoggingOptions,
	}

	backup := sp.S3Configuration
	if backup == nil {
		backup = sp.S3Update
	}

	if backup != nil {
		dest.S3BackupDescription = buildS3BackupDescription(backup)
	}

	return dest
}

// buildIcebergDestination converts icebergDestinationInput to the backend type.
func buildIcebergDestination(ic *icebergDestinationInput) *IcebergDestinationDescription {
	if ic == nil {
		return nil
	}

	return &IcebergDestinationDescription{
		BufferingHints:                    ic.BufferingHints,
		CatalogConfiguration:              ic.CatalogConfiguration,
		CloudWatchLoggingOptions:          ic.CloudWatchLoggingOptions,
		ProcessingConfiguration:           ic.ProcessingConfiguration,
		RetryOptions:                      ic.RetryOptions,
		S3Destination:                     buildS3DestinationDescription(ic.S3Configuration),
		SchemaEvolutionConfiguration:      ic.SchemaEvolutionConfiguration,
		TableCreationConfiguration:        ic.TableCreationConfiguration,
		DestinationTableConfigurationList: ic.DestinationTableConfigurationList,
		RoleARN:                           ic.RoleARN,
		S3BackupMode:                      ic.S3BackupMode,
		AppendOnly:                        ic.AppendOnly,
	}
}

// buildSnowflakeDestination converts snowflakeDestinationInput to the backend type.
func buildSnowflakeDestination(sf *snowflakeDestinationInput) *SnowflakeDestinationDescription {
	if sf == nil {
		return nil
	}

	s3Config := sf.S3Configuration
	if s3Config == nil {
		s3Config = sf.S3Update
	}

	return &SnowflakeDestinationDescription{
		BufferingHints:              sf.BufferingHints,
		CloudWatchLoggingOptions:    sf.CloudWatchLoggingOptions,
		ProcessingConfiguration:     sf.ProcessingConfiguration,
		RetryOptions:                sf.RetryOptions,
		S3Destination:               buildS3DestinationDescription(s3Config),
		SecretsManagerConfiguration: sf.SecretsManagerConfiguration,
		SnowflakeRoleConfiguration:  sf.SnowflakeRoleConfiguration,
		SnowflakeVpcConfiguration:   sf.SnowflakeVpcConfiguration,
		AccountURL:                  sf.AccountURL,
		ContentColumnName:           sf.ContentColumnName,
		DataLoadingOption:           sf.DataLoadingOption,
		Database:                    sf.Database,
		MetaDataColumnName:          sf.MetaDataColumnName,
		RoleARN:                     sf.RoleARN,
		S3BackupMode:                sf.S3BackupMode,
		Schema:                      sf.Schema,
		Table:                       sf.Table,
		User:                        sf.User,
	}
}

// buildS3BackupDescription converts an s3BackupInput to the backend type.
// defaultS3BufferingHints mirrors the real SDK's documented default (types.go's
// BufferingHints doc comment: "The default value is 300"/"The default value is 5"),
// applied whenever a real client omits BufferingHints from an S3-family destination
// config -- AWS still requires the response's BufferingHints, so a real client that
// never sets it must still get it back.
func defaultS3BufferingHints() *BufferingHints {
	return &BufferingHints{SizeInMBs: defaultS3BufferingSizeMBs, IntervalInSeconds: defaultS3BufferingIntervalSeconds}
}

// defaultS3EncryptionConfiguration mirrors the real SDK's documented default
// (types.go's EncryptionConfiguration doc comment: "If no value is specified, the
// default is no encryption"), applied whenever a real client omits
// EncryptionConfiguration -- required on the response regardless.
func defaultS3EncryptionConfiguration() *S3EncryptionConfiguration {
	return &S3EncryptionConfiguration{NoEncryptionConfig: "NoEncryption"}
}

func buildS3BackupDescription(b *s3BackupInput) *S3BackupDescription {
	if b == nil {
		return nil
	}

	// S3BackupDescription is typed as a full S3DestinationDescription on the
	// wire (firehose types.go:1575, 2621), so it carries that type's required
	// members -- BufferingHints, EncryptionConfiguration and CompressionFormat
	// -- even though all three are optional on the INPUT. Defaulted here so a
	// caller who omits them still gets a decodable response. See
	// gopherstack-r80d batch 28.
	bufferingHints := b.BufferingHints
	if bufferingHints == nil {
		bufferingHints = defaultS3BufferingHints()
	}

	encryption := b.EncryptionConfiguration
	if encryption == nil {
		encryption = defaultS3EncryptionConfiguration()
	}

	compression := b.CompressionFormat
	if compression == "" {
		compression = compressionFormatUncompressed
	}

	return &S3BackupDescription{
		BucketARN:                b.BucketARN,
		RoleARN:                  b.RoleARN,
		Prefix:                   b.Prefix,
		ErrorOutputPrefix:        b.ErrorOutputPrefix,
		CompressionFormat:        compression,
		BufferingHints:           bufferingHints,
		EncryptionConfiguration:  encryption,
		CloudWatchLoggingOptions: b.CloudWatchLoggingOptions,
	}
}

// buildSourceDescription converts source config inputs to the backend type.
func buildSourceDescription(
	ks *kinesisStreamSrcInput,
	msk *mskSourceConfigurationInput,
	db *databaseSourceConfigurationInput,
	directPut *directPutSourceConfigurationInput,
) *SourceDescription {
	if ks != nil {
		return &SourceDescription{
			KinesisStreamSourceDescription: &KinesisStreamSourceDescription{
				KinesisStreamARN: ks.KinesisStreamARN,
				RoleARN:          ks.RoleARN,
			},
		}
	}

	if msk != nil {
		return &SourceDescription{
			MSKSourceDescription: &MSKSourceDescription{
				MSKClusterARN:               msk.MSKClusterARN,
				TopicName:                   msk.TopicName,
				ReadFromTimestamp:           msk.ReadFromTimestamp,
				AuthenticationConfiguration: msk.AuthenticationConfiguration,
			},
		}
	}

	if db != nil {
		return &SourceDescription{DatabaseSourceDescription: buildDatabaseSourceDescription(db)}
	}

	if directPut != nil {
		return &SourceDescription{
			DirectPutSourceDescription: &DirectPutSourceDescription{
				ThroughputHintInMBs: directPut.ThroughputHintInMBs,
			},
		}
	}

	return nil
}

func buildDatabaseIncludeExcludeList(l *databaseIncludeExcludeListInput) *DatabaseIncludeExcludeList {
	if l == nil {
		return nil
	}

	return &DatabaseIncludeExcludeList{Include: l.Include, Exclude: l.Exclude}
}

func buildDatabaseSourceDescription(db *databaseSourceConfigurationInput) *DatabaseSourceDescription {
	desc := &DatabaseSourceDescription{
		Endpoint:               db.Endpoint,
		Port:                   db.Port,
		SnapshotWatermarkTable: db.SnapshotWatermarkTable,
		SSLMode:                db.SSLMode,
		Type:                   db.Type,
		SurrogateKeys:          db.SurrogateKeys,
		Databases:              buildDatabaseIncludeExcludeList(db.Databases),
		Tables:                 buildDatabaseIncludeExcludeList(db.Tables),
		Columns:                buildDatabaseIncludeExcludeList(db.Columns),
	}

	if db.DatabaseSourceAuthenticationConfiguration != nil {
		desc.DatabaseSourceAuthenticationConfiguration = &DatabaseSourceAuthenticationConfiguration{
			SecretsManagerConfiguration: db.DatabaseSourceAuthenticationConfiguration.SecretsManagerConfiguration,
		}
	}

	if db.DatabaseSourceVPCConfiguration != nil {
		desc.DatabaseSourceVPCConfiguration = &DatabaseSourceVPCConfiguration{
			VPCEndpointServiceName: db.DatabaseSourceVPCConfiguration.VPCEndpointServiceName,
		}
	}

	return desc
}

// validateSingleDestination rejects a CreateDeliveryStream request that names more than
// one destination configuration. Real AWS accepts exactly one destination type per call
// (S3DestinationConfiguration and ExtendedS3DestinationConfiguration are mutually exclusive
// aliases for the same "S3 family" slot and are counted together).
func validateSingleDestination(in *createDeliveryStreamInput) error {
	provided := 0
	if in.S3DestinationConfiguration != nil || in.ExtendedS3DestinationConfiguration != nil {
		provided++
	}
	if in.HTTPEndpointDestinationConfiguration != nil {
		provided++
	}
	if in.RedshiftDestinationConfiguration != nil {
		provided++
	}
	if in.AmazonOpenSearchServiceDestinationConfiguration != nil {
		provided++
	}
	if in.ElasticsearchDestinationConfiguration != nil {
		provided++
	}
	if in.SplunkDestinationConfiguration != nil {
		provided++
	}
	if in.IcebergDestinationConfiguration != nil {
		provided++
	}
	if in.SnowflakeDestinationConfiguration != nil {
		provided++
	}
	if in.AmazonOpenSearchServerlessDestinationConfiguration != nil {
		provided++
	}

	if provided > 1 {
		return fmt.Errorf("%w: at most one destination configuration may be specified, got %d",
			ErrValidation, provided)
	}

	// AmazonOpenSearchServerlessDestinationConfiguration is a real destination type
	// (see the field's own doc comment) this backend has no field/build path for --
	// reject explicitly rather than silently create a stream with zero destinations.
	if in.AmazonOpenSearchServerlessDestinationConfiguration != nil {
		return fmt.Errorf(
			"%w: AmazonOpenSearchServerlessDestinationConfiguration is not supported by this emulator",
			ErrValidation)
	}

	return nil
}

func (h *Handler) handleCreateDeliveryStream(
	ctx context.Context,
	in *createDeliveryStreamInput,
) (*createDeliveryStreamOutput, error) {
	if err := validateTags(in.Tags); err != nil {
		return nil, err
	}

	// ExtendedS3 takes precedence over plain S3.
	rawS3 := in.ExtendedS3DestinationConfiguration
	if rawS3 == nil {
		rawS3 = in.S3DestinationConfiguration
	}

	if rawS3 != nil {
		if err := validateDataFormatConversion(rawS3.DataFormatConversionConfiguration); err != nil {
			return nil, err
		}
	}

	if err := validateSingleDestination(in); err != nil {
		return nil, err
	}

	if err := validateEncryptionConfigInput(in.DeliveryStreamEncryptionConfigurationInput); err != nil {
		return nil, err
	}

	s, err := h.Backend.CreateDeliveryStream(ctx, CreateDeliveryStreamInput{
		Name:               in.DeliveryStreamName,
		DeliveryStreamType: in.DeliveryStreamType,
		S3Destination:      buildS3DestinationDescription(rawS3),
		HTTPEndpointDestination: buildHTTPEndpointDestination(
			in.HTTPEndpointDestinationConfiguration,
		),
		RedshiftDestination: buildRedshiftDestination(in.RedshiftDestinationConfiguration),
		OpenSearchDestination: buildOpenSearchDestination(
			in.AmazonOpenSearchServiceDestinationConfiguration,
		),
		ElasticsearchDestination: buildElasticsearchDestination(
			in.ElasticsearchDestinationConfiguration,
		),
		SplunkDestination:    buildSplunkDestination(in.SplunkDestinationConfiguration),
		IcebergDestination:   buildIcebergDestination(in.IcebergDestinationConfiguration),
		SnowflakeDestination: buildSnowflakeDestination(in.SnowflakeDestinationConfiguration),
		Source: buildSourceDescription(
			in.KinesisStreamSourceConfiguration, in.MSKSourceConfiguration,
			in.DatabaseSourceConfiguration, in.DirectPutSourceConfiguration,
		),
	})
	if err != nil {
		return nil, err
	}

	if len(in.Tags) > 0 {
		tagMap := make(map[string]string, len(in.Tags))
		for _, t := range in.Tags {
			tagMap[t.Key] = t.Value
		}

		_ = h.Backend.TagDeliveryStream(ctx, in.DeliveryStreamName, tagMap)
	}

	if in.DeliveryStreamEncryptionConfigurationInput != nil {
		if encErr := h.Backend.StartDeliveryStreamEncryption(
			ctx, in.DeliveryStreamName, in.DeliveryStreamEncryptionConfigurationInput,
		); encErr != nil {
			return nil, encErr
		}
	}

	return &createDeliveryStreamOutput{DeliveryStreamARN: s.ARN}, nil
}

type deleteDeliveryStreamOutput struct{}

func (h *Handler) handleDeleteDeliveryStream(
	ctx context.Context,
	in *deliveryStreamNameInput,
) (*deleteDeliveryStreamOutput, error) {
	if err := h.Backend.DeleteDeliveryStream(ctx, in.DeliveryStreamName); err != nil {
		return nil, err
	}

	return &deleteDeliveryStreamOutput{}, nil
}

// destinationDescriptionOutput mirrors AWS's DestinationDescription shape: a
// DestinationId plus at most one populated type-specific description. Real
// DescribeDeliveryStream responses nest every destination type under a single
// "Destinations" list on the wire rather than exposing separate per-type lists.
type destinationDescriptionOutput struct {
	ExtendedS3DestinationDescription              *S3DestinationDescription            `json:"ExtendedS3DestinationDescription,omitempty"`              //nolint:lll // AWS field name
	HTTPEndpointDestinationDescription            *HTTPEndpointDestinationDescription  `json:"HttpEndpointDestinationDescription,omitempty"`            //nolint:lll // AWS field name (note "Http" casing)
	RedshiftDestinationDescription                *RedshiftDestinationDescription      `json:"RedshiftDestinationDescription,omitempty"`                //nolint:lll // AWS field name
	AmazonopensearchserviceDestinationDescription *OpenSearchDestinationDescription    `json:"AmazonopensearchserviceDestinationDescription,omitempty"` //nolint:lll // AWS field name (exact casing)
	ElasticsearchDestinationDescription           *ElasticsearchDestinationDescription `json:"ElasticsearchDestinationDescription,omitempty"`           //nolint:lll // AWS field name
	SplunkDestinationDescription                  *SplunkDestinationDescription        `json:"SplunkDestinationDescription,omitempty"`                  //nolint:lll // AWS field name
	IcebergDestinationDescription                 *IcebergDestinationDescription       `json:"IcebergDestinationDescription,omitempty"`                 //nolint:lll // AWS field name
	SnowflakeDestinationDescription               *SnowflakeDestinationDescription     `json:"SnowflakeDestinationDescription,omitempty"`               //nolint:lll // AWS field name
	DestinationID                                 string                               `json:"DestinationId"`
}

type deliveryStreamDescriptionFields struct {
	EncryptionConfiguration *EncryptionConfig              `json:"DeliveryStreamEncryptionConfiguration,omitempty"` //nolint:lll // AWS field name
	Source                  *SourceDescription             `json:"Source,omitempty"`
	CreateTimestamp         *int64                         `json:"CreateTimestamp,omitempty"`
	LastUpdateTimestamp     *int64                         `json:"LastUpdateTimestamp,omitempty"` //nolint:lll // AWS field name
	DeliveryStreamName      string                         `json:"DeliveryStreamName"`
	DeliveryStreamARN       string                         `json:"DeliveryStreamARN"`
	DeliveryStreamStatus    string                         `json:"DeliveryStreamStatus"`
	DeliveryStreamType      string                         `json:"DeliveryStreamType,omitempty"` //nolint:lll // AWS field name
	VersionID               string                         `json:"VersionId,omitempty"`
	Destinations            []destinationDescriptionOutput `json:"Destinations"`
	HasMoreDestinations     bool                           `json:"HasMoreDestinations"`
}

type describeDeliveryStreamInput struct {
	DeliveryStreamName          string `json:"DeliveryStreamName"`
	ExclusiveStartDestinationID string `json:"ExclusiveStartDestinationId"`
	Limit                       int    `json:"Limit"`
}

type describeDeliveryStreamOutput struct {
	DeliveryStreamDescription deliveryStreamDescriptionFields `json:"DeliveryStreamDescription"`
}

func (h *Handler) handleDescribeDeliveryStream(
	ctx context.Context,
	in *describeDeliveryStreamInput,
) (*describeDeliveryStreamOutput, error) {
	s, err := h.Backend.DescribeDeliveryStream(ctx, in.DeliveryStreamName)
	if err != nil {
		return nil, err
	}

	var createTS, updateTS *int64
	if !s.CreateTimestamp.IsZero() {
		ts := s.CreateTimestamp.Unix()
		createTS = &ts
	}

	if !s.LastUpdateTimestamp.IsZero() {
		ts := s.LastUpdateTimestamp.Unix()
		updateTS = &ts
	}

	desc := deliveryStreamDescriptionFields{
		DeliveryStreamName:      s.Name,
		DeliveryStreamARN:       s.ARN,
		DeliveryStreamStatus:    s.Status,
		DeliveryStreamType:      s.DeliveryStreamType,
		VersionID:               s.VersionID,
		EncryptionConfiguration: s.Encryption,
		Source:                  s.Source,
		CreateTimestamp:         createTS,
		LastUpdateTimestamp:     updateTS,
		Destinations:            buildDestinationDescriptions(s),
		HasMoreDestinations:     false,
	}

	return &describeDeliveryStreamOutput{DeliveryStreamDescription: desc}, nil
}

// defaultDestinationID is the synthetic DestinationId AWS assigns to a stream's first
// (and, in this backend, only) destination when none has been explicitly stamped yet.
const defaultDestinationID = "destinationId-000000000001"

// destinationIDOrDefault returns id, or defaultDestinationID when id is empty.
func destinationIDOrDefault(id string) string {
	if id == "" {
		return defaultDestinationID
	}

	return id
}

// buildDestinationDescriptions converts a DeliveryStream's per-type destination fields
// into the AWS wire shape: a single "Destinations" list of DestinationDescription
// entries, each carrying a DestinationId plus exactly one populated type-specific
// description. AWS never exposes separate top-level lists per destination type.
func buildDestinationDescriptions(s *DeliveryStream) []destinationDescriptionOutput {
	destinations := make([]destinationDescriptionOutput, 0, 1)

	if s.S3Destination != nil {
		d := *s.S3Destination
		destinations = append(destinations, destinationDescriptionOutput{
			DestinationID:                    destinationIDOrDefault(d.DestinationID),
			ExtendedS3DestinationDescription: &d,
		})
	}

	if s.HTTPEndpointDestination != nil {
		d := *s.HTTPEndpointDestination
		destinations = append(destinations, destinationDescriptionOutput{
			DestinationID:                      destinationIDOrDefault(d.DestinationID),
			HTTPEndpointDestinationDescription: &d,
		})
	}

	if s.RedshiftDestination != nil {
		d := *s.RedshiftDestination
		destinations = append(destinations, destinationDescriptionOutput{
			DestinationID:                  destinationIDOrDefault(d.DestinationID),
			RedshiftDestinationDescription: &d,
		})
	}

	if s.OpenSearchDestination != nil {
		d := *s.OpenSearchDestination
		destinations = append(destinations, destinationDescriptionOutput{
			DestinationID: destinationIDOrDefault(d.DestinationID),
			AmazonopensearchserviceDestinationDescription: &d,
		})
	}

	if s.ElasticsearchDestination != nil {
		d := *s.ElasticsearchDestination
		destinations = append(destinations, destinationDescriptionOutput{
			DestinationID:                       destinationIDOrDefault(d.DestinationID),
			ElasticsearchDestinationDescription: &d,
		})
	}

	if s.SplunkDestination != nil {
		d := *s.SplunkDestination
		destinations = append(destinations, destinationDescriptionOutput{
			DestinationID:                destinationIDOrDefault(d.DestinationID),
			SplunkDestinationDescription: &d,
		})
	}

	if s.IcebergDestination != nil {
		d := *s.IcebergDestination
		destinations = append(destinations, destinationDescriptionOutput{
			DestinationID:                 destinationIDOrDefault(d.DestinationID),
			IcebergDestinationDescription: &d,
		})
	}

	if s.SnowflakeDestination != nil {
		d := *s.SnowflakeDestination
		destinations = append(destinations, destinationDescriptionOutput{
			DestinationID:                   destinationIDOrDefault(d.DestinationID),
			SnowflakeDestinationDescription: &d,
		})
	}

	return destinations
}

type listDeliveryStreamsInput struct {
	ExclusiveStartDeliveryStreamName string `json:"ExclusiveStartDeliveryStreamName"`
	DeliveryStreamType               string `json:"DeliveryStreamType"`
	Limit                            int    `json:"Limit"`
}

type listDeliveryStreamsOutput struct {
	DeliveryStreamNames    []string `json:"DeliveryStreamNames"`
	HasMoreDeliveryStreams bool     `json:"HasMoreDeliveryStreams"`
}

func (h *Handler) handleListDeliveryStreams(
	ctx context.Context,
	in *listDeliveryStreamsInput,
) (*listDeliveryStreamsOutput, error) {
	// ListDeliveryStreams declares no exception beyond UnknownError
	// (deserializers.go: deserializeOpErrorListDeliveryStreams), so an
	// unrecognized DeliveryStreamType cannot be rejected -- it just matches
	// no stored stream, same as any other unmatched filter value.
	names := h.Backend.ListDeliveryStreamsByType(ctx, in.DeliveryStreamType)

	// Apply ExclusiveStartDeliveryStreamName cursor.
	if in.ExclusiveStartDeliveryStreamName != "" {
		startIdx := -1
		for i, n := range names {
			if n == in.ExclusiveStartDeliveryStreamName {
				startIdx = i

				break
			}
		}
		if startIdx >= 0 {
			names = names[startIdx+1:]
		}
	}

	hasMore := false
	limit := in.Limit
	if limit <= 0 || limit > maxListLimit {
		limit = maxListLimit
	}

	if len(names) > limit {
		names = names[:limit]
		hasMore = true
	}

	return &listDeliveryStreamsOutput{
		DeliveryStreamNames:    names,
		HasMoreDeliveryStreams: hasMore,
	}, nil
}
