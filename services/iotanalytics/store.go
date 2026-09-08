package iotanalytics

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	statusActive          = "ACTIVE"
	statusSucceeded       = "SUCCEEDED"
	errCodeInvalidRequest = "InvalidRequestException"
	maxRetentionDays      = 2147483647
	// defaultRegion is used when the request context carries no region (e.g.
	// an unsigned request); ARNs must still be well-formed.
	defaultRegion = config.DefaultRegion
	// latestVersion is the special versionId string clients may pass to GetDatasetContent
	// / DeleteDatasetContent to select the most recently created content version,
	// regardless of its status.
	latestVersion = "$LATEST"
	// latestSucceededVersion is the special versionId string clients may pass to
	// GetDatasetContent / DeleteDatasetContent to select the most recently created
	// content version whose status is SUCCEEDED. It is also the AWS-documented default
	// when versionId is omitted.
	latestSucceededVersion = "$LATEST_SUCCEEDED"
)

const (
	// maxChannelMessages caps the number of messages stored per channel.
	maxChannelMessages = 1000
	// maxDatasetContents caps the number of content versions stored per dataset.
	maxDatasetContents = 100
	// maxPipelineReprocessings caps reprocessing jobs stored per pipeline.
	maxPipelineReprocessings = 100
	// maxTagsPerResource caps the number of tags per resource, matching AWS limits.
	maxTagsPerResource = 50
	// maxResourceNameLen is the maximum allowed length for resource names.
	maxResourceNameLen = 128
	// maxTagKeyLen is the maximum allowed length for a tag key.
	maxTagKeyLen = 128
	// maxTagValueLen is the maximum allowed length for a tag value.
	maxTagValueLen = 256
	// maxBatchMessages is the maximum number of messages in a BatchPutMessage call.
	maxBatchMessages = 100
	// maxMessagePayloadBytes is the maximum payload size per message (128 KB).
	maxMessagePayloadBytes = 128 * 1024
	// maxBatchPayloadBytes is the maximum total payload size per batch (500 KB).
	maxBatchPayloadBytes = 500 * 1024
	// maxMessageIDLen is the maximum length for a message ID.
	maxMessageIDLen = 128
	// maxSampleMessages is the maximum number of sample messages.
	maxSampleMessages = 10
	// defaultSampleMessages is the default number of sample messages.
	defaultSampleMessages = 10
	// minPipelineActivities/maxPipelineActivities bound CreatePipelineInput/
	// UpdatePipelineInput's PipelineActivities ("The list can be 2-25 PipelineActivity
	// objects and must contain both a channel and a datastore activity",
	// api_op_CreatePipeline.go / api_op_UpdatePipeline.go).
	minPipelineActivities = 2
	maxPipelineActivities = 25
)

// validateResourceName checks that name is 1-128 ASCII letters, digits, or underscores.
// AWS IoT Analytics does not allow hyphens or non-ASCII letters.
func validateResourceName(name string) error {
	if len(name) == 0 || len(name) > maxResourceNameLen {
		return fmt.Errorf("%w: name must be 1-%d characters", ErrValidation, maxResourceNameLen)
	}

	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return fmt.Errorf("%w: name must contain only ASCII letters, digits, or underscores", ErrValidation)
		}

		if r > unicode.MaxASCII {
			return fmt.Errorf("%w: name must contain only ASCII characters", ErrValidation)
		}
	}

	return nil
}

// validateTagKey checks tag key constraints: 1-128 chars, valid charset, no aws: prefix.
func validateTagKey(key string) error {
	if len(key) == 0 || len(key) > maxTagKeyLen {
		return fmt.Errorf("%w: tag key must be 1-%d characters", ErrValidation, maxTagKeyLen)
	}

	if strings.HasPrefix(key, "aws:") {
		return fmt.Errorf("%w: tag key must not start with 'aws:'", ErrValidation)
	}

	for _, r := range key {
		if !isValidTagChar(r) {
			return fmt.Errorf("%w: tag key contains invalid character %q", ErrValidation, r)
		}
	}

	return nil
}

// validateTagValue checks tag value constraints: 0-256 chars, valid charset.
func validateTagValue(value string) error {
	if len(value) > maxTagValueLen {
		return fmt.Errorf("%w: tag value must be 0-%d characters", ErrValidation, maxTagValueLen)
	}

	for _, r := range value {
		if !isValidTagChar(r) {
			return fmt.Errorf("%w: tag value contains invalid character %q", ErrValidation, r)
		}
	}

	return nil
}

// isValidTagChar returns true for characters allowed in tag keys and values.
// AWS allows: [a-zA-Z0-9_.:/=+\-@].
func isValidTagChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) ||
		r == '_' || r == '.' || r == ':' || r == '/' ||
		r == '=' || r == '+' || r == '-' || r == '@'
}

