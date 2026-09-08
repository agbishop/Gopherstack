package detective_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/detective"
)

// TestStartInvestigation_DerivesEntityType verifies EntityType is derived
// server-side from the EntityArn resource segment. The real StartInvestigation
// request shape has no EntityType input member at all -- Detective infers it
// from whether the ARN names an IAM role or an IAM user -- so a real SDK
// caller never sends one, and the emulator must not depend on client input to
// populate it.
func TestStartInvestigation_DerivesEntityType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entityARN  string
		wantType   string
		wantErrLen bool
	}{
		{
			name:      "IAM role ARN",
			entityARN: "arn:aws:iam::123456789012:role/example-role",
			wantType:  "IAM_ROLE",
		},
		{
			name:      "IAM user ARN",
			entityARN: "arn:aws:iam::123456789012:user/example-user",
			wantType:  "IAM_USER",
		},
		{
			name:      "IAM role ARN with path",
			entityARN: "arn:aws:iam::123456789012:role/path/to/example-role",
			wantType:  "IAM_ROLE",
		},
		{
			name:       "non-IAM ARN rejected",
			entityARN:  "arn:aws:s3:::somebucket",
			wantErrLen: true,
		},
		{
			name:       "IAM group ARN rejected",
			entityARN:  "arn:aws:iam::123456789012:group/example-group",
			wantErrLen: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := detective.NewInMemoryBackend("000000000000", "us-east-1")

			g, err := b.CreateGraph(nil)
			require.NoError(t, err)

			id, startErr := b.StartInvestigation(
				g.Arn, tc.entityARN, time.Now().Add(-time.Hour).UTC(), time.Now().UTC(),
			)
			if tc.wantErrLen {
				require.Error(t, startErr)

				return
			}

			require.NoError(t, startErr)

			inv, getErr := b.GetInvestigation(g.Arn, id)
			require.NoError(t, getErr)
			assert.Equal(t, tc.wantType, inv.EntityType)
		})
	}
}

// TestStartInvestigation_ReachesTerminalStatus verifies GetInvestigation's
// Status field ("based on the completion status of the investigation" per
// aws-sdk-go-v2/service/detective/types.GetInvestigationOutput.Status doc
// comment, with real terminal values RUNNING/FAILED/SUCCESSFUL) actually
// reaches a terminal value. Indicator derivation (builtInIndicators) is a
// pure function computed synchronously at StartInvestigation time, so
// nothing is still "running" by the time it returns -- a client that polls
// GetInvestigation waiting for RUNNING to end must not wait forever.
func TestStartInvestigation_ReachesTerminalStatus(t *testing.T) {
	t.Parallel()

	b := detective.NewInMemoryBackend("000000000000", "us-east-1")

	g, err := b.CreateGraph(nil)
	require.NoError(t, err)

	id, startErr := b.StartInvestigation(
		g.Arn, "arn:aws:iam::123456789012:user/alice", time.Now().Add(-time.Hour).UTC(), time.Now().UTC(),
	)
	require.NoError(t, startErr)

	inv, getErr := b.GetInvestigation(g.Arn, id)
	require.NoError(t, getErr)
	assert.NotEqual(t, "RUNNING", inv.Status, "investigation must reach a terminal Status, not stay RUNNING forever")
	assert.Equal(t, "SUCCESSFUL", inv.Status)
}

// TestListIndicators_OnlySeverityFloorTypesProduced pins that StartInvestigation
// only ever sets Severity to INFORMATIONAL (there is no code path that derives
// or assigns any other Severity value), so ListIndicators never surfaces the
// FLAGGED_IP_ADDRESS/IMPOSSIBLE_TRAVEL/RELATED_FINDING/RELATED_FINDING_GROUP
// indicator types. builtInIndicators used to gate those four on Severity >=
// MEDIUM/HIGH; that gating was removed as dead code, so this test now guards
// against reintroducing them as well as against the Severity floor moving.
// gopherstack-b6wo: real Detective derives Severity from the indicators
// discovered during analysis (aws-sdk-go-v2/service/detective/types.go's
// InvestigationDetail.Severity doc comment: "Severity based on the likelihood
// and impact of the indicators of compromise discovered in the investigation"),
// which requires real threat-intelligence analysis this emulator does not
// perform -- INFORMATIONAL is the honest floor, not a bug to fix by inventing
// a fake derivation rule.
func TestListIndicators_OnlySeverityFloorTypesProduced(t *testing.T) {
	t.Parallel()

	b := detective.NewInMemoryBackend("000000000000", "us-east-1")

	g, err := b.CreateGraph(nil)
	require.NoError(t, err)

	id, startErr := b.StartInvestigation(
		g.Arn, "arn:aws:iam::123456789012:role/alice", time.Now().Add(-time.Hour).UTC(), time.Now().UTC(),
	)
	require.NoError(t, startErr)

	inv, getErr := b.GetInvestigation(g.Arn, id)
	require.NoError(t, getErr)
	require.Equal(t, "INFORMATIONAL", inv.Severity, "StartInvestigation has no path to a non-floor Severity")

	indicators, _, listErr := b.ListIndicators(g.Arn, id, "", 0, "")
	require.NoError(t, listErr)

	unreachable := map[string]bool{
		"FLAGGED_IP_ADDRESS":    true,
		"IMPOSSIBLE_TRAVEL":     true,
		"RELATED_FINDING":       true,
		"RELATED_FINDING_GROUP": true,
	}

	for _, ind := range indicators {
		assert.False(t, unreachable[ind.IndicatorType],
			"IndicatorType %q requires Severity >= MEDIUM, which no investigation ever reaches", ind.IndicatorType)
	}
}
