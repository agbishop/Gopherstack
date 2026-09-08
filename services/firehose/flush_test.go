package firehose_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/firehose"
)

// flushRegion is the AWS region used by the delivery-pipeline tests in this file.
const flushRegion = "us-east-1"

var errLambdaUnavailable = errors.New("lambda unavailable")

// mockS3Storer captures PutObject calls for assertions.
type mockS3Storer struct {
	calls []*mockS3PutCall
}

type mockS3PutCall struct {
	bucket string
	key    string
	body   []byte
}

func (m *mockS3Storer) PutObject(
	_ context.Context,
	input *s3sdk.PutObjectInput,
) (*s3sdk.PutObjectOutput, error) {
	body, _ := io.ReadAll(input.Body)
	m.calls = append(m.calls, &mockS3PutCall{
		bucket: aws.ToString(input.Bucket),
		key:    aws.ToString(input.Key),
		body:   body,
	})

	return &s3sdk.PutObjectOutput{}, nil
}

// mockLambdaInvoker simulates Lambda transformation responses.
type mockLambdaInvoker struct {
	err      error
	response []byte
}

func (m *mockLambdaInvoker) InvokeFunction(
	_ context.Context,
	_ string,
	_ string,
	_ []byte,
) ([]byte, int, error) {
	return m.response, 200, m.err
}

// mockCWLogsBackend simulates the CloudWatch Logs backend a destination's
// CloudWatchLoggingOptions delivers failure events to via SetCWLogsBackend.
type mockCWLogsBackend struct {
	ensured  []string
	putCalls []mockCWLogsPutCall
}

type mockCWLogsPutCall struct {
	group    string
	stream   string
	messages []string
}

func (m *mockCWLogsBackend) EnsureLogGroupAndStream(groupName, streamName string) error {
	m.ensured = append(m.ensured, groupName+"|"+streamName)

	return nil
}

func (m *mockCWLogsBackend) PutLogLines(groupName, streamName string, messages []string) error {
	m.putCalls = append(m.putCalls, mockCWLogsPutCall{group: groupName, stream: streamName, messages: messages})

	return nil
}

func TestS3Delivery_SizeBasedFlush(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "flush-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN: "arn:aws:s3:::my-bucket",
			BufferingHints: &firehose.BufferingHints{
				SizeInMBs:         1,
				IntervalInSeconds: 300,
			},
		},
	})
	require.NoError(t, err)

	// Write just under 1 MB — should not flush yet.
	underLimit := make([]byte, 900*1024)
	require.NoError(t, b.PutRecord(context.TODO(), "flush-stream", underLimit))
	assert.Empty(t, s3mock.calls, "should not flush before size limit")

	// Push over 1 MB — should trigger a flush.
	overLimit := make([]byte, 200*1024)
	require.NoError(t, b.PutRecord(context.TODO(), "flush-stream", overLimit))
	assert.Len(t, s3mock.calls, 1, "should have flushed to S3")
	assert.Equal(t, "my-bucket", s3mock.calls[0].bucket)
}

func TestS3Delivery_FlushAll(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "flush-all-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN: "arn:aws:s3:::flush-bucket",
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.PutRecord(context.TODO(), "flush-all-stream", []byte("hello")))
	b.FlushAll(t.Context())

	require.Len(t, s3mock.calls, 1)
	assert.Equal(t, "flush-bucket", s3mock.calls[0].bucket)
	assert.Contains(t, string(s3mock.calls[0].body), "hello")
}

func TestS3Delivery_GzipCompression(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "gzip-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN:         "arn:aws:s3:::gzip-bucket",
			CompressionFormat: "GZIP",
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.PutRecord(context.TODO(), "gzip-stream", []byte("compressed content")))
	b.FlushAll(t.Context())

	require.Len(t, s3mock.calls, 1)
	// GZIP magic bytes.
	assert.Equal(t, []byte{0x1f, 0x8b}, s3mock.calls[0].body[:2])
}

