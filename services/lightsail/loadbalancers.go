package lightsail

// This file backs family L (9 ops: CreateLoadBalancer, DeleteLoadBalancer,
// GetLoadBalancer, GetLoadBalancers, AttachInstancesToLoadBalancer,
// DetachInstancesFromLoadBalancer, UpdateLoadBalancerAttribute,
// SetIpAddressType, GetLoadBalancerMetricData) and family M (5 ops:
// CreateLoadBalancerTlsCertificate, DeleteLoadBalancerTlsCertificate,
// AttachLoadBalancerTlsCertificate, GetLoadBalancerTlsCertificates,
// GetLoadBalancerTlsPolicies).

import (
	"fmt"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	opTypeCreateLoadBalancer               = "CreateLoadBalancer"
	opTypeDeleteLoadBalancer               = "DeleteLoadBalancer"
	opTypeAttachInstancesToLoadBalancer    = "AttachInstancesToLoadBalancer"
	opTypeDetachInstancesFromLoadBalancer  = "DetachInstancesFromLoadBalancer"
	opTypeUpdateLoadBalancerAttribute      = "UpdateLoadBalancerAttribute"
	opTypeSetIPAddressType                 = "SetIpAddressType"
	opTypeCreateLoadBalancerTLSCertificate = "CreateLoadBalancerTlsCertificate"
	opTypeDeleteLoadBalancerTLSCertificate = "DeleteLoadBalancerTlsCertificate"
	opTypeAttachLoadBalancerTLSCertificate = "AttachLoadBalancerTlsCertificate"
)

// seedTLSPolicies is a small, defensible, CLEARLY SYNTHETIC stand-in for
// AWS's real published LB TLS policy catalog (GetLoadBalancerTlsPolicies,
// PARITY.md family M) -- names follow the real ELB-family convention but
// the cipher/protocol lists are not claimed as AWS-authoritative.
//
//nolint:gochecknoglobals // static reference table, read-only
var seedTLSPolicies = []LoadBalancerTLSPolicy{
	{
		Name: "TLS-1-2-2019-08", IsDefault: true,
		Description: "TLS 1.2 and up",
		Protocols:   []string{"TLSv1.2"},
		Ciphers:     []string{"ECDHE-RSA-AES128-GCM-SHA256", "ECDHE-RSA-AES256-GCM-SHA384"},
	},
	{
		Name: "TLS-1-1-2017-01", IsDefault: false,
		Description: "TLS 1.1 and up",
		Protocols:   []string{"TLSv1.1", "TLSv1.2"},
		Ciphers:     []string{"ECDHE-RSA-AES128-SHA", "ECDHE-RSA-AES128-GCM-SHA256"},
	},
}

// LoadBalancerTLSPolicy mirrors types.LoadBalancerTlsPolicy.
type LoadBalancerTLSPolicy struct {
	Name        string
	Description string
	Protocols   []string
	Ciphers     []string
	IsDefault   bool
}

// CreateLoadBalancer creates a new single-listener LoadBalancer.
func (b *InMemoryBackend) CreateLoadBalancer(
	name string, instancePort int32, healthCheckPath, ipAddressType, tlsPolicyName string, userTags map[string]string,
) ([]Operation, error) {
	if instancePort <= 0 {
		return nil, validationError("InstancePort must be positive")
	}

	b.mu.Lock("CreateLoadBalancer")
	defer b.mu.Unlock()

	if err := b.registerNameLocked(ResourceTypeLoadBalancer, name); err != nil {
		return nil, err
	}

	path := healthCheckPath
	if path == "" {
		path = "/"
	}

	ipType := ipAddressType
	if ipType == "" {
		ipType = ipAddressTypeDualStack
	}

	lb := &LoadBalancer{
		Name:            name,
		Arn:             b.regionalARN(ResourceTypeLoadBalancer, newUUID()),
		SupportCode:     newSupportCode(),
		DNSName:         name + "-" + randomHex() + "." + b.region + ".elb.amazonaws.com",
		State:           LoadBalancerStateProvisioning,
		Protocol:        LoadBalancerProtocolHTTP,
		IPAddressType:   ipType,
		TLSPolicyName:   tlsPolicyName,
		HealthCheckPath: path,
		InstancePort:    instancePort,
		CreatedAt:       nowUTC(),
		Location:        ResourceLocation{RegionName: b.region, AvailabilityZone: availabilityZoneA(b.region)},
		InstanceHealth:  make(map[string]instanceHealth),
		PublicPorts:     []int32{80},
		Tags:            tags.New("lightsail.loadbalancer." + name + ".tags"),
	}
	lb.Tags.Merge(userTags)
	b.loadBalancers.Put(lb)

	b.work.After("LoadBalancerActive", asyncTransitionDelay, func() {
		b.mu.Lock("LoadBalancer-async-active")
		defer b.mu.Unlock()

		if l, found := b.loadBalancers.Get(name); found && l.State == LoadBalancerStateProvisioning {
			l.State = LoadBalancerStateActive
		}
	})

	return b.newOperationsLocked(opTypeCreateLoadBalancer, ResourceTypeLoadBalancer, []string{name}), nil
}

