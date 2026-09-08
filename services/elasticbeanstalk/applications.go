package elasticbeanstalk

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// --- Application store.Table/Index helpers. Callers must hold b.mu. ---

func (b *InMemoryBackend) applicationGet(region, name string) (*Application, bool) {
	return b.applications.Get(regionKey(region, name))
}

func (b *InMemoryBackend) applicationPut(v *Application) { b.applications.Put(v) }

func (b *InMemoryBackend) applicationDelete(region, name string) {
	b.applications.Delete(regionKey(region, name))
}

func (b *InMemoryBackend) applicationsInRegion(region string) []*Application {
	return b.applicationsByRegion.Get(region)
}

// applicationByARN looks up an application by ARN, scoped to region: an
// application created in one region must never resolve when queried from
// another (see TestEBTagRegionIsolation), so the index key is the composite
// "region|ARN", not ARN alone.
func (b *InMemoryBackend) applicationByARN(region, resourceARN string) (*Application, bool) {
	list := b.applicationsByARN.Get(regionKey(region, resourceARN))
	if len(list) == 0 {
		return nil, false
	}

	return list[0], true
}

// --- Application operations ---

// CreateApplication creates a new Elastic Beanstalk application.
func (b *InMemoryBackend) CreateApplication(
	ctx context.Context,
	name, description string,
	tags map[string]string,
) (*Application, error) {
	b.mu.Lock("CreateApplication")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.applicationGet(region, name); ok {
		return nil, fmt.Errorf("%w: application %s already exists", ErrAlreadyExists, name)
	}

	appARN := arn.Build("elasticbeanstalk", region, b.accountID, "application/"+name)

	app := &Application{
		ApplicationName: name,
		ApplicationARN:  appARN,
		Description:     description,
		DateCreated:     nowISO8601(),
		DateUpdated:     nowISO8601(),
		Tags:            copyTags(tags),
		region:          region,
	}
	b.applicationPut(app)

	// Real AWS: "Creates an application that has one configuration template
	// named default and no application versions" -- see defaultConfigTemplateName.
	b.createDefaultConfigurationTemplate(region, name)

	return cloneApplication(app), nil
}

// DescribeApplications returns applications, optionally filtered by names.
// Results are sorted by ApplicationName for deterministic output.
func (b *InMemoryBackend) DescribeApplications(ctx context.Context, names []string) []*Application {
	b.mu.RLock("DescribeApplications")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	if len(names) == 0 {
		apps := b.applicationsInRegion(region)
		list := make([]*Application, 0, len(apps))

		for _, app := range apps {
			list = append(list, cloneApplication(app))
		}

		sort.Slice(list, func(i, j int) bool {
			return list[i].ApplicationName < list[j].ApplicationName
		})

		return list
	}

	list := make([]*Application, 0, len(names))

	for _, name := range names {
		if app, ok := b.applicationGet(region, name); ok {
			list = append(list, cloneApplication(app))
		}
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].ApplicationName < list[j].ApplicationName
	})

	return list
}

// UpdateApplication updates an application's description.
func (b *InMemoryBackend) UpdateApplication(ctx context.Context, name, description string) (*Application, error) {
	b.mu.Lock("UpdateApplication")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	app, ok := b.applicationGet(region, name)
	if !ok {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, name)
	}

	app.Description = description
	app.DateUpdated = nowISO8601()

	return cloneApplication(app), nil
}

// UpdateApplicationResourceLifecycle stores the resource lifecycle service role on the application (improvement #7).
func (b *InMemoryBackend) UpdateApplicationResourceLifecycle(
	ctx context.Context,
	appName, serviceRole string,
) (*Application, error) {
	b.mu.Lock("UpdateApplicationResourceLifecycle")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	app, ok := b.applicationGet(region, appName)
	if !ok {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, appName)
	}

	app.ResourceLifecycleServiceRole = serviceRole

	return cloneApplication(app), nil
}

// DeleteApplication removes an application and all associated versions and
// configuration templates. Real AWS: "You cannot delete an application that
// has a running environment" -- unless terminateEnvByForce is set, in which
// case running environments are terminated first.
func (b *InMemoryBackend) DeleteApplication(ctx context.Context, name string, terminateEnvByForce bool) error {
	b.mu.Lock("DeleteApplication")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if _, ok := b.applicationGet(region, name); !ok {
		return fmt.Errorf("%w: application %s not found", ErrNotFound, name)
	}

	// The index result is cloned before the loop since store.Index slices
	// mutate under Delete (see pkgs/store gotcha).
	var running []*Environment

	for _, env := range slices.Clone(b.environmentsInRegion(region)) {
		if env.ApplicationName == name {
			running = append(running, env)
		}
	}

	if len(running) > 0 {
		if !terminateEnvByForce {
			return fmt.Errorf("%w: application %s has a running environment", ErrInvalidParameter, name)
		}

		for _, env := range running {
			b.terminateEnvironmentLocked(region, env)
		}
	}

	// Cascade: remove all application versions belonging to this application.
	for _, ver := range slices.Clone(b.appVersionsInRegion(region)) {
		if ver.ApplicationName == name {
			b.appVersionDelete(region, ver.ApplicationName, ver.VersionLabel)
		}
	}

	// Cascade: remove all configuration templates belonging to this application.
	for _, tmpl := range slices.Clone(b.configTemplatesInRegion(region)) {
		if tmpl.ApplicationName == name {
			b.configTemplateDelete(region, tmpl.ApplicationName, tmpl.TemplateName)
		}
	}

	// applicationDelete also removes app from every registered index
	// (byRegion, byARN) automatically -- see store.Table.Delete.
	b.applicationDelete(region, name)

	return nil
}

// addApplicationInternal seeds an application directly into the backend, bypassing validation.
// Caller must hold the write lock.
func (b *InMemoryBackend) addApplicationInternal(region string, app *Application) {
	cp := cloneApplication(app)
	cp.region = region
	b.applicationPut(cp)
}
