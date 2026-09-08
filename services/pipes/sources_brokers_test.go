package pipes_test

// Covers the broker-style sources (MSK, self-managed Kafka, RabbitMQ,
// ActiveMQ): credentials, VPC config, starting position/consumer group,
// FilterCriteria multi-pattern matching, clone isolation, updates, and
// combined source+target round trips through the HTTP handler.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/pipes"
)

// TestSourceParams_MSK verifies Managed Streaming Kafka source parameters round-trip.
func TestSourceParams_MSK(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		topicName        string
		startingPosition string
		consumerGroupID  string
		batchSize        int
	}{
		{
			name:             "trim_horizon_no_group",
			topicName:        "events",
			startingPosition: "TRIM_HORIZON",
			batchSize:        100,
		},
		{
			name:             "latest_with_consumer_group",
			topicName:        "orders",
			startingPosition: "LATEST",
			batchSize:        50,
			consumerGroupID:  "my-group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			mskParams := map[string]any{
				"TopicName":        tt.topicName,
				"StartingPosition": tt.startingPosition,
				"BatchSize":        tt.batchSize,
			}
			if tt.consumerGroupID != "" {
				mskParams["ConsumerGroupId"] = tt.consumerGroupID
			}

			resp := auditCreate(t, h, tt.name+"-msk-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:kafka:us-west-2:123456789012:cluster/msk/uuid",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
				"SourceParameters": map[string]any{
					"ManagedStreamingKafkaParameters": mskParams,
				},
			})

			sp, _ := resp["SourceParameters"].(map[string]any)
			mp, _ := sp["ManagedStreamingKafkaParameters"].(map[string]any)
			require.NotNil(t, mp, "ManagedStreamingKafkaParameters missing")
			assert.Equal(t, tt.topicName, mp["TopicName"])
			assert.Equal(t, tt.startingPosition, mp["StartingPosition"])
			assert.EqualValues(t, tt.batchSize, mp["BatchSize"])
		})
	}
}

// TestSourceParams_SelfManagedKafka verifies self-managed Kafka source parameters.
func TestSourceParams_SelfManagedKafka(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                       string
		topicName                  string
		consumerGroupID            string
		additionalBootstrapServers []string
		batchSize                  int
	}{
		{
			name:      "basic_kafka",
			topicName: "my-topic",
			batchSize: 100,
		},
		{
			name:                       "kafka_with_extras",
			topicName:                  "audit-topic",
			batchSize:                  25,
			additionalBootstrapServers: []string{"broker1:9092", "broker2:9092"},
			consumerGroupID:            "audit-group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			kafkaParams := map[string]any{
				"TopicName": tt.topicName,
				"BatchSize": tt.batchSize,
			}
			if len(tt.additionalBootstrapServers) > 0 {
				kafkaParams["AdditionalBootstrapServers"] = tt.additionalBootstrapServers
			}
			if tt.consumerGroupID != "" {
				kafkaParams["ConsumerGroupId"] = tt.consumerGroupID
			}

			resp := auditCreate(t, h, tt.name+"-kafka-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "smk://broker1:9092",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
				"SourceParameters": map[string]any{
					"SelfManagedKafkaParameters": kafkaParams,
				},
			})

			sp, _ := resp["SourceParameters"].(map[string]any)
			kp, _ := sp["SelfManagedKafkaParameters"].(map[string]any)
			require.NotNil(t, kp, "SelfManagedKafkaParameters missing")
			assert.Equal(t, tt.topicName, kp["TopicName"])
			assert.EqualValues(t, tt.batchSize, kp["BatchSize"])
		})
	}
}

