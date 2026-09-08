package ecs

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	deploymentRolloutStateInProgress = "IN_PROGRESS"
	deploymentRolloutStateCompleted  = "COMPLETED"
	deploymentRolloutStateFailed     = "FAILED"
)

// validateDeploymentController returns an error for unsupported controller types.
func validateDeploymentController(dc *DeploymentController) error {
	if dc == nil {
		return nil
	}

	switch strings.ToUpper(dc.Type) {
	case deploymentControllerRolling, "":
		return nil
	case deploymentControllerExternal:
		return nil
	case deploymentControllerCodeDeploy:
		return fmt.Errorf(
			"%w: CODE_DEPLOY deployment controller is not supported; use ROLLING or EXTERNAL",
			ErrInvalidParameter,
		)
	default:
		return fmt.Errorf(
			"%w: unknown deployment controller type %q; valid values: ROLLING, EXTERNAL",
			ErrInvalidParameter, dc.Type,
		)
	}
}

// serviceRevisionArnFor derives the ARN of the service revision captured by a
// deployment from the owning service's ARN, following the
// arn:aws:ecs:region:account:service-revision/cluster/service/id scheme.
func serviceRevisionArnFor(svc *Service, revisionID string) string {
	return strings.Replace(svc.ServiceArn, ":service/", ":service-revision/", 1) + "/" + revisionID
}

// newPrimaryDeployment builds the initial PRIMARY deployment for a newly created service.
func newPrimaryDeployment(svc *Service) Deployment {
	now := float64UnixNow()
	revisionID := uuid.NewString()

	return Deployment{
		ID:                 "ecs-svc/" + uuid.NewString(),
		Status:             deploymentStatusPrimary,
		TaskDefinition:     svc.TaskDefinition,
		LaunchType:         svc.LaunchType,
		PlatformVersion:    platformVersionLatest,
		RolloutState:       deploymentRolloutStateInProgress,
		RolloutStateReason: "ECS deployment ecs-svc created.",
		ServiceRevisionArn: serviceRevisionArnFor(svc, revisionID),
		DesiredCount:       svc.DesiredCount,
		CreatedAt:          &now,
		UpdatedAt:          &now,
	}
}

// newActiveDeployment builds a new primary deployment on service update.
// The previous PRIMARY deployment is expected to be demoted to ACTIVE separately.
func newActiveDeployment(svc *Service) Deployment {
	now := float64UnixNow()
	revisionID := uuid.NewString()

	return Deployment{
		ID:                 "ecs-svc/" + uuid.NewString(),
		Status:             deploymentStatusPrimary,
		TaskDefinition:     svc.TaskDefinition,
		LaunchType:         svc.LaunchType,
		PlatformVersion:    platformVersionLatest,
		RolloutState:       deploymentRolloutStateInProgress,
		RolloutStateReason: "ECS deployment ecs-svc created.",
		ServiceRevisionArn: serviceRevisionArnFor(svc, revisionID),
		DesiredCount:       svc.DesiredCount,
		CreatedAt:          &now,
		UpdatedAt:          &now,
	}
}

// buildServiceRevision projects a service's current configuration and a specific
// deployment record into the ServiceRevision shape returned by DescribeServiceRevisions.
func buildServiceRevision(svc *Service, d Deployment) ServiceRevision {
	return ServiceRevision{
		CapacityProviderStrategy:    svc.CapacityProviderStrategy,
		ClusterArn:                  svc.ClusterArn,
		CreatedAt:                   d.CreatedAt,
		LaunchType:                  d.LaunchType,
		LoadBalancers:               svc.LoadBalancers,
		NetworkConfiguration:        svc.NetworkConfiguration,
		Monitoring:                  svc.Monitoring,
		PlatformVersion:             d.PlatformVersion,
		ServiceArn:                  svc.ServiceArn,
		ServiceConnectConfiguration: svc.ServiceConnectConfiguration,
		ServiceRegistries:           svc.ServiceRegistries,
		ServiceRevisionArn:          d.ServiceRevisionArn,
		TaskDefinition:              d.TaskDefinition,
	}
}

