package cognitoidp

import (
	"context"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
	"github.com/blackbirdworks/gopherstack/pkgs/worker"
)

const (
	defaultCognitoIDPJanitorInterval = 30 * time.Second
	initialExpiredTokenCapacity      = 32
)

// Janitor is the Cognito IDP background worker that evicts expired refresh tokens.
type Janitor struct {
	Backend     *InMemoryBackend `json:"backend,omitempty"`
	Interval    time.Duration    `json:"interval"`
	TaskTimeout time.Duration    `json:"taskTimeout"`
}

// NewJanitor creates a new Cognito IDP Janitor for the given backend.
// If interval is zero it falls back to defaultCognitoIDPJanitorInterval.
func NewJanitor(backend *InMemoryBackend, interval time.Duration) *Janitor {
	if interval == 0 {
		interval = defaultCognitoIDPJanitorInterval
	}

	return &Janitor{
		Backend:  backend,
		Interval: interval,
	}
}

// Run runs the janitor loop until ctx is cancelled.
func (j *Janitor) Run(ctx context.Context) {
	g := worker.NewGroup(ctx, "cognitoidp")
	g.Ticker("RefreshTokenSweeper", j.Interval, j.TaskTimeout, j.SweepOnce)

	<-ctx.Done()
	g.Stop()
}

// SweepOnce runs a single sweep pass. Exposed for testing.
func (j *Janitor) SweepOnce(ctx context.Context) {
	j.sweepExpiredRefreshTokens(ctx)
	j.Backend.EvictExpiredMFASessions()

	if n := j.Backend.AdvanceUserImportJobStatuses(userImportJobCompletionDelay); n > 0 {
		telemetry.RecordWorkerItems("cognitoidp", "UserImportJobAdvancer", n)
		logger.Load(ctx).InfoContext(ctx, "cognitoidp janitor: advanced user import job statuses", "count", n)
	}

	if n := j.Backend.EvictExpiredAttrVerificationCodes(); n > 0 {
		telemetry.RecordWorkerItems("cognitoidp", "AttrVerificationCodeSweeper", n)
		logger.Load(ctx).InfoContext(ctx, "cognitoidp janitor: evicted attr verification codes", "count", n)
	}
}

// sweepExpiredRefreshTokens removes refresh tokens whose ExpiresAt is in the past.
func (j *Janitor) sweepExpiredRefreshTokens(ctx context.Context) {
	now := time.Now().UTC()
	b := j.Backend

	b.mu.Lock("SweepExpiredRefreshTokens")
	expiredTokens := make([]string, 0, initialExpiredTokenCapacity)
	for token, entry := range b.refreshTokens {
		if !entry.ExpiresAt.IsZero() && !entry.ExpiresAt.After(now) {
			expiredTokens = append(expiredTokens, token)
		}
	}

	for _, token := range expiredTokens {
		b.deleteRefreshTokenLocked(token)
	}
	b.mu.Unlock()

	count := len(expiredTokens)
	telemetry.RecordWorkerItems("cognitoidp", "RefreshTokenSweeper", count)
	telemetry.RecordWorkerTask("cognitoidp", "RefreshTokenSweeper", "success")

	if count > 0 {
		logger.Load(ctx).InfoContext(ctx, "cognitoidp janitor: expired refresh tokens evicted", "count", count)
	}
}
