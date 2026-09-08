package ecs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strconv"
	"sync"

	"github.com/moby/moby/api/pkg/stdcopy"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	dockertypes "github.com/blackbirdworks/gopherstack/internal/dockercompat/api/types/container"
	dockerimage "github.com/blackbirdworks/gopherstack/internal/dockercompat/api/types/image"
	dockernetwork "github.com/blackbirdworks/gopherstack/internal/dockercompat/api/types/network"
	"github.com/blackbirdworks/gopherstack/internal/dockercompat/client"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// dockerClient is the subset of the Docker API used by realDockerRunner.
// It is defined as an interface to allow injection of fakes in tests.
type dockerClient interface {
	ImagePull(
		ctx context.Context,
		refStr string,
		options dockerimage.PullOptions,
	) (io.ReadCloser, error)
	ContainerCreate(
		ctx context.Context,
		config *dockertypes.Config,
		hostConfig *dockertypes.HostConfig,
		networkingConfig *dockernetwork.NetworkingConfig,
		platform *ocispec.Platform,
		containerName string,
	) (dockertypes.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options dockertypes.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options dockertypes.StopOptions) error
	ContainerRemove(
		ctx context.Context,
		containerID string,
		options dockertypes.RemoveOptions,
	) error
	// ContainerLogs matches github.com/moby/moby/client@v0.5.1's
	// (*Client).ContainerLogs(ctx, containerID, ContainerLogsOptions)
	// (ContainerLogsResult, error), with ContainerLogsResult being exactly
	// io.ReadCloser and the compat option type substituted in, so the
	// concrete client (internal/dockercompat/client.Client) satisfies this
	// unchanged.
	ContainerLogs(
		ctx context.Context,
		containerID string,
		options dockertypes.LogsOptions,
	) (io.ReadCloser, error)
	// ContainerWait matches github.com/moby/moby/client@v0.5.1's
	// (*Client).ContainerWait(ctx, containerID, ContainerWaitOptions)
	// ContainerWaitResult, with the compat option/result types substituted in,
	// so the concrete client (internal/dockercompat/client.Client) satisfies
	// this unchanged.
	ContainerWait(
		ctx context.Context,
		containerID string,
		options dockertypes.WaitOptions,
	) dockertypes.WaitResult
}

// NewDockerRunner creates a TaskRunner backed by the local Docker daemon.
// It uses the standard DOCKER_HOST / DOCKER_TLS_VERIFY environment variables
// via client.FromEnv, so it works both locally and inside docker-in-docker.
func NewDockerRunner(ctx context.Context) (TaskRunner, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv(), client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	return newDockerRunnerWithClient(ctx, cli), nil
}

// newDockerRunnerWithClient creates a realDockerRunner using the provided dockerClient.
// This constructor is used by tests to inject a fake Docker client.
func newDockerRunnerWithClient(ctx context.Context, cli dockerClient) *realDockerRunner {
	if ctx == nil {
		ctx = context.Background()
	}

	return &realDockerRunner{
		cli:         cli,
		containers:  make(map[string][]string),
		logCancels:  make(map[string]context.CancelFunc),
		waitCancels: make(map[string]context.CancelFunc),
		svcCtx:      ctx,
	}
}

// realDockerRunner is a TaskRunner that launches Docker containers.
type realDockerRunner struct {
	containers        map[string][]string
	logCancels        map[string]context.CancelFunc
	waitCancels       map[string]context.CancelFunc
	cli               dockerClient
	cwLogs            CWLogsBackend
	completionHandler func(taskArn, containerName string, exitCode int)
	svcCtx            context.Context
	logWG             sync.WaitGroup
	waitWG            sync.WaitGroup
	mu                sync.Mutex
}

// SetCWLogsBackend wires CloudWatch Logs so this runner forwards awslogs-driver
// containers' stdout/stderr as they run. Implements cwLogsRunner (logs.go);
// called via InMemoryBackend.SetCWLogsBackend. Safe to call concurrently
// with RunTask.
func (r *realDockerRunner) SetCWLogsBackend(cwl CWLogsBackend) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cwLogs = cwl
}

func (r *realDockerRunner) cwLogsBackend() CWLogsBackend {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.cwLogs
}

