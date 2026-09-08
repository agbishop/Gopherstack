package transcribe_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/transcribe"
)

// ── Creation-time Tags are attached to the resource ARN ─────────────────────
//
// Real AWS treats the Tags parameter on Start*Job/Create* operations as
// immediately-attached resource tags, retrievable via ListTagsForResource
// for that resource's ARN (arn:aws:transcribe:<region>:<account>:<type>/<name>).
// Get/Describe/List outputs for vocabularies, filters, and language models never
// echo Tags directly, so ListTagsForResource is the only way to observe them.

func TestCreationTags_SyncToResourceARN(t *testing.T) {
	t.Parallel()

	const region = "us-east-1"
	const account = "123456789012"

	tests := []struct {
		create func(b *transcribe.InMemoryBackend) error
		name   string
		arn    string
	}{
		{
			name: "StartTranscriptionJob",
			arn:  "arn:aws:transcribe:" + region + ":" + account + ":transcription-job/tagged-ts-job",
			create: func(b *transcribe.InMemoryBackend) error {
				_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
					JobName:      "tagged-ts-job",
					LanguageCode: "en-US",
					Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
					Tags:         map[string]string{"team": "asr"},
				})

				return err
			},
		},
		{
			name: "StartCallAnalyticsJob",
			arn:  "arn:aws:transcribe:" + region + ":" + account + ":call-analytics-job/tagged-ca-job",
			create: func(b *transcribe.InMemoryBackend) error {
				_, err := b.StartCallAnalyticsJob(&transcribe.CallAnalyticsJob{
					CallAnalyticsJobName: "tagged-ca-job",
					LanguageCode:         "en-US",
					Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
					Tags:                 map[string]string{"team": "ca"},
				})

				return err
			},
		},
		{
			name: "StartMedicalScribeJob",
			arn:  "arn:aws:transcribe:" + region + ":" + account + ":medical-scribe-job/tagged-scribe-job",
			create: func(b *transcribe.InMemoryBackend) error {
				_, err := b.StartMedicalScribeJob(&transcribe.MedicalScribeJob{
					MedicalScribeJobName: "tagged-scribe-job",
					Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
					DataAccessRoleArn:    "arn:aws:iam::123456789012:role/Scribe",
					OutputBucketName:     "scribe-out",
					Settings: &transcribe.MedicalScribeSettings{
						ShowSpeakerLabels: true, MaxSpeakerLabels: 2,
					},
					Tags: map[string]string{"team": "scribe"},
				})

				return err
			},
		},
		{
			name: "StartMedicalTranscriptionJob",
			arn:  "arn:aws:transcribe:" + region + ":" + account + ":medical-transcription-job/tagged-mt-job",
			create: func(b *transcribe.InMemoryBackend) error {
				_, err := b.StartMedicalTranscriptionJob(&transcribe.MedicalTranscriptionJob{
					MedicalTranscriptionJobName: "tagged-mt-job",
					LanguageCode:                "en-US",
					Specialty:                   "PRIMARYCARE",
					Type:                        "DICTATION",
					Media:                       transcribe.Media{MediaFileURI: "s3://b/f"},
					Tags:                        map[string]string{"team": "medical"},
				})

				return err
			},
		},
		{
			name: "CreateVocabulary",
			arn:  "arn:aws:transcribe:" + region + ":" + account + ":vocabulary/tagged-vocab",
			create: func(b *transcribe.InMemoryBackend) error {
				_, err := b.CreateVocabulary(&transcribe.Vocabulary{
					VocabularyName: "tagged-vocab",
					LanguageCode:   "en-US",
					Phrases:        []string{"hello"},
					Tags:           map[string]string{"team": "vocab"},
				})

				return err
			},
		},
		{
			name: "CreateVocabularyFilter",
			arn:  "arn:aws:transcribe:" + region + ":" + account + ":vocabulary-filter/tagged-filter",
			create: func(b *transcribe.InMemoryBackend) error {
				_, err := b.CreateVocabularyFilter(&transcribe.VocabularyFilter{
					VocabularyFilterName: "tagged-filter",
					LanguageCode:         "en-US",
					Words:                []string{"bleep"},
					Tags:                 map[string]string{"team": "filter"},
				})

				return err
			},
		},
		{
			name: "CreateMedicalVocabulary",
			arn:  "arn:aws:transcribe:" + region + ":" + account + ":medical-vocabulary/tagged-med-vocab",
			create: func(b *transcribe.InMemoryBackend) error {
				_, err := b.CreateMedicalVocabulary(
					"tagged-med-vocab", "en-US", "s3://bucket/v.txt", map[string]string{"team": "med-vocab"},
				)

				return err
			},
		},
		{
			name: "CreateLanguageModel",
			arn:  "arn:aws:transcribe:" + region + ":" + account + ":language-model/tagged-model",
			create: func(b *transcribe.InMemoryBackend) error {
				_, err := b.CreateLanguageModel(&transcribe.LanguageModel{
					ModelName:     "tagged-model",
					BaseModelName: "WideBand",
					LanguageCode:  "en-US",
					InputDataConfig: &transcribe.InputDataConfig{
						S3Uri:             "s3://bucket/training/",
						DataAccessRoleArn: "arn:aws:iam::123456789012:role/TranscribeRole",
					},
					Tags: map[string]string{"team": "model"},
				})

				return err
			},
		},
		{
			name: "CreateCallAnalyticsCategory",
			arn:  "arn:aws:transcribe:" + region + ":" + account + ":call-analytics-category/tagged-category",
			create: func(b *transcribe.InMemoryBackend) error {
				_, err := b.CreateCallAnalyticsCategory(&transcribe.CallAnalyticsCategory{
					CategoryName: "tagged-category",
					InputType:    "POST_CALL",
					Rules: []transcribe.CallAnalyticsRule{
						{NonTalkTimeFilter: &transcribe.NonTalkTimeFilter{Threshold: 30000}},
					},
					Tags: map[string]string{"team": "category"},
				})

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := transcribe.NewInMemoryBackend()
			require.NoError(t, tt.create(b))

			tags, err := b.ListTagsForResource(tt.arn)
			require.NoError(t, err)
			assert.NotEmpty(t, tags, "expected tags recorded for ARN %s", tt.arn)
		})
	}
}

