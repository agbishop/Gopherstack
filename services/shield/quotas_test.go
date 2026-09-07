package shield_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/shield"
)

// TestInMemoryBackend_CreateProtectionPerTypeQuota verifies CreateProtection enforces the
// subscriptionMaxProtectionsPerType=100 quota it reports via DescribeSubscription
// (SubscriptionLimits.ProtectionLimits.ProtectedResourceTypeLimits), returning
// LimitsExceededException semantics once exceeded for a single resource type.
func TestInMemoryBackend_CreateProtectionPerTypeQuota(t *testing.T) {
	t.Parallel()

	const maxPerType = 100

	b := shield.NewInMemoryBackend(testAccountID, testRegion)
	require.NoError(t, b.CreateSubscription())

	for i := range maxPerType {
		arn := fmt.Sprintf("arn:aws:ec2:us-east-1:000000000000:eip-allocation/eipalloc-%d", i)
		_, err := b.CreateProtection(fmt.Sprintf("eip-%d", i), arn, nil)
		require.NoError(t, err)
	}

	_, err := b.CreateProtection(
		"eip-over", "arn:aws:ec2:us-east-1:000000000000:eip-allocation/eipalloc-over", nil,
	)
	require.ErrorIs(t, err, shield.ErrLimitExceeded)
	assert.Equal(t, maxPerType, shield.ProtectionCount(b))
}

// TestInMemoryBackend_CreateProtectionTotalQuota verifies CreateProtection enforces the overall
// subscriptionMaxProtections=1000 quota. Uses resource ARNs that don't match any of the 6 known
// Shield-protectable resource types so the per-type quota (which would otherwise cap at 600
// across all 6 types) never interferes with reaching the 1000 total.
func TestInMemoryBackend_CreateProtectionTotalQuota(t *testing.T) {
	t.Parallel()

	const maxProtections = 1000

	b := shield.NewInMemoryBackend(testAccountID, testRegion)
	require.NoError(t, b.CreateSubscription())

	for i := range maxProtections {
		arn := fmt.Sprintf("arn:aws:example:us-east-1:000000000000:thing/%d", i)
		_, err := b.CreateProtection(fmt.Sprintf("thing-%d", i), arn, nil)
		require.NoError(t, err)
	}

	_, err := b.CreateProtection("one-too-many", "arn:aws:example:us-east-1:000000000000:thing/over", nil)
	require.ErrorIs(t, err, shield.ErrLimitExceeded)
	assert.Equal(t, maxProtections, shield.ProtectionCount(b))
}

// TestInMemoryBackend_CreateProtectionGroupQuota verifies CreateProtectionGroup enforces the
// subscriptionMaxProtectionGroups=100 quota reported via
// SubscriptionLimits.ProtectionGroupLimits.MaxProtectionGroups.
func TestInMemoryBackend_CreateProtectionGroupQuota(t *testing.T) {
	t.Parallel()

	const maxGroups = 100

	b := shield.NewInMemoryBackend(testAccountID, testRegion)
	require.NoError(t, b.CreateSubscription())

	for i := range maxGroups {
		_, err := b.CreateProtectionGroup(fmt.Sprintf("grp-%d", i), shield.AggregationSum, shield.PatternAll, "", nil)
		require.NoError(t, err)
	}

	_, err := b.CreateProtectionGroup("grp-over", shield.AggregationSum, shield.PatternAll, "", nil)
	require.ErrorIs(t, err, shield.ErrLimitExceeded)
	assert.Equal(t, maxGroups, shield.ProtectionGroupCount(b))
}

// TestInMemoryBackend_CreateProtectionGroupArbitraryMembersQuota verifies CreateProtectionGroup
// enforces the subscriptionMaxMembersPerGroup=10000 ARBITRARY-pattern quota reported via
// SubscriptionLimits.ProtectionGroupLimits.PatternTypeLimits.ArbitraryPatternLimits.MaxMembers.
func TestInMemoryBackend_CreateProtectionGroupArbitraryMembersQuota(t *testing.T) {
	t.Parallel()

	const maxMembers = 10000

	b := shield.NewInMemoryBackend(testAccountID, testRegion)
	require.NoError(t, b.CreateSubscription())

	members := make([]string, maxMembers+1)
	for i := range members {
		members[i] = fmt.Sprintf("arn:aws:ec2:us-east-1:000000000000:eip-allocation/eipalloc-%d", i)
	}

	_, err := b.CreateProtectionGroup("grp-1", shield.AggregationSum, shield.PatternArbitrary, "", members)
	require.ErrorIs(t, err, shield.ErrLimitExceeded)
}

// TestInMemoryBackend_UpdateProtectionGroupArbitraryMembersQuota verifies UpdateProtectionGroup
// enforces the same ARBITRARY-pattern member cap as CreateProtectionGroup, but -- unlike
// CreateProtectionGroup -- reports ErrValidation, not ErrLimitExceeded: UpdateProtectionGroup's
// real error catalog has no LimitsExceededException (gopherstack-g2l5, deserializers.go's
// deserializeOpErrorUpdateProtectionGroup declares only InternalErrorException/
// InvalidParameterException/OptimisticLockException/ResourceNotFoundException/UnknownError).
// Also verifies the group's members are left unmutated by the rejected update.
func TestInMemoryBackend_UpdateProtectionGroupArbitraryMembersQuota(t *testing.T) {
	t.Parallel()

	const maxMembers = 10000

	b := shield.NewInMemoryBackend(testAccountID, testRegion)
	require.NoError(t, b.CreateSubscription())

	original := []string{"arn:aws:ec2:us-east-1:000000000000:eip-allocation/eipalloc-1"}

	_, err := b.CreateProtectionGroup("grp-1", shield.AggregationSum, shield.PatternArbitrary, "", original)
	require.NoError(t, err)

	members := make([]string, maxMembers+1)
	for i := range members {
		members[i] = fmt.Sprintf("arn:aws:ec2:us-east-1:000000000000:eip-allocation/eipalloc-%d", i)
	}

	err = b.UpdateProtectionGroup("grp-1", shield.AggregationSum, shield.PatternArbitrary, "", members)
	require.ErrorIs(t, err, shield.ErrValidation)
	require.NotErrorIs(t, err, shield.ErrLimitExceeded)

	pg, err := b.DescribeProtectionGroup("grp-1")
	require.NoError(t, err)
	assert.Equal(t, original, pg.Members)
}
