package sesv2

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
)

// Response-map keys used by the tenant family. The tenant map already stores
// its fields PascalCase (matching the AWS wire shape directly, since these
// ops never had a persisted lowerCamelCase internal representation), so no
// separate wire_output.go DTO is needed here -- see the "keyStatus" doc
// comment in store.go for the same convention used elsewhere in this file.
const (
	keyTenantName          = "TenantName"
	keyTenantID            = "TenantId"
	keyTenantARN           = "TenantArn"
	keySendingStatus       = "SendingStatus"
	keyCreatedTimestamp    = "CreatedTimestamp"
	keyAssociatedTimestamp = "AssociatedTimestamp"
	keyResourceArn         = "ResourceArn"
	keyResourceType        = "ResourceType"
	keyTags                = "Tags"
	keySuppressedReasons   = "SuppressedReasons"
	keySuppressionScope    = "SuppressionScope"

	sendingStatusEnabled  = "ENABLED"
	sendingStatusDisabled = "DISABLED"

	// Real types.SuppressionListReason enum values.
	suppressionListReasonBounce    = "BOUNCE"
	suppressionListReasonComplaint = "COMPLAINT"

	// Real types.SuppressionListScope enum values.
	suppressionListScopeAccount = "ACCOUNT"
	suppressionListScopeTenant  = "TENANT"
)

// ---- tenants ----

// tenantARN builds the ARN for a tenant: arn:{partition}:ses:{region}:{account}:tenant/{name}.
func (b *InMemoryBackend) tenantARN(tenantName string) string {
	return arn.Build("ses", b.region, b.accountID, "tenant/"+tenantName)
}

// resourceTypeFromARN infers a SES v2 ResourceType (EMAIL_IDENTITY,
// CONFIGURATION_SET, EMAIL_TEMPLATE) from the resource segment of an ARN,
// matching the documented SES ARN resource prefixes. Returns "" if the ARN
// doesn't match a known resource kind.
func resourceTypeFromARN(resourceArn string) string {
	switch {
	case strings.Contains(resourceArn, ":identity/"):
		return "EMAIL_IDENTITY"
	case strings.Contains(resourceArn, ":configuration-set/"):
		return "CONFIGURATION_SET"
	case strings.Contains(resourceArn, ":template/"):
		return "EMAIL_TEMPLATE"
	default:
		return ""
	}
}

// CreateTenant creates a new tenant. Returns AlreadyExistsException-shaped
// error if the tenant already exists (matches real SES v2 behavior).
func (b *InMemoryBackend) CreateTenant(tenantName string, tags map[string]string) (*tenantOutput, error) {
	b.mu.Lock("CreateTenant")
	defer b.mu.Unlock()

	if _, exists := b.tenants[tenantName]; exists {
		return nil, fmt.Errorf("%w: Tenant %s already exists", ErrAlreadyExists, tenantName)
	}

	resourceARN := b.tenantARN(tenantName)

	t := map[string]any{
		keyTenantName:       tenantName,
		keyTenantID:         "tenant-" + uuid.New().String(),
		keyTenantARN:        resourceARN,
		keySendingStatus:    sendingStatusEnabled,
		keyCreatedTimestamp: awstime.Epoch(time.Now()),
	}

	if len(tags) > 0 {
		t[keyTags] = tagsToEntries(tags)
		b.putResourceTagsLocked(resourceARN, tags)
	}

	b.tenants[tenantName] = t

	return toTenantOutput(t), nil
}

// GetTenant returns the full tenant record (TenantName/TenantId/TenantArn/
// SendingStatus/CreatedTimestamp/Tags), matching types.Tenant.
func (b *InMemoryBackend) GetTenant(tenantName string) (*tenantOutput, error) {
	b.mu.RLock("GetTenant")
	defer b.mu.RUnlock()

	t, ok := b.tenants[tenantName]
	if !ok {
		return nil, fmt.Errorf("%w: Tenant %s not found", ErrNotFound, tenantName)
	}

	out := toTenantOutput(t)
	out.Tags = tagsToEntries(b.liveTagsLocked(b.tenantARN(tenantName)))

	return out, nil
}

