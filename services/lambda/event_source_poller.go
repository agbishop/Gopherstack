package lambda

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

const (
	// arnKinesisPartCount is the number of colon-separated parts in a Kinesis ARN.
	arnKinesisPartCount = 6
	// millisToSeconds converts Unix milliseconds to a float64 second timestamp.
	millisToSeconds = 1000.0
)

// DynamoDBStreamRecord is a single record from a DynamoDB stream.
// The image fields use DynamoDB JSON wire format (e.g. {"pk": {"S": "value"}}).
type DynamoDBStreamRecord struct {
	NewImage                    map[string]any
	OldImage                    map[string]any
	Keys                        map[string]any
	EventID                     string
	EventName                   string // INSERT, MODIFY, or REMOVE
	SequenceNumber              string
	StreamViewType              string
	ApproximateCreationDateTime float64 // Unix epoch seconds
	SizeBytes                   int64
}

// DynamoDBStreamsReader reads records from a DynamoDB stream.
// It is implemented by the DynamoDB backend adapter in the CLI wiring layer.
type DynamoDBStreamsReader interface {
	// DescribeStreamShards returns the ordered list of shard IDs for the stream.
	// streamARN is the full DynamoDB stream ARN.
	DescribeStreamShards(streamARN string) ([]string, error)
	// GetStreamShardIterator returns an iterator for a specific shard of the stream.
	// streamARN is the full DynamoDB stream ARN, shardID is the shard identifier,
	// and iteratorType is TRIM_HORIZON or LATEST.
	GetStreamShardIterator(streamARN, shardID, iteratorType string) (string, error)
	// GetStreamRecords reads up to limit records from the given iterator.
	GetStreamRecords(iteratorToken string, limit int) ([]DynamoDBStreamRecord, string, error)
}

// KinesisReader is the interface for reading Kinesis records.
// It is implemented by the kinesis backend. streamARN is the full Kinesis
// stream ARN (not just the bare name): a stream is region-scoped, and the
// ARN is the only place the target region lives at this call boundary.
type KinesisReader interface {
	// GetShardIDs returns the shard IDs for the given stream.
	GetShardIDs(streamARN string) ([]string, error)
	// GetShardIterator returns an iterator token for a shard.
	GetShardIterator(streamARN, shardID, iteratorType, startingSeqNum string) (string, error)
	// GetRecords reads up to limit records from the given iterator, returning records and next iterator.
	GetRecords(iteratorToken string, limit int) ([]KinesisRecord, string, error)
}

// KinesisRecord is a single record from a Kinesis shard.
type KinesisRecord struct {
	ArrivalTime    time.Time
	PartitionKey   string
	SequenceNumber string
	Data           []byte
}

// SQSMessage is a single SQS message delivered to a Lambda function via ESM.
type SQSMessage struct {
	Attributes             map[string]string
	MessageAttributes      map[string]SQSMessageAttribute
	MessageID              string
	ReceiptHandle          string
	Body                   string
	MD5OfBody              string
	MD5OfMessageAttributes string
	// SentTimestampMillis is the SQS SentTimestamp in Unix milliseconds, used to
	// enforce MaximumRecordAgeInSeconds. Zero means unknown (never expired).
	SentTimestampMillis int64
}

// SQSMessageAttribute is a user-supplied SQS message attribute delivered to the
// Lambda function in the event record's messageAttributes map.
type SQSMessageAttribute struct {
	DataType    string `json:"dataType"`
	StringValue string `json:"stringValue,omitempty"`
	BinaryValue []byte `json:"binaryValue,omitempty"`
}

// SQSReader is the interface for consuming SQS messages in the ESM poller.
// It is implemented by the SQS backend adapter in the CLI wiring layer.
type SQSReader interface {
	// ReceiveMessagesLocal pulls up to maxMessages from the queue identified by queueARN.
	ReceiveMessagesLocal(queueARN string, maxMessages int) ([]*SQSMessage, error)
	// DeleteMessagesLocal removes the messages identified by receiptHandles from the queue.
	DeleteMessagesLocal(queueARN string, receiptHandles []string) error
}

