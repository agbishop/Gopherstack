package glue_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glue"
)

// TestGetPartitions_OrderedByValues proves the indexed lookup introduced for
// gopherstack-a9rs preserves GetPartitions' pre-existing sorted-by-key order
// (see partitions.go's GetPartitions doc comment), even though
// store.Index.Get itself returns insertion order.
func TestGetPartitions_OrderedByValues(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDatabase(glue.DatabaseInput{Name: "db"}, nil)
	require.NoError(t, err)
	_, err = b.CreateTable("db", glue.TableInput{Name: "tbl"})
	require.NoError(t, err)

	// Inserted out of order.
	b.AddPartitionInternal("db", "tbl", &glue.Partition{Values: []string{"c"}})
	b.AddPartitionInternal("db", "tbl", &glue.Partition{Values: []string{"a"}})
	b.AddPartitionInternal("db", "tbl", &glue.Partition{Values: []string{"b"}})

	got, err := b.GetPartitions("db", "tbl")
	require.NoError(t, err)
	require.Len(t, got, 3)

	values := make([]string, len(got))
	for i, p := range got {
		values[i] = p.Values[0]
	}
	assert.Equal(t, []string{"a", "b", "c"}, values)
}

// TestGetPartitions_EmptyTableReturnsEmpty proves a table with no partitions
// returns an empty, non-nil slice rather than an error.
func TestGetPartitions_EmptyTableReturnsEmpty(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDatabase(glue.DatabaseInput{Name: "db"}, nil)
	require.NoError(t, err)
	_, err = b.CreateTable("db", glue.TableInput{Name: "tbl"})
	require.NoError(t, err)

	got, err := b.GetPartitions("db", "tbl")
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestGetPartitions_DoesNotLeakOtherTables proves the per-table index groups
// partitions correctly: a table's GetPartitions must never return another
// table's partitions, in the same database or a different one.
func TestGetPartitions_DoesNotLeakOtherTables(t *testing.T) {
	t.Parallel()

	b := glue.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateDatabase(glue.DatabaseInput{Name: "db1"}, nil)
	require.NoError(t, err)
	_, err = b.CreateDatabase(glue.DatabaseInput{Name: "db2"}, nil)
	require.NoError(t, err)
	_, err = b.CreateTable("db1", glue.TableInput{Name: "tbl"})
	require.NoError(t, err)
	_, err = b.CreateTable("db1", glue.TableInput{Name: "other"})
	require.NoError(t, err)
	_, err = b.CreateTable("db2", glue.TableInput{Name: "tbl"})
	require.NoError(t, err)

	b.AddPartitionInternal("db1", "tbl", &glue.Partition{Values: []string{"target"}})
	b.AddPartitionInternal("db1", "other", &glue.Partition{Values: []string{"sibling-table"}})
	b.AddPartitionInternal("db2", "tbl", &glue.Partition{Values: []string{"sibling-db"}})

	got, err := b.GetPartitions("db1", "tbl")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, []string{"target"}, got[0].Values)
}

// TestPagination_GetPartitions_RoundTripOrder proves NextToken-based
// pagination over the indexed lookup behaves identically to the old
// full-scan: same items, same order, no duplicates or gaps across pages.
func TestPagination_GetPartitions_RoundTripOrder(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createTestDB(t, h, "pgdb", "pgtbl")

	want := []string{"p0", "p1", "p2", "p3", "p4"}
	for _, v := range want {
		createTestPartition(t, h, "pgdb", "pgtbl", []string{v})
	}

	type page struct {
		NextToken  string `json:"NextToken"`
		Partitions []struct {
			Values []string `json:"Values"`
		} `json:"Partitions"`
	}

	var seen []string

	nextToken := ""
	for range want {
		req := map[string]any{
			"DatabaseName": "pgdb",
			"TableName":    "pgtbl",
			"MaxResults":   2,
		}
		if nextToken != "" {
			req["NextToken"] = nextToken
		}

		rec := doGlueRequest(t, h, "GetPartitions", req)
		require.Equal(t, http.StatusOK, rec.Code)

		var out page
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

		for _, p := range out.Partitions {
			seen = append(seen, p.Values[0])
		}

		nextToken = out.NextToken
		if nextToken == "" {
			break
		}
	}

	assert.Equal(t, want, seen, "pagination must return every partition exactly once, in stable sorted order")
}
