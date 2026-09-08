package polly

import (
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

// SynthesizeSpeech validates options and returns deterministic audio or speech-mark output.
func (b *InMemoryBackend) SynthesizeSpeech(options SynthesisOptions) (*SynthesizedSpeech, error) {
	limit := maxSpeechTextLen
	if options.TextType == textTypeSSML {
		limit = maxSpeechSSMLLen
	}
	if len(options.Text) > limit {
		return nil, fmt.Errorf("%w: text exceeds maximum length of %d characters", ErrTextLengthExceeded, limit)
	}

	normal, err := b.validateOptions(options, false)
	if err != nil {
		return nil, err
	}

	if normal.OutputFormat == outputFormatJSON {
		return &SynthesizedSpeech{
			Data:              speechMarks(normal),
			ContentType:       "application/x-json-stream",
			RequestCharacters: len(normal.Text),
		}, nil
	}

	data := syntheticAudioBytes(normal)

	return &SynthesizedSpeech{
		Data:              data,
		ContentType:       contentTypeForFormat(normal.OutputFormat),
		RequestCharacters: len(normal.Text),
	}, nil
}

func (b *InMemoryBackend) validateOptions(options SynthesisOptions, forTask bool) (SynthesisOptions, error) {
	options = defaultOptions(options)
	if options.Text == "" || options.VoiceID == "" {
		return options, fmt.Errorf("%w: Text and VoiceId are required", ErrValidation)
	}
	if len(options.LexiconNames) > maxLexiconNames {
		return options, fmt.Errorf("%w: LexiconNames must not exceed %d entries", ErrValidation, maxLexiconNames)
	}
	if !slices.Contains(validEngines(), options.Engine) {
		return options, fmt.Errorf("%w: invalid Engine %q", ErrValidation, options.Engine)
	}
	if !slices.Contains(validOutputFormats(), options.OutputFormat) {
		return options, fmt.Errorf("%w: invalid OutputFormat %q", ErrValidation, options.OutputFormat)
	}
	if !slices.Contains(validTextTypes(), options.TextType) {
		return options, fmt.Errorf("%w: invalid TextType %q", ErrValidation, options.TextType)
	}
	if !validSampleRate(options.OutputFormat, options.SampleRate, forTask) {
		return options, fmt.Errorf(
			"%w: invalid SampleRate %q for %s",
			ErrInvalidSampleRate,
			options.SampleRate,
			options.OutputFormat,
		)
	}
	if err := validateSpeechMarks(options); err != nil {
		return options, err
	}
	if err := validateSSML(options.TextType, options.Text); err != nil {
		return options, err
	}

	b.mu.RLock("validateSynthesisOptions")
	defer b.mu.RUnlock()

	if err := b.checkVoiceSupport(options.VoiceID, options.Engine, options.LanguageCode); err != nil {
		return options, err
	}
	for _, name := range options.LexiconNames {
		if !b.lexicons.Has(name) {
			return options, fmt.Errorf("%w: lexicon %q", ErrLexiconNotFound, name)
		}
	}

	return options, nil
}

// checkVoiceSupport validates that voice id can render the requested engine and
// language, returning the AWS-accurate exception for the first constraint that
// fails: an unrecognized VoiceId, an engine the voice doesn't support
// (EngineNotSupportedException), or a language/dialect the voice doesn't speak
// (LanguageNotSupportedException).
func (b *InMemoryBackend) checkVoiceSupport(id, engine, languageCode string) error {
	for _, voice := range b.voices {
		if voice.ID != id {
			continue
		}
		if !slices.Contains(voice.SupportedEngines, engine) {
			return fmt.Errorf("%w: voice %q does not support engine %q", ErrEngineNotSupported, id, engine)
		}
		if languageCode == "" || voice.LanguageCode == languageCode ||
			slices.Contains(voice.AdditionalLanguageCodes, languageCode) {
			return nil
		}

		return fmt.Errorf("%w: voice %q does not support language %q", ErrLanguageNotSupported, id, languageCode)
	}

	return fmt.Errorf("%w: unknown VoiceId %q", ErrValidation, id)
}

func defaultOptions(options SynthesisOptions) SynthesisOptions {
	if options.Engine == "" {
		options.Engine = defaultEngine
	}
	if options.TextType == "" {
		options.TextType = textTypeText
	}
	if options.OutputFormat == "" {
		options.OutputFormat = outputFormatMP3
	}
	if options.SampleRate == "" {
		switch {
		case options.OutputFormat == outputFormatOggOpus:
			options.SampleRate = "48000"
		case options.OutputFormat == outputFormatMulaw || options.OutputFormat == outputFormatAlaw:
			options.SampleRate = "8000"
		case options.OutputFormat == outputFormatPCM:
			options.SampleRate = defaultSampleRatePCM
		case options.Engine != defaultEngine:
			options.SampleRate = "24000"
		default:
			options.SampleRate = defaultSampleRateMP3
		}
	}

	return options
}

func validateSpeechMarks(options SynthesisOptions) error {
	for _, speechMark := range options.SpeechMarkTypes {
		if !slices.Contains(validSpeechMarkTypes(), speechMark) {
			return fmt.Errorf("%w: invalid SpeechMarkType %q", ErrValidation, speechMark)
		}
		// AWS: "SSML speech marks are not supported for plain text-type input."
		if speechMark == textTypeSSML && options.TextType != textTypeSSML {
			return fmt.Errorf(
				"%w: SpeechMarkTypes ssml requires TextType ssml", ErrSsmlMarksNotSupportedForTextType,
			)
		}
	}
	if len(options.SpeechMarkTypes) > 0 && options.OutputFormat != outputFormatJSON {
		return fmt.Errorf("%w: speech marks require json OutputFormat", ErrMarksNotSupportedForFormat)
	}
	if len(options.SpeechMarkTypes) == 0 && options.OutputFormat == outputFormatJSON {
		return fmt.Errorf("%w: json OutputFormat requires SpeechMarkTypes", ErrValidation)
	}

	return nil
}

// validateSSML checks that text is well-formed XML wrapped in a <speak> root
// element when textType is "ssml" -- AWS rejects malformed or unwrapped SSML
// with InvalidSsmlException. Plain-text input (textType != "ssml") is never
// checked here.
func validateSSML(textType, text string) error {
	if textType != textTypeSSML {
		return nil
	}

	decoder := xml.NewDecoder(strings.NewReader(text))
	depth, root := 0, ""
	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidSsml, err)
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				root = elem.Name.Local
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}

	if depth != 0 {
		return fmt.Errorf("%w: unbalanced SSML element tags", ErrInvalidSsml)
	}
	if root != "speak" {
		return fmt.Errorf("%w: SSML text must be wrapped in a <speak> root element", ErrInvalidSsml)
	}

	return nil
}

