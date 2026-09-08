//go:build !integration

package mediastoredata_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/mediastoredata"
)

// chromeUserAgent is a realistic browser User-Agent value, used to prove a
// browser-shaped request (which cannot set User-Agent to an SDK marker --
// see RouteMatcher's doc comment) still matches via X-Amz-User-Agent.
const chromeUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

func newTestHandler(t *testing.T) *mediastoredata.Handler {
	t.Helper()

	return mediastoredata.NewHandler(mediastoredata.NewInMemoryBackend("us-east-1"))
}

func doRequest(
	t *testing.T,
	h *mediastoredata.Handler,
	method, path string,
	body []byte,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	e := echo.New()
	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("User-Agent", "aws-sdk-go-v2/1.0.0 mediastoredata/1.29.18")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestMediaStoreData_Name(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, "MediaStoreData", h.Name())
}

func TestMediaStoreData_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.GetSupportedOperations()

	assert.Contains(t, ops, "PutObject")
	assert.Contains(t, ops, "GetObject")
	assert.Contains(t, ops, "DeleteObject")
	assert.Contains(t, ops, "ListItems")
	assert.Contains(t, ops, "DescribeObject")
}

func TestMediaStoreData_RouteMatcher(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	e := echo.New()

	tests := []struct {
		name          string
		userAgent     string
		xAmzUserAgent string
		wantMatch     bool
	}{
		{
			name:      "sdk_user_agent_matches",
			userAgent: "aws-sdk-go-v2/1.0.0 mediastoredata/1.29.18",
			wantMatch: true,
		},
		{
			name:      "s3_user_agent_does_not_match",
			userAgent: "aws-sdk-go-v2/1.0.0 s3/1.0.0",
			wantMatch: false,
		},
		{
			name:      "empty_user_agent_does_not_match",
			userAgent: "",
			wantMatch: false,
		},
		{
			// A real browser cannot set User-Agent itself (forbidden by the
			// Fetch spec) -- the browser sends its own literal UA there, and
			// the AWS SDK for JavaScript puts its SDK identification in
			// X-Amz-User-Agent instead. MediaStore Data's serviceId is
			// "MediaStore Data" (with a space); the SDK's user-agent escaping
			// turns the space into a hyphen ("MediaStore-Data"), which is a
			// different literal string from aws-sdk-go-v2's module-path-derived
			// "mediastoredata" marker, not just a case difference.
			name:          "browser_user_agent_matches_via_x_amz_user_agent",
			userAgent:     chromeUserAgent,
			xAmzUserAgent: "aws-sdk-js/3.1094.0 ua/2.1 os/browser lang/js md/react-native api/MediaStore-Data/3.1094.0",
			wantMatch:     true,
		},
		{
			// Same browser shape, but for a different service (S3) -- must NOT
			// be claimed by MediaStoreData's matcher.
			name:          "browser_wrong_service_x_amz_user_agent_does_not_match",
			userAgent:     chromeUserAgent,
			xAmzUserAgent: "aws-sdk-js/3.1094.0 ua/2.1 os/browser lang/js api/S3/3.1094.0",
			wantMatch:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			if tt.xAmzUserAgent != "" {
				req.Header.Set("X-Amz-User-Agent", tt.xAmzUserAgent)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			got := h.RouteMatcher()(c)
			assert.Equal(t, tt.wantMatch, got)
		})
	}
}

func TestMediaStoreData_PutObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		contentType string
		body        []byte
		wantStatus  int
		wantETag    bool
	}{
		{
			name:        "simple_object",
			path:        "/video/clip.mp4",
			body:        []byte("video content"),
			contentType: "video/mp4",
			wantStatus:  http.StatusOK,
			wantETag:    true,
		},
		{
			name:        "empty_body",
			path:        "/empty/file.txt",
			body:        []byte{},
			contentType: "text/plain",
			wantStatus:  http.StatusOK,
			wantETag:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			headers := map[string]string{
				"Content-Type": tt.contentType,
			}
			rec := doRequest(t, h, http.MethodPut, tt.path, tt.body, headers)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantETag {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["ETag"])
				assert.NotEmpty(t, resp["StorageClass"])
			}
		})
	}
}

