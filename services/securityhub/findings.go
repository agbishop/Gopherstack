package securityhub

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// findingCustomerManagedFields lists ASFF fields that AWS documents as
// "cannot be updated" by BatchImportFindings once a finding already exists --
// Security Hub retains whatever value (or absence) the finding already had,
// ignoring whatever a re-import request supplies for these fields, since
// they're managed by Security Hub customers/automation rules, not finding
// providers.
// https://docs.aws.amazon.com/securityhub/1.0/APIReference/API_BatchImportFindings.html
var findingCustomerManagedFields = []string{ //nolint:gochecknoglobals // read-only lookup data
	"Note", "UserDefinedFields", "VerificationState", "Workflow",
}

// findingHistoryIgnoredFields are ASFF fields GetFindingHistory documents as
// excluded from the change log ("changes made to any fields...except
// top-level timestamp fields, such as the CreatedAt and UpdatedAt fields").
var findingHistoryIgnoredFields = map[string]bool{ //nolint:gochecknoglobals // read-only lookup data
	keyCreatedAt:       true,
	keyUpdatedAt:       true,
	keyFirstObservedAt: true,
	keyLastObservedAt:  true,
}

const (
	findingHistorySourceImport      = "BATCH_IMPORT_FINDINGS"
	findingHistorySourceBatchUpdate = "BATCH_UPDATE_FINDINGS"

	maxFindingHistoryResults = 100

	// comparisonNotEquals and comparisonNotContains name the two
	// StringFilterComparison/MapFilterComparison values that both
	// compareStringFilter/isNegativeStringComparison (this file) and
	// compareMapFilter (findings_v2.go) branch on.
	comparisonNotEquals   = "NOT_EQUALS"
	comparisonNotContains = "NOT_CONTAINS"
)

func findingKey(productArn, id string) string {
	return productArn + "|" + id
}

// validateASFFRequiredFields checks that a finding has the mandatory ASFF fields.
// AWS rejects findings missing any of these fields in BatchImportFindings.
// Returns a non-empty error message if validation fails.
func validateASFFRequiredFields(f map[string]any, productArn, id string) string {
	if productArn == "" {
		return "ProductArn is required"
	}

	if id == "" {
		return "Id is required"
	}

	if v, _ := f[keyAwsAccountID].(string); v == "" {
		return "AwsAccountId is required"
	}

	if v, _ := f["GeneratorId"].(string); v == "" {
		return "GeneratorId is required"
	}

	if v, _ := f["Title"].(string); v == "" {
		return "Title is required"
	}

	if v, _ := f["Description"].(string); v == "" {
		return "Description is required"
	}

	// At least one of CreatedAt/UpdatedAt/FirstObservedAt/LastObservedAt must be present.
	hasTimestamp := false
	for _, k := range []string{keyCreatedAt, keyUpdatedAt, keyFirstObservedAt, keyLastObservedAt} {
		if v, _ := f[k].(string); v != "" {
			hasTimestamp = true

			break
		}
	}

	if !hasTimestamp {
		return "At least one of CreatedAt, UpdatedAt, FirstObservedAt, or LastObservedAt is required"
	}

	// Resources must be a non-empty array.
	resources, _ := f["Resources"].([]any)
	if len(resources) == 0 {
		return "Resources must contain at least one entry"
	}

	return ""
}

func (b *InMemoryBackend) ImportFindings(findings []map[string]any) (int, int, []map[string]any) {
	b.mu.Lock("ImportFindings")
	defer b.mu.Unlock()

	successCount := 0
	failedCount := 0
	var failedFindings []map[string]any

	for _, f := range findings {
		productArn, _ := f[keyProductArn].(string)
		id, _ := f["Id"].(string)

		if msg := validateASFFRequiredFields(f, productArn, id); msg != "" {
			failedCount++
			failedFindings = append(failedFindings, map[string]any{
				"Id":            id,
				keyProductArn:   productArn,
				keyErrorCode:    "InvalidInputException",
				keyErrorMessage: msg,
			})

			continue
		}

		key := findingKey(productArn, id)
		existing, hadExisting := b.findings[key]

		// Copy the finding
		stored := make(map[string]any, len(f))
		maps.Copy(stored, f)

		if hadExisting {
			preserveCustomerManagedFields(stored, existing)
		}

		b.applyAutomationRules(stored)

		b.findings[key] = stored
		successCount++

		if hadExisting {
			changes := diffFindingFields(existing, stored)
			b.recordFindingHistory(productArn, id, false, changes, findingHistorySourceImport)
		} else {
			b.recordFindingHistory(productArn, id, true, nil, findingHistorySourceImport)
		}
	}

	return successCount, failedCount, failedFindings
}

