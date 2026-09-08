package acm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/acm"
)

// TestACMHandler_DNSValidationWorkflow verifies the full PENDING_VALIDATION → ISSUED flow.
func TestACMHandler_DNSValidationWorkflow(t *testing.T) {
	t.Parallel()

	h := newACMHandler()

	// Request with DNS validation
	reqRec := postACMJSON(t, h, "RequestCertificate",
		`{"DomainName":"workflow.example.com","ValidationMethod":"DNS"}`)
	require.Equal(t, http.StatusOK, reqRec.Code)

	var reqOut struct {
		CertificateArn string `json:"CertificateArn"`
	}
	require.NoError(t, json.Unmarshal(reqRec.Body.Bytes(), &reqOut))
	require.NotEmpty(t, reqOut.CertificateArn)

	// Describe should show PENDING_VALIDATION with CNAME records
	descBody, _ := json.Marshal(map[string]string{"CertificateArn": reqOut.CertificateArn})
	descRec := postACMJSON(t, h, "DescribeCertificate", string(descBody))
	require.Equal(t, http.StatusOK, descRec.Code)

	var descOut struct {
		Certificate struct {
			Status                  string `json:"Status"`
			DomainValidationOptions []struct {
				ResourceRecord *struct {
					Type string `json:"Type"`
				} `json:"ResourceRecord"`
				ValidationStatus string `json:"ValidationStatus"`
			} `json:"DomainValidationOptions"`
		} `json:"Certificate"`
	}
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))
	// Initial describe may already show ISSUED (auto-validate is quick), so accept either.
	assert.Contains(t, []string{"PENDING_VALIDATION", "ISSUED"}, descOut.Certificate.Status)
	require.NotEmpty(t, descOut.Certificate.DomainValidationOptions)
	assert.NotNil(t, descOut.Certificate.DomainValidationOptions[0].ResourceRecord)
	assert.Equal(t, "CNAME", descOut.Certificate.DomainValidationOptions[0].ResourceRecord.Type)

	// Wait for auto-transition to ISSUED
	require.Eventually(t, func() bool {
		rec := postACMJSON(t, h, "DescribeCertificate", string(descBody))
		var out struct {
			Certificate struct {
				Status string `json:"Status"`
			} `json:"Certificate"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)

		return out.Certificate.Status == "ISSUED"
	}, 2*time.Second, 50*time.Millisecond, "cert should transition to ISSUED")
}

func TestACMHandler_ResendValidationEmail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *acm.Handler) string
		buildBody    func(certARN string) string
		name         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success_pending_email",
			setup: func(t *testing.T, h *acm.Handler) string {
				t.Helper()
				rec := postACMJSON(t, h, "RequestCertificate",
					`{"DomainName":"email-resend.example.com","ValidationMethod":"EMAIL"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				var out struct {
					CertificateArn string `json:"CertificateArn"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

				return out.CertificateArn
			},
			buildBody: func(certARN string) string {
				b, _ := json.Marshal(map[string]string{
					"CertificateArn":   certARN,
					"Domain":           "email-resend.example.com",
					"ValidationDomain": "email-resend.example.com",
				})

				return string(b)
			},
			wantCode: http.StatusOK,
		},
		{
			name: "cert_not_found",
			setup: func(_ *testing.T, _ *acm.Handler) string {
				return "arn:aws:acm:us-east-1:000000000000:certificate/none"
			},
			buildBody: func(certARN string) string {
				b, _ := json.Marshal(map[string]string{
					"CertificateArn":   certARN,
					"Domain":           "example.com",
					"ValidationDomain": "example.com",
				})

				return string(b)
			},
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ResourceNotFoundException"},
		},
		{
			name: "cert_already_issued",
			setup: func(t *testing.T, h *acm.Handler) string {
				t.Helper()
				rec := postACMJSON(t, h, "RequestCertificate",
					`{"DomainName":"issued.example.com"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				var out struct {
					CertificateArn string `json:"CertificateArn"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

				return out.CertificateArn
			},
			buildBody: func(certARN string) string {
				b, _ := json.Marshal(map[string]string{
					"CertificateArn":   certARN,
					"Domain":           "issued.example.com",
					"ValidationDomain": "issued.example.com",
				})

				return string(b)
			},
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ValidationException"},
		},
		{
			name: "dns_cert_rejects_resend",
			setup: func(t *testing.T, h *acm.Handler) string {
				t.Helper()
				rec := postACMJSON(t, h, "RequestCertificate",
					`{"DomainName":"dns.example.com","ValidationMethod":"DNS"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				var out struct {
					CertificateArn string `json:"CertificateArn"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

				return out.CertificateArn
			},
			buildBody: func(certARN string) string {
				b, _ := json.Marshal(map[string]string{
					"CertificateArn":   certARN,
					"Domain":           "dns.example.com",
					"ValidationDomain": "dns.example.com",
				})

				return string(b)
			},
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ValidationException"},
		},
		{
			name:  "missing_domain",
			setup: func(_ *testing.T, _ *acm.Handler) string { return "" },
			buildBody: func(_ string) string {
				return `{"CertificateArn":"arn:aws:acm:us-east-1:1:certificate/0","ValidationDomain":"x.com"}`
			},
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ValidationException"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			certARN := ""
			if tt.setup != nil {
				certARN = tt.setup(t, h)
			}
			body := tt.buildBody(certARN)
			rec := postACMJSON(t, h, "ResendValidationEmail", body)
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestACMHandler_RevokeCertificate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *acm.Handler) string
		buildBody    func(certARN string) string
		name         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success_unspecified",
			setup: func(t *testing.T, h *acm.Handler) string {
				t.Helper()
				rec := postACMJSON(t, h, "RequestCertificate", `{"DomainName":"revoke.example.com"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				var out struct {
					CertificateArn string `json:"CertificateArn"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

				return out.CertificateArn
			},
			buildBody: func(certARN string) string {
				b, _ := json.Marshal(map[string]string{
					"CertificateArn":   certARN,
					"RevocationReason": "UNSPECIFIED",
				})

				return string(b)
			},
			wantCode: http.StatusOK,
		},
		{
			name: "success_key_compromise",
			setup: func(t *testing.T, h *acm.Handler) string {
				t.Helper()
				rec := postACMJSON(t, h, "RequestCertificate", `{"DomainName":"revoke2.example.com"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				var out struct {
					CertificateArn string `json:"CertificateArn"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

				return out.CertificateArn
			},
			buildBody: func(certARN string) string {
				b, _ := json.Marshal(map[string]string{
					"CertificateArn":   certARN,
					"RevocationReason": "KEY_COMPROMISE",
				})

				return string(b)
			},
			wantCode: http.StatusOK,
		},
		{
			name:  "cert_not_found",
			setup: func(_ *testing.T, _ *acm.Handler) string { return "" },
			buildBody: func(_ string) string {
				return `{"CertificateArn":"arn:aws:acm:us-east-1:000000000000:certificate/none","RevocationReason":"UNSPECIFIED"}`
			},
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ResourceNotFoundException"},
		},
		{
			name:  "invalid_reason",
			setup: func(_ *testing.T, _ *acm.Handler) string { return "" },
			buildBody: func(_ string) string {
				return `{"CertificateArn":"arn:aws:acm:us-east-1:1:certificate/0","RevocationReason":"BOGUS_REASON"}`
			},
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ValidationException"},
		},
		{
			name: "already_revoked",
			setup: func(t *testing.T, h *acm.Handler) string {
				t.Helper()
				rec := postACMJSON(t, h, "RequestCertificate", `{"DomainName":"revoke3.example.com"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				var out struct {
					CertificateArn string `json:"CertificateArn"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				// Revoke once
				b, _ := json.Marshal(map[string]string{
					"CertificateArn":   out.CertificateArn,
					"RevocationReason": "UNSPECIFIED",
				})
				postACMJSON(t, h, "RevokeCertificate", string(b))

				return out.CertificateArn
			},
			buildBody: func(certARN string) string {
				b, _ := json.Marshal(map[string]string{
					"CertificateArn":   certARN,
					"RevocationReason": "UNSPECIFIED",
				})

				return string(b)
			},
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidStateException"},
		},
		{
			name:  "missing_revocation_reason",
			setup: func(_ *testing.T, _ *acm.Handler) string { return "" },
			buildBody: func(_ string) string {
				return `{"CertificateArn":"arn:aws:acm:us-east-1:1:certificate/0"}`
			},
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ValidationException"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			certARN := ""
			if tt.setup != nil {
				certARN = tt.setup(t, h)
			}
			body := tt.buildBody(certARN)
			rec := postACMJSON(t, h, "RevokeCertificate", body)
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestACMHandler_RevokeCertificate_DescribeShowsRevoked(t *testing.T) {
	t.Parallel()

	h := newACMHandler()

	// Create a cert
	reqRec := postACMJSON(t, h, "RequestCertificate", `{"DomainName":"revoke-describe.example.com"}`)
	require.Equal(t, http.StatusOK, reqRec.Code)

	var reqOut struct {
		CertificateArn string `json:"CertificateArn"`
	}
	require.NoError(t, json.Unmarshal(reqRec.Body.Bytes(), &reqOut))

	// Revoke it
	revokeBody, _ := json.Marshal(map[string]string{
		"CertificateArn":   reqOut.CertificateArn,
		"RevocationReason": "KEY_COMPROMISE",
	})
	revokeRec := postACMJSON(t, h, "RevokeCertificate", string(revokeBody))
	require.Equal(t, http.StatusOK, revokeRec.Code)

	// Describe should show REVOKED with RevocationReason
	descBody, _ := json.Marshal(map[string]string{"CertificateArn": reqOut.CertificateArn})
	descRec := postACMJSON(t, h, "DescribeCertificate", string(descBody))
	require.Equal(t, http.StatusOK, descRec.Code)

	var descOut struct {
		Certificate struct {
			RevokedAt        *int64 `json:"RevokedAt"`
			Status           string `json:"Status"`
			RevocationReason string `json:"RevocationReason"`
		} `json:"Certificate"`
	}
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))
	assert.Equal(t, "REVOKED", descOut.Certificate.Status)
	assert.Equal(t, "KEY_COMPROMISE", descOut.Certificate.RevocationReason)
	assert.NotNil(t, descOut.Certificate.RevokedAt)
}

func TestACMHandler_UpdateCertificateOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *acm.Handler) string
		buildBody    func(certARN string) string
		name         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "enable_transparency_logging",
			setup: func(t *testing.T, h *acm.Handler) string {
				t.Helper()
				rec := postACMJSON(t, h, "RequestCertificate", `{"DomainName":"opts.example.com"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				var out struct {
					CertificateArn string `json:"CertificateArn"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

				return out.CertificateArn
			},
			buildBody: func(certARN string) string {
				b, _ := json.Marshal(map[string]any{
					"CertificateArn": certARN,
					"Options": map[string]string{
						"CertificateTransparencyLoggingPreference": "ENABLED",
					},
				})

				return string(b)
			},
			wantCode: http.StatusOK,
		},
		{
			name: "disable_transparency_logging",
			setup: func(t *testing.T, h *acm.Handler) string {
				t.Helper()
				rec := postACMJSON(t, h, "RequestCertificate", `{"DomainName":"opts2.example.com"}`)
				require.Equal(t, http.StatusOK, rec.Code)
				var out struct {
					CertificateArn string `json:"CertificateArn"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

				return out.CertificateArn
			},
			buildBody: func(certARN string) string {
				b, _ := json.Marshal(map[string]any{
					"CertificateArn": certARN,
					"Options": map[string]string{
						"CertificateTransparencyLoggingPreference": "DISABLED",
					},
				})

				return string(b)
			},
			wantCode: http.StatusOK,
		},
		{
			name:  "cert_not_found",
			setup: func(_ *testing.T, _ *acm.Handler) string { return "" },
			buildBody: func(_ string) string {
				const certARN = "arn:aws:acm:us-east-1:000000000000:certificate/none"
				b, _ := json.Marshal(map[string]any{
					"CertificateArn": certARN,
					"Options": map[string]string{
						"CertificateTransparencyLoggingPreference": "ENABLED",
					},
				})

				return string(b)
			},
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ResourceNotFoundException"},
		},
		{
			name:  "invalid_preference",
			setup: func(_ *testing.T, _ *acm.Handler) string { return "" },
			buildBody: func(_ string) string {
				return `{"CertificateArn":"arn:aws:acm:us-east-1:1:certificate/0",` +
					`"Options":{"CertificateTransparencyLoggingPreference":"BOGUS"}}`
			},
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ValidationException"},
		},
		{
			name:  "missing_preference",
			setup: func(_ *testing.T, _ *acm.Handler) string { return "" },
			buildBody: func(_ string) string {
				return `{"CertificateArn":"arn:aws:acm:us-east-1:1:certificate/0","Options":{}}`
			},
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ValidationException"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			certARN := ""
			if tt.setup != nil {
				certARN = tt.setup(t, h)
			}
			body := tt.buildBody(certARN)
			rec := postACMJSON(t, h, "UpdateCertificateOptions", body)
			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestACMHandler_UpdateCertificateOptions_DescribeShowsOptions(t *testing.T) {
	t.Parallel()

	h := newACMHandler()

	// Create a cert
	reqRec := postACMJSON(t, h, "RequestCertificate", `{"DomainName":"opts-describe.example.com"}`)
	require.Equal(t, http.StatusOK, reqRec.Code)

	var reqOut struct {
		CertificateArn string `json:"CertificateArn"`
	}
	require.NoError(t, json.Unmarshal(reqRec.Body.Bytes(), &reqOut))

	// Update options
	optsBody, _ := json.Marshal(map[string]any{
		"CertificateArn": reqOut.CertificateArn,
		"Options": map[string]string{
			"CertificateTransparencyLoggingPreference": "DISABLED",
		},
	})
	optsRec := postACMJSON(t, h, "UpdateCertificateOptions", string(optsBody))
	require.Equal(t, http.StatusOK, optsRec.Code)

	// Describe should reflect options
	descBody, _ := json.Marshal(map[string]string{"CertificateArn": reqOut.CertificateArn})
	descRec := postACMJSON(t, h, "DescribeCertificate", string(descBody))
	require.Equal(t, http.StatusOK, descRec.Code)

	var descOut struct {
		Certificate struct {
			Options *struct {
				CertificateTransparencyLoggingPreference string `json:"CertificateTransparencyLoggingPreference"`
			} `json:"Options"`
		} `json:"Certificate"`
	}
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))
	require.NotNil(t, descOut.Certificate.Options)
	assert.Equal(t, "DISABLED", descOut.Certificate.Options.CertificateTransparencyLoggingPreference)
}

// TestACMHandler_RevokeCertificate_PendingValidationRejected verifies that PENDING certs cannot be revoked.
//
// STRENGTHENED (gopherstack-bzyl): previously asserted InvalidStateException, a code
// RevokeCertificate's real deserializer never declares (deserializers.go, acm@v1.43.4:
// AccessDeniedException/ConflictException/InvalidArnException/ResourceInUseException/
// ResourceNotFoundException/ThrottlingException/ValidationException only). ConflictException's
// own doc text ("trying to update a resource ... that is already being created or updated.
// Wait for the previous operation to finish and try again") is a direct match for a cert
// still mid-validation, and it IS declared -- the old assertion was pinning an undeclared code.
func TestACMHandler_RevokeCertificate_PendingValidationRejected(t *testing.T) {
	t.Parallel()

	h := newACMHandler()

	// Create cert with DNS validation → starts PENDING
	reqRec := postACMJSON(t, h, "RequestCertificate",
		`{"DomainName":"pending-revoke.example.com","ValidationMethod":"DNS"}`)
	require.Equal(t, http.StatusOK, reqRec.Code)

	var reqOut struct {
		CertificateArn string `json:"CertificateArn"`
	}
	require.NoError(t, json.Unmarshal(reqRec.Body.Bytes(), &reqOut))

	// Poll until status is PENDING_VALIDATION (autoValidate may fire quickly)
	// We need to act before auto-validation fires; if it already fired, skip test.
	b := acm.NewInMemoryBackend("000000000000", "us-east-1")
	_ = b // just for reference to domain

	body, _ := json.Marshal(map[string]string{"CertificateArn": reqOut.CertificateArn})
	descRec := postACMJSON(t, h, "DescribeCertificate", string(body))
	require.Equal(t, http.StatusOK, descRec.Code)

	var descOut struct {
		Certificate struct {
			Status string `json:"Status"`
		} `json:"Certificate"`
	}
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))

	if descOut.Certificate.Status != "PENDING_VALIDATION" {
		t.Skip("cert auto-validated before test could run")
	}

	// Try to revoke PENDING cert
	revokeBody, _ := json.Marshal(map[string]string{
		"CertificateArn":   reqOut.CertificateArn,
		"RevocationReason": "UNSPECIFIED",
	})
	revokeRec := postACMJSON(t, h, "RevokeCertificate", string(revokeBody))
	assert.Equal(t, http.StatusBadRequest, revokeRec.Code)
	assert.Contains(t, revokeRec.Body.String(), "ConflictException")
}

// TestACMHandler_StatusLifecycle_DescribeReflectsNewStatus verifies that lifecycle status
// transitions are visible via DescribeCertificate through the full HTTP stack.
func TestACMHandler_StatusLifecycle_DescribeReflectsNewStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupCert  func(t *testing.T, b *acm.InMemoryBackend) string
		transition func(t *testing.T, b *acm.InMemoryBackend, certARN string)
		name       string
		wantStatus string
	}{
		{
			name: "issued_to_expired",
			setupCert: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"lifecycle-expire.example.com",
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
			transition: func(t *testing.T, b *acm.InMemoryBackend, certARN string) {
				t.Helper()
				require.NoError(t, b.ExpireCertificate(context.Background(), certARN))
			},
			wantStatus: "EXPIRED",
		},
		{
			name: "issued_to_inactive",
			setupCert: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"lifecycle-inactive.example.com",
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
			transition: func(t *testing.T, b *acm.InMemoryBackend, certARN string) {
				t.Helper()
				require.NoError(t, b.InactivateCertificate(context.Background(), certARN))
			},
			wantStatus: "INACTIVE",
		},
		{
			name: "pending_to_validation_timed_out",
			setupCert: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"lifecycle-timeout.example.com",
					"",
					"DNS",
					"",
					"",
					"",
					"",
					nil,
				)
				require.NoError(t, err)
				require.Equal(t, "PENDING_VALIDATION", cert.Status)

				return cert.ARN
			},
			transition: func(t *testing.T, b *acm.InMemoryBackend, certARN string) {
				t.Helper()
				require.NoError(t, b.TimeoutPendingValidation(context.Background(), certARN))
			},
			wantStatus: "VALIDATION_TIMED_OUT",
		},
		{
			name: "pending_to_failed",
			setupCert: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"lifecycle-fail.example.com",
					"",
					"EMAIL",
					"",
					"",
					"",
					"",
					nil,
				)
				require.NoError(t, err)
				require.Equal(t, "PENDING_VALIDATION", cert.Status)

				return cert.ARN
			},
			transition: func(t *testing.T, b *acm.InMemoryBackend, certARN string) {
				t.Helper()
				require.NoError(t, b.FailCertificate(context.Background(), certARN, "NO_AVAILABLE_CONTACTS"))
			},
			wantStatus: "FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			h := acm.NewHandler(b)

			certARN := tt.setupCert(t, b)
			tt.transition(t, b, certARN)

			descBody, _ := json.Marshal(map[string]string{"CertificateArn": certARN})
			descRec := postACMJSON(t, h, "DescribeCertificate", string(descBody))
			require.Equal(t, http.StatusOK, descRec.Code)

			var descOut struct {
				Certificate struct {
					Status        string `json:"Status"`
					FailureReason string `json:"FailureReason,omitempty"`
				} `json:"Certificate"`
			}
			require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descOut))
			assert.Equal(t, tt.wantStatus, descOut.Certificate.Status)
		})
	}
}
