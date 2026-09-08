package fsx

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// storedFileSystem is the persisted form of a FileSystem.
// time.Time is first: non-pointer prefix (wall, ext) reduces GC pointer bytes.
//
// The fields below DNSName/StorageType/... are shared across file-system
// types where the real AWS wire shape happens to reuse the same concept
// (e.g. DeploymentType is a member of LustreConfiguration,
// WindowsConfiguration, OntapConfiguration, and OpenZFSConfiguration alike),
// so one field backs the corresponding member on whichever *Configuration
// block toFileSystem() populates for this FileSystemType.
type storedFileSystem struct {
	CreationTime                  time.Time         `json:"creationTime"`
	Tags                          map[string]string `json:"tags"`
	FileSystemID                  string            `json:"fileSystemId"`
	FileSystemType                string            `json:"fileSystemType"`
	FileSystemTypeVersion         string            `json:"fileSystemTypeVersion,omitempty"`
	Lifecycle                     string            `json:"lifecycle"`
	ResourceARN                   string            `json:"resourceArn"`
	DNSName                       string            `json:"dnsName,omitempty"`
	StorageType                   string            `json:"storageType,omitempty"`
	VpcID                         string            `json:"vpcId,omitempty"`
	OwnerID                       string            `json:"ownerId,omitempty"`
	DeploymentType                string            `json:"deploymentType,omitempty"`
	MountName                     string            `json:"mountName,omitempty"`
	ActiveDirectoryID             string            `json:"activeDirectoryId,omitempty"`
	PreferredSubnetID             string            `json:"preferredSubnetId,omitempty"`
	DailyAutomaticBackupStartTime string            `json:"dailyAutomaticBackupStartTime,omitempty"`
	WeeklyMaintenanceStartTime    string            `json:"weeklyMaintenanceStartTime,omitempty"`
	RootVolumeID                  string            `json:"rootVolumeId,omitempty"`
	SubnetIDs                     []string          `json:"subnetIds,omitempty"`
	NetworkInterfaceIDs           []string          `json:"networkInterfaceIds,omitempty"`
	StorageCapacityGiB            int32             `json:"storageCapacity,omitempty"`
	ThroughputCapacity            int32             `json:"throughputCapacity,omitempty"`
	ThroughputCapacityPerHAPair   int32             `json:"throughputCapacityPerHAPair,omitempty"`
	AutomaticBackupRetentionDays  int32             `json:"automaticBackupRetentionDays,omitempty"`
	HAPairs                       int32             `json:"haPairs,omitempty"`
	CopyTagsToBackups             bool              `json:"copyTagsToBackups,omitempty"`
	CopyTagsToVolumes             bool              `json:"copyTagsToVolumes,omitempty"`
}

func (s *storedFileSystem) toFileSystem() *FileSystem {
	fs := &FileSystem{
		CreationTime:          epochTime(s.CreationTime),
		Tags:                  tagsMapToSlice(s.Tags),
		FileSystemID:          s.FileSystemID,
		FileSystemType:        s.FileSystemType,
		FileSystemTypeVersion: s.FileSystemTypeVersion,
		Lifecycle:             s.Lifecycle,
		ResourceARN:           s.ResourceARN,
		DNSName:               s.DNSName,
		StorageCapacityGiB:    s.StorageCapacityGiB,
		StorageType:           s.StorageType,
		VpcID:                 s.VpcID,
		OwnersID:              s.OwnerID,
		SubnetIDs:             s.SubnetIDs,
		NetworkInterfaceIDs:   s.NetworkInterfaceIDs,
	}

	switch s.FileSystemType {
	case fileSystemTypeLustre:
		fs.LustreConfiguration = s.toLustreConfiguration()
	case fileSystemTypeWindows:
		fs.WindowsConfiguration = s.toWindowsConfiguration()
	case fileSystemTypeONTAP:
		fs.OntapConfiguration = s.toOntapConfiguration()
	case fileSystemTypeOpenZFS:
		fs.OpenZFSConfiguration = s.toOpenZFSConfiguration()
	}

	return fs
}

// toLustreConfiguration always populates a LustreConfiguration block for
// Lustre file systems. The terraform-provider-aws Read path treats a nil
// LustreConfiguration as an empty result, so a Lustre file system must echo
// this back even when the create request sent no LustreConfiguration.
func (s *storedFileSystem) toLustreConfiguration() *LustreConfiguration {
	return &LustreConfiguration{
		DeploymentType: s.DeploymentType,
		MountName:      s.MountName,
		DataRepositoryConfiguration: &DataRepositoryConfiguration{
			Lifecycle: dataRepositoryLifecycleDisabled,
		},
	}
}

func (s *storedFileSystem) toWindowsConfiguration() *WindowsConfiguration {
	return &WindowsConfiguration{
		ActiveDirectoryID:             s.ActiveDirectoryID,
		DailyAutomaticBackupStartTime: s.DailyAutomaticBackupStartTime,
		DeploymentType:                s.DeploymentType,
		PreferredSubnetID:             s.PreferredSubnetID,
		RemoteAdministrationEndpoint:  s.DNSName,
		WeeklyMaintenanceStartTime:    s.WeeklyMaintenanceStartTime,
		AutomaticBackupRetentionDays:  s.AutomaticBackupRetentionDays,
		ThroughputCapacity:            s.ThroughputCapacity,
		CopyTagsToBackups:             s.CopyTagsToBackups,
	}
}

func (s *storedFileSystem) toOntapConfiguration() *OntapConfiguration {
	return &OntapConfiguration{
		Endpoints: &FileSystemEndpoints{
			Management:   &FileSystemEndpoint{DNSName: s.DNSName},
			Intercluster: &FileSystemEndpoint{DNSName: "intercluster." + s.DNSName},
		},
		DailyAutomaticBackupStartTime: s.DailyAutomaticBackupStartTime,
		DeploymentType:                s.DeploymentType,
		PreferredSubnetID:             s.PreferredSubnetID,
		AutomaticBackupRetentionDays:  s.AutomaticBackupRetentionDays,
		HAPairs:                       s.HAPairs,
		ThroughputCapacity:            s.ThroughputCapacity,
		ThroughputCapacityPerHAPair:   s.ThroughputCapacityPerHAPair,
	}
}

