package elasticbeanstalk_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/elasticbeanstalk"
)

func TestInMemoryBackend_CreateApplicationVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs    error
		setup        func(b *elasticbeanstalk.InMemoryBackend)
		name         string
		appName      string
		versionLabel string
		wantErr      bool
	}{
		{
			name:         "create success",
			appName:      "my-app",
			versionLabel: "v1",
			setup: func(b *elasticbeanstalk.InMemoryBackend) {
				_, _ = b.CreateApplication(context.Background(), "my-app", "", nil)
			},
		},
		{
			name:         "create duplicate",
			appName:      "my-app",
			versionLabel: "v1",
			setup: func(b *elasticbeanstalk.InMemoryBackend) {
				_, _ = b.CreateApplication(context.Background(), "my-app", "", nil)
				_, _ = b.CreateApplicationVersion(context.Background(), "my-app", "v1", "", "", "", nil)
			},
			wantErr:   true,
			wantErrIs: awserr.ErrAlreadyExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()

			if tt.setup != nil {
				tt.setup(b)
			}

			ver, err := b.CreateApplicationVersion(
				context.Background(), tt.appName, tt.versionLabel, "version desc", "", "", nil,
			)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.appName, ver.ApplicationName)
			assert.Equal(t, tt.versionLabel, ver.VersionLabel)
			assert.Equal(t, "Processed", ver.Status)
			assert.Contains(t, ver.ApplicationVersionARN, tt.appName)
		})
	}
}

// TestInMemoryBackend_CreateApplicationVersion_RequiresApplication verifies AWS's
// documented CreateApplicationVersion behavior: if no application is found
// with this name, and AutoCreateApplication is false, returns an
// InvalidParameterValue error.
func TestInMemoryBackend_CreateApplicationVersion_RequiresApplication(t *testing.T) {
	t.Parallel()

	t.Run("missing application without AutoCreateApplication errors", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend()
		_, err := b.CreateApplicationVersionWithParams(
			context.Background(), "ghost-app", "v1", elasticbeanstalk.ApplicationVersionParams{},
		)
		require.Error(t, err)
		require.ErrorIs(t, err, awserr.ErrInvalidParameter)
	})

	t.Run("missing application with AutoCreateApplication succeeds", func(t *testing.T) {
		t.Parallel()
		b := newTestBackend()
		ver, err := b.CreateApplicationVersionWithParams(
			context.Background(), "auto-app", "v1",
			elasticbeanstalk.ApplicationVersionParams{AutoCreateApplication: true},
		)
		require.NoError(t, err)
		assert.Equal(t, "auto-app", ver.ApplicationName)

		apps := b.DescribeApplications(context.Background(), []string{"auto-app"})
		require.Len(t, apps, 1)
	})
}

// TestInMemoryBackend_DeleteApplicationVersion_RefusesRunningEnvironment locks
// real AWS's DeleteApplicationVersion doc comment: "You cannot delete an
// application version that is associated with a running environment".
func TestInMemoryBackend_DeleteApplicationVersion_RefusesRunningEnvironment(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := newTestBackend()

	_, err := b.CreateApplication(ctx, "eb-app", "", nil)
	require.NoError(t, err)
	_, err = b.CreateApplicationVersion(ctx, "eb-app", "v1", "", "", "", nil)
	require.NoError(t, err)
	_, err = b.CreateEnvironment(ctx, "eb-app", "eb-env", "", "", nil,
		elasticbeanstalk.CreateEnvironmentParams{VersionLabel: "v1"})
	require.NoError(t, err)

	err = b.DeleteApplicationVersion(ctx, "eb-app", "v1")
	require.Error(t, err)
	require.ErrorIs(t, err, awserr.ErrInvalidParameter)

	vers := b.DescribeApplicationVersions(ctx, "eb-app", nil)
	require.Len(t, vers, 1, "version must survive a refused delete")

	_, err = b.TerminateEnvironment(ctx, "eb-app", "eb-env")
	require.NoError(t, err)

	require.NoError(t, b.DeleteApplicationVersion(ctx, "eb-app", "v1"))
}

// TestInMemoryBackend_UpdateApplicationVersion_BumpsDateUpdated verifies that
// UpdateApplicationVersion advances DateUpdated on every mutation, not just at
// creation time.
func TestInMemoryBackend_UpdateApplicationVersion_BumpsDateUpdated(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	_, err := b.CreateApplication(context.Background(), "app2", "", nil)
	require.NoError(t, err)
	ver, err := b.CreateApplicationVersion(context.Background(), "app2", "v1", "orig", "", "", nil)
	require.NoError(t, err)
	created := ver.DateUpdated

	time.Sleep(time.Second)

	updated, err := b.UpdateApplicationVersion(context.Background(), "app2", "v1", "new desc")
	require.NoError(t, err)
	assert.NotEqual(t, created, updated.DateUpdated)
}
