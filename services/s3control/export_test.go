package s3control

// AccessBlockCount returns the number of stored public access block configurations.
func AccessBlockCount(b *InMemoryBackend) int {
	b.mu.RLock("AccessBlockCount")
	defer b.mu.RUnlock()

	return b.configs.Len()
}

// AccessGrantsInstanceCount returns the number of stored access grants instances.
func AccessGrantsInstanceCount(b *InMemoryBackend) int {
	b.mu.RLock("AccessGrantsInstanceCount")
	defer b.mu.RUnlock()

	return b.accessGrantsInstances.Len()
}

// AccessGrantCount returns the number of stored access grants.
func AccessGrantCount(b *InMemoryBackend) int {
	b.mu.RLock("AccessGrantCount")
	defer b.mu.RUnlock()

	return b.accessGrants.Len()
}

// AccessGrantsLocationCount returns the number of stored access grants locations.
func AccessGrantsLocationCount(b *InMemoryBackend) int {
	b.mu.RLock("AccessGrantsLocationCount")
	defer b.mu.RUnlock()

	return b.accessGrantsLocations.Len()
}

// AccessPointCount returns the number of stored access points.
func AccessPointCount(b *InMemoryBackend) int {
	b.mu.RLock("AccessPointCount")
	defer b.mu.RUnlock()

	return b.accessPoints.Len()
}

// ObjectLambdaAccessPointCount returns the number of stored Object Lambda access points.
func ObjectLambdaAccessPointCount(b *InMemoryBackend) int {
	b.mu.RLock("ObjectLambdaAccessPointCount")
	defer b.mu.RUnlock()

	return b.objectLambdaAccessPoints.Len()
}

// OutpostsBucketCount returns the number of stored S3 Outposts buckets.
func OutpostsBucketCount(b *InMemoryBackend) int {
	b.mu.RLock("OutpostsBucketCount")
	defer b.mu.RUnlock()

	return b.outpostsBuckets.Len()
}

// BatchJobCount returns the number of stored batch jobs.
func BatchJobCount(b *InMemoryBackend) int {
	b.mu.RLock("BatchJobCount")
	defer b.mu.RUnlock()

	return b.batchJobs.Len()
}

// MRAPRequestCount returns the number of stored MRAP async requests.
func MRAPRequestCount(b *InMemoryBackend) int {
	b.mu.RLock("MRAPRequestCount")
	defer b.mu.RUnlock()

	return b.mrapRequests.Len()
}

// StorageLensGroupCount returns the number of stored Storage Lens groups.
func StorageLensGroupCount(b *InMemoryBackend) int {
	b.mu.RLock("StorageLensGroupCount")
	defer b.mu.RUnlock()

	return b.storageLensGroups.Len()
}

// HandlerOpsLen returns the number of operations in GetSupportedOperations.
func HandlerOpsLen(h *Handler) int {
	return len(h.GetSupportedOperations())
}

// AccessPointPABCount returns the number of per-AP public access block configs stored.
func AccessPointPABCount(b *InMemoryBackend) int {
	b.mu.RLock("AccessPointPABCount")
	defer b.mu.RUnlock()

	return b.accessPointPABs.Len()
}

// MRAPCount returns the number of stored MRAP instances.
func MRAPCount(b *InMemoryBackend) int {
	b.mu.RLock("MRAPCount")
	defer b.mu.RUnlock()

	return b.mraps.Len()
}

// StorageLensConfigCount returns the number of stored Storage Lens configurations.
func StorageLensConfigCount(b *InMemoryBackend) int {
	b.mu.RLock("StorageLensConfigCount")
	defer b.mu.RUnlock()

	return len(b.storageLensConfigs)
}

// MRAPRoutesCount returns the number of stored MRAP route configurations.
func MRAPRoutesCount(b *InMemoryBackend) int {
	b.mu.RLock("MRAPRoutesCount")
	defer b.mu.RUnlock()

	return len(b.mrapRoutes)
}
