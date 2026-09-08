package fis

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// ----------------------------------------
// Experiment lifecycle
// ----------------------------------------

// resolveActionsMode validates and resolves the experiment options' actions mode,
// defaulting to "run-all" when the caller omits experimentOptions entirely (real
// AWS FIS behaviour: skip-all is opt-in for dry-run validation of an experiment's
// configuration without injecting faults).
func resolveActionsMode(opts *startExperimentExperimentOptionsDTO) (string, error) {
	if opts == nil || opts.ActionsMode == "" {
		return actionsModeRunAll, nil
	}

	switch opts.ActionsMode {
	case actionsModeRunAll, actionsModeSkipAll:
		return opts.ActionsMode, nil
	default:
		return "", fmt.Errorf(
			"%w: experimentOptions.actionsMode must be %q or %q; got %q",
			ErrValidation, actionsModeRunAll, actionsModeSkipAll, opts.ActionsMode,
		)
	}
}

// StartExperiment creates and starts a new experiment from a template.
func (b *InMemoryBackend) StartExperiment(
	_ context.Context,
	input *startExperimentRequest,
	accountID, region string,
) (*Experiment, error) {
	// Check clientToken idempotency first (read-only fast path).
	if input.ClientToken != "" {
		b.mu.RLock("StartExperiment-idempotency")
		existingID, ok := b.expClientTokens[input.ClientToken]
		b.mu.RUnlock()

		if ok {
			return b.GetExperiment(existingID)
		}
	}

	actionsMode, err := resolveActionsMode(input.ExperimentOptions)
	if err != nil {
		return nil, err
	}

	id := generateID("EXP")
	arnStr := arn.Build("fis", region, accountID, "experiment/"+id)

	// expCtx derives from b.svcCtx — NOT the HTTP request context — so the experiment
	// goroutine is not cancelled when the HTTP response is sent, but IS cancelled on shutdown.
	expCtx, cancel := context.WithCancel(b.svcCtx)

	var (
		snapshot  *Experiment
		tplForRun *ExperimentTemplate
	)

	// The lever-engaged / quota / template-lookup checks and the Put that follows
	// them must happen under a single write lock: reading them under an RLock and
	// re-locking to write (the previous approach) left a TOCTOU window where two
	// concurrent StartExperiment calls could both observe experimentCount <
	// maxExperiments before either had written its new experiment.
	startErr := func() error {
		b.mu.Lock("StartExperiment")
		defer b.mu.Unlock()

		if b.safetyLever != nil && b.safetyLever.State.Status == "engaged" {
			return fmt.Errorf("%w: safety lever is engaged", ErrSafetyLeverEngaged)
		}

		if b.experiments.Len() >= maxExperiments {
			return fmt.Errorf(
				"%w: experiment count would exceed the limit of %d",
				ErrTooManyExperiments,
				maxExperiments,
			)
		}

		tpl, ok := b.templates.Get(input.ExperimentTemplateID)
		if !ok {
			return fmt.Errorf("%w: %s", ErrTemplateNotFound, input.ExperimentTemplateID)
		}

		tplAccountCount := len(b.targetAccountConfigsByTemplate.Get(input.ExperimentTemplateID))

		exp := buildExperimentFromTemplate(id, arnStr, tpl, input.Tags, actionsMode, cancel)
		exp.TargetAccountConfigurationsCount = tplAccountCount

		// Clone the template BEFORE passing to the goroutine so template updates don't race.
		tplForRun = cloneTemplate(tpl)

		b.experiments.Put(exp)

		if input.ClientToken != "" {
			b.expClientTokens[input.ClientToken] = id
		}

		// Take the snapshot while holding the lock, before launching the goroutine,
		// so the background goroutine cannot mutate exp while we're reading it.
		snapshot = cloneExperiment(exp)

		return nil
	}()
	if startErr != nil {
		cancel() // release the context created above; no goroutine will consume it

		return nil, startErr
	}

	// Run the experiment lifecycle in the background.
	go b.runExperiment(expCtx, id, tplForRun, actionsMode)

	return snapshot, nil
}

// buildExperimentTargets converts template targets into their experiment-scoped
// equivalent, carrying Filters/ResourceTags/SelectionMode through as
// informational metadata alongside the (not-yet-resolved) ResourceArns —
// matching the real AWS FIS wire shape (types.ExperimentTarget).
func buildExperimentTargets(tplTargets map[string]ExperimentTemplateTarget) map[string]ExperimentTarget {
	targets := make(map[string]ExperimentTarget, len(tplTargets))

	for name, t := range tplTargets {
		filters := make([]ExperimentTemplateTargetFilter, len(t.Filters))
		for i, f := range t.Filters {
			filters[i] = ExperimentTemplateTargetFilter{
				Path:   f.Path,
				Values: append([]string(nil), f.Values...),
			}
		}

		targets[name] = ExperimentTarget{
			ResourceType:  t.ResourceType,
			SelectionMode: t.SelectionMode,
			ResourceArns:  append([]string(nil), t.ResourceArns...),
			ResourceTags:  copyStringMap(t.ResourceTags),
			Filters:       filters,
			Parameters:    copyStringMap(t.Parameters),
		}
	}

	return targets
}

