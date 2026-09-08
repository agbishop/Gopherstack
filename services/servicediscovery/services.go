package servicediscovery

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"
)

// CreateService creates a new Cloud Map service.
func (b *InMemoryBackend) CreateService(
	name, namespaceID, description, svcType string,
	dnsConfig *DNSConfig,
	hcc *HealthCheckConfig,
	hccc *HealthCheckCustomConfig,
	tags map[string]string,
) (*Service, error) {
	b.mu.Lock("CreateService")
	defer b.mu.Unlock()

	var ns *Namespace

	if namespaceID != "" {
		var ok bool

		ns, ok = b.namespaces.Get(namespaceID)
		if !ok {
			return nil, fmt.Errorf("%w: namespace %s not found", ErrNamespaceNotFound, namespaceID)
		}

		if err := b.checkServiceNameAvailable(namespaceID, name, ns.Type); err != nil {
			return nil, err
		}
	}

	// Derive service type when not explicitly set.
	resolvedType := svcType
	if resolvedType == "" && ns != nil {
		switch ns.Type {
		case namespaceTypeHTTP:
			resolvedType = serviceTypeHTTP
		case namespaceTypeDNSPrivate, namespaceTypeDNSPublic:
			if dnsConfig != nil {
				resolvedType = serviceTypeDNSHTTP
			} else {
				resolvedType = serviceTypeDNS
			}
		}
	}

	id := b.nextSvcID()

	svc := &Service{
		ID:                      id,
		ARN:                     b.serviceARN(id),
		Name:                    name,
		NamespaceID:             namespaceID,
		Description:             description,
		Type:                    resolvedType,
		DNSConfig:               copyDNSConfig(dnsConfig),
		HealthCheckConfig:       copyHealthCheckConfig(hcc),
		HealthCheckCustomConfig: copyHealthCheckCustomConfig(hccc),
		Tags:                    copyTags(tags),
		CreatedAt:               time.Now(),
	}

	b.services.Put(svc)

	return copyService(svc), nil
}

// checkServiceNameAvailable enforces CreateService's documented name-collision
// rule within a namespace: "For services that are accessible by DNS queries,
// you can't create multiple services with names that differ only by case
// (such as EXAMPLE and example) ... However, if you use a namespace that's
// only accessible by API calls [HTTP], then you can create services that with
// names that differ only by case." (api_op_CreateService.go doc comment).
// Caller must hold the write lock.
func (b *InMemoryBackend) checkServiceNameAvailable(namespaceID, name, nsType string) error {
	for _, existing := range b.services.All() {
		if existing.NamespaceID != namespaceID {
			continue
		}

		collides := existing.Name == name
		if nsType != namespaceTypeHTTP {
			collides = strings.EqualFold(existing.Name, name)
		}

		if collides {
			return fmt.Errorf(
				"%w: service %s already exists in namespace %s",
				ErrServiceAlreadyExists,
				name,
				namespaceID,
			)
		}
	}

	return nil
}

// DeleteService deletes a service by ID.
// Returns ResourceInUse if instances are still registered.
func (b *InMemoryBackend) DeleteService(id string) error {
	b.mu.Lock("DeleteService")
	defer b.mu.Unlock()

	if !b.services.Has(id) {
		return fmt.Errorf("%w: service %s not found", ErrServiceNotFound, id)
	}

	if insts := b.instancesByService.Get(id); len(insts) > 0 {
		return fmt.Errorf("%w: service %s has registered instances; deregister them first", ErrResourceInUse, id)
	}

	b.services.Delete(id)
	delete(b.serviceAttributes, id)

	return nil
}

// GetService returns a service by ID.
func (b *InMemoryBackend) GetService(id string) (*Service, error) {
	b.mu.RLock("GetService")
	defer b.mu.RUnlock()

	svc, ok := b.services.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: service %s not found", ErrServiceNotFound, id)
	}

	cp := copyService(svc)
	cp.InstanceCount = len(b.instancesByService.Get(id))

	return cp, nil
}

// resolveNamespaceIDFilter rewrites a NAMESPACE_ID filter's values from ARN
// form to bare ID, matching ServiceFilter's documented "Specify one namespace
// ID or ARN" semantics (types.ServiceFilter doc comment). Must be called with
// b.mu already held by the caller.
func (b *InMemoryBackend) resolveNamespaceIDFilter(f FilterValue) FilterValue {
	if f.empty() {
		return f
	}

	resolved := make([]string, len(f.Values))

	for i, v := range f.Values {
		if matches := b.namespacesByARN.Get(v); len(matches) > 0 {
			resolved[i] = matches[0].ID
		} else {
			resolved[i] = v
		}
	}

	return FilterValue{Condition: f.Condition, Values: resolved}
}

