package acm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/acm"
)

func TestACMBackend_RequestCertificate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		domain           string
		validationMethod string
		wantErr          error
		wantDomain       string
		wantStatus       string
		wantType         string
		wantPendingFirst bool
	}{
		{
			name:       "success_no_validation",
			domain:     "example.com",
			wantDomain: "example.com",
			wantStatus: "ISSUED",
			wantType:   "AMAZON_ISSUED",
		},
		{
			name:             "dns_validation_pending",
			domain:           "dns.example.com",
			validationMethod: "DNS",
			wantDomain:       "dns.example.com",
			wantStatus:       "PENDING_VALIDATION",
			wantType:         "AMAZON_ISSUED",
			wantPendingFirst: true,
		},
		{
			name:             "email_validation_pending",
			domain:           "email.example.com",
			validationMethod: "EMAIL",
			wantDomain:       "email.example.com",
			wantStatus:       "PENDING_VALIDATION",
			wantType:         "AMAZON_ISSUED",
			wantPendingFirst: true,
		},
		{
			// STRENGTHENED (gopherstack-bzyl): previously pinned ErrInvalidParameter
			// (ValidationException), a code RequestCertificate's real deserializer
			// never declares (deserializers.go, acm@v1.43.4) -- only
			// InvalidParameterException. The old assertion was pinning the bug.
			name:    "empty_domain",
			domain:  "",
			wantErr: acm.ErrRequestCertInvalidParameter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			cert, err := b.RequestCertificate(
				context.Background(),
				tt.domain,
				"",
				tt.validationMethod,
				"",
				"",
				"",
				"",
				nil,
			)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Contains(t, cert.ARN, "arn:aws:acm:")
			assert.Equal(t, tt.wantDomain, cert.DomainName)
			assert.Equal(t, tt.wantStatus, cert.Status)
			assert.Equal(t, tt.wantType, cert.Type)
			assert.NotEmpty(t, cert.CertificateBody, "CertificateBody should be set")

			if tt.wantPendingFirst {
				// Wait for auto-validation
				require.Eventually(t, func() bool {
					c, descErr := b.DescribeCertificate(context.Background(), cert.ARN)

					return descErr == nil && c.Status == "ISSUED"
				}, 2*time.Second, 50*time.Millisecond, "certificate should transition to ISSUED")
			}
		})
	}
}

func TestACMBackend_RequestCertificate_Extended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantDVOLenMsg            string
		wantDVORecordType        string
		wantDVODomain            string
		wantDomain               string
		wantDVORecordNameSubstr  string
		name                     string
		wantDVOValidationStatus  string
		wantDVORecordValueSubstr string
		domain                   string
		wantDVOValidationMethod  string
		wantSANs                 []string
		sans                     []string
		wantDVOLen               int
		verifyDVOFields          bool
	}{
		{
			name:       "with_sans",
			domain:     "example.com",
			sans:       []string{"www.example.com", "api.example.com"},
			wantDomain: "example.com",
			// Real AWS ACM always includes the primary domain as the first SAN entry.
			wantSANs:      []string{"example.com", "www.example.com", "api.example.com"},
			wantDVOLen:    3,
			wantDVOLenMsg: "should have DVOs for primary + 2 SANs",
		},
		{
			name:                     "dns_validation_options",
			domain:                   "example.com",
			sans:                     nil,
			wantDomain:               "example.com",
			wantDVOLen:               1,
			verifyDVOFields:          true,
			wantDVODomain:            "example.com",
			wantDVOValidationStatus:  "PENDING_VALIDATION",
			wantDVOValidationMethod:  "DNS",
			wantDVORecordType:        "CNAME",
			wantDVORecordNameSubstr:  "example.com",
			wantDVORecordValueSubstr: "acm-validations.aws",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			cert, err := b.RequestCertificate(context.Background(), tt.domain, "", "DNS", "", "", "", "", tt.sans)
			require.NoError(t, err)

			assert.Equal(t, tt.wantDomain, cert.DomainName)

			if len(tt.wantSANs) > 0 {
				assert.Equal(t, tt.wantSANs, cert.SubjectAlternativeNames)
			}

			if tt.wantDVOLen > 0 {
				if tt.wantDVOLenMsg != "" {
					assert.Len(t, cert.DomainValidationOptions, tt.wantDVOLen, tt.wantDVOLenMsg)
				} else {
					assert.Len(t, cert.DomainValidationOptions, tt.wantDVOLen)
				}
			}

			if tt.verifyDVOFields {
				require.Len(t, cert.DomainValidationOptions, 1)
				dvo := cert.DomainValidationOptions[0]
				assert.Equal(t, tt.wantDVODomain, dvo.DomainName)
				assert.Equal(t, tt.wantDVOValidationStatus, dvo.ValidationStatus)
				assert.Equal(t, tt.wantDVOValidationMethod, dvo.ValidationMethod)
				require.NotNil(t, dvo.ResourceRecord)
				assert.Equal(t, tt.wantDVORecordType, dvo.ResourceRecord.Type)
				assert.Contains(t, dvo.ResourceRecord.Name, tt.wantDVORecordNameSubstr)
				assert.Contains(t, dvo.ResourceRecord.Value, tt.wantDVORecordValueSubstr)
			}
		})
	}
}

