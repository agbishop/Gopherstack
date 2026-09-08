package transcribe_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/transcribe"
)

func TestInMemoryBackend_SnapshotRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(b *transcribe.InMemoryBackend) string
		verify func(t *testing.T, b *transcribe.InMemoryBackend, id string)
		name   string
	}{
		{
			name: "round_trip_preserves_state",
			setup: func(b *transcribe.InMemoryBackend) string {
				job, err := b.StartTranscriptionJob(
					&transcribe.TranscriptionJob{
						JobName:      "test-job",
						LanguageCode: "en-US",
						Media:        transcribe.Media{MediaFileURI: "s3://my-bucket/audio.mp3"},
					},
				)
				if err != nil {
					return ""
				}

				return job.JobName
			},
			verify: func(t *testing.T, b *transcribe.InMemoryBackend, id string) {
				t.Helper()

				job, err := b.GetTranscriptionJob(id)
				require.NoError(t, err)
				assert.Equal(t, id, job.JobName)
				assert.Equal(t, "en-US", job.LanguageCode)
			},
		},
		{
			name:  "empty_backend_round_trip",
			setup: func(_ *transcribe.InMemoryBackend) string { return "" },
			verify: func(t *testing.T, b *transcribe.InMemoryBackend, _ string) {
				t.Helper()

				jobs, _ := b.ListTranscriptionJobs("", "", "", 0)
				assert.Empty(t, jobs)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := transcribe.NewInMemoryBackend()
			id := tt.setup(original)

			snap := original.Snapshot(t.Context())
			require.NotNil(t, snap)

			fresh := transcribe.NewInMemoryBackend()
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tt.verify(t, fresh, id)
		})
	}
}

// TestInMemoryBackend_SnapshotRestore_AllTableCounts verifies Snapshot/Restore
// round-trips all 9 store.Table-backed resources by count, complementing
// TestInMemoryBackend_SnapshotRestore_FullState's field-level assertions below.
func TestInMemoryBackend_SnapshotRestore_AllTableCounts(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	seedOneOfEachResource(t, b)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := transcribe.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), snap))

	assertAllResourceCounts(t, b2, 1)
}

func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// resourceTagARN is the synthetic ARN used to exercise the raw (non-store.Table)
// resourceTags map in the full-state round-trip test below.
const resourceTagARN = "arn:aws:transcribe:us-east-1:123456789012:transcription-job/full-job"

