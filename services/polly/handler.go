package polly

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	pollyPathPrefix = "/v1/"
	pollyPriority   = service.PriorityPathVersioned
)

const (
	opDeleteLexicon              = "DeleteLexicon"
	opDescribeVoices             = "DescribeVoices"
	opGetLexicon                 = "GetLexicon"
	opGetSpeechSynthesisTask     = "GetSpeechSynthesisTask"
	opListLexicons               = "ListLexicons"
	opListSpeechSynthesisTasks   = "ListSpeechSynthesisTasks"
	opPutLexicon                 = "PutLexicon"
	opStartSpeechSynthesisStream = "StartSpeechSynthesisStream"
	opStartSpeechSynthesisTask   = "StartSpeechSynthesisTask"
	opSynthesizeSpeech           = "SynthesizeSpeech"
	opUnknown                    = "Unknown"
	queryEngine                  = "Engine"
	queryLanguageCode            = "LanguageCode"
	queryGender                  = "Gender"
	queryIncludeAdditional       = "IncludeAdditionalLanguageCodes"
	queryStatus                  = "Status"
	queryNextToken               = "NextToken"
	queryMaxResults              = "MaxResults"
	headerRequestCharacters      = "X-Amzn-Requestcharacters"
	headerStreamEngine           = "X-Amzn-Engine"
	headerStreamLanguageCode     = "X-Amzn-Languagecode"
	headerStreamLexiconNames     = "X-Amzn-Lexiconnames"
	headerStreamOutputFormat     = "X-Amzn-Outputformat"
	headerStreamSampleRate       = "X-Amzn-Samplerate"
	headerStreamVoiceID          = "X-Amzn-Voiceid"
	eventTypeHeader              = ":event-type"
	messageTypeHeader            = ":message-type"
	contentTypeHeader            = ":content-type"
	eventMessageType             = "event"
	millisecondsPerSecond        = 1000
)

// Handler implements Amazon Polly REST JSON operations.
type Handler struct {
	Backend *InMemoryBackend
}

// NewHandler creates handler for backend.
func NewHandler(backend *InMemoryBackend) *Handler { return &Handler{Backend: backend} }

// Reset clears backend state.
func (h *Handler) Reset() { h.Backend.Reset() }

// Name returns service name.
func (h *Handler) Name() string { return "Polly" }

// GetSupportedOperations returns implemented SDK operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		opDeleteLexicon,
		opDescribeVoices,
		opGetLexicon,
		opGetSpeechSynthesisTask,
		opListLexicons,
		opListSpeechSynthesisTasks,
		opPutLexicon,
		opStartSpeechSynthesisStream,
		opStartSpeechSynthesisTask,
		opSynthesizeSpeech,
	}
}

// ChaosServiceName returns service key.
func (h *Handler) ChaosServiceName() string { return "polly" }

// ChaosOperations returns injectable operation list.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns backend region.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher matches versioned Polly routes.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		request := c.Request()

		return strings.HasPrefix(request.URL.Path, pollyPathPrefix) &&
			parseRoute(request.Method, request.URL.Path).operation != opUnknown
	}
}

// MatchPriority returns REST service routing priority.
func (h *Handler) MatchPriority() int { return pollyPriority }

// ExtractOperation derives operation from request method and path.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	return parseRoute(c.Request().Method, c.Request().URL.Path).operation
}

// ExtractResource derives task ID, lexicon name, or ARN from request path.
func (h *Handler) ExtractResource(c *echo.Context) string {
	return parseRoute(c.Request().Method, c.Request().URL.Path).resource
}

// Handler returns request dispatcher.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		route := parseRoute(c.Request().Method, c.Request().URL.Path)
		if route.operation == opUnknown {
			return writeError(c, http.StatusNotFound, opUnknown, "unknown Polly route")
		}

		err := h.dispatch(c, route)
		if err == nil {
			return nil
		}

		return h.writeBackendError(c, err)
	}
}

