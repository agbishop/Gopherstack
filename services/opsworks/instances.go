package opsworks

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreateInstance creates a new instance in a stack/layer. StackId,
// LayerIds (at least one), and InstanceType are all "This member is
// required" on the real CreateInstanceInput (confirmed against
// aws-sdk-go-v2/service/opsworks@v1.31.0's api_op_CreateInstance.go).
func (b *InMemoryBackend) CreateInstance(stackID, layerID, instanceType string) (*Instance, error) {
	if stackID == "" || layerID == "" || instanceType == "" {
		return nil, ErrValidation
	}

	b.mu.Lock("CreateInstance")
	defer b.mu.Unlock()

	if !b.stacks.Has(stackID) {
		return nil, ErrStackNotFound
	}

	if !b.layers.Has(layerID) {
		return nil, ErrLayerNotFound
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	// Use a UUID-derived suffix to avoid length-based race conditions.
	hostname := fmt.Sprintf("gopherstack-%s", id[:8])

	i := &storedInstance{
		CreatedAt:    now,
		StackID:      stackID,
		LayerID:      layerID,
		InstanceID:   id,
		Arn:          b.instanceARN(id),
		Hostname:     hostname,
		InstanceType: instanceType,
		Status:       instanceStatusStopped,
	}
	b.instances.Put(i)

	return i.toInstance(), nil
}

// RegisterInstance registers an on-premises instance with a stack. StackId
// is "This member is required" on the real RegisterInstanceInput (confirmed
// against aws-sdk-go-v2/service/opsworks@v1.31.0's
// api_op_RegisterInstance.go); Hostname is not.
func (b *InMemoryBackend) RegisterInstance(stackID, hostname string) (string, error) {
	if stackID == "" {
		return "", ErrValidation
	}

	b.mu.Lock("RegisterInstance")
	defer b.mu.Unlock()

	if !b.stacks.Has(stackID) {
		return "", ErrStackNotFound
	}

	id := uuid.NewString()
	now := time.Now().UTC()

	h := hostname
	if h == "" {
		h = fmt.Sprintf("registered-%s", id[:8])
	}

	i := &storedInstance{
		CreatedAt:  now,
		StackID:    stackID,
		InstanceID: id,
		Arn:        b.instanceARN(id),
		Hostname:   h,
		Status:     instanceStatusStopped,
		Registered: true,
	}
	b.instances.Put(i)

	return id, nil
}

// DeregisterInstance deregisters an instance from OpsWorks.
func (b *InMemoryBackend) DeregisterInstance(instanceID string) error {
	b.mu.Lock("DeregisterInstance")
	defer b.mu.Unlock()

	if !b.instances.Delete(instanceID) {
		return ErrInstanceNotFound
	}

	return nil
}

// AssignInstance assigns a registered instance to a layer. The target
// layer must exist and belong to the same stack as the instance -- AWS
// does not document cross-stack assignment as valid, and this backend
// previously accepted any layer ID (even one from an unrelated stack, or
// one that didn't exist at all) without checking. AssignInstance's doc
// comment (aws-sdk-go-v2/service/opsworks@v1.31.0's api_op_AssignInstance.go)
// also documents "You cannot use this action with instances that were
// created with OpsWorks Stacks" -- i.e. it only applies to RegisterInstance'd
// instances, never CreateInstance'd ones; storedInstance.Registered (set
// only by RegisterInstance) is what this backend already tracks that on.
func (b *InMemoryBackend) AssignInstance(instanceID string, layerIDs []string) error {
	b.mu.Lock("AssignInstance")
	defer b.mu.Unlock()

	i, ok := b.instances.Get(instanceID)
	if !ok {
		return ErrInstanceNotFound
	}

	if !i.Registered {
		return ErrValidation
	}

	if len(layerIDs) == 0 {
		return ErrValidation
	}

	layerID := layerIDs[0]

	l, ok := b.layers.Get(layerID)
	if !ok {
		return ErrLayerNotFound
	}

	if l.StackID != i.StackID {
		return ErrValidation
	}

	i.LayerID = layerID

	return nil
}

// UnassignInstance removes an instance from its layer.
func (b *InMemoryBackend) UnassignInstance(instanceID string) error {
	b.mu.Lock("UnassignInstance")
	defer b.mu.Unlock()

	i, ok := b.instances.Get(instanceID)
	if !ok {
		return ErrInstanceNotFound
	}

	i.LayerID = ""

	return nil
}

// DescribeInstances returns instances filtered by stack, layer, or IDs.
func (b *InMemoryBackend) DescribeInstances(stackID, layerID string, instanceIDs []string) ([]*Instance, error) {
	b.mu.RLock("DescribeInstances")
	defer b.mu.RUnlock()

	if len(instanceIDs) > 0 {
		result := make([]*Instance, 0, len(instanceIDs))
		for _, id := range instanceIDs {
			i, ok := b.instances.Get(id)
			if !ok {
				return nil, ErrInstanceNotFound
			}
			result = append(result, i.toInstance())
		}

		return result, nil
	}

	source := stackScoped(stackID, b.instances.All, b.instancesByStack.Get)

	result := make([]*Instance, 0, len(source))
	for _, i := range source {
		if layerID != "" && i.LayerID != layerID {
			continue
		}
		result = append(result, i.toInstance())
	}

	return result, nil
}

// UpdateInstance updates an instance's hostname.
func (b *InMemoryBackend) UpdateInstance(instanceID, hostname string) error {
	b.mu.Lock("UpdateInstance")
	defer b.mu.Unlock()

	i, ok := b.instances.Get(instanceID)
	if !ok {
		return ErrInstanceNotFound
	}

	if hostname != "" {
		i.Hostname = hostname
	}

	return nil
}

// DeleteInstance deletes an instance. AWS requires the instance be stopped
// first (api_op_DeleteInstance.go: "You must stop an instance before you
// can delete it.").
func (b *InMemoryBackend) DeleteInstance(instanceID string) error {
	b.mu.Lock("DeleteInstance")
	defer b.mu.Unlock()

	i, ok := b.instances.Get(instanceID)
	if !ok {
		return ErrInstanceNotFound
	}

	if i.Status != instanceStatusStopped {
		return ErrValidation
	}

	b.instances.Delete(instanceID)

	return nil
}

// StartInstance transitions an instance to online. Real AWS moves an
// instance through several transient states (requested, pending, booting,
// running_setup) before reaching online; this backend has no time-based
// scheduler, so it commits directly to the terminal state instead of
// leaving the instance parked in a transient status that nothing ever
// advances (previously "starting", which is not even a valid AWS OpsWorks
// instance-status value, and which left DescribeInstances pollers spinning
// forever since no code ever moved it on to "online").
func (b *InMemoryBackend) StartInstance(instanceID string) error {
	b.mu.Lock("StartInstance")
	defer b.mu.Unlock()

	i, ok := b.instances.Get(instanceID)
	if !ok {
		return ErrInstanceNotFound
	}

	i.Status = instanceStatusOnline

	return nil
}

// StopInstance transitions an instance to stopped. See StartInstance's doc
// comment for why this commits directly to the terminal state instead of
// parking the instance in the transient "stopping" status.
func (b *InMemoryBackend) StopInstance(instanceID string) error {
	b.mu.Lock("StopInstance")
	defer b.mu.Unlock()

	i, ok := b.instances.Get(instanceID)
	if !ok {
		return ErrInstanceNotFound
	}

	i.Status = instanceStatusStopped

	return nil
}

// RebootInstance transitions an instance back to online. See StartInstance's
// doc comment for why this commits directly to the terminal state instead of
// parking the instance in a transient status.
func (b *InMemoryBackend) RebootInstance(instanceID string) error {
	b.mu.Lock("RebootInstance")
	defer b.mu.Unlock()

	i, ok := b.instances.Get(instanceID)
	if !ok {
		return ErrInstanceNotFound
	}

	i.Status = instanceStatusOnline

	return nil
}
