package codebuild

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) buildProjectARN(name string) string {
	return arn.Build("codebuild", b.region, b.accountID, "project/"+name)
}

// lookupByNameOrARN finds a project by name or by its ARN.
func (b *InMemoryBackend) lookupByNameOrARN(nameOrARN string) (*Project, bool) {
	if p, ok := b.projects.Get(nameOrARN); ok {
		return p, true
	}

	if matches := b.projectsByARN.Get(nameOrARN); len(matches) > 0 {
		return matches[0], true
	}

	return nil, false
}

// ProjectConfig holds all configurable fields for creating or updating a project.
type ProjectConfig struct {
	Cache                   *ProjectCache
	Source                  *ProjectSource
	Artifacts               *ProjectArtifacts
	Tags                    map[string]string
	BuildBatchConfig        *BuildBatchConfig
	VpcConfig               *VpcConfig
	LogsConfig              *LogsConfig
	Environment             *ProjectEnvironment
	BadgeEnabled            *bool
	Description             string
	Name                    string
	EncryptionKey           string
	ServiceRole             string
	ResourceAccessRole      string
	SourceVersion           string
	SecondaryArtifacts      []ProjectArtifacts
	SecondarySourceVersions []ProjectSourceVersion
	SecondarySources        []ProjectSource
	FileSystemLocations     []FileSystemLocation
	TimeoutInMinutes        int32
	QueuedTimeoutInMinutes  int32
	ConcurrentBuildLimit    int32
	AutoRetryLimit          int32
}

// applyBadge sets p.Badge from a CreateProject/UpdateProject badgeEnabled
// request (aws-sdk-go-v2/service/codebuild@v1.72.4/api_op_CreateProject.go's
// BadgeEnabled *bool, api_op_UpdateProject.go's identical field). nil means
// the request didn't mention badgeEnabled, leaving the current value
// unchanged (Update partial-update semantics; on Create, p.Badge starts
// nil, so nil correctly leaves badging disabled). Enabling for the first
// time generates a stable badgeRequestUrl; re-enabling an already-enabled
// badge leaves its URL unchanged, matching real AWS not rotating it on every
// UpdateProject call.
func (b *InMemoryBackend) applyBadge(p *Project, badgeEnabled *bool, name string) {
	if badgeEnabled == nil {
		return
	}

	if !*badgeEnabled {
		p.Badge = &ProjectBadge{BadgeEnabled: false}

		return
	}

	if p.Badge != nil && p.Badge.BadgeEnabled {
		return
	}

	url := "https://codebuild." + b.region + ".amazonaws.com/badges?uuid=" + uuid.NewString() + "&project=" + name
	p.Badge = &ProjectBadge{BadgeEnabled: true, BadgeRequestURL: url}
}

// CreateProject creates a new CodeBuild project.
func (b *InMemoryBackend) CreateProject(cfg ProjectConfig) (*Project, error) {
	b.mu.Lock("CreateProject")
	defer b.mu.Unlock()

	if cfg.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}

	if b.projects.Has(cfg.Name) {
		return nil, ErrAlreadyExists
	}

	tagsCopy := make(map[string]string, len(cfg.Tags))
	maps.Copy(tagsCopy, cfg.Tags)

	now := float64(time.Now().Unix())
	p := &Project{
		Name:                    cfg.Name,
		Arn:                     b.buildProjectARN(cfg.Name),
		Description:             cfg.Description,
		ServiceRole:             cfg.ServiceRole,
		EncryptionKey:           cfg.EncryptionKey,
		ResourceAccessRole:      cfg.ResourceAccessRole,
		TimeoutInMinutes:        cfg.TimeoutInMinutes,
		QueuedTimeoutInMinutes:  cfg.QueuedTimeoutInMinutes,
		ConcurrentBuildLimit:    cfg.ConcurrentBuildLimit,
		AutoRetryLimit:          cfg.AutoRetryLimit,
		Tags:                    tagsCopy,
		SecondarySources:        cfg.SecondarySources,
		SecondaryArtifacts:      cfg.SecondaryArtifacts,
		SecondarySourceVersions: cfg.SecondarySourceVersions,
		FileSystemLocations:     cfg.FileSystemLocations,
		Cache:                   cfg.Cache,
		LogsConfig:              cfg.LogsConfig,
		VpcConfig:               cfg.VpcConfig,
		BuildBatchConfig:        cfg.BuildBatchConfig,
		SourceVersion:           cfg.SourceVersion,
		Created:                 now,
		LastModified:            now,
	}

	if cfg.Source != nil {
		p.Source = *cfg.Source
	}

	if cfg.Artifacts != nil {
		p.Artifacts = *cfg.Artifacts
	}

	if cfg.Environment != nil {
		p.Environment = *cfg.Environment
	}

	b.applyBadge(p, cfg.BadgeEnabled, cfg.Name)

	b.projects.Put(p)

	out := *p

	return &out, nil
}

