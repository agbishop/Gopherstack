package ssm

import (
	"context"
	"crypto/cipher"
	"strconv"
	"sync"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	documentFormatJSON = "JSON"
	statusActive       = "Active"
)
const (
	StringType        = "String"
	StringListType    = "StringList"
	SecureStringType  = "SecureString"
	maxHistoryResults = 50
	// defaultCommandExpirySecs is the default TTL for SSM commands in seconds (1 hour).
	// AWS SSM commands expire after 1 hour by default.
	defaultCommandExpirySecs = 3600
	// maxHistoryCap is the maximum number of history entries retained per parameter.
	// Older entries beyond this cap are evicted to prevent unbounded growth.
	maxHistoryCap = 100
	// maxDocumentVersionCap is the maximum number of versions retained per document.
	// Matches the AWS-side limit and prevents unbounded growth via repeated UpdateDocument.
	maxDocumentVersionCap = 1000
	// resourceTypeParameter is the SSM resource type for parameters.
	resourceTypeParameter = "Parameter"
)

// KMSEncryptor provides symmetric encrypt/decrypt for SecureString parameters.
// Implemented by an adapter wrapping the KMS backend.
type KMSEncryptor interface {
	// EncryptSSM encrypts plaintext using the given KMS key and returns ciphertext bytes.
	EncryptSSM(keyID string, plaintext []byte) ([]byte, error)
	// DecryptSSM decrypts ciphertext and returns plaintext bytes.
	DecryptSSM(ciphertext []byte) ([]byte, error)
}

// InMemoryBackend implements StorageBackend using a concurrency-safe map.
type InMemoryBackend struct {
	kms                        KMSEncryptor
	gcm                        cipher.AEAD
	parameterPolicyNotifier    ParameterPolicyNotifier
	registry                   *store.Registry
	parameters                 map[string]*store.Table[Parameter]
	maintenanceWindows         map[string]*store.Table[MaintenanceWindow]
	maintenanceWindowTargets   map[string]*store.Table[MaintenanceWindowTarget]
	maintenanceWindowTasks     map[string]*store.Table[MaintenanceWindowTask]
	sessions                   map[string]*store.Table[Session]
	accessRequests             map[string]*store.Table[AccessRequest]
	patchGroupToBaseline       map[string]map[string]string
	tags                       map[string]map[string]*tags.Tags
	associations               map[string]*store.Table[Association]
	documentVersions           map[string]map[string][]DocumentVersion
	documentPermissions        map[string]map[string][]string
	documentSharedVersions     map[string]map[string]map[string]string
	commands                   map[string]*store.Table[Command]
	commandInvocations         map[string]map[string][]CommandInvocation
	history                    map[string]map[string][]ParameterHistory
	resourceDataSyncs          map[string]*store.Table[ResourceDataSync]
	documents                  map[string]*store.Table[Document]
	opsItems                   map[string]*store.Table[OpsItem]
	opsItemRelatedItems        map[string]map[string][]OpsItemRelatedItem
	opsMetadata                map[string]*store.Table[OpsMetadata]
	compliance                 map[string]map[string][]ComplianceItem
	activations                map[string]*store.Table[Activation]
	cloudConnectors            map[string]*store.Table[CloudConnector]
	inventory                  map[string]map[string][]InventoryItem
	associationExecutions      map[string]map[string][]AssociationExecution
	automationExecutions       map[string]*store.Table[AutomationExecution]
	serviceSettings            map[string]*store.Table[ServiceSetting]
	resourcePolicies           map[string]map[string][]*ResourcePolicy
	executionPreviews          map[string]*store.Table[ExecutionPreview]
	instancePatchStates        map[string]*store.Table[InstancePatchState]
	instancePatches            map[string]map[string][]PatchComplianceData
	instanceProperties         map[string]*store.Table[InstanceProperty]
	availablePatches           map[string][]Patch
	mu                         *lockmetrics.RWMutex
	inventoryDeletions         map[string][]InventoryDeletion
	miscResourceTags           map[string]map[string]map[string]string
	resourceIDToOpsMetadataArn map[string]map[string]string
	opsItemEvents              map[string][]OpsItemEventSummary
	parameterLabels            map[string]map[string]map[int64][]string
	associationExecTargets     map[string]map[string][]AssociationExecutionTarget
	patchBaselines             map[string]*store.Table[PatchBaseline]
	notifiedParameterPolicies  map[string]map[string]map[string]struct{}
	automationExecDelaySecs    float64
	commandExecDelaySecs       float64
	commandExpirySecs          float64
	tableMu                    sync.Mutex
}

