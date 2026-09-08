package elbv2

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// validateLBName applies load-balancer-specific name rules on top of validateResourceName:
// underscores are not allowed, and the name must be at least 2 characters.
func validateLBName(name string) error {
	if err := validateResourceName(name, "load balancer"); err != nil {
		return err
	}

	if len(name) < minLBNameLength {
		return fmt.Errorf(
			"%w: load balancer name must be at least 2 characters",
			ErrInvalidParameter,
		)
	}

	for _, c := range name {
		if c == '_' {
			return fmt.Errorf(
				"%w: load balancer name cannot contain underscores",
				ErrInvalidParameter,
			)
		}
	}

	return nil
}

// canonicalHostedZoneIDForLB returns the canonical hosted-zone ID for the given LB type and region.
// Returns a sensible default when the combination is not in the known map.
func canonicalHostedZoneIDForLB(lbType, region string) string {
	// ALB hosted zone IDs per region (partial list — falls back to us-east-1).
	albZones := map[string]string{
		"us-east-1":      "Z35SXDOTRQ7X7K",
		"us-east-2":      "Z3AADJGX6KTTL2",
		"us-west-1":      "Z368ELLRRE2KJ0",
		"us-west-2":      "Z1H1FL5HABSF5",
		"eu-west-1":      "Z32O12XQLNTSW2",
		"eu-west-2":      "ZHURV8PSTC4K8",
		"eu-central-1":   "Z215JYRZR1TBD5",
		"ap-southeast-1": "Z1LMS91P8CMLE5",
		"ap-southeast-2": "Z1GM3OXH4ZPM65",
		"ap-northeast-1": "Z14GRHDCWA56QT",
		"ap-south-1":     "ZP97RAFLXTNZK",
		"sa-east-1":      "ZTK26PT1VY4CU",
		"ca-central-1":   "ZQSVJUPU6J1EY",
	}
	// NLB hosted zone IDs per region.
	nlbZones := map[string]string{
		"us-east-1":      "Z26RNL4JYFTOTI",
		"us-east-2":      "ZLMOA37VPKANP",
		"us-west-1":      "Z24FKFUX50B4VW",
		"us-west-2":      "Z18D5FSROUN65G",
		"eu-west-1":      "Z2IFOLAFXWLO4F",
		"eu-west-2":      "ZD4D7Y8KGAS4G",
		"eu-central-1":   "Z3F0SRJ5LGBH90",
		"ap-southeast-1": "ZKVM6ETL9YU8P",
		"ap-southeast-2": "ZCT6FZBF4DROD",
		"ap-northeast-1": "Z31USIVHYNEOWT",
		"ap-south-1":     "ZVDDRBQ08TROA",
		"sa-east-1":      "ZTK26PT1VY4CU",
		"ca-central-1":   "Z2EPGBWURTVAEP",
	}

	switch lbType {
	case lbTypeNetwork:
		if id, ok := nlbZones[region]; ok {
			return id
		}

		return "Z26RNL4JYFTOTI"
	default:
		if id, ok := albZones[region]; ok {
			return id
		}

		return "Z35SXDOTRQ7X7K"
	}
}

// lbDNSName returns the DNS name for a load balancer following the real AWS format.
// ALB/GWLB: {name}-{id}.{region}.elb.amazonaws.com
// NLB:      {name}-{id}.elb.{region}.amazonaws.com.
func lbDNSName(name, lbType, region string) string {
	const fixedID = "00000001"
	switch lbType {
	case lbTypeNetwork:
		return fmt.Sprintf("%s-%s.elb.%s.amazonaws.com", name, fixedID, region)
	default:
		return fmt.Sprintf("%s-%s.%s.elb.amazonaws.com", name, fixedID, region)
	}
}

func (b *InMemoryBackend) lbARN(name string) string {
	return arn.Build(
		"elasticloadbalancing",
		b.region,
		b.accountID,
		"loadbalancer/app/"+name+"/0123456789abcdef",
	)
}

