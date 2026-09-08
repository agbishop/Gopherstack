package accessanalyzer

import (
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
)

// AddFinding adds a synthetic finding to an analyzer (for testing / resource scan simulation).
func (b *InMemoryBackend) AddFinding(
	analyzerName, resourceType, resourceArn string,
	action []string,
	principal map[string]string,
	isPublic *bool,
) (*Finding, error) {
	b.mu.Lock("AddFinding")
	defer b.mu.Unlock()

	if !b.analyzers.Has(analyzerName) {
		return nil, ErrAnalyzerNotFound
	}

	analyzerARN := b.analyzerARN(analyzerName)
	now := time.Now().UTC()
	f := &Finding{
		ID:           uuid.NewString(),
		AnalyzerArn:  analyzerARN,
		Status:       FindingStatusActive,
		ResourceType: resourceType,
		ResourceArn:  resourceArn,
		Action:       append([]string(nil), action...),
		Principal:    cloneTags(principal),
		IsPublic:     isPublic,
		UpdatedAt:    now,
		CreatedAt:    now,
	}

	b.findings.Put(f)

	return copyFinding(f), nil
}

// GetFinding returns a finding by ID.
func (b *InMemoryBackend) GetFinding(analyzerName, findingID string) (*Finding, error) {
	b.mu.RLock("GetFinding")
	defer b.mu.RUnlock()

	if !b.analyzers.Has(analyzerName) {
		return nil, ErrAnalyzerNotFound
	}

	f, exists := b.findings.Get(findingID)
	if !exists || findingAnalyzerIndexKeyFn(f) != analyzerName {
		return nil, ErrFindingNotFound
	}

	return copyFinding(f), nil
}

// matchesFindingFilter reports whether f satisfies every criterion in
// filter, using the Eq operator on the finding attributes this backend
// tracks as scalar/list fields ("status", "resourceType", "resource", "id").
// A criterion using Contains/Neq/Exists, or keyed by an attribute this
// backend does not model as a direct Finding field (e.g. "principal.AWS",
// "condition.KEY", "action", "isPublic", "createdAt", "resourceRegion"), is
// not evaluated -- the finding is treated as matching that one criterion
// rather than silently excluded, since gopherstack has no honest way to
// decide it doesn't match. See PARITY.md.
func matchesFindingFilter(f *Finding, filter map[string]FilterCriterion) bool {
	for key, crit := range filter {
		if len(crit.Eq) == 0 {
			continue
		}

		var actual string

		switch key {
		case "status":
			actual = string(f.Status)
		case "resourceType":
			actual = f.ResourceType
		case "resource":
			actual = f.ResourceArn
		case "id":
			actual = f.ID
		default:
			continue
		}

		matched := slices.Contains(crit.Eq, actual)

		if !matched {
			return false
		}
	}

	return true
}

// sortFindingAttribute reports the value of the finding attribute named by
// attributeName, matching the same set matchesFindingFilter honours
// ("status", "resourceType", "resource", "id") -- an attribute this backend
// does not track as a direct Finding field (e.g. "createdAt", "isPublic")
// returns "", false, since there is no honest value to sort on.
func sortFindingAttribute(f *Finding, attributeName string) (string, bool) {
	switch attributeName {
	case "status":
		return string(f.Status), true
	case "resourceType":
		return f.ResourceType, true
	case pathResource:
		return f.ResourceArn, true
	case "id":
		return f.ID, true
	default:
		return "", false
	}
}

// sortFindings orders findings by crit, falling back to the default
// ascending-by-ID order when crit is nil or names an attribute this backend
// does not track directly (see sortFindingAttribute).
func sortFindings(findings []*Finding, crit *FindingSortCriteria) {
	if crit == nil {
		sort.Slice(findings, func(i, j int) bool {
			return findings[i].ID < findings[j].ID
		})

		return
	}

	if len(findings) > 0 {
		if _, ok := sortFindingAttribute(findings[0], crit.AttributeName); !ok {
			sort.Slice(findings, func(i, j int) bool {
				return findings[i].ID < findings[j].ID
			})

			return
		}
	}

	desc := crit.OrderBy == "DESC"

	sort.Slice(findings, func(i, j int) bool {
		vi, _ := sortFindingAttribute(findings[i], crit.AttributeName)
		vj, _ := sortFindingAttribute(findings[j], crit.AttributeName)

		if desc {
			return vi > vj
		}

		return vi < vj
	})
}

// ListFindings returns findings for an analyzer, optionally filtered.
func (b *InMemoryBackend) ListFindings(
	analyzerName string,
	filter map[string]FilterCriterion,
	status string,
	sortCrit *FindingSortCriteria,
	maxResults int,
	nextToken string,
) ([]*Finding, string, error) {
	b.mu.RLock("ListFindings")
	defer b.mu.RUnlock()

	if !b.analyzers.Has(analyzerName) {
		return nil, "", ErrAnalyzerNotFound
	}

	group := b.findingsByAnalyzer.Get(analyzerName)
	findings := make([]*Finding, 0, len(group))

	for _, f := range group {
		if status != "" && string(f.Status) != status {
			continue
		}

		if !matchesFindingFilter(f, filter) {
			continue
		}

		findings = append(findings, copyFinding(f))
	}

	sortFindings(findings, sortCrit)

	// Simple token-based pagination by finding ID prefix.
	start := 0

	if nextToken != "" {
		for i, f := range findings {
			if f.ID == nextToken {
				start = i

				break
			}
		}
	}

	findings = findings[start:]

	if maxResults > 0 && len(findings) > maxResults {
		return findings[:maxResults], findings[maxResults].ID, nil
	}

	return findings, "", nil
}

