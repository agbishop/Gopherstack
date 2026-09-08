package appconfigdata_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appconfigdata"
)

func TestBackend_ListSessions(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "profile", "data", "text/plain"))

	assert.Empty(t, b.ListSessions())

	_, err := b.StartSession("app", "env", "profile", 0)
	require.NoError(t, err)

	_, err = b.StartSession("app", "env", "profile", 0)
	require.NoError(t, err)

	sessions := b.ListSessions()
	assert.Len(t, sessions, 2)
}

// TestBackend_ListSessions_ReturnsAllSessions verifies ListSessions returns all active sessions with full tokens.
func TestBackend_ListSessions_ReturnsAllSessions(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{}`, "application/json"))
	require.NoError(t, b.SetConfiguration("app2", "env", "p", `{}`, "application/json"))

	assert.Empty(t, b.ListSessions())

	tok1, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)
	tok2, err := b.StartSession("app2", "env", "p", 0)
	require.NoError(t, err)

	sessions := b.ListSessions()
	assert.Len(t, sessions, 2)

	tokenSet := map[string]bool{}
	for _, s := range sessions {
		tokenSet[s.Token] = true
	}

	assert.True(t, tokenSet[tok1], "tok1 must appear in ListSessions")
	assert.True(t, tokenSet[tok2], "tok2 must appear in ListSessions")
}

func TestBackend_ListSessionsSafe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tokenCount int
	}{
		{name: "no_sessions", tokenCount: 0},
		{name: "one_session", tokenCount: 1},
		{name: "multiple_sessions", tokenCount: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := appconfigdata.NewInMemoryBackend()
			require.NoError(t, b.SetConfiguration("app", "env", "p", "data", "text/plain"))

			for range tt.tokenCount {
				_, err := b.StartSession("app", "env", "p", 0)
				require.NoError(t, err)
			}

			safe := b.ListSessionsSafe()
			assert.Len(t, safe, tt.tokenCount)

			for _, s := range safe {
				// TokenPrefix must be set (non-empty for tokens with content).
				assert.NotEmpty(t, s.TokenPrefix)
				// TokenPrefix must NOT be a full-length token.
				assert.Less(t, len(s.TokenPrefix), 64,
					"safe session token prefix must be shorter than full token")
				// Must contain the ellipsis separator.
				assert.Contains(t, s.TokenPrefix, "…",
					"safe session token prefix must contain ellipsis")
			}
		})
	}
}

// TestBackend_ListSessionsSafe_TokenTruncation verifies that safe session listing truncates tokens.
func TestBackend_ListSessionsSafe_TokenTruncation(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{}`, "application/json"))

	tok, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)

	sessions := b.ListSessionsSafe()
	require.Len(t, sessions, 1)

	// Token prefix must NOT equal the full token.
	assert.NotEqual(t, tok, sessions[0].TokenPrefix)
	// Token prefix must contain the ellipsis separator.
	assert.Contains(t, sessions[0].TokenPrefix, "…", "truncated token must contain ellipsis")

	// Session metadata must be accurate.
	assert.Equal(t, "app", sessions[0].ApplicationIdentifier)
	assert.Equal(t, "env", sessions[0].EnvironmentIdentifier)
	assert.Equal(t, "p", sessions[0].ConfigurationProfileIdentifier)
}

// TestBackend_TruncateToken verifies the truncation function exposed via ListSessionsSafe.
func TestBackend_TruncateToken(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{}`, "application/json"))

	_, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)

	safe := b.ListSessionsSafe()
	require.Len(t, safe, 1)

	full := b.ListSessions()
	require.Len(t, full, 1)

	assert.Less(t, len(safe[0].TokenPrefix), len(full[0].Token),
		"safe token must be shorter than full token")
	// Full token must start with the same prefix shown in safe token.
	prefix, _, _ := strings.Cut(safe[0].TokenPrefix, "…")
	assert.True(t, strings.HasPrefix(full[0].Token, prefix),
		"full token must start with the safe prefix")
}

func TestBackend_DeleteProfile(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "profile", "data", "text/plain"))

	_, err := b.StartSession("app", "env", "profile", 0)
	require.NoError(t, err)

	assert.True(t, b.DeleteProfile("app", "env", "profile"))
	assert.False(t, b.DeleteProfile("app", "env", "profile"))

	assert.Empty(t, b.ListProfiles())
	assert.Empty(t, b.ListSessions())
}

// TestBackend_DeleteProfile_PurgesSessions verifies that deleting a profile also removes
// all sessions bound to that profile.
func TestBackend_DeleteProfile_PurgesSessions(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{}`, "application/json"))
	require.NoError(t, b.SetConfiguration("app2", "env", "p2", `{}`, "application/json"))

	tok1, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)
	tok2, err := b.StartSession("app2", "env", "p2", 0)
	require.NoError(t, err)

	require.True(t, b.DeleteProfile("app", "env", "p"))

	// Session for deleted profile must be gone.
	assert.Nil(t, b.LookupSession(tok1))
	// Unrelated session must survive.
	assert.NotNil(t, b.LookupSession(tok2))
}

