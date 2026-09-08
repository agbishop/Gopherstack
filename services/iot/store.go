package iot

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/collections"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// RuleDispatcher is implemented by the CLI wiring layer and dispatches rule actions.
type RuleDispatcher interface {
	SendToSQS(queueURL, body string) error
	InvokeLambda(ctx context.Context, functionARN string, payload []byte) error
}

// InMemoryBackend is the in-memory implementation of StorageBackend.
type InMemoryBackend struct {
	dispatcher             RuleDispatcher
	shadows                map[shadowKey]*ThingShadow // thing+name → shadow
	resourceTags           map[string]map[string]string
	certificateTransfers   map[string]string
	thingBillingGroups     map[string]string
	thingThingGroups       map[string][]string
	packageVersionSboms    map[string]*SbomDocument
	jobTargets             map[string][]string
	securityProfileTargets map[string][]string
	thingPrincipals        map[string][]string
	thingPrincipalTypes    map[string]map[string]string // thingName -> principal -> thingPrincipalType
	auditMitigationTasks   map[string]string
	auditTasks             map[string]string
	thingGroupMembers      map[string][]string
	policyVersions         map[string][]*PolicyVersion
	provTemplateVersions   map[string][]*ProvisioningTemplateVersion
	policyTargets          map[string][]string

	// registry lets Reset collapse every converted resource table's
	// lifecycle to one call (registry.ResetAll()) instead of hand-rolled
	// re-initialization. See store_setup.go's registerAllTables for the
	// full list of tables and the (documented) fields left as raw maps
	// above/below instead.
	registry *store.Registry

	customMetrics              *store.Table[CustomMetric]
	rules                      *store.Table[TopicRule]
	fleetMetrics               *store.Table[FleetMetric]
	thingTypes                 *store.Table[ThingType]
	thingGroups                *store.Table[ThingGroup]
	certificates               *store.Table[Certificate]
	topicRuleDestinations      *store.Table[TopicRuleDestination]
	certificateProviders       *store.Table[CertificateProvider]
	jobs                       *store.Table[Job]
	jobExecutions              *store.Table[JobExecution]
	jobTemplates               *store.Table[JobTemplate]
	roleAliases                *store.Table[RoleAlias]
	domainConfigs              *store.Table[DomainConfiguration]
	dimensions                 *store.Table[Dimension]
	authorizers                *store.Table[Authorizer]
	billingGroups              *store.Table[BillingGroup]
	scheduledAudits            *store.Table[ScheduledAudit]
	mitigationActions          *store.Table[MitigationAction]
	securityProfiles           *store.Table[SecurityProfile]
	caCertificates             *store.Table[CACertificate]
	streams                    *store.Table[IoTStream]
	policies                   *store.Table[Policy]
	provTemplates              *store.Table[ProvisioningTemplate]
	things                     *store.Table[Thing]
	auditTaskObjects           *store.Table[AuditTask]
	otaUpdates                 *store.Table[OTAUpdate]
	iotPackages                *store.Table[IoTPackage]
	auditSuppressions          *store.Table[AuditSuppression]
	auditFindings              *store.Table[AuditFinding]
	v2LoggingLevels            *store.Table[V2LoggingLevel]
	commands                   *store.Table[IoTCommand]
	registrationTasks          *store.Table[ThingRegistrationTask]
	auditMitigationTaskObjects *store.Table[AuditMitigationTask]
	detectMitigationTasks      *store.Table[DetectMitigationTask]
	activeViolations           *store.Table[ActiveViolation]

	auditConfiguration         *AccountAuditConfiguration
	packageVersions2           map[string]map[string]*IoTPackageVersion
	packageConfig              *PackageConfiguration
	v2LoggingOptions           *V2LoggingOptions
	loggingOptions             *LoggingOptions
	eventConfigurations        *EventConfigurations
	commandExecutions          map[string]*IoTCommandExecution
	thingIndexingConfig        *ThingIndexingConfiguration
	thingGroupIndexingConfig   *ThingGroupIndexingConfiguration
	auditMitigationExecutions  map[string][]*AuditMitigationActionExecution
	detectMitigationExecutions map[string][]*DetectMitigationActionExecution
	accountEncryptionConfig    *AccountEncryptionConfiguration
	sbomValidationResults      map[string][]*SbomValidationResult
	metricValues               map[string][]*MetricDatapoint
	thingConnectivity          map[string]*ThingConnectivityData
	behaviorTrainingSummaries  map[string][]*BehaviorModelTrainingSummary
	registrationCode           string
	defaultAuthorizer          string
	accountID                  string
	region                     string
	violationEvents            []*ViolationEvent
	mqttPort                   int
	mu                         sync.RWMutex
}

