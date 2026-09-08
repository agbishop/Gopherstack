package resourcegroups

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"
)

// Grouping action constants.
const (
	groupingActionGroup   = "GROUP"
	groupingActionUngroup = "UNGROUP"
	groupingStatusSuccess = "SUCCESS"
	groupingStatusFailed  = "FAILED"
)

// GroupingStatus error codes for failed grouping operations.
const (
	groupingErrInvalidARN       = "INVALID_ARN"
	groupingErrResourceNotFound = "RESOURCE_NOT_FOUND"
)

// listGroupResourcesFilterResourceType is the filter name for filtering ListGroupResources by resource type.
const listGroupResourcesFilterResourceType = "resource-type"

// ListGroupingStatuses filter names (types.ListGroupingStatusesFilterName).
const (
	listGroupingStatusesFilterNameStatus      = "status"
	listGroupingStatusesFilterNameResourceArn = "resource-arn"
)

// errQueryGroupNotGroupable: GroupResources/UngroupResources only work on
// static-membership groups. Real AWS membership for a ResourceQuery-based
// group is computed dynamically from the query, not by explicit add/remove
// (API docs: GroupResources "You can only use this operation with"
// AWS::EC2::HostManagement/CapacityReservationPool/ApplicationGroup groups;
// UngroupResources "doesn't work with any resource groups that are
// automatically populated by tag-based or CloudFormation stack-based
// queries").
const errQueryGroupNotGroupable = "cannot group/ungroup resources on a group with a ResourceQuery; " +
	"membership for query-based groups is computed dynamically from the query"

// GroupResources associates a list of resource ARNs with a group.
// Duplicate ARNs are silently ignored; each ARN is only added once.
func (b *InMemoryBackend) GroupResources(
	ctx context.Context,
	nameOrARN string,
	resourceARNs []string,
) ([]string, error) {
	b.mu.Lock("GroupResources")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	name := resolveGroupName(nameOrARN)

	g, ok := b.groups.Get(regionKey(region, name))
	if !ok {
		return nil, fmt.Errorf("%w: group %s not found", ErrNotFound, name)
	}

	if g.ResourceQuery != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, errQueryGroupNotGroupable)
	}

	resStore := b.groupResourcesStore(region)

	if resStore[name] == nil {
		resStore[name] = []string{}
	}

	existing := make(map[string]struct{}, len(resStore[name]))

	for _, a := range resStore[name] {
		existing[a] = struct{}{}
	}

	now := time.Now().UTC()
	succeeded := make([]string, 0, len(resourceARNs))
	statusStore := b.groupingStatusesStore(region)

	for _, a := range resourceARNs {
		if _, dup := existing[a]; !dup {
			resStore[name] = append(resStore[name], a)
			existing[a] = struct{}{}
		}

		succeeded = append(succeeded, a)
		statusStore[name] = append(statusStore[name], GroupingStatusItem{
			ResourceArn: a,
			Action:      groupingActionGroup,
			Status:      groupingStatusSuccess,
			UpdatedAt:   now,
		})
	}

	return succeeded, nil
}

// UngroupResources removes a list of resource ARNs from a group.
// ARNs that are not currently in the group are returned in Failed[].
func (b *InMemoryBackend) UngroupResources(
	ctx context.Context,
	nameOrARN string,
	resourceARNs []string,
) (*UngroupResourcesResult, error) {
	b.mu.Lock("UngroupResources")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)
	name := resolveGroupName(nameOrARN)

	g, ok := b.groups.Get(regionKey(region, name))
	if !ok {
		return nil, fmt.Errorf("%w: group %s not found", ErrNotFound, name)
	}

	if g.ResourceQuery != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, errQueryGroupNotGroupable)
	}

	resStore := b.groupResourcesStore(region)
	existing := make(map[string]struct{}, len(resStore[name]))

	for _, a := range resStore[name] {
		existing[a] = struct{}{}
	}

	remove := make(map[string]struct{}, len(resourceARNs))
	for _, a := range resourceARNs {
		remove[a] = struct{}{}
	}

	kept := resStore[name][:0:0]
	for _, a := range resStore[name] {
		if _, removed := remove[a]; !removed {
			kept = append(kept, a)
		}
	}

	resStore[name] = kept

	now := time.Now().UTC()
	result := &UngroupResourcesResult{
		Succeeded: make([]string, 0),
		Failed:    make([]GroupingFailedItem, 0),
	}

	statusStore := b.groupingStatusesStore(region)

	for _, a := range resourceARNs {
		if _, wasMember := existing[a]; wasMember {
			result.Succeeded = append(result.Succeeded, a)
			statusStore[name] = append(statusStore[name], GroupingStatusItem{
				ResourceArn: a,
				Action:      groupingActionUngroup,
				Status:      groupingStatusSuccess,
				UpdatedAt:   now,
			})
		} else {
			result.Failed = append(result.Failed, GroupingFailedItem{
				ResourceArn:  a,
				ErrorCode:    groupingErrResourceNotFound,
				ErrorMessage: fmt.Sprintf("resource %s is not a member of group %s", a, name),
			})
			statusStore[name] = append(statusStore[name], GroupingStatusItem{
				ResourceArn:  a,
				Action:       groupingActionUngroup,
				Status:       groupingStatusFailed,
				ErrorCode:    groupingErrResourceNotFound,
				ErrorMessage: fmt.Sprintf("resource %s is not a member of group %s", a, name),
				UpdatedAt:    now,
			})
		}
	}

	return result, nil
}

