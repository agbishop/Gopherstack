package elasticsearch

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// CreateDomain creates a new Elasticsearch domain.
func (b *InMemoryBackend) CreateDomain(ctx context.Context, inp CreateDomainInput) (*Domain, error) {
	if inp.Name == "" {
		return nil, fmt.Errorf("%w: DomainName is required", ErrValidation)
	}

	if !domainNameRe.MatchString(inp.Name) {
		return nil, fmt.Errorf(
			"%w: DomainName must be 3-28 lowercase alphanumeric characters or hyphens and start with a letter",
			ErrValidation,
		)
	}

	region := getRegion(ctx, b.region)
	b.mu.Lock("CreateDomain")
	defer b.mu.Unlock()

	if _, exists := b.domainGet(region, inp.Name); exists {
		return nil, fmt.Errorf("%w: domain %s already exists", ErrDomainAlreadyExists, inp.Name)
	}

	esVersion := inp.ElasticsearchVersion
	if esVersion == "" {
		esVersion = defaultElasticsearchVersion
	} else if !validElasticsearchVersions[esVersion] {
		return nil, fmt.Errorf("%w: invalid ElasticsearchVersion %q", ErrValidation, esVersion)
	}

	domainARN := arn.Build("es", region, b.accountID, "domain/"+inp.Name)
	domainID := b.accountID + "/" + inp.Name
	endpoint := fmt.Sprintf("search-%s-%s.%s.es.amazonaws.com", inp.Name, b.accountID, region)

	clusterConfig := inp.ClusterConfig
	if clusterConfig.InstanceCount == 0 {
		clusterConfig.InstanceCount = 1
	}

	if clusterConfig.InstanceType == "" {
		clusterConfig.InstanceType = defaultInstanceType
	}

	now := time.Now()
	d := &Domain{
		region:                      region,
		Name:                        inp.Name,
		DomainID:                    domainID,
		ARN:                         domainARN,
		ElasticsearchVersion:        esVersion,
		Endpoint:                    endpoint,
		Status:                      statusActiveCap,
		ClusterConfig:               clusterConfig,
		EBSOptions:                  inp.EBSOptions,
		SnapshotOptions:             inp.SnapshotOptions,
		AdvancedOptions:             inp.AdvancedOptions,
		AccessPolicies:              inp.AccessPolicies,
		VPCOptions:                  cloneVPCOptions(inp.VPCOptions),
		CognitoOptions:              cloneCognitoOptions(inp.CognitoOptions),
		AdvancedSecurityOptions:     cloneAdvancedSecurityOptions(inp.AdvancedSecurityOptions),
		AutoTuneOptions:             cloneAutoTuneOptions(inp.AutoTuneOptions),
		DeploymentStrategyOptions:   cloneDeploymentStrategyOptions(inp.DeploymentStrategyOptions),
		LogPublishingOptions:        cloneLogPublishingOptions(inp.LogPublishingOptions),
		EncryptionAtRestEnabled:     inp.EncryptionAtRestEnabled,
		NodeToNodeEncryptionEnabled: inp.NodeToNodeEncryptionEnabled,
		EnforceHTTPS:                inp.EnforceHTTPS,
		TLSSecurityPolicy:           inp.TLSSecurityPolicy,
		CreatedAt:                   now,
		ConfigUpdatedAt:             now,
		ConfigVersion:               1,
		Tags:                        tags.New("elasticsearch." + region + "." + inp.Name + ".tags"),
	}

	if len(inp.Tags) > 0 {
		d.Tags.Merge(inp.Tags)
	}

	b.domainPut(d)
	b.arnIndexStore(region)[domainARN] = inp.Name

	if b.dnsRegistrar != nil {
		b.dnsRegistrar.Register(endpoint)
	}

	return domainCopy(d), nil
}

// DeleteDomain removes a domain by name.
func (b *InMemoryBackend) DeleteDomain(ctx context.Context, name string) (*Domain, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("DeleteDomain")
	defer b.mu.Unlock()

	d, exists := b.domainGet(region, name)
	if !exists {
		return nil, fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, name)
	}

	cp := domainCopy(d)
	d.Tags.Close()
	delete(b.arnIndexStore(region), d.ARN)
	delete(b.vpcAccessStore(region), name)

	assocs := b.packageAssociationsStore(region)
	for packageID, domains := range assocs {
		if idx := slices.Index(domains, name); idx >= 0 {
			assocs[packageID] = slices.Delete(domains, idx, idx+1)
		}
	}

	b.domainDelete(region, name)

	if b.dnsRegistrar != nil {
		b.dnsRegistrar.Deregister(cp.Endpoint)
	}

	return cp, nil
}

// DescribeDomain returns details about a domain.
func (b *InMemoryBackend) DescribeDomain(ctx context.Context, name string) (*Domain, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeDomain")
	defer b.mu.RUnlock()

	d, exists := b.domainGet(region, name)
	if !exists {
		return nil, fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, name)
	}

	return domainCopy(d), nil
}

// ListDomainNames returns the sorted names of all domains in the request's region.
func (b *InMemoryBackend) ListDomainNames(ctx context.Context) []string {
	region := getRegion(ctx, b.region)
	b.mu.RLock("ListDomainNames")
	defer b.mu.RUnlock()

	domains := b.domainsInRegion(region)
	names := make([]string, 0, len(domains))
	for _, d := range domains {
		names = append(names, d.Name)
	}

	slices.Sort(names)

	return names
}

// findDomainByARN returns the domain matching the given ARN within the given
// region, or nil if not found. Caller must hold at least a read lock.
func (b *InMemoryBackend) findDomainByARN(region, domainARN string) *Domain {
	name, ok := b.arnIndexStoreRO(region)[domainARN]
	if !ok {
		return nil
	}

	d, _ := b.domainGet(region, name)

	return d
}

// AddDomainInternal seeds a domain directly into the backend for testing.
// Tags are initialised fresh for the seeded domain.
func (b *InMemoryBackend) AddDomainInternal(ctx context.Context, d Domain) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("AddDomainInternal")
	defer b.mu.Unlock()

	cp := d
	if cp.Tags == nil {
		cp.Tags = tags.New("elasticsearch." + region + "." + cp.Name + ".tags")
	}

	cp.region = region
	b.domainPut(&cp)

	if cp.ARN != "" {
		b.arnIndexStore(region)[cp.ARN] = cp.Name
	}
}
