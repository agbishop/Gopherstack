package stepfunctions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	dynamodbpkg "github.com/blackbirdworks/gopherstack/services/dynamodb"
	ecsbackend "github.com/blackbirdworks/gopherstack/services/ecs"
	gluebackend "github.com/blackbirdworks/gopherstack/services/glue"
	s3pkg "github.com/blackbirdworks/gopherstack/services/s3"
	"github.com/blackbirdworks/gopherstack/services/sns"
	"github.com/blackbirdworks/gopherstack/services/sqs"
	"github.com/blackbirdworks/gopherstack/services/stepfunctions/asl"
)

// sqsAdapter adapts sqs.StorageBackend to asl.SQSIntegration.
type sqsAdapter struct {
	backend sqs.StorageBackend
}

// NewSQSIntegration creates a new SQS integration adapter.
func NewSQSIntegration(backend sqs.StorageBackend) asl.SQSIntegration {
	return &sqsAdapter{backend: backend}
}

// SFNSendMessage implements asl.SQSIntegration.
func (a *sqsAdapter) SFNSendMessage(
	_ context.Context,
	queueURL, messageBody, groupID, deduplicationID string,
	delaySeconds int,
) (string, string, error) {
	out, err := a.backend.SendMessage(&sqs.SendMessageInput{
		QueueURL:               queueURL,
		MessageBody:            messageBody,
		MessageGroupID:         groupID,
		MessageDeduplicationID: deduplicationID,
		DelaySeconds:           delaySeconds,
	})
	if err != nil {
		return "", "", err
	}

	return out.MessageID, out.MD5OfBody, nil
}

// snsAdapter adapts sns.StorageBackend to asl.SNSIntegration.
type snsAdapter struct {
	backend sns.StorageBackend
}

// NewSNSIntegration creates a new SNS integration adapter.
func NewSNSIntegration(backend sns.StorageBackend) asl.SNSIntegration {
	return &snsAdapter{backend: backend}
}

// SFNPublish implements asl.SNSIntegration.
func (a *snsAdapter) SFNPublish(_ context.Context, topicARN, message, subject string) (string, error) {
	return a.backend.Publish(topicARN, message, subject, "", nil)
}

// dynamoDBAdapter adapts dynamodb.StorageBackend to asl.DynamoDBIntegration.
type dynamoDBAdapter struct {
	backend dynamodbpkg.StorageBackend
}

// Compile-time assertion: dynamoDBAdapter must implement asl.DynamoDBIntegration.
var _ asl.DynamoDBIntegration = (*dynamoDBAdapter)(nil)

// NewDynamoDBIntegration creates a new DynamoDB integration adapter.
func NewDynamoDBIntegration(backend dynamodbpkg.StorageBackend) asl.DynamoDBIntegration {
	return &dynamoDBAdapter{backend: backend}
}

// s3Adapter adapts s3.StorageBackend to asl.S3Reader, used to resolve Map
// state ItemReader (Distributed Map) items from S3 objects.
type s3Adapter struct {
	backend s3pkg.StorageBackend
}

// Compile-time assertion: s3Adapter must implement asl.S3Reader.
var _ asl.S3Reader = (*s3Adapter)(nil)

// NewS3Integration creates a new S3 integration adapter for Map state ItemReader.
func NewS3Integration(backend s3pkg.StorageBackend) asl.S3Reader {
	return &s3Adapter{backend: backend}
}

// GetObjectBytes implements asl.S3Reader.
func (a *s3Adapter) GetObjectBytes(ctx context.Context, bucket, key string) ([]byte, error) {
	out, err := a.backend.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// s3ResultWriterAdapter adapts s3.StorageBackend to asl.S3Writer, used to
// export Distributed Map ResultWriter output to S3.
type s3ResultWriterAdapter struct {
	backend s3pkg.StorageBackend
}

// Compile-time assertion: s3ResultWriterAdapter must implement asl.S3Writer.
var _ asl.S3Writer = (*s3ResultWriterAdapter)(nil)

// NewS3ResultWriterIntegration creates an S3 integration adapter for
// Distributed Map ResultWriter.
func NewS3ResultWriterIntegration(backend s3pkg.StorageBackend) asl.S3Writer {
	return &s3ResultWriterAdapter{backend: backend}
}

// PutObjectBytes implements asl.S3Writer.
func (a *s3ResultWriterAdapter) PutObjectBytes(ctx context.Context, bucket, key string, data []byte) error {
	_, err := a.backend.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})

	return err
}