// validSampleRate reports whether rate is a documented value for format.
// mp3/ogg_vorbis differ by operation: SynthesizeSpeech's doc comment lists
// "8000, 16000, 22050, 24000, 44100 and 48000" but
// StartSpeechSynthesisTask's lists only "8000, 16000, 22050, and 24000" --
// both api_op_*.go SampleRate field docs, aws-sdk-go-v2/service/polly@v1.60.4.
// Every other format's valid set is identical across both operations.
func validSampleRate(format, rate string, forTask bool) bool {
	rates := map[string][]string{
		outputFormatMP3:     {"8000", "16000", "22050", "24000", "44100", "48000"},
		outputFormatOGG:     {"8000", "16000", "22050", "24000", "44100", "48000"},
		outputFormatOggOpus: {"48000"},
		outputFormatPCM:     {"8000", "16000"},
		outputFormatMulaw:   {"8000"},
		outputFormatAlaw:    {"8000"},
		outputFormatJSON:    {"8000", "16000", "22050", "24000"},
	}
	if forTask && (format == outputFormatMP3 || format == outputFormatOGG) {
		return slices.Contains([]string{"8000", "16000", "22050", "24000"}, rate)
	}

	return slices.Contains(rates[format], rate)
}

func contentTypeForFormat(format string) string {
	contentTypes := map[string]string{
		outputFormatMP3:     "audio/mpeg",
		outputFormatOGG:     "audio/ogg",
		outputFormatOggOpus: "audio/ogg",
		outputFormatPCM:     "audio/pcm",
		outputFormatMulaw:   "audio/mulaw",
		outputFormatAlaw:    "audio/alaw",
	}

	return contentTypes[format]
}

// speechMarkItem is a timed speech mark entry used when building speech mark output.
type speechMarkItem struct {
	line string
	time int
}

// buildSentenceMarks returns one speechMarkItem per sentence in text, splitting
// on '.', '!' and '?' delimiters. Text with no sentence-ending punctuation is
// treated as a single sentence, matching AWS Polly behaviour.
func buildSentenceMarks(text string) []speechMarkItem {
	if text == "" {
		return nil
	}
	var out []speechMarkItem
	start := 0
	for i := range len(text) {
		ch := text[i]
		if ch != '.' && ch != '!' && ch != '?' {
			continue
		}
		end := i + 1
		if sentence := strings.TrimSpace(text[start:end]); sentence != "" {
			t := start * msPerCharacter
			out = append(out, speechMarkItem{
				time: t,
				line: fmt.Sprintf(`{"time":%d,"type":"sentence","start":%d,"end":%d,"value":%q}`,
					t, start, end, sentence),
			})
		}
		start = end
		for start < len(text) && (text[start] == ' ' || text[start] == '\n' ||
			text[start] == '\r' || text[start] == '\t') {
			start++
		}
	}
	if sentence := strings.TrimSpace(text[start:]); sentence != "" {
		t := start * msPerCharacter
		out = append(out, speechMarkItem{
			time: t,
			line: fmt.Sprintf(`{"time":%d,"type":"sentence","start":%d,"end":%d,"value":%q}`,
				t, start, len(text), sentence),
		})
	}

	return out
}

