package dms

// ReplicationInstanceCount returns the number of replication instances. Used only in tests.
func (b *InMemoryBackend) ReplicationInstanceCount() int {
	b.mu.RLock("ReplicationInstanceCount")
	defer b.mu.RUnlock()

	return b.replicationInstances.Len()
}

// EndpointCount returns the number of endpoints. Used only in tests.
func (b *InMemoryBackend) EndpointCount() int {
	b.mu.RLock("EndpointCount")
	defer b.mu.RUnlock()

	return b.endpoints.Len()
}

// ReplicationTaskCount returns the number of replication tasks. Used only in tests.
func (b *InMemoryBackend) ReplicationTaskCount() int {
	b.mu.RLock("ReplicationTaskCount")
	defer b.mu.RUnlock()

	return b.replicationTasks.Len()
}

// DataMigrationCount returns the number of data migrations. Used only in tests.
func (b *InMemoryBackend) DataMigrationCount() int {
	b.mu.RLock("DataMigrationCount")
	defer b.mu.RUnlock()

	return b.dataMigrations.Len()
}

// DataProviderCount returns the number of data providers. Used only in tests.
func (b *InMemoryBackend) DataProviderCount() int {
	b.mu.RLock("DataProviderCount")
	defer b.mu.RUnlock()

	return b.dataProviders.Len()
}

// EventSubscriptionCount returns the number of event subscriptions. Used only in tests.
func (b *InMemoryBackend) EventSubscriptionCount() int {
	b.mu.RLock("EventSubscriptionCount")
	defer b.mu.RUnlock()

	return b.eventSubscriptions.Len()
}

// AddEventInternal seeds a DMS operational event directly without HTTP. Used only in tests.
func (b *InMemoryBackend) AddEventInternal(sourceID, sourceType, msg string, cats []string) {
	b.mu.Lock("AddEventInternal")
	defer b.mu.Unlock()
	b.appendEvent(b.region, sourceID, sourceType, msg, cats)
}

// FleetAdvisorCollectorCount returns the number of Fleet Advisor collectors. Used only in tests.
func (b *InMemoryBackend) FleetAdvisorCollectorCount() int {
	b.mu.RLock("FleetAdvisorCollectorCount")
	defer b.mu.RUnlock()

	return b.fleetAdvisorCollectors.Len()
}

// InstanceProfileCount returns the number of instance profiles. Used only in tests.
func (b *InMemoryBackend) InstanceProfileCount() int {
	b.mu.RLock("InstanceProfileCount")
	defer b.mu.RUnlock()

	return b.instanceProfiles.Len()
}

// ConnectionCount returns the number of stored connections. Used only in tests.
func (b *InMemoryBackend) ConnectionCount() int {
	b.mu.RLock("ConnectionCount")
	defer b.mu.RUnlock()

	return b.connections.Len()
}

// FleetAdvisorCollectorByIDCount returns entries in the fleetAdvisorCollectorsByID index. Used only in tests.
func (b *InMemoryBackend) FleetAdvisorCollectorByIDCount() int {
	b.mu.RLock("FleetAdvisorCollectorByIDCount")
	defer b.mu.RUnlock()

	return b.fleetAdvisorCollectorsByID.Len()
}

// DataMigrationByARNCount returns entries in the dataMigrationsByARN index. Used only in tests.
func (b *InMemoryBackend) DataMigrationByARNCount() int {
	b.mu.RLock("DataMigrationByARNCount")
	defer b.mu.RUnlock()

	return b.dataMigrationsByARN.Len()
}

// DataProviderByARNCount returns entries in the dataProvidersByARN index. Used only in tests.
func (b *InMemoryBackend) DataProviderByARNCount() int {
	b.mu.RLock("DataProviderByARNCount")
	defer b.mu.RUnlock()

	return b.dataProvidersByARN.Len()
}

// InstanceProfileByARNCount returns entries in the instanceProfilesByARN index. Used only in tests.
func (b *InMemoryBackend) InstanceProfileByARNCount() int {
	b.mu.RLock("InstanceProfileByARNCount")
	defer b.mu.RUnlock()

	return b.instanceProfilesByARN.Len()
}

// HasEndpointSchemas reports whether the endpointSchemas side map still
// holds an entry for arn in region. Used only in tests.
func (b *InMemoryBackend) HasEndpointSchemas(region, arn string) bool {
	b.mu.RLock("HasEndpointSchemas")
	defer b.mu.RUnlock()

	_, ok := b.endpointSchemasStoreRO(region)[arn]

	return ok
}
