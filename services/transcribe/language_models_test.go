package transcribe_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/transcribe"
)

func TestCreateLanguageModel_InputDataConfig(t *testing.T) {
	t.Parallel()

	t.Run("base_model_name_validated", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.CreateLanguageModel(&transcribe.LanguageModel{
			ModelName:     "my-model",
			BaseModelName: "InvalidBase",
			LanguageCode:  "en-US",
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("valid_narrow_band_accepted", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		m, err := b.CreateLanguageModel(&transcribe.LanguageModel{
			ModelName:     "narrow-model",
			BaseModelName: "NarrowBand",
			LanguageCode:  "en-US",
			InputDataConfig: &transcribe.InputDataConfig{
				S3Uri:             "s3://bucket/training/",
				DataAccessRoleArn: "arn:aws:iam::123456789012:role/TranscribeRole",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "NarrowBand", m.BaseModelName)
		assert.NotNil(t, m.InputDataConfig)
	})

	t.Run("input_data_config_required", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.CreateLanguageModel(&transcribe.LanguageModel{
			ModelName:     "model-no-input-data-config",
			BaseModelName: "WideBand",
			LanguageCode:  "en-US",
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})

	t.Run("input_data_config_s3_uri_required", func(t *testing.T) {
		t.Parallel()

		b := transcribe.NewInMemoryBackend()
		_, err := b.CreateLanguageModel(&transcribe.LanguageModel{
			ModelName:     "model-no-s3",
			BaseModelName: "WideBand",
			LanguageCode:  "en-US",
			InputDataConfig: &transcribe.InputDataConfig{
				DataAccessRoleArn: "arn:aws:iam::123456789012:role/TranscribeRole",
			},
		})
		require.ErrorIs(t, err, transcribe.ErrValidation)
	})
}

func TestHTTP_CreateLanguageModel_WithInputDataConfig(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	rec := doTranscribeRequest(t, h, "CreateLanguageModel", map[string]any{
		"ModelName":     "my-clm",
		"BaseModelName": "WideBand",
		"LanguageCode":  "en-US",
		"InputDataConfig": map[string]any{
			"S3Uri":             "s3://my-bucket/training/",
			"DataAccessRoleArn": "arn:aws:iam::123456789012:role/TranscribeRole",
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "my-clm")
}

func TestCreateLanguageModel(t *testing.T) {
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
				"ModelName":     "my-model",
				"BaseModelName": "WideBand",
				"LanguageCode":  "en-US",
				"InputDataConfig": map[string]any{
					"S3Uri":             "s3://bucket/training/",
					"DataAccessRoleArn": "arn:aws:iam::123456789012:role/TranscribeRole",
				},
			},
			wantCode: http.StatusOK,
			wantKey:  "my-model",
		},
		{
			name: "duplicate",
			setup: func(t *testing.T, b *transcribe.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateLanguageModel(
					&transcribe.LanguageModel{
						ModelName:     "dup-model",
						BaseModelName: "WideBand",
						LanguageCode:  "en-US",
						InputDataConfig: &transcribe.InputDataConfig{
							S3Uri:             "s3://bucket/training/",
							DataAccessRoleArn: "arn:aws:iam::123456789012:role/TranscribeRole",
						},
					},
				)
				require.NoError(t, err)
			},
			body: map[string]any{
				"ModelName":     "dup-model",
				"BaseModelName": "WideBand",
				"LanguageCode":  "en-US",
				"InputDataConfig": map[string]any{
					"S3Uri":             "s3://bucket/training/",
					"DataAccessRoleArn": "arn:aws:iam::123456789012:role/TranscribeRole",
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

			rec := doTranscribeRequest(t, h, "CreateLanguageModel", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantKey != "" {
				assert.Contains(t, rec.Body.String(), tt.wantKey)
			}
		})
	}
}

func TestDeleteLanguageModel(t *testing.T) {
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
				_, err := b.CreateLanguageModel(
					&transcribe.LanguageModel{
						ModelName:     "model-to-delete",
						BaseModelName: "WideBand",
						LanguageCode:  "en-US",
						InputDataConfig: &transcribe.InputDataConfig{
							S3Uri:             "s3://bucket/training/",
							DataAccessRoleArn: "arn:aws:iam::123456789012:role/TranscribeRole",
						},
					},
				)
				require.NoError(t, err)
			},
			body:     map[string]any{"ModelName": "model-to-delete"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			setup:    func(_ *testing.T, _ *transcribe.InMemoryBackend) {},
			body:     map[string]any{"ModelName": "missing-model"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := transcribe.NewInMemoryBackend()
			h := transcribe.NewHandler(b)
			tt.setup(t, b)

			rec := doTranscribeRequest(t, h, "DeleteLanguageModel", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestModelStatus_Completed verifies language model responds COMPLETED.
func TestModelStatus_Completed(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	h := transcribe.NewHandler(b)

	rec := doTranscribeRequest(t, h, "CreateLanguageModel", map[string]any{
		"ModelName":     "model-status-test",
		"BaseModelName": "WideBand",
		"LanguageCode":  "en-US",
		"InputDataConfig": map[string]any{
			"S3Uri":             "s3://bucket/training/",
			"DataAccessRoleArn": "arn:aws:iam::123456789012:role/TranscribeRole",
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "COMPLETED")
}

// ── LanguageModel InputDataConfig echo ────────────────────────────────────────

func TestDescribeLanguageModel_IncludesInputDataConfig(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	doTranscribeRequest(t, h, "CreateLanguageModel", map[string]any{
		"ModelName":     "my-model",
		"BaseModelName": "WideBand",
		"LanguageCode":  "en-US",
		"InputDataConfig": map[string]any{
			"S3Uri":             "s3://bucket/training/",
			"DataAccessRoleArn": "arn:aws:iam::123456789012:role/TranscribeRole",
		},
	})

	rec := doTranscribeRequest(t, h, "DescribeLanguageModel", map[string]any{
		"ModelName": "my-model",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "InputDataConfig")
	assert.Contains(t, body, "s3://bucket/training/")
}

// TestDescribeLanguageModel_TimeFields verifies CreateTime and LastModifiedTime
// are in the DescribeLanguageModel response.
func TestDescribeLanguageModel_TimeFields(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)

	createRec := doTranscribeRequest(t, h, "CreateLanguageModel", map[string]any{
		"ModelName":     "time-field-model",
		"BaseModelName": "NarrowBand",
		"LanguageCode":  "en-US",
		"InputDataConfig": map[string]any{
			"S3Uri":             "s3://my-bucket/data/",
			"DataAccessRoleArn": "arn:aws:iam::123456789012:role/transcribe",
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code, "create model: %s", createRec.Body)

	descRec := doTranscribeRequest(t, h, "DescribeLanguageModel", map[string]any{
		"ModelName": "time-field-model",
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &raw))
	lm, _ := raw["LanguageModel"].(map[string]any)
	require.NotNil(t, lm, "LanguageModel must be present")

	assert.NotNil(t, lm["CreateTime"], "CreateTime must be present in DescribeLanguageModel response")
	assert.NotNil(t, lm["LastModifiedTime"], "LastModifiedTime must be present in DescribeLanguageModel response")
}

// TestDescribeLanguageModel_UpgradeAvailability verifies UpgradeAvailability is in response.
func TestDescribeLanguageModel_UpgradeAvailability(t *testing.T) {
	t.Parallel()

	h := newTestTranscribeHandler(t)

	createRec := doTranscribeRequest(t, h, "CreateLanguageModel", map[string]any{
		"ModelName":     "upgrade-avail-model",
		"BaseModelName": "NarrowBand",
		"LanguageCode":  "en-US",
		"InputDataConfig": map[string]any{
			"S3Uri":             "s3://my-bucket/data/",
			"DataAccessRoleArn": "arn:aws:iam::123456789012:role/transcribe",
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	descRec := doTranscribeRequest(t, h, "DescribeLanguageModel", map[string]any{
		"ModelName": "upgrade-avail-model",
	})
	require.Equal(t, http.StatusOK, descRec.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &raw))
	lm, _ := raw["LanguageModel"].(map[string]any)
	require.NotNil(t, lm)

	_, hasField := lm["UpgradeAvailability"]
	assert.True(t, hasField, "UpgradeAvailability must be present in DescribeLanguageModel response")
}

// TestDescribeLanguageModel_TimestampFieldsAreJSONNumbers verifies CreateTime and
// LastModifiedTime round-trip as epoch-seconds JSON numbers, matching the real
// aws-sdk-go-v2 deserializer's expectations (see handler_test.go's equivalent
// check for Start/GetTranscriptionJob and friends).
func TestDescribeLanguageModel_TimestampFieldsAreJSONNumbers(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	h := transcribe.NewHandler(b)

	doTranscribeRequest(t, h, "CreateLanguageModel", map[string]any{
		"ModelName":     "lm-epoch-describe",
		"BaseModelName": "WideBand",
		"LanguageCode":  "en-US",
		"InputDataConfig": map[string]any{
			"S3Uri":             "s3://bucket/training/",
			"DataAccessRoleArn": "arn:aws:iam::123456789012:role/TranscribeRole",
		},
	})

	rec := doTranscribeRequest(t, h, "DescribeLanguageModel", map[string]any{
		"ModelName": "lm-epoch-describe",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		LanguageModel map[string]json.RawMessage `json:"LanguageModel"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	for _, field := range []string{"CreateTime", "LastModifiedTime"} {
		val, ok := out.LanguageModel[field]
		require.True(t, ok, "expected field %s in response %s", field, rec.Body.String())

		var num json.Number
		err := json.Unmarshal(val, &num)
		assert.NoError(t, err, "field %s must decode as a JSON number (epoch seconds), got %s", field, val)
	}
}

func TestHTTP_ListLanguageModels(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	h := transcribe.NewHandler(b)

	doTranscribeRequest(t, h, "CreateLanguageModel", map[string]any{
		"ModelName":     "lm-list",
		"BaseModelName": "WideBand",
		"LanguageCode":  "en-US",
		"InputDataConfig": map[string]any{
			"S3Uri":             "s3://bucket/training/",
			"DataAccessRoleArn": "arn:aws:iam::123456789012:role/TranscribeRole",
		},
	})

	rec := doTranscribeRequest(t, h, "ListLanguageModels", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "lm-list")
}

// TestListLanguageModels_NameContains verifies the NameContains filter
// (case-insensitive substring match), per the real ListLanguageModelsInput field.
func TestListLanguageModels_NameContains(t *testing.T) {
	t.Parallel()

	b := transcribe.NewInMemoryBackend()
	h := transcribe.NewHandler(b)

	for _, name := range []string{"clinical-notes-model", "clinical-summary-model", "sports-model"} {
		rec := doTranscribeRequest(t, h, "CreateLanguageModel", map[string]any{
			"ModelName":     name,
			"BaseModelName": "WideBand",
			"LanguageCode":  "en-US",
			"InputDataConfig": map[string]any{
				"S3Uri":             "s3://bucket/training/",
				"DataAccessRoleArn": "arn:aws:iam::123456789012:role/TranscribeRole",
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	list, _ := b.ListLanguageModels("", "clinical", "", 0)
	require.Len(t, list, 2)

	list, _ = b.ListLanguageModels("", "SPORTS", "", 0)
	require.Len(t, list, 1, "NameContains must be case-insensitive")
	assert.Equal(t, "sports-model", list[0].ModelName)
}

// TestCreateLanguageModel_EchoesInputDataConfig verifies CreateLanguageModelOutput
// echoes back InputDataConfig, a real field gopherstack previously dropped.
func TestCreateLanguageModel_EchoesInputDataConfig(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithBackend(t)
	rec := doTranscribeRequest(t, h, "CreateLanguageModel", map[string]any{
		"ModelName":     "echo-config-model",
		"BaseModelName": "WideBand",
		"LanguageCode":  "en-US",
		"InputDataConfig": map[string]any{
			"S3Uri":             "s3://my-bucket/training/",
			"DataAccessRoleArn": "arn:aws:iam::123456789012:role/TranscribeRole",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	cfg, ok := raw["InputDataConfig"].(map[string]any)
	require.True(t, ok, "CreateLanguageModelOutput must echo InputDataConfig")
	assert.Equal(t, "s3://my-bucket/training/", cfg["S3Uri"])
}
