package route53

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

const (
	dnsNS1Default = "ns1.gopherstack.invalid"
	dnsNS2Default = "ns2.gopherstack.invalid"

	defaultNSTTL  = 172800 // 48 hours — AWS default for zone-apex NS records
	defaultSOATTL = 900    // 15 minutes — AWS default for SOA records
)

const (
	zoneIDChars  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	zoneIDLength = 13
)

func randomZoneID() string { return randomID(zoneIDChars, zoneIDLength) }

// normaliseName ensures the zone/record name ends with a dot.
func normaliseName(name string) string {
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}

	return name
}

// CreateHostedZone creates a new hosted zone. When delegationSetID is
// non-empty, the zone is linked to that reusable delegation set (which must
// already exist, see ErrDelegationSetNotFound) and inherits its name
// servers; otherwise the zone gets the default system-assigned name
// servers. When private and vpcID is non-empty, the zone is associated with
// that VPC as part of creation — the same as real AWS's CreateHostedZone
// VPC member, which every typed client sends for a private zone.
func (b *InMemoryBackend) CreateHostedZone(
	name, callerRef, comment string,
	private bool,
	delegationSetID string,
	vpcID, vpcRegion string,
) (*HostedZone, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}

	if callerRef == "" {
		return nil, fmt.Errorf("%w: callerReference is required", ErrInvalidInput)
	}

	name = normaliseName(name)

	b.mu.Lock("CreateHostedZone")
	defer b.mu.Unlock()

	existing, err := b.matchExistingHostedZone(name, callerRef, comment, delegationSetID, private)
	if existing != nil || err != nil {
		return existing, err
	}

	nameServers, err := b.resolveZoneNameServers(delegationSetID)
	if err != nil {
		return nil, err
	}

	id := "Z" + randomZoneID()
	hz := HostedZone{
		ID:              id,
		Name:            name,
		CallerReference: callerRef,
		Comment:         comment,
		PrivateZone:     private,
		CreatedAt:       time.Now(),
		DelegationSetID: delegationSetID,
		NameServers:     nameServers,
	}

	zd := &zoneData{
		zone:    hz,
		records: make(map[string]*ResourceRecordSet),
	}
	b.zones.Put(zd)
	seedZoneAutoRecords(zd, name, nameServers)

	if private && vpcID != "" {
		b.vpcAssociations[id] = append(b.vpcAssociations[id], vpcAssociation{
			VPCID:     vpcID,
			VPCRegion: vpcRegion,
		})
	}

	// Register a synthetic INSYNC change so that GetChange on the zone-creation
	// change ID (used by Terraform's waiter) returns INSYNC immediately.
	syntheticChangeID := "C" + id
	b.changes.Put(&ChangeInfo{
		ID:          "/change/" + syntheticChangeID,
		Status:      "INSYNC",
		SubmittedAt: time.Now(),
	})

	cp := hz

	return &cp, nil
}

// matchExistingHostedZone implements CreateHostedZone's CallerReference
// idempotency: reusing a CallerReference with the exact same
// Name/Comment/PrivateZone/DelegationSetID is a safe retry and returns the
// existing zone (non-nil HostedZone, nil error). Reusing it with any
// different parameter is rejected — real AWS returns HostedZoneAlreadyExists
// (409) rather than silently returning (or silently creating a second zone
// for) mismatched input (nil HostedZone, non-nil error). Both return values
// nil means no CallerReference collision was found and zone creation should
// proceed. Caller must hold b.mu.
func (b *InMemoryBackend) matchExistingHostedZone(
	name, callerRef, comment, delegationSetID string,
	private bool,
) (*HostedZone, error) {
	for _, zd := range b.zones.All() {
		if zd.zone.CallerReference != callerRef {
			continue
		}

		if zd.zone.Name == name && zd.zone.Comment == comment &&
			zd.zone.PrivateZone == private && zd.zone.DelegationSetID == delegationSetID {
			cp := zd.zone
			cp.ResourceRecordSetCount = len(zd.records)

			return &cp, nil
		}

		return nil, fmt.Errorf(
			"%w: a hosted zone already exists for CallerReference %s with different parameters",
			ErrHostedZoneAlreadyExists,
			callerRef,
		)
	}

	return nil, nil //nolint:nilnil // (nil, nil) is the documented "no collision" sentinel
}

