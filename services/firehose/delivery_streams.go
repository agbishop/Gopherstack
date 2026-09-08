package firehose

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/collections"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// CreateDeliveryStreamInput holds the input for creating a delivery stream.
type CreateDeliveryStreamInput struct {
	S3Destination            *S3DestinationDescription
	HTTPEndpointDestination  *HTTPEndpointDestinationDescription
	RedshiftDestination      *RedshiftDestinationDescription
	OpenSearchDestination    *OpenSearchDestinationDescription
	ElasticsearchDestination *ElasticsearchDestinationDescription
	SplunkDestination        *SplunkDestinationDescription
	IcebergDestination       *IcebergDestinationDescription
	SnowflakeDestination     *SnowflakeDestinationDescription
	Source                   *SourceDescription
	Name                     string
	DeliveryStreamType       string
}

// CreateDeliveryStream creates a new delivery stream.
func (b *InMemoryBackend) CreateDeliveryStream(
	ctx context.Context, input CreateDeliveryStreamInput,
) (*DeliveryStream, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, fmt.Errorf("%w: DeliveryStreamName is required", ErrValidation)
	}

	var (
		region                 string
		result                 *DeliveryStream
		kinesisStreamARN       string
		shouldPoll             bool
		kinesisBackendNotWired bool
		err                    error
	)

	func() {
		b.mu.Lock("CreateDeliveryStream")
		defer b.mu.Unlock()

		region = getRegionFromContext(ctx, b)

		if _, ok := b.streams.Get(regionKey(region, input.Name)); ok {
			err = fmt.Errorf("%w: stream %s already exists", ErrAlreadyExists, input.Name)

			return
		}

		if input.S3Destination != nil && input.S3Destination.DestinationID == "" {
			input.S3Destination.DestinationID = "destinationId-000000000001"
		}

		streamType := input.DeliveryStreamType
		if streamType == "" {
			streamType = deliveryStreamTypeDirectPut
		}

		now := time.Now()
		streamARN := arn.Build("firehose", region, b.accountID, "deliverystream/"+input.Name)
		s := &DeliveryStream{
			Name:                     input.Name,
			ARN:                      streamARN,
			DeliveryStreamType:       streamType,
			VersionID:                "1",
			Status:                   "ACTIVE",
			Records:                  [][]byte{},
			BackupRecords:            [][]byte{},
			Tags:                     tags.New("firehose." + region + "." + input.Name + ".tags"),
			AccountID:                b.accountID,
			Region:                   region,
			S3Destination:            input.S3Destination,
			HTTPEndpointDestination:  input.HTTPEndpointDestination,
			RedshiftDestination:      input.RedshiftDestination,
			OpenSearchDestination:    input.OpenSearchDestination,
			ElasticsearchDestination: input.ElasticsearchDestination,
			SplunkDestination:        input.SplunkDestination,
			IcebergDestination:       input.IcebergDestination,
			SnowflakeDestination:     input.SnowflakeDestination,
			Source:                   input.Source,
			CreateTimestamp:          now,
			LastUpdateTimestamp:      now,
			lastFlush:                now,
		}
		b.streams.Put(s)
		b.invalidateNamesCacheLocked(region)

		// Collect Kinesis poller info while holding the lock.
		wantsKinesisSource := streamType == deliveryStreamTypeKinesisSource &&
			input.Source != nil &&
			input.Source.KinesisStreamSourceDescription != nil
		shouldPoll = wantsKinesisSource && b.kinesisBackend != nil
		kinesisBackendNotWired = wantsKinesisSource && b.kinesisBackend == nil

		if shouldPoll {
			kinesisStreamARN = input.Source.KinesisStreamSourceDescription.KinesisStreamARN
		}

		result = streamCopy(s)
	}()

	if err != nil {
		return nil, err
	}

	if shouldPoll {
		b.launchKinesisPoller(region, input.Name, kinesisStreamARN)
	}

	if kinesisBackendNotWired {
		logger.Load(ctx).WarnContext(ctx, "firehose: Kinesis source polling skipped, no Kinesis backend wired",
			"stream", input.Name, "region", region)
	}

	return result, nil
}

