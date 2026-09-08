package apigatewayv2

import (
	"sort"
	"time"
)

// snapshotRoutingLocked copies the API's current routes and integrations for
// pinning into a Deployment. The data plane (handleHTTPAPIProxy) matches
// against this frozen copy for any stage pinned to the resulting deployment,
// instead of the API's live, mutable state (gopherstack-cfr1). Caller must
// hold b.mu.Lock.
func (b *InMemoryBackend) snapshotRoutingLocked(apiID string) ([]Route, []Integration) {
	liveRoutes := b.routesByAPI.Get(apiID)
	routes := make([]Route, len(liveRoutes))

	for i, r := range liveRoutes {
		routes[i] = *r
	}

	liveIntegrations := b.integrationsByAPI.Get(apiID)
	integrations := make([]Integration, len(liveIntegrations))

	for i, integ := range liveIntegrations {
		integrations[i] = *integ
	}

	return routes, integrations
}

// CreateDeployment creates a new deployment for an API.
func (b *InMemoryBackend) CreateDeployment(apiID string, input CreateDeploymentInput) (*Deployment, error) {
	b.mu.Lock("CreateDeployment")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	routes, integrations := b.snapshotRoutingLocked(apiID)

	id := randomID()
	deployment := &Deployment{
		DeploymentID:     id,
		APIID:            apiID,
		Description:      input.Description,
		DeploymentStatus: "DEPLOYED",
		CreatedDate:      isoTime{time.Now()},
		Routes:           routes,
		Integrations:     integrations,
	}

	b.deployments.Put(deployment)

	// When a stage name is provided, link the deployment to that stage (AWS behaviour).
	if input.StageName != "" {
		s, stageExists := b.stages.Get(stageKey(apiID, input.StageName))
		if !stageExists {
			return nil, ErrStageNotFound
		}

		s.DeploymentID = id
	}

	cp := *deployment

	return &cp, nil
}

// autoDeployLocked triggers an automatic deployment for every stage of the API
// that has AutoDeploy enabled. Real API Gateway v2 creates a fresh deployment
// and repoints the stage at it whenever a route, integration, or other routing
// configuration changes on an auto-deploy-enabled stage. The caller must hold
// b.mu.Lock.
func (b *InMemoryBackend) autoDeployLocked(apiID string) {
	stages := b.stagesByAPI.Get(apiID)

	hasAutoDeploy := false

	for _, s := range stages {
		if s.AutoDeploy {
			hasAutoDeploy = true

			break
		}
	}

	if !hasAutoDeploy {
		return
	}

	now := isoTime{time.Now()}
	routes, integrations := b.snapshotRoutingLocked(apiID)

	for _, s := range stages {
		if !s.AutoDeploy {
			continue
		}

		id := randomID()
		b.deployments.Put(&Deployment{
			DeploymentID:     id,
			APIID:            apiID,
			Description:      "Automatic deployment triggered by changes to the Api configuration",
			DeploymentStatus: "DEPLOYED",
			AutoDeployed:     true,
			CreatedDate:      now,
			Routes:           routes,
			Integrations:     integrations,
		})
		s.DeploymentID = id
		s.LastUpdatedDate = now
	}
}

// GetDeployment retrieves a deployment by ID.
func (b *InMemoryBackend) GetDeployment(apiID, deploymentID string) (*Deployment, error) {
	b.mu.RLock("GetDeployment")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	dep, ok := b.deployments.Get(deploymentKey(apiID, deploymentID))
	if !ok {
		return nil, ErrDeploymentNotFound
	}

	cp := *dep

	return &cp, nil
}

// GetDeployments retrieves all deployments for an API.
func (b *InMemoryBackend) GetDeployments(apiID string) ([]Deployment, error) {
	b.mu.RLock("GetDeployments")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	deployments := b.deploymentsByAPI.Get(apiID)
	result := make([]Deployment, 0, len(deployments))

	for _, dep := range deployments {
		result = append(result, *dep)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].DeploymentID < result[j].DeploymentID
	})

	return result, nil
}

// DeleteDeployment removes a deployment from an API.
func (b *InMemoryBackend) DeleteDeployment(apiID, deploymentID string) error {
	b.mu.Lock("DeleteDeployment")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return ErrAPINotFound
	}

	if !b.deployments.Delete(deploymentKey(apiID, deploymentID)) {
		return ErrDeploymentNotFound
	}

	return nil
}

// UpdateDeployment updates fields on an existing deployment.
func (b *InMemoryBackend) UpdateDeployment(
	apiID, deploymentID string,
	input UpdateDeploymentInput,
) (*Deployment, error) {
	b.mu.Lock("UpdateDeployment")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	dep, ok := b.deployments.Get(deploymentKey(apiID, deploymentID))
	if !ok {
		return nil, ErrDeploymentNotFound
	}

	if input.Description != "" {
		dep.Description = input.Description
	}

	cp := *dep

	return &cp, nil
}
