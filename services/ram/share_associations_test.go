package ram_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ram"
)

// TestDisassociateResourceShare_SoftDeletesInPlace verifies that DisassociateResourceShare
// marks matching rows DISASSOCIATED rather than removing them from the backend's
// association slice -- the same soft-delete pattern DeleteResourceShare already used, so
// GetResourceShareAssociations(associationStatus=DISASSOCIATED) can see the history and a
// later AssociateResourceShare can reactivate the row.
func TestDisassociateResourceShare_SoftDeletesInPlace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		initials          []string
		toDisassoc        []string
		wantAssociated    int
		wantDisassociated int
	}{
		{
			name:              "disassociate one of two principals",
			initials:          []string{"111111111111", "222222222222"},
			toDisassoc:        []string{"111111111111"},
			wantAssociated:    1,
			wantDisassociated: 1,
		},
		{
			name:              "disassociate all principals",
			initials:          []string{"111111111111", "222222222222"},
			toDisassoc:        []string{"111111111111", "222222222222"},
			wantAssociated:    0,
			wantDisassociated: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ram.NewInMemoryBackend("000000000000", "us-east-1")
			rs, err := b.CreateResourceShare("disassoc-test", true, nil, tt.initials, nil)
			require.NoError(t, err)

			_, err = b.DisassociateResourceShare(rs.ARN, tt.toDisassoc, nil)
			require.NoError(t, err)

			// The row count is unchanged (soft delete): every initial principal still
			// has a row, split between ASSOCIATED and DISASSOCIATED.
			assocs := b.GetResourceShareAssociations("PRINCIPAL", []string{rs.ARN})
			assert.Len(t, assocs, len(tt.initials))
			assert.Equal(t, tt.wantAssociated, countByStatus(assocs, "ASSOCIATED"))
			assert.Equal(t, tt.wantDisassociated, countByStatus(assocs, "DISASSOCIATED"))
		})
	}
}

// TestAssociateResourceShare_ReactivatesDisassociatedEntity verifies that re-associating
// an entity that was previously disassociated flips its existing row back to ASSOCIATED
// in place, instead of accumulating a duplicate row.
func TestAssociateResourceShare_ReactivatesDisassociatedEntity(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")
	rs, err := b.CreateResourceShare("reactivate-test", true, nil, []string{"111111111111"}, nil)
	require.NoError(t, err)

	_, err = b.DisassociateResourceShare(rs.ARN, []string{"111111111111"}, nil)
	require.NoError(t, err)

	assocs := b.GetResourceShareAssociations("PRINCIPAL", []string{rs.ARN})
	require.Len(t, assocs, 1)
	assert.Equal(t, "DISASSOCIATED", assocs[0].Status)

	added, err := b.AssociateResourceShare(rs.ARN, []string{"111111111111"}, nil)
	require.NoError(t, err)
	require.Len(t, added, 1)
	assert.Equal(t, "ASSOCIATED", added[0].Status)

	// Still exactly one row for this entity: the prior row was reactivated in place,
	// not duplicated.
	assocs = b.GetResourceShareAssociations("PRINCIPAL", []string{rs.ARN})
	require.Len(t, assocs, 1)
	assert.Equal(t, "ASSOCIATED", assocs[0].Status)
	assert.Equal(t, 1, ram.AssociationCount(b))
}

// countByStatus returns how many associations in assocs have the given status.
func countByStatus(assocs []*ram.ResourceShareAssociation, status string) int {
	n := 0

	for _, a := range assocs {
		if a.Status == status {
			n++
		}
	}

	return n
}

func TestAssociateResourceShare_Idempotent(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")
	rs, err := b.CreateResourceShare("idem-share", true, nil, nil, nil)
	require.NoError(t, err)

	// First association creates one entry.
	added1, err := b.AssociateResourceShare(rs.ARN, []string{"111111111111"}, nil)
	require.NoError(t, err)
	assert.Len(t, added1, 1)

	// Repeated association with the same entity must be a no-op (no new entry).
	added2, err := b.AssociateResourceShare(rs.ARN, []string{"111111111111"}, nil)
	require.NoError(t, err)
	assert.Empty(t, added2, "re-associating the same entity must return no new associations")

	// Exactly one association should exist in total.
	assocs := b.GetResourceShareAssociations("PRINCIPAL", []string{rs.ARN})
	assert.Len(t, assocs, 1)
}

