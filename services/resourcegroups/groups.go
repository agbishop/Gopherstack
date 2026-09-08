package resourcegroups

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// groupNameMaxLen and groupDescMaxLen match AWS limits.
const (
	groupNameMaxLen = 300
	groupDescMaxLen = 512
)

const configParamAllowedResourceTypes = "allowed-resource-types"

// ListGroupsFilterName constants for ListGroups Filters field. These are the
// exact five values of the real types.GroupFilterName enum -- there is no
// "name-prefix" filter in the real API (a prior gopherstack-only invention
// has been removed; see PARITY.md).
const (
	listGroupsFilterConfigurationType = "configuration-type"
	listGroupsFilterResourceType      = "resource-type"
	listGroupsFilterOwner             = "owner"
	listGroupsFilterDisplayName       = "display-name"
	listGroupsFilterCriticality       = "criticality"
)

// groupCriticalityMaxValue matches the real CreateGroup/UpdateGroup API docs:
// "a scale of 1 to 10, with a rank of 1 being the most critical, and a rank
// of 10 being least critical." 0 means "not provided" and is left unset
// (CreateGroup) or unchanged (UpdateGroup).
const groupCriticalityMaxValue = 10

// groupNameRe matches valid Resource Groups group names (AWS rule).
var groupNameRe = regexp.MustCompile(`^[a-zA-Z0-9_.−\-]+$`)

// groupNameReservedPrefixes lists prefixes that AWS does not allow for group names.
var groupNameReservedPrefixes = []string{ //nolint:gochecknoglobals // lookup table, initialized once
	"aws",
	"AWS",
}

// validResourceQueryTypes lists the only two supported query types.
var validResourceQueryTypes = map[string]bool{ //nolint:gochecknoglobals // lookup table, initialized once
	"TAG_FILTERS_1_0":          true,
	"CLOUDFORMATION_STACK_1_0": true,
}

// validConfigTypes maps each recognized configuration Type to its allowed
// parameter names.  An empty slice means the type takes no parameters.
var validConfigTypes = map[string][]string{ //nolint:gochecknoglobals // lookup table, initialized once
	"AWS::EC2::HostManagement": {
		configParamAllowedResourceTypes,
		"any-of-allowed-resource-types",
		"deletion-protection",
	},
	"AWS::EC2::CapacityReservationPool": {},
	"AWS::ResourceGroups::Generic": {
		configParamAllowedResourceTypes,
		"any-of-allowed-resource-types",
	},
	"AWS::AppRegistry::Application":               {configParamAllowedResourceTypes},
	"AWS::NetworkFirewall::RuleGroup":             {configParamAllowedResourceTypes},
	"AWS::Route53Resolver::FirewallRuleGroup":     {configParamAllowedResourceTypes},
	"AWS::ServiceCatalogAppRegistry::Application": {configParamAllowedResourceTypes},
}

// validateGroupName validates that a group name conforms to AWS naming rules.
func validateGroupName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: Name is required", ErrValidation)
	}

	if len(name) > groupNameMaxLen {
		return fmt.Errorf("%w: Name must be at most %d characters", ErrValidation, groupNameMaxLen)
	}

	if !groupNameRe.MatchString(name) {
		return fmt.Errorf("%w: Name must match pattern [a-zA-Z0-9_.−-]+", ErrValidation)
	}

	nameLower := strings.ToLower(name)
	for _, prefix := range groupNameReservedPrefixes {
		if strings.HasPrefix(nameLower, strings.ToLower(prefix)) {
			return fmt.Errorf(
				"%w: Name must not start with reserved prefix %q",
				ErrValidation,
				prefix,
			)
		}
	}

	return nil
}

// validateDescription validates that a description conforms to AWS length rules.
func validateDescription(desc string) error {
	if len(desc) > groupDescMaxLen {
		return fmt.Errorf(
			"%w: Description must be at most %d characters",
			ErrValidation,
			groupDescMaxLen,
		)
	}

	return nil
}

