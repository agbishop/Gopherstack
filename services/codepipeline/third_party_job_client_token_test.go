package codepipeline_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cpsdk "github.com/aws/aws-sdk-go-v2/service/codepipeline"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codepipeline"
)

func seedThirdPartyJob(h *codepipeline.Handler, id, nonce string) {
	h.Backend.AddJobInternal(&codepipeline.Job{
		ID:     id,
		Nonce:  nonce,
		Status: "Queued",
		ActionTypeID: codepipeline.ActionTypeID{
			Category: "Build", Owner: "ThirdParty", Provider: "MyBuild", Version: "1",
		},
	})
}

func pollForClientID(t *testing.T, client *cpsdk.Client) string {
	t.Helper()

	out, err := client.PollForThirdPartyJobs(t.Context(), &cpsdk.PollForThirdPartyJobsInput{
		ActionTypeId: &types.ActionTypeId{
			Category: types.ActionCategoryBuild, Owner: types.ActionOwnerThirdParty,
			Provider: aws.String("MyBuild"), Version: aws.String("1"),
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Jobs, 1)
	require.NotNil(t, out.Jobs[0].ClientId)
	require.NotEmpty(t, *out.Jobs[0].ClientId)

	return *out.Jobs[0].ClientId
}

// TestPollForThirdPartyJobs_IssuesClientID proves PollForThirdPartyJobs now
// issues a ClientId (gopherstack-1y6n): before the fix, jobId was the only
// field on the wire and nothing anywhere in the backend ever set ClientId,
// so a worker had no clientToken it could legitimately present to the four
// consumer operations.
func TestPollForThirdPartyJobs_IssuesClientID(t *testing.T) {
	t.Parallel()

	h := codepipeline.NewHandler(codepipeline.NewInMemoryBackend("123456789012", "us-east-1"))
	client := newTestCodePipelineClient(t, h)
	seedThirdPartyJob(h, "tp-issue", "nonce-issue")

	first := pollForClientID(t, client)
	second := pollForClientID(t, client)

	assert.Equal(t, first, second, "re-polling the same still-queued job must return the same ClientId")
}

// TestThirdPartyJobClientToken_RoundTrip drives the full worker flow through
// a real SDK client: poll for a job, get its ClientId back, and present that
// same value as clientToken on all four consumer operations. Before the fix
// this couldn't be proven at all -- PollForThirdPartyJobs never returned a
// ClientId, and GetThirdPartyJobDetails/PutThirdPartyJobSuccessResult/
// PutThirdPartyJobFailureResult discarded whatever clientToken was sent
// (`_ = clientToken`) instead of checking it against anything.
func TestThirdPartyJobClientToken_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result string // "success" or "failure"
	}{
		{name: "success result", result: "success"},
		{name: "failure result", result: "failure"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := codepipeline.NewHandler(codepipeline.NewInMemoryBackend("123456789012", "us-east-1"))
			client := newTestCodePipelineClient(t, h)
			seedThirdPartyJob(h, "tp-rt-"+tt.result, "nonce-rt")

			clientToken := pollForClientID(t, client)

			ackOut, err := client.AcknowledgeThirdPartyJob(t.Context(), &cpsdk.AcknowledgeThirdPartyJobInput{
				JobId:       aws.String("tp-rt-" + tt.result),
				Nonce:       aws.String("nonce-rt"),
				ClientToken: aws.String(clientToken),
			})
			require.NoError(t, err)
			assert.Equal(t, types.JobStatusInProgress, ackOut.Status)

			detailsOut, err := client.GetThirdPartyJobDetails(t.Context(), &cpsdk.GetThirdPartyJobDetailsInput{
				JobId: aws.String("tp-rt-" + tt.result), ClientToken: aws.String(clientToken),
			})
			require.NoError(t, err)
			require.NotNil(t, detailsOut.JobDetails)

			switch tt.result {
			case "success":
				_, err = client.PutThirdPartyJobSuccessResult(t.Context(), &cpsdk.PutThirdPartyJobSuccessResultInput{
					JobId: aws.String("tp-rt-" + tt.result), ClientToken: aws.String(clientToken),
				})
			default:
				_, err = client.PutThirdPartyJobFailureResult(t.Context(), &cpsdk.PutThirdPartyJobFailureResultInput{
					JobId: aws.String("tp-rt-" + tt.result), ClientToken: aws.String(clientToken),
					FailureDetails: &types.FailureDetails{
						Message: aws.String("boom"), Type: types.FailureTypeJobFailed,
					},
				})
			}
			require.NoError(t, err)
		})
	}
}

