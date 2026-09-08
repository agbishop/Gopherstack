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

// Default gateway connectivity/firmware state recorded at CreateWirelessGateway
// time, returned by GetWirelessGatewayStatistics / GetWirelessGatewayFirmwareInformation
// once a gateway is found to exist.
const (
	defaultGatewayConnectionStatus = "Connected"
	defaultGatewayFirmwareVersion  = "1.0.0"
	defaultGatewayFirmwareModel    = "GW-001"
	defaultGatewayFirmwareStation  = "LNS"
)

func wirelessGatewayARN(region, accountID, id string) string {
	return arn.Build("iotwireless", region, accountID, fmt.Sprintf("WirelessGateway/%s", id))
}

// copyWirelessGateway returns a shallow copy of gw with independent Tags and
// LoRaWAN.
func copyWirelessGateway(gw *WirelessGateway) *WirelessGateway {
	cp := *gw
	cp.Tags = make(map[string]string, len(gw.Tags))
	maps.Copy(cp.Tags, gw.Tags)
	cp.LoRaWAN = copyLoRaWANGateway(gw.LoRaWAN)

	return &cp
}

// CreateWirelessGateway creates a new wireless gateway.
func (b *InMemoryBackend) CreateWirelessGateway(
	accountID, region, name, description string,
	loRaWAN *LoRaWANGateway,
	tags map[string]string,
) (*WirelessGateway, error) {
	b.mu.Lock("CreateWirelessGateway")
	defer b.mu.Unlock()

	id := uuid.NewString()
	arn := wirelessGatewayARN(region, accountID, id)

	gw := &WirelessGateway{
		ID:               id,
		ARN:              arn,
		Name:             name,
		Description:      description,
		LoRaWAN:          loRaWAN,
		Tags:             newTagsCopy(tags),
		CreatedAt:        time.Now(),
		ConnectionStatus: defaultGatewayConnectionStatus,
		FirmwareVersion:  defaultGatewayFirmwareVersion,
		FirmwareModel:    defaultGatewayFirmwareModel,
		FirmwareStation:  defaultGatewayFirmwareStation,
		AccountID:        accountID,
		Region:           region,
	}

	b.gateways.Put(gw)
	b.storeResourceTagsLocked(arn, tags)

	return copyWirelessGateway(gw), nil
}

// GetWirelessGateway returns a wireless gateway by ID.
func (b *InMemoryBackend) GetWirelessGateway(accountID, region, id string) (*WirelessGateway, error) {
	b.mu.RLock("GetWirelessGateway")
	defer b.mu.RUnlock()

	gw, ok := b.gateways.Get(compositeKey(accountID, region, id))
	if !ok {
		return nil, ErrGatewayNotFound
	}

	return copyWirelessGateway(gw), nil
}

// ListWirelessGateways returns all wireless gateways for the given account and region,
// sorted by name for deterministic output.
func (b *InMemoryBackend) ListWirelessGateways(accountID, region string) []*WirelessGateway {
	b.mu.RLock("ListWirelessGateways")
	defer b.mu.RUnlock()

	all := b.gateways.All()
	result := make([]*WirelessGateway, 0, len(all))

	for _, gw := range all {
		if gw.AccountID == accountID && gw.Region == region {
			result = append(result, copyWirelessGateway(gw))
		}
	}

	slices.SortFunc(result, func(a, b *WirelessGateway) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return result
}

// DeleteWirelessGateway deletes a wireless gateway.
func (b *InMemoryBackend) DeleteWirelessGateway(accountID, region, id string) error {
	b.mu.Lock("DeleteWirelessGateway")
	defer b.mu.Unlock()

	key := compositeKey(accountID, region, id)

	gw, ok := b.gateways.Get(key)
	if !ok {
		return ErrGatewayNotFound
	}

	delete(b.resourceTags, gw.ARN)
	delete(b.wirelessGatewayThings, id)
	delete(b.wirelessGatewayCerts, id)
	delete(b.positions, id)
	b.gatewayTasks.Delete(id)
	b.gateways.Delete(key)

	return nil
}

// UpdateWirelessGateway updates mutable fields on an existing wireless
// gateway. UpdateWirelessGatewayInput carries JoinEuiFilters/MaxEirp/
// NetIDFilters as top-level fields (not nested under a LoRaWAN sub-object,
// unlike Create -- api_op_UpdateWirelessGateway.go:27), so they merge
// field-by-field into the stored LoRaWANGateway; a nil/empty value leaves
// the existing field untouched.
func (b *InMemoryBackend) UpdateWirelessGateway(
	accountID, region, id, name, description string,
	joinEuiFilters [][]string, netIDFilters []string, maxEirp *float32,
) error {
	b.mu.Lock("UpdateWirelessGateway")
	defer b.mu.Unlock()

	gw, ok := b.gateways.Get(compositeKey(accountID, region, id))
	if !ok {
		return ErrGatewayNotFound
	}

	if name != "" {
		gw.Name = name
	}

	gw.Description = description

	if joinEuiFilters == nil && netIDFilters == nil && maxEirp == nil {
		return nil
	}

	if gw.LoRaWAN == nil {
		gw.LoRaWAN = &LoRaWANGateway{}
	}

	if joinEuiFilters != nil {
		gw.LoRaWAN.JoinEuiFilters = joinEuiFilters
	}

	if netIDFilters != nil {
		gw.LoRaWAN.NetIDFilters = netIDFilters
	}

	if maxEirp != nil {
		gw.LoRaWAN.MaxEirp = maxEirp
	}

	return nil
}

// AssociateWirelessGatewayWithThing associates a wireless gateway with an IoT Thing.
// Returns ErrGatewayNotFound when the gateway does not exist.
func (b *InMemoryBackend) AssociateWirelessGatewayWithThing(
	accountID, region, gatewayID, thingArn string,
) error {
	b.mu.Lock("AssociateWirelessGatewayWithThing")
	defer b.mu.Unlock()

	if !b.gateways.Has(compositeKey(accountID, region, gatewayID)) {
		return ErrGatewayNotFound
	}

	b.wirelessGatewayThings[gatewayID] = thingArn

	return nil
}

// GetWirelessGatewayThingArn returns the ARN of the IoT Thing associated with
// a wireless gateway, or "" if AssociateWirelessGatewayWithThing was never
// called (or the association was since cleared).
func (b *InMemoryBackend) GetWirelessGatewayThingArn(gatewayID string) string {
	b.mu.RLock("GetWirelessGatewayThingArn")
	defer b.mu.RUnlock()

	return b.wirelessGatewayThings[gatewayID]
}

// AddWirelessGatewayInternal inserts a WirelessGateway directly into the backend, bypassing ID generation.
// Intended for test setup only.
func (b *InMemoryBackend) AddWirelessGatewayInternal(accountID, region string, gw *WirelessGateway) {
	b.mu.Lock("AddWirelessGatewayInternal")
	defer b.mu.Unlock()

	cp := copyWirelessGateway(gw)
	cp.AccountID = accountID
	cp.Region = region
	b.gateways.Put(cp)
	b.storeResourceTagsLocked(gw.ARN, gw.Tags)
}

// DisassociateWirelessGatewayFromThing clears the thing association for a gateway.
func (b *InMemoryBackend) DisassociateWirelessGatewayFromThing(
	accountID, region, gatewayID string,
) error {
	b.mu.Lock("DisassociateWirelessGatewayFromThing")
	defer b.mu.Unlock()

	if !b.gateways.Has(compositeKey(accountID, region, gatewayID)) {
		return ErrGatewayNotFound
	}

	delete(b.wirelessGatewayThings, gatewayID)

	return nil
}
