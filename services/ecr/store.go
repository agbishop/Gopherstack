package ecr

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// compile-time assertion: InMemoryBackend must satisfy the Backend interface.
var _ Backend = (*InMemoryBackend)(nil)

// InMemoryBackend stores ECR repository state in memory.
type InMemoryBackend struct {
	// registry lets Reset/Snapshot/Restore collapse the resource-table
	// lifecycle to one call each (registry.ResetAll/SnapshotAll/RestoreAll)
	// instead of hand-rolled per-map wiring. See pkgs/store's package doc and
	// the services/sqs pilot (commit 0f09d77c) for the pattern this follows.
	// See store_setup.go for every table registered on it, and for the
	// fields deliberately left as plain maps below instead.
	registry                    *store.Registry
	repos                       *store.Table[Repository]
	images                      *store.Table[Image]
	imagesByRepo                *store.Index[Image]
	imageScanFindings           *store.Table[ImageScanFindingsResult]
	imageScanFindingsByRepo     *store.Index[ImageScanFindingsResult]
	pullThroughCacheRules       *store.Table[PullThroughCacheRule]
	repositoryCreationTemplates *store.Table[RepositoryCreationTemplate]
	lifecyclePolicies           *store.Table[lifecyclePolicyEntry]
	lifecyclePolicyPreviews     *store.Table[LifecyclePolicyPreviewResult]
	repositoryPolicies          *store.Table[repositoryPolicyEntry]
	accountSettings             *store.Table[accountSettingEntry]
	pullTimeUpdateExclusions    *store.Table[PullTimeUpdateExclusion]

	// The following are deliberately left as plain maps -- see the doc above
	// registerAllTables in store_setup.go for why each one is exempt.
	repoTags               map[string]map[string]string
	signingConfig          *SigningSettings
	tagIndex               map[string]map[string]string
	digestTagsIndex        map[string]map[string][]string
	uploadedLayers         map[string]map[string]int64
	layerUploads           map[string]*layerUploadState
	repoUploadIndex        map[string]map[string]struct{}
	mu                     *lockmetrics.RWMutex
	registryScanningConfig *RegistryScanningSettings
	replicationConfig      *ReplicationConfig
	lifecycleLastEvaluated map[string]time.Time
	registryPolicy         string
	accountID              string
	region                 string
	endpoint               string
	layerUploadQueue       []layerUploadQueueEntry
	layerUploadSeq         uint64
	replicationSettleDelay time.Duration
}

// NewInMemoryBackend creates a new InMemoryBackend with the given account ID and region.
func NewInMemoryBackend(accountID, region, endpoint string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:               store.NewRegistry(),
		tagIndex:               make(map[string]map[string]string),
		digestTagsIndex:        make(map[string]map[string][]string),
		uploadedLayers:         make(map[string]map[string]int64),
		layerUploads:           make(map[string]*layerUploadState),
		repoUploadIndex:        make(map[string]map[string]struct{}),
		layerUploadQueue:       make([]layerUploadQueueEntry, 0),
		repoTags:               make(map[string]map[string]string),
		registryScanningConfig: &RegistryScanningSettings{ScanType: scanTypeBasic},
		replicationConfig:      &ReplicationConfig{},
		lifecycleLastEvaluated: make(map[string]time.Time),
		mu:                     lockmetrics.New("ecr"),
		accountID:              accountID,
		region:                 region,
		endpoint:               endpoint,
	}

	registerAllTables(b)

	return b
}

// SetEndpoint updates the registry endpoint used in repository URIs.
func (b *InMemoryBackend) SetEndpoint(endpoint string) {
	b.mu.Lock("SetEndpoint")
	defer b.mu.Unlock()

	b.endpoint = endpoint
}

// ProxyEndpoint returns the registry endpoint used in repository URIs and
// authorization tokens. It satisfies the Backend interface.
func (b *InMemoryBackend) ProxyEndpoint() string {
	b.mu.RLock("ProxyEndpoint")
	defer b.mu.RUnlock()

	return b.endpoint
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string {
	b.mu.RLock("Region")
	defer b.mu.RUnlock()

	return b.region
}

// AccountID returns the AWS account ID associated with this registry.
func (b *InMemoryBackend) AccountID() string {
	b.mu.RLock("AccountID")
	defer b.mu.RUnlock()

	return b.accountID
}

// regionFor resolves the region to use for a request, preferring the per-request
// region carried on the context (via pkgs/awsmeta) and falling back to the
// backend's configured region when the context carries none.
func (b *InMemoryBackend) regionFor(ctx context.Context) string {
	if r := awsmeta.Region(ctx); r != "" {
		return r
	}

	return b.region
}

// Reset clears all state in the backend.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.tagIndex = make(map[string]map[string]string)
	b.digestTagsIndex = make(map[string]map[string][]string)
	b.lifecycleLastEvaluated = make(map[string]time.Time)
	b.uploadedLayers = make(map[string]map[string]int64)
	b.layerUploads = make(map[string]*layerUploadState)
	b.repoUploadIndex = make(map[string]map[string]struct{})
	b.layerUploadQueue = make([]layerUploadQueueEntry, 0)
	b.layerUploadSeq = 0
	b.repoTags = make(map[string]map[string]string)
	b.registryPolicy = ""
	b.registryScanningConfig = &RegistryScanningSettings{ScanType: scanTypeBasic}
	b.replicationConfig = &ReplicationConfig{}
	b.signingConfig = nil
}
