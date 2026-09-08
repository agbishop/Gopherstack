package transcribe_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/transcribe"
)

// newTestTranscribeHandler creates a Handler backed by a fresh InMemoryBackend.
func newTestTranscribeHandler(t *testing.T) *transcribe.Handler {
	t.Helper()

	return transcribe.NewHandler(transcribe.NewInMemoryBackend())
}

// newHandlerWithBackend creates a Handler and returns it alongside its backing
// InMemoryBackend, for tests that need to seed or inspect backend state directly.
func newHandlerWithBackend(t *testing.T) (*transcribe.Handler, *transcribe.InMemoryBackend) {
	t.Helper()

	b := transcribe.NewInMemoryBackend()
	h := transcribe.NewHandler(b)

	return h, b
}

func doTranscribeRequest(t *testing.T, h *transcribe.Handler, action string, body any) *httptest.ResponseRecorder {
	t.Helper()

	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Transcribe."+action)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err = h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestTranscribe_Name(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)
	assert.Equal(t, "Transcribe", h.Name())
}

func TestTranscribe_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)
	ops := h.GetSupportedOperations()
	assert.Contains(t, ops, "StartTranscriptionJob")
	assert.Contains(t, ops, "GetTranscriptionJob")
	assert.Contains(t, ops, "ListTranscriptionJobs")
}

// TestTranscribe_SupportedOperations_CountAndOrder verifies the dispatch table and
// the advertised operation list agree on the number of supported operations (43),
// and that GetSupportedOperations returns them in sorted order.
func TestTranscribe_SupportedOperations_CountAndOrder(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)
	ops := h.GetSupportedOperations()

	require.NotEmpty(t, ops)
	assert.Len(t, ops, 43)
	assert.Equal(t, 43, transcribe.HandlerOpsLen(h))

	for i := 1; i < len(ops); i++ {
		assert.LessOrEqual(t, ops[i-1], ops[i],
			"ops not sorted at index %d: %s > %s", i, ops[i-1], ops[i])
	}
}

func TestTranscribe_MatchPriority(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)
	assert.Equal(t, 100, h.MatchPriority())
}

func TestTranscribe_RouteMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{
			name:   "matching_request",
			target: "Transcribe.StartTranscriptionJob",
			want:   true,
		},
		{
			name:   "non_matching_request",
			target: "OtherService.Action",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestTranscribeHandler(t)
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.want, h.RouteMatcher()(c))
		})
	}
}

func TestTranscribe_ExtractOperation(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Amz-Target", "Transcribe.StartTranscriptionJob")
	c := e.NewContext(req, httptest.NewRecorder())
	assert.Equal(t, "StartTranscriptionJob", h.ExtractOperation(c))
}

func TestTranscribe_ExtractResource(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)
	e := echo.New()
	body := `{"TranscriptionJobName":"my-job"}`
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body)))
	req.Header.Set("X-Amz-Target", "Transcribe.GetTranscriptionJob")
	c := e.NewContext(req, httptest.NewRecorder())
	assert.Equal(t, "my-job", h.ExtractResource(c))
}

type transcribeSetupAction struct {
	body   map[string]any
	action string
}

func TestTranscribe_HandlerActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body         map[string]any
		name         string
		action       string
		setupActions []transcribeSetupAction
		wantContains []string
		wantCode     int
	}{
		{
			name:   "StartTranscriptionJob",
			action: "StartTranscriptionJob",
			body: map[string]any{
				"TranscriptionJobName": "test-job",
				"LanguageCode":         "en-US",
				"Media": map[string]any{
					"MediaFileURI": "s3://my-bucket/audio.mp3",
				},
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"test-job", "COMPLETED"},
		},
		{
			name: "StartTranscriptionJob_AlreadyExists",
			setupActions: []transcribeSetupAction{
				{action: "StartTranscriptionJob", body: map[string]any{
					"TranscriptionJobName": "dup-job",
					"LanguageCode":         "en-US",
				}},
			},
			action: "StartTranscriptionJob",
			body: map[string]any{
				"TranscriptionJobName": "dup-job",
				"LanguageCode":         "en-US",
			},
			wantCode: http.StatusConflict,
		},
		{
			name: "GetTranscriptionJob",
			setupActions: []transcribeSetupAction{
				{action: "StartTranscriptionJob", body: map[string]any{
					"TranscriptionJobName": "get-job",
					"LanguageCode":         "en-US",
				}},
			},
			action: "GetTranscriptionJob",
			body: map[string]any{
				"TranscriptionJobName": "get-job",
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"get-job"},
		},
		{
			name:   "GetTranscriptionJob_NotFound",
			action: "GetTranscriptionJob",
			body: map[string]any{
				"TranscriptionJobName": "no-such-job",
			},
			wantCode: http.StatusNotFound,
		},
		{
			name: "ListTranscriptionJobs",
			setupActions: []transcribeSetupAction{
				{action: "StartTranscriptionJob", body: map[string]any{
					"TranscriptionJobName": "list-job-1",
					"LanguageCode":         "en-US",
				}},
				{action: "StartTranscriptionJob", body: map[string]any{
					"TranscriptionJobName": "list-job-2",
					"LanguageCode":         "en-US",
				}},
			},
			action:       "ListTranscriptionJobs",
			body:         map[string]any{},
			wantCode:     http.StatusOK,
			wantContains: []string{"list-job-1", "list-job-2"},
		},
		{
			name:     "UnknownAction",
			action:   "UnknownAction",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestTranscribeHandler(t)

			for _, sa := range tt.setupActions {
				doTranscribeRequest(t, h, sa.action, sa.body)
			}

			rec := doTranscribeRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, want := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), want)
			}
		})
	}
}