// TestSourceParams_RabbitMQ verifies RabbitMQ broker source parameters.
func TestSourceParams_RabbitMQ(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		queueName   string
		virtualHost string
		batchSize   int
	}{
		{
			name:      "basic_rabbitmq",
			queueName: "my-queue",
			batchSize: 100,
		},
		{
			name:        "rabbitmq_with_vhost",
			queueName:   "orders",
			virtualHost: "/prod",
			batchSize:   10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			rmqParams := map[string]any{
				"QueueName": tt.queueName,
				"BatchSize": tt.batchSize,
				"Credentials": map[string]any{
					"BasicAuth": "arn:aws:secretsmanager:us-west-2:123456789012:secret:s",
				},
			}
			if tt.virtualHost != "" {
				rmqParams["VirtualHost"] = tt.virtualHost
			}

			resp := auditCreate(t, h, tt.name+"-rmq-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:mq:us-west-2:123456789012:broker:rmq:b-uuid",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
				"SourceParameters": map[string]any{
					"RabbitMQBrokerParameters": rmqParams,
				},
			})

			sp, _ := resp["SourceParameters"].(map[string]any)
			rp, _ := sp["RabbitMQBrokerParameters"].(map[string]any)
			require.NotNil(t, rp, "RabbitMQBrokerParameters missing")
			assert.Equal(t, tt.queueName, rp["QueueName"])
			assert.EqualValues(t, tt.batchSize, rp["BatchSize"])
			if tt.virtualHost != "" {
				assert.Equal(t, tt.virtualHost, rp["VirtualHost"])
			}
		})
	}
}

// TestSourceParams_ActiveMQ verifies ActiveMQ broker source parameters.
func TestSourceParams_ActiveMQ(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		queueName string
		batchSize int
	}{
		{name: "basic_activemq", queueName: "my-queue", batchSize: 100},
		{name: "activemq_large_batch", queueName: "orders", batchSize: 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			resp := auditCreate(t, h, tt.name+"-amq-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:mq:us-west-2:123456789012:broker:amq:b-uuid",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
				"SourceParameters": map[string]any{
					"ActiveMQBrokerParameters": map[string]any{
						"QueueName": tt.queueName,
						"BatchSize": tt.batchSize,
						"Credentials": map[string]any{
							"BasicAuth": "arn:aws:secretsmanager:us-west-2:123456789012:secret:s",
						},
					},
				},
			})

			sp, _ := resp["SourceParameters"].(map[string]any)
			ap, _ := sp["ActiveMQBrokerParameters"].(map[string]any)
			require.NotNil(t, ap, "ActiveMQBrokerParameters missing")
			assert.Equal(t, tt.queueName, ap["QueueName"])
			assert.EqualValues(t, tt.batchSize, ap["BatchSize"])
		})
	}
}

// --- Self-managed Kafka source tests ---

// TestSelfManagedKafka_Credentials verifies all 4 credential types.
func TestSelfManagedKafka_Credentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		credKey   string
		credValue string
	}{
		{
			name:      "basic_auth",
			credKey:   "BasicAuth",
			credValue: "arn:aws:secretsmanager:us-east-1:123456789012:secret:basic",
		},
		{
			name:      "client_cert_tls",
			credKey:   "ClientCertificateTlsAuth",
			credValue: "arn:aws:secretsmanager:us-east-1:123456789012:secret:tls",
		},
		{
			name:      "sasl_scram_256",
			credKey:   "SaslScram256Auth",
			credValue: "arn:aws:secretsmanager:us-east-1:123456789012:secret:scram256",
		},
		{
			name:      "sasl_scram_512",
			credKey:   "SaslScram512Auth",
			credValue: "arn:aws:secretsmanager:us-east-1:123456789012:secret:scram512",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:kafka:us-east-1:123456789012:cluster/mycluster/topic",
				"Target":  b2LambdaTarget,
				"SourceParameters": map[string]any{
					"SelfManagedKafkaParameters": map[string]any{
						"TopicName": "my-topic",
						"Credentials": map[string]any{
							tt.credKey: tt.credValue,
						},
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			sp := resp["SourceParameters"].(map[string]any)
			smk := sp["SelfManagedKafkaParameters"].(map[string]any)
			creds, ok := smk["Credentials"].(map[string]any)
			require.True(t, ok, "Credentials should be object")
			assert.Equal(t, tt.credValue, creds[tt.credKey])
		})
	}
}