// BatchGetProjects returns projects by name or ARN. Missing names are returned separately.
func (b *InMemoryBackend) BatchGetProjects(names []string) ([]*Project, []string) {
	b.mu.RLock("BatchGetProjects")
	defer b.mu.RUnlock()

	found := make([]*Project, 0, len(names))
	notFound := make([]string, 0, len(names))

	for _, name := range names {
		if p, ok := b.lookupByNameOrARN(name); ok {
			out := *p
			found = append(found, &out)
		} else {
			notFound = append(notFound, name)
		}
	}

	return found, notFound
}

// applyProjectOptionalFields copies non-zero optional fields from cfg into p.
func applyProjectOptionalFields(p *Project, cfg ProjectConfig) {
	if cfg.Source != nil {
		p.Source = *cfg.Source
	}

	if cfg.Artifacts != nil {
		p.Artifacts = *cfg.Artifacts
	}

	if cfg.Environment != nil {
		p.Environment = *cfg.Environment
	}

	if cfg.SecondarySources != nil {
		p.SecondarySources = cfg.SecondarySources
	}

	if cfg.SecondaryArtifacts != nil {
		p.SecondaryArtifacts = cfg.SecondaryArtifacts
	}

	if cfg.SecondarySourceVersions != nil {
		p.SecondarySourceVersions = cfg.SecondarySourceVersions
	}

	if cfg.FileSystemLocations != nil {
		p.FileSystemLocations = cfg.FileSystemLocations
	}

	if cfg.Cache != nil {
		p.Cache = cfg.Cache
	}

	if cfg.LogsConfig != nil {
		p.LogsConfig = cfg.LogsConfig
	}

	if cfg.VpcConfig != nil {
		p.VpcConfig = cfg.VpcConfig
	}

	if cfg.BuildBatchConfig != nil {
		p.BuildBatchConfig = cfg.BuildBatchConfig
	}

	if cfg.SourceVersion != "" {
		p.SourceVersion = cfg.SourceVersion
	}
}

// UpdateProject updates fields on an existing project.
func (b *InMemoryBackend) UpdateProject(name string, cfg ProjectConfig) (*Project, error) {
	b.mu.Lock("UpdateProject")
	defer b.mu.Unlock()

	p, ok := b.lookupByNameOrARN(name)
	if !ok {
		return nil, ErrNotFound
	}

	if cfg.Description != "" {
		p.Description = cfg.Description
	}

	if cfg.ServiceRole != "" {
		p.ServiceRole = cfg.ServiceRole
	}

	if cfg.EncryptionKey != "" {
		p.EncryptionKey = cfg.EncryptionKey
	}

	if cfg.ResourceAccessRole != "" {
		p.ResourceAccessRole = cfg.ResourceAccessRole
	}

	if cfg.TimeoutInMinutes != 0 {
		p.TimeoutInMinutes = cfg.TimeoutInMinutes
	}

	if cfg.QueuedTimeoutInMinutes != 0 {
		p.QueuedTimeoutInMinutes = cfg.QueuedTimeoutInMinutes
	}

	if cfg.ConcurrentBuildLimit != 0 {
		p.ConcurrentBuildLimit = cfg.ConcurrentBuildLimit
	}

	if cfg.AutoRetryLimit != 0 {
		p.AutoRetryLimit = cfg.AutoRetryLimit
	}

	applyProjectOptionalFields(p, cfg)
	b.applyBadge(p, cfg.BadgeEnabled, p.Name)

	if len(cfg.Tags) > 0 {
		p.Tags = mergeTags(p.Tags, cfg.Tags)
	}

	p.LastModified = float64(time.Now().Unix())

	out := *p

	return &out, nil
}

