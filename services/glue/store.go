package glue

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = awserr.New("EntityNotFoundException", awserr.ErrNotFound)

// ErrAlreadyExists is returned when a resource already exists.
var ErrAlreadyExists = awserr.New("AlreadyExistsException", awserr.ErrAlreadyExists)

// ErrConflict is returned when an operation is rejected because of the
// current state of a related resource, mirroring AWS's ConflictException.
// Confirmed as the documented error (not EntityNotFoundException or
// InvalidInputException) for DeleteGlossary ("a glossary cannot be deleted if
// it still contains glossary terms") and DeleteFormType ("a form type cannot
// be deleted if it is still referenced by an asset type") in
// aws-sdk-go-v2/service/glue/deserializers.go's
// awsAwsjson11_deserializeOpErrorDeleteGlossary/DeleteFormType error switches.
var ErrConflict = awserr.New("ConflictException", awserr.ErrConflict)

// ErrConcurrentRunsExceeded is returned by StartJobRun/StartWorkflowRun when
// the job/workflow's MaxConcurrentRuns limit is already reached, mirroring
// AWS's ConcurrentRunsExceededException (confirmed in
// aws-sdk-go-v2/service/glue/deserializers.go's
// awsAwsjson11_deserializeOpErrorStartJobRun/StartWorkflowRun error switches).
var ErrConcurrentRunsExceeded = awserr.New("ConcurrentRunsExceededException", awserr.ErrConflict)

// ErrValidation is returned when input validation fails.
//
// Glue's per-operation error models (aws-sdk-go-v2/service/glue deserializers.go)
// list InvalidInputException — not ValidationException — as the hand-validation
// error for the overwhelming majority of Create/Update/Delete operations (e.g.
// CreateDatabase, CreateTable, CreateJob, CreateCrawler, CreateTrigger,
// CreateBlueprint, CreateCustomEntityType, CreateUsageProfile, tag validation).
// A handful of newer operations (e.g. DeleteConnectionType) do document
// ValidationException instead, but since this sentinel is shared across every
// hand-rolled validation check in the backend, InvalidInputException is the more
// accurate default.
var ErrValidation = awserr.New("InvalidInputException", awserr.ErrInvalidParameter)

// ErrResourceNumberLimitExceeded is returned when a create call would push a
// resource kind past its documented account quota, mirroring AWS's
// ResourceNumberLimitExceededException (confirmed in
// aws-sdk-go-v2/service/glue/deserializers.go's
// awsAwsjson11_deserializeOpErrorCreateDevEndpoint error switch; the quota
// value itself is AWS's published default, docs.aws.amazon.com/general/latest/gr/glue.html
// "Max development endpoint per account: 25").
var ErrResourceNumberLimitExceeded = awserr.New("ResourceNumberLimitExceededException", awserr.ErrInvalidParameter)

// maxDevEndpointsPerAccount is AWS's documented default quota (adjustable in
// real AWS via Service Quotas, fixed here since this backend has no per-account
// quota-adjustment concept).
const maxDevEndpointsPerAccount = 25

// glueARNParts is the number of colon-separated parts in a Glue ARN.
// Format: arn:aws:glue:{region}:{account}:{resourceType}/{name}.

const glueARNParts = 6

const errEntityNotFoundCode = "EntityNotFoundException"

const stateRunning = "RUNNING"

const stateStarting = "STARTING"

const stateReady = "READY"

const stateStopping = "STOPPING"

const stateStopped = "STOPPED"

const stateSucceeded = "SUCCEEDED"

const stateFailed = "FAILED"

const stateTimeout = "TIMEOUT"

const stateError = "ERROR"

const stateWaiting = "WAITING"

const stateCompleted = "COMPLETED"

const reconcilerTickDivisor = 5

const stateAvailable = "AVAILABLE"

const stateDeleting = "DELETING"

const stateActive = "ACTIVE"

const stateScheduled = "SCHEDULED"

const stateNotScheduled = "NOT_SCHEDULED"

const sortDirectionDescending = "DESCENDING"

