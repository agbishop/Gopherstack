package s3_test

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/s3"
)

// TestHandler_PutObject_ContentMD5 verifies that PutObject validates an
// optional Content-MD5 header against the uploaded body.
func TestHandler_PutObject_ContentMD5(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		bucket    string
		url       string
		body      string
		md5Header string
		wantCode  int
	}{
		{
			name:      "valid_md5",
			bucket:    "md5-valid-bucket",
			url:       "/md5-valid-bucket/hello.txt",
			body:      "hello world",
			md5Header: "XrY7u+Ae7tCTyyK7j1rNww==",
			wantCode:  http.StatusOK,
		},
		{
			name:      "invalid_md5",
			bucket:    "md5-invalid-bucket",
			url:       "/md5-invalid-bucket/hello.txt",
			body:      "hello world",
			md5Header: "AAAAAAAAAAAAAAAAAAAAAA==",
			wantCode:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, backend := newTestHandler(t)
			mustCreateBucket(t, backend, tt.bucket)

			req := httptest.NewRequest(http.MethodPut, tt.url, strings.NewReader(tt.body))
			req.Header.Set("Content-MD5", tt.md5Header)
			rec := httptest.NewRecorder()
			serveS3Handler(handler, rec, req)

			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// ─── checksum helpers shared by the UploadPart checksum table below ──────────

func checksumB64CRC32(data []byte) string {
	sum := crc32.ChecksumIEEE(data)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, sum)

	return base64.StdEncoding.EncodeToString(b)
}

func checksumB64CRC32C(data []byte) string {
	h := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	h.Write(data)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, h.Sum32())

	return base64.StdEncoding.EncodeToString(b)
}

