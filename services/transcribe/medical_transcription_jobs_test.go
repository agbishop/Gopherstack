package transcribe_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/transcribe"
)

func TestStartMedicalTranscriptionJob_SpecialtyType(t *testing.T) {
	t.Parallel()

	t.Run("valid_job_accepted", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		job, err := b.StartMedicalTranscriptionJob(&transcribe.MedicalTranscriptionJob{
			MedicalTranscriptionJobName: "med-job-ok",
			LanguageCode:                "en-US",
			Media:                       transcribe.Media{MediaFileURI: "s3://b/f.mp3"},
			Specialty:                   "PRIMARYCARE",
			Type:                        "CONVERSATION",
		})
		require.NoError(t, err)
		assert.Equal(t, "COMPLETED", job.TranscriptionJobStatus)
		assert.Equal(t, "PRIMARYCARE", job.Specialty)
		assert.Equal(t, "CONVERSATION", job.Type)
	})

	t.Run("missing_specialty_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartMedicalTranscriptionJob(&transcribe.MedicalTranscriptionJob{
			MedicalTranscriptionJobName: "med-job-no-specialty",
			LanguageCode:                "en-US",
			Media:                       transcribe.Media{MediaFileURI: "s3://b/f"},
			Type:                        "DICTATION",
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("invalid_type_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartMedicalTranscriptionJob(&transcribe.MedicalTranscriptionJob{
			MedicalTranscriptionJobName: "med-job-bad-type",
			LanguageCode:                "en-US",
			Media:                       transcribe.Media{MediaFileURI: "s3://b/f"},
			Specialty:                   "PRIMARYCARE",
			Type:                        "PODCAST",
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("non_en_us_language_code_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartMedicalTranscriptionJob(&transcribe.MedicalTranscriptionJob{
			MedicalTranscriptionJobName: "med-job-bad-lang",
			LanguageCode:                "fr-FR",
			Media:                       transcribe.Media{MediaFileURI: "s3://b/f"},
			Specialty:                   "PRIMARYCARE",
			Type:                        "CONVERSATION",
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("phi_content_identification_accepted", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		job, err := b.StartMedicalTranscriptionJob(&transcribe.MedicalTranscriptionJob{
			MedicalTranscriptionJobName:      "med-job-phi",
			LanguageCode:                     "en-US",
			Media:                            transcribe.Media{MediaFileURI: "s3://b/f"},
			Specialty:                        "PRIMARYCARE",
			Type:                             "CONVERSATION",
			MedicalContentIdentificationType: "PHI",
		})
		require.NoError(t, err)
		assert.Equal(t, "PHI", job.MedicalContentIdentificationType)
	})
}

func TestHTTP_StartMedicalTranscriptionJob(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	rec := doTranscribeRequest(t, h, "StartMedicalTranscriptionJob", map[string]any{
		"MedicalTranscriptionJobName": "http-med-job",
		"LanguageCode":                "en-US",
		"Media":                       map[string]any{"MediaFileUri": "s3://b/f.mp3"},
		"Specialty":                   "PRIMARYCARE",
		"Type":                        "DICTATION",
		"OutputBucketName":            "med-output",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "http-med-job")
}

func TestHTTP_StartMedicalTranscriptionJob_ContentIdentificationType(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	rec := doTranscribeRequest(t, h, "StartMedicalTranscriptionJob", map[string]any{
		"MedicalTranscriptionJobName": "http-med-job-cit",
		"LanguageCode":                "en-US",
		"Media":                       map[string]any{"MediaFileUri": "s3://b/f.mp3"},
		"Specialty":                   "PRIMARYCARE",
		"Type":                        "CONVERSATION",
		"ContentIdentificationType":   "PHI",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	job := out["MedicalTranscriptionJob"].(map[string]any)
	assert.Equal(t, "PHI", job["ContentIdentificationType"])
}

func TestDeleteMedicalTranscriptionJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*testing.T, *transcribe.InMemoryBackend)
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "success",
			setup: func(_ *testing.T, b *transcribe.InMemoryBackend) {
				b.AddMedicalTranscriptionJobInternal(&transcribe.MedicalTranscriptionJob{
					MedicalTranscriptionJobName: "mt-job-del",
					TranscriptionJobStatus:      "COMPLETED",
				})
			},
			body:     map[string]any{"MedicalTranscriptionJobName": "mt-job-del"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			setup:    func(_ *testing.T, _ *transcribe.InMemoryBackend) {},
			body:     map[string]any{"MedicalTranscriptionJobName": "no-such-mt-job"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := transcribe.NewInMemoryBackend()
			h := transcribe.NewHandler(b)
			tt.setup(t, b)

			rec := doTranscribeRequest(t, h, "DeleteMedicalTranscriptionJob", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// ── GetMedicalTranscriptionJob full field echo ─────────────────────────────────

func TestGetMedicalTranscriptionJob_FullFieldEcho(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	doTranscribeRequest(t, h, "StartMedicalTranscriptionJob", map[string]any{
		"MedicalTranscriptionJobName": "med-echo-job",
		"LanguageCode":                "en-US",
		"Media":                       map[string]any{"MediaFileUri": "s3://b/f"},
		"Specialty":                   "PRIMARYCARE",
		"Type":                        "DICTATION",
		"OutputBucketName":            "med-output",
	})

	rec := doTranscribeRequest(t, h, "GetMedicalTranscriptionJob", map[string]any{
		"MedicalTranscriptionJobName": "med-echo-job",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "med-echo-job")
	assert.Contains(t, body, "COMPLETED")
	assert.Contains(t, body, "PRIMARYCARE")
	assert.Contains(t, body, "DICTATION")
	assert.Contains(t, body, "med-output")
	assert.Contains(t, body, "CreationTime")
}

// TestMedicalTranscriptionJobTranscriptURI verifies that
// GetMedicalTranscriptionJob and StartMedicalTranscriptionJob return a
// populated Transcript.TranscriptFileURI, matching real AWS behaviour.
// The previous implementation omitted the Transcript field entirely, causing
// clients that read the output location to get an empty string or nil pointer.
func TestMedicalTranscriptionJobTranscriptURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		outputBucket  string
		outputKey     string
		wantURIPrefix string
		wantURISuffix string
	}{
		{
			name:          "custom_bucket_no_key_uses_job_name",
			outputBucket:  "my-transcripts",
			outputKey:     "",
			wantURIPrefix: "s3://my-transcripts/",
			wantURISuffix: ".json",
		},
		{
			name:          "custom_bucket_with_key",
			outputBucket:  "my-transcripts",
			outputKey:     "custom/path/output.json",
			wantURIPrefix: "s3://my-transcripts/custom/path/output.json",
			wantURISuffix: "",
		},
		{
			name:          "no_bucket_uses_synthetic",
			outputBucket:  "",
			outputKey:     "",
			wantURIPrefix: "s3://synthetic-transcripts/",
			wantURISuffix: ".json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestTranscribeHandler(t)
			jobName := "med-job-" + tt.name

			body := map[string]any{
				"MedicalTranscriptionJobName": jobName,
				"LanguageCode":                "en-US",
				"MediaFormat":                 "mp3",
				"Specialty":                   "PRIMARYCARE",
				"Type":                        "DICTATION",
				"Media":                       map[string]any{"MediaFileUri": "s3://input-bucket/audio.mp3"},
			}
			if tt.outputBucket != "" {
				body["OutputBucketName"] = tt.outputBucket
			}
			if tt.outputKey != "" {
				body["OutputKey"] = tt.outputKey
			}

			startRec := doTranscribeRequest(t, h, "StartMedicalTranscriptionJob", body)
			require.Equal(t, http.StatusOK, startRec.Code)

			getRec := doTranscribeRequest(t, h, "GetMedicalTranscriptionJob", map[string]any{
				"MedicalTranscriptionJobName": jobName,
			})
			require.Equal(t, http.StatusOK, getRec.Code)

			var out struct {
				MedicalTranscriptionJob struct {
					Transcript struct {
						TranscriptFileURI string `json:"TranscriptFileUri"`
					} `json:"Transcript"`
				} `json:"MedicalTranscriptionJob"`
			}
			require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &out))

			uri := out.MedicalTranscriptionJob.Transcript.TranscriptFileURI
			assert.NotEmpty(t, uri, "Transcript.TranscriptFileURI must not be empty")
			assert.Contains(t, uri, tt.wantURIPrefix,
				"TranscriptFileUri should contain expected prefix")
			if tt.wantURISuffix != "" {
				assert.Contains(t, uri, tt.wantURISuffix,
					"TranscriptFileUri should contain expected suffix")
			}
		})
	}
}

// TestStartMedicalTranscriptionJob_TranscriptURIInStartResponse verifies
// that StartMedicalTranscriptionJob also returns Transcript in the response body,
// not just GetMedicalTranscriptionJob.
func TestStartMedicalTranscriptionJob_TranscriptURIInStartResponse(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)

	startRec := doTranscribeRequest(t, h, "StartMedicalTranscriptionJob", map[string]any{
		"MedicalTranscriptionJobName": "med-start-resp-job",
		"LanguageCode":                "en-US",
		"MediaFormat":                 "mp3",
		"Specialty":                   "PRIMARYCARE",
		"Type":                        "DICTATION",
		"OutputBucketName":            "output-bucket",
		"Media":                       map[string]any{"MediaFileUri": "s3://input/audio.mp3"},
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	var out struct {
		MedicalTranscriptionJob struct {
			Transcript struct {
				TranscriptFileURI string `json:"TranscriptFileUri"`
			} `json:"Transcript"`
		} `json:"MedicalTranscriptionJob"`
	}
	require.NoError(t, json.Unmarshal(startRec.Body.Bytes(), &out))

	uri := out.MedicalTranscriptionJob.Transcript.TranscriptFileURI
	assert.NotEmpty(t, uri, "StartMedicalTranscriptionJob response must include TranscriptFileUri")
	assert.True(t, strings.HasPrefix(uri, "s3://output-bucket/"), "URI should start with s3://output-bucket/")
}

func TestHTTP_ListMedicalTranscriptionJobs(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)

	startRec := doTranscribeRequest(t, h, "StartMedicalTranscriptionJob", map[string]any{
		"MedicalTranscriptionJobName": "med-list-job",
		"LanguageCode":                "en-US",
		"MediaFormat":                 "mp3",
		"Specialty":                   "PRIMARYCARE",
		"Type":                        "DICTATION",
		"Media":                       map[string]any{"MediaFileUri": "s3://input/audio.mp3"},
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	listRec := doTranscribeRequest(t, h, "ListMedicalTranscriptionJobs", map[string]any{})
	require.Equal(t, http.StatusOK, listRec.Code)
	assert.Contains(t, listRec.Body.String(), "med-list-job")
}

// TestListMedicalTranscriptionJobs_JobNameContainsAndSummaryShape verifies the
// JobNameContains filter and that summaries are trimmed to the real
// MedicalTranscriptionJobSummary fields (no Settings/Media/Tags, which only belong
// on the full MedicalTranscriptionJob shape), plus the OutputLocationType field.
func TestListMedicalTranscriptionJobs_JobNameContainsAndSummaryShape(t *testing.T) {
	t.Parallel()

	h, b := newHandlerWithBackend(t)

	for _, name := range []string{"intake-visit-1", "intake-visit-2", "followup-visit"} {
		rec := doTranscribeRequest(t, h, "StartMedicalTranscriptionJob", map[string]any{
			"MedicalTranscriptionJobName": name,
			"LanguageCode":                "en-US",
			"Specialty":                   "PRIMARYCARE",
			"Type":                        "DICTATION",
			"Media":                       map[string]any{"MediaFileUri": "s3://input/audio.mp3"},
			"OutputBucketName":            "custom-bucket",
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	list, _ := b.ListMedicalTranscriptionJobs("", "intake", "", 0)
	require.Len(t, list, 2)

	listRec := doTranscribeRequest(t, h, "ListMedicalTranscriptionJobs", map[string]any{
		"JobNameContains": "followup",
	})
	require.Equal(t, http.StatusOK, listRec.Code)

	var raw struct {
		MedicalTranscriptionJobSummaries []map[string]json.RawMessage `json:"MedicalTranscriptionJobSummaries"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &raw))
	require.Len(t, raw.MedicalTranscriptionJobSummaries, 1)

	summary := raw.MedicalTranscriptionJobSummaries[0]
	assert.NotContains(t, summary, "Settings", "MedicalTranscriptionJobSummary must not include Settings")
	assert.NotContains(t, summary, "Media", "MedicalTranscriptionJobSummary must not include Media")
	assert.NotContains(t, summary, "Tags", "MedicalTranscriptionJobSummary must not include Tags")

	var outputLocationType string
	require.NoError(t, json.Unmarshal(summary["OutputLocationType"], &outputLocationType))
	assert.Equal(t, "CUSTOMER_BUCKET", outputLocationType)
}
