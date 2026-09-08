package eventbridge

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/ctxval"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// LambdaInvoker can invoke a Lambda function by name/ARN with a payload.
type LambdaInvoker interface {
	InvokeFunction(
		ctx context.Context,
		name string,
		invocationType string,
		payload []byte,
	) ([]byte, int, error)
}

// SQSSender can send a message to an SQS queue by URL or ARN.
type SQSSender interface {
	SendMessageToQueue(ctx context.Context, queueARN, messageBody string) error
}

// SNSPublisher can publish a message to an SNS topic by ARN.
type SNSPublisher interface {
	PublishToTopic(ctx context.Context, topicARN, message string) error
}

// KinesisFirehosePublisher can put records to a Kinesis Data Firehose delivery stream.
type KinesisFirehosePublisher interface {
	PutRecord(ctx context.Context, deliveryStreamARN, data string) error
}

// KinesisStreamPublisher can put records to a Kinesis Data Stream.
type KinesisStreamPublisher interface {
	PutRecord(ctx context.Context, streamARN, partitionKey, data string) error
}

// ECSTaskRunner can run an ECS task.
type ECSTaskRunner interface {
	RunTask(ctx context.Context, clusterARN string, payload []byte) error
}

// ECSTaskRunnerWithParams is an optional extension of ECSTaskRunner for
// adapters that can honor the target's full EcsParameters (TaskDefinitionArn,
// LaunchType, TaskCount, NetworkConfiguration, ...) instead of depending on
// the delivered payload happening to carry a "TaskDefinition" key. deliverToECS
// probes dt.ECS for this interface via a type assertion -- the same
// optional-capability pattern services/sts uses for AccountSettingsLookup
// (see services/iam/account.go's "Outbound Web Identity Federation" section)
// -- so ECSTaskRunner implementations that only implement RunTask keep
// compiling and working exactly as before.
type ECSTaskRunnerWithParams interface {
	RunTaskWithParams(ctx context.Context, clusterARN string, params *EcsParameters, payload []byte) error
}

// StepFunctionsExecutor can start a Step Functions state machine execution.
type StepFunctionsExecutor interface {
	// StartExecution starts an execution of the state machine identified by stateMachineARN.
	// The name may be empty (the backend will generate one). input is a JSON string.
	StartExecution(stateMachineARN, name, input string) error
}

// DeliveryTargets holds optional service references for event fan-out.
type DeliveryTargets struct {
	Lambda          LambdaInvoker
	SQS             SQSSender
	SNS             SNSPublisher
	KinesisFirehose KinesisFirehosePublisher
	KinesisStream   KinesisStreamPublisher
	ECS             ECSTaskRunner
	StepFunctions   StepFunctionsExecutor
	CloudWatchLogs  CloudWatchLogsPublisher
	APIDestinations APIDestinationResolver
	EventBusRouter  EventBusRouter
}

// EventBusRouter routes a matched event to another event bus, implementing
// real AWS's support for a PutTargets Target ARN that names an event bus
// (see api_op_PutTargets.go's doc comment in aws-sdk-go-v2/service/eventbridge:
// "Set that account's event bus as a target of the rules in your account. To
// send the matched events to the other account, specify that account's event
// bus as the Arn value when you run PutTargets"). It is implemented by the
// backend itself so same-account routing works without external wiring (see
// SetDeliveryTargets' APIDestinations self-registration).
type EventBusRouter interface {
	// RouteEventToBus delivers the event carried by envelope to the event bus
	// identified by targetARN, and returns true if delivery failed.
	RouteEventToBus(ctx context.Context, targetARN string, envelope map[string]any) bool
}

// APIDestinationResolver resolves an API-destination ARN to its concrete
// invocation config plus the connection credentials used to authenticate the
// outbound request, and throttles delivery to the destination's configured
// rate. It is implemented by the backend and consulted at delivery time.
type APIDestinationResolver interface {
	// ResolveAPIDestination returns the resolved destination, or false if the
	// ARN does not identify a known API destination.
	ResolveAPIDestination(destARN string) (*ResolvedAPIDestination, bool)
	// WaitAPIDestinationRateLimit blocks until the destination's configured
	// rate permits another request, or ctx is done. A non-positive rate is
	// unlimited.
	WaitAPIDestinationRateLimit(ctx context.Context, destARN string, ratePerSecond int)
}

