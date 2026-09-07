package rds_test

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rds"
)

// TestActivityStream_Lifecycle exercises the Start/Stop/ModifyActivityStream
// wire shapes end-to-end against a real DB cluster ARN, verifying:
//   - Start emits KinesisStreamName/KmsKeyId/Mode/Status/ApplyImmediately.
//   - Stop emits KinesisStreamName/KmsKeyId/Status and clears cluster state.
//   - Modify emits PolicyStatus (not the gopherstack-invented "AuditPolicy"
//     element from a prior version of this response, which doesn't exist on
//     aws-sdk-go-v2's ModifyActivityStreamOutput) plus KinesisStreamName/Mode.
//   - Operating against a nonexistent cluster returns DBClusterNotFound, not
//     InvalidParameterValue (a prior version of Start/Stop/Modify returned
//     InvalidParameterValue for this case).
func TestActivityStream_Lifecycle(t *testing.T) {
	t.Parallel()

	h := newRDSHandler()
	postRDSForm(t, h,
		"Action=CreateDBCluster&Version=2014-10-31"+
			"&DBClusterIdentifier=as-cluster&Engine=aurora-postgresql"+
			"&MasterUsername=admin&MasterUserPassword=password123")

	clusterARN := "arn:aws:rds:us-east-1:000000000000:cluster:as-cluster"

	startRec := postRDSForm(t, h, fmt.Sprintf(
		"Action=StartActivityStream&Version=2014-10-31&ResourceArn=%s&KmsKeyId=my-key&Mode=sync",
		clusterARN,
	))
	require.Equal(t, http.StatusOK, startRec.Code, "body: %s", startRec.Body.String())

	var startResp struct {
		Result struct {
			KinesisStreamName string `xml:"KinesisStreamName"`
			KmsKeyID          string `xml:"KmsKeyId"`
			Status            string `xml:"Status"`
			Mode              string `xml:"Mode"`
			ApplyImmediately  bool   `xml:"ApplyImmediately"`
		} `xml:"StartActivityStreamResult"`
	}
	require.NoError(t, xml.Unmarshal(startRec.Body.Bytes(), &startResp))
	assert.Equal(t, "my-key", startResp.Result.KmsKeyID)
	assert.Equal(t, "sync", startResp.Result.Mode)
	assert.Equal(t, "started", startResp.Result.Status)
	assert.NotEmpty(t, startResp.Result.KinesisStreamName)
	assert.True(t, startResp.Result.ApplyImmediately)

	// Starting again while already started is InvalidDBClusterStateFault.
	dupRec := postRDSForm(t, h, fmt.Sprintf(
		"Action=StartActivityStream&Version=2014-10-31&ResourceArn=%s&KmsKeyId=my-key&Mode=sync",
		clusterARN,
	))
	assert.Equal(t, http.StatusBadRequest, dupRec.Code)
	assert.Contains(t, dupRec.Body.String(), "InvalidDBClusterStateFault")

	modRec := postRDSForm(t, h, fmt.Sprintf(
		"Action=ModifyActivityStream&Version=2014-10-31&ResourceArn=%s&AuditPolicyState=locked",
		clusterARN,
	))
	require.Equal(t, http.StatusOK, modRec.Code, "body: %s", modRec.Body.String())

	var modResp struct {
		Result struct {
			KinesisStreamName string `xml:"KinesisStreamName"`
			KmsKeyID          string `xml:"KmsKeyId"`
			Mode              string `xml:"Mode"`
			Status            string `xml:"Status"`
			PolicyStatus      string `xml:"PolicyStatus"`
		} `xml:"ModifyActivityStreamResult"`
	}
	require.NoError(t, xml.Unmarshal(modRec.Body.Bytes(), &modResp))
	assert.Equal(t, "locked", modResp.Result.PolicyStatus)
	assert.Equal(t, "started", modResp.Result.Status)
	assert.NotEmpty(t, modResp.Result.KinesisStreamName)
	assert.NotContains(t, modRec.Body.String(), "<AuditPolicy>",
		"AuditPolicy is not a real ModifyActivityStreamOutput field")

	stopRec := postRDSForm(t, h, fmt.Sprintf(
		"Action=StopActivityStream&Version=2014-10-31&ResourceArn=%s",
		clusterARN,
	))
	require.Equal(t, http.StatusOK, stopRec.Code, "body: %s", stopRec.Body.String())

	var stopResp struct {
		Result struct {
			KinesisStreamName string `xml:"KinesisStreamName"`
			KmsKeyID          string `xml:"KmsKeyId"`
			Status            string `xml:"Status"`
		} `xml:"StopActivityStreamResult"`
	}
	require.NoError(t, xml.Unmarshal(stopRec.Body.Bytes(), &stopResp))
	assert.Equal(t, "stopped", stopResp.Result.Status)

	// Stopping again (not started) is InvalidDBClusterStateFault.
	dupStopRec := postRDSForm(t, h, fmt.Sprintf(
		"Action=StopActivityStream&Version=2014-10-31&ResourceArn=%s",
		clusterARN,
	))
	assert.Equal(t, http.StatusBadRequest, dupStopRec.Code)
	assert.Contains(t, dupStopRec.Body.String(), "InvalidDBClusterStateFault")
}

