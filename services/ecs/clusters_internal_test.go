package ecs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClusterKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{
			input: "my-cluster",
			want:  "my-cluster",
		},
		{
			input: "arn:aws:ecs:us-east-1:000000000000:cluster/my-cluster",
			want:  "my-cluster",
		},
		{
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, clusterKey(tt.input))
		})
	}
}

// TestDeleteService_CleansServiceDeployments verifies ServiceDeployment
// records are cleaned up on DeleteService (not DeleteCluster, which now
// refuses to delete a cluster with services still in it -- see
// clusterDependencyViolationLocked) and that the now-empty cluster deletes
// cleanly afterward.
func TestDeleteService_CleansServiceDeployments(t *testing.T) {
	t.Parallel()
	b := newTestBackend()

	_, err := b.CreateCluster(CreateClusterInput{ClusterName: "test-cluster"})
	if err != nil {
		t.Fatalf("CreateCluster: %v", err)
	}

	_, err = b.RegisterTaskDefinition(RegisterTaskDefinitionInput{
		Family:               "myapp",
		ContainerDefinitions: []ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	if err != nil {
		t.Fatalf("register task def: %v", err)
	}

	svc, err := b.CreateService(CreateServiceInput{
		Cluster:        "test-cluster",
		ServiceName:    "my-svc",
		TaskDefinition: "myapp",
		DesiredCount:   0,
	})
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	// CreateService itself records a real ServiceDeployment for the initial
	// PRIMARY deployment (see syncServiceDeploymentsLocked in
	// service_deployments.go), keyed by ServiceDeploymentArn — not by
	// ServiceArn. Confirm it exists before asserting the cascade delete.
	deployments, err := b.ListServiceDeployments("test-cluster", "my-svc")
	if err != nil {
		t.Fatalf("ListServiceDeployments: %v", err)
	}

	if len(deployments) != 1 {
		t.Fatalf("ListServiceDeployments before delete = %d entries, want 1", len(deployments))
	}

	// Also inject a second, independently-keyed entry (as an external caller
	// or an older revision might leave behind) to prove the cascade matches on
	// the ServiceArn field rather than assuming a single well-known key.
	extraArn := "arn:aws:ecs:us-east-1:123456789012:service-deployment/test-cluster/my-svc/abc"
	b.mu.Lock("test-inject")
	b.serviceDeployments.Put(&ServiceDeployment{
		ServiceDeploymentArn: extraArn,
		ServiceArn:           svc.ServiceArn,
	})
	b.mu.Unlock()

	_, err = b.DeleteService("test-cluster", "my-svc")
	if err != nil {
		t.Fatalf("DeleteService: %v", err)
	}

	// Both the real and the injected service deployment should be gone.
	b.mu.RLock("test-verify")
	_, realStillExists := b.serviceDeployments.Get(deployments[0].ServiceDeploymentArn)
	_, extraStillExists := b.serviceDeployments.Get(extraArn)
	b.mu.RUnlock()

	if realStillExists {
		t.Error("real service deployment not deleted with service")
	}

	if extraStillExists {
		t.Error("injected service deployment not deleted with service")
	}

	// The cluster is now empty of services/tasks/container instances, so
	// deletion succeeds.
	if _, err = b.DeleteCluster("test-cluster"); err != nil {
		t.Fatalf("DeleteCluster on empty cluster: %v", err)
	}
}
