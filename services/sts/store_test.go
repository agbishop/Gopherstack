package sts_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/services/sts"
)

func TestNewInMemoryBackendWithConfig_GetCallerIdentityUsesInjectedAccountID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		accountID string
		wantARN   string
	}{
		{
			name:      "default account",
			accountID: sts.MockAccountID,
			wantARN:   "arn:aws:iam::000000000000:root",
		},
		{
			name:      "custom account",
			accountID: "123456789012",
			wantARN:   "arn:aws:iam::123456789012:root",
		},
		{
			name:      "another custom account",
			accountID: "999999999999",
			wantARN:   "arn:aws:iam::999999999999:root",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackendWithConfig(tc.accountID)
			resp, err := b.GetCallerIdentity("", "")
			require.NoError(t, err)
			assert.Equal(t, tc.accountID, resp.GetCallerIdentityResult.Account)
			assert.Equal(t, tc.wantARN, resp.GetCallerIdentityResult.Arn)
		})
	}
}

func TestMockAccountID_IsZeroesNotAmazonAccount(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "000000000000", sts.MockAccountID,
		"MockAccountID should be all-zeros to match other services")
}

// TestAssumeRole_EvictsExpiredSessionsWithoutJanitor verifies that creating a
// new session opportunistically sweeps expired sessions once the store grows
// past the eviction threshold, so b.sessions stays bounded even when the
// background janitor is disabled.
func TestAssumeRole_EvictsExpiredSessionsWithoutJanitor(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()

	// Seed more expired sessions than the eviction threshold so the next insert
	// triggers the inline sweep.
	expired := sts.SessionEvictThreshold + 16
	past := time.Now().UTC().Add(-time.Hour)

	for i := range expired {
		akid := fmt.Sprintf("ASIAEXPIRED%06d", i)
		b.AddSessionInternal(&sts.SessionInfo{
			AccessKeyID: akid,
			Expiration:  past,
		})
	}

	require.Equal(t, expired, b.SessionCount(), "all seeded expired sessions present before insert")

	// A single real AssumeRole insert should evict every expired session and add
	// exactly one live session.
	resp, err := b.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/Role1",
		RoleSessionName: "live",
		DurationSeconds: 900,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AssumeRoleResult.Credentials.AccessKeyID)

	require.Equal(t, 1, b.SessionCount(),
		"expired sessions must be evicted on insert, leaving only the live one")
}

// TestAssumeRole_BelowThresholdKeepsExpired confirms the sweep is a no-op below
// the threshold (steady-state inserts stay O(1) and do not eagerly scan).
func TestAssumeRole_BelowThresholdKeepsExpired(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	past := time.Now().UTC().Add(-time.Hour)

	b.AddSessionInternal(&sts.SessionInfo{AccessKeyID: "ASIAEXPIRED0", Expiration: past})

	_, err := b.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/Role1",
		RoleSessionName: "live",
		DurationSeconds: 900,
	})
	require.NoError(t, err)

	// Below threshold: the expired session is not eagerly swept, so both remain.
	require.Equal(t, 2, b.SessionCount())
}

// TestStoreSessionEvictionSeparateFromStore verifies that storeSession does not
// hold the write-lock during eviction: seeding sessions beyond the evict
// threshold and then storing a new one via AssumeRole must trigger the sweep.
func TestStoreSessionEvictionSeparateFromStore(t *testing.T) {
	t.Parallel()

	// Seed sessions beyond the evict threshold, then verify a new storeSession
	// call (via AssumeRole) evicts expired ones without blocking other stores.
	// The test exercises the threshold crossing to confirm eviction triggers.
	b := sts.NewInMemoryBackend()

	const aboveThreshold = sts.SessionEvictThreshold + 10

	// Seed expired sessions via AddSessionInternal to bypass storeSession.
	for i := range aboveThreshold {
		b.AddSessionInternal(&sts.SessionInfo{
			AccessKeyID: fmt.Sprintf("ASIA%016d", i),
			Expiration:  time.Now().UTC().Add(-time.Hour),
		})
	}

	require.Equal(
		t,
		aboveThreshold,
		b.SessionCount(),
		"all seeded expired sessions present before trigger",
	)

	// AssumeRole stores a new live session, triggering maybeEvictExpiredSessions.
	resp, err := b.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/R",
		RoleSessionName: "live",
		DurationSeconds: 900,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.AssumeRoleResult.Credentials.AccessKeyID)

	// After eviction, only the one live session should remain.
	assert.Equal(t, 1, b.SessionCount(), "expired sessions evicted, only live session remains")
}

