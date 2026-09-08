package glue_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glue"
)

func TestReset(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("111111111111", "us-west-2")
	_, err := b.CreateDatabase(glue.DatabaseInput{Name: "db1"}, nil)
	require.NoError(t, err)
	require.Equal(t, 1, glue.DatabaseCount(b))

	b.Reset()

	assert.Equal(t, 0, glue.DatabaseCount(b))
	assert.Equal(t, 0, glue.TableCount(b))
	assert.Equal(t, 0, glue.CrawlerCount(b))
	assert.Equal(t, 0, glue.JobCount(b))
	assert.Equal(t, 0, glue.PartitionCount(b))
	assert.Equal(t, 0, glue.ConnectionCount(b))
	assert.Equal(t, 0, glue.BlueprintCount(b))
}

func TestMultipleResetCycle(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("111111111111", "us-west-2")

	for range 3 {
		_, err := b.CreateDatabase(glue.DatabaseInput{Name: "db"}, nil)
		require.NoError(t, err)
		require.Equal(t, 1, glue.DatabaseCount(b))

		b.Reset()

		assert.Equal(t, 0, glue.DatabaseCount(b))
	}
}

func TestHandlerReset(t *testing.T) {
	t.Parallel()

	backend := glue.NewInMemoryBackend("111111111111", "us-east-1")
	h := glue.NewHandler(backend)

	_, err := backend.CreateDatabase(glue.DatabaseInput{Name: "db1"}, nil)
	require.NoError(t, err)

	h.Reset()

	assert.Equal(t, 0, glue.DatabaseCount(backend))
}

func TestExportCountHelpers(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")

	assert.Equal(t, 0, glue.DatabaseCount(b))
	assert.Equal(t, 0, glue.TableCount(b))
	assert.Equal(t, 0, glue.CrawlerCount(b))
	assert.Equal(t, 0, glue.JobCount(b))
	assert.Equal(t, 0, glue.PartitionCount(b))
	assert.Equal(t, 0, glue.TableVersionCount(b))
	assert.Equal(t, 0, glue.ConnectionCount(b))
	assert.Equal(t, 0, glue.BlueprintCount(b))
	assert.Equal(t, 0, glue.CustomEntityTypeCount(b))
	assert.Equal(t, 0, glue.DataQualityResultCount(b))
	assert.Equal(t, 0, glue.DevEndpointCount(b))

	_, err := b.CreateDatabase(glue.DatabaseInput{Name: "db1"}, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, glue.DatabaseCount(b))
}

func TestSortedGetDatabases(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")

	for _, name := range []string{"gamma", "alpha", "beta"} {
		_, err := b.CreateDatabase(glue.DatabaseInput{Name: name}, nil)
		require.NoError(t, err)
	}

	dbs := b.GetDatabases()
	require.Len(t, dbs, 3)

	assert.Equal(t, "alpha", dbs[0].Name)
	assert.Equal(t, "beta", dbs[1].Name)
	assert.Equal(t, "gamma", dbs[2].Name)
}

// TestDeleteDatabase_CascadesUDFsAndTableChildren proves DeleteDatabase makes
// the database's user-defined functions and every table's column statistics/
// table optimizers unreachable too, matching the real DeleteDatabase's
// documented contract (aws-sdk-go-v2/service/glue@v1.152.0
// api_op_DeleteDatabase.go: "you no longer have access to the tables ... and
// the user-defined functions in the deleted database").
func TestDeleteDatabase_CascadesUDFsAndTableChildren(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDatabase(glue.DatabaseInput{Name: "db"}, nil)
	require.NoError(t, err)
	_, err = b.CreateTable("db", glue.TableInput{Name: "tbl"})
	require.NoError(t, err)
	_, err = b.CreateUserDefinedFunction("db", glue.UserDefinedFunction{FunctionName: "fn"}, nil)
	require.NoError(t, err)
	require.NoError(t, b.UpdateColumnStatisticsForTable(
		"db", "tbl", []*glue.ColumnStatistics{{ColumnName: "col", ColumnType: "string"}},
	))

	require.Equal(t, 1, glue.UDFCount(b))
	require.Equal(t, 1, glue.TableColumnStatsCount(b))

	require.NoError(t, b.DeleteDatabase("db"))

	assert.Zero(t, glue.UDFCount(b))
	assert.Zero(t, glue.TableColumnStatsCount(b))

	_, err = b.GetUserDefinedFunction("db", "fn")
	assert.ErrorIs(t, err, glue.ErrNotFound)
}

func TestHandlerSnapshotRestore(t *testing.T) {
	t.Parallel()

	backend := glue.NewInMemoryBackend("000000000000", "us-east-1")
	h := glue.NewHandler(backend)

	_, err := backend.CreateDatabase(glue.DatabaseInput{Name: "db1"}, nil)
	require.NoError(t, err)

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	backend2 := glue.NewInMemoryBackend("000000000000", "us-east-1")
	h2 := glue.NewHandler(backend2)

	err = h2.Restore(t.Context(), snap)
	require.NoError(t, err)

	assert.Equal(t, 1, glue.DatabaseCount(backend2))
}
