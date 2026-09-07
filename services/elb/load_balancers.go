package elb

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

var (
	// lbNameRe matches valid Classic ELB names: 1-32 chars, alphanumeric + hyphens,
	// must start and end with alphanumeric.
	lbNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-]{0,30}[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)
)

const (
	// dnsHashModLB is the modulus for generating deterministic DNS host IDs.
	dnsHashModLB = 10_000_000_000

	// dnsHashModHZ is the modulus for generating deterministic hosted zone IDs.
	dnsHashModHZ = 1_000_000_000
)

// regionHostedZoneIDs maps AWS region names to the Classic ELB hosted zone ID for that region.
var regionHostedZoneIDs = map[string]string{ //nolint:gochecknoglobals // immutable lookup table
	"us-east-1":      "Z35SXDOTRQ7X7K",
	"us-east-2":      "Z3AADJGX6KTTL2",
	"us-west-1":      "Z368ELLRRE2KJ0",
	"us-west-2":      "Z1H1FL5HABSF5",
	"eu-west-1":      "Z32O12XQLNTSW2",
	"eu-west-2":      "ZHURV8PSTC4K8",
	"eu-central-1":   "Z215JYRZR1TBD5",
	"ap-northeast-1": "Z14GRHDCWA56QT",
	"ap-southeast-1": "Z1LMS91P8CMLE5",
	"ap-southeast-2": "Z1GM3OXH4ZPM65",
}

// dnsNameSuffix returns a stable 10-digit numeric suffix for an ELB DNS name,
// derived from a hash of the account ID and LB name.
func dnsNameSuffix(accountID, lbName string) string {
	h := httputils.FNV64a(accountID + ":" + lbName)

	return fmt.Sprintf("%010d", h%dnsHashModLB)
}

// canonicalHostedZoneIDForRegion returns the Classic ELB hosted zone ID for the given region.
// Falls back to a synthetic ID for unknown regions.
func canonicalHostedZoneIDForRegion(region string) string {
	if id, ok := regionHostedZoneIDs[region]; ok {
		return id
	}

	h := httputils.FNV32a("elb-hzid:" + region)

	return fmt.Sprintf("Z%09d", h%dnsHashModHZ)
}

// lbCopy returns a deep copy of a LoadBalancer, excluding the Tags pointer (which is
// shared and safe for concurrent reads through its own sync primitives).
func lbCopy(lb *LoadBalancer) LoadBalancer {
	cp := *lb

	cp.Listeners = make([]Listener, len(lb.Listeners))
	for i, l := range lb.Listeners {
		lCopy := l
		if l.PolicyNames != nil {
			lCopy.PolicyNames = make([]string, len(l.PolicyNames))
			copy(lCopy.PolicyNames, l.PolicyNames)
		}

		cp.Listeners[i] = lCopy
	}

	cp.Instances = make([]Instance, len(lb.Instances))
	copy(cp.Instances, lb.Instances)

	cp.AvailabilityZones = make([]string, len(lb.AvailabilityZones))
	copy(cp.AvailabilityZones, lb.AvailabilityZones)

	cp.SecurityGroups = make([]string, len(lb.SecurityGroups))
	copy(cp.SecurityGroups, lb.SecurityGroups)

	cp.Subnets = make([]string, len(lb.Subnets))
	copy(cp.Subnets, lb.Subnets)

	cp.BackendServerDescriptions = make([]BackendServerDescription, len(lb.BackendServerDescriptions))
	for i, bsd := range lb.BackendServerDescriptions {
		bsdCopy := bsd
		bsdCopy.PolicyNames = make([]string, len(bsd.PolicyNames))
		copy(bsdCopy.PolicyNames, bsd.PolicyNames)
		cp.BackendServerDescriptions[i] = bsdCopy
	}

	if lb.HealthCheck != nil {
		hc := *lb.HealthCheck
		cp.HealthCheck = &hc
	}

	return cp
}

