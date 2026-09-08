package route53resolver

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const dirPrefixLen = 2

// copyIPAddressesWithIDs copies ips, assigning a generated IPID to any entry
// that doesn't already have one.
func copyIPAddressesWithIDs(ips []IPAddress) []IPAddress {
	cp := make([]IPAddress, len(ips))
	for i, ip := range ips {
		cp[i] = ip
		if cp[i].IPID == "" {
			cp[i].IPID = "rni-" + uuid.New().String()[:8]
		}
	}

	return cp
}

func (b *InMemoryBackend) CreateResolverEndpoint(
	ctx context.Context,
	name, direction, vpcID string,
	ips []IPAddress,
	securityGroupIDs []string,
	resolverEndpointType string,
	protocols []string,
	outpostArn, preferredInstanceType, creatorRequestID string,
	rniEnhancedMetricsEnabled, targetNameServerMetricsEnabled bool,
	dns64Enabled, ipv6InternetAccessEnabled bool,
) (*ResolverEndpoint, error) {
	b.mu.Lock("CreateResolverEndpoint")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidation)
	}

	if direction != directionInbound && direction != directionOutbound {
		return nil, fmt.Errorf(
			"%w: Direction must be %s or %s",
			ErrValidation,
			directionInbound,
			directionOutbound,
		)
	}

	if resolverEndpointType == "" {
		resolverEndpointType = endpointTypeIPV4
	}
	switch resolverEndpointType {
	case endpointTypeIPV4, endpointTypeIPV6, endpointTypeDualStack:
		// valid
	default:
		return nil, fmt.Errorf(
			"%w: ResolverEndpointType must be IPV4, IPV6, or DUALSTACK",
			ErrValidation,
		)
	}

	if len(protocols) == 0 {
		protocols = []string{"Do53"}
	}

	dirPrefix := direction
	if len(dirPrefix) > dirPrefixLen {
		dirPrefix = dirPrefix[:dirPrefixLen]
	}
	id := "rslvr-" + dirPrefix + "-" + uuid.New().String()[:8]
	epARN := arn.Build("route53resolver", region, b.accountID, "resolver-endpoint/"+id)

	ipsCopy := copyIPAddressesWithIDs(ips)

	sgCopy := make([]string, len(securityGroupIDs))
	copy(sgCopy, securityGroupIDs)

	protocolsCopy := make([]string, len(protocols))
	copy(protocolsCopy, protocols)

	if ep, handled, err := b.matchExistingEndpointByCreatorRequestID(
		region, creatorRequestID, name, direction, vpcID, ipsCopy, sgCopy, resolverEndpointType,
		protocolsCopy, outpostArn, preferredInstanceType,
		rniEnhancedMetricsEnabled, targetNameServerMetricsEnabled,
		dns64Enabled, ipv6InternetAccessEnabled,
	); handled {
		return ep, err
	}

	now := currentTime()
	ep := &ResolverEndpoint{
		ID:                             id,
		ARN:                            epARN,
		Name:                           name,
		Direction:                      direction,
		Status:                         statusOperational,
		VpcID:                          vpcID,
		HostVPCID:                      vpcID,
		IPAddresses:                    ipsCopy,
		SecurityGroupIDs:               sgCopy,
		ResolverEndpointType:           resolverEndpointType,
		AccountID:                      b.accountID,
		Region:                         region,
		Protocols:                      protocolsCopy,
		OutpostArn:                     outpostArn,
		PreferredInstanceType:          preferredInstanceType,
		CreatorRequestID:               creatorRequestID,
		CreationTime:                   now,
		ModificationTime:               now,
		RniEnhancedMetricsEnabled:      rniEnhancedMetricsEnabled,
		TargetNameServerMetricsEnabled: targetNameServerMetricsEnabled,
		DNS64Enabled:                   dns64Enabled,
		Ipv6InternetAccessEnabled:      ipv6InternetAccessEnabled,
	}
	b.endpoints.Put(ep)

	return cloneEndpoint(ep), nil
}

// ListResolverEndpointIPAddresses returns the IP addresses associated with a resolver endpoint.
func (b *InMemoryBackend) ListResolverEndpointIPAddresses(ctx context.Context, endpointID string) ([]IPAddress, error) {
	b.mu.RLock("ListResolverEndpointIpAddresses")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	ep, ok := b.endpoints.Get(regionalKey(region, endpointID))
	if !ok {
		return nil, fmt.Errorf("%w: resolver endpoint %s not found", ErrNotFound, endpointID)
	}
	cp := make([]IPAddress, len(ep.IPAddresses))
	copy(cp, ep.IPAddresses)

	return cp, nil
}