func TestMediaStoreData_GetObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		putPath    string
		getPath    string
		putBody    []byte
		wantStatus int
		wantBody   bool
	}{
		{
			name:       "existing_object",
			putPath:    "/video/clip.mp4",
			putBody:    []byte("hello world"),
			getPath:    "/video/clip.mp4",
			wantStatus: http.StatusOK,
			wantBody:   true,
		},
		{
			name:       "missing_object",
			putPath:    "",
			putBody:    nil,
			getPath:    "/missing/file.mp4",
			wantStatus: http.StatusNotFound,
			wantBody:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.putPath != "" {
				doRequest(t, h, http.MethodPut, tt.putPath, tt.putBody, nil)
			}

			rec := doRequest(t, h, http.MethodGet, tt.getPath, nil, nil)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantBody {
				assert.Equal(t, tt.putBody, rec.Body.Bytes())
			}
		})
	}
}

func TestMediaStoreData_DeleteObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		wantStatus int
		putFirst   bool
	}{
		{
			name:       "existing_object",
			putFirst:   true,
			path:       "/video/clip.mp4",
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_object",
			putFirst:   false,
			path:       "/missing/file.mp4",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.putFirst {
				doRequest(t, h, http.MethodPut, tt.path, []byte("data"), nil)
			}

			rec := doRequest(t, h, http.MethodDelete, tt.path, nil, nil)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestMediaStoreData_ListItems(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		objects   map[string][]byte
		listPath  string
		wantNames []string
		wantTypes []string
	}{
		{
			name: "list_root",
			objects: map[string][]byte{
				"/video/clip.mp4": []byte("a"),
				"/audio/song.mp3": []byte("b"),
			},
			listPath:  "/",
			wantNames: []string{"audio", "video"},
			wantTypes: []string{"FOLDER", "FOLDER"},
		},
		{
			name: "list_subfolder",
			objects: map[string][]byte{
				"/video/clip.mp4":    []byte("a"),
				"/video/trailer.mp4": []byte("b"),
			},
			listPath:  "/?Path=video",
			wantNames: []string{"clip.mp4", "trailer.mp4"},
			wantTypes: []string{"OBJECT", "OBJECT"},
		},
		{
			name:      "empty_result",
			objects:   map[string][]byte{},
			listPath:  "/",
			wantNames: []string{},
			wantTypes: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for path, body := range tt.objects {
				doRequest(t, h, http.MethodPut, path, body, nil)
			}

			rec := doRequest(t, h, http.MethodGet, tt.listPath, nil, nil)

			assert.Equal(t, http.StatusOK, rec.Code)

			var resp struct {
				Items []struct {
					Name string `json:"Name"`
					Type string `json:"Type"`
				} `json:"Items"`
			}

			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Len(t, resp.Items, len(tt.wantNames))

			for i, item := range resp.Items {
				assert.Equal(t, tt.wantNames[i], item.Name)
				assert.Equal(t, tt.wantTypes[i], item.Type)
			}
		})
	}
}

func TestMediaStoreData_DescribeObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		putBody    []byte
		wantStatus int
		putFirst   bool
		wantETag   bool
	}{
		{
			name:       "existing_object",
			putFirst:   true,
			putBody:    []byte("some content"),
			path:       "/video/clip.mp4",
			wantStatus: http.StatusOK,
			wantETag:   true,
		},
		{
			name:       "missing_object",
			putFirst:   false,
			putBody:    nil,
			path:       "/missing.mp4",
			wantStatus: http.StatusNotFound,
			wantETag:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.putFirst {
				doRequest(t, h, http.MethodPut, tt.path, tt.putBody, map[string]string{"Content-Type": "video/mp4"})
			}

			rec := doRequest(t, h, http.MethodHead, tt.path, nil, nil)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantETag {
				assert.NotEmpty(t, rec.Header().Get("ETag"))
				assert.NotEmpty(t, rec.Header().Get("Content-Length"))
			}
		})
	}
}

