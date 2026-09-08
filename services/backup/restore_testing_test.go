package backup_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/backup"
)

func TestCreateRestoreTestingSelection_Validation(t *testing.T) {
	t.Parallel()

	t.Run("missing IamRoleArn is a validation error", func(t *testing.T) {
		t.Parallel()
		b := backup.NewInMemoryBackend("000000000000", "us-east-1")
		_, err := b.CreateRestoreTestingPlan("plan-a", "", 0, nil)
		require.NoError(t, err)

		_, err = b.CreateRestoreTestingSelection("plan-a", "sel-a", backup.RestoreTestingSelectionInput{
			ProtectedResourceType: "EC2",
		})
		require.ErrorIs(t, err, backup.ErrValidation)
	})

	t.Run("missing ProtectedResourceType is a validation error", func(t *testing.T) {
		t.Parallel()
		b := backup.NewInMemoryBackend("000000000000", "us-east-1")
		_, err := b.CreateRestoreTestingPlan("plan-b", "", 0, nil)
		require.NoError(t, err)

		_, err = b.CreateRestoreTestingSelection("plan-b", "sel-b", backup.RestoreTestingSelectionInput{
			IAMRoleArn: "arn:aws:iam::000000000000:role/r",
		})
		require.ErrorIs(t, err, backup.ErrValidation)
	})

	t.Run("full shape round-trips through Get", func(t *testing.T) {
		t.Parallel()
		b := backup.NewInMemoryBackend("000000000000", "us-east-1")
		_, err := b.CreateRestoreTestingPlan("plan-c", "", 0, nil)
		require.NoError(t, err)

		sel, err := b.CreateRestoreTestingSelection("plan-c", "sel-c", backup.RestoreTestingSelectionInput{
			ProtectedResourceType: "EC2",
			IAMRoleArn:            "arn:aws:iam::000000000000:role/r",
			ProtectedResourceArns: []string{"*"},
			ProtectedResourceConditions: &backup.ProtectedResourceConditions{
				StringEquals: []backup.KeyValue{{Key: "Environment", Value: "prod"}},
			},
			RestoreMetadataOverrides: map[string]string{"newVolumeName": "restored"},
			ValidationWindowHours:    24,
		})
		require.NoError(t, err)

		got, err := b.GetRestoreTestingSelection("plan-c", "sel-c")
		require.NoError(t, err)
		assert.Equal(t, sel.IAMRoleArn, got.IAMRoleArn)
		assert.Equal(t, []string{"*"}, got.ProtectedResourceArns)
		require.NotNil(t, got.ProtectedResourceConditions)
		require.Len(t, got.ProtectedResourceConditions.StringEquals, 1)
		assert.Equal(t, "Environment", got.ProtectedResourceConditions.StringEquals[0].Key)
		assert.Equal(t, "restored", got.RestoreMetadataOverrides["newVolumeName"])
		assert.Equal(t, int64(24), got.ValidationWindowHours)
	})
}

func TestDeleteRestoreTestingPlan_RequiresSelectionsDeletedFirst(t *testing.T) {
	t.Parallel()

	t.Run("plan with no selections deletes cleanly", func(t *testing.T) {
		t.Parallel()
		b := backup.NewInMemoryBackend("000000000000", "us-east-1")
		_, err := b.CreateRestoreTestingPlan("empty-plan", "", 0, nil)
		require.NoError(t, err)

		require.NoError(t, b.DeleteRestoreTestingPlan("empty-plan"))
	})

	t.Run("plan with an active selection is rejected", func(t *testing.T) {
		t.Parallel()
		b := backup.NewInMemoryBackend("000000000000", "us-east-1")
		_, err := b.CreateRestoreTestingPlan("busy-plan", "", 0, nil)
		require.NoError(t, err)
		_, err = b.CreateRestoreTestingSelection("busy-plan", "sel-a", backup.RestoreTestingSelectionInput{
			ProtectedResourceType: "EC2",
			IAMRoleArn:            "arn:aws:iam::000000000000:role/r",
		})
		require.NoError(t, err)

		err = b.DeleteRestoreTestingPlan("busy-plan")
		require.ErrorIs(t, err, backup.ErrInvalidRequest)

		_, getErr := b.GetRestoreTestingPlan("busy-plan")
		require.NoError(t, getErr, "plan must still exist after the rejected delete")
		_, selErr := b.GetRestoreTestingSelection("busy-plan", "sel-a")
		require.NoError(t, selErr, "selection must not be cascade-deleted by the rejected delete")
	})
}

func TestUpdateRestoreTestingSelection_FullReplace(t *testing.T) {
	t.Parallel()
	b := backup.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateRestoreTestingPlan("plan-u", "", 0, nil)
	require.NoError(t, err)
	_, err = b.CreateRestoreTestingSelection("plan-u", "sel-u", backup.RestoreTestingSelectionInput{
		ProtectedResourceType: "EC2",
		IAMRoleArn:            "arn:aws:iam::000000000000:role/original",
	})
	require.NoError(t, err)

	updated, err := b.UpdateRestoreTestingSelection("plan-u", "sel-u", backup.RestoreTestingSelectionInput{
		IAMRoleArn:            "arn:aws:iam::000000000000:role/updated",
		ValidationWindowHours: 48,
	})
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iam::000000000000:role/updated", updated.IAMRoleArn)
	assert.Equal(t, int64(48), updated.ValidationWindowHours)
	// ProtectedResourceType is immutable on Update per the real API.
	assert.Equal(t, "EC2", updated.ProtectedResourceType)
}

func TestListScanJobsFiltered_AccountIDWildcard(t *testing.T) {
	t.Parallel()
	b := backup.NewInMemoryBackend("123456789012", "us-east-1")

	j1 := b.StartScanJob("arn:aws:backup:us-east-1:123456789012:backup-vault:v1", backup.StartScanJobInput{
		BackupVaultName: "v1",
	})
	j2 := b.StartScanJob("arn:aws:backup:us-east-1:123456789012:backup-vault:v1", backup.StartScanJobInput{
		BackupVaultName: "v1",
	})

	// api_op_ListScanJobs.go's ByAccountId doc: "If used from an Amazon Web
	// Services Organizations management account, passing * returns all jobs
	// across the organization." No seeded job's AccountID is ever the
	// literal string "*", so this only passes if "*" is honored as a
	// wildcard rather than compared for equality.
	got, _ := b.ListScanJobsFiltered(backup.ListScanJobsFilter{AccountID: "*"})
	require.Len(t, got, 2)
	gotIDs := []string{got[0].ScanJobID, got[1].ScanJobID}
	assert.ElementsMatch(t, []string{j1.ScanJobID, j2.ScanJobID}, gotIDs)

	// A literal, non-matching account ID still excludes everything.
	none, _ := b.ListScanJobsFiltered(backup.ListScanJobsFilter{AccountID: "999999999999"})
	assert.Empty(t, none)
}
