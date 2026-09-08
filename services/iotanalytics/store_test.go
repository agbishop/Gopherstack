package iotanalytics_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"

	"github.com/blackbirdworks/gopherstack/services/iotanalytics"
)

func TestCreateResourceARNsUseCtxbagRegionAndAccount(t *testing.T) {
	t.Parallel()

	ctx := awsmeta.Set(context.Background(), &awsmeta.Metadata{
		Account:   "222233334444",
		Region:    "eu-west-2",
		Partition: "aws",
	})

	b := iotanalytics.NewInMemoryBackend()

	ch, err := b.CreateChannel(ctx, "ch1", nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iotanalytics:eu-west-2:222233334444:channel/ch1", ch.ARN)

	ds, err := b.CreateDatastore(ctx, "ds1", nil, nil, nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iotanalytics:eu-west-2:222233334444:datastore/ds1", ds.ARN)

	p, err := b.CreatePipeline(ctx, "p1", nil, validPipelineActivities())
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iotanalytics:eu-west-2:222233334444:pipeline/p1", p.ARN)
}

func TestCreateResourceARNsFallBackToDefaultRegion(t *testing.T) {
	t.Parallel()

	b := iotanalytics.NewInMemoryBackend()

	// Background context carries no ctxbag: region falls back to the service
	// default and the account to the awsmeta default, keeping ARNs well-formed.
	ch, err := b.CreateChannel(context.Background(), "ch2", nil, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iotanalytics:us-east-1:000000000000:channel/ch2", ch.ARN)
}

// TestInMemoryBackend_Reset verifies Reset clears every resource family.
func TestInMemoryBackend_Reset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(b *iotanalytics.InMemoryBackend)
		name     string
		wantChan int
		wantDS   int
		wantSet  int
		wantPipe int
	}{
		{
			name: "empty_backend",
		},
		{
			name: "populated_backend_clears_all",
			setup: func(b *iotanalytics.InMemoryBackend) {
				b.AddChannelInternal("c1")
				b.AddDatastoreInternal("d1")
				b.AddDatasetInternal("s1")
				b.AddPipelineInternal("p1")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iotanalytics.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(b)
			}

			b.Reset()

			assert.Equal(t, 0, iotanalytics.ChannelCount(b))
			assert.Equal(t, 0, iotanalytics.DatastoreCount(b))
			assert.Equal(t, 0, iotanalytics.DatasetCount(b))
			assert.Equal(t, 0, iotanalytics.PipelineCount(b))
		})
	}
}

// TestInMemoryBackend_MultipleResetCycle verifies repeated Reset calls keep clearing state.
func TestInMemoryBackend_MultipleResetCycle(t *testing.T) {
	t.Parallel()

	b := iotanalytics.NewInMemoryBackend()

	for i := range 3 {
		b.AddChannelInternal("ch")
		b.AddDatastoreInternal("ds")
		b.Reset()

		assert.Equal(t, 0, iotanalytics.ChannelCount(b), "iteration %d", i)
		assert.Equal(t, 0, iotanalytics.DatastoreCount(b), "iteration %d", i)
	}
}

// TestInMemoryBackend_SeedHelpers verifies the AddXInternal test-seed helpers for every family.
func TestInMemoryBackend_SeedHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		seedName string
		kind     string
	}{
		{name: "channel", seedName: "my_ch", kind: "channel"},
		{name: "datastore", seedName: "my_ds", kind: "datastore"},
		{name: "dataset", seedName: "my_set", kind: "dataset"},
		{name: "pipeline", seedName: "my_pipe", kind: "pipeline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iotanalytics.NewInMemoryBackend()

			switch tt.kind {
			case "channel":
				ch := b.AddChannelInternal(tt.seedName)
				require.NotNil(t, ch)
				assert.Equal(t, tt.seedName, ch.Name)
				assert.Equal(t, 1, iotanalytics.ChannelCount(b))
			case "datastore":
				ds := b.AddDatastoreInternal(tt.seedName)
				require.NotNil(t, ds)
				assert.Equal(t, tt.seedName, ds.Name)
				assert.Equal(t, 1, iotanalytics.DatastoreCount(b))
			case "dataset":
				d := b.AddDatasetInternal(tt.seedName)
				require.NotNil(t, d)
				assert.Equal(t, tt.seedName, d.Name)
				assert.Equal(t, 1, iotanalytics.DatasetCount(b))
			case "pipeline":
				p := b.AddPipelineInternal(tt.seedName)
				require.NotNil(t, p)
				assert.Equal(t, tt.seedName, p.Name)
				assert.Equal(t, 1, iotanalytics.PipelineCount(b))
			}
		})
	}
}

