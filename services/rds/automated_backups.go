package rds

import (
	"fmt"
	"slices"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// maybeRegisterAutomatedBackup registers an automated backup entry if retention > 0.
func (b *InMemoryBackend) maybeRegisterAutomatedBackup(
	id, engine string,
	port, allocatedStorage int,
	opts DBInstanceOptions,
) {
	if opts.BackupRetentionPeriod <= 0 {
		return
	}

	b.automatedBackups[id] = &DBInstanceAutomatedBackup{
		DBInstanceIdentifier:  id,
		DbiResourceID:         id,
		Engine:                engine,
		EngineVersion:         opts.EngineVersion,
		DBInstanceArn:         arn.Build("rds", b.region, b.accountID, fmt.Sprintf("db:%s", id)),
		Region:                b.region,
		Status:                instanceStatusAvailable,
		AllocatedStorage:      allocatedStorage,
		Port:                  port,
		BackupRetentionPeriod: opts.BackupRetentionPeriod,
		Encrypted:             opts.StorageEncrypted,
	}
}

// DescribeDBInstanceAutomatedBackups returns automated backup records for instances.
// If instanceID is non-empty, filters to that instance.
func (b *InMemoryBackend) DescribeDBInstanceAutomatedBackups(instanceID string) []DBInstanceAutomatedBackup {
	b.mu.RLock("DescribeDBInstanceAutomatedBackups")
	defer b.mu.RUnlock()

	result := make([]DBInstanceAutomatedBackup, 0, len(b.automatedBackups))
	for _, ab := range b.automatedBackups {
		// DBInstanceIdentifier is a case-insensitive AWS identifier (see
		// normalizeID); automatedBackups is a plain, unnormalized map, so
		// this filter must fold case itself.
		if instanceID != "" && !idEqual(ab.DBInstanceIdentifier, instanceID) {
			continue
		}
		result = append(result, *ab)
	}

	return result
}

// CreateDBClusterAutomatedBackup records an automated backup for a cluster.
// Called internally when creating clusters with backup retention > 0.
func (b *InMemoryBackend) CreateDBClusterAutomatedBackup(
	clusterID string,
) *DBClusterAutomatedBackup {
	b.mu.Lock("CreateDBClusterAutomatedBackup")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(normalizeID(clusterID))
	if !exists {
		return nil
	}

	backup := &DBClusterAutomatedBackup{
		DBClusterIdentifier: cluster.DBClusterIdentifier,
		DBClusterResourceID: fmt.Sprintf("cluster-%s", cluster.DBClusterIdentifier),
		Engine:              cluster.Engine,
		EngineVersion:       cluster.EngineVersion,
		Region:              b.region,
		Status:              clusterBackupStatusAvailable,
		StorageEncrypted:    cluster.StorageEncrypted,
	}
	b.clusterAutomatedBackups.Put(backup)

	return backup
}

// DeleteDBClusterAutomatedBackup deletes a cluster automated backup by resource ID.
func (b *InMemoryBackend) DeleteDBClusterAutomatedBackup(
	resourceID string,
) (*DBClusterAutomatedBackup, error) {
	b.mu.Lock("DeleteDBClusterAutomatedBackup")
	defer b.mu.Unlock()

	for _, backup := range b.clusterAutomatedBackups.All() {
		if backup.DBClusterResourceID == resourceID || idEqual(backup.DBClusterIdentifier, resourceID) {
			cp := *backup
			cp.Status = clusterBackupStatusDeleted
			b.clusterAutomatedBackups.Delete(clusterAutomatedBackupsKeyFn(backup))

			return &cp, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrDBClusterAutomatedBackupNotFound, resourceID)
}

// DescribeDBClusterAutomatedBackups lists cluster automated backups, optionally filtered.
func (b *InMemoryBackend) DescribeDBClusterAutomatedBackups(
	clusterID string,
) []DBClusterAutomatedBackup {
	b.mu.RLock("DescribeDBClusterAutomatedBackups")
	defer b.mu.RUnlock()

	result := make([]DBClusterAutomatedBackup, 0, b.clusterAutomatedBackups.Len())
	for _, backup := range b.clusterAutomatedBackups.All() {
		if clusterID != "" && !idEqual(backup.DBClusterIdentifier, clusterID) {
			continue
		}
		result = append(result, *backup)
	}

	slices.SortFunc(result, func(a, b DBClusterAutomatedBackup) int {
		if a.DBClusterIdentifier < b.DBClusterIdentifier {
			return -1
		}
		if a.DBClusterIdentifier > b.DBClusterIdentifier {
			return 1
		}

		return 0
	})

	return result
}

// DeleteDBInstanceAutomatedBackup marks an automated backup as deleted.
func (b *InMemoryBackend) DeleteDBInstanceAutomatedBackup(
	resourceID string,
) (*DBInstanceAutomatedBackup, error) {
	b.mu.Lock("DeleteDBInstanceAutomatedBackup")
	defer b.mu.Unlock()

	for key, backup := range b.automatedBackups {
		if backup.DbiResourceID == resourceID || idEqual(backup.DBInstanceIdentifier, resourceID) {
			cp := *backup
			cp.Status = clusterBackupStatusDeleted
			delete(b.automatedBackups, key)

			return &cp, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrDBInstanceAutomatedBackupNotFound, resourceID)
}

// StartDBInstanceAutomatedBackupsReplication starts cross-region replication for an instance backup.
func (b *InMemoryBackend) StartDBInstanceAutomatedBackupsReplication(
	sourceInstanceARN string,
	backupRetentionPeriod int,
) (*DBInstanceAutomatedBackup, error) {
	b.mu.Lock("StartDBInstanceAutomatedBackupsReplication")
	defer b.mu.Unlock()

	if sourceInstanceARN == "" {
		return nil, fmt.Errorf("%w: SourceDBInstanceArn is required", ErrInvalidParameter)
	}

	// Derive a simple ID from the ARN for keying the backup record.
	instanceID := sourceInstanceARN
	key := "repl-" + instanceID

	backup := &DBInstanceAutomatedBackup{
		DBInstanceArn:         sourceInstanceARN,
		DBInstanceIdentifier:  instanceID,
		DbiResourceID:         fmt.Sprintf("db-%s", instanceID),
		Region:                b.region,
		Status:                instanceBackupStatusReplicating,
		BackupRetentionPeriod: backupRetentionPeriod,
	}
	b.automatedBackups[key] = backup
	cp := *backup

	return &cp, nil
}

// StopDBInstanceAutomatedBackupsReplication stops cross-region replication for an instance backup.
func (b *InMemoryBackend) StopDBInstanceAutomatedBackupsReplication(
	sourceInstanceARN string,
) (*DBInstanceAutomatedBackup, error) {
	b.mu.Lock("StopDBInstanceAutomatedBackupsReplication")
	defer b.mu.Unlock()

	for key, backup := range b.automatedBackups {
		if backup.DBInstanceArn == sourceInstanceARN ||
			backup.DBInstanceIdentifier == sourceInstanceARN {
			cp := *backup
			cp.Status = instanceStatusStopped
			delete(b.automatedBackups, key)

			return &cp, nil
		}
	}

	// StopDBInstanceAutomatedBackupsReplication only models
	// DBInstanceNotFoundFault and InvalidDBInstanceStateFault (rds
	// deserializers.go awsAwsquery_deserializeOpErrorStopDBInstanceAutomatedBackupsReplication);
	// there's no dedicated "replication not active" fault, so a source ARN
	// with no replication entry is DBInstanceNotFound, not a fabricated success.
	return nil, fmt.Errorf("%w: %s", ErrInstanceNotFound, sourceInstanceARN)
}

const (
	clusterBackupStatusAvailable = instanceStatusAvailable
	clusterBackupStatusDeleted   = "deleted"

	instanceBackupStatusReplicating = "replicating"
	instanceBackupStatusRetained    = "retained"
)
