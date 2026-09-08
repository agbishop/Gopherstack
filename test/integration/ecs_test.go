package integration_test

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ECS_CreateCluster(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	clusterName := "test-cluster-" + uuid.NewString()[:8]

	out, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})

	require.NoError(t, err)
	require.NotNil(t, out.Cluster)
	assert.Equal(t, clusterName, aws.ToString(out.Cluster.ClusterName))
	assert.NotEmpty(t, aws.ToString(out.Cluster.ClusterArn))
	assert.Equal(t, "ACTIVE", aws.ToString(out.Cluster.Status))
}

// TestIntegration_ECS_CreateCluster_Idempotent covers real ECS's documented
// behaviour: CreateCluster with an existing name returns the existing cluster,
// not an error. ClusterAlreadyExistsException is not a real ECS exception --
// it appears in no per-op deserializeOpError switch and has no shape in the
// pinned SDK's types/errors.go. It was removed as an invented code in
// fa0e68c21; this test asserted the invented behaviour until now.
func TestIntegration_ECS_CreateCluster_Idempotent(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	clusterName := "dupe-cluster-" + uuid.NewString()[:8]

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	again, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err, "real ECS CreateCluster is idempotent on an existing name")
	require.NotNil(t, again.Cluster)
	assert.Equal(t, clusterName, aws.ToString(again.Cluster.ClusterName),
		"the second call must return the existing cluster, not a new or empty one")
}

func TestIntegration_ECS_DescribeClusters(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	clusterName := "describe-cluster-" + uuid.NewString()[:8]

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	out, err := client.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: []string{clusterName},
	})
	require.NoError(t, err)
	require.Len(t, out.Clusters, 1)
	assert.Equal(t, clusterName, aws.ToString(out.Clusters[0].ClusterName))
}

func TestIntegration_ECS_DeleteCluster(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	clusterName := "delete-cluster-" + uuid.NewString()[:8]

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	out, err := client.DeleteCluster(ctx, &ecs.DeleteClusterInput{
		Cluster: aws.String(clusterName),
	})
	require.NoError(t, err)
	require.NotNil(t, out.Cluster)
	assert.Equal(t, clusterName, aws.ToString(out.Cluster.ClusterName))
}

func TestIntegration_ECS_RegisterTaskDefinition(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	family := "test-family-" + uuid.NewString()[:8]

	out, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{
				Name:      aws.String("nginx"),
				Image:     aws.String("nginx:latest"),
				Essential: aws.Bool(true),
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, out.TaskDefinition)
	assert.Equal(t, family, aws.ToString(out.TaskDefinition.Family))
	assert.NotEmpty(t, aws.ToString(out.TaskDefinition.TaskDefinitionArn))
	assert.Equal(t, int32(1), out.TaskDefinition.Revision)
	assert.Equal(t, ecstypes.TaskDefinitionStatusActive, out.TaskDefinition.Status)
}

func TestIntegration_ECS_RegisterTaskDefinition_MultipleRevisions(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	family := "multi-rev-" + uuid.NewString()[:8]

	for i := int32(1); i <= 3; i++ {
		out, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
			Family: aws.String(family),
			ContainerDefinitions: []ecstypes.ContainerDefinition{
				{Name: aws.String("app"), Image: aws.String("nginx:latest")},
			},
		})
		require.NoError(t, err)
		assert.Equal(t, i, out.TaskDefinition.Revision)
	}
}

func TestIntegration_ECS_DescribeTaskDefinition(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	family := "describe-td-" + uuid.NewString()[:8]

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	// Describe by family name.
	out, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: aws.String(family),
	})
	require.NoError(t, err)
	require.NotNil(t, out.TaskDefinition)
	assert.Equal(t, family, aws.ToString(out.TaskDefinition.Family))

	// Describe by ARN.
	out2, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
	})
	require.NoError(t, err)
	require.NotNil(t, out2.TaskDefinition)
}

func TestIntegration_ECS_ListTaskDefinitions(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	families := []string{
		"list-td-a-" + suffix,
		"list-td-b-" + suffix,
	}

	for _, f := range families {
		_, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
			Family: aws.String(f),
			ContainerDefinitions: []ecstypes.ContainerDefinition{
				{Name: aws.String("app"), Image: aws.String("nginx")},
			},
		})
		require.NoError(t, err)
	}

	out, err := client.ListTaskDefinitions(ctx, &ecs.ListTaskDefinitionsInput{
		FamilyPrefix: aws.String("list-td-"),
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(out.TaskDefinitionArns), 2)
}

