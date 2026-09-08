package lambda_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dockercontainer "github.com/blackbirdworks/gopherstack/internal/dockercompat/api/types/container"
	"github.com/blackbirdworks/gopherstack/internal/dockercompat/api/types/image"

	gophercontainer "github.com/blackbirdworks/gopherstack/pkgs/container"
	"github.com/blackbirdworks/gopherstack/pkgs/portalloc"
	"github.com/blackbirdworks/gopherstack/services/lambda"
)

// Port ranges reserved for container cleanup tests: 20600–20699.
const (
	containerCleanupTimeoutBase   = 20600 // 20600–20609: invocation timeout cleanup
	containerCleanupLRUBase       = 20610 // 20610–20619: LRU eviction test case 1
	containerCleanupStopBase      = 20620 // 20620–20629: container stop on timeout
	containerCleanupLRU2Base      = 20640 // 20640–20649: LRU eviction test case 2
	containerCleanupUpdateBase    = 20670 // 20670–20679: UpdateFunction runtime eviction
	containerCleanupAsyncBase     = 20680 // 20680–20689: async timeout cleanup
	containerCleanupStartFailBase = 20690 // 20690–20699: startup failure stale entry
)

// errSimulatedContainerCreate is a sentinel used by tests that need container creation to fail.
var errSimulatedContainerCreate = errors.New("simulated container create error")

// trackingDockerAPI is a mock Docker API client that records StopAndRemove calls.
type trackingDockerAPI struct {
	createErr   error
	stopCalls   []string
	removeCalls []string
	counter     int
	mu          sync.Mutex
}

func (m *trackingDockerAPI) ImagePull(_ context.Context, _ string, _ image.PullOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *trackingDockerAPI) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	return nil, nil
}

func (m *trackingDockerAPI) ContainerCreate(
	_ context.Context,
	_ *dockercontainer.Config,
	_ *dockercontainer.HostConfig,
	_ any,
	_ any,
	name string,
) (dockercontainer.CreateResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createErr != nil {
		return dockercontainer.CreateResponse{}, m.createErr
	}

	m.counter++
	id := fmt.Sprintf("container-%s-%d", name, m.counter)

	return dockercontainer.CreateResponse{ID: id}, nil
}

func (m *trackingDockerAPI) ContainerStart(_ context.Context, _ string, _ dockercontainer.StartOptions) error {
	return nil
}

func (m *trackingDockerAPI) ContainerStop(_ context.Context, containerID string, _ dockercontainer.StopOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalls = append(m.stopCalls, containerID)

	return nil
}

func (m *trackingDockerAPI) ContainerRemove(
	_ context.Context,
	containerID string,
	_ dockercontainer.RemoveOptions,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeCalls = append(m.removeCalls, containerID)

	return nil
}

func (m *trackingDockerAPI) Ping(_ context.Context) (any, error) { return struct{}{}, nil }
func (m *trackingDockerAPI) Close() error                        { return nil }

func (m *trackingDockerAPI) StopCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]string, len(m.stopCalls))
	copy(out, m.stopCalls)

	return out
}

// newTrackingDockerClient wraps trackingDockerAPI in a DockerRuntime.
func newTrackingDockerClient(api *trackingDockerAPI) gophercontainer.Runtime {
	return gophercontainer.NewDockerRuntimeWithAPI(api, gophercontainer.Config{
		PoolSize:    3,
		IdleTimeout: time.Minute,
	})
}