// SetTaskCompletionHandler wires fn to be called when a container this runner
// started exits on its own -- not via an explicit StopTask/rollback, which
// cancel the watch instead (see cancelContainerWait) -- so the caller can move
// the owning task to STOPPED. Safe to call concurrently with RunTask.
func (r *realDockerRunner) SetTaskCompletionHandler(fn func(taskArn, containerName string, exitCode int)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.completionHandler = fn
}

func (r *realDockerRunner) taskCompletionHandler() func(taskArn, containerName string, exitCode int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.completionHandler
}

func (r *realDockerRunner) RunTask(task *Task, td *TaskDefinition) error {
	ctx := r.svcCtx
	log := logger.Load(ctx)

	// Bound once at call time so a concurrent SetCWLogsBackend doesn't change
	// the target mid-task; nil means logs stay unforwarded (cwLogsRunner unwired).
	cwLogs := r.cwLogsBackend()
	// Bound once at call time for the same reason; nil means no watcher is
	// started at all (taskCompletionRunner unwired, e.g. in tests that don't
	// need it), so container exits are only observed via an explicit StopTask.
	completionHandler := r.taskCompletionHandler()
	taskID := taskIDFromARN(task.TaskArn)

	// started accumulates container IDs that were successfully started during
	// this call.  If any later container fails we roll back by stopping and
	// removing everything accumulated so far, keeping RunTask atomic.
	var started []string

	for _, cd := range td.ContainerDefinitions {
		if err := r.pullImage(ctx, cd.Image); err != nil {
			r.rollbackContainers(ctx, started)

			return err
		}

		containerID, err := r.createContainer(ctx, task, cd)
		if err != nil {
			r.rollbackContainers(ctx, started)

			return err
		}

		if startErr := r.cli.ContainerStart(ctx, containerID, dockertypes.StartOptions{}); startErr != nil {
			// Clean up the just-created container before rolling back the rest.
			if rmErr := r.cli.ContainerRemove(ctx, containerID, dockertypes.RemoveOptions{Force: true}); rmErr != nil {
				log.WarnContext(
					ctx, "failed to remove container after start failure",
					"containerID", containerID,
					"error", rmErr,
				)
			}

			r.rollbackContainers(ctx, started)

			return fmt.Errorf("start container %s: %w", containerID, startErr)
		}

		started = append(started, containerID)

		if cwLogs != nil {
			if group, stream, ok := awslogsTarget(cd, taskID); ok {
				r.forwardContainerLogs(cwLogs, containerID, group, stream)
			}
		}

		if completionHandler != nil {
			r.watchContainerExit(completionHandler, task.TaskArn, containerID, cd.Name)
		}
	}

	// All containers started successfully; register them in the tracking map.
	r.mu.Lock()
	defer r.mu.Unlock()

	r.containers[task.TaskArn] = append(r.containers[task.TaskArn], started...)

	return nil
}

// rollbackContainers stops and force-removes a set of already-started
// containers.  Errors are logged but not returned so that all containers are
// attempted.  This is called on the error path of RunTask to ensure the
// task leaves no running containers behind.
func (r *realDockerRunner) rollbackContainers(ctx context.Context, containerIDs []string) {
	log := logger.Load(ctx)
	timeout := 10

	for _, id := range containerIDs {
		// These containers are being torn down unconditionally below (force
		// remove), so stop any in-flight log forwarder before that happens
		// rather than leaving it to notice via a Follow read error. Likewise
		// for the exit watcher: this is a rollback, not a natural exit, so it
		// must not report task completion.
		r.cancelLogForwarding(id)
		r.cancelContainerWait(id)

		if err := r.cli.ContainerStop(ctx, id, dockertypes.StopOptions{Timeout: &timeout}); err != nil {
			log.WarnContext(
				ctx,
				"failed to stop container during rollback",
				"containerID",
				id,
				"error",
				err,
			)
		}

		if err := r.cli.ContainerRemove(ctx, id, dockertypes.RemoveOptions{Force: true}); err != nil {
			log.WarnContext(
				ctx,
				"failed to remove container during rollback",
				"containerID",
				id,
				"error",
				err,
			)
		}
	}
}

// pullImage pulls a Docker image and drains the response body.
func (r *realDockerRunner) pullImage(ctx context.Context, image string) error {
	reader, err := r.cli.ImagePull(ctx, image, dockerimage.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", image, err)
	}

	if _, copyErr := io.Copy(io.Discard, reader); copyErr != nil {
		_ = reader.Close()

		return fmt.Errorf("drain image pull response for %s: %w", image, copyErr)
	}

	if closeErr := reader.Close(); closeErr != nil {
		return fmt.Errorf("close image pull reader for %s: %w", image, closeErr)
	}

	return nil
}

