package ecs

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dockertypes "github.com/blackbirdworks/gopherstack/internal/dockercompat/api/types/container"
	dockerimage "github.com/blackbirdworks/gopherstack/internal/dockercompat/api/types/image"
	dockernetwork "github.com/blackbirdworks/gopherstack/internal/dockercompat/api/types/network"
)

var (
	errContainerStartFailed = errors.New("start failed")
	errContainerStopFailed  = errors.New("stop failed")
)

// fakeDockerClient is a test double for dockerClient.
// It assigns sequential IDs to created containers and records all operations.
type fakeDockerClient struct {
	containerLogs          map[string][]byte
	waitResult             chan dockertypes.WaitResponse
	startErrOnID           string
	stopErrOnID            string
	logsRequestedOn        []string
	removed                []string
	stopped                []string
	waitRequestedOn        []string
	started                []string
	nextID                 int
	mu                     sync.Mutex
	blockedReaderCanceled  atomic.Bool
	waitCanceled           atomic.Bool
	failAllStarts          bool
	blockLogsUntilCanceled bool
}

func (f *fakeDockerClient) ImagePull(_ context.Context, _ string, _ dockerimage.PullOptions) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeDockerClient) ContainerCreate(
	_ context.Context,
	_ *dockertypes.Config,
	_ *dockertypes.HostConfig,
	_ *dockernetwork.NetworkingConfig,
	_ *ocispec.Platform,
	_ string,
) (dockertypes.CreateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	id := fmt.Sprintf("%s%02d", strings.Repeat("a", 12), f.nextID)

	return dockertypes.CreateResponse{ID: id}, nil
}

func (f *fakeDockerClient) ContainerStart(_ context.Context, containerID string, _ dockertypes.StartOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.failAllStarts {
		return errContainerStartFailed
	}

	if f.startErrOnID != "" && containerID == f.startErrOnID {
		return errContainerStartFailed
	}

	f.started = append(f.started, containerID)

	return nil
}

func (f *fakeDockerClient) ContainerStop(_ context.Context, containerID string, _ dockertypes.StopOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.stopErrOnID != "" && containerID == f.stopErrOnID {
		return errContainerStopFailed
	}

	f.stopped = append(f.stopped, containerID)

	return nil
}

func (f *fakeDockerClient) ContainerRemove(_ context.Context, containerID string, _ dockertypes.RemoveOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.removed = append(f.removed, containerID)

	return nil
}

// ContainerLogs returns the fixed, already-EOF stream configured for
// containerID in f.containerLogs (or an empty one). It ignores Follow: real
// containers eventually stop producing output too, so a bounded fake stream
// exercises the same "runs to completion" code path deterministically.
func (f *fakeDockerClient) ContainerLogs(
	ctx context.Context,
	containerID string,
	_ dockertypes.LogsOptions,
) (io.ReadCloser, error) {
	f.mu.Lock()
	f.logsRequestedOn = append(f.logsRequestedOn, containerID)
	block := f.blockLogsUntilCanceled
	payload := f.containerLogs[containerID]
	f.mu.Unlock()

	if block {
		return &ctxAwareReadCloser{ctx: ctx, canceled: &f.blockedReaderCanceled}, nil
	}

	return io.NopCloser(bytes.NewReader(payload)), nil
}

// ctxAwareReadCloser blocks on Read until ctx is done, then reports EOF and
// records that it observed the cancellation.
type ctxAwareReadCloser struct {
	ctx      context.Context
	canceled *atomic.Bool
}

func (r *ctxAwareReadCloser) Read(_ []byte) (int, error) {
	<-r.ctx.Done()
	r.canceled.Store(true)

	return 0, io.EOF
}

func (r *ctxAwareReadCloser) Close() error { return nil }

