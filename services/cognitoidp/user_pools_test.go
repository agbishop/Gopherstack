package cognitoidp_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cognitoidp"
)

func TestCreateUserPool_PasswordPolicy_Persisted(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPoolWithOpts("pp-pool", cognitoidp.UserPoolOptions{
		PasswordPolicy: &cognitoidp.PasswordPolicy{
			MinimumLength:    12,
			RequireUppercase: true,
			RequireSymbols:   true,
		},
	})
	require.NoError(t, err)

	got, err := b.DescribeUserPool(pool.ID)
	require.NoError(t, err)
	require.NotNil(t, got.PasswordPolicy)
	assert.Equal(t, 12, got.PasswordPolicy.MinimumLength)
	assert.True(t, got.PasswordPolicy.RequireUppercase)
	assert.True(t, got.PasswordPolicy.RequireSymbols)
}

// TestDeleteUserPool_ClearsUserDeviceState verifies DeleteUserPool's user
// cascade clears devices/authEvents for each user, not just the user record
// itself. The cascade deletes users directly (b.users.Delete) instead of
// calling AdminDeleteUser, so it does not inherit AdminDeleteUser's own
// devices/authEvents cleanup -- the cascade-variant of the ghost-row bug
// class, where a parent delete bypasses the single-resource delete path that
// holds the fix.
func TestDeleteUserPool_ClearsUserDeviceState(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("del-pool-device-state")
	require.NoError(t, err)

	_, err = b.AdminCreateUser(pool.ID, "some-user", "Pass1234!", nil)
	require.NoError(t, err)

	b.SeedDeviceForTest(pool.ID, "some-user", &cognitoidp.Device{DeviceKey: "dev1", Status: "valid"})
	b.SeedAuthEventForTest(pool.ID, "some-user", &cognitoidp.AuthEvent{EventID: "ev1", EventType: "SignIn"})
	require.True(t, b.HasDeviceStateForTest(pool.ID, "some-user"))

	otherPool, err := b.CreateUserPool("del-pool-device-state-sibling")
	require.NoError(t, err)
	_, err = b.AdminCreateUser(otherPool.ID, "some-user", "Pass1234!", nil)
	require.NoError(t, err)
	b.SeedDeviceForTest(otherPool.ID, "some-user", &cognitoidp.Device{DeviceKey: "dev1", Status: "valid"})

	require.NoError(t, b.DeleteUserPool(pool.ID))

	assert.False(t, b.HasDeviceStateForTest(pool.ID, "some-user"))
	assert.True(t, b.HasDeviceStateForTest(otherPool.ID, "some-user"),
		"deleting one pool must not disturb another pool's device state")
}

// TestDeleteUserPool_RefusesWhenDomainAttached covers gopherstack-tq5q:
// deleting a pool that still owns a domain must be refused, matching real
// AWS Cognito (InvalidParameterException: "User pool cannot be deleted...
// domain configured that should be deleted first" -- confirmed via the AWS
// API's documented error, github.com/hashicorp/terraform-provider-aws#16479)
// rather than silently orphaning the domain the way this backend used to.
func TestDeleteUserPool_RefusesWhenDomainAttached(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("domain-lockout-pool")
	require.NoError(t, err)

	_, err = b.CreateUserPoolDomain(pool.ID, "lockout-domain")
	require.NoError(t, err)

	deleteErr := b.DeleteUserPool(pool.ID)
	require.ErrorIs(t, deleteErr, cognitoidp.ErrInvalidParameter)
	assert.Equal(t, 1, b.UserPoolCount(), "pool must survive a refused delete")
	assert.NotNil(t, b.FindUserPoolDomain("lockout-domain"), "domain must survive a refused delete")

	// AWS's documented remediation: delete the domain first (still possible
	// through the normal path since the pool is still alive), then the pool.
	require.NoError(t, b.DeleteUserPoolDomain(pool.ID, "lockout-domain"))
	require.NoError(t, b.DeleteUserPool(pool.ID))

	// Recovery: the domain name is immediately usable again by a new pool.
	newPool, err := b.CreateUserPool("post-lockout-pool")
	require.NoError(t, err)
	_, err = b.CreateUserPoolDomain(newPool.ID, "lockout-domain")
	require.NoError(t, err)
}

