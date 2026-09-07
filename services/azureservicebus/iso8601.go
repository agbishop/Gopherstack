package azureservicebus

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidISO8601Duration is returned by parseISO8601Duration for a string
// that doesn't match the supported PnDTnHnMnS/PTnHnMnS subset.
var ErrInvalidISO8601Duration = errors.New("azureservicebus: invalid ISO 8601 duration")

// Seconds-per-unit conversion factors for the D/H/M components of an ISO
// 8601 duration.
const (
	secondsPerDay    = 24 * 60 * 60
	secondsPerHour   = 60 * 60
	secondsPerMinute = 60
)

// iso8601DurationPattern matches the PnDTnHnMnS / PTnHnMnS subset of ISO
// 8601 durations that real Service Bus's LockDuration/MaxDeliveryCount/
// DefaultMessageTimeToLive XML elements use (e.g. "PT1M", "PT30S", "P14D",
// "PT5M30.5S"). Weeks (PnW), months, and years are deliberately not
// supported -- Service Bus never emits or expects them for these
// properties.
var iso8601DurationPattern = regexp.MustCompile(
	`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?)?$`,
)

// parseISO8601Duration parses the PnDTnHnMnS/PTnHnMnS subset of ISO 8601
// durations used by Service Bus's LockDuration and DefaultMessageTimeToLive
// XML properties (MaxDeliveryCount, unlike those two, is a plain integer,
// not a duration, and does not go through this parser). Fractional seconds
// are supported. A value that would overflow
// time.Duration's int64-nanosecond range is clamped to the maximum
// representable time.Duration (~292 years) rather than erroring -- real
// Service Bus's own "infinite" sentinel for these fields,
// "P10675199DT2H48M5.4775807S", is itself just .NET's TimeSpan.MaxValue
// (which is exactly math.MaxInt64 100-nanosecond ticks, i.e. very close to
// Go's own int64-nanosecond time.Duration ceiling) spelled out in ISO 8601,
// so this clamp is a deliberate, documented approximation of that sentinel
// rather than a real precision loss for any duration Service Bus itself
// would ever produce.
func parseISO8601Duration(s string) (time.Duration, error) {
	m := iso8601DurationPattern.FindStringSubmatch(s)
	if m == nil || (m[1] == "" && m[2] == "" && m[3] == "" && m[4] == "") {
		return 0, fmt.Errorf("%w: %q", ErrInvalidISO8601Duration, s)
	}

	var totalSeconds float64

	for i, unitSeconds := range [...]float64{secondsPerDay, secondsPerHour, secondsPerMinute, 1} {
		if m[i+1] == "" {
			continue
		}

		v, err := strconv.ParseFloat(m[i+1], 64)
		if err != nil {
			return 0, fmt.Errorf("%w: %q: %w", ErrInvalidISO8601Duration, s, err)
		}

		totalSeconds += v * unitSeconds
	}

	nanos := totalSeconds * float64(time.Second)
	// float64(math.MaxInt64) rounds UP to exactly 2^63 (one more than the
	// actual int64 maximum, 2^63-1, which has no exact float64
	// representation) -- so this comparison must be >=, not >. With a plain
	// >, nanos == 2^63 would fail the clamp check and fall through to
	// time.Duration(nanos), converting an out-of-range float64 to int64:
	// implementation-defined behavior that yields a negative duration on
	// amd64/arm64, silently corrupting LockDuration/DefaultMessageTimeToLive
	// into something negative instead of clamping to the documented maximum.
	if nanos >= float64(math.MaxInt64) {
		return time.Duration(math.MaxInt64), nil
	}

	return time.Duration(nanos), nil
}

// zeroISO8601Duration is the shortest valid ISO 8601 representation of "no
// duration", used for a non-positive input to formatISO8601Duration.
const zeroISO8601Duration = "PT0S"

// formatISO8601Duration is parseISO8601Duration's inverse, used to emit
// LockDuration/DefaultMessageTimeToLive on Get/List responses (see atom.go).
// A non-positive duration formats as zeroISO8601Duration.
func formatISO8601Duration(d time.Duration) string {
	if d <= 0 {
		return zeroISO8601Duration
	}

	totalWhole := int64(d / time.Second)
	fracNanos := int64(d % time.Second)

	days := totalWhole / secondsPerDay
	rem := totalWhole % secondsPerDay
	hours := rem / secondsPerHour
	rem %= secondsPerHour
	minutes := rem / secondsPerMinute
	seconds := rem % secondsPerMinute

	var b strings.Builder

	b.WriteString("P")

	if days > 0 {
		fmt.Fprintf(&b, "%dD", days)
	}

	if hours == 0 && minutes == 0 && seconds == 0 && fracNanos == 0 {
		return b.String()
	}

	b.WriteString("T")

	if hours > 0 {
		fmt.Fprintf(&b, "%dH", hours)
	}

	if minutes > 0 {
		fmt.Fprintf(&b, "%dM", minutes)
	}

	switch {
	case fracNanos > 0:
		secondsFloat := float64(seconds) + float64(fracNanos)/float64(time.Second)
		fmt.Fprintf(&b, "%sS", strconv.FormatFloat(secondsFloat, 'f', -1, 64))
	case seconds > 0:
		fmt.Fprintf(&b, "%dS", seconds)
	}

	return b.String()
}
