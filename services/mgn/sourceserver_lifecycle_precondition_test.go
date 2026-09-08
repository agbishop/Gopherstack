package mgn_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	mgnsdk "github.com/aws/aws-sdk-go-v2/service/mgn"
	"github.com/aws/aws-sdk-go-v2/service/mgn/types"
	"github.com/stretchr/testify/require"
)

// waitForContinuousReplication polls DescribeSourceServers until serverID's
// DataReplicationState reaches CONTINUOUS -- the launchable precondition
// ChangeServerLifeCycleState enforces (api_op_ChangeServerLifeCycleState.go).
func waitForContinuousReplication(t *testing.T, client *mgnsdk.Client, serverID string) {
	t.Helper()

	require.Eventually(t, func() bool {
		out, err := client.DescribeSourceServers(t.Context(), &mgnsdk.DescribeSourceServersInput{
			Filters: &types.DescribeSourceServersRequestFilters{SourceServerIDs: []string{serverID}},
		})

		return err == nil && len(out.Items) == 1 &&
			out.Items[0].DataReplicationInfo != nil &&
			out.Items[0].DataReplicationInfo.DataReplicationState == types.DataReplicationStateContinuous
	}, defaultAsyncWait, defaultAsyncPoll, "source server never reached CONTINUOUS")
}

// TestMarkAsArchived_LifeCycleStatePrecondition proves MarkAsArchived enforces
// api_op_MarkAsArchived.go's documented precondition ("This command only
// works for SourceServers with a lifecycle. state which equals DISCONNECTED
// or CUTOVER"), which the backend did not check before the fix.
func TestMarkAsArchived_LifeCycleStatePrecondition(t *testing.T) {
	t.Parallel()

	t.Run("blocked lifecycle state rejected", func(t *testing.T) {
		t.Parallel()

		h, client := newTestHandlerAndClient(t)
		ctx := t.Context()

		seeded := seedSourceServerViaImport(t, h, client, "archive-blocked")
		serverID := aws.ToString(seeded.SourceServerID)

		// Fresh import leaves the server NOT_READY, not DISCONNECTED/CUTOVER.
		_, err := client.MarkAsArchived(ctx, &mgnsdk.MarkAsArchivedInput{SourceServerID: aws.String(serverID)})
		require.Error(t, err)

		var conflict *types.ConflictException
		require.ErrorAs(t, err, &conflict)
	})

	t.Run("cutover state allowed", func(t *testing.T) {
		t.Parallel()

		h, client := newTestHandlerAndClient(t)
		ctx := t.Context()

		seeded := seedSourceServerViaImport(t, h, client, "archive-cutover")
		serverID := aws.ToString(seeded.SourceServerID)

		// ChangeServerLifeCycleState now requires CONTINUOUS replication --
		// reach it legitimately before using this op as the shortcut to
		// CUTOVER; MarkAsArchived's own precondition is what this subtest
		// verifies, not whether ChangeServerLifeCycleState works pre-replication.
		waitForContinuousReplication(t, client, serverID)

		_, err := client.ChangeServerLifeCycleState(ctx, &mgnsdk.ChangeServerLifeCycleStateInput{
			SourceServerID: aws.String(serverID),
			LifeCycle: &types.ChangeServerLifeCycleStateSourceServerLifecycle{
				State: types.ChangeServerLifeCycleStateSourceServerLifecycleStateCutover,
			},
		})
		require.NoError(t, err)

		out, err := client.MarkAsArchived(ctx, &mgnsdk.MarkAsArchivedInput{SourceServerID: aws.String(serverID)})
		require.NoError(t, err)
		require.True(t, aws.ToBool(out.IsArchived))
	})

	t.Run("disconnected state allowed", func(t *testing.T) {
		t.Parallel()

		h, client := newTestHandlerAndClient(t)
		ctx := t.Context()

		seeded := seedSourceServerViaImport(t, h, client, "archive-disconnected")
		serverID := aws.ToString(seeded.SourceServerID)

		_, err := client.DisconnectFromService(ctx, &mgnsdk.DisconnectFromServiceInput{
			SourceServerID: aws.String(serverID),
		})
		require.NoError(t, err)

		out, err := client.MarkAsArchived(ctx, &mgnsdk.MarkAsArchivedInput{SourceServerID: aws.String(serverID)})
		require.NoError(t, err)
		require.True(t, aws.ToBool(out.IsArchived))
	})
}