// Compile-time assertion that InMemoryBackend implements StorageBackend.
var _ StorageBackend = (*InMemoryBackend)(nil)

// mqttDefaultPort is the default TCP port for the embedded MQTT broker.
const mqttDefaultPort = 1883

// NewInMemoryBackend creates a new InMemoryBackend with default values.
func NewInMemoryBackend() *InMemoryBackend {
	b := &InMemoryBackend{
		registry: store.NewRegistry(),

		certificateTransfers:   make(map[string]string),
		thingBillingGroups:     make(map[string]string),
		thingThingGroups:       make(map[string][]string),
		packageVersionSboms:    make(map[string]*SbomDocument),
		jobTargets:             make(map[string][]string),
		policyTargets:          make(map[string][]string),
		securityProfileTargets: make(map[string][]string),
		thingPrincipals:        make(map[string][]string),
		thingPrincipalTypes:    make(map[string]map[string]string),
		auditMitigationTasks:   make(map[string]string),
		auditTasks:             make(map[string]string),
		thingGroupMembers:      make(map[string][]string),
		policyVersions:         make(map[string][]*PolicyVersion),
		provTemplateVersions:   make(map[string][]*ProvisioningTemplateVersion),
		resourceTags:           make(map[string]map[string]string),
		packageVersions2:       make(map[string]map[string]*IoTPackageVersion),
		shadows:                make(map[shadowKey]*ThingShadow),
		commandExecutions:      make(map[string]*IoTCommandExecution),

		auditMitigationExecutions:  make(map[string][]*AuditMitigationActionExecution),
		detectMitigationExecutions: make(map[string][]*DetectMitigationActionExecution),

		sbomValidationResults:     make(map[string][]*SbomValidationResult),
		metricValues:              make(map[string][]*MetricDatapoint),
		thingConnectivity:         make(map[string]*ThingConnectivityData),
		behaviorTrainingSummaries: make(map[string][]*BehaviorModelTrainingSummary),

		accountID: "000000000000",
		region:    "us-east-1",
		mqttPort:  mqttDefaultPort,
	}

	registerAllTables(b)

	return b
}

// NewInMemoryBackendWithConfig creates a new InMemoryBackend with the given account and region.
func NewInMemoryBackendWithConfig(accountID, region string) *InMemoryBackend {
	b := NewInMemoryBackend()
	b.accountID = accountID
	b.region = region

	return b
}

// resetBatch3 clears all batch-3 backend state (called from Reset, lock held).
func (b *InMemoryBackend) resetBatch3() {
	b.packageVersions2 = make(map[string]map[string]*IoTPackageVersion)
	b.packageConfig = nil
	b.v2LoggingOptions = nil
	b.loggingOptions = nil
	b.eventConfigurations = nil
	b.commandExecutions = make(map[string]*IoTCommandExecution)
}

// Reset clears all backend state. Useful for test isolation.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Clears every table registered in store_setup.go's registerAllTables.
	// b.shadows is not part of the registry (no pure keyFn without changing
	// ThingShadow's shape), so it needs its own clear here.
	b.registry.ResetAll()
	b.shadows = make(map[shadowKey]*ThingShadow)

	b.certificateTransfers = make(map[string]string)
	b.thingBillingGroups = make(map[string]string)
	b.thingThingGroups = make(map[string][]string)
	b.packageVersionSboms = make(map[string]*SbomDocument)
	b.jobTargets = make(map[string][]string)
	b.policyTargets = make(map[string][]string)
	b.securityProfileTargets = make(map[string][]string)
	b.thingPrincipals = make(map[string][]string)
	b.thingPrincipalTypes = make(map[string]map[string]string)
	b.auditMitigationTasks = make(map[string]string)
	b.auditTasks = make(map[string]string)
	b.thingGroupMembers = make(map[string][]string)
	b.policyVersions = make(map[string][]*PolicyVersion)
	b.provTemplateVersions = make(map[string][]*ProvisioningTemplateVersion)
	b.resourceTags = make(map[string]map[string]string)
	b.resetBatch3()
	b.registrationCode = ""
	b.defaultAuthorizer = ""
	b.auditConfiguration = nil
	b.thingIndexingConfig = nil
	b.thingGroupIndexingConfig = nil
	b.resetDeviceDefender()
	b.resetFinalOps()
}

