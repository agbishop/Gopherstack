package s3_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdk_s3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3BucketNotificationCRUD(t *testing.T) {
	t.Parallel()
	handler, sdkClient := newTestHandler(t)
	bucket := "notif-test-bucket"

	_, err := sdkClient.CreateBucket(t.Context(), &sdk_s3.CreateBucketInput{Bucket: &bucket})
	require.NoError(t, err)

	notifXML := `<NotificationConfiguration><TopicConfiguration>` +
		`<Topic>arn:aws:sns:us-east-1:000000000000:my-topic</Topic>` +
		`<Event>s3:ObjectCreated:*</Event></TopicConfiguration></NotificationConfiguration>`

	// PutBucketNotificationConfiguration
	req := httptest.NewRequest(
		http.MethodPut,
		"/"+bucket+"?notification",
		strings.NewReader(notifXML),
	)
	rec := httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// GetBucketNotificationConfiguration
	req = httptest.NewRequest(http.MethodGet, "/"+bucket+"?notification", nil)
	rec = httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "TopicConfiguration")

	// GetBucketNotificationConfiguration on bucket without notifications
	bucket2 := "notif-empty-bucket"
	_, err = sdkClient.CreateBucket(
		t.Context(),
		&sdk_s3.CreateBucketInput{Bucket: aws.String(bucket2)},
	)
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodGet, "/"+bucket2+"?notification", nil)
	rec = httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "NotificationConfiguration")
}

// TestS3BucketWebsiteCRUD verifies put/get/delete bucket website configuration.

func TestHandler_BucketNotificationStub(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		wantStatus int
	}{
		{name: "PUT ?notification returns 200", method: http.MethodPut, wantStatus: http.StatusOK},
		{name: "GET ?notification returns 200", method: http.MethodGet, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, backend := newTestHandler(t)
			mustCreateBucket(t, backend, "notify-bkt")

			req := httptest.NewRequest(tt.method, "/notify-bkt?notification", nil)
			rec := httptest.NewRecorder()
			serveS3Handler(handler, rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// mockNotificationDispatcher is a test double for NotificationDispatcher.
type mockNotificationDispatcher struct {
	created []notificationEvent
	deleted []notificationEvent
	mu      sync.Mutex
}

type notificationEvent struct {
	bucket   string
	key      string
	notifXML string
}

func (m *mockNotificationDispatcher) DispatchObjectCreated(
	_ context.Context, bucket, key, _ string, _ int64, notifXML string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, notificationEvent{bucket: bucket, key: key, notifXML: notifXML})
}

func (m *mockNotificationDispatcher) DispatchObjectCopied(
	_ context.Context, bucket, key, _ string, _ int64, notifXML string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, notificationEvent{bucket: bucket, key: key, notifXML: notifXML})
}

func (m *mockNotificationDispatcher) DispatchObjectPosted(
	_ context.Context, bucket, key, _ string, _ int64, notifXML string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, notificationEvent{bucket: bucket, key: key, notifXML: notifXML})
}

func (m *mockNotificationDispatcher) DispatchObjectCompleted(
	_ context.Context, bucket, key, _ string, _ int64, notifXML string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, notificationEvent{bucket: bucket, key: key, notifXML: notifXML})
}

func (m *mockNotificationDispatcher) DispatchObjectDeleted(
	_ context.Context, bucket, key, notifXML string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, notificationEvent{bucket: bucket, key: key, notifXML: notifXML})
}

func (m *mockNotificationDispatcher) DispatchObjectRestorePost(
	_ context.Context, _, _, _ string,
) {
	// Unused by current tests; satisfy NotificationDispatcher.
}