func TestMediaStoreData_Lifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		contentType string
		wantFolder  string
		content     []byte
	}{
		{
			name:        "mp4_object_in_media_folder",
			path:        "/media/test.mp4",
			content:     []byte("test media content"),
			contentType: "video/mp4",
			wantFolder:  "media",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Put object.
			putRec := doRequest(t, h, http.MethodPut, tt.path, tt.content, map[string]string{
				"Content-Type": tt.contentType,
			})
			require.Equal(t, http.StatusOK, putRec.Code)

			// Get object and verify body matches.
			getRec := doRequest(t, h, http.MethodGet, tt.path, nil, nil)
			require.Equal(t, http.StatusOK, getRec.Code)
			assert.Equal(t, tt.content, getRec.Body.Bytes())

			// Describe object.
			headRec := doRequest(t, h, http.MethodHead, tt.path, nil, nil)
			require.Equal(t, http.StatusOK, headRec.Code)
			assert.NotEmpty(t, headRec.Header().Get("ETag"))

			// List items at root.
			listRec := doRequest(t, h, http.MethodGet, "/", nil, nil)
			require.Equal(t, http.StatusOK, listRec.Code)

			var listResp struct {
				Items []struct {
					Name string `json:"Name"`
					Type string `json:"Type"`
				} `json:"Items"`
			}

			require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
			require.Len(t, listResp.Items, 1)
			assert.Equal(t, tt.wantFolder, listResp.Items[0].Name)
			assert.Equal(t, "FOLDER", listResp.Items[0].Type)

			// Delete object.
			delRec := doRequest(t, h, http.MethodDelete, tt.path, nil, nil)
			require.Equal(t, http.StatusOK, delRec.Code)

			// Verify deletion.
			getRec2 := doRequest(t, h, http.MethodGet, tt.path, nil, nil)
			assert.Equal(t, http.StatusNotFound, getRec2.Code)
		})
	}
}

func TestMediaStoreData_PutObject_SHA256InResponse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	content := []byte("hello sha256")
	sum := sha256.Sum256(content)
	wantSHA := hex.EncodeToString(sum[:])

	rec := doRequest(t, h, http.MethodPut, "/test/sha.bin", content, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, wantSHA, resp["ContentSHA256"])
	assert.NotEmpty(t, rec.Header().Get("ETag"))
}

func TestMediaStoreData_GetObject_XAmzSHA256Header(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	content := []byte("sha header check")

	doRequest(t, h, http.MethodPut, "/sha/file.bin", content, nil)

	rec := doRequest(t, h, http.MethodGet, "/sha/file.bin", nil, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("X-Amz-Content-Sha256"))
	assert.Equal(t, "bytes", rec.Header().Get("Accept-Ranges"))
}

func TestMediaStoreData_PathValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		wantType   string // __type on the wire; empty means "don't check" (2xx cases).
		wantStatus int
	}{
		{
			name:       "empty_path_rejected",
			path:       "/",
			wantStatus: http.StatusBadRequest,
			wantType:   "ValidationException",
		},
		{
			name:       "traversal_rejected",
			path:       "/../etc/passwd",
			wantStatus: http.StatusBadRequest,
			wantType:   "ValidationException",
		},
		{
			name:       "valid_path_accepted",
			path:       "/folder/file.mp4",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodPut, tt.path, []byte("data"), nil)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantType != "" {
				// __type must be a real mediastoredata/AWS-wide error name --
				// NOT a fabricated service-specific exception -- since the
				// real SDK error model has no per-op ValidationException
				// entry but ValidationException is the established
				// AWS-wide/gopherstack-wide convention for parameter
				// validation failures (see errors.go).
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantType, resp["__type"])
			}
		})
	}
}

func TestMediaStoreData_StorageClassValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		storageClass string
		wantType     string // __type on the wire; empty means "don't check" (2xx cases).
		wantStatus   int
	}{
		{name: "temporal_valid", storageClass: "TEMPORAL", wantStatus: http.StatusOK},
		// "STANDARD" is an UploadAvailability value, not a StorageClass -- real
		// MediaStore Data's only StorageClass is "TEMPORAL", so this must be
		// rejected the same as any other unknown storage class.
		{
			name: "standard_rejected_not_a_storage_class", storageClass: "STANDARD",
			wantStatus: http.StatusBadRequest, wantType: "ValidationException",
		},
		{name: "empty_defaults_to_temporal", storageClass: "", wantStatus: http.StatusOK},
		{
			name: "unknown_rejected", storageClass: "GLACIER",
			wantStatus: http.StatusBadRequest, wantType: "ValidationException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			headers := map[string]string{}
			if tt.storageClass != "" {
				headers["X-Amz-Storage-Class"] = tt.storageClass
			}

			rec := doRequest(t, h, http.MethodPut, "/cls/file.bin", []byte("data"), headers)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantType != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantType, resp["__type"])
			}
		})
	}
}

