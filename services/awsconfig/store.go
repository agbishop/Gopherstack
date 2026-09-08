package awsconfig

import (
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	recorderStatusActive  = "ACTIVE"
	recorderStatusPending = "PENDING"
	recorderStatusSuccess = "SUCCESS"
)

// InMemoryBackend is the in-memory store for AWS Config resources.
//
// Phase 3.3 datalayer conversion: every map[string]*T resource collection is
// now a *store.Table[T] registered on b.registry (see store_setup.go's
// registerAllTables), which collapses Reset/Snapshot/Restore to one
// b.registry call each instead of one hand-written block per map. Fields
// whose value is not a *T (a scalar, a slice, or a nested map) have no
// natural store.Table key and are left as plain maps -- see each field's own
// comment for why, and persistence.go's doc comment for the persistence
// audit of each.
type InMemoryBackend struct {
	registry  *store.Registry
	recorders *store.Table[ConfigurationRecorder]
	// recordersByServicePrincipal indexes recorders by ServicePrincipal, used
	// by PutThirdPartyServiceLinkedConfigurationRecorder to find the existing
	// third-party service-linked recorder (if any) owned by a given service
	// principal. Customer-managed recorders and AWS-native service-linked
	// recorders (created via PutServiceLinkedConfigurationRecorder, which
	// tracks its own link via serviceLinkedRecorders below instead) group
	// under the empty-string key and are never looked up through this index.
	recordersByServicePrincipal *store.Index[ConfigurationRecorder]
	// serviceLinkedRecorders tracks servicePrincipal -> recorder-name links for
	// service-linked recorders (see ServiceLinkedRecorderLink's doc comment).
	serviceLinkedRecorders *store.Table[ServiceLinkedRecorderLink]
	channels               *store.Table[DeliveryChannel]
	// connectors holds connections between AWS Config and third-party cloud
	// service providers (PutConnector/GetConnector/ListConnectors/DeleteConnector).
	connectors       *store.Table[Connector]
	aggregationAuths *store.Table[AggregationAuthorization]
	configRules      *store.Table[ConfigRule]
	// ruleEvaluations is a scalar-valued map (rule name → rolled-up compliance
	// type) -- no *T for store.Table to key on -- so it stays a plain map.
	ruleEvaluations map[string]string
	// ruleResourceEvals holds per-(rule, resource) evaluation results, flattened
	// to a single store.Table[StoredEvaluation] keyed by
	// "<ruleName>|<resourceType>\x1f<resourceID>" (storedEvaluationKeyFn),
	// mirroring codecommit's nested-per-parent conversion. Two secondary
	// indexes replace the two access patterns the nested map used to answer
	// directly: ruleResourceEvalsByRule ("all evaluations for rule X", used by
	// evaluateManagedRuleLocked/recomputeRollupLocked/GetComplianceDetailsByConfigRule)
	// and ruleResourceEvalsByResource ("all evaluations for resource Y across
	// every rule", used by GetComplianceDetailsByResource).
	ruleResourceEvals           *store.Table[StoredEvaluation]
	ruleResourceEvalsByRule     *store.Index[StoredEvaluation]
	ruleResourceEvalsByResource *store.Index[StoredEvaluation]
	// resourceHistory is a slice-valued map (resourceKey → ordered history) --
	// no *T for store.Table to key on -- so it stays a plain map.
	resourceHistory map[string][]ResourceConfigItem
	// resourceEvaluations records StartResourceEvaluation runs keyed by
	// evaluation id (ResourceEvaluation.ResourceEvaluationID, a real field).
	resourceEvaluations *store.Table[ResourceEvaluation]
	aggregators         *store.Table[ConfigurationAggregator]
	conformancePacks    *store.Table[ConformancePack]
	// conformancePackRules tracks which config rules each conformance pack
	// deployed (see ConformancePackRuleLink's doc comment), keyed by
	// "<packName>|<ruleName>" with a "byPack" index answering "every rule this
	// pack deployed" (used by the compliance family and cascade delete).
	conformancePackRules       *store.Table[ConformancePackRuleLink]
	conformancePackRulesByPack *store.Index[ConformancePackRuleLink]
	orgConfigRules             *store.Table[OrganizationConfigRule]
	orgConformancePacks        *store.Table[OrganizationConformancePack]
	storedQueries              *store.Table[StoredQuery]
	// resourceTags is a slice-valued map (ARN → tags) -- left as a plain map.
	resourceTags       map[string][]Tag
	retentionConfigs   *store.Table[RetentionConfiguration]
	remediationConfigs *store.Table[RemediationConfiguration]
	// remediationExecutions tracks StartRemediationExecution runs, keyed by
	// "<ruleName>|<resourceType>\x1f<resourceID>" with a "byRule" index
	// answering "every execution for this rule" (used by
	// DescribeRemediationExecutionStatus), mirroring ruleResourceEvals'
	// composite-key pattern.
	remediationExecutions       *store.Table[RemediationExecutionStatusEntry]
	remediationExecutionsByRule *store.Index[RemediationExecutionStatusEntry]
	// remediationExceptions is a slice-valued map (rule name → exceptions) --
	// left as a plain map.
	remediationExceptions map[string][]RemediationException
	// resourceConfigs holds discovered resource configuration items,
	// flattened to a single store.Table[ResourceConfigItem] keyed by
	// "<resourceType>|<resourceID>" (resourceConfigItemKeyFn) since
	// ResourceConfigItem already carries real ResourceType/ResourceID
	// fields -- no hidden field needed, unlike codecommit's dirty tables.
	// resourceConfigsByType replaces the old map[string]map[string]*T's
	// outer-map lookup for "all resources of type X" (ListDiscoveredResources).
	resourceConfigs       *store.Table[ResourceConfigItem]
	resourceConfigsByType *store.Index[ResourceConfigItem]
	// customRulePolicies/orgCustomRulePolicies are scalar-valued maps (rule
	// name → policy text) -- left as plain maps.
	customRulePolicies     map[string]string
	orgCustomRulePolicies  map[string]string
	mu                     *lockmetrics.RWMutex
	accountID              string
	region                 string
	ruleCounter            int
	orgRuleCounter         int
	conformancePackCounter int
	aggregatorCounter      int
	resourceEvalCounter    int
	captureCounter         int
}