// validateCriticality validates the optional Criticality field shared by
// CreateGroup and UpdateGroup. 0 means "not provided".
func validateCriticality(criticality int) error {
	if criticality != 0 && (criticality < 1 || criticality > groupCriticalityMaxValue) {
		return fmt.Errorf(
			"%w: Criticality must be between 1 and %d",
			ErrValidation,
			groupCriticalityMaxValue,
		)
	}

	return nil
}

// validateResourceQuery validates that a ResourceQuery is well-formed.
func validateResourceQuery(q *ResourceQuery) error {
	if q == nil {
		return nil
	}

	if !validResourceQueryTypes[q.Type] {
		return fmt.Errorf(
			"%w: ResourceQuery.Type must be TAG_FILTERS_1_0 or CLOUDFORMATION_STACK_1_0, got %q",
			ErrValidation,
			q.Type,
		)
	}

	if q.Query == "" {
		return fmt.Errorf("%w: ResourceQuery.Query must be a non-empty JSON string", ErrValidation)
	}

	var raw json.RawMessage
	if err := json.Unmarshal([]byte(q.Query), &raw); err != nil {
		return fmt.Errorf(
			"%w: ResourceQuery.Query is not valid JSON: %s",
			ErrValidation,
			err.Error(),
		)
	}

	return nil
}

// validateConfiguration validates each GroupConfigurationItem against the allow-list.
func validateConfiguration(items []GroupConfigurationItem) error {
	for _, item := range items {
		allowedParams, ok := validConfigTypes[item.Type]
		if !ok {
			return fmt.Errorf(
				"%w: unsupported configuration type %q; must be one of AWS::EC2::HostManagement, "+
					"AWS::EC2::CapacityReservationPool, AWS::ResourceGroups::Generic, "+
					"AWS::AppRegistry::Application, etc",
				ErrValidation,
				item.Type,
			)
		}

		if len(allowedParams) == 0 && len(item.Parameters) > 0 {
			return fmt.Errorf(
				"%w: configuration type %q does not accept any parameters",
				ErrValidation,
				item.Type,
			)
		}

		allowed := make(map[string]bool, len(allowedParams))
		for _, p := range allowedParams {
			allowed[p] = true
		}

		for _, param := range item.Parameters {
			if !allowed[param.Name] {
				return fmt.Errorf(
					"%w: parameter %q is not valid for configuration type %q",
					ErrValidation,
					param.Name,
					item.Type,
				)
			}
		}
	}

	return nil
}

// CreateGroupOption customizes optional CreateGroup fields (Owner,
// DisplayName, Criticality) that the real CreateGroupInput supports
// alongside the required Name/Description/ResourceQuery/Tags/Configuration
// parameters. Modeled as functional options (rather than widening
// CreateGroup's positional parameter list) to avoid breaking every existing
// call site for what are, on the wire, optional fields.
type CreateGroupOption func(*Group)

// WithOwner sets the Owner field at group-creation time.
func WithOwner(owner string) CreateGroupOption {
	return func(g *Group) { g.Owner = owner }
}

// WithDisplayName sets the DisplayName field at group-creation time.
func WithDisplayName(displayName string) CreateGroupOption {
	return func(g *Group) { g.DisplayName = displayName }
}

// WithCriticality sets the Criticality field at group-creation time.
func WithCriticality(criticality int) CreateGroupOption {
	return func(g *Group) { g.Criticality = criticality }
}

