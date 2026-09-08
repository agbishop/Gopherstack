package cognitoidentity_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cognitoidentity"
)

func TestInMemoryBackend_CreateIdentityPool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget   error
		name        string
		poolName    string
		wantErr     bool
		allowUnauth bool
	}{
		{
			name:        "success",
			poolName:    "my-pool",
			allowUnauth: true,
		},
		{
			name:      "empty_name",
			poolName:  "",
			wantErr:   true,
			errTarget: cognitoidentity.ErrInvalidParameter,
		},
		{
			name:      "duplicate_name",
			poolName:  "my-pool",
			wantErr:   true,
			errTarget: cognitoidentity.ErrIdentityPoolAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.name == "duplicate_name" {
				_, setupErr := b.CreateIdentityPool(
					context.Background(),
					"my-pool",
					true,
					false,
					"",
					nil,
					nil,
					nil,
				)
				require.NoError(t, setupErr)
			}

			pool, err := b.CreateIdentityPool(
				context.Background(),
				tt.poolName,
				tt.allowUnauth,
				false,
				"",
				nil,
				nil,
				nil,
			)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, pool.IdentityPoolID)
			assert.Equal(t, tt.poolName, pool.IdentityPoolName)
			assert.Contains(t, pool.IdentityPoolID, "us-east-1:")
		})
	}
}

func TestInMemoryBackend_DeleteIdentityPool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget error
		name      string
		poolID    string
		wantErr   bool
	}{
		{
			name:   "success",
			poolID: "pool-to-delete",
		},
		{
			name:      "not_found",
			poolID:    "nonexistent-pool",
			wantErr:   true,
			errTarget: cognitoidentity.ErrIdentityPoolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			var realPoolID string

			if tt.name == "success" {
				pool, setupErr := b.CreateIdentityPool(
					context.Background(),
					"delete-pool",
					true,
					false,
					"",
					nil,
					nil,
					nil,
				)
				require.NoError(t, setupErr)
				realPoolID = pool.IdentityPoolID
			} else {
				realPoolID = tt.poolID
			}

			err := b.DeleteIdentityPool(context.Background(), realPoolID)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)

			_, descErr := b.DescribeIdentityPool(context.Background(), realPoolID)
			require.Error(t, descErr)
		})
	}
}

func TestInMemoryBackend_DescribeIdentityPool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget error
		name      string
		poolID    string
		wantErr   bool
	}{
		{
			name:   "success",
			poolID: "real",
		},
		{
			name:      "not_found",
			poolID:    "nonexistent",
			wantErr:   true,
			errTarget: cognitoidentity.ErrIdentityPoolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			var poolID string

			if tt.name == "success" {
				pool, setupErr := b.CreateIdentityPool(
					context.Background(),
					"describe-pool",
					true,
					false,
					"",
					nil,
					nil,
					nil,
				)
				require.NoError(t, setupErr)
				poolID = pool.IdentityPoolID
			} else {
				poolID = tt.poolID
			}

			pool, err := b.DescribeIdentityPool(context.Background(), poolID)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, poolID, pool.IdentityPoolID)
			assert.Equal(t, "describe-pool", pool.IdentityPoolName)
		})
	}
}

func TestInMemoryBackend_ListIdentityPools(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	_, err1 := b.CreateIdentityPool(context.Background(), "pool-a", true, false, "", nil, nil, nil)
	require.NoError(t, err1)

	_, err2 := b.CreateIdentityPool(context.Background(), "pool-b", false, false, "", nil, nil, nil)
	require.NoError(t, err2)

	pools, _ := b.ListIdentityPools(context.Background(), 0, "")
	assert.Len(t, pools, 2)

	limited, _ := b.ListIdentityPools(context.Background(), 1, "")
	assert.Len(t, limited, 1)
}