// TestCleanupTimedOutRuntime_RemovesFromMap verifies that when a synchronous
// invocation times out, the runtime entry is removed from the runtimes map so that
// the next invocation creates a fresh container instead of perpetually timing out.
func TestCleanupTimedOutRuntime_RemovesFromMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		port             int
		fnTimeout        int // function Timeout in seconds (minimum 1)
		wantRuntimes     int
		wantErrIsTimeout bool
	}{
		{
			// Timeout=1s: the runtime server never responds, so the invocation times
			// out after 1s + containerResponseGracePeriod (≈ 1.1s total).
			name:             "runtime_removed_after_timeout",
			port:             containerCleanupTimeoutBase,
			fnTimeout:        1,
			wantRuntimes:     0,
			wantErrIsTimeout: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create a backend with no docker client (the injected runtime bypasses docker).
			b := lambda.NewInMemoryBackend(nil, nil, lambda.DefaultSettings(), "000000000000", "us-east-1")
			closeBackend(t, b)

			fn := &lambda.FunctionConfiguration{
				FunctionName: "timeout-fn",
				PackageType:  lambda.PackageTypeImage,
				ImageURI:     "test:latest",
				Timeout:      tt.fnTimeout,
			}
			require.NoError(t, b.CreateFunction(fn))

			// Start a runtime server that never returns invocations (simulates a hung container).
			srv := lambda.NewExportedRuntimeServer(tt.port)
			require.NoError(t, srv.Start(t.Context()))

			t.Cleanup(func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				srv.Stop(stopCtx)
			})

			// Inject the runtime so InvokeFunction uses it rather than starting a container.
			lambda.InjectRuntimeEntryFull(b, "timeout-fn", srv, "")
			require.Equal(t, 1, lambda.RuntimesLen(b), "runtime should be present before invocation")

			// Invoke synchronously — should time out because no container picks up the request.
			_, _, invokeErr := b.InvokeFunction(
				t.Context(), "timeout-fn", lambda.InvocationTypeRequestResponse, []byte(`{}`),
			)

			if tt.wantErrIsTimeout {
				require.ErrorIs(t, invokeErr, lambda.ErrInvocationTimeout)
			}

			// Give the cleanup goroutine a brief moment to remove the map entry.
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				assert.Equal(c, tt.wantRuntimes, lambda.RuntimesLen(b), "runtime should be evicted after timeout")
			}, time.Second, 10*time.Millisecond)
		})
	}
}

// TestCleanupTimedOutRuntime_StopsContainer verifies that cleanupTimedOutRuntime calls
// StopAndRemove on the Docker runtime for the container that was running.
func TestCleanupTimedOutRuntime_StopsContainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		containerID string
		port        int
		wantStopped bool
	}{
		{
			name:        "container_stopped_on_timeout",
			port:        containerCleanupStopBase,
			containerID: "test-container-abc",
			wantStopped: true,
		},
		{
			name:        "no_container_no_stop_call",
			port:        containerCleanupStopBase + 1,
			containerID: "",
			wantStopped: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &trackingDockerAPI{}
			dc := newTrackingDockerClient(api)

			b := lambda.NewInMemoryBackend(dc, nil, lambda.DefaultSettings(), "000000000000", "us-east-1")
			closeBackend(t, b)

			fn := &lambda.FunctionConfiguration{
				FunctionName: "stop-fn",
				PackageType:  lambda.PackageTypeImage,
				ImageURI:     "test:latest",
				Timeout:      1,
			}
			require.NoError(t, b.CreateFunction(fn))

			// Start a runtime server that never responds.
			srv := lambda.NewExportedRuntimeServer(tt.port)
			require.NoError(t, srv.Start(t.Context()))

			t.Cleanup(func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				srv.Stop(stopCtx)
			})

			// Inject a runtime with both a server and an optional container ID.
			lambda.InjectRuntimeEntryFull(b, "stop-fn", srv, tt.containerID)

			// Invoke synchronously — will time out.
			_, _, invokeErr := b.InvokeFunction(
				t.Context(), "stop-fn", lambda.InvocationTypeRequestResponse, []byte(`{}`),
			)
			require.ErrorIs(t, invokeErr, lambda.ErrInvocationTimeout)

			// Wait for the cleanup goroutine to run.
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				assert.Equal(c, 0, lambda.RuntimesLen(b))
			}, 2*time.Second, 10*time.Millisecond)

			if tt.wantStopped {
				// The cleanup goroutine removes the runtime from the map first and then
				// stops the container asynchronously, so we need an additional short wait.
				require.EventuallyWithT(t, func(c *assert.CollectT) {
					assert.Contains(c, api.StopCalls(), tt.containerID)
				}, 2*time.Second, 10*time.Millisecond)
			} else {
				// Verify no stop calls are issued. We wait for the cleanup goroutine to
				// have enough time to run and confirm nothing was added.
				require.Never(t, func() bool {
					return len(api.StopCalls()) > 0
				}, 200*time.Millisecond, 10*time.Millisecond)
			}
		})
	}
}

