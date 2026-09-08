package mediastoredata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

const (
	itemTypeObject = "OBJECT"
	// maxListItemsResults is the AWS upper bound on ListItems MaxResults.
	maxListItemsResults = 1000
)

const (
	// msdMatchPriority must be higher than S3 (0) to intercept mediastoredata SDK requests.
	msdMatchPriority = 87
	// userAgentMarker is the marker aws-sdk-go-v2 puts in its User-Agent header
	// for MediaStore Data requests -- derived from the Go module path
	// ("mediastoredata"), so it has no separator and no space.
	userAgentMarker = "mediastoredata"
	// jsUserAgentMarker is the marker the AWS SDK for JavaScript puts in its
	// User-Agent (Node) or X-Amz-User-Agent (browser) header instead. Its
	// serviceId is "MediaStore Data" (with a space); the SDK's user-agent
	// escaping turns the space into a hyphen, producing "MediaStore-Data" --
	// a different literal string from userAgentMarker above, not just a case
	// difference, so it needs its own marker rather than case-insensitivity
	// alone.
	jsUserAgentMarker = "mediastore-data"
)

// Handler is the Echo HTTP handler for Amazon MediaStore Data operations.
type Handler struct {
	Backend *InMemoryBackend
}

// NewHandler creates a new MediaStore Data handler.
func NewHandler(backend *InMemoryBackend) *Handler {
	return &Handler{Backend: backend}
}

// Name returns the service name.
func (h *Handler) Name() string { return "MediaStoreData" }

// GetSupportedOperations returns the list of supported operations.
func (h *Handler) GetSupportedOperations() []string {
	return []string{
		"PutObject",
		"GetObject",
		"DeleteObject",
		"ListItems",
		"DescribeObject",
	}
}

// ChaosServiceName returns the lowercase AWS service name for fault rule matching.
func (h *Handler) ChaosServiceName() string { return "mediastoredata" }

// ChaosOperations returns all operations that can be fault-injected.
func (h *Handler) ChaosOperations() []string { return h.GetSupportedOperations() }

// ChaosRegions returns all regions this handler instance handles.
func (h *Handler) ChaosRegions() []string { return []string{h.Backend.Region()} }

// RouteMatcher returns a function that matches MediaStore Data requests.
// It identifies requests by the "mediastoredata"/"mediastore-data" marker
// present in either the User-Agent header (set by native SDKs) or the
// X-Amz-User-Agent header (used by the AWS SDK for JavaScript in a browser,
// which cannot set User-Agent itself -- see service.MatchesUserAgentMarker).
func (h *Handler) RouteMatcher() service.Matcher {
	return func(c *echo.Context) bool {
		return service.MatchesUserAgentMarker(
			c.Request().Header,
			userAgentMarker,
			jsUserAgentMarker,
		)
	}
}

// MatchPriority returns the routing priority.
func (h *Handler) MatchPriority() int { return msdMatchPriority }

// ExtractOperation returns the operation name from the request method.
func (h *Handler) ExtractOperation(c *echo.Context) string {
	r := c.Request()

	switch r.Method {
	case http.MethodPut:
		return "PutObject"
	case http.MethodGet:
		if r.URL.Path == "/" || r.URL.Path == "" {
			return "ListItems"
		}

		return "GetObject"
	case http.MethodDelete:
		return "DeleteObject"
	case http.MethodHead:
		return "DescribeObject"
	}

	return "Unknown"
}

// ExtractResource extracts the path from the URL.
func (h *Handler) ExtractResource(c *echo.Context) string {
	return c.Request().URL.Path
}

// requestContext returns the request context enriched with the per-request AWS
// region (from SigV4 credential scope, falling back to the backend default).
// Backend operations call getRegion on this context to route to the correct
// region-isolated store.
func (h *Handler) requestContext(c *echo.Context) context.Context {
	region := httputils.ExtractRegionFromRequest(c.Request(), h.Backend.Region())

	return context.WithValue(c.Request().Context(), regionContextKey{}, region)
}

// Handler returns the Echo handler function.
func (h *Handler) Handler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		r := c.Request()
		log := logger.Load(r.Context())

		switch r.Method {
		case http.MethodPut:
			return h.handlePutObject(c)
		case http.MethodGet:
			if r.URL.Path == "/" || r.URL.Path == "" {
				return h.handleListItems(c)
			}

			return h.handleGetObject(c)
		case http.MethodDelete:
			return h.handleDeleteObject(c)
		case http.MethodHead:
			return h.handleDescribeObject(c)
		}

		log.WarnContext(
			r.Context(),
			"mediastoredata: unmatched request",
			"method",
			r.Method,
			"path",
			r.URL.Path,
		)

		return writeErrorJSON(
			c,
			http.StatusMethodNotAllowed,
			"MethodNotAllowedException",
			"method not allowed",
		)
	}
}

