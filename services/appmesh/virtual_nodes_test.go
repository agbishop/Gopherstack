package appmesh_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appmesh"
)

// ─── VirtualNode CRUD (handler) ───

func TestAppMesh_VirtualNodeCRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	doRequest(t, h, http.MethodPut, "/meshes", map[string]any{"meshName": "m1"})

	// Create
	rec := doRequest(t, h, http.MethodPut, "/meshes/m1/virtualNodes",
		map[string]any{"virtualNodeName": "vn1"})
	assert.Equal(t, http.StatusOK, rec.Code)
	vn := getBody(t, rec)
	assert.Equal(t, "vn1", vn["virtualNodeName"])
	assert.Contains(t, vn["metadata"].(map[string]any)["arn"].(string), "virtualNode/vn1")

	// Describe
	rec = doRequest(t, h, http.MethodGet, "/meshes/m1/virtualNodes/vn1", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// List
	rec = doRequest(t, h, http.MethodGet, "/meshes/m1/virtualNodes", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	body := getBody(t, rec)
	assert.Len(t, body["virtualNodes"].([]any), 1)

	// Update
	rec = doRequest(t, h, http.MethodPut, "/meshes/m1/virtualNodes/vn1",
		map[string]any{"spec": map[string]any{}})
	assert.Equal(t, http.StatusOK, rec.Code)

	// Delete
	rec = doRequest(t, h, http.MethodDelete, "/meshes/m1/virtualNodes/vn1", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Confirm deleted
	rec = doRequest(t, h, http.MethodGet, "/meshes/m1/virtualNodes/vn1", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestAppMesh_VirtualNodeDeleteReferencedByService verifies DeleteVirtualNode
// is blocked while a virtual service still lists the node as its provider —
// per the DeleteVirtualNode doc comment (aws-sdk-go-v2/service/appmesh@v1.38.4/
// api_op_DeleteVirtualNode.go): "You must delete any virtual services that
// list a virtual node as a service provider before you can delete the
// virtual node itself".
func TestAppMesh_VirtualNodeDeleteReferencedByService(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	doRequest(t, h, http.MethodPut, "/meshes", map[string]any{"meshName": "m1"})
	doRequest(t, h, http.MethodPut, "/meshes/m1/virtualNodes",
		map[string]any{"virtualNodeName": "vn1"})
	doRequest(t, h, http.MethodPut, "/meshes/m1/virtualServices",
		map[string]any{
			"virtualServiceName": "vs1",
			"spec": map[string]any{
				"provider": map[string]any{
					"virtualNode": map[string]any{"virtualNodeName": "vn1"},
				},
			},
		})

	rec := doRequest(t, h, http.MethodDelete, "/meshes/m1/virtualNodes/vn1", nil)
	assert.Equal(t, http.StatusConflict, rec.Code)
	body := getBody(t, rec)
	assert.Equal(t, "ResourceInUseException", body["code"])

	doRequest(t, h, http.MethodDelete, "/meshes/m1/virtualServices/vs1", nil)
	rec = doRequest(t, h, http.MethodDelete, "/meshes/m1/virtualNodes/vn1", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestBackend_DeleteVirtualNodeReferencedByService(t *testing.T) {
	t.Parallel()

	b := appmesh.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateMesh("m1", nil, nil)
	require.NoError(t, err)
	_, err = b.CreateVirtualNode("m1", "vn1", nil, nil)
	require.NoError(t, err)
	spec := []byte(`{"provider":{"virtualNode":{"virtualNodeName":"vn1"}}}`)
	_, err = b.CreateVirtualService("m1", "vs1", spec, nil)
	require.NoError(t, err)

	_, err = b.DeleteVirtualNode("m1", "vn1")
	require.ErrorIs(t, err, appmesh.ErrVirtualNodeInUse)
}
