package pipes

import (
	"fmt"
	"time"
)

// SQSSourceParameters holds SQS-specific source configuration.
type SQSSourceParameters struct {
	BatchSize                      int `json:"BatchSize,omitempty"`
	MaximumBatchingWindowInSeconds int `json:"MaximumBatchingWindowInSeconds,omitempty"`
}

// KinesisStreamSourceParameters holds Kinesis-specific source configuration.
type KinesisStreamSourceParameters struct {
	StartingPositionTimestamp      *time.Time        `json:"StartingPositionTimestamp,omitempty"`
	DeadLetterConfig               *DeadLetterConfig `json:"DeadLetterConfig,omitempty"`
	StartingPosition               string            `json:"StartingPosition,omitempty"`
	OnPartialBatchItemFailure      string            `json:"OnPartialBatchItemFailure,omitempty"`
	BatchSize                      int               `json:"BatchSize,omitempty"`
	MaximumBatchingWindowInSeconds int               `json:"MaximumBatchingWindowInSeconds,omitempty"`
	MaximumRecordAgeInSeconds      int               `json:"MaximumRecordAgeInSeconds,omitempty"`
	MaximumRetryAttempts           int               `json:"MaximumRetryAttempts,omitempty"`
	ParallelizationFactor          int               `json:"ParallelizationFactor,omitempty"`
}

// DynamoDBStreamSourceParameters holds DynamoDB stream source configuration.
type DynamoDBStreamSourceParameters struct {
	DeadLetterConfig               *DeadLetterConfig `json:"DeadLetterConfig,omitempty"`
	StartingPosition               string            `json:"StartingPosition,omitempty"`
	OnPartialBatchItemFailure      string            `json:"OnPartialBatchItemFailure,omitempty"`
	BatchSize                      int               `json:"BatchSize,omitempty"`
	MaximumBatchingWindowInSeconds int               `json:"MaximumBatchingWindowInSeconds,omitempty"`
	MaximumRecordAgeInSeconds      int               `json:"MaximumRecordAgeInSeconds,omitempty"`
	MaximumRetryAttempts           int               `json:"MaximumRetryAttempts,omitempty"`
	ParallelizationFactor          int               `json:"ParallelizationFactor,omitempty"`
}

// MSKAccessCredentials holds authentication credentials for MSK sources.
// Exactly one field is populated (models an AWS union type).
type MSKAccessCredentials struct {
	ClientCertificateTLSAuth string `json:"ClientCertificateTlsAuth,omitempty"`
	SaslScram512Auth         string `json:"SaslScram512Auth,omitempty"`
}

// MSKSourceParameters holds MSK source configuration.
type MSKSourceParameters struct {
	Credentials                    *MSKAccessCredentials `json:"Credentials,omitempty"`
	TopicName                      string                `json:"TopicName,omitempty"`
	StartingPosition               string                `json:"StartingPosition,omitempty"`
	ConsumerGroupID                string                `json:"ConsumerGroupId,omitempty"`
	BatchSize                      int                   `json:"BatchSize,omitempty"`
	MaximumBatchingWindowInSeconds int                   `json:"MaximumBatchingWindowInSeconds,omitempty"`
}

// SelfManagedKafkaAccessCredentials holds authentication credentials for self-managed Kafka.
// Exactly one field is populated (models an AWS union type).
type SelfManagedKafkaAccessCredentials struct {
	BasicAuth                string `json:"BasicAuth,omitempty"`
	ClientCertificateTLSAuth string `json:"ClientCertificateTlsAuth,omitempty"`
	SaslScram256Auth         string `json:"SaslScram256Auth,omitempty"`
	SaslScram512Auth         string `json:"SaslScram512Auth,omitempty"`
}

// SelfManagedKafkaVpc holds VPC configuration for self-managed Kafka connectivity.
type SelfManagedKafkaVpc struct {
	SecurityGroup []string `json:"SecurityGroup,omitempty"`
	Subnets       []string `json:"Subnets,omitempty"`
}

