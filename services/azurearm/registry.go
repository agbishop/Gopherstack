package azurearm

import (
	"context"
	"sort"
	"strings"
)

// ResourceProvider is the small interface a resource-provider namespace
// (Microsoft.Storage today; Microsoft.ServiceBus/Microsoft.DocumentDB in
// M8/M9, Microsoft.KeyVault/Microsoft.AppConfiguration in M11/M12) plugs
// into the registry with. Deliberately narrow (AZURE.md section 10.6) so
// each future milestone adds one file and one registry entry.
//
// Put/Get/Delete/ListKeys all return the full ARM wire response body (or nil
// for Delete's 204 case) -- the ResourceProvider owns its own response
// shape entirely, including the common id/name/type/location/tags/
// properties.provisioningState envelope (Resource.Body / a hand-built
// equivalent), rather than the registry re-wrapping it.
type ResourceProvider interface {
	// Namespace returns the ARM provider namespace this implementation
	// serves, e.g. "Microsoft.Storage".
	Namespace() string
	// ResourceTypes returns the resource types this provider serves, for
	// the /providers/{ns} registration-status response.
	ResourceTypes() []ResourceTypeDef
	// Put creates or updates the resource identified by id from body,
	// returning the full wire response body.
	Put(ctx context.Context, id ResourceID, body map[string]any) (map[string]any, error)
	// Get returns the full wire response body for id.
	Get(ctx context.Context, id ResourceID) (map[string]any, error)
	// Delete removes the resource identified by id.
	Delete(ctx context.Context, id ResourceID) error
	// List returns every resource of id's LeafType, scoped to id's
	// ResourceGroup if set, else the whole subscription.
	List(ctx context.Context, id ResourceID) ([]map[string]any, error)
	// ListKeys returns the POST .../listKeys response body for id.
	ListKeys(ctx context.Context, id ResourceID) (map[string]any, error)
	// Reset clears all of this provider's own state, called by
	// Handler.Reset via Registry.ResetAll (the /_gopherstack/reset
	// endpoint) -- without this, a dedicated provider's resources survive
	// a reset that's supposed to wipe every service's state.
	Reset()
	// DeleteResourcesInGroup deletes every resource this provider owns in
	// resourceGroup, called by Registry.DeleteResourcesInGroup when a
	// resource group is deleted -- without this, a dedicated provider's
	// resources in that group are orphaned (InMemoryBackend's own generic
	// resources are cascaded directly by DeleteResourceGroup, but a
	// dedicated provider's internal state is opaque to InMemoryBackend).
	DeleteResourcesInGroup(ctx context.Context, resourceGroup string)
}

// Registry dispatches generic-resource operations to the ResourceProvider
// registered for a namespace, falling back to InMemoryBackend's own
// generic (metadata-only) resource storage for any namespace with no
// dedicated provider -- so PUT/GET/DELETE of an arbitrary
// Microsoft.SomeFutureThing/whatever resource still round-trips instead of
// 404ing, per AZURE.md section 10.1's generic-resource-plane requirement.
type Registry struct {
	backend   *InMemoryBackend
	providers map[string]ResourceProvider
}

// NewRegistry creates a Registry backed by backend, with no providers
// registered yet (use Register).
func NewRegistry(backend *InMemoryBackend) *Registry {
	return &Registry{backend: backend, providers: make(map[string]ResourceProvider)}
}

// Register adds a ResourceProvider to the registry, keyed by its
// lowercased Namespace() -- ARM namespaces are matched case-insensitively
// (AZURE.md section 10.1: "Microsoft.storage" and "microsoft.STORAGE" must
// both reach the same dedicated provider, not fall through to the generic
// pass-through).
func (r *Registry) Register(p ResourceProvider) {
	r.providers[strings.ToLower(p.Namespace())] = p
}

// Providers returns the namespaces of every registered dedicated
// ResourceProvider, sorted, for the provider-list endpoint.
func (r *Registry) Providers() []string {
	out := make([]string, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p.Namespace())
	}

	sort.Strings(out)

	return out
}

// providerFor returns the dedicated ResourceProvider for id.Namespace, or
// nil if none is registered (generic pass-through applies).
func (r *Registry) providerFor(id ResourceID) ResourceProvider {
	return r.providers[strings.ToLower(id.Namespace)]
}

// ProviderNamed returns the dedicated ResourceProvider registered for ns
// (matched case-insensitively, AZURE.md section 10.1), and whether one was
// found. Used by the /providers/{ns} and /providers/{ns}/register handlers,
// which take ns directly from the URL path rather than a parsed ResourceID.
func (r *Registry) ProviderNamed(ns string) (ResourceProvider, bool) {
	p, ok := r.providers[strings.ToLower(ns)]

	return p, ok
}

// Put creates or updates the resource identified by id.
func (r *Registry) Put(ctx context.Context, id ResourceID, body map[string]any) (map[string]any, bool, error) {
	if p := r.providerFor(id); p != nil {
		_, getErr := p.Get(ctx, id)
		created := getErr != nil

		respBody, err := p.Put(ctx, id, body)

		return respBody, created, err
	}

	return r.backend.putGenericResource(id, body)
}

