package s3control

import (
	"fmt"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

// AssociateAccessGrantsIdentityCenter associates an IAM Identity Center instance with an S3 Access Grants instance.
func (b *InMemoryBackend) AssociateAccessGrantsIdentityCenter(accountID, identityCenterArn string) {
	b.mu.Lock("AssociateAccessGrantsIdentityCenter")
	defer b.mu.Unlock()

	inst, ok := b.accessGrantsInstances.Get(accountID)
	if !ok {
		inst = &AccessGrantsInstance{
			AccountID:               accountID,
			AccessGrantsInstanceID:  defaultAccessGrantsInstanceID,
			AccessGrantsInstanceArn: fmt.Sprintf(arnFmtAccessGrantsInstance, b.region, accountID),
			CreatedAt:               nowRFC3339(),
		}
		b.accessGrantsInstances.Put(inst)
	}

	inst.IdentityCenterArn = identityCenterArn
	inst.IdentityCenterInstanceArn = identityCenterArn
}

// CreateAccessGrantsInstance creates an S3 Access Grants instance for an account.
func (b *InMemoryBackend) CreateAccessGrantsInstance(accountID, identityCenterArn string) *AccessGrantsInstance {
	b.mu.Lock("CreateAccessGrantsInstance")
	defer b.mu.Unlock()

	inst := &AccessGrantsInstance{
		AccountID:                 accountID,
		AccessGrantsInstanceID:    defaultAccessGrantsInstanceID,
		AccessGrantsInstanceArn:   fmt.Sprintf(arnFmtAccessGrantsInstance, b.region, accountID),
		IdentityCenterArn:         identityCenterArn,
		IdentityCenterInstanceArn: identityCenterArn,
		CreatedAt:                 nowRFC3339(),
	}
	b.accessGrantsInstances.Put(inst)

	cp := *inst

	return &cp
}

// CreateAccessGrant creates an access grant for an account.
// Returns ErrValidation if permission is empty.
func (b *InMemoryBackend) CreateAccessGrant(
	accountID, locationID, granteeType, granteeIdentifier, permission, applicationArn string,
) (*AccessGrant, error) {
	if permission == "" {
		return nil, fmt.Errorf("permission is required: %w", ErrValidation)
	}

	b.mu.Lock("CreateAccessGrant")
	defer b.mu.Unlock()

	id := b.newID("grant")
	arn := fmt.Sprintf(arnFmtAccessGrant, b.region, accountID, id)

	grant := &AccessGrant{
		AccountID:              accountID,
		AccessGrantID:          id,
		AccessGrantArn:         arn,
		AccessGrantsLocationID: locationID,
		GrantScope:             fmt.Sprintf("s3://%s/*", locationID),
		Permission:             permission,
		GranteeType:            granteeType,
		GranteeIdentifier:      granteeIdentifier,
		ApplicationArn:         applicationArn,
		CreatedAt:              nowRFC3339(),
	}
	b.accessGrants.Put(grant)

	cp := *grant

	return &cp, nil
}

// CreateAccessGrantsLocation creates an Access Grants location.
func (b *InMemoryBackend) CreateAccessGrantsLocation(
	accountID, locationScope, iamRoleArn string,
) *AccessGrantsLocation {
	b.mu.Lock("CreateAccessGrantsLocation")
	defer b.mu.Unlock()

	id := b.newID("location")
	arn := fmt.Sprintf(arnFmtAccessGrantsLocation, b.region, accountID, id)

	loc := &AccessGrantsLocation{
		AccountID:               accountID,
		AccessGrantsLocationID:  id,
		AccessGrantsLocationArn: arn,
		LocationScope:           locationScope,
		IAMRoleArn:              iamRoleArn,
		CreatedAt:               nowRFC3339(),
	}
	b.accessGrantsLocations.Put(loc)

	cp := *loc

	return &cp
}

// ---- Access Grants Instance ----

// ListAccessGrantsInstances returns all Access Grants instances for the account.
func (b *InMemoryBackend) ListAccessGrantsInstances(accountID string) []*AccessGrantsInstance {
	b.mu.RLock("ListAccessGrantsInstances")
	defer b.mu.RUnlock()

	inst, ok := b.accessGrantsInstances.Get(accountID)
	if !ok {
		return nil
	}

	return []*AccessGrantsInstance{inst}
}

func (b *InMemoryBackend) GetAccessGrantsInstance(accountID string) (*AccessGrantsInstance, error) {
	b.mu.RLock("GetAccessGrantsInstance")
	defer b.mu.RUnlock()

	inst, ok := b.accessGrantsInstances.Get(accountID)
	if !ok {
		return nil, awserr.New("AccessGrantsInstanceNotExistsError", awserr.ErrNotFound)
	}

	return inst, nil
}

// errAccessGrantsInstanceNotEmpty is returned when DeleteAccessGrantsInstance
// is called while the instance still has grants or locations attached, or a
// live IAM Identity Center association — both required-clear-first per
// DeleteAccessGrantsInstance's doc comment (api_op_DeleteAccessGrantsInstance.go,
// aws-sdk-go-v2/service/s3control@v1.73.0). No typed exception exists for
// either conflict (types/errors.go's full list has none), so this reuses
// the generic "BadRequestException" sentinel (ErrValidation) already used
// elsewhere for S3 Access Grants validation failures.
var errAccessGrantsInstanceNotEmpty = ErrValidation

// DeleteAccessGrantsInstance removes the Access Grants instance and
// cascade-cleans its resource policy and generic resource tags. It does NOT
// cascade-delete AccessGrants or AccessGrantsLocations — those, plus any
// Identity Center association, must be cleared first (see
// errAccessGrantsInstanceNotEmpty); any of the three still present rejects
// the delete instead of silently succeeding.
func (b *InMemoryBackend) DeleteAccessGrantsInstance(accountID string) error {
	b.mu.Lock("DeleteAccessGrantsInstance")
	defer b.mu.Unlock()

	// arn is "" if no instance exists; deleting resourceTags[""] is a
	// harmless no-op, so no separate not-found branch is needed here --
	// DeleteAccessGrantsInstance is idempotent in the real API too.
	var arn string
	if inst, ok := b.accessGrantsInstances.Get(accountID); ok {
		arn = inst.AccessGrantsInstanceArn
		if inst.IdentityCenterArn != "" {
			return errAccessGrantsInstanceNotEmpty
		}
	}

	for _, g := range b.accessGrants.All() {
		if g.AccountID == accountID {
			return errAccessGrantsInstanceNotEmpty
		}
	}
	for _, loc := range b.accessGrantsLocations.All() {
		if loc.AccountID == accountID {
			return errAccessGrantsInstanceNotEmpty
		}
	}

	b.accessGrantsInstances.Delete(accountID)
	delete(b.accessGrantsInstancePolicies, accountID)
	delete(b.resourceTags, arn)

	return nil
}

// GetAccessGrantsInstanceResourcePolicy returns the resource policy for an AGI.
func (b *InMemoryBackend) GetAccessGrantsInstanceResourcePolicy(accountID string) (string, error) {
	b.mu.RLock("GetAccessGrantsInstanceResourcePolicy")
	defer b.mu.RUnlock()

	policy := b.accessGrantsInstancePolicies[accountID]

	return policy, nil
}

// PutAccessGrantsInstanceResourcePolicy sets the resource policy for an AGI.
func (b *InMemoryBackend) PutAccessGrantsInstanceResourcePolicy(accountID, policy string) {
	b.mu.Lock("PutAccessGrantsInstanceResourcePolicy")
	defer b.mu.Unlock()

	b.accessGrantsInstancePolicies[accountID] = policy
}

// DeleteAccessGrantsInstanceResourcePolicy removes the resource policy.
func (b *InMemoryBackend) DeleteAccessGrantsInstanceResourcePolicy(accountID string) {
	b.mu.Lock("DeleteAccessGrantsInstanceResourcePolicy")
	defer b.mu.Unlock()

	delete(b.accessGrantsInstancePolicies, accountID)
}

// DissociateAccessGrantsIdentityCenter removes the identity center association.
func (b *InMemoryBackend) DissociateAccessGrantsIdentityCenter(accountID string) {
	b.mu.Lock("DissociateAccessGrantsIdentityCenter")
	defer b.mu.Unlock()

	if inst, ok := b.accessGrantsInstances.Get(accountID); ok {
		inst.IdentityCenterArn = ""
	}
}

// GetAccessGrantsInstanceForPrefix returns the AGI for a given S3 prefix.
func (b *InMemoryBackend) GetAccessGrantsInstanceForPrefix(
	accountID, prefix string,
) (*AccessGrantsInstance, error) {
	b.mu.RLock("GetAccessGrantsInstanceForPrefix")
	defer b.mu.RUnlock()

	inst, ok := b.accessGrantsInstances.Get(accountID)
	if !ok {
		return nil, awserr.New("AccessGrantsInstanceNotExistsError", awserr.ErrNotFound)
	}
	_ = prefix

	return inst, nil
}

// ---- Access Grants CRUD ----

// GetAccessGrant returns an access grant by ID.
func (b *InMemoryBackend) GetAccessGrant(accountID, grantID string) (*AccessGrant, error) {
	b.mu.RLock("GetAccessGrant")
	defer b.mu.RUnlock()

	key := accountID + ":" + grantID
	grant, ok := b.accessGrants.Get(key)
	if !ok {
		return nil, awserr.New("NoSuchAccessGrant", awserr.ErrNotFound)
	}

	return grant, nil
}

// DeleteAccessGrant removes an access grant and cascade-cleans its generic
// resource tags.
func (b *InMemoryBackend) DeleteAccessGrant(accountID, grantID string) error {
	b.mu.Lock("DeleteAccessGrant")
	defer b.mu.Unlock()

	key := accountID + ":" + grantID

	grant, ok := b.accessGrants.Get(key)
	if !ok {
		return awserr.New("NoSuchAccessGrant", awserr.ErrNotFound)
	}

	arn := grant.AccessGrantArn

	b.accessGrants.Delete(key)
	delete(b.resourceTags, arn)

	return nil
}

// AccessGrantsFilter holds ListAccessGrants/ListCallerAccessGrants's query
// filters (s3control@v1.73.4 api_op_ListAccessGrants.go /
// api_op_ListCallerAccessGrants.go).
type AccessGrantsFilter struct {
	GrantScope        string
	ApplicationArn    string
	GranteeIdentifier string
	GranteeType       string
	Permission        string
}

func matchesAccessGrantFilter(g *AccessGrant, filter AccessGrantsFilter) bool {
	if filter.GrantScope != "" && g.GrantScope != filter.GrantScope {
		return false
	}
	if filter.ApplicationArn != "" && g.ApplicationArn != filter.ApplicationArn {
		return false
	}
	if filter.GranteeIdentifier != "" && g.GranteeIdentifier != filter.GranteeIdentifier {
		return false
	}
	if filter.GranteeType != "" && g.GranteeType != filter.GranteeType {
		return false
	}
	if filter.Permission != "" && g.Permission != filter.Permission {
		return false
	}

	return true
}

// ListAccessGrants returns all access grants for an account matching filter.
func (b *InMemoryBackend) ListAccessGrants(accountID string, filter AccessGrantsFilter) []*AccessGrant {
	b.mu.RLock("ListAccessGrants")
	defer b.mu.RUnlock()

	var out []*AccessGrant
	for _, g := range b.accessGrants.All() {
		if g.AccountID != accountID {
			continue
		}
		if !matchesAccessGrantFilter(g, filter) {
			continue
		}
		cp := *g
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AccessGrantID < out[j].AccessGrantID })

	return out
}

