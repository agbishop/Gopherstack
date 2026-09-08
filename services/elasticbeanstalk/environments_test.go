package elasticbeanstalk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/elasticbeanstalk"
)

func TestInMemoryBackend_CreateEnvironment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		setup     func(b *elasticbeanstalk.InMemoryBackend)
		name      string
		appName   string
		envName   string
		wantErr   bool
	}{
		{
			name:    "create success",
			appName: "my-app",
			envName: "my-env",
		},
		{
			name:    "create duplicate",
			appName: "my-app",
			envName: "dup-env",
			setup: func(b *elasticbeanstalk.InMemoryBackend) {
				_, _ = b.CreateEnvironment(
					context.Background(), "my-app", "dup-env", "", "", nil,
					elasticbeanstalk.CreateEnvironmentParams{},
				)
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

			env, err := b.CreateEnvironment(
				context.Background(),
				tt.appName,
				tt.envName,
				"64bit Amazon Linux",
				"test env",
				nil,
				elasticbeanstalk.CreateEnvironmentParams{},
			)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.appName, env.ApplicationName)
			assert.Equal(t, tt.envName, env.EnvironmentName)
			assert.Equal(t, "Ready", env.Status)
			assert.Equal(t, "Green", env.Health)
			assert.Contains(t, env.EnvironmentARN, tt.appName)
		})
	}
}

func TestInMemoryBackend_DescribeEnvironments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		appFilter string
		envFilter []string
		envIDs    []string
		wantCount int
	}{
		{
			name:      "list all",
			wantCount: 3,
		},
		{
			name:      "filter by app",
			appFilter: "app-a",
			wantCount: 2,
		},
		{
			name:      "filter by env name",
			envFilter: []string{"env-1"},
			wantCount: 1,
		},
		{
			name:      "filter by env id",
			envIDs:    []string{"e-00000001"},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()
			ctx := context.Background()
			params := elasticbeanstalk.CreateEnvironmentParams{}
			_, _ = b.CreateEnvironment(ctx, "app-a", "env-1", "", "", nil, params)
			_, _ = b.CreateEnvironment(ctx, "app-a", "env-2", "", "", nil, params)
			_, _ = b.CreateEnvironment(ctx, "app-b", "env-3", "", "", nil, params)

			envs := b.DescribeEnvironments(context.Background(), tt.appFilter, tt.envFilter, tt.envIDs)
			assert.Len(t, envs, tt.wantCount)
		})
	}
}

func TestInMemoryBackend_TerminateEnvironment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErrIs error
		name      string
		appName   string
		envName   string
		wantErr   bool
	}{
		{
			name:    "terminate existing",
			appName: "my-app",
			envName: "my-env",
		},
		{
			name:      "terminate not found",
			appName:   "my-app",
			envName:   "nonexistent",
			wantErr:   true,
			wantErrIs: awserr.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newTestBackend()

			if tt.envName == "my-env" {
				_, _ = b.CreateEnvironment(
					context.Background(), "my-app", "my-env", "", "", nil,
					elasticbeanstalk.CreateEnvironmentParams{},
				)
			}

			env, err := b.TerminateEnvironment(context.Background(), tt.appName, tt.envName)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrIs != nil {
					require.ErrorIs(t, err, tt.wantErrIs)
				}

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "Terminated", env.Status)
			// Verify it's gone.
			envs := b.DescribeEnvironments(context.Background(), "my-app", []string{"my-env"}, nil)
			assert.Empty(t, envs)
		})
	}
}

// TestInMemoryBackend_TerminateEnvironment_ClearsManagedActionHistory verifies
// that terminating an environment clears its managed action history. Otherwise
// recreating an environment with the same (user-chosen, reusable) name
// inherits the terminated environment's stale history.
func TestInMemoryBackend_TerminateEnvironment_ClearsManagedActionHistory(t *testing.T) {
	t.Parallel()

	b := newTestBackend()
	ctx := context.Background()

	_, err := b.CreateEnvironment(ctx, "my-app", "my-env", "", "", nil,
		elasticbeanstalk.CreateEnvironmentParams{})
	require.NoError(t, err)

	b.AddManagedActionHistory(ctx, "my-env", "action-1", "InstanceRefresh", "ghost action", "Succeeded")
	require.NotEmpty(t, b.DescribeEnvironmentManagedActionHistory(ctx, "my-env"))

	_, err = b.TerminateEnvironment(ctx, "my-app", "my-env")
	require.NoError(t, err)

	_, err = b.CreateEnvironment(ctx, "my-app", "my-env", "", "", nil,
		elasticbeanstalk.CreateEnvironmentParams{})
	require.NoError(t, err)

	assert.Empty(t, b.DescribeEnvironmentManagedActionHistory(ctx, "my-env"),
		"recreated environment must not inherit the terminated environment's managed action history")
}