// TestActivityStream_ClusterNotFound verifies Start/Stop/ModifyActivityStream
// return the correct not-found code (not InvalidParameterValue) for a
// nonexistent cluster ARN, matching real RDS's documented errors for these
// operations.
//
// ModifyActivityStream's expected code differs from Start/Stop: its declared
// error set (rds@v1.124.1 deserializers.go
// awsAwsquery_deserializeOpErrorModifyActivityStream) has no
// DBClusterNotFoundFault case at all, only DBInstanceNotFound/
// InvalidDBInstanceState/ResourceNotFoundFault -- consistent with its doc
// comment restricting the op to RDS for Oracle/SQL Server DB instances,
// unlike Start/StopActivityStream which are Aurora-cluster-scoped and do
// declare DBClusterNotFoundFault. This case previously asserted
// DBClusterNotFound for all three, which was wrong for ModifyActivityStream
// (gopherstack-fm1e) -- corrected here rather than weakened.
func TestActivityStream_ClusterNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		query       string
		wantCode    string
		wantAbsent  string
		wantAbsent2 string
	}{
		{
			name: "StartActivityStream",
			query: "Action=StartActivityStream&Version=2014-10-31" +
				"&ResourceArn=arn:aws:rds:us-east-1:000000000000:cluster:missing&KmsKeyId=k&Mode=sync",
			wantCode:    "DBClusterNotFound",
			wantAbsent:  "InvalidParameterValue",
			wantAbsent2: "DBInstanceNotFound",
		},
		{
			name: "StopActivityStream",
			query: "Action=StopActivityStream&Version=2014-10-31" +
				"&ResourceArn=arn:aws:rds:us-east-1:000000000000:cluster:missing",
			wantCode:    "DBClusterNotFound",
			wantAbsent:  "InvalidParameterValue",
			wantAbsent2: "DBInstanceNotFound",
		},
		{
			name: "ModifyActivityStream",
			query: "Action=ModifyActivityStream&Version=2014-10-31" +
				"&ResourceArn=arn:aws:rds:us-east-1:000000000000:cluster:missing&AuditPolicyState=locked",
			wantCode:    "DBInstanceNotFound",
			wantAbsent:  "InvalidParameterValue",
			wantAbsent2: "DBClusterNotFound",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRDSHandler()
			rec := postRDSForm(t, h, tt.query)
			assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
			assert.Contains(t, rec.Body.String(), tt.wantCode)
			assert.NotContains(t, rec.Body.String(), tt.wantAbsent)
			assert.NotContains(t, rec.Body.String(), tt.wantAbsent2)
		})
	}
}

