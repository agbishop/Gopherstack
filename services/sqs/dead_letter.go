package sqs

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// redrivePolicy represents the JSON structure of an SQS RedrivePolicy attribute.
type redrivePolicy struct {
	DeadLetterTargetArn string      `json:"deadLetterTargetArn"`
	MaxReceiveCount     json.Number `json:"maxReceiveCount"`
}

// applyRedrivePolicy parses the RedrivePolicy attribute and wires up DLQ fields on q.
func applyRedrivePolicy(q *Queue, attrs map[string]string, backend *InMemoryBackend) error {
	raw, ok := attrs[attrRedrivePolicy]
	if !ok {
		return nil
	}
	if raw == "" {
		q.MaxReceiveCount = 0
		q.dlq = nil

		return nil
	}

	var pol redrivePolicy

	if err := json.Unmarshal([]byte(raw), &pol); err != nil {
		return &InvalidParameterError{Message: "Invalid value for the parameter RedrivePolicy."}
	}

	count, err := pol.MaxReceiveCount.Int64()
	if err != nil || count <= 0 {
		return &InvalidParameterError{Message: "Invalid value for the parameter RedrivePolicy."}
	}

	_, dlqName := parseQueueARNOrURL(pol.DeadLetterTargetArn)

	// DLQ must reside in the same region as the source queue (AWS rule).
	dlq, exists := backend.lookupQueueByName(q.Region, dlqName)
	if !exists {
		return &InvalidParameterError{
			Message: fmt.Sprintf(
				"Value %v for parameter RedrivePolicy is invalid. Reason: Dead letter target does not exist.",
				raw,
			),
		}
	}

	if q.IsFIFO != dlq.IsFIFO {
		return &InvalidParameterError{
			Message: fmt.Sprintf(
				"Value %v for parameter RedrivePolicy is invalid. Reason: Dead letter target does not match source queue type.",
				raw,
			),
		}
	}

	if allowErr := checkRedriveAllowPolicy(dlq, q.Attributes[attrQueueArn]); allowErr != nil {
		return &InvalidParameterError{
			Message: fmt.Sprintf(
				"Value %v for parameter RedrivePolicy is invalid. Reason: %s",
				raw, allowErr.Error(),
			),
		}
	}

	q.MaxReceiveCount = int(count)
	q.dlq = dlq

	now := time.Now()

	q.mu.Lock()
	defer q.mu.Unlock()

	var remaining []*Message
	for _, msg := range q.messages {
		if !tryRouteToDLQ(q, msg, now) {
			remaining = append(remaining, msg)
		}
	}
	q.messages = remaining

	return nil
}

// errRedriveDeniedAll / errRedriveDeniedByQueue are the reasons surfaced when a
// source queue's RedrivePolicy is rejected by the dead-letter target's
// RedriveAllowPolicy attribute.
var (
	errRedriveDeniedAll = errors.New(
		"redrive permission is denied because the dead-letter target queue's RedriveAllowPolicy" +
			" has redrivePermission set to denyAll",
	)
	errRedriveDeniedByQueue = errors.New(
		"redrive permission is denied because this queue's ARN is not listed in the dead-letter" +
			" target queue's RedriveAllowPolicy sourceQueueArns",
	)
)