// SelfManagedKafkaSourceParameters holds self-managed Kafka source configuration.
type SelfManagedKafkaSourceParameters struct {
	Credentials                    *SelfManagedKafkaAccessCredentials `json:"Credentials,omitempty"`
	Vpc                            *SelfManagedKafkaVpc               `json:"Vpc,omitempty"`
	TopicName                      string                             `json:"TopicName,omitempty"`
	StartingPosition               string                             `json:"StartingPosition,omitempty"`
	ConsumerGroupID                string                             `json:"ConsumerGroupId,omitempty"`
	ServerRootCaCertificate        string                             `json:"ServerRootCaCertificate,omitempty"`
	AdditionalBootstrapServers     []string                           `json:"AdditionalBootstrapServers,omitempty"`
	BatchSize                      int                                `json:"BatchSize,omitempty"`
	MaximumBatchingWindowInSeconds int                                `json:"MaximumBatchingWindowInSeconds,omitempty"`
}

// MQBrokerCredentials holds credentials for ActiveMQ or RabbitMQ broker sources.
type MQBrokerCredentials struct {
	BasicAuth string `json:"BasicAuth,omitempty"`
}

// RabbitMQBrokerSourceParameters holds RabbitMQ broker source configuration.
type RabbitMQBrokerSourceParameters struct {
	Credentials                    *MQBrokerCredentials `json:"Credentials,omitempty"`
	QueueName                      string               `json:"QueueName,omitempty"`
	VirtualHost                    string               `json:"VirtualHost,omitempty"`
	BatchSize                      int                  `json:"BatchSize,omitempty"`
	MaximumBatchingWindowInSeconds int                  `json:"MaximumBatchingWindowInSeconds,omitempty"`
}

// ActiveMQBrokerSourceParameters holds ActiveMQ broker source configuration.
type ActiveMQBrokerSourceParameters struct {
	Credentials                    *MQBrokerCredentials `json:"Credentials,omitempty"`
	QueueName                      string               `json:"QueueName,omitempty"`
	BatchSize                      int                  `json:"BatchSize,omitempty"`
	MaximumBatchingWindowInSeconds int                  `json:"MaximumBatchingWindowInSeconds,omitempty"`
}

// SourceParameters holds source-specific configuration.
type SourceParameters struct {
	FilterCriteria                  *FilterCriteria                   `json:"FilterCriteria,omitempty"`
	SqsQueueParameters              *SQSSourceParameters              `json:"SqsQueueParameters,omitempty"`
	KinesisStreamParameters         *KinesisStreamSourceParameters    `json:"KinesisStreamParameters,omitempty"`
	DynamoDBStreamParameters        *DynamoDBStreamSourceParameters   `json:"DynamoDBStreamParameters,omitempty"`
	ManagedStreamingKafkaParameters *MSKSourceParameters              `json:"ManagedStreamingKafkaParameters,omitempty"`
	SelfManagedKafkaParameters      *SelfManagedKafkaSourceParameters `json:"SelfManagedKafkaParameters,omitempty"`
	RabbitMQBrokerParameters        *RabbitMQBrokerSourceParameters   `json:"RabbitMQBrokerParameters,omitempty"`
	ActiveMQBrokerParameters        *ActiveMQBrokerSourceParameters   `json:"ActiveMQBrokerParameters,omitempty"`
}

func cloneSelfManagedKafkaVpc(src *SelfManagedKafkaVpc) *SelfManagedKafkaVpc {
	if src == nil {
		return nil
	}
	vpc := *src
	vpc.SecurityGroup = append([]string(nil), src.SecurityGroup...)
	vpc.Subnets = append([]string(nil), src.Subnets...)

	return &vpc
}

