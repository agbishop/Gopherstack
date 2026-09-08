package iotwireless

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func multicastGroupARN(region, accountID, id string) string {
	return arn.Build("iotwireless", region, accountID, fmt.Sprintf("MulticastGroup/%s", id))
}

func copyMulticastGroup(mg *MulticastGroup) *MulticastGroup {
	cp := *mg
	cp.Tags = make(map[string]string, len(mg.Tags))
	maps.Copy(cp.Tags, mg.Tags)
	cp.LoRaWAN = copyLoRaWANMulticast(mg.LoRaWAN)

	return &cp
}

// CreateMulticastGroup creates a new multicast group.
func (b *InMemoryBackend) CreateMulticastGroup(
	accountID, region, name, description string,
	loRaWAN *LoRaWANMulticast,
	tags map[string]string,
) (*MulticastGroup, error) {
	b.mu.Lock("CreateMulticastGroup")
	defer b.mu.Unlock()

	id := uuid.NewString()
	arn := multicastGroupARN(region, accountID, id)

	mg := &MulticastGroup{
		ID:          id,
		ARN:         arn,
		Name:        name,
		Description: description,
		Status:      "Pending",
		LoRaWAN:     loRaWAN,
		Tags:        newTagsCopy(tags),
		CreatedAt:   time.Now(),
		AccountID:   accountID,
		Region:      region,
	}

	b.multicastGroups.Put(mg)
	b.storeResourceTagsLocked(arn, tags)

	return copyMulticastGroup(mg), nil
}

// GetMulticastGroup returns a multicast group by ID.
func (b *InMemoryBackend) GetMulticastGroup(accountID, region, id string) (*MulticastGroup, error) {
	b.mu.RLock("GetMulticastGroup")
	defer b.mu.RUnlock()

	mg, ok := b.multicastGroups.Get(compositeKey(accountID, region, id))
	if !ok {
		return nil, ErrMulticastGroupNotFound
	}

	return copyMulticastGroup(mg), nil
}

