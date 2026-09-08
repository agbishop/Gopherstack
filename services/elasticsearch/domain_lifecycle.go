package elasticsearch

import (
	"context"
	"fmt"
)

// CancelElasticsearchServiceSoftwareUpdate cancels a scheduled software update.
// Because the in-memory backend never schedules updates this is a no-op.
func (b *InMemoryBackend) CancelElasticsearchServiceSoftwareUpdate(
	ctx context.Context, domainName string,
) (*Domain, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("CancelElasticsearchServiceSoftwareUpdate")
	defer b.mu.RUnlock()

	d, exists := b.domainGet(region, domainName)
	if !exists {
		return nil, fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	return domainCopy(d), nil
}

// DeleteElasticsearchServiceRole deletes the Elasticsearch service-linked
// IAM role. Real AWS: "Role deletion will fail if any existing VPC domains
// use the role. You must delete any such Elasticsearch domains before
// deleting the role." The in-memory backend has no IAM state, so this only
// enforces that precondition against tracked domains.
func (b *InMemoryBackend) DeleteElasticsearchServiceRole() error {
	b.mu.RLock("DeleteElasticsearchServiceRole")
	defer b.mu.RUnlock()

	for _, d := range b.domains.All() {
		if d.VPCOptions != nil {
			return fmt.Errorf(
				"%w: domain %s uses a VPC and still uses the service-linked role",
				ErrServiceRoleInUse, d.Name,
			)
		}
	}

	return nil
}

// GetUpgradeHistory validates a domain exists and returns empty history (no upgrade state tracked).
func (b *InMemoryBackend) GetUpgradeHistory(ctx context.Context, domainName string) error {
	region := getRegion(ctx, b.region)
	b.mu.RLock("GetUpgradeHistory")
	defer b.mu.RUnlock()

	if _, exists := b.domainGet(region, domainName); !exists {
		return fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	return nil
}

// GetUpgradeStatus validates a domain exists and returns (no upgrade in progress in-memory).
func (b *InMemoryBackend) GetUpgradeStatus(ctx context.Context, domainName string) error {
	region := getRegion(ctx, b.region)
	b.mu.RLock("GetUpgradeStatus")
	defer b.mu.RUnlock()

	if _, exists := b.domainGet(region, domainName); !exists {
		return fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	return nil
}

// StartElasticsearchServiceSoftwareUpdate schedules a software update (no-op in-memory).
func (b *InMemoryBackend) StartElasticsearchServiceSoftwareUpdate(
	ctx context.Context, domainName string,
) (*Domain, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("StartElasticsearchServiceSoftwareUpdate")
	defer b.mu.RUnlock()

	d, exists := b.domainGet(region, domainName)
	if !exists {
		return nil, fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	return domainCopy(d), nil
}

// UpgradeElasticsearchDomain upgrades a domain to the target version.
func (b *InMemoryBackend) UpgradeElasticsearchDomain(
	ctx context.Context, domainName, targetVersion string,
) (*Domain, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("UpgradeElasticsearchDomain")
	defer b.mu.Unlock()

	d, exists := b.domainGet(region, domainName)
	if !exists {
		return nil, fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	if targetVersion != "" {
		d.ElasticsearchVersion = targetVersion
	}

	return domainCopy(d), nil
}