// TestActivityStream_StateConflict verifies the wire code for the
// already-started/not-started conflict path, per operation, mirroring
// TestActivityStream_ClusterNotFound above.
//
// gopherstack-74yw: ModifyActivityStream's declared error set (rds@v1.124.1
// deserializers.go awsAwsquery_deserializeOpErrorModifyActivityStream) has no
// InvalidDBClusterStateFault case at all -- only DBInstanceNotFound/
// InvalidDBInstanceState/ResourceNotFoundFault -- unlike Start/StopActivityStream
// (awsAwsquery_deserializeOpError{Start,Stop}ActivityStream), which both declare
// InvalidDBClusterStateFault. Before the fix, all three raised
// ErrActivityStreamNotStarted/ErrActivityStreamAlreadyStarted, both hardcoded
// to InvalidDBClusterStateFault in the dispatch table, so Modify emitted a code
// no real client can receive from that operation.
func TestActivityStream_StateConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		clusterID  string
		setup      string // extra request to run before the conflicting one, "" for none
		query      string
		wantCode   string
		wantAbsent string
	}{
		{
			name:      "StartActivityStream_AlreadyStarted",
			clusterID: "as-conflict-start",
			setup: "Action=StartActivityStream&Version=2014-10-31" +
				"&ResourceArn=%[1]s&KmsKeyId=k&Mode=sync",
			query: "Action=StartActivityStream&Version=2014-10-31" +
				"&ResourceArn=%[1]s&KmsKeyId=k&Mode=sync",
			wantCode:   "InvalidDBClusterStateFault",
			wantAbsent: "InvalidDBInstanceState",
		},
		{
			name:       "StopActivityStream_NotStarted",
			clusterID:  "as-conflict-stop",
			query:      "Action=StopActivityStream&Version=2014-10-31&ResourceArn=%[1]s",
			wantCode:   "InvalidDBClusterStateFault",
			wantAbsent: "InvalidDBInstanceState",
		},
		{
			name:      "ModifyActivityStream_NotStarted",
			clusterID: "as-conflict-modify",
			query: "Action=ModifyActivityStream&Version=2014-10-31" +
				"&ResourceArn=%[1]s&AuditPolicyState=locked",
			wantCode:   "InvalidDBInstanceState",
			wantAbsent: "InvalidDBClusterStateFault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newRDSHandler()
			postRDSForm(t, h, fmt.Sprintf(
				"Action=CreateDBCluster&Version=2014-10-31&DBClusterIdentifier=%s"+
					"&Engine=aurora-postgresql&MasterUsername=admin&MasterUserPassword=password123",
				tt.clusterID))
			clusterARN := "arn:aws:rds:us-east-1:000000000000:cluster:" + tt.clusterID

			if tt.setup != "" {
				setupRec := postRDSForm(t, h, fmt.Sprintf(tt.setup, clusterARN))
				require.Equal(t, http.StatusOK, setupRec.Code, "setup body: %s", setupRec.Body.String())
			}

			rec := postRDSForm(t, h, fmt.Sprintf(tt.query, clusterARN))
			assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
			assert.Contains(t, rec.Body.String(), tt.wantCode)
			assert.NotContains(t, rec.Body.String(), tt.wantAbsent)
		})
	}
}

// TestActivityStream_BackendErrors exercises the backend methods directly
// for the not-yet-started error paths.
//
// The ModifyActivityStream assertion was strengthened by gopherstack-74yw: it
// previously also asserted ErrActivityStreamNotStarted, the same sentinel as
// StopActivityStream's. That sentinel maps unconditionally to
// InvalidDBClusterStateFault in the dispatch table, which ModifyActivityStream
// does not declare (see TestActivityStream_StateConflict) -- so the old
// assertion only proved sentinel identity, not the wire code a real client
// would see, and passed even with the bug present. ModifyActivityStream now
// raises ErrInvalidDBInstanceState for this conflict instead, which the
// dispatch table already maps to the code its declared error set requires.
func TestActivityStream_BackendErrors(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)
	_, err := b.CreateDBCluster(
		"as-backend-cluster", "aurora-mysql", "admin", "", "", 0, nil, rds.DBClusterOptions{},
	)
	require.NoError(t, err)

	_, err = b.StopActivityStream("as-backend-cluster")
	require.ErrorIs(t, err, rds.ErrActivityStreamNotStarted)

	_, err = b.ModifyActivityStream("as-backend-cluster", "locked")
	require.ErrorIs(t, err, rds.ErrInvalidDBInstanceState)
	require.NotErrorIs(t, err, rds.ErrActivityStreamNotStarted)

	_, err = b.StartActivityStream("as-backend-cluster", "key-1", "")
	require.NoError(t, err)

	cluster, err := b.ModifyActivityStream("as-backend-cluster", "unlocked")
	require.NoError(t, err)
	assert.Equal(t, "unlocked", cluster.ActivityStreamAuditPolicy)
}
