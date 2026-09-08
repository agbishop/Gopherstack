package glacier

import "time"

// Vault stores all metadata and state for a single Glacier vault.
//
// AccountID and Region are not part of any AWS wire response (those are built
// from explicit response DTOs in handler.go) but are needed, alongside
// VaultARN, to key and index Vault in the *store.Table[Vault]/[store.Index]
// pkgs/store conversion -- see store_setup.go. Archives is nested state kept
// INLINE on Vault (rather than its own store.Table) because every access site
// scopes archives by vault and Archive itself carries no natural
// cross-vault identity field to key a flat table by.
type Vault struct {
	Tags     map[string]string   `json:"tags,omitempty"`
	Archives map[string]*Archive `json:"archives,omitempty"`
	// NumberOfArchivesAtLastInventory is the archive count captured when
	// LastInventoryDate was last set (InitiateJob for inventory-retrieval). Per
	// api_op_DeleteVault.go's doc comment, DeleteVault checks this -- "no
	// archives ... as of the last inventory" -- rather than live NumberOfArchives.
	//
	// Pointer, not int64: CreateVault always sets it (to a pointer-to-zero), so
	// nil is reserved for a vault decoded from a pre-gopherstack-x8em snapshot,
	// which never had this field at all. DeleteVault uses that nil to fall back
	// to the pre-x8em live-archive-count check instead of guessing a value
	// (gopherstack-c8sa).
	NumberOfArchivesAtLastInventory *int64 `json:"numberOfArchivesAtLastInventory,omitempty"`
	// SizeInBytesAtLastInventory is SizeInBytes captured at the same point as
	// NumberOfArchivesAtLastInventory: DescribeVaultOutput.SizeInBytes carries
	// the identical "as of the last inventory date" qualification
	// (gopherstack-zpo5). Pointer for the same pre-x8em-snapshot reason as
	// NumberOfArchivesAtLastInventory.
	SizeInBytesAtLastInventory *int64 `json:"sizeInBytesAtLastInventory,omitempty"`
	// WriteSinceLastInventory tracks DeleteVault's other documented condition,
	// "no writes ... since the last inventory": set on any archive add/remove,
	// cleared when LastInventoryDate is refreshed. Pointer for the same
	// pre-x8em-snapshot reason as NumberOfArchivesAtLastInventory.
	WriteSinceLastInventory *bool    `json:"writeSinceLastInventory,omitempty"`
	AccessPolicy            string   `json:"accessPolicy,omitempty"`
	NotificationSNSTopic    string   `json:"notificationSNSTopic,omitempty"`
	VaultARN                string   `json:"vaultARN"`
	VaultName               string   `json:"vaultName"`
	AccountID               string   `json:"accountID"`
	Region                  string   `json:"region"`
	CreationDate            string   `json:"creationDate"`
	LastInventoryDate       string   `json:"lastInventoryDate,omitempty"`
	NotificationEvents      []string `json:"notificationEvents,omitempty"`
	NumberOfArchives        int64    `json:"numberOfArchives"`
	SizeInBytes             int64    `json:"sizeInBytes"`
}

// Archive stores metadata for a single archive uploaded to a vault.
type Archive struct {
	ArchiveID      string `json:"archiveID"`
	Description    string `json:"description,omitempty"`
	CreationDate   string `json:"creationDate"`
	SHA256TreeHash string `json:"sha256TreeHash,omitempty"`
	Size           int64  `json:"size"`
}

