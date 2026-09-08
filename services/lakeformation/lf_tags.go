package lakeformation

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

// AddLFTagInternal seeds an LF-tag directly for testing.
func (b *InMemoryBackend) AddLFTagInternal(catalogID, tagKey string, tagValues []string) {
	b.mu.Lock("AddLFTagInternal")
	defer b.mu.Unlock()

	vals := make([]string, len(tagValues))
	copy(vals, tagValues)

	b.lfTags.Put(&LFTag{
		CatalogID: catalogID,
		TagKey:    tagKey,
		TagValues: vals,
	})
}

// CreateLFTag creates a new LF tag with the given values.
func (b *InMemoryBackend) CreateLFTag(catalogID, tagKey string, tagValues []string) error {
	if tagKey == "" {
		return fmt.Errorf("TagKey is required: %w", ErrValidation)
	}

	if len(tagValues) == 0 {
		return fmt.Errorf("TagValues must not be empty: %w", ErrValidation)
	}

	b.mu.Lock("CreateLFTag")
	defer b.mu.Unlock()

	k := lfTagKeyStr(catalogID, tagKey)

	if b.lfTags.Has(k) {
		return awserr.New(
			"LF tag already exists: "+tagKey,
			awserr.ErrAlreadyExists,
		)
	}

	vals := make([]string, len(tagValues))
	copy(vals, tagValues)

	b.lfTags.Put(&LFTag{
		CatalogID: catalogID,
		TagKey:    tagKey,
		TagValues: vals,
	})

	return nil
}

// DeleteLFTag removes a LF tag.
func (b *InMemoryBackend) DeleteLFTag(catalogID, tagKey string) error {
	if tagKey == "" {
		return fmt.Errorf("TagKey is required: %w", ErrValidation)
	}

	b.mu.Lock("DeleteLFTag")
	defer b.mu.Unlock()

	k := lfTagKeyStr(catalogID, tagKey)

	if !b.lfTags.Has(k) {
		return awserr.New(
			"LF tag not found: "+tagKey,
			awserr.ErrNotFound,
		)
	}

	b.lfTags.Delete(k)
	b.detachLFTagFromAllResourcesLocked(catalogID, tagKey)

	return nil
}

// detachLFTagFromAllResourcesLocked removes every resourceLFTags entry for
// (catalogID, tagKey). Tag keys are reusable, so a stale attachment would let
// a re-created key inherit LFTagPolicy grants matched against it. Caller must
// hold b.mu for writing.
func (b *InMemoryBackend) detachLFTagFromAllResourcesLocked(catalogID, tagKey string) {
	for resourceKey, pairs := range b.resourceLFTags {
		remaining := slices.DeleteFunc(slices.Clone(pairs), func(p LFTagPair) bool {
			return p.CatalogID == catalogID && p.TagKey == tagKey
		})

		if len(remaining) == 0 {
			delete(b.resourceLFTags, resourceKey)
		} else {
			b.resourceLFTags[resourceKey] = remaining
		}
	}
}

// GetLFTag returns the LF tag for the given catalog and key.
func (b *InMemoryBackend) GetLFTag(catalogID, tagKey string) (*LFTag, error) {
	if tagKey == "" {
		return nil, fmt.Errorf("TagKey is required: %w", ErrValidation)
	}

	b.mu.RLock("GetLFTag")
	defer b.mu.RUnlock()

	k := lfTagKeyStr(catalogID, tagKey)

	tag, ok := b.lfTags.Get(k)
	if !ok {
		return nil, awserr.New(
			"LF tag not found: "+tagKey,
			awserr.ErrNotFound,
		)
	}

	return copyLFTag(tag), nil
}

// UpdateLFTag adds and removes values from an existing LF tag.
// TagValues are sorted after modification for deterministic output.
func (b *InMemoryBackend) UpdateLFTag(catalogID, tagKey string, tagValuesToAdd, tagValuesToDelete []string) error {
	if tagKey == "" {
		return fmt.Errorf("TagKey is required: %w", ErrValidation)
	}

	b.mu.Lock("UpdateLFTag")
	defer b.mu.Unlock()

	k := lfTagKeyStr(catalogID, tagKey)

	tag, ok := b.lfTags.Get(k)
	if !ok {
		return awserr.New(
			"LF tag not found: "+tagKey,
			awserr.ErrNotFound,
		)
	}

	vals := tag.TagValues

	for _, v := range tagValuesToAdd {
		if !slices.Contains(vals, v) {
			vals = append(vals, v)
		}
	}

	for _, v := range tagValuesToDelete {
		vals = slices.DeleteFunc(vals, func(s string) bool { return s == v })
	}

	sort.Strings(vals)
	tag.TagValues = vals

	return nil
}

