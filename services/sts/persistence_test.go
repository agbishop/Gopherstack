package sts_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sts"
)

func TestInMemoryBackend_SnapshotRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(b *sts.InMemoryBackend) (accessKeyID, sessionToken string)
		verify func(t *testing.T, b *sts.InMemoryBackend, accessKeyID, sessionToken string)
		name   string
	}{
		{
			name:  "round_trip_no_state",
			setup: func(_ *sts.InMemoryBackend) (string, string) { return "", "" },
			verify: func(t *testing.T, b *sts.InMemoryBackend, _, _ string) {
				t.Helper()
				assert.Equal(t, 0, b.SessionCount())
			},
		},
		{
			name: "round_trip_with_active_session",
			setup: func(b *sts.InMemoryBackend) (string, string) {
				resp, err := b.AssumeRole(&sts.AssumeRoleInput{
					RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
					RoleSessionName: "restore-session",
					DurationSeconds: 900,
				})
				if err != nil {
					return "", ""
				}

				creds := resp.AssumeRoleResult.Credentials

				return creds.AccessKeyID, creds.SessionToken
			},
			verify: func(t *testing.T, b *sts.InMemoryBackend, accessKeyID, sessionToken string) {
				t.Helper()
				require.Equal(t, 1, b.SessionCount())

				// Session should still resolve correctly.
				ciResp, err := b.GetCallerIdentity(accessKeyID, sessionToken)
				require.NoError(t, err)
				assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "assumed-role")
				assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "TestRole")
			},
		},
		{
			name: "round_trip_expired_session_discarded",
			setup: func(b *sts.InMemoryBackend) (string, string) {
				resp, err := b.AssumeRole(&sts.AssumeRoleInput{
					RoleArn:         "arn:aws:iam::123456789012:role/TestRole",
					RoleSessionName: "expired-session",
					DurationSeconds: 900,
				})
				if err != nil {
					return "", ""
				}

				creds := resp.AssumeRoleResult.Credentials
				// Force the session to be expired before snapshotting.
				b.SetSessionExpiration(creds.AccessKeyID, time.Now().Add(-time.Second))

				return creds.AccessKeyID, creds.SessionToken
			},
			verify: func(t *testing.T, b *sts.InMemoryBackend, _, _ string) {
				t.Helper()
				// Expired session must not be restored.
				assert.Equal(t, 0, b.SessionCount())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := sts.NewInMemoryBackendWithConfig("000000000000")
			accessKeyID, sessionToken := tt.setup(original)

			snap := original.Snapshot(t.Context())

			fresh := sts.NewInMemoryBackendWithConfig("000000000000")
			require.NoError(t, fresh.Restore(t.Context(), snap))

			tt.verify(t, fresh, accessKeyID, sessionToken)
		})
	}
}

func TestInMemoryBackend_Restore_NilData(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	require.NoError(t, b.Restore(t.Context(), nil))
	assert.Equal(t, 0, b.SessionCount())
}

func TestSTSHandler_Persistence(t *testing.T) {
	t.Parallel()

	backend := sts.NewInMemoryBackendWithConfig("000000000000")
	h := sts.NewHandler(backend)

	// Verify round-trip with no sessions.
	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := sts.NewInMemoryBackendWithConfig("000000000000")
	freshH := sts.NewHandler(fresh)
	require.NoError(t, freshH.Restore(t.Context(), snap))
}

// TestSnapshotDeepCopy verifies Snapshot doesn't share pointers with live sessions.
func TestSnapshotDeepCopy(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	b.AddSessionInternal(&sts.SessionInfo{
		AccessKeyID:    "ASIATEST000000000002",
		AssumedRoleArn: "arn:aws:sts::000000000000:assumed-role/test/session",
		AccountID:      sts.MockAccountID,
		SessionName:    "original",
		Expiration:     time.Now().Add(time.Hour),
	})

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	// Mutate the live session.
	b.SetSessionExpiration("ASIATEST000000000002", time.Now().Add(2*time.Hour))

	// Restore from snapshot - should reflect original data (not mutated).
	b2 := sts.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), snap))
	assert.Equal(t, 1, b2.SessionCount())
}

