package timestreamwrite

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
)

// recordGoesToMemoryStore reports whether a record should be counted as a memory-store
// write based on the table's retention and magnetic store configuration.
//
// Rules (matching the AWS API routing behaviour):
//  1. If the table has no MagneticStoreWriteProperties, or magnetic store writes are
//     disabled, all records go to memory store regardless of timestamp.
//  2. If no memory retention period is configured, all records go to memory store.
//  3. Otherwise, records whose InternalTimestamp falls within the memory retention
//     window (i.e. after the cutoff) go to memory store; older records go to magnetic store.
func recordGoesToMemoryStore(r Record, tbl *Table, now time.Time) bool {
	if tbl == nil {
		return true
	}

	if tbl.MagneticStoreWriteProperties == nil || !tbl.MagneticStoreWriteProperties.EnableMagneticStoreWrites {
		return true
	}

	if tbl.RetentionProperties == nil || tbl.RetentionProperties.MemoryStoreRetentionPeriodInHours == 0 {
		return true
	}

	retention := time.Duration(tbl.RetentionProperties.MemoryStoreRetentionPeriodInHours) * time.Hour
	cutoff := now.Add(-retention)

	return r.InternalTimestamp.After(cutoff)
}

// recordOutsideRetention reports whether ts lies outside the table's memory-store
// retention window and the table has no magnetic store write path to receive it.
// Per RejectedRecordsException (types/errors.go, timestreamwrite@v1.38.4), "Records
// with timestamps that lie outside the retention duration of the memory store" are
// rejected -- unless EnableMagneticStoreWrites lets them land in the magnetic store
// instead (recordGoesToMemoryStore routes those there rather than rejecting).
func recordOutsideRetention(ts time.Time, tbl *Table, now time.Time) bool {
	if tbl == nil || tbl.RetentionProperties == nil || tbl.RetentionProperties.MemoryStoreRetentionPeriodInHours == 0 {
		return false
	}

	if tbl.MagneticStoreWriteProperties != nil && tbl.MagneticStoreWriteProperties.EnableMagneticStoreWrites {
		return false
	}

	retention := time.Duration(tbl.RetentionProperties.MemoryStoreRetentionPeriodInHours) * time.Hour
	cutoff := now.Add(-retention)

	return !ts.After(cutoff)
}

// recordKey computes a deterministic dedup key for a record using measure name,
// time, time unit, and sorted dimension name=value pairs.
func recordKey(r Record) string {
	dims := make([]string, 0, len(r.Dimensions))
	for _, d := range r.Dimensions {
		dims = append(dims, d.Name+"="+d.Value)
	}

	sort.Strings(dims)

	return strings.Join([]string{r.MeasureName, r.Time, r.TimeUnit, strings.Join(dims, ",")}, "\x00")
}

// WriteRecords appends records to the specified table.
//
// Lock ordering: global RLock first, then per-table WLock on the *tableRecords
// slot. The global read lock prevents structural changes
// (CreateTable/DeleteTable/CreateDatabase/DeleteDatabase) from racing with
// writes; the slot's write lock serialises concurrent writes to the same
// table while allowing writes to different tables to proceed in parallel.
//
// Records are mutated through the slot pointer (slot.records = append(...))
// rather than the enclosing map, so two writers in different tables of the
// same database never write to the b.records[dbName] map concurrently.
func (b *InMemoryBackend) WriteRecords(dbName, tblName string, records []Record) (*WriteRecordsOutput, error) {
	b.mu.RLock("WriteRecords")
	defer b.mu.RUnlock()

	if !b.databases.Has(dbName) {
		return nil, fmt.Errorf("%w: database %s not found", ErrDatabaseNotFound, dbName)
	}

	slot, ok := b.records[dbName][tblName]
	if !ok {
		return nil, fmt.Errorf("%w: table %s not found", ErrTableNotFound, tblName)
	}

	tbl, _ := b.tables.Get(tableKey(dbName, tblName))

	slot.mu.Lock("WriteRecords")
	defer slot.mu.Unlock()

	if slot.recordIndex == nil {
		slot.recordIndex = make(map[string]int)
	}

	rejected, memoryInserted, magneticInserted := writeRecordsIntoSlot(slot, records, tbl, time.Now().UTC())

	if len(rejected) > 0 {
		return nil, &RejectedRecordsError{RejectedRecords: rejected}
	}

	total := memoryInserted + magneticInserted

	// Record counts are bounded by request size limits (< MaxInt32).
	return &WriteRecordsOutput{ //#nosec G115
		Total:         total,
		MemoryStore:   memoryInserted,
		MagneticStore: magneticInserted,
	}, nil
}