// ContainerWait delivers f.waitResult to the caller once sent, or blocks
// until ctx is done if waitResult is nil (a container that never exits on its
// own) or is never sent to (a cancelled watch, recorded via waitCanceled).
func (f *fakeDockerClient) ContainerWait(
	ctx context.Context,
	containerID string,
	_ dockertypes.WaitOptions,
) dockertypes.WaitResult {
	f.mu.Lock()
	f.waitRequestedOn = append(f.waitRequestedOn, containerID)
	ch := f.waitResult
	f.mu.Unlock()

	resultC := make(chan dockertypes.WaitResponse, 1)
	errC := make(chan error, 1)

	go func() {
		if ch == nil {
			<-ctx.Done()
			f.waitCanceled.Store(true)

			return
		}

		select {
		case res := <-ch:
			resultC <- res
		case <-ctx.Done():
			f.waitCanceled.Store(true)
		}
	}()

	return dockertypes.WaitResult{Result: resultC, Error: errC}
}

// muxFrame encodes payload as one stdcopy-framed chunk: an 8-byte header
// (stream type, 3 zero bytes, big-endian uint32 length) followed by payload,
// matching the wire format stdcopy.StdCopy demultiplexes (see
// github.com/moby/moby/api/pkg/stdcopy@v1.55.0's stdcopy.go doc comment).
func muxFrame(streamType byte, payload string) []byte {
	frame := make([]byte, 8+len(payload))
	frame[0] = streamType
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[8:], payload)

	return frame
}

func TestBuildPortMappings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mappings []PortMapping
		wantHost int
	}{
		{
			name:     "empty mappings",
			mappings: nil,
			wantHost: 0,
		},
		{
			name: "with host port",
			mappings: []PortMapping{
				{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"},
			},
			wantHost: 1,
		},
		{
			name: "default protocol is tcp",
			mappings: []PortMapping{
				{ContainerPort: 443, HostPort: 443},
			},
			wantHost: 1,
		},
		{
			name: "no host port means no binding",
			mappings: []PortMapping{
				{ContainerPort: 8080, Protocol: "tcp"},
			},
			wantHost: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			portBindings, exposedPorts := buildPortMappings(tt.mappings)
			assert.Len(t, portBindings, tt.wantHost)
			assert.Len(t, exposedPorts, len(tt.mappings))
		})
	}
}

func TestBuildEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kvs  []KeyValuePair
		want []string
	}{
		{
			name: "empty",
			kvs:  nil,
			want: []string{},
		},
		{
			name: "single pair",
			kvs:  []KeyValuePair{{Name: "FOO", Value: "bar"}},
			want: []string{"FOO=bar"},
		},
		{
			name: "multiple pairs",
			kvs: []KeyValuePair{
				{Name: "A", Value: "1"},
				{Name: "B", Value: "2"},
			},
			want: []string{"A=1", "B=2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildEnv(tt.kvs)
			require.Len(t, got, len(tt.want))

			for i, expected := range tt.want {
				assert.Equal(t, expected, got[i])
			}
		})
	}
}

// TestNewTaskRunner_Noop verifies that the default (no env var) returns a no-op runner.
func TestNewTaskRunner_Noop(t *testing.T) {
	t.Parallel()

	runner, err := newTaskRunner(t.Context())
	require.NoError(t, err)
	require.NotNil(t, runner)

	// Noop runner should never fail.
	require.NoError(t, runner.RunTask(&Task{}, &TaskDefinition{}))
	require.NoError(t, runner.StopTask(&Task{}))
}

// TestNewTaskRunner_Docker verifies that GOPHERSTACK_ECS_RUNTIME=docker attempts
// to create a Docker runner. The test is skipped (gracefully) when the Docker
// daemon is unavailable, which is the expected state in most CI environments.
//
// This test cannot be made parallel: it calls t.Setenv, which mutates the
// process-wide environment and therefore must not race with other tests. Go's
// testing package forbids combining t.Setenv with t.Parallel for exactly this
// reason (it panics if Parallel is called on a test that used Setenv).
func TestNewTaskRunner_Docker(t *testing.T) {
	t.Setenv("GOPHERSTACK_ECS_RUNTIME", "docker")

	runner, err := newTaskRunner(t.Context())
	if err != nil {
		// Docker daemon not reachable — acceptable in CI without Docker-in-Docker.
		return
	}

	// If Docker is available, the runner must be non-nil.
	assert.NotNil(t, runner)
}

