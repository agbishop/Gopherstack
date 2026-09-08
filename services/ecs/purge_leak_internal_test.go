package ecs

import (
	"testing"
	"time"
)

func TestReconcilerSemEviction_OnClusterDelete(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	r := NewReconciler(b)
	b.RegisterClusterDeleteHook(r.EvictCluster)

	// DeleteCluster now refuses (ClusterContains*Exception) while a cluster
	// still has services/tasks, so this test seeds the per-cluster semaphore
	// directly via clusterSem -- the same lazy-creation path RunOnce would
	// take when reconciling a service -- rather than routing through a real
	// service+task that would then block the deletions below.
	for _, cluster := range []string{"c1", "c2"} {
		if _, err := b.CreateCluster(CreateClusterInput{ClusterName: cluster}); err != nil {
			t.Fatalf("CreateCluster(%s): %v", cluster, err)
		}

		r.clusterSem(cluster)
	}

	if got := r.SemCount(); got != 2 {
		t.Fatalf("SemCount after seeding = %d, want 2", got)
	}

	if _, err := b.DeleteCluster("c1"); err != nil {
		t.Fatalf("DeleteCluster: %v", err)
	}
	if got := r.SemCount(); got != 1 {
		t.Errorf("SemCount after DeleteCluster = %d, want 1 (sem evicted)", got)
	}

	// Purge removes the remaining cluster and must also fire the eviction hook.
	b.Purge(t.Context(), time.Now().Add(time.Hour))

	if got := r.SemCount(); got != 0 {
		t.Errorf("SemCount after Purge = %d, want 0", got)
	}
}

func TestPurge_CleansServiceAndDaemonState(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	tdArn := registerSimpleTaskDef(t, b, "purge-app", "nginx")

	if _, err := b.CreateCluster(CreateClusterInput{ClusterName: "pc"}); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}

	svc, err := b.CreateService(CreateServiceInput{
		ServiceName:    "svc",
		Cluster:        "pc",
		TaskDefinition: tdArn,
		DesiredCount:   1,
	})
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	// Seed the auxiliary maps that older Purge code paths leaked.
	daemonArn := "arn:aws:ecs:us-east-1:123456789012:daemon/pc/d1"

	b.mu.Lock("seed")
	b.serviceDeployments.Put(&ServiceDeployment{
		ServiceDeploymentArn: "sd-1",
		ServiceArn:           svc.ServiceArn,
	})
	b.daemons.Put(&Daemon{DaemonArn: daemonArn, DaemonName: "d1", ClusterArn: svc.ClusterArn})
	b.daemonDeployments.Put(&DaemonDeployment{DaemonDeploymentArn: "dd-1", DaemonArn: daemonArn})
	b.daemonTaskDefs[daemonArn] = []*DaemonTaskDefinition{{DaemonTaskDefinitionArn: daemonArn}}
	b.mu.Unlock()

	b.Purge(t.Context(), time.Now().Add(time.Hour))

	if got := b.ServiceDeploymentCount(); got != 0 {
		t.Errorf("serviceDeployments after purge = %d, want 0", got)
	}

	b.mu.RLock("verify")
	defer b.mu.RUnlock()

	if got := b.daemons.Len(); got != 0 {
		t.Errorf("daemons after purge = %d, want 0", got)
	}
	if got := b.daemonDeployments.Len(); got != 0 {
		t.Errorf("daemonDeployments after purge = %d, want 0", got)
	}
	if len(b.daemonTaskDefs) != 0 {
		t.Errorf("daemonTaskDefs after purge = %d, want 0", len(b.daemonTaskDefs))
	}
	if got := b.clusters.Len(); got != 0 {
		t.Errorf("clusters after purge = %d, want 0", got)
	}
	if len(b.serviceIndex) != 0 {
		t.Errorf("serviceIndex after purge = %d, want 0", len(b.serviceIndex))
	}
}

func TestPurge_RevisionCutoff(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	// Register two families with two revisions each.
	for _, fam := range []string{"famA", "famB"} {
		registerSimpleTaskDef(t, b, fam, "img:1")
		registerSimpleTaskDef(t, b, fam, "img:2")
	}

	cutoff := time.Now()

	// Age the first revision of each family to before the cutoff; leave the
	// second revision newer than the cutoff.
	b.mu.Lock("age")
	for _, fam := range []string{"famA", "famB"} {
		b.taskDefinitions[fam][0].RegisteredAt = cutoff.Add(-time.Hour)
		b.taskDefinitions[fam][1].RegisteredAt = cutoff.Add(time.Hour)
	}
	b.mu.Unlock()

	b.Purge(t.Context(), cutoff)

	b.mu.RLock("verify")
	defer b.mu.RUnlock()

	// Each family should retain only its single newer-than-cutoff revision.
	for _, fam := range []string{"famA", "famB"} {
		if got := len(b.taskDefinitions[fam]); got != 1 {
			t.Errorf("family %s revisions after purge = %d, want 1", fam, got)
		}
	}

	// The aged revisions must also be gone from the ARN index.
	if got := b.taskDefByArn.Len(); got != 2 {
		t.Errorf("taskDefByArn size after purge = %d, want 2", got)
	}
}

