package codeconnections

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateHost creates a new host.
func (b *InMemoryBackend) CreateHost(
	ctx context.Context,
	name, providerType, providerEndpoint string,
	vpcConfig *VpcConfiguration,
	tags map[string]string,
) (*Host, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidation)
	}

	if providerEndpoint == "" {
		return nil, fmt.Errorf("%w: ProviderEndpoint is required", ErrValidation)
	}

	if providerType == "" || !validProviderTypes()[providerType] {
		return nil, fmt.Errorf("%w: invalid ProviderType %q", ErrValidation, providerType)
	}

	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("CreateHost")
	defer b.mu.Unlock()

	// A duplicate Name is NOT rejected: CreateHost's real error list is
	// exactly [LimitExceededException] (aws-sdk-go-v2/service/
	// codeconnections@v1.13.4 deserializers.go's
	// awsAwsjson10_deserializeOpErrorCreateHost switch) -- no
	// ResourceAlreadyExistsException -- so a real client's second create for
	// the same name gets a distinct ARN, not an error.

	id := uuid.NewString()
	hostArn := arn.Build("codeconnections", region, b.accountID, "host/"+id)

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	host := &Host{
		Name:             name,
		HostArn:          hostArn,
		ProviderType:     providerType,
		ProviderEndpoint: providerEndpoint,
		Status:           "AVAILABLE",
		VpcConfiguration: vpcConfig,
		Tags:             tagsCopy,
		CreatedAt:        time.Now().UTC(),
	}

	b.hosts.Put(host)

	cp := *host
	cp.Tags = make(map[string]string, len(host.Tags))
	maps.Copy(cp.Tags, host.Tags)

	return &cp, nil
}

// GetHost retrieves a host by ARN, scoped to the caller's request region (see GetConnection).
func (b *InMemoryBackend) GetHost(ctx context.Context, hostArn string) (*Host, error) {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.RLock("GetHost")
	defer b.mu.RUnlock()

	host, ok := b.hosts.Get(hostArn)
	if !ok || regionFromARN(hostArn) != region {
		return nil, ErrNotFound
	}

	cp := *host
	cp.Tags = make(map[string]string, len(host.Tags))
	maps.Copy(cp.Tags, host.Tags)

	return &cp, nil
}

// hostExistsLocked returns true if a host with hostArn exists in region.
// Must be called with at least an RLock held.
func (b *InMemoryBackend) hostExistsLocked(region, hostArn string) bool {
	_, ok := b.hosts.Get(hostArn)

	return ok && regionFromARN(hostArn) == region
}

// connectionHasReferenceToHostLocked returns true if any connection in region
// references hostArn. Must be called with at least an RLock held.
func (b *InMemoryBackend) connectionHasReferenceToHostLocked(region, hostArn string) bool {
	for _, conn := range b.connectionsByRegion.Get(region) {
		if conn.HostArn == hostArn {
			return true
		}
	}

	return false
}

// DeleteHost removes a host by ARN. The real operation documents that all
// connections associated to a host must be deleted before the host itself
// can be deleted.
func (b *InMemoryBackend) DeleteHost(ctx context.Context, hostArn string) error {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("DeleteHost")
	defer b.mu.Unlock()

	host, ok := b.hosts.Get(hostArn)
	if !ok || regionFromARN(hostArn) != region {
		return ErrNotFound
	}

	if b.connectionHasReferenceToHostLocked(region, hostArn) {
		return fmt.Errorf("%w: host %q has active connections; delete them first", ErrResourceInUse, host.Name)
	}

	b.hosts.Delete(hostArn)

	return nil
}

// AddHostInternal seeds a host directly for testing.
func (b *InMemoryBackend) AddHostInternal(_ context.Context, host *Host) {
	b.mu.Lock("AddHostInternal")
	defer b.mu.Unlock()

	b.hosts.Put(host)
}

// ListHosts returns all hosts sorted by name.
func (b *InMemoryBackend) ListHosts(ctx context.Context) []*Host {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.RLock("ListHosts")
	defer b.mu.RUnlock()

	hs := b.hostsByRegion.Get(region)
	result := make([]*Host, 0, len(hs))

	for _, host := range hs {
		cp := *host
		cp.Tags = make(map[string]string, len(host.Tags))
		maps.Copy(cp.Tags, host.Tags)
		result = append(result, &cp)
	}

	// Name is not unique (CreateHost has no ResourceAlreadyExistsException for
	// a duplicate name, see CreateHost above), so HostArn (always unique)
	// breaks ties -- without it, two hosts sharing a name have no total order
	// between them.
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}

		return result[i].HostArn < result[j].HostArn
	})

	return result
}

// UpdateHost updates the provider endpoint and/or VPC configuration for a host.
func (b *InMemoryBackend) UpdateHost(
	ctx context.Context,
	hostArn, providerEndpoint string,
	vpcConfig *VpcConfiguration,
) error {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("UpdateHost")
	defer b.mu.Unlock()

	host, ok := b.hosts.Get(hostArn)
	if !ok || regionFromARN(hostArn) != region {
		return ErrNotFound
	}

	// ProviderEndpoint/VpcConfiguration are not part of any index key (hosts
	// is keyed by HostArn; byRegion derives from HostArn), so mutating the
	// stored *Host in place is safe -- no Delete+Put needed.
	if providerEndpoint != "" {
		host.ProviderEndpoint = providerEndpoint
	}

	if vpcConfig != nil {
		host.VpcConfiguration = vpcConfig
	}

	return nil
}