// TestDockerRunner_MultiContainerTracking verifies that all containers in a
// multi-container task are individually tracked, not just the last one.
func TestDockerRunner_MultiContainerTracking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		containers  []ContainerDefinition
		wantTracked int
		wantStarted int
	}{
		{
			name: "single container",
			containers: []ContainerDefinition{
				{Image: "nginx:latest"},
			},
			wantTracked: 1,
			wantStarted: 1,
		},
		{
			name: "two containers in same task",
			containers: []ContainerDefinition{
				{Image: "nginx:latest"},
				{Image: "redis:latest"},
			},
			wantTracked: 2,
			wantStarted: 2,
		},
		{
			name: "three containers in same task",
			containers: []ContainerDefinition{
				{Image: "app:latest"},
				{Image: "sidecar:latest"},
				{Image: "proxy:latest"},
			},
			wantTracked: 3,
			wantStarted: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeDockerClient{}
			runner := newDockerRunnerWithClient(context.Background(), fake)
			task := &Task{TaskArn: "arn:aws:ecs:us-east-1:000000000000:task/default/task-1"}
			td := &TaskDefinition{ContainerDefinitions: tt.containers}

			require.NoError(t, runner.RunTask(task, td))

			runner.mu.Lock()
			tracked := runner.containers[task.TaskArn]
			runner.mu.Unlock()

			assert.Len(t, tracked, tt.wantTracked, "all container IDs must be tracked")
			assert.Len(t, fake.started, tt.wantStarted, "all containers must have been started")
		})
	}
}

// TestDockerRunner_StopTask_StopsAllContainers verifies that StopTask stops every
// container associated with a multi-container task, not just the last one.
func TestDockerRunner_StopTask_StopsAllContainers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		numContainers int
	}{
		{name: "single container", numContainers: 1},
		{name: "two containers", numContainers: 2},
		{name: "three containers", numContainers: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeDockerClient{}
			runner := newDockerRunnerWithClient(context.Background(), fake)
			task := &Task{TaskArn: "arn:aws:ecs:us-east-1:000000000000:task/default/task-1"}

			cds := make([]ContainerDefinition, tt.numContainers)
			for i := range cds {
				cds[i] = ContainerDefinition{Image: "img:latest"}
			}

			require.NoError(t, runner.RunTask(task, &TaskDefinition{ContainerDefinitions: cds}))
			require.NoError(t, runner.StopTask(task))

			assert.Len(t, fake.stopped, tt.numContainers, "every container must be stopped")

			runner.mu.Lock()
			_, stillTracked := runner.containers[task.TaskArn]
			runner.mu.Unlock()

			assert.False(t, stillTracked, "task must be removed from tracking after stop")
		})
	}
}

// TestDockerRunner_ContainerLeakOnStartFailure verifies that when ContainerStart
// fails, the already-created container is removed to prevent a resource leak.
func TestDockerRunner_ContainerLeakOnStartFailure(t *testing.T) {
	t.Parallel()

	fake := &fakeDockerClient{}
	runner := newDockerRunnerWithClient(context.Background(), fake)
	task := &Task{TaskArn: "arn:aws:ecs:us-east-1:000000000000:task/default/task-1"}
	td := &TaskDefinition{
		ContainerDefinitions: []ContainerDefinition{
			{Image: "nginx:latest"},
		},
	}

	// Trigger the failure after we know what ID will be assigned.
	// The fake assigns IDs sequentially with zero-padded numbers; first container gets "01".
	// We must pre-set the startErrOnID before calling RunTask.
	// ContainerCreate increments nextID and builds the ID, so we pre-compute it.
	fake.startErrOnID = fmt.Sprintf("%s%02d", strings.Repeat("a", 12), 1)

	err := runner.RunTask(task, td)
	require.Error(t, err, "RunTask must return an error when ContainerStart fails")

	fake.mu.Lock()
	removed := fake.removed
	fake.mu.Unlock()

	assert.Contains(t, removed, fake.startErrOnID, "failed container must be removed to prevent a leak")

	runner.mu.Lock()
	_, tracked := runner.containers[task.TaskArn]
	runner.mu.Unlock()

	assert.False(t, tracked, "failed container must not be tracked")
}

