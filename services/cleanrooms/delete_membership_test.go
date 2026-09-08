package cleanrooms_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/cleanrooms"
)

// TestDeleteMembership_RejectsWhileResourcesRemain covers api_op_DeleteMembership.go:
// "Deletes a specified membership. All resources under a membership must be
// deleted." Each case seeds exactly one membership-scoped resource type,
// confirms DeleteMembership is rejected with ConflictException while it
// exists, then confirms deletion succeeds once that resource is removed.
func TestDeleteMembership_RejectsWhileResourcesRemain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		create func(t *testing.T, b *cleanrooms.InMemoryBackend, membershipID string) (cleanup func())
		name   string
	}{
		{
			name: "configuredTableAssociation",
			create: func(t *testing.T, b *cleanrooms.InMemoryBackend, membershipID string) func() {
				t.Helper()
				table, err := b.CreateConfiguredTable(
					"table-1", "a table", map[string]any{"referenceArn": "arn:aws:glue:table"},
					[]string{"col1"}, "DIRECT_QUERY", nil,
				)
				require.NoError(t, err)
				assoc, err := b.CreateConfiguredTableAssociation(
					membershipID, "assoc-1", "an association", table.ID,
					"arn:aws:iam::111122223333:role/cleanrooms", nil,
				)
				require.NoError(t, err)

				return func() {
					require.NoError(t, b.DeleteConfiguredTableAssociation(membershipID, assoc.ID))
				}
			},
		},
		{
			name: "analysisTemplate",
			create: func(t *testing.T, b *cleanrooms.InMemoryBackend, membershipID string) func() {
				t.Helper()
				tmpl, err := b.CreateAnalysisTemplate(
					membershipID, "template-1", "a template", "SQL",
					map[string]any{"text": "SELECT 1"}, nil, nil,
				)
				require.NoError(t, err)

				return func() {
					require.NoError(t, b.DeleteAnalysisTemplate(membershipID, tmpl.ID))
				}
			},
		},
		{
			name: "privacyBudgetTemplate",
			create: func(t *testing.T, b *cleanrooms.InMemoryBackend, membershipID string) func() {
				t.Helper()
				budget, err := b.CreatePrivacyBudgetTemplate(
					membershipID, "DIFFERENTIAL_PRIVACY", "NOT_ALLOWED",
					map[string]any{"epsilon": float64(10)}, nil,
				)
				require.NoError(t, err)

				return func() {
					require.NoError(t, b.DeletePrivacyBudgetTemplate(membershipID, budget.ID))
				}
			},
		},
		{
			name: "idMappingTable",
			create: func(t *testing.T, b *cleanrooms.InMemoryBackend, membershipID string) func() {
				t.Helper()
				table, err := b.CreateIDMappingTable(
					membershipID, "id-mapping-1", "an id mapping table",
					map[string]any{"inputReferenceArn": "arn:aws:cleanrooms-ml:id-mapping-workflow"},
					"arn:aws:kms:key/1", nil,
				)
				require.NoError(t, err)

				return func() {
					require.NoError(t, b.DeleteIDMappingTable(membershipID, table.ID))
				}
			},
		},
		{
			name: "idNamespaceAssociation",
			create: func(t *testing.T, b *cleanrooms.InMemoryBackend, membershipID string) func() {
				t.Helper()
				assoc, err := b.CreateIDNamespaceAssociation(
					membershipID, "id-namespace-1", "an id namespace association",
					map[string]any{"inputReferenceArn": "arn:aws:entityresolution:id-namespace"}, nil, nil,
				)
				require.NoError(t, err)

				return func() {
					require.NoError(t, b.DeleteIDNamespaceAssociation(membershipID, assoc.ID))
				}
			},
		},
		{
			name: "configuredAudienceModelAssociation",
			create: func(t *testing.T, b *cleanrooms.InMemoryBackend, membershipID string) func() {
				t.Helper()
				cama, err := b.CreateConfiguredAudienceModelAssociation(
					membershipID, "arn:aws:cleanrooms-ml:configured-audience-model/1", "cama-1",
					"a configured audience model association", true, nil,
				)
				require.NoError(t, err)

				return func() {
					require.NoError(t, b.DeleteConfiguredAudienceModelAssociation(membershipID, cama.ID))
				}
			},
		},
		{
			name: "intermediateTable",
			create: func(t *testing.T, b *cleanrooms.InMemoryBackend, membershipID string) func() {
				t.Helper()
				table, err := b.CreateIntermediateTable(
					membershipID, "intermediate-1", "an intermediate table", "arn:aws:kms:key/2",
					map[string]any{"sqlParameters": map[string]any{"queryString": "SELECT 1"}},
					30, nil,
				)
				require.NoError(t, err)

				return func() {
					require.NoError(t, b.DeleteIntermediateTable(membershipID, table.ID))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cleanrooms.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
			collab, err := b.CreateCollaboration(
				"collab-1", "a collab", "creator", []string{"CAN_QUERY"}, nil, "ENABLED", nil,
			)
			require.NoError(t, err)
			membership, err := b.CreateMembership(
				collab.ID, "ENABLED", []string{"CAN_QUERY"}, nil, nil, nil,
			)
			require.NoError(t, err)

			cleanup := tt.create(t, b, membership.ID)

			err = b.DeleteMembership(membership.ID)
			require.ErrorIs(t, err, cleanrooms.ErrConflict)

			cleanup()

			require.NoError(t, b.DeleteMembership(membership.ID))
		})
	}
}

func TestDeleteMembership_SucceedsWhenEmpty(t *testing.T) {
	t.Parallel()

	b := cleanrooms.NewInMemoryBackend(config.DefaultAccountID, config.DefaultRegion)
	collab, err := b.CreateCollaboration(
		"collab-1", "a collab", "creator", []string{"CAN_QUERY"}, nil, "ENABLED", nil,
	)
	require.NoError(t, err)
	membership, err := b.CreateMembership(collab.ID, "ENABLED", []string{"CAN_QUERY"}, nil, nil, nil)
	require.NoError(t, err)

	require.NoError(t, b.DeleteMembership(membership.ID))

	_, err = b.GetMembership(membership.ID)
	require.ErrorIs(t, err, cleanrooms.ErrNotFound)
}
