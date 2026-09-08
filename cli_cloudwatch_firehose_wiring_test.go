package main

import (
	"log/slog"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	cwbackend "github.com/blackbirdworks/gopherstack/services/cloudwatch"
	firehosebackend "github.com/blackbirdworks/gopherstack/services/firehose"
	s3backend "github.com/blackbirdworks/gopherstack/services/s3"
)

// TestInitializeServices_CloudWatchFirehoseWiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) rather than invoking
// wireCloudWatchFirehose directly, so that deleting the wiring call from
// wireStorageAndSecretsIntegrations -- not just breaking the helper function itself -- is what
// this test is sensitive to. Same shape as cli_firehose_kinesis_wiring_test.go.
//
// Regression test for gopherstack-vjmc: a metric stream created with a Firehose destination
// must actually deliver matched metric data to that delivery stream, not silently accept the
// stream (reporting "running") and never deliver anything. Before the fix, SetFirehosePutter
// was never called from cli.go, so b.firehosePutter stayed nil in a real running server and
// this test's S3 object (Firehose's own delivery destination) never appeared.
func TestInitializeServices_CloudWatchFirehoseWiring(t *testing.T) {
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

	cwH, ok := byName["CloudWatch"].(*cwbackend.Handler)
	require.True(t, ok, "CloudWatch handler must be registered")

	cwBk, ok := cwH.Backend.(*cwbackend.InMemoryBackend)
	require.True(t, ok, "CloudWatch backend must be an InMemoryBackend")

	firehoseH, ok := byName["Firehose"].(*firehosebackend.Handler)
	require.True(t, ok, "Firehose handler must be registered")

	fhBk, ok := firehoseH.Backend.(*firehosebackend.InMemoryBackend)
	require.True(t, ok, "Firehose backend must be an InMemoryBackend")

	s3H, ok := byName["S3"].(*s3backend.S3Handler)
	require.True(t, ok, "S3 handler must be registered")

	s3Bk, ok := s3H.Backend.(*s3backend.InMemoryBackend)
	require.True(t, ok, "S3 backend must be an InMemoryBackend")

	ctx := t.Context()

	bucketName := "cw-firehose-wiring-bucket"
	_, err = s3Bk.CreateBucket(ctx, &sdk_s3.CreateBucketInput{Bucket: aws.String(bucketName)})
	require.NoError(t, err)

	stream, err := fhBk.CreateDeliveryStream(ctx, firehosebackend.CreateDeliveryStreamInput{
		Name: "cw-firehose-wiring-stream",
		S3Destination: &firehosebackend.S3DestinationDescription{
			BucketARN: "arn:aws:s3:::" + bucketName,
			BufferingHints: &firehosebackend.BufferingHints{
				SizeInMBs:         1,
				IntervalInSeconds: 0,
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, cwBk.PutMetricStream(&cwbackend.MetricStream{
		Name:         "cw-firehose-wiring-metric-stream",
		FirehoseArn:  "arn:aws:firehose:us-east-1:000000000000:deliverystream/" + stream.Name,
		RoleArn:      "arn:aws:iam::000000000000:role/metric-stream-role",
		OutputFormat: "json",
	}))

	require.NoError(t, cwBk.PutMetricData("AWS/EC2", []cwbackend.MetricDatum{{
		MetricName: "CPUUtilization",
		Timestamp:  time.Now(),
		Value:      77,
		HasValue:   true,
	}}))

	// The background interval flusher (Handler.StartWorker) isn't running in this
	// composition-root test, so force delivery of whatever the poller has buffered on each
	// tick rather than waiting on a real flush interval -- this test is about the CloudWatch
	// wiring, not flush-timing mechanics (covered elsewhere).
	require.Eventually(t, func() bool {
		fhBk.FlushAll(ctx)

		return deliveredObjectContains(ctx, t, s3Bk, bucketName, "CPUUtilization")
	}, 10*time.Second, 100*time.Millisecond,
		"a CloudWatch metric stream's matched data must reach its Firehose destination through "+
			"the actual cli.go composition root's wiring (wireCloudWatchFirehose), not just "+
			"wired via the helper called directly")
}
