package azuretable

import (
	"context"
	"fmt"
)

// Snapshot/Restore's actual implementation (backendSnapshot,
// InMemoryBackend.Snapshot/Restore, and their null/mismatch validation) now
// lives in pkgs/odatatable (see interfaces.go's package doc comment for the
// M6 extraction this package underwent). Handler.Snapshot/Restore below are
// unchanged: they still delegate to whatever h.Backend implements, so they
// work identically whether Backend is odatatable's InMemoryBackend or any
// other StorageBackend implementation a test substitutes.

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
