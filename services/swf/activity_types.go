package swf

import (
	"fmt"
	"time"
)

// RegisterActivityType registers a new activity type with optional default settings.
func (b *InMemoryBackend) RegisterActivityType(
	domain, name, version, description string,
	defaults ActivityTypeDefaults,
) error {
	if domain == "" {
		return fmt.Errorf("%w: domain is required", ErrValidation)
	}
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrValidation)
	}
	if version == "" {
		return fmt.Errorf("%w: version is required", ErrValidation)
	}
	if err := validateDuration(defaults.DefaultTaskHeartbeatTimeout); err != nil {
		return err
	}
	if err := validateDuration(defaults.DefaultTaskScheduleToCloseTimeout); err != nil {
		return err
	}
	if err := validateDuration(defaults.DefaultTaskScheduleToStartTimeout); err != nil {
		return err
	}
	if err := validateDuration(defaults.DefaultTaskStartToCloseTimeout); err != nil {
		return err
	}

	b.mu.Lock("RegisterActivityType")
	defer b.mu.Unlock()

	if err := b.requireActiveDomainLocked(domain); err != nil {
		return err
	}

	key := domain + ":" + name + ":" + version
	if b.activities.Has(key) {
		return fmt.Errorf("%w: activity type %s/%s", ErrTypeAlreadyExists, name, version)
	}

	b.activities.Put(&ActivityType{
		Domain:       domain,
		Name:         name,
		Version:      version,
		Status:       statusRegistered,
		Description:  description,
		CreationDate: float64(time.Now().UnixMilli()) / milliDivisor,
		Defaults:     defaults,
	})

	return nil
}

// ListActivityTypes returns activity types for a domain, optionally filtered by registrationStatus.
func (b *InMemoryBackend) ListActivityTypes(
	domain, registrationStatus string,
) ([]ActivityType, error) {
	if err := validateRegistrationStatus(registrationStatus); err != nil {
		return nil, err
	}

	b.mu.RLock("ListActivityTypes")
	defer b.mu.RUnlock()

	byDomain := b.activitiesByDomain.Get(domain)
	out := make([]ActivityType, 0, len(byDomain))

	for _, at := range byDomain {
		if registrationStatus != "" && at.Status != registrationStatus {
			continue
		}
		out = append(out, *at)
	}

	return out, nil
}

// DescribeActivityType returns the details of an activity type.
func (b *InMemoryBackend) DescribeActivityType(
	domain, name, version string,
) (*ActivityType, error) {
	b.mu.RLock("DescribeActivityType")
	defer b.mu.RUnlock()

	key := domain + ":" + name + ":" + version
	at, ok := b.activities.Get(key)
	if !ok {
		return nil, fmt.Errorf("%w: activity type %s/%s not found", ErrNotFound, name, version)
	}
	cp := *at

	return &cp, nil
}

// DeprecateActivityType marks an activity type as deprecated.
func (b *InMemoryBackend) DeprecateActivityType(domain, name, version string) error {
	b.mu.Lock("DeprecateActivityType")
	defer b.mu.Unlock()

	key := domain + ":" + name + ":" + version
	at, ok := b.activities.Get(key)
	if !ok {
		return fmt.Errorf("%w: activity type %s/%s not found", ErrNotFound, name, version)
	}
	if at.Status == statusDeprecated {
		return fmt.Errorf("%w: activity type %s/%s", ErrTypeDeprecated, name, version)
	}
	at.Status = statusDeprecated
	at.DeprecationDate = float64(time.Now().UnixMilli()) / milliDivisor

	return nil
}

// UndeprecateActivityType re-activates a deprecated activity type.
func (b *InMemoryBackend) UndeprecateActivityType(domain, name, version string) error {
	b.mu.Lock("UndeprecateActivityType")
	defer b.mu.Unlock()

	key := domain + ":" + name + ":" + version
	at, ok := b.activities.Get(key)
	if !ok {
		return fmt.Errorf("%w: activity type %s/%s not found", ErrNotFound, name, version)
	}
	if at.Status == statusRegistered {
		return fmt.Errorf("%w: activity type %s/%s", ErrTypeAlreadyExists, name, version)
	}
	at.Status = statusRegistered
	at.DeprecationDate = 0

	return nil
}

// DeleteActivityType permanently removes a deprecated activity type. Real AWS
// requires the type to be deprecated first (TypeNotDeprecatedFault otherwise);
// after deletion, new activities of that type can no longer be scheduled, but
// activities already started before the type was deleted continue to run.
func (b *InMemoryBackend) DeleteActivityType(domain, name, version string) error {
	b.mu.Lock("DeleteActivityType")
	defer b.mu.Unlock()

	key := domain + ":" + name + ":" + version
	at, ok := b.activities.Get(key)
	if !ok {
		return fmt.Errorf("%w: activity type %s/%s not found", ErrNotFound, name, version)
	}
	if at.Status != statusDeprecated {
		return fmt.Errorf(
			"%w: activity type %s/%s must be deprecated before deletion",
			ErrTypeNotDeprecated, name, version,
		)
	}
	b.activities.Delete(key)

	return nil
}
