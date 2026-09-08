package iotanalytics_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iotanalytics"
)

// newPersistenceTestBackend creates a backend with one populated entry in
// every store.Table (channels, datastores, datasets, pipelines) plus every
// raw map left un-converted (tags -- populated implicitly by the Create*
// calls below, channelMessages, datasetContents) and the singleton
// loggingOptions field, so a Snapshot from it exercises the entire persisted
// surface of the backend.
func newPersistenceTestBackend(t *testing.T) *iotanalytics.InMemoryBackend {
	t.Helper()

	b := iotanalytics.NewInMemoryBackend()
	ctx := context.Background()

	_, err := b.CreateChannel(ctx, "ch1", map[string]string{"env": "test"}, nil, nil)
	require.NoError(t, err)

	_, err = b.CreateDatastore(ctx, "ds1", map[string]string{"team": "iot"}, nil, nil, nil, nil)
	require.NoError(t, err)

	dataset, err := b.CreateDataset(ctx, "dataset1", map[string]string{"owner": "me"}, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)

	_, err = b.CreatePipeline(ctx, "pipeline1", map[string]string{"stage": "prod"}, []iotanalytics.PipelineActivity{
		{Channel: &iotanalytics.PipelineChannelActivity{ChannelName: "ch1", Name: "chAct"}},
		{Datastore: &iotanalytics.PipelineDatastoreActivity{DatastoreName: "ds1", Name: "dsAct"}},
	})
	require.NoError(t, err)

	// Populate channelMessages (raw map left un-converted) by driving
	// BatchPutMessage through the Handler -- the backend method's message
	// type is unexported, so the HTTP path is how an external test reaches it.
	h := iotanalytics.NewHandler(b)
	rec := doRequest(t, h, http.MethodPost, "/messages/batch", map[string]any{
		"channelName": "ch1",
		"messages": []map[string]any{
			{"messageId": "m1", "payload": []byte("hello")},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Populate datasetContents (raw map left un-converted).
	_, err = b.CreateDatasetContent(dataset.Name, "")
	require.NoError(t, err)

	// Populate loggingOptions (singleton field).
	require.NoError(t, b.PutLoggingOptions(&iotanalytics.LoggingOptions{
		RoleARN: "arn:aws:iam::000000000000:role/logging",
		Level:   "ERROR",
		Enabled: true,
	}))

	return b
}

// TestInMemoryBackend_SnapshotRestore_FullState round-trips every store.Table
// (channels, datastores, datasets, pipelines) and every raw map left
// un-converted (tags, channelMessages, datasetContents), plus the singleton
// loggingOptions field, through Snapshot -> Restore into a fresh backend.
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := newPersistenceTestBackend(t)

	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := iotanalytics.NewInMemoryBackend()
	require.NoError(t, fresh.Restore(t.Context(), snap))

	// channels table.
	channels := fresh.ListChannels()
	require.Len(t, channels, 1)
	assert.Equal(t, "ch1", channels[0].Name)

	// datastores table.
	datastores := fresh.ListDatastores()
	require.Len(t, datastores, 1)
	assert.Equal(t, "ds1", datastores[0].Name)

	// datasets table.
	datasets := fresh.ListDatasets()
	require.Len(t, datasets, 1)
	assert.Equal(t, "dataset1", datasets[0].Name)

	// pipelines table.
	pipelines := fresh.ListPipelines()
	require.Len(t, pipelines, 1)
	assert.Equal(t, "pipeline1", pipelines[0].Name)
	require.Len(t, pipelines[0].Activities, 2)
	require.NotNil(t, pipelines[0].Activities[0].Channel)
	assert.Equal(t, "ch1", pipelines[0].Activities[0].Channel.ChannelName)

	// tags raw map (keyed by ARN, populated by the Create* calls above).
	tags, err := fresh.ListTagsForResource(channels[0].ARN)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, iotanalytics.TagDTO{Key: "env", Value: "test"}, tags[0])

	// channelMessages raw map.
	msgs, err := fresh.SampleChannelData("ch1", 10, false, 0, false, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, []byte("hello"), msgs[0])

	// datasetContents raw map.
	contents, err := fresh.ListDatasetContents("dataset1")
	require.NoError(t, err)
	require.Len(t, contents, 1)

	// loggingOptions singleton field.
	opts, err := fresh.DescribeLoggingOptions()
	require.NoError(t, err)
	assert.True(t, opts.Enabled)
	assert.Equal(t, "arn:aws:iam::000000000000:role/logging", opts.RoleARN)
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend (including the pre-Phase-3.3
// format, which decodes with Version == 0) is discarded cleanly rather than
// partially decoded: the backend resets to empty state and Restore returns
// no error.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b := newPersistenceTestBackend(t)

	err := b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.Zero(t, iotanalytics.ChannelCount(b))
	assert.Zero(t, iotanalytics.DatastoreCount(b))
	assert.Zero(t, iotanalytics.DatasetCount(b))
	assert.Zero(t, iotanalytics.PipelineCount(b))

	assert.Empty(t, b.ListChannels())

	_, err = b.DescribeLoggingOptions()
	require.ErrorIs(t, err, iotanalytics.ErrLoggingOptionsNotFound)
}

// TestInMemoryBackend_RestoreInvalidData verifies malformed JSON surfaces as
// an error rather than being silently discarded (that path is reserved for a
// syntactically valid but version-mismatched snapshot; see
// TestInMemoryBackend_RestoreVersionMismatch).
func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := iotanalytics.NewInMemoryBackend()
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestHandler_SnapshotRestoreDelegate verifies Handler.Snapshot/Restore
// delegate to the backend (the wiring cli.go's generic setupPersistence
// relies on).
func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	h := iotanalytics.NewHandler(newPersistenceTestBackend(t))

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := iotanalytics.NewHandler(iotanalytics.NewInMemoryBackend())
	require.NoError(t, h2.Restore(t.Context(), snap))

	channels := h2.Backend.ListChannels()
	require.Len(t, channels, 1)
	assert.Equal(t, "ch1", channels[0].Name)
}

// TestInMemoryBackend_PersistenceRoundTrip is a lighter-weight companion to
// TestInMemoryBackend_SnapshotRestore_FullState: it seeds one of each resource family via the
// AddXInternal test helpers (rather than driving every field through Create*) and checks only
// the per-family counts plus one representative field round-trip.
func TestInMemoryBackend_PersistenceRoundTrip(t *testing.T) {
	t.Parallel()

	b := iotanalytics.NewInMemoryBackend()
	b.AddChannelInternal("saved_ch")
	b.AddDatastoreInternal("saved_ds")
	b.AddDatasetInternal("saved_set")
	b.AddPipelineInternal("saved_pipe")

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)
	require.NotEmpty(t, snap)

	b2 := iotanalytics.NewInMemoryBackend()
	err := b2.Restore(t.Context(), snap)
	require.NoError(t, err)

	assert.Equal(t, 1, iotanalytics.ChannelCount(b2))
	assert.Equal(t, 1, iotanalytics.DatastoreCount(b2))
	assert.Equal(t, 1, iotanalytics.DatasetCount(b2))
	assert.Equal(t, 1, iotanalytics.PipelineCount(b2))

	ch, err := b2.DescribeChannel("saved_ch")
	require.NoError(t, err)
	assert.Equal(t, "saved_ch", ch.Name)
}