// ── Deleting a resource forgets its tags ─────────────────────────────────────

func TestDelete_ForgetsResourceTags(t *testing.T) {
	t.Parallel()

	const arn = "arn:aws:transcribe:us-east-1:123456789012:transcription-job/deleted-tagged-job"

	b := transcribe.NewInMemoryBackend()

	_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
		JobName:      "deleted-tagged-job",
		LanguageCode: "en-US",
		Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
		Tags:         map[string]string{"team": "asr"},
	})
	require.NoError(t, err)

	tags, err := b.ListTagsForResource(arn)
	require.NoError(t, err)
	assert.NotEmpty(t, tags)

	require.NoError(t, b.DeleteTranscriptionJob("deleted-tagged-job"))

	tags, err = b.ListTagsForResource(arn)
	require.NoError(t, err)
	assert.Empty(t, tags, "tags should be forgotten once the resource is deleted")
}

// ── Untagged creation does not fabricate an empty tag entry ─────────────────

func TestCreationWithoutTags_LeavesResourceTagsEmpty(t *testing.T) {
	t.Parallel()

	const arn = "arn:aws:transcribe:us-east-1:123456789012:transcription-job/untagged-job"

	b := transcribe.NewInMemoryBackend()

	_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
		JobName:      "untagged-job",
		LanguageCode: "en-US",
		Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
	})
	require.NoError(t, err)

	tags, err := b.ListTagsForResource(arn)
	require.NoError(t, err)
	assert.Empty(t, tags)
}

