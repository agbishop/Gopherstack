package azurearm

import "sort"

// PutResourceGroup creates or updates the resource group named name. Always
// synchronous (AZURE.md section 10.3): the caller determines 200 vs 201 from
// the returned created bool.
func (b *InMemoryBackend) PutResourceGroup(name, location string, tags map[string]string) (ResourceGroup, bool) {
	b.mu.Lock("PutResourceGroup")
	defer b.mu.Unlock()

	key := resourceGroupKey(name)

	existing, ok := b.resourceGroups[key]
	created := !ok

	if location == "" && ok {
		location = existing.Location
	}

	group := &ResourceGroup{Name: name, Location: location, Tags: tags}
	b.resourceGroups[key] = group

	return *group, created
}

// GetResourceGroup returns the resource group named name.
func (b *InMemoryBackend) GetResourceGroup(name string) (ResourceGroup, error) {
	b.mu.RLock("GetResourceGroup")
	defer b.mu.RUnlock()

	group, ok := b.resourceGroups[resourceGroupKey(name)]
	if !ok {
		return ResourceGroup{}, ErrResourceGroupNotFound
	}

	return *group, nil
}

// DeleteResourceGroup deletes the resource group named name, along with
// every generic resource whose ResourceGroup matches (case-insensitively).
// Resources owned by a dedicated ResourceProvider's own internal state
// (e.g. storage accounts) are NOT cascaded here -- see registry.go's
// DeleteResourcesInGroup, which handler.go calls across every registered
// provider before calling this.
func (b *InMemoryBackend) DeleteResourceGroup(name string) error {
	b.mu.Lock("DeleteResourceGroup")
	defer b.mu.Unlock()

	key := resourceGroupKey(name)
	if _, ok := b.resourceGroups[key]; !ok {
		return ErrResourceGroupNotFound
	}

	delete(b.resourceGroups, key)

	for k, r := range b.resources {
		if resourceGroupKey(r.ID.ResourceGroup) == key {
			delete(b.resources, k)
		}
	}

	return nil
}

// ListResourceGroups returns every resource group, sorted by name.
func (b *InMemoryBackend) ListResourceGroups() []ResourceGroup {
	b.mu.RLock("ListResourceGroups")
	defer b.mu.RUnlock()

	out := make([]ResourceGroup, 0, len(b.resourceGroups))
	for _, g := range b.resourceGroups {
		out = append(out, *g)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// ResourceGroupExists reports whether a resource group named name exists.
func (b *InMemoryBackend) ResourceGroupExists(name string) bool {
	b.mu.RLock("ResourceGroupExists")
	defer b.mu.RUnlock()

	_, ok := b.resourceGroups[resourceGroupKey(name)]

	return ok
}