func (s *storedFileSystem) toOpenZFSConfiguration() *OpenZFSConfiguration {
	return &OpenZFSConfiguration{
		DailyAutomaticBackupStartTime: s.DailyAutomaticBackupStartTime,
		DeploymentType:                s.DeploymentType,
		PreferredSubnetID:             s.PreferredSubnetID,
		RootVolumeID:                  s.RootVolumeID,
		WeeklyMaintenanceStartTime:    s.WeeklyMaintenanceStartTime,
		AutomaticBackupRetentionDays:  s.AutomaticBackupRetentionDays,
		ThroughputCapacity:            s.ThroughputCapacity,
		CopyTagsToBackups:             s.CopyTagsToBackups,
		CopyTagsToVolumes:             s.CopyTagsToVolumes,
	}
}

// createFileSystemInput holds parameters for CreateFileSystem.
type createFileSystemInput struct {
	LustreConfiguration  *createLustreConfiguration  `json:"LustreConfiguration,omitempty"`
	WindowsConfiguration *createWindowsConfiguration `json:"WindowsConfiguration,omitempty"`
	OntapConfiguration   *createOntapConfiguration   `json:"OntapConfiguration,omitempty"`
	OpenZFSConfiguration *createOpenZFSConfiguration `json:"OpenZFSConfiguration,omitempty"`
	FileSystemType       string                      `json:"FileSystemType"`
	StorageType          string                      `json:"StorageType,omitempty"`
	VpcID                string                      `json:"VpcId,omitempty"`
	ClientRequestToken   string                      `json:"ClientRequestToken,omitempty"`
	Tags                 []Tag                       `json:"Tags,omitempty"`
	SubnetIDs            []string                    `json:"SubnetIds,omitempty"`
	SecurityGroupIDs     []string                    `json:"SecurityGroupIds,omitempty"`
	StorageCapacityGiB   int32                       `json:"StorageCapacity,omitempty"`
}

// createLustreConfiguration mirrors the CreateFileSystemLustreConfiguration
// block sent by the AWS provider for Lustre file systems.
type createLustreConfiguration struct {
	DeploymentType string `json:"DeploymentType,omitempty"`
}

// createWindowsConfiguration mirrors CreateFileSystemWindowsConfiguration.
// ThroughputCapacity is a required member on the real SDK type.
type createWindowsConfiguration struct {
	ActiveDirectoryID             string   `json:"ActiveDirectoryId,omitempty"`
	DailyAutomaticBackupStartTime string   `json:"DailyAutomaticBackupStartTime,omitempty"`
	DeploymentType                string   `json:"DeploymentType,omitempty"`
	PreferredSubnetID             string   `json:"PreferredSubnetId,omitempty"`
	WeeklyMaintenanceStartTime    string   `json:"WeeklyMaintenanceStartTime,omitempty"`
	Aliases                       []string `json:"Aliases,omitempty"`
	AutomaticBackupRetentionDays  int32    `json:"AutomaticBackupRetentionDays,omitempty"`
	ThroughputCapacity            int32    `json:"ThroughputCapacity,omitempty"`
	CopyTagsToBackups             bool     `json:"CopyTagsToBackups,omitempty"`
}

// createOntapConfiguration mirrors CreateFileSystemOntapConfiguration.
// DeploymentType is a required member on the real SDK type.
type createOntapConfiguration struct {
	DailyAutomaticBackupStartTime string `json:"DailyAutomaticBackupStartTime,omitempty"`
	DeploymentType                string `json:"DeploymentType,omitempty"`
	FsxAdminPassword              string `json:"FsxAdminPassword,omitempty"`
	PreferredSubnetID             string `json:"PreferredSubnetId,omitempty"`
	AutomaticBackupRetentionDays  int32  `json:"AutomaticBackupRetentionDays,omitempty"`
	HAPairs                       int32  `json:"HAPairs,omitempty"`
	ThroughputCapacity            int32  `json:"ThroughputCapacity,omitempty"`
	ThroughputCapacityPerHAPair   int32  `json:"ThroughputCapacityPerHAPair,omitempty"`
}

// createOpenZFSConfiguration mirrors CreateFileSystemOpenZFSConfiguration.
// DeploymentType and ThroughputCapacity are required members on the real
// SDK type.
type createOpenZFSConfiguration struct {
	DailyAutomaticBackupStartTime string `json:"DailyAutomaticBackupStartTime,omitempty"`
	DeploymentType                string `json:"DeploymentType,omitempty"`
	PreferredSubnetID             string `json:"PreferredSubnetId,omitempty"`
	WeeklyMaintenanceStartTime    string `json:"WeeklyMaintenanceStartTime,omitempty"`
	AutomaticBackupRetentionDays  int32  `json:"AutomaticBackupRetentionDays,omitempty"`
	ThroughputCapacity            int32  `json:"ThroughputCapacity,omitempty"`
	CopyTagsToBackups             bool   `json:"CopyTagsToBackups,omitempty"`
	CopyTagsToVolumes             bool   `json:"CopyTagsToVolumes,omitempty"`
}

// fileSystemMinCapacity returns the real-AWS minimum StorageCapacity (GiB)
// for fsType, and false if fsType is not one of the four supported types.
func fileSystemMinCapacity(fsType string) (int32, bool) {
	minCapByType := map[string]int32{
		fileSystemTypeLustre:  minStorageCapacityLustre,
		fileSystemTypeWindows: minStorageCapacityWindows,
		fileSystemTypeONTAP:   minStorageCapacityONTAP,
		fileSystemTypeOpenZFS: minStorageCapacityOpenZFS,
	}
	minCap, ok := minCapByType[fsType]

	return minCap, ok
}

// subnetIDPattern matches real AWS's CreateFileSystem SubnetIds member
// pattern: "^(subnet-[0-9a-f]{8,})$".
var subnetIDPattern = regexp.MustCompile(`^subnet-[0-9a-f]{8,}$`)

// securityGroupIDPattern matches real AWS's CreateFileSystem SecurityGroupIds
// member pattern: "^(sg-[0-9a-f]{8,})$".
var securityGroupIDPattern = regexp.MustCompile(`^sg-[0-9a-f]{8,}$`)

