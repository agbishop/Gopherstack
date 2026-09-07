package awsconfig

import "fmt"

// PutRetentionConfiguration creates or updates a retention configuration.
func (b *InMemoryBackend) PutRetentionConfiguration(name string, days int32) error {
	if name == "" {
		return fmt.Errorf("%w: RetentionConfiguration name is required", ErrInvalidParameterValue)
	}

	b.mu.Lock("PutRetentionConfiguration")
	defer b.mu.Unlock()

	b.retentionConfigs.Put(&RetentionConfiguration{
		Name:                  name,
		RetentionPeriodInDays: days,
	})

	return nil
}

// DescribeRetentionConfigurations returns all retention configurations.
func (b *InMemoryBackend) DescribeRetentionConfigurations() []RetentionConfiguration {
	b.mu.RLock("DescribeRetentionConfigurations")
	defer b.mu.RUnlock()

	all := b.retentionConfigs.All()
	out := make([]RetentionConfiguration, 0, len(all))

	for _, rc := range all {
		out = append(out, *rc)
	}

	return out
}

// DeleteRetentionConfiguration removes a retention configuration by name.
func (b *InMemoryBackend) DeleteRetentionConfiguration(name string) error {
	b.mu.Lock("DeleteRetentionConfiguration")
	defer b.mu.Unlock()

	b.retentionConfigs.Delete(name)

	return nil
}
