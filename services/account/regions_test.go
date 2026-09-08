package account_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/account"
)

// TestBackend_ListRegions_InvalidNextToken verifies an undecodable pagination
// cursor is reported as a ValidationException-prefixed error (mapped to
// ValidationException/400 by writeBackendError in handler.go), matching
// real ListRegions' modeled error set (no ResourceNotFoundException).
func TestBackend_ListRegions_InvalidNextToken(t *testing.T) {
	t.Parallel()

	b := account.NewInMemoryBackend("000000000000", "us-east-1")

	_, _, err := b.ListRegions(nil, 0, "not-valid-base64!!")
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "ValidationException"), "got: %v", err)
}

// TestBackend_ListRegions_EmptyResult verifies a filter matching no region
// returns an empty (non-nil) slice and no pagination token, rather than an
// error.
func TestBackend_ListRegions_EmptyResult(t *testing.T) {
	t.Parallel()

	b := account.NewInMemoryBackend("000000000000", "us-east-1")

	regions, next, err := b.ListRegions([]account.RegionOptStatus{account.RegionOptStatusDisabled}, 0, "")
	require.NoError(t, err)
	assert.Empty(t, regions)
	assert.Empty(t, next)
}

// TestBackend_GetRegionOptStatus_UnknownRegion verifies the returned error is
// ValidationException-prefixed (not ResourceNotFoundException): real AWS's
// GetRegionOptStatus modeled error set (verified against
// aws-sdk-go-v2/service/account's deserializers.go and the public API
// reference) has no ResourceNotFoundException.
func TestBackend_GetRegionOptStatus_UnknownRegion(t *testing.T) {
	t.Parallel()

	b := account.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.GetRegionOptStatus("zz-nonexistent-1")
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "ValidationException"), "got: %v", err)
}

// TestBackend_EnableRegion_UnknownRegion and
// TestBackend_DisableRegion_UnknownRegion mirror the GetRegionOptStatus case:
// EnableRegion/DisableRegion's modeled error sets also lack
// ResourceNotFoundException.
func TestBackend_EnableRegion_UnknownRegion(t *testing.T) {
	t.Parallel()

	b := account.NewInMemoryBackend("000000000000", "us-east-1")

	err := b.EnableRegion("zz-nonexistent-1")
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "ValidationException"), "got: %v", err)
}

func TestBackend_DisableRegion_UnknownRegion(t *testing.T) {
	t.Parallel()

	b := account.NewInMemoryBackend("000000000000", "us-east-1")

	err := b.DisableRegion("zz-nonexistent-1")
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "ValidationException"), "got: %v", err)
}

// TestBackend_EnableRegion_EnabledByDefaultRejected and
// TestBackend_DisableRegion_EnabledByDefaultRejected verify an
// ENABLED_BY_DEFAULT region cannot be opted in/out, per AWS's
// invalidRegionOptTarget ValidationException reason.
func TestBackend_EnableRegion_EnabledByDefaultRejected(t *testing.T) {
	t.Parallel()

	b := account.NewInMemoryBackend("000000000000", "us-east-1")

	err := b.EnableRegion("us-east-1")
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "ValidationException"), "got: %v", err)
}

func TestBackend_DisableRegion_EnabledByDefaultRejected(t *testing.T) {
	t.Parallel()

	b := account.NewInMemoryBackend("000000000000", "us-east-1")

	err := b.DisableRegion("us-east-1")
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "ValidationException"), "got: %v", err)
}

// TestBackend_ListRegions_Deterministic verifies ListRegions always returns
// regions sorted alphabetically by RegionName, independent of filter/paging,
// matching AWS's documented alphabetical ordering.
func TestBackend_ListRegions_Deterministic(t *testing.T) {
	t.Parallel()

	b := account.NewInMemoryBackend("000000000000", "us-east-1")

	regions, _, err := b.ListRegions(nil, 0, "")
	require.NoError(t, err)
	require.NotEmpty(t, regions)

	for i := 1; i < len(regions); i++ {
		assert.Less(t, regions[i-1].RegionName, regions[i].RegionName)
	}
}

// TestBackend_GetRegionOptStatus_Classification verifies every seeded region's
// opt-in classification against real AWS: ap-southeast-1 (Singapore, launched
// 2010) and ap-northeast-1 (Tokyo, launched 2011) predate the 2019 opt-in
// region policy and are ENABLED_BY_DEFAULT like the other original regions,
// not opt-in ENABLED (gopherstack-5py7).
func TestBackend_GetRegionOptStatus_Classification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		want   account.RegionOptStatus
		region string
	}{
		{name: "us-east-1", region: "us-east-1", want: account.RegionOptStatusEnabledDefault},
		{name: "us-east-2", region: "us-east-2", want: account.RegionOptStatusEnabledDefault},
		{name: "us-west-1", region: "us-west-1", want: account.RegionOptStatusEnabledDefault},
		{name: "us-west-2", region: "us-west-2", want: account.RegionOptStatusEnabledDefault},
		{name: "eu-west-1", region: "eu-west-1", want: account.RegionOptStatusEnabledDefault},
		{name: "eu-central-1", region: "eu-central-1", want: account.RegionOptStatusEnabledDefault},
		{name: "ap-southeast-1", region: "ap-southeast-1", want: account.RegionOptStatusEnabledDefault},
		{name: "ap-northeast-1", region: "ap-northeast-1", want: account.RegionOptStatusEnabledDefault},
		{name: "af-south-1", region: "af-south-1", want: account.RegionOptStatusEnabled},
		{name: "ap-east-1", region: "ap-east-1", want: account.RegionOptStatusEnabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := account.NewInMemoryBackend("000000000000", "us-east-1")

			got, err := b.GetRegionOptStatus(tt.region)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBackend_DisableRegion_RejectsNonOptInRegion verifies DisableRegion
// refuses to disable regions that are enabled by default (not opt-in), per
// AWS's invalidRegionOptTarget ValidationException reason. Singapore and
// Tokyo are not opt-in regions, so real AWS rejects disabling them the same
// way it rejects disabling us-east-1 (gopherstack-5py7).
func TestBackend_DisableRegion_RejectsNonOptInRegion(t *testing.T) {
	t.Parallel()

	for _, region := range []string{"ap-southeast-1", "ap-northeast-1"} {
		t.Run(region, func(t *testing.T) {
			t.Parallel()

			b := account.NewInMemoryBackend("000000000000", "us-east-1")

			err := b.DisableRegion(region)
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "ValidationException"), "got: %v", err)

			status, statusErr := b.GetRegionOptStatus(region)
			require.NoError(t, statusErr)
			assert.Equal(t, account.RegionOptStatusEnabledDefault, status)
		})
	}
}