func TestIntegration_ECS_CreateService(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "svc-cluster-" + suffix
	family := "svc-task-" + suffix
	serviceName := "my-service-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	out, err := client.CreateService(ctx, &ecs.CreateServiceInput{
		ServiceName:    aws.String(serviceName),
		Cluster:        aws.String(clusterName),
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
		DesiredCount:   aws.Int32(2),
	})

	require.NoError(t, err)
	require.NotNil(t, out.Service)
	assert.Equal(t, serviceName, aws.ToString(out.Service.ServiceName))
	assert.Equal(t, int32(2), out.Service.DesiredCount)
	assert.Equal(t, "ACTIVE", aws.ToString(out.Service.Status))
}

func TestIntegration_ECS_ListServiceDeployments(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "lsd-cluster-" + suffix
	family := "lsd-task-" + suffix
	serviceName := "lsd-service-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	_, err = client.CreateService(ctx, &ecs.CreateServiceInput{
		ServiceName:    aws.String(serviceName),
		Cluster:        aws.String(clusterName),
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
		DesiredCount:   aws.Int32(1),
	})
	require.NoError(t, err)

	// A real client decodes ListServiceDeploymentsOutput.ServiceDeployments
	// ([]types.ServiceDeploymentBrief) into typed struct fields, not the
	// wire's bare-string-array shape this op used to emit -- so a non-nil,
	// non-empty decode here proves the shape, not just the JSON keys.
	out, err := client.ListServiceDeployments(ctx, &ecs.ListServiceDeploymentsInput{
		Cluster: aws.String(clusterName),
		Service: aws.String(serviceName),
	})
	require.NoError(t, err)
	require.Len(t, out.ServiceDeployments, 1)

	brief := out.ServiceDeployments[0]
	assert.NotEmpty(t, aws.ToString(brief.ServiceDeploymentArn))
	assert.Contains(t, aws.ToString(brief.ClusterArn), clusterName)
	assert.Contains(t, aws.ToString(brief.ServiceArn), serviceName)
	assert.NotEmpty(t, brief.Status)
	assert.NotNil(t, brief.CreatedAt)
	assert.NotNil(t, brief.StartedAt)
}

func TestIntegration_ECS_DescribeServices(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "dsvc-cluster-" + suffix
	family := "dsvc-task-" + suffix
	serviceName := "describe-svc-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	_, err = client.CreateService(ctx, &ecs.CreateServiceInput{
		ServiceName:        aws.String(serviceName),
		Cluster:            aws.String(clusterName),
		TaskDefinition:     regOut.TaskDefinition.TaskDefinitionArn,
		DesiredCount:       aws.Int32(1),
		SchedulingStrategy: ecstypes.SchedulingStrategyReplica,
	})
	require.NoError(t, err)

	out, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(clusterName),
		Services: []string{serviceName},
	})
	require.NoError(t, err)
	require.Len(t, out.Services, 1)
	assert.Equal(t, serviceName, aws.ToString(out.Services[0].ServiceName))
	assert.Equal(t, ecstypes.SchedulingStrategyReplica, out.Services[0].SchedulingStrategy)
}

func TestIntegration_ECS_UpdateService(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "upd-cluster-" + suffix
	family := "upd-task-" + suffix
	serviceName := "update-svc-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	_, err = client.CreateService(ctx, &ecs.CreateServiceInput{
		ServiceName:    aws.String(serviceName),
		Cluster:        aws.String(clusterName),
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
		DesiredCount:   aws.Int32(1),
	})
	require.NoError(t, err)

	out, err := client.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:      aws.String(clusterName),
		Service:      aws.String(serviceName),
		DesiredCount: aws.Int32(5),
	})
	require.NoError(t, err)
	require.NotNil(t, out.Service)
	assert.Equal(t, int32(5), out.Service.DesiredCount)
}

func TestIntegration_ECS_DeleteService(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "del-svc-cluster-" + suffix
	family := "del-svc-task-" + suffix
	serviceName := "delete-svc-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	_, err = client.CreateService(ctx, &ecs.CreateServiceInput{
		ServiceName:    aws.String(serviceName),
		Cluster:        aws.String(clusterName),
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
		DesiredCount:   aws.Int32(1),
	})
	require.NoError(t, err)

	out, err := client.DeleteService(ctx, &ecs.DeleteServiceInput{
		Cluster: aws.String(clusterName),
		Service: aws.String(serviceName),
		Force:   aws.Bool(true),
	})
	require.NoError(t, err)
	require.NotNil(t, out.Service)
	assert.Equal(t, serviceName, aws.ToString(out.Service.ServiceName))
}

func TestIntegration_ECS_RunTask(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "run-cluster-" + suffix
	family := "run-task-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("nginx"), Image: aws.String("nginx:latest"), Essential: aws.Bool(true)},
		},
	})
	require.NoError(t, err)

	out, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
		Count:          aws.Int32(1),
	})

	require.NoError(t, err)
	require.Len(t, out.Tasks, 1)
	assert.NotEmpty(t, aws.ToString(out.Tasks[0].TaskArn))
	assert.Equal(t, "RUNNING", aws.ToString(out.Tasks[0].LastStatus))
}

