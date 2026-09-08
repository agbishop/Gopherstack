package emr

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// toStepHadoopJarStep converts a request-side StepHadoopJarStepInput (real
// types.HadoopJarStepConfig, Properties as a []KeyValue array) into the
// response-side StepHadoopJarStep (real types.HadoopStepConfig, Properties
// as a plain map) -- see StepHadoopJarStepInput's doc comment (models.go)
// for why the two shapes genuinely differ on the real wire.
func toStepHadoopJarStep(in StepHadoopJarStepInput) StepHadoopJarStep {
	var props map[string]string
	if len(in.Properties) > 0 {
		props = make(map[string]string, len(in.Properties))
		for _, kv := range in.Properties {
			props[kv.Key] = kv.Value
		}
	}

	return StepHadoopJarStep{
		Jar:        in.Jar,
		MainClass:  in.MainClass,
		Args:       in.Args,
		Properties: props,
	}
}

// buildInitialSteps converts input StepSpec records into Step records.
// executionRoleArn is RunJobFlowInput.StepExecutionRoleArn, a real,
// call-level (not per-step) field applied to every initial step
// (types.RunJobFlowInput.StepExecutionRoleArn, emr@v1.64.4
// api_op_RunJobFlow.go) -- echoed back as each Step.ExecutionRoleArn.
func (b *InMemoryBackend) buildInitialSteps(specs []StepSpec, executionRoleArn string) []Step {
	steps := make([]Step, 0, len(specs))
	now := awstime.Epoch(time.Now())

	for _, spec := range specs {
		actionOnFailure := spec.ActionOnFailure
		if actionOnFailure == "" {
			actionOnFailure = "TERMINATE_CLUSTER"
		}

		steps = append(steps, Step{
			ID:               b.nextStepID(),
			Name:             spec.Name,
			HadoopJarStep:    toStepHadoopJarStep(spec.HadoopJarStep),
			ActionOnFailure:  actionOnFailure,
			ExecutionRoleArn: executionRoleArn,
			Status: StepStatus{
				State:    StepStatePending,
				Timeline: StepTimeline{CreationDateTime: now},
			},
		})
	}

	return steps
}

// AddJobFlowSteps adds steps to a cluster and returns their IDs.
// executionRoleArn is AddJobFlowStepsInput.ExecutionRoleArn, a real,
// call-level (not per-step) field (emr@v1.64.4 api_op_AddJobFlowSteps.go)
// applied to every step added by this call -- echoed back as each
// Step.ExecutionRoleArn.
func (b *InMemoryBackend) AddJobFlowSteps(
	ctx context.Context, jobFlowID string, specs []StepSpec, executionRoleArn string,
) ([]string, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("AddJobFlowSteps")
	defer b.mu.Unlock()

	cluster, ok := b.clusterGet(region, jobFlowID)
	if !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, jobFlowID)
	}

	if !clusterAcceptsSteps(cluster.Status.State) {
		return nil, fmt.Errorf(
			"%w: cluster %s is in state %s", errClusterNotAcceptingSteps, jobFlowID, cluster.Status.State,
		)
	}

	now := awstime.Epoch(time.Now())
	ids := make([]string, 0, len(specs))

	for _, spec := range specs {
		actionOnFailure := spec.ActionOnFailure
		if actionOnFailure == "" {
			actionOnFailure = "TERMINATE_CLUSTER"
		}

		step := Step{
			ID:               b.nextStepID(),
			Name:             spec.Name,
			HadoopJarStep:    toStepHadoopJarStep(spec.HadoopJarStep),
			ActionOnFailure:  actionOnFailure,
			ExecutionRoleArn: executionRoleArn,
			Status: StepStatus{
				State:    StepStatePending,
				Timeline: StepTimeline{CreationDateTime: now},
			},
		}

		cluster.steps = append(cluster.steps, step)
		ids = append(ids, step.ID)
	}

	return ids, nil
}