// TestSessionExpiryConsistent verifies the isSessionExpired helper is applied
// consistently by ValidateSessionCredential and GetCallerIdentity.
func TestSessionExpiryConsistent(t *testing.T) {
	t.Parallel()

	t.Run("expired_session_rejected_by_validate_credential", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/R",
			RoleSessionName: "session",
		})
		require.NoError(t, err)

		creds := resp.AssumeRoleResult.Credentials
		// Force expiry in the past.
		b.SetSessionExpiration(creds.AccessKeyID, time.Now().Add(-time.Minute))

		_, err = b.ValidateSessionCredential(creds.AccessKeyID, creds.SessionToken)
		require.ErrorIs(t, err, sts.ErrSessionNotFound)
	})

	t.Run("expired_session_returns_expired_token_error", func(t *testing.T) {
		t.Parallel()

		b := sts.NewInMemoryBackend()
		resp, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/R",
			RoleSessionName: "session",
		})
		require.NoError(t, err)

		creds := resp.AssumeRoleResult.Credentials
		b.SetSessionExpiration(creds.AccessKeyID, time.Now().Add(-time.Minute))

		// After expiry GetCallerIdentity returns ExpiredTokenException, matching AWS.
		_, err = b.GetCallerIdentity(creds.AccessKeyID, "")
		require.ErrorIs(t, err, sts.ErrSessionExpired)
	})
}

// TestResolvePrincipal_KindReflectsSessionType verifies that ResolvePrincipal
// (the awsmeta.PrincipalResolver implementation consumed by IAM's cross-service
// enforcement middleware) only reports Kind=AssumedRole for sessions minted by
// an actual role-assumption operation. GetSessionToken/GetFederationToken/
// GetDelegatedAccessToken keep the caller's own identity — they are not role
// assumptions — and mislabeling them as AssumedRole sends
// services/iam/middleware.go's resolveAssumedRoleIdentityPolicies down the
// GetPoliciesForRole path with a garbage "role name" derived from a user/root/
// federated-user ARN, which fails to resolve and silently falls through to
// unenforced (full IAM bypass) instead of any policy check.
func TestResolvePrincipal_KindReflectsSessionType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mint     func(t *testing.T, b *sts.InMemoryBackend) (accessKeyID, sessionToken string)
		name     string
		wantKind awsmeta.PrincipalKind
	}{
		{
			name: "assume_role_is_assumed_role",
			mint: func(t *testing.T, b *sts.InMemoryBackend) (string, string) {
				t.Helper()

				resp, err := b.AssumeRole(&sts.AssumeRoleInput{
					RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
					RoleSessionName: "sess",
				})
				require.NoError(t, err)

				creds := resp.AssumeRoleResult.Credentials

				return creds.AccessKeyID, creds.SessionToken
			},
			wantKind: awsmeta.PrincipalKindAssumedRole,
		},
		{
			name: "assume_root_is_assumed_role",
			mint: func(t *testing.T, b *sts.InMemoryBackend) (string, string) {
				t.Helper()

				resp, err := b.AssumeRoot(&sts.AssumeRootInput{
					TargetPrincipal: "123456789012",
					TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				})
				require.NoError(t, err)

				creds := resp.AssumeRootResult.Credentials

				return creds.AccessKeyID, creds.SessionToken
			},
			wantKind: awsmeta.PrincipalKindAssumedRole,
		},
		{
			name: "get_session_token_is_not_assumed_role",
			mint: func(t *testing.T, b *sts.InMemoryBackend) (string, string) {
				t.Helper()

				resp, err := b.GetSessionToken(&sts.GetSessionTokenInput{})
				require.NoError(t, err)

				creds := resp.GetSessionTokenResult.Credentials

				return creds.AccessKeyID, creds.SessionToken
			},
			wantKind: awsmeta.PrincipalKindUser,
		},
		{
			name: "get_federation_token_is_not_assumed_role",
			mint: func(t *testing.T, b *sts.InMemoryBackend) (string, string) {
				t.Helper()

				resp, err := b.GetFederationToken(&sts.GetFederationTokenInput{Name: "alice"})
				require.NoError(t, err)

				creds := resp.GetFederationTokenResult.Credentials

				return creds.AccessKeyID, creds.SessionToken
			},
			wantKind: awsmeta.PrincipalKindUser,
		},
		{
			name: "get_delegated_access_token_is_not_assumed_role",
			mint: func(t *testing.T, b *sts.InMemoryBackend) (string, string) {
				t.Helper()

				resp, err := b.GetDelegatedAccessToken(&sts.GetDelegatedAccessTokenInput{TradeInToken: "trade-in-1"})
				require.NoError(t, err)

				creds := resp.GetDelegatedAccessTokenResult.Credentials

				return creds.AccessKeyID, creds.SessionToken
			},
			wantKind: awsmeta.PrincipalKindUser,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			accessKeyID, sessionToken := tt.mint(t, b)

			p, ok := b.ResolvePrincipal(t.Context(), accessKeyID, sessionToken)
			require.True(t, ok)
			assert.Equal(t, tt.wantKind, p.Kind)
		})
	}
}