// TestThirdPartyJobClientToken_RejectsMismatch proves each of the four
// clientToken-consuming operations rejects a token that doesn't match the
// job's issued ClientId -- including a job that was never polled (so has no
// issued ClientId at all) -- with InvalidClientTokenException, the error
// code modeled on all four ops' deserializers.
func TestThirdPartyJobClientToken_RejectsMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		call func(t *testing.T, client *cpsdk.Client, jobID, badToken string) error
		name string
	}{
		{
			name: "AcknowledgeThirdPartyJob",
			call: func(t *testing.T, client *cpsdk.Client, jobID, badToken string) error {
				t.Helper()
				_, err := client.AcknowledgeThirdPartyJob(t.Context(), &cpsdk.AcknowledgeThirdPartyJobInput{
					JobId: aws.String(jobID), Nonce: aws.String("nonce-mismatch"), ClientToken: aws.String(badToken),
				})

				return err
			},
		},
		{
			name: "GetThirdPartyJobDetails",
			call: func(t *testing.T, client *cpsdk.Client, jobID, badToken string) error {
				t.Helper()
				_, err := client.GetThirdPartyJobDetails(t.Context(), &cpsdk.GetThirdPartyJobDetailsInput{
					JobId: aws.String(jobID), ClientToken: aws.String(badToken),
				})

				return err
			},
		},
		{
			name: "PutThirdPartyJobSuccessResult",
			call: func(t *testing.T, client *cpsdk.Client, jobID, badToken string) error {
				t.Helper()
				_, err := client.PutThirdPartyJobSuccessResult(t.Context(), &cpsdk.PutThirdPartyJobSuccessResultInput{
					JobId: aws.String(jobID), ClientToken: aws.String(badToken),
				})

				return err
			},
		},
		{
			name: "PutThirdPartyJobFailureResult",
			call: func(t *testing.T, client *cpsdk.Client, jobID, badToken string) error {
				t.Helper()
				_, err := client.PutThirdPartyJobFailureResult(t.Context(), &cpsdk.PutThirdPartyJobFailureResultInput{
					JobId: aws.String(jobID), ClientToken: aws.String(badToken),
					FailureDetails: &types.FailureDetails{
						Message: aws.String("boom"), Type: types.FailureTypeJobFailed,
					},
				})

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := codepipeline.NewHandler(codepipeline.NewInMemoryBackend("123456789012", "us-east-1"))
			client := newTestCodePipelineClient(t, h)
			seedThirdPartyJob(h, "tp-mismatch-"+tt.name, "nonce-mismatch")

			issued := pollForClientID(t, client)

			err := tt.call(t, client, "tp-mismatch-"+tt.name, issued+"-wrong")
			require.Error(t, err)

			var apiErr smithy.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, "InvalidClientTokenException", apiErr.ErrorCode())
		})
	}
}

// TestThirdPartyJobClientToken_UnpolledJobRejectsAnyToken proves a job that
// was never returned by PollForThirdPartyJobs (so never had a ClientId
// issued) rejects every clientToken, including the literal "token" this
// package's older tests used to pass without any backend check at all.
func TestThirdPartyJobClientToken_UnpolledJobRejectsAnyToken(t *testing.T) {
	t.Parallel()

	h := codepipeline.NewHandler(codepipeline.NewInMemoryBackend("123456789012", "us-east-1"))
	client := newTestCodePipelineClient(t, h)
	h.Backend.AddJobInternal(&codepipeline.Job{ID: "tp-unpolled", Nonce: "n", Status: "Queued"})

	_, err := client.GetThirdPartyJobDetails(t.Context(), &cpsdk.GetThirdPartyJobDetailsInput{
		JobId: aws.String("tp-unpolled"), ClientToken: aws.String("token"),
	})
	require.Error(t, err)

	var apiErr smithy.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "InvalidClientTokenException", apiErr.ErrorCode())
}

// TestPlainJobPath_UnaffectedByClientTokenCheck proves the non-third-party
// Job flow (AcknowledgeJob/PutJobSuccessResult), which carries no
// clientToken at all, is unaffected by the third-party ClientID/clientToken
// check added to the ThirdPartyJob* operations.
func TestPlainJobPath_UnaffectedByClientTokenCheck(t *testing.T) {
	t.Parallel()

	h := codepipeline.NewHandler(codepipeline.NewInMemoryBackend("123456789012", "us-east-1"))
	client := newTestCodePipelineClient(t, h)
	h.Backend.AddJobInternal(&codepipeline.Job{ID: "plain-job", Nonce: "n", Status: "Queued"})

	ackOut, err := client.AcknowledgeJob(t.Context(), &cpsdk.AcknowledgeJobInput{
		JobId: aws.String("plain-job"), Nonce: aws.String("n"),
	})
	require.NoError(t, err)
	assert.Equal(t, types.JobStatusInProgress, ackOut.Status)

	_, err = client.PutJobSuccessResult(t.Context(), &cpsdk.PutJobSuccessResultInput{
		JobId: aws.String("plain-job"),
	})
	require.NoError(t, err)
}
