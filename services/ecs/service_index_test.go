package ecs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ecs"
)

// TestECSServiceIndexConsistency verifies that the serviceIndex flat map stays
// consistent with b.services across CreateService, DeleteService, and
// DeleteCluster, and that getServicesForReconciler returns exactly the indexed
// set on each tick.
func TestECSServiceIndexConsistency(t *testing.T) {
	t.Parallel()

	newBackend := func(t *testing.T) *ecs.InMemoryBackend {
		t.Helper()

		return ecs.NewInMemoryBackend("000000000000", "us-east-1", nil)
	}

	registerTD := func(t *testing.T, b *ecs.InMemoryBackend) string {
		t.Helper()
		td, err := b.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
			Family: "bench",
			ContainerDefinitions: []ecs.ContainerDefinition{
				{Name: "app", Image: "alpine"},
			},
		})
		require.NoError(t, err)

		return td.TaskDefinitionArn
	}

	tests := []struct {
		run  func(t *testing.T)
		name string
	}{
		{
			name: "created_service_included_in_reconciler_snapshot",
			run: func(t *testing.T) {
				t.Helper()

				b := newBackend(t)
				tdArn := registerTD(t, b)

				_, err := b.CreateService(ecs.CreateServiceInput{
					ServiceName:    "svc-a",
					TaskDefinition: tdArn,
					DesiredCount:   1,
				})
				require.NoError(t, err)

				snaps := b.GetServicesForReconcilerForTest()
				names := make(map[string]bool, len(snaps))
				for _, s := range snaps {
					names[s.ServiceName] = true
				}

				require.True(t, names["svc-a"], "created service missing from reconciler snapshot")
			},
		},
		{
			name: "deleted_service_excluded_from_reconciler_snapshot",
			run: func(t *testing.T) {
				t.Helper()

				b := newBackend(t)
				tdArn := registerTD(t, b)

				_, err := b.CreateService(ecs.CreateServiceInput{
					ServiceName:    "svc-b",
					TaskDefinition: tdArn,
					DesiredCount:   1,
				})
				require.NoError(t, err)

				_, err = b.DeleteService("", "svc-b", true)
				require.NoError(t, err)

				snaps := b.GetServicesForReconcilerForTest()
				for _, s := range snaps {
					require.NotEqual(t, "svc-b", s.ServiceName)
				}
			},
		},
		{
			name: "deleted_cluster_clears_all_services_from_index",
			run: func(t *testing.T) {
				t.Helper()

				b := newBackend(t)
				tdArn := registerTD(t, b)

				clusterName := "mycluster"
				_, err := b.CreateCluster(ecs.CreateClusterInput{ClusterName: clusterName})
				require.NoError(t, err)

				for _, name := range []string{"alpha", "beta", "gamma"} {
					_, err = b.CreateService(ecs.CreateServiceInput{
						ServiceName:    name,
						Cluster:        clusterName,
						TaskDefinition: tdArn,
						DesiredCount:   1,
					})
					require.NoError(t, err)
				}

				require.Len(t, b.GetServicesForReconcilerForTest(), 3)

				// DeleteCluster now refuses while the cluster still has
				// services, so delete each one first (force bypasses the
				// desiredCount guard -- irrelevant to what this test covers).
				for _, name := range []string{"alpha", "beta", "gamma"} {
					_, err = b.DeleteService(clusterName, name, true)
					require.NoError(t, err)
				}

				_, err = b.DeleteCluster(clusterName)
				require.NoError(t, err)

				snaps := b.GetServicesForReconcilerForTest()
				require.Empty(t, snaps, "index not cleared after service+cluster deletion")
			},
		},
		{
			name: "reset_clears_service_index",
			run: func(t *testing.T) {
				t.Helper()

				b := newBackend(t)
				tdArn := registerTD(t, b)

				_, err := b.CreateService(ecs.CreateServiceInput{
					ServiceName:    "svc-c",
					TaskDefinition: tdArn,
					DesiredCount:   1,
				})
				require.NoError(t, err)

				b.Reset()

				require.Empty(t, b.GetServicesForReconcilerForTest())
			},
		},
		{
			name: "multiple_clusters_indexed_correctly",
			run: func(t *testing.T) {
				t.Helper()

				b := newBackend(t)
				tdArn := registerTD(t, b)

				clusters := []string{"cluster-x", "cluster-y"}
				for _, cl := range clusters {
					_, err := b.CreateCluster(ecs.CreateClusterInput{ClusterName: cl})
					require.NoError(t, err)

					_, err = b.CreateService(ecs.CreateServiceInput{
						ServiceName:    "svc",
						Cluster:        cl,
						TaskDefinition: tdArn,
						DesiredCount:   1,
					})
					require.NoError(t, err)
				}

				snaps := b.GetServicesForReconcilerForTest()
				require.Len(t, snaps, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t)
		})
	}
}

// BenchmarkGetServicesForReconciler measures the reconciler snapshot path under
// the indexed implementation, confirming single-pass O(n) cost without the
// pre-counting nested-loop overhead.
func BenchmarkGetServicesForReconciler(b *testing.B) {
	backend := ecs.NewInMemoryBackend("000000000000", "us-east-1", nil)
	ctx := context.Background()
	_ = ctx

	td, err := backend.RegisterTaskDefinition(ecs.RegisterTaskDefinitionInput{
		Family:               "bench",
		ContainerDefinitions: []ecs.ContainerDefinition{{Name: "app", Image: "alpine"}},
	})
	if err != nil {
		b.Fatal(err)
	}

	const n = 500
	for i := range n {
		cluster := "cluster-" + string(rune('a'+i%5))
		// ignore cluster-already-exists errors
		_, _ = backend.CreateCluster(ecs.CreateClusterInput{ClusterName: cluster})

		_, err = backend.CreateService(ecs.CreateServiceInput{
			ServiceName:    "svc-bench-" + string(rune('a'+i/26%26)) + string(rune('a'+i%26)),
			Cluster:        cluster,
			TaskDefinition: td.TaskDefinitionArn,
			DesiredCount:   1,
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()

	for range b.N {
		snaps := backend.GetServicesForReconcilerForTest()
		if len(snaps) != n {
			b.Fatalf("got %d snapshots, want %d", len(snaps), n)
		}
	}
}
