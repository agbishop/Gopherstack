package opensearch

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// AssociatePackage associates a package with a domain.
func (b *InMemoryBackend) AssociatePackage(
	packageID, domainName string,
) (*DomainPackageDetails, error) {
	if packageID == "" {
		return nil, fmt.Errorf("%w: PackageID is required", ErrInvalidParameter)
	}

	if domainName == "" {
		return nil, fmt.Errorf("%w: DomainName is required", ErrInvalidParameter)
	}

	b.mu.Lock("AssociatePackage")
	defer b.mu.Unlock()

	if !b.packages.Has(packageID) {
		return nil, fmt.Errorf("%w: package %s not found", ErrPackageNotFound, packageID)
	}

	if !b.domains.Has(domainName) {
		return nil, fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	b.addPackageAssociation(packageID, domainName)

	pkg, _ := b.packages.Get(packageID)

	return &DomainPackageDetails{
		PackageID:   packageID,
		DomainName:  domainName,
		State:       pkgStateActive,
		PackageName: pkg.PackageName,
		PackageType: pkg.PackageType,
		LastUpdated: float64(b.clock().Unix()),
	}, nil
}

// addPackageAssociation records a package↔domain association in both the
// forward (packageAssociations) and reverse (domainPackages) indexes.
// Caller must hold the write lock.
func (b *InMemoryBackend) addPackageAssociation(packageID, domainName string) {
	if b.packageAssociations[packageID] == nil {
		b.packageAssociations[packageID] = make(map[string]bool)
	}
	b.packageAssociations[packageID][domainName] = true

	if b.domainPackages[domainName] == nil {
		b.domainPackages[domainName] = make(map[string]bool)
	}
	b.domainPackages[domainName][packageID] = true
}

// removePackageAssociation removes a package↔domain association from both the
// forward and reverse indexes. Caller must hold the write lock.
func (b *InMemoryBackend) removePackageAssociation(packageID, domainName string) {
	if domains, ok := b.packageAssociations[packageID]; ok {
		delete(domains, domainName)
		if len(domains) == 0 {
			delete(b.packageAssociations, packageID)
		}
	}

	if pkgs, ok := b.domainPackages[domainName]; ok {
		delete(pkgs, packageID)
		if len(pkgs) == 0 {
			delete(b.domainPackages, domainName)
		}
	}
}

// AssociatePackages associates multiple packages with a domain.
func (b *InMemoryBackend) AssociatePackages(
	domainName string,
	packageIDs []string,
) ([]DomainPackageDetails, error) {
	if domainName == "" {
		return nil, fmt.Errorf("%w: DomainName is required", ErrInvalidParameter)
	}

	if len(packageIDs) == 0 {
		return nil, fmt.Errorf("%w: PackageList must not be empty", ErrInvalidParameter)
	}

	b.mu.Lock("AssociatePackages")
	defer b.mu.Unlock()

	if !b.domains.Has(domainName) {
		return nil, fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	results := make([]DomainPackageDetails, 0, len(packageIDs))

	for _, pkgID := range packageIDs {
		pkg, exists := b.packages.Get(pkgID)
		if !exists {
			return nil, fmt.Errorf("%w: package %s not found", ErrPackageNotFound, pkgID)
		}

		b.addPackageAssociation(pkgID, domainName)
		results = append(results, DomainPackageDetails{
			PackageID:   pkgID,
			DomainName:  domainName,
			State:       pkgStateActive,
			PackageName: pkg.PackageName,
			PackageType: pkg.PackageType,
			LastUpdated: float64(b.clock().Unix()),
		})
	}

	return results, nil
}

// CreatePackage creates a new OpenSearch package.
func (b *InMemoryBackend) CreatePackage(
	name, pkgType, description string,
	source *PackageSource,
	encryptionOptions *PackageEncryptionOptions,
) (*Package, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: PackageName is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreatePackage")
	defer b.mu.Unlock()

	b.packageCounter++
	id := fmt.Sprintf("F%d", b.packageCounter)
	now := float64(time.Now().Unix())

	pkg := &Package{
		PackageID:                id,
		PackageName:              name,
		PackageType:              pkgType,
		PackageDescription:       description,
		PackageStatus:            pkgStatusAvailable,
		PackageSource:            source,
		PackageEncryptionOptions: encryptionOptions,
		AvailablePackageVersion:  "1",
		CreatedAt:                now,
		VersionHistory: []*PackageVersionHistory{
			{
				PackageVersion: "1",
				CommitMessage:  "initial version",
				CreatedAt:      now,
			},
		},
	}
	b.packages.Put(pkg)

	cp := *pkg

	return &cp, nil
}

// DeletePackage removes a package by ID. Real AWS: "The package can't be
// associated with any OpenSearch Service domain".
func (b *InMemoryBackend) DeletePackage(packageID string) (*Package, error) {
	b.mu.Lock("DeletePackage")
	defer b.mu.Unlock()

	pkg, exists := b.packages.Get(packageID)
	if !exists {
		return nil, fmt.Errorf("%w: package %s not found", ErrPackageNotFound, packageID)
	}

	if len(b.packageAssociations[packageID]) > 0 {
		return nil, fmt.Errorf("%w: package %s is associated with a domain", ErrPackageAssociated, packageID)
	}

	cp := *pkg
	b.packages.Delete(packageID)

	return &cp, nil
}

// DescribePackages returns packages matching filters, paginated per
// nextToken/maxResults. filters is keyed by DescribePackagesFilterName
// (opensearch@v1.75.4 types/enums.go): PackageID, PackageName, PackageStatus
// and PackageType are all fields this backend's Package tracks and applies
// here; EngineVersion and PackageOwner are also valid enum members but this
// backend has no such fields on Package to filter against (structural gap,
// documented in PARITY.md) -- those two filter names are accepted but have
// no effect, same as an absent filter. With no PackageID filter, the
// unfiltered set is read via Snapshot() (ordered by PackageID ascending) so
// pkgs/page.New's offset-based pagination sees a reproducible order across
// calls, rather than All()'s unspecified map order.
func (b *InMemoryBackend) DescribePackages(
	filters map[string][]string, nextToken string, maxResults int,
) (page.Page[*Package], error) {
	b.mu.RLock("DescribePackages")
	defer b.mu.RUnlock()

	ids := filters["PackageID"]

	var base []*Package

	if len(ids) == 0 {
		base = make([]*Package, 0, b.packages.Len())
		for _, pkg := range b.packages.Snapshot() {
			cp := *pkg
			base = append(base, &cp)
		}
	} else {
		base = make([]*Package, 0, len(ids))

		for _, id := range ids {
			pkg, exists := b.packages.Get(id)
			if !exists {
				return page.Page[*Package]{}, fmt.Errorf("%w: package %s not found", ErrPackageNotFound, id)
			}

			cp := *pkg
			base = append(base, &cp)
		}
	}

	all := make([]*Package, 0, len(base))

	for _, pkg := range base {
		if names := filters["PackageName"]; len(names) > 0 && !slices.Contains(names, pkg.PackageName) {
			continue
		}

		if statuses := filters["PackageStatus"]; len(statuses) > 0 && !slices.Contains(statuses, pkg.PackageStatus) {
			continue
		}

		if types := filters["PackageType"]; len(types) > 0 && !slices.Contains(types, pkg.PackageType) {
			continue
		}

		all = append(all, pkg)
	}

	limit := maxResults
	if limit <= 0 {
		limit = len(all)
	}

	out := page.New(all, nextToken, limit, limit)

	return out, nil
}

// GetPackageVersionHistory returns the version history for a package.
func (b *InMemoryBackend) GetPackageVersionHistory(
	packageID string,
) ([]*PackageVersionHistory, error) {
	b.mu.RLock("GetPackageVersionHistory")
	defer b.mu.RUnlock()

	pkg, exists := b.packages.Get(packageID)
	if !exists {
		return nil, fmt.Errorf("%w: package %s not found", ErrPackageNotFound, packageID)
	}

	out := make([]*PackageVersionHistory, len(pkg.VersionHistory))
	for i, vh := range pkg.VersionHistory {
		cp := *vh
		out[i] = &cp
	}

	return out, nil
}

// UpdatePackage updates a package's description and adds a version history entry.
func (b *InMemoryBackend) UpdatePackage(packageID, description string) (*Package, error) {
	b.mu.Lock("UpdatePackage")
	defer b.mu.Unlock()

	pkg, exists := b.packages.Get(packageID)
	if !exists {
		return nil, fmt.Errorf("%w: package %s not found", ErrPackageNotFound, packageID)
	}

	pkg.PackageDescription = description
	pkg.VersionHistory = append(pkg.VersionHistory, &PackageVersionHistory{
		PackageVersion: strconv.Itoa(len(pkg.VersionHistory) + 1),
		CommitMessage:  "updated",
		CreatedAt:      float64(time.Now().Unix()),
	})

	cp := *pkg

	return &cp, nil
}

// UpdatePackageScope applies operation (ADD/REMOVE/OVERRIDE, types.
// PackageScopeOperationEnum in the pinned SDK) to the package's user list and
// returns the resulting scope.
func (b *InMemoryBackend) UpdatePackageScope(packageID, operation string, users []string) (*Package, error) {
	b.mu.Lock("UpdatePackageScope")
	defer b.mu.Unlock()

	pkg, exists := b.packages.Get(packageID)
	if !exists {
		return nil, fmt.Errorf("%w: package %s not found", ErrPackageNotFound, packageID)
	}

	switch operation {
	case "ADD":
		for _, u := range users {
			if !slices.Contains(pkg.PackageUserList, u) {
				pkg.PackageUserList = append(pkg.PackageUserList, u)
			}
		}
	case "REMOVE":
		pkg.PackageUserList = slices.DeleteFunc(pkg.PackageUserList, func(u string) bool {
			return slices.Contains(users, u)
		})
	case "OVERRIDE":
		pkg.PackageUserList = users
	}

	cp := *pkg

	return &cp, nil
}

// ListPackagesForDomain returns packages associated with a domain.
func (b *InMemoryBackend) ListPackagesForDomain(domainName string) []*Package {
	b.mu.RLock("ListPackagesForDomain")
	defer b.mu.RUnlock()

	var out []*Package

	for pkgID := range b.domainPackages[domainName] {
		pkg, exists := b.packages.Get(pkgID)
		if exists {
			cp := *pkg
			out = append(out, &cp)
		}
	}

	if out == nil {
		out = []*Package{}
	}

	return out
}

// ListDomainsForPackage returns domain names associated with a package.
func (b *InMemoryBackend) ListDomainsForPackage(packageID string) []string {
	b.mu.RLock("ListDomainsForPackage")
	defer b.mu.RUnlock()

	domains := b.packageAssociations[packageID]
	out := make([]string, 0, len(domains))

	for d := range domains {
		out = append(out, d)
	}

	slices.Sort(out)

	return out
}

// DissociatePackage removes a package association from a domain.
func (b *InMemoryBackend) DissociatePackage(
	packageID, domainName string,
) (*DomainPackageDetails, error) {
	if packageID == "" {
		return nil, fmt.Errorf("%w: PackageID is required", ErrInvalidParameter)
	}

	if domainName == "" {
		return nil, fmt.Errorf("%w: DomainName is required", ErrInvalidParameter)
	}

	b.mu.Lock("DissociatePackage")
	defer b.mu.Unlock()

	if !b.domains.Has(domainName) {
		return nil, fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	pkg, _ := b.packages.Get(packageID)

	b.removePackageAssociation(packageID, domainName)

	details := &DomainPackageDetails{
		PackageID:   packageID,
		DomainName:  domainName,
		State:       domainPackageStatusDissociating,
		LastUpdated: float64(b.clock().Unix()),
	}
	if pkg != nil {
		details.PackageName = pkg.PackageName
		details.PackageType = pkg.PackageType
	}

	return details, nil
}

// DissociatePackages removes multiple package associations from a domain.
func (b *InMemoryBackend) DissociatePackages(
	domainName string,
	packageIDs []string,
) ([]DomainPackageDetails, error) {
	if domainName == "" {
		return nil, fmt.Errorf("%w: DomainName is required", ErrInvalidParameter)
	}

	b.mu.Lock("DissociatePackages")
	defer b.mu.Unlock()

	if !b.domains.Has(domainName) {
		return nil, fmt.Errorf("%w: domain %s not found", ErrDomainNotFound, domainName)
	}

	results := make([]DomainPackageDetails, 0, len(packageIDs))

	for _, pkgID := range packageIDs {
		pkg, _ := b.packages.Get(pkgID)

		b.removePackageAssociation(pkgID, domainName)

		details := DomainPackageDetails{
			PackageID:   pkgID,
			DomainName:  domainName,
			State:       domainPackageStatusDissociating,
			LastUpdated: float64(b.clock().Unix()),
		}
		if pkg != nil {
			details.PackageName = pkg.PackageName
			details.PackageType = pkg.PackageType
		}

		results = append(results, details)
	}

	return results, nil
}

// AddPackageInternal seeds a package directly for use in tests.
func (b *InMemoryBackend) AddPackageInternal(packageID, packageName, packageType string) {
	b.mu.Lock("AddPackageInternal")
	defer b.mu.Unlock()

	now := float64(time.Now().Unix())
	b.packages.Put(&Package{
		PackageID:     packageID,
		PackageName:   packageName,
		PackageType:   packageType,
		PackageStatus: pkgStatusAvailable,
		CreatedAt:     now,
		VersionHistory: []*PackageVersionHistory{
			{
				PackageVersion: "1",
				CommitMessage:  "initial version",
				CreatedAt:      now,
			},
		},
	})
}
