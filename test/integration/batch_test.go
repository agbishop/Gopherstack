package integration_test

import (
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/batch"
	batchtypes "github.com/aws/aws-sdk-go-v2/service/batch/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_Batch_ComputeEnvironmentLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createBatchClient(t)
	ctx := t.Context()

	ceName := "test-ce-" + uuid.NewString()[:8]

	// CreateComputeEnvironment
	createOut, err := client.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String(ceName),
		Type:                   batchtypes.CETypeManaged,
		State:                  batchtypes.CEStateEnabled,
	})
	require.NoError(t, err)
	assert.Equal(t, ceName, aws.ToString(createOut.ComputeEnvironmentName))
	assert.NotEmpty(t, aws.ToString(createOut.ComputeEnvironmentArn))

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.UpdateComputeEnvironment(cleanupCtx, &batch.UpdateComputeEnvironmentInput{
			ComputeEnvironment: aws.String(ceName),
			State:              batchtypes.CEStateDisabled,
		})
		_, _ = client.DeleteComputeEnvironment(cleanupCtx, &batch.DeleteComputeEnvironmentInput{
			ComputeEnvironment: aws.String(ceName),
		})
	})

	// DescribeComputeEnvironments
	descOut, err := client.DescribeComputeEnvironments(ctx, &batch.DescribeComputeEnvironmentsInput{
		ComputeEnvironments: []string{ceName},
	})
	require.NoError(t, err)
	require.Len(t, descOut.ComputeEnvironments, 1)
	assert.Equal(t, ceName, aws.ToString(descOut.ComputeEnvironments[0].ComputeEnvironmentName))

	// DescribeComputeEnvironments - list all
	listOut, err := client.DescribeComputeEnvironments(ctx, &batch.DescribeComputeEnvironmentsInput{})
	require.NoError(t, err)

	found := false

	for _, ce := range listOut.ComputeEnvironments {
		if aws.ToString(ce.ComputeEnvironmentName) == ceName {
			found = true

			break
		}
	}

	assert.True(t, found, "created compute environment should appear in list")

	// Disable before delete (required by AWS)
	_, err = client.UpdateComputeEnvironment(ctx, &batch.UpdateComputeEnvironmentInput{
		ComputeEnvironment: aws.String(ceName),
		State:              batchtypes.CEStateDisabled,
	})
	require.NoError(t, err)

	// DeleteComputeEnvironment
	_, err = client.DeleteComputeEnvironment(ctx, &batch.DeleteComputeEnvironmentInput{
		ComputeEnvironment: aws.String(ceName),
	})
	require.NoError(t, err)

	// Verify deleted
	descOut2, err := client.DescribeComputeEnvironments(ctx, &batch.DescribeComputeEnvironmentsInput{
		ComputeEnvironments: []string{ceName},
	})
	require.NoError(t, err)
	assert.Empty(t, descOut2.ComputeEnvironments)
}

