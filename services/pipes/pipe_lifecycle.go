package pipes

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

var pipeNameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func validateCreatePipeInput(in CreatePipeInput) error {
	if err := validatePipeName(in.Name); err != nil {
		return err
	}
	if err := validateDesiredState(in.DesiredState); err != nil {
		return err
	}
	if in.Source == "" {
		return fmt.Errorf("%w: Source is required", ErrValidation)
	}
	if in.Target == "" {
		return fmt.Errorf("%w: Target is required", ErrValidation)
	}
	if in.RoleARN == "" {
		return fmt.Errorf("%w: RoleArn is required", ErrValidation)
	}
	if err := validateTags(in.Tags); err != nil {
		return err
	}
	if len(in.Tags) > maxTagsPerPipe {
		return fmt.Errorf("%w: pipe would exceed %d tags limit", ErrValidation, maxTagsPerPipe)
	}
	if err := validateSourceBatchSize(in.SourceParameters); err != nil {
		return err
	}
	if err := validateSourceStartingPosition(in.SourceParameters); err != nil {
		return err
	}
	if err := validateSourceRequiredFields(in.SourceParameters); err != nil {
		return err
	}
	if in.SourceParameters != nil {
		if err := validateFilterCriteria(in.SourceParameters.FilterCriteria); err != nil {
			return err
		}
	}
	if err := validateTargetRequiredFields(in.TargetParameters); err != nil {
		return err
	}

	return validateLogConfigurationRequiredFields(in.LogConfiguration)
}

func (b *InMemoryBackend) CreatePipe(ctx context.Context, in CreatePipeInput) (*Pipe, error) {
	if err := validateCreatePipeInput(in); err != nil {
		return nil, err
	}

	b.mu.Lock("CreatePipe")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	pipesTable := b.pipesTable(region)

	if pipesTable.Len() >= maxPipesPerAcct {
		return nil, fmt.Errorf(
			"%w: account has reached the maximum number of pipes (%d)",
			ErrQuota,
			maxPipesPerAcct,
		)
	}
	if pipesTable.Has(in.Name) {
		return nil, fmt.Errorf("%w: pipe %s already exists", ErrAlreadyExists, in.Name)
	}
	if in.DesiredState == "" {
		in.DesiredState = stateRunning
	}

	now := time.Now()
	pipeARN := arn.Build("pipes", region, b.accountID, "pipe/"+in.Name)
	p := &Pipe{
		Name: in.Name, ARN: pipeARN, RoleARN: in.RoleARN,
		Source: in.Source, Target: in.Target, Description: in.Description,
		Enrichment: in.Enrichment, KmsKeyIdentifier: in.KmsKeyIdentifier,
		DesiredState: in.DesiredState, CurrentState: stateCreating,
		AccountID: b.accountID, Region: region,
		CreationTime: now, LastModifiedTime: now,
		Tags:                 mergeTags(nil, in.Tags),
		SourceParameters:     in.SourceParameters,
		TargetParameters:     in.TargetParameters,
		LogConfiguration:     in.LogConfiguration,
		EnrichmentParameters: in.EnrichmentParameters,
	}
	pipesTable.Put(p)

	cp := clonePipe(p)
	b.runDelayed(func() {
		b.completeCreateTransition(region, in.Name, in.DesiredState)
	})

	return cp, nil
}

// completeCreateTransition moves a pipe from CREATING to its desired state.
func (b *InMemoryBackend) completeCreateTransition(region, name, desiredState string) {
	b.mu.Lock("completeCreateTransition")
	defer b.mu.Unlock()
	p, ok := b.pipesTable(region).Get(name)
	if !ok {
		return
	}
	if p.CurrentState == stateCreating {
		p.CurrentState = desiredState
		p.LastModifiedTime = time.Now()
	}
}

func (b *InMemoryBackend) GetPipe(ctx context.Context, name string) (*Pipe, error) {
	b.mu.RLock("GetPipe")
	defer b.mu.RUnlock()
	p, ok := b.pipesTable(getRegion(ctx, b.region)).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: pipe %s not found", ErrNotFound, name)
	}

	return clonePipe(p), nil
}

