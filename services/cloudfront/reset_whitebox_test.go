package cloudfront

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReset_InvalidationReadyAt verifies that Reset() clears the reconciler's
// pending-invalidation readiness maps, not just the invalidation resources
// themselves.
func TestReset_InvalidationReadyAt(t *testing.T) {
	t.Parallel()

	b := NewInMemoryBackend(context.Background(), "123456789012", "us-east-1")

	dist, err := b.CreateDistribution("cr-1", "test dist", true, nil)
	require.NoError(t, err)

	_, err = b.CreateInvalidation(dist.ID, "inv-cr-1", []string{"/*"})
	require.NoError(t, err)

	b.mu.RLock("test")
	pending := len(b.invalidationReadyAt[dist.ID])
	b.mu.RUnlock()
	require.Equal(t, 1, pending, "sanity: invalidation should be pending reconciliation")

	b.Reset()

	b.mu.RLock("test")
	defer b.mu.RUnlock()

	assert.Empty(t, b.invalidationReadyAt, "invalidationReadyAt must be cleared by Reset")
	assert.Empty(t, b.tenantInvalidationReadyAt, "tenantInvalidationReadyAt must be cleared by Reset")
}
