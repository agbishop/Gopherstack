package fis

import (
	"context"
	"crypto/rand"
	"maps"
	"math/big"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	statusDisengaged = "disengaged"
)

// ----------------------------------------
// Status constants
// ----------------------------------------

// Experiment status values. These match types.ExperimentStatus's enum exactly
// (pending, initiating, running, completed, stopping, stopped, failed,
// cancelled) -- there is deliberately no "completing" value: an earlier
// gopherstack revision invented one that does not exist in the real AWS FIS
// SDK model, and it has been removed (see runExperiment).
const (
	statusPending    = "pending"
	statusInitiating = "initiating"
	statusRunning    = "running"
	statusStopping   = "stopping"
	statusStopped    = "stopped"
	statusCompleted  = "completed"
	statusCancelled  = "cancelled"
	statusFailed     = "failed"
)

// Action status values, matching types.ExperimentActionStatus's enum exactly
// (pending, initiating, running, completed, cancelled, stopping, stopped,
// failed, skipped). As with the experiment-level status above, there is no
// "completing" value.
const (
	actionStatusPending    = "pending"
	actionStatusInitiating = "initiating"
	actionStatusRunning    = "running"
	actionStatusCompleted  = "completed"
	actionStatusStopped    = "stopped"
	actionStatusFailed     = "failed"
	actionStatusCancelled  = "cancelled"
	actionStatusSkipped    = "skipped"
)

// Experiment report status values, matching types.ExperimentReportStatus's enum
// exactly (pending, running, completed, cancelled, failed).
const (
	experimentReportStatusPending   = "pending"
	experimentReportStatusCompleted = "completed"
	experimentReportStatusCancelled = "cancelled"
	experimentReportStatusFailed    = "failed"
)

// Actions mode values, matching types.ActionsMode's enum exactly.
const (
	actionsModeRunAll  = "run-all"
	actionsModeSkipAll = "skip-all"
)

// stopConditionSourceAlarm is the CreateExperimentTemplateStopConditionInput
// Source value naming a CloudWatch alarm (aws-sdk-go-v2/service/fis/types:
// "Specify aws:cloudwatch:alarm if the stop condition is defined by a
// CloudWatch alarm"). alarmStateValueAlarm mirrors CloudWatch's own "ALARM"
// state string; FIS does not import cloudwatch, so this is a local literal
// copy of stable AWS wire vocabulary rather than a shared constant.
const (
	stopConditionSourceAlarm = "aws:cloudwatch:alarm"
	alarmStateValueAlarm     = "ALARM"
)

// ----------------------------------------
// ID / ARN helpers
// ----------------------------------------

const idChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// idTotalLength is the total length of generated IDs including the prefix.
// AWS FIS uses 16-character IDs (e.g., "EXT2zP9aBcDeFgHi").
const idTotalLength = 16

// maxExperiments is the maximum number of experiments that can exist concurrently.
const maxExperiments = 1000

// maxTagsPerResource is the maximum number of tags allowed per resource.
const maxTagsPerResource = 50

// maxTagKeyLen / maxTagValueLen are the AWS tag size limits.
const (
	maxTagKeyLen   = 128
	maxTagValueLen = 256
)

// generateID creates a random ID with the given prefix so that total length == idTotalLength.
func generateID(prefix string) string {
	length := idTotalLength - len(prefix)
	length = max(length, 1)

	b := make([]byte, length)

	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(idChars))))
		if err != nil {
			b[i] = idChars[0]

			continue
		}

		b[i] = idChars[n.Int64()]
	}

	return prefix + string(b)
}

// toUnix converts a [time.Time] to the epoch-seconds wire format the restjson1
// protocol expects for FIS timestamps (see pkgs/awstime.Epoch).
func toUnix(t time.Time) float64 {
	return awstime.Epoch(t)
}

func toUnixPtr(t *time.Time) *float64 {
	if t == nil {
		return nil
	}

	v := toUnix(*t)

	return &v
}

// ----------------------------------------
// InMemoryBackend implementation
// ----------------------------------------

