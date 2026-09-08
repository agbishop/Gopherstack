package rekognition_test

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

	"github.com/blackbirdworks/gopherstack/services/rekognition"
)

// fakeS3Backend is a minimal rekognition.S3Backend test double: HeadObject
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

func newWiredHandler(t *testing.T, objects map[string]bool) *rekognition.Handler {
	t.Helper()

	b := rekognition.NewInMemoryBackend("000000000000", "us-east-1")
	b.SetS3Backend(&fakeS3Backend{objects: objects})

	return rekognition.NewHandler(b)
}

func s3ImageRef(bucket, key string) map[string]any {
	return map[string]any{"S3Object": map[string]any{"Bucket": bucket, "Name": key}}
}

// TestImageOps_S3ObjectValidation proves that once S3 is wired via
// SetS3Backend, every sync image-analysis op rejects an Image.S3Object
// naming a bucket/key that does not exist (InvalidS3ObjectException) and
// allows one that does. Fails against the pre-fix code: these ops never
// called into S3 at all, so a nonexistent object was silently accepted.
func TestImageOps_S3ObjectValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
	}{
		{name: "detect_labels", action: "DetectLabels"},
		{name: "detect_custom_labels", action: "DetectCustomLabels"},
		{name: "detect_moderation_labels", action: "DetectModerationLabels"},
		{name: "detect_protective_equipment", action: "DetectProtectiveEquipment"},
		{name: "detect_text", action: "DetectText"},
		{name: "detect_faces", action: "DetectFaces"},
		{name: "recognize_celebrities", action: "RecognizeCelebrities"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newWiredHandler(t, map[string]bool{"good-bucket/photo.jpg": true})

			rec := doRequest(t, h, tt.action, map[string]any{
				"Image": s3ImageRef("missing-bucket", "photo.jpg"),
			})
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "InvalidS3ObjectException", resp["__type"])

			rec = doRequest(t, h, tt.action, map[string]any{
				"Image": s3ImageRef("good-bucket", "photo.jpg"),
			})
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

// TestCompareFaces_S3ObjectValidation proves CompareFaces checks both
// SourceImage and TargetImage.
func TestCompareFaces_S3ObjectValidation(t *testing.T) {
	t.Parallel()

	h := newWiredHandler(t, map[string]bool{"good-bucket/a.jpg": true, "good-bucket/b.jpg": true})

	rec := doRequest(t, h, "CompareFaces", map[string]any{
		"SourceImage": s3ImageRef("missing-bucket", "a.jpg"),
		"TargetImage": s3ImageRef("good-bucket", "b.jpg"),
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = doRequest(t, h, "CompareFaces", map[string]any{
		"SourceImage": s3ImageRef("good-bucket", "a.jpg"),
		"TargetImage": s3ImageRef("missing-bucket", "b.jpg"),
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = doRequest(t, h, "CompareFaces", map[string]any{
		"SourceImage": s3ImageRef("good-bucket", "a.jpg"),
		"TargetImage": s3ImageRef("good-bucket", "b.jpg"),
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestSearchByImageOps_S3ObjectValidation proves SearchFacesByImage and
// SearchUsersByImage apply the same Image.S3Object check.
func TestSearchByImageOps_S3ObjectValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
	}{
		{name: "search_faces_by_image", action: "SearchFacesByImage"},
		{name: "search_users_by_image", action: "SearchUsersByImage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newWiredHandler(t, map[string]bool{"good-bucket/photo.jpg": true})

			createRec := doRequest(t, h, "CreateCollection", map[string]any{"CollectionId": "c1"})
			require.Equal(t, http.StatusOK, createRec.Code)

			rec := doRequest(t, h, tt.action, map[string]any{
				"CollectionId": "c1",
				"Image":        s3ImageRef("missing-bucket", "photo.jpg"),
			})
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "InvalidS3ObjectException", resp["__type"])

			rec = doRequest(t, h, tt.action, map[string]any{
				"CollectionId": "c1",
				"Image":        s3ImageRef("good-bucket", "photo.jpg"),
			})
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

// TestStartVideoOps_S3ObjectValidation proves every async Start* video-job
// op applies the same Video.S3Object check.
func TestStartVideoOps_S3ObjectValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		action string
	}{
		{name: "start_celebrity_recognition", action: "StartCelebrityRecognition"},
		{name: "start_content_moderation", action: "StartContentModeration"},
		{name: "start_face_detection", action: "StartFaceDetection"},
		{name: "start_face_search", action: "StartFaceSearch"},
		{name: "start_label_detection", action: "StartLabelDetection"},
		{name: "start_person_tracking", action: "StartPersonTracking"},
		{name: "start_segment_detection", action: "StartSegmentDetection"},
		{name: "start_text_detection", action: "StartTextDetection"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newWiredHandler(t, map[string]bool{"good-bucket/video.mp4": true})

			rec := doRequest(t, h, tt.action, map[string]any{
				"Video": s3ImageRef("missing-bucket", "video.mp4"),
			})
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var resp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "InvalidS3ObjectException", resp["__type"])

			rec = doRequest(t, h, tt.action, map[string]any{
				"Video": s3ImageRef("good-bucket", "video.mp4"),
			})
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

// TestStartMediaAnalysisJob_S3ObjectValidation proves StartMediaAnalysisJob
// applies the check to Input.S3Object.
func TestStartMediaAnalysisJob_S3ObjectValidation(t *testing.T) {
	t.Parallel()

	h := newWiredHandler(t, map[string]bool{"good-bucket/video.mp4": true})

	body := func(bucket string) map[string]any {
		return map[string]any{
			"Input":            map[string]any{"S3Object": map[string]any{"Bucket": bucket, "Name": "video.mp4"}},
			"OperationsConfig": map[string]any{"DetectModerationLabels": map[string]any{}},
			"OutputConfig":     map[string]any{"S3Bucket": "output-bucket"},
		}
	}

	rec := doRequest(t, h, "StartMediaAnalysisJob", body("missing-bucket"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "InvalidS3ObjectException", resp["__type"])

	rec = doRequest(t, h, "StartMediaAnalysisJob", body("good-bucket"))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestDetectLabels_UnwiredS3StaysPermissive proves the unwired path stays
// permissive: with no SetS3Backend call, a nonexistent S3Object must still
// be accepted, matching this repo's unwired-hook-stays-permissive
// convention (roughly 150 services construct backends in tests with no
// cross-service hooks wired).
func TestDetectLabels_UnwiredS3StaysPermissive(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "DetectLabels", map[string]any{
		"Image": s3ImageRef("nonexistent-bucket", "photo.jpg"),
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStartLabelDetection_UnwiredS3StaysPermissive is
// TestDetectLabels_UnwiredS3StaysPermissive for the async Start* path.
func TestStartLabelDetection_UnwiredS3StaysPermissive(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "StartLabelDetection", map[string]any{
		"Video": s3ImageRef("nonexistent-bucket", "video.mp4"),
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestDetectLabels_InlineBytesUnaffected proves an Image carrying inline
// Bytes instead of an S3Object is never subject to the S3 existence check,
// even when S3 is wired.
func TestDetectLabels_InlineBytesUnaffected(t *testing.T) {
	t.Parallel()

	h := newWiredHandler(t, map[string]bool{})

	rec := doRequest(t, h, "DetectLabels", map[string]any{
		"Image": map[string]any{"Bytes": []byte("fake-image-bytes")},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}
