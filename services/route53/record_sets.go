package route53

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	recordTypeA     = "A"
	recordTypeAAAA  = "AAAA"
	recordTypeCNAME = "CNAME"
	recordTypeMX    = "MX"
	recordTypeTXT   = "TXT"
	recordTypeNS    = "NS"
	recordTypeSOA   = "SOA"
	recordTypePTR   = "PTR"
	recordTypeSRV   = "SRV"
	recordTypeCAA   = "CAA"
	recordTypeDS    = "DS"
	recordTypeHTTPS = "HTTPS"
	recordTypeSVCB  = "SVCB"
	recordTypeSSHFP = "SSHFP"
	recordTypeTLSA  = "TLSA"
	recordTypeNAPTR = "NAPTR"
	recordTypeSPF   = "SPF"
)

// validRecordTypes is the set of record types that AWS Route 53 accepts for
// user-managed ResourceRecordSets (DNSKEY/NSEC are DNSSEC-internal only).
//
//nolint:gochecknoglobals // package-level table initialized once at startup
var validRecordTypes = map[string]bool{
	recordTypeA: true, recordTypeAAAA: true, recordTypeCNAME: true,
	recordTypeMX: true, recordTypeTXT: true, recordTypeNS: true,
	recordTypeSOA: true, recordTypePTR: true, recordTypeSRV: true,
	recordTypeCAA: true, recordTypeDS: true, recordTypeHTTPS: true,
	recordTypeSVCB: true, recordTypeSSHFP: true, recordTypeTLSA: true,
	recordTypeNAPTR: true, recordTypeSPF: true,
}

const (
	maxChangesPerBatch = 1000
	maxTTL             = 2147483647
)

// record value format validators (minimal per-type).
var (
	reMX  = regexp.MustCompile(`^\d+ \S+$`)
	reSRV = regexp.MustCompile(`^\d+ \d+ \d+ \S+$`)
	reCAA = regexp.MustCompile(`^\d+ \S+ "`)
	// reHostname matches a DNS domain name (used for CNAME/NS/PTR targets).
	// Underscores are permitted because service-discovery names (e.g. _sip._tcp)
	// are common and accepted by Route 53. A trailing dot is optional.
	reHostname = regexp.MustCompile(
		`^(\*\.)?([A-Za-z0-9_]([A-Za-z0-9_-]{0,61}[A-Za-z0-9_])?\.)+[A-Za-z][A-Za-z0-9-]{0,61}[A-Za-z0-9]\.?$`,
	)
	// reDS matches "<KeyTag> <Algorithm> <DigestType> <Digest(hex)>".
	reDS = regexp.MustCompile(`^\d+ \d+ \d+ [0-9A-Fa-f]+$`)
	// reNAPTR matches "<order> <pref> \"flags\" \"service\" \"regexp\" <replacement>".
	reNAPTR = regexp.MustCompile(`^\d+ \d+ "[^"]*" "[^"]*" "[^"]*" \S+$`)
)

// recordSetKey builds the map key for a resource record set.
// When SetIdentifier is non-empty it is included so routing-policy records
// with the same name/type can coexist.
func recordSetKey(name, rrType, setIdentifier string) string {
	base := strings.ToLower(strings.TrimSuffix(name, ".")) + "|" + strings.ToUpper(rrType)
	if setIdentifier != "" {
		return base + "|" + setIdentifier
	}

	return base
}

// maxCharacterStringLen is the DNS limit for a single quoted character-string
// (RFC 1035): each string inside a TXT/SPF value may hold at most 255 octets.
const maxCharacterStringLen = 255

// isValidIPv4 reports whether value is a dotted-quad IPv4 address.
func isValidIPv4(value string) bool {
	return net.ParseIP(value) != nil && !strings.Contains(value, ":")
}

// isValidIPv6 reports whether value is an IPv6 address.
func isValidIPv6(value string) bool {
	return net.ParseIP(value) != nil && strings.Contains(value, ":")
}

// recordValueValidators maps a record type to a predicate that accepts a valid
// value. Types absent from the map (SOA/HTTPS/SVCB/SSHFP/TLSA) accept any value.
//
//nolint:gochecknoglobals // static dispatch table initialized once at startup
var recordValueValidators = map[string]func(string) bool{
	recordTypeA:     isValidIPv4,
	recordTypeAAAA:  isValidIPv6,
	recordTypeMX:    reMX.MatchString,
	recordTypeSRV:   reSRV.MatchString,
	recordTypeCAA:   reCAA.MatchString,
	recordTypeTXT:   isValidCharacterStrings,
	recordTypeSPF:   isValidCharacterStrings,
	recordTypeCNAME: reHostname.MatchString,
	recordTypeNS:    reHostname.MatchString,
	recordTypePTR:   reHostname.MatchString,
	recordTypeDS:    reDS.MatchString,
	recordTypeNAPTR: reNAPTR.MatchString,
}