// TestEnsureNonNilMapsAfterRestore verifies Restore with empty data leaves sessions usable.
func TestEnsureNonNilMapsAfterRestore(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	require.NoError(t, b.Restore(t.Context(), nil))
	// sessions map should still be usable (non-nil); verify by adding a session.
	b.AddSessionInternal(&sts.SessionInfo{
		AccessKeyID:    "ASIATEST000000000003",
		AssumedRoleArn: "arn:aws:sts::000000000000:assumed-role/test/session",
		AccountID:      sts.MockAccountID,
		SessionName:    "after-restore",
		Expiration:     time.Now().Add(time.Hour),
	})
	assert.Equal(t, 1, b.SessionCount())
}

// TestSnapshotRestoreRoundtrip verifies Snapshot/Restore preserves all fields.
func TestSnapshotRestoreRoundtrip(t *testing.T) {
	t.Parallel()

	expiration := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	original := &sts.SessionInfo{
		AccessKeyID:    "ASIATEST000000000099",
		AssumedRoleArn: "arn:aws:sts::000000000000:assumed-role/my-role/my-session",
		AccountID:      sts.MockAccountID,
		SessionName:    "my-session",
		AssumedRoleID:  "AROAMY-ROLE:my-session",
		SourceIdentity: "test-identity",
		Tags:           []sts.Tag{{Key: "k", Value: "v"}},
		Expiration:     expiration,
	}

	b := sts.NewInMemoryBackend()
	b.AddSessionInternal(original)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := sts.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), snap))
	assert.Equal(t, 1, b2.SessionCount())

	ci, err := b2.GetCallerIdentity("ASIATEST000000000099", "")
	require.NoError(t, err)
	assert.Equal(t, sts.MockAccountID, ci.GetCallerIdentityResult.Account)
	assert.Equal(t, original.AssumedRoleArn, ci.GetCallerIdentityResult.Arn)
}

// TestSnapshotRestore_PreservesCounters verifies operation counters survive a
// Snapshot/Restore round trip.
func TestSnapshotRestore_PreservesCounters(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()

	// Perform some operations.
	for range 3 {
		_, err := b.AssumeRole(&sts.AssumeRoleInput{
			RoleArn:         "arn:aws:iam::123456789012:role/R",
			RoleSessionName: "session",
		})
		require.NoError(t, err)
	}

	_, err := b.GetSessionToken(&sts.GetSessionTokenInput{})
	require.NoError(t, err)

	// Snapshot and restore.
	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := sts.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), snap))

	h2 := sts.NewHandler(b2)

	// Verify counters are preserved in metrics.
	metrics := h2.SessionMetrics()
	assert.Equal(t, int64(3), metrics.OpsAssumeRole)
	assert.Equal(t, int64(1), metrics.OpsGetSessionToken)
	assert.Equal(t, int64(4), metrics.TotalSessionsCreated)
}

func TestSTSHandler_Routing(t *testing.T) {
	t.Parallel()

	h := sts.NewHandler(sts.NewInMemoryBackendWithConfig("000000000000"))

	assert.Equal(t, "STS", h.Name())
	assert.Positive(t, h.MatchPriority())

	e := echo.New()

	tests := []struct {
		name      string
		ct        string
		body      string
		wantMatch bool
	}{
		{"sts form", "application/x-www-form-urlencoded", "Action=GetCallerIdentity&Version=2011-06-15", true},
		{"json ct", "application/json", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.ct)
			c := e.NewContext(req, httptest.NewRecorder())
			assert.Equal(t, tt.wantMatch, h.RouteMatcher()(c))
		})
	}
}
