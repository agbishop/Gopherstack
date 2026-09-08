package codecommit_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	codecommitsdk "github.com/aws/aws-sdk-go-v2/service/codecommit"
	"github.com/aws/aws-sdk-go-v2/service/codecommit/types"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codecommit"
)

// newOrphanCodeTestHandler stands up a bare handler the same way
// newSentinelTestHandler does, for the mergeOption/pullRequestStatus/
// continuation-token sentinel tests below. None of these need any
// repository/PR state: the invalid-input check they exercise runs before
// the handler ever looks up backend state.
func newOrphanCodeTestHandler(t *testing.T) *codecommit.Handler {
	t.Helper()

	backend := codecommit.NewInMemoryBackend("123456789012", "us-east-1")

	return codecommit.NewHandler(backend)
}

// TestBatchDescribeMergeConflicts_InvalidMergeOption_InvalidMergeOptionException
// proves BatchDescribeMergeConflicts reports a bad mergeOption via
// InvalidMergeOptionException, not a fabricated "InvalidParameterException"
// -- confirmed against awsAwsjson11_deserializeOpErrorBatchDescribeMergeConflicts
// (codecommit@v1.36.4 deserializers.go), whose switch has no case for
// InvalidParameterException but does declare InvalidMergeOptionException.
func TestBatchDescribeMergeConflicts_InvalidMergeOption_InvalidMergeOptionException(t *testing.T) {
	t.Parallel()

	h := newOrphanCodeTestHandler(t)
	client := newTestCodeCommitClient(t, h)

	_, err := client.BatchDescribeMergeConflicts(t.Context(), &codecommitsdk.BatchDescribeMergeConflictsInput{
		RepositoryName:             aws.String("repo"),
		DestinationCommitSpecifier: aws.String("main"),
		SourceCommitSpecifier:      aws.String("feat"),
		MergeOption:                types.MergeOptionTypeEnum("BOGUS_MERGE_OPTION"),
	})
	require.Error(t, err)

	var imo *types.InvalidMergeOptionException
	require.ErrorAsf(t, err, &imo, "expected a real InvalidMergeOptionException from the SDK deserializer, got %v", err)
}

// TestCreateUnreferencedMergeCommit_InvalidMergeOption_InvalidMergeOptionException
// is TestBatchDescribeMergeConflicts_InvalidMergeOption_InvalidMergeOptionException's
// sibling for CreateUnreferencedMergeCommit -- confirmed against
// awsAwsjson11_deserializeOpErrorCreateUnreferencedMergeCommit.
func TestCreateUnreferencedMergeCommit_InvalidMergeOption_InvalidMergeOptionException(t *testing.T) {
	t.Parallel()

	h := newOrphanCodeTestHandler(t)
	client := newTestCodeCommitClient(t, h)

	_, err := client.CreateUnreferencedMergeCommit(t.Context(), &codecommitsdk.CreateUnreferencedMergeCommitInput{
		RepositoryName:             aws.String("repo"),
		DestinationCommitSpecifier: aws.String("main"),
		SourceCommitSpecifier:      aws.String("feat"),
		MergeOption:                types.MergeOptionTypeEnum("BOGUS_MERGE_OPTION"),
	})
	require.Error(t, err)

	var imo *types.InvalidMergeOptionException
	require.ErrorAsf(t, err, &imo, "expected a real InvalidMergeOptionException from the SDK deserializer, got %v", err)
}

// TestGetMergeConflicts_InvalidMergeOption_InvalidMergeOptionException is
// TestBatchDescribeMergeConflicts_InvalidMergeOption_InvalidMergeOptionException's
// sibling for GetMergeConflicts -- confirmed against
// awsAwsjson11_deserializeOpErrorGetMergeConflicts.
func TestGetMergeConflicts_InvalidMergeOption_InvalidMergeOptionException(t *testing.T) {
	t.Parallel()

	h := newOrphanCodeTestHandler(t)
	client := newTestCodeCommitClient(t, h)

	_, err := client.GetMergeConflicts(t.Context(), &codecommitsdk.GetMergeConflictsInput{
		RepositoryName:             aws.String("repo"),
		DestinationCommitSpecifier: aws.String("main"),
		SourceCommitSpecifier:      aws.String("feat"),
		MergeOption:                types.MergeOptionTypeEnum("BOGUS_MERGE_OPTION"),
	})
	require.Error(t, err)

	var imo *types.InvalidMergeOptionException
	require.ErrorAsf(t, err, &imo, "expected a real InvalidMergeOptionException from the SDK deserializer, got %v", err)
}

