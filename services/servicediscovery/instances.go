package servicediscovery

import (
	"fmt"
	"sort"
	"time"
)

// RegisterInstance registers an instance to a service. If the service has a
// HealthCheckCustomConfig and attrs contains AWS_INIT_HEALTH_STATUS (HEALTHY
// or UNHEALTHY), that value seeds the instance's initial custom health
// status -- matching real Cloud Map: "If the service configuration includes
// HealthCheckCustomConfig, you can optionally use AWS_INIT_HEALTH_STATUS to
// specify the initial status of the custom health check ... If you don't
// specify a value ..., the initial status is HEALTHY."
// (api_op_RegisterInstance.go doc comment).
func (b *InMemoryBackend) RegisterInstance(serviceID, instanceID string, attrs map[string]string) (string, error) {
	b.mu.Lock("RegisterInstance")
	defer b.mu.Unlock()

	svc, ok := b.services.Get(serviceID)
	if !ok {
		return "", fmt.Errorf("%w: service %s not found", ErrServiceNotFound, serviceID)
	}

	inst := &Instance{
		ID:         instanceID,
		ServiceID:  serviceID,
		Attributes: copyAttrs(attrs),
	}
	b.instances.Put(inst)

	key := instanceKey(serviceID, instanceID)
	if svc.HealthCheckCustomConfig != nil {
		switch attrs[instanceAttrInitHealthStatus] {
		case instanceHealthStatusHealthy:
			b.instanceHealthStatuses[key] = instanceHealthStatusHealthy
		case instanceHealthStatusUnhealthy:
			b.instanceHealthStatuses[key] = instanceHealthStatusUnhealthy
		}
	}

	b.resyncServiceDNS(svc)

	b.instanceRevision++

	now := time.Now()
	opID := b.nextOpID()
	b.operations.Put(&Operation{
		ID:         opID,
		Type:       operationTypeRegisterInstance,
		Status:     operationStatusSuccess,
		Targets:    map[string]string{typeInstance: instanceID, typeService: serviceID},
		CreateDate: now,
		UpdateDate: now,
	})

	return opID, nil
}

// DeregisterInstance deregisters an instance from a service.
func (b *InMemoryBackend) DeregisterInstance(serviceID, instanceID string) (string, error) {
	b.mu.Lock("DeregisterInstance")
	defer b.mu.Unlock()

	key := instanceKey(serviceID, instanceID)
	if !b.instances.Has(key) {
		return "", fmt.Errorf("%w: instance %s in service %s not found", ErrInstanceNotFound, instanceID, serviceID)
	}

	b.instances.Delete(key)
	delete(b.instanceHealthStatuses, key)

	if svc, ok := b.services.Get(serviceID); ok {
		b.resyncServiceDNS(svc)
	}

	b.instanceRevision++

	now := time.Now()
	opID := b.nextOpID()
	b.operations.Put(&Operation{
		ID:         opID,
		Type:       operationTypeDeregisterInstance,
		Status:     operationStatusSuccess,
		Targets:    map[string]string{typeInstance: instanceID, typeService: serviceID},
		CreateDate: now,
		UpdateDate: now,
	})

	return opID, nil
}

// GetInstance returns a registered instance.
func (b *InMemoryBackend) GetInstance(serviceID, instanceID string) (*Instance, error) {
	b.mu.RLock("GetInstance")
	defer b.mu.RUnlock()

	key := instanceKey(serviceID, instanceID)
	inst, ok := b.instances.Get(key)

	if !ok {
		return nil, fmt.Errorf("%w: instance %s in service %s not found", ErrInstanceNotFound, instanceID, serviceID)
	}

	cp := *inst
	cp.Attributes = copyAttrs(inst.Attributes)

	return &cp, nil
}

