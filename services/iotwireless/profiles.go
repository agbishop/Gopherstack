package iotwireless

// Device profiles and service profiles are both simple flat resources with
// no nested wire structure: same fields, same CRUD shape. Kept in one file
// (rather than two near-identical files) per project convention: a `dupl`
// finding from splitting near-identical logic into separate files means the
// ops belong grouped in one cohesive file instead.

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func deviceProfileARN(region, accountID, id string) string {
	return arn.Build("iotwireless", region, accountID, fmt.Sprintf("DeviceProfile/%s", id))
}

// copyDeviceProfile returns a shallow copy of dp with independent Tags,
// LoRaWAN, and Sidewalk.
func copyDeviceProfile(dp *DeviceProfile) *DeviceProfile {
	cp := *dp
	cp.Tags = make(map[string]string, len(dp.Tags))
	maps.Copy(cp.Tags, dp.Tags)
	cp.LoRaWAN = copyLoRaWANDeviceProfile(dp.LoRaWAN)
	cp.Sidewalk = copySidewalkGetDeviceProfile(dp.Sidewalk)

	return &cp
}

// CreateDeviceProfile creates a new device profile. sidewalk being non-nil
// (even if empty -- SidewalkCreateDeviceProfile has no fields of its own)
// distinguishes a request that asked for a Sidewalk profile from one that
// didn't -- see sidewalkGetDeviceProfileFromCreate.
func (b *InMemoryBackend) CreateDeviceProfile(
	accountID, region, name string,
	loRaWAN *LoRaWANDeviceProfile,
	sidewalk *SidewalkCreateDeviceProfile,
	tags map[string]string,
) (*DeviceProfile, error) {
	b.mu.Lock("CreateDeviceProfile")
	defer b.mu.Unlock()

	id := uuid.NewString()
	arn := deviceProfileARN(region, accountID, id)

	dp := &DeviceProfile{
		ID:        id,
		ARN:       arn,
		Name:      name,
		LoRaWAN:   loRaWAN,
		Sidewalk:  sidewalkGetDeviceProfileFromCreate(sidewalk != nil),
		Tags:      newTagsCopy(tags),
		CreatedAt: time.Now(),
		AccountID: accountID,
		Region:    region,
	}

	b.deviceProfiles.Put(dp)
	b.storeResourceTagsLocked(arn, tags)

	return copyDeviceProfile(dp), nil
}

// GetDeviceProfile returns a device profile by ID.
func (b *InMemoryBackend) GetDeviceProfile(accountID, region, id string) (*DeviceProfile, error) {
	b.mu.RLock("GetDeviceProfile")
	defer b.mu.RUnlock()

	dp, ok := b.deviceProfiles.Get(compositeKey(accountID, region, id))
	if !ok {
		return nil, ErrDeviceProfileNotFound
	}

	return copyDeviceProfile(dp), nil
}