// buildExperimentActions converts template actions into their experiment-scoped
// equivalent, all initialised to pending.
func buildExperimentActions(tplActions map[string]ExperimentTemplateAction) map[string]ExperimentAction {
	actions := make(map[string]ExperimentAction, len(tplActions))

	for name, a := range tplActions {
		actions[name] = ExperimentAction{
			ActionID:    a.ActionID,
			Description: a.Description,
			Parameters:  copyStringMap(a.Parameters),
			Targets:     copyStringMap(a.Targets),
			StartAfter:  append([]string(nil), a.StartAfter...),
			Status:      ExperimentActionStatus{Status: actionStatusPending},
		}
	}

	return actions
}

// buildExperimentFromTemplate constructs a new Experiment from a template, input
// tags, and the resolved actions mode.
func buildExperimentFromTemplate(
	id, arnStr string,
	tpl *ExperimentTemplate,
	inputTags map[string]string,
	actionsMode string,
	cancel context.CancelFunc,
) *Experiment {
	stopConditions := make([]ExperimentStopCondition, len(tpl.StopConditions))
	for i, sc := range tpl.StopConditions {
		stopConditions[i] = ExperimentStopCondition(sc)
	}

	logConfig := copyLogConfiguration(tpl.LogConfiguration)

	expOptions := &ExperimentExperimentOptions{ActionsMode: actionsMode}
	if tpl.ExperimentOptions != nil {
		expOptions.AccountTargeting = tpl.ExperimentOptions.AccountTargeting
		expOptions.EmptyTargetResolutionMode = tpl.ExperimentOptions.EmptyTargetResolutionMode
	}

	reportConfig := copyReportConfigForExperiment(tpl.ExperimentReportConfiguration)

	var report *ExperimentReport
	if reportConfig != nil {
		report = &ExperimentReport{State: &ExperimentReportState{Status: experimentReportStatusPending}}
	}

	// expCtx derives from svcCtx — NOT the HTTP request context — so the experiment
	// goroutine is not cancelled when the HTTP response is sent.
	// cancel is passed in from StartExperiment and stored on the returned experiment.

	now := time.Now()

	return &Experiment{
		ID:                            id,
		Arn:                           arnStr,
		ExperimentTemplateID:          tpl.ID,
		RoleArn:                       tpl.RoleArn,
		Status:                        ExperimentStatus{Status: statusPending},
		Targets:                       buildExperimentTargets(tpl.Targets),
		Actions:                       buildExperimentActions(tpl.Actions),
		StopConditions:                stopConditions,
		LogConfiguration:              logConfig,
		ExperimentOptions:             expOptions,
		ExperimentReportConfiguration: reportConfig,
		ExperimentReport:              report,
		Tags:                          copyStringMap(inputTags),
		CreationTime:                  now,
		StartTime:                     now,
		cancel:                        cancel,
	}
}

// copyReportConfigForExperiment deep-copies a template's report configuration
// into the experiment-scoped equivalent shape (identical wire fields, distinct
// Go types mirroring the ExperimentTemplateReportConfiguration /
// ExperimentReportConfiguration split in the real AWS FIS SDK).
func copyReportConfigForExperiment(cfg *ExperimentTemplateReportConfiguration) *ExperimentReportConfiguration {
	if cfg == nil {
		return nil
	}

	out := &ExperimentReportConfiguration{
		PreExperimentDuration:  cfg.PreExperimentDuration,
		PostExperimentDuration: cfg.PostExperimentDuration,
	}

	if cfg.DataSources != nil {
		dashboards := make(
			[]ExperimentReportConfigurationCloudWatchDashboard,
			len(cfg.DataSources.CloudWatchDashboards),
		)
		for i, d := range cfg.DataSources.CloudWatchDashboards {
			dashboards[i] = ExperimentReportConfigurationCloudWatchDashboard(d)
		}

		out.DataSources = &ExperimentReportConfigurationDataSources{CloudWatchDashboards: dashboards}
	}

	if cfg.Outputs != nil && cfg.Outputs.S3Configuration != nil {
		out.Outputs = &ExperimentReportConfigurationOutputs{
			S3Configuration: &ExperimentReportConfigurationOutputsS3Configuration{
				BucketName: cfg.Outputs.S3Configuration.BucketName,
				Prefix:     cfg.Outputs.S3Configuration.Prefix,
			},
		}
	}

	return out
}

