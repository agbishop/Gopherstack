package memorydb

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// seedDefaultParameterGroupsLocked seeds the built-in default single-region and
// multi-region parameter groups. Caller must hold b.mu or be in the constructor.
func (b *InMemoryBackend) seedDefaultParameterGroupsLocked() {
	families := []struct {
		name   string
		family string
		desc   string
	}{
		{"default.memorydb-redis6", familyRedis6, "Default parameter group for MemoryDB Redis 6.2"},
		{"default.memorydb-redis7", familyRedis7, "Default parameter group for MemoryDB Redis 7.x"},
		{"default.memorydb-valkey7", familyValkey7, "Default parameter group for MemoryDB Valkey 7.x"},
		{"default.memorydb-valkey8", familyValkey8, "Default parameter group for MemoryDB Valkey 8.x"},
	}
	for _, f := range families {
		pgARN := arn.Build("memorydb", b.defaultRegion, b.accountID, "parametergroup/"+f.name)
		pg := &ParameterGroup{
			Name:        f.name,
			ARN:         pgARN,
			Description: f.desc,
			Family:      f.family,
			Parameters:  defaultParametersByFamily(f.family),
			Tags:        make(map[string]string),
			CreatedAt:   time.Now(),
		}
		b.parameterGroupsStore(b.defaultRegion).Put(pg)
		b.arnToResourceStore(b.defaultRegion)[pgARN] = resourceRef{Kind: resourceKindParameterGroup, Name: f.name}
	}

	mrFamilies := []struct {
		name   string
		family string
		desc   string
	}{
		{
			"default.memorydb-redis6.multiregion",
			familyRedis6,
			"Default multi-region parameter group for MemoryDB Redis 6.2",
		},
		{
			"default.memorydb-redis7.multiregion",
			familyRedis7,
			"Default multi-region parameter group for MemoryDB Redis 7.x",
		},
		{
			"default.memorydb-valkey7.multiregion",
			familyValkey7,
			"Default multi-region parameter group for MemoryDB Valkey 7.x",
		},
		{
			"default.memorydb-valkey8.multiregion",
			familyValkey8,
			"Default multi-region parameter group for MemoryDB Valkey 8.x",
		},
	}
	for _, f := range mrFamilies {
		mrARN := arn.Build("memorydb", b.defaultRegion, b.accountID, "multiregionparametergroup/"+f.name)
		mrpg := &MultiRegionParameterGroup{
			Name:        f.name,
			ARN:         mrARN,
			Description: f.desc,
			Family:      f.family,
			Parameters:  defaultParametersByFamily(f.family),
			Tags:        make(map[string]string),
			CreatedAt:   time.Now(),
		}
		b.multiRegionParameterGroups.Put(mrpg)
	}
}

// -- Cluster operations ----------------------------------------------------------

// CreateParameterGroup creates a new parameter group.
func (b *InMemoryBackend) CreateParameterGroup(
	ctx context.Context,
	req *createParameterGroupRequest,
) (*ParameterGroup, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	if req.Family == "" {
		return nil, fmt.Errorf("family is required: %w", ErrValidation)
	}

	if err := validateResourceName(req.ParameterGroupName, "parameter group"); err != nil {
		return nil, err
	}

	if _, exists := b.parameterGroupsStore(region).Get(req.ParameterGroupName); exists {
		return nil, ErrParameterGroupAlreadyExists
	}

	pgARN := arn.Build("memorydb", region, b.accountID, "parametergroup/"+req.ParameterGroupName)

	pg := &ParameterGroup{
		Name:        req.ParameterGroupName,
		ARN:         pgARN,
		Description: req.Description,
		Family:      req.Family,
		Parameters:  defaultParametersByFamily(req.Family),
		Tags:        tagsFromSlice(req.Tags),
		CreatedAt:   time.Now(),
	}

	b.parameterGroupsStore(region).Put(pg)
	b.arnToResourceStore(region)[pgARN] = resourceRef{Kind: resourceKindParameterGroup, Name: req.ParameterGroupName}

	// Return a clone, not the live table entry: the entry stays reachable for
	// concurrent UpdateParameterGroup/TagResource calls, which mutate its
	// Parameters/Tags maps in place under b.mu -- returning the raw pointer
	// would let the JSON encoder (which runs after this call returns and the
	// lock is released) race with those in-place mutations.
	return cloneParameterGroup(pg), nil
}