// preserveCustomerManagedFields overwrites stored's customer-managed fields
// (findingCustomerManagedFields) with existing's values, deleting the field
// from stored entirely if existing never had it set. AWS documents that
// BatchImportFindings cannot update these fields once a finding exists --
// see findingCustomerManagedFields for the doc citation.
func preserveCustomerManagedFields(stored, existing map[string]any) {
	for _, field := range findingCustomerManagedFields {
		if v, ok := existing[field]; ok {
			stored[field] = v
		} else {
			delete(stored, field)
		}
	}
}

func (b *InMemoryBackend) GetFindings(
	filters map[string]any,
	sortCriteria []map[string]any,
	nextToken string,
	maxResults int,
) ([]map[string]any, string) {
	b.mu.RLock("GetFindings")
	defer b.mu.RUnlock()

	var results []map[string]any

	for _, f := range b.findings {
		if matchesFindingFilters(f, filters) {
			results = append(results, f)
		}
	}

	sortFindings(results, sortCriteria)

	if maxResults <= 0 || maxResults > 100 {
		maxResults = 100
	}

	start := decodeToken(nextToken)
	if start >= len(results) {
		return []map[string]any{}, ""
	}

	end := start + maxResults
	end = min(end, len(results))

	page := results[start:end]
	nextOut := ""

	if end < len(results) {
		nextOut = encodeToken(end)
	}

	return page, nextOut
}

// sortFindings sorts findings in place per sortCriteria, each element of
// which is the ASFF wire shape {"Field": string, "SortOrder": "asc"|"desc"}
// (types.SortCriterion). Earlier criteria take precedence; later criteria
// break ties, matching AWS's documented multi-field sort semantics.
//
// findings is always freshly collected from a `map[string]map[string]any`
// (b.findings), so it arrives in no defined order at all -- not any
// meaningful "prior order" -- and GetFindings/GetFindingsV2 paginate this
// same slice across separate calls. Without a final deterministic tiebreak
// (also the entire ordering when sortCriteria is empty, the common case),
// two calls serving consecutive pages can independently land tied/unordered
// findings on either side of the boundary and drop or duplicate results.
func sortFindings(findings []map[string]any, sortCriteria []map[string]any) {
	type criterion struct {
		field string
		desc  bool
	}

	criteria := make([]criterion, 0, len(sortCriteria))

	for _, sc := range sortCriteria {
		field, _ := sc["Field"].(string)
		if field == "" {
			continue
		}

		order, _ := sc[keySortOrder].(string)
		criteria = append(criteria, criterion{field: field, desc: order == "desc"})
	}

	sort.SliceStable(findings, func(i, j int) bool {
		for _, c := range criteria {
			vi := findingSortValue(findings[i], c.field)
			vj := findingSortValue(findings[j], c.field)

			if vi == vj {
				continue
			}

			if c.desc {
				return vi > vj
			}

			return vi < vj
		}

		return findingSortValue(findings[i], keyProductArn)+"|"+findingSortValue(findings[i], "Id") <
			findingSortValue(findings[j], keyProductArn)+"|"+findingSortValue(findings[j], "Id")
	})
}

// findingSortValue returns a comparable string representation of finding's
// top-level field, used to order findings for a single sort criterion.
// Fields absent on the finding sort as "" (first in ascending order).
func findingSortValue(f map[string]any, field string) string {
	v, ok := f[field]
	if !ok {
		return ""
	}

	if s, isStr := v.(string); isStr {
		return s
	}

	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}

	return string(b)
}