// ListLFTags returns a paginated list of LF tags for the given catalog.
// ListLFTags returns the account's LF-tags. resourceShareType FOREIGN always
// returns none: this backend models a single account with no RAM
// cross-account sharing, so no LF-tag is ever foreign
// (api_op_ListLFTags.go, lakeformation@v1.50.4) -- gopherstack-4ly2.
func (b *InMemoryBackend) ListLFTags(
	catalogID, resourceShareType string, maxResults int, nextToken string,
) ([]*LFTag, string) {
	b.mu.RLock("ListLFTags")
	defer b.mu.RUnlock()

	if resourceShareType == "FOREIGN" {
		return nil, ""
	}

	all := make([]*LFTag, 0, b.lfTags.Len())

	for _, t := range b.lfTags.All() {
		if catalogID == "" || t.CatalogID == catalogID {
			all = append(all, copyLFTag(t))
		}
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].CatalogID != all[j].CatalogID {
			return all[i].CatalogID < all[j].CatalogID
		}

		return all[i].TagKey < all[j].TagKey
	})

	return paginate(all, maxResults, nextToken, defaultMaxResults)
}

// copyLFTag returns a deep copy of the LFTag, including a copy of TagValues.
func copyLFTag(t *LFTag) *LFTag {
	if t == nil {
		return nil
	}

	cp := *t

	if t.TagValues != nil {
		cp.TagValues = make([]string, len(t.TagValues))
		copy(cp.TagValues, t.TagValues)
	}

	return &cp
}

// AddLFTagsToResource attaches LF-tags to the specified resource.
// Valid tags are always stored; failures are returned for any tag not found.
// This mirrors AWS behavior where valid tags are applied even if some fail.
func (b *InMemoryBackend) AddLFTagsToResource(catalogID string, resource *Resource, lfTags []LFTagPair) []LFTagError {
	b.mu.Lock("AddLFTagsToResource")
	defer b.mu.Unlock()

	failures := make([]LFTagError, 0, len(lfTags))
	resourceKey := resourceToKey(resource)
	existing := b.resourceLFTags[resourceKey]

	for _, pair := range lfTags {
		k := lfTagKeyStr(catalogID, pair.TagKey)
		tag, ok := b.lfTags.Get(k)
		if !ok {
			failures = append(failures, LFTagError{
				LFTag: &pair,
				Error: &errorDetail{
					ErrorCode:    "EntityNotFoundException",
					ErrorMessage: "LF tag not found: " + pair.TagKey,
				},
			})

			continue
		}

		// Validate tag values against allowed values.
		invalid := false
		for _, v := range pair.TagValues {
			if !slices.Contains(tag.TagValues, v) {
				pair := pair
				failures = append(failures, LFTagError{
					LFTag: &pair,
					Error: &errorDetail{
						ErrorCode:    errCodeInvalidInput,
						ErrorMessage: "invalid tag value: " + v,
					},
				})
				invalid = true

				break
			}
		}
		if invalid {
			continue
		}

		// Store the valid tag association.
		found := false

		for i, ex := range existing {
			if ex.TagKey == pair.TagKey {
				existing[i] = pair
				found = true

				break
			}
		}

		if !found {
			existing = append(existing, pair)
		}
	}

	b.resourceLFTags[resourceKey] = existing

	return failures
}

