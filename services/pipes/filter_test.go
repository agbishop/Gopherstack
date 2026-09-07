package pipes_test

// Exercises the matchesAnyFilter/matchesJSONPattern engine (filter.go) end to
// end through the runner: JSON event-pattern field matching, prefix/suffix/
// anything-but operators, multi-filter OR semantics, and nested pattern
// recursion (gopherstack-a2vk).

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/pipes"
)

// TestFilter_JSONPattern verifies that JSON event pattern filters are
// evaluated against the structured message body (not substring match).
func TestFilter_JSONPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pattern     string
		msgBodies   []string
		wantDeleted []string
	}{
		{
			name:    "exact_field_match_passes",
			pattern: `{"type":["order"]}`,
			msgBodies: []string{
				`{"type":"order","id":1}`,
				`{"type":"inventory","id":2}`,
			},
			wantDeleted: []string{"rh-0"},
		},
		{
			name:    "multi_field_match_both_must_match",
			pattern: `{"type":["order"],"status":["paid"]}`,
			msgBodies: []string{
				`{"type":"order","status":"paid","id":1}`,
				`{"type":"order","status":"pending","id":2}`,
				`{"type":"invoice","status":"paid","id":3}`,
			},
			wantDeleted: []string{"rh-0"},
		},
		{
			name:    "no_match_drops_all",
			pattern: `{"type":["missing-type"]}`,
			msgBodies: []string{
				`{"type":"order"}`,
			},
			wantDeleted: nil,
		},
		{
			name:    "empty_pattern_passes_all",
			pattern: "",
			msgBodies: []string{
				`{"type":"order"}`,
				`{"type":"inventory"}`,
			},
			wantDeleted: []string{"rh-0", "rh-1"},
		},
		{
			name:    "non_json_pattern_uses_substring",
			pattern: "order",
			msgBodies: []string{
				`{"type":"order"}`,
				`{"type":"inventory"}`,
			},
			wantDeleted: []string{"rh-0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b3Backend()
			_, err := b.CreatePipe(t.Context(), pipes.CreatePipeInput{
				Name:         tt.name + "-pipe",
				RoleARN:      "arn:aws:iam::111122223333:role/r",
				Source:       b3SQSSource,
				Target:       b3LambdaTarget,
				DesiredState: "RUNNING",
				SourceParameters: &pipes.SourceParameters{
					FilterCriteria: &pipes.FilterCriteria{
						Filters: []pipes.Filter{{Pattern: tt.pattern}},
					},
				},
			})
			require.NoError(t, err)
			pipes.WaitPipeRunning(t, b, tt.name+"-pipe")

			msgs := make([]*pipes.SQSMessage, len(tt.msgBodies))
			for i, body := range tt.msgBodies {
				msgs[i] = &pipes.SQSMessage{
					MessageID:     "m-" + string(rune('0'+i)),
					ReceiptHandle: "rh-" + string(rune('0'+i)),
					Body:          body,
				}
			}

			sqsReader := &b3MockSQSReader{messages: msgs}
			lambdaInvoker := &b3MockLambdaInvoker{}

			runner := pipes.NewRunner(b)
			runner.SetSQSReader(sqsReader)
			runner.SetLambdaInvoker(lambdaInvoker)

			pipes.PollAllPipesOnce(t.Context(), runner)

			sqsReader.mu.Lock()
			deleted := sqsReader.deleted
			sqsReader.mu.Unlock()

			assert.Equal(t, tt.wantDeleted, deleted)
		})
	}
}