// matchesFindingFilters is a simplified filter check.
// AWS SecurityHub finding filters are complex; we support a basic subset.
func matchesFindingFilters(finding, filters map[string]any) bool {
	if len(filters) == 0 {
		return true
	}

	fID, _ := finding["Id"].(string)
	if !matchesStringFilter(fID, filters["Id"]) {
		return false
	}

	fArn, _ := finding[keyProductArn].(string)
	if !matchesStringFilter(fArn, filters[keyProductArn]) {
		return false
	}

	// Additional string field filters. "Type" here is the AwsSecurityFindingFilters
	// filter parameter name, a distinct concept from the FindingHistoryUpdateSource
	// "Type" wire field in recordFindingHistory (see its nolint comment).
	for _, fieldKey := range []string{
		keyAwsAccountID, "GeneratorId", keyTitle, keyDescription,
		"RecordState", "Type", "ResourceType", "ResourceId",
	} {
		fVal, _ := finding[fieldKey].(string)
		if !matchesStringFilter(fVal, filters[fieldKey]) {
			return false
		}
	}

	// SeverityLabel/WorkflowStatus/ComplianceStatus are filter names for values
	// nested under the finding's Severity.Label/Workflow.Status/Compliance.Status
	// objects (securityhub@v1.75.4 types/types.go AwsSecurityFinding), not flat
	// top-level finding fields.
	for _, fieldKey := range findingNestedFilterFields {
		if !matchesStringFilter(findingFieldString(finding, fieldKey), filters[fieldKey]) {
			return false
		}
	}

	return true
}

// findingNestedFilterFields are the finding filter/group-by field names that
// findingFieldString resolves from a nested location rather than a flat key.
var findingNestedFilterFields = []string{ //nolint:gochecknoglobals // read-only lookup data
	keyFilterSeverityLabel, keyFilterWorkflowStatus, keyFilterComplianceStatus,
}

// findingFieldString resolves an ASFF finding-filter/group-by field name to
// its string value on finding. SeverityLabel/WorkflowStatus/ComplianceStatus
// name values nested under the finding's Severity.Label/Workflow.Status/
// Compliance.Status objects (securityhub@v1.75.4 types/types.go
// AwsSecurityFinding) -- BatchImportFindings/BatchUpdateFindings never write
// a flat key by those names, so a literal finding[key] lookup always saw ""
// and could never match a real filter or group-by value. Every other key is
// a real flat top-level ASFF field, looked up directly.
func findingFieldString(finding map[string]any, key string) string {
	switch key {
	case keyFilterSeverityLabel:
		return nestedFindingString(finding, "Severity", "Label")
	case keyFilterWorkflowStatus:
		return nestedFindingString(finding, "Workflow", "Status")
	case keyFilterComplianceStatus:
		return nestedFindingString(finding, "Compliance", "Status")
	default:
		s, _ := finding[key].(string)

		return s
	}
}

// nestedFindingString reads finding[outer][inner] as a string, returning ""
// if outer is absent or not an object, or inner is absent or not a string.
func nestedFindingString(finding map[string]any, outer, inner string) string {
	obj, ok := finding[outer].(map[string]any)
	if !ok {
		return ""
	}

	s, _ := obj[inner].(string)

	return s
}

// compareStringFilter evaluates one SecurityHub StringFilter comparison,
// reporting whether fieldVal matches val under comp. Unrecognized/empty comp
// defaults to EQUALS, matching the real API.
func compareStringFilter(comp, fieldVal, val string) bool {
	switch comp {
	case comparisonNotEquals:
		return fieldVal != val
	case "PREFIX":
		return strings.HasPrefix(fieldVal, val)
	case "PREFIX_NOT_EQUALS":
		return !strings.HasPrefix(fieldVal, val)
	case "CONTAINS":
		return strings.Contains(fieldVal, val)
	case comparisonNotContains:
		return !strings.Contains(fieldVal, val)
	case "CONTAINS_WORD":
		return matchesWholeWord(fieldVal, val)
	default: // EQUALS
		return fieldVal == val
	}
}

// isNegativeStringComparison reports whether comp is one of StringFilter's
// documented "exclude" comparisons (NOT_CONTAINS/NOT_EQUALS/
// PREFIX_NOT_EQUALS), as opposed to an "include" comparison
// (CONTAINS/EQUALS/PREFIX/CONTAINS_WORD, and the empty/unrecognized default
// which compareStringFilter treats as EQUALS).
func isNegativeStringComparison(comp string) bool {
	switch comp {
	case comparisonNotContains, comparisonNotEquals, "PREFIX_NOT_EQUALS":
		return true
	default:
		return false
	}
}

