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

func wirelessDeviceARN(region, accountID, id string) string {
	return arn.Build("iotwireless", region, accountID, fmt.Sprintf("WirelessDevice/%s", id))
}

// copyWirelessDevice returns a shallow copy of d with independent Tags,
// LoRaWAN, and Sidewalk.
func copyWirelessDevice(d *WirelessDevice) *WirelessDevice {
	cp := *d
	cp.Tags = make(map[string]string, len(d.Tags))
	maps.Copy(cp.Tags, d.Tags)
	cp.LoRaWAN = copyLoRaWANDevice(d.LoRaWAN)
	cp.Sidewalk = copySidewalkDevice(d.Sidewalk)

	return &cp
}

// CreateWirelessDevice creates a new wireless device.
func (b *InMemoryBackend) CreateWirelessDevice(
	accountID, region, name, devType, destinationName, description, positioning string,
	loRaWAN *LoRaWANDevice, sidewalk *SidewalkCreateWirelessDevice,
	tags map[string]string,
) (*WirelessDevice, error) {
	b.mu.Lock("CreateWirelessDevice")
	defer b.mu.Unlock()

	id := uuid.NewString()
	arn := wirelessDeviceARN(region, accountID, id)

	d := &WirelessDevice{
		ID:              id,
		ARN:             arn,
		Name:            name,
		Type:            devType,
		DestinationName: destinationName,
		Description:     description,
		Positioning:     positioning,
		LoRaWAN:         loRaWAN,
		Sidewalk:        sidewalkDeviceFromCreate(sidewalk),
		Tags:            newTagsCopy(tags),
		CreatedAt:       time.Now(),
		AccountID:       accountID,
		Region:          region,
	}

	b.devices.Put(d)
	b.storeResourceTagsLocked(arn, tags)

	return copyWirelessDevice(d), nil
}

// GetWirelessDevice returns a wireless device by ID.
func (b *InMemoryBackend) GetWirelessDevice(accountID, region, id string) (*WirelessDevice, error) {
	b.mu.RLock("GetWirelessDevice")
	defer b.mu.RUnlock()

	d, ok := b.devices.Get(compositeKey(accountID, region, id))
	if !ok {
		return nil, ErrDeviceNotFound
	}

	return copyWirelessDevice(d), nil
}

// ListWirelessDevicesFilter narrows ListWirelessDevices' result set. Each
// non-empty field is a query-string filter on ListWirelessDevicesInput
// (destinationName/deviceProfileId/serviceProfileId/fuotaTaskId/
// multicastGroupId/wirelessDeviceType -- api_op_ListWirelessDevices.go:29-56,
// serializers.go:6439). Multiple set filters combine with AND: this isn't
// documented in the SDK model (the client just forwards whatever query
// params are set), so this is a chosen semantic, matching how every other
// narrowing List filter in this codebase intersects rather than unions.
type ListWirelessDevicesFilter struct {
	DestinationName    string
	DeviceProfileID    string
	ServiceProfileID   string
	FuotaTaskID        string
	MulticastGroupID   string
	WirelessDeviceType string
}

// matches reports whether d satisfies every non-empty filter field. An empty
// filter field means "not requested" and never excludes a device -- matching
// ListWirelessDevicesInput's *string/enum fields being nil (query param
// absent) when the client omits them.
func (f ListWirelessDevicesFilter) matches(b *InMemoryBackend, d *WirelessDevice) bool {
	if f.DestinationName != "" && d.DestinationName != f.DestinationName {
		return false
	}

	if f.WirelessDeviceType != "" && d.Type != f.WirelessDeviceType {
		return false
	}

	if f.DeviceProfileID != "" && !hasDeviceProfileID(d, f.DeviceProfileID) {
		return false
	}

	if f.ServiceProfileID != "" &&
		(d.LoRaWAN == nil || d.LoRaWAN.ServiceProfileID == nil || *d.LoRaWAN.ServiceProfileID != f.ServiceProfileID) {
		return false
	}

	if f.FuotaTaskID != "" && !b.fuotaTaskDevices[f.FuotaTaskID][d.ID] {
		return false
	}

	if f.MulticastGroupID != "" && !b.multicastGroupDevices[f.MulticastGroupID][d.ID] {
		return false
	}

	return true
}