func TestS3Delivery_NoS3Backend(t *testing.T) {
	t.Parallel()

	// Without S3 backend, records are buffered but no delivery is attempted.
	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "no-s3-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN: "arn:aws:s3:::bucket",
			BufferingHints: &firehose.BufferingHints{
				SizeInMBs: 1,
			},
		},
	})
	require.NoError(t, err)

	// Two records of 512 KB each sum to 1 MB and would trigger a size-based flush —
	// but with no S3 backend wired up, no delivery should be attempted.
	require.NoError(t, b.PutRecord(context.TODO(), "no-s3-stream", make([]byte, 512*1024)))
	require.NoError(t, b.PutRecord(context.TODO(), "no-s3-stream", make([]byte, 512*1024)))
}

func TestLambdaTransformation_OkRecordsDelivered(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	lambdaResponse := `{"records":[` +
		`{"recordId":"r1","result":"Ok","data":"aGVsbG8="},` +
		`{"recordId":"r2","result":"Dropped","data":""},` +
		`{"recordId":"r3","result":"ProcessingFailed","data":""}` +
		`]}`
	lambdaMock := &mockLambdaInvoker{response: []byte(lambdaResponse)}

	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)
	b.SetLambdaBackend(lambdaMock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "lambda-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN: "arn:aws:s3:::transform-bucket",
			ProcessingConfiguration: &firehose.ProcessingConfiguration{
				Enabled: true,
				Processors: []firehose.Processor{
					{
						Type: "Lambda",
						Parameters: []firehose.ProcessorParameter{
							{ParameterName: "LambdaArn", ParameterValue: "my-transform-fn"},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.PutRecord(context.TODO(), "lambda-stream", []byte("input")))
	b.FlushAll(t.Context())

	require.Len(t, s3mock.calls, 1)
	// Only "Ok" record data ("hello" from base64 "aGVsbG8=") should be delivered.
	assert.Contains(t, string(s3mock.calls[0].body), "hello")
}

func TestLambdaTransformation_AllDropped(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	lambdaResponse := `{"records":[{"recordId":"r1","result":"Dropped","data":""}]}`
	lambdaMock := &mockLambdaInvoker{response: []byte(lambdaResponse)}

	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)
	b.SetLambdaBackend(lambdaMock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "drop-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN: "arn:aws:s3:::drop-bucket",
			ProcessingConfiguration: &firehose.ProcessingConfiguration{
				Enabled: true,
				Processors: []firehose.Processor{
					{
						Type: "Lambda",
						Parameters: []firehose.ProcessorParameter{
							{ParameterName: "LambdaArn", ParameterValue: "drop-fn"},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.PutRecord(context.TODO(), "drop-stream", []byte("input")))
	b.FlushAll(t.Context())

	// All records dropped → no S3 delivery.
	assert.Empty(t, s3mock.calls)
}

// TestLambdaTransformation_ErrorRoutesToErrorOutput verifies that a Lambda invocation
// error routes the source records to the S3 error output (under the ErrorOutputPrefix)
// rather than dropping them silently or delivering them to the main destination path.
func TestLambdaTransformation_ErrorRoutesToErrorOutput(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	lambdaMock := &mockLambdaInvoker{err: errLambdaUnavailable}

	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)
	b.SetLambdaBackend(lambdaMock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "err-lambda-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN:         "arn:aws:s3:::err-bucket",
			ErrorOutputPrefix: "errors/",
			ProcessingConfiguration: &firehose.ProcessingConfiguration{
				Enabled: true,
				Processors: []firehose.Processor{
					{
						Type: "Lambda",
						Parameters: []firehose.ProcessorParameter{
							{ParameterName: "LambdaArn", ParameterValue: "my-fn"},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.PutRecord(context.TODO(), "err-lambda-stream", []byte("input")))
	b.FlushAll(t.Context())

	// Lambda error → the original record is routed to the error output, not dropped.
	require.Len(t, s3mock.calls, 1)
	assert.Equal(t, "err-bucket", s3mock.calls[0].bucket)
	assert.Contains(t, s3mock.calls[0].key, "errors/", "failed records must land under ErrorOutputPrefix")
	assert.Contains(t, string(s3mock.calls[0].body), "input")
}

// TestS3Key_IncludesUUIDSuffix verifies the generated S3 object key follows the
// AWS Firehose convention and contains a UUID suffix.
func TestS3Key_IncludesUUIDSuffix(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "key-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN: "arn:aws:s3:::my-bucket",
		},
	})
	require.NoError(t, err)

	err = b.PutRecord(context.TODO(), "key-stream", []byte("hello"))
	require.NoError(t, err)

	b.FlushAll(t.Context())

	require.Len(t, s3mock.calls, 1)
	// Key should have format: {ts}/key-stream-1-{ts}-{uuid}
	key := s3mock.calls[0].key
	assert.Contains(t, key, "key-stream-1-")
	// UUID is 36 chars; key ends with a UUID segment
	assert.Greater(t, len(key), 36, "key should contain UUID: %s", key)
}

// TestS3Delivery_KeyFormat verifies the S3 key follows the AWS Firehose convention:
// {prefix}{yyyy/MM/dd/HH/}{stream}-1-{ts}-{uuid} (exercised via the handler).
func TestS3Delivery_KeyFormat(t *testing.T) {
	t.Parallel()

	h, s3mock := auditHandler(t)

	auditCreateStream(t, h, "key-stream", map[string]any{
		"S3DestinationConfiguration": map[string]any{
			"BucketARN": "arn:aws:s3:::key-bucket",
			"Prefix":    "my-prefix/",
		},
	})

	auditPut(t, h, "key-stream", "hello")
	h.Backend.(*firehose.InMemoryBackend).FlushAll(t.Context())

	require.Len(t, s3mock.calls, 1)
	key := s3mock.calls[0].key

	assert.Equal(t, "key-bucket", s3mock.calls[0].bucket)
	assert.True(t, strings.HasPrefix(key, "my-prefix/"), "key must start with prefix; got %q", key)
	assert.Contains(t, key, "key-stream", "key must contain stream name")
	// date segment: 4 digit year followed by /MM/dd/HH/
	assert.Regexp(t, `\d{4}/\d{2}/\d{2}/\d{2}/`, key, "key must contain date path")
}

// TestS3Delivery_GZIPCompression_ViaHandler verifies that records delivered
// with CompressionFormat=GZIP are gzip-compressed and contain the original content.
func TestS3Delivery_GZIPCompression_ViaHandler(t *testing.T) {
	t.Parallel()

	h, s3mock := auditHandler(t)

	auditCreateStream(t, h, "gzip-stream", map[string]any{
		"S3DestinationConfiguration": map[string]any{
			"BucketARN":         "arn:aws:s3:::gzip-bucket",
			"CompressionFormat": "GZIP",
		},
	})

	auditPut(t, h, "gzip-stream", "compressed-record")
	h.Backend.(*firehose.InMemoryBackend).FlushAll(t.Context())

	require.Len(t, s3mock.calls, 1)
	body := s3mock.calls[0].body

	decompressed := gunzip(t, body)
	assert.Contains(t, string(decompressed), "compressed-record")
}

// TestS3Delivery_MultiRecordNewlineSeparated verifies that multiple records delivered
// in one flush are joined with newlines.
func TestS3Delivery_MultiRecordNewlineSeparated(t *testing.T) {
	t.Parallel()

	h, s3mock := auditHandler(t)

	auditCreateStream(t, h, "multi-stream", map[string]any{
		"S3DestinationConfiguration": map[string]any{
			"BucketARN": "arn:aws:s3:::multi-bucket",
		},
	})

	rec := doFirehoseRequest(t, h, "PutRecordBatch", map[string]any{
		"DeliveryStreamName": "multi-stream",
		"Records": []map[string]any{
			{"Data": base64.StdEncoding.EncodeToString([]byte("rec-A"))},
			{"Data": base64.StdEncoding.EncodeToString([]byte("rec-B"))},
			{"Data": base64.StdEncoding.EncodeToString([]byte("rec-C"))},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	h.Backend.(*firehose.InMemoryBackend).FlushAll(t.Context())

	require.Len(t, s3mock.calls, 1)
	body := string(s3mock.calls[0].body)
	assert.Contains(t, body, "rec-A")
	assert.Contains(t, body, "rec-B")
	assert.Contains(t, body, "rec-C")
	assert.Equal(t, 3, strings.Count(body, "\n"), "each record gets a trailing newline; three records → three newlines")
}

// TestS3Delivery_BufferingHintsSizeOverride verifies that setting SizeInMBs=1 causes a
// size-based flush when records exceed 1 MB.
func TestS3Delivery_BufferingHintsSizeOverride(t *testing.T) {
	t.Parallel()

	h, s3mock := auditHandler(t)

	auditCreateStream(t, h, "size-hint-stream", map[string]any{
		"S3DestinationConfiguration": map[string]any{
			"BucketARN": "arn:aws:s3:::size-hint-bucket",
			"BufferingHints": map[string]any{
				"SizeInMBs":         1,
				"IntervalInSeconds": 300,
			},
		},
	})

	// Under 1 MB — no flush expected.
	smallRecord := make([]byte, 900*1024)
	rec := doFirehoseRequest(t, h, "PutRecord", map[string]any{
		"DeliveryStreamName": "size-hint-stream",
		"Record":             map[string]any{"Data": base64.StdEncoding.EncodeToString(smallRecord)},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, s3mock.calls, "no flush below size limit")

	// Over 1 MB — flush expected.
	overRecord := make([]byte, 200*1024)
	rec = doFirehoseRequest(t, h, "PutRecord", map[string]any{
		"DeliveryStreamName": "size-hint-stream",
		"Record":             map[string]any{"Data": base64.StdEncoding.EncodeToString(overRecord)},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Len(t, s3mock.calls, 1, "exceeding SizeInMBs must trigger flush")
}

// TestHTTPEndpoint_DeliveryFormat_ViaHandler verifies the AWS Firehose HTTP delivery
// payload format: requestId, timestamp, and base64-encoded records.
func TestHTTPEndpoint_DeliveryFormat_ViaHandler(t *testing.T) {
	t.Parallel()

	srv := newCaptureServer(t, http.StatusOK)

	h := newTestFirehoseHandler(t)
	b := h.Backend.(*firehose.InMemoryBackend)

	auditCreateStream(t, h, "http-fmt-stream", map[string]any{
		"HTTPEndpointDestinationConfiguration": map[string]any{
			"EndpointConfiguration": map[string]any{
				"Url":       srv.srv.URL,
				"AccessKey": "test-access-key",
			},
			"BufferingHints": map[string]any{
				"SizeInMBs":         1,
				"IntervalInSeconds": 300,
			},
		},
	})

	auditPut(t, h, "http-fmt-stream", "http-payload-data")
	b.FlushAll(t.Context())

	requests := srv.captured()
	require.Len(t, requests, 1)

	var payload struct {
		RequestID string `json:"requestId"`
		Records   []struct {
			Data string `json:"data"`
		} `json:"records"`
		Timestamp int64 `json:"timestamp"`
	}
	require.NoError(t, json.Unmarshal(requests[0].body, &payload))

	assert.NotEmpty(t, payload.RequestID)
	assert.Positive(t, payload.Timestamp)
	require.Len(t, payload.Records, 1)
	decoded, err := base64.StdEncoding.DecodeString(payload.Records[0].Data)
	require.NoError(t, err)
	assert.Equal(t, "http-payload-data", string(decoded))

	// Access key must be in the Authorization header.
	authHeader := requests[0].headers.Get("X-Amz-Firehose-Access-Key")
	assert.Equal(t, "test-access-key", authHeader)
}

// TestOpenSearch_BulkNDJSON_ViaHandler verifies that records are delivered to
// OpenSearch as NDJSON _bulk requests.
func TestOpenSearch_BulkNDJSON_ViaHandler(t *testing.T) {
	t.Parallel()

	srv := newCaptureServer(t, http.StatusOK)

	h := newTestFirehoseHandler(t)
	b := h.Backend.(*firehose.InMemoryBackend)

	auditCreateStream(t, h, "os-audit-stream", map[string]any{
		"AmazonOpenSearchServiceDestinationConfiguration": map[string]any{
			"ClusterEndpoint": srv.srv.URL,
			"IndexName":       "audit-index",
			"RoleARN":         "arn:aws:iam::000000000000:role/firehose",
		},
	})

	auditPut(t, h, "os-audit-stream", `{"event":"test"}`)
	b.FlushAll(t.Context())

	requests := srv.captured()
	require.Len(t, requests, 1)

	// NDJSON: first line is action line, second is document.
	lines := strings.Split(strings.TrimRight(string(requests[0].body), "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "NDJSON must have at least action+document lines")

	var action map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &action))
	_, hasIndex := action["index"]
	assert.True(t, hasIndex, "first NDJSON line must be an index action")
}

// TestSplunk_Modes_ViaHandler verifies both Raw and Event HEC delivery modes.
func TestSplunk_Modes_ViaHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		endpointType   string
		wantBodyPrefix string
		contentType    string
	}{
		{
			name:           "raw_mode",
			endpointType:   "Raw",
			wantBodyPrefix: "splunk-raw-data",
			contentType:    "text/plain",
		},
		{
			name:         "event_mode",
			endpointType: "Event",
			contentType:  "application/json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newCaptureServer(t, http.StatusOK)

			h := newTestFirehoseHandler(t)
			b := h.Backend.(*firehose.InMemoryBackend)
			streamName := "splunk-" + tc.endpointType

			auditCreateStream(t, h, streamName, map[string]any{
				"SplunkDestinationConfiguration": map[string]any{
					"HECEndpoint":     srv.srv.URL,
					"HECEndpointType": tc.endpointType,
					"HECToken":        "tok-" + tc.endpointType,
				},
			})

			auditPut(t, h, streamName, "splunk-raw-data")
			b.FlushAll(t.Context())

			requests := srv.captured()
			require.Len(t, requests, 1)

			auth := requests[0].headers.Get("Authorization")
			assert.Equal(t, "Splunk tok-"+tc.endpointType, auth)

			contentType := requests[0].headers.Get("Content-Type")
			assert.Contains(t, contentType, tc.contentType)

			if tc.endpointType == "Raw" {
				assert.Contains(t, string(requests[0].body), "splunk-raw-data")
			} else {
				var evtBody map[string]any
				require.NoError(t, json.Unmarshal(requests[0].body, &evtBody))
				_, hasEvent := evtBody["event"]
				assert.True(t, hasEvent, "Event mode must wrap data in {event:...}")
			}
		})
	}
}

// okLambdaResponse builds a Lambda transform response body that marks every supplied
// (recordID, data) pair as "Ok".
func okLambdaResponse(records ...struct{ id, data string }) []byte {
	type rec struct {
		RecordID string `json:"recordId"`
		Result   string `json:"result"`
		Data     string `json:"data"`
	}
	out := struct {
		Records []rec `json:"records"`
	}{}
	for _, r := range records {
		out.Records = append(out.Records, rec{
			RecordID: r.id,
			Result:   "Ok",
			Data:     base64.StdEncoding.EncodeToString([]byte(r.data)),
		})
	}
	b, _ := json.Marshal(out)

	return b
}

// TestLambdaTransform_AllDestinations verifies the Lambda transform runs for non-S3
// destinations too (not only S3), so a configured ProcessingConfiguration transforms
// records before they reach the HTTP endpoint.
func TestLambdaTransform_AllDestinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		transform  string
		wantOnWire string
	}{
		{name: "transformed_payload_reaches_http", transform: "TRANSFORMED", wantOnWire: "TRANSFORMED"},
		{name: "different_payload", transform: "hello-world", wantOnWire: "hello-world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := newCaptureServer(t, http.StatusOK)
			b := firehose.NewInMemoryBackend("000000000000", flushRegion)
			// Record IDs are the record's zero-based index (see buildLambdaTransformPayload).
			b.SetLambdaBackend(&mockLambdaInvoker{
				response: okLambdaResponse(struct{ id, data string }{"0", tt.transform}),
			})

			stream, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
				Name: "http-transform",
				HTTPEndpointDestination: &firehose.HTTPEndpointDestinationDescription{
					EndpointConfiguration: &firehose.HTTPEndpointConfiguration{URL: srv.srv.URL},
					RetryOptions:          &firehose.RetryOptions{DurationInSeconds: 1},
					ProcessingConfiguration: &firehose.ProcessingConfiguration{
						Enabled: true,
						Processors: []firehose.Processor{{
							Type: "Lambda",
							Parameters: []firehose.ProcessorParameter{
								{ParameterName: "LambdaArn", ParameterValue: "fn"},
							},
						}},
					},
				},
			})
			require.NoError(t, err)

			require.NoError(t, b.PutRecord(context.TODO(), stream.Name, []byte("original")))
			b.FlushAll(context.Background())

			assert.Eventually(t, func() bool { return len(srv.captured()) > 0 },
				3*time.Second, 20*time.Millisecond)

			var payload struct {
				Records []struct {
					Data string `json:"data"`
				} `json:"records"`
			}
			require.NoError(t, json.Unmarshal(srv.captured()[0].body, &payload))
			require.Len(t, payload.Records, 1)
			decoded, decErr := base64.StdEncoding.DecodeString(payload.Records[0].Data)
			require.NoError(t, decErr)
			assert.Equal(t, tt.wantOnWire, string(decoded))
		})
	}
}

// TestProcessingFailed_RoutesToErrorOutput verifies that Lambda "ProcessingFailed"
// records are written to the S3 ErrorOutputPrefix and counted in FailedRecords, while
// "Ok" records are delivered to the main prefix.
func TestProcessingFailed_RoutesToErrorOutput(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	lambdaResponse := `{"records":[` +
		`{"recordId":"0","result":"Ok","data":"b2s="},` + // "ok"
		`{"recordId":"1","result":"ProcessingFailed","data":""}` +
		`]}`
	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)
	b.SetLambdaBackend(&mockLambdaInvoker{response: []byte(lambdaResponse)})

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "failroute",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN:         "arn:aws:s3:::main-bucket",
			ErrorOutputPrefix: "errors/",
			ProcessingConfiguration: &firehose.ProcessingConfiguration{
				Enabled: true,
				Processors: []firehose.Processor{{
					Type:       "Lambda",
					Parameters: []firehose.ProcessorParameter{{ParameterName: "LambdaArn", ParameterValue: "fn"}},
				}},
			},
		},
	})
	require.NoError(t, err)

	_, err = b.PutRecordBatch(context.TODO(), "failroute", [][]byte{[]byte("first"), []byte("second")})
	require.NoError(t, err)
	b.FlushAll(context.Background())

	require.Len(t, s3mock.calls, 2)

	var main, errOut *mockS3PutCall
	for _, c := range s3mock.calls {
		if strings.Contains(c.key, "errors/") {
			errOut = c
		} else {
			main = c
		}
	}
	require.NotNil(t, main, "expected a main-prefix delivery")
	require.NotNil(t, errOut, "expected an error-output delivery")
	assert.Contains(t, string(main.body), "ok")
	assert.Contains(t, string(errOut.body), "second", "original failed record routed to error output")

	assert.Equal(t, int64(1), firehose.StreamFailedRecords(b, flushRegion, "failroute"))
}

