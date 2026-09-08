package transcribe_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	sdktypes "github.com/aws/aws-sdk-go-v2/service/transcribe/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/transcribe"
)

func TestStartTranscriptionJob_FullInput(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	job, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
		JobName:              "full-input-job",
		LanguageCode:         "en-US",
		MediaFormat:          "mp3",
		MediaSampleRateHertz: 16000,
		Media:                transcribe.Media{MediaFileURI: "s3://bucket/audio.mp3"},
		OutputBucketName:     "my-output-bucket",
		OutputKey:            "results/job.json",
		Settings: &transcribe.TranscriptionSettings{
			ShowSpeakerLabels: true,
			MaxSpeakerLabels:  4,
			ShowAlternatives:  true,
			MaxAlternatives:   3,
		},
		ContentRedaction: &transcribe.ContentRedaction{
			RedactionType:   "PII",
			RedactionOutput: "redacted",
		},
		Subtitles: &transcribe.SubtitlesOutput{
			Formats: []string{"vtt", "srt"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "COMPLETED", job.JobStatus)
	assert.Equal(t, "en-US", job.LanguageCode)
	assert.Equal(t, "mp3", job.MediaFormat)
	assert.Equal(t, int32(16000), job.MediaSampleRateHertz)
	assert.Equal(t, "my-output-bucket", job.OutputBucketName)
	assert.NotNil(t, job.ContentRedaction)
	assert.NotNil(t, job.Subtitles)
	assert.Len(t, job.Subtitles.Formats, 2)
	assert.Len(t, job.Subtitles.SubtitleFileURIs, 2)
	assert.NotEmpty(t, job.TranscriptText)
	assert.NotEmpty(t, job.TranscriptJSON)
	assert.False(t, job.CreationTime.IsZero())
	assert.False(t, job.CompletionTime.IsZero())
}

func TestTranscriptionJobName_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		jobName string
		wantErr bool
	}{
		{name: "valid_alphanumeric", jobName: "my-job-123", wantErr: false},
		{name: "valid_dots_underscores", jobName: "my.job_test", wantErr: false},
		{name: "valid_200_chars", jobName: strings.Repeat("a", 200), wantErr: false},
		{name: "empty_name", jobName: "", wantErr: true},
		{name: "space_in_name", jobName: "bad name", wantErr: true},
		{name: "slash_in_name", jobName: "bad/name", wantErr: true},
		{name: "too_long_201_chars", jobName: strings.Repeat("a", 201), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := transcribe.NewInMemoryBackend()
			_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
				JobName:      tc.jobName,
				LanguageCode: "en-US",
				Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			})

			if tc.wantErr {
				require.ErrorIs(t, err, transcribe.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestErrValidationSentinel verifies ErrValidation wraps ErrInvalidParameter.
func TestErrValidationSentinel(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	_, err := b.StartTranscriptionJob(
		&transcribe.TranscriptionJob{
			JobName:      "",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: ""},
		},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, transcribe.ErrValidation)
}

// TestErrAlreadyExistsSentinel verifies ErrAlreadyExists is returned on duplicate.
func TestErrAlreadyExistsSentinel(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	_, err := b.StartTranscriptionJob(
		&transcribe.TranscriptionJob{
			JobName:      "dup",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
		},
	)
	require.NoError(t, err)

	_, err = b.StartTranscriptionJob(
		&transcribe.TranscriptionJob{
			JobName:      "dup",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
		},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, transcribe.ErrAlreadyExists)
}

func TestLanguageCode_Validation(t *testing.T) {
	t.Parallel()

	t.Run("valid_language_code_accepted", func(t *testing.T) {
		t.Parallel()

		validCodes := []string{"en-US", "es-US", "fr-FR", "de-DE", "ja-JP", "zh-CN", "ko-KR"}
		b := transcribe.NewInMemoryBackend()

		for i, code := range validCodes {
			_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
				JobName:      "job-valid-lang-" + code + "-" + string(rune('a'+i)),
				LanguageCode: code,
				Media:        transcribe.Media{MediaFileURI: "s3://b/f.mp3"},
			})
			require.NoError(t, err, "expected %s to be valid", code)
		}
	})

	t.Run("invalid_language_code_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-bad-lang",
			LanguageCode: "xx-XX",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("empty_language_code_without_identify_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName: "job-no-lang",
			Media:   transcribe.Media{MediaFileURI: "s3://b/f"},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	// Anti-drift: every code the pinned SDK's types.LanguageCode enum knows about
	// must validate. Catches a future hand-maintained allowlist falling behind again.
	t.Run("every_sdk_enum_code_accepted", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()

		for i, code := range sdktypes.LanguageCode("").Values() {
			_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
				JobName:      fmt.Sprintf("job-sdk-lang-%d", i),
				LanguageCode: string(code),
				Media:        transcribe.Media{MediaFileURI: "s3://b/f.mp3"},
			})
			require.NoError(t, err, "expected SDK LanguageCode %s to be accepted", code)
		}
	})
}

