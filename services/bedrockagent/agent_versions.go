package bedrockagent

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"
)

// ---------------------------------------------------------------------------
// Agent version CRUD
// ---------------------------------------------------------------------------

// CreateAgentVersion creates a numbered snapshot of an agent.
//
// Note: real Bedrock Agents has no CreateAgentVersion wire operation --
// numbered agent versions are created as a side effect of CreateAgentAlias
// when called with no routingConfiguration (see the doAgentAlias helper in
// CreateAgentAlias below, which calls newAgentVersionLocked directly while
// already holding b.mu). This method stays exported on InMemoryBackend/
// StorageBackend for internal/programmatic use (e.g. tests seeding a
// version directly) but is deliberately not reachable from any HTTP route.
func (b *InMemoryBackend) CreateAgentVersion(
	_ context.Context, agentID, description string,
) (*AgentVersion, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	av, err := b.newAgentVersionLocked(agentID, description)
	if err != nil {
		return nil, err
	}

	return agentVersionCopy(av), nil
}

// newAgentVersionLocked creates a numbered snapshot of an agent. Callers
// must hold b.mu.Lock.
func (b *InMemoryBackend) newAgentVersionLocked(agentID, description string) (*AgentVersion, error) {
	a, ok := b.agents.Get(agentID)
	if !ok {
		return nil, fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	b.agentVersionCtrs[agentID]++
	versionNum := b.agentVersionCtrs[agentID]
	version := strconv.Itoa(versionNum)

	now := time.Now().UTC()
	av := &AgentVersion{
		AgentID:                 agentID,
		AgentARN:                a.AgentARN,
		AgentName:               a.AgentName,
		AgentVersion:            version,
		AgentStatus:             agentStatusPrepared,
		FoundationModel:         a.FoundationModel,
		Instruction:             a.Instruction,
		RoleARN:                 a.RoleARN,
		IdleSessionTTLInSeconds: a.IdleSessionTTLInSeconds,
		Description:             description,
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	b.agentVersions.Put(av)
	b.snapshotSubResourcesLocked(agentID, version)

	return av, nil
}

// snapshotSubResourcesLocked deep-copies an agent's DRAFT action groups,
// collaborators, and knowledge-base associations into the newly created
// numbered version. Real AWS does this at the moment a numbered version is
// created (confirmed via GetAgentActionGroup's API reference: its
// {agentVersion} path pattern `(DRAFT|[0-9]{0,4}[1-9][0-9]{0,4})` accepts
// non-DRAFT versions too, unlike Create/Associate which are DRAFT-only) --
// without this, Get/List against a numbered version always comes back empty
// even though DRAFT had content at snapshot time. Callers must hold b.mu.Lock.
func (b *InMemoryBackend) snapshotSubResourcesLocked(agentID, version string) {
	draftScope := agentVersionScope(agentID, defaultAgentVersion)

	for _, ag := range b.actionGroupsByAgentVersion.Get(draftScope) {
		cp := *ag
		cp.AgentVersion = version
		b.actionGroups.Put(&cp)
	}

	for _, c := range b.agentCollaboratorsByAgentVersion.Get(draftScope) {
		cp := *c
		cp.AgentVersion = version
		b.agentCollaborators.Put(&cp)
	}

	for _, kb := range b.agentKBAssocsByAgentVersion.Get(draftScope) {
		cp := *kb
		cp.AgentVersion = version
		b.agentKBAssocs.Put(&cp)
	}
}

// GetAgentVersion returns a specific agent version.
//
// Not-found precedence note: the pre-Phase-3.3 map checked for the presence
// of the per-agent inner map (created lazily by the first
// CreateAgentVersion call and never removed except by DeleteAgent) to decide
// between "agent not found" and "agent version not found". That inner-map
// presence check is not reproducible with a flat store.Table + secondary
// Index (an Index prunes a group the moment its last member is deleted, see
// pkgs/store's Index.remove), so this checks b.agents.Has(agentID) instead --
// identical result in every case except the one where every version of a
// still-existing agent has been deleted via DeleteAgentVersion, where the
// pre-conversion code returned "agent not found" (arguably itself a
// mislabeled error) and this returns "agent version not found".
func (b *InMemoryBackend) GetAgentVersion(
	_ context.Context, agentID, agentVersion string,
) (*AgentVersion, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.agents.Has(agentID) {
		return nil, fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	av, ok := b.agentVersions.Get(agentVersionKey(agentID, agentVersion))
	if !ok {
		return nil, fmt.Errorf("%w: agent version %q not found", ErrNotFound, agentVersion)
	}

	return agentVersionCopy(av), nil
}

// DeleteAgentVersion deletes an agent version. See the not-found precedence
// note on GetAgentVersion.
//
// Real AWS (api_op_DeleteAgentVersion.go): "By default, this value is false
// and deletion is stopped if the resource is in use. If you set it to true,
// the resource will be deleted even if the resource is in use." An agent
// version is "in use" when an alias's routingConfiguration still points at
// it (types.AgentAliasRoutingConfigurationListItem.AgentVersion) -- that
// reference is the only wire-visible relationship a version participates
// in, so it is the only one this check can honor without inventing AWS
// behavior the SDK doesn't state.
func (b *InMemoryBackend) DeleteAgentVersion(
	_ context.Context, agentID, agentVersion string, skipResourceInUseCheck bool,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.agents.Has(agentID) {
		return fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	key := agentVersionKey(agentID, agentVersion)
	if !b.agentVersions.Has(key) {
		return fmt.Errorf("%w: agent version %q not found", ErrNotFound, agentVersion)
	}

	if !skipResourceInUseCheck {
		for _, al := range b.agentAliasesByAgent.Get(agentID) {
			for _, r := range al.RoutingConfiguration {
				if r.AgentVersion == agentVersion {
					return fmt.Errorf(
						"%w: agent version %q is referenced by alias %q",
						ErrResourceInUse, agentVersion, al.AgentAliasID,
					)
				}
			}
		}
	}

	b.agentVersions.Delete(key)
	b.deleteSubResourcesLocked(agentID, agentVersion)

	return nil
}

// deleteSubResourcesLocked removes every action group, collaborator, and
// knowledge-base association scoped to the given (agentID, version) --
// the snapshot rows written by snapshotSubResourcesLocked for numbered
// versions, or the live DRAFT rows. Callers must hold b.mu.Lock.
func (b *InMemoryBackend) deleteSubResourcesLocked(agentID, version string) {
	scope := agentVersionScope(agentID, version)

	for _, ag := range slices.Clone(b.actionGroupsByAgentVersion.Get(scope)) {
		b.actionGroups.Delete(agActionGroupKey(ag.AgentID, ag.AgentVersion, ag.ActionGroupID))
	}

	for _, cb := range slices.Clone(b.agentCollaboratorsByAgentVersion.Get(scope)) {
		b.agentCollaborators.Delete(agentCollabKey(cb.AgentID, cb.AgentVersion, cb.CollaboratorID))
	}

	for _, kba := range slices.Clone(b.agentKBAssocsByAgentVersion.Get(scope)) {
		b.agentKBAssocs.Delete(agKBKey(kba.AgentID, kba.AgentVersion, kba.KnowledgeBaseID))
	}
}

// ListAgentVersions returns paginated agent version summaries.
func (b *InMemoryBackend) ListAgentVersions(
	_ context.Context, agentID string, maxResults int, nextToken string,
) ([]*AgentVersionSummary, string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.agents.Has(agentID) {
		return nil, "", fmt.Errorf("%w: agent %q not found", ErrNotFound, agentID)
	}

	group := b.agentVersionsByAgent.Get(agentID)
	keys := tableIDs(group, func(av *AgentVersion) string { return av.AgentVersion })
	keys, outToken := paginate(keys, nextToken, maxResults)

	out := make([]*AgentVersionSummary, 0, len(keys))

	for _, k := range keys {
		av, _ := b.agentVersions.Get(agentVersionKey(agentID, k))
		out = append(out, &AgentVersionSummary{
			AgentName:    av.AgentName,
			AgentVersion: av.AgentVersion,
			AgentStatus:  av.AgentStatus,
			Description:  av.Description,
			CreatedAt:    av.CreatedAt,
			UpdatedAt:    av.UpdatedAt,
		})
	}

	return out, outToken, nil
}

func agentVersionCopy(av *AgentVersion) *AgentVersion {
	cp := *av

	return &cp
}