// validateTags validates a slice of tag DTOs, including the AWS-wide 50-tag-per-resource
// limit. Callers that merge this batch with an existing tag set (e.g. TagResource) must
// additionally check the combined count; this only bounds the incoming batch itself, which
// is exactly the limit for a resource created with these tags in a single call.
func validateTags(tags []TagDTO) error {
	if len(tags) > maxTagsPerResource {
		return fmt.Errorf("%w: resource may not have more than %d tags", ErrValidation, maxTagsPerResource)
	}

	for _, t := range tags {
		if err := validateTagKey(t.Key); err != nil {
			return err
		}

		if err := validateTagValue(t.Value); err != nil {
			return err
		}
	}

	return nil
}

// validateRetentionPeriod checks that retention period has exactly one of Unlimited or NumberOfDays set.
func validateRetentionPeriod(rp *RetentionPeriod) error {
	if rp == nil {
		return nil
	}

	if rp.Unlimited && rp.NumberOfDays > 0 {
		return fmt.Errorf("%w: retentionPeriod: exactly one of unlimited or numberOfDays must be set", ErrValidation)
	}

	if !rp.Unlimited && rp.NumberOfDays < 1 {
		return fmt.Errorf("%w: retentionPeriod: numberOfDays must be >= 1 when unlimited is false", ErrValidation)
	}

	if rp.NumberOfDays > maxRetentionDays {
		return fmt.Errorf("%w: retentionPeriod: numberOfDays must be <= 2147483647", ErrValidation)
	}

	return nil
}

// validateDatastorePartitions checks that every partition dimension has exactly one of
// AttributePartition/TimestampPartition set, and that the set variant carries a non-empty
// AttributeName. This mirrors the AWS SDK's client-side validators (validatePartition /
// validateTimestampPartition in the generated iotanalytics client), which require
// AttributeName on both variants; a raw HTTP caller bypassing SDK-side validation would hit
// this same requirement server-side.
func validateDatastorePartitions(p *DatastorePartitions) error {
	if p == nil {
		return nil
	}

	for i, entry := range p.Partitions {
		if err := validateDatastorePartitionEntry(i, entry); err != nil {
			return err
		}
	}

	return nil
}

// validateDatastorePartitionEntry validates a single partition dimension union at index i.
func validateDatastorePartitionEntry(i int, entry DatastorePartitionEntry) error {
	switch {
	case entry.AttributePartition != nil && entry.TimestampPartition != nil:
		return fmt.Errorf(
			"%w: partitions[%d]: exactly one of attributePartition or timestampPartition must be set",
			ErrValidation, i,
		)
	case entry.AttributePartition != nil:
		if entry.AttributePartition.AttributeName == "" {
			return fmt.Errorf("%w: partitions[%d]: attributePartition.attributeName is required", ErrValidation, i)
		}
	case entry.TimestampPartition != nil:
		if entry.TimestampPartition.AttributeName == "" {
			return fmt.Errorf("%w: partitions[%d]: timestampPartition.attributeName is required", ErrValidation, i)
		}
	default:
		return fmt.Errorf(
			"%w: partitions[%d]: exactly one of attributePartition or timestampPartition must be set",
			ErrValidation, i,
		)
	}

	return nil
}

// countSetPipelineActivityFields returns how many of PipelineActivity's union members are set.
func countSetPipelineActivityFields(a PipelineActivity) int {
	set := []bool{
		a.Channel != nil, a.Lambda != nil, a.Datastore != nil, a.AddAttributes != nil,
		a.RemoveAttributes != nil, a.SelectAttributes != nil, a.Filter != nil, a.Math != nil,
		a.DeviceRegistryEnrich != nil, a.DeviceShadowEnrich != nil,
	}

	n := 0

	for _, s := range set {
		if s {
			n++
		}
	}

	return n
}

// validatePipelineActivities checks that activities has 2-25 entries, contains exactly one
// channel activity and exactly one datastore activity, and that no single entry sets more
// than one activity type -- matching CreatePipelineInput/UpdatePipelineInput's documented
// PipelineActivities contract ("The list can be 2-25 PipelineActivity objects and must
// contain both a channel and a datastore activity. Each entry in the list must contain only
// one activity.", api_op_CreatePipeline.go / api_op_UpdatePipeline.go). The SDK's client-side
// validator only checks each activity's own required sub-fields (validatePipelineActivity in
// validators.go), not this aggregate shape, so a real typed client can send a request this
// backend must reject server-side.
func validatePipelineActivities(activities []PipelineActivity) error {
	if len(activities) < minPipelineActivities || len(activities) > maxPipelineActivities {
		return fmt.Errorf(
			"%w: pipelineActivities must contain %d-%d activities",
			ErrValidation, minPipelineActivities, maxPipelineActivities,
		)
	}

	channelCount, datastoreCount := 0, 0

	for i, a := range activities {
		if countSetPipelineActivityFields(a) != 1 {
			return fmt.Errorf("%w: pipelineActivities[%d] must set exactly one activity", ErrValidation, i)
		}

		if a.Channel != nil {
			channelCount++
		}

		if a.Datastore != nil {
			datastoreCount++
		}
	}

	if channelCount != 1 {
		return fmt.Errorf("%w: pipelineActivities must contain exactly one channel activity", ErrValidation)
	}

	if datastoreCount != 1 {
		return fmt.Errorf("%w: pipelineActivities must contain exactly one datastore activity", ErrValidation)
	}

	return nil
}

