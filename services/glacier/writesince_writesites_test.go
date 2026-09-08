package glacier_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glacier"
)

// TestDeleteVault_WriteSinceLastInventory_UploadArchive pins archives.go's
// UploadArchive setting WriteSinceLastInventory. No inventory ever runs in
// this test, so NumberOfArchivesAtLastInventory stays at CreateVault's
// zero-pointer default -- a blocked delete here is attributable only to the
// write flag, isolating this specific site.
func TestDeleteVault_WriteSinceLastInventory_UploadArchive(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	const vaultName = "upload-writesince-vault"
	createVault(t, h, vaultName)

	archiveID := uploadArchiveHTTP(t, h, vaultName)

	assertVaultDeleteRejected(t, h, vaultName, deleteVaultHTTP(t, h, vaultName))

	// Clearing half: empty the vault, then an inventory refresh should clear
	// the flag and let the delete through.
	deleteArchiveHTTP(t, h, vaultName, archiveID)
	initiateInventoryJobHTTP(t, h, vaultName)

	rec := deleteVaultHTTP(t, h, vaultName)
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

// TestDeleteVault_WriteSinceLastInventory_DeleteArchive pins archives.go's
// DeleteArchive setting WriteSinceLastInventory. The archive is seeded via
// AddVaultInternal/AddArchiveInternal (bypassing UploadArchive's own flag
// write) with the flag explicitly false and NumberOfArchivesAtLastInventory
// explicitly zero, so DeleteArchive's own write is the only thing that can
// flip the guard.
func TestDeleteVault_WriteSinceLastInventory_DeleteArchive(t *testing.T) {
	t.Parallel()

	bk := glacier.NewInMemoryBackend()
	h := glacier.NewHandler(bk)
	h.AccountID = testAccountID
	h.DefaultRegion = testRegion

	const vaultName = "delete-writesince-vault"
	const archiveID = "seed-archive-1"

	bk.AddVaultInternal(testAccountID, testRegion, &glacier.Vault{
		VaultName:                       vaultName,
		NumberOfArchives:                1,
		SizeInBytes:                     10,
		NumberOfArchivesAtLastInventory: new(int64),
		WriteSinceLastInventory:         new(false),
	})
	bk.AddArchiveInternal(testAccountID, testRegion, vaultName, &glacier.Archive{ArchiveID: archiveID, Size: 10})

	deleteArchiveHTTP(t, h, vaultName, archiveID)

	assertVaultDeleteRejected(t, h, vaultName, deleteVaultHTTP(t, h, vaultName))

	initiateInventoryJobHTTP(t, h, vaultName)

	rec := deleteVaultHTTP(t, h, vaultName)
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

// TestDeleteVault_WriteSinceLastInventory_CompleteMultipartUpload pins
// multipart_uploads.go's CompleteMultipartUpload setting
// WriteSinceLastInventory. This is the vault's only write and no inventory
// ever runs before the first delete attempt, isolating this site from
// UploadArchive/DeleteArchive's own flag writes.
func TestDeleteVault_WriteSinceLastInventory_CompleteMultipartUpload(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	const vaultName = "complete-mp-writesince-vault"
	createVault(t, h, vaultName)

	rec := doRequestWithHeaders(t, h, http.MethodPost,
		"/"+testAccountID+"/vaults/"+vaultName+"/multipart-uploads", "",
		map[string]string{"X-Amz-Part-Size": "1048576"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	uploadID := rec.Header().Get("X-Amz-Multipart-Upload-Id")
	require.NotEmpty(t, uploadID)

	partBody := strings.Repeat("a", 1<<20)
	rec = doRequestWithHeaders(t, h, http.MethodPut,
		"/"+testAccountID+"/vaults/"+vaultName+"/multipart-uploads/"+uploadID,
		partBody, map[string]string{"Content-Range": "bytes 0-1048575/*"})
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	headers := map[string]string{
		"X-Amz-Archive-Size":     "1048576",
		"X-Amz-Sha256-Tree-Hash": glacier.ComputeTreeHash([]byte(partBody)),
	}
	rec = doRequestWithHeaders(t, h, http.MethodPost,
		"/"+testAccountID+"/vaults/"+vaultName+"/multipart-uploads/"+uploadID, "", headers)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var completeResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &completeResp))
	archiveID, _ := completeResp["archiveId"].(string)
	require.NotEmpty(t, archiveID)

	assertVaultDeleteRejected(t, h, vaultName, deleteVaultHTTP(t, h, vaultName))

	deleteArchiveHTTP(t, h, vaultName, archiveID)
	initiateInventoryJobHTTP(t, h, vaultName)

	rec = deleteVaultHTTP(t, h, vaultName)
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}