// resetFinalOps clears final-stub-batch backend state (called from Reset,
// lock held).
func (b *InMemoryBackend) resetFinalOps() {
	b.accountEncryptionConfig = nil
	b.sbomValidationResults = make(map[string][]*SbomValidationResult)
	b.metricValues = make(map[string][]*MetricDatapoint)
	b.thingConnectivity = make(map[string]*ThingConnectivityData)
	b.behaviorTrainingSummaries = make(map[string][]*BehaviorModelTrainingSummary)
}

// resetDeviceDefender clears all Device Defender backend state (audit + detect
// mitigation-action tasks and violations). Called from Reset, lock held.
func (b *InMemoryBackend) resetDeviceDefender() {
	b.auditMitigationExecutions = make(map[string][]*AuditMitigationActionExecution)
	b.detectMitigationExecutions = make(map[string][]*DetectMitigationActionExecution)
	b.violationEvents = nil
}

// SetRuleDispatcher wires the SQS/Lambda action dispatcher.
func (b *InMemoryBackend) SetRuleDispatcher(d RuleDispatcher) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.dispatcher = d
}

// GetDispatcher returns the current rule dispatcher (used by the broker hook).
func (b *InMemoryBackend) GetDispatcher() RuleDispatcher {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.dispatcher
}

// GetRules returns a snapshot of all active rules (used by the broker hook).
func (b *InMemoryBackend) GetRules() []*TopicRule {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*TopicRule, 0, b.rules.Len())

	for _, r := range b.rules.All() {
		out = append(out, cloneTopicRule(r))
	}

	return out
}

// cloneThing creates a deep copy of a Thing.
func cloneThing(t *Thing) *Thing {
	attrs := make(map[string]string, len(t.Attributes))
	maps.Copy(attrs, t.Attributes)

	return &Thing{
		ThingName:     t.ThingName,
		ThingTypeName: t.ThingTypeName,
		ThingType:     t.ThingType,
		ThingID:       t.ThingID,
		ARN:           t.ARN,
		Attributes:    attrs,
		Version:       t.Version,
		CreatedAt:     t.CreatedAt,
	}
}

// applyAttributePayload returns the attribute map that results from applying
// an AttributePayload update on top of an existing attribute set, matching
// AWS IoT's documented UpdateThing/UpdateThingGroup semantics: merge unset
// or false REPLACES existing attributes, merge true merges them; either
// way, a payload attribute with an empty string value deletes it from the
// result. A nil payload, or one with a nil Attributes map, is a no-op.
func applyAttributePayload(existing map[string]string, payload *AttributePayload) map[string]string {
	if payload == nil || payload.Attributes == nil {
		return existing
	}

	merge := payload.Merge != nil && *payload.Merge

	result := make(map[string]string, len(existing)+len(payload.Attributes))
	if merge {
		maps.Copy(result, existing)
	}

	for k, v := range payload.Attributes {
		if v == "" {
			delete(result, k)
		} else {
			result[k] = v
		}
	}

	return result
}

// cloneSbomDocument creates a deep copy of a SbomDocument.
func cloneSbomDocument(s *SbomDocument) *SbomDocument {
	if s == nil {
		return nil
	}

	cp := SbomDocument{}

	if s.S3Location != nil {
		loc := *s.S3Location
		cp.S3Location = &loc
	}

	return &cp
}

// sortedKeys returns a sorted slice of the keys in a map.
func sortedKeys[V any](m map[string]V) []string {
	keys := collections.SortedKeys(m)

	return keys
}

