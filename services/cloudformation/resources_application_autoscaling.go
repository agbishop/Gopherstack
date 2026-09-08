package cloudformation

import (
	"fmt"

	appautoscalingbackend "github.com/blackbirdworks/gopherstack/services/applicationautoscaling"
)

func (rc *ResourceCreator) createAppAutoScalingResource(
	logicalID, resourceType string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, bool, error) {
	switch resourceType {
	case "AWS::ApplicationAutoScaling::ScalableTarget":
		id, err := rc.createAppAutoScalingScalableTarget(logicalID, props, params, physicalIDs)

		return id, true, err
	case "AWS::ApplicationAutoScaling::ScalingPolicy":
		id, err := rc.createAppAutoScalingScalingPolicy(logicalID, props, params, physicalIDs)

		return id, true, err
	default:
		return "", false, nil
	}
}

func (rc *ResourceCreator) createAppAutoScalingScalableTarget(
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.AppAutoScaling == nil {
		return logicalID + "-stub", nil
	}

	serviceNamespace := strProp(props, "ServiceNamespace", params, physicalIDs)
	resourceID := strProp(props, "ResourceId", params, physicalIDs)
	scalableDimension := strProp(props, "ScalableDimension", params, physicalIDs)

	if serviceNamespace == "" {
		serviceNamespace = "ecs"
	}
	if resourceID == "" {
		resourceID = "service/" + logicalID + "/default"
	}
	if scalableDimension == "" {
		scalableDimension = "ecs:service:DesiredCount"
	}

	var minCap, maxCap int32 = 1, 10
	if v, ok := props["MinCapacity"].(float64); ok {
		minCap = int32(v)
	}
	if v, ok := props["MaxCapacity"].(float64); ok {
		maxCap = int32(v)
	}

	roleARN := strProp(props, "RoleARN", params, physicalIDs)

	target, err := rc.backends.AppAutoScaling.Backend.RegisterScalableTarget(
		serviceNamespace, resourceID, scalableDimension, &minCap, &maxCap, nil, roleARN, nil,
	)
	if err != nil {
		return "", fmt.Errorf("register scalable target %s: %w", resourceID, err)
	}

	return target.ARN, nil
}

func (rc *ResourceCreator) deleteAppAutoScalingScalableTarget(arn string) error {
	if rc.backends.AppAutoScaling == nil {
		return nil
	}

	// Physical ID is the ARN; parse serviceNamespace/resourceID/scalableDimension from it.
	// Format: arn:aws:application-autoscaling:<region>:<account>:scalable-target/<uuid>
	// We stored it by ARN index — use DeregisterScalableTarget with ARN lookup.
	// The backend's DeregisterScalableTarget takes (serviceNamespace, resourceID, scalableDimension).
	// We store the ARN as physical ID; find via DescribeScalableTargets with empty filter.
	targets, _, _ := rc.backends.AppAutoScaling.Backend.DescribeScalableTargets(
		appautoscalingbackend.DescribeScalableTargetsFilter{},
	)
	for _, t := range targets {
		if t.ARN == arn {
			return rc.backends.AppAutoScaling.Backend.DeregisterScalableTarget(
				t.ServiceNamespace, t.ResourceID, t.ScalableDimension,
			)
		}
	}

	return nil
}

func (rc *ResourceCreator) createAppAutoScalingScalingPolicy(
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.AppAutoScaling == nil {
		return logicalID + "-stub", nil
	}

	policyName := strProp(props, "PolicyName", params, physicalIDs)
	if policyName == "" {
		policyName = logicalID
	}

	serviceNamespace := strProp(props, "ServiceNamespace", params, physicalIDs)
	resourceID := strProp(props, "ResourceId", params, physicalIDs)
	scalableDimension := strProp(props, "ScalableDimension", params, physicalIDs)
	policyType := strProp(props, "PolicyType", params, physicalIDs)

	if serviceNamespace == "" {
		serviceNamespace = "ecs"
	}
	if resourceID == "" {
		resourceID = "service/" + logicalID + "/default"
	}
	if scalableDimension == "" {
		scalableDimension = "ecs:service:DesiredCount"
	}

	policy, err := rc.backends.AppAutoScaling.Backend.PutScalingPolicy(
		serviceNamespace, resourceID, scalableDimension, policyName, policyType, nil, nil, nil,
	)
	if err != nil {
		return "", fmt.Errorf("put scaling policy %s: %w", policyName, err)
	}

	return policy.ARN, nil
}

func (rc *ResourceCreator) deleteAppAutoScalingScalingPolicy(policyARN string) error {
	if rc.backends.AppAutoScaling == nil {
		return nil
	}

	policies, _, _ := rc.backends.AppAutoScaling.Backend.DescribeScalingPolicies(
		appautoscalingbackend.DescribeScalingPoliciesFilter{},
	)
	for _, p := range policies {
		if p.ARN == policyARN {
			return rc.backends.AppAutoScaling.Backend.DeleteScalingPolicy(
				p.ServiceNamespace, p.ResourceID, p.ScalableDimension, p.PolicyName,
			)
		}
	}

	return nil
}

// deleteAppAutoScalingResource handles deletion for Application Auto Scaling resource types.
func (rc *ResourceCreator) deleteAppAutoScalingResource(resourceType, physicalID string) (bool, error) {
	switch resourceType {
	case "AWS::ApplicationAutoScaling::ScalableTarget":
		return true, rc.deleteAppAutoScalingScalableTarget(physicalID)
	case "AWS::ApplicationAutoScaling::ScalingPolicy":
		return true, rc.deleteAppAutoScalingScalingPolicy(physicalID)
	}

	return false, nil
}
