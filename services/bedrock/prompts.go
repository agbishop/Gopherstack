package bedrock

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreatePrompt creates a new Bedrock Prompt.
func (b *InMemoryBackend) CreatePrompt(
	name, description string,
	tags map[string]string,
) (*Prompt, error) {
	b.mu.Lock("CreatePrompt")
	defer b.mu.Unlock()

	if _, ok := b.promptsByName[name]; ok {
		return nil, fmt.Errorf("%w: prompt %q already exists", ErrAlreadyExists, name)
	}

	b.promptCounter++
	id := fmt.Sprintf("prompt-%08d", b.promptCounter)
	promptArn := arn.Build("bedrock", b.region, b.accountID, "prompt/"+id)
	now := time.Now()

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	p := &Prompt{
		CreatedAt:   now,
		UpdatedAt:   now,
		PromptID:    id,
		PromptArn:   promptArn,
		Name:        name,
		Description: description,
		Tags:        tagsCopy,
	}
	b.prompts.Put(p)
	b.promptsByName[name] = id
	cp := *p

	return &cp, nil
}

// GetPrompt returns a Prompt by ID.
func (b *InMemoryBackend) GetPrompt(promptID string) (*Prompt, error) {
	b.mu.RLock("GetPrompt")
	defer b.mu.RUnlock()

	p, ok := b.prompts.Get(promptID)
	if !ok {
		return nil, fmt.Errorf("%w: prompt %q not found", ErrNotFound, promptID)
	}

	cp := *p

	return &cp, nil
}

// ListPrompts returns all prompts with pagination.
func (b *InMemoryBackend) ListPrompts(maxResults int, nextToken string) ([]*Prompt, string) {
	b.mu.RLock("ListPrompts")
	defer b.mu.RUnlock()

	list := make([]*Prompt, 0, b.prompts.Len())
	for _, p := range b.prompts.All() {
		cp := *p
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

	return paginate(list, maxResults, nextToken)
}

// UpdatePrompt updates a Prompt.
func (b *InMemoryBackend) UpdatePrompt(promptID, name, description string) (*Prompt, error) {
	b.mu.Lock("UpdatePrompt")
	defer b.mu.Unlock()

	p, ok := b.prompts.Get(promptID)
	if !ok {
		return nil, fmt.Errorf("%w: prompt %q not found", ErrNotFound, promptID)
	}

	if name != "" && name != p.Name {
		delete(b.promptsByName, p.Name)
		p.Name = name
		b.promptsByName[name] = promptID
	}

	if description != "" {
		p.Description = description
	}

	p.UpdatedAt = time.Now()
	cp := *p

	return &cp, nil
}

// DeletePrompt deletes a Prompt and its versions.
// Without this, GetPromptVersion/ListPromptVersions keep returning rows for a
// prompt ID that no longer resolves (gopherstack-jkiu, same shape as
// gopherstack-wg7i's DeleteKnowledgeBase fix).
func (b *InMemoryBackend) DeletePrompt(promptID string) error {
	b.mu.Lock("DeletePrompt")
	defer b.mu.Unlock()

	p, ok := b.prompts.Get(promptID)
	if !ok {
		return fmt.Errorf("%w: prompt %q not found", ErrNotFound, promptID)
	}

	delete(b.promptsByName, p.Name)
	delete(b.agentTags, p.PromptArn)
	delete(b.promptVersionCounters, promptID)

	// Reset, not delete: see DeleteFlow's comment in flows.go -- same
	// register-once-panic-on-reregister landmine applies to promptVersionsStore.
	if versions, versionsOK := b.promptVersions[promptID]; versionsOK {
		versions.Reset()
	}

	b.prompts.Delete(promptID)

	return nil
}
