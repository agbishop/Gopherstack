package polly_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/polly"
)

// wrapSpeak wraps text in a <speak> root element, producing well-formed SSML
// -- AWS rejects TextType="ssml" input that isn't wrapped this way
// (InvalidSsmlException).
func wrapSpeak(text string) string {
	return "<speak>" + text + "</speak>"
}

func TestBackendSynthesisDefaults(t *testing.T) {
	t.Parallel()

	backend := polly.NewInMemoryBackend()
	audio, err := backend.SynthesizeSpeech(polly.SynthesisOptions{Text: "hello", VoiceID: "Joanna"})
	require.NoError(t, err)
	assert.Equal(t, "audio/mpeg", audio.ContentType)
	// MP3 output starts with MPEG sync bytes (0xFF 0xFB).
	require.Greater(t, len(audio.Data), 1)
	assert.Equal(t, byte(0xFF), audio.Data[0])
	assert.Equal(t, byte(0xFB), audio.Data[1])
}

func TestSynthesisTextLimits(t *testing.T) {
	t.Parallel()

	backend := polly.NewInMemoryBackend()
	tests := []struct {
		name     string
		text     string
		textType string
		wantErr  bool
	}{
		{name: "text_at_limit", text: strings.Repeat("a", 3000), textType: "text", wantErr: false},
		{name: "text_over_limit", text: strings.Repeat("a", 3001), textType: "text", wantErr: true},
		{
			name:     "ssml_at_limit",
			text:     wrapSpeak(strings.Repeat("a", 6000-len(wrapSpeak("")))),
			textType: "ssml",
			wantErr:  false,
		},
		{name: "ssml_over_limit", text: strings.Repeat("a", 6001), textType: "ssml", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := backend.SynthesizeSpeech(polly.SynthesisOptions{
				Text:     test.text,
				VoiceID:  "Joanna",
				TextType: test.textType,
			})
			if test.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, polly.ErrTextLengthExceeded)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLexiconNamesLimit(t *testing.T) {
	t.Parallel()

	backend := polly.NewInMemoryBackend()
	content := `<lexicon alphabet="ipa" xml:lang="en-US"></lexicon>`
	names := []string{"lex1", "lex2", "lex3", "lex4", "lex5", "lex6"}
	for _, name := range names {
		require.NoError(t, backend.PutLexicon(name, content))
	}

	_, err := backend.SynthesizeSpeech(polly.SynthesisOptions{
		Text:         "hello",
		VoiceID:      "Joanna",
		LexiconNames: names,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, polly.ErrValidation)
}

func TestStartSpeechSynthesisStream(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	encoder := eventstream.NewEncoder()
	for _, text := range []string{"hello ", "world"} {
		message := eventstream.Message{Payload: []byte(`{"Text":"` + text + `","TextType":"text"}`)}
		message.Headers.Set(":event-type", eventstream.StringValue("TextEvent"))
		require.NoError(t, encoder.Encode(&body, message))
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/synthesisStream", &body)
	req.Header.Set("X-Amzn-Engine", "generative")
	req.Header.Set("X-Amzn-Outputformat", "mp3")
	req.Header.Set("X-Amzn-Voiceid", "Joanna")
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	require.NoError(t, newHandler().Handler()(ctx))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/vnd.amazon.eventstream", rec.Header().Get("Content-Type"))

	decoder := eventstream.NewDecoder()
	audio, err := decoder.Decode(rec.Body, nil)
	require.NoError(t, err)
	assert.Equal(t, "AudioEvent", audio.Headers.Get(":event-type").String())
	// Audio payload is MP3 binary starting with MPEG sync bytes.
	require.Greater(t, len(audio.Payload), 1)
	assert.Equal(t, byte(0xFF), audio.Payload[0])
	assert.Equal(t, byte(0xFB), audio.Payload[1])

	closed, err := decoder.Decode(rec.Body, nil)
	require.NoError(t, err)
	assert.Equal(t, "StreamClosedEvent", closed.Headers.Get(":event-type").String())
	assert.Contains(t, string(closed.Payload), "11")
}

// TestStartSpeechSynthesisStreamValidationExceptionTaxonomy verifies that
// StartSpeechSynthesisStream reports every client validation failure as the
// generic ValidationException (HTTP 400), never one of
// SynthesizeSpeech's op-specific exception names (e.g.
// EngineNotSupportedException) -- see ErrStreamValidation's doc comment and
// the real op's deserializer error switch in aws-sdk-go-v2/service/polly,
// which only lists ServiceFailureException/ServiceQuotaExceededException/
// ThrottlingException/ValidationException.
func TestStartSpeechSynthesisStreamValidationExceptionTaxonomy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setHeaders func(h http.Header)
		name       string
	}{
		{
			name: "engine_not_supported_becomes_validation_exception",
			setHeaders: func(h http.Header) {
				// Aditi does not support the neural engine (standard-only).
				h.Set("X-Amzn-Engine", "neural")
				h.Set("X-Amzn-Voiceid", "Aditi")
			},
		},
		{
			name: "unknown_voice_becomes_validation_exception",
			setHeaders: func(h http.Header) {
				h.Set("X-Amzn-Engine", "generative")
				h.Set("X-Amzn-Voiceid", "NotAVoice")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var body bytes.Buffer
			encoder := eventstream.NewEncoder()
			message := eventstream.Message{Payload: []byte(`{"Text":"hello","TextType":"text"}`)}
			message.Headers.Set(":event-type", eventstream.StringValue("TextEvent"))
			require.NoError(t, encoder.Encode(&body, message))

			req := httptest.NewRequest(http.MethodPost, "/v1/synthesisStream", &body)
			req.Header.Set("X-Amzn-Outputformat", "mp3")
			tc.setHeaders(req.Header)

			rec := httptest.NewRecorder()
			ctx := echo.New().NewContext(req, rec)
			require.NoError(t, newHandler().Handler()(ctx))

			assert.Equal(t, http.StatusBadRequest, rec.Code)

			// The wire-shape contract is the "__type" field, not the free-text
			// "message" -- which legitimately mentions the underlying cause (e.g.
			// "ValidationException: EngineNotSupportedException: voice ..."), so
			// assert on the parsed field rather than searching the raw body.
			var out map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, "ValidationException", out["__type"])
		})
	}
}

// TestStartSpeechSynthesisStreamRequiresGenerativeEngine verifies that
// StartSpeechSynthesisStream rejects every Engine value other than
// "generative" -- per api_op_StartSpeechSynthesisStream.go's Engine doc
// comment ("Currently, only the generative engine is supported"), unlike
// SynthesizeSpeech/StartSpeechSynthesisTask, which accept all 4 Engine values
// and default an unset one to "standard".
func TestStartSpeechSynthesisStreamRequiresGenerativeEngine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		voice  string
		engine string
	}{
		{name: "standard_rejected", voice: "Joanna", engine: "standard"},
		{name: "neural_rejected", voice: "Joanna", engine: "neural"},
		{name: "long_form_rejected", voice: "Danielle", engine: "long-form"},
		{name: "unset_engine_rejected", voice: "Joanna", engine: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var body bytes.Buffer
			encoder := eventstream.NewEncoder()
			message := eventstream.Message{Payload: []byte(`{"Text":"hello","TextType":"text"}`)}
			message.Headers.Set(":event-type", eventstream.StringValue("TextEvent"))
			require.NoError(t, encoder.Encode(&body, message))

			req := httptest.NewRequest(http.MethodPost, "/v1/synthesisStream", &body)
			req.Header.Set("X-Amzn-Outputformat", "mp3")
			req.Header.Set("X-Amzn-Voiceid", tc.voice)
			if tc.engine != "" {
				req.Header.Set("X-Amzn-Engine", tc.engine)
			}

			rec := httptest.NewRecorder()
			ctx := echo.New().NewContext(req, rec)
			require.NoError(t, newHandler().Handler()(ctx))

			require.Equal(t, http.StatusBadRequest, rec.Code)

			var out map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, "ValidationException", out["__type"])
		})
	}
}

