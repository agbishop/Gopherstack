package awsconfig

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// PutDeliveryChannel creates or updates a delivery channel. An empty/blank name
// errors InvalidDeliveryChannelNameException, matching real AWS Config's declared
// error model (verified against aws-sdk-go-v2/service/configservice's
// PutDeliveryChannel deserializer).
func (b *InMemoryBackend) PutDeliveryChannel(
	name, s3Bucket, snsArn, s3KeyPrefix string,
	props *DeliverySnapshotProperties,
) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: DeliveryChannel name is required", ErrInvalidDeliveryChannelName)
	}

	if s3Bucket == "" {
		// PutDeliveryChannel's declared set has no code for a missing s3BucketName:
		// NoSuchBucketException is "the specified bucket does not exist", not "required"
		// (configservice@v1.68.4 types/errors.go), and ValidationException/
		// InvalidParameterValueException aren't declared for this op either.
		return fmt.Errorf("%w: DeliveryChannel s3BucketName is required", ErrValidation)
	}

	b.mu.Lock("PutDeliveryChannel")
	defer b.mu.Unlock()

	b.channels.Put(&DeliveryChannel{
		Name:                             name,
		S3Bucket:                         s3Bucket,
		SNSArn:                           snsArn,
		S3KeyPrefix:                      s3KeyPrefix,
		ConfigSnapshotDeliveryProperties: props,
	})

	return nil
}

// DescribeDeliveryChannels returns delivery channels filtered by the provided name list.
// An empty/nil names list returns all channels sorted by name.
func (b *InMemoryBackend) DescribeDeliveryChannels(names []string) []DeliveryChannel {
	b.mu.RLock("DescribeDeliveryChannels")
	defer b.mu.RUnlock()

	out := make([]DeliveryChannel, 0, b.channels.Len())

	if len(names) == 0 {
		for _, c := range b.channels.All() {
			out = append(out, *c)
		}
	} else {
		for _, n := range names {
			if c, ok := b.channels.Get(n); ok {
				out = append(out, *c)
			}
		}
	}

	slices.SortFunc(out, func(a, b DeliveryChannel) int {
		if a.Name < b.Name {
			return -1
		}

		if a.Name > b.Name {
			return 1
		}

		return 0
	})

	return out
}

// DeleteDeliveryChannel removes a delivery channel by name. Real AWS:
// "Before you can delete the delivery channel, you must stop the customer
// managed configuration recorder".
func (b *InMemoryBackend) DeleteDeliveryChannel(name string) error {
	if name == "" {
		// Declared set is LastDeliveryChannelDeleteFailedException/
		// NoSuchDeliveryChannelException only -- no validation-shaped code fits an
		// empty name (configservice@v1.68.4 deserializers.go).
		return fmt.Errorf("%w: DeliveryChannelName is required", ErrValidation)
	}

	b.mu.Lock("DeleteDeliveryChannel")
	defer b.mu.Unlock()

	if !b.channels.Has(name) {
		return fmt.Errorf("%w: %s", ErrNoSuchDeliveryChannel, name)
	}

	for _, r := range b.recorders.All() {
		if r.Status == recorderStatusActive {
			return fmt.Errorf(
				"%w: configuration recorder %s is still recording",
				ErrLastDeliveryChannelDeleteFailed, r.Name,
			)
		}
	}

	b.channels.Delete(name)

	return nil
}

// DeliverConfigSnapshot triggers an on-demand snapshot delivery through the
// named delivery channel and returns a generated snapshot ID, matching real
// AWS Config's DeliverConfigSnapshotOutput.ConfigSnapshotId. Errors (verified
// against aws-sdk-go-v2/service/configservice's DeliverConfigSnapshot
// deserializer, which declares NoSuchDeliveryChannelException/
// NoAvailableConfigurationRecorderException/NoRunningConfigurationRecorderException):
//   - NoSuchDeliveryChannelException when the named channel does not exist
//   - NoAvailableConfigurationRecorderException when no configuration recorder
//     has ever been created
//   - NoRunningConfigurationRecorderException when recorders exist but none is
//     currently ACTIVE
func (b *InMemoryBackend) DeliverConfigSnapshot(channelName string) (string, error) {
	b.mu.Lock("DeliverConfigSnapshot")
	defer b.mu.Unlock()

	if !b.channels.Has(channelName) {
		return "", fmt.Errorf("%w: %s", ErrNoSuchDeliveryChannel, channelName)
	}

	if b.recorders.Len() == 0 {
		return "", fmt.Errorf("%w: no configuration recorder configured", ErrNoAvailableConfigurationRecorder)
	}

	running := false

	for _, r := range b.recorders.All() {
		if r.Status == recorderStatusActive {
			running = true

			break
		}
	}

	if !running {
		return "", fmt.Errorf("%w: no configuration recorder is running", ErrNoRunningConfigurationRecorder)
	}

	b.captureCounter++

	snapshotID := fmt.Sprintf(
		"%08x-0000-0000-0000-%012d",
		time.Now().Unix(), b.captureCounter,
	)

	return snapshotID, nil
}

// DescribeDeliveryChannelStatus returns statuses for delivery channels.
// If names is empty, all channels are returned.
func (b *InMemoryBackend) DescribeDeliveryChannelStatus(names []string) []DeliveryChannelStatus {
	b.mu.RLock("DescribeDeliveryChannelStatus")
	defer b.mu.RUnlock()

	var channelNames []string

	if len(names) == 0 {
		for _, c := range b.channels.All() {
			channelNames = append(channelNames, c.Name)
		}
	} else {
		for _, name := range names {
			if b.channels.Has(name) {
				channelNames = append(channelNames, name)
			}
		}
	}

	out := make([]DeliveryChannelStatus, 0, len(channelNames))

	for _, name := range channelNames {
		out = append(out, DeliveryChannelStatus{
			Name:                      name,
			ConfigHistoryDeliveryInfo: &DeliveryChannelStatusInfo{LastStatus: recorderStatusSuccess},
			ConfigStreamDeliveryInfo:  &DeliveryChannelStatusInfo{LastStatus: recorderStatusSuccess},
		})
	}

	return out
}
