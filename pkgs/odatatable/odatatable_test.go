// Package odatatable_test provides standalone coverage for pkgs/odatatable's
// public API. The engine this package implements is already exercised
// end-to-end (HTTP wire protocol included) by services/azuretable's own
// extensive test suite, since services/azuretable re-exports this package's
// types/functions under its historical names (see
// services/azuretable/interfaces.go's package doc comment) -- so this file
// deliberately does not re-duplicate that coverage. It instead checks the
// engine works correctly when driven directly, independent of any one wire
// protocol, since that is the exact property services/cosmosdb's Table API
// (a second, independent caller) now relies on.
package odatatable_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/odatatable"
)

func TestInMemoryBackend_EntityLifecycle(t *testing.T) {
	t.Parallel()

	b := odatatable.NewInMemoryBackend()
	require.NoError(t, b.CreateTable("t"))
	require.ErrorIs(t, b.CreateTable("t"), odatatable.ErrTableAlreadyExists)

	props := map[string]odatatable.EntityProperty{
		"Age": {Type: odatatable.EdmInt32, Value: int32(30)},
	}

	info, err := b.InsertEntity("t", "p", "r", props)
	require.NoError(t, err)
	assert.Equal(t, "p", info.PartitionKey)
	assert.Equal(t, "r", info.RowKey)
	assert.NotEmpty(t, info.ETag)

	_, err = b.InsertEntity("t", "p", "r", props)
	require.ErrorIs(t, err, odatatable.ErrEntityAlreadyExists)

	got, err := b.GetEntity("t", "p", "r")
	require.NoError(t, err)
	assert.Equal(t, int32(30), got.Properties["Age"].Value)

	replaced, err := b.ReplaceEntity("t", "p", "r", map[string]odatatable.EntityProperty{
		"Name": {Type: odatatable.EdmString, Value: "hi"},
	}, odatatable.IfMatchAny)
	require.NoError(t, err)
	_, hasAge := replaced.Properties["Age"]
	assert.False(t, hasAge, "replace must drop properties absent from the new body")

	_, err = b.ReplaceEntity("t", "p", "r", nil, "bogus-etag")
	require.ErrorIs(t, err, odatatable.ErrETagMismatch)

	require.NoError(t, b.DeleteEntity("t", "p", "r", odatatable.IfMatchAny))
	_, err = b.GetEntity("t", "p", "r")
	require.ErrorIs(t, err, odatatable.ErrEntityNotFound)

	require.NoError(t, b.DeleteTable("t"))
	require.ErrorIs(t, b.DeleteTable("t"), odatatable.ErrTableNotFound)
}

func TestInMemoryBackend_QueryEntities_FilterAndTop(t *testing.T) {
	t.Parallel()

	b := odatatable.NewInMemoryBackend()
	require.NoError(t, b.CreateTable("t"))

	for i, rowKey := range []string{"r1", "r2", "r3"} {
		_, err := b.InsertEntity("t", "p", rowKey, map[string]odatatable.EntityProperty{
			"N": {Type: odatatable.EdmInt32, Value: int32(i)},
		})
		require.NoError(t, err)
	}

	node, err := odatatable.ParseFilter("N ge 1")
	require.NoError(t, err)

	results, err := b.QueryEntities("t", node, 0)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	limited, err := b.QueryEntities("t", nil, 1)
	require.NoError(t, err)
	assert.Len(t, limited, 1)

	_, err = odatatable.ParseFilter("N ge")
	require.ErrorIs(t, err, odatatable.ErrFilterParse)
}

