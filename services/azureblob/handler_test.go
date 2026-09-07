package azureblob_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azureblob"
)

const testAccount = "devstoreaccount1"

func newTestHandler(t *testing.T) *azureblob.Handler {
	t.Helper()

	backend := azureblob.NewInMemoryBackend()

	return azureblob.NewHandler(backend)
}

// doRequest builds an echo context for method/path (with optional headers and
// body) and invokes the handler directly, mirroring services/sqs's doRequest.
func doRequest(
	t *testing.T,
	h *azureblob.Handler,
	method, path string,
	body []byte,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, http.NoBody)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.Handler()(c))

	return rec
}

func TestContainerLifecycle_CreateListDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "create_list_delete_container"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, http.MethodPut, "/"+testAccount+"/mycontainer?restype=container", nil, nil)
			require.Equal(t, http.StatusCreated, rec.Code, tt.name)
			assert.NotEmpty(t, rec.Header().Get("X-Ms-Version"))
			assert.NotEmpty(t, rec.Header().Get("X-Ms-Request-Id"))
			assert.NotEmpty(t, rec.Header().Get("Date"))

			rec = doRequest(t, h, http.MethodGet, "/"+testAccount+"?comp=list", nil, nil)
			require.Equal(t, http.StatusOK, rec.Code, tt.name)
			assert.Contains(t, rec.Body.String(), "<Name>mycontainer</Name>")
			assert.Contains(t, rec.Body.String(), "<EnumerationResults")

			rec = doRequest(t, h, http.MethodDelete, "/"+testAccount+"/mycontainer?restype=container", nil, nil)
			require.Equal(t, http.StatusAccepted, rec.Code, tt.name)

			rec = doRequest(t, h, http.MethodGet, "/"+testAccount+"?comp=list", nil, nil)
			require.Equal(t, http.StatusOK, rec.Code, tt.name)
			assert.NotContains(t, rec.Body.String(), "<Name>mycontainer</Name>")
		})
	}
}

func TestDeleteContainer_MissingReturns404(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		container string
	}{
		{name: "missing_container_404", container: "does-not-exist"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, http.MethodDelete, "/"+testAccount+"/"+tt.container+"?restype=container", nil, nil)

			require.Equal(t, http.StatusNotFound, rec.Code, tt.name)
			assert.Contains(t, rec.Body.String(), "ContainerNotFound")
		})
	}
}

func TestBlobLifecycle_PutGetHeadDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "put_get_head_delete_blob", body: "hello azure blob"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createContainer(t, h, "mycontainer")

			putHeaders := map[string]string{
				"X-Ms-Blob-Type": "BlockBlob",
				"Content-Type":   "text/plain",
			}
			rec := doRequest(t, h, http.MethodPut, "/"+testAccount+"/mycontainer/myblob.txt",
				[]byte(tt.body), putHeaders)
			require.Equal(t, http.StatusCreated, rec.Code, tt.name)
			assert.NotEmpty(t, rec.Header().Get("ETag"), tt.name)

			rec = doRequest(t, h, http.MethodGet, "/"+testAccount+"/mycontainer/myblob.txt", nil, nil)
			require.Equal(t, http.StatusOK, rec.Code, tt.name)
			assert.Equal(t, tt.body, rec.Body.String(), tt.name)
			assert.Equal(t, "text/plain", rec.Header().Get("Content-Type"), tt.name)
			assert.Equal(t, "BlockBlob", rec.Header().Get("X-Ms-Blob-Type"), tt.name)

			rec = doRequest(t, h, http.MethodHead, "/"+testAccount+"/mycontainer/myblob.txt", nil, nil)
			require.Equal(t, http.StatusOK, rec.Code, tt.name)
			assert.Empty(t, rec.Body.String(), tt.name)
			assert.Equal(t, strconv.Itoa(len(tt.body)), rec.Header().Get("Content-Length"), tt.name)

			rec = doRequest(t, h, http.MethodDelete, "/"+testAccount+"/mycontainer/myblob.txt", nil, nil)
			require.Equal(t, http.StatusAccepted, rec.Code, tt.name)

			rec = doRequest(t, h, http.MethodGet, "/"+testAccount+"/mycontainer/myblob.txt", nil, nil)
			require.Equal(t, http.StatusNotFound, rec.Code, tt.name)
			assert.Contains(t, rec.Body.String(), "BlobNotFound", tt.name)
		})
	}
}

