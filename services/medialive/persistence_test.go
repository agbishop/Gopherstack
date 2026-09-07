package medialive_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
	"github.com/blackbirdworks/gopherstack/services/medialive"
)

// TestInMemoryBackend_RestoreInvalidData verifies that malformed JSON is
// reported as an error rather than silently discarded or partially applied.
func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := medialive.NewInMemoryBackend("000000000000", "us-east-1")
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

	b := medialive.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateChannel(
		"seed-channel", "", "", medialive.ChannelAnywhereSettings{}, medialive.ChannelCreateExtras{}, nil,
	)
	require.NoError(t, err)

	// A syntactically valid but version-less/mismatched snapshot.
	err = b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.Equal(t, 0, medialive.ChannelCount(b))
}

// TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero verifies that a
// snapshot with no version field at all (the pre-Phase-3.3 shape) decodes
// with Version == 0, which mismatches medialiveSnapshotVersion and is
// discarded the same way any other incompatible version is -- not partially
// applied.
func TestInMemoryBackend_RestoreOldSnapshotDecodesAsZero(t *testing.T) {
	t.Parallel()

	b := medialive.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateChannel(
		"seed-channel", "", "", medialive.ChannelAnywhereSettings{}, medialive.ChannelCreateExtras{}, nil,
	)
	require.NoError(t, err)

	// Pre-Phase-3.3 shape: plain resource maps, no "version" or "tables" key.
	err = b.Restore(t.Context(), []byte(`{"channels":{"c1":{"id":"c1","name":"old"}}}`))
	require.NoError(t, err)

	assert.Equal(t, 0, medialive.ChannelCount(b))
}

