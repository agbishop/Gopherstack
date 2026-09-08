package glacier

import (
	"sort"
	"time"
)

// archiveIDLength is the length of the random archive ID suffix.
const archiveIDLength = 60

// cloneArchive returns a shallow copy of an Archive.
func cloneArchive(a *Archive) *Archive {
	cp := *a

	return &cp
}

// UploadArchive uploads an archive to a vault.
func (b *InMemoryBackend) UploadArchive(
	accountID, region, vaultName, description, checksum string,
	size int64, data []byte,
) (*Archive, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	v, ok := b.vaults.Get(vaultARN(accountID, region, vaultName))
	if !ok {
		return nil, ErrVaultNotFound
	}

	archiveID := generateID(archiveIDLength)
	a := &Archive{
		ArchiveID:      archiveID,
		Description:    description,
		CreationDate:   formatDate(time.Now()),
		Size:           size,
		SHA256TreeHash: checksum,
	}

	if v.Archives == nil {
		v.Archives = make(map[string]*Archive)
	}

	v.Archives[archiveID] = a
	b.archiveData[archiveID] = append([]byte(nil), data...)
	v.NumberOfArchives++
	v.SizeInBytes += size
	v.WriteSinceLastInventory = new(true)

	return a, nil
}

// DeleteArchive deletes an archive from a vault.
//
// Per api_op_DeleteArchive.go's doc comment, DeleteArchive is idempotent:
// "Attempting to delete an already-deleted archive does not result in an
// error." This emulator has no record of which archive IDs previously
// existed, so -- like AWS's documented idempotent retry case -- deleting an
// archiveID not currently present in an existing vault is a silent no-op
// rather than ErrArchiveNotFound.
func (b *InMemoryBackend) DeleteArchive(accountID, region, vaultName, archiveID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	vArn := vaultARN(accountID, region, vaultName)

	v, ok := b.vaults.Get(vArn)
	if !ok {
		return ErrVaultNotFound
	}

	a, ok := v.Archives[archiveID]
	if !ok {
		return nil
	}

	if err := b.checkVaultLockDelete(vArn, glacierActionDeleteArchive, a.CreationDate); err != nil {
		return err
	}

	if v.NumberOfArchives > 0 {
		v.NumberOfArchives--
	}

	if v.SizeInBytes >= a.Size {
		v.SizeInBytes -= a.Size
	} else {
		v.SizeInBytes = 0
	}

	delete(v.Archives, archiveID)
	delete(b.archiveData, archiveID)
	v.WriteSinceLastInventory = new(true)

	return nil
}

// ListArchives returns all archives for the given vault.
func (b *InMemoryBackend) ListArchives(accountID, region, vaultName string) ([]*Archive, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	v, ok := b.vaults.Get(vaultARN(accountID, region, vaultName))
	if !ok {
		return nil, ErrVaultNotFound
	}

	result := make([]*Archive, 0, len(v.Archives))

	for _, a := range v.Archives {
		result = append(result, cloneArchive(a))
	}

	sort.Slice(result, func(i, j int) bool { return result[i].ArchiveID < result[j].ArchiveID })

	return result, nil
}

// GetArchiveData returns the data for an archive.
func (b *InMemoryBackend) GetArchiveData(archiveID string) ([]byte, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	data, ok := b.archiveData[archiveID]
	if !ok {
		return nil, false
	}

	return data, true
}

// AddArchiveInternal adds an archive directly to the backend for testing.
// It does not update the vault's NumberOfArchives or SizeInBytes counters;
// callers that need accounting to be correct should update the vault via AddVaultInternal.
//
// It is a no-op if the vault does not already exist: Archives is stored
// inline on Vault (see models.go), so there is no vault-less orphan slot to
// write into -- and, just as before this conversion, every other archive-
// reading path (ListArchives, DeleteArchive, InitiateJob, ...) already
// requires the vault to exist first, so a pre-conversion "orphan" archive
// entry was equally unreachable.
func (b *InMemoryBackend) AddArchiveInternal(accountID, region, vaultName string, a *Archive) {
	b.mu.Lock()
	defer b.mu.Unlock()

	v, ok := b.vaults.Get(vaultARN(accountID, region, vaultName))
	if !ok {
		return
	}

	if v.Archives == nil {
		v.Archives = make(map[string]*Archive)
	}

	v.Archives[a.ArchiveID] = cloneArchive(a)
	b.archiveData[a.ArchiveID] = make([]byte, a.Size)
}
