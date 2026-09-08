package glue

// Code in this file supports Phase 3.3 of the datalayer refactor: every
// map[string]*T resource field on InMemoryBackend whose key is a pure
// function of the stored value's own fields is registered exactly once,
// here, as a *store.Table[T] on b.registry. See pkgs/store's package doc and
// the services/ec2 conversion (commit 12e611a4) for the pattern this follows.
//
// A number of fields are deliberately NOT registered here and remain plain
// maps -- see the comment block above registerAllTables for the list and why.
import (
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

func databaseKeyFn(v *Database) string { return v.Name }

// tableRecordKeyFn is the [store.Table] key function for the glue Table type.
// Named to avoid colliding with the tableKey composite-key helper or the
// Table type itself.
func tableRecordKeyFn(v *Table) string { return tableKey(v.DatabaseName, v.Name) }

func crawlerKeyFn(v *Crawler) string { return v.Name }

func jobKeyFn(v *Job) string { return v.Name }

func partitionEntryKeyFn(v *Partition) string {
	return partitionKey(v.DatabaseName, v.TableName, v.Values)
}

// partitionTableIndexKeyFn groups partitions by owning table for
// InMemoryBackend.GetPartitions, so it can look up one table's partitions
// directly instead of scanning every partition in the backend.
func partitionTableIndexKeyFn(v *Partition) string { return tableKey(v.DatabaseName, v.TableName) }

// tableVersionEntryKeyFn derives the composite db|table|versionID key from a
// TableVersion's own nested Table pointer. It tolerates a nil Table (rather
// than panicking) because AddTableVersionInternal is the only writer and, for
// this identity function to be pure, it always populates Table's
// DatabaseName/Name to match the dbName/tableName it was given -- see that
// method for details.
func tableVersionEntryKeyFn(v *TableVersion) string {
	var dbName, tableName string
	if v.Table != nil {
		dbName, tableName = v.Table.DatabaseName, v.Table.Name
	}

	return tableVersionKey(dbName, tableName, v.VersionID)
}

func connectionKeyFn(v *Connection) string { return v.Name }

func blueprintKeyFn(v *Blueprint) string { return v.Name }

func customEntityTypeKeyFn(v *CustomEntityType) string { return v.Name }

func dataQualityResultKeyFn(v *DataQualityResult) string { return v.ResultID }

func devEndpointKeyFn(v *DevEndpoint) string { return v.EndpointName }

func jobBookmarkKeyFn(v *JobBookmark) string { return v.JobName }

func dataQualityRulesetKeyFn(v *DataQualityRuleset) string { return v.Name }

func dataQualityEvalRunKeyFn(v *DataQualityEvaluationRun) string { return v.RunID }

func triggerKeyFn(v *Trigger) string { return v.Name }

func workflowKeyFn(v *Workflow) string { return v.Name }

func registryKeyFn(v *Registry) string { return v.Name }

// schemaEntryKeyFn wraps the existing schemaKey composite-key helper.
func schemaEntryKeyFn(v *Schema) string { return schemaKey(v.RegistryName, v.SchemaName) }

func udfEntryKeyFn(v *UserDefinedFunction) string { return v.DatabaseName + "|" + v.FunctionName }

func securityConfigKeyFn(v *SecurityConfiguration) string { return v.Name }

func sessionKeyFn(v *Session) string { return v.SessionID }

func tableOptimizerEntryKeyFn(v *tableOptimizerRecord) string {
	return v.DatabaseName + "|" + v.TableName + "|" + v.Optimizer.Type
}

func mlTransformKeyFn(v *MLTransform) string { return v.TransformID }

func catalogEntryKeyFn(v *CatalogEntry) string { return v.CatalogID }

func usageProfileKeyFn(v *UsageProfile) string { return v.Name }

func blueprintRunKeyFn(v *BlueprintRun) string { return v.RunID }

func dqRecommendationRunKeyFn(v *DQRuleRecommendationRun) string { return v.RecommendationRunID }

func columnStatTaskSettingsEntryKeyFn(v *ColumnStatisticsTaskSettings) string {
	return v.DatabaseName + "|" + v.TableName
}

func columnStatTaskRunKeyFn(v *ColumnStatisticsTaskRun) string { return v.ColumnStatisticsTaskRunID }

func materializedViewRunKeyFn(v *MaterializedViewRefreshRun) string { return v.TaskRunID }

func integrationKeyFn(v *Integration) string { return v.IntegrationName }

func integrationResourcePropKeyFn(v *IntegrationResourceProperty) string { return v.ResourceArn }

func integrationTablePropKeyFn(v *IntegrationTableProperties) string {
	return v.ResourceArn + "|" + v.TableName
}

// mlTaskRunEntryKeyFn wraps the existing mlTaskRunKey composite-key helper.
func mlTaskRunEntryKeyFn(v *MLTaskRun) string { return mlTaskRunKey(v.TransformID, v.TaskRunID) }

// dqStatisticAnnotationKeyFn wraps the existing dqAnnotationKey composite-key helper.
func dqStatisticAnnotationKeyFn(v *StatisticAnnotation) string {
	return dqAnnotationKey(v.ProfileID, v.StatisticID)
}

func customConnectionTypeKeyFn(v *ConnectionTypeInfo) string { return v.ConnectionType }

func glossaryKeyFn(v *Glossary) string { return v.ID }

func glossaryTermKeyFn(v *GlossaryTerm) string { return v.ID }

func assetTypeKeyFn(v *AssetType) string { return v.ID }

func assetKeyFn(v *Asset) string { return v.ID }

func formTypeKeyFn(v *FormType) string { return v.ID }

// registerAllTables registers every converted resource map on b.registry
// exactly once. It must be called during construction only (immediately
// after b.registry is created), never on every Reset() -- store.Register
// panics on a duplicate name, so runtime resets go through
// registry.ResetAll() instead (see InMemoryBackend.Reset in store.go).
//
// The following resource fields are deliberately left as plain maps (not
// registered here) because their key is not a pure function of the stored
// value's own fields, which store.Table requires, or because the field holds
// a one-to-many list rather than a single value per key:
//   - partitionIndexes: value type PartitionIndex carries no database/table
//     identity field of its own; keyed externally as "db|table|indexName".
//     GetPartitionIndexes also relies on a prefix scan over that raw key.
//   - tableColumnStats / partitionColumnStats: value type ColumnStatistics
//     carries only ColumnName, no database/table (or partition) identity;
//     keyed externally.
//   - resourcePolicies: value type resourcePolicyEntry carries no resource-ARN
//     identity field; keyed externally by ARN or "__global__".
//   - catalogEncryptionSettings / catalogImports: value types carry no
//     catalogID identity field; keyed externally by catalogID or accountID.
//   - schemaVersionMetadata: nested map[string]string value, not a *T pointer
//     -- store.Table requires a pointer-to-struct value with its own identity.
//   - jobRuns / workflowRuns / schemaVersions / sessionStatements /
//     crawlHistory: one-to-many (map[string][]*T) history/list fields, not
//     the map[string]*T shape store.Table replaces.
//   - jobRunReadyAt / jobRunDoneAt / jobRunTimeoutAt / jobRunStopAt /
//     crawlerReadyAt: ephemeral lifecycle-timer bookkeeping (ready/done/
//     timeout/stop rather than resource state), not persisted, and not
//     map[string]*T.
//   - iterableFormItems: a nested per-asset, per-iterable-form-name collection
//     (map[string]map[string]map[string]*iterableFormItemRecord), not the
//     map[string]*T shape store.Table replaces -- see the field's doc comment
//     on InMemoryBackend in store.go.
func registerAllTables(b *InMemoryBackend) {
	for _, register := range tableRegistrations {
		register(b)
	}
}

// tableRegistrations is the data-driven list registerAllTables walks: one
// closure per resource table, each binding its own store.New/store.Register
// call to the concrete field and value type. A closure list (rather than one
// statement per field in a single function body) keeps registerAllTables
// small regardless of how many resource tables the backend grows to.
//
//nolint:gochecknoglobals // registration table, analogous to errCodeLookup-style lookup tables elsewhere
var tableRegistrations = []func(*InMemoryBackend){
	func(b *InMemoryBackend) {
		b.databases = store.Register(b.registry, "databases", store.New(databaseKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tables = store.Register(b.registry, "tables", store.New(tableRecordKeyFn))
	},
	func(b *InMemoryBackend) {
		b.crawlers = store.Register(b.registry, "crawlers", store.New(crawlerKeyFn))
	},
	func(b *InMemoryBackend) {
		b.jobs = store.Register(b.registry, "jobs", store.New(jobKeyFn))
	},
	func(b *InMemoryBackend) {
		b.partitions = store.Register(b.registry, "partitions", store.New(partitionEntryKeyFn))
		b.partitionsByTable = b.partitions.AddIndex("partitionsByTable", partitionTableIndexKeyFn)
	},
	func(b *InMemoryBackend) {
		b.tableVersions = store.Register(b.registry, "tableVersions", store.New(tableVersionEntryKeyFn))
	},
	func(b *InMemoryBackend) {
		b.connections = store.Register(b.registry, "connections", store.New(connectionKeyFn))
	},
	func(b *InMemoryBackend) {
		b.blueprints = store.Register(b.registry, "blueprints", store.New(blueprintKeyFn))
	},
	func(b *InMemoryBackend) {
		b.customEntityTypes = store.Register(b.registry, "customEntityTypes", store.New(customEntityTypeKeyFn))
	},
	func(b *InMemoryBackend) {
		b.dataQualityResult = store.Register(b.registry, "dataQualityResult", store.New(dataQualityResultKeyFn))
	},
	func(b *InMemoryBackend) {
		b.devEndpoints = store.Register(b.registry, "devEndpoints", store.New(devEndpointKeyFn))
	},
	func(b *InMemoryBackend) {
		b.jobBookmarks = store.Register(b.registry, "jobBookmarks", store.New(jobBookmarkKeyFn))
	},
	func(b *InMemoryBackend) {
		b.dataQualityRulesets = store.Register(b.registry, "dataQualityRulesets", store.New(dataQualityRulesetKeyFn))
	},
	func(b *InMemoryBackend) {
		b.dataQualityEvalRuns = store.Register(
			b.registry,
			"dataQualityEvalRuns",
			store.New(dataQualityEvalRunKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.triggers = store.Register(b.registry, "triggers", store.New(triggerKeyFn))
	},
	func(b *InMemoryBackend) {
		b.workflows = store.Register(b.registry, "workflows", store.New(workflowKeyFn))
	},
	func(b *InMemoryBackend) {
		b.classifiers = store.Register(b.registry, "classifiers", store.New(classifierName))
	},
	func(b *InMemoryBackend) {
		b.registries = store.Register(b.registry, "registries", store.New(registryKeyFn))
	},
	func(b *InMemoryBackend) {
		b.schemas = store.Register(b.registry, "schemas", store.New(schemaEntryKeyFn))
	},
	func(b *InMemoryBackend) {
		b.udfs = store.Register(b.registry, "udfs", store.New(udfEntryKeyFn))
	},
	func(b *InMemoryBackend) {
		b.securityConfigs = store.Register(b.registry, "securityConfigs", store.New(securityConfigKeyFn))
	},
	func(b *InMemoryBackend) {
		b.sessions = store.Register(b.registry, "sessions", store.New(sessionKeyFn))
	},
	func(b *InMemoryBackend) {
		b.tableOptimizers = store.Register(b.registry, "tableOptimizers", store.New(tableOptimizerEntryKeyFn))
	},
	func(b *InMemoryBackend) {
		b.mlTransforms = store.Register(b.registry, "mlTransforms", store.New(mlTransformKeyFn))
	},
	func(b *InMemoryBackend) {
		b.catalogs = store.Register(b.registry, "catalogs", store.New(catalogEntryKeyFn))
	},
	func(b *InMemoryBackend) {
		b.usageProfiles = store.Register(b.registry, "usageProfiles", store.New(usageProfileKeyFn))
	},
	func(b *InMemoryBackend) {
		b.blueprintRuns = store.Register(b.registry, "blueprintRuns", store.New(blueprintRunKeyFn))
	},
	func(b *InMemoryBackend) {
		b.dqRecommendationRuns = store.Register(
			b.registry,
			"dqRecommendationRuns",
			store.New(dqRecommendationRunKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.columnStatTaskSettings = store.Register(
			b.registry,
			"columnStatTaskSettings",
			store.New(columnStatTaskSettingsEntryKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.columnStatTaskRuns = store.Register(b.registry, "columnStatTaskRuns", store.New(columnStatTaskRunKeyFn))
	},
	func(b *InMemoryBackend) {
		b.materializedViewRuns = store.Register(
			b.registry,
			"materializedViewRuns",
			store.New(materializedViewRunKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.integrations = store.Register(b.registry, "integrations", store.New(integrationKeyFn))
	},
	func(b *InMemoryBackend) {
		b.integrationResourceProps = store.Register(
			b.registry,
			"integrationResourceProps",
			store.New(integrationResourcePropKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.integrationTableProps = store.Register(
			b.registry,
			"integrationTableProps",
			store.New(integrationTablePropKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.mlTaskRuns = store.Register(b.registry, "mlTaskRuns", store.New(mlTaskRunEntryKeyFn))
	},
	func(b *InMemoryBackend) {
		b.dqStatisticAnnotations = store.Register(
			b.registry,
			"dqStatisticAnnotations",
			store.New(dqStatisticAnnotationKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.customConnectionTypes = store.Register(
			b.registry,
			"customConnectionTypes",
			store.New(customConnectionTypeKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.glossaries = store.Register(b.registry, "glossaries", store.New(glossaryKeyFn))
	},
	func(b *InMemoryBackend) {
		b.glossaryTerms = store.Register(b.registry, "glossaryTerms", store.New(glossaryTermKeyFn))
	},
	func(b *InMemoryBackend) {
		b.assetTypes = store.Register(b.registry, "assetTypes", store.New(assetTypeKeyFn))
	},
	func(b *InMemoryBackend) {
		b.assets = store.Register(b.registry, "assets", store.New(assetKeyFn))
	},
	func(b *InMemoryBackend) {
		b.formTypes = store.Register(b.registry, "formTypes", store.New(formTypeKeyFn))
	},
}
