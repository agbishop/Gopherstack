package cloudtrail

import (
	"fmt"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// CreateChannel creates a new CloudTrail channel.
func (b *InMemoryBackend) CreateChannel(
	name, source string,
	destinations []Destination,
	kv map[string]string,
) (*Channel, error) {
	b.mu.Lock("CreateChannel")
	defer b.mu.Unlock()

	if name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrValidation)
	}
	if matches := b.channelsByName.Get(name); len(matches) > 0 {
		return nil, fmt.Errorf("%w: channel %s already exists", ErrChannelAlreadyExists, name)
	}

	b.channelCounter++
	id := fmt.Sprintf("channel-%06d", b.channelCounter)
	channelARN := arn.Build("cloudtrail", b.region, b.accountID, "channel/"+id)
	t := tags.New("cloudtrail.channel." + id + ".tags")
	if len(kv) > 0 {
		t.Merge(kv)
	}
	ch := &Channel{
		ChannelID:    id,
		ChannelARN:   channelARN,
		Name:         name,
		Source:       source,
		Destinations: destinations,
		Tags:         t,
	}
	b.channels.Put(ch)

	cp := *ch

	return &cp, nil
}

// DeleteChannel deletes a channel by ID or ARN.
func (b *InMemoryBackend) DeleteChannel(channelIDOrARN string) error {
	b.mu.Lock("DeleteChannel")
	defer b.mu.Unlock()

	ch := b.findChannelLocked(channelIDOrARN)
	if ch == nil {
		return fmt.Errorf("%w: channel %s not found", ErrChannelNotFound, channelIDOrARN)
	}

	ch.Tags.Close()
	b.channels.Delete(ch.ChannelID)
	b.resourcePolicies.Delete(ch.ChannelARN)

	return nil
}

// GetChannel returns a channel by ID or ARN.
func (b *InMemoryBackend) GetChannel(channelIDOrARN string) (*Channel, error) {
	b.mu.RLock("GetChannel")
	defer b.mu.RUnlock()

	ch := b.findChannelLocked(channelIDOrARN)
	if ch == nil {
		return nil, fmt.Errorf("%w: channel %s not found", ErrChannelNotFound, channelIDOrARN)
	}
	cp := *ch

	return &cp, nil
}

// UpdateChannel updates an existing channel's name and/or destinations.
func (b *InMemoryBackend) UpdateChannel(
	channelIDOrARN, name string,
	destinations []Destination,
) (*Channel, error) {
	b.mu.Lock("UpdateChannel")
	defer b.mu.Unlock()

	ch := b.findChannelLocked(channelIDOrARN)
	if ch == nil {
		return nil, fmt.Errorf("%w: channel %s not found", ErrChannelNotFound, channelIDOrARN)
	}
	if name != "" && name != ch.Name {
		// ch.Name is an indexed field (channelsByName): delete before mutating
		// so the old index entry is removed using the pre-mutation value, then
		// re-Put to rebuild every index (byARN, byName) under the new state.
		b.channels.Delete(ch.ChannelID)
		ch.Name = name
		b.channels.Put(ch)
	}
	if destinations != nil {
		ch.Destinations = destinations
	}
	cp := *ch

	return &cp, nil
}

// ListChannels returns all channels.
func (b *InMemoryBackend) ListChannels() []*Channel {
	b.mu.RLock("ListChannels")
	defer b.mu.RUnlock()

	all := b.channels.All()
	list := make([]*Channel, 0, len(all))
	for _, ch := range all {
		cp := *ch
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ChannelARN < list[j].ChannelARN })

	return list
}
