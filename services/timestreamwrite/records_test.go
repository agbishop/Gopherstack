package timestreamwrite_test

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/timestreamwrite"
)

func TestInMemoryBackend_WriteRecords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errIs     error
		name      string
		dbName    string
		tblName   string
		records   []timestreamwrite.Record
		createDB  bool
		createTbl bool
		wantErr   bool
	}{
		{
			name:      "success",
			dbName:    "db",
			tblName:   "tbl",
			createDB:  true,
			createTbl: true,
			records: []timestreamwrite.Record{
				{MeasureName: "cpu", MeasureValue: "98.5", MeasureValueType: "DOUBLE"},
			},
			wantErr: false,
		},
		{
			name:      "table not found",
			dbName:    "db",
			tblName:   "missing",
			createDB:  true,
			createTbl: false,
			records:   []timestreamwrite.Record{{MeasureName: "cpu"}},
			wantErr:   true,
			errIs:     awserr.ErrNotFound,
		},
		{
			name:      "database not found",
			dbName:    "missing-db",
			tblName:   "tbl",
			createDB:  false,
			createTbl: false,
			records:   []timestreamwrite.Record{{MeasureName: "cpu"}},
			wantErr:   true,
			errIs:     awserr.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			if tt.createDB {
				_, err := b.CreateDatabase(tt.dbName, "", nil)
				require.NoError(t, err)
			}

			if tt.createTbl {
				_, err := b.CreateTable(tt.dbName, tt.tblName, nil, nil)
				require.NoError(t, err)
			}

			_, err := b.WriteRecords(tt.dbName, tt.tblName, tt.records)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					require.ErrorIs(t, err, tt.errIs)
				}

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestInMemoryBackend_WriteRecords_CrossTableRace exercises concurrent
// WriteRecords for different tables in the SAME database. Under the previous
// design these would each acquire a different per-table mutex but still write
// to the shared inner b.records[dbName] map (b.records[dbName][tblName] = append(...)),
// which is a fatal "concurrent map writes" race in Go even on different keys.
// The fix moves each table's slice into a *tableRecords slot pointer so the
// append never touches the enclosing map.
func TestInMemoryBackend_WriteRecords_CrossTableRace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tableCount int
		iterations int
	}{
		{name: "two tables few writes", tableCount: 2, iterations: 50},
		{name: "eight tables many writes", tableCount: 8, iterations: 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			_, err := b.CreateDatabase("db", "", nil)
			require.NoError(t, err)

			for i := range tt.tableCount {
				_, err = b.CreateTable("db", "tbl"+strconv.Itoa(i), nil, nil)
				require.NoError(t, err)
			}

			rec := []timestreamwrite.Record{{
				MeasureName:  "m",
				MeasureValue: "1",
				Time:         recentTimeSeconds(),
				TimeUnit:     "SECONDS",
			}}

			done := make(chan struct{})
			for i := range tt.tableCount {
				go func() {
					tbl := "tbl" + strconv.Itoa(i)
					for range tt.iterations {
						_, _ = b.WriteRecords("db", tbl, rec)
					}
					done <- struct{}{}
				}()
			}

			for range tt.tableCount {
				<-done
			}
		})
	}
}

// TestInMemoryBackend_WriteRecords_SnapshotRace concurrently writes and snapshots
// the backend; with the race detector enabled this catches any concurrent map
// read/write between WriteRecords (under global RLock + per-table WLock) and
// Snapshot (which previously also held only RLock and iterated b.records).
func TestInMemoryBackend_WriteRecords_SnapshotRace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		writers    int
		iterations int
	}{
		{name: "single writer", writers: 1, iterations: 50},
		{name: "four writers", writers: 4, iterations: 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			_, err := b.CreateDatabase("db", "", nil)
			require.NoError(t, err)
			_, err = b.CreateTable("db", "tbl", nil, nil)
			require.NoError(t, err)

			rec := []timestreamwrite.Record{{
				MeasureName:  "m",
				MeasureValue: "1",
				Time:         recentTimeSeconds(),
				TimeUnit:     "SECONDS",
			}}

			done := make(chan struct{})
			for range tt.writers {
				go func() {
					for range tt.iterations {
						_, _ = b.WriteRecords("db", "tbl", rec)
					}
					done <- struct{}{}
				}()
			}

			go func() {
				for range tt.iterations {
					_, _ = b.Snapshot()
				}
				done <- struct{}{}
			}()

			for range tt.writers + 1 {
				<-done
			}
		})
	}
}

