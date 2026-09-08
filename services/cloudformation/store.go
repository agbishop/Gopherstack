package cloudformation

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

type StorageBackend interface {
	CreateStack(
		ctx context.Context,
		name, templateBody string,
		params []Parameter,
		opts StackOptions,
	) (*Stack, error)
	UpdateStack(
		ctx context.Context,
		nameOrID, templateBody string,
		params []Parameter,
		opts StackOptions,
	) (*Stack, error)
	DeleteStack(ctx context.Context, nameOrID string) error
	DescribeStack(nameOrID string) (*Stack, error)
	ListStacks(statusFilter []string, nextToken string) (page.Page[StackSummary], error)
	DescribeStackEvents(nameOrID, nextToken string) (page.Page[StackEvent], error)
	DescribeStackResource(nameOrID, logicalID string) (*StackResource, error)
	ListStackResources(nameOrID, nextToken string) (page.Page[StackResourceSummary], error)
	DescribeStackResources(nameOrID string) ([]StackResource, error)
	ListExports(nextToken string) (page.Page[Export], error)
	ListImports(exportName, nextToken string) (page.Page[string], error)
	CreateChangeSet(
		ctx context.Context,
		stackName, changeSetName, templateBody, description string,
		params []Parameter,
		capabilities []string,
		tags []Tag,
	) (*ChangeSet, error)
	DescribeChangeSet(stackName, changeSetName string) (*ChangeSet, error)
	ExecuteChangeSet(ctx context.Context, stackName, changeSetName string) error
	DeleteChangeSet(stackName, changeSetName string) error
	ListChangeSets(stackName, nextToken string) (page.Page[ChangeSetSummary], error)
	GetTemplate(nameOrID string) (string, error)
	ListAll() []*Stack
	// Drift detection
	DetectStackDrift(nameOrID string) (string, error)
	DetectStackResourceDrift(nameOrID, logicalID string) (*StackResourceDrift, error)
	DescribeStackDriftDetectionStatus(detectionID string) (*DriftDetectionStatus, error)
	DescribeStackResourceDrifts(nameOrID string) ([]StackResourceDrift, error)
	// Stack policy
	SetStackPolicy(nameOrID, policy string) error
	GetStackPolicy(nameOrID string) (string, error)
	// Template analysis
	GetTemplateSummary(templateBody, stackName string) (*TemplateSummary, error)
	EstimateTemplateCost(templateBody string, params []Parameter) (string, error)
	// Stack management
	ContinueUpdateRollback(ctx context.Context, nameOrID string) error
	CancelUpdateStack(ctx context.Context, nameOrID string) error
	DescribeAccountLimits() []AccountLimit
	// Stack Sets
	CreateStackSet(name, description, templateBody string, opts StackSetOptions) (*StackSet, error)
	UpdateStackSet(name, description, templateBody string, opts StackSetOptions) (*StackSet, string, error)
	DeleteStackSet(name string) error
	DescribeStackSet(name string) (*StackSet, error)
	StackSetRegions(name string) []string
	ListStackSets(nextToken, status string) (page.Page[StackSetSummary], error)
	CreateStackInstances(
		ctx context.Context,
		stackSetName string,
		accounts, ouIDs, regions []string,
	) (string, error)
	DeleteStackInstances(
		ctx context.Context,
		stackSetName string,
		accounts, ouIDs, regions []string,
	) (string, error)
	UpdateStackInstances(stackSetName string, accounts, ouIDs, regions []string) (string, error)
	ListStackInstances(
		stackSetName, nextToken string, filter ListStackInstancesFilter,
	) (page.Page[StackInstance], error)
	DescribeStackInstance(stackSetName, account, region string) (*StackInstance, error)
	DetectStackSetDrift(stackSetName string) (string, error)
	ListStackSetOperations(
		stackSetName, nextToken string,
	) (page.Page[StackSetOperationSummary], error)
	DescribeStackSetOperation(stackSetName, operationID string) (*StackSetOperation, error)
	StopStackSetOperation(stackSetName, operationID string) error
	ListStackSetOperationResults(
		stackSetName, operationID, nextToken string,
	) ([]StackSetOperationResult, error)
	ListStackSetAutoDeploymentTargets(stackSetName string) ([]AutoDeploymentTarget, error)
	ImportStacksToStackSet(stackSetName string, stackIDs []string) (string, error)
	ListStackInstanceResourceDrifts(
		stackSetName, operationID, account, region string,
	) ([]StackResourceDrift, error)
	// Generated templates
	CreateGeneratedTemplate(name string, resources []string) (*GeneratedTemplate, error)
	UpdateGeneratedTemplate(id, name string) (*GeneratedTemplate, error)
	DeleteGeneratedTemplate(id string) error
	DescribeGeneratedTemplate(id string) (*GeneratedTemplate, error)
	GetGeneratedTemplate(id string) (string, error)
	ListGeneratedTemplates(nextToken string) (page.Page[GeneratedTemplate], error)
	// Resource scans
	StartResourceScan() (string, error)
	DescribeResourceScan(scanID string) (*ResourceScan, error)
	ListResourceScans(nextToken string) (page.Page[ResourceScan], error)
	ListResourceScanResources(scanID, nextToken string) ([]ScannedResource, error)
	ListResourceScanRelatedResources(scanID string, resources []string) ([]string, error)
	// Type management
	ActivateType(typeName, typeArn string) (string, error)
	DeactivateType(typeName, typeArn string) error
	RegisterType(typeName, schemaHandlerPackage string) (string, error)
	DeregisterType(typeName, typeArn, versionID string) error
	PublishType(typeName string) (string, error)
	SetTypeDefaultVersion(arn, version string) error
	SetTypeConfiguration(typeName, configuration string) (string, error)
	BatchDescribeTypeConfigurations(
		identifiers []TypeConfigurationIdentifier,
	) ([]TypeConfigurationDetail, []BatchDescribeTypeConfigurationsError, []TypeConfigurationIdentifier)
	ListTypes(nextToken string) ([]TypeSummary, error)
	ListTypeVersions(typeName, deprecatedStatus string) ([]string, error)
	ListTypeRegistrations(typeName, nextToken string) ([]string, error)
	DescribeTypeRegistration(registrationToken string) (string, error)
	DescribeType(typeName, arn, versionID string) (*TypeDetails, error)
	TestType(typeName, arn string) (string, error)
	RegisterPublisher(connectionArn string) (string, error)
	DescribePublisher(publisherID string) (string, error)
	// Stack refactor
	CreateStackRefactor(
		description string,
		resourceMappings []ResourceMapping,
		enableStackCreation bool,
	) (string, error)
	DescribeStackRefactor(stackRefactorID string) (*StackRefactor, error)
	ExecuteStackRefactor(stackRefactorID string) error
	ListStackRefactors(nextToken string) ([]StackRefactorSummary, error)
	ListStackRefactorActions(stackRefactorID string) ([]StackRefactorAction, error)
	// Org access
	ActivateOrganizationsAccess() error
	DeactivateOrganizationsAccess() error
	DescribeOrganizationsAccess() (string, error)
	// Misc
	SignalResource(stackName, logicalID, uniqueID, status string) error
	RollbackStack(ctx context.Context, stackName string) (*Stack, error)
	RecordHandlerProgress(bearerToken, operationStatus string) error
	GetHookResult(hookResultToken string) (string, error)
	ListHookResults(hookResultToken, nextToken string) ([]HookResult, error)
	DescribeChangeSetHooks(stackName, changeSetName string) ([]ChangeSetHook, error)
	DescribeEvents(stackName, nextToken string, failedOnly bool) (page.Page[StackEvent], error)
	UpdateTerminationProtection(stackName string, enable bool) error
	ValidateTemplate(templateBody string) (*TemplateSummary, error)
}