// resolveZoneNameServers returns the default name servers, or — when
// delegationSetID is non-empty — the reusable delegation set's own name
// servers. Caller must hold b.mu.
func (b *InMemoryBackend) resolveZoneNameServers(delegationSetID string) ([]string, error) {
	if delegationSetID == "" {
		return []string{dnsNS1Default, dnsNS2Default}, nil
	}

	ds, ok := b.reusableDelegationSets.Get(delegationSetID)
	if !ok {
		return nil, fmt.Errorf(
			"%w: reusable delegation set %s not found",
			ErrDelegationSetNotFound,
			delegationSetID,
		)
	}

	return append([]string(nil), ds.NameServers...), nil
}

// seedZoneAutoRecords populates zd with the zone-apex NS and SOA records
// that AWS auto-creates for every new hosted zone, using the zone's actual
// name servers.
func seedZoneAutoRecords(zd *zoneData, name string, nameServers []string) {
	nsValues := make([]ResourceRecord, 0, len(nameServers))
	for _, ns := range nameServers {
		nsValues = append(nsValues, ResourceRecord{Value: ns + "."})
	}

	nsKey := recordSetKey(name, "NS", "")
	zd.records[nsKey] = &ResourceRecordSet{
		Name:    name,
		Type:    "NS",
		TTL:     defaultNSTTL,
		Records: nsValues,
	}
	soaKey := recordSetKey(name, "SOA", "")
	zd.records[soaKey] = &ResourceRecordSet{
		Name: name,
		Type: "SOA",
		TTL:  defaultSOATTL,
		Records: []ResourceRecord{
			{Value: nameServers[0] + ". awsdns-hostmaster.amazon.com. 1 7200 900 1209600 86400"},
		},
	}
}

// zoneUserRecordCount returns the number of records in zd that are not the
// zone-apex NS or SOA records seeded at creation time.
func zoneUserRecordCount(zd *zoneData) int {
	nsKey := recordSetKey(zd.zone.Name, "NS", "")
	soaKey := recordSetKey(zd.zone.Name, "SOA", "")
	count := 0
	for key := range zd.records {
		if key != nsKey && key != soaKey {
			count++
		}
	}

	return count
}

// DeleteHostedZone removes a hosted zone and all its record sets.
func (b *InMemoryBackend) DeleteHostedZone(zoneID string) error {
	b.mu.Lock("DeleteHostedZone")
	defer b.mu.Unlock()

	zd, ok := b.zones.Get(zoneID)
	if !ok {
		return fmt.Errorf("%w: hosted zone %s not found", ErrHostedZoneNotFound, zoneID)
	}

	// AWS rejects deletion of zones that still contain resource record sets,
	// but allows deletion when only the default NS and SOA records remain.
	if zoneUserRecordCount(zd) > 0 {
		return fmt.Errorf(
			"%w: hosted zone %s contains resource record sets that must be deleted first",
			ErrHostedZoneNotEmpty,
			zoneID,
		)
	}

	// AWS also rejects deletion while DNSSEC signing is enabled; disable it
	// (DisableHostedZoneDNSSEC) before deleting.
	if zd.dnssecEnabled {
		return fmt.Errorf(
			"%w: hosted zone %s has DNSSEC signing enabled and must be disabled first",
			ErrHostedZoneNotEmpty,
			zoneID,
		)
	}

	// Deregister all DNS records before deletion.
	if b.dns != nil {
		for _, rrs := range zd.records {
			if rrs.Type == recordTypeA || rrs.Type == recordTypeCNAME || rrs.Type == recordTypeAAAA ||
				rrs.AliasTarget != nil {
				b.dns.Deregister(rrs.Name)
			}
		}
	}

	// Cascade: delete VPC associations for this zone.
	delete(b.vpcAssociations, zoneID)
	// Cascade: delete query logging configs for this zone. The index's
	// backing slice is cloned before the loop because deleting from the
	// table mutates the very index groups the slice is a view over.
	for _, cfg := range slices.Clone(b.queryLoggingConfigsByZone.Get(zoneID)) {
		b.queryLoggingConfigs.Delete(cfg.ID)
	}
	// Cascade: delete key signing keys for this zone. Same clone-before-delete
	// reasoning as above.
	for _, ksk := range slices.Clone(b.keySigningKeysByZone.Get(zoneID)) {
		b.keySigningKeys.Delete(kskKey(ksk.HostedZoneID, ksk.Name))
	}

	b.zones.Delete(zoneID)
	delete(b.tags, zoneID)

	return nil
}