// TestFilter_PatternOperators verifies prefix, suffix, and anything-but
// pattern operators in JSON event pattern matching.
func TestFilter_PatternOperators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pattern   string
		msgBody   string
		wantMatch bool
	}{
		{
			name:      "prefix_operator_matches",
			pattern:   `{"type":[{"prefix":"ord"}]}`,
			msgBody:   `{"type":"order"}`,
			wantMatch: true,
		},
		{
			name:      "prefix_operator_no_match",
			pattern:   `{"type":[{"prefix":"inv"}]}`,
			msgBody:   `{"type":"order"}`,
			wantMatch: false,
		},
		{
			name:      "suffix_operator_matches",
			pattern:   `{"type":[{"suffix":"der"}]}`,
			msgBody:   `{"type":"order"}`,
			wantMatch: true,
		},
		{
			name:      "suffix_operator_no_match",
			pattern:   `{"type":[{"suffix":"xyz"}]}`,
			msgBody:   `{"type":"order"}`,
			wantMatch: false,
		},
		{
			name:      "anything_but_matches_when_not_excluded",
			pattern:   `{"status":[{"anything-but":["cancelled","failed"]}]}`,
			msgBody:   `{"status":"paid"}`,
			wantMatch: true,
		},
		{
			name:      "anything_but_no_match_when_excluded",
			pattern:   `{"status":[{"anything-but":["cancelled","failed"]}]}`,
			msgBody:   `{"status":"cancelled"}`,
			wantMatch: false,
		},
		{
			// AWS event-pattern docs' own example: `"state": [ { "anything-but": "initializing" } ]`
			// -- a single string, not a list.
			name:      "anything_but_single_value_matches_when_not_excluded",
			pattern:   `{"status":[{"anything-but":"cancelled"}]}`,
			msgBody:   `{"status":"paid"}`,
			wantMatch: true,
		},
		{
			name:      "anything_but_single_value_no_match_when_excluded",
			pattern:   `{"status":[{"anything-but":"cancelled"}]}`,
			msgBody:   `{"status":"cancelled"}`,
			wantMatch: false,
		},
		{
			// docs: `"ProductName": [ { "exists": true } ]` matches when the field is present.
			name:      "exists_true_matches_present_field",
			pattern:   `{"type":[{"exists":true}]}`,
			msgBody:   `{"type":"order"}`,
			wantMatch: true,
		},
		{
			name:      "exists_true_no_match_absent_field",
			pattern:   `{"type":[{"exists":true}]}`,
			msgBody:   `{"other":"x"}`,
			wantMatch: false,
		},
		{
			// docs: `"ProductName": [ { "exists": false } ]` matches when the field is absent.
			name:      "exists_false_matches_absent_field",
			pattern:   `{"type":[{"exists":false}]}`,
			msgBody:   `{"other":"x"}`,
			wantMatch: true,
		},
		{
			name:      "exists_false_no_match_present_field",
			pattern:   `{"type":[{"exists":false}]}`,
			msgBody:   `{"type":"order"}`,
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b3Backend()
			_, err := b.CreatePipe(t.Context(), pipes.CreatePipeInput{
				Name:         "op-" + tt.name,
				RoleARN:      "arn:aws:iam::111122223333:role/r",
				Source:       b3SQSSource,
				Target:       b3LambdaTarget,
				DesiredState: "RUNNING",
				SourceParameters: &pipes.SourceParameters{
					FilterCriteria: &pipes.FilterCriteria{
						Filters: []pipes.Filter{{Pattern: tt.pattern}},
					},
				},
			})
			require.NoError(t, err)
			pipes.WaitPipeRunning(t, b, "op-"+tt.name)

			sqsReader := &b3MockSQSReader{
				messages: []*pipes.SQSMessage{{MessageID: "m1", ReceiptHandle: "rh1", Body: tt.msgBody}},
			}
			lambdaInvoker := &b3MockLambdaInvoker{}

			runner := pipes.NewRunner(b)
			runner.SetSQSReader(sqsReader)
			runner.SetLambdaInvoker(lambdaInvoker)

			pipes.PollAllPipesOnce(t.Context(), runner)

			sqsReader.mu.Lock()
			deleted := sqsReader.deleted
			sqsReader.mu.Unlock()

			if tt.wantMatch {
				assert.Equal(t, []string{"rh1"}, deleted, "message should pass filter and be deleted")
			} else {
				assert.Empty(t, deleted, "message should be dropped by filter and not deleted")
			}
		})
	}
}

