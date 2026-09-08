package neptune

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) parameterGroupGet(region, name string) (*DBParameterGroup, bool) {
	return b.parameterGroups.Get(regionKey(region, name))
}

func (b *InMemoryBackend) parameterGroupHas(region, name string) bool {
	return b.parameterGroups.Has(regionKey(region, name))
}

func (b *InMemoryBackend) parameterGroupPut(v *DBParameterGroup) { b.parameterGroups.Put(v) }

func (b *InMemoryBackend) parameterGroupDelete(region, name string) {
	b.parameterGroups.Delete(regionKey(region, name))
}

func (b *InMemoryBackend) parameterGroupsInRegion(region string) []*DBParameterGroup {
	return b.parameterGroupsByRegion.Get(region)
}

// parameterOverridesFor returns (creating if necessary) the parameter
// override map for a DB parameter group. Callers must hold the write lock.
func (b *InMemoryBackend) parameterOverridesFor(region, name string) map[string]ParameterValue {
	key := regionKey(region, name)
	if b.parameterOverrides[key] == nil {
		b.parameterOverrides[key] = make(map[string]ParameterValue)
	}

	return b.parameterOverrides[key]
}

// parameterGroupARN returns the region-scoped ARN for a Neptune DB parameter group.
func (b *InMemoryBackend) parameterGroupARN(region, name string) string {
	return arn.Build("rds", region, b.accountID, "pg:"+name)
}

// CopyDBParameterGroup copies a Neptune DB parameter group.
func (b *InMemoryBackend) CopyDBParameterGroup(
	ctx context.Context,
	sourceName, targetName, targetDescription string,
) (*DBParameterGroup, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("CopyDBParameterGroup")
	defer b.mu.Unlock()
	src, err := copyPreconditions(
		func(n string) (*DBParameterGroup, bool) { return b.parameterGroupGet(region, n) },
		sourceName, targetName,
		"SourceDBParameterGroupIdentifier is required",
		"TargetDBParameterGroupIdentifier is required",
		ErrParameterGroupNotFound, ErrParameterGroupAlreadyExists,
	)
	if err != nil {
		return nil, err
	}
	pg := &DBParameterGroup{
		region:                 region,
		DBParameterGroupName:   targetName,
		DBParameterGroupArn:    b.parameterGroupARN(region, targetName),
		DBParameterGroupFamily: src.DBParameterGroupFamily,
		Description:            resolveCopyDescription(targetDescription, src.Description),
	}
	b.parameterGroupPut(pg)
	cp := *pg

	return &cp, nil
}

// CreateDBParameterGroup creates a Neptune DB parameter group.
func (b *InMemoryBackend) CreateDBParameterGroup(
	ctx context.Context, name, family, description string,
) (*DBParameterGroup, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: DBParameterGroupName is required", ErrInvalidParameter)
	}
	if family == "" || !validNeptuneParameterGroupFamily(family) {
		return nil, fmt.Errorf(
			"%w: DBParameterGroupFamily %q is not valid; must be one of neptune1.2, neptune1.3, neptune1.4",
			ErrInvalidParameter,
			family,
		)
	}
	region := getRegion(ctx, b.region)
	b.mu.Lock("CreateDBParameterGroup")
	defer b.mu.Unlock()
	if b.parameterGroupHas(region, name) {
		return nil, fmt.Errorf(
			"%w: parameter group %s already exists",
			ErrParameterGroupAlreadyExists,
			name,
		)
	}
	pg := &DBParameterGroup{
		region:                 region,
		DBParameterGroupName:   name,
		DBParameterGroupArn:    b.parameterGroupARN(region, name),
		DBParameterGroupFamily: family,
		Description:            description,
	}
	b.parameterGroupPut(pg)
	cp := *pg

	return &cp, nil
}

// DeleteDBParameterGroup deletes a Neptune DB parameter group.
func (b *InMemoryBackend) DeleteDBParameterGroup(ctx context.Context, name string) error {
	region := getRegion(ctx, b.region)
	b.mu.Lock("DeleteDBParameterGroup")
	defer b.mu.Unlock()
	if !b.parameterGroupHas(region, name) {
		return fmt.Errorf("%w: parameter group %s not found", ErrParameterGroupNotFound, name)
	}
	for _, inst := range b.instancesInRegion(region) {
		if inst.DBParameterGroupName == name {
			return fmt.Errorf(
				"%w: parameter group %s is used by instance %s",
				ErrParameterGroupInUse,
				name,
				inst.DBInstanceIdentifier,
			)
		}
	}
	b.parameterGroupDelete(region, name)
	delete(b.tagsStore(region), b.parameterGroupARN(region, name))
	delete(b.parameterOverrides, regionKey(region, name))

	return nil
}

