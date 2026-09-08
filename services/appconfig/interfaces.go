package appconfig

import "context"

// StorageBackend defines the operations supported by the AppConfig in-memory backend.
type StorageBackend interface {
	// PaginationSecret returns the HMAC secret used to sign pagination tokens.
	PaginationSecret() string

	// Snapshot and Restore implement persistence.Persistable. Handler
	// delegates to them (see persistence.go) so cli.go's generic
	// setupPersistence picks AppConfig up.
	Snapshot(ctx context.Context) []byte
	Restore(ctx context.Context, data []byte) error

	// CreateApplication creates a new AppConfig application. tags are
	// applied inline at creation time (see CreateExperimentDefinition's
	// doc comment for why TagResource is not used for this).
	CreateApplication(name, description string, tags map[string]string) (*Application, error)
	// GetApplication retrieves an application by ID.
	GetApplication(applicationID string) (*Application, error)
	// ListApplications returns paginated applications.
	ListApplications(nextToken string, maxResults int) ([]Application, string)
	// UpdateApplication updates an application's name and description. A nil
	// name/description means the field was omitted from the request and must
	// be left unchanged (real UpdateApplicationInput.Name/Description are
	// optional *string members; only a present, non-nil member overwrites
	// the existing value -- see AWS AppConfig's UpdateApplication contract).
	UpdateApplication(applicationID string, name, description *string) (*Application, error)
	// DeleteApplication deletes an application by ID.
	DeleteApplication(applicationID string) error

	// CreateEnvironment creates a new environment within an application.
	// tags are applied inline at creation time (see CreateApplication).
	CreateEnvironment(
		applicationID, name, description string,
		monitors []Monitor,
		tags map[string]string,
	) (*Environment, error)
	// GetEnvironment retrieves an environment by application and environment ID.
	GetEnvironment(applicationID, environmentID string) (*Environment, error)
	// ListEnvironments returns paginated environments for an application.
	ListEnvironments(applicationID, nextToken string, maxResults int) ([]Environment, string, error)
	// UpdateEnvironment updates an environment's name, description, and
	// monitors. A nil name/description leaves the field unchanged (see
	// UpdateApplication doc); a nil monitors leaves the existing monitor
	// list unchanged, while a non-nil (possibly empty) slice replaces it,
	// matching UpdateEnvironmentInput's optional Monitors member.
	UpdateEnvironment(
		applicationID, environmentID string,
		name, description *string,
		monitors *[]Monitor,
	) (*Environment, error)
	// DeleteEnvironment deletes an environment.
	DeleteEnvironment(applicationID, environmentID, deletionProtectionCheck string) error

	// CreateConfigurationProfile creates a new configuration profile. tags
	// are applied inline at creation time (see CreateApplication).
	CreateConfigurationProfile(
		applicationID, name, description, locationURI, profileType, retrievalRoleArn, kmsKeyIdentifier string,
		validators []Validator,
		tags map[string]string,
	) (*ConfigurationProfile, error)
	// GetConfigurationProfile retrieves a configuration profile.
	GetConfigurationProfile(applicationID, profileID string) (*ConfigurationProfile, error)
	// ListConfigurationProfiles returns paginated profiles for an application.
	ListConfigurationProfiles(
		applicationID, nextToken, profileType string,
		maxResults int,
	) ([]ConfigurationProfile, string, error)
	// UpdateConfigurationProfile updates a configuration profile. Nil
	// name/description/retrievalRoleArn/kmsKeyIdentifier leave the field
	// unchanged; a nil validators leaves the existing validator list
	// unchanged, while a non-nil (possibly empty) slice replaces it --
	// matching UpdateConfigurationProfileInput's optional members.
	UpdateConfigurationProfile(
		applicationID, profileID string,
		name, description, retrievalRoleArn, kmsKeyIdentifier *string,
		validators *[]Validator,
	) (*ConfigurationProfile, error)
	// DeleteConfigurationProfile deletes a configuration profile.
	DeleteConfigurationProfile(applicationID, profileID, deletionProtectionCheck string) error

	// CreateHostedConfigurationVersion creates a hosted configuration
	// version. latestVersionNumber implements the optional
	// optimistic-concurrency check real AWS binds to the
	// "Latest-Version-Number" request header: when non-nil, it must match
	// the profile's current latest version or the call is rejected.
	CreateHostedConfigurationVersion(
		applicationID, profileID, contentType, description, versionLabel string,
		content []byte,
		latestVersionNumber *int32,
	) (*HostedConfigurationVersion, error)
	// GetHostedConfigurationVersion retrieves a hosted configuration version.
	GetHostedConfigurationVersion(
		applicationID, profileID string,
		versionNumber int32,
	) (*HostedConfigurationVersion, error)
	// ListHostedConfigurationVersions returns paginated versions for a profile.
	ListHostedConfigurationVersions(
		applicationID, profileID, nextToken, versionLabel string,
		maxResults int,
	) ([]HostedConfigurationVersion, string, error)
	// DeleteHostedConfigurationVersion deletes a hosted configuration version.
	DeleteHostedConfigurationVersion(applicationID, profileID string, versionNumber int32) error

	// CreateDeploymentStrategy creates a new deployment strategy. tags are
	// applied inline at creation time (see CreateApplication).
	CreateDeploymentStrategy(
		name, description string,
		deploymentDuration, bakeTime int32,
		growthFactor float32,
		growthType, replicateTo string,
		tags map[string]string,
	) (*DeploymentStrategy, error)
	// GetDeploymentStrategy retrieves a deployment strategy by ID.
	GetDeploymentStrategy(strategyID string) (*DeploymentStrategy, error)
	// ListDeploymentStrategies returns paginated deployment strategies.
	ListDeploymentStrategies(nextToken string, maxResults int) ([]DeploymentStrategy, string)
	// UpdateDeploymentStrategy updates a deployment strategy. A nil
	// description leaves the field unchanged (real
	// UpdateDeploymentStrategyInput.Description is an optional *string
	// member); name has no counterpart in the real API and is applied only
	// when non-empty, matching this backend's pre-existing behavior.
	UpdateDeploymentStrategy(
		strategyID, name string,
		description *string,
		deploymentDuration, bakeTime int32,
		growthFactor float32,
	) (*DeploymentStrategy, error)
	// DeleteDeploymentStrategy deletes a deployment strategy.
	DeleteDeploymentStrategy(strategyID string) error

	// StartDeployment starts a deployment. See its doc comment in
	// deployments.go for kmsKeyIdentifier/latestDeploymentNumber/tags
	// semantics.
	StartDeployment(
		applicationID, environmentID, configProfileID, strategyID, configVersion, description string,
		kmsKeyIdentifier *string,
		latestDeploymentNumber *int32,
		tags map[string]string,
	) (*Deployment, error)
	// GetDeployment retrieves a deployment by application, environment, and deployment number.
	GetDeployment(applicationID, environmentID string, deploymentNumber int32) (*Deployment, error)
	// ListDeployments returns paginated deployments for an environment.
	ListDeployments(
		applicationID, environmentID, nextToken string,
		maxResults int,
	) ([]Deployment, string, error)
	// StopDeployment stops an in-progress deployment, or -- when
	// allowRevert is true and the deployment is already COMPLETE --
	// reverts the environment to the previous configuration version
	// (real StopDeploymentInput.AllowRevert semantics).
	StopDeployment(
		applicationID, environmentID string, deploymentNumber int32, allowRevert bool,
	) (*Deployment, error)

	// ListTagsForResource returns the tags for a resource by ARN.
	ListTagsForResource(resourceArn string) (map[string]string, error)
	// TagResource adds or updates tags on a resource.
	TagResource(resourceArn string, tags map[string]string) error
	// UntagResource removes tags from a resource.
	UntagResource(resourceArn string, tagKeys []string) error

	// CreateExtension creates a new AppConfig extension. tags are applied
	// inline at creation time (see CreateApplication).
	CreateExtension(
		name, description string,
		actions map[string][]ExtensionAction,
		parameters map[string]ExtensionParameter,
		tags map[string]string,
	) (*Extension, error)
	// GetExtension retrieves an extension by identifier (ID or name) and
	// optional version number (0 means unspecified: the highest version).
	GetExtension(extensionIdentifier string, versionNumber int32) (*Extension, error)
	// ListExtensions returns paginated extensions, optionally filtered by name.
	ListExtensions(
		nextToken string,
		maxResults int,
		nameFilter string,
	) ([]Extension, string)
	// UpdateExtension updates an extension's description, actions, and
	// parameters. A nil description leaves the field unchanged (real
	// UpdateExtensionInput.Description is an optional *string member).
	UpdateExtension(
		extensionIdentifier string,
		description *string,
		actions map[string][]ExtensionAction,
		parameters map[string]ExtensionParameter,
	) (*Extension, error)
	// DeleteExtension deletes an extension version by identifier (ID or
	// name) and optional version number (0 means unspecified: the highest
	// version).
	DeleteExtension(extensionIdentifier string, versionNumber int32) error

	// CreateExtensionAssociation creates an association between an
	// extension and a resource. tags are applied inline at creation time
	// (see CreateApplication).
	CreateExtensionAssociation(
		extensionIdentifier, resourceIdentifier string,
		parameters map[string]string,
		extensionVersionNumber *int32,
		tags map[string]string,
	) (*ExtensionAssociation, error)
	// GetExtensionAssociation retrieves an extension association by ID.
	GetExtensionAssociation(extensionAssociationID string) (*ExtensionAssociation, error)
	// ListExtensionAssociations returns paginated extension associations.
	ListExtensionAssociations(
		nextToken, extensionIdentifier, resourceIdentifier string,
		extensionVersionNumber int32,
		maxResults int,
	) ([]ExtensionAssociation, string)
	// UpdateExtensionAssociation updates an extension association's parameters.
	UpdateExtensionAssociation(
		extensionAssociationID string,
		parameters map[string]string,
	) (*ExtensionAssociation, error)
	// DeleteExtensionAssociation deletes an extension association by ID.
	DeleteExtensionAssociation(extensionAssociationID string) error

	// GetAccountSettings returns the account-level AppConfig settings.
	GetAccountSettings() (*AccountSettings, error)
	// UpdateAccountSettings updates account-level AppConfig settings.
	UpdateAccountSettings(
		deletionProtection *DeletionProtectionSettings, vendedMetrics *VendedMetricsSettings,
	) (*AccountSettings, error)

	// GetConfiguration retrieves the latest deployed configuration (deprecated API).
	GetConfiguration(
		application, environment, configuration string,
	) (*HostedConfigurationVersion, error)
	// ValidateConfiguration validates a configuration version against its validators.
	ValidateConfiguration(applicationID, profileID, configurationVersion string) error

	// CurrentDeployedConfiguration returns the content, content type, and
	// version label of the configuration currently active (the most
	// recently COMPLETEd deployment) for the given
	// application/environment/configuration-profile, each resolved by ID
	// or name exactly like GetConfiguration. It is a public read accessor
	// for callers outside this package; the appconfig -> appconfigdata
	// bridge (bd gopherstack-uiyi, see DeployedConfigurationPublisher)
	// pushes the same data automatically as deployments complete, so this
	// accessor's own callers are external/read-path only.
	CurrentDeployedConfiguration(
		application, environment, configuration string,
	) (content []byte, contentType, versionLabel string, err error)

	// CreateExperimentDefinition creates a new experiment definition
	// attached to a feature-flag configuration profile. applicationIdentifier,
	// environmentIdentifier, and configurationProfileIdentifier are each
	// resolved by ID or name and validated against real backend state.
	// control must be non-nil with a non-nil FlagValue; every element of
	// treatments must likewise carry a non-nil FlagValue. Key on both is
	// server-generated (see the Treatment doc comment in models.go). tags
	// are applied inline to the new definition's ARN at creation time (see
	// CreateExperimentDefinition's doc comment in experiment_definitions.go
	// for why -- avoids the inline-Tags-dropped bug tracked by bd
	// gopherstack-lcan).
	CreateExperimentDefinition(
		applicationIdentifier, name, environmentIdentifier, configurationProfileIdentifier, flagKey,
		audienceRule, audienceDescription, hypothesis, launchCriteria string,
		control *Treatment,
		treatments []Treatment,
		tags map[string]string,
	) (*ExperimentDefinition, error)
	// GetExperimentDefinition retrieves an experiment definition by
	// application and experiment definition identifier (each accepted by
	// ID or name).
	GetExperimentDefinition(
		applicationIdentifier, experimentDefinitionIdentifier string,
	) (*ExperimentDefinition, error)
	// ListExperimentDefinitions returns experiment definitions across the
	// account, optionally filtered by application/configuration-profile/
	// environment identifier and status. An identifier filter that cannot
	// be resolved yields an empty result (a filter with no matches), not
	// an error.
	ListExperimentDefinitions(
		applicationIdentifier, configurationProfileIdentifier, environmentIdentifier, status, nextToken string,
		maxResults int,
	) ([]ExperimentDefinition, string)
	// UpdateExperimentDefinition updates an experiment definition. A nil
	// pointer field means the request omitted it and it is left unchanged;
	// a non-nil treatments fully replaces the treatment list (fresh Keys
	// assigned). Returns ErrConflict if a RUNNING run currently exists for
	// this definition, matching real AWS's "cannot update ... while an
	// experiment run is active."
	UpdateExperimentDefinition(
		applicationIdentifier, experimentDefinitionIdentifier string,
		audienceDescription, audienceRule *string,
		control *Treatment,
		hypothesis, launchCriteria *string,
		treatments *[]Treatment,
	) (*ExperimentDefinition, error)
	// DeleteExperimentDefinition archives (deleteType "ARCHIVE", the
	// default this backend applies when deleteType is empty -- see the doc
	// comment in experiment_definitions.go for why) or permanently
	// destroys (deleteType "DESTROY") an experiment definition. DESTROY
	// cascade-deletes every run, run event, and tag scoped to it.
	DeleteExperimentDefinition(applicationIdentifier, experimentDefinitionIdentifier, deleteType string) error

	// StartExperimentRun starts a new run of an experiment definition.
	// Only one run may be RUNNING per definition at a time (ErrConflict
	// otherwise). exposurePercentage nil defaults to 0 -- see the doc
	// comment in experiment_runs.go. tags are applied inline to the new
	// run's own ARN, same inline-tagging fix as CreateExperimentDefinition.
	StartExperimentRun(
		applicationIdentifier, experimentDefinitionIdentifier, description string,
		exposurePercentage *float32,
		treatmentOverrides map[string]string,
		tags map[string]string,
	) (*ExperimentRun, error)
	// GetExperimentRun retrieves an experiment run by application,
	// experiment definition identifier, and run number.
	GetExperimentRun(
		applicationIdentifier, experimentDefinitionIdentifier string, run int32,
	) (*ExperimentRun, error)
	// ListExperimentRuns returns paginated runs for an experiment
	// definition, optionally filtered by status.
	ListExperimentRuns(
		applicationIdentifier, experimentDefinitionIdentifier, status, nextToken string,
		maxResults int,
	) ([]ExperimentRun, string, error)
	// UpdateExperimentRun updates a RUNNING experiment run's description,
	// exposure percentage (which can only increase, matching real AWS),
	// and/or treatment overrides. A nil field is left unchanged. Returns
	// ErrBadRequest if the run is not RUNNING or the new exposure
	// percentage would decrease it.
	UpdateExperimentRun(
		applicationIdentifier, experimentDefinitionIdentifier string,
		run int32,
		description *string,
		exposurePercentage *float32,
		treatmentOverrides *TreatmentOverrides,
	) (*ExperimentRun, error)
	// StopExperimentRun stops a RUNNING experiment run, moving it to DONE.
	// Returns ErrBadRequest if the run is not currently RUNNING.
	StopExperimentRun(
		applicationIdentifier, experimentDefinitionIdentifier string,
		run int32,
		result *ExperimentRunResult,
	) (*ExperimentRun, error)
	// ListExperimentRunEvents returns the events this backend actually
	// recorded during the run's lifecycle (RUN_STARTED/EXPOSURE_UPDATED/
	// OVERRIDES_UPDATED/RUN_STOPPED), most-recent-first -- never a
	// fabricated timeline.
	ListExperimentRunEvents(
		applicationIdentifier, experimentDefinitionIdentifier string,
		run int32,
		nextToken string,
		maxResults int,
	) ([]ExperimentRunEvent, string, error)
}
