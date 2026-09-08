package cloudwatch_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

func TestCloudWatchBackend_NewInMemoryBackend(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NotNil(t, b)
}

// TestCloudWatchBackend_Reset_TotalMetrics verifies that Reset() resyncs the
// totalMetrics running counter (#60) with the cleared b.metrics map, mirroring
// the fix already applied on the Restore path.
func TestCloudWatchBackend_Reset_TotalMetrics(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()

	b.StoreDatumForTest("NS", cloudwatch.MetricDatum{MetricName: "m1"})
	b.StoreDatumForTest("NS", cloudwatch.MetricDatum{MetricName: "m2"})
	require.Equal(t, 2, b.TotalMetricsForTest())

	b.Reset()

	assert.Equal(t, 0, b.TotalMetricsForTest(), "totalMetrics must resync to 0 after Reset")

	// A stale, too-high counter would wrongly reject writes under
	// cwMaxTotalMetricRecords; confirm the post-Reset backend can still
	// accept new metric series.
	b.StoreDatumForTest("NS", cloudwatch.MetricDatum{MetricName: "m3"})
	assert.Equal(t, 1, b.TotalMetricsForTest())
}

// ---------------------------------------------------------------------------
// Dataset: GetDataset / AssociateDatasetKmsKey / DisassociateDatasetKmsKey
// ---------------------------------------------------------------------------

const validDatasetKmsKeyArn = "arn:aws:kms:us-east-1:123456789012:key/1234abcd-12ab-34cd-56ef-1234567890ab"

func TestCloudWatchBackend_GetDataset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		identifier string
		wantErr    bool
	}{
		{identifier: "default"},
		{
			identifier: "arn:aws:cloudwatch:us-east-1:123456789012:dataset/default",
		},
		{identifier: "not-default", wantErr: true},
		{identifier: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.identifier, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

			ds, err := b.GetDataset(tt.identifier)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, cloudwatch.ErrDatasetNotFound)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, "default", ds.DatasetID)
			assert.Contains(t, ds.Arn, "dataset/default")
			assert.Empty(t, ds.KmsKeyArn, "default dataset has no KMS key until associated")
		})
	}
}

func TestCloudWatchBackend_AssociateDatasetKmsKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kmsKeyArn string
		name      string
		wantErr   bool
	}{
		{name: "valid_key_arn", kmsKeyArn: validDatasetKmsKeyArn},
		{name: "empty", kmsKeyArn: "", wantErr: true},
		{
			name:      "alias_arn_rejected",
			kmsKeyArn: "arn:aws:kms:us-east-1:123456789012:alias/my-key",
			wantErr:   true,
		},
		{name: "bare_key_id_rejected", kmsKeyArn: "1234abcd-12ab-34cd-56ef-1234567890ab", wantErr: true},
		{name: "not_a_kms_arn", kmsKeyArn: "not-an-arn", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

			err := b.AssociateDatasetKmsKey("default", tt.kmsKeyArn)
			if tt.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)

			ds, getErr := b.GetDataset("default")
			require.NoError(t, getErr)
			assert.Equal(t, tt.kmsKeyArn, ds.KmsKeyArn)
		})
	}
}

func TestCloudWatchBackend_DisassociateDatasetKmsKey(t *testing.T) {
	t.Parallel()

	t.Run("no_key_associated", func(t *testing.T) {
		t.Parallel()

		b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
		err := b.DisassociateDatasetKmsKey("default")
		require.Error(t, err)
		assert.ErrorIs(t, err, cloudwatch.ErrDatasetNotFound)
	})

	t.Run("removes_associated_key", func(t *testing.T) {
		t.Parallel()

		b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
		require.NoError(t, b.AssociateDatasetKmsKey("default", validDatasetKmsKeyArn))

		require.NoError(t, b.DisassociateDatasetKmsKey("default"))

		ds, err := b.GetDataset("default")
		require.NoError(t, err)
		assert.Empty(t, ds.KmsKeyArn)
	})
}

// ---------------------------------------------------------------------------
// OTel enrichment: GetOTelEnrichment / StartOTelEnrichment / StopOTelEnrichment
// ---------------------------------------------------------------------------

func TestCloudWatchBackend_OTelEnrichment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action     func(b *cloudwatch.InMemoryBackend) error
		name       string
		wantStatus string
	}{
		{
			name:       "default_is_stopped",
			action:     func(*cloudwatch.InMemoryBackend) error { return nil },
			wantStatus: "Stopped",
		},
		{
			name:       "start_sets_running",
			action:     (*cloudwatch.InMemoryBackend).StartOTelEnrichment,
			wantStatus: "Running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
			require.NoError(t, tt.action(b))

			status, err := b.GetOTelEnrichment()
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, status)
		})
	}
}

func TestCloudWatchBackend_OTelEnrichment_StartThenStop(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1")

	require.NoError(t, b.StartOTelEnrichment())
	status, err := b.GetOTelEnrichment()
	require.NoError(t, err)
	assert.Equal(t, "Running", status)

	require.NoError(t, b.StopOTelEnrichment())
	status2, err2 := b.GetOTelEnrichment()
	require.NoError(t, err2)
	assert.Equal(t, "Stopped", status2)
}
