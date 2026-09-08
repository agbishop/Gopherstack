package timestreamquery_test

import (
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	tqsdk "github.com/aws/aws-sdk-go-v2/service/timestreamquery"
	"github.com/aws/aws-sdk-go-v2/service/timestreamquery/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/timestreamquery"
	"github.com/blackbirdworks/gopherstack/services/timestreamwrite"
)

const (
	rtTestRegion    = "us-east-1"
	rtTestAccountID = "000000000000"
)

// newTestHandlerAndClient stands up a fresh in-memory timestreamquery
// backend and a real aws-sdk-go-v2 client against an httptest server running
// its Handler, wired through the same pkgs/service registry/router used in
// production. It also registers a timestreamwrite handler on the same
// registry: TagResource/UntagResource/ListTagsForResource for Timestream
// Query resources are deliberately NOT claimed by timestreamquery's own
// RouteMatcher (writeServiceTagOps in handler.go) and route to
// timestreamwrite's handler instead -- exactly like production (cli.go
// registers both providers into one registry), so a test that only stands up
// timestreamquery would 404 on every tag op regardless of backend
// correctness.
func newTestHandlerAndClient(t *testing.T) *tqsdk.Client {
	t.Helper()

	backend := timestreamquery.NewInMemoryBackend(rtTestAccountID, rtTestRegion)
	h := timestreamquery.NewHandler(backend)
	writeBackend := timestreamwrite.NewInMemoryBackend()
	writeHandler := timestreamwrite.NewHandler(writeBackend)
	backend.SetTagWriteBackend(writeBackend)

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(h))
	require.NoError(t, registry.Register(writeHandler))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion(rtTestRegion),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return tqsdk.NewFromConfig(cfg, func(o *tqsdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

// TestCreateScheduledQuery_TagsRoundTrip drives CreateScheduledQuery, the
// only timestreamquery Create* op whose real Input struct accepts Tags
// (timestreamquery@v1.39.4: api_op_CreateScheduledQuery.go, `Tags
// []types.Tag`), through the real SDK client and asserts
// ListTagsForResource sees what was supplied at creation (gopherstack-2mwl).
func TestCreateScheduledQuery_TagsRoundTrip(t *testing.T) {
	t.Parallel()

	client := newTestHandlerAndClient(t)

	wantTags := []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}}

	out, err := client.CreateScheduledQuery(t.Context(), &tqsdk.CreateScheduledQueryInput{
		Name:                           aws.String("tagged-scheduled-query"),
		QueryString:                    aws.String("SELECT 1"),
		ScheduledQueryExecutionRoleArn: aws.String("arn:aws:iam::000000000000:role/tsq-role"),
		ScheduleConfiguration: &types.ScheduleConfiguration{
			ScheduleExpression: aws.String("rate(1 hour)"),
		},
		NotificationConfiguration: &types.NotificationConfiguration{
			SnsConfiguration: &types.SnsConfiguration{
				TopicArn: aws.String("arn:aws:sns:us-east-1:000000000000:tsq-topic"),
			},
		},
		ErrorReportConfiguration: &types.ErrorReportConfiguration{
			S3Configuration: &types.S3Configuration{
				BucketName: aws.String("tsq-error-bucket"),
			},
		},
		Tags: wantTags,
	})
	require.NoError(t, err)

	got, err := client.ListTagsForResource(t.Context(), &tqsdk.ListTagsForResourceInput{
		ResourceARN: out.Arn,
	})
	require.NoError(t, err)
	assert.Equal(t, wantTags, got.Tags)
}

// TestDeleteScheduledQuery_RemovesSharedTags drives CreateScheduledQuery (with
// Tags) then DeleteScheduledQuery through the real SDK client, and asserts
// ListTagsForResource no longer reports the deleted ARN's tags. Without
// cleanup, tags mirrored into the shared TimestreamWrite tag store on create
// outlive the scheduled query itself, since TimestreamWrite's TagResource
// accepts any ARN containing "scheduled-query/" without checking whether it
// still exists (services/timestreamwrite/store.go isKnownARNLocked).
func TestDeleteScheduledQuery_RemovesSharedTags(t *testing.T) {
	t.Parallel()

	client := newTestHandlerAndClient(t)

	out, err := client.CreateScheduledQuery(t.Context(), &tqsdk.CreateScheduledQueryInput{
		Name:                           aws.String("ghost-tag-test"),
		QueryString:                    aws.String("SELECT 1"),
		ScheduledQueryExecutionRoleArn: aws.String("arn:aws:iam::000000000000:role/tsq-role"),
		ScheduleConfiguration: &types.ScheduleConfiguration{
			ScheduleExpression: aws.String("rate(1 hour)"),
		},
		NotificationConfiguration: &types.NotificationConfiguration{
			SnsConfiguration: &types.SnsConfiguration{
				TopicArn: aws.String("arn:aws:sns:us-east-1:000000000000:tsq-topic"),
			},
		},
		ErrorReportConfiguration: &types.ErrorReportConfiguration{
			S3Configuration: &types.S3Configuration{
				BucketName: aws.String("tsq-error-bucket"),
			},
		},
		Tags: []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	_, err = client.DeleteScheduledQuery(t.Context(), &tqsdk.DeleteScheduledQueryInput{
		ScheduledQueryArn: out.Arn,
	})
	require.NoError(t, err)

	got, err := client.ListTagsForResource(t.Context(), &tqsdk.ListTagsForResourceInput{
		ResourceARN: out.Arn,
	})
	require.NoError(t, err)
	assert.Empty(t, got.Tags, "deleting a scheduled query must not leave its tags behind in the shared tag store")
}
