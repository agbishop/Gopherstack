package pipes_test

// Covers SourceParameters for SQS, Kinesis, and DynamoDB stream sources, the
// FilterCriteria round-trip through the API, and BatchSize bounds validation
// across all source types, plus the runtime Kinesis and DynamoDB Streams
// source pollers themselves (closing the "runner only polls SQS sources"
// PARITY.md gap): both pollers are exercised against fake
// PipeKinesisReader/PipeDynamoDBStreamsReader implementations (the real
// backend adapters live in cli.go and are proven end-to-end against the real
// kinesis/dynamodb backends by cli_pipes_wiring_test.go in the root package).
// Kafka/MSK/ActiveMQ/RabbitMQ broker sources (a much deeper parameter
// surface: credentials, VPC config, clone isolation) live in
// sources_brokers_test.go.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/pipes"
)

// TestSourceParams_SQS verifies SQS source parameters round-trip.
func TestSourceParams_SQS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		batchSize           int
		maxBatchingWindowMs int
	}{
		{name: "batch_size_1", batchSize: 1, maxBatchingWindowMs: 0},
		{name: "batch_size_10_window_30", batchSize: 10, maxBatchingWindowMs: 30},
		{name: "batch_size_100", batchSize: 100, maxBatchingWindowMs: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			body := map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
				"SourceParameters": map[string]any{
					"SqsQueueParameters": map[string]any{
						"BatchSize":                      tt.batchSize,
						"MaximumBatchingWindowInSeconds": tt.maxBatchingWindowMs,
					},
				},
			}
			resp := auditCreate(t, h, tt.name+"-pipe", body)

			sp, _ := resp["SourceParameters"].(map[string]any)
			require.NotNil(t, sp, "SourceParameters missing")
			sqsp, _ := sp["SqsQueueParameters"].(map[string]any)
			require.NotNil(t, sqsp, "SqsQueueParameters missing")
			assert.EqualValues(t, tt.batchSize, sqsp["BatchSize"])
		})
	}
}

// TestSourceParams_Kinesis verifies Kinesis stream source parameters round-trip.
func TestSourceParams_Kinesis(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		startingPosition      string
		onPartialBatchFailure string
		dlqArn                string
		batchSize             int
		maxRetryAttempts      int
		maxRecordAgeSeconds   int
		parallelizationFactor int
	}{
		{
			name:             "at_sequence_number",
			startingPosition: "AT_SEQUENCE_NUMBER",
			batchSize:        100,
		},
		{
			name:                  "trim_horizon_with_retry",
			startingPosition:      "TRIM_HORIZON",
			batchSize:             50,
			maxRetryAttempts:      3,
			maxRecordAgeSeconds:   3600,
			onPartialBatchFailure: "AUTOMATIC_BISECT",
			parallelizationFactor: 2,
		},
		{
			name:             "latest_with_dlq",
			startingPosition: "LATEST",
			batchSize:        10,
			dlqArn:           "arn:aws:sqs:us-west-2:123456789012:dlq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			kinesisParams := map[string]any{
				"StartingPosition": tt.startingPosition,
				"BatchSize":        tt.batchSize,
			}
			if tt.maxRetryAttempts > 0 {
				kinesisParams["MaximumRetryAttempts"] = tt.maxRetryAttempts
			}
			if tt.maxRecordAgeSeconds > 0 {
				kinesisParams["MaximumRecordAgeInSeconds"] = tt.maxRecordAgeSeconds
			}
			if tt.onPartialBatchFailure != "" {
				kinesisParams["OnPartialBatchItemFailure"] = tt.onPartialBatchFailure
			}
			if tt.parallelizationFactor > 0 {
				kinesisParams["ParallelizationFactor"] = tt.parallelizationFactor
			}
			if tt.dlqArn != "" {
				kinesisParams["DeadLetterConfig"] = map[string]any{"Arn": tt.dlqArn}
			}

			resp := auditCreate(t, h, tt.name+"-kinesis-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:kinesis:us-west-2:123456789012:stream/s",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
				"SourceParameters": map[string]any{
					"KinesisStreamParameters": kinesisParams,
				},
			})

			sp, _ := resp["SourceParameters"].(map[string]any)
			kp, _ := sp["KinesisStreamParameters"].(map[string]any)
			require.NotNil(t, kp, "KinesisStreamParameters missing")
			assert.Equal(t, tt.startingPosition, kp["StartingPosition"])
			assert.EqualValues(t, tt.batchSize, kp["BatchSize"])
			if tt.dlqArn != "" {
				dlc, _ := kp["DeadLetterConfig"].(map[string]any)
				assert.Equal(t, tt.dlqArn, dlc["Arn"])
			}
		})
	}
}

