package quicksight_test

import (
	"testing"

	quicksightsdk "github.com/aws/aws-sdk-go-v2/service/quicksight"

	"github.com/blackbirdworks/gopherstack/pkgs/sdkcheck"
	"github.com/blackbirdworks/gopherstack/services/quicksight"
)

// TestSDKCompleteness verifies that every operation exposed by the AWS SDK v2
// quicksight client is either listed in GetSupportedOperations() or explicitly
// acknowledged in the notImplemented slice.
func TestSDKCompleteness(t *testing.T) {
	t.Parallel()

	backend := quicksight.NewInMemoryBackend("000000000000", "us-east-1")
	h := quicksight.NewHandler(backend)

	notImplemented := []string{
		// Added by the quicksight SDK bump v1.123.1 -> v1.129.0; unimplemented.
		"BatchDescribeUserLimits",
		"CreateApprovalPolicy",
		"CreateDlpSetting",
		"CreateLimitsProfile",
		"DeleteApp",
		"DeleteApprovalPolicy",
		"DeleteDlpSetting",
		"DeleteLimitsProfile",
		"DescribeApp",
		"DescribeAppPermissions",
		"DescribeApprovalPolicy",
		"DescribeDlpSetting",
		"DescribeLimitsProfile",
		"ListApprovalPolicies",
		"ListApps",
		"ListDlpSettings",
		"ListLimitsProfiles",
		"SearchApps",
		"UpdateAppPermissions",
		"UpdateApprovalPolicy",
		"UpdateDlpSetting",
		"UpdateLimitsProfile",
	}

	sdkcheck.CheckCompleteness(t, &quicksightsdk.Client{}, h.GetSupportedOperations(), notImplemented)
}
