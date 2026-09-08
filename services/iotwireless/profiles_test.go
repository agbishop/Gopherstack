package iotwireless_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iotwireless"
)

// TestInMemoryBackend_SortedListDeviceProfiles verifies deterministic sort order for device profiles.
func TestInMemoryBackend_SortedListDeviceProfiles(t *testing.T) {
	t.Parallel()

	b := iotwireless.NewInMemoryBackend()

	for _, name := range []string{"dp-z", "dp-a", "dp-m"} {
		_, err := b.CreateDeviceProfile(testAccountID, testRegion, name, nil, nil, nil)
		require.NoError(t, err)
	}

	profiles := b.ListDeviceProfiles(testAccountID, testRegion, "")
	require.Len(t, profiles, 3)
	assert.Equal(t, "dp-a", profiles[0].Name)
	assert.Equal(t, "dp-m", profiles[1].Name)
	assert.Equal(t, "dp-z", profiles[2].Name)
}

func TestInMemoryBackend_ServiceProfileCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		profileName string
		wantErr     bool
	}{
		{
			name:        "create_and_get",
			profileName: "profile-1",
		},
		{
			name:    "get_nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bk := iotwireless.NewInMemoryBackend()

			if tt.wantErr {
				_, err := bk.GetServiceProfile(testAccountID, testRegion, "no-such-id")
				require.Error(t, err)

				return
			}

			sp, err := bk.CreateServiceProfile(
				testAccountID,
				testRegion,
				tt.profileName,
				nil,
				map[string]string{"tier": "standard"},
			)
			require.NoError(t, err)
			assert.Equal(t, tt.profileName, sp.Name)
			assert.NotEmpty(t, sp.ID)
			assert.NotEmpty(t, sp.ARN)
			assert.Equal(t, "standard", sp.Tags["tier"])

			got, err := bk.GetServiceProfile(testAccountID, testRegion, sp.ID)
			require.NoError(t, err)
			assert.Equal(t, sp.ID, got.ID)

			err = bk.DeleteServiceProfile(testAccountID, testRegion, sp.ID)
			require.NoError(t, err)

			_, err = bk.GetServiceProfile(testAccountID, testRegion, sp.ID)
			require.Error(t, err)
		})
	}
}

func TestInMemoryBackend_ServiceProfile_DeleteNotFound(t *testing.T) {
	t.Parallel()

	bk := iotwireless.NewInMemoryBackend()
	err := bk.DeleteServiceProfile(testAccountID, testRegion, "no-such-id")
	require.Error(t, err)
	assert.ErrorIs(t, err, iotwireless.ErrServiceProfileNotFound)
}

func TestInMemoryBackend_DeleteDeviceProfile_InUse(t *testing.T) {
	t.Parallel()

	bk := iotwireless.NewInMemoryBackend()

	dp, err := bk.CreateDeviceProfile(testAccountID, testRegion, "dp-1", nil, nil, nil)
	require.NoError(t, err)

	_, err = bk.CreateWirelessDevice(
		testAccountID, testRegion, "dev-1", "LoRaWAN", "dest-1", "", "",
		&iotwireless.LoRaWANDevice{DeviceProfileID: &dp.ID}, nil, nil,
	)
	require.NoError(t, err)

	err = bk.DeleteDeviceProfile(testAccountID, testRegion, dp.ID)
	require.ErrorIs(t, err, iotwireless.ErrDeviceProfileInUse)

	_, err = bk.GetDeviceProfile(testAccountID, testRegion, dp.ID)
	require.NoError(t, err, "device profile must still exist after the refused delete")
}

func TestInMemoryBackend_DeleteServiceProfile_InUse(t *testing.T) {
	t.Parallel()

	bk := iotwireless.NewInMemoryBackend()

	sp, err := bk.CreateServiceProfile(testAccountID, testRegion, "sp-1", nil, nil)
	require.NoError(t, err)

	_, err = bk.CreateWirelessDevice(
		testAccountID, testRegion, "dev-1", "LoRaWAN", "dest-1", "", "",
		&iotwireless.LoRaWANDevice{ServiceProfileID: &sp.ID}, nil, nil,
	)
	require.NoError(t, err)

	err = bk.DeleteServiceProfile(testAccountID, testRegion, sp.ID)
	require.ErrorIs(t, err, iotwireless.ErrServiceProfileInUse)

	_, err = bk.GetServiceProfile(testAccountID, testRegion, sp.ID)
	require.NoError(t, err, "service profile must still exist after the refused delete")
}

func TestInMemoryBackend_ListServiceProfiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		profileNames []string
		wantCount    int
	}{
		{name: "empty", wantCount: 0},
		{name: "two", profileNames: []string{"sp-1", "sp-2"}, wantCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bk := iotwireless.NewInMemoryBackend()

			for _, name := range tt.profileNames {
				_, err := bk.CreateServiceProfile(testAccountID, testRegion, name, nil, nil)
				require.NoError(t, err)
			}

			profiles := bk.ListServiceProfiles(testAccountID, testRegion)
			assert.Len(t, profiles, tt.wantCount)
		})
	}
}