// CreateGroup creates a new resource group.
// The Tags field in the returned Group points to a fresh Tags copy; it is
// safe to read but callers should not pass it back to mutation methods.
// configuration is optional; when non-nil it is stored atomically with the group.
// opts sets the optional Owner/DisplayName/Criticality fields (all unset by default).
func (b *InMemoryBackend) CreateGroup(
	ctx context.Context,
	name, description string,
	resourceQuery *ResourceQuery,
	inputTags *tags.Tags,
	configuration []GroupConfigurationItem,
	opts ...CreateGroupOption,
) (*Group, error) {
	if err := validateGroupName(name); err != nil {
		return nil, err
	}

	if err := validateDescription(description); err != nil {
		return nil, err
	}

	identity := &Group{}
	for _, opt := range opts {
		opt(identity)
	}

	if err := validateCriticality(identity.Criticality); err != nil {
		return nil, err
	}

	if err := validateResourceQuery(resourceQuery); err != nil {
		return nil, err
	}

	if len(configuration) > 0 {
		if err := validateConfiguration(configuration); err != nil {
			return nil, err
		}

		// AWS rejects groups that specify both a ResourceQuery and a Configuration.
		if resourceQuery != nil {
			return nil, fmt.Errorf(
				"%w: a group cannot have both a ResourceQuery and a Configuration; "+
					"use one or the other",
				ErrValidation,
			)
		}
	}

	if inputTags != nil {
		tagMap := inputTags.Clone()
		if err := validateTagKeys(tagMap); err != nil {
			return nil, err
		}
	}

	b.mu.Lock("CreateGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	tableKey := regionKey(region, name)

	if b.groups.Has(tableKey) {
		return nil, fmt.Errorf("%w: group %s already exists", ErrAlreadyExists, name)
	}

	groupARN := arn.Build("resource-groups", region, b.accountID, "group/"+name)

	// Clone caller-provided tags into a backend-owned collection so that the
	// caller cannot mutate backend state by keeping a reference to inputTags.
	var backendTags *tags.Tags
	if inputTags == nil {
		backendTags = tags.New("rg." + name + ".tags")
	} else {
		backendTags = tags.FromMap("rg."+name+".tags", inputTags.Clone())
	}

	g := &Group{
		Name:          name,
		ARN:           groupARN,
		Description:   description,
		Tags:          backendTags,
		ResourceQuery: resourceQuery,
		Owner:         identity.Owner,
		DisplayName:   identity.DisplayName,
		Criticality:   identity.Criticality,
	}
	b.groups.Put(g)

	if len(configuration) > 0 {
		b.groupConfigurationsStore(region)[name] = cloneConfigItems(configuration)
	}

	cp := *g

	return &cp, nil
}

// GetGroup returns a resource group by name or ARN.
func (b *InMemoryBackend) GetGroup(ctx context.Context, nameOrARN string) (*Group, error) {
	b.mu.RLock("GetGroup")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	name := resolveGroupName(nameOrARN)

	g, ok := b.groups.Get(regionKey(region, name))
	if !ok {
		return nil, fmt.Errorf("%w: group %s not found", ErrNotFound, name)
	}

	cp := *g

	return &cp, nil
}

// UpdateGroup updates the description, display name, owner, and criticality
// of a resource group. Pass an empty description/displayName/owner to leave
// it unchanged (UpdateGroupInput.Description is optional, per
// api_op_UpdateGroup.go: no validateOpUpdateGroupInput required-field check
// exists for it, unlike CreateGroup).
// Pass criticality=0 to leave it unchanged. Criticality must be 1-10 if non-zero.
func (b *InMemoryBackend) UpdateGroup(
	ctx context.Context,
	nameOrARN, description, displayName, owner string,
	criticality int,
) (*Group, error) {
	if err := validateDescription(description); err != nil {
		return nil, err
	}

	if err := validateCriticality(criticality); err != nil {
		return nil, err
	}

	b.mu.Lock("UpdateGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	name := resolveGroupName(nameOrARN)

	g, ok := b.groups.Get(regionKey(region, name))
	if !ok {
		return nil, fmt.Errorf("%w: group %s not found", ErrNotFound, name)
	}

	if description != "" {
		g.Description = description
	}

	if displayName != "" {
		g.DisplayName = displayName
	}

	if criticality != 0 {
		g.Criticality = criticality
	}

	if owner != "" {
		g.Owner = owner
	}

	cp := *g

	return &cp, nil
}

// UpdateGroupQuery updates the resource query of a resource group identified by name or ARN.
func (b *InMemoryBackend) UpdateGroupQuery(
	ctx context.Context,
	nameOrARN string,
	query *ResourceQuery,
) (*Group, error) {
	if err := validateResourceQuery(query); err != nil {
		return nil, err
	}

	b.mu.Lock("UpdateGroupQuery")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	name := resolveGroupName(nameOrARN)

	g, ok := b.groups.Get(regionKey(region, name))
	if !ok {
		return nil, fmt.Errorf("%w: group %s not found", ErrNotFound, name)
	}

	// api_op_UpdateGroupQuery.go:42-43: "A resource group can contain either
	// a Configuration or a ResourceQuery, but not both."
	if len(b.groupConfigurations[region][name]) > 0 {
		return nil, fmt.Errorf(
			"%w: a group cannot have both a ResourceQuery and a Configuration; "+
				"use one or the other",
			ErrValidation,
		)
	}

	g.ResourceQuery = query
	cp := *g

	return &cp, nil
}

// DeleteGroup deletes a resource group by name or ARN, returning a copy of
// the group as it existed immediately before deletion (matching AWS, whose
// DeleteGroupOutput echoes back the deleted group's description).
// It cascades to remove all associated resources, configurations,
// grouping-status records, and tag-sync tasks for the group.
func (b *InMemoryBackend) DeleteGroup(ctx context.Context, nameOrARN string) (*Group, error) {
	b.mu.Lock("DeleteGroup")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	name := resolveGroupName(nameOrARN)
	tableKey := regionKey(region, name)

	g, ok := b.groups.Get(tableKey)
	if !ok {
		return nil, fmt.Errorf("%w: group %s not found", ErrNotFound, name)
	}

	cp := *g

	b.groups.Delete(tableKey)
	g.Tags.Close()

	// Cascade: remove all derived state for this group.
	if b.groupResources[region] != nil {
		delete(b.groupResources[region], name)
	}

	if b.groupingStatuses[region] != nil {
		delete(b.groupingStatuses[region], name)
	}

	if b.groupConfigurations[region] != nil {
		delete(b.groupConfigurations[region], name)
	}

	// Cancel any tag-sync tasks bound to this group. slices.Clone the index
	// result first: Table.Delete mutates the index's backing slice in place,
	// which would otherwise corrupt this in-progress range.
	for _, task := range slices.Clone(b.tagSyncTasksByRegion.Get(region)) {
		if task.GroupName == name {
			b.tagSyncTasks.Delete(regionKey(region, task.TaskArn))
		}
	}

	return &cp, nil
}

// ListGroups returns resource groups sorted by name, optionally filtered and paginated.
// Supported filter names: "configuration-type", "resource-type", "owner",
// "display-name", "criticality" (the exact types.GroupFilterName enum).
// An empty filters slice returns all groups (up to maxResults).
// Returns the page of groups and a continuation token (empty when no more results).
func (b *InMemoryBackend) ListGroups(
	ctx context.Context,
	filters []ListGroupsFilter,
	nextToken string,
	maxResults int,
) ([]Group, string) {
	b.mu.RLock("ListGroups")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	regionGroups := b.groupsByRegion.Get(region)
	out := make([]Group, 0, len(regionGroups))

	for _, g := range regionGroups {
		if !b.groupMatchesFilters(region, g, filters) {
			continue
		}

		cp := *g
		cp.Tags = nil
		out = append(out, cp)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	page, token := paginate(out, func(g Group) string { return g.Name }, nextToken, maxResults)

	return page, token
}

// groupMatchesFilters returns true when a group satisfies all provided filter criteria.
// Must be called under an active read lock.
func (b *InMemoryBackend) groupMatchesFilters(region string, g *Group, filters []ListGroupsFilter) bool {
	if len(filters) == 0 {
		return true
	}

	var configs []GroupConfigurationItem
	if b.groupConfigurations[region] != nil {
		configs = b.groupConfigurations[region][g.Name]
	}

	for _, f := range filters {
		switch f.Name {
		case listGroupsFilterConfigurationType:
			if !configMatchesTypeFilter(configs, f.Values) {
				return false
			}
		case listGroupsFilterResourceType:
			if !configMatchesResourceTypeFilter(configs, f.Values) {
				return false
			}
		case listGroupsFilterOwner:
			if !slices.Contains(f.Values, g.Owner) {
				return false
			}
		case listGroupsFilterDisplayName:
			if !slices.Contains(f.Values, g.DisplayName) {
				return false
			}
		case listGroupsFilterCriticality:
			if !slices.Contains(f.Values, strconv.Itoa(g.Criticality)) {
				return false
			}
		}
	}

	return true
}

// configMatchesTypeFilter returns true if any configuration item has a Type matching one of values.
func configMatchesTypeFilter(configs []GroupConfigurationItem, values []string) bool {
	for _, item := range configs {
		if slices.Contains(values, item.Type) {
			return true
		}
	}

	return false
}

// configMatchesResourceTypeFilter returns true if any configuration item has an
// allowed-resource-types parameter containing one of values.
func configMatchesResourceTypeFilter(configs []GroupConfigurationItem, values []string) bool {
	for _, item := range configs {
		for _, param := range item.Parameters {
			if param.Name != configParamAllowedResourceTypes {
				continue
			}

			for _, pv := range param.Values {
				if slices.Contains(values, pv) {
					return true
				}
			}
		}
	}

	return false
}

// PutGroupConfiguration stores a deep copy of items for the named group.
// It validates each item's Type and Parameters against the known allow-list.
func (b *InMemoryBackend) PutGroupConfiguration(
	ctx context.Context,
	nameOrARN string,
	items []GroupConfigurationItem,
) error {
	if err := validateConfiguration(items); err != nil {
		return err
	}

	b.mu.Lock("PutGroupConfiguration")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	name := resolveGroupName(nameOrARN)

	g, ok := b.groups.Get(regionKey(region, name))
	if !ok {
		return fmt.Errorf("%w: group %s not found", ErrNotFound, name)
	}

	// api_op_PutGroupConfiguration.go:45-46: "A resource group can contain
	// either a Configuration or a ResourceQuery, but not both."
	if g.ResourceQuery != nil {
		return fmt.Errorf(
			"%w: a group cannot have both a ResourceQuery and a Configuration; "+
				"use one or the other",
			ErrValidation,
		)
	}

	b.groupConfigurationsStore(region)[name] = cloneConfigItems(items)

	return nil
}

// GetGroupConfigurationItems returns a deep copy of the stored configuration for a group.
func (b *InMemoryBackend) GetGroupConfigurationItems(
	ctx context.Context,
	nameOrARN string,
) ([]GroupConfigurationItem, error) {
	b.mu.RLock("GetGroupConfigurationItems")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	name := resolveGroupName(nameOrARN)

	if !b.groups.Has(regionKey(region, name)) {
		return nil, fmt.Errorf("%w: group %s not found", ErrNotFound, name)
	}

	var configs []GroupConfigurationItem
	if b.groupConfigurations[region] != nil {
		configs = b.groupConfigurations[region][name]
	}

	return cloneConfigItems(configs), nil
}

// cloneConfigItems returns a deep copy of a GroupConfigurationItem slice.
func cloneConfigItems(items []GroupConfigurationItem) []GroupConfigurationItem {
	if items == nil {
		return []GroupConfigurationItem{}
	}

	cp := make([]GroupConfigurationItem, len(items))

	for i, item := range items {
		cp[i] = GroupConfigurationItem{Type: item.Type}
		if len(item.Parameters) > 0 {
			cp[i].Parameters = make([]GroupConfigurationParameter, len(item.Parameters))
			for j, p := range item.Parameters {
				cp[i].Parameters[j] = GroupConfigurationParameter{Name: p.Name}
				if len(p.Values) > 0 {
					cp[i].Parameters[j].Values = make([]string, len(p.Values))
					copy(cp[i].Parameters[j].Values, p.Values)
				}
			}
		}
	}

	return cp
}
