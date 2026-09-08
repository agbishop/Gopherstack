package dynamodb_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/dynamodb"
)

func newFISDynamoDBHandler() *dynamodb.DynamoDBHandler {
	db := dynamodb.NewInMemoryDB()

	return dynamodb.NewHandler(db)
}

func TestDynamoDB_FISActions(t *testing.T) {
	t.Parallel()

	h := newFISDynamoDBHandler()
	actions := h.FISActions()

	ids := make([]string, len(actions))
	for i, a := range actions {
		ids[i] = a.ActionID
	}

	assert.Contains(t, ids, "aws:dynamodb:global-table-pause-replication")
}

func TestDynamoDB_FISActions_TargetType(t *testing.T) {
	t.Parallel()

	h := newFISDynamoDBHandler()

	actions := h.FISActions()
	require.Len(t, actions, 1)
	assert.Equal(t, "aws:dynamodb:global-table", actions[0].TargetType)
}

func TestDynamoDB_ExecuteFISAction_PauseReplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		targets  []string
		duration time.Duration
		wantErr  bool
	}{
		{
			name:    "single_table_no_duration",
			targets: []string{"arn:aws:dynamodb:us-east-1:000000000000:table/MyTable"},
			wantErr: false,
		},
		{
			name:     "single_table_with_duration",
			targets:  []string{"arn:aws:dynamodb:us-east-1:000000000000:table/TimedTable"},
			duration: 100 * time.Millisecond,
			wantErr:  false,
		},
		{
			name:    "no_targets",
			targets: []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := dynamodb.NewInMemoryDB()
			h := dynamodb.NewHandler(db)

			err := h.ExecuteFISAction(t.Context(), service.FISActionExecution{
				ActionID: "aws:dynamodb:global-table-pause-replication",
				Targets:  tt.targets,
				Duration: tt.duration,
			})

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Verify replication pause state is recorded.
			if len(tt.targets) > 0 {
				assert.True(t, db.IsReplicationPaused(tt.targets[0]),
					"replication should be marked as paused for target %s", tt.targets[0])
			}

			// Verify the pause clears after the duration.
			if tt.duration > 0 && len(tt.targets) > 0 {
				time.Sleep(tt.duration + 50*time.Millisecond)

				assert.False(t, db.IsReplicationPaused(tt.targets[0]),
					"replication pause should have expired after duration")
			}
		})
	}
}

func TestDynamoDB_ExecuteFISAction_Unknown(t *testing.T) {
	t.Parallel()

	h := newFISDynamoDBHandler()

	err := h.ExecuteFISAction(t.Context(), service.FISActionExecution{
		ActionID: "aws:dynamodb:unknown-action",
		Targets:  []string{"some-table"},
	})

	require.NoError(t, err)
}

func TestDynamoDB_ExecuteFISAction_PauseReplication_CtxCancel(t *testing.T) {
	t.Parallel()

	db := dynamodb.NewInMemoryDB()
	h := dynamodb.NewHandler(db)

	const tableARN = "arn:aws:dynamodb:us-east-1:000000000000:table/CancelTable"

	ctx, cancel := context.WithCancel(t.Context())

	// Activate indefinite pause (dur==0).
	err := h.ExecuteFISAction(ctx, service.FISActionExecution{
		ActionID: "aws:dynamodb:global-table-pause-replication",
		Targets:  []string{tableARN},
		Duration: 0,
	})
	require.NoError(t, err)

	assert.True(t, db.IsReplicationPaused(tableARN), "pause should be active")

	// Cancel ctx (simulates StopExperiment).
	cancel()

	require.Eventually(t, func() bool {
		return !db.IsReplicationPaused(tableARN)
	}, 2*time.Second, 20*time.Millisecond, "pause should clear after ctx cancel")
}

func TestDynamoDB_IsReplicationPaused_LazyEviction(t *testing.T) {
	t.Parallel()

	db := dynamodb.NewInMemoryDB()

	const tableARN = "arn:aws:dynamodb:us-east-1:000000000000:table/LazyTable"

	// Inject an already-expired entry directly (no goroutine, guaranteed expired).
	db.InjectExpiredReplicationPauseForTest(tableARN)

	assert.False(t, db.IsReplicationPaused(tableARN), "expired pause should not be reported active")

	// After lazy eviction the map should no longer contain the key.
	assert.False(
		t,
		db.IsReplicationPaused(tableARN),
		"second call should also return false (entry evicted)",
	)
}

