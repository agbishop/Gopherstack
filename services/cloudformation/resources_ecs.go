package cloudformation

import (
	"context"
	"fmt"
	"strings"

	ecsbackend "github.com/blackbirdworks/gopherstack/services/ecs"
)

// ecsServiceARNMinParts is the minimum number of slash-delimited path parts
// in a valid ECS service ARN: prefix, cluster name, service name.
const ecsServiceARNMinParts = 3

// ---- ECS ----

func (rc *ResourceCreator) createECSCluster(
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.ECS == nil {
		return logicalID + "-stub", nil
	}

	name := strProp(props, "ClusterName", params, physicalIDs)
	if name == "" {
		name = logicalID
	}

	cluster, err := rc.backends.ECS.Backend.CreateCluster(ecsbackend.CreateClusterInput{
		ClusterName: name,
	})
	if err != nil {
		return "", fmt.Errorf("create ECS cluster %s: %w", name, err)
	}

	return cluster.ClusterArn, nil
}

func (rc *ResourceCreator) deleteECSCluster(arn string) error {
	if rc.backends.ECS == nil {
		return nil
	}

	name := resourceNameFromARN(arn)

	_, err := rc.backends.ECS.Backend.DeleteCluster(name)

	return err
}

func (rc *ResourceCreator) createECSTaskDefinition(
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.ECS == nil {
		return logicalID + "-stub", nil
	}

	family := strProp(props, "Family", params, physicalIDs)
	if family == "" {
		family = logicalID
	}

	networkMode := strProp(props, "NetworkMode", params, physicalIDs)

	var containerDefs []ecsbackend.ContainerDefinition
	if list, ok := props["ContainerDefinitions"].([]any); ok {
		containerDefs = parseContainerDefinitions(list, params, physicalIDs)
	}

	td, err := rc.backends.ECS.Backend.RegisterTaskDefinition(ecsbackend.RegisterTaskDefinitionInput{
		Family:               family,
		NetworkMode:          networkMode,
		ContainerDefinitions: containerDefs,
	})
	if err != nil {
		return "", fmt.Errorf("register ECS task definition %s: %w", family, err)
	}

	return td.TaskDefinitionArn, nil
}

func (rc *ResourceCreator) deleteECSTaskDefinition(arn string) error {
	if rc.backends.ECS == nil {
		return nil
	}

	_, err := rc.backends.ECS.Backend.DeregisterTaskDefinition(arn)

	return err
}

// parseContainerDefinitions converts CloudFormation container definition property maps
// to ECS ContainerDefinition structs.
func parseContainerDefinitions(
	list []any,
	params, physicalIDs map[string]string,
) []ecsbackend.ContainerDefinition {
	defs := make([]ecsbackend.ContainerDefinition, 0, len(list))

	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		cd := ecsbackend.ContainerDefinition{
			Name:  resolve(m["Name"], params, physicalIDs),
			Image: resolve(m["Image"], params, physicalIDs),
		}

		if cpu, ok2 := m["Cpu"].(float64); ok2 {
			cd.CPU = int(cpu)
		}

		if mem, ok2 := m["Memory"].(float64); ok2 {
			cd.Memory = int(mem)
		}

		defs = append(defs, cd)
	}

	return defs
}

func (rc *ResourceCreator) createECSService(
	_ context.Context,
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.ECS == nil {
		return logicalID + "-stub", nil
	}

	serviceName := strProp(props, "ServiceName", params, physicalIDs)
	if serviceName == "" {
		serviceName = logicalID
	}

	cluster := strProp(props, "Cluster", params, physicalIDs)
	taskDef := strProp(props, "TaskDefinition", params, physicalIDs)
	launchType := strProp(props, "LaunchType", params, physicalIDs)

	var desiredCount int
	if v, ok := props["DesiredCount"].(float64); ok {
		desiredCount = int(v)
	}

	svc, err := rc.backends.ECS.Backend.CreateService(ecsbackend.CreateServiceInput{
		ServiceName:    serviceName,
		Cluster:        cluster,
		TaskDefinition: taskDef,
		LaunchType:     launchType,
		DesiredCount:   desiredCount,
	})
	if err != nil {
		return "", fmt.Errorf("create ECS service %s: %w", serviceName, err)
	}

	return svc.ServiceArn, nil
}

func (rc *ResourceCreator) deleteECSService(arn string) error {
	if rc.backends.ECS == nil {
		return nil
	}

	// ARN format: arn:aws:ecs:{region}:{account}:service/{cluster}/{service}
	// After splitting on "/" we need at least 3 parts: prefix, cluster name, service name.
	parts := strings.Split(arn, "/")
	if len(parts) < ecsServiceARNMinParts {
		return nil
	}

	// Last part is the service name, second-to-last is the cluster name.
	serviceName := parts[len(parts)-1]
	clusterName := parts[len(parts)-2]

	// Force: CloudFormation tears a service down without the caller scaling it
	// to zero first, matching real stack deletion.
	_, err := rc.backends.ECS.Backend.DeleteService(clusterName, serviceName, true)

	return err
}

// ---- ECR ----

func (rc *ResourceCreator) createECRRepository(
	ctx context.Context,
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.ECR == nil {
		return logicalID + "-stub", nil
	}

	name := strProp(props, "RepositoryName", params, physicalIDs)
	if name == "" {
		name = strings.ToLower(logicalID)
	}

	repo, err := rc.backends.ECR.Backend.CreateRepository(ctx, name, "", false, "", "")
	if err != nil {
		return "", fmt.Errorf("create ECR repository %s: %w", name, err)
	}

	return repo.RepositoryARN, nil
}

func (rc *ResourceCreator) deleteECRRepository(ctx context.Context, arn string, props map[string]any) error {
	if rc.backends.ECR == nil {
		return nil
	}

	name := resourceNameFromARN(arn)

	// EmptyOnDelete gates force per AWS::ECR::Repository's real property
	// (docs.aws.amazon.com/AWSCloudFormation/latest/TemplateReference/aws-resource-ecr-repository.html):
	// absent/false means the repository must be empty to delete.
	emptyOnDelete, _ := props["EmptyOnDelete"].(bool)

	_, err := rc.backends.ECR.Backend.DeleteRepository(ctx, name, emptyOnDelete)

	return err
}
