package ram_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ram"
)

func TestBackend_GetResourceShare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(*testing.T, *ram.InMemoryBackend) string
		name    string
		wantErr bool
	}{
		{
			name: "found",
			setup: func(t *testing.T, b *ram.InMemoryBackend) string {
				t.Helper()
				rs, err := b.CreateResourceShare("found-share", true, nil, nil, nil)
				require.NoError(t, err)

				return rs.ARN
			},
		},
		{
			name: "not found",
			setup: func(_ *testing.T, _ *ram.InMemoryBackend) string {
				return "arn:aws:ram:us-east-1:000000000000:resource-share/missing"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ram.NewInMemoryBackend("000000000000", "us-east-1")
			shareARN := tt.setup(t, b)

			rs, err := b.GetResourceShare(shareARN)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, shareARN, rs.ARN)
		})
	}
}

func TestDeleteResourceShare_RemovesFromMemory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		principals []string
	}{
		{
			name:       "share with associations is fully removed",
			principals: []string{"123456789012"},
		},
		{
			name:       "share without associations is fully removed",
			principals: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ram.NewInMemoryBackend("000000000000", "us-east-1")
			rs, err := b.CreateResourceShare("del-test", true, nil, tt.principals, nil)
			require.NoError(t, err)

			err = b.DeleteResourceShare(rs.ARN)
			require.NoError(t, err)

			// Share should not be retrievable.
			_, err = b.GetResourceShare(rs.ARN)
			require.Error(t, err)

			// Associations for the deleted share should be DISASSOCIATED (AWS soft-deletes them).
			assocs := b.GetResourceShareAssociations("", []string{rs.ARN})
			for _, a := range assocs {
				assert.Equal(t, "DISASSOCIATED", a.Status, "associations for deleted share should be DISASSOCIATED")
			}
		})
	}
}

func TestAllowExternalPrincipals_FalseRejectsOnCreate(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateResourceShare(
		"restrict-create",
		false,
		nil,
		[]string{"999999999999"},
		nil,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ram.ErrValidation)
}

// TestAllowExternalPrincipals_FalseAllowsSameAccountIAMPrincipal verifies that a
// same-account IAM role/user ARN is never treated as an external principal: real AWS's
// AllowExternalPrincipals gates other AWS accounts (api_op_CreateResourceShare.go:
// "principals outside your organization"), not in-account IAM identities, and
// AssociateResourceShare's Principals doc lists IAM role/user ARNs as their own
// principal kind, always scoped to the resource share's own account.
func TestAllowExternalPrincipals_FalseAllowsSameAccountIAMPrincipal(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")

	rs, err := b.CreateResourceShare(
		"same-account-iam-principal",
		false,
		nil,
		[]string{"arn:aws:iam::000000000000:role/MyRole"},
		nil,
	)
	require.NoError(t, err)

	assocs := b.GetResourceShareAssociations("PRINCIPAL", []string{rs.ARN})
	require.Len(t, assocs, 1)
	assert.False(t, assocs[0].External, "same-account IAM role ARN must not be flagged external")

	invs := b.GetResourceShareInvitations(nil, []string{rs.ARN})
	assert.Empty(t, invs, "a same-account principal must not generate a pending invitation")
}

// TestAccuracy2_CreateResourceShare_RejectedExternalPrincipalLeavesNoOrphan verifies
// that a CreateResourceShare call rejected for an external principal (when
// AllowExternalPrincipals is false) does not leave a partially created resource
// share behind. Previously the share (and any associations for principals
// processed before the rejected one) were committed before validation ran,
// so a failed call still left orphaned state -- including reserving the
// share name, which made every retry with the same name fail with
// ResourceShareAlreadyExistsException.
func TestCreateResourceShare_RejectedExternalPrincipalLeavesNoOrphan(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateResourceShare(
		"no-orphan-create",
		false,
		nil,
		[]string{"000000000000", "999999999999"},
		nil,
	)
	require.ErrorIs(t, err, ram.ErrValidation)

	shares := b.ListResourceShares("SELF", "")
	assert.Empty(t, shares, "rejected CreateResourceShare must not leave an orphaned share")

	// Retrying with the same name must succeed -- it would previously fail
	// with ResourceShareAlreadyExistsException because the first (failed)
	// call had already reserved the name.
	rs, err := b.CreateResourceShare("no-orphan-create", true, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "no-orphan-create", rs.Name)
}