// TestTerminateTargetInstances_LifeCycleStatePrecondition proves
// TerminateTargetInstances enforces api_op_TerminateTargetInstances.go's
// documented block list ("This command will not work for any Source Server
// with a lifecycle.state of TESTING, CUTTING_OVER, or CUTOVER"), which the
// backend did not check before the fix.
func TestTerminateTargetInstances_LifeCycleStatePrecondition(t *testing.T) {
	t.Parallel()

	t.Run("testing state rejected", func(t *testing.T) {
		t.Parallel()

		h, client := newTestHandlerAndClient(t)
		ctx := t.Context()

		seeded := seedSourceServerViaImport(t, h, client, "terminate-testing")
		serverID := aws.ToString(seeded.SourceServerID)

		// ChangeServerLifeCycleState now requires CONTINUOUS replication --
		// reach it legitimately before using this op as the shortcut to
		// READY_FOR_TEST; TerminateTargetInstances's own block list is what
		// this subtest verifies, not whether ChangeServerLifeCycleState works
		// pre-replication.
		waitForContinuousReplication(t, client, serverID)

		_, err := client.ChangeServerLifeCycleState(ctx, &mgnsdk.ChangeServerLifeCycleStateInput{
			SourceServerID: aws.String(serverID),
			LifeCycle: &types.ChangeServerLifeCycleStateSourceServerLifecycle{
				State: types.ChangeServerLifeCycleStateSourceServerLifecycleStateReadyForTest,
			},
		})
		require.NoError(t, err)

		// StartTest moves the server straight to TESTING (synchronous).
		_, err = client.StartTest(ctx, &mgnsdk.StartTestInput{SourceServerIDs: []string{serverID}})
		require.NoError(t, err)

		_, err = client.TerminateTargetInstances(ctx, &mgnsdk.TerminateTargetInstancesInput{
			SourceServerIDs: []string{serverID},
		})
		require.Error(t, err)

		var conflict *types.ConflictException
		require.ErrorAs(t, err, &conflict)
	})

	t.Run("cutting over state rejected", func(t *testing.T) {
		t.Parallel()

		h, client := newTestHandlerAndClient(t)
		ctx := t.Context()

		seeded := seedSourceServerViaImport(t, h, client, "terminate-cutting-over")
		serverID := aws.ToString(seeded.SourceServerID)

		// ChangeServerLifeCycleState now requires CONTINUOUS replication --
		// reach it legitimately before using this op as the shortcut to
		// READY_FOR_CUTOVER; TerminateTargetInstances's own block list is
		// what this subtest verifies, not whether ChangeServerLifeCycleState
		// works pre-replication.
		waitForContinuousReplication(t, client, serverID)

		_, err := client.ChangeServerLifeCycleState(ctx, &mgnsdk.ChangeServerLifeCycleStateInput{
			SourceServerID: aws.String(serverID),
			LifeCycle: &types.ChangeServerLifeCycleStateSourceServerLifecycle{
				State: types.ChangeServerLifeCycleStateSourceServerLifecycleStateReadyForCutover,
			},
		})
		require.NoError(t, err)

		// StartCutover moves the server straight to CUTTING_OVER (synchronous).
		_, err = client.StartCutover(ctx, &mgnsdk.StartCutoverInput{SourceServerIDs: []string{serverID}})
		require.NoError(t, err)

		_, err = client.TerminateTargetInstances(ctx, &mgnsdk.TerminateTargetInstancesInput{
			SourceServerIDs: []string{serverID},
		})
		require.Error(t, err)

		var conflict *types.ConflictException
		require.ErrorAs(t, err, &conflict)
	})

	t.Run("cutover state rejected", func(t *testing.T) {
		t.Parallel()

		h, client := newTestHandlerAndClient(t)
		ctx := t.Context()

		seeded := seedSourceServerViaImport(t, h, client, "terminate-cutover")
		serverID := aws.ToString(seeded.SourceServerID)

		// ChangeServerLifeCycleState now requires CONTINUOUS replication --
		// reach it legitimately before using this op as the shortcut to
		// CUTOVER; TerminateTargetInstances's own block list is what this
		// subtest verifies, not whether ChangeServerLifeCycleState works
		// pre-replication.
		waitForContinuousReplication(t, client, serverID)

		_, err := client.ChangeServerLifeCycleState(ctx, &mgnsdk.ChangeServerLifeCycleStateInput{
			SourceServerID: aws.String(serverID),
			LifeCycle: &types.ChangeServerLifeCycleStateSourceServerLifecycle{
				State: types.ChangeServerLifeCycleStateSourceServerLifecycleStateCutover,
			},
		})
		require.NoError(t, err)

		_, err = client.TerminateTargetInstances(ctx, &mgnsdk.TerminateTargetInstancesInput{
			SourceServerIDs: []string{serverID},
		})
		require.Error(t, err)

		var conflict *types.ConflictException
		require.ErrorAs(t, err, &conflict)
	})

	t.Run("non-blocked state allowed", func(t *testing.T) {
		t.Parallel()

		h, client := newTestHandlerAndClient(t)
		ctx := t.Context()

		seeded := seedSourceServerViaImport(t, h, client, "terminate-allowed")
		serverID := aws.ToString(seeded.SourceServerID)

		// Fresh import leaves the server NOT_READY -- not in the block list.
		_, err := client.TerminateTargetInstances(ctx, &mgnsdk.TerminateTargetInstancesInput{
			SourceServerIDs: []string{serverID},
		})
		require.NoError(t, err)
	})
}

