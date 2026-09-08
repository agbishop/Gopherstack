package rds

import "fmt"

// StartActivityStream starts the database activity stream for a DB cluster.
func (b *InMemoryBackend) StartActivityStream(clusterID, kmsKeyID, mode string) (*DBCluster, error) {
	b.mu.Lock("StartActivityStream")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(normalizeID(clusterID))
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}

	if cluster.ActivityStreamStatus == activityStreamStatusStarted {
		return nil, fmt.Errorf(
			"%w: already started for cluster %s",
			ErrActivityStreamAlreadyStarted,
			clusterID,
		)
	}

	if mode == "" {
		mode = "async"
	}
	cluster.ActivityStreamStatus = activityStreamStatusStarted
	cluster.ActivityStreamMode = mode
	cluster.ActivityStreamKMSKeyID = kmsKeyID
	cluster.ActivityStreamKinesisStreamName = fmt.Sprintf("aws-rds-das-%s-%s", b.region, cluster.DBClusterIdentifier)

	return cluster, nil
}

// StopActivityStream stops the database activity stream for a DB cluster.
func (b *InMemoryBackend) StopActivityStream(clusterID string) (*DBCluster, error) {
	b.mu.Lock("StopActivityStream")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(normalizeID(clusterID))
	if !exists {
		return nil, fmt.Errorf("%w: cluster %s not found", ErrClusterNotFound, clusterID)
	}

	if cluster.ActivityStreamStatus != activityStreamStatusStarted {
		return nil, fmt.Errorf(
			"%w: activity stream is not started for cluster %s",
			ErrActivityStreamNotStarted,
			clusterID,
		)
	}

	cluster.ActivityStreamStatus = "stopped"
	cluster.ActivityStreamMode = ""
	cluster.ActivityStreamKMSKeyID = ""
	cluster.ActivityStreamKinesisStreamName = ""

	return cluster, nil
}

// ModifyActivityStream modifies activity stream settings for a DB cluster.
//
// Real ModifyActivityStream (unlike Start/StopActivityStream) is documented
// as "supported for RDS for Oracle and Microsoft SQL Server" only, and its
// ResourceArn field doc names a DB instance ARN, not a cluster ARN
// (api_op_ModifyActivityStream.go). Its declared error set
// (deserializers.go awsAwsquery_deserializeOpErrorModifyActivityStream) has
// no DBClusterNotFoundFault or InvalidDBClusterStateFault at all -- only
// DBInstanceNotFound/InvalidDBInstanceState/ResourceNotFoundFault -- so a
// not-found ARN or a not-started conflict emits those codes instead of the
// cluster-shaped ones Start/StopActivityStream declare (gopherstack-fm1e,
// gopherstack-74yw), even though this backend still tracks activity-stream
// state on the DBCluster record.
func (b *InMemoryBackend) ModifyActivityStream(clusterID string, auditPolicy string) (*DBCluster, error) {
	b.mu.Lock("ModifyActivityStream")
	defer b.mu.Unlock()

	cluster, exists := b.clusters.Get(normalizeID(clusterID))
	if !exists {
		return nil, fmt.Errorf("%w: resource %s not found", ErrInstanceNotFound, clusterID)
	}

	if cluster.ActivityStreamStatus != activityStreamStatusStarted {
		return nil, fmt.Errorf(
			"%w: activity stream must be started to modify it for cluster %s",
			ErrInvalidDBInstanceState,
			clusterID,
		)
	}

	cluster.ActivityStreamAuditPolicy = auditPolicy

	return cluster, nil
}

const activityStreamStatusStarted = "started"
