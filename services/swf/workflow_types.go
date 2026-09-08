package swf

import (
	"fmt"
	"time"
)

// RegisterWorkflowType registers a new workflow type with optional default settings.
func (b *InMemoryBackend) RegisterWorkflowType(
	domain, name, version, description string,
	defaults WorkflowTypeDefaults,
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
	if err := validateChildPolicy(defaults.DefaultChildPolicy); err != nil {
		return err
	}
	if err := validateDuration(defaults.DefaultTaskStartToCloseTimeout); err != nil {
		return err
	}
	if err := validateDuration(defaults.DefaultExecutionStartToCloseTimeout); err != nil {
		return err
	}

	b.mu.Lock("RegisterWorkflowType")
	defer b.mu.Unlock()

	if err := b.requireActiveDomainLocked(domain); err != nil {
		return err
	}

	key := domain + ":" + name + ":" + version
	if b.workflows.Has(key) {
		return fmt.Errorf("%w: %s/%s", ErrTypeAlreadyExists, name, version)
	}

	b.workflows.Put(&WorkflowType{
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

// ListWorkflowTypes returns workflow types for a domain, optionally filtered by registrationStatus.
func (b *InMemoryBackend) ListWorkflowTypes(
	domain, registrationStatus string,
) ([]WorkflowType, error) {
	if err := validateRegistrationStatus(registrationStatus); err != nil {
		return nil, err
	}

	b.mu.RLock("ListWorkflowTypes")
	defer b.mu.RUnlock()

	byDomain := b.workflowsByDomain.Get(domain)
	out := make([]WorkflowType, 0, len(byDomain))

	for _, wt := range byDomain {
		if registrationStatus != "" && wt.Status != registrationStatus {
			continue
		}
		out = append(out, *wt)
	}

	return out, nil
}

// DescribeWorkflowType returns the details of a workflow type.
func (b *InMemoryBackend) DescribeWorkflowType(
	domain, name, version string,
) (*WorkflowType, error) {
	b.mu.RLock("DescribeWorkflowType")
	defer b.mu.RUnlock()

	key := domain + ":" + name + ":" + version
	wt, ok := b.workflows.Get(key)
	if !ok {
		return nil, fmt.Errorf("%w: workflow type %s/%s not found", ErrNotFound, name, version)
	}
	cp := *wt

	return &cp, nil
}

// DeprecateWorkflowType marks a workflow type as deprecated.
func (b *InMemoryBackend) DeprecateWorkflowType(domain, name, version string) error {
	b.mu.Lock("DeprecateWorkflowType")
	defer b.mu.Unlock()

	key := domain + ":" + name + ":" + version
	wt, ok := b.workflows.Get(key)
	if !ok {
		return fmt.Errorf("%w: workflow type %s/%s not found", ErrNotFound, name, version)
	}
	if wt.Status == statusDeprecated {
		return fmt.Errorf("%w: workflow type %s/%s", ErrTypeDeprecated, name, version)
	}
	wt.Status = statusDeprecated
	wt.DeprecationDate = float64(time.Now().UnixMilli()) / milliDivisor

	return nil
}

// UndeprecateWorkflowType re-activates a deprecated workflow type.
func (b *InMemoryBackend) UndeprecateWorkflowType(domain, name, version string) error {
	b.mu.Lock("UndeprecateWorkflowType")
	defer b.mu.Unlock()

	key := domain + ":" + name + ":" + version
	wt, ok := b.workflows.Get(key)
	if !ok {
		return fmt.Errorf("%w: workflow type %s/%s not found", ErrNotFound, name, version)
	}
	if wt.Status == statusRegistered {
		return fmt.Errorf("%w: workflow type %s/%s", ErrTypeAlreadyExists, name, version)
	}
	wt.Status = statusRegistered
	wt.DeprecationDate = 0

	return nil
}

// DeleteWorkflowType permanently removes a deprecated workflow type. Real AWS
// requires the type to be deprecated first (TypeNotDeprecatedFault otherwise);
// after deletion, StartWorkflowExecution can no longer reference it, but
// executions already running under it are unaffected.
func (b *InMemoryBackend) DeleteWorkflowType(domain, name, version string) error {
	b.mu.Lock("DeleteWorkflowType")
	defer b.mu.Unlock()

	key := domain + ":" + name + ":" + version
	wt, ok := b.workflows.Get(key)
	if !ok {
		return fmt.Errorf("%w: workflow type %s/%s not found", ErrNotFound, name, version)
	}
	if wt.Status != statusDeprecated {
		return fmt.Errorf(
			"%w: workflow type %s/%s must be deprecated before deletion",
			ErrTypeNotDeprecated, name, version,
		)
	}
	b.workflows.Delete(key)

	return nil
}
