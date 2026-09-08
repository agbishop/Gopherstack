package codepipeline

import (
	"context"
	"fmt"
	"maps"
	"sort"
)

// CreateCustomActionType stores a new custom action type.
//
// gopherstack-wlab: the guard below emits InvalidStructureException on a
// duplicate category/provider/version -- not in this op's declared error
// set per botocore codepipeline/2015-07-09/service-2.json
// (ConcurrentModificationException/InvalidTagsException/
// LimitExceededException/TooManyTagsException/ValidationException only).
// Left unfixed: no declared code's doc text fits "duplicate", and the
// idempotent-upsert theory tried in gopherstack-3djp doesn't hold (see
// DeleteCustomActionType). NOTE: this does not break errors.As for a real
// caller -- see undeclared_error_codes_test.go.
func (b *InMemoryBackend) CreateCustomActionType(
	ctx context.Context,
	cat *CustomActionType,
) (*CustomActionType, error) {
	b.mu.Lock("CreateCustomActionType")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	catKey := customActionTypeKey{Category: cat.Category, Provider: cat.Provider, Version: cat.Version}
	key := regionKey(region, catKey.String())

	if b.customActionTypes.Has(key) {
		return nil, fmt.Errorf("%w: custom action type %q/%q/%q already exists",
			ErrAlreadyExists, cat.Category, cat.Provider, cat.Version)
	}

	if cat.Owner == "" {
		cat.Owner = keyOwnerCustom
	}

	cp := copyCustomActionType(cat)
	cp.region = region
	cp.arn = b.buildActionTypeARN(region, cp)
	b.customActionTypes.Put(cp)

	return copyCustomActionType(cp), nil
}

// DeleteCustomActionType removes a custom action type.
//
// gopherstack-wlab: the guard below emits ActionTypeNotFoundException on a
// nonexistent type -- not in this op's declared error set per botocore
// codepipeline/2015-07-09/service-2.json (ConcurrentModificationException/
// ValidationException only). Left unfixed: the "HTTP 200, empty body"
// doc sentence cited in gopherstack-3djp for an idempotent-delete fix is
// generic Response-shape boilerplate (also appears on DisableStageTransition,
// which DOES throw PipelineNotFoundException on a missing pipeline) --
// not evidence either way. No declared code fits "not found". NOTE: this
// does not break errors.As for a real caller -- see
// undeclared_error_codes_test.go.
// Returns ResourceInUseException if any pipeline references the type.
func (b *InMemoryBackend) DeleteCustomActionType(ctx context.Context, category, provider, version string) error {
	b.mu.Lock("DeleteCustomActionType")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	key := regionKey(region, customActionTypeKey{Category: category, Provider: provider, Version: version}.String())

	if !b.customActionTypes.Has(key) {
		return fmt.Errorf("%w: custom action type %q/%q/%q", ErrActionTypeNotFound, category, provider, version)
	}

	// Check that no pipeline references this action type.
	for _, p := range b.pipelinesByRegion.Get(region) {
		for _, stage := range p.Declaration.Stages {
			for _, action := range stage.Actions {
				at := action.ActionTypeID
				if at.Category == category && at.Provider == provider && at.Version == version {
					return fmt.Errorf("%w: action type %q/%q/%q is in use by pipeline %q",
						ErrResourceInUse, category, provider, version, p.Declaration.Name)
				}
			}
		}
	}

	b.customActionTypes.Delete(key)

	return nil
}

// GetActionType retrieves a custom action type.
func (b *InMemoryBackend) GetActionType(
	ctx context.Context,
	category, owner, provider, version string,
) (*CustomActionType, error) {
	b.mu.RLock("GetActionType")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	key := regionKey(region, customActionTypeKey{Category: category, Provider: provider, Version: version}.String())

	cat, ok := b.customActionTypes.Get(key)
	if !ok {
		return nil, fmt.Errorf("%w: action type %q/%q/%q/%q", ErrActionTypeNotFound, category, owner, provider, version)
	}

	return copyCustomActionType(cat), nil
}

// AddCustomActionTypeInternal seeds a custom action type into the backend's default region (for testing).
func (b *InMemoryBackend) AddCustomActionTypeInternal(cat *CustomActionType) {
	b.mu.Lock("AddCustomActionTypeInternal")
	defer b.mu.Unlock()

	cp := copyCustomActionType(cat)
	cp.region = b.region
	cp.arn = b.buildActionTypeARN(b.region, cp)
	b.customActionTypes.Put(cp)
}

