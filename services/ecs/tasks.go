package ecs

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	platformVersionLatest = "LATEST"
	platformVersion140    = "1.4.0"
	platformVersion130    = "1.3.0"
	platformVersion120    = "1.2.0"
	platformVersion110    = "1.1.0"
	platformVersion100    = "1.0.0"
)

// isValidFargatePlatformVersion reports whether pv is a known Fargate platform version.
func isValidFargatePlatformVersion(pv string) bool {
	switch pv {
	case platformVersionLatest,
		platformVersion140,
		platformVersion130,
		platformVersion120,
		platformVersion110,
		platformVersion100:
		return true
	}

	return false
}

// validatePlatformVersion returns an error if the platform version is non-empty
// and not recognized.
func validatePlatformVersion(pv string) error {
	if pv == "" {
		return nil
	}

	upper := strings.ToUpper(pv)
	if isValidFargatePlatformVersion(upper) || isValidFargatePlatformVersion(pv) {
		return nil
	}

	return fmt.Errorf(
		"%w: unknown Fargate platform version %q; valid values: LATEST, 1.4.0, 1.3.0, 1.2.0, 1.1.0, 1.0.0",
		ErrInvalidParameter,
		pv,
	)
}

const connectivityConnected = "CONNECTED"

// taskWork pairs a task with its definition for lock-free Docker API calls and
// for collecting the final task state when building the API response.
type taskWork struct {
	task *Task
	td   *TaskDefinition
}

// RunTask starts one or more tasks on the given cluster.
func (b *InMemoryBackend) RunTask(input RunTaskInput) ([]Task, error) {
	if input.TaskDefinition == "" {
		return nil, fmt.Errorf("%w: taskDefinition is required", ErrInvalidParameter)
	}

	if err := validatePlatformVersion(input.PlatformVersion); err != nil {
		return nil, err
	}

	count := input.Count
	if count <= 0 {
		count = 1
	}

	clusterName := clusterKey(b.resolveCluster(input.Cluster))

	var (
		ferr error
		work []taskWork
	)

	func() {
		b.mu.Lock("RunTask")
		defer b.mu.Unlock()

		b.ensureClusterLocked(clusterName)

		if err := b.validateCapacityProviderStrategyLocked(input.CapacityProviderStrategy); err != nil {
			ferr = err

			return
		}

		td, err := b.findTaskDefinitionLocked(input.TaskDefinition)
		if err != nil {
			ferr = err

			return
		}

		clusterArn := arn.Build("ecs", b.region, b.accountID, fmt.Sprintf("cluster/%s", clusterName))

		launchType := input.LaunchType
		if launchType == "" {
			launchType = launchTypeFargate
		}

		// Resolve task tags respecting propagateTags + ECS-managed-tags semantics.
		// We read tdTags while holding the write lock — safe, no deadlock risk.
		tdTags := b.resourceTags[resourceTagKey(td.TaskDefinitionArn)]
		resolvedTags := resolveTaskTags(
			input.Tags,
			input.PropagateTags,
			input.EnableECSManagedTags,
			clusterName,
			input.serviceNameForTags,
			td,
			tdTags,
			input.serviceTagsForPropagate,
		)

		// Create all task entries in PROVISIONING state under the lock so they are
		// immediately visible, then release the lock before issuing Docker API calls.
		work = b.createTaskEntriesLocked(
			clusterName,
			clusterArn,
			launchType,
			resolvedTags,
			count,
			td,
			input,
		)
	}()

	if ferr != nil {
		return nil, ferr
	}

	b.startTasksOutsideLock(work)

	// Snapshot final task states to build the API response.
	b.mu.RLock("RunTask-response")
	defer b.mu.RUnlock()

	tasks := make([]Task, 0, len(work))
	for _, w := range work {
		cp := *w.task
		tasks = append(tasks, cp)
	}

	return tasks, nil
}

