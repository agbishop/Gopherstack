package firehose

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// FlushAll forces delivery of all buffered records across all streams in all regions.
// Used by tests and for graceful shutdown.
func (b *InMemoryBackend) FlushAll(ctx context.Context) {
	type streamRef struct {
		region string
		name   string
	}

	var refs []streamRef

	func() {
		b.mu.RLock("FlushAll")
		defer b.mu.RUnlock()

		all := b.streams.All()
		refs = make([]streamRef, 0, len(all))
		for _, s := range all {
			refs = append(refs, streamRef{region: s.Region, name: s.Name})
		}
	}()

	for _, ref := range refs {
		b.flushStream(ctx, ref.region, ref.name)
	}
}

// RunFlusher starts the background interval flusher goroutine.
func (b *InMemoryBackend) RunFlusher(ctx context.Context) {
	go b.intervalFlusher(ctx)
}

// intervalFlusher periodically flushes streams whose interval threshold has been reached.
func (b *InMemoryBackend) intervalFlusher(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, ref := range b.dueFlushRefs() {
				b.flushStream(ctx, ref.region, ref.name)
			}
		}
	}
}

// flushRef identifies a delivery stream due for an interval-based flush.
type flushRef struct {
	region string
	name   string
}

// dueFlushRefs returns the streams flagged as holding buffered records whose
// interval threshold has been reached, checked under the read lock.
func (b *InMemoryBackend) dueFlushRefs() []flushRef {
	b.mu.RLock("intervalFlusher")
	defer b.mu.RUnlock()

	var refs []flushRef

	// Only inspect streams flagged as holding buffered records, rather than
	// scanning every region×stream on each tick.
	for region, pending := range b.pendingFlush {
		for name := range pending {
			s, ok := b.streams.Get(regionKey(region, name))
			if ok && b.shouldFlushByIntervalLocked(s) {
				refs = append(refs, flushRef{region: region, name: name})
			}
		}
	}

	return refs
}

// isBackupEnabledLocked returns true when S3 backup mode is enabled for the stream.
// Must be called with the write lock held.
func (b *InMemoryBackend) isBackupEnabledLocked(s *DeliveryStream) bool {
	if s.S3Destination != nil && strings.EqualFold(s.S3Destination.S3BackupMode, "Enabled") {
		return true
	}
	if s.HTTPEndpointDestination != nil && strings.EqualFold(s.HTTPEndpointDestination.S3BackupMode, "Enabled") {
		return true
	}

	return false
}

// bufferingHints returns the effective buffering hints for a stream, checking all
// configured destination types in priority order.
func bufferingHints(s *DeliveryStream) *BufferingHints {
	if s.S3Destination != nil && s.S3Destination.BufferingHints != nil {
		return s.S3Destination.BufferingHints
	}

	if s.HTTPEndpointDestination != nil && s.HTTPEndpointDestination.BufferingHints != nil {
		return s.HTTPEndpointDestination.BufferingHints
	}

	if s.OpenSearchDestination != nil && s.OpenSearchDestination.BufferingHints != nil {
		return s.OpenSearchDestination.BufferingHints
	}

	if s.ElasticsearchDestination != nil && s.ElasticsearchDestination.BufferingHints != nil {
		return s.ElasticsearchDestination.BufferingHints
	}

	if s.IcebergDestination != nil && s.IcebergDestination.BufferingHints != nil {
		return s.IcebergDestination.BufferingHints
	}

	if s.SnowflakeDestination != nil && s.SnowflakeDestination.BufferingHints != nil {
		h := s.SnowflakeDestination.BufferingHints

		return &BufferingHints{SizeInMBs: h.SizeInMBs, IntervalInSeconds: h.IntervalInSeconds}
	}

	return nil
}

// shouldFlushLocked returns true when a size-based flush should happen.
// Must be called with the write lock held.
func (b *InMemoryBackend) shouldFlushLocked(s *DeliveryStream) bool {
	if len(s.Records) == 0 || !b.hasActiveDestinationLocked(s) {
		return false
	}

	hints := bufferingHints(s)
	sizeLimit := 5 // default 5 MB
	if hints != nil && hints.SizeInMBs > 0 {
		sizeLimit = hints.SizeInMBs
	}

	return s.bufferSizeBytes >= sizeLimit*1024*1024
}

