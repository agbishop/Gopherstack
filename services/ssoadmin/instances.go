package ssoadmin

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CreateInstance creates a new SSO instance.
func (b *InMemoryBackend) CreateInstance(
	name, ownerAccountID, identityStoreID string,
	tags map[string]string,
) (*Instance, error) {
	if len(tags) > 0 {
		if err := validateTags(tags); err != nil {
			return nil, err
		}
	}

	b.mu.Lock("CreateInstance")
	defer b.mu.Unlock()

	id := uuid.NewString()[:uuidShortLen]
	instanceArn := "arn:aws:sso:::instance/ssoins-" + id

	if ownerAccountID == "" {
		ownerAccountID = b.accountID
	}
	if identityStoreID == "" {
		prefix := ownerAccountID
		if len(prefix) > identityStoreIDPrefixLen {
			prefix = prefix[:identityStoreIDPrefixLen]
		}
		raw := "d-" + prefix + "0000000000"
		if len(raw) > identityStoreIDMaxLen {
			raw = raw[:identityStoreIDMaxLen]
		}
		identityStoreID = raw
	}

	inst := &Instance{
		InstanceArn:     instanceArn,
		Name:            name,
		OwnerAccountID:  ownerAccountID,
		IdentityStoreID: identityStoreID,
		Status:          instanceStatusCreateInProgress,
		CreatedDate:     time.Now().UTC(),
		Tags:            make(map[string]string),
	}
	maps.Copy(inst.Tags, tags)
	b.instances.Put(inst)

	cp := *inst
	cp.Tags = make(map[string]string, len(inst.Tags))
	maps.Copy(cp.Tags, inst.Tags)

	return &cp, nil
}

// ListInstances returns all SSO instances, pruning DELETE_IN_PROGRESS entries.
func (b *InMemoryBackend) ListInstances() []*Instance {
	b.mu.Lock("ListInstances")
	defer b.mu.Unlock()

	list := make([]*Instance, 0, b.instances.Len())
	for _, inst := range b.instances.All() {
		if inst.Status == instanceStatusDeleteInProgress {
			// Prune resources then remove instance entry.
			b.cascadeDeleteInstance(inst.InstanceArn)
			b.instances.Delete(inst.InstanceArn)

			continue
		}
		if inst.Status == instanceStatusCreateInProgress {
			inst.Status = instanceStatusActive
		}
		cp := *inst
		list = append(list, &cp)
	}

	slices.SortFunc(list, func(a, b *Instance) int {
		return strings.Compare(a.InstanceArn, b.InstanceArn)
	})

	return list
}

// DescribeInstance returns a specific SSO instance.
// Lazily transitions CREATE_IN_PROGRESS → ACTIVE on first read.
func (b *InMemoryBackend) DescribeInstance(instanceArn string) (*Instance, error) {
	b.mu.Lock("DescribeInstance")
	defer b.mu.Unlock()

	inst, ok := b.instances.Get(instanceArn)
	if !ok {
		return nil, ErrInstanceNotFound
	}
	if inst.Status == instanceStatusCreateInProgress {
		inst.Status = instanceStatusActive
	}

	cp := *inst
	cp.Tags = make(map[string]string, len(inst.Tags))
	maps.Copy(cp.Tags, inst.Tags)

	return &cp, nil
}

// DeleteInstance transitions an instance to DELETE_IN_PROGRESS and cascades deletion of dependent resources.
// The instance entry is retained briefly with DELETE_IN_PROGRESS status and pruned on next ListInstances.
func (b *InMemoryBackend) DeleteInstance(instanceArn string) error {
	b.mu.Lock("DeleteInstance")
	defer b.mu.Unlock()

	inst, ok := b.instances.Get(instanceArn)
	if !ok {
		return ErrInstanceNotFound
	}
	inst.Status = instanceStatusDeleteInProgress
	b.cascadeDeleteInstance(instanceArn)

	return nil
}

