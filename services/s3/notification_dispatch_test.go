package s3_test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/s3"
)

func TestHandler_NotificationDispatch_PutObject(t *testing.T) {
	t.Parallel()

	handler, backend := newTestHandler(t)
	mustCreateBucket(t, backend, "notif-put")

	notifXML := `<NotificationConfiguration>` +
		`<QueueConfiguration><Id>q1</Id>` +
		`<Queue>arn:aws:sqs:us-east-1:000000000000:my-queue</Queue>` +
		`<Event>s3:ObjectCreated:*</Event></QueueConfiguration>` +
		`</NotificationConfiguration>`
	req := httptest.NewRequest(
		http.MethodPut,
		"/notif-put?notification",
		strings.NewReader(notifXML),
	)
	rec := httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	mock := &mockNotificationDispatcher{}
	handler.SetNotificationDispatcher(mock)

	req = httptest.NewRequest(http.MethodPut, "/notif-put/key1", strings.NewReader("hello"))
	rec = httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Eventually(t, func() bool {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		return len(mock.created) == 1
	}, 200*time.Millisecond, 5*time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Equal(t, "notif-put", mock.created[0].bucket)
	assert.Equal(t, "key1", mock.created[0].key)
}

func TestHandler_NotificationDispatch_DeleteObject(t *testing.T) {
	t.Parallel()

	handler, backend := newTestHandler(t)
	mustCreateBucket(t, backend, "notif-del")
	mustPutObject(t, backend, "notif-del", "key1", []byte("data"))

	notifXML := `<NotificationConfiguration>` +
		`<QueueConfiguration><Id>q1</Id>` +
		`<Queue>arn:aws:sqs:us-east-1:000000000000:my-queue</Queue>` +
		`<Event>s3:ObjectRemoved:*</Event></QueueConfiguration>` +
		`</NotificationConfiguration>`
	putNotifReq := httptest.NewRequest(
		http.MethodPut,
		"/notif-del?notification",
		strings.NewReader(notifXML),
	)
	rec := httptest.NewRecorder()
	serveS3Handler(handler, rec, putNotifReq)
	require.Equal(t, http.StatusOK, rec.Code)

	mock := &mockNotificationDispatcher{}
	handler.SetNotificationDispatcher(mock)

	req := httptest.NewRequest(http.MethodDelete, "/notif-del/key1", nil)
	rec = httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	require.Eventually(t, func() bool {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		return len(mock.deleted) == 1
	}, 200*time.Millisecond, 5*time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Equal(t, "notif-del", mock.deleted[0].bucket)
	assert.Equal(t, "key1", mock.deleted[0].key)
}

func TestHandler_NotificationDispatch_NoDispatchWithoutConfig(t *testing.T) {
	t.Parallel()

	handler, backend := newTestHandler(t)
	mustCreateBucket(t, backend, "no-notif")

	mock := &mockNotificationDispatcher{}
	handler.SetNotificationDispatcher(mock)

	req := httptest.NewRequest(http.MethodPut, "/no-notif/key1", strings.NewReader("hello"))
	rec := httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	time.Sleep(20 * time.Millisecond)
	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Empty(t, mock.created)
	assert.Empty(t, mock.deleted)
}

func TestHandler_NotificationDispatch_CopyObject(t *testing.T) {
	t.Parallel()

	handler, backend := newTestHandler(t)
	mustCreateBucket(t, backend, "notif-copy")
	mustPutObject(t, backend, "notif-copy", "src-key", []byte("source data"))

	notifXML := `<NotificationConfiguration>` +
		`<QueueConfiguration><Id>q1</Id>` +
		`<Queue>arn:aws:sqs:us-east-1:000000000000:copy-queue</Queue>` +
		`<Event>s3:ObjectCreated:*</Event></QueueConfiguration>` +
		`</NotificationConfiguration>`
	req := httptest.NewRequest(
		http.MethodPut,
		"/notif-copy?notification",
		strings.NewReader(notifXML),
	)
	rec := httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	mock := &mockNotificationDispatcher{}
	handler.SetNotificationDispatcher(mock)

	req = httptest.NewRequest(http.MethodPut, "/notif-copy/dest-key", nil)
	req.Header.Set("X-Amz-Copy-Source", "/notif-copy/src-key")
	rec = httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Eventually(t, func() bool {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		return len(mock.created) == 1
	}, 200*time.Millisecond, 5*time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Equal(t, "notif-copy", mock.created[0].bucket)
	assert.Equal(t, "dest-key", mock.created[0].key)
}

func TestHandler_NotificationDispatch_CompleteMultipartUpload(t *testing.T) {
	t.Parallel()

	handler, backend := newTestHandler(t)
	mustCreateBucket(t, backend, "notif-mpu")

	notifXML := `<NotificationConfiguration>` +
		`<QueueConfiguration><Id>q1</Id>` +
		`<Queue>arn:aws:sqs:us-east-1:000000000000:mpu-queue</Queue>` +
		`<Event>s3:ObjectCreated:*</Event></QueueConfiguration>` +
		`</NotificationConfiguration>`
	req := httptest.NewRequest(
		http.MethodPut,
		"/notif-mpu?notification",
		strings.NewReader(notifXML),
	)
	rec := httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	mock := &mockNotificationDispatcher{}
	handler.SetNotificationDispatcher(mock)

	// Start multipart upload.
	req = httptest.NewRequest(http.MethodPost, "/notif-mpu/mp-key?uploads", nil)
	rec = httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var initResp s3.InitiateMultipartUploadResult
	require.NoError(t, xml.NewDecoder(rec.Body).Decode(&initResp))
	uploadID := initResp.UploadID

	// Upload a part.
	req = httptest.NewRequest(
		http.MethodPut,
		"/notif-mpu/mp-key?partNumber=1&uploadId="+uploadID,
		strings.NewReader("part1"),
	)
	rec = httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	etag1 := rec.Header().Get("ETag")

	// Complete the upload.
	completeXML := fmt.Sprintf(
		`<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>%s</ETag></Part></CompleteMultipartUpload>`,
		etag1,
	)
	req = httptest.NewRequest(
		http.MethodPost,
		"/notif-mpu/mp-key?uploadId="+uploadID,
		strings.NewReader(completeXML),
	)
	rec = httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Eventually(t, func() bool {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		return len(mock.created) == 1
	}, 200*time.Millisecond, 5*time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Equal(t, "notif-mpu", mock.created[0].bucket)
	assert.Equal(t, "mp-key", mock.created[0].key)
}

// TestHandler_NotificationDispatch_PostObject_EventNameIsPost verifies that a
// browser-style POST upload (post_object.go) fires the s3:ObjectCreated:Post
// event, not s3:ObjectCreated:Put. AWS documents Put/Post/Copy/
// CompleteMultipartUpload as distinct ObjectCreated sub-events; a notification
// rule scoped to exactly "s3:ObjectCreated:Post" must fire for a POST upload.
func TestHandler_NotificationDispatch_PostObject_EventNameIsPost(t *testing.T) {
	t.Parallel()

	handler, backend := newTestHandler(t)
	mustCreateBucket(t, backend, "notif-post")

	notifXML := `<NotificationConfiguration>` +
		`<QueueConfiguration><Id>q1</Id>` +
		`<Queue>arn:aws:sqs:us-east-1:000000000000:post-queue</Queue>` +
		`<Event>s3:ObjectCreated:Post</Event></QueueConfiguration>` +
		`</NotificationConfiguration>`
	req := httptest.NewRequest(
		http.MethodPut,
		"/notif-post?notification",
		strings.NewReader(notifXML),
	)
	rec := httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	queue := &captureQueue{}
	handler.SetNotificationDispatcher(
		s3.NewNotificationDispatcher(&s3.NotificationTargets{SQSSender: queue}, "us-east-1"),
	)

	body, contentType := buildPostForm(t, map[string]string{"key": "posted.txt"}, "posted.txt", []byte("hi"))
	req = httptest.NewRequest(http.MethodPost, "/notif-post", body)
	req.Header.Set("Content-Type", contentType)
	rec = httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	require.Eventually(t, func() bool {
		queue.mu.Lock()
		defer queue.mu.Unlock()

		return len(queue.messages) == 1
	}, 200*time.Millisecond, 5*time.Millisecond)

	queue.mu.Lock()
	defer queue.mu.Unlock()
	assert.Contains(t, queue.messages[0], `"eventName":"s3:ObjectCreated:Post"`)
}

func TestHandler_NotificationDispatch_DeleteObjects(t *testing.T) {
	t.Parallel()

	handler, backend := newTestHandler(t)
	mustCreateBucket(t, backend, "notif-delobj")
	mustPutObject(t, backend, "notif-delobj", "key1", []byte("data1"))
	mustPutObject(t, backend, "notif-delobj", "key2", []byte("data2"))

	notifXML := `<NotificationConfiguration>` +
		`<QueueConfiguration><Id>q1</Id>` +
		`<Queue>arn:aws:sqs:us-east-1:000000000000:del-queue</Queue>` +
		`<Event>s3:ObjectRemoved:*</Event></QueueConfiguration>` +
		`</NotificationConfiguration>`
	req := httptest.NewRequest(
		http.MethodPut,
		"/notif-delobj?notification",
		strings.NewReader(notifXML),
	)
	rec := httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	mock := &mockNotificationDispatcher{}
	handler.SetNotificationDispatcher(mock)

	deleteXML := `<Delete><Object><Key>key1</Key></Object><Object><Key>key2</Key></Object></Delete>`
	req = httptest.NewRequest(http.MethodPost, "/notif-delobj?delete", strings.NewReader(deleteXML))
	rec = httptest.NewRecorder()
	serveS3Handler(handler, rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Eventually(t, func() bool {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		return len(mock.deleted) == 2
	}, 200*time.Millisecond, 5*time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Equal(t, "notif-delobj", mock.deleted[0].bucket)
	assert.Equal(t, "notif-delobj", mock.deleted[1].bucket)
}

// ---- Object Lock tests ----