func TestIntegration_ECS_RunTask_TransitionToRunning(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "trans-cluster-" + suffix
	family := "trans-task-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	runOut, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
		Count:          aws.Int32(1),
	})
	require.NoError(t, err)

	taskArn := aws.ToString(runOut.Tasks[0].TaskArn)

	// Task should be immediately in RUNNING state (no Docker runtime configured).
	descOut, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterName),
		Tasks:   []string{taskArn},
	})
	require.NoError(t, err)
	require.Len(t, descOut.Tasks, 1)
	assert.Equal(t, "RUNNING", aws.ToString(descOut.Tasks[0].LastStatus))
}

func TestIntegration_ECS_StopTask(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "stop-cluster-" + suffix
	family := "stop-task-family-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	runOut, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
		Count:          aws.Int32(1),
	})
	require.NoError(t, err)

	taskArn := aws.ToString(runOut.Tasks[0].TaskArn)

	stopOut, err := client.StopTask(ctx, &ecs.StopTaskInput{
		Cluster: aws.String(clusterName),
		Task:    aws.String(taskArn),
		Reason:  aws.String("test stop"),
	})
	require.NoError(t, err)
	require.NotNil(t, stopOut.Task)
	assert.Equal(t, "STOPPED", aws.ToString(stopOut.Task.LastStatus))
	assert.Equal(t, "test stop", aws.ToString(stopOut.Task.StoppedReason))
}

func TestIntegration_ECS_ListTasks(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "list-tasks-cluster-" + suffix
	family := "list-tasks-family-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	_, err = client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
		Count:          aws.Int32(3),
	})
	require.NoError(t, err)

	out, err := client.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster: aws.String(clusterName),
	})
	require.NoError(t, err)
	assert.Len(t, out.TaskArns, 3)
}

func TestIntegration_ECS_ListServices(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "list-svc-cluster-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	tdOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String("list-svc-td-" + suffix),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	// Create a FARGATE/REPLICA service.
	_, err = client.CreateService(ctx, &ecs.CreateServiceInput{
		Cluster:            aws.String(clusterName),
		ServiceName:        aws.String("svc-a-" + suffix),
		TaskDefinition:     tdOut.TaskDefinition.TaskDefinitionArn,
		DesiredCount:       aws.Int32(1),
		LaunchType:         ecstypes.LaunchTypeFargate,
		SchedulingStrategy: ecstypes.SchedulingStrategyReplica,
	})
	require.NoError(t, err)

	// Create an EC2/DAEMON service.
	_, err = client.CreateService(ctx, &ecs.CreateServiceInput{
		Cluster:            aws.String(clusterName),
		ServiceName:        aws.String("svc-b-" + suffix),
		TaskDefinition:     tdOut.TaskDefinition.TaskDefinitionArn,
		DesiredCount:       aws.Int32(0),
		LaunchType:         ecstypes.LaunchTypeEc2,
		SchedulingStrategy: ecstypes.SchedulingStrategyDaemon,
	})
	require.NoError(t, err)

	// No filter — all 2 services.
	out, err := client.ListServices(ctx, &ecs.ListServicesInput{
		Cluster: aws.String(clusterName),
	})
	require.NoError(t, err)
	assert.Len(t, out.ServiceArns, 2)

	// Filter by FARGATE — only 1 service.
	outFargate, err := client.ListServices(ctx, &ecs.ListServicesInput{
		Cluster:    aws.String(clusterName),
		LaunchType: ecstypes.LaunchTypeFargate,
	})
	require.NoError(t, err)
	assert.Len(t, outFargate.ServiceArns, 1)

	// Filter by DAEMON scheduling strategy — only 1 service.
	outDaemon, err := client.ListServices(ctx, &ecs.ListServicesInput{
		Cluster:            aws.String(clusterName),
		SchedulingStrategy: ecstypes.SchedulingStrategyDaemon,
	})
	require.NoError(t, err)
	assert.Len(t, outDaemon.ServiceArns, 1)
}