// handlePutObject handles PUT /{Path+}.
func (h *Handler) handlePutObject(c *echo.Context) error {
	r := c.Request()
	log := logger.Load(r.Context())

	body, err := httputils.ReadBody(r)
	if err != nil {
		log.ErrorContext(r.Context(), "mediastoredata: failed to read body", "error", err)

		return writeErrorJSON(
			c,
			http.StatusInternalServerError,
			"InternalServerError",
			"failed to read request body",
		)
	}

	// Verify x-amz-content-sha256 if supplied by the SDK. This mirrors real
	// S3-family behavior (the "XAmzContentSHA256Mismatch" error, returned
	// when the declared payload hash doesn't match the actual body) rather
	// than a fabricated mediastoredata-specific exception name -- this check
	// is a generic SigV4-payload-integrity concern shared by signed AWS REST
	// APIs, not app-level validation specific to this service, so it isn't
	// enumerated in mediastoredata's own narrow 4-exception model. It is also
	// NOT redundant with pkgs/httputils.SigV4Validator: that validator only
	// confirms the request signature is internally self-consistent with
	// whatever payload hash the client declared -- it never independently
	// recomputes the hash of the bytes actually received, so a declared hash
	// that doesn't match the real body slips past it undetected.
	if declared := r.Header.Get("X-Amz-Content-Sha256"); declared != "" &&
		declared != "UNSIGNED-PAYLOAD" && declared != "STREAMING-AWS4-HMAC-SHA256-PAYLOAD" {
		if actual := contentSHA256(body); actual != declared {
			return writeErrorJSON(c, http.StatusBadRequest, "XAmzContentSHA256Mismatch",
				fmt.Sprintf("content SHA256 mismatch: got %s, expected %s", actual, declared))
		}
	}

	path := r.URL.Path
	contentType := r.Header.Get("Content-Type")
	cacheControl := r.Header.Get("Cache-Control")
	storageClass := r.Header.Get("X-Amz-Storage-Class")
	uploadAvailability := r.Header.Get("X-Amz-Upload-Availability")

	obj, putErr := h.Backend.PutObject(
		h.requestContext(
			c,
		),
		path,
		body,
		contentType,
		cacheControl,
		storageClass,
		uploadAvailability,
	)
	if putErr != nil {
		return h.writeError(c, putErr)
	}

	// Real MediaStore Data returns ETag only in the JSON response body (see
	// aws-sdk-go-v2/service/mediastoredata's PutObjectOutput deserializer,
	// which has no HTTP-binding for ETag). Echoing it as a response header
	// too is extra and harmless -- the SDK client only reads the body -- but
	// keep this comment accurate so a future auditor doesn't mistake it for
	// a documented wire requirement.
	c.Response().Header().Set("ETag", obj.ETag)

	return c.JSON(http.StatusOK, map[string]string{
		"ContentSHA256": obj.SHA256,
		"ETag":          obj.ETag,
		"StorageClass":  obj.StorageClass,
	})
}

// handleGetObject handles GET /{Path+}.
func (h *Handler) handleGetObject(c *echo.Context) error {
	r := c.Request()

	obj, err := h.Backend.GetObject(h.requestContext(c), r.URL.Path)
	if err != nil {
		return h.writeError(c, err)
	}

	// Evaluate conditional request headers before sending body.
	if status, ok := evalConditional(r, obj); !ok {
		w := c.Response()
		setObjectHeaders(w, obj)
		w.WriteHeader(status)

		return nil
	}

	// Handle Range request.
	if rangeHdr := r.Header.Get("Range"); rangeHdr != "" {
		return h.handleRangeGet(c, obj, rangeHdr)
	}

	w := c.Response()
	setObjectHeaders(w, obj)
	// AWS MediaStore Data returns X-Amz-Content-SHA256 on GET responses.
	w.Header().Set("X-Amz-Content-Sha256", obj.SHA256)
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusOK)

	if _, writeErr := w.Write(obj.Body); writeErr != nil {
		logger.Load(r.Context()).
			ErrorContext(r.Context(), "mediastoredata: failed to write response body", "error", writeErr)
	}

	return nil
}

