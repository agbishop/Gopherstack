package acmpca_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/acmpca"
)

func TestInMemoryBackend_IssueCertificate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		validityDays int
		wantErr      bool
	}{
		{
			name:         "issue cert with default validity",
			validityDays: 0,
			wantErr:      false,
		},
		{
			name:         "issue cert with explicit validity",
			validityDays: 90,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			ca, err := b.CreateCertificateAuthority(
				context.Background(),
				"ROOT",
				acmpca.CertificateAuthorityConfiguration{
					Subject: acmpca.CertificateAuthoritySubject{CommonName: "Test Root CA"},
				},
			)
			require.NoError(t, err)

			// Get the CA's CSR as the cert to issue (for simplicity we reuse the self-signed CA cert's pub key)
			subCA, err := b.CreateCertificateAuthority(
				context.Background(),
				"SUBORDINATE",
				acmpca.CertificateAuthorityConfiguration{
					Subject: acmpca.CertificateAuthoritySubject{CommonName: "Test Sub CA"},
				},
			)
			require.NoError(t, err)

			csr, err := b.GetCertificateAuthorityCsr(context.Background(), subCA.ARN)
			require.NoError(t, err)

			cert, err := b.IssueCertificate(context.Background(), ca.ARN, csr, tt.validityDays)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, cert.ARN)
			assert.NotEmpty(t, cert.Serial)
			assert.NotEmpty(t, cert.CertBody)
		})
	}
}

// TestInMemoryBackend_CertificateValidation covers certificate-issuance and
// revocation validation edge cases.
func TestInMemoryBackend_CertificateValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, b *acmpca.InMemoryBackend)
		name string
	}{
		{
			name: "revoke sets RevokedAt and RevocationReason",
			run: func(t *testing.T, b *acmpca.InMemoryBackend) {
				t.Helper()

				ca, err := b.CreateCertificateAuthority(
					context.Background(),
					"ROOT",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "Revoke CA"},
					},
				)
				require.NoError(t, err)

				subCA, err := b.CreateCertificateAuthority(
					context.Background(),
					"SUBORDINATE",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "Sub CA"},
					},
				)
				require.NoError(t, err)

				csr, err := b.GetCertificateAuthorityCsr(context.Background(), subCA.ARN)
				require.NoError(t, err)

				cert, err := b.IssueCertificate(context.Background(), ca.ARN, csr, 365)
				require.NoError(t, err)

				err = b.RevokeCertificate(context.Background(), ca.ARN, cert.Serial, "KEY_COMPROMISE")
				require.NoError(t, err)

				got, err := b.GetCertificate(context.Background(), ca.ARN, cert.ARN)
				require.NoError(t, err)
				assert.Equal(t, "REVOKED", got.Status)
				assert.NotNil(t, got.RevokedAt)
				assert.Equal(t, "KEY_COMPROMISE", got.RevocationReason)
			},
		},
		{
			name: "revoking an already-revoked certificate returns RequestAlreadyProcessedException",
			run: func(t *testing.T, b *acmpca.InMemoryBackend) {
				t.Helper()

				ca, err := b.CreateCertificateAuthority(
					context.Background(),
					"ROOT",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "Double Revoke CA"},
					},
				)
				require.NoError(t, err)

				subCA, err := b.CreateCertificateAuthority(
					context.Background(),
					"SUBORDINATE",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "Sub CA"},
					},
				)
				require.NoError(t, err)

				csr, err := b.GetCertificateAuthorityCsr(context.Background(), subCA.ARN)
				require.NoError(t, err)

				cert, err := b.IssueCertificate(context.Background(), ca.ARN, csr, 365)
				require.NoError(t, err)

				err = b.RevokeCertificate(context.Background(), ca.ARN, cert.Serial, "KEY_COMPROMISE")
				require.NoError(t, err)

				err = b.RevokeCertificate(context.Background(), ca.ARN, cert.Serial, "SUPERSEDED")
				require.ErrorIs(t, err, acmpca.ErrRequestAlreadyProcessed)

				got, err := b.GetCertificate(context.Background(), ca.ARN, cert.ARN)
				require.NoError(t, err)
				assert.Equal(t, "KEY_COMPROMISE", got.RevocationReason)
			},
		},
		{
			name: "revoke with invalid reason returns error",
			run: func(t *testing.T, b *acmpca.InMemoryBackend) {
				t.Helper()

				ca, err := b.CreateCertificateAuthority(
					context.Background(),
					"ROOT",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "Revoke CA"},
					},
				)
				require.NoError(t, err)

				err = b.RevokeCertificate(context.Background(), ca.ARN, "doesNotMatter", "INVALID_REASON")
				require.ErrorIs(t, err, acmpca.ErrInvalidRequest)
			},
		},
		{
			name: "issueCA with empty CSR returns error",
			run: func(t *testing.T, b *acmpca.InMemoryBackend) {
				t.Helper()

				ca, err := b.CreateCertificateAuthority(
					context.Background(),
					"ROOT",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "CA"},
					},
				)
				require.NoError(t, err)

				_, err = b.IssueCertificate(context.Background(), ca.ARN, "", 365)
				require.ErrorIs(t, err, acmpca.ErrMalformedCSR)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.run(t, newTestBackend())
		})
	}
}
