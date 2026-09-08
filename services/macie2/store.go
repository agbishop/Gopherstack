package macie2

import (
	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	defaultFrequency = "SIX_HOURS"
	statusEnabled    = "ENABLED"
	statusPaused     = "PAUSED"
	statusDisabled   = "DISABLED"
	defaultMatchDist = int32(50)
	// defaultFindingScore is 2 (Medium): real types.Severity.Score is an
	// integer 1 (Low) to 3 (High), not an arbitrary float.
	defaultFindingScore = int64(2)
	// categoryClassification is the real FindingCategory enum value for a
	// sensitive-data finding ("CLASSIFICATION"); "POLICY" is the other. Not
	// to be confused with the JSON "category" ClassificationDetails
	// substructure of the same finding.
	categoryClassification = "CLASSIFICATION"
	keyType                = "type"
	keyCreatedAt           = "createdAt"
	keyUpdatedAt           = "updatedAt"
	keyJobStatus           = "jobStatus"
	defaultPageSize        = 50

	jobStatusRunning    = "RUNNING"
	jobStatusIdle       = "IDLE"
	jobStatusCancelled  = "CANCELLED"
	jobStatusUserPaused = "USER_PAUSED"

	errResourceNotFound  = "ResourceNotFoundException"
	errConflictException = "ConflictException"
	errValidation        = "ValidationException"
	errMacieNotEnabled   = "AccessDeniedException"

	maxTagCount    = 50
	maxTagKeyLen   = 128
	maxTagValueLen = 256
	maxMatchDist   = int32(300)
	minMatchDist   = int32(1)
)

// InMemoryBackend implements StorageBackend using in-memory maps.
type InMemoryBackend struct {
	mu *lockmetrics.RWMutex
	// registry holds every store.Table-backed resource field so their
	// lifecycle (reset/snapshot/restore) collapses to one Registry call each
	// -- see store_setup.go's registerAllTables. Every table below is
	// "clean" (registered directly, no DTO-registry needed): each value
	// type already carries its own identity as a real (non-json:"-")
	// field.
	registry              *store.Registry
	allowLists            *store.Table[storedAllowList]      // id → allow list
	customDataIDs         *store.Table[storedCustomDataID]   // id → custom data identifier
	findingsFilters       *store.Table[storedFindingsFilter] // id → findings filter
	findings              *store.Table[storedFinding]        // id → finding
	s3Buckets             *store.Table[S3BucketMetadata]     // bucketArn → bucket metadata
	tags                  map[string]map[string]string       // resourceARN → tags
	session               *Session
	classificationJobs    *store.Table[ClassificationJob]             // jobID → job
	members               *store.Table[Member]                        // accountID → member
	invitations           *store.Table[Invitation]                    // invitationID → invitation
	administrator         *AdministratorAccount                       // this account's admin
	orgAdminAccounts      *store.Table[OrgAdminAccount]               // accountID → org admin
	orgConfig             *OrgConfig                                  // org configuration
	autoDiscoveryConfig   *AutoDiscoveryConfig                        // automated discovery config
	autoDiscoveryAccounts *store.Table[AutoDiscoveryAccount]          // accountID → auto discovery
	classExportConfig     *ClassificationExportConfig                 // export config
	classScopes           *store.Table[ClassificationScope]           // scopeID → scope
	findingsPubConfig     *FindingsPublicationConfig                  // findings publication config
	resourceProfiles      *store.Table[ResourceProfile]               // resourceArn → profile
	resourceDetections    map[string][]ResourceProfileDetection       // resourceArn → detections
	revealConfig          *RevealConfiguration                        // reveal config
	sensitivityTemplates  *store.Table[SensitivityInspectionTemplate] // templateID → template
	paginationSecret      string
	accountID             string
	region                string
}

// NewInMemoryBackend constructs a new InMemoryBackend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		mu:                  lockmetrics.New("macie2"),
		accountID:           accountID,
		region:              region,
		registry:            store.NewRegistry(),
		tags:                make(map[string]map[string]string),
		orgConfig:           &OrgConfig{AutoEnable: false},
		autoDiscoveryConfig: &AutoDiscoveryConfig{Status: statusDisabled},
		resourceDetections:  make(map[string][]ResourceProfileDetection),
		paginationSecret:    uuid.New().String(),
	}

	registerAllTables(b)

	return b
}

// listPaginated locks for reading, projects/sorts items, and paginates,
// returning the page plus continuation token. Shared by the List* methods.
func listPaginated[T any, R any](
	b *InMemoryBackend,
	lockName string,
	items []T,
	mapFn func(T) (R, bool),
	sortFn func([]R),
	token string,
	limit int,
) ([]R, string, error) {
	b.mu.RLock(lockName)
	defer b.mu.RUnlock()

	data, next := mapSortPaginate(items, mapFn, sortFn, token, b.paginationSecret, limit)

	return data, next, nil
}

func mapSortPaginate[T any, R any](
	items []T,
	mapFn func(T) (R, bool),
	sortFn func([]R),
	token string,
	secret string,
	limit int,
) ([]R, string) {
	result := make([]R, 0, len(items))

	for _, item := range items {
		if mapped, ok := mapFn(item); ok {
			result = append(result, mapped)
		}
	}

	sortFn(result)

	data, next := paginate(result, token, secret, limit)

	return data, next
}

func paginate[T any](data []T, token, secret string, limit int) ([]T, string) {
	p := page.NewHMAC(data, token, secret, limit, defaultPageSize)

	return p.Data, p.Next
}

// AccountID returns the account ID.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Region returns the region.
func (b *InMemoryBackend) Region() string { return b.region }

// Reset clears all state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.session = nil
	b.registry.ResetAll()
	b.tags = make(map[string]map[string]string)
	b.administrator = nil
	b.orgConfig = &OrgConfig{AutoEnable: false}
	b.autoDiscoveryConfig = &AutoDiscoveryConfig{Status: statusDisabled}
	b.classExportConfig = nil
	b.findingsPubConfig = nil
	b.resourceDetections = make(map[string][]ResourceProfileDetection)
	b.revealConfig = nil
}