// copyLogConfiguration deep-copies a template log configuration into its experiment equivalent.
func copyLogConfiguration(tplLog *ExperimentTemplateLogConfiguration) *ExperimentLogConfiguration {
	if tplLog == nil {
		return nil
	}

	lc := &ExperimentLogConfiguration{LogSchemaVersion: tplLog.LogSchemaVersion}

	if tplLog.CloudWatchLogsConfiguration != nil {
		lc.CloudWatchLogsConfiguration = &ExperimentCloudWatchLogsConfiguration{
			LogGroupArn: tplLog.CloudWatchLogsConfiguration.LogGroupArn,
		}
	}

	if tplLog.S3Configuration != nil {
		lc.S3Configuration = &ExperimentS3Configuration{
			BucketName: tplLog.S3Configuration.BucketName,
			Prefix:     tplLog.S3Configuration.Prefix,
		}
	}

	return lc
}

// GetExperiment retrieves an experiment by ID.
func (b *InMemoryBackend) GetExperiment(id string) (*Experiment, error) {
	b.mu.RLock("GetExperiment")
	defer b.mu.RUnlock()

	exp, ok := b.experiments.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrExperimentNotFound, id)
	}

	return cloneExperiment(exp), nil
}

// StopExperiment stops a running experiment.
func (b *InMemoryBackend) StopExperiment(id string) (*Experiment, error) {
	var lockErr error
	var snap *Experiment

	func() {
		b.mu.Lock("StopExperiment")
		defer b.mu.Unlock()

		exp, ok := b.experiments.Get(id)
		if !ok {
			lockErr = fmt.Errorf("%w: %s", ErrExperimentNotFound, id)

			return
		}

		s := exp.Status.Status
		if s != statusPending && s != statusInitiating && s != statusRunning {
			lockErr = fmt.Errorf("%w: %s", ErrExperimentNotRunning, id)

			return
		}

		// Signal the background goroutine to stop.
		if exp.cancel != nil {
			exp.cancel()
		}

		// Immediately reflect the transition to stopping in the response.
		exp.Status = ExperimentStatus{Status: statusStopping}

		snap = cloneExperiment(exp)
	}()
	if lockErr != nil {
		return nil, lockErr
	}

	return snap, nil
}

// ListExperiments returns all experiments sorted by ID.
func (b *InMemoryBackend) ListExperiments() ([]*Experiment, error) {
	b.mu.RLock("ListExperiments")
	defer b.mu.RUnlock()

	all := b.experiments.All()
	result := make([]*Experiment, 0, len(all))

	for _, exp := range all {
		result = append(result, cloneExperiment(exp))
	}

	slices.SortFunc(result, func(a, b *Experiment) int { return strings.Compare(a.ID, b.ID) })

	return result, nil
}

// StopAllExperiments cancels every running experiment goroutine.
// Called during graceful shutdown to prevent goroutine leaks.
func (b *InMemoryBackend) StopAllExperiments() {
	b.mu.Lock("StopAllExperiments")
	defer b.mu.Unlock()

	for _, exp := range b.experiments.All() {
		if exp.cancel != nil {
			exp.cancel()
		}
	}
}

// ----------------------------------------
// Phase 3 — Resolved Targets
// ----------------------------------------

// ListExperimentResolvedTargets returns the resolved target groups for the given
// experiment.
func (b *InMemoryBackend) ListExperimentResolvedTargets(id string) ([]ExperimentResolvedTarget, error) {
	b.mu.RLock("ListExperimentResolvedTargets")
	defer b.mu.RUnlock()

	exp, ok := b.experiments.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrExperimentNotFound, id)
	}

	resolved := make([]ExperimentResolvedTarget, 0, len(exp.Targets))

	for name, tgt := range exp.Targets {
		resolved = append(resolved, ExperimentResolvedTarget{
			ResourceType: tgt.ResourceType,
			TargetName:   name,
		})
	}

	slices.SortFunc(
		resolved,
		func(a, b ExperimentResolvedTarget) int { return strings.Compare(a.TargetName, b.TargetName) },
	)

	return resolved, nil
}

// ----------------------------------------
// Deep copy helpers
// ----------------------------------------