// TestS3BackupMode_DeliversBackupRecords verifies that with S3BackupMode=Enabled the
// accumulated backup copies are actually delivered to the backup bucket.
func TestS3BackupMode_DeliversBackupRecords(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "backup-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN:    "arn:aws:s3:::main-bucket",
			S3BackupMode: "Enabled",
			S3BackupDescription: &firehose.S3BackupDescription{
				BucketARN: "arn:aws:s3:::backup-bucket",
				Prefix:    "backup/",
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.PutRecord(context.TODO(), "backup-stream", []byte("payload")))
	b.FlushAll(context.Background())

	var mainSeen, backupSeen bool
	for _, c := range s3mock.calls {
		switch c.bucket {
		case "main-bucket":
			mainSeen = true
			assert.Contains(t, string(c.body), "payload")
		case "backup-bucket":
			backupSeen = true
			assert.Contains(t, c.key, "backup/")
			assert.Contains(t, string(c.body), "payload")
		}
	}
	assert.True(t, mainSeen, "main delivery expected")
	assert.True(t, backupSeen, "S3 backup delivery expected")
}

// TestDataFormatConversion verifies that DataFormatConversionConfiguration is honored
// end-to-end: valid JSON records are converted to a canonical form and delivered, while
// records that cannot be parsed are routed to the error output.
func TestDataFormatConversion(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "convert-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN:         "arn:aws:s3:::convert-bucket",
			ErrorOutputPrefix: "conv-errors/",
			FileExtension:     ".parquet",
			DataFormatConversion: &firehose.DataFormatConversionConfig{
				Enabled:             true,
				SchemaConfiguration: &firehose.SchemaConfiguration{DatabaseName: "db", TableName: "t"},
				InputFormatConfiguration: &firehose.InputFormatConfiguration{
					Deserializer: &firehose.Deserializer{OpenXJSONSerDe: &firehose.OpenXJSONSerDe{}},
				},
				OutputFormatConfiguration: &firehose.OutputFormatConfiguration{
					Serializer: &firehose.Serializer{ParquetSerDe: &firehose.ParquetSerDe{}},
				},
			},
		},
	})
	require.NoError(t, err)

	_, err = b.PutRecordBatch(context.TODO(), "convert-stream",
		[][]byte{[]byte(`{"b":2,"a":1}`), []byte("not-json")})
	require.NoError(t, err)
	b.FlushAll(context.Background())

	require.Len(t, s3mock.calls, 2)

	var converted, errOut *mockS3PutCall
	for _, c := range s3mock.calls {
		if strings.Contains(c.key, "conv-errors/") {
			errOut = c
		} else {
			converted = c
		}
	}
	require.NotNil(t, converted)
	require.NotNil(t, errOut)
	// Canonical, key-sorted JSON output.
	assert.Contains(t, string(converted.body), `{"a":1,"b":2}`)
	// FileExtension applied to the converted object key.
	assert.Contains(t, converted.key, ".parquet")
	// Unparseable record routed to the error output.
	assert.Contains(t, string(errOut.body), "not-json")
	assert.Equal(t, int64(1), firehose.StreamFailedRecords(b, flushRegion, "convert-stream"))
}

