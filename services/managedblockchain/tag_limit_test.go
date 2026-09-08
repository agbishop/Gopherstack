package managedblockchain_test

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	managedblockchainsdk "github.com/aws/aws-sdk-go-v2/service/managedblockchain"
	"github.com/aws/aws-sdk-go-v2/service/managedblockchain/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/managedblockchain"
)

// manyTags returns n key-value pairs with unique keys.
func manyTags(n int) map[string]string {
	tags := make(map[string]string, n)
	for i := range n {
		tags[fmt.Sprintf("k%d", i)] = "v"
	}

	return tags
}

// TestTagResource_TooManyTags: TagResourceRequest.Tags reuses the
// InputTagMap shape (botocore managedblockchain 2018-09-24
// service-2.json.gz: "max": 50, "min": 0), and its own doc says "with an
// overall maximum of 50 tags added to each resource" -- the cap is on the
// resource's resulting tag count, not the request's tag count.
// TagResource's own deserializeOpError switch (aws-sdk-go-v2
// managedblockchain@v1.34.4 deserializers.go:3555)
// declares TooManyTagsException.
func TestTagResource_TooManyTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newSDKTestClient(t, h)
	ctx := t.Context()

	createOut, err := client.CreateNetwork(ctx, &managedblockchainsdk.CreateNetworkInput{
		ClientRequestToken: aws.String("tok-1"),
		Name:               aws.String("net1"),
		Framework:          types.FrameworkHyperledgerFabric,
		FrameworkVersion:   aws.String("1.4"),
		VotingPolicy: &types.VotingPolicy{
			ApprovalThresholdPolicy: &types.ApprovalThresholdPolicy{
				ThresholdPercentage:     aws.Int32(50),
				ProposalDurationInHours: aws.Int32(24),
				ThresholdComparator:     types.ThresholdComparatorGreaterThan,
			},
		},
		MemberConfiguration: &types.MemberConfiguration{
			Name: aws.String("m1"),
			FrameworkConfiguration: &types.MemberFrameworkConfiguration{
				Fabric: &types.MemberFabricConfiguration{
					AdminUsername: aws.String("admin"),
					AdminPassword: aws.String("Passw0rd!"),
				},
			},
		},
	})
	require.NoError(t, err)

	netOut, err := client.GetNetwork(ctx, &managedblockchainsdk.GetNetworkInput{
		NetworkId: createOut.NetworkId,
	})
	require.NoError(t, err)
	arn := *netOut.Network.Arn

	_, err = client.TagResource(ctx, &managedblockchainsdk.TagResourceInput{
		ResourceArn: aws.String(arn),
		Tags:        manyTags(51),
	})
	require.Error(t, err)

	var limitErr *types.TooManyTagsException
	require.ErrorAs(t, err, &limitErr,
		"expected *types.TooManyTagsException (TagResource's own declared type for the 50-tag max), got: %v", err)

	tagsOut, err := client.ListTagsForResource(ctx, &managedblockchainsdk.ListTagsForResourceInput{
		ResourceArn: aws.String(arn),
	})
	require.NoError(t, err)
	require.Empty(t, tagsOut.Tags, "a rejected TagResource must not mutate the resource's existing tags")
}

// TestTagResource_TooManyTags_Cumulative proves the check is on the
// resource's resulting total, not just the size of this request: a
// resource that already carries 45 tags rejects a request adding 10 more
// distinct keys (55 > 50), even though 10 alone is well under the cap.
func TestTagResource_TooManyTags_Cumulative(t *testing.T) {
	t.Parallel()

	b := managedblockchain.NewInMemoryBackend()
	n := b.AddNetworkInternal(testRegion, testAccountID, "net1")

	err := b.TagResource(n.Arn, manyTags(45))
	require.NoError(t, err)

	err = b.TagResource(n.Arn, func() map[string]string {
		tags := make(map[string]string, 10)
		for i := 45; i < 55; i++ {
			tags[fmt.Sprintf("k%d", i)] = "v"
		}

		return tags
	}())
	require.Error(t, err)
	require.ErrorIs(t, err, managedblockchain.ErrTooManyTags)

	tags, err := b.ListTagsForResource(n.Arn)
	require.NoError(t, err)
	require.Len(t, tags, 45, "a rejected cumulative TagResource must not mutate the resource's existing tags")
}

// TestCreateNetwork_TooManyTags: CreateNetworkInput.Tags reuses the same
// InputTagMap shape, and CreateNetwork's own deserializeOpError switch
// (deserializers.go:457) also declares TooManyTagsException.
func TestCreateNetwork_TooManyTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newSDKTestClient(t, h)
	ctx := t.Context()

	_, err := client.CreateNetwork(ctx, &managedblockchainsdk.CreateNetworkInput{
		ClientRequestToken: aws.String("tok-1"),
		Name:               aws.String("net1"),
		Tags:               manyTags(51),
		Framework:          types.FrameworkHyperledgerFabric,
		FrameworkVersion:   aws.String("1.4"),
		VotingPolicy: &types.VotingPolicy{
			ApprovalThresholdPolicy: &types.ApprovalThresholdPolicy{
				ThresholdPercentage:     aws.Int32(50),
				ProposalDurationInHours: aws.Int32(24),
				ThresholdComparator:     types.ThresholdComparatorGreaterThan,
			},
		},
		MemberConfiguration: &types.MemberConfiguration{
			Name: aws.String("m1"),
			FrameworkConfiguration: &types.MemberFrameworkConfiguration{
				Fabric: &types.MemberFabricConfiguration{
					AdminUsername: aws.String("admin"),
					AdminPassword: aws.String("Passw0rd!"),
				},
			},
		},
	})
	require.Error(t, err)

	var limitErr *types.TooManyTagsException
	require.ErrorAs(t, err, &limitErr,
		"expected *types.TooManyTagsException, got: %v", err)
}

