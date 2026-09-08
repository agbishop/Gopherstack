package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	firehosebackend "github.com/blackbirdworks/gopherstack/services/firehose"
	redshiftdatabackend "github.com/blackbirdworks/gopherstack/services/redshiftdata"
	s3backend "github.com/blackbirdworks/gopherstack/services/s3"
)

// TestInitializeServices_FirehoseRedshiftWiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) rather than invoking
// wireFirehoseRedshift directly, so that deleting the wiring call from
// wireStorageAndSecretsIntegrations -- not just breaking the helper function itself -- is what
// this test is sensitive to. Same shape as cli_firehose_kinesis_wiring_test.go.
//
// Regression test for gopherstack-lgwb: a delivery stream with a Redshift destination must
// actually issue the COPY command against the real Redshift Data API after S3 staging, not
// silently log a WARN and never execute it. Before the fix, SetRedshiftDataBackend was never
// called from cli.go, so b.redshiftData stayed nil in a real running server and this test's
// Redshift Data statement never appeared.
func TestInitializeServices_FirehoseRedshiftWiring(t *testing.T) {
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

	redshiftdataH, ok := byName["RedshiftData"].(*redshiftdatabackend.Handler)
	require.True(t, ok, "RedshiftData handler must be registered")

	redshiftdataBk, ok := redshiftdataH.Backend.(*redshiftdatabackend.InMemoryBackend)
	require.True(t, ok, "RedshiftData backend must be an InMemoryBackend")

	s3H, ok := byName["S3"].(*s3backend.S3Handler)
	require.True(t, ok, "S3 handler must be registered")

	s3Bk, ok := s3H.Backend.(*s3backend.InMemoryBackend)
	require.True(t, ok, "S3 backend must be an InMemoryBackend")

	ctx := t.Context()

	bucketName := "firehose-redshift-wiring-bucket"
	_, err = s3Bk.CreateBucket(ctx, &sdk_s3.CreateBucketInput{Bucket: aws.String(bucketName)})
	require.NoError(t, err)

	stream, err := fhBk.CreateDeliveryStream(ctx, firehosebackend.CreateDeliveryStreamInput{
		Name: "firehose-redshift-wiring-stream",
		RedshiftDestination: &firehosebackend.RedshiftDestinationDescription{
			ClusterJDBCURL: "jdbc:redshift://firehose-wiring-cluster.abc123.us-east-1.redshift.amazonaws.com:5439/wiringdb",
			Username:       "wiringuser",
			RoleARN:        "arn:aws:iam::000000000000:role/firehose-redshift-role",
			CopyCommand: &firehosebackend.RedshiftCopyCommand{
				DataTableName: "wiring_table",
			},
			S3Destination: &firehosebackend.S3DestinationDescription{
				BucketARN: "arn:aws:s3:::" + bucketName,
				BufferingHints: &firehosebackend.BufferingHints{
					SizeInMBs:         1,
					IntervalInSeconds: 0,
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, stream)

	require.NoError(t, fhBk.PutRecord(ctx, stream.Name, []byte("firehose-redshift-wiring-payload")))

	require.Eventually(t, func() bool {
		fhBk.FlushAll(ctx)

		return redshiftHasCopyStatement(ctx, t, redshiftdataBk, "firehose-wiring-cluster", "wiringdb", "wiring_table")
	}, 10*time.Second, 100*time.Millisecond,
		"a Redshift destination's COPY command must actually execute against the real Redshift "+
			"Data API through the actual cli.go composition root's wiring (wireFirehoseRedshift), "+
			"not just wired via the helper called directly")
}

// redshiftHasCopyStatement reports whether the Redshift Data backend recorded a statement for
// clusterID/database whose SQL references dataTableName (a COPY into wiring_table).
func redshiftHasCopyStatement(
	ctx context.Context, t *testing.T, backend *redshiftdatabackend.InMemoryBackend,
	clusterID, database, dataTableName string,
) bool {
	t.Helper()

	stmts, _, err := backend.ListStatements(ctx, redshiftdatabackend.ListStatementsFilter{
		ClusterIdentifier: clusterID,
		Database:          database,
	})
	if err != nil {
		return false
	}

	for _, stmt := range stmts {
		if strings.Contains(stmt.QueryString, "COPY") && strings.Contains(stmt.QueryString, dataTableName) {
			return true
		}
	}

	return false
}