// createServiceDefaults resolves CreateServiceInput's own documented
// per-field defaults for launchType, schedulingStrategy, propagateTags, and
// AvailabilityZoneRebalancing (which defaults to ENABLED when unspecified).
func createServiceDefaults(input CreateServiceInput) (string, string, string, string) {
	launchType := input.LaunchType
	if launchType == "" {
		launchType = launchTypeFargate
	}

	schedulingStrategy := input.SchedulingStrategy
	if schedulingStrategy == "" {
		schedulingStrategy = "REPLICA"
	}

	propagateTags := input.PropagateTags
	if propagateTags == "" {
		propagateTags = propagateTagsNone
	}

	azRebalancing := input.AvailabilityZoneRebalancing
	if azRebalancing == "" {
		azRebalancing = azRebalancingEnabled
	}

	return launchType, schedulingStrategy, propagateTags, azRebalancing
}

// CreateService creates a new ECS service.
func (b *InMemoryBackend) CreateService(input CreateServiceInput) (*Service, error) {
	if input.ServiceName == "" {
		return nil, fmt.Errorf("%w: serviceName is required", ErrInvalidParameter)
	}

	if input.TaskDefinition == "" {
		return nil, fmt.Errorf("%w: taskDefinition is required", ErrInvalidParameter)
	}

	clusterName := clusterKey(b.resolveCluster(input.Cluster))

	if err := validateDeploymentController(input.DeploymentController); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateService")
	defer b.mu.Unlock()

	b.ensureClusterLocked(clusterName)

	if b.services.Has(scopedKey(clusterName, input.ServiceName)) {
		return nil, fmt.Errorf("%w: service %s already exists", ErrInvalidParameter, input.ServiceName)
	}

	if err := b.validateCapacityProviderStrategyLocked(input.CapacityProviderStrategy); err != nil {
		return nil, err
	}

	td, err := b.findTaskDefinitionLocked(input.TaskDefinition)
	if err != nil {
		return nil, err
	}

	launchType, schedulingStrategy, propagateTags, azRebalancing := createServiceDefaults(input)

	svc := &Service{
		CreatedAt: time.Now(),
		ServiceArn: fmt.Sprintf(
			"arn:aws:ecs:%s:%s:service/%s/%s",
			b.region,
			b.accountID,
			clusterName,
			input.ServiceName,
		),
		ServiceName: input.ServiceName,
		ClusterArn: arn.Build(
			"ecs",
			b.region,
			b.accountID,
			fmt.Sprintf("cluster/%s", clusterName),
		),
		TaskDefinition:                td.TaskDefinitionArn,
		Status:                        statusActive,
		LaunchType:                    launchType,
		SchedulingStrategy:            schedulingStrategy,
		PropagateTags:                 propagateTags,
		AvailabilityZoneRebalancing:   azRebalancing,
		Tags:                          input.Tags,
		LoadBalancers:                 input.LoadBalancers,
		ServiceRegistries:             input.ServiceRegistries,
		DeploymentConfiguration:       input.DeploymentConfiguration.withAWSDefaults(),
		DeploymentController:          input.DeploymentController,
		NetworkConfiguration:          input.NetworkConfiguration,
		CapacityProviderStrategy:      input.CapacityProviderStrategy,
		PlacementConstraints:          input.PlacementConstraints,
		PlacementStrategy:             input.PlacementStrategy,
		ServiceConnectConfiguration:   input.ServiceConnectConfiguration,
		HealthCheckGracePeriodSeconds: healthCheckGracePeriodOrDefault(input.HealthCheckGracePeriodSeconds),
		Monitoring:                    input.Monitoring,
		DesiredCount:                  input.DesiredCount,
		EnableExecuteCommand:          input.EnableExecuteCommand,
	}

	svc.Deployments = []Deployment{newPrimaryDeployment(svc)}

	b.services.Put(svc)
	b.serviceIndex[svcRef{cluster: clusterName, name: input.ServiceName}] = true

	b.addServiceRevisionLocked(svc)
	b.syncServiceDeploymentsLocked(svc)

	// Mirror tags into the resourceTags side map so TagResource/UntagResource/
	// ListTagsForResource (and DescribeServices with Include=[TAGS], which reads
	// tags from this map -- see enrichService below) see the tags applied at
	// creation; svc.Tags and resourceTags are independent copies that must stay
	// synchronized.
	if len(input.Tags) > 0 {
		b.setResourceTagsLocked(svc.ServiceArn, input.Tags)
	}

	cp := *svc
	cp.Tags = copyTags(b.resourceTags[resourceTagKey(svc.ServiceArn)])

	return &cp, nil
}

