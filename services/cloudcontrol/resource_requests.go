package cloudcontrol

import (
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// GetResourceRequestStatus returns a copy of the ProgressEvent for the given request token.
// Events are retained in the map until Reset() is called.
// An unrecognized requestToken returns ErrRequestTokenNotFound (RequestTokenNotFoundException),
// the only error this operation declares -- not ErrNotFound (ResourceNotFoundException),
// which describes a missing *resource*, not a missing *request token*.
func (b *InMemoryBackend) GetResourceRequestStatus(requestToken string) (*ProgressEvent, error) {
	b.mu.RLock("GetResourceRequestStatus")
	defer b.mu.RUnlock()

	event, ok := b.requests.Get(requestToken)
	if !ok {
		return nil, ErrRequestTokenNotFound
	}

	return copyEvent(event), nil
}

// CancelResourceRequest cancels the request identified by requestToken.
// An unrecognized requestToken returns ErrRequestTokenNotFound (RequestTokenNotFoundException).
// Cancelling an already-terminal request (SUCCESS, FAILED, CANCEL_COMPLETE, CANCEL_IN_PROGRESS)
// returns ErrConcurrentModification (ConcurrentModificationException), matching the real AWS
// API reference for this operation -- not a validation error.
func (b *InMemoryBackend) CancelResourceRequest(requestToken string) (*ProgressEvent, error) {
	b.mu.Lock("CancelResourceRequest")
	defer b.mu.Unlock()

	event, ok := b.requests.Get(requestToken)
	if !ok {
		return nil, ErrRequestTokenNotFound
	}

	if event.OperationStatus != "IN_PROGRESS" {
		return nil, ErrConcurrentModification
	}

	cancelled := &ProgressEvent{
		EventTime:       unixEpochTime{time.Now()},
		TypeName:        event.TypeName,
		Identifier:      event.Identifier,
		RequestToken:    requestToken,
		Operation:       event.Operation,
		OperationStatus: opStatusCancelComplete,
		ResourceModel:   event.ResourceModel,
	}
	b.requests.Put(cancelled)

	return copyEvent(cancelled), nil
}

// ResourceRequestFilter holds optional filter criteria for ListResourceRequests.
// This mirrors the real SDK's types.ResourceRequestStatusFilter exactly:
// Operations and OperationStatuses only. There is no TypeName member on the
// real filter shape (confirmed against aws-sdk-go-v2/service/cloudcontrol/types
// and botocore's service-2.json) -- ListResourceRequests has no wire-level way
// to filter by resource type.
type ResourceRequestFilter struct {
	Operations        []string
	OperationStatuses []string
}

// eventMatchesFilter reports whether event passes the given filter.
// A nil filter matches every event. An unrecognized Operations/OperationStatuses
// value simply never matches any real event's Operation/OperationStatus -- it is
// NOT rejected as an error, because ListResourceRequests declares zero errors in
// the real model (confirmed: botocore's service-2.json has an empty "errors" list
// for this operation, unlike every other CloudControl op).
func eventMatchesFilter(event *ProgressEvent, filter *ResourceRequestFilter) bool {
	if filter == nil {
		return true
	}

	if len(filter.Operations) > 0 && !slices.Contains(filter.Operations, event.Operation) {
		return false
	}

	if len(filter.OperationStatuses) > 0 && !slices.Contains(filter.OperationStatuses, event.OperationStatus) {
		return false
	}

	return true
}

// ListResourceRequests returns all tracked resource requests, optionally filtered
// by operation type, operation status, and/or resource type name. Results are sorted
// by EventTime descending (most recent first) for deterministic output. Never
// errors on the filter contents (see eventMatchesFilter).
func (b *InMemoryBackend) ListResourceRequests(
	filter *ResourceRequestFilter, maxResults int, nextToken string,
) ([]*ProgressEvent, string, error) {
	b.mu.RLock("ListResourceRequests")
	defer b.mu.RUnlock()

	var out []*ProgressEvent

	b.requests.Range(func(event *ProgressEvent) bool {
		if eventMatchesFilter(event, filter) {
			out = append(out, event)
		}

		return true
	})

	// Sort by EventTime descending so the most-recent request appears first.
	sort.Slice(out, func(i, j int) bool {
		return out[i].EventTime.After(out[j].EventTime.Time)
	})

	pg := page.New(out, nextToken, maxResults, defaultListMaxResults)

	// Deep-copy the page items so callers cannot mutate backend state.
	result := make([]*ProgressEvent, len(pg.Data))
	for i, e := range pg.Data {
		result[i] = copyEvent(e)
	}

	return result, pg.Next, nil
}

// copyEvent returns a shallow copy of a ProgressEvent so callers cannot mutate backend state.
func copyEvent(e *ProgressEvent) *ProgressEvent {
	if e == nil {
		return nil
	}

	cp := *e

	return &cp
}

// AddProgressEvent inserts a ProgressEvent directly into the requests map.
// This is intended for use in tests to set up specific request states that
// cannot be reached through the normal API (e.g. IN_PROGRESS).
func (b *InMemoryBackend) AddProgressEvent(event *ProgressEvent) {
	b.mu.Lock("AddProgressEvent")
	defer b.mu.Unlock()

	b.requests.Put(event)
}
