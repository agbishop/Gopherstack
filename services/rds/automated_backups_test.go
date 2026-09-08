package rds_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

func TestDeleteDBClusterAutomatedBackup(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		// Create a cluster to populate the backup
		_, err := b.CreateDBCluster(
			"cluster-bk",
			"aurora-mysql",
			"admin",
			"db",
			"",
			3306,
			nil,
			rds.DBClusterOptions{},
		)
		require.NoError(t, err)
		b.CreateDBClusterAutomatedBackup("cluster-bk")

		backups := b.DescribeDBClusterAutomatedBackups("cluster-bk")
		require.Len(t, backups, 1)

		deleted, err := b.DeleteDBClusterAutomatedBackup("cluster-bk")
		require.NoError(t, err)
		assert.Equal(t, "deleted", deleted.Status)

		backups = b.DescribeDBClusterAutomatedBackups("cluster-bk")
		assert.Empty(t, backups)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		_, err := b.DeleteDBClusterAutomatedBackup("missing")
		require.Error(t, err)
		require.ErrorIs(t, err, rds.ErrDBClusterAutomatedBackupNotFound)
	})
}

func TestDescribeDBClusterAutomatedBackups(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		backups := b.DescribeDBClusterAutomatedBackups("")
		assert.Empty(t, backups)
	})

	t.Run("lists backups", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		_, err := b.CreateDBCluster(
			"cluster-1",
			"aurora-mysql",
			"admin",
			"db",
			"",
			3306,
			nil,
			rds.DBClusterOptions{},
		)
		require.NoError(t, err)
		b.CreateDBClusterAutomatedBackup("cluster-1")

		backups := b.DescribeDBClusterAutomatedBackups("")
		require.Len(t, backups, 1)
		assert.Equal(t, "cluster-1", backups[0].DBClusterIdentifier)
		assert.Equal(t, "available", backups[0].Status)
	})

	t.Run("filter by cluster", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		for _, id := range []string{"c1", "c2"} {
			_, err := b.CreateDBCluster(
				id,
				"aurora-mysql",
				"admin",
				"db",
				"",
				3306,
				nil,
				rds.DBClusterOptions{},
			)
			require.NoError(t, err)
			b.CreateDBClusterAutomatedBackup(id)
		}

		backups := b.DescribeDBClusterAutomatedBackups("c1")
		require.Len(t, backups, 1)
		assert.Equal(t, "c1", backups[0].DBClusterIdentifier)
	})
}

func TestDeleteDBInstanceAutomatedBackup(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		// Start replication to create an automated backup record
		_, err := b.StartDBInstanceAutomatedBackupsReplication(
			"arn:aws:rds:us-east-1:123:db:src", 7,
		)
		require.NoError(t, err)

		backups := b.DescribeDBInstanceAutomatedBackups("")
		require.NotEmpty(t, backups)

		deleted, err := b.DeleteDBInstanceAutomatedBackup(backups[0].DBInstanceIdentifier)
		require.NoError(t, err)
		assert.Equal(t, "deleted", deleted.Status)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		_, err := b.DeleteDBInstanceAutomatedBackup("missing")
		require.Error(t, err)
		require.ErrorIs(t, err, rds.ErrDBInstanceAutomatedBackupNotFound)
	})
}

func TestStartDBInstanceAutomatedBackupsReplication(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		backup, err := b.StartDBInstanceAutomatedBackupsReplication(
			"arn:aws:rds:us-east-1:123:db:src", 7,
		)
		require.NoError(t, err)
		assert.Equal(t, "replicating", backup.Status)
		assert.Equal(t, 7, backup.BackupRetentionPeriod)
	})

	t.Run("empty ARN", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		_, err := b.StartDBInstanceAutomatedBackupsReplication("", 7)
		require.Error(t, err)
		require.ErrorIs(t, err, rds.ErrInvalidParameter)
	})
}

func TestStopDBInstanceAutomatedBackupsReplication(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		src := "arn:aws:rds:us-east-1:123:db:src"
		_, err := b.StartDBInstanceAutomatedBackupsReplication(src, 7)
		require.NoError(t, err)

		stopped, err := b.StopDBInstanceAutomatedBackupsReplication(src)
		require.NoError(t, err)
		assert.Equal(t, "stopped", stopped.Status)
	})

	t.Run("not running returns DBInstanceNotFound", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend(t)
		// Stop without start: the op only models DBInstanceNotFoundFault and
		// InvalidDBInstanceStateFault, so an absent replication entry errors
		// rather than fabricating a "stopped" record.
		_, err := b.StopDBInstanceAutomatedBackupsReplication(
			"arn:aws:rds:us-east-1:123:db:notfound",
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, rds.ErrInstanceNotFound)
	})
}