// DescribeServices returns services for the given cluster, optionally filtered by name.
// Unknown service names are returned as failures, not errors, matching AWS behaviour.
func (b *InMemoryBackend) DescribeServices(
	cluster string,
	serviceNames []string,
) ([]Service, []Failure, error) {
	clusterName := clusterKey(b.resolveCluster(cluster))

	b.mu.RLock("DescribeServices")
	defer b.mu.RUnlock()

	if !b.clusters.Has(clusterName) {
		return nil, nil, fmt.Errorf("%w: %s", ErrClusterNotFound, cluster)
	}

	if len(serviceNames) == 0 {
		svcs := b.servicesByCluster.Get(clusterName)
		out := make([]Service, 0, len(svcs))
		for _, s := range svcs {
			out = append(out, b.enrichService(s, clusterName))
		}

		return out, nil, nil
	}

	out := make([]Service, 0, len(serviceNames))
	failures := make([]Failure, 0, len(serviceNames))

	for _, name := range serviceNames {
		// Support ARN lookup by extracting the service name.
		key := serviceKey(name)

		s, found := b.services.Get(scopedKey(clusterName, key))
		if !found {
			failures = append(failures, Failure{
				Arn:    name,
				Reason: statusMissing,
				Detail: fmt.Sprintf("service %s not found", name),
			})

			continue
		}

		out = append(out, b.enrichService(s, clusterName))
	}

	return out, failures, nil
}

// DescribeServiceRevisions returns the service revisions captured by past and
// current deployments across all clusters that match the given ARNs.
// Unknown ARNs are reported as failures, matching AWS behaviour for batch describes.
func (b *InMemoryBackend) DescribeServiceRevisions(arns []string) ([]ServiceRevision, []Failure) {
	b.mu.RLock("DescribeServiceRevisions")
	defer b.mu.RUnlock()

	byArn := make(map[string]ServiceRevision)

	for _, svc := range b.services.All() {
		for _, d := range svc.Deployments {
			if d.ServiceRevisionArn == "" {
				continue
			}

			byArn[d.ServiceRevisionArn] = buildServiceRevision(svc, d)
		}
	}

	out := make([]ServiceRevision, 0, len(arns))
	failures := make([]Failure, 0)

	for _, arn := range arns {
		rev, ok := byArn[arn]
		if !ok {
			failures = append(failures, Failure{
				Arn:    arn,
				Reason: statusMissing,
				Detail: fmt.Sprintf("service revision %s not found", arn),
			})

			continue
		}

		out = append(out, rev)
	}

	return out, failures
}

// serviceKey extracts service name from an ARN or returns name as-is.
func serviceKey(serviceRef string) string {
	for i := len(serviceRef) - 1; i >= 0; i-- {
		if serviceRef[i] == '/' {
			return serviceRef[i+1:]
		}
	}

	return serviceRef
}