// TestDynamicPartitioning verifies that dynamic partitioning splits records into
// per-partition objects using the !{partitionKeyFromQuery:...} prefix expression, and
// routes records that cannot be partitioned to the error output.
func TestDynamicPartitioning(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "partition-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN:                        "arn:aws:s3:::part-bucket",
			Prefix:                           "data/cust=!{partitionKeyFromQuery:.customer}/",
			ErrorOutputPrefix:                "part-errors/",
			DynamicPartitioningConfiguration: &firehose.DynamicPartitioningConfiguration{Enabled: true},
		},
	})
	require.NoError(t, err)

	_, err = b.PutRecordBatch(context.TODO(), "partition-stream", [][]byte{
		[]byte(`{"customer":"alice","v":1}`),
		[]byte(`{"customer":"bob","v":2}`),
		[]byte(`{"customer":"alice","v":3}`),
		[]byte(`{"no_customer":true}`),
	})
	require.NoError(t, err)
	b.FlushAll(context.Background())

	var aliceKey, bobKey, errKey string
	for _, c := range s3mock.calls {
		switch {
		case strings.Contains(c.key, "part-errors/"):
			errKey = c.key
		case strings.Contains(c.key, "cust=alice/"):
			aliceKey = c.key
		case strings.Contains(c.key, "cust=bob/"):
			bobKey = c.key
		}
	}
	assert.NotEmpty(t, aliceKey, "expected an alice partition object")
	assert.NotEmpty(t, bobKey, "expected a bob partition object")
	assert.NotEmpty(t, errKey, "record without partition key routed to error output")
	assert.Equal(t, int64(1), firehose.StreamFailedRecords(b, flushRegion, "partition-stream"))
}