// TestCreateOps_TooManyTags exercises every remaining Create* op that
// accepts an initial Tags map and declares TooManyTagsException on its own
// deserializeOpError switch (aws-sdk-go-v2 managedblockchain@v1.34.4
// deserializers.go): CreateMember (via nested MemberConfiguration.Tags,
// deserializers.go:277), CreateNode (deserializers.go:640), CreateProposal
// (deserializers.go:820), and CreateAccessor (deserializers.go:85). Each
// op's own initial tag set has no pre-existing tags to add to, so
// len(tags) > 50 alone crosses the per-resource cap.
func TestCreateOps_TooManyTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		call func(b *managedblockchain.InMemoryBackend) error
		name string
	}{
		{
			name: "create member",
			call: func(b *managedblockchain.InMemoryBackend) error {
				n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
				inv := b.AddInvitationInternal(testRegion, testAccountID, n.ID, n.Name)
				_, err := b.CreateMember(
					testRegion, testAccountID, n.ID, inv.InvitationID, "m1", "", "admin", "", manyTags(51))

				return err
			},
		},
		{
			name: "create node",
			call: func(b *managedblockchain.InMemoryBackend) error {
				n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
				m := b.AddMemberInternal(testRegion, testAccountID, n.ID, "m1")
				_, err := b.CreateNode(
					testRegion, testAccountID, n.ID, m.ID, "bc.t3.small.ethereum", "", "", manyTags(51))

				return err
			},
		},
		{
			name: "create proposal",
			call: func(b *managedblockchain.InMemoryBackend) error {
				n := b.AddNetworkInternal(testRegion, testAccountID, "net1")
				m := b.AddMemberInternal(testRegion, testAccountID, n.ID, "m1")
				_, err := b.CreateProposal(
					testRegion, testAccountID, n.ID, m.ID, "desc", nil, manyTags(51))

				return err
			},
		},
		{
			name: "create accessor",
			call: func(b *managedblockchain.InMemoryBackend) error {
				_, err := b.CreateAccessor(testRegion, testAccountID, "BILLING_TOKEN", "ETHEREUM_MAINNET", manyTags(51))

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := managedblockchain.NewInMemoryBackend()
			err := tt.call(b)
			require.Error(t, err)
			require.ErrorIs(t, err, managedblockchain.ErrTooManyTags)
		})
	}
}

// TestCreateNetwork_TooManyMemberTags isolates the nested
// MemberConfiguration.Tags check: the network-level Tags stay well under
// the cap so only the member's tag set can trip the error.
func TestCreateNetwork_TooManyMemberTags(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newSDKTestClient(t, h)
	ctx := t.Context()

	_, err := client.CreateNetwork(ctx, &managedblockchainsdk.CreateNetworkInput{
		ClientRequestToken: aws.String("tok-1"),
		Name:               aws.String("net1"),
		Tags:               manyTags(1),
		Framework:          types.FrameworkHyperledgerFabric,
		FrameworkVersion:   aws.String("1.4"),
		VotingPolicy: &types.VotingPolicy{
			ApprovalThresholdPolicy: &types.ApprovalThresholdPolicy{
				ThresholdPercentage:     aws.Int32(50),
				ProposalDurationInHours: aws.Int32(24),
				ThresholdComparator:     types.ThresholdComparatorGreaterThan,
			},
		},
		MemberConfiguration: &types.MemberConfiguration{
			Name: aws.String("m1"),
			Tags: manyTags(51),
			FrameworkConfiguration: &types.MemberFrameworkConfiguration{
				Fabric: &types.MemberFabricConfiguration{
					AdminUsername: aws.String("admin"),
					AdminPassword: aws.String("Passw0rd!"),
				},
			},
		},
	})
	require.Error(t, err)

	var limitErr *types.TooManyTagsException
	require.ErrorAs(t, err, &limitErr,
		"expected *types.TooManyTagsException from the nested MemberConfiguration.Tags check, got: %v", err)

	listOut, err := client.ListNetworks(ctx, &managedblockchainsdk.ListNetworksInput{})
	require.NoError(t, err)
	require.Empty(t, listOut.Networks, "a rejected CreateNetwork must not create a network")
}

// TestTagResource_ExactlyFiftyTags proves the boundary itself is not
// rejected: exactly 50 tags on a previously untagged resource must succeed
// (botocore InputTagMap "max": 50 permits 50, only 51+ crosses it).
func TestTagResource_ExactlyFiftyTags(t *testing.T) {
	t.Parallel()

	b := managedblockchain.NewInMemoryBackend()
	n := b.AddNetworkInternal(testRegion, testAccountID, "net1")

	err := b.TagResource(n.Arn, manyTags(50))
	require.NoError(t, err)

	tags, err := b.ListTagsForResource(n.Arn)
	require.NoError(t, err)
	require.Len(t, tags, 50)
}