// enrichService fills in runtime-computed counts for a service and
// updates each deployment's RunningCount/PendingCount + RolloutState.
// Must be called with at least an RLock held.
func (b *InMemoryBackend) enrichService(s *Service, clusterName string) Service {
	cp := *s

	running := 0
	pending := 0

	// Per-deployment counters keyed by task definition ARN.
	deplRunning := make(map[string]int, len(s.Deployments))
	deplPending := make(map[string]int, len(s.Deployments))

	for _, t := range b.tasksByCluster.Get(clusterName) {
		if t.Group == "service:"+s.ServiceName {
			switch t.LastStatus {
			case statusRunning:
				running++
				deplRunning[t.TaskDefinitionArn]++
			case statusPending, statusProvisioning:
				pending++
				deplPending[t.TaskDefinitionArn]++
			}
		}
	}

	cp.RunningCount = running
	cp.PendingCount = pending

	// Update per-deployment counts and advance RolloutState to COMPLETED
	// when the PRIMARY deployment has reached its desired count.
	deployments := make([]Deployment, len(s.Deployments))

	for i, d := range s.Deployments {
		d.RunningCount = deplRunning[d.TaskDefinition]
		d.PendingCount = deplPending[d.TaskDefinition]

		if d.Status == deploymentStatusPrimary &&
			d.RolloutState == deploymentRolloutStateInProgress &&
			d.RunningCount >= d.DesiredCount && d.DesiredCount > 0 {
			d.RolloutState = deploymentRolloutStateCompleted
			d.RolloutStateReason = fmt.Sprintf(
				"ECS deployment ecs-svc completed. %d out of %d tasks running.",
				d.RunningCount, d.DesiredCount,
			)
		}

		deployments[i] = d
	}

	cp.Deployments = deployments

	return cp
}

func applyServiceConfigUpdates(svc *Service, input UpdateServiceInput) {
	if len(input.CapacityProviderStrategy) > 0 {
		svc.CapacityProviderStrategy = input.CapacityProviderStrategy
	}

	if len(input.PlacementConstraints) > 0 {
		svc.PlacementConstraints = input.PlacementConstraints
	}

	if len(input.PlacementStrategy) > 0 {
		svc.PlacementStrategy = input.PlacementStrategy
	}

	if input.ServiceConnectConfiguration != nil {
		svc.ServiceConnectConfiguration = input.ServiceConnectConfiguration
	}

	if input.NetworkConfiguration != nil {
		svc.NetworkConfiguration = input.NetworkConfiguration
	}

	if input.PropagateTags != "" {
		svc.PropagateTags = input.PropagateTags
	}

	if len(input.LoadBalancers) > 0 {
		svc.LoadBalancers = input.LoadBalancers
	}

	if input.EnableExecuteCommand != nil {
		svc.EnableExecuteCommand = *input.EnableExecuteCommand
	}

	// UpdateServiceInput.AvailabilityZoneRebalancing's own doc comment: "For
	// update service requests, when no value is specified ... Amazon ECS
	// defaults to the existing service's AvailabilityZoneRebalancing value" --
	// so an empty input leaves svc.AvailabilityZoneRebalancing untouched.
	if input.AvailabilityZoneRebalancing != "" {
		svc.AvailabilityZoneRebalancing = input.AvailabilityZoneRebalancing
	}

	if input.HealthCheckGracePeriodSeconds != nil {
		svc.HealthCheckGracePeriodSeconds = input.HealthCheckGracePeriodSeconds
	}

	if input.Monitoring != nil {
		svc.Monitoring = input.Monitoring
	}
}

// healthCheckGracePeriodOrDefault applies CreateServiceInput's own doc
// comment, which states that if a grace period value isn't specified, the
// default value of 0 is used.
func healthCheckGracePeriodOrDefault(v *int) *int {
	if v != nil {
		return v
	}

	zero := 0

	return &zero
}