// newFullPersistenceTestBackend creates a backend with one populated entry in
// every store.Table (jobs, callAnalyticsCategories, languageModels,
// medicalVocabularies, vocabularies, vocabularyFilters, callAnalyticsJobs,
// medicalScribeJobs, medicalTranscriptionJobs -- see
// services/transcribe/store_setup.go) plus the raw resourceTags field left
// un-converted, so a Snapshot from it exercises the entire persisted surface
// of the backend.
func newFullPersistenceTestBackend(t *testing.T) *transcribe.InMemoryBackend {
	t.Helper()

	b := transcribe.NewInMemoryBackend()

	_, err := b.StartTranscriptionJob(&transcribe.TranscriptionJob{
		JobName:      "full-job",
		LanguageCode: "en-US",
		Media:        transcribe.Media{MediaFileURI: "s3://my-bucket/audio.mp3"},
	})
	require.NoError(t, err)

	_, err = b.CreateCallAnalyticsCategory(&transcribe.CallAnalyticsCategory{
		CategoryName: "full-category",
		InputType:    "POST_CALL",
		Rules: []transcribe.CallAnalyticsRule{
			{NonTalkTimeFilter: &transcribe.NonTalkTimeFilter{Threshold: 30000}},
		},
	})
	require.NoError(t, err)

	_, err = b.CreateLanguageModel(&transcribe.LanguageModel{
		ModelName:     "full-model",
		BaseModelName: "WideBand",
		LanguageCode:  "en-US",
		InputDataConfig: &transcribe.InputDataConfig{
			S3Uri:             "s3://bucket/training/",
			DataAccessRoleArn: "arn:aws:iam::123456789012:role/TranscribeRole",
		},
	})
	require.NoError(t, err)

	_, err = b.CreateMedicalVocabulary("full-medical-vocab", "en-US", "s3://my-bucket/vocab.txt", nil)
	require.NoError(t, err)

	_, err = b.CreateVocabulary(&transcribe.Vocabulary{
		VocabularyName: "full-vocab",
		LanguageCode:   "en-US",
		Phrases:        []string{"hello", "world"},
	})
	require.NoError(t, err)

	_, err = b.CreateVocabularyFilter(&transcribe.VocabularyFilter{
		VocabularyFilterName: "full-filter",
		LanguageCode:         "en-US",
		Words:                []string{"badword"},
	})
	require.NoError(t, err)

	_, err = b.StartCallAnalyticsJob(&transcribe.CallAnalyticsJob{
		CallAnalyticsJobName: "full-ca-job",
		LanguageCode:         "en-US",
	})
	require.NoError(t, err)

	_, err = b.StartMedicalScribeJob(&transcribe.MedicalScribeJob{
		MedicalScribeJobName: "full-scribe-job",
		DataAccessRoleArn:    "arn:aws:iam::123456789012:role/transcribe-role",
		OutputBucketName:     "my-output-bucket",
		Settings:             &transcribe.MedicalScribeSettings{ShowSpeakerLabels: true, MaxSpeakerLabels: 2},
	})
	require.NoError(t, err)

	_, err = b.StartMedicalTranscriptionJob(&transcribe.MedicalTranscriptionJob{
		MedicalTranscriptionJobName: "full-medical-job",
		LanguageCode:                "en-US",
		Specialty:                   "PRIMARYCARE",
		Type:                        "CONVERSATION",
	})
	require.NoError(t, err)

	require.NoError(t, b.TagResource(resourceTagARN, map[string]string{"env": "test"}))

	return b
}