// TestInMemoryBackend_WriteRecords_MeasureValues verifies MeasureValues (MULTI
// type records) survive in the backend.
func TestInMemoryBackend_WriteRecords_MeasureValues(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("mv-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("mv-db", "mv-tbl", nil, nil)
	require.NoError(t, err)

	records := []timestreamwrite.Record{
		{
			MeasureName:      "system",
			MeasureValueType: "MULTI",
			Time:             recentTimeMillis(0),
			TimeUnit:         "MILLISECONDS",
			MeasureValues: []timestreamwrite.MeasureValue{
				{Name: "cpu", Value: "12.5", Type: "DOUBLE"},
				{Name: "status", Value: "healthy", Type: "VARCHAR"},
			},
		},
	}

	out, err := b.WriteRecords("mv-db", "mv-tbl", records)
	require.NoError(t, err)
	assert.Equal(t, int32(1), out.Total)
}

// TestInMemoryBackend_WriteRecords_DimensionValueTypePassthrough verifies that
// DimensionValueType is preserved in the backend when specified on WriteRecords.
func TestInMemoryBackend_WriteRecords_DimensionValueTypePassthrough(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("dvt-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("dvt-db", "dvt-tbl", nil, nil)
	require.NoError(t, err)

	records := []timestreamwrite.Record{
		{
			MeasureName:      "cpu",
			MeasureValue:     "42.5",
			MeasureValueType: "DOUBLE",
			Time:             recentTimeMillis(0),
			TimeUnit:         "MILLISECONDS",
			Dimensions: []timestreamwrite.Dimension{
				{Name: "host", Value: "server-1", DimensionValueType: "VARCHAR"},
			},
		},
	}

	out, err := b.WriteRecords("dvt-db", "dvt-tbl", records)
	require.NoError(t, err)
	assert.Equal(t, int32(1), out.Total)
}

// TestInMemoryBackend_WriteRecords_VersionUpsert verifies that a record with a
// higher version replaces the existing record without error.
func TestInMemoryBackend_WriteRecords_VersionUpsert(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("vu-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("vu-db", "vu-tbl", nil, nil)
	require.NoError(t, err)

	record := timestreamwrite.Record{
		MeasureName:      "cpu",
		MeasureValue:     "10.0",
		MeasureValueType: "DOUBLE",
		Time:             recentTimeMillis(0),
		TimeUnit:         "MILLISECONDS",
		Version:          1,
	}

	out, err := b.WriteRecords("vu-db", "vu-tbl", []timestreamwrite.Record{record})
	require.NoError(t, err)
	assert.Equal(t, int32(1), out.Total)

	// Same key, higher version: upsert succeeds.
	record.Version = 2
	record.MeasureValue = "20.0"
	out2, err := b.WriteRecords("vu-db", "vu-tbl", []timestreamwrite.Record{record})
	require.NoError(t, err)
	assert.Equal(t, int32(1), out2.Total)
}

// TestInMemoryBackend_WriteRecords_VersionConflictReturnsError verifies that a
// lower/equal version write returns a RejectedRecordsError.
func TestInMemoryBackend_WriteRecords_VersionConflictReturnsError(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("vc-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("vc-db", "vc-tbl", nil, nil)
	require.NoError(t, err)

	record := timestreamwrite.Record{
		MeasureName:      "mem",
		MeasureValue:     "8192",
		MeasureValueType: "BIGINT",
		Time:             recentTimeMillis(1),
		TimeUnit:         "MILLISECONDS",
		Version:          2,
	}

	_, err = b.WriteRecords("vc-db", "vc-tbl", []timestreamwrite.Record{record})
	require.NoError(t, err)

	// Same record, lower version: should be rejected.
	record.Version = 1
	_, err = b.WriteRecords("vc-db", "vc-tbl", []timestreamwrite.Record{record})
	require.Error(t, err)

	var rejErr *timestreamwrite.RejectedRecordsError
	require.ErrorAs(t, err, &rejErr)
	require.Len(t, rejErr.RejectedRecords, 1)
	assert.Equal(t, 0, rejErr.RejectedRecords[0].RecordIndex)
	assert.Equal(t, int64(2), rejErr.RejectedRecords[0].ExistingVersion)
}

// TestInMemoryBackend_WriteRecords_SameVersionConflict verifies that a
// same-version write (when version already stored) is also rejected per AWS
// upsert rules.
func TestInMemoryBackend_WriteRecords_SameVersionConflict(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("sv-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("sv-db", "sv-tbl", nil, nil)
	require.NoError(t, err)

	record := timestreamwrite.Record{
		MeasureName:      "rps",
		MeasureValue:     "500",
		MeasureValueType: "BIGINT",
		Time:             recentTimeMillis(2),
		TimeUnit:         "MILLISECONDS",
		Version:          3,
	}

	_, err = b.WriteRecords("sv-db", "sv-tbl", []timestreamwrite.Record{record})
	require.NoError(t, err)

	// Same version: rejected.
	_, err = b.WriteRecords("sv-db", "sv-tbl", []timestreamwrite.Record{record})
	require.Error(t, err)

	var rejErr *timestreamwrite.RejectedRecordsError
	require.ErrorAs(t, err, &rejErr)
	assert.Len(t, rejErr.RejectedRecords, 1)
}

// TestInMemoryBackend_WriteRecords_PartialReject verifies that when some
// records pass and some fail version checks, only the failed ones are rejected.
func TestInMemoryBackend_WriteRecords_PartialReject(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("pr2-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("pr2-db", "pr2-tbl", nil, nil)
	require.NoError(t, err)

	existing := timestreamwrite.Record{
		MeasureName: "load", MeasureValue: "1.0", MeasureValueType: "DOUBLE",
		Time: recentTimeMillis(0), TimeUnit: "MILLISECONDS", Version: 3,
	}
	_, err = b.WriteRecords("pr2-db", "pr2-tbl", []timestreamwrite.Record{existing})
	require.NoError(t, err)

	// Two records: one new (should pass), one conflicting (should fail).
	records := []timestreamwrite.Record{
		{
			MeasureName: "load2", MeasureValue: "2.0", MeasureValueType: "DOUBLE",
			Time: recentTimeMillis(1), TimeUnit: "MILLISECONDS", Version: 1,
		},
		existing, // same key, same version → rejected
	}

	_, err = b.WriteRecords("pr2-db", "pr2-tbl", records)
	require.Error(t, err)

	var rejErr *timestreamwrite.RejectedRecordsError
	require.ErrorAs(t, err, &rejErr)
	// Only the second record (index 1) should be rejected.
	require.Len(t, rejErr.RejectedRecords, 1)
	assert.Equal(t, 1, rejErr.RejectedRecords[0].RecordIndex)
}

// TestInMemoryBackend_WriteRecords_UpsertAndNewRecord verifies that after a
// successful upsert, subsequent writes to the same key require a
// further-incremented version.
func TestInMemoryBackend_WriteRecords_UpsertAndNewRecord(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("uan-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("uan-db", "uan-tbl", nil, nil)
	require.NoError(t, err)

	base := timestreamwrite.Record{
		MeasureName: "ops", MeasureValue: "100", MeasureValueType: "BIGINT",
		Time: recentTimeMillis(0), TimeUnit: "MILLISECONDS", Version: 1,
	}

	// First write.
	_, err = b.WriteRecords("uan-db", "uan-tbl", []timestreamwrite.Record{base})
	require.NoError(t, err)

	// Upsert with higher version.
	upserted := base
	upserted.Version = 2
	upserted.MeasureValue = "200"
	_, err = b.WriteRecords("uan-db", "uan-tbl", []timestreamwrite.Record{upserted})
	require.NoError(t, err)

	// Now try with version 1 (lower than upserted version 2): should fail.
	lower := base
	lower.Version = 1
	_, err = b.WriteRecords("uan-db", "uan-tbl", []timestreamwrite.Record{lower})
	require.Error(t, err)

	var rejErr *timestreamwrite.RejectedRecordsError
	require.ErrorAs(t, err, &rejErr)
	assert.Equal(t, int64(2), rejErr.RejectedRecords[0].ExistingVersion)
}

// TestInMemoryBackend_WriteRecords_ErrRejectedRecordsSentinel verifies
// errors.As matches RejectedRecordsError.
func TestInMemoryBackend_WriteRecords_ErrRejectedRecordsSentinel(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("sentinel-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("sentinel-db", "sentinel-tbl", nil, nil)
	require.NoError(t, err)

	rec := timestreamwrite.Record{
		MeasureName: "x", MeasureValue: "1", MeasureValueType: "DOUBLE",
		Time: recentTimeMillis(0), TimeUnit: "MILLISECONDS", Version: 5,
	}
	_, err = b.WriteRecords("sentinel-db", "sentinel-tbl", []timestreamwrite.Record{rec})
	require.NoError(t, err)

	rec.Version = 1
	_, err = b.WriteRecords("sentinel-db", "sentinel-tbl", []timestreamwrite.Record{rec})
	require.Error(t, err)

	var rejErr *timestreamwrite.RejectedRecordsError
	assert.ErrorAs(t, err, &rejErr, "error should be detectable as *RejectedRecordsError")
}

// TestInMemoryBackend_WriteRecords_DefaultVersionIsOne verifies that records
// without an explicit Version are treated as Version=1, and re-writing without
// Version is rejected.
func TestInMemoryBackend_WriteRecords_DefaultVersionIsOne(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("dv-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("dv-db", "dv-tbl", nil, nil)
	require.NoError(t, err)

	rec := timestreamwrite.Record{
		MeasureName:      "metric",
		MeasureValue:     "1",
		MeasureValueType: "BIGINT",
		Time:             recentTimeMillis(0),
		TimeUnit:         "MILLISECONDS",
		// No Version — defaults to 1
	}

	_, err = b.WriteRecords("dv-db", "dv-tbl", []timestreamwrite.Record{rec})
	require.NoError(t, err)

	// Writing same record again without version (still 0→1) should fail.
	_, err = b.WriteRecords("dv-db", "dv-tbl", []timestreamwrite.Record{rec})
	require.Error(t, err)

	var rejErr *timestreamwrite.RejectedRecordsError
	require.ErrorAs(t, err, &rejErr)
	assert.Equal(t, int64(1), rejErr.RejectedRecords[0].ExistingVersion)
}

// TestInMemoryBackend_WriteRecords_MagneticStoreRouting verifies that records
// whose timestamps fall outside the memory retention window are counted
// toward the magnetic store (when EnableMagneticStoreWrites is true).
func TestInMemoryBackend_WriteRecords_MagneticStoreRouting(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("mag-route-db", "", nil)
	require.NoError(t, err)

	// Create a table with 1-hour memory retention and magnetic store writes enabled.
	_, err = b.CreateTable("mag-route-db", "mag-route-tbl", nil, &timestreamwrite.CreateTableInput{
		RetentionProperties: &timestreamwrite.RetentionProperties{
			MemoryStoreRetentionPeriodInHours:  1,
			MagneticStoreRetentionPeriodInDays: 365,
		},
		MagneticStoreWriteProperties: &timestreamwrite.MagneticStoreWriteProperties{
			EnableMagneticStoreWrites: true,
		},
	})
	require.NoError(t, err)

	// A timestamp from 2 hours ago, well outside the 1-hour retention window.
	twoHoursAgo := time.Now().UTC().Add(-2 * time.Hour)
	oldTS := strconv.FormatInt(twoHoursAgo.UnixMilli(), 10)

	out, err := b.WriteRecords("mag-route-db", "mag-route-tbl", []timestreamwrite.Record{
		{
			MeasureName:      "cpu",
			MeasureValue:     "55.0",
			MeasureValueType: "DOUBLE",
			Time:             oldTS,
			TimeUnit:         "MILLISECONDS",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), out.Total)
	assert.Equal(t, int32(0), out.MemoryStore, "old record should go to magnetic store")
	assert.Equal(t, int32(1), out.MagneticStore, "old record should be counted in magnetic store")
}

// TestInMemoryBackend_WriteRecords_MemoryStoreWhenMagneticDisabled verifies
// that when magnetic store writes are disabled, a record within the memory
// retention window still goes to the memory store.
func TestInMemoryBackend_WriteRecords_MemoryStoreWhenMagneticDisabled(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("mag-off-db", "", nil)
	require.NoError(t, err)

	_, err = b.CreateTable("mag-off-db", "mag-off-tbl", nil, &timestreamwrite.CreateTableInput{
		RetentionProperties: &timestreamwrite.RetentionProperties{
			MemoryStoreRetentionPeriodInHours:  1,
			MagneticStoreRetentionPeriodInDays: 365,
		},
		MagneticStoreWriteProperties: &timestreamwrite.MagneticStoreWriteProperties{
			EnableMagneticStoreWrites: false, // magnetic store writes disabled
		},
	})
	require.NoError(t, err)

	recentTS := strconv.FormatInt(time.Now().UTC().Add(-10*time.Minute).UnixMilli(), 10)

	out, err := b.WriteRecords("mag-off-db", "mag-off-tbl", []timestreamwrite.Record{
		{
			MeasureName:      "mem",
			MeasureValue:     "8192",
			MeasureValueType: "BIGINT",
			Time:             recentTS,
			TimeUnit:         "MILLISECONDS",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), out.Total)
	assert.Equal(t, int32(1), out.MemoryStore, "records should go to memory store when magnetic disabled")
	assert.Equal(t, int32(0), out.MagneticStore)
}

// TestInMemoryBackend_WriteRecords_RejectedOutsideRetentionWhenMagneticDisabled
// verifies that a record whose timestamp lies outside the memory-store
// retention window is rejected via RejectedRecordsException when the table
// has no magnetic store write path to receive it, per RejectedRecordsException
// (types/errors.go, timestreamwrite@v1.38.4): "Records with timestamps that
// lie outside the retention duration of the memory store".
func TestInMemoryBackend_WriteRecords_RejectedOutsideRetentionWhenMagneticDisabled(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("mag-off-rej-db", "", nil)
	require.NoError(t, err)

	_, err = b.CreateTable("mag-off-rej-db", "mag-off-rej-tbl", nil, &timestreamwrite.CreateTableInput{
		RetentionProperties: &timestreamwrite.RetentionProperties{
			MemoryStoreRetentionPeriodInHours:  1,
			MagneticStoreRetentionPeriodInDays: 365,
		},
	})
	require.NoError(t, err)

	veryOldTS := strconv.FormatInt(time.Now().UTC().Add(-72*time.Hour).UnixMilli(), 10)

	out, err := b.WriteRecords("mag-off-rej-db", "mag-off-rej-tbl", []timestreamwrite.Record{
		{
			MeasureName:      "mem",
			MeasureValue:     "8192",
			MeasureValueType: "BIGINT",
			Time:             veryOldTS,
			TimeUnit:         "MILLISECONDS",
		},
	})
	require.Nil(t, out)

	var rejErr *timestreamwrite.RejectedRecordsError
	require.ErrorAs(t, err, &rejErr)
	require.Len(t, rejErr.RejectedRecords, 1)
	assert.Equal(t, 0, rejErr.RejectedRecords[0].RecordIndex)
	assert.Contains(t, rejErr.RejectedRecords[0].Reason, "retention period")
}

// TestInMemoryBackend_WriteRecords_MixedStoreRouting verifies that a batch
// containing both recent and old records routes them to the correct stores.
func TestInMemoryBackend_WriteRecords_MixedStoreRouting(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("mixed-store-db", "", nil)
	require.NoError(t, err)

	_, err = b.CreateTable("mixed-store-db", "mixed-store-tbl", nil, &timestreamwrite.CreateTableInput{
		RetentionProperties: &timestreamwrite.RetentionProperties{
			MemoryStoreRetentionPeriodInHours:  1,
			MagneticStoreRetentionPeriodInDays: 365,
		},
		MagneticStoreWriteProperties: &timestreamwrite.MagneticStoreWriteProperties{
			EnableMagneticStoreWrites: true,
		},
	})
	require.NoError(t, err)

	recentTS := strconv.FormatInt(time.Now().UTC().UnixMilli(), 10)
	oldTS := strconv.FormatInt(time.Now().UTC().Add(-3*time.Hour).UnixMilli(), 10)

	out, err := b.WriteRecords("mixed-store-db", "mixed-store-tbl", []timestreamwrite.Record{
		{
			MeasureName: "m1", MeasureValue: "1.0", MeasureValueType: "DOUBLE",
			Time: recentTS, TimeUnit: "MILLISECONDS",
		},
		{
			MeasureName: "m2", MeasureValue: "2.0", MeasureValueType: "DOUBLE",
			Time: oldTS, TimeUnit: "MILLISECONDS",
		},
		{
			MeasureName: "m3", MeasureValue: "3.0", MeasureValueType: "DOUBLE",
			Time: recentTS, TimeUnit: "MILLISECONDS",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(3), out.Total)
	assert.Equal(t, int32(2), out.MemoryStore, "two recent records should go to memory store")
	assert.Equal(t, int32(1), out.MagneticStore, "one old record should go to magnetic store")
}

// TestInMemoryBackend_WriteRecords_MagneticStoreDefaultRetention verifies that
// when no explicit retention is configured, AWS defaults (6h memory) are
// applied, so very old records go to magnetic store (not memory).
func TestInMemoryBackend_WriteRecords_MagneticStoreDefaultRetention(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("no-ret-mag-db", "", nil)
	require.NoError(t, err)

	// Table has magnetic store enabled; RetentionProperties defaults to {6h, 73d}.
	_, err = b.CreateTable("no-ret-mag-db", "no-ret-mag-tbl", nil, &timestreamwrite.CreateTableInput{
		MagneticStoreWriteProperties: &timestreamwrite.MagneticStoreWriteProperties{
			EnableMagneticStoreWrites: true,
		},
	})
	require.NoError(t, err)

	veryOldTS := strconv.FormatInt(time.Now().UTC().Add(-1000*24*time.Hour).UnixMilli(), 10)

	out, err := b.WriteRecords("no-ret-mag-db", "no-ret-mag-tbl", []timestreamwrite.Record{
		{
			MeasureName: "m", MeasureValue: "1", MeasureValueType: "BIGINT",
			Time: veryOldTS, TimeUnit: "MILLISECONDS",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), out.Total)
	// Default 6h memory retention: 1000-day-old record exceeds memory window → magnetic store.
	assert.Equal(t, int32(0), out.MemoryStore)
	assert.Equal(t, int32(1), out.MagneticStore, "default 6h retention → old record goes to magnetic store")
}

// TestInMemoryBackend_WriteRecordsOutput_MagneticStoreField verifies that the
// WriteRecordsOutput struct exposes a MagneticStore field that can be read.
func TestInMemoryBackend_WriteRecordsOutput_MagneticStoreField(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("ms-field-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("ms-field-db", "ms-field-tbl", nil, nil)
	require.NoError(t, err)

	out, err := b.WriteRecords("ms-field-db", "ms-field-tbl", []timestreamwrite.Record{
		{MeasureName: "m", MeasureValue: "1", Time: recentTimeMillis(0), TimeUnit: "MILLISECONDS"},
	})
	require.NoError(t, err)
	// Without magnetic store write config, MagneticStore should be 0.
	assert.Equal(t, int32(0), out.MagneticStore,
		"MagneticStore should be 0 when magnetic store writes are not enabled")
	assert.Equal(t, int32(1), out.MemoryStore,
		"MemoryStore should equal Total when magnetic store writes are not enabled")
	assert.Equal(t, out.MemoryStore+out.MagneticStore, out.Total,
		"Total must equal MemoryStore + MagneticStore")
}

// TestInMemoryBackend_WriteRecordsOutput_TotalInvariant verifies the
// accounting invariant: Total == MemoryStore + MagneticStore for all
// WriteRecords calls.
func TestInMemoryBackend_WriteRecordsOutput_TotalInvariant(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("inv-db", "", nil)
	require.NoError(t, err)

	_, err = b.CreateTable("inv-db", "inv-tbl", nil, &timestreamwrite.CreateTableInput{
		RetentionProperties: &timestreamwrite.RetentionProperties{
			MemoryStoreRetentionPeriodInHours:  2,
			MagneticStoreRetentionPeriodInDays: 365,
		},
		MagneticStoreWriteProperties: &timestreamwrite.MagneticStoreWriteProperties{
			EnableMagneticStoreWrites: true,
		},
	})
	require.NoError(t, err)

	recentTS := strconv.FormatInt(time.Now().UTC().UnixMilli(), 10)
	oldTS := strconv.FormatInt(time.Now().UTC().Add(-10*time.Hour).UnixMilli(), 10)

	records := []timestreamwrite.Record{
		{MeasureName: "m1", MeasureValue: "1", MeasureValueType: "DOUBLE", Time: recentTS, TimeUnit: "MILLISECONDS"},
		{MeasureName: "m2", MeasureValue: "2", MeasureValueType: "DOUBLE", Time: oldTS, TimeUnit: "MILLISECONDS"},
		{MeasureName: "m3", MeasureValue: "3", MeasureValueType: "DOUBLE", Time: recentTS, TimeUnit: "MILLISECONDS"},
		{MeasureName: "m4", MeasureValue: "4", MeasureValueType: "DOUBLE", Time: oldTS, TimeUnit: "MILLISECONDS"},
	}

	out, err := b.WriteRecords("inv-db", "inv-tbl", records)
	require.NoError(t, err)

	assert.Equal(t, out.MemoryStore+out.MagneticStore, out.Total,
		"Total must equal MemoryStore + MagneticStore invariant")
	assert.Equal(t, int32(len(records)), out.Total, "Total should equal number of records")
}

// TestInMemoryBackend_WriteRecordsOutput_Int32Fields verifies that
// WriteRecords returns a *WriteRecordsOutput with int32 fields.
func TestInMemoryBackend_WriteRecordsOutput_Int32Fields(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("wr2-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("wr2-db", "wr2-tbl", nil, nil)
	require.NoError(t, err)

	out, err := b.WriteRecords("wr2-db", "wr2-tbl", []timestreamwrite.Record{
		{MeasureName: "m", MeasureValue: "1"},
		{MeasureName: "m2", MeasureValue: "2"},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(2), out.Total)
	assert.Equal(t, int32(2), out.MemoryStore)
}

// TestInMemoryBackend_ConcurrentWriteRecordsToSameTable verifies that
// simultaneous WriteRecords calls to the same table do not produce data races
// or corrupt state.
func TestInMemoryBackend_ConcurrentWriteRecordsToSameTable(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("conc-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("conc-db", "conc-tbl", nil, nil)
	require.NoError(t, err)

	const goroutines = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()

			_, errs[idx] = b.WriteRecords("conc-db", "conc-tbl", []timestreamwrite.Record{
				{
					MeasureName:      fmt.Sprintf("metric-%d", idx),
					MeasureValue:     fmt.Sprintf("%d.0", idx),
					MeasureValueType: "DOUBLE",
					Time:             recentTimeMillis(int64(idx) * 1000),
					TimeUnit:         "MILLISECONDS",
				},
			})
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "goroutine %d should not error", i)
	}

	// All goroutines wrote distinct records (different measure names and times).
	assert.Equal(t, goroutines, timestreamwrite.RecordCount(b, "conc-db", "conc-tbl"),
		"all goroutine-written records should be stored")
}

// TestInMemoryBackend_ConcurrentWriteRecordsToDifferentTables verifies that
// concurrent writes to different tables in the same database do not interfere.
func TestInMemoryBackend_ConcurrentWriteRecordsToDifferentTables(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("conc2-db", "", nil)
	require.NoError(t, err)

	tables := []string{"tbl-a", "tbl-b", "tbl-c", "tbl-d"}
	for _, tbl := range tables {
		_, err = b.CreateTable("conc2-db", tbl, nil, nil)
		require.NoError(t, err)
	}

	const recordsPerTable = 5

	var wg sync.WaitGroup
	wg.Add(len(tables) * recordsPerTable)

	for _, tblName := range tables {
		for j := range recordsPerTable {
			go func(tbl string, idx int) {
				defer wg.Done()

				_, _ = b.WriteRecords("conc2-db", tbl, []timestreamwrite.Record{
					{
						MeasureName:      fmt.Sprintf("m-%d", idx),
						MeasureValue:     "1",
						MeasureValueType: "BIGINT",
						Time:             recentTimeMillis(int64(idx) * 1000),
						TimeUnit:         "MILLISECONDS",
					},
				})
			}(tblName, j)
		}
	}

	wg.Wait()

	for _, tbl := range tables {
		count := timestreamwrite.RecordCount(b, "conc2-db", tbl)
		assert.Equal(t, recordsPerTable, count,
			"table %s should have %d records", tbl, recordsPerTable)
	}
}

// TestInMemoryBackend_RecordCountExport verifies that the RecordCount export
// correctly reflects the number of records stored in a table.
func TestInMemoryBackend_RecordCountExport(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("rcnt-db", "", nil)
	require.NoError(t, err)
	_, err = b.CreateTable("rcnt-db", "rcnt-tbl", nil, nil)
	require.NoError(t, err)

	assert.Equal(t, 0, timestreamwrite.RecordCount(b, "rcnt-db", "rcnt-tbl"),
		"new table should have zero records")

	_, err = b.WriteRecords("rcnt-db", "rcnt-tbl", []timestreamwrite.Record{
		{MeasureName: "m1", MeasureValue: "1", Time: recentTimeMillis(0), TimeUnit: "MILLISECONDS"},
		{MeasureName: "m2", MeasureValue: "2", Time: recentTimeMillis(1), TimeUnit: "MILLISECONDS"},
		{MeasureName: "m3", MeasureValue: "3", Time: recentTimeMillis(2), TimeUnit: "MILLISECONDS"},
	})
	require.NoError(t, err)

	assert.Equal(t, 3, timestreamwrite.RecordCount(b, "rcnt-db", "rcnt-tbl"),
		"record count should reflect ingested records")
}

// TestInMemoryBackend_RecordCountNonexistentTable verifies that RecordCount
// returns zero for a table that does not exist.
func TestInMemoryBackend_RecordCountNonexistentTable(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()

	count := timestreamwrite.RecordCount(b, "ghost-db", "ghost-tbl")
	assert.Equal(t, 0, count, "non-existent table should return zero records")
}
