package medialive

import (
	"fmt"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// --- Channel operations ---

// validateAnywhereSettings checks that a caller-supplied AnywhereSettings
// references resources that actually exist. ClusterID was validated before
// this fix; ChannelPlacementGroupID was silently accepted without any
// existence check (gopherstack-jb9i) -- a caller referencing an unknown
// placement group got a 201/200 response but the value never appeared in
// any group's derived "channels" list. A placement group is nested under a
// cluster (composite key "<clusterID>/<groupID>", see cpgKey), so the
// placement-group check is keyed off the SAME AnywhereSettings.ClusterID the
// caller supplied -- matching AWS: a channelPlacementGroupId is only
// meaningful together with its owning clusterId. Caller must already hold
// b.mu (Lock or RLock).
func (b *InMemoryBackend) validateAnywhereSettings(s ChannelAnywhereSettings) error {
	if s.ClusterID != "" && !b.clusters.Has(s.ClusterID) {
		return fmt.Errorf("%w: cluster %s not found", ErrInvalidParameter, s.ClusterID)
	}

	if s.ChannelPlacementGroupID != "" &&
		!b.channelPlacementGroups.Has(cpgKey(s.ClusterID, s.ChannelPlacementGroupID)) {
		return fmt.Errorf(
			"%w: channelPlacementGroup %s not found", ErrInvalidParameter, s.ChannelPlacementGroupID,
		)
	}

	return nil
}

// followingChannelArns returns the sorted set of channel ARNs whose
// LinkedChannelSettings.Follower.PrimaryChannelArn matches primaryARN --
// the real DescribePrimaryChannelSettings.FollowingChannelArns is computed
// by MediaLive from every OTHER channel's follower settings, not stored on
// the primary channel itself, so gopherstack derives it the same way at
// read time (same pattern as channelIDsForCluster/channelIDsForPlacementGroup/
// channelPlacementGroupIDsForNode). Caller must already hold b.mu (Lock or
// RLock).
func (b *InMemoryBackend) followingChannelArns(primaryARN string) []string {
	if primaryARN == "" {
		return nil
	}

	arns := []string{}

	for _, ch := range b.channels.All() {
		if ch.LinkedChannelSettings.Follower.PrimaryChannelArn == primaryARN {
			arns = append(arns, ch.ARN)
		}
	}

	sort.Strings(arns)

	return arns
}

// toChannelWithDerived converts a stored channel to its domain shape and
// stamps in every read-time-derived field (currently just
// LinkedChannelSettings.Primary.FollowingChannelArns). Caller must already
// hold b.mu (Lock or RLock).
func (b *InMemoryBackend) toChannelWithDerived(ch *storedChannel) *Channel {
	out := ch.toChannel()
	out.LinkedChannelSettings.Primary.FollowingChannelArns = b.followingChannelArns(ch.ARN)

	return out
}

// applyChannelCreateExtras copies the gopherstack-jb9i CreateChannelInput
// members onto a freshly-created storedChannel. Split out of CreateChannel
// to keep it under the project's funlen/gocyclo/gocognit budget.
func applyChannelCreateExtras(ch *storedChannel, extras ChannelCreateExtras) {
	ch.CdiInputSpecification = extras.CdiInputSpecification
	ch.ChannelEngineVersion = extras.ChannelEngineVersion
	ch.ChannelSecurityGroups = extras.ChannelSecurityGroups
	ch.Destinations = extras.Destinations
	ch.EncoderSettings = extras.EncoderSettings
	ch.InferenceSettings = extras.InferenceSettings
	ch.InputAttachments = extras.InputAttachments
	ch.InputSpecification = extras.InputSpecification
	ch.LinkedChannelSettings = extras.LinkedChannelSettings
	ch.LogLevel = extras.LogLevel
	ch.Maintenance = extras.Maintenance
	ch.Vpc = extras.Vpc
}

// applyChannelUpdateExtras overwrites only the fields the caller included in
// the UpdateChannel request body (each guarded by its own HasX flag), same
// "include this parameter only if you want to change it" convention as
// anywhereSettings/networkSettings/renewalSettings elsewhere in this
// service. Split out of UpdateChannel to keep it under the project's
// funlen/gocyclo/gocognit budget.
func applyChannelUpdateExtras(ch *storedChannel, extras ChannelUpdateExtras) {
	if extras.HasCdiInputSpecification {
		ch.CdiInputSpecification = extras.CdiInputSpecification
	}

	if extras.HasChannelEngineVersion {
		ch.ChannelEngineVersion = extras.ChannelEngineVersion
	}

	if extras.HasChannelSecurityGroups {
		ch.ChannelSecurityGroups = extras.ChannelSecurityGroups
	}

	if extras.HasDestinations {
		ch.Destinations = extras.Destinations
	}

	if extras.HasEncoderSettings {
		ch.EncoderSettings = extras.EncoderSettings
	}

	if extras.HasInferenceSettings {
		ch.InferenceSettings = extras.InferenceSettings
	}

	if extras.HasInputAttachments {
		ch.InputAttachments = extras.InputAttachments
	}

	if extras.HasInputSpecification {
		ch.InputSpecification = extras.InputSpecification
	}

	if extras.HasLinkedChannelSettings {
		ch.LinkedChannelSettings = extras.LinkedChannelSettings
	}

	if extras.HasLogLevel {
		ch.LogLevel = extras.LogLevel
	}

	if extras.HasMaintenance {
		ch.Maintenance = extras.Maintenance
	}

	if extras.HasVpc {
		ch.Vpc = extras.Vpc
	}
}

// CreateChannel creates a new channel.
func (b *InMemoryBackend) CreateChannel(
	name, channelClass, roleArn string,
	anywhereSettings ChannelAnywhereSettings,
	extras ChannelCreateExtras,
	tags map[string]string,
) (*Channel, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name required", ErrInvalidParameter)
	}

	if channelClass == "" {
		channelClass = channelClassStandard
	}

	b.mu.Lock("CreateChannel")
	defer b.mu.Unlock()

	if err := b.validateAnywhereSettings(anywhereSettings); err != nil {
		return nil, err
	}

	id := newID()
	ch := &storedChannel{
		ARN:              b.channelARN(id),
		ID:               id,
		Name:             name,
		ChannelClass:     channelClass,
		RoleARN:          roleArn,
		State:            stateIdle,
		Tags:             copyTags(tags),
		AnywhereSettings: anywhereSettings,
	}
	applyChannelCreateExtras(ch, extras)

	b.channels.Put(ch)

	return b.toChannelWithDerived(ch), nil
}