// TestInMemoryBackend_ExportCountHelpers exercises the white-box count helpers across families.
func TestInMemoryBackend_ExportCountHelpers(t *testing.T) {
	t.Parallel()

	b := iotanalytics.NewInMemoryBackend()

	assert.Equal(t, 0, iotanalytics.ChannelCount(b))
	assert.Equal(t, 0, iotanalytics.DatastoreCount(b))
	assert.Equal(t, 0, iotanalytics.DatasetCount(b))
	assert.Equal(t, 0, iotanalytics.PipelineCount(b))

	b.AddChannelInternal("c1")
	b.AddChannelInternal("c2")
	assert.Equal(t, 2, iotanalytics.ChannelCount(b))

	b.AddDatastoreInternal("d1")
	assert.Equal(t, 1, iotanalytics.DatastoreCount(b))

	b.AddDatasetInternal("s1")
	assert.Equal(t, 1, iotanalytics.DatasetCount(b))

	b.AddPipelineInternal("p1")
	assert.Equal(t, 1, iotanalytics.PipelineCount(b))
}

// TestInMemoryBackend_ErrSentinels verifies sentinel error messages.
func TestInMemoryBackend_ErrSentinels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{
			name:    "ErrValidation",
			err:     iotanalytics.ErrValidation,
			wantMsg: "validation error",
		},
		{
			name:    "ErrAlreadyExists",
			err:     iotanalytics.ErrAlreadyExists,
			wantMsg: "resource already exists",
		},
		{
			name:    "ErrResourceNotFound",
			err:     iotanalytics.ErrResourceNotFound,
			wantMsg: "resource not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Error(t, tt.err)
			assert.Contains(t, tt.err.Error(), tt.wantMsg)
		})
	}
}

// TestInMemoryBackend_ErrAlreadyExists_Direct verifies that creating a duplicate resource via
// the backend directly (not through the HTTP handler) returns ErrAlreadyExists, for every
// resource family.
func TestInMemoryBackend_ErrAlreadyExists_Direct(t *testing.T) {
	t.Parallel()

	tests := []struct {
		create func(b *iotanalytics.InMemoryBackend) error
		name   string
	}{
		{
			name: "channel",
			create: func(b *iotanalytics.InMemoryBackend) error {
				_, err := b.CreateChannel(context.Background(), "dup", nil, nil, nil)

				return err
			},
		},
		{
			name: "datastore",
			create: func(b *iotanalytics.InMemoryBackend) error {
				_, err := b.CreateDatastore(context.Background(), "dup", nil, nil, nil, nil, nil)

				return err
			},
		},
		{
			name: "dataset",
			create: func(b *iotanalytics.InMemoryBackend) error {
				_, err := b.CreateDataset(context.Background(), "dup", nil, nil, nil, nil, nil, nil, nil)

				return err
			},
		},
		{
			name: "pipeline",
			create: func(b *iotanalytics.InMemoryBackend) error {
				_, err := b.CreatePipeline(context.Background(), "dup", nil, validPipelineActivities())

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iotanalytics.NewInMemoryBackend()
			require.NoError(t, tt.create(b))

			err := tt.create(b)
			require.Error(t, err)
			assert.ErrorIs(t, err, iotanalytics.ErrAlreadyExists)
		})
	}
}

// TestInMemoryBackend_RetentionPeriodValidation exercises validateRetentionPeriod (shared by
// every resource family that accepts a RetentionPeriod) via CreateChannel.
func TestInMemoryBackend_RetentionPeriodValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		retention *iotanalytics.RetentionPeriod
		name      string
		wantErr   bool
	}{
		{
			name:      "nil_retention_ok",
			retention: nil,
			wantErr:   false,
		},
		{
			name:      "unlimited_ok",
			retention: &iotanalytics.RetentionPeriod{Unlimited: true},
			wantErr:   false,
		},
		{
			name:      "positive_days_ok",
			retention: &iotanalytics.RetentionPeriod{NumberOfDays: 30},
			wantErr:   false,
		},
		{
			name:      "zero_days_rejected",
			retention: &iotanalytics.RetentionPeriod{NumberOfDays: 0},
			wantErr:   true,
		},
		{
			name:      "negative_days_rejected",
			retention: &iotanalytics.RetentionPeriod{NumberOfDays: -1},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iotanalytics.NewInMemoryBackend()
			_, err := b.CreateChannel(context.Background(), "ret_ch", nil, nil, tt.retention)

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, iotanalytics.ErrValidation)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
