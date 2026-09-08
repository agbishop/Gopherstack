package eks_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ekssdk "github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/eks"
)

// TestAddon_NamespaceConfig_RoundTrip proves CreateAddonInput.NamespaceConfig
// (aws-sdk-go-v2/service/eks@v1.90.4 api_op_CreateAddon.go) is parsed and
// echoed back on Addon.NamespaceConfig (types.go:141), matching the
// request/response snapshot fixtures' "namespaceConfig":{"namespace":...}
// shape.
func TestAddon_NamespaceConfig_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	client := newTestEKSClient(t, h)
	ctx := context.Background()

	_, err := client.CreateCluster(ctx, &ekssdk.CreateClusterInput{
		Name:               aws.String("c1"),
		RoleArn:            aws.String("arn:aws:iam::123456789012:role/eks"),
		ResourcesVpcConfig: &types.VpcConfigRequest{},
	})
	require.NoError(t, err)

	_, err = client.CreateAddon(ctx, &ekssdk.CreateAddonInput{
		ClusterName: aws.String("c1"),
		AddonName:   aws.String("aws-ebs-csi-driver"),
		NamespaceConfig: &types.AddonNamespaceConfigRequest{
			Namespace: aws.String("ebs-csi"),
		},
	})
	require.NoError(t, err)

	out, err := client.DescribeAddon(ctx, &ekssdk.DescribeAddonInput{
		ClusterName: aws.String("c1"),
		AddonName:   aws.String("aws-ebs-csi-driver"),
	})
	require.NoError(t, err)
	require.NotNil(
		t,
		out.Addon.NamespaceConfig,
		"NamespaceConfig must round-trip onto the DescribeAddon response",
	)
	assert.Equal(t, "ebs-csi", aws.ToString(out.Addon.NamespaceConfig.Namespace))
}

// TestAddon_NamespaceConfig_Absent_OmitsNamespaceConfig proves that omitting
// NamespaceConfig at create time leaves it unset on readback, matching a
// real add-on installed at its documented default namespace rather than an
// override.
func TestAddon_NamespaceConfig_Absent_OmitsNamespaceConfig(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(
		t,
		h,
		http.MethodPost,
		"/clusters/c1/addons",
		map[string]any{"addonName": "vpc-cni"},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	addon := getAddon(t, h, "vpc-cni")
	assert.NotContains(t, addon, "namespaceConfig",
		"namespaceConfig must be absent when the create request did not specify one")
}

// TestAddon_NamespaceConfig_PodIdentityAssociationUsesCustomNamespace proves
// replaceAddonPodIdentityAssociationsLocked (addons.go) installs addon-owned
// pod identity associations into the addon's NamespaceConfig.Namespace when
// one was given at create time, instead of unconditionally using
// addonPodIdentityNamespace ("kube-system").
func TestAddon_NamespaceConfig_PodIdentityAssociationUsesCustomNamespace(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{
		"addonName":       "aws-efs-csi-driver",
		"namespaceConfig": map[string]any{"namespace": "efs-csi"},
		"podIdentityAssociations": []map[string]any{
			{"roleArn": "arn:aws:iam::123456789012:role/efs", "serviceAccount": "efs-csi-sa"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	assocs := listAddonAssociations(t, h)
	require.Len(t, assocs, 1)
	assert.Equal(t, "efs-csi", assocs[0]["namespace"],
		"addon-owned association must land in the addon's configured namespace, not kube-system")
}

// TestAddon_NamespaceConfig_Absent_PodIdentityAssociationDefaultsToKubeSystem
// locks in that the kube-system default (correct for the AWS-managed
// add-ons: vpc-cni, coredns, kube-proxy) survives the NamespaceConfig wiring
// when no override is given.
func TestAddon_NamespaceConfig_Absent_PodIdentityAssociationDefaultsToKubeSystem(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{
		"addonName": "vpc-cni",
		"podIdentityAssociations": []map[string]any{
			{"roleArn": "arn:aws:iam::123456789012:role/vpc", "serviceAccount": "aws-node"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	assocs := listAddonAssociations(t, h)
	require.Len(t, assocs, 1)
	assert.Equal(t, "kube-system", assocs[0]["namespace"])
}

// TestAddon_NamespaceConfig_ImmutableOnUpdate proves UpdateAddonInput
// (aws-sdk-go-v2/service/eks@v1.90.4 api_op_UpdateAddon.go) has no
// NamespaceConfig member -- a real client cannot change an add-on's
// namespace after creation -- so the emulator must ignore an inbound
// "namespaceConfig" key on the update body rather than acting on it.
func TestAddon_NamespaceConfig_ImmutableOnUpdate(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{
		"addonName":       "aws-efs-csi-driver",
		"namespaceConfig": map[string]any{"namespace": "efs-csi"},
	})

	rec := doREST(
		t,
		h,
		http.MethodPost,
		"/clusters/c1/addons/aws-efs-csi-driver/update",
		map[string]any{
			"namespaceConfig": map[string]any{"namespace": "attempted-override"},
		},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	addon := getAddon(t, h, "aws-efs-csi-driver")
	nsCfg, ok := addon["namespaceConfig"].(map[string]any)
	require.True(t, ok)
	assert.Equal(
		t,
		"efs-csi",
		nsCfg["namespace"],
		"UpdateAddonInput has no NamespaceConfig member; it must not change the addon's namespace",
	)
}

// TestAddon_NamespaceConfig_PersistenceRoundTrip proves Addon.Namespace
// (models.go) survives a Handler-mediated snapshot/restore cycle. Tables are
// snapshotted generically as plain JSON (pkgs/store/registry.go
// Registry.SnapshotAll/RestoreAll marshal each *Table[V]'s contents via
// encoding/json), so this is additive: an eksSnapshotVersion bump is not
// required, and a pre-fix snapshot with no "namespace" key restores it as the
// zero value ("").
func TestAddon_NamespaceConfig_PersistenceRoundTrip(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")

	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateAddon("c1", "aws-efs-csi-driver", "", "", "", "", "efs-csi", nil, nil)
	require.NoError(t, err)

	h := eks.NewHandler(b)
	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	b2 := eks.NewInMemoryBackend(t.Context(), "123456789012", "us-east-1")
	require.NoError(t, b2.Restore(t.Context(), snap))

	addon, err := b2.DescribeAddon("c1", "aws-efs-csi-driver")
	require.NoError(t, err)
	assert.Equal(t, "efs-csi", addon.Namespace)
}
