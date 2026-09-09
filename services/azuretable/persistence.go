package azuretable

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/odatatable"
)

// Snapshot/Restore's actual implementation (backendSnapshot,
// InMemoryBackend.Snapshot/Restore, and their null/mismatch validation) now
// lives in pkgs/odatatable (see interfaces.go's package doc comment for the
// M6 extraction this package underwent). Handler.Snapshot/Restore below are
// unchanged: they still delegate to whatever h.Backend implements, so they
// work identically whether Backend is odatatable's InMemoryBackend or any
// other StorageBackend implementation a test substitutes.

// azureTableSnapshotVersion identifies the shape of this service's persisted
// state. Must be bumped whenever a change to the underlying
// odatatable.storedTable/storedEntity shape would make an older snapshot
// unsafe to decode as the current shape -- passed into
// odatatable.NewInMemoryBackend (see store.go), which enforces it on
// Snapshot/Restore. This is the same value (2) this const carried when it
// lived directly in this file, pre-M6 extraction: the shape hasn't changed,
// only which package owns the encode/decode logic.
//
// It is declared here, in services/azuretable's own package, rather than as
// a single value pkgs/odatatable owns internally, specifically so
// pkgs/persistence's snapshot-version guard (see
// snapshotversion_guard_test.go's doc comment) keeps tracking this service's
// persisted-state shape: the guard scans services/*/ packages only, not
// pkgs/, so a const (and matching *Snapshot-suffixed struct, see
// azureTableSnapshot below) living only in pkgs/odatatable would be
// invisible to it -- exactly the coverage regression this fixes.
const azureTableSnapshotVersion = 2

// azureTableSnapshot exists purely so the snapshot-version guard has a
// *Snapshot-suffixed struct (with an int Version field) to find and expand
// in this package -- see azureTableSnapshotVersion's doc comment. The real
// on-disk shape is pkgs/odatatable's own backendSnapshot; this type is never
// constructed or marshaled here. Tables' field type names
// odatatable.StoredTable/StoredEntity are exported aliases onto
// odatatable's actual unexported storedTable/storedEntity (see models.go in
// that package) purely so their names show up here -- the guard cannot see
// past a cross-package type into its own fields (a documented blind spot,
// see pkgs/persistence's scanServiceDir doc comment), so a nested field
// added to storedTable/storedEntity itself is NOT automatically caught by
// this struct; only a rename of this field, its declared type name, or a
// bump of azureTableSnapshotVersion above is. Keep this struct's shape in
// hand-maintained lockstep with pkgs/odatatable/persistence.go's
// backendSnapshot whenever that one changes.
type azureTableSnapshot struct {
	Tables  map[string]*odatatable.StoredTable `json:"tables"`
	Version int                                `json:"version"`
}

// This blank-identifier assignment references azureTableSnapshot so
// unused-type linters don't flag it as dead code: the type exists only for
// the snapshot-version guard's AST scan (see its own doc comment above) and
// is otherwise never constructed.
var _ = (*azureTableSnapshot)(nil)

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	type snapshotter interface {
		Snapshot(ctx context.Context) []byte
	}
	if s, ok := h.Backend.(snapshotter); ok {
		return s.Snapshot(ctx)
	}

	return nil
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	type restorer interface {
		Restore(context.Context, []byte) error
	}
	if r, ok := h.Backend.(restorer); ok {
		if err := r.Restore(ctx, data); err != nil {
			return fmt.Errorf("azuretable: restore snapshot: %w", err)
		}
	}

	return nil
}
