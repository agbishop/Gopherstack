package transcribe_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	transcribesdk "github.com/aws/aws-sdk-go-v2/service/transcribe"
	sdktypes "github.com/aws/aws-sdk-go-v2/service/transcribe/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/transcribe"
)

// TestListVocabularies_LastModifiedTime_RealClient proves ListVocabularies'
// per-item VocabularyInfo carries LastModifiedTime, matching the real
// deserializer (deserializers.go's VocabularyInfo case, shared with
// ListMedicalVocabularies) rather than the trimmed three-field shape this
// service previously emitted.
func TestListVocabularies_LastModifiedTime_RealClient(t *testing.T) {
	t.Parallel()

	h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
	client := newTranscribeSDKClient(t, h)
	ctx := t.Context()

	_, err := client.CreateVocabulary(ctx, &transcribesdk.CreateVocabularyInput{
		VocabularyName: aws.String("wire-vocab"),
		LanguageCode:   sdktypes.LanguageCodeEnUs,
		Phrases:        []string{"hello"},
	})
	require.NoError(t, err)

	out, err := client.ListVocabularies(ctx, &transcribesdk.ListVocabulariesInput{})
	require.NoError(t, err)
	require.Len(t, out.Vocabularies, 1)
	require.NotNil(t, out.Vocabularies[0].LastModifiedTime, "VocabularyInfo.LastModifiedTime must round-trip")
}

// TestListMedicalVocabularies_LastModifiedTime_RealClient is
// ListVocabularies' sibling above, for the medical vocabulary family - the
// same VocabularyInfo type is shared by both real deserializers.
func TestListMedicalVocabularies_LastModifiedTime_RealClient(t *testing.T) {
	t.Parallel()

	h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
	client := newTranscribeSDKClient(t, h)
	ctx := t.Context()

	_, err := client.CreateMedicalVocabulary(ctx, &transcribesdk.CreateMedicalVocabularyInput{
		VocabularyName:    aws.String("wire-medical-vocab"),
		LanguageCode:      sdktypes.LanguageCodeEnUs,
		VocabularyFileUri: aws.String("s3://b/vocab.txt"),
	})
	require.NoError(t, err)

	out, err := client.ListMedicalVocabularies(ctx, &transcribesdk.ListMedicalVocabulariesInput{})
	require.NoError(t, err)
	require.Len(t, out.Vocabularies, 1)
	require.NotNil(t, out.Vocabularies[0].LastModifiedTime, "VocabularyInfo.LastModifiedTime must round-trip")
}