func TestInMemoryBackend_UpdateIdentityPool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget error
		name      string
		wantErr   bool
	}{
		{name: "success"},
		{
			name:      "not_found",
			wantErr:   true,
			errTarget: cognitoidentity.ErrIdentityPoolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			var poolID string

			if tt.name == "success" {
				pool, setupErr := b.CreateIdentityPool(
					context.Background(),
					"update-pool",
					true,
					false,
					"",
					nil,
					nil,
					nil,
				)
				require.NoError(t, setupErr)
				poolID = pool.IdentityPoolID
			} else {
				poolID = "nonexistent"
			}

			updated, err := b.UpdateIdentityPool(
				context.Background(),
				poolID,
				"update-pool",
				false,
				true,
				"",
				nil,
				nil,
				nil,
			)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)
			assert.False(t, updated.AllowUnauthenticatedIdentities)
			assert.True(t, updated.AllowClassicFlow)
		})
	}
}

func TestInMemoryBackend_UpdateIdentityPool_RenameConflict(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	pool1, err := b.CreateIdentityPool(
		context.Background(),
		"pool-one",
		true,
		false,
		"",
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)

	_, err = b.CreateIdentityPool(context.Background(), "pool-two", true, false, "", nil, nil, nil)
	require.NoError(t, err)

	// Attempt to rename pool-one to pool-two (conflict).
	_, err = b.UpdateIdentityPool(
		context.Background(),
		pool1.IdentityPoolID,
		"pool-two",
		true,
		false,
		"",
		nil,
		nil,
		nil,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, cognitoidentity.ErrIdentityPoolAlreadyExists)
}

func TestInMemoryBackend_DeleteIdentityPool_CleansIdentities(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	pool, err := b.CreateIdentityPool(
		context.Background(),
		"clean-pool",
		true,
		false,
		"",
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)

	// Create an identity inside the pool.
	identity, err := b.GetID(context.Background(), pool.IdentityPoolID, "000000000000", nil)
	require.NoError(t, err)
	require.NotEmpty(t, identity.IdentityID)

	// Delete the pool.
	require.NoError(t, b.DeleteIdentityPool(context.Background(), pool.IdentityPoolID))

	// Pool should be gone.
	_, err = b.DescribeIdentityPool(context.Background(), pool.IdentityPoolID)
	require.ErrorIs(t, err, cognitoidentity.ErrIdentityPoolNotFound)

	// Identity from the deleted pool should no longer be usable.
	_, err = b.GetCredentialsForIdentity(context.Background(), identity.IdentityID, nil)
	require.ErrorIs(t, err, cognitoidentity.ErrIdentityPoolNotFound)
}

func TestInMemoryBackend_CreateIdentityPool_WithProviders(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	providers := []cognitoidentity.IdentityProvider{
		{
			ProviderName:         "cognito-idp.us-east-1.amazonaws.com/us-east-1_xxx",
			ClientID:             "client123",
			ServerSideTokenCheck: true,
		},
	}

	pool, err := b.CreateIdentityPool(
		context.Background(),
		"provider-pool",
		true,
		false,
		"",
		providers,
		map[string]string{
			"graph.facebook.com": "123456789",
		},
		map[string]string{"env": "test"},
	)
	require.NoError(t, err)
	assert.Len(t, pool.IdentityProviders, 1)
	assert.Equal(t, "client123", pool.IdentityProviders[0].ClientID)
	assert.Equal(t, "123456789", pool.SupportedLoginProviders["graph.facebook.com"])
	assert.Equal(t, "test", pool.Tags["env"])
}

func TestInMemoryBackend_DeveloperProviderName_RoundTrip(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateIdentityPool(
		context.Background(),
		"dev-pool",
		true,
		false,
		"developer.myapp.com",
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "developer.myapp.com", pool.DeveloperProviderName)

	described, err := b.DescribeIdentityPool(context.Background(), pool.IdentityPoolID)
	require.NoError(t, err)
	assert.Equal(t, "developer.myapp.com", described.DeveloperProviderName)
}

