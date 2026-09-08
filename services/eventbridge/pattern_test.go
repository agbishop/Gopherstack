package eventbridge_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/eventbridge"
)

// matchPatternForTest exposes the internal matchPattern via a test helper.
// We test it through the backend's PutEvents fan-out behavior.
// Direct unit tests use a table-driven approach via the exported TestMatchPattern.

func TestPattern_ExactMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		event   string
		want    bool
	}{
		{
			name:    "source exact match - positive",
			pattern: `{"source": ["my.service"]}`,
			event:   `{"source": "my.service"}`,
			want:    true,
		},
		{
			name:    "source exact match - negative",
			pattern: `{"source": ["my.service"]}`,
			event:   `{"source": "other.service"}`,
			want:    false,
		},
		{
			name:    "source multi-value - match first",
			pattern: `{"source": ["a", "b"]}`,
			event:   `{"source": "a"}`,
			want:    true,
		},
		{
			name:    "source multi-value - match second",
			pattern: `{"source": ["a", "b"]}`,
			event:   `{"source": "b"}`,
			want:    true,
		},
		{
			name:    "source multi-value - no match",
			pattern: `{"source": ["a", "b"]}`,
			event:   `{"source": "c"}`,
			want:    false,
		},
		{
			name:    "multiple fields - both match",
			pattern: `{"source": ["svc"], "detail-type": ["MyEvent"]}`,
			event:   `{"source": "svc", "detail-type": "MyEvent"}`,
			want:    true,
		},
		{
			name:    "multiple fields - one mismatch",
			pattern: `{"source": ["svc"], "detail-type": ["MyEvent"]}`,
			event:   `{"source": "svc", "detail-type": "Other"}`,
			want:    false,
		},
		{
			name:    "nested detail match",
			pattern: `{"detail": {"status": ["ok"]}}`,
			event:   `{"detail": {"status": "ok"}}`,
			want:    true,
		},
		{
			name:    "nested detail mismatch",
			pattern: `{"detail": {"status": ["ok"]}}`,
			event:   `{"detail": {"status": "fail"}}`,
			want:    false,
		},
		{
			name:    "empty pattern matches everything",
			pattern: `{}`,
			event:   `{"source": "anything"}`,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := eventbridge.MatchPatternForTest(tt.pattern, tt.event)
			assert.Equal(t, tt.want, got, "pattern=%s event=%s", tt.pattern, tt.event)
		})
	}
}

