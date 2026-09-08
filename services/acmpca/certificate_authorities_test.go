package acmpca_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/acmpca"
)

const (
	testAccountID = "000000000000"
	testRegion    = "us-east-1"
)

func newTestBackend() *acmpca.InMemoryBackend {
	return acmpca.NewInMemoryBackend(testAccountID, testRegion)
}

func TestInMemoryBackend_CreateCertificateAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cfg        acmpca.CertificateAuthorityConfiguration
		name       string
		caType     string
		wantStatus string
		wantErr    bool
	}{
		{
			name:   "root CA defaults",
			caType: "ROOT",
			cfg: acmpca.CertificateAuthorityConfiguration{
				Subject:          acmpca.CertificateAuthoritySubject{CommonName: "Test Root CA"},
				KeyAlgorithm:     "EC_prime256v1",
				SigningAlgorithm: "SHA256WITHECDSA",
			},
			wantStatus: "ACTIVE",
		},
		{
			name:   "subordinate CA starts pending certificate",
			caType: "SUBORDINATE",
			cfg: acmpca.CertificateAuthorityConfiguration{
				Subject: acmpca.CertificateAuthoritySubject{CommonName: "Sub CA"},
			},
			wantStatus: "PENDING_CERTIFICATE",
		},
		{
			name:    "invalid type",
			caType:  "INVALID",
			wantErr: true,
		},
		{
			name:   "empty type defaults to ROOT",
			caType: "",
			cfg: acmpca.CertificateAuthorityConfiguration{
				Subject: acmpca.CertificateAuthoritySubject{CommonName: "Default Root"},
			},
			wantStatus: "ACTIVE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			ca, err := b.CreateCertificateAuthority(context.Background(), tt.caType, tt.cfg)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, ca.ARN)
			assert.Equal(t, tt.wantStatus, ca.Status)
		})
	}
}

func TestInMemoryBackend_DescribeCertificateAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		caARN   string
		wantErr bool
	}{
		{
			name:    "existing CA",
			caARN:   "",
			wantErr: false,
		},
		{
			name:    "non-existent CA",
			caARN:   "arn:aws:acm-pca:us-east-1:000000000000:certificate-authority/nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			var caARN string

			if tt.caARN == "" {
				ca, err := b.CreateCertificateAuthority(
					context.Background(),
					"ROOT",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "Test CA"},
					},
				)
				require.NoError(t, err)
				caARN = ca.ARN
			} else {
				caARN = tt.caARN
			}

			ca, err := b.DescribeCertificateAuthority(context.Background(), caARN)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, caARN, ca.ARN)
		})
	}
}

func TestInMemoryBackend_ListCertificateAuthorities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		createN   int
		wantCount int
	}{
		{
			name:      "empty list",
			createN:   0,
			wantCount: 0,
		},
		{
			name:      "two CAs",
			createN:   2,
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()

			for i := range tt.createN {
				_, err := b.CreateCertificateAuthority(
					context.Background(),
					"ROOT",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "CA"},
					},
				)
				require.NoError(t, err, "creating CA %d", i)
			}

			p, err := b.ListCertificateAuthorities(context.Background(), "", 0, "")
			require.NoError(t, err)
			assert.Len(t, p.Data, tt.wantCount)
		})
	}
}

func TestInMemoryBackend_DeleteCertificateAuthority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		caARN   string
		wantErr bool
	}{
		{
			name:    "existing CA after disable",
			caARN:   "",
			wantErr: false,
		},
		{
			name:    "non-existent CA",
			caARN:   "arn:aws:acm-pca:us-east-1:000000000000:certificate-authority/nonexistent",
			wantErr: true,
		},
		{
			name:    "active CA without disabling first",
			caARN:   "active",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			var caARN string

			switch tt.caARN {
			case "":
				ca, err := b.CreateCertificateAuthority(
					context.Background(),
					"ROOT",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "Test CA"},
					},
				)
				require.NoError(t, err)
				caARN = ca.ARN
				// Disable the CA first (AWS requirement before deletion).
				require.NoError(t, b.UpdateCertificateAuthority(context.Background(), caARN, "DISABLED"))
			case "active":
				ca, err := b.CreateCertificateAuthority(
					context.Background(),
					"ROOT",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "Active CA"},
					},
				)
				require.NoError(t, err)
				caARN = ca.ARN
				// Do NOT disable — deletion should fail.
			default:
				caARN = tt.caARN
			}

			err := b.DeleteCertificateAuthority(context.Background(), caARN, 0)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			ca, err := b.DescribeCertificateAuthority(context.Background(), caARN)
			require.NoError(t, err)
			assert.Equal(t, "DELETED", ca.Status)
		})
	}
}