// handleRangeGet serves a partial response for a Range GET.
func (h *Handler) handleRangeGet(c *echo.Context, obj *Object, rangeHdr string) error {
	size := obj.ContentLength
	start, end, ok := parseByteRange(rangeHdr, size)

	if !ok {
		c.Response().Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))

		// "RequestedRangeNotSatisfiableException" is the real modeled name
		// (aws-sdk-go-v2/service/mediastoredata/types.
		// RequestedRangeNotSatisfiableException) -- a fabricated
		// "InvalidRangeException" __type here would not deserialize into the
		// typed SDK error a real client checks for via errors.As, even
		// though the HTTP status code (416) was already correct.
		return writeErrorJSON(c, http.StatusRequestedRangeNotSatisfiable,
			"RequestedRangeNotSatisfiableException", "requested range not satisfiable")
	}

	chunk := obj.Body[start : end+1]
	w := c.Response()
	setObjectHeaders(w, obj)
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(chunk)), 10))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-Amz-Content-Sha256", contentSHA256(chunk))
	w.WriteHeader(http.StatusPartialContent)

	if _, writeErr := w.Write(chunk); writeErr != nil {
		logger.Load(c.Request().Context()).
			ErrorContext(c.Request().Context(), "mediastoredata: failed to write range body", "error", writeErr)
	}

	return nil
}

// handleDeleteObject handles DELETE /{Path+}.
func (h *Handler) handleDeleteObject(c *echo.Context) error {
	r := c.Request()

	if err := h.Backend.DeleteObject(h.requestContext(c), r.URL.Path); err != nil {
		return h.writeError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

// listItemsOutput is the JSON response for ListItems.
type listItemsOutput struct {
	NextToken *string     `json:"NextToken,omitempty"`
	Items     []itemEntry `json:"Items"`
}

// itemEntry represents a single item in a ListItems response.
type itemEntry struct {
	LastModified  *float64 `json:"LastModified,omitempty"`
	ContentLength *int64   `json:"ContentLength,omitempty"`
	ContentType   string   `json:"ContentType,omitempty"`
	ETag          string   `json:"ETag,omitempty"`
	Name          string   `json:"Name"`
	Type          string   `json:"Type"`
}

// handleListItems handles GET / or GET /?Path=...
func (h *Handler) handleListItems(c *echo.Context) error {
	q := c.Request().URL.Query()

	in := ListItemsInput{
		FolderPath: q.Get("Path"),
		NextToken:  q.Get("NextToken"),
	}

	if raw := q.Get("MaxResults"); raw != "" {
		// AWS MediaStore Data bounds ListItems MaxResults to 1-1000.
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxListItemsResults {
			return writeErrorJSON(
				c,
				http.StatusBadRequest,
				"ValidationException",
				"MaxResults must be between 1 and 1000",
			)
		}

		in.MaxResults = n
	}

	result := h.Backend.ListItems(h.requestContext(c), in)
	entries := make([]itemEntry, 0, len(result.Items))

	for _, item := range result.Items {
		entry := itemEntry{
			Name: item.Name,
			Type: item.Type,
		}

		if item.Type == itemTypeObject {
			ts := float64(item.LastModified.Unix())
			entry.LastModified = &ts
			entry.ContentLength = &item.ContentLength
			entry.ContentType = item.ContentType
			entry.ETag = item.ETag
		}

		entries = append(entries, entry)
	}

	out := listItemsOutput{Items: entries}
	if result.NextToken != "" {
		out.NextToken = &result.NextToken
	}

	return c.JSON(http.StatusOK, out)
}

// handleDescribeObject handles HEAD /{Path+}.
func (h *Handler) handleDescribeObject(c *echo.Context) error {
	r := c.Request()

	obj, err := h.Backend.GetObject(h.requestContext(c), r.URL.Path)
	if err != nil {
		return h.writeError(c, err)
	}

	w := c.Response()
	setObjectHeaders(w, obj)
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusOK)

	return nil
}

// setObjectHeaders sets common response headers for an object.
func setObjectHeaders(w http.ResponseWriter, obj *Object) {
	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(obj.ContentLength, 10))
	w.Header().Set("ETag", obj.ETag)
	w.Header().Set("Last-Modified", obj.LastModified.UTC().Format(http.TimeFormat))

	if obj.CacheControl != "" {
		w.Header().Set("Cache-Control", obj.CacheControl)
	}
}

// evalConditional checks conditional request headers (If-Match, If-None-Match,
// If-Modified-Since, If-Unmodified-Since). Returns (statusCode, proceed).
// proceed=false means the caller should write statusCode and stop.
func evalConditional(r *http.Request, obj *Object) (int, bool) {
	etag := obj.ETag

	// If-None-Match: return 304 if client already has this version.
	if inm := r.Header.Get("If-None-Match"); inm != "" {
		if etagMatches(inm, etag) {
			return http.StatusNotModified, false
		}
	}

	// If-Match: return 412 if client's precondition is not satisfied.
	if im := r.Header.Get("If-Match"); im != "" {
		if !etagMatches(im, etag) {
			return http.StatusPreconditionFailed, false
		}
	}

	// If-Modified-Since: return 304 if object has not changed.
	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		if t, err := http.ParseTime(ims); err == nil && !obj.LastModified.After(t) {
			return http.StatusNotModified, false
		}
	}

	// If-Unmodified-Since: return 412 if object has been modified.
	if ius := r.Header.Get("If-Unmodified-Since"); ius != "" {
		if t, err := http.ParseTime(ius); err == nil && obj.LastModified.After(t) {
			return http.StatusPreconditionFailed, false
		}
	}

	return http.StatusOK, true
}

