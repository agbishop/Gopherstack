package eventpattern_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/blackbirdworks/gopherstack/pkgs/eventpattern"
)

func TestCompareNumeric(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		op   string
		num  float64
		val  float64
		want bool
	}{
		{"gt_true", ">", 10, 5, true},
		{"gt_false", ">", 5, 10, false},
		{"gte_equal", ">=", 5, 5, true},
		{"lt_true", "<", 5, 10, true},
		{"lte_equal", "<=", 5, 5, true},
		{"eq_true", "=", 5, 5, true},
		{"eq_false", "=", 5, 6, false},
		{"unknown_op", "!=", 5, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, eventpattern.CompareNumeric(tt.op, tt.num, tt.val))
		})
	}
}

func TestMatchNumericRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		rules []any
		num   float64
		want  bool
	}{
		{"single_rule_matches", []any{">", 100.0}, 150, true},
		{"single_rule_no_match", []any{">", 100.0}, 50, false},
		{"range_matches", []any{">", 0.0, "<", 10.0}, 5, true},
		{"range_no_match", []any{">", 0.0, "<", 10.0}, 15, false},
		{"malformed_op_type", []any{5.0, 5.0}, 5, false},
		{"malformed_val_type", []any{">", "five"}, 5, false},
		{"empty_rules", []any{}, 5, true},
		{"trailing_incomplete_pair_ignored", []any{">", 0.0, "<"}, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, eventpattern.MatchNumericRules(tt.num, tt.rules))
		})
	}
}

func TestToFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in     any
		name   string
		want   float64
		wantOK bool
	}{
		{float64(5), "float64", 5, true},
		{int(5), "int", 5, true},
		{int64(5), "int64", 5, true},
		{"5", "string_not_numeric", 0, false},
		{nil, "nil", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := eventpattern.ToFloat64(tt.in)
			assert.Equal(t, tt.wantOK, ok)
			assert.InDelta(t, tt.want, got, 0)
		})
	}
}

func TestMatchCIDR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cidr string
		ip   string
		want bool
	}{
		{"in_range", "10.0.0.0/24", "10.0.0.5", true},
		{"out_of_range", "10.0.0.0/24", "10.0.1.5", false},
		{"malformed_cidr", "not-a-cidr", "10.0.0.5", false},
		{"malformed_ip", "10.0.0.0/24", "not-an-ip", false},
		{"ipv6_in_range", "2001:db8::/32", "2001:db8::1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, eventpattern.MatchCIDR(tt.cidr, tt.ip))
		})
	}
}
