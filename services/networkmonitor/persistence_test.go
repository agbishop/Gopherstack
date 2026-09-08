package networkmonitor_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/networkmonitor"
)

// TestInMemoryBackend_RestoreInvalidData verifies that malformed JSON is
// reported as an error rather than silently discarded or partially applied.
func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := networkmonitor.NewInMemoryBackend("us-east-1", "000000000000")
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend is discarded cleanly rather than
// partially decoded: the backend resets to empty state and Restore returns
// no error.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b := networkmonitor.NewInMemoryBackend("us-east-1", "000000000000")
	ctx := t.Context()

	_, err := b.CreateMonitor(ctx, "seed-monitor", nil, nil, nil)
	require.NoError(t, err)

	// A syntactically valid but version-mismatched snapshot.
	err = b.Restore(ctx, []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	_, err = b.GetMonitor(ctx, "seed-monitor")
	require.Error(t, err)

	summaries, _, err := b.ListMonitors(ctx, "", "", 0)
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

// TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero verifies that the
// pre-Phase-3.3 on-disk shape (region-nested monitors map, no version/tables
// fields) decodes with Version == 0, which mismatches
// networkmonitorSnapshotVersion and is discarded the same way any other
// incompatible version is -- not partially applied.
func TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero(t *testing.T) {
	t.Parallel()

	b := networkmonitor.NewInMemoryBackend("us-east-1", "000000000000")
	ctx := t.Context()

	_, err := b.CreateMonitor(ctx, "seed-monitor", nil, nil, nil)
	require.NoError(t, err)

	// The pre-refactor shape: region-nested monitors map, no version/tables keys.
	oldShape := `{"monitors":{"us-east-1":{"seed-monitor":{"monitorName":"seed-monitor"}}},` +
		`"next_probe_seq":1}`
	err = b.Restore(ctx, []byte(oldShape))
	require.NoError(t, err)

	_, err = b.GetMonitor(ctx, "seed-monitor")
	require.Error(t, err)
}

// TestInMemoryBackend_SnapshotRestore_FullState exercises a Snapshot->Restore
// round trip across the store.Table-backed monitors collection (composite
// "region|name" key, see store_setup.go) plus its nested, order-sensitive
// []*Probe slices. Two regions are seeded to confirm region isolation
// survives the round trip.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := networkmonitor.NewInMemoryBackend("us-east-1", "111122223333")
	ctx := t.Context()

	ctxEast := networkmonitor.WithRegion("us-east-1")
	ctxWest := networkmonitor.WithRegion("us-west-2")

	monEast, err := original.CreateMonitor(ctxEast, "east-monitor", nil, nil,
		map[string]string{"env": "prod"})
	require.NoError(t, err)

	monWest, err := original.CreateMonitor(ctxWest, "west-monitor", nil, nil,
		map[string]string{"env": "dev"})
	require.NoError(t, err)

	probe, err := original.CreateProbe(ctxEast, "east-monitor", &networkmonitor.ProbeInputForTest{
		Destination: "10.0.0.1",
		Protocol:    "ICMP",
		SourceArn:   "arn:aws:ec2:us-east-1:111122223333:subnet/subnet-abc",
	}, map[string]string{"probe-tag": "yes"})
	require.NoError(t, err)

	snap := original.Snapshot(ctx)
	require.NotNil(t, snap)

	fresh := networkmonitor.NewInMemoryBackend(config.DefaultRegion, config.DefaultAccountID)
	require.NoError(t, fresh.Restore(ctx, snap))

	// east-monitor: monitor fields + Tags + nested probe survive.
	gotEast, err := fresh.GetMonitor(ctxEast, "east-monitor")
	require.NoError(t, err)
	assert.Equal(t, monEast.MonitorArn, gotEast.MonitorArn)
	assert.Equal(t, "prod", gotEast.Tags["env"])
	require.Len(t, gotEast.Probes, 1)
	assert.Equal(t, probe.ProbeID, gotEast.Probes[0].ProbeID)
	assert.Equal(t, "10.0.0.1", gotEast.Probes[0].Destination)
	assert.Equal(t, "yes", gotEast.Probes[0].Tags["probe-tag"])

	// west-monitor: region isolation preserved across the round trip.
	gotWest, err := fresh.GetMonitor(ctxWest, "west-monitor")
	require.NoError(t, err)
	assert.Equal(t, monWest.MonitorArn, gotWest.MonitorArn)
	assert.Equal(t, "dev", gotWest.Tags["env"])

	_, err = fresh.GetMonitor(ctxWest, "east-monitor")
	require.Error(t, err, "east-monitor must not leak into us-west-2")

	tags, err := fresh.ListTagsForResource(ctxEast, gotEast.MonitorArn)
	require.NoError(t, err)
	assert.Equal(t, "prod", tags["env"])

	probeTags, err := fresh.ListTagsForResource(ctxEast, probe.ProbeArn)
	require.NoError(t, err)
	assert.Equal(t, "yes", probeTags["probe-tag"])

	// nextProbeSeq survived: a new probe on either backend must not collide
	// with the restored probe's ID.
	probe2, err := fresh.CreateProbe(ctxEast, "east-monitor", &networkmonitor.ProbeInputForTest{
		Destination: "10.0.0.2",
		Protocol:    "ICMP",
		SourceArn:   "arn:aws:ec2:us-east-1:111122223333:subnet/subnet-abc",
	}, nil)
	require.NoError(t, err)
	assert.NotEqual(t, probe.ProbeID, probe2.ProbeID)
}
