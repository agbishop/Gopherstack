package glue_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glue"
)

// setupPartitionBenchBackend seeds numTables tables, each with
// partitionsPerTable partitions, and returns the backend along with the name
// of one table in the middle of the set -- so GetPartitions on it must skip
// past every other table's partitions in a full-table scan.
func setupPartitionBenchBackend(b *testing.B, numTables, partitionsPerTable int) (*glue.InMemoryBackend, string) {
	b.Helper()

	backend := glue.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := backend.CreateDatabase(glue.DatabaseInput{Name: "benchdb"}, nil)
	require.NoError(b, err)

	var targetTable string

	for t := range numTables {
		tableName := fmt.Sprintf("tbl%04d", t)
		if t == numTables/2 {
			targetTable = tableName
		}

		_, createErr := backend.CreateTable("benchdb", glue.TableInput{Name: tableName})
		require.NoError(b, createErr)

		for p := range partitionsPerTable {
			backend.AddPartitionInternal("benchdb", tableName, &glue.Partition{
				Values: []string{fmt.Sprintf("%06d", p)},
			})
		}
	}

	return backend, targetTable
}

// BenchmarkGetPartitions_ManyTables measures GetPartitions for one table
// buried among many other tables' partitions -- the shape gopherstack-a9rs
// describes: a full backend-wide partition scan instead of an indexed
// per-table lookup.
func BenchmarkGetPartitions_ManyTables(b *testing.B) {
	backend, target := setupPartitionBenchBackend(b, 200, 50)

	b.ResetTimer()

	for range b.N {
		_, err := backend.GetPartitions("benchdb", target)
		if err != nil {
			b.Fatal(err)
		}
	}
}
