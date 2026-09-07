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

// TestACMHandler_ExportCertificate_AmazonIssued covers ACM's
// exportable-public-certificates gating (confirmed this pass against the
// live AWS API reference: API_ExportCertificate.html's documented Errors
// section and API_CertificateOptions.html's Export field doc -- see
// validateCertExportable in certificates.go): a still-pending AMAZON_ISSUED
// certificate keeps the pre-existing RequestInProgressException; an
// issued-but-not-opted-in one now correctly returns ValidationException
// (previously, incorrectly, also RequestInProgressException, which per the
// op's own doc specifically means "not yet issued" -- a real client would
// have been told to wait and retry for a condition that could never change);
// an issued AND opted-in (Options.Export=ENABLED) one now genuinely succeeds,
// a capability that did not exist before this pass.
func TestACMHandler_ExportCertificate_AmazonIssued(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(t *testing.T, h *acm.Handler) string
		name        string
		wantErrType string // empty means the export must succeed
	}{
		{
			name: "still_pending_returns_request_in_progress",
			setup: func(t *testing.T, h *acm.Handler) string {
				t.Helper()
				// ValidationMethod must be DNS/EMAIL to actually land in
				// PENDING_VALIDATION -- omitting it hits buildInitialDVOList's
				// default case, which issues the certificate synchronously
				// (certificates.go), so an omitted ValidationMethod would make
				// this case indistinguishable from the "already issued" one below.
				reqRec := postACMJSON(t, h, "RequestCertificate",
					`{"DomainName":"export-pending.example.com","ValidationMethod":"DNS"}`)
				require.Equal(t, http.StatusOK, reqRec.Code)

				var reqOut struct {
					CertificateArn string `json:"CertificateArn"`
				}
				require.NoError(t, json.Unmarshal(reqRec.Body.Bytes(), &reqOut))

				return reqOut.CertificateArn
			},
			wantErrType: "RequestInProgressException",
		},
		{
			name: "issued_without_export_enabled_returns_validation_exception",
			setup: func(t *testing.T, h *acm.Handler) string {
				t.Helper()

				return requestAndAwaitIssued(t, h, "export-noteligible.example.com", "")
			},
			wantErrType: "ValidationException",
		},
		{
			name: "issued_with_export_enabled_succeeds",
			setup: func(t *testing.T, h *acm.Handler) string {
				t.Helper()

				return requestAndAwaitIssued(t, h, "export-eligible.example.com", "ENABLED")
			},
			wantErrType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			certARN := tt.setup(t, h)

			body, _ := json.Marshal(map[string]string{
				"CertificateArn": certARN,
				"Passphrase":     "dGVzdA==",
			})
			rec := postACMJSON(t, h, "ExportCertificate", string(body))

			if tt.wantErrType == "" {
				require.Equal(t, http.StatusOK, rec.Code)

				var out struct {
					Certificate      string `json:"Certificate"`
					CertificateChain string `json:"CertificateChain"`
					PrivateKey       string `json:"PrivateKey"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Contains(t, out.Certificate, "BEGIN CERTIFICATE")
				assert.NotEmpty(t, out.PrivateKey)

				// Exported must now flip to true and be visible on ListCertificates
				// -- no longer gated to PRIVATE certificates only (see
				// buildCertificateSummary's doc comment, handler_certificates.go).
				listRec := postACMJSON(t, h, "ListCertificates", `{}`)
				require.Equal(t, http.StatusOK, listRec.Code)

				var listOut struct {
					CertificateSummaryList []struct {
						CertificateArn string `json:"CertificateArn"`
						Exported       bool   `json:"Exported"`
					} `json:"CertificateSummaryList"`
				}
				require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listOut))

				var found bool
				for _, s := range listOut.CertificateSummaryList {
					if s.CertificateArn == certARN {
						found = true

						assert.True(t, s.Exported)
					}
				}
				assert.True(t, found, "exported certificate must appear in ListCertificates")

				return
			}

			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp struct {
				Type string `json:"__type"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(t, tt.wantErrType, errResp.Type)
		})
	}
}

