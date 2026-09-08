package iotwireless

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func destinationARN(region, accountID, name string) string {
	return arn.Build("iotwireless", region, accountID, fmt.Sprintf("Destination/%s", name))
}

// copyDestination returns a shallow copy of dest with an independent Tags map.
func copyDestination(dest *Destination) *Destination {
	cp := *dest
	cp.Tags = make(map[string]string, len(dest.Tags))
	maps.Copy(cp.Tags, dest.Tags)

	return &cp
}

// CreateDestination creates a new destination.
func (b *InMemoryBackend) CreateDestination(
	accountID, region, name, expression, expressionType, roleArn, description string,
	tags map[string]string,
) (*Destination, error) {
	b.mu.Lock("CreateDestination")
	defer b.mu.Unlock()

	arn := destinationARN(region, accountID, name)

	dest := &Destination{
		Name:           name,
		ARN:            arn,
		Expression:     expression,
		ExpressionType: expressionType,
		RoleArn:        roleArn,
		Description:    description,
		Tags:           newTagsCopy(tags),
		CreatedAt:      time.Now(),
		AccountID:      accountID,
		Region:         region,
	}

	b.destinations.Put(dest)
	b.storeResourceTagsLocked(arn, tags)

	return copyDestination(dest), nil
}

// GetDestination returns a destination by name.
func (b *InMemoryBackend) GetDestination(accountID, region, name string) (*Destination, error) {
	b.mu.RLock("GetDestination")
	defer b.mu.RUnlock()

	dest, ok := b.destinations.Get(compositeKey(accountID, region, name))
	if !ok {
		return nil, ErrDestinationNotFound
	}

	return copyDestination(dest), nil
}

// ListDestinations returns all destinations for the given account and region.
// ListDestinations returns all destinations for the given account and region,
// sorted by name for deterministic output.
func (b *InMemoryBackend) ListDestinations(accountID, region string) []*Destination {
	b.mu.RLock("ListDestinations")
	defer b.mu.RUnlock()

	all := b.destinations.All()
	result := make([]*Destination, 0, len(all))

	for _, dest := range all {
		if dest.AccountID == accountID && dest.Region == region {
			result = append(result, copyDestination(dest))
		}
	}

	slices.SortFunc(result, func(a, b *Destination) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return result
}

// DeleteDestination deletes a destination by name. Real AWS models
// ConflictException on this op; a wireless device's DestinationName is the
// only referrer a destination has (CreateWirelessDeviceInput.DestinationName
// is required), so deletion is refused while any device in the
// account/region still references it.
func (b *InMemoryBackend) DeleteDestination(accountID, region, name string) error {
	b.mu.Lock("DeleteDestination")
	defer b.mu.Unlock()

	key := compositeKey(accountID, region, name)

	dest, ok := b.destinations.Get(key)
	if !ok {
		return ErrDestinationNotFound
	}

	for _, d := range b.devices.All() {
		if d.AccountID == accountID && d.Region == region && d.DestinationName == name {
			return ErrDestinationInUse
		}
	}

	delete(b.resourceTags, dest.ARN)
	b.destinations.Delete(key)

	return nil
}

// UpdateDestination updates mutable fields on an existing destination.
func (b *InMemoryBackend) UpdateDestination(
	accountID, region, name, expression, expressionType, roleArn, description string,
) error {
	b.mu.Lock("UpdateDestination")
	defer b.mu.Unlock()

	dest, ok := b.destinations.Get(compositeKey(accountID, region, name))
	if !ok {
		return ErrDestinationNotFound
	}

	if expression != "" {
		dest.Expression = expression
	}

	if expressionType != "" {
		dest.ExpressionType = expressionType
	}

	if roleArn != "" {
		dest.RoleArn = roleArn
	}

	dest.Description = description

	return nil
}

// AddDestinationInternal inserts a Destination directly into the backend, bypassing ID generation.
// Intended for test setup only.
func (b *InMemoryBackend) AddDestinationInternal(accountID, region string, dest *Destination) {
	b.mu.Lock("AddDestinationInternal")
	defer b.mu.Unlock()

	cp := copyDestination(dest)
	cp.AccountID = accountID
	cp.Region = region
	b.destinations.Put(cp)
	b.storeResourceTagsLocked(dest.ARN, dest.Tags)
}