// TestInMemoryBackend_SnapshotRestore_FullState round-trips every store.Table
// (jobs, callAnalyticsCategories, languageModels, medicalVocabularies,
// vocabularies, vocabularyFilters, callAnalyticsJobs, medicalScribeJobs,
// medicalTranscriptionJobs) and the raw resourceTags field through
// Snapshot -> Restore into a fresh backend.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := newFullPersistenceTestBackend(t)

	snap := original.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	fresh := transcribe.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	// jobs table.
	job, err := fresh.GetTranscriptionJob("full-job")
	require.NoError(t, err)
	assert.Equal(t, "en-US", job.LanguageCode)

	// callAnalyticsCategories table.
	cat, err := fresh.GetCallAnalyticsCategory("full-category")
	require.NoError(t, err)
	assert.Equal(t, "POST_CALL", cat.InputType)

	// languageModels table.
	model, err := fresh.DescribeLanguageModel("full-model")
	require.NoError(t, err)
	assert.Equal(t, "WideBand", model.BaseModelName)

	// medicalVocabularies table.
	medVocab, err := fresh.GetMedicalVocabulary("full-medical-vocab")
	require.NoError(t, err)
	assert.Equal(t, "s3://my-bucket/vocab.txt", medVocab.VocabularyFileURI)

	// vocabularies table.
	vocab, err := fresh.GetVocabulary("full-vocab")
	require.NoError(t, err)
	assert.Equal(t, []string{"hello", "world"}, vocab.Phrases)

	// vocabularyFilters table.
	filter, err := fresh.GetVocabularyFilter("full-filter")
	require.NoError(t, err)
	assert.Equal(t, []string{"badword"}, filter.Words)

	// callAnalyticsJobs table.
	caJob, err := fresh.GetCallAnalyticsJob("full-ca-job")
	require.NoError(t, err)
	assert.Equal(t, "en-US", caJob.LanguageCode)

	// medicalScribeJobs table.
	scribeJob, err := fresh.GetMedicalScribeJob("full-scribe-job")
	require.NoError(t, err)
	assert.Equal(t, "my-output-bucket", scribeJob.OutputBucketName)

	// medicalTranscriptionJobs table.
	medJob, err := fresh.GetMedicalTranscriptionJob("full-medical-job")
	require.NoError(t, err)
	assert.Equal(t, "PRIMARYCARE", medJob.Specialty)

	// resourceTags (raw field).
	tags, err := fresh.ListTagsForResource(resourceTagARN)
	require.NoError(t, err)
	assert.Equal(t, "test", tags["env"])
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend (including a syntactically valid
// but future/unknown version) is discarded cleanly rather than partially
// decoded: the backend resets to empty state and Restore returns no error.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b := newFullPersistenceTestBackend(t)

	err := b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	jobs, _ := b.ListTranscriptionJobs("", "", "", 0)
	assert.Empty(t, jobs)

	cats, _ := b.ListCallAnalyticsCategories("", 0)
	assert.Empty(t, cats)

	models, _ := b.ListLanguageModels("", "", "", 0)
	assert.Empty(t, models)

	medVocabs, _ := b.ListMedicalVocabularies("", "", "", 0)
	assert.Empty(t, medVocabs)

	vocabs, _ := b.ListVocabularies("", "", "", 0)
	assert.Empty(t, vocabs)

	filters, _ := b.ListVocabularyFilters("", "", 0)
	assert.Empty(t, filters)

	caJobs, _ := b.ListCallAnalyticsJobs("", "", "", 0)
	assert.Empty(t, caJobs)

	scribeJobs, _ := b.ListMedicalScribeJobs("", "", "", 0)
	assert.Empty(t, scribeJobs)

	medJobs, _ := b.ListMedicalTranscriptionJobs("", "", "", 0)
	assert.Empty(t, medJobs)

	tags, err := b.ListTagsForResource(resourceTagARN)
	require.NoError(t, err)
	assert.Empty(t, tags)
}

// TestInMemoryBackend_RestoreOldFormatDecodesAsVersionZero verifies that a
// pre-Phase-3.3 snapshot (no "version"/"tables" fields at all -- the old flat
// map-of-maps shape) decodes with Version == 0, which mismatches
// transcribeSnapshotVersion and is discarded the same way any other
// incompatible version is -- never partially decoded under the old shape.
func TestInMemoryBackend_RestoreOldFormatDecodesAsVersionZero(t *testing.T) {
	t.Parallel()

	oldFormatSnap := `{` +
		`"jobs":{"legacy":{"jobName":"legacy","languageCode":"en-US"}},` +
		`"callAnalyticsCategories":{},` +
		`"languageModels":{},` +
		`"medicalVocabularies":{},` +
		`"vocabularies":{},` +
		`"vocabularyFilters":{},` +
		`"callAnalyticsJobs":{},` +
		`"medicalScribeJobs":{},` +
		`"medicalTranscriptionJobs":{}}`

	b := transcribe.NewInMemoryBackend()
	err := b.Restore(t.Context(), []byte(oldFormatSnap))
	require.NoError(t, err)

	jobs, _ := b.ListTranscriptionJobs("", "", "", 0)
	assert.Empty(t, jobs, "old-format snapshot must not be partially decoded")
}

// TestHandler_SnapshotRestoreDelegate verifies Handler.Snapshot/Restore
// delegate to the backend (the wiring cli.go's generic setupPersistence
// relies on).
func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	backend := newFullPersistenceTestBackend(t)
	h := transcribe.NewHandler(backend)

	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	freshBackend := transcribe.NewInMemoryBackend()
	freshHandler := transcribe.NewHandler(freshBackend)
	require.NoError(t, freshHandler.Restore(t.Context(), snap))

	job, err := freshBackend.GetTranscriptionJob("full-job")
	require.NoError(t, err)
	assert.Equal(t, "en-US", job.LanguageCode)
}