// NewInMemoryBackend creates a new empty InMemoryBackend.
func NewInMemoryBackend() *InMemoryBackend {
	b := &InMemoryBackend{
		gcm:                        newInstanceGCM(),
		registry:                   store.NewRegistry(),
		parameters:                 make(map[string]*store.Table[Parameter]),
		history:                    make(map[string]map[string][]ParameterHistory),
		tags:                       make(map[string]map[string]*tags.Tags),
		documents:                  make(map[string]*store.Table[Document]),
		documentVersions:           make(map[string]map[string][]DocumentVersion),
		documentPermissions:        make(map[string]map[string][]string),
		documentSharedVersions:     make(map[string]map[string]map[string]string),
		commands:                   make(map[string]*store.Table[Command]),
		commandInvocations:         make(map[string]map[string][]CommandInvocation),
		activations:                make(map[string]*store.Table[Activation]),
		cloudConnectors:            make(map[string]*store.Table[CloudConnector]),
		associations:               make(map[string]*store.Table[Association]),
		maintenanceWindows:         make(map[string]*store.Table[MaintenanceWindow]),
		maintenanceWindowTargets:   make(map[string]*store.Table[MaintenanceWindowTarget]),
		maintenanceWindowTasks:     make(map[string]*store.Table[MaintenanceWindowTask]),
		sessions:                   make(map[string]*store.Table[Session]),
		accessRequests:             make(map[string]*store.Table[AccessRequest]),
		patchGroupToBaseline:       make(map[string]map[string]string),
		opsItems:                   make(map[string]*store.Table[OpsItem]),
		opsItemRelatedItems:        make(map[string]map[string][]OpsItemRelatedItem),
		opsMetadata:                make(map[string]*store.Table[OpsMetadata]),
		patchBaselines:             make(map[string]*store.Table[PatchBaseline]),
		inventory:                  make(map[string]map[string][]InventoryItem),
		compliance:                 make(map[string]map[string][]ComplianceItem),
		resourceDataSyncs:          make(map[string]*store.Table[ResourceDataSync]),
		parameterLabels:            make(map[string]map[string]map[int64][]string),
		automationExecutions:       make(map[string]*store.Table[AutomationExecution]),
		serviceSettings:            make(map[string]*store.Table[ServiceSetting]),
		resourcePolicies:           make(map[string]map[string][]*ResourcePolicy),
		executionPreviews:          make(map[string]*store.Table[ExecutionPreview]),
		instancePatchStates:        make(map[string]*store.Table[InstancePatchState]),
		instancePatches:            make(map[string]map[string][]PatchComplianceData),
		instanceProperties:         make(map[string]*store.Table[InstanceProperty]),
		availablePatches:           make(map[string][]Patch),
		commandExpirySecs:          defaultCommandExpirySecs,
		mu:                         lockmetrics.New("ssm"),
		resourceIDToOpsMetadataArn: make(map[string]map[string]string),
		miscResourceTags:           make(map[string]map[string]map[string]string),
		opsItemEvents:              make(map[string][]OpsItemEventSummary),
		associationExecutions:      make(map[string]map[string][]AssociationExecution),
		associationExecTargets:     make(map[string]map[string][]AssociationExecutionTarget),
		inventoryDeletions:         make(map[string][]InventoryDeletion),
		notifiedParameterPolicies:  make(map[string]map[string]map[string]struct{}),
	}

	b.registerDefaultDocuments(defaultRegion)

	return b
}

// WithKMS attaches a KMSEncryptor so SecureString parameters whose KeyID is
// set are encrypted/decrypted using the real KMS backend instead of the
// built-in mock key.
func (b *InMemoryBackend) WithKMS(e KMSEncryptor) *InMemoryBackend {
	b.kms = e

	return b
}

// WithCommandTTL sets the TTL used for the ExpiresAfter field on new commands.
// A zero or negative value falls back to the default (3600 seconds / 1 hour).
func (b *InMemoryBackend) WithCommandTTL(d time.Duration) *InMemoryBackend {
	if d > 0 {
		b.commandExpirySecs = d.Seconds()
	}

	return b
}

// WithCommandExecDelay sets how long a SendCommand invocation stays in the
// InProgress state before completing. The default of zero means commands
// complete synchronously (fast). A positive delay makes the InProgress window
// observable to SDK waiters: reads lazily complete the command once the delay
// has elapsed.
func (b *InMemoryBackend) WithCommandExecDelay(d time.Duration) *InMemoryBackend {
	if d > 0 {
		b.commandExecDelaySecs = d.Seconds()
	}

	return b
}

// WithAutomationExecDelay sets how long a StartAutomationExecution stays in the
// InProgress state before reaching a terminal status. Zero (the default)
// completes automations synchronously; a positive delay makes the InProgress
// window observable, with reads lazily completing the execution once elapsed.
func (b *InMemoryBackend) WithAutomationExecDelay(d time.Duration) *InMemoryBackend {
	if d > 0 {
		b.automationExecDelaySecs = d.Seconds()
	}

	return b
}
func getRegion(ctx context.Context) string {
	if region, ok := ctx.Value(regionContextKey{}).(string); ok {
		return region
	}

	return defaultRegion
}

