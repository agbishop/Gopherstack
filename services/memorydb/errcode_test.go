package memorydb_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -- Error __type wire-shape regression tests -------------------------------
//
// Real AWS MemoryDB defines no generic "ResourceNotFoundException" /
// "ResourceInUseException" / "InvalidRequestException" faults -- every error
// is resource-specific (see aws-sdk-go-v2/service/memorydb/types/errors.go).
// A real SDK client type-switches on the response's "__type" field via
// errors.As(&types.ClusterNotFoundFault{}, ...); collapsing every not-found
// error into one generic bucket makes that always fail to match.

func responseType(t *testing.T, body []byte) string {
	t.Helper()

	var resp map[string]any
	require.NoError(t, json.Unmarshal(body, &resp))

	typ, _ := resp["__type"].(string)

	return typ
}

func TestErrCode_ClusterNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "DeleteCluster", map[string]any{"ClusterName": "no-such-cluster"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "ClusterNotFoundFault", responseType(t, rec.Body.Bytes()))
}

// TestErrCode_CreateCluster_SnapshotNotFound pins current behavior for
// gopherstack-me2v: CreateCluster's declared error set has no
// SnapshotNotFoundFault, so this asserts what the handler emits today
// without endorsing it as correct -- see the landmine comment in
// clusters.go's CreateCluster restore-from-snapshot branch.
func TestErrCode_CreateCluster_SnapshotNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateCluster", map[string]any{
		"ClusterName":  "restore-cluster",
		"NodeType":     "db.r6g.large",
		"SnapshotName": "no-such-snapshot",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "SnapshotNotFoundFault", responseType(t, rec.Body.Bytes()))
}

func TestErrCode_ACLInUse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateACL", map[string]any{"ACLName": "in-use-acl"})
	doRequest(t, h, "CreateCluster", map[string]any{
		"ClusterName": "acl-cluster",
		"NodeType":    "db.t4g.small",
		"ACLName":     "in-use-acl",
	})

	rec := doRequest(t, h, "DeleteACL", map[string]any{"ACLName": "in-use-acl"})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "InvalidACLStateFault", responseType(t, rec.Body.Bytes()))
}

// TestDeleteUser_CascadesFromACL verifies DeleteUser succeeds for a user that
// is a member of an ACL and removes it from that ACL's membership, per
// api_op_DeleteUser.go: "The user will be removed from all ACLs and in turn
// removed from all clusters." DeleteUser must not refuse the operation the
// way DeleteACL/DeleteSubnetGroup correctly do for their own in-use faults.
func TestDeleteUser_CascadesFromACL(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateUser", map[string]any{
		"UserName":     "in-use-user",
		"AccessString": "on ~* +@all",
	})
	doRequest(t, h, "CreateACL", map[string]any{
		"ACLName":   "user-holder-acl",
		"UserNames": []string{"in-use-user"},
	})

	rec := doRequest(t, h, "DeleteUser", map[string]any{"UserName": "in-use-user"})
	require.Equal(t, http.StatusOK, rec.Code)

	descRec := doRequest(t, h, "DescribeACLs", map[string]any{"ACLName": "user-holder-acl"})
	require.Equal(t, http.StatusOK, descRec.Code)

	var resp struct {
		ACLs []struct {
			UserNames []string `json:"UserNames"`
		} `json:"ACLs"`
	}
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &resp))
	require.Len(t, resp.ACLs, 1)
	assert.NotContains(t, resp.ACLs[0].UserNames, "in-use-user")
}

