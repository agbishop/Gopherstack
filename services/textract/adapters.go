package textract

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// validateAdapterFeatureTypes checks that adapter feature types are restricted to FORMS and QUERIES.
func validateAdapterFeatureTypes(featureTypes []string) error {
	if len(featureTypes) == 0 {
		return fmt.Errorf("%w: FeatureTypes must contain at least one value", ErrValidation)
	}

	for _, ft := range featureTypes {
		if !adapterAllowedFeatureTypes[ft] {
			return fmt.Errorf(
				"%w: invalid FeatureType %q for adapter (valid: FORMS, QUERIES)",
				ErrValidation,
				ft,
			)
		}
	}

	return nil
}

// buildAdapterARN constructs an ARN for a Textract adapter.
func buildAdapterARN(region, accountID, adapterID string) string {
	return arn.Build("textract", region, accountID, fmt.Sprintf("adapter/%s", adapterID))
}

// arnAdapterID extracts the adapter ID from a Textract adapter ARN.
// Returns empty string if the ARN is not an adapter ARN (e.g. it's a version ARN).
func arnAdapterID(resourceARN string) string {
	if len(resourceARN) <= 8 || resourceARN[:4] != "arn:" {
		return ""
	}

	const prefix = "adapter/"

	idx := lastIndex(resourceARN, prefix)
	if idx < 0 {
		return ""
	}

	rest := resourceARN[idx+len(prefix):]
	if contains(rest, "/version/") {
		return ""
	}

	return rest
}

// resolveARNToAdapter finds an adapter by ARN or adapter ID in the given
// region. The old map[string]*Adapter this replaces was always keyed by
// AdapterID (see CreateAdapterWithToken), so a linear scan checking
// a.AdapterID == X was behaviorally identical to a keyed lookup; this is a
// mechanical Table.Get in place of that scan, not a behavior change.
func resolveARNToAdapter(adapters *store.Table[Adapter], region, resourceARN string) (*Adapter, bool) {
	// Try direct adapter ID match first.
	if a, ok := adapters.Get(regionKey(region, resourceARN)); ok {
		return a, true
	}

	// Try ARN match: arn:aws:textract:region:account:adapter/adapterID
	adapterID := arnAdapterID(resourceARN)
	if adapterID == "" {
		return nil, false
	}

	return adapters.Get(regionKey(region, adapterID))
}

// lastIndex finds the last occurrence of substr in s.
func lastIndex(s, substr string) int {
	last := -1
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			last = i
		}
	}

	return last
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return lastIndex(s, substr) >= 0
}

// CreateAdapter creates a new Textract adapter and returns it.
func (b *InMemoryBackend) CreateAdapter(
	ctx context.Context,
	name, description, autoUpdate string,
	featureTypes []string,
	tags map[string]string,
) (*Adapter, error) {
	return b.CreateAdapterWithToken(ctx, name, description, autoUpdate, featureTypes, tags, "")
}