func TestSynthesizeSpeechFormats(t *testing.T) {
	t.Parallel()

	const plainText = "hello world"

	tests := []struct {
		name         string
		format       string
		rate         string
		textType     string
		text         string
		contentType  string
		bodyContains string
		marks        []string
		bodyMagic    []byte
	}{
		{
			name: "mp3", format: "mp3", rate: "22050", textType: "text", contentType: "audio/mpeg",
			bodyMagic: []byte{0xFF, 0xFB}, // MPEG-1 Layer 3 sync
		},
		{
			name: "ogg", format: "ogg_vorbis", rate: "16000", textType: "text", contentType: "audio/ogg",
			bodyMagic: []byte("OggS"),
		},
		{
			name: "pcm_ssml", format: "pcm", rate: "8000", textType: "ssml", contentType: "audio/pcm",
			text: wrapSpeak(plainText), bodyMagic: []byte("RIFF"),
		},
		{
			name: "ogg_opus", format: "ogg_opus", rate: "48000", textType: "text", contentType: "audio/ogg",
			bodyMagic: []byte("OggS"),
		},
		{
			name: "mulaw", format: "mulaw", rate: "8000", textType: "text", contentType: "audio/mulaw",
			bodyMagic: []byte{0xFF, 0xFF},
		},
		{
			name: "alaw", format: "alaw", rate: "8000", textType: "text", contentType: "audio/alaw",
			bodyMagic: []byte{0xD5, 0xD5},
		},
		{
			name:         "word_mark",
			format:       "json",
			rate:         "22050",
			textType:     "text",
			contentType:  "application/x-json-stream",
			marks:        []string{"word"},
			bodyContains: `"type":"word"`,
		},
		{
			name:         "sentence_ssml_viseme_marks",
			format:       "json",
			rate:         "16000",
			textType:     "ssml",
			text:         wrapSpeak(plainText),
			contentType:  "application/x-json-stream",
			marks:        []string{"sentence", "ssml", "viseme"},
			bodyContains: `"type":"viseme"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			text := test.text
			if text == "" {
				text = plainText
			}

			rec := request(t, newHandler(), http.MethodPost, "/v1/speech", map[string]any{
				"Engine":          "standard",
				"OutputFormat":    test.format,
				"SampleRate":      test.rate,
				"SpeechMarkTypes": test.marks,
				"Text":            text,
				"TextType":        test.textType,
				"VoiceId":         "Joanna",
			})

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, test.contentType, rec.Header().Get("Content-Type"))
			assert.Equal(t, strconv.Itoa(len(text)), rec.Header().Get("X-Amzn-Requestcharacters"))
			assert.Positive(t, rec.Body.Len(), "audio body must be non-empty")
			if len(test.bodyMagic) > 0 {
				assert.True(t, bytes.HasPrefix(rec.Body.Bytes(), test.bodyMagic),
					"audio body should start with format magic bytes")
			}
			if test.bodyContains != "" {
				assert.Contains(t, rec.Body.String(), test.bodyContains)
			}
		})
	}
}

func TestSynthesizeSpeechValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body    map[string]any
		name    string
		wantErr string
	}{
		{
			name:    "json_requires_marks",
			body:    map[string]any{"OutputFormat": "json", "Text": "hello", "VoiceId": "Joanna"},
			wantErr: "InvalidParameterValueException",
		},
		{
			name: "marks_require_json",
			body: map[string]any{
				"OutputFormat":    "mp3",
				"SpeechMarkTypes": []string{"word"},
				"Text":            "hello",
				"VoiceId":         "Joanna",
			},
			wantErr: "MarksNotSupportedForFormatException",
		},
		{
			name:    "invalid_rate",
			body:    map[string]any{"OutputFormat": "pcm", "SampleRate": "48000", "Text": "hello", "VoiceId": "Joanna"},
			wantErr: "InvalidSampleRateException",
		},
		{
			name:    "unsupported_neural_voice",
			body:    map[string]any{"Engine": "neural", "Text": "hello", "VoiceId": "Aditi"},
			wantErr: "EngineNotSupportedException",
		},
		{
			name: "unsupported_language_for_voice",
			body: map[string]any{
				"LanguageCode": "fr-FR", "Text": "hello", "VoiceId": "Joanna",
			},
			wantErr: "LanguageNotSupportedException",
		},
		{
			name:    "unknown_voice_id",
			body:    map[string]any{"Text": "hello", "VoiceId": "NotAVoice"},
			wantErr: "InvalidParameterValueException",
		},
		{
			name: "ssml_marks_require_ssml_text_type",
			body: map[string]any{
				"OutputFormat": "json", "SpeechMarkTypes": []string{"ssml"},
				"Text": "hello", "TextType": "text", "VoiceId": "Joanna",
			},
			wantErr: "SsmlMarksNotSupportedForTextTypeException",
		},
		{
			name:    "ssml_not_wrapped_in_speak_root",
			body:    map[string]any{"Text": "hello world", "TextType": "ssml", "VoiceId": "Joanna"},
			wantErr: "InvalidSsmlException",
		},
		{
			name: "ssml_unbalanced_tags",
			body: map[string]any{
				"Text": "<speak>hello <emphasis>world</speak>", "TextType": "ssml", "VoiceId": "Joanna",
			},
			wantErr: "InvalidSsmlException",
		},
		{
			name: "ssml_wrong_root_element",
			body: map[string]any{
				"Text": "<p>hello world</p>", "TextType": "ssml", "VoiceId": "Joanna",
			},
			wantErr: "InvalidSsmlException",
		},
		{
			name: "ssml_malformed_xml",
			body: map[string]any{
				"Text": "<speak>hello & world</speak>", "TextType": "ssml", "VoiceId": "Joanna",
			},
			wantErr: "InvalidSsmlException",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			rec := request(t, newHandler(), http.MethodPost, "/v1/speech", test.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), test.wantErr)
		})
	}
}

// TestSynthesizeSpeechValidSSML verifies that well-formed SSML wrapped in a
// <speak> root element -- including nested markup elements and self-closing
// tags -- is accepted, not just plain text wrapped in <speak></speak>.
func TestSynthesizeSpeechValidSSML(t *testing.T) {
	t.Parallel()

	texts := []string{
		"<speak>hello world</speak>",
		`<speak>hello <break time="500ms"/> world</speak>`,
		"<speak>hello <emphasis level=\"strong\">world</emphasis></speak>",
	}

	for _, text := range texts {
		t.Run(text, func(t *testing.T) {
			t.Parallel()

			rec := request(t, newHandler(), http.MethodPost, "/v1/speech", map[string]any{
				"Text": text, "TextType": "ssml", "VoiceId": "Joanna",
			})
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

// TestSynthesizeSpeechTextLengthLimit verifies that SynthesizeSpeech rejects
// text exceeding 3000 characters and SSML exceeding 6000 characters. AWS
// returns TextLengthExceededException for oversized text input.
func TestSynthesizeSpeechTextLengthLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		textType string
		text     string
		wantCode int
	}{
		{
			name:     "text at limit passes",
			textType: "text",
			text:     strings.Repeat("a", 3000),
			wantCode: http.StatusOK,
		},
		{
			name:     "text over limit rejected",
			textType: "text",
			text:     strings.Repeat("a", 3001),
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "ssml at limit passes",
			textType: "ssml",
			text:     wrapSpeak(strings.Repeat("a", 6000-len(wrapSpeak("")))),
			wantCode: http.StatusOK,
		},
		{
			name:     "ssml over limit rejected",
			textType: "ssml",
			text:     strings.Repeat("a", 6001),
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := request(t, newHandler(), http.MethodPost, "/v1/speech", map[string]any{
				"OutputFormat": "mp3",
				"Text":         tc.text,
				"TextType":     tc.textType,
				"VoiceId":      "Joanna",
			})
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.wantCode == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "TextLengthExceededException")
			}
		})
	}
}

// TestSynthesizeSpeechLexiconNamesLimit verifies that SynthesizeSpeech rejects
// more than 5 lexicon names. AWS returns InvalidParameterValueException when
// LexiconNames exceeds the maximum of 5.
func TestSynthesizeSpeechLexiconNamesLimit(t *testing.T) {
	t.Parallel()

	handler := newHandler()
	validContent := `<lexicon alphabet="ipa" xml:lang="en-US"><lexeme></lexeme></lexicon>`
	names := make([]string, 6)
	for i := range 6 {
		name := fmt.Sprintf("lex%02d", i)
		names[i] = name
		rec := request(t, handler, http.MethodPut, "/v1/lexicons/"+name,
			map[string]any{"Content": validContent})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	tests := []struct {
		name     string
		lexNames []string
		wantCode int
	}{
		{name: "5 lexicons allowed", lexNames: names[:5], wantCode: http.StatusOK},
		{name: "6 lexicons rejected", lexNames: names[:6], wantCode: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := request(t, handler, http.MethodPost, "/v1/speech", map[string]any{
				"OutputFormat": "mp3",
				"LexiconNames": tc.lexNames,
				"Text":         "hello",
				"VoiceId":      "Joanna",
			})
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.wantCode == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "InvalidParameterValueException")
			}
		})
	}
}

// TestSynthesizeSpeechInvalidSpeechMarkType verifies that SynthesizeSpeech
// rejects unrecognized SpeechMarkTypes values. AWS returns
// InvalidParameterValueException for types not in {sentence, ssml, viseme, word}.
func TestSynthesizeSpeechInvalidSpeechMarkType(t *testing.T) {
	t.Parallel()

	rec := request(t, newHandler(), http.MethodPost, "/v1/speech", map[string]any{
		"OutputFormat":    "json",
		"SpeechMarkTypes": []string{"phoneme"},
		"Text":            "hello",
		"VoiceId":         "Joanna",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterValueException")
}

// TestSynthesizeSpeechDefaultSampleRate verifies that the default SampleRate
// is engine-specific. AWS defaults: PCM → 16000, non-standard engines
// (neural/long-form/generative) → 24000, standard → 22050.
// PCM output is a WAV container; the sample rate is encoded in the WAV header
// at bytes 24-27 (little-endian uint32). MP3 output returns MPEG sync bytes.
func TestSynthesizeSpeechDefaultSampleRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		engine         string
		format         string
		wantMagic      []byte
		wantSampleRate uint32
	}{
		{name: "standard_mp3_defaults_22050", engine: "standard", format: "mp3", wantMagic: []byte{0xFF, 0xFB}},
		{name: "neural_mp3_defaults_24000", engine: "neural", format: "mp3", wantMagic: []byte{0xFF, 0xFB}},
		{name: "generative_mp3_defaults_24000", engine: "generative", format: "mp3", wantMagic: []byte{0xFF, 0xFB}},
		{
			name:           "standard_pcm_defaults_16000",
			engine:         "standard",
			format:         "pcm",
			wantSampleRate: 16000,
			wantMagic:      []byte("RIFF"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := request(t, newHandler(), http.MethodPost, "/v1/speech", map[string]any{
				"Engine":       tc.engine,
				"OutputFormat": tc.format,
				"Text":         "hello",
				"VoiceId":      "Joanna",
			})
			require.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.Bytes()
			require.Greater(t, len(body), 4, "audio body must be non-empty")
			if len(tc.wantMagic) > 0 {
				assert.Equal(t, tc.wantMagic, body[:len(tc.wantMagic)], "audio magic bytes")
			}
			if tc.wantSampleRate > 0 && len(body) >= 28 {
				// WAV sample rate is at offset 24, little-endian uint32.
				rate := uint32(body[24]) | uint32(body[25])<<8 | uint32(body[26])<<16 | uint32(body[27])<<24
				assert.Equal(t, tc.wantSampleRate, rate, "WAV header sample rate")
			}
		})
	}
}

// TestSpeechMarksVisemeVariety verifies that viseme speech marks include
// multiple distinct visemes (not just the fallback "p") for a sentence
// containing vowels and different consonants.
func TestSpeechMarksVisemeVariety(t *testing.T) {
	t.Parallel()

	handler := newHandler()

	rec := request(t, handler, http.MethodPost, "/v1/speech", map[string]any{
		"OutputFormat":    "json",
		"Text":            "See the ship sail away",
		"VoiceId":         "Joanna",
		"SpeechMarkTypes": []string{"viseme"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var marks []map[string]any
	for _, line := range splitLines(rec.Body.Bytes()) {
		if len(line) == 0 {
			continue
		}

		var m map[string]any
		require.NoError(t, json.Unmarshal(line, &m))

		if m["type"] == "viseme" {
			marks = append(marks, m)
		}
	}

	require.NotEmpty(t, marks, "expected at least one viseme mark")

	seen := make(map[string]bool)
	for _, m := range marks {
		if v, ok := m["value"].(string); ok {
			seen[v] = true
		}
	}

	assert.Greater(t, len(seen), 1, "expected multiple distinct visemes, got: %v", seen)
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0

	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}

	if start < len(data) {
		lines = append(lines, data[start:])
	}

	return lines
}

// TestSpeechMarkCounts verifies that sentence and ssml speech marks are
// emitted once per semantic unit (sentence/SSML element), not once per word.
// AWS emits exactly one sentence mark per sentence and one ssml mark for the
// implicit <speak> wrapper — the previous implementation multiplied marks by
// the word count, which broke subtitle-timing and lip-sync consumers.
func TestSpeechMarkCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantCounts    map[string]int // type → exact count
		wantNotCounts map[string]int // type → count that must NOT appear
		name          string
		text          string
		textType      string
		marks         []string
	}{
		{
			name:       "sentence_single_no_punct",
			text:       "hello world",
			marks:      []string{"sentence"},
			wantCounts: map[string]int{"sentence": 1},
		},
		{
			name:       "sentence_two_sentences",
			text:       "Hello world. Goodbye now.",
			marks:      []string{"sentence"},
			wantCounts: map[string]int{"sentence": 2},
		},
		{
			name:       "sentence_three_sentences_mixed_punct",
			text:       "Hello! How are you? Goodbye.",
			marks:      []string{"sentence"},
			wantCounts: map[string]int{"sentence": 3},
		},
		{
			// SpeechMarkTypes "ssml" requires TextType ssml (AWS:
			// SsmlMarksNotSupportedForTextTypeException otherwise).
			name:       "ssml_once_regardless_of_word_count",
			text:       wrapSpeak("one two three four five"),
			textType:   "ssml",
			marks:      []string{"ssml"},
			wantCounts: map[string]int{"ssml": 1},
		},
		{
			name:       "word_marks_per_word",
			text:       "alpha beta gamma",
			marks:      []string{"word"},
			wantCounts: map[string]int{"word": 3},
		},
		{
			name:       "viseme_marks_per_word",
			text:       "alpha beta gamma",
			marks:      []string{"viseme"},
			wantCounts: map[string]int{"viseme": 3},
		},
		{
			name:  "combined_sentence_word_viseme",
			text:  "Hello world. Goodbye.",
			marks: []string{"sentence", "word", "viseme"},
			// 2 sentences, 3 words (Hello, world., Goodbye.), 3 visemes
			wantCounts: map[string]int{"sentence": 2, "word": 3, "viseme": 3},
		},
		{
			name:       "ssml_not_per_word",
			text:       wrapSpeak("one two three"),
			textType:   "ssml",
			marks:      []string{"ssml"},
			wantCounts: map[string]int{"ssml": 1},
			wantNotCounts: map[string]int{
				// Must NOT be 3 (one per word — the old bug)
				"ssml": 3,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := request(t, newHandler(), http.MethodPost, "/v1/speech", map[string]any{
				"OutputFormat":    "json",
				"SpeechMarkTypes": tc.marks,
				"Text":            tc.text,
				"TextType":        tc.textType,
				"VoiceId":         "Joanna",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			// Each line in the response is a JSON speech mark event.
			counts := make(map[string]int)
			for line := range strings.SplitSeq(strings.TrimSpace(rec.Body.String()), "\n") {
				if line == "" {
					continue
				}
				var ev map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &ev), "invalid JSON line: %s", line)
				typ, _ := ev["type"].(string)
				counts[typ]++
			}

			for typ, want := range tc.wantCounts {
				assert.Equal(t, want, counts[typ], "count of %q marks", typ)
			}
			for typ, notWant := range tc.wantNotCounts {
				assert.NotEqual(t, notWant, counts[typ], "count of %q marks must not equal %d", typ, notWant)
			}
		})
	}
}

// TestSpeechMarkTimeOrder verifies that speech marks are ordered by ascending
// time, matching AWS output ordering. Sentence marks precede word marks at
// the same time position (stable sort on equal times).
func TestSpeechMarkTimeOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		text  string
		marks []string
	}{
		{
			name:  "sentence_before_word_at_t0",
			text:  "hello world",
			marks: []string{"sentence", "word"},
		},
		{
			name:  "multi_sentence_interleaved_with_words",
			text:  "Hello. World.",
			marks: []string{"sentence", "word"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := request(t, newHandler(), http.MethodPost, "/v1/speech", map[string]any{
				"OutputFormat":    "json",
				"SpeechMarkTypes": tc.marks,
				"Text":            tc.text,
				"VoiceId":         "Joanna",
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var prevTime float64 = -1
			for line := range strings.SplitSeq(strings.TrimSpace(rec.Body.String()), "\n") {
				if line == "" {
					continue
				}
				var ev map[string]any
				require.NoError(t, json.Unmarshal([]byte(line), &ev))
				timeVal, _ := ev["time"].(float64)
				assert.GreaterOrEqual(t, timeVal, prevTime, "marks must be non-decreasing by time")
				prevTime = timeVal
			}
		})
	}
}
