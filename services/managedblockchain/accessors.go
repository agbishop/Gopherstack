package managedblockchain

import (
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

const (
	// accessorStatusAvailable is the status for a ready accessor.
	accessorStatusAvailable = "AVAILABLE"
	// accessorDefaultType is the default accessor type.
	accessorDefaultType = "BILLING_TOKEN"
)

// accessorARN builds the ARN for a Managed Blockchain accessor.
func accessorARN(region, accountID, accessorID string) string {
	return arn.Build("managedblockchain", region, accountID, "accessors/"+accessorID)
}

// cloneAccessor returns a deep copy of a with the Tags map cloned.
func cloneAccessor(a *Accessor) *Accessor {
	cp := *a
	cp.Tags = maps.Clone(a.Tags)

	return &cp
}

// CreateAccessor creates a new accessor for token-based access.
func (b *InMemoryBackend) CreateAccessor(
	region, accountID, accessorType, networkType string,
	tags map[string]string,
) (*Accessor, error) {
	b.mu.Lock("CreateAccessor")
	defer b.mu.Unlock()

	if err := checkTagLimit(nil, tags); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	accessorID := uuid.NewString()
	billingToken := uuid.NewString()

	aType := accessorType
	if aType == "" {
		aType = accessorDefaultType
	}

	t := make(map[string]string)
	maps.Copy(t, tags)

	accessor := &Accessor{
		ID:           accessorID,
		Arn:          accessorARN(region, accountID, accessorID),
		BillingToken: billingToken,
		Type:         aType,
		NetworkType:  networkType,
		Status:       accessorStatusAvailable,
		CreationDate: &now,
		Tags:         t,
	}

	b.accessors.Put(accessor)
	b.arnToResource[accessor.Arn] = accessor

	return cloneAccessor(accessor), nil
}

// GetAccessor returns an accessor by ID.
func (b *InMemoryBackend) GetAccessor(accessorID string) (*Accessor, error) {
	b.mu.RLock("GetAccessor")
	defer b.mu.RUnlock()

	accessor, exists := b.accessors.Get(accessorID)
	if !exists {
		return nil, ErrAccessorNotFound
	}

	return cloneAccessor(accessor), nil
}

// DeleteAccessor removes an accessor.
func (b *InMemoryBackend) DeleteAccessor(accessorID string) error {
	b.mu.Lock("DeleteAccessor")
	defer b.mu.Unlock()

	accessor, exists := b.accessors.Get(accessorID)
	if !exists {
		return ErrAccessorNotFound
	}

	delete(b.arnToResource, accessor.Arn)
	b.accessors.Delete(accessorID)

	return nil
}

// ListAccessors returns all accessors sorted by ID, optionally filtered.
func (b *InMemoryBackend) ListAccessors(filter ListAccessorsFilter) ([]*Accessor, error) {
	b.mu.RLock("ListAccessors")
	defer b.mu.RUnlock()

	all := make([]*Accessor, 0, b.accessors.Len())

	for _, a := range b.accessors.All() {
		if filter.NetworkType != "" && a.NetworkType != filter.NetworkType {
			continue
		}

		all = append(all, cloneAccessor(a))
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].ID < all[j].ID
	})

	return all, nil
}

// AddAccessorInternal adds an accessor directly to the backend (for testing and seeding).
func (b *InMemoryBackend) AddAccessorInternal(region, accountID, accessorType, networkType string) *Accessor {
	b.mu.Lock("AddAccessorInternal")
	defer b.mu.Unlock()

	now := time.Now().UTC()
	accessorID := uuid.NewString()
	billingToken := uuid.NewString()

	accessor := &Accessor{
		ID:           accessorID,
		Arn:          accessorARN(region, accountID, accessorID),
		BillingToken: billingToken,
		Type:         accessorType,
		NetworkType:  networkType,
		Status:       accessorStatusAvailable,
		CreationDate: &now,
		Tags:         make(map[string]string),
	}

	b.accessors.Put(accessor)
	b.arnToResource[accessor.Arn] = accessor

	return cloneAccessor(accessor)
}