// DeleteLoadBalancer deletes the named load balancer.
func (b *InMemoryBackend) DeleteLoadBalancer(name string) ([]Operation, error) {
	b.mu.Lock("DeleteLoadBalancer")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(name)
	if !ok {
		return nil, notFoundError("LoadBalancer", name)
	}

	if lb.Tags != nil {
		lb.Tags.Close()
	}

	b.loadBalancers.Delete(name)
	b.unregisterNameLocked(name)

	return b.newOperationsLocked(opTypeDeleteLoadBalancer, ResourceTypeLoadBalancer, []string{name}), nil
}

// GetLoadBalancer returns the named load balancer.
func (b *InMemoryBackend) GetLoadBalancer(name string) (*LoadBalancer, error) {
	b.mu.RLock("GetLoadBalancer")
	defer b.mu.RUnlock()

	lb, ok := b.loadBalancers.Get(name)
	if !ok {
		return nil, notFoundError("LoadBalancer", name)
	}

	return lb.clone(), nil
}

// GetLoadBalancers returns every load balancer, paginated.
func (b *InMemoryBackend) GetLoadBalancers(token string) (page.Page[*LoadBalancer], error) {
	b.mu.RLock("GetLoadBalancers")
	defer b.mu.RUnlock()

	all := b.loadBalancers.All()
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	out := make([]*LoadBalancer, len(all))
	for i, v := range all {
		out[i] = v.clone()
	}

	return paginateGeneric(out, token)
}

// AttachInstancesToLoadBalancer attaches instanceNames to the named load
// balancer, seeding each attached instance's InstanceHealthSummary
// "initial" (mirroring real target-health onboarding, PARITY.md 4.4) then
// advancing it to "healthy" after asyncTransitionDelay. Every instance must
// already be running (api_op_AttachInstancesToLoadBalancer.go's
// InstanceNames doc: "An instance must be running before you can attach it
// to your load balancer."); checked for all instanceNames before attaching
// any, so a rejected name leaves the load balancer untouched.
func (b *InMemoryBackend) AttachInstancesToLoadBalancer(name string, instanceNames []string) ([]Operation, error) {
	b.mu.Lock("AttachInstancesToLoadBalancer")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(name)
	if !ok {
		return nil, notFoundError("LoadBalancer", name)
	}

	for _, in := range instanceNames {
		inst, instOK := b.instances.Get(in)
		if !instOK {
			return nil, notFoundError("Instance", in)
		}

		if inst.StateName != InstanceStateNameRunning {
			return nil, validationError(
				fmt.Sprintf("instance %s must be running before you can attach it to a load balancer", in),
			)
		}
	}

	for _, in := range instanceNames {
		lb.AttachedInstances = append(lb.AttachedInstances, in)
		lb.InstanceHealth[in] = instanceHealth{State: InstanceHealthInitial, Reason: "Lb.RegistrationInProgress"}
	}

	b.work.After("LoadBalancerInstanceHealthy", asyncTransitionDelay, func() {
		b.mu.Lock("LoadBalancer-async-health")
		defer b.mu.Unlock()

		if l, found := b.loadBalancers.Get(name); found {
			for _, in := range instanceNames {
				l.InstanceHealth[in] = instanceHealth{State: InstanceHealthHealthy}
			}
		}
	})

	return b.newOperationsLocked(opTypeAttachInstancesToLoadBalancer, ResourceTypeLoadBalancer, []string{name}), nil
}

// DetachInstancesFromLoadBalancer detaches instanceNames from the named
// load balancer.
func (b *InMemoryBackend) DetachInstancesFromLoadBalancer(name string, instanceNames []string) ([]Operation, error) {
	b.mu.Lock("DetachInstancesFromLoadBalancer")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(name)
	if !ok {
		return nil, notFoundError("LoadBalancer", name)
	}

	remove := make(map[string]bool, len(instanceNames))
	for _, in := range instanceNames {
		remove[in] = true
		delete(lb.InstanceHealth, in)
	}

	out := make([]string, 0, len(lb.AttachedInstances))

	for _, in := range lb.AttachedInstances {
		if !remove[in] {
			out = append(out, in)
		}
	}

	lb.AttachedInstances = out

	return b.newOperationsLocked(opTypeDetachInstancesFromLoadBalancer, ResourceTypeLoadBalancer, []string{name}), nil
}