func TestIntegration_ECS_ContainerInstances(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "ci-cluster-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	// Register a container instance.
	regOut, err := client.RegisterContainerInstance(ctx, &ecs.RegisterContainerInstanceInput{
		Cluster: aws.String(clusterName),
	})
	require.NoError(t, err)
	require.NotNil(t, regOut.ContainerInstance)
	assert.NotEmpty(t, aws.ToString(regOut.ContainerInstance.ContainerInstanceArn))
	assert.Equal(t, "ACTIVE", aws.ToString(regOut.ContainerInstance.Status))
	assert.True(t, regOut.ContainerInstance.AgentConnected)

	ciArn := regOut.ContainerInstance.ContainerInstanceArn

	// List container instances.
	listOut, err := client.ListContainerInstances(ctx, &ecs.ListContainerInstancesInput{
		Cluster: aws.String(clusterName),
	})
	require.NoError(t, err)
	assert.Len(t, listOut.ContainerInstanceArns, 1)
	assert.Equal(t, aws.ToString(ciArn), listOut.ContainerInstanceArns[0])

	// Describe container instances.
	descOut, err := client.DescribeContainerInstances(ctx, &ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(clusterName),
		ContainerInstances: []string{aws.ToString(ciArn)},
	})
	require.NoError(t, err)
	require.Len(t, descOut.ContainerInstances, 1)
	assert.Equal(t, aws.ToString(ciArn), aws.ToString(descOut.ContainerInstances[0].ContainerInstanceArn))

	// Drain the instance.
	drainOut, err := client.UpdateContainerInstancesState(ctx, &ecs.UpdateContainerInstancesStateInput{
		Cluster:            aws.String(clusterName),
		ContainerInstances: []string{aws.ToString(ciArn)},
		Status:             ecstypes.ContainerInstanceStatusDraining,
	})
	require.NoError(t, err)
	require.Len(t, drainOut.ContainerInstances, 1)
	assert.Equal(t, "DRAINING", aws.ToString(drainOut.ContainerInstances[0].Status))

	// Deregister the instance.
	deregOut, err := client.DeregisterContainerInstance(ctx, &ecs.DeregisterContainerInstanceInput{
		Cluster:           aws.String(clusterName),
		ContainerInstance: ciArn,
		Force:             aws.Bool(true),
	})
	require.NoError(t, err)
	require.NotNil(t, deregOut.ContainerInstance)
	assert.Equal(t, "INACTIVE", aws.ToString(deregOut.ContainerInstance.Status))

	// Confirm it was removed.
	listOut2, err := client.ListContainerInstances(ctx, &ecs.ListContainerInstancesInput{
		Cluster: aws.String(clusterName),
	})
	require.NoError(t, err)
	assert.Empty(t, listOut2.ContainerInstanceArns)
}

func TestIntegration_ECS_TaskSets(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "ts-cluster-" + suffix
	serviceName := "ts-service-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	tdOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String("ts-td-" + suffix),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	_, err = client.CreateService(ctx, &ecs.CreateServiceInput{
		Cluster:        aws.String(clusterName),
		ServiceName:    aws.String(serviceName),
		TaskDefinition: tdOut.TaskDefinition.TaskDefinitionArn,
		DesiredCount:   aws.Int32(1),
	})
	require.NoError(t, err)

	// Create two task sets.
	ts1Out, err := client.CreateTaskSet(ctx, &ecs.CreateTaskSetInput{
		Cluster:        aws.String(clusterName),
		Service:        aws.String(serviceName),
		TaskDefinition: tdOut.TaskDefinition.TaskDefinitionArn,
	})
	require.NoError(t, err)
	require.NotNil(t, ts1Out.TaskSet)
	assert.NotEmpty(t, aws.ToString(ts1Out.TaskSet.TaskSetArn))
	assert.Equal(t, "ACTIVE", aws.ToString(ts1Out.TaskSet.Status))

	ts2Out, err := client.CreateTaskSet(ctx, &ecs.CreateTaskSetInput{
		Cluster:        aws.String(clusterName),
		Service:        aws.String(serviceName),
		TaskDefinition: tdOut.TaskDefinition.TaskDefinitionArn,
	})
	require.NoError(t, err)

	ts1Arn := ts1Out.TaskSet.TaskSetArn
	ts2Arn := ts2Out.TaskSet.TaskSetArn

	// Describe task sets.
	descOut, err := client.DescribeTaskSets(ctx, &ecs.DescribeTaskSetsInput{
		Cluster:  aws.String(clusterName),
		Service:  aws.String(serviceName),
		TaskSets: []string{aws.ToString(ts1Arn)},
	})
	require.NoError(t, err)
	require.Len(t, descOut.TaskSets, 1)
	assert.Equal(t, aws.ToString(ts1Arn), aws.ToString(descOut.TaskSets[0].TaskSetArn))

	// Update task set scale.
	updateOut, err := client.UpdateTaskSet(ctx, &ecs.UpdateTaskSetInput{
		Cluster: aws.String(clusterName),
		Service: aws.String(serviceName),
		TaskSet: ts1Arn,
		Scale: &ecstypes.Scale{
			Unit:  ecstypes.ScaleUnitPercent,
			Value: 25.0,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, updateOut.TaskSet)
	assert.InDelta(t, 25.0, updateOut.TaskSet.Scale.Value, 0.001)

	// Set primary task set.
	primaryOut, err := client.UpdateServicePrimaryTaskSet(ctx, &ecs.UpdateServicePrimaryTaskSetInput{
		Cluster:        aws.String(clusterName),
		Service:        aws.String(serviceName),
		PrimaryTaskSet: ts1Arn,
	})
	require.NoError(t, err)
	require.NotNil(t, primaryOut.TaskSet)
	assert.Equal(t, "PRIMARY", aws.ToString(primaryOut.TaskSet.Status))

	// Verify ts1 is PRIMARY.
	descOut2, err := client.DescribeTaskSets(ctx, &ecs.DescribeTaskSetsInput{
		Cluster:  aws.String(clusterName),
		Service:  aws.String(serviceName),
		TaskSets: []string{aws.ToString(ts1Arn)},
	})
	require.NoError(t, err)
	require.Len(t, descOut2.TaskSets, 1)
	assert.Equal(t, "PRIMARY", aws.ToString(descOut2.TaskSets[0].Status))

	// Delete task set 2.
	delOut, err := client.DeleteTaskSet(ctx, &ecs.DeleteTaskSetInput{
		Cluster: aws.String(clusterName),
		Service: aws.String(serviceName),
		TaskSet: ts2Arn,
	})
	require.NoError(t, err)
	require.NotNil(t, delOut.TaskSet)
}

func TestIntegration_ECS_ExecuteCommand(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "exec-cluster-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	tdOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String("exec-td-" + suffix),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest"), Essential: aws.Bool(true)},
		},
	})
	require.NoError(t, err)

	runOut, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: tdOut.TaskDefinition.TaskDefinitionArn,
		Count:          aws.Int32(1),
		// ECS Exec must be opted in at launch or ExecuteCommand is rejected (AWS behavior).
		EnableExecuteCommand: true,
	})
	require.NoError(t, err)
	require.Len(t, runOut.Tasks, 1)

	taskArn := runOut.Tasks[0].TaskArn

	// ExecuteCommand on running task.
	execOut, err := client.ExecuteCommand(ctx, &ecs.ExecuteCommandInput{
		Cluster:     aws.String(clusterName),
		Task:        taskArn,
		Container:   aws.String("app"),
		Command:     aws.String("/bin/sh"),
		Interactive: true,
	})
	require.NoError(t, err)
	require.NotNil(t, execOut.Session)
	assert.NotEmpty(t, aws.ToString(execOut.Session.SessionId))
	assert.NotEmpty(t, aws.ToString(execOut.Session.StreamUrl))
	assert.NotEmpty(t, aws.ToString(execOut.Session.TokenValue))
	assert.NotEmpty(t, aws.ToString(execOut.ClusterArn))
	assert.Equal(t, aws.ToString(taskArn), aws.ToString(execOut.TaskArn))
}