func (b *InMemoryBackend) GetResolverEndpoint(ctx context.Context, id string) (*ResolverEndpoint, error) {
	b.mu.RLock("GetResolverEndpoint")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	ep, ok := b.endpoints.Get(regionalKey(region, id))
	if !ok {
		return nil, fmt.Errorf("%w: resolver endpoint %s not found", ErrNotFound, id)
	}

	return cloneEndpoint(ep), nil
}

func (b *InMemoryBackend) ListResolverEndpoints(ctx context.Context) []*ResolverEndpoint {
	b.mu.RLock("ListResolverEndpoints")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	regionEps := b.endpointsByRegion.Get(region)
	list := make([]*ResolverEndpoint, 0, len(regionEps))
	for _, ep := range regionEps {
		list = append(list, cloneEndpoint(ep))
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	return list
}

func (b *InMemoryBackend) DeleteResolverEndpoint(ctx context.Context, id string) error {
	b.mu.Lock("DeleteResolverEndpoint")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	ep, ok := b.endpoints.Get(regionalKey(region, id))
	if !ok {
		return fmt.Errorf("%w: resolver endpoint %s not found", ErrNotFound, id)
	}

	tags := b.tagsStore(region)

	// Clean up tags.
	delete(tags, ep.ARN)

	// Cascade: delete rules belonging to this endpoint, plus their tags and
	// rule associations. slices.Clone before deleting in the loop: Table.Delete
	// mutates the byRegion index in place, so iterating the live index result
	// directly while deleting from it would be unsafe.
	regionRules := slices.Clone(b.rulesByRegion.Get(region))
	for _, r := range regionRules {
		if r.ResolverEndpointID != id {
			continue
		}

		delete(tags, r.ARN)

		regionAssocs := slices.Clone(b.ruleAssociationsByRegion.Get(region))
		for _, assoc := range regionAssocs {
			if assoc.ResolverRuleID == r.ID {
				b.ruleAssociations.Delete(regionalKey(region, assoc.ID))
			}
		}

		b.rules.Delete(regionalKey(region, r.ID))
	}

	b.endpoints.Delete(regionalKey(region, id))

	return nil
}

// AssociateResolverEndpointIPAddress adds an IP address to a resolver endpoint.
func (b *InMemoryBackend) AssociateResolverEndpointIPAddress(
	ctx context.Context,
	endpointID, subnetID, ip, ipv6 string,
) (*ResolverEndpoint, error) {
	b.mu.Lock("AssociateResolverEndpointIPAddress")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	ep, ok := b.endpoints.Get(regionalKey(region, endpointID))
	if !ok {
		return nil, fmt.Errorf("%w: resolver endpoint %s not found", ErrNotFound, endpointID)
	}

	newIP := IPAddress{
		IPID:     "rni-" + uuid.New().String()[:8],
		SubnetID: subnetID,
		IP:       ip,
		Ipv6:     ipv6,
	}
	ep.IPAddresses = append(ep.IPAddresses, newIP)
	ep.ModificationTime = currentTime()

	return cloneEndpoint(ep), nil
}

// cloneEndpoint returns a deep copy of a ResolverEndpoint.
// matchExistingEndpointByCreatorRequestID looks up an existing resolver
// endpoint created under creatorRequestID. handled is false when
// creatorRequestID is empty or no match is found (caller proceeds to
// create); when handled is true, the caller must return (ep, err) as-is --
// either the existing resource (matching retry) or a ResourceExistsException
// (conflicting retry, see resolverEndpointMatchesCreate).
func (b *InMemoryBackend) matchExistingEndpointByCreatorRequestID(
	region, creatorRequestID, name, direction, vpcID string,
	ips []IPAddress,
	securityGroupIDs []string,
	resolverEndpointType string,
	protocols []string,
	outpostArn, preferredInstanceType string,
	rniEnhancedMetricsEnabled, targetNameServerMetricsEnabled bool,
	dns64Enabled, ipv6InternetAccessEnabled bool,
) (*ResolverEndpoint, bool, error) {
	if creatorRequestID == "" {
		return nil, false, nil
	}

	for _, existing := range b.endpointsByRegion.Get(region) {
		if existing.CreatorRequestID != creatorRequestID {
			continue
		}
		if resolverEndpointMatchesCreate(
			existing, name, direction, vpcID, ips, securityGroupIDs, resolverEndpointType,
			protocols, outpostArn, preferredInstanceType,
			rniEnhancedMetricsEnabled, targetNameServerMetricsEnabled,
			dns64Enabled, ipv6InternetAccessEnabled,
		) {
			return cloneEndpoint(existing), true, nil
		}

		return nil, true, fmt.Errorf(
			"%w: a resolver endpoint already exists for CreatorRequestId %s with different parameters",
			ErrAlreadyExists, creatorRequestID,
		)
	}

	return nil, false, nil
}

// resolverEndpointMatchesCreate reports whether a CreateResolverEndpoint
// retry's parameters match the resource already created under the same
// CreatorRequestId -- a matching retry returns the existing resource
// (CreatorRequestId doc comment, api_op_CreateResolverEndpoint.go:38-40:
// "allows failed requests to be retried without the risk of running the
// operation twice"); a conflicting one raises ResourceExistsException, the
// error CreateResolverEndpoint's own deserializer declares.
func resolverEndpointMatchesCreate(
	existing *ResolverEndpoint,
	name, direction, vpcID string,
	ips []IPAddress,
	securityGroupIDs []string,
	resolverEndpointType string,
	protocols []string,
	outpostArn, preferredInstanceType string,
	rniEnhancedMetricsEnabled, targetNameServerMetricsEnabled bool,
	dns64Enabled, ipv6InternetAccessEnabled bool,
) bool {
	return existing.Name == name &&
		existing.Direction == direction &&
		existing.VpcID == vpcID &&
		existing.ResolverEndpointType == resolverEndpointType &&
		existing.OutpostArn == outpostArn &&
		existing.PreferredInstanceType == preferredInstanceType &&
		existing.RniEnhancedMetricsEnabled == rniEnhancedMetricsEnabled &&
		existing.TargetNameServerMetricsEnabled == targetNameServerMetricsEnabled &&
		existing.DNS64Enabled == dns64Enabled &&
		existing.Ipv6InternetAccessEnabled == ipv6InternetAccessEnabled &&
		slices.Equal(existing.SecurityGroupIDs, securityGroupIDs) &&
		slices.Equal(existing.Protocols, protocols) &&
		ipAddressesEqualIgnoringID(existing.IPAddresses, ips)
}

// ipAddressesEqualIgnoringID compares caller-supplied IP address fields,
// ignoring IPID: IPID is server-generated (see the uuid assignment in
// CreateResolverEndpoint above), so it never matches across two otherwise
// identical create calls.
func ipAddressesEqualIgnoringID(a, b []IPAddress) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].SubnetID != b[i].SubnetID || a[i].IP != b[i].IP || a[i].Ipv6 != b[i].Ipv6 {
			return false
		}
	}

	return true
}