func TestACMBackend_DescribeCertificate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		name    string
		arn     string
	}{
		{
			name:    "not_found",
			arn:     "arn:aws:acm:us-east-1:000000000000:certificate/nonexistent",
			wantErr: acm.ErrCertNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.DescribeCertificate(context.Background(), tt.arn)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestACMBackend_DeleteCertificate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		setup   func(t *testing.T, b *acm.InMemoryBackend) string
		name    string
	}{
		{
			name: "success",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(context.Background(), "delete-me.com", "", "", "", "", "", "", nil)
				require.NoError(t, err)

				return cert.ARN
			},
		},
		{
			name:    "not_found",
			setup:   func(*testing.T, *acm.InMemoryBackend) string { return "nonexistent-arn" },
			wantErr: acm.ErrCertNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			arn := tt.setup(t, b)
			err := b.DeleteCertificate(context.Background(), arn)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestACMBackend_ImportCertificate(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := generateTestCert(t)

	tests := []struct {
		wantErr    error
		name       string
		certBody   string
		privateKey string
		certChain  string
		wantType   string
		wantStatus string
	}{
		{
			name:       "success",
			certBody:   certPEM,
			privateKey: keyPEM,
			wantType:   "IMPORTED",
			wantStatus: "ISSUED",
		},
		{
			name:       "with_chain",
			certBody:   certPEM,
			privateKey: keyPEM,
			certChain:  certPEM,
			wantType:   "IMPORTED",
			wantStatus: "ISSUED",
		},
		{
			name:       "missing_cert",
			certBody:   "",
			privateKey: keyPEM,
			wantErr:    acm.ErrInvalidParameter,
		},
		{
			name:       "missing_key",
			certBody:   certPEM,
			privateKey: "",
			wantErr:    acm.ErrInvalidParameter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			cert, err := b.ImportCertificate(context.Background(), tt.certBody, tt.privateKey, tt.certChain, "")

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Contains(t, cert.ARN, "arn:aws:acm:")
			assert.Equal(t, tt.wantType, cert.Type)
			assert.Equal(t, tt.wantStatus, cert.Status)
			assert.Equal(t, tt.certBody, cert.CertificateBody)
			assert.Equal(t, tt.privateKey, cert.PrivateKey)
			assert.Equal(t, tt.certChain, cert.CertificateChain)
		})
	}
}

func TestACMBackend_RenewCertificate(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := generateTestCert(t)

	tests := []struct {
		wantErr     error
		setup       func(t *testing.T, b *acm.InMemoryBackend) string
		name        string
		wantNewCert bool
	}{
		{
			name: "success_amazon_issued",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"renew.example.com",
					"",
					"",
					"",
					"",
					"",
					"",
					nil,
				)
				require.NoError(t, err)

				return cert.ARN
			},
			wantNewCert: true,
		},
		{
			name: "imported_not_eligible",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.ImportCertificate(context.Background(), certPEM, keyPEM, "", "")
				require.NoError(t, err)

				return cert.ARN
			},
			wantErr: acm.ErrNotEligible,
		},
		{
			name:    "not_found",
			setup:   func(*testing.T, *acm.InMemoryBackend) string { return "nonexistent-arn" },
			wantErr: acm.ErrCertNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			certARN := tt.setup(t, b)

			var originalBody string
			var originalNotAfter time.Time
			if tt.wantNewCert {
				orig, err := b.DescribeCertificate(context.Background(), certARN)
				require.NoError(t, err)
				originalBody = orig.CertificateBody
				originalNotAfter = orig.NotAfter
			}

			err := b.RenewCertificate(context.Background(), certARN)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)

			if tt.wantNewCert {
				renewed, descErr := b.DescribeCertificate(context.Background(), certARN)
				require.NoError(t, descErr)
				assert.NotEmpty(t, renewed.CertificateBody)
				assert.NotEqual(t, originalBody, renewed.CertificateBody, "cert body should be regenerated")
				assert.True(t, renewed.NotAfter.After(originalNotAfter) || renewed.NotAfter.Equal(originalNotAfter),
					"NotAfter should be at least as late as the original")
				assert.False(t, renewed.NotBefore.IsZero(), "NotBefore should be set")
				assert.False(t, renewed.NotAfter.IsZero(), "NotAfter should be set")
			}
		})
	}
}

