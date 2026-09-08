package apigateway

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// GetStages returns all stages for a REST API.
func (b *InMemoryBackend) GetStages(restAPIID string) ([]Stage, error) {
	b.mu.RLock("GetStages")
	defer b.mu.RUnlock()

	if !b.restApis.Has(restAPIID) {
		return nil, fmt.Errorf("%w: REST API %s not found", ErrRestAPINotFound, restAPIID)
	}

	group := b.stagesByAPI.Get(restAPIID)
	all := make([]Stage, 0, len(group))
	for _, s := range group {
		cp := *s
		cp.InvokeURL = stageInvokeURL(restAPIID, s.StageName)
		all = append(all, cp)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].StageName < all[j].StageName })

	return all, nil
}

// GetStage returns a single stage.
func (b *InMemoryBackend) GetStage(restAPIID, stageName string) (*Stage, error) {
	b.mu.RLock("GetStage")
	defer b.mu.RUnlock()

	if !b.restApis.Has(restAPIID) {
		return nil, fmt.Errorf("%w: REST API %s not found", ErrRestAPINotFound, restAPIID)
	}
	s, stageOK := b.stages.Get(stageKey(restAPIID, stageName))
	if !stageOK {
		return nil, fmt.Errorf("%w: stage %s not found", ErrResourceNotFound, stageName)
	}
	cp := *s
	cp.InvokeURL = stageInvokeURL(restAPIID, stageName)

	return &cp, nil
}

// DeleteStage removes a stage.
func (b *InMemoryBackend) DeleteStage(restAPIID, stageName string) error {
	b.mu.Lock("DeleteStage")
	defer b.mu.Unlock()

	if !b.restApis.Has(restAPIID) {
		return fmt.Errorf("%w: REST API %s not found", ErrRestAPINotFound, restAPIID)
	}
	if !b.stages.Delete(stageKey(restAPIID, stageName)) {
		return fmt.Errorf("%w: stage %s not found", ErrResourceNotFound, stageName)
	}
	b.clearStageThrottleBuckets(restAPIID, stageName)

	return nil
}

// CreateStage creates a new deployment stage for a REST API without creating a deployment.
func (b *InMemoryBackend) CreateStage(input CreateStageInput) (*Stage, error) {
	if input.RestAPIID == "" {
		return nil, fmt.Errorf("%w: restApiId is required", ErrInvalidParameter)
	}

	if input.StageName == "" {
		return nil, fmt.Errorf("%w: stageName is required", ErrInvalidParameter)
	}

	if input.DeploymentID == "" {
		return nil, fmt.Errorf("%w: deploymentId is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateStage")
	defer b.mu.Unlock()

	if !b.restApis.Has(input.RestAPIID) {
		return nil, fmt.Errorf("%w: REST API %s not found", ErrRestAPINotFound, input.RestAPIID)
	}

	if !b.deployments.Has(deploymentKey(input.RestAPIID, input.DeploymentID)) {
		return nil, fmt.Errorf("%w: deployment %s not found", ErrDeploymentNotFound, input.DeploymentID)
	}

	if b.stages.Has(stageKey(input.RestAPIID, input.StageName)) {
		return nil, fmt.Errorf("%w: stage %q already exists", ErrAlreadyExists, input.StageName)
	}

	variables := input.Variables
	if variables == nil {
		variables = make(map[string]string)
	}

	now := unixEpochTime{time.Now()}
	backendTags := tags.FromMap("apigw.stage."+input.RestAPIID+"."+input.StageName+".tags", input.Tags)
	stage := &Stage{
		Tags:                 backendTags,
		StageName:            input.StageName,
		RestAPIID:            input.RestAPIID,
		DeploymentID:         input.DeploymentID,
		Description:          input.Description,
		Variables:            variables,
		CreatedDate:          now,
		LastUpdatedDate:      now,
		CanarySettings:       input.CanarySettings,
		TracingEnabled:       input.TracingEnabled,
		CacheClusterEnabled:  input.CacheClusterEnabled,
		CacheClusterSize:     input.CacheClusterSize,
		CacheClusterStatus:   cacheClusterStatusFor(input.CacheClusterEnabled),
		DocumentationVersion: input.DocumentationVersion,
	}
	b.stages.Put(stage)

	cp := *stage

	return &cp, nil
}

// UpdateStage updates mutable fields on a deployment stage.
func (b *InMemoryBackend) UpdateStage(restAPIID, stageName string, input UpdateStageInput) (*Stage, error) {
	b.mu.Lock("UpdateStage")
	defer b.mu.Unlock()
	if !b.restApis.Has(restAPIID) {
		return nil, fmt.Errorf("%w: %s", ErrRestAPINotFound, restAPIID)
	}
	stage, ok := b.stages.Get(stageKey(restAPIID, stageName))
	if !ok {
		return nil, fmt.Errorf("%w: stage %q not found", ErrStageNotFound, stageName)
	}
	if input.DeploymentID != "" && !b.deployments.Has(deploymentKey(restAPIID, input.DeploymentID)) {
		return nil, fmt.Errorf("%w: deployment %s not found", ErrDeploymentNotFound, input.DeploymentID)
	}

	applyUpdateStageFields(stage, input)
	stage.LastUpdatedDate = unixEpochTime{time.Now()}
	cp := *stage

	return &cp, nil
}

// applyUpdateStageFields merges every UpdateStageInput field provided (the
// zero value means "not provided") onto stage in place. Split out of
// UpdateStage to keep that function's own cyclomatic complexity low.
func applyUpdateStageFields(stage *Stage, input UpdateStageInput) {
	if input.Description != "" {
		stage.Description = input.Description
	}
	if input.DeploymentID != "" {
		stage.DeploymentID = input.DeploymentID
	}
	if input.Variables != nil {
		stage.Variables = input.Variables
	}
	if input.CanarySettings != nil {
		stage.CanarySettings = input.CanarySettings
	}
	if input.AccessLogSettings != nil {
		stage.AccessLogSettings = input.AccessLogSettings
	}
	if input.MethodSettings != nil {
		stage.MethodSettings = input.MethodSettings
	}
	if input.TracingEnabled != nil {
		stage.TracingEnabled = *input.TracingEnabled
	}
	if input.ClientCertificateID != "" {
		stage.ClientCertificateID = input.ClientCertificateID
	}
	if input.CacheClusterEnabled != nil {
		stage.CacheClusterEnabled = *input.CacheClusterEnabled
		stage.CacheClusterStatus = cacheClusterStatusFor(stage.CacheClusterEnabled)
	}
	if input.CacheClusterSize != "" {
		stage.CacheClusterSize = input.CacheClusterSize
	}
	if input.DocumentationVersion != "" {
		stage.DocumentationVersion = input.DocumentationVersion
	}
}

// cacheClusterStatusFor derives the AWS CacheClusterStatus enum value
// ("AVAILABLE"/"NOT_AVAILABLE") from whether the stage's cache cluster is enabled.
func cacheClusterStatusFor(enabled bool) string {
	if enabled {
		return statusAvailable
	}

	return "NOT_AVAILABLE"
}