type route struct {
	operation string
	resource  string
}

func parseRoute(method, path string) route {
	exactRoutes := map[string]map[string]string{
		"/v1/speech":          {http.MethodPost: opSynthesizeSpeech},
		"/v1/synthesisStream": {http.MethodPost: opStartSpeechSynthesisStream},
		"/v1/synthesisTasks": {
			http.MethodPost: opStartSpeechSynthesisTask,
			http.MethodGet:  opListSpeechSynthesisTasks,
		},
		"/v1/voices":   {http.MethodGet: opDescribeVoices},
		"/v1/lexicons": {http.MethodGet: opListLexicons},
	}
	if byMethod, ok := exactRoutes[path]; ok {
		if operation, found := byMethod[method]; found {
			return route{operation: operation}
		}
	}

	resourceRoutes := []struct {
		operations map[string]string
		prefix     string
	}{
		{prefix: "/v1/synthesisTasks/", operations: map[string]string{http.MethodGet: opGetSpeechSynthesisTask}},
		{
			prefix: "/v1/lexicons/",
			operations: map[string]string{
				http.MethodPut: opPutLexicon, http.MethodGet: opGetLexicon, http.MethodDelete: opDeleteLexicon,
			},
		},
	}
	for _, configured := range resourceRoutes {
		if strings.HasPrefix(path, configured.prefix) {
			if operation, ok := configured.operations[method]; ok {
				return route{operation: operation, resource: suffix(path, configured.prefix)}
			}
		}
	}

	return route{operation: opUnknown}
}

func suffix(path, prefix string) string {
	value, err := url.PathUnescape(strings.TrimPrefix(path, prefix))
	if err != nil {
		return ""
	}

	return value
}

func (h *Handler) dispatch(c *echo.Context, r route) error {
	switch r.operation {
	case opSynthesizeSpeech:
		return h.synthesizeSpeech(c)
	case opStartSpeechSynthesisStream:
		return h.startSpeechSynthesisStream(c)
	case opStartSpeechSynthesisTask:
		return h.startTask(c)
	case opGetSpeechSynthesisTask:
		return h.getTask(c, r.resource)
	case opListSpeechSynthesisTasks:
		return h.listTasks(c)
	case opPutLexicon:
		return h.putLexicon(c, r.resource)
	case opGetLexicon:
		return h.getLexicon(c, r.resource)
	case opDeleteLexicon:
		return h.deleteLexicon(c, r.resource)
	case opListLexicons:
		return h.listLexicons(c)
	case opDescribeVoices:
		return h.describeVoices(c)
	default:
		return fmt.Errorf("%w: unknown operation", ErrValidation)
	}
}

type synthesisInput struct {
	Engine          string   `json:"Engine"`
	LanguageCode    string   `json:"LanguageCode"`
	OutputFormat    string   `json:"OutputFormat"`
	SampleRate      string   `json:"SampleRate"`
	Text            string   `json:"Text"`
	TextType        string   `json:"TextType"`
	VoiceID         string   `json:"VoiceId"`
	LexiconNames    []string `json:"LexiconNames"`
	SpeechMarkTypes []string `json:"SpeechMarkTypes"`
}

func (in synthesisInput) options() SynthesisOptions {
	return SynthesisOptions(in)
}

func (h *Handler) synthesizeSpeech(c *echo.Context) error {
	var in synthesisInput
	if err := decodeRequest(c, &in); err != nil {
		return err
	}

	result, err := h.Backend.SynthesizeSpeech(in.options())
	if err != nil {
		return err
	}

	c.Response().Header().Set("Content-Type", result.ContentType)
	c.Response().Header().Set(headerRequestCharacters, strconv.Itoa(result.RequestCharacters))

	return c.Blob(http.StatusOK, result.ContentType, result.Data)
}