// DescribeParameterGroups returns parameter groups, optionally filtered by name.
func (b *InMemoryBackend) DescribeParameterGroups(ctx context.Context, name string) ([]*ParameterGroup, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	t := b.parameterGroups[region]

	if name != "" {
		pg, ok := tableGet(t, name)
		if !ok {
			return nil, ErrParameterGroupNotFound
		}

		return []*ParameterGroup{cloneParameterGroup(pg)}, nil
	}

	all := tableAll(t)
	result := make([]*ParameterGroup, 0, len(all))
	for _, pg := range all {
		result = append(result, cloneParameterGroup(pg))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// DeleteParameterGroup removes a parameter group.
func (b *InMemoryBackend) DeleteParameterGroup(ctx context.Context, name string) (*ParameterGroup, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	pg, ok := b.parameterGroupsStore(region).Get(name)
	if !ok {
		return nil, ErrParameterGroupNotFound
	}

	for _, c := range tableAll(b.clusters[region]) {
		if c.ParameterGroupName == name {
			return nil, fmt.Errorf(
				"parameter group %q is associated with cluster %q: %w",
				name, c.Name, ErrParameterGroupInUse,
			)
		}
	}

	b.parameterGroupsStore(region).Delete(name)
	delete(b.arnToResourceStore(region), pg.ARN)

	return pg, nil
}

// UpdateParameterGroup modifies parameter values in a parameter group.
func (b *InMemoryBackend) UpdateParameterGroup(
	ctx context.Context,
	req *updateParameterGroupRequest,
) (*ParameterGroup, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	pg, ok := b.parameterGroupsStore(region).Get(req.ParameterGroupName)
	if !ok {
		return nil, ErrParameterGroupNotFound
	}

	for _, pnv := range req.ParameterNameValues {
		pg.Parameters[pnv.ParameterName] = pnv.ParameterValue
	}

	return cloneParameterGroup(pg), nil
}

// -- Tag operations --------------------------------------------------------------

// defaultParametersByFamily returns the built-in parameter defaults for each engine family.
func defaultParametersByFamily(family string) map[string]string {
	base := map[string]string{
		"maxmemory-policy":              "noeviction",
		"timeout":                       "0",
		"tcp-keepalive":                 "300",
		"lazyfree-lazy-eviction":        "no",
		"lazyfree-lazy-expire":          "no",
		"lazyfree-lazy-server-del":      "no",
		"replica-lazy-flush":            "no",
		"activedefrag":                  "no",
		"active-expire-enabled":         "1",
		"active-expire-effort":          "1",
		"lfu-log-factor":                "10",
		"lfu-decay-time":                "1",
		"hash-max-listpack-entries":     "128",
		"hash-max-listpack-value":       "64",
		"list-max-listpack-size":        "-2",
		"list-compress-depth":           "0",
		"set-max-intset-entries":        "512",
		"zset-max-listpack-entries":     "128",
		"zset-max-listpack-value":       "64",
		"activerehashing":               paramValueYes,
		"hz":                            "10",
		"dynamic-hz":                    paramValueYes,
		"aof-rewrite-incremental-fsync": paramValueYes,
		"rdb-save-incremental-fsync":    paramValueYes,
		"jemalloc-bg-thread":            paramValueYes,
		"close-on-slave-write":          paramValueYes,
		"repl-backlog-size":             "1048576",
		"repl-backlog-ttl":              "3600",
		"slowlog-log-slower-than":       "10000",
		"slowlog-max-len":               "128",
		"latency-monitor-threshold":     "0",
		"tracking-table-max-keys":       "0",
		"list-max-ziplist-size":         "-2",
		"cluster-node-timeout":          "15000",
		"cluster-migration-barrier":     "1",
		"cluster-require-full-coverage": paramValueYes,
		"cluster-allow-reads-when-down": "no",
	}
	_ = family // family-specific overrides could go here

	return base
}

// DescribeParameters returns the parameters map for a given parameter group.
func (b *InMemoryBackend) DescribeParameters(
	ctx context.Context,
	parameterGroupName string,
) (map[string]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)

	if parameterGroupName == "" {
		return nil, fmt.Errorf("parameter group name is required: %w", ErrValidation)
	}

	pg, ok := tableGet(b.parameterGroups[region], parameterGroupName)
	if !ok {
		return nil, ErrParameterGroupNotFound
	}

	return maps.Clone(pg.Parameters), nil
}

// ResetParameterGroup resets parameters in a parameter group back to family defaults.
// If parameterNames is non-empty and allParameters is false, only those keys are reset.
// If allParameters is true or parameterNames is empty, all parameters are reset.
func (b *InMemoryBackend) ResetParameterGroup(
	ctx context.Context,
	name string,
	parameterNames []string,
	allParameters bool,
) (*ParameterGroup, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)

	pg, ok := b.parameterGroupsStore(region).Get(name)
	if !ok {
		return nil, ErrParameterGroupNotFound
	}

	defaults := defaultParametersByFamily(pg.Family)

	if len(parameterNames) > 0 && !allParameters {
		for _, pn := range parameterNames {
			if dv, found := defaults[pn]; found {
				pg.Parameters[pn] = dv
			} else {
				delete(pg.Parameters, pn)
			}
		}
	} else {
		pg.Parameters = maps.Clone(defaults)
	}

	return cloneParameterGroup(pg), nil
}

// -- Shard operations -----------------------------------------------------------

// cloneParameterGroup returns a shallow copy of the parameter group with separate maps.
func cloneParameterGroup(pg *ParameterGroup) *ParameterGroup {
	if pg == nil {
		return nil
	}

	cp := *pg
	cp.Tags = maps.Clone(pg.Tags)
	cp.Parameters = maps.Clone(pg.Parameters)

	return &cp
}

// AddParameterGroupInternal inserts a parameter group directly into the backend for testing.
func (b *InMemoryBackend) AddParameterGroupInternal(name, family string) *ParameterGroup {
	b.mu.Lock()
	defer b.mu.Unlock()

	pgARN := arn.Build("memorydb", b.defaultRegion, b.accountID, "parametergroup/"+name)
	pg := &ParameterGroup{
		Name:       name,
		ARN:        pgARN,
		Family:     family,
		Parameters: make(map[string]string),
		Tags:       make(map[string]string),
		CreatedAt:  time.Now(),
	}
	b.parameterGroupsStore(b.defaultRegion).Put(pg)
	b.arnToResourceStore(b.defaultRegion)[pgARN] = resourceRef{Kind: resourceKindParameterGroup, Name: name}

	return pg
}
