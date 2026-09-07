package glacier_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glacier"
)

func TestInitiateMultipartUpload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		vaultName    string
		description  string
		partSize     string
		wantStatus   int
		wantUploadID bool
	}{
		{
			name:         "success_with_description",
			vaultName:    "mp-vault",
			description:  "my backup",
			partSize:     "4194304",
			wantStatus:   http.StatusCreated,
			wantUploadID: true,
		},
		{
			name:         "missing_part_size_rejected",
			vaultName:    "mp-vault2",
			description:  "",
			partSize:     "",
			wantStatus:   http.StatusBadRequest,
			wantUploadID: false,
		},
		{
			name:       "vault_not_found",
			vaultName:  "nonexistent",
			partSize:   "1048576",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			if tt.wantStatus != http.StatusNotFound {
				rec := doRequest(t, h, http.MethodPut, "/-/vaults/"+tt.vaultName, "")
				require.Equal(t, http.StatusCreated, rec.Code)
			}

			headers := map[string]string{}
			if tt.description != "" {
				headers["X-Amz-Archive-Description"] = tt.description
			}

			if tt.partSize != "" {
				headers["X-Amz-Part-Size"] = tt.partSize
			}

			rec := doRequestWithHeaders(
				t,
				h,
				http.MethodPost,
				"/-/vaults/"+tt.vaultName+"/multipart-uploads",
				"",
				headers,
			)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantUploadID {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["uploadId"])

				uploadID := rec.Header().Get("X-Amz-Multipart-Upload-Id")
				assert.NotEmpty(t, uploadID)
			}
		})
	}
}

// ----------------------------------------
// UploadMultipartPart
// ----------------------------------------

func TestUploadMultipartPart(t *testing.T) {
	t.Parallel()

	// partBody is exactly 1 MiB, matching the 1048576-byte part size declared
	// at InitiateMultipartUpload below, so it passes the part-size/alignment
	// check as well as the tree-hash check.
	partBody := strings.Repeat("a", 1<<20)
	partChecksum := glacier.ComputeTreeHash([]byte(partBody))

	tests := []struct {
		name        string
		uploadID    string
		rangeHdr    string
		checksum    string
		body        string
		wantStatus  int
		setupUpload bool
	}{
		{
			name:        "success",
			setupUpload: true,
			rangeHdr:    "bytes 0-1048575/*",
			checksum:    partChecksum,
			body:        partBody,
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "checksum_mismatch_rejected",
			setupUpload: true,
			rangeHdr:    "bytes 0-1048575/*",
			checksum:    "abc123",
			body:        partBody,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "upload_not_found",
			setupUpload: false,
			uploadID:    "nonexistent-upload-id",
			rangeHdr:    "bytes 0-1048575/*",
			body:        partBody,
			wantStatus:  http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			vaultName := "upload-part-vault"

			rec := doRequest(t, h, http.MethodPut, "/-/vaults/"+vaultName, "")
			require.Equal(t, http.StatusCreated, rec.Code)

			uploadID := tt.uploadID

			if tt.setupUpload {
				rec = doRequestWithHeaders(
					t,
					h,
					http.MethodPost,
					"/-/vaults/"+vaultName+"/multipart-uploads",
					"",
					map[string]string{"X-Amz-Part-Size": "1048576"},
				)
				require.Equal(t, http.StatusCreated, rec.Code)
				uploadID = rec.Header().Get("X-Amz-Multipart-Upload-Id")
				require.NotEmpty(t, uploadID)
			}

			headers := map[string]string{
				"Content-Range":          tt.rangeHdr,
				"X-Amz-Sha256-Tree-Hash": tt.checksum,
			}

			rec = doRequestWithHeaders(
				t, h, http.MethodPut,
				"/-/vaults/"+vaultName+"/multipart-uploads/"+uploadID,
				tt.body, headers,
			)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// ----------------------------------------
// CompleteMultipartUpload
// ----------------------------------------

func TestCompleteMultipartUpload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		uploadID    string
		wantStatus  int
		setupUpload bool
		wantArchive bool
	}{
		{
			name:        "success",
			setupUpload: true,
			wantStatus:  http.StatusCreated,
			wantArchive: true,
		},
		{
			name:       "upload_not_found",
			uploadID:   "nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			vaultName := "complete-mp-vault"

			rec := doRequest(t, h, http.MethodPut, "/-/vaults/"+vaultName, "")
			require.Equal(t, http.StatusCreated, rec.Code)

			uploadID := tt.uploadID

			if tt.setupUpload {
				rec = doRequestWithHeaders(
					t,
					h,
					http.MethodPost,
					"/-/vaults/"+vaultName+"/multipart-uploads",
					"",
					map[string]string{"X-Amz-Part-Size": "1048576"},
				)
				require.Equal(t, http.StatusCreated, rec.Code)
				uploadID = rec.Header().Get("X-Amz-Multipart-Upload-Id")
				require.NotEmpty(t, uploadID)

				partBody := strings.Repeat("a", 1<<20)
				rec = doRequestWithHeaders(
					t, h, http.MethodPut,
					"/-/vaults/"+vaultName+"/multipart-uploads/"+uploadID,
					partBody, map[string]string{"Content-Range": "bytes 0-1048575/*"},
				)
				require.Equal(t, http.StatusNoContent, rec.Code)
			}

			headers := map[string]string{
				"X-Amz-Archive-Size":     "1048576",
				"X-Amz-Sha256-Tree-Hash": glacier.ComputeTreeHash([]byte(strings.Repeat("a", 1<<20))),
			}

			rec = doRequestWithHeaders(
				t, h, http.MethodPost,
				"/-/vaults/"+vaultName+"/multipart-uploads/"+uploadID,
				"", headers,
			)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantArchive {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.NotEmpty(t, resp["archiveId"])
				assert.NotEmpty(t, rec.Header().Get("X-Amz-Archive-Id"))
			}
		})
	}
}