// cloneExperiment returns a snapshot of an Experiment safe to return outside the lock.
// The cancel field is intentionally NOT copied.
func cloneExperiment(exp *Experiment) *Experiment {
	cp := *exp
	cp.cancel = nil
	cp.CreationTime = exp.CreationTime
	cp.Tags = copyStringMap(exp.Tags)
	cp.Targets = cloneExperimentTargets(exp.Targets)
	cp.Actions = cloneExperimentActions(exp.Actions)

	if exp.StopConditions != nil {
		cp.StopConditions = append([]ExperimentStopCondition(nil), exp.StopConditions...)
	}

	if exp.EndTime != nil {
		et := *exp.EndTime
		cp.EndTime = &et
	}

	cp.LogConfiguration = cloneExperimentLogConfiguration(exp.LogConfiguration)

	if exp.ExperimentOptions != nil {
		opt := *exp.ExperimentOptions
		cp.ExperimentOptions = &opt
	}

	cp.ExperimentReportConfiguration = cloneExperimentReportConfig(exp.ExperimentReportConfiguration)
	cp.ExperimentReport = cloneExperimentReport(exp.ExperimentReport)

	return &cp
}

// cloneExperimentTargets deep-copies a running experiment's resolved targets.
func cloneExperimentTargets(targets map[string]ExperimentTarget) map[string]ExperimentTarget {
	if targets == nil {
		return nil
	}

	cp := make(map[string]ExperimentTarget, len(targets))

	for k, v := range targets {
		t := v
		t.ResourceArns = append([]string(nil), v.ResourceArns...)
		t.ResourceTags = copyStringMap(v.ResourceTags)
		t.Parameters = copyStringMap(v.Parameters)

		filters := make([]ExperimentTemplateTargetFilter, len(v.Filters))
		for i, f := range v.Filters {
			filters[i] = ExperimentTemplateTargetFilter{
				Path:   f.Path,
				Values: append([]string(nil), f.Values...),
			}
		}

		t.Filters = filters
		cp[k] = t
	}

	return cp
}

// cloneExperimentLogConfiguration deep-copies a running experiment's log configuration.
func cloneExperimentLogConfiguration(logConfig *ExperimentLogConfiguration) *ExperimentLogConfiguration {
	if logConfig == nil {
		return nil
	}

	lc := *logConfig

	if logConfig.CloudWatchLogsConfiguration != nil {
		cwl := *logConfig.CloudWatchLogsConfiguration
		lc.CloudWatchLogsConfiguration = &cwl
	}

	if logConfig.S3Configuration != nil {
		s3 := *logConfig.S3Configuration
		lc.S3Configuration = &s3
	}

	return &lc
}

// cloneExperimentActions deep-copies a running experiment's actions.
func cloneExperimentActions(actions map[string]ExperimentAction) map[string]ExperimentAction {
	if actions == nil {
		return nil
	}

	cp := make(map[string]ExperimentAction, len(actions))

	for k, v := range actions {
		a := v
		a.Parameters = copyStringMap(v.Parameters)
		a.Targets = copyStringMap(v.Targets)

		if v.StartTime != nil {
			st := *v.StartTime
			a.StartTime = &st
		}

		if v.EndTime != nil {
			et := *v.EndTime
			a.EndTime = &et
		}

		cp[k] = a
	}

	return cp
}

// cloneExperimentReportConfig deep-copies an experiment's report configuration.
func cloneExperimentReportConfig(cfg *ExperimentReportConfiguration) *ExperimentReportConfiguration {
	if cfg == nil {
		return nil
	}

	cp := *cfg

	if cfg.DataSources != nil {
		ds := *cfg.DataSources
		ds.CloudWatchDashboards = append(
			[]ExperimentReportConfigurationCloudWatchDashboard(nil),
			cfg.DataSources.CloudWatchDashboards...,
		)
		cp.DataSources = &ds
	}

	if cfg.Outputs != nil {
		out := *cfg.Outputs
		if cfg.Outputs.S3Configuration != nil {
			s3 := *cfg.Outputs.S3Configuration
			out.S3Configuration = &s3
		}
		cp.Outputs = &out
	}

	return &cp
}

// cloneExperimentReport deep-copies an experiment's generated report.
func cloneExperimentReport(rep *ExperimentReport) *ExperimentReport {
	if rep == nil {
		return nil
	}

	cp := *rep
	cp.S3Reports = append([]ExperimentReportS3Report(nil), rep.S3Reports...)

	if rep.State != nil {
		st := *rep.State
		if rep.State.Error != nil {
			errCp := *rep.State.Error
			st.Error = &errCp
		}
		cp.State = &st
	}

	return &cp
}

// ----------------------------------------
// Experiment goroutine
// ----------------------------------------

// lifecycleDelay is the short pause between lifecycle state transitions so that
// SDK polling can observe intermediate states (initiating, completing, stopping).
const lifecycleDelay = 10 * time.Millisecond