type streamTextEvent struct {
	Text     string `json:"Text"`
	TextType string `json:"TextType"`
}

func (h *Handler) startSpeechSynthesisStream(c *echo.Context) error {
	options := SynthesisOptions{
		Engine:       c.Request().Header.Get(headerStreamEngine),
		LanguageCode: c.Request().Header.Get(headerStreamLanguageCode),
		LexiconNames: splitHeader(c.Request().Header.Values(headerStreamLexiconNames)),
		OutputFormat: c.Request().Header.Get(headerStreamOutputFormat),
		SampleRate:   c.Request().Header.Get(headerStreamSampleRate),
		VoiceID:      c.Request().Header.Get(headerStreamVoiceID),
	}

	// AWS: "Currently, only the generative engine is supported" for this op
	// (api_op_StartSpeechSynthesisStream.go's Engine doc comment) -- unlike
	// SynthesizeSpeech/StartSpeechSynthesisTask, which accept all 4 Engine
	// values and default an unset one to "standard".
	if options.Engine != engineGenerative {
		return fmt.Errorf("%w: engine %q is not supported; StartSpeechSynthesisStream requires generative",
			ErrStreamValidation, options.Engine)
	}

	text, textType, err := decodeStreamText(c.Request().Body)
	if err != nil {
		return fmt.Errorf("%w: invalid synthesis stream: %w", ErrStreamValidation, err)
	}
	options.Text = text
	options.TextType = textType

	result, err := h.Backend.SynthesizeSpeech(options)
	if err != nil {
		// AWS models StartSpeechSynthesisStream's client errors as the generic
		// ValidationException, not SynthesizeSpeech's op-specific exceptions --
		// see ErrStreamValidation's doc comment. writeBackendError checks
		// ErrStreamValidation before any of the more specific sentinels this
		// wraps, so this remapping wins regardless of which validation failed.
		return fmt.Errorf("%w: %w", ErrStreamValidation, err)
	}

	var stream bytes.Buffer
	encoder := eventstream.NewEncoder()
	if encodeErr := encodeEvent(encoder, &stream, "AudioEvent", result.Data); encodeErr != nil {
		return encodeErr
	}

	closed, err := json.Marshal(map[string]int{"RequestCharacters": result.RequestCharacters})
	if err != nil {
		return err
	}
	if encodeErr := encodeEvent(encoder, &stream, "StreamClosedEvent", closed); encodeErr != nil {
		return encodeErr
	}

	return c.Blob(http.StatusOK, "application/vnd.amazon.eventstream", stream.Bytes())
}

func splitHeader(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		for name := range strings.SplitSeq(value, ",") {
			if strings.TrimSpace(name) != "" {
				out = append(out, strings.Trim(strings.TrimSpace(name), `"`))
			}
		}
	}

	return out
}

func decodeStreamText(body io.Reader) (string, string, error) {
	decoder := eventstream.NewDecoder()
	textType := textTypeText
	var texts []string

	for {
		message, err := decoder.Decode(body, nil)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", "", err
		}

		eventType := message.Headers.Get(eventTypeHeader)
		if eventType == nil || eventType.String() != "TextEvent" {
			continue
		}

		var event streamTextEvent
		if jsonErr := json.Unmarshal(message.Payload, &event); jsonErr != nil {
			return "", "", jsonErr
		}
		texts = append(texts, event.Text)
		if event.TextType != "" {
			textType = event.TextType
		}
	}

	return strings.Join(texts, ""), textType, nil
}

func encodeEvent(encoder *eventstream.Encoder, out io.Writer, eventType string, payload []byte) error {
	message := eventstream.Message{Payload: payload}
	message.Headers.Set(eventTypeHeader, eventstream.StringValue(eventType))
	message.Headers.Set(messageTypeHeader, eventstream.StringValue(eventMessageType))
	if eventType == "StreamClosedEvent" {
		message.Headers.Set(contentTypeHeader, eventstream.StringValue("application/json"))
	}

	return encoder.Encode(out, message)
}