// shouldFlushByIntervalLocked returns true when an interval-based flush should happen.
// Must be called with the read lock held.
func (b *InMemoryBackend) shouldFlushByIntervalLocked(s *DeliveryStream) bool {
	if len(s.Records) == 0 || !b.hasActiveDestinationLocked(s) {
		return false
	}

	hints := bufferingHints(s)
	interval := 300 // default 300 seconds
	if hints != nil && hints.IntervalInSeconds > 0 {
		interval = hints.IntervalInSeconds
	}

	return time.Since(s.lastFlush) >= time.Duration(interval)*time.Second
}

// flushSnapshot holds a point-in-time snapshot of records extracted from a stream.
type flushSnapshot struct {
	s3Dest            *S3DestinationDescription
	httpDest          *HTTPEndpointDestinationDescription
	redshiftDest      *RedshiftDestinationDescription
	openSearchDest    *OpenSearchDestinationDescription
	elasticsearchDest *ElasticsearchDestinationDescription
	splunkDest        *SplunkDestinationDescription
	icebergDest       *IcebergDestinationDescription
	snowflakeDest     *SnowflakeDestinationDescription
	streamARN         string
	streamName        string
	region            string
	records           [][]byte
	// backupRecords are the source records copied for an S3 backup destination
	// (S3BackupMode=Enabled); they are delivered to the backup bucket verbatim.
	backupRecords [][]byte
}

// extractForFlushLocked snapshots and resets the stream buffer when shouldFlushLocked
// returns true. Returns nil when no flush is needed. Must be called with the write lock held.
func (b *InMemoryBackend) extractForFlushLocked(s *DeliveryStream) *flushSnapshot {
	if !b.shouldFlushLocked(s) {
		return nil
	}

	return b.extractAllRecordsLocked(s)
}

// hasActiveDestinationLocked reports whether the stream has at least one configured
// delivery destination. Must be called with the read lock (or write lock) held. Split
// across hasCoreActiveDestinationLocked/hasSearchOrLakeActiveDestination so neither half
// grows past the cyclomatic-complexity budget as destination families are added.
func (b *InMemoryBackend) hasActiveDestinationLocked(s *DeliveryStream) bool {
	return b.hasCoreActiveDestinationLocked(s) || hasSearchOrLakeActiveDestination(s)
}

// hasCoreActiveDestinationLocked checks the original S3/HTTP/Redshift destination family.
// Must be called with the read lock (or write lock) held.
func (b *InMemoryBackend) hasCoreActiveDestinationLocked(s *DeliveryStream) bool {
	if s.S3Destination != nil && b.s3 != nil {
		return true
	}

	if s.HTTPEndpointDestination != nil &&
		s.HTTPEndpointDestination.EndpointConfiguration != nil &&
		s.HTTPEndpointDestination.EndpointConfiguration.URL != "" {
		return true
	}

	return s.RedshiftDestination != nil && s.RedshiftDestination.ClusterJDBCURL != ""
}

// hasSearchOrLakeActiveDestination checks the search-engine (OpenSearch/Elasticsearch/
// Splunk) and data-lake (Iceberg/Snowflake) destination families. Does not touch backend
// state, so no lock requirement.
func hasSearchOrLakeActiveDestination(s *DeliveryStream) bool {
	if s.OpenSearchDestination != nil &&
		(s.OpenSearchDestination.DomainARN != "" || s.OpenSearchDestination.ClusterEndpoint != "") {
		return true
	}

	if s.ElasticsearchDestination != nil &&
		(s.ElasticsearchDestination.DomainARN != "" || s.ElasticsearchDestination.ClusterEndpoint != "") {
		return true
	}

	if s.SplunkDestination != nil && s.SplunkDestination.HECEndpoint != "" {
		return true
	}

	return hasLakeS3Destination(s.IcebergDestination.getS3()) || hasLakeS3Destination(s.SnowflakeDestination.getS3())
}