// requestAndAwaitIssued creates an AMAZON_ISSUED certificate (optionally with
// Options.Export set) and polls DescribeCertificate until it reaches ISSUED
// (auto-validation fires after acm.autoValidateDelayMS, matching the existing
// TestACMHandler_GetCertificate_Issued_Succeeds pattern in this file), so
// tests exercising post-issuance behavior aren't racing the auto-validate
// timer.
func requestAndAwaitIssued(t *testing.T, h *acm.Handler, domainName, exportOption string) string {
	t.Helper()

	reqBody := map[string]any{"DomainName": domainName}
	if exportOption != "" {
		reqBody["Options"] = map[string]string{"Export": exportOption}
	}

	reqJSON, err := json.Marshal(reqBody)
	require.NoError(t, err)

	reqRec := postACMJSON(t, h, "RequestCertificate", string(reqJSON))
	require.Equal(t, http.StatusOK, reqRec.Code)

	var reqOut struct {
		CertificateArn string `json:"CertificateArn"`
	}
	require.NoError(t, json.Unmarshal(reqRec.Body.Bytes(), &reqOut))

	require.Eventually(t, func() bool {
		rec := postACMJSON(t, h, "DescribeCertificate",
			`{"CertificateArn":"`+reqOut.CertificateArn+`"}`)

		var out struct {
			Certificate struct {
				Status string `json:"Status"`
			} `json:"Certificate"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)

		return out.Certificate.Status == "ISSUED"
	}, 2*time.Second, 20*time.Millisecond)

	return reqOut.CertificateArn
}

func TestACMHandler_RenewCertificate_Imported_Returns_RequestInProgressException(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		certType string
	}{
		{name: "imported_cert_not_renewable", certType: "imported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			src, err := b.RequestCertificate(
				context.Background(),
				"renew-noteligible.example.com",
				"",
				"",
				"",
				"",
				"",
				"",
				nil,
			)
			require.NoError(t, err)

			imported, err := b.ImportCertificate(context.Background(), src.CertificateBody, src.PrivateKey, "", "")
			require.NoError(t, err)

			h := acm.NewHandler(b)
			body, _ := json.Marshal(map[string]string{"CertificateArn": imported.ARN})
			rec := postACMJSON(t, h, "RenewCertificate", string(body))
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp struct {
				Type string `json:"__type"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(t, "RequestInProgressException", errResp.Type,
				"RenewCertificate on IMPORTED cert must return RequestInProgressException, not RequestError")
		})
	}
}

func TestACMHandler_GetCertificate_NonIssuedStates_Returns_RequestInProgressException(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(t *testing.T, b *acm.InMemoryBackend) string
		name  string
	}{
		{
			name: "pending_validation",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"get-pending.example.com",
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
		},
		{
			name: "validation_timed_out",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"get-timedout.example.com",
					"",
					"DNS",
					"",
					"",
					"",
					"",
					nil,
				)
				require.NoError(t, err)
				require.NoError(t, b.TimeoutPendingValidation(context.Background(), cert.ARN))

				return cert.ARN
			},
		},
		{
			name: "failed",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"get-failed.example.com",
					"",
					"EMAIL",
					"",
					"",
					"",
					"",
					nil,
				)
				require.NoError(t, err)
				require.NoError(t, b.FailCertificate(context.Background(), cert.ARN, "CAA_ERROR"))

				return cert.ARN
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			certARN := tt.setup(t, b)

			h := acm.NewHandler(b)
			body, _ := json.Marshal(map[string]string{"CertificateArn": certARN})
			rec := postACMJSON(t, h, "GetCertificate", string(body))
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp struct {
				Type string `json:"__type"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(
				t,
				"RequestInProgressException",
				errResp.Type,
				"GetCertificate on non-issued cert (%s) must return RequestInProgressException, not InvalidStateException",
				tt.name,
			)
		})
	}
}

// STRENGTHENED (gopherstack-bzyl): previously asserted InvalidStateException, which
// RevokeCertificate's real deserializer never declares (deserializers.go, acm@v1.43.4).
// ConflictException is declared and its doc text ("wait for the previous operation to
// finish and try again") matches a cert still mid-validation; see
// TestACMHandler_RevokeCertificate_PendingValidationRejected for the fuller rationale.
func TestACMHandler_RevokeCertificate_PendingValidation_Returns_ConflictException(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "dns_validation_pending"},
		{name: "email_validation_pending"},
	}

	methods := map[string]string{
		"dns_validation_pending":   "DNS",
		"email_validation_pending": "EMAIL",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			method := methods[tt.name]
			body, _ := json.Marshal(map[string]string{
				"DomainName":       "revoke-pending.example.com",
				"ValidationMethod": method,
			})
			reqRec := postACMJSON(t, h, "RequestCertificate", string(body))
			require.Equal(t, http.StatusOK, reqRec.Code)

			var reqOut struct {
				CertificateArn string `json:"CertificateArn"`
			}
			require.NoError(t, json.Unmarshal(reqRec.Body.Bytes(), &reqOut))

			// Describe to check status; skip if auto-validated.
			descBody, _ := json.Marshal(map[string]string{"CertificateArn": reqOut.CertificateArn})
			descRec := postACMJSON(t, h, "DescribeCertificate", string(descBody))
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

			revokeBody, _ := json.Marshal(map[string]string{
				"CertificateArn":   reqOut.CertificateArn,
				"RevocationReason": "UNSPECIFIED",
			})
			rec := postACMJSON(t, h, "RevokeCertificate", string(revokeBody))
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp struct {
				Type string `json:"__type"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(
				t,
				"ConflictException",
				errResp.Type,
				"RevokeCertificate on PENDING_VALIDATION cert must return ConflictException, a code it "+
					"actually declares, not InvalidStateException or ValidationException",
			)
		})
	}
}