// applyUpdateFields patches the pipe with non-zero values from the update input.
func applyUpdateFields(p *Pipe, in UpdatePipeInput) {
	if in.RoleARN != "" {
		p.RoleARN = in.RoleARN
	}
	if in.Target != "" {
		p.Target = in.Target
	}
	if in.DesiredState != "" {
		p.DesiredState = in.DesiredState
	}
	if in.Enrichment != "" {
		p.Enrichment = in.Enrichment
	}
	if in.KmsKeyIdentifier != nil {
		p.KmsKeyIdentifier = *in.KmsKeyIdentifier
	}
	if in.Description != nil {
		p.Description = *in.Description
	}
	if in.SourceParameters != nil {
		p.SourceParameters = in.SourceParameters
	}
	if in.TargetParameters != nil {
		p.TargetParameters = in.TargetParameters
	}
	if in.LogConfiguration != nil {
		p.LogConfiguration = in.LogConfiguration
	}
	if in.EnrichmentParameters != nil {
		p.EnrichmentParameters = in.EnrichmentParameters
	}
}

func (b *InMemoryBackend) UpdatePipe(ctx context.Context, name string, in UpdatePipeInput) (*Pipe, error) {
	if err := validateDesiredState(in.DesiredState); err != nil {
		return nil, err
	}
	if err := validateSourceBatchSize(in.SourceParameters); err != nil {
		return nil, err
	}
	if err := validateUpdateSourceRequiredFields(in.SourceParameters); err != nil {
		return nil, err
	}
	if in.SourceParameters != nil {
		if err := validateFilterCriteria(in.SourceParameters.FilterCriteria); err != nil {
			return nil, err
		}
	}
	if err := validateTargetRequiredFields(in.TargetParameters); err != nil {
		return nil, err
	}
	if err := validateLogConfigurationRequiredFields(in.LogConfiguration); err != nil {
		return nil, err
	}
	if in.RoleARN == "" {
		return nil, fmt.Errorf("%w: RoleArn is required", ErrValidation)
	}

	b.mu.Lock("UpdatePipe")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	p, ok := b.pipesTable(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: pipe %s not found", ErrNotFound, name)
	}
	if p.CurrentState == stateDeleting {
		return nil, fmt.Errorf(
			"%w: pipe %s is being deleted",
			ErrConflict, name,
		)
	}
	applyUpdateFields(p, in)

	prevDesiredState := p.DesiredState
	if in.DesiredState != "" {
		prevDesiredState = in.DesiredState
	}
	p.CurrentState = stateUpdating
	p.LastModifiedTime = time.Now()
	cp := clonePipe(p)

	b.runDelayed(func() {
		b.completeUpdateTransition(region, name, prevDesiredState)
	})

	return cp, nil
}

// completeUpdateTransition moves a pipe from UPDATING to its desired state.
func (b *InMemoryBackend) completeUpdateTransition(region, name, desiredState string) {
	b.mu.Lock("completeUpdateTransition")
	defer b.mu.Unlock()
	p, ok := b.pipesTable(region).Get(name)
	if !ok {
		return
	}
	if p.CurrentState == stateUpdating {
		p.CurrentState = desiredState
		p.LastModifiedTime = time.Now()
	}
}

func (b *InMemoryBackend) DeletePipe(ctx context.Context, name string) (*Pipe, error) {
	b.mu.Lock("DeletePipe")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	p, ok := b.pipesTable(region).Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: pipe %s not found", ErrNotFound, name)
	}
	if p.CurrentState == stateDeleting {
		return nil, fmt.Errorf(
			"%w: pipe %s is already being deleted",
			ErrConflict, name,
		)
	}
	p.CurrentState = stateDeleting
	p.LastModifiedTime = time.Now()
	cp := clonePipe(p)

	b.runDelayed(func() {
		b.completeDeleteTransition(region, name)
	})

	return cp, nil
}

// completeDeleteTransition removes the pipe after it has been marked DELETING.
func (b *InMemoryBackend) completeDeleteTransition(region, name string) {
	b.mu.Lock("completeDeleteTransition")
	defer b.mu.Unlock()
	pipesTable := b.pipesTable(region)
	p, ok := pipesTable.Get(name)
	if !ok {
		return
	}
	if p.CurrentState == stateDeleting {
		// Delete also removes p from the ARN secondary index automatically.
		pipesTable.Delete(name)
		// Prune the per-pipe enrichment counter to prevent unbounded growth.
		if ct, exists := b.enrichmentCounts[region]; exists {
			ct.Delete(name)
		}
	}
}