// validateSubnetIDs rejects malformed subnet IDs. This is a format-only
// check: gopherstack does not model VPC/subnet/AZ topology, so unlike real
// AWS it cannot also verify a given subnet actually exists or belongs to the
// file system's VPC (real AWS's InvalidNetworkSettings.InvalidSubnetId
// covers both the format case and the existence case; this only catches the
// former).
func validateSubnetIDs(subnetIDs []string) error {
	for _, id := range subnetIDs {
		if !subnetIDPattern.MatchString(id) {
			return fmt.Errorf("%w: SubnetIds contains malformed subnet ID %q", ErrInvalidNetworkSettings, id)
		}
	}

	return nil
}

// validateSecurityGroupIDs rejects malformed security group IDs (format-only
// check; see validateSubnetIDs's doc comment for what's not covered).
func validateSecurityGroupIDs(securityGroupIDs []string) error {
	for _, id := range securityGroupIDs {
		if !securityGroupIDPattern.MatchString(id) {
			return fmt.Errorf(
				"%w: SecurityGroupIds contains malformed security group ID %q", ErrInvalidNetworkSettings, id,
			)
		}
	}

	return nil
}

// applyLustreConfig sets the Lustre-specific fields on fs. LustreConfiguration
// is optional on the real CreateFileSystemInput; an absent block (or an
// absent DeploymentType within it) defaults to SCRATCH_1, matching real AWS.
func applyLustreConfig(fs *storedFileSystem, cfg *createLustreConfiguration) {
	fs.MountName = generateLustreMountName()
	if cfg != nil {
		fs.DeploymentType = cfg.DeploymentType
	}

	if fs.DeploymentType == "" {
		fs.DeploymentType = lustreDeploymentTypeScratch1
	}
}

// applyWindowsConfig sets the Windows-specific fields on fs. Real AWS
// requires WindowsConfiguration with a ThroughputCapacity for every WINDOWS
// CreateFileSystem call.
func applyWindowsConfig(fs *storedFileSystem, cfg *createWindowsConfiguration) error {
	if cfg == nil {
		return ErrMissingFileSystemConfiguration
	}

	if cfg.ThroughputCapacity <= 0 {
		return fmt.Errorf("%w: WindowsConfiguration.ThroughputCapacity is required", ErrValidation)
	}

	fs.ActiveDirectoryID = cfg.ActiveDirectoryID
	fs.DailyAutomaticBackupStartTime = cfg.DailyAutomaticBackupStartTime
	fs.DeploymentType = cfg.DeploymentType
	fs.PreferredSubnetID = cfg.PreferredSubnetID
	fs.WeeklyMaintenanceStartTime = cfg.WeeklyMaintenanceStartTime
	fs.ThroughputCapacity = cfg.ThroughputCapacity
	fs.CopyTagsToBackups = cfg.CopyTagsToBackups
	fs.AutomaticBackupRetentionDays = cfg.AutomaticBackupRetentionDays

	if fs.DeploymentType == "" {
		fs.DeploymentType = windowsDeploymentTypeSingleAZ1
	}

	if fs.AutomaticBackupRetentionDays == 0 {
		fs.AutomaticBackupRetentionDays = defaultAutomaticBackupRetentionDays
	}

	return nil
}

// applyOntapConfig sets the ONTAP-specific fields on fs. Real AWS requires
// OntapConfiguration with a DeploymentType, and either ThroughputCapacity or
// ThroughputCapacityPerHAPair, for every ONTAP CreateFileSystem call.
func applyOntapConfig(fs *storedFileSystem, cfg *createOntapConfiguration) error {
	if cfg == nil {
		return ErrMissingFileSystemConfiguration
	}

	if cfg.DeploymentType == "" {
		return fmt.Errorf("%w: OntapConfiguration.DeploymentType is required", ErrValidation)
	}

	if cfg.ThroughputCapacity <= 0 && cfg.ThroughputCapacityPerHAPair <= 0 {
		return fmt.Errorf(
			"%w: OntapConfiguration.ThroughputCapacity or ThroughputCapacityPerHAPair is required", ErrValidation,
		)
	}

	fs.DailyAutomaticBackupStartTime = cfg.DailyAutomaticBackupStartTime
	fs.DeploymentType = cfg.DeploymentType
	fs.PreferredSubnetID = cfg.PreferredSubnetID
	fs.AutomaticBackupRetentionDays = cfg.AutomaticBackupRetentionDays
	fs.HAPairs = cfg.HAPairs
	fs.ThroughputCapacity = cfg.ThroughputCapacity
	fs.ThroughputCapacityPerHAPair = cfg.ThroughputCapacityPerHAPair

	if fs.HAPairs == 0 {
		fs.HAPairs = defaultHAPairs
	}

	if fs.ThroughputCapacity == 0 {
		fs.ThroughputCapacity = cfg.ThroughputCapacityPerHAPair * fs.HAPairs
	}

	if fs.AutomaticBackupRetentionDays == 0 {
		fs.AutomaticBackupRetentionDays = defaultAutomaticBackupRetentionDays
	}

	return nil
}

// applyOpenZFSConfig sets the OpenZFS-specific fields on fs (except
// RootVolumeID, which the caller assigns after creating the backing root
// volume). Real AWS requires OpenZFSConfiguration with a DeploymentType and
// ThroughputCapacity for every OPENZFS CreateFileSystem call.
func applyOpenZFSConfig(fs *storedFileSystem, cfg *createOpenZFSConfiguration) error {
	if cfg == nil {
		return ErrMissingFileSystemConfiguration
	}

	if cfg.DeploymentType == "" {
		return fmt.Errorf("%w: OpenZFSConfiguration.DeploymentType is required", ErrValidation)
	}

	if cfg.ThroughputCapacity <= 0 {
		return fmt.Errorf("%w: OpenZFSConfiguration.ThroughputCapacity is required", ErrValidation)
	}

	fs.DailyAutomaticBackupStartTime = cfg.DailyAutomaticBackupStartTime
	fs.DeploymentType = cfg.DeploymentType
	fs.PreferredSubnetID = cfg.PreferredSubnetID
	fs.WeeklyMaintenanceStartTime = cfg.WeeklyMaintenanceStartTime
	fs.ThroughputCapacity = cfg.ThroughputCapacity
	fs.CopyTagsToBackups = cfg.CopyTagsToBackups
	fs.CopyTagsToVolumes = cfg.CopyTagsToVolumes
	fs.AutomaticBackupRetentionDays = cfg.AutomaticBackupRetentionDays

	if fs.AutomaticBackupRetentionDays == 0 {
		fs.AutomaticBackupRetentionDays = defaultAutomaticBackupRetentionDays
	}

	return nil
}

