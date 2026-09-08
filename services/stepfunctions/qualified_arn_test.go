package stepfunctions_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/stepfunctions"
)

// TestStartExecution_VersionQualifiedARN verifies that StartExecution
// accepts a version-qualified stateMachineArn (stateMachineArn:N) and runs
// the definition frozen at that version, not whatever the live state
// machine's definition currently is. AWS: "A state machine version ARN ...
// Step Functions doesn't associate executions that you start with a version
// ARN with any aliases that point to that version.".
func TestStartExecution_VersionQualifiedARN(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(), "ver-start-sm", minimalDefinition, validRoleARN, "STANDARD",
	)
	require.NoError(t, err)

	v1, err := b.PublishStateMachineVersion(sm.StateMachineArn, "v1", "")
	require.NoError(t, err)

	// Mutate the live definition after publishing v1 -- the version must
	// keep running its own frozen snapshot, not this new definition.
	newDef := `{"StartAt":"S2","States":{"S2":{"Type":"Pass","End":true}}}`
	_, _, err = b.UpdateStateMachine(sm.StateMachineArn, newDef, "")
	require.NoError(t, err)

	exec, err := b.StartExecution(v1.StateMachineVersionArn, "ver-exec", "{}")
	require.NoError(t, err)

	// Execution ARNs never carry a version/alias qualifier -- AWS builds
	// them from the base (unqualified) state machine ARN and name.
	assert.Equal(t, sm.StateMachineArn, exec.StateMachineArn)
	assert.Equal(t, v1.StateMachineVersionArn, exec.StateMachineVersionArn)
	assert.Empty(t, exec.StateMachineAliasArn)

	described, err := b.DescribeStateMachineForExecution(exec.ExecutionArn)
	require.NoError(t, err)
	assert.JSONEq(t, minimalDefinition, described.Definition, "must run v1's frozen definition, not the live one")
}

// TestStartExecution_AliasQualifiedARN_SingleVersion verifies StartExecution
// resolves an alias ARN with a single (100%-weighted) routing target.
func TestStartExecution_AliasQualifiedARN_SingleVersion(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(), "alias-start-sm", minimalDefinition, validRoleARN, "STANDARD",
	)
	require.NoError(t, err)

	v1, err := b.PublishStateMachineVersion(sm.StateMachineArn, "v1", "")
	require.NoError(t, err)

	alias, err := b.CreateStateMachineAlias(sm.StateMachineArn, "live", "", []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: v1.StateMachineVersionArn, Weight: 100},
	})
	require.NoError(t, err)

	exec, err := b.StartExecution(alias.StateMachineAliasArn, "alias-exec", "{}")
	require.NoError(t, err)

	assert.Equal(t, sm.StateMachineArn, exec.StateMachineArn)
	assert.Equal(t, v1.StateMachineVersionArn, exec.StateMachineVersionArn)
	assert.Equal(t, alias.StateMachineAliasArn, exec.StateMachineAliasArn)
}

// TestStartExecution_AliasQualifiedARN_WeightedRouting verifies that a
// 2-version weighted alias only ever routes to one of the two configured
// versions, and that a 0/100 split is deterministic (never picks the
// 0-weighted version).
func TestStartExecution_AliasQualifiedARN_WeightedRouting(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(), "weighted-alias-sm", minimalDefinition, validRoleARN, "STANDARD",
	)
	require.NoError(t, err)

	v1, err := b.PublishStateMachineVersion(sm.StateMachineArn, "v1", "")
	require.NoError(t, err)
	v2, err := b.PublishStateMachineVersion(sm.StateMachineArn, "v2", "")
	require.NoError(t, err)

	alias, err := b.CreateStateMachineAlias(sm.StateMachineArn, "canary", "", []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: v1.StateMachineVersionArn, Weight: 0},
		{StateMachineVersionArn: v2.StateMachineVersionArn, Weight: 100},
	})
	require.NoError(t, err)

	for i := range 20 {
		exec, startErr := b.StartExecution(
			alias.StateMachineAliasArn, "canary-exec-"+string(rune('a'+i)), "{}",
		)
		require.NoError(t, startErr)
		assert.Equal(t, v2.StateMachineVersionArn, exec.StateMachineVersionArn,
			"0-weighted version must never be selected")
	}
}

// TestStartExecution_QualifiedARN_NotFound verifies a version/alias-shaped
// ARN that doesn't match any published version or alias still surfaces
// StateMachineDoesNotExist, not a panic or a wrong-shaped error.
func TestStartExecution_QualifiedARN_NotFound(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	_, err := b.StartExecution(
		"arn:aws:states:us-east-1:123456789012:stateMachine:ghost:7", "x", "{}",
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, stepfunctions.ErrStateMachineDoesNotExist)
}

// TestStartSyncExecution_VersionQualifiedARN verifies StartSyncExecution
// (EXPRESS-only) also resolves version-qualified ARNs.
func TestStartSyncExecution_VersionQualifiedARN(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(), "ver-sync-sm", minimalDefinition, validRoleARN, "EXPRESS",
	)
	require.NoError(t, err)

	v1, err := b.PublishStateMachineVersion(sm.StateMachineArn, "v1", "")
	require.NoError(t, err)

	result, err := b.StartSyncExecution(v1.StateMachineVersionArn, "sync-exec", "{}")
	require.NoError(t, err)
	assert.Equal(t, sm.StateMachineArn, result.StateMachineArn)
	assert.Equal(t, "SUCCEEDED", result.Status)
}

