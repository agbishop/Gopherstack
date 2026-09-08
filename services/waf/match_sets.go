package waf

// match_sets.go groups the WAF Classic match-set families whose CRUD shape
// is nearly identical (a name, a changeToken, and a slice of tuples/strings
// updated via INSERT/DELETE): ByteMatchSet, SizeConstraintSet,
// SqlInjectionMatchSet, XssMatchSet, GeoMatchSet, RegexPatternSet, and
// RegexMatchSet. Splitting these into one file per family previously tripped
// golangci-lint's dupl (clone-detection) check across file boundaries;
// keeping them together in a single file resolves that by construction,
// with no lint suppression needed.

import (
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// --- ByteMatchSet ---

func (b *InMemoryBackend) byteMatchSetARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("bytematchset/%s", id))
}

// CreateByteMatchSet creates a new ByteMatchSet.
func (b *InMemoryBackend) CreateByteMatchSet(name, changeToken string) (*ByteMatchSet, error) {
	b.mu.Lock("CreateByteMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	bms := &ByteMatchSet{
		ByteMatchSetId:  id,
		Name:            name,
		ByteMatchTuples: []ByteMatchTuple{},
	}
	b.byteMatchSets.Put(bms)

	return bms, nil
}

// GetByteMatchSet retrieves a ByteMatchSet by ID.
func (b *InMemoryBackend) GetByteMatchSet(id string) (*ByteMatchSet, error) {
	b.mu.RLock("GetByteMatchSet")
	defer b.mu.RUnlock()

	bms, ok := b.byteMatchSets.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return bms, nil
}

// UpdateByteMatchSet updates a ByteMatchSet's tuples.
func (b *InMemoryBackend) UpdateByteMatchSet(id, changeToken string, updates []ByteMatchSetUpdate) error {
	b.mu.Lock("UpdateByteMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	bms, ok := b.byteMatchSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	for _, u := range updates {
		tuples, err := applyEntryUpdate(bms.ByteMatchTuples, u.Action, u.ByteMatchTuple,
			func(a, b ByteMatchTuple) bool {
				return a.TargetString == b.TargetString && a.FieldToMatch.Type == b.FieldToMatch.Type
			})
		if err != nil {
			return err
		}

		bms.ByteMatchTuples = tuples
	}

	return nil
}

// DeleteByteMatchSet deletes a ByteMatchSet. Real AWS rejects deletion
// while it is still used by a Rule (WAFReferencedItemException) or still
// contains any ByteMatchTuples (WAFNonEmptyEntityException).
func (b *InMemoryBackend) DeleteByteMatchSet(id, changeToken string) error {
	b.mu.Lock("DeleteByteMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	bms, ok := b.byteMatchSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	if b.matchSetReferenced(id) {
		return ErrReferencedItem
	}

	if len(bms.ByteMatchTuples) > 0 {
		return ErrNonEmptyEntity
	}

	b.byteMatchSets.Delete(id)
	delete(b.tags, b.byteMatchSetARN(id))

	return nil
}

// ListByteMatchSets returns summaries of all ByteMatchSets.
func (b *InMemoryBackend) ListByteMatchSets() []ByteMatchSetSummary {
	b.mu.RLock("ListByteMatchSets")
	defer b.mu.RUnlock()

	all := b.byteMatchSets.All()
	result := make([]ByteMatchSetSummary, 0, len(all))
	for _, s := range all {
		result = append(result, ByteMatchSetSummary{ByteMatchSetId: s.ByteMatchSetId, Name: s.Name})
	}

	sort.Slice(
		result,
		func(i, j int) bool { return result[i].ByteMatchSetId < result[j].ByteMatchSetId },
	)

	return result
}

// --- SizeConstraintSet ---

func (b *InMemoryBackend) sizeConstraintSetARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("sizeconstraintset/%s", id))
}

// CreateSizeConstraintSet creates a new SizeConstraintSet.
func (b *InMemoryBackend) CreateSizeConstraintSet(name, changeToken string) (*SizeConstraintSet, error) {
	b.mu.Lock("CreateSizeConstraintSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	scs := &SizeConstraintSet{
		SizeConstraintSetId: id,
		Name:                name,
		SizeConstraints:     []SizeConstraint{},
	}
	b.sizeConstraintSets.Put(scs)

	return scs, nil
}

// GetSizeConstraintSet retrieves a SizeConstraintSet by ID.
func (b *InMemoryBackend) GetSizeConstraintSet(id string) (*SizeConstraintSet, error) {
	b.mu.RLock("GetSizeConstraintSet")
	defer b.mu.RUnlock()

	scs, ok := b.sizeConstraintSets.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return scs, nil
}