func cloneEndpoint(ep *ResolverEndpoint) *ResolverEndpoint {
	cp := *ep
	cp.IPAddresses = make([]IPAddress, len(ep.IPAddresses))
	copy(cp.IPAddresses, ep.IPAddresses)

	if ep.SecurityGroupIDs != nil {
		cp.SecurityGroupIDs = make([]string, len(ep.SecurityGroupIDs))
		copy(cp.SecurityGroupIDs, ep.SecurityGroupIDs)
	} else {
		cp.SecurityGroupIDs = []string{}
	}

	if ep.Protocols != nil {
		cp.Protocols = make([]string, len(ep.Protocols))
		copy(cp.Protocols, ep.Protocols)
	}

	return &cp
}

// AddEndpointInternal adds a resolver endpoint directly to the backend (test seed helper).
func (b *InMemoryBackend) AddEndpointInternal(name, direction string) *ResolverEndpoint {
	b.mu.Lock("AddEndpointInternal")
	defer b.mu.Unlock()

	id := "rslvr-in-" + uuid.New().String()[:8]
	epARN := arn.Build("route53resolver", b.region, b.accountID, "resolver-endpoint/"+id)
	ep := &ResolverEndpoint{
		ID:               id,
		ARN:              epARN,
		Name:             name,
		Direction:        direction,
		Status:           statusOperational,
		IPAddresses:      []IPAddress{},
		SecurityGroupIDs: []string{},
		AccountID:        b.accountID,
		Region:           b.region,
	}
	b.endpoints.Put(ep)

	return cloneEndpoint(ep)
}

