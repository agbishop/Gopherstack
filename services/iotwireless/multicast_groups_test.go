package iotwireless_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iotwireless"
)

func TestInMemoryBackend_MulticastGroup_SortedList(t *testing.T) {
	t.Parallel()

	b := iotwireless.NewInMemoryBackend()

	for _, name := range []string{"mg-z", "mg-a", "mg-m"} {
		_, err := b.CreateMulticastGroup(testAccountID, testRegion, name, "", nil, nil)
		require.NoError(t, err)
	}

	groups := b.ListMulticastGroups(testAccountID, testRegion)
	require.Len(t, groups, 3)
	assert.Equal(t, "mg-a", groups[0].Name)
	assert.Equal(t, "mg-m", groups[1].Name)
	assert.Equal(t, "mg-z", groups[2].Name)
}

func TestInMemoryBackend_DeleteMulticastGroup_InUseByFuotaTask(t *testing.T) {
	t.Parallel()

	b := iotwireless.NewInMemoryBackend()

	mg, err := b.CreateMulticastGroup(testAccountID, testRegion, "mg-1", "", nil, nil)
	require.NoError(t, err)

	ft, err := b.CreateFuotaTask(testAccountID, testRegion, "ft-1", "", "", "", "", 0, 0, 0, nil, nil)
	require.NoError(t, err)

	err = b.AssociateMulticastGroupWithFuotaTask(ft.ID, mg.ID)
	require.NoError(t, err)

	err = b.DeleteMulticastGroup(testAccountID, testRegion, mg.ID)
	require.ErrorIs(t, err, iotwireless.ErrMulticastGroupInUse)

	_, err = b.GetMulticastGroup(testAccountID, testRegion, mg.ID)
	require.NoError(t, err, "multicast group must still exist after the refused delete")
}

func TestInMemoryBackend_Reset_ClearsMulticastGroups(t *testing.T) {
	t.Parallel()

	b := iotwireless.NewInMemoryBackend()

	_, err := b.CreateMulticastGroup(testAccountID, testRegion, "mg1", "", nil, nil)
	require.NoError(t, err)

	b.Reset()

	groups := b.ListMulticastGroups(testAccountID, testRegion)
	assert.Empty(t, groups)
}