// EventSourcePoller polls Kinesis streams, SQS queues, and DynamoDB streams for
// new records and invokes Lambda functions for enabled event source mappings.
type EventSourcePoller struct {
	kinesisReader    KinesisReader
	sqsReader        SQSReader
	ddbStreamsReader DynamoDBStreamsReader
	lambdaBackend    *InMemoryBackend
	shardIterators   map[string]string
	// sqsBatchBuffers holds partial SQS batches per mapping UUID while the
	// MaximumBatchingWindow has not yet elapsed. Keyed by ESM UUID.
	sqsBatchBuffers map[string]*sqsBatchBuffer
	mu              *lockmetrics.RWMutex
	// notifyC allows callers to trigger an immediate poll without waiting for the next tick.
	notifyC chan struct{}
	// sqsInvoker is an optional override for the Lambda invocation step used
	// when processing SQS messages. When nil the real InMemoryBackend is used.
	// Returns the raw invocation response body (may be nil) and any error.
	// Intended for use in unit tests only.
	sqsInvoker func(ctx context.Context, fnName string) ([]byte, error)
	// ddbInvoker is an optional override for the Lambda invocation step used
	// when processing DynamoDB stream records. When nil the real InMemoryBackend is used.
	// Intended for use in unit tests only.
	ddbInvoker func(ctx context.Context, fnName string, payload []byte) error
	// cancelSignal is an optional channel that is closed when the poller's context is cancelled.
	// Used only in tests to detect lifecycle shutdown.
	cancelSignal chan struct{}
}

// NewEventSourcePoller creates a new EventSourcePoller.
func NewEventSourcePoller(
	lambdaBackend *InMemoryBackend,
	kinesisReader KinesisReader,
) *EventSourcePoller {
	return &EventSourcePoller{
		lambdaBackend:   lambdaBackend,
		kinesisReader:   kinesisReader,
		shardIterators:  make(map[string]string),
		sqsBatchBuffers: make(map[string]*sqsBatchBuffer),
		mu:              lockmetrics.New("lambda.esm"),
		notifyC:         make(chan struct{}, 1),
	}
}

// Notify triggers an immediate poll cycle without waiting for the next tick.
// It is safe to call concurrently. If a notification is already pending, this is a no-op.
func (p *EventSourcePoller) Notify() {
	select {
	case p.notifyC <- struct{}{}:
	default:
	}
}

// SetSQSReader sets the SQS reader used to poll SQS queues for ESM delivery.
func (p *EventSourcePoller) SetSQSReader(r SQSReader) {
	p.mu.Lock("SetSQSReader")
	defer p.mu.Unlock()

	p.sqsReader = r
}

// SetDynamoDBStreamsReader sets the DynamoDB Streams reader used to poll DynamoDB
// streams for ESM delivery.
func (p *EventSourcePoller) SetDynamoDBStreamsReader(r DynamoDBStreamsReader) {
	p.mu.Lock("SetDynamoDBStreamsReader")
	defer p.mu.Unlock()

	p.ddbStreamsReader = r
}

// getDDBStreamsReader returns the DynamoDB Streams reader under a read lock.
func (p *EventSourcePoller) getDDBStreamsReader() DynamoDBStreamsReader {
	p.mu.RLock("getDDBStreamsReader")
	defer p.mu.RUnlock()

	return p.ddbStreamsReader
}

const (
	// defaultPollInterval is how often the poller ticks to check for new records.
	defaultPollInterval = 1 * time.Second
	// maxBackoffInterval is the maximum idle-backoff interval when no enabled mappings exist.
	maxBackoffInterval = 16 * time.Second
)

// Start runs the event source poller as a background goroutine.
// It returns immediately; the goroutine stops when ctx is cancelled.
func (p *EventSourcePoller) Start(ctx context.Context) {
	go p.run(ctx)
}

func (p *EventSourcePoller) run(ctx context.Context) {
	interval := defaultPollInterval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer func() {
		if p.cancelSignal != nil {
			close(p.cancelSignal)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.notifyC:
			// Received explicit notification; reset to default interval and poll immediately.
			interval = defaultPollInterval
			ticker.Reset(interval)
			p.poll(ctx)
		case <-ticker.C:
			interval = p.pollAndBackoff(ctx, interval, ticker)
		}
	}
}

// pollAndBackoff runs a single poll cycle and adjusts the ticker interval based
// on whether any enabled mappings were found. It returns the new interval.
func (p *EventSourcePoller) pollAndBackoff(
	ctx context.Context,
	interval time.Duration,
	ticker *time.Ticker,
) time.Duration {
	enabled := p.poll(ctx)
	if enabled > 0 {
		if interval != defaultPollInterval {
			interval = defaultPollInterval
			ticker.Reset(interval)
		}

		return interval
	}

	// No enabled mappings; apply exponential backoff up to maxBackoffInterval.
	if interval < maxBackoffInterval {
		interval *= 2
		if interval > maxBackoffInterval {
			interval = maxBackoffInterval
		}

		ticker.Reset(interval)
	}

	return interval
}

