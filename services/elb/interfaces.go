package elb

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// StorageBackend is the interface for the ELB in-memory store.
//
// Every operation that touches per-load-balancer state takes a context.Context
// so the backend can resolve the caller's AWS region and route the operation to
// that region's isolated store. Region returns the backend's default region for
// callers (e.g. the HTTP handler) that need a fallback when the request omits a
// region.
type StorageBackend interface {
	Reset()
	Region() string

	CreateLoadBalancer(ctx context.Context, input CreateLoadBalancerInput) (*LoadBalancer, error)
	DeleteLoadBalancer(ctx context.Context, name string) error
	DescribeLoadBalancers(ctx context.Context, names []string) ([]LoadBalancer, error)

	CreateLoadBalancerListeners(ctx context.Context, name string, listeners []Listener) error
	DeleteLoadBalancerListeners(ctx context.Context, name string, ports []int32) error

	RegisterInstancesWithLoadBalancer(ctx context.Context, name string, instances []Instance) ([]Instance, error)
	DeregisterInstancesFromLoadBalancer(ctx context.Context, name string, instances []Instance) ([]Instance, error)

	ConfigureHealthCheck(ctx context.Context, name string, hc HealthCheck) (*HealthCheck, error)

	ModifyLoadBalancerAttributes(
		ctx context.Context, name string, attrs LoadBalancerAttributes, mask LoadBalancerAttributesMask,
	) (*LoadBalancerAttributes, error)
	DescribeLoadBalancerAttributes(ctx context.Context, name string) (*LoadBalancerAttributes, error)

	AddTags(ctx context.Context, names []string, kvs []tags.KV) error
	DescribeTags(ctx context.Context, names []string) (map[string][]tags.KV, error)
	RemoveTags(ctx context.Context, names []string, keys []string) error

	ApplySecurityGroupsToLoadBalancer(ctx context.Context, name string, securityGroups []string) ([]string, error)
	AttachLoadBalancerToSubnets(ctx context.Context, name string, subnets []string) ([]string, error)
	DetachLoadBalancerFromSubnets(ctx context.Context, name string, subnets []string) ([]string, error)
	EnableAvailabilityZonesForLoadBalancer(ctx context.Context, name string, azs []string) ([]string, error)
	DisableAvailabilityZonesForLoadBalancer(ctx context.Context, name string, azs []string) ([]string, error)
	SetLoadBalancerListenerSSLCertificate(ctx context.Context, name string, port int32, certID string) error
	SetLoadBalancerPoliciesOfListener(ctx context.Context, name string, port int32, policyNames []string) error
	SetLoadBalancerPoliciesForBackendServer(
		ctx context.Context, name string, instancePort int32, policyNames []string,
	) error

	CreateAppCookieStickinessPolicy(ctx context.Context, name, policyName, cookieName string) error
	CreateLBCookieStickinessPolicy(ctx context.Context, name, policyName string, cookieExpirationPeriod int64) error
	CreateLoadBalancerPolicy(
		ctx context.Context,
		name, policyName, policyTypeName string,
		attrs []PolicyAttribute,
	) error
	DeleteLoadBalancerPolicy(ctx context.Context, name, policyName string) error

	DescribeAccountLimits(ctx context.Context) ([]AccountLimit, error)
	DescribeInstanceHealth(ctx context.Context, name string, instances []Instance) ([]InstanceState, error)
	DescribeLoadBalancerPolicies(ctx context.Context, name string, policyNames []string) ([]LoadBalancerPolicy, error)
	DescribeLoadBalancerPolicyTypes(ctx context.Context, policyTypeNames []string) ([]PolicyTypeDescription, error)
}