// Job stores state for a single Glacier retrieval or inventory job.
type Job struct {
	// readyAt is the simulated time at which an asynchronous retrieval job completes.
	// While time.Now() is before readyAt the job stays InProgress; on read it is then
	// promoted to Succeeded. It is internal state and never serialized.
	readyAt time.Time
	// OutputLocation/SelectParameters are only set for Select jobs -- internal state,
	// echoed back on DescribeJob/ListJobs, and used by handleSelectJobOutput to
	// actually execute the query against the archive (see select.go).
	SelectParameters *selectParametersDTO `json:"selectParameters,omitempty"`
	OutputLocation   *outputLocationDTO   `json:"outputLocation,omitempty"`
	// SHA256TreeHash is the tree hash of the *retrieved range*; per AWS it is only
	// populated once the job has Completed (null while InProgress). For whole-archive
	// retrievals it equals ArchiveSHA256TreeHash.
	SHA256TreeHash string `json:"sha256TreeHash,omitempty"`
	SNSTopic       string `json:"snsTopic,omitempty"`
	Action         string `json:"action"`
	ArchiveID      string `json:"archiveID,omitempty"`
	// ArchiveDescription is the description of the archive being retrieved, copied
	// from the Archive at InitiateJob time. It is not part of the DescribeJob wire
	// response (AWS has no such field there); it exists solely so GetJobOutput can
	// echo it back via the X-Amz-Archive-Description response header, matching
	// real Glacier's GetJobOutputOutput.ArchiveDescription.
	ArchiveDescription string `json:"archiveDescription,omitempty"`
	InventoryFormat    string `json:"inventoryFormat,omitempty"`
	StatusCode         string `json:"statusCode"`
	StatusMessage      string `json:"statusMessage,omitempty"`
	CreationDate       string `json:"creationDate"`
	CompletionDate     string `json:"completionDate,omitempty"`
	Tier               string `json:"tier,omitempty"`
	JobID              string `json:"jobID"`
	// ArchiveSHA256TreeHash is the tree hash of the entire archive, present as soon
	// as the archive-retrieval job is created (it is archive metadata, not
	// job-completion-dependent) -- distinct from SHA256TreeHash on the real wire.
	ArchiveSHA256TreeHash string `json:"archiveSHA256TreeHash,omitempty"`
	JobDescription        string `json:"jobDescription,omitempty"`
	RetrievalByteRange    string `json:"retrievalByteRange,omitempty"`
	// InventoryRetrievalStartDate/EndDate/Limit/Marker hold the (optional)
	// InventoryRetrievalParameters supplied at InitiateJob time for InventoryRetrieval
	// jobs -- internal state, echoed back on DescribeJob/ListJobs via
	// inventoryRetrievalJobDescriptionResponse, and used by handleInventoryJobOutput
	// to filter/paginate the returned inventory (see inventory_retrieval.go).
	InventoryRetrievalStartDate string `json:"inventoryRetrievalStartDate,omitempty"`
	InventoryRetrievalEndDate   string `json:"inventoryRetrievalEndDate,omitempty"`
	InventoryRetrievalLimit     string `json:"inventoryRetrievalLimit,omitempty"`
	InventoryRetrievalMarker    string `json:"inventoryRetrievalMarker,omitempty"`
	VaultName                   string `json:"vaultName"`
	VaultARN                    string `json:"vaultARN"`
	// JobOutputPath is the s3:// URI a Select job's OutputLocation results are
	// written to, echoed on InitiateJob (x-amz-job-output-path header) and
	// DescribeJob/ListJobs.
	JobOutputPath        string `json:"jobOutputPath,omitempty"`
	ArchiveSizeInBytes   int64  `json:"archiveSizeInBytes,omitempty"`
	InventorySizeInBytes int64  `json:"inventorySizeInBytes,omitempty"`
	// SelectOutputWritten marks that this Select job's real S3 output-location
	// objects (job.txt/results/result_manifest.txt, see select_output.go) have
	// already been written, matching real AWS's "written once, never updated"
	// job.txt semantics. Internal state, not part of the DescribeJob wire response;
	// persisted so a restored job never re-writes (and potentially duplicates)
	// output after a snapshot round trip.
	SelectOutputWritten bool `json:"selectOutputWritten,omitempty"`
	Completed           bool `json:"completed"`
}

// vaultLockPolicyRequest is the request body for InitiateVaultLock.
type vaultLockPolicyRequest struct {
	Policy string `json:"Policy"`
}

// vaultNotificationConfig holds the SNS configuration for a vault. GetVaultNotifications
// serves this FLAT at the response root: awsRestjson1_deserializeOpDocumentGetVaultNotificationsOutput
// (which case-matches a "vaultNotificationConfig" wrapper key) exists in
// aws-sdk-go-v2/service/glacier@v1.35.4's deserializers.go but is DEAD CODE -- the real
// op's own HandleDeserialize decodes the body directly via
// awsRestjson1_deserializeDocumentVaultNotificationConfig(&output.VaultNotificationConfig,
// shape), never through that wrapper helper. Confirmed by reading HandleDeserialize
// itself, not the unreachable helper -- see gopherstack-sdk-shape's dead-code trap.
type vaultNotificationConfig struct {
	SNSTopic string   `json:"SNSTopic"`
	Events   []string `json:"Events"`
}