func TestMediaStoreData_UploadAvailabilityValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		uploadAvailability string
		wantType           string // __type on the wire; empty means "don't check" (2xx cases).
		wantStatus         int
	}{
		{name: "standard_valid", uploadAvailability: "STANDARD", wantStatus: http.StatusOK},
		{name: "streaming_valid", uploadAvailability: "STREAMING", wantStatus: http.StatusOK},
		{name: "empty_defaults_to_standard", uploadAvailability: "", wantStatus: http.StatusOK},
		{
			name: "unknown_rejected", uploadAvailability: "BOGUS",
			wantStatus: http.StatusBadRequest, wantType: "ValidationException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			headers := map[string]string{}
			if tt.uploadAvailability != "" {
				headers["X-Amz-Upload-Availability"] = tt.uploadAvailability
			}

			rec := doRequest(t, h, http.MethodPut, "/avail/file.bin", []byte("data"), headers)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantType != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantType, resp["__type"])
			}
		})
	}
}

func TestMediaStoreData_ObjectSizeLimit(t *testing.T) {
	t.Parallel()

	// aws-sdk-go-v2/service/mediastoredata/api_op_PutObject.go:13-14: "Object
	// sizes are limited to 25 MB for standard upload availability and 10 MB
	// for streaming upload availability."
	const streamingLimit = 10 * 1024 * 1024

	tests := []struct {
		name               string
		uploadAvailability string
		wantType           string // __type on the wire; empty means "don't check" (2xx cases).
		bodySize           int
		wantStatus         int
	}{
		{
			name:               "streaming_at_limit_accepted",
			uploadAvailability: "STREAMING",
			bodySize:           streamingLimit,
			wantStatus:         http.StatusOK,
		},
		{
			name: "streaming_over_limit_rejected", uploadAvailability: "STREAMING", bodySize: streamingLimit + 1,
			wantStatus: http.StatusBadRequest, wantType: "ValidationException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			headers := map[string]string{"X-Amz-Upload-Availability": tt.uploadAvailability}
			body := make([]byte, tt.bodySize)

			rec := doRequest(t, h, http.MethodPut, "/size/file.bin", body, headers)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantType != "" {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantType, resp["__type"])
			}
		})
	}
}

func TestMediaStoreData_ConditionalGet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	content := []byte("conditional content")
	doRequest(t, h, http.MethodPut, "/cond/obj.bin", content, nil)

	getRec := doRequest(t, h, http.MethodGet, "/cond/obj.bin", nil, nil)
	etag := getRec.Header().Get("ETag")
	require.NotEmpty(t, etag)

	tests := []struct {
		headers    map[string]string
		name       string
		wantStatus int
	}{
		{
			name:       "if_none_match_matches_304",
			headers:    map[string]string{"If-None-Match": etag},
			wantStatus: http.StatusNotModified,
		},
		{
			name:       "if_none_match_star_304",
			headers:    map[string]string{"If-None-Match": "*"},
			wantStatus: http.StatusNotModified,
		},
		{
			name:       "if_none_match_different_200",
			headers:    map[string]string{"If-None-Match": `"differentetag"`},
			wantStatus: http.StatusOK,
		},
		{
			name:       "if_match_matches_200",
			headers:    map[string]string{"If-Match": etag},
			wantStatus: http.StatusOK,
		},
		{
			name:       "if_match_different_412",
			headers:    map[string]string{"If-Match": `"wrongetag"`},
			wantStatus: http.StatusPreconditionFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, http.MethodGet, "/cond/obj.bin", nil, tt.headers)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestMediaStoreData_RangeRequests(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	content := []byte("0123456789abcdef") // 16 bytes
	doRequest(t, h, http.MethodPut, "/range/obj.bin", content, nil)

	tests := []struct {
		name       string
		rangeHdr   string
		wantBody   []byte
		wantStatus int
	}{
		{
			name:       "full_range",
			rangeHdr:   "bytes=0-15",
			wantStatus: http.StatusPartialContent,
			wantBody:   content,
		},
		{
			name:       "partial_range",
			rangeHdr:   "bytes=4-7",
			wantStatus: http.StatusPartialContent,
			wantBody:   []byte("4567"),
		},
		{
			name:       "suffix_range",
			rangeHdr:   "bytes=-4",
			wantStatus: http.StatusPartialContent,
			wantBody:   []byte("cdef"),
		},
		{
			name:       "open_range",
			rangeHdr:   "bytes=12-",
			wantStatus: http.StatusPartialContent,
			wantBody:   []byte("cdef"),
		},
		{
			name:       "out_of_range_416",
			rangeHdr:   "bytes=100-200",
			wantStatus: http.StatusRequestedRangeNotSatisfiable,
			wantBody:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, http.MethodGet, "/range/obj.bin", nil, map[string]string{"Range": tt.rangeHdr})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantBody != nil {
				assert.Equal(t, tt.wantBody, rec.Body.Bytes())
			}

			if tt.wantStatus == http.StatusPartialContent {
				assert.NotEmpty(t, rec.Header().Get("Content-Range"))
			}

			if tt.wantStatus == http.StatusRequestedRangeNotSatisfiable {
				// __type must be the real modeled exception name
				// (RequestedRangeNotSatisfiableException) -- a real SDK
				// client's errors.As(&types.RequestedRangeNotSatisfiableException{})
				// only matches this exact string, not a fabricated
				// "InvalidRangeException".
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, "RequestedRangeNotSatisfiableException", resp["__type"])
				assert.NotEmpty(t, rec.Header().Get("Content-Range"))
			}
		})
	}
}

