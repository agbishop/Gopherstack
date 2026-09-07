package firehose_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/firehose"
)

func TestFirehoseHandler_CreateDeliveryStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *firehose.Handler)
		name         string
		streamName   string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "success",
			streamName:   "my-stream",
			wantCode:     http.StatusOK,
			wantContains: []string{"arn:aws:firehose:"},
		},
		{
			name:       "already_exists",
			streamName: "my-stream",
			setup: func(t *testing.T, h *firehose.Handler) {
				t.Helper()
				doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{"DeliveryStreamName": "my-stream"})
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestFirehoseHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}
			rec := doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{
				"DeliveryStreamName": tt.streamName,
			})
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestFirehoseHandler_CreateDeliveryStream_WithS3Destination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body         map[string]any
		name         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "s3_destination",
			body: map[string]any{
				"DeliveryStreamName": "s3-stream",
				"S3DestinationConfiguration": map[string]any{
					"BucketARN": "arn:aws:s3:::my-bucket",
					"RoleARN":   "arn:aws:iam::000000000000:role/firehose",
					"BufferingHints": map[string]any{
						"SizeInMBs":         5,
						"IntervalInSeconds": 300,
					},
					"CompressionFormat": "GZIP",
				},
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"DeliveryStreamARN"},
		},
		{
			name: "extended_s3_destination",
			body: map[string]any{
				"DeliveryStreamName": "ext-s3-stream",
				"ExtendedS3DestinationConfiguration": map[string]any{
					"BucketARN": "arn:aws:s3:::ext-bucket",
					"RoleARN":   "arn:aws:iam::000000000000:role/firehose",
					"ProcessingConfiguration": map[string]any{
						"Enabled": true,
						"Processors": []map[string]any{
							{
								"Type": "Lambda",
								"Parameters": []map[string]any{
									{
										"ParameterName":  "LambdaArn",
										"ParameterValue": "my-fn",
									},
								},
							},
						},
					},
				},
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"DeliveryStreamARN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestFirehoseHandler(t)
			rec := doFirehoseRequest(t, h, "CreateDeliveryStream", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// TestCreateDeliveryStream_DeliveryStreamType verifies the DeliveryStreamType and
// KinesisStreamSourceConfiguration wiring (default DirectPut, explicit DirectPut, and
// KinesisStreamAsSource).
func TestCreateDeliveryStream_DeliveryStreamType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantType string
		wantCode int
	}{
		{
			name:     "default_is_DirectPut",
			body:     map[string]any{"DeliveryStreamName": "s"},
			wantCode: http.StatusOK,
			wantType: "DirectPut",
		},
		{
			name: "explicit_DirectPut",
			body: map[string]any{
				"DeliveryStreamName": "s",
				"DeliveryStreamType": "DirectPut",
			},
			wantCode: http.StatusOK,
			wantType: "DirectPut",
		},
		{
			name: "KinesisStreamAsSource",
			body: map[string]any{
				"DeliveryStreamName": "s",
				"DeliveryStreamType": "KinesisStreamAsSource",
				"KinesisStreamSourceConfiguration": map[string]any{
					"KinesisStreamARN": "arn:aws:kinesis:us-east-1:000000000000:stream/my-stream",
					"RoleARN":          "arn:aws:iam::000000000000:role/firehose",
				},
			},
			wantCode: http.StatusOK,
			wantType: "KinesisStreamAsSource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestFirehoseHandler(t)
			rec := doFirehoseRequest(t, h, "CreateDeliveryStream", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusOK && tt.wantType != "" {
				// Describe and verify type.
				desc := doFirehoseRequest(t, h, "DescribeDeliveryStream",
					map[string]any{"DeliveryStreamName": tt.body["DeliveryStreamName"]})
				require.Equal(t, http.StatusOK, desc.Code)
				assert.Contains(t, desc.Body.String(), tt.wantType)
			}
		})
	}
}

// TestCreateDeliveryStream_MSKSource verifies MSKSourceConfiguration wiring.
func TestCreateDeliveryStream_MSKSource(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	rec := doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{
		"DeliveryStreamName": "msk-stream",
		"DeliveryStreamType": "MSKAsSource",
		"MSKSourceConfiguration": map[string]any{
			"MSKClusterARN": "arn:aws:kafka:us-east-1:000000000000:cluster/my-cluster/abc",
			"TopicName":     "my-topic",
			"AuthenticationConfiguration": map[string]any{
				"Connectivity": "PUBLIC",
				"RoleARN":      "arn:aws:iam::000000000000:role/msk",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	desc := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "msk-stream"})
	require.Equal(t, http.StatusOK, desc.Code)

	body := desc.Body.String()
	assert.Contains(t, body, "MSKSourceDescription")
	assert.Contains(t, body, "my-topic")
}

// TestCreateDeliveryStream_Redshift verifies RedshiftDestinationConfiguration wiring.
func TestCreateDeliveryStream_Redshift(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	rec := doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{
		"DeliveryStreamName": "rs-stream",
		"RedshiftDestinationConfiguration": map[string]any{
			"ClusterJDBCURL": "jdbc:redshift://cluster.us-east-1.redshift.amazonaws.com:5439/db",
			"RoleARN":        "arn:aws:iam::000000000000:role/rs",
			"S3BackupConfiguration": map[string]any{
				"BucketARN": "arn:aws:s3:::rs-bucket",
				"RoleARN":   "arn:aws:iam::000000000000:role/rs",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	desc := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "rs-stream"})
	require.Equal(t, http.StatusOK, desc.Code)
	assert.Contains(t, desc.Body.String(), "RedshiftDestinationDescription")
}

// TestCreateDeliveryStream_HTTPEndpoint_Extended verifies HTTP endpoint extended fields
// (buffering hints, retry options, request configuration, CloudWatch logging).
func TestCreateDeliveryStream_HTTPEndpoint_Extended(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	rec := doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{
		"DeliveryStreamName": "http-ext",
		"HTTPEndpointDestinationConfiguration": map[string]any{
			"EndpointConfiguration": map[string]any{
				"Url":  "https://my-endpoint.example.com",
				"Name": "my-endpoint",
			},
			"BufferingHints": map[string]any{
				"SizeInMBs":         5,
				"IntervalInSeconds": 300,
			},
			"RetryOptions": map[string]any{
				"DurationInSeconds": 300,
			},
			"RequestConfiguration": map[string]any{
				"ContentEncoding": "GZIP",
				"CommonAttributes": []map[string]any{
					{"AttributeName": "x-custom", "AttributeValue": "val"},
				},
			},
			"CloudWatchLoggingOptions": map[string]any{
				"Enabled":      true,
				"LogGroupName": "/aws/firehose/http",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	desc := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "http-ext"})
	require.Equal(t, http.StatusOK, desc.Code)

	body := desc.Body.String()
	assert.Contains(t, body, "BufferingHints")
	assert.Contains(t, body, "RetryOptions")
	assert.Contains(t, body, "RequestConfiguration")
}

// TestExtendedS3_AdditionalFieldsPreserved verifies that less-common ExtendedS3 fields
// (ErrorOutputPrefix, FileExtension, CustomTimeZone, encryption, CloudWatch logging,
// dynamic partitioning) round-trip through Describe.
func TestExtendedS3_AdditionalFieldsPreserved(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	rec := doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{
		"DeliveryStreamName": "ext-s3",
		"ExtendedS3DestinationConfiguration": map[string]any{
			"BucketARN":         "arn:aws:s3:::my-bucket",
			"RoleARN":           "arn:aws:iam::000000000000:role/r",
			"ErrorOutputPrefix": "errors/",
			"FileExtension":     ".json.gz",
			"CustomTimeZone":    "America/New_York",
			"EncryptionConfiguration": map[string]any{
				"KMSEncryptionConfig": map[string]any{
					"AWSKMSKeyARN": "arn:aws:kms:us-east-1:000000000000:key/abc",
				},
			},
			"CloudWatchLoggingOptions": map[string]any{
				"Enabled":       true,
				"LogGroupName":  "/aws/firehose/my-stream",
				"LogStreamName": "errors",
			},
			"DynamicPartitioningConfiguration": map[string]any{
				"Enabled": true,
				"RetryOptions": map[string]any{
					"DurationInSeconds": 300,
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	desc := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "ext-s3"})
	require.Equal(t, http.StatusOK, desc.Code)

	body := desc.Body.String()
	assert.Contains(t, body, "errors/")
	assert.Contains(t, body, ".json.gz")
	assert.Contains(t, body, "America/New_York")
	assert.Contains(t, body, "AWSKMSKeyARN")
	assert.Contains(t, body, "CloudWatchLoggingOptions")
	assert.Contains(t, body, "DynamicPartitioningConfiguration")
}

// TestDataFormatConversion_ValidationRejected verifies the AWS-accurate validation:
// enabling format conversion without a schema/serializer is rejected at create time.
func TestDataFormatConversion_ValidationRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cfg  map[string]any
		name string
	}{
		{name: "missing_schema", cfg: map[string]any{"Enabled": true}},
		{
			name: "missing_serializer",
			cfg: map[string]any{
				"Enabled":             true,
				"SchemaConfiguration": map[string]any{"DatabaseName": "db", "TableName": "t"},
				"InputFormatConfiguration": map[string]any{
					"Deserializer": map[string]any{"OpenXJsonSerDe": map[string]any{}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestFirehoseHandler(t)
			rec := doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{
				"DeliveryStreamName": "bad-convert",
				"ExtendedS3DestinationConfiguration": map[string]any{
					"BucketARN":                         "arn:aws:s3:::b",
					"RoleARN":                           "arn:aws:iam::000000000000:role/r",
					"DataFormatConversionConfiguration": tt.cfg,
				},
			})
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "InvalidArgumentException")
		})
	}
}

// TestCreateDeliveryStream_MultipleDestinations_Rejected verifies that CreateDeliveryStream
// rejects a request naming more than one destination configuration, matching real AWS
// (which accepts exactly one destination type per call).
func TestCreateDeliveryStream_MultipleDestinations_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	rec := doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{
		"DeliveryStreamName": "multi-dest-stream",
		"S3DestinationConfiguration": map[string]any{
			"BucketARN": "arn:aws:s3:::b1",
			"RoleARN":   "arn:aws:iam::000000000000:role/r",
		},
		"SplunkDestinationConfiguration": map[string]any{
			"HECEndpoint": "https://splunk.example.com:8088",
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidArgumentException")

	// The rejected stream must not have been created.
	descRec := doFirehoseRequest(t, h, "DescribeDeliveryStream", map[string]any{
		"DeliveryStreamName": "multi-dest-stream",
	})
	assert.Equal(t, http.StatusNotFound, descRec.Code)
}

// TestCreateDeliveryStream_SingleDestination_Accepted verifies that a request naming
// exactly one destination configuration (the S3/ExtendedS3 "family" counts as one slot)
// is accepted.
func TestCreateDeliveryStream_SingleDestination_Accepted(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	rec := doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{
		"DeliveryStreamName": "single-dest-stream",
		"ExtendedS3DestinationConfiguration": map[string]any{
			"BucketARN": "arn:aws:s3:::b1",
			"RoleARN":   "arn:aws:iam::000000000000:role/r",
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// TestCreateDeliveryStream_WithTags verifies that tags supplied at creation time are
// attached to the stream.
func TestCreateDeliveryStream_WithTags(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	rec := doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{
		"DeliveryStreamName": "tagged-create",
		"Tags": []map[string]any{
			{"Key": "env", "Value": "test"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	tagRec := doFirehoseRequest(t, h, "ListTagsForDeliveryStream", map[string]any{
		"DeliveryStreamName": "tagged-create",
	})
	require.Equal(t, http.StatusOK, tagRec.Code)
	assert.Contains(t, tagRec.Body.String(), "env")
	assert.Contains(t, tagRec.Body.String(), "test")
}

// TestCreateDeliveryStream_EmptyName_Handler verifies an empty DeliveryStreamName is
// rejected at the handler layer.
func TestCreateDeliveryStream_EmptyName_Handler(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	rec := doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{"DeliveryStreamName": ""})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFirehoseHandler_DeleteDeliveryStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T, h *firehose.Handler)
		streamName string
		wantCode   int
	}{
		{
			name:       "success",
			streamName: "my-stream",
			setup: func(t *testing.T, h *firehose.Handler) {
				t.Helper()
				doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{"DeliveryStreamName": "my-stream"})
			},
			wantCode: http.StatusOK,
		},
		{
			name:       "not_found",
			streamName: "nonexistent",
			wantCode:   http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestFirehoseHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}
			rec := doFirehoseRequest(t, h, "DeleteDeliveryStream", map[string]any{
				"DeliveryStreamName": tt.streamName,
			})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestDeleteDeliveryStream_Success verifies that a deleted stream no longer appears in
// ListDeliveryStreams.
func TestDeleteDeliveryStream_Success(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	createStream(t, h, "del-stream")

	rec := doFirehoseRequest(t, h, "DeleteDeliveryStream",
		map[string]any{"DeliveryStreamName": "del-stream"})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Stream no longer appears in list.
	list := doFirehoseRequest(t, h, "ListDeliveryStreams", map[string]any{})
	require.Equal(t, http.StatusOK, list.Code)
	assert.NotContains(t, list.Body.String(), "del-stream")
}

func TestDeleteDeliveryStream_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	rec := doFirehoseRequest(t, h, "DeleteDeliveryStream",
		map[string]any{"DeliveryStreamName": "no-such-stream"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestDeleteThenDescribeReturns404 verifies that a deleted stream cannot be described
// (ResourceNotFoundException).
func TestDeleteThenDescribeReturns404(t *testing.T) {
	t.Parallel()

	h, _ := auditHandler(t)
	auditCreateStream(t, h, "del-desc-stream", nil)

	delRec := doFirehoseRequest(t, h, "DeleteDeliveryStream",
		map[string]any{"DeliveryStreamName": "del-desc-stream"})
	require.Equal(t, http.StatusOK, delRec.Code)

	descRec := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "del-desc-stream"})
	assert.Equal(t, http.StatusNotFound, descRec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &body))
	assert.Equal(t, "ResourceNotFoundException", body["__type"])
}

func TestFirehoseHandler_DescribeDeliveryStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *firehose.Handler)
		name         string
		streamName   string
		wantContains []string
		wantCode     int
	}{
		{
			name:       "success",
			streamName: "my-stream",
			setup: func(t *testing.T, h *firehose.Handler) {
				t.Helper()
				doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{"DeliveryStreamName": "my-stream"})
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"DeliveryStreamDescription"},
		},
		{
			name:       "not_found",
			streamName: "nonexistent",
			wantCode:   http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestFirehoseHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}
			rec := doFirehoseRequest(t, h, "DescribeDeliveryStream", map[string]any{
				"DeliveryStreamName": tt.streamName,
			})
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestFirehoseHandler_DescribeDeliveryStream_WithS3Destination(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{
		"DeliveryStreamName": "describe-s3-stream",
		"S3DestinationConfiguration": map[string]any{
			"BucketARN": "arn:aws:s3:::my-bucket",
		},
	})

	rec := doFirehoseRequest(t, h, "DescribeDeliveryStream", map[string]any{
		"DeliveryStreamName": "describe-s3-stream",
	})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ExtendedS3DestinationDescription")
	assert.Contains(t, rec.Body.String(), "arn:aws:s3:::my-bucket")
}

func TestDescribeDeliveryStream_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	rec := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "no-such-stream"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestDescribeDeliveryStream_KinesisSource verifies the KinesisStreamSourceDescription
// wire shape in Describe.
func TestDescribeDeliveryStream_KinesisSource(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	createStream(t, h, "ks-stream", map[string]any{
		"DeliveryStreamType": "KinesisStreamAsSource",
		"KinesisStreamSourceConfiguration": map[string]any{
			"KinesisStreamARN": "arn:aws:kinesis:us-east-1:000000000000:stream/my-ks",
			"RoleARN":          "arn:aws:iam::000000000000:role/r",
		},
	})

	rec := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "ks-stream"})
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "KinesisStreamSourceDescription")
	assert.Contains(t, body, "arn:aws:kinesis:us-east-1:000000000000:stream/my-ks")
}

// TestKinesisStreamAsSource_DescribeShowsSource verifies that the Source field is
// populated in DescribeDeliveryStream for a KinesisStreamAsSource stream.
func TestKinesisStreamAsSource_DescribeShowsSource(t *testing.T) {
	t.Parallel()

	h, _ := auditHandler(t)
	auditCreateStream(t, h, "ks-describe-stream", map[string]any{
		"DeliveryStreamType": "KinesisStreamAsSource",
		"KinesisStreamSourceConfiguration": map[string]any{
			"KinesisStreamARN": "arn:aws:kinesis:us-east-1:000000000000:stream/src-stream",
			"RoleARN":          "arn:aws:iam::000000000000:role/firehose",
		},
	})

	desc := auditDescribe(t, h, "ks-describe-stream")
	assert.Equal(t, "KinesisStreamAsSource", desc["DeliveryStreamType"])

	src, ok := desc["Source"].(map[string]any)
	require.True(t, ok, "Source must be present")
	ksSrc, ok := src["KinesisStreamSourceDescription"].(map[string]any)
	require.True(t, ok, "KinesisStreamSourceDescription must be present")
	assert.Equal(t,
		"arn:aws:kinesis:us-east-1:000000000000:stream/src-stream",
		ksSrc["KinesisStreamARN"],
	)
}

// TestDescribeDeliveryStream_SourceField verifies the Source field round-trips for a
// KinesisStreamAsSource stream.
func TestDescribeDeliveryStream_SourceField(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	createStream(t, h, "src-stream", map[string]any{
		"DeliveryStreamType": "KinesisStreamAsSource",
		"KinesisStreamSourceConfiguration": map[string]any{
			"KinesisStreamARN": "arn:aws:kinesis:us-east-1:000000000000:stream/my-ks",
			"RoleARN":          "arn:aws:iam::000000000000:role/r",
		},
	})

	rec := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "src-stream"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		DeliveryStreamDescription struct {
			Source *firehose.SourceDescription `json:"Source"`
		} `json:"DeliveryStreamDescription"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotNil(t, out.DeliveryStreamDescription.Source)
	require.NotNil(t, out.DeliveryStreamDescription.Source.KinesisStreamSourceDescription)
	assert.Equal(t, "arn:aws:kinesis:us-east-1:000000000000:stream/my-ks",
		out.DeliveryStreamDescription.Source.KinesisStreamSourceDescription.KinesisStreamARN)
}

// TestDescribeDeliveryStream_MSKSourceDescription_Fields verifies MSKSourceDescription
// fields round-trip through Describe.
func TestDescribeDeliveryStream_MSKSourceDescription_Fields(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	createStream(t, h, "msk-desc-stream", map[string]any{
		"DeliveryStreamType": "MSKAsSource",
		"MSKSourceConfiguration": map[string]any{
			"MSKClusterARN": "arn:aws:kafka:us-east-1:000000000000:cluster/my-cluster/abc",
			"TopicName":     "events-topic",
			"AuthenticationConfiguration": map[string]any{
				"Connectivity": "PRIVATE",
				"RoleARN":      "arn:aws:iam::000000000000:role/msk-reader",
			},
		},
	})

	desc := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "msk-desc-stream"})
	require.Equal(t, http.StatusOK, desc.Code)

	body := desc.Body.String()
	assert.Contains(t, body, "MSKSourceDescription")
	assert.Contains(t, body, "events-topic")
	assert.Contains(t, body, "arn:aws:kafka:us-east-1:000000000000:cluster/my-cluster/abc")
	assert.Contains(t, body, "PRIVATE")
}

// TestDescribeDeliveryStream_S3Destination_FieldsPreserved verifies
// ExtendedS3DestinationDescription fields round-trip through Describe.
func TestDescribeDeliveryStream_S3Destination_FieldsPreserved(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	createStream(t, h, "s3-fields-stream", map[string]any{
		"S3DestinationConfiguration": map[string]any{
			"BucketARN":         "arn:aws:s3:::my-test-bucket",
			"RoleARN":           "arn:aws:iam::000000000000:role/firehose",
			"Prefix":            "logs/",
			"CompressionFormat": "GZIP",
		},
	})

	desc := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "s3-fields-stream"})
	require.Equal(t, http.StatusOK, desc.Code)

	body := desc.Body.String()
	assert.Contains(t, body, "ExtendedS3DestinationDescription")
	assert.Contains(t, body, "arn:aws:s3:::my-test-bucket")
	assert.Contains(t, body, "logs/")
	assert.Contains(t, body, "GZIP")
}

// TestDescribeDeliveryStream_Timestamps verifies CreateTimestamp/LastUpdateTimestamp
// are populated in Describe.
func TestDescribeDeliveryStream_Timestamps(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	createStream(t, h, "ts-stream")

	rec := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "ts-stream"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		DeliveryStreamDescription struct {
			CreateTimestamp     *int64 `json:"CreateTimestamp"`
			LastUpdateTimestamp *int64 `json:"LastUpdateTimestamp"`
		} `json:"DeliveryStreamDescription"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.NotNil(t, out.DeliveryStreamDescription.CreateTimestamp)
	assert.NotNil(t, out.DeliveryStreamDescription.LastUpdateTimestamp)
	assert.Positive(t, *out.DeliveryStreamDescription.CreateTimestamp)
}

// TestDescribeDeliveryStream_ReturnsVersionAndType verifies the VersionId and
// DeliveryStreamType default values appear in Describe.
func TestDescribeDeliveryStream_ReturnsVersionAndType(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{"DeliveryStreamName": "vt-stream"})

	rec := doFirehoseRequest(t, h, "DescribeDeliveryStream", map[string]any{
		"DeliveryStreamName": "vt-stream",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"DeliveryStreamType":"DirectPut"`)
	assert.Contains(t, rec.Body.String(), `"VersionId":"1"`)
}

// TestDescribeDeliveryStream_HasMoreDestinations verifies the HasMoreDestinations field
// appears in Describe.
func TestDescribeDeliveryStream_HasMoreDestinations(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	createStream(t, h, "hmd-stream", map[string]any{
		"S3DestinationConfiguration": map[string]any{
			"BucketARN": "arn:aws:s3:::my-bucket",
		},
	})

	rec := doFirehoseRequest(t, h, "DescribeDeliveryStream",
		map[string]any{"DeliveryStreamName": "hmd-stream"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "HasMoreDestinations")
}

// TestDescribeDeliveryStream_OpenSearchAndSplunk verifies that DescribeDeliveryStream
// includes OpenSearch and Splunk destination descriptions in the response.
func TestDescribeDeliveryStream_OpenSearchAndSplunk(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		createBody  map[string]any
		wantContain string
		wantKey     string
	}{
		{
			name: "opensearch_in_describe",
			createBody: map[string]any{
				"DeliveryStreamName": "aos-desc-stream",
				"AmazonOpenSearchServiceDestinationConfiguration": map[string]any{
					"DomainARN": "arn:aws:es:us-east-1:000000000000:domain/my-domain",
					"IndexName": "access-logs",
					"RoleARN":   "arn:aws:iam::000000000000:role/firehose",
				},
			},
			wantKey:     "AmazonopensearchserviceDestinationDescription",
			wantContain: "my-domain",
		},
		{
			name: "splunk_in_describe",
			createBody: map[string]any{
				"DeliveryStreamName": "splunk-desc-stream",
				"SplunkDestinationConfiguration": map[string]any{
					"HECEndpoint":     "https://my-splunk.example.com:8088",
					"HECEndpointType": "Raw",
					"HECToken":        "my-token",
				},
			},
			wantKey:     "SplunkDestinationDescription",
			wantContain: "my-splunk.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestFirehoseHandler(t)

			create := doFirehoseRequest(t, h, "CreateDeliveryStream", tt.createBody)
			require.Equal(t, http.StatusOK, create.Code)

			desc := doFirehoseRequest(t, h, "DescribeDeliveryStream", map[string]any{
				"DeliveryStreamName": tt.createBody["DeliveryStreamName"],
			})
			require.Equal(t, http.StatusOK, desc.Code)

			body := desc.Body.String()
			assert.Contains(t, body, tt.wantKey)
			assert.Contains(t, body, tt.wantContain)
		})
	}
}

// TestDescribeDeliveryStream_CoreFields verifies that Describe returns the expected
// fields: ARN, status, type, VersionId, CreateTimestamp.
func TestDescribeDeliveryStream_CoreFields(t *testing.T) {
	t.Parallel()

	h, _ := auditHandler(t)
	arn := auditCreateStream(t, h, "core-stream", nil)

	desc := auditDescribe(t, h, "core-stream")

	assert.Equal(t, "core-stream", desc["DeliveryStreamName"])
	assert.Equal(t, arn, desc["DeliveryStreamARN"])
	assert.True(t, strings.HasPrefix(arn, "arn:aws:firehose:"), "ARN must start with arn:aws:firehose:")
	assert.Equal(t, "ACTIVE", desc["DeliveryStreamStatus"])
	assert.Equal(t, "DirectPut", desc["DeliveryStreamType"])
	assert.Equal(t, "1", desc["VersionId"])
	assert.NotNil(t, desc["CreateTimestamp"], "CreateTimestamp must be non-nil")
}

// TestDestinationDescribe_AllTypes verifies that each of the five destination types is
// persisted and returned in DescribeDeliveryStream in the correct destination-specific
// field.
func TestDestinationDescribe_AllTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createExtra   map[string]any
		checkDescribe func(t *testing.T, desc map[string]any)
		name          string
	}{
		{
			name: "s3_destination",
			createExtra: map[string]any{
				"S3DestinationConfiguration": map[string]any{
					"BucketARN": "arn:aws:s3:::audit-bucket",
					"RoleARN":   "arn:aws:iam::000000000000:role/firehose",
					"Prefix":    "logs/",
				},
			},
			checkDescribe: func(t *testing.T, desc map[string]any) {
				t.Helper()
				d := singleDestination(t, desc, "ExtendedS3DestinationDescription")
				assert.Equal(t, "arn:aws:s3:::audit-bucket", d["BucketARN"])
				assert.Equal(t, "logs/", d["Prefix"])
			},
		},
		{
			name: "extended_s3_destination",
			createExtra: map[string]any{
				"ExtendedS3DestinationConfiguration": map[string]any{
					"BucketARN":         "arn:aws:s3:::ext-bucket",
					"RoleARN":           "arn:aws:iam::000000000000:role/firehose",
					"CompressionFormat": "GZIP",
					"Prefix":            "ext/",
				},
			},
			checkDescribe: func(t *testing.T, desc map[string]any) {
				t.Helper()
				d := singleDestination(t, desc, "ExtendedS3DestinationDescription")
				assert.Equal(t, "arn:aws:s3:::ext-bucket", d["BucketARN"])
				assert.Equal(t, "GZIP", d["CompressionFormat"])
				assert.Equal(t, "ext/", d["Prefix"])
			},
		},
		{
			name: "http_endpoint_destination",
			createExtra: map[string]any{
				"HTTPEndpointDestinationConfiguration": map[string]any{
					"EndpointConfiguration": map[string]any{
						"Url":  "https://ingest.example.com/firehose",
						"Name": "audit-endpoint",
					},
					"BufferingHints": map[string]any{
						"SizeInMBs":         5,
						"IntervalInSeconds": 60,
					},
				},
			},
			checkDescribe: func(t *testing.T, desc map[string]any) {
				t.Helper()
				d := singleDestination(t, desc, "HttpEndpointDestinationDescription")
				ep := d["EndpointConfiguration"].(map[string]any)
				assert.Equal(t, "https://ingest.example.com/firehose", ep["Url"])
				assert.Equal(t, "audit-endpoint", ep["Name"])
			},
		},
		{
			name: "redshift_destination",
			createExtra: map[string]any{
				"RedshiftDestinationConfiguration": map[string]any{
					"ClusterJDBCURL": "jdbc:redshift://my-cluster.abc.us-east-1.redshift.amazonaws.com:5439/mydb",
					"RoleARN":        "arn:aws:iam::000000000000:role/firehose",
					"CopyCommand": map[string]any{
						"DataTableName":    "events",
						"DataTableColumns": "ts,payload",
					},
					"Username": "admin",
				},
			},
			checkDescribe: func(t *testing.T, desc map[string]any) {
				t.Helper()
				d := singleDestination(t, desc, "RedshiftDestinationDescription")
				assert.Contains(t, d["ClusterJDBCURL"].(string), "redshift")
				copyCommand := d["CopyCommand"].(map[string]any)
				assert.Equal(t, "events", copyCommand["DataTableName"])
				assert.Equal(t, "admin", d["Username"])
			},
		},
		{
			name: "opensearch_destination",
			createExtra: map[string]any{
				"AmazonOpenSearchServiceDestinationConfiguration": map[string]any{
					"ClusterEndpoint":     "https://search-my-domain.us-east-1.es.amazonaws.com",
					"IndexName":           "firehose-logs",
					"IndexRotationPeriod": "OneDay",
					"RoleARN":             "arn:aws:iam::000000000000:role/firehose",
				},
			},
			checkDescribe: func(t *testing.T, desc map[string]any) {
				t.Helper()
				d := singleDestination(t, desc, "AmazonopensearchserviceDestinationDescription")
				assert.Equal(t, "https://search-my-domain.us-east-1.es.amazonaws.com", d["ClusterEndpoint"])
				assert.Equal(t, "firehose-logs", d["IndexName"])
				assert.Equal(t, "OneDay", d["IndexRotationPeriod"])
			},
		},
		{
			name: "splunk_destination",
			createExtra: map[string]any{
				"SplunkDestinationConfiguration": map[string]any{
					"HECEndpoint":     "https://splunk.example.com:8088",
					"HECEndpointType": "Raw",
					"HECToken":        "splunk-token-abc",
				},
			},
			checkDescribe: func(t *testing.T, desc map[string]any) {
				t.Helper()
				d := singleDestination(t, desc, "SplunkDestinationDescription")
				assert.Equal(t, "https://splunk.example.com:8088", d["HECEndpoint"])
				assert.Equal(t, "Raw", d["HECEndpointType"])
				assert.Equal(t, "splunk-token-abc", d["HECToken"])
			},
		},
		{
			name: "msk_source",
			createExtra: map[string]any{
				"MSKSourceConfiguration": map[string]any{
					"MSKClusterARN": "arn:aws:kafka:us-east-1:000000000000:cluster/my-msk/abc",
					"TopicName":     "events",
					"AuthenticationConfiguration": map[string]any{
						"Connectivity": "PUBLIC",
						"RoleARN":      "arn:aws:iam::000000000000:role/firehose",
					},
				},
			},
			checkDescribe: func(t *testing.T, desc map[string]any) {
				t.Helper()
				src, ok := desc["Source"].(map[string]any)
				require.True(t, ok, "Source must be present for MSK")
				msk, ok := src["MSKSourceDescription"].(map[string]any)
				require.True(t, ok, "MSKSourceDescription must be present")
				assert.Equal(t, "events", msk["TopicName"])
				assert.Contains(t, msk["MSKClusterARN"].(string), "kafka")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, _ := auditHandler(t)
			streamName := "audit-dest-" + tc.name
			auditCreateStream(t, h, streamName, tc.createExtra)
			desc := auditDescribe(t, h, streamName)
			tc.checkDescribe(t, desc)
		})
	}
}

// TestS3BackupMode_PersistedInDescribe verifies that S3BackupMode=Enabled on HTTP and
// ExtendedS3 destinations is persisted and returned in DescribeDeliveryStream.
func TestS3BackupMode_PersistedInDescribe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		createExtra   map[string]any
		checkDescribe func(t *testing.T, desc map[string]any)
		name          string
	}{
		{
			name: "extended_s3_backup_enabled",
			createExtra: map[string]any{
				"ExtendedS3DestinationConfiguration": map[string]any{
					"BucketARN":    "arn:aws:s3:::main-bucket",
					"RoleARN":      "arn:aws:iam::000000000000:role/firehose",
					"S3BackupMode": "Enabled",
					"S3BackupConfiguration": map[string]any{
						"BucketARN": "arn:aws:s3:::backup-bucket",
						"RoleARN":   "arn:aws:iam::000000000000:role/firehose",
					},
				},
			},
			checkDescribe: func(t *testing.T, desc map[string]any) {
				t.Helper()
				d := singleDestination(t, desc, "ExtendedS3DestinationDescription")
				assert.Equal(t, "Enabled", d["S3BackupMode"])
				backup := d["S3BackupDescription"].(map[string]any)
				assert.Equal(t, "arn:aws:s3:::backup-bucket", backup["BucketARN"])
			},
		},
		{
			name: "http_endpoint_backup_enabled",
			createExtra: map[string]any{
				"HTTPEndpointDestinationConfiguration": map[string]any{
					"EndpointConfiguration": map[string]any{
						"Url": "https://endpoint.example.com",
					},
					"S3BackupMode": "Enabled",
					// "S3Configuration" is the real wire key for HttpEndpoint's single S3
					// bucket (used only as the backup/failed-data sink) -- confirmed via
					// awsAwsjson11_serializeDocumentHttpEndpointDestinationConfiguration.
					// It is NOT "S3BackupConfiguration" despite the field's role.
					"S3Configuration": map[string]any{
						"BucketARN": "arn:aws:s3:::http-backup-bucket",
						"RoleARN":   "arn:aws:iam::000000000000:role/firehose",
					},
				},
			},
			checkDescribe: func(t *testing.T, desc map[string]any) {
				t.Helper()
				d := singleDestination(t, desc, "HttpEndpointDestinationDescription")
				assert.Equal(t, "Enabled", d["S3BackupMode"])
				// HttpEndpoint's single S3 bucket is wire-keyed "S3DestinationDescription",
				// not "S3BackupDescription" -- confirmed via
				// awsAwsjson11_deserializeDocumentHttpEndpointDestinationDescription.
				backup := d["S3DestinationDescription"].(map[string]any)
				assert.Equal(t, "arn:aws:s3:::http-backup-bucket", backup["BucketARN"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, _ := auditHandler(t)
			streamName := "backup-" + tc.name
			auditCreateStream(t, h, streamName, tc.createExtra)
			desc := auditDescribe(t, h, streamName)
			tc.checkDescribe(t, desc)
		})
	}
}

func TestFirehoseHandler_ListDeliveryStreams(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{"DeliveryStreamName": "s1"})
	doFirehoseRequest(t, h, "CreateDeliveryStream", map[string]any{"DeliveryStreamName": "s2"})

	rec := doFirehoseRequest(t, h, "ListDeliveryStreams", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DeliveryStreamNames")
}

func TestListDeliveryStreams_Empty(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	rec := doFirehoseRequest(t, h, "ListDeliveryStreams", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		DeliveryStreamNames    []string `json:"DeliveryStreamNames"`
		HasMoreDeliveryStreams bool     `json:"HasMoreDeliveryStreams"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Empty(t, out.DeliveryStreamNames)
	assert.False(t, out.HasMoreDeliveryStreams)
}

// TestListDeliveryStreams_Limit verifies the Limit parameter and HasMoreDeliveryStreams
// flag.
func TestListDeliveryStreams_Limit(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	for _, name := range []string{"a-stream", "b-stream", "c-stream"} {
		createStream(t, h, name)
	}

	rec := doFirehoseRequest(t, h, "ListDeliveryStreams", map[string]any{"Limit": 2})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		DeliveryStreamNames    []string `json:"DeliveryStreamNames"`
		HasMoreDeliveryStreams bool     `json:"HasMoreDeliveryStreams"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Len(t, out.DeliveryStreamNames, 2)
	assert.True(t, out.HasMoreDeliveryStreams)
}

// TestListDeliveryStreams_ExclusiveStart verifies the ExclusiveStartDeliveryStreamName
// cursor.
func TestListDeliveryStreams_ExclusiveStart(t *testing.T) {
	t.Parallel()

	h := newTestFirehoseHandler(t)
	for _, name := range []string{"a-stream", "b-stream", "c-stream"} {
		createStream(t, h, name)
	}

	rec := doFirehoseRequest(t, h, "ListDeliveryStreams",
		map[string]any{"ExclusiveStartDeliveryStreamName": "a-stream"})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		DeliveryStreamNames []string `json:"DeliveryStreamNames"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, []string{"b-stream", "c-stream"}, out.DeliveryStreamNames)
}

// TestListDeliveryStreams_Pagination verifies ExclusiveStartDeliveryStreamName cursor
// and HasMoreDeliveryStreams flag end-to-end via the handler.
func TestListDeliveryStreams_Pagination(t *testing.T) {
	t.Parallel()

	h, _ := auditHandler(t)

	for i := range 5 {
		auditCreateStream(t, h, string(rune('a'+i))+"-list-stream", nil)
	}

	// Page 1: limit 2.
	page1Rec := doFirehoseRequest(t, h, "ListDeliveryStreams",
		map[string]any{"Limit": 2})
	var page1 struct {
		DeliveryStreamNames    []string `json:"DeliveryStreamNames"`
		HasMoreDeliveryStreams bool     `json:"HasMoreDeliveryStreams"`
	}
	require.NoError(t, json.Unmarshal(page1Rec.Body.Bytes(), &page1))
	assert.Len(t, page1.DeliveryStreamNames, 2)
	assert.True(t, page1.HasMoreDeliveryStreams)

	// Page 2 using cursor.
	cursor := page1.DeliveryStreamNames[1]
	page2Rec := doFirehoseRequest(t, h, "ListDeliveryStreams", map[string]any{
		"Limit":                            2,
		"ExclusiveStartDeliveryStreamName": cursor,
	})
	var page2 struct {
		DeliveryStreamNames    []string `json:"DeliveryStreamNames"`
		HasMoreDeliveryStreams bool     `json:"HasMoreDeliveryStreams"`
	}
	require.NoError(t, json.Unmarshal(page2Rec.Body.Bytes(), &page2))
	assert.Len(t, page2.DeliveryStreamNames, 2)
	// No overlap between pages.
	for _, n := range page2.DeliveryStreamNames {
		assert.NotContains(t, page1.DeliveryStreamNames, n, "page 2 must not repeat page 1 names")
	}
}

// TestListDeliveryStreams_TypeFilter verifies that the DeliveryStreamType filter is
// honored: only streams of the requested type are returned.
func TestListDeliveryStreams_TypeFilter(t *testing.T) {
	t.Parallel()

	h, _ := auditHandler(t)

	auditCreateStream(t, h, "direct-put-stream", nil)
	auditCreateStream(t, h, "kinesis-src-stream-lf", map[string]any{
		"DeliveryStreamType": "KinesisStreamAsSource",
		"KinesisStreamSourceConfiguration": map[string]any{
			"KinesisStreamARN": "arn:aws:kinesis:us-east-1:000000000000:stream/my-src",
			"RoleARN":          "arn:aws:iam::000000000000:role/firehose",
		},
	})

	// Filter for DirectPut only — the Kinesis-source stream must be excluded.
	rec := doFirehoseRequest(t, h, "ListDeliveryStreams",
		map[string]any{"DeliveryStreamType": "DirectPut"})
	var out struct {
		DeliveryStreamNames []string `json:"DeliveryStreamNames"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Contains(t, out.DeliveryStreamNames, "direct-put-stream")
	assert.NotContains(t, out.DeliveryStreamNames, "kinesis-src-stream-lf")

	// Filter for KinesisStreamAsSource only — the DirectPut stream must be excluded.
	rec2 := doFirehoseRequest(t, h, "ListDeliveryStreams",
		map[string]any{"DeliveryStreamType": "KinesisStreamAsSource"})
	var out2 struct {
		DeliveryStreamNames []string `json:"DeliveryStreamNames"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))
	assert.Contains(t, out2.DeliveryStreamNames, "kinesis-src-stream-lf")
	assert.NotContains(t, out2.DeliveryStreamNames, "direct-put-stream")

	// An unrecognized filter value is not rejected -- ListDeliveryStreams declares no
	// exception beyond UnknownError (deserializers.go: deserializeOpErrorListDeliveryStreams),
	// so it just matches no stream, same as any other value with no match (gopherstack-t2wb).
	recInvalid := doFirehoseRequest(t, h, "ListDeliveryStreams",
		map[string]any{"DeliveryStreamType": "Bogus"})
	require.Equal(t, http.StatusOK, recInvalid.Code)

	var outInvalid struct {
		DeliveryStreamNames []string `json:"DeliveryStreamNames"`
	}
	require.NoError(t, json.Unmarshal(recInvalid.Body.Bytes(), &outInvalid))
	assert.NotContains(t, outInvalid.DeliveryStreamNames, "direct-put-stream")
	assert.NotContains(t, outInvalid.DeliveryStreamNames, "kinesis-src-stream-lf")
}
