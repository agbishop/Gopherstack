package glacier

import "time"

// ExportedInitiateJobRequest is an alias for the unexported initiateJobRequest type,
// made available for use in external (_test) test packages.
type ExportedInitiateJobRequest = initiateJobRequest

// SetRetrievalDelay overrides the simulated asynchronous retrieval window for newly
// initiated jobs (for testing only). A zero delay makes jobs complete immediately.
func SetRetrievalDelay(b *InMemoryBackend, d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.retrievalDelay = d
}

// ComputeTreeHash exposes the internal tree-hash computation for testing.
func ComputeTreeHash(data []byte) string {
	return computeTreeHash(data)
}

// ValidateDescription exposes description validation for testing.
func ValidateDescription(s string) error {
	return validateDescription(s)
}

// ValidateVaultName exposes vault name validation for testing.
func ValidateVaultName(name string) error {
	return validateVaultName(name)
}

// CsvField exposes the CSV field encoder for testing.
func CsvField(s string) string {
	return csvField(s)
}

// IsValidMultipartRange exposes the multipart Content-Range validator for testing.
func IsValidMultipartRange(rangeHeader string) bool {
	return isValidMultipartRange(rangeHeader)
}

// GetVaultLastInventoryDate returns the LastInventoryDate for the named vault.
func GetVaultLastInventoryDate(b *InMemoryBackend, accountID, region, vaultName string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	v, ok := b.vaults.Get(vaultARN(accountID, region, vaultName))
	if !ok {
		return ""
	}

	return v.LastInventoryDate
}

// GetVaultLockState returns the State field for the named vault's lock.
// Returns "Unlocked" if no lock exists.
func GetVaultLockState(b *InMemoryBackend, accountID, region, vaultName string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	lock, ok := b.vaultLocks.Get(vaultARN(accountID, region, vaultName))
	if !ok {
		return "Unlocked"
	}

	return lock.State
}

// SetVaultLockExpired backdates the expiration of an InProgress vault lock so that
// expiry logic can be exercised without real time passing.
func SetVaultLockExpired(b *InMemoryBackend, accountID, region, vaultName string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if lock, ok := b.vaultLocks.Get(vaultARN(accountID, region, vaultName)); ok {
		lock.ExpirationDate = "2000-01-01T00:00:00.000Z"
	}
}

// VaultCount returns the number of vaults in the backend (for testing only).
func VaultCount(b *InMemoryBackend) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.vaults.Len()
}

// ArchiveCount returns the total number of archives across all vaults (for testing only).
func ArchiveCount(b *InMemoryBackend) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	total := 0

	for _, v := range b.vaults.All() {
		total += len(v.Archives)
	}

	return total
}

// ArchiveDataCount returns the number of raw archive byte payloads held in
// b.archiveData (for testing only). Kept separate from ArchiveCount since
// archiveData outlives its owning vault's Archive entry across Reset() --
// see gopherstack-xvm1.
func ArchiveDataCount(b *InMemoryBackend) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.archiveData)
}

// MultipartUploadCount returns the total number of in-progress multipart uploads (for testing only).
func MultipartUploadCount(b *InMemoryBackend) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.multipartUploads.Len()
}

// ProvisionedCapacityCount returns the total number of provisioned capacity units (for testing only).
func ProvisionedCapacityCount(b *InMemoryBackend) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	total := 0

	for _, caps := range b.provisionedCapacity {
		total += len(caps)
	}

	return total
}

// VaultLockCount returns the number of vault locks (for testing only).
func VaultLockCount(b *InMemoryBackend) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.vaultLocks.Len()
}

// JobCount returns the total number of jobs across all vaults (for testing only).
func JobCount(b *InMemoryBackend) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.jobs.Len()
}

// VaultIndexCount returns the number of entries in the vaultsByAccountRegion index
// for a given accountID and region (for testing only).
func VaultIndexCount(b *InMemoryBackend, accountID, region string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.vaultsByAccountRegion.Get(acctRegionKey(accountID, region)))
}

// SetJobCreationDate backdates a job's CreationDate (for testing only) so ordering
// logic can be exercised deterministically without relying on real time.Now() gaps.
func SetJobCreationDate(b *InMemoryBackend, accountID, region, vaultName, jobID, creationDate string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	vArn := vaultARN(accountID, region, vaultName)
	if j, ok := b.jobs.Get(jobKey(vArn, jobID)); ok {
		j.CreationDate = creationDate
	}
}
