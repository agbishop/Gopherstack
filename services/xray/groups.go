package xray

import (
	"fmt"
	"sort"
	"time"
)

func cloneGroup(g *Group) *Group {
	cp := *g

	return &cp
}

// CreateGroup creates a new X-Ray group with the given name and filter expression.
func (b *InMemoryBackend) CreateGroup(name, filterExpr string) (*Group, error) {
	b.mu.Lock("CreateGroup")
	defer b.mu.Unlock()

	if b.groups.Has(name) {
		return nil, fmt.Errorf("%w: group %s already exists", ErrGroupAlreadyExists, name)
	}

	g := &Group{
		GroupARN:         b.groupARN(name),
		GroupName:        name,
		FilterExpression: filterExpr,
		CreatedAt:        time.Now(),
	}
	b.groups.Put(g)

	return cloneGroup(g), nil
}

// CreateGroupWithInsights creates a new group with full InsightsConfiguration.
func (b *InMemoryBackend) CreateGroupWithInsights(name, filterExpr string, ic InsightsConfiguration) (*Group, error) {
	b.mu.Lock("CreateGroupWithInsights")
	defer b.mu.Unlock()

	if b.groups.Has(name) {
		return nil, fmt.Errorf("%w: group %s already exists", ErrGroupAlreadyExists, name)
	}

	g := &Group{
		GroupARN:              b.groupARN(name),
		GroupName:             name,
		FilterExpression:      filterExpr,
		InsightsConfiguration: ic,
		CreatedAt:             time.Now(),
	}
	b.groups.Put(g)

	return cloneGroup(g), nil
}

// GetGroup returns the group with the given name, or by ARN if name is empty.
func (b *InMemoryBackend) GetGroup(name string) (*Group, error) {
	b.mu.RLock("GetGroup")
	defer b.mu.RUnlock()

	g, ok := b.groups.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: group %s not found", ErrGroupNotFound, name)
	}

	return cloneGroup(g), nil
}

// GetGroupByARN returns the group with the given ARN.
func (b *InMemoryBackend) GetGroupByARN(arn string) (*Group, error) {
	b.mu.RLock("GetGroupByARN")
	defer b.mu.RUnlock()

	if list := b.groupsByARN.Get(arn); len(list) > 0 {
		return cloneGroup(list[0]), nil
	}

	return nil, fmt.Errorf("%w: group with ARN %s not found", ErrGroupNotFound, arn)
}

// GetGroups returns all groups sorted by name.
func (b *InMemoryBackend) GetGroups() []Group {
	b.mu.RLock("GetGroups")
	defer b.mu.RUnlock()

	all := b.groups.All()
	out := make([]Group, 0, len(all))

	for _, g := range all {
		out = append(out, *cloneGroup(g))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].GroupName < out[j].GroupName
	})

	return out
}

// UpdateGroup updates the filter expression for the group with the given name.
func (b *InMemoryBackend) UpdateGroup(name, filterExpr string) (*Group, error) {
	b.mu.Lock("UpdateGroup")
	defer b.mu.Unlock()

	g, ok := b.groups.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: group %s not found", ErrGroupNotFound, name)
	}

	g.FilterExpression = filterExpr

	return cloneGroup(g), nil
}

// UpdateGroupByARN updates a group by ARN or name. filterExpr and insights are
// pointer-semantic: a nil pointer leaves the corresponding field unchanged, matching
// the real UpdateGroupInput shape where FilterExpression and InsightsConfiguration are
// both independently optional (unlike CreateGroup, omitting one on update must not
// reset the other to its zero value).
func (b *InMemoryBackend) UpdateGroupByARN(
	name, arn string,
	filterExpr *string,
	insights *InsightsConfiguration,
) (*Group, error) {
	b.mu.Lock("UpdateGroupByARN")
	defer b.mu.Unlock()

	var g *Group

	if arn != "" {
		if list := b.groupsByARN.Get(arn); len(list) > 0 {
			g = list[0]
		}
	} else {
		g, _ = b.groups.Get(name)
	}

	if g == nil {
		key := name
		if arn != "" {
			key = arn
		}

		return nil, fmt.Errorf("%w: group %s not found", ErrGroupNotFound, key)
	}

	if filterExpr != nil {
		g.FilterExpression = *filterExpr
	}

	if insights != nil {
		g.InsightsConfiguration = *insights
	}

	return cloneGroup(g), nil
}

// DeleteGroup removes the group with the given name.
func (b *InMemoryBackend) DeleteGroup(name string) error {
	b.mu.Lock("DeleteGroup")
	defer b.mu.Unlock()

	if !b.groups.Delete(name) {
		return fmt.Errorf("%w: group %s not found", ErrGroupNotFound, name)
	}

	delete(b.resourceTags, b.groupARN(name))

	return nil
}

// DeleteGroupByARN removes the group with the given ARN or name.
func (b *InMemoryBackend) DeleteGroupByARN(name, arn string) error {
	b.mu.Lock("DeleteGroupByARN")
	defer b.mu.Unlock()

	if arn != "" {
		list := b.groupsByARN.Get(arn)
		if len(list) == 0 {
			return fmt.Errorf("%w: group with ARN %s not found", ErrGroupNotFound, arn)
		}

		b.groups.Delete(list[0].GroupName)
		delete(b.resourceTags, b.groupARN(list[0].GroupName))

		return nil
	}

	if !b.groups.Delete(name) {
		return fmt.Errorf("%w: group %s not found", ErrGroupNotFound, name)
	}

	delete(b.resourceTags, b.groupARN(name))

	return nil
}