// poll iterates over all enabled event source mappings and processes new records.
// It returns the number of enabled mappings found.
func (p *EventSourcePoller) poll(ctx context.Context) int {
	mappings := p.lambdaBackend.ListEventSourceMappings("", "", "", 0).Data

	activeUUIDs := make(map[string]struct{}, len(mappings))
	enabledCount := 0

	for _, m := range mappings {
		activeUUIDs[m.UUID] = struct{}{}

		if m.State != ESMStateEnabled {
			continue
		}

		enabledCount++
		p.processOneMapping(ctx, m)
	}

	p.sweepStaleIterators(activeUUIDs)

	return enabledCount
}

// processOneMapping dispatches a single enabled event source mapping to the appropriate handler.
func (p *EventSourcePoller) processOneMapping(ctx context.Context, m *EventSourceMapping) {
	if isSQSARN(m.EventSourceARN) {
		var sqsR SQSReader

		func() {
			p.mu.RLock("poll")
			defer p.mu.RUnlock()

			sqsR = p.sqsReader
		}()

		if sqsR != nil {
			p.processSQSMapping(ctx, m, sqsR)
		}

		return
	}

	if isDynamoDBStreamARN(m.EventSourceARN) {
		ddbR := p.getDDBStreamsReader()
		if ddbR != nil {
			p.processDynamoDBStreamMapping(ctx, m, ddbR)
		}

		return
	}

	if streamNameFromARN(m.EventSourceARN) == "" {
		return
	}

	// The full ARN (not just the extracted name) is threaded through to
	// kinesisReader: real AWS streams are region-scoped, and the ARN is the
	// only place the target region lives once StartingPosition polling
	// begins -- see cli.go's kinesisReaderAdapter (gopherstack-qowd).
	p.processMapping(ctx, m, m.EventSourceARN)
}

// sweepStaleIterators removes shard iterator entries for ESMs that no longer exist.
// This prevents unbounded map growth when ESMs are deleted.
func (p *EventSourcePoller) sweepStaleIterators(activeUUIDs map[string]struct{}) {
	p.mu.Lock("sweepStaleIterators")
	defer p.mu.Unlock()

	for key := range p.shardIterators {
		uuid, _, ok := strings.Cut(key, ":")
		if !ok {
			continue
		}

		if _, active := activeUUIDs[uuid]; !active {
			delete(p.shardIterators, key)
		}
	}

	for uuid := range p.sqsBatchBuffers {
		if _, active := activeUUIDs[uuid]; !active {
			delete(p.sqsBatchBuffers, uuid)
		}
	}
}

// invokeESMFunctionEvent delivers an ESM batch to fnName as an async (Event)
// invocation, honoring an optional qualifier (version/alias) so that mappings
// created against a qualified function ARN (arn:...:function:my-func:PROD)
// invoke that specific version/alias rather than $LATEST — matching real
// Lambda ESM behaviour. Bug fix (parity-sweep-3): previously every ESM
// delivery path called InvokeFunction unconditionally, silently ignoring any
// qualifier and always invoking $LATEST.
func (p *EventSourcePoller) invokeESMFunctionEvent(
	ctx context.Context, fnName, qualifier string, payload []byte,
) ([]byte, error) {
	if qualifier == "" {
		result, _, err := p.lambdaBackend.InvokeFunction(ctx, fnName, InvocationTypeEvent, payload)

		return result, err
	}

	result, _, _, _, err := p.lambdaBackend.InvokeFunctionWithQualifier(
		ctx, fnName, qualifier, "", "", InvocationTypeEvent, payload,
	)

	return result, err
}

// RemoveMapping removes any per-mapping poller state for the given ESM UUID.
func (p *EventSourcePoller) RemoveMapping(uuid string) {
	p.mu.Lock("RemoveMapping")
	defer p.mu.Unlock()

	for key := range p.shardIterators {
		mappingUUID, _, ok := strings.Cut(key, ":")
		if !ok || mappingUUID != uuid {
			continue
		}

		delete(p.shardIterators, key)
	}

	delete(p.sqsBatchBuffers, uuid)
}

