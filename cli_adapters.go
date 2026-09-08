package main

import (
	"context"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/services/ecs"
	"github.com/blackbirdworks/gopherstack/services/eventbridge"
	"github.com/blackbirdworks/gopherstack/services/kinesis"
	"github.com/blackbirdworks/gopherstack/services/sagemaker"
)

// === Scheduler Runner Adapters ===
//
// These adapt service backends to the scheduler.Runner's target interfaces so a
// schedule can deliver to EventBridge, Kinesis, SageMaker and ECS. Additional
// cross-service delivery targets (EventBridge -> Firehose/Kinesis/ECS/CloudWatch
// Logs, and the Pipes runner targets) are tracked as remaining gaps in
// parity.md and wired in a later pass.

type schedEventBusAdapter struct {
	backend *eventbridge.InMemoryBackend
}

func (a *schedEventBusAdapter) PutSchedulerEvent(
	ctx context.Context,
	busARN, source, detailType, detail string,
) error {
	parts := strings.Split(busARN, "/")
	busName := parts[len(parts)-1]

	now := time.Now()
	entries := []eventbridge.EventEntry{
		{
			EventBusName: busName,
			Source:       source,
			DetailType:   detailType,
			Detail:       detail,
			Time:         &now,
		},
	}
	_, err := a.backend.PutEvents(ctx, entries)

	return err
}

type schedKinesisAdapter struct {
	backend *kinesis.InMemoryBackend
}

func (a *schedKinesisAdapter) PutSchedulerRecord(
	ctx context.Context,
	streamARN, partitionKey string,
	data []byte,
) error {
	parts := strings.Split(streamARN, "/")
	streamName := parts[len(parts)-1]
	_, err := a.backend.PutRecord(ctx, &kinesis.PutRecordInput{
		StreamName:   streamName,
		PartitionKey: partitionKey,
		Data:         data,
	})

	return err
}

type schedSageMakerAdapter struct {
	backend *sagemaker.InMemoryBackend
}

func (a *schedSageMakerAdapter) StartPipelineExecution(
	ctx context.Context,
	pipelineARN string,
	params map[string]string,
) error {
	parts := strings.Split(pipelineARN, "/")
	pipelineName := parts[len(parts)-1]

	pipelineParams := make([]sagemaker.PipelineParameter, 0, len(params))
	for name, value := range params {
		pipelineParams = append(pipelineParams, sagemaker.PipelineParameter{Name: name, Value: value})
	}

	_, err := a.backend.StartPipelineExecutionFull(ctx, sagemaker.StartPipelineExecutionOptions{
		PipelineName:       pipelineName,
		PipelineParameters: pipelineParams,
	})

	return err
}

type schedECSAdapter struct {
	backend *ecs.InMemoryBackend
}

func (a *schedECSAdapter) RunSchedulerTask(
	_ context.Context,
	taskDefARN, launchType string,
	taskCount int,
) error {
	_, err := a.backend.RunTask(ecs.RunTaskInput{
		TaskDefinition: taskDefARN,
		LaunchType:     launchType,
		Count:          taskCount,
	})

	return err
}