func TestDynamoDB_IsReplicationPaused_ByNameSuffix(t *testing.T) {
	t.Parallel()

	db := dynamodb.NewInMemoryDB()
	h := dynamodb.NewHandler(db)

	const tableARN = "arn:aws:dynamodb:us-east-1:000000000000:table/SuffixTable"
	const tableName = "SuffixTable"

	err := h.ExecuteFISAction(t.Context(), service.FISActionExecution{
		ActionID: "aws:dynamodb:global-table-pause-replication",
		Targets:  []string{tableARN},
		Duration: 0,
	})
	require.NoError(t, err)

	// Should be accessible by ARN.
	assert.True(t, db.IsReplicationPaused(tableARN), "should be paused by ARN")

	// Reactivate to check by table name (suffix lookup).
	err = h.ExecuteFISAction(t.Context(), service.FISActionExecution{
		ActionID: "aws:dynamodb:global-table-pause-replication",
		Targets:  []string{tableARN},
		Duration: 0,
	})
	require.NoError(t, err)

	assert.True(t, db.IsReplicationPaused(tableName), "should be paused by table name suffix")
}

func TestDynamoDB_IsReplicationPaused_NotFound(t *testing.T) {
	t.Parallel()

	db := dynamodb.NewInMemoryDB()

	assert.False(
		t,
		db.IsReplicationPaused("nonexistent-table"),
		"unknown table should not be paused",
	)
}

func TestDynamoDB_ExecuteFISAction_NonInMemoryBackend(t *testing.T) {
	t.Parallel()

	// ExecuteFISAction with a non-InMemoryDB backend should return nil gracefully.
	h := dynamodb.NewHandler(nil)

	err := h.ExecuteFISAction(t.Context(), service.FISActionExecution{
		ActionID: "aws:dynamodb:global-table-pause-replication",
		Targets:  []string{"some-table"},
	})

	require.NoError(t, err)
}

func TestDynamoDB_ScheduleReplicationPauseCleanup_MissingEntry_Continue(t *testing.T) {
	t.Parallel()

	db := dynamodb.NewInMemoryDB()

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // already cancelled so cleanup fires synchronously

	// Call cleanup with a table that was never added to the map.
	// Should hit the !ok continue branch without panicking.
	db.ScheduleReplicationPauseCleanupForTest(ctx, []string{"never-added-table"}, time.Millisecond)
}

// TestDynamoDB_FISPauseReplication_BlocksCrossRegionWrite verifies that an
// active aws:dynamodb:global-table-pause-replication fault stops item writes
// from propagating to sibling regions of a global table -- mirroring
// TestGlobalTable_CrossRegionWritePropagation in global_tables_test.go, but
// with the fault active on the source table.
func TestDynamoDB_FISPauseReplication_BlocksCrossRegionWrite(t *testing.T) {
	t.Parallel()

	backend := dynamodb.NewInMemoryDB()
	handler := dynamodb.NewHandler(backend)

	code, _ := invokeOp(t, handler, "CreateGlobalTable", map[string]any{
		"GlobalTableName": "FISPausedTable",
		"ReplicationGroup": []map[string]any{
			{"RegionName": "us-east-1"},
			{"RegionName": "eu-west-1"},
		},
	})
	require.Equal(t, http.StatusOK, code)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	err := handler.ExecuteFISAction(ctx, service.FISActionExecution{
		ActionID: "aws:dynamodb:global-table-pause-replication",
		Targets:  []string{"FISPausedTable"},
	})
	require.NoError(t, err)
	require.True(t, backend.IsReplicationPaused("FISPausedTable"))

	putCode, _ := invokeOp(t, handler, "PutItem", map[string]any{
		"TableName": "FISPausedTable",
		"Item": map[string]any{
			"pk":   map[string]any{"S": "item-1"},
			"data": map[string]any{"S": "hello"},
		},
	})
	require.Equal(t, http.StatusOK, putCode)

	euTable, euOK := backend.GetTableInRegion("FISPausedTable", "eu-west-1")
	require.True(t, euOK, "eu-west-1 table should exist")
	assert.Empty(t, euTable.GetItems(), "write should not propagate while replication is paused")
}
