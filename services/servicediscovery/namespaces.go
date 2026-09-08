package servicediscovery

import (
	"fmt"
	"sort"
	"time"
)

// createNamespace is the internal helper used by all three create-namespace operations.
func (b *InMemoryBackend) createNamespace(
	name, nsType, description, vpc string,
	soaTTL int64,
	tags map[string]string,
) (string, error) {
	b.mu.Lock("createNamespace")
	defer b.mu.Unlock()

	if existing := b.namespacesByName.Get(name); len(existing) > 0 {
		return "", fmt.Errorf("%w: namespace %s already exists", ErrNamespaceAlreadyExists, name)
	}

	id := b.nextNsID()

	var props *NamespaceProperties
	switch nsType {
	case namespaceTypeDNSPrivate, namespaceTypeDNSPublic:
		ttl := soaTTL
		if ttl == 0 {
			ttl = defaultSOATTL
		}

		props = &NamespaceProperties{
			DNSProperties: &DNSProperties{
				HostedZoneID: b.hostedZoneID(id, name, nsType == namespaceTypeDNSPrivate, vpc),
				SOA:          &SOA{TTL: ttl},
			},
		}
	case namespaceTypeHTTP:
		props = &NamespaceProperties{
			HTTPProperties: &HTTPProperties{HTTPName: name},
		}
	}

	now := time.Now()
	ns := &Namespace{
		ID:          id,
		ARN:         b.namespaceARN(id),
		Name:        name,
		Type:        nsType,
		Description: description,
		VPC:         vpc,
		Properties:  props,
		Tags:        copyTags(tags),
		CreatedAt:   now,
	}
	b.namespaces.Put(ns)

	opID := b.nextOpID()
	b.operations.Put(&Operation{
		ID:         opID,
		Type:       operationTypeCreateNamespace,
		Status:     operationStatusSuccess,
		Targets:    map[string]string{typeNamespace: id},
		CreateDate: now,
		UpdateDate: now,
	})

	return opID, nil
}

// CreateHTTPNamespace creates an HTTP namespace.
func (b *InMemoryBackend) CreateHTTPNamespace(name, description string, tags map[string]string) (string, error) {
	return b.createNamespace(name, namespaceTypeHTTP, description, "", 0, tags)
}

// CreatePrivateDNSNamespace creates a private DNS namespace.
// soaTTL defaults to 15 when zero.
func (b *InMemoryBackend) CreatePrivateDNSNamespace(
	name, description, vpc string,
	soaTTL int64,
	tags map[string]string,
) (string, error) {
	return b.createNamespace(name, namespaceTypeDNSPrivate, description, vpc, soaTTL, tags)
}

// CreatePublicDNSNamespace creates a public DNS namespace.
// soaTTL defaults to 15 when zero.
func (b *InMemoryBackend) CreatePublicDNSNamespace(
	name, description string,
	soaTTL int64,
	tags map[string]string,
) (string, error) {
	return b.createNamespace(name, namespaceTypeDNSPublic, description, "", soaTTL, tags)
}

// DeleteNamespace deletes a namespace by ID.
// Returns ResourceInUse if the namespace still has services.
func (b *InMemoryBackend) DeleteNamespace(id string) (string, error) {
	b.mu.Lock("DeleteNamespace")
	defer b.mu.Unlock()

	if !b.namespaces.Has(id) {
		return "", fmt.Errorf("%w: namespace %s not found", ErrNamespaceNotFound, id)
	}

	for _, svc := range b.services.All() {
		if svc.NamespaceID == id {
			return "", fmt.Errorf("%w: namespace %s has services; delete them first", ErrResourceInUse, id)
		}
	}

	b.namespaces.Delete(id)

	now := time.Now()
	opID := b.nextOpID()
	b.operations.Put(&Operation{
		ID:         opID,
		Type:       operationTypeDeleteNamespace,
		Status:     operationStatusSuccess,
		Targets:    map[string]string{typeNamespace: id},
		CreateDate: now,
		UpdateDate: now,
	})

	return opID, nil
}

// GetNamespace returns a namespace by ID.
func (b *InMemoryBackend) GetNamespace(id string) (*Namespace, error) {
	b.mu.RLock("GetNamespace")
	defer b.mu.RUnlock()

	ns, ok := b.namespaces.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: namespace %s not found", ErrNamespaceNotFound, id)
	}

	cp := copyNamespace(ns)
	cp.ServiceCount = b.countServicesInNamespace(id)

	return cp, nil
}