// TestDescribeMergeConflicts_InvalidMergeOption_InvalidMergeOptionException is
// TestBatchDescribeMergeConflicts_InvalidMergeOption_InvalidMergeOptionException's
// sibling for DescribeMergeConflicts -- confirmed against
// awsAwsjson11_deserializeOpErrorDescribeMergeConflicts.
func TestDescribeMergeConflicts_InvalidMergeOption_InvalidMergeOptionException(t *testing.T) {
	t.Parallel()

	h := newOrphanCodeTestHandler(t)
	client := newTestCodeCommitClient(t, h)

	_, err := client.DescribeMergeConflicts(t.Context(), &codecommitsdk.DescribeMergeConflictsInput{
		RepositoryName:             aws.String("repo"),
		DestinationCommitSpecifier: aws.String("main"),
		SourceCommitSpecifier:      aws.String("feat"),
		FilePath:                   aws.String("a.go"),
		MergeOption:                types.MergeOptionTypeEnum("BOGUS_MERGE_OPTION"),
	})
	require.Error(t, err)

	var imo *types.InvalidMergeOptionException
	require.ErrorAsf(t, err, &imo, "expected a real InvalidMergeOptionException from the SDK deserializer, got %v", err)
}

// TestListPullRequests_InvalidStatus_InvalidPullRequestStatusException proves
// ListPullRequests reports a bad pullRequestStatus via
// InvalidPullRequestStatusException, not a fabricated "InvalidParameterException"
// -- confirmed against awsAwsjson11_deserializeOpErrorListPullRequests, whose
// switch has no case for InvalidParameterException but does declare
// InvalidPullRequestStatusException.
func TestListPullRequests_InvalidStatus_InvalidPullRequestStatusException(t *testing.T) {
	t.Parallel()

	h := newOrphanCodeTestHandler(t)
	client := newTestCodeCommitClient(t, h)

	_, err := client.ListPullRequests(t.Context(), &codecommitsdk.ListPullRequestsInput{
		RepositoryName:    aws.String("repo"),
		PullRequestStatus: types.PullRequestStatusEnum("BOGUS_STATUS"),
	})
	require.Error(t, err)

	var ips *types.InvalidPullRequestStatusException
	require.ErrorAsf(
		t, err, &ips, "expected a real InvalidPullRequestStatusException from the SDK deserializer, got %v", err,
	)
}

// TestUpdatePullRequestStatus_InvalidStatus_InvalidPullRequestStatusException is
// TestListPullRequests_InvalidStatus_InvalidPullRequestStatusException's sibling
// for UpdatePullRequestStatus -- confirmed against
// awsAwsjson11_deserializeOpErrorUpdatePullRequestStatus.
func TestUpdatePullRequestStatus_InvalidStatus_InvalidPullRequestStatusException(t *testing.T) {
	t.Parallel()

	h := newOrphanCodeTestHandler(t)
	client := newTestCodeCommitClient(t, h)

	_, err := client.UpdatePullRequestStatus(t.Context(), &codecommitsdk.UpdatePullRequestStatusInput{
		PullRequestId:     aws.String("1"),
		PullRequestStatus: types.PullRequestStatusEnum("BOGUS_STATUS"),
	})
	require.Error(t, err)

	var ips *types.InvalidPullRequestStatusException
	require.ErrorAsf(
		t, err, &ips, "expected a real InvalidPullRequestStatusException from the SDK deserializer, got %v", err,
	)
}

// TestGetDifferences_InvalidNextToken_InvalidContinuationTokenException proves
// GetDifferences reports an undecodable NextToken via
// InvalidContinuationTokenException, not a fabricated "InvalidParameterException"
// -- confirmed against awsAwsjson11_deserializeOpErrorGetDifferences, whose
// switch has no case for InvalidParameterException but does declare
// InvalidContinuationTokenException.
func TestGetDifferences_InvalidNextToken_InvalidContinuationTokenException(t *testing.T) {
	t.Parallel()

	h := newOrphanCodeTestHandler(t)
	client := newTestCodeCommitClient(t, h)

	_, err := client.GetDifferences(t.Context(), &codecommitsdk.GetDifferencesInput{
		RepositoryName:       aws.String("repo"),
		AfterCommitSpecifier: aws.String("main"),
		NextToken:            aws.String("!!!not-valid-base64!!!"),
	})
	require.Error(t, err)

	var ict *types.InvalidContinuationTokenException
	require.ErrorAsf(
		t, err, &ict, "expected a real InvalidContinuationTokenException from the SDK deserializer, got %v", err,
	)
}

// TestListFileCommitHistory_InvalidNextToken_InvalidContinuationTokenException is
// TestGetDifferences_InvalidNextToken_InvalidContinuationTokenException's sibling
// for ListFileCommitHistory -- confirmed against
// awsAwsjson11_deserializeOpErrorListFileCommitHistory.
func TestListFileCommitHistory_InvalidNextToken_InvalidContinuationTokenException(t *testing.T) {
	t.Parallel()

	h := newOrphanCodeTestHandler(t)
	client := newTestCodeCommitClient(t, h)

	_, err := client.ListFileCommitHistory(t.Context(), &codecommitsdk.ListFileCommitHistoryInput{
		RepositoryName: aws.String("repo"),
		FilePath:       aws.String("a.go"),
		NextToken:      aws.String("!!!not-valid-base64!!!"),
	})
	require.Error(t, err)

	var ict *types.InvalidContinuationTokenException
	require.ErrorAsf(
		t, err, &ict, "expected a real InvalidContinuationTokenException from the SDK deserializer, got %v", err,
	)
}
