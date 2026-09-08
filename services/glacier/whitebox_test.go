package glacier

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func multipartPartsRowCount(b *InMemoryBackend) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.multipartParts)
}

func multipartPartDataRowCount(b *InMemoryBackend) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.multipartPartData)
}

// TestDeleteVault_CascadeCleansMultipartParts verifies that deleting a vault with
// in-progress multipart uploads cleans up their (raw map, not a *store.Table)
// multipartParts rows too, not just the multipartUploads table entry -- regression
// test for a leak where DeleteVault cascade-deleted the MultipartUpload row but left
// its uploaded-part rows orphaned in multipartParts forever.
func TestDeleteVault_CascadeCleansMultipartParts(t *testing.T) {
	t.Parallel()

	const (
		accountID = "123456789012"
		region    = "us-east-1"
		vaultName = "mpu-vault"
	)

	b := NewInMemoryBackend()

	_, err := b.CreateVault(accountID, region, vaultName)
	require.NoError(t, err)

	up, err := b.InitiateMultipartUpload(accountID, region, vaultName, "desc", 1<<20)
	require.NoError(t, err)

	partData := make([]byte, 1<<20)
	err = b.UploadMultipartPart(
		accountID, region, vaultName, up.MultipartUploadID, "bytes 0-1048575/*", "", partData,
	)
	require.NoError(t, err)

	require.Equal(t, 1, multipartPartsRowCount(b), "part row present before delete")
	require.Equal(t, 1, multipartPartDataRowCount(b), "part data row present before delete")

	err = b.DeleteVault(accountID, region, vaultName)
	require.NoError(t, err)

	require.Equal(t, 0, b.multipartUploads.Len(), "multipart upload table must be empty after delete")
	require.Equal(t, 0, multipartPartsRowCount(b),
		"multipartParts must be empty after vault delete — no leak")
	require.Equal(t, 0, multipartPartDataRowCount(b),
		"multipartPartData must be empty after vault delete — no leak")
}