// UpdateFindings archives or marks active the specified findings, selected
// by findingIDs when non-empty, otherwise by resourceArn (both are
// independent optional UpdateFindingsInput members -- serializers.go:
// 3305-3315 guards each with its own `!= nil` check). When both are
// supplied, resourceArn further narrows the findingIDs selection.
func (b *InMemoryBackend) UpdateFindings(
	analyzerName string,
	findingIDs []string,
	resourceArn string,
	status FindingStatus,
) error {
	b.mu.Lock("UpdateFindings")
	defer b.mu.Unlock()

	if !b.analyzers.Has(analyzerName) {
		return ErrAnalyzerNotFound
	}

	now := time.Now().UTC()

	if len(findingIDs) > 0 {
		for _, id := range findingIDs {
			f, exists := b.findings.Get(id)
			if !exists || findingAnalyzerIndexKeyFn(f) != analyzerName {
				continue
			}

			if resourceArn != "" && f.ResourceArn != resourceArn {
				continue
			}

			f.Status = status
			f.UpdatedAt = now
		}

		return nil
	}

	if resourceArn == "" {
		return nil
	}

	for _, f := range b.findingsByAnalyzer.Get(analyzerName) {
		if f.ResourceArn != resourceArn {
			continue
		}

		f.Status = status
		f.UpdatedAt = now
	}

	return nil
}

// GetFindingV2 returns a finding in V2 format (same data, different shape).
func (b *InMemoryBackend) GetFindingV2(analyzerArn, findingID string) (*Finding, error) {
	b.mu.RLock("GetFindingV2")
	defer b.mu.RUnlock()

	var analyzerName string

	for _, a := range b.analyzers.All() {
		if a.Arn == analyzerArn {
			analyzerName = a.Name

			break
		}
	}

	if analyzerName == "" {
		return nil, ErrAnalyzerNotFound
	}

	f, ok := b.findings.Get(findingID)
	if !ok || findingAnalyzerIndexKeyFn(f) != analyzerName {
		return nil, ErrFindingNotFound
	}

	return copyFinding(f), nil
}

// ListFindingsV2 returns findings in V2 format for an analyzer identified by ARN.
func (b *InMemoryBackend) ListFindingsV2(
	analyzerArn, status string,
	filter map[string]FilterCriterion,
	sortCrit *FindingSortCriteria,
	maxResults int,
	nextToken string,
) ([]*Finding, string, error) {
	b.mu.RLock("ListFindingsV2")
	defer b.mu.RUnlock()

	var analyzerName string

	for _, a := range b.analyzers.All() {
		if a.Arn == analyzerArn {
			analyzerName = a.Name

			break
		}
	}

	if analyzerName == "" {
		return nil, "", ErrAnalyzerNotFound
	}

	group := b.findingsByAnalyzer.Get(analyzerName)
	findings := make([]*Finding, 0, len(group))

	for _, f := range group {
		if status != "" && string(f.Status) != status {
			continue
		}

		if !matchesFindingFilter(f, filter) {
			continue
		}

		findings = append(findings, copyFinding(f))
	}

	sortFindings(findings, sortCrit)

	start := 0

	if nextToken != "" {
		for i, f := range findings {
			if f.ID == nextToken {
				start = i

				break
			}
		}
	}

	findings = findings[start:]

	if maxResults > 0 && len(findings) > maxResults {
		return findings[:maxResults], findings[maxResults].ID, nil
	}

	return findings, "", nil
}

// GetFindingsStatistics returns counts of findings by status for an analyzer.
func (b *InMemoryBackend) GetFindingsStatistics(analyzerArn string) (map[string]int, error) {
	b.mu.RLock("GetFindingsStatistics")
	defer b.mu.RUnlock()

	var analyzerName string

	for _, a := range b.analyzers.All() {
		if a.Arn == analyzerArn {
			analyzerName = a.Name

			break
		}
	}

	if analyzerName == "" {
		return nil, ErrAnalyzerNotFound
	}

	counts := map[string]int{
		string(FindingStatusActive):   0,
		string(FindingStatusArchived): 0,
		string(FindingStatusResolved): 0,
	}

	for _, f := range b.findingsByAnalyzer.Get(analyzerName) {
		counts[string(f.Status)]++
	}

	return counts, nil
}

// GenerateFindingRecommendation records a recommendation request for a finding.
func (b *InMemoryBackend) GenerateFindingRecommendation(analyzerArn, findingID string) error {
	b.mu.Lock("GenerateFindingRecommendation")
	defer b.mu.Unlock()

	f, ok := b.findings.Get(findingID)
	if !ok || f.AnalyzerArn != analyzerArn {
		return ErrFindingNotFound
	}

	now := time.Now().UTC()
	completed := now

	rec := &FindingRecommendation{
		ID:                 findingID,
		AnalyzerArn:        analyzerArn,
		ResourceArn:        f.ResourceArn,
		RecommendationType: RecommendationTypeUnusedPermission,
		Status:             "SUCCEEDED",
		StartedAt:          now,
		CompletedAt:        &completed,
	}

	b.findingRecommendations.Put(rec)

	return nil
}

// GetFindingRecommendation returns recommendations for a finding.
func (b *InMemoryBackend) GetFindingRecommendation(
	analyzerArn, findingID string,
) (*FindingRecommendation, error) {
	b.mu.RLock("GetFindingRecommendation")
	defer b.mu.RUnlock()

	rec, ok := b.findingRecommendations.Get(findingID)
	if !ok {
		return nil, ErrFindingNotFound
	}

	if rec.AnalyzerArn != analyzerArn {
		return nil, ErrFindingNotFound
	}

	cp := *rec

	return &cp, nil
}
