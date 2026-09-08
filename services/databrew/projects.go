package databrew

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) projectARN(region, name string) string {
	return arn.Build("databrew", region, b.accountID, "project/"+name)
}

func (b *InMemoryBackend) CreateProject(
	ctx context.Context,
	name, datasetName, recipeName, roleArn string,
	sample Sample,
	tags map[string]string,
) (*Project, error) {
	b.mu.Lock("CreateProject")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if name == "" {
		return nil, ErrValidation
	}
	t := b.projectsTable(region)
	if t.Has(name) {
		return nil, ErrAlreadyExists
	}
	if sample.Type != "" && sample.Type != "FIRST_N" && sample.Type != "LAST_N" &&
		sample.Type != "RANDOM" {
		return nil, fmt.Errorf("%w: invalid Sample.Type %q", ErrValidation, sample.Type)
	}
	p := &Project{
		Name: name, Arn: b.projectARN(region, name), DatasetName: datasetName,
		RecipeName: recipeName, RoleArn: roleArn, Sample: sample,
		Tags: maps.Clone(tags), AccountID: b.accountID,
		CreateDate: float64(time.Now().Unix()), LastModifiedDate: float64(time.Now().Unix()),
	}
	t.Put(p)

	return p, nil
}

func (b *InMemoryBackend) DescribeProject(ctx context.Context, name string) (*Project, error) {
	b.mu.RLock("DescribeProject")
	defer b.mu.RUnlock()
	region := getRegion(ctx, b.defaultRegion)
	p, ok := b.projectsTable(region).Get(name)
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	cp.Tags = maps.Clone(p.Tags)

	return &cp, nil
}

func (b *InMemoryBackend) ListProjects(
	ctx context.Context,
	maxResults int,
	nextToken string,
) ([]*Project, string) {
	b.mu.RLock("ListProjects")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	t := b.projectsTable(region)
	keys := snapshotKeys(t, projectKeyFn)
	pageKeys, next := paginateKeys(keys, maxResults, nextToken)
	out := make([]*Project, 0, len(pageKeys))
	for _, k := range pageKeys {
		v, _ := t.Get(k)
		cp := *v
		cp.Tags = maps.Clone(v.Tags)
		out = append(out, &cp)
	}

	return out, next
}

// UpdateProject modifies a project's RoleArn and Sample. DatasetName is
// deliberately NOT settable here: aws-sdk-go-v2/service/databrew's
// UpdateProjectInput has no DatasetName member (only Name/RoleArn/Sample) --
// a project's dataset is fixed at creation and is not one of the documented
// updatable fields.
func (b *InMemoryBackend) UpdateProject(
	ctx context.Context,
	name, roleArn string,
	sample Sample,
) error {
	b.mu.Lock("UpdateProject")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	p, ok := b.projectsTable(region).Get(name)
	if !ok {
		return ErrNotFound
	}
	if sample.Type != "" && sample.Type != "FIRST_N" && sample.Type != "LAST_N" &&
		sample.Type != "RANDOM" {
		return fmt.Errorf("%w: invalid Sample.Type %q", ErrValidation, sample.Type)
	}
	if roleArn != "" {
		p.RoleArn = roleArn
	}
	p.Sample = sample
	p.LastModifiedDate = float64(time.Now().Unix())

	return nil
}

// OpenProjectSession records that a project session was started against
// name, setting OpenDate (a real types.Project member, deserializers.go's
// awsRestjson1_deserializeDocumentProject case "OpenDate") to now. Real AWS
// sets it when a working session is opened via StartProjectSession, the
// only such backend event this in-memory emulator has.
func (b *InMemoryBackend) OpenProjectSession(ctx context.Context, name string) (*Project, error) {
	b.mu.Lock("OpenProjectSession")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	p, ok := b.projectsTable(region).Get(name)
	if !ok {
		return nil, ErrNotFound
	}
	p.OpenDate = float64(time.Now().Unix())
	cp := *p
	cp.Tags = maps.Clone(p.Tags)

	return &cp, nil
}

// projectJobName returns the name of a job currently referencing name as its
// ProjectName, or "" if none does. Callers must hold at least b.mu.RLock.
func (b *InMemoryBackend) projectJobName(region, name string) string {
	t := b.jobsTable(region)
	for _, k := range snapshotKeys(t, jobKeyFn) {
		j, ok := t.Get(k)
		if ok && j.ProjectName == name {
			return j.Name
		}
	}

	return ""
}

// DeleteProject rejects deleting a project still referenced by a job, for
// the same reason DeleteDataset does (see its doc comment): CreateJob's
// validateJobResourceRefs already refuses to create a job naming a
// nonexistent project. ConflictException is modeled for DeleteProject
// (aws-sdk-go-v2/service/databrew's awsRestjson1_deserializeOpErrorDeleteProject).
func (b *InMemoryBackend) DeleteProject(ctx context.Context, name string) error {
	b.mu.Lock("DeleteProject")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if !b.projectsTable(region).Has(name) {
		return ErrNotFound
	}
	if j := b.projectJobName(region, name); j != "" {
		return fmt.Errorf("%w: project %q is used by job %q", ErrConflict, name, j)
	}
	b.projectsTable(region).Delete(name)

	return nil
}
