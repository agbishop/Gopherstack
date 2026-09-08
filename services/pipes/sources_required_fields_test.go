package pipes_test

// Covers CreatePipe/UpdatePipe required-field validation for the nested
// source union types not covered by sources_test.go's
// TestSourceStartingPosition_Required: ActiveMQ/RabbitMQ Credentials and
// QueueName, and MSK/SelfManagedKafka TopicName.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/pipes"
)

// TestSourceMQBrokerRequiredFields verifies that CreatePipe rejects ActiveMQ
// and RabbitMQ broker sources missing Credentials or QueueName, matching
// aws-sdk-go-v2 pipes validators.go's validatePipeSourceActiveMQBrokerParameters
// and validatePipeSourceRabbitMQBrokerParameters (both require Credentials
// and QueueName).
func TestSourceMQBrokerRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sp        *pipes.SourceParameters
		name      string
		wantError bool
	}{
		{
			name: "activemq_missing_credentials_rejected",
			sp: &pipes.SourceParameters{
				ActiveMQBrokerParameters: &pipes.ActiveMQBrokerSourceParameters{QueueName: "q"},
			},
			wantError: true,
		},
		{
			name: "activemq_missing_queue_name_rejected",
			sp: &pipes.SourceParameters{
				ActiveMQBrokerParameters: &pipes.ActiveMQBrokerSourceParameters{
					Credentials: &pipes.MQBrokerCredentials{
						BasicAuth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:s",
					},
				},
			},
			wantError: true,
		},
		{
			name: "activemq_complete_accepted",
			sp: &pipes.SourceParameters{
				ActiveMQBrokerParameters: &pipes.ActiveMQBrokerSourceParameters{
					Credentials: &pipes.MQBrokerCredentials{
						BasicAuth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:s",
					},
					QueueName: "q",
				},
			},
			wantError: false,
		},
		{
			name: "rabbitmq_missing_credentials_rejected",
			sp: &pipes.SourceParameters{
				RabbitMQBrokerParameters: &pipes.RabbitMQBrokerSourceParameters{QueueName: "q"},
			},
			wantError: true,
		},
		{
			name: "rabbitmq_missing_queue_name_rejected",
			sp: &pipes.SourceParameters{
				RabbitMQBrokerParameters: &pipes.RabbitMQBrokerSourceParameters{
					Credentials: &pipes.MQBrokerCredentials{
						BasicAuth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:s",
					},
				},
			},
			wantError: true,
		},
		{
			name: "rabbitmq_complete_accepted",
			sp: &pipes.SourceParameters{
				RabbitMQBrokerParameters: &pipes.RabbitMQBrokerSourceParameters{
					Credentials: &pipes.MQBrokerCredentials{
						BasicAuth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:s",
					},
					QueueName: "q",
				},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b3Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				Name:             tt.name + "-pipe",
				RoleARN:          "arn:aws:iam::111122223333:role/r",
				Source:           b3SQSSource,
				Target:           b3LambdaTarget,
				DesiredState:     "RUNNING",
				SourceParameters: tt.sp,
			})

			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestSourceKafkaTopicNameRequired verifies that CreatePipe rejects MSK and
// SelfManagedKafka sources missing TopicName, matching aws-sdk-go-v2 pipes
// validators.go's validatePipeSourceManagedStreamingKafkaParameters and
// validatePipeSourceSelfManagedKafkaParameters (both require TopicName).
func TestSourceKafkaTopicNameRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sp        *pipes.SourceParameters
		name      string
		wantError bool
	}{
		{
			name: "msk_missing_topic_name_rejected",
			sp: &pipes.SourceParameters{
				ManagedStreamingKafkaParameters: &pipes.MSKSourceParameters{},
			},
			wantError: true,
		},
		{
			name: "msk_with_topic_name_accepted",
			sp: &pipes.SourceParameters{
				ManagedStreamingKafkaParameters: &pipes.MSKSourceParameters{TopicName: "t"},
			},
			wantError: false,
		},
		{
			name: "self_managed_kafka_missing_topic_name_rejected",
			sp: &pipes.SourceParameters{
				SelfManagedKafkaParameters: &pipes.SelfManagedKafkaSourceParameters{},
			},
			wantError: true,
		},
		{
			name: "self_managed_kafka_with_topic_name_accepted",
			sp: &pipes.SourceParameters{
				SelfManagedKafkaParameters: &pipes.SelfManagedKafkaSourceParameters{TopicName: "t"},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b3Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				Name:             tt.name + "-pipe",
				RoleARN:          "arn:aws:iam::111122223333:role/r",
				Source:           b3SQSSource,
				Target:           b3LambdaTarget,
				DesiredState:     "RUNNING",
				SourceParameters: tt.sp,
			})

			if tt.wantError {
				require.Error(t, err)
				assert.ErrorIs(t, err, pipes.ErrValidation)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestUpdateSourceMQBrokerCredentialsRequired verifies the Create/Update
// asymmetry for ActiveMQ/RabbitMQ broker sources: UpdatePipe still requires
// Credentials (validateUpdatePipeSourceActiveMQBrokerParameters,
// validateUpdatePipeSourceRabbitMQBrokerParameters) but does NOT require
// QueueName, unlike CreatePipe -- QueueName cannot be changed after creation.
func TestUpdateSourceMQBrokerCredentialsRequired(t *testing.T) {
	t.Parallel()

	t.Run("activemq_update_missing_credentials_rejected", func(t *testing.T) {
		t.Parallel()

		b := b3Backend()
		_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
			Name:         "activemq-update-pipe",
			RoleARN:      "arn:aws:iam::111122223333:role/r",
			Source:       b3SQSSource,
			Target:       b3LambdaTarget,
			DesiredState: "RUNNING",
		})
		require.NoError(t, err)

		_, err = b.UpdatePipe(context.Background(), "activemq-update-pipe", pipes.UpdatePipeInput{
			RoleARN: "arn:aws:iam::111122223333:role/r",
			SourceParameters: &pipes.SourceParameters{
				ActiveMQBrokerParameters: &pipes.ActiveMQBrokerSourceParameters{},
			},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, pipes.ErrValidation)
	})

	t.Run("activemq_update_credentials_without_queue_name_accepted", func(t *testing.T) {
		t.Parallel()

		b := b3Backend()
		_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
			Name:         "activemq-update-pipe2",
			RoleARN:      "arn:aws:iam::111122223333:role/r",
			Source:       b3SQSSource,
			Target:       b3LambdaTarget,
			DesiredState: "RUNNING",
		})
		require.NoError(t, err)

		_, err = b.UpdatePipe(context.Background(), "activemq-update-pipe2", pipes.UpdatePipeInput{
			RoleARN: "arn:aws:iam::111122223333:role/r",
			SourceParameters: &pipes.SourceParameters{
				ActiveMQBrokerParameters: &pipes.ActiveMQBrokerSourceParameters{
					Credentials: &pipes.MQBrokerCredentials{
						BasicAuth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:s",
					},
				},
			},
		})
		assert.NoError(t, err)
	})

	t.Run("rabbitmq_update_missing_credentials_rejected", func(t *testing.T) {
		t.Parallel()

		b := b3Backend()
		_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
			Name:         "rabbitmq-update-pipe",
			RoleARN:      "arn:aws:iam::111122223333:role/r",
			Source:       b3SQSSource,
			Target:       b3LambdaTarget,
			DesiredState: "RUNNING",
		})
		require.NoError(t, err)

		_, err = b.UpdatePipe(context.Background(), "rabbitmq-update-pipe", pipes.UpdatePipeInput{
			RoleARN: "arn:aws:iam::111122223333:role/r",
			SourceParameters: &pipes.SourceParameters{
				RabbitMQBrokerParameters: &pipes.RabbitMQBrokerSourceParameters{},
			},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, pipes.ErrValidation)
	})
}