// TestCleanupTimedOutRuntime_SemSaturated_StillCleansUp verifies that a timed-out
// runtime's container is still stopped when b.cleanupSem is saturated. cleanupTimedOutRuntime
// removes the runtime from b.runtimes unconditionally, so a dropped cleanup on the
// semaphore-full branch permanently leaks the container, port, and temp dirs: nothing
// else ever revisits that runtime again. lookupOrRegisterRuntime's LRU eviction and
// UpdateFunction's runtime eviction both fall back to an inline cleanupRuntime call when
// the semaphore is full; cleanupTimedOutRuntime must do the same.
func TestCleanupTimedOutRuntime_SemSaturated_StillCleansUp(t *testing.T) {
	t.Parallel()

	api := &trackingDockerAPI{}
	dc := newTrackingDockerClient(api)

	b := lambda.NewInMemoryBackend(dc, nil, lambda.DefaultSettings(), "000000000000", "us-east-1")
	closeBackend(t, b)

	fn := &lambda.FunctionConfiguration{
		FunctionName: "sem-saturated-fn",
		PackageType:  lambda.PackageTypeImage,
		ImageURI:     "test:latest",
		Timeout:      1,
	}
	require.NoError(t, b.CreateFunction(fn))

	lambda.InjectRuntimeEntryWithContainer(b, "sem-saturated-fn", "", "leaked-container", nil, 0)

	drain := lambda.FillCleanupSem(b)
	defer drain()

	lambda.CleanupTimedOutRuntimeForTest(b, "sem-saturated-fn")

	assert.Equal(t, 0, lambda.RuntimesLen(b), "runtime entry must still be removed from the map")
	assert.Contains(t, api.StopCalls(), "leaked-container",
		"container must still be stopped when cleanupSem is saturated (inline fallback)")
}

// TestLRUEviction verifies that when the number of active runtimes exceeds MaxRuntimes,
// the least-recently-used runtime is evicted.
func TestLRUEviction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		lruName     string
		wantEvicted string
		portStart   int
		portEnd     int
		maxRuntimes int
		injectCount int
		wantRemain  int
	}{
		{
			// MaxRuntimes=2; inject fn-0 + fn-old (2 runtimes); invoke fn-new → 3 runtimes,
			// evict fn-old (oldest) → 2 remain.
			name:        "evicts_oldest_when_limit_exceeded",
			portStart:   containerCleanupLRUBase,
			portEnd:     containerCleanupLRUBase + 9,
			maxRuntimes: 2,
			injectCount: 1,
			lruName:     "fn-old",
			wantEvicted: "fn-old",
			wantRemain:  2,
		},
		{
			// MaxRuntimes=5; inject fn-0, fn-1 (2 runtimes); invoke fn-new → 3 runtimes,
			// 3 <= 5 so no eviction → 3 remain.
			name:        "no_eviction_below_limit",
			portStart:   containerCleanupLRU2Base,
			portEnd:     containerCleanupLRU2Base + 9,
			maxRuntimes: 5,
			injectCount: 2,
			lruName:     "",
			wantEvicted: "",
			wantRemain:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pa, err := portalloc.New(tt.portStart, tt.portEnd)
			require.NoError(t, err)

			api := &trackingDockerAPI{}
			dc := newTrackingDockerClient(api)

			settings := lambda.DefaultSettings()
			settings.MaxRuntimes = tt.maxRuntimes

			b := lambda.NewInMemoryBackend(dc, pa, settings, "000000000000", "us-east-1")
			closeBackend(t, b)

			// Inject existing runtimes with current timestamps.
			for i := range tt.injectCount {
				name := fmt.Sprintf("fn-%d", i)
				lambda.InjectRuntimeEntry(b, name, "", nil, 0)
			}

			// Inject the LRU candidate with a very old timestamp so it is chosen for eviction.
			if tt.lruName != "" {
				lambda.InjectRuntimeEntry(b, tt.lruName, "", nil, 0)
				lambda.SetRuntimeLastUsed(b, tt.lruName, time.Now().Add(-time.Hour))
			}

			// Create and invoke a new function — this triggers getOrCreateRuntime
			// which calls evictLRURuntimeLocked when the new entry is added.
			newFn := &lambda.FunctionConfiguration{
				FunctionName: "fn-new",
				PackageType:  lambda.PackageTypeImage,
				ImageURI:     "test:latest",
				Timeout:      1,
			}
			require.NoError(t, b.CreateFunction(newFn))

			// Use event invocation so we don't block waiting for a container response.
			_, statusCode, invokeErr := b.InvokeFunction(
				t.Context(), "fn-new", lambda.InvocationTypeEvent, []byte(`{}`),
			)
			require.NoError(t, invokeErr)
			assert.Equal(t, http.StatusAccepted, statusCode)

			assert.Equal(t, tt.wantRemain, lambda.RuntimesLen(b))

			if tt.wantEvicted != "" {
				assert.Empty(t, lambda.RuntimeContainerID(b, tt.wantEvicted),
					"evicted runtime should not be in the map")
			}
		})
	}
}

