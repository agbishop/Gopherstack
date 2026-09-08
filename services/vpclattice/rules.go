package vpclattice

import (
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// resolveRuleID resolves a rule identifier within a listener to a rule ID.
func (b *InMemoryBackend) resolveRuleID(serviceID, listenerID, identifier string) (string, bool) {
	if r, ok := b.rules.Get(identifier); ok && r.ServiceID == serviceID && r.ListenerID == listenerID {
		return identifier, true
	}
	for _, r := range b.rulesByListener.Get(listenerID) {
		if r.ServiceID == serviceID && r.ARN == identifier {
			return r.ID, true
		}
	}

	return "", false
}

// ------- Rule operations -------

// CreateRule creates a listener rule.
func (b *InMemoryBackend) CreateRule(
	serviceID, listenerID, name string,
	priority int32,
	action *RuleAction,
	match *RuleMatch,
	tags map[string]string,
) (*Rule, error) {
	if name == "" {
		return nil, ErrInvalidParameter
	}

	b.mu.Lock("CreateRule")
	defer b.mu.Unlock()

	svcID, ok := b.resolveServiceID(serviceID)
	if !ok {
		return nil, ErrNotFound
	}

	lID, ok := b.resolveListenerID(svcID, listenerID)
	if !ok {
		return nil, ErrNotFound
	}

	// check duplicate name within listener
	for _, r := range b.rulesByListener.Get(lID) {
		if r.Name == name {
			return nil, ErrAlreadyExists
		}
	}

	now := time.Now().UTC()
	id := newID(idPrefixRule)
	ruleARN := b.buildRuleARN(svcID, lID, id)

	r := &storedRule{
		ARN:           ruleARN,
		ID:            id,
		ServiceID:     svcID,
		ListenerID:    lID,
		Name:          name,
		Priority:      priority,
		Action:        action,
		Match:         match,
		Tags:          copyTags(tags),
		CreatedAt:     now,
		LastUpdatedAt: now,
	}

	b.rules.Put(r)
	b.tags[ruleARN] = copyTags(tags)

	return r.toRule(), nil
}

// GetRule returns a rule.
func (b *InMemoryBackend) GetRule(serviceID, listenerID, ruleID string) (*Rule, error) {
	b.mu.RLock("GetRule")
	defer b.mu.RUnlock()

	svcID, ok := b.resolveServiceID(serviceID)
	if !ok {
		return nil, ErrNotFound
	}

	lID, ok := b.resolveListenerID(svcID, listenerID)
	if !ok {
		return nil, ErrNotFound
	}

	rID, ok := b.resolveRuleID(svcID, lID, ruleID)
	if !ok {
		return nil, ErrNotFound
	}

	r, _ := b.rules.Get(rID)

	return r.toRule(), nil
}

// UpdateRule updates a rule. Real AWS rejects modifying a default listener
// rule with ValidationException ("You can't modify a default listener
// rule. To modify a default listener rule, use UpdateListener." --
// aws-sdk-go-v2/service/vpclattice's api_op_UpdateRule.go doc comment).
func (b *InMemoryBackend) UpdateRule(
	serviceID, listenerID, ruleID string,
	priority int32,
	action *RuleAction,
	match *RuleMatch,
) (*Rule, error) {
	b.mu.Lock("UpdateRule")
	defer b.mu.Unlock()

	svcID, ok := b.resolveServiceID(serviceID)
	if !ok {
		return nil, ErrNotFound
	}

	lID, ok := b.resolveListenerID(svcID, listenerID)
	if !ok {
		return nil, ErrNotFound
	}

	rID, ok := b.resolveRuleID(svcID, lID, ruleID)
	if !ok {
		return nil, ErrNotFound
	}

	r, _ := b.rules.Get(rID)

	if r.IsDefault {
		return nil, ErrInvalidParameter
	}

	if priority != 0 {
		r.Priority = priority
	}

	if action != nil {
		r.Action = action
	}

	if match != nil {
		r.Match = match
	}

	r.LastUpdatedAt = time.Now().UTC()

	return r.toRule(), nil
}

// DeleteRule deletes a rule.
func (b *InMemoryBackend) DeleteRule(serviceID, listenerID, ruleID string) error {
	b.mu.Lock("DeleteRule")
	defer b.mu.Unlock()

	svcID, ok := b.resolveServiceID(serviceID)
	if !ok {
		return ErrNotFound
	}

	lID, ok := b.resolveListenerID(svcID, listenerID)
	if !ok {
		return ErrNotFound
	}

	rID, ok := b.resolveRuleID(svcID, lID, ruleID)
	if !ok {
		return ErrNotFound
	}

	r, _ := b.rules.Get(rID)

	if r.IsDefault {
		return ErrInvalidParameter
	}

	b.rules.Delete(rID)
	delete(b.tags, r.ARN)

	return nil
}

// ListRules lists rules for a listener.
func (b *InMemoryBackend) ListRules(
	serviceID, listenerID string,
	maxResults int32,
	nextToken string,
) ([]*RuleSummary, string, error) {
	b.mu.RLock("ListRules")
	defer b.mu.RUnlock()

	svcID, ok := b.resolveServiceID(serviceID)
	if !ok {
		return nil, "", ErrNotFound
	}

	lID, ok := b.resolveListenerID(svcID, listenerID)
	if !ok {
		return nil, "", ErrNotFound
	}

	all := make([]*RuleSummary, 0)

	for _, r := range b.rulesByListener.Get(lID) {
		if r.ServiceID == svcID {
			all = append(all, r.toSummary())
		}
	}

	sort.Slice(all, func(i, j int) bool { return all[i].Priority < all[j].Priority })

	p := page.New(all, nextToken, int(maxResults), defaultMaxResults)

	return p.Data, p.Next, nil
}

// BatchUpdateRule updates multiple rules atomically.
func (b *InMemoryBackend) BatchUpdateRule(
	serviceID, listenerID string,
	updates []*RuleUpdate,
) ([]*RuleUpdateSuccess, []*RuleUpdateFailure, error) {
	b.mu.Lock("BatchUpdateRule")
	defer b.mu.Unlock()

	svcID, ok := b.resolveServiceID(serviceID)
	if !ok {
		return nil, nil, ErrNotFound
	}

	lID, ok := b.resolveListenerID(svcID, listenerID)
	if !ok {
		return nil, nil, ErrNotFound
	}

	successes := make([]*RuleUpdateSuccess, 0, len(updates))
	failures := make([]*RuleUpdateFailure, 0)
	now := time.Now().UTC()

	for _, u := range updates {
		rID, found := b.resolveRuleID(svcID, lID, u.RuleIdentifier)
		if !found {
			failures = append(failures, &RuleUpdateFailure{
				RuleIdentifier: u.RuleIdentifier,
				Code:           "NOT_FOUND",
				Message:        "Rule not found",
			})

			continue
		}

		r, _ := b.rules.Get(rID)

		if u.Priority != 0 {
			r.Priority = u.Priority
		}

		if u.Action != nil {
			r.Action = u.Action
		}

		if u.Match != nil {
			r.Match = u.Match
		}

		r.LastUpdatedAt = now
		successes = append(successes, &RuleUpdateSuccess{
			ARN:       r.ARN,
			ID:        r.ID,
			Name:      r.Name,
			Priority:  r.Priority,
			IsDefault: r.IsDefault,
			Action:    r.Action,
			Match:     r.Match,
		})
	}

	return successes, failures, nil
}
