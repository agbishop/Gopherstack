package timestreamwrite_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
	"github.com/blackbirdworks/gopherstack/services/timestreamwrite"
)

func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend (including the pre-Phase-3.3
// format, which decodes with Version == 0) is discarded cleanly rather than
// partially decoded: the backend resets to empty state and Restore returns
// no error.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("seed-db", "", nil)
	require.NoError(t, err)

	// A syntactically valid but version-less/mismatched snapshot.
	err = b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.Equal(t, 0, timestreamwrite.DatabaseCount(b))
	assert.Equal(t, 0, timestreamwrite.TableCount(b))
	assert.Equal(t, 0, timestreamwrite.BatchLoadTaskCount(b))
	assert.Equal(t, 0, timestreamwrite.TagCount(b))
}

// TestInMemoryBackend_SnapshotRestore_FullState exercises a Snapshot->Restore
// round trip across every resource family the Phase 3.3 pkgs/store
// conversion touched: the three store.Table-backed collections (databases,
// tables -- flattened from the old nested map via the composite
// "database|table" key plus its byDatabase index -- and batchLoadTasks),
// plus every plain map left un-converted (records, tags) and the nextTaskID
// counter.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := timestreamwrite.NewInMemoryBackend()

	db1Created, err := original.CreateDatabase("db1", "", map[string]string{"env": "test"})
	require.NoError(t, err)
	_, err = original.CreateDatabase("db2", "", nil)
	require.NoError(t, err)

	_, err = original.CreateTable("db1", "tbl1", map[string]string{"team": "obs"}, &timestreamwrite.CreateTableInput{
		RetentionProperties: &timestreamwrite.RetentionProperties{
			MemoryStoreRetentionPeriodInHours:  12,
			MagneticStoreRetentionPeriodInDays: 30,
		},
	})
	require.NoError(t, err)

	_, err = original.CreateTable("db1", "tbl2", nil, nil)
	require.NoError(t, err)

	_, err = original.CreateTable("db2", "tbl3", nil, nil)
	require.NoError(t, err)

	_, err = original.WriteRecords("db1", "tbl1", []timestreamwrite.Record{
		{
			MeasureName:      "cpu",
			MeasureValue:     "42.5",
			MeasureValueType: "DOUBLE",
			Time:             recentTimeSeconds(),
			TimeUnit:         "SECONDS",
			Dimensions:       []timestreamwrite.Dimension{{Name: "host", Value: "a"}},
		},
	})
	require.NoError(t, err)

	require.NoError(t, original.TagResource(db1Created.ARN, map[string]string{"k": "v"}))

	task1, err := original.CreateBatchLoadTask("db1", "tbl1", nil, nil, nil, 0)
	require.NoError(t, err)

	snap, err := original.Snapshot()
	require.NoError(t, err)
	require.NotNil(t, snap)

	fresh := timestreamwrite.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	db1, err := fresh.DescribeDatabase("db1")
	require.NoError(t, err)
	assert.Equal(t, 2, db1.TableCount)

	_, err = fresh.DescribeDatabase("db2")
	require.NoError(t, err)

	tbl1, err := fresh.DescribeTable("db1", "tbl1")
	require.NoError(t, err)
	require.NotNil(t, tbl1.RetentionProperties)
	assert.Equal(t, int64(12), tbl1.RetentionProperties.MemoryStoreRetentionPeriodInHours)

	tables, err := fresh.ListTables("db1")
	require.NoError(t, err)
	require.Len(t, tables, 2)

	tables2, err := fresh.ListTables("db2")
	require.NoError(t, err)
	require.Len(t, tables2, 1)
	assert.Equal(t, "tbl3", tables2[0].TableName)

	assert.Equal(t, 1, timestreamwrite.RecordCount(fresh, "db1", "tbl1"))

	tags := fresh.ListTagsForResource(db1Created.ARN)
	assert.Equal(t, "v", tags["k"])

	task, err := fresh.DescribeBatchLoadTask(task1.TaskID)
	require.NoError(t, err)
	assert.Equal(t, "db1", task.TargetDatabaseName)

	// nextTaskID must have survived the round trip so a post-restore task ID
	// continues the sequence instead of colliding with task1's.
	task2, err := fresh.CreateBatchLoadTask("db1", "tbl1", nil, nil, nil, 0)
	require.NoError(t, err)
	assert.NotEqual(t, task1.TaskID, task2.TaskID)

	// Writing a new, higher-version record after restore must still exercise
	// the per-table dedup index correctly (rebuilt fresh by Restore).
	_, err = fresh.WriteRecords("db1", "tbl1", []timestreamwrite.Record{
		{
			MeasureName:      "cpu",
			MeasureValue:     "43.1",
			MeasureValueType: "DOUBLE",
			Time:             recentTimeSeconds(),
			TimeUnit:         "SECONDS",
			Dimensions:       []timestreamwrite.Dimension{{Name: "host", Value: "a"}},
			Version:          2,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, timestreamwrite.RecordCount(fresh, "db1", "tbl1"))
}

// TestInMemoryBackend_SnapshotRestore_EmptyState verifies that a backend with
// no resources at all round-trips cleanly (every store.Table and raw map
// left empty, not nil).
func TestInMemoryBackend_SnapshotRestore_EmptyState(t *testing.T) {
	t.Parallel()

	original := timestreamwrite.NewInMemoryBackend()

	snap, err := original.Snapshot()
	require.NoError(t, err)
	require.NotNil(t, snap)

	fresh := timestreamwrite.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	assert.Equal(t, 0, timestreamwrite.DatabaseCount(fresh))
	assert.Equal(t, 0, timestreamwrite.TableCount(fresh))
	assert.Equal(t, 0, timestreamwrite.BatchLoadTaskCount(fresh))
	assert.Equal(t, 0, timestreamwrite.TagCount(fresh))

	_, err = fresh.CreateDatabase("db", "", nil)
	require.NoError(t, err)
}

// Test_Handler_SnapshotRestore verifies Handler.Snapshot/Restore
// (persistence.go) delegate to the backend -- the shape persistence.Manager
// actually drives. cli.go's setupPersistence registers a service.Registerable
// (the *Handler returned by Provider.Init) in the persistence.Manager only if
// that Handler itself satisfies Snapshot(ctx)/Restore(ctx, []byte);
// InMemoryBackend implementing Snapshot()([]byte,error)/Restore(ctx,[]byte)error
// is not enough on its own, since Handler.Backend's methods are never
// promoted (Backend is a named field, not embedded), and the shapes differ
// (no ctx, returns an error) so Handler.Snapshot must adapt. Mirrors
// services/securityhub's Test_Handler_SnapshotRestore.
func Test_Handler_SnapshotRestore(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	backend := timestreamwrite.NewInMemoryBackend()
	h := timestreamwrite.NewHandler(backend)

	// Compile-time proof Handler satisfies the persistence layer's contract.
	var _ persistence.Persistable = h

	_, err := backend.CreateDatabase("handler-db", "", map[string]string{"env": "test"})
	require.NoError(t, err)

	data := h.Snapshot(ctx)
	require.NotEmpty(t, data)

	restoredBackend := timestreamwrite.NewInMemoryBackend()
	restoredHandler := timestreamwrite.NewHandler(restoredBackend)
	require.NoError(t, restoredHandler.Restore(ctx, data))

	assert.Equal(t, 1, timestreamwrite.DatabaseCount(restoredBackend))

	db, err := restoredBackend.DescribeDatabase("handler-db")
	require.NoError(t, err)
	assert.Equal(t, "handler-db", db.DatabaseName)
}

// TestInMemoryBackend_SnapshotRestore_BasicRoundTrip verifies a basic
// Snapshot/Restore round trip across a database, table, and batch load task.
func TestInMemoryBackend_SnapshotRestore_BasicRoundTrip(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	_, err := b.CreateDatabase("snap-db", "", map[string]string{"key": "value"})
	require.NoError(t, err)
	_, err = b.CreateTable("snap-db", "snap-tbl", nil, nil)
	require.NoError(t, err)
	_, err = b.CreateBatchLoadTask("snap-db", "snap-tbl", nil, nil, nil, 0)
	require.NoError(t, err)

	data, err := b.Snapshot()
	require.NoError(t, err)
	require.NotEmpty(t, data)

	b2 := timestreamwrite.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), data))

	assert.Equal(t, 1, timestreamwrite.DatabaseCount(b2))
	assert.Equal(t, 1, timestreamwrite.TableCount(b2))
	assert.Equal(t, 1, timestreamwrite.BatchLoadTaskCount(b2))

	db, err := b2.DescribeDatabase("snap-db")
	require.NoError(t, err)
	assert.Equal(t, "snap-db", db.DatabaseName)
}