// ResolvedAPIDestination is the flattened, delivery-ready view of an API
// destination and its associated connection auth.
type ResolvedAPIDestination struct {
	OAuth                 *ResolvedOAuth
	HTTPMethod            string
	Endpoint              string
	AuthType              string
	APIKeyName            string
	APIKeyValue           string
	BasicUsername         string
	BasicPassword         string
	HeaderParameters      []ConnectionHeaderParameter
	QueryStringParameters []ConnectionQueryStringParameter
	BodyParameters        []ConnectionBodyParameter
	RateLimitPerSecond    int
}

// ResolvedOAuth carries the OAuth client-credentials configuration used to mint
// a bearer token for an API destination.
type ResolvedOAuth struct {
	AuthorizationEndpoint string
	HTTPMethod            string
	ClientID              string
	ClientSecret          string
	HeaderParameters      []ConnectionHeaderParameter
	QueryStringParameters []ConnectionQueryStringParameter
	BodyParameters        []ConnectionBodyParameter
}

// CloudWatchLogsPublisher delivers an event to a CloudWatch Logs log group.
type CloudWatchLogsPublisher interface {
	PutLogEvents(ctx context.Context, logGroupName, logStreamName string, logEvents []any) error
}

// deliverScheduledRule delivers a scheduled-rule synthetic event directly to the
// rule's targets, bypassing pattern matching. On real AWS, scheduled rules invoke
// targets directly; they are NOT routed through event pattern matching.
func (b *InMemoryBackend) deliverScheduledRule(
	ctx context.Context,
	rule Rule,
	busName, region, detailType string,
) {
	const detail = `{"scheduled":true}`

	var (
		snapped   []*Target
		accountID string
		dt        DeliveryTargets
		timeout   time.Duration
		busDLQ    *DeadLetterConfig
	)

	func() {
		b.mu.Lock("deliverScheduledRule")
		defer b.mu.Unlock()

		var storedTargets *store.Table[Target]
		if regionTargets := b.targets[region]; regionTargets != nil {
			storedTargets = regionTargets[b.targetKey(busName, rule.Name)]
		}
		snapped = snapshotTargets(storedTargets)
		accountID = b.accountID
		dt = *b.deliveryTargets
		timeout = b.deliveryTimeout
		if bus, exists := b.busesTable(region).Get(ebBusKey(busName)); exists {
			busDLQ = bus.DeadLetterConfig
		}
		// Log the event so diagnostic callers (GetEventLog) can observe it.
		eventID := uuid.NewString()
		b.eventLog = append(b.eventLog, EventLogEntry{
			ID:           eventID,
			Source:       "aws.events",
			DetailType:   detailType,
			Detail:       detail,
			EventBusName: busName,
			Time:         time.Now(),
		})
		if len(b.eventLog) > maxEventLogSize {
			b.eventLog = b.eventLog[len(b.eventLog)-maxEventLogSize:]
		}
	}()

	if len(snapped) == 0 {
		return
	}

	entry := EventEntry{
		Source:       "aws.events",
		DetailType:   detailType,
		Detail:       detail,
		EventBusName: busName,
	}
	envelope := buildDeliveryEnvelope(entry, accountID, region)

	var wg sync.WaitGroup
	for _, t := range snapped {
		target := t
		wg.Go(func() {
			deliverToTargetBounded(ctx, target, envelope, dt, timeout, busDLQ)
		})
	}
	wg.Wait()
}