// startTasksOutsideLock starts containers outside the lock to avoid serializing other
// operations behind potentially slow Docker API calls (image pull, network setup, etc.).
func (b *InMemoryBackend) startTasksOutsideLock(work []taskWork) {
	for _, w := range work {
		clusterName := clusterKey(clusterFromTaskARN(w.task.TaskArn))

		b.ensureAwslogsStreams(w.task, w.td)

		if b.runner == nil {
			if b.maybeRegisterStartLifecycle(w.task, clusterName) {
				continue
			}

			b.applyNoRunnerTransition(w.task, clusterName)

			continue
		}

		// Transition PROVISIONING → PENDING before the potentially-slow container
		// runtime call, then PENDING → RUNNING/STOPPED based on the result.
		b.applyPendingTransition(w.task)
		runErr := b.runner.RunTask(w.task, w.td)
		b.applyRunnerTransition(w.task, clusterName, runErr)
	}
}

// applyPendingTransition moves a PROVISIONING task to PENDING under the lock.
// Called for tasks with a real container runner, before the runner is invoked.
func (b *InMemoryBackend) applyPendingTransition(task *Task) {
	b.mu.Lock("RunTask-setPending")
	defer b.mu.Unlock()

	if task.LastStatus == statusProvisioning {
		task.LastStatus = statusPending
	}
}

// maybeRegisterStartLifecycle enrolls a no-runner task in the observable start
// pipeline when a start delay is configured, returning true if it did. When it
// returns false the caller applies the immediate transition instead. Must be
// called without any lock held.
func (b *InMemoryBackend) maybeRegisterStartLifecycle(task *Task, clusterName string) bool {
	b.mu.Lock("RunTask-registerStartLifecycle")
	defer b.mu.Unlock()

	if b.startDelay <= 0 || task.LastStatus != statusProvisioning {
		return false
	}

	b.registerStartLifecycleLocked(task, clusterName)

	return true
}

// applyNoRunnerTransition transitions a PROVISIONING task through PENDING to RUNNING
// when no container runtime is configured. Must be called without any lock held.
func (b *InMemoryBackend) applyNoRunnerTransition(task *Task, clusterName string) {
	b.mu.Lock("RunTask-setRunning")
	defer b.mu.Unlock()

	if task.LastStatus != statusProvisioning {
		return
	}

	// Pass through PENDING (resource provisioning complete, container starting).
	task.LastStatus = statusPending
	// Immediately advance to RUNNING since there is no real container to wait for.
	task.LastStatus = statusRunning
	syncContainerStatuses(task, nil)

	if c, _ := b.clusters.Get(clusterName); c != nil {
		c.PendingTasksCount--
		c.RunningTasksCount++
	}

	b.registerTaskWithELBv2Locked(task, clusterName)
}

// applyRunnerTransition transitions a PENDING task to RUNNING or STOPPED
// based on the container runtime result. Must be called without any lock held.
func (b *InMemoryBackend) applyRunnerTransition(task *Task, clusterName string, runErr error) {
	b.mu.Lock("RunTask-setRunning")
	defer b.mu.Unlock()

	// Only update status if no concurrent operation (e.g. StopTask) has
	// already changed the task away from PENDING. A task enters PENDING just
	// before the runner call via applyPendingTransition.
	if task.LastStatus != statusPending {
		return
	}

	if runErr == nil {
		task.LastStatus = statusRunning
		syncContainerStatuses(task, nil)

		if c, _ := b.clusters.Get(clusterName); c != nil {
			c.PendingTasksCount--
			c.RunningTasksCount++
		}

		b.registerTaskWithELBv2Locked(task, clusterName)

		return
	}

	// Container start failed — mark STOPPED so the task does not
	// remain in PROVISIONING permanently (resource leak + wrong semantics).
	now := time.Now()
	task.LastStatus = statusStopped
	task.DesiredStatus = statusStopped
	task.StoppedAt = &now
	task.StoppedReason = fmt.Sprintf("container start failed: %v", runErr)
	exitCode := 1
	syncContainerStatuses(task, &exitCode)

	if c, _ := b.clusters.Get(clusterName); c != nil {
		c.PendingTasksCount--
	}

	// A failed launch for a service task counts against the deployment circuit
	// breaker, which may trip the deployment to FAILED and (if enabled) roll the
	// service back to its last stable task definition.
	b.recordServiceTaskFailureLocked(clusterName, task)
}

