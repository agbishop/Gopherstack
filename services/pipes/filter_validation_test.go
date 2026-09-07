package pipes_test

// Regression coverage for gopherstack-sphp: Filter.Pattern syntax validation
// at CreatePipe/UpdatePipe time. Mirrors
// services/eventbridge/pattern_validation_test.go's shape, scoped to the
// matcher set filter.go actually implements (services/pipes/filter.go).

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/pipes"
)

func filterPatternCases() []struct {
	name        string
	pattern     string
	expectError bool
} {
	return []struct {
		name        string
		pattern     string
		expectError bool
	}{
		{name: "empty_pattern", pattern: "", expectError: false},
		{name: "plain_substring_pattern", pattern: "order", expectError: false},
		{name: "valid_exact_match", pattern: `{"type":["order"]}`, expectError: false},
		{
			name:        "valid_nested_object",
			pattern:     `{"dynamodb":{"NewImage":{"id":{"S":["1"]}}}}`,
			expectError: false,
		},
		{name: "valid_prefix_matcher", pattern: `{"type":[{"prefix":"ord"}]}`, expectError: false},
		{name: "valid_suffix_matcher", pattern: `{"type":[{"suffix":"der"}]}`, expectError: false},
		{name: "valid_exists_matcher", pattern: `{"type":[{"exists":true}]}`, expectError: false},
		{name: "valid_numeric_matcher", pattern: `{"amount":[{"numeric":[">",100]}]}`, expectError: false},
		{name: "valid_cidr_matcher", pattern: `{"sourceIp":[{"cidr":"10.0.0.0/24"}]}`, expectError: false},
		{
			name:        "valid_anything_but_single_string",
			pattern:     `{"status":[{"anything-but":"cancelled"}]}`,
			expectError: false,
		},
		{
			name:        "valid_anything_but_string_list",
			pattern:     `{"status":[{"anything-but":["a","b"]}]}`,
			expectError: false,
		},
		{
			name:        "invalid_bare_scalar_field_value",
			pattern:     `{"type":"order"}`,
			expectError: true,
		},
		{
			name:        "invalid_number_field_value",
			pattern:     `{"source":42}`,
			expectError: true,
		},
		{
			name:        "invalid_malformed_json",
			pattern:     `{"type":`,
			expectError: true,
		},
		{
			name:        "invalid_unknown_matcher_wildcard",
			pattern:     `{"type":[{"wildcard":"ord*"}]}`,
			expectError: true,
		},
		{
			name:        "invalid_unknown_matcher_equals_ignore_case",
			pattern:     `{"type":[{"equals-ignore-case":"order"}]}`,
			expectError: true,
		},
		{
			name:        "invalid_or_combinator",
			pattern:     `{"$or":[{"type":["order"]},{"type":["payment"]}]}`,
			expectError: true,
		},
		{
			name:        "invalid_nested_prefix_ignore_case",
			pattern:     `{"type":[{"prefix":{"equals-ignore-case":"ord"}}]}`,
			expectError: true,
		},
		{
			name:        "invalid_anything_but_numeric",
			pattern:     `{"amount":[{"anything-but":5}]}`,
			expectError: true,
		},
		{
			name:        "invalid_anything_but_object_form",
			pattern:     `{"type":[{"anything-but":{"prefix":"x"}}]}`,
			expectError: true,
		},
		{
			name:        "invalid_anything_but_mixed_list",
			pattern:     `{"status":[{"anything-but":["a",5]}]}`,
			expectError: true,
		},
	}
}

// TestFilterPatternValidation_CreatePipe verifies CreatePipe rejects a
// structurally invalid Filter.Pattern with ValidationException instead of
// silently accepting it (gopherstack-sphp).
func TestFilterPatternValidation_CreatePipe(t *testing.T) {
	t.Parallel()

	for _, tt := range filterPatternCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := pipes.NewInMemoryBackend("111122223333", "eu-west-1")
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				Name:         "cp-" + tt.name,
				RoleARN:      "arn:aws:iam::111122223333:role/r",
				Source:       "arn:aws:sqs:eu-west-1:111122223333:q",
				Target:       "arn:aws:lambda:eu-west-1:111122223333:function:fn",
				DesiredState: "RUNNING",
				SourceParameters: &pipes.SourceParameters{
					FilterCriteria: &pipes.FilterCriteria{
						Filters: []pipes.Filter{{Pattern: tt.pattern}},
					},
				},
			})

			if tt.expectError {
				assert.Error(t, err, "expected CreatePipe error for pattern %s", tt.pattern)
			} else {
				require.NoError(t, err, "expected no CreatePipe error for pattern %s", tt.pattern)
			}
		})
	}
}

// TestFilterPatternValidation_UpdatePipe verifies UpdatePipe rejects a
// structurally invalid Filter.Pattern the same way CreatePipe does
// (gopherstack-sphp).
func TestFilterPatternValidation_UpdatePipe(t *testing.T) {
	t.Parallel()

	for _, tt := range filterPatternCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := pipes.NewInMemoryBackend("111122223333", "eu-west-1")
			name := "up-" + tt.name
			_, err := b.CreatePipe(context.Background(), pipes.CreatePipeInput{
				Name:         name,
				RoleARN:      "arn:aws:iam::111122223333:role/r",
				Source:       "arn:aws:sqs:eu-west-1:111122223333:q",
				Target:       "arn:aws:lambda:eu-west-1:111122223333:function:fn",
				DesiredState: "RUNNING",
			})
			require.NoError(t, err)

			_, err = b.UpdatePipe(context.Background(), name, pipes.UpdatePipeInput{
				RoleARN: "arn:aws:iam::111122223333:role/r",
				SourceParameters: &pipes.SourceParameters{
					FilterCriteria: &pipes.FilterCriteria{
						Filters: []pipes.Filter{{Pattern: tt.pattern}},
					},
				},
			})

			if tt.expectError {
				assert.Error(t, err, "expected UpdatePipe error for pattern %s", tt.pattern)
			} else {
				require.NoError(t, err, "expected no UpdatePipe error for pattern %s", tt.pattern)
			}
		})
	}
}
