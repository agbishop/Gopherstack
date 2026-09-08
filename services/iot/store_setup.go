package iot

// Every map[string]*T resource field on InMemoryBackend whose key is a pure
// function of the value's own fields is registered exactly once, here, as a
// *store.Table[T] on b.registry (see pkgs/store's package doc). Fields that
// don't fit that model remain plain maps — see registerAllTables below.
import "github.com/blackbirdworks/gopherstack/pkgs/store"

func thingsKeyFn(v *Thing) string                               { return v.ThingName }
func thingTypesKeyFn(v *ThingType) string                       { return v.ThingTypeName }
func thingGroupsKeyFn(v *ThingGroup) string                     { return v.ThingGroupName }
func certificatesKeyFn(v *Certificate) string                   { return v.CertificateID }
func policiesKeyFn(v *Policy) string                            { return v.PolicyName }
func rulesKeyFn(v *TopicRule) string                            { return v.RuleName }
func jobsKeyFn(v *Job) string                                   { return v.JobID }
func jobExecutionsKeyFn(v *JobExecution) string                 { return jobExecKey(v.JobID, v.ThingName) }
func jobTemplatesKeyFn(v *JobTemplate) string                   { return v.JobTemplateID }
func billingGroupsKeyFn(v *BillingGroup) string                 { return v.BillingGroupName }
func topicRuleDestinationsKeyFn(v *TopicRuleDestination) string { return v.ARN }
func certificateProvidersKeyFn(v *CertificateProvider) string   { return v.CertificateProviderName }
func roleAliasesKeyFn(v *RoleAlias) string                      { return v.RoleAlias }
func domainConfigsKeyFn(v *DomainConfiguration) string          { return v.DomainConfigurationName }
func dimensionsKeyFn(v *Dimension) string                       { return v.Name }
func authorizersKeyFn(v *Authorizer) string                     { return v.AuthorizerName }
func scheduledAuditsKeyFn(v *ScheduledAudit) string             { return v.ScheduledAuditName }
func mitigationActionsKeyFn(v *MitigationAction) string         { return v.ActionName }
func securityProfilesKeyFn(v *SecurityProfile) string           { return v.SecurityProfileName }
func caCertificatesKeyFn(v *CACertificate) string               { return v.CertificateID }
func streamsKeyFn(v *IoTStream) string                          { return v.StreamID }
func provTemplatesKeyFn(v *ProvisioningTemplate) string         { return v.TemplateName }
func auditTaskObjectsKeyFn(v *AuditTask) string                 { return v.TaskID }
func otaUpdatesKeyFn(v *OTAUpdate) string                       { return v.OTAUpdateID }
func iotPackagesKeyFn(v *IoTPackage) string                     { return v.PackageName }
func auditSuppressionsKeyFn(v *AuditSuppression) string {
	return auditSuppressionKey(v.CheckName, v.ResourceIdentifier)
}
func auditFindingsKeyFn(v *AuditFinding) string                     { return v.FindingID }
func v2LoggingLevelsKeyFn(v *V2LoggingLevel) string                 { return v2LogLevelKey(v.Target) }
func commandsKeyFn(v *IoTCommand) string                            { return v.CommandID }
func registrationTasksKeyFn(v *ThingRegistrationTask) string        { return v.TaskID }
func auditMitigationTaskObjectsKeyFn(v *AuditMitigationTask) string { return v.TaskID }
func detectMitigationTasksKeyFn(v *DetectMitigationTask) string     { return v.TaskID }
func activeViolationsKeyFn(v *ActiveViolation) string               { return v.ViolationID }
func fleetMetricsKeyFn(v *FleetMetric) string                       { return v.MetricName }
func customMetricsKeyFn(v *CustomMetric) string                     { return v.MetricName }