// changePipeDesiredState is the shared body of StartPipe and StopPipe.
func (b *InMemoryBackend) changePipeDesiredState(
	ctx context.Context,
	name string,
	opts pipeDesiredStateOpts,
) (*Pipe, error) {
	b.mu.Lock(opts.lockName)
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	p, ok := b.pipesTable(region).Get(name)

	if !ok {
		return nil, fmt.Errorf("%w: pipe %s not found", ErrNotFound, name)
	}

	if p.CurrentState == opts.blockedState || p.CurrentState == stateDeleting {
		return nil, fmt.Errorf(
			"%w: pipe %s is in a transitional state %s",
			ErrConflict, name, p.CurrentState,
		)
	}

	if p.DesiredState == opts.desiredState {
		return nil, fmt.Errorf(
			"%w: pipe %s already has desired state %s",
			ErrConflict, name, opts.desiredState,
		)
	}

	p.DesiredState = opts.desiredState
	p.CurrentState = opts.transitState
	p.StateReason = ""
	p.LastModifiedTime = time.Now()
	cp := clonePipe(p)

	b.runDelayed(func() {
		opts.completeAfter(region, name)
	})

	return cp, nil
}

func (b *InMemoryBackend) StartPipe(ctx context.Context, name string) (*Pipe, error) {
	return b.changePipeDesiredState(ctx, name, pipeDesiredStateOpts{
		lockName:      "StartPipe",
		blockedState:  stateStarting,
		desiredState:  stateRunning,
		transitState:  stateStarting,
		completeAfter: b.completeStartTransition,
	})
}

// completeStartTransition moves a pipe from STARTING to RUNNING.
func (b *InMemoryBackend) completeStartTransition(region, name string) {
	b.mu.Lock("completeStartTransition")
	defer b.mu.Unlock()
	p, ok := b.pipesTable(region).Get(name)
	if !ok {
		return
	}
	if p.CurrentState == stateStarting {
		p.CurrentState = stateRunning
		p.LastModifiedTime = time.Now()
	}
}

func (b *InMemoryBackend) StopPipe(ctx context.Context, name string) (*Pipe, error) {
	return b.changePipeDesiredState(ctx, name, pipeDesiredStateOpts{
		lockName:      "StopPipe",
		blockedState:  stateStopping,
		desiredState:  stateStopped,
		transitState:  stateStopping,
		completeAfter: b.completeStopTransition,
	})
}

// completeStopTransition moves a pipe from STOPPING to STOPPED.
func (b *InMemoryBackend) completeStopTransition(region, name string) {
	b.mu.Lock("completeStopTransition")
	defer b.mu.Unlock()
	p, ok := b.pipesTable(region).Get(name)
	if !ok {
		return
	}
	if p.CurrentState == stateStopping {
		p.CurrentState = stateStopped
		p.LastModifiedTime = time.Now()
	}
}

// MarkPipeFailed updates a pipe to a failed state with a reason message.
// It searches all regions for the named pipe.
func (b *InMemoryBackend) MarkPipeFailed(name, state, reason string) {
	b.mu.Lock("MarkPipeFailed")
	defer b.mu.Unlock()

	for _, regionTable := range b.pipes {
		if p, ok := regionTable.Get(name); ok {
			p.CurrentState = state
			p.StateReason = reason
			p.LastModifiedTime = time.Now()

			return
		}
	}
}

func validatePipeName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: pipe name must not be empty", ErrValidation)
	}
	if len(name) > maxPipeNameLen {
		return fmt.Errorf(
			"%w: pipe name exceeds maximum length of %d characters",
			ErrValidation,
			maxPipeNameLen,
		)
	}
	if !pipeNameRE.MatchString(name) {
		return fmt.Errorf(
			"%w: pipe name %q contains invalid characters (allowed: a-z, A-Z, 0-9, -, _)",
			ErrValidation,
			name,
		)
	}

	return nil
}

func validateDesiredState(state string) error {
	if state == "" || state == stateRunning || state == stateStopped {
		return nil
	}

	return fmt.Errorf("%w: DesiredState must be RUNNING or STOPPED, got %q", ErrValidation, state)
}

// validateLogConfigurationRequiredFields enforces LogConfiguration.Level as
// required when LogConfiguration is set, matching aws-sdk-go-v2 pipes
// validators.go's validatePipeLogConfigurationParameters. Reached identically
// from CreatePipe and UpdatePipe: both validateOpCreatePipeInput and
// validateOpUpdatePipeInput nest into it only when LogConfiguration != nil.
func validateLogConfigurationRequiredFields(lc *LogConfiguration) error {
	if lc == nil {
		return nil
	}
	if lc.Level == "" {
		return fmt.Errorf("%w: LogConfiguration.Level is required", ErrValidation)
	}

	return nil
}