// TestIntegration_ECS_DockerRuntime tests ECS task execution when the Docker
// runtime is available. This test is skipped unless GOPHERSTACK_ECS_RUNTIME is
// set to "docker" on the test server and the local environment exposes a
// Docker-enabled Gopherstack endpoint via GOPHERSTACK_TEST_ECS_DOCKER_ENDPOINT.
//
// To run:
//
//	GOPHERSTACK_TEST_ECS_DOCKER_ENDPOINT=http://localhost:8000 \
//	 go test ./test/integration/... -run TestIntegration_ECS_DockerRuntime
func TestIntegration_ECS_DockerRuntime(t *testing.T) {
	t.Parallel()

	dockerEndpoint := os.Getenv("GOPHERSTACK_TEST_ECS_DOCKER_ENDPOINT")
	if dockerEndpoint == "" {
		t.Skip("skipping docker-runtime ECS test: GOPHERSTACK_TEST_ECS_DOCKER_ENDPOINT not set")
	}

	ctx := t.Context()
	suffix := uuid.NewString()[:8]

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("test", "test", ""),
		),
	)
	require.NoError(t, err)

	client := ecs.NewFromConfig(cfg, func(o *ecs.Options) {
		o.BaseEndpoint = &dockerEndpoint
	})

	// Create a cluster.
	clusterName := "docker-cluster-" + suffix
	_, err = client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	// Register a minimal nginx task definition.
	family := "docker-nginx-" + suffix
	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{
				Name:      aws.String("nginx"),
				Image:     aws.String("nginx:latest"),
				Essential: aws.Bool(true),
			},
		},
	})
	require.NoError(t, err)

	// Run the task via the Docker runtime; task should reach RUNNING.
	runOut, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
		Count:          aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, runOut.Tasks, 1)

	taskArn := aws.ToString(runOut.Tasks[0].TaskArn)

	// Verify the task transitioned from PROVISIONING to RUNNING.
	descOut, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterName),
		Tasks:   []string{taskArn},
	})
	require.NoError(t, err)
	require.Len(t, descOut.Tasks, 1)
	assert.Equal(t, "RUNNING", aws.ToString(descOut.Tasks[0].LastStatus),
		"task should be RUNNING after Docker container start")

	// Clean up: stop the task.
	_, err = client.StopTask(ctx, &ecs.StopTaskInput{
		Cluster: aws.String(clusterName),
		Task:    aws.String(taskArn),
		Reason:  aws.String("integration test cleanup"),
	})
	require.NoError(t, err)
}