// processMapping reads new records from all shards and invokes Lambda.
func (p *EventSourcePoller) processMapping(ctx context.Context, m *EventSourceMapping, streamARN string) {
	shardIDs, err := p.kinesisReader.GetShardIDs(streamARN)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "event source poller: failed to get shard IDs",
			"stream", streamARN, "error", err)

		return
	}

	for _, shardID := range shardIDs {
		iterKey := m.UUID + ":" + shardID

		var (
			it     string
			exists bool
		)

		func() {
			p.mu.Lock("processMapping")
			defer p.mu.Unlock()

			it, exists = p.shardIterators[iterKey]
		}()

		if !exists {
			// Initialize iterator at starting position
			it, err = p.kinesisReader.GetShardIterator(streamARN, shardID, m.StartingPosition, "")
			if err != nil {
				logger.Load(ctx).WarnContext(ctx, "event source poller: failed to get shard iterator",
					"stream", streamARN, "shard", shardID, "error", err)

				continue
			}

			func() {
				p.mu.Lock("processMapping")
				defer p.mu.Unlock()

				p.shardIterators[iterKey] = it
			}()
		}

		records, nextIt, readErr := p.kinesisReader.GetRecords(it, m.BatchSize)
		if readErr != nil {
			// Iterator may have expired; reset it
			func() {
				p.mu.Lock("processMapping")
				defer p.mu.Unlock()

				delete(p.shardIterators, iterKey)
			}()
			logger.Load(ctx).WarnContext(ctx, "event source poller: GetRecords failed, resetting iterator",
				"stream", streamARN, "shard", shardID, "error", readErr)

			continue
		}

		func() {
			p.mu.Lock("processMapping")
			defer p.mu.Unlock()

			p.shardIterators[iterKey] = nextIt
		}()

		if len(records) == 0 {
			continue
		}

		p.invokeLambda(ctx, m, streamARN, shardID, records)
	}
}

// invokeLambda formats the Kinesis records as a Lambda event and invokes the function.
func (p *EventSourcePoller) invokeLambda(
	ctx context.Context,
	m *EventSourceMapping,
	streamARN, shardID string,
	records []KinesisRecord,
) {
	type kinesisRecord struct {
		KinesisSchemaVersion        string  `json:"kinesisSchemaVersion"`
		PartitionKey                string  `json:"partitionKey"`
		SequenceNumber              string  `json:"sequenceNumber"`
		Data                        string  `json:"data"`
		ApproximateArrivalTimestamp float64 `json:"approximateArrivalTimestamp"`
	}
	type lambdaRecord struct {
		EventSource       string        `json:"eventSource"`
		EventVersion      string        `json:"eventVersion"`
		EventID           string        `json:"eventID"`
		EventName         string        `json:"eventName"`
		InvokeIdentityArn string        `json:"invokeIdentityArn"`
		AWSRegion         string        `json:"awsRegion"`
		EventSourceARN    string        `json:"eventSourceARN"`
		Kinesis           kinesisRecord `json:"kinesis"`
	}
	type lambdaEvent struct {
		Records []lambdaRecord `json:"Records"`
	}

	eventRecords := make([]lambdaRecord, 0, len(records))
	for _, r := range records {
		// Apply FilterCriteria: records matching no filter are skipped.
		if !eventFilterMatches(m.FilterCriteria, kinesisFilterView(r.PartitionKey, r.SequenceNumber, r.Data)) {
			continue
		}

		eventRecords = append(eventRecords, lambdaRecord{
			Kinesis: kinesisRecord{
				KinesisSchemaVersion:        "1.0",
				PartitionKey:                r.PartitionKey,
				SequenceNumber:              r.SequenceNumber,
				Data:                        base64.StdEncoding.EncodeToString(r.Data),
				ApproximateArrivalTimestamp: float64(r.ArrivalTime.UnixMilli()) / millisToSeconds,
			},
			EventSource:       "aws:kinesis",
			EventVersion:      "1.0",
			EventID:           fmt.Sprintf("%s:%s", shardID, r.SequenceNumber),
			EventName:         "aws:kinesis:record",
			InvokeIdentityArn: m.FunctionARN,
			AWSRegion:         p.lambdaBackend.region,
			EventSourceARN:    m.EventSourceARN,
		})
	}

	if len(eventRecords) == 0 {
		return
	}

	payload, err := json.Marshal(lambdaEvent{Records: eventRecords})
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "event source poller: failed to marshal event", "error", err)

		return
	}

	// Extract function name (and optional version/alias qualifier) from ARN.
	fnName, qualifier := functionNameAndQualifierFromARN(m.FunctionARN)
	if fnName == "" {
		fnName = m.FunctionARN
	}

	_, err = p.invokeESMFunctionEvent(ctx, fnName, qualifier, payload)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "event source poller: Lambda invocation failed",
			"function", fnName, "stream", streamARN, "error", err)
	} else {
		logger.Load(ctx).DebugContext(ctx, "event source poller: invoked Lambda",
			"function", fnName, "records", len(records))
	}
}

