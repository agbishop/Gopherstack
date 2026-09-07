package glacier

import (
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/ptrconv"
)

// cloneVault returns a deep copy of the vault with Tags and NotificationEvents cloned.
func cloneVault(v *Vault) *Vault {
	cp := *v
	cp.Tags = maps.Clone(v.Tags)
	cp.NotificationEvents = append([]string(nil), v.NotificationEvents...)

	return &cp
}

// CreateVault creates a new Glacier vault.
func (b *InMemoryBackend) CreateVault(accountID, region, vaultName string) (*Vault, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if vaultName == "" {
		return nil, ErrValidation
	}

	vArn := vaultARN(accountID, region, vaultName)

	if b.vaults.Has(vArn) {
		return nil, ErrResourceInUse
	}

	v := &Vault{
		VaultName:                       vaultName,
		VaultARN:                        vArn,
		AccountID:                       accountID,
		Region:                          region,
		CreationDate:                    formatDate(time.Now()),
		Tags:                            make(map[string]string),
		Archives:                        make(map[string]*Archive),
		NumberOfArchivesAtLastInventory: new(int64),
		SizeInBytesAtLastInventory:      new(int64),
		WriteSinceLastInventory:         new(bool),
	}
	b.vaults.Put(v)

	return v, nil
}

// DescribeVault returns vault metadata.
func (b *InMemoryBackend) DescribeVault(accountID, region, vaultName string) (*Vault, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	v, ok := b.vaults.Get(vaultARN(accountID, region, vaultName))
	if !ok {
		return nil, ErrVaultNotFound
	}

	return cloneVault(v), nil
}

// DeleteVault deletes a vault.
func (b *InMemoryBackend) DeleteVault(accountID, region, vaultName string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	vArn := vaultARN(accountID, region, vaultName)

	v, ok := b.vaults.Get(vArn)
	if !ok {
		return ErrVaultNotFound
	}

	if err := b.checkVaultLockDelete(vArn, glacierActionDeleteVault, ""); err != nil {
		return err
	}

	// Per api_op_DeleteVault.go's doc comment: delete only if there are no
	// archives as of the last inventory and no writes since -- not whether
	// the vault is empty right now.
	//
	// A nil NumberOfArchivesAtLastInventory means this vault came from a
	// snapshot taken before gopherstack-x8em added these fields (CreateVault
	// always sets them, so a live vault is never nil here). Fall back to the
	// pre-x8em check -- live archive count -- rather than treating the
	// missing fields as "empty as of inventory" (gopherstack-c8sa).
	if v.NumberOfArchivesAtLastInventory == nil {
		if len(v.Archives) > 0 {
			return ErrVaultNotEmpty
		}
	} else if *v.NumberOfArchivesAtLastInventory > 0 || ptrconv.Bool(v.WriteSinceLastInventory) {
		return ErrVaultNotEmpty
	}

	// Cascade-delete jobs and multipart uploads scoped to this vault. The
	// index results must be cloned before deleting in the loop: Table.Delete
	// mutates the very index slice Get returned.
	for _, j := range slices.Clone(b.jobsByVault.Get(vArn)) {
		b.jobs.Delete(jobKey(vArn, j.JobID))
	}

	for _, up := range slices.Clone(b.multipartUploadsByVault.Get(vArn)) {
		b.multipartUploads.Delete(multipartUploadKey(vArn, up.MultipartUploadID))
		// multipartParts is a plain map (not a *store.Table -- see store_setup.go's
		// package doc), so it is NOT covered by the b.multipartUploads.Delete above
		// and must be cleaned up explicitly here too, exactly as
		// AbortMultipartUpload/CompleteMultipartUpload already do for a single
		// upload -- otherwise deleting a vault with in-progress multipart uploads
		// leaves orphaned rows behind forever.
		uKey := uploadKey{
			AccountID: accountID, Region: region, VaultName: vaultName,
			UploadID: up.MultipartUploadID,
		}
		delete(b.multipartParts, uKey)
		delete(b.multipartPartData, uKey)
	}

	b.vaultLocks.Delete(vArn)
	b.vaults.Delete(vArn)

	return nil
}

// sortedVaultNames returns a sorted copy of the vaults slice ordered by VaultName.
func sortedVaultNames(vaults []*Vault) []*Vault {
	sorted := make([]*Vault, len(vaults))
	copy(sorted, vaults)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].VaultName < sorted[j].VaultName
	})

	return sorted
}

// ListVaults returns all vaults for the given account and region.
func (b *InMemoryBackend) ListVaults(accountID, region string) []*Vault {
	b.mu.RLock()
	defer b.mu.RUnlock()

	group := b.vaultsByAccountRegion.Get(acctRegionKey(accountID, region))
	result := make([]*Vault, 0, len(group))

	for _, v := range group {
		result = append(result, cloneVault(v))
	}

	return sortedVaultNames(result)
}

// AddVaultInternal adds a vault directly to the backend for testing.
//
// VaultARN, AccountID, and Region are always (re)computed from the accountID
// and region parameters rather than trusted from v, mirroring how
// CreateVault derives them: they are what key and index the vault in the
// vaults *store.Table, so an untrusted/stale value here would silently
// misfile (or collide with) the entry -- see the "Watch mutating-key" note
// in store_setup.go's package doc.
func (b *InMemoryBackend) AddVaultInternal(accountID, region string, v *Vault) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cp := *v
	cp.VaultARN = vaultARN(accountID, region, v.VaultName)
	cp.AccountID = accountID
	cp.Region = region

	if cp.Tags == nil {
		cp.Tags = make(map[string]string)
	}

	if cp.Archives == nil {
		cp.Archives = make(map[string]*Archive)
	}

	b.vaults.Put(&cp)
}