// TestInMemoryBackend_SnapshotRestore_FullState exercises a Snapshot->Restore
// round trip across every store.Table-backed resource family the Phase 3.3
// conversion touched (15 tables, including the composite-key
// channelPlacementGroups table), plus every raw map left un-converted (tags,
// scheduleActions) and the ephemeral pendingTransferDeviceIDs index that must
// be correctly rebuilt from the restored inputDevices table.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := medialive.NewInMemoryBackend("111122223333", "us-west-2")

	channel, err := original.CreateChannel(
		"chan-1", "STANDARD", "role-arn", medialive.ChannelAnywhereSettings{},
		medialive.ChannelCreateExtras{
			LogLevel:              "INFO",
			CdiInputSpecification: medialive.CdiInputSpecification{Resolution: "HD"},
			ChannelSecurityGroups: []string{"sg-1"},
		},
		map[string]string{"env": "test"},
	)
	require.NoError(t, err)

	input, err := original.CreateInput("input-1", "UDP_PUSH", "role-arn", nil)
	require.NoError(t, err)

	_, err = original.CreateInputSecurityGroup(
		[]medialive.WhitelistRule{{Cidr: "10.0.0.0/24"}}, nil,
	)
	require.NoError(t, err)

	_, err = original.ClaimDevice("device-1")
	require.NoError(t, err)
	require.NoError(t, original.TransferInputDevice("device-1", "999999999999", "us-east-1", "transfer me"))

	multiplex, err := original.CreateMultiplex(
		"mux-1", []string{"us-west-2a"}, medialive.MultiplexSettings{TransportStreamBitrate: 1000000}, nil,
	)
	require.NoError(t, err)

	cluster, err := original.CreateCluster(
		"cluster-1", "ON_PREMISES", "role-arn",
		medialive.ClusterNetworkSettings{
			DefaultRoute: "if-a",
			InterfaceMappings: []medialive.InterfaceMapping{
				{LogicalInterfaceName: "if-a", NetworkID: "net-1"},
			},
		},
		nil,
	)
	require.NoError(t, err)

	// AnywhereSettings references the cluster, so it's set via UpdateChannel
	// here (after the cluster exists) rather than at CreateChannel above.
	channel, err = original.UpdateChannel(
		channel.ID, "", "",
		medialive.ChannelAnywhereSettings{ClusterID: cluster.ID}, true, medialive.ChannelUpdateExtras{},
	)
	require.NoError(t, err)

	signalMap, err := original.CreateSignalMap("sigmap-1", "desc", "arn:discovery", nil, nil, nil)
	require.NoError(t, err)

	cwGroup, err := original.CreateCloudWatchAlarmTemplateGroup("cwgroup-1", "desc", nil)
	require.NoError(t, err)

	cwTemplate, err := original.CreateCloudWatchAlarmTemplate(
		"cwtmpl-1", "desc", cwGroup.ID, "CPUUtilization", "AWS/MediaLive",
		"Average", "GreaterThanThreshold", "CLUSTER", "notBreaching",
		80, 1, 1, 60, nil,
	)
	require.NoError(t, err)

	ebGroup, err := original.CreateEventBridgeRuleTemplateGroup("ebgroup-1", "desc", nil)
	require.NoError(t, err)

	ebTemplate, err := original.CreateEventBridgeRuleTemplate(
		"ebtmpl-1", "desc", ebGroup.ID, "MEDIALIVE_MULTIPLEX_ALERT", nil, nil,
	)
	require.NoError(t, err)

	offerings, _, err := original.ListOfferings(0, "")
	require.NoError(t, err)
	require.NotEmpty(t, offerings)

	reservation, err := original.PurchaseOffering(
		offerings[0].OfferingID, "reservation-1", "", 1,
		medialive.RenewalSettings{AutomaticRenewal: "ENABLED", RenewalCount: 2},
		nil,
	)
	require.NoError(t, err)

	network, err := original.CreateNetwork("network-1", nil, nil, nil)
	require.NoError(t, err)

	sdiSource, err := original.CreateSdiSource("sdi-1", "SINGLE", "QUADRANT", nil)
	require.NoError(t, err)

	cpg, err := original.CreateChannelPlacementGroup(cluster.ID, "cpg-1", nil, nil)
	require.NoError(t, err)

	require.NoError(t, original.CreateTags(channel.ARN, map[string]string{"team": "media"}))

	_, err = original.BatchUpdateSchedule(channel.ID, []medialive.ScheduleAction{
		{ActionName: "action-1", ActionType: "INPUT_SWITCH"},
	}, nil)
	require.NoError(t, err)

	require.NoError(t, err)
	_, err = original.UpdateAccountConfiguration("kms-key-1")
	require.NoError(t, err)

	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := medialive.NewInMemoryBackend("000000000000", "us-east-1")
	require.NoError(t, fresh.Restore(t.Context(), snap))

	// Scalars.
	assert.Equal(t, "111122223333", fresh.AccountID())
	assert.Equal(t, "us-west-2", fresh.Region())

	cfg, err := fresh.DescribeAccountConfiguration()
	require.NoError(t, err)
	assert.Equal(t, "kms-key-1", cfg.KmsKeyID)

	// Direct tables.
	gotChannel, err := fresh.DescribeChannel(channel.ID)
	require.NoError(t, err)
	assert.Equal(t, "chan-1", gotChannel.Name)
	assert.Equal(t, cluster.ID, gotChannel.AnywhereSettings.ClusterID,
		"AnywhereSettings must survive Snapshot/Restore, not just Create")
	assert.Equal(t, "INFO", gotChannel.LogLevel,
		"LogLevel (gopherstack-jb9i) must survive Snapshot/Restore, not just Create")
	assert.Equal(t, "HD", gotChannel.CdiInputSpecification.Resolution,
		"CdiInputSpecification (gopherstack-jb9i) must survive Snapshot/Restore, not just Create")
	assert.Equal(t, []string{"sg-1"}, gotChannel.ChannelSecurityGroups,
		"ChannelSecurityGroups (gopherstack-jb9i) must survive Snapshot/Restore, not just Create")

	gotInput, err := fresh.DescribeInput(input.ID)
	require.NoError(t, err)
	assert.Equal(t, "input-1", gotInput.Name)

	assert.Equal(t, 1, medialive.InputSecurityGroupCount(fresh))

	gotMultiplex, err := fresh.DescribeMultiplex(multiplex.ID)
	require.NoError(t, err)
	assert.Equal(t, "mux-1", gotMultiplex.Name)

	gotCluster, err := fresh.DescribeCluster(cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, "cluster-1", gotCluster.Name)
	assert.Equal(t, "if-a", gotCluster.NetworkSettings.DefaultRoute,
		"NetworkSettings must survive Snapshot/Restore, not just Create")
	assert.Equal(t, []string{channel.ID}, gotCluster.ChannelIDs,
		"ChannelIds must be derivable from the restored channels table, not just live state")

	gotSignalMap, err := fresh.GetSignalMap(signalMap.ID)
	require.NoError(t, err)
	assert.Equal(t, "sigmap-1", gotSignalMap.Name)

	gotCWGroup, err := fresh.GetCloudWatchAlarmTemplateGroup(cwGroup.ID)
	require.NoError(t, err)
	assert.Equal(t, "cwgroup-1", gotCWGroup.Name)

	gotCWTemplate, err := fresh.GetCloudWatchAlarmTemplate(cwTemplate.ID)
	require.NoError(t, err)
	assert.Equal(t, "cwtmpl-1", gotCWTemplate.Name)

	gotEBGroup, err := fresh.GetEventBridgeRuleTemplateGroup(ebGroup.ID)
	require.NoError(t, err)
	assert.Equal(t, "ebgroup-1", gotEBGroup.Name)

	gotEBTemplate, err := fresh.GetEventBridgeRuleTemplate(ebTemplate.ID)
	require.NoError(t, err)
	assert.Equal(t, "ebtmpl-1", gotEBTemplate.Name)

	gotReservation, err := fresh.DescribeReservation(reservation.ReservationID)
	require.NoError(t, err)
	assert.Equal(t, "reservation-1", gotReservation.Name)
	assert.Equal(t, "ENABLED", gotReservation.RenewalSettings.AutomaticRenewal,
		"RenewalSettings must survive Snapshot/Restore, not just Purchase")

	gotNetwork, err := fresh.DescribeNetwork(network.ID)
	require.NoError(t, err)
	assert.Equal(t, "network-1", gotNetwork.Name)

	gotSdiSource, err := fresh.DescribeSdiSource(sdiSource.ID)
	require.NoError(t, err)
	assert.Equal(t, "sdi-1", gotSdiSource.Name)

	// Composite-key table.
	gotCPG, err := fresh.DescribeChannelPlacementGroup(cluster.ID, cpg.ID)
	require.NoError(t, err)
	assert.Equal(t, "cpg-1", gotCPG.Name)

	// Plain (un-converted) maps that were persisted before Phase 3.3.
	tags, err := fresh.ListTagsForResource(channel.ARN)
	require.NoError(t, err)
	assert.Equal(t, "media", tags["team"])

	schedule, err := fresh.DescribeSchedule(channel.ID)
	require.NoError(t, err)
	require.Len(t, schedule, 1)
	assert.Equal(t, "action-1", schedule[0].ActionName)

	// Ephemeral pendingTransferDeviceIDs index, rebuilt from the restored
	// inputDevices table's PendingTransfer field (not itself persisted).
	transfers, _, err := fresh.ListInputDeviceTransfers("OUTGOING", 0, "")
	require.NoError(t, err)
	require.Len(t, transfers, 1)
	assert.Equal(t, "device-1", transfers[0].DeviceID)
}

