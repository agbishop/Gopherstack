package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// ensureRegionGroupsSeeded seeds the built-in "default" schedule group in region the
// first time the region is touched. It replicates the pre-conversion
// scheduleGroupsStore's lazy-init side effect: since the default group can never be
// deleted (see DeleteScheduleGroup), a region having zero groups is equivalent to
// "never seeded" and vice versa. Callers must hold b.mu (or equivalent).
func (b *InMemoryBackend) ensureRegionGroupsSeeded(region string) {
	if len(b.scheduleGroupsByRegion.Get(region)) == 0 {
		b.seedDefaultGroup(region)
	}
}

// scheduleGroupRegionOf returns the region a ScheduleGroup belongs to, derived from
// its ARN (always built with the owning region -- see CreateScheduleGroup/
// seedDefaultGroup), falling back to the backend's own default region for a
// malformed/test-injected ARN. Mirrors the pre-conversion AddScheduleGroupInternal's
// regionFromARN(g.ARN, b.region) call.
func (b *InMemoryBackend) scheduleGroupRegionOf(g *ScheduleGroup) string {
	return regionFromARN(g.ARN, b.region)
}

// scheduleGroupTableKeyFn is the store.Table primary key function for scheduleGroups.
func (b *InMemoryBackend) scheduleGroupTableKeyFn(g *ScheduleGroup) string {
	return regionKey(b.scheduleGroupRegionOf(g), g.Name)
}

// scheduleGroupARNIndexKeyFn is the scheduleGroupsByARN index key function.
func (b *InMemoryBackend) scheduleGroupARNIndexKeyFn(g *ScheduleGroup) string {
	return regionKey(b.scheduleGroupRegionOf(g), g.ARN)
}

// seedDefaultGroup creates the built-in "default" schedule group in the given region.
// It is invoked from ensureRegionGroupsSeeded the first time a region is seen (and
// from Reset/Restore). Callers must hold b.mu (or be in single-threaded setup).
func (b *InMemoryBackend) seedDefaultGroup(region string) {
	now := time.Now().UTC()
	groupARN := arn.Build("scheduler", region, b.accountID, "schedule-group/"+defaultGroupName)
	g := &ScheduleGroup{
		Name:                 defaultGroupName,
		ARN:                  groupARN,
		State:                scheduleGroupStateActive,
		CreationDate:         now,
		LastModificationDate: now,
		Tags:                 tags.New("scheduler.schedulegroup." + defaultGroupName + ".tags"),
	}
	b.scheduleGroups.Put(g)
}

// CreateScheduleGroup creates a new schedule group with the given name and optional tags.
func (b *InMemoryBackend) CreateScheduleGroup(
	ctx context.Context,
	name string,
	initialTags map[string]string,
) (*ScheduleGroup, error) {
	if err := validateGroupName(name); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateScheduleGroup")
	defer b.mu.Unlock()

	b.ensureRegionGroupsSeeded(region)

	if b.scheduleGroups.Has(regionKey(region, name)) {
		return nil, fmt.Errorf("%w: schedule group %s already exists", ErrAlreadyExists, name)
	}

	groupARN := arn.Build("scheduler", region, b.accountID, "schedule-group/"+name)
	now := time.Now().UTC()
	g := &ScheduleGroup{
		Name:                 name,
		ARN:                  groupARN,
		State:                scheduleGroupStateActive,
		CreationDate:         now,
		LastModificationDate: now,
		Tags:                 tags.New("scheduler.schedulegroup." + name + ".tags"),
	}
	g.Tags.Merge(initialTags)
	b.scheduleGroups.Put(g)

	return cloneScheduleGroup(g), nil
}

// GetScheduleGroup returns the schedule group with the given name.
func (b *InMemoryBackend) GetScheduleGroup(ctx context.Context, name string) (*ScheduleGroup, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("GetScheduleGroup")
	defer b.mu.RUnlock()

	b.ensureRegionGroupsSeeded(region)

	g, ok := b.scheduleGroups.Get(regionKey(region, name))
	if !ok {
		return nil, fmt.Errorf("%w: schedule group %s not found", ErrNotFound, name)
	}

	return cloneScheduleGroup(g), nil
}

