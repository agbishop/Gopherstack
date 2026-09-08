package eventbridge_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/eventbridge"
)

func TestPutRule_EnforcesPerBusLimit(t *testing.T) {
	t.Parallel()
	b := newBackend()

	const limit = 300
	for i := range limit {
		_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
			Name:         fmt.Sprintf("rule-%d", i),
			EventPattern: `{"source":["x"]}`,
			State:        "ENABLED",
		})
		require.NoError(t, err, "rule %d should be created", i)
	}

	_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         "rule-overflow",
		EventPattern: `{"source":["x"]}`,
		State:        "ENABLED",
	})
	require.ErrorIs(t, err, eventbridge.ErrResourceLimitExceeded)
}

func TestPutRule_UpdateExistingDoesNotCountAgainstLimit(t *testing.T) {
	t.Parallel()
	b := newBackend()

	_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         "my-rule",
		EventPattern: `{"source":["x"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	for range 5 {
		_, err = b.PutRule(context.Background(), eventbridge.PutRuleInput{
			Name:         "my-rule",
			EventPattern: `{"source":["y"]}`,
			State:        "ENABLED",
		})
		require.NoError(t, err)
	}
}

func TestPutRule_ValidatesStateEnum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		state   string
		wantErr bool
	}{
		{name: "enabled accepted", state: "ENABLED"},
		{name: "disabled accepted", state: "DISABLED"},
		{name: "cloudtrail managed state accepted", state: "ENABLED_WITH_ALL_CLOUDTRAIL_MANAGEMENT_EVENTS"},
		{name: "omitted defaults to enabled", state: ""},
		{name: "unrecognized value rejected", state: "BOGUS", wantErr: true},
		{name: "lowercase enabled rejected", state: "enabled", wantErr: true},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend()

			rule, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
				Name:         fmt.Sprintf("state-rule-%d", i),
				EventPattern: `{"source":["x"]}`,
				State:        tt.state,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, eventbridge.ErrInvalidParameter)

				return
			}

			require.NoError(t, err)

			wantState := tt.state
			if wantState == "" {
				wantState = "ENABLED"
			}
			assert.Equal(t, wantState, rule.State)
		})
	}
}

func TestPutRule_InvalidScheduleExpressionRejected(t *testing.T) {
	t.Parallel()

	invalid := []struct {
		name string
		expr string
	}{
		{"unknown prefix", "invalid(expression)"},
		{"rate zero", "rate(0 minutes)"},
		{"rate negative", "rate(-1 hours)"},
		{"rate non-number", "rate(abc minutes)"},
		{"cron too few fields", "cron(* * *)"},
		{"bare string", "foo"},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend()
			_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
				Name:               "r",
				ScheduleExpression: tt.expr,
			})
			require.ErrorIs(t, err, eventbridge.ErrInvalidParameter)
		})
	}
}

func TestPutRule_ValidScheduleExpressionsAccepted(t *testing.T) {
	t.Parallel()

	valid := []string{
		"rate(1 minute)",
		"rate(5 minutes)",
		"rate(1 hour)",
		"rate(24 hours)",
		"rate(1 day)",
		"cron(0 12 * * ? *)",
	}

	for _, expr := range valid {
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			b := newBackend()
			_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
				Name:               "r",
				ScheduleExpression: expr,
			})
			require.NoError(t, err)
		})
	}
}

func TestPutRule_ManagedByPreserved(t *testing.T) {
	t.Parallel()
	b := newBackend()

	// PutRuleInput.ManagedBy has no JSON tag (json:"-") because real AWS's
	// PutRuleInput has no such wire member at all -- it can only be set via a
	// direct in-process Go call (an internal same-process AWS-service
	// integration seeding a managed rule), never via the public PutRule API.
	// This whitebox test exercises exactly that internal seeding path.
	_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         "managed-rule",
		EventPattern: `{"source":["x"]}`,
		ManagedBy:    "scheduler.amazonaws.com",
	})
	require.NoError(t, err)

	rule, err := b.DescribeRule(context.Background(), "managed-rule", "")
	require.NoError(t, err)
	assert.Equal(t, "scheduler.amazonaws.com", rule.ManagedBy)
}

func TestPutRule_ManagedByNotWireSettable(t *testing.T) {
	t.Parallel()
	e := echo.New()
	b := newBackend()
	h := eventbridge.NewHandler(b)

	// A real (or malicious) client sending "ManagedBy" over the wire must not
	// be able to mark its own rule as AWS-service-managed -- real AWS's
	// PutRule has no such request member.
	rec := auditMakeRequest(t, h, e, "PutRule", map[string]any{
		"Name":         "forged-managed-rule",
		"EventPattern": `{"source":["x"]}`,
		"ManagedBy":    "scheduler.amazonaws.com",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	rule, err := b.DescribeRule(context.Background(), "forged-managed-rule", "")
	require.NoError(t, err)
	assert.Empty(t, rule.ManagedBy, "ManagedBy must not be settable via the public PutRule wire API")
}

// TestManagedRuleException_RejectsRuleLevelMutations verifies that once a
// rule is service-managed (Rule.ManagedBy non-empty -- only reachable via the
// internal ManagedBy seeding path proven above), every rule-level mutating op
// rejects it with ManagedRuleException, matching real AWS.
func TestManagedRuleException_RejectsRuleLevelMutations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		op   func(t *testing.T, b *eventbridge.InMemoryBackend) error
		name string
	}{
		{
			name: "PutRule on an existing managed rule",
			op: func(_ *testing.T, b *eventbridge.InMemoryBackend) error {
				_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
					Name:         "managed-rule",
					EventPattern: `{"source":["y"]}`,
				})

				return err
			},
		},
		{
			name: "DeleteRule",
			op: func(_ *testing.T, b *eventbridge.InMemoryBackend) error {
				return b.DeleteRule(context.Background(), "managed-rule", "")
			},
		},
		{
			name: "EnableRule",
			op: func(_ *testing.T, b *eventbridge.InMemoryBackend) error {
				return b.EnableRule(context.Background(), "managed-rule", "")
			},
		},
		{
			name: "DisableRule",
			op: func(_ *testing.T, b *eventbridge.InMemoryBackend) error {
				return b.DisableRule(context.Background(), "managed-rule", "")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend()
			_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
				Name:         "managed-rule",
				EventPattern: `{"source":["x"]}`,
				ManagedBy:    "scheduler.amazonaws.com",
			})
			require.NoError(t, err)

			err = tt.op(t, b)
			require.ErrorIs(t, err, eventbridge.ErrManagedRule)

			// The rule itself must be untouched by the rejected mutation.
			rule, descErr := b.DescribeRule(context.Background(), "managed-rule", "")
			require.NoError(t, descErr)
			assert.Equal(t, "scheduler.amazonaws.com", rule.ManagedBy)
		})
	}
}

func TestTestEventPattern_Match(t *testing.T) {
	t.Parallel()
	b := newBackend()

	matched, err := b.TestEventPattern(
		context.Background(),
		`{"source":["myapp"],"detail-type":["OrderPlaced"]}`,
		`{"source":"myapp","detail-type":"OrderPlaced","detail":{}}`,
	)
	require.NoError(t, err)
	assert.True(t, matched)
}

func TestTestEventPattern_NoMatch(t *testing.T) {
	t.Parallel()
	b := newBackend()

	matched, err := b.TestEventPattern(
		context.Background(),
		`{"source":["myapp"]}`,
		`{"source":"other","detail-type":"T","detail":{}}`,
	)
	require.NoError(t, err)
	assert.False(t, matched)
}

func TestTestEventPattern_InvalidPattern(t *testing.T) {
	t.Parallel()
	b := newBackend()

	_, err := b.TestEventPattern(context.Background(), "not-json", `{"source":"x"}`)
	require.ErrorIs(t, err, eventbridge.ErrInvalidParameter)
}

func TestTestEventPattern_InvalidEvent(t *testing.T) {
	t.Parallel()
	b := newBackend()

	_, err := b.TestEventPattern(context.Background(), `{"source":["x"]}`, "not-json")
	require.ErrorIs(t, err, eventbridge.ErrInvalidParameter)
}

func TestListRuleNamesByTarget_FiltersToMatchingRules(t *testing.T) {
	t.Parallel()
	b := newBackend()

	targetARN := "arn:aws:lambda:us-east-1:123456789012:function:shared-fn"

	for _, ruleName := range []string{"rule-a", "rule-b", "rule-c"} {
		_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
			Name:         ruleName,
			EventPattern: `{"source":["x"]}`,
		})
		require.NoError(t, err)
	}

	_, err := b.PutTargets(context.Background(), "rule-a", "", []eventbridge.Target{{ID: "t1", Arn: targetARN}})
	require.NoError(t, err)
	_, err = b.PutTargets(context.Background(), "rule-c", "", []eventbridge.Target{{ID: "t1", Arn: targetARN}})
	require.NoError(t, err)
	_, err = b.PutTargets(context.Background(), "rule-b", "", []eventbridge.Target{
		{ID: "t1", Arn: "arn:aws:sqs:us-east-1:123456789012:other-q"},
	})
	require.NoError(t, err)

	names, _, err := b.ListRuleNamesByTarget(context.Background(), targetARN, "", "", 0)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"rule-a", "rule-c"}, names)
}

func TestRule_EnableDisable(t *testing.T) {
	t.Parallel()
	b := newBackend()

	_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         "toggle-rule",
		EventPattern: `{"source":["x"]}`,
		State:        "ENABLED",
	})
	require.NoError(t, err)

	err = b.DisableRule(context.Background(), "toggle-rule", "")
	require.NoError(t, err)

	rule, err := b.DescribeRule(context.Background(), "toggle-rule", "")
	require.NoError(t, err)
	assert.Equal(t, "DISABLED", rule.State)

	err = b.EnableRule(context.Background(), "toggle-rule", "")
	require.NoError(t, err)

	rule, err = b.DescribeRule(context.Background(), "toggle-rule", "")
	require.NoError(t, err)
	assert.Equal(t, "ENABLED", rule.State)
}

func TestRule_NameTooLong(t *testing.T) {
	t.Parallel()
	b := newBackend()

	_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         strings.Repeat("a", 65),
		EventPattern: `{"source":["x"]}`,
	})
	require.ErrorIs(t, err, eventbridge.ErrInvalidParameter)
}

func TestRule_MutuallyExclusivePatternAndSchedule(t *testing.T) {
	t.Parallel()
	b := newBackend()

	_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:               "bad-rule",
		EventPattern:       `{"source":["x"]}`,
		ScheduleExpression: "rate(1 minute)",
	})
	require.ErrorIs(t, err, eventbridge.ErrInvalidParameter)
}

func TestRule_RequiresPatternOrSchedule(t *testing.T) {
	t.Parallel()
	b := newBackend()

	_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name: "empty-rule",
	})
	require.ErrorIs(t, err, eventbridge.ErrInvalidParameter)
}

func TestRule_DefaultStateIsEnabled(t *testing.T) {
	t.Parallel()
	b := newBackend()

	_, err := b.PutRule(context.Background(), eventbridge.PutRuleInput{
		Name:         "auto-enabled",
		EventPattern: `{"source":["x"]}`,
	})
	require.NoError(t, err)

	rule, err := b.DescribeRule(context.Background(), "auto-enabled", "")
	require.NoError(t, err)
	assert.Equal(t, "ENABLED", rule.State)
}

func TestPutRule_MutualExclusion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		input   eventbridge.PutRuleInput
		name    string
	}{
		{
			name: "both set rejects",
			input: eventbridge.PutRuleInput{
				Name:               "r",
				ScheduleExpression: "rate(1 minute)",
				EventPattern:       `{"source":["x"]}`,
			},
			wantErr: eventbridge.ErrInvalidParameter,
		},
		{
			name:    "neither set rejects",
			input:   eventbridge.PutRuleInput{Name: "r"},
			wantErr: eventbridge.ErrInvalidParameter,
		},
		{
			name:    "only schedule allowed",
			input:   eventbridge.PutRuleInput{Name: "r", ScheduleExpression: "rate(5 minutes)"},
			wantErr: nil,
		},
		{
			name:    "only pattern allowed",
			input:   eventbridge.PutRuleInput{Name: "r", EventPattern: `{"source":["test"]}`},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend()
			_, err := b.PutRule(context.Background(), tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTestEventPattern_EventJSONValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		name    string
		pattern string
		event   string
		wantOk  bool
	}{
		{
			name:    "valid pattern and event matches",
			pattern: `{"source":["my.app"]}`,
			event:   `{"source":"my.app","detail-type":"Login","detail":{}}`,
			wantOk:  true,
		},
		{
			name:    "valid pattern non-matching event",
			pattern: `{"source":["other.app"]}`,
			event:   `{"source":"my.app","detail-type":"Login","detail":{}}`,
			wantOk:  false,
		},
		{
			name:    "invalid event JSON rejects",
			pattern: `{"source":["test"]}`,
			event:   `not-json`,
			wantErr: eventbridge.ErrInvalidParameter,
		},
		{
			name:    "invalid pattern JSON rejects",
			pattern: `not-json`,
			event:   `{"source":"my.app"}`,
			wantErr: eventbridge.ErrInvalidParameter,
		},
		{
			name:    "empty pattern rejects",
			pattern: "",
			event:   `{"source":"my.app"}`,
			wantErr: eventbridge.ErrInvalidParameter,
		},
		{
			name:    "empty event rejects",
			pattern: `{"source":["my.app"]}`,
			event:   "",
			wantErr: eventbridge.ErrInvalidParameter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend()
			ok, err := b.TestEventPattern(context.Background(), tt.pattern, tt.event)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOk, ok)
		})
	}
}

func TestListRuleNamesByTarget_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		targetARN string
		want      []string
	}{
		{
			name:      "finds rules with matching target",
			targetARN: "arn:aws:lambda:us-east-1:123:function:fn",
			want:      []string{"r1"},
		},
		{
			name:      "no match returns empty",
			targetARN: "arn:aws:lambda:us-east-1:123:function:other",
			want:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend()
			_, err := b.PutRule(
				context.Background(),
				eventbridge.PutRuleInput{Name: "r1", ScheduleExpression: "rate(1 minute)"},
			)
			require.NoError(t, err)
			_, err = b.PutTargets(context.Background(), "r1", "", []eventbridge.Target{
				{ID: "t1", Arn: "arn:aws:lambda:us-east-1:123:function:fn"},
			})
			require.NoError(t, err)

			got, _, err := b.ListRuleNamesByTarget(context.Background(), tt.targetARN, "", "", 0)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestListRules_Limit asserts the Limit request parameter is threaded through to
// backend pagination for ListRules.
func TestListRules_Limit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		limit    int
		numRules int
		wantPage int
		wantMore bool
	}{
		{name: "limit_2_of_5", limit: 2, numRules: 5, wantPage: 2, wantMore: true},
		{name: "limit_3_of_3", limit: 3, numRules: 3, wantPage: 3, wantMore: false},
		{name: "zero_limit_uses_default", limit: 0, numRules: 5, wantPage: 5, wantMore: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			for i := range tt.numRules {
				_, err := b.PutRule(t.Context(), eventbridge.PutRuleInput{
					Name:         fmt.Sprintf("rule-%02d", i),
					EventPattern: `{"source":["x"]}`,
					State:        "ENABLED",
				})
				require.NoError(t, err)
			}

			rules, next, err := b.ListRules(t.Context(), "default", "", "", tt.limit)
			require.NoError(t, err)
			assert.Len(t, rules, tt.wantPage)
			if tt.wantMore {
				assert.NotEmpty(t, next)
			} else {
				assert.Empty(t, next)
			}
		})
	}
}

// TestListRules_LimitPaginationCoversAll walks all pages using the Limit and
// confirms every rule is returned exactly once across pages.
func TestListRules_LimitPaginationCoversAll(t *testing.T) {
	t.Parallel()

	b := newBackend()
	const total = 7
	for i := range total {
		_, err := b.PutRule(t.Context(), eventbridge.PutRuleInput{
			Name:         fmt.Sprintf("pg-%02d", i),
			EventPattern: `{"source":["x"]}`,
			State:        "ENABLED",
		})
		require.NoError(t, err)
	}

	seen := map[string]bool{}
	token := ""
	pages := 0
	for {
		rules, next, err := b.ListRules(t.Context(), "default", "", token, 3)
		require.NoError(t, err)
		pages++
		require.LessOrEqual(t, len(rules), 3)
		for _, r := range rules {
			assert.False(t, seen[r.Name], "duplicate rule %s", r.Name)
			seen[r.Name] = true
		}
		if next == "" {
			break
		}
		token = next
		require.LessOrEqual(t, pages, total, "pagination did not terminate")
	}

	assert.Len(t, seen, total)
	assert.Equal(t, 3, pages) // 3 + 3 + 1
}