// deliverEvents fan-outs events to matching rule targets. It runs
// asynchronously and does not block PutEvents. filterRuleARNs, when
// non-empty, restricts delivery to only those rule ARNs -- used by replay
// (StartReplayInput.Destination.FilterArns); PutEvents always passes nil.
func (b *InMemoryBackend) deliverEvents(
	ctx context.Context,
	region string,
	entries []EventEntry,
	targets DeliveryTargets,
	timeout time.Duration,
	filterRuleARNs map[string]struct{},
) {
	groups := b.buildDeliveryPlan(region, entries, filterRuleARNs)

	// Deliver outside the lock. Targets within a rule run concurrently; each
	// gets its own bounded context so a hung downstream service cannot block
	// the goroutine beyond the configured timeout.
	for _, g := range groups {
		var wg sync.WaitGroup
		for _, t := range g.targets {
			target := t
			envelope := g.envelope
			wg.Go(func() {
				deliverToTargetBounded(ctx, target, envelope, targets, timeout, g.busDLQ)
			})
		}
		wg.Wait()
	}
}

// deliveryGroup is one matched rule's delivery work: a shared event envelope,
// the snapshot of targets to deliver it to, and the bus-level DeadLetterConfig
// a target falls back to when it has none of its own (see sendToDLQ).
type deliveryGroup struct {
	envelope map[string]any
	busDLQ   *DeadLetterConfig
	targets  []*Target
}

// buildDeliveryPlan matches each entry against its bus under the read lock and
// returns one delivery group per matched rule. Rather than deep-copying every
// bus's rules, index and targets on the hot path, it snapshots only the matched
// rules' targets, bounding per-PutEvents work to the buses the entries reference
// and the rules that matched. Snapshotting under the lock lets delivery run
// without racing concurrent mutations (PutRule/DeleteRule/PutTargets/RemoveTargets).
// Groups preserve the original rule-by-rule, entry-by-entry ordering.
func (b *InMemoryBackend) buildDeliveryPlan(
	region string,
	entries []EventEntry,
	filterRuleARNs map[string]struct{},
) []deliveryGroup {
	b.mu.RLock("buildDeliveryPlan")
	defer b.mu.RUnlock()

	accountID := b.accountID
	// Read directly without lazy-init: buildDeliveryPlan holds only RLock, so
	// calling ruleIndexStore/targetsStore (which write on nil) races with other
	// concurrent deliverEvents goroutines. Nil map reads are safe in Go.
	ruleIndex := b.ruleIndex[region]
	targetsStore := b.targets[region]

	// len(entries) is a lower-bound capacity hint, not an exact size: each
	// entry matches zero or more rules, so the final group count can be
	// smaller or larger.
	groups := make([]deliveryGroup, 0, len(entries))
	for _, entry := range entries {
		groups = append(
			groups,
			b.matchedDeliveryGroupsForEntry(entry, region, accountID, ruleIndex, targetsStore, filterRuleARNs)...,
		)
	}

	return groups
}

// matchedDeliveryGroupsForEntry returns one deliveryGroup per rule on entry's
// bus that matches for delivery (see ruleMatchesForDelivery) and has at least
// one stored target. Extracted from buildDeliveryPlan to keep its cognitive
// complexity down; must be called with b.mu held for reading.
func (b *InMemoryBackend) matchedDeliveryGroupsForEntry(
	entry EventEntry,
	region, accountID string,
	ruleIndex map[string]map[ruleIndexKey]map[string]*Rule,
	targetsStore map[string]*store.Table[Target],
	filterRuleARNs map[string]struct{},
) []deliveryGroup {
	busName := entry.EventBusName
	if busName == "" {
		busName = defaultEventBusName
	}

	busKey := ebBusKey(busName)
	eventEnvelope := buildEventEnvelope(entry)

	var busDLQ *DeadLetterConfig
	if bus, exists := b.busesTable(region).Get(busKey); exists {
		busDLQ = bus.DeadLetterConfig
	}

	var groups []deliveryGroup
	for _, rule := range indexedRulesForEvent(ruleIndex[busKey], entry.Source, entry.DetailType) {
		if !ruleMatchesForDelivery(rule, eventEnvelope, filterRuleARNs) {
			continue
		}

		storedTargets := targetsStore[b.targetKey(busName, rule.Name)]
		if storedTargets == nil || storedTargets.Len() == 0 {
			continue
		}

		// Build the delivery envelope once per matched rule so all targets
		// for this rule share the same event id, matching AWS behaviour.
		groups = append(groups, deliveryGroup{
			envelope: buildDeliveryEnvelope(entry, accountID, region),
			busDLQ:   busDLQ,
			targets:  snapshotTargets(storedTargets),
		})
	}

	return groups
}