// TestSelfManagedKafka_Vpc verifies VPC config for self-managed Kafka.
func TestSelfManagedKafka_Vpc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		subnets       []any
		securityGroup []any
	}{
		{
			name:          "single_subnet",
			subnets:       []any{"subnet-aaa"},
			securityGroup: []any{"sg-111"},
		},
		{
			name:          "multi_subnet",
			subnets:       []any{"subnet-aaa", "subnet-bbb", "subnet-ccc"},
			securityGroup: []any{"sg-111", "sg-222"},
		},
		{
			name:          "no_security_group",
			subnets:       []any{"subnet-ddd"},
			securityGroup: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			vpcConf := map[string]any{"Subnets": tt.subnets}
			if tt.securityGroup != nil {
				vpcConf["SecurityGroup"] = tt.securityGroup
			}
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:kafka:us-east-1:123456789012:cluster/mycluster/topic",
				"Target":  b2LambdaTarget,
				"SourceParameters": map[string]any{
					"SelfManagedKafkaParameters": map[string]any{
						"TopicName": "my-topic",
						"Vpc":       vpcConf,
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			sp := resp["SourceParameters"].(map[string]any)
			smk := sp["SelfManagedKafkaParameters"].(map[string]any)
			vpc, ok := smk["Vpc"].(map[string]any)
			require.True(t, ok, "Vpc should be object")
			subnets, ok := vpc["Subnets"].([]any)
			require.True(t, ok)
			assert.Len(t, subnets, len(tt.subnets))
		})
	}
}

// TestSelfManagedKafka_StartingPosition verifies StartingPosition variants.
func TestSelfManagedKafka_StartingPosition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		startingPosition string
	}{
		{name: "trim_horizon", startingPosition: "TRIM_HORIZON"},
		{name: "latest", startingPosition: "LATEST"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  "arn:aws:kafka:us-east-1:123456789012:cluster/c/t",
				Target:  b2LambdaTarget,
				SourceParameters: &pipes.SourceParameters{
					SelfManagedKafkaParameters: &pipes.SelfManagedKafkaSourceParameters{
						TopicName:        "my-topic",
						StartingPosition: tt.startingPosition,
						ConsumerGroupID:  "my-group",
					},
				},
			})
			require.NoError(t, err)

			p, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t, tt.startingPosition,
				p.SourceParameters.SelfManagedKafkaParameters.StartingPosition)
			assert.Equal(t, "my-group",
				p.SourceParameters.SelfManagedKafkaParameters.ConsumerGroupID)
		})
	}
}

// TestSelfManagedKafka_BootstrapServers verifies AdditionalBootstrapServers.
func TestSelfManagedKafka_BootstrapServers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		servers []any
		wantLen int
	}{
		{
			name:    "single_server",
			servers: []any{"kafka.example.com:9092"},
			wantLen: 1,
		},
		{
			name:    "multiple_servers",
			servers: []any{"kafka1.example.com:9092", "kafka2.example.com:9092", "kafka3.example.com:9092"},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:kafka:us-east-1:123456789012:cluster/c/t",
				"Target":  b2LambdaTarget,
				"SourceParameters": map[string]any{
					"SelfManagedKafkaParameters": map[string]any{
						"TopicName":                  "my-topic",
						"AdditionalBootstrapServers": tt.servers,
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			sp := resp["SourceParameters"].(map[string]any)
			smk := sp["SelfManagedKafkaParameters"].(map[string]any)
			servers, ok := smk["AdditionalBootstrapServers"].([]any)
			require.True(t, ok)
			assert.Len(t, servers, tt.wantLen)
			assert.Equal(t, tt.servers[0], servers[0])
		})
	}
}

// --- MSK source tests ---

// TestMSK_Credentials verifies both MSK credential types.
func TestMSK_Credentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		credKey   string
		credValue string
	}{
		{
			name:      "client_cert_tls",
			credKey:   "ClientCertificateTlsAuth",
			credValue: "arn:aws:secretsmanager:us-east-1:123456789012:secret:tls",
		},
		{
			name:      "sasl_scram_512",
			credKey:   "SaslScram512Auth",
			credValue: "arn:aws:secretsmanager:us-east-1:123456789012:secret:scram",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:kafka:us-east-1:123456789012:cluster/msk",
				"Target":  b2LambdaTarget,
				"SourceParameters": map[string]any{
					"ManagedStreamingKafkaParameters": map[string]any{
						"TopicName": "my-topic",
						"Credentials": map[string]any{
							tt.credKey: tt.credValue,
						},
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			sp := resp["SourceParameters"].(map[string]any)
			msk := sp["ManagedStreamingKafkaParameters"].(map[string]any)
			creds, ok := msk["Credentials"].(map[string]any)
			require.True(t, ok, "Credentials should be object")
			assert.Equal(t, tt.credValue, creds[tt.credKey])
		})
	}
}