// MQTTPort returns the configured TCP port for the MQTT broker.
func (b *InMemoryBackend) MQTTPort() int {
	return b.mqttPort
}

// CreateThing creates a new IoT Thing.
func (b *InMemoryBackend) CreateThing(input *CreateThingInput) (*CreateThingOutput, error) {
	if input.ThingName == "" {
		return nil, fmt.Errorf("%w: ThingName is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.things.Has(input.ThingName) {
		return nil, fmt.Errorf("%w: thing %q already exists", ErrAlreadyExists, input.ThingName)
	}

	attrs := make(map[string]string)

	if input.AttributePayload != nil && input.AttributePayload.Attributes != nil {
		maps.Copy(attrs, input.AttributePayload.Attributes)
	}

	arn := arn.Build("iot", b.region, b.accountID, fmt.Sprintf("thing/%s", input.ThingName))
	id := uuid.NewString()

	b.things.Put(&Thing{
		ThingName:     input.ThingName,
		ThingTypeName: input.ThingTypeName,
		ThingType:     input.ThingTypeName,
		ThingID:       id,
		Attributes:    attrs,
		ARN:           arn,
		Version:       1,
		CreatedAt:     time.Now(),
	})

	if input.BillingGroupName != "" {
		b.thingBillingGroups[input.ThingName] = input.BillingGroupName
	}

	return &CreateThingOutput{
		ThingName: input.ThingName,
		ThingARN:  arn,
		ThingID:   id,
	}, nil
}

// DescribeThing returns a deep copy of an existing Thing.
func (b *InMemoryBackend) DescribeThing(thingName string) (*Thing, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	t, ok := b.things.Get(thingName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrThingNotFound, thingName)
	}

	clone := cloneThing(t)
	clone.BillingGroupName = b.thingBillingGroups[thingName]

	return clone, nil
}

// ListThings returns all Things sorted by name.
func (b *InMemoryBackend) ListThings() []*Thing {
	b.mu.RLock()
	defer b.mu.RUnlock()

	items := b.things.Snapshot()
	out := make([]*Thing, 0, len(items))

	for _, v := range items {
		out = append(out, cloneThing(v))
	}

	return out
}

// DeleteThing deletes a Thing by name. Any JobExecution rows targeting this
// thing are cascade-deleted along with it (mirrors DeleteJob's cascade over
// the other side of the same jobId/thingName key) so a deleted thing never
// leaves a ghost JobExecution behind for DescribeJobExecution/
// ListJobExecutionsForThing to keep returning.
func (b *InMemoryBackend) DeleteThing(thingName string, expectedVersion int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	t, ok := b.things.Get(thingName)
	if !ok {
		return fmt.Errorf("%w: %s", ErrThingNotFound, thingName)
	}

	if expectedVersion != 0 && expectedVersion != t.Version {
		return fmt.Errorf("%w: expected version %d, current version %d",
			ErrVersionConflict, expectedVersion, t.Version)
	}

	if principals := b.thingPrincipals[thingName]; len(principals) > 0 {
		return fmt.Errorf("%w: thing %q has attached principals", ErrDeleteConflict, thingName)
	}

	thingARN := arn.Build("iot", b.region, b.accountID, fmt.Sprintf("thing/%s", thingName))

	b.things.Delete(thingName)
	delete(b.resourceTags, thingARN)
	delete(b.thingBillingGroups, thingName)

	for _, exec := range b.jobExecutions.All() {
		if exec.ThingName == thingName {
			b.jobExecutions.Delete(jobExecKey(exec.JobID, exec.ThingName))
		}
	}

	return nil
}

// appendUnique appends v to items unless it's already present, matching AWS
// IoT's idempotent attach semantics (re-attaching an already-attached
// principal/policy/target succeeds without creating a duplicate list entry).
func appendUnique(items []string, v string) []string {
	if slices.Contains(items, v) {
		return items
	}

	return append(items, v)
}

// DescribeEndpoint returns the MQTT broker endpoint address.
func (b *InMemoryBackend) DescribeEndpoint(_ string) (*DescribeEndpointOutput, error) {
	return &DescribeEndpointOutput{
		EndpointAddress: fmt.Sprintf("mqtt.%s.amazonaws.com", b.region),
	}, nil
}