// applyFileSystemTypeConfig dispatches to the per-type config applier for
// fs.FileSystemType, keeping CreateFileSystem short and its complexity
// low enough to need no complexity-suppression comment.
func applyFileSystemTypeConfig(fs *storedFileSystem, input *createFileSystemInput) error {
	switch fs.FileSystemType {
	case fileSystemTypeLustre:
		applyLustreConfig(fs, input.LustreConfiguration)

		return nil
	case fileSystemTypeWindows:
		return applyWindowsConfig(fs, input.WindowsConfiguration)
	case fileSystemTypeONTAP:
		return applyOntapConfig(fs, input.OntapConfiguration)
	case fileSystemTypeOpenZFS:
		return applyOpenZFSConfig(fs, input.OpenZFSConfiguration)
	default:
		return nil
	}
}

// fsCreateTokenEntry records the fingerprint and resulting file system ID for
// a ClientRequestToken accepted by CreateFileSystem, so a retried request can
// be matched against the original one under lock. Fields are exported so
// this round-trips through backendSnapshot's plain-map JSON encoding (see
// persistence.go) instead of silently encoding as "{}" the way an
// unexported-field struct would.
type fsCreateTokenEntry struct {
	Fingerprint  string `json:"fingerprint"`
	FileSystemID string `json:"fileSystemId"`
}

// fingerprintCreateFileSystemInput returns a canonical encoding of the
// resource-shaping parameters of a CreateFileSystem request (everything
// except ClientRequestToken itself, which is the dedup key, not content to
// compare). Used to detect whether a retried request carrying a
// previously-seen ClientRequestToken supplies the same parameters, per real
// AWS's documented contract: "If a file system with the specified client
// request token exists and the parameters match, CreateFileSystem returns
// the description of the existing file system. If ... the parameters don't
// match, this call returns IncompatibleParameterError." (see AWS docs).
func fingerprintCreateFileSystemInput(input *createFileSystemInput) (string, error) {
	cp := *input
	cp.ClientRequestToken = ""

	b, err := json.Marshal(cp)
	if err != nil {
		return "", fmt.Errorf("canonicalize CreateFileSystem request: %w", err)
	}

	return string(b), nil
}

// dedupCreateFileSystemLocked implements CreateFileSystem's idempotency-token
// contract; must be called with b.mu held. The returned bool reports whether
// the caller should return immediately with the accompanying (*FileSystem,
// error) rather than proceeding to create a new file system.
func (b *InMemoryBackend) dedupCreateFileSystemLocked(token, fingerprint string) (*FileSystem, bool, error) {
	entry, ok := b.createFileSystemTokens[token]
	if !ok {
		return nil, false, nil
	}

	if entry.Fingerprint != fingerprint {
		return nil, true, fmt.Errorf(
			"%w: ClientRequestToken %q was already used to create a file system with different parameters",
			ErrIncompatibleParameter, token,
		)
	}

	existing, found := b.fileSystems.Get(entry.FileSystemID)
	if !found {
		// The file system behind this token was since deleted; treat the
		// retry as a fresh create rather than erroring.
		return nil, false, nil
	}

	return existing.toFileSystem(), true, nil
}

// CreateFileSystem creates a new file system. A non-empty ClientRequestToken
// makes the call idempotent: a repeat call with the same token and the same
// parameters returns the original file system instead of creating a second
// one; a repeat call with the same token but different parameters returns
// IncompatibleParameterError (see dedupCreateFileSystemLocked).
func (b *InMemoryBackend) CreateFileSystem(input *createFileSystemInput) (*FileSystem, error) {
	if input.FileSystemType == "" {
		return nil, ErrValidation
	}

	minCap, ok := fileSystemMinCapacity(input.FileSystemType)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported FileSystemType %q", ErrValidation, input.FileSystemType)
	}

	if input.StorageCapacityGiB == 0 {
		input.StorageCapacityGiB = minCap
	} else if input.StorageCapacityGiB < minCap {
		return nil, fmt.Errorf(
			"%w: StorageCapacity %d GiB is below the minimum of %d GiB for %s",
			ErrValidation, input.StorageCapacityGiB, minCap, input.FileSystemType,
		)
	}

	if err := validateCreateTags(input.Tags); err != nil {
		return nil, err
	}

	if err := validateSubnetIDs(input.SubnetIDs); err != nil {
		return nil, err
	}

	if err := validateSecurityGroupIDs(input.SecurityGroupIDs); err != nil {
		return nil, err
	}

	fingerprint, fpErr := fingerprintCreateFileSystemInput(input)
	if fpErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrValidation, fpErr)
	}

	b.mu.Lock("CreateFileSystem")
	defer b.mu.Unlock()

	if input.ClientRequestToken != "" {
		if fs, done, dedupErr := b.dedupCreateFileSystemLocked(input.ClientRequestToken, fingerprint); done {
			return fs, dedupErr
		}
	}

	id := newFileSystemID()
	arn := b.fsARN(id)
	now := time.Now().UTC()

	tags := tagsSliceToMap(input.Tags)

	fs := &storedFileSystem{
		CreationTime:        now,
		Tags:                tags,
		FileSystemID:        id,
		FileSystemType:      input.FileSystemType,
		Lifecycle:           lifecycleAvailable,
		ResourceARN:         arn,
		DNSName:             fmt.Sprintf("%s.fsx.%s.amazonaws.com", id, b.region),
		StorageCapacityGiB:  input.StorageCapacityGiB,
		StorageType:         input.StorageType,
		VpcID:               input.VpcID,
		OwnerID:             b.accountID,
		SubnetIDs:           input.SubnetIDs,
		NetworkInterfaceIDs: networkInterfaceIDsForSubnets(input.SubnetIDs),
	}

	if err := applyFileSystemTypeConfig(fs, input); err != nil {
		return nil, err
	}

	if fs.FileSystemType == fileSystemTypeOpenZFS {
		fs.RootVolumeID = b.createOpenZFSRootVolumeLocked(fs)
	}

	b.fileSystems.Put(fs)
	b.tags[arn] = tags

	if input.ClientRequestToken != "" {
		b.createFileSystemTokens[input.ClientRequestToken] = fsCreateTokenEntry{
			Fingerprint:  fingerprint,
			FileSystemID: fs.FileSystemID,
		}
	}

	return fs.toFileSystem(), nil
}

