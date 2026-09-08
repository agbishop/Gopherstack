package networkmonitor

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// CreateMonitor creates a new monitor.
func (b *InMemoryBackend) CreateMonitor(
	ctx context.Context,
	name string,
	aggregationPeriod *int64,
	probeInputs []createMonitorProbeInput,
	tags map[string]string,
) (*Monitor, error) {
	if err := validateMonitorName(name); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.defaultRegion)
	period := defaultAggregationPeriod

	if aggregationPeriod != nil {
		if *aggregationPeriod != 30 && *aggregationPeriod != 60 {
			return nil, fmt.Errorf("%w: aggregationPeriod must be 30 or 60", ErrValidation)
		}

		period = *aggregationPeriod
	}

	b.mu.Lock("CreateMonitor")
	defer b.mu.Unlock()

	key := regionKey(region, name)

	if b.monitors.Has(key) {
		return nil, fmt.Errorf("%w: monitor %q already exists", ErrAlreadyExists, name)
	}

	if len(b.monitorsByRegion.Get(region)) >= maxMonitorsPerAccountRegion {
		return nil, fmt.Errorf(
			"%w: an account cannot have more than %d monitors per Region",
			ErrServiceQuotaExceeded,
			maxMonitorsPerAccountRegion,
		)
	}

	now := time.Now().UTC()
	monARN := b.buildMonitorARN(region, name)

	probes, err := b.buildNestedProbes(region, name, probeInputs, now)
	if err != nil {
		return nil, err
	}

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	m := &Monitor{
		MonitorArn:        monARN,
		MonitorName:       name,
		Region:            region,
		State:             monitorStateActive,
		AggregationPeriod: period,
		Probes:            probes,
		Tags:              tagsCopy,
		CreatedAt:         &now,
		ModifiedAt:        &now,
	}

	b.monitors.Put(m)

	return monitorCopy(m), nil
}

// buildNestedProbes field-validates and builds the []*Probe for
// CreateMonitor's nested probes list. Must be called with b.mu held (write
// lock) since it allocates probe IDs. Field validation (400
// ValidationException) runs for every input before the probe-count/
// per-subnet quota check (402 ServiceQuotaExceededException) so malformed
// input takes precedence over a resource-limit condition, matching how AWS
// APIs generally order member validation ahead of business-logic checks.
func (b *InMemoryBackend) buildNestedProbes(
	region, monitorName string,
	probeInputs []createMonitorProbeInput,
	now time.Time,
) ([]*Probe, error) {
	protocols := make([]string, len(probeInputs))
	sourceArns := make([]string, len(probeInputs))

	for i, pi := range probeInputs {
		proto, err := validateProbeInput(pi.Destination, pi.Protocol, pi.SourceArn, pi.DestinationPort, pi.PacketSize)
		if err != nil {
			return nil, err
		}

		protocols[i] = proto
		sourceArns[i] = pi.SourceArn
	}

	if err := validateProbeQuotas(nil, sourceArns); err != nil {
		return nil, err
	}

	var probes []*Probe

	for i, pi := range probeInputs {
		probeID := b.nextProbeID()
		probeARN := b.buildProbeARN(region, monitorName, probeID)
		af := detectAddressFamily(pi.Destination)

		probeTags := make(map[string]string, len(pi.Tags))
		maps.Copy(probeTags, pi.Tags)

		probes = append(probes, &Probe{
			ProbeID:         probeID,
			ProbeArn:        probeARN,
			SourceArn:       pi.SourceArn,
			Destination:     pi.Destination,
			Protocol:        protocols[i],
			DestinationPort: pi.DestinationPort,
			PacketSize:      pi.PacketSize,
			State:           probeStateActive,
			AddressFamily:   af,
			CreatedAt:       &now,
			ModifiedAt:      &now,
			Tags:            probeTags,
		})
	}

	return probes, nil
}

// DeleteMonitor deletes a monitor and its probes.
func (b *InMemoryBackend) DeleteMonitor(ctx context.Context, name string) error {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("DeleteMonitor")
	defer b.mu.Unlock()

	key := regionKey(region, name)

	if !b.monitors.Has(key) {
		return fmt.Errorf("%w: monitor %q not found", ErrNotFound, name)
	}

	b.monitors.Delete(key)

	return nil
}