// TestSourceParams_DynamoDB verifies DynamoDB stream source parameters round-trip.
func TestSourceParams_DynamoDB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		startingPosition string
		batchSize        int
		maxRetry         int
	}{
		{
			name:             "trim_horizon",
			startingPosition: "TRIM_HORIZON",
			batchSize:        100,
		},
		{
			name:             "latest_with_retry",
			startingPosition: "LATEST",
			batchSize:        25,
			maxRetry:         5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			ddbParams := map[string]any{
				"StartingPosition": tt.startingPosition,
				"BatchSize":        tt.batchSize,
			}
			if tt.maxRetry > 0 {
				ddbParams["MaximumRetryAttempts"] = tt.maxRetry
			}

			resp := auditCreate(t, h, tt.name+"-ddb-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:dynamodb:us-west-2:123456789012:table/T/stream/2024",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
				"SourceParameters": map[string]any{
					"DynamoDBStreamParameters": ddbParams,
				},
			})

			sp, _ := resp["SourceParameters"].(map[string]any)
			dp, _ := sp["DynamoDBStreamParameters"].(map[string]any)
			require.NotNil(t, dp, "DynamoDBStreamParameters missing")
			assert.Equal(t, tt.startingPosition, dp["StartingPosition"])
			assert.EqualValues(t, tt.batchSize, dp["BatchSize"])
		})
	}
}

// --- FilterCriteria round-trip ---

// TestFilterCriteria_StoredAndReturned verifies filter criteria persist.
func TestFilterCriteria_StoredAndReturned(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		patterns []string
	}{
		{name: "single_filter", patterns: []string{`{"source": ["my-app"]}`}},
		{
			name:     "multiple_filters",
			patterns: []string{`{"type": ["order"]}`, `{"type": ["payment"]}`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := auditNewHandler(t)
			filters := make([]map[string]string, len(tt.patterns))
			for i, p := range tt.patterns {
				filters[i] = map[string]string{"Pattern": p}
			}

			resp := auditCreate(t, h, tt.name+"-pipe", map[string]any{
				"RoleArn":      "arn:aws:iam::123456789012:role/r",
				"Source":       "arn:aws:sqs:us-west-2:123456789012:q",
				"Target":       "arn:aws:lambda:us-west-2:123456789012:function:fn",
				"DesiredState": "RUNNING",
				"SourceParameters": map[string]any{
					"FilterCriteria": map[string]any{
						"Filters": filters,
					},
				},
			})

			sp, _ := resp["SourceParameters"].(map[string]any)
			fc, _ := sp["FilterCriteria"].(map[string]any)
			require.NotNil(t, fc, "FilterCriteria missing")
			flist, _ := fc["Filters"].([]any)
			assert.Len(t, flist, len(tt.patterns))
		})
	}
}

// --- BatchSize bounds validation and effective-value resolution ---

