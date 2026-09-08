package appmesh

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) virtualNodeARN(meshName, name string) string {
	return arn.Build("appmesh", b.region, b.accountID, fmt.Sprintf("mesh/%s/virtualNode/%s", meshName, name))
}

func (b *InMemoryBackend) CreateVirtualNode(
	meshName, name string, spec json.RawMessage, tags map[string]string,
) (*VirtualNode, error) {
	b.mu.Lock("CreateVirtualNode")
	defer b.mu.Unlock()
	if !b.meshes.Has(meshName) {
		return nil, ErrMeshNotFound
	}
	key := meshChildKey(meshName, name)
	if b.virtualNodes.Has(key) {
		return nil, ErrVirtualNodeAlreadyExists
	}
	arn := b.virtualNodeARN(meshName, name)
	vn := &VirtualNode{
		Meta:            newMeta(arn, b.accountID),
		MeshName:        meshName,
		VirtualNodeName: name,
		Spec:            normalizeSpec(spec),
		Status:          statusActive,
	}
	b.virtualNodes.Put(vn)
	if len(tags) > 0 {
		b.tags[arn] = cloneTags(tags)
	}

	return vn, nil
}

func (b *InMemoryBackend) DescribeVirtualNode(meshName, name string) (*VirtualNode, error) {
	b.mu.RLock("DescribeVirtualNode")
	defer b.mu.RUnlock()
	if !b.meshes.Has(meshName) {
		return nil, ErrMeshNotFound
	}
	vn, ok := b.virtualNodes.Get(meshChildKey(meshName, name))
	if !ok {
		return nil, ErrVirtualNodeNotFound
	}

	return vn, nil
}

func (b *InMemoryBackend) UpdateVirtualNode(meshName, name string, spec json.RawMessage) (*VirtualNode, error) {
	b.mu.Lock("UpdateVirtualNode")
	defer b.mu.Unlock()
	if !b.meshes.Has(meshName) {
		return nil, ErrMeshNotFound
	}
	vn, ok := b.virtualNodes.Get(meshChildKey(meshName, name))
	if !ok {
		return nil, ErrVirtualNodeNotFound
	}
	vn.Spec = normalizeSpec(spec)
	vn.Meta.UpdatedAt = time.Now().UTC()
	vn.Meta.Version++

	return vn, nil
}

// virtualNodeIsServiceProvider reports whether any virtual service in
// meshName lists name as its provider (types.VirtualServiceProvider's
// virtualNode field) — see the DeleteVirtualNode doc comment cited on
// ErrVirtualNodeInUse's use below.
func (b *InMemoryBackend) virtualNodeIsServiceProvider(meshName, name string) bool {
	for _, vs := range b.virtualSvcsByMesh.Get(meshName) {
		var body struct {
			Provider *virtualServiceProviderBody `json:"provider"`
		}
		if err := json.Unmarshal(vs.Spec, &body); err != nil {
			continue
		}
		if body.Provider != nil && body.Provider.VirtualNode != nil &&
			body.Provider.VirtualNode.VirtualNodeName != nil &&
			*body.Provider.VirtualNode.VirtualNodeName == name {
			return true
		}
	}

	return false
}

func (b *InMemoryBackend) DeleteVirtualNode(meshName, name string) (*VirtualNode, error) {
	b.mu.Lock("DeleteVirtualNode")
	defer b.mu.Unlock()
	if !b.meshes.Has(meshName) {
		return nil, ErrMeshNotFound
	}
	key := meshChildKey(meshName, name)
	vn, ok := b.virtualNodes.Get(key)
	if !ok {
		return nil, ErrVirtualNodeNotFound
	}
	// DeleteVirtualNode doc comment (aws-sdk-go-v2/service/appmesh@v1.38.4/
	// api_op_DeleteVirtualNode.go): "You must delete any virtual services
	// that list a virtual node as a service provider before you can delete
	// the virtual node itself."
	if b.virtualNodeIsServiceProvider(meshName, name) {
		return nil, ErrVirtualNodeInUse
	}
	b.virtualNodes.Delete(key)
	delete(b.tags, vn.Meta.Arn)
	vn.Status = statusDeleted

	return vn, nil
}

//nolint:dupl // list/create pattern is structurally identical across resource types
func (b *InMemoryBackend) ListVirtualNodes(
	meshName string, maxResults int32, nextToken string,
) ([]*VirtualNodeSummary, string, error) {
	b.mu.RLock("ListVirtualNodes")
	defer b.mu.RUnlock()
	if !b.meshes.Has(meshName) {
		return nil, "", ErrMeshNotFound
	}
	nodes := b.virtualNodesByMesh.Get(meshName)
	names := make([]string, len(nodes))
	for i, vn := range nodes {
		names[i] = vn.VirtualNodeName
	}
	sort.Strings(names)
	items, next := paginateStrings(names, nextToken, maxResults)
	summaries := make([]*VirtualNodeSummary, 0, len(items))
	for _, n := range items {
		vn, _ := b.virtualNodes.Get(meshChildKey(meshName, n))
		summaries = append(summaries, &VirtualNodeSummary{
			CreatedAt:       vn.Meta.CreatedAt,
			UpdatedAt:       vn.Meta.UpdatedAt,
			Arn:             vn.Meta.Arn,
			MeshName:        meshName,
			VirtualNodeName: vn.VirtualNodeName,
			MeshOwner:       vn.Meta.MeshOwner,
			ResourceOwner:   vn.Meta.ResourceOwner,
			Version:         vn.Meta.Version,
		})
	}

	return summaries, next, nil
}