// TestPendingFlush_Efficiency verifies only streams holding buffered records are
// tracked for interval flushing, and the entry is cleared once the buffer is flushed.
func TestPendingFlush_Efficiency(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "pending-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN:      "arn:aws:s3:::b",
			BufferingHints: &firehose.BufferingHints{SizeInMBs: 100, IntervalInSeconds: 300},
		},
	})
	require.NoError(t, err)

	// No buffered records yet → nothing pending.
	assert.Equal(t, 0, firehose.PendingFlushCount(b))

	// Buffer a small record (well under the size threshold) → stream becomes pending.
	require.NoError(t, b.PutRecord(context.TODO(), "pending-stream", []byte("x")))
	assert.Equal(t, 1, firehose.PendingFlushCount(b))

	// Flushing empties the buffer → the pending entry is cleared.
	b.FlushAll(context.Background())
	assert.Equal(t, 0, firehose.PendingFlushCount(b))
}

// TestLambdaTransformError_DeliversCloudWatchLogEvent verifies that a delivery failure on
// a destination with CloudWatchLoggingOptions.Enabled actually reaches the wired
// CloudWatch Logs backend (gopherstack-pe7x), instead of only being logged locally.
func TestLambdaTransformError_DeliversCloudWatchLogEvent(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	lambdaMock := &mockLambdaInvoker{err: errLambdaUnavailable}
	cwLogsMock := &mockCWLogsBackend{}

	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)
	b.SetLambdaBackend(lambdaMock)
	b.SetCWLogsBackend(cwLogsMock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "cwlog-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN:         "arn:aws:s3:::err-bucket",
			ErrorOutputPrefix: "errors/",
			ProcessingConfiguration: &firehose.ProcessingConfiguration{
				Enabled: true,
				Processors: []firehose.Processor{
					{
						Type: "Lambda",
						Parameters: []firehose.ProcessorParameter{
							{ParameterName: "LambdaArn", ParameterValue: "my-fn"},
						},
					},
				},
			},
			CloudWatchLoggingOptions: &firehose.CloudWatchLoggingOptions{
				Enabled:       true,
				LogGroupName:  "/aws/kinesisfirehose/cwlog-stream",
				LogStreamName: "S3Delivery",
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.PutRecord(context.TODO(), "cwlog-stream", []byte("input")))
	b.FlushAll(t.Context())

	require.Len(t, cwLogsMock.putCalls, 1,
		"delivery failure must be delivered to the wired CloudWatch Logs backend")
	call := cwLogsMock.putCalls[0]
	assert.Equal(t, "/aws/kinesisfirehose/cwlog-stream", call.group)
	assert.Equal(t, "S3Delivery", call.stream)
	require.Len(t, call.messages, 1)
	assert.Contains(t, call.messages[0], "cwlog-stream")
	assert.Contains(t, call.messages[0], "lambda transform invocation failed")
	assert.Contains(t, cwLogsMock.ensured, "/aws/kinesisfirehose/cwlog-stream|S3Delivery")
}