// streamNameFromARN extracts the stream name from a Kinesis ARN.
// Example: arn:aws:kinesis:us-east-1:000000000000:stream/my-stream → my-stream.
func streamNameFromARN(arn string) string {
	const prefix = "arn:aws:kinesis:"
	if len(arn) <= len(prefix) {
		return ""
	}

	// Format: arn:aws:kinesis:region:account:stream/name
	parts := strings.SplitN(arn, ":", arnKinesisPartCount)
	if len(parts) < arnKinesisPartCount {
		return ""
	}

	last := parts[arnKinesisPartCount-1]
	const streamPrefix = "stream/"
	if len(last) <= len(streamPrefix) {
		return ""
	}

	return last[len(streamPrefix):]
}

// functionNameFromARN extracts the bare function name from a Lambda ARN,
// stripping any trailing version/alias qualifier segment.
// Example: arn:aws:lambda:us-east-1:000000000000:function:my-func:PROD → my-func.
//
// Bug fix (parity-sweep-3): this used to SplitN on ":" with a fixed part
// count, which for a qualified ARN left the qualifier glued onto the name
// (e.g. "my-func:PROD") and broke every existence/lookup check that expects
// a bare function name (janitor health checks, tag lookups). It now
// delegates to functionNameAndQualifierFromARN and returns just the name;
// callers that need the qualifier (event delivery) use that helper directly.
func functionNameFromARN(arn string) string {
	name, _ := functionNameAndQualifierFromARN(arn)

	return name
}

// isSQSARN reports whether the given ARN identifies an SQS queue.
func isSQSARN(resourceARN string) bool {
	return strings.HasPrefix(resourceARN, "arn:aws:sqs:")
}

// isDynamoDBStreamARN reports whether the given ARN identifies a DynamoDB stream.
func isDynamoDBStreamARN(resourceARN string) bool {
	return strings.HasPrefix(resourceARN, "arn:aws:dynamodb:") && strings.Contains(resourceARN, "/stream/")
}

// processDynamoDBStreamMapping polls all shards of a DynamoDB stream and invokes Lambda.
// Shard IDs are discovered via DescribeStreamShards on each poll cycle; new shards are
// detected automatically. Records are batched per shard to preserve ordering guarantees.
func (p *EventSourcePoller) processDynamoDBStreamMapping(
	ctx context.Context,
	m *EventSourceMapping,
	reader DynamoDBStreamsReader,
) {
	shardIDs, err := reader.DescribeStreamShards(m.EventSourceARN)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "event source poller: failed to describe DDB stream shards",
			"stream", m.EventSourceARN, "error", err)

		return
	}

	if len(shardIDs) == 0 {
		// No shards yet (stream may be initialising); nothing to poll.
		return
	}

	for _, shardID := range shardIDs {
		p.processDDBShard(ctx, m, reader, shardID)
	}
}