// TestInMemoryBackend_SnapshotRestore_PreservesSchema verifies Schema
// survives snapshot/restore.
func TestInMemoryBackend_SnapshotRestore_PreservesSchema(t *testing.T) {
	t.Parallel()

	b1 := timestreamwrite.NewInMemoryBackend()
	_, err := b1.CreateDatabase("snap-schema-db", "", nil)
	require.NoError(t, err)
	_, err = b1.CreateTable("snap-schema-db", "snap-schema-tbl", nil, &timestreamwrite.CreateTableInput{
		Schema: &timestreamwrite.Schema{
			CompositePartitionKey: []timestreamwrite.PartitionKey{
				{Type: "DIMENSION", Name: "az", EnforcementInRecord: "REQUIRED"},
			},
		},
	})
	require.NoError(t, err)

	data, err := b1.Snapshot()
	require.NoError(t, err)

	b2 := timestreamwrite.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), data))

	tbl, err := b2.DescribeTable("snap-schema-db", "snap-schema-tbl")
	require.NoError(t, err)
	require.NotNil(t, tbl.Schema)
	require.Len(t, tbl.Schema.CompositePartitionKey, 1)
	assert.Equal(t, "az", tbl.Schema.CompositePartitionKey[0].Name)
}

// TestInMemoryBackend_SnapshotRestore_PreservesMagneticStoreRejectedDataLocation
// verifies MagneticStoreRejectedDataLocation survives snapshot/restore.
func TestInMemoryBackend_SnapshotRestore_PreservesMagneticStoreRejectedDataLocation(t *testing.T) {
	t.Parallel()

	b1 := timestreamwrite.NewInMemoryBackend()
	_, err := b1.CreateDatabase("snap-msrdl-db", "", nil)
	require.NoError(t, err)
	_, err = b1.CreateTable("snap-msrdl-db", "snap-msrdl-tbl", nil, &timestreamwrite.CreateTableInput{
		MagneticStoreWriteProperties: &timestreamwrite.MagneticStoreWriteProperties{
			EnableMagneticStoreWrites: true,
			MagneticStoreRejectedDataLocation: &timestreamwrite.MagneticStoreRejectedDataLocation{
				S3Configuration: &timestreamwrite.S3Configuration{
					BucketName: "snap-bucket",
				},
			},
		},
	})
	require.NoError(t, err)

	data, err := b1.Snapshot()
	require.NoError(t, err)

	b2 := timestreamwrite.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), data))

	tbl, err := b2.DescribeTable("snap-msrdl-db", "snap-msrdl-tbl")
	require.NoError(t, err)
	require.NotNil(t, tbl.MagneticStoreWriteProperties)
	require.NotNil(t, tbl.MagneticStoreWriteProperties.MagneticStoreRejectedDataLocation)
	require.NotNil(t, tbl.MagneticStoreWriteProperties.MagneticStoreRejectedDataLocation.S3Configuration)
	assert.Equal(t, "snap-bucket",
		tbl.MagneticStoreWriteProperties.MagneticStoreRejectedDataLocation.S3Configuration.BucketName)
}

