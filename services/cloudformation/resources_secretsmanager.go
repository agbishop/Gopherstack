package cloudformation

import (
	"context"
	"fmt"

	secretsmanagerbackend "github.com/blackbirdworks/gopherstack/services/secretsmanager"
)

func (rc *ResourceCreator) createSecretsManagerSupplementalResource(
	ctx context.Context,
	logicalID, resourceType string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, bool, error) {
	switch resourceType {
	case "AWS::SecretsManager::RotationSchedule":
		id, err := rc.createSecretsManagerRotationSchedule(ctx, logicalID, props, params, physicalIDs)

		return id, true, err
	case "AWS::SecretsManager::SecretTargetAttachment":
		id := rc.createSecretsManagerSecretTargetAttachment(logicalID, props, params, physicalIDs)

		return id, true, nil
	default:
		return "", false, nil
	}
}

// createSecretsManagerRotationSchedule wires the CFN resource to the real RotateSecret
// operation (secretsmanager@v1.44.4 api_op_RotateSecret.go's RotateSecretInput /
// types.RotationRulesType) instead of only computing a physical ID: the prior version never
// called into the SecretsManager backend at all, so RotationLambdaARN/RotationRules were
// accepted and silently discarded — the secret's rotation configuration was never actually set.
func (rc *ResourceCreator) createSecretsManagerRotationSchedule(
	ctx context.Context,
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	secretID := strProp(props, "SecretId", params, physicalIDs)
	if secretID == "" {
		secretID = logicalID
	}

	if rc.backends.SecretsManager == nil {
		return secretID + "-rotation", nil
	}

	rotateImmediately := true
	if v, ok := props["RotateImmediatelyOnUpdate"].(bool); ok {
		rotateImmediately = v
	}

	input := &secretsmanagerbackend.RotateSecretInput{
		SecretID:          secretID,
		RotationLambdaARN: strProp(props, "RotationLambdaARN", params, physicalIDs),
		RotationRules:     parseSecretsManagerRotationRules(props, params, physicalIDs),
		RotateImmediately: &rotateImmediately,
	}

	if _, err := rc.backends.SecretsManager.Backend.RotateSecret(ctx, input); err != nil {
		return "", fmt.Errorf("configure Secrets Manager rotation for %s: %w", secretID, err)
	}

	// Physical ID is the secret ID — the rotation is configured on the secret itself.
	return secretID + "-rotation", nil
}

func parseSecretsManagerRotationRules(
	props map[string]any,
	params, physicalIDs map[string]string,
) *secretsmanagerbackend.RotationRulesType {
	rules, ok := props["RotationRules"].(map[string]any)
	if !ok {
		return nil
	}

	out := &secretsmanagerbackend.RotationRulesType{
		Duration:           strProp(rules, "Duration", params, physicalIDs),
		ScheduleExpression: strProp(rules, "ScheduleExpression", params, physicalIDs),
	}
	if days, hasDays := rules["AutomaticallyAfterDays"].(float64); hasDays {
		days64 := int64(days)
		out.AutomaticallyAfterDays = &days64
	}

	return out
}

func (rc *ResourceCreator) createSecretsManagerSecretTargetAttachment(
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) string {
	secretID := strProp(props, "SecretId", params, physicalIDs)
	targetID := strProp(props, "TargetId", params, physicalIDs)
	targetType := strProp(props, "TargetType", params, physicalIDs)

	// Physical ID encodes the attachment; no real backend operation needed.
	id := secretID + ":attachment:" + targetType + ":" + targetID
	if id == ":attachment::" {
		id = logicalID + "-attachment"
	}

	return id
}

// deleteSecretsManagerSupplementalResource handles deletion for SecretsManager supplemental
// resource types that have no real backend operation to reverse.
func (rc *ResourceCreator) deleteSecretsManagerSupplementalResource(resourceType, _ string) bool {
	switch resourceType {
	case "AWS::SecretsManager::RotationSchedule", "AWS::SecretsManager::SecretTargetAttachment":
		return true
	}

	return false
}

func (rc *ResourceCreator) createSecretsManagerResourcePolicy(
	ctx context.Context,
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.SecretsManager == nil {
		return logicalID + "-stub", nil
	}

	secretID := strProp(props, "SecretId", params, physicalIDs)
	policy := strProp(props, "ResourcePolicy", params, physicalIDs)

	if _, err := rc.backends.SecretsManager.Backend.PutResourcePolicy(
		ctx,
		&secretsmanagerbackend.PutResourcePolicyInput{
			SecretID:       secretID,
			ResourcePolicy: policy,
		},
	); err != nil {
		return "", fmt.Errorf("create Secrets Manager resource policy for %s: %w", secretID, err)
	}

	return secretID, nil
}

func (rc *ResourceCreator) deleteSecretsManagerResourcePolicy(ctx context.Context, secretID string) error {
	if rc.backends.SecretsManager == nil {
		return nil
	}

	_, err := rc.backends.SecretsManager.Backend.DeleteResourcePolicy(
		ctx,
		&secretsmanagerbackend.DeleteResourcePolicyInput{
			SecretID: secretID,
		},
	)

	return err
}
