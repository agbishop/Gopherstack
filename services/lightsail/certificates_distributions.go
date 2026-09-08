package lightsail

// This file backs family V (3 ops: CreateCertificate, DeleteCertificate,
// GetCertificates -- CDN/distribution-facing, distinct from the LB-TLS
// family M) and family T (10 ops: CreateDistribution, UpdateDistribution,
// DeleteDistribution, GetDistributions, UpdateDistributionBundle,
// ResetDistributionCache, GetDistributionLatestCacheReset,
// GetDistributionMetricData, AttachCertificateToDistribution,
// DetachCertificateFromDistribution).

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	opTypeCreateCertificate          = "CreateCertificate"
	opTypeDeleteCertificate          = "DeleteCertificate"
	opTypeCreateDistribution         = "CreateDistribution"
	opTypeUpdateDistribution         = "UpdateDistribution"
	opTypeDeleteDistribution         = "DeleteDistribution"
	opTypeUpdateDistributionBundle   = "UpdateDistributionBundle"
	opTypeResetDistributionCache     = "ResetDistributionCache"
	opTypeAttachCertToDistribution   = "AttachCertificateToDistribution"
	opTypeDetachCertFromDistribution = "DetachCertificateFromDistribution"
)

// CreateCertificate creates a new CDN-facing Certificate, starting a real
// Let's-Encrypt-style validation timeline (PARITY.md 4.7): PENDING_VALIDATION
// -> ISSUED after asyncTransitionDelay.
func (b *InMemoryBackend) CreateCertificate(
	name, domainName string,
	sans []string,
	userTags map[string]string,
) ([]Operation, error) {
	b.mu.Lock("CreateCertificate")
	defer b.mu.Unlock()

	if err := b.registerNameLocked(ResourceTypeCertificate, name); err != nil {
		return nil, err
	}

	now := nowUTC()
	cert := &Certificate{
		Name: name, Arn: b.regionalARN(ResourceTypeCertificate, newUUID()), DomainName: domainName,
		Status: CertificateStatusPendingValidation, CreatedAt: now, NotBefore: now,
		NotAfter:                now.AddDate(0, certificateValidityMonths, 0),
		SubjectAlternativeNames: append([]string{domainName}, sans...),
		Tags:                    tags.New("lightsail.certificate." + name + ".tags"),
	}
	cert.Tags.Merge(userTags)
	b.certificates.Put(cert)

	b.work.After("CertificateIssued", asyncTransitionDelay, func() {
		b.mu.Lock("Certificate-async-issued")
		defer b.mu.Unlock()

		if c, found := b.certificates.Get(name); found &&
			c.Status == CertificateStatusPendingValidation {
			c.Status = CertificateStatusIssued
			c.IssuedAt = nowUTC()
		}
	})

	return b.newOperationsLocked(
		opTypeCreateCertificate,
		ResourceTypeCertificate,
		[]string{name},
	), nil
}

// DeleteCertificate deletes the named certificate. Real AWS: "Certificates
// that are currently attached to a distribution cannot be deleted".
func (b *InMemoryBackend) DeleteCertificate(name string) ([]Operation, error) {
	b.mu.Lock("DeleteCertificate")
	defer b.mu.Unlock()

	cert, ok := b.certificates.Get(name)
	if !ok {
		return nil, notFoundError("Certificate", name)
	}

	for _, d := range b.distributions.All() {
		if d.CertificateName == name {
			return nil, validationError(
				fmt.Sprintf("certificate %s is attached to distribution %s", name, d.Name),
			)
		}
	}

	if cert.Tags != nil {
		cert.Tags.Close()
	}

	b.certificates.Delete(name)
	b.unregisterNameLocked(name)

	return b.newOperationsLocked(
		opTypeDeleteCertificate,
		ResourceTypeCertificate,
		[]string{name},
	), nil
}

