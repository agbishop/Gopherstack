package kms

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// DefaultJanitorInterval exposes the package default janitor interval for testing.
const DefaultJanitorInterval = defaultKMSJanitorInterval

// SetTags exposes setTags for testing.
func (h *Handler) SetTags(resourceID string, kv map[string]string) { h.setTags(resourceID, kv) }

// RemoveTags exposes removeTags for testing.
func (h *Handler) RemoveTags(resourceID string, keys []string) { h.removeTags(resourceID, keys) }

// GetTags exposes getTags for testing.
func (h *Handler) GetTags(resourceID string) map[string]string { return h.getTags(resourceID) }

// HandlerOpsLen returns the number of pre-built dispatch operations.
func HandlerOpsLen(h *Handler) int {
	return len(h.actions)
}

// KeyCount returns the total number of keys stored across all regions.
func KeyCount(b *InMemoryBackend) int {
	b.mu.RLock("KeyCount")
	defer b.mu.RUnlock()

	n := 0
	for _, t := range b.keys {
		n += t.Len()
	}

	return n
}

// AliasCount returns the total number of aliases stored across all regions.
func AliasCount(b *InMemoryBackend) int {
	b.mu.RLock("AliasCount")
	defer b.mu.RUnlock()

	n := 0
	for _, t := range b.aliases {
		n += t.Len()
	}

	return n
}

// GrantCount returns the total number of grants stored across all regions.
func GrantCount(b *InMemoryBackend) int {
	b.mu.RLock("GrantCount")
	defer b.mu.RUnlock()

	n := 0
	for _, gs := range b.grants {
		n += gs.table.Len()
	}

	return n
}

// CustomKeyStoreCount returns the total number of custom key stores across all regions.
func CustomKeyStoreCount(b *InMemoryBackend) int {
	b.mu.RLock("CustomKeyStoreCount")
	defer b.mu.RUnlock()

	n := 0
	for _, t := range b.customKeyStores {
		n += t.Len()
	}

	return n
}

// SetDeletionDateForTest directly sets a key's DeletionDate to the given time.
// Used to simulate elapsed deletion windows without sleeping.
func (b *InMemoryBackend) SetDeletionDateForTest(keyID string, t time.Time) {
	b.mu.Lock("SetDeletionDateForTest")
	defer b.mu.Unlock()

	for _, tbl := range b.keys {
		if key, ok := tbl.Get(keyID); ok {
			key.DeletionDate = UnixTimeFloat(t)

			return
		}
	}
}

// GetJanitorInterval returns the Interval configured on the handler's janitor.
// Used in tests to verify WithJanitor correctly propagates the interval.
func (h *Handler) GetJanitorInterval() time.Duration {
	if h.janitor == nil {
		return 0
	}

	return h.janitor.Interval
}

// GetJanitorTaskTimeout returns the TaskTimeout configured on the handler's janitor.
// Used in tests to verify WithJanitor correctly propagates the timeout.
func (h *Handler) GetJanitorTaskTimeout() time.Duration {
	if h.janitor == nil {
		return 0
	}

	return h.janitor.TaskTimeout
}

// ScheduleJanitorExpiry pushes an expiry entry into the janitor's heap for testing.
func (j *Janitor) ScheduleJanitorExpiry(keyID string, fireAt float64, isDeletion bool) {
	kind := expiryKindMaterial
	if isDeletion {
		kind = expiryKindDeletion
	}

	j.scheduleExpiry("", keyID, fireAt, kind)
}

// GrantTokenTTL exposes grantTokenTTL for testing.
const GrantTokenTTL = grantTokenTTL

// SetGrantTokenIssuedAt directly sets a grant's TokenIssuedAt for expiry testing.
func (b *InMemoryBackend) SetGrantTokenIssuedAt(grantID string, t time.Time) {
	b.mu.Lock("SetGrantTokenIssuedAt")
	defer b.mu.Unlock()

	for _, gs := range b.grants {
		if g, ok := gs.table.Get(grantID); ok {
			g.TokenIssuedAt = t

			return
		}
	}
}

// MaxKeyMaterialHistoryEntriesForTest exposes the key material history cap.
const MaxKeyMaterialHistoryEntriesForTest = maxKeyMaterialHistoryEntries

