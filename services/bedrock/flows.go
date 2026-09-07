package bedrock

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// FlowStatus is not SCREAMING_SNAKE_CASE on the wire like most bedrock-agent
// status enums: aws-sdk-go-v2/service/bedrockagent/types.FlowStatus values
// are "Prepared"/"Preparing"/"NotPrepared"/"Failed" (see
// services/bedrockagent/models.go, which already got this right).
const (
	flowStatusNotPrepared = "NotPrepared"
	flowStatusPrepared    = "Prepared"
)

// CreateFlow creates a new Bedrock Flow.
func (b *InMemoryBackend) CreateFlow(
	name, description string,
	tags map[string]string,
) (*Flow, error) {
	b.mu.Lock("CreateFlow")
	defer b.mu.Unlock()

	if _, ok := b.flowsByName[name]; ok {
		return nil, fmt.Errorf("%w: flow %q already exists", ErrAlreadyExists, name)
	}

	b.flowCounter++
	id := fmt.Sprintf("flow-%08d", b.flowCounter)
	flowArn := arn.Build("bedrock", b.region, b.accountID, "flow/"+id)
	now := time.Now()

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	f := &Flow{
		CreatedAt:   now,
		UpdatedAt:   now,
		FlowID:      id,
		FlowArn:     flowArn,
		Name:        name,
		Description: description,
		Status:      flowStatusNotPrepared,
		Tags:        tagsCopy,
	}
	b.flows.Put(f)
	b.flowsByName[name] = id
	cp := *f

	return &cp, nil
}

// GetFlow returns a Flow by ID.
func (b *InMemoryBackend) GetFlow(flowID string) (*Flow, error) {
	b.mu.RLock("GetFlow")
	defer b.mu.RUnlock()

	f, ok := b.flows.Get(flowID)
	if !ok {
		return nil, fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	cp := *f

	return &cp, nil
}

// ListFlows returns all flows with pagination.
func (b *InMemoryBackend) ListFlows(maxResults int, nextToken string) ([]*Flow, string) {
	b.mu.RLock("ListFlows")
	defer b.mu.RUnlock()

	list := make([]*Flow, 0, b.flows.Len())
	for _, f := range b.flows.All() {
		cp := *f
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	return paginate(list, maxResults, nextToken)
}

// UpdateFlow updates a Flow.
func (b *InMemoryBackend) UpdateFlow(flowID, name, description string) (*Flow, error) {
	b.mu.Lock("UpdateFlow")
	defer b.mu.Unlock()

	f, ok := b.flows.Get(flowID)
	if !ok {
		return nil, fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	if name != "" && name != f.Name {
		delete(b.flowsByName, f.Name)
		f.Name = name
		b.flowsByName[name] = flowID
	}

	if description != "" {
		f.Description = description
	}

	f.UpdatedAt = time.Now()
	cp := *f

	return &cp, nil
}

// DeleteFlow deletes a Flow, its versions, and its aliases.
// Without this, GetFlowVersion/ListFlowVersions and GetFlowAlias/
// ListFlowAliases keep returning rows for a flow ID that no longer resolves
// (gopherstack-jkiu, same shape as gopherstack-wg7i's DeleteKnowledgeBase fix).
func (b *InMemoryBackend) DeleteFlow(flowID string) error {
	b.mu.Lock("DeleteFlow")
	defer b.mu.Unlock()

	f, ok := b.flows.Get(flowID)
	if !ok {
		return fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	delete(b.flowsByName, f.Name)
	delete(b.agentTags, f.FlowArn)
	delete(b.flowVersionCounters, flowID)

	// Reset, not delete: flowVersionsStore registers the table under
	// "flowVersions:"+flowID in b.registry once; deleting the map entry here
	// would make a later accessor re-Register the same name and panic. See
	// DeleteAgent's comment in agents.go for the full rationale.
	if versions, versionsOK := b.flowVersions[flowID]; versionsOK {
		versions.Reset()
	}

	b.flowAliases.Range(func(fa *FlowAlias) bool {
		if fa.FlowID == flowID {
			b.flowAliases.Delete(flowAliasKey(fa.FlowID, fa.FlowAliasID))
		}

		return true
	})

	b.flows.Delete(flowID)

	return nil
}

// PrepareFlow transitions a Flow to PREPARED status.
func (b *InMemoryBackend) PrepareFlow(flowID string) (*Flow, error) {
	b.mu.Lock("PrepareFlow")
	defer b.mu.Unlock()

	f, ok := b.flows.Get(flowID)
	if !ok {
		return nil, fmt.Errorf("%w: flow %q not found", ErrNotFound, flowID)
	}

	f.Status = flowStatusPrepared
	f.UpdatedAt = time.Now()
	cp := *f

	return &cp, nil
}

// ValidateFlowDefinition validates a flow definition (stub — always succeeds).
func (b *InMemoryBackend) ValidateFlowDefinition() ([]any, error) {
	return []any{}, nil
}
