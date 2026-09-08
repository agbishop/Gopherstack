package translate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	translateTargetPrefix = "AWSShineFrontendService_20170701."
	translateContentType  = "application/x-amz-json-1.1"
	unknownOperation      = "Unknown"

	keyName                = "Name"
	keyJobID               = "JobId"
	keyResourceARN         = "ResourceArn"
	keyJobStatus           = "JobStatus"
	keyStatus              = "Status"
	keySourceLanguageCode  = "SourceLanguageCode"
	keyTargetLanguageCode  = "TargetLanguageCode"
	keyTargetLanguageCodes = "TargetLanguageCodes"
	keyLanguageCode        = "LanguageCode"
	keyLanguageName        = "LanguageName"

	// sourceLangAuto is the sentinel SourceLanguageCode value that requests
	// automatic source-language detection (real AWS resolves it via an
	// internal Amazon Comprehend call); it is never validated against
	// knownLanguageCodesTable the way a real language code is.
	sourceLangAuto = "auto"
)

type opFunc func(map[string]any) (map[string]any, error)

// Handler serves Amazon Translate JSON operations.
type Handler struct {
	Backend *InMemoryBackend
	ops     map[string]opFunc
}

// NewHandler creates a Translate handler backed by in-memory state.
func NewHandler(backend *InMemoryBackend) *Handler {
	h := &Handler{Backend: backend}
	h.ops = h.buildOps()

	return h
}

// Reset clears backend state.
func (h *Handler) Reset() { h.Backend.Reset() }

// Name returns service name.
func (h *Handler) Name() string { return "Translate" }

// ChaosServiceName returns service key for fault matching.
func (h *Handler) ChaosServiceName() string { return "translate" }

// ChaosOperations returns fault-injectable operations.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns configured service region.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// MatchPriority returns header matching priority.
func (h *Handler) MatchPriority() int { return service.PriorityHeaderExact }

// RouteMatcher matches Translate X-Amz-Target headers.
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().Header.Get("X-Amz-Target"), translateTargetPrefix)
	}
}

// ExtractOperation returns the operation name from the request target.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	target := c.Request().Header.Get("X-Amz-Target")
	if !strings.HasPrefix(target, translateTargetPrefix) {
		return unknownOperation
	}

	return strings.TrimPrefix(target, translateTargetPrefix)
}

// ExtractResource retrieves the primary resource name.
func (h *Handler) ExtractResource(c *echo.Context) string {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return ""
	}

	var values map[string]any
	if unmarshalErr := json.Unmarshal(body, &values); unmarshalErr != nil {
		return ""
	}

	for _, key := range []string{keyName, keyJobID, keyResourceARN} {
		if v, ok := values[key].(string); ok && v != "" {
			return v
		}
	}

	return ""
}

// GetSupportedOperations returns all implemented operation names.
func (h *Handler) GetSupportedOperations() []string {
	ops := make([]string, 0, len(h.ops))
	for name := range h.ops {
		ops = append(ops, name)
	}

	return ops
}

// Handler returns the Echo HTTP handler.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return service.HandleTarget(
			c, logger.Load(c.Request().Context()), h.Name(), translateContentType,
			h.GetSupportedOperations(), h.dispatch, h.handleError,
		)
	}
}

func (h *Handler) dispatch(_ context.Context, action string, body []byte) ([]byte, error) {
	fn, ok := h.ops[action]
	if !ok {
		return nil, fmt.Errorf("%w: operation %q", ErrValidation, action)
	}

	var input map[string]any
	if len(body) > 0 {
		if err := json.Unmarshal(body, &input); err != nil {
			return nil, fmt.Errorf("%w: invalid JSON: %w", ErrValidation, err)
		}
	}

	if input == nil {
		input = make(map[string]any)
	}

	output, err := fn(input)
	if err != nil {
		return nil, err
	}

	return json.Marshal(output)
}

