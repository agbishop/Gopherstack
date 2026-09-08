package efs_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/efs"
)

// TestCreateTags_Validates verifies that the legacy CreateTags operation
// enforces the same tag constraints as TagResource.
//
// wantErrIs was efs.ErrValidation on every tag-validation test in this file
// until this pass; validateTags' callers (CreateAccessPoint, CreateFileSystem,
// TagResource, CreateTags) all declare BadRequest, never ValidationException
// (efs@v1.44.4 deserializers.go) -- the old assertions locked in the exact
// wire-code defect this pass fixed.
func TestCreateTags_Validates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		tags      map[string]string
		name      string
		wantErr   bool
	}{
		{
			name:    "valid_tags_accepted",
			tags:    map[string]string{"env": "prod"},
			wantErr: false,
		},
		{
			name:      "empty_key_rejected",
			tags:      map[string]string{"": "value"},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
		{
			name:      "aws_prefix_rejected",
			tags:      map[string]string{"aws:reserved": "value"},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
		{
			name: "too_many_tags_rejected",
			tags: func() map[string]string {
				m := make(map[string]string, 51)
				for i := range 51 {
					m[strings.Repeat("k", i+1)] = "v"
				}

				return m
			}(),
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(
				context.Background(),
				efs.CreateFileSystemRequest{CreationToken: "tags-" + tt.name},
			)
			require.NoError(t, err)

			err = b.CreateTags(context.Background(), fs.FileSystemID, tt.tags)
			if tt.wantErr {
				require.ErrorIs(t, err, tt.wantErrIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestTagResource_ARNIndex verifies TagResource uses the ARN index for lookup.
func TestTagResource_ARNIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		useARN  bool
		wantErr bool
	}{
		{name: "tag_by_id", useARN: false},
		{name: "tag_by_arn", useARN: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-tag-arn-"+tt.name))
			require.NoError(t, err)

			resourceID := fs.FileSystemID
			if tt.useARN {
				resourceID = fs.FileSystemArn
			}

			err = b.TagResource(context.Background(), resourceID, map[string]string{"env": "test"})
			require.NoError(t, err)

			tags, err := b.ListTagsForResource(context.Background(), resourceID)
			require.NoError(t, err)
			assert.Equal(t, "test", tags["env"])
		})
	}
}

// TestTagValidation verifies key length, value length, aws: prefix, and max count
// on CreateFileSystem's Tags field.
func TestTagValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		tags      map[string]string
		name      string
		wantErr   bool
	}{
		{
			name: "valid_tags_accepted",
			tags: map[string]string{"env": "prod", "team": "eng"},
		},
		{
			name:      "aws_prefix_key_rejected",
			tags:      map[string]string{"aws:reserved": "value"},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
		{
			name:      "empty_key_rejected",
			tags:      map[string]string{"": "value"},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
		{
			name:      "value_too_long_rejected",
			tags:      map[string]string{"key": generateString(257)},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			_, err := b.CreateFileSystem(context.Background(), efs.CreateFileSystemRequest{
				CreationToken: "tok-tagval-" + tt.name,
				Tags:          tt.tags,
			})

			if tt.wantErr {
				require.ErrorIs(t, err, tt.wantErrIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestTagValidation_TagResource verifies validation on TagResource.
func TestTagValidation_TagResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		tags      map[string]string
		name      string
		wantErr   bool
	}{
		{
			name: "valid_tags_accepted",
			tags: map[string]string{"k": "v"},
		},
		{
			name:      "aws_prefix_rejected",
			tags:      map[string]string{"aws:tag": "val"},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-tagres-"+tt.name))
			require.NoError(t, err)

			err = b.TagResource(context.Background(), fs.FileSystemID, tt.tags)

			if tt.wantErr {
				require.ErrorIs(t, err, tt.wantErrIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
