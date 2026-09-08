package neptune

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) clusterParameterGroupGet(
	region, name string,
) (*DBClusterParameterGroup, bool) {
	return b.clusterParameterGroups.Get(regionKey(region, name))
}

func (b *InMemoryBackend) clusterParameterGroupHas(region, name string) bool {
	return b.clusterParameterGroups.Has(regionKey(region, name))
}

func (b *InMemoryBackend) clusterParameterGroupPut(v *DBClusterParameterGroup) {
	b.clusterParameterGroups.Put(v)
}

func (b *InMemoryBackend) clusterParameterGroupDelete(region, name string) {
	b.clusterParameterGroups.Delete(regionKey(region, name))
}

func (b *InMemoryBackend) clusterParameterGroupsInRegion(region string) []*DBClusterParameterGroup {
	return b.clusterParameterGroupsByRegion.Get(region)
}

// clusterParameterOverridesFor returns (creating if necessary) the parameter
// override map for a DB cluster parameter group. Callers must hold the write lock.
func (b *InMemoryBackend) clusterParameterOverridesFor(region, name string) map[string]ParameterValue {
	key := regionKey(region, name)
	if b.clusterParameterOverrides[key] == nil {
		b.clusterParameterOverrides[key] = make(map[string]ParameterValue)
	}

	return b.clusterParameterOverrides[key]
}

// clusterParameterGroupARN returns the region-scoped ARN for a Neptune DB cluster parameter group.
func (b *InMemoryBackend) clusterParameterGroupARN(region, name string) string {
	return arn.Build("rds", region, b.accountID, "cluster-pg:"+name)
}

// CreateDBClusterParameterGroup creates a Neptune DB cluster parameter group.
func (b *InMemoryBackend) CreateDBClusterParameterGroup(
	ctx context.Context,
	name, family, description string,
) (*DBClusterParameterGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: DBClusterParameterGroupName is required", ErrInvalidParameter)
	}
	if family == "" || !validNeptuneParameterGroupFamily(family) {
		return nil, fmt.Errorf(
			"%w: DBParameterGroupFamily %q is not valid; must be one of neptune1.2, neptune1.3, neptune1.4",
			ErrInvalidParameter,
			family,
		)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("CreateDBClusterParameterGroup")
	defer b.mu.Unlock()
	if b.clusterParameterGroupHas(region, name) {
		return nil, fmt.Errorf(
			"%w: cluster parameter group %s already exists",
			ErrClusterParameterGroupAlreadyExists,
			name,
		)
	}
	pg := &DBClusterParameterGroup{
		region:                      region,
		DBClusterParameterGroupName: name,
		DBClusterParameterGroupArn:  b.clusterParameterGroupARN(region, name),
		DBParameterGroupFamily:      family,
		Description:                 description,
	}
	b.clusterParameterGroupPut(pg)
	cp := *pg

	return &cp, nil
}

// DescribeDBClusterParameterGroups returns all Neptune cluster parameter groups or a specific one.
func (b *InMemoryBackend) DescribeDBClusterParameterGroups(
	ctx context.Context, name string,
) ([]DBClusterParameterGroup, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeDBClusterParameterGroups")
	defer b.mu.RUnlock()
	if name != "" {
		pg, exists := b.clusterParameterGroupGet(region, name)
		if !exists {
			return nil, fmt.Errorf(
				"%w: cluster parameter group %s not found",
				ErrClusterParameterGroupNotFound,
				name,
			)
		}
		cp := *pg

		return []DBClusterParameterGroup{cp}, nil
	}
	groups := b.clusterParameterGroupsInRegion(region)
	result := make([]DBClusterParameterGroup, 0, len(groups))
	for _, pg := range groups {
		result = append(result, *pg)
	}
	slices.SortFunc(result, func(a, b DBClusterParameterGroup) int {
		return strings.Compare(a.DBClusterParameterGroupName, b.DBClusterParameterGroupName)
	})

	return result, nil
}

// DeleteDBClusterParameterGroup deletes a Neptune DB cluster parameter group.
func (b *InMemoryBackend) DeleteDBClusterParameterGroup(ctx context.Context, name string) error {
	region := getRegion(ctx, b.region)
	b.mu.Lock("DeleteDBClusterParameterGroup")
	defer b.mu.Unlock()
	if !b.clusterParameterGroupHas(region, name) {
		return fmt.Errorf(
			"%w: cluster parameter group %s not found",
			ErrClusterParameterGroupNotFound,
			name,
		)
	}
	for _, c := range b.clustersInRegion(region) {
		if c.DBClusterParameterGroupName == name {
			return fmt.Errorf(
				"%w: cluster parameter group %s is used by cluster %s",
				ErrParameterGroupInUse,
				name,
				c.DBClusterIdentifier,
			)
		}
	}
	b.clusterParameterGroupDelete(region, name)
	delete(b.tagsStore(region), b.clusterParameterGroupARN(region, name))
	delete(b.clusterParameterOverrides, regionKey(region, name))

	return nil
}