// processDDBShard polls a single DynamoDB stream shard and invokes Lambda with any records.
func (p *EventSourcePoller) processDDBShard(
	ctx context.Context,
	m *EventSourceMapping,
	reader DynamoDBStreamsReader,
	shardID string,
) {
	iterKey := m.UUID + ":" + shardID

	var (
		it     string
		exists bool
	)

	func() {
		p.mu.RLock("processDDBShard.read")
		defer p.mu.RUnlock()

		it, exists = p.shardIterators[iterKey]
	}()

	var err error

	if !exists {
		it, err = reader.GetStreamShardIterator(m.EventSourceARN, shardID, m.StartingPosition)
		if err != nil {
			logger.Load(ctx).WarnContext(ctx, "event source poller: failed to get DDB shard iterator",
				"stream", m.EventSourceARN, "shard", shardID, "error", err)

			return
		}

		func() {
			p.mu.Lock("processDDBShard.initIter")
			defer p.mu.Unlock()

			p.shardIterators[iterKey] = it
		}()
	}

	records, nextIt, readErr := reader.GetStreamRecords(it, m.BatchSize)
	if readErr != nil {
		func() {
			p.mu.Lock("processDDBShard.resetIter")
			defer p.mu.Unlock()

			delete(p.shardIterators, iterKey)
		}()
		logger.Load(ctx).WarnContext(ctx, "event source poller: DDB GetStreamRecords failed, resetting iterator",
			"stream", m.EventSourceARN, "shard", shardID, "error", readErr)

		return
	}

	func() {
		p.mu.Lock("processDDBShard.advanceIter")
		defer p.mu.Unlock()

		p.shardIterators[iterKey] = nextIt
	}()

	if len(records) == 0 {
		return
	}

	p.invokeLambdaForDDB(ctx, m, records)
}

// invokeLambdaForDDB formats DynamoDB stream records as a Lambda event and invokes the function.
func (p *EventSourcePoller) invokeLambdaForDDB(
	ctx context.Context,
	m *EventSourceMapping,
	records []DynamoDBStreamRecord,
) {
	type ddbStreamRecord struct {
		Keys                        map[string]any `json:"Keys,omitempty"`
		NewImage                    map[string]any `json:"NewImage,omitempty"`
		OldImage                    map[string]any `json:"OldImage,omitempty"`
		SequenceNumber              string         `json:"SequenceNumber"`
		StreamViewType              string         `json:"StreamViewType,omitempty"`
		ApproximateCreationDateTime float64        `json:"ApproximateCreationDateTime,omitempty"`
		SizeBytes                   int64          `json:"SizeBytes,omitempty"`
	}
	type lambdaRecord struct {
		EventID        string          `json:"eventID"`
		EventName      string          `json:"eventName"`
		EventVersion   string          `json:"eventVersion"`
		EventSource    string          `json:"eventSource"`
		AWSRegion      string          `json:"awsRegion"`
		EventSourceARN string          `json:"eventSourceARN"`
		Dynamodb       ddbStreamRecord `json:"dynamodb"`
	}
	type lambdaEvent struct {
		Records []lambdaRecord `json:"Records"`
	}

	eventRecords := make([]lambdaRecord, 0, len(records))
	for _, r := range records {
		// Apply FilterCriteria: records matching no filter are skipped.
		if !eventFilterMatches(m.FilterCriteria, dynamoDBFilterView(r.EventName, r.NewImage, r.OldImage, r.Keys)) {
			continue
		}

		eventRecords = append(eventRecords, lambdaRecord{
			EventID:        r.EventID,
			EventName:      r.EventName,
			EventVersion:   "1.1",
			EventSource:    "aws:dynamodb",
			AWSRegion:      p.lambdaBackend.region,
			EventSourceARN: m.EventSourceARN,
			Dynamodb: ddbStreamRecord{
				SequenceNumber:              r.SequenceNumber,
				ApproximateCreationDateTime: r.ApproximateCreationDateTime,
				StreamViewType:              r.StreamViewType,
				SizeBytes:                   r.SizeBytes,
				Keys:                        r.Keys,
				NewImage:                    r.NewImage,
				OldImage:                    r.OldImage,
			},
		})
	}

	if len(eventRecords) == 0 {
		return
	}

	payload, err := json.Marshal(lambdaEvent{Records: eventRecords})
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "event source poller: failed to marshal DDB event", "error", err)

		return
	}

	fnName, qualifier := functionNameAndQualifierFromARN(m.FunctionARN)
	if fnName == "" {
		fnName = m.FunctionARN
	}

	var invokeErr error
	if p.ddbInvoker != nil {
		invokeErr = p.ddbInvoker(ctx, fnName, payload)
	} else {
		_, invokeErr = p.invokeESMFunctionEvent(ctx, fnName, qualifier, payload)
	}

	if invokeErr != nil {
		logger.Load(ctx).WarnContext(ctx, "event source poller: DDB Lambda invocation failed",
			"function", fnName, "stream", m.EventSourceARN, "error", invokeErr)
	} else {
		logger.Load(ctx).DebugContext(ctx, "event source poller: invoked Lambda for DDB stream",
			"function", fnName, "records", len(records))
	}
}