// DescribeDBParameterGroups returns all Neptune DB parameter groups or a specific one.
func (b *InMemoryBackend) DescribeDBParameterGroups(
	ctx context.Context,
	name string,
) ([]DBParameterGroup, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeDBParameterGroups")
	defer b.mu.RUnlock()
	if name != "" {
		pg, exists := b.parameterGroupGet(region, name)
		if !exists {
			return nil, fmt.Errorf(
				"%w: parameter group %s not found",
				ErrParameterGroupNotFound,
				name,
			)
		}
		cp := *pg

		return []DBParameterGroup{cp}, nil
	}
	groups := b.parameterGroupsInRegion(region)
	result := make([]DBParameterGroup, 0, len(groups))
	for _, pg := range groups {
		result = append(result, *pg)
	}
	slices.SortFunc(result, func(a, b DBParameterGroup) int {
		return strings.Compare(a.DBParameterGroupName, b.DBParameterGroupName)
	})

	return result, nil
}

// ModifyDBParameterGroup applies parameter value overrides to a Neptune DB
// parameter group -- see parameter_catalog.go for the validation/storage
// this now performs (previously a disguised no-op: params were accepted and
// discarded).
func (b *InMemoryBackend) ModifyDBParameterGroup(
	ctx context.Context,
	name string,
	params []ParameterInput,
) (*DBParameterGroup, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("ModifyDBParameterGroup")
	defer b.mu.Unlock()
	pg, exists := b.parameterGroupGet(region, name)
	if !exists {
		return nil, fmt.Errorf("%w: parameter group %s not found", ErrParameterGroupNotFound, name)
	}
	if err := applyParameterInputs(b.parameterOverridesFor(region, name), params); err != nil {
		return nil, err
	}
	cp := *pg

	return &cp, nil
}

// ResetDBParameterGroup resets a Neptune DB parameter group's parameters to
// their engine-default values -- either all of them (resetAll) or only the
// named subset.
func (b *InMemoryBackend) ResetDBParameterGroup(
	ctx context.Context,
	name string,
	resetAll bool,
	params []ParameterInput,
) (*DBParameterGroup, error) {
	region := getRegion(ctx, b.region)
	b.mu.Lock("ResetDBParameterGroup")
	defer b.mu.Unlock()
	pg, exists := b.parameterGroupGet(region, name)
	if !exists {
		return nil, fmt.Errorf("%w: parameter group %s not found", ErrParameterGroupNotFound, name)
	}
	if err := resetParameterInputs(b.parameterOverridesFor(region, name), resetAll, params); err != nil {
		return nil, err
	}
	cp := *pg

	return &cp, nil
}

// DescribeDBParameters returns the merged engine-default/user-override
// parameter list for a Neptune DB parameter group.
func (b *InMemoryBackend) DescribeDBParameters(ctx context.Context, name string) ([]EngineParameter, error) {
	region := getRegion(ctx, b.region)
	b.mu.RLock("DescribeDBParameters")
	defer b.mu.RUnlock()
	if !b.parameterGroupHas(region, name) {
		return nil, fmt.Errorf("%w: parameter group %s not found", ErrParameterGroupNotFound, name)
	}

	return describeParameters(b.parameterOverrides[regionKey(region, name)]), nil
}

// AddParameterGroupInternal creates a DB parameter group directly. Used for seeding tests.
func (b *InMemoryBackend) AddParameterGroupInternal(name, family string) *DBParameterGroup {
	b.mu.Lock("AddParameterGroupInternal")
	defer b.mu.Unlock()
	pg := &DBParameterGroup{
		region:                 b.region,
		DBParameterGroupName:   name,
		DBParameterGroupFamily: family,
		Description:            "seeded for tests",
	}
	b.parameterGroupPut(pg)
	cp := *pg

	return &cp
}