// ListMulticastGroups returns all multicast groups for the given account and region,
// sorted by name for deterministic output.
func (b *InMemoryBackend) ListMulticastGroups(accountID, region string) []*MulticastGroup {
	b.mu.RLock("ListMulticastGroups")
	defer b.mu.RUnlock()

	all := b.multicastGroups.All()
	result := make([]*MulticastGroup, 0, len(all))

	for _, mg := range all {
		if mg.AccountID == accountID && mg.Region == region {
			result = append(result, copyMulticastGroup(mg))
		}
	}

	slices.SortFunc(result, func(a, b *MulticastGroup) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return result
}

// DeleteMulticastGroup deletes a multicast group by ID, cascading the
// cleanup to its wireless-device association set so no ghost entry survives
// the group's deletion. Real AWS refuses this while the group is in use by
// a FUOTA task (api_op_DeleteMulticastGroup.go: "Deletes a multicast group
// if it is not in use by a FUOTA task").
func (b *InMemoryBackend) DeleteMulticastGroup(accountID, region, id string) error {
	b.mu.Lock("DeleteMulticastGroup")
	defer b.mu.Unlock()

	key := compositeKey(accountID, region, id)

	mg, ok := b.multicastGroups.Get(key)
	if !ok {
		return ErrMulticastGroupNotFound
	}

	for _, members := range b.fuotaTaskMulticast {
		if members[id] {
			return ErrMulticastGroupInUse
		}
	}

	delete(b.resourceTags, mg.ARN)
	delete(b.multicastGroupDevices, id)
	b.multicastGroups.Delete(key)

	return nil
}

// UpdateMulticastGroup updates mutable fields on an existing multicast
// group. UpdateMulticastGroupInput.LoRaWAN is the same LoRaWANMulticast
// shape as CreateMulticastGroup's (api_op_UpdateMulticastGroup.go:39), so a
// non-nil value replaces the stored LoRaWAN wholesale rather than merging
// field by field.
func (b *InMemoryBackend) UpdateMulticastGroup(
	accountID, region, id, name, description string,
	loRaWAN *LoRaWANMulticast,
) error {
	b.mu.Lock("UpdateMulticastGroup")
	defer b.mu.Unlock()

	mg, ok := b.multicastGroups.Get(compositeKey(accountID, region, id))
	if !ok {
		return ErrMulticastGroupNotFound
	}

	if name != "" {
		mg.Name = name
	}

	mg.Description = description

	if loRaWAN != nil {
		mg.LoRaWAN = loRaWAN
	}

	return nil
}

// AssociateWirelessDeviceWithMulticastGroup records the association of a
// wireless device with a multicast group. A multicast group can have many
// associated devices, so this adds to a per-group set rather than
// overwriting a single slot.
func (b *InMemoryBackend) AssociateWirelessDeviceWithMulticastGroup(multicastGroupID, wirelessDeviceID string) error {
	b.mu.Lock("AssociateWirelessDeviceWithMulticastGroup")
	defer b.mu.Unlock()

	b.addMulticastGroupDeviceLocked(multicastGroupID, wirelessDeviceID)

	return nil
}

// addMulticastGroupDeviceLocked adds wirelessDeviceID to multicastGroupID's
// association set. Must be called with b.mu held for writing.
func (b *InMemoryBackend) addMulticastGroupDeviceLocked(multicastGroupID, wirelessDeviceID string) {
	if b.multicastGroupDevices[multicastGroupID] == nil {
		b.multicastGroupDevices[multicastGroupID] = make(map[string]bool)
	}

	b.multicastGroupDevices[multicastGroupID][wirelessDeviceID] = true
}

// CancelMulticastGroupSession marks the multicast group session as cancelled.
// If no session is active, the call is a no-op (idempotent).
func (b *InMemoryBackend) CancelMulticastGroupSession(multicastGroupID string) error {
	b.mu.Lock("CancelMulticastGroupSession")
	defer b.mu.Unlock()

	delete(b.multicastGroupSessions, multicastGroupID)
	delete(b.multicastGroupSessionStart, multicastGroupID)

	return nil
}

// DisassociateWirelessDeviceFromMulticastGroup removes a single device from
// a multicast group's association set. wirelessDeviceID is the
// {WirelessDeviceId} path segment of DELETE
// /multicast-groups/{Id}/wireless-devices/{WirelessDeviceId}; an empty value
// (e.g. from a caller that can't recover it) falls back to clearing every
// device associated with the group, preserving the prior behavior.
func (b *InMemoryBackend) DisassociateWirelessDeviceFromMulticastGroup(
	multicastGroupID, wirelessDeviceID string,
) error {
	b.mu.Lock("DisassociateWirelessDeviceFromMulticastGroup")
	defer b.mu.Unlock()

	if wirelessDeviceID == "" {
		delete(b.multicastGroupDevices, multicastGroupID)

		return nil
	}

	delete(b.multicastGroupDevices[multicastGroupID], wirelessDeviceID)

	return nil
}

// ListMulticastGroupDeviceIDs returns the IDs of wireless devices currently
// associated with a multicast group, sorted for deterministic output.
func (b *InMemoryBackend) ListMulticastGroupDeviceIDs(multicastGroupID string) []string {
	b.mu.RLock("ListMulticastGroupDeviceIDs")
	defer b.mu.RUnlock()

	ids := make([]string, 0, len(b.multicastGroupDevices[multicastGroupID]))
	for id := range b.multicastGroupDevices[multicastGroupID] {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	return ids
}

// StartBulkAssociateWirelessDeviceWithMulticastGroup associates every
// wireless device in the account/region with the multicast group. Real AWS
// filters candidates by the request's QueryString search expression against
// device attributes; this backend has no query-expression evaluator, so it
// emulates the "all qualifying devices" semantics by associating the full
// device corpus, matching this package's existing convention for List* ops
// that accept-but-can't-honor a narrowing filter (see PARITY.md gaps).
func (b *InMemoryBackend) StartBulkAssociateWirelessDeviceWithMulticastGroup(
	accountID, region, multicastGroupID string,
) error {
	b.mu.Lock("StartBulkAssociateWirelessDeviceWithMulticastGroup")
	defer b.mu.Unlock()

	for _, d := range b.devices.All() {
		if d.AccountID == accountID && d.Region == region {
			b.addMulticastGroupDeviceLocked(multicastGroupID, d.ID)
		}
	}

	return nil
}

// StartBulkDisassociateWirelessDeviceFromMulticastGroup disassociates every
// wireless device in the account/region from the multicast group -- the
// bulk-disassociate counterpart of StartBulkAssociateWirelessDeviceWithMulticastGroup.
func (b *InMemoryBackend) StartBulkDisassociateWirelessDeviceFromMulticastGroup(multicastGroupID string) error {
	b.mu.Lock("StartBulkDisassociateWirelessDeviceFromMulticastGroup")
	defer b.mu.Unlock()

	delete(b.multicastGroupDevices, multicastGroupID)

	return nil
}

// StartMulticastGroupSession marks a multicast group session as active,
// recording its start time so GetMulticastGroupSession can report it back.
func (b *InMemoryBackend) StartMulticastGroupSession(multicastGroupID string) error {
	b.mu.Lock("StartMulticastGroupSession")
	defer b.mu.Unlock()

	b.multicastGroupSessions[multicastGroupID] = true
	b.multicastGroupSessionStart[multicastGroupID] = time.Now().UTC()

	return nil
}

// GetMulticastGroupSession returns the start time of a multicast group's active
// session. Returns ErrMulticastGroupSessionNotFound if no session has been
// started (or it has since been cancelled), matching real AWS's
// ResourceNotFoundException for a group with no active session.
func (b *InMemoryBackend) GetMulticastGroupSession(multicastGroupID string) (time.Time, error) {
	b.mu.RLock("GetMulticastGroupSession")
	defer b.mu.RUnlock()

	if !b.multicastGroupSessions[multicastGroupID] {
		return time.Time{}, ErrMulticastGroupSessionNotFound
	}

	return b.multicastGroupSessionStart[multicastGroupID], nil
}
