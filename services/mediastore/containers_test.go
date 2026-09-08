package mediastore_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
	"github.com/blackbirdworks/gopherstack/services/mediastore"
)

func TestInMemoryBackend_CreateContainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errSentinel error
		tags        map[string]string
		name        string
		container   string
		wantErr     bool
	}{
		{
			name:      "creates container successfully",
			container: "my-container",
		},
		{
			name:        "duplicate name returns already exists",
			container:   "dup-container",
			wantErr:     true,
			errSentinel: awserr.ErrAlreadyExists,
		},
		{
			name:        "empty tag key is rejected",
			container:   "tagged-container",
			tags:        map[string]string{"": "value"},
			wantErr:     true,
			errSentinel: mediastore.ErrEmptyTagKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()

			if errors.Is(tt.errSentinel, awserr.ErrAlreadyExists) {
				_, err := b.CreateContainer(context.Background(), testAccountID, tt.container, nil)
				require.NoError(t, err)
			}

			c, err := b.CreateContainer(context.Background(), testAccountID, tt.container, tt.tags)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, tt.errSentinel == nil || errors.Is(err, tt.errSentinel))

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.container, c.Name)
			assert.NotEmpty(t, c.ARN)
			assert.NotEmpty(t, c.Endpoint)
			assert.Equal(t, "ACTIVE", c.Status)
			assert.NotNil(t, c.CreationTime)
		})
	}
}

func TestInMemoryBackend_DeleteContainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errSentinel error
		name        string
		container   string
		createFirst bool
		wantErr     bool
	}{
		{
			name:        "deletes existing container",
			container:   "to-delete",
			createFirst: true,
		},
		{
			name:        "not found returns error",
			container:   "missing",
			createFirst: false,
			wantErr:     true,
			errSentinel: awserr.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()

			if tt.createFirst {
				_, err := b.CreateContainer(context.Background(), testAccountID, tt.container, nil)
				require.NoError(t, err)
			}

			err := b.DeleteContainer(context.Background(), tt.container)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, tt.errSentinel == nil || errors.Is(err, tt.errSentinel))

				return
			}

			require.NoError(t, err)

			_, err = b.DescribeContainer(context.Background(), tt.container)
			require.Error(t, err)
		})
	}
}

func TestInMemoryBackend_DescribeContainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errSentinel error
		name        string
		container   string
		createFirst bool
		wantErr     bool
	}{
		{
			name:        "describes existing container",
			container:   "describe-me",
			createFirst: true,
		},
		{
			name:        "not found returns error",
			container:   "missing",
			createFirst: false,
			wantErr:     true,
			errSentinel: awserr.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()

			if tt.createFirst {
				_, err := b.CreateContainer(context.Background(), testAccountID, tt.container, nil)
				require.NoError(t, err)
			}

			c, err := b.DescribeContainer(context.Background(), tt.container)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, tt.errSentinel == nil || errors.Is(err, tt.errSentinel))

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.container, c.Name)
		})
	}
}

func TestInMemoryBackend_ListContainers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		createN   int
		wantCount int
	}{
		{
			name:      "empty list",
			createN:   0,
			wantCount: 0,
		},
		{
			name:      "lists all containers",
			createN:   3,
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()

			for i := range tt.createN {
				_, err := b.CreateContainer(context.Background(), testAccountID, fmt.Sprintf("container-%d", i), nil)
				require.NoError(t, err)
			}

			containers, _, err := b.ListContainers(context.Background(), "", 0)
			require.NoError(t, err)
			assert.Len(t, containers, tt.wantCount)
		})
	}
}

func TestInMemoryBackend_ContainerPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errSentinel error
		name        string
		container   string
		policy      string
		createFirst bool
		wantErr     bool
	}{
		{
			name:        "put and get policy",
			container:   "policy-container",
			policy:      `{"Version":"2012-10-17"}`,
			createFirst: true,
		},
		{
			name:        "get policy from missing container",
			container:   "missing",
			wantErr:     true,
			errSentinel: awserr.ErrNotFound,
		},
		{
			name:        "get policy when none set",
			container:   "no-policy",
			createFirst: true,
			wantErr:     true,
			errSentinel: awserr.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()

			if tt.createFirst {
				_, err := b.CreateContainer(context.Background(), testAccountID, tt.container, nil)
				require.NoError(t, err)
			}

			if tt.policy != "" {
				err := b.PutContainerPolicy(context.Background(), tt.container, tt.policy)
				require.NoError(t, err)
			}

			policy, err := b.GetContainerPolicy(context.Background(), tt.container)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, tt.errSentinel == nil || errors.Is(err, tt.errSentinel))

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.policy, policy)
		})
	}
}