func TestHandler_AutomatedBackupOps(t *testing.T) {
	t.Parallel()
	h := newRDSHandler()

	// Start replication
	rec := postRDSForm(t, h,
		"Action=StartDBInstanceAutomatedBackupsReplication&Version=2014-10-31"+
			"&SourceDBInstanceArn=arn:aws:rds:us-east-1:123:db:src"+
			"&BackupRetentionPeriod=7")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "replicating")

	// Describe instance automated backups
	rec = postRDSForm(t, h, "Action=DescribeDBInstanceAutomatedBackups&Version=2014-10-31")
	assert.Equal(t, http.StatusOK, rec.Code)

	// Stop replication
	rec = postRDSForm(t, h,
		"Action=StopDBInstanceAutomatedBackupsReplication&Version=2014-10-31"+
			"&SourceDBInstanceArn=arn:aws:rds:us-east-1:123:db:src")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "stopped")

	// Describe cluster automated backups (empty)
	rec = postRDSForm(t, h, "Action=DescribeDBClusterAutomatedBackups&Version=2014-10-31")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCreateDBClusterAutomatedBackup_ResourceID(t *testing.T) {
	t.Parallel()
	b := newTestBackend(t)

	_, err := b.CreateDBCluster("cluster-1", "aurora-mysql", "admin", "db", "", 3306, nil, rds.DBClusterOptions{})
	require.NoError(t, err)
	backup := b.CreateDBClusterAutomatedBackup("cluster-1")
	require.NotNil(t, backup)
	assert.NotEmpty(t, backup.DBClusterResourceID)
	assert.Equal(t, "cluster-1", backup.DBClusterIdentifier)
}

func TestInstanceBackup_CreatedWithRetention(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBInstance(
		"backup-db",
		"postgres",
		"db.t3.micro",
		"",
		"admin",
		"",
		20,
		rds.DBInstanceOptions{
			BackupRetentionPeriod: 7,
		},
	)
	require.NoError(t, err)

	backups := b.DescribeDBInstanceAutomatedBackups("backup-db")
	require.NotEmpty(t, backups)
	assert.Equal(t, "backup-db", backups[0].DBInstanceIdentifier)
}

func TestInstanceBackup_NoRetentionNoBackup(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBInstance(
		"nobackup-db",
		"postgres",
		"db.t3.micro",
		"",
		"admin",
		"",
		20,
		rds.DBInstanceOptions{
			BackupRetentionPeriod: 0,
		},
	)
	require.NoError(t, err)

	backups := b.DescribeDBInstanceAutomatedBackups("nobackup-db")
	assert.Empty(t, backups)
}

func TestInstanceBackup_Delete(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	_, err := b.CreateDBInstance(
		"del-backup-db",
		"postgres",
		"db.t3.micro",
		"",
		"admin",
		"",
		20,
		rds.DBInstanceOptions{
			BackupRetentionPeriod: 3,
		},
	)
	require.NoError(t, err)

	backups := b.DescribeDBInstanceAutomatedBackups("del-backup-db")
	require.NotEmpty(t, backups)

	_, err = b.DeleteDBInstanceAutomatedBackup(backups[0].DbiResourceID)
	require.NoError(t, err)

	backupsAfter := b.DescribeDBInstanceAutomatedBackups("del-backup-db")
	assert.Empty(t, backupsAfter)
}

func TestInstanceBackup_Replication(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()
	sourceARN := "arn:aws:rds:us-west-2:123456789012:db:source-db"

	backup, err := b.StartDBInstanceAutomatedBackupsReplication(sourceARN, 7)
	require.NoError(t, err)
	assert.Equal(t, sourceARN, backup.DBInstanceArn)
	assert.Equal(t, 7, backup.BackupRetentionPeriod)

	stopped, err := b.StopDBInstanceAutomatedBackupsReplication(sourceARN)
	require.NoError(t, err)
	assert.Equal(t, sourceARN, stopped.DBInstanceArn)
}