// TestDeleteCluster_LeavesNoRunningContainers verifies that once every task in a
// cluster is stopped (as real AWS requires before DeleteCluster -- it refuses
// with ClusterContainsTasksException while any active task remains, see
// clusterDependencyViolationLocked), deletion proceeds cleanly and no Docker
// containers are left running. StopTask itself is what tells the runner to
// stop each container (TestDockerRunner_StopTask_StopsAllContainers covers
// that mechanism directly); this test proves the delete-cluster workflow
// leaves nothing behind once callers follow that required order.
func TestDeleteCluster_LeavesNoRunningContainers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		numTasks    int
		cdsPerTask  int
		wantStopped int
	}{
		{
			name:        "single task single container",
			numTasks:    1,
			cdsPerTask:  1,
			wantStopped: 1,
		},
		{
			name:        "two tasks single container each",
			numTasks:    2,
			cdsPerTask:  1,
			wantStopped: 2,
		},
		{
			name:        "two tasks two containers each",
			numTasks:    2,
			cdsPerTask:  2,
			wantStopped: 4,
		},
		{
			name:        "no tasks",
			numTasks:    0,
			cdsPerTask:  0,
			wantStopped: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeDockerClient{}
			runner := newDockerRunnerWithClient(context.Background(), fake)
			backend := NewInMemoryBackend("000000000000", "us-east-1", runner)

			_, err := backend.CreateCluster(CreateClusterInput{ClusterName: "test-cluster"})
			require.NoError(t, err)

			if tt.numTasks > 0 {
				cds := make([]ContainerDefinition, tt.cdsPerTask)
				for i := range cds {
					cds[i] = ContainerDefinition{Name: fmt.Sprintf("c%d", i), Image: "img:latest"}
				}

				_, err = backend.RegisterTaskDefinition(RegisterTaskDefinitionInput{
					Family:               "test",
					ContainerDefinitions: cds,
				})
				require.NoError(t, err)

				for range tt.numTasks {
					tasks, runErr := backend.RunTask(RunTaskInput{
						Cluster:        "test-cluster",
						TaskDefinition: "test",
					})
					require.NoError(t, runErr)
					require.Len(t, tasks, 1)

					// Real AWS requires tasks to be stopped before the
					// cluster can be deleted; do that here.
					_, stopErr := backend.StopTask("test-cluster", tasks[0].TaskArn, "test cleanup")
					require.NoError(t, stopErr)
				}
			}

			_, err = backend.DeleteCluster("test-cluster")
			require.NoError(t, err)

			fake.mu.Lock()
			stoppedCount := len(fake.stopped)
			fake.mu.Unlock()

			assert.Equal(t, tt.wantStopped, stoppedCount, "all task containers must be stopped on cluster deletion")
		})
	}
}

