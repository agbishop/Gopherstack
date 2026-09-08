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

func newTestBackend() *elasticbeanstalk.InMemoryBackend {
	return elasticbeanstalk.NewInMemoryBackend("123456789012", "us-east-1")
}

func TestInMemoryBackend_CreateApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs   error
		setup       func(b *elasticbeanstalk.InMemoryBackend)
		name        string
		appName     string
		description string
		wantErr     bool
	}{
		{
			name:        "create success",
			appName:     "my-app",
			description: "test app",
		},
		{
			name:    "create duplicate",
			appName: "dup-app",
			setup: func(b *elasticbeanstalk.InMemoryBackend) {
				_, _ = b.CreateApplication(context.Background(), "dup-app", "", nil)
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

			app, err := b.CreateApplication(
				context.Background(), tt.appName, tt.description, map[string]string{"env": "test"},
			)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.appName, app.ApplicationName)
			assert.Equal(t, tt.description, app.Description)
			assert.Contains(t, app.ApplicationARN, tt.appName)
		})
	}
}

func TestInMemoryBackend_DescribeApplications(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filter    []string
		wantCount int
	}{
		{
			name:      "list all",
			filter:    nil,
			wantCount: 2,
		},
		{
			name:      "filter by name",
			filter:    []string{"app-a"},
			wantCount: 1,
		},
		{
			name:      "filter missing",
			filter:    []string{"nonexistent"},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			_, _ = b.CreateApplication(context.Background(), "app-a", "", nil)
			_, _ = b.CreateApplication(context.Background(), "app-b", "", nil)

			apps := b.DescribeApplications(context.Background(), tt.filter)
			assert.Len(t, apps, tt.wantCount)
		})
	}
}

func TestInMemoryBackend_DeleteApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		appName   string
		wantErr   bool
	}{
		{
			name:    "delete existing",
			appName: "del-app",
		},
		{
			name:      "delete not found",
			appName:   "nonexistent",
			wantErr:   true,
			wantErrIs: awserr.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()

			if tt.appName == "del-app" {
				_, _ = b.CreateApplication(context.Background(), "del-app", "", nil)
			}

			err := b.DeleteApplication(context.Background(), tt.appName, false)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			apps := b.DescribeApplications(context.Background(), []string{tt.appName})
			assert.Empty(t, apps)
		})
	}
}

// TestInMemoryBackend_DeleteApplication_ClearsManagedActionHistory verifies
// that the DeleteApplication cascade -- which force-terminates the
// application's environments rather than calling TerminateEnvironment
// directly -- still clears their managed action history. Otherwise
// recreating an environment with the same (user-chosen, reusable) name under
// a recreated application inherits the terminated environment's stale
// history.
func TestInMemoryBackend_DeleteApplication_ClearsManagedActionHistory(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	ctx := context.Background()

	_, err := b.CreateApplication(ctx, "my-app", "", nil)
	require.NoError(t, err)

	_, err = b.CreateEnvironment(ctx, "my-app", "my-env", "", "", nil,
		elasticbeanstalk.CreateEnvironmentParams{})
	require.NoError(t, err)

	b.AddManagedActionHistory(ctx, "my-env", "action-1", "InstanceRefresh", "ghost action", "Succeeded")
	require.NotEmpty(t, b.DescribeEnvironmentManagedActionHistory(ctx, "my-env"))

	err = b.DeleteApplication(ctx, "my-app", true)
	require.NoError(t, err)

	_, err = b.CreateApplication(ctx, "my-app", "", nil)
	require.NoError(t, err)

	_, err = b.CreateEnvironment(ctx, "my-app", "my-env", "", "", nil,
		elasticbeanstalk.CreateEnvironmentParams{})
	require.NoError(t, err)

	assert.Empty(
		t,
		b.DescribeEnvironmentManagedActionHistory(ctx, "my-env"),
		"recreated environment must not inherit the deleted application's terminated environment's managed action history",
	)
}

// TestInMemoryBackend_UpdateApplication_BumpsDateUpdated verifies that UpdateApplication
// advances DateUpdated on every mutation, not just at creation time.
func TestInMemoryBackend_UpdateApplication_BumpsDateUpdated(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	app, err := b.CreateApplication(context.Background(), "app1", "orig", nil)
	require.NoError(t, err)
	created := app.DateUpdated

	time.Sleep(time.Second)

	updated, err := b.UpdateApplication(context.Background(), "app1", "new desc")
	require.NoError(t, err)
	assert.NotEqual(t, created, updated.DateUpdated)
}
