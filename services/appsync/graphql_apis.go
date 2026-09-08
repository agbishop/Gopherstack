package appsync

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	apiTypeGraphQL = "GRAPHQL"
)

// maxEnvironmentVariables is the AWS-enforced limit on environment variables per GraphQL API.
const maxEnvironmentVariables = 25

// isValidAuthType returns true if the given authentication type is valid.
func isValidAuthType(a AuthenticationType) bool {
	switch a {
	case AuthTypeAPIKey, AuthTypeIAM, AuthTypeCognito, AuthTypeOIDC, AuthTypeLambda:
		return true
	default:
		return false
	}
}

// isValidGraphqlAPIType returns true if the given API type is valid.
func isValidGraphqlAPIType(t string) bool {
	return t == apiTypeGraphQL || t == "MERGED"
}

// CreateGraphqlAPI creates a new GraphQL API.
func (b *InMemoryBackend) CreateGraphqlAPI(
	name string,
	authType AuthenticationType,
	xrayEnabled bool,
	apiType string,
	visibility string,
	additionalAuthProviders []AdditionalAuthenticationProvider,
	tagMap map[string]string,
	cfg *GraphqlAPIConfig,
) (*GraphqlAPI, error) {
	b.mu.Lock("CreateGraphqlApi")
	defer b.mu.Unlock()

	if authType != "" && !isValidAuthType(authType) {
		return nil, fmt.Errorf("%w: invalid authenticationType %q", ErrValidation, authType)
	}

	if authType == "" {
		authType = AuthTypeAPIKey
	}

	if apiType == "" {
		apiType = apiTypeGraphQL
	} else if !isValidGraphqlAPIType(apiType) {
		return nil, fmt.Errorf("%w: invalid apiType %q, must be GRAPHQL or MERGED", ErrValidation, apiType)
	}

	if visibility == "" {
		visibility = VisibilityGlobal
	} else if visibility != VisibilityGlobal && visibility != VisibilityPrivate {
		return nil, fmt.Errorf("%w: invalid visibility %q, must be GLOBAL or PRIVATE", ErrValidation, visibility)
	}

	apiID := randomAPIID()
	apiARN := arn.Build("appsync", b.region, b.accountID, "apis/"+apiID)

	graphqlEndpoint := fmt.Sprintf("%s/v1/apis/%s/graphql", b.endpoint, apiID)

	now := time.Now().Unix()

	api := &GraphqlAPI{
		APIID:                             apiID,
		ARN:                               apiARN,
		Name:                              name,
		AuthenticationType:                authType,
		Visibility:                        visibility,
		AdditionalAuthenticationProviders: additionalAuthProviders,
		Region:                            b.region,
		Owner:                             b.accountID,
		XrayEnabled:                       xrayEnabled,
		APIType:                           apiType,
		IntrospectionConfig:               IntrospectionConfigEnabled,
		CreatedAt:                         now,
		UpdatedAt:                         now,
		URIs: map[string]string{
			apiTypeGraphQL: graphqlEndpoint,
			"REALTIME":     graphqlEndpoint,
		},
		Tags: tags.New("appsync.api." + apiID + ".tags"),
	}

	applyGraphqlAPIConfig(api, cfg)

	for k, v := range tagMap {
		api.Tags.Set(k, v)
	}

	b.apis.Put(api)

	cp := *api

	return &cp, nil
}

// applyGraphqlAPIConfig applies optional auth/logging config onto a GraphqlAPI.
func applyGraphqlAPIConfig(api *GraphqlAPI, cfg *GraphqlAPIConfig) {
	if cfg == nil {
		return
	}

	if cfg.UserPoolConfig != nil {
		api.UserPoolConfig = cfg.UserPoolConfig
	}

	if cfg.OpenIDConnectConfig != nil {
		api.OpenIDConnectConfig = cfg.OpenIDConnectConfig
	}

	if cfg.LambdaAuthorizerConfig != nil {
		api.LambdaAuthorizerConfig = cfg.LambdaAuthorizerConfig
	}

	if cfg.LogConfig != nil {
		api.LogConfig = cfg.LogConfig
	}

	if cfg.IntrospectionConfig != "" {
		api.IntrospectionConfig = cfg.IntrospectionConfig
	}

	if cfg.OwnerContact != "" {
		api.OwnerContact = cfg.OwnerContact
	}

	if cfg.QueryDepthLimit != 0 {
		api.QueryDepthLimit = cfg.QueryDepthLimit
	}

	if cfg.ResolverCountLimit != 0 {
		api.ResolverCountLimit = cfg.ResolverCountLimit
	}
}

// GetGraphqlAPI returns a GraphQL API by ID.
func (b *InMemoryBackend) GetGraphqlAPI(apiID string) (*GraphqlAPI, error) {
	b.mu.RLock("GetGraphqlApi")
	defer b.mu.RUnlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return nil, fmt.Errorf("%w: api %s not found", ErrNotFound, apiID)
	}

	cp := *api

	return &cp, nil
}