// ruleMatchesForDelivery reports whether rule should receive entry's event:
// ENABLED with a non-empty EventPattern, included in filterRuleARNs when
// that filter is non-empty (replay's FilterArns; PutEvents always passes an
// empty filter), and the event pattern matches. eventEnvelope is the entry's
// JSON-encoded event (see buildEventEnvelope), not the per-target delivery
// envelope built separately below.
func ruleMatchesForDelivery(rule *Rule, eventEnvelope string, filterRuleARNs map[string]struct{}) bool {
	if rule.State != "ENABLED" || rule.EventPattern == "" {
		return false
	}

	if len(filterRuleARNs) > 0 {
		if _, ok := filterRuleARNs[rule.Arn]; !ok {
			return false
		}
	}

	return matchCompiledPattern(rule.compiledPattern, eventEnvelope)
}

// snapshotTargets returns copies of the stored target structs so delivery cannot
// race a concurrent PutTargets/RemoveTargets mutating the stored values. A nil
// stored table (region/rule never touched) yields an empty, non-nil slice.
func snapshotTargets(stored *store.Table[Target]) []*Target {
	if stored == nil {
		return nil
	}

	all := stored.All()
	out := make([]*Target, 0, len(all))
	for _, t := range all {
		targetCopy := *t
		out = append(out, &targetCopy)
	}

	return out
}

// deliverToTargetBounded delivers a single event to a single target, applying a per-call
// timeout when timeout > 0, with retry logic from target.RetryPolicy.
func deliverToTargetBounded(
	ctx context.Context,
	target *Target,
	envelope map[string]any,
	dt DeliveryTargets,
	timeout time.Duration,
	busDLQ *DeadLetterConfig,
) {
	maxAttempts := defaultMaxRetryAttempts
	maxAgeSeconds := defaultMaxEventAgeSeconds

	if target.RetryPolicy != nil {
		if target.RetryPolicy.MaximumRetryAttempts >= 0 {
			maxAttempts = target.RetryPolicy.MaximumRetryAttempts
		}
		if target.RetryPolicy.MaximumEventAgeInSeconds > 0 {
			maxAgeSeconds = target.RetryPolicy.MaximumEventAgeInSeconds
		}
	}

	eventAge := extractEventAge(envelope)

	for attempt := 0; attempt <= maxAttempts; attempt++ {
		if int(eventAge.Seconds()) > maxAgeSeconds {
			sendToDLQ(ctx, target, envelope, dt, busDLQ, "MaximumEventAgeExceeded")

			return
		}

		var delivErr bool
		if timeout <= 0 {
			delivErr = deliverToTarget(ctx, target, envelope, dt)
		} else {
			tCtx, cancel := context.WithTimeout(ctx, timeout)
			delivErr = deliverToTarget(tCtx, target, envelope, dt)
			cancel()
		}

		if !delivErr {
			return
		}

		if attempt == maxAttempts {
			sendToDLQ(ctx, target, envelope, dt, busDLQ, "DeliveryFailure")

			return
		}
	}
}

// extractEventAge returns the age of the event from the envelope's "time" field.
func extractEventAge(envelope map[string]any) time.Duration {
	timeVal, ok := envelope["time"].(string)
	if !ok {
		return 0
	}

	t, err := time.Parse(time.RFC3339, timeVal)
	if err != nil {
		return 0
	}

	age := time.Since(t)
	if age < 0 {
		return 0
	}

	return age
}

