package appconfig

import (
	"time"
)

// Application represents an AppConfig application. This struct backs both
// persistence (store.Table snapshots marshal it directly, see store_setup.go)
// and, historically, the wire response -- CreatedAt/UpdatedAt must keep their
// JSON tags so Snapshot/Restore round-trips them; applicationToOutput
// (handler_applications.go) strips them for the real API response instead,
// since the real types.Application (aws-sdk-go-v2/service/appconfig@v1.48.4
// types/types.go, checked 2026-08-13) has Description/Id/Name only.
type Application struct {
	CreatedAt   time.Time `json:"CreatedAt,omitzero"`
	UpdatedAt   time.Time `json:"UpdatedAt,omitzero"`
	ID          string    `json:"Id"`
	Name        string    `json:"Name"`
	Description string    `json:"Description,omitempty"`
}

// Monitor represents an Amazon CloudWatch alarm used to monitor an AppConfig environment.
type Monitor struct {
	AlarmArn     string `json:"AlarmArn"`
	AlarmRoleArn string `json:"AlarmRoleArn,omitempty"`
}

// Environment represents an AppConfig environment. Same persistence-vs-wire
// split as Application above: CreatedAt/UpdatedAt keep their JSON tags for
// Snapshot/Restore; environmentToOutput strips them for the real API
// response (types.Environment, same SDK version, has no such members).
type Environment struct {
	CreatedAt     time.Time `json:"CreatedAt,omitzero"`
	UpdatedAt     time.Time `json:"UpdatedAt,omitzero"`
	ApplicationID string    `json:"ApplicationId"`
	ID            string    `json:"Id"`
	Name          string    `json:"Name"`
	Description   string    `json:"Description,omitempty"`
	State         string    `json:"State"`
	Monitors      []Monitor `json:"Monitors,omitempty"`
}

// Validator represents a validator for a configuration profile.
type Validator struct {
	Type    string `json:"Type"`    // JSON_SCHEMA or LAMBDA
	Content string `json:"Content"` // JSON schema doc or Lambda ARN
}

// ConfigurationProfile represents an AppConfig configuration profile. Same
// persistence-vs-wire split as Environment above: real
// Get/Create/UpdateConfigurationProfileOutput (appconfig@v1.48.4
// api_op_GetConfigurationProfile.go:44-90, checked 2026-09-08) has no
// CreatedAt member. CreatedAt exists here purely as internal state --
// DeletionProtectionCheck's "created in the past hour" exclusion needs it
// (gopherstack-z4v1) -- and is stripped for the wire by
// configurationProfileToOutput (configuration_profiles.go).
type ConfigurationProfile struct {
	CreatedAt        time.Time `json:"CreatedAt,omitzero"`
	ApplicationID    string    `json:"ApplicationId"`
	ID               string    `json:"Id"`
	Name             string    `json:"Name"`
	Description      string    `json:"Description,omitempty"`
	LocationURI      string    `json:"LocationUri"`
	Type             string    `json:"Type,omitempty"`
	RetrievalRoleArn string    `json:"RetrievalRoleArn,omitempty"`
	// KmsKeyIdentifier is a real Get/Create/UpdateConfigurationProfileOutput
	// member (appconfig@v1.48.4 api_op_GetConfigurationProfile.go) echoing
	// back whatever key ID/alias/ARN the caller supplied. KmsKeyArn is the
	// same output's other KMS member but is left unmodeled: it requires
	// resolving an identifier to a real KMS key ARN, which this backend has
	// no honest way to do (same rationale as HostedConfigurationVersionSummary
	// below).
	KmsKeyIdentifier string      `json:"KmsKeyIdentifier,omitempty"`
	Validators       []Validator `json:"Validators,omitempty"`
}

// ConfigurationProfileSummary is the shape ListConfigurationProfiles
// returns (types.ConfigurationProfileSummary, deserializers.go:12061) -- a
// strict subset of ConfigurationProfile: no Description, RetrievalRoleArn,
// or the full Validators list. ValidatorTypes carries the validator kinds
// only, one entry per Validators member.
type ConfigurationProfileSummary struct {
	ApplicationID  string   `json:"ApplicationId"`
	ID             string   `json:"Id"`
	Name           string   `json:"Name"`
	LocationURI    string   `json:"LocationUri"`
	Type           string   `json:"Type,omitempty"`
	ValidatorTypes []string `json:"ValidatorTypes,omitempty"`
}