// TestFilter_MultipleFilters verifies that multiple filter patterns create
// an OR condition (any matching filter passes the message).
func TestFilter_MultipleFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		filters     []pipes.Filter
		msgBodies   []string
		wantMatched int
	}{
		{
			name: "two_patterns_or_logic",
			filters: []pipes.Filter{
				{Pattern: `{"type":["order"]}`},
				{Pattern: `{"type":["payment"]}`},
			},
			msgBodies: []string{
				`{"type":"order"}`,
				`{"type":"payment"}`,
				`{"type":"inventory"}`,
			},
			wantMatched: 2,
		},
		{
			name: "empty_filter_passes_all",
			filters: []pipes.Filter{
				{Pattern: `{"type":["order"]}`},
				{Pattern: ""},
			},
			msgBodies: []string{
				`{"type":"order"}`,
				`{"type":"inventory"}`,
			},
			wantMatched: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b3Backend()
			_, err := b.CreatePipe(t.Context(), pipes.CreatePipeInput{
				Name:         "mf-" + tt.name,
				RoleARN:      "arn:aws:iam::111122223333:role/r",
				Source:       b3SQSSource,
				Target:       b3LambdaTarget,
				DesiredState: "RUNNING",
				SourceParameters: &pipes.SourceParameters{
					FilterCriteria: &pipes.FilterCriteria{
						Filters: tt.filters,
					},
				},
			})
			require.NoError(t, err)
			pipes.WaitPipeRunning(t, b, "mf-"+tt.name)

			msgs := make([]*pipes.SQSMessage, len(tt.msgBodies))
			for i, body := range tt.msgBodies {
				msgs[i] = &pipes.SQSMessage{
					MessageID:     "m" + string(rune('0'+i)),
					ReceiptHandle: "rh" + string(rune('0'+i)),
					Body:          body,
				}
			}

			sqsReader := &b3MockSQSReader{messages: msgs}
			lambdaInvoker := &b3MockLambdaInvoker{}

			runner := pipes.NewRunner(b)
			runner.SetSQSReader(sqsReader)
			runner.SetLambdaInvoker(lambdaInvoker)

			pipes.PollAllPipesOnce(t.Context(), runner)

			sqsReader.mu.Lock()
			deleted := sqsReader.deleted
			sqsReader.mu.Unlock()

			assert.Len(t, deleted, tt.wantMatched,
				"%d messages should pass the OR filter", tt.wantMatched)
		})
	}
}

