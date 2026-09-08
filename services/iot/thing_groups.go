package iot

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// AddThingToThingGroup adds a thing to a thing group.
func (b *InMemoryBackend) AddThingToThingGroup(input *AddThingToThingGroupInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := thingKey(input.ThingName, input.ThingArn)
	b.thingThingGroups[key] = append(b.thingThingGroups[key], input.ThingGroupName)

	// Also update the group membership index.
	groupName := input.ThingGroupName
	if groupName == "" {
		groupName = input.ThingGroupArn
	}

	thingName := thingKey(input.ThingName, input.ThingArn)
	b.addThingToGroupByName(thingName, groupName)

	return nil
}

// CreateThingGroup creates a new IoT Thing Group.
func (b *InMemoryBackend) CreateThingGroup(input *CreateThingGroupInput) (*ThingGroup, error) {
	if input.ThingGroupName == "" {
		return nil, fmt.Errorf("%w: ThingGroupName is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.thingGroups.Has(input.ThingGroupName) {
		return nil, fmt.Errorf("%w: thing group %q already exists", ErrAlreadyExists, input.ThingGroupName)
	}

	arn := arn.Build("iot", b.region, b.accountID, fmt.Sprintf("thinggroup/%s", input.ThingGroupName))
	id := uuid.NewString()

	attrs := make(map[string]string)
	if input.Attributes != nil {
		maps.Copy(attrs, input.Attributes)
	}

	tg := &ThingGroup{
		ThingGroupName:  input.ThingGroupName,
		ThingGroupARN:   arn,
		ThingGroupID:    id,
		Description:     input.Description,
		ParentGroupName: input.ParentGroupName,
		Attributes:      attrs,
		Members:         []string{},
		Version:         1,
		CreatedAt:       time.Now(),
	}

	b.thingGroups.Put(tg)
	b.thingGroupMembers[input.ThingGroupName] = []string{}
	b.putResourceTagsLocked(tg.ThingGroupARN, input.Tags)

	return tg, nil
}

// DescribeThingGroup returns a ThingGroup by name.
func (b *InMemoryBackend) DescribeThingGroup(thingGroupName string) (*ThingGroup, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	tg, ok := b.thingGroups.Get(thingGroupName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrThingGroupNotFound, thingGroupName)
	}

	cp := *tg
	cp.Attributes = make(map[string]string, len(tg.Attributes))
	maps.Copy(cp.Attributes, tg.Attributes)
	cp.Members = make([]string, len(tg.Members))
	copy(cp.Members, tg.Members)

	return &cp, nil
}

// ListThingGroups returns all thing groups sorted by name.
func (b *InMemoryBackend) ListThingGroups() []*ThingGroup {
	b.mu.RLock()
	defer b.mu.RUnlock()

	items := b.thingGroups.Snapshot()
	out := make([]*ThingGroup, 0, len(items))

	for _, v := range items {
		tg := v
		cp := *tg
		cp.Attributes = make(map[string]string, len(tg.Attributes))
		maps.Copy(cp.Attributes, tg.Attributes)
		cp.Members = make([]string, len(tg.Members))
		copy(cp.Members, tg.Members)
		out = append(out, &cp)
	}

	return out
}

// UpdateThingGroup updates an existing thing group and returns the new version.
func (b *InMemoryBackend) UpdateThingGroup(input *UpdateThingGroupInput) (int64, error) {
	if input.ThingGroupName == "" {
		return 0, fmt.Errorf("%w: ThingGroupName is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	tg, ok := b.thingGroups.Get(input.ThingGroupName)
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrThingGroupNotFound, input.ThingGroupName)
	}

	if input.ExpectedVersion != 0 && input.ExpectedVersion != tg.Version {
		return 0, fmt.Errorf("%w: expected version %d but current is %d",
			ErrVersionConflict, input.ExpectedVersion, tg.Version)
	}

	if input.Description != "" {
		tg.Description = input.Description
	}

	if input.Attributes != nil {
		tg.Attributes = applyAttributePayload(tg.Attributes, &AttributePayload{
			Attributes: input.Attributes,
			Merge:      input.Merge,
		})
	}

	tg.Version++

	return tg.Version, nil
}

