package glue

import "time"

// GlueResourceNameForTest exposes glueResourceName for unit tests.
func GlueResourceNameForTest(resourceARN, resourceType string) string {
	return glueResourceName(resourceARN, resourceType)
}

// PendingDueForTest reports whether any lifecycle transition is due at now. It
// exposes the reconciler's cheap pending-work guard so tests can assert the
// performance optimisation: an idle backend reports no due work (so the reconciler
// never takes the global write lock).
func PendingDueForTest(b *InMemoryBackend, now time.Time) bool {
	b.mu.RLock("PendingDueForTest")
	defer b.mu.RUnlock()

	return b.pendingDueLocked(now)
}

// AdvanceStatesForTest exposes advanceStates so tests can drive lazy transition
// advancement deterministically without waiting on the ticker.
func AdvanceStatesForTest(b *InMemoryBackend, now time.Time) {
	b.advanceStates(now)
}

// CustomConnectionTypeCountForTest returns the number of registered custom
// connection types. Used only in tests.
func CustomConnectionTypeCountForTest(b *InMemoryBackend) int {
	b.mu.RLock("CustomConnectionTypeCountForTest")
	defer b.mu.RUnlock()

	return b.customConnectionTypes.Len()
}

// DatabaseCount returns the number of databases stored in the backend. Used only in tests.
func DatabaseCount(b *InMemoryBackend) int {
	b.mu.RLock("DatabaseCount")
	defer b.mu.RUnlock()

	return b.databases.Len()
}

// TableCount returns the total number of tables stored in the backend. Used only in tests.
func TableCount(b *InMemoryBackend) int {
	b.mu.RLock("TableCount")
	defer b.mu.RUnlock()

	return b.tables.Len()
}

// CrawlerCount returns the number of crawlers stored in the backend. Used only in tests.
func CrawlerCount(b *InMemoryBackend) int {
	b.mu.RLock("CrawlerCount")
	defer b.mu.RUnlock()

	return b.crawlers.Len()
}

// JobCount returns the number of jobs stored in the backend. Used only in tests.
func JobCount(b *InMemoryBackend) int {
	b.mu.RLock("JobCount")
	defer b.mu.RUnlock()

	return b.jobs.Len()
}

// PartitionCount returns the number of partitions stored in the backend. Used only in tests.
func PartitionCount(b *InMemoryBackend) int {
	b.mu.RLock("PartitionCount")
	defer b.mu.RUnlock()

	return b.partitions.Len()
}

// TableVersionCount returns the number of table versions stored in the backend. Used only in tests.
func TableVersionCount(b *InMemoryBackend) int {
	b.mu.RLock("TableVersionCount")
	defer b.mu.RUnlock()

	return b.tableVersions.Len()
}

// ConnectionCount returns the number of connections stored in the backend. Used only in tests.
func ConnectionCount(b *InMemoryBackend) int {
	b.mu.RLock("ConnectionCount")
	defer b.mu.RUnlock()

	return b.connections.Len()
}

// BlueprintCount returns the number of blueprints stored in the backend. Used only in tests.
func BlueprintCount(b *InMemoryBackend) int {
	b.mu.RLock("BlueprintCount")
	defer b.mu.RUnlock()

	return b.blueprints.Len()
}

// CustomEntityTypeCount returns the number of custom entity types in the backend. Used only in tests.
func CustomEntityTypeCount(b *InMemoryBackend) int {
	b.mu.RLock("CustomEntityTypeCount")
	defer b.mu.RUnlock()

	return b.customEntityTypes.Len()
}

// DataQualityResultCount returns the number of data quality results in the backend. Used only in tests.
func DataQualityResultCount(b *InMemoryBackend) int {
	b.mu.RLock("DataQualityResultCount")
	defer b.mu.RUnlock()

	return b.dataQualityResult.Len()
}

// DevEndpointCount returns the number of dev endpoints in the backend. Used only in tests.
func DevEndpointCount(b *InMemoryBackend) int {
	b.mu.RLock("DevEndpointCount")
	defer b.mu.RUnlock()

	return b.devEndpoints.Len()
}

// HandlerOpsLen returns the number of operations registered in the handler's dispatch table.
func (h *Handler) HandlerOpsLen() int {
	return len(h.ops)
}

// JobRunCount returns the total number of job runs across all jobs in the backend. Used only in tests.
func JobRunCount(b *InMemoryBackend) int {
	b.mu.RLock("JobRunCount")
	defer b.mu.RUnlock()

	total := 0
	for _, runs := range b.jobRuns {
		total += len(runs)
	}

	return total
}

// DataQualityRulesetCount returns the number of data quality rulesets in the backend. Used only in tests.
func DataQualityRulesetCount(b *InMemoryBackend) int {
	b.mu.RLock("DataQualityRulesetCount")
	defer b.mu.RUnlock()

	return b.dataQualityRulesets.Len()
}

// DataQualityEvalRunCount returns the number of data quality evaluation runs in the backend. Used only in tests.
func DataQualityEvalRunCount(b *InMemoryBackend) int {
	b.mu.RLock("DataQualityEvalRunCount")
	defer b.mu.RUnlock()

	return b.dataQualityEvalRuns.Len()
}

// MLTaskRunCount returns the total number of ML task runs across all transforms. Used only in tests.
func MLTaskRunCount(b *InMemoryBackend) int {
	b.mu.RLock("MLTaskRunCount")
	defer b.mu.RUnlock()

	return b.mlTaskRuns.Len()
}

// TableColumnStatsCount returns the number of stored table column-statistics
// entries in the backend. Used only in tests.
func TableColumnStatsCount(b *InMemoryBackend) int {
	b.mu.RLock("TableColumnStatsCount")
	defer b.mu.RUnlock()

	return len(b.tableColumnStats)
}

// PartitionColumnStatsCount returns the number of stored partition
// column-statistics entries in the backend. Used only in tests.
func PartitionColumnStatsCount(b *InMemoryBackend) int {
	b.mu.RLock("PartitionColumnStatsCount")
	defer b.mu.RUnlock()

	return len(b.partitionColumnStats)
}

// TableOptimizerCount returns the number of registered table optimizers in
// the backend. Used only in tests.
func TableOptimizerCount(b *InMemoryBackend) int {
	b.mu.RLock("TableOptimizerCount")
	defer b.mu.RUnlock()

	return b.tableOptimizers.Len()
}

// UDFCount returns the number of registered user-defined functions in the
// backend. Used only in tests.
func UDFCount(b *InMemoryBackend) int {
	b.mu.RLock("UDFCount")
	defer b.mu.RUnlock()

	return b.udfs.Len()
}