// UpdateService updates an existing ECS service.
func (b *InMemoryBackend) UpdateService(input UpdateServiceInput) (*Service, error) {
	if input.Service == "" {
		return nil, fmt.Errorf("%w: service is required", ErrInvalidParameter)
	}

	clusterName := clusterKey(b.resolveCluster(input.Cluster))
	serviceKey := serviceKey(input.Service)

	b.mu.Lock("UpdateService")
	defer b.mu.Unlock()

	if !b.clusters.Has(clusterName) {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, input.Cluster)
	}

	svc, ok := b.services.Get(scopedKey(clusterName, serviceKey))
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrServiceNotFound, input.Service)
	}

	if len(input.CapacityProviderStrategy) > 0 {
		if err := b.validateCapacityProviderStrategyLocked(input.CapacityProviderStrategy); err != nil {
			return nil, err
		}
	}

	if input.DesiredCount != nil {
		svc.DesiredCount = *input.DesiredCount
	}

	newTaskDef := false

	if input.TaskDefinition != "" {
		td, err := b.findTaskDefinitionLocked(input.TaskDefinition)
		if err != nil {
			return nil, err
		}

		svc.TaskDefinition = td.TaskDefinitionArn
		newTaskDef = true
	}

	// UpdateServiceInput.ForceNewDeployment's own doc comment: "you can use
	// this option to start a new deployment with no service definition
	// changes" -- so a deployment must be rotated even when TaskDefinition
	// wasn't itself changed, reusing the service's current one.
	if newTaskDef || input.ForceNewDeployment {
		// Create a new PRIMARY deployment and demote the old one to ACTIVE.
		svc.Deployments = rotatePrimaryDeployment(svc)
	}

	if input.DeploymentConfiguration != nil {
		svc.DeploymentConfiguration = input.DeploymentConfiguration
	}

	applyServiceConfigUpdates(svc, input)

	b.addServiceRevisionLocked(svc)
	b.syncServiceDeploymentsLocked(svc)

	cp := *svc
	// resourceTags (kept in sync by TagResource/UntagResource) is authoritative,
	// not the creation-time svc.Tags snapshot -- see CreateService.
	cp.Tags = copyTags(b.resourceTags[resourceTagKey(svc.ServiceArn)])

	return &cp, nil
}

// rotatePrimaryDeployment demotes the existing PRIMARY deployment to ACTIVE and
// prepends a fresh PRIMARY for the new task definition.
func rotatePrimaryDeployment(svc *Service) []Deployment {
	deployments := make([]Deployment, 0, len(svc.Deployments)+1)
	newPrimary := newActiveDeployment(svc)
	deployments = append(deployments, newPrimary)

	for _, d := range svc.Deployments {
		if d.Status == deploymentStatusPrimary {
			d.Status = statusActive
		}

		deployments = append(deployments, d)
	}

	return deployments
}

// DeleteService removes a service from the cluster.
func (b *InMemoryBackend) DeleteService(cluster, serviceName string, force ...bool) (*Service, error) {
	clusterName := clusterKey(b.resolveCluster(cluster))
	key := serviceKey(serviceName)
	forced := len(force) > 0 && force[0]

	b.mu.Lock("DeleteService")
	defer b.mu.Unlock()

	if !b.clusters.Has(clusterName) {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, cluster)
	}

	svc, ok := b.services.Get(scopedKey(clusterName, key))
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrServiceNotFound, serviceName)
	}

	if !forced && (svc.DesiredCount != 0 || svc.RunningCount != 0) {
		return nil, fmt.Errorf(
			"%w: the service %s is still active (desiredCount=%d, runningCount=%d); "+
				"update it to a desired count of zero or delete with force",
			ErrInvalidParameter, serviceName, svc.DesiredCount, svc.RunningCount,
		)
	}

	// Capture the authoritative tags before deleteResourceTagsLocked below
	// clears the resourceTags side-map entry, so the deleted-service snapshot
	// returned to the caller still reflects the tags that were in effect
	// (matching real AWS, which echoes the deleted resource's tags on
	// DeleteService, not an empty set).
	tags := copyTags(b.resourceTags[resourceTagKey(svc.ServiceArn)])

	b.services.Delete(scopedKey(clusterName, key))
	b.deleteTaskSetsForServiceLocked(svc.ServiceArn)
	delete(b.serviceIndex, svcRef{cluster: clusterName, name: key})
	b.deleteServiceDeploymentsForServiceLocked(svc.ServiceArn)
	b.deleteResourceTagsLocked(svc.ServiceArn)

	cp := *svc
	cp.Tags = tags

	return &cp, nil
}

