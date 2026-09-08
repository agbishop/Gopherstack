package codebuild

import (
	"context"
	"fmt"
)

type startBuildInput struct {
	ArtifactsOverride                *ProjectArtifacts      `json:"artifactsOverride,omitempty"`
	CacheOverride                    *ProjectCache          `json:"cacheOverride,omitempty"`
	RegistryCredentialOverride       *RegistryCredential    `json:"registryCredentialOverride,omitempty"`
	FleetOverride                    *ProjectFleet          `json:"fleetOverride,omitempty"`
	SourceAuthOverride               *SourceAuth            `json:"sourceAuthOverride,omitempty"`
	BuildStatusConfigOverride        *BuildStatusConfig     `json:"buildStatusConfigOverride,omitempty"`
	GitSubmodulesConfigOverride      *GitSubmodulesConfig   `json:"gitSubmodulesConfigOverride,omitempty"`
	InsecureSslOverride              *bool                  `json:"insecureSslOverride,omitempty"`
	ReportBuildStatusOverride        *bool                  `json:"reportBuildStatusOverride,omitempty"`
	PrivilegedModeOverride           *bool                  `json:"privilegedModeOverride,omitempty"`
	GitCloneDepthOverride            *int32                 `json:"gitCloneDepthOverride,omitempty"`
	AutoRetryLimitOverride           *int32                 `json:"autoRetryLimitOverride,omitempty"`
	SourceVersion                    string                 `json:"sourceVersion,omitempty"`
	EncryptionKeyOverride            string                 `json:"encryptionKeyOverride,omitempty"`
	ImageOverride                    string                 `json:"imageOverride,omitempty"`
	ServiceRoleOverride              string                 `json:"serviceRoleOverride,omitempty"`
	BuildspecOverride                string                 `json:"buildspecOverride,omitempty"`
	SourceTypeOverride               string                 `json:"sourceTypeOverride,omitempty"`
	SourceLocationOverride           string                 `json:"sourceLocationOverride,omitempty"`
	EnvironmentTypeOverride          string                 `json:"environmentTypeOverride,omitempty"`
	CertificateOverride              string                 `json:"certificateOverride,omitempty"`
	ImagePullCredentialsTypeOverride string                 `json:"imagePullCredentialsTypeOverride,omitempty"`
	HostKernelOverride               string                 `json:"hostKernelOverride,omitempty"`
	ComputeTypeOverride              string                 `json:"computeTypeOverride,omitempty"`
	IdempotencyToken                 string                 `json:"idempotencyToken,omitempty"`
	ProjectName                      string                 `json:"projectName"`
	SecondaryArtifactsOverride       []ProjectArtifacts     `json:"secondaryArtifactsOverride,omitempty"`
	SecondarySourcesOverride         []ProjectSource        `json:"secondarySourcesOverride,omitempty"`
	SecondarySourcesVersionOverride  []ProjectSourceVersion `json:"secondarySourcesVersionOverride,omitempty"`
	EnvironmentVariablesOverride     []EnvironmentVariable  `json:"environmentVariablesOverride,omitempty"`
	TimeoutInMinutesOverride         int32                  `json:"timeoutInMinutesOverride,omitempty"`
	QueuedTimeoutInMinutesOverride   int32                  `json:"queuedTimeoutInMinutesOverride,omitempty"`
	DebugSessionEnabled              bool                   `json:"debugSessionEnabled,omitempty"`
}

type startBuildOutput struct {
	Build *Build `json:"build"`
}

func (h *Handler) handleStartBuild(
	_ context.Context,
	in *startBuildInput,
) (*startBuildOutput, error) {
	if in.ProjectName == "" {
		return nil, fmt.Errorf("%w: projectName is required", errInvalidRequest)
	}

	build, err := h.Backend.StartBuild(in.ProjectName, StartBuildConfig{
		EnvVarsOverride:                  in.EnvironmentVariablesOverride,
		BuildspecOverride:                in.BuildspecOverride,
		ComputeTypeOverride:              in.ComputeTypeOverride,
		ImageOverride:                    in.ImageOverride,
		ServiceRoleOverride:              in.ServiceRoleOverride,
		SourceVersion:                    in.SourceVersion,
		TimeoutInMinutesOverride:         in.TimeoutInMinutesOverride,
		DebugSessionEnabled:              in.DebugSessionEnabled,
		ArtifactsOverride:                in.ArtifactsOverride,
		CacheOverride:                    in.CacheOverride,
		RegistryCredentialOverride:       in.RegistryCredentialOverride,
		FleetOverride:                    in.FleetOverride,
		SourceAuthOverride:               in.SourceAuthOverride,
		BuildStatusConfigOverride:        in.BuildStatusConfigOverride,
		GitSubmodulesConfigOverride:      in.GitSubmodulesConfigOverride,
		InsecureSslOverride:              in.InsecureSslOverride,
		ReportBuildStatusOverride:        in.ReportBuildStatusOverride,
		PrivilegedModeOverride:           in.PrivilegedModeOverride,
		GitCloneDepthOverride:            in.GitCloneDepthOverride,
		SourceTypeOverride:               in.SourceTypeOverride,
		SourceLocationOverride:           in.SourceLocationOverride,
		EnvironmentTypeOverride:          in.EnvironmentTypeOverride,
		CertificateOverride:              in.CertificateOverride,
		ImagePullCredentialsTypeOverride: in.ImagePullCredentialsTypeOverride,
		HostKernelOverride:               in.HostKernelOverride,
		EncryptionKeyOverride:            in.EncryptionKeyOverride,
		SecondaryArtifactsOverride:       in.SecondaryArtifactsOverride,
		SecondarySourcesOverride:         in.SecondarySourcesOverride,
		SecondarySourcesVersionOverride:  in.SecondarySourcesVersionOverride,
		QueuedTimeoutInMinutesOverride:   in.QueuedTimeoutInMinutesOverride,
		AutoRetryLimitOverride:           in.AutoRetryLimitOverride,
	})
	if err != nil {
		return nil, err
	}

	return &startBuildOutput{Build: build}, nil
}

