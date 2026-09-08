package apigatewayv2

import (
	"strings"
	"time"
)

// tokenBucket is a classic token-bucket rate limiter. ratePerSec tokens are added per
// second up to burst; each allowed request consumes one token. Mirrors
// services/apigateway/usage.go's tokenBucket -- the two packages don't share an
// exported type, so this is a deliberate duplication rather than reuse.
type tokenBucket struct {
	last       time.Time
	ratePerSec float64
	burst      float64
	tokens     float64
}

func newTokenBucket(ratePerSec float64, burst int32, now time.Time) *tokenBucket {
	b := float64(burst)
	if b <= 0 {
		// AWS applies a default burst when only a rate is configured.
		b = ratePerSec
	}
	if b < 1 {
		b = 1
	}

	return &tokenBucket{ratePerSec: ratePerSec, burst: b, tokens: b, last: now}
}

// allow refills the bucket based on elapsed time and consumes one token, returning
// whether the request is permitted.
func (tb *tokenBucket) allow(now time.Time) bool {
	if elapsed := now.Sub(tb.last).Seconds(); elapsed > 0 {
		tb.tokens += elapsed * tb.ratePerSec
		if tb.tokens > tb.burst {
			tb.tokens = tb.burst
		}
		tb.last = now
	}

	if tb.tokens >= 1 {
		tb.tokens--

		return true
	}

	return false
}

// routeThrottleKey composes the map key for a stage's per-route token bucket.
func routeThrottleKey(apiID, stageName, routeKey string) string {
	return apiID + "\x00" + stageName + "\x00" + routeKey
}

// routeSettingsFor resolves the throttle settings that apply to routeKey on stage:
// the per-route override in RouteSettings ("Route settings for the stage, by
// routeKey" -- aws-sdk-go-v2 apigatewayv2 types.Stage.RouteSettings doc) when
// present, otherwise the stage's DefaultRouteSettings ("Default route settings
// for the stage" -- types.Stage.DefaultRouteSettings doc). A present override is
// used as-is rather than merged field-by-field with the default, matching this
// repo's apigateway v1 MethodSettings precedent (usage.go's
// stageMethodSettingFor: "[a] specific entry replaces the wildcard entirely
// rather than merging with it").
func routeSettingsFor(stage *Stage, routeKey string) *RouteSettings {
	if rs, ok := stage.RouteSettings[routeKey]; ok {
		return &rs
	}

	return stage.DefaultRouteSettings
}

// EnforceRouteThrottle applies a stage's route-level throttling
// (RouteSettings/DefaultRouteSettings) for a request to routeKey on stage. A
// ThrottlingRateLimit of 0 (or unset) means "not configured" and is never
// enforced: RouteSettings.ThrottlingRateLimit is `omitempty`, so the zero value
// is indistinguishable from absence -- matching this package's apigateway v1
// counterpart (EnforceMethodThrottle's identical "RateLimit > 0" gate). It
// returns ErrThrottled when the limit is exceeded, or nil when unconfigured, the
// stage doesn't exist, or the request is within the configured rate.
func (b *InMemoryBackend) EnforceRouteThrottle(apiID, stageName, routeKey string) error {
	b.mu.Lock("EnforceRouteThrottle")
	defer b.mu.Unlock()

	stage, ok := b.stages.Get(stageKey(apiID, stageName))
	if !ok {
		return nil
	}

	settings := routeSettingsFor(stage, routeKey)
	if settings == nil || settings.ThrottlingRateLimit <= 0 {
		return nil
	}

	now := time.Now()
	mapKey := routeThrottleKey(apiID, stageName, routeKey)

	bucket, exists := b.routeThrottleBuckets[mapKey]
	if !exists {
		bucket = newTokenBucket(settings.ThrottlingRateLimit, settings.ThrottlingBurstLimit, now)
		b.routeThrottleBuckets[mapKey] = bucket
	}

	// Re-apply the currently configured rate/burst on every call so an
	// UpdateStage that changes the limit takes effect immediately, rather than
	// freezing the bucket at whatever settings were in force when it was first
	// created.
	bucket.ratePerSec = settings.ThrottlingRateLimit
	burst := float64(settings.ThrottlingBurstLimit)
	if burst <= 0 {
		burst = settings.ThrottlingRateLimit
	}
	if burst < 1 {
		burst = 1
	}
	bucket.burst = burst
	if bucket.tokens > bucket.burst {
		bucket.tokens = bucket.burst
	}

	if !bucket.allow(now) {
		return ErrThrottled
	}

	return nil
}

// clearStageThrottleBuckets removes every route throttle bucket belonging to
// apiID/stageName, so a deleted stage's buckets don't leak in memory forever
// (the same ghost-row class as an orphaned stage MethodSettings bucket -- see
// apigateway's clearStageThrottleBuckets). Callers must hold b.mu.
func (b *InMemoryBackend) clearStageThrottleBuckets(apiID, stageName string) {
	prefix := apiID + "\x00" + stageName + "\x00"
	for k := range b.routeThrottleBuckets {
		if strings.HasPrefix(k, prefix) {
			delete(b.routeThrottleBuckets, k)
		}
	}
}

// clearRouteThrottleBucket removes the token bucket for one apiID/stageName/routeKey,
// so a deleted or renamed route doesn't leave a stale bucket that a later route
// reusing the same key would inherit. Callers must hold b.mu.
func (b *InMemoryBackend) clearRouteThrottleBucket(apiID, stageName, routeKey string) {
	delete(b.routeThrottleBuckets, routeThrottleKey(apiID, stageName, routeKey))
}
