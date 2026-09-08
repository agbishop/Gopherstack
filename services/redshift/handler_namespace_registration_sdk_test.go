package redshift_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	redshiftsdk "github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/redshift"
)

// TestSDKRoundTrip_RegisterNamespace locks the fix for gopherstack-3jqz:
// RegisterNamespace/DeregisterNamespace took `_ url.Values` -- the entire
// request, including required ConsumerIdentifiers and NamespaceIdentifier
// (redshift@v1.65.4 api_op_RegisterNamespace.go:33,41) -- and returned static
// XML with no state change. Driving the real SDK client is what proves this:
// unfixed code accepted ANY NamespaceIdentifier unconditionally (even one
// naming a cluster that doesn't exist), where real AWS -- and this fix --
// rejects it with the operation's own declared faults
// (awsAwsquery_deserializeOpErrorRegisterNamespace: ClusterNotFound,
// InvalidClusterState, InvalidNamespaceFault).
func TestSDKRoundTrip_RegisterNamespace(t *testing.T) {
	t.Parallel()

	t.Run("cluster identifier registers a real, available cluster", func(t *testing.T) {
		t.Parallel()

		backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
		h := redshift.NewHandler(backend)
		client := newTestRedshiftClient(t, h)
		ctx := t.Context()

		_, err := backend.CreateCluster("rt-ns-cluster1", "dc2.large", "dev", "admin", nil, "")
		require.NoError(t, err)

		out, err := client.RegisterNamespace(ctx, &redshiftsdk.RegisterNamespaceInput{
			ConsumerIdentifiers: []string{"111111111111"},
			NamespaceIdentifier: &types.NamespaceIdentifierUnionMemberProvisionedIdentifier{
				Value: types.ProvisionedIdentifier{ClusterIdentifier: aws.String("rt-ns-cluster1")},
			},
		})
		require.NoError(t, err)
		assert.Equal(t, types.NamespaceRegistrationStatusRegistering, out.Status)
	})

	t.Run("nonexistent cluster identifier is rejected, not silently accepted", func(t *testing.T) {
		t.Parallel()

		backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
		h := redshift.NewHandler(backend)
		client := newTestRedshiftClient(t, h)
		ctx := t.Context()

		_, err := client.RegisterNamespace(ctx, &redshiftsdk.RegisterNamespaceInput{
			ConsumerIdentifiers: []string{"111111111111"},
			NamespaceIdentifier: &types.NamespaceIdentifierUnionMemberProvisionedIdentifier{
				Value: types.ProvisionedIdentifier{ClusterIdentifier: aws.String("rt-does-not-exist")},
			},
		})
		require.Error(t, err)

		var notFound *types.ClusterNotFoundFault
		require.ErrorAs(t, err, &notFound, "want ClusterNotFoundFault, got %v", err)
	})

	t.Run("cluster not in available state is rejected", func(t *testing.T) {
		t.Parallel()

		backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
		h := redshift.NewHandler(backend)
		client := newTestRedshiftClient(t, h)
		ctx := t.Context()

		_, err := backend.CreateCluster("rt-ns-cluster2", "dc2.large", "dev", "admin", nil, "")
		require.NoError(t, err)
		_, err = backend.PauseCluster("rt-ns-cluster2")
		require.NoError(t, err)

		_, err = client.RegisterNamespace(ctx, &redshiftsdk.RegisterNamespaceInput{
			ConsumerIdentifiers: []string{"111111111111"},
			NamespaceIdentifier: &types.NamespaceIdentifierUnionMemberProvisionedIdentifier{
				Value: types.ProvisionedIdentifier{ClusterIdentifier: aws.String("rt-ns-cluster2")},
			},
		})
		require.Error(t, err)

		var invalidState *types.InvalidClusterStateFault
		require.ErrorAs(t, err, &invalidState, "want InvalidClusterStateFault, got %v", err)
	})

	t.Run("serverless identifier registers a real namespace and workgroup", func(t *testing.T) {
		t.Parallel()

		backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
		h := redshift.NewHandler(backend)
		client := newTestRedshiftClient(t, h)
		ctx := t.Context()

		_, err := backend.CreateNamespace(redshift.CreateNamespaceParams{NamespaceName: "rt-ns-sl1"})
		require.NoError(t, err)
		_, err = backend.CreateWorkgroup("rt-wg-sl1", "rt-ns-sl1", redshift.WorkgroupParams{}, nil)
		require.NoError(t, err)

		out, err := client.RegisterNamespace(ctx, &redshiftsdk.RegisterNamespaceInput{
			ConsumerIdentifiers: []string{"111111111111"},
			NamespaceIdentifier: &types.NamespaceIdentifierUnionMemberServerlessIdentifier{
				Value: types.ServerlessIdentifier{
					NamespaceIdentifier: aws.String("rt-ns-sl1"),
					WorkgroupIdentifier: aws.String("rt-wg-sl1"),
				},
			},
		})
		require.NoError(t, err)
		assert.Equal(t, types.NamespaceRegistrationStatusRegistering, out.Status)
	})

	t.Run("serverless identifier naming an unknown namespace is rejected", func(t *testing.T) {
		t.Parallel()

		backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
		h := redshift.NewHandler(backend)
		client := newTestRedshiftClient(t, h)
		ctx := t.Context()

		_, err := client.RegisterNamespace(ctx, &redshiftsdk.RegisterNamespaceInput{
			ConsumerIdentifiers: []string{"111111111111"},
			NamespaceIdentifier: &types.NamespaceIdentifierUnionMemberServerlessIdentifier{
				Value: types.ServerlessIdentifier{
					NamespaceIdentifier: aws.String("rt-does-not-exist"),
					WorkgroupIdentifier: aws.String("rt-wg-does-not-exist"),
				},
			},
		})
		require.Error(t, err)

		var invalidNS *types.InvalidNamespaceFault
		require.ErrorAs(t, err, &invalidNS, "want InvalidNamespaceFault, got %v", err)
	})

	t.Run("deregister reports the Deregistering status", func(t *testing.T) {
		t.Parallel()

		backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)
		h := redshift.NewHandler(backend)
		client := newTestRedshiftClient(t, h)
		ctx := t.Context()

		_, err := backend.CreateCluster("rt-ns-cluster3", "dc2.large", "dev", "admin", nil, "")
		require.NoError(t, err)

		namespaceID := &types.NamespaceIdentifierUnionMemberProvisionedIdentifier{
			Value: types.ProvisionedIdentifier{ClusterIdentifier: aws.String("rt-ns-cluster3")},
		}

		_, err = client.RegisterNamespace(ctx, &redshiftsdk.RegisterNamespaceInput{
			ConsumerIdentifiers: []string{"111111111111"},
			NamespaceIdentifier: namespaceID,
		})
		require.NoError(t, err)

		out, err := client.DeregisterNamespace(ctx, &redshiftsdk.DeregisterNamespaceInput{
			ConsumerIdentifiers: []string{"111111111111"},
			NamespaceIdentifier: namespaceID,
		})
		require.NoError(t, err)
		assert.Equal(t, types.NamespaceRegistrationStatusDeregistering, out.Status)
	})
}