func TestInMemoryBackend_AccessLogging(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		container   string
		start       bool
		wantEnabled bool
	}{
		{
			name:        "start access logging",
			container:   "log-me",
			start:       true,
			wantEnabled: true,
		},
		{
			name:        "stop access logging",
			container:   "no-log",
			start:       false,
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()

			_, err := b.CreateContainer(context.Background(), testAccountID, tt.container, nil)
			require.NoError(t, err)

			if tt.start {
				require.NoError(t, b.StartAccessLogging(context.Background(), tt.container))
			} else {
				require.NoError(t, b.StartAccessLogging(context.Background(), tt.container))
				require.NoError(t, b.StopAccessLogging(context.Background(), tt.container))
			}

			c, err := b.DescribeContainer(context.Background(), tt.container)
			require.NoError(t, err)
			assert.Equal(t, tt.wantEnabled, c.AccessLoggingEnabled)
		})
	}
}

// waitForContainerStatus polls DescribeContainer until want is observed or
// timeout elapses, returning the final observed status (or an empty string
// if DescribeContainer errored, e.g. because the container was actually
// removed). Modeled on services/redshift's reconciler_test.go waitFor
// helper: since advanceContainerStates only runs lazily (no background
// goroutine), the caller must keep calling a read path for a due transition
// to ever apply.
func waitForContainerStatus(
	t *testing.T,
	b *mediastore.InMemoryBackend,
	name, want string,
	timeout time.Duration,
) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	last := ""

	for time.Now().Before(deadline) {
		c, err := b.DescribeContainer(context.Background(), name)
		if err != nil {
			last = ""
		} else {
			last = c.Status
		}

		if last == want {
			return last
		}

		time.Sleep(2 * time.Millisecond)
	}

	return last
}

// TestInMemoryBackend_ContainerActivationDelay verifies the CREATING/
// DELETING transient-lifecycle simulation gated by SetActivationDelay: with
// no delay configured (the default), transitions stay synchronous (matching
// every other test in this file, which never sets a delay); with a delay
// configured, CreateContainer/DeleteContainer genuinely hold the transient
// AWS status until the delay elapses, observable via DescribeContainer polls
// -- matching real AWS's asynchronous container lifecycle instead of the
// previous always-instant behavior.
func TestInMemoryBackend_ContainerActivationDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		delay time.Duration
	}{
		{
			name:  "zero_delay_is_synchronous",
			delay: 0,
		},
		{
			name:  "positive_delay_is_observable",
			delay: 20 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend()
			b.SetActivationDelay(tt.delay)

			container := "activation-delay-test"

			created, err := b.CreateContainer(context.Background(), testAccountID, container, nil)
			require.NoError(t, err)

			if tt.delay == 0 {
				assert.Equal(t, "ACTIVE", created.Status)
			} else {
				assert.Equal(t, "CREATING", created.Status)

				got := waitForContainerStatus(t, b, container, "ACTIVE", time.Second)
				assert.Equal(t, "ACTIVE", got, "container never transitioned to ACTIVE")
			}

			err = b.DeleteContainer(context.Background(), container)
			require.NoError(t, err)

			if tt.delay == 0 {
				_, err = b.DescribeContainer(context.Background(), container)
				require.Error(t, err, "container should be gone immediately with no activation delay")

				return
			}

			// With a delay configured, the container must still be visible
			// (in DELETING) immediately after DeleteContainer returns...
			mid, err := b.DescribeContainer(context.Background(), container)
			require.NoError(t, err, "container should still be visible mid-deletion")
			assert.Equal(t, "DELETING", mid.Status)

			// ...and actually gone once the delay elapses.
			deadline := time.Now().Add(time.Second)

			for {
				_, err = b.DescribeContainer(context.Background(), container)
				if err != nil {
					break
				}

				if time.Now().After(deadline) {
					t.Fatal("container was never removed after its deletion delay elapsed")
				}

				time.Sleep(2 * time.Millisecond)
			}

			require.Error(t, err)
		})
	}
}
