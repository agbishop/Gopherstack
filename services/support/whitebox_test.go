package support

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doWhiteboxSupportRequest(t *testing.T, h *Handler, action string, body any) *httptest.ResponseRecorder {
	t.Helper()

	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSSupport_20130415."+action)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.Handler()(c))

	return rec
}

func seedAttachmentSetCreationTimes(b *InMemoryBackend, n int) {
	b.mu.Lock("seedAttachmentSetCreationTimes")
	defer b.mu.Unlock()

	now := time.Now()
	for range n {
		b.attachmentSetCreationTimes = append(b.attachmentSetCreationTimes, now)
	}
}

func seedDescribeAttachmentCallTimes(b *InMemoryBackend, n int) {
	b.mu.Lock("seedDescribeAttachmentCallTimes")
	defer b.mu.Unlock()

	now := time.Now()
	for range n {
		b.describeAttachmentCallTimes = append(b.describeAttachmentCallTimes, now)
	}
}

// TestSupport_AddAttachmentsToSet_AttachmentLimitExceeded verifies the
// AttachmentLimitExceeded rate limit on new-set creation is real and
// reachable (not a stub): once the sliding window is saturated, the next
// creation is rejected with the correct wire type; after seeding fewer than
// the threshold, creation still succeeds.
func TestSupport_AddAttachmentsToSet_AttachmentLimitExceeded(t *testing.T) {
	t.Parallel()

	t.Run("at_threshold_rejected", func(t *testing.T) {
		t.Parallel()

		b := NewInMemoryBackend()
		seedAttachmentSetCreationTimes(b, maxAttachmentSetCreationsPerWindow)

		_, _, err := b.AddAttachmentsToSetWithAttachments("", []Attachment{
			{FileName: "f.txt", Data: []byte("x")},
		})
		require.ErrorIs(t, err, ErrAttachmentLimitExceeded)
	})

	t.Run("below_threshold_succeeds", func(t *testing.T) {
		t.Parallel()

		b := NewInMemoryBackend()
		seedAttachmentSetCreationTimes(b, maxAttachmentSetCreationsPerWindow-1)

		_, _, err := b.AddAttachmentsToSetWithAttachments("", []Attachment{
			{FileName: "f.txt", Data: []byte("x")},
		})
		require.NoError(t, err)
	})
}

// TestSupport_DescribeAttachment_LimitExceeded verifies the
// DescribeAttachmentLimitExceeded rate limit is real and reachable.
func TestSupport_DescribeAttachment_LimitExceeded(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend()
	b.AddAttachmentInternal(&Attachment{AttachmentID: "att-rl", FileName: "f.txt", Data: []byte("x")})
	seedDescribeAttachmentCallTimes(b, maxDescribeAttachmentCallsPerWindow)

	_, err := b.DescribeAttachment("att-rl")
	require.ErrorIs(t, err, ErrDescribeAttachmentLimitExceeded)
}

// TestSupport_ConsumeAttachmentSet_DanglingAttachmentID verifies that
// consuming an attachment set referencing an attachment ID no longer present
// in the attachments table (e.g. restored from a hand-edited or
// cross-version snapshot) does not panic on a nil *Attachment dereference --
// the public API can never produce this state itself, since an attachment
// and its owning set are always added together and swept together
// (janitor.go), but persisted state is not guaranteed to preserve that
// invariant.
func TestSupport_ConsumeAttachmentSet_DanglingAttachmentID(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend()
	b.attachmentSets.Put(&AttachmentSet{
		ID:            "orphan-set",
		Expiry:        time.Now().Add(time.Hour),
		AttachmentIDs: []string{"missing-attachment"},
	})

	require.NotPanics(t, func() {
		c, err := b.CreateCaseWithOptions(CreateCaseOptions{
			Subject: "dangling attachment", CommunicationBody: "body",
			AttachmentSetID: "orphan-set",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, c.CaseID)
	})
}

// TestSupport_CreateCase_CaseCreationLimitExceeded verifies that
// CreateCase rejects new cases once the account-wide open-case cap is
// reached, and that resolving one open case frees a slot.
func TestSupport_CreateCase_CaseCreationLimitExceeded(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend()

	for i := range maxOpenCases {
		b.AddCaseInternal(&Case{
			CaseID:      "case-seed-" + strconv.Itoa(i),
			Status:      "opened",
			CreatedTime: time.Now(),
		})
	}

	_, err := b.CreateCaseWithOptions(CreateCaseOptions{
		Subject: "one too many", CommunicationBody: "body",
	})
	require.ErrorIs(t, err, ErrCaseCreationLimitExceeded)

	// Resolving one open case must free a slot.
	cases := b.DescribeCases(nil, false)
	require.NotEmpty(t, cases)
	_, resolveErr := b.ResolveCase(cases[0].CaseID)
	require.NoError(t, resolveErr)

	c, err := b.CreateCaseWithOptions(CreateCaseOptions{
		Subject: "fits now", CommunicationBody: "body",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, c.CaseID)
}

// TestSupport_CreateCase_CaseCreationLimitExceeded_WireType verifies the HTTP
// handler surfaces the real "CaseCreationLimitExceeded" __type, not a generic
// validation error.
func TestSupport_CreateCase_CaseCreationLimitExceeded_WireType(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend()
	h := NewHandler(b)

	for i := range maxOpenCases {
		b.AddCaseInternal(&Case{
			CaseID:      "case-seed-" + strconv.Itoa(i),
			Status:      "opened",
			CreatedTime: time.Now(),
		})
	}

	rec := doWhiteboxSupportRequest(t, h, "CreateCase", map[string]any{
		"subject": "over the limit", "communicationBody": "body",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var result map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, "CaseCreationLimitExceeded", result["__type"])
}