// sendToDLQ sends an event to the dead-letter queue if configured.
func sendToDLQ(
	ctx context.Context,
	target *Target,
	envelope map[string]any,
	dt DeliveryTargets,
	busDLQ *DeadLetterConfig,
	reason string,
) {
	dlq := target.DeadLetterConfig
	if dlq == nil || dlq.Arn == "" {
		// A target with no DLQ of its own falls back to the bus's, matching
		// real EventBridge: DeadLetterConfig can be set at the target or the
		// bus, and the bus's applies when the target has none.
		dlq = busDLQ
	}
	if dlq == nil || dlq.Arn == "" {
		return
	}
	if dt.SQS == nil {
		return
	}

	log := logger.Load(ctx)
	payload, _ := json.Marshal(envelope)
	dlqARN := dlq.Arn

	if err := dt.SQS.SendMessageToQueue(ctx, dlqARN, string(payload)); err != nil {
		log.WarnContext(ctx, "EventBridge: failed to send event to DLQ",
			"dlq", dlqARN, "reason", reason, "error", err)
	}
}

func indexedRulesForEvent(
	index map[ruleIndexKey]map[string]*Rule,
	source, detailType string,
) []*Rule {
	if len(index) == 0 {
		return nil
	}

	candidateKeys := []ruleIndexKey{
		{source: source, detailType: detailType},
		{source: source, detailType: ruleIndexAny},
		{source: ruleIndexAny, detailType: detailType},
		{source: ruleIndexAny, detailType: ruleIndexAny},
	}

	rulesByName := make(map[string]*Rule)
	for _, key := range candidateKeys {
		bucket := index[key]
		maps.Copy(rulesByName, bucket)
	}

	rules := make([]*Rule, 0, len(rulesByName))
	for _, rule := range rulesByName {
		rules = append(rules, rule)
	}

	return rules
}

// buildEventEnvelope creates a JSON string representing the normalized event for pattern matching.
func buildEventEnvelope(entry EventEntry) string {
	envelope := map[string]any{
		"source":      entry.Source,
		"detail-type": entry.DetailType,
	}

	if entry.EventBusName != "" {
		envelope["event-bus-name"] = entry.EventBusName
	}

	if len(entry.Resources) > 0 {
		resources := make([]any, len(entry.Resources))
		for i, r := range entry.Resources {
			resources[i] = r
		}

		envelope["resources"] = resources
	}

	if entry.Detail != "" {
		var detail map[string]any
		if err := json.Unmarshal([]byte(entry.Detail), &detail); err == nil {
			envelope["detail"] = detail
		} else {
			envelope["detail"] = entry.Detail
		}
	}

	b, _ := json.Marshal(envelope)

	return string(b)
}

// deliverToTarget delivers a single event to a single target.
// Returns true if delivery failed (triggering retry/DLQ).
func deliverToTarget(
	ctx context.Context,
	target *Target,
	envelope map[string]any,
	dt DeliveryTargets,
) bool {
	targetARN := target.Arn
	payload := buildPayload(target, envelope)

	switch {
	case isLambdaARN(targetARN):
		return deliverToLambda(ctx, dt.Lambda, targetARN, payload)
	case isSQSARN(targetARN):
		return deliverToSQS(ctx, dt.SQS, targetARN, payload)
	case isSNSARN(targetARN):
		return deliverToSNS(ctx, dt.SNS, targetARN, payload)
	case isKinesisFirehoseARN(targetARN):
		return deliverToKinesisFirehose(ctx, dt.KinesisFirehose, targetARN, payload)
	case isKinesisStreamARN(targetARN):
		return deliverToKinesisStream(ctx, dt.KinesisStream, targetARN, payload)
	case isECSARN(targetARN):
		return deliverToECS(ctx, dt.ECS, targetARN, payload, target.EcsParameters)
	case isStateMachineARN(targetARN):
		return deliverToStepFunctions(ctx, dt.StepFunctions, targetARN, payload)
	case isCloudWatchLogsARN(targetARN):
		return deliverToCloudWatchLogs(ctx, dt.CloudWatchLogs, targetARN, payload)
	case isAPIDestinationARN(targetARN):
		return deliverToAPIDestination(ctx, dt.APIDestinations, target, payload)
	case isEventBusARN(targetARN):
		// Routes the raw event, not payload: Input/InputPath/InputTransformer
		// are target-invocation overrides and don't apply when the "target"
		// is itself an event bus that will run its own rule matching on the
		// real event (api_op_PutTargets.go: these overrides are documented as
		// unavailable when the target is a cross-account event bus).
		return deliverToEventBus(ctx, dt.EventBusRouter, targetARN, envelope)
	default:
		logger.Load(ctx).
			WarnContext(ctx, "EventBridge: unsupported target ARN type", "arn", targetARN)
	}

	return false
}

