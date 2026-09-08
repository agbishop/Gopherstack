package waf

import (
	"fmt"
	"maps"
	"sort"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) webACLARN(id string) string {
	return arn.Build("waf", "", b.accountID, fmt.Sprintf("webacl/%s", id))
}

// CreateWebACL creates a new WebACL.
func (b *InMemoryBackend) CreateWebACL(
	name, metricName string,
	defaultAction WafAction,
	changeToken string,
	tags map[string]string,
) (*WebACL, error) {
	b.mu.Lock("CreateWebACL")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return nil, err
	}

	id := uuid.New().String()
	acl := &WebACL{
		WebACLId:      id,
		Name:          name,
		MetricName:    metricName,
		DefaultAction: defaultAction,
		Rules:         []ActivatedRule{},
		WebACLArn:     b.webACLARN(id),
	}
	b.webACLs.Put(acl)

	if len(tags) > 0 {
		b.tags[acl.WebACLArn] = maps.Clone(tags)
	}

	return acl, nil
}

// GetWebACL retrieves a WebACL by ID.
func (b *InMemoryBackend) GetWebACL(id string) (*WebACL, error) {
	b.mu.RLock("GetWebACL")
	defer b.mu.RUnlock()

	acl, ok := b.webACLs.Get(id)
	if !ok {
		return nil, ErrNotFound
	}

	return acl, nil
}

// UpdateWebACL updates a WebACL's default action and rules.
func (b *InMemoryBackend) UpdateWebACL(
	id, changeToken string,
	defaultAction *WafAction,
	updates []WebACLUpdate,
) error {
	b.mu.Lock("UpdateWebACL")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	acl, ok := b.webACLs.Get(id)
	if !ok {
		return ErrNotFound
	}

	if defaultAction != nil {
		acl.DefaultAction = *defaultAction
	}

	for _, u := range updates {
		if err := applyWebACLUpdate(acl, u); err != nil {
			return err
		}
	}

	sort.Slice(acl.Rules, func(i, j int) bool {
		return acl.Rules[i].Priority < acl.Rules[j].Priority
	})

	return nil
}

// applyWebACLUpdate inserts or deletes a single ActivatedRule. Real AWS
// rejects a redundant insert/delete with WAFInvalidOperationException
// (types/errors.go: "You tried to add a Rule to a WebACL, but the Rule
// already exists"/"...remove a Rule..., but the Rule isn't in the
// specified WebACL").
func applyWebACLUpdate(acl *WebACL, u WebACLUpdate) error {
	activated := false
	for _, r := range acl.Rules {
		if r.RuleId == u.ActivatedRule.RuleId {
			activated = true

			break
		}
	}

	switch u.Action {
	case updateInsert:
		if activated {
			return fmt.Errorf("%w: rule %q is already activated in this WebACL",
				ErrInvalidOperation, u.ActivatedRule.RuleId)
		}

		acl.Rules = append(acl.Rules, u.ActivatedRule)
	case updateDelete:
		if !activated {
			return fmt.Errorf("%w: rule %q isn't activated in this WebACL",
				ErrInvalidOperation, u.ActivatedRule.RuleId)
		}

		filtered := acl.Rules[:0]
		for _, r := range acl.Rules {
			if r.RuleId != u.ActivatedRule.RuleId {
				filtered = append(filtered, r)
			}
		}
		acl.Rules = filtered
	}

	return nil
}

// DeleteWebACL deletes a WebACL. Real AWS rejects deletion while the WebACL
// still contains any Rules (WAFNonEmptyEntityException) -- callers must
// first UpdateWebACL to remove every ActivatedRule.
func (b *InMemoryBackend) DeleteWebACL(id, changeToken string) error {
	b.mu.Lock("DeleteWebACL")
	defer b.mu.Unlock()

	if err := b.validateChangeToken(changeToken); err != nil {
		return err
	}

	acl, ok := b.webACLs.Get(id)
	if !ok {
		return ErrNotFound
	}

	if len(acl.Rules) > 0 {
		return ErrNonEmptyEntity
	}

	b.webACLs.Delete(id)
	delete(b.tags, acl.WebACLArn)

	return nil
}

// ListWebACLs returns summaries of all WebACLs.
func (b *InMemoryBackend) ListWebACLs() []WebACLSummary {
	b.mu.RLock("ListWebACLs")
	defer b.mu.RUnlock()

	all := b.webACLs.All()
	result := make([]WebACLSummary, 0, len(all))
	for _, acl := range all {
		result = append(result, WebACLSummary{WebACLId: acl.WebACLId, Name: acl.Name})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].WebACLId < result[j].WebACLId })

	return result
}