// InMemoryBackend is a concurrency-safe in-memory CloudFormation backend.
type InMemoryBackend struct {
	// registry lets Reset/Snapshot/Restore collapse the tables below to one
	// call each instead of hand-rolled per-map boilerplate. See store_setup.go
	// for the registrations and for why the maps further down are NOT
	// registered (nested or slice-valued, which store.Table cannot represent).
	registry            *store.Registry
	stacks              *store.Table[Stack]
	exports             *store.Table[Export]
	driftDetections     *store.Table[DriftDetectionStatus]
	stackSets           *store.Table[StackSet]
	generatedTemplates  *store.Table[GeneratedTemplate]
	resourceScans       *store.Table[ResourceScan]
	typeRegistry        *store.Table[RegisteredType]
	typeRegistrations   *store.Table[TypeRegistrationRecord]
	publishers          *store.Table[Publisher]
	stackRefactors      *store.Table[StackRefactor]
	hookResults         *store.Table[HookResult]
	stackIDIndex        map[string]string // stackID (ARN) → stackName
	events              map[string][]StackEvent
	resources           map[string]map[string]*StackResource
	changeSets          map[string]map[string]*ChangeSet
	stackPolicies       map[string]string
	stackInstances      map[string][]StackInstance                      // stackSetName → instances
	stackSetOperations  map[string]map[string]*StackSetOperation        // stackSetName → operationID → op
	typeConfigs         map[string]string                               // typeName → config json
	handlerProgress     map[string]string                               // bearerToken → status
	signals             map[string][]SignalRecord                       // stackName+logicalID → records
	stackSetOpResults   map[string]map[string][]StackSetOperationResult // stackSetName → opID → results
	typeVersions        map[string][]*RegisteredTypeVersion             // typeArn → versions
	resourceScanItems   map[string][]ScannedResource                    // scanID → scanned resources
	resourceDriftStatus map[string]map[string]string                    // stackID → logicalID → drift status
	resourceDriftDetail map[string]map[string]StackResourceDrift        // stackID → logicalID → drift detail
	driftByStackID      map[string][]string                             // stackID → detectionIDs (reverse index)
	creator             *ResourceCreator
	resolver            DynamicRefResolver
	orgDirectory        OrganizationsDirectory
	mu                  *lockmetrics.RWMutex
	accountID           string
	region              string
	orgAccessEnabled    bool
}

