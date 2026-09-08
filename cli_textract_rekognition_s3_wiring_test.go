package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/portalloc"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	rekognitionbackend "github.com/blackbirdworks/gopherstack/services/rekognition"
	s3backend "github.com/blackbirdworks/gopherstack/services/s3"
	textractbackend "github.com/blackbirdworks/gopherstack/services/textract"
)

// doJSONTargetRequest sends a JSON-protocol request (X-Amz-Target routing)
// to h and returns the decoded response body alongside the HTTP status.
func doJSONTargetRequest(
	t *testing.T,
	h echo.HandlerFunc,
	target string,
	body map[string]any,
) (int, map[string]any) {
	t.Helper()

	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	require.NoError(t, h(c))

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	return rec.Code, resp
}

// newTestAppContext builds a minimal service.AppContext for driving the real
// initializeServices composition root, matching the pattern used by the
// other cli_*_wiring_test.go files (e.g. TestInitializeServices_LambdaS3CodeWiring).
func newTestAppContext(t *testing.T, portFrom, portTo int) *service.AppContext {
	t.Helper()

	cli := &CLI{AccountID: "000000000000", Region: "us-east-1", faultStore: chaos.NewFaultStore()}
	portAlloc, err := portalloc.New(portFrom, portTo)
	require.NoError(t, err)

	return &service.AppContext{
		Logger:     slog.Default(),
		Config:     cli,
		JanitorCtx: t.Context(),
		PortAlloc:  portAlloc,
	}
}

// TestInitializeServices_TextractS3Wiring drives the real composition root
// (initializeServices) rather than invoking wireTextractS3 directly, so
// deleting the wiring call from wireStorageAndSecretsIntegrations -- not
// just breaking the helper function itself -- is what this test is
// sensitive to (gopherstack-eshx).
func TestInitializeServices_TextractS3Wiring(t *testing.T) {
	t.Parallel()

	appCtx := newTestAppContext(t, 19400, 19500)

	services, err := initializeServices(appCtx)
	require.NoError(t, err)

	byName := serviceByName(services)

	trH, ok := byName["Textract"].(*textractbackend.Handler)
	require.True(t, ok, "Textract handler must be registered")

	s3H, ok := byName["S3"].(*s3backend.S3Handler)
	require.True(t, ok, "S3 handler must be registered")

	s3Bk, ok := s3H.Backend.(*s3backend.InMemoryBackend)
	require.True(t, ok, "S3 backend must be an InMemoryBackend")

	ctx := t.Context()

	_, err = s3Bk.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("textract-wiring-bucket")})
	require.NoError(t, err)

	_, err = s3Bk.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("textract-wiring-bucket"),
		Key:    aws.String("doc.pdf"),
		Body:   bytes.NewReader([]byte("fake-pdf-bytes")),
	})
	require.NoError(t, err)

	status, resp := doJSONTargetRequest(t, trH.Handler(), "Textract.AnalyzeDocument", map[string]any{
		"Document": map[string]any{
			"S3Object": map[string]any{"Bucket": "textract-wiring-bucket", "Name": "missing.pdf"},
		},
		"FeatureTypes": []string{"TABLES"},
	})
	require.Equal(t, http.StatusBadRequest, status,
		"a Document.S3Object naming a bucket/key absent from the real S3 backend must be rejected "+
			"through the real cli.go composition root's S3 wiring (wireTextractS3), not processed "+
			"regardless of whether the object exists")
	require.Equal(t, "InvalidS3ObjectException", resp["__type"])

	status, _ = doJSONTargetRequest(t, trH.Handler(), "Textract.AnalyzeDocument", map[string]any{
		"Document": map[string]any{
			"S3Object": map[string]any{"Bucket": "textract-wiring-bucket", "Name": "doc.pdf"},
		},
		"FeatureTypes": []string{"TABLES"},
	})
	require.Equal(t, http.StatusOK, status)
}

// TestInitializeServices_RekognitionS3Wiring is
// TestInitializeServices_TextractS3Wiring for Rekognition (gopherstack-eshx).
func TestInitializeServices_RekognitionS3Wiring(t *testing.T) {
	t.Parallel()

	appCtx := newTestAppContext(t, 19500, 19600)

	services, err := initializeServices(appCtx)
	require.NoError(t, err)

	byName := serviceByName(services)

	rH, ok := byName["Rekognition"].(*rekognitionbackend.Handler)
	require.True(t, ok, "Rekognition handler must be registered")

	s3H, ok := byName["S3"].(*s3backend.S3Handler)
	require.True(t, ok, "S3 handler must be registered")

	s3Bk, ok := s3H.Backend.(*s3backend.InMemoryBackend)
	require.True(t, ok, "S3 backend must be an InMemoryBackend")

	ctx := t.Context()

	_, err = s3Bk.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("rekognition-wiring-bucket")})
	require.NoError(t, err)

	_, err = s3Bk.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String("rekognition-wiring-bucket"),
		Key:    aws.String("photo.jpg"),
		Body:   bytes.NewReader([]byte("fake-jpeg-bytes")),
	})
	require.NoError(t, err)

	status, resp := doJSONTargetRequest(t, rH.Handler(), "RekognitionService.DetectLabels", map[string]any{
		"Image": map[string]any{
			"S3Object": map[string]any{"Bucket": "rekognition-wiring-bucket", "Name": "missing.jpg"},
		},
	})
	require.Equal(t, http.StatusBadRequest, status,
		"an Image.S3Object naming a bucket/key absent from the real S3 backend must be rejected "+
			"through the real cli.go composition root's S3 wiring (wireRekognitionS3), not processed "+
			"regardless of whether the object exists")
	require.Equal(t, "InvalidS3ObjectException", resp["__type"])

	status, _ = doJSONTargetRequest(t, rH.Handler(), "RekognitionService.DetectLabels", map[string]any{
		"Image": map[string]any{
			"S3Object": map[string]any{"Bucket": "rekognition-wiring-bucket", "Name": "photo.jpg"},
		},
	})
	require.Equal(t, http.StatusOK, status)
}