// TestInMemoryBackend_SnapshotRestore_PreservesRecordIndex verifies that
// after restore, the version-based upsert index works correctly (version
// conflicts are detected).
func TestInMemoryBackend_SnapshotRestore_PreservesRecordIndex(t *testing.T) {
	t.Parallel()

	b1 := timestreamwrite.NewInMemoryBackend()
	_, err := b1.CreateDatabase("snap-idx-db", "", nil)
	require.NoError(t, err)
	_, err = b1.CreateTable("snap-idx-db", "snap-idx-tbl", nil, nil)
	require.NoError(t, err)

	rec := timestreamwrite.Record{
		MeasureName: "metric", MeasureValue: "7", MeasureValueType: "BIGINT",
		Time: recentTimeMillis(0), TimeUnit: "MILLISECONDS", Version: 4,
	}
	_, err = b1.WriteRecords("snap-idx-db", "snap-idx-tbl", []timestreamwrite.Record{rec})
	require.NoError(t, err)

	data, err := b1.Snapshot()
	require.NoError(t, err)

	b2 := timestreamwrite.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), data))

	// After restore, writing with lower version should be rejected.
	rec.Version = 2
	_, err = b2.WriteRecords("snap-idx-db", "snap-idx-tbl", []timestreamwrite.Record{rec})
	require.Error(t, err)

	var rejErr *timestreamwrite.RejectedRecordsError
	require.ErrorAs(t, err, &rejErr)
	assert.Equal(t, int64(4), rejErr.RejectedRecords[0].ExistingVersion)
}

