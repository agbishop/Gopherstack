package bedrock

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	aliasStatusPrepared = "PREPARED"
)

func agentAliasKey(agentID, aliasID string) string { return agentID + "/" + aliasID }

// CreateAgentAlias creates an alias for an agent.
func (b *InMemoryBackend) CreateAgentAlias(
	agentID, aliasName, agentVersion string,
) (*AgentAlias, error) {
	b.mu.Lock("CreateAgentAlias")
	defer b.mu.Unlock()

	if _, ok := b.agents.Get(agentID); !ok {
		return nil, fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	b.agentAliasCounter++
	aliasID := fmt.Sprintf("alias-%08d", b.agentAliasCounter)
	aliasArn := arn.Build("bedrock", b.region, b.accountID, "agent-alias/"+agentID+"/"+aliasID)
	now := time.Now()

	alias := &AgentAlias{
		CreatedAt:            now,
		UpdatedAt:            now,
		AgentAliasID:         aliasID,
		AgentAliasArn:        aliasArn,
		AgentAliasName:       aliasName,
		AgentID:              agentID,
		RoutingConfiguration: []AgentAliasRouting{{AgentVersion: agentVersion}},
		AliasStatus:          aliasStatusPrepared,
		AgentAliasHistoryEvents: []AgentAliasHistoryEvent{{
			StartDate:            now,
			RoutingConfiguration: []AgentAliasRouting{{AgentVersion: agentVersion}},
		}},
	}
	b.agentAliases.Put(alias)
	cp := *alias

	return &cp, nil
}

// GetAgentAlias returns an alias by ID.
func (b *InMemoryBackend) GetAgentAlias(agentID, aliasID string) (*AgentAlias, error) {
	b.mu.RLock("GetAgentAlias")
	defer b.mu.RUnlock()

	alias, ok := b.agentAliases.Get(agentAliasKey(agentID, aliasID))
	if !ok {
		return nil, fmt.Errorf("%w: agent alias %q not found", ErrNotFound, aliasID)
	}

	cp := *alias

	return &cp, nil
}

// ListAgentAliases lists aliases for an agent.
func (b *InMemoryBackend) ListAgentAliases(
	agentID string,
	maxResults int,
	nextToken string,
) ([]*AgentAlias, string) {
	b.mu.RLock("ListAgentAliases")
	defer b.mu.RUnlock()

	list := make([]*AgentAlias, 0, b.agentAliases.Len())

	for _, alias := range b.agentAliases.All() {
		if alias.AgentID == agentID {
			cp := *alias
			list = append(list, &cp)
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].AgentAliasName != list[j].AgentAliasName {
			return list[i].AgentAliasName < list[j].AgentAliasName
		}

		return list[i].AgentAliasID < list[j].AgentAliasID
	})

	return paginate(list, maxResults, nextToken)
}

// UpdateAgentAlias updates an agent alias.
func (b *InMemoryBackend) UpdateAgentAlias(
	agentID, aliasID, aliasName, agentVersion string,
) (*AgentAlias, error) {
	b.mu.Lock("UpdateAgentAlias")
	defer b.mu.Unlock()

	key := agentAliasKey(agentID, aliasID)

	alias, ok := b.agentAliases.Get(key)
	if !ok {
		return nil, fmt.Errorf("%w: agent alias %q not found", ErrNotFound, aliasID)
	}

	if aliasName != "" {
		alias.AgentAliasName = aliasName
	}

	if agentVersion != "" {
		now := time.Now()
		if len(alias.AgentAliasHistoryEvents) > 0 {
			last := &alias.AgentAliasHistoryEvents[len(alias.AgentAliasHistoryEvents)-1]
			last.EndDate = &now
		}
		alias.RoutingConfiguration = []AgentAliasRouting{{AgentVersion: agentVersion}}
		alias.AgentAliasHistoryEvents = append(alias.AgentAliasHistoryEvents, AgentAliasHistoryEvent{
			StartDate:            now,
			RoutingConfiguration: []AgentAliasRouting{{AgentVersion: agentVersion}},
		})
	}

	alias.UpdatedAt = time.Now()
	cp := *alias

	return &cp, nil
}

// DeleteAgentAlias deletes an agent alias.
func (b *InMemoryBackend) DeleteAgentAlias(agentID, aliasID string) error {
	b.mu.Lock("DeleteAgentAlias")
	defer b.mu.Unlock()

	key := agentAliasKey(agentID, aliasID)

	alias, ok := b.agentAliases.Get(key)
	if !ok {
		return fmt.Errorf("%w: agent alias %q not found", ErrNotFound, aliasID)
	}

	delete(b.agentTags, alias.AgentAliasArn)
	b.agentAliases.Delete(key)

	return nil
}