func deliverToLambda(ctx context.Context, svc LambdaInvoker, arn, payload string) bool {
	if svc == nil {
		return false
	}
	if _, _, err := svc.InvokeFunction(ctx, arn, "Event", []byte(payload)); err != nil {
		logger.Load(ctx).
			WarnContext(ctx, "EventBridge failed to invoke Lambda target", "arn", arn, "error", err)

		return true
	}

	return false
}

func deliverToSQS(ctx context.Context, svc SQSSender, arn, payload string) bool {
	if svc == nil {
		return false
	}
	if err := svc.SendMessageToQueue(ctx, arn, payload); err != nil {
		logger.Load(ctx).
			WarnContext(ctx, "EventBridge failed to deliver to SQS target", "arn", arn, "error", err)

		return true
	}

	return false
}

func deliverToSNS(ctx context.Context, svc SNSPublisher, arn, payload string) bool {
	if svc == nil {
		return false
	}
	if err := svc.PublishToTopic(ctx, arn, payload); err != nil {
		logger.Load(ctx).
			WarnContext(ctx, "EventBridge failed to publish to SNS target", "arn", arn, "error", err)

		return true
	}

	return false
}

func deliverToKinesisFirehose(
	ctx context.Context,
	svc KinesisFirehosePublisher,
	arn, payload string,
) bool {
	if svc == nil {
		return false
	}
	if err := svc.PutRecord(ctx, arn, payload); err != nil {
		logger.Load(ctx).WarnContext(ctx, "EventBridge failed to put record to Kinesis Firehose",
			"arn", arn, "error", err)

		return true
	}

	return false
}

func deliverToKinesisStream(
	ctx context.Context,
	svc KinesisStreamPublisher,
	arn, payload string,
) bool {
	if svc == nil {
		return false
	}
	partitionKey := uuid.New().String()
	if err := svc.PutRecord(ctx, arn, partitionKey, payload); err != nil {
		logger.Load(ctx).WarnContext(ctx, "EventBridge failed to put record to Kinesis Data Stream",
			"arn", arn, "error", err)

		return true
	}

	return false
}

// deliverToECS runs the target's ECS task. When svc also implements
// ECSTaskRunnerWithParams, the target's EcsParameters (TaskDefinitionArn,
// LaunchType, TaskCount, NetworkConfiguration, ...) are threaded through
// instead of relying on the payload to carry a "TaskDefinition" key.
func deliverToECS(ctx context.Context, svc ECSTaskRunner, arn, payload string, params *EcsParameters) bool {
	if svc == nil {
		return false
	}

	var err error
	if withParams, ok := svc.(ECSTaskRunnerWithParams); ok {
		err = withParams.RunTaskWithParams(ctx, arn, params, []byte(payload))
	} else {
		err = svc.RunTask(ctx, arn, []byte(payload))
	}

	if err != nil {
		logger.Load(ctx).
			WarnContext(ctx, "EventBridge failed to run ECS task", "arn", arn, "error", err)

		return true
	}

	return false
}

// buildPayload constructs the message payload for a target from a pre-built event envelope.
// Priority: Input override → InputPath → InputTransformer → full event envelope.
func buildPayload(target *Target, envelope map[string]any) string {
	if target.Input != "" {
		return target.Input
	}

	if target.InputPath != "" {
		return applyInputPath(target.InputPath, envelope)
	}

	if target.InputTransformer != nil {
		return applyInputTransformer(target.InputTransformer, envelope)
	}

	b, _ := json.Marshal(envelope)

	return string(b)
}