// AcceptCertificateTransfer accepts a pending certificate transfer, moving
// ownership to the target account recorded by the earlier TransferCertificate
// call and activating (or not) the certificate per SetAsActive.
func (b *InMemoryBackend) AcceptCertificateTransfer(input *AcceptCertificateTransferInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	cert, ok := b.certificates.Get(input.CertificateID)
	if !ok {
		return fmt.Errorf("certificate %q not found: %w", input.CertificateID, ErrCertificateNotFound)
	}

	if cert.Status != certStatusPendingTransfer {
		return fmt.Errorf("%w: certificate %q is not pending transfer", ErrValidation, input.CertificateID)
	}

	targetAccount, pending := b.certificateTransfers[input.CertificateID]
	if !pending {
		return fmt.Errorf("%w: certificate %q is not pending transfer", ErrValidation, input.CertificateID)
	}

	if input.SetAsActive {
		cert.Status = certStatusActive
	} else {
		cert.Status = certStatusInactive
	}

	cert.PreviousOwnedBy = cert.OwnedBy
	cert.OwnedBy = targetAccount
	cert.LastModifiedAt = time.Now()
	cert.TransferAcceptDate = time.Now()
	cert.TransferredTo = ""
	delete(b.certificateTransfers, input.CertificateID)

	return nil
}

// thingKey returns the canonical map key for a thing, preferring name over ARN.
func thingKey(name, arn string) string {
	if name != "" {
		return name
	}

	return arn
}

// packageVersionKey builds the composite key for a package version.
func packageVersionKey(packageName, versionName string) string {
	return packageName + "/" + versionName
}

// AttachThingPrincipal attaches a principal (certificate or Cognito identity) to a thing.
func (b *InMemoryBackend) AttachThingPrincipal(input *AttachThingPrincipalInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.thingPrincipals[input.ThingName] = appendUnique(b.thingPrincipals[input.ThingName], input.Principal)

	principalType := input.ThingPrincipalType
	if principalType == "" {
		principalType = defaultThingPrincipalType
	}
	if b.thingPrincipalTypes[input.ThingName] == nil {
		b.thingPrincipalTypes[input.ThingName] = make(map[string]string)
	}
	b.thingPrincipalTypes[input.ThingName][input.Principal] = principalType

	return nil
}

// UpdateThing updates attributes and/or type of an existing thing.
func (b *InMemoryBackend) UpdateThing(input *UpdateThingInput) error {
	if input.ThingName == "" {
		return fmt.Errorf("%w: ThingName is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	t, ok := b.things.Get(input.ThingName)
	if !ok {
		return fmt.Errorf("%w: %s", ErrThingNotFound, input.ThingName)
	}

	if input.ExpectedVersion != 0 && input.ExpectedVersion != t.Version {
		return fmt.Errorf("%w: expected version %d but current is %d",
			ErrVersionConflict, input.ExpectedVersion, t.Version)
	}

	if input.RemoveThingType {
		t.ThingTypeName = ""
		t.ThingType = ""
	} else if input.ThingTypeName != "" {
		t.ThingTypeName = input.ThingTypeName
		t.ThingType = input.ThingTypeName
	}

	t.Attributes = applyAttributePayload(t.Attributes, input.AttributePayload)

	t.Version++

	return nil
}

// ListThingPrincipals returns principals attached to the given thing.
func (b *InMemoryBackend) ListThingPrincipals(thingName string) ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.things.Has(thingName) {
		return nil, fmt.Errorf("%w: %s", ErrThingNotFound, thingName)
	}

	principals := b.thingPrincipals[thingName]
	if principals == nil {
		return []string{}, nil
	}

	out := make([]string, len(principals))
	copy(out, principals)

	return out, nil
}

// AddThingInternal seeds a Thing directly into the backend for testing.
func (b *InMemoryBackend) AddThingInternal(t Thing) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if t.ThingID == "" {
		t.ThingID = uuid.NewString()
	}

	if t.ARN == "" {
		t.ARN = arn.Build("iot", b.region, b.accountID, fmt.Sprintf("thing/%s", t.ThingName))
	}

	if t.Attributes == nil {
		t.Attributes = make(map[string]string)
	}

	if t.Version == 0 {
		t.Version = 1
	}

	b.things.Put(&t)
}