// TestDeleteUserPool_DomainRefusal_DoesNotDisturbSiblingDomain is the
// negative case: a refused delete on one pool must not touch another pool's
// domain.
func TestDeleteUserPool_DomainRefusal_DoesNotDisturbSiblingDomain(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	poolA, err := b.CreateUserPool("domain-sibling-a")
	require.NoError(t, err)
	poolB, err := b.CreateUserPool("domain-sibling-b")
	require.NoError(t, err)

	_, err = b.CreateUserPoolDomain(poolA.ID, "domain-a")
	require.NoError(t, err)
	_, err = b.CreateUserPoolDomain(poolB.ID, "domain-b")
	require.NoError(t, err)

	require.ErrorIs(t, b.DeleteUserPool(poolA.ID), cognitoidp.ErrInvalidParameter)
	assert.NotNil(t, b.FindUserPoolDomain("domain-b"))

	require.NoError(t, b.DeleteUserPoolDomain(poolA.ID, "domain-a"))
	require.NoError(t, b.DeleteUserPool(poolA.ID))

	assert.NotNil(t, b.FindUserPoolDomain("domain-b"), "deleting one pool must not disturb another pool's domain")
}

// TestDeleteUserPool_ClearsResourceTags verifies DeleteUserPool clears the
// pool's own resourceTags entry. ListTagsForResource does a bare map lookup
// on ARN with no pool-existence check, and TaggedResources feeds the
// cross-service Resource Groups Tagging API (cli.go's wireTaggingCognitoIDP),
// so a stale entry keeps a deleted pool's tags visible through both paths.
func TestDeleteUserPool_ClearsResourceTags(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	pool, err := b.CreateUserPool("del-pool-tags")
	require.NoError(t, err)
	b.TagResource(pool.ARN, map[string]string{"env": "prod"})

	otherPool, err := b.CreateUserPool("del-pool-tags-sibling")
	require.NoError(t, err)
	b.TagResource(otherPool.ARN, map[string]string{"env": "staging"})

	require.NoError(t, b.DeleteUserPool(pool.ID))

	assert.Empty(t, b.ListTagsForResource(pool.ARN))
	assert.Equal(t, map[string]string{"env": "staging"}, b.ListTagsForResource(otherPool.ARN),
		"deleting one pool must not disturb another pool's tags")
}

func TestHandler_CreateUserPool_WithPasswordPolicy(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{
		"PoolName": "policy-pool",
		"Policies": map[string]any{
			"PasswordPolicy": map[string]any{
				"MinimumLength":    10,
				"RequireUppercase": true,
				"RequireNumbers":   true,
			},
		},
		"AutoVerifiedAttributes": []string{"email"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		UserPool struct {
			Policies *struct {
				PasswordPolicy *struct {
					MinimumLength  int  `json:"MinimumLength,omitempty"`
					RequireNumbers bool `json:"RequireNumbers,omitempty"`
				} `json:"PasswordPolicy"`
			} `json:"Policies"`
			AutoVerifiedAttributes []string `json:"AutoVerifiedAttributes,omitempty"`
		} `json:"UserPool"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.UserPool.Policies)
	require.NotNil(t, resp.UserPool.Policies.PasswordPolicy)
	assert.Equal(t, 10, resp.UserPool.Policies.PasswordPolicy.MinimumLength)
	assert.True(t, resp.UserPool.Policies.PasswordPolicy.RequireNumbers)
	assert.Equal(t, []string{"email"}, resp.UserPool.AutoVerifiedAttributes)
}

func TestHandler_DescribeUserPool_IncludesPolicy(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{
		"PoolName": "describe-policy-pool",
		"Policies": map[string]any{
			"PasswordPolicy": map[string]any{"MinimumLength": 12},
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp struct {
		UserPool struct {
			ID string `json:"Id,omitempty"`
		} `json:"UserPool"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))

	rec := doCognitoRequest(t, h, "DescribeUserPool", map[string]any{"UserPoolId": createResp.UserPool.ID})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		UserPool struct {
			Policies *struct {
				PasswordPolicy *struct {
					MinimumLength int `json:"MinimumLength,omitempty"`
				} `json:"PasswordPolicy"`
			} `json:"Policies"`
		} `json:"UserPool"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.UserPool.Policies)
	require.NotNil(t, resp.UserPool.Policies.PasswordPolicy)
	assert.Equal(t, 12, resp.UserPool.Policies.PasswordPolicy.MinimumLength)
}

func TestUserPool_CRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		poolName string
	}{
		{name: "basic_create_describe_delete", poolName: "test-pool-crud"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			// Create.
			rec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{"PoolName": tt.poolName})
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			var createResp struct {
				UserPool struct {
					ID   string `json:"Id"`
					Name string `json:"Name"`
				} `json:"UserPool"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
			poolID := createResp.UserPool.ID
			assert.Equal(t, tt.poolName, createResp.UserPool.Name)
			assert.NotEmpty(t, poolID)

			// Describe.
			descRec := doCognitoRequest(t, h, "DescribeUserPool", map[string]any{"UserPoolId": poolID})
			require.Equal(t, http.StatusOK, descRec.Code)

			var descResp struct {
				UserPool struct {
					ID string `json:"Id"`
				} `json:"UserPool"`
			}
			require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
			assert.Equal(t, poolID, descResp.UserPool.ID)

			// Delete.
			delRec := doCognitoRequest(t, h, "DeleteUserPool", map[string]any{"UserPoolId": poolID})
			require.Equal(t, http.StatusOK, delRec.Code)

			// Describe after delete returns error.
			afterRec := doCognitoRequest(t, h, "DescribeUserPool", map[string]any{"UserPoolId": poolID})
			assert.Equal(t, http.StatusBadRequest, afterRec.Code)
		})
	}
}