// DeleteThingGroup deletes a thing group by name.
func (b *InMemoryBackend) DeleteThingGroup(thingGroupName string, expectedVersion int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	tg, ok := b.thingGroups.Get(thingGroupName)
	if !ok {
		return fmt.Errorf("%w: %s", ErrThingGroupNotFound, thingGroupName)
	}

	if expectedVersion != 0 && expectedVersion != tg.Version {
		return fmt.Errorf("%w: expected version %d, current version %d",
			ErrVersionConflict, expectedVersion, tg.Version)
	}

	for _, member := range b.thingGroupMembers[thingGroupName] {
		b.removeThingFromGroupIndexes(member, thingGroupName)
	}

	b.thingGroups.Delete(thingGroupName)
	delete(b.thingGroupMembers, thingGroupName)
	delete(b.resourceTags, tg.ThingGroupARN)

	return nil
}

// RemoveThingFromThingGroup removes a thing from a thing group.
func (b *InMemoryBackend) RemoveThingFromThingGroup(input *RemoveThingFromThingGroupInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	groupName := input.ThingGroupName
	if groupName == "" {
		groupName = input.ThingGroupArn
	}

	thingName := input.ThingName
	if thingName == "" {
		thingName = input.ThingArn
	}

	b.removeThingFromGroupIndexes(thingName, groupName)

	return nil
}

// ListThingsInThingGroup returns all things in a given thing group.
func (b *InMemoryBackend) ListThingsInThingGroup(input *ListThingsInThingGroupInput) ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.thingGroups.Has(input.ThingGroupName) {
		return nil, fmt.Errorf("%w: %s", ErrThingGroupNotFound, input.ThingGroupName)
	}

	members := b.thingGroupMembers[input.ThingGroupName]
	out := make([]string, len(members))
	copy(out, members)

	return out, nil
}

// addThingToGroupByName adds thingName to groupName in thingGroupMembers (dedup).
// Must be called with b.mu held.
func (b *InMemoryBackend) addThingToGroupByName(thingName, groupName string) {
	members := b.thingGroupMembers[groupName]

	if slices.Contains(members, thingName) {
		return
	}

	b.thingGroupMembers[groupName] = append(members, thingName)
}

func (b *InMemoryBackend) ListThingGroupsForThing(thingName string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []string
	for groupName, members := range b.thingGroupMembers {
		if slices.Contains(members, thingName) {
			out = append(out, groupName)
		}
	}
	sort.Strings(out)

	return out
}