// fsxIDHexLen is the hex-character length used for most FSx resource IDs
// (e.g. "fs-0123456789abcdef0").
const fsxIDHexLen = 17

// newFSXHexUUID returns n lowercase hex characters derived from a fresh
// UUID with the separating hyphens stripped, so the result is safe to embed
// directly after a resource ID prefix. uuid.New().String() is hyphenated
// (8-4-4-4-12); a naive [:n] slice embeds literal "-" characters into the ID
// once n crosses a hyphen boundary (mirrors the fix for networkInterfaceIDsForSubnets
// below, and gopherstack-28ce in services/ec2).
func newFSXHexUUID(n int) string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:n]
}

func newFileSystemID() string                { return "fs-" + newFSXHexUUID(fsxIDHexLen) }
func newStorageVirtualMachineID() string     { return "svm-" + newFSXHexUUID(fsxIDHexLen) }
func newDataRepositoryAssociationID() string { return "dra-" + newFSXHexUUID(fsxIDHexLen) }
func newFSxBackupID() string                 { return "backup-" + newFSXHexUUID(fsxIDHexLen) }
func newDataRepositoryTaskID() string        { return "task-" + newFSXHexUUID(fsxIDHexLen) }
func newFileCacheID() string                 { return "fc-" + newFSXHexUUID(fsxIDHexLen) }

const fsxVolumeIDHexLen = 16

func newFSxVolumeID() string { return "fsvol-" + newFSXHexUUID(fsxVolumeIDHexLen) }

const fsxVolumeSnapshotIDHexLen = 12

func newFSxVolumeSnapshotID() string { return "fsvolsnap-" + newFSXHexUUID(fsxVolumeSnapshotIDHexLen) }

// generateLustreMountName returns a short, lowercase alphanumeric mount name in
// the style AWS assigns to Lustre file systems (e.g. "abcd1234").
func generateLustreMountName() string {
	raw := strings.ReplaceAll(uuid.New().String(), "-", "")
	if len(raw) > lustreMountNameLen {
		raw = raw[:lustreMountNameLen]
	}

	return raw
}

// networkInterfaceIDsForSubnets returns one synthetic ENI ID per subnet, as AWS
// attaches an elastic network interface to the file system in each subnet.
func networkInterfaceIDsForSubnets(subnetIDs []string) []string {
	if len(subnetIDs) == 0 {
		return nil
	}

	enis := make([]string, 0, len(subnetIDs))
	for range subnetIDs {
		enis = append(enis, "eni-"+strings.ReplaceAll(uuid.New().String(), "-", "")[:17])
	}

	return enis
}

// DescribeFileSystems returns file systems, optionally filtered by IDs.
func (b *InMemoryBackend) DescribeFileSystems(
	ids []string,
	maxResults int32,
	nextToken string,
) ([]*FileSystem, string, error) {
	b.mu.RLock("DescribeFileSystems")
	defer b.mu.RUnlock()

	if maxResults <= 0 {
		maxResults = maxResultsDefault
	}

	var all []*storedFileSystem

	if len(ids) > 0 {
		for _, id := range ids {
			fs, ok := b.fileSystems.Get(id)
			if !ok {
				return nil, "", ErrFileSystemNotFound
			}

			all = append(all, fs)
		}
	} else {
		all = b.fileSystems.All()

		sort.Slice(all, func(i, j int) bool { return all[i].FileSystemID < all[j].FileSystemID })
	}

	start := 0
	if nextToken != "" {
		for i, fs := range all {
			if fs.FileSystemID == nextToken {
				start = i

				break
			}
		}
	}

	end := min(start+int(maxResults), len(all))
	page := all[start:end]

	var next string
	if end < len(all) {
		next = all[end].FileSystemID
	}

	result := make([]*FileSystem, len(page))
	for i, fs := range page {
		result[i] = fs.toFileSystem()
	}

	return result, next, nil
}

// DeleteFileSystem removes a file system. For ONTAP, real AWS requires every
// SVM and volume to be deleted first and refuses otherwise; for every other
// type it cascades to the child resources real AWS also tears down as part
// of file-system deletion: storage virtual machines (and, transitively,
// their volumes and those volumes' snapshots), directly-attached volumes
// (e.g. an OpenZFS root/child volume), data repository associations, and DNS
// aliases. Backups and data repository tasks are intentionally left alone:
// real AWS backups persist independently of the file system they were taken
// from, and data repository tasks are historical execution records.
func (b *InMemoryBackend) DeleteFileSystem(fileSystemID string) error {
	b.mu.Lock("DeleteFileSystem")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(fileSystemID)
	if !ok {
		return ErrFileSystemNotFound
	}

	if fs.FileSystemType == fileSystemTypeONTAP {
		if err := b.requireNoONTAPChildrenLocked(fileSystemID); err != nil {
			return err
		}
	}

	b.cascadeDeleteFileSystemChildrenLocked(fileSystemID)

	delete(b.aliases, fileSystemID)
	b.fileSystems.Delete(fileSystemID)
	delete(b.tags, fs.ResourceARN)

	return nil
}

// requireNoONTAPChildrenLocked returns ErrValidation if fileSystemID still has
// any storage virtual machine or volume attached. Real AWS requires an ONTAP
// file system's SVMs and volumes to be deleted first (api_op_DeleteFileSystem.go);
// other file system types cascade-delete their children instead. Caller must
// already hold b.mu.
func (b *InMemoryBackend) requireNoONTAPChildrenLocked(fileSystemID string) error {
	var hasSVM bool

	b.storageVirtualMachines.Range(func(s *storedStorageVirtualMachine) bool {
		if s.FileSystemID == fileSystemID {
			hasSVM = true

			return false
		}

		return true
	})

	if hasSVM {
		return fmt.Errorf(
			"%w: file system %s has storage virtual machines; delete them first", ErrValidation, fileSystemID,
		)
	}

	var hasVolume bool

	b.volumes.Range(func(v *storedVolume) bool {
		if v.FileSystemID == fileSystemID {
			hasVolume = true

			return false
		}

		return true
	})

	if hasVolume {
		return fmt.Errorf("%w: file system %s has volumes; delete them first", ErrValidation, fileSystemID)
	}

	return nil
}

