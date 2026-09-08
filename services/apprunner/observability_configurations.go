package apprunner

import (
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// CreateObservabilityConfiguration creates a new observability config revision.
func (b *InMemoryBackend) CreateObservabilityConfiguration(
	name, tracingVendor string,
	tags map[string]string,
) (*ObservabilityConfiguration, error) {
	b.mu.Lock("CreateObservabilityConfiguration")
	defer b.mu.Unlock()

	revisions := b.obsByName[name]
	revision := int32(len(revisions) + 1) //nolint:gosec // revision count is always small
	id := newID()
	obsArn := b.obsARN(name, revision, id)
	now := time.Now().UTC()

	if len(revisions) > 0 {
		revisions[len(revisions)-1].Latest = false
	}

	cfg := &storedObservabilityConfiguration{
		ObservabilityConfigurationArn:      obsArn,
		ObservabilityConfigurationName:     name,
		ObservabilityConfigurationRevision: revision,
		TracingVendor:                      tracingVendor,
		Status:                             obsStatusActive,
		Latest:                             true,
		CreatedAt:                          now,
	}

	b.observabilityConfigs.Put(cfg)
	b.obsByName[name] = append(revisions, cfg)

	if len(tags) > 0 {
		b.tags[obsArn] = make(map[string]string)
		maps.Copy(b.tags[obsArn], tags)
	}

	cp := cfg.toObs()

	return &cp, nil
}

// DescribeObservabilityConfiguration returns an observability config by ARN.
func (b *InMemoryBackend) DescribeObservabilityConfiguration(obsArn string) (*ObservabilityConfiguration, error) {
	b.mu.RLock("DescribeObservabilityConfiguration")
	defer b.mu.RUnlock()

	cfg, ok := b.observabilityConfigs.Get(obsArn)
	if !ok {
		return nil, fmt.Errorf("observability configuration %s not found: %w", obsArn, ErrNotFound)
	}

	cp := cfg.toObs()

	return &cp, nil
}

// DeleteObservabilityConfiguration deletes an observability config.
func (b *InMemoryBackend) DeleteObservabilityConfiguration(obsArn string) (*ObservabilityConfiguration, error) {
	b.mu.Lock("DeleteObservabilityConfiguration")
	defer b.mu.Unlock()

	cfg, ok := b.observabilityConfigs.Get(obsArn)
	if !ok {
		return nil, fmt.Errorf("observability configuration %s not found: %w", obsArn, ErrNotFound)
	}

	// DeleteObservabilityConfiguration doc (api_op_DeleteObservabilityConfiguration.go):
	// "You can't delete a configuration that's used by one or more App
	// Runner services."
	if b.serviceUsesObservabilityConfig(obsArn) {
		return nil, fmt.Errorf(
			"observability configuration %s is used by one or more services: %w", obsArn, ErrInvalidParameter,
		)
	}

	cfg.Status = obsStatusInactive
	cfg.DeletedAt = time.Now().UTC()
	cp := cfg.toObs()

	b.observabilityConfigs.Delete(obsArn)
	delete(b.tags, obsArn)

	revisions := b.obsByName[cfg.ObservabilityConfigurationName]
	remaining := revisions[:0]
	for _, r := range revisions {
		if r.ObservabilityConfigurationArn != obsArn {
			remaining = append(remaining, r)
		}
	}

	if len(remaining) == 0 {
		delete(b.obsByName, cfg.ObservabilityConfigurationName)
	} else {
		remaining[len(remaining)-1].Latest = true
		b.obsByName[cfg.ObservabilityConfigurationName] = remaining
	}

	return &cp, nil
}

// ListObservabilityConfigurations returns observability configs with optional name filter.
//
//nolint:dupl // mirrors ListAutoScalingConfigurations's latest-only filtering by design.
func (b *InMemoryBackend) ListObservabilityConfigurations(
	nameFilter string,
	latestOnly bool,
	maxResults int32,
	nextToken string,
) ([]*ObservabilityConfigurationSummary, string, error) {
	b.mu.RLock("ListObservabilityConfigurations")
	defer b.mu.RUnlock()

	items := b.observabilityConfigs.Snapshot()

	all := make([]*ObservabilityConfigurationSummary, 0, len(items))
	seen := make(map[string]struct{})

	for _, cfg := range items {
		if nameFilter != "" && cfg.ObservabilityConfigurationName != nameFilter {
			continue
		}

		if latestOnly {
			revs := b.obsByName[cfg.ObservabilityConfigurationName]
			if len(revs) == 0 {
				continue
			}
			latest := revs[len(revs)-1]
			if _, already := seen[cfg.ObservabilityConfigurationName]; already {
				continue
			}
			seen[cfg.ObservabilityConfigurationName] = struct{}{}
			s := latest.toSummary()
			all = append(all, &s)
		} else {
			s := cfg.toSummary()
			all = append(all, &s)
		}
	}

	limit := int(maxResults)
	pg := page.New(all, nextToken, limit, defaultMaxResults)

	return pg.Data, pg.Next, nil
}