func TestPutBlob_RequiresBlockBlobType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		blobType string
	}{
		{name: "missing_blob_type_rejected", blobType: ""},
		{name: "page_blob_type_rejected", blobType: "PageBlob"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createContainer(t, h, "mycontainer")

			headers := map[string]string{}
			if tt.blobType != "" {
				headers["X-Ms-Blob-Type"] = tt.blobType
			}

			rec := doRequest(t, h, http.MethodPut, "/"+testAccount+"/mycontainer/myblob.txt", []byte("x"), headers)

			require.Equal(t, http.StatusBadRequest, rec.Code, tt.name)
			assert.Contains(t, rec.Body.String(), "InvalidHeaderValue", tt.name)
		})
	}
}

func TestPutBlob_MissingContainerReturns404(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "put_blob_missing_container"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(t, h, http.MethodPut, "/"+testAccount+"/does-not-exist/myblob.txt",
				[]byte("x"), map[string]string{"X-Ms-Blob-Type": "BlockBlob"})

			require.Equal(t, http.StatusNotFound, rec.Code, tt.name)
			assert.Contains(t, rec.Body.String(), "ContainerNotFound", tt.name)
		})
	}
}

func TestGetBlob_MissingBlobReturns404(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "get_missing_blob"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createContainer(t, h, "mycontainer")

			rec := doRequest(t, h, http.MethodGet, "/"+testAccount+"/mycontainer/does-not-exist.txt", nil, nil)

			require.Equal(t, http.StatusNotFound, rec.Code, tt.name)
			assert.Contains(t, rec.Body.String(), "BlobNotFound", tt.name)
		})
	}
}

func TestGetBlob_RangeHeaderPartialRead(t *testing.T) {
	t.Parallel()

	const body = "0123456789"

	tests := []struct {
		name       string
		rangeValue string
		wantBody   string
		wantStatus int
	}{
		{name: "start_end", rangeValue: "bytes=2-5", wantStatus: http.StatusPartialContent, wantBody: "2345"},
		{name: "open_ended", rangeValue: "bytes=7-", wantStatus: http.StatusPartialContent, wantBody: "789"},
		{name: "suffix", rangeValue: "bytes=-3", wantStatus: http.StatusPartialContent, wantBody: "789"},
		{
			name: "unsatisfiable", rangeValue: "bytes=100-200",
			wantStatus: http.StatusRequestedRangeNotSatisfiable, wantBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createContainer(t, h, "mycontainer")
			doRequest(t, h, http.MethodPut, "/"+testAccount+"/mycontainer/data.bin",
				[]byte(body), map[string]string{"X-Ms-Blob-Type": "BlockBlob"})

			rec := doRequest(t, h, http.MethodGet, "/"+testAccount+"/mycontainer/data.bin", nil,
				map[string]string{"Range": tt.rangeValue})

			require.Equal(t, tt.wantStatus, rec.Code, tt.name)
			if tt.wantStatus == http.StatusPartialContent {
				assert.Equal(t, tt.wantBody, rec.Body.String(), tt.name)
				assert.NotEmpty(t, rec.Header().Get("Content-Range"), tt.name)
			}
		})
	}
}