// runExperiment manages the full lifecycle of a single experiment: pending →
// initiating → running → completed/stopped/cancelled/failed. These are exactly
// the statuses in the real AWS FIS SDK's types.ExperimentStatus enum — there is
// deliberately no "completing" broadcast between running and completed (an
// earlier revision invented one; see the status constants in store.go).
func (b *InMemoryBackend) runExperiment(
	ctx context.Context,
	expID string,
	tpl *ExperimentTemplate,
	actionsMode string,
) {
	unsubscribeStopConditions := b.subscribeStopConditions(expID, tpl.StopConditions)
	defer unsubscribeStopConditions()

	// PENDING → INITIATING.
	b.setExperimentStatus(expID, statusInitiating)
	b.setAllActionStatuses(expID, actionStatusInitiating)

	initiatingTimer := time.NewTimer(lifecycleDelay)
	defer initiatingTimer.Stop()
	select {
	case <-ctx.Done():
		// Stopped before the experiment ever started running: real AWS FIS
		// reports this as "cancelled", distinct from "stopped" (interrupting an
		// experiment that had already reached "running").
		b.cleanupActions(nil, expID, statusCancelled, actionStatusCancelled)

		return
	case <-initiatingTimer.C:
	}

	// INITIATING → RUNNING.
	b.setExperimentStatus(expID, statusRunning)

	skip := actionsMode == actionsModeSkipAll
	if skip {
		// Dry-run mode: validate targets/stop-conditions/permissions without
		// injecting any faults or calling any external action provider.
		b.setAllActionStatuses(expID, actionStatusSkipped)
	} else {
		b.setAllActionStatuses(expID, actionStatusRunning)
	}

	var (
		faultRules  []chaos.FaultRule
		maxDuration time.Duration
		failReason  string
	)

	if !skip {
		// Build fault rules and run actions respecting startAfter dependencies.
		faultRules, maxDuration, failReason = b.executeActionsOrdered(ctx, expID, tpl)
	}

	if failReason != "" {
		// markExperimentFailed (called from within executeActionsOrdered) already
		// finalized exp.Status/EndTime/Actions/ExperimentReport with the
		// structured failure info — only release resources here, don't re-set
		// status (that would clobber the structured ExperimentStatusError).
		b.releaseFaultRulesAndCancel(faultRules, expID)

		return
	}

	b.waitForCompletionOrStop(ctx, expID, faultRules, maxDuration)
}

// subscribeStopConditions registers a CloudWatch alarm-state-change subscription
// (gopherstack-9939) for each "aws:cloudwatch:alarm" stop condition in
// conditions, stopping the experiment the same way StopExperiment does when the
// named alarm transitions to ALARM. A nil alarmSubscriber (nothing wired into
// FIS) makes this a no-op, leaving the experiment exactly as it behaved before
// this feature. The returned func unsubscribes everything registered here and
// must be called once the experiment reaches a terminal state.
func (b *InMemoryBackend) subscribeStopConditions(
	expID string, conditions []ExperimentTemplateStopCondition,
) func() {
	b.mu.RLock("subscribeStopConditions")
	sub := b.alarmSubscriber
	b.mu.RUnlock()

	if sub == nil {
		return func() {}
	}

	var unsubs []func()

	for _, sc := range conditions {
		if sc.Source != stopConditionSourceAlarm {
			continue
		}

		unsubs = append(unsubs, sub.SubscribeAlarmStateChange(sc.Value, func(newState string) {
			if newState == alarmStateValueAlarm {
				_, _ = b.StopExperiment(expID)
			}
		}))
	}

	return func() {
		for _, unsub := range unsubs {
			unsub()
		}
	}
}

// waitForCompletionOrStop waits for the experiment's action duration to elapse
// (or completes immediately when there is none) and transitions to the terminal
// completed/stopped status, reacting to an early stop signal via ctx
// cancellation at any point.
func (b *InMemoryBackend) waitForCompletionOrStop(
	ctx context.Context,
	expID string,
	faultRules []chaos.FaultRule,
	maxDuration time.Duration,
) {
	if maxDuration == 0 {
		// No timed actions: give SDK polling a brief window to observe "running"
		// before completing.
		grace := time.NewTimer(lifecycleDelay)
		defer grace.Stop()

		select {
		case <-ctx.Done():
			b.cleanupActions(faultRules, expID, statusStopped, actionStatusStopped)
		case <-grace.C:
			b.cleanupActions(faultRules, expID, statusCompleted, actionStatusCompleted)
		}

		return
	}

	timer := time.NewTimer(maxDuration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		// Manually stopped or context cancelled — transition through stopping.
		b.setExperimentStatus(expID, statusStopping)
		b.cleanupActions(faultRules, expID, statusStopped, actionStatusStopped)
	case <-timer.C:
		// All actions completed naturally.
		grace := time.NewTimer(lifecycleDelay)
		defer grace.Stop()

		select {
		case <-ctx.Done():
			b.cleanupActions(faultRules, expID, statusStopped, actionStatusStopped)
		case <-grace.C:
			b.cleanupActions(faultRules, expID, statusCompleted, actionStatusCompleted)
		}
	}
}