// UpdateGraphqlAPI updates an existing GraphQL API's name and/or authentication type.
func (b *InMemoryBackend) UpdateGraphqlAPI(
	apiID, name string,
	authType AuthenticationType,
	xrayEnabled *bool,
	visibility string,
	additionalAuthProviders []AdditionalAuthenticationProvider,
	cfg *GraphqlAPIConfig,
) (*GraphqlAPI, error) {
	b.mu.Lock("UpdateGraphqlApi")
	defer b.mu.Unlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return nil, fmt.Errorf("%w: api %s not found", ErrNotFound, apiID)
	}

	if authType != "" && !isValidAuthType(authType) {
		return nil, fmt.Errorf("%w: invalid authenticationType %q", ErrValidation, authType)
	}

	if visibility != "" && visibility != VisibilityGlobal && visibility != VisibilityPrivate {
		return nil, fmt.Errorf("%w: invalid visibility %q, must be GLOBAL or PRIVATE", ErrValidation, visibility)
	}

	if name != "" {
		api.Name = name
	}

	if authType != "" {
		api.AuthenticationType = authType
	}

	if xrayEnabled != nil {
		api.XrayEnabled = *xrayEnabled
	}

	if visibility != "" {
		api.Visibility = visibility
	}

	if additionalAuthProviders != nil {
		api.AdditionalAuthenticationProviders = additionalAuthProviders
	}

	applyGraphqlAPIConfig(api, cfg)

	api.UpdatedAt = time.Now().Unix()

	cp := *api

	return &cp, nil
}

// ListGraphqlAPIs returns all GraphQL APIs, optionally filtered by apiType (apiTypeGraphQL or "MERGED").
func (b *InMemoryBackend) ListGraphqlAPIs(apiType string) ([]*GraphqlAPI, error) {
	b.mu.RLock("ListGraphqlApis")
	defer b.mu.RUnlock()

	apis := b.apis.All()
	out := make([]*GraphqlAPI, 0, len(apis))

	for _, api := range apis {
		if apiType != "" && api.APIType != apiType {
			continue
		}

		cp := *api
		out = append(out, &cp)
	}

	slices.SortFunc(out, func(a, b *GraphqlAPI) int {
		return strings.Compare(a.Name, b.Name)
	})

	return out, nil
}

// DeleteGraphqlAPI deletes a GraphQL API by ID.
func (b *InMemoryBackend) DeleteGraphqlAPI(apiID string) error {
	b.mu.Lock("DeleteGraphqlApi")
	defer b.mu.Unlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return fmt.Errorf("%w: api %s not found", ErrNotFound, apiID)
	}

	// Snapshot data sources before deletion so we can release their tag
	// resources. Cloned because Table.Delete below mutates the same
	// index-owned backing slice datasourcesByAPI.Get returns.
	dss := slices.Clone(b.datasourcesByAPI.Get(apiID))

	b.apis.Delete(apiID)
	b.schemas.Delete(apiID)

	for _, ds := range dss {
		b.datasources.Delete(datasourceKey(apiID, ds.Name))
	}

	for _, r := range slices.Clone(b.resolversByAPI.Get(apiID)) {
		b.resolvers.Delete(resolverTableKey(apiID, r.TypeName, r.FieldName))
	}

	// Cascade-delete sub-resources added as part of issue #842.
	delete(b.apiKeys, apiID)
	b.apiCaches.Delete(apiID)

	for _, fn := range slices.Clone(b.functionsByAPI.Get(apiID)) {
		b.functions.Delete(functionKey(apiID, fn.FunctionID))
	}

	for _, t := range slices.Clone(b.typesByAPI.Get(apiID)) {
		b.types.Delete(apiTypeKey(apiID, t.Name))
	}

	b.cascadeDeleteAPIAssociations(apiID)

	if api.Tags != nil {
		api.Tags.Close()
	}

	for _, ds := range dss {
		if ds != nil && ds.Tags != nil {
			ds.Tags.Close()
		}
	}

	return nil
}

// cascadeDeleteAPIAssociations removes source/merged API associations and the
// domain name association referencing apiID -- otherwise they outlive the
// API, leaving Get/ListSourceAPIAssociations and GetApiAssociation pointing
// at a resource that no longer exists. Caller must hold the write lock.
func (b *InMemoryBackend) cascadeDeleteAPIAssociations(apiID string) {
	for _, assoc := range slices.Clone(b.sourceAssocs.All()) {
		if assoc.SourceAPIID == apiID || assoc.MergedAPIID == apiID {
			b.sourceAssocs.Delete(assoc.AssociationID)
		}
	}

	for _, assoc := range slices.Clone(b.apiAssociations.All()) {
		if assoc.APIID != apiID {
			continue
		}

		b.apiAssociations.Delete(assoc.DomainName)

		if dn, ok := b.domainNames.Get(assoc.DomainName); ok {
			dn.APIID = ""
		}
	}
}

// GetGraphqlAPIEnvironmentVariables returns environment variables for a GraphQL API.
func (b *InMemoryBackend) GetGraphqlAPIEnvironmentVariables(apiID string) (map[string]string, error) {
	b.mu.RLock("GetGraphqlApiEnvironmentVariables")
	defer b.mu.RUnlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return nil, fmt.Errorf("%w: api %s not found", ErrNotFound, apiID)
	}

	if api.EnvironmentVariables == nil {
		return map[string]string{}, nil
	}

	out := maps.Clone(api.EnvironmentVariables)

	return out, nil
}

// PutGraphqlAPIEnvironmentVariables replaces the environment variables for a GraphQL API.
func (b *InMemoryBackend) PutGraphqlAPIEnvironmentVariables(
	apiID string,
	envVars map[string]string,
) (map[string]string, error) {
	b.mu.Lock("PutGraphqlApiEnvironmentVariables")
	defer b.mu.Unlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return nil, fmt.Errorf("%w: api %s not found", ErrNotFound, apiID)
	}

	if len(envVars) > maxEnvironmentVariables {
		return nil, fmt.Errorf(
			"%w: environment variables cannot exceed %d entries",
			ErrValidation,
			maxEnvironmentVariables,
		)
	}

	api.EnvironmentVariables = maps.Clone(envVars)

	return maps.Clone(api.EnvironmentVariables), nil
}
