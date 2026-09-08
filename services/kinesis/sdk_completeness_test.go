package kinesis_test

import (
	"testing"

	kinesissdk "github.com/aws/aws-sdk-go-v2/service/kinesis"

	"github.com/blackbirdworks/gopherstack/pkgs/sdkcheck"
	"github.com/blackbirdworks/gopherstack/services/kinesis"
)

// TestSDKCompleteness verifies that every operation exposed by the AWS SDK v2
// kinesis client is either listed in GetSupportedOperations() or explicitly
// acknowledged in the notImplemented slice.  The test fails when the upstream
// SDK adds a new operation that gopherstack has not yet handled.
func TestSDKCompleteness(t *testing.T) {
	t.Parallel()

	backend := kinesis.NewInMemoryBackend()
	h := kinesis.NewHandler(backend)

	// Added by the aws-sdk-go-v2/service/kinesis v1.53.0 bump; unimplemented.
	notImplemented := []string{
		"CreateChannel",
		"DeleteChannel",
		"DescribeChannel",
		"ListChannels",
		"UpdateChannel",
	}
	sdkcheck.CheckCompleteness(t, &kinesissdk.Client{}, h.GetSupportedOperations(), notImplemented)
}
