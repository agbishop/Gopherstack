package sts_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sts"
)

// TestJanitor_SweepsExpiredSessions verifies that the janitor removes sessions
// whose Expiration is in the past.
func TestJanitor_SweepsExpiredSessions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		totalSessions  int
		expireCount    int
		wantCountAfter int
	}{
		{
			name:           "one_of_two_sessions_expired",
			totalSessions:  2,
			expireCount:    1,
			wantCountAfter: 1,
		},
		{
			name:           "all_sessions_expired",
			totalSessions:  3,
			expireCount:    3,
			wantCountAfter: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()

			accessKeyIDs := make([]string, tt.totalSessions)

			for i := range tt.totalSessions {
				resp, err := b.AssumeRole(&sts.AssumeRoleInput{
					RoleArn:         "arn:aws:iam::123456789012:role/Role1",
					RoleSessionName: fmt.Sprintf("session%d", i),
					DurationSeconds: 900,
				})
				require.NoError(t, err)

				accessKeyIDs[i] = resp.AssumeRoleResult.Credentials.AccessKeyID
			}

			require.Equal(t, tt.totalSessions, b.SessionCount())

			// Force the first tt.expireCount sessions into the past.
			for i := range tt.expireCount {
				b.SetSessionExpiration(accessKeyIDs[i], time.Now().Add(-1*time.Second))
			}

			// Run the janitor with a very short interval.
			j := sts.NewJanitor(b, 10*time.Millisecond)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			go j.Run(ctx)

			// Wait until the expired sessions are gone.
			require.Eventually(t, func() bool {
				return b.SessionCount() == tt.wantCountAfter
			}, 2*time.Second, 20*time.Millisecond)
		})
	}
}

// TestJanitor_SweepOnce_STS verifies that SweepOnce triggers an immediate sweep
// of expired sessions without requiring a ticker to fire.
func TestJanitor_SweepOnce_STS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		expiredCount   int
		activeCount    int
		wantAfterSweep int
	}{
		{
			name:           "expired_sessions_removed",
			expiredCount:   2,
			activeCount:    1,
			wantAfterSweep: 1,
		},
		{
			name:           "no_expired_sessions",
			expiredCount:   0,
			activeCount:    2,
			wantAfterSweep: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()

			totalSessions := tt.expiredCount + tt.activeCount
			accessKeyIDs := make([]string, totalSessions)

			for i := range totalSessions {
				resp, err := b.AssumeRole(&sts.AssumeRoleInput{
					RoleArn:         "arn:aws:iam::123456789012:role/Role1",
					RoleSessionName: fmt.Sprintf("session%d", i),
					DurationSeconds: 900,
				})
				require.NoError(t, err)

				accessKeyIDs[i] = resp.AssumeRoleResult.Credentials.AccessKeyID
			}

			// Force the first tt.expiredCount sessions to be expired.
			for i := range tt.expiredCount {
				b.SetSessionExpiration(accessKeyIDs[i], time.Now().Add(-time.Second))
			}

			j := sts.NewJanitor(b, time.Minute)
			j.SweepOnce(t.Context())

			assert.Equal(t, tt.wantAfterSweep, b.SessionCount())
		})
	}
}

func TestHandler_SessionMetrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		expiredCount      int
		activeCount       int
		wantActive        int
		wantExpired       int
		wantExpiredSwept  int64
		wantSweepCount    int64
		runImmediateSweep bool
	}{
		{
			name:              "reports_active_and_expired_before_sweep",
			expiredCount:      1,
			activeCount:       2,
			wantActive:        2,
			wantExpired:       1,
			wantExpiredSwept:  0,
			wantSweepCount:    0,
			runImmediateSweep: false,
		},
		{
			name:              "reports_sweep_counters_after_sweep",
			expiredCount:      2,
			activeCount:       1,
			wantActive:        1,
			wantExpired:       0,
			wantExpiredSwept:  2,
			wantSweepCount:    1,
			runImmediateSweep: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := sts.NewInMemoryBackend()
			handler := sts.NewHandler(backend).WithJanitor(time.Minute)

			totalSessions := tt.expiredCount + tt.activeCount
			accessKeyIDs := make([]string, totalSessions)

			for i := range totalSessions {
				resp, err := backend.AssumeRole(&sts.AssumeRoleInput{
					RoleArn:         "arn:aws:iam::123456789012:role/Role1",
					RoleSessionName: fmt.Sprintf("metrics-session-%d", i),
					DurationSeconds: 900,
				})
				require.NoError(t, err)
				accessKeyIDs[i] = resp.AssumeRoleResult.Credentials.AccessKeyID
			}

			for i := range tt.expiredCount {
				backend.SetSessionExpiration(accessKeyIDs[i], time.Now().Add(-time.Second))
			}

			if tt.runImmediateSweep {
				handler.GetJanitor().SweepOnce(t.Context())
			}

			metrics := handler.SessionMetrics()
			assert.Equal(t, tt.wantActive, metrics.ActiveSessions)
			assert.Equal(t, tt.wantExpired, metrics.ExpiredSessions)
			assert.Equal(t, tt.wantSweepCount, metrics.SweepCount)
			assert.Equal(t, tt.wantExpiredSwept, metrics.ExpiredEvictions)
		})
	}
}