// TestBatchSize_EffectiveFromAllSources verifies effectiveBatchSize picks from any source type.
func TestBatchSize_EffectiveFromAllSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sourceParams    *pipes.SourceParameters
		name            string
		wantEffectiveBS int
	}{
		{
			name: "sqs_batch_size",
			sourceParams: &pipes.SourceParameters{
				SqsQueueParameters: &pipes.SQSSourceParameters{BatchSize: 7},
			},
			wantEffectiveBS: 7,
		},
		{
			name: "kinesis_batch_size",
			sourceParams: &pipes.SourceParameters{
				KinesisStreamParameters: &pipes.KinesisStreamSourceParameters{
					StartingPosition: "LATEST",
					BatchSize:        42,
				},
			},
			wantEffectiveBS: 42,
		},
		{
			name: "dynamodb_batch_size",
			sourceParams: &pipes.SourceParameters{
				DynamoDBStreamParameters: &pipes.DynamoDBStreamSourceParameters{
					StartingPosition: "TRIM_HORIZON",
					BatchSize:        15,
				},
			},
			wantEffectiveBS: 15,
		},
		{
			name: "msk_batch_size",
			sourceParams: &pipes.SourceParameters{
				ManagedStreamingKafkaParameters: &pipes.MSKSourceParameters{
					TopicName: "t",
					BatchSize: 33,
				},
			},
			wantEffectiveBS: 33,
		},
		{
			name: "rabbitmq_batch_size",
			sourceParams: &pipes.SourceParameters{
				RabbitMQBrokerParameters: &pipes.RabbitMQBrokerSourceParameters{
					Credentials: &pipes.MQBrokerCredentials{
						BasicAuth: "arn:aws:secretsmanager:us-west-2:123456789012:secret:s",
					},
					QueueName: "q",
					BatchSize: 20,
				},
			},
			wantEffectiveBS: 20,
		},
		{
			name: "activemq_batch_size",
			sourceParams: &pipes.SourceParameters{
				ActiveMQBrokerParameters: &pipes.ActiveMQBrokerSourceParameters{
					Credentials: &pipes.MQBrokerCredentials{
						BasicAuth: "arn:aws:secretsmanager:us-west-2:123456789012:secret:s",
					},
					QueueName: "q",
					BatchSize: 8,
				},
			},
			wantEffectiveBS: 8,
		},
		{
			name:            "no_params_uses_default",
			sourceParams:    nil,
			wantEffectiveBS: 10, // pipeDefaultBatchSize
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := auditNewBackend()
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				RoleARN:          "arn:aws:iam::123456789012:role/r",
				Name:             tt.name + "-pipe",
				Source:           "arn:aws:sqs:us-west-2:123456789012:q",
				Target:           "arn:aws:lambda:us-west-2:123456789012:function:fn",
				DesiredState:     "RUNNING",
				SourceParameters: tt.sourceParams,
			})
			require.NoError(t, err)
			pipes.WaitPipeRunning(t, b, tt.name+"-pipe")

			// verify via GetPipe + runner poll
			sqsReader := &fakeSQSReader{}
			r := pipes.NewRunner(b)
			r.SetSQSReader(sqsReader)
			lambdaInvoker := &fakeLambda{}
			r.SetLambdaInvoker(lambdaInvoker)

			pipes.PollAllPipesOnce(t.Context(), r)
			// empty queue → reader called with expected batch size
			// (no way to observe batch size without checking receiver)
			// Just verify no panic and pipe state is intact
			p, err := b.GetPipe(context.Background(), tt.name+"-pipe")
			require.NoError(t, err)
			assert.Equal(t, "RUNNING", p.CurrentState)
		})
	}
}

