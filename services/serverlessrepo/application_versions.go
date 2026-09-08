package serverlessrepo

import (
	"fmt"
	"sort"
	"time"
)

// cloneVersion returns a deep copy of v, including slice fields.
func cloneVersion(v *ApplicationVersion) *ApplicationVersion {
	cp := *v
	cp.RequiredCapabilities = cloneStringSlice(v.RequiredCapabilities)
	cp.ParameterDefinitions = cloneParameterDefinitions(v.ParameterDefinitions)

	return &cp
}

// AddVersionInternal creates a version directly in the backend, bypassing validation.
// Useful for seeding test state. The application must already exist.
func (b *InMemoryBackend) AddVersionInternal(appName, semanticVersion string) *ApplicationVersion {
	b.mu.Lock("AddVersionInternal")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(appName)
	if !ok {
		return nil
	}

	v := &ApplicationVersion{
		ApplicationID:        app.ApplicationID,
		SemanticVersion:      semanticVersion,
		AppName:              appName,
		CreationTime:         time.Now(),
		ParameterDefinitions: []ParameterDefinition{},
		RequiredCapabilities: []string{},
		ResourcesSupported:   true,
	}
	b.appVersions.Put(v)

	return cloneVersion(v)
}

// CreateApplicationVersion creates a new version for an application.
func (b *InMemoryBackend) CreateApplicationVersion(
	appName string,
	semanticVersion string,
	sourceCodeURL string,
	templateURL string,
) (*ApplicationVersion, error) {
	return b.CreateApplicationVersionWithOptions(appName, semanticVersion, CreateApplicationVersionOptions{
		SourceCodeURL: sourceCodeURL,
		TemplateURL:   templateURL,
	})
}

// CreateApplicationVersionWithOptions creates a version including optional archive metadata.
func (b *InMemoryBackend) CreateApplicationVersionWithOptions(
	appName string,
	semanticVersion string,
	opts CreateApplicationVersionOptions,
) (*ApplicationVersion, error) {
	b.mu.Lock("CreateApplicationVersion")
	defer b.mu.Unlock()

	app, ok := b.applications.Get(appName)
	if !ok {
		// CreateApplicationVersion's modelled error set has no NotFoundException
		// (deserializers.go awsRestjson1_deserializeOpErrorCreateApplicationVersion):
		// an unknown ApplicationId is a BadRequestException, not a 404.
		return nil, fmt.Errorf("%w: could not find application %q", ErrValidation, appName)
	}

	if semanticVersion == "" {
		return nil, fmt.Errorf("%w: semanticVersion is required", ErrValidation)
	}

	if !isValidSemanticVersion(semanticVersion) {
		return nil, fmt.Errorf("%w: semanticVersion must be a valid semantic version (e.g. 1.0.0)", ErrValidation)
	}

	if opts.SourceCodeURL == "" && opts.SourceCodeArchiveURL == "" && opts.TemplateURL == "" {
		return nil, fmt.Errorf(
			"%w: at least one of sourceCodeUrl, sourceCodeArchiveUrl or templateUrl is required",
			ErrValidation,
		)
	}

	if b.appVersions.Has(versionKey(appName, semanticVersion)) {
		return nil, fmt.Errorf(
			"%w: version %s already exists for application %q",
			ErrVersionAlreadyExists,
			semanticVersion,
			appName,
		)
	}

	// Generate a synthetic template URL when the caller provides only a sourceCodeURL.
	resolvedTemplateURL := opts.TemplateURL
	if resolvedTemplateURL == "" && (opts.SourceCodeURL != "" || opts.SourceCodeArchiveURL != "") {
		resolvedTemplateURL = synthesizeTemplateURL(appName, semanticVersion)
	}

	v := &ApplicationVersion{
		ApplicationID:        app.ApplicationID,
		SemanticVersion:      semanticVersion,
		AppName:              appName,
		SourceCodeURL:        opts.SourceCodeURL,
		SourceCodeArchiveURL: opts.SourceCodeArchiveURL,
		TemplateURL:          resolvedTemplateURL,
		CreationTime:         time.Now(),
		ParameterDefinitions: []ParameterDefinition{},
		RequiredCapabilities: []string{},
		ResourcesSupported:   true,
	}
	b.appVersions.Put(v)

	// Track the latest created version on the application itself so GetApplication
	// returns the most recently created version by default.
	app.SemanticVersion = semanticVersion

	return cloneVersion(v), nil
}

// GetApplicationVersion returns a specific version of an application by semantic version string.
func (b *InMemoryBackend) GetApplicationVersion(appName, semanticVersion string) (*ApplicationVersion, error) {
	b.mu.RLock("GetApplicationVersion")
	defer b.mu.RUnlock()

	if !b.applications.Has(appName) {
		return nil, fmt.Errorf("%w: could not find application %q", ErrApplicationNotFound, appName)
	}

	v, ok := b.appVersions.Get(versionKey(appName, semanticVersion))
	if !ok {
		return nil, fmt.Errorf(
			"%w: could not find version %q for application %q",
			ErrApplicationNotFound,
			semanticVersion,
			appName,
		)
	}

	return cloneVersion(v), nil
}

// ListApplicationVersions returns all versions for an application sorted by semantic version.
func (b *InMemoryBackend) ListApplicationVersions(appName string) ([]*ApplicationVersion, error) {
	b.mu.RLock("ListApplicationVersions")
	defer b.mu.RUnlock()

	if !b.applications.Has(appName) {
		return nil, fmt.Errorf("%w: could not find application %q", ErrApplicationNotFound, appName)
	}

	versions := b.appVersionsByApp.Get(appName)
	list := make([]*ApplicationVersion, 0, len(versions))

	for _, v := range versions {
		list = append(list, cloneVersion(v))
	}

	// store.Index preserves insertion order, not sort order -- explicit sort
	// is required for the deterministic-by-semanticVersion result this
	// method has always returned.
	sort.Slice(list, func(i, j int) bool {
		return list[i].SemanticVersion < list[j].SemanticVersion
	})

	return list, nil
}