func TestACMBackend_ExportCertificate(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := generateTestCert(t)

	tests := []struct {
		wantErr error
		setup   func(t *testing.T, b *acm.InMemoryBackend) string
		checkFn func(t *testing.T, cert *acm.Certificate)
		name    string
	}{
		{
			name: "success_imported",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.ImportCertificate(context.Background(), certPEM, keyPEM, "", "")
				require.NoError(t, err)

				return cert.ARN
			},
			checkFn: func(t *testing.T, cert *acm.Certificate) {
				t.Helper()
				assert.Equal(t, certPEM, cert.CertificateBody)
				assert.Equal(t, keyPEM, cert.PrivateKey)
			},
		},
		{
			// Confirmed against the live AWS API reference this pass
			// (API_ExportCertificate.html, API_CertificateOptions.html): an
			// AMAZON_ISSUED certificate without Options.Export=ENABLED is not
			// exportable, and the correct error is ValidationException, not
			// RequestInProgressException (that error's documented meaning is
			// specifically "not yet issued", which this ISSUED-but-ineligible
			// certificate is not). See validateCertExportable, certificates.go.
			name: "fails_amazon_issued_without_export_enabled",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"amazon.example.com",
					"",
					"",
					"",
					"",
					"",
					"",
					nil,
				)
				require.NoError(t, err)

				return cert.ARN
			},
			wantErr: acm.ErrInvalidParameter,
		},
		{
			// New capability this pass: an AMAZON_ISSUED certificate that opted
			// in via Options.Export=ENABLED is now genuinely exportable.
			name: "success_amazon_issued_with_export_enabled",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"amazon-exportable.example.com",
					"",
					"",
					"",
					"",
					"",
					"",
					nil,
				)
				require.NoError(t, err)
				require.NoError(t, b.SetExportPreference(context.Background(), cert.ARN, "ENABLED"))

				return cert.ARN
			},
			checkFn: func(t *testing.T, cert *acm.Certificate) {
				t.Helper()
				assert.Contains(t, cert.CertificateBody, "BEGIN CERTIFICATE")
				assert.NotEmpty(t, cert.PrivateKey)
			},
		},
		{
			name:    "not_found",
			setup:   func(*testing.T, *acm.InMemoryBackend) string { return "nonexistent-arn" },
			wantErr: acm.ErrCertNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			certARN := tt.setup(t, b)
			cert, err := b.ExportCertificate(context.Background(), certARN, nil)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			tt.checkFn(t, cert)
		})
	}
}

func TestACMBackend_GetCertificate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		setup   func(t *testing.T, b *acm.InMemoryBackend) string
		name    string
	}{
		{
			name: "success_amazon_issued",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(context.Background(), "get.example.com", "", "", "", "", "", "", nil)
				require.NoError(t, err)

				return cert.ARN
			},
		},
		{
			name:    "not_found",
			setup:   func(*testing.T, *acm.InMemoryBackend) string { return "nonexistent-arn" },
			wantErr: acm.ErrCertNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			certARN := tt.setup(t, b)
			certBody, _, err := b.GetCertificate(context.Background(), certARN)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, certBody)
			assert.Contains(t, certBody, "BEGIN CERTIFICATE")
		})
	}
}

