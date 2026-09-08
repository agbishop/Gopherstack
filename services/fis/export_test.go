package fis

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// DefaultJanitorInterval exposes the package default janitor interval for testing.
const DefaultJanitorInterval = defaultFISJanitorInterval

// LifecycleDelayForTest exposes lifecycleDelay so tests can advance a synctest
// fake clock past a lifecycle transition without hardcoding the real duration.
const LifecycleDelayForTest = lifecycleDelay

// DefaultExperimentTTL exposes the package default experiment TTL for testing.
const DefaultExperimentTTL = defaultFISExperimentTTL

// ExportedInMemoryBackend exposes the InMemoryBackend for tests.
type ExportedInMemoryBackend = InMemoryBackend

// ExperimentTemplateActionForTest exposes ExperimentTemplateAction for tests.
type ExperimentTemplateActionForTest = ExperimentTemplateAction

// ErrResourceNotFoundForTest exposes ErrResourceNotFound for tests.
var ErrResourceNotFoundForTest = ErrResourceNotFound

// NewTestBackend creates a new InMemoryBackend for testing.
func NewTestBackend() *InMemoryBackend {
	return NewInMemoryBackend("000000000000", "us-east-1")
}

// ParseISODurationForTest exposes parseISODuration for testing.
func ParseISODurationForTest(s string) time.Duration {
	return parseISODuration(s)
}

// ParsePercentageForTest exposes parsePercentage for testing.
func ParsePercentageForTest(s string) float64 {
	return parsePercentage(s)
}

// ParseOperationsForTest exposes parseOperations for testing.
func ParseOperationsForTest(s string) []string {
	return parseOperations(s)
}

// FaultErrorForActionForTest exposes faultErrorForAction for testing.
func FaultErrorForActionForTest(actionID string) chaos.FaultError {
	return faultErrorForAction(actionID)
}

// BuildFaultRulesForTest exposes buildFaultRules for testing.
func BuildFaultRulesForTest(action ExperimentTemplateAction) []chaos.FaultRule {
	return buildFaultRules(action)
}

// ApplySelectionModeForTest exposes applySelectionMode for testing.
func ApplySelectionModeForTest(arns []string, mode string) []string {
	return applySelectionMode(arns, mode)
}

// CreateTestEchoForExtract creates an echo.Context for testing ExtractOperation/ExtractResource.
func CreateTestEchoForExtract(t *testing.T, _ *Handler, method, path string) *echo.Context {
	t.Helper()

	e := echo.New()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	return c
}

// MockFISActionProvider is a test-only FISActionProvider that returns a configurable error.
type MockFISActionProvider struct {
	ExecErr     error
	Definitions []service.FISActionDefinition
	Execs       []service.FISActionExecution
	Calls       int
}

func (m *MockFISActionProvider) FISActions() []service.FISActionDefinition {
	return m.Definitions
}

func (m *MockFISActionProvider) ExecuteFISAction(_ context.Context, exec service.FISActionExecution) error {
	m.Calls++
	m.Execs = append(m.Execs, exec)

	return m.ExecErr
}

// ErrMockAction is a sentinel error for mock action failures.
var ErrMockAction = errors.New("mock action failed")

// SetExperimentTerminal sets an experiment to a terminal state with a specific end time,
// cancelling the background goroutine to prevent races.
// Used only in tests.
func (b *InMemoryBackend) SetExperimentTerminal(id, status string, endTime time.Time) {
	b.mu.Lock("SetExperimentTerminal")

	exp, ok := b.experiments.Get(id)
	if !ok {
		b.mu.Unlock()

		return
	}

	// Cancel the background goroutine before mutating so it cannot race
	// and overwrite the forced terminal values.
	if exp.cancel != nil {
		exp.cancel()
	}

	exp.Status = ExperimentStatus{Status: status}
	exp.EndTime = &endTime

	b.mu.Unlock()
}

