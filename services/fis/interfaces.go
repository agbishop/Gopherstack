package fis

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// ----------------------------------------
// Compile-time interface check
// ----------------------------------------

var _ StorageBackend = (*InMemoryBackend)(nil)

// ----------------------------------------
// AlarmStateSubscriber interface
// ----------------------------------------

// AlarmStateSubscriber lets FIS subscribe to a CloudWatch alarm's state changes
// so an "aws:cloudwatch:alarm" stop condition can react directly to it instead
// of polling (gopherstack-x842, gopherstack-9939). FIS does not import
// cloudwatch; cli.go wires a collaborator satisfying this interface in.
type AlarmStateSubscriber interface {
	// SubscribeAlarmStateChange registers cb to run whenever the alarm
	// identified by alarmArn changes state, and returns an unsubscribe func.
	SubscribeAlarmStateChange(alarmArn string, cb func(newState string)) (unsubscribe func())
}

// ----------------------------------------
// StorageBackend interface
// ----------------------------------------

// StorageBackend is the interface implemented by the FIS in-memory store.
type StorageBackend interface {
	// Template operations
	CreateExperimentTemplate(
		input *createExperimentTemplateRequest,
		accountID, region string,
	) (*ExperimentTemplate, error)
	GetExperimentTemplate(id string) (*ExperimentTemplate, error)
	UpdateExperimentTemplate(id string, input *updateExperimentTemplateRequest) (*ExperimentTemplate, error)
	DeleteExperimentTemplate(id string) error
	ListExperimentTemplates() ([]*ExperimentTemplate, error)

	// Experiment operations
	StartExperiment(ctx context.Context, input *startExperimentRequest, accountID, region string) (*Experiment, error)
	GetExperiment(id string) (*Experiment, error)
	StopExperiment(id string) (*Experiment, error)
	ListExperiments() ([]*Experiment, error)

	// Phase 3 — resolved targets
	ListExperimentResolvedTargets(id string) ([]ExperimentResolvedTarget, error)

	// Phase 3 — safety lever
	GetSafetyLever(id string) (*SafetyLever, error)
	UpdateSafetyLeverState(id string, input *updateSafetyLeverStateRequest) (*SafetyLever, error)

	// Action / target-resource-type discovery
	ListActions() []ActionSummary
	GetAction(id string) (*ActionSummary, error)
	ListTargetResourceTypes() []TargetResourceTypeSummary
	GetTargetResourceType(resourceType string) (*TargetResourceTypeSummary, error)

	// Tag operations
	ListTagsForResource(resourceARN string) (map[string]string, error)
	TagResource(resourceARN string, tags map[string]string) error
	UntagResource(resourceARN string, keys []string) error

	// Target account configuration operations (template-scoped)
	CreateTargetAccountConfiguration(
		templateID, accountID, roleArn, description string,
	) (*TargetAccountConfiguration, error)
	DeleteTargetAccountConfiguration(templateID, accountID string) (*TargetAccountConfiguration, error)
	GetTargetAccountConfiguration(templateID, accountID string) (*TargetAccountConfiguration, error)
	UpdateTargetAccountConfiguration(
		templateID, accountID string,
		roleArn, description *string,
	) (*TargetAccountConfiguration, error)
	ListTargetAccountConfigurations(templateID string) ([]*TargetAccountConfiguration, error)

	// Experiment target account configuration operations (experiment-scoped, read-only)
	GetExperimentTargetAccountConfiguration(
		experimentID, accountID string,
	) (*ExperimentTargetAccountConfiguration, error)
	ListExperimentTargetAccountConfigurations(experimentID string) ([]*ExperimentTargetAccountConfiguration, error)

	// SetFaultStore injects the chaos FaultStore used for inject-api-* actions.
	SetFaultStore(store *chaos.FaultStore)

	// SetActionProviders registers external service action providers.
	SetActionProviders(providers []service.FISActionProvider)

	// SetAlarmStateSubscriber registers the CloudWatch alarm-state-change hook
	// that drives "aws:cloudwatch:alarm" stop conditions.
	SetAlarmStateSubscriber(sub AlarmStateSubscriber)
}