// InMemoryBackend is the in-memory backend for IoT Analytics.
//
// channels, datastores, datasets, and pipelines are each a *store.Table[T]
// (see store_setup.go): all four key off a real, non-json:"-" identity field
// the value type already carries (Name), so none need a DTO wrapper, and none
// need a secondary store.Index since nothing in this backend does an
// ARN-keyed reverse lookup against them (resolveARNResource parses the
// resource name back out of the ARN string and looks it up directly). tags,
// channelMessages, and datasetContents are left as plain maps: none of their
// value types is a *T (map[string]string, []ChannelMessage, and []*DatasetContent
// respectively), so none fits store.Table's keyed-by-single-identity-value
// shape. See persistence.go for how they round-trip alongside the registered
// tables.
type InMemoryBackend struct {
	loggingOptions  *LoggingOptions
	channelMessages map[string][]ChannelMessage
	datasetContents map[string][]*DatasetContent
	tags            map[string]map[string]string
	channels        *store.Table[Channel]
	datastores      *store.Table[Datastore]
	datasets        *store.Table[Dataset]
	pipelines       *store.Table[Pipeline]
	registry        *store.Registry
	svcCtx          context.Context
	lambda          LambdaInvoker
	thingRegistry   ThingRegistry
	thingShadows    ThingShadowStore
	mu              *lockmetrics.RWMutex
}

// NewInMemoryBackend creates a new in-memory IoT Analytics backend with a background service context.
func NewInMemoryBackend() *InMemoryBackend {
	return NewInMemoryBackendWithContext(context.Background())
}

// NewInMemoryBackendWithContext creates a new in-memory IoT Analytics backend whose
// background goroutines are bounded by svcCtx. If svcCtx is nil, [context.Background] is used.
func NewInMemoryBackendWithContext(svcCtx context.Context) *InMemoryBackend {
	if svcCtx == nil {
		svcCtx = context.Background()
	}

	b := &InMemoryBackend{
		tags:            make(map[string]map[string]string),
		channelMessages: make(map[string][]ChannelMessage),
		datasetContents: make(map[string][]*DatasetContent),
		registry:        store.NewRegistry(),
		svcCtx:          svcCtx,
		mu:              lockmetrics.New("iotanalytics"),
	}

	registerAllTables(b)

	return b
}

// SetLambdaBackend wires the Lambda invoker for RunPipelineActivity's "lambda" activity.
func (b *InMemoryBackend) SetLambdaBackend(lambda LambdaInvoker) {
	b.mu.Lock("SetLambdaBackend")
	defer b.mu.Unlock()

	b.lambda = lambda
}

// SetThingRegistry wires the IoT device registry lookup for RunPipelineActivity's
// "deviceRegistryEnrich" activity.
func (b *InMemoryBackend) SetThingRegistry(registry ThingRegistry) {
	b.mu.Lock("SetThingRegistry")
	defer b.mu.Unlock()

	b.thingRegistry = registry
}

// SetThingShadowStore wires the IoT device shadow lookup for RunPipelineActivity's
// "deviceShadowEnrich" activity.
func (b *InMemoryBackend) SetThingShadowStore(shadows ThingShadowStore) {
	b.mu.Lock("SetThingShadowStore")
	defer b.mu.Unlock()

	b.thingShadows = shadows
}

// Reset clears all backend state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.tags = make(map[string]map[string]string)
	b.channelMessages = make(map[string][]ChannelMessage)
	b.datasetContents = make(map[string][]*DatasetContent)
	b.loggingOptions = nil
}

// arnIdentity resolves the region and account for ARN construction from the
// request ctxbag, falling back to the service default region when none is set.
func arnIdentity(ctx context.Context) (string, string) {
	region := awsmeta.Region(ctx)
	if region == "" {
		region = defaultRegion
	}

	return region, awsmeta.Account(ctx)
}

// resourceARN builds an IoT Analytics ARN for the given resource type and name
// using the request-scoped region and account from the ctxbag.
func resourceARN(ctx context.Context, resourceType, name string) string {
	region, account := arnIdentity(ctx)

	return arn.Build("iotanalytics", region, account, fmt.Sprintf("%s/%s", resourceType, name))
}

// sortedKeys returns the keys of map m in sorted order.
func sortedKeys[K cmp.Ordered, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	return keys
}

// tagsToMap converts a slice of tagDTO to a map.
func tagsToMap(tags []TagDTO) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[t.Key] = t.Value
	}

	return m
}

// mapToTagsSorted converts a map to a slice of tagDTO sorted by key.
func mapToTagsSorted(m map[string]string) []tagDTO {
	keys := sortedKeys(m)
	result := make([]tagDTO, 0, len(m))

	for _, k := range keys {
		result = append(result, tagDTO{Key: k, Value: m[k]})
	}

	return result
}

// cloneRetentionPeriod deep-copies a RetentionPeriod pointer.
func cloneRetentionPeriod(rp *RetentionPeriod) *RetentionPeriod {
	if rp == nil {
		return nil
	}

	cp := *rp

	return &cp
}
