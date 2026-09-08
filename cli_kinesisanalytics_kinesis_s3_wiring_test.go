package main

import (
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	kasdk "github.com/aws/aws-sdk-go-v2/service/kinesisanalytics"
	katypes "github.com/aws/aws-sdk-go-v2/service/kinesisanalytics/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	kinesisbackend "github.com/blackbirdworks/gopherstack/services/kinesis"
	kinesisanalyticsbackend "github.com/blackbirdworks/gopherstack/services/kinesisanalytics"
	s3backend "github.com/blackbirdworks/gopherstack/services/s3"
)

// discoverSchemaErrorCode extracts the smithy error code from err, or "" if err isn't one.
func discoverSchemaErrorCode(err error) string {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		return apiErr.ErrorCode()
	}

	return ""
}

// TestInitializeServices_KinesisAnalyticsKinesisS3Wiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) rather than invoking
// wireKinesisAnalyticsCrossService directly, so that deleting the wiring call from
// wireStorageAndSecretsIntegrations -- not just breaking the helper function itself -- is what
// this test is sensitive to. Same shape as cli_glacier_select_s3_wiring_test.go/
// cli_iotanalytics_lambda_iot_wiring_test.go. Covers DiscoverInputSchema's real sampling+
// inference (services/kinesisanalytics/discover_schema.go): S3 objects and Kinesis stream
// records put directly into the wired sibling backends must be visible through the SDK client,
// with real inferred columns -- not a canned shape. A Firehose-sourced request has no reader
// wired at all (services/firehose has no accessor to read back ingested records) and must fail
// with the documented UnableToDetectSchemaException, not something misleading.
func TestInitializeServices_KinesisAnalyticsKinesisS3Wiring(t *testing.T) {
	t.Parallel()

	cli := &CLI{AccountID: "000000000000", Region: "us-east-1"}
	appCtx := &service.AppContext{
		Logger:     slog.Default(),
		Config:     cli,
		JanitorCtx: t.Context(),
	}
	cli.faultStore = chaos.NewFaultStore()

	services, err := initializeServices(appCtx)
	require.NoError(t, err)

	byName := serviceByName(services)

	kaH, ok := byName["KinesisAnalytics"].(*kinesisanalyticsbackend.Handler)
	require.True(t, ok, "KinesisAnalytics handler must be registered")

	kinesisH, ok := byName["Kinesis"].(*kinesisbackend.Handler)
	require.True(t, ok, "Kinesis handler must be registered")

	kinesisBk, ok := kinesisH.Backend.(*kinesisbackend.InMemoryBackend)
	require.True(t, ok, "Kinesis backend must be an InMemoryBackend")

	s3H, ok := byName["S3"].(*s3backend.S3Handler)
	require.True(t, ok, "S3 handler must be registered")

	s3Bk, ok := s3H.Backend.(*s3backend.InMemoryBackend)
	require.True(t, ok, "S3 backend must be an InMemoryBackend")

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(kaH))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	ctx := t.Context()

	cfg, err := awscfg.LoadDefaultConfig(
		ctx,
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	client := kasdk.NewFromConfig(cfg, func(o *kasdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})

	roleARN := "arn:aws:iam::000000000000:role/role"

	t.Run("s3_wired_samples_real_object", func(t *testing.T) {
		t.Parallel()

		bucketName := "ka-wiring-test-bucket"

		_, createErr := s3Bk.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucketName)})
		require.NoError(t, createErr)

		body := "{\"ticker\":\"AMZN\",\"price\":42,\"active\":true}\n" +
			"{\"ticker\":\"GOOG\",\"price\":101.5,\"active\":false}\n"

		_, putErr := s3Bk.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String("wiring-data.json"),
			Body:   strings.NewReader(body),
		})
		require.NoError(t, putErr)

		out, discErr := client.DiscoverInputSchema(ctx, &kasdk.DiscoverInputSchemaInput{
			S3Configuration: &katypes.S3Configuration{
				BucketARN: aws.String("arn:aws:s3:::" + bucketName),
				FileKey:   aws.String("wiring-data.json"),
				RoleARN:   aws.String(roleARN),
			},
		})
		require.NoError(t, discErr,
			"DiscoverInputSchema must sample the real S3 object through the actual cli.go "+
				"composition root's S3 wiring, not just the wiring helper called directly")

		require.NotNil(t, out.InputSchema)
		require.Len(t, out.InputSchema.RecordColumns, 3)

		// Alphabetical column order: active, price, ticker (discover_schema.go's
		// collectColumnNames).
		assert.Equal(t, "active", aws.ToString(out.InputSchema.RecordColumns[0].Name))
		assert.Equal(t, "BOOLEAN", aws.ToString(out.InputSchema.RecordColumns[0].SqlType))
		assert.Equal(t, "price", aws.ToString(out.InputSchema.RecordColumns[1].Name))
		assert.Equal(t, "DOUBLE", aws.ToString(out.InputSchema.RecordColumns[1].SqlType))
		assert.Equal(t, "ticker", aws.ToString(out.InputSchema.RecordColumns[2].Name))
		assert.Equal(t, "VARCHAR(4)", aws.ToString(out.InputSchema.RecordColumns[2].SqlType))

		require.Len(t, out.RawInputRecords, 2)
		assert.JSONEq(t, `{"ticker":"AMZN","price":42,"active":true}`, out.RawInputRecords[0])
	})

	t.Run("kinesis_wired_samples_real_stream", func(t *testing.T) {
		t.Parallel()

		streamName := "ka-wiring-test-stream"

		createErr := kinesisBk.CreateStream(ctx, &kinesisbackend.CreateStreamInput{
			StreamName: streamName,
			ShardCount: 1,
		})
		require.NoError(t, createErr)

		for _, data := range []string{`{"id":1,"name":"a"}`, `{"id":2,"name":"bb"}`} {
			_, putErr := kinesisBk.PutRecord(ctx, &kinesisbackend.PutRecordInput{
				StreamName:   streamName,
				PartitionKey: "pk",
				Data:         []byte(data),
			})
			require.NoError(t, putErr)
		}

		out, discErr := client.DiscoverInputSchema(ctx, &kasdk.DiscoverInputSchemaInput{
			ResourceARN: aws.String("arn:aws:kinesis:us-east-1:000000000000:stream/" + streamName),
			RoleARN:     aws.String(roleARN),
		})
		require.NoError(t, discErr,
			"DiscoverInputSchema must sample the real Kinesis stream through the actual cli.go "+
				"composition root's Kinesis wiring (kinesisAnalyticsStreamReaderAdapter), not "+
				"just the wiring helper called directly")

		require.NotNil(t, out.InputSchema)
		require.Len(t, out.InputSchema.RecordColumns, 2)

		// Alphabetical column order: id, name.
		assert.Equal(t, "id", aws.ToString(out.InputSchema.RecordColumns[0].Name))
		assert.Equal(t, "INTEGER", aws.ToString(out.InputSchema.RecordColumns[0].SqlType))
		assert.Equal(t, "name", aws.ToString(out.InputSchema.RecordColumns[1].Name))
		assert.Equal(t, "VARCHAR(4)", aws.ToString(out.InputSchema.RecordColumns[1].SqlType))
	})

	t.Run("firehose_source_reports_unable_to_detect_schema", func(t *testing.T) {
		t.Parallel()

		_, discErr := client.DiscoverInputSchema(ctx, &kasdk.DiscoverInputSchemaInput{
			ResourceARN: aws.String("arn:aws:firehose:us-east-1:000000000000:deliverystream/does-not-matter"),
			RoleARN:     aws.String(roleARN),
		})
		require.Error(t, discErr, "no Firehose reader is wired -- a Firehose ResourceARN must fail, not fabricate")
		assert.Equal(t, "UnableToDetectSchemaException", discoverSchemaErrorCode(discErr),
			"a Firehose source must fail with the documented, real AWS error for an unreachable "+
				"source, not a generic 500 or a silently-fabricated schema")
	})
}
