package apigateway

import (
	"fmt"
	"sort"
	"time"
)

// CreateUsagePlan creates a new usage plan.
func (b *InMemoryBackend) CreateUsagePlan(input CreateUsagePlanInput) (*UsagePlan, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateUsagePlan")
	defer b.mu.Unlock()

	id := randomID(apiIDLength)
	backendTags := initTagsFromInput("apigw.usageplan."+id+".tags", input.Tags)

	plan := &UsagePlan{
		ID:          id,
		Name:        input.Name,
		Description: input.Description,
		Throttle:    input.Throttle,
		Quota:       input.Quota,
		APIStages:   input.APIStages,
		Tags:        backendTags,
	}
	b.usagePlans.Put(plan)

	cp := *plan

	return &cp, nil
}

// CreateUsagePlanKey associates an API key with a usage plan.
func (b *InMemoryBackend) CreateUsagePlanKey(input CreateUsagePlanKeyInput) (*UsagePlanKey, error) {
	if input.UsagePlanID == "" {
		return nil, fmt.Errorf("%w: usagePlanId is required", ErrInvalidParameter)
	}

	if input.KeyID == "" {
		return nil, fmt.Errorf("%w: keyId is required", ErrInvalidParameter)
	}

	if input.KeyType == "" {
		return nil, fmt.Errorf("%w: keyType is required", ErrInvalidParameter)
	}

	if input.KeyType != "API_KEY" {
		return nil, fmt.Errorf("%w: keyType must be API_KEY, got %q", ErrInvalidParameter, input.KeyType)
	}

	b.mu.Lock("CreateUsagePlanKey")
	defer b.mu.Unlock()

	if !b.usagePlans.Has(input.UsagePlanID) {
		return nil, fmt.Errorf("%w: usage plan %s not found", ErrUsagePlanNotFound, input.UsagePlanID)
	}

	apiKey, exists := b.apiKeys.Get(input.KeyID)
	if !exists {
		return nil, fmt.Errorf("%w: API key %s not found", ErrAPIKeyNotFound, input.KeyID)
	}

	if b.usagePlanKeys.Has(usagePlanKeyKey(input.UsagePlanID, input.KeyID)) {
		return nil, fmt.Errorf("%w: key %s already associated with usage plan", ErrAlreadyExists, input.KeyID)
	}

	upk := &UsagePlanKey{
		ID:          apiKey.ID,
		Type:        input.KeyType,
		Value:       apiKey.Value,
		Name:        apiKey.Name,
		UsagePlanID: input.UsagePlanID,
	}
	b.usagePlanKeys.Put(upk)

	cp := *upk

	return &cp, nil
}

// GetUsagePlan retrieves a usage plan by ID.
func (b *InMemoryBackend) GetUsagePlan(id string) (*UsagePlan, error) {
	b.mu.RLock("GetUsagePlan")
	defer b.mu.RUnlock()
	p, ok := b.usagePlans.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: usage plan %s not found", ErrUsagePlanNotFound, id)
	}
	cp := *p

	return &cp, nil
}