func TestIntegration_Batch_JobQueueLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createBatchClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	ceName := "jq-ce-" + suffix
	jqName := "test-jq-" + suffix

	// Create compute environment first
	ceOut, err := client.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String(ceName),
		Type:                   batchtypes.CETypeManaged,
		State:                  batchtypes.CEStateEnabled,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.UpdateJobQueue(cleanupCtx, &batch.UpdateJobQueueInput{
			JobQueue: aws.String(jqName),
			State:    batchtypes.JQStateDisabled,
		})
		_, _ = client.DeleteJobQueue(cleanupCtx, &batch.DeleteJobQueueInput{
			JobQueue: aws.String(jqName),
		})
		_, _ = client.UpdateComputeEnvironment(cleanupCtx, &batch.UpdateComputeEnvironmentInput{
			ComputeEnvironment: aws.String(ceName),
			State:              batchtypes.CEStateDisabled,
		})
		_, _ = client.DeleteComputeEnvironment(cleanupCtx, &batch.DeleteComputeEnvironmentInput{
			ComputeEnvironment: aws.String(ceName),
		})
	})

	// CreateJobQueue
	createOut, err := client.CreateJobQueue(ctx, &batch.CreateJobQueueInput{
		JobQueueName: aws.String(jqName),
		Priority:     aws.Int32(10),
		State:        batchtypes.JQStateEnabled,
		ComputeEnvironmentOrder: []batchtypes.ComputeEnvironmentOrder{
			{
				ComputeEnvironment: ceOut.ComputeEnvironmentArn,
				Order:              aws.Int32(1),
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, jqName, aws.ToString(createOut.JobQueueName))
	assert.NotEmpty(t, aws.ToString(createOut.JobQueueArn))

	// DescribeJobQueues
	descOut, err := client.DescribeJobQueues(ctx, &batch.DescribeJobQueuesInput{
		JobQueues: []string{jqName},
	})
	require.NoError(t, err)
	require.Len(t, descOut.JobQueues, 1)
	assert.Equal(t, jqName, aws.ToString(descOut.JobQueues[0].JobQueueName))

	// Disable before delete (required)
	_, err = client.UpdateJobQueue(ctx, &batch.UpdateJobQueueInput{
		JobQueue: aws.String(jqName),
		State:    batchtypes.JQStateDisabled,
	})
	require.NoError(t, err)

	// DeleteJobQueue
	_, err = client.DeleteJobQueue(ctx, &batch.DeleteJobQueueInput{
		JobQueue: aws.String(jqName),
	})
	require.NoError(t, err)

	// Verify deleted
	descOut2, err := client.DescribeJobQueues(ctx, &batch.DescribeJobQueuesInput{
		JobQueues: []string{jqName},
	})
	require.NoError(t, err)
	assert.Empty(t, descOut2.JobQueues)
}

func TestIntegration_Batch_JobDefinitionLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createBatchClient(t)
	ctx := t.Context()

	jdName := "test-jd-" + uuid.NewString()[:8]

	// RegisterJobDefinition
	registerOut, err := client.RegisterJobDefinition(ctx, &batch.RegisterJobDefinitionInput{
		JobDefinitionName: aws.String(jdName),
		Type:              batchtypes.JobDefinitionTypeContainer,
	})
	require.NoError(t, err)
	assert.Equal(t, jdName, aws.ToString(registerOut.JobDefinitionName))
	assert.NotEmpty(t, aws.ToString(registerOut.JobDefinitionArn))
	assert.Equal(t, int32(1), aws.ToInt32(registerOut.Revision))

	jdARN := aws.ToString(registerOut.JobDefinitionArn)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeregisterJobDefinition(cleanupCtx, &batch.DeregisterJobDefinitionInput{
			JobDefinition: aws.String(jdARN),
		})
	})

	// DescribeJobDefinitions
	descOut, err := client.DescribeJobDefinitions(ctx, &batch.DescribeJobDefinitionsInput{
		JobDefinitionName: aws.String(jdName),
	})
	require.NoError(t, err)
	require.NotEmpty(t, descOut.JobDefinitions)
	assert.Equal(t, jdName, aws.ToString(descOut.JobDefinitions[0].JobDefinitionName))

	// DeregisterJobDefinition
	_, err = client.DeregisterJobDefinition(ctx, &batch.DeregisterJobDefinitionInput{
		JobDefinition: aws.String(jdARN),
	})
	require.NoError(t, err)

	// Verify inactive after deregister — status filter is not supported by the handler,
	// so query by name and check the status field directly.
	descOut2, err := client.DescribeJobDefinitions(ctx, &batch.DescribeJobDefinitionsInput{
		JobDefinitionName: aws.String(jdName),
	})
	require.NoError(t, err)
	require.Len(t, descOut2.JobDefinitions, 1)
	assert.Equal(t, "INACTIVE", aws.ToString(descOut2.JobDefinitions[0].Status))
}

func TestIntegration_Batch_ConsumableResourceLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createBatchClient(t)
	ctx := t.Context()

	crName := "test-cr-" + uuid.NewString()[:8]

	// Create
	createOut, err := client.CreateConsumableResource(ctx, &batch.CreateConsumableResourceInput{
		ConsumableResourceName: aws.String(crName),
		TotalQuantity:          aws.Int64(100),
	})
	require.NoError(t, err)
	assert.Equal(t, crName, aws.ToString(createOut.ConsumableResourceName))
	assert.NotEmpty(t, aws.ToString(createOut.ConsumableResourceArn))

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteConsumableResource(cleanupCtx, &batch.DeleteConsumableResourceInput{
			ConsumableResource: aws.String(crName),
		})
	})

	// Describe
	descOut, err := client.DescribeConsumableResource(ctx, &batch.DescribeConsumableResourceInput{
		ConsumableResource: aws.String(crName),
	})
	require.NoError(t, err)
	assert.Equal(t, crName, aws.ToString(descOut.ConsumableResourceName))

	// Delete
	_, err = client.DeleteConsumableResource(ctx, &batch.DeleteConsumableResourceInput{
		ConsumableResource: aws.String(crName),
	})
	require.NoError(t, err)
}

