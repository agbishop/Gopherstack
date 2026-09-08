package ram_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ram"
)

const testExternalAcct = "999999999999"

func externalPrincipalAssociation(
	t *testing.T, b *ram.InMemoryBackend, shareARN string,
) *ram.ResourceShareAssociation {
	t.Helper()

	assocs := b.GetResourceShareAssociations("PRINCIPAL", []string{shareARN})
	for _, a := range assocs {
		if a.AssociatedEntity == testExternalAcct {
			return a
		}
	}

	t.Fatalf("no association found for principal %s on share %s", testExternalAcct, shareARN)

	return nil
}

func TestRejectResourceShareInvitation_DisassociatesReceiverPrincipal(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")
	rs, err := b.CreateResourceShare("reject-share", true, nil, nil, nil)
	require.NoError(t, err)

	_, err = b.AssociateResourceShare(rs.ARN, []string{testExternalAcct}, nil)
	require.NoError(t, err)

	assoc := externalPrincipalAssociation(t, b, rs.ARN)
	require.Equal(t, "ASSOCIATED", assoc.Status,
		"AssociateResourceShare must associate the external principal immediately, pending invitation acceptance")

	invs := b.GetResourceShareInvitations(nil, []string{rs.ARN})
	require.Len(t, invs, 1)

	_, err = b.RejectResourceShareInvitation(invs[0].InvitationARN)
	require.NoError(t, err)

	got := externalPrincipalAssociation(t, b, rs.ARN)
	assert.Equal(t, "DISASSOCIATED", got.Status,
		"rejecting the invitation must disassociate the receiver's principal -- "+
			"a rejected invitation must not leave the principal permanently ASSOCIATED")
}

func TestAcceptResourceShareInvitation_LeavesPrincipalAssociated(t *testing.T) {
	t.Parallel()

	b := ram.NewInMemoryBackend("000000000000", "us-east-1")
	rs, err := b.CreateResourceShare("accept-share", true, nil, nil, nil)
	require.NoError(t, err)

	_, err = b.AssociateResourceShare(rs.ARN, []string{testExternalAcct}, nil)
	require.NoError(t, err)

	invs := b.GetResourceShareInvitations(nil, []string{rs.ARN})
	require.Len(t, invs, 1)

	_, err = b.AcceptResourceShareInvitation(invs[0].InvitationARN)
	require.NoError(t, err)

	got := externalPrincipalAssociation(t, b, rs.ARN)
	assert.Equal(t, "ASSOCIATED", got.Status, "accepting must leave the principal associated")
}

func TestInvitation_LazyExpiry(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := ram.NewInMemoryBackend("000000000000", "us-east-1")
		rs, err := b.CreateResourceShare("expiry-share", true, nil, nil, nil)
		require.NoError(t, err)

		_, err = b.AssociateResourceShare(rs.ARN, []string{testExternalAcct}, nil)
		require.NoError(t, err)

		invsBefore := b.GetResourceShareInvitations(nil, []string{rs.ARN})
		require.Len(t, invsBefore, 1)
		require.Equal(t, "PENDING", invsBefore[0].Status)

		time.Sleep(12*time.Hour + time.Second)

		invsAfter := b.GetResourceShareInvitations(nil, []string{rs.ARN})
		require.Len(t, invsAfter, 1)
		assert.Equal(t, "EXPIRED", invsAfter[0].Status,
			"a PENDING invitation older than the expiry window must lazily transition to EXPIRED on read")

		assoc := externalPrincipalAssociation(t, b, rs.ARN)
		assert.Equal(t, "DISASSOCIATED", assoc.Status,
			"an expired invitation must disassociate the receiver's principal, matching documented AWS behavior")

		_, err = b.AcceptResourceShareInvitation(invsBefore[0].InvitationARN)
		require.ErrorIs(t, err, ram.ErrInvitationExpired,
			"accepting a lazily-expired invitation must fail with ErrInvitationExpired")
	})
}

func TestRejectResourceShareInvitation_AlreadyExpiredReturnsExpiredError(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		b := ram.NewInMemoryBackend("000000000000", "us-east-1")
		rs, err := b.CreateResourceShare("expiry-reject-share", true, nil, nil, nil)
		require.NoError(t, err)

		invs := b.GetResourceShareInvitations(nil, nil)
		require.Empty(t, invs)

		_, err = b.AssociateResourceShare(rs.ARN, []string{"888888888888"}, nil)
		require.NoError(t, err)

		invs = b.GetResourceShareInvitations(nil, []string{rs.ARN})
		require.Len(t, invs, 1)

		time.Sleep(12*time.Hour + time.Second)

		// First call transitions PENDING -> EXPIRED lazily and reports it.
		_, err = b.RejectResourceShareInvitation(invs[0].InvitationARN)
		require.Error(t, err)
		require.ErrorIs(t, err, ram.ErrInvitationExpired)

		// A second call must hit the already-EXPIRED branch, not re-run the lazy check.
		_, err = b.RejectResourceShareInvitation(invs[0].InvitationARN)
		require.ErrorIs(t, err, ram.ErrInvitationExpired)
	})
}