// hasLakeS3Destination reports whether a data-lake destination's required S3 staging
// location has a bucket configured.
func hasLakeS3Destination(s3Dest *S3DestinationDescription) bool {
	return s3Dest != nil && s3Dest.BucketARN != ""
}

// getS3 returns d's required S3 staging destination, or nil when d itself is nil.
func (d *IcebergDestinationDescription) getS3() *S3DestinationDescription {
	if d == nil {
		return nil
	}

	return d.S3Destination
}

// getS3 returns d's required S3 staging destination, or nil when d itself is nil.
func (d *SnowflakeDestinationDescription) getS3() *S3DestinationDescription {
	if d == nil {
		return nil
	}

	return d.S3Destination
}

// extractAllRecordsLocked unconditionally snapshots and resets the stream buffer.
// Returns nil when there are no records or no active delivery destination.
// Must be called with the write lock held.
func (b *InMemoryBackend) extractAllRecordsLocked(s *DeliveryStream) *flushSnapshot {
	if len(s.Records) == 0 || !b.hasActiveDestinationLocked(s) {
		return nil
	}

	snap := &flushSnapshot{
		records:       s.Records,
		backupRecords: s.BackupRecords,
		streamARN:     s.ARN,
		streamName:    s.Name,
		region:        s.Region,
	}

	if s.S3Destination != nil && b.s3 != nil {
		d := *s.S3Destination
		snap.s3Dest = &d
	}

	if s.HTTPEndpointDestination != nil {
		d := *s.HTTPEndpointDestination
		snap.httpDest = &d
	}

	if s.RedshiftDestination != nil {
		d := *s.RedshiftDestination
		snap.redshiftDest = &d
	}

	if s.OpenSearchDestination != nil {
		d := *s.OpenSearchDestination
		snap.openSearchDest = &d
	}

	if s.ElasticsearchDestination != nil {
		d := *s.ElasticsearchDestination
		snap.elasticsearchDest = &d
	}

	if s.SplunkDestination != nil {
		d := *s.SplunkDestination
		snap.splunkDest = &d
	}

	if s.IcebergDestination != nil {
		d := *s.IcebergDestination
		snap.icebergDest = &d
	}

	if s.SnowflakeDestination != nil {
		d := *s.SnowflakeDestination
		snap.snowflakeDest = &d
	}

	s.Records = [][]byte{}
	s.BackupRecords = [][]byte{}
	s.bufferSizeBytes = 0
	s.lastFlush = time.Now()

	return snap
}

// deliverSnapshot applies optional Lambda transformation and delivers records to all
// configured destinations, routing processing/delivery failures to the S3 error output and
// recording the FailedRecords metric. Called after the write lock has been released.
func (b *InMemoryBackend) deliverSnapshot(ctx context.Context, snap *flushSnapshot, streamName string) {
	if snap.s3Dest != nil {
		b.deliverS3Destination(ctx, snap, streamName)
	}

	if snap.httpDest != nil {
		b.deliverProcessedNonS3(ctx, snap, streamName, snap.httpDest.ProcessingConfiguration,
			snap.httpDest.S3BackupDescription, snap.httpDest.CloudWatchLoggingOptions,
			func(recs [][]byte) {
				b.deliverToHTTPEndpoint(ctx, recs, snap.httpDest, snap.streamARN)
			})
	}

	if snap.redshiftDest != nil {
		b.deliverProcessedNonS3(ctx, snap, streamName, snap.redshiftDest.ProcessingConfiguration,
			snap.redshiftDest.S3BackupDescription, nil,
			func(recs [][]byte) {
				b.deliverToRedshift(ctx, recs, snap.redshiftDest, snap.streamARN, streamName)
			})
	}

	if snap.openSearchDest != nil {
		b.deliverProcessedNonS3(ctx, snap, streamName, snap.openSearchDest.ProcessingConfiguration,
			snap.openSearchDest.S3BackupDescription, snap.openSearchDest.CloudWatchLoggingOptions,
			func(recs [][]byte) {
				b.deliverToOpenSearch(ctx, recs, snap.openSearchDest, snap.streamARN)
			})
	}

	if snap.elasticsearchDest != nil {
		b.deliverProcessedNonS3(ctx, snap, streamName, snap.elasticsearchDest.ProcessingConfiguration,
			snap.elasticsearchDest.S3BackupDescription, snap.elasticsearchDest.CloudWatchLoggingOptions,
			func(recs [][]byte) {
				b.deliverToElasticsearch(ctx, recs, snap.elasticsearchDest, snap.streamARN)
			})
	}

	if snap.splunkDest != nil {
		b.deliverProcessedNonS3(ctx, snap, streamName, snap.splunkDest.ProcessingConfiguration,
			snap.splunkDest.S3BackupDescription, snap.splunkDest.CloudWatchLoggingOptions,
			func(recs [][]byte) {
				b.deliverToSplunk(ctx, recs, snap.splunkDest, snap.streamARN)
			})
	}

	if snap.icebergDest != nil {
		b.deliverProcessedNonS3(ctx, snap, streamName, snap.icebergDest.ProcessingConfiguration,
			nil, snap.icebergDest.CloudWatchLoggingOptions,
			func(recs [][]byte) {
				b.deliverToIceberg(ctx, recs, snap.icebergDest, streamName)
			})
	}

	if snap.snowflakeDest != nil {
		b.deliverProcessedNonS3(ctx, snap, streamName, snap.snowflakeDest.ProcessingConfiguration,
			nil, snap.snowflakeDest.CloudWatchLoggingOptions,
			func(recs [][]byte) {
				b.deliverToSnowflake(ctx, recs, snap.snowflakeDest, streamName)
			})
	}
}

