package bedrock

import (
	"fmt"
	"maps"
	"sort"
	"strconv"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	agentTransitionDelay = 100 * time.Millisecond // delay for agent PREPARING → terminal state transition
)

const (
	agentStatusDraft       = "DRAFT"
	agentStatusPrepared    = "PREPARED"
	agentStatusPreparing   = "PREPARING"
	agentStatusNotPrepared = "NOT_PREPARED"
	agentStatusFailed      = "FAILED"
	actionGroupEnabled     = "ENABLED"
)

// CreateAgent creates a new Bedrock Agent.
func (b *InMemoryBackend) CreateAgent(
	agentName, foundationModel, instruction, roleArn string,
	tags map[string]string,
) (*Agent, error) {
	return b.CreateAgentWithConfiguration(AgentConfiguration{
		AgentName:       agentName,
		FoundationModel: foundationModel,
		Instruction:     instruction,
		RoleArn:         roleArn,
		Tags:            tags,
	})
}

// CreateAgentWithConfiguration creates an agent retaining extended AWS configuration.
func (b *InMemoryBackend) CreateAgentWithConfiguration(config AgentConfiguration) (*Agent, error) {
	b.mu.Lock("CreateAgent")
	defer b.mu.Unlock()

	if _, ok := b.agentsByName[config.AgentName]; ok {
		return nil, fmt.Errorf("%w: agent %q already exists", ErrAlreadyExists, config.AgentName)
	}

	b.agentCounter++
	id := fmt.Sprintf("agent-%08d", b.agentCounter)
	agentArn := arn.Build("bedrock", b.region, b.accountID, "agent/"+id)
	now := time.Now()

	tagsCopy := make(map[string]string, len(config.Tags))
	maps.Copy(tagsCopy, config.Tags)

	ag := &Agent{
		CreatedAt:              now,
		UpdatedAt:              now,
		AgentID:                id,
		AgentArn:               agentArn,
		AgentName:              config.AgentName,
		AgentStatus:            agentStatusNotPrepared,
		AgentVersion:           agentStatusDraft,
		AgentCollaboration:     config.AgentCollaboration,
		Description:            config.Description,
		FoundationModel:        config.FoundationModel,
		Instruction:            config.Instruction,
		RoleArn:                config.RoleArn,
		Tags:                   tagsCopy,
		GuardrailConfiguration: maps.Clone(config.GuardrailConfiguration),
		MemoryConfiguration:    maps.Clone(config.MemoryConfiguration),
	}
	b.agents.Put(ag)
	b.agentsByName[config.AgentName] = id

	cp := *ag

	return &cp, nil
}

// GetAgent returns a Bedrock Agent by ID.
func (b *InMemoryBackend) GetAgent(agentID string) (*Agent, error) {
	b.mu.Lock("GetAgent")
	defer b.mu.Unlock()

	ag, ok := b.agents.Get(agentID)
	if !ok {
		return nil, fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	b.advanceAgentStatus(ag)
	cp := *ag

	return &cp, nil
}

// ListAgents returns all agents with pagination.
func (b *InMemoryBackend) ListAgents(maxResults int, nextToken string) ([]*Agent, string) {
	b.mu.Lock("ListAgents")
	defer b.mu.Unlock()

	list := make([]*Agent, 0, b.agents.Len())
	for _, ag := range b.agents.All() {
		b.advanceAgentStatus(ag)
		cp := *ag
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].AgentName < list[j].AgentName })

	return paginate(list, maxResults, nextToken)
}

// UpdateAgent updates a Bedrock Agent.
func (b *InMemoryBackend) UpdateAgent(
	agentID, foundationModel, instruction, roleArn string,
) (*Agent, error) {
	return b.UpdateAgentWithConfiguration(agentID, AgentConfiguration{
		FoundationModel: foundationModel,
		Instruction:     instruction,
		RoleArn:         roleArn,
	})
}