// TestErrCode_SubnetGroupInUse also covers a state-correctness gap: previously
// DeleteSubnetGroup had no in-use check at all and would delete a subnet
// group still referenced by a live cluster.
func TestErrCode_SubnetGroupInUse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateSubnetGroup", map[string]any{
		"SubnetGroupName": "in-use-sg",
		"SubnetIds":       []string{"subnet-1"},
	})
	doRequest(t, h, "CreateCluster", map[string]any{
		"ClusterName":     "sg-cluster",
		"NodeType":        "db.t4g.small",
		"SubnetGroupName": "in-use-sg",
	})

	rec := doRequest(t, h, "DeleteSubnetGroup", map[string]any{"SubnetGroupName": "in-use-sg"})
	require.Equal(t, http.StatusBadRequest, rec.Code,
		"DeleteSubnetGroup must reject deleting a subnet group still referenced by a cluster")
	assert.Equal(t, "SubnetGroupInUseFault", responseType(t, rec.Body.Bytes()))

	// The subnet group must still exist.
	rec = doRequest(t, h, "DescribeSubnetGroups", map[string]any{"SubnetGroupName": "in-use-sg"})
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestErrCode_ParameterGroupInUse covers a state-correctness gap: previously
// DeleteParameterGroup had no in-use check at all and would delete a
// parameter group still referenced by a live cluster
// (api_op_DeleteParameterGroup.go: "You cannot delete a parameter group if
// it is associated with any clusters.").
func TestErrCode_ParameterGroupInUse(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateParameterGroup", map[string]any{
		"ParameterGroupName": "in-use-pg",
		"Family":             "memorydb_redis7",
	})
	doRequest(t, h, "CreateCluster", map[string]any{
		"ClusterName":        "pg-cluster",
		"NodeType":           "db.t4g.small",
		"ParameterGroupName": "in-use-pg",
	})

	rec := doRequest(t, h, "DeleteParameterGroup", map[string]any{"ParameterGroupName": "in-use-pg"})
	require.Equal(t, http.StatusBadRequest, rec.Code,
		"DeleteParameterGroup must reject deleting a parameter group still referenced by a cluster")
	assert.Equal(t, "InvalidParameterGroupStateFault", responseType(t, rec.Body.Bytes()))

	// The parameter group must still exist.
	rec = doRequest(t, h, "DescribeParameterGroups", map[string]any{"ParameterGroupName": "in-use-pg"})
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestErrCode_TagResourceInvalidARN(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "TagResource", map[string]any{
		"ResourceArn": "arn:aws:memorydb:us-east-1:123456789012:cluster/does-not-exist",
		"Tags":        []map[string]string{{"Key": "k", "Value": "v"}},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "InvalidARNFault", responseType(t, rec.Body.Bytes()))
}

// TestErrCode_UpdateClusterUnknownACL proves UpdateCluster rejects an ACLName
// that names no known ACL instead of silently assigning the cluster a
// dangling reference (real AWS fault: ACLNotFoundFault).
func TestErrCode_UpdateClusterUnknownACL(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateCluster", map[string]any{
		"ClusterName": "acl-update-cluster",
		"NodeType":    "db.r6g.large",
		"ACLName":     "open-access",
	})

	rec := doRequest(t, h, "UpdateCluster", map[string]any{
		"ClusterName": "acl-update-cluster",
		"ACLName":     "no-such-acl",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "ACLNotFoundFault", responseType(t, rec.Body.Bytes()))
}

func TestErrCode_ServiceUpdateNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doRequest(t, h, "CreateCluster", map[string]any{
		"ClusterName": "su-not-found-cluster",
		"NodeType":    "db.t4g.small",
		"ACLName":     "open-access",
	})

	rec := doRequest(t, h, "BatchUpdateCluster", map[string]any{
		"ClusterNames": []string{"su-not-found-cluster"},
		"ServiceUpdate": map[string]any{
			"ServiceUpdateNameToApply": "no-such-service-update",
		},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "ServiceUpdateNotFoundFault", responseType(t, rec.Body.Bytes()))
}

func TestErrCode_MultiRegionParameterGroupNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "DescribeMultiRegionParameters", map[string]any{
		"MultiRegionParameterGroupName": "no-such-mrpg",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "MultiRegionParameterGroupNotFoundFault", responseType(t, rec.Body.Bytes()))
}