// KeyMaterialHistoryLenForTest returns the number of retained historical key
// materials for the given keyID across all regions.
func (b *InMemoryBackend) KeyMaterialHistoryLenForTest(keyID string) int {
	b.mu.RLock("KeyMaterialHistoryLenForTest")
	defer b.mu.RUnlock()

	for _, regionHist := range b.keyMaterialHistory {
		if hist, ok := regionHist[keyID]; ok {
			return len(hist)
		}
	}

	return 0
}

// GrantsByKeyCount returns the number of entries in the grantsByKey index for the given region+key.
func GrantsByKeyCount(b *InMemoryBackend, region, keyID string) int {
	b.mu.RLock("GrantsByKeyCount")
	defer b.mu.RUnlock()

	if gs, ok := b.grants[region]; ok {
		return len(gs.byKey.Get(keyID))
	}

	return 0
}

// GrantIndexesConsistent checks that the byToken and byKey secondary indexes on
// each region's grants table are consistent with the canonical grants table.
// store.Table/store.Index maintain both automatically on every Put/Delete, so
// this is a structural sanity check rather than the manual three-map
// consistency check it replaces. Returns false and a description if not.
func GrantIndexesConsistent(b *InMemoryBackend) (bool, string) {
	b.mu.RLock("GrantIndexesConsistent")
	defer b.mu.RUnlock()

	for region, gs := range b.grants {
		for _, g := range gs.table.All() {
			tokenMatches := gs.byToken.Get(g.GrantToken)
			if !slices.ContainsFunc(tokenMatches, func(m *Grant) bool { return m.GrantID == g.GrantID }) {
				return false, fmt.Sprintf(
					"grant %s token %s missing from byToken index[%s]",
					g.GrantID,
					g.GrantToken,
					region,
				)
			}

			keyMatches := gs.byKey.Get(g.KeyID)
			if !slices.ContainsFunc(keyMatches, func(m *Grant) bool { return m.GrantID == g.GrantID }) {
				return false, fmt.Sprintf("grant %s missing from byKey index[%s][%s]", g.GrantID, region, g.KeyID)
			}
		}
	}

	return true, ""
}

// ResolutionCacheLen returns the number of entries in the alias/ARN resolution cache.
func ResolutionCacheLen(b *InMemoryBackend) int {
	n := 0
	b.keyIDResolutionCache.Range(func(_, _ any) bool {
		n++

		return true
	})

	return n
}

// ResolutionCacheHas reports whether key is present in the resolution cache.
func ResolutionCacheHas(b *InMemoryBackend, key string) bool {
	_, ok := b.keyIDResolutionCache.Load(key)

	return ok
}

// LastUsageExists reports whether a lastUsage entry exists for region:keyID.
func LastUsageExists(b *InMemoryBackend, region, keyID string) bool {
	_, ok := b.lastUsage.Load(region + ":" + keyID)

	return ok
}

// ImportWrappingKeyExists reports whether an importWrappingKeys entry exists for keyID.
func ImportWrappingKeyExists(b *InMemoryBackend, keyID string) bool {
	_, ok := b.importWrappingKeys.Load(keyID)

	return ok
}

// SetKeyCreationDateForTest backdates a key's CreationDate so that auto-rotation
// tests can simulate an elapsed rotation period without sleeping.
func (b *InMemoryBackend) SetKeyCreationDateForTest(keyID string, t time.Time) {
	b.mu.Lock("SetKeyCreationDateForTest")
	defer b.mu.Unlock()

	for _, tbl := range b.keys {
		if key, ok := tbl.Get(keyID); ok {
			key.CreationDate = UnixTimeFloat(t)

			return
		}
	}
}

// ErrForceRotateKeyNotFound is returned by ForceRotateForTest when keyID is absent.
var ErrForceRotateKeyNotFound = errors.New("key not found")

// ForceRotateForTest rotates the key material for keyID, bypassing the
// on-demand rate limit. Used in tests to drive the history cap.
func (b *InMemoryBackend) ForceRotateForTest(keyID string) error {
	b.mu.Lock("ForceRotateForTest")
	defer b.mu.Unlock()

	for region, tbl := range b.keys {
		if key, ok := tbl.Get(keyID); ok {
			return b.rotateKeyMaterialLocked(region, key, rotationTypeAWSKMS)
		}
	}

	return fmt.Errorf("%w: %s", ErrForceRotateKeyNotFound, keyID)
}