// GetHostedZone returns a single hosted zone.
func (b *InMemoryBackend) GetHostedZone(zoneID string) (*HostedZone, error) {
	b.mu.RLock("GetHostedZone")
	defer b.mu.RUnlock()

	zd, ok := b.zones.Get(zoneID)
	if !ok {
		return nil, fmt.Errorf("%w: hosted zone %s not found", ErrHostedZoneNotFound, zoneID)
	}

	cp := zd.zone
	cp.ResourceRecordSetCount = len(zd.records)

	return &cp, nil
}

// ListHostedZones returns hosted zones sorted by name, with optional pagination.
// delegationSetID restricts results to zones associated with that reusable
// delegation set; hostedZoneType == "PrivateHostedZone" restricts results to
// private zones (route53@v1.65.6 api_op_ListHostedZones.go).
func (b *InMemoryBackend) ListHostedZones(
	marker string,
	maxItems int,
	delegationSetID, hostedZoneType string,
) (page.Page[HostedZone], error) {
	b.mu.RLock("ListHostedZones")
	defer b.mu.RUnlock()

	all := b.zones.All()
	result := make([]HostedZone, 0, len(all))
	for _, zd := range all {
		if delegationSetID != "" && zd.zone.DelegationSetID != delegationSetID {
			continue
		}
		if hostedZoneType == "PrivateHostedZone" && !zd.zone.PrivateZone {
			continue
		}
		cp := zd.zone
		cp.ResourceRecordSetCount = len(zd.records)
		result = append(result, cp)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}

		return result[i].Name < result[j].Name
	})

	return page.New(result, marker, maxItems, route53DefaultMaxItems), nil
}

// ListHostedZonesByName returns hosted zones sorted by name, paginating by DNSName and zoneID.
func (b *InMemoryBackend) ListHostedZonesByName(
	dnsName, zoneID string,
	maxItems int,
) ([]HostedZone, string, string, error) {
	b.mu.RLock("ListHostedZonesByName")
	defer b.mu.RUnlock()

	all := b.zones.All()
	result := make([]HostedZone, 0, len(all))
	for _, zd := range all {
		cp := zd.zone
		cp.ResourceRecordSetCount = len(zd.records)
		result = append(result, cp)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}

		return result[i].Name < result[j].Name
	})

	var startIndex int
	if dnsName != "" {
		startIndex = len(result)
		for i, z := range result {
			if z.Name > dnsName || (z.Name == dnsName && strings.TrimPrefix(z.ID, "/hostedzone/") >= zoneID) {
				startIndex = i

				break
			}
		}
	}

	if startIndex >= len(result) {
		return []HostedZone{}, "", "", nil
	}

	endIndex := startIndex + maxItems
	var nextDNSName, nextZoneID string
	if endIndex < len(result) {
		nextDNSName = result[endIndex].Name
		nextZoneID = strings.TrimPrefix(result[endIndex].ID, "/hostedzone/")
	} else {
		endIndex = len(result)
	}

	return result[startIndex:endIndex], nextDNSName, nextZoneID, nil
}

// AddZoneInternal adds a hosted zone directly into the backend for testing.
func (b *InMemoryBackend) AddZoneInternal(hz HostedZone) {
	b.mu.Lock("AddZoneInternal")
	defer b.mu.Unlock()
	b.zones.Put(&zoneData{zone: hz, records: make(map[string]*ResourceRecordSet)})
}

// UpdateHostedZoneComment updates the comment on an existing hosted zone.
func (b *InMemoryBackend) UpdateHostedZoneComment(zoneID, comment string) (*HostedZone, error) {
	b.mu.Lock("UpdateHostedZoneComment")
	defer b.mu.Unlock()

	zd, ok := b.zones.Get(zoneID)
	if !ok {
		return nil, fmt.Errorf("%w: hosted zone %s not found", ErrHostedZoneNotFound, zoneID)
	}

	zd.zone.Comment = comment
	cp := zd.zone
	cp.ResourceRecordSetCount = len(zd.records)

	return &cp, nil
}

// GetHostedZoneCount returns the total number of hosted zones.
func (b *InMemoryBackend) GetHostedZoneCount() int {
	b.mu.RLock("GetHostedZoneCount")
	defer b.mu.RUnlock()

	return b.zones.Len()
}