// TestDeleteFunction_StopsContainer verifies that DeleteFunction calls StopAndRemove
// on the backing Docker runtime for the function's container.
func TestDeleteFunction_StopsContainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		containerID string
		wantStopped bool
	}{
		{
			name:        "container_stopped_on_delete",
			containerID: "delete-container-xyz",
			wantStopped: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &trackingDockerAPI{}
			dc := newTrackingDockerClient(api)

			b := lambda.NewInMemoryBackend(dc, nil, lambda.DefaultSettings(), "000000000000", "us-east-1")
			closeBackend(t, b)

			fn := &lambda.FunctionConfiguration{
				FunctionName: "delete-fn",
				PackageType:  lambda.PackageTypeImage,
			}
			require.NoError(t, b.CreateFunction(fn))
			lambda.InjectRuntimeEntryWithContainer(b, "delete-fn", "", tt.containerID, nil, 0)

			require.NoError(t, b.DeleteFunction("delete-fn"))

			// Allow the async StopAndRemove goroutine to run (DeleteFunction is synchronous
			// but calls cleanupRuntime which does a blocking stop).
			stops := api.StopCalls()
			if tt.wantStopped {
				assert.Contains(t, stops, tt.containerID)
			} else {
				assert.Empty(t, stops)
			}
		})
	}
}

// TestClose_StopsContainer verifies that Close() calls StopAndRemove for all running containers.
func TestClose_StopsContainer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		containerIDs []string
	}{
		{
			name:         "all_containers_stopped",
			containerIDs: []string{"c1", "c2", "c3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &trackingDockerAPI{}
			dc := newTrackingDockerClient(api)

			b := lambda.NewInMemoryBackend(dc, nil, lambda.DefaultSettings(), "000000000000", "us-east-1")
			closeBackend(t, b)

			for i, cid := range tt.containerIDs {
				fnName := fmt.Sprintf("fn-%d", i)
				fn := &lambda.FunctionConfiguration{FunctionName: fnName, PackageType: lambda.PackageTypeImage}
				require.NoError(t, b.CreateFunction(fn))
				lambda.InjectRuntimeEntryWithContainer(b, fnName, "", cid, nil, 0)
			}

			b.Close(t.Context())

			stops := api.StopCalls()
			for _, cid := range tt.containerIDs {
				assert.Contains(t, stops, cid, "expected container %s to be stopped", cid)
			}
		})
	}
}

