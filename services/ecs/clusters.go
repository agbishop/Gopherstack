package ecs

import (
	"fmt"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// resolveCluster returns the cluster ARN/name to use, defaulting to "default".
func (b *InMemoryBackend) resolveCluster(cluster string) string {
	if cluster == "" {
		return defaultCluster
	}

	return cluster
}

// clusterKey extracts the cluster name from either a full ARN or a bare name.
func clusterKey(clusterRef string) string {
	if !strings.HasPrefix(clusterRef, "arn:") {
		return clusterRef
	}

	for i := len(clusterRef) - 1; i >= 0; i-- {
		if clusterRef[i] == '/' {
			return clusterRef[i+1:]
		}
	}

	return clusterRef
}

// CreateCluster creates a new ECS cluster.
func (b *InMemoryBackend) CreateCluster(input CreateClusterInput) (*Cluster, error) {
	name := input.ClusterName
	if name == "" {
		name = defaultCluster
	}

	b.mu.Lock("CreateCluster")
	defer b.mu.Unlock()

	// Real ECS's CreateCluster is idempotent: calling it again with an
	// existing ClusterName returns the existing cluster rather than
	// erroring (CreateCluster's own deserializeOpError models no
	// "already exists" exception at all, and no such type exists anywhere
	// in ecs@v1.90.0's SDK).
	if existing, ok := b.clusters.Get(name); ok {
		cp := *existing

		return &cp, nil
	}

	if err := b.validateCapacityProviderStrategyLocked(input.DefaultCapacityProviderStrategy); err != nil {
		return nil, err
	}

	cluster := &Cluster{
		CreatedAt:                       time.Now(),
		ClusterArn:                      arn.Build("ecs", b.region, b.accountID, fmt.Sprintf("cluster/%s", name)),
		ClusterName:                     name,
		Status:                          statusActive,
		Settings:                        input.Settings,
		CapacityProviders:               input.CapacityProviders,
		DefaultCapacityProviderStrategy: input.DefaultCapacityProviderStrategy,
		ServiceConnectDefaults:          input.ServiceConnectDefaults,
	}
	b.clusters.Put(cluster)

	if len(input.Tags) > 0 {
		b.setResourceTagsLocked(cluster.ClusterArn, input.Tags)
	}

	cp := *cluster

	return &cp, nil
}

// ListClusters returns all clusters.
func (b *InMemoryBackend) ListClusters() ([]Cluster, error) {
	b.mu.RLock("ListClusters")
	defer b.mu.RUnlock()

	all := b.clusters.All()
	out := make([]Cluster, 0, len(all))
	for _, c := range all {
		out = append(out, b.enrichCluster(c))
	}

	return out, nil
}

// DescribeClusters returns cluster metadata. Per DescribeClustersInput.Clusters
// ("If you do not specify a cluster, the default cluster is assumed."), an
// empty clusterNames describes only the "default" cluster -- unlike
// ListClusters, a different operation whose own input documents no such
// default-substitution and legitimately returns everything.
// Unknown cluster names are returned as failures, not errors, matching AWS behaviour.
func (b *InMemoryBackend) DescribeClusters(clusterNames []string) ([]Cluster, []Failure, error) {
	b.mu.Lock("DescribeClusters")
	defer b.mu.Unlock()

	if len(clusterNames) == 0 {
		b.ensureClusterLocked(defaultCluster)
		clusterNames = []string{defaultCluster}
	}

	out := make([]Cluster, 0, len(clusterNames))
	failures := make([]Failure, 0, len(clusterNames))

	for _, name := range clusterNames {
		key := clusterKey(name)

		c, ok := b.clusters.Get(key)
		if !ok {
			failures = append(failures, Failure{
				Arn:    name,
				Reason: statusMissing,
				Detail: fmt.Sprintf("cluster %s not found", name),
			})

			continue
		}

		out = append(out, b.enrichCluster(c))
	}

	return out, failures, nil
}

// enrichCluster fills in runtime-computed counts for a cluster.
// Must be called with at least an RLock held.
// Running and pending task counts are cached on the cluster struct and updated
// incrementally at each state transition to avoid an O(n) task scan here.
func (b *InMemoryBackend) enrichCluster(c *Cluster) Cluster {
	cp := *c

	cp.ActiveServicesCount = len(b.servicesByCluster.Get(c.ClusterName))
	cp.RegisteredContainerInstancesCount = len(b.containerInstancesByCluster.Get(c.ClusterName))

	// RunningTasksCount and PendingTasksCount are maintained as cached counters
	// on the Cluster struct. No task iteration needed here.

	return cp
}

// clusterDependencyViolationLocked returns the AWS ClusterContains*Exception
// naming the first dependent resource type blocking deletion of clusterName,
// or nil if the cluster has none. Mirrors real AWS: DeleteCluster refuses
// rather than cascading -- services and tasks must be deleted, and container
// instances deregistered, first. Must be called with b.mu held.
func (b *InMemoryBackend) clusterDependencyViolationLocked(clusterName string) error {
	if svcs := b.servicesInClusterLocked(clusterName); len(svcs) > 0 {
		return fmt.Errorf("%w: cluster %s still has services", ErrClusterContainsServices, clusterName)
	}

	for _, task := range b.tasksInClusterLocked(clusterName) {
		if task.LastStatus != statusStopped {
			return fmt.Errorf("%w: cluster %s still has active tasks", ErrClusterContainsTasks, clusterName)
		}
	}

	if ci := b.containerInstancesInClusterLocked(clusterName); len(ci) > 0 {
		return fmt.Errorf(
			"%w: cluster %s still has registered container instances",
			ErrClusterContainsContainerInstances, clusterName,
		)
	}

	return nil
}

// DeleteCluster removes a cluster. Matching real AWS, this fails with a
// ClusterContains*Exception if the cluster still has services, active tasks,
// or registered container instances -- it does NOT cascade-delete them.
func (b *InMemoryBackend) DeleteCluster(clusterName string) (*Cluster, error) {
	key := clusterKey(clusterName)

	var (
		found       bool
		cp          Cluster
		tasksToStop []*Task
	)

	// Snapshot task pointers while still holding the lock so we can defensively
	// tell the runner to stop their containers after releasing it. Performing
	// Docker API calls under the backend lock would unnecessarily serialize
	// other operations; StopTask is a no-op for a task with no tracked
	// container, so this is safe even though these tasks are already STOPPED.
	guardErr := func() error {
		b.mu.Lock("DeleteCluster")
		defer b.mu.Unlock()

		c, ok := b.clusters.Get(key)
		if !ok {
			return nil
		}

		found = true

		if err := b.clusterDependencyViolationLocked(key); err != nil {
			return err
		}

		// Only already-STOPPED tasks (and empty service/container-instance
		// sets) can remain here -- clusterDependencyViolationLocked already
		// refused anything else. Clear their bookkeeping so a later
		// same-named cluster doesn't inherit stale entries.
		clusterTasks := b.tasksInClusterLocked(key)
		if b.runner != nil {
			tasksToStop = append(tasksToStop, clusterTasks...)
		}

		for _, task := range clusterTasks {
			b.taskProtections.Delete(task.TaskArn)
			delete(b.lifecycle, task.TaskArn)
		}

		b.clusters.Delete(key)
		b.deleteResourceTagsLocked(c.ClusterArn)
		b.deleteServicesForClusterLocked(key)
		b.deleteTasksForClusterLocked(key)
		b.deleteContainerInstancesForClusterLocked(key)
		delete(b.attributes, key)
		delete(b.tasksByInstance, key)

		cp = *c

		return nil
	}()

	if !found {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, clusterName)
	}

	if guardErr != nil {
		return nil, guardErr
	}

	// Docker API calls happen outside the lock so other backend operations are
	// not serialized behind potentially slow container stops.
	for _, task := range tasksToStop {
		_ = b.runner.StopTask(task)
	}

	// Notify hooks (e.g. reconciler semaphore eviction) after releasing the lock.
	b.fireClusterDeleteHooks(key)

	return &cp, nil
}

