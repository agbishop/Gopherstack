package efs_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	sdktypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/efs"
)

// TestLifecyclePolicyValidation verifies invalid enum values are rejected.
//
// wantErrIs was efs.ErrValidation until this pass; PutLifecycleConfiguration
// declares BadRequest, never ValidationException (efs@v1.44.4 deserializers.go)
// -- the old assertion locked in the exact wire-code defect this pass fixed.
func TestLifecyclePolicyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		policy    efs.LifecyclePolicy
		name      string
		wantErr   bool
	}{
		{
			name:   "valid_transition_to_ia",
			policy: efs.LifecyclePolicy{TransitionToIA: "AFTER_30_DAYS"},
		},
		{
			name:   "valid_transition_to_primary",
			policy: efs.LifecyclePolicy{TransitionToPrimaryStorageClass: "AFTER_1_ACCESS"},
		},
		{
			name:      "invalid_transition_to_ia",
			policy:    efs.LifecyclePolicy{TransitionToIA: "AFTER_FOREVER"},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
		{
			name:      "invalid_transition_to_primary",
			policy:    efs.LifecyclePolicy{TransitionToPrimaryStorageClass: "NEVER"},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
		{
			name:      "none_not_a_real_enum_member_rejected",
			policy:    efs.LifecyclePolicy{TransitionToIA: "NONE"},
			wantErr:   true,
			wantErrIs: efs.ErrBadRequest,
		},
		{
			name:   "empty_policy_valid",
			policy: efs.LifecyclePolicy{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-lp-"+tt.name))
			require.NoError(t, err)

			_, err = b.PutLifecycleConfiguration(
				context.Background(),
				fs.FileSystemID,
				[]efs.LifecyclePolicy{tt.policy},
			)

			if tt.wantErr {
				require.ErrorIs(t, err, tt.wantErrIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Anti-drift: every value the pinned SDK's types.TransitionToIARules and
// types.TransitionToArchiveRules enums know about must be accepted, so a
// hand-maintained allowlist can't fall behind again.
func TestLifecyclePolicy_EverySDKEnumValueAccepted(t *testing.T) {
	t.Parallel()

	for i, v := range sdktypes.TransitionToIARules("").Values() {
		t.Run("transition_to_ia_"+string(v), func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq(fmt.Sprintf("tok-ia-%d", i)))
			require.NoError(t, err)

			_, err = b.PutLifecycleConfiguration(
				context.Background(),
				fs.FileSystemID,
				[]efs.LifecyclePolicy{{TransitionToIA: string(v)}},
			)
			require.NoError(t, err, "expected SDK TransitionToIARules %s to be accepted", v)
		})
	}

	for i, v := range sdktypes.TransitionToArchiveRules("").Values() {
		t.Run("transition_to_archive_"+string(v), func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			fs, err := b.CreateFileSystem(context.Background(), fsReq(fmt.Sprintf("tok-archive-%d", i)))
			require.NoError(t, err)

			_, err = b.PutLifecycleConfiguration(
				context.Background(),
				fs.FileSystemID,
				[]efs.LifecyclePolicy{{TransitionToArchive: string(v)}},
			)
			require.NoError(t, err, "expected SDK TransitionToArchiveRules %s to be accepted", v)
		})
	}
}

// TestPutLifecycleConfiguration_RequiresAvailableFileSystem verifies
// PutLifecycleConfiguration rejects requests while the file system's
// lifecycle state is not "available" (efs@v1.44.4 types/errors.go:
// IncorrectFileSystemLifeCycleState, "Returned if the file system's lifecycle
// state is not \"available\"").
func TestPutLifecycleConfiguration_RequiresAvailableFileSystem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr       error
		name          string
		activateDelay time.Duration
	}{
		{name: "creating_state_rejected", activateDelay: time.Hour, wantErr: efs.ErrIncorrectFileSystemLifeCycleState},
		{name: "available_state_allowed", activateDelay: 0, wantErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestEFSBackend()
			efs.SetFSActivationDelay(b, tt.activateDelay)

			fs, err := b.CreateFileSystem(context.Background(), fsReq("tok-lc-lifecycle-"+tt.name))
			require.NoError(t, err)

			_, err = b.PutLifecycleConfiguration(
				context.Background(),
				fs.FileSystemID,
				[]efs.LifecyclePolicy{{TransitionToIA: "AFTER_30_DAYS"}},
			)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
