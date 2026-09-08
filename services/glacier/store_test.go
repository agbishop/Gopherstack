package glacier_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glacier"
)

func TestReset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantLen int
	}{
		{name: "backend_empty_after_reset", wantLen: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := glacier.NewInMemoryBackend()
			_, err := b.CreateVault(testAccountID, testRegion, "vault-a")
			require.NoError(t, err)
			assert.Equal(t, 1, glacier.VaultCount(b))

			b.Reset()

			assert.Equal(t, tt.wantLen, glacier.VaultCount(b))
			assert.Equal(t, tt.wantLen, glacier.ArchiveCount(b))
			assert.Equal(t, tt.wantLen, glacier.MultipartUploadCount(b))
			assert.Equal(t, tt.wantLen, glacier.ProvisionedCapacityCount(b))
		})
	}
}

// TestReset_ArchiveDataCleared verifies that Reset() clears raw archive byte
// payloads, not just the Archive metadata nested on Vault. Left unclear this
// is also a memory-retention concern: archiveData holds actual archive
// bytes, unbounded by vault/archive count once orphaned.
func TestReset_ArchiveDataCleared(t *testing.T) {
	t.Parallel()

	b := glacier.NewInMemoryBackend()

	_, err := b.CreateVault(testAccountID, testRegion, "vault-a")
	require.NoError(t, err)

	_, err = b.UploadArchive(testAccountID, testRegion, "vault-a", "desc", "", 5, []byte("hello"))
	require.NoError(t, err)

	require.Equal(t, 1, glacier.ArchiveDataCount(b), "sanity: archive bytes should be stored")

	b.Reset()

	assert.Equal(t, 0, glacier.ArchiveDataCount(b), "archiveData must be cleared by Reset")
}

// TestMultipleResetCycle verifies reset + repopulate + reset clears everything.
func TestMultipleResetCycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		vaultsBefore int
		wantAfter    int
	}{
		{name: "two_vaults_then_reset", vaultsBefore: 2, wantAfter: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := glacier.NewInMemoryBackend()

			for i := range tt.vaultsBefore {
				_, err := b.CreateVault(testAccountID, testRegion, "vault-"+string(rune('a'+i)))
				require.NoError(t, err)
			}

			assert.Equal(t, tt.vaultsBefore, glacier.VaultCount(b))

			b.Reset()
			b.Reset() // double reset should be safe

			assert.Equal(t, tt.wantAfter, glacier.VaultCount(b))
		})
	}
}

// TestSeedHelpers verifies AddVaultInternal and AddArchiveInternal work correctly.
func TestSeedHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		vaultName    string
		archiveID    string
		wantVaults   int
		wantArchives int
	}{
		{name: "seed_vault_and_archive", vaultName: "seeded", archiveID: "arch-1", wantVaults: 1, wantArchives: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := glacier.NewInMemoryBackend()
			b.AddVaultInternal(testAccountID, testRegion, &glacier.Vault{VaultName: tt.vaultName})
			b.AddArchiveInternal(testAccountID, testRegion, tt.vaultName, &glacier.Archive{ArchiveID: tt.archiveID})

			assert.Equal(t, tt.wantVaults, glacier.VaultCount(b))
			assert.Equal(t, tt.wantArchives, glacier.ArchiveCount(b))
		})
	}
}

// TestExportCountHelpers verifies all four export count helpers work.
func TestExportCountHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		wantVaults       int
		wantArchives     int
		wantMultipart    int
		wantProvCapacity int
	}{
		{name: "counts_reflect_state", wantVaults: 1, wantArchives: 1, wantMultipart: 1, wantProvCapacity: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := glacier.NewInMemoryBackend()

			_, err := b.CreateVault(testAccountID, testRegion, "v1")
			require.NoError(t, err)

			_, err = b.UploadArchive(testAccountID, testRegion, "v1", "desc", "chk", 100, []byte("data"))
			require.NoError(t, err)

			_, err = b.InitiateMultipartUpload(testAccountID, testRegion, "v1", "desc", 1024*1024)
			require.NoError(t, err)

			_, err = b.PurchaseProvisionedCapacity(testAccountID)
			require.NoError(t, err)

			assert.Equal(t, tt.wantVaults, glacier.VaultCount(b))
			assert.Equal(t, tt.wantArchives, glacier.ArchiveCount(b))
			assert.Equal(t, tt.wantMultipart, glacier.MultipartUploadCount(b))
			assert.Equal(t, tt.wantProvCapacity, glacier.ProvisionedCapacityCount(b))
		})
	}
}

func TestSeedHelpers_AddJobInternal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantJobs int
	}{
		{name: "add_job_internal", wantJobs: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bk := glacier.NewInMemoryBackend()
			bk.AddVaultInternal(
				testAccountID,
				testRegion,
				&glacier.Vault{VaultName: "job-seed-vault"},
			)
			bk.AddJobInternal(testAccountID, testRegion, "job-seed-vault", &glacier.Job{
				JobID:  "test-job-id",
				Action: "InventoryRetrieval",
			})

			assert.Equal(t, tt.wantJobs, glacier.JobCount(bk))
		})
	}
}

func TestExportCountHelpers_JobCount_VaultLockCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		wantJobs       int
		wantVaultLocks int
	}{
		{name: "helpers_work", wantJobs: 2, wantVaultLocks: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bk := glacier.NewInMemoryBackend()
			bk.AddVaultInternal(testAccountID, testRegion, &glacier.Vault{VaultName: "v1"})
			bk.AddVaultInternal(testAccountID, testRegion, &glacier.Vault{VaultName: "v2"})
			bk.AddJobInternal(
				testAccountID,
				testRegion,
				"v1",
				&glacier.Job{JobID: "j1", Action: "InventoryRetrieval"},
			)
			bk.AddJobInternal(
				testAccountID,
				testRegion,
				"v2",
				&glacier.Job{JobID: "j2", Action: "InventoryRetrieval"},
			)

			require.NoError(t, bk.SetVaultLock(testAccountID, testRegion, "v1", "{}", "lockid1"))

			assert.Equal(t, tt.wantJobs, glacier.JobCount(bk))
			assert.Equal(t, tt.wantVaultLocks, glacier.VaultLockCount(bk))
		})
	}
}