// UpdateSizeConstraintSet updates a SizeConstraintSet's constraints.
func (b *InMemoryBackend) UpdateSizeConstraintSet(
	id, changeToken string,
	updates []SizeConstraintSetUpdate,
) error {
	b.mu.Lock("UpdateSizeConstraintSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	scs, ok := b.sizeConstraintSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	for _, u := range updates {
		constraints, err := applyEntryUpdate(scs.SizeConstraints, u.Action, u.SizeConstraint,
			func(a, b SizeConstraint) bool {
				return a.FieldToMatch.Type == b.FieldToMatch.Type && a.Size == b.Size
			})
		if err != nil {
			return err
		}

		scs.SizeConstraints = constraints
	}

	return nil
}

// DeleteSizeConstraintSet deletes a SizeConstraintSet. Real AWS rejects
// deletion while it is still used by a Rule (WAFReferencedItemException) or
// still contains any SizeConstraints (WAFNonEmptyEntityException).
func (b *InMemoryBackend) DeleteSizeConstraintSet(id, changeToken string) error {
	b.mu.Lock("DeleteSizeConstraintSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	scs, ok := b.sizeConstraintSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	if b.matchSetReferenced(id) {
		return ErrReferencedItem
	}

	if len(scs.SizeConstraints) > 0 {
		return ErrNonEmptyEntity
	}

	b.sizeConstraintSets.Delete(id)
	delete(b.tags, b.sizeConstraintSetARN(id))

	return nil
}

// ListSizeConstraintSets returns summaries of all SizeConstraintSets.
func (b *InMemoryBackend) ListSizeConstraintSets() []SizeConstraintSetSummary {
	b.mu.RLock("ListSizeConstraintSets")
	defer b.mu.RUnlock()

	all := b.sizeConstraintSets.All()
	result := make([]SizeConstraintSetSummary, 0, len(all))
	for _, s := range all {
		result = append(
			result,
			SizeConstraintSetSummary{SizeConstraintSetId: s.SizeConstraintSetId, Name: s.Name},
		)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].SizeConstraintSetId < result[j].SizeConstraintSetId
	})

	return result
}

// --- SqlInjectionMatchSet ---

func (b *InMemoryBackend) sqlInjectionMatchSetARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("sqlinjectionmatchset/%s", id))
}

// CreateSqlInjectionMatchSet creates a new SqlInjectionMatchSet.
//
//nolint:revive,staticcheck // AWS SDK naming
func (b *InMemoryBackend) CreateSqlInjectionMatchSet(
	name, changeToken string,
) (*SqlInjectionMatchSet, error) {
	b.mu.Lock("CreateSqlInjectionMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	sims := &SqlInjectionMatchSet{
		SqlInjectionMatchSetId:  id,
		Name:                    name,
		SqlInjectionMatchTuples: []SqlInjectionMatchTuple{},
	}
	b.sqlInjectionMatchSets.Put(sims)

	return sims, nil
}

// GetSqlInjectionMatchSet retrieves a SqlInjectionMatchSet by ID.
//
//nolint:revive,staticcheck // AWS SDK naming
func (b *InMemoryBackend) GetSqlInjectionMatchSet(id string) (*SqlInjectionMatchSet, error) {
	b.mu.RLock("GetSqlInjectionMatchSet")
	defer b.mu.RUnlock()

	sims, ok := b.sqlInjectionMatchSets.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return sims, nil
}

// UpdateSqlInjectionMatchSet updates a SqlInjectionMatchSet's tuples.
//
//nolint:revive,staticcheck // AWS SDK naming
func (b *InMemoryBackend) UpdateSqlInjectionMatchSet(
	id, changeToken string,
	updates []SqlInjectionMatchSetUpdate,
) error {
	b.mu.Lock("UpdateSqlInjectionMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	sims, ok := b.sqlInjectionMatchSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	for _, u := range updates {
		tuples, err := applyEntryUpdate(sims.SqlInjectionMatchTuples, u.Action, u.SqlInjectionMatchTuple,
			func(a, b SqlInjectionMatchTuple) bool {
				return a.FieldToMatch.Type == b.FieldToMatch.Type && a.TextTransformation == b.TextTransformation
			})
		if err != nil {
			return err
		}

		sims.SqlInjectionMatchTuples = tuples
	}

	return nil
}