func TestIdentifyLanguage(t *testing.T) {
	t.Parallel()

	t.Run("identify_language_allows_empty_language_code", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		job, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:          "job-identify",
			IdentifyLanguage: true,
			LanguageOptions:  []string{"en-US", "es-US", "fr-FR"},
			Media:            transcribe.Media{MediaFileURI: "s3://b/f"},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, job.LanguageCode)
		assert.Greater(t, job.IdentifiedLanguageScore, float32(0))
	})

	t.Run("identify_multiple_languages_accepts_options", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		job, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:                   "job-identify-multi",
			IdentifyMultipleLanguages: true,
			LanguageOptions:           []string{"en-US", "es-US"},
			Media:                     transcribe.Media{MediaFileURI: "s3://b/f"},
		})
		require.NoError(t, err)
		assert.NotEmpty(t, job.LanguageCode)
	})

	t.Run("too_few_language_options_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:          "job-few-opts",
			IdentifyLanguage: true,
			LanguageOptions:  []string{"en-US"},
			Media:            transcribe.Media{MediaFileURI: "s3://b/f"},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("too_many_language_options_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		opts := make([]string, 11)
		for i := range opts {
			opts[i] = "en-US"
		}

		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:          "job-many-opts",
			IdentifyLanguage: true,
			LanguageOptions:  opts,
			Media:            transcribe.Media{MediaFileURI: "s3://b/f"},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("language_id_settings_accepted_for_identify_language", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		job, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:          "job-lang-id-settings-ok",
			IdentifyLanguage: true,
			LanguageOptions:  []string{"en-US", "es-US"},
			LanguageIDSettings: map[string]transcribe.LanguageIDSettings{
				"en-US": {VocabularyName: "vocab1", LanguageModelName: "model1"},
			},
			Media: transcribe.Media{MediaFileURI: "s3://b/f"},
		})
		require.NoError(t, err)
		assert.Equal(t, "model1", job.LanguageIDSettings["en-US"].LanguageModelName)
	})

	t.Run("language_id_settings_too_many_entries_rejected", func(t *testing.T) {
		t.Parallel()

		settings := make(map[string]transcribe.LanguageIDSettings, 6)
		for _, code := range []string{"en-US", "es-US", "fr-FR", "de-DE", "ja-JP", "ko-KR"} {
			settings[code] = transcribe.LanguageIDSettings{VocabularyName: "v"}
		}

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:            "job-lang-id-settings-too-many",
			IdentifyLanguage:   true,
			LanguageIDSettings: settings,
			Media:              transcribe.Media{MediaFileURI: "s3://b/f"},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("language_id_settings_unsupported_code_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:          "job-lang-id-settings-bad-code",
			IdentifyLanguage: true,
			LanguageIDSettings: map[string]transcribe.LanguageIDSettings{
				"xx-XX": {VocabularyName: "v"},
			},
			Media: transcribe.Media{MediaFileURI: "s3://b/f"},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("language_id_settings_model_name_rejected_with_identify_multiple", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:                   "job-lang-id-settings-multi-model",
			IdentifyMultipleLanguages: true,
			LanguageOptions:           []string{"en-US", "es-US"},
			LanguageIDSettings: map[string]transcribe.LanguageIDSettings{
				"en-US": {LanguageModelName: "model1"},
			},
			Media: transcribe.Media{MediaFileURI: "s3://b/f"},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})
}

func TestMediaFormat_Validation(t *testing.T) {
	t.Parallel()

	validFormats := []string{"mp3", "mp4", "wav", "flac", "ogg", "amr", "webm", "m4a"}

	for _, format := range validFormats {
		t.Run("valid_format_"+format, func(t *testing.T) {
			t.Parallel()

			b := transcribe.NewInMemoryBackend()
			_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
				JobName:      "job-format-" + format,
				LanguageCode: "en-US",
				MediaFormat:  format,
				Media:        transcribe.Media{MediaFileURI: "s3://b/f." + format},
			})
			require.NoError(t, err)
		})
	}

	t.Run("invalid_format_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-bad-format",
			LanguageCode: "en-US",
			MediaFormat:  "avi",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f.avi"},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})
}