// cascadeDeleteFileSystemChildrenLocked removes every SVM, volume, and data
// repository association that belongs to fileSystemID. Caller must already
// hold b.mu.
func (b *InMemoryBackend) cascadeDeleteFileSystemChildrenLocked(fileSystemID string) {
	var svmIDs []string

	b.storageVirtualMachines.Range(func(s *storedStorageVirtualMachine) bool {
		if s.FileSystemID == fileSystemID {
			svmIDs = append(svmIDs, s.StorageVirtualMachineID)
		}

		return true
	})

	for _, id := range svmIDs {
		b.deleteStorageVirtualMachineLocked(id)
	}

	var volumeIDs []string

	b.volumes.Range(func(v *storedVolume) bool {
		if v.FileSystemID == fileSystemID {
			volumeIDs = append(volumeIDs, v.VolumeID)
		}

		return true
	})

	for _, id := range volumeIDs {
		b.deleteVolumeLocked(id)
	}

	var draIDs []string

	b.dataRepositoryAssocs.Range(func(d *storedDataRepositoryAssoc) bool {
		if d.FileSystemID == fileSystemID {
			draIDs = append(draIDs, d.AssociationID)
		}

		return true
	})

	for _, id := range draIDs {
		if d, ok := b.dataRepositoryAssocs.Get(id); ok {
			delete(b.tags, d.ResourceARN)
		}

		b.dataRepositoryAssocs.Delete(id)
	}
}

// updateWindowsConfiguration mirrors UpdateFileSystemWindowsConfiguration.
type updateWindowsConfiguration struct {
	DailyAutomaticBackupStartTime string `json:"DailyAutomaticBackupStartTime,omitempty"`
	WeeklyMaintenanceStartTime    string `json:"WeeklyMaintenanceStartTime,omitempty"`
	AutomaticBackupRetentionDays  int32  `json:"AutomaticBackupRetentionDays,omitempty"`
	ThroughputCapacity            int32  `json:"ThroughputCapacity,omitempty"`
}

// updateOntapConfiguration mirrors UpdateFileSystemOntapConfiguration.
type updateOntapConfiguration struct {
	DailyAutomaticBackupStartTime string `json:"DailyAutomaticBackupStartTime,omitempty"`
	FsxAdminPassword              string `json:"FsxAdminPassword,omitempty"`
	AutomaticBackupRetentionDays  int32  `json:"AutomaticBackupRetentionDays,omitempty"`
	HAPairs                       int32  `json:"HAPairs,omitempty"`
	ThroughputCapacity            int32  `json:"ThroughputCapacity,omitempty"`
	ThroughputCapacityPerHAPair   int32  `json:"ThroughputCapacityPerHAPair,omitempty"`
}

// updateOpenZFSConfiguration mirrors UpdateFileSystemOpenZFSConfiguration.
type updateOpenZFSConfiguration struct {
	DailyAutomaticBackupStartTime string `json:"DailyAutomaticBackupStartTime,omitempty"`
	WeeklyMaintenanceStartTime    string `json:"WeeklyMaintenanceStartTime,omitempty"`
	AutomaticBackupRetentionDays  int32  `json:"AutomaticBackupRetentionDays,omitempty"`
	ThroughputCapacity            int32  `json:"ThroughputCapacity,omitempty"`
}

// updateFileSystemInput holds parameters for UpdateFileSystem.
type updateFileSystemInput struct {
	WindowsConfiguration *updateWindowsConfiguration `json:"WindowsConfiguration,omitempty"`
	OntapConfiguration   *updateOntapConfiguration   `json:"OntapConfiguration,omitempty"`
	OpenZFSConfiguration *updateOpenZFSConfiguration `json:"OpenZFSConfiguration,omitempty"`
	FileSystemID         string                      `json:"FileSystemId"`
	StorageCapacityGiB   int32                       `json:"StorageCapacity,omitempty"`
}

// applyWindowsUpdate applies non-zero fields from cfg onto fs, matching real
// AWS's "only overwrites existing properties with non-null values provided
// in the request" UpdateFileSystem semantics.
func applyWindowsUpdate(fs *storedFileSystem, cfg *updateWindowsConfiguration) {
	if cfg == nil {
		return
	}

	if cfg.DailyAutomaticBackupStartTime != "" {
		fs.DailyAutomaticBackupStartTime = cfg.DailyAutomaticBackupStartTime
	}

	if cfg.WeeklyMaintenanceStartTime != "" {
		fs.WeeklyMaintenanceStartTime = cfg.WeeklyMaintenanceStartTime
	}

	if cfg.AutomaticBackupRetentionDays > 0 {
		fs.AutomaticBackupRetentionDays = cfg.AutomaticBackupRetentionDays
	}

	if cfg.ThroughputCapacity > 0 {
		fs.ThroughputCapacity = cfg.ThroughputCapacity
	}
}

// applyOntapUpdate applies non-zero fields from cfg onto fs.
func applyOntapUpdate(fs *storedFileSystem, cfg *updateOntapConfiguration) {
	if cfg == nil {
		return
	}

	if cfg.DailyAutomaticBackupStartTime != "" {
		fs.DailyAutomaticBackupStartTime = cfg.DailyAutomaticBackupStartTime
	}

	if cfg.AutomaticBackupRetentionDays > 0 {
		fs.AutomaticBackupRetentionDays = cfg.AutomaticBackupRetentionDays
	}

	if cfg.HAPairs > 0 {
		fs.HAPairs = cfg.HAPairs
	}

	if cfg.ThroughputCapacity > 0 {
		fs.ThroughputCapacity = cfg.ThroughputCapacity
	}

	if cfg.ThroughputCapacityPerHAPair > 0 {
		fs.ThroughputCapacityPerHAPair = cfg.ThroughputCapacityPerHAPair
	}
}