// DeleteSqlInjectionMatchSet deletes a SqlInjectionMatchSet. Real AWS
// rejects deletion while it is still used by a Rule
// (WAFReferencedItemException) or still contains any
// SqlInjectionMatchTuples (WAFNonEmptyEntityException).
//
//nolint:revive,staticcheck // AWS SDK naming
func (b *InMemoryBackend) DeleteSqlInjectionMatchSet(id, changeToken string) error {
	b.mu.Lock("DeleteSqlInjectionMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	sims, ok := b.sqlInjectionMatchSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	if b.matchSetReferenced(id) {
		return ErrReferencedItem
	}

	if len(sims.SqlInjectionMatchTuples) > 0 {
		return ErrNonEmptyEntity
	}

	b.sqlInjectionMatchSets.Delete(id)
	delete(b.tags, b.sqlInjectionMatchSetARN(id))

	return nil
}

// ListSqlInjectionMatchSets returns summaries of all SqlInjectionMatchSets.
//
//nolint:revive,staticcheck // AWS SDK naming
func (b *InMemoryBackend) ListSqlInjectionMatchSets() []SqlInjectionMatchSetSummary {
	b.mu.RLock("ListSqlInjectionMatchSets")
	defer b.mu.RUnlock()

	all := b.sqlInjectionMatchSets.All()
	result := make([]SqlInjectionMatchSetSummary, 0, len(all))
	for _, s := range all {
		result = append(result, SqlInjectionMatchSetSummary{
			SqlInjectionMatchSetId: s.SqlInjectionMatchSetId,
			Name:                   s.Name,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].SqlInjectionMatchSetId < result[j].SqlInjectionMatchSetId
	})

	return result
}

// --- XssMatchSet ---

func (b *InMemoryBackend) xssMatchSetARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("xssmatchset/%s", id))
}

// CreateXssMatchSet creates a new XssMatchSet.
//
//nolint:revive,staticcheck // AWS SDK naming
func (b *InMemoryBackend) CreateXssMatchSet(name, changeToken string) (*XssMatchSet, error) {
	b.mu.Lock("CreateXssMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	xms := &XssMatchSet{
		XssMatchSetId:  id,
		Name:           name,
		XssMatchTuples: []XssMatchTuple{},
	}
	b.xssMatchSets.Put(xms)

	return xms, nil
}

// GetXssMatchSet retrieves an XssMatchSet by ID.
//
//nolint:revive,staticcheck // AWS SDK naming
func (b *InMemoryBackend) GetXssMatchSet(id string) (*XssMatchSet, error) {
	b.mu.RLock("GetXssMatchSet")
	defer b.mu.RUnlock()

	xms, ok := b.xssMatchSets.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return xms, nil
}

// UpdateXssMatchSet updates an XssMatchSet's tuples.
//
//nolint:revive,staticcheck // AWS SDK naming
func (b *InMemoryBackend) UpdateXssMatchSet(id, changeToken string, updates []XssMatchSetUpdate) error {
	b.mu.Lock("UpdateXssMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	xms, ok := b.xssMatchSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	for _, u := range updates {
		tuples, err := applyEntryUpdate(xms.XssMatchTuples, u.Action, u.XssMatchTuple,
			func(a, b XssMatchTuple) bool {
				return a.FieldToMatch.Type == b.FieldToMatch.Type && a.TextTransformation == b.TextTransformation
			})
		if err != nil {
			return err
		}

		xms.XssMatchTuples = tuples
	}

	return nil
}

// DeleteXssMatchSet deletes an XssMatchSet. Real AWS rejects deletion
// while it is still used by a Rule (WAFReferencedItemException) or still
// contains any XssMatchTuples (WAFNonEmptyEntityException).
//
//nolint:revive,staticcheck // AWS SDK naming
func (b *InMemoryBackend) DeleteXssMatchSet(id, changeToken string) error {
	b.mu.Lock("DeleteXssMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	xms, ok := b.xssMatchSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	if b.matchSetReferenced(id) {
		return ErrReferencedItem
	}

	if len(xms.XssMatchTuples) > 0 {
		return ErrNonEmptyEntity
	}

	b.xssMatchSets.Delete(id)
	delete(b.tags, b.xssMatchSetARN(id))

	return nil
}

// ListXssMatchSets returns summaries of all XssMatchSets.
//
//nolint:revive,staticcheck // AWS SDK naming
func (b *InMemoryBackend) ListXssMatchSets() []XssMatchSetSummary {
	b.mu.RLock("ListXssMatchSets")
	defer b.mu.RUnlock()

	all := b.xssMatchSets.All()
	result := make([]XssMatchSetSummary, 0, len(all))
	for _, s := range all {
		result = append(result, XssMatchSetSummary{XssMatchSetId: s.XssMatchSetId, Name: s.Name})
	}

	sort.Slice(
		result,
		func(i, j int) bool { return result[i].XssMatchSetId < result[j].XssMatchSetId },
	)

	return result
}