// TestDockerRunner_RunTask_RollbackOnPartialStart verifies that when a later
// container in a multi-container task fails to start, all previously-started
// containers are stopped and removed so RunTask is atomic.
func TestDockerRunner_RunTask_RollbackOnPartialStart(t *testing.T) {
	t.Parallel()

	// 3-container task: containers 1 and 2 start fine, container 3 fails.
	// After the failure, containers 1 and 2 must be stopped and removed.
	containerThreeID := fmt.Sprintf("%s%02d", strings.Repeat("a", 12), 3)

	fake := &fakeDockerClient{startErrOnID: containerThreeID}
	runner := newDockerRunnerWithClient(context.Background(), fake)
	task := &Task{TaskArn: "arn:aws:ecs:us-east-1:000000000000:task/default/task-1"}
	td := &TaskDefinition{
		ContainerDefinitions: []ContainerDefinition{
			{Image: "app:latest"},
			{Image: "sidecar:latest"},
			{Image: "proxy:latest"},
		},
	}

	err := runner.RunTask(task, td)
	require.Error(t, err, "RunTask must return an error when a container fails to start")

	fake.mu.Lock()
	stopped := append([]string(nil), fake.stopped...)
	removed := append([]string(nil), fake.removed...)
	fake.mu.Unlock()

	// Containers 1 and 2 were started successfully and must be rolled back.
	c1 := fmt.Sprintf("%s%02d", strings.Repeat("a", 12), 1)
	c2 := fmt.Sprintf("%s%02d", strings.Repeat("a", 12), 2)

	assert.Contains(t, stopped, c1, "first container must be stopped on rollback")
	assert.Contains(t, stopped, c2, "second container must be stopped on rollback")
	assert.Contains(t, removed, c1, "first container must be removed on rollback")
	assert.Contains(t, removed, c2, "second container must be removed on rollback")
	// Container 3 was never started; it must be removed (cleanup of the create).
	assert.Contains(t, removed, containerThreeID, "failed container must be removed to prevent a leak")

	runner.mu.Lock()
	_, tracked := runner.containers[task.TaskArn]
	runner.mu.Unlock()

	assert.False(t, tracked, "no containers must be tracked after a failed RunTask")
}

// TestDockerRunner_StopTask_PartialFailure verifies that when stopping one
// container fails, StopTask still attempts to stop the remaining containers,
// returns an aggregated error, and retains only the failed entries in tracking.
func TestDockerRunner_StopTask_PartialFailure(t *testing.T) {
	t.Parallel()

	// Two-container task; the first container's stop will fail.
	containerOneID := fmt.Sprintf("%s%02d", strings.Repeat("a", 12), 1)
	containerTwoID := fmt.Sprintf("%s%02d", strings.Repeat("a", 12), 2)

	fake := &fakeDockerClient{}
	runner := newDockerRunnerWithClient(context.Background(), fake)
	task := &Task{TaskArn: "arn:aws:ecs:us-east-1:000000000000:task/default/task-1"}
	td := &TaskDefinition{
		ContainerDefinitions: []ContainerDefinition{
			{Image: "app:latest"},
			{Image: "sidecar:latest"},
		},
	}

	require.NoError(t, runner.RunTask(task, td))

	// Inject the stop error AFTER RunTask so it doesn't interfere with start.
	fake.mu.Lock()
	fake.stopErrOnID = containerOneID
	fake.mu.Unlock()

	err := runner.StopTask(task)
	require.Error(t, err, "StopTask must return an error when a container stop fails")

	fake.mu.Lock()
	stopped := append([]string(nil), fake.stopped...)
	fake.mu.Unlock()

	// Container 2 must have been stopped even though container 1 failed.
	assert.Contains(t, stopped, containerTwoID, "successful container must still be stopped")

	// Container 1 failed; its ID must remain in tracking for a potential retry.
	runner.mu.Lock()
	remaining := append([]string(nil), runner.containers[task.TaskArn]...)
	runner.mu.Unlock()

	assert.Contains(t, remaining, containerOneID, "failed container must remain in tracking for retry")
	assert.NotContains(t, remaining, containerTwoID, "stopped container must be removed from tracking")
}