// getServicesForReconciler returns a snapshot of all services for the reconciler.
// Uses the flat serviceIndex for O(len) single-pass iteration without nested loops
// or a pre-counting pass.
func (b *InMemoryBackend) getServicesForReconciler() []serviceSnapshot {
	b.mu.RLock("GetServicesForReconciler")
	defer b.mu.RUnlock()

	out := make([]serviceSnapshot, 0, len(b.serviceIndex))

	for ref := range b.serviceIndex {
		svc, _ := b.services.Get(scopedKey(ref.cluster, ref.name))
		out = append(out, serviceSnapshot{
			clusterName: ref.cluster,
			service:     cloneServiceForSnapshot(svc),
		})
	}

	return out
}

// cloneServiceForSnapshot copies svc for use outside the lock. `*svc` alone is
// not enough: Deployments is written in place by element (see
// recordServiceTaskFailureLocked's `svc.Deployments[idx].FailedTasks++` and
// evaluateCircuitBreakerLocked's `dep.RolloutState = ...`), so a shallow copy's
// Deployments slice would still alias the live backing array the reconciler
// reads from unlocked -- exactly the class of race gopherstack-urw6 audits for.
// Must be called with at least the read lock held.
func cloneServiceForSnapshot(svc *Service) Service {
	cp := *svc
	cp.Deployments = append([]Deployment(nil), svc.Deployments...)

	return cp
}

// serviceSnapshot is a point-in-time copy of a service for the reconciler.
type serviceSnapshot struct {
	clusterName string
	service     Service
}

// CountRunningTasksForService counts running tasks for a service on a cluster.
func (b *InMemoryBackend) CountRunningTasksForService(clusterName, serviceName string) int {
	b.mu.RLock("CountRunningTasksForService")
	defer b.mu.RUnlock()

	count := 0
	group := "service:" + serviceName

	for _, t := range b.tasksByCluster.Get(clusterName) {
		if t.Group == group && t.LastStatus == statusRunning {
			count++
		}
	}

	return count
}

// StartTaskForService launches a task on behalf of a service.
// It reads the service's PropagateTags, Tags, and EnableECSManagedTags settings so
// that tasks spawned by the reconciler honour the same tag propagation as tasks
// started directly via RunTask.
func (b *InMemoryBackend) StartTaskForService(
	clusterName, serviceName, taskDefinitionArn string,
) error {
	// Snapshot service config without holding the lock during RunTask.
	var svcPropagateTags string
	var svcTags []Tag
	var svcEnableExec bool
	var svcLaunchType string
	var svcPlacementConstraints []PlacementConstraint
	var svcPlacementStrategy []PlacementStrategy

	func() {
		b.mu.RLock("StartTaskForService-svcSnap")
		defer b.mu.RUnlock()

		if svc, found := b.services.Get(scopedKey(clusterName, serviceName)); found {
			svcPropagateTags = svc.PropagateTags
			// resourceTags (kept in sync by TagResource/UntagResource) is
			// authoritative, not the creation-time svc.Tags snapshot -- see
			// CreateService. A PropagateTags=SERVICE task must inherit the
			// service's CURRENT tags, including any applied via TagResource
			// after creation, not just the tags supplied at CreateService time.
			svcTags = copyTags(b.resourceTags[resourceTagKey(svc.ServiceArn)])
			svcEnableExec = svc.EnableExecuteCommand
			svcLaunchType = svc.LaunchType
			svcPlacementConstraints = svc.PlacementConstraints
			svcPlacementStrategy = svc.PlacementStrategy
		}
	}()

	_, err := b.RunTask(RunTaskInput{
		Cluster:                 clusterName,
		TaskDefinition:          taskDefinitionArn,
		Count:                   1,
		Group:                   "service:" + serviceName,
		LaunchType:              svcLaunchType,
		PropagateTags:           svcPropagateTags,
		serviceNameForTags:      serviceName,
		serviceTagsForPropagate: svcTags,
		EnableExecuteCommand:    svcEnableExec,
		PlacementConstraints:    svcPlacementConstraints,
		PlacementStrategy:       svcPlacementStrategy,
	})

	return err
}