// --- GeoMatchSet ---

func (b *InMemoryBackend) geoMatchSetARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("geomatchset/%s", id))
}

// CreateGeoMatchSet creates a new GeoMatchSet.
func (b *InMemoryBackend) CreateGeoMatchSet(name, changeToken string) (*GeoMatchSet, error) {
	b.mu.Lock("CreateGeoMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	gms := &GeoMatchSet{
		GeoMatchSetId:       id,
		Name:                name,
		GeoMatchConstraints: []GeoMatchConstraint{},
	}
	b.geoMatchSets.Put(gms)

	return gms, nil
}

// GetGeoMatchSet retrieves a GeoMatchSet by ID.
func (b *InMemoryBackend) GetGeoMatchSet(id string) (*GeoMatchSet, error) {
	b.mu.RLock("GetGeoMatchSet")
	defer b.mu.RUnlock()

	gms, ok := b.geoMatchSets.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return gms, nil
}

// UpdateGeoMatchSet updates a GeoMatchSet's constraints.
func (b *InMemoryBackend) UpdateGeoMatchSet(id, changeToken string, updates []GeoMatchSetUpdate) error {
	b.mu.Lock("UpdateGeoMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	gms, ok := b.geoMatchSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	for _, u := range updates {
		constraints, err := applyEntryUpdate(gms.GeoMatchConstraints, u.Action, u.GeoMatchConstraint,
			func(a, b GeoMatchConstraint) bool { return a.Type == b.Type && a.Value == b.Value })
		if err != nil {
			return err
		}

		gms.GeoMatchConstraints = constraints
	}

	return nil
}

// DeleteGeoMatchSet deletes a GeoMatchSet. Real AWS rejects deletion while
// it is still used by a Rule (WAFReferencedItemException) or still
// contains any GeoMatchConstraints (WAFNonEmptyEntityException).
func (b *InMemoryBackend) DeleteGeoMatchSet(id, changeToken string) error {
	b.mu.Lock("DeleteGeoMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	gms, ok := b.geoMatchSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	if b.matchSetReferenced(id) {
		return ErrReferencedItem
	}

	if len(gms.GeoMatchConstraints) > 0 {
		return ErrNonEmptyEntity
	}

	b.geoMatchSets.Delete(id)
	delete(b.tags, b.geoMatchSetARN(id))

	return nil
}

// ListGeoMatchSets returns summaries of all GeoMatchSets.
func (b *InMemoryBackend) ListGeoMatchSets() []GeoMatchSetSummary {
	b.mu.RLock("ListGeoMatchSets")
	defer b.mu.RUnlock()

	all := b.geoMatchSets.All()
	result := make([]GeoMatchSetSummary, 0, len(all))
	for _, s := range all {
		result = append(result, GeoMatchSetSummary{GeoMatchSetId: s.GeoMatchSetId, Name: s.Name})
	}

	sort.Slice(
		result,
		func(i, j int) bool { return result[i].GeoMatchSetId < result[j].GeoMatchSetId },
	)

	return result
}

// --- RegexPatternSet ---

func (b *InMemoryBackend) regexPatternSetARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("regexpatternset/%s", id))
}

// CreateRegexPatternSet creates a new RegexPatternSet.
func (b *InMemoryBackend) CreateRegexPatternSet(name, changeToken string) (*RegexPatternSet, error) {
	b.mu.Lock("CreateRegexPatternSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	rps := &RegexPatternSet{
		RegexPatternSetId:   id,
		Name:                name,
		RegexPatternStrings: []string{},
	}
	b.regexPatternSets.Put(rps)

	return rps, nil
}

// GetRegexPatternSet retrieves a RegexPatternSet by ID.
func (b *InMemoryBackend) GetRegexPatternSet(id string) (*RegexPatternSet, error) {
	b.mu.RLock("GetRegexPatternSet")
	defer b.mu.RUnlock()

	rps, ok := b.regexPatternSets.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return rps, nil
}

// UpdateRegexPatternSet updates a RegexPatternSet's pattern strings.
func (b *InMemoryBackend) UpdateRegexPatternSet(id, changeToken string, updates []RegexPatternSetUpdate) error {
	b.mu.Lock("UpdateRegexPatternSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	rps, ok := b.regexPatternSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	for _, u := range updates {
		patterns, err := applyEntryUpdate(rps.RegexPatternStrings, u.Action, u.RegexPatternString,
			func(a, b string) bool { return a == b })
		if err != nil {
			return err
		}

		rps.RegexPatternStrings = patterns
	}

	return nil
}

