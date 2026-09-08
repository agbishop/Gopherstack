package cognitoidentity

import (
	"context"
	"fmt"
)

// SetIdentityPoolRoles configures IAM roles for an identity pool.
// Only the roles that are present (non-empty) in the provided map are updated;
// existing roles for omitted keys are preserved.
func (b *InMemoryBackend) SetIdentityPoolRoles(
	ctx context.Context,
	poolID, authenticatedARN, unauthenticatedARN string,
	roleMappings map[string]RoleMapping,
) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("SetIdentityPoolRoles")
	defer b.mu.Unlock()

	if _, ok := b.poolGet(region, poolID); !ok {
		return fmt.Errorf("%w: identity pool %q not found", ErrIdentityPoolNotFound, poolID)
	}

	existing, ok := b.rolesGet(region, poolID)
	if !ok {
		existing = &IdentityRoles{region: region, poolID: poolID}
		b.rolesPut(existing)
	}

	// Only update the roles that the caller provided (non-empty value = explicitly set).
	if authenticatedARN != "" {
		existing.AuthenticatedRoleARN = authenticatedARN
	}

	if unauthenticatedARN != "" {
		existing.UnauthenticatedRoleARN = unauthenticatedARN
	}

	if roleMappings != nil {
		existing.RoleMappings = cloneRoleMappings(roleMappings)
	}

	return nil
}

// GetIdentityPoolRoles returns the IAM roles configured for an identity pool.
func (b *InMemoryBackend) GetIdentityPoolRoles(
	ctx context.Context,
	poolID string,
) (*IdentityRoles, error) {
	if poolID == "" {
		return nil, fmt.Errorf("%w: IdentityPoolId is required", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("GetIdentityPoolRoles")
	defer b.mu.RUnlock()

	if _, ok := b.poolGet(region, poolID); !ok {
		return nil, fmt.Errorf("%w: identity pool %q not found", ErrIdentityPoolNotFound, poolID)
	}

	roles, ok := b.rolesGet(region, poolID)
	if !ok {
		return &IdentityRoles{}, nil
	}

	cp := *roles
	cp.RoleMappings = cloneRoleMappings(roles.RoleMappings)

	return &cp, nil
}

// GetPrincipalTagAttributeMap returns the principal tag attribute map for a pool and provider.
func (b *InMemoryBackend) GetPrincipalTagAttributeMap(
	ctx context.Context,
	poolID, providerName string,
) (*PrincipalTagMapping, error) {
	if providerName == "" {
		return nil, fmt.Errorf("%w: IdentityProviderName is required", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("GetPrincipalTagAttributeMap")
	defer b.mu.RUnlock()

	if _, ok := b.poolGet(region, poolID); !ok {
		return nil, fmt.Errorf("%w: identity pool %q not found", ErrIdentityPoolNotFound, poolID)
	}

	if m, ok := b.principalTagGet(region, poolID, providerName); ok {
		return clonePrincipalTagMapping(m), nil
	}

	return &PrincipalTagMapping{UseDefaults: true}, nil
}

// SetPrincipalTagAttributeMap configures principal tag attribute mappings for a pool and provider.
func (b *InMemoryBackend) SetPrincipalTagAttributeMap(
	ctx context.Context,
	poolID string,
	providerName string,
	useDefaults bool,
	principalTags map[string]string,
) (*PrincipalTagMapping, error) {
	if providerName == "" {
		return nil, fmt.Errorf("%w: IdentityProviderName is required", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("SetPrincipalTagAttributeMap")
	defer b.mu.Unlock()

	if _, ok := b.poolGet(region, poolID); !ok {
		return nil, fmt.Errorf("%w: identity pool %q not found", ErrIdentityPoolNotFound, poolID)
	}

	mapping := &PrincipalTagMapping{
		UseDefaults:   useDefaults,
		PrincipalTags: cloneStringMap(principalTags),
		region:        region,
		poolID:        poolID,
		providerName:  providerName,
	}

	b.principalTagPut(mapping)

	return clonePrincipalTagMapping(mapping), nil
}

// clonePrincipalTagMapping returns a deep copy of a PrincipalTagMapping.
func clonePrincipalTagMapping(m *PrincipalTagMapping) *PrincipalTagMapping {
	return &PrincipalTagMapping{
		UseDefaults:   m.UseDefaults,
		PrincipalTags: cloneStringMap(m.PrincipalTags),
		region:        m.region,
		poolID:        m.poolID,
		providerName:  m.providerName,
	}
}

// cloneRoleMappings returns a deep copy of a RoleMapping map.
func cloneRoleMappings(m map[string]RoleMapping) map[string]RoleMapping {
	if m == nil {
		return nil
	}

	out := make(map[string]RoleMapping, len(m))

	for k, v := range m {
		rm := RoleMapping{
			Type:                    v.Type,
			AmbiguousRoleResolution: v.AmbiguousRoleResolution,
		}

		if v.RulesConfiguration != nil {
			rules := make([]MappingRule, len(v.RulesConfiguration.Rules))
			copy(rules, v.RulesConfiguration.Rules)
			rm.RulesConfiguration = &RulesConfiguration{Rules: rules}
		}

		out[k] = rm
	}

	return out
}
