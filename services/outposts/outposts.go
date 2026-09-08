package outposts

import (
	"sort"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

func isValidHardwareType(v string) bool {
	return v == HardwareTypeRack || v == HardwareTypeServer
}

// CreateOutpost creates a new Outpost belonging to an existing Site, and
// seeds it with one COMPUTE asset (see assets.go's seedAssetForOutpostLocked).
func (b *InMemoryBackend) CreateOutpost(req *createOutpostRequest) (*Outpost, error) {
	if req.Name == "" {
		return nil, validationError("Name is required")
	}

	if req.SiteId == "" {
		return nil, validationError("SiteId is required")
	}

	if req.SupportedHardwareType != "" && !isValidHardwareType(req.SupportedHardwareType) {
		return nil, validationError("invalid SupportedHardwareType: " + req.SupportedHardwareType)
	}

	b.mu.Lock("CreateOutpost")
	defer b.mu.Unlock()

	site, ok := b.resolveSiteLocked(req.SiteId)
	if !ok {
		return nil, notFoundError(resourceSite, req.SiteId)
	}

	if len(b.outpostsBySite.Get(site.ID)) >= maxOutpostsPerSite {
		return nil, quotaExceededError("maximum number of Outposts per site reached")
	}

	id := newOutpostID()
	t := tags.New("outposts.outpost." + id + ".tags")
	t.Merge(req.Tags)

	o := &Outpost{
		ID:                    id,
		ARN:                   b.OutpostARN(id),
		SiteID:                site.ID,
		SiteARN:               site.ARN,
		Name:                  req.Name,
		Description:           req.Description,
		AvailabilityZone:      req.AvailabilityZone,
		AvailabilityZoneID:    req.AvailabilityZoneId,
		LifeCycleStatus:       LifeCycleStatusActive,
		OwnerID:               b.accountID,
		SupportedHardwareType: req.SupportedHardwareType,
		Tags:                  t,
	}

	b.outposts.Put(o)
	b.seedAssetForOutpostLocked(id)

	return o.clone(), nil
}

// clone returns a deep copy of o, so the returned Outpost shares no mutable
// memory with the backend's stored copy. Subscriptions is a slice (each
// element itself holding an OrderIDs slice) that recordOriginalSubscriptionLocked
// and CreateRenewal append to -- cloning it keeps a caller's copy stable even
// if the backend's slice is later reallocated or its elements read
// concurrently. Tags is deliberately NOT cloned: *tags.Tags is its own
// concurrency-safe type (backed by a safemap with its own lock), so sharing
// the pointer is safe by design -- see pkgs/tags.
func (o *Outpost) clone() *Outpost {
	cp := *o
	cp.Subscriptions = cloneSubscriptions(o.Subscriptions)

	return &cp
}

// cloneSubscriptions returns a deep copy of subs, including each
// Subscription's OrderIDs slice.
func cloneSubscriptions(subs []Subscription) []Subscription {
	if subs == nil {
		return nil
	}

	cp := make([]Subscription, len(subs))

	for i, s := range subs {
		cp[i] = s
		cp[i].OrderIDs = cloneStrs(s.OrderIDs)
	}

	return cp
}

// GetOutpost returns a copy of the Outpost identified by idOrARN.
func (b *InMemoryBackend) GetOutpost(idOrARN string) (*Outpost, error) {
	b.mu.RLock("GetOutpost")
	defer b.mu.RUnlock()

	o, ok := b.resolveOutpostLocked(idOrARN)
	if !ok {
		return nil, notFoundError(resourceOutpost, idOrARN)
	}

	return o.clone(), nil
}

// UpdateOutpost applies a partial update to the Outpost identified by
// idOrARN.
func (b *InMemoryBackend) UpdateOutpost(idOrARN string, req *updateOutpostRequest) (*Outpost, error) {
	if req.SupportedHardwareType != "" && !isValidHardwareType(req.SupportedHardwareType) {
		return nil, validationError("invalid SupportedHardwareType: " + req.SupportedHardwareType)
	}

	b.mu.Lock("UpdateOutpost")
	defer b.mu.Unlock()

	o, ok := b.resolveOutpostLocked(idOrARN)
	if !ok {
		return nil, notFoundError(resourceOutpost, idOrARN)
	}

	if req.Description != "" {
		o.Description = req.Description
	}

	if req.Name != "" {
		o.Name = req.Name
	}

	if req.SupportedHardwareType != "" {
		o.SupportedHardwareType = req.SupportedHardwareType
	}

	return o.clone(), nil
}

// DeleteOutpost deletes the Outpost identified by idOrARN, along with its
// seeded assets. Rejected (ConflictException) while a capacity task is
// still REQUESTED against it -- a real FK-integrity check, not a stub.
func (b *InMemoryBackend) DeleteOutpost(idOrARN string) error {
	b.mu.Lock("DeleteOutpost")
	defer b.mu.Unlock()

	o, ok := b.resolveOutpostLocked(idOrARN)
	if !ok {
		return notFoundError(resourceOutpost, idOrARN)
	}

	for _, t := range b.capacityTasksByOutpost.Get(o.ID) {
		if t.Status == CapacityTaskStatusRequested {
			return conflictErrorWithResource(ResourceTypeOutpost, o.ID, "outpost has an active capacity task: "+t.ID)
		}
	}

	for _, a := range b.assetsByOutpost.Get(o.ID) {
		b.assets.Delete(a.ID)
	}

	// Ghost-row guard: ConsumeCapacity (capacity_ledger.go) keys every
	// runningInstance by AssetID/OutpostID, and DeleteOutpost only blocks on
	// a REQUESTED capacity task -- COMPLETED ones (and any capacity they
	// consumed) don't stop deletion, so orphaned rows would otherwise
	// survive here forever unless services/ec2 later happens to terminate
	// that exact instance ID.
	for _, ri := range b.runningInstancesByOutpost.Get(o.ID) {
		b.runningInstances.Delete(ri.InstanceID)
	}

	// renewalIdempotency entries for o.ID are unreachable the instant o is
	// gone -- CreateRenewal resolves idOrARN before ever consulting the
	// cache, so a retried request fails at resolveOutpostLocked and never
	// reaches renewalIdempotency. Pruning here is pure memory hygiene, not a
	// behavior change.
	prefix := o.ID + "::"
	for k := range b.renewalIdempotency {
		if strings.HasPrefix(k, prefix) {
			delete(b.renewalIdempotency, k)
		}
	}

	if o.Tags != nil {
		o.Tags.Close()
	}

	b.outposts.Delete(o.ID)

	return nil
}

// listOutpostsFilter holds ListOutposts' optional filters.
type listOutpostsFilter struct {
	availabilityZones   []string
	availabilityZoneIDs []string
	lifeCycleStatuses   []string
}

func matchesOutpostFilter(o *Outpost, f listOutpostsFilter) bool {
	if len(f.availabilityZones) > 0 && !containsStr(f.availabilityZones, o.AvailabilityZone) {
		return false
	}

	if len(f.availabilityZoneIDs) > 0 && !containsStr(f.availabilityZoneIDs, o.AvailabilityZoneID) {
		return false
	}

	if len(f.lifeCycleStatuses) > 0 && !containsStr(f.lifeCycleStatuses, o.LifeCycleStatus) {
		return false
	}

	return true
}

// ListOutposts returns a page of Outposts matching f.
func (b *InMemoryBackend) ListOutposts(f listOutpostsFilter, token string, limit int) page.Page[*Outpost] {
	b.mu.RLock("ListOutposts")
	defer b.mu.RUnlock()

	all := b.outposts.Snapshot()
	filtered := make([]*Outpost, 0, len(all))

	for _, o := range all {
		if matchesOutpostFilter(o, f) {
			// Clone before returning: these are the live, backend-owned
			// pointers, and UpdateOutpost/StartOutpostDecommission mutate
			// them in place after this call returns and the lock is
			// released -- see clone's doc comment.
			filtered = append(filtered, o.clone())
		}
	}

	return page.New(filtered, token, limit, defaultPageLimit)
}

// StartOutpostDecommission requests decommissioning of the Outpost
// identified by idOrARN. See PARITY.md's "Outpost lifecycle" note for the
// SKIPPED/BLOCKED/REQUESTED state machine this implements: SKIPPED when a
// decommission is already pending (idempotent replay), REQUESTED otherwise
// (BLOCKED never occurs -- this backend has no cross-service
// blocking-resource data, so BlockingResourceTypes is always empty).
// ValidateOnly performs every check but never mutates state.
func (b *InMemoryBackend) StartOutpostDecommission(idOrARN string, validateOnly bool) (string, error) {
	b.mu.Lock("StartOutpostDecommission")
	defer b.mu.Unlock()

	o, ok := b.resolveOutpostLocked(idOrARN)
	if !ok {
		return "", notFoundError(resourceOutpost, idOrARN)
	}

	if o.LifeCycleStatus == LifeCycleStatusPendingDecommission {
		return DecommissionStatusSkipped, nil
	}

	if !validateOnly {
		o.LifeCycleStatus = LifeCycleStatusPendingDecommission
	}

	return DecommissionStatusRequested, nil
}

// GetOutpostBillingInformation returns a copy of the Outpost identified by
// idOrARN, for the handler to read PaymentOption/PaymentTerm/
// ContractEndDate/Subscriptions off of.
func (b *InMemoryBackend) GetOutpostBillingInformation(idOrARN string) (*Outpost, error) {
	b.mu.RLock("GetOutpostBillingInformation")
	defer b.mu.RUnlock()

	o, ok := b.resolveOutpostLocked(idOrARN)
	if !ok {
		return nil, notFoundError(resourceOutpost, idOrARN)
	}

	return o.clone(), nil
}

// GetOutpostInstanceTypes returns the Outpost (for OutpostArn/OutpostId in
// the response) and the instance-type capacities CURRENTLY CONFIGURED on
// it -- the aggregate of every seeded Asset's
// ComputeAttributes.InstanceTypeCapacities, which StartCapacityTask mutates
// on completion and capacity_ledger.go's ConsumeCapacity/ReleaseCapacity
// deplete/restore as services/ec2 launches/terminates instances onto it.
// An instance type whose available Count has been fully consumed is
// omitted -- matching real AWS depleting an Outpost's currently-configured
// capacity as instances launch onto it. This is deliberately distinct from
// GetOutpostSupportedInstanceTypes (below), which answers a different
// question -- see PARITY.md's trap #5.
func (b *InMemoryBackend) GetOutpostInstanceTypes(idOrARN string) (*Outpost, []InstanceTypeCapacity, error) {
	b.mu.RLock("GetOutpostInstanceTypes")
	defer b.mu.RUnlock()

	o, ok := b.resolveOutpostLocked(idOrARN)
	if !ok {
		return nil, nil, notFoundError(resourceOutpost, idOrARN)
	}

	totals := make(map[string]int32)

	for _, a := range b.assetsByOutpost.Get(o.ID) {
		if a.ComputeAttributes == nil {
			continue
		}

		for _, c := range a.ComputeAttributes.InstanceTypeCapacities {
			totals[c.InstanceType] += c.Count
		}
	}

	instanceTypes := make([]string, 0, len(totals))
	for it, count := range totals {
		if count <= 0 {
			continue
		}

		instanceTypes = append(instanceTypes, it)
	}

	sort.Strings(instanceTypes)

	out := make([]InstanceTypeCapacity, 0, len(instanceTypes))
	for _, it := range instanceTypes {
		out = append(out, InstanceTypeCapacity{InstanceType: it, Count: totals[it]})
	}

	return o.clone(), out, nil
}

// GetOutpostSupportedInstanceTypes returns everything the Outpost's
// hardware type could support -- a superset that "generally include[s]
// instance types that are not currently configured" per the SDK's own doc
// comment. assetID/orderID, if given, must reference real resources but do
// not further filter the (already small, static) result set -- a documented
// simplification, see PARITY.md.
func (b *InMemoryBackend) GetOutpostSupportedInstanceTypes(
	idOrARN, assetID, orderID string,
) ([]OrderableInstanceType, error) {
	b.mu.RLock("GetOutpostSupportedInstanceTypes")
	defer b.mu.RUnlock()

	o, ok := b.resolveOutpostLocked(idOrARN)
	if !ok {
		return nil, notFoundError(resourceOutpost, idOrARN)
	}

	if assetID != "" {
		a, assetOK := b.assets.Get(assetID)
		if !assetOK || a.OutpostID != o.ID {
			return nil, notFoundError(resourceAsset, assetID)
		}
	}

	if orderID != "" {
		ord, orderOK := b.orders.Get(orderID)
		if !orderOK || ord.OutpostID != o.ID {
			return nil, notFoundError(resourceOrder, orderID)
		}
	}

	return orderableInstanceTypesForHardware(o.SupportedHardwareType), nil
}