// createContainer creates a Docker container for the given container definition.
func (r *realDockerRunner) createContainer(
	ctx context.Context,
	task *Task,
	cd ContainerDefinition,
) (string, error) {
	portBindings, exposedPorts := buildPortMappings(cd.PortMappings)
	env := buildEnv(cd.Environment)

	resp, err := r.cli.ContainerCreate(
		ctx,
		&dockertypes.Config{
			Image:        cd.Image,
			Env:          env,
			ExposedPorts: exposedPorts,
			Labels: map[string]string{
				"gopherstack.ecs.task":    task.TaskArn,
				"gopherstack.ecs.cluster": task.ClusterArn,
			},
		},
		&dockertypes.HostConfig{
			PortBindings: portBindings,
		},
		nil,
		nil,
		"",
	)
	if err != nil {
		return "", fmt.Errorf("create container for %s: %w", cd.Image, err)
	}

	return resp.ID, nil
}

// buildPortMappings converts PortMappings to Moby network.PortMap and network.PortSet.
func buildPortMappings(mappings []PortMapping) (dockernetwork.PortMap, dockernetwork.PortSet) {
	portBindings := dockernetwork.PortMap{}
	exposedPorts := dockernetwork.PortSet{}

	for _, pm := range mappings {
		proto := pm.Protocol
		if proto == "" {
			proto = transportTCP
		}

		containerPort, err := dockernetwork.ParsePort(fmt.Sprintf("%d/%s", pm.ContainerPort, proto))
		if err != nil {
			continue
		}

		exposedPorts[containerPort] = struct{}{}

		if pm.HostPort > 0 {
			portBindings[containerPort] = []dockernetwork.PortBinding{
				{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: strconv.Itoa(pm.HostPort)},
			}
		}
	}

	return portBindings, exposedPorts
}

// buildEnv converts KeyValuePairs to Docker-compatible "KEY=VALUE" strings.
func buildEnv(kvs []KeyValuePair) []string {
	env := make([]string, 0, len(kvs))
	for _, kv := range kvs {
		env = append(env, fmt.Sprintf("%s=%s", kv.Name, kv.Value))
	}

	return env
}

func (r *realDockerRunner) StopTask(task *Task) error {
	// Snapshot the container IDs while holding the lock but without removing
	// the entry yet — we only remove it once all stops have been attempted.
	var containerIDs []string

	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		containerIDs = append([]string(nil), r.containers[task.TaskArn]...)
	}()

	if len(containerIDs) == 0 {
		return nil
	}

	ctx := r.svcCtx
	timeout := 10

	var (
		errs   []error
		failed []string
	)

	for _, containerID := range containerIDs {
		if err := r.cli.ContainerStop(ctx, containerID, dockertypes.StopOptions{Timeout: &timeout}); err != nil {
			errs = append(errs, fmt.Errorf("stop container %s: %w", containerID, err))
			failed = append(failed, containerID)

			continue
		}

		// Only cancel once the container is confirmed stopped; a container
		// that failed to stop stays tracked for retry and may still be
		// producing output worth forwarding, or may yet exit on its own.
		r.cancelLogForwarding(containerID)
		r.cancelContainerWait(containerID)
	}

	// Update the tracking map: remove the entry entirely on full success, or
	// retain only the containers that could not be stopped so callers can retry.
	func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		if len(failed) == 0 {
			delete(r.containers, task.TaskArn)
		} else {
			r.containers[task.TaskArn] = failed
		}
	}()

	return errors.Join(errs...)
}

// forwardContainerLogs streams a just-started container's stdout/stderr to
// CloudWatch Logs, in its own goroutine so RunTask returns without waiting
// for the container to exit. The goroutine ends -- and releases its map
// entry and logWG slot -- when the log stream reaches EOF (container exit or
// removal, in production) or cancelLogForwarding(containerID) is called.
func (r *realDockerRunner) forwardContainerLogs(cwLogs CWLogsBackend, containerID, group, stream string) {
	ctx, cancel := context.WithCancel(r.svcCtx)

	r.mu.Lock()
	r.logCancels[containerID] = cancel
	r.mu.Unlock()

	r.logWG.Go(func() {
		defer cancel()
		defer func() {
			r.mu.Lock()
			delete(r.logCancels, containerID)
			r.mu.Unlock()
		}()

		rc, err := r.cli.ContainerLogs(ctx, containerID, dockertypes.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			return
		}
		defer rc.Close()

		w := &awslogsWriter{cwLogs: cwLogs, group: group, stream: stream}
		// Not using err: StdCopy's error is either a Systemerr frame or an
		// I/O error, neither actionable once we're mid-stream best-effort
		// forwarding logs; it always returns on EOF regardless.
		_, _ = stdcopy.StdCopy(w, w, rc)
		w.flush()
	})
}

