package inspector2_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/inspector2"
)

// TestFilterSuppression guards against the "accepted but never done" bug
// class this family of AWS emulators keeps shipping (guardduty's
// Filter.Action ARCHIVE, securityhub's automation rules): real CreateFilter
// documents that "When the filter action is set to SUPPRESS this action
// creates a suppression rule" (aws-sdk-go-v2/service/inspector2's
// api_op_CreateFilter.go doc comment) -- a persistent rule, not a one-off,
// so it must both retroactively suppress existing matches and prospectively
// suppress future ones.
func TestFilterSuppression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(t *testing.T, b *inspector2.InMemoryBackend)
		name string
	}{
		{
			name: "create_suppress_filter_suppresses_matching_active_findings",
			fn: func(t *testing.T, b *inspector2.InMemoryBackend) {
				t.Helper()

				match, err := b.SeedFinding(inspector2.Finding{Type: "PACKAGE_VULNERABILITY", Title: "supp-me"})
				require.NoError(t, err)

				other, err := b.SeedFinding(inspector2.Finding{Type: "CODE_VULNERABILITY", Title: "not-me"})
				require.NoError(t, err)

				_, err = b.CreateFilter(
					"suppress-package-vuln", "SUPPRESS", "", "",
					map[string]any{
						"findingType": []any{map[string]any{"value": "PACKAGE_VULNERABILITY"}},
					},
					nil,
				)
				require.NoError(t, err)

				statuses := findingStatusesByARN(t, b)
				assert.Equal(t, "SUPPRESSED", statuses[match.FindingArn])
				assert.Equal(t, "ACTIVE", statuses[other.FindingArn])
			},
		},
		{
			name: "seed_finding_after_suppress_filter_arrives_pre_suppressed",
			fn: func(t *testing.T, b *inspector2.InMemoryBackend) {
				t.Helper()

				_, err := b.CreateFilter(
					"suppress-network", "SUPPRESS", "", "",
					map[string]any{
						"findingType": []any{map[string]any{"value": "NETWORK_REACHABILITY"}},
					},
					nil,
				)
				require.NoError(t, err)

				f, err := b.SeedFinding(inspector2.Finding{Type: "NETWORK_REACHABILITY", Title: "new-match"})
				require.NoError(t, err)

				assert.Equal(t, "SUPPRESSED", f.Status)
			},
		},
		{
			name: "update_filter_action_to_suppress_applies_to_existing_findings",
			fn: func(t *testing.T, b *inspector2.InMemoryBackend) {
				t.Helper()

				created, err := b.CreateFilter(
					"initially-none", "NONE", "", "",
					map[string]any{
						"findingType": []any{map[string]any{"value": "CODE_VULNERABILITY"}},
					},
					nil,
				)
				require.NoError(t, err)

				f, err := b.SeedFinding(inspector2.Finding{Type: "CODE_VULNERABILITY", Title: "later-suppressed"})
				require.NoError(t, err)
				require.Equal(t, "ACTIVE", findingStatusesByARN(t, b)[f.FindingArn])

				_, err = b.UpdateFilter(created.Arn, "SUPPRESS", "", "", nil)
				require.NoError(t, err)

				assert.Equal(t, "SUPPRESSED", findingStatusesByARN(t, b)[f.FindingArn])
			},
		},
		{
			name: "non_matching_finding_untouched_by_suppress_filter",
			fn: func(t *testing.T, b *inspector2.InMemoryBackend) {
				t.Helper()

				f, err := b.SeedFinding(inspector2.Finding{Type: "PACKAGE_VULNERABILITY", Title: "unrelated"})
				require.NoError(t, err)

				_, err = b.CreateFilter(
					"suppress-title", "SUPPRESS", "", "",
					map[string]any{
						"title": []any{map[string]any{"value": "does-not-match"}},
					},
					nil,
				)
				require.NoError(t, err)

				assert.Equal(t, "ACTIVE", findingStatusesByARN(t, b)[f.FindingArn])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.fn(t, inspector2.NewInMemoryBackend("123456789012", "us-east-1"))
		})
	}
}

func findingStatusesByARN(t *testing.T, b *inspector2.InMemoryBackend) map[string]string {
	t.Helper()

	findings, _, err := b.ListFindings(0, "", nil, "", "")
	require.NoError(t, err)

	out := make(map[string]string, len(findings))
	for _, f := range findings {
		out[f.FindingArn] = f.Status
	}

	return out
}