// TestMSK_StartingPositionAndConsumerGroup verifies MSK source fields.
func TestMSK_StartingPositionAndConsumerGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		startingPosition string
		consumerGroupID  string
	}{
		{
			name:             "trim_horizon_with_group",
			startingPosition: "TRIM_HORIZON",
			consumerGroupID:  "my-consumer-group",
		},
		{
			name:             "latest_with_group",
			startingPosition: "LATEST",
			consumerGroupID:  "another-group",
		},
		{
			name:            "no_position_with_group",
			consumerGroupID: "group-only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  "arn:aws:kafka:us-east-1:123456789012:cluster/msk",
				Target:  b2LambdaTarget,
				SourceParameters: &pipes.SourceParameters{
					ManagedStreamingKafkaParameters: &pipes.MSKSourceParameters{
						TopicName:        "my-topic",
						StartingPosition: tt.startingPosition,
						ConsumerGroupID:  tt.consumerGroupID,
					},
				},
			})
			require.NoError(t, err)

			p, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)

			msk := p.SourceParameters.ManagedStreamingKafkaParameters
			assert.Equal(t, tt.startingPosition, msk.StartingPosition)
			assert.Equal(t, tt.consumerGroupID, msk.ConsumerGroupID)
		})
	}
}

// --- ActiveMQ source tests ---

// TestActiveMQ_Credentials verifies ActiveMQ BasicAuth credential.
func TestActiveMQ_Credentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		basicAuth string
		queueName string
	}{
		{
			name:      "basic_auth_cred",
			basicAuth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:amq-creds",
			queueName: "my.queue",
		},
		{
			name:      "different_queue_and_cred",
			basicAuth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:other-creds",
			queueName: "other.queue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:mq:us-east-1:123456789012:broker:mybroker:b-abc",
				"Target":  b2LambdaTarget,
				"SourceParameters": map[string]any{
					"ActiveMQBrokerParameters": map[string]any{
						"QueueName": tt.queueName,
						"Credentials": map[string]any{
							"BasicAuth": tt.basicAuth,
						},
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			sp := resp["SourceParameters"].(map[string]any)
			amq := sp["ActiveMQBrokerParameters"].(map[string]any)
			creds, ok := amq["Credentials"].(map[string]any)
			require.True(t, ok, "Credentials should be object")
			assert.Equal(t, tt.basicAuth, creds["BasicAuth"])
			assert.Equal(t, tt.queueName, amq["QueueName"])
		})
	}
}

// TestActiveMQ_BatchingParams verifies ActiveMQ BatchSize and batching window.
func TestActiveMQ_BatchingParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		batchSize       float64
		batchWindowSecs float64
	}{
		{name: "batch_10", batchSize: 10, batchWindowSecs: 0},
		{name: "batch_100_window_5", batchSize: 100, batchWindowSecs: 5},
		{name: "batch_1_window_30", batchSize: 1, batchWindowSecs: 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			params := map[string]any{
				"QueueName": "test.queue",
				"Credentials": map[string]any{
					"BasicAuth": "arn:aws:secretsmanager:us-east-1:123456789012:secret:cred",
				},
				"BatchSize": tt.batchSize,
			}
			if tt.batchWindowSecs > 0 {
				params["MaximumBatchingWindowInSeconds"] = tt.batchWindowSecs
			}
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:mq:us-east-1:123456789012:broker:mybroker:b-abc",
				"Target":  b2LambdaTarget,
				"SourceParameters": map[string]any{
					"ActiveMQBrokerParameters": params,
				},
			}
			resp := b2Create(t, h, tt.name, body)

			sp := resp["SourceParameters"].(map[string]any)
			amq := sp["ActiveMQBrokerParameters"].(map[string]any)
			assert.InEpsilon(t, tt.batchSize, amq["BatchSize"], 0.01)
		})
	}
}

// --- RabbitMQ source tests ---