// HostedConfigurationVersion represents a hosted configuration version.
type HostedConfigurationVersion struct {
	CreatedAt              time.Time `json:"CreatedAt,omitzero"`
	ApplicationID          string    `json:"ApplicationId"`
	ConfigurationProfileID string    `json:"ConfigurationProfileId"`
	ContentType            string    `json:"ContentType"`
	Description            string    `json:"Description,omitempty"`
	VersionLabel           string    `json:"VersionLabel,omitempty"`
	Content                []byte    `json:"-"`
	VersionNumber          int32     `json:"VersionNumber"`
}

// HostedConfigurationVersionSummary is the shape ListHostedConfigurationVersions
// returns (types.HostedConfigurationVersionSummary, deserializers.go:13825) --
// a strict subset of HostedConfigurationVersion: no CreatedAt (Get-only).
// KmsKeyArn is a real Summary member too, but this backend never resolves an
// identifier to a real KMS key ARN (ConfigurationProfile.KmsKeyIdentifier is
// modeled and echoed back verbatim; the ARN itself is not) -- so there is no
// honest value to put here. Left absent rather than fabricated, same
// rationale as personalize's undocumented FailureReason members
// (gopherstack-sm02).
type HostedConfigurationVersionSummary struct {
	ApplicationID          string `json:"ApplicationId"`
	ConfigurationProfileID string `json:"ConfigurationProfileId"`
	ContentType            string `json:"ContentType"`
	Description            string `json:"Description,omitempty"`
	VersionLabel           string `json:"VersionLabel,omitempty"`
	VersionNumber          int32  `json:"VersionNumber"`
}

// DeploymentStrategy represents an AppConfig deployment strategy. Same
// persistence-vs-wire split as Application above: CreatedAt/UpdatedAt keep
// their JSON tags for Snapshot/Restore; deploymentStrategyToOutput strips
// them for the real API response (types.DeploymentStrategy, same SDK
// version, has no such members).
type DeploymentStrategy struct {
	CreatedAt                   time.Time `json:"CreatedAt,omitzero"`
	UpdatedAt                   time.Time `json:"UpdatedAt,omitzero"`
	ID                          string    `json:"Id"`
	Name                        string    `json:"Name"`
	Description                 string    `json:"Description,omitempty"`
	GrowthType                  string    `json:"GrowthType"`
	ReplicateTo                 string    `json:"ReplicateTo"`
	DeploymentDurationInMinutes int32     `json:"DeploymentDurationInMinutes"`
	GrowthFactor                float32   `json:"GrowthFactor"`
	FinalBakeTimeInMinutes      int32     `json:"FinalBakeTimeInMinutes"`
}

// DeploymentEvent represents a single event in a deployment's history.
// ActionInvocations is intentionally unmodeled: this backend does not
// simulate real extension-action execution (Lambda invocation, SSM
// documents, ...), so a real SDK client's ActionInvocations would always
// come back empty here regardless -- see AppliedExtensions on Deployment
// for the same rationale. That matches AWS's own shape (the field is
// optional) rather than fabricating invocation data.
type DeploymentEvent struct {
	OccurredAt  time.Time `json:"OccurredAt,omitzero"`
	EventType   string    `json:"EventType"`
	Description string    `json:"Description,omitempty"`
	TriggeredBy string    `json:"TriggeredBy,omitempty"`
}

// AppliedExtension identifies an extension association that was in effect
// for an application, environment, or configuration profile when a
// deployment started.
type AppliedExtension struct {
	Parameters             map[string]string `json:"Parameters,omitempty"`
	ExtensionAssociationID string            `json:"ExtensionAssociationId,omitempty"`
	ExtensionID            string            `json:"ExtensionId,omitempty"`
	VersionNumber          int32             `json:"VersionNumber,omitempty"`
}