// CreateDynamicThingGroup creates a dynamic thing group with a query string.
func (b *InMemoryBackend) CreateDynamicThingGroup(input *CreateThingGroupInput) (*ThingGroup, error) {
	if input.ThingGroupName == "" {
		return nil, fmt.Errorf("%w: ThingGroupName is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.thingGroups.Has(input.ThingGroupName) {
		return nil, fmt.Errorf("%w: dynamic thing group %q already exists", ErrAlreadyExists, input.ThingGroupName)
	}
	arn := arn.Build("iot", b.region, b.accountID, fmt.Sprintf("thinggroup/%s", input.ThingGroupName))
	id := uuid.NewString()
	attrs := make(map[string]string)
	if input.Attributes != nil {
		maps.Copy(attrs, input.Attributes)
	}
	tg := &ThingGroup{
		ThingGroupName:  input.ThingGroupName,
		ThingGroupARN:   arn,
		ThingGroupID:    id,
		Description:     input.Description,
		ParentGroupName: input.ParentGroupName,
		Attributes:      attrs,
		Members:         []string{},
		Version:         1,
		CreatedAt:       time.Now(),
		QueryString:     input.QueryString,
		IndexName:       input.IndexName,
		QueryVersion:    input.QueryVersion,
		IsDynamic:       true,
	}
	b.thingGroups.Put(tg)
	b.thingGroupMembers[input.ThingGroupName] = []string{}
	b.putResourceTagsLocked(tg.ThingGroupARN, input.Tags)

	return tg, nil
}

// DeleteDynamicThingGroup deletes a dynamic thing group.
func (b *InMemoryBackend) DeleteDynamicThingGroup(name string, expectedVersion int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	tg, ok := b.thingGroups.Get(name)
	if !ok {
		return fmt.Errorf("%w: %s", ErrThingGroupNotFound, name)
	}

	if expectedVersion != 0 && expectedVersion != tg.Version {
		return fmt.Errorf("%w: expected version %d, current version %d",
			ErrVersionConflict, expectedVersion, tg.Version)
	}

	for _, member := range b.thingGroupMembers[name] {
		b.removeThingFromGroupIndexes(member, name)
	}

	b.thingGroups.Delete(name)
	delete(b.thingGroupMembers, name)
	delete(b.resourceTags, tg.ThingGroupARN)

	return nil
}

// UpdateDynamicThingGroup updates a dynamic thing group and returns the new version.
func (b *InMemoryBackend) UpdateDynamicThingGroup(input *UpdateThingGroupInput) (int64, error) {
	if input.ThingGroupName == "" {
		return 0, fmt.Errorf("%w: ThingGroupName is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	tg, ok := b.thingGroups.Get(input.ThingGroupName)
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrThingGroupNotFound, input.ThingGroupName)
	}

	if input.ExpectedVersion != 0 && input.ExpectedVersion != tg.Version {
		return 0, fmt.Errorf("%w: expected version %d but current is %d",
			ErrVersionConflict, input.ExpectedVersion, tg.Version)
	}

	if input.Description != "" {
		tg.Description = input.Description
	}
	if input.QueryString != "" {
		tg.QueryString = input.QueryString
	}
	if input.IndexName != "" {
		tg.IndexName = input.IndexName
	}
	if input.QueryVersion != "" {
		tg.QueryVersion = input.QueryVersion
	}
	tg.Version++

	return tg.Version, nil
}

// UpdateThingGroupsForThing adds and/or removes a thing from the named thing
// groups, keeping the thing->groups and group->things indexes consistent.
func (b *InMemoryBackend) UpdateThingGroupsForThing(input *UpdateThingGroupsForThingInput) error {
	if input.ThingName == "" {
		return fmt.Errorf("%w: ThingName is required", ErrValidation)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.things.Has(input.ThingName) {
		return fmt.Errorf("%w: %s", ErrThingNotFound, input.ThingName)
	}

	for _, g := range input.ThingGroupsToAdd {
		if !b.thingGroups.Has(g) {
			return fmt.Errorf("%w: %s", ErrThingGroupNotFound, g)
		}
	}

	for _, g := range input.ThingGroupsToRemove {
		if !b.thingGroups.Has(g) {
			return fmt.Errorf("%w: %s", ErrThingGroupNotFound, g)
		}
	}

	for _, g := range input.ThingGroupsToAdd {
		b.addThingToGroupByName(input.ThingName, g)

		if !slices.Contains(b.thingThingGroups[input.ThingName], g) {
			b.thingThingGroups[input.ThingName] = append(b.thingThingGroups[input.ThingName], g)
		}
	}

	for _, g := range input.ThingGroupsToRemove {
		b.removeThingFromGroupIndexes(input.ThingName, g)
	}

	return nil
}

// removeThingFromGroupIndexes removes thingName from groupName's member list
// and removes groupName from thingName's group list. Must be called with
// b.mu held.
func (b *InMemoryBackend) removeThingFromGroupIndexes(thingName, groupName string) {
	members := b.thingGroupMembers[groupName]
	filteredMembers := make([]string, 0, len(members))

	for _, m := range members {
		if m != thingName {
			filteredMembers = append(filteredMembers, m)
		}
	}

	b.thingGroupMembers[groupName] = filteredMembers

	groups := b.thingThingGroups[thingName]
	filteredGroups := make([]string, 0, len(groups))

	for _, g := range groups {
		if g != groupName {
			filteredGroups = append(filteredGroups, g)
		}
	}

	b.thingThingGroups[thingName] = filteredGroups
}