// recordValueErrors maps a record type to the sentinel returned when its value
// fails validation.
//
//nolint:gochecknoglobals // static dispatch table initialized once at startup
var recordValueErrors = map[string]error{
	recordTypeA:     ErrInvalidARecord,
	recordTypeAAAA:  ErrInvalidAAAARecord,
	recordTypeMX:    ErrInvalidMXRecord,
	recordTypeSRV:   ErrInvalidSRVRecord,
	recordTypeCAA:   ErrInvalidCAARecord,
	recordTypeTXT:   ErrInvalidTXTRecord,
	recordTypeSPF:   ErrInvalidSPFRecord,
	recordTypeCNAME: ErrInvalidCNAMERecord,
	recordTypeNS:    ErrInvalidNSRecord,
	recordTypePTR:   ErrInvalidPTRRecord,
	recordTypeDS:    ErrInvalidDSRecord,
	recordTypeNAPTR: ErrInvalidNAPTRRecord,
}

// validateRecordValue checks per-type value format constraints.
func validateRecordValue(rrType, value string) error {
	validate, ok := recordValueValidators[rrType]
	if !ok || validate(value) {
		return nil
	}

	return fmt.Errorf("invalid %s record value %q: %w", rrType, value, recordValueErrors[rrType])
}

// isValidCharacterStrings validates a TXT/SPF value. Route 53 requires the value
// to be one or more double-quoted character-strings (e.g. `"v=spf1 ~all"` or
// `"chunk-a" "chunk-b"`), each at most 255 octets. Embedded quotes must be
// backslash-escaped.
func isValidCharacterStrings(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 2 || trimmed[0] != '"' || trimmed[len(trimmed)-1] != '"' {
		return false
	}

	inString := false
	escaped := false
	segLen := 0

	for i := range len(trimmed) {
		ch := trimmed[i]
		if escaped {
			escaped, segLen = false, segLen+1

			continue
		}

		switch ch {
		case '\\':
			escaped = true
		case '"':
			if inString && segLen > maxCharacterStringLen {
				return false
			}
			inString, segLen = !inString, 0
		case ' ':
			if inString {
				segLen++
			}
		default:
			if !inString {
				return false
			}
			segLen++
		}
	}

	return !inString && !escaped
}

// countRoutingPolicies returns how many of the mutually exclusive
// routing-policy fields are set on rrs.
func countRoutingPolicies(rrs ResourceRecordSet) int {
	count := 0
	// Weight is a pointer: nil means omitted (no weighted routing), non-nil means
	// the caller explicitly set a weight (including Weight=0, which stops traffic).
	if rrs.Weight != nil {
		count++
	}

	if rrs.Region != "" {
		count++
	}

	if rrs.GeoLocation != nil {
		count++
	}

	if rrs.Failover != "" {
		count++
	}

	if rrs.MultiValueAnswer {
		count++
	}

	if rrs.GeoProximityLocation != nil {
		count++
	}

	if rrs.CidrRoutingConfig != nil {
		count++
	}

	return count
}

// validateRoutingPolicyCardinality enforces AWS's "at most one routing
// policy per ResourceRecordSet" rule and the SetIdentifier pairing it implies.
func validateRoutingPolicyCardinality(policyCount int, setIdentifier string) error {
	if policyCount > 1 {
		return fmt.Errorf(
			"%w: only one routing policy may be set per ResourceRecordSet",
			ErrInvalidInput,
		)
	}

	if policyCount == 1 && setIdentifier == "" {
		return fmt.Errorf(
			"%w: SetIdentifier is required when a routing policy is specified",
			ErrInvalidInput,
		)
	}

	if policyCount == 0 && setIdentifier != "" {
		return fmt.Errorf(
			"%w: SetIdentifier is not allowed without a routing policy",
			ErrInvalidInput,
		)
	}

	return nil
}

// validateRoutingPolicyFields validates the per-field constraints of the
// routing-policy fields that carry a value beyond simple presence
// (Failover's enum, Weight's range).
func validateRoutingPolicyFields(rrs ResourceRecordSet) error {
	if rrs.Failover != "" && rrs.Failover != FailoverPrimary && rrs.Failover != FailoverSecondary {
		return fmt.Errorf("%w: Failover must be PRIMARY or SECONDARY", ErrInvalidInput)
	}

	if rrs.Weight != nil && (*rrs.Weight < 0 || *rrs.Weight > 255) {
		return fmt.Errorf("%w: Weight must be in range [0, 255]", ErrInvalidInput)
	}

	return nil
}

