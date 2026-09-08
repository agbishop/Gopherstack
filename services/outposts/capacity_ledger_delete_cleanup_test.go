package outposts_test

import (
	"encoding/json"
	"testing"

	outpostssdk "github.com/aws/aws-sdk-go-v2/service/outposts"
	"github.com/stretchr/testify/require"
)

// snapshotTables is a local decode of backendSnapshot's exported shape,
// just enough to inspect one table's raw contents in a black-box test.
type snapshotTables struct {
	Tables map[string]json.RawMessage `json:"tables"`
}

// TestDeleteOutpost_CleansRunningInstanceLedger guards against a ghost-row
// leak: ConsumeCapacity (capacity_ledger.go) keys every runningInstance row
// by AssetID/OutpostID, but DeleteOutpost only rejects while a capacity task
// is REQUESTED -- a COMPLETED one (and any capacity it consumed) does not
// block deletion, so an Outpost can be deleted while EC2 instances are still
// recorded as running on its now-deleted Asset. Without an explicit cleanup
// here, the row would only ever be removed if services/ec2's
// TerminateInstances later happened to call ReleaseCapacity for that exact
// instance ID -- an unbounded leak otherwise (runningInstances is a
// registered store.Table, included in every Snapshot).
func TestDeleteOutpost_CleansRunningInstanceLedger(t *testing.T) {
	t.Parallel()

	h, client := newTestHandlerAndClient(t)
	siteID := createTestSite(t, client)
	created := createTestOutpost(t, client, siteID)

	assets, err := client.ListAssets(
		t.Context(),
		&outpostssdk.ListAssetsInput{OutpostIdentifier: created.OutpostId},
	)
	require.NoError(t, err)
	require.Len(t, assets.Assets, 1)

	waitForCapacityTaskCompletion(t, client, created.OutpostId, assets.Assets[0].AssetId, "m5.xlarge", 2)

	const leakedInstanceID = "i-leaktest0123456"

	require.NoError(t, h.Backend.ConsumeCapacity(
		*created.OutpostArn, "m5.xlarge", rtTestAccountID, []string{leakedInstanceID},
	))

	_, err = client.DeleteOutpost(t.Context(), &outpostssdk.DeleteOutpostInput{
		OutpostId: created.OutpostId,
	})
	require.NoError(t, err)

	var snap snapshotTables
	require.NoError(t, json.Unmarshal(h.Snapshot(t.Context()), &snap))

	require.NotContains(t, string(snap.Tables["runningInstances"]), leakedInstanceID,
		"DeleteOutpost must clean up runningInstances rows for its own Asset(s), not leak them")
}