func TestInMemoryBackend_CreateUserPool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		poolName string
	}{
		{
			name:     "success",
			poolName: "my-pool",
		},
		{
			name:     "duplicate_name",
			poolName: "my-pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			var firstID string
			if tt.name == "duplicate_name" {
				// AWS Cognito does not enforce unique pool names — CreateUserPool
				// has no "already exists" exception in its own SDK model
				// (aws-sdk-go-v2/service/cognitoidentityprovider@v1.67.4). A
				// second pool with the same name must succeed with a distinct ID.
				first, setupErr := b.CreateUserPool("my-pool")
				require.NoError(t, setupErr)
				firstID = first.ID
			}

			pool, createErr := b.CreateUserPool(tt.poolName)
			require.NoError(t, createErr)
			assert.NotEmpty(t, pool.ID)
			assert.Equal(t, tt.poolName, pool.Name)
			assert.NotEmpty(t, pool.ARN)

			if tt.name == "duplicate_name" {
				assert.NotEqual(t, firstID, pool.ID)
			}
		})
	}
}

func TestInMemoryBackend_DescribeUserPool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget  error
		name       string
		userPoolID string
		wantErr    bool
	}{
		{
			name:    "success",
			wantErr: false,
		},
		{
			name:       "not_found",
			userPoolID: "us-east-1_nonexistent",
			wantErr:    true,
			errTarget:  cognitoidp.ErrUserPoolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			pool, setupErr := b.CreateUserPool("test-pool")
			require.NoError(t, setupErr)

			poolID := pool.ID
			if tt.userPoolID != "" {
				poolID = tt.userPoolID
			}

			got, err := b.DescribeUserPool(poolID)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, poolID, got.ID)
		})
	}
}

func TestInMemoryBackend_ListUserPools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		numPools int
	}{
		{
			name:     "empty",
			numPools: 0,
		},
		{
			name:     "multiple_pools",
			numPools: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			for i := range tt.numPools {
				_, err := b.CreateUserPool("pool-" + strconv.Itoa(i))
				require.NoError(t, err)
			}

			pools := b.ListUserPools()
			assert.Len(t, pools, tt.numPools)
		})
	}
}

func TestInMemoryBackend_GetUserPoolJWKS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget  error
		userPoolID func(b *cognitoidp.InMemoryBackend) string
		name       string
		wantErr    bool
	}{
		{
			name: "success",
			userPoolID: func(b *cognitoidp.InMemoryBackend) string {
				pool, _ := b.CreateUserPool("p")

				return pool.ID
			},
		},
		{
			name: "pool_not_found",
			userPoolID: func(_ *cognitoidp.InMemoryBackend) string {
				return "us-east-1_nonexistent"
			},
			wantErr:   true,
			errTarget: cognitoidp.ErrUserPoolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			poolID := tt.userPoolID(b)

			jwks, err := b.GetUserPoolJWKS(poolID)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)
			require.Len(t, jwks.Keys, 1)
			assert.Equal(t, "RSA", jwks.Keys[0].Kty)
			assert.Equal(t, "RS256", jwks.Keys[0].Alg)
			assert.Equal(t, "sig", jwks.Keys[0].Use)
			assert.NotEmpty(t, jwks.Keys[0].N)
			assert.NotEmpty(t, jwks.Keys[0].E)
		})
	}
}