// DeleteTenant removes a tenant and cascades cleanup of its resource
// associations (both directions of the tenant<->resource index) so no ghost
// rows remain after delete.
func (b *InMemoryBackend) DeleteTenant(tenantName string) error {
	b.mu.Lock("DeleteTenant")
	defer b.mu.Unlock()

	delete(b.tenants, tenantName)
	delete(b.resourceTags, b.tenantARN(tenantName))

	for _, resourceArn := range b.tenantResources[tenantName] {
		b.resourceTenants[resourceArn] = removeString(b.resourceTenants[resourceArn], tenantName)

		if len(b.resourceTenants[resourceArn]) == 0 {
			delete(b.resourceTenants, resourceArn)
		}
	}

	delete(b.tenantResources, tenantName)

	return nil
}

// tenantInfo projects a full tenant record down to the types.TenantInfo shape
// used by ListTenants (no SendingStatus, no Tags).
func tenantInfo(t map[string]any) map[string]any {
	info := map[string]any{keyTenantName: t[keyTenantName]}

	for _, k := range []string{keyTenantID, keyTenantARN, keyCreatedTimestamp} {
		if v, ok := t[k]; ok {
			info[k] = v
		}
	}

	return info
}

// ListTenants lists all tenants associated with the account, paginated.
func (b *InMemoryBackend) ListTenants(nextToken string, pageSize int) ([]tenantInfoOutput, string, error) {
	b.mu.RLock("ListTenants")

	all := make([]map[string]any, 0, len(b.tenants))
	for _, t := range b.tenants {
		all = append(all, tenantInfo(t))
	}

	b.mu.RUnlock()

	sortMapsByStringKey(all, keyTenantName)

	page, next, err := paginateMaps(all, nextToken, pageSize, keyTenantName)
	if err != nil {
		return nil, "", err
	}

	out := make([]tenantInfoOutput, 0, len(page))
	for _, t := range page {
		out = append(out, toTenantInfoOutput(t))
	}

	return out, next, nil
}

// PutTenantSuppressionAttributes configures the suppression-list preferences
// for a tenant (which reasons auto-suppress a destination, and whether the
// tenant uses its own suppression list or the account-level one). Returns
// NotFoundException if the tenant doesn't exist, matching real SES v2.
func (b *InMemoryBackend) PutTenantSuppressionAttributes(
	tenantName string,
	suppressedReasons []string,
	suppressionScope string,
) error {
	for _, r := range suppressedReasons {
		if r != suppressionListReasonBounce && r != suppressionListReasonComplaint {
			return fmt.Errorf(
				"%w: SuppressedReasons entries must be %s or %s, got %q",
				ErrInvalidInput, suppressionListReasonBounce, suppressionListReasonComplaint, r,
			)
		}
	}

	if suppressionScope != "" && suppressionScope != suppressionListScopeAccount &&
		suppressionScope != suppressionListScopeTenant {
		return fmt.Errorf(
			"%w: SuppressionScope must be %s or %s, got %q",
			ErrInvalidInput, suppressionListScopeAccount, suppressionListScopeTenant, suppressionScope,
		)
	}

	b.mu.Lock("PutTenantSuppressionAttributes")
	defer b.mu.Unlock()

	t, ok := b.tenants[tenantName]
	if !ok {
		return fmt.Errorf("%w: Tenant %s not found", ErrNotFound, tenantName)
	}

	reasons := make([]string, len(suppressedReasons))
	copy(reasons, suppressedReasons)
	t[keySuppressedReasons] = reasons
	t[keySuppressionScope] = suppressionScope

	return nil
}