// TestFilter_NestedPatterns verifies nested EventBridge-style pattern
// matching (gopherstack-a2vk): descending into nested objects such as a
// DynamoDB Streams NewImage, ANDing multiple fields at one level, ORing
// multiple array entries for one field, numeric/cidr content filters, and
// the deliberate fail-closed behavior for a bare (non-array/non-object)
// pattern value and for an unrecognized matcher-object key.
func TestFilter_NestedPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		msgBody string
		// rawPattern marks a pattern that CreatePipe's Filter.Pattern
		// validation (gopherstack-sphp) now rejects at creation time -- the
		// pipe is created with an empty pattern and the invalid pattern is
		// injected directly into the backend afterward, via
		// SetFilterPatternForTest, so the case still exercises filter.go's
		// runtime fail-closed matching (the defense-in-depth layer behind
		// creation-time validation, gopherstack-lrgk) rather than a
		// CreatePipe rejection.
		rawPattern bool
		wantMatch  bool
	}{
		{
			name:      "nested_dynamodb_newimage_matches",
			pattern:   `{"dynamodb":{"NewImage":{"id":{"S":["1"]}}}}`,
			msgBody:   `{"dynamodb":{"NewImage":{"id":{"S":"1"},"status":{"S":"open"}}}}`,
			wantMatch: true,
		},
		{
			name:      "nested_dynamodb_newimage_value_mismatch",
			pattern:   `{"dynamodb":{"NewImage":{"id":{"S":["1"]}}}}`,
			msgBody:   `{"dynamodb":{"NewImage":{"id":{"S":"2"},"status":{"S":"open"}}}}`,
			wantMatch: false,
		},
		{
			name:      "nested_field_absent_no_match",
			pattern:   `{"dynamodb":{"NewImage":{"id":{"S":["1"]}}}}`,
			msgBody:   `{"dynamodb":{"OldImage":{"id":{"S":"1"}}}}`,
			wantMatch: false,
		},
		{
			name:      "nested_parent_field_absent_no_match",
			pattern:   `{"dynamodb":{"NewImage":{"id":{"S":["1"]}}}}`,
			msgBody:   `{"eventName":"INSERT"}`,
			wantMatch: false,
		},
		{
			name:      "multi_key_and_both_present_matches",
			pattern:   `{"type":["order"],"region":["us-east-1"]}`,
			msgBody:   `{"type":"order","region":"us-east-1"}`,
			wantMatch: true,
		},
		{
			name:      "multi_key_and_one_mismatched_no_match",
			pattern:   `{"type":["order"],"region":["us-east-1"]}`,
			msgBody:   `{"type":"order","region":"eu-west-1"}`,
			wantMatch: false,
		},
		{
			name:      "multi_entry_or_second_value_matches",
			pattern:   `{"type":["order","payment","refund"]}`,
			msgBody:   `{"type":"payment"}`,
			wantMatch: true,
		},
		{
			name:      "multi_entry_or_no_value_matches",
			pattern:   `{"type":["order","payment","refund"]}`,
			msgBody:   `{"type":"inventory"}`,
			wantMatch: false,
		},
		{
			name:      "numeric_operator_matches",
			pattern:   `{"amount":[{"numeric":[">",100]}]}`,
			msgBody:   `{"amount":150}`,
			wantMatch: true,
		},
		{
			name:      "numeric_operator_no_match",
			pattern:   `{"amount":[{"numeric":[">",100]}]}`,
			msgBody:   `{"amount":50}`,
			wantMatch: false,
		},
		{
			name:      "cidr_operator_matches",
			pattern:   `{"sourceIp":[{"cidr":"10.0.0.0/24"}]}`,
			msgBody:   `{"sourceIp":"10.0.0.5"}`,
			wantMatch: true,
		},
		{
			name:      "cidr_operator_no_match",
			pattern:   `{"sourceIp":[{"cidr":"10.0.0.0/24"}]}`,
			msgBody:   `{"sourceIp":"10.0.1.5"}`,
			wantMatch: false,
		},
		{
			// A bare (non-array) pattern value is not valid EventBridge
			// syntax (services/eventbridge/pattern.go's validatePatternObject
			// rejects it outright at compile time; pipes' own CreatePipe now
			// rejects it too, gopherstack-sphp) -- filter.go's matching still
			// fails closed instead of silently treating the scalar as an
			// exact-match value, for a pattern that reaches it by some other
			// path (a pre-existing pipe from before validation shipped).
			name:       "bare_scalar_pattern_value_never_matches",
			pattern:    `{"type":"order"}`,
			msgBody:    `{"type":"order"}`,
			rawPattern: true,
			wantMatch:  false,
		},
		{
			// An unrecognized matcher-object key (not one of exists/prefix/
			// suffix/numeric/anything-but/cidr) must never silently match
			// everything -- deliberately fails closed. CreatePipe now
			// rejects this pattern too (gopherstack-sphp).
			name:       "unrecognized_matcher_object_never_matches",
			pattern:    `{"type":[{"wildcard":"ord*"}]}`,
			msgBody:    `{"type":"order"}`,
			rawPattern: true,
			wantMatch:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b3Backend()
			createPattern := tt.pattern
			if tt.rawPattern {
				createPattern = ""
			}
			_, err := b.CreatePipe(t.Context(), pipes.CreatePipeInput{
				Name:         "np-" + tt.name,
				RoleARN:      "arn:aws:iam::111122223333:role/r",
				Source:       b3SQSSource,
				Target:       b3LambdaTarget,
				DesiredState: "RUNNING",
				SourceParameters: &pipes.SourceParameters{
					FilterCriteria: &pipes.FilterCriteria{
						Filters: []pipes.Filter{{Pattern: createPattern}},
					},
				},
			})
			require.NoError(t, err)
			if tt.rawPattern {
				b.SetFilterPatternForTest("np-"+tt.name, tt.pattern)
			}
			pipes.WaitPipeRunning(t, b, "np-"+tt.name)

			sqsReader := &b3MockSQSReader{
				messages: []*pipes.SQSMessage{{MessageID: "m1", ReceiptHandle: "rh1", Body: tt.msgBody}},
			}
			lambdaInvoker := &b3MockLambdaInvoker{}

			runner := pipes.NewRunner(b)
			runner.SetSQSReader(sqsReader)
			runner.SetLambdaInvoker(lambdaInvoker)

			pipes.PollAllPipesOnce(t.Context(), runner)

			sqsReader.mu.Lock()
			deleted := sqsReader.deleted
			sqsReader.mu.Unlock()

			if tt.wantMatch {
				assert.Equal(t, []string{"rh1"}, deleted, "message should pass filter and be deleted")
			} else {
				assert.Empty(t, deleted, "message should be dropped by filter and not deleted")
			}
		})
	}
}