// createTaskEntriesLocked creates task entries in PROVISIONING state.
// Must be called with write lock held; the lock is NOT released here.
func (b *InMemoryBackend) createTaskEntriesLocked(
	clusterName, clusterArn, launchType string,
	resolvedTags []Tag,
	count int,
	td *TaskDefinition,
	input RunTaskInput,
) []taskWork {
	work := make([]taskWork, 0, count)

	for range count {
		taskArn := fmt.Sprintf(
			"arn:aws:ecs:%s:%s:task/%s/%s",
			b.region, b.accountID, clusterName, uuid.NewString(),
		)

		now := time.Now()

		// Resolve the effective task IAM role: per-run override takes precedence.
		taskRoleArn := td.TaskRoleArn
		if input.Overrides != nil && input.Overrides.TaskRoleArn != "" {
			taskRoleArn = input.Overrides.TaskRoleArn
		}

		// CapacityProviderName reflects the capacity provider actually selected for
		// this task. AWS distributes tasks across the strategy's providers by
		// weight/base; this backend does not model that distribution and always
		// selects the first entry (documented simplification -- see the
		// CapacityProviderName doc comment on the Task struct in models.go).
		var capacityProviderName string
		if len(input.CapacityProviderStrategy) > 0 {
			capacityProviderName = input.CapacityProviderStrategy[0].CapacityProvider
		}

		task := &Task{
			TaskArn:              taskArn,
			ClusterArn:           clusterArn,
			TaskDefinitionArn:    td.TaskDefinitionArn,
			LastStatus:           statusProvisioning,
			DesiredStatus:        statusRunning,
			Group:                input.Group,
			LaunchType:           launchType,
			StartedBy:            input.StartedBy,
			PlatformVersion:      input.PlatformVersion,
			PropagateTags:        input.PropagateTags,
			Tags:                 resolvedTags,
			StartedAt:            &now,
			Connectivity:         connectivityConnected,
			ConnectivityAt:       &now,
			Overrides:            input.Overrides,
			NetworkConfiguration: input.NetworkConfiguration,
			EnableExecuteCommand: input.EnableExecuteCommand,
			TaskRoleArn:          taskRoleArn,
			CapacityProviderName: capacityProviderName,
		}

		// Mirror tags into the resourceTags side map so TagResource/UntagResource/
		// ListTagsForResource (and DescribeTasks, which reads tags live from
		// resourceTags -- see DescribeTasks below) see the tags applied at
		// creation, matching the fix already applied to ExpressGatewayService and
		// CapacityProvider. Without this, task.Tags and resourceTags are two
		// independent stores and a TagResource call after RunTask is invisible to
		// DescribeTasks.
		if len(resolvedTags) > 0 {
			b.setResourceTagsLocked(taskArn, resolvedTags)
		}

		if launchType == launchTypeFargate {
			task.Attachments = []TaskAttachment{newFargateTaskAttachment(taskArn)}
		} else {
			// EC2 launch type: select a container instance respecting placement
			// constraints and strategies, then record it in the reverse index.
			// Merge task-definition constraints with any run-time override constraints.
			constraints := mergeConstraints(td.PlacementConstraints, input.PlacementConstraints)
			if instanceArn := selectContainerInstance(
				b.containerInstancesByCluster.Get(clusterName),
				b.tasksByCluster.Get(clusterName),
				constraints,
				input.PlacementStrategy,
				input.serviceNameForTags,
			); instanceArn != "" {
				task.ContainerInstanceArn = instanceArn
				b.indexTaskOnInstance(clusterName, instanceArn, taskArn)
			}
		}

		task.Containers = buildContainersForTask(task, td)

		b.tasks.Put(task)
		work = append(work, taskWork{task: task, td: td})

		// Increment the cached pending counter on the cluster.
		if c, _ := b.clusters.Get(clusterName); c != nil {
			c.PendingTasksCount++
		}
	}

	return work
}

// DescribeTasks returns tasks on a given cluster, optionally filtered by ARN.
// Unknown task ARNs are returned as failures, not errors, matching AWS behaviour.
func (b *InMemoryBackend) DescribeTasks(
	cluster string,
	taskArns []string,
) ([]Task, []Failure, error) {
	clusterName := clusterKey(b.resolveCluster(cluster))

	b.mu.RLock("DescribeTasks")
	defer b.mu.RUnlock()

	if !b.clusters.Has(clusterName) {
		return nil, nil, fmt.Errorf("%w: %s", ErrClusterNotFound, cluster)
	}

	if len(taskArns) == 0 {
		clusterTasks := b.tasksByCluster.Get(clusterName)
		out := make([]Task, 0, len(clusterTasks))
		for _, t := range clusterTasks {
			out = append(out, b.taskWithLiveTagsLocked(t))
		}

		return out, nil, nil
	}

	out := make([]Task, 0, len(taskArns))
	failures := make([]Failure, 0, len(taskArns))

	for _, arn := range taskArns {
		t, found := b.tasks.Get(arn)
		if !found || clusterKey(t.ClusterArn) != clusterName {
			failures = append(failures, Failure{
				Arn:    arn,
				Reason: statusMissing,
				Detail: fmt.Sprintf("task %s not found", arn),
			})

			continue
		}

		out = append(out, b.taskWithLiveTagsLocked(t))
	}

	return out, failures, nil
}

