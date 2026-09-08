package bedrockagent

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"
)

// ---------------------------------------------------------------------------
// Flow CRUD
// ---------------------------------------------------------------------------

// CreateFlow creates a new flow.
func (b *InMemoryBackend) CreateFlow(ctx context.Context, cfg FlowConfig) (*Flow, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}

	region := ctxRegion(ctx, b.defaultRegion)

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.flowsByName[cfg.Name]; exists {
		return nil, fmt.Errorf("%w: flow %q already exists", ErrAlreadyExists, cfg.Name)
	}

	id := b.nextID("flow", &b.flowCounter)
	now := time.Now().UTC()

	f := &Flow{
		FlowID:      id,
		FlowARN:     b.buildFlowARN(region, id),
		Name:        cfg.Name,
		Status:      flowStatusNotPrepared,
		Description: cfg.Description,
		RoleARN:     cfg.RoleARN,
		Definition:  cfg.Definition,
		Version:     "DRAFT",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	b.flows.Put(f)
	b.flowsByName[cfg.Name] = id
	b.tags[f.FlowARN] = maps.Clone(cfg.Tags)

	return flowCopy(f), nil
}

// GetFlow returns a flow.
func (b *InMemoryBackend) GetFlow(_ context.Context, flowID string) (*Flow, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	f, ok := b.flows.Get(flowID)
	if !ok {
		return nil, fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	return flowCopy(f), nil
}

// UpdateFlow updates a flow.
func (b *InMemoryBackend) UpdateFlow(_ context.Context, flowID string, cfg FlowConfig) (*Flow, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	f, ok := b.flows.Get(flowID)
	if !ok {
		return nil, fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	applyFlowConfig(f, cfg)
	f.UpdatedAt = time.Now().UTC()

	return flowCopy(f), nil
}

func applyFlowConfig(f *Flow, cfg FlowConfig) {
	if cfg.Name != "" {
		f.Name = cfg.Name
	}

	if cfg.Description != "" {
		f.Description = cfg.Description
	}

	if cfg.RoleARN != "" {
		f.RoleARN = cfg.RoleARN
	}

	if cfg.Definition != nil {
		f.Definition = cfg.Definition
	}
}

// DeleteFlow deletes a flow.
func (b *InMemoryBackend) DeleteFlow(_ context.Context, flowID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	f, ok := b.flows.Get(flowID)
	if !ok {
		return fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	delete(b.flowsByName, f.Name)
	b.flows.Delete(flowID)
	delete(b.tags, f.FlowARN)

	for _, fv := range slices.Clone(b.flowVersionsByFlow.Get(flowID)) {
		b.flowVersions.Delete(flowVersionKey(fv.FlowID, fv.Version))
	}

	delete(b.flowVersionCtrs, flowID)

	for _, al := range slices.Clone(b.flowAliasesByFlow.Get(flowID)) {
		b.flowAliases.Delete(flowAliasKey(al.FlowID, al.AliasID))
		delete(b.tags, al.AliasARN)
	}

	return nil
}

// ListFlows returns paginated flow summaries.
func (b *InMemoryBackend) ListFlows(
	_ context.Context, maxResults int, nextToken string,
) ([]*FlowSummary, string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	ids := tableIDs(b.flows.Snapshot(), func(f *Flow) string { return f.FlowID })
	ids, outToken := paginate(ids, nextToken, maxResults)

	out := make([]*FlowSummary, 0, len(ids))

	for _, id := range ids {
		f, _ := b.flows.Get(id)
		out = append(out, &FlowSummary{
			FlowID:      f.FlowID,
			FlowARN:     f.FlowARN,
			Name:        f.Name,
			Status:      f.Status,
			Description: f.Description,
			Version:     f.Version,
			CreatedAt:   f.CreatedAt,
			UpdatedAt:   f.UpdatedAt,
		})
	}

	return out, outToken, nil
}

// PrepareFlow transitions a flow to prepared status.
func (b *InMemoryBackend) PrepareFlow(_ context.Context, flowID string) (*Flow, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	f, ok := b.flows.Get(flowID)
	if !ok {
		return nil, fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	f.Status = flowStatusPrepared
	f.UpdatedAt = time.Now().UTC()

	return flowCopy(f), nil
}

// ValidateFlowDefinition validates a flow definition (stub - always passes).
func (b *InMemoryBackend) ValidateFlowDefinition(
	_ context.Context, _ map[string]any,
) ([]FlowValidationError, error) {
	return []FlowValidationError{}, nil
}

// ---------------------------------------------------------------------------
// Flow version CRUD
// ---------------------------------------------------------------------------

// CreateFlowVersion creates a numbered snapshot of a flow.
func (b *InMemoryBackend) CreateFlowVersion(
	_ context.Context, flowID, description string,
) (*FlowVersion, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	f, ok := b.flows.Get(flowID)
	if !ok {
		return nil, fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	b.flowVersionCtrs[flowID]++
	vNum := b.flowVersionCtrs[flowID]
	version := strconv.Itoa(vNum)

	fv := &FlowVersion{
		FlowID:      flowID,
		FlowARN:     f.FlowARN,
		Name:        f.Name,
		Version:     version,
		Status:      flowStatusPrepared,
		Definition:  f.Definition,
		Description: description,
		CreatedAt:   time.Now().UTC(),
		RoleARN:     f.RoleARN,
	}

	b.flowVersions.Put(fv)

	return flowVersionCopy(fv), nil
}

// GetFlowVersion returns a flow version. See the not-found precedence note
// on GetAgentVersion in the Agent version CRUD section above -- the same
// b.flows.Has(flowID)-instead-of-inner-map-presence reasoning applies here.
func (b *InMemoryBackend) GetFlowVersion(
	_ context.Context, flowID, flowVersion string,
) (*FlowVersion, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.flows.Has(flowID) {
		return nil, fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	fv, ok := b.flowVersions.Get(flowVersionKey(flowID, flowVersion))
	if !ok {
		return nil, fmt.Errorf("%w: flow version %q not found", ErrNotFound, flowVersion)
	}

	return flowVersionCopy(fv), nil
}

// DeleteFlowVersion deletes a flow version.
//
// Real AWS (api_op_DeleteFlowVersion.go): "By default, this value is false
// and deletion is stopped if the resource is in use. If you set it to true,
// the resource will be deleted even if the resource is in use." A flow
// version is "in use" when a flow alias's routingConfiguration still points
// at it (types.FlowAliasRoutingConfigurationListItem.FlowVersion) -- the
// only wire-visible relationship a flow version participates in.
func (b *InMemoryBackend) DeleteFlowVersion(
	_ context.Context, flowID, flowVersion string, skipResourceInUseCheck bool,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.flows.Has(flowID) {
		return fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	key := flowVersionKey(flowID, flowVersion)
	if !b.flowVersions.Has(key) {
		return fmt.Errorf("%w: flow version %q not found", ErrNotFound, flowVersion)
	}

	if !skipResourceInUseCheck {
		for _, al := range b.flowAliasesByFlow.Get(flowID) {
			for _, r := range al.RoutingConfiguration {
				if r.FlowVersion == flowVersion {
					return fmt.Errorf(
						"%w: flow version %q is referenced by alias %q",
						ErrResourceInUse, flowVersion, al.AliasID,
					)
				}
			}
		}
	}

	b.flowVersions.Delete(key)

	return nil
}

// ListFlowVersions returns paginated flow version summaries.
func (b *InMemoryBackend) ListFlowVersions(
	_ context.Context, flowID string, maxResults int, nextToken string,
) ([]*FlowVersionSummary, string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.flows.Has(flowID) {
		return nil, "", fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	group := b.flowVersionsByFlow.Get(flowID)
	keys := tableIDs(group, func(fv *FlowVersion) string { return fv.Version })
	keys, outToken := paginate(keys, nextToken, maxResults)

	out := make([]*FlowVersionSummary, 0, len(keys))

	for _, k := range keys {
		fv, _ := b.flowVersions.Get(flowVersionKey(flowID, k))
		out = append(out, &FlowVersionSummary{
			FlowID:    fv.FlowID,
			Arn:       fv.FlowARN,
			Version:   fv.Version,
			Status:    fv.Status,
			CreatedAt: fv.CreatedAt,
		})
	}

	return out, outToken, nil
}

// ---------------------------------------------------------------------------
// Flow alias CRUD
// ---------------------------------------------------------------------------

func flowAliasKey(flowID, aliasID string) string { return flowID + "/" + aliasID }

// CreateFlowAlias creates a flow alias.
func (b *InMemoryBackend) CreateFlowAlias(
	ctx context.Context, flowID string, cfg FlowAliasConfig,
) (*FlowAlias, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}

	region := ctxRegion(ctx, b.defaultRegion)

	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.flows.Has(flowID) {
		return nil, fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	id := b.nextID("falias", &b.flowAliasCounter)
	now := time.Now().UTC()

	al := &FlowAlias{
		AliasID:              id,
		AliasARN:             b.buildFlowAliasARN(region, flowID, id),
		FlowID:               flowID,
		Name:                 cfg.Name,
		Description:          cfg.Description,
		RoutingConfiguration: cfg.RoutingConfiguration,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	b.flowAliases.Put(al)
	// Real AWS: CreateFlowAliasInput accepts a "tags" member, but
	// CreateFlowAliasOutput never echoes tags back -- readable only via
	// ListTagsForResource(AliasArn).
	b.tags[al.AliasARN] = maps.Clone(cfg.Tags)

	return flowAliasCopy(al), nil
}

// GetFlowAlias returns a flow alias.
func (b *InMemoryBackend) GetFlowAlias(_ context.Context, flowID, aliasID string) (*FlowAlias, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	al, ok := b.flowAliases.Get(flowAliasKey(flowID, aliasID))
	if !ok {
		return nil, fmt.Errorf("%w: flow alias %q not found", ErrNotFound, aliasID)
	}

	return flowAliasCopy(al), nil
}

// UpdateFlowAlias updates a flow alias.
func (b *InMemoryBackend) UpdateFlowAlias(
	_ context.Context, flowID, aliasID string, cfg FlowAliasConfig,
) (*FlowAlias, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	al, ok := b.flowAliases.Get(flowAliasKey(flowID, aliasID))
	if !ok {
		return nil, fmt.Errorf("%w: flow alias %q not found", ErrNotFound, aliasID)
	}

	if cfg.Name != "" {
		al.Name = cfg.Name
	}

	if cfg.Description != "" {
		al.Description = cfg.Description
	}

	if cfg.RoutingConfiguration != nil {
		al.RoutingConfiguration = cfg.RoutingConfiguration
	}

	al.UpdatedAt = time.Now().UTC()

	return flowAliasCopy(al), nil
}

// DeleteFlowAlias deletes a flow alias.
func (b *InMemoryBackend) DeleteFlowAlias(_ context.Context, flowID, aliasID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := flowAliasKey(flowID, aliasID)

	al, ok := b.flowAliases.Get(key)
	if !ok {
		return fmt.Errorf("%w: flow alias %q not found", ErrNotFound, aliasID)
	}

	b.flowAliases.Delete(key)
	delete(b.tags, al.AliasARN)

	return nil
}

// ListFlowAliases returns paginated flow alias summaries.
func (b *InMemoryBackend) ListFlowAliases(
	_ context.Context, flowID string, maxResults int, nextToken string,
) ([]*FlowAliasSummary, string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	group := b.flowAliasesByFlow.Get(flowID)
	ids := tableIDs(group, func(al *FlowAlias) string { return al.AliasID })
	ids, outToken := paginate(ids, nextToken, maxResults)

	out := make([]*FlowAliasSummary, 0, len(ids))

	for _, id := range ids {
		al, _ := b.flowAliases.Get(flowAliasKey(flowID, id))
		out = append(out, &FlowAliasSummary{
			AliasID:     al.AliasID,
			AliasARN:    al.AliasARN,
			FlowID:      al.FlowID,
			Name:        al.Name,
			Description: al.Description,
			CreatedAt:   al.CreatedAt,
			UpdatedAt:   al.UpdatedAt,
		})
	}

	return out, outToken, nil
}

func flowCopy(f *Flow) *Flow {
	cp := *f

	return &cp
}

func flowVersionCopy(fv *FlowVersion) *FlowVersion {
	cp := *fv

	return &cp
}

func flowAliasCopy(al *FlowAlias) *FlowAlias {
	cp := *al

	if al.RoutingConfiguration != nil {
		cp.RoutingConfiguration = append([]FlowAliasRouting{}, al.RoutingConfiguration...)
	}

	return &cp
}
