package eks_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/eks"
)

func TestReset(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	assert.Equal(t, 1, b.ClusterCount())

	b.Reset()

	assert.Equal(t, 0, b.ClusterCount())
	assert.Equal(t, 0, b.NodegroupCount())
	assert.Equal(t, 0, b.AccessEntryCount())
	assert.Equal(t, 0, b.AddonCount())
	assert.Equal(t, 0, b.FargateProfileCount())
	assert.Equal(t, 0, b.CapabilityCount())
	assert.Equal(t, 0, b.SubscriptionCount())
}

func TestMultipleResetCycle(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	for range 3 {
		_, err := b.CreateCluster("cluster", "1.32", "", nil, nil, nil)
		require.NoError(t, err)

		b.Reset()

		assert.Equal(t, 0, b.ClusterCount())
	}
}

func TestHandlerReset(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)

	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	assert.Equal(t, 1, h.Backend.ClusterCount())

	h.Reset()

	assert.Equal(t, 0, h.Backend.ClusterCount())
}

func TestErrValidation(t *testing.T) {
	t.Parallel()

	require.Error(t, eks.ErrValidation)
	assert.NotEqual(t, eks.ErrNotFound, eks.ErrValidation)
	assert.NotEqual(t, eks.ErrAlreadyExists, eks.ErrValidation)
}

func TestSeedHelpers(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	b.AddClusterInternal(&eks.Cluster{Name: "seeded", Version: "1.30", Status: "ACTIVE"})
	assert.Equal(t, 1, b.ClusterCount())

	b.AddNodegroupInternal(&eks.Nodegroup{NodegroupName: "ng1", ClusterName: "seeded"})
	assert.Equal(t, 1, b.NodegroupCount())
}

func TestExportCountHelpers(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	assert.Equal(t, 0, b.ClusterCount())
	assert.Equal(t, 0, b.NodegroupCount())
	assert.Equal(t, 0, b.AccessEntryCount())
	assert.Equal(t, 0, b.AddonCount())
	assert.Equal(t, 0, b.FargateProfileCount())
	assert.Equal(t, 0, b.PodIdentityAssociationCount())
	assert.Equal(t, 0, b.CapabilityCount())
	assert.Equal(t, 0, b.SubscriptionCount())
}

func TestAccessEntryAddAddon(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	b.AddClusterInternal(&eks.Cluster{Name: "c1", Status: "ACTIVE"})

	b.AddAccessEntryInternal(&eks.AccessEntry{
		ClusterName:  "c1",
		PrincipalARN: "arn:aws:iam::123:role/r",
		Type:         "STANDARD",
	})

	b.AddAddonInternal(&eks.Addon{ClusterName: "c1", AddonName: "coredns", Status: "ACTIVE"})

	assert.Equal(t, 1, b.AccessEntryCount())
	assert.Equal(t, 1, b.AddonCount())
}

func TestAddFargateProfile(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	b.AddFargateProfileInternal(&eks.FargateProfile{
		ClusterName:        "c1",
		FargateProfileName: "fp1",
		Status:             "ACTIVE",
	})

	assert.Equal(t, 1, b.FargateProfileCount())
}

func TestAddCapabilityAndSubscription(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	b.AddCapabilityInternal(&eks.Capability{ClusterName: "c1", CapabilityName: "cap1", Status: "ACTIVE"})
	b.AddSubscriptionInternal(&eks.AnywhereSubscription{ID: "sub1", Name: "s1", Status: "ACTIVE"})

	assert.Equal(t, 1, b.CapabilityCount())
	assert.Equal(t, 1, b.SubscriptionCount())
}

func TestResetClearsAllResourceKinds(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)

	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{"addonName": "addon1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/capabilities", map[string]any{
		"capabilityName":          "cap1",
		"type":                    "ARGOCD",
		"roleArn":                 "arn:aws:iam::123456789012:role/capability-role",
		"deletePropagationPolicy": "RETAIN",
	})
	doREST(t, h, http.MethodPost, "/eks-anywhere-subscriptions", map[string]any{
		"name": "sub1",
		"term": map[string]any{"unit": "MONTHS", "duration": 12},
	})

	assert.Equal(t, 1, h.Backend.ClusterCount())
	assert.Equal(t, 1, h.Backend.AddonCount())
	assert.Equal(t, 1, h.Backend.CapabilityCount())
	assert.Equal(t, 1, h.Backend.SubscriptionCount())

	h.Reset()

	assert.Equal(t, 0, h.Backend.ClusterCount())
	assert.Equal(t, 0, h.Backend.AddonCount())
	assert.Equal(t, 0, h.Backend.CapabilityCount())
	assert.Equal(t, 0, h.Backend.SubscriptionCount())
}

func TestResetWithTaggedResources(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)

	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, map[string]string{"a": "1"})
	require.NoError(t, err)

	_, err = b.CreateNodegroup(
		"c1", "ng1", "", "", "", "", "", nil, 1, 1, 2, eks.NodegroupInput{}, map[string]string{"b": "2"},
	)
	require.NoError(t, err)

	_, err = b.CreateAccessEntry("c1", "arn:aws:iam::123:role/r", "STANDARD", "", nil, nil)
	require.NoError(t, err)

	_, err = b.CreateAddon("c1", "coredns", "", "", "", "", map[string]string{"c": "3"}, nil)
	require.NoError(t, err)

	_, err = b.CreateFargateProfile("c1", "fp1", "", nil, nil, map[string]string{"d": "4"})
	require.NoError(t, err)

	// Reset should not panic even with tagged resources.
	b.Reset()

	assert.Equal(t, 0, b.ClusterCount())
	assert.Equal(t, 0, b.FargateProfileCount())
}

func TestEKSBackendRegion(t *testing.T) {
	t.Parallel()

	backend := eks.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")
	assert.Equal(t, "us-east-1", backend.Region())
}