// taskWithLiveTagsLocked returns a copy of t with Tags sourced from the
// resourceTags side map instead of t's own creation-time snapshot, so tags
// applied via TagResource/UntagResource after the task was started are
// reflected. Must be called with at least a read lock held.
func (b *InMemoryBackend) taskWithLiveTagsLocked(t *Task) Task {
	cp := *t
	cp.Tags = copyTags(b.resourceTags[resourceTagKey(t.TaskArn)])

	return cp
}

// StopTask stops a running task.
func (b *InMemoryBackend) StopTask(cluster, taskArn, reason string) (*Task, error) {
	clusterName := clusterKey(b.resolveCluster(cluster))

	var (
		ferr        error
		delayedCp   *Task
		fastCp      Task
		fastTask    *Task
		instanceArn string
	)

	func() {
		b.mu.Lock("StopTask")
		defer b.mu.Unlock()

		if !b.clusters.Has(clusterName) {
			ferr = fmt.Errorf("%w: %s", ErrClusterNotFound, cluster)

			return
		}

		task, ok := b.tasks.Get(taskArn)
		if !ok || clusterKey(task.ClusterArn) != clusterName {
			ferr = fmt.Errorf("%w: task %s not found", ErrInvalidParameter, taskArn)

			return
		}

		now := time.Now()
		prevStatus := task.LastStatus
		task.DesiredStatus = statusStopped
		task.StoppedReason = reason

		// Decrement the cached cluster counters once, as the task leaves its active
		// state. This is done up front for both the fast and delayed paths so the
		// counters stay correct regardless of when the task finally reaches STOPPED.
		if c, _ := b.clusters.Get(clusterName); c != nil {
			switch prevStatus {
			case statusRunning:
				c.RunningTasksCount--
			case statusProvisioning, statusPending:
				c.PendingTasksCount--
			}
		}

		// Delayed path: leave the task in DEACTIVATING and let the lifecycle stepper
		// advance it through STOPPING/DEPROVISIONING/STOPPED so SDK waiters observe
		// the intermediate states. Only used when a stop delay is configured and the
		// task is actually in an active state.
		if b.stopDelay > 0 && isStoppableStatus(prevStatus) {
			task.LastStatus = statusDeactivating
			b.lifecycle[taskArn] = &taskLifecycle{
				clusterName: clusterName,
				kind:        lifecycleKindStop,
				phase:       statusDeactivating,
				nextAt:      now.Add(b.stopDelay),
				reason:      reason,
			}

			cp := b.taskWithLiveTagsLocked(task)
			delayedCp = &cp

			return
		}

		// Fast path: transition straight to STOPPED. Clear any lifecycle entry
		// left over from a start (or a prior, still in-flight stop) pipeline --
		// otherwise the lifecycle stepper later advances the stale entry and
		// resurrects this task past STOPPED (e.g. back to PENDING/RUNNING).
		task.LastStatus = statusStopped
		task.StoppedAt = &now
		syncContainerStatuses(task, nil)
		b.deregisterTaskFromELBv2Locked(task, clusterName)
		delete(b.lifecycle, taskArn)

		instanceArn = task.ContainerInstanceArn
		fastCp = b.taskWithLiveTagsLocked(task)
		fastTask = task
	}()

	if ferr != nil {
		return nil, ferr
	}

	if delayedCp != nil {
		return delayedCp, nil
	}

	// Docker API calls happen outside the lock so other backend operations are
	// not serialized behind potentially slow container stops.
	if b.runner != nil {
		_ = b.runner.StopTask(fastTask)
	}

	// Clean up task protection entry and reverse index to avoid stale entries.
	func() {
		b.mu.Lock("StopTask-cleanup")
		defer b.mu.Unlock()

		b.taskProtections.Delete(taskArn)
		b.unindexTaskFromInstance(clusterName, instanceArn, taskArn)
	}()

	return &fastCp, nil
}

