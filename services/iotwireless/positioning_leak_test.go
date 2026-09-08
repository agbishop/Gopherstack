package iotwireless_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iotwireless"
)

// TestDelete_ClearsPosition verifies DeleteWirelessDevice and
// DeleteWirelessGateway clear the resource's entry in the positions map.
// GetPosition has no existence check against the resource itself, so a
// leaked entry is directly observable: querying the position for a deleted
// resource's own ID would otherwise still return it, and positions is
// persisted verbatim in Snapshot(), so the leak also grows the snapshot
// without bound.
func TestDelete_ClearsPosition(t *testing.T) {
	t.Parallel()

	t.Run("wireless device", func(t *testing.T) {
		t.Parallel()

		bk := iotwireless.NewInMemoryBackend()
		d, err := bk.CreateWirelessDevice(
			testAccountID, testRegion, "pos-leak-device", "LoRaWAN", "", "", "", nil, nil, nil,
		)
		require.NoError(t, err)
		otherD, err := bk.CreateWirelessDevice(
			testAccountID, testRegion, "pos-leak-device-sibling", "LoRaWAN", "", "", "", nil, nil, nil,
		)
		require.NoError(t, err)

		require.NoError(t, bk.UpdatePosition(d.ID, map[string]any{"latitude": 1.0}))
		require.NoError(t, bk.UpdatePosition(otherD.ID, map[string]any{"latitude": 1.0}))
		require.NotEmpty(t, bk.GetPosition(d.ID))

		require.NoError(t, bk.DeleteWirelessDevice(testAccountID, testRegion, d.ID))

		assert.Empty(t, bk.GetPosition(d.ID))
		assert.NotEmpty(t, bk.GetPosition(otherD.ID), "deleting one device must not disturb another's position")
	})

	t.Run("wireless gateway", func(t *testing.T) {
		t.Parallel()

		bk := iotwireless.NewInMemoryBackend()
		gw, err := bk.CreateWirelessGateway(testAccountID, testRegion, "pos-leak-gateway", "", nil, nil)
		require.NoError(t, err)
		otherGw, err := bk.CreateWirelessGateway(testAccountID, testRegion, "pos-leak-gateway-sibling", "", nil, nil)
		require.NoError(t, err)

		require.NoError(t, bk.UpdatePosition(gw.ID, map[string]any{"latitude": 1.0}))
		require.NoError(t, bk.UpdatePosition(otherGw.ID, map[string]any{"latitude": 1.0}))
		require.NotEmpty(t, bk.GetPosition(gw.ID))

		require.NoError(t, bk.DeleteWirelessGateway(testAccountID, testRegion, gw.ID))

		assert.Empty(t, bk.GetPosition(gw.ID))
		assert.NotEmpty(t, bk.GetPosition(otherGw.ID), "deleting one gateway must not disturb another's position")
	})
}