// ensureClusterLocked returns the cluster maps, auto-creating the default cluster if needed.
// Must be called with write lock held.
func (b *InMemoryBackend) ensureClusterLocked(clusterName string) {
	if !b.clusters.Has(clusterName) && clusterName == defaultCluster {
		b.clusters.Put(&Cluster{
			CreatedAt: time.Now(),
			ClusterArn: arn.Build(
				"ecs",
				b.region,
				b.accountID,
				fmt.Sprintf("cluster/%s", clusterName),
			),
			ClusterName: clusterName,
			Status:      statusActive,
		})
	}
}

// PutClusterCapacityProviders associates capacity providers with a cluster.
func (b *InMemoryBackend) PutClusterCapacityProviders(
	cluster string,
	capacityProviders []string,
	defaultCapacityProviderStrategy []CapacityProviderStrategyItem,
) (*Cluster, error) {
	clusterName := clusterKey(b.resolveCluster(cluster))

	b.mu.Lock("PutClusterCapacityProviders")
	defer b.mu.Unlock()

	c, ok := b.clusters.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, cluster)
	}

	if err := b.validateCapacityProviderStrategyLocked(defaultCapacityProviderStrategy); err != nil {
		return nil, err
	}

	c.CapacityProviders = capacityProviders
	c.DefaultCapacityProviderStrategy = defaultCapacityProviderStrategy

	cp := b.enrichCluster(c)

	return &cp, nil
}

// UpdateClusterSettings updates the settings for an ECS cluster.
func (b *InMemoryBackend) UpdateClusterSettings(
	cluster string,
	settings []ClusterSetting,
) (*Cluster, error) {
	clusterName := clusterKey(b.resolveCluster(cluster))

	b.mu.Lock("UpdateClusterSettings")
	defer b.mu.Unlock()

	c, ok := b.clusters.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, cluster)
	}

	c.Settings = settings

	cp := b.enrichCluster(c)

	return &cp, nil
}

// UpdateCluster updates an ECS cluster's settings. Capacity-provider
// association is not part of this operation in real AWS -- see
// PutClusterCapacityProviders for that.
func (b *InMemoryBackend) UpdateCluster(input UpdateClusterInput) (*Cluster, error) {
	clusterName := clusterKey(b.resolveCluster(input.Cluster))

	b.mu.Lock("UpdateCluster")
	defer b.mu.Unlock()

	c, ok := b.clusters.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, input.Cluster)
	}

	if input.Settings != nil {
		c.Settings = input.Settings
	}

	if input.ServiceConnectDefaults != nil {
		c.ServiceConnectDefaults = input.ServiceConnectDefaults
	}

	cp := b.enrichCluster(c)

	return &cp, nil
}