// isStoppableStatus reports whether a task in the given state has an active
// lifecycle that can be observably wound down (as opposed to one that is already
// stopped or mid-transition).
func isStoppableStatus(status string) bool {
	switch status {
	case statusRunning, statusPending, statusProvisioning:
		return true
	default:
		return false
	}
}

// markTaskStoppedByContainerExit finalizes taskArn to STOPPED when a
// container the Docker runner started exits on its own, without an explicit
// StopTask call -- wired as realDockerRunner's completion handler (see
// SetTaskCompletionHandler in docker_runner.go and its wiring in
// provider.go). "Essential container in task exited" is real ECS's own stop
// reason for this case. A concurrent StopTask always wins the race to
// finalize first: this is a no-op once the task has left an active state.
func (b *InMemoryBackend) markTaskStoppedByContainerExit(taskArn, containerName string, exitCode int) {
	var (
		clusterName string
		instanceArn string
		stopped     bool
	)

	func() {
		b.mu.Lock("markTaskStoppedByContainerExit")
		defer b.mu.Unlock()

		task, ok := b.tasks.Get(taskArn)
		if !ok || !isStoppableStatus(task.LastStatus) {
			return
		}

		prevStatus := task.LastStatus
		clusterName = clusterKey(task.ClusterArn)
		now := time.Now()

		task.LastStatus = statusStopped
		task.DesiredStatus = statusStopped
		task.StoppedAt = &now

		if task.StoppedReason == "" {
			task.StoppedReason = "Essential container in task exited"
		}

		syncContainerStatuses(task, nil)
		setContainerExitCode(task, containerName, exitCode)

		if c, _ := b.clusters.Get(clusterName); c != nil {
			switch prevStatus {
			case statusRunning:
				c.RunningTasksCount--
			case statusPending, statusProvisioning:
				c.PendingTasksCount--
			}
		}

		b.deregisterTaskFromELBv2Locked(task, clusterName)
		delete(b.lifecycle, taskArn)

		instanceArn = task.ContainerInstanceArn
		stopped = true
	}()

	if !stopped {
		return
	}

	func() {
		b.mu.Lock("markTaskStoppedByContainerExit-cleanup")
		defer b.mu.Unlock()

		b.taskProtections.Delete(taskArn)
		b.unindexTaskFromInstance(clusterName, instanceArn, taskArn)
	}()
}

// ListTasksInput holds optional filters for ListTasks.
type ListTasksInput struct {
	Cluster           string
	ContainerInstance string
	Family            string
	ServiceName       string
	DesiredStatus     string
	LaunchType        string
	StartedBy         string
}

func (b *InMemoryBackend) ListTasks(cluster string) ([]string, error) {
	return b.ListTasksFiltered(ListTasksInput{Cluster: cluster})
}

// ListTasksFiltered returns task ARNs matching the given filters.
// Per ListTasksInput.DesiredStatus's doc ("The default status filter is
// RUNNING"), an unset DesiredStatus narrows to RUNNING tasks rather than
// matching every status.
func (b *InMemoryBackend) ListTasksFiltered(input ListTasksInput) ([]string, error) {
	clusterName := clusterKey(b.resolveCluster(input.Cluster))

	b.mu.RLock("ListTasksFiltered")
	defer b.mu.RUnlock()

	if !b.clusters.Has(clusterName) {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, input.Cluster)
	}

	wantDesiredStatus := input.DesiredStatus
	if wantDesiredStatus == "" {
		wantDesiredStatus = statusRunning
	}

	clusterTasks := b.tasksByCluster.Get(clusterName)
	arns := make([]string, 0, len(clusterTasks))
	for _, task := range clusterTasks {
		if input.ContainerInstance != "" && task.ContainerInstanceArn != input.ContainerInstance {
			continue
		}
		if !strings.EqualFold(task.DesiredStatus, wantDesiredStatus) {
			continue
		}
		if input.LaunchType != "" && !strings.EqualFold(task.LaunchType, input.LaunchType) {
			continue
		}
		if input.StartedBy != "" && task.StartedBy != input.StartedBy {
			continue
		}
		if input.Family != "" && !strings.Contains(task.TaskDefinitionArn, "/"+input.Family+":") {
			continue
		}
		if input.ServiceName != "" && task.Group != "service:"+input.ServiceName {
			continue
		}
		arns = append(arns, task.TaskArn)
	}

	return arns, nil
}

