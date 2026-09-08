package dms

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// CreateDataProvider creates a new data provider.
func (b *InMemoryBackend) CreateDataProvider(
	ctx context.Context,
	name, engine, description string,
	kv map[string]string,
) (*DataProvider, error) {
	b.mu.Lock("CreateDataProvider")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if b.dataProviders.Has(regionKey(region, name)) {
		return nil, fmt.Errorf("%w: data provider %s already exists", ErrAlreadyExists, name)
	}

	providerARN := arn.Build("dms", region, b.accountID, "data-provider:"+uuid.NewString())
	t := tags.New("dms.data-provider." + name + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}

	now := time.Now().UTC()
	dp := &DataProvider{
		DataProviderName: name,
		DataProviderArn:  providerARN,
		Engine:           engine,
		Description:      description,
		AccountID:        b.accountID,
		Region:           region,
		CreationTime:     now,
		Tags:             t,
	}
	b.dataProviders.Put(dp)
	cp := *dp

	return &cp, nil
}

// AddDataProviderInternal seeds a data provider directly without HTTP.
func (b *InMemoryBackend) AddDataProviderInternal(name, engine string) {
	b.mu.Lock("AddDataProviderInternal")
	defer b.mu.Unlock()
	providerARN := arn.Build("dms", b.region, b.accountID, "data-provider:"+uuid.NewString())
	t := tags.New("dms.data-provider." + name + ".tags")
	now := time.Now().UTC()
	dp := &DataProvider{
		DataProviderName: name,
		DataProviderArn:  providerARN,
		Engine:           engine,
		AccountID:        b.accountID,
		Region:           b.region,
		CreationTime:     now,
		Tags:             t,
	}
	b.dataProviders.Put(dp)
}

// DeleteDataProvider deletes a data provider by name or ARN. Real AWS: "All
// migration projects associated with the data provider must be deleted or
// modified before you can delete the data provider".
func (b *InMemoryBackend) DeleteDataProvider(ctx context.Context, nameOrArn string) (*DataProvider, error) {
	b.mu.Lock("DeleteDataProvider")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if dp, ok := b.dataProviders.Get(regionKey(region, nameOrArn)); ok {
		if b.migrationProjectUsesDataProviderLocked(region, dp.DataProviderArn) {
			return nil, fmt.Errorf("%w: data provider %s has associated migration projects", ErrInvalidState, nameOrArn)
		}

		cp := *dp
		dp.Tags.Close()
		b.dataProviders.Delete(regionKey(region, nameOrArn))

		return &cp, nil
	}

	if dp, ok := lookupUnique(b.dataProvidersByARN, regionKey(region, nameOrArn)); ok {
		if b.migrationProjectUsesDataProviderLocked(region, dp.DataProviderArn) {
			return nil, fmt.Errorf("%w: data provider %s has associated migration projects", ErrInvalidState, nameOrArn)
		}

		cp := *dp
		dp.Tags.Close()
		b.dataProviders.Delete(regionKey(region, dp.DataProviderName))

		return &cp, nil
	}

	return nil, fmt.Errorf("%w: data provider %s not found", ErrNotFound, nameOrArn)
}

// migrationProjectUsesDataProviderLocked reports whether any migration
// project in region references dataProviderArn as a source or target data
// provider. Caller must hold b.mu.
func (b *InMemoryBackend) migrationProjectUsesDataProviderLocked(region, dataProviderArn string) bool {
	for _, mp := range b.migrationProjectsByRegion.Get(region) {
		for _, d := range mp.SourceDataProviderDescriptors {
			if d.DataProviderArn == dataProviderArn {
				return true
			}
		}

		for _, d := range mp.TargetDataProviderDescriptors {
			if d.DataProviderArn == dataProviderArn {
				return true
			}
		}
	}

	return false
}

// ModifyDataProvider updates a data provider. Real AWS: "You must remove the
// data provider from all migration projects before you can modify it"
// (databasemigrationservice@v1.66.4 api_op_ModifyDataProvider.go:16-17).
func (b *InMemoryBackend) ModifyDataProvider(
	ctx context.Context,
	nameOrArn, engine, description string,
) (*DataProvider, error) {
	b.mu.Lock("ModifyDataProvider")
	defer b.mu.Unlock()

	dp := b.findDataProvider(ctx, nameOrArn)
	if dp == nil {
		return nil, fmt.Errorf("%w: data provider %s not found", ErrNotFound, nameOrArn)
	}

	region := getRegion(ctx, b.region)
	if b.migrationProjectUsesDataProviderLocked(region, dp.DataProviderArn) {
		return nil, fmt.Errorf(
			"%w: data provider %s has associated migration projects",
			ErrInvalidState,
			nameOrArn,
		)
	}

	if engine != "" {
		dp.Engine = engine
	}

	if description != "" {
		dp.Description = description
	}

	cp := *dp

	return &cp, nil
}

// findDataProvider locates a data provider by name or ARN within the request
// region (must hold a lock).
func (b *InMemoryBackend) findDataProvider(ctx context.Context, nameOrArn string) *DataProvider {
	region := getRegion(ctx, b.region)
	if dp, ok := b.dataProviders.Get(regionKey(region, nameOrArn)); ok {
		return dp
	}

	if dp, ok := lookupUnique(b.dataProvidersByARN, regionKey(region, nameOrArn)); ok {
		return dp
	}

	return nil
}

// DescribeDataProviders returns all data providers (optionally filtered by name/arn).
func (b *InMemoryBackend) DescribeDataProviders(ctx context.Context, nameOrArn string) ([]*DataProvider, error) {
	b.mu.RLock("DescribeDataProviders")
	defer b.mu.RUnlock()

	if nameOrArn != "" {
		dp := b.findDataProvider(ctx, nameOrArn)
		if dp == nil {
			return []*DataProvider{}, nil
		}

		cp := *dp

		return []*DataProvider{&cp}, nil
	}

	items := b.dataProvidersByRegion.Get(getRegion(ctx, b.region))
	list := make([]*DataProvider, 0, len(items))
	for _, dp := range items {
		cp := *dp
		list = append(list, &cp)
	}

	return list, nil
}