// Test_Handler_SnapshotRestore verifies Handler.Snapshot/Restore
// (persistence.go) delegate to the backend -- the shape persistence.Manager
// actually drives. cli.go's setupPersistence registers a service.Registerable
// (the *Handler returned by Provider.Init) in the persistence.Manager only if
// that Handler itself satisfies Snapshot(ctx)/Restore(ctx, []byte);
// InMemoryBackend implementing the same two methods is not enough on its own,
// since Handler.Backend is the StorageBackend interface and does not promote
// them. Mirrors services/securityhub's Test_Handler_SnapshotRestore.
func Test_Handler_SnapshotRestore(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h := medialive.NewHandler(medialive.NewInMemoryBackend("000000000000", "us-east-1"))

	// Compile-time proof Handler satisfies the persistence layer's contract.
	var _ persistence.Persistable = h

	_, err := h.Backend.CreateChannel(
		"delegate-channel", "", "", medialive.ChannelAnywhereSettings{}, medialive.ChannelCreateExtras{}, nil,
	)
	require.NoError(t, err)

	snap := h.Snapshot(ctx)
	require.NotNil(t, snap)

	h2 := medialive.NewHandler(medialive.NewInMemoryBackend("000000000000", "us-east-1"))
	require.NoError(t, h2.Restore(ctx, snap))

	assert.Equal(t, 1, medialive.ChannelCount(h2.Backend.(*medialive.InMemoryBackend)))
}
