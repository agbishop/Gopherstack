package guardduty

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// GetFindings returns findings by IDs.
func (b *InMemoryBackend) GetFindings(detectorID string, findingIDs []string) ([]*Finding, error) {
	b.mu.RLock("GetFindings")
	defer b.mu.RUnlock()

	if !b.detectors.Has(detectorID) {
		return nil, ErrDetectorNotFound
	}

	results := make([]*Finding, 0, len(findingIDs))

	for _, id := range findingIDs {
		f, ok := b.findings.Get(detectorKey(detectorID, id))
		if !ok {
			return nil, ErrFindingNotFound
		}

		results = append(results, f)
	}

	return results, nil
}

// FindingsQuery holds the optional filter/sort/pagination parameters for
// ListFindings, mirroring types.ListFindingsInput's FindingCriteria,
// SortCriteria, MaxResults, and NextToken fields.
type FindingsQuery struct {
	Criteria   map[string]Condition
	SortAttr   string
	SortOrder  string
	NextToken  string
	MaxResults int32
}

// ListFindings returns finding IDs for a detector, filtered by
// q.Criteria, sorted per q.SortAttr/q.SortOrder (defaulting to ID
// ascending), and paginated per q.MaxResults/q.NextToken.
func (b *InMemoryBackend) ListFindings(detectorID string, q FindingsQuery) ([]string, string, error) {
	b.mu.RLock("ListFindings")
	defer b.mu.RUnlock()

	if !b.detectors.Has(detectorID) {
		return nil, "", ErrDetectorNotFound
	}

	all := b.findingsByDetector.Get(detectorID)

	matched := make([]*Finding, 0, len(all))

	for _, f := range all {
		if matchesFindingCriteria(f, q.Criteria) {
			matched = append(matched, f)
		}
	}

	sortFindings(matched, q.SortAttr, q.SortOrder)

	offset, err := decodeToken(q.NextToken)
	if err != nil {
		return nil, "", ErrValidation
	}

	size := resolvePageSize(int(q.MaxResults))
	page, nextToken := paginate(matched, offset, size)

	ids := make([]string, len(page))
	for i, f := range page {
		ids[i] = f.ID
	}

	return ids, nextToken, nil
}

// ArchiveFindings marks findings as archived.
func (b *InMemoryBackend) ArchiveFindings(detectorID string, findingIDs []string) error {
	b.mu.Lock("ArchiveFindings")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return ErrDetectorNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, id := range findingIDs {
		if f, ok := b.findings.Get(detectorKey(detectorID, id)); ok {
			f.Service.Archived = true
			f.UpdatedAt = now
		}
	}

	return nil
}

// UnarchiveFindings marks findings as unarchived.
func (b *InMemoryBackend) UnarchiveFindings(detectorID string, findingIDs []string) error {
	b.mu.Lock("UnarchiveFindings")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return ErrDetectorNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, id := range findingIDs {
		if f, ok := b.findings.Get(detectorKey(detectorID, id)); ok {
			f.Service.Archived = false
			f.UpdatedAt = now
		}
	}

	return nil
}

// CreateSampleFindings creates sample findings for a detector.
func (b *InMemoryBackend) CreateSampleFindings(detectorID string, findingTypes []string) error {
	b.mu.Lock("CreateSampleFindings")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return ErrDetectorNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if len(findingTypes) == 0 {
		findingTypes = []string{"UnauthorizedAccess:IAMUser/ConsoleLoginSuccess.B"}
	}

	for _, ft := range findingTypes {
		id := strings.ReplaceAll(uuid.New().String(), "-", "")
		f := &Finding{
			AccountID:     b.accountID,
			Arn:           b.findingARN(detectorID, id),
			CreatedAt:     now,
			Description:   "Sample finding: " + ft,
			DetectorID:    detectorID,
			ID:            id,
			Region:        b.region,
			Severity:      defaultFindingSeverity,
			Title:         "Sample: " + ft,
			Type:          ft,
			UpdatedAt:     now,
			SchemaVersion: "2.0",
			Service: FindingService{
				Archived:       false,
				Count:          1,
				DetectorID:     detectorID,
				EventFirstSeen: now,
				EventLastSeen:  now,
				ResourceRole:   "TARGET",
				ServiceName:    "guardduty",
			},
			Resource: FindingResource{
				ResourceType: "AccessKey",
			},
		}

		f.Service.Archived = b.matchesArchiveFilter(detectorID, f)
		b.findings.Put(f)
	}

	return nil
}

// FindingStatisticsQuery holds the optional filter/group-by parameters for
// GetFindingsStatistics, mirroring types.GetFindingsStatisticsInput's
// FindingCriteria, GroupBy, and OrderBy fields.
type FindingStatisticsQuery struct {
	Criteria   map[string]Condition
	GroupBy    string
	OrderBy    string
	MaxResults int32
}

// GetFindingsStatistics returns finding statistics for a detector. When
// q.GroupBy is unset it returns the deprecated countBySeverity aggregate
// (FindingStatistics.CountBySeverity); when set to one of
// ACCOUNT/DATE/FINDING_TYPE/RESOURCE/SEVERITY it returns the corresponding
// groupedByX list instead, matching real GuardDuty's documented behavior
// ("This parameter is deprecated. Please set GroupBy to 'SEVERITY' to
// return GroupedBySeverity instead.").
func (b *InMemoryBackend) GetFindingsStatistics(detectorID string, q FindingStatisticsQuery) (map[string]any, error) {
	b.mu.RLock("GetFindingsStatistics")
	defer b.mu.RUnlock()

	if !b.detectors.Has(detectorID) {
		return nil, ErrDetectorNotFound
	}

	var matched []*Finding

	for _, f := range b.findingsByDetector.Get(detectorID) {
		if matchesFindingCriteria(f, q.Criteria) {
			matched = append(matched, f)
		}
	}

	return map[string]any{
		"findingStatistics": findingStatisticsFor(matched, q.GroupBy, q.OrderBy, q.MaxResults),
	}, nil
}

// UpdateFindingsFeedback updates the feedback for findings.
func (b *InMemoryBackend) UpdateFindingsFeedback(detectorID string, findingIDs []string, feedback string) error {
	b.mu.Lock("UpdateFindingsFeedback")
	defer b.mu.Unlock()

	if !b.detectors.Has(detectorID) {
		return ErrDetectorNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339)

	for _, id := range findingIDs {
		if f, ok := b.findings.Get(detectorKey(detectorID, id)); ok {
			f.Service.UserFeedback = feedback
			f.UpdatedAt = now
		}
	}

	return nil
}
