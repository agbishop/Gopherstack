package cloudformation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
)

// TestResourceCreator_Extra_SecretsManagerResourcePolicy verifies a resource policy is attached to
// a real secret and removed on delete.
func TestResourceCreator_Extra_SecretsManagerResourcePolicy(t *testing.T) {
	t.Parallel()

	backends := newDependentServiceBackends(t)
	rc := cloudformation.NewResourceCreator(backends)
	ctx := t.Context()

	secretPhys, err := rc.Create(ctx, "MySecret", "AWS::SecretsManager::Secret",
		map[string]any{"Name": "phase5-secret"}, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, secretPhys)

	policyPhys, err := rc.Create(ctx, "MyPolicy", "AWS::SecretsManager::ResourcePolicy",
		map[string]any{
			"SecretId":       secretPhys,
			"ResourcePolicy": `{"Version":"2012-10-17","Statement":[]}`,
		}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, secretPhys, policyPhys)

	require.NoError(t, rc.Delete(ctx, "AWS::SecretsManager::ResourcePolicy", policyPhys, nil))
}

// TestResourceCreator_SecretsManager_RotationSchedule_CreateDelete verifies
// that rotation schedule and secret target attachment return stable physical IDs.
func TestResourceCreator_SecretsManager_RotationSchedule_CreateDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		props        map[string]any
		name         string
		logicalID    string
		resourceType string
	}{
		{
			name:         "rotation_schedule",
			logicalID:    "MyRotation",
			resourceType: "AWS::SecretsManager::RotationSchedule",
			props: map[string]any{
				"SecretId":          "my-secret",
				"RotationLambdaARN": "arn:aws:lambda:us-east-1:000000000000:function:rotator",
			},
		},
		{
			name:         "secret_target_attachment",
			logicalID:    "MyAttachment",
			resourceType: "AWS::SecretsManager::SecretTargetAttachment",
			props: map[string]any{
				"SecretId":   "my-secret",
				"TargetId":   "db-1234",
				"TargetType": "AWS::RDS::DBInstance",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backends := newExtraServiceBackends(t)
			rc := cloudformation.NewResourceCreator(backends)

			// RotationSchedule now really calls RotateSecret, which requires the
			// referenced secret to exist and already have a current version.
			_, err := rc.Create(t.Context(), "MySecret", "AWS::SecretsManager::Secret",
				map[string]any{"Name": "my-secret", "SecretString": "s3cr3t"}, nil, nil)
			require.NoError(t, err)

			physID, err := rc.Create(t.Context(), tt.logicalID, tt.resourceType, tt.props, nil, nil)
			require.NoError(t, err)
			assert.NotEmpty(t, physID)

			err = rc.Delete(t.Context(), tt.resourceType, physID, nil)
			require.NoError(t, err)
		})
	}
}