// TestFilter_ExactMatchTypeSensitivity pins matchesExactRule's type-sensitive
// exact-match semantics (gopherstack-50hq, following up gopherstack-a2vk): a
// string pattern element no longer coerces to match a numerically- or
// boolean-equal event value, matching EventBridge's real behavior. The last
// case also exercises a non-comparable decoded type (a JSON array as a
// pattern element) to pin the use of reflect.DeepEqual over == -- == would
// panic there instead of just failing to match.
func TestFilter_ExactMatchTypeSensitivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		msgBody string
		// rawPattern: see the identical field on TestFilter_NestedPatterns's
		// table -- this pattern is also rejected by CreatePipe's Filter.Pattern
		// validation (gopherstack-sphp), since a raw JSON array is not a
		// valid matcher-array element under real EventBridge pattern syntax
		// either.
		rawPattern bool
		wantMatch  bool
	}{
		{
			name:      "string_pattern_vs_numeric_value_no_match",
			pattern:   `{"amount":["5"]}`,
			msgBody:   `{"amount":5}`,
			wantMatch: false,
		},
		{
			name:      "numeric_pattern_vs_numeric_value_matches",
			pattern:   `{"amount":[5]}`,
			msgBody:   `{"amount":5}`,
			wantMatch: true,
		},
		{
			name:      "bool_pattern_vs_bool_value_matches",
			pattern:   `{"active":[true]}`,
			msgBody:   `{"active":true}`,
			wantMatch: true,
		},
		{
			name:      "string_pattern_vs_bool_value_no_match",
			pattern:   `{"active":["true"]}`,
			msgBody:   `{"active":true}`,
			wantMatch: false,
		},
		{
			// The rule element itself is a JSON array (not an object, so it
			// isn't treated as a content-filter like {"prefix":...}), and
			// the message field is also an array -- both decode to the same
			// non-comparable dynamic type ([]any), which is exactly the
			// shape that panics under == but not under reflect.DeepEqual.
			name:       "array_pattern_element_vs_array_value_no_match_no_panic",
			pattern:    `{"payload":[[1,2]]}`,
			msgBody:    `{"payload":[3,4]}`,
			rawPattern: true,
			wantMatch:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := b3Backend()
			createPattern := tt.pattern
			if tt.rawPattern {
				createPattern = ""
			}
			_, err := b.CreatePipe(t.Context(), pipes.CreatePipeInput{
				Name:         "ts-" + tt.name,
				RoleARN:      "arn:aws:iam::111122223333:role/r",
				Source:       b3SQSSource,
				Target:       b3LambdaTarget,
				DesiredState: "RUNNING",
				SourceParameters: &pipes.SourceParameters{
					FilterCriteria: &pipes.FilterCriteria{
						Filters: []pipes.Filter{{Pattern: createPattern}},
					},
				},
			})
			require.NoError(t, err)
			if tt.rawPattern {
				b.SetFilterPatternForTest("ts-"+tt.name, tt.pattern)
			}
			pipes.WaitPipeRunning(t, b, "ts-"+tt.name)

			sqsReader := &b3MockSQSReader{
				messages: []*pipes.SQSMessage{{MessageID: "m1", ReceiptHandle: "rh1", Body: tt.msgBody}},
			}
			lambdaInvoker := &b3MockLambdaInvoker{}

			runner := pipes.NewRunner(b)
			runner.SetSQSReader(sqsReader)
			runner.SetLambdaInvoker(lambdaInvoker)

			pipes.PollAllPipesOnce(t.Context(), runner)

			sqsReader.mu.Lock()
			deleted := sqsReader.deleted
			sqsReader.mu.Unlock()

			if tt.wantMatch {
				assert.Equal(t, []string{"rh1"}, deleted, "message should pass filter and be deleted")
			} else {
				assert.Empty(t, deleted, "message should be dropped by filter and not deleted")
			}
		})
	}
}