func TestInMemoryBackend_UpdateUserPool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget        error
		setup            func(b *cognitoidp.InMemoryBackend) string
		name             string
		mfaConfiguration string
		wantMfa          string
		wantErr          bool
	}{
		{
			name: "update_mfa_to_optional",
			setup: func(b *cognitoidp.InMemoryBackend) string {
				p, _ := b.CreateUserPool("pool")

				return p.ID
			},
			mfaConfiguration: "OPTIONAL",
			wantMfa:          "OPTIONAL",
		},
		{
			name: "pool_not_found",
			setup: func(_ *cognitoidp.InMemoryBackend) string {
				return "us-east-1_missing"
			},
			mfaConfiguration: "ON",
			wantErr:          true,
			errTarget:        cognitoidp.ErrUserPoolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			poolID := tt.setup(b)

			err := b.UpdateUserPool(poolID, tt.mfaConfiguration)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)

			pool, descErr := b.DescribeUserPool(poolID)
			require.NoError(t, descErr)
			assert.Equal(t, tt.wantMfa, pool.MfaConfiguration)
		})
	}
}

func TestInMemoryBackend_GetPoolMetrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget error
		name      string
		wantErr   bool
	}{
		{
			name: "success",
		},
		{
			name:      "pool_not_found",
			wantErr:   true,
			errTarget: cognitoidp.ErrUserPoolNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if !tt.wantErr {
				p, _ := b.CreateUserPool("pool")
				_, _ = b.AdminCreateUser(p.ID, "user1", "Pass1!", nil)
				_, _ = b.CreateUserPoolClient(p.ID, "client1")
				_, _ = b.CreateGroup(p.ID, "grp", "", 0)

				m, err := b.GetPoolMetrics(p.ID)
				require.NoError(t, err)
				assert.Equal(t, 1, m.UserCount)
				assert.Equal(t, 1, m.ClientCount)
				assert.Equal(t, 1, m.GroupCount)

				return
			}

			_, err := b.GetPoolMetrics("us-east-1_missing")
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.errTarget)
		})
	}
}