// validateRoutingPolicy enforces AWS mutual-exclusion rules for routing policies.
func validateRoutingPolicy(rrs ResourceRecordSet) error {
	if err := validateRoutingPolicyCardinality(countRoutingPolicies(rrs), rrs.SetIdentifier); err != nil {
		return err
	}

	if err := validateRoutingPolicyFields(rrs); err != nil {
		return err
	}

	return validateGeoProximityLocation(rrs.GeoProximityLocation)
}

// geoProximityLocationFieldCount counts how many of the three mutually exclusive
// routing fields are set on a GeoProximityLocation.
func geoProximityLocationFieldCount(gpl *GeoProximityLocation) int {
	count := 0
	if gpl.AWSRegion != "" {
		count++
	}
	if gpl.Coordinates != nil {
		count++
	}
	if gpl.LocalZoneGroup != "" {
		count++
	}

	return count
}

// validateGeoProximityCoordinates validates the lat/lon values inside a Coordinates block.
func validateGeoProximityCoordinates(coords *GeoProximityCoordinates) error {
	const (
		latMin = -90.0
		latMax = 90.0
		lonMin = -180.0
		lonMax = 180.0
	)

	lat, err := strconv.ParseFloat(coords.Latitude, 64)
	if err != nil || lat < latMin || lat > latMax {
		return fmt.Errorf(
			"%w: GeoProximityLocation Coordinates Latitude must be a number in [%g, %g]",
			ErrInvalidInput, latMin, latMax,
		)
	}

	lon, err := strconv.ParseFloat(coords.Longitude, 64)
	if err != nil || lon < lonMin || lon > lonMax {
		return fmt.Errorf(
			"%w: GeoProximityLocation Coordinates Longitude must be a number in [%g, %g]",
			ErrInvalidInput, lonMin, lonMax,
		)
	}

	return nil
}

// validateGeoProximityLocation enforces AWS constraints on GeoProximityLocation routing config.
func validateGeoProximityLocation(gpl *GeoProximityLocation) error {
	if gpl == nil {
		return nil
	}

	count := geoProximityLocationFieldCount(gpl)

	if count == 0 {
		return fmt.Errorf(
			"%w: GeoProximityLocation requires one of AWSRegion, Coordinates, or LocalZoneGroup",
			ErrInvalidInput,
		)
	}

	if count > 1 {
		return fmt.Errorf(
			"%w: GeoProximityLocation must specify exactly one of AWSRegion, Coordinates, or LocalZoneGroup",
			ErrInvalidInput,
		)
	}

	const (
		biasMin = -99
		biasMax = 99
	)

	if gpl.Bias < biasMin || gpl.Bias > biasMax {
		return fmt.Errorf(
			"%w: GeoProximityLocation Bias must be in range [%d, %d]",
			ErrInvalidInput, biasMin, biasMax,
		)
	}

	if gpl.Coordinates != nil {
		return validateGeoProximityCoordinates(gpl.Coordinates)
	}

	return nil
}

// validateChangeType checks the record type is one AWS Route 53 accepts.
func validateChangeType(rrs ResourceRecordSet) error {
	if !validRecordTypes[rrs.Type] {
		return fmt.Errorf("%w: unsupported record type %q", ErrInvalidAction, rrs.Type)
	}

	return nil
}

// validateChangeTTL enforces AWS's TTL rules: required and range-checked for
// non-alias records; alias records must omit TTL entirely.
func validateChangeTTL(rrs ResourceRecordSet) error {
	if rrs.AliasTarget != nil {
		return nil
	}

	if rrs.TTL <= 0 {
		return fmt.Errorf("%w: TTL must be > 0 for non-alias records", ErrInvalidAction)
	}

	if rrs.TTL > maxTTL {
		return fmt.Errorf("%w: TTL exceeds maximum value %d", ErrInvalidAction, maxTTL)
	}

	return nil
}

// validateChangeCNAME enforces AWS's CNAME placement rules: not permitted at
// the zone apex, and not permitted to coexist with any other record type at
// the same name.
func validateChangeCNAME(zd *zoneData, rrs ResourceRecordSet) error {
	if rrs.Type != recordTypeCNAME {
		return nil
	}

	if rrs.Name == zd.zone.Name {
		return fmt.Errorf(
			"%w: CNAME record not permitted at zone apex %s",
			ErrInvalidAction,
			zd.zone.Name,
		)
	}

	key := recordSetKey(rrs.Name, rrs.Type, "")
	rrsName := strings.ToLower(strings.TrimSuffix(rrs.Name, "."))

	for existKey, existRRS := range zd.records {
		if existKey == key {
			continue
		}

		existName := strings.ToLower(strings.TrimSuffix(existRRS.Name, "."))
		if existName == rrsName && existRRS.Type != recordTypeCNAME {
			return fmt.Errorf(
				"%w: CNAME cannot coexist with other types at name %s",
				ErrInvalidAction, rrs.Name,
			)
		}
	}

	return nil
}