// etagMatches reports whether the wildcard "*" or a quoted ETag from the
// header value matches target.
func etagMatches(header, target string) bool {
	if strings.TrimSpace(header) == "*" {
		return true
	}

	for part := range strings.SplitSeq(header, ",") {
		if strings.TrimSpace(part) == target {
			return true
		}
	}

	return false
}

// parseSuffixRange parses "bytes=-N" (last N bytes).
func parseSuffixRange(endStr string, size int64) (int64, int64, bool) {
	n, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil || n < 0 {
		return 0, 0, false
	}

	return max(size-n, 0), size - 1, true
}

// parseOpenRange parses "bytes=N-" (from N to end).
func parseOpenRange(startStr string, size int64) (int64, int64, bool) {
	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}

	return start, size - 1, true
}

// parseClosedRange parses "bytes=N-M". end is not yet clamped to size.
func parseClosedRange(startStr, endStr string) (int64, int64, bool) {
	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}

	end, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil || end < start {
		return 0, 0, false
	}

	return start, end, true
}

// parseByteRange parses a "bytes=start-end" Range header value.
// Returns (start, end, ok). end is inclusive and clamped to size-1.
// Only single-range specs are supported.
func parseByteRange(header string, size int64) (int64, int64, bool) {
	spec, ok := strings.CutPrefix(header, "bytes=")
	if !ok || strings.Contains(spec, ",") {
		return 0, 0, false
	}

	startStr, endStr, hasDash := strings.Cut(spec, "-")
	if !hasDash {
		return 0, 0, false
	}

	var start, end int64

	switch {
	case startStr == "" && endStr != "":
		start, end, ok = parseSuffixRange(endStr, size)
	case startStr != "" && endStr == "":
		start, end, ok = parseOpenRange(startStr, size)
	default:
		start, end, ok = parseClosedRange(startStr, endStr)
		if ok && end >= size {
			end = size - 1
		}
	}

	if !ok || start >= size {
		return 0, 0, false
	}

	return start, end, true
}

// writeError maps backend errors to appropriate HTTP responses.
//
// ObjectNotFoundException and InternalServerError are both part of the real
// mediastoredata SDK's 4-exception model. ErrInvalidPath/ErrInvalidStorageClass
// already carry "ValidationException" as their wrapped message (see
// errors.go) -- err.Error() below is used verbatim as both the message AND
// (via the case's hardcoded literal) implicitly documents that the __type
// must match, so there's no separate fabricated exception name here anymore.
func (h *Handler) writeError(c *echo.Context, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return writeErrorJSON(c, http.StatusNotFound, "ObjectNotFoundException", err.Error())
	case errors.Is(err, ErrInvalidPath),
		errors.Is(err, ErrInvalidStorageClass),
		errors.Is(err, ErrInvalidUploadAvailability),
		errors.Is(err, ErrObjectTooLarge):
		return writeErrorJSON(c, http.StatusBadRequest, "ValidationException", err.Error())
	}

	return writeErrorJSON(c, http.StatusInternalServerError, "InternalServerError", err.Error())
}

// errorResponse returns a JSON-serialisable error payload.
func errorResponse(code, msg string) map[string]string {
	return map[string]string{"__type": code, "message": msg}
}

// writeErrorJSON writes an error response with both the JSON __type body
// field AND the X-Amzn-ErrorType header real mediastoredata sets on every
// error response (deserializeOpError<Op> in aws-sdk-go-v2/service/
// mediastoredata/deserializers.go checks this header BEFORE ever attempting
// to decode a body). Setting only the body is insufficient: DescribeObject
// is a HEAD request, and net/http's client transport always discards a HEAD
// response's body per RFC 7231 §4.3.2 regardless of what the server sent, so
// without this header a real SDK caller can NEVER recover a typed error
// (ObjectNotFoundException, InternalServerError, ...) from a failed
// DescribeObject -- every such error silently degrades to an untyped
// "UnknownError" smithy.GenericAPIError.
func writeErrorJSON(c *echo.Context, status int, code, msg string) error {
	c.Response().Header().Set("X-Amzn-Errortype", code)

	return c.JSON(status, errorResponse(code, msg))
}