// processSQSMapping polls an SQS queue, invokes Lambda with the messages, and
// deletes messages that were successfully processed. When ReportBatchItemFailures
// is in the ESM's FunctionResponseTypes and the invocation response contains a
// batchItemFailures list, only messages NOT in that list are deleted; the failed
// ones remain in the queue and become visible again after their visibility timeout.
func (p *EventSourcePoller) processSQSMapping(ctx context.Context, m *EventSourceMapping, reader SQSReader) {
	msgs, err := reader.ReceiveMessagesLocal(m.EventSourceARN, m.BatchSize)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "esm sqs: failed to receive messages",
			"queue", m.EventSourceARN, "error", err)

		return
	}

	log := logger.Load(ctx)

	// Enforce MaximumRecordAgeInSeconds: messages older than the limit are dropped
	// (deleted) instead of invoked, matching AWS's record-age expiry.
	kept, expired := splitByRecordAge(msgs, m.MaximumRecordAgeInSeconds)
	if len(expired) > 0 {
		if delErr := reader.DeleteMessagesLocal(m.EventSourceARN, receiptHandles(expired)); delErr != nil {
			log.WarnContext(ctx, "esm sqs: failed to delete expired messages", "error", delErr)
		}
	}

	// Apply FilterCriteria: records that match no filter are discarded (deleted)
	// without invoking the function, exactly as AWS drops filtered-out records.
	matched, filtered := splitByFilter(m.FilterCriteria, kept)
	if len(filtered) > 0 {
		if delErr := reader.DeleteMessagesLocal(m.EventSourceARN, receiptHandles(filtered)); delErr != nil {
			log.WarnContext(ctx, "esm sqs: failed to delete filtered messages", "error", delErr)
		}
	}

	// Honor MaximumBatchingWindowInSeconds: buffer partial batches until the batch
	// fills or the window elapses before invoking.
	batch, flush := p.accumulateSQSBatch(m, matched)
	if !flush || len(batch) == 0 {
		return
	}

	toDelete := p.deliverSQSBatch(ctx, m, batch)
	if len(toDelete) == 0 {
		return
	}

	if delErr := reader.DeleteMessagesLocal(m.EventSourceARN, toDelete); delErr != nil {
		log.WarnContext(ctx, "esm sqs: failed to delete messages",
			"queue", m.EventSourceARN, "error", delErr)
	}
}

// deliverSQSBatch invokes the function for a batch of SQS messages, honoring
// BisectBatchOnFunctionError. It returns the receipt handles that should be
// deleted from the queue.
func (p *EventSourcePoller) deliverSQSBatch(
	ctx context.Context,
	m *EventSourceMapping,
	batch []*SQSMessage,
) []string {
	toDelete, invErr := p.invokeLambdaForSQS(ctx, m, batch)
	if invErr == nil {
		return toDelete
	}

	log := logger.Load(ctx)

	// BisectBatchOnFunctionError: on a whole-batch function error, split the batch
	// in half and retry each half so a single poison message does not fail the
	// entire batch. Batches of one that still fail are left for redelivery.
	if m.BisectBatchOnFunctionError && len(batch) > 1 {
		const bisectParts = 2

		mid := len(batch) / bisectParts
		left := p.deliverSQSBatch(ctx, m, batch[:mid])
		right := p.deliverSQSBatch(ctx, m, batch[mid:])

		return append(left, right...)
	}

	log.WarnContext(ctx, "esm sqs: Lambda invocation failed",
		"function", m.FunctionARN, "error", invErr)

	return nil
}

// sqsEventRecord is a single record in the SQS event payload delivered to Lambda.
type sqsEventRecord struct {
	Attributes             map[string]string              `json:"attributes,omitempty"`
	MessageAttributes      map[string]SQSMessageAttribute `json:"messageAttributes"`
	MessageID              string                         `json:"messageId"`
	ReceiptHandle          string                         `json:"receiptHandle"`
	Body                   string                         `json:"body"`
	MD5OfBody              string                         `json:"md5OfBody"`
	MD5OfMessageAttributes string                         `json:"md5OfMessageAttributes,omitempty"`
	EventSource            string                         `json:"eventSource"`
	EventSourceARN         string                         `json:"eventSourceARN"`
	AWSRegion              string                         `json:"awsRegion"`
}