// vaultAccessPolicy wraps the vault access policy document. GetVaultAccessPolicy
// serves this FLAT at the response root for the same dead-code reason as
// vaultNotificationConfig above: awsRestjson1_deserializeOpGetVaultAccessPolicy's live
// HandleDeserialize calls awsRestjson1_deserializeDocumentVaultAccessPolicy(&output.Policy,
// shape) directly; the "policy"-wrapping awsRestjson1_deserializeOpDocumentGetVaultAccessPolicyOutput
// helper is never called.
type vaultAccessPolicy struct {
	Policy string `json:"Policy"`
}

// createVaultResponse is the response body for CreateVault.
type createVaultResponse struct {
	Location string `json:"Location"`
}

// describeVaultResponse is the response body for DescribeVault / ListVaults item.
//
// NumberOfArchives and SizeInBytes are pointers: both are documented as
// returning null until an inventory has run on the vault (gopherstack-zpo5),
// and int64 cannot express that -- omitempty on a zero int64 would also drop
// a genuine "inventory found zero archives" result.
type describeVaultResponse struct {
	NumberOfArchives  *int64 `json:"NumberOfArchives,omitempty"`
	SizeInBytes       *int64 `json:"SizeInBytes,omitempty"`
	VaultARN          string `json:"VaultARN"`
	VaultName         string `json:"VaultName"`
	CreationDate      string `json:"CreationDate"`
	LastInventoryDate string `json:"LastInventoryDate,omitempty"`
}

// listVaultsResponse is the response body for ListVaults.
type listVaultsResponse struct {
	Marker    *string                 `json:"Marker,omitempty"`
	VaultList []describeVaultResponse `json:"VaultList"`
}

// uploadArchiveResponse is the response header/body for UploadArchive.
type uploadArchiveResponse struct {
	ArchiveID string `json:"archiveId"`
	Checksum  string `json:"checksum"`
	Location  string `json:"location"`
}

// initiateJobRequest is the request body for InitiateJob. Its shape matches the real
// SDK's JobParameters type directly: InitiateJobInput has no other top-level members,
// so JobParameters IS the whole request body (confirmed via the real SDK's
// awsRestjson1_serializeOpInitiateJob, which streams awsRestjson1_serializeDocumentJobParameters
// straight onto the request as the httpPayload).
type initiateJobRequest struct {
	InventoryRetrievalParameters *inventoryRetrievalParamsRequest `json:"InventoryRetrievalParameters,omitempty"`
	OutputLocation               *outputLocationDTO               `json:"OutputLocation,omitempty"`
	SelectParameters             *selectParametersDTO             `json:"SelectParameters,omitempty"`
	Type                         string                           `json:"Type"`
	ArchiveID                    string                           `json:"ArchiveId,omitempty"`
	Description                  string                           `json:"Description,omitempty"`
	Tier                         string                           `json:"Tier,omitempty"`
	SNSTopic                     string                           `json:"SNSTopic,omitempty"`
	InventoryFormat              string                           `json:"Format,omitempty"`
	RetrievalByteRange           string                           `json:"RetrievalByteRange,omitempty"`
}

// inventoryRetrievalParamsRequest mirrors the real SDK's InventoryRetrievalJobInput:
// options for a range (date/marker/limit filtered) vault inventory retrieval.
type inventoryRetrievalParamsRequest struct {
	StartDate string `json:"StartDate,omitempty"`
	EndDate   string `json:"EndDate,omitempty"`
	Limit     string `json:"Limit,omitempty"`
	Marker    string `json:"Marker,omitempty"`
}

// inventoryRetrievalJobDescriptionResponse mirrors the real SDK's
// InventoryRetrievalJobDescription: the echoed-back InventoryRetrievalParameters
// nested object on DescribeJob/ListJobs/InitiateJob responses for InventoryRetrieval
// jobs. NOTE: unlike the request-side DTO, this ALSO carries Format -- the real
// GlacierJobDescription has no top-level Format field at all, only this nested one.
type inventoryRetrievalJobDescriptionResponse struct {
	StartDate string `json:"StartDate,omitempty"`
	EndDate   string `json:"EndDate,omitempty"`
	Format    string `json:"Format,omitempty"`
	Limit     string `json:"Limit,omitempty"`
	Marker    string `json:"Marker,omitempty"`
}

