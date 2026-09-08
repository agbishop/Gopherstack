package cloudformation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
	secretsmanagerbackend "github.com/blackbirdworks/gopherstack/services/secretsmanager"
)

// TestCreateSecretsManagerRotationSchedule_ConfiguresRotation confirms that creating an
// AWS::SecretsManager::RotationSchedule resource actually applies the rotation config to the
// target secret (RotateSecretInput's real shape: secretsmanager@v1.44.4
// api_op_RotateSecret.go/types.RotationRulesType), instead of only computing a physical ID and
// silently discarding RotationLambdaARN/RotationRules.
func TestCreateSecretsManagerRotationSchedule_ConfiguresRotation(t *testing.T) {
	t.Parallel()

	backends := newServiceBackends()
	rc := cloudformation.NewResourceCreator(backends)

	_, err := backends.SecretsManager.Backend.CreateSecret(t.Context(), &secretsmanagerbackend.CreateSecretInput{
		Name:         "unit-cfn-rotated-secret",
		SecretString: "s3cr3t",
	})
	require.NoError(t, err)

	props := map[string]any{
		"SecretId":          "unit-cfn-rotated-secret",
		"RotationLambdaARN": "arn:aws:lambda:us-east-1:000000000000:function:unit-cfn-rotator",
		"RotationRules": map[string]any{
			"AutomaticallyAfterDays": float64(30),
		},
	}

	physID, err := rc.Create(t.Context(), "MyRotation", "AWS::SecretsManager::RotationSchedule", props, nil, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, physID)

	desc, err := backends.SecretsManager.Backend.DescribeSecret(
		t.Context(),
		&secretsmanagerbackend.DescribeSecretInput{SecretID: "unit-cfn-rotated-secret"},
	)
	require.NoError(t, err)

	assert.Equal(t, "arn:aws:lambda:us-east-1:000000000000:function:unit-cfn-rotator", desc.RotationLambdaARN)
	assert.True(t, desc.RotationEnabled)
	require.NotNil(t, desc.RotationRules)
	require.NotNil(t, desc.RotationRules.AutomaticallyAfterDays)
	assert.Equal(t, int64(30), *desc.RotationRules.AutomaticallyAfterDays)
}
