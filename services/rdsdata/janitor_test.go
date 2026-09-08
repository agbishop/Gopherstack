package rdsdata_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/rdsdata"
)

// TestJanitor_SweepExpiredTransactions verifies the Janitor reaps a
// transaction idle past its timeout or open past its max lifetime -- the
// automatic expiry BeginTransaction documents (rdsdata@v1.35.4
// api_op_BeginTransaction.go) -- and leaves everything else alone.
func TestJanitor_SweepExpiredTransactions(t *testing.T) {
	t.Parallel()

	const (
		idleTimeout = 3 * time.Minute
		maxLifetime = 24 * time.Hour
	)

	tests := []struct {
		name        string
		createdAgo  time.Duration
		activityAgo time.Duration
		wantSwept   bool
	}{
		{name: "fresh_transaction_kept", createdAgo: time.Second, activityAgo: time.Second, wantSwept: false},
		{name: "idle_past_timeout_swept", createdAgo: 4 * time.Minute, activityAgo: 4 * time.Minute, wantSwept: true},
		{name: "idle_within_timeout_kept", createdAgo: 2 * time.Minute, activityAgo: 2 * time.Minute, wantSwept: false},
		{
			name:       "past_max_lifetime_swept_even_if_active",
			createdAgo: 25 * time.Hour, activityAgo: time.Second, wantSwept: true,
		},
		{name: "within_max_lifetime_kept", createdAgo: 23 * time.Hour, activityAgo: time.Second, wantSwept: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := rdsdata.NewInMemoryBackend("000000000000", "us-east-1")

			txID, err := b.BeginTransaction(t.Context(), "arn:aws:rds:us-east-1:000000000000:cluster:janitor")
			require.NoError(t, err)

			now := time.Now()
			rdsdata.SetTransactionTimes(b, txID, now.Add(-tt.createdAgo), now.Add(-tt.activityAgo))

			janitor := rdsdata.NewJanitor(b, time.Minute, idleTimeout, maxLifetime)
			janitor.SweepOnce(t.Context())

			txns := b.ListTransactions(t.Context())
			hasEngineTx := rdsdata.EngineHasTx(b)

			if tt.wantSwept {
				assert.NotContains(t, txns, txID, "expired transaction must be evicted")
				assert.False(t, hasEngineTx(txID), "expired transaction's engine tx must be rolled back")
			} else {
				assert.Contains(t, txns, txID, "non-expired transaction must remain")
				assert.True(t, hasEngineTx(txID), "non-expired transaction's engine tx must remain open")
			}
		})
	}
}

// TestJanitor_ExecuteStatement_ResetsIdleClock verifies a statement that
// references a transaction id refreshes LastActivityAt, so a transaction
// still in active use survives a sweep even though it was last touched long
// ago at BeginTransaction time.
func TestJanitor_ExecuteStatement_ResetsIdleClock(t *testing.T) {
	t.Parallel()

	b := rdsdata.NewInMemoryBackend("000000000000", "us-east-1")

	arn := "arn:aws:rds:us-east-1:000000000000:cluster:janitor-touch"

	txID, err := b.BeginTransaction(t.Context(), arn)
	require.NoError(t, err)

	stale := time.Now().Add(-5 * time.Minute)
	rdsdata.SetTransactionTimes(b, txID, stale, stale)

	_, _, _, _, err = b.ExecuteStatement(t.Context(), arn, "SELECT 1", txID)
	require.NoError(t, err)

	janitor := rdsdata.NewJanitor(b, time.Minute, 3*time.Minute, 24*time.Hour)
	janitor.SweepOnce(t.Context())

	txns := b.ListTransactions(t.Context())
	assert.Contains(t, txns, txID, "ExecuteStatement must have refreshed the idle clock")
}

// TestJanitor_NewJanitor_Defaults verifies zero-value interval/timeouts fall
// back to the AWS-documented defaults rather than a busy-looping or
// never-firing janitor.
func TestJanitor_NewJanitor_Defaults(t *testing.T) {
	t.Parallel()

	b := rdsdata.NewInMemoryBackend("000000000000", "us-east-1")
	janitor := rdsdata.NewJanitor(b, 0, 0, 0)

	assert.Equal(t, time.Minute, janitor.Interval)
	assert.Equal(t, 3*time.Minute, janitor.IdleTimeout)
	assert.Equal(t, 24*time.Hour, janitor.MaxLifetime)
}