// TestDeleteDaemon_CleansRevisionsAndDeployments proves that DeleteDaemon
// removes the daemonRevisions and daemonDeployments rows that
// CreateDaemon/UpdateDaemon created for it. Previously DeleteDaemon only
// removed the daemons table entry -- daemonRevisions and daemonDeployments
// were never touched, so every daemon revision and deployment ever created
// leaked forever, even after the owning daemon (and eventually its cluster)
// was deleted.
func TestDeleteDaemon_CleansRevisionsAndDeployments(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	if _, err := b.CreateCluster(CreateClusterInput{ClusterName: "dd-cluster"}); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}

	tdArn, err := b.RegisterDaemonTaskDefinition(RegisterDaemonTaskDefinitionInput{
		Family: "dd-family",
		ContainerDefinitions: []DaemonContainerDefinition{
			{Name: "agent", Image: "example/agent:latest", Essential: true},
		},
	})
	if err != nil {
		t.Fatalf("RegisterDaemonTaskDefinition: %v", err)
	}

	d, err := b.CreateDaemon(CreateDaemonInput{
		DaemonName:              "dd1",
		ClusterArn:              "dd-cluster",
		DaemonTaskDefinitionArn: tdArn.DaemonTaskDefinitionArn,
		CapacityProviderArns:    []string{"arn:aws:ecs:us-east-1:000000000000:capacity-provider/cp1"},
	})
	if err != nil {
		t.Fatalf("CreateDaemon: %v", err)
	}

	// A second UpdateDaemon call creates a second revision + deployment.
	_, err = b.UpdateDaemon(UpdateDaemonInput{
		DaemonArn:               d.DaemonArn,
		DaemonTaskDefinitionArn: tdArn.DaemonTaskDefinitionArn,
		CapacityProviderArns:    []string{"arn:aws:ecs:us-east-1:000000000000:capacity-provider/cp1"},
	})
	if err != nil {
		t.Fatalf("UpdateDaemon: %v", err)
	}

	b.mu.RLock("precheck")
	revCount := b.daemonRevisions.Len()
	depCount := b.daemonDeployments.Len()
	b.mu.RUnlock()

	if revCount == 0 || depCount == 0 {
		t.Fatalf("expected non-zero daemonRevisions/daemonDeployments before delete, got %d/%d", revCount, depCount)
	}

	if _, err = b.DeleteDaemon(d.DaemonArn); err != nil {
		t.Fatalf("DeleteDaemon: %v", err)
	}

	b.mu.RLock("postcheck")
	defer b.mu.RUnlock()

	if got := b.daemonRevisions.Len(); got != 0 {
		t.Errorf("daemonRevisions after DeleteDaemon = %d, want 0", got)
	}
	if got := b.daemonDeployments.Len(); got != 0 {
		t.Errorf("daemonDeployments after DeleteDaemon = %d, want 0", got)
	}
}

// TestPurgeCluster_CleansDaemonRevisionsAndDeployments proves that purging a
// cluster (which cascades to every daemon it owns) also removes their
// daemonRevisions rows. Previously the cleanup code deleted from
// daemonRevisions by DaemonArn, but that table is keyed by
// DaemonRevisionArn, so the delete never matched anything -- a real, silent,
// permanently-leaking bug preserved verbatim through a prior mechanical
// refactor (see the removed NOTE comment in purge.go).
func TestPurgeCluster_CleansDaemonRevisionsAndDeployments(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	if _, err := b.CreateCluster(CreateClusterInput{ClusterName: "pd-cluster"}); err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}

	tdArn, err := b.RegisterDaemonTaskDefinition(RegisterDaemonTaskDefinitionInput{
		Family: "pd-family",
		ContainerDefinitions: []DaemonContainerDefinition{
			{Name: "agent", Image: "example/agent:latest", Essential: true},
		},
	})
	if err != nil {
		t.Fatalf("RegisterDaemonTaskDefinition: %v", err)
	}

	if _, err = b.CreateDaemon(CreateDaemonInput{
		DaemonName:              "pd1",
		ClusterArn:              "pd-cluster",
		DaemonTaskDefinitionArn: tdArn.DaemonTaskDefinitionArn,
		CapacityProviderArns:    []string{"arn:aws:ecs:us-east-1:000000000000:capacity-provider/cp1"},
	}); err != nil {
		t.Fatalf("CreateDaemon: %v", err)
	}

	b.mu.RLock("precheck")
	revCount := b.daemonRevisions.Len()
	b.mu.RUnlock()

	if revCount == 0 {
		t.Fatalf("expected non-zero daemonRevisions before purge, got %d", revCount)
	}

	b.Purge(t.Context(), time.Now().Add(time.Hour))

	b.mu.RLock("postcheck")
	defer b.mu.RUnlock()

	if got := b.daemonRevisions.Len(); got != 0 {
		t.Errorf("daemonRevisions after Purge = %d, want 0", got)
	}
	if got := b.daemonDeployments.Len(); got != 0 {
		t.Errorf("daemonDeployments after Purge = %d, want 0", got)
	}
}