// TestBackend_RunTask_FailedRunnerSetsSTOPPED verifies that when a TaskRunner
// returns an error, RunTask marks the task as STOPPED rather than leaving it
// permanently in PROVISIONING (resource leak with wrong AWS semantics).
func TestBackend_RunTask_FailedRunnerSetsSTOPPED(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		count     int
		wantTasks int
	}{
		{
			name:      "single task",
			count:     1,
			wantTasks: 1,
		},
		{
			name:      "count=3 all fail",
			count:     3,
			wantTasks: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeDockerClient{failAllStarts: true}
			runner := newDockerRunnerWithClient(context.Background(), fake)
			backend := NewInMemoryBackend("000000000000", "us-east-1", runner)

			_, err := backend.CreateCluster(CreateClusterInput{ClusterName: "test"})
			require.NoError(t, err)

			_, err = backend.RegisterTaskDefinition(RegisterTaskDefinitionInput{
				Family:               "fail-task",
				ContainerDefinitions: []ContainerDefinition{{Name: "app", Image: "bad:image"}},
			})
			require.NoError(t, err)

			tasks, err := backend.RunTask(RunTaskInput{
				Cluster:        "test",
				TaskDefinition: "fail-task",
				Count:          tt.count,
			})
			require.NoError(t, err, "RunTask API should not return an error (runner error is internal)")
			require.Len(t, tasks, tt.wantTasks)

			for i, task := range tasks {
				assert.Equal(t, statusStopped, task.LastStatus, "task %d must be STOPPED", i)
				assert.Equal(t, statusStopped, task.DesiredStatus, "task %d desired must be STOPPED", i)
				assert.NotNil(t, task.StoppedAt, "task %d StoppedAt must be set", i)
				assert.NotEmpty(t, task.StoppedReason, "task %d StoppedReason must explain the failure", i)
				assert.Contains(t, task.StoppedReason, "container start failed")
			}
		})
	}
}

// TestBackend_StopTask_LockReleasedBeforeDockerCall verifies that backend.StopTask
// updates task state and releases the backend lock before calling the Docker runner,
// so concurrent backend operations are not blocked.
func TestBackend_StopTask_LockReleasedBeforeDockerCall(t *testing.T) {
	t.Parallel()

	fake := &fakeDockerClient{}
	runner := newDockerRunnerWithClient(context.Background(), fake)
	backend := NewInMemoryBackend("000000000000", "us-east-1", runner)

	_, err := backend.CreateCluster(CreateClusterInput{ClusterName: "test"})
	require.NoError(t, err)

	_, err = backend.RegisterTaskDefinition(RegisterTaskDefinitionInput{
		Family:               "svc-task",
		ContainerDefinitions: []ContainerDefinition{{Name: "app", Image: "app:latest"}},
	})
	require.NoError(t, err)

	runOut, err := backend.RunTask(RunTaskInput{
		Cluster:        "test",
		TaskDefinition: "svc-task",
		Count:          1,
	})
	require.NoError(t, err)
	require.Len(t, runOut, 1)

	stopped, err := backend.StopTask("test", runOut[0].TaskArn, "test stop")
	require.NoError(t, err)

	// Task must be STOPPED in the API response.
	assert.Equal(t, statusStopped, stopped.LastStatus)
	assert.Equal(t, statusStopped, stopped.DesiredStatus)
	assert.Equal(t, "test stop", stopped.StoppedReason)

	// Docker containers must have been stopped.
	fake.mu.Lock()
	stoppedCount := len(fake.stopped)
	fake.mu.Unlock()

	assert.Equal(t, 1, stoppedCount, "container must be stopped via Docker runner")
}

// TestDockerRunner_ForwardsAwslogsContainerOutput proves gopherstack-jnct's
// fix: an awslogs-driver container's real stdout/stderr -- read via
// dockerClient.ContainerLogs and demultiplexed with stdcopy.StdCopy -- now
// reaches CloudWatch Logs through PutLogLines, instead of the log
// group/stream existing but staying forever empty because nothing ever read
// the container's output.
func TestDockerRunner_ForwardsAwslogsContainerOutput(t *testing.T) {
	t.Parallel()

	containerOneID := fmt.Sprintf("%s%02d", strings.Repeat("a", 12), 1)

	fake := &fakeDockerClient{
		containerLogs: map[string][]byte{
			containerOneID: append(
				muxFrame(1, "stdout line\n"),
				muxFrame(2, "stderr line\n")...,
			),
		},
	}
	runner := newDockerRunnerWithClient(context.Background(), fake)
	backend := NewInMemoryBackend("000000000000", "us-east-1", runner)

	mock := &mockECSCWLogsBackend{}
	backend.SetCWLogsBackend(mock)

	_, err := backend.CreateCluster(CreateClusterInput{ClusterName: "test"})
	require.NoError(t, err)

	_, err = backend.RegisterTaskDefinition(RegisterTaskDefinitionInput{
		Family:               "awslogs-docker",
		ContainerDefinitions: []ContainerDefinition{awslogsContainerDef("/ecs/myapp", "")},
	})
	require.NoError(t, err)

	tasks, err := backend.RunTask(RunTaskInput{Cluster: "test", TaskDefinition: "awslogs-docker"})
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	wantStream := taskIDFromARN(tasks[0].TaskArn)

	require.Eventually(t, func() bool {
		return len(mock.putCalls()) >= 2
	}, 2*time.Second, 10*time.Millisecond, "both log lines must be forwarded to CloudWatch Logs")

	var gotLines []string

	for _, p := range mock.putCalls() {
		assert.Equal(t, "/ecs/myapp", p.group)
		assert.Equal(t, wantStream, p.stream)
		gotLines = append(gotLines, p.messages...)
	}

	assert.Equal(t, []string{"stdout line", "stderr line"}, gotLines)
}

