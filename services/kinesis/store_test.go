package kinesis_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/kinesis"
)

// TestMultipleResetCycle verifies repeated Reset() calls are safe.
func TestMultipleResetCycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	b := h.Backend.(*kinesis.InMemoryBackend)

	for range 3 {
		doRequest(t, h, "CreateStream", map[string]any{"StreamName": "rc-stream", "ShardCount": 1})
		b.Reset()
		assert.Equal(t, 0, b.StreamCount())
	}
}

// TestReset_MinimumThroughputBillingCommitment verifies that Reset() restores
// the account's billing commitment to DISABLED, not the zero value (empty
// status string).
func TestReset_MinimumThroughputBillingCommitment(t *testing.T) {
	t.Parallel()

	b := kinesis.NewInMemoryBackend()
	ctx := context.Background()

	before, err := b.DescribeAccountSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "DISABLED", before.MinimumThroughputBillingCommitment.Status)

	_, err = b.UpdateAccountSettings(ctx, &kinesis.UpdateAccountSettingsInput{
		MinimumThroughputBillingCommitment: &kinesis.MinimumThroughputBillingCommitmentInput{
			Status: "ENABLED",
		},
	})
	require.NoError(t, err)

	enabled, err := b.DescribeAccountSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "ENABLED", enabled.MinimumThroughputBillingCommitment.Status)

	b.Reset()

	after, err := b.DescribeAccountSettings(ctx)
	require.NoError(t, err)
	assert.Equal(
		t,
		"DISABLED",
		after.MinimumThroughputBillingCommitment.Status,
		"reset must restore DISABLED, not the zero value",
	)
	assert.True(t, after.MinimumThroughputBillingCommitment.StartedAt.IsZero())
}

// TestSeedHelper verifies AddStreamInternal works.
func TestSeedHelper(t *testing.T) {
	t.Parallel()

	b := kinesis.NewInMemoryBackend()
	assert.Equal(t, 0, b.StreamCount())

	b.AddStreamInternal(&kinesis.Stream{
		Name:   "seeded-stream",
		ARN:    "arn:aws:kinesis:us-east-1:123:stream/seeded-stream",
		Status: "ACTIVE",
	})

	assert.Equal(t, 1, b.StreamCount())
}

func TestAddStreamInternal_DefaultsStreamMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		streamName string
		streamMode string
		wantMode   string
	}{
		{
			name:       "empty_mode_defaults_to_provisioned",
			streamName: "internal-stream-1",
			streamMode: "",
			wantMode:   "PROVISIONED",
		},
		{name: "on_demand_preserved", streamName: "internal-stream-2", streamMode: "ON_DEMAND", wantMode: "ON_DEMAND"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := kinesis.NewInMemoryBackend()

			b.AddStreamInternal(&kinesis.Stream{
				Name:       tt.streamName,
				ARN:        "arn:aws:kinesis:us-east-1:123456789012:stream/" + tt.streamName,
				Status:     "ACTIVE",
				StreamMode: tt.streamMode,
			})

			out, err := b.DescribeStream(context.Background(), &kinesis.DescribeStreamInput{StreamName: tt.streamName})
			require.NoError(t, err)
			assert.Equal(t, tt.wantMode, out.StreamMode)
		})
	}
}

func TestListAll(t *testing.T) {
	t.Parallel()

	bk := kinesis.NewInMemoryBackend()

	// Empty
	assert.Empty(t, bk.ListAll(context.Background()))

	// Create some streams
	require.NoError(t, bk.CreateStream(context.Background(), &kinesis.CreateStreamInput{StreamName: "s1"}))
	require.NoError(t, bk.CreateStream(context.Background(), &kinesis.CreateStreamInput{StreamName: "s2"}))

	all := bk.ListAll(context.Background())
	assert.Len(t, all, 2)

	names := make([]string, len(all))
	for i, s := range all {
		names[i] = s.Name
		assert.NotEmpty(t, s.ARN)
		assert.NotEmpty(t, s.Status)
	}

	assert.ElementsMatch(t, []string{"s1", "s2"}, names)
}

func TestBackendWithConfig(t *testing.T) {
	t.Parallel()

	bk := kinesis.NewInMemoryBackendWithConfig("123456789012", "eu-west-1")
	require.NoError(t, bk.CreateStream(context.Background(), &kinesis.CreateStreamInput{StreamName: "regional-stream"}))

	all := bk.ListAll(context.Background())
	require.Len(t, all, 1)
	assert.Contains(t, all[0].ARN, "eu-west-1")
	assert.Contains(t, all[0].ARN, "123456789012")
}
