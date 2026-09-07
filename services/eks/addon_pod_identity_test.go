package eks_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/eks"
)

// listAddonAssociations returns the pod identity association summaries for a
// cluster via ListPodIdentityAssociations (GET .../pod-identity-associations).
func listAddonAssociations(t *testing.T, h *eks.Handler, cluster string) []map[string]any {
	t.Helper()

	rec := doREST(t, h, http.MethodGet, "/clusters/"+cluster+"/pod-identity-associations", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	items, ok := parseResp(t, rec)["associations"].([]any)
	require.True(t, ok, "response must have an 'associations' array")

	out := make([]map[string]any, len(items))
	for i, it := range items {
		out[i], ok = it.(map[string]any)
		require.True(t, ok)
	}

	return out
}

// addonPodIdentityIDs extracts addon["podIdentityAssociations"] as a
// []string, failing the test if the key is missing or not a string array.
func addonPodIdentityIDs(t *testing.T, addon map[string]any) []string {
	t.Helper()

	raw, ok := addon["podIdentityAssociations"].([]any)
	require.True(t, ok, "addon must always carry a podIdentityAssociations array")

	ids := make([]string, len(raw))
	for i, v := range raw {
		ids[i], ok = v.(string)
		require.True(t, ok)
	}

	return ids
}

func getAddon(t *testing.T, h *eks.Handler, addonName string) map[string]any {
	t.Helper()

	rec := doREST(t, h, http.MethodGet, "/clusters/c1/addons/"+addonName, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	addon, ok := parseResp(t, rec)["addon"].(map[string]any)
	require.True(t, ok)

	return addon
}

func TestAddon_UpdateAddon_PodIdentityAssociations_AbsentLeavesUnchanged(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{"addonName": "vpc-cni"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/addons/vpc-cni/update", map[string]any{
		"podIdentityAssociations": []map[string]any{
			{"roleArn": "arn:aws:iam::123456789012:role/r1", "serviceAccount": "sa1"},
			{"roleArn": "arn:aws:iam::123456789012:role/r2", "serviceAccount": "sa2"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	idsBefore := addonPodIdentityIDs(t, getAddon(t, h, "vpc-cni"))
	require.Len(t, idsBefore, 2)

	rec = doREST(t, h, http.MethodPost, "/clusters/c1/addons/vpc-cni/update", map[string]any{
		"addonVersion": "v1.18.6-eksbuild.1",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	idsAfter := addonPodIdentityIDs(t, getAddon(t, h, "vpc-cni"))
	assert.ElementsMatch(t, idsBefore, idsAfter,
		"omitting podIdentityAssociations must leave the addon's existing associations unchanged")

	assocs := listAddonAssociations(t, h, "c1")
	assert.Len(t, assocs, 2, "existing associations must survive an update that omits podIdentityAssociations")
}

func TestAddon_UpdateAddon_PodIdentityAssociations_EmptyDeletesOnlyThisAddon(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{"addonName": "vpc-cni"})
	doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{"addonName": "coredns"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/addons/vpc-cni/update", map[string]any{
		"podIdentityAssociations": []map[string]any{
			{"roleArn": "arn:aws:iam::123456789012:role/vpc-r1", "serviceAccount": "vpc-sa1"},
			{"roleArn": "arn:aws:iam::123456789012:role/vpc-r2", "serviceAccount": "vpc-sa2"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doREST(t, h, http.MethodPost, "/clusters/c1/addons/coredns/update", map[string]any{
		"podIdentityAssociations": []map[string]any{
			{"roleArn": "arn:aws:iam::123456789012:role/coredns-r1", "serviceAccount": "coredns-sa1"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	corednsIDsBefore := addonPodIdentityIDs(t, getAddon(t, h, "coredns"))
	require.Len(t, corednsIDsBefore, 1)

	rec = doREST(t, h, http.MethodPost, "/clusters/c1/addons/vpc-cni/update", map[string]any{
		"podIdentityAssociations": []any{},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	vpcIDsAfter := addonPodIdentityIDs(t, getAddon(t, h, "vpc-cni"))
	assert.Empty(t, vpcIDsAfter,
		"an empty podIdentityAssociations array must delete all associations owned by the addon")

	corednsIDsAfter := addonPodIdentityIDs(t, getAddon(t, h, "coredns"))
	assert.Equal(t, corednsIDsBefore, corednsIDsAfter,
		"a separate untouched addon must keep its own associations")

	assocs := listAddonAssociations(t, h, "c1")
	require.Len(t, assocs, 1, "exactly the coredns-owned association must survive the vpc-cni deletion")
	assert.Equal(t, "coredns-sa1", assocs[0]["serviceAccount"])
	assert.Equal(t, corednsIDsAfter[0], assocs[0]["associationId"])
}

func TestAddon_UpdateAddon_PodIdentityAssociations_PopulatedReplaces(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{"addonName": "vpc-cni"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/addons/vpc-cni/update", map[string]any{
		"podIdentityAssociations": []map[string]any{
			{"roleArn": "arn:aws:iam::123456789012:role/old", "serviceAccount": "old-sa"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	oldIDs := addonPodIdentityIDs(t, getAddon(t, h, "vpc-cni"))
	require.Len(t, oldIDs, 1)

	rec = doREST(t, h, http.MethodPost, "/clusters/c1/addons/vpc-cni/update", map[string]any{
		"podIdentityAssociations": []map[string]any{
			{"roleArn": "arn:aws:iam::123456789012:role/new1", "serviceAccount": "new-sa1"},
			{"roleArn": "arn:aws:iam::123456789012:role/new2", "serviceAccount": "new-sa2"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	newIDs := addonPodIdentityIDs(t, getAddon(t, h, "vpc-cni"))
	require.Len(t, newIDs, 2, "the addon must now list exactly the replacement associations")
	assert.NotContains(t, newIDs, oldIDs[0], "the previous association must be gone from the addon's readback")

	assocs := listAddonAssociations(t, h, "c1")
	require.Len(t, assocs, 2, "populated podIdentityAssociations must replace the previous set exactly")

	gotSAs := []string{assocs[0]["serviceAccount"].(string), assocs[1]["serviceAccount"].(string)}
	assert.ElementsMatch(t, []string{"new-sa1", "new-sa2"}, gotSAs)

	for _, a := range assocs {
		id, ok := a["associationId"].(string)
		require.True(t, ok)

		full := parseResp(t, doREST(t, h, http.MethodGet, "/clusters/c1/pod-identity-associations/"+id, nil))
		assoc, ok := full["association"].(map[string]any)
		require.True(t, ok)

		switch assoc["serviceAccount"] {
		case "new-sa1":
			assert.Equal(t, "arn:aws:iam::123456789012:role/new1", assoc["roleArn"])
		case "new-sa2":
			assert.Equal(t, "arn:aws:iam::123456789012:role/new2", assoc["roleArn"])
		default:
			t.Fatalf("unexpected serviceAccount %v", assoc["serviceAccount"])
		}
	}
}

func TestAddon_UpdateAddon_PodIdentityAssociations_RejectsMissingFields(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/addons", map[string]any{"addonName": "vpc-cni"})

	rec := doREST(t, h, http.MethodPost, "/clusters/c1/addons/vpc-cni/update", map[string]any{
		"podIdentityAssociations": []map[string]any{
			{"roleArn": "arn:aws:iam::123456789012:role/r1"},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	idsAfter := addonPodIdentityIDs(t, getAddon(t, h, "vpc-cni"))
	assert.Empty(t, idsAfter, "a rejected update must not create partial associations")
}
