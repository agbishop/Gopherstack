package mediastoredata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// contentSHA256 returns the hex-encoded SHA-256 digest of body.
func contentSHA256(body []byte) string {
	sum := sha256.Sum256(body)

	return hex.EncodeToString(sum[:])
}

// cloneObject returns a shallow copy of obj. Body is shared (CoW: objects are
// immutable after storage, so callers only read the body, never mutate it).
func cloneObject(obj *Object) *Object {
	cp := *obj

	return &cp
}

// PutObject stores an object at the given path.
// Returns ErrInvalidPath if path is malformed or ErrInvalidStorageClass if
// storageClass is unrecognised.
func (b *InMemoryBackend) PutObject(
	ctx context.Context,
	path string, body []byte, contentType, cacheControl, storageClass, uploadAvailability string,
) (*Object, error) {
	if err := ValidatePath(path); err != nil {
		return nil, err
	}

	if storageClass == "" {
		storageClass = "TEMPORAL"
	} else if !isValidStorageClass(storageClass) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidStorageClass, storageClass)
	}

	if uploadAvailability == "" {
		uploadAvailability = "STANDARD"
	} else if !isValidUploadAvailability(uploadAvailability) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidUploadAvailability, uploadAvailability)
	}

	maxSize := maxObjectSizeStandard
	if uploadAvailability == "STREAMING" {
		maxSize = maxObjectSizeStreaming
	}

	if len(body) > maxSize {
		return nil, fmt.Errorf("%w: object size %d exceeds the %d-byte limit for %s upload availability",
			ErrObjectTooLarge, len(body), maxSize, uploadAvailability)
	}

	b.mu.Lock("PutObject")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)
	key := normalizePath(path)

	// Clone the input body to prevent callers mutating the stored slice.
	stored := append([]byte(nil), body...)
	sha := contentSHA256(stored)
	obj := &Object{
		Path:               key,
		Body:               stored,
		SHA256:             sha,
		ETag:               fmt.Sprintf(`"%s"`, sha),
		ContentType:        contentType,
		CacheControl:       cacheControl,
		StorageClass:       storageClass,
		LastModified:       time.Now().UTC(),
		ContentLength:      int64(len(stored)),
		UploadAvailability: uploadAvailability,
	}
	b.state(region).Put(obj)

	return cloneObject(obj), nil
}

// GetObject retrieves an object by path.
func (b *InMemoryBackend) GetObject(ctx context.Context, path string) (*Object, error) {
	if err := ValidatePath(path); err != nil {
		return nil, err
	}

	b.mu.RLock("GetObject")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	tbl := b.stateRO(region)

	if tbl == nil {
		return nil, fmt.Errorf("%w: object %q not found", ErrNotFound, path)
	}

	key := normalizePath(path)
	obj, ok := tbl.Get(key)

	if !ok {
		return nil, fmt.Errorf("%w: object %q not found", ErrNotFound, path)
	}

	return cloneObject(obj), nil
}

// DeleteObject removes an object by path.
func (b *InMemoryBackend) DeleteObject(ctx context.Context, path string) error {
	if err := ValidatePath(path); err != nil {
		return err
	}

	b.mu.Lock("DeleteObject")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)
	tbl := b.stateRO(region)

	if tbl == nil {
		return fmt.Errorf("%w: object %q not found", ErrNotFound, path)
	}

	key := normalizePath(path)
	if !tbl.Delete(key) {
		return fmt.Errorf("%w: object %q not found", ErrNotFound, path)
	}

	return nil
}

// UpdateObjectMetadata updates content-type and cache-control on an existing
// object without re-uploading the body. Returns ErrNotFound if path is absent.
//
// Not a real MediaStore Data API operation (the SDK has none -- PutObject
// always overwrites the full object). This backs the dashboard-only PATCH
// /dashboard/api/mediastoredata/objects endpoint (dashboard/ui.go's
// registerMediaStoreDataUpdateMetadataRoute), a gopherstack-internal UI
// convenience, not a wire-routed AWS operation -- see gopherstack-vxmb.
func (b *InMemoryBackend) UpdateObjectMetadata(ctx context.Context, path, contentType, cacheControl string) error {
	if err := ValidatePath(path); err != nil {
		return err
	}

	b.mu.Lock("UpdateObjectMetadata")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.defaultRegion)
	tbl := b.stateRO(region)

	if tbl == nil {
		return fmt.Errorf("%w: object %q not found", ErrNotFound, path)
	}

	key := normalizePath(path)
	obj, ok := tbl.Get(key)

	if !ok {
		return fmt.Errorf("%w: object %q not found", ErrNotFound, path)
	}

	// In-place mutation is safe here: Path (the table's primary key) is not
	// being changed, and this table has no [store.Index] registered, so
	// there is no stale-index risk (see .claude/memories/parity-principles.md).
	obj.ContentType = contentType
	obj.CacheControl = cacheControl
	obj.LastModified = time.Now().UTC()

	return nil
}
