package codestarconnections

import (
	"context"
	"fmt"
	"maps"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// connectionHasReferenceToHostLocked returns true if any connection in the region references hostArn.
// Must be called with at least an RLock held.
func (b *InMemoryBackend) connectionHasReferenceToHostLocked(region, hostArn string) bool {
	for _, conn := range b.connectionsByRegion.Get(region) {
		if conn.HostArn == hostArn {
			return true
		}
	}

	return false
}

// CreateHost creates a new CodeStar host.
//
// The validation below (ErrValidation, wire InvalidInputException) has no
// correct declared type to send: this op's own switch
// (codestarconnections@v1.38.4 deserializers.go
// deserializeOpErrorCreateHost) is exactly [LimitExceededException] -- no
// InvalidInputException, and no ValidationException equivalent exists
// anywhere in this SDK module. Recorded, not fixed (gopherstack-6flj/uox6
// error-envelope sweep).
func (b *InMemoryBackend) CreateHost(
	ctx context.Context,
	name, providerType, providerEndpoint string,
	vpcConfig *VpcConfiguration,
	tags map[string]string,
) (*Host, error) {
	if err := validateHostName(name); err != nil {
		return nil, err
	}

	if providerEndpoint == "" {
		return nil, fmt.Errorf("%w: ProviderEndpoint is required", ErrValidation)
	}

	if len(providerEndpoint) > maxProviderEndpointLen {
		return nil, fmt.Errorf("%w: ProviderEndpoint must not exceed %d characters",
			ErrValidation, maxProviderEndpointLen)
	}

	if providerType != "" && !validProviderTypes()[providerType] {
		return nil, fmt.Errorf("%w: invalid ProviderType %q", ErrValidation, providerType)
	}

	if err := validateTags(tags); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("CreateHost")
	defer b.mu.Unlock()

	// A duplicate Name is NOT rejected: CreateHost's real error list is
	// exactly [LimitExceededException] (codestarconnections@v1.38.4
	// deserializers.go's awsAwsjson10_deserializeOpErrorCreateHost switch) --
	// no InvalidInputException case for a name collision -- so a real
	// client's second create for the same name gets a distinct ARN, not an
	// error (same behavior as sibling codeconnections@v1.13.4, confirmed
	// independently against its own identical switch).

	id := uuid.NewString()
	hostArn := arn.Build("codestar-connections", region, b.accountID, "host/"+name+"/"+id[:8])

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	host := &Host{
		Name:             name,
		HostArn:          hostArn,
		ProviderType:     providerType,
		ProviderEndpoint: providerEndpoint,
		Status:           HostStatusPending,
		VpcConfiguration: vpcConfig,
		Tags:             tagsCopy,
	}
	b.hosts.Put(host)

	cp := *host
	cp.Tags = make(map[string]string, len(host.Tags))
	maps.Copy(cp.Tags, host.Tags)

	return &cp, nil
}

// GetHost returns a host by ARN.
func (b *InMemoryBackend) GetHost(_ context.Context, hostArn string) (*Host, error) {
	b.mu.RLock("GetHost")
	defer b.mu.RUnlock()

	host, ok := b.hosts.Get(hostArn)
	if !ok {
		return nil, fmt.Errorf("%w: host not found: %s", ErrNotFound, hostArn)
	}

	cp := *host
	cp.Tags = make(map[string]string, len(host.Tags))
	maps.Copy(cp.Tags, host.Tags)

	return &cp, nil
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

	// Name is not unique (CreateHost has no ResourceAlreadyExistsException
	// for a duplicate name, see errors.go), so HostArn (always unique)
	// breaks ties -- sort.Slice is not stable, and without a tie-break two
	// hosts sharing a name have no total order between them.
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}

		return result[i].HostArn < result[j].HostArn
	})

	return result
}

// DeleteHost removes a host by ARN. Returns ErrResourceInUse if any connection references the host.
func (b *InMemoryBackend) DeleteHost(ctx context.Context, hostArn string) error {
	region := regionFromARN(hostArn, getRegion(ctx, b.defaultRegion))

	b.mu.Lock("DeleteHost")
	defer b.mu.Unlock()

	host, ok := b.hosts.Get(hostArn)
	if !ok {
		return fmt.Errorf("%w: host not found: %s", ErrNotFound, hostArn)
	}

	if b.connectionHasReferenceToHostLocked(region, hostArn) {
		return fmt.Errorf("%w: host %q has active connections; delete them first", ErrResourceInUse, host.Name)
	}

	b.hosts.Delete(hostArn)

	return nil
}

// UpdateHost updates the provider endpoint and optional VPC configuration for a host.
func (b *InMemoryBackend) UpdateHost(
	_ context.Context,
	hostArn, providerEndpoint string,
	vpcConfig *VpcConfiguration,
) error {
	// UpdateHost's own error switch (codestarconnections@v1.38.4
	// deserializers.go) declares ConflictException/ResourceNotFoundException/
	// ResourceUnavailableException/UnsupportedOperationException -- no
	// InvalidInputException and no ValidationException equivalent exists
	// anywhere in this SDK module. No correct code exists to send for a
	// too-long ProviderEndpoint; ErrValidation (wrong for this op) is left
	// rather than substituted with an equally-wrong declared code
	// (gopherstack-6flj/uox6 error-envelope sweep).
	if providerEndpoint != "" && len(providerEndpoint) > maxProviderEndpointLen {
		return fmt.Errorf("%w: ProviderEndpoint must not exceed %d characters", ErrValidation, maxProviderEndpointLen)
	}

	b.mu.Lock("UpdateHost")
	defer b.mu.Unlock()

	host, ok := b.hosts.Get(hostArn)
	if !ok {
		return fmt.Errorf("%w: host not found: %s", ErrNotFound, hostArn)
	}

	// ProviderEndpoint/VpcConfiguration are not part of any index key
	// (hosts is keyed by HostArn; byRegion/byName derive from HostArn/Name),
	// so mutating the stored *Host in place is safe -- no Delete+Put needed.
	if providerEndpoint != "" {
		host.ProviderEndpoint = providerEndpoint
	}

	if vpcConfig != nil {
		host.VpcConfiguration = vpcConfig
	}

	return nil
}
