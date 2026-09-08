package lakeformation

import (
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// InMemoryBackend is the in-memory backend for Lake Formation.
type InMemoryBackend struct {
	dataLakeSettings       *DataLakeSettings
	resourceLFTags         map[string][]LFTagPair
	lfTags                 *store.Table[LFTag]
	transactions           *store.Table[transactionInfo]
	dataCellsFilters       *store.Table[DataCellsFilter]
	lfTagExpressions       *store.Table[LFTagExpression]
	resources              *store.Table[ResourceInfo]
	queries                map[string]string
	identityCenterConfigs  *store.Table[IdentityCenterConfiguration]
	permissionsMap         *store.Table[PermissionEntry]
	mu                     *lockmetrics.RWMutex
	registry               *store.Registry
	tableStorageOptimizers map[string][]StorageOptimizer
	tableObjects           map[string][]PartitionedTableObjectsList
	permissionsList        []*PermissionEntry
	lakeFormationOptIns    []*LFOptIn
}

var _ StorageBackend = (*InMemoryBackend)(nil)

// NewInMemoryBackend creates a new in-memory Lake Formation backend.
func NewInMemoryBackend() *InMemoryBackend {
	b := &InMemoryBackend{
		dataLakeSettings:       &DataLakeSettings{},
		permissionsList:        make([]*PermissionEntry, 0),
		lakeFormationOptIns:    make([]*LFOptIn, 0),
		resourceLFTags:         make(map[string][]LFTagPair),
		queries:                make(map[string]string),
		tableStorageOptimizers: make(map[string][]StorageOptimizer),
		tableObjects:           make(map[string][]PartitionedTableObjectsList),
		mu:                     lockmetrics.New("lakeformation"),
		registry:               store.NewRegistry(),
	}

	registerAllTables(b)

	return b
}

// Reset restores the backend to a clean initial state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.resetTablesLocked()
	b.dataLakeSettings = &DataLakeSettings{}
	b.permissionsList = make([]*PermissionEntry, 0)
	b.lakeFormationOptIns = make([]*LFOptIn, 0)
	b.resourceLFTags = make(map[string][]LFTagPair)
	b.queries = make(map[string]string)
	b.tableStorageOptimizers = make(map[string][]StorageOptimizer)
	b.tableObjects = make(map[string][]PartitionedTableObjectsList)
}

// resetTablesLocked resets every store.Table-backed resource field to empty:
// the "clean" tables via one b.registry.ResetAll() call, plus the "dirty"
// transactions table and the permissionsMap derived cache individually since
// neither is registered on b.registry (see store_setup.go). The caller MUST
// hold b.mu for writing.
func (b *InMemoryBackend) resetTablesLocked() {
	b.registry.ResetAll()
	b.transactions.Reset()
	b.permissionsMap.Reset()
}

const defaultMaxResults = 100

// paginate is a simple opaque paginator for slices.
func paginate[T any](items []T, maxResults int, nextToken string, defaultMax int) ([]T, string) {
	pg := page.New(items, nextToken, maxResults, defaultMax)

	return pg.Data, pg.Next
}