// TestTranscribe_ConflictIs409 verifies that duplicate resources produce HTTP 409
// across every op family that supports creation.
func TestTranscribe_ConflictIs409(t *testing.T) {
	t.Parallel()

	tests := []struct {
		first  map[string]any
		second map[string]any
		name   string
		action string
	}{
		{
			name:   "transcription_job",
			action: "StartTranscriptionJob",
			first:  map[string]any{"TranscriptionJobName": "job-dup", "LanguageCode": "en-US"},
			second: map[string]any{"TranscriptionJobName": "job-dup", "LanguageCode": "en-US"},
		},
		{
			name:   "call_analytics_category",
			action: "CreateCallAnalyticsCategory",
			first: map[string]any{
				"CategoryName": "cat-dup",
				"Rules":        []map[string]any{{"NonTalkTimeFilter": map[string]any{"Threshold": 30000}}},
			},
			second: map[string]any{
				"CategoryName": "cat-dup",
				"Rules":        []map[string]any{{"NonTalkTimeFilter": map[string]any{"Threshold": 30000}}},
			},
		},
		{
			name:   "language_model",
			action: "CreateLanguageModel",
			first: map[string]any{
				"ModelName":     "mdl-dup",
				"BaseModelName": "WideBand",
				"LanguageCode":  "en-US",
				"InputDataConfig": map[string]any{
					"S3Uri":             "s3://bucket/training/",
					"DataAccessRoleArn": "arn:aws:iam::123456789012:role/TranscribeRole",
				},
			},
			second: map[string]any{
				"ModelName":     "mdl-dup",
				"BaseModelName": "WideBand",
				"LanguageCode":  "en-US",
				"InputDataConfig": map[string]any{
					"S3Uri":             "s3://bucket/training/",
					"DataAccessRoleArn": "arn:aws:iam::123456789012:role/TranscribeRole",
				},
			},
		},
		{
			name:   "vocabulary",
			action: "CreateVocabulary",
			first:  map[string]any{"VocabularyName": "voc-dup", "LanguageCode": "en-US"},
			second: map[string]any{"VocabularyName": "voc-dup", "LanguageCode": "en-US"},
		},
		{
			name:   "vocabulary_filter",
			action: "CreateVocabularyFilter",
			first: map[string]any{
				"VocabularyFilterName": "flt-dup",
				"LanguageCode":         "en-US",
				"Words":                []string{"bad"},
			},
			second: map[string]any{
				"VocabularyFilterName": "flt-dup",
				"LanguageCode":         "en-US",
				"Words":                []string{"bad"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestTranscribeHandler(t)

			rec1 := doTranscribeRequest(t, h, tt.action, tt.first)
			require.Equal(t, http.StatusOK, rec1.Code)

			rec2 := doTranscribeRequest(t, h, tt.action, tt.second)
			assert.Equal(t, http.StatusConflict, rec2.Code)
			assert.Contains(t, rec2.Body.String(), "ConflictException")
		})
	}
}

// TestTranscribe_ValidationIs400 verifies that missing required fields produce
// HTTP 400 across every op family.
func TestTranscribe_ValidationIs400(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{
			name:   "start_job_missing_job_name",
			action: "StartTranscriptionJob",
			body:   map[string]any{"LanguageCode": "en-US"},
		},
		{
			name:   "start_job_missing_language_code",
			action: "StartTranscriptionJob",
			body:   map[string]any{"TranscriptionJobName": "j1"},
		},
		{
			name:   "create_language_model_missing_base_model",
			action: "CreateLanguageModel",
			body:   map[string]any{"ModelName": "m1", "LanguageCode": "en-US"},
		},
		{
			name:   "create_language_model_missing_language_code",
			action: "CreateLanguageModel",
			body:   map[string]any{"ModelName": "m1", "BaseModelName": "WideBand"},
		},
		{
			name:   "create_medical_vocab_missing_file_uri",
			action: "CreateMedicalVocabulary",
			body:   map[string]any{"VocabularyName": "v1", "LanguageCode": "en-US"},
		},
		{
			name:   "create_vocabulary_missing_language",
			action: "CreateVocabulary",
			body:   map[string]any{"VocabularyName": "v1"},
		},
		{
			name:   "create_vocabulary_filter_missing_language",
			action: "CreateVocabularyFilter",
			body:   map[string]any{"VocabularyFilterName": "f1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestTranscribeHandler(t)
			rec := doTranscribeRequest(t, h, tt.action, tt.body)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "BadRequestException")
		})
	}
}