func TestMediaSampleRateHertz_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rate    int32
		wantErr bool
	}{
		{name: "valid_8000", rate: 8000, wantErr: false},
		{name: "valid_16000", rate: 16000, wantErr: false},
		{name: "valid_48000", rate: 48000, wantErr: false},
		{name: "zero_accepted", rate: 0, wantErr: false},
		{name: "below_8000_rejected", rate: 7999, wantErr: true},
		{name: "above_48000_rejected", rate: 48001, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := transcribe.NewInMemoryBackend()
			_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
				JobName:              "job-rate-" + tc.name,
				LanguageCode:         "en-US",
				MediaSampleRateHertz: tc.rate,
				Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			})

			if tc.wantErr {
				require.ErrorIs(t, err, transcribe.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSettings_Validation(t *testing.T) {
	t.Parallel()

	t.Run("valid_settings_accepted", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-settings-ok",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			Settings: &transcribe.TranscriptionSettings{
				ShowSpeakerLabels:      true,
				MaxSpeakerLabels:       4,
				ShowAlternatives:       true,
				MaxAlternatives:        3,
				VocabularyFilterMethod: "mask",
			},
		})
		require.NoError(t, err)
	})

	t.Run("invalid_max_speaker_labels_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-bad-speakers",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			Settings: &transcribe.TranscriptionSettings{
				ShowSpeakerLabels: true,
				MaxSpeakerLabels:  1,
			},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("show_speaker_labels_without_max_speaker_labels_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-speakers-no-max",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			Settings:     &transcribe.TranscriptionSettings{ShowSpeakerLabels: true},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("max_speaker_labels_without_show_speaker_labels_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-max-speakers-no-show",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			Settings:     &transcribe.TranscriptionSettings{MaxSpeakerLabels: 4},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("show_alternatives_without_max_alternatives_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-alts-no-max",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			Settings:     &transcribe.TranscriptionSettings{ShowAlternatives: true},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("max_alternatives_without_show_alternatives_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-max-alts-no-show",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			Settings:     &transcribe.TranscriptionSettings{MaxAlternatives: 3},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("invalid_vocabulary_filter_method_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-bad-filter-method",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			Settings: &transcribe.TranscriptionSettings{
				VocabularyFilterMethod: "invalid-method",
			},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})
}

func TestContentRedaction_Validation(t *testing.T) {
	t.Parallel()

	t.Run("valid_pii_redaction_accepted", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-redact-ok",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			ContentRedaction: &transcribe.ContentRedaction{
				RedactionType:   "PII",
				RedactionOutput: "redacted_and_unredacted",
			},
		})
		require.NoError(t, err)
	})

	t.Run("missing_redaction_type_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:          "job-redact-no-type",
			LanguageCode:     "en-US",
			Media:            transcribe.Media{MediaFileURI: "s3://b/f"},
			ContentRedaction: &transcribe.ContentRedaction{RedactionOutput: "redacted"},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("invalid_redaction_type_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-bad-redact-type",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			ContentRedaction: &transcribe.ContentRedaction{
				RedactionType: "NOTPII",
			},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("missing_redaction_output_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-redact-no-output",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			ContentRedaction: &transcribe.ContentRedaction{
				RedactionType: "PII",
			},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})
}