// TestHandler_CreateUserPool_MfaConfiguration proves CreateUserPool's
// MfaConfiguration request field (api_op_CreateUserPool.go) is actually
// stored on the pool it creates, rather than silently discarded in favor of
// the always-OFF default -- unlike UpdateUserPool/SetUserPoolMfaConfig,
// which do wire it through (see handleUpdateUserPoolWithOpts).
func TestHandler_CreateUserPool_MfaConfiguration(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doCognitoRequest(t, h, "CreateUserPool", map[string]any{
		"PoolName":         "mfa-at-create-pool",
		"MfaConfiguration": "ON",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp struct {
		UserPool struct {
			Id string `json:"Id"` //nolint:revive,staticcheck // matches wire field name.
		} `json:"UserPool"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))

	rec = doCognitoRequest(t, h, "GetUserPoolMfaConfig", map[string]any{
		"UserPoolId": createResp.UserPool.Id,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var mfaResp struct {
		MfaConfiguration string `json:"MfaConfiguration,omitempty"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &mfaResp))
	assert.Equal(t, "ON", mfaResp.MfaConfiguration)
}

func TestGetUserPoolMfaConfig_DefaultsToOFF(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	poolID, _ := setupHandlerPoolAndClient(t, h, "mfa-default-pool")

	rec := doCognitoRequest(t, h, "GetUserPoolMfaConfig", map[string]any{
		"UserPoolId": poolID,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		MfaConfiguration string `json:"MfaConfiguration,omitempty"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "OFF", out.MfaConfiguration)
}

// TestInMemoryBackend_UserPoolReplicas covers CreateUserPoolReplica's real
// validation (pool must exist, replica Region must differ from the primary
// pool's own Region, at most one secondary replica per user directory) plus
// the UpdateUserPoolReplica/DeleteUserPoolReplica/ListUserPoolReplicas
// lifecycle.
func TestInMemoryBackend_UserPoolReplicas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errTarget error
		setup     func(t *testing.T, b *cognitoidp.InMemoryBackend) (poolID, region string)
		name      string
		wantErr   bool
	}{
		{
			name: "success",
			setup: func(t *testing.T, b *cognitoidp.InMemoryBackend) (string, string) {
				t.Helper()

				pool, err := b.CreateUserPool("replica-pool-ok")
				require.NoError(t, err)

				return pool.ID, "us-west-2"
			},
		},
		{
			name: "pool_not_found",
			setup: func(t *testing.T, _ *cognitoidp.InMemoryBackend) (string, string) {
				t.Helper()

				return "us-east-1_nonexistent", "us-west-2"
			},
			wantErr:   true,
			errTarget: cognitoidp.ErrUserPoolNotFound,
		},
		{
			name: "same_region_as_primary_rejected",
			setup: func(t *testing.T, b *cognitoidp.InMemoryBackend) (string, string) {
				t.Helper()

				pool, err := b.CreateUserPool("replica-pool-same-region")
				require.NoError(t, err)

				// newTestBackend's primary Region is "us-east-1" -- see testhelpers_test.go.
				return pool.ID, "us-east-1"
			},
			wantErr:   true,
			errTarget: cognitoidp.ErrInvalidParameter,
		},
		{
			name: "second_replica_for_same_pool_rejected",
			setup: func(t *testing.T, b *cognitoidp.InMemoryBackend) (string, string) {
				t.Helper()

				pool, err := b.CreateUserPool("replica-pool-second")
				require.NoError(t, err)

				_, err = b.CreateUserPoolReplica(pool.ID, "us-west-2", nil)
				require.NoError(t, err)

				return pool.ID, "eu-west-1"
			},
			wantErr:   true,
			errTarget: cognitoidp.ErrInvalidParameter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			poolID, region := tt.setup(t, b)

			replica, err := b.CreateUserPoolReplica(poolID, region, map[string]string{"env": "test"})

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.errTarget)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, region, replica.RegionName)
			assert.Equal(t, "SECONDARY", replica.Role)
			assert.Equal(t, "INACTIVE", replica.Status)
			assert.Contains(t, replica.ARN, region)

			// Newly-created replica is listed.
			listed, err := b.ListUserPoolReplicas(poolID)
			require.NoError(t, err)
			require.Len(t, listed, 1)
			assert.Equal(t, region, listed[0].RegionName)

			// Activate it.
			updated, err := b.UpdateUserPoolReplica(poolID, region, "ACTIVE")
			require.NoError(t, err)
			assert.Equal(t, "ACTIVE", updated.Status)

			// Invalid status rejected.
			_, err = b.UpdateUserPoolReplica(poolID, region, "BOGUS")
			require.ErrorIs(t, err, cognitoidp.ErrInvalidParameter)

			// Updating/deleting a nonexistent region fails with ErrReplicaNotFound.
			_, err = b.UpdateUserPoolReplica(poolID, "ap-south-1", "ACTIVE")
			require.ErrorIs(t, err, cognitoidp.ErrReplicaNotFound)

			// Delete it -- returned copy reports DELETING, and it's gone from List.
			deleted, err := b.DeleteUserPoolReplica(poolID, region)
			require.NoError(t, err)
			assert.Equal(t, "DELETING", deleted.Status)

			listed, err = b.ListUserPoolReplicas(poolID)
			require.NoError(t, err)
			assert.Empty(t, listed)

			// Second delete fails: replica no longer exists.
			_, err = b.DeleteUserPoolReplica(poolID, region)
			require.Error(t, err)
			assert.ErrorIs(t, err, cognitoidp.ErrReplicaNotFound)
		})
	}
}

// TestDeleteUserPoolReplica_CleansResourceTags covers gopherstack-rdq3:
// unlike a user pool (random id), a replica's ARN is deterministic
// (region + pool id), so deleting a replica and recreating one for the same
// pool and Region genuinely inherits the dead replica's tags unless
// DeleteUserPoolReplica clears resourceTags[replicaARN].
func TestDeleteUserPoolReplica_CleansResourceTags(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	pool, err := b.CreateUserPool("replica-tags-pool")
	require.NoError(t, err)

	created, err := b.CreateUserPoolReplica(pool.ID, "us-west-2", map[string]string{"env": "prod"})
	require.NoError(t, err)
	replicaARN := created.ARN

	require.NotEmpty(t, b.ListTagsForResource(replicaARN))

	_, err = b.DeleteUserPoolReplica(pool.ID, "us-west-2")
	require.NoError(t, err)

	assert.Empty(t, b.ListTagsForResource(replicaARN), "deleted replica's ARN must not still resolve tags")

	// Recreate a replica for the same pool+Region: since the ARN is
	// deterministic, it must not inherit the dead replica's tags.
	recreated, err := b.CreateUserPoolReplica(pool.ID, "us-west-2", nil)
	require.NoError(t, err)
	require.Equal(t, replicaARN, recreated.ARN, "replica ARN must be deterministic for the same pool+Region")

	assert.Empty(
		t, b.ListTagsForResource(recreated.ARN), "recreated replica must not inherit the deleted replica's tags",
	)
}

// TestDeleteUserPoolReplica_DoesNotDisturbSiblingTags is the negative case:
// deleting one pool's replica must not touch another pool's replica tags.
func TestDeleteUserPoolReplica_DoesNotDisturbSiblingTags(t *testing.T) {
	t.Parallel()

	b := newTestBackend()

	keepPool, err := b.CreateUserPool("replica-keep-pool")
	require.NoError(t, err)
	doomedPool, err := b.CreateUserPool("replica-doomed-pool")
	require.NoError(t, err)

	kept, err := b.CreateUserPoolReplica(keepPool.ID, "us-west-2", map[string]string{"keep": "me"})
	require.NoError(t, err)
	doomed, err := b.CreateUserPoolReplica(doomedPool.ID, "us-west-2", map[string]string{"doomed": "yes"})
	require.NoError(t, err)

	_, err = b.DeleteUserPoolReplica(doomedPool.ID, "us-west-2")
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"keep": "me"}, b.ListTagsForResource(kept.ARN))
	assert.Empty(t, b.ListTagsForResource(doomed.ARN))
}

// TestHandler_UserPoolReplicas covers the HTTP handler wire shape for
// CreateUserPoolReplica/ListUserPoolReplicas/UpdateUserPoolReplica/
// DeleteUserPoolReplica end to end.
func TestHandler_UserPoolReplicas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		regionName string
		wantCode   int
	}{
		{name: "success", regionName: "us-west-2", wantCode: http.StatusOK},
		{name: "same_region_rejected", regionName: "us-east-1", wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			poolID, _ := setupHandlerPoolAndClient(t, h, "replica-handler-pool")

			createRec := doCognitoRequest(t, h, "CreateUserPoolReplica", map[string]any{
				"UserPoolId": poolID,
				"RegionName": tt.regionName,
			})
			require.Equal(t, tt.wantCode, createRec.Code, createRec.Body.String())

			if tt.wantCode != http.StatusOK {
				return
			}

			var createResp struct {
				UserPoolReplica struct {
					RegionName  string `json:"RegionName,omitempty"`
					Role        string `json:"Role,omitempty"`
					Status      string `json:"Status,omitempty"`
					UserPoolArn string `json:"UserPoolArn,omitempty"`
				} `json:"UserPoolReplica"`
			}
			require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
			assert.Equal(t, tt.regionName, createResp.UserPoolReplica.RegionName)
			assert.Equal(t, "SECONDARY", createResp.UserPoolReplica.Role)
			assert.Equal(t, "INACTIVE", createResp.UserPoolReplica.Status)
			assert.NotEmpty(t, createResp.UserPoolReplica.UserPoolArn)

			listRec := doCognitoRequest(t, h, "ListUserPoolReplicas", map[string]any{
				"UserPoolId": poolID,
			})
			require.Equal(t, http.StatusOK, listRec.Code)

			var listResp struct {
				UserPoolReplicas []struct {
					RegionName string `json:"RegionName,omitempty"`
				} `json:"UserPoolReplicas,omitempty"`
			}
			require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
			require.Len(t, listResp.UserPoolReplicas, 1)
			assert.Equal(t, tt.regionName, listResp.UserPoolReplicas[0].RegionName)

			updateRec := doCognitoRequest(t, h, "UpdateUserPoolReplica", map[string]any{
				"UserPoolId": poolID,
				"RegionName": tt.regionName,
				"Status":     "ACTIVE",
			})
			require.Equal(t, http.StatusOK, updateRec.Code)

			var updateResp struct {
				UserPoolReplica struct {
					Status string `json:"Status,omitempty"`
				} `json:"UserPoolReplica"`
			}
			require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updateResp))
			assert.Equal(t, "ACTIVE", updateResp.UserPoolReplica.Status)

			deleteRec := doCognitoRequest(t, h, "DeleteUserPoolReplica", map[string]any{
				"UserPoolId": poolID,
				"RegionName": tt.regionName,
			})
			require.Equal(t, http.StatusOK, deleteRec.Code)

			var deleteResp struct {
				UserPoolReplica struct {
					Status string `json:"Status,omitempty"`
				} `json:"UserPoolReplica"`
			}
			require.NoError(t, json.Unmarshal(deleteRec.Body.Bytes(), &deleteResp))
			assert.Equal(t, "DELETING", deleteResp.UserPoolReplica.Status)
		})
	}
}
