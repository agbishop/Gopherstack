package eks_test

import (
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/eks"
)

func TestAddon_ConfigurationValues_RoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{
		"addonName":           "vpc-cni",
		"configurationValues": `{"env":{"ENABLE_PREFIX_DELEGATION":"true"}}`,
		"resolveConflicts":    "OVERWRITE",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/c1/addons/vpc-cni", nil)
	addon := parseResp(t, rec2)["addon"].(map[string]any)
	assert.JSONEq(t, `{"env":{"ENABLE_PREFIX_DELEGATION":"true"}}`, addon["configurationValues"].(string))
	assert.NotContains(t, addon, "resolveConflicts",
		"types.Addon has no resolveConflicts member; it is request-only")
}

func TestAddon_ResolveConflicts_InvalidValue_Rejected(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateAddon("c1", "vpc-cni", "", "", "", "INVALID", nil)
	require.ErrorIs(t, err, eks.ErrValidation)
}

func TestAddon_ResolveConflicts_ValidValues_Accepted(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	for _, rc := range []string{"OVERWRITE", "NONE", "PRESERVE"} {
		addonName := "addon-" + rc
		_, err = b.CreateAddon("c1", addonName, "", "", "", rc, nil)
		assert.NoError(t, err, "resolveConflicts=%s should be accepted", rc)
	}
}

func TestAddon_EmptyResolveConflicts_Accepted(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateAddon("c1", "vpc-cni", "", "", "", "", nil)
	require.NoError(t, err, "empty resolveConflicts must be accepted")
}

func TestAddon_UpdateAddon_Configuration_And_ResolveConflicts(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{"addonName": "coredns"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/addons/coredns/update", map[string]any{
		"configurationValues": `{"replicaCount":3}`,
		"resolveConflicts":    "PRESERVE",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/c1/addons/coredns", nil)
	addon := parseResp(t, rec2)["addon"].(map[string]any)
	assert.Equal(t, `{"replicaCount":3}`, addon["configurationValues"])
	assert.NotContains(t, addon, "resolveConflicts",
		"types.Addon has no resolveConflicts member; it is request-only")
}

func TestAddon_UpdateAddon_InvalidResolveConflicts_Rejected(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)
	_, err = b.CreateAddon("c1", "vpc-cni", "", "", "", "", nil)
	require.NoError(t, err)

	_, err = b.UpdateAddon("c1", "vpc-cni", "", "", "", "BAD", nil)
	require.ErrorIs(t, err, eks.ErrValidation)
}

func TestAddon_ConfigurationValues_AbsentWhenEmpty(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{"addonName": "coredns"})

	rec := doREST(t, h, http.MethodGet, "/clusters/c1/addons/coredns", nil)
	addon := parseResp(t, rec)["addon"].(map[string]any)
	_, hasCfg := addon["configurationValues"]
	assert.False(t, hasCfg, "configurationValues must be absent when empty")
}

func TestEKS_CreateAddon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *eks.Handler)
		body       map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "create_addon_success",
			setup: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "my-cluster"})
			},
			body: map[string]any{
				"addonName":    "vpc-cni",
				"addonVersion": "v1.12.0-eksbuild.1",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "create_addon_missing_name",
			setup: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "my-cluster"})
			},
			body:       map[string]any{"addonVersion": "v1.0"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create_addon_cluster_not_found",
			body:       map[string]any{"addonName": "vpc-cni"},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "create_addon_duplicate",
			setup: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "my-cluster"})
				doREST(t, h, http.MethodPost, "/clusters/my-cluster/addons",
					map[string]any{"addonName": "vpc-cni"})
			},
			body:       map[string]any{"addonName": "vpc-cni"},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doREST(t, h, http.MethodPost, "/clusters/my-cluster/addons", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				resp := parseResp(t, rec)
				addon, ok := resp["addon"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "my-cluster", addon["clusterName"])
				assert.Equal(t, "vpc-cni", addon["addonName"])
				assert.NotEmpty(t, addon["addonArn"])
				assert.Equal(t, "CREATING", addon["status"])
			}
		})
	}
}

func TestAddon_Status_ACTIVE_On_Create(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "addon-status-cluster")

	addon, err := b.CreateAddon("addon-status-cluster", "vpc-cni", "", "", "", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "CREATING", addon.Status)
}

func TestAddon_Status_DELETING_On_Delete(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "addon-del-cluster")
	_, _ = b.CreateAddon("addon-del-cluster", "coredns", "", "", "", "", nil)

	deleted, err := b.DeleteAddon("addon-del-cluster", "coredns")
	require.NoError(t, err)
	assert.Equal(t, "DELETING", deleted.Status)
}

func TestAddon_ARN_Format(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "arn-addon-cluster")

	addon, err := b.CreateAddon("arn-addon-cluster", "vpc-cni", "", "", "", "", nil)
	require.NoError(t, err)

	assert.Contains(t, addon.ARN, "arn:aws:eks:")
	assert.Contains(t, addon.ARN, ":addon/")
	assert.Contains(t, addon.ARN, "arn-addon-cluster")
	assert.Contains(t, addon.ARN, "vpc-cni")
}

func TestAddonCreatesAsCreating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		addonName string
	}{
		{name: "vpc_cni_starts_creating", addonName: "vpc-cni"},
		{name: "coredns_starts_creating", addonName: "coredns"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			_, err := b.CreateCluster(
				"cl", "1.32", "arn:aws:iam::123456789012:role/role", nil, nil, nil,
			)
			require.NoError(t, err)

			addon, err := b.CreateAddon("cl", tc.addonName, "", "", "", "", nil)
			require.NoError(t, err)
			assert.Equal(t, "CREATING", addon.Status, tc.name)
		})
	}
}

func TestAddonTransitionsToActive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "transitions_to_active"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				b := newBackend(t)
				_, err := b.CreateCluster(
					"cl", "1.32", "arn:aws:iam::123456789012:role/role", nil, nil, nil,
				)
				require.NoError(t, err)

				_, err = b.CreateAddon("cl", "vpc-cni", "", "", "", "", nil)
				require.NoError(t, err)

				time.Sleep(300 * time.Millisecond)

				addon, err := b.DescribeAddon("cl", "vpc-cni")
				require.NoError(t, err)
				assert.Equal(t, "ACTIVE", addon.Status, tc.name)
			})
		})
	}
}