// Deployment represents an AppConfig deployment.
type Deployment struct {
	StartedAt                time.Time `json:"StartedAt,omitzero"`
	CompletedAt              time.Time `json:"CompletedAt,omitzero"`
	ApplicationID            string    `json:"ApplicationId"`
	EnvironmentID            string    `json:"EnvironmentId"`
	ConfigurationProfileID   string    `json:"ConfigurationProfileId"`
	DeploymentStrategyID     string    `json:"DeploymentStrategyId"`
	ConfigurationVersion     string    `json:"ConfigurationVersion"`
	State                    string    `json:"State"`
	TriggeredBy              string    `json:"TriggeredBy,omitempty"`
	Description              string    `json:"Description,omitempty"`
	ConfigurationName        string    `json:"ConfigurationName,omitempty"`
	ConfigurationLocationURI string    `json:"ConfigurationLocationUri,omitempty"`
	// KmsKeyIdentifier is a real Get/Start/StopDeploymentOutput member
	// (appconfig@v1.48.4 api_op_GetDeployment.go), snapshotted from the
	// deployed profile's own KmsKeyIdentifier at StartDeployment time, same
	// as ConfigurationName/ConfigurationLocationURI above.
	KmsKeyIdentifier            string             `json:"KmsKeyIdentifier,omitempty"`
	GrowthType                  string             `json:"GrowthType,omitempty"`
	VersionLabel                string             `json:"VersionLabel,omitempty"`
	EventLog                    []DeploymentEvent  `json:"EventLog,omitempty"`
	AppliedExtensions           []AppliedExtension `json:"AppliedExtensions,omitempty"`
	PercentageComplete          float32            `json:"PercentageComplete,omitempty"`
	GrowthFactor                float32            `json:"GrowthFactor,omitempty"`
	DeploymentNumber            int32              `json:"DeploymentNumber"`
	DeploymentDurationInMinutes int32              `json:"DeploymentDurationInMinutes,omitempty"`
	FinalBakeTimeInMinutes      int32              `json:"FinalBakeTimeInMinutes,omitempty"`
}

// DeploymentSummary is the shape ListDeployments returns
// (types.DeploymentSummary, deserializers.go:12583) -- a strict subset of
// Deployment: no ApplicationId, EnvironmentId, DeploymentStrategyId,
// Description, ConfigurationLocationUri, EventLog, or AppliedExtensions.
// Type is a real DeploymentSummary member that GetDeployment's own output
// shape lacks entirely -- see deploymentToSummary in deployments.go for how
// it's populated.
type DeploymentSummary struct {
	StartedAt                   time.Time `json:"StartedAt,omitzero"`
	CompletedAt                 time.Time `json:"CompletedAt,omitzero"`
	ConfigurationProfileID      string    `json:"ConfigurationProfileId"`
	ConfigurationVersion        string    `json:"ConfigurationVersion"`
	State                       string    `json:"State"`
	Type                        string    `json:"Type,omitempty"`
	ConfigurationName           string    `json:"ConfigurationName,omitempty"`
	GrowthType                  string    `json:"GrowthType,omitempty"`
	VersionLabel                string    `json:"VersionLabel,omitempty"`
	PercentageComplete          float32   `json:"PercentageComplete,omitempty"`
	GrowthFactor                float32   `json:"GrowthFactor,omitempty"`
	DeploymentNumber            int32     `json:"DeploymentNumber"`
	DeploymentDurationInMinutes int32     `json:"DeploymentDurationInMinutes,omitempty"`
	FinalBakeTimeInMinutes      int32     `json:"FinalBakeTimeInMinutes,omitempty"`
}

// ExtensionAction represents a single action in an AppConfig extension.
type ExtensionAction struct {
	Name        string `json:"Name,omitempty"`
	Description string `json:"Description,omitempty"`
	RoleArn     string `json:"RoleArn,omitempty"`
	URI         string `json:"Uri,omitempty"`
}

// ExtensionParameter describes a parameter accepted by an extension.
// Dynamic is a real types.Parameter member (appconfig@v1.48.4
// deserializers.go's Dynamic case, shared by request and response) that was
// previously discarded on input and never emitted on output.
type ExtensionParameter struct {
	Description string `json:"Description,omitempty"`
	Dynamic     bool   `json:"Dynamic,omitempty"`
	Required    bool   `json:"Required,omitempty"`
}

// Extension represents an AppConfig extension.
type Extension struct {
	Actions       map[string][]ExtensionAction  `json:"Actions,omitempty"`
	Parameters    map[string]ExtensionParameter `json:"Parameters,omitempty"`
	Arn           string                        `json:"Arn"`
	Description   string                        `json:"Description,omitempty"`
	ID            string                        `json:"Id"`
	Name          string                        `json:"Name"`
	VersionNumber int32                         `json:"VersionNumber"`
}