func TestIntegration_Batch_SchedulingPolicyLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createBatchClient(t)
	ctx := t.Context()

	spName := "test-sp-" + uuid.NewString()[:8]

	// Create
	createOut, err := client.CreateSchedulingPolicy(ctx, &batch.CreateSchedulingPolicyInput{
		Name: aws.String(spName),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, aws.ToString(createOut.Arn))

	spArn := aws.ToString(createOut.Arn)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteSchedulingPolicy(cleanupCtx, &batch.DeleteSchedulingPolicyInput{
			Arn: aws.String(spArn),
		})
	})

	// ListSchedulingPolicies
	listOut, err := client.ListSchedulingPolicies(ctx, &batch.ListSchedulingPoliciesInput{})
	require.NoError(t, err)

	found := false

	for _, sp := range listOut.SchedulingPolicies {
		if aws.ToString(sp.Arn) == spArn {
			found = true

			break
		}
	}

	assert.True(t, found, "created scheduling policy should appear in list")

	// Delete
	_, err = client.DeleteSchedulingPolicy(ctx, &batch.DeleteSchedulingPolicyInput{
		Arn: aws.String(spArn),
	})
	require.NoError(t, err)
}

func TestIntegration_Batch_ServiceEnvironmentLifecycle(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createBatchClient(t)
	ctx := t.Context()

	seName := "test-se-" + uuid.NewString()[:8]

	// Create
	createOut, err := client.CreateServiceEnvironment(ctx, &batch.CreateServiceEnvironmentInput{
		ServiceEnvironmentName: aws.String(seName),
		ServiceEnvironmentType: batchtypes.ServiceEnvironmentTypeSagemakerTraining,
		State:                  batchtypes.ServiceEnvironmentStateEnabled,
		CapacityLimits: []batchtypes.CapacityLimit{
			{
				CapacityUnit: aws.String("NUM_INSTANCES"),
				MaxCapacity:  aws.Int32(10),
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, seName, aws.ToString(createOut.ServiceEnvironmentName))
	assert.NotEmpty(t, aws.ToString(createOut.ServiceEnvironmentArn))

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.DeleteServiceEnvironment(cleanupCtx, &batch.DeleteServiceEnvironmentInput{
			ServiceEnvironment: aws.String(seName),
		})
	})

	// Describe
	descOut, err := client.DescribeServiceEnvironments(ctx, &batch.DescribeServiceEnvironmentsInput{
		ServiceEnvironments: []string{seName},
	})
	require.NoError(t, err)
	require.Len(t, descOut.ServiceEnvironments, 1)
	assert.Equal(t, seName, aws.ToString(descOut.ServiceEnvironments[0].ServiceEnvironmentName))

	// AWS requires DISABLED before delete (api_op_DeleteServiceEnvironment.go).
	_, err = client.UpdateServiceEnvironment(ctx, &batch.UpdateServiceEnvironmentInput{
		ServiceEnvironment: aws.String(seName),
		State:              batchtypes.ServiceEnvironmentStateDisabled,
	})
	require.NoError(t, err)

	// Delete
	_, err = client.DeleteServiceEnvironment(ctx, &batch.DeleteServiceEnvironmentInput{
		ServiceEnvironment: aws.String(seName),
	})
	require.NoError(t, err)
}

func TestIntegration_Batch_JobDefinitionRevisions(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createBatchClient(t)
	ctx := t.Context()

	defName := "test-def-rev-" + uuid.NewString()[:8]

	// Register two revisions.
	for range 2 {
		_, err := client.RegisterJobDefinition(ctx, &batch.RegisterJobDefinitionInput{
			JobDefinitionName: aws.String(defName),
			Type:              batchtypes.JobDefinitionTypeContainer,
			ContainerProperties: &batchtypes.ContainerProperties{
				Image: aws.String("busybox"),
			},
		})
		require.NoError(t, err)
	}

	resp, err := client.DescribeJobDefinitions(ctx, &batch.DescribeJobDefinitionsInput{
		JobDefinitionName: aws.String(defName),
		Status:            aws.String("ACTIVE"),
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(resp.JobDefinitions), 2)

	// Verify descending revision order.
	for i := 1; i < len(resp.JobDefinitions); i++ {
		assert.GreaterOrEqual(
			t,
			aws.ToInt32(resp.JobDefinitions[i-1].Revision),
			aws.ToInt32(resp.JobDefinitions[i].Revision),
		)
	}
}

func TestIntegration_Batch_ListJobsAllQueues(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createBatchClient(t)
	ctx := t.Context()

	ceName := "test-ce-all-" + uuid.NewString()[:8]
	_, err := client.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String(ceName),
		Type:                   batchtypes.CETypeUnmanaged,
		State:                  batchtypes.CEStateEnabled,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.UpdateComputeEnvironment(cleanupCtx, &batch.UpdateComputeEnvironmentInput{
			ComputeEnvironment: aws.String(ceName),
			State:              batchtypes.CEStateDisabled,
		})
		_, _ = client.DeleteComputeEnvironment(cleanupCtx, &batch.DeleteComputeEnvironmentInput{
			ComputeEnvironment: aws.String(ceName),
		})
	})

	qName := "test-q-all-" + uuid.NewString()[:8]
	_, err = client.CreateJobQueue(ctx, &batch.CreateJobQueueInput{
		JobQueueName: aws.String(qName),
		Priority:     aws.Int32(1),
		State:        batchtypes.JQStateEnabled,
		ComputeEnvironmentOrder: []batchtypes.ComputeEnvironmentOrder{
			{ComputeEnvironment: aws.String(ceName), Order: aws.Int32(1)},
		},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.UpdateJobQueue(cleanupCtx, &batch.UpdateJobQueueInput{
			JobQueue: aws.String(qName),
			State:    batchtypes.JQStateDisabled,
		})
		_, _ = client.DeleteJobQueue(cleanupCtx, &batch.DeleteJobQueueInput{
			JobQueue: aws.String(qName),
		})
	})

	jdName := "test-jd-all-" + uuid.NewString()[:8]
	_, err = client.RegisterJobDefinition(ctx, &batch.RegisterJobDefinitionInput{
		JobDefinitionName: aws.String(jdName),
		Type:              batchtypes.JobDefinitionTypeContainer,
	})
	require.NoError(t, err)

	jobName := "test-job-all-" + uuid.NewString()[:8]
	submitOut, err := client.SubmitJob(ctx, &batch.SubmitJobInput{
		JobName:       aws.String(jobName),
		JobQueue:      aws.String(qName),
		JobDefinition: aws.String(jdName),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, aws.ToString(submitOut.JobId))

	// AWS Batch ListJobs requires a grouping key (jobQueue, arrayJobId, or
	// multiNodeJobId); a no-arg "all queues" listing is rejected with a
	// ClientException. To list across all queues, enumerate the queues and
	// call ListJobs per-queue, aggregating the results.
	queuesOut, err := client.DescribeJobQueues(ctx, &batch.DescribeJobQueuesInput{})
	require.NoError(t, err)

	found := false
	for _, q := range queuesOut.JobQueues {
		// Real AWS Batch defaults an unfiltered ListJobs to RUNNING-only; the
		// unmanaged CE here never schedules the job past SUBMITTED, so filter
		// explicitly for it.
		listOut, lerr := client.ListJobs(ctx, &batch.ListJobsInput{
			JobQueue:  q.JobQueueName,
			JobStatus: batchtypes.JobStatusSubmitted,
		})
		require.NoError(t, lerr)
		for _, s := range listOut.JobSummaryList {
			if aws.ToString(s.JobId) == aws.ToString(submitOut.JobId) {
				found = true

				break
			}
		}
		if found {
			break
		}
	}
	assert.True(t, found, "submitted job should appear in per-queue list aggregation")
}

func TestIntegration_Batch_UpdateJobQueue_ComputeEnvironments(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createBatchClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	ce1Name := "updjq-ce1-" + suffix
	ce2Name := "updjq-ce2-" + suffix
	jqName := "updjq-jq-" + suffix

	// Create two compute environments.
	for _, ceName := range []string{ce1Name, ce2Name} {
		name := ceName
		_, err := client.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
			ComputeEnvironmentName: aws.String(name),
			Type:                   batchtypes.CETypeUnmanaged,
			State:                  batchtypes.CEStateEnabled,
		})
		require.NoError(t, err)

		t.Cleanup(func() {
			cleanupCtx, cancel := cleanupContext(t)
			defer cancel()

			_, _ = client.UpdateComputeEnvironment(cleanupCtx, &batch.UpdateComputeEnvironmentInput{
				ComputeEnvironment: aws.String(name),
				State:              batchtypes.CEStateDisabled,
			})
			_, _ = client.DeleteComputeEnvironment(cleanupCtx, &batch.DeleteComputeEnvironmentInput{
				ComputeEnvironment: aws.String(name),
			})
		})
	}

	// Create job queue with ce1 only.
	_, err := client.CreateJobQueue(ctx, &batch.CreateJobQueueInput{
		JobQueueName: aws.String(jqName),
		Priority:     aws.Int32(5),
		State:        batchtypes.JQStateEnabled,
		ComputeEnvironmentOrder: []batchtypes.ComputeEnvironmentOrder{
			{ComputeEnvironment: aws.String(ce1Name), Order: aws.Int32(1)},
		},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.UpdateJobQueue(cleanupCtx, &batch.UpdateJobQueueInput{
			JobQueue: aws.String(jqName),
			State:    batchtypes.JQStateDisabled,
		})
		_, _ = client.DeleteJobQueue(cleanupCtx, &batch.DeleteJobQueueInput{
			JobQueue: aws.String(jqName),
		})
	})

	// Update the job queue to use ce2 and change priority.
	newPriority := int32(20)
	_, err = client.UpdateJobQueue(ctx, &batch.UpdateJobQueueInput{
		JobQueue: aws.String(jqName),
		Priority: aws.Int32(newPriority),
		ComputeEnvironmentOrder: []batchtypes.ComputeEnvironmentOrder{
			{ComputeEnvironment: aws.String(ce2Name), Order: aws.Int32(1)},
		},
	})
	require.NoError(t, err)

	// Verify updates were applied.
	descOut, err := client.DescribeJobQueues(ctx, &batch.DescribeJobQueuesInput{
		JobQueues: []string{jqName},
	})
	require.NoError(t, err)
	require.Len(t, descOut.JobQueues, 1)
	assert.Equal(t, newPriority, aws.ToInt32(descOut.JobQueues[0].Priority))
	require.Len(t, descOut.JobQueues[0].ComputeEnvironmentOrder, 1)
	assert.Equal(t, ce2Name, aws.ToString(descOut.JobQueues[0].ComputeEnvironmentOrder[0].ComputeEnvironment))
}