// generateTestCert creates a test domain, generates a self-signed cert via the
// backend, and returns the certificate PEM and private key PEM.
func generateTestCert(t *testing.T) (string, string) {
	t.Helper()

	b := acm.NewInMemoryBackend("000000000000", "us-east-1")
	cert, err := b.RequestCertificate(context.Background(), "test.example.com", "", "", "", "", "", "", nil)
	require.NoError(t, err)

	// Retrieve stored PEM data via GetCertificate
	certBody, _, getCertErr := b.GetCertificate(context.Background(), cert.ARN)
	require.NoError(t, getCertErr)
	require.NotEmpty(t, certBody)

	// Use cert body from describe to get PEM and key
	described, descErr := b.DescribeCertificate(context.Background(), cert.ARN)
	require.NoError(t, descErr)

	return described.CertificateBody, described.PrivateKey
}

// TestACMBackend_CertificateBodyIsPEM verifies the generated cert body is valid PEM.
func TestACMBackend_CertificateBodyIsPEM(t *testing.T) {
	t.Parallel()

	b := acm.NewInMemoryBackend("000000000000", "us-east-1")
	cert, err := b.RequestCertificate(context.Background(), "pem.example.com", "", "", "", "", "", "", nil)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(cert.CertificateBody, "-----BEGIN CERTIFICATE-----"))
	assert.True(t, strings.HasPrefix(cert.PrivateKey, "-----BEGIN EC PRIVATE KEY-----"))
}

// TestACMBackend_ExportCertificate_Passphrase verifies passphrase encryption in ExportCertificate.
func TestACMBackend_ExportCertificate_Passphrase(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := generateTestCert(t)

	tests := []struct {
		name             string
		wantKeyHeader    string
		passphrase       []byte
		wantKeyEncrypted bool
	}{
		{
			name:             "no_passphrase_returns_plain",
			passphrase:       nil,
			wantKeyHeader:    "-----BEGIN EC PRIVATE KEY-----",
			wantKeyEncrypted: false,
		},
		{
			name:             "with_passphrase_returns_encrypted",
			passphrase:       []byte("s3cr3t"),
			wantKeyHeader:    "-----BEGIN ENCRYPTED PRIVATE KEY-----",
			wantKeyEncrypted: true,
		},
		{
			name:             "empty_passphrase_returns_plain",
			passphrase:       []byte{},
			wantKeyHeader:    "-----BEGIN EC PRIVATE KEY-----",
			wantKeyEncrypted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			imported, err := b.ImportCertificate(context.Background(), certPEM, keyPEM, "", "")
			require.NoError(t, err)

			exported, exportErr := b.ExportCertificate(context.Background(), imported.ARN, tt.passphrase)
			require.NoError(t, exportErr)

			assert.True(t, strings.HasPrefix(exported.PrivateKey, tt.wantKeyHeader),
				"PrivateKey should start with %q, got: %.50s", tt.wantKeyHeader, exported.PrivateKey)
			assert.NotEmpty(t, exported.CertificateBody)
			assert.NotEmpty(t, exported.CertificateChain)
		})
	}
}

// TestACMBackend_ImportCertificate_KeyUsageParsed verifies that imported certs have key usages parsed.
func TestACMBackend_ImportCertificate_KeyUsageParsed(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := generateTestCert(t)

	b := acm.NewInMemoryBackend("000000000000", "us-east-1")
	cert, err := b.ImportCertificate(context.Background(), certPEM, keyPEM, "", "")
	require.NoError(t, err)

	described, descErr := b.DescribeCertificate(context.Background(), cert.ARN)
	require.NoError(t, descErr)

	assert.NotEmpty(t, described.KeyUsage, "imported cert should have key usages parsed from X.509")
	assert.Contains(t, described.KeyUsage, "DIGITAL_SIGNATURE")
	assert.NotEmpty(t, described.ExtendedKeyUsage, "imported cert should have extended key usages parsed")
	assert.Contains(t, described.ExtendedKeyUsage, "TLS_WEB_SERVER_AUTHENTICATION")
}

// generateTestCertWithKeyAlgorithm requests a certificate with the given
// KeyAlgorithm and returns its PEM body/key, for feeding into ImportCertificate
// to test that the imported cert's real key type is reflected, not assumed.
func generateTestCertWithKeyAlgorithm(t *testing.T, keyAlgorithm string) (string, string) {
	t.Helper()

	b := acm.NewInMemoryBackend("000000000000", "us-east-1")
	cert, err := b.RequestCertificate(
		context.Background(), "keyalgo.example.com", "", "", "", keyAlgorithm, "", "", nil,
	)
	require.NoError(t, err)

	described, descErr := b.DescribeCertificate(context.Background(), cert.ARN)
	require.NoError(t, descErr)

	return described.CertificateBody, described.PrivateKey
}