// TestInMemoryBackend_UpdateIdentityPool_DeveloperProviderName proves
// api_op_CreateIdentityPool.go's "Once you have set a developer provider name, you
// cannot change it" invariant, and that a pool created without one can still adopt
// one through Update.
func TestInMemoryBackend_UpdateIdentityPool_DeveloperProviderName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		createDPN string
		updateDPN string
		want      string
	}{
		{
			name:      "already set cannot change",
			createDPN: "developer.myapp.com",
			updateDPN: "developer.other.com",
			want:      "developer.myapp.com",
		},
		{
			name:      "unset can be adopted",
			createDPN: "",
			updateDPN: "developer.adopted.com",
			want:      "developer.adopted.com",
		},
		{
			name:      "already set survives an empty update",
			createDPN: "developer.myapp.com",
			updateDPN: "",
			want:      "developer.myapp.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			pool, err := b.CreateIdentityPool(
				context.Background(), "dev-pool", true, false, tt.createDPN, nil, nil, nil,
			)
			require.NoError(t, err)

			updated, err := b.UpdateIdentityPool(
				context.Background(), pool.IdentityPoolID, "dev-pool", true, false,
				tt.updateDPN, nil, nil, nil,
			)
			require.NoError(t, err)
			assert.Equal(t, tt.want, updated.DeveloperProviderName)

			described, err := b.DescribeIdentityPool(context.Background(), pool.IdentityPoolID)
			require.NoError(t, err)
			assert.Equal(t, tt.want, described.DeveloperProviderName)
		})
	}
}

func TestInMemoryBackend_UpdateIdentityPool_WithTags(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateIdentityPool(
		context.Background(),
		"tag-update-pool",
		true,
		false,
		"",
		nil,
		nil,
		map[string]string{"env": "dev"},
	)
	require.NoError(t, err)
	assert.Equal(t, "dev", pool.Tags["env"])

	updated, err := b.UpdateIdentityPool(context.Background(),
		pool.IdentityPoolID,
		"tag-update-pool",
		true,
		false,
		"",
		nil,
		nil,
		map[string]string{"env": "prod"},
	)
	require.NoError(t, err)
	assert.Equal(t, "prod", updated.Tags["env"])
}

func TestInMemoryBackend_DescribeIdentityPool_EmptyID(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	_, err := b.DescribeIdentityPool(context.Background(), "")
	require.Error(t, err)
	assert.ErrorIs(t, err, cognitoidentity.ErrInvalidParameter)
}

func TestInMemoryBackend_DeleteIdentityPool_EmptyID(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	err := b.DeleteIdentityPool(context.Background(), "")
	require.Error(t, err)
	assert.ErrorIs(t, err, cognitoidentity.ErrInvalidParameter)
}

func TestInMemoryBackend_UpdateIdentityPool_EmptyID(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	_, err := b.UpdateIdentityPool(context.Background(), "", "name", true, false, "", nil, nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, cognitoidentity.ErrInvalidParameter)
}

func TestInMemoryBackend_ListIdentityPools_NextToken(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	for _, name := range []string{"pool-a", "pool-b", "pool-c"} {
		_, err := b.CreateIdentityPool(context.Background(), name, true, false, "", nil, nil, nil)
		require.NoError(t, err)
	}

	// First page of 2.
	page1, token := b.ListIdentityPools(context.Background(), 2, "")
	require.Len(t, page1, 2)
	assert.NotEmpty(t, token, "nextToken must be returned when there are more pages")

	// Second page.
	page2, token2 := b.ListIdentityPools(context.Background(), 2, token)
	require.Len(t, page2, 1)
	assert.Empty(t, token2, "no further pages expected")

	// All names combined should cover all pools.
	all := append(page1, page2...) //nolint:gocritic // intentional
	assert.Len(t, all, 3)
}