// UpdateResolverEndpoint updates name, endpoint type, and/or protocols of a resolver endpoint.
func (b *InMemoryBackend) UpdateResolverEndpoint(
	ctx context.Context,
	id, name, resolverEndpointType string,
	protocols []string,
	rniEnhancedMetricsEnabled, targetNameServerMetricsEnabled *bool,
	dns64Enabled, ipv6InternetAccessEnabled *bool,
	updateIPAddresses []UpdateIPAddress,
) (*ResolverEndpoint, error) {
	b.mu.Lock("UpdateResolverEndpoint")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	ep, ok := b.endpoints.Get(regionalKey(region, id))
	if !ok {
		return nil, fmt.Errorf("%w: resolver endpoint %s not found", ErrNotFound, id)
	}
	switch resolverEndpointType {
	case "", endpointTypeIPV4, endpointTypeIPV6, endpointTypeDualStack:
	default:
		return nil, fmt.Errorf(
			"%w: ResolverEndpointType must be IPV4, IPV6, or DUALSTACK",
			ErrValidation,
		)
	}
	if err := validateUpdateIPAddresses(ep.IPAddresses, updateIPAddresses); err != nil {
		return nil, err
	}
	if name != "" {
		ep.Name = name
	}
	if resolverEndpointType != "" {
		ep.ResolverEndpointType = resolverEndpointType
	}
	if len(protocols) > 0 {
		protocolsCopy := make([]string, len(protocols))
		copy(protocolsCopy, protocols)
		ep.Protocols = protocolsCopy
	}
	if rniEnhancedMetricsEnabled != nil {
		ep.RniEnhancedMetricsEnabled = *rniEnhancedMetricsEnabled
	}
	if targetNameServerMetricsEnabled != nil {
		ep.TargetNameServerMetricsEnabled = *targetNameServerMetricsEnabled
	}
	if dns64Enabled != nil {
		ep.DNS64Enabled = *dns64Enabled
	}
	if ipv6InternetAccessEnabled != nil {
		ep.Ipv6InternetAccessEnabled = *ipv6InternetAccessEnabled
	}
	applyUpdateIPAddresses(ep.IPAddresses, updateIPAddresses)
	ep.ModificationTime = currentTime()

	return cloneEndpoint(ep), nil
}

// UpdateIPAddress carries a per-IP IPv6 assignment from
// UpdateResolverEndpointInput.UpdateIpAddresses -- verified against
// api_op_UpdateResolverEndpoint.go: "Specifies the IPv6 address when you
// update the Resolver endpoint from IPv4 to dual-stack." Each entry
// identifies an existing IP by IpId (the id returned by
// ListResolverEndpointIpAddresses) and supplies its new Ipv6 value.
type UpdateIPAddress struct {
	IPID string
	Ipv6 string
}

// validateUpdateIPAddresses rejects an UpdateIpAddresses entry that names an
// IpId not present on the endpoint, before any field is mutated -- same
// validate-before-mutate discipline as ResolverEndpointType (see
// TestUpdateResolverEndpoint_RejectedTypeLeavesNameUnchanged).
func validateUpdateIPAddresses(existing []IPAddress, updates []UpdateIPAddress) error {
	for _, u := range updates {
		found := false
		for _, ip := range existing {
			if ip.IPID == u.IPID {
				found = true

				break
			}
		}
		if !found {
			return fmt.Errorf("%w: IP address %s not found on endpoint", ErrNotFound, u.IPID)
		}
	}

	return nil
}

func applyUpdateIPAddresses(existing []IPAddress, updates []UpdateIPAddress) {
	for _, u := range updates {
		for i := range existing {
			if existing[i].IPID == u.IPID {
				existing[i].Ipv6 = u.Ipv6

				break
			}
		}
	}
}

// DisassociateResolverEndpointIPAddress removes an IP address from a resolver endpoint.
func (b *InMemoryBackend) DisassociateResolverEndpointIPAddress(
	ctx context.Context,
	endpointID, ipID string,
) (*ResolverEndpoint, error) {
	b.mu.Lock("DisassociateResolverEndpointIPAddress")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	ep, ok := b.endpoints.Get(regionalKey(region, endpointID))
	if !ok {
		return nil, fmt.Errorf("%w: resolver endpoint %s not found", ErrNotFound, endpointID)
	}

	newIPs := make([]IPAddress, 0, len(ep.IPAddresses))
	found := false
	for _, ip := range ep.IPAddresses {
		if ip.IPID == ipID {
			found = true

			continue
		}
		newIPs = append(newIPs, ip)
	}
	if !found {
		return nil, fmt.Errorf(
			"%w: IP address %s not found on endpoint %s",
			ErrNotFound,
			ipID,
			endpointID,
		)
	}
	ep.IPAddresses = newIPs

	return cloneEndpoint(ep), nil
}

// --- Resolver Rule Update ---