// outputLocationDTO mirrors the real SDK's OutputLocation/S3Location: the location
// where Select job results are (nominally) delivered. Reused verbatim for both the
// InitiateJob request and the DescribeJob/ListJobs response -- the real
// GlacierJobDescription.OutputLocation has the identical shape to
// JobParameters.OutputLocation on the wire.
type outputLocationDTO struct {
	S3 *s3LocationDTO `json:"S3,omitempty"`
}

// s3LocationDTO mirrors the real SDK's S3Location. AccessControlList/Encryption/
// Tagging/UserMetadata are accepted and echoed back but otherwise inert: gopherstack
// has no cross-service S3 write-back (see select.go doc comment for how Select job
// output is actually served).
type s3LocationDTO struct {
	Encryption        *s3EncryptionDTO  `json:"Encryption,omitempty"`
	Tagging           map[string]string `json:"Tagging,omitempty"`
	UserMetadata      map[string]string `json:"UserMetadata,omitempty"`
	BucketName        string            `json:"BucketName,omitempty"`
	Prefix            string            `json:"Prefix,omitempty"`
	CannedACL         string            `json:"CannedACL,omitempty"`
	StorageClass      string            `json:"StorageClass,omitempty"`
	AccessControlList []s3GrantDTO      `json:"AccessControlList,omitempty"`
}

// s3GrantDTO mirrors the real SDK's Grant type (an ACL grant on S3Location).
type s3GrantDTO struct {
	Grantee    *s3GranteeDTO `json:"Grantee,omitempty"`
	Permission string        `json:"Permission,omitempty"`
}

// s3GranteeDTO mirrors the real SDK's Grantee type.
type s3GranteeDTO struct {
	Type         string `json:"Type,omitempty"`
	DisplayName  string `json:"DisplayName,omitempty"`
	EmailAddress string `json:"EmailAddress,omitempty"`
	ID           string `json:"ID,omitempty"`
	URI          string `json:"URI,omitempty"`
}

// s3EncryptionDTO mirrors the real SDK's Encryption type.
type s3EncryptionDTO struct {
	EncryptionType string `json:"EncryptionType,omitempty"`
	KMSContext     string `json:"KMSContext,omitempty"`
	KMSKeyID       string `json:"KMSKeyId,omitempty"`
}

// selectParametersDTO mirrors the real SDK's SelectParameters. Reused verbatim for
// both the InitiateJob request and the DescribeJob/ListJobs response.
type selectParametersDTO struct {
	InputSerialization  *inputSerializationDTO  `json:"InputSerialization,omitempty"`
	OutputSerialization *outputSerializationDTO `json:"OutputSerialization,omitempty"`
	Expression          string                  `json:"Expression,omitempty"`
	ExpressionType      string                  `json:"ExpressionType,omitempty"`
}

// inputSerializationDTO mirrors the real SDK's InputSerialization (Select-job source
// format). Only Csv is a real member on the wire -- Glacier Select, like S3 Select,
// only ever supports CSV-encoded archives. The wire key is lowercase "csv" (confirmed
// via aws-sdk-go-v2/service/glacier@v1.35.4's awsRestjson1_serializeDocumentInputSerialization
// / awsRestjson1_deserializeDocumentInputSerialization, both `object.Key("csv")` /
// `case "csv":`) -- an anomaly among glacier's otherwise-PascalCase field names.
type inputSerializationDTO struct {
	Csv *csvInputDTO `json:"csv,omitempty"`
}

// csvInputDTO mirrors the real SDK's CSVInput.
type csvInputDTO struct {
	Comments             string `json:"Comments,omitempty"`
	FieldDelimiter       string `json:"FieldDelimiter,omitempty"`
	FileHeaderInfo       string `json:"FileHeaderInfo,omitempty"`
	QuoteCharacter       string `json:"QuoteCharacter,omitempty"`
	QuoteEscapeCharacter string `json:"QuoteEscapeCharacter,omitempty"`
	RecordDelimiter      string `json:"RecordDelimiter,omitempty"`
}

// outputSerializationDTO mirrors the real SDK's OutputSerialization. Same lowercase
// "csv" wire key anomaly as inputSerializationDTO -- see that type's comment.
type outputSerializationDTO struct {
	Csv *csvOutputDTO `json:"csv,omitempty"`
}

