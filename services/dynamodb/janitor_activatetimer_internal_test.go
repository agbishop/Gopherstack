package dynamodb

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sdkdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	sdktypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createDelayedTableInput(name string) *sdkdynamodb.CreateTableInput {
	rc, wc := int64(5), int64(5)

	return &sdkdynamodb.CreateTableInput{
		TableName: aws.String(name),
		KeySchema: []sdktypes.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: sdktypes.KeyTypeHash},
		},
		AttributeDefinitions: []sdktypes.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: sdktypes.ScalarAttributeTypeS},
		},
		ProvisionedThroughput: &sdktypes.ProvisionedThroughput{
			ReadCapacityUnits:  &rc,
			WriteCapacityUnits: &wc,
		},
	}
}

// TestJanitor_RunTableCleaner_CancelsLiveActivateTimer restores the coverage lost from
// TestDeleteWhileCreating in 51c94bac4: DeleteTable now rejects a CREATING table
// (ResourceInUseException) instead of queuing it, so DeleteTable itself can no longer hand
// runTableCleaner a table with a genuinely pending activateTimer -- that route is dead, and
// doubly so: DeleteTable already calls stopTableTimers itself (table_ops.go:487) before
// db.deletingTables.Put (table_ops.go:490), so runTableCleaner's own stopTableTimers call
// (janitor.go:198) is a belt-and-braces second stop, not the only one. No input can currently
// reach it with a live timer. This test is a regression guard for that belt-and-braces call in
// case the DeleteTable guard above is ever loosened or bypassed: it drives db.deletingTables the
// way DeleteTable used to, then calls the real runTableCleaner.
//
// victimTable is queued while CREATING; controlTable is left to activate normally. Both share
// the same create delay, so controlTable reaching ACTIVE proves virtual time genuinely advanced
// (guards against a vacuous pass), while victimTable staying CREATING proves activateTimer was
// actually cancelled, not just racing a slower assertion.
func TestJanitor_RunTableCleaner_CancelsLiveActivateTimer(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		db := NewInMemoryDB()
		db.SetCreateDelay(150 * time.Millisecond)
		ctx := t.Context()

		_, err := db.CreateTable(ctx, createDelayedTableInput("victim-table"))
		require.NoError(t, err)
		_, err = db.CreateTable(ctx, createDelayedTableInput("control-table"))
		require.NoError(t, err)

		victim, ok := db.GetTable("victim-table")
		require.True(t, ok)
		control, ok := db.GetTable("control-table")
		require.True(t, ok)

		victim.mu.RLock("test-precondition")
		victimStatus := victim.Status
		victim.mu.RUnlock()
		require.Equal(t, string(sdktypes.TableStatusCreating), victimStatus,
			"victim must still be CREATING when queued -- otherwise this isn't testing a live timer")

		// Mirror what DeleteTable used to do for a CREATING table before 51c94bac4 added its
		// guard: remove from db.tables, queue into deletingTables.
		db.mu.Lock("test-queue-for-deletion")
		db.tables.Delete(tableKey(db.defaultRegion, "victim-table"))
		db.deletingTables.Put(victim)
		db.mu.Unlock()

		j := NewJanitor(db, Settings{})
		j.runTableCleaner(ctx)

		// Advance past the original create delay -- if stopTableTimers failed to cancel
		// activateTimer, activateTableLocked fires here and flips Status on a table that's
		// already been torn down.
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()

		victim.mu.RLock("test-verify")
		victimStatus = victim.Status
		victim.mu.RUnlock()
		assert.Equal(t, string(sdktypes.TableStatusCreating), victimStatus,
			"stopTableTimers must cancel the activate timer before runTableCleaner discards the table")

		_, stillExists := db.GetTable("victim-table")
		assert.False(t, stillExists, "victim must not be resurrected in db.tables")

		control.mu.RLock("test-verify")
		controlStatus := control.Status
		control.mu.RUnlock()
		assert.Equal(t, string(sdktypes.TableStatusActive), controlStatus,
			"control table must reach ACTIVE normally, proving virtual time actually advanced")
	})
}