func TestAllowExternalPrincipals_FalseRejectsExternal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		principal string
		wantErr   bool
	}{
		{
			name:      "own account allowed when flag false",
			principal: "000000000000",
			wantErr:   false,
		},
		{
			name:      "external account ID rejected when flag false",
			principal: "999999999999",
			wantErr:   true,
		},
		{
			name:      "IAM role ARN rejected when flag false",
			principal: "arn:aws:iam::111111111111:role/MyRole",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ram.NewInMemoryBackend("000000000000", "us-east-1")
			rs, err := b.CreateResourceShare("restrict-share", false, nil, nil, nil)
			require.NoError(t, err)

			_, err = b.AssociateResourceShare(rs.ARN, []string{tt.principal}, nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ram.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestAccuracy2_AssociateResourceShare_RejectedExternalPrincipalLeavesNoOrphan verifies
// that AssociateResourceShare does not commit associations for principals
// processed before a later external-principal rejection.
func TestAssociateResourceShare_RejectedExternalPrincipalLeavesNoOrphan(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")
	rs, err := b.CreateResourceShare("assoc-no-orphan", false, nil, nil, nil)
	require.NoError(t, err)

	_, err = b.AssociateResourceShare(
		rs.ARN,
		[]string{"000000000000", "999999999999"},
		nil,
	)
	require.ErrorIs(t, err, ram.ErrValidation)

	assocs := b.GetResourceShareAssociations("PRINCIPAL", []string{rs.ARN})
	assert.Empty(t, assocs, "rejected AssociateResourceShare must not commit any associations")

	invs := b.GetResourceShareInvitations(nil, []string{rs.ARN})
	assert.Empty(t, invs, "rejected AssociateResourceShare must not create invitations")
}

// TestRefinement1_AssociateResourceShare_External verifies that principals with non-account-ID
// format (e.g. ARNs) are flagged as external.
func TestAssociateResourceShare_External(t *testing.T) {
	t.Parallel()

	tests := []struct {
		principal    string
		name         string
		wantExternal bool
	}{
		{
			name:         "own account - not external",
			principal:    "000000000000",
			wantExternal: false,
		},
		{
			name:         "other account - external",
			principal:    "999999999999",
			wantExternal: true,
		},
		{
			name:         "iam role ARN - external",
			principal:    "arn:aws:iam::111111111111:role/MyRole",
			wantExternal: true,
		},
		{
			name:         "iam role ARN same account - not external",
			principal:    "arn:aws:iam::000000000000:role/MyRole",
			wantExternal: false,
		},
		{
			name:         "iam user ARN same account - not external",
			principal:    "arn:aws:iam::000000000000:user/MyUser",
			wantExternal: false,
		},
		{
			name:         "organization ARN same account segment - still external",
			principal:    "arn:aws:organizations::000000000000:organization/o-exampleorgid",
			wantExternal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ram.NewInMemoryBackend("000000000000", "us-east-1")

			rs, err := b.CreateResourceShare("share-"+tt.name, true, nil, nil, nil)
			require.NoError(t, err)

			assocs, err := b.AssociateResourceShare(rs.ARN, []string{tt.principal}, nil)
			require.NoError(t, err)
			require.Len(t, assocs, 1)
			assert.Equal(t, tt.wantExternal, assocs[0].External)
		})
	}
}

// TestRefinement1_GetResourceShareAssociations_TypeFilter verifies type filtering.
func TestGetResourceShareAssociations_TypeFilter(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")

	rs, err := b.CreateResourceShare(
		"assoc-filter", false, nil,
		[]string{"000000000000"},
		[]string{"arn:aws:ec2:us-east-1:000000000000:subnet/sub-1"},
	)
	require.NoError(t, err)

	principals := b.GetResourceShareAssociations("PRINCIPAL", []string{rs.ARN})
	assert.Len(t, principals, 1)
	assert.Equal(t, "PRINCIPAL", principals[0].AssociationType)

	resources := b.GetResourceShareAssociations("RESOURCE", []string{rs.ARN})
	assert.Len(t, resources, 1)
	assert.Equal(t, "RESOURCE", resources[0].AssociationType)
}

// TestRefinement1_ResourceShareAssociation_DuplicateIdempotent verifies that associating
// the same entity twice is a no-op.
func TestResourceShareAssociation_DuplicateIdempotent(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")

	rs, err := b.CreateResourceShare("idem-share", false, nil, nil, nil)
	require.NoError(t, err)

	_, err = b.AssociateResourceShare(rs.ARN, []string{"000000000000"}, nil)
	require.NoError(t, err)

	// Second association of same entity should not duplicate.
	added, err := b.AssociateResourceShare(rs.ARN, []string{"000000000000"}, nil)
	require.NoError(t, err)
	assert.Empty(t, added, "duplicate association should not add a new entry")
	assert.Equal(t, 1, ram.AssociationCount(b))
}