// StartTask places tasks on specific container instances (as opposed to RunTask which auto-places).
func (b *InMemoryBackend) StartTask(input StartTaskInput) ([]Task, []Failure, error) {
	if input.TaskDefinition == "" {
		return nil, nil, fmt.Errorf("%w: taskDefinition is required", ErrInvalidParameter)
	}

	if len(input.ContainerInstances) == 0 {
		return nil, nil, fmt.Errorf(
			"%w: at least one container instance is required",
			ErrInvalidParameter,
		)
	}

	clusterName := clusterKey(b.resolveCluster(input.Cluster))

	var (
		ferr     error
		tasks    []Task
		failures []Failure
		td       *TaskDefinition
	)

	func() {
		b.mu.Lock("StartTask")
		defer b.mu.Unlock()

		b.ensureClusterLocked(clusterName)

		var err error

		td, err = b.findTaskDefinitionLocked(input.TaskDefinition)
		if err != nil {
			ferr = err

			return
		}

		clusterArn := arn.Build("ecs", b.region, b.accountID, fmt.Sprintf("cluster/%s", clusterName))

		tasks = make([]Task, 0, len(input.ContainerInstances))
		failures = make([]Failure, 0, len(input.ContainerInstances))

		for _, ciArn := range input.ContainerInstances {
			if _, found := b.containerInstances.Get(scopedKey(clusterName, ciArn)); !found {
				failures = append(failures, Failure{
					Arn:    ciArn,
					Reason: statusMissing,
					Detail: fmt.Sprintf("container instance %s not found", ciArn),
				})

				continue
			}

			taskID := uuid.New().String()
			taskArn := arn.Build(
				"ecs",
				b.region,
				b.accountID,
				fmt.Sprintf("task/%s/%s", clusterName, taskID),
			)
			now := time.Now()

			t := &Task{
				TaskArn:              taskArn,
				ClusterArn:           clusterArn,
				TaskDefinitionArn:    td.TaskDefinitionArn,
				LastStatus:           statusRunning,
				DesiredStatus:        statusRunning,
				Group:                input.Group,
				LaunchType:           "EC2",
				ContainerInstanceArn: ciArn,
				StartedBy:            input.StartedBy,
				StartedAt:            &now,
			}

			b.tasks.Put(t)
			tasks = append(tasks, *t)
		}
	}()

	if ferr != nil {
		return nil, nil, ferr
	}

	for i := range tasks {
		b.ensureAwslogsStreams(&tasks[i], td)
	}

	return tasks, failures, nil
}

// GetTaskProtection returns the protection state for the given tasks on a cluster.
func (b *InMemoryBackend) GetTaskProtection(
	cluster string,
	taskArns []string,
) ([]TaskProtection, []Failure, error) {
	clusterName := clusterKey(b.resolveCluster(cluster))

	b.mu.RLock("GetTaskProtection")
	defer b.mu.RUnlock()

	if !b.clusters.Has(clusterName) {
		return nil, nil, fmt.Errorf("%w: %s", ErrClusterNotFound, cluster)
	}

	if len(taskArns) == 0 {
		clusterTasks := b.tasksByCluster.Get(clusterName)
		out := make([]TaskProtection, 0, len(clusterTasks))

		for _, t := range clusterTasks {
			out = append(out, b.taskProtectionLocked(t.TaskArn))
		}

		return out, nil, nil
	}

	out := make([]TaskProtection, 0, len(taskArns))
	failures := make([]Failure, 0, len(taskArns))

	for _, arn := range taskArns {
		if t, found := b.tasks.Get(arn); !found || clusterKey(t.ClusterArn) != clusterName {
			failures = append(failures, Failure{
				Arn:    arn,
				Reason: statusMissing,
				Detail: fmt.Sprintf("task %s not found", arn),
			})

			continue
		}

		out = append(out, b.taskProtectionLocked(arn))
	}

	return out, failures, nil
}