// ── Tags operations ───────────────────────────────────────────────────────────

func TestTagResource_StoresAndReturns(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()

	err := b.TagResource("arn:aws:transcribe:us-east-1:123456789012:transcriptionjob/my-job",
		map[string]string{"env": "prod", "team": "ml"})
	require.NoError(t, err)

	tags, err := b.ListTagsForResource("arn:aws:transcribe:us-east-1:123456789012:transcriptionjob/my-job")
	require.NoError(t, err)
	assert.Equal(t, "prod", tags["env"])
	assert.Equal(t, "ml", tags["team"])
}

func TestUntagResource_RemovesKeys(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	arn := "arn:aws:transcribe:us-east-1:123456789012:transcriptionjob/my-job"

	require.NoError(t, b.TagResource(arn, map[string]string{"env": "prod", "team": "ml", "owner": "alice"}))
	require.NoError(t, b.UntagResource(arn, []string{"env", "owner"}))

	tags, err := b.ListTagsForResource(arn)
	require.NoError(t, err)
	assert.Len(t, tags, 1)
	assert.Equal(t, "ml", tags["team"])
}

func TestListTagsForResource_UnknownARN_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	tags, err := b.ListTagsForResource("arn:aws:transcribe:us-east-1:123456789012:transcriptionjob/none")
	require.NoError(t, err)
	assert.Empty(t, tags)
}

func TestHTTP_TagResource(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	rec := doTranscribeRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": "arn:aws:transcribe:us-east-1:123456789012:transcriptionjob/my-job",
		"Tags":        []map[string]string{{"Key": "env", "Value": "prod"}},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHTTP_ListTagsForResource_ReturnsStoredTags(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	arn := "arn:aws:transcribe:us-east-1:123456789012:transcriptionjob/my-job"

	doTranscribeRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": arn,
		"Tags": []map[string]string{
			{"Key": "env", "Value": "prod"},
			{"Key": "team", "Value": "ml"},
		},
	})

	rec := doTranscribeRequest(t, h, "ListTagsForResource", map[string]any{
		"ResourceArn": arn,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Tags []struct {
			Key   string `json:"Key"`
			Value string `json:"Value"`
		} `json:"Tags"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	tagMap := make(map[string]string, len(resp.Tags))
	for _, tag := range resp.Tags {
		tagMap[tag.Key] = tag.Value
	}
	assert.Equal(t, "prod", tagMap["env"])
	assert.Equal(t, "ml", tagMap["team"])
}

func TestHTTP_UntagResource(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	arn := "arn:aws:transcribe:us-east-1:123456789012:transcriptionjob/my-job"

	doTranscribeRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": arn,
		"Tags": []map[string]string{
			{"Key": "env", "Value": "prod"},
			{"Key": "team", "Value": "ml"},
		},
	})

	doTranscribeRequest(t, h, "UntagResource", map[string]any{
		"ResourceArn": arn,
		"TagKeys":     []string{"env"},
	})

	rec := doTranscribeRequest(t, h, "ListTagsForResource", map[string]any{
		"ResourceArn": arn,
	})
	var resp struct {
		Tags []struct {
			Key   string `json:"Key"`
			Value string `json:"Value"`
		} `json:"Tags"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	tagMap := make(map[string]string, len(resp.Tags))
	for _, tag := range resp.Tags {
		tagMap[tag.Key] = tag.Value
	}
	assert.NotContains(t, tagMap, "env")
	assert.Equal(t, "ml", tagMap["team"])
}

// ── Backend Reset clears resourceTags ────────────────────────────────────────

func TestReset_ClearsResourceTags(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	arn := "arn:aws:transcribe:us-east-1:123456789012:transcriptionjob/my-job"
	require.NoError(t, b.TagResource(arn, map[string]string{"k": "v"}))

	b.Reset()

	tags, err := b.ListTagsForResource(arn)
	require.NoError(t, err)
	assert.Empty(t, tags)
}