// CreateAdapterWithToken creates an adapter with ClientRequestToken dedup.
func (b *InMemoryBackend) CreateAdapterWithToken(
	ctx context.Context,
	name, description, autoUpdate string,
	featureTypes []string,
	tags map[string]string,
	clientRequestToken string,
) (*Adapter, error) {
	if err := validateAdapterFeatureTypes(featureTypes); err != nil {
		return nil, err
	}

	if autoUpdate == "" {
		autoUpdate = autoUpdateDisabled
	} else if autoUpdate != autoUpdateEnabled && autoUpdate != autoUpdateDisabled {
		return nil, fmt.Errorf(
			"%w: AutoUpdate must be ENABLED or DISABLED",
			ErrValidation,
		)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateAdapter")
	defer b.mu.Unlock()

	// Idempotency check.
	if clientRequestToken != "" {
		if existingID, ok := b.adapterClientTokenToIDStore(region)[clientRequestToken]; ok {
			if existing, ok2 := b.adapters.Get(regionKey(region, existingID)); ok2 {
				return cloneAdapter(existing), nil
			}
		}
	}

	adapterID := uuid.NewString()
	adapter := &Adapter{
		Region:             region,
		AdapterID:          adapterID,
		AdapterName:        name,
		AutoUpdate:         autoUpdate,
		CreationTime:       time.Now(),
		Description:        description,
		FeatureTypes:       append([]string{}, featureTypes...),
		Tags:               cloneTags(tags),
		ClientRequestToken: clientRequestToken,
	}
	b.adapters.Put(adapter)

	if clientRequestToken != "" {
		b.adapterClientTokenToIDStore(region)[clientRequestToken] = adapterID
	}

	return cloneAdapter(adapter), nil
}

// GetAdapter retrieves an adapter by ID.
func (b *InMemoryBackend) GetAdapter(ctx context.Context, adapterID string) (*Adapter, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("GetAdapter")
	defer b.mu.RUnlock()

	adapter, ok := b.adapters.Get(regionKey(region, adapterID))
	if !ok {
		return nil, fmt.Errorf("%w: adapter %s not found", ErrAdapterNotFound, adapterID)
	}

	return cloneAdapter(adapter), nil
}

// UpdateAdapter updates mutable fields on an existing adapter. name and
// description are true optionals -- nil means the field was omitted from the
// request and is left unchanged, matching UpdateAdapterInput.AdapterName /
// .Description (both *string in the real SDK, api_op_UpdateAdapter.go)
// rather than the enum-typed AutoUpdate, which the SDK sends only when
// non-empty (serializers.go's `len(v.AutoUpdate) > 0` guard).
func (b *InMemoryBackend) UpdateAdapter(
	ctx context.Context, adapterID string, name, description *string, autoUpdate string,
) (*Adapter, error) {
	if autoUpdate != "" && autoUpdate != autoUpdateEnabled && autoUpdate != autoUpdateDisabled {
		return nil, fmt.Errorf("%w: AutoUpdate must be ENABLED or DISABLED", ErrValidation)
	}

	if name == nil && description == nil && autoUpdate == "" {
		return nil, fmt.Errorf(
			"%w: at least one of AdapterName, Description, AutoUpdate must be specified",
			ErrValidation,
		)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateAdapter")
	defer b.mu.Unlock()

	adapter, ok := b.adapters.Get(regionKey(region, adapterID))
	if !ok {
		return nil, fmt.Errorf("%w: adapter %s not found", ErrAdapterNotFound, adapterID)
	}

	if name != nil {
		adapter.AdapterName = *name
	}

	if description != nil {
		adapter.Description = *description
	}

	if autoUpdate != "" {
		adapter.AutoUpdate = autoUpdate
	}

	return cloneAdapter(adapter), nil
}

// ListAdapters returns a sorted list of all adapters for the request region.
func (b *InMemoryBackend) ListAdapters(ctx context.Context) []Adapter {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListAdapters")
	defer b.mu.RUnlock()

	adapters := b.adaptersByRegion.Get(region)
	out := make([]Adapter, 0, len(adapters))

	for _, a := range adapters {
		out = append(out, *cloneAdapter(a))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].AdapterID < out[j].AdapterID
	})

	return out
}

// DeleteAdapter removes an adapter and all its versions by ID.
func (b *InMemoryBackend) DeleteAdapter(ctx context.Context, adapterID string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteAdapter")
	defer b.mu.Unlock()

	key := regionKey(region, adapterID)
	if !b.adapters.Has(key) {
		return fmt.Errorf("%w: adapter %s not found", ErrAdapterNotFound, adapterID)
	}

	b.adapters.Delete(key)

	// Remove all versions belonging to this adapter in this region. The index
	// slice mutates under Table.Delete, so it is cloned before the loop.
	versions := slices.Clone(b.adapterVersionsByAdapter.Get(key))
	for _, av := range versions {
		b.adapterVersions.Delete(adapterVersionTableKey(av))
	}

	return nil
}

// cloneAdapter returns a deep copy of an Adapter.
func cloneAdapter(a *Adapter) *Adapter {
	cp := *a
	cp.FeatureTypes = make([]string, len(a.FeatureTypes))
	copy(cp.FeatureTypes, a.FeatureTypes)
	cp.Tags = cloneTags(a.Tags)

	return &cp
}

// BuildAdapterARN returns the ARN for an adapter (exported for handler use).
func (b *InMemoryBackend) BuildAdapterARN(adapterID string) string {
	return buildAdapterARN(b.region, b.accountID, adapterID)
}