// TestRabbitMQ_Credentials verifies RabbitMQ BasicAuth credential.
func TestRabbitMQ_Credentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		basicAuth   string
		queueName   string
		virtualHost string
	}{
		{
			name:        "default_vhost",
			basicAuth:   "arn:aws:secretsmanager:us-east-1:123456789012:secret:rmq-cred",
			queueName:   "my.queue",
			virtualHost: "/",
		},
		{
			name:        "custom_vhost",
			basicAuth:   "arn:aws:secretsmanager:us-east-1:123456789012:secret:rmq-cred-2",
			queueName:   "other.queue",
			virtualHost: "my-vhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:mq:us-east-1:123456789012:broker:rmqbroker:b-xyz",
				"Target":  b2LambdaTarget,
				"SourceParameters": map[string]any{
					"RabbitMQBrokerParameters": map[string]any{
						"QueueName":   tt.queueName,
						"VirtualHost": tt.virtualHost,
						"Credentials": map[string]any{
							"BasicAuth": tt.basicAuth,
						},
					},
				},
			}
			resp := b2Create(t, h, tt.name, body)

			sp := resp["SourceParameters"].(map[string]any)
			rmq := sp["RabbitMQBrokerParameters"].(map[string]any)
			creds, ok := rmq["Credentials"].(map[string]any)
			require.True(t, ok, "Credentials should be object")
			assert.Equal(t, tt.basicAuth, creds["BasicAuth"])
			assert.Equal(t, tt.virtualHost, rmq["VirtualHost"])
		})
	}
}

// TestRabbitMQ_VirtualHost verifies VirtualHost variants.
func TestRabbitMQ_VirtualHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		virtualHost string
	}{
		{name: "root_vhost", virtualHost: "/"},
		{name: "named_vhost", virtualHost: "production"},
		{name: "nested_vhost", virtualHost: "app/production"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  "arn:aws:mq:us-east-1:123456789012:broker:rmq:b-abc",
				Target:  b2LambdaTarget,
				SourceParameters: &pipes.SourceParameters{
					RabbitMQBrokerParameters: &pipes.RabbitMQBrokerSourceParameters{
						QueueName:   "my.queue",
						VirtualHost: tt.virtualHost,
						Credentials: &pipes.MQBrokerCredentials{
							BasicAuth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:cred",
						},
					},
				},
			})
			require.NoError(t, err)

			p, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t, tt.virtualHost, p.SourceParameters.RabbitMQBrokerParameters.VirtualHost)
		})
	}
}

// --- FilterCriteria multi-pattern tests ---

type mockSQSFilter struct {
	msgs []*pipes.SQSMessage
}

func (m *mockSQSFilter) ReceivePipeMessages(_ string, _ int) ([]*pipes.SQSMessage, error) {
	return m.msgs, nil
}

func (m *mockSQSFilter) DeletePipeMessages(_ string, _ []string) error {
	return nil
}

type mockLambdaFilter struct {
	payload []byte
	called  bool
}

func (m *mockLambdaFilter) InvokeFunction(
	_ context.Context,
	_ string,
	_ string,
	payload []byte,
) ([]byte, int, error) {
	m.called = true
	m.payload = payload

	return nil, 200, nil
}

