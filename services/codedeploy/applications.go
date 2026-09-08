package codedeploy

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// CreateApplication creates a new CodeDeploy application.
func (b *InMemoryBackend) CreateApplication(name, computePlatform string, kv map[string]string) (*Application, error) {
	b.mu.Lock("CreateApplication")
	defer b.mu.Unlock()

	if b.applications.Has(name) {
		return nil, fmt.Errorf("%w: application %s already exists", ErrAlreadyExists, name)
	}

	if computePlatform == "" {
		computePlatform = computePlatformServer
	}

	if err := validateComputePlatform(computePlatform); err != nil {
		return nil, err
	}

	appID := uuid.NewString()
	t := tags.New("codedeploy.application." + name + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}

	app := &Application{
		ApplicationName: name,
		ApplicationID:   appID,
		ComputePlatform: computePlatform,
		AccountID:       b.accountID,
		Region:          b.region,
		CreationTime:    time.Now().UTC(),
		Tags:            t,
	}
	b.applications.Put(app)

	cp := *app

	return &cp, nil
}

// GetApplication returns an application by name.
func (b *InMemoryBackend) GetApplication(name string) (*Application, error) {
	b.mu.RLock("GetApplication")
	defer b.mu.RUnlock()

	app, ok := b.applications.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: application %s not found", ErrNotFound, name)
	}

	cp := *app

	return &cp, nil
}

// ListApplications returns all application names in sorted order.
func (b *InMemoryBackend) ListApplications() []string {
	b.mu.RLock("ListApplications")
	defer b.mu.RUnlock()

	// Table.Snapshot returns entries sorted by primary key, which for
	// applications is ApplicationName, so this is already alphabetical.
	items := b.applications.Snapshot()
	names := make([]string, len(items))
	for i, app := range items {
		names[i] = app.ApplicationName
	}

	return names
}

// ListApplicationDetails returns all applications as structs.
func (b *InMemoryBackend) ListApplicationDetails() []*Application {
	b.mu.RLock("ListApplicationDetails")
	defer b.mu.RUnlock()

	all := b.applications.All()
	list := make([]*Application, 0, len(all))
	for _, app := range all {
		cp := *app
		list = append(list, &cp)
	}

	return list
}

// DeleteApplication deletes an application and all its deployment groups.
func (b *InMemoryBackend) DeleteApplication(name string) error {
	b.mu.Lock("DeleteApplication")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(name)
	if !ok {
		// DeleteApplication's deserializer models no ApplicationDoesNotExistException
		// (aws-sdk-go-v2/service/codedeploy deserializers.go) -- this code is provably
		// wrong here, but idempotent-success vs. a different code is unconfirmed.
		// Do NOT "fix" this by guessing; needs real evidence (gopherstack-3pz8).
		return fmt.Errorf("%w: application %s not found", ErrNotFound, name)
	}

	app.Tags.Close()

	// slices.Clone before the delete loop: deleting from b.deploymentGroups
	// mutates the same byApplication index bucket this slice is a view of.
	for _, dg := range slices.Clone(b.deploymentGroupsByApp.Get(name)) {
		dg.Tags.Close()
		b.deploymentGroups.Delete(dgKey(dg.ApplicationName, dg.DeploymentGroupName))
	}

	b.deleteApplicationRevisions(name)

	b.applications.Delete(name)

	return nil
}

// UpdateApplication renames a CodeDeploy application, updating all referencing deployment
// groups and deployments.
func (b *InMemoryBackend) UpdateApplication(name, newName string) error {
	b.mu.Lock("UpdateApplication")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(name)
	if !ok {
		return fmt.Errorf("%w: application %s not found", ErrNotFound, name)
	}

	if newName == "" || newName == name {
		return nil
	}

	if b.applications.Has(newName) {
		return fmt.Errorf("%w: application %s already exists", ErrAlreadyExists, newName)
	}

	// ApplicationName is the store.Table primary key, so Put-after-in-place-
	// mutate would leave a stale entry at the old key: delete, mutate, Put.
	b.applications.Delete(name)
	app.ApplicationName = newName
	b.applications.Put(app)

	// Move all deployment groups for this application to the new app name.
	// DeploymentGroup.ApplicationName is both part of the deploymentGroups
	// primary key (see dgKey) and the deploymentGroupsByApp index key, so
	// each affected group needs the same delete/mutate/Put treatment.
	// slices.Clone before the delete loop: deleting mutates the same
	// byApplication index bucket this slice is a view of.
	for _, dg := range slices.Clone(b.deploymentGroupsByApp.Get(name)) {
		b.deploymentGroups.Delete(dgKey(dg.ApplicationName, dg.DeploymentGroupName))
		dg.ApplicationName = newName
		b.deploymentGroups.Put(dg)
	}

	// Update all existing deployments that reference the old application
	// name. ApplicationName is not part of Deployment's primary key
	// (DeploymentID is), so in-place mutation is safe here.
	for _, d := range b.deployments.All() {
		if d.ApplicationName == name {
			d.ApplicationName = newName
		}
	}

	b.renameApplicationRevisions(name, newName)

	return nil
}

// BatchGetApplications returns application structs for the given names.
// Names that do not exist are silently omitted (AWS behavior).
func (b *InMemoryBackend) BatchGetApplications(names []string) []*Application {
	b.mu.RLock("BatchGetApplications")
	defer b.mu.RUnlock()

	result := make([]*Application, 0, len(names))

	for _, name := range names {
		app, ok := b.applications.Get(name)
		if !ok {
			continue
		}

		cp := *app
		result = append(result, &cp)
	}

	return result
}

// AddApplicationInternal adds an application directly to the backend without validation.
// Used for test seeding only.
func (b *InMemoryBackend) AddApplicationInternal(app *Application) {
	b.mu.Lock("AddApplicationInternal")
	defer b.mu.Unlock()

	app.Tags = ensureTags(app.Tags, "codedeploy.application."+app.ApplicationName+".tags")

	if app.ApplicationID == "" {
		app.ApplicationID = uuid.NewString()
	}

	if app.CreationTime.IsZero() {
		app.CreationTime = time.Now().UTC()
	}

	b.applications.Put(app)
}

// ApplicationARN builds an ARN for a CodeDeploy application.
func (b *InMemoryBackend) ApplicationARN(name string) string {
	return arn.Build("codedeploy", b.region, b.accountID, "application:"+name)
}
