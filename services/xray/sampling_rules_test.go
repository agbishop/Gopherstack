package xray_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/xray"
)

func TestInMemoryBackend_CreateSamplingRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs   error
		name        string
		rule        xray.SamplingRule
		createFirst bool
		wantErr     bool
	}{
		{
			name: "creates rule",
			rule: xray.SamplingRule{RuleName: "my-rule", FixedRate: 0.05, Priority: 1},
		},
		{
			name:        "duplicate returns conflict",
			rule:        xray.SamplingRule{RuleName: "dup-rule"},
			createFirst: true,
			wantErr:     true,
			wantErrIs:   awserr.ErrConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			if tt.createFirst {
				_, err := b.CreateSamplingRule(tt.rule)
				require.NoError(t, err)
			}

			r, err := b.CreateSamplingRule(tt.rule)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.rule.RuleName, r.RuleName)
			assert.NotEmpty(t, r.RuleARN)
		})
	}
}

func TestInMemoryBackend_GetSamplingRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		ruleNames     []string
		wantUserRules int // number of user-created rules (not counting Default)
	}{
		{
			// A fresh backend always has the built-in Default rule.
			name:          "default_rule_present",
			wantUserRules: 0,
		},
		{
			name:          "multiple rules sorted by priority",
			ruleNames:     []string{"rule-b", "rule-a", "rule-c"},
			wantUserRules: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			for _, name := range tt.ruleNames {
				_, err := b.CreateSamplingRule(xray.SamplingRule{RuleName: name, Priority: 1})
				require.NoError(t, err)
			}

			rules := b.GetSamplingRules()
			// +1 for the always-present Default rule.
			assert.Len(t, rules, tt.wantUserRules+1)

			// Default rule should be last (highest priority value = lowest precedence).
			if len(rules) > 0 {
				assert.Equal(t, "Default", rules[len(rules)-1].RuleName,
					"Default rule should be sorted last (priority 10000)")
			}
		})
	}
}

func TestInMemoryBackend_UpdateSamplingRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		ruleName  string
		updates   xray.SamplingRule
		create    bool
		wantErr   bool
	}{
		{
			name:     "updates service name",
			ruleName: "my-rule",
			create:   true,
			updates:  xray.SamplingRule{ServiceName: "updated-svc"},
		},
		{
			name:      "not found",
			ruleName:  "missing-rule",
			wantErr:   true,
			wantErrIs: awserr.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			if tt.create {
				_, err := b.CreateSamplingRule(xray.SamplingRule{RuleName: tt.ruleName})
				require.NoError(t, err)
			}

			r, err := b.UpdateSamplingRule(tt.ruleName, tt.updates)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.updates.ServiceName, r.ServiceName)
		})
	}
}

func TestInMemoryBackend_DeleteSamplingRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		ruleName  string
		create    bool
		wantErr   bool
	}{
		{
			name:     "deletes existing rule",
			ruleName: "my-rule",
			create:   true,
		},
		{
			name:      "not found",
			ruleName:  "missing-rule",
			wantErr:   true,
			wantErrIs: awserr.ErrNotFound,
		},
		{
			name:      "cannot delete Default rule",
			ruleName:  "Default",
			wantErr:   true,
			wantErrIs: awserr.ErrInvalidParameter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			if tt.create {
				_, err := b.CreateSamplingRule(xray.SamplingRule{RuleName: tt.ruleName, Priority: 1})
				require.NoError(t, err)
			}

			r, err := b.DeleteSamplingRule(tt.ruleName, "")

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.ruleName, r.RuleName)

			// After deleting user rule, only the Default rule should remain.
			rules := b.GetSamplingRules()
			require.Len(t, rules, 1)
			assert.Equal(t, "Default", rules[0].RuleName, "Default rule should always remain")
		})
	}
}

func TestDeleteSamplingRule_ClearsResourceTagsOnRecreate(t *testing.T) {
	t.Parallel()

	b := newTestBackend(t)

	created, err := b.CreateSamplingRule(xray.SamplingRule{RuleName: "reused-rule", Priority: 1})
	require.NoError(t, err)
	require.NoError(t, b.TagResource(created.RuleARN, map[string]string{"env": "prod"}))

	_, err = b.DeleteSamplingRule("reused-rule", "")
	require.NoError(t, err)

	recreated, err := b.CreateSamplingRule(xray.SamplingRule{RuleName: "reused-rule", Priority: 1})
	require.NoError(t, err)
	require.Equal(t, created.RuleARN, recreated.RuleARN)

	tags, err := b.ListTagsForResource(recreated.RuleARN)
	require.NoError(t, err)
	assert.Empty(t, tags)
}