// TestIntegration_ECS_UpdateCluster verifies UpdateCluster updates a cluster.
func TestIntegration_ECS_UpdateCluster(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	clusterName := "update-cluster-" + uuid.NewString()[:8]

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	out, err := client.UpdateCluster(ctx, &ecs.UpdateClusterInput{
		Cluster: aws.String(clusterName),
		Settings: []ecstypes.ClusterSetting{
			{Name: ecstypes.ClusterSettingNameContainerInsights, Value: aws.String("enabled")},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out.Cluster)
	assert.Equal(t, clusterName, aws.ToString(out.Cluster.ClusterName))
}

// TestIntegration_ECS_ListTaskDefinitionFamilies verifies listing families.
func TestIntegration_ECS_ListTaskDefinitionFamilies(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	family1 := "fam-a-" + suffix
	family2 := "fam-b-" + suffix

	for _, f := range []string{family1, family2} {
		_, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
			Family: aws.String(f),
			ContainerDefinitions: []ecstypes.ContainerDefinition{
				{Name: aws.String("c1"), Image: aws.String("nginx:latest")},
			},
		})
		require.NoError(t, err)
	}

	out, err := client.ListTaskDefinitionFamilies(ctx, &ecs.ListTaskDefinitionFamiliesInput{
		FamilyPrefix: aws.String("fam-"),
	})
	require.NoError(t, err)

	found := make(map[string]bool)
	for _, f := range out.Families {
		found[f] = true
	}

	assert.True(t, found[family1], "family1 should appear")
	assert.True(t, found[family2], "family2 should appear")
}

// TestIntegration_ECS_Tagging verifies TagResource, UntagResource, ListTagsForResource.
func TestIntegration_ECS_Tagging(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	clusterName := "tag-cluster-" + uuid.NewString()[:8]

	createOut, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	arn := aws.ToString(createOut.Cluster.ClusterArn)

	_, err = client.TagResource(ctx, &ecs.TagResourceInput{
		ResourceArn: aws.String(arn),
		Tags: []ecstypes.Tag{
			{Key: aws.String("env"), Value: aws.String("test")},
			{Key: aws.String("team"), Value: aws.String("platform")},
		},
	})
	require.NoError(t, err)

	listOut, err := client.ListTagsForResource(ctx, &ecs.ListTagsForResourceInput{
		ResourceArn: aws.String(arn),
	})
	require.NoError(t, err)
	assert.Len(t, listOut.Tags, 2)

	_, err = client.UntagResource(ctx, &ecs.UntagResourceInput{
		ResourceArn: aws.String(arn),
		TagKeys:     []string{"env"},
	})
	require.NoError(t, err)

	listOut2, err := client.ListTagsForResource(ctx, &ecs.ListTagsForResourceInput{
		ResourceArn: aws.String(arn),
	})
	require.NoError(t, err)
	assert.Len(t, listOut2.Tags, 1)
	assert.Equal(t, "team", aws.ToString(listOut2.Tags[0].Key))
}

// TestIntegration_ECS_StartTask verifies StartTask places a task on a container instance.
func TestIntegration_ECS_StartTask(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "start-task-cluster-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	family := "start-task-" + suffix
	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx:latest")},
		},
	})
	require.NoError(t, err)

	// Register a container instance first.
	ciOut, err := client.RegisterContainerInstance(ctx, &ecs.RegisterContainerInstanceInput{
		Cluster: aws.String(clusterName),
	})
	require.NoError(t, err)
	require.NotNil(t, ciOut.ContainerInstance)

	ciArn := aws.ToString(ciOut.ContainerInstance.ContainerInstanceArn)

	out, err := client.StartTask(ctx, &ecs.StartTaskInput{
		Cluster:            aws.String(clusterName),
		TaskDefinition:     regOut.TaskDefinition.TaskDefinitionArn,
		ContainerInstances: []string{ciArn},
	})
	require.NoError(t, err)
	assert.Len(t, out.Tasks, 1)
	assert.Equal(t, ciArn, aws.ToString(out.Tasks[0].ContainerInstanceArn))
}

// ----- Integration tests for new ECS operations (issue #1109) -----