// ExportSetting values for {Get,Put}DataCatalogExportConfiguration. Status
// reuses these two values rather than the SDK's richer ExportStatus enum,
// since this backend has no async export pipeline to simulate -- see catalogs.go.
const exportSettingEnabled = "ENABLED"

const exportSettingDisabled = "DISABLED"

// maxNameLen is the maximum length (in characters) for Glue resource names.
// AWS enforces a 255-character limit for database, table, crawler, and job names.
const maxNameLen = 255

// maxTagsPerResource is the maximum number of tags allowed per Glue resource.
const maxTagsPerResource = 50

// maxTagKeyLen is the maximum byte length of a tag key.
const maxTagKeyLen = 128

// maxTagValueLen is the maximum byte length of a tag value.
const maxTagValueLen = 256

// validateTags checks that tags conform to AWS Glue limits:
// max 50 tags, key 1-128 chars, value 0-256 chars.
func validateTags(tags map[string]string) error {
	if len(tags) > maxTagsPerResource {
		return fmt.Errorf("%w: too many tags: maximum is %d", ErrValidation, maxTagsPerResource)
	}
	for k, v := range tags {
		if len(k) == 0 || len(k) > maxTagKeyLen {
			return fmt.Errorf("%w: tag key length must be 1-%d", ErrValidation, maxTagKeyLen)
		}
		if len(v) > maxTagValueLen {
			return fmt.Errorf("%w: tag value length must be 0-%d", ErrValidation, maxTagValueLen)
		}
	}

	return nil
}