func TestSubtitles_Validation(t *testing.T) {
	t.Parallel()

	t.Run("valid_subtitle_formats_accepted", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		job, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-subtitles-ok",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			Subtitles:    &transcribe.SubtitlesOutput{Formats: []string{"vtt", "srt"}},
		})
		require.NoError(t, err)
		assert.Len(t, job.Subtitles.SubtitleFileURIs, 2)
		assert.Contains(t, job.Subtitles.SubtitleFileURIs[0], ".vtt")
		assert.Contains(t, job.Subtitles.SubtitleFileURIs[1], ".srt")
	})

	t.Run("invalid_subtitle_format_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName:      "job-bad-subtitle",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
			Subtitles:    &transcribe.SubtitlesOutput{Formats: []string{"txt"}},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})
}

func TestTranscriptJSON_Generated(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	job, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
		JobName:      "job-transcript-json",
		LanguageCode: "en-US",
		Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, job.TranscriptJSON)
	assert.Contains(t, string(job.TranscriptJSON), "results")
	assert.Contains(t, string(job.TranscriptJSON), "transcripts")
	assert.Contains(t, string(job.TranscriptJSON), "items")
}

func TestOutputBucketName_Routing(t *testing.T) {
	t.Parallel()

	h, b := newHandlerWithBackend(t)

	_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
		JobName:          "job-output-bucket",
		LanguageCode:     "en-US",
		Media:            transcribe.Media{MediaFileURI: "s3://b/f"},
		OutputBucketName: "my-results-bucket",
		OutputKey:        "prefix/my-job.json",
	})
	require.NoError(t, err)

	rec := doTranscribeRequest(t, h, "GetTranscriptionJob", map[string]any{
		"TranscriptionJobName": "job-output-bucket",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "my-results-bucket")
}

func TestJobExecutionSettings_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		settings *transcribe.JobExecutionSettings
		name     string
		wantErr  bool
	}{
		{name: "unset", settings: nil},
		{name: "role_without_deferred", settings: &transcribe.JobExecutionSettings{DataAccessRoleArn: "role"}},
		{
			name: "deferred_with_role",
			settings: &transcribe.JobExecutionSettings{
				AllowDeferredExecution: true,
				DataAccessRoleArn:      "arn:aws:iam::123456789012:role/transcribe",
			},
		},
		{
			name:     "deferred_without_role",
			settings: &transcribe.JobExecutionSettings{AllowDeferredExecution: true},
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			job, err := transcribe.NewInMemoryBackend().StartTranscriptionJob(&transcribe.TranscriptionJob{
				JobName:              "job-execution-" + tc.name,
				LanguageCode:         "en-US",
				Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
				JobExecutionSettings: tc.settings,
			})
			if tc.wantErr {
				require.ErrorIs(t, err, transcribe.ErrValidation)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.settings, job.JobExecutionSettings)
		})
	}
}

func TestDeferredTranscriptionJob_Lifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		failureReason string
		finalStatus   string
	}{
		{name: "completes", finalStatus: "COMPLETED"},
		{name: "fails", failureReason: "synthetic failure", finalStatus: "FAILED"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := transcribe.NewInMemoryBackend()
			job, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
				JobName:       "deferred-" + tc.name,
				LanguageCode:  "en-US",
				Media:         transcribe.Media{MediaFileURI: "s3://b/f"},
				FailureReason: tc.failureReason,
				JobExecutionSettings: &transcribe.JobExecutionSettings{
					AllowDeferredExecution: true,
					DataAccessRoleArn:      "arn:aws:iam::123456789012:role/transcribe",
				},
			})
			require.NoError(t, err)
			assert.Equal(t, "QUEUED", job.JobStatus)

			jobs, _ := b.ListTranscriptionJobs("QUEUED", "", "", 0)
			assert.Len(t, jobs, 1)

			job, err = b.GetTranscriptionJob(job.JobName)
			require.NoError(t, err)
			assert.Equal(t, "IN_PROGRESS", job.JobStatus)

			job, err = b.GetTranscriptionJob(job.JobName)
			require.NoError(t, err)
			assert.Equal(t, tc.finalStatus, job.JobStatus)
			assert.False(t, job.CompletionTime.IsZero())
		})
	}
}

