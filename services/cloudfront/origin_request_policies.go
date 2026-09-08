package cloudfront

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// orpConfigPartial reports whether cfg sets some but not all of the three
// sub-configs. The real UpdateOriginRequestPolicy requires the entire config
// on every call (aws-sdk-go-v2 api_op_UpdateOriginRequestPolicy.go: "You
// cannot update some fields independent of others"), so a partial config is
// invalid rather than a partial update.
func orpConfigPartial(cfg *OriginRequestPolicyConfig) bool {
	set := 0
	if cfg.HeadersConfig != nil {
		set++
	}
	if cfg.CookiesConfig != nil {
		set++
	}
	if cfg.QueryStringsConfig != nil {
		set++
	}

	return set > 0 && set < 3
}

// CreateOriginRequestPolicy creates a new Origin Request Policy.
func (b *InMemoryBackend) CreateOriginRequestPolicy(
	name, comment string,
	opts ...*OriginRequestPolicyConfig,
) (*OriginRequestPolicy, error) {
	b.mu.Lock("CreateOriginRequestPolicy")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: Name must not be empty", ErrValidation)
	}

	if _, exists := b.originRequestPolicyByName[name]; exists {
		return nil, fmt.Errorf(
			"%w: origin request policy with name %q already exists",
			ErrOriginRequestPolicyAlreadyExists,
			name,
		)
	}

	id := generateID()
	p := &OriginRequestPolicy{
		ID:      id,
		Name:    name,
		Comment: comment,
		ETag:    uuid.NewString(),
	}

	if len(opts) > 0 && opts[0] != nil {
		cfg := opts[0]
		if orpConfigPartial(cfg) {
			return nil, fmt.Errorf(
				"%w: HeadersConfig, CookiesConfig, and QueryStringsConfig must all be provided together",
				ErrValidation,
			)
		}
		p.HeadersConfig = cfg.HeadersConfig
		p.CookiesConfig = cfg.CookiesConfig
		p.QueryStringsConfig = cfg.QueryStringsConfig
	}

	b.originRequestPolicies.Put(p)
	b.originRequestPolicyByName[name] = id
	cp := *p

	return &cp, nil
}

// GetOriginRequestPolicy returns an Origin Request Policy by ID.
func (b *InMemoryBackend) GetOriginRequestPolicy(id string) (*OriginRequestPolicy, error) {
	b.mu.RLock("GetOriginRequestPolicy")
	defer b.mu.RUnlock()

	p, ok := b.originRequestPolicies.Get(id)
	if !ok {
		return nil, fmt.Errorf(
			"%w: origin request policy %s not found",
			ErrOriginRequestPolicyNotFound,
			id,
		)
	}

	cp := *p

	return &cp, nil
}

// ListOriginRequestPolicies returns all Origin Request Policies sorted by ID.
func (b *InMemoryBackend) ListOriginRequestPolicies() []*OriginRequestPolicy {
	b.mu.RLock("ListOriginRequestPolicies")
	defer b.mu.RUnlock()

	list := make([]*OriginRequestPolicy, 0, b.originRequestPolicies.Len())
	for _, p := range b.originRequestPolicies.All() {
		cp := *p
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	return list
}

// UpdateOriginRequestPolicy updates an existing Origin Request Policy.
func (b *InMemoryBackend) UpdateOriginRequestPolicy(
	id, name, comment string,
	opts ...*OriginRequestPolicyConfig,
) (*OriginRequestPolicy, error) {
	b.mu.Lock("UpdateOriginRequestPolicy")
	defer b.mu.Unlock()

	p, ok := b.originRequestPolicies.Get(id)
	if !ok {
		return nil, fmt.Errorf(
			"%w: origin request policy %s not found",
			ErrOriginRequestPolicyNotFound,
			id,
		)
	}

	if p.Managed {
		return nil, fmt.Errorf(
			"%w: origin request policy %s is an AWS-managed policy and cannot be updated",
			ErrIllegalUpdate, id,
		)
	}

	if name == "" {
		return nil, fmt.Errorf("%w: Name must not be empty", ErrValidation)
	}

	if len(opts) > 0 && opts[0] != nil && orpConfigPartial(opts[0]) {
		return nil, fmt.Errorf(
			"%w: HeadersConfig, CookiesConfig, and QueryStringsConfig must all be provided together",
			ErrValidation,
		)
	}

	if name != p.Name {
		if _, exists := b.originRequestPolicyByName[name]; exists {
			return nil, fmt.Errorf(
				"%w: origin request policy with name %q already exists",
				ErrOriginRequestPolicyAlreadyExists,
				name,
			)
		}

		delete(b.originRequestPolicyByName, p.Name)
		b.originRequestPolicyByName[name] = id
	}

	p.Name = name
	p.Comment = comment
	p.ETag = uuid.NewString()
	if len(opts) > 0 && opts[0] != nil {
		cfg := opts[0]
		p.HeadersConfig = cfg.HeadersConfig
		p.CookiesConfig = cfg.CookiesConfig
		p.QueryStringsConfig = cfg.QueryStringsConfig
	}

	cp := *p

	return &cp, nil
}

// DeleteOriginRequestPolicy deletes an Origin Request Policy by ID.
func (b *InMemoryBackend) DeleteOriginRequestPolicy(id string) error {
	b.mu.Lock("DeleteOriginRequestPolicy")
	defer b.mu.Unlock()

	p, ok := b.originRequestPolicies.Get(id)
	if !ok {
		return fmt.Errorf(
			"%w: origin request policy %s not found",
			ErrOriginRequestPolicyNotFound,
			id,
		)
	}

	if p.Managed {
		return fmt.Errorf(
			"%w: origin request policy %s is an AWS-managed policy and cannot be deleted",
			ErrIllegalDelete, id,
		)
	}

	if b.tokenReferencedByAnyDistribution(id) {
		return fmt.Errorf(
			"%w: origin request policy %s is attached to a distribution",
			ErrOriginRequestPolicyInUse, id,
		)
	}

	delete(b.originRequestPolicyByName, p.Name)
	b.originRequestPolicies.Delete(id)

	return nil
}

// --- Field Level Encryption CRUD ---
