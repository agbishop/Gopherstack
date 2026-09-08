package glacier

var _ StorageBackend = (*InMemoryBackend)(nil)

// StorageBackend is the interface for the Glacier backend.
type StorageBackend interface {
	CreateVault(accountID, region, vaultName string) (*Vault, error)
	DescribeVault(accountID, region, vaultName string) (*Vault, error)
	DeleteVault(accountID, region, vaultName string) error
	ListVaults(accountID, region string) []*Vault

	UploadArchive(accountID, region, vaultName, description, checksum string, size int64, data []byte) (*Archive, error)
	DeleteArchive(accountID, region, vaultName, archiveID string) error
	ListArchives(accountID, region, vaultName string) ([]*Archive, error)
	GetArchiveData(archiveID string) ([]byte, bool)

	InitiateJob(accountID, region, vaultName string, req *initiateJobRequest) (*Job, error)
	DescribeJob(accountID, region, vaultName, jobID string) (*Job, error)
	ListJobs(accountID, region, vaultName string) ([]*Job, error)

	SetVaultNotifications(accountID, region, vaultName, snsTopic string, events []string) error
	GetVaultNotifications(accountID, region, vaultName string) (string, []string, error)
	DeleteVaultNotifications(accountID, region, vaultName string) error

	SetVaultAccessPolicy(accountID, region, vaultName, policy string) error
	GetVaultAccessPolicy(accountID, region, vaultName string) (string, error)
	DeleteVaultAccessPolicy(accountID, region, vaultName string) error

	AddTagsToVault(accountID, region, vaultName string, tags map[string]string) error
	ListTagsForVault(accountID, region, vaultName string) (map[string]string, error)
	RemoveTagsFromVault(accountID, region, vaultName string, tagKeys []string) error

	// Multipart upload operations.
	InitiateMultipartUpload(accountID, region, vaultName, description string, partSize int64) (*MultipartUpload, error)
	UploadMultipartPart(accountID, region, vaultName, uploadID, rangeHeader, checksum string, data []byte) error
	CompleteMultipartUpload(
		accountID, region, vaultName, uploadID, checksum string,
		archiveSize int64,
	) (*Archive, error)
	AbortMultipartUpload(accountID, region, vaultName, uploadID string) error
	ListMultipartUploads(accountID, region, vaultName string) []*MultipartUpload
	ListParts(accountID, region, vaultName, uploadID string) (*ListPartsOutput, error)

	// Vault lock operations.
	GetVaultLock(accountID, region, vaultName string) (*VaultLock, error)
	SetVaultLock(accountID, region, vaultName, policy, lockID string) error
	AbortVaultLock(accountID, region, vaultName string) error
	CompleteVaultLock(accountID, region, vaultName, lockID string) error

	// Data retrieval policy operations.
	GetDataRetrievalPolicy(accountID string) string
	SetDataRetrievalPolicy(accountID string, policy []byte)

	// Provisioned capacity operations.
	ListProvisionedCapacity(accountID string) []*ProvisionedCapacity
	PurchaseProvisionedCapacity(accountID string) (*ProvisionedCapacity, error)

	// SetJobInventorySize persists the computed InventorySizeInBytes on a completed
	// inventory-retrieval job so that subsequent DescribeJob calls return it.
	SetJobInventorySize(accountID, region, vaultName, jobID string, size int64)

	Reset()
}