// buildDeliveryEnvelope creates the full AWS EventBridge event envelope used for delivery payloads.
// It includes id, version, time, account, region, source, detail-type, resources, and detail.
func buildDeliveryEnvelope(entry EventEntry, accountID, region string) map[string]any {
	eventTime := time.Now()
	if entry.Time != nil {
		eventTime = *entry.Time
	}

	var detail any
	if entry.Detail != "" {
		var d any
		if err := json.Unmarshal([]byte(entry.Detail), &d); err == nil {
			detail = d
		} else {
			detail = entry.Detail
		}
	}

	resources := entry.Resources
	if resources == nil {
		resources = []string{}
	}

	return map[string]any{
		"version":     "0",
		"id":          uuid.New().String(),
		"source":      entry.Source,
		"account":     accountID,
		"time":        eventTime.UTC().Format(time.RFC3339),
		"region":      region,
		"resources":   resources,
		"detail-type": entry.DetailType,
		"detail":      detail,
	}
}

// isLambdaARN returns true if the ARN identifies a Lambda function.
func isLambdaARN(arn string) bool {
	return strings.Contains(arn, ":lambda:") || strings.HasPrefix(arn, "arn:aws:lambda:")
}

// isSQSARN returns true if the ARN identifies an SQS queue.
func isSQSARN(arn string) bool {
	return strings.Contains(arn, ":sqs:") || strings.HasPrefix(arn, "arn:aws:sqs:")
}

// isSNSARN returns true if the ARN identifies an SNS topic.
func isSNSARN(arn string) bool {
	return strings.Contains(arn, ":sns:") || strings.HasPrefix(arn, "arn:aws:sns:")
}

// isKinesisFirehoseARN returns true if the ARN identifies a Kinesis Data Firehose delivery stream.
func isKinesisFirehoseARN(arn string) bool {
	return strings.Contains(arn, ":firehose:")
}

// isKinesisStreamARN returns true if the ARN identifies a Kinesis Data Stream.
func isKinesisStreamARN(arn string) bool {
	return strings.Contains(arn, ":kinesis:") && strings.Contains(arn, ":stream/")
}

// isECSARN returns true if the ARN identifies an ECS cluster or task.
func isECSARN(arn string) bool {
	return strings.Contains(arn, ":ecs:")
}

// isStateMachineARN returns true if the ARN identifies a Step Functions state machine.
func isStateMachineARN(arn string) bool {
	return strings.Contains(arn, ":states:") && strings.Contains(arn, ":stateMachine:")
}

func deliverToStepFunctions(
	ctx context.Context,
	svc StepFunctionsExecutor,
	arn, payload string,
) bool {
	if svc == nil {
		return false
	}
	if err := svc.StartExecution(arn, "", payload); err != nil {
		logger.Load(ctx).
			WarnContext(ctx, "EventBridge failed to start Step Functions execution", "arn", arn, "error", err)

		return true
	}

	return false
}

func isCloudWatchLogsARN(arn string) bool {
	return strings.HasPrefix(arn, "arn:aws:logs:")
}

func isAPIDestinationARN(arn string) bool {
	return strings.HasPrefix(arn, "arn:aws:events:") && strings.Contains(arn, ":api-destination/")
}

// isEventBusARN returns true if the ARN identifies an EventBridge event bus.
func isEventBusARN(arn string) bool {
	return strings.HasPrefix(arn, "arn:aws:events:") && strings.Contains(arn, ":event-bus/")
}

func deliverToEventBus(ctx context.Context, router EventBusRouter, arn string, envelope map[string]any) bool {
	if router == nil {
		return false
	}

	return router.RouteEventToBus(ctx, arn, envelope)
}

func deliverToCloudWatchLogs(ctx context.Context, svc CloudWatchLogsPublisher, arn, payload string) bool {
	if svc == nil {
		return false
	}
	parts := strings.Split(arn, ":")
	if len(parts) < 7 || parts[5] != "log-group" {
		return false
	}
	logGroupName := parts[6]

	err := svc.PutLogEvents(ctx, logGroupName, "EventBridge", []any{payload})

	return err != nil
}