func TestHTTP_StartTranscriptionJob_FullInput(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	rec := doTranscribeRequest(t, h, "StartTranscriptionJob", map[string]any{
		"TranscriptionJobName": "http-full-job",
		"LanguageCode":         "en-US",
		"MediaFormat":          "wav",
		"MediaSampleRateHertz": 16000,
		"Media":                map[string]any{"MediaFileUri": "s3://bucket/audio.wav"},
		"OutputBucketName":     "my-output",
		"Settings": map[string]any{
			"ShowSpeakerLabels": true,
			"MaxSpeakerLabels":  3,
		},
		"ContentRedaction": map[string]any{
			"RedactionType":   "PII",
			"RedactionOutput": "redacted",
		},
		"JobExecutionSettings": map[string]any{
			"AllowDeferredExecution": true,
			"DataAccessRoleArn":      "arn:aws:iam::123456789012:role/transcribe",
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "http-full-job")
	assert.Contains(t, body, "QUEUED")
	assert.Contains(t, body, "JobExecutionSettings")
}

func TestDeleteTranscriptionJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*testing.T, *transcribe.Handler)
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *transcribe.Handler) {
				t.Helper()
				_, err := h.Backend.StartTranscriptionJob(
					&transcribe.TranscriptionJob{
						JobName:      "job-to-delete",
						LanguageCode: "en-US",
						Media:        transcribe.Media{MediaFileURI: "s3://bucket/file.mp4"},
					},
				)
				require.NoError(t, err)
			},
			body:     map[string]any{"TranscriptionJobName": "job-to-delete"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			setup:    func(_ *testing.T, _ *transcribe.Handler) {},
			body:     map[string]any{"TranscriptionJobName": "missing-job"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "missing_name",
			setup:    func(_ *testing.T, _ *transcribe.Handler) {},
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestTranscribeHandler(t)
			tt.setup(t, h)

			rec := doTranscribeRequest(t, h, "DeleteTranscriptionJob", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestDeleteAfterReset_Is404 verifies delete after reset returns 404.
func TestDeleteAfterReset_Is404(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	h := transcribe.NewHandler(b)

	_, err := b.StartTranscriptionJob(
		&transcribe.TranscriptionJob{
			JobName:      "reset-job",
			LanguageCode: "en-US",
			Media:        transcribe.Media{MediaFileURI: "s3://b/f"},
		},
	)
	require.NoError(t, err)

	h.Reset()

	rec := doTranscribeRequest(t, h, "DeleteTranscriptionJob", map[string]any{
		"TranscriptionJobName": "reset-job",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestListTranscriptionJobsPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		count         int
		wantFirstPage int
		maxResults    int32 // 0 means "omit MaxResults from the request"
		wantNextToken bool
	}{
		{
			name:          "single_page",
			count:         5,
			wantFirstPage: 5,
			wantNextToken: false,
		},
		{
			name:          "multi_page_default_page_size",
			count:         105, // exceeds transcribeDefaultPageSize=100
			wantFirstPage: 100,
			wantNextToken: true,
		},
		{
			// MaxResults must be honored: a caller-supplied value smaller than the
			// default page size shrinks the page, proving it is no longer ignored.
			name:          "honors_max_results_within_bounds",
			count:         15,
			maxResults:    10,
			wantFirstPage: 10,
			wantNextToken: true,
		},
		{
			// A MaxResults value at the exact remaining count yields no NextToken.
			name:          "honors_max_results_exact_remaining",
			count:         5,
			maxResults:    5,
			wantFirstPage: 5,
			wantNextToken: false,
		},
		{
			// AWS documents "Valid Range: Minimum value of 1. Maximum value of 100."
			// A caller-supplied value above the documented maximum is clamped to 100,
			// not honored verbatim and not rejected.
			name:          "clamps_max_results_above_documented_upper_bound",
			count:         105,
			maxResults:    500,
			wantFirstPage: 100,
			wantNextToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestTranscribeHandler(t)

			for i := range tt.count {
				_, err := h.Backend.StartTranscriptionJob(&transcribe.TranscriptionJob{
					JobName:      fmt.Sprintf("job-%04d", i),
					LanguageCode: "en-US",
					Media:        transcribe.Media{MediaFileURI: fmt.Sprintf("s3://bucket/file%d.mp4", i)},
				})
				require.NoError(t, err)
			}

			body := map[string]any{}
			if tt.maxResults != 0 {
				body["MaxResults"] = tt.maxResults
			}

			rec := doTranscribeRequest(t, h, "ListTranscriptionJobs", body)
			assert.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			summaries, summariesOK := resp["TranscriptionJobSummaries"].([]any)
			require.True(t, summariesOK)
			assert.Len(t, summaries, tt.wantFirstPage)

			if tt.wantNextToken {
				nextToken, tokenOK := resp["NextToken"].(string)
				require.True(t, tokenOK, "NextToken should be present")
				assert.NotEmpty(t, nextToken)

				// Second page using the token.
				page2Body := map[string]any{"NextToken": nextToken}
				if tt.maxResults != 0 {
					page2Body["MaxResults"] = tt.maxResults
				}

				rec2 := doTranscribeRequest(t, h, "ListTranscriptionJobs", page2Body)
				assert.Equal(t, http.StatusOK, rec2.Code)

				var resp2 map[string]any
				require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))

				summaries2, summaries2OK := resp2["TranscriptionJobSummaries"].([]any)
				require.True(t, summaries2OK)
				assert.Len(t, summaries2, tt.count-tt.wantFirstPage)
				assert.Empty(t, resp2["NextToken"])
			} else {
				assert.Empty(t, resp["NextToken"])
			}
		})
	}
}

// TestStatusConstant_Completed verifies named status constants are used in responses.
func TestStatusConstant_Completed(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	h := transcribe.NewHandler(b)

	rec := doTranscribeRequest(t, h, "StartTranscriptionJob", map[string]any{
		"TranscriptionJobName": "status-test",
		"LanguageCode":         "en-US",
	})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "COMPLETED")
}