// ExtensionSummary is the shape ListExtensions returns (types.ExtensionSummary,
// deserializers.go:13700) -- a strict subset of Extension: no Actions or
// Parameters.
type ExtensionSummary struct {
	Arn           string `json:"Arn"`
	Description   string `json:"Description,omitempty"`
	ID            string `json:"Id"`
	Name          string `json:"Name"`
	VersionNumber int32  `json:"VersionNumber"`
}

// ExtensionAssociation represents an association between an extension and an AppConfig resource.
type ExtensionAssociation struct {
	Parameters             map[string]string `json:"Parameters,omitempty"`
	Arn                    string            `json:"Arn"`
	ExtensionArn           string            `json:"ExtensionArn"`
	ID                     string            `json:"Id"`
	ResourceArn            string            `json:"ResourceArn"`
	ExtensionVersionNumber int32             `json:"ExtensionVersionNumber"`
}

// ExtensionAssociationSummary is the shape ListExtensionAssociations returns
// (types.ExtensionAssociationSummary, deserializers.go:13608) -- a strict
// subset of ExtensionAssociation: no Arn, Parameters, or
// ExtensionVersionNumber.
type ExtensionAssociationSummary struct {
	ExtensionArn string `json:"ExtensionArn"`
	ID           string `json:"Id"`
	ResourceArn  string `json:"ResourceArn"`
}

// DeletionProtectionSettings represents the deletion protection configuration for an account.
type DeletionProtectionSettings struct {
	Enabled                   *bool  `json:"Enabled,omitempty"`
	ProtectionPeriodInMinutes *int32 `json:"ProtectionPeriodInMinutes,omitempty"`
}

// VendedMetricsSettings represents the vended-metrics configuration for an
// account -- a real Get/UpdateAccountSettingsOutput member
// (appconfig@v1.48.4 api_op_GetAccountSettings.go) alongside
// DeletionProtection, previously unmodeled on both directions.
type VendedMetricsSettings struct {
	Enabled *bool `json:"Enabled,omitempty"`
}

// AccountSettings holds account-level AppConfig settings.
type AccountSettings struct {
	DeletionProtection *DeletionProtectionSettings `json:"DeletionProtection,omitempty"`
	VendedMetrics      *VendedMetricsSettings      `json:"VendedMetrics,omitempty"`
}

// AttributeValue is a single attribute value attached to a Treatment's
// FlagValue.AttributeValues map. Real AWS AppConfig models this as a tagged
// union (aws-sdk-go-v2/service/appconfig/types.AttributeValue:
// BooleanValue | NumberValue | StringValue | NumberArray | StringArray,
// selected by which JSON key is present on the wire -- see
// awsRestjson1_serializeDocumentAttributeValue in the SDK's serializers.go).
// This backend does not evaluate flag values (no flag-evaluation engine
// exists here, matching the family's other "storage only" fields), so
// rather than reimplement a Go union type it stores whichever member(s) the
// client sent and echoes them back unchanged on the same wire keys -- a
// faithful round-trip without fabricating semantics this backend can't
// give meaning to.
type AttributeValue struct {
	BooleanValue *bool     `json:"BooleanValue,omitempty"`
	NumberValue  *float64  `json:"NumberValue,omitempty"`
	StringValue  *string   `json:"StringValue,omitempty"`
	NumberArray  []float64 `json:"NumberArray,omitempty"`
	StringArray  []string  `json:"StringArray,omitempty"`
}

// FlagValue is the feature flag value served to users assigned to a Treatment.
type FlagValue struct {
	AttributeValues map[string]AttributeValue `json:"AttributeValues,omitempty"`
	Enabled         bool                      `json:"Enabled"`
}

// Treatment represents one variation (or the control) evaluated during an
// experiment. Key is server-generated: real CreateExperimentDefinition's
// TreatmentInput/UpdateExperimentDefinitionInput carry no client-supplied
// key at all (only Description/FlagValue/Weight), so AWS itself must assign
// it. This backend assigns "Control" to the control treatment and
// "Treatment1".."TreatmentN" (1-indexed, in the order supplied) to the
// rest -- see CreateExperimentDefinition's doc comment in
// experiment_definitions.go for why this specific, deterministic scheme was
// chosen over a random one.
type Treatment struct {
	FlagValue   *FlagValue `json:"FlagValue,omitempty"`
	Description string     `json:"Description,omitempty"`
	Key         string     `json:"Key,omitempty"`
	Weight      float32    `json:"Weight"`
}

