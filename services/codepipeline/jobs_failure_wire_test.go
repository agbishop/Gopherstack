package codepipeline_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cpsdk "github.com/aws/aws-sdk-go-v2/service/codepipeline"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codepipeline"
)

// TestPutJobFailureResult_StoresFailureDetails proves a real SDK client's
// FailureDetails.Message/Type survive PutJobFailureResult instead of being
// discarded. Before the fix, PutJobFailureResult's backend method threw the
// message away with `_ = message` and never even parsed Type off the wire
// (putJobFailureResultInput.FailureDetails had no Type field), so a worker
// reporting why a job failed left no trace anywhere in the backend.
func TestPutJobFailureResult_StoresFailureDetails(t *testing.T) {
	t.Parallel()

	h := codepipeline.NewHandler(codepipeline.NewInMemoryBackend("123456789012", "us-east-1"))
	client := newTestCodePipelineClient(t, h)

	h.Backend.AddJobInternal(&codepipeline.Job{ID: "job-fail-wire", Nonce: "n", Status: "InProgress"})

	_, err := client.PutJobFailureResult(t.Context(), &cpsdk.PutJobFailureResultInput{
		JobId: aws.String("job-fail-wire"),
		FailureDetails: &types.FailureDetails{
			Message: aws.String("build step exited 1"),
			Type:    types.FailureTypeJobFailed,
		},
	})
	require.NoError(t, err)

	job, err := h.Backend.GetJobDetails(t.Context(), "job-fail-wire")
	require.NoError(t, err)
	assert.Equal(t, "build step exited 1", job.FailureMessage)
	assert.Equal(t, string(types.FailureTypeJobFailed), job.FailureType)
}

// TestPutThirdPartyJobFailureResult_StoresFailureDetails is the same proof
// for the third-party job path, which stores the same status/failure details
// once the clientToken matches the job's issued ClientID.
func TestPutThirdPartyJobFailureResult_StoresFailureDetails(t *testing.T) {
	t.Parallel()

	h := codepipeline.NewHandler(codepipeline.NewInMemoryBackend("123456789012", "us-east-1"))
	client := newTestCodePipelineClient(t, h)

	h.Backend.AddJobInternal(&codepipeline.Job{
		ID: "tp-job-fail-wire", Nonce: "n", Status: "InProgress", ClientID: "token-abc",
	})

	_, err := client.PutThirdPartyJobFailureResult(t.Context(), &cpsdk.PutThirdPartyJobFailureResultInput{
		JobId:       aws.String("tp-job-fail-wire"),
		ClientToken: aws.String("token-abc"),
		FailureDetails: &types.FailureDetails{
			Message: aws.String("external system rejected the revision"),
			Type:    types.FailureTypeRevisionOutOfSync,
		},
	})
	require.NoError(t, err)

	job, err := h.Backend.GetJobDetails(t.Context(), "tp-job-fail-wire")
	require.NoError(t, err)
	assert.Equal(t, "external system rejected the revision", job.FailureMessage)
	assert.Equal(t, string(types.FailureTypeRevisionOutOfSync), job.FailureType)
}