type startTaskInput struct {
	OutputS3BucketName string `json:"OutputS3BucketName"`
	OutputS3KeyPrefix  string `json:"OutputS3KeyPrefix"`
	SNSTopicArn        string `json:"SnsTopicArn"`
	synthesisInput
}

type taskOutput struct {
	SNSTopicArn       string   `json:"SnsTopicArn,omitempty"`
	LanguageCode      string   `json:"LanguageCode,omitempty"`
	VoiceID           string   `json:"VoiceId"`
	OutputFormat      string   `json:"OutputFormat"`
	OutputURI         string   `json:"OutputUri"`
	Engine            string   `json:"Engine"`
	TextType          string   `json:"TextType"`
	TaskStatusReason  string   `json:"TaskStatusReason,omitempty"`
	SampleRate        string   `json:"SampleRate"`
	TaskStatus        string   `json:"TaskStatus"`
	TaskID            string   `json:"TaskId"`
	SpeechMarkTypes   []string `json:"SpeechMarkTypes,omitempty"`
	LexiconNames      []string `json:"LexiconNames,omitempty"`
	CreationTime      float64  `json:"CreationTime"`
	RequestCharacters int      `json:"RequestCharacters"`
}

func buildTaskOutput(task *SpeechSynthesisTask) taskOutput {
	return taskOutput{
		CreationTime:      float64(task.CreationTime.UnixMilli()) / millisecondsPerSecond,
		Engine:            task.Options.Engine,
		LanguageCode:      task.Options.LanguageCode,
		LexiconNames:      task.Options.LexiconNames,
		OutputFormat:      task.Options.OutputFormat,
		OutputURI:         task.OutputURI,
		RequestCharacters: len(task.Options.Text),
		SampleRate:        task.Options.SampleRate,
		SNSTopicArn:       task.SNSTopicArn,
		SpeechMarkTypes:   task.Options.SpeechMarkTypes,
		TaskID:            task.TaskID,
		TaskStatus:        task.TaskStatus,
		TaskStatusReason:  task.TaskStatusReason,
		TextType:          task.Options.TextType,
		VoiceID:           task.Options.VoiceID,
	}
}

func (h *Handler) startTask(c *echo.Context) error {
	var in startTaskInput
	if err := decodeRequest(c, &in); err != nil {
		return err
	}

	task, err := h.Backend.StartSpeechSynthesisTask(
		in.options(), in.OutputS3BucketName, in.OutputS3KeyPrefix, in.SNSTopicArn,
	)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{"SynthesisTask": buildTaskOutput(task)})
}

func (h *Handler) getTask(c *echo.Context, id string) error {
	task, err := h.Backend.GetSpeechSynthesisTask(id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{"SynthesisTask": buildTaskOutput(task)})
}

func (h *Handler) listTasks(c *echo.Context) error {
	maxResults, err := maxResultsParam(c.QueryParam(queryMaxResults))
	if err != nil {
		return err
	}

	tasks, token, err := h.Backend.ListSpeechSynthesisTasks(
		c.QueryParam(queryStatus),
		c.QueryParam(queryNextToken),
		maxResults,
	)
	if err != nil {
		return err
	}

	out := make([]taskOutput, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, buildTaskOutput(task))
	}

	resp := map[string]any{"SynthesisTasks": out}
	// AWS omits NextToken when there are no further results.
	if token != "" {
		resp["NextToken"] = token
	}

	return c.JSON(http.StatusOK, resp)
}

type putLexiconInput struct {
	Content string `json:"Content"`
}

func (h *Handler) putLexicon(c *echo.Context, name string) error {
	var in putLexiconInput
	if err := decodeRequest(c, &in); err != nil {
		return err
	}
	if err := h.Backend.PutLexicon(name, in.Content); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, struct{}{})
}

