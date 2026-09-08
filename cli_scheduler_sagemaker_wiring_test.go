package main

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	sagemakerbackend "github.com/blackbirdworks/gopherstack/services/sagemaker"
	schedulerbackend "github.com/blackbirdworks/gopherstack/services/scheduler"
)

// TestInitializeServices_SchedulerSageMakerWiring drives the actual composition root
// (initializeServices, the function cli.go's Run() calls) and the real scheduler runner
// worker loop rather than invoking schedSageMakerAdapter directly.
//
// Regression test for gopherstack-q466: schedSageMakerAdapter.StartPipelineExecution was a
// no-op that always returned nil without ever calling the SageMaker backend. A schedule
// targeting a SageMaker pipeline reported success at every layer but created no execution.
func TestInitializeServices_SchedulerSageMakerWiring(t *testing.T) {
	t.Parallel()

	cli := &CLI{AccountID: "000000000000", Region: "us-east-1"}
	appCtx := &service.AppContext{
		Logger:     slog.Default(),
		Config:     cli,
		JanitorCtx: t.Context(),
	}
	cli.faultStore = chaos.NewFaultStore()

	services, err := initializeServices(appCtx)
	require.NoError(t, err)

	byName := serviceByName(services)

	schedH, ok := byName["Scheduler"].(*schedulerbackend.Handler)
	require.True(t, ok, "Scheduler handler must be registered")

	sagemakerH, ok := byName["SageMaker"].(*sagemakerbackend.Handler)
	require.True(t, ok, "SageMaker handler must be registered")

	ctx := t.Context()

	pipelineName := "scheduler-sagemaker-wiring-pipeline"
	pipeline, err := sagemakerH.Backend.CreatePipeline(
		ctx, pipelineName, `{"Steps":[]}`, "arn:aws:iam::000000000000:role/r", nil,
	)
	require.NoError(t, err)

	schedBk, ok := schedH.Backend.(*schedulerbackend.InMemoryBackend)
	require.True(t, ok, "Scheduler backend must be an InMemoryBackend")

	_, err = schedBk.CreateSchedule(
		ctx,
		"scheduler-sagemaker-wiring-schedule", "", "rate(1 minute)", "", "",
		schedulerbackend.Target{
			ARN:     pipeline.PipelineArn,
			RoleARN: "arn:aws:iam::000000000000:role/r",
			SageMakerPipelineParameters: &schedulerbackend.SageMakerPipelineParameters{
				PipelineParameterList: []schedulerbackend.SageMakerPipelineParameter{
					{Name: "p1", Value: "v1"},
				},
			},
		},
		"ENABLED", schedulerbackend.FlexibleTimeWindow{Mode: "OFF"},
	)
	require.NoError(t, err)

	require.NoError(t, schedH.StartWorker(ctx))
	t.Cleanup(func() { schedH.Shutdown(ctx) })

	require.Eventually(t, func() bool {
		executions, _ := sagemakerH.Backend.ListPipelineExecutions(ctx, sagemakerbackend.ListPipelineExecutionsParams{
			PipelineName: pipelineName,
		})

		return len(executions) > 0
	}, 10*time.Second, 100*time.Millisecond,
		"a schedule targeting a SageMaker pipeline must actually start a pipeline execution "+
			"through the real cli.go composition root's scheduler wiring (wireSchedulerRunner), "+
			"not silently report success while creating no execution")
}
