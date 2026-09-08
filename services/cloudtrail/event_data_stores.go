package cloudtrail

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// CreateEventDataStore creates a new CloudTrail event data store.
func (b *InMemoryBackend) CreateEventDataStore(
	name string,
	multiRegionEnabled, organizationEnabled, terminationProtected bool,
	retentionPeriod int32,
	advancedEventSelectors []AdvancedEventSelector,
	billingMode, kmsKeyID string,
	kv map[string]string,
) (*EventDataStore, error) {
	b.mu.Lock("CreateEventDataStore")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidation)
	}
	if matches := b.edsByName.Get(name); len(matches) > 0 {
		return nil, fmt.Errorf("%w: event data store %s already exists", ErrEventDataStoreAlreadyExists, name)
	}

	b.edsCounter++
	id := fmt.Sprintf("eds-%06d", b.edsCounter)
	edsARN := arn.Build("cloudtrail", b.region, b.accountID, "eventdatastore/"+id)
	t := tags.New("cloudtrail.eds." + id + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}
	if billingMode == "" {
		billingMode = "EXTENDABLE_RETENTION_PRICING"
	}
	if retentionPeriod == 0 {
		retentionPeriod = 2557
	}
	now := time.Now().UTC()
	eds := &EventDataStore{
		EventDataStoreID:       id,
		EventDataStoreARN:      edsARN,
		Name:                   name,
		Status:                 statusEnabled,
		MultiRegionEnabled:     multiRegionEnabled,
		OrganizationEnabled:    organizationEnabled,
		TerminationProtected:   terminationProtected,
		RetentionPeriod:        retentionPeriod,
		AdvancedEventSelectors: copyAdvancedEventSelectors(advancedEventSelectors),
		BillingMode:            billingMode,
		KMSKeyID:               kmsKeyID,
		FederationStatus:       "DISABLED",
		CreatedTimestamp:       now,
		UpdatedTimestamp:       now,
		Tags:                   t,
	}
	b.eventDataStores.Put(eds)

	cp := *eds
	cp.AdvancedEventSelectors = copyAdvancedEventSelectors(eds.AdvancedEventSelectors)

	return &cp, nil
}

// DeleteEventDataStore deletes an event data store by ID or ARN.
// Returns ErrTerminationProtected if termination protection is enabled.
func (b *InMemoryBackend) DeleteEventDataStore(edsIDOrARN string) error {
	b.mu.Lock("DeleteEventDataStore")
	defer b.mu.Unlock()

	eds := b.findEventDataStoreLocked(edsIDOrARN)
	if eds == nil {
		return fmt.Errorf("%w: event data store %s not found", ErrEventDataStoreNotFound, edsIDOrARN)
	}
	if eds.TerminationProtected {
		return fmt.Errorf(
			"%w: event data store %s has termination protection enabled",
			ErrTerminationProtected,
			edsIDOrARN,
		)
	}

	eds.Tags.Close()
	b.eventDataStores.Delete(eds.EventDataStoreID)
	delete(b.eventConfigs, eds.EventDataStoreARN)
	b.resourcePolicies.Delete(eds.EventDataStoreARN)

	return nil
}

// GetEventDataStore returns an event data store by ID or ARN.
func (b *InMemoryBackend) GetEventDataStore(edsIDOrARN string) (*EventDataStore, error) {
	b.mu.RLock("GetEventDataStore")
	defer b.mu.RUnlock()

	eds := b.findEventDataStoreLocked(edsIDOrARN)
	if eds == nil {
		return nil, fmt.Errorf("%w: event data store %s not found", ErrEventDataStoreNotFound, edsIDOrARN)
	}
	cp := *eds

	return &cp, nil
}

// UpdateEventDataStore updates an existing event data store.
func (b *InMemoryBackend) UpdateEventDataStore(
	edsIDOrARN string,
	name string,
	multiRegionEnabled, organizationEnabled, terminationProtected *bool,
	retentionPeriod *int32,
	advancedEventSelectors []AdvancedEventSelector,
	billingMode, kmsKeyID string,
) (*EventDataStore, error) {
	b.mu.Lock("UpdateEventDataStore")
	defer b.mu.Unlock()

	eds := b.findEventDataStoreLocked(edsIDOrARN)
	if eds == nil {
		return nil, fmt.Errorf("%w: event data store %s not found", ErrEventDataStoreNotFound, edsIDOrARN)
	}
	if name != "" && name != eds.Name {
		// eds.Name is an indexed field (edsByName): delete before mutating so
		// the old index entry is removed using the pre-mutation value, then
		// re-Put to rebuild every index (byARN, byName) under the new state.
		b.eventDataStores.Delete(eds.EventDataStoreID)
		eds.Name = name
		b.eventDataStores.Put(eds)
	}
	if multiRegionEnabled != nil {
		eds.MultiRegionEnabled = *multiRegionEnabled
	}
	if organizationEnabled != nil {
		eds.OrganizationEnabled = *organizationEnabled
	}
	if terminationProtected != nil {
		eds.TerminationProtected = *terminationProtected
	}
	if retentionPeriod != nil {
		eds.RetentionPeriod = *retentionPeriod
	}
	if advancedEventSelectors != nil {
		eds.AdvancedEventSelectors = copyAdvancedEventSelectors(advancedEventSelectors)
	}
	if billingMode != "" {
		eds.BillingMode = billingMode
	}
	if kmsKeyID != "" {
		eds.KMSKeyID = kmsKeyID
	}
	eds.UpdatedTimestamp = time.Now().UTC()
	cp := *eds
	cp.AdvancedEventSelectors = copyAdvancedEventSelectors(eds.AdvancedEventSelectors)

	return &cp, nil
}