// TestLambdaTransformError_UnwiredCloudWatchLogsStaysPermissive verifies that a
// destination with CloudWatchLoggingOptions.Enabled still delivers normally (records
// routed to the error output) when CloudWatch Logs has not been wired in via
// SetCWLogsBackend -- an unwired hook must be a silent no-op, never a rejection.
func TestLambdaTransformError_UnwiredCloudWatchLogsStaysPermissive(t *testing.T) {
	t.Parallel()

	s3mock := &mockS3Storer{}
	lambdaMock := &mockLambdaInvoker{err: errLambdaUnavailable}

	b := firehose.NewInMemoryBackend("000000000000", flushRegion)
	b.SetS3Backend(s3mock)
	b.SetLambdaBackend(lambdaMock)

	_, err := b.CreateDeliveryStream(context.TODO(), firehose.CreateDeliveryStreamInput{
		Name: "cwlog-unwired-stream",
		S3Destination: &firehose.S3DestinationDescription{
			BucketARN:         "arn:aws:s3:::err-bucket",
			ErrorOutputPrefix: "errors/",
			ProcessingConfiguration: &firehose.ProcessingConfiguration{
				Enabled: true,
				Processors: []firehose.Processor{
					{
						Type: "Lambda",
						Parameters: []firehose.ProcessorParameter{
							{ParameterName: "LambdaArn", ParameterValue: "my-fn"},
						},
					},
				},
			},
			CloudWatchLoggingOptions: &firehose.CloudWatchLoggingOptions{
				Enabled:       true,
				LogGroupName:  "/aws/kinesisfirehose/cwlog-unwired-stream",
				LogStreamName: "S3Delivery",
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, b.PutRecord(context.TODO(), "cwlog-unwired-stream", []byte("input")))
	b.FlushAll(t.Context())

	require.Len(t, s3mock.calls, 1)
	assert.Contains(t, s3mock.calls[0].key, "errors/",
		"delivery must still succeed and route the failed record when CloudWatch Logs is unwired")
}