func (h *Handler) handleError(_ context.Context, c *echo.Context, _ string, err error) error {
	code := "InternalServerException"
	status := http.StatusInternalServerError

	switch {
	case errors.Is(err, ErrNotFound):
		code, status = "ResourceNotFoundException", http.StatusBadRequest
	case errors.Is(err, ErrConflict):
		code, status = "ConflictException", http.StatusBadRequest
	case errors.Is(err, ErrValidation):
		code, status = "InvalidRequestException", http.StatusBadRequest
	case errors.Is(err, ErrInvalidParameter):
		code, status = "InvalidParameterValueException", http.StatusBadRequest
	case errors.Is(err, ErrLimitExceeded):
		code, status = "LimitExceededException", http.StatusBadRequest
	case errors.Is(err, ErrTextSizeLimitExceeded):
		code, status = "TextSizeLimitExceededException", http.StatusBadRequest
	case errors.Is(err, ErrTooManyTags):
		code, status = "TooManyTagsException", http.StatusBadRequest
	case errors.Is(err, ErrUnsupportedLanguagePair):
		code, status = "UnsupportedLanguagePairException", http.StatusBadRequest
	case errors.Is(err, ErrUnsupportedDisplayLanguage):
		code, status = "UnsupportedDisplayLanguageCodeException", http.StatusBadRequest
	case errors.Is(err, ErrInvalidFilter):
		code, status = "InvalidFilterException", http.StatusBadRequest
	case errors.Is(err, ErrConcurrentModification):
		code, status = "ConcurrentModificationException", http.StatusBadRequest
	}

	c.Response().Header().Set("Content-Type", translateContentType)

	return c.JSON(status, map[string]string{
		"__type":  code,
		"message": err.Error(),
	})
}

func (h *Handler) buildOps() map[string]opFunc {
	return map[string]opFunc{
		"ImportTerminology":          h.importTerminology,
		"GetTerminology":             h.getTerminology,
		"DeleteTerminology":          h.deleteTerminology,
		"ListTerminologies":          h.listTerminologies,
		"CreateParallelData":         h.createParallelData,
		"GetParallelData":            h.getParallelData,
		"UpdateParallelData":         h.updateParallelData,
		"DeleteParallelData":         h.deleteParallelData,
		"ListParallelData":           h.listParallelData,
		"StartTextTranslationJob":    h.startTextTranslationJob,
		"StopTextTranslationJob":     h.stopTextTranslationJob,
		"DescribeTextTranslationJob": h.describeTextTranslationJob,
		"ListTextTranslationJobs":    h.listTextTranslationJobs,
		"TranslateText":              h.translateText,
		"TranslateDocument":          h.translateDocument,
		"ListLanguages":              h.listLanguages,
		"TagResource":                h.tagResource,
		"UntagResource":              h.untagResource,
		"ListTagsForResource":        h.listTagsForResource,
	}
}

// --- Wire helpers shared across op-family handler files ---

func extractEncryptionKey(input map[string]any) *EncryptionKey {
	ek, ok := input["EncryptionKey"].(map[string]any)
	if !ok {
		return nil
	}

	return &EncryptionKey{
		ID:   strField(ek, "Id"),
		Type: strField(ek, "Type"),
	}
}

func extractTags(input map[string]any) map[string]string {
	return extractTagsFromSlice(input, "Tags")
}

func extractTagsFromSlice(input map[string]any, key string) map[string]string {
	raw, ok := input[key].([]any)
	if !ok {
		return nil
	}

	tags := make(map[string]string, len(raw))

	for _, item := range raw {
		m, mok := item.(map[string]any)
		if !mok {
			continue
		}

		k, _ := m["Key"].(string)
		v, _ := m["Value"].(string)

		if k != "" {
			tags[k] = v
		}
	}

	return tags
}

func strField(m map[string]any, key string) string {
	v, _ := m[key].(string)

	return v
}

func maxResultsField(m map[string]any) int {
	switch v := m["MaxResults"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}

	return 0
}

func strSliceField(m map[string]any, key string) []string {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(raw))

	for _, v := range raw {
		if s, sok := v.(string); sok {
			out = append(out, s)
		}
	}

	return out
}
