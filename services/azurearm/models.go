package azurearm

import "context"

// requestHostContextKey is the context key WithRequestHost/
// RequestHostFromContext use to thread the ARM request's own Host header
// hostname (no port) through to a ResourceProvider's Put/Get/List, which the
// ResourceProvider interface's signature otherwise has no room for. Used by
// rp_storage.go to implement AZURE.md section 10.4's endpoint-advertisement
// design without changing the shared ResourceProvider interface.
type requestHostContextKey struct{}

// WithRequestHost returns a copy of ctx carrying host (the ARM request's own
// Host header, hostname only, no port).
func WithRequestHost(ctx context.Context, host string) context.Context {
	return context.WithValue(ctx, requestHostContextKey{}, host)
}

// RequestHostFromContext returns the host stashed by WithRequestHost, or ""
// if none was set (e.g. a unit test calling a ResourceProvider directly).
func RequestHostFromContext(ctx context.Context) string {
	host, _ := ctx.Value(requestHostContextKey{}).(string)

	return host
}

// ResourceGroup is the stored representation of an ARM resource group.
type ResourceGroup struct {
	Name     string            `json:"name"`
	Location string            `json:"location"`
	Tags     map[string]string `json:"tags,omitempty"`
}

// Body returns the ARM wire response body for g, scoped to subscriptionID.
func (g ResourceGroup) Body(subscriptionID string) map[string]any {
	return map[string]any{
		"id":       "/subscriptions/" + subscriptionID + "/resourceGroups/" + g.Name,
		"name":     g.Name,
		"type":     "Microsoft.Resources/resourceGroups",
		"location": g.Location,
		"tags":     tagsOrEmpty(g.Tags),
		"properties": map[string]any{
			"provisioningState": provisioningStateSucceeded,
		},
	}
}

// Resource is the generic stored representation of an ARM resource (any
// namespace/type not owned by a specific ResourceProvider -- see registry.go's
// fallback behavior). ResourceProvider implementations (e.g. rp_storage.go)
// may keep their own richer internal representation and build the wire body
// directly instead of using this type; Resource exists for the generic
// pass-through path every namespace gets even without a dedicated RP.
type Resource struct {
	ID         ResourceID
	Location   string
	Tags       map[string]string
	Properties map[string]any
	SKU        map[string]any
	Kind       string
}

// provisioningStateSucceeded is the only provisioningState this emulator
// ever returns -- ARM is always synchronous here (AZURE.md section 10.3).
const provisioningStateSucceeded = "Succeeded"

// Body returns the ARM wire response body for r.
func (r Resource) Body() map[string]any {
	props := map[string]any{}

	for k, v := range r.Properties {
		props[k] = v
	}

	props["provisioningState"] = provisioningStateSucceeded

	body := map[string]any{
		"id":         r.ID.ARMID(),
		"name":       r.ID.LeafName(),
		"type":       r.ID.ResourceType(),
		"location":   r.Location,
		"tags":       tagsOrEmpty(r.Tags),
		"properties": props,
	}

	if len(r.SKU) > 0 {
		body["sku"] = r.SKU
	}

	if r.Kind != "" {
		body["kind"] = r.Kind
	}

	return body
}

// tagsOrEmpty returns tags, or an empty (non-nil) map, so the wire body's
// "tags" field always serializes as {} rather than null.
func tagsOrEmpty(tags map[string]string) map[string]string {
	if tags == nil {
		return map[string]string{}
	}

	return tags
}

// stringTags coerces an arbitrary JSON-decoded "tags" field (map[string]any,
// from a request body) into map[string]string, dropping non-string values.
func stringTags(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}

	out := make(map[string]string, len(m))

	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}

	return out
}

// ResourceTypeDef describes one resource type a ResourceProvider serves --
// its type name, supported API versions, and whether it has child resource
// types nested beneath it (informational only in this MVP; see registry.go).
type ResourceTypeDef struct {
	Type        string
	APIVersions []string
	HasChildren bool
}