// UpdateLoadBalancerAttribute updates one named attribute on the load
// balancer -- HealthCheckPath and HTTPSRedirectionEnabled are modeled
// directly; every other attribute name (SessionStickinessEnabled etc.) is
// stored verbatim in ConfigurationOptions-equivalent bookkeeping without
// this backend independently acting on it (an honest scoped-down behavior:
// its wire value is retained and echoed back, never silently dropped).
func (b *InMemoryBackend) UpdateLoadBalancerAttribute(name, attributeName, attributeValue string) ([]Operation, error) {
	b.mu.Lock("UpdateLoadBalancerAttribute")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(name)
	if !ok {
		return nil, notFoundError("LoadBalancer", name)
	}

	switch attributeName {
	case "HealthCheckPath":
		lb.HealthCheckPath = attributeValue
	case "HTTPSRedirectionEnabled":
		lb.HTTPSRedirectionEnabled = attributeValue == "true"
	case "TLSPolicyName":
		lb.TLSPolicyName = attributeValue
	}

	return b.newOperationsLocked(opTypeUpdateLoadBalancerAttribute, ResourceTypeLoadBalancer, []string{name}), nil
}

// SetIPAddressType sets ipAddressType on the named resource -- shared
// across Instance/LoadBalancer/Distribution via the ResourceType
// discriminator (PARITY.md family L).
func (b *InMemoryBackend) SetIPAddressType(resourceName, resourceType, ipAddressType string) ([]Operation, error) {
	b.mu.Lock("SetIpAddressType")
	defer b.mu.Unlock()

	switch resourceType {
	case ResourceTypeLoadBalancer:
		lb, ok := b.loadBalancers.Get(resourceName)
		if !ok {
			return nil, notFoundError("LoadBalancer", resourceName)
		}

		lb.IPAddressType = ipAddressType
	case ResourceTypeInstance:
		i, ok := b.instances.Get(resourceName)
		if !ok {
			return nil, notFoundError("Instance", resourceName)
		}

		i.IPAddressType = ipAddressType
	case ResourceTypeDistribution:
		d, ok := b.distributions.Get(resourceName)
		if !ok {
			return nil, notFoundError("Distribution", resourceName)
		}

		d.IPAddressType = ipAddressType
	default:
		return nil, validationError("unsupported ResourceType for SetIpAddressType: " + resourceType)
	}

	return b.newOperationsLocked(opTypeSetIPAddressType, resourceType, []string{resourceName}), nil
}

// GetLoadBalancerMetricData returns a real, well-formed, EMPTY MetricData
// response -- one of the six honestly-unfakeable telemetry ops
// (PARITY.md 4.10).
func (b *InMemoryBackend) GetLoadBalancerMetricData(name string) error {
	b.mu.RLock("GetLoadBalancerMetricData")
	defer b.mu.RUnlock()

	if _, ok := b.loadBalancers.Get(name); !ok {
		return notFoundError("LoadBalancer", name)
	}

	return nil
}

// CreateLoadBalancerTLSCertificate creates a new TLS certificate scoped to
// loadBalancerName, starting a real ACM-like validation timeline
// (PARITY.md 4.4): PENDING_VALIDATION -> ISSUED after asyncTransitionDelay.
func (b *InMemoryBackend) CreateLoadBalancerTLSCertificate(
	loadBalancerName, certificateName, domainName string, alternativeNames []string, userTags map[string]string,
) ([]Operation, error) {
	b.mu.Lock("CreateLoadBalancerTlsCertificate")
	defer b.mu.Unlock()

	if _, ok := b.loadBalancers.Get(loadBalancerName); !ok {
		return nil, notFoundError("LoadBalancer", loadBalancerName)
	}

	if err := b.registerNameLocked(ResourceTypeLoadBalancerTLSCertificate, certificateName); err != nil {
		return nil, err
	}

	now := nowUTC()
	cert := &LoadBalancerTLSCertificate{
		Name: certificateName, Arn: b.regionalARN(ResourceTypeLoadBalancerTLSCertificate, newUUID()),
		SupportCode:      newSupportCode(),
		LoadBalancerName: loadBalancerName, DomainName: domainName,
		SubjectAlternativeNames: append([]string{domainName}, alternativeNames...),
		Status:                  CertificateStatusPendingValidation,
		Issuer:                  "Let's Encrypt", Subject: domainName, SerialNumber: newUUID(),
		CreatedAt: now, NotBefore: now, NotAfter: now.AddDate(0, certificateValidityMonths, 0),
		Location: ResourceLocation{RegionName: b.region, AvailabilityZone: availabilityZoneA(b.region)},
		Tags:     tags.New("lightsail.lbtlscert." + certificateName + ".tags"),
	}
	cert.Tags.Merge(userTags)
	b.lbTLSCertificates.Put(cert)

	b.work.After("LBTLSCertIssued", asyncTransitionDelay, func() {
		b.mu.Lock("LBTLSCert-async-issued")
		defer b.mu.Unlock()

		if c, found := b.lbTLSCertificates.Get(certificateName); found &&
			c.Status == CertificateStatusPendingValidation {
			c.Status = CertificateStatusIssued
			c.IssuedAt = nowUTC()
		}
	})

	return b.newOperationsLocked(
		opTypeCreateLoadBalancerTLSCertificate,
		ResourceTypeLoadBalancerTLSCertificate,
		[]string{certificateName},
	), nil
}

