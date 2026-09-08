package outposts

import (
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// CreateSite creates a new Outposts Site.
func (b *InMemoryBackend) CreateSite(req *createSiteRequest) (*Site, error) {
	if req.Name == "" {
		return nil, validationError("Name is required")
	}

	b.mu.Lock("CreateSite")
	defer b.mu.Unlock()

	if b.sites.Len() >= maxSitesPerRegion {
		return nil, quotaExceededError("maximum number of sites per Region reached")
	}

	id := newSiteID()
	t := tags.New("outposts.site." + id + ".tags")
	t.Merge(req.Tags)

	s := &Site{
		ID:                     id,
		ARN:                    b.SiteARN(id),
		AccountID:              b.accountID,
		Name:                   req.Name,
		Description:            req.Description,
		Notes:                  req.Notes,
		OperatingAddress:       fromAddressWire(req.OperatingAddress),
		ShippingAddress:        fromAddressWire(req.ShippingAddress),
		RackPhysicalProperties: fromRackPropsWire(req.RackPhysicalProperties),
		Tags:                   t,
	}

	b.sites.Put(s)

	return s.clone(), nil
}

// clone returns a deep copy of s, so the returned Site shares no mutable
// memory with the backend's stored copy. OperatingAddress/ShippingAddress/
// RackPhysicalProperties are pointers -- UpdateSiteRackPhysicalProperties in
// particular mutates the pointed-to RackPhysicalProperties fields in place
// (see mergeRackPhysicalProperties), which would otherwise race with any
// earlier caller still holding (and reading) a "copy" whose pointer aliases
// the same struct. Tags is deliberately NOT cloned: *tags.Tags is its own
// concurrency-safe type (backed by a safemap with its own lock), so sharing
// the pointer is safe by design -- see pkgs/tags.
func (s *Site) clone() *Site {
	cp := *s
	cp.OperatingAddress = s.OperatingAddress.clone()
	cp.ShippingAddress = s.ShippingAddress.clone()
	cp.RackPhysicalProperties = s.RackPhysicalProperties.clone()

	return &cp
}

// clone returns a deep copy of a, or nil if a is nil.
func (a *Address) clone() *Address {
	if a == nil {
		return nil
	}

	cp := *a

	return &cp
}

// clone returns a deep copy of r, or nil if r is nil.
func (r *RackPhysicalProperties) clone() *RackPhysicalProperties {
	if r == nil {
		return nil
	}

	cp := *r

	return &cp
}

// GetSite returns a copy of the Site identified by idOrARN.
func (b *InMemoryBackend) GetSite(idOrARN string) (*Site, error) {
	b.mu.RLock("GetSite")
	defer b.mu.RUnlock()

	s, ok := b.resolveSiteLocked(idOrARN)
	if !ok {
		return nil, notFoundError(resourceSite, idOrARN)
	}

	return s.clone(), nil
}

// UpdateSite applies a partial update to the Site identified by idOrARN.
func (b *InMemoryBackend) UpdateSite(idOrARN string, req *updateSiteRequest) (*Site, error) {
	b.mu.Lock("UpdateSite")
	defer b.mu.Unlock()

	s, ok := b.resolveSiteLocked(idOrARN)
	if !ok {
		return nil, notFoundError(resourceSite, idOrARN)
	}

	if req.Description != "" {
		s.Description = req.Description
	}

	if req.Name != "" {
		s.Name = req.Name
	}

	if req.Notes != "" {
		s.Notes = req.Notes
	}

	return s.clone(), nil
}

// DeleteSite deletes the Site identified by idOrARN. Rejected
// (ConflictException) while any Outpost still references it -- a real
// FK-integrity check.
func (b *InMemoryBackend) DeleteSite(idOrARN string) error {
	b.mu.Lock("DeleteSite")
	defer b.mu.Unlock()

	s, ok := b.resolveSiteLocked(idOrARN)
	if !ok {
		return notFoundError(resourceSite, idOrARN)
	}

	if outposts := b.outpostsBySite.Get(s.ID); len(outposts) > 0 {
		return conflictError("site has " + resourceOutpost + "s associated with it: " + s.ID)
	}

	if s.Tags != nil {
		s.Tags.Close()
	}

	b.sites.Delete(s.ID)

	return nil
}

// listSitesFilter holds ListSites' optional filters.
type listSitesFilter struct {
	cities        []string
	countryCodes  []string
	statesRegions []string
}

func matchesSiteFilter(s *Site, f listSitesFilter) bool {
	if len(f.cities) > 0 || len(f.countryCodes) > 0 || len(f.statesRegions) > 0 {
		if s.OperatingAddress == nil {
			return false
		}
	}

	if len(f.cities) > 0 && !containsStr(f.cities, s.OperatingAddress.City) {
		return false
	}

	if len(f.countryCodes) > 0 && !containsStr(f.countryCodes, s.OperatingAddress.CountryCode) {
		return false
	}

	if len(f.statesRegions) > 0 && !containsStr(f.statesRegions, s.OperatingAddress.StateOrRegion) {
		return false
	}

	return true
}

// ListSites returns a page of Sites matching f.
func (b *InMemoryBackend) ListSites(f listSitesFilter, token string, limit int) page.Page[*Site] {
	b.mu.RLock("ListSites")
	defer b.mu.RUnlock()

	all := b.sites.Snapshot()
	filtered := make([]*Site, 0, len(all))

	for _, s := range all {
		if matchesSiteFilter(s, f) {
			// Clone before returning: these are the live, backend-owned
			// pointers, and UpdateSite/UpdateSiteAddress/
			// UpdateSiteRackPhysicalProperties mutate them in place after
			// this call returns and the lock is released -- see clone's
			// doc comment.
			filtered = append(filtered, s.clone())
		}
	}

	return page.New(filtered, token, limit, defaultPageLimit)
}

// siteHasInProgressOrderLocked reports whether any Outpost belonging to
// siteID has an order still PREPARING or IN_PROGRESS (this backend's two
// pre-delivery states -- see orders.go). UpdateSiteAddress's doc comment
// says "You can't update a site address if there is an order in progress";
// UpdateSiteRackPhysicalProperties's says "an order of IN_PROGRESS" -- both
// reject while true. Callers must hold b.mu.
func (b *InMemoryBackend) siteHasInProgressOrderLocked(siteID string) bool {
	for _, o := range b.outpostsBySite.Get(siteID) {
		for _, ord := range b.ordersByOutpost.Get(o.ID) {
			if ord.Status == OrderStatusPreparing || ord.Status == OrderStatusInProgress {
				return true
			}
		}
	}

	return false
}

// GetSiteAddress returns the address of the given type for the Site
// identified by idOrARN.
func (b *InMemoryBackend) GetSiteAddress(idOrARN, addressType string) (*Address, error) {
	b.mu.RLock("GetSiteAddress")
	defer b.mu.RUnlock()

	s, ok := b.resolveSiteLocked(idOrARN)
	if !ok {
		return nil, notFoundError(resourceSite, idOrARN)
	}

	addr := b.siteAddressByType(s, addressType)
	if addr == nil {
		return nil, notFoundError("site address", idOrARN)
	}

	return addr.clone(), nil
}

func (b *InMemoryBackend) siteAddressByType(s *Site, addressType string) *Address {
	switch addressType {
	case AddressTypeShipping:
		return s.ShippingAddress
	case AddressTypeOperating:
		return s.OperatingAddress
	default:
		return nil
	}
}

// UpdateSiteAddress replaces the address of the given type for the Site
// identified by idOrARN with addr (a full replacement, not a merge -- the
// only PUT in this service, per its own doc comment).
func (b *InMemoryBackend) UpdateSiteAddress(idOrARN, addressType string, addr *Address) (*Address, error) {
	if addressType != AddressTypeShipping && addressType != AddressTypeOperating {
		return nil, validationError("invalid AddressType: " + addressType)
	}

	b.mu.Lock("UpdateSiteAddress")
	defer b.mu.Unlock()

	s, ok := b.resolveSiteLocked(idOrARN)
	if !ok {
		return nil, notFoundError(resourceSite, idOrARN)
	}

	if b.siteHasInProgressOrderLocked(s.ID) {
		return nil, conflictError("site has an order in progress: " + s.ID)
	}

	cp := *addr

	switch addressType {
	case AddressTypeShipping:
		s.ShippingAddress = &cp
	case AddressTypeOperating:
		s.OperatingAddress = &cp
	}

	return &cp, nil
}

// mergeRackPhysicalProperties overwrites only the non-empty fields of patch
// onto base, creating base if it was nil.
func mergeRackPhysicalProperties(base *RackPhysicalProperties, patch *RackPhysicalProperties) *RackPhysicalProperties {
	if base == nil {
		base = &RackPhysicalProperties{}
	}

	if patch.FiberOpticCableType != "" {
		base.FiberOpticCableType = patch.FiberOpticCableType
	}

	if patch.MaximumSupportedWeightLbs != "" {
		base.MaximumSupportedWeightLbs = patch.MaximumSupportedWeightLbs
	}

	if patch.OpticalStandard != "" {
		base.OpticalStandard = patch.OpticalStandard
	}

	if patch.PowerConnector != "" {
		base.PowerConnector = patch.PowerConnector
	}

	if patch.PowerDrawKva != "" {
		base.PowerDrawKva = patch.PowerDrawKva
	}

	if patch.PowerFeedDrop != "" {
		base.PowerFeedDrop = patch.PowerFeedDrop
	}

	if patch.PowerPhase != "" {
		base.PowerPhase = patch.PowerPhase
	}

	if patch.UplinkCount != "" {
		base.UplinkCount = patch.UplinkCount
	}

	if patch.UplinkGbps != "" {
		base.UplinkGbps = patch.UplinkGbps
	}

	return base
}

// UpdateSiteRackPhysicalProperties merges patch's non-empty fields into the
// Site identified by idOrARN's RackPhysicalProperties.
func (b *InMemoryBackend) UpdateSiteRackPhysicalProperties(
	idOrARN string,
	patch *RackPhysicalProperties,
) (*Site, error) {
	b.mu.Lock("UpdateSiteRackPhysicalProperties")
	defer b.mu.Unlock()

	s, ok := b.resolveSiteLocked(idOrARN)
	if !ok {
		return nil, notFoundError(resourceSite, idOrARN)
	}

	if b.siteHasInProgressOrderLocked(s.ID) {
		return nil, conflictError("site has an order in progress: " + s.ID)
	}

	s.RackPhysicalProperties = mergeRackPhysicalProperties(s.RackPhysicalProperties, patch)

	return s.clone(), nil
}