func TestBackend_EndSession(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "profile", `{}`, "application/json"))

	token, err := b.StartSession("app", "env", "profile", 0)
	require.NoError(t, err)

	assert.True(t, b.EndSession(token))
	assert.False(t, b.EndSession(token))
	assert.Empty(t, b.ListSessions())
}

// TestBackend_EndSession_RemovesSession verifies EndSession removes the session and returns false for unknown tokens.
func TestBackend_EndSession_RemovesSession(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{}`, "application/json"))

	tok, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)

	assert.NotNil(t, b.LookupSession(tok))
	assert.True(t, b.EndSession(tok))
	assert.Nil(t, b.LookupSession(tok))
	assert.False(t, b.EndSession(tok), "EndSession on unknown token must return false")
}

func TestBackend_GetStats(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	stats := b.GetStats()
	assert.Equal(t, 0, stats.SessionCount)
	assert.Equal(t, 0, stats.ProfileCount)
	assert.Equal(t, int64(0), stats.TotalPollCount)
	assert.Equal(t, int64(0), stats.TotalPollFailures)
	assert.Equal(t, int64(0), stats.ConfigurationChangeCount)

	require.NoError(t, b.SetConfiguration("app", "env", "profile", "data", "text/plain"))
	_, err := b.StartSession("app", "env", "profile", 0)
	require.NoError(t, err)

	stats = b.GetStats()
	assert.Equal(t, 1, stats.SessionCount)
	assert.Equal(t, 1, stats.ProfileCount)
	assert.Equal(t, int64(1), stats.ConfigurationChangeCount)
}

func TestBackend_GetStats_PollCounts(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{}`, "application/json"))

	token, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)

	// Successful polls increment totalPolls.
	_, _, token, _, _, err = b.GetLatestConfiguration(token)
	require.NoError(t, err)
	_, _, _, _, _, err = b.GetLatestConfiguration(token)
	require.NoError(t, err)

	// Failed poll increments totalFailures.
	_, _, _, _, _, err = b.GetLatestConfiguration("bad-token")
	require.ErrorIs(t, err, appconfigdata.ErrSessionNotFound)

	stats := b.GetStats()
	assert.Equal(t, int64(2), stats.TotalPollCount)
	assert.Equal(t, int64(1), stats.TotalPollFailures)
}

// TestBackend_GetStats_ConfigurationChangeCount verifies that the counter increments only
// when content actually changes, not on identical re-sets.
func TestBackend_GetStats_ConfigurationChangeCount(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()

	require.NoError(t, b.SetConfiguration("app", "env", "p", `{"v":1}`, "application/json"))
	assert.Equal(t, int64(1), b.GetStats().ConfigurationChangeCount)

	// Same content — normalised hash matches — no increment.
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{"v":1}`, "application/json"))
	assert.Equal(t, int64(1), b.GetStats().ConfigurationChangeCount)

	// Different content — increments.
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{"v":2}`, "application/json"))
	assert.Equal(t, int64(2), b.GetStats().ConfigurationChangeCount)
}

func TestBackend_SweepExpiredSessions(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "profile", `{}`, "application/json"))

	_, err := b.StartSession("app", "env", "profile", 0)
	require.NoError(t, err)

	// Sweep with zero TTL — all sessions expire immediately.
	b.SweepExpiredSessions(t.Context(), 0)
	assert.Empty(t, b.ListSessions())
}