// ListSteps returns steps for a cluster, optionally filtered by state and/or ID.
func (b *InMemoryBackend) ListSteps(
	ctx context.Context,
	clusterID string,
	stepStates []string,
	stepIDs []string,
	marker string,
) ([]Step, string) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListSteps")
	defer b.mu.RUnlock()

	cluster, ok := b.clusterGet(region, clusterID)
	if !ok {
		return []Step{}, ""
	}

	stateSet := buildStateSet(stepStates)
	idSet := buildStringSet(stepIDs)

	steps := make([]Step, len(cluster.steps))
	for i, s := range cluster.steps {
		steps[i] = s
		steps[i].Status = effectiveStepStatus(s.Status)
	}

	filtered := filterSteps(steps, stateSet, idSet)

	// AWS returns most recently added first.
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}

	p := page.New(filtered, marker, listStepsPageSize, listStepsPageSize)

	return p.Data, p.Next
}

// ListBootstrapActions returns the bootstrap actions for a cluster, paginated.
func (b *InMemoryBackend) ListBootstrapActions(
	ctx context.Context,
	clusterID, marker string,
) ([]Command, string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListBootstrapActions")
	defer b.mu.RUnlock()

	cluster, ok := b.clusterGet(region, clusterID)
	if !ok {
		return nil, "", fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterID)
	}

	commands := make([]Command, len(cluster.bootstrapActions))
	for i, ba := range cluster.bootstrapActions {
		commands[i] = Command{
			Name:       ba.Name,
			ScriptPath: ba.ScriptBootstrapAction.Path,
			Args:       slices.Clone(ba.ScriptBootstrapAction.Args),
		}
	}

	p := page.New(commands, marker, listBootstrapActionsPageSize, listBootstrapActionsPageSize)

	return p.Data, p.Next, nil
}

func filterSteps(steps []Step, stateSet, idSet map[string]bool) []Step {
	filtered := make([]Step, 0, len(steps))

	for _, s := range steps {
		if stateSet != nil && !stateSet[s.Status.State] {
			continue
		}

		if idSet != nil && !idSet[s.ID] {
			continue
		}

		filtered = append(filtered, s)
	}

	return filtered
}

func buildStringSet(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}

	set := make(map[string]bool, len(items))
	for _, s := range items {
		set[s] = true
	}

	return set
}

// DescribeStep returns a single step by cluster ID and step ID.
func (b *InMemoryBackend) DescribeStep(ctx context.Context, clusterID, stepID string) (*Step, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeStep")
	defer b.mu.RUnlock()

	cluster, ok := b.clusterGet(region, clusterID)
	if !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterID)
	}

	for _, s := range cluster.steps {
		if s.ID == stepID {
			cp := s
			cp.Status = effectiveStepStatus(s.Status)

			return &cp, nil
		}
	}

	return nil, fmt.Errorf("%w: step %s not found", ErrNotFound, stepID)
}

// CancelSteps cancels pending steps on a cluster.
func (b *InMemoryBackend) CancelSteps(
	ctx context.Context,
	clusterID string,
	stepIDs []string,
) ([]*CancelStepsInfo, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("CancelSteps")
	defer b.mu.Unlock()

	cluster, ok := b.clusterGet(region, clusterID)
	if !ok {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrNotFound, clusterID)
	}

	idSet := buildStringSet(stepIDs)
	results := make([]*CancelStepsInfo, 0, len(stepIDs))

	for i := range cluster.steps {
		s := &cluster.steps[i]
		if idSet == nil || idSet[s.ID] {
			results = append(results, cancelStep(s))
		}
	}

	return results, nil
}

// cancelStep cancels a single step in place if it is still (effectively)
// PENDING, and reports the outcome using the real CancelStepsRequestStatus
// enum -- SUBMITTED (the cancellation was accepted) or FAILED (the step is
// no longer cancellable, e.g. already COMPLETED or CANCELLED). AWS's actual
// enum has only these two values; this backend previously returned the
// non-existent "SUCCESS"/"QUEUED" strings.
func cancelStep(s *Step) *CancelStepsInfo {
	if effectiveStepStatus(s.Status).State != StepStatePending {
		return &CancelStepsInfo{
			StepID: s.ID,
			Status: cancelStepsStatusFailed,
			Reason: "Step is not in a state to be cancelled",
		}
	}

	s.Status.State = StepStateCancelled

	return &CancelStepsInfo{StepID: s.ID, Status: cancelStepsStatusSubmitted}
}