func TestPattern_PrefixMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		event   string
		want    bool
	}{
		{
			name:    "prefix match positive",
			pattern: `{"source": [{"prefix": "com.example"}]}`,
			event:   `{"source": "com.example.service"}`,
			want:    true,
		},
		{
			name:    "prefix match negative",
			pattern: `{"source": [{"prefix": "com.example"}]}`,
			event:   `{"source": "org.other.service"}`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := eventbridge.MatchPatternForTest(tt.pattern, tt.event)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPattern_ExistsMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		event   string
		want    bool
	}{
		{
			name:    "exists:true - field present",
			pattern: `{"source": [{"exists": true}]}`,
			event:   `{"source": "svc"}`,
			want:    true,
		},
		{
			name:    "exists:true - field absent",
			pattern: `{"source": [{"exists": true}]}`,
			event:   `{"other": "svc"}`,
			want:    false,
		},
		{
			name:    "exists:false - field absent",
			pattern: `{"source": [{"exists": false}]}`,
			event:   `{"other": "svc"}`,
			want:    true,
		},
		{
			name:    "exists:false - field present",
			pattern: `{"source": [{"exists": false}]}`,
			event:   `{"source": "svc"}`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := eventbridge.MatchPatternForTest(tt.pattern, tt.event)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPattern_NumericMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		event   string
		want    bool
	}{
		{
			name:    "numeric > positive",
			pattern: `{"detail": {"count": [{"numeric": [">", 5]}]}}`,
			event:   `{"detail": {"count": 10}}`,
			want:    true,
		},
		{
			name:    "numeric > negative",
			pattern: `{"detail": {"count": [{"numeric": [">", 5]}]}}`,
			event:   `{"detail": {"count": 3}}`,
			want:    false,
		},
		{
			name:    "numeric range",
			pattern: `{"detail": {"count": [{"numeric": [">=", 1, "<=", 10]}]}}`,
			event:   `{"detail": {"count": 5}}`,
			want:    true,
		},
		{
			name:    "numeric range - outside",
			pattern: `{"detail": {"count": [{"numeric": [">=", 1, "<=", 10]}]}}`,
			event:   `{"detail": {"count": 15}}`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := eventbridge.MatchPatternForTest(tt.pattern, tt.event)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPattern_AnythingButMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		event   string
		want    bool
	}{
		{
			name:    "anything-but list - not in list",
			pattern: `{"source": [{"anything-but": ["bad", "ugly"]}]}`,
			event:   `{"source": "good"}`,
			want:    true,
		},
		{
			name:    "anything-but list - in list",
			pattern: `{"source": [{"anything-but": ["bad", "ugly"]}]}`,
			event:   `{"source": "bad"}`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := eventbridge.MatchPatternForTest(tt.pattern, tt.event)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestPattern_AnythingButNonScalarListElement_RejectedAtCompile pins the
// gopherstack-lrgk fix: {"anything-but": [{"x":1}]} is structurally valid
// JSON but not a form real AWS supports (anything-but lists hold only
// strings or only numbers), so compilePattern -- and by extension PutRule --
// must reject it rather than silently accepting a pattern that later panics
// while matching.
func TestPattern_AnythingButNonScalarListElement_RejectedAtCompile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
	}{
		{
			name:    "object element",
			pattern: `{"foo": [{"anything-but": [{"x":1}]}]}`,
		},
		{
			name:    "array element",
			pattern: `{"foo": [{"anything-but": [["x"]]}]}`,
		},
		{
			name:    "bare object value",
			pattern: `{"foo": [{"anything-but": {"unknown-key": 1}}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := eventbridge.CompilePatternErrorForTest(tt.pattern)
			require.Error(t, err)
		})
	}
}

// TestPattern_AnythingButNonScalarListElement_NoPanic reproduces
// gopherstack-lrgk directly: MatchPatternForTest calls compilePattern then
// matchCompiledPattern, exactly the real matching path used by PutEvents
// delivery. Before the fix, compilePattern accepted this pattern and
// matchCompiledPattern panicked comparing map[string]any with ==; now
// compilePattern rejects it, so matching never runs and the call returns
// false instead of crashing.
func TestPattern_AnythingButNonScalarListElement_NoPanic(t *testing.T) {
	t.Parallel()

	got := eventbridge.MatchPatternForTest(`{"foo": [{"anything-but": [{"x":1}]}]}`, `{"foo": {"x": 1}}`)
	assert.False(t, got)
}

// TestPattern_AnythingBut_DefenseInDepth_NoPanicWhenValidationBypassed proves
// matchAnythingBut's reflect.DeepEqual fix (gopherstack-lrgk) holds on its
// own, independent of validateAnythingButValue: it builds a compiledPattern
// directly (skipping compilePattern's validation) so a non-scalar
// anything-but list element reaches matchCompiledPattern regardless of
// validation, and asserts matching still completes without panicking.
func TestPattern_AnythingBut_DefenseInDepth_NoPanicWhenValidationBypassed(t *testing.T) {
	t.Parallel()

	got := eventbridge.MatchCompiledPatternBypassingValidationForTest(
		`{"foo": [{"anything-but": [{"x":1}]}]}`, `{"foo": {"x": 1}}`,
	)
	assert.False(t, got)
}

// TestPattern_AnythingButNested covers the object form of anything-but, which
// negates a nested matcher (prefix/suffix/wildcard/equals-ignore-case/numeric).
// AWS EventBridge supports these; see the content-filtering documentation.
func TestPattern_AnythingButNested(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		event   string
		want    bool
	}{
		{
			name:    "anything-but prefix - excluded",
			pattern: `{"detail": {"state": [{"anything-but": {"prefix": "init"}}]}}`,
			event:   `{"detail": {"state": "initializing"}}`,
			want:    false,
		},
		{
			name:    "anything-but prefix - allowed",
			pattern: `{"detail": {"state": [{"anything-but": {"prefix": "init"}}]}}`,
			event:   `{"detail": {"state": "running"}}`,
			want:    true,
		},
		{
			name:    "anything-but prefix list - excluded",
			pattern: `{"detail": {"x": [{"anything-but": {"prefix": ["a", "b"]}}]}}`,
			event:   `{"detail": {"x": "apple"}}`,
			want:    false,
		},
		{
			name:    "anything-but suffix - excluded",
			pattern: `{"detail": {"state": [{"anything-but": {"suffix": "ing"}}]}}`,
			event:   `{"detail": {"state": "running"}}`,
			want:    false,
		},
		{
			name:    "anything-but equals-ignore-case - excluded",
			pattern: `{"detail": {"state": [{"anything-but": {"equals-ignore-case": "INIT"}}]}}`,
			event:   `{"detail": {"state": "init"}}`,
			want:    false,
		},
		{
			name:    "anything-but wildcard - excluded",
			pattern: `{"detail": {"state": [{"anything-but": {"wildcard": "*ing"}}]}}`,
			event:   `{"detail": {"state": "running"}}`,
			want:    false,
		},
		{
			name:    "anything-but numeric - excluded",
			pattern: `{"detail": {"n": [{"anything-but": {"numeric": [">", 5]}}]}}`,
			event:   `{"detail": {"n": 10}}`,
			want:    false,
		},
		{
			name:    "anything-but numeric - allowed",
			pattern: `{"detail": {"n": [{"anything-but": {"numeric": [">", 5]}}]}}`,
			event:   `{"detail": {"n": 3}}`,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := eventbridge.MatchPatternForTest(tt.pattern, tt.event)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestPattern_PrefixSuffixIgnoreCase covers the case-insensitive nested form of
// the prefix and suffix matchers, which AWS EventBridge supports via
// {"prefix": {"equals-ignore-case": "..."}}.
func TestPattern_PrefixSuffixIgnoreCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		event   string
		want    bool
	}{
		{
			name:    "prefix ignore-case - match",
			pattern: `{"detail": {"c": [{"prefix": {"equals-ignore-case": "ABC"}}]}}`,
			event:   `{"detail": {"c": "abcdef"}}`,
			want:    true,
		},
		{
			name:    "prefix ignore-case - no match",
			pattern: `{"detail": {"c": [{"prefix": {"equals-ignore-case": "ABC"}}]}}`,
			event:   `{"detail": {"c": "xyzabc"}}`,
			want:    false,
		},
		{
			name:    "suffix ignore-case - match",
			pattern: `{"detail": {"c": [{"suffix": {"equals-ignore-case": "XYZ"}}]}}`,
			event:   `{"detail": {"c": "fooxyz"}}`,
			want:    true,
		},
		{
			name:    "suffix ignore-case - no match",
			pattern: `{"detail": {"c": [{"suffix": {"equals-ignore-case": "XYZ"}}]}}`,
			event:   `{"detail": {"c": "xyzfoo"}}`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := eventbridge.MatchPatternForTest(tt.pattern, tt.event)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPattern_CIDRMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		event   string
		want    bool
	}{
		{
			name:    "CIDR match - IP inside range",
			pattern: `{"detail": {"sourceIPAddress": [{"cidr": "10.0.0.0/24"}]}}`,
			event:   `{"detail": {"sourceIPAddress": "10.0.0.5"}}`,
			want:    true,
		},
		{
			name:    "CIDR match - IP outside range",
			pattern: `{"detail": {"sourceIPAddress": [{"cidr": "10.0.0.0/24"}]}}`,
			event:   `{"detail": {"sourceIPAddress": "10.0.1.1"}}`,
			want:    false,
		},
		{
			name:    "CIDR match - invalid IP",
			pattern: `{"detail": {"sourceIPAddress": [{"cidr": "10.0.0.0/24"}]}}`,
			event:   `{"detail": {"sourceIPAddress": "not-an-ip"}}`,
			want:    false,
		},
		{
			name:    "CIDR match - exact network boundary (lower)",
			pattern: `{"detail": {"ip": [{"cidr": "192.168.1.0/24"}]}}`,
			event:   `{"detail": {"ip": "192.168.1.0"}}`,
			want:    true,
		},
		{
			name:    "CIDR match - exact network boundary (upper)",
			pattern: `{"detail": {"ip": [{"cidr": "192.168.1.0/24"}]}}`,
			event:   `{"detail": {"ip": "192.168.1.255"}}`,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := eventbridge.MatchPatternForTest(tt.pattern, tt.event)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPattern_WildcardMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		event   string
		want    bool
	}{
		{
			name:    "wildcard suffix match - positive",
			pattern: `{"source": [{"wildcard": "com.example.*"}]}`,
			event:   `{"source": "com.example.service"}`,
			want:    true,
		},
		{
			name:    "wildcard suffix match - negative",
			pattern: `{"source": [{"wildcard": "com.example.*"}]}`,
			event:   `{"source": "org.other.service"}`,
			want:    false,
		},
		{
			name:    "wildcard exact match via star",
			pattern: `{"source": [{"wildcard": "*.service"}]}`,
			event:   `{"source": "com.example.service"}`,
			want:    true,
		},
		{
			// '?' has no special meaning in EventBridge wildcard patterns --
			// only '*' is a wildcard meta-character
			// (eb-event-patterns-content-based-filtering.html#eb-filtering-wildcard-matching
			// documents no '?' form). It must match literally.
			name:    "wildcard question mark is literal - positive",
			pattern: `{"source": [{"wildcard": "com.example.?"}]}`,
			event:   `{"source": "com.example.?"}`,
			want:    true,
		},
		{
			name:    "wildcard question mark is literal - does not match arbitrary char",
			pattern: `{"source": [{"wildcard": "com.example.?"}]}`,
			event:   `{"source": "com.example.a"}`,
			want:    false,
		},
		{
			name:    "wildcard star only matches anything",
			pattern: `{"source": [{"wildcard": "*"}]}`,
			event:   `{"source": "anything.at.all"}`,
			want:    true,
		},
		{
			name:    "wildcard empty pattern matches empty string",
			pattern: `{"source": [{"wildcard": ""}]}`,
			event:   `{"source": ""}`,
			want:    true,
		},
		{
			// "EventBridge supports using the backslash character (\) to
			// specify the literal * and \ characters in wildcard filters:
			// The string \* represents the literal * character" (same doc
			// section). An escaped star must not expand.
			name:    "wildcard escaped star is literal - positive",
			pattern: `{"source": [{"wildcard": "value\\*end"}]}`,
			event:   `{"source": "value*end"}`,
			want:    true,
		},
		{
			name:    "wildcard escaped star is literal - does not expand",
			pattern: `{"source": [{"wildcard": "value\\*end"}]}`,
			event:   `{"source": "valueXend"}`,
			want:    false,
		},
		{
			// "The string \\ represents the literal \ character" (same doc
			// section).
			name:    "wildcard escaped backslash is literal - positive",
			pattern: `{"source": [{"wildcard": "a\\\\b"}]}`,
			event:   `{"source": "a\\b"}`,
			want:    true,
		},
		{
			name:    "wildcard escaped backslash is literal - no bare match",
			pattern: `{"source": [{"wildcard": "a\\\\b"}]}`,
			event:   `{"source": "ab"}`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := eventbridge.MatchPatternForTest(tt.pattern, tt.event)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPattern_ArrayEventValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		event   string
		want    bool
	}{
		{
			name:    "event field is array - any element matches",
			pattern: `{"resources": ["arn:aws:s3:::bucket1"]}`,
			event:   `{"resources": ["arn:aws:s3:::bucket1", "arn:aws:s3:::bucket2"]}`,
			want:    true,
		},
		{
			name:    "event field is array - no element matches",
			pattern: `{"resources": ["arn:aws:s3:::bucket3"]}`,
			event:   `{"resources": ["arn:aws:s3:::bucket1", "arn:aws:s3:::bucket2"]}`,
			want:    false,
		},
		{
			name:    "event array with prefix matcher",
			pattern: `{"resources": [{"prefix": "arn:aws:s3:::"}]}`,
			event:   `{"resources": ["arn:aws:s3:::bucket1", "arn:aws:s3:::bucket2"]}`,
			want:    true,
		},
		{
			name:    "event array with exists matcher",
			pattern: `{"resources": [{"exists": true}]}`,
			event:   `{"resources": ["arn:aws:s3:::bucket1"]}`,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := eventbridge.MatchPatternForTest(tt.pattern, tt.event)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPatternMatching_PrefixSuffixAnythingBut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		pattern       string
		event         eventbridge.EventEntry
		wantDelivered bool
	}{
		{
			name:          "prefix_match_delivers",
			pattern:       `{"source": [{"prefix": "aws."}]}`,
			event:         eventbridge.EventEntry{Source: "aws.s3", DetailType: "ObjectCreated", Detail: `{}`},
			wantDelivered: true,
		},
		{
			name:          "prefix_no_match_skips",
			pattern:       `{"source": [{"prefix": "aws."}]}`,
			event:         eventbridge.EventEntry{Source: "custom.service", DetailType: "Evt", Detail: `{}`},
			wantDelivered: false,
		},
		{
			name:          "suffix_match_delivers",
			pattern:       `{"source": [{"suffix": ".events"}]}`,
			event:         eventbridge.EventEntry{Source: "aws.events", DetailType: "Scheduled Event", Detail: `{}`},
			wantDelivered: true,
		},
		{
			name:          "anything_but_match_delivers",
			pattern:       `{"source": [{"anything-but": ["skip.me"]}]}`,
			event:         eventbridge.EventEntry{Source: "other.src", DetailType: "Evt", Detail: `{}`},
			wantDelivered: true,
		},
		{
			name:          "anything_but_excluded_skips",
			pattern:       `{"source": [{"anything-but": ["skip.me"]}]}`,
			event:         eventbridge.EventEntry{Source: "skip.me", DetailType: "Evt", Detail: `{}`},
			wantDelivered: false,
		},
		{
			name:          "exists_true_delivers_when_field_present",
			pattern:       `{"detail": {"code": [{"exists": true}]}}`,
			event:         eventbridge.EventEntry{Source: "svc", DetailType: "Evt", Detail: `{"code": "200"}`},
			wantDelivered: true,
		},
		{
			name:          "exists_true_skips_when_field_absent",
			pattern:       `{"detail": {"code": [{"exists": true}]}}`,
			event:         eventbridge.EventEntry{Source: "svc", DetailType: "Evt", Detail: `{}`},
			wantDelivered: false,
		},
		{
			name:          "numeric_gt_delivers",
			pattern:       `{"detail": {"count": [{"numeric": [">", 5]}]}}`,
			event:         eventbridge.EventEntry{Source: "svc", DetailType: "Evt", Detail: `{"count": 10}`},
			wantDelivered: true,
		},
		{
			name:          "numeric_gt_skips_below",
			pattern:       `{"detail": {"count": [{"numeric": [">", 5]}]}}`,
			event:         eventbridge.EventEntry{Source: "svc", DetailType: "Evt", Detail: `{"count": 3}`},
			wantDelivered: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sqsMock := newMockSQSSender()
			b := setupDeliveryBackend(t, sqsMock, newMockLambdaInvoker())
			const (
				queueARN = "arn:aws:sqs:us-east-1:123456789012:pattern-queue"
				ruleName = "pattern-rule"
			)

			_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
				Name:         ruleName,
				EventPattern: tc.pattern,
				State:        "ENABLED",
			})
			require.NoError(t, err)

			_, err = b.PutTargets(context.Background(), ruleName, "default", []eventbridge.Target{
				{ID: "t1", Arn: queueARN},
			})
			require.NoError(t, err)

			b.PutEvents(context.Background(), []eventbridge.EventEntry{tc.event})

			// PutEvents delivers asynchronously; give it a moment.
			require.Eventually(t, func() bool {
				msgs := sqsMock.MessagesFor(queueARN)

				return tc.wantDelivered == (len(msgs) > 0)
			}, 2*time.Second, 20*time.Millisecond)
		})
	}
}
