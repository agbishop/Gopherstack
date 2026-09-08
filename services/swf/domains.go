package swf

import "fmt"

// RegisterDomain registers a new SWF domain with the given retention period.
// retention must be "0"-"90" or "NONE" (empty defaults to "NONE").
func (b *InMemoryBackend) RegisterDomain(name, description, retention string) error {
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrValidation)
	}
	if retention == "" {
		retention = retentionNone
	}
	if err := validateRetention(retention); err != nil {
		return err
	}

	b.mu.Lock("RegisterDomain")
	defer b.mu.Unlock()

	// DomainAlreadyExistsFault's doc ("You may get this fault if you are
	// registering a domain that is either already registered or deprecated")
	// covers both statuses -- unlike RegisterActivityType/RegisterWorkflowType,
	// RegisterDomain does not model DomainDeprecatedFault at all.
	if _, ok := b.domains.Get(name); ok {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, name)
	}

	b.domains.Put(&Domain{
		Name:                                   name,
		Description:                            description,
		Status:                                 statusRegistered,
		Arn:                                    domainARN(defaultRegion, defaultAccountID, name),
		WorkflowExecutionRetentionPeriodInDays: retention,
	})

	return nil
}

// ListDomains returns all domains with the given registrationStatus.
// An empty status returns all domains.
func (b *InMemoryBackend) ListDomains(registrationStatus string) ([]Domain, error) {
	if err := validateRegistrationStatus(registrationStatus); err != nil {
		return nil, err
	}

	b.mu.RLock("ListDomains")
	defer b.mu.RUnlock()

	out := make([]Domain, 0, b.domains.Len())
	for _, d := range b.domains.All() {
		if registrationStatus == "" || d.Status == registrationStatus {
			out = append(out, *d)
		}
	}

	return out, nil
}

// DescribeDomain returns the details of a registered SWF domain.
func (b *InMemoryBackend) DescribeDomain(name string) (*Domain, error) {
	b.mu.RLock("DescribeDomain")
	defer b.mu.RUnlock()

	d, ok := b.domains.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	cp := *d

	return &cp, nil
}

// DeprecateDomain marks a domain as deprecated. Per the real AWS doc comment
// on DeprecateDomain ("Deprecating a domain also deprecates all activity and
// workflow types registered in the domain. Executions that were started
// before the domain was deprecated continue to run."), this cascades
// DEPRECATED onto every REGISTERED workflow/activity type in the domain --
// executions are deliberately left untouched (they keep running).
func (b *InMemoryBackend) DeprecateDomain(name string) error {
	b.mu.Lock("DeprecateDomain")
	defer b.mu.Unlock()

	d, ok := b.domains.Get(name)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if d.Status == statusDeprecated {
		return fmt.Errorf("%w: %s", ErrDeprecated, name)
	}
	d.Status = statusDeprecated

	for _, wt := range b.workflowsByDomain.Get(name) {
		if wt.Status == statusRegistered {
			wt.Status = statusDeprecated
		}
	}
	for _, at := range b.activitiesByDomain.Get(name) {
		if at.Status == statusRegistered {
			at.Status = statusDeprecated
		}
	}

	return nil
}

// requireActiveDomainLocked returns an error unless name is a REGISTERED domain.
// RegisterActivityType, RegisterWorkflowType and StartWorkflowExecution model
// UnknownResourceFault but not DomainDeprecatedFault, so a deprecated domain
// surfaces the same fault as a missing one here -- matching UnknownResourceFault's
// doc ("the named resource ... is no longer available for this operation") and
// DeprecateDomain's doc ("it cannot be used to create new workflow executions or
// register new types"). Caller must hold at least the read lock.
func (b *InMemoryBackend) requireActiveDomainLocked(name string) error {
	d, ok := b.domains.Get(name)
	if !ok || d.Status == statusDeprecated {
		return fmt.Errorf("%w: domain %s not found", ErrNotFound, name)
	}

	return nil
}

// UndeprecateDomain re-activates a deprecated domain.
func (b *InMemoryBackend) UndeprecateDomain(name string) error {
	b.mu.Lock("UndeprecateDomain")
	defer b.mu.Unlock()

	d, ok := b.domains.Get(name)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if d.Status == statusRegistered {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, name)
	}
	d.Status = statusRegistered

	return nil
}