// deliverProcessedNonS3 runs the shared delivery pipeline for non-S3 destinations: it
// applies the Lambda transform, routes processing failures to the S3 backup destination (if
// configured) and records them in the FailedRecords metric, then delivers the surviving
// records via the supplied deliver func. It also delivers any S3 backup copies.
func (b *InMemoryBackend) deliverProcessedNonS3(
	ctx context.Context,
	snap *flushSnapshot,
	streamName string,
	pc *ProcessingConfiguration,
	backup *S3BackupDescription,
	cwLog *CloudWatchLoggingOptions,
	deliver func(records [][]byte),
) {
	ok, failed, err := b.applyTransform(ctx, snap.records, pc, snap.streamARN, snap.region)
	if err != nil {
		b.logDeliveryIssue(ctx, cwLog, streamName,
			"lambda transform invocation failed; routing records to backup", err)
		failed = append(failed, snap.records...)
		ok = nil
	}

	if len(failed) > 0 {
		if backup != nil {
			_, _ = b.writeRecordsToBucket(ctx, failed, backup.BucketARN,
				backup.Prefix, "", backup.CompressionFormat, streamName)
		}
		b.recordFailedRecords(snap.region, streamName, len(failed))
	}

	if len(ok) > 0 {
		deliver(ok)
	}

	b.deliverS3Backup(ctx, snap, backup, streamName)
}

// deliverS3Backup delivers the buffered S3 backup copies (accumulated when S3BackupMode is
// Enabled) to the backup bucket. It is a no-op when there are no backup records or no
// backup destination is configured.
func (b *InMemoryBackend) deliverS3Backup(
	ctx context.Context,
	snap *flushSnapshot,
	backup *S3BackupDescription,
	streamName string,
) {
	if len(snap.backupRecords) == 0 || backup == nil || backup.BucketARN == "" {
		return
	}

	_, _ = b.writeRecordsToBucket(ctx, snap.backupRecords, backup.BucketARN,
		backup.Prefix, "", backup.CompressionFormat, streamName)
}