// RemoveLFTagsFromResource detaches LF-tags from the specified resource.
// Failures are returned for any tag not currently attached to the resource.
func (b *InMemoryBackend) RemoveLFTagsFromResource(
	_ string,
	resource *Resource,
	lfTags []LFTagPair,
) []LFTagError {
	b.mu.Lock("RemoveLFTagsFromResource")
	defer b.mu.Unlock()

	failures := make([]LFTagError, 0, len(lfTags))
	resourceKey := resourceToKey(resource)
	existing := b.resourceLFTags[resourceKey]

	for _, pair := range lfTags {
		found := false

		for _, ex := range existing {
			if ex.TagKey == pair.TagKey {
				found = true

				break
			}
		}

		if !found {
			pair := pair
			failures = append(failures, LFTagError{
				LFTag: &pair,
				Error: &errorDetail{
					ErrorCode:    "EntityNotFoundException",
					ErrorMessage: "LF tag not attached to resource: " + pair.TagKey,
				},
			})

			continue
		}

		existing = slices.DeleteFunc(existing, func(e LFTagPair) bool {
			return e.TagKey == pair.TagKey
		})
	}

	if len(existing) == 0 {
		delete(b.resourceLFTags, resourceKey)
	} else {
		b.resourceLFTags[resourceKey] = existing
	}

	return failures
}

// validLFTagResourceKind reports whether resource is a Database, Table, or
// TableWithColumns resource -- the only kinds GetResourceLFTags/
// AddLFTagsToResource/RemoveLFTagsFromResource accept ("The database, table,
// or column resource...", api_op_GetResourceLFTags.go:30-33 and
// api_op_AddLFTagsToResource.go:29-31; RemoveLFTagsFromResource spells it out
// explicitly: "Only database, table, or tableWithColumns resource are
// allowed.", api_op_RemoveLFTagsFromResource.go:12-14, aws-sdk-go-v2/service/
// lakeformation@v1.50.4). gopherstack previously accepted every Resource
// union member here (a permissive superset of what AWS accepts).
func validLFTagResourceKind(r *Resource) bool {
	return r != nil && (r.Database != nil || r.Table != nil || r.TableWithColumns != nil)
}

// GetResourceLFTags returns the LF-tags currently attached to a resource.
func (b *InMemoryBackend) GetResourceLFTags(_ string, resource *Resource) ([]LFTagPair, error) {
	if resource == nil {
		return nil, fmt.Errorf("resource is required: %w", ErrValidation)
	}
	if !validLFTagResourceKind(resource) {
		return nil, fmt.Errorf("Resource must be a Database, Table, or TableWithColumns resource: %w", ErrValidation)
	}

	b.mu.RLock("GetResourceLFTags")
	defer b.mu.RUnlock()

	resourceKey := resourceToKey(resource)
	pairs := b.resourceLFTags[resourceKey]

	result := make([]LFTagPair, len(pairs))
	copy(result, pairs)

	return result, nil
}

const (
	lfSplitInTwo  = 2  // SplitN limit for two-part key parsing
	lfItoaInitCap = 10 // initial capacity for itoa byte slice
)

// ---------------------------------------------------------------------------
// SearchTablesByLFTags — real implementation
// ---------------------------------------------------------------------------

// SearchTablesByLFTags returns tables whose LF-tags match all of the given expression tags.
// It scans b.resourceLFTags for resources of type "table:" and checks whether all
// expression tags are present in the resource's tag set.
func (b *InMemoryBackend) SearchTablesByLFTags(
	expression []LFTag,
	catalogID string,
	maxResults int,
	nextToken string,
) ([]TaggedTable, string) {
	b.mu.RLock("SearchTablesByLFTags")
	defer b.mu.RUnlock()

	const tablePrefix = "table:"

	results := make([]TaggedTable, 0, len(b.resourceLFTags))

	for key, pairs := range b.resourceLFTags {
		if !strings.HasPrefix(key, tablePrefix) {
			continue
		}

		// Check whether all expression tags match.
		if !lfTagsMatchExpression(pairs, expression) {
			continue
		}

		// Extract database + table name from key (format: "table:{db}.{table}").
		rest := strings.TrimPrefix(key, tablePrefix)
		parts := strings.SplitN(rest, ".", lfSplitInTwo)

		if len(parts) != lfSplitInTwo {
			continue
		}

		dbName := parts[0]
		tableName := parts[1]

		tagged := TaggedTable{
			Table: &TableResource{
				CatalogID:    catalogID,
				DatabaseName: dbName,
				Name:         tableName,
			},
			LFTagsOnTable: cloneLFTagPairs(pairs),
		}

		results = append(results, tagged)
	}

	return paginateTaggedTables(results, maxResults, nextToken)
}