func (h *Handler) getLexicon(c *echo.Context, name string) error {
	lexicon, err := h.Backend.GetLexicon(name)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{
		"Lexicon": map[string]string{
			"Content": lexicon.Content,
			"Name":    lexicon.Name,
		},
		"LexiconAttributes": lexiconAttributes(lexicon),
	})
}

func (h *Handler) deleteLexicon(c *echo.Context, name string) error {
	if err := h.Backend.DeleteLexicon(name); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, struct{}{})
}

func (h *Handler) listLexicons(c *echo.Context) error {
	lexicons := h.Backend.ListLexicons()
	offset, err := parseToken(c.QueryParam(queryNextToken), len(lexicons))
	if err != nil {
		return err
	}
	maxResults, err := maxResultsParam(c.QueryParam(queryMaxResults))
	if err != nil {
		return err
	}
	end := min(offset+maxResults, len(lexicons))
	descriptions := make([]map[string]any, 0, end-offset)
	for _, lexicon := range lexicons[offset:end] {
		descriptions = append(descriptions, map[string]any{
			"Name":       lexicon.Name,
			"Attributes": lexiconAttributes(lexicon),
		})
	}

	resp := map[string]any{"Lexicons": descriptions}
	// AWS omits NextToken when there are no further results.
	if end < len(lexicons) {
		resp["NextToken"] = encodeToken(end)
	}

	return c.JSON(http.StatusOK, resp)
}

// lexiconAttributes builds the LexiconAttributes shape (Alphabet,
// LanguageCode, LastModified, LexemesCount, LexiconArn, Size). Real Polly's
// LexiconAttributes has no Name field -- GetLexicon's Name lives on the
// sibling "Lexicon" member, and ListLexicons' Name is a sibling of
// "Attributes" on each LexiconDescription item, never inside it.
func lexiconAttributes(lexicon *Lexicon) map[string]any {
	return map[string]any{
		"Alphabet":     lexicon.Alphabet,
		"LanguageCode": lexicon.LanguageCode,
		"LastModified": float64(lexicon.LastModified.UnixMilli()) / millisecondsPerSecond,
		"LexemesCount": lexicon.LexemesCount,
		"LexiconArn":   lexicon.ARN,
		"Size":         lexicon.Size,
	}
}

func (h *Handler) describeVoices(c *echo.Context) error {
	include, err := strconv.ParseBool(defaultQueryValue(c.QueryParam(queryIncludeAdditional), "false"))
	if err != nil {
		return fmt.Errorf("%w: invalid IncludeAdditionalLanguageCodes", ErrValidation)
	}

	voices, err := h.Backend.DescribeVoices(DescribeVoicesFilter{
		Engine:                         c.QueryParam(queryEngine),
		LanguageCode:                   c.QueryParam(queryLanguageCode),
		Gender:                         c.QueryParam(queryGender),
		IncludeAdditionalLanguageCodes: include,
	})
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]any{"Voices": voices})
}

func decodeRequest(c *echo.Context, value any) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return fmt.Errorf("%w: failed to read body", ErrValidation)
	}
	if jsonErr := json.Unmarshal(body, value); jsonErr != nil {
		return fmt.Errorf("%w: invalid JSON: %w", ErrValidation, jsonErr)
	}

	return nil
}

// pollyErrorEntry maps a backend sentinel error to its AWS __type name and
// HTTP status code.
type pollyErrorEntry struct {
	sentinel error
	typ      string
	status   int
}

