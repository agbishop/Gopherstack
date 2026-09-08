package waf

import (
	"fmt"
	"maps"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) ipSetARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("ipset/%s", id))
}

// CreateIPSet creates a new IPSet.
func (b *InMemoryBackend) CreateIPSet(name, changeToken string, tags map[string]string) (*IPSet, error) {
	b.mu.Lock("CreateIPSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	ipSet := &IPSet{
		IPSetId:          id,
		Name:             name,
		IPSetDescriptors: []IPSetDescriptor{},
	}
	b.ipSets.Put(ipSet)

	if len(tags) > 0 {
		b.tags[b.ipSetARN(id)] = maps.Clone(tags)
	}

	return ipSet, nil
}

// GetIPSet retrieves an IPSet by ID.
func (b *InMemoryBackend) GetIPSet(id string) (*IPSet, error) {
	b.mu.RLock("GetIPSet")
	defer b.mu.RUnlock()

	ipSet, ok := b.ipSets.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return ipSet, nil
}

// UpdateIPSet updates an IPSet's descriptors.
func (b *InMemoryBackend) UpdateIPSet(id, changeToken string, updates []IPSetUpdate) error {
	b.mu.Lock("UpdateIPSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	ipSet, ok := b.ipSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	for _, u := range updates {
		descriptors, err := applyEntryUpdate(ipSet.IPSetDescriptors, u.Action, u.IPSetDescriptor,
			func(a, b IPSetDescriptor) bool { return a.Type == b.Type && a.Value == b.Value })
		if err != nil {
			return err
		}

		ipSet.IPSetDescriptors = descriptors
	}

	return nil
}

// DeleteIPSet deletes an IPSet. Real AWS rejects deletion while the IPSet is
// still used by a Rule/RateBasedRule predicate (WAFReferencedItemException)
// or still contains any IPSetDescriptors (WAFNonEmptyEntityException).
func (b *InMemoryBackend) DeleteIPSet(id, changeToken string) error {
	b.mu.Lock("DeleteIPSet")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	ipSet, ok := b.ipSets.Get(id)
	if !ok {
		return ErrNotFound
	}

	if b.matchSetReferenced(id) {
		return ErrReferencedItem
	}

	if len(ipSet.IPSetDescriptors) > 0 {
		return ErrNonEmptyEntity
	}

	b.ipSets.Delete(id)
	delete(b.tags, b.ipSetARN(id))

	return nil
}

// ListIPSets returns summaries of all IPSets.
func (b *InMemoryBackend) ListIPSets() []IPSetSummary {
	b.mu.RLock("ListIPSets")
	defer b.mu.RUnlock()

	all := b.ipSets.All()
	result := make([]IPSetSummary, 0, len(all))
	for _, s := range all {
		result = append(result, IPSetSummary{IPSetId: s.IPSetId, Name: s.Name})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].IPSetId < result[j].IPSetId })

	return result
}