// TestGetBlob_XMsRangeHeader proves x-ms-range is accepted as an alias for
// the standard Range header (with x-ms-range taking precedence when both are
// present, matching real Azure Blob's documented behavior) -- the Python
// (azure-storage-blob) and JS (@azure/storage-blob) Get Blob clients send
// x-ms-range exclusively, never Range, so accepting only "Range" made this
// backend's GetBlob unreachable from those SDKs' default download path. See AZURE.md
// section 7's M4 cross-SDK smoke test, which caught this.
func TestGetBlob_XMsRangeHeader(t *testing.T) {
	t.Parallel()

	const body = "0123456789"

	tests := []struct {
		name             string
		headers          map[string]string
		wantBody         string
		wantContentRange string
	}{
		{
			name: "x_ms_range_only", headers: map[string]string{"X-Ms-Range": "bytes=2-5"},
			wantBody: "2345", wantContentRange: "bytes 2-5/10",
		},
		{
			name:     "x_ms_range_takes_precedence_over_range",
			headers:  map[string]string{"X-Ms-Range": "bytes=2-5", "Range": "bytes=7-9"},
			wantBody: "2345", wantContentRange: "bytes 2-5/10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createContainer(t, h, "mycontainer")
			doRequest(t, h, http.MethodPut, "/"+testAccount+"/mycontainer/data.bin",
				[]byte(body), map[string]string{"X-Ms-Blob-Type": "BlockBlob"})

			rec := doRequest(t, h, http.MethodGet, "/"+testAccount+"/mycontainer/data.bin", nil, tt.headers)

			require.Equal(t, http.StatusPartialContent, rec.Code, tt.name)
			assert.Equal(t, tt.wantBody, rec.Body.String(), tt.name)
			assert.Equal(t, tt.wantContentRange, rec.Header().Get("Content-Range"), tt.name)
		})
	}
}

func TestListBlobs_MissingContainerReturns404(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "list_blobs_missing_container"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			rec := doRequest(
				t,
				h,
				http.MethodGet,
				"/"+testAccount+"/does-not-exist?restype=container&comp=list",
				nil,
				nil,
			)

			require.Equal(t, http.StatusNotFound, rec.Code, tt.name)
			assert.Contains(t, rec.Body.String(), "ContainerNotFound", tt.name)
		})
	}
}

func TestListBlobs_ReturnsAllBlobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		blobs []string
	}{
		{name: "two_blobs", blobs: []string{"a.txt", "b.txt"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createContainer(t, h, "mycontainer")

			for _, name := range tt.blobs {
				doRequest(t, h, http.MethodPut, "/"+testAccount+"/mycontainer/"+name,
					[]byte("data"), map[string]string{"X-Ms-Blob-Type": "BlockBlob"})
			}

			rec := doRequest(t, h, http.MethodGet, "/"+testAccount+"/mycontainer?restype=container&comp=list", nil, nil)

			require.Equal(t, http.StatusOK, rec.Code, tt.name)
			for _, name := range tt.blobs {
				assert.Contains(t, rec.Body.String(), "<Name>"+name+"</Name>", tt.name)
			}
		})
	}
}

func TestHandler_Reset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "reset_clears_containers"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createContainer(t, h, "mycontainer")

			h.Reset()

			rec := doRequest(t, h, http.MethodGet, "/"+testAccount+"?comp=list", nil, nil)
			require.Equal(t, http.StatusOK, rec.Code, tt.name)
			assert.NotContains(t, rec.Body.String(), "<Name>mycontainer</Name>", tt.name)
		})
	}
}

func TestHandler_Name(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, "AzureBlob", h.Name())
}

func TestHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.GetSupportedOperations()

	assert.Contains(t, ops, "PutBlob")
	assert.Contains(t, ops, "GetBlob")
	assert.Contains(t, ops, "ListContainers")
}

// TestErrNilAppContext and TestProviderInit live in provider_test.go.

func createContainer(t *testing.T, h *azureblob.Handler, name string) {
	t.Helper()

	rec := doRequest(t, h, http.MethodPut, "/"+testAccount+"/"+name+"?restype=container", nil, nil)
	require.Equal(t, http.StatusCreated, rec.Code)
}