// matchesStringFilter checks a single string field value against a SecurityHub
// filter value: a []StringFilter-shaped list of {Value, Comparison} entries.
// Per StringFilter's documented same-field combination rule
// (securityhub@v1.75.4 types.StringFilter): CONTAINS/EQUALS/PREFIX entries
// are joined by OR ("a finding matches if it matches any one of those
// filters"); NOT_CONTAINS/NOT_EQUALS/PREFIX_NOT_EQUALS entries are joined by
// AND; the two groups then combine by AND (Security Hub "first processes the
// PREFIX filters, and then the NOT_EQUALS ... filters").
func matchesStringFilter(fieldVal string, filterVal any) bool {
	items, ok := filterVal.([]any)
	if !ok {
		return true
	}

	var hasPositive, positiveMatched bool

	for _, item := range items {
		m, isMap := item.(map[string]any)
		if !isMap {
			continue
		}

		val, _ := m["Value"].(string)
		comp, _ := m["Comparison"].(string)

		if isNegativeStringComparison(comp) {
			if !compareStringFilter(comp, fieldVal, val) {
				return false
			}

			continue
		}

		hasPositive = true
		if compareStringFilter(comp, fieldVal, val) {
			positiveMatched = true
		}
	}

	return !hasPositive || positiveMatched
}

func (b *InMemoryBackend) UpdateFindings(filters map[string]any, note map[string]any, recordState string) error {
	b.mu.Lock("UpdateFindings")
	defer b.mu.Unlock()

	if !b.hubEnabled {
		return ErrHubNotEnabled
	}

	for key, f := range b.findings {
		if !matchesFindingFilters(f, filters) {
			continue
		}

		before := make(map[string]any, len(f))
		maps.Copy(before, f)

		if note != nil {
			f["Note"] = note
		}

		if recordState != "" {
			f["RecordState"] = recordState
		}

		b.findings[key] = f

		productArn, _ := f[keyProductArn].(string)
		id, _ := f["Id"].(string)
		b.recordFindingHistory(productArn, id, false, diffFindingFields(before, f), findingHistorySourceBatchUpdate)
	}

	return nil
}

func (b *InMemoryBackend) BatchUpdateFindings(
	findingIdentifiers []map[string]any,
	updates map[string]any,
) ([]map[string]any, []map[string]any) {
	b.mu.Lock("BatchUpdateFindings")
	defer b.mu.Unlock()

	var processedFindings []map[string]any
	var unprocessedFindings []map[string]any

	for _, ident := range findingIdentifiers {
		productArn, _ := ident[keyProductArn].(string)
		id, _ := ident["Id"].(string)
		key := findingKey(productArn, id)

		f, exists := b.findings[key]
		if !exists {
			unprocessedFindings = append(unprocessedFindings, map[string]any{
				keyFindingIdentifier: ident,
				keyErrorCode:         errCodeInvalidInput,
				keyErrorMessage:      msgFindingNotFound,
			})

			continue
		}

		before := make(map[string]any, len(f))
		maps.Copy(before, f)

		// Apply updates
		maps.Copy(f, updates)

		b.findings[key] = f
		processedFindings = append(processedFindings, ident)

		b.recordFindingHistory(productArn, id, false, diffFindingFields(before, f), findingHistorySourceBatchUpdate)
	}

	if processedFindings == nil {
		processedFindings = []map[string]any{}
	}

	if unprocessedFindings == nil {
		unprocessedFindings = []map[string]any{}
	}

	return processedFindings, unprocessedFindings
}