// applyTransform runs the configured Lambda transform over records, separating the records
// to deliver (ok) from records that failed processing (failed, to be routed to the error
// output). When no transform is configured, all records pass through as ok. A non-nil error
// indicates the invocation itself failed; callers route all source records to the error
// output in that case.
func (b *InMemoryBackend) applyTransform(
	ctx context.Context,
	records [][]byte,
	pc *ProcessingConfiguration,
	streamARN, region string,
) ([][]byte, [][]byte, error) {
	if b.lambda == nil || pc == nil || !pc.Enabled {
		return records, nil, nil
	}

	functionName := lambdaFunctionName(pc)
	if functionName == "" {
		return records, nil, nil
	}

	payload, idToOriginal := buildLambdaTransformPayload(records, streamARN, region)
	if payload == nil {
		return nil, nil, ErrTransformPayload
	}

	result, _, invokeErr := b.lambda.InvokeFunction(ctx, functionName, "RequestResponse", payload)
	if invokeErr != nil {
		return nil, nil, fmt.Errorf("lambda transform invocation failed: %w", invokeErr)
	}

	outcome, parsed := parseLambdaTransformResponse(result, idToOriginal)
	if !parsed {
		return nil, nil, fmt.Errorf("%w: malformed lambda transform response", ErrTransformPayload)
	}

	return outcome.Ok, outcome.Failed, nil
}

// lambdaFunctionName extracts the Lambda function ARN from a ProcessingConfiguration.
func lambdaFunctionName(pc *ProcessingConfiguration) string {
	for _, proc := range pc.Processors {
		if proc.Type != "Lambda" {
			continue
		}
		for _, p := range proc.Parameters {
			if p.ParameterName == "LambdaArn" {
				return p.ParameterValue
			}
		}
	}

	return ""
}

// recordFailedRecords increments the FailedRecords delivery metric for a stream.
func (b *InMemoryBackend) recordFailedRecords(region, streamName string, n int) {
	if n <= 0 {
		return
	}

	b.mu.Lock("recordFailedRecords")
	defer b.mu.Unlock()

	if s, ok := b.streams.Get(regionKey(region, streamName)); ok {
		s.Metrics.FailedRecords += int64(n)
	}
}

// logDeliveryIssue emits a delivery error to the logger, honouring the destination's
// CloudWatch logging options: when logging is enabled the configured log group/stream are
// attached so operators can correlate the failure, matching the CloudWatch error log that
// Firehose writes for failed deliveries. When CloudWatch Logs has been wired in (see
// SetCWLogsBackend), the same event is also delivered there; when it hasn't, this stays a
// local-log-only no-op rather than failing the flush.
func (b *InMemoryBackend) logDeliveryIssue(
	ctx context.Context,
	cwLog *CloudWatchLoggingOptions,
	streamName, msg string,
	err error,
) {
	attrs := []any{"stream", streamName, "error", err}
	if cwLog != nil && cwLog.Enabled {
		attrs = append(attrs, "logGroup", cwLog.LogGroupName, "logStream", cwLog.LogStreamName)
		b.deliverCWLogEvent(cwLog, streamName, msg, err)
	}

	logger.Load(ctx).WarnContext(ctx, "firehose: "+msg, attrs...)
}

// deliverCWLogEvent writes a delivery-failure event to the destination's configured
// CloudWatch Logs group/stream. A no-op when CloudWatch Logs has not been wired in, or when
// the destination didn't specify a log group/stream despite Enabled being set.
func (b *InMemoryBackend) deliverCWLogEvent(cwLog *CloudWatchLoggingOptions, streamName, msg string, err error) {
	if b.cwLogs == nil || cwLog.LogGroupName == "" || cwLog.LogStreamName == "" {
		return
	}

	if ensureErr := b.cwLogs.EnsureLogGroupAndStream(cwLog.LogGroupName, cwLog.LogStreamName); ensureErr != nil {
		return
	}

	line := fmt.Sprintf("firehose: %s (stream=%s): %v", msg, streamName, err)
	_ = b.cwLogs.PutLogLines(cwLog.LogGroupName, cwLog.LogStreamName, []string{line})
}

// flushStream delivers all buffered records for a stream to S3.
func (b *InMemoryBackend) flushStream(ctx context.Context, region, streamName string) {
	var snap *flushSnapshot

	func() {
		b.mu.Lock("flushStream")
		defer b.mu.Unlock()

		s, ok := b.streams.Get(regionKey(region, streamName))
		if !ok {
			return
		}

		snap = b.extractAllRecordsLocked(s)
		b.clearPendingFlushLocked(region, streamName)
	}()

	if snap != nil {
		b.deliverSnapshot(ctx, snap, streamName)
	}
}