// validateChangeRecordValues validates every resource-record value against
// its type's AWS wire-format rules.
func validateChangeRecordValues(rrs ResourceRecordSet) error {
	for _, rr := range rrs.Records {
		if err := validateRecordValue(rrs.Type, rr.Value); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidAction, err.Error())
		}
	}

	return nil
}

// validateChangeActionState enforces the action-specific state checks: a
// DELETE must target an existing record set whose values exactly match, and
// a CREATE must not collide with an existing record set. UPSERT has no state
// precondition (create-or-replace unconditionally), matching AWS.
func validateChangeActionState(zd *zoneData, ch Change) error {
	rrs := ch.ResourceRecordSet
	key := recordSetKey(rrs.Name, rrs.Type, rrs.SetIdentifier)

	switch ch.Action {
	case ChangeActionDelete:
		existing, exists := zd.records[key]
		if !exists {
			return fmt.Errorf(
				"%w: record set %s %s not found for DELETE",
				ErrInvalidAction,
				rrs.Name,
				rrs.Type,
			)
		}

		// AWS requires a DELETE to specify values that exactly match the existing
		// record set (TTL and all resource record values, or the AliasTarget).
		// If they do not match, Route 53 returns InvalidChangeBatch rather than
		// silently deleting the record.
		return deleteValuesMatch(existing, &rrs)
	case ChangeActionCreate:
		if _, exists := zd.records[key]; exists {
			return fmt.Errorf(
				"%w: record set %s %s already exists",
				ErrInvalidAction,
				rrs.Name,
				rrs.Type,
			)
		}
	case ChangeActionUpsert:
		// No state precondition: AWS's UPSERT creates or replaces the record
		// set unconditionally, regardless of whether it already exists.
	}

	return nil
}

// validateChange validates a single change against current zone state.
func validateChange(zd *zoneData, ch Change) error {
	rrs := ch.ResourceRecordSet

	if err := validateChangeType(rrs); err != nil {
		return err
	}

	if err := validateChangeTTL(rrs); err != nil {
		return err
	}

	if err := validateChangeCNAME(zd, rrs); err != nil {
		return err
	}

	if err := validateChangeRecordValues(rrs); err != nil {
		return err
	}

	if err := validateRoutingPolicy(rrs); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidAction, err.Error())
	}

	return validateChangeActionState(zd, ch)
}

// deleteValuesMatch enforces AWS's DELETE exact-match rule. When deleting a
// resource record set you must supply the same TTL and the same set of resource
// record values (or the same AliasTarget) that the record currently holds. If
// the supplied change omits values/TTL entirely (a bare name+type delete) AWS
// still accepts it, so we only enforce a match when the caller actually provided
// values to compare against.
func deleteValuesMatch(existing, want *ResourceRecordSet) error {
	// Alias vs non-alias mismatch is always an error when an AliasTarget is given.
	if want.AliasTarget != nil || existing.AliasTarget != nil {
		if !aliasTargetsEqual(existing.AliasTarget, want.AliasTarget) {
			return deleteMismatchErr(want)
		}

		return nil
	}

	// Bare delete: no values and no TTL supplied — accept (matches AWS, which
	// keys the delete on name+type+SetIdentifier in that case).
	if len(want.Records) == 0 && want.TTL == 0 {
		return nil
	}

	if want.TTL != 0 && want.TTL != existing.TTL {
		return deleteMismatchErr(want)
	}

	if len(want.Records) > 0 && !sameValueSet(recordValues(existing), recordValues(want)) {
		return deleteMismatchErr(want)
	}

	return nil
}

// aliasTargetsEqual reports whether two AliasTargets are equivalent for the
// purpose of DELETE matching (DNS name compared case-insensitively, ignoring a
// trailing dot, alongside hosted-zone ID and EvaluateTargetHealth).
func aliasTargetsEqual(a, b *AliasTarget) bool {
	if a == nil || b == nil {
		return a == b
	}

	aName := strings.ToLower(strings.TrimSuffix(a.DNSName, "."))
	bName := strings.ToLower(strings.TrimSuffix(b.DNSName, "."))

	return aName == bName &&
		a.HostedZoneID == b.HostedZoneID &&
		a.EvaluateTargetHealth == b.EvaluateTargetHealth
}

// sameValueSet reports whether two value slices contain the same multiset of
// values, irrespective of order (Route 53 treats resource record values as an
// unordered set).
func sameValueSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[v]++
	}

	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}

	return true
}

