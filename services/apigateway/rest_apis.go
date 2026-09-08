package apigateway

import (
	"fmt"
	"sort"
	"time"
)

// CreateRestAPI creates a new REST API and its root resource.
func (b *InMemoryBackend) CreateRestAPI(input CreateRestAPIInput) (*RestAPI, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateRestAPI")
	defer b.mu.Unlock()

	id := randomID(apiIDLength)
	backendTags := initTagsFromInput("apigw.api."+id+".tags", input.Tags)
	rootID := randomID(resourceIDLength)

	api := &RestAPI{
		ID:                     id,
		Name:                   input.Name,
		Description:            input.Description,
		CreatedDate:            unixEpochTime{time.Now()},
		Tags:                   backendTags,
		RootResourceID:         rootID,
		BinaryMediaTypes:       input.BinaryMediaTypes,
		EndpointConfiguration:  input.EndpointConfiguration,
		Policy:                 input.Policy,
		APIKeySource:           input.APIKeySource,
		MinimumCompressionSize: input.MinimumCompressionSize,
		// APIStatus is AWS-managed and always AVAILABLE: gopherstack creates
		// RestApis synchronously with no UPDATING/PENDING/FAILED transition.
		APIStatus:                 statusAvailable,
		DisableExecuteAPIEndpoint: input.DisableExecuteAPIEndpoint,
		EndpointAccessMode:        input.EndpointAccessMode,
	}

	root := &Resource{
		ID:              rootID,
		ParentID:        "",
		PathPart:        "",
		Path:            "/",
		RestAPIID:       id,
		ResourceMethods: make(map[string]*Method),
	}

	b.restApis.Put(api)
	b.resources.Put(root)

	cp := *api

	return &cp, nil
}

// DeleteRestAPI removes a REST API and all its resources.
func (b *InMemoryBackend) DeleteRestAPI(restAPIID string) error {
	b.mu.Lock("DeleteRestAPI")
	defer b.mu.Unlock()

	api, ok := b.restApis.Get(restAPIID)
	if !ok {
		return fmt.Errorf("%w: REST API %s not found", ErrRestAPINotFound, restAPIID)
	}
	api.Tags.Close()
	b.restApis.Delete(restAPIID)
	b.deleteAPIChildrenLocked(restAPIID)

	return nil
}

// deleteAPIChildrenLocked removes every resource-family entry scoped to
// restAPIID (resources, deployments, stages, authorizers, requestValidators,
// documentationParts, documentationVersions, models, gatewayResponses) via
// each table's "byAPI" index (gatewayResponses has none, so it's scanned
// directly). Callers must hold b.mu.
func (b *InMemoryBackend) deleteAPIChildrenLocked(restAPIID string) {
	for _, r := range append([]*Resource{}, b.resourcesByAPI.Get(restAPIID)...) {
		b.resources.Delete(resourceKeyFn(r))
	}
	for _, d := range append([]*Deployment{}, b.deploymentsByAPI.Get(restAPIID)...) {
		b.deployments.Delete(deploymentKeyFn(d))
	}
	for _, s := range append([]*Stage{}, b.stagesByAPI.Get(restAPIID)...) {
		b.stages.Delete(stageKeyFn(s))
	}
	for _, a := range append([]*Authorizer{}, b.authorizersByAPI.Get(restAPIID)...) {
		b.authorizers.Delete(authorizerKeyFn(a))
	}
	for _, v := range append([]*RequestValidator{}, b.requestValidatorsByAPI.Get(restAPIID)...) {
		b.requestValidators.Delete(requestValidatorKeyFn(v))
	}
	for _, p := range append([]*DocumentationPart{}, b.documentationPartsByAPI.Get(restAPIID)...) {
		b.documentationParts.Delete(documentationPartKeyFn(p))
	}
	for _, v := range append([]*DocumentationVersion{}, b.documentationVersionsByAPI.Get(restAPIID)...) {
		b.documentationVersions.Delete(documentationVersionKeyFn(v))
	}
	for _, m := range append([]*Model{}, b.modelsByAPI.Get(restAPIID)...) {
		b.models.Delete(modelKeyFn(m))
	}
	b.deleteGatewayResponsesForAPILocked(restAPIID)
	delete(b.resourceVersions, restAPIID)
}

// GetRestAPI returns a single REST API.
func (b *InMemoryBackend) GetRestAPI(restAPIID string) (*RestAPI, error) {
	b.mu.RLock("GetRestAPI")
	defer b.mu.RUnlock()

	api, ok := b.restApis.Get(restAPIID)
	if !ok {
		return nil, fmt.Errorf("%w: REST API %s not found", ErrRestAPINotFound, restAPIID)
	}
	cp := *api

	return &cp, nil
}

// GetRestAPIs returns all REST APIs with pagination.
func (b *InMemoryBackend) GetRestAPIs(limit int, position string) ([]RestAPI, string, error) {
	b.mu.RLock("GetRestAPIs")
	defer b.mu.RUnlock()

	all := make([]RestAPI, 0, b.restApis.Len())
	for _, api := range b.restApis.All() {
		all = append(all, *api)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	page, pos := paginatePageByKey(all, limit, position, func(a RestAPI) string { return a.ID })

	return page, pos, nil
}

// UpdateRestAPI updates the name and/or description of a REST API.
func (b *InMemoryBackend) UpdateRestAPI(restAPIID string, input UpdateRestAPIInput) (*RestAPI, error) {
	b.mu.Lock("UpdateRestAPI")
	defer b.mu.Unlock()

	api, ok := b.restApis.Get(restAPIID)
	if !ok {
		return nil, fmt.Errorf("%w: REST API %s not found", ErrRestAPINotFound, restAPIID)
	}

	if input.Name != "" {
		api.Name = input.Name
	}

	// Description is a *string (see UpdateRestAPIInput's doc comment): a
	// non-nil pointer means the PATCH touched this field at all, including an
	// explicit "remove" (which patch.go encodes as a pointer to "").
	if input.Description != nil {
		api.Description = *input.Description
	}

	if input.Policy != "" {
		api.Policy = input.Policy
	}

	if input.APIKeySource != "" {
		api.APIKeySource = input.APIKeySource
	}

	if input.EndpointAccessMode != "" {
		api.EndpointAccessMode = input.EndpointAccessMode
	}

	if input.SecurityPolicy != "" {
		api.SecurityPolicy = input.SecurityPolicy
	}

	if input.DisableExecuteAPIEndpoint != nil {
		api.DisableExecuteAPIEndpoint = *input.DisableExecuteAPIEndpoint
	}

	if input.BinaryMediaTypes != nil {
		api.BinaryMediaTypes = input.BinaryMediaTypes
	}

	if input.EndpointConfiguration != nil {
		api.EndpointConfiguration = input.EndpointConfiguration
	}

	if input.MinimumCompressionSize != nil {
		api.MinimumCompressionSize = *input.MinimumCompressionSize
	}

	cp := *api

	return &cp, nil
}