// maxEventBusRoutingHops bounds a chain of event-bus targets (a rule on bus A
// targets bus B, whose own rule targets bus C, ...) so a misconfigured cycle
// (a rule targeting its own bus, directly or transitively) cannot recurse
// RouteEventToBus's synchronous call chain forever. Real AWS has no such
// limit -- each hop there is an independent service call -- but this backend
// runs every hop in-process on the same goroutine.
const maxEventBusRoutingHops = 5

//nolint:gochecknoglobals // ctxval.Key is a singleton by design (see pkgs/ctxval).
var eventBusHopsKey = ctxval.NewKey[int]("eventbridge-bus-routing-hops")

// arnSegments is the number of colon-separated fields in a well-formed ARN:
// arn:{partition}:{service}:{region}:{account}:{resource}.
const arnSegments = 6

// parseEventBusARN extracts the region, account ID, and bus name from an
// event-bus ARN (arn:{partition}:events:{region}:{account}:event-bus/{name},
// see accessors.go's busARN). The last return is false if arn does not have
// this shape.
func parseEventBusARN(arn string) (string, string, string, bool) {
	parts := strings.SplitN(arn, ":", arnSegments)
	if len(parts) != arnSegments || parts[0] != "arn" || parts[2] != "events" {
		return "", "", "", false
	}

	name, found := strings.CutPrefix(parts[5], "event-bus/")
	if !found || name == "" {
		return "", "", "", false
	}

	return parts[3], parts[4], name, true
}

// entryFromEnvelope rebuilds an EventEntry from a delivery envelope (see
// buildDeliveryEnvelope) for re-entry into PutEvents against another bus.
// Time is intentionally left unset: the receiving bus treats this as a fresh
// incoming event, matching how a real cross-bus PutTargets delivery is a new
// service call against the target bus.
func entryFromEnvelope(envelope map[string]any, busName string) EventEntry {
	entry := EventEntry{EventBusName: busName}

	if src, ok := envelope["source"].(string); ok {
		entry.Source = src
	}

	if dt, ok := envelope["detail-type"].(string); ok {
		entry.DetailType = dt
	}

	if detail, ok := envelope["detail"]; ok {
		if b, err := json.Marshal(detail); err == nil {
			entry.Detail = string(b)
		}
	}

	if resources, ok := envelope["resources"].([]string); ok {
		entry.Resources = resources
	}

	return entry
}

// RouteEventToBus implements EventBusRouter by re-entering PutEvents for the
// target bus, so an event-bus ARN target gets the same logging, archive
// capture, and rule-matching/delivery fan-out a direct PutEvents call to that
// bus would receive. Cross-account ARNs are structurally out of scope -- this
// backend models a single AWS account, so a target whose account segment
// differs from b.accountID cannot be resolved -- and are dropped like any
// other target ARN this backend does not implement delivery for (see
// deliverToTarget's default case).
func (b *InMemoryBackend) RouteEventToBus(ctx context.Context, targetARN string, envelope map[string]any) bool {
	region, accountID, busName, ok := parseEventBusARN(targetARN)
	if !ok || accountID != b.accountID {
		return false
	}

	hops, _ := eventBusHopsKey.Get(ctx)
	if hops >= maxEventBusRoutingHops {
		logger.Load(ctx).WarnContext(ctx,
			"EventBridge: dropping cross-bus event, max routing hops exceeded",
			"targetBusArn", targetARN)

		return false
	}

	routeCtx := context.WithValue(ctx, regionContextKey{}, region)
	routeCtx = eventBusHopsKey.Set(routeCtx, hops+1)

	entry := entryFromEnvelope(envelope, busName)

	if _, err := b.PutEvents(routeCtx, []EventEntry{entry}); err != nil {
		logger.Load(ctx).WarnContext(ctx, "EventBridge: failed to route event to target bus",
			"targetBusArn", targetARN, "error", err)

		return true
	}

	return false
}