func TestIntegration_ECS_AccountSettings(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	// PutAccountSetting
	putOut, err := client.PutAccountSetting(ctx, &ecs.PutAccountSettingInput{
		Name:  ecstypes.SettingNameContainerInsights,
		Value: aws.String("enabled"),
	})
	require.NoError(t, err)
	require.NotNil(t, putOut.Setting)
	assert.Equal(t, ecstypes.SettingNameContainerInsights, putOut.Setting.Name)
	assert.Equal(t, "enabled", aws.ToString(putOut.Setting.Value))

	// PutAccountSettingDefault
	defaultOut, err := client.PutAccountSettingDefault(ctx, &ecs.PutAccountSettingDefaultInput{
		Name:  ecstypes.SettingNameServiceLongArnFormat,
		Value: aws.String("enabled"),
	})
	require.NoError(t, err)
	require.NotNil(t, defaultOut.Setting)

	// ListAccountSettings
	listOut, err := client.ListAccountSettings(ctx, &ecs.ListAccountSettingsInput{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(listOut.Settings), 2)

	// DeleteAccountSetting
	_, err = client.DeleteAccountSetting(ctx, &ecs.DeleteAccountSettingInput{
		Name: ecstypes.SettingNameContainerInsights,
	})
	require.NoError(t, err)
}

func TestIntegration_ECS_Attributes(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	clusterName := "attr-cluster-" + uuid.NewString()[:8]

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	// PutAttributes
	putOut, err := client.PutAttributes(ctx, &ecs.PutAttributesInput{
		Cluster: aws.String(clusterName),
		Attributes: []ecstypes.Attribute{{
			Name:       aws.String("env"),
			Value:      aws.String("test"),
			TargetId:   aws.String("inst-1"),
			TargetType: ecstypes.TargetTypeContainerInstance,
		}},
	})
	require.NoError(t, err)
	assert.Len(t, putOut.Attributes, 1)

	// ListAttributes
	listOut, err := client.ListAttributes(ctx, &ecs.ListAttributesInput{
		Cluster:    aws.String(clusterName),
		TargetType: ecstypes.TargetTypeContainerInstance,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(listOut.Attributes), 1)

	// DeleteAttributes
	_, err = client.DeleteAttributes(ctx, &ecs.DeleteAttributesInput{
		Cluster: aws.String(clusterName),
		Attributes: []ecstypes.Attribute{{
			Name:       aws.String("env"),
			TargetId:   aws.String("inst-1"),
			TargetType: ecstypes.TargetTypeContainerInstance,
		}},
	})
	require.NoError(t, err)
}

func TestIntegration_ECS_ClusterCapacityProviders(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	clusterName := "cp-cluster-" + uuid.NewString()[:8]

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	// Create a capacity provider first.
	cpName := "cp-" + uuid.NewString()[:8]
	_, err = client.CreateCapacityProvider(ctx, &ecs.CreateCapacityProviderInput{
		Name: aws.String(cpName),
		AutoScalingGroupProvider: &ecstypes.AutoScalingGroupProvider{
			AutoScalingGroupArn: aws.String("arn:aws:autoscaling:us-east-1:000000000000:autoScalingGroup:fake"),
		},
	})
	require.NoError(t, err)

	// PutClusterCapacityProviders
	putOut, err := client.PutClusterCapacityProviders(ctx, &ecs.PutClusterCapacityProvidersInput{
		Cluster:           aws.String(clusterName),
		CapacityProviders: []string{cpName},
		DefaultCapacityProviderStrategy: []ecstypes.CapacityProviderStrategyItem{
			{CapacityProvider: aws.String(cpName), Weight: 1},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, putOut.Cluster)
	assert.Contains(t, putOut.Cluster.CapacityProviders, cpName)
}

func TestIntegration_ECS_UpdateClusterSettings(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	clusterName := "settings-cluster-" + uuid.NewString()[:8]

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	out, err := client.UpdateClusterSettings(ctx, &ecs.UpdateClusterSettingsInput{
		Cluster: aws.String(clusterName),
		Settings: []ecstypes.ClusterSetting{
			{Name: ecstypes.ClusterSettingNameContainerInsights, Value: aws.String("enabled")},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out.Cluster)
	require.Len(t, out.Cluster.Settings, 1)
	assert.Equal(t, "enabled", aws.ToString(out.Cluster.Settings[0].Value))
}

func TestIntegration_ECS_TaskProtection(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	clusterName := "tp-cluster-" + uuid.NewString()[:8]

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	_, err = client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String("tp-family"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("nginx")},
		},
	})
	require.NoError(t, err)

	runOut, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: aws.String("tp-family"),
		Count:          aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, runOut.Tasks, 1)

	taskArn := aws.ToString(runOut.Tasks[0].TaskArn)

	// UpdateTaskProtection
	upOut, err := client.UpdateTaskProtection(ctx, &ecs.UpdateTaskProtectionInput{
		Cluster:           aws.String(clusterName),
		Tasks:             []string{taskArn},
		ProtectionEnabled: true,
	})
	require.NoError(t, err)
	assert.Len(t, upOut.ProtectedTasks, 1)

	// GetTaskProtection
	getOut, err := client.GetTaskProtection(ctx, &ecs.GetTaskProtectionInput{
		Cluster: aws.String(clusterName),
		Tasks:   []string{taskArn},
	})
	require.NoError(t, err)
	assert.Len(t, getOut.ProtectedTasks, 1)
	assert.True(t, getOut.ProtectedTasks[0].ProtectionEnabled)
}

// TestIntegration_ECS_RunTask_ContainerNetworkBindings round-trips a task
// definition with a port mapping through RunTask and DescribeTasks, verifying
// the nested containers[].networkBindings shape (bindIP/containerPort/hostPort/
// protocol) that a list-only sweep would never exercise.
func TestIntegration_ECS_RunTask_ContainerNetworkBindings(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "netbind-cluster-" + suffix
	family := "netbind-task-" + suffix

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{
				Name:      aws.String("web"),
				Image:     aws.String("nginx:latest"),
				Essential: aws.Bool(true),
				PortMappings: []ecstypes.PortMapping{
					{
						ContainerPort: aws.Int32(8080),
						HostPort:      aws.Int32(30080),
						Protocol:      ecstypes.TransportProtocolTcp,
					},
				},
			},
		},
	})
	require.NoError(t, err)

	runOut, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
		Count:          aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, runOut.Tasks, 1)

	taskArn := aws.ToString(runOut.Tasks[0].TaskArn)

	descOut, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterName),
		Tasks:   []string{taskArn},
	})
	require.NoError(t, err)
	require.Len(t, descOut.Tasks, 1)
	require.Len(t, descOut.Tasks[0].Containers, 1)

	container := descOut.Tasks[0].Containers[0]
	assert.Equal(t, "web", aws.ToString(container.Name))
	assert.Equal(t, "RUNNING", aws.ToString(container.LastStatus))
	require.Len(t, container.NetworkBindings, 1)

	binding := container.NetworkBindings[0]
	assert.Equal(t, int32(8080), aws.ToInt32(binding.ContainerPort))
	assert.Equal(t, int32(30080), aws.ToInt32(binding.HostPort))
	assert.Equal(t, ecstypes.TransportProtocolTcp, binding.Protocol)
	assert.Equal(t, "0.0.0.0", aws.ToString(binding.BindIP))
}