func TestMediaStoreData_ListItems_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Insert 5 objects.
	for i := range 5 {
		doRequest(t, h, http.MethodPut, fmt.Sprintf("/items/%02d.bin", i), []byte("data"), nil)
	}

	// Page 1: MaxResults=2.
	rec1 := doRequest(t, h, http.MethodGet, "/?Path=items&MaxResults=2", nil, nil)
	require.Equal(t, http.StatusOK, rec1.Code)

	var page1 struct {
		NextToken *string `json:"NextToken"`
		Items     []struct {
			Name string `json:"Name"`
		} `json:"Items"`
	}

	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &page1))
	assert.Len(t, page1.Items, 2)
	require.NotNil(t, page1.NextToken)

	// Page 2: continue from NextToken.
	rec2 := doRequest(t, h, http.MethodGet, "/?Path=items&MaxResults=2&NextToken="+*page1.NextToken, nil, nil)
	require.Equal(t, http.StatusOK, rec2.Code)

	var page2 struct {
		NextToken *string `json:"NextToken"`
		Items     []struct {
			Name string `json:"Name"`
		} `json:"Items"`
	}

	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &page2))
	assert.Len(t, page2.Items, 2)

	// Names must not overlap pages.
	for _, i1 := range page1.Items {
		for _, i2 := range page2.Items {
			assert.NotEqual(t, i1.Name, i2.Name)
		}
	}
}

// TestMediaStoreData_ListItems_MaxResultsBound verifies ListItems rejects a
// MaxResults outside the AWS 1-1000 range with a ValidationException.
func TestMediaStoreData_ListItems_MaxResultsBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{name: "valid", query: "/?MaxResults=10", wantStatus: http.StatusOK},
		{name: "at_upper_bound", query: "/?MaxResults=1000", wantStatus: http.StatusOK},
		{name: "over_upper_bound", query: "/?MaxResults=1001", wantStatus: http.StatusBadRequest},
		{name: "zero", query: "/?MaxResults=0", wantStatus: http.StatusBadRequest},
		{name: "non_numeric", query: "/?MaxResults=lots", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodGet, tt.query, nil, nil)
			assert.Equal(t, tt.wantStatus, rec.Code, "body: %s", rec.Body.String())
		})
	}
}

func TestMediaStoreData_ContentSHA256Verification(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	content := []byte("verify sha")
	sum := sha256.Sum256(content)
	correctSHA := hex.EncodeToString(sum[:])

	t.Run("correct_sha_accepted", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodPut, "/verify/obj.bin", content, map[string]string{
			"X-Amz-Content-SHA256": correctSHA,
		})
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("wrong_sha_rejected", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodPut, "/verify/obj2.bin", content, map[string]string{
			"X-Amz-Content-SHA256": "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		// "XAmzContentSHA256Mismatch" is the real AWS error name for this
		// condition (used by S3 and other signed REST APIs for a declared
		// payload hash that doesn't match the actual body), not a fabricated
		// mediastoredata-specific "InvalidContentSHA256Exception".
		var resp map[string]string
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		assert.Equal(t, "XAmzContentSHA256Mismatch", resp["__type"])
	})

	t.Run("unsigned_payload_skips_check", func(t *testing.T) {
		t.Parallel()

		rec := doRequest(t, h, http.MethodPut, "/verify/obj3.bin", content, map[string]string{
			"X-Amz-Content-SHA256": "UNSIGNED-PAYLOAD",
		})
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