// TestModifiedAtTracking verifies ModifiedAt is updated on UpdateSamplingRule.
func TestModifiedAtTracking(t *testing.T) {
	t.Parallel()

	b := xray.NewInMemoryBackend("000000000000", "us-east-1")

	r, err := b.CreateSamplingRule(xray.SamplingRule{RuleName: "track-rule", FixedRate: 0.1, Priority: 1})
	require.NoError(t, err)

	createdAt := r.CreatedAt
	modifiedAt := r.ModifiedAt

	// Small sleep to ensure timestamps differ.
	time.Sleep(time.Millisecond)

	updated, err := b.UpdateSamplingRule("track-rule", xray.SamplingRule{ServiceName: "svc"})
	require.NoError(t, err)

	assert.Equal(t, createdAt, updated.CreatedAt)
	assert.True(t, updated.ModifiedAt.After(modifiedAt))
}

// TestAddSamplingRuleInternal verifies the seed helper.
func TestAddSamplingRuleInternal(t *testing.T) {
	t.Parallel()

	b := xray.NewInMemoryBackend("000000000000", "us-east-1")
	b.AddSamplingRuleInternal(xray.SamplingRule{RuleName: "seed-rule", FixedRate: 0.5, Priority: 5})

	// Default rule + seed-rule = 2.
	assert.Equal(t, 2, b.SamplingRuleCount())

	rules := b.GetSamplingRules()
	require.Len(t, rules, 2)
	// Sorted by priority: seed-rule (5) comes before Default (10000).
	assert.Equal(t, "seed-rule", rules[0].RuleName)
	assert.NotEmpty(t, rules[0].RuleARN)
	assert.Equal(t, "Default", rules[1].RuleName)
}

// TestDefaultSamplingRulePresent verifies the Default rule is always seeded.
func TestDefaultSamplingRulePresent(t *testing.T) {
	t.Parallel()

	b := xray.NewInMemoryBackend("000000000000", "us-east-1")
	rules := b.GetSamplingRules()

	var found bool

	for _, r := range rules {
		if r.RuleName == "Default" {
			found = true
			assert.EqualValues(t, 10000, r.Priority, "Default rule must have priority 10000")
			assert.NotEmpty(t, r.RuleARN, "Default rule must have an ARN")
		}
	}

	assert.True(t, found, "Default sampling rule must always be present in a new backend")
}

// TestDefaultSamplingRuleUndeletableBackend verifies the Default rule cannot be deleted via backend.
func TestDefaultSamplingRuleUndeletableBackend(t *testing.T) {
	t.Parallel()

	b := xray.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.DeleteSamplingRule("Default", "")
	require.Error(t, err, "DeleteSamplingRule(Default) must return an error")
	assert.ErrorIs(t, err, xray.ErrDefaultRuleUndeletable)
}

// TestDefaultSamplingRuleSortedLast verifies Default rule is last in priority-sorted list.
func TestDefaultSamplingRuleSortedLast(t *testing.T) {
	t.Parallel()

	b := xray.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateSamplingRule(xray.SamplingRule{RuleName: "my-rule", Priority: 100, FixedRate: 0.1})
	require.NoError(t, err)

	rules := b.GetSamplingRules()
	require.NotEmpty(t, rules)
	assert.Equal(t, "Default", rules[len(rules)-1].RuleName,
		"Default rule (priority 10000) must be sorted last")
}

// TestSamplingRules_SortedByPriority verifies rules are returned sorted by priority ascending.
func TestSamplingRules_SortedByPriority(t *testing.T) {
	t.Parallel()

	b := xray.NewInMemoryBackend("000000000000", "us-east-1")

	// Create rules with various priorities.
	rules := []xray.SamplingRule{
		{RuleName: "high-prio", Priority: 500, FixedRate: 0.1},
		{RuleName: "low-prio", Priority: 9000, FixedRate: 0.05},
		{RuleName: "top-prio", Priority: 50, FixedRate: 0.5},
	}

	for _, r := range rules {
		_, err := b.CreateSamplingRule(r)
		require.NoError(t, err)
	}

	got := b.GetSamplingRules()
	require.NotEmpty(t, got)

	// Verify ascending priority order.
	for i := 1; i < len(got); i++ {
		assert.LessOrEqual(t, got[i-1].Priority, got[i].Priority,
			"rules must be sorted by priority ascending (index %d→%d: %s→%s)",
			i-1, i, got[i-1].RuleName, got[i].RuleName)
	}
}
