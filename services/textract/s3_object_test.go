package textract_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/textract"
)

// fakeS3Backend is a minimal textract.S3Backend test double: HeadObject
// errors for any bucket/key not in objects, standing in for a wired S3
// backend without depending on services/s3.
type fakeS3Backend struct {
	objects map[string]bool // "bucket/key" -> exists
}

var errFakeNoSuchKey = errors.New("NoSuchKey")

func (f *fakeS3Backend) HeadObject(
	_ context.Context,
	input *sdk_s3.HeadObjectInput,
) (*sdk_s3.HeadObjectOutput, error) {
	key := aws.ToString(input.Bucket) + "/" + aws.ToString(input.Key)
	if f.objects[key] {
		return &sdk_s3.HeadObjectOutput{}, nil
	}

	return nil, errFakeNoSuchKey
}

func newWiredHandler(t *testing.T, objects map[string]bool) *textract.Handler {
	t.Helper()

	b := textract.NewInMemoryBackendSync("123456789012", "us-east-1")
	b.SetS3Backend(&fakeS3Backend{objects: objects})

	return textract.NewHandler(b)
}

// TestSyncDocumentOps_S3ObjectValidation proves that once S3 is wired via
// SetS3Backend, a sync document op rejects an S3Object naming a bucket/key
// that does not exist (InvalidS3ObjectException) and allows one that does.
// Fails against the pre-fix code: these ops never called into S3 at all, so
// a nonexistent object was silently accepted.
func TestSyncDocumentOps_S3ObjectValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   func(bucket, key string) map[string]any
		name   string
		action string
	}{
		{
			name:   "analyze_document",
			action: "AnalyzeDocument",
			body: func(bucket, key string) map[string]any {
				return map[string]any{
					"Document": map[string]any{
						"S3Object": map[string]any{"Bucket": bucket, "Name": key},
					},
					"FeatureTypes": []string{"TABLES"},
				}
			},
		},
		{
			name:   "analyze_expense",
			action: "AnalyzeExpense",
			body: func(bucket, key string) map[string]any {
				return map[string]any{"Document": map[string]any{
					"S3Object": map[string]any{"Bucket": bucket, "Name": key},
				}}
			},
		},
		{
			name:   "detect_document_text",
			action: "DetectDocumentText",
			body: func(bucket, key string) map[string]any {
				return map[string]any{"Document": map[string]any{
					"S3Object": map[string]any{"Bucket": bucket, "Name": key},
				}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newWiredHandler(t, map[string]bool{"good-bucket/file.pdf": true})

			rec := doTextractRequest(t, h, tt.action, tt.body("missing-bucket", "file.pdf"))
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "InvalidS3ObjectException", resp["__type"])

			rec = doTextractRequest(t, h, tt.action, tt.body("good-bucket", "file.pdf"))
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

// TestAnalyzeID_S3ObjectValidation proves AnalyzeID applies the same check
// to every entry of DocumentPages, not just the first.
func TestAnalyzeID_S3ObjectValidation(t *testing.T) {
	t.Parallel()

	h := newWiredHandler(t, map[string]bool{"good-bucket/front.jpg": true, "good-bucket/back.jpg": true})

	rec := doTextractRequest(t, h, "AnalyzeID", map[string]any{
		"DocumentPages": []map[string]any{
			{"S3Object": map[string]any{"Bucket": "good-bucket", "Name": "front.jpg"}},
			{"S3Object": map[string]any{"Bucket": "missing-bucket", "Name": "back.jpg"}},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "InvalidS3ObjectException", resp["__type"])

	rec = doTextractRequest(t, h, "AnalyzeID", map[string]any{
		"DocumentPages": []map[string]any{
			{"S3Object": map[string]any{"Bucket": "good-bucket", "Name": "front.jpg"}},
			{"S3Object": map[string]any{"Bucket": "good-bucket", "Name": "back.jpg"}},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStartOps_S3ObjectValidation proves the async Start* ops apply the same
// DocumentLocation.S3Object check as the sync ops above.
func TestStartOps_S3ObjectValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		action       string
		featureTypes []string
	}{
		{name: "start_document_analysis", action: "StartDocumentAnalysis", featureTypes: []string{"TABLES"}},
		{name: "start_document_text_detection", action: "StartDocumentTextDetection"},
		{name: "start_expense_analysis", action: "StartExpenseAnalysis"},
		{name: "start_lending_analysis", action: "StartLendingAnalysis"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newWiredHandler(t, map[string]bool{"good-bucket/file.pdf": true})

			body := func(bucket string) map[string]any {
				b := map[string]any{"DocumentLocation": map[string]any{
					"S3Object": map[string]any{"Bucket": bucket, "Name": "file.pdf"},
				}}
				if tt.featureTypes != nil {
					b["FeatureTypes"] = tt.featureTypes
				}

				return b
			}

			rec := doTextractRequest(t, h, tt.action, body("missing-bucket"))
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "InvalidS3ObjectException", resp["__type"])

			rec = doTextractRequest(t, h, tt.action, body("good-bucket"))
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

// TestAnalyzeDocument_UnwiredS3StaysPermissive proves the unwired path stays
// permissive: with no SetS3Backend call, a nonexistent S3Object must still
// be accepted, matching this repo's unwired-hook-stays-permissive
// convention (roughly 150 services construct backends in tests with no
// cross-service hooks wired).
func TestAnalyzeDocument_UnwiredS3StaysPermissive(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doTextractRequest(t, h, "AnalyzeDocument", map[string]any{
		"Document": map[string]any{
			"S3Object": map[string]any{"Bucket": "nonexistent-bucket", "Name": "file.pdf"},
		},
		"FeatureTypes": []string{"TABLES"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStartDocumentAnalysis_UnwiredS3StaysPermissive is
// TestAnalyzeDocument_UnwiredS3StaysPermissive for the async Start* path.
func TestStartDocumentAnalysis_UnwiredS3StaysPermissive(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doTextractRequest(t, h, "StartDocumentAnalysis", map[string]any{
		"DocumentLocation": map[string]any{
			"S3Object": map[string]any{"Bucket": "nonexistent-bucket", "Name": "file.pdf"},
		},
		"FeatureTypes": []string{"TABLES"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// createAdapterForS3Test creates an adapter and returns its AdapterId, so
// CreateAdapterVersion tests have a valid parent to attach a version to.
func createAdapterForS3Test(t *testing.T, h *textract.Handler) string {
	t.Helper()

	rec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "s3-validation-adapter",
		"FeatureTypes": []string{"FORMS"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	return resp["AdapterId"]
}

// createAdapterVersionBody builds a CreateAdapterVersion request body whose
// DatasetConfig.ManifestS3Object names bucket/key.
func createAdapterVersionBody(adapterID, bucket, key string) map[string]any {
	return map[string]any{
		"AdapterId": adapterID,
		"DatasetConfig": map[string]any{
			"ManifestS3Object": map[string]any{"Bucket": bucket, "Name": key},
		},
		"OutputConfig": map[string]any{"S3Bucket": "test-output-bucket"},
	}
}

// TestCreateAdapterVersion_S3ObjectValidation proves that once S3 is wired,
// CreateAdapterVersion rejects a DatasetConfig.ManifestS3Object naming a
// bucket/key that does not exist and accepts one that does. Fails against
// the pre-fix code: DatasetConfig.ManifestS3Object was never checked against
// S3 at all.
func TestCreateAdapterVersion_S3ObjectValidation(t *testing.T) {
	t.Parallel()

	h := newWiredHandler(t, map[string]bool{"good-bucket/manifest.json": true})
	adapterID := createAdapterForS3Test(t, h)

	rec := doTextractRequest(t, h, "CreateAdapterVersion",
		createAdapterVersionBody(adapterID, "missing-bucket", "manifest.json"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "InvalidS3ObjectException", resp["__type"])

	rec = doTextractRequest(t, h, "CreateAdapterVersion",
		createAdapterVersionBody(adapterID, "good-bucket", "manifest.json"))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestCreateAdapterVersion_UnwiredS3StaysPermissive proves that with no
// SetS3Backend call, a nonexistent ManifestS3Object is still accepted,
// matching this repo's unwired-hook-stays-permissive convention.
func TestCreateAdapterVersion_UnwiredS3StaysPermissive(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	adapterID := createAdapterForS3Test(t, h)

	rec := doTextractRequest(t, h, "CreateAdapterVersion",
		createAdapterVersionBody(adapterID, "nonexistent-bucket", "manifest.json"))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestAnalyzeDocument_InlineBytesUnaffected proves a Document carrying
// inline Bytes instead of an S3Object is never subject to the S3 existence
// check, even when S3 is wired.
func TestAnalyzeDocument_InlineBytesUnaffected(t *testing.T) {
	t.Parallel()

	h := newWiredHandler(t, map[string]bool{})

	rec := doTextractRequest(t, h, "AnalyzeDocument", map[string]any{
		"Document":     map[string]any{"Bytes": []byte("fake-image-bytes")},
		"FeatureTypes": []string{"TABLES"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}