func TestIntegration_Batch_SubmitJob_DisabledQueue(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createBatchClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	ceName := "disabled-ce-" + suffix
	jqName := "disabled-jq-" + suffix
	jdName := "disabled-jd-" + suffix

	_, err := client.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String(ceName),
		Type:                   batchtypes.CETypeUnmanaged,
		State:                  batchtypes.CEStateEnabled,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.UpdateComputeEnvironment(cleanupCtx, &batch.UpdateComputeEnvironmentInput{
			ComputeEnvironment: aws.String(ceName),
			State:              batchtypes.CEStateDisabled,
		})
		_, _ = client.DeleteComputeEnvironment(cleanupCtx, &batch.DeleteComputeEnvironmentInput{
			ComputeEnvironment: aws.String(ceName),
		})
	})

	_, err = client.CreateJobQueue(ctx, &batch.CreateJobQueueInput{
		JobQueueName: aws.String(jqName),
		Priority:     aws.Int32(1),
		State:        batchtypes.JQStateEnabled,
		ComputeEnvironmentOrder: []batchtypes.ComputeEnvironmentOrder{
			{ComputeEnvironment: aws.String(ceName), Order: aws.Int32(1)},
		},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.UpdateJobQueue(cleanupCtx, &batch.UpdateJobQueueInput{
			JobQueue: aws.String(jqName),
			State:    batchtypes.JQStateDisabled,
		})
		_, _ = client.DeleteJobQueue(cleanupCtx, &batch.DeleteJobQueueInput{
			JobQueue: aws.String(jqName),
		})
	})

	_, err = client.RegisterJobDefinition(ctx, &batch.RegisterJobDefinitionInput{
		JobDefinitionName: aws.String(jdName),
		Type:              batchtypes.JobDefinitionTypeContainer,
	})
	require.NoError(t, err)

	// Disable the queue.
	_, err = client.UpdateJobQueue(ctx, &batch.UpdateJobQueueInput{
		JobQueue: aws.String(jqName),
		State:    batchtypes.JQStateDisabled,
	})
	require.NoError(t, err)

	// Submitting a job to a disabled queue must fail.
	_, err = client.SubmitJob(ctx, &batch.SubmitJobInput{
		JobName:       aws.String("should-fail"),
		JobQueue:      aws.String(jqName),
		JobDefinition: aws.String(jdName),
	})
	assert.Error(t, err, "submitting to a DISABLED queue should return an error")
}