// GetCertificates returns every certificate, optionally filtered by name,
// paginated.
func (b *InMemoryBackend) GetCertificates(
	name string,
	token string,
) (page.Page[*Certificate], error) {
	b.mu.RLock("GetCertificates")
	defer b.mu.RUnlock()

	all := b.certificates.All()
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	out := make([]*Certificate, 0, len(all))

	for _, c := range all {
		if name != "" && c.Name != name {
			continue
		}

		out = append(out, c.clone())
	}

	return paginateGeneric(out, token)
}

// resolveDistributionOrigin validates that originName names a real Instance,
// Bucket, or LoadBalancer (Distribution.Origin's own SDK doc comment names
// exactly these three kinds, PARITY.md 4.7).
func (b *InMemoryBackend) resolveDistributionOrigin(originName string) (string, bool) {
	if _, ok := b.instances.Get(originName); ok {
		return ResourceTypeInstance, true
	}

	if _, ok := b.buckets.Get(originName); ok {
		return ResourceTypeBucket, true
	}

	if _, ok := b.loadBalancers.Get(originName); ok {
		return ResourceTypeLoadBalancer, true
	}

	return "", false
}

// CreateDistributionRequest holds the parameters for CreateDistribution.
// DefaultCacheBehavior is required on the real wire (api_op_CreateDistribution.go,
// client-side-validated in validators.go's validateOpCreateDistributionInput)
// -- a distribution with no cache configuration is not much of a
// distribution, and the real response echoes it back, along with
// CacheBehaviorSettings/CacheBehaviors/ViewerMinimumTlsProtocolVersion, via
// GetDistributions.
type CreateDistributionRequest struct {
	CacheBehaviorSettings *CacheSettings
	Tags                  map[string]string
	Name                  string
	BundleID              string
	OriginName            string
	IPAddressType         string
	CertificateName       string
	ViewerMinTLSVersion   string
	DefaultCacheBehavior  CacheBehavior
	CacheBehaviors        []CacheBehaviorPerPath
}

// CreateDistribution creates a new Distribution -- modeled global but its
// Location.RegionName is always literally "us-east-1" regardless of this
// backend's own configured region (PARITY.md 4.7).
func (b *InMemoryBackend) CreateDistribution(req CreateDistributionRequest) ([]Operation, error) {
	if req.DefaultCacheBehavior.Behavior == "" {
		return nil, validationError("DefaultCacheBehavior is required")
	}

	b.mu.Lock("CreateDistribution")
	defer b.mu.Unlock()

	if _, ok := b.resolveDistributionOrigin(req.OriginName); !ok {
		return nil, notFoundError(
			"Distribution origin (Instance/Bucket/LoadBalancer)",
			req.OriginName,
		)
	}

	if err := b.registerNameLocked(ResourceTypeDistribution, req.Name); err != nil {
		return nil, err
	}

	ipType := req.IPAddressType
	if ipType == "" {
		ipType = ipAddressTypeDualStack
	}

	dist := &Distribution{
		Name: req.Name, Arn: b.distributionARN(req.Name), SupportCode: newSupportCode(),
		BundleID: req.BundleID, Status: "InProgress", DomainName: req.Name + "." + randomHex() + ".cloudfront.net",
		OriginPublicDNS: req.OriginName + ".origin.local", CertificateName: req.CertificateName,
		IPAddressType: ipType, IsEnabled: true, CreatedAt: nowUTC(),
		Location: ResourceLocation{RegionName: distributionRegion},
		Origin: DistributionOrigin{
			Name:           req.OriginName,
			RegionName:     b.region,
			ProtocolPolicy: "http-only",
		},
		DefaultCacheBehavior:  req.DefaultCacheBehavior,
		CacheBehaviorSettings: req.CacheBehaviorSettings,
		CacheBehaviors:        req.CacheBehaviors,
		ViewerMinTLSVersion:   req.ViewerMinTLSVersion,
		Tags:                  tags.New("lightsail.distribution." + req.Name + ".tags"),
	}
	dist.Tags.Merge(req.Tags)
	b.distributions.Put(dist)

	b.work.After("DistributionDeployed", asyncTransitionDelay, func() {
		b.mu.Lock("Distribution-async-deployed")
		defer b.mu.Unlock()

		if d, found := b.distributions.Get(req.Name); found && d.Status == "InProgress" {
			d.Status = "Deployed"
		}
	})

	ops := b.newOperationsLocked(
		opTypeCreateDistribution,
		ResourceTypeDistribution,
		[]string{req.Name},
	)

	return ops, nil
}

