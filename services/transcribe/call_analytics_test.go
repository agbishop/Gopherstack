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

func TestStartCallAnalyticsJob_ChannelDefinitions(t *testing.T) {
	t.Parallel()

	t.Run("valid_two_channels_accepted", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		job, err := b.StartCallAnalyticsJob(&transcribe.CallAnalyticsJob{
			CallAnalyticsJobName: "ca-job-ok",
			LanguageCode:         "en-US",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			ChannelDefinitions: []transcribe.ChannelDefinition{
				{ChannelID: 0, ParticipantRole: "AGENT"},
				{ChannelID: 1, ParticipantRole: "CUSTOMER"},
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "COMPLETED", job.CallAnalyticsJobStatus)
		assert.Len(t, job.ChannelDefinitions, 2)
	})

	t.Run("only_one_channel_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartCallAnalyticsJob(&transcribe.CallAnalyticsJob{
			CallAnalyticsJobName: "ca-job-one-channel",
			LanguageCode:         "en-US",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			ChannelDefinitions: []transcribe.ChannelDefinition{
				{ChannelID: 0, ParticipantRole: "AGENT"},
			},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("duplicate_roles_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.StartCallAnalyticsJob(&transcribe.CallAnalyticsJob{
			CallAnalyticsJobName: "ca-job-dup-roles",
			LanguageCode:         "en-US",
			Media:                transcribe.Media{MediaFileURI: "s3://b/f"},
			ChannelDefinitions: []transcribe.ChannelDefinition{
				{ChannelID: 0, ParticipantRole: "AGENT"},
				{ChannelID: 1, ParticipantRole: "AGENT"},
			},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})
}

func TestCreateCallAnalyticsCategory_Rules(t *testing.T) {
	t.Parallel()

	t.Run("valid_category_with_rules_accepted", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		cat, err := b.CreateCallAnalyticsCategory(&transcribe.CallAnalyticsCategory{
			CategoryName: "my-category",
			InputType:    "POST_CALL",
			Rules: []transcribe.CallAnalyticsRule{
				{
					NonTalkTimeFilter: &transcribe.NonTalkTimeFilter{
						Threshold:       30000,
						ParticipantRole: "CUSTOMER",
					},
				},
			},
		})
		require.NoError(t, err)
		assert.Len(t, cat.Rules, 1)
		assert.Equal(t, "POST_CALL", cat.InputType)
	})

	t.Run("invalid_input_type_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.CreateCallAnalyticsCategory(&transcribe.CallAnalyticsCategory{
			CategoryName: "bad-type",
			InputType:    "INVALID_TYPE",
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("update_category_with_new_rules", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.CreateCallAnalyticsCategory(&transcribe.CallAnalyticsCategory{
			CategoryName: "my-cat-update",
			InputType:    "POST_CALL",
			Rules: []transcribe.CallAnalyticsRule{
				{NonTalkTimeFilter: &transcribe.NonTalkTimeFilter{Threshold: 30000}},
			},
		})
		require.NoError(t, err)

		updated, err := b.UpdateCallAnalyticsCategory(&transcribe.CallAnalyticsCategory{
			CategoryName: "my-cat-update",
			InputType:    "REAL_TIME",
			Rules: []transcribe.CallAnalyticsRule{
				{
					TranscriptFilter: &transcribe.TranscriptFilter{
						TranscriptFilterType: "EXACT",
						Targets:              []string{"cancellation"},
					},
				},
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "REAL_TIME", updated.InputType)
		assert.Len(t, updated.Rules, 1)
	})

	t.Run("missing_rules_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.CreateCallAnalyticsCategory(&transcribe.CallAnalyticsCategory{
			CategoryName: "no-rules-cat",
			InputType:    "POST_CALL",
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("empty_rules_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.CreateCallAnalyticsCategory(&transcribe.CallAnalyticsCategory{
			CategoryName: "empty-rules-cat",
			InputType:    "POST_CALL",
			Rules:        []transcribe.CallAnalyticsRule{},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("too_many_rules_rejected", func(t *testing.T) {
		t.Parallel()

		rules := make([]transcribe.CallAnalyticsRule, 21)
		for i := range rules {
			rules[i] = transcribe.CallAnalyticsRule{NonTalkTimeFilter: &transcribe.NonTalkTimeFilter{Threshold: 1000}}
		}

		b := transcribe.NewInMemoryBackend()
		_, err := b.CreateCallAnalyticsCategory(&transcribe.CallAnalyticsCategory{
			CategoryName: "too-many-rules-cat",
			InputType:    "POST_CALL",
			Rules:        rules,
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("transcript_filter_missing_targets_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.CreateCallAnalyticsCategory(&transcribe.CallAnalyticsCategory{
			CategoryName: "tf-no-targets-cat",
			InputType:    "POST_CALL",
			Rules: []transcribe.CallAnalyticsRule{
				{TranscriptFilter: &transcribe.TranscriptFilter{TranscriptFilterType: "EXACT"}},
			},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("sentiment_filter_missing_sentiments_rejected", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.CreateCallAnalyticsCategory(&transcribe.CallAnalyticsCategory{
			CategoryName: "sf-no-sentiments-cat",
			InputType:    "POST_CALL",
			Rules: []transcribe.CallAnalyticsRule{
				{SentimentFilter: &transcribe.SentimentFilter{}},
			},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})
}

func TestHTTP_StartCallAnalyticsJob(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	rec := doTranscribeRequest(t, h, "StartCallAnalyticsJob", map[string]any{
		"CallAnalyticsJobName": "http-ca-job",
		"LanguageCode":         "en-US",
		"Media":                map[string]any{"MediaFileUri": "s3://b/f.mp3"},
		"ChannelDefinitions": []map[string]any{
			{"ChannelId": 0, "ParticipantRole": "AGENT"},
			{"ChannelId": 1, "ParticipantRole": "CUSTOMER"},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "http-ca-job")
}

func TestCreateCallAnalyticsCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*testing.T, *transcribe.InMemoryBackend)
		body     map[string]any
		name     string
		wantKey  string
		wantCode int
	}{
		{
			name:  "success",
			setup: func(_ *testing.T, _ *transcribe.InMemoryBackend) {},
			body: map[string]any{
				"CategoryName": "my-category",
				"InputType":    "POST_CALL",
				"Rules": []map[string]any{
					{"NonTalkTimeFilter": map[string]any{"Threshold": 30000}},
				},
			},
			wantCode: http.StatusOK,
			wantKey:  "my-category",
		},
		{
			name: "duplicate",
			setup: func(t *testing.T, b *transcribe.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateCallAnalyticsCategory(
					&transcribe.CallAnalyticsCategory{
						CategoryName: "dup-cat",
						InputType:    "POST_CALL",
						Rules: []transcribe.CallAnalyticsRule{
							{NonTalkTimeFilter: &transcribe.NonTalkTimeFilter{Threshold: 30000}},
						},
					},
				)
				require.NoError(t, err)
			},
			body: map[string]any{
				"CategoryName": "dup-cat",
				"Rules": []map[string]any{
					{"NonTalkTimeFilter": map[string]any{"Threshold": 30000}},
				},
			},
			wantCode: http.StatusConflict,
		},
		{
			name:     "missing_name",
			setup:    func(_ *testing.T, _ *transcribe.InMemoryBackend) {},
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := transcribe.NewInMemoryBackend()
			h := transcribe.NewHandler(b)
			tt.setup(t, b)

			rec := doTranscribeRequest(t, h, "CreateCallAnalyticsCategory", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantKey != "" {
				assert.Contains(t, rec.Body.String(), tt.wantKey)
			}
		})
	}
}

func TestDeleteCallAnalyticsCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*testing.T, *transcribe.InMemoryBackend)
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "success",
			setup: func(t *testing.T, b *transcribe.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateCallAnalyticsCategory(
					&transcribe.CallAnalyticsCategory{
						CategoryName: "cat-to-delete",
						InputType:    "POST_CALL",
						Rules: []transcribe.CallAnalyticsRule{
							{NonTalkTimeFilter: &transcribe.NonTalkTimeFilter{Threshold: 30000}},
						},
					},
				)
				require.NoError(t, err)
			},
			body:     map[string]any{"CategoryName": "cat-to-delete"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			setup:    func(_ *testing.T, _ *transcribe.InMemoryBackend) {},
			body:     map[string]any{"CategoryName": "missing-cat"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := transcribe.NewInMemoryBackend()
			h := transcribe.NewHandler(b)
			tt.setup(t, b)

			rec := doTranscribeRequest(t, h, "DeleteCallAnalyticsCategory", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestDeleteCallAnalyticsCategory_Idempotent verifies a second delete of the same
// category returns 404 rather than succeeding again.
func TestDeleteCallAnalyticsCategory_Idempotent(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	h := transcribe.NewHandler(b)

	b.AddCallAnalyticsCategoryInternal(&transcribe.CallAnalyticsCategory{CategoryName: "once-cat"})

	rec1 := doTranscribeRequest(t, h, "DeleteCallAnalyticsCategory", map[string]any{
		"CategoryName": "once-cat",
	})
	require.Equal(t, http.StatusOK, rec1.Code)

	rec2 := doTranscribeRequest(t, h, "DeleteCallAnalyticsCategory", map[string]any{
		"CategoryName": "once-cat",
	})
	assert.Equal(t, http.StatusNotFound, rec2.Code)
}

// TestCreateCallAnalyticsCategory_OutputShape verifies CategoryProperties in response.
func TestCreateCallAnalyticsCategory_OutputShape(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	h := transcribe.NewHandler(b)

	rec := doTranscribeRequest(t, h, "CreateCallAnalyticsCategory", map[string]any{
		"CategoryName": "shape-cat",
		"InputType":    "POST_CALL",
		"Rules": []map[string]any{
			{"NonTalkTimeFilter": map[string]any{"Threshold": 30000}},
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "CategoryProperties")
	assert.Contains(t, rec.Body.String(), "shape-cat")
	assert.Contains(t, rec.Body.String(), "POST_CALL")
}

func TestDeleteCallAnalyticsJob(t *testing.T) {
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
				b.AddCallAnalyticsJobInternal(&transcribe.CallAnalyticsJob{
					CallAnalyticsJobName:   "ca-job-del",
					CallAnalyticsJobStatus: "COMPLETED",
				})
			},
			body:     map[string]any{"CallAnalyticsJobName": "ca-job-del"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			setup:    func(_ *testing.T, _ *transcribe.InMemoryBackend) {},
			body:     map[string]any{"CallAnalyticsJobName": "no-such-ca-job"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := transcribe.NewInMemoryBackend()
			h := transcribe.NewHandler(b)
			tt.setup(t, b)

			rec := doTranscribeRequest(t, h, "DeleteCallAnalyticsJob", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// ── GetCallAnalyticsJob full field echo ────────────────────────────────────────

func TestGetCallAnalyticsJob_FullFieldEcho(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	doTranscribeRequest(t, h, "StartCallAnalyticsJob", map[string]any{
		"CallAnalyticsJobName": "ca-echo-job",
		"LanguageCode":         "en-US",
		"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
		"DataAccessRoleArn":    "arn:aws:iam::123456789012:role/CARole",
		"ChannelDefinitions": []map[string]any{
			{"ChannelId": 0, "ParticipantRole": "AGENT"},
			{"ChannelId": 1, "ParticipantRole": "CUSTOMER"},
		},
	})

	rec := doTranscribeRequest(t, h, "GetCallAnalyticsJob", map[string]any{
		"CallAnalyticsJobName": "ca-echo-job",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "ca-echo-job")
	assert.Contains(t, body, "COMPLETED")
	assert.Contains(t, body, "en-US")
	assert.Contains(t, body, "DataAccessRoleArn")
	assert.Contains(t, body, "ChannelDefinitions")
	assert.Contains(t, body, "AGENT")
	assert.Contains(t, body, "CreationTime")
}

// ── CallAnalyticsJob Tags in input ────────────────────────────────────────────

func TestStartCallAnalyticsJob_TagsInInput(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	rec := doTranscribeRequest(t, h, "StartCallAnalyticsJob", map[string]any{
		"CallAnalyticsJobName": "ca-tagged-job",
		"LanguageCode":         "en-US",
		"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
		"Tags":                 []map[string]string{{"Key": "project", "Value": "sales"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Tags should be echoed in response.
	assert.Contains(t, rec.Body.String(), "ca-tagged-job")
}

// TestCallAnalyticsJobIncludesTranscriptURI verifies that completed
// CallAnalytics jobs include a Transcript.TranscriptFileUri in the response.
// Real AWS always populates this field for COMPLETED jobs; the emulator
// previously omitted it, causing callers to get an empty transcript URI.
func TestCallAnalyticsJobIncludesTranscriptURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		media    map[string]any
		langCode string
	}{
		{
			name:     "s3_media_uri",
			langCode: "en-US",
			media:    map[string]any{"MediaFileUri": "s3://my-bucket/call.mp3"},
		},
		{
			name:     "wav_format",
			langCode: "es-US",
			media:    map[string]any{"MediaFileUri": "s3://calls-bucket/recording.wav"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestTranscribeHandler(t)
			jobName := "parity-ca-job-" + tt.name

			startRec := doTranscribeRequest(t, h, "StartCallAnalyticsJob", map[string]any{
				"CallAnalyticsJobName": jobName,
				"LanguageCode":         tt.langCode,
				"Media":                tt.media,
			})
			require.Equal(t, http.StatusOK, startRec.Code)

			getRec := doTranscribeRequest(t, h, "GetCallAnalyticsJob", map[string]any{
				"CallAnalyticsJobName": jobName,
			})
			require.Equal(t, http.StatusOK, getRec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &resp))

			job, ok := resp["CallAnalyticsJob"].(map[string]any)
			require.True(t, ok, "CallAnalyticsJob must be present")
			assert.Equal(t, "COMPLETED", job["CallAnalyticsJobStatus"])

			transcript, ok := job["Transcript"].(map[string]any)
			require.True(t, ok, "Transcript must be present in COMPLETED job response")

			uri, _ := transcript["TranscriptFileUri"].(string)
			assert.NotEmpty(t, uri, "TranscriptFileUri must be non-empty")
			assert.True(t, strings.HasPrefix(uri, "s3://"),
				"TranscriptFileUri must be an S3 URI, got: %s", uri)
			assert.Contains(t, uri, jobName,
				"TranscriptFileUri must include the job name")
		})
	}
}

// TestChannelId_JSONKey verifies the JSON key is "ChannelId" not "ChannelID".
func TestChannelId_JSONKey(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)

	rec := doTranscribeRequest(t, h, "StartCallAnalyticsJob", map[string]any{
		"CallAnalyticsJobName": "chan-id-job",
		"DataAccessRoleArn":    "arn:aws:iam::123456789012:role/transcribe",
		"Media":                map[string]any{"MediaFileUri": "s3://my-bucket/call.mp3"},
		"ChannelDefinitions": []map[string]any{
			{"ChannelId": 0, "ParticipantRole": "AGENT"},
			{"ChannelId": 1, "ParticipantRole": "CUSTOMER"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "start job: %s", rec.Body)

	descRec := doTranscribeRequest(t, h, "GetCallAnalyticsJob", map[string]any{
		"CallAnalyticsJobName": "chan-id-job",
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &raw))
	job, _ := raw["CallAnalyticsJob"].(map[string]any)
	require.NotNil(t, job, "CallAnalyticsJob must be present")

	defs, _ := job["ChannelDefinitions"].([]any)
	require.NotEmpty(t, defs, "ChannelDefinitions must be present")

	def0, _ := defs[0].(map[string]any)
	_, hasID := def0["ChannelId"]
	_, hasWrong := def0["ChannelID"]

	assert.True(t, hasID, "ChannelDefinition must use ChannelId (not ChannelID)")
	assert.False(t, hasWrong, "ChannelID must NOT appear (wrong casing)")
}

func TestHTTP_ListCallAnalyticsJobs(t *testing.T) {
	t.Parallel()
	h := newTestTranscribeHandler(t)
	rec := doTranscribeRequest(t, h, "StartCallAnalyticsJob", map[string]any{
		"CallAnalyticsJobName": "list-job",
		"DataAccessRoleArn":    "arn:aws:iam::123456789012:role/transcribe",
		"Media":                map[string]any{"MediaFileUri": "s3://bucket/call.mp3"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	listRec := doTranscribeRequest(t, h, "ListCallAnalyticsJobs", map[string]any{})
	assert.Equal(t, http.StatusOK, listRec.Code)
	assert.Contains(t, listRec.Body.String(), "list-job")
}

func TestHTTP_CallAnalyticsCategory(t *testing.T) {
	t.Parallel()
	h := newTestTranscribeHandler(t)

	// Create
	rec := doTranscribeRequest(t, h, "CreateCallAnalyticsCategory", map[string]any{
		"CategoryName": "test-cat",
		"Rules": []map[string]any{
			{
				"NonTalkTimeFilter": map[string]any{
					"Threshold": 1000,
					"AbsoluteTimeRange": map[string]any{
						"First": 5000,
					},
				},
			},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Get
	getRec := doTranscribeRequest(t, h, "GetCallAnalyticsCategory", map[string]any{
		"CategoryName": "test-cat",
	})
	assert.Equal(t, http.StatusOK, getRec.Code)
	assert.Contains(t, getRec.Body.String(), "test-cat")

	// Update
	upRec := doTranscribeRequest(t, h, "UpdateCallAnalyticsCategory", map[string]any{
		"CategoryName": "test-cat",
		"Rules": []map[string]any{
			{
				"InterruptionFilter": map[string]any{
					"Threshold": 2000,
				},
			},
		},
	})
	assert.Equal(t, http.StatusOK, upRec.Code)

	// List
	listRec := doTranscribeRequest(t, h, "ListCallAnalyticsCategories", map[string]any{})
	assert.Equal(t, http.StatusOK, listRec.Code)
	assert.Contains(t, listRec.Body.String(), "test-cat")
}

// TestCallAnalyticsCategory_CreateTimeAndLastUpdateTimeEchoed verifies
// CategoryProperties includes CreateTime and LastUpdateTime, real fields
// gopherstack previously dropped from Create/Get/Update/List responses.
func TestCallAnalyticsCategory_CreateTimeAndLastUpdateTimeEchoed(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)

	createRec := doTranscribeRequest(t, h, "CreateCallAnalyticsCategory", map[string]any{
		"CategoryName": "time-fields-cat",
		"InputType":    "POST_CALL",
		"Rules": []map[string]any{
			{"NonTalkTimeFilter": map[string]any{"Threshold": 30000}},
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code, createRec.Body.String())

	var createRaw struct {
		CategoryProperties map[string]json.RawMessage `json:"CategoryProperties"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createRaw))

	for _, field := range []string{"CreateTime", "LastUpdateTime"} {
		val, ok := createRaw.CategoryProperties[field]
		require.True(t, ok, "expected field %s in CreateCallAnalyticsCategory response", field)

		var num json.Number
		require.NoError(t, json.Unmarshal(val, &num), "field %s must be a JSON number (epoch seconds)", field)
	}

	updateRec := doTranscribeRequest(t, h, "UpdateCallAnalyticsCategory", map[string]any{
		"CategoryName": "time-fields-cat",
		"InputType":    "POST_CALL",
		"Rules": []map[string]any{
			{"NonTalkTimeFilter": map[string]any{"Threshold": 30000}},
		},
	})
	require.Equal(t, http.StatusOK, updateRec.Code)

	var updateRaw struct {
		CategoryProperties map[string]json.RawMessage `json:"CategoryProperties"`
	}
	require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updateRaw))
	assert.Contains(t, updateRaw.CategoryProperties, "LastUpdateTime")
}

// TestListCallAnalyticsJobs_JobNameContainsAndStartTime verifies the JobNameContains
// filter and that StartTime is present in each summary, matching the real
// ListCallAnalyticsJobsInput/CallAnalyticsJobSummary fields.
func TestListCallAnalyticsJobs_JobNameContainsAndStartTime(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	h := transcribe.NewHandler(b)

	for _, name := range []string{"support-call-1", "support-call-2", "sales-call-1"} {
		rec := doTranscribeRequest(t, h, "StartCallAnalyticsJob", map[string]any{
			"CallAnalyticsJobName": name,
			"DataAccessRoleArn":    "arn:aws:iam::123456789012:role/transcribe",
			"Media":                map[string]any{"MediaFileUri": "s3://bucket/call.mp3"},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	list, _ := b.ListCallAnalyticsJobs("", "support", "", 0)
	require.Len(t, list, 2)

	listRec := doTranscribeRequest(t, h, "ListCallAnalyticsJobs", map[string]any{
		"JobNameContains": "sales",
	})
	require.Equal(t, http.StatusOK, listRec.Code)

	var raw struct {
		CallAnalyticsJobSummaries []map[string]json.RawMessage `json:"CallAnalyticsJobSummaries"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &raw))
	require.Len(t, raw.CallAnalyticsJobSummaries, 1)
	assert.Contains(t, raw.CallAnalyticsJobSummaries[0], "StartTime")
}