// ListCallerAccessGrants returns access grants visible to the caller,
// optionally filtered by grantScope.
func (b *InMemoryBackend) ListCallerAccessGrants(accountID, grantScope string) []*AccessGrant {
	return b.ListAccessGrants(accountID, AccessGrantsFilter{GrantScope: grantScope})
}

// GetAccessGrantsLocation returns an access grants location by ID.
func (b *InMemoryBackend) GetAccessGrantsLocation(
	accountID, locationID string,
) (*AccessGrantsLocation, error) {
	b.mu.RLock("GetAccessGrantsLocation")
	defer b.mu.RUnlock()

	key := accountID + ":" + locationID
	loc, ok := b.accessGrantsLocations.Get(key)
	if !ok {
		return nil, awserr.New("NoSuchAccessGrantsLocation", awserr.ErrNotFound)
	}

	return loc, nil
}

// errAccessGrantsLocationNotEmpty is returned when DeleteAccessGrantsLocation
// is called while grants still reference the location -- per
// DeleteAccessGrantsLocation's doc comment (api_op_DeleteAccessGrantsLocation.go,
// aws-sdk-go-v2/service/s3control@v1.73.4): "You can only delete a location
// registration from an S3 Access Grants instance if there are no grants
// associated with this location." No typed exception exists for this
// conflict (types/errors.go's full list has none), so this reuses the
// generic "BadRequestException" sentinel (ErrValidation), matching
// errAccessGrantsInstanceNotEmpty's precedent for the sibling instance
// precondition.
var errAccessGrantsLocationNotEmpty = ErrValidation

