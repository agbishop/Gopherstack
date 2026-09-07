package rds_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRDSHandler_FormActions_Clusters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		body            string
		setupBodies     []string
		wantContains    []string
		wantNotContains []string
		wantCode        int
	}{
		// Cluster tests
		{
			name: "CreateDBCluster",
			body: "Action=CreateDBCluster&Version=2014-10-31" +
				"&DBClusterIdentifier=my-cluster&Engine=aurora-postgresql&MasterUsername=admin&DatabaseName=mydb",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateDBClusterResponse", "my-cluster", "aurora-postgresql"},
		},
		{
			name:         "CreateDBCluster_EmptyID",
			body:         "Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "CreateDBCluster_Duplicate",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=dup-cluster&Engine=aurora-postgresql",
			},
			body:         "Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=dup-cluster&Engine=aurora-postgresql",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterAlreadyExists"},
		},
		{
			name: "DescribeDBClusters",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=list-cluster&Engine=aurora-postgresql",
			},
			body:         "Action=DescribeDBClusters&Version=2014-10-31",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeDBClustersResponse", "list-cluster"},
		},
		{
			name:         "DescribeDBClusters_NotFound",
			body:         "Action=DescribeDBClusters&Version=2014-10-31&DBClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterNotFound"},
		},
		{
			name: "DeleteDBCluster",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=del-cluster&Engine=aurora-postgresql",
			},
			body: "Action=DeleteDBCluster&Version=2014-10-31&DBClusterIdentifier=del-cluster" +
				"&SkipFinalSnapshot=true",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteDBClusterResponse", "del-cluster"},
		},
		{
			name:         "DeleteDBCluster_NotFound",
			body:         "Action=DeleteDBCluster&Version=2014-10-31&DBClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterNotFound"},
		},
		{
			name: "DeleteDBCluster_MissingSkipFinalSnapshotAndFinalSnapshotID",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=del-cluster-nosnap" +
					"&Engine=aurora-postgresql",
			},
			body:         "Action=DeleteDBCluster&Version=2014-10-31&DBClusterIdentifier=del-cluster-nosnap",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterCombination", "FinalDBSnapshotIdentifier"},
		},
		{
			name: "DeleteDBCluster_WithFinalSnapshot",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=del-cluster-finalsnap" +
					"&Engine=aurora-postgresql",
			},
			body: "Action=DeleteDBCluster&Version=2014-10-31&DBClusterIdentifier=del-cluster-finalsnap" +
				"&FinalDBSnapshotIdentifier=del-cluster-finalsnap-final",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteDBClusterResponse", "del-cluster-finalsnap"},
		},
		{
			name: "ModifyDBCluster",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=mod-cluster&Engine=aurora-postgresql",
			},
			body: "Action=ModifyDBCluster&Version=2014-10-31&DBClusterIdentifier=mod-cluster" +
				"&DBClusterParameterGroupName=my-cluster-pg",
			wantCode:     http.StatusOK,
			wantContains: []string{"ModifyDBClusterResponse", "my-cluster-pg"},
		},
		{
			name: "CreateDBCluster_ReplicationSourceIdentifier",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=src-repl&Engine=aurora-postgresql",
			},
			body: "Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=replica-repl" +
				"&Engine=aurora-postgresql&ReplicationSourceIdentifier=src-repl",
			wantCode: http.StatusOK,
			wantContains: []string{
				"CreateDBClusterResponse", "replica-repl",
				"<ReplicationSourceIdentifier>src-repl</ReplicationSourceIdentifier>",
			},
		},
		{
			name: "CreateDBCluster_ReplicationSourceIdentifier_NotFound",
			body: "Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=replica-no-src" +
				"&Engine=aurora-postgresql&ReplicationSourceIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterNotFound"},
		},
		{
			name: "DescribeDBClusters_ReadReplicaIdentifiers",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=src-rr&Engine=aurora-postgresql",
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=replica-rr" +
					"&Engine=aurora-postgresql&ReplicationSourceIdentifier=src-rr",
			},
			body:     "Action=DescribeDBClusters&Version=2014-10-31&DBClusterIdentifier=src-rr",
			wantCode: http.StatusOK,
			wantContains: []string{
				"<ReadReplicaIdentifiers><ReadReplicaIdentifier>replica-rr</ReadReplicaIdentifier></ReadReplicaIdentifiers>",
			},
		},
		{
			name: "DeleteDBCluster_ReplicaRemovedFromSourceReadReplicaIdentifiers",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=src-del-rr&Engine=aurora-postgresql",
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=replica-del-rr" +
					"&Engine=aurora-postgresql&ReplicationSourceIdentifier=src-del-rr",
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=replica-keep-rr" +
					"&Engine=aurora-postgresql&ReplicationSourceIdentifier=src-del-rr",
				"Action=DeleteDBCluster&Version=2014-10-31&DBClusterIdentifier=replica-del-rr" +
					"&SkipFinalSnapshot=true",
			},
			body:     "Action=DescribeDBClusters&Version=2014-10-31&DBClusterIdentifier=src-del-rr",
			wantCode: http.StatusOK,
			wantContains: []string{
				"<ReadReplicaIdentifiers><ReadReplicaIdentifier>replica-keep-rr</ReadReplicaIdentifier></ReadReplicaIdentifiers>",
			},
			wantNotContains: []string{"replica-del-rr"},
		},
		{
			name: "DeleteDBCluster_SourceDeletionOrphansReplica",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=src-orphan-rr&Engine=aurora-postgresql",
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=replica-orphan-rr" +
					"&Engine=aurora-postgresql&ReplicationSourceIdentifier=src-orphan-rr",
				"Action=DeleteDBCluster&Version=2014-10-31&DBClusterIdentifier=src-orphan-rr" +
					"&SkipFinalSnapshot=true",
			},
			body:     "Action=DescribeDBClusters&Version=2014-10-31&DBClusterIdentifier=replica-orphan-rr",
			wantCode: http.StatusOK,
			wantContains: []string{
				"<ReplicationSourceIdentifier>src-orphan-rr</ReplicationSourceIdentifier>",
			},
		},
		// Cluster Parameter Group tests
		{
			name: "CreateDBClusterParameterGroup",
			body: "Action=CreateDBClusterParameterGroup&Version=2014-10-31" +
				"&DBClusterParameterGroupName=my-cpg&DBParameterGroupFamily=aurora-postgresql14&Description=My+cluster+pg",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateDBClusterParameterGroupResponse", "my-cpg"},
		},
		{
			name: "DescribeDBClusterParameterGroups",
			setupBodies: []string{
				"Action=CreateDBClusterParameterGroup&Version=2014-10-31" +
					"&DBClusterParameterGroupName=list-cpg&DBParameterGroupFamily=aurora-postgresql14",
			},
			body:         "Action=DescribeDBClusterParameterGroups&Version=2014-10-31",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeDBClusterParameterGroupsResponse", "list-cpg"},
		},
		// Cluster Snapshot tests
		{
			name: "CreateDBClusterSnapshot",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=snap-cluster&Engine=aurora-postgresql",
			},
			body: "Action=CreateDBClusterSnapshot&Version=2014-10-31" +
				"&DBClusterSnapshotIdentifier=cluster-snap-1&DBClusterIdentifier=snap-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateDBClusterSnapshotResponse", "cluster-snap-1"},
		},
		{
			name: "CreateDBClusterSnapshot_ClusterNotFound",
			body: "Action=CreateDBClusterSnapshot&Version=2014-10-31" +
				"&DBClusterSnapshotIdentifier=orphan-snap&DBClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterNotFound"},
		},
		{
			name: "DescribeDBClusterSnapshots",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=snap-cluster2&Engine=aurora-postgresql",
				"Action=CreateDBClusterSnapshot&Version=2014-10-31" +
					"&DBClusterSnapshotIdentifier=list-csnap&DBClusterIdentifier=snap-cluster2",
			},
			body:         "Action=DescribeDBClusterSnapshots&Version=2014-10-31",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeDBClusterSnapshotsResponse", "list-csnap"},
		},
		// StartDBCluster / StopDBCluster tests
		{
			name: "StartDBCluster",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=start-cluster&Engine=aurora-postgresql",
				"Action=StopDBCluster&Version=2014-10-31&DBClusterIdentifier=start-cluster",
			},
			body:         "Action=StartDBCluster&Version=2014-10-31&DBClusterIdentifier=start-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"StartDBClusterResponse", "start-cluster", "available"},
		},
		{
			name:         "StartDBCluster_NotFound",
			body:         "Action=StartDBCluster&Version=2014-10-31&DBClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterNotFound"},
		},
		{
			name:         "StartDBCluster_EmptyID",
			body:         "Action=StartDBCluster&Version=2014-10-31&DBClusterIdentifier=",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "StopDBCluster",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=stop-cluster&Engine=aurora-postgresql",
			},
			body:         "Action=StopDBCluster&Version=2014-10-31&DBClusterIdentifier=stop-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"StopDBClusterResponse", "stop-cluster", "stopped"},
		},
		{
			name:         "StopDBCluster_NotFound",
			body:         "Action=StopDBCluster&Version=2014-10-31&DBClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterNotFound"},
		},
		{
			name:         "StopDBCluster_EmptyID",
			body:         "Action=StopDBCluster&Version=2014-10-31&DBClusterIdentifier=",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		// DeleteDBClusterSnapshot tests
		{
			name: "DeleteDBClusterSnapshot",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=delsnap-cluster&Engine=aurora-postgresql",
				"Action=CreateDBClusterSnapshot&Version=2014-10-31" +
					"&DBClusterSnapshotIdentifier=del-csnap&DBClusterIdentifier=delsnap-cluster",
			},
			body:         "Action=DeleteDBClusterSnapshot&Version=2014-10-31&DBClusterSnapshotIdentifier=del-csnap",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteDBClusterSnapshotResponse", "del-csnap"},
		},
		{
			name:         "DeleteDBClusterSnapshot_NotFound",
			body:         "Action=DeleteDBClusterSnapshot&Version=2014-10-31&DBClusterSnapshotIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterSnapshotNotFound"},
		},
		{
			name:         "DeleteDBClusterSnapshot_EmptyID",
			body:         "Action=DeleteDBClusterSnapshot&Version=2014-10-31&DBClusterSnapshotIdentifier=",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		// RestoreDBClusterFromSnapshot tests
		{
			name: "RestoreDBClusterFromSnapshot",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=restore-src&Engine=aurora-postgresql",
				"Action=CreateDBClusterSnapshot&Version=2014-10-31" +
					"&DBClusterSnapshotIdentifier=restore-snap&DBClusterIdentifier=restore-src",
			},
			body: "Action=RestoreDBClusterFromSnapshot&Version=2014-10-31" +
				"&DBClusterIdentifier=restored-cluster&SnapshotIdentifier=restore-snap&Engine=aurora-postgresql",
			wantCode:     http.StatusOK,
			wantContains: []string{"RestoreDBClusterFromSnapshotResponse", "restored-cluster"},
		},
		{
			name: "RestoreDBClusterFromSnapshot_SnapshotNotFound",
			body: "Action=RestoreDBClusterFromSnapshot&Version=2014-10-31" +
				"&DBClusterIdentifier=new-cluster&SnapshotIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterSnapshotNotFound"},
		},
		{
			name: "RestoreDBClusterFromSnapshot_EmptyID",
			body: "Action=RestoreDBClusterFromSnapshot&Version=2014-10-31" +
				"&DBClusterIdentifier=&SnapshotIdentifier=some-snap",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "RestoreDBClusterFromSnapshot_EmptySnapshotID",
			body: "Action=RestoreDBClusterFromSnapshot&Version=2014-10-31" +
				"&DBClusterIdentifier=some-cluster&SnapshotIdentifier=",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		// RestoreDBClusterToPointInTime tests
		{
			name: "RestoreDBClusterToPointInTime",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31" +
					"&DBClusterIdentifier=pitr-src&Engine=aurora-postgresql&MasterUsername=admin",
			},
			body: "Action=RestoreDBClusterToPointInTime&Version=2014-10-31" +
				"&DBClusterIdentifier=pitr-restored&SourceDBClusterIdentifier=pitr-src",
			wantCode:     http.StatusOK,
			wantContains: []string{"RestoreDBClusterToPointInTimeResponse", "pitr-restored"},
		},
		{
			name: "RestoreDBClusterToPointInTime_SourceNotFound",
			body: "Action=RestoreDBClusterToPointInTime&Version=2014-10-31" +
				"&DBClusterIdentifier=pitr-new&SourceDBClusterIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterNotFound"},
		},
		{
			name: "RestoreDBClusterToPointInTime_EmptySourceID",
			body: "Action=RestoreDBClusterToPointInTime&Version=2014-10-31" +
				"&DBClusterIdentifier=pitr-new&SourceDBClusterIdentifier=",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		// CopyDBClusterSnapshot tests
		{
			name: "CopyDBClusterSnapshot",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=copy-src-cluster&Engine=aurora-postgresql",
				"Action=CreateDBClusterSnapshot&Version=2014-10-31" +
					"&DBClusterSnapshotIdentifier=copy-src-snap&DBClusterIdentifier=copy-src-cluster",
			},
			body: "Action=CopyDBClusterSnapshot&Version=2014-10-31" +
				"&SourceDBClusterSnapshotIdentifier=copy-src-snap&TargetDBClusterSnapshotIdentifier=copy-dst-snap",
			wantCode:     http.StatusOK,
			wantContains: []string{"CopyDBClusterSnapshotResponse", "copy-dst-snap"},
		},
		{
			name: "CopyDBClusterSnapshot_SourceNotFound",
			body: "Action=CopyDBClusterSnapshot&Version=2014-10-31" +
				"&SourceDBClusterSnapshotIdentifier=nonexistent&TargetDBClusterSnapshotIdentifier=dst-snap",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterSnapshotNotFound"},
		},
		{
			name: "CopyDBClusterSnapshot_EmptySourceID",
			body: "Action=CopyDBClusterSnapshot&Version=2014-10-31" +
				"&SourceDBClusterSnapshotIdentifier=&TargetDBClusterSnapshotIdentifier=dst-snap",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		// CreateDBClusterEndpoint tests
		{
			name: "CreateDBClusterEndpoint",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=ep-cluster&Engine=aurora-postgresql",
			},
			body: "Action=CreateDBClusterEndpoint&Version=2014-10-31" +
				"&DBClusterEndpointIdentifier=my-endpoint&DBClusterIdentifier=ep-cluster&EndpointType=READER",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateDBClusterEndpointResponse", "my-endpoint", "READER"},
		},
		{
			name: "CreateDBClusterEndpoint_ClusterNotFound",
			body: "Action=CreateDBClusterEndpoint&Version=2014-10-31" +
				"&DBClusterEndpointIdentifier=ep&DBClusterIdentifier=nonexistent&EndpointType=READER",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterNotFound"},
		},
		{
			name: "CreateDBClusterEndpoint_EmptyID",
			body: "Action=CreateDBClusterEndpoint&Version=2014-10-31" +
				"&DBClusterEndpointIdentifier=&DBClusterIdentifier=some-cluster&EndpointType=READER",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "CreateDBClusterEndpoint_EmptyClusterID",
			body: "Action=CreateDBClusterEndpoint&Version=2014-10-31" +
				"&DBClusterEndpointIdentifier=my-ep&DBClusterIdentifier=&EndpointType=READER",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		// DescribeDBClusterEndpoints tests
		{
			name: "DescribeDBClusterEndpoints",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=eplist-cluster&Engine=aurora-postgresql",
				"Action=CreateDBClusterEndpoint&Version=2014-10-31" +
					"&DBClusterEndpointIdentifier=list-ep&DBClusterIdentifier=eplist-cluster&EndpointType=READER",
			},
			body:         "Action=DescribeDBClusterEndpoints&Version=2014-10-31&DBClusterIdentifier=eplist-cluster",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeDBClusterEndpointsResponse", "list-ep"},
		},
		{
			// DBClusterEndpointIdentifier is a filter, not an existence check --
			// aws-sdk-go-v2/service/rds@v1.124.1's deserializers.go declares no
			// not-found error for it (only DBClusterNotFoundFault, for an
			// invalid DBClusterIdentifier), so a non-matching value returns an
			// empty list rather than an error.
			name:            "DescribeDBClusterEndpoints_NoMatch",
			body:            "Action=DescribeDBClusterEndpoints&Version=2014-10-31&DBClusterEndpointIdentifier=nonexistent",
			wantCode:        http.StatusOK,
			wantContains:    []string{"DescribeDBClusterEndpointsResponse"},
			wantNotContains: []string{"DBClusterEndpointNotFound"},
		},
		// DeleteDBClusterEndpoint tests
		{
			name: "DeleteDBClusterEndpoint",
			setupBodies: []string{
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=dep-cluster&Engine=aurora-postgresql",
				"Action=CreateDBClusterEndpoint&Version=2014-10-31" +
					"&DBClusterEndpointIdentifier=dep-ep&DBClusterIdentifier=dep-cluster&EndpointType=READER",
			},
			body:         "Action=DeleteDBClusterEndpoint&Version=2014-10-31&DBClusterEndpointIdentifier=dep-ep",
			wantCode:     http.StatusOK,
			wantContains: []string{"DeleteDBClusterEndpointResponse", "dep-ep"},
		},
		{
			name:         "DeleteDBClusterEndpoint_NotFound",
			body:         "Action=DeleteDBClusterEndpoint&Version=2014-10-31&DBClusterEndpointIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBClusterEndpointNotFound"},
		},
		// DescribeValidDBInstanceModifications tests
		{
			name: "DescribeValidDBInstanceModifications",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=mod-valid-db&Engine=postgres",
			},
			body:         "Action=DescribeValidDBInstanceModifications&Version=2014-10-31&DBInstanceIdentifier=mod-valid-db",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeValidDBInstanceModificationsResponse", "coreCount", "threadsPerCore"},
		},
		{
			name:         "DescribeValidDBInstanceModifications_NotFound",
			body:         "Action=DescribeValidDBInstanceModifications&Version=2014-10-31&DBInstanceIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBInstanceNotFound"},
		},
		// StartExportTask tests
		{
			name: "StartExportTask",
			body: "Action=StartExportTask&Version=2014-10-31" +
				"&ExportTaskIdentifier=my-export&SourceArn=arn:aws:rds:us-east-1:000000000000:snapshot:my-snap" +
				"&S3BucketName=my-bucket&IamRoleArn=arn:aws:iam::000000000000:role/export-role" +
				"&KmsKeyId=arn:aws:kms:us-east-1:000000000000:key/test-key",
			wantCode:     http.StatusOK,
			wantContains: []string{"StartExportTaskResponse", "my-export", "complete"},
		},
		{
			name: "StartExportTask_EmptyID",
			body: "Action=StartExportTask&Version=2014-10-31" +
				"&ExportTaskIdentifier=&SourceArn=arn:aws:rds:us-east-1:000000000000:snapshot:my-snap" +
				"&IamRoleArn=arn:aws:iam::000000000000:role/export-role" +
				"&KmsKeyId=arn:aws:kms:us-east-1:000000000000:key/test-key",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "StartExportTask_MissingIamRoleArn",
			body: "Action=StartExportTask&Version=2014-10-31" +
				"&ExportTaskIdentifier=no-role&SourceArn=arn:aws:rds:us-east-1:000000000000:snapshot:my-snap" +
				"&KmsKeyId=arn:aws:kms:us-east-1:000000000000:key/test-key",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "StartExportTask_MissingKmsKeyId",
			body: "Action=StartExportTask&Version=2014-10-31" +
				"&ExportTaskIdentifier=no-key&SourceArn=arn:aws:rds:us-east-1:000000000000:snapshot:my-snap" +
				"&IamRoleArn=arn:aws:iam::000000000000:role/export-role",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name: "StartExportTask_Duplicate",
			setupBodies: []string{
				"Action=StartExportTask&Version=2014-10-31" +
					"&ExportTaskIdentifier=dup-export&SourceArn=arn:aws:rds:us-east-1:000000000000:snapshot:s1" +
					"&IamRoleArn=arn:aws:iam::000000000000:role/export-role" +
					"&KmsKeyId=arn:aws:kms:us-east-1:000000000000:key/test-key",
			},
			body: "Action=StartExportTask&Version=2014-10-31" +
				"&ExportTaskIdentifier=dup-export&SourceArn=arn:aws:rds:us-east-1:000000000000:snapshot:s1" +
				"&IamRoleArn=arn:aws:iam::000000000000:role/export-role" +
				"&KmsKeyId=arn:aws:kms:us-east-1:000000000000:key/test-key",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ExportTaskAlreadyExists"},
		},
		// DescribeExportTasks tests
		{
			name: "DescribeExportTasks",
			setupBodies: []string{
				"Action=StartExportTask&Version=2014-10-31" +
					"&ExportTaskIdentifier=list-export&SourceArn=arn:aws:rds:us-east-1:000000000000:snapshot:s2" +
					"&IamRoleArn=arn:aws:iam::000000000000:role/export-role" +
					"&KmsKeyId=arn:aws:kms:us-east-1:000000000000:key/test-key",
			},
			body:         "Action=DescribeExportTasks&Version=2014-10-31",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeExportTasksResponse", "list-export"},
		},
		{
			name:         "DescribeExportTasks_NotFound",
			body:         "Action=DescribeExportTasks&Version=2014-10-31&ExportTaskIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ExportTaskNotFound"},
		},
		// CancelExportTask tests
		{
			name: "CancelExportTask",
			setupBodies: []string{
				"Action=StartExportTask&Version=2014-10-31" +
					"&ExportTaskIdentifier=cancel-export&SourceArn=arn:aws:rds:us-east-1:000000000000:snapshot:s3" +
					"&IamRoleArn=arn:aws:iam::000000000000:role/export-role" +
					"&KmsKeyId=arn:aws:kms:us-east-1:000000000000:key/test-key",
			},
			body:         "Action=CancelExportTask&Version=2014-10-31&ExportTaskIdentifier=cancel-export",
			wantCode:     http.StatusOK,
			wantContains: []string{"CancelExportTaskResponse", "cancel-export", "canceled"},
		},
		{
			name:         "CancelExportTask_EmptyID",
			body:         "Action=CancelExportTask&Version=2014-10-31&ExportTaskIdentifier=",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidParameterValue"},
		},
		{
			name:         "CancelExportTask_NotFound",
			body:         "Action=CancelExportTask&Version=2014-10-31&ExportTaskIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ExportTaskNotFound"},
		},
		{
			name: "CreateDBInstanceReadReplica",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=source-db&Engine=postgres",
			},
			body: "Action=CreateDBInstanceReadReplica&Version=2014-10-31" +
				"&DBInstanceIdentifier=replica-db&SourceDBInstanceIdentifier=source-db",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateDBInstanceReadReplicaResponse", "replica-db", "source-db"},
		},
		{
			name: "CreateDBInstanceReadReplica_SourceNotFound",
			body: "Action=CreateDBInstanceReadReplica&Version=2014-10-31" +
				"&DBInstanceIdentifier=replica-db&SourceDBInstanceIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBInstanceNotFound"},
		},
		{
			name: "PromoteReadReplica",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=promo-source&Engine=postgres",
				"Action=CreateDBInstanceReadReplica&Version=2014-10-31" +
					"&DBInstanceIdentifier=promo-replica&SourceDBInstanceIdentifier=promo-source",
			},
			body:         "Action=PromoteReadReplica&Version=2014-10-31&DBInstanceIdentifier=promo-replica",
			wantCode:     http.StatusOK,
			wantContains: []string{"PromoteReadReplicaResponse", "promo-replica"},
		},
		// Misc tests
		{
			name: "RebootDBInstance",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=reboot-db&Engine=postgres",
			},
			body:         "Action=RebootDBInstance&Version=2014-10-31&DBInstanceIdentifier=reboot-db",
			wantCode:     http.StatusOK,
			wantContains: []string{"RebootDBInstanceResponse", "reboot-db"},
		},
		{
			name:         "RebootDBInstance_NotFound",
			body:         "Action=RebootDBInstance&Version=2014-10-31&DBInstanceIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBInstanceNotFound"},
		},
		{
			name:         "DescribeDBEngineVersions",
			body:         "Action=DescribeDBEngineVersions&Version=2014-10-31",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeDBEngineVersionsResponse", "postgres"},
		},
		{
			name:            "DescribeDBEngineVersions_ByEngine",
			body:            "Action=DescribeDBEngineVersions&Version=2014-10-31&Engine=mysql",
			wantCode:        http.StatusOK,
			wantContains:    []string{"mysql"},
			wantNotContains: []string{"aurora-postgresql"},
		},
		{
			name:         "DescribeOrderableDBInstanceOptions",
			body:         "Action=DescribeOrderableDBInstanceOptions&Version=2014-10-31&Engine=postgres",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeOrderableDBInstanceOptionsResponse", "db.t3.micro"},
		},
		{
			name: "DescribeDBLogFiles",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=log-db&Engine=postgres",
			},
			body:         "Action=DescribeDBLogFiles&Version=2014-10-31&DBInstanceIdentifier=log-db",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeDBLogFilesResponse"},
		},
		{
			name:         "DescribeDBLogFiles_NotFound",
			body:         "Action=DescribeDBLogFiles&Version=2014-10-31&DBInstanceIdentifier=nonexistent",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBInstanceNotFound"},
		},
		{
			name: "DownloadDBLogFilePortion",
			setupBodies: []string{
				"Action=CreateDBInstance&Version=2014-10-31&DBInstanceIdentifier=logportion-db&Engine=postgres",
			},
			body: "Action=DownloadDBLogFilePortion&Version=2014-10-31" +
				"&DBInstanceIdentifier=logportion-db&LogFileName=error/postgresql.log",
			wantCode:     http.StatusOK,
			wantContains: []string{"DownloadDBLogFilePortionResponse"},
		},
		{
			name: "DownloadDBLogFilePortion_NotFound",
			body: "Action=DownloadDBLogFilePortion&Version=2014-10-31" +
				"&DBInstanceIdentifier=nonexistent&LogFileName=error.log",
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"DBInstanceNotFound"},
		},
		{
			name:         "DescribeGlobalClusters",
			body:         "Action=DescribeGlobalClusters&Version=2014-10-31",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeGlobalClustersResponse"},
		},
		{
			name:         "DescribeOptionGroupOptions",
			body:         "Action=DescribeOptionGroupOptions&Version=2014-10-31&EngineName=mysql",
			wantCode:     http.StatusOK,
			wantContains: []string{"DescribeOptionGroupOptionsResponse"},
		},
		{
			name: "CreateDBInstance_WithParameterGroup",
			body: "Action=CreateDBInstance&Version=2014-10-31" +
				"&DBInstanceIdentifier=pg-db&Engine=postgres&DBParameterGroupName=my-custom-pg",
			wantCode:     http.StatusOK,
			wantContains: []string{"CreateDBInstanceResponse", "pg-db", "my-custom-pg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRDSHandler()

			for _, setup := range tt.setupBodies {
				postRDSForm(t, h, setup)
			}

			rec := postRDSForm(t, h, tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)

			body := rec.Body.String()
			for _, s := range tt.wantContains {
				assert.Contains(t, body, s)
			}
			for _, s := range tt.wantNotContains {
				assert.NotContains(t, body, s)
			}
		})
	}
}