// CreateTenantResourceAssociation associates a resource with a tenant.
// Real SES v2 returns NotFoundException if the tenant doesn't exist.
func (b *InMemoryBackend) CreateTenantResourceAssociation(tenantName, resourceArn string) error {
	b.mu.Lock("CreateTenantResourceAssociation")
	defer b.mu.Unlock()

	if _, ok := b.tenants[tenantName]; !ok {
		return fmt.Errorf("%w: Tenant %s not found", ErrNotFound, tenantName)
	}

	b.tenantResources[tenantName] = append(b.tenantResources[tenantName], resourceArn)
	b.resourceTenants[resourceArn] = append(b.resourceTenants[resourceArn], tenantName)

	return nil
}

// DeleteTenantResourceAssociation removes a tenant<->resource association.
func (b *InMemoryBackend) DeleteTenantResourceAssociation(tenantName, resourceArn string) error {
	b.mu.Lock("DeleteTenantResourceAssociation")
	defer b.mu.Unlock()

	b.tenantResources[tenantName] = removeString(b.tenantResources[tenantName], resourceArn)
	b.resourceTenants[resourceArn] = removeString(b.resourceTenants[resourceArn], tenantName)

	return nil
}

// ListResourceTenants lists the tenants a resource is associated with,
// matching types.ResourceTenantMetadata (TenantName/TenantId/ResourceArn/
// AssociatedTimestamp). Note real SES v2 doesn't track a TenantId lookup
// separately from the tenant record; we derive it from the tenant map when
// available.
func (b *InMemoryBackend) ListResourceTenants(
	resourceArn, nextToken string,
	pageSize int,
) ([]resourceTenantOutput, string, error) {
	b.mu.RLock("ListResourceTenants")

	names := b.resourceTenants[resourceArn]

	all := make([]map[string]any, 0, len(names))

	for _, n := range names {
		item := map[string]any{
			keyTenantName:          n,
			keyResourceArn:         resourceArn,
			keyAssociatedTimestamp: awstime.Epoch(time.Now()),
		}
		if t, ok := b.tenants[n]; ok {
			if id, hasID := t[keyTenantID]; hasID {
				item[keyTenantID] = id
			}
		}

		all = append(all, item)
	}

	b.mu.RUnlock()

	sortMapsByStringKey(all, keyTenantName)

	page, next, err := paginateMaps(all, nextToken, pageSize, keyTenantName)
	if err != nil {
		return nil, "", err
	}

	out := make([]resourceTenantOutput, 0, len(page))
	for _, t := range page {
		out = append(out, toResourceTenantOutput(t))
	}

	return out, next, nil
}

// ListTenantResources lists the resources associated with a tenant, matching
// types.TenantResource (ResourceArn/ResourceType only -- no timestamp). The
// only supported filter key is RESOURCE_TYPE (types.ListTenantResourcesFilterKey).
func (b *InMemoryBackend) ListTenantResources(
	tenantName string,
	filter map[string]string,
	nextToken string,
	pageSize int,
) ([]tenantResourceOutput, string, error) {
	b.mu.RLock("ListTenantResources")
	arns := b.tenantResources[tenantName]
	b.mu.RUnlock()

	wantType := filter["RESOURCE_TYPE"]

	all := make([]map[string]any, 0, len(arns))

	for _, a := range arns {
		rt := resourceTypeFromARN(a)
		if wantType != "" && rt != wantType {
			continue
		}

		item := map[string]any{keyResourceArn: a}
		if rt != "" {
			item[keyResourceType] = rt
		}

		all = append(all, item)
	}

	sortMapsByStringKey(all, keyResourceArn)

	page, next, err := paginateMaps(all, nextToken, pageSize, keyResourceArn)
	if err != nil {
		return nil, "", err
	}

	out := make([]tenantResourceOutput, 0, len(page))
	for _, t := range page {
		out = append(out, toTenantResourceOutput(t))
	}

	return out, next, nil
}

// sortMapsByStringKey sorts a slice of response maps by a string-valued key
// so paginateMaps' NextToken cursor is stable across calls.
func sortMapsByStringKey(items []map[string]any, key string) {
	sort.Slice(items, func(i, j int) bool {
		si, _ := items[i][key].(string)
		sj, _ := items[j][key].(string)

		return si < sj
	})
}

func removeString(s []string, v string) []string {
	out := s[:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}

	return out
}
