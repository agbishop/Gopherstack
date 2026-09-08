package acm_test

import (
	"testing"

	acmsdk "github.com/aws/aws-sdk-go-v2/service/acm"

	"github.com/blackbirdworks/gopherstack/pkgs/sdkcheck"
	"github.com/blackbirdworks/gopherstack/services/acm"
)

// TestSDKCompleteness verifies that every operation exposed by the AWS SDK v2
// acm client is either listed in GetSupportedOperations() or explicitly
// acknowledged in the notImplemented slice.  The test fails when the upstream
// SDK adds a new operation that gopherstack has not yet handled.
func TestSDKCompleteness(t *testing.T) {
	t.Parallel()

	backend := acm.NewInMemoryBackend("000000000000", "us-east-1")
	h := acm.NewHandler(backend)

	// Added by the aws-sdk-go-v2/service/acm v1.49.0 bump; unimplemented.
	notImplemented := []string{
		"ListCertificateDomainValidations",
	}
	sdkcheck.CheckCompleteness(t, &acmsdk.Client{}, h.GetSupportedOperations(), notImplemented)
}