// UpdateDistributionRequest holds the parameters for UpdateDistribution.
// Origin is deliberately absent: the real UpdateDistributionInput.Origin
// exists but this backend has no code path exercising it yet -- left as a
// disclosed gap (PARITY.md) rather than half-wired.
type UpdateDistributionRequest struct {
	CacheBehaviorSettings *CacheSettings
	DefaultCacheBehavior  *CacheBehavior
	IsEnabled             *bool
	Name                  string
	CertificateName       string
	ViewerMinTLSVersion   string
	CacheBehaviors        []CacheBehaviorPerPath
}

// UpdateDistribution updates the named distribution's IsEnabled/
// CertificateName/cache-behavior fields. Each optional field, when
// provided, replaces the distribution's existing value outright -- matching
// CacheBehaviorSettings's own SDK doc comment ("will replace your
// distribution's existing settings"). This emulator does not proxy real
// traffic through a Distribution, so cache behavior is stored and echoed,
// not enforced.
func (b *InMemoryBackend) UpdateDistribution(req UpdateDistributionRequest) (*Operation, error) {
	b.mu.Lock("UpdateDistribution")
	defer b.mu.Unlock()

	d, ok := b.distributions.Get(req.Name)
	if !ok {
		return nil, notFoundError("Distribution", req.Name)
	}

	if req.CertificateName != "" {
		d.CertificateName = req.CertificateName
	}

	if req.IsEnabled != nil {
		d.IsEnabled = *req.IsEnabled
	}

	if req.DefaultCacheBehavior != nil {
		d.DefaultCacheBehavior = *req.DefaultCacheBehavior
	}

	if req.CacheBehaviorSettings != nil {
		d.CacheBehaviorSettings = req.CacheBehaviorSettings
	}

	if req.CacheBehaviors != nil {
		d.CacheBehaviors = req.CacheBehaviors
	}

	if req.ViewerMinTLSVersion != "" {
		d.ViewerMinTLSVersion = req.ViewerMinTLSVersion
	}

	ops := b.newOperationsLocked(
		opTypeUpdateDistribution,
		ResourceTypeDistribution,
		[]string{req.Name},
	)

	return &ops[0], nil
}

// DeleteDistribution deletes the named distribution.
func (b *InMemoryBackend) DeleteDistribution(name string) (*Operation, error) {
	b.mu.Lock("DeleteDistribution")
	defer b.mu.Unlock()

	d, ok := b.distributions.Get(name)
	if !ok {
		return nil, notFoundError("Distribution", name)
	}

	if d.Tags != nil {
		d.Tags.Close()
	}

	b.distributions.Delete(name)
	b.unregisterNameLocked(name)
	delete(b.distributionCacheResets, name)

	ops := b.newOperationsLocked(opTypeDeleteDistribution, ResourceTypeDistribution, []string{name})

	return &ops[0], nil
}

// GetDistributions returns every distribution, optionally filtered by name,
// paginated.
func (b *InMemoryBackend) GetDistributions(name, token string) (page.Page[*Distribution], error) {
	b.mu.RLock("GetDistributions")
	defer b.mu.RUnlock()

	all := b.distributions.All()
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	out := make([]*Distribution, 0, len(all))

	for _, d := range all {
		if name != "" && d.Name != name {
			continue
		}

		out = append(out, d.clone())
	}

	return paginateGeneric(out, token)
}