// DescribeChannel returns a channel by ID.
func (b *InMemoryBackend) DescribeChannel(channelID string) (*Channel, error) {
	b.mu.RLock("DescribeChannel")
	defer b.mu.RUnlock()

	ch, ok := b.channels.Get(channelID)
	if !ok {
		return nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, channelID)
	}

	return b.toChannelWithDerived(ch), nil
}

// UpdateChannel updates a channel's mutable fields.
func (b *InMemoryBackend) UpdateChannel(
	channelID, name, roleArn string,
	anywhereSettings ChannelAnywhereSettings,
	hasAnywhereSettings bool,
	extras ChannelUpdateExtras,
) (*Channel, error) {
	b.mu.Lock("UpdateChannel")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(channelID)
	if !ok {
		return nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, channelID)
	}

	if name != "" {
		ch.Name = name
	}

	if roleArn != "" {
		ch.RoleARN = roleArn
	}

	if hasAnywhereSettings {
		if err := b.validateAnywhereSettings(anywhereSettings); err != nil {
			return nil, err
		}

		ch.AnywhereSettings = anywhereSettings
	}

	applyChannelUpdateExtras(ch, extras)

	return b.toChannelWithDerived(ch), nil
}

// DeleteChannel deletes a channel.
// api_op_DeleteChannel.go's doc comment ("Starts deletion of channel.")
// mirrors StartChannel's "Starts an existing channel" -- the channel is
// removed immediately (deterministic emulation), but the response carries
// DELETING to match the same intermediate-state contract StartChannel/
// StopChannel already follow in this file.
func (b *InMemoryBackend) DeleteChannel(channelID string) (*Channel, error) {
	b.mu.Lock("DeleteChannel")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(channelID)
	if !ok {
		return nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, channelID)
	}

	if ch.State == stateRunning {
		return nil, fmt.Errorf("%w: channel must be idle before deleting", ErrConflict)
	}

	out := b.toChannelWithDerived(ch)
	out.State = stateDeleting
	b.channels.Delete(channelID)

	return out, nil
}