// taskProtectionLocked returns the protection for a task ARN (defaults to unprotected).
// Must be called with at least an RLock held.
func (b *InMemoryBackend) taskProtectionLocked(taskArn string) TaskProtection {
	if tp, ok := b.taskProtections.Get(taskArn); ok {
		return *tp
	}

	return TaskProtection{TaskArn: taskArn, ProtectionEnabled: false}
}

// UpdateTaskProtection updates task protection for tasks in a cluster.
func (b *InMemoryBackend) UpdateTaskProtection(
	cluster string,
	taskArns []string,
	protectionEnabled bool,
	expiresInMinutes *int,
) ([]TaskProtection, []Failure, error) {
	clusterName := clusterKey(b.resolveCluster(cluster))

	if len(taskArns) == 0 {
		return nil, nil, fmt.Errorf("%w: tasks is required", ErrInvalidParameter)
	}

	b.mu.Lock("UpdateTaskProtection")
	defer b.mu.Unlock()

	if !b.clusters.Has(clusterName) {
		return nil, nil, fmt.Errorf("%w: %s", ErrClusterNotFound, cluster)
	}

	out := make([]TaskProtection, 0, len(taskArns))
	failures := make([]Failure, 0, len(taskArns))

	for _, arn := range taskArns {
		if t, found := b.tasks.Get(arn); !found || clusterKey(t.ClusterArn) != clusterName {
			failures = append(failures, Failure{
				Arn:    arn,
				Reason: statusMissing,
				Detail: fmt.Sprintf("task %s not found", arn),
			})

			continue
		}

		tp := &TaskProtection{
			TaskArn:           arn,
			ProtectionEnabled: protectionEnabled,
		}

		if protectionEnabled && expiresInMinutes != nil {
			exp := time.Now().Add(time.Duration(*expiresInMinutes) * time.Minute)
			tp.ExpirationDate = &exp
		}

		b.taskProtections.Put(tp)
		out = append(out, *tp)
	}

	return out, failures, nil
}

// ExecuteCommand simulates an ECS Exec session.
func (b *InMemoryBackend) ExecuteCommand(
	cluster, task, container, command string,
	interactive bool,
) (*ExecuteCommandOutput, error) {
	if task == "" {
		return nil, fmt.Errorf("%w: task is required", ErrInvalidParameter)
	}

	if command == "" {
		return nil, fmt.Errorf("%w: command is required", ErrInvalidParameter)
	}

	clusterName := clusterKey(b.resolveCluster(cluster))

	b.mu.RLock("ExecuteCommand")
	defer b.mu.RUnlock()

	if !b.clusters.Has(clusterName) {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, cluster)
	}

	t, ok := b.tasks.Get(task)
	if !ok || clusterKey(t.ClusterArn) != clusterName {
		return nil, fmt.Errorf("%w: task %s not found", ErrInvalidParameter, task)
	}

	if t.LastStatus != statusRunning {
		return nil, fmt.Errorf("%w: task %s is not in RUNNING state", ErrInvalidParameter, task)
	}

	if !t.EnableExecuteCommand {
		return nil, fmt.Errorf(
			"%w: ECS Exec is not enabled on task %s; set enableExecuteCommand=true when launching the task",
			ErrInvalidParameter, task,
		)
	}

	clusterObj, _ := b.clusters.Get(clusterName)
	sessionID := uuid.NewString()

	return &ExecuteCommandOutput{
		ClusterArn: clusterObj.ClusterArn,
		ContainerArn: arn.Build(
			"ecs",
			b.region,
			b.accountID,
			fmt.Sprintf("container/%s", uuid.NewString()),
		),
		ContainerName: container,
		TaskArn:       t.TaskArn,
		Interactive:   interactive,
		Session: Session{
			SessionID: sessionID,
			// Honesty signal: gopherstack has no real SSM data channel or ECS Exec
			// agent, so this stream URL is deliberately NOT the real
			// ssmmessages.<region>.amazonaws.com host (which would look connectable
			// but lie). It uses the reserved, non-resolvable ".invalid" TLD
			// (RFC 6761) so callers cannot mistake it for a live AWS endpoint.
			StreamURL: fmt.Sprintf(
				"wss://ssm-emulated.gopherstack.invalid/v1/data-channel/%s",
				sessionID,
			),
			TokenValue: uuid.NewString(),
		},
	}, nil
}