// DeleteAccessGrantsLocation removes an access grants location and
// cascade-cleans its generic resource tags. Rejects the delete instead of
// silently succeeding while any access grant still references the location
// (see errAccessGrantsLocationNotEmpty).
func (b *InMemoryBackend) DeleteAccessGrantsLocation(accountID, locationID string) error {
	b.mu.Lock("DeleteAccessGrantsLocation")
	defer b.mu.Unlock()

	key := accountID + ":" + locationID

	loc, ok := b.accessGrantsLocations.Get(key)
	if !ok {
		return awserr.New("NoSuchAccessGrantsLocation", awserr.ErrNotFound)
	}

	for _, g := range b.accessGrants.All() {
		if g.AccountID == accountID && g.AccessGrantsLocationID == locationID {
			return errAccessGrantsLocationNotEmpty
		}
	}

	arn := loc.AccessGrantsLocationArn

	b.accessGrantsLocations.Delete(key)
	delete(b.resourceTags, arn)

	return nil
}

// UpdateAccessGrantsLocation updates the IAM role ARN for a location.
func (b *InMemoryBackend) UpdateAccessGrantsLocation(
	accountID, locationID, iamRoleArn string,
) (*AccessGrantsLocation, error) {
	b.mu.Lock("UpdateAccessGrantsLocation")
	defer b.mu.Unlock()

	key := accountID + ":" + locationID
	loc, ok := b.accessGrantsLocations.Get(key)
	if !ok {
		return nil, awserr.New("NoSuchAccessGrantsLocation", awserr.ErrNotFound)
	}
	loc.IAMRoleArn = iamRoleArn

	return loc, nil
}

