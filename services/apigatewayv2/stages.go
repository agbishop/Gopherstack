package apigatewayv2

import (
	"fmt"
	"sort"
	"time"
)

// CreateStage creates a new stage for an API.
func (b *InMemoryBackend) CreateStage(apiID string, input CreateStageInput) (*Stage, error) {
	b.mu.Lock("CreateStage")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	if input.StageName == "" {
		return nil, fmt.Errorf("%w: stageName is required", ErrBadRequest)
	}

	if b.stages.Has(stageKey(apiID, input.StageName)) {
		return nil, fmt.Errorf("%w: stage %q already exists", ErrAlreadyExists, input.StageName)
	}

	now := isoTime{time.Now()}
	stage := &Stage{
		StageName:            input.StageName,
		APIID:                apiID,
		DeploymentID:         input.DeploymentID,
		Description:          input.Description,
		ClientCertificateID:  input.ClientCertificateID,
		AutoDeploy:           input.AutoDeploy,
		StageVariables:       input.StageVariables,
		Tags:                 copyTags(input.Tags),
		CreatedDate:          now,
		LastUpdatedDate:      now,
		AccessLogSettings:    input.AccessLogSettings,
		DefaultRouteSettings: input.DefaultRouteSettings,
		RouteSettings:        input.RouteSettings,
	}

	b.stages.Put(stage)

	cp := *stage

	return &cp, nil
}

// GetStage retrieves a stage by name.
func (b *InMemoryBackend) GetStage(apiID, stageName string) (*Stage, error) {
	b.mu.RLock("GetStage")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	s, ok := b.stages.Get(stageKey(apiID, stageName))
	if !ok {
		return nil, ErrStageNotFound
	}

	cp := *s

	return &cp, nil
}

// GetStages retrieves all stages for an API.
func (b *InMemoryBackend) GetStages(apiID string) ([]Stage, error) {
	b.mu.RLock("GetStages")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	stages := b.stagesByAPI.Get(apiID)
	result := make([]Stage, 0, len(stages))

	for _, s := range stages {
		result = append(result, *s)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StageName < result[j].StageName
	})

	return result, nil
}

// DeleteStage removes a stage from an API.
func (b *InMemoryBackend) DeleteStage(apiID, stageName string) error {
	b.mu.Lock("DeleteStage")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return ErrAPINotFound
	}

	if !b.stages.Delete(stageKey(apiID, stageName)) {
		return ErrStageNotFound
	}

	b.clearStageThrottleBuckets(apiID, stageName)

	return nil
}

// UpdateStage updates fields on an existing stage.
func (b *InMemoryBackend) UpdateStage(apiID, stageName string, input UpdateStageInput) (*Stage, error) {
	b.mu.Lock("UpdateStage")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	s, ok := b.stages.Get(stageKey(apiID, stageName))
	if !ok {
		return nil, ErrStageNotFound
	}

	// Real AWS: "If you created an API using quick create, the $default
	// stage is managed by API Gateway. You can't modify the $default
	// stage." (service-2.json, Stage.ApiGatewayManaged doc).
	if s.APIGatewayManaged {
		return nil, fmt.Errorf("%w: a quick-create managed stage can't be modified", ErrBadRequest)
	}

	if input.DeploymentID != "" {
		s.DeploymentID = input.DeploymentID
	}

	if input.Description != "" {
		s.Description = input.Description
	}

	if input.ClientCertificateID != "" {
		s.ClientCertificateID = input.ClientCertificateID
	}

	if input.AutoDeploy != nil {
		s.AutoDeploy = *input.AutoDeploy
	}

	if input.StageVariables != nil {
		s.StageVariables = input.StageVariables
	}

	if input.AccessLogSettings != nil {
		clone := *input.AccessLogSettings
		s.AccessLogSettings = &clone
	}

	if input.DefaultRouteSettings != nil {
		clone := *input.DefaultRouteSettings
		s.DefaultRouteSettings = &clone
	}

	if input.RouteSettings != nil {
		s.RouteSettings = input.RouteSettings
	}

	s.LastUpdatedDate = isoTime{time.Now()}

	cp := *s

	return &cp, nil
}

// DeleteAccessLogSettings clears the access log settings on a stage.
func (b *InMemoryBackend) DeleteAccessLogSettings(apiID, stageName string) error {
	b.mu.Lock("DeleteAccessLogSettings")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return ErrAPINotFound
	}

	s, ok := b.stages.Get(stageKey(apiID, stageName))
	if !ok {
		return ErrStageNotFound
	}

	s.AccessLogSettings = nil
	s.LastUpdatedDate = isoTime{time.Now()}

	return nil
}

// DeleteRouteSettings removes per-route settings for a specific routeKey from a stage.
func (b *InMemoryBackend) DeleteRouteSettings(apiID, stageName, routeKey string) error {
	b.mu.Lock("DeleteRouteSettings")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return ErrAPINotFound
	}

	s, ok := b.stages.Get(stageKey(apiID, stageName))
	if !ok {
		return ErrStageNotFound
	}

	if s.RouteSettings != nil {
		delete(s.RouteSettings, routeKey)
	}

	s.LastUpdatedDate = isoTime{time.Now()}

	return nil
}