func TestIntegration_Batch_ListJobs_ByStatus(t *testing.T) {
	t.Parallel()
	dumpContainerLogsOnFailure(t)

	client := createBatchClient(t)
	ctx := t.Context()

	suffix := uuid.NewString()[:8]
	ceName := "liststs-ce-" + suffix
	jqName := "liststs-jq-" + suffix
	jdName := "liststs-jd-" + suffix

	_, err := client.CreateComputeEnvironment(ctx, &batch.CreateComputeEnvironmentInput{
		ComputeEnvironmentName: aws.String(ceName),
		Type:                   batchtypes.CETypeUnmanaged,
		State:                  batchtypes.CEStateEnabled,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.UpdateComputeEnvironment(cleanupCtx, &batch.UpdateComputeEnvironmentInput{
			ComputeEnvironment: aws.String(ceName),
			State:              batchtypes.CEStateDisabled,
		})
		_, _ = client.DeleteComputeEnvironment(cleanupCtx, &batch.DeleteComputeEnvironmentInput{
			ComputeEnvironment: aws.String(ceName),
		})
	})

	_, err = client.CreateJobQueue(ctx, &batch.CreateJobQueueInput{
		JobQueueName: aws.String(jqName),
		Priority:     aws.Int32(1),
		State:        batchtypes.JQStateEnabled,
		ComputeEnvironmentOrder: []batchtypes.ComputeEnvironmentOrder{
			{ComputeEnvironment: aws.String(ceName), Order: aws.Int32(1)},
		},
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext(t)
		defer cancel()

		_, _ = client.UpdateJobQueue(cleanupCtx, &batch.UpdateJobQueueInput{
			JobQueue: aws.String(jqName),
			State:    batchtypes.JQStateDisabled,
		})
		_, _ = client.DeleteJobQueue(cleanupCtx, &batch.DeleteJobQueueInput{
			JobQueue: aws.String(jqName),
		})
	})

	_, err = client.RegisterJobDefinition(ctx, &batch.RegisterJobDefinitionInput{
		JobDefinitionName: aws.String(jdName),
		Type:              batchtypes.JobDefinitionTypeContainer,
	})
	require.NoError(t, err)

	// Submit two jobs.
	for i := range 2 {
		_, err = client.SubmitJob(ctx, &batch.SubmitJobInput{
			JobName:       aws.String("list-status-job-" + suffix + "-" + strconv.Itoa(i)),
			JobQueue:      aws.String(jqName),
			JobDefinition: aws.String(jdName),
		})
		require.NoError(t, err)
	}

	// List with SUBMITTED status filter - should return both jobs.
	listOut, err := client.ListJobs(ctx, &batch.ListJobsInput{
		JobQueue:  aws.String(jqName),
		JobStatus: batchtypes.JobStatusSubmitted,
	})
	require.NoError(t, err)
	assert.Len(t, listOut.JobSummaryList, 2)

	// List with RUNNING status filter - should return zero jobs.
	runningOut, err := client.ListJobs(ctx, &batch.ListJobsInput{
		JobQueue:  aws.String(jqName),
		JobStatus: batchtypes.JobStatusRunning,
	})
	require.NoError(t, err)
	assert.Empty(t, runningOut.JobSummaryList)
}