func TestInstanceBackup_HTTP(t *testing.T) {
	t.Parallel()

	h := newBatch2Handler()

	rec := postRDSForm(t, h, url.Values{
		"Action":  {"DescribeDBInstanceAutomatedBackups"},
		"Version": {"2014-10-31"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":                {"StartDBInstanceAutomatedBackupsReplication"},
		"Version":               {"2014-10-31"},
		"SourceDBInstanceArn":   {"arn:aws:rds:us-west-2:123456789012:db:source-db"},
		"BackupRetentionPeriod": {"14"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = postRDSForm(t, h, url.Values{
		"Action":              {"StopDBInstanceAutomatedBackupsReplication"},
		"Version":             {"2014-10-31"},
		"SourceDBInstanceArn": {"arn:aws:rds:us-west-2:123456789012:db:source-db"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPersistence_ClusterAutomatedBackups(t *testing.T) {
	t.Parallel()

	b := newBatch2Backend()

	// Create a cluster first, which triggers an automated backup entry
	_, err := b.CreateDBCluster(
		"backup-cluster",
		"aurora-postgresql",
		"admin",
		"",
		"",
		5432,
		nil,
		rds.DBClusterOptions{},
	)
	require.NoError(t, err)
	b.CreateDBClusterAutomatedBackup("backup-cluster")

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := rds.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, b2.Restore(t.Context(), snap))

	backups := b2.DescribeDBClusterAutomatedBackups("backup-cluster")
	assert.NotEmpty(t, backups)
	assert.Equal(t, "backup-cluster", backups[0].DBClusterIdentifier)
}

func TestAutomatedBackup_DescribeClusterViaHandler(t *testing.T) {
	t.Parallel()

	b := newBatch3Backend()
	h := rds.NewHandler(b)

	_, err := b.CreateDBCluster("bkp-cluster", "aurora-postgresql", "admin", "", "", 0, nil,
		rds.DBClusterOptions{})
	require.NoError(t, err)

	rec := postRDSForm(t, h,
		"Action=DescribeDBClusterAutomatedBackups&Version=2014-10-31&DBClusterIdentifier=bkp-cluster")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DescribeDBClusterAutomatedBackupsResponse")
}

func TestAutomatedBackup_DescribeInstanceViaHandler(t *testing.T) {
	t.Parallel()

	h := newBatch3Handler()

	rec := postRDSForm(t, h,
		"Action=DescribeDBInstanceAutomatedBackups&Version=2014-10-31")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DescribeDBInstanceAutomatedBackupsResponse")
}

// Test_DeleteDBInstance_DeleteAutomatedBackups verifies that
// DeleteAutomatedBackups=false retains the instance's automated backup record
// after deletion, while the AWS default (true) removes it.
func Test_DeleteDBInstance_DeleteAutomatedBackups(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                   string
		deleteAutomatedBackups bool
		wantBackupsRemain      bool
	}{
		{name: "default true removes the automated backup", deleteAutomatedBackups: true},
		{name: "false retains the automated backup", deleteAutomatedBackups: false, wantBackupsRemain: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := rds.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreateDBInstance(
				"del-inst-backup", "postgres", "db.t3.micro", "", "admin", "",
				20, rds.DBInstanceOptions{BackupRetentionPeriod: 7},
			)
			require.NoError(t, err)
			require.Len(t, b.DescribeDBInstanceAutomatedBackups("del-inst-backup"), 1)

			_, err = b.DeleteDBInstanceWithOptions("del-inst-backup", true, "", tt.deleteAutomatedBackups)
			require.NoError(t, err)

			backups := b.DescribeDBInstanceAutomatedBackups("del-inst-backup")
			if tt.wantBackupsRemain {
				assert.Len(t, backups, 1)
			} else {
				assert.Empty(t, backups)
			}
		})
	}
}

// TestDeleteDBInstanceClearsAutomatedBackup verifies that deleting an instance removes its automated backup.
func TestDeleteDBInstanceClearsAutomatedBackup(t *testing.T) {
	t.Parallel()
	b := newTestBackend(t)
	_, err := b.CreateDBInstance("db-del", "postgres", "db.t3.micro", "", "admin", "", 20,
		rds.DBInstanceOptions{BackupRetentionPeriod: 7})
	require.NoError(t, err)
	// Verify backup was created.
	backups := b.DescribeDBInstanceAutomatedBackups("db-del")
	require.Len(t, backups, 1)
	// Delete the instance.
	_, err = b.DeleteDBInstance("db-del")
	require.NoError(t, err)
	// Backup should be removed.
	backups = b.DescribeDBInstanceAutomatedBackups("db-del")
	assert.Empty(t, backups, "automated backup should be removed when instance is deleted")
}