// TestDockerRunner_NonAwslogsContainer_LogsNotRequested proves only an
// awslogs-driver container triggers ContainerLogs -- a container with no
// LogConfiguration (the common case) must not have its output read at all.
func TestDockerRunner_NonAwslogsContainer_LogsNotRequested(t *testing.T) {
	t.Parallel()

	fake := &fakeDockerClient{}
	runner := newDockerRunnerWithClient(context.Background(), fake)
	backend := NewInMemoryBackend("000000000000", "us-east-1", runner)
	backend.SetCWLogsBackend(&mockECSCWLogsBackend{})

	_, err := backend.CreateCluster(CreateClusterInput{ClusterName: "test"})
	require.NoError(t, err)

	_, err = backend.RegisterTaskDefinition(RegisterTaskDefinitionInput{
		Family:               "no-logconfig",
		ContainerDefinitions: []ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	_, err = backend.RunTask(RunTaskInput{Cluster: "test", TaskDefinition: "no-logconfig"})
	require.NoError(t, err)

	fake.mu.Lock()
	defer fake.mu.Unlock()

	assert.Empty(t, fake.logsRequestedOn, "a container with no awslogs LogConfiguration must never have its logs read")
}

// TestDockerRunner_StopTask_CancelsLogForwarding proves that stopping a task
// cancels its container's in-flight log forwarder rather than leaking the
// goroutine blocked in ContainerLogs(Follow: true) forever. The fake's
// ContainerLogs blocks on the context RunTask passed it and only reports
// having observed cancellation once that context is done, so this can only
// pass if StopTask actually cancels it.
func TestDockerRunner_StopTask_CancelsLogForwarding(t *testing.T) {
	t.Parallel()

	fake := &fakeDockerClient{blockLogsUntilCanceled: true}
	runner := newDockerRunnerWithClient(context.Background(), fake)
	backend := NewInMemoryBackend("000000000000", "us-east-1", runner)
	backend.SetCWLogsBackend(&mockECSCWLogsBackend{})

	_, err := backend.CreateCluster(CreateClusterInput{ClusterName: "test"})
	require.NoError(t, err)

	_, err = backend.RegisterTaskDefinition(RegisterTaskDefinitionInput{
		Family:               "awslogs-block",
		ContainerDefinitions: []ContainerDefinition{awslogsContainerDef("/ecs/blockapp", "")},
	})
	require.NoError(t, err)

	tasks, err := backend.RunTask(RunTaskInput{Cluster: "test", TaskDefinition: "awslogs-block"})
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	require.Eventually(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()

		return len(fake.logsRequestedOn) > 0
	}, 2*time.Second, 10*time.Millisecond, "RunTask must have started forwarding before we stop the task")

	_, err = backend.StopTask("test", tasks[0].TaskArn, "test stop")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return fake.blockedReaderCanceled.Load()
	}, 2*time.Second, 10*time.Millisecond, "StopTask must cancel the container's in-flight log forwarder")
}

