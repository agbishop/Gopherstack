package support

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	// maxAttachmentSets caps the number of in-flight staged attachment sets to prevent unbounded growth.
	maxAttachmentSets = 1000

	maxAttachmentsPerSet = 3
	maxAttachmentSize    = 5 * 1024 * 1024

	// attachmentSetCreationWindow / maxAttachmentSetCreationsPerWindow back
	// AttachmentLimitExceeded ("The limit for the number of attachment sets
	// created in a short period of time has been exceeded" -- real AWS
	// publishes no exact number). The threshold is set comfortably above the
	// maxAttachmentSets storage cap so bulk-creation tests that stress that
	// cap (e.g. eviction-at-cap coverage) are unaffected; it exists purely so
	// the modeled exception is reachable and real, not a stub.
	attachmentSetCreationWindow        = time.Minute
	maxAttachmentSetCreationsPerWindow = 1200

	// describeAttachmentWindow / maxDescribeAttachmentCallsPerWindow back
	// DescribeAttachmentLimitExceeded ("The limit for the number of
	// DescribeAttachment requests in a short period of time has been
	// exceeded").
	describeAttachmentWindow            = time.Minute
	maxDescribeAttachmentCallsPerWindow = 1000
)

// AddAttachmentsToSet creates a new attachment set and returns its ID.
func (b *InMemoryBackend) AddAttachmentsToSet(attachmentSetID string) (string, time.Time, error) {
	b.mu.Lock("AddAttachmentsToSet")
	defer b.mu.Unlock()

	if attachmentSetID == "" {
		attachmentSetID = uuid.New().String()
	}

	expiry := time.Now().Add(time.Hour)
	b.attachmentSets.Put(&AttachmentSet{ID: attachmentSetID, Expiry: expiry})

	return attachmentSetID, expiry, nil
}

// DescribeAttachment returns the attachment with the given ID. It takes the
// write lock (not RLock) because every call records a timestamp against the
// DescribeAttachmentLimitExceeded rate-limit window.
func (b *InMemoryBackend) DescribeAttachment(attachmentID string) (*Attachment, error) {
	b.mu.Lock("DescribeAttachment")
	defer b.mu.Unlock()

	now := time.Now()
	b.describeAttachmentCallTimes = pruneOldLocked(b.describeAttachmentCallTimes, describeAttachmentWindow, now)

	if len(b.describeAttachmentCallTimes) >= maxDescribeAttachmentCallsPerWindow {
		return nil, fmt.Errorf("%w: too many DescribeAttachment requests", ErrDescribeAttachmentLimitExceeded)
	}

	b.describeAttachmentCallTimes = append(b.describeAttachmentCallTimes, now)

	a, ok := b.attachments.Get(attachmentID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAttachmentNotFound, attachmentID)
	}

	cp := *a

	return &cp, nil
}

// pruneOldLocked drops timestamps older than window from the front of times,
// which is kept in non-decreasing append order. The caller must hold b.mu for
// writing.
func pruneOldLocked(times []time.Time, window time.Duration, now time.Time) []time.Time {
	i := 0
	for i < len(times) && now.Sub(times[i]) >= window {
		i++
	}

	if i == 0 {
		return times
	}

	return append([]time.Time(nil), times[i:]...)
}

// AddAttachmentInternal seeds an attachment directly into the backend (for testing).
func (b *InMemoryBackend) AddAttachmentInternal(a *Attachment) {
	b.mu.Lock("AddAttachmentInternal")
	defer b.mu.Unlock()

	cp := *a
	if a.Data != nil {
		cp.Data = make([]byte, len(a.Data))
		copy(cp.Data, a.Data)
	}

	b.attachments.Put(&cp)
}

