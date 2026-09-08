package firehose

import (
	"context"

	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Storer is the subset of S3 operations that Firehose needs to deliver objects.
type S3Storer interface {
	PutObject(ctx context.Context, input *sdk_s3.PutObjectInput) (*sdk_s3.PutObjectOutput, error)
}

// LambdaInvoker is the subset of Lambda operations that Firehose needs for transformation.
type LambdaInvoker interface {
	InvokeFunction(ctx context.Context, name string, invocationType string, payload []byte) ([]byte, int, error)
}

// RedshiftDataExecutor is the subset of Redshift Data API operations that Firehose needs to
// issue the COPY command against staged S3 data. Modeled after S3Storer/LambdaInvoker: a
// minimal in-process interface a sibling backend can satisfy, wired via SetRedshiftDataBackend.
type RedshiftDataExecutor interface {
	ExecuteStatement(ctx context.Context, sql, clusterIdentifier, database, dbUser string) error
}

// CWLogsBackend is the subset of CloudWatch Logs operations that Firehose needs to deliver
// a destination's CloudWatchLoggingOptions error-log events, wired via SetCWLogsBackend.
type CWLogsBackend interface {
	EnsureLogGroupAndStream(groupName, streamName string) error
	PutLogLines(groupName, streamName string, messages []string) error
}

// KinesisReader is the subset of Kinesis operations that Firehose needs to poll source streams.
type KinesisReader interface {
	// ListShards returns all open shard IDs for the named stream.
	ListShards(streamName string) ([]string, error)
	// GetShardIterator returns a TRIM_HORIZON iterator token for the given stream/shard.
	GetShardIterator(streamName, shardID string) (string, error)
	// GetRecords reads up to limit records. Returns raw data slices, next iterator token, and error.
	GetRecords(shardIterator string, limit int) (records [][]byte, nextIterator string, err error)
}

// StorageBackend defines the interface for Firehose backend implementations.
// All mutating methods must be safe for concurrent use.
type StorageBackend interface {
	CreateDeliveryStream(ctx context.Context, input CreateDeliveryStreamInput) (*DeliveryStream, error)
	DeleteDeliveryStream(ctx context.Context, name string) error
	DescribeDeliveryStream(ctx context.Context, name string) (*DeliveryStream, error)
	ListDeliveryStreams(ctx context.Context) []string
	ListDeliveryStreamsByType(ctx context.Context, streamType string) []string
	PutRecord(ctx context.Context, streamName string, data []byte) error
	PutRecordBatch(ctx context.Context, streamName string, records [][]byte) (int, error)
	// IsStreamEncrypted reports whether server-side encryption is currently enabled on the
	// named stream, used to populate PutRecord/PutRecordBatch's optional Encrypted field.
	IsStreamEncrypted(ctx context.Context, streamName string) bool
	UpdateDestination(ctx context.Context, streamName, currentVersionID string, input UpdateDestinationInput) error
	ListTagsForDeliveryStream(ctx context.Context, name string) (map[string]string, error)
	TagDeliveryStream(ctx context.Context, name string, kv map[string]string) error
	UntagDeliveryStream(ctx context.Context, name string, keys []string) error
	StartDeliveryStreamEncryption(ctx context.Context, name string, input *EncryptionConfigInput) error
	StopDeliveryStreamEncryption(ctx context.Context, name string) error
	Reset()
	Region() string
	RunFlusher(ctx context.Context)
	FlushAll(ctx context.Context)
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error
	AddStreamInternal(s *DeliveryStream)
}
