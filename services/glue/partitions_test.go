package glue_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glue"
)

func TestBatchCreatePartition_RoundTrip(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDatabase(glue.DatabaseInput{Name: "db"}, nil)
	require.NoError(t, err)
	_, err = b.CreateTable("db", glue.TableInput{Name: "tbl"})
	require.NoError(t, err)

	inputs := []glue.PartitionInput{
		{Values: []string{"2024", "01"}},
		{Values: []string{"2024", "02"}},
	}

	created, errs := b.BatchCreatePartition("db", "tbl", inputs)

	assert.Len(t, created, 2)
	assert.Empty(t, errs)
	assert.Equal(t, 2, glue.PartitionCount(b))

	// Duplicate should produce an error.
	created2, errs2 := b.BatchCreatePartition("db", "tbl", inputs)
	assert.Empty(t, created2)
	assert.Len(t, errs2, 2)
}

func TestBatchDeletePartition_RemovesPartition(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	b.AddPartitionInternal("db", "tbl", &glue.Partition{Values: []string{"2024", "01"}})
	b.AddPartitionInternal("db", "tbl", &glue.Partition{Values: []string{"2024", "02"}})

	errs := b.BatchDeletePartition("db", "tbl", []glue.PartitionValueList{
		{Values: []string{"2024", "01"}},
	})

	assert.Empty(t, errs)
	assert.Equal(t, 1, glue.PartitionCount(b))

	// Delete a non-existent partition.
	errs2 := b.BatchDeletePartition("db", "tbl", []glue.PartitionValueList{
		{Values: []string{"9999", "99"}},
	})

	assert.Len(t, errs2, 1)
	assert.Equal(t, "EntityNotFoundException", errs2[0].ErrorDetail.ErrorCode)
}

func TestBatchDeleteTable_CascadesPartitions(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDatabase(glue.DatabaseInput{Name: "db"}, nil)
	require.NoError(t, err)
	_, err = b.CreateTable("db", glue.TableInput{Name: "tbl"})
	require.NoError(t, err)

	b.AddPartitionInternal("db", "tbl", &glue.Partition{Values: []string{"2024"}})
	b.AddTableVersionInternal("db", "tbl", &glue.TableVersion{VersionID: "1"})

	require.Equal(t, 1, glue.PartitionCount(b))
	require.Equal(t, 1, glue.TableVersionCount(b))

	tableErrs := b.BatchDeleteTable("db", []string{"tbl"})

	assert.Empty(t, tableErrs)
	assert.Equal(t, 0, glue.TableCount(b))
	assert.Equal(t, 0, glue.PartitionCount(b))
	assert.Equal(t, 0, glue.TableVersionCount(b))
}

// TestBatchDeleteTable_CascadesColumnStatsAndOptimizers proves DeleteTable/
// BatchDeleteTable does not leave ghost column statistics or table
// optimizers behind: before the fix, these two side maps were never scoped
// to the deleting table, so a table dropped and recreated under the same
// name would silently resurface its predecessor's stale statistics.
func TestBatchDeleteTable_CascadesColumnStatsAndOptimizers(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDatabase(glue.DatabaseInput{Name: "db"}, nil)
	require.NoError(t, err)
	_, err = b.CreateTable("db", glue.TableInput{Name: "tbl"})
	require.NoError(t, err)

	b.AddPartitionInternal("db", "tbl", &glue.Partition{Values: []string{"2024"}})

	require.NoError(t, b.UpdateColumnStatisticsForTable(
		"db", "tbl", []*glue.ColumnStatistics{{ColumnName: "col", ColumnType: "string"}},
	))
	require.NoError(t, b.UpdateColumnStatisticsForPartition(
		"db", "tbl", []string{"2024"}, []*glue.ColumnStatistics{{ColumnName: "col", ColumnType: "string"}},
	))
	require.NoError(t, b.CreateTableOptimizer("", "db", "tbl", "compaction", glue.TableOptimizerConfiguration{}))

	require.Equal(t, 1, glue.TableColumnStatsCount(b))
	require.Equal(t, 1, glue.PartitionColumnStatsCount(b))
	require.Equal(t, 1, glue.TableOptimizerCount(b))

	tableErrs := b.BatchDeleteTable("db", []string{"tbl"})
	require.Empty(t, tableErrs)

	assert.Zero(t, glue.TableColumnStatsCount(b))
	assert.Zero(t, glue.PartitionColumnStatsCount(b))
	assert.Zero(t, glue.TableOptimizerCount(b))

	stats, err := b.GetColumnStatisticsForTable("db", "tbl", nil)
	require.NoError(t, err)
	assert.Empty(t, stats)

	pstats, err := b.GetColumnStatisticsForPartition("db", "tbl", []string{"2024"}, nil)
	require.NoError(t, err)
	assert.Empty(t, pstats)
}

// TestBatchDeletePartition_CascadesColumnStats proves deleting a single
// partition (without deleting its table) also drops that partition's column
// statistics, rather than leaving them reachable if a future partition
// happens to reuse the same values.
func TestBatchDeletePartition_CascadesColumnStats(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDatabase(glue.DatabaseInput{Name: "db"}, nil)
	require.NoError(t, err)
	_, err = b.CreateTable("db", glue.TableInput{Name: "tbl"})
	require.NoError(t, err)

	b.AddPartitionInternal("db", "tbl", &glue.Partition{Values: []string{"2024"}})
	require.NoError(t, b.UpdateColumnStatisticsForPartition(
		"db", "tbl", []string{"2024"}, []*glue.ColumnStatistics{{ColumnName: "col", ColumnType: "string"}},
	))
	require.Equal(t, 1, glue.PartitionColumnStatsCount(b))

	errs := b.BatchDeletePartition("db", "tbl", []glue.PartitionValueList{{Values: []string{"2024"}}})
	require.Empty(t, errs)

	assert.Zero(t, glue.PartitionColumnStatsCount(b))

	pstats, err := b.GetColumnStatisticsForPartition("db", "tbl", []string{"2024"}, nil)
	require.NoError(t, err)
	assert.Empty(t, pstats)
}