// ListDeviceProfiles returns all device profiles for the given account and
// region, sorted by name for deterministic output. deviceProfileType, if
// non-empty, filters to "LoRaWAN" or "Sidewalk" profiles (types.go's
// ListDeviceProfilesInput.DeviceProfileType), determined by which of the
// profile's LoRaWAN/Sidewalk sub-objects is set -- a profile always has
// exactly one, never both, since CreateDeviceProfile accepts only one.
func (b *InMemoryBackend) ListDeviceProfiles(accountID, region, deviceProfileType string) []*DeviceProfile {
	b.mu.RLock("ListDeviceProfiles")
	defer b.mu.RUnlock()

	all := b.deviceProfiles.All()
	result := make([]*DeviceProfile, 0, len(all))

	for _, dp := range all {
		if dp.AccountID != accountID || dp.Region != region {
			continue
		}

		if deviceProfileType != "" && !deviceProfileMatchesType(dp, deviceProfileType) {
			continue
		}

		result = append(result, copyDeviceProfile(dp))
	}

	slices.SortFunc(result, func(a, b *DeviceProfile) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return result
}

// deviceProfileMatchesType reports whether dp is of the given
// types.DeviceProfileType ("LoRaWAN" or "Sidewalk").
func deviceProfileMatchesType(dp *DeviceProfile, deviceProfileType string) bool {
	switch deviceProfileType {
	case "LoRaWAN":
		return dp.LoRaWAN != nil
	case "Sidewalk":
		return dp.Sidewalk != nil
	default:
		return false
	}
}

// DeleteDeviceProfile deletes a device profile by ID. Real AWS models
// ConflictException on this op; the only referrer a device profile has is a
// wireless device's LoRaWAN.DeviceProfileID/Sidewalk.DeviceProfileID, so
// deletion is refused while any device in the account/region still
// references it.
func (b *InMemoryBackend) DeleteDeviceProfile(accountID, region, id string) error {
	b.mu.Lock("DeleteDeviceProfile")
	defer b.mu.Unlock()

	key := compositeKey(accountID, region, id)

	dp, ok := b.deviceProfiles.Get(key)
	if !ok {
		return ErrDeviceProfileNotFound
	}

	for _, d := range b.devices.All() {
		if d.AccountID == accountID && d.Region == region && hasDeviceProfileID(d, id) {
			return ErrDeviceProfileInUse
		}
	}

	delete(b.resourceTags, dp.ARN)
	b.deviceProfiles.Delete(key)

	return nil
}

// AddDeviceProfileInternal inserts a DeviceProfile directly into the backend, bypassing ID generation.
// Intended for test setup only.
func (b *InMemoryBackend) AddDeviceProfileInternal(accountID, region string, dp *DeviceProfile) {
	b.mu.Lock("AddDeviceProfileInternal")
	defer b.mu.Unlock()

	cp := copyDeviceProfile(dp)
	cp.AccountID = accountID
	cp.Region = region
	b.deviceProfiles.Put(cp)
	b.storeResourceTagsLocked(dp.ARN, dp.Tags)
}

func serviceProfileARN(region, accountID, id string) string {
	return arn.Build("iotwireless", region, accountID, fmt.Sprintf("ServiceProfile/%s", id))
}

// copyServiceProfile returns a shallow copy of sp with independent Tags and
// LoRaWAN.
func copyServiceProfile(sp *ServiceProfile) *ServiceProfile {
	cp := *sp
	cp.Tags = make(map[string]string, len(sp.Tags))
	maps.Copy(cp.Tags, sp.Tags)
	cp.LoRaWAN = copyLoRaWANServiceProfile(sp.LoRaWAN)

	return &cp
}

// CreateServiceProfile creates a new service profile.
func (b *InMemoryBackend) CreateServiceProfile(
	accountID, region, name string,
	loRaWAN *LoRaWANServiceProfile,
	tags map[string]string,
) (*ServiceProfile, error) {
	b.mu.Lock("CreateServiceProfile")
	defer b.mu.Unlock()

	id := uuid.NewString()
	arn := serviceProfileARN(region, accountID, id)

	sp := &ServiceProfile{
		ID:        id,
		ARN:       arn,
		Name:      name,
		LoRaWAN:   loRaWAN,
		Tags:      newTagsCopy(tags),
		CreatedAt: time.Now(),
		AccountID: accountID,
		Region:    region,
	}

	b.serviceProfiles.Put(sp)
	b.storeResourceTagsLocked(arn, tags)

	return copyServiceProfile(sp), nil
}

// GetServiceProfile returns a service profile by ID.
func (b *InMemoryBackend) GetServiceProfile(accountID, region, id string) (*ServiceProfile, error) {
	b.mu.RLock("GetServiceProfile")
	defer b.mu.RUnlock()

	sp, ok := b.serviceProfiles.Get(compositeKey(accountID, region, id))
	if !ok {
		return nil, ErrServiceProfileNotFound
	}

	return copyServiceProfile(sp), nil
}

// ListServiceProfiles returns all service profiles for the given account and region,
// sorted by name for deterministic output.
func (b *InMemoryBackend) ListServiceProfiles(accountID, region string) []*ServiceProfile {
	b.mu.RLock("ListServiceProfiles")
	defer b.mu.RUnlock()

	all := b.serviceProfiles.All()
	result := make([]*ServiceProfile, 0, len(all))

	for _, sp := range all {
		if sp.AccountID == accountID && sp.Region == region {
			result = append(result, copyServiceProfile(sp))
		}
	}

	slices.SortFunc(result, func(a, b *ServiceProfile) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return result
}

// DeleteServiceProfile deletes a service profile. Real AWS models
// ConflictException on this op; the only referrer a service profile has is
// a LoRaWAN wireless device's LoRaWAN.ServiceProfileID, so deletion is
// refused while any device in the account/region still references it.
func (b *InMemoryBackend) DeleteServiceProfile(accountID, region, id string) error {
	b.mu.Lock("DeleteServiceProfile")
	defer b.mu.Unlock()

	key := compositeKey(accountID, region, id)

	sp, ok := b.serviceProfiles.Get(key)
	if !ok {
		return ErrServiceProfileNotFound
	}

	for _, d := range b.devices.All() {
		if d.AccountID == accountID && d.Region == region &&
			d.LoRaWAN != nil && d.LoRaWAN.ServiceProfileID != nil && *d.LoRaWAN.ServiceProfileID == id {
			return ErrServiceProfileInUse
		}
	}

	delete(b.resourceTags, sp.ARN)
	b.serviceProfiles.Delete(key)

	return nil
}

// AddServiceProfileInternal inserts a ServiceProfile directly into the backend, bypassing ID generation.
// Intended for test setup only.
func (b *InMemoryBackend) AddServiceProfileInternal(accountID, region string, sp *ServiceProfile) {
	b.mu.Lock("AddServiceProfileInternal")
	defer b.mu.Unlock()

	cp := copyServiceProfile(sp)
	cp.AccountID = accountID
	cp.Region = region
	b.serviceProfiles.Put(cp)
	b.storeResourceTagsLocked(sp.ARN, sp.Tags)
}