// hasDeviceProfileID checks LoRaWAN.DeviceProfileID for LoRaWAN devices and
// Sidewalk.DeviceProfileID for Sidewalk devices -- real AWS's DeviceProfileID
// filter applies to whichever wireless technology the device uses.
func hasDeviceProfileID(d *WirelessDevice, id string) bool {
	if d.LoRaWAN != nil && d.LoRaWAN.DeviceProfileID != nil && *d.LoRaWAN.DeviceProfileID == id {
		return true
	}

	if d.Sidewalk != nil && d.Sidewalk.DeviceProfileID != nil && *d.Sidewalk.DeviceProfileID == id {
		return true
	}

	return false
}

// ListWirelessDevices returns wireless devices for the given account and
// region matching filter, sorted by name for deterministic output.
func (b *InMemoryBackend) ListWirelessDevices(
	accountID, region string, filter ListWirelessDevicesFilter,
) []*WirelessDevice {
	b.mu.RLock("ListWirelessDevices")
	defer b.mu.RUnlock()

	all := b.devices.All()
	result := make([]*WirelessDevice, 0, len(all))

	for _, d := range all {
		if d.AccountID == accountID && d.Region == region && filter.matches(b, d) {
			result = append(result, copyWirelessDevice(d))
		}
	}

	slices.SortFunc(result, func(a, b *WirelessDevice) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return result
}

// DeleteWirelessDevice deletes a wireless device.
func (b *InMemoryBackend) DeleteWirelessDevice(accountID, region, id string) error {
	b.mu.Lock("DeleteWirelessDevice")
	defer b.mu.Unlock()

	key := compositeKey(accountID, region, id)

	d, ok := b.devices.Get(key)
	if !ok {
		return ErrDeviceNotFound
	}

	delete(b.resourceTags, d.ARN)
	delete(b.wirelessDeviceThings, id)
	delete(b.queuedMessages, id)
	delete(b.positions, id)

	for _, members := range b.multicastGroupDevices {
		delete(members, id)
	}

	for _, members := range b.fuotaTaskDevices {
		delete(members, id)
	}

	b.devices.Delete(key)

	return nil
}

// AssociateWirelessDeviceWithThing associates a wireless device with an IoT Thing.
// Returns ErrDeviceNotFound when the device does not exist.
func (b *InMemoryBackend) AssociateWirelessDeviceWithThing(
	accountID, region, wirelessDeviceID, thingArn string,
) error {
	b.mu.Lock("AssociateWirelessDeviceWithThing")
	defer b.mu.Unlock()

	if !b.devices.Has(compositeKey(accountID, region, wirelessDeviceID)) {
		return ErrDeviceNotFound
	}

	b.wirelessDeviceThings[wirelessDeviceID] = thingArn

	return nil
}

// GetWirelessDeviceThingArn returns the ARN of the IoT Thing associated with
// a wireless device, or "" if AssociateWirelessDeviceWithThing was never
// called (or the association was since cleared).
func (b *InMemoryBackend) GetWirelessDeviceThingArn(wirelessDeviceID string) string {
	b.mu.RLock("GetWirelessDeviceThingArn")
	defer b.mu.RUnlock()

	return b.wirelessDeviceThings[wirelessDeviceID]
}

// AddWirelessDeviceInternal inserts a WirelessDevice directly into the backend, bypassing ID generation.
// Intended for test setup only.
func (b *InMemoryBackend) AddWirelessDeviceInternal(accountID, region string, d *WirelessDevice) {
	b.mu.Lock("AddWirelessDeviceInternal")
	defer b.mu.Unlock()

	cp := copyWirelessDevice(d)
	cp.AccountID = accountID
	cp.Region = region
	b.devices.Put(cp)
	b.storeResourceTagsLocked(d.ARN, d.Tags)
}

// UpdateWirelessDevice updates mutable fields on an existing wireless device.
// loRaWAN/sidewalk are merged field-by-field into the stored configuration
// (rather than wholesale replaced), matching real AWS's UpdateWirelessDevice
// semantics of updating only the LoRaWANUpdateDevice/SidewalkUpdateWirelessDevice
// sub-fields the client actually supplied (e.g. LoRaWANUpdateDevice only
// carries DeviceProfileID/ServiceProfileID/ABP/FPorts -- not DevEui -- so a
// full replace would silently drop DevEui that CreateWirelessDevice
// originally stored).
func (b *InMemoryBackend) UpdateWirelessDevice(
	accountID, region, id, name, description, destinationName, positioning string,
	loRaWAN *LoRaWANUpdateDevice, sidewalk *SidewalkUpdateWirelessDevice,
) error {
	b.mu.Lock("UpdateWirelessDevice")
	defer b.mu.Unlock()

	d, ok := b.devices.Get(compositeKey(accountID, region, id))
	if !ok {
		return ErrDeviceNotFound
	}

	if name != "" {
		d.Name = name
	}

	d.Description = description

	if destinationName != "" {
		d.DestinationName = destinationName
	}

	if positioning != "" {
		d.Positioning = positioning
	}

	d.LoRaWAN = mergeLoRaWANDeviceUpdate(d.LoRaWAN, loRaWAN)
	d.Sidewalk = mergeSidewalkDeviceUpdate(d.Sidewalk, sidewalk)

	return nil
}

// DisassociateWirelessDeviceFromThing clears the thing association for a wireless device.
func (b *InMemoryBackend) DisassociateWirelessDeviceFromThing(
	accountID, region, wirelessDeviceID string,
) error {
	b.mu.Lock("DisassociateWirelessDeviceFromThing")
	defer b.mu.Unlock()

	if !b.devices.Has(compositeKey(accountID, region, wirelessDeviceID)) {
		return ErrDeviceNotFound
	}

	delete(b.wirelessDeviceThings, wirelessDeviceID)

	return nil
}

// --- Queued messages operations ---

// ListQueuedMessages returns queued messages for a wireless device.
func (b *InMemoryBackend) ListQueuedMessages(wirelessDeviceID string) []QueuedMessage {
	b.mu.RLock("ListQueuedMessages")
	defer b.mu.RUnlock()

	msgs, ok := b.queuedMessages[wirelessDeviceID]
	if !ok {
		return []QueuedMessage{}
	}

	result := make([]QueuedMessage, len(msgs))
	copy(result, msgs)

	return result
}

// DeleteQueuedMessages clears the message queue for a wireless device.
func (b *InMemoryBackend) DeleteQueuedMessages(wirelessDeviceID string) error {
	b.mu.Lock("DeleteQueuedMessages")
	defer b.mu.Unlock()

	delete(b.queuedMessages, wirelessDeviceID)

	return nil
}

// EnqueueMessage appends a downlink message to a wireless device's message
// queue, so that a subsequent ListQueuedMessages reflects messages sent via
// SendDataToWirelessDevice.
func (b *InMemoryBackend) EnqueueMessage(wirelessDeviceID string, msg QueuedMessage) {
	b.mu.Lock("EnqueueMessage")
	defer b.mu.Unlock()

	b.queuedMessages[wirelessDeviceID] = append(b.queuedMessages[wirelessDeviceID], msg)
}

// QueuedMessage represents a downlink message queued for a wireless device.
type QueuedMessage struct {
	ReceivedAt    time.Time
	MessageID     string
	PayloadBase64 string
	TransmitMode  int32
}
