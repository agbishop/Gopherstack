package main

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	firehosesdk "github.com/aws/aws-sdk-go-v2/service/firehose"
	firehosetypes "github.com/aws/aws-sdk-go-v2/service/firehose/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	firehosebackend "github.com/blackbirdworks/gopherstack/services/firehose"
	kinesisbackend "github.com/blackbirdworks/gopherstack/services/kinesis"
	s3backend "github.com/blackbirdworks/gopherstack/services/s3"
)

// TestInitializeServices_FirehoseKinesisSourceWiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) rather than invoking
// wireFirehoseKinesisSource directly, so that deleting the wiring call from
// wireStorageAndSecretsIntegrations -- not just breaking the helper function itself -- is what
// this test is sensitive to. Same shape as cli_kinesisanalytics_kinesis_s3_wiring_test.go.
//
// Regression test for gopherstack-o4ny: a delivery stream created with
// DeliveryStreamType=KinesisStreamAsSource must actually poll its source Kinesis stream and
// deliver ingested records to the configured S3 destination, not silently accept the stream
// and never ingest anything. Before the fix, SetKinesisBackend was never called from cli.go, so
// b.kinesisBackend stayed nil in a real running server and this test's S3 object never appeared.
func TestInitializeServices_FirehoseKinesisSourceWiring(t *testing.T) {
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

	firehoseH, ok := byName["Firehose"].(*firehosebackend.Handler)
	require.True(t, ok, "Firehose handler must be registered")

	fhBk, ok := firehoseH.Backend.(*firehosebackend.InMemoryBackend)
	require.True(t, ok, "Firehose backend must be an InMemoryBackend")

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
	require.NoError(t, registry.Register(firehoseH))
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

	fhClient := firehosesdk.NewFromConfig(cfg, func(o *firehosesdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})

	bucketName := "firehose-kinesis-wiring-bucket"
	_, err = s3Bk.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucketName)})
	require.NoError(t, err)

	streamName := "firehose-kinesis-wiring-stream"
	require.NoError(t, kinesisBk.CreateStream(ctx, &kinesisbackend.CreateStreamInput{
		StreamName: streamName,
		ShardCount: 1,
	}))

	roleARN := "arn:aws:iam::000000000000:role/role"
	streamARN := "arn:aws:kinesis:us-east-1:000000000000:stream/" + streamName
	bucketARN := "arn:aws:s3:::" + bucketName

	_, err = fhClient.CreateDeliveryStream(ctx, &firehosesdk.CreateDeliveryStreamInput{
		DeliveryStreamName: aws.String("kinesis-wiring-delivery-stream"),
		DeliveryStreamType: firehosetypes.DeliveryStreamTypeKinesisStreamAsSource,
		KinesisStreamSourceConfiguration: &firehosetypes.KinesisStreamSourceConfiguration{
			KinesisStreamARN: aws.String(streamARN),
			RoleARN:          aws.String(roleARN),
		},
		ExtendedS3DestinationConfiguration: &firehosetypes.ExtendedS3DestinationConfiguration{
			BucketARN: aws.String(bucketARN),
			RoleARN:   aws.String(roleARN),
			BufferingHints: &firehosetypes.BufferingHints{
				IntervalInSeconds: aws.Int32(1),
				SizeInMBs:         aws.Int32(1),
			},
		},
	})
	require.NoError(t, err, "CreateDeliveryStream must succeed even before the record is polled")

	payload := "kinesis-source-record-payload"
	_, err = kinesisBk.PutRecord(ctx, &kinesisbackend.PutRecordInput{
		StreamName:   streamName,
		PartitionKey: "pk",
		Data:         []byte(payload),
	})
	require.NoError(t, err)

	// The background interval flusher (Handler.StartWorker) isn't running in this
	// composition-root test, so force delivery of whatever the poller has buffered on each
	// tick rather than waiting on a real flush interval -- this test is about the Kinesis
	// wiring, not flush-timing mechanics (covered elsewhere).
	require.Eventually(t, func() bool {
		fhBk.FlushAll(ctx)

		return deliveredObjectContains(ctx, t, s3Bk, bucketName, payload)
	}, 10*time.Second, 100*time.Millisecond,
		"a record put into the source Kinesis stream must be polled by Firehose through the "+
			"actual cli.go composition root's Kinesis wiring (wireFirehoseKinesisSource) and "+
			"delivered to the configured S3 destination, not just wired via the helper called directly")
}

// deliveredObjectContains reports whether any object in bucket contains payload.
func deliveredObjectContains(
	ctx context.Context, t *testing.T, s3Bk *s3backend.InMemoryBackend, bucket, payload string,
) bool {
	t.Helper()

	listOut, err := s3Bk.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket)})
	if err != nil {
		return false
	}

	for _, obj := range listOut.Contents {
		getOut, getErr := s3Bk.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: obj.Key})
		if getErr != nil {
			continue
		}

		body, readErr := io.ReadAll(getOut.Body)
		_ = getOut.Body.Close()

		if readErr == nil && strings.Contains(string(body), payload) {
			return true
		}
	}

	return false
}