// writeRecordsIntoSlot processes records into a slot, returning rejected records and store counts.
func writeRecordsIntoSlot(
	slot *tableRecords, records []Record, tbl *Table, now time.Time,
) ([]RejectedRecord, int32, int32) {
	var rejected []RejectedRecord

	var memoryInserted, magneticInserted int32

	for i, r := range records {
		ts := parseTimestreamTime(r.Time, r.TimeUnit)

		if recordOutsideRetention(ts, tbl, now) {
			rejected = append(rejected, RejectedRecord{
				RecordIndex: i,
				Reason: fmt.Sprintf(
					"The record timestamp is outside the retention period. "+
						"Current retention period for memory store is %d hours",
					tbl.RetentionProperties.MemoryStoreRetentionPeriodInHours,
				),
			})

			continue
		}

		key := recordKey(r)

		newVersion := r.Version
		if newVersion == 0 {
			newVersion = 1
		}

		if idx, exists := slot.recordIndex[key]; exists {
			mem, mag, rej := upsertRecord(slot, idx, i, r, newVersion, tbl, now)
			memoryInserted += mem
			magneticInserted += mag
			if rej != nil {
				rejected = append(rejected, *rej)
			}
		} else {
			mem, mag := insertRecord(slot, r, newVersion, tbl, now)
			memoryInserted += mem
			magneticInserted += mag
		}
	}

	return rejected, memoryInserted, magneticInserted
}

// upsertRecord updates an existing record if the new version is higher, or rejects it.
func upsertRecord(
	slot *tableRecords, idx, recIdx int, r Record, newVersion int64, tbl *Table, now time.Time,
) (int32, int32, *RejectedRecord) {
	existingVersion := slot.records[idx].Version
	if existingVersion == 0 {
		existingVersion = 1
	}

	if newVersion <= existingVersion {
		return 0, 0, &RejectedRecord{
			RecordIndex:     recIdx,
			Reason:          "Record with same dimensions, time and measure name already exists with same or higher version",
			ExistingVersion: existingVersion,
		}
	}

	cp := r
	cp.Version = newVersion
	cp.InternalTimestamp = parseTimestreamTime(r.Time, r.TimeUnit)
	slot.records[idx] = cp

	if recordGoesToMemoryStore(cp, tbl, now) {
		return 1, 0, nil
	}

	return 0, 1, nil
}

// insertRecord appends a new record to the slot and returns store routing counts.
func insertRecord(slot *tableRecords, r Record, newVersion int64, tbl *Table, now time.Time) (int32, int32) {
	cp := r
	cp.Version = newVersion
	cp.InternalTimestamp = parseTimestreamTime(r.Time, r.TimeUnit)
	slot.recordIndex[recordKey(r)] = len(slot.records)
	slot.records = append(slot.records, cp)

	if recordGoesToMemoryStore(cp, tbl, now) {
		return 1, 0
	}

	return 0, 1
}

func parseTimestreamTime(ts, unit string) time.Time {
	val, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return time.Now().UTC()
	}

	switch strings.ToUpper(unit) {
	case "SECONDS":
		return time.Unix(val, 0).UTC()
	case "MILLISECONDS":
		return time.UnixMilli(val).UTC()
	case "MICROSECONDS":
		return time.UnixMicro(val).UTC()
	case "NANOSECONDS":
		return time.Unix(0, val).UTC()
	default:
		return time.UnixMilli(val).UTC()
	}
}

// SweepRetention prunes records that exceed the memory store retention period.
func (b *InMemoryBackend) SweepRetention(ctx context.Context) {
	b.mu.Lock("SweepRetention")
	defer b.mu.Unlock()

	now := time.Now().UTC()
	totalPruned := 0

	for _, tbl := range b.tables.All() {
		totalPruned += b.pruneTableRecords(tbl.DatabaseName, tbl.TableName, tbl, now)
	}

	if totalPruned > 0 {
		telemetry.RecordWorkerItems("timestreamwrite", "RetentionSweeper", totalPruned)
		logger.Load(ctx).InfoContext(ctx, "Timestream janitor: expired records pruned", "count", totalPruned)
	}

	telemetry.RecordWorkerTask("timestreamwrite", "RetentionSweeper", "success")
}

// pruneTableRecords drops records older than the table's memory-store retention
// window. Returns the number of records removed. Caller must hold b.mu in write
// mode. Returns 0 (no-op) when the table has no retention configured or no slot.
func (b *InMemoryBackend) pruneTableRecords(dbName, tblName string, tbl *Table, now time.Time) int {
	if tbl.RetentionProperties == nil || tbl.RetentionProperties.MemoryStoreRetentionPeriodInHours == 0 {
		return 0
	}

	slot := b.records[dbName][tblName]
	if slot == nil {
		return 0
	}

	retention := time.Duration(tbl.RetentionProperties.MemoryStoreRetentionPeriodInHours) * time.Hour
	cutoff := now.Add(-retention)

	newRecords := make([]Record, 0, len(slot.records))
	for _, r := range slot.records {
		if r.InternalTimestamp.After(cutoff) {
			newRecords = append(newRecords, r)
		}
	}

	pruned := len(slot.records) - len(newRecords)
	if pruned > 0 {
		slot.records = newRecords
		// Rebuild the dedup index after pruning to keep offsets consistent.
		slot.recordIndex = make(map[string]int, len(newRecords))
		for i, r := range slot.records {
			slot.recordIndex[recordKey(r)] = i
		}
	}

	return pruned
}
