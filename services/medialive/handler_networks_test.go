package medialive_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetwork_CRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/prod/networks", map[string]any{
		"name":    "net-1",
		"ipPools": []map[string]any{{"cidr": "10.0.0.0/16"}},
		"routes":  []map[string]any{{"cidr": "0.0.0.0/0", "gateway": "10.0.0.1"}},
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	created := decodeBody(t, rec.Body.Bytes())
	id := created["id"].(string)
	assert.Equal(t, "ACTIVE", created["state"])
	assert.NotEmpty(t, created["ipPools"])

	rec = doRequest(t, h, http.MethodGet, "/prod/networks/"+id, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodPut, "/prod/networks/"+id, map[string]any{"name": "net-1-upd"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "net-1-upd", decodeBody(t, rec.Body.Bytes())["name"])

	rec = doRequest(t, h, http.MethodGet, "/prod/networks", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Len(t, decodeBody(t, rec.Body.Bytes())["networks"].([]any), 1)

	rec = doRequest(t, h, http.MethodDelete, "/prod/networks/"+id, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/prod/networks/"+id, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestNetwork_AssociatedClusterIDsAndDeleteGuard locks in a fix for
// gopherstack-1um: DescribeNetworkOutput's "associatedClusterIds" was
// hardcoded to an always-empty slice regardless of which Clusters actually
// referenced the network via NetworkSettings.InterfaceMappings, and
// DeleteNetwork had no guard at all -- real DeleteNetwork requires the
// Network have no resources associated with it
// (api_op_DeleteNetwork.go: "The Network must have no resources associated
// with it.").
func TestNetwork_AssociatedClusterIDsAndDeleteGuard(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/prod/networks", map[string]any{"name": "net-assoc"})
	require.Equal(t, http.StatusCreated, rec.Code)
	networkID := decodeBody(t, rec.Body.Bytes())["id"].(string)
	assert.Equal(t, []any{}, decodeBody(t, rec.Body.Bytes())["associatedClusterIds"])

	rec = doRequest(t, h, http.MethodPost, "/prod/clusters", map[string]any{
		"name": "net-assoc-cluster",
		"networkSettings": map[string]any{
			"interfaceMappings": []map[string]any{
				{"logicalInterfaceName": "if-a", "networkId": networkID},
			},
		},
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	clusterID := decodeBody(t, rec.Body.Bytes())["id"].(string)

	// Describe now reports the live association.
	rec = doRequest(t, h, http.MethodGet, "/prod/networks/"+networkID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []any{clusterID}, decodeBody(t, rec.Body.Bytes())["associatedClusterIds"])

	// Delete is rejected while a cluster is associated.
	rec = doRequest(t, h, http.MethodDelete, "/prod/networks/"+networkID, nil)
	assert.Equal(t, http.StatusConflict, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/prod/networks/"+networkID, nil)
	assert.Equal(t, http.StatusOK, rec.Code, "network must still exist after the rejected delete")

	// Removing the cluster clears the association and allows delete.
	rec = doRequest(t, h, http.MethodDelete, "/prod/clusters/"+clusterID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodDelete, "/prod/networks/"+networkID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestNetwork_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		name     string
		method   string
		path     string
		wantCode int
	}{
		{
			name: "create without name", method: http.MethodPost, path: "/prod/networks",
			body: map[string]any{}, wantCode: http.StatusBadRequest,
		},
		{
			name: "describe missing", method: http.MethodGet,
			path: "/prod/networks/missing", wantCode: http.StatusNotFound,
		},
		{
			name: "update missing", method: http.MethodPut, path: "/prod/networks/missing",
			body: map[string]any{"name": "x"}, wantCode: http.StatusNotFound,
		},
		{
			name: "delete missing", method: http.MethodDelete,
			path: "/prod/networks/missing", wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			rec := doRequest(t, h, tc.method, tc.path, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}