// TestNamespaceRegistration_ConsumerIdentifiersStateMutation drives the
// backend directly (not over HTTP) to prove RegisterNamespace/
// DeregisterNamespace mutate real, observable ConsumerIdentifiers state --
// unlike the SDK-level tests above, there is no describe/list operation in
// this SDK version for a client to independently re-read the registration
// afterward, so each call's own returned *NamespaceRegistration is the only
// place that state is observable. Unfixed code (a static XML response
// ignoring the request) would return the same canned Status regardless of
// which consumers were registered/deregistered; this asserts the actual
// consumer set changes.
func TestNamespaceRegistration_ConsumerIdentifiersStateMutation(t *testing.T) {
	t.Parallel()

	backend := redshift.NewInMemoryBackend("000000000000", rtTestRegion)

	_, err := backend.CreateCluster("rt-ns-mutation", "dc2.large", "dev", "admin", nil, "")
	require.NoError(t, err)

	reg, err := backend.RegisterNamespace([]string{"111111111111", "222222222222"}, "rt-ns-mutation", "", "")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"111111111111", "222222222222"}, reg.ConsumerIdentifiers)
	assert.Equal(t, "Registering", reg.Status)

	reg, err = backend.DeregisterNamespace([]string{"111111111111"}, "rt-ns-mutation", "", "")
	require.NoError(t, err)
	assert.Equal(
		t,
		[]string{"222222222222"},
		reg.ConsumerIdentifiers,
		"deregistering one consumer must not drop the other",
	)
	assert.Equal(t, "Deregistering", reg.Status)

	reg, err = backend.DeregisterNamespace([]string{"222222222222"}, "rt-ns-mutation", "", "")
	require.NoError(t, err)
	assert.Empty(t, reg.ConsumerIdentifiers, "deregistering the last consumer must leave an empty set")
}