// TestDescribeStateMachine_VersionQualifiedARN verifies DescribeStateMachine
// itself resolves version-qualified ARNs (the real AWS mechanism for
// fetching version details -- there is no separate DescribeStateMachineVersion
// operation in the actual AWS API).
func TestDescribeStateMachine_VersionQualifiedARN(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(), "describe-ver-sm", minimalDefinition, validRoleARN, "STANDARD",
	)
	require.NoError(t, err)

	newDef := `{"StartAt":"S2","States":{"S2":{"Type":"Pass","End":true}}}`
	_, _, err = b.UpdateStateMachine(sm.StateMachineArn, newDef, "")
	require.NoError(t, err)

	v, err := b.PublishStateMachineVersion(sm.StateMachineArn, "v1", "")
	require.NoError(t, err)

	described, err := b.DescribeStateMachine(v.StateMachineVersionArn)
	require.NoError(t, err)
	// Unlike execution start, Describe echoes the version ARN back as
	// StateMachineArn (AWS: "If you specified a state machine version ARN
	// in your request, the API returns the version ARN").
	assert.Equal(t, v.StateMachineVersionArn, described.StateMachineArn)
	assert.Equal(t, newDef, described.Definition)
	assert.Equal(t, sm.Name, described.Name)
}

// TestUpdateStateMachineAlias_EmptyRoutingLeavesConfigUnchanged verifies
// that passing an empty RoutingConfiguration to UpdateStateMachineAlias
// leaves the alias's existing (already-validated) routing untouched rather
// than clearing it -- see aliases.go's UpdateStateMachineAlias, which only
// calls validateRoutingConfig (and assigns) when len(routing) > 0. This is
// one of two ways gopherstack-t8iz asked to be ruled out as a path to an
// alias with an empty RoutingConfiguration (the other is persistence
// restore, covered by TestAliasRoutingConfiguration_NotPersistedAcrossRestore).
func TestUpdateStateMachineAlias_EmptyRoutingLeavesConfigUnchanged(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(), "empty-routing-update-sm", minimalDefinition, validRoleARN, "STANDARD",
	)
	require.NoError(t, err)

	v1, err := b.PublishStateMachineVersion(sm.StateMachineArn, "v1", "")
	require.NoError(t, err)

	alias, err := b.CreateStateMachineAlias(sm.StateMachineArn, "live", "", []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: v1.StateMachineVersionArn, Weight: 100},
	})
	require.NoError(t, err)

	updated, err := b.UpdateStateMachineAlias(
		alias.StateMachineAliasArn, "new-desc", []stepfunctions.AliasRoutingConfig{},
	)
	require.NoError(t, err)
	assert.Equal(t, alias.RoutingConfiguration, updated.RoutingConfiguration,
		"empty routing on update must leave the existing routing config unchanged, never clear it")
	assert.NotEmpty(t, updated.RoutingConfiguration)

	// Confirm resolveExecutionTarget (via StartExecution) still resolves
	// through the alias afterward -- the routing config was never cleared.
	exec, err := b.StartExecution(alias.StateMachineAliasArn, "post-update-exec", "{}")
	require.NoError(t, err)
	assert.Equal(t, v1.StateMachineVersionArn, exec.StateMachineVersionArn)
}

// TestAliasRoutingConfiguration_NotPersistedAcrossRestore verifies that
// Restore cannot reintroduce a StateMachineAlias at all, let alone one with
// an empty RoutingConfiguration: persistence.go's newPersistedDTORegistry
// only registers stateMachines, activities, and executions, so aliases are
// never part of backendSnapshot. This rules out the snapshot/restore path
// as a way to construct the alias pickRoutedVersion (qualified_arn.go)
// guards against -- see gopherstack-t8iz.
func TestAliasRoutingConfiguration_NotPersistedAcrossRestore(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	original := stepfunctions.NewInMemoryBackend()
	sm, err := original.CreateStateMachine(ctx, "restore-alias-sm", minimalDefinition, validRoleARN, "STANDARD")
	require.NoError(t, err)

	v1, err := original.PublishStateMachineVersion(sm.StateMachineArn, "v1", "")
	require.NoError(t, err)

	alias, err := original.CreateStateMachineAlias(sm.StateMachineArn, "live", "", []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: v1.StateMachineVersionArn, Weight: 100},
	})
	require.NoError(t, err)

	snap := original.Snapshot(ctx)
	require.NotNil(t, snap)

	fresh := stepfunctions.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(ctx, snap))

	_, err = fresh.DescribeStateMachineAlias(alias.StateMachineAliasArn)
	require.ErrorIs(t, err, stepfunctions.ErrStateMachineAliasDoesNotExist,
		"aliases are not part of backendSnapshot, so restore must not resurrect one -- "+
			"with or without routing config")
}

// TestUpdateStateMachine_RevisionIDChangesPerUpdate verifies that every
// UpdateStateMachine call produces a fresh opaque RevisionId, and that a
// freshly-created (never updated) state machine has none.
func TestUpdateStateMachine_RevisionIDChangesPerUpdate(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(), "revid-sm", minimalDefinition, validRoleARN, "STANDARD",
	)
	require.NoError(t, err)

	fresh, err := b.DescribeStateMachine(sm.StateMachineArn)
	require.NoError(t, err)
	assert.Empty(t, fresh.RevisionID, "a never-updated state machine should have no revisionId")

	_, rev1, err := b.UpdateStateMachine(sm.StateMachineArn, "", validRoleARN)
	require.NoError(t, err)
	assert.NotEmpty(t, rev1)

	_, rev2, err := b.UpdateStateMachine(sm.StateMachineArn, "", validRoleARN)
	require.NoError(t, err)
	assert.NotEmpty(t, rev2)
	assert.NotEqual(t, rev1, rev2, "each update must produce a new revisionId")

	described, err := b.DescribeStateMachine(sm.StateMachineArn)
	require.NoError(t, err)
	assert.Equal(t, rev2, described.RevisionID)
}
