package azureblob_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/azureblob"
)

func TestSnapshotRestore_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "roundtrip_container_and_blob"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			b := azureblob.NewInMemoryBackend()
			require.NoError(t, b.CreateContainer("c1"))
			_, err := b.PutBlob("c1", "blob1", []byte("payload"), "text/plain")
			require.NoError(t, err)

			data := b.Snapshot(ctx)
			require.NotEmpty(t, data, tt.name)

			restored := azureblob.NewInMemoryBackend()
			require.NoError(t, restored.Restore(ctx, data))

			containers := restored.ListContainers()
			require.Len(t, containers, 1, tt.name)
			assert.Equal(t, "c1", containers[0].Name, tt.name)

			_, blobData, err := restored.GetBlob("c1", "blob1")
			require.NoError(t, err, tt.name)
			assert.Equal(t, "payload", string(blobData), tt.name)
		})
	}
}

// TestRestore_EtagSeqReseeded is a regression test: etagSeq (store.go) is
// process-local, not part of backendSnapshot, so a naive Restore leaves it at
// its zero value. That reintroduces the exact collision
// TestInMemoryBackend_PutBlobOverwrites/identical_content_still_changes_etag
// guards against in-process -- the first Put Blob after Restore reused seq=1,
// the same value the pre-restore process's first-ever Put Blob used, so
// overwriting a blob with byte-identical content right after a restore
// reproduced the pre-restart ETag exactly.
func TestRestore_EtagSeqReseeded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{name: "identical_overwrite_after_restore_gets_new_etag", data: "same-bytes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			b := azureblob.NewInMemoryBackend()
			require.NoError(t, b.CreateContainer("c1"))
			before, err := b.PutBlob("c1", "blob1", []byte(tt.data), "")
			require.NoError(t, err, tt.name)

			snap := b.Snapshot(ctx)

			restored := azureblob.NewInMemoryBackend()
			require.NoError(t, restored.Restore(ctx, snap), tt.name)

			after, err := restored.PutBlob("c1", "blob1", []byte(tt.data), "")
			require.NoError(t, err, tt.name)

			assert.NotEqual(t, before.ETag, after.ETag, tt.name)
		})
	}
}

func TestRestore_IncompatibleVersionStartsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
	}{
		{name: "garbage_bytes_discarded", data: []byte(`{"version":999,"containers":{}}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			b := azureblob.NewInMemoryBackend()
			require.NoError(t, b.CreateContainer("preexisting"))

			require.NoError(t, b.Restore(ctx, tt.data))

			assert.Empty(t, b.ListContainers(), tt.name)
		})
	}
}

// TestRestore_RejectsNullEntries is a regression test: a JSON `null` value
// inside "containers" or a container's "Blobs" decodes to a nil pointer
// without a JSON-unmarshal error, and previously nothing checked for that
// before storing it -- the first thing to dereference it later
// (storedContainer.Blobs, or storedBlob.info() for a null blob) would panic.
// Restore must reject the whole snapshot instead.
func TestRestore_RejectsNullEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		name    string
		data    []byte
	}{
		{
			name:    "null_container",
			data:    []byte(`{"version":1,"containers":{"c1":null}}`),
			wantErr: azureblob.ErrSnapshotContainerNull,
		},
		{
			name:    "null_blob",
			data:    []byte(`{"version":1,"containers":{"c1":{"Name":"c1","Blobs":{"b1":null}}}}`),
			wantErr: azureblob.ErrSnapshotBlobNull,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			b := azureblob.NewInMemoryBackend()
			require.NoError(t, b.CreateContainer("preexisting"))

			err := b.Restore(ctx, tt.data)
			require.ErrorIs(t, err, tt.wantErr, tt.name)

			// A rejected snapshot must not have partially mutated state.
			containers := b.ListContainers()
			require.Len(t, containers, 1, tt.name)
			assert.Equal(t, "preexisting", containers[0].Name, tt.name)
		})
	}
}

func TestHandlerSnapshotRestore_Delegates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "handler_delegates_to_backend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			backend := azureblob.NewInMemoryBackend()
			h := azureblob.NewHandler(backend)
			require.NoError(t, backend.CreateContainer("c1"))

			data := h.Snapshot(ctx)
			require.NotEmpty(t, data, tt.name)

			restoredBackend := azureblob.NewInMemoryBackend()
			restoredHandler := azureblob.NewHandler(restoredBackend)
			require.NoError(t, restoredHandler.Restore(ctx, data))

			assert.Len(t, restoredBackend.ListContainers(), 1, tt.name)
		})
	}
}