// cancelLogForwarding stops the in-flight log forwarder for containerID, if
// any. Called once a container is confirmed stopped/removed so its forwarder
// goroutine doesn't block on Follow indefinitely against a dead container.
func (r *realDockerRunner) cancelLogForwarding(containerID string) {
	r.mu.Lock()
	cancel, ok := r.logCancels[containerID]
	r.mu.Unlock()

	if ok {
		cancel()
	}
}

// watchContainerExit waits for containerID to stop and, unless the wait is
// cancelled first (see cancelContainerWait, called from StopTask/rollback
// once a container is confirmed stopped by an explicit request), reports the
// exit to handler so the owning task can move to STOPPED. Runs in its own
// goroutine so RunTask does not block on it, and terminates on container exit,
// cancellation, or shutdown (ctx derives from r.svcCtx) -- never leaked.
//
// Only the first container in a task to exit is meant to drive the task to
// STOPPED (see markTaskStoppedByContainerExit); siblings of a multi-container
// task are not forcibly stopped here, an approximation left for a future
// change since real ECS task definitions used with Step Functions .sync are
// overwhelmingly single-container batch jobs.
func (r *realDockerRunner) watchContainerExit(
	handler func(taskArn, containerName string, exitCode int),
	taskArn, containerID, containerName string,
) {
	ctx, cancel := context.WithCancel(r.svcCtx)

	r.mu.Lock()
	r.waitCancels[containerID] = cancel
	r.mu.Unlock()

	r.waitWG.Go(func() {
		defer cancel()
		defer func() {
			r.mu.Lock()
			delete(r.waitCancels, containerID)
			r.mu.Unlock()
		}()

		result := r.cli.ContainerWait(ctx, containerID, dockertypes.WaitOptions{})

		select {
		case res := <-result.Result:
			handler(taskArn, containerName, int(res.StatusCode))
		case <-result.Error:
		case <-ctx.Done():
		}
	})
}

// cancelContainerWait stops the in-flight exit watcher for containerID, if
// any. Called once a container is confirmed stopped/removed so its watcher
// doesn't report a self-inflicted stop as a natural exit.
func (r *realDockerRunner) cancelContainerWait(containerID string) {
	r.mu.Lock()
	cancel, ok := r.waitCancels[containerID]
	r.mu.Unlock()

	if ok {
		cancel()
	}
}

// awslogsWriter demultiplexed via stdcopy.StdCopy writes both stdout and
// stderr to the same instance: the awslogs driver interleaves both into one
// CloudWatch Logs stream, same as the real ECS agent. It splits the combined
// bytes into lines and forwards each as it completes.
type awslogsWriter struct {
	cwLogs CWLogsBackend
	group  string
	stream string
	buf    []byte
}

func (w *awslogsWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)

	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}

		line := string(w.buf[:idx])
		w.buf = w.buf[idx+1:]
		_ = w.cwLogs.PutLogLines(w.group, w.stream, []string{line})
	}

	return len(p), nil
}

// flush forwards a final line left over with no trailing newline.
func (w *awslogsWriter) flush() {
	if len(w.buf) == 0 {
		return
	}

	_ = w.cwLogs.PutLogLines(w.group, w.stream, []string{string(w.buf)})
	w.buf = nil
}

// newTaskRunner creates the appropriate TaskRunner based on the
// GOPHERSTACK_ECS_RUNTIME environment variable.
// Returns a no-op runner when the environment variable is absent or "none".
func newTaskRunner(ctx context.Context) (TaskRunner, error) {
	switch os.Getenv("GOPHERSTACK_ECS_RUNTIME") {
	case "docker":
		return NewDockerRunner(ctx)
	default:
		// "none" or unset – no-op
		return NewNoopRunner(), nil
	}
}