// applyOpenZFSUpdate applies non-zero fields from cfg onto fs.
func applyOpenZFSUpdate(fs *storedFileSystem, cfg *updateOpenZFSConfiguration) {
	if cfg == nil {
		return
	}

	if cfg.DailyAutomaticBackupStartTime != "" {
		fs.DailyAutomaticBackupStartTime = cfg.DailyAutomaticBackupStartTime
	}

	if cfg.WeeklyMaintenanceStartTime != "" {
		fs.WeeklyMaintenanceStartTime = cfg.WeeklyMaintenanceStartTime
	}

	if cfg.AutomaticBackupRetentionDays > 0 {
		fs.AutomaticBackupRetentionDays = cfg.AutomaticBackupRetentionDays
	}

	if cfg.ThroughputCapacity > 0 {
		fs.ThroughputCapacity = cfg.ThroughputCapacity
	}
}

// UpdateFileSystem updates a file system's configuration.
func (b *InMemoryBackend) UpdateFileSystem(input *updateFileSystemInput) (*FileSystem, error) {
	b.mu.Lock("UpdateFileSystem")
	defer b.mu.Unlock()

	fs, ok := b.fileSystems.Get(input.FileSystemID)
	if !ok {
		return nil, ErrFileSystemNotFound
	}

	if input.StorageCapacityGiB > 0 {
		fs.StorageCapacityGiB = input.StorageCapacityGiB
	}

	switch fs.FileSystemType {
	case fileSystemTypeWindows:
		applyWindowsUpdate(fs, input.WindowsConfiguration)
	case fileSystemTypeONTAP:
		applyOntapUpdate(fs, input.OntapConfiguration)
	case fileSystemTypeOpenZFS:
		applyOpenZFSUpdate(fs, input.OpenZFSConfiguration)
	}

	return fs.toFileSystem(), nil
}

// createFileSystemFromBackupInput holds parameters for
// CreateFileSystemFromBackup. SubnetIds is a required real
// CreateFileSystemFromBackupInput member (api_op_CreateFileSystemFromBackup.go)
// that was previously entirely absent here, silently discarding every real
// client's subnet placement; now accepted, format-validated (same pattern as
// CreateFileSystem), and echoed back on FileSystem.SubnetIds/NetworkInterfaceIds.
// FileSystemType/VpcID have no counterpart on the real input at all (the
// restored file system's type is always inherited from the source file
// system, never overridable) -- pre-existing, harmless since no real client's
// generated request type can ever populate them.
type createFileSystemFromBackupInput struct {
	FileSystemType        string   `json:"FileSystemType,omitempty"`
	BackupID              string   `json:"BackupId"`
	FileSystemTypeVersion string   `json:"FileSystemTypeVersion,omitempty"`
	StorageType           string   `json:"StorageType,omitempty"`
	VpcID                 string   `json:"VpcId,omitempty"`
	Tags                  []Tag    `json:"Tags,omitempty"`
	SubnetIDs             []string `json:"SubnetIds,omitempty"`
	SecurityGroupIDs      []string `json:"SecurityGroupIds,omitempty"`
	StorageCapacityGiB    int32    `json:"StorageCapacity,omitempty"`
}

// copyFileSystemTypeConfig copies every type-specific config field from src
// onto dst (except MountName and RootVolumeID, which the caller regenerates
// fresh for the new file system rather than reusing the source's). Used by
// CreateFileSystemFromBackup so a restored WINDOWS/ONTAP/OPENZFS file system
// still gets a populated, non-zero-valued config block instead of an
// all-defaults one -- real AWS carries these settings over from the source
// file system a backup was taken from.
func copyFileSystemTypeConfig(dst, src *storedFileSystem) {
	dst.DeploymentType = src.DeploymentType
	dst.ActiveDirectoryID = src.ActiveDirectoryID
	dst.PreferredSubnetID = src.PreferredSubnetID
	dst.DailyAutomaticBackupStartTime = src.DailyAutomaticBackupStartTime
	dst.WeeklyMaintenanceStartTime = src.WeeklyMaintenanceStartTime
	dst.ThroughputCapacity = src.ThroughputCapacity
	dst.ThroughputCapacityPerHAPair = src.ThroughputCapacityPerHAPair
	dst.AutomaticBackupRetentionDays = src.AutomaticBackupRetentionDays
	dst.HAPairs = src.HAPairs
	dst.CopyTagsToBackups = src.CopyTagsToBackups
	dst.CopyTagsToVolumes = src.CopyTagsToVolumes
}

// fileSystemFromBackupFields is the set of scalar fields
// CreateFileSystemFromBackup either takes from the request or, when absent,
// falls back to the source file system's own value.
type fileSystemFromBackupFields struct {
	fsType        string
	fsTypeVersion string
	storageType   string
	capacity      int32
}

// resolveFileSystemFromBackupFields applies input's explicit overrides,
// falling back to srcFS's values (real AWS's "defaults to the parameters of
// the file system that was backed up, unless overridden" contract). srcFS may
// be nil if the source file system has since been deleted.
func resolveFileSystemFromBackupFields(
	input *createFileSystemFromBackupInput,
	srcFS *storedFileSystem,
) fileSystemFromBackupFields {
	f := fileSystemFromBackupFields{
		fsType:        input.FileSystemType,
		fsTypeVersion: input.FileSystemTypeVersion,
		storageType:   input.StorageType,
		capacity:      input.StorageCapacityGiB,
	}

	if srcFS == nil {
		return f
	}

	if f.fsType == "" {
		f.fsType = srcFS.FileSystemType
	}

	if f.fsTypeVersion == "" {
		f.fsTypeVersion = srcFS.FileSystemTypeVersion
	}

	if f.storageType == "" {
		f.storageType = srcFS.StorageType
	}

	if f.capacity == 0 {
		f.capacity = srcFS.StorageCapacityGiB
	}

	return f
}

