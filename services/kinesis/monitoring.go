package kinesis

import "context"

// uniqueStrings returns a deduplicated copy of ss, preserving order.
func uniqueStrings(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))

	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}

	return out
}

// removeStrings returns a copy of ss with all elements in remove deleted.
func removeStrings(ss, remove []string) []string {
	removeSet := make(map[string]struct{}, len(remove))
	for _, s := range remove {
		removeSet[s] = struct{}{}
	}

	out := make([]string, 0, len(ss))

	for _, s := range ss {
		if _, ok := removeSet[s]; !ok {
			out = append(out, s)
		}
	}

	return out
}

// EnableEnhancedMonitoring adds shard-level metrics to a stream.
func (b *InMemoryBackend) EnableEnhancedMonitoring(
	ctx context.Context,
	input *EnableEnhancedMonitoringInput,
) (*EnableEnhancedMonitoringOutput, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("EnableEnhancedMonitoring")
	defer b.mu.Unlock()

	stream, ok := b.streams.Get(streamKey(region, input.StreamName))
	if !ok {
		return nil, ErrStreamNotFound
	}
	stream.mu.Lock("EnableEnhancedMonitoring.stream")
	defer stream.mu.Unlock()

	current := make([]string, len(stream.EnhancedMonitoring))
	copy(current, stream.EnhancedMonitoring)

	combined := make([]string, 0, len(current)+len(input.ShardLevelMetrics))
	combined = append(combined, current...)
	combined = append(combined, input.ShardLevelMetrics...)
	desired := uniqueStrings(combined)
	stream.EnhancedMonitoring = desired

	return &EnableEnhancedMonitoringOutput{
		StreamName:               stream.Name,
		StreamARN:                stream.ARN,
		CurrentShardLevelMetrics: current,
		DesiredShardLevelMetrics: desired,
	}, nil
}

// DisableEnhancedMonitoring removes shard-level metrics from a stream.
func (b *InMemoryBackend) DisableEnhancedMonitoring(
	ctx context.Context,
	input *DisableEnhancedMonitoringInput,
) (*DisableEnhancedMonitoringOutput, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DisableEnhancedMonitoring")
	defer b.mu.Unlock()

	stream, ok := b.streams.Get(streamKey(region, input.StreamName))
	if !ok {
		return nil, ErrStreamNotFound
	}
	stream.mu.Lock("DisableEnhancedMonitoring.stream")
	defer stream.mu.Unlock()

	current := make([]string, len(stream.EnhancedMonitoring))
	copy(current, stream.EnhancedMonitoring)

	desired := removeStrings(current, input.ShardLevelMetrics)
	stream.EnhancedMonitoring = desired

	return &DisableEnhancedMonitoringOutput{
		StreamName:               stream.Name,
		StreamARN:                stream.ARN,
		CurrentShardLevelMetrics: current,
		DesiredShardLevelMetrics: desired,
	}, nil
}