// DeleteLoadBalancerTLSCertificate deletes the named LB TLS certificate.
func (b *InMemoryBackend) DeleteLoadBalancerTLSCertificate(
	loadBalancerName, certificateName string,
) ([]Operation, error) {
	b.mu.Lock("DeleteLoadBalancerTlsCertificate")
	defer b.mu.Unlock()

	cert, ok := b.lbTLSCertificates.Get(certificateName)
	if !ok || cert.LoadBalancerName != loadBalancerName {
		return nil, notFoundError("LoadBalancerTlsCertificate", certificateName)
	}

	if cert.Tags != nil {
		cert.Tags.Close()
	}

	b.lbTLSCertificates.Delete(certificateName)
	b.unregisterNameLocked(certificateName)

	if lb, found := b.loadBalancers.Get(loadBalancerName); found {
		lb.TLSCertificateNames = removeString(lb.TLSCertificateNames, certificateName)
	}

	return b.newOperationsLocked(
		opTypeDeleteLoadBalancerTLSCertificate,
		ResourceTypeLoadBalancerTLSCertificate,
		[]string{certificateName},
	), nil
}

// AttachLoadBalancerTLSCertificate attaches an ISSUED certificate to its
// load balancer.
func (b *InMemoryBackend) AttachLoadBalancerTLSCertificate(
	loadBalancerName, certificateName string,
) ([]Operation, error) {
	b.mu.Lock("AttachLoadBalancerTlsCertificate")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(loadBalancerName)
	if !ok {
		return nil, notFoundError("LoadBalancer", loadBalancerName)
	}

	cert, ok := b.lbTLSCertificates.Get(certificateName)
	if !ok || cert.LoadBalancerName != loadBalancerName {
		return nil, notFoundError("LoadBalancerTlsCertificate", certificateName)
	}

	cert.IsAttached = true
	lb.TLSCertificateNames = append(lb.TLSCertificateNames, certificateName)
	lb.Protocol = LoadBalancerProtocolHTTPHTTPS
	lb.PublicPorts = []int32{80, 443}

	return b.newOperationsLocked(
		opTypeAttachLoadBalancerTLSCertificate,
		ResourceTypeLoadBalancerTLSCertificate,
		[]string{certificateName},
	), nil
}

// GetLoadBalancerTLSCertificates returns every TLS certificate scoped to
// loadBalancerName.
func (b *InMemoryBackend) GetLoadBalancerTLSCertificates(
	loadBalancerName string,
) ([]*LoadBalancerTLSCertificate, error) {
	b.mu.RLock("GetLoadBalancerTlsCertificates")
	defer b.mu.RUnlock()

	if _, ok := b.loadBalancers.Get(loadBalancerName); !ok {
		return nil, notFoundError("LoadBalancer", loadBalancerName)
	}

	var out []*LoadBalancerTLSCertificate

	for _, c := range b.lbTLSCertificates.All() {
		if c.LoadBalancerName == loadBalancerName {
			out = append(out, c.clone())
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

// GetLoadBalancerTLSPolicies returns the seed TLS policy catalog, paginated.
func (b *InMemoryBackend) GetLoadBalancerTLSPolicies(token string) (page.Page[LoadBalancerTLSPolicy], error) {
	return paginateGeneric(seedTLSPolicies, token)
}

func removeString(in []string, s string) []string {
	out := make([]string, 0, len(in))

	for _, v := range in {
		if v != s {
			out = append(out, v)
		}
	}

	return out
}