// ----------------------------------------
// AbortMultipartUpload
// ----------------------------------------

func TestAbortMultipartUpload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		uploadID    string
		wantStatus  int
		setupUpload bool
	}{
		{
			name:        "success",
			setupUpload: true,
			wantStatus:  http.StatusNoContent,
		},
		{
			name:       "upload_not_found",
			uploadID:   "nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			vaultName := "abort-mp-vault"

			rec := doRequest(t, h, http.MethodPut, "/-/vaults/"+vaultName, "")
			require.Equal(t, http.StatusCreated, rec.Code)

			uploadID := tt.uploadID

			if tt.setupUpload {
				rec = doRequestWithHeaders(
					t,
					h,
					http.MethodPost,
					"/-/vaults/"+vaultName+"/multipart-uploads",
					"",
					map[string]string{"X-Amz-Part-Size": "1048576"},
				)
				require.Equal(t, http.StatusCreated, rec.Code)
				uploadID = rec.Header().Get("X-Amz-Multipart-Upload-Id")
				require.NotEmpty(t, uploadID)
			}

			rec = doRequest(t, h, http.MethodDelete, "/-/vaults/"+vaultName+"/multipart-uploads/"+uploadID, "")
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// ----------------------------------------
// ListMultipartUploads
// ----------------------------------------

func TestListMultipartUploads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		numUploads int
		wantCount  int
		wantStatus int
	}{
		{
			name:       "empty_list",
			numUploads: 0,
			wantCount:  0,
			wantStatus: http.StatusOK,
		},
		{
			name:       "two_uploads",
			numUploads: 2,
			wantCount:  2,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			vaultName := "list-mp-vault"

			rec := doRequest(t, h, http.MethodPut, "/-/vaults/"+vaultName, "")
			require.Equal(t, http.StatusCreated, rec.Code)

			for range tt.numUploads {
				rec = doRequestWithHeaders(
					t,
					h,
					http.MethodPost,
					"/-/vaults/"+vaultName+"/multipart-uploads",
					"",
					map[string]string{"X-Amz-Part-Size": "1048576"},
				)
				require.Equal(t, http.StatusCreated, rec.Code)
			}

			rec = doRequest(t, h, http.MethodGet, "/-/vaults/"+vaultName+"/multipart-uploads", "")
			assert.Equal(t, tt.wantStatus, rec.Code)

			var resp struct {
				UploadsList []any `json:"UploadsList"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Len(t, resp.UploadsList, tt.wantCount)
		})
	}
}

// ----------------------------------------
// ListParts
// ----------------------------------------

func TestListParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		uploadID    string
		uploadParts int
		wantStatus  int
		wantParts   int
	}{
		{
			name:        "empty_parts",
			uploadParts: 0,
			wantStatus:  http.StatusOK,
			wantParts:   0,
		},
		{
			name:        "two_parts",
			uploadParts: 2,
			wantStatus:  http.StatusOK,
			wantParts:   2,
		},
		{
			name:       "upload_not_found",
			uploadID:   "nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			vaultName := "list-parts-vault"

			rec := doRequest(t, h, http.MethodPut, "/-/vaults/"+vaultName, "")
			require.Equal(t, http.StatusCreated, rec.Code)

			uploadID := tt.uploadID

			if tt.wantStatus == http.StatusOK {
				rec = doRequestWithHeaders(
					t,
					h,
					http.MethodPost,
					"/-/vaults/"+vaultName+"/multipart-uploads",
					"",
					map[string]string{"X-Amz-Part-Size": "1048576"},
				)
				require.Equal(t, http.StatusCreated, rec.Code)
				uploadID = rec.Header().Get("X-Amz-Multipart-Upload-Id")
				require.NotEmpty(t, uploadID)

				const partSize = 1 << 20

				for i := range tt.uploadParts {
					start := i * partSize
					rangeHdr := fmt.Sprintf("bytes %d-%d/*", start, start+partSize-1)
					body := strings.Repeat(string(rune('a'+i)), partSize)

					headers := map[string]string{
						"Content-Range":          rangeHdr,
						"X-Amz-Sha256-Tree-Hash": glacier.ComputeTreeHash([]byte(body)),
					}

					pr := doRequestWithHeaders(
						t, h, http.MethodPut,
						"/-/vaults/"+vaultName+"/multipart-uploads/"+uploadID,
						body, headers,
					)
					require.Equal(t, http.StatusNoContent, pr.Code)
				}
			}

			rec = doRequest(t, h, http.MethodGet, "/-/vaults/"+vaultName+"/multipart-uploads/"+uploadID, "")
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var resp struct {
					Parts []any `json:"Parts"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Len(t, resp.Parts, tt.wantParts)
			}
		})
	}
}