// InMemoryBackend stores Glue state in memory. Most resource collections are
// *store.Table[T], registered once on b.registry via registerAllTables (see
// store_setup.go); a handful remain plain maps because their key is not a
// pure function of the stored value's own fields, or because they hold a
// one-to-many history/list rather than a single value per key -- see the
// comment above registerAllTables in store_setup.go for the full list and
// rationale.
type InMemoryBackend struct {
	databases                 *store.Table[Database]
	tables                    *store.Table[Table]
	crawlers                  *store.Table[Crawler]
	jobs                      *store.Table[Job]
	partitions                *store.Table[Partition]
	partitionsByTable         *store.Index[Partition]
	partitionIndexes          map[string]*PartitionIndex // key: "databaseName|tableName|indexName"
	tableVersions             *store.Table[TableVersion]
	connections               *store.Table[Connection]
	blueprints                *store.Table[Blueprint]
	customEntityTypes         *store.Table[CustomEntityType]
	dataQualityResult         *store.Table[DataQualityResult]
	devEndpoints              *store.Table[DevEndpoint]
	jobRuns                   map[string][]*JobRun // key: jobName
	jobBookmarks              *store.Table[JobBookmark]
	dataQualityRulesets       *store.Table[DataQualityRuleset]
	dataQualityEvalRuns       *store.Table[DataQualityEvaluationRun]
	triggers                  *store.Table[Trigger]
	workflows                 *store.Table[Workflow]
	workflowRuns              map[string][]*WorkflowRun // key: workflowName
	classifiers               *store.Table[Classifier]
	registries                *store.Table[Registry]
	schemas                   *store.Table[Schema]
	schemaVersions            map[string][]*SchemaVersion // key: schemaARN
	udfs                      *store.Table[UserDefinedFunction]
	securityConfigs           *store.Table[SecurityConfiguration]
	sessions                  *store.Table[Session]
	sessionStatements         map[string][]*Statement // key: sessionID
	tableOptimizers           *store.Table[tableOptimizerRecord]
	tableColumnStats          map[string]*ColumnStatistics    // key: "dbName|tableName|colName"
	partitionColumnStats      map[string]*ColumnStatistics    // key: partKey+"|"+colName
	resourcePolicies          map[string]*resourcePolicyEntry // key: resourceARN or "__global__"
	mlTransforms              *store.Table[MLTransform]
	catalogs                  *store.Table[CatalogEntry]
	catalogEncryptionSettings map[string]*DataCatalogEncryptionSettings // key: catalogID or accountID
	usageProfiles             *store.Table[UsageProfile]
	blueprintRuns             *store.Table[BlueprintRun]
	dqRecommendationRuns      *store.Table[DQRuleRecommendationRun]
	columnStatTaskSettings    *store.Table[ColumnStatisticsTaskSettings]
	columnStatTaskRuns        *store.Table[ColumnStatisticsTaskRun]
	materializedViewRuns      *store.Table[MaterializedViewRefreshRun]
	integrations              *store.Table[Integration]
	integrationResourceProps  *store.Table[IntegrationResourceProperty]
	integrationTableProps     *store.Table[IntegrationTableProperties]
	mlTaskRuns                *store.Table[MLTaskRun]
	catalogImports            map[string]*CatalogImportStatus // key: catalogID or accountID
	schemaVersionMetadata     map[string]map[string]string    // key: schemaVersionID → key → value
	crawlHistory              map[string][]*CrawlHistoryEntry // key: crawlerName
	dqStatisticAnnotations    *store.Table[StatisticAnnotation]
	glueIdentityCenterConfig  *IdentityCenterConfig
	dataCatalogExportConfig   *DataCatalogExportConfiguration
	registry                  *store.Registry
	mu                        *lockmetrics.RWMutex

	// lifecycle reconciler timers
	jobRunReadyAt      map[string]map[string]time.Time // jobName → runID → readyAt for STARTING→RUNNING
	jobRunDoneAt       map[string]map[string]time.Time // jobName → runID → doneAt for RUNNING→SUCCEEDED
	jobRunTimeoutAt    map[string]map[string]time.Time // jobName → runID → timeoutAt for RUNNING→TIMEOUT
	jobRunStopAt       map[string]map[string]time.Time // jobName → runID → stopAt for STOPPING→STOPPED
	crawlerReadyAt     map[string]time.Time            // crawlerName → readyAt for RUNNING→READY
	integrationReadyAt map[string]time.Time            // integrationName → readyAt for CREATING→ACTIVE

	// connection-type registry (custom types registered via RegisterConnectionType).
	customConnectionTypes *store.Table[ConnectionTypeInfo]

	// Data Catalog business-glossary / asset-catalog resources (see
	// glossaries.go, assets.go, forms.go).
	glossaries    *store.Table[Glossary]
	glossaryTerms *store.Table[GlossaryTerm]
	assetTypes    *store.Table[AssetType]
	assets        *store.Table[Asset]
	formTypes     *store.Table[FormType]
	// iterableFormItems holds items within an asset's iterable forms (e.g. a
	// table asset's "columns"), keyed assetID -> iterableFormName -> itemID.
	// Real Glue has no operation that explicitly creates these -- the only
	// documented write path is PutAttachment targeting an item via
	// ItemIdentifier+IterableFormName (see PutAttachment in assets.go) -- so
	// this cannot be a store.Table (its key is not a pure function of a
	// single value's own fields; it is a nested per-asset, per-form
	// collection), matching the rationale documented in store_setup.go for
	// the other raw-map fields above.
	iterableFormItems iterableFormItemsMap

	// reconcileStop signals the managed reconciler goroutine to exit. See the
	// non-pointer reconciler bookkeeping fields at the end of the struct.
	reconcileStop chan struct{}

	accountID string
	region    string

	// Managed reconciler lifecycle bookkeeping. The reconciler is started by the
	// service framework via StartWorker (BackgroundWorker) and stopped by Shutdown
	// (Shutdowner), replacing the previous unmanaged goroutine that leaked because
	// nothing ever called Close. reconcileMu guards this bookkeeping; b.mu continues
	// to guard resource state. These pointer-free fields are placed last to keep the
	// GC pointer-scan region minimal (govet fieldalignment).
	reconcileWG       sync.WaitGroup
	reconcileMu       sync.Mutex
	reconcileInterval time.Duration
	reconcileOn       bool
}