func speechMarks(options SynthesisOptions) []byte {
	var marks []speechMarkItem

	for _, typ := range options.SpeechMarkTypes {
		switch typ {
		case "sentence":
			marks = append(marks, buildSentenceMarks(options.Text)...)
		case textTypeSSML:
			marks = append(marks, speechMarkItem{
				time: 0,
				line: fmt.Sprintf(`{"time":0,"type":"ssml","start":0,"end":%d,"value":"<speak>"}`, len(options.Text)),
			})
		}
	}

	needWord := slices.Contains(options.SpeechMarkTypes, "word")
	needViseme := slices.Contains(options.SpeechMarkTypes, "viseme")
	if needWord || needViseme {
		offset := 0
		for word := range strings.FieldsSeq(options.Text) {
			start := strings.Index(options.Text[offset:], word) + offset
			end := start + len(word)
			timeMs := start * msPerCharacter
			if needWord {
				marks = append(marks, speechMarkItem{
					time: timeMs,
					line: fmt.Sprintf(`{"time":%d,"type":"word","start":%d,"end":%d,"value":%q}`,
						timeMs, start, end, word),
				})
			}
			if needViseme {
				marks = append(marks, speechMarkItem{
					time: timeMs,
					line: fmt.Sprintf(`{"time":%d,"type":"viseme","value":%q}`, timeMs, wordFirstViseme(word)),
				})
			}
			offset = end
		}
	}

	// Stable sort by time so sentence marks precede word marks at equal time positions.
	sort.SliceStable(marks, func(i, j int) bool { return marks[i].time < marks[j].time })

	lines := make([]string, 0, len(marks))
	for _, m := range marks {
		lines = append(lines, m.line)
	}

	if len(lines) == 0 {
		for _, typ := range options.SpeechMarkTypes {
			lines = append(lines, fmt.Sprintf(`{"time":0,"type":"%s","start":0,"end":%d,"value":%q}`,
				typ, len(options.Text), options.Text))
		}
	}

	return []byte(strings.Join(lines, "\n") + "\n")
}

// syntheticAudioBytes returns minimal but format-correct audio bytes for the given output format.
// PCM → RIFF/WAV container with one silent frame.
// MP3 → minimal MPEG-1 Layer 3 sync frame header.
// OGG/ogg_opus → OGG capture pattern + minimal Vorbis identification.
// mulaw/alaw → a few headerless companded silence samples.
func syntheticAudioBytes(opts SynthesisOptions) []byte {
	switch opts.OutputFormat {
	case outputFormatPCM:
		return minimalWAV(opts.SampleRate)
	case outputFormatMP3:
		return minimalMP3Frame()
	case outputFormatOGG, outputFormatOggOpus:
		return minimalOGG()
	case outputFormatMulaw, outputFormatAlaw:
		return minimalCompandedAudio(opts.OutputFormat)
	default:
		return minimalWAV(opts.SampleRate)
	}
}

// minimalCompandedAudio returns a few silent samples for headerless companded
// (mu-law/a-law) audio: AWS returns raw audio/mulaw or audio/alaw bytes with no
// RIFF/WAV container, unlike this backend's pcm output.
func minimalCompandedAudio(format string) []byte {
	silence := byte(mulawSilenceByte)
	if format == outputFormatAlaw {
		silence = alawSilenceByte
	}

	out := make([]byte, compandedSampleLen)
	for i := range out {
		out[i] = silence
	}

	return out
}

