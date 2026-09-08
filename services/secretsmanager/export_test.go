package secretsmanager

import (
	"context"
	"time"
)

// CtxWithRegion returns ctx carrying region as the per-request AWS region,
// for backend-level tests that need to address a specific region (e.g. a
// replica secret's own region) without going through the HTTP handler.
func CtxWithRegion(ctx context.Context, region string) context.Context {
	return context.WithValue(ctx, regionContextKey{}, region)
}

// SecretCount returns the total number of secrets in the backend across all regions.
func SecretCount(b *InMemoryBackend) int {
	b.mu.RLock("SecretCount")
	defer b.mu.RUnlock()

	return b.secrets.Len()
}

// ResourcePolicyCount returns the total number of resource policies across all regions.
func ResourcePolicyCount(b *InMemoryBackend) int {
	b.mu.RLock("ResourcePolicyCount")
	defer b.mu.RUnlock()

	total := 0
	for _, regionPolicies := range b.resourcePolicies {
		total += len(regionPolicies)
	}

	return total
}

// ReplicationConfigCount returns the total number of replication configs across all regions.
func ReplicationConfigCount(b *InMemoryBackend) int {
	b.mu.RLock("ReplicationConfigCount")
	defer b.mu.RUnlock()

	total := 0
	for _, regionConfigs := range b.replicationConfigs {
		total += len(regionConfigs)
	}

	return total
}

// HandlerOpsLen returns the number of operations registered in the handler dispatch table.
func HandlerOpsLen(h *Handler) int {
	return len(h.ops)
}

// NextCronTime exposes nextCronTime for white-box testing.
func NextCronTime(expr string, from time.Time) (time.Time, bool) {
	return nextCronTime(expr, from)
}

// RotationDue exposes rotationDue for white-box testing.
func RotationDue(rules *RotationRulesType, now time.Time, base *float64) bool {
	return rotationDue(rules, now, base)
}

// RunScheduledRotationsForTest exposes runScheduledRotations for deterministic testing.
// Pass a time far enough in the future to ensure all due rotations fire.
func (b *InMemoryBackend) RunScheduledRotationsForTest(now time.Time) {
	b.runScheduledRotations(now)
}

// SetNowForTest overrides the clock function used by the backend for deterministic time control.
// Restore with b.SetNowForTest(time.Now) after the test.
func (b *InMemoryBackend) SetNowForTest(fn func() time.Time) {
	b.now = fn
}

// SecretVersionRaw returns the raw stored SecretVersion for the given secret
// (by name) and version ID -- or the AWSCURRENT version when versionID is
// empty -- for white-box testing of KMS-at-rest storage (e.g. asserting
// Ciphertext is populated and SecretString/SecretBinary are cleared once a
// KMSEncryptor is wired). The second return is false when the secret or
// version does not exist.
func SecretVersionRaw(b *InMemoryBackend, region, name, versionID string) (*SecretVersion, bool) {
	b.mu.RLock("SecretVersionRaw")
	defer b.mu.RUnlock()

	secret, ok := b.secretGet(region, name)
	if !ok {
		return nil, false
	}

	if versionID == "" {
		versionID = secret.CurrentVersionID
	}

	v, ok := secret.Versions[versionID]

	return v, ok
}