// TestChangeServerLifeCycleState_LifeCycleStatePrecondition proves
// ChangeServerLifeCycleState enforces api_op_ChangeServerLifeCycleState.go's
// documented precondition ("This command only works if the Source Server is
// already launchable (dataReplicationInfo.lagDuration is not null)"), which
// the backend did not check before the fix.
func TestChangeServerLifeCycleState_LifeCycleStatePrecondition(t *testing.T) {
	t.Parallel()

	t.Run("not yet launchable rejected", func(t *testing.T) {
		t.Parallel()

		h, client := newTestHandlerAndClient(t)
		ctx := t.Context()

		seeded := seedSourceServerViaImport(t, h, client, "change-lifecycle-blocked")
		serverID := aws.ToString(seeded.SourceServerID)

		// Fresh import has not reached CONTINUOUS replication yet.
		_, err := client.ChangeServerLifeCycleState(ctx, &mgnsdk.ChangeServerLifeCycleStateInput{
			SourceServerID: aws.String(serverID),
			LifeCycle: &types.ChangeServerLifeCycleStateSourceServerLifecycle{
				State: types.ChangeServerLifeCycleStateSourceServerLifecycleStateReadyForCutover,
			},
		})
		require.Error(t, err)

		var conflict *types.ConflictException
		require.ErrorAs(t, err, &conflict)
	})

	t.Run("launchable allowed", func(t *testing.T) {
		t.Parallel()

		h, client := newTestHandlerAndClient(t)
		ctx := t.Context()

		seeded := seedSourceServerViaImport(t, h, client, "change-lifecycle-allowed")
		serverID := aws.ToString(seeded.SourceServerID)

		waitForContinuousReplication(t, client, serverID)

		out, err := client.ChangeServerLifeCycleState(ctx, &mgnsdk.ChangeServerLifeCycleStateInput{
			SourceServerID: aws.String(serverID),
			LifeCycle: &types.ChangeServerLifeCycleStateSourceServerLifecycle{
				State: types.ChangeServerLifeCycleStateSourceServerLifecycleStateReadyForCutover,
			},
		})
		require.NoError(t, err)
		require.NotNil(t, out.LifeCycle)
		require.Equal(t, types.LifeCycleStateReadyForCutover, out.LifeCycle.State)
	})
}
