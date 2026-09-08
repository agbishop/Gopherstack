package ecs

import "strings"

const (
	logDriverAwslogs       = "awslogs"
	optAwslogsGroup        = "awslogs-group"
	optAwslogsStreamPrefix = "awslogs-stream-prefix"
)

// cwLogsRunner is implemented by TaskRunners that can forward container
// output to CloudWatch Logs (currently only realDockerRunner). Runners that
// don't implement it (noopRunner, test fakes) simply stay unwired.
type cwLogsRunner interface {
	SetCWLogsBackend(cwl CWLogsBackend)
}

// SetCWLogsBackend wires CloudWatch Logs so an awslogs-driver container's log
// group/stream become discoverable when its task starts, and (when the
// configured TaskRunner supports it) its stdout/stderr get forwarded there.
// Passing nil restores the historical behavior of LogConfiguration being
// stored and echoed with no effect on CloudWatch Logs.
func (b *InMemoryBackend) SetCWLogsBackend(cwl CWLogsBackend) {
	b.mu.Lock("SetCWLogsBackend")
	b.cwLogs = cwl
	b.mu.Unlock()

	if r, ok := b.runner.(cwLogsRunner); ok {
		r.SetCWLogsBackend(cwl)
	}
}

// ensureAwslogsStreams creates the CloudWatch Logs group/stream named by each
// awslogs-driver container in td, when a CWLogsBackend is wired. Must be
// called without the backend lock held: it calls out to another backend.
func (b *InMemoryBackend) ensureAwslogsStreams(task *Task, td *TaskDefinition) {
	if task == nil || td == nil {
		return
	}

	var cwl CWLogsBackend

	b.mu.RLock("ensureAwslogsStreams")
	cwl = b.cwLogs
	b.mu.RUnlock()

	if cwl == nil {
		return
	}

	taskID := taskIDFromARN(task.TaskArn)

	for _, cd := range td.ContainerDefinitions {
		group, stream, ok := awslogsTarget(cd, taskID)
		if !ok {
			continue
		}

		_ = cwl.EnsureLogGroupAndStream(group, stream)
	}
}

// awslogsTarget returns the CloudWatch Logs group/stream an awslogs-driver
// container definition is bound to, matching the naming ensureAwslogsStreams
// uses to create them. ok is false for any non-awslogs driver or a missing
// awslogs-group option, in which case group/stream are meaningless.
func awslogsTarget(cd ContainerDefinition, taskID string) (string, string, bool) {
	lc := cd.LogConfiguration
	if lc == nil || lc.LogDriver != logDriverAwslogs {
		return "", "", false
	}

	group := lc.Options[optAwslogsGroup]
	if group == "" {
		return "", "", false
	}

	stream := awslogsStreamName(lc.Options[optAwslogsStreamPrefix], cd.Name, taskID)

	return group, stream, true
}

// taskIDFromARN extracts the task ID (the segment after the last "/") from an
// ECS task ARN of either format handled by clusterFromTaskARN.
func taskIDFromARN(taskARN string) string {
	if idx := strings.LastIndex(taskARN, "/"); idx != -1 {
		return taskARN[idx+1:]
	}

	return taskARN
}

// awslogsStreamName derives the log stream name for an awslogs-driver
// container. Per the aws-sdk-go-v2 doc for LogConfiguration's
// awslogs-stream-prefix option (service/ecs types/types.go): "If you specify
// a prefix with this option, then the log stream takes the format
// prefix-name/container-name/ecs-task-id." When no prefix is configured, real
// ECS instead names the stream after the Docker-assigned container ID; this
// is computed here (in ensureAwslogsStreams, before the container exists) so
// the task ID alone is used as an approximation instead (gopherstack-jnct).
func awslogsStreamName(prefix, containerName, taskID string) string {
	if prefix == "" {
		return taskID
	}

	return prefix + "/" + containerName + "/" + taskID
}
