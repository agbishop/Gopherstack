package inspector2

import (
	"fmt"
	"maps"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	filterNameMinLen = 3
	filterNameMaxLen = 64

	filterActionNone     = "NONE"
	filterActionSuppress = "SUPPRESS"
)

// onceFilterNamePattern lazily compiles the real Inspector2 filter-name
// pattern (alphanumeric plus dot/underscore/dash), exactly once.
//
//nolint:gochecknoglobals // read-only package-level regexp, built once via sync.OnceValue
var onceFilterNamePattern = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
})

// validateFilterName enforces AWS's real CreateFilter name constraint: 3-64
// characters, alphanumeric plus dot/underscore/dash. Real AWS returns
// ValidationException for violations; this backend previously accepted any
// non-empty string.
func validateFilterName(name string) error {
	if len(name) < filterNameMinLen || len(name) > filterNameMaxLen {
		return fmt.Errorf(
			"%w: filter name must be between %d and %d characters, got %d",
			ErrValidation, filterNameMinLen, filterNameMaxLen, len(name),
		)
	}

	if !onceFilterNamePattern().MatchString(name) {
		return fmt.Errorf(
			"%w: filter name must contain only alphanumeric characters, dots, underscores, and dashes",
			ErrValidation,
		)
	}

	return nil
}

// validateFilterAction returns an error if action is not a valid Inspector2 filter action.
func validateFilterAction(action string) error {
	validActions := map[string]bool{
		filterActionNone:     true,
		filterActionSuppress: true,
	}
	if action == "" || validActions[action] {
		return nil
	}

	return fmt.Errorf("%w: filter action must be NONE or SUPPRESS, got %q", ErrValidation, action)
}

// suppressMatchingFindings applies f's SUPPRESS action to every currently-
// ACTIVE finding matching its criteria, transitioning them to SUPPRESSED.
// Real CreateFilter's own doc comment ("When the filter action is set to
// SUPPRESS this action creates a suppression rule") describes an ongoing
// rule, not a one-off action -- the finding-creation-time half of that rule
// (newly seeded findings matching an already-active SUPPRESS filter) is
// handled by matchesSuppressFilter, called from findings.go's
// SeedFinding/AddFinding. Reverting a finding to ACTIVE when a filter is
// later deleted or its action changed away from SUPPRESS is not modeled:
// neither the SDK doc comments nor the API Reference say whether real
// Inspector2 does this, and guessing wrong would trade a disclosed gap for a
// fabricated behavior. Caller must already hold b.mu.
func (b *InMemoryBackend) suppressMatchingFindings(f *Filter) {
	if f.Action != filterActionSuppress {
		return
	}

	fc := parseFindingFilterCriteria(f.Criteria)

	b.findings.Range(func(sf *storedFinding) bool {
		if sf.Status == findingStatusActive && fc.matches(&sf.Finding) {
			sf.Status = findingStatusSuppressed
		}

		return true
	})
}

// matchesSuppressFilter reports whether f matches any stored SUPPRESS-action
// filter's criteria -- see suppressMatchingFindings for the retroactive
// half of the same rule. Caller must already hold b.mu.
func (b *InMemoryBackend) matchesSuppressFilter(f *Finding) bool {
	matched := false

	b.filters.Range(func(filt *Filter) bool {
		if filt.Action != filterActionSuppress {
			return true
		}

		if parseFindingFilterCriteria(filt.Criteria).matches(f) {
			matched = true

			return false
		}

		return true
	})

	return matched
}

func (b *InMemoryBackend) buildFilterARN() string {
	id := uuid.New().String()

	return arn.Build(inspector2Service, b.region, b.accountID, "filter/"+id)
}

// CreateFilter creates a new findings filter.
func (b *InMemoryBackend) CreateFilter(
	name, action, description, reason string,
	criteria map[string]any,
	tags map[string]string,
) (*Filter, error) {
	b.mu.Lock("CreateFilter")
	defer b.mu.Unlock()

	if err := validateFilterName(name); err != nil {
		return nil, err
	}

	if err := validateFilterAction(action); err != nil {
		return nil, err
	}

	if err := validateTags(tags); err != nil {
		return nil, err
	}

	nameTaken := false
	b.filters.Range(func(f *Filter) bool {
		if f.Name == name {
			nameTaken = true

			return false
		}

		return true
	})

	if nameTaken {
		return nil, ErrFilterAlreadyExists
	}

	filterARN := b.buildFilterARN()
	now := time.Now().UTC()

	f := &Filter{
		Arn:         filterARN,
		Name:        name,
		Action:      action,
		Description: description,
		Reason:      reason,
		OwnerID:     b.accountID,
		CreatedAt:   now,
		UpdatedAt:   now,
		Criteria:    criteria,
		Tags:        tags,
	}

	b.filters.Put(f)
	b.suppressMatchingFindings(f)

	if len(tags) > 0 {
		b.tags[filterARN] = maps.Clone(tags)
	}

	return f, nil
}

// UpdateFilter updates an existing filter.
func (b *InMemoryBackend) UpdateFilter(
	filterARN, action, description, reason string,
	criteria map[string]any,
) (*Filter, error) {
	b.mu.Lock("UpdateFilter")
	defer b.mu.Unlock()

	if err := validateFilterAction(action); err != nil {
		return nil, err
	}

	f, ok := b.filters.Get(filterARN)
	if !ok {
		return nil, ErrFilterNotFound
	}

	if action != "" {
		f.Action = action
	}

	if description != "" {
		f.Description = description
	}

	if reason != "" {
		f.Reason = reason
	}

	if criteria != nil {
		f.Criteria = criteria
	}

	f.UpdatedAt = time.Now().UTC()
	b.suppressMatchingFindings(f)

	return f, nil
}

// DeleteFilter deletes a filter by ARN.
func (b *InMemoryBackend) DeleteFilter(filterARN string) error {
	b.mu.Lock("DeleteFilter")
	defer b.mu.Unlock()

	if !b.filters.Delete(filterARN) {
		return ErrFilterNotFound
	}

	delete(b.tags, filterARN)

	return nil
}

// ListFilters returns all filters, optionally filtered by ARNs and action.
func (b *InMemoryBackend) ListFilters(arns []string, action string) ([]*Filter, error) {
	b.mu.RLock("ListFilters")
	defer b.mu.RUnlock()

	arnSet := make(map[string]bool, len(arns))
	for _, a := range arns {
		arnSet[a] = true
	}

	var result []*Filter

	for _, f := range b.filters.Snapshot() {
		if len(arnSet) > 0 && !arnSet[f.Arn] {
			continue
		}

		if action != "" && f.Action != action {
			continue
		}

		result = append(result, f)
	}

	return result, nil
}