// ----------------------------------------
// GetVaultLock
// ----------------------------------------

func TestMultipartUpload_FullRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		vaultName string
	}{
		{
			name:      "initiate_upload_complete",
			vaultName: "roundtrip-vault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			// Create vault
			rec := doRequest(t, h, http.MethodPut, "/-/vaults/"+tt.vaultName, "")
			require.Equal(t, http.StatusCreated, rec.Code)

			// Initiate multipart upload
			headers := map[string]string{
				"X-Amz-Archive-Description": "my-backup",
				"X-Amz-Part-Size":           "4194304",
			}
			rec = doRequestWithHeaders(
				t,
				h,
				http.MethodPost,
				"/-/vaults/"+tt.vaultName+"/multipart-uploads",
				"",
				headers,
			)
			require.Equal(t, http.StatusCreated, rec.Code)
			uploadID := rec.Header().Get("X-Amz-Multipart-Upload-Id")
			require.NotEmpty(t, uploadID)

			// Upload a part
			partBody := strings.Repeat("a", 4194304)
			partChecksum := glacier.ComputeTreeHash([]byte(partBody))
			partHeaders := map[string]string{
				"Content-Range":          "bytes 0-4194303/*",
				"X-Amz-Sha256-Tree-Hash": partChecksum,
			}
			rec = doRequestWithHeaders(
				t, h, http.MethodPut,
				"/-/vaults/"+tt.vaultName+"/multipart-uploads/"+uploadID,
				partBody, partHeaders,
			)
			require.Equal(t, http.StatusNoContent, rec.Code)

			// List parts — should have 1
			rec = doRequest(t, h, http.MethodGet, "/-/vaults/"+tt.vaultName+"/multipart-uploads/"+uploadID, "")
			require.Equal(t, http.StatusOK, rec.Code)

			var partsResp struct {
				Parts []any `json:"Parts"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &partsResp))
			assert.Len(t, partsResp.Parts, 1)

			// List uploads — should have 1
			rec = doRequest(t, h, http.MethodGet, "/-/vaults/"+tt.vaultName+"/multipart-uploads", "")
			require.Equal(t, http.StatusOK, rec.Code)

			var uploadsResp struct {
				UploadsList []any `json:"UploadsList"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &uploadsResp))
			assert.Len(t, uploadsResp.UploadsList, 1)

			// Complete the upload
			completeHeaders := map[string]string{
				"X-Amz-Archive-Size":     "4194304",
				"X-Amz-Sha256-Tree-Hash": partChecksum,
			}
			rec = doRequestWithHeaders(
				t, h, http.MethodPost,
				"/-/vaults/"+tt.vaultName+"/multipart-uploads/"+uploadID,
				"", completeHeaders,
			)
			require.Equal(t, http.StatusCreated, rec.Code)

			var completeResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &completeResp))
			assert.NotEmpty(t, completeResp["archiveId"])

			// After completion, list uploads should be empty
			rec = doRequest(t, h, http.MethodGet, "/-/vaults/"+tt.vaultName+"/multipart-uploads", "")
			require.Equal(t, http.StatusOK, rec.Code)
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &uploadsResp))
			assert.Empty(t, uploadsResp.UploadsList)
		})
	}
}

// ----------------------------------------
// Persistence round-trip for new operations
// ----------------------------------------