func cloneSourceParameters(src *SourceParameters) *SourceParameters {
	sp := *src
	if src.FilterCriteria != nil {
		fc := *src.FilterCriteria
		fc.Filters = append([]Filter(nil), src.FilterCriteria.Filters...)
		sp.FilterCriteria = &fc
	}
	if src.SqsQueueParameters != nil {
		v := *src.SqsQueueParameters
		sp.SqsQueueParameters = &v
	}
	if src.KinesisStreamParameters != nil {
		v := *src.KinesisStreamParameters
		v.DeadLetterConfig = cloneDeadLetterConfig(v.DeadLetterConfig)
		sp.KinesisStreamParameters = &v
	}
	if src.DynamoDBStreamParameters != nil {
		v := *src.DynamoDBStreamParameters
		v.DeadLetterConfig = cloneDeadLetterConfig(v.DeadLetterConfig)
		sp.DynamoDBStreamParameters = &v
	}
	if src.ManagedStreamingKafkaParameters != nil {
		v := *src.ManagedStreamingKafkaParameters
		if v.Credentials != nil {
			c := *v.Credentials
			v.Credentials = &c
		}
		sp.ManagedStreamingKafkaParameters = &v
	}
	if src.SelfManagedKafkaParameters != nil {
		v := *src.SelfManagedKafkaParameters
		v.AdditionalBootstrapServers = append(
			[]string(nil), src.SelfManagedKafkaParameters.AdditionalBootstrapServers...,
		)
		if v.Credentials != nil {
			c := *v.Credentials
			v.Credentials = &c
		}
		v.Vpc = cloneSelfManagedKafkaVpc(v.Vpc)
		sp.SelfManagedKafkaParameters = &v
	}
	if src.RabbitMQBrokerParameters != nil {
		v := *src.RabbitMQBrokerParameters
		if v.Credentials != nil {
			c := *v.Credentials
			v.Credentials = &c
		}
		sp.RabbitMQBrokerParameters = &v
	}
	if src.ActiveMQBrokerParameters != nil {
		v := *src.ActiveMQBrokerParameters
		if v.Credentials != nil {
			c := *v.Credentials
			v.Credentials = &c
		}
		sp.ActiveMQBrokerParameters = &v
	}

	return &sp
}

func sourceBatchSize(sp *SourceParameters) int {
	switch {
	case sp.SqsQueueParameters != nil && sp.SqsQueueParameters.BatchSize > 0:
		return sp.SqsQueueParameters.BatchSize
	case sp.KinesisStreamParameters != nil && sp.KinesisStreamParameters.BatchSize > 0:
		return sp.KinesisStreamParameters.BatchSize
	case sp.DynamoDBStreamParameters != nil && sp.DynamoDBStreamParameters.BatchSize > 0:
		return sp.DynamoDBStreamParameters.BatchSize
	case sp.ManagedStreamingKafkaParameters != nil && sp.ManagedStreamingKafkaParameters.BatchSize > 0:
		return sp.ManagedStreamingKafkaParameters.BatchSize
	case sp.SelfManagedKafkaParameters != nil && sp.SelfManagedKafkaParameters.BatchSize > 0:
		return sp.SelfManagedKafkaParameters.BatchSize
	case sp.RabbitMQBrokerParameters != nil && sp.RabbitMQBrokerParameters.BatchSize > 0:
		return sp.RabbitMQBrokerParameters.BatchSize
	case sp.ActiveMQBrokerParameters != nil && sp.ActiveMQBrokerParameters.BatchSize > 0:
		return sp.ActiveMQBrokerParameters.BatchSize
	}

	return 0
}

const maxBatchSize = 10000

// batchSizeEntry pairs a BatchSize value with its source type label.
type batchSizeEntry struct {
	Name string
	Size int
}

// sourceBatchSizes collects all configured BatchSize values from SourceParameters
// along with their source type labels.
func sourceBatchSizes(sp *SourceParameters) []batchSizeEntry {
	var out []batchSizeEntry

	if p := sp.SqsQueueParameters; p != nil {
		out = append(out, batchSizeEntry{"SQS", p.BatchSize})
	}
	if p := sp.KinesisStreamParameters; p != nil {
		out = append(out, batchSizeEntry{"Kinesis", p.BatchSize})
	}
	if p := sp.DynamoDBStreamParameters; p != nil {
		out = append(out, batchSizeEntry{"DynamoDB", p.BatchSize})
	}
	if p := sp.ManagedStreamingKafkaParameters; p != nil {
		out = append(out, batchSizeEntry{"MSK", p.BatchSize})
	}
	if p := sp.SelfManagedKafkaParameters; p != nil {
		out = append(out, batchSizeEntry{"SelfManagedKafka", p.BatchSize})
	}
	if p := sp.RabbitMQBrokerParameters; p != nil {
		out = append(out, batchSizeEntry{"RabbitMQ", p.BatchSize})
	}
	if p := sp.ActiveMQBrokerParameters; p != nil {
		out = append(out, batchSizeEntry{"ActiveMQ", p.BatchSize})
	}

	return out
}

// validateSourceBatchSize checks that all BatchSize fields in SourceParameters
// are within the valid range [0, 10000]. Negative values and values above
// 10000 are rejected with a ValidationException, matching AWS behaviour.
func validateSourceBatchSize(sp *SourceParameters) error {
	if sp == nil {
		return nil
	}

	for _, bs := range sourceBatchSizes(sp) {
		if bs.Size < 0 || bs.Size > maxBatchSize {
			return fmt.Errorf(
				"%w: %s BatchSize must be between 0 and %d, got %d",
				ErrValidation, bs.Name, maxBatchSize, bs.Size,
			)
		}
	}

	return nil
}