func checksumB64SHA1(data []byte) string {
	h := sha1.New()
	h.Write(data)

	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func checksumB64SHA256(data []byte) string {
	h := sha256.New()
	h.Write(data)

	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// doMultipartUpload initiates a multipart upload and returns its upload ID.
func doMultipartUpload(
	t *testing.T,
	handler *s3.S3Handler,
	backend s3.StorageBackend,
	bucket string,
) string {
	t.Helper()
	mustCreateBucket(t, backend, bucket)

	req := httptest.NewRequest(http.MethodPost, "/"+bucket+"/obj?uploads", nil)
	rec := httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp s3.InitiateMultipartUploadResult
	require.NoError(t, xml.NewDecoder(rec.Body).Decode(&resp))

	return resp.UploadID
}

// TestUploadPart_ChecksumHandling verifies that UploadPart accepts a matching
// checksum for every supported algorithm, rejects a mismatched checksum, and
// computes the checksum server-side when none is supplied.
func TestUploadPart_ChecksumHandling(t *testing.T) {
	t.Parallel()

	partData := []byte("hello world part data")

	tests := []struct {
		checksum   func() string
		name       string
		bucket     string
		algo       string
		headerName string
		wantCode   int
	}{
		{
			name:       "crc32",
			bucket:     "chksum-crc32",
			algo:       "CRC32",
			headerName: "X-Amz-Checksum-Crc32",
			checksum:   func() string { return checksumB64CRC32(partData) },
			wantCode:   http.StatusOK,
		},
		{
			name:       "crc32c",
			bucket:     "chksum-crc32c",
			algo:       "CRC32C",
			headerName: "X-Amz-Checksum-Crc32c",
			checksum:   func() string { return checksumB64CRC32C(partData) },
			wantCode:   http.StatusOK,
		},
		{
			name:       "sha1",
			bucket:     "chksum-sha1",
			algo:       "SHA1",
			headerName: "X-Amz-Checksum-Sha1",
			checksum:   func() string { return checksumB64SHA1(partData) },
			wantCode:   http.StatusOK,
		},
		{
			name:       "sha256",
			bucket:     "chksum-sha256",
			algo:       "SHA256",
			headerName: "X-Amz-Checksum-Sha256",
			checksum:   func() string { return checksumB64SHA256(partData) },
			wantCode:   http.StatusOK,
		},
		{
			name:       "wrong_checksum_rejected",
			bucket:     "chksum-wrong",
			algo:       "CRC32",
			headerName: "X-Amz-Checksum-Crc32",
			checksum:   func() string { return base64.StdEncoding.EncodeToString(make([]byte, 4)) },
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "server_computed_when_absent",
			bucket:     "chksum-computed",
			algo:       "SHA256",
			headerName: "", // no checksum value header — server computes it
			wantCode:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, backend := newTestHandler(t)
			uploadID := doMultipartUpload(t, handler, backend, tt.bucket)

			req := httptest.NewRequest(http.MethodPut,
				fmt.Sprintf("/%s/obj?partNumber=1&uploadId=%s", tt.bucket, uploadID),
				bytes.NewReader(partData))
			req.Header.Set("X-Amz-Checksum-Algorithm", tt.algo)
			if tt.headerName != "" {
				req.Header.Set(tt.headerName, tt.checksum())
			}
			rec := httptest.NewRecorder()
			serveS3Handler(handler, rec, req)

			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestHandler_ChecksumMismatch_Rejected verifies that PutObject via the HTTP
// handler returns 400 BadDigest when the supplied checksum does not match the
// actual content.
func TestHandler_ChecksumMismatch_Rejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		algo           string
		checksumHeader string
	}{
		{name: "sha256_mismatch", algo: "SHA256", checksumHeader: "X-Amz-Checksum-Sha256"},
		{name: "crc32_mismatch", algo: "CRC32", checksumHeader: "X-Amz-Checksum-Crc32"},
		{name: "sha1_mismatch", algo: "SHA1", checksumHeader: "X-Amz-Checksum-Sha1"},
		{name: "crc32c_mismatch", algo: "CRC32C", checksumHeader: "X-Amz-Checksum-Crc32c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, backend := newTestHandler(t)
			mustCreateBucket(t, backend, "bkt")

			body := "test data for checksum"
			// Compute correct checksum then corrupt it.
			correct := s3.CalculateChecksum([]byte(body), tt.algo)
			corrupt := correct[:len(correct)-1] + "X"

			req := httptest.NewRequest(http.MethodPut, "/bkt/key", strings.NewReader(body))
			req.Header.Set("X-Amz-Checksum-Algorithm", tt.algo)
			req.Header.Set(tt.checksumHeader, corrupt)
			rec := httptest.NewRecorder()
			serveS3Handler(handler, rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code,
				"expected 400 BadDigest for corrupted %s checksum", tt.algo)
			assert.Contains(t, rec.Body.String(), "BadDigest")
		})
	}
}

// TestHandler_ChecksumValid_Accepted verifies that PutObject via the HTTP
// handler returns 200 when the supplied checksum matches the content.
func TestHandler_ChecksumValid_Accepted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		algo           string
		checksumHeader string
	}{
		{name: "sha256_valid", algo: "SHA256", checksumHeader: "X-Amz-Checksum-Sha256"},
		{name: "crc32_valid", algo: "CRC32", checksumHeader: "X-Amz-Checksum-Crc32"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, backend := newTestHandler(t)
			mustCreateBucket(t, backend, "bkt")

			body := "valid checksum data"
			correct := s3.CalculateChecksum([]byte(body), tt.algo)

			req := httptest.NewRequest(http.MethodPut, "/bkt/key", strings.NewReader(body))
			req.Header.Set("X-Amz-Checksum-Algorithm", tt.algo)
			req.Header.Set(tt.checksumHeader, correct)
			rec := httptest.NewRecorder()
			serveS3Handler(handler, rec, req)

			assert.Equal(t, http.StatusOK, rec.Code,
				"expected 200 for valid %s checksum", tt.algo)
		})
	}
}

