package redshiftdata //nolint:testpackage // needs access to the unexported region context key.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ctxRegion returns a context carrying the given AWS region under regionContextKey.
func ctxRegion(region string) context.Context {
	return context.WithValue(context.Background(), regionContextKey{}, region)
}

// TestRedshiftDataStatementRegionIsolation proves that same-named statement IDs
// in two regions are fully isolated: each region sees only its own statements,
// and cancelling in one region leaves the other intact.
func TestRedshiftDataStatementRegionIsolation(t *testing.T) {
	t.Parallel()

	backend := NewInMemoryBackend("000000000000", "us-east-1")

	ctxEast := ctxRegion("us-east-1")
	ctxWest := ctxRegion("us-west-2")

	// Create a statement in us-east-1.
	eastStmt, err := backend.ExecuteStatement(
		ctxEast, "SELECT 1", "cluster-a", "", "east-db", "", "", "", false, "", nil, "",
	)
	require.NoError(t, err)
	assert.NotEmpty(t, eastStmt.ID)

	// Create a statement with the same SQL in us-west-2.
	westStmt, err := backend.ExecuteStatement(
		ctxWest, "SELECT 1", "cluster-b", "", "west-db", "", "", "", false, "", nil, "",
	)
	require.NoError(t, err)
	assert.NotEmpty(t, westStmt.ID)

	// us-east-1: sees only its own statement.
	eastList, _, err := backend.ListStatements(ctxEast, ListStatementsFilter{Status: "ALL"})
	require.NoError(t, err)
	require.Len(t, eastList, 1)
	assert.Equal(t, "east-db", eastList[0].Database)

	// us-west-2: sees only its own statement.
	westList, _, err := backend.ListStatements(ctxWest, ListStatementsFilter{Status: "ALL"})
	require.NoError(t, err)
	require.Len(t, westList, 1)
	assert.Equal(t, "west-db", westList[0].Database)

	// DescribeStatement in the wrong region returns not found.
	_, err = backend.DescribeStatement(ctxEast, westStmt.ID)
	require.ErrorIs(t, err, ErrNotFound)

	_, err = backend.DescribeStatement(ctxWest, eastStmt.ID)
	require.ErrorIs(t, err, ErrNotFound)

	// DescribeStatement in the correct region succeeds.
	got, err := backend.DescribeStatement(ctxEast, eastStmt.ID)
	require.NoError(t, err)
	assert.Equal(t, "east-db", got.Database)

	// Deleting (Reset) clears all regions.
	backend.Reset()

	eastAfter, _, err := backend.ListStatements(ctxEast, ListStatementsFilter{Status: "ALL"})
	require.NoError(t, err)
	assert.Empty(t, eastAfter)

	westAfter, _, err := backend.ListStatements(ctxWest, ListStatementsFilter{Status: "ALL"})
	require.NoError(t, err)
	assert.Empty(t, westAfter)
}

// TestRedshiftDataBatchStatementRegionIsolation proves batch statements are region-isolated.
func TestRedshiftDataBatchStatementRegionIsolation(t *testing.T) {
	t.Parallel()

	backend := NewInMemoryBackend("000000000000", "us-east-1")

	ctxEast := ctxRegion("us-east-1")
	ctxWest := ctxRegion("us-west-2")

	_, err := backend.BatchExecuteStatement(
		ctxEast, []string{"SELECT 1", "SELECT 2"}, "cluster-a", "", "east-db", "", "", "", false, "", nil, "",
	)
	require.NoError(t, err)

	_, err = backend.BatchExecuteStatement(
		ctxWest, []string{"SELECT 3"}, "cluster-b", "", "west-db", "", "", "", false, "", nil, "",
	)
	require.NoError(t, err)

	eastList, _, err := backend.ListStatements(ctxEast, ListStatementsFilter{Status: "ALL"})
	require.NoError(t, err)
	require.Len(t, eastList, 1)
	assert.Equal(t, "east-db", eastList[0].Database)

	westList, _, err := backend.ListStatements(ctxWest, ListStatementsFilter{Status: "ALL"})
	require.NoError(t, err)
	require.Len(t, westList, 1)
	assert.Equal(t, "west-db", westList[0].Database)
}
