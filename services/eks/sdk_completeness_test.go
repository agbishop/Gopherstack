package eks_test

import (
	"testing"

	ekssdk "github.com/aws/aws-sdk-go-v2/service/eks"

	"github.com/blackbirdworks/gopherstack/pkgs/sdkcheck"
	"github.com/blackbirdworks/gopherstack/services/eks"
)

// TestSDKCompleteness verifies that every operation exposed by the AWS SDK v2
// eks client is either listed in GetSupportedOperations() or explicitly
// acknowledged in the notImplemented slice.  The test fails when the upstream
// SDK adds a new operation that gopherstack has not yet handled.
func TestSDKCompleteness(t *testing.T) {
	t.Parallel()

	backend := eks.NewInMemoryBackend(t.Context(), "000000000000", "us-east-1")
	h := eks.NewHandler(backend)
	sdkcheck.CheckCompleteness(t, &ekssdk.Client{}, h.GetSupportedOperations(), []string{
		// Added by the eks SDK bump v1.90.4 -> v1.98.0; unimplemented.
		"ActivateCertificateAuthority",
		"CreateCertificateAuthority",
		"DeleteCertificateAuthority",
		"DescribeCertificateAuthority",
		"ListCertificateAuthorities",
	})
}
