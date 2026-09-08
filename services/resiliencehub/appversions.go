package resiliencehub

import (
	"sort"
	"strconv"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// resolveAppVersionLocked returns the AppVersion identified by (app, number),
// where an empty or draftVersion number means the draft. Callers must hold
// b.mu.
func resolveAppVersionLocked(a *App, number string) (*AppVersion, bool) {
	if number == "" || number == draftVersion {
		return a.Draft, true
	}

	for _, v := range a.Published {
		if v.Number == number {
			return v, true
		}
	}

	return nil, false
}

// DescribeAppVersion returns AdditionalInfo for (appArn, appVersion).
func (b *InMemoryBackend) DescribeAppVersion(appArn, appVersion string) (*App, *AppVersion, error) {
	b.mu.RLock("DescribeAppVersion")
	defer b.mu.RUnlock()

	a, ok := b.resolveAppLocked(appArn)
	if !ok {
		return nil, nil, notFoundError(resourceApp, appArn)
	}

	v, ok := resolveAppVersionLocked(a, appVersion)
	if !ok {
		return nil, nil, notFoundError(resourceAppVersion, appVersion)
	}

	return a, v.clone(), nil
}

// UpdateAppVersion updates AdditionalInfo on the draft AppVersion (this op
// takes no AppVersion parameter on the wire -- it always targets the draft,
// per PARITY.md's "operates on the draft only" invariant).
func (b *InMemoryBackend) UpdateAppVersion(
	appArn string,
	additionalInfo map[string][]string,
) (*App, *AppVersion, error) {
	b.mu.Lock("UpdateAppVersion")
	defer b.mu.Unlock()

	a, ok := b.resolveAppLocked(appArn)
	if !ok {
		return nil, nil, notFoundError(resourceApp, appArn)
	}

	if additionalInfo != nil {
		a.Draft.AdditionalInfo = cloneStrSliceMap(additionalInfo)
	}

	return a, a.Draft.clone(), nil
}

// ListAppVersions returns every version (draft plus each published snapshot)
// of the App identified by appArn, optionally filtered by [startTime,
// endTime] on CreationTime.
func (b *InMemoryBackend) ListAppVersions(
	appArn string, startTime, endTime time.Time, token string, limit int,
) (page.Page[appVersionSummaryWire], error) {
	b.mu.RLock("ListAppVersions")
	defer b.mu.RUnlock()

	a, ok := b.resolveAppLocked(appArn)
	if !ok {
		return page.Page[appVersionSummaryWire]{}, notFoundError(resourceApp, appArn)
	}

	all := make([]*AppVersion, 0, len(a.Published)+1)
	all = append(all, a.Published...)
	all = append(all, a.Draft)

	sort.Slice(all, func(i, j int) bool { return all[i].CreationTime.Before(all[j].CreationTime) })

	out := make([]appVersionSummaryWire, 0, len(all))

	for _, v := range all {
		if !startTime.IsZero() && v.CreationTime.Before(startTime) {
			continue
		}

		if !endTime.IsZero() && v.CreationTime.After(endTime) {
			continue
		}

		summary := appVersionSummaryWire{
			AppVersion:   v.Number,
			VersionName:  v.VersionName,
			CreationTime: epochPtrT(v.CreationTime),
		}

		if v.Number != draftVersion {
			if n, err := strconv.ParseInt(v.Number, 10, 64); err == nil {
				summary.Identifier = &n
			}
		}

		out = append(out, summary)
	}

	return page.New(out, token, limit, defaultPageLimit), nil
}

// PublishAppVersion snapshots the App's current draft AppVersion into a new,
// immutable numbered version. The draft itself continues to be edited
// forward from where it was -- publishing only takes a deep-copy snapshot,
// it does not reset or replace the draft.
func (b *InMemoryBackend) PublishAppVersion(appArn, versionName string) (*App, string, error) {
	b.mu.Lock("PublishAppVersion")
	defer b.mu.Unlock()

	a, ok := b.resolveAppLocked(appArn)
	if !ok {
		return nil, "", notFoundError(resourceApp, appArn)
	}

	number := strconv.FormatInt(a.NextVersionNumber, 10)

	snapshot := a.Draft.clone()
	snapshot.Number = number
	snapshot.VersionName = versionName
	snapshot.CreationTime = time.Now().UTC()

	a.Published = append(a.Published, snapshot)
	a.NextVersionNumber++

	return a.clone(), number, nil
}

// DescribeAppVersionTemplate returns the template body for (appArn,
// appVersion).
func (b *InMemoryBackend) DescribeAppVersionTemplate(appArn, appVersion string) (string, error) {
	b.mu.RLock("DescribeAppVersionTemplate")
	defer b.mu.RUnlock()

	a, ok := b.resolveAppLocked(appArn)
	if !ok {
		return "", notFoundError(resourceApp, appArn)
	}

	v, ok := resolveAppVersionLocked(a, appVersion)
	if !ok {
		return "", notFoundError(resourceAppVersion, appVersion)
	}

	return v.TemplateBody, nil
}

// PutDraftAppVersionTemplate replaces the draft AppVersion's template body.
func (b *InMemoryBackend) PutDraftAppVersionTemplate(appArn, templateBody string) (*App, error) {
	if templateBody == "" {
		return nil, validationError("appTemplateBody is required")
	}

	b.mu.Lock("PutDraftAppVersionTemplate")
	defer b.mu.Unlock()

	a, ok := b.resolveAppLocked(appArn)
	if !ok {
		return nil, notFoundError(resourceApp, appArn)
	}

	a.Draft.TemplateBody = templateBody

	return a.clone(), nil
}

// findAppComponent returns the component named name, or nil.
func findAppComponent(v *AppVersion, name string) *AppComponent {
	for _, c := range v.AppComponents {
		if c.Name == name {
			return c
		}
	}

	return nil
}

// findAppComponentByID returns the component with the given id, or nil.
func findAppComponentByID(v *AppVersion, id string) *AppComponent {
	for _, c := range v.AppComponents {
		if c.ID == id {
			return c
		}
	}

	return nil
}

// CreateAppVersionAppComponent adds a new AppComponent to the App's draft
// version.
func (b *InMemoryBackend) CreateAppVersionAppComponent(
	appArn string, req *createAppVersionAppComponentRequest,
) (*App, *AppComponent, error) {
	if req.Name == "" {
		return nil, nil, validationError("name is required")
	}

	if req.Type == "" {
		return nil, nil, validationError("type is required")
	}

	b.mu.Lock("CreateAppVersionAppComponent")
	defer b.mu.Unlock()

	a, ok := b.resolveAppLocked(appArn)
	if !ok {
		return nil, nil, notFoundError(resourceApp, appArn)
	}

	if findAppComponent(a.Draft, req.Name) != nil {
		return nil, nil, conflictError("app component already exists: " + req.Name)
	}

	id := req.ID
	if id == "" {
		id = newAppComponentID()
	}

	c := &AppComponent{ID: id, Name: req.Name, Type: req.Type, AdditionalInfo: cloneStrSliceMap(req.AdditionalInfo)}
	a.Draft.AppComponents = append(a.Draft.AppComponents, c)

	return a.clone(), c.clone(), nil
}

// DescribeAppVersionAppComponent returns the component named id in
// (appArn, appVersion) -- reads may target any version, draft or published.
func (b *InMemoryBackend) DescribeAppVersionAppComponent(appArn, appVersion, id string) (*App, *AppComponent, error) {
	b.mu.RLock("DescribeAppVersionAppComponent")
	defer b.mu.RUnlock()

	a, ok := b.resolveAppLocked(appArn)
	if !ok {
		return nil, nil, notFoundError(resourceApp, appArn)
	}

	v, ok := resolveAppVersionLocked(a, appVersion)
	if !ok {
		return nil, nil, notFoundError(resourceAppVersion, appVersion)
	}

	c := findAppComponentByID(v, id)
	if c == nil {
		return nil, nil, notFoundError(resourceAppComponent, id)
	}

	return a, c.clone(), nil
}

// renameAppComponentResourcesLocked repoints every resource on v whose
// AppComponents assignment names oldName to newName instead. This backend
// tracks a resource's component assignment purely by name (see
// wire_convert.go's toPhysicalResourceWire doc comment), so without this a
// rename would silently orphan every resource previously assigned to the
// component -- they would no longer match any component name at all. Callers
// must hold b.mu.
func renameAppComponentResourcesLocked(v *AppVersion, oldName, newName string) {
	for _, r := range v.Resources {
		for i, name := range r.AppComponents {
			if name == oldName {
				r.AppComponents[i] = newName
			}
		}
	}
}

// UpdateAppVersionAppComponent updates the component with the given id on
// the draft version. Renaming to a name already used by another component on
// the same version is rejected (ConflictException), matching
// CreateAppVersionAppComponent's own duplicate-name rule -- this backend's
// resource-to-component assignment is name-keyed (see
// renameAppComponentResourcesLocked), so two components sharing a name would
// make that assignment ambiguous.
func (b *InMemoryBackend) UpdateAppVersionAppComponent(
	appArn string, req *updateAppVersionAppComponentRequest, id string,
) (*App, *AppComponent, error) {
	b.mu.Lock("UpdateAppVersionAppComponent")
	defer b.mu.Unlock()

	a, ok := b.resolveAppLocked(appArn)
	if !ok {
		return nil, nil, notFoundError(resourceApp, appArn)
	}

	c := findAppComponentByID(a.Draft, id)
	if c == nil {
		return nil, nil, notFoundError(resourceAppComponent, id)
	}

	if req.Name != "" && req.Name != c.Name {
		if existing := findAppComponent(a.Draft, req.Name); existing != nil && existing.ID != id {
			return nil, nil, conflictError("app component already exists: " + req.Name)
		}

		renameAppComponentResourcesLocked(a.Draft, c.Name, req.Name)
		c.Name = req.Name
	}

	if req.Type != "" {
		c.Type = req.Type
	}

	if req.AdditionalInfo != nil {
		c.AdditionalInfo = cloneStrSliceMap(req.AdditionalInfo)
	}

	return a.clone(), c.clone(), nil
}

// DeleteAppVersionAppComponent deletes the component with the given id from
// the draft version. Rejected (ConflictException) if any resource is still
// assigned to it -- per the SDK's own doc comment ("You will not be able to
// delete an Application Component if it has resources associated with it").
func (b *InMemoryBackend) DeleteAppVersionAppComponent(appArn, id string) (*App, *AppComponent, error) {
	b.mu.Lock("DeleteAppVersionAppComponent")
	defer b.mu.Unlock()

	a, ok := b.resolveAppLocked(appArn)
	if !ok {
		return nil, nil, notFoundError(resourceApp, appArn)
	}

	c := findAppComponentByID(a.Draft, id)
	if c == nil {
		return nil, nil, notFoundError(resourceAppComponent, id)
	}

	for _, r := range a.Draft.Resources {
		if containsStr(r.AppComponents, c.Name) {
			return nil, nil, conflictError("app component has associated resources: " + c.Name)
		}
	}

	kept := make([]*AppComponent, 0, len(a.Draft.AppComponents)-1)

	for _, existing := range a.Draft.AppComponents {
		if existing.ID != id {
			kept = append(kept, existing)
		}
	}

	a.Draft.AppComponents = kept

	return a.clone(), c.clone(), nil
}

// ListAppVersionAppComponents returns every AppComponent on (appArn,
// appVersion).
func (b *InMemoryBackend) ListAppVersionAppComponents(
	appArn, appVersion, token string, limit int,
) (*App, page.Page[*AppComponent], error) {
	b.mu.RLock("ListAppVersionAppComponents")
	defer b.mu.RUnlock()

	a, ok := b.resolveAppLocked(appArn)
	if !ok {
		return nil, page.Page[*AppComponent]{}, notFoundError(resourceApp, appArn)
	}

	v, ok := resolveAppVersionLocked(a, appVersion)
	if !ok {
		return nil, page.Page[*AppComponent]{}, notFoundError(resourceAppVersion, appVersion)
	}

	return a, page.New(v.AppComponents, token, limit, defaultPageLimit), nil
}