// DeleteScheduleGroup removes the schedule group with the given name.
// The built-in "default" group cannot be deleted.
// All schedules within the group are also deleted.
func (b *InMemoryBackend) DeleteScheduleGroup(ctx context.Context, name string) error {
	if name == defaultGroupName {
		return fmt.Errorf("%w: cannot delete the default schedule group", ErrValidation)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteScheduleGroup")
	defer b.mu.Unlock()

	g, ok := b.scheduleGroups.Get(regionKey(region, name))
	if !ok {
		return fmt.Errorf("%w: schedule group %s not found", ErrNotFound, name)
	}

	// Cascade-delete all schedules belonging to this group.
	for _, s := range b.schedulesByRegion.Get(region) {
		if s.GroupName == name {
			b.schedules.Delete(regionKey(region, scheduleKey(s.GroupName, s.Name)))
			if s.Tags != nil {
				s.Tags.Close()
			}
		}
	}

	b.scheduleGroups.Delete(regionKey(region, name))
	g.Tags.Close()

	return nil
}

// ListScheduleGroups returns schedule groups optionally filtered by name prefix.
// When maxResults > 0 and nextToken is non-empty it resumes after the token (last seen name).
// Returns the page of groups and the next continuation token (empty when no more results).
func (b *InMemoryBackend) ListScheduleGroups(
	ctx context.Context, namePrefix, nextToken string, maxResults int,
) ([]*ScheduleGroup, string) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListScheduleGroups")
	defer b.mu.RUnlock()

	b.ensureRegionGroupsSeeded(region)

	groups := b.scheduleGroupsByRegion.Get(region)
	list := make([]*ScheduleGroup, 0, len(groups))

	for _, g := range groups {
		if namePrefix != "" && !strings.HasPrefix(g.Name, namePrefix) {
			continue
		}

		list = append(list, cloneScheduleGroup(g))
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	return paginate(list, func(g *ScheduleGroup) string { return g.Name }, nextToken, maxResults)
}

// scheduleGroupByARN resolves a schedule group by its ARN within region via the
// byARN index, returning nil when no group matches. Callers must hold b.mu.
func (b *InMemoryBackend) scheduleGroupByARN(region, resourceARN string) *ScheduleGroup {
	if matches := b.scheduleGroupsByARN.Get(regionKey(region, resourceARN)); len(matches) > 0 {
		return matches[0]
	}

	return nil
}

// AddScheduleGroupInternal inserts a schedule group directly for testing purposes.
// Must only be used from test code.
func (b *InMemoryBackend) AddScheduleGroupInternal(g *ScheduleGroup) {
	b.mu.Lock("AddScheduleGroupInternal")
	defer b.mu.Unlock()

	if g.Tags == nil {
		g.Tags = tags.New("scheduler.schedulegroup." + g.Name + ".tags")
	}

	// Seed the built-in "default" group for the group's region first (matching the
	// pre-conversion scheduleGroupsStore's lazy-init side effect), then insert g.
	// The group's region is derived from its ARN by the table's key function.
	b.ensureRegionGroupsSeeded(b.scheduleGroupRegionOf(g))
	b.scheduleGroups.Put(g)
}

// cloneScheduleGroup returns a deep copy of a schedule group (including a snapshot of its Tags).
func cloneScheduleGroup(g *ScheduleGroup) *ScheduleGroup {
	cp := *g
	cp.Tags = nil

	if g.Tags != nil {
		cp.Tags = tags.FromMap("scheduler.schedulegroup."+g.Name+".tags.clone", g.Tags.Clone())
	}

	return &cp
}

// validateGroupName validates a schedule group name, also rejecting "default" as a creation target.
func validateGroupName(name string) error {
	if err := validateName(name); err != nil {
		return err
	}

	if name == defaultGroupName {
		return fmt.Errorf("%w: cannot create a group named %q (reserved)", ErrValidation, defaultGroupName)
	}

	return nil
}
