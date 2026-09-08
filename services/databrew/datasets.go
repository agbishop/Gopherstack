package databrew

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) datasetARN(region, name string) string {
	return arn.Build("databrew", region, b.accountID, "dataset/"+name)
}

func (b *InMemoryBackend) CreateDataset(
	ctx context.Context,
	name, format string,
	input DatasetInput,
	formatOpts DatasetFormatOptions,
	tags map[string]string,
	pathOptions *PathOptions,
) (*Dataset, error) {
	b.mu.Lock("CreateDataset")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if name == "" {
		return nil, ErrValidation
	}
	t := b.datasetsTable(region)
	if t.Has(name) {
		return nil, ErrAlreadyExists
	}
	source := "S3"
	if input.DataCatalogInputDefinition != nil {
		source = "DATA_CATALOG"
	} else if input.DatabaseInputDefinition != nil {
		source = "DATABASE"
	}
	ds := &Dataset{
		Name: name, Arn: b.datasetARN(region, name), Format: format,
		Input: input, FormatOptions: formatOpts, Tags: maps.Clone(tags),
		PathOptions: pathOptions, AccountID: b.accountID,
		Source: source, CreateDate: float64(time.Now().Unix()),
		LastModifiedDate: float64(time.Now().Unix()),
	}
	t.Put(ds)

	return ds, nil
}

func (b *InMemoryBackend) DescribeDataset(ctx context.Context, name string) (*Dataset, error) {
	b.mu.RLock("DescribeDataset")
	defer b.mu.RUnlock()
	region := getRegion(ctx, b.defaultRegion)
	ds, ok := b.datasetsTable(region).Get(name)
	if !ok {
		return nil, ErrNotFound
	}
	cp := *ds
	cp.Tags = maps.Clone(ds.Tags)

	return &cp, nil
}

func (b *InMemoryBackend) ListDatasets(
	ctx context.Context,
	maxResults int,
	nextToken string,
) ([]*Dataset, string) {
	b.mu.RLock("ListDatasets")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	t := b.datasetsTable(region)
	keys := snapshotKeys(t, datasetKeyFn)
	pageKeys, next := paginateKeys(keys, maxResults, nextToken)
	out := make([]*Dataset, 0, len(pageKeys))
	for _, k := range pageKeys {
		v, _ := t.Get(k)
		cp := *v
		cp.Tags = maps.Clone(v.Tags)
		out = append(out, &cp)
	}

	return out, next
}

func (b *InMemoryBackend) UpdateDataset(
	ctx context.Context,
	name, format string,
	input DatasetInput,
	formatOpts DatasetFormatOptions,
	pathOptions *PathOptions,
) error {
	b.mu.Lock("UpdateDataset")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	ds, ok := b.datasetsTable(region).Get(name)
	if !ok {
		return ErrNotFound
	}
	ds.Format = format
	ds.Input = input
	ds.FormatOptions = formatOpts
	ds.PathOptions = pathOptions
	ds.LastModifiedDate = float64(time.Now().Unix())

	return nil
}

// datasetProjectName returns the name of a project currently referencing
// name as its DatasetName, or "" if none does. Callers must hold at least
// b.mu.RLock.
func (b *InMemoryBackend) datasetProjectName(region, name string) string {
	t := b.projectsTable(region)
	for _, k := range snapshotKeys(t, projectKeyFn) {
		p, ok := t.Get(k)
		if ok && p.DatasetName == name {
			return p.Name
		}
	}

	return ""
}

// datasetJobName returns the name of a job currently referencing name as its
// DatasetName, or "" if none does. Callers must hold at least b.mu.RLock.
func (b *InMemoryBackend) datasetJobName(region, name string) string {
	t := b.jobsTable(region)
	for _, k := range snapshotKeys(t, jobKeyFn) {
		j, ok := t.Get(k)
		if ok && j.DatasetName == name {
			return j.Name
		}
	}

	return ""
}

// DeleteDataset rejects deleting a dataset still referenced by a project or
// job: CreateJob's own validateJobResourceRefs already refuses to create a
// job naming a nonexistent dataset, so allowing the reverse -- deleting a
// dataset out from under an existing project/job -- would produce the exact
// dangling reference that check exists to prevent. ConflictException is
// modeled for DeleteDataset (aws-sdk-go-v2/service/databrew's
// awsRestjson1_deserializeOpErrorDeleteDataset).
func (b *InMemoryBackend) DeleteDataset(ctx context.Context, name string) error {
	b.mu.Lock("DeleteDataset")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if !b.datasetsTable(region).Has(name) {
		return ErrNotFound
	}
	if p := b.datasetProjectName(region, name); p != "" {
		return fmt.Errorf("%w: dataset %q is used by project %q", ErrConflict, name, p)
	}
	if j := b.datasetJobName(region, name); j != "" {
		return fmt.Errorf("%w: dataset %q is used by job %q", ErrConflict, name, j)
	}
	b.datasetsTable(region).Delete(name)

	return nil
}