// TestJanitor_TaskTimeout_STS verifies that WithJanitor propagates the
// taskTimeout variadic argument correctly for the STS handler.
func TestJanitor_TaskTimeout_STS(t *testing.T) {
	t.Parallel()

	b := sts.NewInMemoryBackend()
	h := sts.NewHandler(b)

	// Passing a 30s task timeout should not cause any issue.
	h.WithJanitor(10*time.Millisecond, 30*time.Second)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // immediately cancelled — goroutine should exit right away

	err := h.StartWorker(ctx)
	require.NoError(t, err)
}

// not removed by the janitor.
func TestJanitor_PreservesActiveSessions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		sessionCount   int
		wantCountAfter int
	}{
		{name: "two_active_sessions", sessionCount: 2, wantCountAfter: 2},
		{name: "one_active_session", sessionCount: 1, wantCountAfter: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()

			for i := range tt.sessionCount {
				_, err := b.AssumeRole(&sts.AssumeRoleInput{
					RoleArn:         "arn:aws:iam::123456789012:role/Role1",
					RoleSessionName: fmt.Sprintf("session%d", i),
					DurationSeconds: 900,
				})
				require.NoError(t, err)
			}

			j := sts.NewJanitor(b, 10*time.Millisecond)
			ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
			defer cancel()

			go j.Run(ctx)
			<-ctx.Done()

			// All sessions still present since none are expired.
			assert.Equal(t, tt.wantCountAfter, b.SessionCount())
		})
	}
}

// TestGetCallerIdentity_ExpiredSession_ReturnsExpiredToken verifies that an expired
// ASIA session returns ExpiredTokenException rather than falling back to the root identity.
// This matches AWS behaviour where expired temporary credentials are explicitly rejected.
func TestGetCallerIdentity_ExpiredSession_ReturnsExpiredToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		expiredAgo time.Duration
	}{
		{
			name:       "expired_one_second_ago",
			expiredAgo: time.Second,
		},
		{
			name:       "expired_one_hour_ago",
			expiredAgo: time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()

			resp, err := b.AssumeRole(&sts.AssumeRoleInput{
				RoleArn:         "arn:aws:iam::123456789012:role/MyRole",
				RoleSessionName: "my-session",
				DurationSeconds: 900,
			})
			require.NoError(t, err)

			creds := resp.AssumeRoleResult.Credentials

			// Verify the session is valid before expiry.
			ciResp, err := b.GetCallerIdentity(creds.AccessKeyID, creds.SessionToken)
			require.NoError(t, err)
			assert.Contains(t, ciResp.GetCallerIdentityResult.Arn, "assumed-role")

			// Force the session to be expired.
			b.SetSessionExpiration(creds.AccessKeyID, time.Now().Add(-tt.expiredAgo))

			// After expiry, GetCallerIdentity must return ExpiredTokenException, not root identity.
			_, err = b.GetCallerIdentity(creds.AccessKeyID, creds.SessionToken)
			require.ErrorIs(t, err, sts.ErrSessionExpired)
		})
	}
}

// TestHandler_StartWorker verifies StartWorker behaviour with and without a janitor.
func TestHandler_StartWorker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		withJanitor bool
	}{
		{name: "with_janitor", withJanitor: true},
		{name: "without_janitor", withJanitor: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sts.NewInMemoryBackend()
			h := sts.NewHandler(b)

			if tt.withJanitor {
				h.WithJanitor(10 * time.Millisecond)
			}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			err := h.StartWorker(ctx)
			require.NoError(t, err)
		})
	}
}

// TestSTSJanitor_DefaultInterval verifies that a zero interval in WithJanitor
// results in the default interval being used.
func TestSTSJanitor_DefaultInterval(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval time.Duration
		want     time.Duration
	}{
		{
			name:     "zero_uses_default",
			interval: 0,
			want:     sts.DefaultJanitorInterval,
		},
		{
			name:     "custom_interval_propagated",
			interval: 5 * time.Minute,
			want:     5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := sts.NewHandler(sts.NewInMemoryBackend())
			h.WithJanitor(tt.interval)

			assert.Equal(t, tt.want, h.GetJanitorInterval())
		})
	}
}
