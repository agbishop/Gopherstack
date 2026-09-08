package appconfig

import (
	"fmt"
	"maps"
	"sort"
	"time"
)

// CreateEnvironment creates a new environment within an application. See
// CreateExperimentDefinition's doc comment for why tags are applied
// directly to b.tags rather than via TagResource.
func (b *InMemoryBackend) CreateEnvironment(
	applicationID, name, description string,
	monitors []Monitor,
	tags map[string]string,
) (*Environment, error) {
	b.mu.Lock("CreateEnvironment")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrBadRequest)
	}

	if !b.applications.Has(applicationID) {
		return nil, fmt.Errorf("%w: application %s", ErrApplicationNotFound, applicationID)
	}

	if len(b.environmentsByAppName.Get(appNameKey(applicationID, name))) > 0 {
		return nil, fmt.Errorf(
			"%w: environment with name %q already exists in application %s",
			ErrConflict,
			name,
			applicationID,
		)
	}

	now := time.Now()
	env := &Environment{
		ID:            newResourceID(),
		ApplicationID: applicationID,
		Name:          name,
		Description:   description,
		State:         "READY_FOR_DEPLOYMENT",
		Monitors:      monitors,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	b.environments.Put(env)

	if len(tags) > 0 {
		b.tags[b.appconfigARN("application/"+applicationID+"/environment/"+env.ID)] = maps.Clone(tags)
	}

	cp := *env

	return &cp, nil
}

// GetEnvironment retrieves an environment by application and environment ID.
func (b *InMemoryBackend) GetEnvironment(
	applicationID, environmentID string,
) (*Environment, error) {
	b.mu.RLock("GetEnvironment")
	defer b.mu.RUnlock()

	env, ok := b.environments.Get(environmentID)
	if !ok || env.ApplicationID != applicationID {
		return nil, fmt.Errorf("%w: environment %s", ErrEnvironmentNotFound, environmentID)
	}

	cp := *env

	return &cp, nil
}

// ListEnvironments returns paginated environments for an application.
func (b *InMemoryBackend) ListEnvironments(
	applicationID, nextToken string,
	maxResults int,
) ([]Environment, string, error) {
	b.mu.RLock("ListEnvironments")
	defer b.mu.RUnlock()

	if !b.applications.Has(applicationID) {
		return nil, "", fmt.Errorf("%w: application %s", ErrApplicationNotFound, applicationID)
	}

	envs := b.environmentsByApp.Get(applicationID)
	out := make([]Environment, 0, len(envs))

	for _, e := range envs {
		out = append(out, *e)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	page, token := appConfigPaginate(out, nextToken, b.paginationSecret, maxResults)

	return page, token, nil
}

// UpdateEnvironment updates an environment's name, description, and
// monitors. A nil name/description/monitors means the request omitted that
// field, and AWS AppConfig leaves an omitted field unchanged rather than
// clearing it.
func (b *InMemoryBackend) UpdateEnvironment(
	applicationID, environmentID string,
	name, description *string,
	monitors *[]Monitor,
) (*Environment, error) {
	b.mu.Lock("UpdateEnvironment")
	defer b.mu.Unlock()

	existing, ok := b.environments.Get(environmentID)
	if !ok || existing.ApplicationID != applicationID {
		return nil, fmt.Errorf("%w: environment %s", ErrEnvironmentNotFound, environmentID)
	}

	updated := *existing
	if name != nil && *name != "" {
		updated.Name = *name
	}

	if description != nil {
		updated.Description = *description
	}

	if monitors != nil {
		updated.Monitors = *monitors
	}

	updated.UpdatedAt = time.Now()
	b.environments.Put(&updated)
	cp := updated

	return &cp, nil
}

// DeleteEnvironment deletes an environment. deletionProtectionCheck is the
// DeleteEnvironmentInput.DeletionProtectionCheck header value (BYPASS, APPLY,
// ACCOUNT_DEFAULT, or "" for an absent header, which behaves like
// ACCOUNT_DEFAULT).
func (b *InMemoryBackend) DeleteEnvironment(applicationID, environmentID, deletionProtectionCheck string) error {
	b.mu.Lock("DeleteEnvironment")
	defer b.mu.Unlock()

	env, ok := b.environments.Get(environmentID)
	if !ok || env.ApplicationID != applicationID {
		return fmt.Errorf("%w: environment %s", ErrEnvironmentNotFound, environmentID)
	}

	if err := b.checkDeletionProtectionLocked(
		deletionProtectionCheck, applicationID, environmentID, "", env.CreatedAt,
	); err != nil {
		return err
	}

	envArn := b.appconfigARN("application/" + applicationID + "/environment/" + environmentID)
	b.environments.Delete(environmentID)
	delete(b.tags, envArn)
	b.deleteExtensionAssociationsForResourceLocked(envArn)
	b.deleteDeployedConfigsLocked(func(appID, envID, _ string) bool {
		return appID == applicationID && envID == environmentID
	})

	return nil
}

// environmentOutput is the real API response shape for an environment --
// types.Environment (aws-sdk-go-v2/service/appconfig@v1.48.4 types/types.go,
// checked 2026-08-13) has ApplicationId/Description/Id/Monitors/Name/State
// only, no CreatedAt/UpdatedAt. Environment itself keeps those two fields
// (with JSON tags) for Snapshot/Restore; this converter strips them for the
// wire.
type environmentOutput struct {
	ApplicationID string    `json:"ApplicationId"`
	ID            string    `json:"Id"`
	Name          string    `json:"Name"`
	Description   string    `json:"Description,omitempty"`
	State         string    `json:"State"`
	Monitors      []Monitor `json:"Monitors,omitempty"`
}

func environmentToOutput(e Environment) environmentOutput {
	return environmentOutput{
		ApplicationID: e.ApplicationID,
		ID:            e.ID,
		Name:          e.Name,
		Description:   e.Description,
		State:         e.State,
		Monitors:      e.Monitors,
	}
}