// TestInMemoryBackend_SnapshotRestore_PreservesKmsKeyID verifies that a
// database's KmsKeyId survives a snapshot/restore cycle.
func TestInMemoryBackend_SnapshotRestore_PreservesKmsKeyID(t *testing.T) {
	t.Parallel()

	b1 := timestreamwrite.NewInMemoryBackend()
	_, err := b1.CreateDatabase("snap-kms-db", "", nil)
	require.NoError(t, err)

	kmsKey := "arn:aws:kms:us-east-1:000000000000:key/snap-key"
	_, err = b1.UpdateDatabase("snap-kms-db", kmsKey)
	require.NoError(t, err)

	data, err := b1.Snapshot()
	require.NoError(t, err)

	b2 := timestreamwrite.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), data))

	db, err := b2.DescribeDatabase("snap-kms-db")
	require.NoError(t, err)
	assert.Equal(t, kmsKey, db.KmsKeyID, "KmsKeyId should survive snapshot/restore")
}

// TestInMemoryBackend_SnapshotRestore_PreservesBatchLoadTaskProgressReport
// verifies that a BatchLoadTask's ProgressReport survives a snapshot/restore
// cycle.
func TestInMemoryBackend_SnapshotRestore_PreservesBatchLoadTaskProgressReport(t *testing.T) {
	t.Parallel()

	b1 := timestreamwrite.NewInMemoryBackend()
	now := time.Now()

	pr := &timestreamwrite.BatchLoadProgressReport{
		RecordsProcessed:        2000,
		RecordsIngested:         1950,
		RecordIngestionFailures: 50,
		FileFailures:            2,
		BytesMetered:            1024 * 512,
	}

	b1.AddBatchLoadTaskInternal(&timestreamwrite.BatchLoadTask{
		TaskID:             "snap-pr-task",
		TargetDatabaseName: "snap-pr-db",
		TargetTableName:    "snap-pr-tbl",
		TaskStatus:         timestreamwrite.BatchLoadStatusInProgress,
		CreationTime:       now,
		LastUpdatedTime:    now,
		ProgressReport:     pr,
	})

	data, err := b1.Snapshot()
	require.NoError(t, err)

	b2 := timestreamwrite.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), data))

	task, err := b2.DescribeBatchLoadTask("snap-pr-task")
	require.NoError(t, err)
	require.NotNil(t, task.ProgressReport, "ProgressReport should survive snapshot/restore")
	assert.Equal(t, int64(2000), task.ProgressReport.RecordsProcessed)
	assert.Equal(t, int64(1950), task.ProgressReport.RecordsIngested)
	assert.Equal(t, int64(50), task.ProgressReport.RecordIngestionFailures)
}