// TestRefinement1_DeleteResourceShare_SoftDelete verifies that a deleted resource share
// is marked DELETED (not removed from the map) and associated entities become DISASSOCIATED.
func TestDeleteResourceShare_SoftDelete(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")

	rs, err := b.CreateResourceShare("soft-share", true, nil, []string{"999999999999"}, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, ram.AssociationCount(b))

	err = b.DeleteResourceShare(rs.ARN)
	require.NoError(t, err)

	// Share record still exists (DELETED status).
	assert.Equal(t, 1, ram.ResourceShareCount(b))

	// GetResourceShare should return NotFound for deleted shares.
	_, err = b.GetResourceShare(rs.ARN)
	require.ErrorIs(t, err, ram.ErrNotFound)

	// Double-delete returns NotFound.
	err = b.DeleteResourceShare(rs.ARN)
	require.ErrorIs(t, err, ram.ErrNotFound)
}

// TestRefinement1_ListResourceShares_StatusFilter verifies status filtering.
func TestListResourceShares_StatusFilter(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")

	rs, err := b.CreateResourceShare("share-active", false, nil, nil, nil)
	require.NoError(t, err)

	// Delete one share → DELETED status.
	err = b.DeleteResourceShare(rs.ARN)
	require.NoError(t, err)

	_, err = b.CreateResourceShare("share-active-2", false, nil, nil, nil)
	require.NoError(t, err)

	// Status="ACTIVE" should return only the non-deleted share.
	active := b.ListResourceShares("SELF", "ACTIVE")
	assert.Len(t, active, 1)
	assert.Equal(t, "share-active-2", active[0].Name)

	// Status="" returns all non-deleted.
	all := b.ListResourceShares("SELF", "")
	assert.Len(t, all, 1)
}

// TestRefinement1_ListResourceShares_SortedByName verifies output is sorted.
func TestListResourceShares_SortedByName(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")

	for _, name := range []string{"zz-share", "aa-share", "mm-share"} {
		_, err := b.CreateResourceShare(name, false, nil, nil, nil)
		require.NoError(t, err)
	}

	list := b.ListResourceShares("SELF", "")
	require.Len(t, list, 3)
	assert.Equal(t, "aa-share", list[0].Name)
	assert.Equal(t, "mm-share", list[1].Name)
	assert.Equal(t, "zz-share", list[2].Name)
}

// TestRefinement1_AddResourceShareInternal verifies seed helper works.
func TestAddResourceShareInternal(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")
	rs := ram.NewTestResourceShare("arn:aws:ram:us-east-1:000000000000:resource-share/seed-1", "seed-share")
	ram.AddResourceShareInternal(b, rs)
	assert.Equal(t, 1, ram.ResourceShareCount(b))
}

// TestRefinement1_ErrAlreadyExists_ShareName verifies name collision on CreateResourceShare.
func TestErrAlreadyExists_ShareName(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")

	_, err := b.CreateResourceShare("duplicate", false, nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateResourceShare("duplicate", false, nil, nil, nil)
	require.ErrorIs(t, err, ram.ErrAlreadyExists)
}

// TestRefinement1_UpdateResourceShare_SyncAssocName verifies that updating the share name
// is propagated to existing associations.
func TestUpdateResourceShare_SyncAssocName(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")

	rs, err := b.CreateResourceShare("old-name", false, nil, []string{"000000000000"}, nil)
	require.NoError(t, err)

	_, err = b.UpdateResourceShare(rs.ARN, "new-name", nil)
	require.NoError(t, err)

	assocs := b.GetResourceShareAssociations("PRINCIPAL", []string{rs.ARN})
	require.Len(t, assocs, 1)
	assert.Equal(t, "new-name", assocs[0].ResourceShareName)
}