// copyActionTypeExecutorConfiguration deep-copies an ActionTypeExecutorConfiguration,
// including its (at most one, per the real API) populated JobWorker/Lambda variant.
func copyActionTypeExecutorConfiguration(c *ActionTypeExecutorConfiguration) *ActionTypeExecutorConfiguration {
	if c == nil {
		return nil
	}

	conf := *c

	if c.JobWorkerExecutorConfiguration != nil {
		jw := *c.JobWorkerExecutorConfiguration
		jw.PollingAccounts = append([]string(nil), jw.PollingAccounts...)
		jw.PollingServicePrincipals = append([]string(nil), jw.PollingServicePrincipals...)
		conf.JobWorkerExecutorConfiguration = &jw
	}

	if c.LambdaExecutorConfiguration != nil {
		lc := *c.LambdaExecutorConfiguration
		conf.LambdaExecutorConfiguration = &lc
	}

	return &conf
}

// copyActionTypeExecutor deep-copies an ActionTypeExecutor (the Executor member
// of an ActionTypeDeclaration), or returns nil for a record with no declaration
// data (never touched by UpdateActionType).
func copyActionTypeExecutor(e *ActionTypeExecutor) *ActionTypeExecutor {
	if e == nil {
		return nil
	}

	cp := *e
	cp.Configuration = copyActionTypeExecutorConfiguration(e.Configuration)

	if e.JobTimeout != nil {
		jt := *e.JobTimeout
		cp.JobTimeout = &jt
	}

	return &cp
}

func copyCustomActionType(c *CustomActionType) *CustomActionType {
	cp := *c

	if c.Tags != nil {
		cp.Tags = make(map[string]string, len(c.Tags))
		maps.Copy(cp.Tags, c.Tags)
	}

	if c.ConfigurationProperties != nil {
		cp.ConfigurationProperties = make([]ActionConfigurationProperty, len(c.ConfigurationProperties))
		copy(cp.ConfigurationProperties, c.ConfigurationProperties)
	}

	if c.Settings != nil {
		s := *c.Settings
		cp.Settings = &s
	}

	cp.Executor = copyActionTypeExecutor(c.Executor)

	if c.Permissions != nil {
		p := *c.Permissions
		p.AllowedAccounts = append([]string(nil), p.AllowedAccounts...)
		cp.Permissions = &p
	}

	if c.Properties != nil {
		cp.Properties = make([]ActionTypeProperty, len(c.Properties))
		copy(cp.Properties, c.Properties)
	}

	if c.Urls != nil {
		u := *c.Urls
		cp.Urls = &u
	}

	return &cp
}

// UpdateActionType updates an action type using the real ActionTypeDeclaration
// shape (Description/Executor/InputArtifactDetails/OutputArtifactDetails/
// Permissions/Properties/Urls). Only fields ActionTypeDeclaration can express are
// replaced; the legacy CreateCustomActionType-era Settings/ConfigurationProperties
// (and Tags) have no equivalent in this op's real input and are preserved
// unchanged on the existing record -- AWS's real UpdateActionType has no way to
// clear data its own input shape can't carry.
func (b *InMemoryBackend) UpdateActionType(ctx context.Context, upd *CustomActionType) error {
	b.mu.Lock("UpdateActionType")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	key := regionKey(region, customActionTypeKey{
		Category: upd.Category,
		Provider: upd.Provider,
		Version:  upd.Version,
	}.String())

	existing, ok := b.customActionTypes.Get(key)
	if !ok {
		return ErrActionTypeNotFound
	}

	cp := copyCustomActionType(existing)
	cp.region = region
	cp.Description = upd.Description
	cp.Executor = upd.Executor
	cp.InputArtifactDetails = upd.InputArtifactDetails
	cp.OutputArtifactDetails = upd.OutputArtifactDetails
	cp.Permissions = upd.Permissions
	cp.Properties = upd.Properties
	cp.Urls = upd.Urls

	cp = copyCustomActionType(cp)
	b.customActionTypes.Put(cp)

	return nil
}

// ListActionTypes returns all registered action types in the request region.
func (b *InMemoryBackend) ListActionTypes(ctx context.Context) []*CustomActionType {
	b.mu.RLock("ListActionTypes")
	defer b.mu.RUnlock()

	entries := b.customActionTypesByRegion.Get(getRegion(ctx, b.region))

	result := make([]*CustomActionType, 0, len(entries))

	for _, cat := range entries {
		result = append(result, copyCustomActionType(cat))
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Provider < result[j].Provider
	})

	return result
}