// validateSourceStartingPosition enforces that Kinesis and DynamoDB Streams
// sources specify StartingPosition, matching aws-sdk-go-v2 pipes
// validators.go's validatePipeSourceKinesisStreamParameters and
// validatePipeSourceDynamoDBStreamParameters (both mark it required). This
// applies only at CreatePipe: UpdatePipeSourceParameters carries no such
// requirement, since starting position cannot be changed after creation.
func validateSourceStartingPosition(sp *SourceParameters) error {
	if sp == nil {
		return nil
	}
	if kp := sp.KinesisStreamParameters; kp != nil && kp.StartingPosition == "" {
		return fmt.Errorf("%w: KinesisStreamParameters.StartingPosition is required", ErrValidation)
	}
	if dp := sp.DynamoDBStreamParameters; dp != nil && dp.StartingPosition == "" {
		return fmt.Errorf("%w: DynamoDBStreamParameters.StartingPosition is required", ErrValidation)
	}

	return nil
}

// validateSourceRequiredFields enforces required nested source fields at
// CreatePipe, matching aws-sdk-go-v2 pipes validators.go's
// validatePipeSourceActiveMQBrokerParameters, validatePipeSourceRabbitMQBrokerParameters
// (Credentials and QueueName required), validatePipeSourceManagedStreamingKafkaParameters,
// and validatePipeSourceSelfManagedKafkaParameters (TopicName required).
func validateSourceRequiredFields(sp *SourceParameters) error {
	if sp == nil {
		return nil
	}
	if mp := sp.ActiveMQBrokerParameters; mp != nil {
		if mp.Credentials == nil {
			return fmt.Errorf("%w: ActiveMQBrokerParameters.Credentials is required", ErrValidation)
		}
		if mp.QueueName == "" {
			return fmt.Errorf("%w: ActiveMQBrokerParameters.QueueName is required", ErrValidation)
		}
	}
	if mp := sp.RabbitMQBrokerParameters; mp != nil {
		if mp.Credentials == nil {
			return fmt.Errorf("%w: RabbitMQBrokerParameters.Credentials is required", ErrValidation)
		}
		if mp.QueueName == "" {
			return fmt.Errorf("%w: RabbitMQBrokerParameters.QueueName is required", ErrValidation)
		}
	}
	if kp := sp.ManagedStreamingKafkaParameters; kp != nil && kp.TopicName == "" {
		return fmt.Errorf("%w: ManagedStreamingKafkaParameters.TopicName is required", ErrValidation)
	}
	if kp := sp.SelfManagedKafkaParameters; kp != nil && kp.TopicName == "" {
		return fmt.Errorf("%w: SelfManagedKafkaParameters.TopicName is required", ErrValidation)
	}

	return nil
}

// validateUpdateSourceRequiredFields enforces required nested source fields at
// UpdatePipe, matching aws-sdk-go-v2 pipes validators.go's
// validateUpdatePipeSourceActiveMQBrokerParameters and
// validateUpdatePipeSourceRabbitMQBrokerParameters: only Credentials is
// required on update. QueueName and TopicName cannot be changed after
// creation and carry no update-side requirement; Kinesis and DynamoDB
// Streams StartingPosition likewise (see validateSourceStartingPosition).
func validateUpdateSourceRequiredFields(sp *SourceParameters) error {
	if sp == nil {
		return nil
	}
	if mp := sp.ActiveMQBrokerParameters; mp != nil && mp.Credentials == nil {
		return fmt.Errorf("%w: ActiveMQBrokerParameters.Credentials is required", ErrValidation)
	}
	if mp := sp.RabbitMQBrokerParameters; mp != nil && mp.Credentials == nil {
		return fmt.Errorf("%w: RabbitMQBrokerParameters.Credentials is required", ErrValidation)
	}

	return nil
}

func (p *Pipe) effectiveBatchSize() int {
	if p.SourceParameters != nil {
		if bs := sourceBatchSize(p.SourceParameters); bs > 0 {
			return bs
		}
	}

	return pipeDefaultBatchSize
}