// executeActionsOrdered executes template actions in startAfter dependency order.
// Chaos fault rules are applied first, then external actions run in topological order.
// Returns accumulated fault rules, the maximum action duration, and a non-empty failure reason on error.
func (b *InMemoryBackend) executeActionsOrdered(
	ctx context.Context,
	expID string,
	tpl *ExperimentTemplate,
) ([]chaos.FaultRule, time.Duration, string) {
	var faultRules []chaos.FaultRule

	var maxDuration time.Duration

	// Sort actions into topological order respecting startAfter: topoSortActions
	// already guarantees every action appears after all of its dependencies, so
	// no separate per-action dependency wait is needed here.
	ordered := topoSortActions(tpl.Actions)

	for _, name := range ordered {
		action := tpl.Actions[name]

		// Check context before each action.
		select {
		case <-ctx.Done():
			return faultRules, maxDuration, ""
		default:
		}

		dur := parseISODuration(action.Parameters["duration"])
		if dur > maxDuration {
			maxDuration = dur
		}

		switch {
		case strings.HasPrefix(action.ActionID, "aws:fis:inject-api-"):
			faultRules = append(faultRules, buildFaultRules(action)...)
			// Apply immediately so faults are active as soon as possible.
			if len(faultRules) > 0 && b.getFaultStore() != nil {
				b.getFaultStore().AppendRules(buildFaultRules(action))
			}
		case action.ActionID == actionIDWait:
			// Wait action — duration already captured above.
		default:
			ea := externalAction{
				actionID:   action.ActionID,
				params:     copyStringMap(action.Parameters),
				targets:    action.Targets,
				duration:   dur,
				tplTargets: tpl.Targets,
			}

			b.setActionStatus(expID, name, actionStatusRunning)

			if err := b.executeExternalAction(ctx, ea); err != nil {
				b.markExperimentFailed(expID, name, err.Error())

				return faultRules, maxDuration, err.Error()
			}

			b.setActionStatus(expID, name, actionStatusCompleted)
		}
	}

	return faultRules, maxDuration, ""
}

// topoSortActions returns action names in a topological order respecting startAfter.
// Actions with no dependencies come first; actions whose dependencies are all earlier come later.
// The result is deterministic: within the same dependency level, actions are sorted by name.
func topoSortActions(actions map[string]ExperimentTemplateAction) []string {
	inDegree := make(map[string]int, len(actions))
	dependents := make(map[string][]string, len(actions)) // name → names that depend on it

	for name := range actions {
		if _, ok := inDegree[name]; !ok {
			inDegree[name] = 0
		}
	}

	for name, action := range actions {
		for _, dep := range action.StartAfter {
			inDegree[name]++
			dependents[dep] = append(dependents[dep], name)
		}
	}

	// Collect zero-in-degree nodes, sorted for determinism.
	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	slices.Sort(queue)

	result := make([]string, 0, len(actions))

	for len(queue) > 0 {
		// Pop front.
		cur := queue[0]
		queue = queue[1:]
		result = append(result, cur)

		// Reduce in-degree for dependents.
		next := make([]string, 0)

		for _, dep := range dependents[cur] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				next = append(next, dep)
			}
		}

		slices.Sort(next)
		queue = append(queue, next...)
	}

	return result
}

// setActionStatus atomically updates a single action's status.
func (b *InMemoryBackend) setActionStatus(expID, actionName, status string) {
	b.mu.Lock("setActionStatus")
	defer b.mu.Unlock()

	if exp, ok := b.experiments.Get(expID); ok {
		if action, ok2 := exp.Actions[actionName]; ok2 {
			action.Status = ExperimentActionStatus{Status: status}
			exp.Actions[actionName] = action
		}
	}
}

// applySelectionMode scopes resolved target ARNs to the count or percentage
// requested by mode. ExperimentTemplateTarget.SelectionMode says COUNT(n) and
// PERCENT(n) are "chosen from the identified targets at random"; gopherstack
// takes the first N in stored order instead, so a run is reproducible. Only
// the count is observable to a caller, which this preserves.
func applySelectionMode(arns []string, mode string) []string {
	if len(arns) == 0 {
		return arns
	}

	var n int

	switch {
	case strings.HasPrefix(mode, "COUNT("):
		v, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(mode, "COUNT("), ")"))
		if err != nil {
			return arns
		}

		n = v
	case strings.HasPrefix(mode, "PERCENT("):
		v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimPrefix(mode, "PERCENT("), ")"), 64)
		if err != nil {
			return arns
		}

		n = int(math.Ceil(float64(len(arns)) * v / percentageDivisor))
	default:
		return arns
	}

	if n > len(arns) {
		n = len(arns)
	}

	if n < 0 {
		n = 0
	}

	return arns[:n]
}

