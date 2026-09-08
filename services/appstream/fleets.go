package appstream

import (
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

const (
	fleetStateRunning = "RUNNING"
	fleetStateStopped = "STOPPED"

	defaultFleetType         = "ON_DEMAND"
	defaultMaxUserDuration   = 57600 // 16 hours
	defaultDisconnectTimeout = 300   // 5 minutes
)

type storedFleet struct {
	EnableDefaultInternetAccess *bool             `json:"enableDefaultInternetAccess,omitempty"`
	CreatedTime                 time.Time         `json:"createdTime"`
	Tags                        map[string]string `json:"tags"`
	Name                        string            `json:"name"`
	Arn                         string            `json:"arn"`
	DisplayName                 string            `json:"displayName"`
	Description                 string            `json:"description"`
	InstanceType                string            `json:"instanceType"`
	FleetType                   string            `json:"fleetType"`
	State                       string            `json:"state"`
	ImageName                   string            `json:"imageName,omitempty"`
	ImageArn                    string            `json:"imageArn,omitempty"`
	DesiredInstances            int               `json:"desiredInstances"`
	MaxUserDurationSecs         int               `json:"maxUserDurationSecs"`
	DisconnectTimeoutSecs       int               `json:"disconnectTimeoutSecs"`
	IdleDisconnectTimeoutSecs   int               `json:"idleDisconnectTimeoutSecs"`
}

func (f *storedFleet) toFleet() *Fleet {
	tags := make(map[string]string)
	maps.Copy(tags, f.Tags)

	return &Fleet{
		EnableDefaultInternetAccess: f.EnableDefaultInternetAccess,
		CreatedTime:                 f.CreatedTime,
		Tags:                        tags,
		Name:                        f.Name,
		Arn:                         f.Arn,
		DisplayName:                 f.DisplayName,
		Description:                 f.Description,
		InstanceType:                f.InstanceType,
		FleetType:                   f.FleetType,
		State:                       f.State,
		ImageName:                   f.ImageName,
		ImageArn:                    f.ImageArn,
		DesiredInstances:            f.DesiredInstances,
		MaxUserDurationSecs:         f.MaxUserDurationSecs,
		DisconnectTimeoutSecs:       f.DisconnectTimeoutSecs,
		IdleDisconnectTimeoutSecs:   f.IdleDisconnectTimeoutSecs,
	}
}

func (b *InMemoryBackend) fleetARN(name string) string {
	return arn.Build("appstream", b.region, b.accountID, fmt.Sprintf("fleet/%s", name))
}

// isValidFleetType reports whether ft is an accepted AWS fleet type.
func isValidFleetType(ft string) bool {
	switch ft {
	case "ALWAYS_ON", "ON_DEMAND", "ELASTIC":
		return true
	}

	return false
}

// CreateFleet creates a new fleet.
func (b *InMemoryBackend) CreateFleet(
	name, displayName, description, instanceType, fleetType, imageName, imageArn string,
	desiredInstances, maxUserDuration, disconnectTimeout, idleDisconnectTimeout int,
	enableDefaultInternetAccess *bool,
	tags map[string]string,
) (*Fleet, error) {
	if instanceType == "" {
		return nil, fmt.Errorf("%w: InstanceType is required", awserr.ErrInvalidParameter)
	}

	if fleetType != "" && !isValidFleetType(fleetType) {
		return nil, fmt.Errorf(
			"%w: FleetType %q is not valid; must be ALWAYS_ON, ON_DEMAND, or ELASTIC",
			awserr.ErrInvalidParameter,
			fleetType,
		)
	}

	b.mu.Lock("CreateFleet")
	defer b.mu.Unlock()

	if b.fleets.Has(name) {
		return nil, ErrAlreadyExists
	}

	arn := b.fleetARN(name)
	storedTags := make(map[string]string)
	maps.Copy(storedTags, tags)

	ft := fleetType
	if ft == "" {
		ft = defaultFleetType
	}

	mux := maxUserDuration
	if mux == 0 {
		mux = defaultMaxUserDuration
	}

	dt := disconnectTimeout
	if dt == 0 {
		dt = defaultDisconnectTimeout
	}

	desired := desiredInstances
	if desired == 0 {
		desired = 1
	}

	f := &storedFleet{
		EnableDefaultInternetAccess: enableDefaultInternetAccess,
		CreatedTime:                 time.Now().UTC(),
		Tags:                        storedTags,
		Name:                        name,
		Arn:                         arn,
		DisplayName:                 displayName,
		Description:                 description,
		InstanceType:                instanceType,
		FleetType:                   ft,
		State:                       fleetStateStopped,
		ImageName:                   imageName,
		ImageArn:                    imageArn,
		DesiredInstances:            desired,
		MaxUserDurationSecs:         mux,
		DisconnectTimeoutSecs:       dt,
		IdleDisconnectTimeoutSecs:   idleDisconnectTimeout,
	}
	b.fleets.Put(f)
	b.tags[arn] = storedTags

	return f.toFleet(), nil
}

// DescribeFleets returns fleets, optionally filtered by names.
func (b *InMemoryBackend) DescribeFleets(names []string) ([]*Fleet, error) {
	b.mu.RLock("DescribeFleets")
	defer b.mu.RUnlock()

	if len(names) > 0 {
		var result []*Fleet

		for _, name := range names {
			f, ok := b.fleets.Get(name)
			if !ok {
				return nil, ErrNotFound
			}

			result = append(result, f.toFleet())
		}

		return result, nil
	}

	result := make([]*Fleet, 0, b.fleets.Len())
	for _, f := range b.fleets.All() {
		result = append(result, f.toFleet())
	}

	return result, nil
}

// UpdateFleet updates mutable fields of an existing fleet.
func (b *InMemoryBackend) UpdateFleet(
	name, displayName, description, instanceType, imageName, imageArn string,
	desiredInstances, maxUserDuration, disconnectTimeout, idleDisconnectTimeout int,
	enableDefaultInternetAccess *bool,
) (*Fleet, error) {
	b.mu.Lock("UpdateFleet")
	defer b.mu.Unlock()

	f, ok := b.fleets.Get(name)
	if !ok {
		return nil, ErrNotFound
	}

	if displayName != "" {
		f.DisplayName = displayName
	}

	if description != "" {
		f.Description = description
	}

	if instanceType != "" {
		f.InstanceType = instanceType
	}

	if imageName != "" {
		f.ImageName = imageName
	}

	if imageArn != "" {
		f.ImageArn = imageArn
	}

	if desiredInstances > 0 {
		f.DesiredInstances = desiredInstances
	}

	if maxUserDuration > 0 {
		f.MaxUserDurationSecs = maxUserDuration
	}

	if disconnectTimeout > 0 {
		f.DisconnectTimeoutSecs = disconnectTimeout
	}

	if idleDisconnectTimeout >= 0 && idleDisconnectTimeout != f.IdleDisconnectTimeoutSecs {
		f.IdleDisconnectTimeoutSecs = idleDisconnectTimeout
	}

	if enableDefaultInternetAccess != nil {
		f.EnableDefaultInternetAccess = enableDefaultInternetAccess
	}

	return f.toFleet(), nil
}

// DeleteFleet removes a fleet. Returns ErrResourceInUse if fleet is running.
func (b *InMemoryBackend) DeleteFleet(name string) error {
	b.mu.Lock("DeleteFleet")
	defer b.mu.Unlock()

	f, ok := b.fleets.Get(name)
	if !ok {
		return ErrNotFound
	}

	if f.State == fleetStateRunning {
		return ErrResourceInUse
	}

	delete(b.tags, f.Arn)
	b.fleets.Delete(name)
	delete(b.associations, name)

	return nil
}

// StartFleet transitions a fleet to RUNNING.
func (b *InMemoryBackend) StartFleet(name string) error {
	b.mu.Lock("StartFleet")
	defer b.mu.Unlock()

	f, ok := b.fleets.Get(name)
	if !ok {
		return ErrNotFound
	}

	if f.State == fleetStateRunning {
		return ErrFleetNotStopped
	}

	f.State = fleetStateRunning

	return nil
}

// StopFleet transitions a fleet to STOPPED. Idempotent: stopping an
// already-stopped fleet succeeds (real AWS's StopFleet has no state-conflict
// exception -- only ResourceNotFoundException and ConcurrentModificationException).
func (b *InMemoryBackend) StopFleet(name string) error {
	b.mu.Lock("StopFleet")
	defer b.mu.Unlock()

	f, ok := b.fleets.Get(name)
	if !ok {
		return ErrNotFound
	}

	f.State = fleetStateStopped

	return nil
}

// AssociateFleet links a fleet and a stack.
func (b *InMemoryBackend) AssociateFleet(fleetName, stackName string) error {
	b.mu.Lock("AssociateFleet")
	defer b.mu.Unlock()

	if !b.fleets.Has(fleetName) {
		return ErrNotFound
	}

	if !b.stacks.Has(stackName) {
		return ErrNotFound
	}

	if b.associations[fleetName] == nil {
		b.associations[fleetName] = make(map[string]bool)
	}

	b.associations[fleetName][stackName] = true

	return nil
}

// DisassociateFleet unlinks a fleet and a stack.
func (b *InMemoryBackend) DisassociateFleet(fleetName, stackName string) error {
	b.mu.Lock("DisassociateFleet")
	defer b.mu.Unlock()

	if !b.fleets.Has(fleetName) {
		return ErrNotFound
	}

	if !b.stacks.Has(stackName) {
		return ErrNotFound
	}

	if b.associations[fleetName] != nil {
		delete(b.associations[fleetName], stackName)
	}

	return nil
}

// ListAssociatedFleets returns fleet names associated with a stack.
func (b *InMemoryBackend) ListAssociatedFleets(stackName string) ([]string, error) {
	b.mu.RLock("ListAssociatedFleets")
	defer b.mu.RUnlock()

	if !b.stacks.Has(stackName) {
		return nil, ErrNotFound
	}

	var fleets []string

	for fleet, stacks := range b.associations {
		if stacks[stackName] {
			fleets = append(fleets, fleet)
		}
	}

	return fleets, nil
}

// ListAssociatedStacks returns stack names associated with a fleet.
func (b *InMemoryBackend) ListAssociatedStacks(fleetName string) ([]string, error) {
	b.mu.RLock("ListAssociatedStacks")
	defer b.mu.RUnlock()

	if !b.fleets.Has(fleetName) {
		return nil, ErrNotFound
	}

	stacks := b.associations[fleetName]
	result := make([]string, 0, len(stacks))

	for stack := range stacks {
		result = append(result, stack)
	}

	return result, nil
}