// TestCreateTable_DuplicateRejection_CancelsRejectedActivateTimer covers the genuinely-live site
// TestJanitor_RunTableCleaner_CancelsLiveActivateTimer above is not: CreateTable's duplicate-name
// rejection (table_ops.go:155-171). A second CreateTable for the same name while the first is
// still CREATING builds its own newTable and arms its own activateTimer *before* discovering the
// name is taken, then discards that table via stopTableTimers + mu.Close.
//
// The public API returns only an error for that path -- it hands back no pointer to the *Table it
// rejected, and that *Table is never reachable through db.tables (insertNewTableLocked returns
// false before ever calling db.tables.Put), so nothing external can ever observe it again. To
// assert on it at all, this rebuilds the rejected *Table using the same unexported building
// blocks CreateTable itself calls -- newTableFromCreateInput, then the same Status/activateTimer
// lines CreateTable inlines, then the real insertNewTableLocked and stopTableTimers -- in the same
// order table_ops.go uses them, rather than reimplementing their logic.
func TestCreateTable_DuplicateRejection_CancelsRejectedActivateTimer(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		db := NewInMemoryDB()
		db.SetCreateDelay(150 * time.Millisecond)
		ctx := t.Context()

		_, err := db.CreateTable(ctx, createDelayedTableInput("dup-table"))
		require.NoError(t, err)

		survivor, ok := db.GetTable("dup-table")
		require.True(t, ok)

		// table_ops.go:135-145: build the table and arm its activateTimer exactly as CreateTable
		// does when db.createDelay > 0.
		rejected := newTableFromCreateInput("dup-table", createDelayedTableInput("dup-table"))
		rejected.Status = string(sdktypes.TableStatusCreating)
		rejected.activateTimer = time.AfterFunc(db.createDelay, func() {
			activateTableLocked(rejected)
		})

		// table_ops.go:167-171: the duplicate-name rejection branch.
		inserted := db.insertNewTableLocked(db.defaultRegion, "dup-table", rejected)
		require.False(t, inserted, "insertNewTableLocked must report the name already taken")
		stopTableTimers(rejected)
		rejected.mu.Close()

		time.Sleep(200 * time.Millisecond)
		synctest.Wait()

		rejected.mu.RLock("test-verify")
		rejectedStatus := rejected.Status
		rejected.mu.RUnlock()
		assert.Equal(t, string(sdktypes.TableStatusCreating), rejectedStatus,
			"stopTableTimers must cancel the rejected duplicate's activate timer")

		survivor.mu.RLock("test-verify")
		survivorStatus := survivor.Status
		survivor.mu.RUnlock()
		assert.Equal(t, string(sdktypes.TableStatusActive), survivorStatus,
			"the surviving original table must still activate normally -- proves the right timer was cancelled")
	})
}

// TestDynamoDB_Reset_CancelsLiveActivateTimer covers the other genuinely-live site: Reset
// (store.go), reachable through the real POST /_gopherstack/reset endpoint. Unlike DeleteTable,
// Reset walks db.tables.All() with no status filter, so a table still CREATING when Reset runs
// has a genuinely live activateTimer at that point. TestDynamoDB_Reset_ClosesMutex (janitor_test.go)
// only asserts Reset doesn't panic; it never exercises a live timer (its tables are created with
// no create delay, so they're ACTIVE with no activateTimer at all) and never checks that
// cancellation actually suppressed the callback.
func TestDynamoDB_Reset_CancelsLiveActivateTimer(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		db := NewInMemoryDB()
		db.SetCreateDelay(150 * time.Millisecond)
		ctx := t.Context()

		_, err := db.CreateTable(ctx, createDelayedTableInput("reset-table"))
		require.NoError(t, err)

		table, ok := db.GetTable("reset-table")
		require.True(t, ok)

		table.mu.RLock("test-precondition")
		status := table.Status
		table.mu.RUnlock()
		require.Equal(t, string(sdktypes.TableStatusCreating), status,
			"table must still be CREATING when Reset runs -- otherwise this isn't testing a live timer")

		db.Reset()

		// Advance past the original create delay -- if Reset's stopTableTimers failed to cancel
		// activateTimer, activateTableLocked fires here on a table Reset already discarded.
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()

		table.mu.RLock("test-verify")
		status = table.Status
		table.mu.RUnlock()
		assert.Equal(t, string(sdktypes.TableStatusCreating), status,
			"Reset's stopTableTimers must cancel the activate timer before discarding the table")

		_, stillExists := db.GetTable("reset-table")
		assert.False(t, stillExists, "table must not be resurrected after Reset")
	})
}