// NewInMemoryBackend creates a new in-memory Glue backend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		partitionIndexes:          make(map[string]*PartitionIndex),
		jobRuns:                   make(map[string][]*JobRun),
		workflowRuns:              make(map[string][]*WorkflowRun),
		schemaVersions:            make(map[string][]*SchemaVersion),
		sessionStatements:         make(map[string][]*Statement),
		tableColumnStats:          make(map[string]*ColumnStatistics),
		partitionColumnStats:      make(map[string]*ColumnStatistics),
		resourcePolicies:          make(map[string]*resourcePolicyEntry),
		catalogEncryptionSettings: make(map[string]*DataCatalogEncryptionSettings),
		catalogImports:            make(map[string]*CatalogImportStatus),
		schemaVersionMetadata:     make(map[string]map[string]string),
		crawlHistory:              make(map[string][]*CrawlHistoryEntry),
		iterableFormItems:         make(iterableFormItemsMap),
		registry:                  store.NewRegistry(),
		mu:                        lockmetrics.New("glue"),
		accountID:                 accountID,
		region:                    region,
		jobRunReadyAt:             make(map[string]map[string]time.Time),
		jobRunDoneAt:              make(map[string]map[string]time.Time),
		jobRunTimeoutAt:           make(map[string]map[string]time.Time),
		jobRunStopAt:              make(map[string]map[string]time.Time),
		crawlerReadyAt:            make(map[string]time.Time),
		integrationReadyAt:        make(map[string]time.Time),
	}

	registerAllTables(b)

	return b
}

// Reset clears all backend state, returning it to the initial empty state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()

	b.partitionIndexes = make(map[string]*PartitionIndex)
	b.jobRuns = make(map[string][]*JobRun)
	b.workflowRuns = make(map[string][]*WorkflowRun)
	b.schemaVersions = make(map[string][]*SchemaVersion)
	b.sessionStatements = make(map[string][]*Statement)
	b.tableColumnStats = make(map[string]*ColumnStatistics)
	b.partitionColumnStats = make(map[string]*ColumnStatistics)
	b.resourcePolicies = make(map[string]*resourcePolicyEntry)
	b.catalogEncryptionSettings = make(map[string]*DataCatalogEncryptionSettings)
	b.catalogImports = make(map[string]*CatalogImportStatus)
	b.schemaVersionMetadata = make(map[string]map[string]string)
	b.resetStubFixState()

	b.resetLifecycleStateLocked()
}

// resetStubFixState clears the raw map backing the de-stubbed crawl-history
// feature. Kept separate from Reset to stay under the funlen limit. Must be
// called with b.mu held.
func (b *InMemoryBackend) resetStubFixState() {
	b.crawlHistory = make(map[string][]*CrawlHistoryEntry)
	b.iterableFormItems = make(iterableFormItemsMap)
}

// resetLifecycleStateLocked clears identity-center config and pending
// lifecycle transition timers. Clearing the timers ensures a reset never
// resurrects a crawler/job-run state change scheduled against now-deleted
// resources. Must be called with b.mu held.
func (b *InMemoryBackend) resetLifecycleStateLocked() {
	b.glueIdentityCenterConfig = nil
	b.dataCatalogExportConfig = nil
	b.jobRunReadyAt = make(map[string]map[string]time.Time)
	b.jobRunDoneAt = make(map[string]map[string]time.Time)
	b.jobRunTimeoutAt = make(map[string]map[string]time.Time)
	b.jobRunStopAt = make(map[string]map[string]time.Time)
	b.crawlerReadyAt = make(map[string]time.Time)
	b.integrationReadyAt = make(map[string]time.Time)
}

// Region returns the backend region.
func (b *InMemoryBackend) Region() string { return b.region }

// AccountID returns the backend account ID.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// glueResourceName extracts the resource name from a Glue ARN for a given resource type.
// Glue ARNs have the format: arn:aws:glue:{region}:{account}:{resourceType}/{name}.
// This allows matching ARNs even when the account ID differs (e.g. empty vs 000000000000).
func glueResourceName(resourceARN, resourceType string) string {
	// Split into exactly glueARNParts parts: arn, aws, glue, region, account, resource
	parts := strings.SplitN(resourceARN, ":", glueARNParts)
	if len(parts) != glueARNParts {
		return ""
	}

	prefix := resourceType + "/"
	if !strings.HasPrefix(parts[5], prefix) {
		return ""
	}

	return parts[5][len(prefix):]
}

// glueServiceName is the canonical service name used in import attribution.
const glueServiceName = "Glue"