// AddLoadBalancerInternal inserts a pre-built LoadBalancer for seeding test state.
// The lb is deep-copied on insertion. Tags is initialised if nil. Region
// defaults to the backend's configured region if unset, matching every other
// insertion path (e.g. CreateLoadBalancer) so the seeded LB is reachable via
// the default-region fallback in getRegion.
func (b *InMemoryBackend) AddLoadBalancerInternal(lb LoadBalancer) {
	b.mu.Lock("AddLoadBalancerInternal")
	defer b.mu.Unlock()

	if lb.Region == "" {
		lb.Region = b.region
	}

	if lb.Tags == nil {
		lb.Tags = tags.New("elb." + lb.LoadBalancerName)
	}

	if lb.Listeners == nil {
		lb.Listeners = []Listener{}
	}

	if lb.Instances == nil {
		lb.Instances = []Instance{}
	}

	if lb.BackendServerDescriptions == nil {
		lb.BackendServerDescriptions = []BackendServerDescription{}
	}

	if lb.AvailabilityZones == nil {
		lb.AvailabilityZones = []string{}
	}

	if lb.SecurityGroups == nil {
		lb.SecurityGroups = []string{}
	}

	if lb.Subnets == nil {
		lb.Subnets = []string{}
	}

	cp := lbCopy(&lb)
	b.lbs.Put(&cp)
}

// validateCreateLBName checks that the LB name is present and well-formed.
func validateCreateLBName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: LoadBalancerName is required", ErrInvalidParameter)
	}

	if !lbNameRe.MatchString(name) {
		return fmt.Errorf(
			"%w: LoadBalancerName must be 1-32 alphanumeric characters or hyphens, "+
				"starting and ending with alphanumeric",
			ErrInvalidParameter,
		)
	}

	return nil
}

// resolveScheme normalises the scheme parameter, returning an error for invalid values.
func resolveScheme(scheme string) (string, error) {
	if scheme == "" {
		scheme = "internet-facing"
	}

	if scheme != "internet-facing" && scheme != "internal" {
		return "", fmt.Errorf(
			"%w: Scheme must be 'internet-facing' or 'internal'",
			ErrInvalidScheme,
		)
	}

	return scheme, nil
}

// validateCreateLBZones checks listener, AZ, and subnet constraints.
func validateCreateLBZones(input CreateLoadBalancerInput) error {
	if len(input.Listeners) == 0 {
		return fmt.Errorf(
			"%w: at least one Listener is required to create a load balancer",
			ErrInvalidParameter,
		)
	}

	if len(input.AvailabilityZones) > 0 && len(input.Subnets) > 0 {
		return fmt.Errorf(
			"%w: AvailabilityZones and Subnets are mutually exclusive",
			ErrInvalidConfiguration,
		)
	}

	if len(input.AvailabilityZones) == 0 && len(input.Subnets) == 0 {
		return fmt.Errorf(
			"%w: at least one AvailabilityZone or Subnet is required",
			ErrInvalidParameter,
		)
	}

	return nil
}

// nonNilStrings returns src, or an empty (non-nil) slice when src is nil, so
// stored load balancers never carry nil slices.
func nonNilStrings(src []string) []string {
	if src == nil {
		return []string{}
	}

	return src
}

// deriveVPCID returns the synthetic VPC ID for a load balancer. VPC-mode load
// balancers (those with subnets) get a stable ID derived from the first 8
// characters of the account ID; EC2-Classic load balancers get an empty ID.
func (b *InMemoryBackend) deriveVPCID(subnets []string) string {
	const vpcSuffixLen = 8

	if len(subnets) == 0 {
		return ""
	}

	acctSuffix := b.accountID
	if len(acctSuffix) > vpcSuffixLen {
		acctSuffix = acctSuffix[:vpcSuffixLen]
	}

	return "vpc-" + acctSuffix
}

