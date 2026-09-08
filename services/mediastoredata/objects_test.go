//go:build !integration

package mediastoredata_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/mediastoredata"
)

func TestInMemoryBackend_PutObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errSentinel      error
		name             string
		path             string
		contentType      string
		storageClass     string
		wantStorageClass string
		body             []byte
		wantErr          bool
	}{
		{
			name:             "stores_object_successfully",
			path:             "/video/clip.mp4",
			body:             []byte("video content"),
			contentType:      "video/mp4",
			storageClass:     "TEMPORAL",
			wantStorageClass: "TEMPORAL",
		},
		{
			name:        "empty_path_rejected",
			path:        "/",
			body:        []byte("data"),
			wantErr:     true,
			errSentinel: mediastoredata.ErrInvalidPath,
		},
		{
			name:        "dotdot_path_rejected",
			path:        "/a/../b",
			body:        []byte("data"),
			wantErr:     true,
			errSentinel: mediastoredata.ErrInvalidPath,
		},
		{
			name:        "path_too_long_rejected",
			path:        "/" + strings.Repeat("a", 901),
			body:        []byte("data"),
			wantErr:     true,
			errSentinel: mediastoredata.ErrInvalidPath,
		},
		{
			name:         "invalid_storage_class_rejected",
			path:         "/valid/path.mp4",
			body:         []byte("data"),
			storageClass: "GLACIER",
			wantErr:      true,
			errSentinel:  mediastoredata.ErrInvalidStorageClass,
		},
		{
			// "STANDARD" is a valid x-amz-upload-availability value but is NOT
			// a MediaStore Data StorageClass -- the only real StorageClass is
			// "TEMPORAL" (see aws-sdk-go-v2/service/mediastoredata/types.
			// StorageClass). Confusing the two must not silently succeed.
			name:         "standard_storage_class_rejected",
			path:         "/valid/path.mp4",
			body:         []byte("data"),
			storageClass: "STANDARD",
			wantErr:      true,
			errSentinel:  mediastoredata.ErrInvalidStorageClass,
		},
		{
			name:             "empty_storage_class_defaults_to_temporal",
			path:             "/valid/path.mp4",
			body:             []byte("data"),
			storageClass:     "",
			wantStorageClass: "TEMPORAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			obj, err := b.PutObject(context.Background(), tt.path, tt.body, tt.contentType, "", tt.storageClass, "")

			if tt.wantErr {
				require.Error(t, err)
				if tt.errSentinel != nil {
					require.ErrorIs(t, err, tt.errSentinel)
				}

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, obj.ETag)
			assert.NotEmpty(t, obj.SHA256)
			assert.Equal(t, tt.wantStorageClass, obj.StorageClass)
			assert.Equal(t, int64(len(tt.body)), obj.ContentLength)
		})
	}
}

func TestInMemoryBackend_PutObject_SizeLimit(t *testing.T) {
	t.Parallel()

	// aws-sdk-go-v2/service/mediastoredata/api_op_PutObject.go:13-14: "Object
	// sizes are limited to 25 MB for standard upload availability and 10 MB
	// for streaming upload availability."
	const (
		standardLimit  = 25 * 1024 * 1024
		streamingLimit = 10 * 1024 * 1024
	)

	tests := []struct {
		name               string
		uploadAvailability string
		bodySize           int
		wantErr            bool
	}{
		{name: "standard_at_limit_accepted", uploadAvailability: "STANDARD", bodySize: standardLimit},
		{
			name:               "standard_over_limit_rejected",
			uploadAvailability: "STANDARD",
			bodySize:           standardLimit + 1,
			wantErr:            true,
		},
		{name: "streaming_at_limit_accepted", uploadAvailability: "STREAMING", bodySize: streamingLimit},
		{
			name:               "streaming_over_limit_rejected",
			uploadAvailability: "STREAMING",
			bodySize:           streamingLimit + 1,
			wantErr:            true,
		},
		// A body over the streaming limit but under the standard limit must
		// still be rejected when upload availability is STREAMING -- the
		// limit depends on which availability was requested, not a single
		// global cap.
		{
			name:               "over_streaming_limit_but_under_standard_limit_still_rejected",
			uploadAvailability: "STREAMING", bodySize: streamingLimit + 1024,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			body := make([]byte, tt.bodySize)

			_, err := b.PutObject(
				context.Background(),
				"/size/file.mp4", body, "video/mp4", "", "TEMPORAL", tt.uploadAvailability,
			)

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, mediastoredata.ErrObjectTooLarge)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestInMemoryBackend_GetObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errSentinel error
		name        string
		putPath     string
		getPath     string
		body        []byte
		wantErr     bool
	}{
		{
			name:    "retrieves_existing_object",
			putPath: "/video/clip.mp4",
			getPath: "/video/clip.mp4",
			body:    []byte("hello world"),
		},
		{
			name:        "missing_object_not_found",
			putPath:     "",
			getPath:     "/missing/file.mp4",
			wantErr:     true,
			errSentinel: mediastoredata.ErrNotFound,
		},
		{
			name:        "invalid_path_rejected",
			getPath:     "/",
			wantErr:     true,
			errSentinel: mediastoredata.ErrInvalidPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.putPath != "" {
				_, err := b.PutObject(context.Background(), tt.putPath, tt.body, "video/mp4", "", "TEMPORAL", "")
				require.NoError(t, err)
			}

			obj, err := b.GetObject(context.Background(), tt.getPath)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errSentinel != nil {
					require.ErrorIs(t, err, tt.errSentinel)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.body, obj.Body)
		})
	}
}