// mergeTags returns a new map containing dst's entries merged with src.
func mergeTags(dst, src map[string]string) map[string]string {
	if dst == nil {
		dst = make(map[string]string, len(src))
	}

	maps.Copy(dst, src)

	return dst
}

// DeleteProject removes a project by name. Its builds are NOT deleted
// (api_op_DeleteProject.go: "Deletes a build project. When you delete a
// project, its builds are not deleted."). Idempotent: real AWS's
// DeleteProject declares no ResourceNotFoundException (botocore
// codebuild/2016-10-06/service-2.json operations.DeleteProject.errors: only
// InvalidInputException), so deleting an already-gone project is not an
// error.
func (b *InMemoryBackend) DeleteProject(name string) error {
	b.mu.Lock("DeleteProject")
	defer b.mu.Unlock()

	b.projects.Delete(name)

	return nil
}

// ListProjects returns all project names sorted by name, ascending.
func (b *InMemoryBackend) ListProjects() []string {
	return b.ListProjectsSortedBy("")
}

// ListProjectsSortedBy returns all project names ordered per sortBy
// (CREATED_TIME|LAST_MODIFIED_TIME|NAME; any other value, including "",
// defaults to NAME), always ascending. Callers apply sortOrder/pagination on
// top via [paginateIDs].
func (b *InMemoryBackend) ListProjectsSortedBy(sortBy string) []string {
	b.mu.RLock("ListProjectsSortedBy")
	defer b.mu.RUnlock()

	items := b.projects.Snapshot() // NAME-ascending by construction

	switch sortBy {
	case sortByCreatedTime:
		sort.SliceStable(items, func(i, j int) bool { return items[i].Created < items[j].Created })
	case sortByLastModifiedTime:
		sort.SliceStable(items, func(i, j int) bool { return items[i].LastModified < items[j].LastModified })
	}

	names := make([]string, len(items))
	for i, p := range items {
		names[i] = p.Name
	}

	return names
}

// UpdateProjectVisibility sets the visibility of a project by ARN.
// Returns the publicProjectAlias (non-empty only when visibility is PUBLIC_READ).
func (b *InMemoryBackend) UpdateProjectVisibility(projectArn, visibility string) (string, error) {
	b.mu.Lock("UpdateProjectVisibility")
	defer b.mu.Unlock()

	matches := b.projectsByARN.Get(projectArn)
	if len(matches) == 0 {
		return "", ErrNotFound
	}

	p := matches[0]
	p.Visibility = visibility

	if visibility == "PUBLIC_READ" {
		if p.PublicProjectAlias == "" {
			p.PublicProjectAlias = uuid.NewString()
		}
	} else {
		p.PublicProjectAlias = ""
	}

	return p.PublicProjectAlias, nil
}

// InvalidateProjectCache is a no-op cache invalidation (returns ErrNotFound if project missing).
func (b *InMemoryBackend) InvalidateProjectCache(projectName string) error {
	b.mu.RLock("InvalidateProjectCache")
	defer b.mu.RUnlock()

	if !b.projects.Has(projectName) {
		return ErrNotFound
	}

	return nil
}

// ListSharedProjects returns an empty list (no shared projects in emulator).
func (b *InMemoryBackend) ListSharedProjects() []string {
	return []string{}
}
