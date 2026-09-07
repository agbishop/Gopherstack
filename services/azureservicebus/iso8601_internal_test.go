// Whitebox: parseISO8601Duration/formatISO8601Duration are unexported and
// have no other exported seam to exercise them through.
package azureservicebus

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseISO8601Duration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr bool
	}{
		{name: "minutes", in: "PT1M", want: time.Minute},
		{name: "seconds", in: "PT30S", want: 30 * time.Second},
		{name: "days", in: "P14D", want: 14 * 24 * time.Hour},
		{name: "hours minutes seconds", in: "PT5M30S", want: 5*time.Minute + 30*time.Second},
		{
			name: "days and time", in: "P1DT2H3M4S",
			want: 24*time.Hour + 2*time.Hour + 3*time.Minute + 4*time.Second,
		},
		{
			name: "fractional seconds", in: "PT1.5S",
			want: time.Second + 500*time.Millisecond,
		},
		{name: "empty string is invalid", in: "", wantErr: true},
		{name: "missing P prefix is invalid", in: "1D", wantErr: true},
		{name: "garbage is invalid", in: "not-a-duration", wantErr: true},
		{name: "bare P with nothing after it is invalid", in: "P", wantErr: true},
		{name: "bare PT with nothing after it is invalid", in: "PT", wantErr: true},
		{
			name: "huge sentinel value clamps to max time.Duration",
			in:   "P10675199DT2H48M5.4775807S",
			want: math.MaxInt64,
		},
		{
			name: "value beyond the sentinel also clamps",
			in:   "P99999999DT0H0M0S",
			want: math.MaxInt64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseISO8601Duration(tt.in)

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidISO8601Duration)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatISO8601Duration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
		in   time.Duration
	}{
		{name: "zero", in: 0, want: zeroISO8601Duration},
		{name: "negative clamps to zero form", in: -time.Second, want: zeroISO8601Duration},
		{name: "minute", in: time.Minute, want: "PT1M"},
		{name: "seconds", in: 30 * time.Second, want: "PT30S"},
		{name: "days", in: 14 * 24 * time.Hour, want: "P14D"},
		{
			name: "days and time",
			in:   24*time.Hour + 2*time.Hour + 3*time.Minute + 4*time.Second,
			want: "P1DT2H3M4S",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, formatISO8601Duration(tt.in))
		})
	}
}

// TestISO8601Duration_RoundTrip covers a scenario that doesn't fit either
// table above: it asserts a structural property (parse(format(d)) == d)
// across a set of representative durations, rather than a single
// input/output pair.
func TestISO8601Duration_RoundTrip(t *testing.T) {
	t.Parallel()

	durations := []time.Duration{
		time.Second, 30 * time.Second, time.Minute, 5 * time.Minute,
		time.Hour, 24 * time.Hour, 14 * 24 * time.Hour,
		time.Hour + 2*time.Minute + 3*time.Second,
	}

	for _, d := range durations {
		formatted := formatISO8601Duration(d)

		parsed, err := parseISO8601Duration(formatted)
		require.NoError(t, err, "formatted value %q for %s should parse back", formatted, d)
		assert.Equal(t, d, parsed, "round-trip mismatch for %s (formatted as %q)", d, formatted)
	}
}