// registerAllTables registers every converted resource map on b.registry
// exactly once. Call only during construction (immediately after b.registry
// is created), never on every Reset() — store.Register panics on a
// duplicate name, so runtime resets go through registry.ResetAll() instead
// (see InMemoryBackend.Reset in store.go).
//
// Fields left as plain maps: shadows (keyed by composite shadowKey{thingName,
// shadowName}, no pure keyFn without changing ThingShadow's shape; cleared
// separately in Reset());
// packageVersionSboms/commandExecutions/thingConnectivity (value carries no
// recoverable identity field for its key); resourceTags, certificateTransfers,
// thingBillingGroups, thingThingGroups, thingGroupMembers, jobTargets,
// policyTargets, securityProfileTargets, thingPrincipals,
// auditMitigationTasks, auditTasks (value isn't map[string]*T);
// policyVersions, provTemplateVersions, auditMitigationExecutions,
// detectMitigationExecutions, sbomValidationResults, metricValues,
// behaviorTrainingSummaries (slice-valued, store.Table holds one *V per key);
// packageVersions2 (nested map, value at the outer key is itself a map).
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
	func(b *InMemoryBackend) { b.things = store.Register(b.registry, "things", store.New(thingsKeyFn)) },
	func(b *InMemoryBackend) {
		b.thingTypes = store.Register(b.registry, "thingTypes", store.New(thingTypesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.thingGroups = store.Register(b.registry, "thingGroups", store.New(thingGroupsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.certificates = store.Register(b.registry, "certificates", store.New(certificatesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.policies = store.Register(b.registry, "policies", store.New(policiesKeyFn))
	},
	func(b *InMemoryBackend) { b.rules = store.Register(b.registry, "rules", store.New(rulesKeyFn)) },
	func(b *InMemoryBackend) { b.jobs = store.Register(b.registry, "jobs", store.New(jobsKeyFn)) },
	func(b *InMemoryBackend) {
		b.jobExecutions = store.Register(b.registry, "jobExecutions", store.New(jobExecutionsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.jobTemplates = store.Register(b.registry, "jobTemplates", store.New(jobTemplatesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.billingGroups = store.Register(b.registry, "billingGroups", store.New(billingGroupsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.topicRuleDestinations = store.Register(
			b.registry, "topicRuleDestinations", store.New(topicRuleDestinationsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.certificateProviders = store.Register(
			b.registry, "certificateProviders", store.New(certificateProvidersKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.roleAliases = store.Register(b.registry, "roleAliases", store.New(roleAliasesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.domainConfigs = store.Register(b.registry, "domainConfigs", store.New(domainConfigsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.dimensions = store.Register(b.registry, "dimensions", store.New(dimensionsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.authorizers = store.Register(b.registry, "authorizers", store.New(authorizersKeyFn))
	},
	func(b *InMemoryBackend) {
		b.scheduledAudits = store.Register(b.registry, "scheduledAudits", store.New(scheduledAuditsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.mitigationActions = store.Register(b.registry, "mitigationActions", store.New(mitigationActionsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.securityProfiles = store.Register(b.registry, "securityProfiles", store.New(securityProfilesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.caCertificates = store.Register(b.registry, "caCertificates", store.New(caCertificatesKeyFn))
	},
	func(b *InMemoryBackend) { b.streams = store.Register(b.registry, "streams", store.New(streamsKeyFn)) },
	func(b *InMemoryBackend) {
		b.provTemplates = store.Register(b.registry, "provTemplates", store.New(provTemplatesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.auditTaskObjects = store.Register(b.registry, "auditTaskObjects", store.New(auditTaskObjectsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.otaUpdates = store.Register(b.registry, "otaUpdates", store.New(otaUpdatesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.iotPackages = store.Register(b.registry, "iotPackages", store.New(iotPackagesKeyFn))
	},
	func(b *InMemoryBackend) {
		b.auditSuppressions = store.Register(b.registry, "auditSuppressions", store.New(auditSuppressionsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.auditFindings = store.Register(b.registry, "auditFindings", store.New(auditFindingsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.v2LoggingLevels = store.Register(b.registry, "v2LoggingLevels", store.New(v2LoggingLevelsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.commands = store.Register(b.registry, "commands", store.New(commandsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.registrationTasks = store.Register(b.registry, "registrationTasks", store.New(registrationTasksKeyFn))
	},
	func(b *InMemoryBackend) {
		b.auditMitigationTaskObjects = store.Register(
			b.registry, "auditMitigationTaskObjects", store.New(auditMitigationTaskObjectsKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.detectMitigationTasks = store.Register(
			b.registry, "detectMitigationTasks", store.New(detectMitigationTasksKeyFn),
		)
	},
	func(b *InMemoryBackend) {
		b.activeViolations = store.Register(b.registry, "activeViolations", store.New(activeViolationsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.fleetMetrics = store.Register(b.registry, "fleetMetrics", store.New(fleetMetricsKeyFn))
	},
	func(b *InMemoryBackend) {
		b.customMetrics = store.Register(b.registry, "customMetrics", store.New(customMetricsKeyFn))
	},
}