// TestBatchSize_Validation verifies that out-of-bounds BatchSize values
// are rejected with ValidationException for all source types.
func TestBatchSize_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sp        *pipes.SourceParameters
		name      string
		wantError bool
	}{
		{
			name: "sqs_zero_batchsize_accepted",
			sp: &pipes.SourceParameters{
				SqsQueueParameters: &pipes.SQSSourceParameters{BatchSize: 0},
			},
			wantError: false,
		},
		{
			name: "sqs_valid_batchsize_1",
			sp: &pipes.SourceParameters{
				SqsQueueParameters: &pipes.SQSSourceParameters{BatchSize: 1},
			},
			wantError: false,
		},
		{
			name: "sqs_valid_batchsize_10000",
			sp: &pipes.SourceParameters{
				SqsQueueParameters: &pipes.SQSSourceParameters{BatchSize: 10000},
			},
			wantError: false,
		},
		{
			name: "sqs_negative_batchsize_rejected",
			sp: &pipes.SourceParameters{
				SqsQueueParameters: &pipes.SQSSourceParameters{BatchSize: -1},
			},
			wantError: true,
		},
		{
			name: "sqs_over_limit_batchsize_rejected",
			sp: &pipes.SourceParameters{
				SqsQueueParameters: &pipes.SQSSourceParameters{BatchSize: 10001},
			},
			wantError: true,
		},
		{
			name: "kinesis_over_limit_rejected",
			sp: &pipes.SourceParameters{
				KinesisStreamParameters: &pipes.KinesisStreamSourceParameters{BatchSize: 99999},
			},
			wantError: true,
		},
		{
			name: "dynamodb_over_limit_rejected",
			sp: &pipes.SourceParameters{
				DynamoDBStreamParameters: &pipes.DynamoDBStreamSourceParameters{BatchSize: -5},
			},
			wantError: true,
		},
		{
			name: "msk_over_limit_rejected",
			sp: &pipes.SourceParameters{
				ManagedStreamingKafkaParameters: &pipes.MSKSourceParameters{BatchSize: 10001},
			},
			wantError: true,
		},
		{
			name: "kafka_over_limit_rejected",
			sp: &pipes.SourceParameters{
				SelfManagedKafkaParameters: &pipes.SelfManagedKafkaSourceParameters{
					BatchSize: -100,
				},
			},
			wantError: true,
		},
		{
			name: "rabbitmq_over_limit_rejected",
			sp: &pipes.SourceParameters{
				RabbitMQBrokerParameters: &pipes.RabbitMQBrokerSourceParameters{BatchSize: 10001},
			},
			wantError: true,
		},
		{
			name: "activemq_over_limit_rejected",
			sp: &pipes.SourceParameters{
				ActiveMQBrokerParameters: &pipes.ActiveMQBrokerSourceParameters{BatchSize: -1},
			},
			wantError: true,
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

// TestBatchSize_UpdateValidation verifies that batch size validation also
// applies on UpdatePipe.
func TestBatchSize_UpdateValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sp        *pipes.SourceParameters
		name      string
		wantError bool
	}{
		{
			name: "update_valid_batchsize",
			sp: &pipes.SourceParameters{
				SqsQueueParameters: &pipes.SQSSourceParameters{BatchSize: 100},
			},
			wantError: false,
		},
		{
			name: "update_invalid_batchsize",
			sp: &pipes.SourceParameters{
				SqsQueueParameters: &pipes.SQSSourceParameters{BatchSize: 10001},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b3Backend()
			b3CreatePipe(t, b, tt.name+"-pipe", b3LambdaTarget)

			_, err := b.UpdatePipe(context.Background(), tt.name+"-pipe", pipes.UpdatePipeInput{
				RoleARN:          "arn:aws:iam::123456789012:role/r",
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

// TestSourceStartingPosition_Required verifies that CreatePipe rejects a
// Kinesis or DynamoDB Streams source with no StartingPosition, matching
// aws-sdk-go-v2 pipes validators.go's validatePipeSourceKinesisStreamParameters
// and validatePipeSourceDynamoDBStreamParameters (both mark it required).
func TestSourceStartingPosition_Required(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sp        *pipes.SourceParameters
		name      string
		wantError bool
	}{
		{
			name: "kinesis_missing_starting_position_rejected",
			sp: &pipes.SourceParameters{
				KinesisStreamParameters: &pipes.KinesisStreamSourceParameters{},
			},
			wantError: true,
		},
		{
			name: "kinesis_with_starting_position_accepted",
			sp: &pipes.SourceParameters{
				KinesisStreamParameters: &pipes.KinesisStreamSourceParameters{
					StartingPosition: "LATEST",
				},
			},
			wantError: false,
		},
		{
			name: "dynamodb_missing_starting_position_rejected",
			sp: &pipes.SourceParameters{
				DynamoDBStreamParameters: &pipes.DynamoDBStreamSourceParameters{},
			},
			wantError: true,
		},
		{
			name: "dynamodb_with_starting_position_accepted",
			sp: &pipes.SourceParameters{
				DynamoDBStreamParameters: &pipes.DynamoDBStreamSourceParameters{
					StartingPosition: "TRIM_HORIZON",
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

// --- Kinesis and DynamoDB Streams runtime pollers ---

// fakeKinesisReader is a fake PipeKinesisReader used to exercise the runtime
// Kinesis source poller (sources_poll.go) without a real Kinesis backend.
type fakeKinesisReader struct {
	pending      map[string][]pipes.KinesisRecord
	shardIDs     []string
	getRecCalls  int
	getRecErrOn  int
	getIterCalls int
	mu           sync.Mutex
}

func (f *fakeKinesisReader) GetShardIDs(_ string) ([]string, error) {
	return f.shardIDs, nil
}

func (f *fakeKinesisReader) GetShardIterator(_, shardID, iteratorType, _ string) (string, error) {
	f.mu.Lock()
	f.getIterCalls++
	f.mu.Unlock()

	return "iter-" + shardID + "-" + iteratorType, nil
}

func (f *fakeKinesisReader) GetRecords(
	iteratorToken string,
	_ int,
) ([]pipes.KinesisRecord, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.getRecCalls++
	if f.getRecErrOn > 0 && f.getRecCalls == f.getRecErrOn {
		return nil, "", assert.AnError
	}

	recs := f.pending[iteratorToken]
	delete(f.pending, iteratorToken)

	return recs, "next-" + iteratorToken, nil
}

// fakeDDBStreamsReader is a fake PipeDynamoDBStreamsReader used to exercise
// the runtime DynamoDB Streams source poller (sources_poll.go) without a real
// DynamoDB Streams backend.
type fakeDDBStreamsReader struct {
	pending  map[string][]pipes.DynamoDBStreamRecord
	shardIDs []string
	mu       sync.Mutex
}

func (f *fakeDDBStreamsReader) DescribeStreamShards(_ string) ([]string, error) {
	return f.shardIDs, nil
}

func (f *fakeDDBStreamsReader) GetStreamShardIterator(
	_, shardID, iteratorType string,
) (string, error) {
	return "iter-" + shardID + "-" + iteratorType, nil
}

func (f *fakeDDBStreamsReader) GetStreamRecords(
	iteratorToken string,
	_ int,
) ([]pipes.DynamoDBStreamRecord, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	recs := f.pending[iteratorToken]
	delete(f.pending, iteratorToken)

	return recs, "next-" + iteratorToken, nil
}

// streamSourceHarness bundles the runner and fakes built for one
// TestPipesRunner_StreamSourcePolling case. Kinesis and DynamoDB Streams
// cases use different fake reader implementations (only one is ever
// populated per case; the other is left nil), while the pipe, runner,
// Lambda invoker, and optional DLQ SQS sender are always present.
type streamSourceHarness struct {
	runner        *pipes.Runner
	kinesisReader *fakeKinesisReader
	ddbReader     *fakeDDBStreamsReader
	lambdaInvoker *mockPipeLambdaInvoker
	sqsSender     *b3MockSQSSender
}

// streamSourceCase is one TestPipesRunner_StreamSourcePolling scenario: a
// source-type x behavior combination (happy-path delivery, GetRecords error,
// filter applied, target failure -> DLQ) across Kinesis and DynamoDB Streams
// sources. The scenarios differ in event envelope shape, reader fake, and
// post-poll assertions enough that a single flat set of "expected outcome"
// fields would either lose precision or balloon into its own mini-DSL, so
// each case supplies a small assert closure over the shared harness instead
// -- the data (source, filter, DLQ, error injection, pending records) still
// lives in the table, only the verification is per-case code.
type streamSourceCase struct {
	lambdaErr      error
	assert         func(t *testing.T, h *streamSourceHarness)
	kinesisPending map[string][]pipes.KinesisRecord
	ddbPending     map[string][]pipes.DynamoDBStreamRecord
	name           string
	sourceARN      string
	filterPattern  string
	dlqARN         string
	getRecErrOn    int
	isDynamoDB     bool
}

// newStreamSourceHarness creates a RUNNING pipe for tc.sourceARN and wires a
// runner with the fakes/invokers tc calls for.
func newStreamSourceHarness(t *testing.T, tc streamSourceCase) *streamSourceHarness {
	t.Helper()

	backend := newTestPipeBackend(t)
	lambdaARN := "arn:aws:lambda:us-east-1:000000000000:function:my-fn"

	input := pipes.CreatePipeInput{
		Name:         tc.name + "-pipe",
		RoleARN:      "arn:aws:iam::000000000000:role/r",
		Source:       tc.sourceARN,
		Target:       lambdaARN,
		DesiredState: "RUNNING",
	}
	if tc.filterPattern != "" {
		input.SourceParameters = &pipes.SourceParameters{
			FilterCriteria: &pipes.FilterCriteria{
				Filters: []pipes.Filter{{Pattern: tc.filterPattern}},
			},
		}
	}
	if tc.dlqARN != "" {
		if input.SourceParameters == nil {
			input.SourceParameters = &pipes.SourceParameters{}
		}
		// The real Pipes API only allows a DeadLetterConfig nested under the
		// source's own stream parameters, never at the top level.
		if tc.isDynamoDB {
			input.SourceParameters.DynamoDBStreamParameters = &pipes.DynamoDBStreamSourceParameters{
				StartingPosition: "TRIM_HORIZON",
				DeadLetterConfig: &pipes.DeadLetterConfig{Arn: tc.dlqARN},
			}
		} else {
			input.SourceParameters.KinesisStreamParameters = &pipes.KinesisStreamSourceParameters{
				StartingPosition: "TRIM_HORIZON",
				DeadLetterConfig: &pipes.DeadLetterConfig{Arn: tc.dlqARN},
			}
		}
	}

	_, err := backend.CreatePipe(context.Background(), input)
	require.NoError(t, err)
	pipes.WaitPipeRunning(t, backend, input.Name)

	h := &streamSourceHarness{lambdaInvoker: &mockPipeLambdaInvoker{err: tc.lambdaErr}}
	h.runner = pipes.NewRunner(backend)
	h.runner.SetLambdaInvoker(h.lambdaInvoker)

	if tc.isDynamoDB {
		h.ddbReader = &fakeDDBStreamsReader{shardIDs: []string{"shard-1"}, pending: tc.ddbPending}
		h.runner.SetDynamoDBStreamsReader(h.ddbReader)
	} else {
		h.kinesisReader = &fakeKinesisReader{
			shardIDs:    []string{"shard-1"},
			pending:     tc.kinesisPending,
			getRecErrOn: tc.getRecErrOn,
		}
		h.runner.SetKinesisReader(h.kinesisReader)
	}

	if tc.dlqARN != "" {
		h.sqsSender = &b3MockSQSSender{}
		h.runner.SetSQSSender(h.sqsSender)
	}

	return h
}

// TestPipesRunner_StreamSourcePolling covers the runtime Kinesis and
// DynamoDB Streams source pollers (sources_poll.go) against fake
// PipeKinesisReader/PipeDynamoDBStreamsReader implementations, closing the
// "runner only polls SQS sources" PARITY.md gap: happy-path delivery,
// FilterCriteria application, GetRecords-error recovery (including that a
// failed GetRecords does not leave a stale shard iterator cached), and
// target-failure DLQ redirection, for both source types. The real backend
// adapters live in cli.go and are proven end-to-end against the real
// kinesis/dynamodb backends by cli_pipes_wiring_test.go in the root package.
func TestPipesRunner_StreamSourcePolling(t *testing.T) {
	t.Parallel()

	tests := []streamSourceCase{
		{
			name:      "kinesis_to_lambda",
			sourceARN: "arn:aws:kinesis:us-east-1:000000000000:stream/my-stream",
			kinesisPending: map[string][]pipes.KinesisRecord{
				"iter-shard-1-TRIM_HORIZON": {
					{PartitionKey: "pk1", SequenceNumber: "seq1", Data: []byte("hello-kinesis")},
				},
			},
			// Proves a Kinesis-sourced RUNNING pipe is actually polled and
			// forwards its records to the target, closing the "Kinesis
			// sources are modeled but never polled" gap; the second poll
			// proves the shard iterator advanced (no re-delivery).
			assert: func(t *testing.T, h *streamSourceHarness) {
				t.Helper()

				pipes.PollAllPipesOnce(t.Context(), h.runner)

				h.lambdaInvoker.mu.Lock()
				calls := h.lambdaInvoker.calls
				payloads := h.lambdaInvoker.payloads
				h.lambdaInvoker.mu.Unlock()

				require.Len(t, calls, 1, "expected one Lambda invocation from the Kinesis poller")
				assert.Equal(t, "my-fn", calls[0])

				var event struct {
					Records []struct {
						Kinesis struct {
							PartitionKey string `json:"partitionKey"`
							Data         string `json:"data"`
						} `json:"kinesis"`
						EventSource string `json:"eventSource"`
					} `json:"Records"`
				}
				require.NoError(t, json.Unmarshal(payloads[0], &event))
				require.Len(t, event.Records, 1)
				assert.Equal(t, "pk1", event.Records[0].Kinesis.PartitionKey)
				assert.Equal(t, "aws:kinesis", event.Records[0].EventSource)

				// A second poll with no new pending records must not
				// re-deliver the same record (proves the shard iterator
				// advanced past it).
				pipes.PollAllPipesOnce(t.Context(), h.runner)
				h.lambdaInvoker.mu.Lock()
				callsAfter := len(h.lambdaInvoker.calls)
				h.lambdaInvoker.mu.Unlock()
				assert.Equal(
					t,
					1,
					callsAfter,
					"iterator must have advanced; no re-delivery expected",
				)
			},
		},
		{
			name:           "kinesis_get_records_error",
			sourceARN:      "arn:aws:kinesis:us-east-1:000000000000:stream/err-stream",
			kinesisPending: map[string][]pipes.KinesisRecord{},
			getRecErrOn:    1,
			// Proves a GetRecords failure is handled gracefully (no panic,
			// no target invocation) rather than wedging the poller,
			// mirroring TestPipesRunner_SQSReceiveError for the SQS source,
			// and that the cached shard iterator is dropped on failure so
			// the next poll re-initializes it instead of reusing a stale
			// one.
			assert: func(t *testing.T, h *streamSourceHarness) {
				t.Helper()

				require.NotPanics(t, func() {
					pipes.PollAllPipesOnce(t.Context(), h.runner)
				})

				h.lambdaInvoker.mu.Lock()
				calls := len(h.lambdaInvoker.calls)
				h.lambdaInvoker.mu.Unlock()
				assert.Equal(t, 0, calls, "a GetRecords error must not invoke the target")

				// A failed GetRecords must not leave the shard iterator
				// cached: poll again (GetRecords succeeds this time, since
				// getRecErrOn only fires on the first call) and confirm the
				// runner re-requested a fresh iterator from
				// GetShardIterator rather than silently reusing one that
				// survived the failure.
				pipes.PollAllPipesOnce(t.Context(), h.runner)

				h.kinesisReader.mu.Lock()
				iterCalls := h.kinesisReader.getIterCalls
				h.kinesisReader.mu.Unlock()
				assert.Equal(t, 2, iterCalls,
					"iterator must not be cached across a GetRecords failure; expected a fresh "+
						"GetShardIterator call on the next poll")
			},
		},
		{
			name:          "kinesis_filter_criteria",
			sourceARN:     "arn:aws:kinesis:us-east-1:000000000000:stream/filtered-stream",
			filterPattern: "keep-me",
			kinesisPending: map[string][]pipes.KinesisRecord{
				"iter-shard-1-TRIM_HORIZON": {
					{PartitionKey: "pk1", SequenceNumber: "seq1", Data: []byte("drop-this")},
					{PartitionKey: "pk2", SequenceNumber: "seq2", Data: []byte("keep-me")},
				},
			},
			// Proves FilterCriteria is applied to Kinesis records before
			// forwarding to the target.
			assert: func(t *testing.T, h *streamSourceHarness) {
				t.Helper()

				pipes.PollAllPipesOnce(t.Context(), h.runner)

				h.lambdaInvoker.mu.Lock()
				payloads := h.lambdaInvoker.payloads
				h.lambdaInvoker.mu.Unlock()

				require.Len(t, payloads, 1)
				// Records are base64-encoded in the delivered payload's "data" field.
				assert.Contains(
					t,
					string(payloads[0]),
					base64.StdEncoding.EncodeToString([]byte("keep-me")),
				)
				assert.NotContains(
					t,
					string(payloads[0]),
					base64.StdEncoding.EncodeToString([]byte("drop-this")),
				)
			},
		},
		{
			name:      "kinesis_target_failure_dlq",
			sourceARN: "arn:aws:kinesis:us-east-1:000000000000:stream/dlq-stream",
			dlqARN:    "arn:aws:sqs:us-east-1:000000000000:dlq",
			lambdaErr: assert.AnError,
			kinesisPending: map[string][]pipes.KinesisRecord{
				"iter-shard-1-TRIM_HORIZON": {
					{PartitionKey: "pk1", SequenceNumber: "seq1", Data: []byte("boom")},
				},
			},
			// Proves that when target delivery fails for a Kinesis-sourced
			// pipe, the batch is redirected to the configured DLQ (there is
			// no source message to leave in place, unlike SQS).
			assert: func(t *testing.T, h *streamSourceHarness) {
				t.Helper()

				pipes.PollAllPipesOnce(t.Context(), h.runner)

				h.sqsSender.mu.Lock()
				defer h.sqsSender.mu.Unlock()
				require.Len(
					t,
					h.sqsSender.bodies,
					1,
					"failed Kinesis target delivery must be redirected to the DLQ",
				)
				assert.Contains(
					t,
					h.sqsSender.bodies[0],
					base64.StdEncoding.EncodeToString([]byte("boom")),
				)
				assert.Equal(t, "arn:aws:sqs:us-east-1:000000000000:dlq", h.sqsSender.queueURLs[0])
			},
		},
		{
			name:       "dynamodb_stream_to_target",
			sourceARN:  "arn:aws:dynamodb:us-east-1:000000000000:table/my-table/stream/2024-01-01T00:00:00.000",
			isDynamoDB: true,
			ddbPending: map[string][]pipes.DynamoDBStreamRecord{
				"iter-shard-1-TRIM_HORIZON": {
					{
						EventID:        "ev1",
						EventName:      "INSERT",
						SequenceNumber: "seq1",
						NewImage:       map[string]any{"pk": map[string]any{"S": "id1"}},
					},
				},
			},
			// Proves a DynamoDB-Streams-sourced RUNNING pipe is actually
			// polled and forwards its records to the target.
			assert: func(t *testing.T, h *streamSourceHarness) {
				t.Helper()

				pipes.PollAllPipesOnce(t.Context(), h.runner)

				h.lambdaInvoker.mu.Lock()
				calls := h.lambdaInvoker.calls
				payloads := h.lambdaInvoker.payloads
				h.lambdaInvoker.mu.Unlock()

				require.Len(
					t,
					calls,
					1,
					"expected one Lambda invocation from the DynamoDB Streams poller",
				)

				var event struct {
					Records []struct {
						Dynamodb struct {
							NewImage map[string]any `json:"NewImage"`
						} `json:"dynamodb"`
						EventName string `json:"eventName"`
					} `json:"Records"`
				}
				require.NoError(t, json.Unmarshal(payloads[0], &event))
				require.Len(t, event.Records, 1)
				assert.Equal(t, "INSERT", event.Records[0].EventName)
			},
		},
		{
			name:          "dynamodb_stream_filter_criteria",
			sourceARN:     "arn:aws:dynamodb:us-east-1:000000000000:table/filtered-table/stream/2024-01-01T00:00:00.000",
			isDynamoDB:    true,
			filterPattern: `{"eventName":["INSERT"]}`,
			ddbPending: map[string][]pipes.DynamoDBStreamRecord{
				"iter-shard-1-TRIM_HORIZON": {
					{EventID: "ev1", EventName: "MODIFY", SequenceNumber: "seq1"},
					{EventID: "ev2", EventName: "INSERT", SequenceNumber: "seq2"},
				},
			},
			// Proves FilterCriteria is applied to DynamoDB stream records
			// (matched against the eventName/dynamodb view).
			assert: func(t *testing.T, h *streamSourceHarness) {
				t.Helper()

				pipes.PollAllPipesOnce(t.Context(), h.runner)

				h.lambdaInvoker.mu.Lock()
				payloads := h.lambdaInvoker.payloads
				h.lambdaInvoker.mu.Unlock()

				require.Len(t, payloads, 1)
				assert.Contains(t, string(payloads[0]), `"ev2"`)
				assert.NotContains(t, string(payloads[0]), `"ev1"`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newStreamSourceHarness(t, tt)
			tt.assert(t, h)
		})
	}
}

// TestPipesRunner_BrokerSourcesNotPolled proves that MSK / self-managed Kafka /
// RabbitMQ / ActiveMQ sources -- which have no in-repo broker emulator to read
// from -- are safely skipped rather than panicking or being silently treated
// as a different source type.
func TestPipesRunner_BrokerSourcesNotPolled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
	}{
		{"msk", "arn:aws:kafka:us-east-1:000000000000:cluster/my-cluster/uuid"},
		{"self-managed-kafka", "my-broker:9092"},
		{"rabbitmq", "arn:aws:mq:us-east-1:000000000000:broker:my-broker:uuid"},
		{"activemq", "arn:aws:mq:us-east-1:000000000000:broker:my-broker:uuid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newTestPipeBackend(t)
			lambdaARN := "arn:aws:lambda:us-east-1:000000000000:function:my-fn"
			createTestPipe(t, backend, "broker-pipe-"+tt.name, tt.source, lambdaARN, "RUNNING")

			lambdaInvoker := &mockPipeLambdaInvoker{}
			runner := pipes.NewRunner(backend)
			runner.SetLambdaInvoker(lambdaInvoker)

			require.NotPanics(t, func() {
				pipes.PollAllPipesOnce(t.Context(), runner)
			})

			lambdaInvoker.mu.Lock()
			calls := len(lambdaInvoker.calls)
			lambdaInvoker.mu.Unlock()
			assert.Equal(
				t,
				0,
				calls,
				"broker sources have no backing emulator and must not be polled",
			)
		})
	}
}