func TestInMemoryBackend_DeleteObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errSentinel error
		name        string
		path        string
		createFirst bool
		wantErr     bool
	}{
		{
			name:        "deletes_existing_object",
			path:        "/delete/me.mp4",
			createFirst: true,
		},
		{
			name:        "missing_object_returns_not_found",
			path:        "/missing/file.mp4",
			wantErr:     true,
			errSentinel: mediastoredata.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.createFirst {
				_, err := b.PutObject(context.Background(), tt.path, []byte("data"), "video/mp4", "", "TEMPORAL", "")
				require.NoError(t, err)
			}

			err := b.DeleteObject(context.Background(), tt.path)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errSentinel != nil {
					require.ErrorIs(t, err, tt.errSentinel)
				}

				return
			}

			require.NoError(t, err)

			_, err = b.GetObject(context.Background(), tt.path)
			require.ErrorIs(t, err, awserr.ErrNotFound)
		})
	}
}

func TestInMemoryBackend_UpdateObjectMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errSentinel error
		name        string
		path        string
		contentType string
		cacheCtrl   string
		createFirst bool
		wantErr     bool
	}{
		{
			name:        "updates_content_type",
			path:        "/update/me.mp4",
			contentType: "application/octet-stream",
			cacheCtrl:   "no-cache",
			createFirst: true,
		},
		{
			name:        "missing_object_returns_not_found",
			path:        "/missing/file.mp4",
			wantErr:     true,
			errSentinel: mediastoredata.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			if tt.createFirst {
				_, err := b.PutObject(context.Background(), tt.path, []byte("data"), "video/mp4", "", "TEMPORAL", "")
				require.NoError(t, err)
			}

			err := b.UpdateObjectMetadata(context.Background(), tt.path, tt.contentType, tt.cacheCtrl)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errSentinel != nil {
					require.ErrorIs(t, err, tt.errSentinel)
				}

				return
			}

			require.NoError(t, err)

			obj, err := b.GetObject(context.Background(), tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.contentType, obj.ContentType)
			assert.Equal(t, tt.cacheCtrl, obj.CacheControl)
		})
	}
}

func TestInMemoryBackend_UploadAvailability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		uploadAvailability string
		want               string
		wantErr            bool
	}{
		{name: "standard_stored_and_returned", uploadAvailability: "STANDARD", want: "STANDARD"},
		{name: "streaming_stored_and_returned", uploadAvailability: "STREAMING", want: "STREAMING"},
		// UploadAvailability defaults to "standard" when omitted (types.UploadAvailability
		// doc comment in aws-sdk-go-v2/service/mediastoredata/api_op_PutObject.go:81:
		// "The default value for this header is standard"), matching the same
		// empty->TEMPORAL default already applied to StorageClass just above.
		{name: "empty_defaults_to_standard", uploadAvailability: "", want: "STANDARD"},
		{name: "invalid_value_rejected", uploadAvailability: "BOGUS", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			_, err := b.PutObject(
				context.Background(),
				"/avail/file.mp4", []byte("data"), "video/mp4", "", "TEMPORAL", tt.uploadAvailability,
			)

			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, mediastoredata.ErrInvalidUploadAvailability)

				return
			}
			require.NoError(t, err)

			obj, err := b.GetObject(context.Background(), "/avail/file.mp4")
			require.NoError(t, err)
			assert.Equal(t, tt.want, obj.UploadAvailability)
		})
	}
}