// CreateFileSystemFromBackup creates a new file system from an existing backup.
func (b *InMemoryBackend) CreateFileSystemFromBackup(input *createFileSystemFromBackupInput) (*FileSystem, error) {
	if err := validateCreateTags(input.Tags); err != nil {
		return nil, err
	}

	if err := validateSubnetIDs(input.SubnetIDs); err != nil {
		return nil, err
	}

	if err := validateSecurityGroupIDs(input.SecurityGroupIDs); err != nil {
		return nil, err
	}

	b.mu.Lock("CreateFileSystemFromBackup")
	defer b.mu.Unlock()

	src, ok := b.backups.Get(input.BackupID)
	if !ok {
		return nil, ErrBackupNotFound
	}

	srcFS, _ := b.fileSystems.Get(src.FileSystemID)
	fields := resolveFileSystemFromBackupFields(input, srcFS)
	fsType := fields.fsType

	id := newFileSystemID()
	arn := b.fsARN(id)
	now := time.Now().UTC()
	tags := tagsSliceToMap(input.Tags)

	fs := &storedFileSystem{
		CreationTime:          now,
		Tags:                  tags,
		FileSystemID:          id,
		FileSystemType:        fsType,
		FileSystemTypeVersion: fields.fsTypeVersion,
		Lifecycle:             lifecycleAvailable,
		ResourceARN:           arn,
		DNSName:               fmt.Sprintf("%s.fsx.%s.amazonaws.com", id, b.region),
		StorageCapacityGiB:    fields.capacity,
		StorageType:           fields.storageType,
		VpcID:                 input.VpcID,
		OwnerID:               b.accountID,
		SubnetIDs:             input.SubnetIDs,
		NetworkInterfaceIDs:   networkInterfaceIDsForSubnets(input.SubnetIDs),
	}

	if srcFS != nil {
		copyFileSystemTypeConfig(fs, srcFS)
	}

	switch fsType {
	case fileSystemTypeLustre:
		fs.MountName = generateLustreMountName()

		if fs.DeploymentType == "" {
			fs.DeploymentType = lustreDeploymentTypeScratch1
		}
	case fileSystemTypeOpenZFS:
		fs.RootVolumeID = b.createOpenZFSRootVolumeLocked(fs)
	}

	b.fileSystems.Put(fs)
	b.tags[arn] = tags

	return fs.toFileSystem(), nil
}

func (b *InMemoryBackend) fsARN(id string) string {
	return arn.Build("fsx", b.region, b.accountID, fmt.Sprintf("file-system/%s", id))
}

// ---------------------------------------------------------------------------
// Aliases
// ---------------------------------------------------------------------------

// AssociateFileSystemAliases adds DNS aliases to a file system.
func (b *InMemoryBackend) AssociateFileSystemAliases(fileSystemID string, aliases []string) ([]FileSystemAlias, error) {
	b.mu.Lock("AssociateFileSystemAliases")
	defer b.mu.Unlock()

	if !b.fileSystems.Has(fileSystemID) {
		return nil, ErrFileSystemNotFound
	}

	existing := make(map[string]struct{}, len(b.aliases[fileSystemID]))
	for _, a := range b.aliases[fileSystemID] {
		existing[a] = struct{}{}
	}

	for _, a := range aliases {
		if _, dup := existing[a]; !dup {
			b.aliases[fileSystemID] = append(b.aliases[fileSystemID], a)
			existing[a] = struct{}{}
		}
	}

	return aliasesToPublic(b.aliases[fileSystemID], "AVAILABLE"), nil
}

// DisassociateFileSystemAliases removes DNS aliases from a file system.
func (b *InMemoryBackend) DisassociateFileSystemAliases(
	fileSystemID string,
	aliases []string,
) ([]FileSystemAlias, error) {
	b.mu.Lock("DisassociateFileSystemAliases")
	defer b.mu.Unlock()

	if !b.fileSystems.Has(fileSystemID) {
		return nil, ErrFileSystemNotFound
	}

	remove := make(map[string]struct{}, len(aliases))
	for _, a := range aliases {
		remove[a] = struct{}{}
	}

	kept := b.aliases[fileSystemID][:0]
	for _, a := range b.aliases[fileSystemID] {
		if _, drop := remove[a]; !drop {
			kept = append(kept, a)
		}
	}

	b.aliases[fileSystemID] = kept

	return aliasesToPublic(aliases, "DELETING"), nil
}

// DescribeFileSystemAliases returns aliases for a file system.
func (b *InMemoryBackend) DescribeFileSystemAliases(
	fileSystemID string,
	maxResults int32,
	nextToken string,
) ([]FileSystemAlias, string, error) {
	b.mu.RLock("DescribeFileSystemAliases")
	defer b.mu.RUnlock()

	if !b.fileSystems.Has(fileSystemID) {
		return nil, "", ErrFileSystemNotFound
	}

	all := b.aliases[fileSystemID]

	if maxResults <= 0 {
		maxResults = maxResultsDefault
	}

	start := 0
	if nextToken != "" {
		for i, a := range all {
			if a == nextToken {
				start = i

				break
			}
		}
	}

	end := min(start+int(maxResults), len(all))
	page := all[start:end]

	var next string
	if end < len(all) {
		next = all[end]
	}

	return aliasesToPublic(page, "AVAILABLE"), next, nil
}

func aliasesToPublic(names []string, lifecycle string) []FileSystemAlias {
	out := make([]FileSystemAlias, len(names))
	for i, n := range names {
		out[i] = FileSystemAlias{Name: n, Lifecycle: lifecycle}
	}

	return out
}

// ---------------------------------------------------------------------------
// Miscellaneous
// ---------------------------------------------------------------------------

// ReleaseFileSystemNfsV3Locks releases NFS v3 locks on a file system.
func (b *InMemoryBackend) ReleaseFileSystemNfsV3Locks(fileSystemID string) error {
	b.mu.RLock("ReleaseFileSystemNfsV3Locks")
	defer b.mu.RUnlock()

	if !b.fileSystems.Has(fileSystemID) {
		return ErrFileSystemNotFound
	}

	return nil
}

// StartMisconfiguredStateRecovery initiates recovery for a misconfigured file system.
func (b *InMemoryBackend) StartMisconfiguredStateRecovery(fileSystemID string) error {
	b.mu.Lock("StartMisconfiguredStateRecovery")
	defer b.mu.Unlock()

	if !b.fileSystems.Has(fileSystemID) {
		return ErrFileSystemNotFound
	}

	return nil
}
