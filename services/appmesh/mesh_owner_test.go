package appmesh_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callerAccount matches newTestHandler's backend account. foreignAccount is
// a different account id, used to prove the meshOwner query param is
// actually read rather than silently ignored (gopherstack-jxsz).
const (
	callerAccount  = "000000000000"
	foreignAccount = "999999999999"
)

// TestAppMesh_MeshOwner_ForbiddenAcrossResources drives every meshOwner-
// carrying resource dispatcher (Describe of each of the 6 sub-resource types
// plus DescribeMesh) with a meshOwner naming a different account than the
// caller's. gopherstack has no AWS RAM cross-account mesh-sharing model, so
// no mesh can ever really be owned by another account: the only honest
// response is ForbiddenException (declared on every op touched here — see
// PARITY.md), not a fabricated cross-account result.
func TestAppMesh_MeshOwner_ForbiddenAcrossResources(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	doRequest(t, h, http.MethodPut, "/meshes", map[string]any{"meshName": "m1"})
	doRequest(t, h, http.MethodPut, "/meshes/m1/virtualNodes", map[string]any{"virtualNodeName": "vn1"})
	doRequest(t, h, http.MethodPut, "/meshes/m1/virtualRouters", map[string]any{"virtualRouterName": "vr1"})
	doRequest(t, h, http.MethodPut, "/meshes/m1/virtualRouter/vr1/routes", map[string]any{"routeName": "rt1"})
	doRequest(t, h, http.MethodPut, "/meshes/m1/virtualServices", map[string]any{"virtualServiceName": "vs1"})
	doRequest(t, h, http.MethodPut, "/meshes/m1/virtualGateways", map[string]any{"virtualGatewayName": "vg1"})
	doRequest(t, h, http.MethodPut, "/meshes/m1/virtualGateway/vg1/gatewayRoutes",
		map[string]any{"gatewayRouteName": "gr1"})

	tests := []struct {
		name string
		path string
	}{
		{"mesh", "/meshes/m1"},
		{"virtualnode", "/meshes/m1/virtualNodes/vn1"},
		{"virtualrouter", "/meshes/m1/virtualRouters/vr1"},
		{"route", "/meshes/m1/virtualRouter/vr1/routes/rt1"},
		{"virtualservice", "/meshes/m1/virtualServices/vs1"},
		{"virtualgateway", "/meshes/m1/virtualGateways/vg1"},
		{"gatewayroute", "/meshes/m1/virtualGateway/vg1/gatewayRoutes/gr1"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, http.MethodGet,
				fmt.Sprintf("%s?meshOwner=%s", tc.path, foreignAccount), nil)
			assert.Equal(t, http.StatusForbidden, rec.Code)
			body := getBody(t, rec)
			assert.Equal(t, "ForbiddenException", body["code"])
		})
	}
}

// TestAppMesh_MeshOwner_CreateRejected_NotCreated verifies a mismatched
// meshOwner on Create rejects with ForbiddenException and leaves the
// resource uncreated.
func TestAppMesh_MeshOwner_CreateRejected_NotCreated(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	doRequest(t, h, http.MethodPut, "/meshes", map[string]any{"meshName": "m1"})

	rec := doRequest(t, h, http.MethodPut,
		fmt.Sprintf("/meshes/m1/virtualNodes?meshOwner=%s", foreignAccount),
		map[string]any{"virtualNodeName": "vn1"})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	body := getBody(t, rec)
	assert.Equal(t, "ForbiddenException", body["code"])

	rec = doRequest(t, h, http.MethodGet, "/meshes/m1/virtualNodes", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, getBody(t, rec)["virtualNodes"].([]any), "rejected create must not have created the resource")
}

// TestAppMesh_MeshOwner_UpdateDeleteRejected_NotMutated verifies a mismatched
// meshOwner on Update/Delete rejects with ForbiddenException and leaves the
// resource unmutated.
func TestAppMesh_MeshOwner_UpdateDeleteRejected_NotMutated(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	doRequest(t, h, http.MethodPut, "/meshes", map[string]any{"meshName": "m1"})
	doRequest(t, h, http.MethodPut, "/meshes/m1/virtualNodes", map[string]any{"virtualNodeName": "vn1"})

	rec := doRequest(t, h, http.MethodPut,
		fmt.Sprintf("/meshes/m1/virtualNodes/vn1?meshOwner=%s", foreignAccount),
		map[string]any{"spec": map[string]any{}})
	assert.Equal(t, http.StatusForbidden, rec.Code)

	rec = doRequest(t, h, http.MethodDelete,
		fmt.Sprintf("/meshes/m1/virtualNodes/vn1?meshOwner=%s", foreignAccount), nil)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/meshes/m1/virtualNodes/vn1", nil)
	require.Equal(t, http.StatusOK, rec.Code, "resource must still exist after rejected update/delete")
	vn := getBody(t, rec)
	meta := vn["metadata"].(map[string]any)
	assert.InEpsilon(t, float64(1), meta["version"], 0, "version must not have incremented from the rejected update")
}

// TestAppMesh_MeshOwner_MatchingOrOmitted_DefaultsToCaller verifies that a
// meshOwner equal to the caller's own account, or omitted entirely, is the
// documented default and succeeds with MeshOwner/ResourceOwner both set to
// the caller's account — not swapped, not the foreign account.
func TestAppMesh_MeshOwner_MatchingOrOmitted_DefaultsToCaller(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	doRequest(t, h, http.MethodPut, "/meshes", map[string]any{"meshName": "m1"})

	tests := []struct {
		name string
		path string
	}{
		{"omitted", "/meshes/m1"},
		{"matching_caller", "/meshes/m1?meshOwner=" + callerAccount},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := doRequest(t, h, http.MethodGet, tc.path, nil)
			require.Equal(t, http.StatusOK, rec.Code)
			body := getBody(t, rec)
			meta := body["metadata"].(map[string]any)
			assert.Equal(t, callerAccount, meta["meshOwner"])
			assert.Equal(t, callerAccount, meta["resourceOwner"])
		})
	}
}