// UpdateAgentWithConfiguration updates extended AWS agent configuration.
func (b *InMemoryBackend) UpdateAgentWithConfiguration(agentID string, config AgentConfiguration) (*Agent, error) {
	b.mu.Lock("UpdateAgent")
	defer b.mu.Unlock()

	ag, ok := b.agents.Get(agentID)
	if !ok {
		return nil, fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	if config.AgentName != "" && config.AgentName != ag.AgentName {
		delete(b.agentsByName, ag.AgentName)
		ag.AgentName = config.AgentName
		b.agentsByName[config.AgentName] = agentID
	}
	if config.FoundationModel != "" {
		ag.FoundationModel = config.FoundationModel
	}
	if config.Instruction != "" {
		ag.Instruction = config.Instruction
	}
	if config.RoleArn != "" {
		ag.RoleArn = config.RoleArn
	}
	if config.Description != "" {
		ag.Description = config.Description
	}
	if config.AgentCollaboration != "" {
		ag.AgentCollaboration = config.AgentCollaboration
	}
	if config.GuardrailConfiguration != nil {
		ag.GuardrailConfiguration = maps.Clone(config.GuardrailConfiguration)
	}
	if config.MemoryConfiguration != nil {
		ag.MemoryConfiguration = maps.Clone(config.MemoryConfiguration)
	}

	ag.AgentStatus = agentStatusNotPrepared
	ag.preparationDueAt = time.Time{}
	ag.FailureReasons = nil
	ag.UpdatedAt = time.Now()
	cp := *ag

	return &cp, nil
}

// DeleteAgent deletes a Bedrock Agent.
// AWS rejects deletion when the agent has active aliases (ConflictException).
func (b *InMemoryBackend) DeleteAgent(agentID string, skipResourceInUseCheck bool) error {
	b.mu.Lock("DeleteAgent")
	defer b.mu.Unlock()

	ag, ok := b.agents.Get(agentID)
	if !ok {
		return fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	hasAlias := false

	b.agentAliases.Range(func(alias *AgentAlias) bool {
		if alias.AgentID == agentID {
			hasAlias = true

			return false
		}

		return true
	})

	if hasAlias && !skipResourceInUseCheck {
		return fmt.Errorf("%w: agent %q has active aliases and cannot be deleted", ErrAlreadyExists, agentID)
	}

	delete(b.agentsByName, ag.AgentName)
	delete(b.agentTags, ag.AgentArn)
	delete(b.agentVersionCounters, agentID)

	// SkipResourceInUseCheck bypassed the guard above; the aliases still
	// exist and must not outlive their agent as ghost rows, so cascade them
	// the same way DeleteFlow cascades flowAliases.
	b.agentAliases.Range(func(alias *AgentAlias) bool {
		if alias.AgentID == agentID {
			delete(b.agentTags, alias.AgentAliasArn)
			b.agentAliases.Delete(agentAliasKey(alias.AgentID, alias.AgentAliasID))
		}

		return true
	})

	// Reset, not delete: agentVersionsStore/agentCollaboratorsStore register
	// the table under "agentVersions:"+agentID / "agentCollaborators:"+agentID
	// in b.registry once; deleting the map entry here would make a later
	// call to either accessor re-Register the same name and panic. Reset
	// clears the rows in place so GetAgentVersion/ListAgentVersions and their
	// collaborator equivalents stop returning ghost rows for this agent.
	if versions, versionsOK := b.agentVersions[agentID]; versionsOK {
		versions.Reset()
	}

	if collaborators, collabOK := b.agentCollaborators[agentID]; collabOK {
		collaborators.Reset()
	}

	b.agents.Delete(agentID)

	return nil
}

// PrepareAgent starts preparation; subsequent reads advance the terminal state.
func (b *InMemoryBackend) PrepareAgent(agentID string) (*Agent, error) {
	b.mu.Lock("PrepareAgent")
	defer b.mu.Unlock()

	ag, ok := b.agents.Get(agentID)
	if !ok {
		return nil, fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	ag.AgentStatus = agentStatusPreparing
	ag.UpdatedAt = time.Now()
	ag.preparationDueAt = ag.UpdatedAt.Add(agentTransitionDelay)
	ag.FailureReasons = nil
	cp := *ag

	return &cp, nil
}

func (b *InMemoryBackend) advanceAgentStatus(ag *Agent) {
	if ag.AgentStatus != agentStatusPreparing || time.Now().Before(ag.preparationDueAt) {
		return
	}
	if ag.FoundationModel == "" {
		ag.AgentStatus = agentStatusFailed
		ag.FailureReasons = []string{"foundationModel is required to prepare an agent"}
	} else {
		ag.AgentStatus = agentStatusPrepared
	}
	ag.UpdatedAt = time.Now()
}

// CreateAgentVersion creates a numbered version snapshot of an Agent.
func (b *InMemoryBackend) CreateAgentVersion(agentID string) (*AgentVersion, error) {
	b.mu.Lock("CreateAgentVersion")
	defer b.mu.Unlock()

	ag, ok := b.agents.Get(agentID)
	if !ok {
		return nil, fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	b.agentVersionCounters[agentID]++
	ver := strconv.Itoa(b.agentVersionCounters[agentID])

	av := &AgentVersion{
		CreatedAt:    time.Now(),
		AgentID:      agentID,
		AgentVersion: ver,
		AgentStatus:  ag.AgentStatus,
	}

	b.agentVersionsStore(agentID).Put(av)
	cp := *av

	return &cp, nil
}

// GetAgentVersion returns a specific Agent version.
func (b *InMemoryBackend) GetAgentVersion(agentID, version string) (*AgentVersion, error) {
	b.mu.RLock("GetAgentVersion")
	defer b.mu.RUnlock()

	versions, versionsOK := b.agentVersions[agentID]
	if !versionsOK {
		return nil, fmt.Errorf("%w: agent %q has no versions", ErrNotFound, agentID)
	}

	av, verOK := versions.Get(version)
	if !verOK {
		return nil, fmt.Errorf(
			"%w: agent version %q not found for agent %q",
			ErrNotFound,
			version,
			agentID,
		)
	}

	cp := *av

	return &cp, nil
}

// ListAgentVersions lists all versions for an Agent.
func (b *InMemoryBackend) ListAgentVersions(
	agentID string,
	maxResults int,
	nextToken string,
) ([]*AgentVersion, string) {
	b.mu.RLock("ListAgentVersions")
	defer b.mu.RUnlock()

	versions := b.agentVersions[agentID]
	list := make([]*AgentVersion, 0)

	if versions != nil {
		for _, av := range versions.All() {
			cp := *av
			list = append(list, &cp)
		}
	}

	sort.Slice(list, func(i, j int) bool { return list[i].AgentVersion < list[j].AgentVersion })

	return paginate(list, maxResults, nextToken)
}

// DeleteAgentVersion deletes a specific Agent version.
func (b *InMemoryBackend) DeleteAgentVersion(agentID, version string) error {
	b.mu.Lock("DeleteAgentVersion")
	defer b.mu.Unlock()

	versions, versionsOK := b.agentVersions[agentID]
	if !versionsOK {
		return fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	if !versions.Has(version) {
		return fmt.Errorf(
			"%w: agent version %q not found for agent %q",
			ErrNotFound,
			version,
			agentID,
		)
	}

	versions.Delete(version)

	return nil
}

// TagAgentResource adds tags to an agent-domain resource identified by its ARN.
func (b *InMemoryBackend) TagAgentResource(resourceArn string, tags map[string]string) error {
	b.mu.Lock("TagAgentResource")
	defer b.mu.Unlock()

	if b.agentTags[resourceArn] == nil {
		b.agentTags[resourceArn] = make(map[string]string)
	}

	maps.Copy(b.agentTags[resourceArn], tags)

	return nil
}

// UntagAgentResource removes tags from an agent-domain resource identified by its ARN.
func (b *InMemoryBackend) UntagAgentResource(resourceArn string, tagKeys []string) error {
	b.mu.Lock("UntagAgentResource")
	defer b.mu.Unlock()

	existing := b.agentTags[resourceArn]
	if existing == nil {
		return nil
	}

	for _, k := range tagKeys {
		delete(existing, k)
	}

	return nil
}

// ListAgentResourceTags returns tags for an agent-domain resource identified by its ARN.
func (b *InMemoryBackend) ListAgentResourceTags(resourceArn string) map[string]string {
	b.mu.RLock("ListAgentResourceTags")
	defer b.mu.RUnlock()

	existing := b.agentTags[resourceArn]
	result := make(map[string]string, len(existing))
	maps.Copy(result, existing)

	return result
}

// GetAgentMemory returns memory entries for an agent session (stub).
func (b *InMemoryBackend) GetAgentMemory(agentID, sessionID string) []any {
	b.mu.RLock("GetAgentMemory")
	defer b.mu.RUnlock()

	key := agentID + "/" + sessionID
	entries := b.agentMemory[key]

	result := make([]any, len(entries))
	copy(result, entries)

	return result
}

// DeleteAgentMemory deletes memory entries for an agent session.
func (b *InMemoryBackend) DeleteAgentMemory(agentID, sessionID string) error {
	b.mu.Lock("DeleteAgentMemory")
	defer b.mu.Unlock()

	if _, ok := b.agents.Get(agentID); !ok {
		return fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	delete(b.agentMemory, agentID+"/"+sessionID)

	return nil
}