// CreateLoadBalancer creates a new Classic ELB load balancer in the caller's region.
func (b *InMemoryBackend) CreateLoadBalancer(
	ctx context.Context,
	input CreateLoadBalancerInput,
) (*LoadBalancer, error) {
	if err := validateCreateLBName(input.LoadBalancerName); err != nil {
		return nil, err
	}

	scheme, err := resolveScheme(input.Scheme)
	if err != nil {
		return nil, err
	}

	b.mu.Lock("CreateLoadBalancer")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if b.lbs.Has(lbTableKey(region, input.LoadBalancerName)) {
		return nil, fmt.Errorf("%w: %q", ErrLoadBalancerAlreadyExists, input.LoadBalancerName)
	}

	const maxLBs = 20
	if len(b.lbsByRegion.Get(region)) >= maxLBs {
		return nil, fmt.Errorf(
			"%w: classic-load-balancers limit of %d exceeded",
			ErrTooManyLoadBalancers, maxLBs,
		)
	}

	if zoneErr := validateCreateLBZones(input); zoneErr != nil {
		return nil, zoneErr
	}

	isVPC := len(input.Subnets) > 0

	suffix := dnsNameSuffix(b.accountID, input.LoadBalancerName)
	dnsPrefix := input.LoadBalancerName + "-" + suffix
	if scheme == "internal" {
		dnsPrefix = "internal-" + dnsPrefix
	}

	dnsName := dnsPrefix + "." + region + ".elb.amazonaws.com"
	lbARN := arn.Build("elasticloadbalancing", region, b.accountID, "loadbalancer/"+input.LoadBalancerName)

	// Ensure non-nil slices so callers never have to nil-check.
	azs := nonNilStrings(input.AvailabilityZones)
	sgs := nonNilStrings(input.SecurityGroups)
	subnets := nonNilStrings(input.Subnets)

	listeners := input.Listeners
	if listeners == nil {
		listeners = []Listener{}
	}

	if certErr := b.validateListenerCertificates(ctx, listeners); certErr != nil {
		return nil, certErr
	}

	vpcID := b.deriveVPCID(subnets)

	lb := &LoadBalancer{
		LoadBalancerName:          input.LoadBalancerName,
		ARN:                       lbARN,
		DNSName:                   dnsName,
		CanonicalHostedZoneName:   dnsName,
		CanonicalHostedZoneNameID: canonicalHostedZoneIDForRegion(region),
		CreatedTime:               time.Now(),
		Scheme:                    scheme,
		AvailabilityZones:         azs,
		SecurityGroups:            sgs,
		Subnets:                   subnets,
		VPCId:                     vpcID,
		Listeners:                 listeners,
		Instances:                 []Instance{},
		BackendServerDescriptions: []BackendServerDescription{},
		Tags:                      tags.New("elb." + input.LoadBalancerName),
		AccountID:                 b.accountID,
		Region:                    region,
		Attributes:                defaultLBAttributes(),
		IsVPC:                     isVPC,
	}

	b.lbs.Put(lb)

	cp := lbCopy(lb)

	return &cp, nil
}

// DeleteLoadBalancer removes a load balancer by name and all of its policies
// within the caller's region. Deleting a name that doesn't exist (or was
// already deleted) is a no-op success, not an error: DeleteLoadBalancer has
// no typed exception for it (deserializers.go's
// awsAwsquery_deserializeOpErrorDeleteLoadBalancer switch is empty besides
// UnknownError), and the SDK's own doc comment says so explicitly ("If the
// load balancer does not exist or has already been deleted, the call to
// DeleteLoadBalancer still succeeds").
func (b *InMemoryBackend) DeleteLoadBalancer(ctx context.Context, name string) error {
	b.mu.Lock("DeleteLoadBalancer")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	lb, ok := b.lbs.Get(lbTableKey(region, name))
	if !ok {
		return nil
	}

	lb.Tags.Close()
	b.lbs.Delete(lbTableKey(region, name))

	// Cascade-delete all policies that belong to this load balancer. The
	// index lookup is copied into a fresh slice first because Table.Delete
	// mutates the very index group policiesByLB.Get returns; iterating the
	// live group while deleting from it would corrupt the in-progress scan.
	policiesToDelete := slices.Clone(b.policiesByLB.Get(lbTableKey(region, name)))
	for _, p := range policiesToDelete {
		b.policies.Delete(policyTableKey(p.Region, p.LoadBalancerName, p.PolicyName))
	}

	return nil
}

// DescribeLoadBalancers returns load balancers in the caller's region,
// optionally filtered by name.
func (b *InMemoryBackend) DescribeLoadBalancers(ctx context.Context, names []string) ([]LoadBalancer, error) {
	b.mu.RLock("DescribeLoadBalancers")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	if len(names) > 0 {
		result := make([]LoadBalancer, 0, len(names))

		for _, name := range names {
			lb, ok := b.lbs.Get(lbTableKey(region, name))
			if !ok {
				return nil, fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
			}

			result = append(result, lbCopy(lb))
		}

		return result, nil
	}

	regionLBs := b.lbsByRegion.Get(region)
	result := make([]LoadBalancer, 0, len(regionLBs))
	for _, lb := range regionLBs {
		result = append(result, lbCopy(lb))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].LoadBalancerName < result[j].LoadBalancerName
	})

	return result, nil
}

// DescribeAccountLimits returns the current ELB account limits.
func (b *InMemoryBackend) DescribeAccountLimits(_ context.Context) ([]AccountLimit, error) {
	b.mu.RLock("DescribeAccountLimits")
	defer b.mu.RUnlock()

	return []AccountLimit{
		{Name: "classic-load-balancers", Max: "20"},
		{Name: "classic-listeners", Max: "100"},
		{Name: "classic-registered-instances", Max: "1000"},
	}, nil
}