// NewInMemoryBackend creates a new InMemoryBackend.
func NewInMemoryBackend() *InMemoryBackend {
	return NewInMemoryBackendWithMeta("123456789012", "us-east-1")
}

// NewInMemoryBackendWithMeta creates a new InMemoryBackend with account and region context.
func NewInMemoryBackendWithMeta(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:              store.NewRegistry(),
		ruleEvaluations:       make(map[string]string),
		resourceHistory:       make(map[string][]ResourceConfigItem),
		resourceTags:          make(map[string][]Tag),
		remediationExceptions: make(map[string][]RemediationException),
		customRulePolicies:    make(map[string]string),
		orgCustomRulePolicies: make(map[string]string),
		mu:                    lockmetrics.New("awsconfig"),
		accountID:             accountID,
		region:                region,
	}

	registerAllTables(b)

	return b
}

// Reset clears all in-memory state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.ruleEvaluations = make(map[string]string)
	b.resourceHistory = make(map[string][]ResourceConfigItem)
	b.resourceEvalCounter = 0
	b.captureCounter = 0
	b.ruleCounter = 0
	b.orgRuleCounter = 0
	b.conformancePackCounter = 0
	b.aggregatorCounter = 0
	b.resourceTags = make(map[string][]Tag)
	b.remediationExceptions = make(map[string][]RemediationException)
	b.customRulePolicies = make(map[string]string)
	b.orgCustomRulePolicies = make(map[string]string)
}