// buildSQSEventPayload marshals a batch of SQS messages into the Lambda SQS event
// payload. Each record includes the user message attributes and their MD5, and
// carries the supplied region (derived from the queue ARN).
func buildSQSEventPayload(region, esmARN string, msgs []*SQSMessage) ([]byte, error) {
	type sqsEvent struct {
		Records []sqsEventRecord `json:"Records"`
	}

	records := make([]sqsEventRecord, len(msgs))

	for i, msg := range msgs {
		msgAttrs := msg.MessageAttributes
		if msgAttrs == nil {
			// AWS always emits messageAttributes as an object (empty when none).
			msgAttrs = map[string]SQSMessageAttribute{}
		}

		records[i] = sqsEventRecord{
			MessageID:              msg.MessageID,
			ReceiptHandle:          msg.ReceiptHandle,
			Body:                   msg.Body,
			Attributes:             msg.Attributes,
			MessageAttributes:      msgAttrs,
			MD5OfBody:              msg.MD5OfBody,
			MD5OfMessageAttributes: msg.MD5OfMessageAttributes,
			EventSource:            "aws:sqs",
			EventSourceARN:         esmARN,
			AWSRegion:              region,
		}
	}

	payload, err := json.Marshal(sqsEvent{Records: records})
	if err != nil {
		return nil, fmt.Errorf("marshal sqs event: %w", err)
	}

	return payload, nil
}

// batchItemFailures is the structure returned by a Lambda function when
// FunctionResponseTypes includes ReportBatchItemFailures.
type batchItemFailures struct {
	BatchItemFailures []struct {
		ItemIdentifier string `json:"itemIdentifier"`
	} `json:"batchItemFailures"`
}

// invokeLambdaForSQS formats SQS messages as a Lambda SQS event and invokes the function.
// It returns the receipt handles of messages that should be deleted from the queue.
// When ReportBatchItemFailures is in m.FunctionResponseTypes and the response body
// contains a batchItemFailures list, only handles NOT in that list are returned.
func (p *EventSourcePoller) invokeLambdaForSQS(
	ctx context.Context,
	m *EventSourceMapping,
	msgs []*SQSMessage,
) ([]string, error) {
	// SQS event records carry the ARN's region, not the backend default region.
	region := sqsRegionFromARN(m.EventSourceARN)
	if region == "" {
		region = p.lambdaBackend.region
	}

	payload, err := buildSQSEventPayload(region, m.EventSourceARN, msgs)
	if err != nil {
		return nil, err
	}

	fnName, qualifier := functionNameAndQualifierFromARN(m.FunctionARN)
	if fnName == "" {
		fnName = m.FunctionARN
	}

	reportFailures := slices.Contains(m.FunctionResponseTypes, "ReportBatchItemFailures")

	var (
		respBody  []byte
		invokeErr error
	)

	if p.sqsInvoker != nil {
		respBody, invokeErr = p.sqsInvoker(ctx, fnName)
	} else {
		respBody, invokeErr = p.invokeESMFunctionEvent(ctx, fnName, qualifier, payload)
	}

	if invokeErr != nil {
		return nil, invokeErr
	}

	logger.Load(ctx).DebugContext(ctx, "esm sqs: invoked Lambda",
		"function", fnName, "messages", len(msgs))

	// Apply partial-batch failure filtering when enabled.
	if handles := filterByBatchItemFailures(reportFailures, respBody, msgs); handles != nil {
		return handles, nil
	}

	// Default: delete all delivered messages.
	allHandles := make([]string, len(msgs))
	for i, msg := range msgs {
		allHandles[i] = msg.ReceiptHandle
	}

	return allHandles, nil
}

// filterByBatchItemFailures parses the ReportBatchItemFailures response and returns
// receipt handles for messages that succeeded. Returns nil if filtering is not applicable.
func filterByBatchItemFailures(reportFailures bool, respBody []byte, msgs []*SQSMessage) []string {
	if !reportFailures || len(respBody) == 0 {
		return nil
	}

	var failures batchItemFailures
	if jsonErr := json.Unmarshal(respBody, &failures); jsonErr != nil || len(failures.BatchItemFailures) == 0 {
		return nil
	}

	failedIDs := make(map[string]struct{}, len(failures.BatchItemFailures))
	for _, f := range failures.BatchItemFailures {
		failedIDs[f.ItemIdentifier] = struct{}{}
	}

	toDelete := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		if _, failed := failedIDs[msg.MessageID]; !failed {
			toDelete = append(toDelete, msg.ReceiptHandle)
		}
	}

	return toDelete
}