func TestAbortMultipartUpload_ThenListParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		vaultName string
	}{
		{
			name:      "abort_removes_upload",
			vaultName: "abort-check-vault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			rec := doRequest(t, h, http.MethodPut, "/-/vaults/"+tt.vaultName, "")
			require.Equal(t, http.StatusCreated, rec.Code)

			rec = doRequestWithHeaders(
				t,
				h,
				http.MethodPost,
				"/-/vaults/"+tt.vaultName+"/multipart-uploads",
				"",
				map[string]string{"X-Amz-Part-Size": "1048576"},
			)
			require.Equal(t, http.StatusCreated, rec.Code)
			uploadID := rec.Header().Get("X-Amz-Multipart-Upload-Id")
			require.NotEmpty(t, uploadID)

			// Abort the upload
			rec = doRequest(t, h, http.MethodDelete, "/-/vaults/"+tt.vaultName+"/multipart-uploads/"+uploadID, "")
			require.Equal(t, http.StatusNoContent, rec.Code)

			// ListParts should now return 404
			rec = doRequest(t, h, http.MethodGet, "/-/vaults/"+tt.vaultName+"/multipart-uploads/"+uploadID, "")
			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

// ----------------------------------------
// Invalid archive size for CompleteMultipartUpload
// ----------------------------------------

func TestCompleteMultipartUpload_InvalidArchiveSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		archSize   string
		wantStatus int
	}{
		{
			name:       "invalid_archive_size_header",
			archSize:   "not-a-number",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			vaultName := "invalid-size-vault"

			rec := doRequest(t, h, http.MethodPut, "/-/vaults/"+vaultName, "")
			require.Equal(t, http.StatusCreated, rec.Code)

			rec = doRequestWithHeaders(
				t,
				h,
				http.MethodPost,
				"/-/vaults/"+vaultName+"/multipart-uploads",
				"",
				map[string]string{"X-Amz-Part-Size": "1048576"},
			)
			require.Equal(t, http.StatusCreated, rec.Code)
			uploadID := rec.Header().Get("X-Amz-Multipart-Upload-Id")
			require.NotEmpty(t, uploadID)

			headers := map[string]string{
				"X-Amz-Archive-Size":     tt.archSize,
				"X-Amz-Sha256-Tree-Hash": "checksum",
			}

			rec = doRequestWithHeaders(
				t, h, http.MethodPost,
				"/-/vaults/"+vaultName+"/multipart-uploads/"+uploadID,
				"", headers,
			)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestInitiateMultipartUpload_RequiresPartSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		partSize   string
		wantStatus int
	}{
		{
			name:       "missing_part_size_rejected",
			partSize:   "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid_1mb_accepted",
			partSize:   "1048576",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "valid_4mb_accepted",
			partSize:   "4194304",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "not_power_of_two_rejected",
			partSize:   "3000000",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "below_1mb_rejected",
			partSize:   "524288",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			createVault(t, h, "mpu-vault")

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost,
				"/"+testAccountID+"/vaults/mpu-vault/multipart-uploads", http.NoBody)
			if tt.partSize != "" {
				req.Header.Set("X-Amz-Part-Size", tt.partSize)
			}
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			err := h.Handler()(c)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// -------------------------------------------------------------------------
// Issue 25: UploadMultipartPart requires valid Content-Range header
// -------------------------------------------------------------------------

func TestIsValidMultipartRange_Unit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "valid_first_part", input: "bytes 0-1048575/*", want: true},
		{name: "valid_second_part", input: "bytes 1048576-2097151/*", want: true},
		{name: "single_byte", input: "bytes 0-0/*", want: true},
		{name: "missing_header", input: "", want: false},
		{name: "missing_bytes_prefix", input: "0-1048575/*", want: false},
		{name: "missing_wildcard", input: "bytes 0-1048575/1048576", want: false},
		{name: "end_before_start", input: "bytes 100-50/*", want: false},
		{name: "negative_start", input: "bytes -1-100/*", want: false},
		{name: "not_numeric", input: "bytes a-b/*", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := glacier.IsValidMultipartRange(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUploadMultipartPart_RequiresContentRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		contentRange string
		wantStatus   int
	}{
		{
			name:         "missing_content_range_rejected",
			contentRange: "",
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "invalid_format_rejected",
			contentRange: "0-1048575",
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "valid_content_range_accepted",
			contentRange: "bytes 0-1048575/*",
			wantStatus:   http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			createVault(t, h, "mpu-part-vault")

			// Initiate a multipart upload first.
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost,
				"/"+testAccountID+"/vaults/mpu-part-vault/multipart-uploads", http.NoBody)
			req.Header.Set("X-Amz-Part-Size", "1048576")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			err := h.Handler()(c)
			require.NoError(t, err)
			require.Equal(t, http.StatusCreated, rec.Code)

			var initResp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initResp))
			uploadID := initResp["uploadId"]
			require.NotEmpty(t, uploadID)

			// Now upload a part.
			req2 := httptest.NewRequest(http.MethodPut,
				"/"+testAccountID+"/vaults/mpu-part-vault/multipart-uploads/"+uploadID,
				strings.NewReader(strings.Repeat("a", 1<<20)))
			if tt.contentRange != "" {
				req2.Header.Set("Content-Range", tt.contentRange)
			}
			rec2 := httptest.NewRecorder()
			c2 := e.NewContext(req2, rec2)
			err = h.Handler()(c2)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, rec2.Code)
		})
	}
}