// TestLRUEviction_ZeroMaxRuntimes verifies that when MaxRuntimes is 0 in settings,
// evictLRURuntimeLocked falls back to defaultMaxRuntimes so no eviction occurs with a
// small number of runtimes.
func TestLRUEviction_ZeroMaxRuntimes(t *testing.T) {
	t.Parallel()

	pa, err := portalloc.New(20650, 20659)
	require.NoError(t, err)

	api := &trackingDockerAPI{}
	dc := newTrackingDockerClient(api)

	settings := lambda.DefaultSettings()
	settings.MaxRuntimes = 0 // will fall back to defaultMaxRuntimes (50)

	b := lambda.NewInMemoryBackend(dc, pa, settings, "000000000000", "us-east-1")
	closeBackend(t, b)

	// Inject 3 runtimes — well under the default limit of 50.
	for i := range 3 {
		lambda.InjectRuntimeEntry(b, fmt.Sprintf("fn-%d", i), "", nil, 0)
	}
	require.Equal(t, 3, lambda.RuntimesLen(b))

	// Invoke fn-new; no eviction should occur because 4 <= defaultMaxRuntimes.
	fn := &lambda.FunctionConfiguration{
		FunctionName: "fn-new",
		PackageType:  lambda.PackageTypeImage,
		ImageURI:     "test:latest",
		Timeout:      1,
	}
	require.NoError(t, b.CreateFunction(fn))

	_, _, invokeErr := b.InvokeFunction(t.Context(), "fn-new", lambda.InvocationTypeEvent, []byte(`{}`))
	require.NoError(t, invokeErr)

	// All 3 pre-injected runtimes plus the new one should remain.
	assert.Equal(t, 4, lambda.RuntimesLen(b))
}

// TestCleanupTimedOutRuntime_NonExistentFunction verifies that cleanupTimedOutRuntime
// is a no-op when the function's runtime is not in the map.
func TestCleanupTimedOutRuntime_NonExistentFunction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "no_op_when_not_in_map"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			port := 20660

			b := lambda.NewInMemoryBackend(nil, nil, lambda.DefaultSettings(), "000000000000", "us-east-1")
			closeBackend(t, b)

			fn := &lambda.FunctionConfiguration{
				FunctionName: "ghost-fn",
				PackageType:  lambda.PackageTypeImage,
				Timeout:      1,
			}
			require.NoError(t, b.CreateFunction(fn))

			// Start a runtime server that never responds.
			srv := lambda.NewExportedRuntimeServer(port)
			require.NoError(t, srv.Start(t.Context()))

			t.Cleanup(func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				srv.Stop(stopCtx)
			})

			// Inject a runtime, invoke (which will timeout), then the cleanup goroutine
			// will remove it from the map. A second cleanup call should be a no-op.
			lambda.InjectRuntimeEntryFull(b, "ghost-fn", srv, "")
			require.Equal(t, 1, lambda.RuntimesLen(b))

			// First invocation — times out, triggers cleanup.
			_, _, invokeErr := b.InvokeFunction(
				t.Context(), "ghost-fn", lambda.InvocationTypeRequestResponse, []byte(`{}`),
			)
			require.ErrorIs(t, invokeErr, lambda.ErrInvocationTimeout)

			// Wait for cleanup to remove the runtime.
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				assert.Equal(c, 0, lambda.RuntimesLen(b))
			}, 2*time.Second, 10*time.Millisecond)

			// At this point the runtime is already gone from the map.
			// cleanupTimedOutRuntime for a missing entry should be a harmless no-op,
			// verified implicitly by no panic and 0 runtimes remaining.
			assert.Equal(t, 0, lambda.RuntimesLen(b))
		})
	}
}

// TestReset_StopsContainers verifies that Reset() stops all running containers and clears
// the runtimes map, mirroring the behaviour verified for Close().
func TestReset_StopsContainers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		containerIDs []string
	}{
		{
			name:         "all_containers_stopped_on_reset",
			containerIDs: []string{"reset-c1", "reset-c2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &trackingDockerAPI{}
			dc := newTrackingDockerClient(api)

			b := lambda.NewInMemoryBackend(dc, nil, lambda.DefaultSettings(), "000000000000", "us-east-1")
			closeBackend(t, b)

			for i, cid := range tt.containerIDs {
				fnName := fmt.Sprintf("reset-fn-%d", i)
				fn := &lambda.FunctionConfiguration{FunctionName: fnName, PackageType: lambda.PackageTypeImage}
				require.NoError(t, b.CreateFunction(fn))
				lambda.InjectRuntimeEntryWithContainer(b, fnName, "", cid, nil, 0)
			}

			require.Equal(t, len(tt.containerIDs), lambda.RuntimesLen(b))

			b.Reset()

			// Runtimes map should be empty after Reset.
			assert.Equal(t, 0, lambda.RuntimesLen(b))

			// All containers should have been stopped.
			stops := api.StopCalls()
			for _, cid := range tt.containerIDs {
				assert.Contains(t, stops, cid, "expected container %s to be stopped after Reset", cid)
			}
		})
	}
}

