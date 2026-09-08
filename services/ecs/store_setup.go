package ecs

// Every map[string]*T resource field on InMemoryBackend that is a pure function
// of the stored value's own identity is registered exactly once, here, as a
// *store.Table[T] on b.registry (see pkgs/store's package doc).
//
// Resources nested as map[string]map[string]*T (cluster -> resource-name ->
// value) are flattened into a single Table keyed by a composite "cluster/name"
// string, with a secondary store.Index grouping by cluster so the old "all X in
// cluster Y" access pattern still resolves in O(k).
//
// A handful of fields are deliberately NOT registered here and remain plain
// maps -- see the comment above registerAllTables for the list and why.
import (
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// scopedKey builds a composite store.Table key for a resource whose identity
// is only unique within an owning scope (a cluster name for services/
// containerInstances/daemons, a service ARN for task sets). It mirrors the
// two-level map[scope]map[id]*T nesting it replaces.
func scopedKey(scope, id string) string { return scope + "/" + id }

// ---- primary key functions ----

func clustersKeyFn(v *Cluster) string                             { return v.ClusterName }
func capacityProvidersKeyFn(v *CapacityProvider) string           { return v.Name }
func accountSettingsKeyFn(v *AccountSetting) string               { return accountSettingKey(v.Name, v.PrincipalArn) }
func taskProtectionsKeyFn(v *TaskProtection) string               { return v.TaskArn }
func serviceDeploymentsKeyFn(v *ServiceDeployment) string         { return v.ServiceDeploymentArn }
func expressGatewayServicesKeyFn(v *ExpressGatewayService) string { return v.ServiceArn }
func daemonRevisionsKeyFn(v *DaemonRevision) string               { return v.DaemonRevisionArn }
func daemonDeploymentsKeyFn(v *DaemonDeployment) string           { return v.DaemonDeploymentArn }

func servicesKeyFn(v *Service) string             { return scopedKey(clusterKey(v.ClusterArn), v.ServiceName) }
func servicesClusterIndexKeyFn(v *Service) string { return clusterKey(v.ClusterArn) }

func tasksKeyFn(v *Task) string             { return v.TaskArn }
func tasksClusterIndexKeyFn(v *Task) string { return clusterKey(v.ClusterArn) }

func containerInstancesKeyFn(v *ContainerInstance) string {
	return scopedKey(clusterKey(v.ClusterArn), v.ContainerInstanceArn)
}
func containerInstancesClusterIndexKeyFn(v *ContainerInstance) string {
	return clusterKey(v.ClusterArn)
}

func taskSetsKeyFn(v *TaskSet) string             { return scopedKey(v.ServiceArn, v.TaskSetArn) }
func taskSetsServiceIndexKeyFn(v *TaskSet) string { return v.ServiceArn }

func daemonsKeyFn(v *Daemon) string             { return scopedKey(clusterKey(v.ClusterArn), v.DaemonName) }
func daemonsClusterIndexKeyFn(v *Daemon) string { return clusterKey(v.ClusterArn) }

// taskDefByArnKeyFn and daemonTaskDefByArnKeyFn back the two ARN-lookup cache
// tables that are NOT registered on b.registry (see registerAllTables) because
// they are derived caches rebuilt from the raw taskDefinitions/
// daemonTaskDefinitions maps, not independently persisted state.
func taskDefByArnKeyFn(v *TaskDefinition) string             { return v.TaskDefinitionArn }
func daemonTaskDefByArnKeyFn(v *DaemonTaskDefinition) string { return v.DaemonTaskDefinitionArn }

// registerAllTables registers every converted resource map on b.registry
// exactly once, and constructs the two unregistered ARN-cache tables. It must
// be called during construction only (immediately after b.registry is created),
// never on every Reset() -- store.Register panics on a duplicate name, so
// runtime resets go through registry.ResetAll() (plus the two unregistered
// caches' own .Reset()) instead; see InMemoryBackend.Reset in store.go.
//
// Deliberately left as plain maps, not registered here:
//   - taskDefinitions, daemonTaskDefinitions, daemonTaskDefs, resourceTags:
//     slice-valued ([]*T/[]Tag), out of scope for store.Table's map[string]*V
//     shape. Task-def revision-slice ordering (latest = last element) is also
//     load-bearing in a way a primary-key-sorted rebuild wouldn't preserve.
//   - tasksByInstance: a bool-keyed reverse-index set, not a resource collection.
//   - serviceIndex: keyed by the svcRef struct, bool-valued.
//   - attributes: composite key needs the cluster, not stored on the value itself.
//   - lifecycle: value type carries no identity field; keyed externally by task ARN.
//   - serviceRevisions, serviceRevisionsByArn: intentionally never populated
//     (see InMemoryBackend.serviceRevisions in store.go -- derived on demand
//     instead), and serviceRevisions is additionally slice-valued.
func registerAllTables(b *InMemoryBackend) {
	b.clusters = store.Register(b.registry, "clusters", store.New(clustersKeyFn))
	b.capacityProviders = store.Register(b.registry, "capacityProviders", store.New(capacityProvidersKeyFn))
	b.accountSettings = store.Register(b.registry, "accountSettings", store.New(accountSettingsKeyFn))
	b.taskProtections = store.Register(b.registry, "taskProtections", store.New(taskProtectionsKeyFn))
	b.serviceDeployments = store.Register(b.registry, "serviceDeployments", store.New(serviceDeploymentsKeyFn))
	b.expressGatewayServices = store.Register(
		b.registry, "expressGatewayServices", store.New(expressGatewayServicesKeyFn),
	)
	b.daemonRevisions = store.Register(b.registry, "daemonRevisions", store.New(daemonRevisionsKeyFn))
	b.daemonDeployments = store.Register(b.registry, "daemonDeployments", store.New(daemonDeploymentsKeyFn))

	b.services = store.Register(b.registry, "services", store.New(servicesKeyFn))
	b.servicesByCluster = b.services.AddIndex("servicesByCluster", servicesClusterIndexKeyFn)

	b.tasks = store.Register(b.registry, "tasks", store.New(tasksKeyFn))
	b.tasksByCluster = b.tasks.AddIndex("tasksByCluster", tasksClusterIndexKeyFn)

	b.containerInstances = store.Register(b.registry, "containerInstances", store.New(containerInstancesKeyFn))
	b.containerInstancesByCluster = b.containerInstances.AddIndex(
		"containerInstancesByCluster", containerInstancesClusterIndexKeyFn,
	)

	b.taskSets = store.Register(b.registry, "taskSets", store.New(taskSetsKeyFn))
	b.taskSetsByService = b.taskSets.AddIndex("taskSetsByService", taskSetsServiceIndexKeyFn)

	b.daemons = store.Register(b.registry, "daemons", store.New(daemonsKeyFn))
	b.daemonsByCluster = b.daemons.AddIndex("daemonsByCluster", daemonsClusterIndexKeyFn)

	// Unregistered derived-cache tables: reset/rebuilt manually, never part of
	// the persisted "tables" blob (see Reset/Restore in store.go/persistence.go).
	b.taskDefByArn = store.New(taskDefByArnKeyFn)
	b.daemonTaskDefByArn = store.New(daemonTaskDefByArnKeyFn)
}

// A store.Index's Get returns a slice OWNED by the index -- it must not be
// mutated or ranged over while concurrently deleting from the backing Table
// (each Table.Delete calls idx.remove, which mutates that same backing slice) --
// so every helper below snapshots the group into a private copy first via
// append(nil, ...). All must be called with the write lock held.

// tasksInClusterLocked returns a snapshot copy of every task in clusterName.
func (b *InMemoryBackend) tasksInClusterLocked(clusterName string) []*Task {
	return append([]*Task(nil), b.tasksByCluster.Get(clusterName)...)
}

// servicesInClusterLocked returns a snapshot copy of every service in clusterName.
func (b *InMemoryBackend) servicesInClusterLocked(clusterName string) []*Service {
	return append([]*Service(nil), b.servicesByCluster.Get(clusterName)...)
}

// containerInstancesInClusterLocked returns a snapshot copy of every container
// instance in clusterName.
func (b *InMemoryBackend) containerInstancesInClusterLocked(clusterName string) []*ContainerInstance {
	return append([]*ContainerInstance(nil), b.containerInstancesByCluster.Get(clusterName)...)
}

// taskSetsForServiceLocked returns a snapshot copy of every task set belonging
// to serviceArn.
func (b *InMemoryBackend) taskSetsForServiceLocked(serviceArn string) []*TaskSet {
	return append([]*TaskSet(nil), b.taskSetsByService.Get(serviceArn)...)
}

// daemonsInClusterLocked returns a snapshot copy of every daemon in clusterName.
func (b *InMemoryBackend) daemonsInClusterLocked(clusterName string) []*Daemon {
	return append([]*Daemon(nil), b.daemonsByCluster.Get(clusterName)...)
}

// deleteServicesForClusterLocked removes every service belonging to clusterName.
func (b *InMemoryBackend) deleteServicesForClusterLocked(clusterName string) {
	for _, s := range b.servicesInClusterLocked(clusterName) {
		b.services.Delete(servicesKeyFn(s))
		b.deleteResourceTagsLocked(s.ServiceArn)
	}
}

// deleteTasksForClusterLocked removes every task belonging to clusterName.
func (b *InMemoryBackend) deleteTasksForClusterLocked(clusterName string) {
	for _, t := range b.tasksInClusterLocked(clusterName) {
		b.tasks.Delete(tasksKeyFn(t))
		b.deleteResourceTagsLocked(t.TaskArn)
	}
}

// deleteContainerInstancesForClusterLocked removes every container instance
// belonging to clusterName.
func (b *InMemoryBackend) deleteContainerInstancesForClusterLocked(clusterName string) {
	for _, ci := range b.containerInstancesInClusterLocked(clusterName) {
		b.containerInstances.Delete(containerInstancesKeyFn(ci))
		b.deleteResourceTagsLocked(ci.ContainerInstanceArn)
	}
}

// deleteTaskSetsForServiceLocked removes every task set belonging to serviceArn.
func (b *InMemoryBackend) deleteTaskSetsForServiceLocked(serviceArn string) {
	for _, ts := range b.taskSetsForServiceLocked(serviceArn) {
		b.taskSets.Delete(taskSetsKeyFn(ts))
		b.deleteResourceTagsLocked(ts.TaskSetArn)
	}
}