// cascadeDeleteInstance removes all resources belonging to instanceArn. Must be called with mu held.
// Index lookups are cloned before deleting in the loop since Table.Delete mutates the
// same index group slice the lookup returned.
func (b *InMemoryBackend) cascadeDeleteInstance(instanceArn string) {
	for _, ps := range slices.Clone(b.permissionSetsByInstance.Get(instanceArn)) {
		psArn := ps.PermissionSetArn
		key := assignmentKey(instanceArn, psArn)
		delete(b.assignments, key)
		delete(b.customerManagedPolicies, psArn)
		b.permissionBoundaries.Delete(psArn)
		b.permissionSets.Delete(psArn)
		// A permission set can be force-deleted here with live assignments
		// (unlike DeletePermissionSet, cascadeDeleteInstance has no
		// "no assignments" precondition), so provisionedAt/assignmentCreationIDs
		// rows keyed off it would otherwise never be reclaimed -- unbounded
		// growth across repeated instance create/delete cycles.
		purgeByPrefix(b.provisionedAt, key+"|")
		purgeByPrefix(b.assignmentCreationIDs, key+"|")
	}
	b.instanceACAs.Delete(instanceArn)
	delete(b.instanceRegions, instanceArn)
	for _, app := range slices.Clone(b.applicationsByInstance.Get(instanceArn)) {
		appArn := app.ApplicationArn
		delete(b.applicationAssignments, appArn)
		delete(b.applicationScopes, appArn)
		delete(b.applicationAuthMethods, appArn)
		delete(b.applicationGrants, appArn)
		delete(b.applicationAssignConfig, appArn)
		delete(b.applicationSessions, appArn)
		b.applications.Delete(appArn)
	}
	for _, tti := range slices.Clone(b.trustedTokenIssuersByInstance.Get(instanceArn)) {
		b.trustedTokenIssuers.Delete(tti.TrustedTokenIssuerArn)
	}
}

// purgeByPrefix deletes every entry of m whose key starts with prefix.
func purgeByPrefix[V any](m map[string]V, prefix string) {
	for k := range m {
		if strings.HasPrefix(k, prefix) {
			delete(m, k)
		}
	}
}

// instanceARNToID extracts the instance ID segment from an instance ARN.
// ARN format: arn:aws:sso:::instance/ssoins-<id>.
func instanceARNToID(instanceArn string) string {
	parts := strings.Split(instanceArn, "/")
	if len(parts) >= 2 { //nolint:mnd // minimum 2 parts needed for valid ARN split
		return parts[len(parts)-1]
	}

	return instanceArn
}

// AddInstanceInternal adds a pre-built Instance directly to the backend for test seeding.
// Must NOT be called concurrently with other backend methods.
func (b *InMemoryBackend) AddInstanceInternal(name string) *Instance {
	b.mu.Lock("AddInstanceInternal")
	defer b.mu.Unlock()

	id := uuid.NewString()[:uuidShortLen]
	arn := "arn:aws:sso:::instance/ssoins-" + id
	inst := &Instance{
		InstanceArn:     arn,
		Name:            name,
		OwnerAccountID:  b.accountID,
		IdentityStoreID: "d-" + b.accountID[:min(len(b.accountID), identityStoreIDPrefixLen)],
		Status:          instanceStatusActive,
		CreatedDate:     time.Now().UTC(),
		Tags:            make(map[string]string),
	}
	b.instances.Put(inst)
	cp := *inst
	cp.Tags = make(map[string]string)

	return &cp
}

// UpdateInstance updates the name and/or PermissionSetsEnabled of an SSO instance.
// permissionSetsEnabled is nil when the caller omitted the field; whatever value
// is supplied is stored verbatim (AWS documents "only true accepted" as a
// business rule, not a wire-level constraint -- we don't reject other values).
func (b *InMemoryBackend) UpdateInstance(instanceArn, name string, permissionSetsEnabled *bool) error {
	b.mu.Lock("UpdateInstance")
	defer b.mu.Unlock()

	inst, ok := b.instances.Get(instanceArn)
	if !ok {
		return ErrInstanceNotFound
	}
	if name != "" {
		inst.Name = name
	}
	if permissionSetsEnabled != nil {
		v := *permissionSetsEnabled
		inst.PermissionSetsEnabled = &v
	}

	return nil
}
