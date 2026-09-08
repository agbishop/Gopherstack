package bedrock

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func flowAliasKey(flowID, aliasID string) string { return flowID + "/" + aliasID }

// CreateFlowAlias creates an alias for a Flow.
func (b *InMemoryBackend) CreateFlowAlias(
	flowID, name, description string,
) (*FlowAlias, error) {
	b.mu.Lock("CreateFlowAlias")
	defer b.mu.Unlock()

	if _, ok := b.flows.Get(flowID); !ok {
		return nil, fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	b.flowAliasCounter++
	aliasID := fmt.Sprintf("flowAlias-%08d", b.flowAliasCounter)
	aliasArn := arn.Build(
		"bedrock",
		b.region,
		b.accountID,
		"flow/"+flowID+"/alias/"+aliasID,
	)
	now := time.Now()

	fa := &FlowAlias{
		CreatedAt:    now,
		UpdatedAt:    now,
		FlowAliasID:  aliasID,
		FlowAliasArn: aliasArn,
		FlowID:       flowID,
		Name:         name,
		Description:  description,
	}
	b.flowAliases.Put(fa)
	cp := *fa

	return &cp, nil
}

// GetFlowAlias returns a Flow alias by ID.
func (b *InMemoryBackend) GetFlowAlias(flowID, aliasID string) (*FlowAlias, error) {
	b.mu.RLock("GetFlowAlias")
	defer b.mu.RUnlock()

	fa, ok := b.flowAliases.Get(flowAliasKey(flowID, aliasID))
	if !ok {
		return nil, fmt.Errorf("%w: flow alias %q not found", ErrNotFound, aliasID)
	}

	cp := *fa

	return &cp, nil
}

// ListFlowAliases lists aliases for a Flow.
func (b *InMemoryBackend) ListFlowAliases(
	flowID string,
	maxResults int,
	nextToken string,
) ([]*FlowAlias, string) {
	b.mu.RLock("ListFlowAliases")
	defer b.mu.RUnlock()

	list := make([]*FlowAlias, 0)

	for _, fa := range b.flowAliases.All() {
		if fa.FlowID == flowID {
			cp := *fa
			list = append(list, &cp)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Name != list[j].Name {
			return list[i].Name < list[j].Name
		}

		return list[i].FlowAliasID < list[j].FlowAliasID
	})

	return paginate(list, maxResults, nextToken)
}

// UpdateFlowAlias updates a Flow alias.
func (b *InMemoryBackend) UpdateFlowAlias(
	flowID, aliasID, name, description string,
) (*FlowAlias, error) {
	b.mu.Lock("UpdateFlowAlias")
	defer b.mu.Unlock()

	fa, ok := b.flowAliases.Get(flowAliasKey(flowID, aliasID))
	if !ok {
		return nil, fmt.Errorf("%w: flow alias %q not found", ErrNotFound, aliasID)
	}

	if name != "" {
		fa.Name = name
	}

	if description != "" {
		fa.Description = description
	}

	fa.UpdatedAt = time.Now()
	cp := *fa

	return &cp, nil
}

// DeleteFlowAlias deletes a Flow alias.
func (b *InMemoryBackend) DeleteFlowAlias(flowID, aliasID string) error {
	b.mu.Lock("DeleteFlowAlias")
	defer b.mu.Unlock()

	key := flowAliasKey(flowID, aliasID)

	fa, ok := b.flowAliases.Get(key)
	if !ok {
		return fmt.Errorf("%w: flow alias %q not found", ErrNotFound, aliasID)
	}

	delete(b.agentTags, fa.FlowAliasArn)
	b.flowAliases.Delete(key)

	return nil
}