// GetUsagePlans returns all usage plans sorted by ID.
func (b *InMemoryBackend) GetUsagePlans() ([]UsagePlan, error) {
	b.mu.RLock("GetUsagePlans")
	defer b.mu.RUnlock()
	all := make([]UsagePlan, 0, b.usagePlans.Len())
	for _, p := range b.usagePlans.All() {
		all = append(all, *p)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	return all, nil
}

// GetUsagePlansForKey returns usage plans that keyID is associated with,
// sorted by ID. Backs GetUsagePlans' keyId query filter (real key: "keyId",
// apigateway@v1.42.4/serializers.go:7521).
func (b *InMemoryBackend) GetUsagePlansForKey(keyID string) ([]UsagePlan, error) {
	b.mu.RLock("GetUsagePlansForKey")
	defer b.mu.RUnlock()
	all := make([]UsagePlan, 0)
	for _, p := range b.usagePlans.All() {
		if b.usagePlanKeys.Has(usagePlanKeyKey(p.ID, keyID)) {
			all = append(all, *p)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	return all, nil
}

// DeleteUsagePlan removes a usage plan by ID along with its key associations.
func (b *InMemoryBackend) DeleteUsagePlan(id string) error {
	b.mu.Lock("DeleteUsagePlan")
	defer b.mu.Unlock()
	if !b.usagePlans.Delete(id) {
		return fmt.Errorf("%w: usage plan %s not found", ErrUsagePlanNotFound, id)
	}
	for _, k := range append([]*UsagePlanKey{}, b.usagePlanKeysByPlan.Get(id)...) {
		b.usagePlanKeys.Delete(usagePlanKeyKeyFn(k))
	}
	delete(b.usageOverrides, id)

	return nil
}

// GetUsagePlanKey retrieves a single key from a usage plan.
func (b *InMemoryBackend) GetUsagePlanKey(usagePlanID, keyID string) (*UsagePlanKey, error) {
	b.mu.RLock("GetUsagePlanKey")
	defer b.mu.RUnlock()
	if !b.usagePlans.Has(usagePlanID) {
		return nil, fmt.Errorf("%w: usage plan %s not found", ErrUsagePlanNotFound, usagePlanID)
	}
	k, ok := b.usagePlanKeys.Get(usagePlanKeyKey(usagePlanID, keyID))
	if !ok {
		return nil, fmt.Errorf("%w: usage plan key %s not found", ErrUsagePlanKeyNotFound, keyID)
	}
	cp := *k

	return &cp, nil
}

// GetUsagePlanKeys returns all keys for a usage plan sorted by ID.
func (b *InMemoryBackend) GetUsagePlanKeys(usagePlanID string) ([]UsagePlanKey, error) {
	b.mu.RLock("GetUsagePlanKeys")
	defer b.mu.RUnlock()
	if !b.usagePlans.Has(usagePlanID) {
		return nil, fmt.Errorf("%w: usage plan %s not found", ErrUsagePlanNotFound, usagePlanID)
	}
	group := b.usagePlanKeysByPlan.Get(usagePlanID)
	all := make([]UsagePlanKey, 0, len(group))
	for _, k := range group {
		all = append(all, *k)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	return all, nil
}

// DeleteUsagePlanKey removes a key from a usage plan.
func (b *InMemoryBackend) DeleteUsagePlanKey(usagePlanID, keyID string) error {
	b.mu.Lock("DeleteUsagePlanKey")
	defer b.mu.Unlock()
	if !b.usagePlans.Has(usagePlanID) {
		return fmt.Errorf("%w: usage plan %s not found", ErrUsagePlanNotFound, usagePlanID)
	}
	if !b.usagePlanKeys.Delete(usagePlanKeyKey(usagePlanID, keyID)) {
		return fmt.Errorf("%w: usage plan key %s not found", ErrUsagePlanKeyNotFound, keyID)
	}
	delete(b.usageOverrides[usagePlanID], keyID)

	return nil
}

// GetUsagePlansPage returns usage plans with cursor-based pagination.
func (b *InMemoryBackend) GetUsagePlansPage(limit int, position string) ([]UsagePlan, string, error) {
	b.mu.RLock("GetUsagePlansPage")
	defer b.mu.RUnlock()

	all := make([]UsagePlan, 0, b.usagePlans.Len())
	for _, p := range b.usagePlans.All() {
		all = append(all, *p)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	page, pos := paginatePageByKey(all, limit, position, func(p UsagePlan) string { return p.ID })

	return page, pos, nil
}

// UpdateUsagePlan updates a usage plan's name, description, throttle, or quota.
func (b *InMemoryBackend) UpdateUsagePlan(input UpdateUsagePlanInput) (*UsagePlan, error) {
	b.mu.Lock("UpdateUsagePlan")
	defer b.mu.Unlock()

	p, ok := b.usagePlans.Get(input.UsagePlanID)
	if !ok {
		return nil, fmt.Errorf("%w: usage plan %s not found", ErrUsagePlanNotFound, input.UsagePlanID)
	}

	if input.Name != "" {
		p.Name = input.Name
	}

	if input.Description != "" {
		p.Description = input.Description
	}

	if input.Throttle != nil {
		p.Throttle = input.Throttle
	}

	if input.Quota != nil {
		p.Quota = input.Quota
	}

	if input.ProductCode != nil {
		p.ProductCode = *input.ProductCode
	}

	if input.APIStages != nil {
		p.APIStages = input.APIStages
	}

	cp := *p

	return &cp, nil
}

// EnforceUsagePlan applies usage-plan quota and throttle limits for an API key on the
// given API stage. It returns nil when the request is allowed or when the key is not
// associated with a usage plan for the stage (unmetered, matching a bare API key),
// ErrQuotaExceeded when the period quota is exhausted, or ErrThrottled when the
// rate/burst limit is exceeded.
func (b *InMemoryBackend) EnforceUsagePlan(apiID, stageName, keyID string) error {
	b.mu.Lock("EnforceUsagePlan")
	defer b.mu.Unlock()

	plan := b.usagePlanForKeyLocked(keyID, apiID, stageName)
	if plan == nil {
		return nil
	}

	return b.usage.enforce(plan, apiID, stageName, keyID, time.Now())
}

// usagePlanForKeyLocked returns the usage plan that both contains keyID and is
// associated with the given api:stage, or nil. Callers must hold b.mu.
func (b *InMemoryBackend) usagePlanForKeyLocked(keyID, apiID, stageName string) *UsagePlan {
	// Deterministic iteration by plan ID so a key in multiple plans resolves stably.
	all := b.usagePlans.All()
	ids := make([]string, 0, len(all))
	for _, p := range all {
		ids = append(ids, p.ID)
	}
	sort.Strings(ids)

	for _, id := range ids {
		plan, _ := b.usagePlans.Get(id)
		if !b.usagePlanKeys.Has(usagePlanKeyKey(id, keyID)) {
			continue
		}
		for i := range plan.APIStages {
			st := plan.APIStages[i]
			if st.RestAPIID == apiID && st.Stage == stageName {
				return plan
			}
		}
	}

	return nil
}