// DeleteRegexPatternSet deletes a RegexPatternSet. Real AWS rejects
// deletion while it is still used by a RegexMatchSet
// (WAFReferencedItemException) or is not itself empty
// (WAFNonEmptyEntityException) -- you can't delete a RegexPatternSet if
// it's still used in any RegexMatchSet or if the RegexPatternSet is not
// empty.
func (b *InMemoryBackend) DeleteRegexPatternSet(id, changeToken string) error {
	b.mu.Lock("DeleteRegexPatternSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	rps, ok := b.regexPatternSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	if b.regexPatternSetReferenced(id) {
		return ErrReferencedItem
	}

	if len(rps.RegexPatternStrings) > 0 {
		return ErrNonEmptyEntity
	}

	b.regexPatternSets.Delete(id)
	delete(b.tags, b.regexPatternSetARN(id))

	return nil
}

// ListRegexPatternSets returns summaries of all RegexPatternSets.
func (b *InMemoryBackend) ListRegexPatternSets() []RegexPatternSetSummary {
	b.mu.RLock("ListRegexPatternSets")
	defer b.mu.RUnlock()

	all := b.regexPatternSets.All()
	result := make([]RegexPatternSetSummary, 0, len(all))
	for _, s := range all {
		result = append(result, RegexPatternSetSummary{RegexPatternSetId: s.RegexPatternSetId, Name: s.Name})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].RegexPatternSetId < result[j].RegexPatternSetId
	})

	return result
}

// --- RegexMatchSet ---

func (b *InMemoryBackend) regexMatchSetARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("regexmatchset/%s", id))
}

// CreateRegexMatchSet creates a new RegexMatchSet.
func (b *InMemoryBackend) CreateRegexMatchSet(name, changeToken string) (*RegexMatchSet, error) {
	b.mu.Lock("CreateRegexMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	rms := &RegexMatchSet{
		RegexMatchSetId:  id,
		Name:             name,
		RegexMatchTuples: []RegexMatchTuple{},
	}
	b.regexMatchSets.Put(rms)

	return rms, nil
}

// GetRegexMatchSet retrieves a RegexMatchSet by ID.
func (b *InMemoryBackend) GetRegexMatchSet(id string) (*RegexMatchSet, error) {
	b.mu.RLock("GetRegexMatchSet")
	defer b.mu.RUnlock()

	rms, ok := b.regexMatchSets.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return rms, nil
}

// UpdateRegexMatchSet updates a RegexMatchSet's tuples.
func (b *InMemoryBackend) UpdateRegexMatchSet(id, changeToken string, updates []RegexMatchSetUpdate) error {
	b.mu.Lock("UpdateRegexMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	rms, ok := b.regexMatchSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	for _, u := range updates {
		tuples, err := applyEntryUpdate(rms.RegexMatchTuples, u.Action, u.RegexMatchTuple,
			func(a, b RegexMatchTuple) bool {
				return a.RegexPatternSetId == b.RegexPatternSetId && a.FieldToMatch.Type == b.FieldToMatch.Type
			})
		if err != nil {
			return err
		}

		rms.RegexMatchTuples = tuples
	}

	return nil
}

// DeleteRegexMatchSet deletes a RegexMatchSet. Real AWS rejects deletion
// while it is still used by a Rule (WAFReferencedItemException) or still
// contains any RegexMatchTuples (WAFNonEmptyEntityException).
func (b *InMemoryBackend) DeleteRegexMatchSet(id, changeToken string) error {
	b.mu.Lock("DeleteRegexMatchSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	rms, ok := b.regexMatchSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	if b.matchSetReferenced(id) {
		return ErrReferencedItem
	}

	if len(rms.RegexMatchTuples) > 0 {
		return ErrNonEmptyEntity
	}

	b.regexMatchSets.Delete(id)
	delete(b.tags, b.regexMatchSetARN(id))

	return nil
}

// ListRegexMatchSets returns summaries of all RegexMatchSets.
func (b *InMemoryBackend) ListRegexMatchSets() []RegexMatchSetSummary {
	b.mu.RLock("ListRegexMatchSets")
	defer b.mu.RUnlock()

	all := b.regexMatchSets.All()
	result := make([]RegexMatchSetSummary, 0, len(all))
	for _, s := range all {
		result = append(result, RegexMatchSetSummary{RegexMatchSetId: s.RegexMatchSetId, Name: s.Name})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].RegexMatchSetId < result[j].RegexMatchSetId
	})

	return result
}
