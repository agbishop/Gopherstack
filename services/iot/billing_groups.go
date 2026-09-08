package iot

import (
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// AddThingToBillingGroup adds a thing to a billing group.
func (b *InMemoryBackend) AddThingToBillingGroup(input *AddThingToBillingGroupInput) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.thingBillingGroups[thingKey(input.ThingName, input.ThingArn)] = input.BillingGroupName

	return nil
}

func (b *InMemoryBackend) ListThingsInBillingGroup(groupName string) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var out []string
	for thingName, bg := range b.thingBillingGroups {
		if bg == groupName {
			out = append(out, thingName)
		}
	}
	sort.Strings(out)

	return out
}

func (b *InMemoryBackend) RemoveThingFromBillingGroup(thingName, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.thingBillingGroups, thingName)

	return nil
}

// BillingGroupProperties holds description for a billing group.
type BillingGroupProperties struct {
	BillingGroupDescription string `json:"billingGroupDescription,omitempty"`
}

// BillingGroupMetadata holds metadata for a billing group.
type BillingGroupMetadata struct {
	CreationDate float64 `json:"creationDate,omitempty"`
}

// BillingGroup represents an IoT billing group.
type BillingGroup struct {
	Tags                   map[string]string      `json:"tags,omitempty"`
	BillingGroupName       string                 `json:"billingGroupName"`
	BillingGroupARN        string                 `json:"billingGroupArn"`
	BillingGroupID         string                 `json:"billingGroupId"`
	BillingGroupProperties BillingGroupProperties `json:"billingGroupProperties,omitzero"`
	BillingGroupMetadata   BillingGroupMetadata   `json:"billingGroupMetadata,omitzero"`
	Version                int64                  `json:"version"`
}

func cloneBillingGroup(bg *BillingGroup) *BillingGroup {
	cp := *bg

	return &cp
}

func (b *InMemoryBackend) billingGroupARN(name string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("billinggroup/%s", name))
}

// CreateBillingGroupInput holds input for CreateBillingGroup.
type CreateBillingGroupInput struct {
	BillingGroupName       string                 `json:"billingGroupName"`
	BillingGroupProperties BillingGroupProperties `json:"billingGroupProperties,omitzero"`
	// []types.Tag on the wire, not a map (serializers.go:1779, aws-sdk-go-v2/service/iot@v1.77.4).
	Tags []tags.KV `json:"tags,omitempty"`
}

func (b *InMemoryBackend) CreateBillingGroup(
	input *CreateBillingGroupInput,
) (*BillingGroup, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.billingGroups.Has(input.BillingGroupName) {
		return nil, fmt.Errorf(
			"billing group %q already exists: %w",
			input.BillingGroupName,
			ErrAlreadyExists,
		)
	}
	bg := &BillingGroup{
		BillingGroupName:       input.BillingGroupName,
		BillingGroupARN:        b.billingGroupARN(input.BillingGroupName),
		BillingGroupID:         uuid.NewString(),
		BillingGroupProperties: input.BillingGroupProperties,
		BillingGroupMetadata: BillingGroupMetadata{
			CreationDate: float64(time.Now().Unix()),
		},
		Tags:    tags.MapFromKV(input.Tags),
		Version: 1,
	}
	b.billingGroups.Put(bg)
	b.putResourceTagsLocked(bg.BillingGroupARN, bg.Tags)

	return cloneBillingGroup(bg), nil
}

func (b *InMemoryBackend) DescribeBillingGroup(name string) (*BillingGroup, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	bg, ok := b.billingGroups.Get(name)
	if !ok {
		return nil, fmt.Errorf("billing group %q not found: %w", name, ErrResourceNotFound)
	}

	return cloneBillingGroup(bg), nil
}

func (b *InMemoryBackend) ListBillingGroups() []*BillingGroup {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*BillingGroup, 0, b.billingGroups.Len())
	for _, v := range b.billingGroups.Snapshot() {
		out = append(out, cloneBillingGroup(v))
	}

	return out
}

func (b *InMemoryBackend) UpdateBillingGroup(
	name string,
	props BillingGroupProperties,
	expectedVersion int64,
) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	bg, ok := b.billingGroups.Get(name)
	if !ok {
		return 0, fmt.Errorf("billing group %q not found: %w", name, ErrResourceNotFound)
	}

	if expectedVersion != 0 && expectedVersion != bg.Version {
		return 0, fmt.Errorf("%w: expected version %d, current version %d",
			ErrVersionConflict, expectedVersion, bg.Version)
	}
	bg.BillingGroupProperties = props
	bg.Version++

	return bg.Version, nil
}

func (b *InMemoryBackend) DeleteBillingGroup(name string, expectedVersion int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	bg, ok := b.billingGroups.Get(name)
	if !ok {
		return fmt.Errorf("billing group %q not found: %w", name, ErrResourceNotFound)
	}

	if expectedVersion != 0 && expectedVersion != bg.Version {
		return fmt.Errorf("%w: expected version %d, current version %d",
			ErrVersionConflict, expectedVersion, bg.Version)
	}
	b.billingGroups.Delete(name)
	delete(b.resourceTags, bg.BillingGroupARN)

	for thingName, group := range b.thingBillingGroups {
		if group == name {
			delete(b.thingBillingGroups, thingName)
		}
	}

	return nil
}