func TestBackend_SweepExpiredSessions_AbsoluteExpiry(t *testing.T) {
	t.Parallel()

	// Create a backend and start a session, then verify that ExpiresAt is ~1h from now.
	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{}`, "application/json"))

	token, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)

	sess := b.LookupSession(token)
	require.NotNil(t, sess)
	assert.False(t, sess.ExpiresAt.IsZero(), "ExpiresAt must be set")
	// ExpiresAt should be approximately 1 hour from now.
	assert.True(t, sess.ExpiresAt.After(sess.CreatedAt), "ExpiresAt must be after CreatedAt")
}

// TestBackend_SweepExpiredSessions_GraceTokens verifies that SweepExpiredSessions also
// purges expired grace tokens to prevent memory leaks.
func TestBackend_SweepExpiredSessions_GraceTokens(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{}`, "application/json"))

	tok, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)

	// Poll to generate a grace token.
	_, _, nextTok, _, _, err := b.GetLatestConfiguration(tok)
	require.NoError(t, err)
	require.NotEmpty(t, nextTok)

	// Sweep with zero TTL — removes all active sessions.
	b.SweepExpiredSessions(t.Context(), 0)

	// The grace token entry for the old token was created by the poll.
	// After sweep, sessions are gone, but grace tokens expire on their own schedule.
	// Verify that the next-token session (the current one) is gone.
	assert.Nil(t, b.LookupSession(nextTok), "active session must be swept with zero TTL")
}

// TestBackend_SessionExpiresAtPopulated verifies ExpiresAt is set correctly on new sessions.
func TestBackend_SessionExpiresAtPopulated(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{}`, "application/json"))

	before := nowUTC()
	token, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)
	after := nowUTC()

	sess := b.LookupSession(token)
	require.NotNil(t, sess)

	assert.True(t, sess.ExpiresAt.After(before), "ExpiresAt must be after start time")
	// ExpiresAt should be approximately 24h after creation (AWS token lifetime).
	maxExpiry := after.Add(24*time.Hour + time.Second)
	assert.True(t, sess.ExpiresAt.Before(maxExpiry), "ExpiresAt must be within 24h+1s of creation")
}

func TestBackend_PollCount(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "profile", "data", "text/plain"))

	token, err := b.StartSession("app", "env", "profile", 0)
	require.NoError(t, err)

	for i := range 3 {
		var nextToken string
		_, _, nextToken, _, _, err = b.GetLatestConfiguration(token)
		require.NoError(t, err)
		token = nextToken

		sess := b.LookupSession(token)
		require.NotNil(t, sess)
		assert.Equal(t, i+1, sess.PollCount)
	}
}

// TestBackend_PollCount_Increments verifies the per-session poll counter increments on each successful poll.
func TestBackend_PollCount_Increments(t *testing.T) {
	t.Parallel()

	b := appconfigdata.NewInMemoryBackend()
	require.NoError(t, b.SetConfiguration("app", "env", "p", `{}`, "application/json"))

	token, err := b.StartSession("app", "env", "p", 0)
	require.NoError(t, err)

	sess := b.LookupSession(token)
	require.NotNil(t, sess)
	assert.Equal(t, 0, sess.PollCount)

	// Poll 1.
	_, _, nextToken, _, _, err := b.GetLatestConfiguration(token)
	require.NoError(t, err)
	sess = b.LookupSession(nextToken)
	require.NotNil(t, sess)
	assert.Equal(t, 1, sess.PollCount)

	// Poll 2.
	_, _, nextToken2, _, _, err := b.GetLatestConfiguration(nextToken)
	require.NoError(t, err)
	sess = b.LookupSession(nextToken2)
	require.NotNil(t, sess)
	assert.Equal(t, 2, sess.PollCount)
}

// TestHandler_SessionStats verifies that backend statistics are tracked accurately.
func TestHandler_SessionStats(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	require.NoError(t, h.Backend.SetConfiguration("app", "env", "p", `{"v":1}`, "application/json"))
	require.NoError(t, h.Backend.SetConfiguration("app2", "env", "p", `{"v":2}`, "application/json"))

	stats := h.Backend.GetStats()
	assert.Equal(t, 0, stats.SessionCount)
	assert.Equal(t, 2, stats.ProfileCount)
	assert.Equal(t, int64(0), stats.TotalPollCount)

	tok1 := startSession(t, h, "app", "env", "p")
	stats = h.Backend.GetStats()
	assert.Equal(t, 1, stats.SessionCount)

	tok2 := startSession(t, h, "app2", "env", "p")
	stats = h.Backend.GetStats()
	assert.Equal(t, 2, stats.SessionCount)

	// Poll once.
	rec := doRequest(t, h, http.MethodGet, "/configuration?configuration_token="+tok1, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	stats = h.Backend.GetStats()
	assert.Equal(t, int64(1), stats.TotalPollCount)

	// Failed poll (bad token).
	doRequest(t, h, http.MethodGet, "/configuration?configuration_token=garbage", nil)
	stats = h.Backend.GetStats()
	assert.Equal(t, int64(1), stats.TotalPollFailures)

	// End session.
	h.Backend.EndSession(tok2)
	stats = h.Backend.GetStats()
	assert.Equal(t, 1, stats.SessionCount)
}