// ModifyDBClusterParameterGroup applies parameter value overrides to a
// Neptune DB cluster parameter group -- see parameter_catalog.go for the
// validation/storage this now performs (previously a disguised no-op).
func (b *InMemoryBackend) ModifyDBClusterParameterGroup(
	ctx context.Context, name string, params []ParameterInput,
) (*DBClusterParameterGroup, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("ModifyDBClusterParameterGroup")
	defer b.mu.Unlock()
	pg, exists := b.clusterParameterGroupGet(region, name)
	if !exists {
		return nil, fmt.Errorf(
			"%w: cluster parameter group %s not found",
			ErrClusterParameterGroupNotFound,
			name,
		)
	}
	if err := applyParameterInputs(b.clusterParameterOverridesFor(region, name), params); err != nil {
		return nil, err
	}
	cp := *pg

	return &cp, nil
}

// DescribeDBClusterParameters returns the merged engine-default/user-override
// parameter list for a Neptune DB cluster parameter group.
func (b *InMemoryBackend) DescribeDBClusterParameters(
	ctx context.Context, name string,
) ([]EngineParameter, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeDBClusterParameters")
	defer b.mu.RUnlock()
	if !b.clusterParameterGroupHas(region, name) {
		return nil, fmt.Errorf(
			"%w: cluster parameter group %s not found",
			ErrClusterParameterGroupNotFound,
			name,
		)
	}

	return describeParameters(b.clusterParameterOverrides[regionKey(region, name)]), nil
}

// CopyDBClusterParameterGroup copies a Neptune DB cluster parameter group.
func (b *InMemoryBackend) CopyDBClusterParameterGroup(
	ctx context.Context,
	sourceName, targetName, targetDescription string,
) (*DBClusterParameterGroup, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("CopyDBClusterParameterGroup")
	defer b.mu.Unlock()
	src, err := copyPreconditions(
		func(n string) (*DBClusterParameterGroup, bool) { return b.clusterParameterGroupGet(region, n) },
		sourceName, targetName,
		"SourceDBClusterParameterGroupIdentifier is required",
		"TargetDBClusterParameterGroupIdentifier is required",
		ErrClusterParameterGroupNotFound, ErrClusterParameterGroupAlreadyExists,
	)
	if err != nil {
		return nil, err
	}
	pg := &DBClusterParameterGroup{
		region:                      region,
		DBClusterParameterGroupName: targetName,
		DBClusterParameterGroupArn:  b.clusterParameterGroupARN(region, targetName),
		DBParameterGroupFamily:      src.DBParameterGroupFamily,
		Description:                 resolveCopyDescription(targetDescription, src.Description),
	}
	b.clusterParameterGroupPut(pg)
	cp := *pg

	return &cp, nil
}

// ResetDBClusterParameterGroup resets a Neptune DB cluster parameter group's
// parameters to their engine-default values -- either all of them (resetAll)
// or only the named subset.
func (b *InMemoryBackend) ResetDBClusterParameterGroup(
	ctx context.Context, name string, resetAll bool, params []ParameterInput,
) (*DBClusterParameterGroup, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("ResetDBClusterParameterGroup")
	defer b.mu.Unlock()
	pg, exists := b.clusterParameterGroupGet(region, name)
	if !exists {
		return nil, fmt.Errorf(
			"%w: cluster parameter group %s not found",
			ErrClusterParameterGroupNotFound,
			name,
		)
	}
	if err := resetParameterInputs(b.clusterParameterOverridesFor(region, name), resetAll, params); err != nil {
		return nil, err
	}
	cp := *pg

	return &cp, nil
}

// AddClusterParameterGroupInternal creates a cluster parameter group directly. Used for seeding tests.
func (b *InMemoryBackend) AddClusterParameterGroupInternal(
	name, family string,
) *DBClusterParameterGroup {
	b.mu.Lock("AddClusterParameterGroupInternal")
	defer b.mu.Unlock()
	pg := &DBClusterParameterGroup{
		region:                      b.region,
		DBClusterParameterGroupName: name,
		DBParameterGroupFamily:      family,
		Description:                 "seeded for tests",
	}
	b.clusterParameterGroupPut(pg)
	cp := *pg

	return &cp
}