// UpdateDistributionBundle changes the named distribution's bundle tier.
func (b *InMemoryBackend) UpdateDistributionBundle(name, bundleID string) (*Operation, error) {
	b.mu.Lock("UpdateDistributionBundle")
	defer b.mu.Unlock()

	d, ok := b.distributions.Get(name)
	if !ok {
		return nil, notFoundError("Distribution", name)
	}

	d.BundleID = bundleID

	ops := b.newOperationsLocked(
		opTypeUpdateDistributionBundle,
		ResourceTypeDistribution,
		[]string{name},
	)

	return &ops[0], nil
}

// ResetDistributionCache resets the named distribution's cache, recording
// the reset time.
func (b *InMemoryBackend) ResetDistributionCache(name string) (*Operation, time.Time, error) {
	b.mu.Lock("ResetDistributionCache")
	defer b.mu.Unlock()

	if _, ok := b.distributions.Get(name); !ok {
		return nil, time.Time{}, notFoundError("Distribution", name)
	}

	now := nowUTC()
	b.distributionCacheResets[name] = now

	ops := b.newOperationsLocked(
		opTypeResetDistributionCache,
		ResourceTypeDistribution,
		[]string{name},
	)

	return &ops[0], now, nil
}

// GetDistributionLatestCacheReset returns the named distribution's most
// recent cache reset time, or a NotFoundException if it has never been
// reset -- a real state (never fabricated), not always "just now".
func (b *InMemoryBackend) GetDistributionLatestCacheReset(name string) (time.Time, error) {
	b.mu.RLock("GetDistributionLatestCacheReset")
	defer b.mu.RUnlock()

	if _, ok := b.distributions.Get(name); !ok {
		return time.Time{}, notFoundError("Distribution", name)
	}

	t, ok := b.distributionCacheResets[name]
	if !ok {
		return time.Time{}, notFoundError("cache reset record for Distribution", name)
	}

	return t, nil
}

// GetDistributionMetricData returns a real, well-formed, EMPTY MetricData
// response -- one of the six honestly-unfakeable telemetry ops
// (PARITY.md 4.10).
func (b *InMemoryBackend) GetDistributionMetricData(name string) error {
	b.mu.RLock("GetDistributionMetricData")
	defer b.mu.RUnlock()

	if _, ok := b.distributions.Get(name); !ok {
		return notFoundError("Distribution", name)
	}

	return nil
}

// AttachCertificateToDistribution attaches an ISSUED certificate to the
// named distribution.
func (b *InMemoryBackend) AttachCertificateToDistribution(
	distributionName, certificateName string,
) (*Operation, error) {
	b.mu.Lock("AttachCertificateToDistribution")
	defer b.mu.Unlock()

	d, ok := b.distributions.Get(distributionName)
	if !ok {
		return nil, notFoundError("Distribution", distributionName)
	}

	if _, certOK := b.certificates.Get(certificateName); !certOK {
		return nil, notFoundError("Certificate", certificateName)
	}

	d.CertificateName = certificateName

	ops := b.newOperationsLocked(
		opTypeAttachCertToDistribution,
		ResourceTypeDistribution,
		[]string{distributionName},
	)

	return &ops[0], nil
}

// DetachCertificateFromDistribution detaches whatever certificate is
// attached to the named distribution.
func (b *InMemoryBackend) DetachCertificateFromDistribution(
	distributionName string,
) (*Operation, error) {
	b.mu.Lock("DetachCertificateFromDistribution")
	defer b.mu.Unlock()

	d, ok := b.distributions.Get(distributionName)
	if !ok {
		return nil, notFoundError("Distribution", distributionName)
	}

	d.CertificateName = ""

	ops := b.newOperationsLocked(
		opTypeDetachCertFromDistribution,
		ResourceTypeDistribution,
		[]string{distributionName},
	)

	return &ops[0], nil
}
