package transcribe_test

import (
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	transcribesdk "github.com/aws/aws-sdk-go-v2/service/transcribe"
	sdktypes "github.com/aws/aws-sdk-go-v2/service/transcribe/types"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/transcribe"
)

// These tests prove that transcribe's central resourceTags side map (written
// by TagResource/UntagResource, via recordResourceTagsLocked at creation) and
// each job/category's own creation-time Tags snapshot are two independently-
// written stores -- Get ops that echoed the snapshot directly went stale as
// soon as TagResource was called after creation, the same shape fixed for
// ecs's DescribeTasks/StopTask/UpdateCapacityProvider (gopherstack-g8k9,
// commit 9a40453a2).

// newTranscribeSDKClient stands up a real aws-sdk-go-v2 transcribe client
// against an httptest server running h, wired through the same
// pkgs/service registry/router used in production.
func newTranscribeSDKClient(t *testing.T, h *transcribe.Handler) *transcribesdk.Client {
	t.Helper()

	e := echo.New()
	registry := service.NewRegistry()
	require.NoError(t, registry.Register(h))
	e.Use(service.NewServiceRouter(registry).RouteHandler())

	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)

	cfg, err := awscfg.LoadDefaultConfig(
		t.Context(),
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	return transcribesdk.NewFromConfig(cfg, func(o *transcribesdk.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
}

func tagMapFromTranscribeTags(tags []sdktypes.Tag) map[string]string {
	out := make(map[string]string, len(tags))
	for _, tag := range tags {
		out[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}

	return out
}

func TestGetTranscriptionJob_TagResource_LiveSync_RealClient(t *testing.T) {
	t.Parallel()

	h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
	client := newTranscribeSDKClient(t, h)
	ctx := t.Context()

	_, err := client.StartTranscriptionJob(ctx, &transcribesdk.StartTranscriptionJobInput{
		TranscriptionJobName: aws.String("livesync-job"),
		LanguageCode:         sdktypes.LanguageCodeEnUs,
		Media:                &sdktypes.Media{MediaFileUri: aws.String("s3://b/f.mp3")},
		Tags:                 []sdktypes.Tag{{Key: aws.String("owner"), Value: aws.String("sre")}},
	})
	require.NoError(t, err)

	_, err = client.TagResource(ctx, &transcribesdk.TagResourceInput{
		ResourceArn: aws.String("arn:aws:transcribe:us-east-1:123456789012:transcription-job/livesync-job"),
		Tags:        []sdktypes.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	out, err := client.GetTranscriptionJob(ctx, &transcribesdk.GetTranscriptionJobInput{
		TranscriptionJobName: aws.String("livesync-job"),
	})
	require.NoError(t, err)
	require.Equal(t,
		map[string]string{"owner": "sre", "env": "prod"},
		tagMapFromTranscribeTags(out.TranscriptionJob.Tags),
	)
}

func TestGetCallAnalyticsCategory_TagResource_LiveSync_RealClient(t *testing.T) {
	t.Parallel()

	h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
	client := newTranscribeSDKClient(t, h)
	ctx := t.Context()

	_, err := client.CreateCallAnalyticsCategory(ctx, &transcribesdk.CreateCallAnalyticsCategoryInput{
		CategoryName: aws.String("livesync-category"),
		Rules: []sdktypes.Rule{
			&sdktypes.RuleMemberNonTalkTimeFilter{Value: sdktypes.NonTalkTimeFilter{
				Threshold: aws.Int64(30000),
			}},
		},
		Tags: []sdktypes.Tag{{Key: aws.String("owner"), Value: aws.String("sre")}},
	})
	require.NoError(t, err)

	_, err = client.TagResource(ctx, &transcribesdk.TagResourceInput{
		ResourceArn: aws.String(
			"arn:aws:transcribe:us-east-1:123456789012:call-analytics-category/livesync-category",
		),
		Tags: []sdktypes.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	out, err := client.GetCallAnalyticsCategory(ctx, &transcribesdk.GetCallAnalyticsCategoryInput{
		CategoryName: aws.String("livesync-category"),
	})
	require.NoError(t, err)
	require.Equal(t,
		map[string]string{"owner": "sre", "env": "prod"},
		tagMapFromTranscribeTags(out.CategoryProperties.Tags),
	)

	listOut, err := client.ListCallAnalyticsCategories(ctx, &transcribesdk.ListCallAnalyticsCategoriesInput{})
	require.NoError(t, err)
	require.Len(t, listOut.Categories, 1)
	require.Equal(t,
		map[string]string{"owner": "sre", "env": "prod"},
		tagMapFromTranscribeTags(listOut.Categories[0].Tags),
	)
}

func TestGetCallAnalyticsJob_TagResource_LiveSync_RealClient(t *testing.T) {
	t.Parallel()

	h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
	client := newTranscribeSDKClient(t, h)
	ctx := t.Context()

	_, err := client.StartCallAnalyticsJob(ctx, &transcribesdk.StartCallAnalyticsJobInput{
		CallAnalyticsJobName: aws.String("livesync-ca-job"),
		Media:                &sdktypes.Media{MediaFileUri: aws.String("s3://b/f.mp3")},
		Tags:                 []sdktypes.Tag{{Key: aws.String("owner"), Value: aws.String("sre")}},
	})
	require.NoError(t, err)

	_, err = client.TagResource(ctx, &transcribesdk.TagResourceInput{
		ResourceArn: aws.String("arn:aws:transcribe:us-east-1:123456789012:call-analytics-job/livesync-ca-job"),
		Tags:        []sdktypes.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	out, err := client.GetCallAnalyticsJob(ctx, &transcribesdk.GetCallAnalyticsJobInput{
		CallAnalyticsJobName: aws.String("livesync-ca-job"),
	})
	require.NoError(t, err)
	require.Equal(t,
		map[string]string{"owner": "sre", "env": "prod"},
		tagMapFromTranscribeTags(out.CallAnalyticsJob.Tags),
	)
}

func TestGetMedicalScribeJob_TagResource_LiveSync_RealClient(t *testing.T) {
	t.Parallel()

	h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
	client := newTranscribeSDKClient(t, h)
	ctx := t.Context()

	_, err := client.StartMedicalScribeJob(ctx, &transcribesdk.StartMedicalScribeJobInput{
		MedicalScribeJobName: aws.String("livesync-scribe-job"),
		Media:                &sdktypes.Media{MediaFileUri: aws.String("s3://b/f.mp3")},
		DataAccessRoleArn:    aws.String("arn:aws:iam::123456789012:role/scribe-role"),
		OutputBucketName:     aws.String("my-output-bucket"),
		Settings: &sdktypes.MedicalScribeSettings{
			ShowSpeakerLabels: aws.Bool(true),
			MaxSpeakerLabels:  aws.Int32(2),
		},
		Tags: []sdktypes.Tag{{Key: aws.String("owner"), Value: aws.String("sre")}},
	})
	require.NoError(t, err)

	_, err = client.TagResource(ctx, &transcribesdk.TagResourceInput{
		ResourceArn: aws.String(
			"arn:aws:transcribe:us-east-1:123456789012:medical-scribe-job/livesync-scribe-job",
		),
		Tags: []sdktypes.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	out, err := client.GetMedicalScribeJob(ctx, &transcribesdk.GetMedicalScribeJobInput{
		MedicalScribeJobName: aws.String("livesync-scribe-job"),
	})
	require.NoError(t, err)
	require.Equal(t,
		map[string]string{"owner": "sre", "env": "prod"},
		tagMapFromTranscribeTags(out.MedicalScribeJob.Tags),
	)
}

func TestGetMedicalTranscriptionJob_TagResource_LiveSync_RealClient(t *testing.T) {
	t.Parallel()

	h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
	client := newTranscribeSDKClient(t, h)
	ctx := t.Context()

	_, err := client.StartMedicalTranscriptionJob(ctx, &transcribesdk.StartMedicalTranscriptionJobInput{
		MedicalTranscriptionJobName: aws.String("livesync-med-job"),
		LanguageCode:                sdktypes.LanguageCodeEnUs,
		Media:                       &sdktypes.Media{MediaFileUri: aws.String("s3://b/f.mp3")},
		OutputBucketName:            aws.String("my-output-bucket"),
		Specialty:                   sdktypes.SpecialtyPrimarycare,
		Type:                        sdktypes.TypeConversation,
		Tags:                        []sdktypes.Tag{{Key: aws.String("owner"), Value: aws.String("sre")}},
	})
	require.NoError(t, err)

	_, err = client.TagResource(ctx, &transcribesdk.TagResourceInput{
		ResourceArn: aws.String(
			"arn:aws:transcribe:us-east-1:123456789012:medical-transcription-job/livesync-med-job",
		),
		Tags: []sdktypes.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
	})
	require.NoError(t, err)

	out, err := client.GetMedicalTranscriptionJob(ctx, &transcribesdk.GetMedicalTranscriptionJobInput{
		MedicalTranscriptionJobName: aws.String("livesync-med-job"),
	})
	require.NoError(t, err)
	require.Equal(t,
		map[string]string{"owner": "sre", "env": "prod"},
		tagMapFromTranscribeTags(out.MedicalTranscriptionJob.Tags),
	)
}