// PutParameter creates or updates a parameter.
const (
	tierStandard           = "Standard"
	tierAdvanced           = "Advanced"
	tierIntelligentTiering = "Intelligent-Tiering"
	// maxStandardValueBytes is the Standard-tier limit (4 KiB).
	maxStandardValueBytes = 4096
	// maxAdvancedValueBytes is the Advanced-tier limit (8 KiB).
	maxAdvancedValueBytes = 8192
)
const (
	defaultPathMaxResults     = 10
	defaultDescribeMaxResults = 50
)

// Shared filter-key literals, reused by several Describe*/List* filter
// matchers (nodeAttributeValue, instanceInformationAttr, associationAttr,
// documentMatchesFilters, paramMatchesFilter) that all filter on the same
// real API attribute name.
const (
	filterKeyInstanceID   = "InstanceId"
	filterKeyName         = "Name"
	filterKeyAgentVersion = "AgentVersion"
)

// cleanupEmptyInnerMap removes the region key from a two-level map when the
// inner map is empty. Prevents empty maps from accumulating indefinitely.
// Caller must hold the write lock.
func cleanupEmptyInnerMap[V any](outer map[string]map[string]V, region string) {
	if len(outer[region]) == 0 {
		delete(outer, region)
	}
}

// parseNextToken converts a NextToken string to an integer start index.
func parseNextToken(token string) int {
	if token == "" {
		return 0
	}

	idx, err := strconv.Atoi(token)
	if err != nil || idx < 0 {
		return 0
	}

	return idx
}

// paginateSlice applies NextToken/MaxResults pagination to an already-ordered
// slice, the same offset-index scheme ListNodes established (instances.go).
// maxResults <= 0 falls back to defaultMax.
func paginateSlice[T any](items []T, nextToken string, maxResults int, defaultMax int) ([]T, string) {
	start := parseNextToken(nextToken)
	if start >= len(items) {
		return []T{}, ""
	}

	if maxResults <= 0 {
		maxResults = defaultMax
	}

	end := start + maxResults

	var next string

	if end < len(items) {
		next = strconv.Itoa(end)
	} else {
		end = len(items)
	}

	return items[start:end], next
}

// Reset clears all in-memory state from the backend. It is used by the
// POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	for _, regionTags := range b.tags {
		for _, t := range regionTags {
			t.Close()
		}
	}

	// b.registry is rebuilt from scratch (rather than registry.ResetAll'd)
	// because Reset also reallocates every *store.Table[V] field below to a
	// brand-new map[string]*store.Table[V] -- the old *store.Table[V]
	// pointers (and their registry registrations under "<name>/<region>")
	// are discarded entirely, so a stale registry would panic on
	// re-registration the next time a region is touched (store.Register
	// panics on a duplicate name; see store_setup.go's getOrCreateTable).
	b.registry = store.NewRegistry()

	b.parameters = make(map[string]*store.Table[Parameter])
	b.history = make(map[string]map[string][]ParameterHistory)
	b.tags = make(map[string]map[string]*tags.Tags)
	b.documents = make(map[string]*store.Table[Document])
	b.documentVersions = make(map[string]map[string][]DocumentVersion)
	b.documentPermissions = make(map[string]map[string][]string)
	b.documentSharedVersions = make(map[string]map[string]map[string]string)
	b.commands = make(map[string]*store.Table[Command])
	b.commandInvocations = make(map[string]map[string][]CommandInvocation)
	b.activations = make(map[string]*store.Table[Activation])
	b.cloudConnectors = make(map[string]*store.Table[CloudConnector])
	b.associations = make(map[string]*store.Table[Association])
	b.maintenanceWindows = make(map[string]*store.Table[MaintenanceWindow])
	b.maintenanceWindowTargets = make(map[string]*store.Table[MaintenanceWindowTarget])
	b.maintenanceWindowTasks = make(map[string]*store.Table[MaintenanceWindowTask])
	b.sessions = make(map[string]*store.Table[Session])
	b.accessRequests = make(map[string]*store.Table[AccessRequest])
	b.patchGroupToBaseline = make(map[string]map[string]string)
	b.opsItems = make(map[string]*store.Table[OpsItem])
	b.opsItemRelatedItems = make(map[string]map[string][]OpsItemRelatedItem)
	b.opsMetadata = make(map[string]*store.Table[OpsMetadata])
	b.patchBaselines = make(map[string]*store.Table[PatchBaseline])
	b.resourceIDToOpsMetadataArn = make(map[string]map[string]string)
	b.miscResourceTags = make(map[string]map[string]map[string]string)
	b.resourceDataSyncs = make(map[string]*store.Table[ResourceDataSync])
	b.parameterLabels = make(map[string]map[string]map[int64][]string)
	b.automationExecutions = make(map[string]*store.Table[AutomationExecution])
	b.serviceSettings = make(map[string]*store.Table[ServiceSetting])
	b.resourcePolicies = make(map[string]map[string][]*ResourcePolicy)
	b.executionPreviews = make(map[string]*store.Table[ExecutionPreview])
	b.inventory = make(map[string]map[string][]InventoryItem)
	b.compliance = make(map[string]map[string][]ComplianceItem)
	b.associationExecutions = make(map[string]map[string][]AssociationExecution)
	b.associationExecTargets = make(map[string]map[string][]AssociationExecutionTarget)
	b.inventoryDeletions = make(map[string][]InventoryDeletion)
	b.notifiedParameterPolicies = make(map[string]map[string]map[string]struct{})
	b.opsItemEvents = nil
	b.instancePatches = make(map[string]map[string][]PatchComplianceData)
	b.availablePatches = make(map[string][]Patch)

	// instancePatchStates/instanceProperties keep their existing
	// map[string]*store.Table[V] (not reallocated) and are cleared in place
	// via t.Reset() instead, then re-registered under their original names:
	// b.registry was just replaced with an empty one, and only a registered
	// table is visible to registry.SnapshotAll/RestoreAll (persistence.go) --
	// skipping re-registration would silently drop these two resources from
	// every snapshot taken after Reset.
	resetAndReregister(b.registry, "instancePatchStates", b.instancePatchStates)
	resetAndReregister(b.registry, "instanceProperties", b.instanceProperties)

	b.registerDefaultDocuments(defaultRegion)
}