// TreatmentOverrides assigns specific entity IDs directly to treatment
// keys, bypassing random assignment. Real AWS models this as a tagged union
// (types.TreatmentOverrides) with exactly one known member, "Inline" (a map
// of entity ID -> treatment key, see
// awsRestjson1_serializeDocumentTreatmentOverrides in the SDK); this
// backend models the union directly as that one member since it is the
// only variant the SDK ships.
type TreatmentOverrides struct {
	Inline map[string]string `json:"Inline,omitempty"`
}

// DeploymentParameters carries the optional KMS/extension-parameter
// configuration a StartExperimentRun/StopExperimentRun/UpdateExperimentRun
// request can attach to the underlying deployment real AWS creates to
// expose treatments. This backend has no such underlying deployment to
// configure (see the ExperimentRun doc comment in experiment_runs.go) and
// real GetExperimentRun/StartExperimentRun/etc. output shapes never echo
// DeploymentParameters back to the caller either -- so it is accepted on
// input and intentionally discarded rather than stored, matching what a
// real client would actually observe (nothing).
type DeploymentParameters struct {
	DynamicExtensionParameters map[string]string `json:"DynamicExtensionParameters,omitempty"`
	Tags                       map[string]string `json:"Tags,omitempty"`
}

// ExperimentRunResult captures the free-text, client-supplied narrative
// outcome of an experiment run (an executive summary and launch/no-launch
// rationale). This backend has no analytics engine to compute these itself
// -- see the results_verdict discussion in PARITY.md -- so it only stores
// and echoes back whatever a caller (typically StopExperimentRun) supplies.
type ExperimentRunResult struct {
	ExecutiveSummary   string `json:"ExecutiveSummary,omitempty"`
	ReasonsNotToLaunch string `json:"ReasonsNotToLaunch,omitempty"`
	ReasonsToLaunch    string `json:"ReasonsToLaunch,omitempty"`
}

// ExperimentDefinitionSnapshot captures an ExperimentDefinition's fields at
// the moment StartExperimentRun was called, matching real AWS's "a snapshot
// of the experiment definition at the time the run was started" semantics:
// a later UpdateExperimentDefinition must not retroactively change what an
// already-started run reports it ran against.
type ExperimentDefinitionSnapshot struct {
	ApplicationID          string      `json:"ApplicationId"`
	AudienceDescription    string      `json:"AudienceDescription,omitempty"`
	AudienceRule           string      `json:"AudienceRule"`
	ConfigurationProfileID string      `json:"ConfigurationProfileId"`
	Control                Treatment   `json:"Control"`
	EnvironmentID          string      `json:"EnvironmentId"`
	FlagKey                string      `json:"FlagKey"`
	Hypothesis             string      `json:"Hypothesis,omitempty"`
	ID                     string      `json:"Id"`
	LaunchCriteria         string      `json:"LaunchCriteria,omitempty"`
	Name                   string      `json:"Name"`
	Treatments             []Treatment `json:"Treatments,omitempty"`
}

// ExperimentDefinition represents an AppConfig experiment definition: the
// purpose, scope, and treatment configuration of an A/B test attached to a
// feature-flag configuration profile.
type ExperimentDefinition struct {
	CreatedAt              time.Time   `json:"CreatedAt,omitzero"`
	UpdatedAt              time.Time   `json:"UpdatedAt,omitzero"`
	ApplicationID          string      `json:"ApplicationId"`
	ID                     string      `json:"Id"`
	Name                   string      `json:"Name"`
	ConfigurationProfileID string      `json:"ConfigurationProfileId"`
	EnvironmentID          string      `json:"EnvironmentId"`
	FlagKey                string      `json:"FlagKey"`
	AudienceDescription    string      `json:"AudienceDescription,omitempty"`
	AudienceRule           string      `json:"AudienceRule"`
	Hypothesis             string      `json:"Hypothesis,omitempty"`
	LaunchCriteria         string      `json:"LaunchCriteria,omitempty"`
	KmsKeyIdentifier       string      `json:"KmsKeyIdentifier,omitempty"`
	Status                 string      `json:"Status"`
	Control                Treatment   `json:"Control"`
	Treatments             []Treatment `json:"Treatments,omitempty"`
}