// TestLookupSession_RequiresSessionTokenForTempCredentials verifies that an
// ASIA-prefixed access key ID alone, without its matching X-Amz-Security-Token,
// does not resolve to the session (gopherstack-g58j). Real AWS rejects a
// SigV4 request for temporary credentials that omits or misstates the session
// token; an absent token must not be treated as an automatic match.
func TestLookupSession_RequiresSessionTokenForTempCredentials(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	resp, err := b.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
		RoleSessionName: "sess",
	})
	require.NoError(t, err)

	creds := resp.AssumeRoleResult.Credentials
	require.NotEmpty(t, creds.SessionToken)

	assert.Nil(t, b.LookupSession(creds.AccessKeyID, ""), "missing token must not match")
	assert.Nil(t, b.LookupSession(creds.AccessKeyID, "wrong-token"), "wrong token must not match")
	assert.NotNil(t, b.LookupSession(creds.AccessKeyID, creds.SessionToken), "correct token must match")

	_, ok := b.ResolvePrincipal(t.Context(), creds.AccessKeyID, "")
	assert.False(t, ok, "ResolvePrincipal must not impersonate the session without its token")
}

// TestAccountID verifies AccountID() returns a non-empty string.
func TestAccountID(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	assert.NotEmpty(t, b.AccountID())
}

// TestRegion verifies Region() returns "us-east-1".
func TestRegion(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	assert.Equal(t, "us-east-1", b.Region())
}

// TestErrSessionNotFoundDefined verifies ErrSessionNotFound is defined with the expected message.
func TestErrSessionNotFoundDefined(t *testing.T) {
	t.Parallel()

	require.Error(t, sts.ErrSessionNotFound)
	assert.Equal(t, "session not found", sts.ErrSessionNotFound.Error())
}

// TestResetClearsSessions verifies Reset() clears sessions via AddSessionInternal.
func TestResetClearsSessions(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	b.AddSessionInternal(&sts.SessionInfo{
		AccessKeyID:    "ASIATEST000000000001",
		AssumedRoleArn: "arn:aws:sts::000000000000:assumed-role/test/session",
		AccountID:      sts.MockAccountID,
		SessionName:    "session",
		Expiration:     time.Now().Add(time.Hour),
	})

	assert.Equal(t, 1, b.SessionCount())

	h := sts.NewHandler(b)
	h.Reset()

	assert.Equal(t, 0, b.SessionCount())
}

// TestAddSessionInternalAndCount verifies AddSessionInternal increases SessionCount.
func TestAddSessionInternalAndCount(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	assert.Equal(t, 0, b.SessionCount())

	for i := range 3 {
		b.AddSessionInternal(&sts.SessionInfo{
			AccessKeyID:    fmt.Sprintf("ASIATEST%012d", i),
			AssumedRoleArn: "arn:aws:sts::000000000000:assumed-role/test/session",
			AccountID:      sts.MockAccountID,
			SessionName:    fmt.Sprintf("session-%d", i),
			Expiration:     time.Now().Add(time.Hour),
		})
	}

	assert.Equal(t, 3, b.SessionCount())
}

