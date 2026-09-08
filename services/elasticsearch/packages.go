package elasticsearch

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// CreatePackage creates a new Elasticsearch package (e.g., a dictionary file).
// PackageSource (S3BucketName + S3Key) is a required member of
// CreatePackageInput in the real API (types.CreatePackageInput.PackageSource
// has no default), so a missing/incomplete source is rejected exactly like a
// missing name or an invalid type.
func (b *InMemoryBackend) CreatePackage(
	ctx context.Context, name, packageType, description string, source PackageSource,
) (*Package, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: PackageName is required", ErrValidation)
	}

	if !validPackageTypes[packageType] {
		return nil, fmt.Errorf(
			"%w: PackageType must be TXT-DICTIONARY, got %q",
			ErrValidation,
			packageType,
		)
	}

	if source.S3BucketName == "" || source.S3Key == "" {
		return nil, fmt.Errorf("%w: PackageSource.S3BucketName and PackageSource.S3Key are required", ErrValidation)
	}

	region := getRegion(ctx, b.region)
	b.mu.Lock("CreatePackage")
	defer b.mu.Unlock()

	packagesByName := b.packagesByNameStore(region)
	if _, exists := packagesByName[name]; exists {
		return nil, fmt.Errorf("%w: package %s already exists", ErrDomainAlreadyExists, name)
	}

	id := fmt.Sprintf("F%010d", b.nextIDLocked())
	now := time.Now()
	pkg := &Package{
		ID:            id,
		Name:          name,
		PackageType:   packageType,
		Description:   description,
		Status:        "AVAILABLE",
		PackageSource: source,
		CreatedAt:     now,
		LastUpdatedAt: now,
		region:        region,
	}
	b.packagePut(pkg)
	packagesByName[name] = id

	cp := *pkg

	return &cp, nil
}

// AssociatePackage associates an Elasticsearch package with a domain.
func (b *InMemoryBackend) AssociatePackage(ctx context.Context, packageID, domainName string) error {
	region := getRegion(ctx, b.region)
	b.mu.Lock("AssociatePackage")
	defer b.mu.Unlock()

	if _, exists := b.packageGet(region, packageID); !exists {
		return fmt.Errorf("%w: package %s not found", ErrPackageNotFound, packageID)
	}

	if _, exists := b.domainGet(region, domainName); !exists {
		return fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	assocs := b.packageAssociationsStore(region)
	if slices.Contains(assocs[packageID], domainName) {
		return fmt.Errorf(
			"%w: package %s is already associated with domain %s",
			ErrPackageAlreadyAssociated, packageID, domainName,
		)
	}

	assocs[packageID] = append(assocs[packageID], domainName)

	return nil
}

// DeletePackage removes a package by ID.
func (b *InMemoryBackend) DeletePackage(ctx context.Context, packageID string) (*Package, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("DeletePackage")
	defer b.mu.Unlock()

	pkg, exists := b.packageGet(region, packageID)
	if !exists {
		return nil, fmt.Errorf("%w: package %s not found", ErrPackageNotFound, packageID)
	}

	cp := *pkg
	delete(b.packagesByNameStore(region), pkg.Name)
	b.packageDelete(region, packageID)
	delete(b.packageAssociationsStore(region), packageID)

	return &cp, nil
}

// DescribePackages returns packages matching the given IDs, or all packages if the list is empty.
func (b *InMemoryBackend) DescribePackages(ctx context.Context, packageIDs []string) []*Package {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribePackages")
	defer b.mu.RUnlock()

	if len(packageIDs) == 0 {
		packages := b.packagesInRegion(region)
		result := make([]*Package, 0, len(packages))
		for _, pkg := range packages {
			cp := *pkg
			result = append(result, &cp)
		}

		return result
	}

	result := make([]*Package, 0, len(packageIDs))
	for _, id := range packageIDs {
		if pkg, exists := b.packageGet(region, id); exists {
			cp := *pkg
			result = append(result, &cp)
		}
	}

	return result
}

// DissociatePackage removes a package association from a domain.
func (b *InMemoryBackend) DissociatePackage(ctx context.Context, packageID, domainName string) error {
	region := getRegion(ctx, b.region)
	b.mu.Lock("DissociatePackage")
	defer b.mu.Unlock()

	if _, exists := b.packageGet(region, packageID); !exists {
		return fmt.Errorf("%w: package %s not found", ErrPackageNotFound, packageID)
	}

	if _, exists := b.domainGet(region, domainName); !exists {
		return fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	associations := b.packageAssociationsStore(region)
	assocs := associations[packageID]
	for i, name := range assocs {
		if name == domainName {
			associations[packageID] = append(assocs[:i], assocs[i+1:]...)

			return nil
		}
	}

	return nil
}

// GetPackageVersionHistory returns the version history for a package.
func (b *InMemoryBackend) GetPackageVersionHistory(ctx context.Context, packageID string) ([]*Package, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("GetPackageVersionHistory")
	defer b.mu.RUnlock()

	pkg, exists := b.packageGet(region, packageID)
	if !exists {
		return nil, fmt.Errorf("%w: package %s not found", ErrPackageNotFound, packageID)
	}

	cp := *pkg

	return []*Package{&cp}, nil
}

// ListDomainsForPackage returns all domain names associated with a package.
func (b *InMemoryBackend) ListDomainsForPackage(ctx context.Context, packageID string) ([]string, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("ListDomainsForPackage")
	defer b.mu.RUnlock()

	if _, exists := b.packageGet(region, packageID); !exists {
		return nil, fmt.Errorf("%w: package %s not found", ErrPackageNotFound, packageID)
	}

	assocs := b.packageAssociationsStoreRO(region)[packageID]
	result := make([]string, len(assocs))
	copy(result, assocs)

	return result, nil
}

// ListPackagesForDomain returns all packages associated with a domain.
func (b *InMemoryBackend) ListPackagesForDomain(ctx context.Context, domainName string) ([]*Package, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("ListPackagesForDomain")
	defer b.mu.RUnlock()

	if _, exists := b.domainGet(region, domainName); !exists {
		return nil, fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	var result []*Package
	for packageID, assocs := range b.packageAssociationsStoreRO(region) {
		if slices.Contains(assocs, domainName) {
			if pkg, exists := b.packageGet(region, packageID); exists {
				cp := *pkg
				result = append(result, &cp)
			}
		}
	}

	return result, nil
}

// UpdatePackage updates a package description.
func (b *InMemoryBackend) UpdatePackage(ctx context.Context, packageID, description string) (*Package, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("UpdatePackage")
	defer b.mu.Unlock()

	pkg, exists := b.packageGet(region, packageID)
	if !exists {
		return nil, fmt.Errorf("%w: package %s not found", ErrPackageNotFound, packageID)
	}

	pkg.Description = description
	pkg.LastUpdatedAt = time.Now()
	cp := *pkg

	return &cp, nil
}