func TestACMHandler_UpdateCertificateOptions_NonIssued_Returns_InvalidStateException(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(t *testing.T, b *acm.InMemoryBackend) string
		name  string
	}{
		{
			name: "revoked_cert",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"update-opts-revoked.example.com",
					"",
					"",
					"",
					"",
					"",
					"",
					nil,
				)
				require.NoError(t, err)

				err = b.RevokeCertificate(context.Background(), cert.ARN, "UNSPECIFIED")
				require.NoError(t, err)

				return cert.ARN
			},
		},
		{
			name: "expired_cert",
			setup: func(t *testing.T, b *acm.InMemoryBackend) string {
				t.Helper()
				cert, err := b.RequestCertificate(
					context.Background(),
					"update-opts-expired.example.com",
					"",
					"",
					"",
					"",
					"",
					"",
					nil,
				)
				require.NoError(t, err)

				err = b.ExpireCertificate(context.Background(), cert.ARN)
				require.NoError(t, err)

				return cert.ARN
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := acm.NewInMemoryBackend("000000000000", "us-east-1")
			certARN := tt.setup(t, b)

			h := acm.NewHandler(b)
			body, _ := json.Marshal(map[string]any{
				"CertificateArn": certARN,
				"Options": map[string]string{
					"CertificateTransparencyLoggingPreference": "ENABLED",
				},
			})
			rec := postACMJSON(t, h, "UpdateCertificateOptions", string(body))
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp struct {
				Type string `json:"__type"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(
				t,
				"InvalidStateException",
				errResp.Type,
				"UpdateCertificateOptions on non-ISSUED cert (%s) must return InvalidStateException, not ValidationException",
				tt.name,
			)
		})
	}
}

// TestACMHandler_GetCertificate_Issued_Succeeds verifies the success path still works after fixes.
func TestACMHandler_GetCertificate_Issued_Succeeds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "amazon_issued_cert"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			reqRec := postACMJSON(t, h, "RequestCertificate", `{"DomainName":"get-issued.example.com"}`)
			require.Equal(t, http.StatusOK, reqRec.Code)

			var reqOut struct {
				CertificateArn string `json:"CertificateArn"`
			}
			require.NoError(t, json.Unmarshal(reqRec.Body.Bytes(), &reqOut))

			// Wait for ISSUED status (immediate for no-validation certs)
			require.Eventually(t, func() bool {
				rec := postACMJSON(t, h, "DescribeCertificate",
					`{"CertificateArn":"`+reqOut.CertificateArn+`"}`)
				var out struct {
					Certificate struct {
						Status string `json:"Status"`
					} `json:"Certificate"`
				}
				_ = json.Unmarshal(rec.Body.Bytes(), &out)

				return out.Certificate.Status == "ISSUED"
			}, 2*time.Second, 20*time.Millisecond)

			body, _ := json.Marshal(map[string]string{"CertificateArn": reqOut.CertificateArn})
			rec := postACMJSON(t, h, "GetCertificate", string(body))
			assert.Equal(t, http.StatusOK, rec.Code)

			var out struct {
				Certificate string `json:"Certificate"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Contains(t, out.Certificate, "BEGIN CERTIFICATE")
		})
	}
}