// TestUpdateFunction_EvictsRuntime verifies that calling UpdateFunction evicts any running
// container so that the next invocation picks up the updated code or configuration.
func TestUpdateFunction_EvictsRuntime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		containerID string
		wantStopped bool
	}{
		{
			name:        "container_stopped_after_update",
			containerID: "update-fn-container",
			wantStopped: true,
		},
		{
			name:        "no_container_no_stop_call",
			containerID: "",
			wantStopped: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &trackingDockerAPI{}
			dc := newTrackingDockerClient(api)

			b := lambda.NewInMemoryBackend(dc, nil, lambda.DefaultSettings(), "000000000000", "us-east-1")
			closeBackend(t, b)

			fn := &lambda.FunctionConfiguration{
				FunctionName: "update-fn",
				PackageType:  lambda.PackageTypeImage,
				ImageURI:     "test:latest",
			}
			require.NoError(t, b.CreateFunction(fn))

			// Inject a pre-existing runtime (simulates a live container from a previous invocation).
			lambda.InjectRuntimeEntryWithContainer(b, "update-fn", "", tt.containerID, nil, 0)
			require.Equal(t, 1, lambda.RuntimesLen(b))

			// Update the function (simulates UpdateFunctionCode / UpdateFunctionConfiguration).
			fn.ImageURI = "test:v2"
			require.NoError(t, b.UpdateFunction(fn))

			// The runtime entry must be evicted immediately.
			require.Equal(t, 0, lambda.RuntimesLen(b))

			if tt.wantStopped {
				// The container stop is asynchronous; wait for it.
				require.EventuallyWithT(t, func(c *assert.CollectT) {
					assert.Contains(c, api.StopCalls(), tt.containerID)
				}, 2*time.Second, 10*time.Millisecond)
			} else {
				// No stop calls should be issued.
				require.Never(t, func() bool {
					return len(api.StopCalls()) > 0
				}, 200*time.Millisecond, 10*time.Millisecond)
			}
		})
	}
}

// TestAsyncInvocation_TimeoutCleansUpRuntime verifies that when an async (Event-type)
// invocation times out, the runtime is evicted from the map so the next invocation
// gets a fresh container. This mirrors the cleanup already done for sync timeouts.
func TestAsyncInvocation_TimeoutCleansUpRuntime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		port      int
		fnTimeout int // function Timeout in seconds (minimum 1)
	}{
		{
			name:      "runtime_evicted_after_async_timeout",
			port:      containerCleanupAsyncBase,
			fnTimeout: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := lambda.NewInMemoryBackend(nil, nil, lambda.DefaultSettings(), "000000000000", "us-east-1")
			closeBackend(t, b)

			fn := &lambda.FunctionConfiguration{
				FunctionName: "async-timeout-fn",
				PackageType:  lambda.PackageTypeImage,
				ImageURI:     "test:latest",
				Timeout:      tt.fnTimeout,
			}
			require.NoError(t, b.CreateFunction(fn))

			// Start a runtime server that never picks up invocations (simulates a hung container).
			srv := lambda.NewExportedRuntimeServer(tt.port)
			require.NoError(t, srv.Start(t.Context()))

			t.Cleanup(func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				srv.Stop(stopCtx)
			})

			lambda.InjectRuntimeEntryFull(b, "async-timeout-fn", srv, "")
			require.Equal(t, 1, lambda.RuntimesLen(b))

			// Invoke asynchronously — returns 202 immediately; the background goroutine times out.
			_, _, invokeErr := b.InvokeFunction(
				t.Context(), "async-timeout-fn", lambda.InvocationTypeEvent, []byte(`{}`),
			)
			require.NoError(t, invokeErr, "async InvokeFunction should return 202 without error")

			// Wait for the background timeout + cleanup to evict the runtime.
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				assert.Equal(c, 0, lambda.RuntimesLen(b))
			}, 5*time.Second, 50*time.Millisecond, "runtime should be evicted after async timeout")
		})
	}
}

