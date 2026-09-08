package opsworks

import (
	"time"

	"github.com/google/uuid"
)

// isValidLayerType reports whether layerType is one of the exact LayerType
// enum values from aws-sdk-go-v2/service/opsworks/types.LayerType.Values()
// -- CreateLayer's Type member on the real API is restricted to this set,
// not a free string.
func isValidLayerType(layerType string) bool {
	switch layerType {
	case "aws-flow-ruby", "ecs-cluster", "java-app", "lb", "web", "php-app",
		"rails-app", "nodejs-app", "memcached", "db-master", "monitoring-master", "custom":
		return true
	default:
		return false
	}
}

// CreateLayer creates a new layer in a stack. Name, Shortname, StackId, and
// Type are all "This member is required" on the real CreateLayerInput
// (confirmed against aws-sdk-go-v2/service/opsworks@v1.31.0's
// api_op_CreateLayer.go), and Type is restricted to the LayerType enum, not
// a free string.
func (b *InMemoryBackend) CreateLayer(stackID, layerType, name, shortname string) (*Layer, error) {
	if name == "" || shortname == "" || stackID == "" || !isValidLayerType(layerType) {
		return nil, ErrValidation
	}

	b.mu.Lock("CreateLayer")
	defer b.mu.Unlock()

	if !b.stacks.Has(stackID) {
		return nil, ErrStackNotFound
	}

	id := uuid.NewString()
	now := time.Now().UTC()

	l := &storedLayer{
		CreatedAt: now,
		StackID:   stackID,
		LayerID:   id,
		Arn:       b.layerARN(id),
		Type:      layerType,
		Name:      name,
		Shortname: shortname,
	}
	b.layers.Put(l)

	return l.toLayer(), nil
}

// DescribeLayers returns layers filtered by stack and/or layer IDs.
func (b *InMemoryBackend) DescribeLayers(stackID string, layerIDs []string) ([]*Layer, error) {
	b.mu.RLock("DescribeLayers")
	defer b.mu.RUnlock()

	if len(layerIDs) > 0 {
		result := make([]*Layer, 0, len(layerIDs))
		for _, id := range layerIDs {
			l, ok := b.layers.Get(id)
			if !ok {
				return nil, ErrLayerNotFound
			}
			result = append(result, l.toLayer())
		}

		return result, nil
	}

	source := stackScoped(stackID, b.layers.All, b.layersByStack.Get)

	result := make([]*Layer, 0, len(source))
	for _, l := range source {
		result = append(result, l.toLayer())
	}

	return result, nil
}

// UpdateLayer updates a layer's name.
func (b *InMemoryBackend) UpdateLayer(layerID, name string) error {
	b.mu.Lock("UpdateLayer")
	defer b.mu.Unlock()

	l, ok := b.layers.Get(layerID)
	if !ok {
		return ErrLayerNotFound
	}

	if name != "" {
		l.Name = name
	}

	return nil
}

// DeleteLayer deletes a layer. AWS requires all associated instances be
// deleted or unassigned first (api_op_DeleteLayer.go: "You must first stop
// and then delete all associated instances or unassign registered
// instances.").
func (b *InMemoryBackend) DeleteLayer(layerID string) error {
	b.mu.Lock("DeleteLayer")
	defer b.mu.Unlock()

	l, ok := b.layers.Get(layerID)
	if !ok {
		return ErrLayerNotFound
	}

	for _, i := range b.instancesByStack.Get(l.StackID) {
		if i.LayerID == layerID {
			return ErrValidation
		}
	}

	b.layers.Delete(layerID)

	return nil
}