// TestFilterCriteria_MultiplePatterns verifies multiple filter patterns behavior.
func TestFilterCriteria_MultiplePatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		msgBody   string
		patterns  []string
		wantMatch bool
	}{
		{
			name:      "first_pattern_matches",
			patterns:  []string{"ORDER_PLACED", "ORDER_CANCELLED"},
			msgBody:   `{"eventType": "ORDER_PLACED", "orderId": "123"}`,
			wantMatch: true,
		},
		{
			name:      "second_pattern_matches",
			patterns:  []string{"ORDER_PLACED", "ORDER_CANCELLED"},
			msgBody:   `{"eventType": "ORDER_CANCELLED", "orderId": "456"}`,
			wantMatch: true,
		},
		{
			name:      "no_pattern_matches",
			patterns:  []string{"ORDER_PLACED", "ORDER_CANCELLED"},
			msgBody:   `{"eventType": "ORDER_SHIPPED", "orderId": "789"}`,
			wantMatch: false,
		},
		{
			name:      "empty_pattern_matches_all",
			patterns:  []string{""},
			msgBody:   `{"anything": true}`,
			wantMatch: true,
		},
		{
			name:      "three_patterns_last_matches",
			patterns:  []string{"alpha", "beta", "gamma"},
			msgBody:   `{"type": "gamma"}`,
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			filters := make([]pipes.Filter, len(tt.patterns))
			for i, p := range tt.patterns {
				filters[i] = pipes.Filter{Pattern: p}
			}

			b := b2Backend()
			pipeName := tt.name + "-pipe"
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    pipeName,
				Source:  b2SQSSource,
				Target:  b2LambdaTarget,
				SourceParameters: &pipes.SourceParameters{
					FilterCriteria:     &pipes.FilterCriteria{Filters: filters},
					SqsQueueParameters: &pipes.SQSSourceParameters{BatchSize: 10},
				},
			})
			require.NoError(t, err)
			pipes.WaitPipeRunning(t, b, pipeName)

			msgs := []*pipes.SQSMessage{{
				MessageID:     "msg-1",
				ReceiptHandle: "rh-1",
				Body:          tt.msgBody,
			}}

			var mockSQS mockSQSFilter
			mockSQS.msgs = msgs
			r := pipes.NewRunner(b)
			r.SetSQSReader(&mockSQS)

			invoker := &mockLambdaFilter{}
			r.SetLambdaInvoker(invoker)

			pipes.PollAllPipesOnce(context.Background(), r)

			if tt.wantMatch {
				assert.True(t, invoker.called, "expected Lambda to be invoked for matching msg")
			} else {
				assert.False(t, invoker.called, "expected Lambda NOT invoked for non-matching msg")
			}
		})
	}
}

// --- Immutability / clone isolation tests ---

// TestClone_SelfManagedKafkaVpcIsolation verifies Kafka VPC clone isolates slices.
func TestClone_SelfManagedKafkaVpcIsolation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "kafka_vpc_clone_a"},
		{name: "kafka_vpc_clone_b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  "arn:aws:kafka:us-east-1:123456789012:cluster/c/t",
				Target:  b2LambdaTarget,
				SourceParameters: &pipes.SourceParameters{
					SelfManagedKafkaParameters: &pipes.SelfManagedKafkaSourceParameters{
						TopicName: "topic",
						Vpc: &pipes.SelfManagedKafkaVpc{
							SecurityGroup: []string{"sg-original"},
							Subnets:       []string{"subnet-original"},
						},
						Credentials: &pipes.SelfManagedKafkaAccessCredentials{
							BasicAuth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:cred",
						},
					},
				},
			})
			require.NoError(t, err)

			p1, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)

			p1.SourceParameters.SelfManagedKafkaParameters.Vpc.SecurityGroup[0] = "mutated"
			p1.SourceParameters.SelfManagedKafkaParameters.Vpc.Subnets[0] = "mutated"
			p1.SourceParameters.SelfManagedKafkaParameters.Credentials.BasicAuth = "mutated"

			p2, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t, "sg-original",
				p2.SourceParameters.SelfManagedKafkaParameters.Vpc.SecurityGroup[0])
			assert.Equal(t, "subnet-original",
				p2.SourceParameters.SelfManagedKafkaParameters.Vpc.Subnets[0])
			assert.Equal(t, "arn:aws:secretsmanager:us-east-1:123456789012:secret:cred",
				p2.SourceParameters.SelfManagedKafkaParameters.Credentials.BasicAuth)
		})
	}
}

// TestClone_MSKCredentialsIsolation verifies MSK credentials clone isolates.
func TestClone_MSKCredentialsIsolation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "msk_cred_clone_a"},
		{name: "msk_cred_clone_b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  "arn:aws:kafka:us-east-1:123456789012:cluster/msk",
				Target:  b2LambdaTarget,
				SourceParameters: &pipes.SourceParameters{
					ManagedStreamingKafkaParameters: &pipes.MSKSourceParameters{
						TopicName: "topic",
						Credentials: &pipes.MSKAccessCredentials{
							SaslScram512Auth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:orig",
						},
					},
				},
			})
			require.NoError(t, err)

			p1, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			p1.SourceParameters.ManagedStreamingKafkaParameters.Credentials.SaslScram512Auth = "mutated"

			p2, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t,
				"arn:aws:secretsmanager:us-east-1:123456789012:secret:orig",
				p2.SourceParameters.ManagedStreamingKafkaParameters.Credentials.SaslScram512Auth)
		})
	}
}

