package awsconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/awsconfig"
)

func TestAWSConfigBackend_PutDeliveryChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		chanName   string
		bucket     string
		topic      string
		wantName   string
		wantBucket string
		wantLen    int
	}{
		{
			name:       "success",
			chanName:   "default",
			bucket:     "my-bucket",
			topic:      "arn:aws:sns:us-east-1:000000000000:my-topic",
			wantLen:    1,
			wantName:   "default",
			wantBucket: "my-bucket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			err := b.PutDeliveryChannel(tt.chanName, tt.bucket, tt.topic, "", nil)
			require.NoError(t, err)

			channels := b.DescribeDeliveryChannels(nil)
			require.Len(t, channels, tt.wantLen)
			assert.Equal(t, tt.wantName, channels[0].Name)
			assert.Equal(t, tt.wantBucket, channels[0].S3Bucket)
		})
	}
}

func TestAWSConfigBackend_DescribeDeliveryChannels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(t *testing.T, b *awsconfig.InMemoryBackend)
		name      string
		wantCount int
	}{
		{
			name:      "empty",
			wantCount: 0,
		},
		{
			name: "one_channel",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(
					t,
					b.PutDeliveryChannel(
						"default",
						"my-bucket",
						"arn:aws:sns:us-east-1:000000000000:my-topic",
						"",
						nil,
					),
				)
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, b)
			}

			channels := b.DescribeDeliveryChannels(nil)
			assert.Len(t, channels, tt.wantCount)
		})
	}
}

func TestAWSConfigBackend_PutDeliveryChannel_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr  error
		name     string
		chanName string
		bucket   string
	}{
		{
			name:     "empty_name_fails",
			chanName: "",
			bucket:   "my-bucket",
			wantErr:  awsconfig.ErrInvalidDeliveryChannelName,
		},
		{
			name:     "empty_bucket_fails",
			chanName: "default",
			bucket:   "",
			wantErr:  awsconfig.ErrValidation,
		},
		{
			name:     "success",
			chanName: "default",
			bucket:   "my-bucket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			err := b.PutDeliveryChannel(tt.chanName, tt.bucket, "", "", nil)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestDescribeDeliveryChannelStatus(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	_ = b.PutDeliveryChannel("chan1", "my-bucket", "", "", nil)

	statuses := b.DescribeDeliveryChannelStatus(nil)
	if len(statuses) != 1 || statuses[0].Name != "chan1" {
		t.Fatalf("DescribeDeliveryChannelStatus: %v", statuses)
	}
}

func TestDescribeDeliveryChannelStatus_Filtered(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()
	_ = b.PutDeliveryChannel("chan1", "bucket1", "", "", nil)
	_ = b.PutDeliveryChannel("chan2", "bucket2", "", "", nil)

	statuses := b.DescribeDeliveryChannelStatus([]string{"chan1"})
	if len(statuses) != 1 || statuses[0].Name != "chan1" {
		t.Fatalf("expected 1 status, got %v", statuses)
	}
}

func TestAWSConfigBackend_DeleteDeliveryChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup   func(t *testing.T, b *awsconfig.InMemoryBackend)
		name    string
		delName string
		wantErr bool
	}{
		{
			name: "success",
			setup: func(t *testing.T, b *awsconfig.InMemoryBackend) {
				t.Helper()
				require.NoError(t, b.PutDeliveryChannel("ch1", "bucket", "", "", nil))
			},
			delName: "ch1",
		},
		{
			name:    "not_found",
			delName: "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, b)
			}

			err := b.DeleteDeliveryChannel(tt.delName)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestAWSConfigBackend_DeleteDeliveryChannel_RejectedWhileRecorderActive
// locks real AWS's DeleteDeliveryChannel doc comment: "Before you can
// delete the delivery channel, you must stop the customer managed
// configuration recorder".
func TestAWSConfigBackend_DeleteDeliveryChannel_RejectedWhileRecorderActive(t *testing.T) {
	t.Parallel()

	b := awsconfig.NewInMemoryBackend()

	require.NoError(t, b.PutDeliveryChannel("ch-active", "bucket", "", "", nil))
	require.NoError(t, b.PutConfigurationRecorder("rec-active", "arn:aws:iam::000000000000:role/r", nil))
	require.NoError(t, b.StartConfigurationRecorder("rec-active"))

	err := b.DeleteDeliveryChannel("ch-active")
	require.ErrorIs(t, err, awsconfig.ErrLastDeliveryChannelDeleteFailed)

	require.NoError(t, b.StopConfigurationRecorder("rec-active"))
	require.NoError(t, b.DeleteDeliveryChannel("ch-active"))
}

func TestAWSConfigBackend_DeliverConfigSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("unknown_channel_errors", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		_, err := b.DeliverConfigSnapshot("nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrNoSuchDeliveryChannel)
	})

	t.Run("no_recorder_configured_errors", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutDeliveryChannel("chan", "bucket", "", "", nil))

		_, err := b.DeliverConfigSnapshot("chan")
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrNoAvailableConfigurationRecorder)
	})

	t.Run("no_running_recorder_errors", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutDeliveryChannel("chan", "bucket", "", "", nil))
		require.NoError(t, b.PutConfigurationRecorder("rec", "arn:aws:iam::123:role/r", nil))

		_, err := b.DeliverConfigSnapshot("chan")
		require.Error(t, err)
		assert.ErrorIs(t, err, awsconfig.ErrNoRunningConfigurationRecorder)
	})

	t.Run("running_recorder_returns_snapshot_id", func(t *testing.T) {
		t.Parallel()

		b := awsconfig.NewInMemoryBackend()
		require.NoError(t, b.PutDeliveryChannel("chan", "bucket", "", "", nil))
		require.NoError(t, b.PutConfigurationRecorder("rec", "arn:aws:iam::123:role/r", nil))
		require.NoError(t, b.StartConfigurationRecorder("rec"))

		id, err := b.DeliverConfigSnapshot("chan")
		require.NoError(t, err)
		assert.NotEmpty(t, id)

		// A second delivery produces a distinct snapshot ID.
		id2, err := b.DeliverConfigSnapshot("chan")
		require.NoError(t, err)
		assert.NotEqual(t, id, id2)
	})
}