// TestIntegration_ECS_Service_LoadBalancers_Deployments round-trips a service
// created with a load balancer target group through CreateService and
// DescribeServices, verifying the service-level loadBalancers list and the
// nested deployments[] entry AWS returns alongside it.
func TestIntegration_ECS_Service_LoadBalancers_Deployments(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createECSClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	clusterName := "lb-svc-cluster-" + suffix
	family := "lb-svc-task-" + suffix
	serviceName := "lb-svc-" + suffix
	targetGroupArn := "arn:aws:elasticloadbalancing:us-east-1:000000000000:targetgroup/tg-" + suffix + "/0123456789abcdef"

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	regOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String(family),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{
				Name:      aws.String("web"),
				Image:     aws.String("nginx:latest"),
				Essential: aws.Bool(true),
				PortMappings: []ecstypes.PortMapping{
					{ContainerPort: aws.Int32(80), Protocol: ecstypes.TransportProtocolTcp},
				},
			},
		},
	})
	require.NoError(t, err)

	_, err = client.CreateService(ctx, &ecs.CreateServiceInput{
		Cluster:        aws.String(clusterName),
		ServiceName:    aws.String(serviceName),
		TaskDefinition: regOut.TaskDefinition.TaskDefinitionArn,
		DesiredCount:   aws.Int32(1),
		LoadBalancers: []ecstypes.LoadBalancer{
			{
				TargetGroupArn: aws.String(targetGroupArn),
				ContainerName:  aws.String("web"),
				ContainerPort:  aws.Int32(80),
			},
		},
	})
	require.NoError(t, err)

	descOut, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(clusterName),
		Services: []string{serviceName},
	})
	require.NoError(t, err)
	require.Len(t, descOut.Services, 1)

	svc := descOut.Services[0]
	require.Len(t, svc.LoadBalancers, 1)
	assert.Equal(t, targetGroupArn, aws.ToString(svc.LoadBalancers[0].TargetGroupArn))
	assert.Equal(t, "web", aws.ToString(svc.LoadBalancers[0].ContainerName))
	assert.Equal(t, int32(80), aws.ToInt32(svc.LoadBalancers[0].ContainerPort))

	require.NotEmpty(t, svc.Deployments)
	assert.Equal(
		t,
		aws.ToString(regOut.TaskDefinition.TaskDefinitionArn),
		aws.ToString(svc.Deployments[0].TaskDefinition),
	)
	assert.Equal(t, int32(1), svc.Deployments[0].DesiredCount)
}
