package lambda_test

import (
	"testing"

	lambdasdk "github.com/aws/aws-sdk-go-v2/service/lambda"

	"github.com/blackbirdworks/gopherstack/pkgs/sdkcheck"
	"github.com/blackbirdworks/gopherstack/services/lambda"
)

// TestSDKCompleteness verifies that every operation exposed by the AWS SDK v2
// lambda client is either listed in GetSupportedOperations() or explicitly
// acknowledged in the notImplemented slice.  The test fails when the upstream
// SDK adds a new operation that gopherstack has not yet handled.
func TestSDKCompleteness(t *testing.T) {
	t.Parallel()

	// NewHandler accepts nil because GetSupportedOperations does not use the backend.
	h := lambda.NewHandler(nil)
	sdkcheck.CheckCompleteness(t, &lambdasdk.Client{}, h.GetSupportedOperations(), []string{
		// Added by the lambda SDK bump v1.101.2 -> v1.107.0; unimplemented.
		"DeleteResourcePolicy",
		"GetResourcePolicy",
		"PutResourcePolicy",
	})
}
