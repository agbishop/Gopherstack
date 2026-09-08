package odatatable

import (
	"net/url"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
)

// minTimestampBump is the smallest amount a stored entity's Timestamp is
// guaranteed to advance on each mutation. Real Table Storage Timestamps have
// 100ns ("tick") resolution; forcing at least this much forward movement
// even when nowFunc returns the same instant twice in a row (e.g. a fast
// test clock, or two mutations landing in the same wall-clock tick)
// guarantees ETag uniqueness across successive writes to the same entity --
// see etagFor.
const minTimestampBump = 100 * time.Nanosecond

// etagTimeLayout formats a Timestamp into an Azure Table Storage-style ETag
// body: a fixed 7-fractional-digit RFC3339 string (100ns "tick" precision),
// url-encoded as a whole by etagFor.
const etagTimeLayout = "2006-01-02T15:04:05.0000000Z"

// etagFor derives an ETag from an entity's Timestamp, in the wire format
// real Azure Table Storage uses: W/"datetime'<url-encoded RFC3339>'". Cosmos
// DB's Table API is wire-identical, so it reuses this format too.
func etagFor(t time.Time) string {
	return `W/"datetime'` + url.QueryEscape(t.UTC().Format(etagTimeLayout)) + `'"`
}

// InMemoryBackend implements StorageBackend using an in-memory map guarded
// by a single RWMutex. Shaped after services/azurequeue's InMemoryBackend.
type InMemoryBackend struct {
	mu     *lockmetrics.RWMutex
	tables map[string]*storedTable
	// nowFunc is the backend's time source, overridable in tests (see
	// SetNowFunc) for deterministic Timestamp/ETag assertions.
	nowFunc func() time.Time
	// etagFunc derives an entity's ETag from its Timestamp, overridable in
	// tests (see SetETagFunc) for deterministic ETag assertions independent
	// of the real wire format.
	etagFunc func(time.Time) string
}

// NewInMemoryBackend creates a new empty InMemoryBackend.
func NewInMemoryBackend() *InMemoryBackend {
	return &InMemoryBackend{
		mu:       lockmetrics.New("odatatable"),
		tables:   make(map[string]*storedTable),
		nowFunc:  time.Now,
		etagFunc: etagFor,
	}
}

func (b *InMemoryBackend) now() time.Time { return b.nowFunc().UTC() }

// entityKey builds the comparable map key for a (partitionKey, rowKey) pair.
// See entityCompositeKey's doc comment (models.go) for why this is a struct,
// not a delimited string.
func entityKey(partitionKey, rowKey string) entityCompositeKey {
	return entityCompositeKey{PartitionKey: partitionKey, RowKey: rowKey}
}

// CreateTable creates a new, empty table. Returns ErrTableAlreadyExists if a
// table with the same name already exists -- unlike services/azurequeue's
// CreateQueue, Table Storage has no metadata-identical-retry idempotency
// exception; a duplicate Create is always a conflict.
func (b *InMemoryBackend) CreateTable(name string) error {
	b.mu.Lock("CreateTable")
	defer b.mu.Unlock()

	if _, ok := b.tables[name]; ok {
		return ErrTableAlreadyExists
	}

	b.tables[name] = &storedTable{Name: name, Entities: make(map[entityCompositeKey]*storedEntity)}

	return nil
}

// DeleteTable removes a table and all of its entities. Returns
// ErrTableNotFound if the table does not exist.
func (b *InMemoryBackend) DeleteTable(name string) error {
	b.mu.Lock("DeleteTable")
	defer b.mu.Unlock()

	if _, ok := b.tables[name]; !ok {
		return ErrTableNotFound
	}

	delete(b.tables, name)

	return nil
}

