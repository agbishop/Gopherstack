package azureblob

import (
	"context"
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// azureBlobSnapshotVersion identifies the shape of backendSnapshot. Must be
// bumped whenever a change to storedContainer/storedBlob would make an older
// snapshot unsafe to decode as the current shape; Restore compares this
// against the persisted value and discards (rather than partially decodes)
// any mismatch, mirroring services/s3 and services/sqs.
const azureBlobSnapshotVersion = 1

// backendSnapshot is the top-level on-disk shape for the Azure Blob backend.
// Containers serialises directly (no DTO layer): storedContainer/storedBlob
// have no unexported fields, so encoding/json round-trips them as-is.
type backendSnapshot struct {
	Containers map[string]*storedContainer `json:"containers"`
	Version    int                         `json:"version"`
}

// Snapshot serialises the backend state to JSON. It implements
// persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	snap := backendSnapshot{
		Version:    azureBlobSnapshotVersion,
		Containers: b.containers,
	}

	return persistence.MarshalSnapshot(ctx, "azureblob", snap)
}

// Restore loads backend state from a JSON snapshot. It implements
// persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "azureblob", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	// etagSeq is process-local, not part of backendSnapshot. Left at zero it
	// restarts at 1, so an identical-content overwrite after a restore
	// reproduces the pre-restart ETag.
	b.etagSeq = uint64(time.Now().UnixNano())

	if snap.Version != azureBlobSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- discard cleanly and start
		// empty instead of erroring, since this is an expected, recoverable
		// condition (e.g. upgrading gopherstack across a snapshot-format
		// change), not data corruption. Mirrors services/s3 and services/sqs.
		logger.Load(ctx).WarnContext(ctx,
			"azureblob: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", azureBlobSnapshotVersion)

		b.containers = make(map[string]*storedContainer)

		return nil
	}

	if snap.Containers == nil {
		snap.Containers = make(map[string]*storedContainer)
	}

	for name, c := range snap.Containers {
		// A JSON `null` value at "containers"[name] decodes to a nil
		// *storedContainer without error; leaving it in place would panic
		// the first time anything dereferences it (e.g. c.Blobs below, or
		// storedBlob.info() for a null blob entry). Reject the whole
		// snapshot rather than silently dropping or fabricating an entry.
		if c == nil {
			return fmt.Errorf("%w: %q", ErrSnapshotContainerNull, name)
		}

		if c.Blobs == nil {
			c.Blobs = make(map[string]*storedBlob)

			continue
		}

		for blobName, blob := range c.Blobs {
			if blob == nil {
				return fmt.Errorf("%w: %q in container %q", ErrSnapshotBlobNull, blobName, name)
			}
		}
	}

	b.containers = snap.Containers

	return nil
}

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
			return fmt.Errorf("azureblob: restore snapshot: %w", err)
		}
	}

	return nil
}