// csvOutputDTO mirrors the real SDK's CSVOutput.
type csvOutputDTO struct {
	FieldDelimiter       string `json:"FieldDelimiter,omitempty"`
	QuoteCharacter       string `json:"QuoteCharacter,omitempty"`
	QuoteEscapeCharacter string `json:"QuoteEscapeCharacter,omitempty"`
	QuoteFields          string `json:"QuoteFields,omitempty"`
	RecordDelimiter      string `json:"RecordDelimiter,omitempty"`
}

// initiateJobResponse is the response for InitiateJob.
type initiateJobResponse struct {
	JobID         string `json:"jobId"`
	Location      string `json:"location"`
	JobOutputPath string `json:"jobOutputPath,omitempty"`
}

// describeJobResponse is the response body for DescribeJob.
type describeJobResponse struct {
	ArchiveSizeInBytes   *int64 `json:"ArchiveSizeInBytes,omitempty"`
	InventorySizeInBytes *int64 `json:"InventorySizeInBytes,omitempty"`
	CompletionDate       string `json:"CompletionDate,omitempty"`
	ArchiveID            string `json:"ArchiveId,omitempty"`
	VaultARN             string `json:"VaultARN"`
	CreationDate         string `json:"CreationDate"`
	StatusCode           string `json:"StatusCode"`
	StatusMessage        string `json:"StatusMessage,omitempty"`
	JobID                string `json:"JobId"`
	Action               string `json:"Action"`
	JobDescription       string `json:"JobDescription,omitempty"`
	JobOutputPath        string `json:"JobOutputPath,omitempty"`
	// InventoryRetrievalParameters is only populated for InventoryRetrieval jobs; per
	// the real GlacierJobDescription shape it is null otherwise. It replaces the
	// invented top-level "Format" field that used to live directly on this struct
	// (the real wire type has no such field -- Format only ever appears nested here).
	InventoryRetrievalParameters *inventoryRetrievalJobDescriptionResponse `json:"InventoryRetrievalParameters,omitempty"`
	// OutputLocation/SelectParameters are only populated for Select jobs.
	OutputLocation   *outputLocationDTO   `json:"OutputLocation,omitempty"`
	SelectParameters *selectParametersDTO `json:"SelectParameters,omitempty"`
	Tier             string               `json:"Tier,omitempty"`
	SHA256TreeHash   string               `json:"SHA256TreeHash,omitempty"`
	// ArchiveSHA256TreeHash is a distinct wire field from SHA256TreeHash: it carries
	// the checksum of the entire archive (always present for a completed archive
	// retrieval job), whereas SHA256TreeHash is the checksum of the retrieved range.
	ArchiveSHA256TreeHash string `json:"ArchiveSHA256TreeHash,omitempty"`
	SNSTopic              string `json:"SNSTopic,omitempty"`
	RetrievalByteRange    string `json:"RetrievalByteRange,omitempty"`
	Completed             bool   `json:"Completed"`
}

// listJobsResponse is the response body for ListJobs.
type listJobsResponse struct {
	Marker  *string               `json:"Marker,omitempty"`
	JobList []describeJobResponse `json:"JobList"`
}

// addTagsRequest is the request body for AddTagsToVault.
type addTagsRequest struct {
	Tags map[string]string `json:"Tags"`
}

// removeTagsRequest is the request body for RemoveTagsFromVault.
type removeTagsRequest struct {
	TagKeys []string `json:"TagKeys"`
}

// listTagsResponse is the response body for ListTagsForVault.
type listTagsResponse struct {
	Tags map[string]string `json:"Tags"`
}

// errorResponse is the standard Glacier error response.
// __type is included because many AWS SDK versions key on it rather than "code".
type errorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Type      string `json:"type"`
	TypeAlias string `json:"__type"`
}

// MultipartUpload holds metadata for an in-progress multipart upload.
type MultipartUpload struct {
	MultipartUploadID  string `json:"MultipartUploadId"`
	VaultARN           string `json:"VaultARN"`
	ArchiveDescription string `json:"ArchiveDescription,omitempty"`
	CreationDate       string `json:"CreationDate"`
	PartSizeInBytes    int64  `json:"PartSizeInBytes"`
}

