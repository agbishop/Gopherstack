package sesv2

import (
	"fmt"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

const (
	scalingModeStandard = "STANDARD"
	scalingModeManaged  = "MANAGED"
)

// DedicatedIPPool represents a SES v2 dedicated IP pool.
type DedicatedIPPool struct {
	PoolName    string `json:"poolName"`
	ScalingMode string `json:"scalingMode"`
}

// dedicatedIPPoolARN builds the ARN for a dedicated IP pool:
// arn:{partition}:ses:{region}:{account}:dedicated-ip-pool/{name}. Confirmed
// against terraform-provider-aws's dedicatedIPPoolARN
// (internal/service/sesv2/dedicated_ip_pool.go), which must construct this
// exact ARN to tag/import real dedicated IP pools. types.DedicatedIpPool has
// no Tags field, so unlike configuration sets/contact lists, tags never echo
// back through GetDedicatedIpPool -- only through ListTagsForResource.
func (b *InMemoryBackend) dedicatedIPPoolARN(name string) string {
	return arn.Build("ses", b.region, b.accountID, "dedicated-ip-pool/"+name)
}

// CreateDedicatedIPPool creates a dedicated IP pool.
func (b *InMemoryBackend) CreateDedicatedIPPool(
	poolName, scalingMode string,
	tags map[string]string,
) (*DedicatedIPPool, error) {
	if strings.TrimSpace(poolName) == "" {
		return nil, fmt.Errorf("%w: PoolName is required", ErrInvalidInput)
	}

	if scalingMode == "" {
		scalingMode = scalingModeStandard
	}

	if scalingMode != scalingModeStandard && scalingMode != scalingModeManaged {
		return nil, fmt.Errorf(
			"%w: ScalingMode must be %s or %s, got %s",
			ErrInvalidInput, scalingModeStandard, scalingModeManaged, scalingMode,
		)
	}

	b.mu.Lock("CreateDedicatedIPPool")
	defer b.mu.Unlock()

	if b.dedicatedIPPools.Has(poolName) {
		return nil, fmt.Errorf(
			"%w: dedicated IP pool %s already exists",
			ErrAlreadyExists,
			poolName,
		)
	}

	pool := &DedicatedIPPool{
		PoolName:    poolName,
		ScalingMode: scalingMode,
	}
	b.dedicatedIPPools.Put(pool)

	if len(tags) > 0 {
		b.putResourceTagsLocked(b.dedicatedIPPoolARN(poolName), tags)
	}

	cp := *pool

	return &cp, nil
}

// GetDedicatedIPPool retrieves a dedicated IP pool.
func (b *InMemoryBackend) GetDedicatedIPPool(poolName string) (*DedicatedIPPool, error) {
	b.mu.RLock("GetDedicatedIPPool")
	defer b.mu.RUnlock()

	pool, ok := b.dedicatedIPPools.Get(poolName)
	if !ok {
		return nil, fmt.Errorf("%w: dedicated IP pool %s not found", ErrNotFound, poolName)
	}

	cp := *pool

	return &cp, nil
}

// DeleteDedicatedIPPool removes a dedicated IP pool.
func (b *InMemoryBackend) DeleteDedicatedIPPool(poolName string) error {
	b.mu.Lock("DeleteDedicatedIPPool")
	defer b.mu.Unlock()

	if !b.dedicatedIPPools.Has(poolName) {
		return fmt.Errorf("%w: dedicated IP pool %s not found", ErrNotFound, poolName)
	}

	b.dedicatedIPPools.Delete(poolName)
	delete(b.resourceTags, b.dedicatedIPPoolARN(poolName))

	return nil
}

// ListDedicatedIPPools returns all dedicated IP pool names.
func (b *InMemoryBackend) ListDedicatedIPPools(nextToken string, pageSize int) page.Page[string] {
	b.mu.RLock("ListDedicatedIPPools")
	defer b.mu.RUnlock()

	snap := b.dedicatedIPPools.Snapshot()
	keys := make([]string, len(snap))

	for i, p := range snap {
		keys[i] = p.PoolName
	}

	return page.New(keys, nextToken, pageSize, sesv2DefaultMaxItems)
}

// PutDedicatedIPPoolScalingAttributes updates a pool's scaling mode.
func (b *InMemoryBackend) PutDedicatedIPPoolScalingAttributes(poolName, scalingMode string) error {
	b.mu.Lock("PutDedicatedIPPoolScalingAttributes")
	defer b.mu.Unlock()

	pool, ok := b.dedicatedIPPools.Get(poolName)
	if !ok {
		return fmt.Errorf("%w: dedicated IP pool %s not found", ErrNotFound, poolName)
	}

	pool.ScalingMode = scalingMode

	return nil
}
