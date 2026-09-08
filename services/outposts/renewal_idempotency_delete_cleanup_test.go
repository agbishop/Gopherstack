package outposts_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	outpostssdk "github.com/aws/aws-sdk-go-v2/service/outposts"
	"github.com/aws/aws-sdk-go-v2/service/outposts/types"
	"github.com/stretchr/testify/require"
)

// TestDeleteOutpost_PrunesRenewalIdempotencyCache guards against a leak:
// renewalIdempotency (renewals.go) caches one entry per (Outpost,
// ClientToken) CreateRenewal call, keyed "outpostID::clientToken", but was
// never cleared on DeleteOutpost -- see gopherstack-vsmv.
func TestDeleteOutpost_PrunesRenewalIdempotencyCache(t *testing.T) {
	t.Parallel()

	h, client := newTestHandlerAndClient(t)
	siteID := createTestSite(t, client)
	created := createTestOutpost(t, client, siteID)

	_, err := client.CreateRenewal(t.Context(), &outpostssdk.CreateRenewalInput{
		OutpostIdentifier: created.OutpostId,
		PaymentOption:     types.PaymentOptionAllUpfront,
		PaymentTerm:       types.PaymentTermOneYear,
		ClientToken:       aws.String("prune-test-token"),
	})
	require.NoError(t, err)
	require.Equal(t, 1, h.Backend.RenewalIdempotencyLenForTest())

	_, err = client.DeleteOutpost(t.Context(), &outpostssdk.DeleteOutpostInput{
		OutpostId: created.OutpostId,
	})
	require.NoError(t, err)

	require.Equal(t, 0, h.Backend.RenewalIdempotencyLenForTest(),
		"DeleteOutpost must prune renewalIdempotency entries for its own Outpost, not leak them")
}