// subnetMappingsToAZs converts SubnetMapping slices into AvailabilityZone structs.
// When only plain subnet IDs are given (no rich mappings), each is wrapped in a SubnetMapping first.
// Zone names are synthesised from the region + index since we have no real VPC service.
func subnetMappingsToAZs(region string, mappings []SubnetMapping) []AvailabilityZone {
	azLetters := "abcdef"
	azs := make([]AvailabilityZone, 0, len(mappings))

	for i, m := range mappings {
		zoneName := region + string(azLetters[i%len(azLetters)])
		azs = append(azs, AvailabilityZone{ZoneName: zoneName, SubnetID: m.SubnetID})
	}

	return azs
}

// subnetsToMappings converts plain subnet ID strings into SubnetMapping values.
func subnetsToMappings(subnets []string) []SubnetMapping {
	out := make([]SubnetMapping, len(subnets))
	for i, s := range subnets {
		out[i] = SubnetMapping{SubnetID: s}
	}

	return out
}

// validateNetworkRefs checks CreateLoadBalancer's SecurityGroups and subnet mappings
// against the wired EC2Resolver. Callers must hold b.mu. A nil ec2Resolver (the
// default) accepts every security-group/subnet id unvalidated.
func (b *InMemoryBackend) validateNetworkRefs(sgs []string, mappings []SubnetMapping) error {
	if b.ec2Resolver == nil {
		return nil
	}

	for _, sg := range sgs {
		if !b.ec2Resolver.SecurityGroupExists(sg) {
			return fmt.Errorf("%w: %s", ErrInvalidSecurityGroup, sg)
		}
	}

	for _, m := range mappings {
		if !b.ec2Resolver.SubnetExists(m.SubnetID) {
			return fmt.Errorf("%w: %s", ErrSubnetNotFound, m.SubnetID)
		}
	}

	return nil
}

// CreateLoadBalancer creates a new load balancer.
func (b *InMemoryBackend) CreateLoadBalancer(input CreateLoadBalancerInput) (*LoadBalancer, error) {
	b.mu.Lock("CreateLoadBalancer")
	defer b.mu.Unlock()

	if input.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrInvalidParameter)
	}

	if err := validateLBName(input.Name); err != nil {
		return nil, err
	}

	for _, lb := range b.loadBalancers.All() {
		if lb.LoadBalancerName == input.Name {
			return nil, ErrLoadBalancerAlreadyExists
		}
	}

	lbArn := b.lbARN(input.Name)

	lbType := input.Type
	switch lbType {
	case "", lbTypeApplication:
		lbType = lbTypeApplication
	case "network", "gateway":
		// valid as-is
	default:
		return nil, fmt.Errorf(
			"%w: invalid Type %q; must be application, network, or gateway",
			ErrInvalidParameter, lbType,
		)
	}

	scheme := input.Scheme
	if scheme == "" {
		scheme = "internet-facing"
	}

	ipType := input.IPAddressType
	if ipType == "" {
		ipType = ipAddressTypeIPv4
	}

	t := tags.New("elbv2.lb." + input.Name + ".tags")
	for _, kv := range input.Tags {
		t.Set(kv.Key, kv.Value)
	}

	var defaultAttrs map[string]string
	switch lbType {
	case lbTypeNetwork, lbTypeGateway:
		defaultAttrs = map[string]string{
			attrAccessLogsS3Enabled:           attrValueFalse,
			attrDeletionProtectionEnabled:     attrValueFalse,
			attrCrossZoneLoadBalancingEnabled: attrValueFalse,
		}
	default: // lbTypeApplication
		defaultAttrs = albDefaultAttributes()
	}

	mappings := input.SubnetMappings
	if len(mappings) == 0 {
		mappings = subnetsToMappings(input.Subnets)
	}

	if err := b.validateNetworkRefs(input.SecurityGroups, mappings); err != nil {
		return nil, err
	}

	azs := subnetMappingsToAZs(b.region, mappings)

	lb := &LoadBalancer{
		LoadBalancerArn:       lbArn,
		LoadBalancerName:      input.Name,
		DNSName:               lbDNSName(input.Name, lbType, b.region),
		CanonicalHostedZoneID: canonicalHostedZoneIDForLB(lbType, b.region),
		CreatedTime:           time.Now().UTC(),
		Scheme:                scheme,
		Type:                  lbType,
		IPAddressType:         ipType,
		VpcID:                 "vpc-00000000",
		AvailabilityZones:     azs,
		SecurityGroups:        input.SecurityGroups,
		State: LoadBalancerState{
			Code:        "active",
			Description: "",
		},
		Attributes: defaultAttrs,
		Tags:       t,
	}

	b.loadBalancers.Put(lb)

	cp := *lb

	return &cp, nil
}