// TestACMBackend_ImportCertificate_KeyAlgorithmDerived verifies that
// ImportCertificate reports the real KeyAlgorithm of the imported key
// (CertificateDetail.KeyAlgorithm: "The algorithm that was used to generate
// the public-private key pair") instead of always assuming EC_prime256v1,
// on both first import and re-import.
func TestACMBackend_ImportCertificate_KeyAlgorithmDerived(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		keyAlgorithm     string
		wantKeyAlgorithm string
	}{
		{name: "rsa_2048", keyAlgorithm: "RSA_2048", wantKeyAlgorithm: "RSA_2048"},
		{name: "rsa_4096", keyAlgorithm: "RSA_4096", wantKeyAlgorithm: "RSA_4096"},
		{name: "ec_secp384r1", keyAlgorithm: "EC_secp384r1", wantKeyAlgorithm: "EC_secp384r1"},
		{name: "ec_prime256v1", keyAlgorithm: "EC_prime256v1", wantKeyAlgorithm: "EC_prime256v1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			certPEM, keyPEM := generateTestCertWithKeyAlgorithm(t, tt.keyAlgorithm)

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			cert, err := b.ImportCertificate(context.Background(), certPEM, keyPEM, "", "")
			require.NoError(t, err)
			assert.Equal(t, tt.wantKeyAlgorithm, cert.KeyAlgorithm)

			described, descErr := b.DescribeCertificate(context.Background(), cert.ARN)
			require.NoError(t, descErr)
			assert.Equal(t, tt.wantKeyAlgorithm, described.KeyAlgorithm)
		})
	}
}

// TestACMBackend_ImportCertificate_ReImport_KeyAlgorithmUpdated verifies that
// re-importing a certificate (CertificateArn set) with a different key type
// updates the stored KeyAlgorithm rather than leaving the prior value stale.
func TestACMBackend_ImportCertificate_ReImport_KeyAlgorithmUpdated(t *testing.T) {
	t.Parallel()

	ecPEM, ecKeyPEM := generateTestCertWithKeyAlgorithm(t, "EC_prime256v1")
	rsaPEM, rsaKeyPEM := generateTestCertWithKeyAlgorithm(t, "RSA_2048")

	b := acm.NewInMemoryBackend("000000000000", "us-east-1")
	original, err := b.ImportCertificate(context.Background(), ecPEM, ecKeyPEM, "", "")
	require.NoError(t, err)
	require.Equal(t, "EC_prime256v1", original.KeyAlgorithm)

	updated, err := b.ImportCertificate(context.Background(), rsaPEM, rsaKeyPEM, "", original.ARN)
	require.NoError(t, err)
	assert.Equal(t, "RSA_2048", updated.KeyAlgorithm)

	described, descErr := b.DescribeCertificate(context.Background(), original.ARN)
	require.NoError(t, descErr)
	assert.Equal(t, "RSA_2048", described.KeyAlgorithm)
}

// TestACMBackend_ValidityAndEligibility verifies that NotBefore, NotAfter, and
// RenewalEligibility are populated correctly for all certificate creation paths.
func TestACMBackend_ValidityAndEligibility(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := generateTestCert(t)

	tests := []struct {
		setup                  func(t *testing.T, b *acm.InMemoryBackend) string
		name                   string
		wantRenewalEligibility string
	}{
		{
			name: "amazon_issued_eligible",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"validity.example.com",
					"",
					"",
					"",
					"",
					"",
					"",
					nil,
				)
				require.NoError(t, err)

				return cert.ARN
			},
			wantRenewalEligibility: "ELIGIBLE",
		},
		{
			name: "imported_ineligible",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.ImportCertificate(context.Background(), certPEM, keyPEM, "", "")
				require.NoError(t, err)

				return cert.ARN
			},
			wantRenewalEligibility: "INELIGIBLE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			certARN := tt.setup(t, b)

			cert, err := b.DescribeCertificate(context.Background(), certARN)
			require.NoError(t, err)

			assert.False(t, cert.NotBefore.IsZero(), "NotBefore should be set")
			assert.False(t, cert.NotAfter.IsZero(), "NotAfter should be set")
			assert.True(t, cert.NotAfter.After(cert.NotBefore), "NotAfter should be after NotBefore")
			assert.Equal(t, tt.wantRenewalEligibility, cert.RenewalEligibility)
		})
	}
}