// ListAccessGrantsLocations returns all locations for an account.
func (b *InMemoryBackend) ListAccessGrantsLocations(accountID string) []*AccessGrantsLocation {
	b.mu.RLock("ListAccessGrantsLocations")
	defer b.mu.RUnlock()

	var out []*AccessGrantsLocation
	for _, loc := range b.accessGrantsLocations.All() {
		if loc.AccountID == accountID {
			cp := *loc
			out = append(out, &cp)
		}
	}
	sort.Slice(
		out,
		func(i, j int) bool { return out[i].AccessGrantsLocationID < out[j].AccessGrantsLocationID },
	)

	return out
}

// GetDataAccess returns a presigned URL for accessing data via access grants.
func (b *InMemoryBackend) GetDataAccess(accountID, target, permission string) (string, error) {
	b.mu.RLock("GetDataAccess")
	defer b.mu.RUnlock()

	// Verify the instance exists
	if !b.accessGrantsInstances.Has(accountID) {
		return "", awserr.New("AccessGrantsInstanceNotExistsError", awserr.ErrNotFound)
	}
	_ = target
	_ = permission

	return "https://s3.amazonaws.com/presigned-mock-url", nil
}

// ---- Seed helpers for testing ----

// AddAccessGrantsInstanceInternal creates an access grants instance directly, for seeding test data.
func (b *InMemoryBackend) AddAccessGrantsInstanceInternal(accountID, identityCenterArn string) *AccessGrantsInstance {
	return b.CreateAccessGrantsInstance(accountID, identityCenterArn)
}

// AddAccessGrantInternal creates an access grant directly, for seeding test data.
func (b *InMemoryBackend) AddAccessGrantInternal(
	accountID, locationID, granteeType, granteeIdentifier, permission string,
) *AccessGrant {
	grant, _ := b.CreateAccessGrant(accountID, locationID, granteeType, granteeIdentifier, permission, "")

	return grant
}