// deleteMismatchErr builds the AWS-style InvalidChangeBatch error returned when
// a DELETE does not match the current values of the record set.
func deleteMismatchErr(rrs *ResourceRecordSet) error {
	return fmt.Errorf(
		"%w: Tried to delete resource record set [name='%s', type='%s'] "+
			"but the values provided do not match the current values",
		ErrInvalidAction,
		rrs.Name,
		rrs.Type,
	)
}

// collectDNSRegisterOp returns a dnsOp (non-nil) if rrs requires DNS registration,
// or nil when no DNS side-effect is needed.
func collectDNSRegisterOp(rrs ResourceRecordSet) *dnsOp {
	switch rrs.Type {
	case recordTypeA, recordTypeAAAA:
		vals := make([]string, 0, len(rrs.Records))
		for _, r := range rrs.Records {
			vals = append(vals, r.Value)
		}
		if rrs.AliasTarget != nil {
			vals = append(vals, strings.TrimSuffix(rrs.AliasTarget.DNSName, "."))
		}
		if len(vals) > 0 {
			return &dnsOp{name: rrs.Name, recordType: rrs.Type, values: vals}
		}
	case recordTypeCNAME:
		vals := make([]string, 0, len(rrs.Records))
		for _, r := range rrs.Records {
			vals = append(vals, r.Value)
		}
		if rrs.AliasTarget != nil {
			vals = append(vals, strings.TrimSuffix(rrs.AliasTarget.DNSName, "."))
		}
		if len(vals) > 0 {
			return &dnsOp{name: rrs.Name, recordType: "CNAME", values: vals}
		}
	default:
		if rrs.AliasTarget != nil {
			return &dnsOp{
				name:       rrs.Name,
				recordType: "ALIAS",
				values:     []string{strings.TrimSuffix(rrs.AliasTarget.DNSName, ".")},
			}
		}
	}

	return nil
}

// dnsEligible reports whether rrs is a record type this backend mirrors
// into the embedded DNS server (A/AAAA/CNAME, or any alias record).
func dnsEligible(rrs ResourceRecordSet) bool {
	return rrs.Type == recordTypeA || rrs.Type == recordTypeCNAME ||
		rrs.Type == recordTypeAAAA || rrs.AliasTarget != nil
}

// applyChange mutates zd for one Change, recording rrs.Name into affected
// when the record is DNS-eligible so the caller can resync it afterward.
// Returns an error only for unknown actions.
func applyChange(zd *zoneData, ch Change, affected map[string]struct{}, hasDNS bool) error {
	rrs := ch.ResourceRecordSet
	key := recordSetKey(rrs.Name, rrs.Type, rrs.SetIdentifier)

	switch ch.Action {
	case ChangeActionCreate, ChangeActionUpsert:
		cp := rrs
		zd.records[key] = &cp

	case ChangeActionDelete:
		delete(zd.records, key)

	default:
		return fmt.Errorf("%w: unknown action %q", ErrInvalidAction, ch.Action)
	}

	if hasDNS && dnsEligible(rrs) {
		affected[rrs.Name] = struct{}{}
	}

	return nil
}

// resyncDNSName clears any embedded-DNS-server registration for name and
// re-derives it from every record set, across every zone, that currently
// resolves it. A plain RegisterRecord/Deregister-per-change approach breaks
// as soon as a name has more than one live record set — an UPSERT that
// replaces a value would leave the old value registered alongside the new
// one (pkgs/dns.RegisterRecord appends, it never replaces), and deleting one
// of several record sets sharing a name (weighted/failover/multivalue
// routing, or two zones that happen to use the same name) would deregister
// the name entirely instead of leaving the survivors resolvable. Caller must
// hold b.mu and have already confirmed b.dns != nil.
func (b *InMemoryBackend) resyncDNSName(name string) {
	b.dns.Deregister(name)

	for _, zd := range b.zones.All() {
		for _, rrs := range zd.records {
			if rrs.Name != name {
				continue
			}

			if op := collectDNSRegisterOp(*rrs); op != nil {
				b.dns.RegisterRecord(op.name, op.recordType, op.values)
			}
		}
	}
}

