package azurearm

import (
	"fmt"
	"strings"
)

// ResourceID is a parsed ARM resource identifier of the generic shape
// /subscriptions/{sub}/resourceGroups/{rg}/providers/{namespace}/{type1}/{name1}[/{type2}/{name2}...].
//
// Types/Names are parallel slices so nested child resources (e.g.
// Microsoft.ServiceBus/namespaces/{ns}/queues/{q}, two type/name pairs) are
// represented without a separate "parent" concept -- exactly the single
// generic path walker AZURE.md section 10.1 requires instead of per-type
// route registration.
type ResourceID struct {
	SubscriptionID string
	ResourceGroup  string
	Namespace      string
	Types          []string
	Names          []string
}

// resourceGroupsSegment is the canonical (mixed-case) rendering of the
// "resourcegroups"/"resourceGroups" path segment. Real ARM, and azurerm
// itself, accept either case on the way in; this is what's rendered back
// out and what parsing normalizes to.
const resourceGroupsSegment = "resourceGroups"

const providersSegment = "providers"

const subscriptionsSegment = "subscriptions"

// splitARMPath splits an ARM request path into its non-empty segments.
func splitARMPath(path string) []string {
	raw := strings.Split(path, "/")
	segs := make([]string, 0, len(raw))

	for _, s := range raw {
		if s != "" {
			segs = append(segs, s)
		}
	}

	return segs
}

// ParseGenericResourcePath parses a full generic-resource ARM path:
//
//	/subscriptions/{sub}/resourceGroups/{rg}/providers/{ns}/{type}/{name}[/{type}/{name}...]
//
// The "resourcegroups"/"resourceGroups" segment is matched case-insensitively
// per AZURE.md section 10.1 -- real ARM accepts both and azurerm emits both
// depending on code path.
func ParseGenericResourcePath(path string) (ResourceID, error) {
	segs := splitARMPath(path)

	const minSegs = 7 // subscriptions/{sub}/resourceGroups/{rg}/providers/{ns}/{type1}

	if len(segs) < minSegs {
		return ResourceID{}, fmt.Errorf("%w: %q", ErrInvalidResourceID, path)
	}

	if !strings.EqualFold(segs[0], subscriptionsSegment) {
		return ResourceID{}, fmt.Errorf("%w: %q", ErrInvalidResourceID, path)
	}

	if !strings.EqualFold(segs[2], resourceGroupsSegment) {
		return ResourceID{}, fmt.Errorf("%w: %q", ErrInvalidResourceID, path)
	}

	if !strings.EqualFold(segs[4], providersSegment) {
		return ResourceID{}, fmt.Errorf("%w: %q", ErrInvalidResourceID, path)
	}

	rest := segs[6:]
	if len(rest) == 0 || len(rest)%2 != 0 {
		return ResourceID{}, fmt.Errorf("%w: %q", ErrInvalidResourceID, path)
	}

	types := make([]string, 0, len(rest)/2)
	names := make([]string, 0, len(rest)/2)

	for i := 0; i < len(rest); i += 2 {
		types = append(types, rest[i])
		names = append(names, rest[i+1])
	}

	return ResourceID{
		SubscriptionID: segs[1],
		ResourceGroup:  segs[3],
		Namespace:      segs[5],
		Types:          types,
		Names:          names,
	}, nil
}

// ParseGenericResourceListPath parses a resource-type LIST path, either
// scoped to a resource group:
//
//	/subscriptions/{sub}/resourceGroups/{rg}/providers/{ns}/{type}
//
// or across the whole subscription:
//
//	/subscriptions/{sub}/providers/{ns}/{type}
//
// Returns the parsed (partial) ResourceID -- Names is empty -- and whether a
// resource group scope was present.
func ParseGenericResourceListPath(path string) (id ResourceID, hasResourceGroup bool, err error) {
	segs := splitARMPath(path)

	// Subscription-scoped: subscriptions/{sub}/providers/{ns}/{type}
	const subScopedLen = 5
	if len(segs) == subScopedLen &&
		strings.EqualFold(segs[0], subscriptionsSegment) &&
		strings.EqualFold(segs[2], providersSegment) {
		return ResourceID{
			SubscriptionID: segs[1],
			Namespace:      segs[3],
			Types:          []string{segs[4]},
		}, false, nil
	}

	// Resource-group-scoped: subscriptions/{sub}/resourceGroups/{rg}/providers/{ns}/{type}
	const rgScopedLen = 7
	if len(segs) == rgScopedLen &&
		strings.EqualFold(segs[0], subscriptionsSegment) &&
		strings.EqualFold(segs[2], resourceGroupsSegment) &&
		strings.EqualFold(segs[4], providersSegment) {
		return ResourceID{
			SubscriptionID: segs[1],
			ResourceGroup:  segs[3],
			Namespace:      segs[5],
			Types:          []string{segs[6]},
		}, true, nil
	}

	return ResourceID{}, false, fmt.Errorf("%w: %q", ErrInvalidResourceID, path)
}

// ResourceType returns the ARM resource type string, e.g.
// "Microsoft.Storage/storageAccounts" or
// "Microsoft.ServiceBus/namespaces/queues".
func (id ResourceID) ResourceType() string {
	return id.Namespace + "/" + strings.Join(id.Types, "/")
}

// LeafType returns the last (innermost) resource type segment.
func (id ResourceID) LeafType() string {
	if len(id.Types) == 0 {
		return ""
	}

	return id.Types[len(id.Types)-1]
}

// LeafName returns the last (innermost) resource name segment -- the name of
// the resource this ID actually identifies.
func (id ResourceID) LeafName() string {
	if len(id.Names) == 0 {
		return ""
	}

	return id.Names[len(id.Names)-1]
}

// ARMID renders the canonical ARM resource ID string for id, always using
// the canonical "resourceGroups" casing regardless of what was parsed.
func (id ResourceID) ARMID() string {
	var b strings.Builder

	fmt.Fprintf(&b, "/subscriptions/%s/resourceGroups/%s/providers/%s",
		id.SubscriptionID, id.ResourceGroup, id.Namespace)

	for i := range id.Types {
		fmt.Fprintf(&b, "/%s/%s", id.Types[i], id.Names[i])
	}

	return b.String()
}

// storeKey returns the case-insensitive lookup key used by InMemoryBackend's
// resource map -- ARM resource IDs are matched case-insensitively across
// subscription/resource-group/namespace/type, mirroring real ARM and
// resource-group name matching specifically (AZURE.md section 10.1).
func (id ResourceID) storeKey() string {
	return strings.ToLower(id.ARMID())
}