// TestClone_ActiveMQCredentialsIsolation verifies ActiveMQ clone isolates credentials.
func TestClone_ActiveMQCredentialsIsolation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "amq_clone_a"},
		{name: "amq_clone_b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  "arn:aws:mq:us-east-1:123456789012:broker:b:b-abc",
				Target:  b2LambdaTarget,
				SourceParameters: &pipes.SourceParameters{
					ActiveMQBrokerParameters: &pipes.ActiveMQBrokerSourceParameters{
						QueueName: "q",
						Credentials: &pipes.MQBrokerCredentials{
							BasicAuth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:amq-orig",
						},
					},
				},
			})
			require.NoError(t, err)

			p1, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			p1.SourceParameters.ActiveMQBrokerParameters.Credentials.BasicAuth = "mutated"

			p2, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t,
				"arn:aws:secretsmanager:us-east-1:123456789012:secret:amq-orig",
				p2.SourceParameters.ActiveMQBrokerParameters.Credentials.BasicAuth)
		})
	}
}

// TestClone_RabbitMQCredentialsIsolation verifies RabbitMQ clone isolates credentials.
func TestClone_RabbitMQCredentialsIsolation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "rmq_clone_a"},
		{name: "rmq_clone_b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b2Backend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN: "arn:aws:iam::123456789012:role/r",
				Name:    tt.name,
				Source:  "arn:aws:mq:us-east-1:123456789012:broker:rmq:b-xyz",
				Target:  b2LambdaTarget,
				SourceParameters: &pipes.SourceParameters{
					RabbitMQBrokerParameters: &pipes.RabbitMQBrokerSourceParameters{
						QueueName: "q",
						Credentials: &pipes.MQBrokerCredentials{
							BasicAuth: "arn:aws:secretsmanager:us-east-1:123456789012:secret:rmq-orig",
						},
					},
				},
			})
			require.NoError(t, err)

			p1, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			p1.SourceParameters.RabbitMQBrokerParameters.Credentials.BasicAuth = "mutated"

			p2, err := b.GetPipe(context.Background(), tt.name)
			require.NoError(t, err)
			assert.Equal(t,
				"arn:aws:secretsmanager:us-east-1:123456789012:secret:rmq-orig",
				p2.SourceParameters.RabbitMQBrokerParameters.Credentials.BasicAuth)
		})
	}
}

// --- Update path tests for new fields ---

// TestUpdate_SelfManagedKafkaCredentials verifies credential updates.
func TestUpdate_SelfManagedKafkaCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		initialCredKey string
		initialCredVal string
		updatedCredKey string
		updatedCredVal string
	}{
		{
			name:           "basic_to_scram512",
			initialCredKey: "BasicAuth",
			initialCredVal: "arn:aws:secretsmanager:us-east-1:123456789012:secret:basic",
			updatedCredKey: "SaslScram512Auth",
			updatedCredVal: "arn:aws:secretsmanager:us-east-1:123456789012:secret:scram",
		},
		{
			name:           "tls_to_scram256",
			initialCredKey: "ClientCertificateTlsAuth",
			initialCredVal: "arn:aws:secretsmanager:us-east-1:123456789012:secret:tls",
			updatedCredKey: "SaslScram256Auth",
			updatedCredVal: "arn:aws:secretsmanager:us-east-1:123456789012:secret:scram256",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			b2Create(t, h, tt.name, map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:kafka:us-east-1:123456789012:cluster/c/t",
				"Target":  b2LambdaTarget,
				"SourceParameters": map[string]any{
					"SelfManagedKafkaParameters": map[string]any{
						"TopicName": "topic",
						"Credentials": map[string]any{
							tt.initialCredKey: tt.initialCredVal,
						},
					},
				},
			})

			updated := b2Update(t, h, tt.name, map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"SourceParameters": map[string]any{
					"SelfManagedKafkaParameters": map[string]any{
						"TopicName": "topic",
						"Credentials": map[string]any{
							tt.updatedCredKey: tt.updatedCredVal,
						},
					},
				},
			})

			sp := updated["SourceParameters"].(map[string]any)
			smk := sp["SelfManagedKafkaParameters"].(map[string]any)
			creds := smk["Credentials"].(map[string]any)
			assert.Equal(t, tt.updatedCredVal, creds[tt.updatedCredKey])
		})
	}
}