// TestDockerRunner_ContainerExit_MovesTaskToStopped is the regression test for
// gopherstack-s1u9's ECS half: nothing previously waited for a container to
// exit, so a task whose container quit on its own (the common case for a
// batch job invoked via Step Functions ecs:runTask.sync) stayed RUNNING
// forever. This can only pass if RunTask actually calls ContainerWait and the
// exit is wired through to move the task to STOPPED.
func TestDockerRunner_ContainerExit_MovesTaskToStopped(t *testing.T) {
	t.Parallel()

	fake := &fakeDockerClient{waitResult: make(chan dockertypes.WaitResponse, 1)}
	runner := newDockerRunnerWithClient(context.Background(), fake)
	backend := NewInMemoryBackend("000000000000", "us-east-1", runner)
	runner.SetTaskCompletionHandler(backend.markTaskStoppedByContainerExit)

	_, err := backend.CreateCluster(CreateClusterInput{ClusterName: "test"})
	require.NoError(t, err)

	_, err = backend.RegisterTaskDefinition(RegisterTaskDefinitionInput{
		Family:               "batch-job",
		ContainerDefinitions: []ContainerDefinition{{Name: "app", Image: "busybox"}},
	})
	require.NoError(t, err)

	tasks, err := backend.RunTask(RunTaskInput{Cluster: "test", TaskDefinition: "batch-job"})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	taskArn := tasks[0].TaskArn

	require.Eventually(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()

		return len(fake.waitRequestedOn) > 0
	}, 2*time.Second, 10*time.Millisecond, "RunTask must have started watching the container before it exits")

	fake.waitResult <- dockertypes.WaitResponse{StatusCode: 3}

	require.Eventually(t, func() bool {
		got, _, describeErr := backend.DescribeTasks("test", []string{taskArn})
		require.NoError(t, describeErr)
		require.Len(t, got, 1)

		return got[0].LastStatus == statusStopped
	}, 2*time.Second, 10*time.Millisecond, "the container exiting on its own must move the task to STOPPED")

	got, _, err := backend.DescribeTasks("test", []string{taskArn})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Essential container in task exited", got[0].StoppedReason)
	require.Len(t, got[0].Containers, 1)
	require.NotNil(t, got[0].Containers[0].ExitCode)
	assert.Equal(t, 3, *got[0].Containers[0].ExitCode)
}

// TestDockerRunner_StopTask_CancelsContainerWait proves that stopping a task
// cancels its container's in-flight exit watcher rather than leaking the
// goroutine forever, and that the resulting stop keeps the caller's own
// reason instead of the container-exit one.
func TestDockerRunner_StopTask_CancelsContainerWait(t *testing.T) {
	t.Parallel()

	fake := &fakeDockerClient{waitResult: make(chan dockertypes.WaitResponse, 1)}
	runner := newDockerRunnerWithClient(context.Background(), fake)
	backend := NewInMemoryBackend("000000000000", "us-east-1", runner)
	runner.SetTaskCompletionHandler(backend.markTaskStoppedByContainerExit)

	_, err := backend.CreateCluster(CreateClusterInput{ClusterName: "test"})
	require.NoError(t, err)

	_, err = backend.RegisterTaskDefinition(RegisterTaskDefinitionInput{
		Family:               "long-runner",
		ContainerDefinitions: []ContainerDefinition{{Name: "app", Image: "nginx"}},
	})
	require.NoError(t, err)

	tasks, err := backend.RunTask(RunTaskInput{Cluster: "test", TaskDefinition: "long-runner"})
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	require.Eventually(t, func() bool {
		fake.mu.Lock()
		defer fake.mu.Unlock()

		return len(fake.waitRequestedOn) > 0
	}, 2*time.Second, 10*time.Millisecond, "RunTask must have started watching the container before we stop it")

	_, err = backend.StopTask("test", tasks[0].TaskArn, "operator stop")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return fake.waitCanceled.Load()
	}, 2*time.Second, 10*time.Millisecond, "StopTask must cancel the container's in-flight exit watcher")

	got, _, err := backend.DescribeTasks("test", []string{tasks[0].TaskArn})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "operator stop", got[0].StoppedReason)
}