// checkRedriveAllowPolicy enforces the dead-letter target queue's
// RedriveAllowPolicy attribute against the source queue attempting to point its
// RedrivePolicy at it. AWS lets a DLQ restrict which source queues may redrive
// into it via three redrivePermission values:
//
//   - allowAll (default when the attribute is absent/empty): any source queue may use it.
//   - denyAll: no source queue may use it.
//   - byQueue: only source queues whose ARN appears in sourceQueueArns may use it.
//
// Without this check, RedriveAllowPolicy is accepted and shape-validated by
// validateRedriveAllowPolicy but never actually constrains anything — a
// disguised stub. srcArn is the ARN of the queue whose RedrivePolicy is being
// applied (the would-be source/DLQ-user), dlq is the dead-letter target.
func checkRedriveAllowPolicy(dlq *Queue, srcArn string) error {
	raw, ok := dlq.Attributes[attrRedriveAllowPolicy]
	if !ok || raw == "" {
		return nil
	}

	var policy struct {
		RedrivePermission string   `json:"redrivePermission"`
		SourceQueueArns   []string `json:"sourceQueueArns"`
	}

	// Malformed policies are rejected at SetQueueAttributes time by
	// validateRedriveAllowPolicy, so a parse failure here means the DLQ was
	// never left with a malformed value; treat it permissively rather than
	// blocking on a defensive parse error.
	//nolint:nilerr // intentional fail-open on a defensive parse error, see comment above
	if json.Unmarshal([]byte(raw), &policy) != nil {
		return nil
	}

	switch policy.RedrivePermission {
	case "", "allowAll":
		return nil
	case "denyAll":
		return errRedriveDeniedAll
	case "byQueue":
		if slices.Contains(policy.SourceQueueArns, srcArn) {
			return nil
		}

		return errRedriveDeniedByQueue
	default:
		return nil
	}
}

// ListDeadLetterSourceQueues returns the URLs of all queues that have the given
// queue configured as their dead-letter queue via a RedrivePolicy.
func (b *InMemoryBackend) ListDeadLetterSourceQueues(
	input *ListDeadLetterSourceQueuesInput,
) (*ListDeadLetterSourceQueuesOutput, error) {
	b.mu.RLock("ListDeadLetterSourceQueues")
	defer b.mu.RUnlock()

	dlq, exists := b.lookupQueueByURL(input.Region, input.QueueURL)
	if !exists {
		return nil, ErrQueueNotFound
	}

	dlqARN := dlq.Attributes[attrQueueArn]

	var urls []string

	for _, q := range b.queues.All() {
		raw, ok := q.Attributes[attrRedrivePolicy]
		if !ok || raw == "" {
			continue
		}

		var pol redrivePolicy
		if err := json.Unmarshal([]byte(raw), &pol); err != nil {
			continue
		}

		count, err := pol.MaxReceiveCount.Int64()
		if err != nil || count <= 0 {
			continue
		}

		if pol.DeadLetterTargetArn == dlqARN {
			urls = append(urls, q.URL)
		}
	}

	sort.Strings(urls)

	p := page.New(urls, input.NextToken, input.MaxResults, sqsDefaultMaxResults)

	return &ListDeadLetterSourceQueuesOutput{QueueURLs: p.Data, NextToken: p.Next}, nil
}

// tryRouteToDLQ moves msg to the DLQ if it exceeds MaxReceiveCount.
// Returns true if the message was moved. Caller must hold q.mu.
//
// TryLock, not Lock: AWS accepts a RedrivePolicy naming q itself, or an
// AB-BA cycle, and q.dlq == q would self-deadlock the non-reentrant q.mu.
// On contention the move is skipped; the message stays visible and is
// retried on the next receive or janitor sweep.
func tryRouteToDLQ(q *Queue, msg *Message, now time.Time) bool {
	if q.MaxReceiveCount > 0 && q.dlq != nil && msg.ApproximateReceiveCount >= q.MaxReceiveCount {
		if !q.dlq.mu.TryLock() {
			return false
		}
		defer q.dlq.mu.Unlock()

		msg.ReceiptHandle = ""

		if msg.Attributes == nil {
			msg.Attributes = make(map[string]string, 1)
		}
		msg.Attributes[attrDeadLetterQueueSourceArn] = q.Attributes[attrQueueArn]

		q.dlq.messages = append(q.dlq.messages, msg)
		if now.Before(msg.VisibleAt) {
			q.dlq.delayedCount++
		}
		q.dlq.hasActivity.Store(true)

		return true
	}

	return false
}