// -------------------------------------------------------------------------
// Issue 26: CompleteMultipartUpload requires X-Amz-Archive-Size and
//
//	X-Amz-Sha256-Tree-Hash headers
//
// -------------------------------------------------------------------------

func TestCompleteMultipartUpload_RequiresHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		archiveSize    string
		treeHash       string
		wantStatus     int
		uploadRealPart bool
	}{
		{
			name:           "both_headers_present_ok",
			archiveSize:    "1048576",
			uploadRealPart: true,
			wantStatus:     http.StatusCreated,
		},
		{
			name:        "missing_archive_size_rejected",
			archiveSize: "",
			treeHash:    strings.Repeat("a", 64),
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "missing_tree_hash_rejected",
			archiveSize: "1048576",
			treeHash:    "",
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			createVault(t, h, "mpu-complete-vault")

			// Initiate upload.
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost,
				"/"+testAccountID+"/vaults/mpu-complete-vault/multipart-uploads", http.NoBody)
			req.Header.Set("X-Amz-Part-Size", "1048576")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			require.NoError(t, h.Handler()(c))
			require.Equal(t, http.StatusCreated, rec.Code)

			var initResp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initResp))
			uploadID := initResp["uploadId"]
			require.NotEmpty(t, uploadID)

			treeHash := tt.treeHash

			if tt.uploadRealPart {
				partBody := strings.Repeat("a", 1<<20)
				partReq := httptest.NewRequest(http.MethodPut,
					"/"+testAccountID+"/vaults/mpu-complete-vault/multipart-uploads/"+uploadID,
					strings.NewReader(partBody))
				partReq.Header.Set("Content-Range", "bytes 0-1048575/*")
				partRec := httptest.NewRecorder()
				require.NoError(t, h.Handler()(e.NewContext(partReq, partRec)))
				require.Equal(t, http.StatusNoContent, partRec.Code)

				treeHash = glacier.ComputeTreeHash([]byte(partBody))
			}

			// Complete the upload.
			req2 := httptest.NewRequest(http.MethodPost,
				"/"+testAccountID+"/vaults/mpu-complete-vault/multipart-uploads/"+uploadID,
				http.NoBody)
			if tt.archiveSize != "" {
				req2.Header.Set("X-Amz-Archive-Size", tt.archiveSize)
			}
			if treeHash != "" {
				req2.Header.Set("X-Amz-Sha256-Tree-Hash", treeHash)
			}
			rec2 := httptest.NewRecorder()
			c2 := e.NewContext(req2, rec2)
			require.NoError(t, h.Handler()(c2))
			assert.Equal(t, tt.wantStatus, rec2.Code)
		})
	}
}

// -------------------------------------------------------------------------
// Issue 27: SetVaultNotifications requires non-empty SNSTopic and Events
// -------------------------------------------------------------------------

func TestListMultipartUploads_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	createVault(t, h, "mpu-page-vault")

	// Create 4 in-progress multipart uploads.
	for range 4 {
		rec := doRequestWithBody(t, h, http.MethodPost,
			"/"+testAccountID+"/vaults/mpu-page-vault/multipart-uploads",
			"", map[string]string{"X-Amz-Part-Size": "1048576"})
		require.Equal(t, http.StatusCreated, rec.Code)
	}

	rec := doRequest(t, h, http.MethodGet,
		"/"+testAccountID+"/vaults/mpu-page-vault/multipart-uploads?limit=2", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	uploads := resp["UploadsList"].([]any)
	assert.Len(t, uploads, 2)

	marker, hasMarker := resp["Marker"]
	assert.True(t, hasMarker, "Marker should be set")
	assert.NotEmpty(t, marker)
}