// Get returns the resource identified by id.
func (r *Registry) Get(ctx context.Context, id ResourceID) (map[string]any, error) {
	if p := r.providerFor(id); p != nil {
		return p.Get(ctx, id)
	}

	return r.backend.getGenericResource(id)
}

// Delete removes the resource identified by id.
func (r *Registry) Delete(ctx context.Context, id ResourceID) error {
	if p := r.providerFor(id); p != nil {
		return p.Delete(ctx, id)
	}

	return r.backend.deleteGenericResource(id)
}

// List returns every resource of id's LeafType, scoped per id.ResourceGroup.
func (r *Registry) List(ctx context.Context, id ResourceID) ([]map[string]any, error) {
	if p := r.providerFor(id); p != nil {
		return p.List(ctx, id)
	}

	return r.backend.listGenericResources(id), nil
}

// ListKeys returns the listKeys response body for id.
func (r *Registry) ListKeys(ctx context.Context, id ResourceID) (map[string]any, error) {
	if p := r.providerFor(id); p != nil {
		return p.ListKeys(ctx, id)
	}

	return nil, ErrResourceNotFound
}

// ResetAll clears every registered dedicated provider's own state. Called
// by Handler.Reset alongside InMemoryBackend.Reset -- the generic pass-through
// resources and dedicated-provider resources (e.g. StorageProvider.accounts)
// are two separate stores, and the /_gopherstack/reset endpoint must clear
// both.
func (r *Registry) ResetAll() {
	for _, p := range r.providers {
		p.Reset()
	}
}

// DeleteResourcesInGroup cascades a resource-group delete into every
// registered dedicated provider, so a provider's own resources in that
// group (e.g. StorageProvider's accounts) don't survive as orphans reachable
// by their own resource ID after the owning group is gone.
func (r *Registry) DeleteResourcesInGroup(ctx context.Context, resourceGroup string) {
	for _, p := range r.providers {
		p.DeleteResourcesInGroup(ctx, resourceGroup)
	}
}

// --- InMemoryBackend's generic (metadata-only) resource storage ---

// putGenericResource creates or updates a generic resource from body,
// merging body's top-level "tags" and "properties" fields (any other
// top-level field, e.g. "sku"/"kind", is preserved verbatim if the resource
// already exists via body carrying them again -- ARM PUT is a full replace
// semantically, matching this milestone's scope note that PATCH-merge
// beyond this is deferred).
func (b *InMemoryBackend) putGenericResource(id ResourceID, body map[string]any) (map[string]any, bool, error) {
	b.mu.Lock("putGenericResource")
	defer b.mu.Unlock()

	key := id.storeKey()
	_, existed := b.resources[key]

	location, _ := body["location"].(string)
	if location == "" {
		if existing, ok := b.resources[key]; ok {
			location = existing.Location
		}
	}

	props, _ := body["properties"].(map[string]any)

	res := &Resource{
		ID:         id,
		Location:   location,
		Tags:       stringTags(body["tags"]),
		Properties: props,
	}

	if sku, ok := body["sku"].(map[string]any); ok {
		res.SKU = sku
	}

	if kind, ok := body["kind"].(string); ok {
		res.Kind = kind
	}

	b.resources[key] = res

	return res.Body(), !existed, nil
}

func (b *InMemoryBackend) getGenericResource(id ResourceID) (map[string]any, error) {
	b.mu.RLock("getGenericResource")
	defer b.mu.RUnlock()

	res, ok := b.resources[id.storeKey()]
	if !ok {
		return nil, ErrResourceNotFound
	}

	return res.Body(), nil
}

func (b *InMemoryBackend) deleteGenericResource(id ResourceID) error {
	b.mu.Lock("deleteGenericResource")
	defer b.mu.Unlock()

	key := id.storeKey()
	if _, ok := b.resources[key]; !ok {
		return ErrResourceNotFound
	}

	delete(b.resources, key)

	return nil
}

func (b *InMemoryBackend) listGenericResources(id ResourceID) []map[string]any {
	b.mu.RLock("listGenericResources")
	defer b.mu.RUnlock()

	leafType := id.LeafType()

	var out []map[string]any

	for _, res := range b.resources {
		if !sameSubscriptionNamespaceType(res.ID, id, leafType) {
			continue
		}

		if id.ResourceGroup != "" && resourceGroupKey(res.ID.ResourceGroup) != resourceGroupKey(id.ResourceGroup) {
			continue
		}

		out = append(out, res.Body())
	}

	sort.Slice(out, func(i, j int) bool {
		return stringField(out[i], "id") < stringField(out[j], "id")
	})

	return out
}

// stringField safely reads a string field from a wire response body map,
// returning "" if absent or not a string -- avoids an unchecked type
// assertion in sort comparators.
func stringField(body map[string]any, key string) string {
	s, _ := body[key].(string)

	return s
}

func sameSubscriptionNamespaceType(res, want ResourceID, leafType string) bool {
	return res.SubscriptionID == want.SubscriptionID &&
		strings.EqualFold(res.Namespace, want.Namespace) &&
		strings.EqualFold(res.LeafType(), leafType)
}
