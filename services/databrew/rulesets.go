package databrew

import (
	"context"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) rulesetARN(region, name string) string {
	return arn.Build("databrew", region, b.accountID, "ruleset/"+name)
}

func (b *InMemoryBackend) CreateRuleset(
	ctx context.Context,
	name, description, targetArn string,
	rules []Rule,
	tags map[string]string,
) (*Ruleset, error) {
	b.mu.Lock("CreateRuleset")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if name == "" {
		return nil, ErrValidation
	}
	t := b.rulesetsTable(region)
	if t.Has(name) {
		return nil, ErrAlreadyExists
	}
	rs := &Ruleset{
		Name: name, Arn: b.rulesetARN(region, name), Description: description,
		TargetArn: targetArn, Rules: append([]Rule(nil), rules...), RuleCount: len(rules),
		Tags: maps.Clone(tags), CreateDate: float64(time.Now().Unix()),
		LastModifiedDate: float64(time.Now().Unix()), AccountID: b.accountID,
	}
	t.Put(rs)

	return rs, nil
}

func (b *InMemoryBackend) DescribeRuleset(ctx context.Context, name string) (*Ruleset, error) {
	b.mu.RLock("DescribeRuleset")
	defer b.mu.RUnlock()
	region := getRegion(ctx, b.defaultRegion)
	rs, ok := b.rulesetsTable(region).Get(name)
	if !ok {
		return nil, ErrNotFound
	}
	cp := *rs
	cp.Tags = maps.Clone(rs.Tags)
	cp.Rules = append([]Rule(nil), rs.Rules...)

	return &cp, nil
}

func (b *InMemoryBackend) ListRulesets(
	ctx context.Context,
	maxResults int,
	nextToken, targetArn string,
) ([]*Ruleset, string) {
	b.mu.RLock("ListRulesets")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.defaultRegion)
	t := b.rulesetsTable(region)
	keys := snapshotKeys(t, rulesetKeyFn)
	filtered := keys
	if targetArn != "" {
		filtered = make([]string, 0, len(keys))
		for _, k := range keys {
			v, _ := t.Get(k)
			if v.TargetArn == targetArn {
				filtered = append(filtered, k)
			}
		}
	}
	pageKeys, next := paginateKeys(filtered, maxResults, nextToken)
	out := make([]*Ruleset, 0, len(pageKeys))
	for _, k := range pageKeys {
		v, _ := t.Get(k)
		cp := *v
		cp.Tags = maps.Clone(v.Tags)
		cp.Rules = append([]Rule(nil), v.Rules...)
		out = append(out, &cp)
	}

	return out, next
}

// UpdateRuleset overwrites Rules unconditionally (UpdateRulesetInput marks
// it "This member is required") but only overwrites Description when
// non-empty: Description has no such marker, so a caller updating just
// Rules must not have their existing Description clobbered.
func (b *InMemoryBackend) UpdateRuleset(
	ctx context.Context,
	name, description string,
	rules []Rule,
) error {
	b.mu.Lock("UpdateRuleset")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	rs, ok := b.rulesetsTable(region).Get(name)
	if !ok {
		return ErrNotFound
	}
	if description != "" {
		rs.Description = description
	}
	rs.Rules = rules
	rs.RuleCount = len(rules)
	rs.LastModifiedDate = float64(time.Now().Unix())

	return nil
}

func (b *InMemoryBackend) DeleteRuleset(ctx context.Context, name string) error {
	b.mu.Lock("DeleteRuleset")
	defer b.mu.Unlock()
	region := getRegion(ctx, b.defaultRegion)
	if !b.rulesetsTable(region).Delete(name) {
		return ErrNotFound
	}

	return nil
}
