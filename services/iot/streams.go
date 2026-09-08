package iot

import (
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// StreamFile holds file info for an IoT stream.
type StreamFile struct {
	S3Bucket string `json:"s3Bucket,omitempty"`
	S3Key    string `json:"s3Key,omitempty"`
	FileID   int32  `json:"fileId,omitempty"`
}

// IoTStream represents an IoT data stream.
//
//nolint:revive // IoTStream is intentional to maintain AWS API naming clarity
type IoTStream struct {
	Tags          map[string]string `json:"tags,omitempty"`
	StreamID      string            `json:"streamId"`
	StreamARN     string            `json:"streamArn"`
	Description   string            `json:"description,omitempty"`
	RoleARN       string            `json:"roleArn,omitempty"`
	Files         []StreamFile      `json:"files,omitempty"`
	CreatedAt     float64           `json:"createdAt,omitempty"`
	LastUpdated   float64           `json:"lastUpdatedAt,omitempty"`
	StreamVersion int32             `json:"streamVersion,omitempty"`
}

func cloneStream(s *IoTStream) *IoTStream {
	cp := *s
	cp.Files = append([]StreamFile(nil), s.Files...)

	return &cp
}

func (b *InMemoryBackend) streamARN(id string) string {
	return arn.Build("iot", b.region, b.accountID, fmt.Sprintf("stream/%s", id))
}

// CreateStreamInput holds input for CreateStream.
type CreateStreamInput struct {
	// []types.Tag on the wire, not a map (serializers.go:4643, aws-sdk-go-v2/service/iot@v1.77.4).
	Tags        []tags.KV    `json:"tags,omitempty"`
	StreamID    string       `json:"streamId"`
	Description string       `json:"description,omitempty"`
	RoleARN     string       `json:"roleArn,omitempty"`
	Files       []StreamFile `json:"files"`
}

func (b *InMemoryBackend) CreateStream(input *CreateStreamInput) (*IoTStream, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.streams.Has(input.StreamID) {
		return nil, fmt.Errorf("stream %q already exists: %w", input.StreamID, ErrAlreadyExists)
	}
	now := float64(time.Now().Unix())
	s := &IoTStream{
		StreamID:      input.StreamID,
		StreamARN:     b.streamARN(input.StreamID),
		Description:   input.Description,
		RoleARN:       input.RoleARN,
		Files:         append([]StreamFile(nil), input.Files...),
		Tags:          tags.MapFromKV(input.Tags),
		StreamVersion: 1,
		CreatedAt:     now,
		LastUpdated:   now,
	}
	b.streams.Put(s)
	b.putResourceTagsLocked(s.StreamARN, s.Tags)

	return cloneStream(s), nil
}

func (b *InMemoryBackend) DescribeStream(id string) (*IoTStream, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	s, ok := b.streams.Get(id)
	if !ok {
		return nil, fmt.Errorf("stream %q not found: %w", id, ErrResourceNotFound)
	}

	return cloneStream(s), nil
}

func (b *InMemoryBackend) ListStreams() []*IoTStream {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]*IoTStream, 0, b.streams.Len())
	for _, v := range b.streams.Snapshot() {
		out = append(out, cloneStream(v))
	}

	return out
}

func (b *InMemoryBackend) UpdateStream(id, description, roleARN string, files []StreamFile) (*IoTStream, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.streams.Get(id)
	if !ok {
		return nil, fmt.Errorf("stream %q not found: %w", id, ErrResourceNotFound)
	}
	if description != "" {
		s.Description = description
	}
	if roleARN != "" {
		s.RoleARN = roleARN
	}
	if len(files) > 0 {
		s.Files = files
	}
	s.StreamVersion++
	s.LastUpdated = float64(time.Now().Unix())

	return cloneStream(s), nil
}

func (b *InMemoryBackend) DeleteStream(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.streams.Has(id) {
		return fmt.Errorf("stream %q not found: %w", id, ErrResourceNotFound)
	}
	b.streams.Delete(id)
	delete(b.resourceTags, b.streamARN(id))

	return nil
}