const (
	MockAccountID = config.DefaultAccountID
	MockRegion    = config.DefaultRegion

	cfnStackType                   = "AWS::CloudFormation::Stack"
	statusCreateInProgress         = "CREATE_IN_PROGRESS"
	statusCreateComplete           = "CREATE_COMPLETE"
	statusCreateFailed             = "CREATE_FAILED"
	statusUpdateInProgress         = "UPDATE_IN_PROGRESS"
	statusUpdateComplete           = "UPDATE_COMPLETE"
	statusUpdateFailed             = "UPDATE_FAILED"
	statusUpdateRollbackInProgress = "UPDATE_ROLLBACK_IN_PROGRESS"
	statusUpdateRollbackComplete   = "UPDATE_ROLLBACK_COMPLETE"
	statusUpdateRollbackFailed     = "UPDATE_ROLLBACK_FAILED"
	statusDeleteInProgress         = "DELETE_IN_PROGRESS"
	statusDeleteComplete           = "DELETE_COMPLETE"
	statusDeleteFailed             = "DELETE_FAILED"
	statusRollbackInProgress       = "ROLLBACK_IN_PROGRESS"
	statusRollbackComplete         = "ROLLBACK_COMPLETE"
	statusRollbackFailed           = "ROLLBACK_FAILED"
	reasonUserInitiated            = "User Initiated"
)

// NewInMemoryBackend creates a new empty CloudFormation backend.
func NewInMemoryBackend() *InMemoryBackend {
	return NewInMemoryBackendWithConfig(MockAccountID, MockRegion, nil)
}

// NewInMemoryBackendWithConfig creates a new backend with the given config and resource creator.
func NewInMemoryBackendWithConfig(
	accountID, region string,
	creator *ResourceCreator,
) *InMemoryBackend {
	var resolver DynamicRefResolver
	if creator != nil {
		resolver = NewDynamicRefResolver(creator.backends)
	}

	b := &InMemoryBackend{
		registry:            store.NewRegistry(),
		stackIDIndex:        make(map[string]string),
		events:              make(map[string][]StackEvent),
		resources:           make(map[string]map[string]*StackResource),
		changeSets:          make(map[string]map[string]*ChangeSet),
		stackPolicies:       make(map[string]string),
		stackInstances:      make(map[string][]StackInstance),
		stackSetOperations:  make(map[string]map[string]*StackSetOperation),
		typeConfigs:         make(map[string]string),
		handlerProgress:     make(map[string]string),
		signals:             make(map[string][]SignalRecord),
		stackSetOpResults:   make(map[string]map[string][]StackSetOperationResult),
		typeVersions:        make(map[string][]*RegisteredTypeVersion),
		resourceScanItems:   make(map[string][]ScannedResource),
		resourceDriftStatus: make(map[string]map[string]string),
		resourceDriftDetail: make(map[string]map[string]StackResourceDrift),
		driftByStackID:      make(map[string][]string),
		creator:             creator,
		resolver:            resolver,
		accountID:           accountID,
		region:              region,
		mu:                  lockmetrics.New("cloudformation"),
	}

	registerAllTables(b)

	// Wire the backend as the NestedStackCreator so nested stacks can be provisioned.
	if creator != nil {
		creator.WithNestedStackCreator(b)
	}

	return b
}