// -----------------------------------------------------------
// ThingType operations
// -----------------------------------------------------------

// -----------------------------------------------------------
// ThingGroup operations
// -----------------------------------------------------------

// -----------------------------------------------------------
// Certificate operations
// -----------------------------------------------------------

// -----------------------------------------------------------
// Policy attachment operations
// -----------------------------------------------------------

// -----------------------------------------------------------
// PolicyVersion operations
// -----------------------------------------------------------

// -----------------------------------------------------------
// TopicRuleDestination operations
// -----------------------------------------------------------

// -----------------------------------------------------------
// CertificateProvider operations
// -----------------------------------------------------------

// -----------------------------------------------------------
// Device Shadow operations
// -----------------------------------------------------------

func (b *InMemoryBackend) DetachThingPrincipal(thingName, principal string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	principals := b.thingPrincipals[thingName]
	for i, p := range principals {
		if p == principal {
			b.thingPrincipals[thingName] = append(principals[:i], principals[i+1:]...)
			delete(b.thingPrincipalTypes[thingName], principal)

			return nil
		}
	}

	return nil // AWS returns success even if not found
}

// thingPrincipalTypeFor returns the recorded thingPrincipalType for a
// thing/principal pair, falling back to defaultThingPrincipalType when the
// attachment recorded no type (e.g. internal registration paths that bypass
// AttachThingPrincipal). Callers must hold b.mu.
func (b *InMemoryBackend) thingPrincipalTypeFor(thingName, principal string) string {
	if t, ok := b.thingPrincipalTypes[thingName][principal]; ok {
		return t
	}

	return defaultThingPrincipalType
}

// ListThingPrincipalsV2 returns typed principal objects attached to a thing.
func (b *InMemoryBackend) ListThingPrincipalsV2(thingName string) ([]*ThingPrincipalObject, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.things.Has(thingName) {
		return nil, fmt.Errorf("%w: %s", ErrThingNotFound, thingName)
	}

	principals := b.thingPrincipals[thingName]
	out := make([]*ThingPrincipalObject, 0, len(principals))

	for _, p := range principals {
		out = append(out, &ThingPrincipalObject{
			Principal:          p,
			ThingPrincipalType: b.thingPrincipalTypeFor(thingName, p),
		})
	}

	return out, nil
}

// ListPrincipalThingsV2 returns typed thing objects a principal is attached to.
func (b *InMemoryBackend) ListPrincipalThingsV2(principal string) []*PrincipalThingObject {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []*PrincipalThingObject
	for thingName, principals := range b.thingPrincipals {
		if !slices.Contains(principals, principal) {
			continue
		}
		out = append(out, &PrincipalThingObject{
			ThingName:          thingName,
			ThingPrincipalType: b.thingPrincipalTypeFor(thingName, principal),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ThingName < out[j].ThingName })

	return out
}

// ThingConnectivityData is a thing's most recently reported connectivity
// status.
type ThingConnectivityData struct {
	DisconnectReason string  `json:"disconnectReason,omitempty"`
	Timestamp        float64 `json:"timestamp,omitempty"`
	Connected        bool    `json:"connected"`
}

// GetThingConnectivityData returns a thing's stored connectivity data,
// defaulting to "not connected" when nothing has been recorded yet.
func (b *InMemoryBackend) GetThingConnectivityData(thingName string) (*ThingConnectivityData, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.things.Has(thingName) {
		return nil, fmt.Errorf("%w: %s", ErrThingNotFound, thingName)
	}

	if cd, ok := b.thingConnectivity[thingName]; ok {
		cp := *cd

		return &cp, nil
	}

	return &ThingConnectivityData{Connected: false}, nil
}

// SetThingConnectivityInternal seeds/updates a thing's connectivity data for
// testing (real connectivity is derived from live MQTT session events, which
// this in-memory backend does not yet wire into fleet indexing).
func (b *InMemoryBackend) SetThingConnectivityInternal(thingName string, data ThingConnectivityData) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cp := data
	b.thingConnectivity[thingName] = &cp
}