// ListInstances returns all instances registered to a service.
func (b *InMemoryBackend) ListInstances(serviceID string) ([]Instance, error) {
	b.mu.RLock("ListInstances")
	defer b.mu.RUnlock()

	if !b.services.Has(serviceID) {
		return nil, fmt.Errorf("%w: service %s not found", ErrServiceNotFound, serviceID)
	}

	insts := b.instancesByService.Get(serviceID)
	result := make([]Instance, 0, len(insts))

	for _, inst := range insts {
		cp := *inst
		cp.Attributes = copyAttrs(inst.Attributes)
		result = append(result, cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result, nil
}

// GetInstancesHealthStatus returns the health status for instances in a service.
// If instanceIDs is non-empty, only those instances are included.
// Instances without a recorded status default to HEALTHY.
func (b *InMemoryBackend) GetInstancesHealthStatus(serviceID string, instanceIDs []string) (map[string]string, error) {
	b.mu.RLock("GetInstancesHealthStatus")
	defer b.mu.RUnlock()

	if !b.services.Has(serviceID) {
		return nil, fmt.Errorf("%w: service %s not found", ErrServiceNotFound, serviceID)
	}

	insts := b.instancesByService.Get(serviceID)

	filter := make(map[string]struct{}, len(instanceIDs))
	for _, id := range instanceIDs {
		filter[id] = struct{}{}
	}

	if len(filter) > 0 {
		registered := make(map[string]struct{}, len(insts))
		for _, inst := range insts {
			registered[inst.ID] = struct{}{}
		}

		for _, id := range instanceIDs {
			if _, ok := registered[id]; !ok {
				return nil, fmt.Errorf("%w: instance %s in service %s not found", ErrInstanceNotFound, id, serviceID)
			}
		}
	}

	statuses := make(map[string]string)

	for _, inst := range insts {
		instID := inst.ID

		if len(filter) > 0 {
			if _, ok := filter[instID]; !ok {
				continue
			}
		}

		key := instanceKey(serviceID, instID)
		status := b.instanceHealthStatuses[key]
		if status == "" {
			status = instanceHealthStatusHealthy
		}

		statuses[instID] = status
	}

	return statuses, nil
}

// UpdateInstanceCustomHealthStatus sets a custom health status for an instance.
// Returns CustomHealthNotFound if the service has no HealthCheckCustomConfig.
func (b *InMemoryBackend) UpdateInstanceCustomHealthStatus(serviceID, instanceID, status string) error {
	b.mu.Lock("UpdateInstanceCustomHealthStatus")
	defer b.mu.Unlock()

	if status != instanceHealthStatusHealthy && status != instanceHealthStatusUnhealthy {
		return fmt.Errorf(
			"%w: status must be %s or %s",
			ErrInvalidInput,
			instanceHealthStatusHealthy,
			instanceHealthStatusUnhealthy,
		)
	}

	svc, ok := b.services.Get(serviceID)
	if !ok {
		return fmt.Errorf("%w: service %s not found", ErrServiceNotFound, serviceID)
	}

	key := instanceKey(serviceID, instanceID)
	if !b.instances.Has(key) {
		return fmt.Errorf("%w: instance %s in service %s not found", ErrInstanceNotFound, instanceID, serviceID)
	}

	if svc.HealthCheckCustomConfig == nil {
		return fmt.Errorf(
			"%w: service %s has no custom health check configured",
			ErrCustomHealthNotFound,
			serviceID,
		)
	}

	b.instanceHealthStatuses[key] = status

	return nil
}

// dnsHostname returns the resolvable DNS name for a Cloud Map service --
// "<service-name>.<namespace-name>", matching real Cloud Map's generated
// resource record name. HTTP namespaces have no DNS at all (only
// DiscoverInstances), so ns being nil or of type HTTP yields "".
func dnsHostname(svcName string, ns *Namespace) string {
	if ns == nil || ns.Type == namespaceTypeHTTP {
		return ""
	}

	return svcName + "." + ns.Name
}

// resyncServiceDNS re-derives svc's DNS registration in the embedded DNS
// server from every currently registered instance's AWS_INSTANCE_IPV4/IPV6/
// CNAME attributes. It deregisters first and rebuilds from scratch -- like
// route53.resyncDNSName -- because pkgs/dns.RegisterRecord only appends
// values and never replaces or removes a single one. Caller must hold b.mu.
func (b *InMemoryBackend) resyncServiceDNS(svc *Service) {
	if b.dns == nil {
		return
	}

	ns, _ := b.namespaces.Get(svc.NamespaceID)

	hostname := dnsHostname(svc.Name, ns)
	if hostname == "" {
		return
	}

	b.dns.Deregister(hostname)

	for _, inst := range b.instancesByService.Get(svc.ID) {
		if ip4 := inst.Attributes[instanceAttrIPv4]; ip4 != "" {
			b.dns.RegisterRecord(hostname, "A", []string{ip4})
		}

		if ip6 := inst.Attributes[instanceAttrIPv6]; ip6 != "" {
			b.dns.RegisterRecord(hostname, "AAAA", []string{ip6})
		}

		if cname := inst.Attributes[instanceAttrCNAME]; cname != "" {
			b.dns.RegisterRecord(hostname, "CNAME", []string{cname})
		}
	}
}