// TestTranscribe_BadParams_NotInternalFailure asserts bad params return
// BadRequestException, not InternalFailureException.
func TestTranscribe_BadParams_NotInternalFailure(t *testing.T) {
	t.Parallel()

	const wantType = "BadRequestException"

	tests := []struct {
		body     map[string]any
		name     string
		action   string
		wantType string
		wantCode int
	}{
		{
			name:     "StartTranscriptionJob_missing_name",
			action:   "StartTranscriptionJob",
			body:     map[string]any{"LanguageCode": "en-US"},
			wantCode: http.StatusBadRequest,
			wantType: wantType,
		},
		{
			name:   "StartTranscriptionJob_invalid_language_code",
			action: "StartTranscriptionJob",
			body: map[string]any{
				"TranscriptionJobName": "bad-lang-job",
				"LanguageCode":         "xx-INVALID",
			},
			wantCode: http.StatusBadRequest,
			wantType: wantType,
		},
		{
			name:   "StartTranscriptionJob_invalid_media_format",
			action: "StartTranscriptionJob",
			body: map[string]any{
				"TranscriptionJobName": "bad-format-job",
				"LanguageCode":         "en-US",
				"MediaFormat":          "bmp",
			},
			wantCode: http.StatusBadRequest,
			wantType: wantType,
		},
		{
			name:   "StartTranscriptionJob_invalid_sample_rate",
			action: "StartTranscriptionJob",
			body: map[string]any{
				"TranscriptionJobName": "bad-rate-job",
				"LanguageCode":         "en-US",
				"MediaSampleRateHertz": 100,
			},
			wantCode: http.StatusBadRequest,
			wantType: wantType,
		},
		{
			name:     "StartCallAnalyticsJob_missing_name",
			action:   "StartCallAnalyticsJob",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
			wantType: wantType,
		},
		{
			name:     "DeleteTranscriptionJob_missing_name",
			action:   "DeleteTranscriptionJob",
			body:     map[string]any{"TranscriptionJobName": ""},
			wantCode: http.StatusBadRequest,
			wantType: wantType,
		},
		{
			name:   "CreateVocabulary_missing_name",
			action: "CreateVocabulary",
			body: map[string]any{
				"LanguageCode":      "en-US",
				"VocabularyFileUri": "s3://bucket/vocab.txt",
			},
			wantCode: http.StatusBadRequest,
			wantType: wantType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := transcribe.NewHandler(transcribe.NewInMemoryBackend())
			rec := doTranscribeRequest(t, h, tt.action, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			assert.Equal(t, tt.wantType, resp["__type"], "expected %s, not InternalFailureException", tt.wantType)
		})
	}
}

// TestTranscribe_ErrorBodyContainsType verifies error responses include __type.
func TestTranscribe_ErrorBodyContainsType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		action   string
		body     map[string]any
		wantType string
		wantCode int
	}{
		{
			name:     "not_found_returns_NotFoundException",
			action:   "GetTranscriptionJob",
			body:     map[string]any{"TranscriptionJobName": "no-such"},
			wantType: "NotFoundException",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "missing_name_returns_BadRequestException",
			action:   "StartTranscriptionJob",
			body:     map[string]any{},
			wantType: "BadRequestException",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "unknown_op_returns_UnknownOperationException",
			action:   "NoSuchOp",
			body:     map[string]any{},
			wantType: "UnknownOperationException",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestTranscribeHandler(t)

			rec := doTranscribeRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantType)
		})
	}
}