// ChangeResourceRecordSets applies a batch of record set changes atomically.
// All changes are validated before any mutation is applied.
func (b *InMemoryBackend) ChangeResourceRecordSets(
	zoneID string,
	changes []Change,
) (string, error) {
	b.mu.Lock("ChangeResourceRecordSets")
	defer b.mu.Unlock()

	zd, ok := b.zones.Get(zoneID)
	if !ok {
		return "", fmt.Errorf("%w: hosted zone %s not found", ErrHostedZoneNotFound, zoneID)
	}

	if len(changes) > maxChangesPerBatch {
		return "", fmt.Errorf(
			"%w: batch exceeds maximum of %d changes",
			ErrInvalidAction, maxChangesPerBatch,
		)
	}

	// Normalise names and validate all changes before mutating anything.
	normalised := make([]Change, len(changes))
	for i, ch := range changes {
		ch.ResourceRecordSet.Name = normaliseName(ch.ResourceRecordSet.Name)
		normalised[i] = ch

		if err := validateChange(zd, ch); err != nil {
			return "", err
		}
	}

	// All valid — apply.
	affected := make(map[string]struct{})

	for _, ch := range normalised {
		if err := applyChange(zd, ch, affected, b.dns != nil); err != nil {
			return "", err
		}
	}

	// Resync DNS outside the record mutation loop, once the batch's final
	// state is known, so a name with more than one surviving record set is
	// re-derived correctly rather than incrementally patched.
	if b.dns != nil {
		for name := range affected {
			b.resyncDNSName(name)
		}
	}

	// Record the change for GetChange.
	changeID := "C" + randomID("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 14) //nolint:mnd // 14-char AWS-style change ID
	ci := &ChangeInfo{
		ID:          "/change/" + changeID,
		Status:      "INSYNC",
		SubmittedAt: time.Now(),
	}
	b.changes.Put(ci)

	return ci.ID, nil
}

// GetChange returns the ChangeInfo for a given change ID.
func (b *InMemoryBackend) GetChange(changeID string) (*ChangeInfo, error) {
	b.mu.RLock("GetChange")
	defer b.mu.RUnlock()

	ci, ok := b.changes.Get(changeID)
	if !ok {
		return nil, fmt.Errorf("%w: change %s not found", ErrChangeNotFound, changeID)
	}

	cp := *ci

	return &cp, nil
}

// ListResourceRecordSets returns resource record sets sorted by (Name, Type, SetIdentifier),
// starting after the given (startName, startType, startIdentifier) tuple, up to maxItems.
// Pass empty strings and 0 to get the first page.
//
// sortRecordSets orders record sets the way AWS ListResourceRecordSets does —
// by Name, then Type, then SetIdentifier — the same lexicographic order
// seekRecordSetStart's pagination cursor walks below.
func sortRecordSets(all []ResourceRecordSet) {
	sort.Slice(all, func(i, j int) bool {
		if all[i].Name != all[j].Name {
			return all[i].Name < all[j].Name
		}

		if all[i].Type != all[j].Type {
			return all[i].Type < all[j].Type
		}

		return all[i].SetIdentifier < all[j].SetIdentifier
	})
}

// seekRecordSetStart returns the index of the first record set at or after
// the (startName, startType, startIdentifier) pagination cursor within a
// slice already sorted by sortRecordSets. An empty startName means "from the
// beginning" (index 0).
func seekRecordSetStart(all []ResourceRecordSet, startName, startType, startIdentifier string) int {
	if startName == "" {
		return 0
	}

	startName = normaliseName(startName)

	start := 0
	for start < len(all) {
		r := all[start]
		if r.Name > startName {
			break
		}

		if r.Name == startName && (startType == "" || r.Type >= startType) {
			if startType == "" || r.Type > startType || startIdentifier == "" ||
				r.SetIdentifier >= startIdentifier {
				break
			}
		}

		start++
	}

	return start
}

// paginateRecordSets truncates an already-sorted-and-seeked slice to maxItems
// (clamped to route53DefaultMaxItems when unset or out of range), filling in
// the Next* pagination cursor fields when more records remain.
func paginateRecordSets(all []ResourceRecordSet, maxItems int) RRSetPage {
	if maxItems <= 0 || maxItems > route53DefaultMaxItems {
		maxItems = route53DefaultMaxItems
	}

	if len(all) <= maxItems {
		return RRSetPage{Records: all}
	}

	next := all[maxItems]

	return RRSetPage{
		Records:        all[:maxItems],
		IsTruncated:    true,
		NextName:       next.Name,
		NextType:       next.Type,
		NextIdentifier: next.SetIdentifier,
	}
}

// ListResourceRecordSets returns a page of the hosted zone's record sets,
// ordered by Name/Type/SetIdentifier, starting from an optional pagination
// cursor.
func (b *InMemoryBackend) ListResourceRecordSets(
	zoneID, startName, startType, startIdentifier string,
	maxItems int,
) (RRSetPage, error) {
	b.mu.RLock("ListResourceRecordSets")
	defer b.mu.RUnlock()

	zd, ok := b.zones.Get(zoneID)
	if !ok {
		return RRSetPage{}, fmt.Errorf(
			"%w: hosted zone %s not found",
			ErrHostedZoneNotFound,
			zoneID,
		)
	}

	all := make([]ResourceRecordSet, 0, len(zd.records))
	for _, rrs := range zd.records {
		cp := *rrs
		all = append(all, cp)
	}

	sortRecordSets(all)

	start := seekRecordSetStart(all, startName, startType, startIdentifier)

	return paginateRecordSets(all[start:], maxItems), nil
}

// recordValues returns the literal resource-record values of a record set
// without any alias recursion. It is used for DELETE match comparison.
func recordValues(rrs *ResourceRecordSet) []string {
	values := make([]string, 0, len(rrs.Records))
	for _, r := range rrs.Records {
		values = append(values, r.Value)
	}

	return values
}

// rrsValues resolves a record set to its answer values. Alias record sets
// recurse into their in-zone target set (honouring EvaluateTargetHealth); when
// the target is out-of-zone it falls back to the literal AliasTarget.DNSName.
func (b *InMemoryBackend) rrsValues(zd *zoneData, rrs *ResourceRecordSet, depth int) []string {
	if rrs.AliasTarget != nil {
		return b.resolveAlias(zd, rrs, depth)
	}

	values := make([]string, 0, len(rrs.Records))
	for _, r := range rrs.Records {
		values = append(values, r.Value)
	}

	return values
}

// resolveAlias resolves an alias record set. It recurses into the target record
// set when it lives in the same hosted zone, following simple and routing-policy
// targets. When EvaluateTargetHealth is set and the resolved target is unhealthy
// (or resolves to nothing), the alias yields no answer — matching Route 53. For
// out-of-zone targets it returns the literal target DNS name.
func (b *InMemoryBackend) resolveAlias(zd *zoneData, rrs *ResourceRecordSet, depth int) []string {
	if depth >= maxAliasDepth {
		return []string{}
	}

	targetName := normaliseName(rrs.AliasTarget.DNSName)

	// In-zone simple target?
	if target, found := zd.records[recordSetKey(targetName, rrs.Type, "")]; found {
		if rrs.AliasTarget.EvaluateTargetHealth && !b.recordHealthy(target) {
			return []string{}
		}
		vals := b.rrsValues(zd, target, depth+1)
		if rrs.AliasTarget.EvaluateTargetHealth && len(vals) == 0 {
			return []string{}
		}

		return vals
	}

	// In-zone routing-policy target set (has SetIdentifier records)?
	if candidates := b.collectRoutingCandidates(zd, targetName, rrs.Type); len(candidates) > 0 {
		vals := b.selectAnswer(zd, candidates, DNSQueryContext{}, depth+1)
		if rrs.AliasTarget.EvaluateTargetHealth && len(vals) == 0 {
			return []string{}
		}

		return vals
	}

	// Out-of-zone target: return the literal alias DNS name.
	return []string{strings.TrimSuffix(rrs.AliasTarget.DNSName, ".")}
}

// collectRoutingCandidates gathers all SetIdentifier-bearing record sets for a
// name/type. The caller must hold at least a read lock.
func (b *InMemoryBackend) collectRoutingCandidates(
	zd *zoneData,
	name, recordType string,
) []*ResourceRecordSet {
	prefix := strings.ToLower(strings.TrimSuffix(name, ".")) + "|" + strings.ToUpper(recordType) + "|"

	var candidates []*ResourceRecordSet
	for k, rrs := range zd.records {
		if strings.HasPrefix(k, prefix) && rrs.SetIdentifier != "" {
			candidates = append(candidates, rrs)
		}
	}

	return candidates
}

// singleAnswerSelector picks the single winning record set for a routing
// policy from its health-filtered candidates, or nil when nothing should
// answer (e.g. no geolocation match). It takes the backend so selectors that
// need extra state (routingCIDR's CIDR collection lookup) can use it; the
// others simply ignore it.
type singleAnswerSelector func(
	b *InMemoryBackend, healthy []*ResourceRecordSet, qctx DNSQueryContext,
) *ResourceRecordSet

// singleAnswerSelectors maps every routing kind that resolves to one winning
// record set to its selection function. routingMultiValue and routingSimple
// are deliberately absent — selectAnswer handles both before ever consulting
// this table, since neither fits the "one selector function" shape
// (routingMultiValue returns several values, not one winning record set;
// routingSimple has no selection algorithm beyond "the only candidate").
//
//nolint:gochecknoglobals // read-only dispatch table built once, lazily, via sync.OnceValue
var singleAnswerSelectors = sync.OnceValue(newSingleAnswerSelectors)

//nolint:exhaustive // routingMultiValue/routingSimple are handled directly by selectAnswer, see doc comment above
func newSingleAnswerSelectors() map[routingKind]singleAnswerSelector {
	return map[routingKind]singleAnswerSelector{
		routingWeighted: func(_ *InMemoryBackend, healthy []*ResourceRecordSet, qctx DNSQueryContext) *ResourceRecordSet {
			return selectWeighted(healthy, qctx)
		},
		routingLatency: func(_ *InMemoryBackend, healthy []*ResourceRecordSet, qctx DNSQueryContext) *ResourceRecordSet {
			return selectLatency(healthy, qctx)
		},
		routingGeo: func(_ *InMemoryBackend, healthy []*ResourceRecordSet, qctx DNSQueryContext) *ResourceRecordSet {
			return selectGeo(healthy, qctx)
		},
		routingGeoProximity: func(_ *InMemoryBackend, healthy []*ResourceRecordSet, qctx DNSQueryContext) *ResourceRecordSet {
			return selectGeoProximity(healthy, qctx)
		},
		routingCIDR: func(b *InMemoryBackend, healthy []*ResourceRecordSet, qctx DNSQueryContext) *ResourceRecordSet {
			return b.selectCIDR(healthy, qctx)
		},
		routingFailover: func(_ *InMemoryBackend, healthy []*ResourceRecordSet, _ DNSQueryContext) *ResourceRecordSet {
			return selectFailover(healthy)
		},
	}
}

// selectAnswer applies the routing policy shared by the candidate record
// sets and returns the resolved answer values for the winning record(s).
// Health checks are consulted so unhealthy records are excluded. The caller
// must hold at least a read lock.
func (b *InMemoryBackend) selectAnswer(
	zd *zoneData,
	candidates []*ResourceRecordSet,
	qctx DNSQueryContext,
	depth int,
) []string {
	kind := classifyRouting(candidates)
	healthy := b.filterHealthy(candidates)

	if kind == routingMultiValue {
		return b.multiValueAnswer(zd, healthy, depth)
	}

	if kind == routingSimple {
		if len(healthy) == 0 {
			return []string{}
		}

		sortBySetIdentifier(healthy)

		return b.rrsValues(zd, healthy[0], depth)
	}

	selector, ok := singleAnswerSelectors()[kind]
	if !ok {
		return []string{}
	}

	chosen := selector(b, healthy, qctx)
	if chosen == nil {
		return []string{}
	}

	return b.rrsValues(zd, chosen, depth)
}

// multiValueAnswer returns the values of up to maxMultiValueRecords healthy
// record sets, ordered by SetIdentifier for determinism.
func (b *InMemoryBackend) multiValueAnswer(
	zd *zoneData,
	healthy []*ResourceRecordSet,
	depth int,
) []string {
	sortBySetIdentifier(healthy)

	values := make([]string, 0, len(healthy))
	for _, rrs := range healthy {
		values = append(values, b.rrsValues(zd, rrs, depth)...)
		if len(values) >= maxMultiValueRecords {
			return values[:maxMultiValueRecords]
		}
	}

	return values
}

// TestDNSAnswer looks up a record in the hosted zone and returns the values the
// resolver would answer with, applying routing policy (weighted, latency,
// geolocation, failover, multivalue), health-check status and alias recursion.
// The DNSQueryContext supplies client-side signals (region, geo, weighted RNG).
func (b *InMemoryBackend) TestDNSAnswer(
	zoneID, recordName, recordType string,
	qctx DNSQueryContext,
) ([]string, error) {
	b.mu.RLock("TestDNSAnswer")
	defer b.mu.RUnlock()

	zd, ok := b.zones.Get(zoneID)
	if !ok {
		return nil, fmt.Errorf("%w: hosted zone %s not found", ErrHostedZoneNotFound, zoneID)
	}

	name := normaliseName(recordName)

	// Simple (non-routing-policy) record wins outright.
	if rrs, found := zd.records[recordSetKey(name, recordType, "")]; found {
		return b.rrsValues(zd, rrs, 0), nil
	}

	candidates := b.collectRoutingCandidates(zd, name, recordType)
	if len(candidates) == 0 {
		return []string{}, nil
	}

	return b.selectAnswer(zd, candidates, qctx, 0), nil
}

// CountResourceRecordSets returns the number of resource record sets in the
// given hosted zone. It returns ErrHostedZoneNotFound if the zone does not exist.
func (b *InMemoryBackend) CountResourceRecordSets(zoneID string) (int, error) {
	b.mu.RLock("CountResourceRecordSets")
	defer b.mu.RUnlock()

	zd, ok := b.zones.Get(zoneID)
	if !ok {
		return 0, fmt.Errorf("%w: hosted zone %s not found", ErrHostedZoneNotFound, zoneID)
	}

	return len(zd.records), nil
}