// externalAction carries the data needed to call an external FISActionProvider.
type externalAction struct {
	params     map[string]string
	targets    map[string]string
	tplTargets map[string]ExperimentTemplateTarget
	actionID   string
	duration   time.Duration
}

// executeExternalAction calls the appropriate FISActionProvider for a non-built-in action.
// Returns an error if the provider reports a failure.
func (b *InMemoryBackend) executeExternalAction(ctx context.Context, ea externalAction) error {
	b.mu.RLock("executeExternalAction")
	providers := b.actionProviders
	b.mu.RUnlock()

	// Resolve target ARNs.
	var targetARNs []string

	for targetKey, targetName := range ea.targets {
		if tgt, ok := ea.tplTargets[targetKey]; ok {
			targetARNs = append(targetARNs, applySelectionMode(tgt.ResourceArns, tgt.SelectionMode)...)
		} else if tgtByName, ok2 := ea.tplTargets[targetName]; ok2 {
			targetARNs = append(targetARNs, applySelectionMode(tgtByName.ResourceArns, tgtByName.SelectionMode)...)
		}
	}

	exec := service.FISActionExecution{
		ActionID:   ea.actionID,
		Parameters: ea.params,
		Targets:    targetARNs,
		Duration:   ea.duration,
	}

	for _, p := range providers {
		for _, def := range p.FISActions() {
			if def.ActionID == ea.actionID {
				return p.ExecuteFISAction(ctx, exec)
			}
		}
	}

	return nil
}

// cleanupActions removes fault rules and sets the final experiment status.
// It also calls exp.cancel() to release the context and prevent goroutine leaks.
func (b *InMemoryBackend) cleanupActions(faultRules []chaos.FaultRule, expID, expStatus, actionStatus string) {
	if len(faultRules) > 0 && b.getFaultStore() != nil {
		b.getFaultStore().DeleteRules(faultRules)
	}

	now := time.Now()
	b.mu.Lock("cleanupActions")
	defer b.mu.Unlock()

	if exp, ok := b.experiments.Get(expID); ok {
		exp.Status = ExperimentStatus{Status: expStatus}
		exp.EndTime = &now

		for name, action := range exp.Actions {
			// Skipped actions (actionsMode "skip-all") keep their terminal
			// "skipped" status rather than being overwritten by the
			// experiment's own terminal actionStatus.
			if action.Status.Status != actionStatusSkipped {
				action.Status = ExperimentActionStatus{Status: actionStatus}
			}

			endTime := now
			action.EndTime = &endTime
			exp.Actions[name] = action
		}

		if exp.ExperimentReportConfiguration != nil {
			exp.ExperimentReport = computeExperimentReport(exp.ExperimentReportConfiguration, exp.ID, expStatus)
		}

		// Release context resources; safe to call multiple times.
		if exp.cancel != nil {
			exp.cancel()
		}
	}
}

// releaseFaultRulesAndCancel removes fault rules and cancels the experiment's
// context after its terminal status has already been finalized elsewhere (by
// markExperimentFailed). It deliberately does NOT touch exp.Status/EndTime/
// Actions/ExperimentReport the way cleanupActions does — markExperimentFailed
// already set those, including the structured ExperimentStatusError, and
// re-setting them here would clobber that error info.
func (b *InMemoryBackend) releaseFaultRulesAndCancel(faultRules []chaos.FaultRule, expID string) {
	if len(faultRules) > 0 && b.getFaultStore() != nil {
		b.getFaultStore().DeleteRules(faultRules)
	}

	b.mu.Lock("releaseFaultRulesAndCancel")
	defer b.mu.Unlock()

	if exp, ok := b.experiments.Get(expID); ok && exp.cancel != nil {
		exp.cancel()
	}
}

// setExperimentStatus atomically updates an experiment's status.
func (b *InMemoryBackend) setExperimentStatus(id, status string) {
	b.mu.Lock("setExperimentStatus")
	defer b.mu.Unlock()

	if exp, ok := b.experiments.Get(id); ok {
		exp.Status = ExperimentStatus{Status: status}
	}
}