// convertViaJSON converts a value to a target type by marshaling to JSON and back.
func convertViaJSON(input, target any) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, target)
}

// outputToAny converts a typed SDK output struct to an untyped map via JSON round-trip.
// This is used by all DynamoDB adapter methods to return an `any` value that the
// ASL executor can pass as the state's Result.
func outputToAny(out any) (any, error) {
	var result any
	if err := convertViaJSON(out, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// SFNPutItem implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNPutItem(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.PutItemInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.PutItem(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNGetItem implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNGetItem(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.GetItemInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.GetItem(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNDeleteItem implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNDeleteItem(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.DeleteItemInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.DeleteItem(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNUpdateItem implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNUpdateItem(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.UpdateItemInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.UpdateItem(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNBatchExecuteStatement implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNBatchExecuteStatement(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.BatchExecuteStatementInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.BatchExecuteStatement(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNBatchGetItem implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNBatchGetItem(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.BatchGetItemInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.BatchGetItem(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNBatchWriteItem implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNBatchWriteItem(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.BatchWriteItemInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.BatchWriteItem(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNCreateBackup implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNCreateBackup(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.CreateBackupInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.CreateBackup(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNCreateGlobalTable implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNCreateGlobalTable(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.CreateGlobalTableInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.CreateGlobalTable(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNCreateTable implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNCreateTable(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.CreateTableInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.CreateTable(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNDeleteBackup implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNDeleteBackup(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.DeleteBackupInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.DeleteBackup(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNDeleteResourcePolicy implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNDeleteResourcePolicy(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.DeleteResourcePolicyInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.DeleteResourcePolicy(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// SFNDeleteTable implements asl.DynamoDBIntegration.
func (a *dynamoDBAdapter) SFNDeleteTable(ctx context.Context, input any) (any, error) {
	var req awsdynamodb.DeleteTableInput
	if err := convertViaJSON(input, &req); err != nil {
		return nil, err
	}

	out, err := a.backend.DeleteTable(ctx, &req)
	if err != nil {
		return nil, err
	}

	return outputToAny(out)
}

// ecsSyncAdapter adapts ecs.InMemoryBackend to asl.ECSSyncWaiter, polling
// DescribeTasks for the task(s) a ".sync" RunTask started (gopherstack-tdp6).
type ecsSyncAdapter struct {
	backend *ecsbackend.InMemoryBackend
}

// NewECSSyncWaiter creates an ECS ".sync" pattern poller.
func NewECSSyncWaiter(backend *ecsbackend.InMemoryBackend) asl.ECSSyncWaiter {
	return &ecsSyncAdapter{backend: backend}
}

const ecsTaskStatusStopped = "STOPPED"

// SFNPollSyncTask implements asl.ECSSyncWaiter.
func (a *ecsSyncAdapter) SFNPollSyncTask(_ context.Context, runTaskResult any) (asl.ECSSyncPoll, error) {
	started := extractECSTasks(runTaskResult)
	if len(started) == 0 {
		return asl.ECSSyncPoll{Done: true, Result: runTaskResult}, nil
	}

	described := make([]ecsbackend.Task, 0, len(started))

	for _, t := range started {
		out, failures, err := a.backend.DescribeTasks(t.ClusterArn, []string{t.TaskArn})
		if err != nil {
			return asl.ECSSyncPoll{}, err
		}

		if len(failures) > 0 || len(out) == 0 {
			return asl.ECSSyncPoll{
				Done:          true,
				Failed:        true,
				FailureReason: fmt.Sprintf("ECS task %s no longer exists", t.TaskArn),
			}, nil
		}

		if out[0].LastStatus != ecsTaskStatusStopped {
			return asl.ECSSyncPoll{}, nil
		}

		described = append(described, out[0])
	}

	failed, reason := ecsTasksFailed(described)
	taskAny := make([]any, len(described))
	for i, t := range described {
		taskAny[i] = t
	}

	return asl.ECSSyncPoll{
		Done:          true,
		Failed:        failed,
		FailureReason: reason,
		Result:        map[string]any{"Tasks": taskAny, "Failures": []any{}},
	}, nil
}

// ecsTasksFailed reports whether any described task stopped with a
// non-zero container exit code -- real ECS's own signal that a task's
// essential container failed rather than completed normally.
func ecsTasksFailed(tasks []ecsbackend.Task) (bool, string) {
	for _, t := range tasks {
		for _, c := range t.Containers {
			if c.ExitCode != nil && *c.ExitCode != 0 {
				return true, fmt.Sprintf(
					"ECS task %s stopped: %s (container %q exited with code %d)",
					t.TaskArn, t.StoppedReason, c.Name, *c.ExitCode,
				)
			}
		}
	}

	return false, ""
}

// extractECSTasks pulls the started tasks out of an ECSIntegration.SFNRunTask
// result, which is always exactly ecs.InMemoryBackend.SFNRunTask's own
// map[string]any{"Tasks": []any, "Failures": []any} (see
// services/ecs/sfn_integration.go), with each Tasks entry an ecs.Task value.
func extractECSTasks(runTaskResult any) []ecsbackend.Task {
	m, ok := runTaskResult.(map[string]any)
	if !ok {
		return nil
	}

	rawTasks, _ := m["Tasks"].([]any)
	tasks := make([]ecsbackend.Task, 0, len(rawTasks))

	for _, rt := range rawTasks {
		if t, isTask := rt.(ecsbackend.Task); isTask {
			tasks = append(tasks, t)
		}
	}

	return tasks
}

// glueSyncAdapter adapts glue.InMemoryBackend to asl.GlueSyncWaiter, polling
// GetJobRun for the job run a ".sync" StartJobRun started (gopherstack-tdp6).
type glueSyncAdapter struct {
	backend *gluebackend.InMemoryBackend
}

// NewGlueSyncWaiter creates a Glue ".sync" pattern poller.
func NewGlueSyncWaiter(backend *gluebackend.InMemoryBackend) asl.GlueSyncWaiter {
	return &glueSyncAdapter{backend: backend}
}

// SFNPollSyncJobRun implements asl.GlueSyncWaiter. Terminal JobRunState
// values per aws-sdk-go-v2/service/glue/types/enums.go's JobRunState enum:
// SUCCEEDED is the only success; STOPPED, FAILED, TIMEOUT, ERROR, and EXPIRED
// are all terminal failures (this backend's reconciler only ever produces
// SUCCEEDED, TIMEOUT, or STOPPED -- see services/glue/reconciler.go -- the
// rest are handled defensively).
func (a *glueSyncAdapter) SFNPollSyncJobRun(_ context.Context, jobName, runID string) (asl.GlueSyncPoll, error) {
	run, err := a.backend.GetJobRun(jobName, runID)
	if err != nil {
		return asl.GlueSyncPoll{}, err
	}

	switch run.JobRunState {
	case "SUCCEEDED":
		return asl.GlueSyncPoll{Done: true, Result: map[string]any{"JobRun": run}}, nil
	// statusFailed == "FAILED", coincidentally shared with execution status.
	case "STOPPED", statusFailed, "TIMEOUT", "ERROR", "EXPIRED":
		reason := run.ErrorMessage
		if reason == "" {
			reason = fmt.Sprintf("Glue job run %s/%s ended in state %s", jobName, runID, run.JobRunState)
		}

		return asl.GlueSyncPoll{Done: true, Failed: true, FailureReason: reason}, nil
	default:
		return asl.GlueSyncPoll{}, nil
	}
}
