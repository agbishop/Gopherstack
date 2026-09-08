package acm_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/acm"
)

// TestJanitor_TimesOutAbandonedPendingValidation drives the janitor sweep --
// not TimeoutPendingValidation called directly -- to cover a certificate
// stuck in PENDING_VALIDATION past AWS's 72h validation window (see
// aws-sdk-go-v2/service/acm@v1.43.4 types/types.go CertificateDetail.Status
// doc) transitioning to VALIDATION_TIMED_OUT. This is coverage, not a
// regression test: the pre-refactor inline sweep already produced this same
// status transition.
func TestJanitor_TimesOutAbandonedPendingValidation(t *testing.T) {
	t.Parallel()

	b := acm.NewInMemoryBackend("000000000000", "us-east-1")
	b.SetAutoValidateDelayForTest(time.Hour)

	cert, err := b.RequestCertificate(context.Background(), "stuck.example.com", "", "DNS", "", "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, "PENDING_VALIDATION", cert.Status)

	b.BackdateCertForTest("us-east-1", cert.ARN, time.Now().UTC().Add(-73*time.Hour), cert.NotAfter)

	b.SweepJanitorOnceForTest()

	got, err := b.DescribeCertificate(context.Background(), cert.ARN)
	require.NoError(t, err)
	require.Equal(t, "VALIDATION_TIMED_OUT", got.Status)
}

// TestJanitor_ExpiresPastNotAfter drives the janitor sweep -- not
// ExpireCertificate called directly -- to cover an ISSUED certificate whose
// NotAfter has passed transitioning to EXPIRED. This is coverage, not a
// regression test: the pre-refactor inline sweep already produced this same
// status transition.
func TestJanitor_ExpiresPastNotAfter(t *testing.T) {
	t.Parallel()

	b := acm.NewInMemoryBackend("000000000000", "us-east-1")

	cert, err := b.RequestCertificate(context.Background(), "expiring.example.com", "", "DNS", "", "", "", "", nil)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		c, descErr := b.DescribeCertificate(context.Background(), cert.ARN)

		return descErr == nil && c.Status == "ISSUED"
	}, 2*time.Second, 50*time.Millisecond, "certificate must auto-validate to ISSUED")

	b.BackdateCertForTest("us-east-1", cert.ARN, cert.CreatedAt, time.Now().UTC().Add(-time.Hour))

	b.SweepJanitorOnceForTest()

	got, err := b.DescribeCertificate(context.Background(), cert.ARN)
	require.NoError(t, err)
	require.Equal(t, "EXPIRED", got.Status)
}

// TestJanitor_TimeoutDoesNotSetFailureReason is a regression test: the
// pre-refactor inline sweep set FailureReason on a VALIDATION_TIMED_OUT cert,
// but aws-sdk-go-v2/service/acm@v1.43.4 types/types.go:518-523 says
// FailureReason "exists only when the certificate status is FAILED".
func TestJanitor_TimeoutDoesNotSetFailureReason(t *testing.T) {
	t.Parallel()

	b := acm.NewInMemoryBackend("000000000000", "us-east-1")
	b.SetAutoValidateDelayForTest(time.Hour)

	cert, err := b.RequestCertificate(context.Background(), "stuck-fr.example.com", "", "DNS", "", "", "", "", nil)
	require.NoError(t, err)

	b.BackdateCertForTest("us-east-1", cert.ARN, time.Now().UTC().Add(-73*time.Hour), cert.NotAfter)

	b.SweepJanitorOnceForTest()

	got, err := b.DescribeCertificate(context.Background(), cert.ARN)
	require.NoError(t, err)
	require.Equal(t, "VALIDATION_TIMED_OUT", got.Status)
	assert.Empty(t, got.FailureReason)
}