func TestDecodeEncodeEntity_RoundTrip(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"PartitionKey": "p", "RowKey": "r",
		"Count": 42,
		"Big": "123", "Big@odata.type": "Edm.Int64"
	}`)

	pk, rk, hasPK, hasRK, props, err := odatatable.DecodeEntityBody(body)
	require.NoError(t, err)
	assert.True(t, hasPK)
	assert.True(t, hasRK)
	assert.Equal(t, "p", pk)
	assert.Equal(t, "r", rk)
	assert.Equal(t, int32(42), props["Count"].Value)
	assert.Equal(t, int64(123), props["Big"].Value)

	entity := odatatable.EntityInfo{
		PartitionKey: pk,
		RowKey:       rk,
		Timestamp:    time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		ETag:         `W/"fixed"`,
		Properties:   props,
	}

	encoded := odatatable.EncodeEntity(entity, "mytable", odatatable.MetadataLevelFull, "", "http://x", "acct")
	assert.Equal(t, "acct.mytable", encoded["odata.type"])
	assert.Equal(t, "p", encoded["PartitionKey"])
	assert.Equal(t, "123", encoded["Big"])
	assert.Equal(t, "Edm.Int64", encoded["Big@odata.type"])

	selected := odatatable.EncodeEntity(entity, "mytable", odatatable.MetadataLevelMinimal, "Count", "http://x", "acct")
	_, hasBig := selected["Big"]
	assert.False(t, hasBig, "$select should drop unrequested properties")
	assert.Equal(t, int32(42), selected["Count"])
}

func TestParseEntityKeyPredicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		predicate string
		wantPK    string
		wantRK    string
		wantOK    bool
	}{
		{name: "in_order", predicate: "PartitionKey='p',RowKey='r'", wantPK: "p", wantRK: "r", wantOK: true},
		{name: "reversed", predicate: "RowKey='r',PartitionKey='p'", wantPK: "p", wantRK: "r", wantOK: true},
		{name: "escaped_quote", predicate: "PartitionKey='a''b',RowKey='r'", wantPK: "a'b", wantRK: "r", wantOK: true},
		{name: "missing_rowkey", predicate: "PartitionKey='p'", wantOK: false},
		{name: "malformed", predicate: "not-a-predicate", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pk, rk, ok := odatatable.ParseEntityKeyPredicate(tt.predicate)
			assert.Equal(t, tt.wantOK, ok)

			if tt.wantOK {
				assert.Equal(t, tt.wantPK, pk)
				assert.Equal(t, tt.wantRK, rk)
			}
		})
	}
}

func TestUnquoteEscapeODataString_RoundTrip(t *testing.T) {
	t.Parallel()

	escaped := odatatable.EscapeODataKey("it's a test")
	unquoted, ok := odatatable.UnquoteODataString("'" + escaped + "'")
	require.True(t, ok)
	assert.Equal(t, "it's a test", unquoted)

	_, ok = odatatable.UnquoteODataString("not-quoted")
	assert.False(t, ok)
}

func TestParseTop(t *testing.T) {
	t.Parallel()

	n, err := odatatable.ParseTop("")
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	n, err = odatatable.ParseTop("5")
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	_, err = odatatable.ParseTop("-1")
	require.Error(t, err)

	_, err = odatatable.ParseTop("not-a-number")
	require.Error(t, err)
}

func TestInMemoryBackend_SnapshotRestore_RoundTrip(t *testing.T) {
	t.Parallel()

	b := odatatable.NewInMemoryBackend()
	require.NoError(t, b.CreateTable("t"))
	_, err := b.InsertEntity("t", "p", "r", map[string]odatatable.EntityProperty{
		"Name": {Type: odatatable.EdmString, Value: "hi"},
	})
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	b2 := odatatable.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), snap))

	got, err := b2.GetEntity("t", "p", "r")
	require.NoError(t, err)
	assert.Equal(t, "hi", got.Properties["Name"].Value)
}

func TestInMemoryBackend_Restore_NullTableRejected(t *testing.T) {
	t.Parallel()

	b := odatatable.NewInMemoryBackend()
	err := b.Restore(t.Context(), []byte(`{"tables":{"t":null},"version":2}`))
	require.ErrorIs(t, err, odatatable.ErrSnapshotTableNull)
}

func TestSetNowFuncAndSetETagFunc(t *testing.T) {
	t.Parallel()

	b := odatatable.NewInMemoryBackend()
	fixed := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	odatatable.SetNowFunc(b, func() time.Time { return fixed })
	odatatable.SetETagFunc(b, func(time.Time) string { return "fixed-etag" })

	require.NoError(t, b.CreateTable("t"))

	info, err := b.InsertEntity("t", "p", "r", nil)
	require.NoError(t, err)
	assert.Equal(t, fixed, info.Timestamp)
	assert.Equal(t, "fixed-etag", info.ETag)
}

func TestEtagFor(t *testing.T) {
	t.Parallel()

	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Contains(t, odatatable.EtagFor(ts), `W/"datetime'`)
}