// TestInMemoryBackend_SnapshotRestore_PreservesRetentionProperties verifies
// that table RetentionProperties survive a snapshot/restore cycle.
func TestInMemoryBackend_SnapshotRestore_PreservesRetentionProperties(t *testing.T) {
	t.Parallel()

	b1 := timestreamwrite.NewInMemoryBackend()
	_, err := b1.CreateDatabase("snap-rp-db", "", nil)
	require.NoError(t, err)
	_, err = b1.CreateTable("snap-rp-db", "snap-rp-tbl", nil, &timestreamwrite.CreateTableInput{
		RetentionProperties: &timestreamwrite.RetentionProperties{
			MemoryStoreRetentionPeriodInHours:  72,
			MagneticStoreRetentionPeriodInDays: 400,
		},
	})
	require.NoError(t, err)

	data, err := b1.Snapshot()
	require.NoError(t, err)

	b2 := timestreamwrite.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), data))

	tbl, err := b2.DescribeTable("snap-rp-db", "snap-rp-tbl")
	require.NoError(t, err)
	require.NotNil(t, tbl.RetentionProperties)
	assert.Equal(t, int64(72), tbl.RetentionProperties.MemoryStoreRetentionPeriodInHours)
	assert.Equal(t, int64(400), tbl.RetentionProperties.MagneticStoreRetentionPeriodInDays)
}

// TestInMemoryBackend_SnapshotRestore_PreservesDataSourceConfig verifies that
// BatchLoadTask DataSourceConfiguration survives a snapshot/restore cycle.
func TestInMemoryBackend_SnapshotRestore_PreservesDataSourceConfig(t *testing.T) {
	t.Parallel()

	b1 := timestreamwrite.NewInMemoryBackend()
	_, err := b1.CreateDatabase("snap-blt-db", "", nil)
	require.NoError(t, err)
	_, err = b1.CreateTable("snap-blt-db", "snap-blt-tbl", nil, nil)
	require.NoError(t, err)

	_, err = b1.CreateBatchLoadTask(
		"snap-blt-db", "snap-blt-tbl",
		&timestreamwrite.DataSourceConfiguration{
			DataFormat: "CSV",
			DataSourceS3Configuration: &timestreamwrite.DataSourceS3Configuration{
				BucketName: "snap-bucket",
			},
		},
		nil,
		&timestreamwrite.DataModelConfiguration{
			DataModel: &timestreamwrite.DataModel{
				TimeColumn: "time",
				TimeUnit:   "MILLISECONDS",
				DimensionMappings: []timestreamwrite.DimensionMapping{
					{SourceColumn: "region", DestinationColumn: "region"},
				},
			},
		},
		7,
	)
	require.NoError(t, err)

	data, err := b1.Snapshot()
	require.NoError(t, err)

	b2 := timestreamwrite.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), data))

	tasks := b2.ListBatchLoadTasks("")
	require.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].DataSourceConfiguration)
	require.NotNil(t, tasks[0].DataSourceConfiguration.DataSourceS3Configuration)
	assert.Equal(t, "snap-bucket", tasks[0].DataSourceConfiguration.DataSourceS3Configuration.BucketName)
	require.NotNil(t, tasks[0].DataModelConfiguration)
	require.NotNil(t, tasks[0].DataModelConfiguration.DataModel)
	assert.Equal(t, "time", tasks[0].DataModelConfiguration.DataModel.TimeColumn)
	require.Len(t, tasks[0].DataModelConfiguration.DataModel.DimensionMappings, 1)
	assert.Equal(t, "region", tasks[0].DataModelConfiguration.DataModel.DimensionMappings[0].SourceColumn)
	assert.Equal(t, int64(7), tasks[0].RecordVersion)
}