func TestListMultipartUploads_LimitValidation(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	createVault(t, h, "mpu-lim-vault")

	tests := []struct {
		name       string
		limit      string
		wantStatus int
	}{
		{name: "limit_0_rejected", limit: "0", wantStatus: http.StatusBadRequest},
		{name: "limit_1001_rejected", limit: "1001", wantStatus: http.StatusBadRequest},
		{name: "limit_1_ok", limit: "1", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, http.MethodGet,
				"/"+testAccountID+"/vaults/mpu-lim-vault/multipart-uploads?limit="+tt.limit, "")
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

// -------------------------------------------------------------------------
// Issue 18: DataRetrievalPolicy strategy validation
// -------------------------------------------------------------------------

func TestListParts_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	createVault(t, h, "lp-vault")

	// Initiate a multipart upload.
	rec := doRequestWithBody(t, h, http.MethodPost,
		"/"+testAccountID+"/vaults/lp-vault/multipart-uploads",
		"", map[string]string{"X-Amz-Part-Size": "1048576"})
	require.Equal(t, http.StatusCreated, rec.Code)

	var upResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &upResp))
	uploadID := upResp["uploadId"].(string)

	// Upload 3 parts.
	partSize := 1048576
	for i := range 3 {
		start := i * partSize
		end := start + partSize - 1
		rangeHeader := fmt.Sprintf("bytes %d-%d/*", start, end)
		rec = doRequestWithBody(t, h, http.MethodPut,
			"/"+testAccountID+"/vaults/lp-vault/multipart-uploads/"+uploadID,
			strings.Repeat("x", partSize),
			map[string]string{"Content-Range": rangeHeader})
		require.Equal(t, http.StatusNoContent, rec.Code)
	}

	// List all 3 parts.
	rec = doRequest(t, h, http.MethodGet,
		"/"+testAccountID+"/vaults/lp-vault/multipart-uploads/"+uploadID, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var partsResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &partsResp))
	parts := partsResp["Parts"].([]any)
	require.Len(t, parts, 3)

	// List with limit=1.
	rec = doRequest(t, h, http.MethodGet,
		"/"+testAccountID+"/vaults/lp-vault/multipart-uploads/"+uploadID+"?limit=1", "")
	require.Equal(t, http.StatusOK, rec.Code)

	var pPage1 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pPage1))
	assert.Len(t, pPage1["Parts"].([]any), 1)
	assert.NotEmpty(t, pPage1["Marker"])
}

// -------------------------------------------------------------------------
// Additional: RetrievalByteRange stored on job
// -------------------------------------------------------------------------

func TestListParts_PartsAlwaysArray(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		seedParts bool
	}{
		{
			name:      "no_parts_returns_empty_array",
			seedParts: false,
		},
		{
			name:      "with_parts_returns_array",
			seedParts: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			createVault(t, h, "listparts-audit-vault")

			e := echo.New()
			req := httptest.NewRequest(http.MethodPost,
				"/"+testAccountID+"/vaults/listparts-audit-vault/multipart-uploads", http.NoBody)
			req.Header.Set("X-Amz-Part-Size", "1048576")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			require.NoError(t, h.Handler()(c))
			require.Equal(t, http.StatusCreated, rec.Code)

			var initResp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initResp))
			uploadID := initResp["uploadId"]
			require.NotEmpty(t, uploadID)

			if tt.seedParts {
				req2 := httptest.NewRequest(http.MethodPut,
					"/"+testAccountID+"/vaults/listparts-audit-vault/multipart-uploads/"+uploadID,
					strings.NewReader(strings.Repeat("a", 1<<20)))
				req2.Header.Set("Content-Range", "bytes 0-1048575/*")
				rec2 := httptest.NewRecorder()
				c2 := e.NewContext(req2, rec2)
				require.NoError(t, h.Handler()(c2))
				require.Equal(t, http.StatusNoContent, rec2.Code)
			}

			listRec := doRequest(t, h, http.MethodGet,
				"/"+testAccountID+"/vaults/listparts-audit-vault/multipart-uploads/"+uploadID, "")
			require.Equal(t, http.StatusOK, listRec.Code)

			var resp map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &resp))

			partsRaw, present := resp["Parts"]
			assert.True(t, present, "Parts key must be present in ListParts response")
			assert.NotEqual(t, "null", string(partsRaw),
				"Parts must never be null (use [] for empty)")

			if !tt.seedParts {
				assert.Equal(t, "[]", string(partsRaw),
					"Parts must be [] when no parts have been uploaded")
			}
		})
	}
}

// -------------------------------------------------------------------------
// Issue 32: DescribeJob returns SNSTopic when job was initiated with one
// -------------------------------------------------------------------------