// resetAndReregister clears every table in m and re-registers it on registry
// under "<name>/<region>". See Reset's comment on
// instancePatchStates/instanceProperties for why this is needed instead of
// just reallocating m.
func resetAndReregister[V any](registry *store.Registry, name string, m map[string]*store.Table[V]) {
	for region, t := range m {
		t.Reset()
		store.Register(registry, name+"/"+region, t)
	}
}

const (
	activationCodeChars     = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	activationCodeLen       = 20
	windowIDPrefix          = "mw-"
	windowTargetIDPrefix    = "mwt-"
	windowTaskIDPrefix      = "mwtask-"
	sessionIDPrefix         = "session-"
	sessionStatusConnected  = "Connected"
	sessionStatusTerminated = "Terminated"
	// maxTerminatedSessionsPerRegion bounds retained terminated (history)
	// sessions; the oldest are evicted first once the cap is exceeded.
	maxTerminatedSessionsPerRegion = 200
	// sessionHistoryRetentionSecs is how long a terminated session is retained
	// for DescribeSessions history before the janitor evicts it (24h, matching
	// AWS Session Manager history retention semantics).
	sessionHistoryRetentionSecs = 24 * 60 * 60
	activationIDPrefix          = "act-"
	baselineIDPrefix            = "pb-"
	opsItemIDPrefix             = "oi-"
	opsMetadataArnTpl           = "arn:aws:ssm:%s:%s:opsmetadata/%s"
	defaultAccountID            = "123456789012"
	defaultRegion               = "us-east-1"
	defaultOpsItemStatus        = "Open"
	defaultActivationExpiryHrs  = 24
)
const (
	commandStatusPending    = "Pending"
	commandStatusInProgress = "InProgress"
	commandStatusSuccess    = "Success"
	commandStatusFailed     = "Failed"
	commandStatusCancelled  = "Cancelled"
	commandStatusTimedOut   = "TimedOut"
	assocStatusSuccess      = "Success"
	faultClient             = "Client"
	opsItemStatusOpen       = "Open"
)

// AccountID returns the mocked AWS account ID used by this backend.
func (b *InMemoryBackend) AccountID() string { return defaultAccountID }

// Region returns the mocked AWS region used by this backend.
func (b *InMemoryBackend) Region() string { return defaultRegion }

const (
	automationStatusPending    = "Pending"
	automationStatusInProgress = "InProgress"
	// automationStatusCancelled is the real AutomationExecutionStatus enum
	// value (types/enums.go); there is no "Stopped" value in the real API.
	automationStatusCancelled = "Cancelled"
	automationStatusSuccess   = "Success"
	automationStatusFailed    = "Failed"
	calendarStateOpen         = "OPEN"
	policyIDPrefix            = "pol-"
	previewIDPrefix           = "ep-"
	connectionStatusConnected = "connected"
	settingStatusCustomized   = "Customized"
	settingStatusDefault      = "Default"
	platformTypeLinux         = "Linux"
	mwExecutionScheduleHours  = 24
)

// timeNow is a variable so tests can override it.
//
//nolint:gochecknoglobals // intentional hook for test time injection
var timeNow = time.Now