// diffFindingFields compares old and new top-level ASFF field maps and
// returns one FindingHistoryUpdate-shaped entry ({"UpdatedField",
// "OldValue", "NewValue"}) per changed field, excluding
// findingHistoryIgnoredFields. A field added or removed entirely still
// produces an entry with only NewValue or only OldValue set, matching how
// FindingHistoryUpdate omits absent values on the wire.
func diffFindingFields(oldF, newF map[string]any) []map[string]any {
	seen := make(map[string]bool, len(oldF)+len(newF))

	var updates []map[string]any

	record := func(field string, oldV, newV any) {
		if findingHistoryIgnoredFields[field] || seen[field] {
			return
		}

		seen[field] = true

		oldRepr := historyValueString(oldV)
		newRepr := historyValueString(newV)

		if oldRepr == nil && newRepr == nil {
			return
		}

		if oldRepr != nil && newRepr != nil && *oldRepr == *newRepr {
			return
		}

		entry := map[string]any{"UpdatedField": field}
		if oldRepr != nil {
			entry["OldValue"] = *oldRepr
		}

		if newRepr != nil {
			entry["NewValue"] = *newRepr
		}

		updates = append(updates, entry)
	}

	for field, newV := range newF {
		record(field, oldF[field], newV)
	}

	for field, oldV := range oldF {
		if _, ok := newF[field]; !ok {
			record(field, oldV, nil)
		}
	}

	return updates
}

// historyValueString renders v as the string form GetFindingHistory reports
// for a changed ASFF field's Old/NewValue. nil renders as nil (field absent).
func historyValueString(v any) *string {
	if v == nil {
		return nil
	}

	if s, ok := v.(string); ok {
		return &s
	}

	b, err := json.Marshal(v)
	if err != nil {
		s := fmt.Sprintf("%v", v)

		return &s
	}

	s := string(b)

	return &s
}

// recordFindingHistory appends one FindingHistoryRecord-shaped entry to the
// finding's change log. The caller must already hold b.mu for writing. A
// no-op for a change event with nothing to report (not a creation and no
// field actually changed), since AWS's GetFindingHistory only records real
// change events.
func (b *InMemoryBackend) recordFindingHistory(
	productArn, id string,
	findingCreated bool,
	updates []map[string]any,
	sourceType string,
) {
	if !findingCreated && len(updates) == 0 {
		return
	}

	if updates == nil {
		updates = []map[string]any{}
	}

	rec := map[string]any{
		keyFindingIdentifier: map[string]any{"Id": id, keyProductArn: productArn},
		"FindingCreated":     findingCreated,
		"UpdateTime":         time.Now().UTC().Format(time.RFC3339),
		"UpdateSource": map[string]any{
			"Type":     sourceType,
			"Identity": arn.Build("iam", b.region, b.accountID, "root"),
		},
		"Updates": updates,
	}

	key := findingKey(productArn, id)
	b.findingHistory[key] = append(b.findingHistory[key], rec)
}

func (b *InMemoryBackend) GetFindingHistory(
	ident map[string]any,
	startTime, endTime string,
	nextToken string,
	maxResults int,
) ([]map[string]any, string) {
	b.mu.RLock("GetFindingHistory")
	defer b.mu.RUnlock()

	productArn, _ := ident[keyProductArn].(string)
	id, _ := ident["Id"].(string)
	key := findingKey(productArn, id)

	start, hasStart := parseHistoryTime(startTime)
	end, hasEnd := parseHistoryTime(endTime)

	var filtered []map[string]any

	for _, rec := range b.findingHistory[key] {
		ts, _ := rec["UpdateTime"].(string)

		t, err := time.Parse(time.RFC3339, ts)
		if err == nil {
			if hasStart && t.Before(start) {
				continue
			}

			if hasEnd && t.After(end) {
				continue
			}
		}

		filtered = append(filtered, rec)
	}

	return paginateSlice(filtered, nextToken, maxResults, maxFindingHistoryResults)
}

// parseHistoryTime parses an ISO-8601 GetFindingHistory StartTime/EndTime
// bound. An empty or malformed value reports hasTime=false so the caller
// treats the bound as unset (matching AWS's documented "if you provide
// neither StartTime nor EndTime" behavior).
func parseHistoryTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}

	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}

	return t, true
}

func (b *InMemoryBackend) GetFindingStatisticsV2(groupByFields []string) []map[string]any {
	b.mu.RLock("GetFindingStatisticsV2")
	defer b.mu.RUnlock()

	items := make([]map[string]any, 0, len(b.findings))
	for _, f := range b.findings {
		items = append(items, flattenFindingGroupByFields(f))
	}

	return groupByResults(items, groupByFields, ocsfStringFieldMap)
}