// GetMonitor returns a monitor by name.
func (b *InMemoryBackend) GetMonitor(ctx context.Context, name string) (*Monitor, error) {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.RLock("GetMonitor")
	defer b.mu.RUnlock()

	m, exists := b.monitors.Get(regionKey(region, name))
	if !exists {
		return nil, fmt.Errorf("%w: monitor %q not found", ErrNotFound, name)
	}

	return monitorCopy(m), nil
}

// UpdateMonitor updates a monitor's aggregation period.
func (b *InMemoryBackend) UpdateMonitor(
	ctx context.Context,
	name string,
	aggregationPeriod int64,
) (*Monitor, error) {
	if aggregationPeriod != 30 && aggregationPeriod != 60 {
		return nil, fmt.Errorf("%w: aggregationPeriod must be 30 or 60", ErrValidation)
	}

	region := getRegion(ctx, b.defaultRegion)

	b.mu.Lock("UpdateMonitor")
	defer b.mu.Unlock()

	m, exists := b.monitors.Get(regionKey(region, name))
	if !exists {
		return nil, fmt.Errorf("%w: monitor %q not found", ErrNotFound, name)
	}

	now := time.Now().UTC()
	m.AggregationPeriod = aggregationPeriod
	m.ModifiedAt = &now

	return monitorCopy(m), nil
}

// ListMonitors returns a filtered, paginated list of monitor summaries.
func (b *InMemoryBackend) ListMonitors(
	ctx context.Context,
	state, nextToken string,
	maxResults int,
) ([]monitorSummary, string, error) {
	region := getRegion(ctx, b.defaultRegion)

	b.mu.RLock("ListMonitors")
	defer b.mu.RUnlock()

	sorted := slices.Clone(b.monitorsByRegion.Get(region))
	slices.SortFunc(sorted, func(a, mo *Monitor) int {
		return strings.Compare(a.MonitorName, mo.MonitorName)
	})

	// Default startIdx past-the-end so an unrecognised token returns nothing.
	startIdx := 0
	if nextToken != "" {
		startIdx = len(sorted)
		for i, m := range sorted {
			if m.MonitorName > nextToken {
				startIdx = i

				break
			}
		}
	}

	if maxResults <= 0 || maxResults > 100 {
		maxResults = 100
	}

	var summaries []monitorSummary
	var outToken string

	for i := startIdx; i < len(sorted); i++ {
		if len(summaries) == maxResults {
			outToken = summaries[len(summaries)-1].MonitorName

			break
		}

		m := sorted[i]
		if state != "" && !strings.EqualFold(m.State, state) {
			continue
		}

		period := m.AggregationPeriod
		summaries = append(summaries, monitorSummary{
			MonitorArn:        m.MonitorArn,
			MonitorName:       m.MonitorName,
			State:             m.State,
			AggregationPeriod: &period,
			Tags:              maps.Clone(m.Tags),
		})
	}

	if summaries == nil {
		summaries = []monitorSummary{}
	}

	return summaries, outToken, nil
}

func validateMonitorName(name string) error {
	if !monitorNameRE.MatchString(name) {
		return fmt.Errorf("%w: monitorName must match [a-zA-Z0-9_-]{1,200}", ErrValidation)
	}

	return nil
}

func monitorCopy(m *Monitor) *Monitor {
	if m == nil {
		return nil
	}

	cp := *m
	cp.Tags = maps.Clone(m.Tags)

	if m.Probes != nil {
		cp.Probes = make([]*Probe, len(m.Probes))
		for i, p := range m.Probes {
			cp.Probes[i] = probeCopy(p)
		}
	}

	if m.CreatedAt != nil {
		t := *m.CreatedAt
		cp.CreatedAt = &t
	}

	if m.ModifiedAt != nil {
		t := *m.ModifiedAt
		cp.ModifiedAt = &t
	}

	return &cp
}