// ── GetTranscriptionJob full field echo ───────────────────────────────────────

func TestGetTranscriptionJob_FullFieldEcho(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)

	// Start job with all fields.
	rec := doTranscribeRequest(t, h, "StartTranscriptionJob", map[string]any{
		"TranscriptionJobName": "full-echo-job",
		"LanguageCode":         "en-US",
		"MediaFormat":          "mp3",
		"MediaSampleRateHertz": 16000,
		"Media":                map[string]any{"MediaFileUri": "s3://bucket/audio.mp3"},
		"OutputBucketName":     "my-output",
		"OutputKey":            "prefix/job.json",
		"Settings": map[string]any{
			"ShowSpeakerLabels": true,
			"MaxSpeakerLabels":  3,
		},
		"ContentRedaction": map[string]any{"RedactionType": "PII", "RedactionOutput": "redacted"},
		"Subtitles":        map[string]any{"Formats": []string{"vtt"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Get should echo all fields.
	getRec := doTranscribeRequest(t, h, "GetTranscriptionJob", map[string]any{
		"TranscriptionJobName": "full-echo-job",
	})
	require.Equal(t, http.StatusOK, getRec.Code)

	body := getRec.Body.String()
	assert.Contains(t, body, "full-echo-job")
	assert.Contains(t, body, "COMPLETED")
	assert.Contains(t, body, "en-US")
	assert.Contains(t, body, "mp3")
	assert.Contains(t, body, "16000")
	assert.Contains(t, body, "my-output")
	assert.Contains(t, body, "ContentRedaction")
	assert.Contains(t, body, "PII")
	assert.Contains(t, body, "ShowSpeakerLabels")
	assert.Contains(t, body, "CreationTime")
	assert.Contains(t, body, "CompletionTime")
}

// ── GetTranscriptionJob Transcript URI routing ────────────────────────────────

func TestGetTranscriptionJob_TranscriptURIRouting(t *testing.T) {
	t.Parallel()

	t.Run("with_output_bucket", func(t *testing.T) {
		t.Parallel()

		h, _ := newHandlerWithBackend(t)
		doTranscribeRequest(t, h, "StartTranscriptionJob", map[string]any{
			"TranscriptionJobName": "bucket-job",
			"LanguageCode":         "en-US",
			"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
			"OutputBucketName":     "my-results",
			"OutputKey":            "my-job.json",
		})

		rec := doTranscribeRequest(t, h, "GetTranscriptionJob", map[string]any{
			"TranscriptionJobName": "bucket-job",
		})
		assert.Contains(t, rec.Body.String(), "s3://my-results/my-job.json")
	})

	t.Run("without_output_bucket_uses_synthetic", func(t *testing.T) {
		t.Parallel()

		h, _ := newHandlerWithBackend(t)
		doTranscribeRequest(t, h, "StartTranscriptionJob", map[string]any{
			"TranscriptionJobName": "synthetic-job",
			"LanguageCode":         "en-US",
			"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
		})

		rec := doTranscribeRequest(t, h, "GetTranscriptionJob", map[string]any{
			"TranscriptionJobName": "synthetic-job",
		})
		assert.Contains(t, rec.Body.String(), "s3://synthetic-transcripts/synthetic-job.json")
	})
}

// ── ListTranscriptionJobs includes timestamps ──────────────────────────────────

func TestListTranscriptionJobs_IncludesTimestamps(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	doTranscribeRequest(t, h, "StartTranscriptionJob", map[string]any{
		"TranscriptionJobName": "ts-job",
		"LanguageCode":         "en-US",
		"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
	})

	rec := doTranscribeRequest(t, h, "ListTranscriptionJobs", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "CreationTime")
}

// TestMediaFileUri_JSONKey verifies the JSON key is "MediaFileUri" not "MediaFileURI".
func TestMediaFileUri_JSONKey(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)

	rec := doTranscribeRequest(t, h, "StartTranscriptionJob", map[string]any{
		"TranscriptionJobName": "media-uri-job",
		"LanguageCode":         "en-US",
		"Media":                map[string]any{"MediaFileUri": "s3://my-bucket/audio.mp3"},
	})
	require.Equal(t, http.StatusOK, rec.Code, "start job: %s", rec.Body)

	descRec := doTranscribeRequest(t, h, "GetTranscriptionJob", map[string]any{
		"TranscriptionJobName": "media-uri-job",
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &raw))
	job, _ := raw["TranscriptionJob"].(map[string]any)
	require.NotNil(t, job, "TranscriptionJob must be present")

	media, _ := job["Media"].(map[string]any)
	require.NotNil(t, media, "Media must be present")

	_, hasURI := media["MediaFileUri"]
	_, hasWrong := media["MediaFileURI"]

	assert.True(t, hasURI, "Media.MediaFileUri must use lowercase 'ri' suffix")
	assert.False(t, hasWrong, "Media.MediaFileURI must NOT appear (wrong casing)")
}

// TestTranscriptionJob_MediaInResponse verifies Media is present in GetTranscriptionJob response.
func TestTranscriptionJob_MediaInResponse(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)

	rec := doTranscribeRequest(t, h, "StartTranscriptionJob", map[string]any{
		"TranscriptionJobName": "media-field-job",
		"LanguageCode":         "en-US",
		"Media":                map[string]any{"MediaFileUri": "s3://my-bucket/audio.mp3"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	descRec := doTranscribeRequest(t, h, "GetTranscriptionJob", map[string]any{
		"TranscriptionJobName": "media-field-job",
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &raw))
	job, _ := raw["TranscriptionJob"].(map[string]any)
	require.NotNil(t, job)

	media, hasMedia := job["Media"]
	assert.True(t, hasMedia, "Media field must be present in GetTranscriptionJob response")
	mediaMap, _ := media.(map[string]any)
	assert.NotEmpty(t, mediaMap["MediaFileUri"], "MediaFileUri must be non-empty")
}

// TestTranscriptionJob_OutputBucketNotInResponse verifies OutputBucketName/OutputKey
// (request-only fields per the real TranscriptionJob shape) are never echoed back at
// the top level of a GetTranscriptionJob/StartTranscriptionJob response -- the real
// AWS API only surfaces the output location via Transcript.TranscriptFileUri.
func TestTranscriptionJob_OutputBucketNotInResponse(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)

	rec := doTranscribeRequest(t, h, "StartTranscriptionJob", map[string]any{
		"TranscriptionJobName": "no-invented-fields-job",
		"LanguageCode":         "en-US",
		"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
		"OutputBucketName":     "should-not-leak-bucket",
		"OutputKey":            "should-not-leak-key",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	job, _ := raw["TranscriptionJob"].(map[string]any)
	require.NotNil(t, job)

	_, hasBucket := job["OutputBucketName"]
	_, hasKey := job["OutputKey"]
	assert.False(t, hasBucket, "OutputBucketName must not appear at top level; not a real TranscriptionJob field")
	assert.False(t, hasKey, "OutputKey must not appear at top level; not a real TranscriptionJob field")

	// The bucket must still be routed into the Transcript URI.
	transcript, _ := job["Transcript"].(map[string]any)
	require.NotNil(t, transcript)
	assert.Contains(t, transcript["TranscriptFileUri"], "should-not-leak-bucket")
}

// TestListTranscriptionJobs_JobNameContains verifies the JobNameContains filter
// (case-insensitive substring match, per the real ListTranscriptionJobsInput field).
func TestListTranscriptionJobs_JobNameContains(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	media := transcribe.Media{MediaFileURI: "s3://b/f"}
	for _, name := range []string{"alpha-report", "beta-report", "gamma-summary"} {
		_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
			JobName: name, LanguageCode: "en-US", Media: media,
		})
		require.NoError(t, err)
	}

	list, _ := b.ListTranscriptionJobs("", "report", "", 0)
	require.Len(t, list, 2)

	list, _ = b.ListTranscriptionJobs("", "REPORT", "", 0)
	require.Len(t, list, 2, "NameContains must be case-insensitive")

	list, _ = b.ListTranscriptionJobs("", "gamma", "", 0)
	require.Len(t, list, 1)
	assert.Equal(t, "gamma-summary", list[0].JobName)

	list, _ = b.ListTranscriptionJobs("", "nonexistent", "", 0)
	assert.Empty(t, list)
}

// TestTranscriptionJob_LanguageIdSettings_Echoed verifies LanguageIdSettings supplied
// on StartTranscriptionJob round-trips through GetTranscriptionJob, matching the real
// TranscriptionJob.LanguageIdSettings field.
func TestTranscriptionJob_LanguageIdSettings_Echoed(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)

	rec := doTranscribeRequest(t, h, "StartTranscriptionJob", map[string]any{
		"TranscriptionJobName": "lang-id-settings-job",
		"IdentifyLanguage":     true,
		"LanguageOptions":      []string{"en-US", "es-ES"},
		"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
		"LanguageIdSettings": map[string]any{
			"en-US": map[string]any{"VocabularyName": "my-vocab"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	job, _ := raw["TranscriptionJob"].(map[string]any)
	require.NotNil(t, job)

	settings, ok := job["LanguageIdSettings"].(map[string]any)
	require.True(t, ok, "LanguageIdSettings must be echoed back")
	enUS, ok := settings["en-US"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "my-vocab", enUS["VocabularyName"])
}