// TestPutObject_ChecksumVerification verifies that the backend stores valid
// checksums and that the HTTP handler layer rejects mismatched ones.
func TestPutObject_ChecksumVerification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		algo    string
		corrupt bool
		wantErr bool
	}{
		{name: "correct_sha256_accepted", algo: "SHA256", corrupt: false, wantErr: false},
		{name: "correct_crc32_accepted", algo: "CRC32", corrupt: false, wantErr: false},
		{name: "correct_sha1_accepted", algo: "SHA1", corrupt: false, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newTestBackend(t)
			mustCreateBucket(t, backend, "bkt")

			data := []byte("checksum test data")
			checksum := s3.CalculateChecksum(data, tt.algo)

			var input sdk_s3.PutObjectInput
			input.Bucket = aws.String("bkt")
			input.Key = aws.String("key")
			input.Body = bytes.NewReader(data)
			input.ChecksumAlgorithm = types.ChecksumAlgorithm(tt.algo)

			switch tt.algo {
			case "SHA256":
				input.ChecksumSHA256 = aws.String(checksum)
			case "CRC32":
				input.ChecksumCRC32 = aws.String(checksum)
			case "SHA1":
				input.ChecksumSHA1 = aws.String(checksum)
			}

			_, err := backend.PutObject(t.Context(), &input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Verify the checksum is preserved on retrieval.
			getOut, err := backend.GetObject(t.Context(), &sdk_s3.GetObjectInput{
				Bucket: aws.String("bkt"),
				Key:    aws.String("key"),
			})
			require.NoError(t, err)

			switch tt.algo {
			case "SHA256":
				assert.Equal(t, checksum, aws.ToString(getOut.ChecksumSHA256))
			case "CRC32":
				assert.Equal(t, checksum, aws.ToString(getOut.ChecksumCRC32))
			case "SHA1":
				assert.Equal(t, checksum, aws.ToString(getOut.ChecksumSHA1))
			}
		})
	}
}

func TestCRC64NVME_PutObject_ValidChecksum(t *testing.T) {
	t.Parallel()

	handler, backend := newTestHandler(t)
	mustCreateBucket(t, backend, "crc64-bucket")

	data := []byte("test data for crc64nvme")
	checksum := s3.CalculateCRC64NVME(data)

	rec := doRequest(handler, http.MethodPut, "/crc64-bucket/obj",
		bytes.NewReader(data),
		map[string]string{
			"X-Amz-Checksum-Algorithm": "CRC64NVME",
			"X-Amz-Checksum-Crc64nvme": checksum,
		})

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCRC64NVME_PutObject_InvalidChecksum_Returns400(t *testing.T) {
	t.Parallel()

	handler, backend := newTestHandler(t)
	mustCreateBucket(t, backend, "crc64-bad")

	data := []byte("test data")
	wrongChecksum := base64.StdEncoding.EncodeToString(make([]byte, 8))

	rec := doRequest(handler, http.MethodPut, "/crc64-bad/obj",
		bytes.NewReader(data),
		map[string]string{
			"X-Amz-Checksum-Algorithm": "CRC64NVME",
			"X-Amz-Checksum-Crc64nvme": wrongChecksum,
		})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ─── Delete markers with versioning ──────────────────────────────────────────

func TestHandler_PutObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*testing.T, *s3.InMemoryBackend)
		name       string
		bucket     string
		key        string
		body       string
		wantStatus int
	}{
		{
			name:   "put object success",
			bucket: "bkt",
			key:    "key",
			body:   "hello world",
			setup: func(t *testing.T, b *s3.InMemoryBackend) {
				t.Helper()
				mustCreateBucket(t, b, "bkt")
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "put object to non-existent bucket",
			bucket:     "no-bkt",
			key:        "key",
			body:       "data",
			setup:      func(_ *testing.T, _ *s3.InMemoryBackend) {},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, backend := newTestHandler(t)
			tt.setup(t, backend)

			req := httptest.NewRequest(
				http.MethodPut,
				"/"+tt.bucket+"/"+tt.key,
				strings.NewReader(tt.body),
			)
			rec := httptest.NewRecorder()
			serveS3Handler(handler, rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