// ExperimentDefinitionSummary is the shape ListExperimentDefinitions returns
// (types.ExperimentDefinitionSummary, deserializers.go:13090) -- a strict
// subset of ExperimentDefinition: no AudienceDescription, AudienceRule,
// Control, KmsKeyIdentifier, LaunchCriteria, or Treatments.
type ExperimentDefinitionSummary struct {
	CreatedAt              time.Time `json:"CreatedAt,omitzero"`
	UpdatedAt              time.Time `json:"UpdatedAt,omitzero"`
	ApplicationID          string    `json:"ApplicationId"`
	ID                     string    `json:"Id"`
	Name                   string    `json:"Name"`
	ConfigurationProfileID string    `json:"ConfigurationProfileId"`
	EnvironmentID          string    `json:"EnvironmentId"`
	FlagKey                string    `json:"FlagKey"`
	Hypothesis             string    `json:"Hypothesis,omitempty"`
	Status                 string    `json:"Status"`
}

// ExperimentRunEvent records a single lifecycle event -- run start, an
// exposure-percentage change, a treatment-override change, or a run stop --
// observed during an experiment run. Events are recorded by this backend as
// they actually occur (see experiment_runs.go), never fabricated after the
// fact or synthesized as a plausible-looking timeline.
type ExperimentRunEvent struct {
	OccurredAt           time.Time          `json:"OccurredAt,omitzero"`
	TreatmentOverrides   TreatmentOverrides `json:"TreatmentOverrides,omitzero"`
	AssociatedDeployment string             `json:"AssociatedDeployment,omitempty"`
	Description          string             `json:"Description,omitempty"`
	EventType            string             `json:"EventType"`
	TriggeredBy          string             `json:"TriggeredBy,omitempty"`
	ExposurePercentage   float32            `json:"ExposurePercentage,omitempty"`
}

// ExperimentRun represents one execution of an ExperimentDefinition:
// audience exposure, treatment overrides, and status over its lifetime.
// Real GetExperimentRun/StartExperimentRun/etc. output shapes have no
// DeploymentParameters member and no embedded event list -- events are
// retrieved separately via ListExperimentRunEvents -- so Events is
// intentionally unexported from JSON (internal bookkeeping only).
type ExperimentRun struct {
	StartedAt                    time.Time                     `json:"StartedAt,omitzero"`
	EndedAt                      time.Time                     `json:"EndedAt,omitzero"`
	UpdatedAt                    time.Time                     `json:"UpdatedAt,omitzero"`
	ApplicationID                string                        `json:"ApplicationId"`
	ExperimentDefinitionID       string                        `json:"ExperimentDefinitionId"`
	Description                  string                        `json:"Description,omitempty"`
	Status                       string                        `json:"Status"`
	Result                       *ExperimentRunResult          `json:"Result,omitempty"`
	ExperimentDefinitionSnapshot *ExperimentDefinitionSnapshot `json:"ExperimentDefinitionSnapshot,omitempty"`
	TreatmentOverrides           TreatmentOverrides            `json:"TreatmentOverrides,omitzero"`
	Events                       []ExperimentRunEvent          `json:"-"`
	ExposurePercentage           float32                       `json:"ExposurePercentage,omitempty"`
	Run                          int32                         `json:"Run"`
}

// ExperimentRunSummary is the shape ListExperimentRuns returns
// (types.ExperimentRunSummary, deserializers.go:13430) -- a strict subset
// of ExperimentRun: no ApplicationId, ExperimentDefinitionSnapshot,
// ExposurePercentage, Result, or TreatmentOverrides.
type ExperimentRunSummary struct {
	StartedAt              time.Time `json:"StartedAt,omitzero"`
	EndedAt                time.Time `json:"EndedAt,omitzero"`
	UpdatedAt              time.Time `json:"UpdatedAt,omitzero"`
	ExperimentDefinitionID string    `json:"ExperimentDefinitionId"`
	Description            string    `json:"Description,omitempty"`
	Status                 string    `json:"Status"`
	Run                    int32     `json:"Run"`
}
