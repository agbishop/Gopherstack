package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

const cwlGroupName = "/pgoload/group"

func cwlStreamName(workerID int) string {
	return fmt.Sprintf("pgoload-stream-%d", workerID)
}

// ensureLogGroup creates cwlGroupName, tolerating one that already exists.
// Log streams are created per-worker in logsWorker, once, before it enters
// its op loop.
func ensureLogGroup(ctx context.Context, cl *cloudwatchlogs.Client, log *slog.Logger) error {
	_, err := cl.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{LogGroupName: aws.String(cwlGroupName)})
	if err == nil {
		return nil
	}

	if _, ok := errors.AsType[*cwltypes.ResourceAlreadyExistsException](err); ok {
		log.InfoContext(ctx, "cloudwatch logs group already exists", "group", cwlGroupName)

		return nil
	}

	return fmt.Errorf("create log group %s: %w", cwlGroupName, err)
}

// logsWorker creates its own log stream once, then repeatedly runs a mix of
// CloudWatch Logs operations, staggered by workerID, until ctx is done.
func logsWorker(ctx context.Context, cl *cloudwatchlogs.Client, workerID int, c *opCounter, log *slog.Logger) {
	if err := ensureLogStream(ctx, cl, workerID); err != nil {
		log.WarnContext(ctx, "cloudwatch logs stream setup failed (continuing)", "worker", workerID, "error", err)
	}

	ops := []opFunc{
		func(ctx context.Context, workerID, i int) error { return logsPutLogEventsOp(ctx, cl, workerID, i) },
		func(ctx context.Context, workerID, i int) error { return logsPutLogEventsOp(ctx, cl, workerID, i) },
		func(ctx context.Context, _, _ int) error { return logsDescribeLogStreamsOp(ctx, cl) },
		func(ctx context.Context, workerID, _ int) error { return logsGetLogEventsOp(ctx, cl, workerID) },
		func(ctx context.Context, _, _ int) error { return logsFilterLogEventsOp(ctx, cl) },
	}

	runOpLoop(ctx, workerID, ops, c, "logs", log)
}

func ensureLogStream(ctx context.Context, cl *cloudwatchlogs.Client, workerID int) error {
	_, err := cl.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(cwlGroupName),
		LogStreamName: aws.String(cwlStreamName(workerID)),
	})
	if err == nil {
		return nil
	}

	if _, ok := errors.AsType[*cwltypes.ResourceAlreadyExistsException](err); ok {
		return nil
	}

	return err
}

func logsPutLogEventsOp(ctx context.Context, cl *cloudwatchlogs.Client, workerID, i int) error {
	_, err := cl.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(cwlGroupName),
		LogStreamName: aws.String(cwlStreamName(workerID)),
		LogEvents: []cwltypes.InputLogEvent{
			{
				Message:   aws.String(fmt.Sprintf("pgoload log line worker=%d iter=%d", workerID, i)),
				Timestamp: aws.Int64(time.Now().UnixMilli()),
			},
		},
	})

	return err
}

func logsDescribeLogStreamsOp(ctx context.Context, cl *cloudwatchlogs.Client) error {
	_, err := cl.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(cwlGroupName),
	})

	return err
}

func logsGetLogEventsOp(ctx context.Context, cl *cloudwatchlogs.Client, workerID int) error {
	_, err := cl.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(cwlGroupName),
		LogStreamName: aws.String(cwlStreamName(workerID)),
	})

	return err
}

func logsFilterLogEventsOp(ctx context.Context, cl *cloudwatchlogs.Client) error {
	_, err := cl.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(cwlGroupName),
	})

	return err
}