// onceErrorTable lazily builds the ordered sentinel-error-to-wire-shape
// table exactly once. Order matters: it is scanned top to bottom and the
// first errors.Is match wins. ErrStreamValidation must stay first --
// startSpeechSynthesisStream wraps the more specific sentinels below
// (ErrEngineNotSupported, ErrValidation, ...) inside ErrStreamValidation, and
// errors.Is walks the whole chain, so checking it first ensures
// StartSpeechSynthesisStream always reports the AWS-accurate generic
// ValidationException instead of a SynthesizeSpeech-specific exception name
// that operation never returns. ErrValidation stays last since it is every
// other sentinel's fallback (see ErrValidation's doc comment).
//
//nolint:gochecknoglobals // read-only package-level lookup table, apigatewayv2 style
var onceErrorTable = sync.OnceValue(func() []pollyErrorEntry {
	return []pollyErrorEntry{
		{ErrStreamValidation, "ValidationException", http.StatusBadRequest},
		{ErrLexiconNotFound, "LexiconNotFoundException", http.StatusNotFound},
		{ErrInvalidTaskID, "InvalidTaskIdException", http.StatusBadRequest},
		// AWS models SynthesisTaskNotFoundException with httpStatusCode 400, not 404.
		{ErrTaskNotFound, "SynthesisTaskNotFoundException", http.StatusBadRequest},
		{ErrTextLengthExceeded, "TextLengthExceededException", http.StatusBadRequest},
		{ErrInvalidSampleRate, "InvalidSampleRateException", http.StatusBadRequest},
		{ErrEngineNotSupported, "EngineNotSupportedException", http.StatusBadRequest},
		{ErrLanguageNotSupported, "LanguageNotSupportedException", http.StatusBadRequest},
		{ErrMarksNotSupportedForFormat, "MarksNotSupportedForFormatException", http.StatusBadRequest},
		{ErrSsmlMarksNotSupportedForTextType, "SsmlMarksNotSupportedForTextTypeException", http.StatusBadRequest},
		{ErrInvalidSsml, "InvalidSsmlException", http.StatusBadRequest},
		{ErrInvalidNextToken, "InvalidNextTokenException", http.StatusBadRequest},
		{ErrInvalidLexicon, "InvalidLexiconException", http.StatusBadRequest},
		{ErrLexiconSizeExceeded, "LexiconSizeExceededException", http.StatusBadRequest},
		{ErrMaxLexemeLengthExceeded, "MaxLexemeLengthExceededException", http.StatusBadRequest},
		{ErrMaxLexiconsNumberExceeded, "MaxLexiconsNumberExceededException", http.StatusBadRequest},
		{ErrUnsupportedPlsAlphabet, "UnsupportedPlsAlphabetException", http.StatusBadRequest},
		{ErrUnsupportedPlsLanguage, "UnsupportedPlsLanguageException", http.StatusBadRequest},
		{ErrInvalidS3Bucket, "InvalidS3BucketException", http.StatusBadRequest},
		{ErrInvalidS3Key, "InvalidS3KeyException", http.StatusBadRequest},
		{ErrInvalidSnsTopicArn, "InvalidSnsTopicArnException", http.StatusBadRequest},
		{ErrValidation, "InvalidParameterValueException", http.StatusBadRequest},
	}
})

func (h *Handler) writeBackendError(c *echo.Context, err error) error {
	for _, entry := range onceErrorTable() {
		if errors.Is(err, entry.sentinel) {
			return writeError(c, entry.status, entry.typ, err.Error())
		}
	}

	return writeError(c, http.StatusInternalServerError, "ServiceFailureException", err.Error())
}

func writeError(c *echo.Context, code int, typ, message string) error {
	return c.JSON(code, map[string]string{
		"__type":  typ,
		"message": message,
	})
}

func defaultQueryValue(got, fallback string) string {
	if got == "" {
		return fallback
	}

	return got
}

func maxResultsParam(value string) (int, error) {
	if value == "" {
		return maxTaskPageSize, nil
	}
	maxResults, err := strconv.Atoi(value)
	if err != nil || maxResults < 1 || maxResults > maxTaskPageSize {
		return 0, fmt.Errorf("%w: MaxResults must be between 1 and %d", ErrValidation, maxTaskPageSize)
	}

	return maxResults, nil
}