// flattenFindingGroupByFields returns a shallow copy of finding with
// SeverityLabel/WorkflowStatus/ComplianceStatus synthesized as flat keys
// from their real nested locations (see findingFieldString), so
// groupByResults's ocsfStringFieldMap-driven lookup -- which expects a flat
// key -- sees the real value instead of always "".
func flattenFindingGroupByFields(finding map[string]any) map[string]any {
	flat := make(map[string]any, len(finding)+len(findingNestedFilterFields))
	maps.Copy(flat, finding)

	for _, fieldKey := range findingNestedFilterFields {
		flat[fieldKey] = findingFieldString(finding, fieldKey)
	}

	return flat
}

// SeverityTrendsCount bucket names GetFindingsTrendsV2 requires
// (securityhub@v1.75.4 types/types.go:19025-19058).
const (
	trendBucketCritical      = "Critical"
	trendBucketFatal         = "Fatal"
	trendBucketHigh          = "High"
	trendBucketInformational = "Informational"
	trendBucketLow           = "Low"
	trendBucketMedium        = "Medium"
	trendBucketOther         = "Other"
	trendBucketUnknown       = "Unknown"

	severityLabelHigh   = "HIGH"
	severityLabelMedium = "MEDIUM"
)

// severityTrendsBucket maps an ASFF SeverityLabel (types.SeverityLabel:
// INFORMATIONAL/LOW/MEDIUM/HIGH/CRITICAL -- securityhub@v1.75.4
// types/enums.go:1797-1806, the only severity vocabulary this backend's
// findings carry, per findings_v2.go's ocsfStringFieldMap comment) onto one
// of the eight SeverityTrendsCount bucket names above. ASFF has no FATAL
// severity, so that bucket is always zero: there's no backing state to
// derive it from, left inert rather than fabricated.
func severityTrendsBucket(label string) string {
	switch strings.ToUpper(label) {
	case "CRITICAL":
		return trendBucketCritical
	case severityLabelHigh:
		return trendBucketHigh
	case severityLabelMedium:
		return trendBucketMedium
	case "LOW":
		return trendBucketLow
	case "INFORMATIONAL":
		return trendBucketInformational
	case "":
		return trendBucketUnknown
	default:
		return trendBucketOther
	}
}

// GetFindingsTrendsV2 returns a single TrendsMetricsResult data point
// (Timestamp + TrendsValues.SeverityTrends -- securityhub@v1.75.4
// types/types.go:19869-19896) aggregating every stored finding's severity.
// The real GetFindingsTrendsV2Input has no GroupByAttribute member at all
// (api_op_GetFindingsTrendsV2.go:22-46); this backend has no time-bucketed
// analytics engine, so unlike the real per-Granularity series this always
// returns one point for the whole store, timestamped at endTime.
func (b *InMemoryBackend) GetFindingsTrendsV2(startTime, endTime string) []map[string]any {
	b.mu.RLock("GetFindingsTrendsV2")
	defer b.mu.RUnlock()

	counts := map[string]int64{
		trendBucketCritical: 0, trendBucketFatal: 0, trendBucketHigh: 0, trendBucketInformational: 0,
		trendBucketLow: 0, trendBucketMedium: 0, trendBucketOther: 0, trendBucketUnknown: 0,
	}

	for _, finding := range b.findings {
		counts[severityTrendsBucket(findingFieldString(finding, keyFilterSeverityLabel))]++
	}

	ts := endTime
	if ts == "" {
		ts = startTime
	}

	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}

	return []map[string]any{
		{
			"Timestamp": ts,
			"TrendsValues": map[string]any{
				"SeverityTrends": map[string]any{
					trendBucketCritical:      counts[trendBucketCritical],
					trendBucketFatal:         counts[trendBucketFatal],
					trendBucketHigh:          counts[trendBucketHigh],
					trendBucketInformational: counts[trendBucketInformational],
					trendBucketLow:           counts[trendBucketLow],
					trendBucketMedium:        counts[trendBucketMedium],
					trendBucketOther:         counts[trendBucketOther],
					trendBucketUnknown:       counts[trendBucketUnknown],
				},
			},
		},
	}
}