// namespaceHTTPName returns ns's Properties.HttpProperties.HttpName, or "" if
// unset (e.g. for DNS namespaces, which never have HttpProperties). Used to
// evaluate the ListNamespaces HTTP_NAME filter.
func namespaceHTTPName(ns *Namespace) string {
	if ns.Properties == nil || ns.Properties.HTTPProperties == nil {
		return ""
	}

	return ns.Properties.HTTPProperties.HTTPName
}

// countServicesInNamespace counts services belonging to a namespace. Caller must hold at least a read lock.
func (b *InMemoryBackend) countServicesInNamespace(namespaceID string) int {
	count := 0
	for _, svc := range b.services.All() {
		if svc.NamespaceID == namespaceID {
			count++
		}
	}

	return count
}

// ListNamespaces returns all namespaces sorted by name, optionally filtered.
func (b *InMemoryBackend) ListNamespaces(filter ListNamespacesFilter) []Namespace {
	b.mu.RLock("ListNamespaces")
	defer b.mu.RUnlock()

	all := b.namespaces.All()
	result := make([]Namespace, 0, len(all))

	for _, ns := range all {
		if !filter.Type.matches(ns.Type) {
			continue
		}

		if !filter.Name.matches(ns.Name) {
			continue
		}

		if !filter.HTTPName.matches(namespaceHTTPName(ns)) {
			continue
		}

		if !resourceOwnerMatches(filter.ResourceOwner) {
			continue
		}

		cp := copyNamespace(ns)
		cp.ServiceCount = b.countServicesInNamespace(ns.ID)
		result = append(result, *cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// UpdateHTTPNamespace updates the description of an HTTP namespace.
func (b *InMemoryBackend) UpdateHTTPNamespace(id, description string) (string, error) {
	return b.updateNamespace(id, namespaceTypeHTTP, description, 0, false)
}

// UpdatePrivateDNSNamespace updates the description of a private DNS
// namespace, and its SOA TTL when hasSOATTL is true (types.
// PrivateDnsNamespaceChange.Properties.DnsProperties.SOA.TTL,
// api_op_UpdatePrivateDnsNamespace.go).
func (b *InMemoryBackend) UpdatePrivateDNSNamespace(
	id, description string,
	soaTTL int64,
	hasSOATTL bool,
) (string, error) {
	return b.updateNamespace(id, namespaceTypeDNSPrivate, description, soaTTL, hasSOATTL)
}

// UpdatePublicDNSNamespace updates the description of a public DNS
// namespace, and its SOA TTL when hasSOATTL is true (types.
// PublicDnsNamespaceChange.Properties.DnsProperties.SOA.TTL,
// api_op_UpdatePublicDnsNamespace.go).
func (b *InMemoryBackend) UpdatePublicDNSNamespace(
	id, description string,
	soaTTL int64,
	hasSOATTL bool,
) (string, error) {
	return b.updateNamespace(id, namespaceTypeDNSPublic, description, soaTTL, hasSOATTL)
}

// updateNamespace is the internal helper for namespace update operations.
func (b *InMemoryBackend) updateNamespace(
	id, nsType, description string,
	soaTTL int64,
	hasSOATTL bool,
) (string, error) {
	b.mu.Lock("updateNamespace")
	defer b.mu.Unlock()

	ns, ok := b.namespaces.Get(id)
	if !ok {
		return "", fmt.Errorf("%w: namespace %s not found", ErrNamespaceNotFound, id)
	}

	if ns.Type != nsType {
		return "", fmt.Errorf("%w: namespace %s is not of type %s", ErrInvalidInput, id, nsType)
	}

	ns.Description = description

	if hasSOATTL && ns.Properties != nil && ns.Properties.DNSProperties != nil &&
		ns.Properties.DNSProperties.SOA != nil {
		ns.Properties.DNSProperties.SOA.TTL = soaTTL
	}

	now := time.Now()
	opID := b.nextOpID()
	b.operations.Put(&Operation{
		ID:         opID,
		Type:       operationTypeUpdateNamespace,
		Status:     operationStatusSuccess,
		Targets:    map[string]string{typeNamespace: id},
		CreateDate: now,
		UpdateDate: now,
	})

	return opID, nil
}