// checkAllArnsFound returns ErrLoadBalancerNotFound if any of the queried ARNs are absent from result.
func checkAllArnsFound(arns []string, result []LoadBalancer) error {
	for _, a := range arns {
		found := false
		for _, lb := range result {
			if lb.LoadBalancerArn == a {
				found = true

				break
			}
		}

		if !found {
			return ErrLoadBalancerNotFound
		}
	}

	return nil
}

// checkAllLBNamesFound returns ErrLoadBalancerNotFound if any of the queried names are absent from result.
func checkAllLBNamesFound(names []string, result []LoadBalancer) error {
	for _, n := range names {
		found := false
		for _, lb := range result {
			if lb.LoadBalancerName == n {
				found = true

				break
			}
		}

		if !found {
			return ErrLoadBalancerNotFound
		}
	}

	return nil
}

// DescribeLoadBalancers returns load balancers filtered by ARNs and/or names.
// The returned LoadBalancer values contain a Tags pointer that is backend-owned; callers must treat it as read-only.
//
// Fast path: when only ARNs are supplied (no names), look them up directly in
// the ARN-keyed map instead of scanning every load balancer in the backend.
func (b *InMemoryBackend) DescribeLoadBalancers(
	arns []string,
	names []string,
) ([]LoadBalancer, error) {
	b.mu.RLock("DescribeLoadBalancers")
	defer b.mu.RUnlock()

	if len(arns) > 0 && len(names) == 0 {
		result := make([]LoadBalancer, 0, len(arns))

		for _, a := range arns {
			if lb, ok := b.loadBalancers.Get(a); ok {
				result = append(result, *lb)
			}
		}

		sortLoadBalancersByName(result)

		if err := checkAllArnsFound(arns, result); err != nil {
			return nil, err
		}

		return result, nil
	}

	result := b.filterLoadBalancersLocked(arns, names)
	sortLoadBalancersByName(result)

	if len(arns) > 0 {
		if err := checkAllArnsFound(arns, result); err != nil {
			return nil, err
		}
	}

	if len(names) > 0 {
		if err := checkAllLBNamesFound(names, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// filterLoadBalancersLocked scans all load balancers and returns those whose
// ARN is in arns (when non-empty) and whose name is in names (when non-empty).
// Caller must hold b.mu (read or write).
func (b *InMemoryBackend) filterLoadBalancersLocked(arns, names []string) []LoadBalancer {
	arnSet := make(map[string]bool, len(arns))
	for _, a := range arns {
		arnSet[a] = true
	}

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	result := make([]LoadBalancer, 0, b.loadBalancers.Len())

	for _, lb := range b.loadBalancers.All() {
		if len(arns) > 0 && !arnSet[lb.LoadBalancerArn] {
			continue
		}

		if len(names) > 0 && !nameSet[lb.LoadBalancerName] {
			continue
		}

		result = append(result, *lb)
	}

	return result
}

// sortLoadBalancersByName sorts load balancers by name in ascending order.
func sortLoadBalancersByName(result []LoadBalancer) {
	sort.Slice(result, func(i, j int) bool {
		return result[i].LoadBalancerName < result[j].LoadBalancerName
	})
}

// DeleteLoadBalancer deletes a load balancer by ARN.
func (b *InMemoryBackend) DeleteLoadBalancer(lbArn string) error {
	b.mu.Lock("DeleteLoadBalancer")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(lbArn)
	if !ok {
		return ErrLoadBalancerNotFound
	}

	if lb.Attributes[attrDeletionProtectionEnabled] == attrValueTrue {
		return fmt.Errorf(
			"%w: load balancer cannot be deleted because deletion protection is enabled",
			ErrOperationNotPermitted,
		)
	}

	// Cascade: delete all listeners and their rules. The index lookups are
	// copied into fresh slices first because Table.Delete mutates the very
	// index groups Index.Get returns; iterating the live group while deleting
	// from it would corrupt the in-progress scan.
	listenersToDelete := append([]*Listener(nil), b.listenersByLB.Get(lbArn)...)
	for _, l := range listenersToDelete {
		rulesToDelete := append([]*Rule(nil), b.rulesByListener.Get(l.ListenerArn)...)
		for _, r := range rulesToDelete {
			r.Tags.Close()
			b.rules.Delete(r.RuleArn)
		}

		l.Tags.Close()
		b.listeners.Delete(l.ListenerArn)
	}

	lb.Tags.Close()
	delete(b.resourcePolicies, lbArn)
	b.loadBalancers.Delete(lbArn)

	return nil
}

// ModifyLoadBalancerAttributes updates attributes on a load balancer.
func (b *InMemoryBackend) ModifyLoadBalancerAttributes(
	lbArn string,
	attrs map[string]string,
) (*LoadBalancer, error) {
	b.mu.Lock("ModifyLoadBalancerAttributes")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(lbArn)
	if !ok {
		return nil, ErrLoadBalancerNotFound
	}

	if lb.Attributes == nil {
		lb.Attributes = make(map[string]string)
	}

	maps.Copy(lb.Attributes, attrs)

	cp := *lb

	return &cp, nil
}

// SetSecurityGroups updates the security groups associated with a load balancer.
func (b *InMemoryBackend) SetSecurityGroups(lbArn string, sgs []string) (*LoadBalancer, error) {
	b.mu.Lock("SetSecurityGroups")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(lbArn)
	if !ok {
		return nil, ErrLoadBalancerNotFound
	}

	if lb.Type != lbTypeApplication {
		return nil, fmt.Errorf(
			"%w: security groups cannot be associated with Network or Gateway Load Balancers",
			ErrInvalidConfigurationRequest,
		)
	}

	lb.SecurityGroups = sgs
	cp := *lb

	return &cp, nil
}

// SetSubnets updates the availability zones / subnets associated with a load balancer.
func (b *InMemoryBackend) SetSubnets(
	lbArn string,
	mappings []SubnetMapping,
) (*LoadBalancer, error) {
	b.mu.Lock("SetSubnets")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(lbArn)
	if !ok {
		return nil, ErrLoadBalancerNotFound
	}

	lb.AvailabilityZones = subnetMappingsToAZs(b.region, mappings)
	cp := *lb

	return &cp, nil
}

// SetIPAddressType updates the IP address type of a load balancer.
func (b *InMemoryBackend) SetIPAddressType(lbArn string, ipType string) (*LoadBalancer, error) {
	b.mu.Lock("SetIPAddressType")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(lbArn)
	if !ok {
		return nil, ErrLoadBalancerNotFound
	}

	switch ipType {
	case ipAddressTypeIPv4, "dualstack", "dualstack-without-public-ipv4":
		// valid
	default:
		return nil, fmt.Errorf(
			"%w: invalid IpAddressType %q; must be ipv4, dualstack, or dualstack-without-public-ipv4",
			ErrInvalidParameter,
			ipType,
		)
	}

	lb.IPAddressType = ipType
	cp := *lb

	return &cp, nil
}

// ModifyIPPools updates the IPAM pool configuration on a load balancer.
func (b *InMemoryBackend) ModifyIPPools(
	lbArn string, ipv4PoolID *string, removeIPv4 bool,
) (*LoadBalancer, error) {
	b.mu.Lock("ModifyIPPools")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(lbArn)
	if !ok {
		return nil, ErrLoadBalancerNotFound
	}

	if removeIPv4 {
		lb.IPv4IPAMPoolID = ""
	}

	if ipv4PoolID != nil {
		lb.IPv4IPAMPoolID = *ipv4PoolID
	}

	cp := *lb

	return &cp, nil
}