// TestDeleteResource_CleansGhostResourceTags proves deleting a cluster, service,
// container instance, task set, or express gateway service also removes its
// resourceTags side-map entry (see deleteResourceTagsLocked in tags.go).
func TestDeleteResource_CleansGhostResourceTags(t *testing.T) {
	t.Parallel()

	t.Run("cluster", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()

		cluster, err := b.CreateCluster(CreateClusterInput{
			ClusterName: "tag-ghost-cluster",
			Tags:        []Tag{{Key: "k", Value: "v"}},
		})
		if err != nil {
			t.Fatalf("CreateCluster: %v", err)
		}

		if _, err = b.DeleteCluster("tag-ghost-cluster"); err != nil {
			t.Fatalf("DeleteCluster: %v", err)
		}

		b.mu.RLock("check")
		_, ghost := b.resourceTags[cluster.ClusterArn]
		b.mu.RUnlock()

		if ghost {
			t.Errorf("resourceTags still has an entry for deleted cluster %s", cluster.ClusterArn)
		}
	})

	t.Run("service", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		tdArn := registerSimpleTaskDef(t, b, "tag-ghost-app", "nginx")

		if _, err := b.CreateCluster(CreateClusterInput{ClusterName: "tag-ghost-svc-cluster"}); err != nil {
			t.Fatalf("CreateCluster: %v", err)
		}

		svc, err := b.CreateService(CreateServiceInput{
			ServiceName:    "tag-ghost-svc",
			Cluster:        "tag-ghost-svc-cluster",
			TaskDefinition: tdArn,
			DesiredCount:   1,
		})
		if err != nil {
			t.Fatalf("CreateService: %v", err)
		}

		if err = b.TagResource(svc.ServiceArn, []Tag{{Key: "k", Value: "v"}}); err != nil {
			t.Fatalf("TagResource: %v", err)
		}

		if _, err = b.DeleteService("tag-ghost-svc-cluster", "tag-ghost-svc", true); err != nil {
			t.Fatalf("DeleteService: %v", err)
		}

		b.mu.RLock("check")
		_, ghost := b.resourceTags[svc.ServiceArn]
		b.mu.RUnlock()

		if ghost {
			t.Errorf("resourceTags still has an entry for deleted service %s", svc.ServiceArn)
		}
	})

	t.Run("task_via_cluster_delete", func(t *testing.T) {
		t.Parallel()

		b := newTestBackend()
		tdArn := registerSimpleTaskDef(t, b, "tag-ghost-task-app", "nginx")

		if _, err := b.CreateCluster(CreateClusterInput{ClusterName: "tag-ghost-task-cluster"}); err != nil {
			t.Fatalf("CreateCluster: %v", err)
		}

		tasks, err := b.RunTask(RunTaskInput{
			Cluster:        "tag-ghost-task-cluster",
			TaskDefinition: tdArn,
			Count:          1,
			Tags:           []Tag{{Key: "k", Value: "v"}},
		})
		if err != nil {
			t.Fatalf("RunTask: %v", err)
		}

		taskArn := tasks[0].TaskArn

		// DeleteCluster now refuses while the cluster has active tasks, so
		// stop the task first (matching what a real AWS caller must do).
		if _, err = b.StopTask("tag-ghost-task-cluster", taskArn, "test"); err != nil {
			t.Fatalf("StopTask: %v", err)
		}

		if _, err = b.DeleteCluster("tag-ghost-task-cluster"); err != nil {
			t.Fatalf("DeleteCluster: %v", err)
		}

		b.mu.RLock("check")
		_, ghost := b.resourceTags[taskArn]
		b.mu.RUnlock()

		if ghost {
			t.Errorf("resourceTags still has an entry for a task deleted via cluster delete %s", taskArn)
		}
	})
}