// ListChannels returns a paginated list of channels.
func (b *InMemoryBackend) ListChannels(
	maxResults int,
	nextToken string,
) ([]*ChannelSummary, string, error) {
	b.mu.RLock("ListChannels")
	defer b.mu.RUnlock()

	all := b.channels.All()

	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	pg := page.New(all, nextToken, maxResults, defaultMaxResults)

	summaries := make([]*ChannelSummary, 0, len(pg.Data))
	for _, ch := range pg.Data {
		s := ch.toSummary()
		s.LinkedChannelSettings.Primary.FollowingChannelArns = b.followingChannelArns(ch.ARN)
		summaries = append(summaries, s)
	}

	return summaries, pg.Next, nil
}

// StartChannel transitions a channel toward RUNNING.
// The stored state advances immediately to RUNNING (deterministic emulation), but
// the API response carries STARTING to match the real AWS intermediate-state contract.
func (b *InMemoryBackend) StartChannel(channelID string) (*Channel, error) {
	b.mu.Lock("StartChannel")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(channelID)
	if !ok {
		return nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, channelID)
	}

	if ch.State != stateIdle {
		return nil, fmt.Errorf("%w: channel must be idle to start", ErrConflict)
	}

	ch.State = stateRunning

	result := b.toChannelWithDerived(ch)
	result.State = stateStarting

	return result, nil
}

// StopChannel transitions a channel toward IDLE.
// The stored state advances immediately to IDLE (deterministic emulation), but
// the API response carries STOPPING to match the real AWS intermediate-state contract.
func (b *InMemoryBackend) StopChannel(channelID string) (*Channel, error) {
	b.mu.Lock("StopChannel")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(channelID)
	if !ok {
		return nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, channelID)
	}

	if ch.State != stateRunning {
		return nil, fmt.Errorf("%w: channel must be running to stop", ErrConflict)
	}

	ch.State = stateIdle

	result := b.toChannelWithDerived(ch)
	result.State = stateStopping

	return result, nil
}

// --- Alerts and versions ---

// ListAlerts returns alerts for a channel (always empty in emulation).
func (b *InMemoryBackend) ListAlerts(channelID string) ([]map[string]any, error) {
	b.mu.RLock("ListAlerts")
	defer b.mu.RUnlock()

	if !b.channels.Has(channelID) {
		return nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, channelID)
	}

	return []map[string]any{}, nil
}

// ListVersions returns the available channel engine versions.
func (b *InMemoryBackend) ListVersions() []ChannelEngineVersion {
	return []ChannelEngineVersion{
		{Version: channelEngineVersion, ExpirationDate: ""},
	}
}

// --- Channel lifecycle extras ---

// UpdateChannelClass changes a channel's class.
func (b *InMemoryBackend) UpdateChannelClass(channelID, channelClass string) (*Channel, error) {
	b.mu.Lock("UpdateChannelClass")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(channelID)
	if !ok {
		return nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, channelID)
	}

	if channelClass == "" {
		return nil, fmt.Errorf("%w: channelClass required", ErrInvalidParameter)
	}

	ch.ChannelClass = channelClass

	return b.toChannelWithDerived(ch), nil
}

// RestartChannelPipelines restarts a channel's pipelines (returns the channel).
func (b *InMemoryBackend) RestartChannelPipelines(
	channelID string,
	_ []string,
) (*Channel, error) {
	b.mu.RLock("RestartChannelPipelines")
	defer b.mu.RUnlock()

	ch, ok := b.channels.Get(channelID)
	if !ok {
		return nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, channelID)
	}

	return b.toChannelWithDerived(ch), nil
}

// DescribeThumbnails returns the channel (thumbnails are not emulated as image data).
func (b *InMemoryBackend) DescribeThumbnails(channelID string) (*Channel, error) {
	b.mu.RLock("DescribeThumbnails")
	defer b.mu.RUnlock()

	ch, ok := b.channels.Get(channelID)
	if !ok {
		return nil, fmt.Errorf("%w: channel %s not found", ErrNotFound, channelID)
	}

	return b.toChannelWithDerived(ch), nil
}