func TestMultipartUpload_FullLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description string
		partSize    string
	}{
		{name: "two_parts_complete", description: "my big file", partSize: "1048576"},
		{name: "no_description", description: "", partSize: "4194304"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			createVault(t, h, "mp-lifecycle-"+tt.name)

			// Initiate.
			headers := map[string]string{"X-Amz-Part-Size": tt.partSize}
			if tt.description != "" {
				headers["X-Amz-Archive-Description"] = tt.description
			}
			rec := doRequestWithHeaders(t, h, http.MethodPost,
				"/"+testAccountID+"/vaults/mp-lifecycle-"+tt.name+"/multipart-uploads",
				"", headers)
			require.Equal(t, http.StatusCreated, rec.Code)
			var initResp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initResp))
			uploadID := initResp["uploadId"]
			require.NotEmpty(t, uploadID)
			// Location header must be set.
			assert.Contains(t, rec.Header().Get("Location"), uploadID)
			assert.Equal(t, uploadID, rec.Header().Get("X-Amz-Multipart-Upload-Id"))

			// Upload part 1.
			part1Data := strings.Repeat("a", 1<<20)
			e := echo.New()
			reqP := httptest.NewRequest(http.MethodPut,
				"/"+testAccountID+"/vaults/mp-lifecycle-"+tt.name+"/multipart-uploads/"+uploadID,
				strings.NewReader(part1Data))
			reqP.Header.Set("Content-Range", "bytes 0-1048575/*")
			recP := httptest.NewRecorder()
			cp := e.NewContext(reqP, recP)
			require.NoError(t, h.Handler()(cp))
			require.Equal(t, http.StatusNoContent, recP.Code)

			// List parts: should have one.
			rec2 := doRequestWithHeaders(t, h, http.MethodGet,
				"/"+testAccountID+"/vaults/mp-lifecycle-"+tt.name+"/multipart-uploads/"+uploadID,
				"", nil)
			require.Equal(t, http.StatusOK, rec2.Code)
			var listParts map[string]any
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &listParts))
			parts := listParts["Parts"].([]any)
			assert.Len(t, parts, 1)

			// Complete.
			checksum := glacier.ComputeTreeHash([]byte(part1Data))
			rec3 := doRequestWithHeaders(t, h, http.MethodPost,
				"/"+testAccountID+"/vaults/mp-lifecycle-"+tt.name+"/multipart-uploads/"+uploadID,
				"", map[string]string{
					"X-Amz-Archive-Size":     "1048576",
					"X-Amz-Sha256-Tree-Hash": checksum,
				})
			require.Equal(t, http.StatusCreated, rec3.Code)
			var completeResp map[string]string
			require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &completeResp))
			archiveID := completeResp["archiveId"]
			assert.NotEmpty(t, archiveID)
			assert.Equal(t, checksum, rec3.Header().Get("X-Amz-Sha256-Tree-Hash"))

			// Upload no longer listed after completion.
			rec4 := doRequestWithHeaders(t, h, http.MethodGet,
				"/"+testAccountID+"/vaults/mp-lifecycle-"+tt.name+"/multipart-uploads", "", nil)
			require.Equal(t, http.StatusOK, rec4.Code)
			var listUploads map[string]any
			require.NoError(t, json.Unmarshal(rec4.Body.Bytes(), &listUploads))
			uploads := listUploads["UploadsList"].([]any)
			assert.Empty(t, uploads)

			// Vault now has the archive. NumberOfArchives only reports the
			// as-of-last-inventory count (gopherstack-zpo5), so an inventory
			// must run before the completed archive is wire-visible.
			invRec := doRequestWithHeaders(t, h, http.MethodPost,
				"/"+testAccountID+"/vaults/mp-lifecycle-"+tt.name+"/jobs",
				`{"Type":"inventory-retrieval"}`, nil)
			require.Equal(t, http.StatusAccepted, invRec.Code)

			descVault := doRequestWithHeaders(t, h, http.MethodGet,
				"/"+testAccountID+"/vaults/mp-lifecycle-"+tt.name, "", nil)
			require.Equal(t, http.StatusOK, descVault.Code)
			var vaultDesc map[string]any
			require.NoError(t, json.Unmarshal(descVault.Body.Bytes(), &vaultDesc))
			assert.EqualValues(t, 1, vaultDesc["NumberOfArchives"])
		})
	}
}