type batchGetBuildsInput struct {
	IDs []string `json:"ids"`
}

type batchGetBuildsOutput struct {
	Builds         []*Build `json:"builds"`
	BuildsNotFound []string `json:"buildsNotFound"`
}

func (h *Handler) handleBatchGetBuilds(
	_ context.Context,
	in *batchGetBuildsInput,
) (*batchGetBuildsOutput, error) {
	found, notFound := h.Backend.BatchGetBuilds(in.IDs)

	return &batchGetBuildsOutput{
		Builds:         found,
		BuildsNotFound: notFound,
	}, nil
}

type stopBuildInput struct {
	ID string `json:"id"`
}

type stopBuildOutput struct {
	Build *Build `json:"build"`
}

func (h *Handler) handleStopBuild(
	_ context.Context,
	in *stopBuildInput,
) (*stopBuildOutput, error) {
	if in.ID == "" {
		return nil, fmt.Errorf("%w: id is required", errInvalidRequest)
	}

	build, err := h.Backend.StopBuild(in.ID)
	if err != nil {
		return nil, err
	}

	return &stopBuildOutput{Build: build}, nil
}

type listBuildsForProjectInput struct {
	ProjectName string `json:"projectName"`
	NextToken   string `json:"nextToken"`
	SortOrder   string `json:"sortOrder"`
}

type listBuildsForProjectOutput struct {
	NextToken string   `json:"nextToken,omitempty"`
	IDs       []string `json:"ids"`
}

func (h *Handler) handleListBuildsForProject(
	_ context.Context,
	in *listBuildsForProjectInput,
) (*listBuildsForProjectOutput, error) {
	if in.ProjectName == "" {
		return nil, fmt.Errorf("%w: projectName is required", errInvalidRequest)
	}

	ids, err := h.Backend.ListBuildsForProject(in.ProjectName)
	if err != nil {
		return nil, err
	}

	if in.SortOrder != "" && len(ids) > defaultListPageSize {
		return nil, fmt.Errorf(
			"%w: sortOrder cannot be set when the project has more than %d builds",
			ErrValidation, defaultListPageSize,
		)
	}

	pg, err := paginateIDs(ids, in.NextToken, in.SortOrder, 0)
	if err != nil {
		return nil, err
	}

	return &listBuildsForProjectOutput{IDs: pg.Data, NextToken: pg.Next}, nil
}

type listBuildsInput struct {
	NextToken string `json:"nextToken"`
	SortOrder string `json:"sortOrder"`
}

type listBuildsOutput struct {
	NextToken string   `json:"nextToken,omitempty"`
	IDs       []string `json:"ids"`
}

func (h *Handler) handleListBuilds(_ context.Context, in *listBuildsInput) (*listBuildsOutput, error) {
	ids := h.Backend.ListBuilds()

	pg, err := paginateIDs(ids, in.NextToken, in.SortOrder, 0)
	if err != nil {
		return nil, err
	}

	return &listBuildsOutput{IDs: pg.Data, NextToken: pg.Next}, nil
}

type batchDeleteBuildsInput struct {
	IDs []string `json:"ids"`
}

type buildNotDeleted struct {
	ID         string `json:"id"`
	StatusCode string `json:"statusCode"`
}

type batchDeleteBuildsOutput struct {
	BuildsDeleted    []string          `json:"buildsDeleted"`
	BuildsNotDeleted []buildNotDeleted `json:"buildsNotDeleted"`
}

func (h *Handler) handleBatchDeleteBuilds(
	_ context.Context,
	in *batchDeleteBuildsInput,
) (*batchDeleteBuildsOutput, error) {
	if len(in.IDs) == 0 {
		return &batchDeleteBuildsOutput{
			BuildsDeleted:    []string{},
			BuildsNotDeleted: []buildNotDeleted{},
		}, nil
	}

	deleted := h.Backend.BatchDeleteBuilds(in.IDs)
	deletedSet := make(map[string]struct{}, len(deleted))

	for _, id := range deleted {
		deletedSet[id] = struct{}{}
	}

	notDeleted := make([]buildNotDeleted, 0)

	for _, id := range in.IDs {
		if _, ok := deletedSet[id]; !ok {
			notDeleted = append(notDeleted, buildNotDeleted{ID: id, StatusCode: "BUILD_ID_NOT_FOUND"})
		}
	}

	return &batchDeleteBuildsOutput{
		BuildsDeleted:    deleted,
		BuildsNotDeleted: notDeleted,
	}, nil
}

type retryBuildInput struct {
	ID string `json:"id"`
}

type retryBuildOutput struct {
	Build *Build `json:"build"`
}

func (h *Handler) handleRetryBuild(
	_ context.Context,
	in *retryBuildInput,
) (*retryBuildOutput, error) {
	if in.ID == "" {
		return nil, fmt.Errorf("%w: id is required", errInvalidRequest)
	}

	build, err := h.Backend.RetryBuild(in.ID)
	if err != nil {
		return nil, err
	}

	return &retryBuildOutput{Build: build}, nil
}