// setAllActionStatuses atomically sets all actions in an experiment to the given status.
func (b *InMemoryBackend) setAllActionStatuses(expID, status string) {
	b.mu.Lock("setAllActionStatuses")
	defer b.mu.Unlock()

	if exp, ok := b.experiments.Get(expID); ok {
		now := time.Now()

		for name, action := range exp.Actions {
			action.Status = ExperimentActionStatus{Status: status}
			action.StartTime = &now
			exp.Actions[name] = action
		}
	}
}

// getFaultStore safely returns the fault store (may be nil).
func (b *InMemoryBackend) getFaultStore() *chaos.FaultStore {
	b.mu.RLock("getFaultStore")
	defer b.mu.RUnlock()

	return b.faultStore
}

// actionExecutionFailedCode is the ExperimentStatusError.Code reported when an
// external action provider fails during experiment execution. AWS FIS does not
// publish a fixed enum for this field (it is a free-form string), so this mirrors
// the class of failure without inventing a fictitious modeled exception name.
const actionExecutionFailedCode = "ActionExecutionFailed"

// markExperimentFailed sets an experiment and all its actions to failed with a
// reason. actionName identifies the template action whose execution failed and is
// reported as the structured error's Location, matching the real AWS FIS
// ExperimentError.Location semantics ("context for the section of the experiment
// template that failed").
func (b *InMemoryBackend) markExperimentFailed(expID, actionName, reason string) {
	b.mu.Lock("markExperimentFailed")
	defer b.mu.Unlock()

	exp, ok := b.experiments.Get(expID)
	if !ok {
		return
	}

	now := time.Now()
	exp.Status = ExperimentStatus{
		Status: statusFailed,
		Reason: reason,
		Error: &ExperimentStatusError{
			Code:      actionExecutionFailedCode,
			Location:  actionName,
			AccountID: b.accountID,
		},
	}
	exp.EndTime = &now

	for name, action := range exp.Actions {
		if action.Status.Status == actionStatusRunning || action.Status.Status == actionStatusPending {
			action.Status = ExperimentActionStatus{Status: actionStatusFailed, Reason: reason}
			endTime := now
			action.EndTime = &endTime
			exp.Actions[name] = action
		}
	}

	if exp.ExperimentReportConfiguration != nil {
		exp.ExperimentReport = computeExperimentReport(exp.ExperimentReportConfiguration, exp.ID, statusFailed)
	}
}

// reportGeneratedType labels the single generated experiment-report artifact.
// ExperimentReportS3Report.ReportType is a free-form string in the real AWS FIS
// SDK model (no fixed enum), so this labels the artifact without inventing a
// modeled enum value.
const reportGeneratedType = "experiment-report"

// missingReportOutputCode is the ExperimentReportError.Code reported when a
// report configuration has no S3 output destination, so the generated report
// has nowhere to be written. Like ExperimentReportS3Report.ReportType,
// ExperimentReportError.Code is a free-form string in the SDK model, not a
// modeled enum.
const missingReportOutputCode = "MissingReportOutputConfiguration"

// computeExperimentReport synthesizes the terminal state of an experiment
// report once its owning experiment reaches a terminal status. Real AWS FIS
// generates the report asynchronously after the experiment finishes;
// gopherstack computes the terminal result immediately since there is no real
// S3/CloudWatch backend to wait on. expTerminalStatus is the experiment's own
// terminal status (statusCompleted/statusStopped/statusCancelled/statusFailed):
// when the experiment was cancelled before it ever started running, there is
// nothing to report, so the report itself is marked cancelled too regardless of
// output configuration.
func computeExperimentReport(cfg *ExperimentReportConfiguration, expID, expTerminalStatus string) *ExperimentReport {
	if expTerminalStatus == statusCancelled {
		return &ExperimentReport{
			State: &ExperimentReportState{
				Status: experimentReportStatusCancelled,
				Reason: "the experiment was cancelled before it started running",
			},
		}
	}

	if cfg.Outputs == nil || cfg.Outputs.S3Configuration == nil || cfg.Outputs.S3Configuration.BucketName == "" {
		return &ExperimentReport{
			State: &ExperimentReportState{
				Status: experimentReportStatusFailed,
				Reason: "the experiment report configuration has no S3 output destination",
				Error:  &ExperimentReportError{Code: missingReportOutputCode},
			},
		}
	}

	bucket := cfg.Outputs.S3Configuration.BucketName
	key := cfg.Outputs.S3Configuration.Prefix + expID + "/report.json"

	return &ExperimentReport{
		S3Reports: []ExperimentReportS3Report{
			{Arn: arn.BuildS3(bucket + "/" + key), ReportType: reportGeneratedType},
		},
		State: &ExperimentReportState{Status: experimentReportStatusCompleted},
	}
}