// ListServices returns all services, optionally filtered.
func (b *InMemoryBackend) ListServices(filter ListServicesFilter) []Service {
	b.mu.RLock("ListServices")
	defer b.mu.RUnlock()

	nsFilter := b.resolveNamespaceIDFilter(filter.NamespaceID)

	all := b.services.All()
	result := make([]Service, 0, len(all))

	for _, svc := range all {
		if !nsFilter.matches(svc.NamespaceID) {
			continue
		}

		if !resourceOwnerMatches(filter.ResourceOwner) {
			continue
		}

		cp := copyService(svc)
		cp.InstanceCount = len(b.instancesByService.Get(svc.ID))
		result = append(result, *cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// UpdateService updates the description and optionally DNSConfig/HealthCheckConfig of a service.
// Returns the operation ID, matching real AWS UpdateService behavior.
func (b *InMemoryBackend) UpdateService(
	id, description string,
	dnsConfig *DNSConfig,
	hcc *HealthCheckConfig,
) (string, error) {
	b.mu.Lock("UpdateService")
	defer b.mu.Unlock()

	svc, ok := b.services.Get(id)
	if !ok {
		return "", fmt.Errorf("%w: service %s not found", ErrServiceNotFound, id)
	}

	svc.Description = description

	if dnsConfig != nil && len(dnsConfig.DNSRecords) > 0 {
		if svc.DNSConfig == nil {
			svc.DNSConfig = &DNSConfig{}
		}

		for i, newRec := range dnsConfig.DNSRecords {
			if i < len(svc.DNSConfig.DNSRecords) {
				svc.DNSConfig.DNSRecords[i].TTL = newRec.TTL
			}
		}
	}

	if hcc != nil {
		svc.HealthCheckConfig = copyHealthCheckConfig(hcc)
	}

	now := time.Now()
	opID := b.nextOpID()
	b.operations.Put(&Operation{
		ID:         opID,
		Type:       operationTypeUpdateService,
		Status:     operationStatusSuccess,
		Targets:    map[string]string{typeService: id},
		CreateDate: now,
		UpdateDate: now,
	})

	return opID, nil
}

// GetServiceAttributes returns the custom attributes for a service.
func (b *InMemoryBackend) GetServiceAttributes(serviceID string) (string, map[string]string, error) {
	b.mu.RLock("GetServiceAttributes")
	defer b.mu.RUnlock()

	svc, ok := b.services.Get(serviceID)
	if !ok {
		return "", nil, fmt.Errorf("%w: service %s not found", ErrServiceNotFound, serviceID)
	}

	attrs, ok := b.serviceAttributes[serviceID]
	if !ok {
		return "", nil, fmt.Errorf("%w: no attributes found for service %s", ErrServiceAttributesNotFound, serviceID)
	}

	return svc.ARN, copyAttrs(attrs), nil
}

// UpdateServiceAttributes sets or merges custom attributes for a service
// identified by ID or ARN (real ServiceId wire field accepts either).
func (b *InMemoryBackend) UpdateServiceAttributes(serviceIDOrARN string, attributes map[string]string) error {
	b.mu.Lock("UpdateServiceAttributes")
	defer b.mu.Unlock()

	svcID := serviceIDOrARN
	if !b.services.Has(svcID) {
		svcMatches := b.servicesByARN.Get(serviceIDOrARN)
		if len(svcMatches) == 0 {
			return fmt.Errorf("%w: service %s not found", ErrServiceNotFound, serviceIDOrARN)
		}

		svcID = svcMatches[0].ID
	}

	existing := b.serviceAttributes[svcID]

	merged := len(existing)
	for k := range attributes {
		if _, ok := existing[k]; !ok {
			merged++
		}
	}

	if merged > maxServiceAttrCount {
		return fmt.Errorf(
			"%w: service %s would have %d attributes, exceeding the maximum of %d",
			ErrServiceAttributesLimitExceeded,
			serviceIDOrARN,
			merged,
			maxServiceAttrCount,
		)
	}

	if existing == nil {
		existing = make(map[string]string)
	}

	maps.Copy(existing, attributes)

	b.serviceAttributes[svcID] = existing

	return nil
}

// DeleteServiceAttributes removes the specified attribute keys from a
// service, per the real DeleteServiceAttributesInput.Attributes doc comment
// ("A list of keys corresponding to each attribute that you want to
// delete") -- it does not clear attributes left unspecified.
func (b *InMemoryBackend) DeleteServiceAttributes(serviceID string, keys []string) error {
	b.mu.Lock("DeleteServiceAttributes")
	defer b.mu.Unlock()

	if !b.services.Has(serviceID) {
		return fmt.Errorf("%w: service %s not found", ErrServiceNotFound, serviceID)
	}

	for _, k := range keys {
		delete(b.serviceAttributes[serviceID], k)
	}

	return nil
}
