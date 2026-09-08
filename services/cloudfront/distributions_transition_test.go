package cloudfront_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudfront"
)

const (
	distTransitionWait = 2 * time.Second
	distTransitionTick = 10 * time.Millisecond
)

// waitForDistributionStatus polls GetDistribution until it reports want.
func waitForDistributionStatus(t *testing.T, b *cloudfront.InMemoryBackend, distID, want string) {
	t.Helper()

	require.Eventually(t, func() bool {
		d, err := b.GetDistribution(distID)

		return err == nil && d.Status == want
	}, distTransitionWait, distTransitionTick, "distribution never reached status %s", want)
}

// TestDistributionStatusTransition covers UpdateDistribution's async
// InProgress -> Deployed transition (distributions.go's
// scheduleDistributionDeployed), both on the live backend and across a
// Snapshot/Restore round trip.
func TestDistributionStatusTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, b *cloudfront.InMemoryBackend, distID string)
		name string
	}{
		{
			name: "reaches deployed",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend, distID string) {
				t.Helper()

				waitForDistributionStatus(t, b, distID, "Deployed")
			},
		},
		{
			name: "survives snapshot restore",
			run: func(t *testing.T, b *cloudfront.InMemoryBackend, distID string) {
				t.Helper()

				data := b.Snapshot(t.Context())
				require.NotEmpty(t, data)

				restored := cloudfront.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")
				require.NoError(t, restored.Restore(t.Context(), data))

				d, err := restored.GetDistribution(distID)
				require.NoError(t, err)
				require.Equal(t, "InProgress", d.Status, "restore should preserve the in-flight InProgress status")

				waitForDistributionStatus(t, restored, distID, "Deployed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudfront.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")

			callerRef := "ref-transition-" + tt.name
			d, err := b.CreateDistribution(callerRef, "orig", true, minimalDistConfig(callerRef, "orig", true))
			require.NoError(t, err)
			require.Equal(t, "Deployed", d.Status)

			upd, err := b.UpdateDistribution(d.ID, "updated", true, minimalDistConfig(callerRef, "updated", true))
			require.NoError(t, err)
			require.Equal(t, "InProgress", upd.Status,
				"UpdateDistribution should return the real intermediate InProgress status")

			tt.run(t, b, d.ID)
		})
	}
}

// distSnapshotProbe decodes the subset of backendSnapshot's JSON shape needed to
// inspect distribution-keyed side maps that DeleteDistribution must clean up.
type distSnapshotProbe struct {
	FuncAssocs  map[string][]cloudfront.FunctionAssociation   `json:"distributionFunctionAssociations"`
	MonitorSubs map[string]*cloudfront.MonitoringSubscription `json:"monitoringSubscriptions"`
}

// TestDeleteDistribution_CleansUpSideMaps proves that deleting a distribution
// removes its entries from distributionFunctionAssociations and
// monitoringSubscriptions, not just the primary distributions table. Before this
// fix, DeleteDistribution cleaned up distributionARNs/distributionCallerRefs/
// invalidations/distributionAliases/distributionWebACLs/the search index, but
// left the distribution's function associations and monitoring subscription
// permanently orphaned in memory -- an unbounded leak for any long-running
// process that creates and deletes many distributions, since generateID() never
// reuses a deleted distribution's ID.
func TestDeleteDistribution_CleansUpSideMaps(t *testing.T) {
	t.Parallel()

	b := cloudfront.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")

	callerRef := "ref-delete-sidemaps"
	d, err := b.CreateDistribution(callerRef, "orig", false, minimalDistConfig(callerRef, "orig", false))
	require.NoError(t, err)

	require.NoError(t, b.SetDistributionFunctionAssociations(d.ID, []cloudfront.FunctionAssociation{
		{FunctionARN: "arn:aws:cloudfront::123456789012:function/f1", EventType: "viewer-request"},
	}))
	require.NoError(t, b.CreateMonitoringSubscription(d.ID, true))

	assocs, err := b.GetDistributionFunctionAssociations(d.ID)
	require.NoError(t, err)
	require.Len(t, assocs, 1, "sanity check: association was recorded before delete")

	_, err = b.GetMonitoringSubscription(d.ID)
	require.NoError(t, err, "sanity check: subscription was recorded before delete")

	require.NoError(t, b.DeleteDistribution(d.ID))

	data := b.Snapshot(t.Context())
	require.NotEmpty(t, data)

	var probe distSnapshotProbe
	require.NoError(t, json.Unmarshal(data, &probe))

	_, leaked := probe.FuncAssocs[d.ID]
	require.False(t, leaked, "distributionFunctionAssociations must not retain a deleted distribution's entry")

	_, leaked = probe.MonitorSubs[d.ID]
	require.False(t, leaked, "monitoringSubscriptions must not retain a deleted distribution's entry")
}
