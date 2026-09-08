package appstream

import (
	"fmt"
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

const (
	appBlockStateActive   = "ACTIVE"
	appBlockStateInactive = "INACTIVE"

	builderStateStopped  = "STOPPED"
	builderStateRunning  = "RUNNING"
	builderStateStarting = "STARTING"
	builderStateStopping = "STOPPING"
)

type storedAppBlock struct {
	CreatedTime time.Time         `json:"createdTime"`
	Tags        map[string]string `json:"tags"`
	Name        string            `json:"name"`
	Arn         string            `json:"arn"`
	Description string            `json:"description"`
	State       string            `json:"state"`
}

func (a *storedAppBlock) toAppBlock() *AppBlock {
	tags := make(map[string]string)
	maps.Copy(tags, a.Tags)

	return &AppBlock{
		CreatedTime: a.CreatedTime,
		Tags:        tags,
		Name:        a.Name,
		Arn:         a.Arn,
		Description: a.Description,
		State:       a.State,
	}
}

type storedAppBlockBuilder struct {
	CreatedTime  time.Time         `json:"createdTime"`
	Tags         map[string]string `json:"tags"`
	Name         string            `json:"name"`
	Arn          string            `json:"arn"`
	Description  string            `json:"description"`
	Platform     string            `json:"platform"`
	InstanceType string            `json:"instanceType"`
	State        string            `json:"state"`
}

func (b *storedAppBlockBuilder) toAppBlockBuilder() *AppBlockBuilder {
	tags := make(map[string]string)
	maps.Copy(tags, b.Tags)

	return &AppBlockBuilder{
		CreatedTime:  b.CreatedTime,
		Tags:         tags,
		Name:         b.Name,
		Arn:          b.Arn,
		Description:  b.Description,
		Platform:     b.Platform,
		InstanceType: b.InstanceType,
		State:        b.State,
	}
}

func (b *InMemoryBackend) appBlockARN(name string) string {
	return arn.Build("appstream", b.region, b.accountID, fmt.Sprintf("app-block/%s", name))
}

func (b *InMemoryBackend) appBlockBuilderARN(name string) string {
	return arn.Build("appstream", b.region, b.accountID, fmt.Sprintf("app-block-builder/%s", name))
}

// CreateAppBlock creates an app block.
func (b *InMemoryBackend) CreateAppBlock(name, description string, tags map[string]string) (*AppBlock, error) {
	b.mu.Lock("CreateAppBlock")
	defer b.mu.Unlock()

	if b.appBlocks.Has(name) {
		return nil, ErrAlreadyExists
	}

	arn := b.appBlockARN(name)
	storedTags := make(map[string]string)
	maps.Copy(storedTags, tags)

	ab := &storedAppBlock{
		CreatedTime: time.Now().UTC(),
		Tags:        storedTags,
		Name:        name,
		Arn:         arn,
		Description: description,
		State:       appBlockStateInactive,
	}
	b.appBlocks.Put(ab)
	b.tags[arn] = storedTags

	return ab.toAppBlock(), nil
}

// DeleteAppBlock removes an app block. Returns ErrResourceInUse if any
// application still references it via AppBlockArn (DeleteAppBlock models
// ResourceInUseException; CreateApplication's AppBlockArn is the only real
// reference to an app block this backend tracks).
func (b *InMemoryBackend) DeleteAppBlock(name string) error {
	b.mu.Lock("DeleteAppBlock")
	defer b.mu.Unlock()

	ab, ok := b.appBlocks.Get(name)
	if !ok {
		return ErrNotFound
	}

	for _, app := range b.applications.All() {
		if app.AppBlockArn == ab.Arn {
			return ErrResourceInUse
		}
	}

	delete(b.tags, ab.Arn)
	b.appBlocks.Delete(name)

	return nil
}

// findAppBlock resolves id against Name (the primary key used by
// CreateAppBlock/DeleteAppBlock) or Arn. Real AWS identifies app blocks by
// ARN in DescribeAppBlocks and in the AppBlockArn member of the
// AppBlockBuilder-AppBlock association operations, so callers on that wire
// path must resolve through this helper rather than indexing b.appBlocks
// directly with the caller-supplied identifier.
func (b *InMemoryBackend) findAppBlock(id string) (*storedAppBlock, bool) {
	if ab, ok := b.appBlocks.Get(id); ok {
		return ab, true
	}

	for _, ab := range b.appBlocks.All() {
		if ab.Arn == id {
			return ab, true
		}
	}

	return nil, false
}

// DescribeAppBlocks returns app blocks, optionally filtered by ARN.
func (b *InMemoryBackend) DescribeAppBlocks(arns []string) ([]*AppBlock, error) {
	b.mu.RLock("DescribeAppBlocks")
	defer b.mu.RUnlock()

	if len(arns) > 0 {
		var result []*AppBlock

		for _, id := range arns {
			ab, ok := b.findAppBlock(id)
			if !ok {
				return nil, ErrNotFound
			}

			result = append(result, ab.toAppBlock())
		}

		return result, nil
	}

	result := make([]*AppBlock, 0, b.appBlocks.Len())
	for _, ab := range b.appBlocks.All() {
		result = append(result, ab.toAppBlock())
	}

	return result, nil
}

// CreateAppBlockBuilder creates an app block builder.
func (b *InMemoryBackend) CreateAppBlockBuilder(
	name, description, platform, instanceType string,
	tags map[string]string,
) (*AppBlockBuilder, error) {
	if instanceType == "" {
		return nil, fmt.Errorf("%w: InstanceType is required", awserr.ErrInvalidParameter)
	}

	b.mu.Lock("CreateAppBlockBuilder")
	defer b.mu.Unlock()

	if b.appBlockBuilders.Has(name) {
		return nil, ErrAlreadyExists
	}

	arn := b.appBlockBuilderARN(name)
	storedTags := make(map[string]string)
	maps.Copy(storedTags, tags)

	bb := &storedAppBlockBuilder{
		CreatedTime:  time.Now().UTC(),
		Tags:         storedTags,
		Name:         name,
		Arn:          arn,
		Description:  description,
		Platform:     platform,
		InstanceType: instanceType,
		State:        builderStateStopped,
	}
	b.appBlockBuilders.Put(bb)
	b.tags[arn] = storedTags

	return bb.toAppBlockBuilder(), nil
}

// DeleteAppBlockBuilder removes an app block builder.
func (b *InMemoryBackend) DeleteAppBlockBuilder(name string) error {
	b.mu.Lock("DeleteAppBlockBuilder")
	defer b.mu.Unlock()

	bb, ok := b.appBlockBuilders.Get(name)
	if !ok {
		return ErrNotFound
	}

	if bb.State == builderStateRunning {
		return ErrResourceInUse
	}

	delete(b.tags, bb.Arn)
	b.appBlockBuilders.Delete(name)
	delete(b.appBlockBuilderAssoc, name)

	return nil
}

// DescribeAppBlockBuilders returns builders, optionally filtered by name.
func (b *InMemoryBackend) DescribeAppBlockBuilders(names []string) ([]*AppBlockBuilder, error) {
	b.mu.RLock("DescribeAppBlockBuilders")
	defer b.mu.RUnlock()

	if len(names) > 0 {
		var result []*AppBlockBuilder

		for _, name := range names {
			bb, ok := b.appBlockBuilders.Get(name)
			if !ok {
				return nil, ErrNotFound
			}

			result = append(result, bb.toAppBlockBuilder())
		}

		return result, nil
	}

	result := make([]*AppBlockBuilder, 0, b.appBlockBuilders.Len())
	for _, bb := range b.appBlockBuilders.All() {
		result = append(result, bb.toAppBlockBuilder())
	}

	return result, nil
}

// StartAppBlockBuilder transitions a builder to RUNNING.
func (b *InMemoryBackend) StartAppBlockBuilder(name string) error {
	b.mu.Lock("StartAppBlockBuilder")
	defer b.mu.Unlock()

	bb, ok := b.appBlockBuilders.Get(name)
	if !ok {
		return ErrNotFound
	}

	if bb.State == builderStateRunning {
		return ErrFleetNotStopped
	}

	bb.State = builderStateRunning

	return nil
}

// StopAppBlockBuilder transitions a builder to STOPPED. Idempotent: stopping
// an already-stopped builder succeeds (real AWS's StopAppBlockBuilder has no
// state-conflict exception -- only ConcurrentModificationException,
// OperationNotPermittedException, and ResourceNotFoundException).
func (b *InMemoryBackend) StopAppBlockBuilder(name string) error {
	b.mu.Lock("StopAppBlockBuilder")
	defer b.mu.Unlock()

	bb, ok := b.appBlockBuilders.Get(name)
	if !ok {
		return ErrNotFound
	}

	bb.State = builderStateStopped

	return nil
}

// UpdateAppBlockBuilder updates mutable builder fields.
func (b *InMemoryBackend) UpdateAppBlockBuilder(name, description, instanceType string) (*AppBlockBuilder, error) {
	b.mu.Lock("UpdateAppBlockBuilder")
	defer b.mu.Unlock()

	bb, ok := b.appBlockBuilders.Get(name)
	if !ok {
		return nil, ErrNotFound
	}

	if description != "" {
		bb.Description = description
	}

	if instanceType != "" {
		bb.InstanceType = instanceType
	}

	return bb.toAppBlockBuilder(), nil
}

// CreateAppBlockBuilderStreamingURL returns a streaming URL for the builder
// along with its expiry time. validitySeconds <= 0 falls back to the real AWS
// default of 3600 seconds.
func (b *InMemoryBackend) CreateAppBlockBuilderStreamingURL(
	name string,
	validitySeconds int64,
) (string, time.Time, error) {
	b.mu.RLock("CreateAppBlockBuilderStreamingURL")
	defer b.mu.RUnlock()

	if !b.appBlockBuilders.Has(name) {
		return "", time.Time{}, ErrNotFound
	}

	validity := validitySeconds
	if validity <= 0 {
		validity = defaultBuilderStreamingURLValiditySeconds
	}

	expires := time.Now().UTC().Add(time.Duration(validity) * time.Second)

	url := fmt.Sprintf("https://appstream2.%s.aws.amazon.com/authenticate?param=builder-%s", b.region, name)

	return url, expires, nil
}

// AssociateAppBlockBuilderAppBlock links a builder to an app block and
// returns the association. appBlockID accepts either the app block Name or
// its Arn -- real AWS's AssociateAppBlockBuilderAppBlock request carries the
// AppBlockArn. The real AssociateAppBlockBuilderAppBlockOutput carries the
// AppBlockBuilderAppBlockAssociation itself
// (deserializeCBOR_AssociateAppBlockBuilderAppBlockOutput in the pinned
// appstream SDK's deserializers.go), not an empty envelope.
func (b *InMemoryBackend) AssociateAppBlockBuilderAppBlock(
	builderName, appBlockID string,
) (*AppBlockBuilderAppBlockAssociation, error) {
	b.mu.Lock("AssociateAppBlockBuilderAppBlock")
	defer b.mu.Unlock()

	if !b.appBlockBuilders.Has(builderName) {
		return nil, ErrNotFound
	}

	ab, ok := b.findAppBlock(appBlockID)
	if !ok {
		return nil, ErrNotFound
	}

	if b.appBlockBuilderAssoc[builderName] == nil {
		b.appBlockBuilderAssoc[builderName] = make(map[string]bool)
	}

	b.appBlockBuilderAssoc[builderName][ab.Name] = true

	return &AppBlockBuilderAppBlockAssociation{
		AppBlockBuilderName: builderName,
		AppBlockArn:         ab.Arn,
		State:               associationStateActive,
	}, nil
}

// DisassociateAppBlockBuilderAppBlock removes a builder-appblock link.
// appBlockID accepts either the app block Name or its Arn, matching
// AssociateAppBlockBuilderAppBlock.
func (b *InMemoryBackend) DisassociateAppBlockBuilderAppBlock(builderName, appBlockID string) error {
	b.mu.Lock("DisassociateAppBlockBuilderAppBlock")
	defer b.mu.Unlock()

	if !b.appBlockBuilders.Has(builderName) {
		return ErrNotFound
	}

	ab, ok := b.findAppBlock(appBlockID)
	if !ok {
		return ErrNotFound
	}

	if b.appBlockBuilderAssoc[builderName] != nil {
		delete(b.appBlockBuilderAssoc[builderName], ab.Name)
	}

	return nil
}

// DescribeAppBlockBuilderAppBlockAssociations lists builder-appblock associations.
// appBlockID accepts either the app block Name or its Arn, matching
// AssociateAppBlockBuilderAppBlock. A non-matching filter yields an empty
// result (real AWS's Describe op has no ResourceNotFoundException).
func (b *InMemoryBackend) DescribeAppBlockBuilderAppBlockAssociations(
	builderName, appBlockID string,
) ([]*AppBlockBuilderAppBlockAssociation, error) {
	b.mu.RLock("DescribeAppBlockBuilderAppBlockAssociations")
	defer b.mu.RUnlock()

	targetName := ""

	if appBlockID != "" {
		ab, ok := b.findAppBlock(appBlockID)
		if !ok {
			return nil, nil
		}

		targetName = ab.Name
	}

	var result []*AppBlockBuilderAppBlockAssociation

	for bName, appBlocks := range b.appBlockBuilderAssoc {
		if builderName != "" && bName != builderName {
			continue
		}

		for abName := range appBlocks {
			ab, ok := b.appBlocks.Get(abName)
			if !ok {
				continue
			}

			if targetName != "" && abName != targetName {
				continue
			}

			result = append(result, &AppBlockBuilderAppBlockAssociation{
				AppBlockBuilderName: bName,
				AppBlockArn:         ab.Arn,
				State:               associationStateActive,
			})
		}
	}

	return result, nil
}