// ListEventDataStores returns all event data stores.
func (b *InMemoryBackend) ListEventDataStores() []*EventDataStore {
	b.mu.RLock("ListEventDataStores")
	defer b.mu.RUnlock()

	all := b.eventDataStores.All()
	list := make([]*EventDataStore, 0, len(all))
	for _, eds := range all {
		cp := *eds
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].EventDataStoreARN < list[j].EventDataStoreARN })

	return list
}

// RestoreEventDataStore restores a deleted event data store (sets status to ENABLED).
func (b *InMemoryBackend) RestoreEventDataStore(edsIDOrARN string) (*EventDataStore, error) {
	b.mu.Lock("RestoreEventDataStore")
	defer b.mu.Unlock()

	eds := b.findEventDataStoreLocked(edsIDOrARN)
	if eds == nil {
		return nil, fmt.Errorf("%w: event data store %s not found", ErrEventDataStoreNotFound, edsIDOrARN)
	}
	eds.Status = statusEnabled
	eds.UpdatedTimestamp = time.Now().UTC()
	cp := *eds

	return &cp, nil
}

// StartEventDataStoreIngestion starts ingestion for an event data store.
func (b *InMemoryBackend) StartEventDataStoreIngestion(edsIDOrARN string) error {
	b.mu.Lock("StartEventDataStoreIngestion")
	defer b.mu.Unlock()

	eds := b.findEventDataStoreLocked(edsIDOrARN)
	if eds == nil {
		return fmt.Errorf("%w: event data store %s not found", ErrEventDataStoreNotFound, edsIDOrARN)
	}
	eds.Status = statusEnabled
	eds.UpdatedTimestamp = time.Now().UTC()

	return nil
}

// StopEventDataStoreIngestion stops ingestion for an event data store.
func (b *InMemoryBackend) StopEventDataStoreIngestion(edsIDOrARN string) error {
	b.mu.Lock("StopEventDataStoreIngestion")
	defer b.mu.Unlock()

	eds := b.findEventDataStoreLocked(edsIDOrARN)
	if eds == nil {
		return fmt.Errorf("%w: event data store %s not found", ErrEventDataStoreNotFound, edsIDOrARN)
	}
	eds.Status = "STOPPED_INGESTION"
	eds.UpdatedTimestamp = time.Now().UTC()

	return nil
}

// DisableFederation disables federation for an event data store.
func (b *InMemoryBackend) DisableFederation(edsIDOrARN string) (*EventDataStore, error) {
	b.mu.Lock("DisableFederation")
	defer b.mu.Unlock()

	eds := b.findEventDataStoreLocked(edsIDOrARN)
	if eds == nil {
		return nil, fmt.Errorf("%w: event data store %s not found", ErrEventDataStoreNotFound, edsIDOrARN)
	}
	eds.FederationStatus = "DISABLED"
	eds.FederationRoleArn = ""
	eds.UpdatedTimestamp = time.Now().UTC()
	cp := *eds

	return &cp, nil
}

// EnableFederation enables federation for an event data store, storing the role ARN.
func (b *InMemoryBackend) EnableFederation(edsIDOrARN, federationRoleArn string) (*EventDataStore, error) {
	b.mu.Lock("EnableFederation")
	defer b.mu.Unlock()

	eds := b.findEventDataStoreLocked(edsIDOrARN)
	if eds == nil {
		return nil, fmt.Errorf("%w: event data store %s not found", ErrEventDataStoreNotFound, edsIDOrARN)
	}
	eds.FederationStatus = "ENABLED"
	eds.FederationRoleArn = federationRoleArn
	eds.UpdatedTimestamp = time.Now().UTC()
	cp := *eds

	return &cp, nil
}