// MultipartPart holds metadata for a single uploaded part.
type MultipartPart struct {
	RangeInBytes   string `json:"RangeInBytes"`
	SHA256TreeHash string `json:"SHA256TreeHash,omitempty"`
}

// VaultLock holds the state of a vault lock policy.
//
// VaultARN is not part of any AWS wire response (getVaultLockResponse is a
// separate, explicit DTO in handler.go) but is needed to key VaultLock in the
// *store.Table[VaultLock] pkgs/store conversion -- see store_setup.go.
type VaultLock struct {
	VaultARN       string `json:"vaultARN"`
	Policy         string `json:"Policy"`
	LockID         string `json:"LockId,omitempty"`
	State          string `json:"State"`
	CreationDate   string `json:"CreationDate,omitempty"`
	ExpirationDate string `json:"ExpirationDate,omitempty"`
}

// ProvisionedCapacity holds a single provisioned capacity unit.
type ProvisionedCapacity struct {
	CapacityID     string `json:"CapacityId"`
	StartDate      string `json:"StartDate"`
	ExpirationDate string `json:"ExpirationDate"`
}

// initiateMultipartUploadResponse is the response for InitiateMultipartUpload.
type initiateMultipartUploadResponse struct {
	Location          string `json:"location"`
	MultipartUploadID string `json:"uploadId"`
}

// completeMultipartUploadResponse is the response for CompleteMultipartUpload.
type completeMultipartUploadResponse struct {
	ArchiveID string `json:"archiveId"`
	Checksum  string `json:"checksum"`
	Location  string `json:"location"`
}

// listMultipartUploadsResponse is the response for ListMultipartUploads.
type listMultipartUploadsResponse struct {
	Marker      *string           `json:"Marker,omitempty"`
	UploadsList []MultipartUpload `json:"UploadsList"`
}

// ListPartsOutput is the response for ListParts.
type ListPartsOutput struct {
	Marker             *string         `json:"Marker,omitempty"`
	MultipartUploadID  string          `json:"MultipartUploadId"`
	VaultARN           string          `json:"VaultARN"`
	ArchiveDescription string          `json:"ArchiveDescription,omitempty"`
	CreationDate       string          `json:"CreationDate"`
	Parts              []MultipartPart `json:"Parts"`
	PartSizeInBytes    int64           `json:"PartSizeInBytes"`
}

// getVaultLockResponse is the response for GetVaultLock.
type getVaultLockResponse struct {
	CreationDate   string `json:"CreationDate,omitempty"`
	ExpirationDate string `json:"ExpirationDate,omitempty"`
	Policy         string `json:"Policy,omitempty"`
	State          string `json:"State"`
}

// listProvisionedCapacityResponse is the response for ListProvisionedCapacity.
type listProvisionedCapacityResponse struct {
	ProvisionedCapacityList []ProvisionedCapacity `json:"ProvisionedCapacityList"`
}

// purchaseProvisionedCapacityResponse is the response for PurchaseProvisionedCapacity.
type purchaseProvisionedCapacityResponse struct {
	CapacityID string `json:"capacityId"`
}

// inventoryArchiveItem is a single archive entry in a JSON-format inventory job output.
type inventoryArchiveItem struct {
	ArchiveID          string `json:"ArchiveId"`
	ArchiveDescription string `json:"ArchiveDescription"`
	CreationDate       string `json:"CreationDate"`
	SHA256TreeHash     string `json:"SHA256TreeHash"`
	Size               int64  `json:"Size"`
}

// dataRetrievalRule is a single rule in the data retrieval policy.
// BytesPerHour pointer comes first so the struct fits in 16 pointer bytes.
type dataRetrievalRule struct {
	BytesPerHour *int64 `json:"BytesPerHour,omitempty"`
	Strategy     string `json:"Strategy"`
}

// dataRetrievalPolicyBody wraps the Rules slice in the AWS request/response envelope.
type dataRetrievalPolicyBody struct {
	Rules []dataRetrievalRule `json:"Rules"`
}

// dataRetrievalPolicyRequest is the outer envelope for SetDataRetrievalPolicy.
type dataRetrievalPolicyRequest struct {
	Policy dataRetrievalPolicyBody `json:"Policy"`
}

// formatDate formats a [time.Time] as an ISO 8601 timestamp.
func formatDate(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