// DeleteDeliveryStream deletes a delivery stream.
func (b *InMemoryBackend) DeleteDeliveryStream(ctx context.Context, name string) error {
	var (
		cancel context.CancelFunc
		err    error
	)

	func() {
		b.mu.Lock("DeleteDeliveryStream")
		defer b.mu.Unlock()

		region := getRegionFromContext(ctx, b)

		s, ok := b.streams.Get(regionKey(region, name))
		if !ok {
			err = fmt.Errorf("%w: stream %s not found", ErrNotFound, name)

			return
		}

		if s.Tags != nil {
			s.Tags.Close()
		}

		b.streams.Delete(regionKey(region, name))
		b.invalidateNamesCacheLocked(region)
		b.clearPendingFlushLocked(region, name)

		// Stop Kinesis poller if one is running for this stream.
		pollers := b.pollerStore(region)
		cancel = pollers[name]
		delete(pollers, name)
	}()

	if err != nil {
		return err
	}

	if cancel != nil {
		cancel()
	}

	return nil
}

// DescribeDeliveryStream returns a delivery stream by name.
func (b *InMemoryBackend) DescribeDeliveryStream(ctx context.Context, name string) (*DeliveryStream, error) {
	b.mu.RLock("DescribeDeliveryStream")
	defer b.mu.RUnlock()

	region := getRegionFromContext(ctx, b)

	s, ok := b.streams.Get(regionKey(region, name))
	if !ok {
		return nil, fmt.Errorf("%w: stream %s not found", ErrNotFound, name)
	}

	return streamCopy(s), nil
}

// ListDeliveryStreams returns all delivery stream names in the request's region
// in alphabetical order.
func (b *InMemoryBackend) ListDeliveryStreams(ctx context.Context) []string {
	return b.ListDeliveryStreamsByType(ctx, "")
}

// ListDeliveryStreamsByType returns delivery stream names in the request's region in
// alphabetical order, optionally filtered to a single DeliveryStreamType (DirectPut or
// KinesisStreamAsSource). An empty streamType returns all streams. The full sorted-name
// list is cached per region and reused across calls until a create/delete invalidates it,
// so repeated listing does not re-sort the whole namespace every time.
func (b *InMemoryBackend) ListDeliveryStreamsByType(ctx context.Context, streamType string) []string {
	region := getRegionFromContext(ctx, b)

	// Fast path: cached full list, no type filter.
	if streamType == "" {
		var (
			out []string
			hit bool
		)

		func() {
			b.mu.RLock("ListDeliveryStreams")
			defer b.mu.RUnlock()

			if cached, ok := b.sortedNamesCache[region]; ok {
				out = make([]string, len(cached))
				copy(out, cached)
				hit = true
			}
		}()

		if hit {
			return out
		}
	}

	b.mu.Lock("ListDeliveryStreams")
	defer b.mu.Unlock()

	items := b.streamsByRegion.Get(region)

	sorted, ok := b.sortedNamesCache[region]
	if !ok {
		sorted = collections.Map(items, func(s *DeliveryStream) string { return s.Name })
		sort.Strings(sorted)
		b.sortedNamesCache[region] = sorted
	}

	if streamType == "" {
		out := make([]string, len(sorted))
		copy(out, sorted)

		return out
	}

	byName := make(map[string]*DeliveryStream, len(items))
	for _, s := range items {
		byName[s.Name] = s
	}

	filtered := make([]string, 0, len(sorted))
	for _, name := range sorted {
		s := byName[name]
		if s == nil {
			continue
		}
		effectiveType := s.DeliveryStreamType
		if effectiveType == "" {
			effectiveType = deliveryStreamTypeDirectPut
		}
		if effectiveType == streamType {
			filtered = append(filtered, name)
		}
	}

	return filtered
}