// InjectExperiment inserts a pre-built experiment directly into the store without
// starting a background goroutine. This allows tests to create experiments in arbitrary
// terminal states without racing against the normal lifecycle goroutine.
// Used only in tests.
func (b *InMemoryBackend) InjectExperiment(exp *Experiment) {
	b.mu.Lock("InjectExperiment")
	defer b.mu.Unlock()

	b.experiments.Put(exp)
}

// ExperimentCount returns the number of experiments stored.
// Used only in tests.
func (b *InMemoryBackend) ExperimentCount() int {
	b.mu.RLock("ExperimentCount")
	defer b.mu.RUnlock()

	return b.experiments.Len()
}

// InjectTemplate inserts a pre-built experiment template directly into the store.
// Used only in tests.
func (b *InMemoryBackend) InjectTemplate(tpl *ExperimentTemplate) {
	b.mu.Lock("InjectTemplate")
	defer b.mu.Unlock()

	b.templates.Put(tpl)
}

// TemplateCount returns the number of experiment templates stored.
// Used only in tests.
func (b *InMemoryBackend) TemplateCount() int {
	b.mu.RLock("TemplateCount")
	defer b.mu.RUnlock()

	return b.templates.Len()
}

// InjectCancel injects a cancel function for an experiment, simulating the one
// stored when StartExperiment creates a background goroutine.
// Used only in tests.
func (b *InMemoryBackend) InjectCancel(id string, cancel context.CancelFunc) {
	b.mu.Lock("InjectCancel")
	defer b.mu.Unlock()

	if exp, ok := b.experiments.Get(id); ok {
		exp.cancel = cancel
	}
}

// GetJanitorTaskTimeout returns the TaskTimeout configured on the handler's janitor.
// Used in tests to verify WithJanitor correctly propagates the timeout.
func (h *Handler) GetJanitorTaskTimeout() time.Duration {
	if h.janitor == nil {
		return 0
	}

	return h.janitor.TaskTimeout
}

// GetJanitorInterval returns the Interval configured on the handler's janitor.
// Used in tests to verify WithJanitor correctly propagates the interval.
func (h *Handler) GetJanitorInterval() time.Duration {
	if h.janitor == nil {
		return 0
	}

	return h.janitor.Interval
}

// GetJanitorExperimentTTL returns the ExperimentTTL configured on the handler's janitor.
// Used in tests to verify WithJanitor correctly propagates the TTL.
func (h *Handler) GetJanitorExperimentTTL() time.Duration {
	if h.janitor == nil {
		return 0
	}

	return h.janitor.ExperimentTTL
}

// AddTemplateInternal inserts a pre-built experiment template with a deep copy.
// Used only in tests as a seed helper.
func (b *InMemoryBackend) AddTemplateInternal(tpl *ExperimentTemplate) {
	b.mu.Lock("AddTemplateInternal")
	defer b.mu.Unlock()

	cp := cloneTemplate(tpl)
	b.templates.Put(cp)
}

// AddTargetAccountConfigInternal inserts a target account configuration for a template.
// The template must already exist. Used only in tests as a seed helper.
func (b *InMemoryBackend) AddTargetAccountConfigInternal(cfg *TargetAccountConfiguration) {
	b.mu.Lock("AddTargetAccountConfigInternal")
	defer b.mu.Unlock()

	cp := *cfg
	b.targetAccountConfigs.Put(&cp)
}

// TargetAccountConfigCount returns the total number of target account configurations across all templates.
// Used only in tests.
func (b *InMemoryBackend) TargetAccountConfigCount() int {
	b.mu.RLock("TargetAccountConfigCount")
	defer b.mu.RUnlock()

	return b.targetAccountConfigs.Len()
}

// HandlerOpsLen returns 0 because the FIS handler uses a switch-based dispatch, not a map.
// Used in tests to maintain consistency with other services.
func (h *Handler) HandlerOpsLen() int {
	return len(h.GetSupportedOperations())
}