// TestBackendDefaultAccountID verifies default account ID is MockAccountID.
func TestBackendDefaultAccountID(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	assert.Equal(t, sts.MockAccountID, b.AccountID())
}

// TestOperationCounters verifies per-operation call counters are incremented
// when operations are invoked.
func TestOperationCounters(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()
	h := sts.NewHandler(backend)

	// Before any operations, metrics should all be zero.
	initialMetrics := h.SessionMetrics()
	assert.Equal(t, int64(0), initialMetrics.OpsAssumeRole)
	assert.Equal(t, int64(0), initialMetrics.OpsGetSessionToken)
	assert.Equal(t, int64(0), initialMetrics.TotalSessionsCreated)

	// Call AssumeRole.
	_, err := backend.AssumeRole(&sts.AssumeRoleInput{
		RoleArn:         "arn:aws:iam::000000000000:role/CounterRole",
		RoleSessionName: "counter-session",
	})
	require.NoError(t, err)

	// Call GetSessionToken.
	_, err = backend.GetSessionToken(&sts.GetSessionTokenInput{DurationSeconds: 3600})
	require.NoError(t, err)

	// Call GetCallerIdentity.
	_, err = backend.GetCallerIdentity("", "")
	require.NoError(t, err)

	// Call GetFederationToken.
	_, err = backend.GetFederationToken(&sts.GetFederationTokenInput{Name: "fed-user"})
	require.NoError(t, err)

	// Check metrics reflect the calls.
	metrics := h.SessionMetrics()
	assert.Equal(t, int64(1), metrics.OpsAssumeRole, "AssumeRole counter")
	assert.Equal(t, int64(1), metrics.OpsGetSessionToken, "GetSessionToken counter")
	assert.Equal(t, int64(1), metrics.OpsGetCallerIdentity, "GetCallerIdentity counter")
	assert.Equal(t, int64(1), metrics.OpsGetFederationToken, "GetFederationToken counter")
	assert.Equal(t, int64(3), metrics.TotalSessionsCreated, "3 sessions created (1 role + 1 session + 1 fed)")
}

// TestTotalSessionsCreated verifies the lifetime session counter is correct
// across multiple session-issuing operations.
func TestTotalSessionsCreated(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()
	h := sts.NewHandler(backend)

	calls := []func() error{
		func() error {
			_, e := backend.AssumeRole(&sts.AssumeRoleInput{
				RoleArn:         "arn:aws:iam::000000000000:role/R1",
				RoleSessionName: "s1",
			})

			return e
		},
		func() error {
			_, e := backend.GetSessionToken(&sts.GetSessionTokenInput{DurationSeconds: 3600})

			return e
		},
		func() error {
			_, e := backend.GetFederationToken(&sts.GetFederationTokenInput{Name: "fed1"})

			return e
		},
		func() error {
			_, e := backend.AssumeRoot(&sts.AssumeRootInput{
				TargetPrincipal: "000000000000",
				TaskPolicyArn:   "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
			})

			return e
		},
	}

	for _, call := range calls {
		require.NoError(t, call())
	}

	metrics := h.SessionMetrics()
	assert.Equal(t, int64(len(calls)), metrics.TotalSessionsCreated)
}

// TestConcurrentSessions verifies that concurrent session creation does not
// cause data races or missed counter increments.
func TestConcurrentSessions(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackend()
	h := sts.NewHandler(backend)

	const workers = 20

	var wg sync.WaitGroup

	for range workers {
		wg.Go(func() {
			_, err := backend.AssumeRole(&sts.AssumeRoleInput{
				RoleArn:         "arn:aws:iam::000000000000:role/ConcurrencyRole",
				RoleSessionName: "sess-concurrent",
			})
			assert.NoError(t, err)
		})
	}

	wg.Wait()

	metrics := h.SessionMetrics()
	assert.Equal(t, int64(workers), metrics.TotalSessionsCreated)
	assert.Equal(t, int64(workers), metrics.OpsAssumeRole)
	assert.Equal(t, workers, metrics.ActiveSessions)
}