// InMemoryBackend is the in-memory implementation of StorageBackend.
type InMemoryBackend struct {
	svcCtx context.Context
	// alarmSubscriber drives "aws:cloudwatch:alarm" stop conditions
	// (gopherstack-x842, gopherstack-9939). Nil (the default) leaves stop
	// conditions validated and stored but otherwise inert.
	alarmSubscriber                AlarmStateSubscriber
	experimentsByArn               *store.Index[Experiment]
	faultStore                     *chaos.FaultStore
	registry                       *store.Registry
	targetAccountConfigs           *store.Table[TargetAccountConfiguration]
	targetAccountConfigsByTemplate *store.Index[TargetAccountConfiguration]
	tplClientTokens                map[string]string // clientToken → templateID
	expClientTokens                map[string]string // clientToken → experimentID
	experiments                    *store.Table[Experiment]
	safetyLever                    *SafetyLever
	mu                             *lockmetrics.RWMutex
	templatesByArn                 *store.Index[ExperimentTemplate]
	templates                      *store.Table[ExperimentTemplate]
	region                         string
	accountID                      string
	actionProviders                []service.FISActionProvider
}

// NewInMemoryBackend creates a new InMemoryBackend with a background service context.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	return NewInMemoryBackendWithContext(context.Background(), accountID, region)
}

// NewInMemoryBackendWithContext creates a new InMemoryBackend whose experiment goroutines
// are parented by svcCtx so they are cancelled on server shutdown. If svcCtx is nil,
// [context.Background] is used.
func NewInMemoryBackendWithContext(svcCtx context.Context, accountID, region string) *InMemoryBackend {
	if svcCtx == nil {
		svcCtx = context.Background()
	}

	safetyLeverARN := arn.Build("fis", region, accountID, "safety-lever/"+accountID)

	b := &InMemoryBackend{
		registry:        store.NewRegistry(),
		tplClientTokens: make(map[string]string),
		expClientTokens: make(map[string]string),
		accountID:       accountID,
		region:          region,
		mu:              lockmetrics.New("fis"),
		svcCtx:          svcCtx,
		safetyLever: &SafetyLever{
			ID:    accountID,
			Arn:   safetyLeverARN,
			Tags:  make(map[string]string),
			State: SafetyLeverState{Status: statusDisengaged},
		},
	}

	registerAllTables(b)

	return b
}

// Reset clears all in-memory state, cancelling any running experiments.
// The safety lever is re-initialised to its default disengaged state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	for _, exp := range b.experiments.All() {
		if exp.cancel != nil {
			exp.cancel()
		}
	}

	safetyLeverARN := arn.Build("fis", b.region, b.accountID, "safety-lever/"+b.accountID)

	b.registry.ResetAll()
	b.tplClientTokens = make(map[string]string)
	b.expClientTokens = make(map[string]string)
	b.safetyLever = &SafetyLever{
		ID:    b.accountID,
		Arn:   safetyLeverARN,
		Tags:  make(map[string]string),
		State: SafetyLeverState{Status: statusDisengaged},
	}
}

// SetFaultStore injects the chaos FaultStore.
func (b *InMemoryBackend) SetFaultStore(store *chaos.FaultStore) {
	b.mu.Lock("SetFaultStore")
	defer b.mu.Unlock()

	b.faultStore = store
}

// SetAlarmStateSubscriber registers the CloudWatch alarm-state-change hook that
// drives "aws:cloudwatch:alarm" stop conditions (gopherstack-x842,
// gopherstack-9939).
func (b *InMemoryBackend) SetAlarmStateSubscriber(sub AlarmStateSubscriber) {
	b.mu.Lock("SetAlarmStateSubscriber")
	defer b.mu.Unlock()

	b.alarmSubscriber = sub
}

// SetActionProviders registers external FIS action providers discovered from the registry.
func (b *InMemoryBackend) SetActionProviders(providers []service.FISActionProvider) {
	b.mu.Lock("SetActionProviders")
	defer b.mu.Unlock()

	b.actionProviders = providers
}

// ----------------------------------------
// Generic copy helpers
// ----------------------------------------

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}

	out := make(map[string]string, len(m))
	maps.Copy(out, m)

	return out
}
