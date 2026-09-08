package transcribe_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/transcribe"
)

func TestStartMedicalScribeJob_RequiredFields(t *testing.T) {
	t.Parallel()

	t.Run("valid_job_accepted", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		job, err := b.StartMedicalScribeJob(&transcribe.MedicalScribeJob{
			MedicalScribeJobName: "scribe-ok",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f.mp3"},
			DataAccessRoleArn:    "arn:aws:iam::123456789012:role/TranscribeRole",
			OutputBucketName:     "my-output-bucket",
			Settings:             &transcribe.MedicalScribeSettings{ShowSpeakerLabels: true, MaxSpeakerLabels: 2},
		})
		require.NoError(t, err)
		assert.Equal(t, "COMPLETED", job.MedicalScribeJobStatus)
	})

	t.Run("missing_data_access_role_arn_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartMedicalScribeJob(&transcribe.MedicalScribeJob{
			MedicalScribeJobName: "scribe-no-role",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			OutputBucketName:     "my-output-bucket",
			Settings:             &transcribe.MedicalScribeSettings{ShowSpeakerLabels: true, MaxSpeakerLabels: 2},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("missing_output_bucket_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartMedicalScribeJob(&transcribe.MedicalScribeJob{
			MedicalScribeJobName: "scribe-no-bucket",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			DataAccessRoleArn:    "arn:aws:iam::123456789012:role/TranscribeRole",
			Settings:             &transcribe.MedicalScribeSettings{ShowSpeakerLabels: true, MaxSpeakerLabels: 2},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("missing_settings_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartMedicalScribeJob(&transcribe.MedicalScribeJob{
			MedicalScribeJobName: "scribe-no-settings",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			DataAccessRoleArn:    "arn:aws:iam::123456789012:role/TranscribeRole",
			OutputBucketName:     "my-output-bucket",
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("both_speaker_labels_and_channel_identification_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartMedicalScribeJob(&transcribe.MedicalScribeJob{
			MedicalScribeJobName: "scribe-both-set",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			DataAccessRoleArn:    "arn:aws:iam::123456789012:role/TranscribeRole",
			OutputBucketName:     "my-output-bucket",
			Settings: &transcribe.MedicalScribeSettings{
				ShowSpeakerLabels:     true,
				MaxSpeakerLabels:      2,
				ChannelIdentification: true,
			},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("neither_speaker_labels_nor_channel_identification_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartMedicalScribeJob(&transcribe.MedicalScribeJob{
			MedicalScribeJobName: "scribe-neither-set",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			DataAccessRoleArn:    "arn:aws:iam::123456789012:role/TranscribeRole",
			OutputBucketName:     "my-output-bucket",
			Settings:             &transcribe.MedicalScribeSettings{},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("show_speaker_labels_without_max_speaker_labels_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartMedicalScribeJob(&transcribe.MedicalScribeJob{
			MedicalScribeJobName: "scribe-no-max-speakers",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			DataAccessRoleArn:    "arn:aws:iam::123456789012:role/TranscribeRole",
			OutputBucketName:     "my-output-bucket",
			Settings:             &transcribe.MedicalScribeSettings{ShowSpeakerLabels: true},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("channel_identification_without_channel_definitions_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartMedicalScribeJob(&transcribe.MedicalScribeJob{
			MedicalScribeJobName: "scribe-no-channel-defs",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			DataAccessRoleArn:    "arn:aws:iam::123456789012:role/TranscribeRole",
			OutputBucketName:     "my-output-bucket",
			Settings:             &transcribe.MedicalScribeSettings{ChannelIdentification: true},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("channel_identification_with_channel_definitions_accepted", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		job, err := b.StartMedicalScribeJob(&transcribe.MedicalScribeJob{
			MedicalScribeJobName: "scribe-channel-defs-ok",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			DataAccessRoleArn:    "arn:aws:iam::123456789012:role/TranscribeRole",
			OutputBucketName:     "my-output-bucket",
			Settings:             &transcribe.MedicalScribeSettings{ChannelIdentification: true},
			ChannelDefinitions: []transcribe.MedicalScribeChannelDefinition{
				{ChannelID: 0, ParticipantRole: "CLINICIAN"},
				{ChannelID: 1, ParticipantRole: "PATIENT"},
			},
		})
		require.NoError(t, err)
		assert.Len(t, job.ChannelDefinitions, 2)
	})
}

func TestDeleteMedicalScribeJob(t *testing.T) {
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
				b.AddMedicalScribeJobInternal(&transcribe.MedicalScribeJob{
					MedicalScribeJobName:   "ms-job-del",
					MedicalScribeJobStatus: "COMPLETED",
				})
			},
			body:     map[string]any{"MedicalScribeJobName": "ms-job-del"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			setup:    func(_ *testing.T, _ *transcribe.InMemoryBackend) {},
			body:     map[string]any{"MedicalScribeJobName": "no-such-ms-job"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := transcribe.NewInMemoryBackend()
			h := transcribe.NewHandler(b)
			tt.setup(t, b)

			rec := doTranscribeRequest(t, h, "DeleteMedicalScribeJob", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// ── GetMedicalScribeJob full field echo ────────────────────────────────────────

func TestGetMedicalScribeJob_FullFieldEcho(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	doTranscribeRequest(t, h, "StartMedicalScribeJob", map[string]any{
		"MedicalScribeJobName": "scribe-echo-job",
		"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
		"DataAccessRoleArn":    "arn:aws:iam::123456789012:role/ScribeRole",
		"OutputBucketName":     "scribe-output",
		"Settings":             map[string]any{"ShowSpeakerLabels": true, "MaxSpeakerLabels": 2},
	})

	rec := doTranscribeRequest(t, h, "GetMedicalScribeJob", map[string]any{
		"MedicalScribeJobName": "scribe-echo-job",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "scribe-echo-job")
	assert.Contains(t, body, "COMPLETED")
	assert.Contains(t, body, "DataAccessRoleArn")
	assert.Contains(t, body, "scribe-output")
	assert.Contains(t, body, "CreationTime")
}

// ── MedicalScribeJob Tags and ClinicalNoteGenerationSettings ─────────────────

func TestStartMedicalScribeJob_TagsAndClinicalNotes(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	rec := doTranscribeRequest(t, h, "StartMedicalScribeJob", map[string]any{
		"MedicalScribeJobName": "scribe-clinical-job",
		"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
		"DataAccessRoleArn":    "arn:aws:iam::123456789012:role/ScribeRole",
		"OutputBucketName":     "scribe-output",
		"Tags":                 []map[string]string{{"Key": "department", "Value": "cardiology"}},
		"Settings": map[string]any{
			"ClinicalNoteGenerationSettings": map[string]any{"NoteTemplate": "SOAP"},
			"ShowSpeakerLabels":              true,
			"MaxSpeakerLabels":               2,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "scribe-clinical-job")
	assert.Contains(t, body, `"NoteTemplate":"SOAP"`)
}

func TestHTTP_ListMedicalScribeJobs(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	rec := doTranscribeRequest(t, h, "StartMedicalScribeJob", map[string]any{
		"MedicalScribeJobName": "scribe-list-job",
		"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
		"DataAccessRoleArn":    "arn:aws:iam::123456789012:role/ScribeRole",
		"OutputBucketName":     "scribe-output",
		"Settings":             map[string]any{"ShowSpeakerLabels": true, "MaxSpeakerLabels": 2},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	listRec := doTranscribeRequest(t, h, "ListMedicalScribeJobs", map[string]any{})
	require.Equal(t, http.StatusOK, listRec.Code)
	assert.Contains(t, listRec.Body.String(), "scribe-list-job")
}

// TestListMedicalScribeJobs_JobNameContainsAndSummaryShape verifies the
// JobNameContains filter and that summaries are trimmed to the real
// MedicalScribeJobSummary fields (no Settings/Media/Tags/ChannelDefinitions, which
// only belong on the full MedicalScribeJob shape).
func TestListMedicalScribeJobs_JobNameContainsAndSummaryShape(t *testing.T) {
	t.Parallel()

	h, b := newHandlerWithBackend(t)

	for _, name := range []string{"cardiology-visit-1", "cardiology-visit-2", "dermatology-visit"} {
		rec := doTranscribeRequest(t, h, "StartMedicalScribeJob", map[string]any{
			"MedicalScribeJobName": name,
			"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
			"DataAccessRoleArn":    "arn:aws:iam::123456789012:role/ScribeRole",
			"OutputBucketName":     "scribe-summary-output",
			"Settings": map[string]any{
				"VocabularyName":    "med-vocab",
				"ShowSpeakerLabels": true,
				"MaxSpeakerLabels":  2,
			},
		})
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	}

	list, _ := b.ListMedicalScribeJobs("", "cardiology", "", 0)
	require.Len(t, list, 2)

	listRec := doTranscribeRequest(t, h, "ListMedicalScribeJobs", map[string]any{
		"JobNameContains": "dermatology",
	})
	require.Equal(t, http.StatusOK, listRec.Code)

	var raw struct {
		MedicalScribeJobSummaries []map[string]json.RawMessage `json:"MedicalScribeJobSummaries"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &raw))
	require.Len(t, raw.MedicalScribeJobSummaries, 1)

	summary := raw.MedicalScribeJobSummaries[0]
	assert.Contains(t, summary, "MedicalScribeJobName")
	assert.Contains(t, summary, "MedicalScribeJobStatus")
	assert.NotContains(t, summary, "Settings", "MedicalScribeJobSummary must not include Settings")
	assert.NotContains(t, summary, "Media", "MedicalScribeJobSummary must not include Media")
	assert.NotContains(t, summary, "Tags", "MedicalScribeJobSummary must not include Tags")
}

// TestMedicalScribeJob_OutputURIsPresentWhenCompleted verifies GetMedicalScribeJob
// returns MedicalScribeOutput (ClinicalDocumentUri/TranscriptFileUri) for a
// COMPLETED job, a real field gopherstack previously omitted entirely.
func TestMedicalScribeJob_OutputURIsPresentWhenCompleted(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	startRec := doTranscribeRequest(t, h, "StartMedicalScribeJob", map[string]any{
		"MedicalScribeJobName": "scribe-output-job",
		"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
		"DataAccessRoleArn":    "arn:aws:iam::123456789012:role/ScribeRole",
		"OutputBucketName":     "scribe-output-bucket",
		"Settings":             map[string]any{"ShowSpeakerLabels": true, "MaxSpeakerLabels": 2},
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	getRec := doTranscribeRequest(t, h, "GetMedicalScribeJob", map[string]any{
		"MedicalScribeJobName": "scribe-output-job",
	})
	require.Equal(t, http.StatusOK, getRec.Code)

	var raw struct {
		MedicalScribeJob struct {
			MedicalScribeOutput struct {
				ClinicalDocumentURI string `json:"ClinicalDocumentUri"`
				TranscriptFileURI   string `json:"TranscriptFileUri"`
			} `json:"MedicalScribeOutput"`
		} `json:"MedicalScribeJob"`
	}
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &raw))
	assert.Contains(t, raw.MedicalScribeJob.MedicalScribeOutput.ClinicalDocumentURI, "scribe-output-bucket")
	assert.Contains(t, raw.MedicalScribeJob.MedicalScribeOutput.TranscriptFileURI, "scribe-output-bucket")
}