// AddAttachmentsToSetWithAttachments stages uploaded files for single-use consumption.
func (b *InMemoryBackend) AddAttachmentsToSetWithAttachments(
	attachmentSetID string,
	attachments []Attachment,
) (string, time.Time, error) {
	b.mu.Lock("AddAttachmentsToSetWithAttachments")
	defer b.mu.Unlock()

	if len(attachments) == 0 {
		return "", time.Time{}, fmt.Errorf("%w: attachments is required", ErrValidation)
	}
	if err := validateAttachments(attachments); err != nil {
		return "", time.Time{}, err
	}
	set, setID, err := b.attachmentSetForAppendLocked(attachmentSetID)
	if err != nil {
		return "", time.Time{}, err
	}
	if len(set.AttachmentIDs)+len(attachments) > maxAttachmentsPerSet {
		return "", time.Time{}, fmt.Errorf(
			"%w: maximum of %d attachments exceeded",
			ErrAttachmentSetSizeLimitExceeded,
			maxAttachmentsPerSet,
		)
	}
	for _, attachment := range attachments {
		attachment.AttachmentID = "att-" + uuid.NewString()
		cp := attachment
		cp.Data = append([]byte(nil), attachment.Data...)
		b.attachments.Put(&cp)
		set.AttachmentIDs = append(set.AttachmentIDs, cp.AttachmentID)
	}
	set.Expiry = time.Now().Add(time.Hour)

	return setID, set.Expiry, nil
}

func validateAttachments(attachments []Attachment) error {
	for _, attachment := range attachments {
		if attachment.FileName == "" || len(attachment.Data) == 0 {
			return fmt.Errorf("%w: invalid attachment", ErrValidation)
		}
		// Real AWS documents this specific limit as AttachmentSetSizeLimitExceeded
		// ("The limits are three attachments and 5 MB per attachment"), not a
		// generic validation failure.
		if len(attachment.Data) > maxAttachmentSize {
			return fmt.Errorf(
				"%w: attachment exceeds maximum size of %d bytes",
				ErrAttachmentSetSizeLimitExceeded,
				maxAttachmentSize,
			)
		}
	}

	return nil
}

func (b *InMemoryBackend) attachmentSetForAppendLocked(id string) (*AttachmentSet, string, error) {
	if id == "" {
		now := time.Now()
		b.attachmentSetCreationTimes = pruneOldLocked(b.attachmentSetCreationTimes, attachmentSetCreationWindow, now)

		if len(b.attachmentSetCreationTimes) >= maxAttachmentSetCreationsPerWindow {
			return nil, "", fmt.Errorf("%w: too many attachment sets created recently", ErrAttachmentLimitExceeded)
		}

		b.attachmentSetCreationTimes = append(b.attachmentSetCreationTimes, now)

		if b.attachmentSets.Len() >= maxAttachmentSets {
			// Evict the set with the earliest (soonest-expired) expiry.
			var oldestID string
			var oldestExpiry time.Time

			found := false

			b.attachmentSets.Range(func(s *AttachmentSet) bool {
				if !found || s.Expiry.Before(oldestExpiry) {
					oldestID = s.ID
					oldestExpiry = s.Expiry
					found = true
				}

				return true
			})

			b.attachmentSets.Delete(oldestID)
		}

		id = uuid.NewString()
		set := &AttachmentSet{ID: id}
		b.attachmentSets.Put(set)

		return set, id, nil
	}

	set, ok := b.attachmentSets.Get(id)
	if !ok {
		return nil, "", fmt.Errorf("%w: %s", ErrAttachmentSetNotFound, id)
	}

	if time.Now().After(set.Expiry) {
		return nil, "", fmt.Errorf("%w: %s", ErrAttachmentSetExpired, id)
	}

	return set, id, nil
}

func (b *InMemoryBackend) consumeAttachmentSetLocked(id string) ([]AttachmentRef, error) {
	if id == "" {
		return nil, nil
	}
	set, ok := b.attachmentSets.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAttachmentSetNotFound, id)
	}
	if time.Now().After(set.Expiry) {
		return nil, fmt.Errorf("%w: %s", ErrAttachmentSetExpired, id)
	}
	refs := make([]AttachmentRef, 0, len(set.AttachmentIDs))
	for _, attachmentID := range set.AttachmentIDs {
		// A set's AttachmentIDs always resolve via the public API (attachments
		// and their owning set are added together and swept together), but a
		// hand-edited or cross-version persisted snapshot could restore a set
		// referencing an attachment that no longer exists. Skip rather than
		// dereference a nil *Attachment.
		attachment, found := b.attachments.Get(attachmentID)
		if !found {
			continue
		}
		refs = append(refs, AttachmentRef{AttachmentID: attachmentID, FileName: attachment.FileName})
	}
	b.attachmentSets.Delete(id)

	return refs, nil
}
