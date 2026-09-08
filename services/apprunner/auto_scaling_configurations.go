package apprunner

import (
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// CreateAutoScalingConfiguration creates a new ASG config revision.
func (b *InMemoryBackend) CreateAutoScalingConfiguration(
	name string,
	maxConcurrency, maxSize, minSize int32,
	tags map[string]string,
) (*AutoScalingConfiguration, error) {
	b.mu.Lock("CreateAutoScalingConfiguration")
	defer b.mu.Unlock()

	revisions := b.asgByName[name]
	revision := int32(len(revisions) + 1) //nolint:gosec // revision count is always small
	id := newID()
	asgArn := b.asgARN(name, revision, id)
	now := time.Now().UTC()

	if len(revisions) > 0 {
		revisions[len(revisions)-1].Latest = false
	}

	if maxConcurrency == 0 {
		maxConcurrency = defaultMaxConcurrency
	}

	if maxSize == 0 {
		maxSize = defaultMaxSize
	}

	if minSize == 0 {
		minSize = defaultMinSize
	}

	cfg := &storedAutoScalingConfiguration{
		AutoScalingConfigurationArn:      asgArn,
		AutoScalingConfigurationName:     name,
		AutoScalingConfigurationRevision: revision,
		Status:                           asgStatusActive,
		MaxConcurrency:                   maxConcurrency,
		MaxSize:                          maxSize,
		MinSize:                          minSize,
		IsDefault:                        false,
		HasAssociatedService:             false,
		Latest:                           true,
		CreatedAt:                        now,
	}

	b.autoScalingConfigs.Put(cfg)
	b.asgByName[name] = append(revisions, cfg)

	if len(tags) > 0 {
		b.tags[asgArn] = make(map[string]string)
		maps.Copy(b.tags[asgArn], tags)
	}

	cp := cfg.toASG()

	return &cp, nil
}

// DescribeAutoScalingConfiguration returns an ASG config by ARN.
func (b *InMemoryBackend) DescribeAutoScalingConfiguration(asgArn string) (*AutoScalingConfiguration, error) {
	b.mu.RLock("DescribeAutoScalingConfiguration")
	defer b.mu.RUnlock()

	cfg, ok := b.autoScalingConfigs.Get(asgArn)
	if !ok {
		return nil, fmt.Errorf("auto scaling configuration %s not found: %w", asgArn, ErrNotFound)
	}

	cp := cfg.toASG()

	return &cp, nil
}

// DeleteAutoScalingConfiguration deletes an ASG config.
func (b *InMemoryBackend) DeleteAutoScalingConfiguration(asgArn string) (*AutoScalingConfiguration, error) {
	b.mu.Lock("DeleteAutoScalingConfiguration")
	defer b.mu.Unlock()

	cfg, ok := b.autoScalingConfigs.Get(asgArn)
	if !ok {
		return nil, fmt.Errorf("auto scaling configuration %s not found: %w", asgArn, ErrNotFound)
	}

	// DeleteAutoScalingConfiguration doc (api_op_DeleteAutoScalingConfiguration.go):
	// "You can't delete the default auto scaling configuration or a
	// configuration that's used by one or more App Runner services."
	if cfg.IsDefault {
		return nil, fmt.Errorf(
			"auto scaling configuration %s is the default and cannot be deleted: %w", asgArn, ErrInvalidParameter,
		)
	}

	if cfg.HasAssociatedService {
		return nil, fmt.Errorf(
			"auto scaling configuration %s is used by one or more services: %w", asgArn, ErrInvalidParameter,
		)
	}

	cfg.Status = asgStatusInactive
	cfg.DeletedAt = time.Now().UTC()
	cp := cfg.toASG()

	b.autoScalingConfigs.Delete(asgArn)
	delete(b.tags, asgArn)

	revisions := b.asgByName[cfg.AutoScalingConfigurationName]
	remaining := revisions[:0]
	for _, r := range revisions {
		if r.AutoScalingConfigurationArn != asgArn {
			remaining = append(remaining, r)
		}
	}

	if len(remaining) == 0 {
		delete(b.asgByName, cfg.AutoScalingConfigurationName)
	} else {
		remaining[len(remaining)-1].Latest = true
		b.asgByName[cfg.AutoScalingConfigurationName] = remaining
	}

	return &cp, nil
}

// ListAutoScalingConfigurations returns ASG configs with optional name filter.
//
//nolint:dupl // mirrors ListObservabilityConfigurations's latest-only filtering by design.
func (b *InMemoryBackend) ListAutoScalingConfigurations(
	nameFilter string,
	latestOnly bool,
	maxResults int32,
	nextToken string,
) ([]*AutoScalingConfigurationSummary, string, error) {
	b.mu.RLock("ListAutoScalingConfigurations")
	defer b.mu.RUnlock()

	items := b.autoScalingConfigs.Snapshot()

	all := make([]*AutoScalingConfigurationSummary, 0, len(items))
	seen := make(map[string]struct{})

	for _, cfg := range items {
		if nameFilter != "" && cfg.AutoScalingConfigurationName != nameFilter {
			continue
		}

		if latestOnly {
			revs := b.asgByName[cfg.AutoScalingConfigurationName]
			if len(revs) == 0 {
				continue
			}
			latest := revs[len(revs)-1]
			if _, already := seen[cfg.AutoScalingConfigurationName]; already {
				continue
			}
			seen[cfg.AutoScalingConfigurationName] = struct{}{}
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

// UpdateDefaultAutoScalingConfiguration sets the default ASG config.
func (b *InMemoryBackend) UpdateDefaultAutoScalingConfiguration(asgArn string) (*AutoScalingConfiguration, error) {
	b.mu.Lock("UpdateDefaultAutoScalingConfiguration")
	defer b.mu.Unlock()

	cfg, ok := b.autoScalingConfigs.Get(asgArn)
	if !ok {
		return nil, fmt.Errorf("auto scaling configuration %s not found: %w", asgArn, ErrNotFound)
	}

	b.autoScalingConfigs.Range(func(c *storedAutoScalingConfiguration) bool {
		c.IsDefault = false

		return true
	})

	cfg.IsDefault = true
	cp := cfg.toASG()

	return &cp, nil
}

// ListServicesForAutoScalingConfiguration returns the ARNs of every live
// service currently associated with asgArn (which may be a full ARN,
// name-only ARN, or bare name -- see resolveASG).
func (b *InMemoryBackend) ListServicesForAutoScalingConfiguration(
	asgArn string,
	maxResults int32,
	nextToken string,
) ([]string, string, error) {
	b.mu.RLock("ListServicesForAutoScalingConfiguration")
	defer b.mu.RUnlock()

	cfg, ok := b.resolveASG(asgArn)
	if !ok {
		return nil, "", fmt.Errorf("auto scaling configuration %s not found: %w", asgArn, ErrNotFound)
	}

	canonical := cfg.AutoScalingConfigurationArn

	arns := make([]string, 0)
	for _, svc := range b.services.Snapshot() {
		if svc.AutoScalingConfigurationArn == canonical {
			arns = append(arns, svc.ServiceArn)
		}
	}

	limit := int(maxResults)
	pg := page.New(arns, nextToken, limit, defaultMaxResults)

	return pg.Data, pg.Next, nil
}