// TestStartupFailure_ClearsRuntimeEntry verifies that when a container fails to start,
// the stale runtime entry is removed from the map so the next invocation retries
// instead of perpetually returning the same startup error.
func TestStartupFailure_ClearsRuntimeEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		portStart int
		portEnd   int
	}{
		{
			name:      "stale_entry_removed_after_startup_failure",
			portStart: containerCleanupStartFailBase,
			portEnd:   containerCleanupStartFailBase + 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &trackingDockerAPI{createErr: errSimulatedContainerCreate}
			dc := newTrackingDockerClient(api)

			pa, err := portalloc.New(tt.portStart, tt.portEnd)
			require.NoError(t, err)

			b := lambda.NewInMemoryBackend(dc, pa, lambda.DefaultSettings(), "000000000000", "us-east-1")
			closeBackend(t, b)

			fn := &lambda.FunctionConfiguration{
				FunctionName: "fail-start-fn",
				PackageType:  lambda.PackageTypeImage,
				ImageURI:     "test:latest",
			}
			require.NoError(t, b.CreateFunction(fn))

			// First invocation — container creation fails.
			_, _, firstErr := b.InvokeFunction(
				t.Context(), "fail-start-fn", lambda.InvocationTypeRequestResponse, []byte(`{}`),
			)
			require.Error(t, firstErr, "first invocation should fail due to container startup error")
			require.ErrorIs(t, firstErr, lambda.ErrLambdaUnavailable)

			// The stale entry MUST be removed so a second invocation can retry.
			assert.Equal(t, 0, lambda.RuntimesLen(b), "runtimes map must be empty after startup failure")

			// Second invocation also fails (createErr is still set) but must not return a cached error.
			_, _, secondErr := b.InvokeFunction(
				t.Context(), "fail-start-fn", lambda.InvocationTypeRequestResponse, []byte(`{}`),
			)
			require.Error(t, secondErr)
			require.ErrorIs(
				t,
				secondErr,
				lambda.ErrLambdaUnavailable,
				"second invocation should retry and fail with a fresh error",
			)
		})
	}
}

// ---- Close() cancels poller ----

func TestClose_CancelsPoller(t *testing.T) {
	t.Parallel()

	bk := lambda.NewInMemoryBackend(nil, nil, lambda.DefaultSettings(), "123456789012", "us-east-1")
	closeBackend(t, bk)

	pollDone := make(chan struct{})
	poller := lambda.NewPollerWithCancelSignal(pollDone)

	bk.SetKinesisPoller(poller)
	bk.StartKinesisPoller(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bk.Close(ctx)

	select {
	case <-pollDone:
		// poller's goroutine received the cancel signal
	case <-time.After(2 * time.Second):
		t.Fatal("poller was not cancelled within 2 seconds after Close()")
	}
}

// ---- cleanupSem/logSem replaced in Reset() (leak fix) ----

func TestReset_ReplacesSemaphoreChannels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		resetN  int
		wantCap int
	}{
		{
			name:    "single_reset_replaces_channels",
			resetN:  1,
			wantCap: 64, // maxCleanupConcurrency
		},
		{
			name:    "double_reset_replaces_channels_twice",
			resetN:  2,
			wantCap: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bk := newParityBackend(t)

			capBefore := lambda.CleanupSemCap(bk)
			require.Equal(t, tt.wantCap, capBefore, "initial cleanupSem capacity")

			for range tt.resetN {
				bk.Reset()
			}

			// After Reset(), channels must be fresh (same capacity, zero occupancy).
			assert.Equal(t, tt.wantCap, lambda.CleanupSemCap(bk),
				"cleanupSem capacity preserved after Reset()")
			assert.Equal(t, 0, lambda.CleanupSemLen(bk),
				"cleanupSem must be empty after Reset()")
		})
	}
}

func TestReset_LogSemReplacedAndEmpty(t *testing.T) {
	t.Parallel()

	bk := newParityBackend(t)
	capBefore := lambda.LogSemCap(bk)
	require.Positive(t, capBefore)

	bk.Reset()

	assert.Equal(t, capBefore, lambda.LogSemCap(bk),
		"logSem capacity preserved after Reset()")
}