// ListTables returns a snapshot of all tables, sorted by name (the order
// Azure's List Tables returns them in).
func (b *InMemoryBackend) ListTables() []TableInfo {
	b.mu.RLock("ListTables")
	defer b.mu.RUnlock()

	out := make([]TableInfo, 0, len(b.tables))
	for _, t := range b.tables {
		out = append(out, TableInfo{Name: t.Name})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// InsertEntity creates a new entity in table. Returns ErrTableNotFound if
// the table does not exist, or ErrEntityAlreadyExists if an entity with the
// same PartitionKey/RowKey already exists.
func (b *InMemoryBackend) InsertEntity(
	table, partitionKey, rowKey string, props map[string]EntityProperty,
) (EntityInfo, error) {
	b.mu.Lock("InsertEntity")
	defer b.mu.Unlock()

	t, ok := b.tables[table]
	if !ok {
		return EntityInfo{}, ErrTableNotFound
	}

	key := entityKey(partitionKey, rowKey)
	if _, exists := t.Entities[key]; exists {
		return EntityInfo{}, ErrEntityAlreadyExists
	}

	e := &storedEntity{
		PartitionKey: partitionKey,
		RowKey:       rowKey,
		Timestamp:    b.now(),
		Properties:   cloneProps(props),
	}
	t.Entities[key] = e

	return b.info(e), nil
}

// GetEntity retrieves a single entity. Returns ErrTableNotFound or
// ErrEntityNotFound as appropriate.
func (b *InMemoryBackend) GetEntity(table, partitionKey, rowKey string) (EntityInfo, error) {
	b.mu.RLock("GetEntity")
	defer b.mu.RUnlock()

	t, ok := b.tables[table]
	if !ok {
		return EntityInfo{}, ErrTableNotFound
	}

	e, ok := t.Entities[entityKey(partitionKey, rowKey)]
	if !ok {
		return EntityInfo{}, ErrEntityNotFound
	}

	return b.info(e), nil
}

// QueryEntities returns entities in table matching filter (nil matches all),
// ordered by (PartitionKey, RowKey), capped at top results (top <= 0 means
// unlimited). Returns ErrTableNotFound if the table does not exist.
func (b *InMemoryBackend) QueryEntities(table string, filter Node, top int) ([]EntityInfo, error) {
	b.mu.RLock("QueryEntities")
	defer b.mu.RUnlock()

	t, ok := b.tables[table]
	if !ok {
		return nil, ErrTableNotFound
	}

	entities := make([]*storedEntity, 0, len(t.Entities))
	for _, e := range t.Entities {
		entities = append(entities, e)
	}

	sort.Slice(entities, func(i, j int) bool {
		if entities[i].PartitionKey != entities[j].PartitionKey {
			return entities[i].PartitionKey < entities[j].PartitionKey
		}

		return entities[i].RowKey < entities[j].RowKey
	})

	out := make([]EntityInfo, 0, len(entities))

	for _, e := range entities {
		info := b.info(e)
		if filter != nil && !EvaluateFilter(filter, info) {
			continue
		}

		out = append(out, info)

		if top > 0 && len(out) >= top {
			break
		}
	}

	return out, nil
}

// checkIfMatch validates ifMatch against an entity's current existence/ETag,
// per StorageBackend's If-Match state doc comment.
func (b *InMemoryBackend) checkIfMatch(e *storedEntity, exists bool, ifMatch string) error {
	switch ifMatch {
	case "":
		return nil
	case IfMatchAny:
		if !exists {
			return ErrEntityNotFound
		}

		return nil
	default:
		if !exists {
			return ErrEntityNotFound
		}

		if b.etagFunc(e.Timestamp) != ifMatch {
			return ErrETagMismatch
		}

		return nil
	}
}

// bumpTimestamp returns the Timestamp a mutated entity should receive:
// b.now() for a brand-new entity, or b.now() advanced by at least
// minTimestampBump past the entity's previous Timestamp for an existing one
// -- guaranteeing a distinct ETag on every mutation (see minTimestampBump).
func (b *InMemoryBackend) bumpTimestamp(e *storedEntity, existedBefore bool) time.Time {
	now := b.now()
	if !existedBefore {
		return now
	}

	if !now.After(e.Timestamp) {
		return e.Timestamp.Add(minTimestampBump)
	}

	return now
}

// ReplaceEntity fully replaces an existing entity's properties, or (ifMatch
// == "") inserts a new one if absent (Insert-Or-Replace / upsert). See
// StorageBackend's doc comment for ifMatch's three states.
func (b *InMemoryBackend) ReplaceEntity(
	table, partitionKey, rowKey string, props map[string]EntityProperty, ifMatch string,
) (EntityInfo, error) {
	b.mu.Lock("ReplaceEntity")
	defer b.mu.Unlock()

	t, ok := b.tables[table]
	if !ok {
		return EntityInfo{}, ErrTableNotFound
	}

	key := entityKey(partitionKey, rowKey)
	e, exists := t.Entities[key]

	if err := b.checkIfMatch(e, exists, ifMatch); err != nil {
		return EntityInfo{}, err
	}

	if !exists {
		e = &storedEntity{PartitionKey: partitionKey, RowKey: rowKey}
		t.Entities[key] = e
	}

	e.Timestamp = b.bumpTimestamp(e, exists)
	e.Properties = cloneProps(props)

	return b.info(e), nil
}

// MergeEntity merges props into an existing entity's properties (properties
// not present in props are left unaffected), or (ifMatch == "") inserts a
// new entity if absent (Insert-Or-Merge / upsert). See StorageBackend's doc
// comment for ifMatch's three states.
func (b *InMemoryBackend) MergeEntity(
	table, partitionKey, rowKey string, props map[string]EntityProperty, ifMatch string,
) (EntityInfo, error) {
	b.mu.Lock("MergeEntity")
	defer b.mu.Unlock()

	t, ok := b.tables[table]
	if !ok {
		return EntityInfo{}, ErrTableNotFound
	}

	key := entityKey(partitionKey, rowKey)
	e, exists := t.Entities[key]

	if err := b.checkIfMatch(e, exists, ifMatch); err != nil {
		return EntityInfo{}, err
	}

	if !exists {
		e = &storedEntity{PartitionKey: partitionKey, RowKey: rowKey, Properties: make(map[string]EntityProperty)}
		t.Entities[key] = e
	}

	e.Timestamp = b.bumpTimestamp(e, exists)

	if e.Properties == nil {
		e.Properties = make(map[string]EntityProperty, len(props))
	}

	// Deep-copy each incoming property (not maps.Copy, which only copies the
	// map's key/value pairs, not what an EdmBinary Value's []byte points at
	// -- see cloneProp) so a caller mutating the []byte it passed in later
	// can never silently corrupt stored state.
	for name, prop := range props {
		e.Properties[name] = cloneProp(prop)
	}

	return b.info(e), nil
}

// DeleteEntity removes an entity after verifying ifMatch. Returns
// ErrTableNotFound, ErrEntityNotFound, or ErrETagMismatch as appropriate.
// Callers always pass a non-empty ifMatch ("*" or a specific ETag): an
// absent If-Match is rejected at the wire layer before reaching the backend.
func (b *InMemoryBackend) DeleteEntity(table, partitionKey, rowKey, ifMatch string) error {
	b.mu.Lock("DeleteEntity")
	defer b.mu.Unlock()

	t, ok := b.tables[table]
	if !ok {
		return ErrTableNotFound
	}

	key := entityKey(partitionKey, rowKey)

	e, exists := t.Entities[key]
	if err := b.checkIfMatch(e, exists, ifMatch); err != nil {
		return err
	}

	delete(t.Entities, key)

	return nil
}

// Reset clears all in-memory state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.tables = make(map[string]*storedTable)
}

// info returns a read-only EntityInfo snapshot of e, including its
// currently-derived ETag.
func (b *InMemoryBackend) info(e *storedEntity) EntityInfo {
	return EntityInfo{
		PartitionKey: e.PartitionKey,
		RowKey:       e.RowKey,
		Timestamp:    e.Timestamp,
		Properties:   cloneProps(e.Properties),
		ETag:         b.etagFunc(e.Timestamp),
	}
}

// cloneProps returns a copy of props deep enough that mutating anything
// reachable from the result can never affect backend state, or vice versa.
// The map itself is always copied (so a caller can't add/remove entries
// through a reference handed back to them), and each EdmBinary property's
// []byte Value is copied too via cloneProp: a map-only ("shallow") copy
// still shares the same backing array between the caller's slice and the
// stored one, so mutating either through its own reference -- with no
// Timestamp bump and no ETag change -- would silently corrupt the other and
// defeat optimistic concurrency entirely.
func cloneProps(props map[string]EntityProperty) map[string]EntityProperty {
	out := make(map[string]EntityProperty, len(props))
	for k, v := range props {
		out[k] = cloneProp(v)
	}

	return out
}

// cloneProp returns p with its Value deep-copied if (and only if) that
// Value is a []byte (EdmBinary); every other EDM type's Value is either an
// immutable Go value (string/int32/int64/float64/bool) or time.Time (also
// safe to copy by value), so only EdmBinary needs special handling here.
func cloneProp(p EntityProperty) EntityProperty {
	if p.Type == EdmBinary {
		if b, ok := p.Value.([]byte); ok {
			p.Value = append([]byte(nil), b...)
		}
	}

	return p
}

// --- Test-support hooks ---
//
// These are exported (not confined to an _test.go file) because
// InMemoryBackend's nowFunc/etagFunc fields are unexported and callers such
// as services/azuretable's export_test.go -- itself outside this package --
// need a seam to override them for deterministic Timestamp/ETag assertions.

// SetNowFunc replaces the backend's time provider with fn for deterministic
// testing of Timestamp/ETag logic without real sleeps.
func SetNowFunc(b *InMemoryBackend, fn func() time.Time) {
	b.nowFunc = fn
}

// SetETagFunc replaces the backend's ETag derivation function with fn for
// deterministic ETag assertions.
func SetETagFunc(b *InMemoryBackend, fn func(time.Time) string) {
	b.etagFunc = fn
}

// EtagFor exposes etagFor for external tests.
func EtagFor(t time.Time) string {
	return etagFor(t)
}