// TestTranscribe_DeleteJobMissingName verifies empty name returns 400 across
// every job family's Delete operation.
func TestTranscribe_DeleteJobMissingName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{
			name:   "delete_transcription_job_empty_name",
			action: "DeleteTranscriptionJob",
			body:   map[string]any{"TranscriptionJobName": ""},
		},
		{
			name:   "delete_call_analytics_job_empty_name",
			action: "DeleteCallAnalyticsJob",
			body:   map[string]any{"CallAnalyticsJobName": ""},
		},
		{
			name:   "delete_medical_scribe_job_empty_name",
			action: "DeleteMedicalScribeJob",
			body:   map[string]any{"MedicalScribeJobName": ""},
		},
		{
			name:   "delete_medical_transcription_job_empty_name",
			action: "DeleteMedicalTranscriptionJob",
			body:   map[string]any{"MedicalTranscriptionJobName": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestTranscribeHandler(t)
			rec := doTranscribeRequest(t, h, tt.action, tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// ── Timestamp wire shape: epoch-seconds numbers, not RFC3339 strings ───────────
//
// The real aws-sdk-go-v2 transcribe deserializer parses CreationTime/StartTime/
// CompletionTime/CreateTime/LastModifiedTime via smithytime.ParseEpochSeconds,
// which requires a JSON number. A JSON string in that position makes the real
// SDK reject the response outright, so every timestamp field must round-trip
// as encoding/json.Number, never as a quoted string.

func TestTranscribe_TimestampFields_AreJSONNumbers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		action     string
		body       map[string]any
		timeFields []string
	}{
		{
			name:   "GetTranscriptionJob",
			action: "StartTranscriptionJob",
			body: map[string]any{
				"TranscriptionJobName": "ts-epoch-job",
				"LanguageCode":         "en-US",
				"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
			},
			timeFields: []string{"CreationTime", "StartTime", "CompletionTime"},
		},
		{
			name:   "StartCallAnalyticsJob",
			action: "StartCallAnalyticsJob",
			body: map[string]any{
				"CallAnalyticsJobName": "ca-epoch-job",
				"LanguageCode":         "en-US",
				"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
			},
			timeFields: []string{"CreationTime", "StartTime", "CompletionTime"},
		},
		{
			name:   "StartMedicalScribeJob",
			action: "StartMedicalScribeJob",
			body: map[string]any{
				"MedicalScribeJobName": "ms-epoch-job",
				"Media":                map[string]any{"MediaFileUri": "s3://b/f"},
				"DataAccessRoleArn":    "arn:aws:iam::123456789012:role/Scribe",
				"OutputBucketName":     "scribe-out",
				"Settings":             map[string]any{"ShowSpeakerLabels": true, "MaxSpeakerLabels": 2},
			},
			timeFields: []string{"CreationTime", "StartTime", "CompletionTime"},
		},
		{
			name:   "StartMedicalTranscriptionJob",
			action: "StartMedicalTranscriptionJob",
			body: map[string]any{
				"MedicalTranscriptionJobName": "mt-epoch-job",
				"LanguageCode":                "en-US",
				"Specialty":                   "PRIMARYCARE",
				"Type":                        "DICTATION",
				"Media":                       map[string]any{"MediaFileUri": "s3://b/f"},
			},
			timeFields: []string{"CreationTime", "StartTime", "CompletionTime"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestTranscribeHandler(t)

			rec := doTranscribeRequest(t, h, tt.action, tt.body)
			require.Equal(t, http.StatusOK, rec.Code)

			var raw map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

			// The response wraps the resource under a single top-level key; find it.
			var inner map[string]json.RawMessage
			for _, v := range raw {
				_ = json.Unmarshal(v, &inner)

				if inner != nil {
					break
				}
			}
			require.NotNil(t, inner, "expected a single wrapped resource object in %s", rec.Body.String())

			for _, field := range tt.timeFields {
				val, ok := inner[field]
				require.True(t, ok, "expected field %s in response %s", field, rec.Body.String())

				var num json.Number
				err := json.Unmarshal(val, &num)
				assert.NoError(t, err, "field %s must decode as a JSON number (epoch seconds), got %s", field, val)
			}
		})
	}
}
