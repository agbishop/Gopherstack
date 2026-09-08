package iotwireless_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iotwireless"
)

func TestInMemoryBackend_DestinationCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		destName    string
		expression  string
		exprType    string
		roleArn     string
		description string
		wantErr     bool
	}{
		{
			name:        "create_and_get",
			destName:    "dest-1",
			expression:  "arn:aws:iot:us-east-1:000000000000:rule/my-rule",
			exprType:    "RuleName",
			roleArn:     "arn:aws:iam::000000000000:role/my-role",
			description: "test destination",
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
				_, err := bk.GetDestination(testAccountID, testRegion, "no-such-name")
				require.Error(t, err)

				return
			}

			dest, err := bk.CreateDestination(
				testAccountID, testRegion,
				tt.destName, tt.expression, tt.exprType, tt.roleArn, tt.description,
				nil,
			)
			require.NoError(t, err)
			assert.Equal(t, tt.destName, dest.Name)
			assert.NotEmpty(t, dest.ARN)
			assert.Equal(t, tt.expression, dest.Expression)

			got, err := bk.GetDestination(testAccountID, testRegion, tt.destName)
			require.NoError(t, err)
			assert.Equal(t, dest.Name, got.Name)

			err = bk.DeleteDestination(testAccountID, testRegion, tt.destName)
			require.NoError(t, err)

			_, err = bk.GetDestination(testAccountID, testRegion, tt.destName)
			require.Error(t, err)
		})
	}
}

func TestInMemoryBackend_Destination_DeleteNotFound(t *testing.T) {
	t.Parallel()

	bk := iotwireless.NewInMemoryBackend()
	err := bk.DeleteDestination(testAccountID, testRegion, "no-such-name")
	require.Error(t, err)
	assert.ErrorIs(t, err, iotwireless.ErrDestinationNotFound)
}

func TestInMemoryBackend_DeleteDestination_InUse(t *testing.T) {
	t.Parallel()

	bk := iotwireless.NewInMemoryBackend()

	_, err := bk.CreateDestination(testAccountID, testRegion, "dest-1", "", "", "", "", nil)
	require.NoError(t, err)

	_, err = bk.CreateWirelessDevice(
		testAccountID, testRegion, "dev-1", "LoRaWAN", "dest-1", "", "", nil, nil, nil,
	)
	require.NoError(t, err)

	err = bk.DeleteDestination(testAccountID, testRegion, "dest-1")
	require.ErrorIs(t, err, iotwireless.ErrDestinationInUse)

	_, err = bk.GetDestination(testAccountID, testRegion, "dest-1")
	require.NoError(t, err, "destination must still exist after the refused delete")
}

func TestInMemoryBackend_ListDestinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		destNames []string
		wantCount int
	}{
		{name: "empty", wantCount: 0},
		{name: "two", destNames: []string{"dest-a", "dest-b"}, wantCount: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bk := iotwireless.NewInMemoryBackend()

			for _, name := range tt.destNames {
				_, err := bk.CreateDestination(testAccountID, testRegion, name, "", "", "", "", nil)
				require.NoError(t, err)
			}

			dests := bk.ListDestinations(testAccountID, testRegion)
			assert.Len(t, dests, tt.wantCount)
		})
	}
}