// StopOldestServiceTask stops the oldest running task for a service.
func (b *InMemoryBackend) StopOldestServiceTask(clusterName, serviceName string) error {
	var (
		oldest      *Task
		instanceArn string
	)

	func() {
		b.mu.Lock("StopOldestServiceTask")
		defer b.mu.Unlock()

		group := "service:" + serviceName

		for _, t := range b.tasksByCluster.Get(clusterName) {
			if t.Group == group && t.LastStatus == statusRunning {
				if oldest == nil ||
					(t.StartedAt != nil && oldest.StartedAt != nil && t.StartedAt.Before(*oldest.StartedAt)) {
					oldest = t
				}
			}
		}

		if oldest == nil {
			return
		}

		now := time.Now()
		oldest.LastStatus = statusStopped
		oldest.DesiredStatus = statusStopped
		oldest.StoppedAt = &now
		oldest.StoppedReason = "service scale-in"
		syncContainerStatuses(oldest, nil)
		b.deregisterTaskFromELBv2Locked(oldest, clusterName)

		// Decrement the cached running counter (scale-in always stops a running task).
		if c, _ := b.clusters.Get(clusterName); c != nil {
			c.RunningTasksCount--
		}

		instanceArn = oldest.ContainerInstanceArn
	}()

	if oldest == nil {
		return nil
	}

	taskArn := oldest.TaskArn

	// Docker API calls happen outside the lock so other backend operations are
	// not serialized behind potentially slow container stops.
	if b.runner != nil {
		_ = b.runner.StopTask(oldest)
	}

	func() {
		b.mu.Lock("StopOldestServiceTask-unindex")
		defer b.mu.Unlock()

		b.unindexTaskFromInstance(clusterName, instanceArn, taskArn)
	}()

	return nil
}

// ListServicesByNamespace returns service ARNs in a cluster whose service name contains the namespace prefix.
func (b *InMemoryBackend) ListServicesByNamespace(cluster, namespace string) ([]string, error) {
	clusterName := clusterKey(b.resolveCluster(cluster))

	b.mu.RLock("ListServicesByNamespace")
	defer b.mu.RUnlock()

	svcs := b.servicesByCluster.Get(clusterName)

	out := make([]string, 0, len(svcs))

	for _, svc := range svcs {
		if namespace == "" || strings.Contains(svc.ServiceName, namespace) {
			out = append(out, svc.ServiceArn)
		}
	}

	return out, nil
}

// ListServices returns service ARNs for a cluster, optionally filtered by launch type and scheduling strategy.
func (b *InMemoryBackend) ListServices(
	cluster, launchType, schedulingStrategy string,
) ([]string, error) {
	clusterName := clusterKey(b.resolveCluster(cluster))

	b.mu.RLock("ListServices")
	defer b.mu.RUnlock()

	if !b.clusters.Has(clusterName) {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, cluster)
	}

	svcs := b.servicesByCluster.Get(clusterName)
	arns := make([]string, 0, len(svcs))

	for _, svc := range svcs {
		if launchType != "" && svc.LaunchType != launchType {
			continue
		}

		if schedulingStrategy != "" && svc.SchedulingStrategy != schedulingStrategy {
			continue
		}

		arns = append(arns, svc.ServiceArn)
	}

	// Stable order so offset-based (cursor) pagination is consistent across calls;
	// b.services is a map, whose iteration order is otherwise random.
	sort.Strings(arns)

	return arns, nil
}
