package s3

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// s3SnapshotVersion identifies the shape of backendSnapshot's Tables blob (the
// set/shape of resources registered on b.registry -- see NewInMemoryBackend in
// store.go). Must be bumped whenever a change there would make an older snapshot
// unsafe to decode as the current shape; Restore compares this against the
// persisted value and discards (rather than partially decodes) any mismatch.
//
// Bumped to 1 here from the previous versionless shape: any snapshot written
// before this change decodes with Version == 0, fails the guard below, and is
// discarded cleanly rather than silently misinterpreted.
const s3SnapshotVersion = 1

// backendSnapshot is the top-level on-disk shape for the S3 backend.
//
// Tables holds one JSON-encoded array per registered table, produced by
// [store.Registry.SnapshotAll] -- currently "buckets" ([]*StoredBucket) and
// "uploads" ([]*StoredMultipartUpload). Both serialise directly (no DTO layer):
// their only non-serialisable fields (mu, StoredMultipartUpload.closed) are
// unexported, so encoding/json already skips them. Restore re-initialises those
// skipped fields via reinitBucketMutexes/reinitUploadMutexes below.
type backendSnapshot struct {
	Tables        map[string]json.RawMessage `json:"tables"`
	Tags          map[string][]types.Tag     `json:"tags"`
	DefaultRegion string                     `json:"defaultRegion"`
	Version       int                        `json:"version"`
}

// Snapshot serialises the backend state to JSON.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	tables, err := b.registry.SnapshotAll()
	if err != nil {
		// The registered tables are plain JSON-friendly structs, so a marshal
		// failure here would indicate a programming error rather than bad
		// input data. Log and skip the snapshot rather than panic, matching
		// the persistence.Persistable contract (nil is skipped by the Manager).
		logger.Load(ctx).WarnContext(ctx, "s3: snapshot table marshal failed", "error", err)

		return nil
	}

	snap := backendSnapshot{
		Version:       s3SnapshotVersion,
		Tables:        tables,
		Tags:          b.tags,
		DefaultRegion: b.defaultRegion,
	}

	return persistence.MarshalSnapshot(ctx, "s3", snap)
}

// Restore loads backend state from a JSON snapshot.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "s3", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != s3SnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption. Mirrors the services/ec2 and services/sqs pilots.
		logger.Load(ctx).WarnContext(ctx,
			"s3: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", s3SnapshotVersion)

		b.registry.ResetAll()
		b.tags = make(map[string][]types.Tag)

		return nil
	}

	if err := b.registry.RestoreAll(snap.Tables); err != nil {
		return fmt.Errorf("s3: restore snapshot tables: %w", err)
	}

	reinitBucketMutexes(b.buckets.All())
	reinitUploadMutexes(b.uploads.All())

	if snap.Tags != nil {
		b.tags = snap.Tags
	} else {
		b.tags = make(map[string][]types.Tag)
	}

	b.defaultRegion = snap.DefaultRegion

	return nil
}

// reinitBucketMutexes reinitialises per-bucket and per-object mutexes after
// deserialisation, since [lockmetrics.RWMutex] values (held via unexported
// pointer fields, and therefore skipped by encoding/json) cannot be
// serialised.
func reinitBucketMutexes(buckets []*StoredBucket) {
	for _, bucket := range buckets {
		reinitSingleBucket(bucket)
	}
}

// reinitSingleBucket reinitialises one bucket's mutex, nil maps, and per-object mutexes.
func reinitSingleBucket(bucket *StoredBucket) {
	if bucket.mu == nil {
		bucket.mu = lockmetrics.New("s3.bucket." + bucket.Name)
	}

	if bucket.Objects == nil {
		bucket.Objects = make(map[string]*StoredObject)
	}

	if bucket.AnalyticsConfigs == nil {
		bucket.AnalyticsConfigs = make(map[string]string)
	}

	if bucket.IntelligentTieringConfigs == nil {
		bucket.IntelligentTieringConfigs = make(map[string]string)
	}

	if bucket.InventoryConfigs == nil {
		bucket.InventoryConfigs = make(map[string]string)
	}

	if bucket.MetricsConfigs == nil {
		bucket.MetricsConfigs = make(map[string]string)
	}

	for _, obj := range bucket.Objects {
		if obj.mu == nil {
			obj.mu = lockmetrics.New("s3.object")
		}
	}
}

// reinitUploadMutexes reinitialises per-upload mutexes after deserialisation.
func reinitUploadMutexes(uploads []*StoredMultipartUpload) {
	for _, u := range uploads {
		if u.mu == nil {
			u.mu = lockmetrics.New("s3.upload")
		}
	}
}

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *S3Handler) Snapshot(ctx context.Context) []byte {
	type snapshotter interface {
		Snapshot(ctx context.Context) []byte
	}
	if s, ok := h.Backend.(snapshotter); ok {
		return s.Snapshot(ctx)
	}

	return nil
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *S3Handler) Restore(ctx context.Context, data []byte) error {
	type restorer interface {
		Restore(context.Context, []byte) error
	}
	if r, ok := h.Backend.(restorer); ok {
		return r.Restore(ctx, data)
	}

	return nil
}