// TestInMemoryBackend_DeleteCertificateAuthority_RestorableUntil verifies that
// deleting a CA sets RestorableUntil to now+PermanentDeletionTimeInDays (defaulting
// to 30 days when unset, matching real AWS ACM PCA behavior), and that restoring
// the CA clears it again.
func TestInMemoryBackend_DeleteCertificateAuthority_RestorableUntil(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		days     int32
		wantDays int
	}{
		{name: "default (unset) is 30 days", days: 0, wantDays: 30},
		{name: "explicit minimum", days: 7, wantDays: 7},
		{name: "explicit maximum", days: 30, wantDays: 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			ca, err := b.CreateCertificateAuthority(
				context.Background(),
				"ROOT",
				acmpca.CertificateAuthorityConfiguration{
					Subject: acmpca.CertificateAuthoritySubject{CommonName: "Restorable CA"},
				},
			)
			require.NoError(t, err)
			require.NoError(t, b.UpdateCertificateAuthority(context.Background(), ca.ARN, "DISABLED"))

			before := time.Now().UTC()
			require.NoError(t, b.DeleteCertificateAuthority(context.Background(), ca.ARN, tt.days))

			deleted, err := b.DescribeCertificateAuthority(context.Background(), ca.ARN)
			require.NoError(t, err)
			assert.Equal(t, "DELETED", deleted.Status)

			wantUntil := before.AddDate(0, 0, tt.wantDays)
			assert.WithinDuration(t, wantUntil, deleted.RestorableUntil, time.Minute)

			require.NoError(t, b.RestoreCertificateAuthority(context.Background(), ca.ARN))

			restored, err := b.DescribeCertificateAuthority(context.Background(), ca.ARN)
			require.NoError(t, err)
			assert.Equal(t, "DISABLED", restored.Status)
			assert.True(t, restored.RestorableUntil.IsZero(), "RestorableUntil must be cleared after restore")
		})
	}
}

func TestInMemoryBackend_GetCertificateAuthorityCsr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantErr bool
	}{
		{
			name:    "existing CA",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend()
			ca, err := b.CreateCertificateAuthority(
				context.Background(),
				"SUBORDINATE",
				acmpca.CertificateAuthorityConfiguration{
					Subject: acmpca.CertificateAuthoritySubject{CommonName: "Sub CA"},
				},
			)
			require.NoError(t, err)

			csr, err := b.GetCertificateAuthorityCsr(context.Background(), ca.ARN)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Contains(t, csr, "CERTIFICATE REQUEST")
		})
	}
}

// TestInMemoryBackend_CertificateAuthorityValidation covers CA-lifecycle
// validation and state-machine edge cases: restoring without an ARN,
// deleting from every deletable state, an out-of-range deletion window, and
// an invalid target status on update.
func TestInMemoryBackend_CertificateAuthorityValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, b *acmpca.InMemoryBackend)
		name string
	}{
		{
			name: "restore requires ca arn",
			run: func(t *testing.T, b *acmpca.InMemoryBackend) {
				t.Helper()

				err := b.RestoreCertificateAuthority(context.Background(), "")
				require.ErrorIs(t, err, acmpca.ErrInvalidArn)
			},
		},
		{
			name: "delete from PENDING_CERTIFICATE state succeeds",
			run: func(t *testing.T, b *acmpca.InMemoryBackend) {
				t.Helper()

				// SUBORDINATE CAs start in PENDING_CERTIFICATE state (no auto-sign).
				ca, err := b.CreateCertificateAuthority(
					context.Background(),
					"SUBORDINATE",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "Pending CA"},
					},
				)
				require.NoError(t, err)
				assert.Equal(t, "PENDING_CERTIFICATE", ca.Status)

				err = b.DeleteCertificateAuthority(context.Background(), ca.ARN, 0)
				require.NoError(t, err)

				got, err := b.DescribeCertificateAuthority(context.Background(), ca.ARN)
				require.NoError(t, err)
				assert.Equal(t, "DELETED", got.Status)
			},
		},
		{
			name: "delete with permanentDeletionDays=5 returns error",
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

				err = b.DeleteCertificateAuthority(context.Background(), ca.ARN, 5)
				require.ErrorIs(t, err, acmpca.ErrInvalidArgs)
			},
		},
		{
			name: "updateCA on a PENDING_CERTIFICATE CA returns InvalidStateException",
			run: func(t *testing.T, b *acmpca.InMemoryBackend) {
				t.Helper()

				// SUBORDINATE CAs start in PENDING_CERTIFICATE state (no auto-sign);
				// per api_op_UpdateCertificateAuthority.go: "Your private CA must be
				// in the ACTIVE or DISABLED state before you can update it."
				ca, err := b.CreateCertificateAuthority(
					context.Background(),
					"SUBORDINATE",
					acmpca.CertificateAuthorityConfiguration{
						Subject: acmpca.CertificateAuthoritySubject{CommonName: "Pending CA"},
					},
				)
				require.NoError(t, err)
				require.Equal(t, "PENDING_CERTIFICATE", ca.Status)

				err = b.UpdateCertificateAuthority(context.Background(), ca.ARN, "ACTIVE")
				require.ErrorIs(t, err, acmpca.ErrInvalidState)

				got, err := b.DescribeCertificateAuthority(context.Background(), ca.ARN)
				require.NoError(t, err)
				assert.Equal(t, "PENDING_CERTIFICATE", got.Status)
			},
		},
		{
			name: "updateCA on a DELETED-but-restorable CA returns InvalidStateException",
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
				require.NoError(t, b.UpdateCertificateAuthority(context.Background(), ca.ARN, "DISABLED"))
				require.NoError(t, b.DeleteCertificateAuthority(context.Background(), ca.ARN, 7))

				// Attempting to update straight back to ACTIVE must fail: only
				// RestoreCertificateAuthority may bring a DELETED CA back (to DISABLED).
				err = b.UpdateCertificateAuthority(context.Background(), ca.ARN, "ACTIVE")
				require.ErrorIs(t, err, acmpca.ErrInvalidState)

				got, err := b.DescribeCertificateAuthority(context.Background(), ca.ARN)
				require.NoError(t, err)
				assert.Equal(t, "DELETED", got.Status)
			},
		},
		{
			name: "updateCA with invalid status returns error",
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

				err = b.UpdateCertificateAuthority(context.Background(), ca.ARN, "INVALID_STATUS")
				require.ErrorIs(t, err, acmpca.ErrInvalidArgs)
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
