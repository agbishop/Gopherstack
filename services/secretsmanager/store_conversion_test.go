package secretsmanager //nolint:testpackage // needs the unexported regionContextKey to exercise multi-region isolation.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullStateSnapshotRestore is the Phase 3.3 full-state round-trip test:
// it exercises every field the store.Table conversion touches -- secret
// material (SecretString AND SecretBinary across two regions sharing the
// SAME secret name, proving the region-qualified composite key survives a
// round trip), multiple staged versions, tags, rotation configuration, and
// the two raw (non-store.Table) maps resourcePolicies and
// replicationConfigs -- to guard against a silent persistence regression.
func TestFullStateSnapshotRestore(t *testing.T) {
	t.Parallel()

	original := NewInMemoryBackendWithConfig("000000000000", "us-east-1")

	ctxEast := smCtxRegion("us-east-1")
	ctxWest := smCtxRegion("us-west-2")

	// Two secrets sharing the SAME name in two different regions: only a
	// region-qualified key (not a bare name) can keep these distinct across
	// a snapshot/restore round trip.
	_, err := original.CreateSecret(ctxEast, &CreateSecretInput{
		Name:         "shared-name",
		Description:  "east secret",
		SecretString: "east-string-value",
		Tags:         []Tag{{Key: "env", Value: "east"}},
	})
	require.NoError(t, err)

	_, err = original.CreateSecret(ctxWest, &CreateSecretInput{
		Name:         "shared-name",
		Description:  "west secret",
		SecretBinary: []byte("west-binary-value"),
	})
	require.NoError(t, err)

	// A second version on the east secret (moves the first version to
	// AWSPREVIOUS).
	_, err = original.PutSecretValue(ctxEast, &PutSecretValueInput{
		SecretID:     "shared-name",
		SecretString: "east-string-value-v2",
	})
	require.NoError(t, err)

	// A resource policy (raw nested map, left unconverted) on the east secret only.
	_, err = original.PutResourcePolicy(ctxEast, &PutResourcePolicyInput{
		SecretID:       "shared-name",
		ResourcePolicy: `{"Version":"2012-10-17","Statement":[]}`,
	})
	require.NoError(t, err)

	// Replication config (raw nested map, left unconverted) on the east secret.
	_, err = original.ReplicateSecretToRegions(ctxEast, &ReplicateSecretToRegionsInput{
		SecretID:          "shared-name",
		AddReplicaRegions: []ReplicaRegion{{Region: "eu-west-1"}},
	})
	require.NoError(t, err)

	// Rotation configuration, to prove RotationRules/RotationEnabled survive.
	// RotateImmediately=false configures the schedule without rotating the
	// value out from under the assertions below (immediate rotation with no
	// LambdaInvoker wired would replace SecretString with a random UUID and
	// add a third version -- see RotateSecret/rotateSecretLocked).
	afterDays := int64(30)
	rotateImmediately := false
	_, err = original.RotateSecret(ctxEast, &RotateSecretInput{
		SecretID:          "shared-name",
		RotationLambdaARN: "arn:aws:lambda:us-east-1:000000000000:function:rotate",
		RotationRules:     &RotationRulesType{AutomaticallyAfterDays: &afterDays},
		RotateImmediately: &rotateImmediately,
	})
	require.NoError(t, err)

	snap := original.Snapshot(context.Background())
	require.NotNil(t, snap)

	fresh := NewInMemoryBackendWithConfig("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(context.Background(), snap))

	// East secret: current string material, description, rotation, and
	// replication config all survive.
	eastVal, err := fresh.GetSecretValue(ctxEast, &GetSecretValueInput{SecretID: "shared-name"})
	require.NoError(t, err)
	assert.Equal(t, "east-string-value-v2", eastVal.SecretString)

	eastDesc, err := fresh.DescribeSecret(ctxEast, &DescribeSecretInput{SecretID: "shared-name"})
	require.NoError(t, err)
	assert.Equal(t, "east secret", eastDesc.Description)
	assert.True(t, eastDesc.RotationEnabled)
	require.NotNil(t, eastDesc.RotationRules)
	require.NotNil(t, eastDesc.RotationRules.AutomaticallyAfterDays)
	assert.Equal(t, int64(30), *eastDesc.RotationRules.AutomaticallyAfterDays)
	require.Len(t, eastDesc.ReplicationStatus, 1)
	assert.Equal(t, "eu-west-1", eastDesc.ReplicationStatus[0].Region)

	eastPolicy, err := fresh.GetResourcePolicy(ctxEast, &GetResourcePolicyInput{SecretID: "shared-name"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"Version":"2012-10-17","Statement":[]}`, eastPolicy.ResourcePolicy)

	// West secret: binary material survives, and remains isolated from the
	// east secret sharing the same name (region-qualified key proof).
	westVal, err := fresh.GetSecretValue(ctxWest, &GetSecretValueInput{SecretID: "shared-name"})
	require.NoError(t, err)
	assert.Equal(t, []byte("west-binary-value"), westVal.SecretBinary)
	assert.Empty(t, westVal.SecretString)

	westPolicy, err := fresh.GetResourcePolicy(ctxWest, &GetResourcePolicyInput{SecretID: "shared-name"})
	require.NoError(t, err)
	assert.Empty(t, westPolicy.ResourcePolicy, "resource policy must stay scoped to the east region")

	// Both versions of the east secret (AWSPREVIOUS holding the original
	// string value) survived the round trip too.
	versions, err := fresh.ListSecretVersionIDs(ctxEast, &ListSecretVersionIDsInput{SecretID: "shared-name"})
	require.NoError(t, err)
	assert.Len(t, versions.Versions, 2)

	// Overall counts: two secrets created directly (same name, different
	// regions) plus the real mirrored replica in eu-west-1 (see
	// upsertReplicaSecretLocked -- ReplicateSecretToRegions creates an
	// independently readable replica Secret, not just status bookkeeping).
	assert.Equal(t, 3, SecretCount(fresh))

	replicaVal, err := fresh.GetSecretValue(smCtxRegion("eu-west-1"), &GetSecretValueInput{SecretID: "shared-name"})
	require.NoError(t, err, "the replica secret must survive the snapshot/restore round trip")
	assert.Equal(t, "east-string-value-v2", replicaVal.SecretString)
}