// TestCallAnalyticsSettings_LanguageIdSettings_RealClient proves
// CallAnalyticsSettings.LanguageIdSettings round-trips through
// StartCallAnalyticsJob/GetCallAnalyticsJob, matching the real
// CallAnalyticsJobSettings type (types/types.go:246), which this service
// never modelled at all.
func TestCallAnalyticsSettings_LanguageIdSettings_RealClient(t *testing.T) {
	t.Parallel()

	h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
	client := newTranscribeSDKClient(t, h)
	ctx := t.Context()

	_, err := client.StartCallAnalyticsJob(ctx, &transcribesdk.StartCallAnalyticsJobInput{
		CallAnalyticsJobName: aws.String("wire-ca-job"),
		Media:                &sdktypes.Media{MediaFileUri: aws.String("s3://b/f.mp3")},
		Settings: &sdktypes.CallAnalyticsJobSettings{
			LanguageIdSettings: map[string]sdktypes.LanguageIdSettings{
				"en-US": {VocabularyName: aws.String("en-vocab")},
			},
		},
	})
	require.NoError(t, err)

	out, err := client.GetCallAnalyticsJob(ctx, &transcribesdk.GetCallAnalyticsJobInput{
		CallAnalyticsJobName: aws.String("wire-ca-job"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.CallAnalyticsJob.Settings)
	require.Contains(t, out.CallAnalyticsJob.Settings.LanguageIdSettings, "en-US")
	require.Equal(t,
		"en-vocab",
		aws.ToString(out.CallAnalyticsJob.Settings.LanguageIdSettings["en-US"].VocabularyName),
	)
}

// TestCallAnalyticsRule_TimeRanges_RealClient proves the AbsoluteTimeRange and
// RelativeTimeRange sub-parameters round-trip on a Call Analytics category
// rule, matching the real Rule union's four filter types (all four embed
// both ranges per types/types.go), none of which this service modelled.
func TestCallAnalyticsRule_TimeRanges_RealClient(t *testing.T) {
	t.Parallel()

	h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
	client := newTranscribeSDKClient(t, h)
	ctx := t.Context()

	_, err := client.CreateCallAnalyticsCategory(ctx, &transcribesdk.CreateCallAnalyticsCategoryInput{
		CategoryName: aws.String("wire-time-range-category"),
		Rules: []sdktypes.Rule{
			&sdktypes.RuleMemberInterruptionFilter{Value: sdktypes.InterruptionFilter{
				AbsoluteTimeRange: &sdktypes.AbsoluteTimeRange{
					StartTime: aws.Int64(1000),
					EndTime:   aws.Int64(5000),
				},
				RelativeTimeRange: &sdktypes.RelativeTimeRange{
					StartPercentage: aws.Int32(10),
					EndPercentage:   aws.Int32(50),
				},
			}},
		},
	})
	require.NoError(t, err)

	out, err := client.GetCallAnalyticsCategory(ctx, &transcribesdk.GetCallAnalyticsCategoryInput{
		CategoryName: aws.String("wire-time-range-category"),
	})
	require.NoError(t, err)
	require.Len(t, out.CategoryProperties.Rules, 1)

	member, ok := out.CategoryProperties.Rules[0].(*sdktypes.RuleMemberInterruptionFilter)
	require.True(t, ok)
	require.NotNil(t, member.Value.AbsoluteTimeRange, "InterruptionFilter.AbsoluteTimeRange must round-trip")
	require.Equal(t, int64(1000), aws.ToInt64(member.Value.AbsoluteTimeRange.StartTime))
	require.Equal(t, int64(5000), aws.ToInt64(member.Value.AbsoluteTimeRange.EndTime))
	require.NotNil(t, member.Value.RelativeTimeRange, "InterruptionFilter.RelativeTimeRange must round-trip")
	require.Equal(t, int32(10), aws.ToInt32(member.Value.RelativeTimeRange.StartPercentage))
	require.Equal(t, int32(50), aws.ToInt32(member.Value.RelativeTimeRange.EndPercentage))
}

// TestMedicalScribeSettings_ClinicalNoteGenerationSettings_RealClient proves
// ClinicalNoteGenerationSettings round-trips nested under
// StartMedicalScribeJobInput.Settings / MedicalScribeJob.Settings, matching
// the real MedicalScribeSettings type (types/types.go:1058). The real
// StartMedicalScribeJobInput has no top-level ClinicalNoteGenerationSettings
// member at all, so a real client can only ever set this nested - the flat
// top-level field this service previously used was unreachable in both
// directions.
func TestMedicalScribeSettings_ClinicalNoteGenerationSettings_RealClient(t *testing.T) {
	t.Parallel()

	h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
	client := newTranscribeSDKClient(t, h)
	ctx := t.Context()

	_, err := client.StartMedicalScribeJob(ctx, &transcribesdk.StartMedicalScribeJobInput{
		MedicalScribeJobName: aws.String("wire-scribe-job"),
		Media:                &sdktypes.Media{MediaFileUri: aws.String("s3://b/f.mp3")},
		DataAccessRoleArn:    aws.String("arn:aws:iam::123456789012:role/ScribeRole"),
		OutputBucketName:     aws.String("scribe-output"),
		Settings: &sdktypes.MedicalScribeSettings{
			ClinicalNoteGenerationSettings: &sdktypes.ClinicalNoteGenerationSettings{
				NoteTemplate: sdktypes.MedicalScribeNoteTemplateBirp,
			},
			ShowSpeakerLabels: aws.Bool(true),
			MaxSpeakerLabels:  aws.Int32(2),
		},
	})
	require.NoError(t, err)

	out, err := client.GetMedicalScribeJob(ctx, &transcribesdk.GetMedicalScribeJobInput{
		MedicalScribeJobName: aws.String("wire-scribe-job"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.MedicalScribeJob.Settings)
	require.NotNil(t, out.MedicalScribeJob.Settings.ClinicalNoteGenerationSettings,
		"MedicalScribeSettings.ClinicalNoteGenerationSettings must round-trip nested under Settings")
	require.Equal(t,
		sdktypes.MedicalScribeNoteTemplateBirp,
		out.MedicalScribeJob.Settings.ClinicalNoteGenerationSettings.NoteTemplate,
	)
}

// TestStartCallAnalyticsJob_Summarization_GenerateAbstractiveSummary_RealClient
// proves gopherstack-zquj: SummarizationSettings tagged its bool
// json:"GenerateSummary", but the real Summarization deserializer switches
// on "GenerateAbstractiveSummary" (types.go:1750-1763, transcribe SDK) --
// every real client read Settings.Summarization.GenerateAbstractiveSummary
// as false/nil regardless of what was requested.
func TestStartCallAnalyticsJob_Summarization_GenerateAbstractiveSummary_RealClient(t *testing.T) {
	t.Parallel()

	h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
	client := newTranscribeSDKClient(t, h)
	ctx := t.Context()

	_, err := client.StartCallAnalyticsJob(ctx, &transcribesdk.StartCallAnalyticsJobInput{
		CallAnalyticsJobName: aws.String("wire-summarization-job"),
		Media:                &sdktypes.Media{MediaFileUri: aws.String("s3://bucket/call.wav")},
		Settings: &sdktypes.CallAnalyticsJobSettings{
			Summarization: &sdktypes.Summarization{
				GenerateAbstractiveSummary: aws.Bool(true),
			},
		},
	})
	require.NoError(t, err)

	out, err := client.GetCallAnalyticsJob(ctx, &transcribesdk.GetCallAnalyticsJobInput{
		CallAnalyticsJobName: aws.String("wire-summarization-job"),
	})
	require.NoError(t, err)
	require.NotNil(t, out.CallAnalyticsJob.Settings)
	require.NotNil(t, out.CallAnalyticsJob.Settings.Summarization)
	require.NotNil(t, out.CallAnalyticsJob.Settings.Summarization.GenerateAbstractiveSummary)
	require.True(t, aws.ToBool(out.CallAnalyticsJob.Settings.Summarization.GenerateAbstractiveSummary))
}