// minimalWAV returns a 46-byte RIFF/WAV file with two silent PCM samples.
func minimalWAV(sampleRateStr string) []byte {
	sampleRate := uint32(defaultWAVSampleRate)
	switch sampleRateStr {
	case "8000":
		sampleRate = 8000
	case "16000":
		sampleRate = 16000
	case "24000":
		sampleRate = 24000
	case "44100":
		sampleRate = 44100
	case "48000":
		sampleRate = 48000
	}
	const numChannels = uint16(1)
	const bitsPerSample = uint16(wavPCMChunkSize)
	const dataLen = uint32(wavSilentDataLen) // 2 silent 16-bit samples
	byteRate := sampleRate * uint32(numChannels) * uint32(bitsPerSample) / wavBitsPerByte
	blockAlign := numChannels * bitsPerSample / wavBitsPerByte
	fileSize := wavHeaderMinusRIFF + dataLen

	buf := make([]byte, 0, wavHeaderSize+dataLen)
	buf = append(buf, 'R', 'I', 'F', 'F')
	buf = binary.LittleEndian.AppendUint32(buf, fileSize)
	buf = append(buf, 'W', 'A', 'V', 'E')
	buf = append(buf, 'f', 'm', 't', ' ')
	buf = binary.LittleEndian.AppendUint32(buf, wavPCMChunkSize) // PCM chunk size
	buf = binary.LittleEndian.AppendUint16(buf, 1)               // PCM format
	buf = binary.LittleEndian.AppendUint16(buf, numChannels)
	buf = binary.LittleEndian.AppendUint32(buf, sampleRate)
	buf = binary.LittleEndian.AppendUint32(buf, byteRate)
	buf = binary.LittleEndian.AppendUint16(buf, blockAlign)
	buf = binary.LittleEndian.AppendUint16(buf, bitsPerSample)
	buf = append(buf, 'd', 'a', 't', 'a')
	buf = binary.LittleEndian.AppendUint32(buf, dataLen)
	buf = append(buf, 0, 0, 0, 0) // two silent 16-bit samples

	return buf
}

// minimalMP3Frame returns a minimal valid MPEG-1 Layer 3 frame header (silent frame).
// Frame sync: 0xFFE0 | layer(01) | bitrate(1001=128k) | samplerate(00=44100) | padding(0) | stereo(00).
func minimalMP3Frame() []byte {
	minimalMP3FrameBytes := []byte{
		0xFF, 0xFB, 0x90, 0x00, // sync + MPEG1 Layer3 128kbps 44100Hz stereo no-padding
		// 417 bytes of silence (128kbps frame at 44100 is 417 bytes)
	}
	const mp3FrameTotalLen = 417 // 128kbps frame at 44100Hz: 4-byte header + 413 bytes silence
	frame := make([]byte, mp3FrameTotalLen)
	copy(frame, minimalMP3FrameBytes)

	return frame
}

// minimalOGG returns the OGG capture pattern + minimal Vorbis identification page.
func minimalOGG() []byte {
	// OGG page header magic: "OggS" capture pattern
	return []byte{
		'O', 'g', 'g', 'S', // capture pattern
		0x00,                   // version
		0x02,                   // header type: beginning of stream
		0, 0, 0, 0, 0, 0, 0, 0, // granule position
		1, 0, 0, 0, // serial number
		0, 0, 0, 0, // page sequence
		0, 0, 0, 0, // checksum placeholder
		1,    // number of segments
		0x1E, // segment table: 30-byte vorbis id header
		// Vorbis identification header (minimal)
		0x01,                         // packet type: identification
		'v', 'o', 'r', 'b', 'i', 's', // codec id
		0, 0, 0, 0, // version
		0x01,                   // channels
		0x44, 0xAC, 0x00, 0x00, // sample rate 44100
		0, 0, 0, 0, // max bitrate
		0, 0, 0, 0, // nominal bitrate
		0, 0, 0, 0, // min bitrate
		0x01, // blocksize
	}
}

func validEngines() []string {
	return []string{defaultEngine, engineNeural, engineLongForm, engineGenerative}
}

func validOutputFormats() []string {
	return []string{
		outputFormatMP3, outputFormatOGG, outputFormatOggOpus,
		outputFormatPCM, outputFormatMulaw, outputFormatAlaw, outputFormatJSON,
	}
}

func validTextTypes() []string { return []string{textTypeText, textTypeSSML} }

func validSpeechMarkTypes() []string { return []string{"sentence", textTypeSSML, "viseme", "word"} }

// wordFirstViseme returns an approximate AWS Polly viseme for the first character of word.
// AWS Polly visemes: p b m → "p", f v → "f", t d → "t", s z → "s",
// k g c q x → "k", n → "n", l → "l", r → "r", vowels → "@", etc.
func wordFirstViseme(word string) string {
	if word == "" {
		return "sil"
	}

	ch := word[0]
	if ch >= 'A' && ch <= 'Z' {
		ch += 32
	}

	if v := wordFirstVisemeVowelOrSibilant(ch); v != "" {
		return v
	}

	return wordFirstVisemeConsonant(ch)
}

func wordFirstVisemeVowelOrSibilant(ch byte) string {
	switch ch {
	case 'a', 'e', 'i', 'o', 'u', 'h':
		return "@"
	case 'p', 'b', 'm':
		return "p"
	case 'f', 'v':
		return "f"
	case 't', 'd':
		return "t"
	case 's', 'z':
		return "s"
	case 'j':
		return "S"
	}

	return ""
}

func wordFirstVisemeConsonant(ch byte) string {
	switch ch {
	case 'k', 'g', 'c', 'q', 'x':
		return "k"
	case 'n':
		return "n"
	case 'l':
		return "l"
	case 'r':
		return "r"
	case 'w', 'y':
		return "u"
	}

	return "p"
}