// ListGroupResources returns resource identifiers associated with a group, optionally
// filtered and paginated. Supported filter Name: "resource-type" (filter by AWS resource type).
// Returns identifiers, a continuation token (empty when no more results), and any error.
func (b *InMemoryBackend) ListGroupResources(
	ctx context.Context,
	nameOrARN string,
	filters []ListGroupResourcesFilter,
	nextToken string,
	maxResults int,
) ([]ResourceIdentifier, string, error) {
	b.mu.RLock("ListGroupResources")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	name := resolveGroupName(nameOrARN)

	if !b.groups.Has(regionKey(region, name)) {
		return nil, "", fmt.Errorf("%w: group %s not found", ErrNotFound, name)
	}

	var arns []string
	if b.groupResources[region] != nil {
		arns = b.groupResources[region][name]
	}

	// Build the desired resource type set from filters (if any).
	wantTypes := make(map[string]bool)
	for _, f := range filters {
		if f.Name == listGroupResourcesFilterResourceType {
			for _, v := range f.Values {
				wantTypes[v] = true
			}
		}
	}

	out := make([]ResourceIdentifier, 0, len(arns))

	for _, a := range arns {
		resType := resourceTypeFromARN(a)

		if len(wantTypes) > 0 && !wantTypes[resType] {
			continue
		}

		out = append(out, ResourceIdentifier{ResourceArn: a, ResourceType: resType})
	}

	// Stable sort by ARN for deterministic pagination.
	sort.Slice(out, func(i, j int) bool { return out[i].ResourceArn < out[j].ResourceArn })

	page, token := paginate(out, func(id ResourceIdentifier) string { return id.ResourceArn }, nextToken, maxResults)

	return page, token, nil
}

// ListGroupingStatuses returns the grouping/ungrouping status history for a group,
// filtered by filters (Name: "status" or "resource-arn", per
// types.ListGroupingStatusesFilterName) and paginated. Returns statuses, a
// continuation token (empty when no more results), and any error.
//
// A nonexistent group is not an error: ListGroupingStatusesInput's declared
// error set (deserializers.go) has no NotFoundException, unlike sibling
// ListGroupResources, which declares it for the same required Group
// identifier -- so an unknown group just yields an empty result
// (gopherstack-m4k0).
func (b *InMemoryBackend) ListGroupingStatuses(
	ctx context.Context,
	nameOrARN string,
	filters []ListGroupingStatusesFilter,
	nextToken string,
	maxResults int,
) ([]GroupingStatusItem, string, error) {
	b.mu.RLock("ListGroupingStatuses")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)
	name := resolveGroupName(nameOrARN)

	var statuses []GroupingStatusItem
	if b.groupingStatuses[region] != nil {
		statuses = b.groupingStatuses[region][name]
	}

	out := make([]GroupingStatusItem, 0, len(statuses))
	for _, s := range statuses {
		if groupingStatusMatchesFilters(s, filters) {
			out = append(out, s)
		}
	}

	page, token := paginate(out, func(s GroupingStatusItem) string {
		return s.ResourceArn + "|" + s.Action + "|" + s.UpdatedAt.Format(time.RFC3339Nano)
	}, nextToken, maxResults)

	return page, token, nil
}

// groupingStatusMatchesFilters returns true when s satisfies every provided
// filter (filters AND together across entries; a single entry's Values
// OR-match, per AWS's Name/Values filter contract).
func groupingStatusMatchesFilters(s GroupingStatusItem, filters []ListGroupingStatusesFilter) bool {
	for _, f := range filters {
		switch f.Name {
		case listGroupingStatusesFilterNameStatus:
			if !slices.Contains(f.Values, s.Status) {
				return false
			}
		case listGroupingStatusesFilterNameResourceArn:
			if !slices.Contains(f.Values, s.ResourceArn) {
				return false
			}
		}
	}

	return true
}