// SearchDatabasesByLFTags returns databases whose LF-tags match all of the given expression tags.
func (b *InMemoryBackend) SearchDatabasesByLFTags(
	expression []LFTag,
	_ string, // catalogID: reserved for future multi-catalog support
	maxResults int,
	nextToken string,
) ([]TaggedDatabase, string) {
	b.mu.RLock("SearchDatabasesByLFTags")
	defer b.mu.RUnlock()

	const dbPrefix = "database:"

	results := make([]TaggedDatabase, 0, len(b.resourceLFTags))

	for key, pairs := range b.resourceLFTags {
		if !strings.HasPrefix(key, dbPrefix) {
			continue
		}

		if !lfTagsMatchExpression(pairs, expression) {
			continue
		}

		dbName := strings.TrimPrefix(key, dbPrefix)

		tagged := TaggedDatabase{
			Database: &DatabaseResource{Name: dbName},
			LFTags:   cloneLFTagPairs(pairs),
		}

		results = append(results, tagged)
	}

	return paginateTaggedDatabases(results, maxResults, nextToken)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// lfTagsMatchExpression returns true when all expression tags are satisfied by pairs.
// An expression tag is satisfied when the same TagKey appears in pairs with a TagValues
// that contains at least one of the expression tag's values.
func lfTagsMatchExpression(pairs []LFTagPair, expression []LFTag) bool {
	for _, exprTag := range expression {
		found := false

		for _, pair := range pairs {
			if pair.TagKey != exprTag.TagKey {
				continue
			}
			// Check if any value in pair.TagValues matches any value in exprTag.TagValues.
			for _, pv := range pair.TagValues {
				if slices.Contains(exprTag.TagValues, pv) {
					found = true

					break
				}
			}
			if found {
				break
			}
		}

		if !found {
			return false
		}
	}

	return true
}

func cloneLFTagPairs(pairs []LFTagPair) []LFTagPair {
	if pairs == nil {
		return nil
	}

	cp := make([]LFTagPair, len(pairs))
	for i, p := range pairs {
		cp[i] = LFTagPair{
			TagKey:    p.TagKey,
			TagValues: append([]string(nil), p.TagValues...),
		}
	}

	return cp
}

func paginateTaggedTables(list []TaggedTable, maxResults int, nextToken string) ([]TaggedTable, string) {
	const defaultMax = 100

	if maxResults <= 0 {
		maxResults = defaultMax
	}

	startIdx := decodeLFPageToken(nextToken)

	if startIdx >= len(list) {
		return []TaggedTable{}, ""
	}

	end := startIdx + maxResults
	var outToken string

	if end < len(list) {
		outToken = itoa(end)
	} else {
		end = len(list)
	}

	return list[startIdx:end], outToken
}

func paginateTaggedDatabases(list []TaggedDatabase, maxResults int, nextToken string) ([]TaggedDatabase, string) {
	const defaultMax = 100

	if maxResults <= 0 {
		maxResults = defaultMax
	}

	startIdx := decodeLFPageToken(nextToken)

	if startIdx >= len(list) {
		return []TaggedDatabase{}, ""
	}

	end := startIdx + maxResults
	var outToken string

	if end < len(list) {
		outToken = itoa(end)
	} else {
		end = len(list)
	}

	return list[startIdx:end], outToken
}

// decodeLFPageToken parses a paginateTaggedTables/paginateTaggedDatabases
// nextToken (a plain decimal offset, produced by itoa) into a non-negative
// slice start index. Uses strconv.Atoi rather than a hand-rolled digit loop
// so a too-long or non-numeric token is rejected outright (ErrSyntax /
// ErrRange) instead of silently overflowing into a negative offset that
// would slip past the `startIdx >= len(list)` guard and panic.
func decodeLFPageToken(token string) int {
	if token == "" {
		return 0
	}

	n, err := strconv.Atoi(token)
	if err != nil || n < 0 {
		return 0
	}

	return n
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	result := make([]byte, 0, lfItoaInitCap)

	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}

	return string(result)
}