func TestMultipartUpload_AbortLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "abort_clears_upload"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			createVault(t, h, "mp-abort-vault")

			rec := doRequestWithHeaders(t, h, http.MethodPost,
				"/"+testAccountID+"/vaults/mp-abort-vault/multipart-uploads",
				"", map[string]string{"X-Amz-Part-Size": "1048576"})
			require.Equal(t, http.StatusCreated, rec.Code)
			var initResp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initResp))
			uploadID := initResp["uploadId"]

			// Abort.
			rec2 := doRequestWithHeaders(t, h, http.MethodDelete,
				"/"+testAccountID+"/vaults/mp-abort-vault/multipart-uploads/"+uploadID, "", nil)
			require.Equal(t, http.StatusNoContent, rec2.Code)

			// ListMultipartUploads should be empty.
			rec3 := doRequestWithHeaders(t, h, http.MethodGet,
				"/"+testAccountID+"/vaults/mp-abort-vault/multipart-uploads", "", nil)
			require.Equal(t, http.StatusOK, rec3.Code)
			var listResp map[string]any
			require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &listResp))
			uploads := listResp["UploadsList"].([]any)
			assert.Empty(t, uploads, tt.name)

			// Listing parts for aborted upload returns 404.
			rec4 := doRequestWithHeaders(t, h, http.MethodGet,
				"/"+testAccountID+"/vaults/mp-abort-vault/multipart-uploads/"+uploadID, "", nil)
			assert.Equal(t, http.StatusNotFound, rec4.Code)
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 14. Error response fidelity
// ─────────────────────────────────────────────────────────────────────────────

func TestListParts_MarkerNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "parts_empty_on_unknown_marker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			createVault(t, h, "parts-marker-vault")

			// Initiate multipart upload.
			rec := doRequestWithHeaders(t, h, http.MethodPost,
				"/"+testAccountID+"/vaults/parts-marker-vault/multipart-uploads", "",
				map[string]string{"X-Amz-Part-Size": "1048576"})
			require.Equal(t, http.StatusCreated, rec.Code)
			var initResp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initResp))
			uploadID := initResp["uploadId"]
			require.NotEmpty(t, uploadID)

			// List parts with unknown marker.
			rec2 := doRequestWithHeaders(t, h, http.MethodGet,
				"/"+testAccountID+"/vaults/parts-marker-vault/multipart-uploads/"+uploadID+"?marker=nonexistent",
				"", nil)
			require.Equal(t, http.StatusOK, rec2.Code)

			var resp map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
			partsRaw := resp["Parts"]
			assert.Equal(t, "[]", string(partsRaw), tt.name)
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 8. Tag limit and validation
// ─────────────────────────────────────────────────────────────────────────────

func TestListParts_MarkerPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantParts int
	}{
		{name: "two_parts_paginated_by_marker", wantParts: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			createVault(t, h, "listparts-pag-vault")

			// Initiate.
			rec := doRequestWithHeaders(t, h, http.MethodPost,
				"/"+testAccountID+"/vaults/listparts-pag-vault/multipart-uploads",
				"", map[string]string{"X-Amz-Part-Size": "1048576"})
			require.Equal(t, http.StatusCreated, rec.Code)
			var initResp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &initResp))
			uploadID := initResp["uploadId"]

			// Upload 2 parts.
			e := echo.New()
			for i := range tt.wantParts {
				start := i * (1 << 20)
				end := start + (1 << 20) - 1
				rangeHdr := fmt.Sprintf("bytes %d-%d/*", start, end)
				req := httptest.NewRequest(http.MethodPut,
					"/"+testAccountID+"/vaults/listparts-pag-vault/multipart-uploads/"+uploadID,
					strings.NewReader(strings.Repeat("x", 1<<20)))
				req.Header.Set("Content-Range", rangeHdr)
				rec2 := httptest.NewRecorder()
				c := e.NewContext(req, rec2)
				require.NoError(t, h.Handler()(c))
				require.Equal(t, http.StatusNoContent, rec2.Code)
			}

			// List first part with limit=1.
			rec = doRequestWithHeaders(t, h, http.MethodGet,
				"/"+testAccountID+"/vaults/listparts-pag-vault/multipart-uploads/"+uploadID+"?limit=1",
				"", nil)
			require.Equal(t, http.StatusOK, rec.Code)
			var page1 map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page1))
			parts1 := page1["Parts"].([]any)
			require.Len(t, parts1, 1)
			markerVal, hasMarker := page1["Marker"].(string)
			assert.True(t, hasMarker, "Marker must be set when there are more parts")

			// Second page using marker (URL-encode because RangeInBytes may contain spaces).
			secondPage := "/" + testAccountID + "/vaults/listparts-pag-vault/multipart-uploads/" +
				uploadID + "?limit=1&marker=" + url.QueryEscape(markerVal)
			rec = doRequestWithHeaders(t, h, http.MethodGet, secondPage, "", nil)
			require.Equal(t, http.StatusOK, rec.Code)
			var page2 map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page2))
			parts2 := page2["Parts"].([]any)
			assert.Len(t, parts2, 1)
			_, hasMarker2 := page2["Marker"]
			assert.False(t, hasMarker2, "no Marker on last page")
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 24. DescribeJob for non-existent vault/job
// ─────────────────────────────────────────────────────────────────────────────
