package mediapackage

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

func newIngestEndpoints(region, channelID string) []storedIngestEndpoint {
	return []storedIngestEndpoint{
		{
			ID:       uuid.NewString(),
			URL:      fmt.Sprintf("https://ingest1.mediapackage.%s.amazonaws.com/in/v2/%s/channel", region, channelID),
			Username: uuid.NewString(),
			Password: uuid.NewString(),
		},
		{
			ID:       uuid.NewString(),
			URL:      fmt.Sprintf("https://ingest2.mediapackage.%s.amazonaws.com/in/v2/%s/channel", region, channelID),
			Username: uuid.NewString(),
			Password: uuid.NewString(),
		},
	}
}

// CreateChannel creates a new MediaPackage channel.
func (b *InMemoryBackend) CreateChannel(id, description string, tags map[string]string) (*Channel, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: Id is required", ErrInvalidParameter)
	}

	b.mu.Lock("CreateChannel")
	defer b.mu.Unlock()

	if b.channels.Has(id) {
		return nil, fmt.Errorf("%w: channel %q already exists", ErrConflict, id)
	}

	tagsCopy := make(map[string]string, len(tags))
	maps.Copy(tagsCopy, tags)

	ch := &storedChannel{
		ARN:             b.buildChannelARN(id),
		ID:              id,
		Description:     description,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		Tags:            tagsCopy,
		IngestEndpoints: newIngestEndpoints(b.region, id),
	}

	b.channels.Put(ch)

	if len(tagsCopy) > 0 {
		b.tags[ch.ARN] = make(map[string]string, len(tagsCopy))
		maps.Copy(b.tags[ch.ARN], tagsCopy)
	}

	return ch.toChannel(), nil
}

// DescribeChannel returns a channel by ID.
func (b *InMemoryBackend) DescribeChannel(id string) (*Channel, error) {
	b.mu.RLock("DescribeChannel")
	defer b.mu.RUnlock()

	ch, ok := b.channels.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: channel %q not found", ErrNotFound, id)
	}

	return ch.toChannel(), nil
}

// UpdateChannel updates a channel's description.
func (b *InMemoryBackend) UpdateChannel(id, description string) (*Channel, error) {
	b.mu.Lock("UpdateChannel")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: channel %q not found", ErrNotFound, id)
	}

	ch.Description = description

	return ch.toChannel(), nil
}

// DeleteChannel deletes a channel and all its origin endpoints.
func (b *InMemoryBackend) DeleteChannel(id string) (*Channel, error) {
	b.mu.Lock("DeleteChannel")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: channel %q not found", ErrNotFound, id)
	}

	// Delete associated origin endpoints.
	matches := slices.Clone(b.originEndpointsByChannel.Get(id))
	for _, ep := range matches {
		b.originEndpoints.Delete(ep.ID)
		delete(b.tags, ep.ARN)
	}

	b.channels.Delete(id)
	delete(b.tags, ch.ARN)

	return ch.toChannel(), nil
}

// ListChannels returns a paginated list of channels sorted by ID.
func (b *InMemoryBackend) ListChannels(maxResults int, nextToken string) ([]*Channel, string, error) {
	b.mu.RLock("ListChannels")
	defer b.mu.RUnlock()

	all := b.channels.Snapshot()

	p := page.New(all, nextToken, maxResults, defaultMaxResults)

	channels := make([]*Channel, 0, len(p.Data))
	for _, ch := range p.Data {
		channels = append(channels, ch.toChannel())
	}

	return channels, p.Next, nil
}

// ConfigureLogs updates the egress/ingress log group configuration for a
// channel. A nil pointer means the caller did not send that
// egressAccessLogs/ingressAccessLogs block and leaves the existing
// configuration untouched, matching AWS's optional-member semantics.
func (b *InMemoryBackend) ConfigureLogs(id string, egressLogGroup, ingressLogGroup *string) (*Channel, error) {
	b.mu.Lock("ConfigureLogs")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: channel %q not found", ErrNotFound, id)
	}

	if egressLogGroup != nil {
		ch.EgressLogGroupName = egressLogGroup
	}

	if ingressLogGroup != nil {
		ch.IngressLogGroupName = ingressLogGroup
	}

	return ch.toChannel(), nil
}

// RotateChannelCredentials changes the channel's first IngestEndpoint's
// username and password (this legacy API only ever touches ingestEndpoints[0]
// -- callers wanting to rotate a specific endpoint use
// RotateIngestEndpointCredentials instead). The endpoint's ID and URL are
// left unchanged; only the credentials are regenerated.
func (b *InMemoryBackend) RotateChannelCredentials(id string) (*Channel, error) {
	b.mu.Lock("RotateChannelCredentials")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: channel %q not found", ErrNotFound, id)
	}

	if len(ch.IngestEndpoints) == 0 {
		return nil, fmt.Errorf("%w: channel %q has no ingest endpoints", ErrNotFound, id)
	}

	ch.IngestEndpoints[0].Username = uuid.NewString()
	ch.IngestEndpoints[0].Password = uuid.NewString()

	return ch.toChannel(), nil
}

// RotateIngestEndpointCredentials rotates credentials for a specific ingest endpoint.
func (b *InMemoryBackend) RotateIngestEndpointCredentials(channelID, ingestEndpointID string) (*Channel, error) {
	b.mu.Lock("RotateIngestEndpointCredentials")
	defer b.mu.Unlock()

	ch, ok := b.channels.Get(channelID)
	if !ok {
		return nil, fmt.Errorf("%w: channel %q not found", ErrNotFound, channelID)
	}

	found := false

	for i, ep := range ch.IngestEndpoints {
		if ep.ID == ingestEndpointID {
			ch.IngestEndpoints[i].Username = uuid.NewString()
			ch.IngestEndpoints[i].Password = uuid.NewString()
			found = true

			break
		}
	}

	if !found {
		return nil, fmt.Errorf(
			"%w: ingest endpoint %q not found in channel %q",
			ErrNotFound,
			ingestEndpointID,
			channelID,
		)
	}

	return ch.toChannel(), nil
}