// --- Combined source + target tests ---

// TestKafkaSourceLambdaTargetRoundtrip verifies full source+target persist via HTTP.
func TestKafkaSourceLambdaTargetRoundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		startingPos     string
		consumerGroupID string
	}{
		{
			name:            "trim_horizon_group_a",
			startingPos:     "TRIM_HORIZON",
			consumerGroupID: "group-a",
		},
		{
			name:            "latest_group_b",
			startingPos:     "LATEST",
			consumerGroupID: "group-b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:kafka:us-east-1:123456789012:cluster/c/t",
				"Target":  b2LambdaTarget,
				"SourceParameters": map[string]any{
					"SelfManagedKafkaParameters": map[string]any{
						"TopicName":        "my-topic",
						"StartingPosition": tt.startingPos,
						"ConsumerGroupId":  tt.consumerGroupID,
						"Credentials": map[string]any{
							"SaslScram512Auth": "arn:aws:secretsmanager:us-east-1:123456789012:secret:s",
						},
						"Vpc": map[string]any{
							"Subnets":       []any{"subnet-aaa"},
							"SecurityGroup": []any{"sg-111"},
						},
					},
				},
				"TargetParameters": map[string]any{
					"LambdaFunctionParameters": map[string]any{
						"InvocationType": "REQUEST_RESPONSE",
					},
				},
			}

			resp := b2Create(t, h, tt.name, body)
			got := b2Describe(t, h, tt.name)

			assert.Equal(t, resp["Name"], got["Name"])
			assert.Equal(t, "CREATING", resp["CurrentState"])

			sp := got["SourceParameters"].(map[string]any)
			smk := sp["SelfManagedKafkaParameters"].(map[string]any)
			assert.Equal(t, tt.startingPos, smk["StartingPosition"])
			assert.Equal(t, tt.consumerGroupID, smk["ConsumerGroupId"])
		})
	}
}

// TestMSKSourceECSTargetRoundtrip verifies MSK source + ECS target roundtrip.
func TestMSKSourceECSTargetRoundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		topicName string
		credType  string
		credVal   string
	}{
		{
			name:      "msk_scram512_ecs",
			topicName: "orders",
			credType:  "SaslScram512Auth",
			credVal:   "arn:aws:secretsmanager:us-east-1:123456789012:secret:scram",
		},
		{
			name:      "msk_tls_ecs",
			topicName: "events",
			credType:  "ClientCertificateTlsAuth",
			credVal:   "arn:aws:secretsmanager:us-east-1:123456789012:secret:tls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := b2Handler(t)
			body := map[string]any{
				"RoleArn": "arn:aws:iam::123456789012:role/r",
				"Source":  "arn:aws:kafka:us-east-1:123456789012:cluster/msk",
				"Target":  b2ECSTarget,
				"SourceParameters": map[string]any{
					"ManagedStreamingKafkaParameters": map[string]any{
						"TopicName": tt.topicName,
						"Credentials": map[string]any{
							tt.credType: tt.credVal,
						},
					},
				},
				"TargetParameters": map[string]any{
					"EcsTaskParameters": map[string]any{
						"TaskDefinitionArn": "arn:aws:ecs:us-east-1:123456789012:task-definition/td:1",
						"LaunchType":        "FARGATE",
						"NetworkConfiguration": map[string]any{
							"AwsvpcConfiguration": map[string]any{
								"Subnets":        []any{"subnet-aaa"},
								"SecurityGroups": []any{"sg-111"},
								"AssignPublicIp": "ENABLED",
							},
						},
					},
				},
			}

			resp := b2Create(t, h, tt.name, body)

			sp := resp["SourceParameters"].(map[string]any)
			msk := sp["ManagedStreamingKafkaParameters"].(map[string]any)
			assert.Equal(t, tt.topicName, msk["TopicName"])
			creds := msk["Credentials"].(map[string]any)
			assert.Equal(t, tt.credVal, creds[tt.credType])

			tp := resp["TargetParameters"].(map[string]any)
			ecs := tp["EcsTaskParameters"].(map[string]any)
			assert.Equal(t, "FARGATE", ecs["LaunchType"])
		})
	}
}
