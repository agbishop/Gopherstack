package acm_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/acm"
)

func newACMHandler() *acm.Handler {
	return acm.NewHandler(acm.NewInMemoryBackend("000000000000", "us-east-1"))
}

// postACMJSON sends an ACM JSON-protocol request with the given target and body.
func postACMJSON(t *testing.T, h *acm.Handler, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "CertificateManager."+target)

	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestACMHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *acm.Handler)
		name         string
		target       string
		body         string
		wantContains []string
		wantCode     int
		omitTarget   bool
	}{
		{
			name:         "RequestCertificate",
			target:       "RequestCertificate",
			body:         `{"DomainName":"example.com"}`,
			wantCode:     http.StatusOK,
			wantContains: []string{"arn:aws:acm:"},
		},
		{
			name:     "RequestCertificate_EmptyDomain",
			target:   "RequestCertificate",
			body:     `{"DomainName":""}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:         "RequestCertificate_DNS_validation",
			target:       "RequestCertificate",
			body:         `{"DomainName":"dns.example.com","ValidationMethod":"DNS"}`,
			wantCode:     http.StatusOK,
			wantContains: []string{"arn:aws:acm:"},
		},
		{
			name:     "DescribeCertificate_NotFound",
			target:   "DescribeCertificate",
			body:     `{"CertificateArn":"arn:aws:acm:us-east-1:000000000000:certificate/nonexistent"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "DescribeCertificate_AfterCreate",
			target: "DescribeCertificate",
			setup: func(t *testing.T, h *acm.Handler) {
				t.Helper()
				postACMJSON(t, h, "RequestCertificate", `{"DomainName":"describe-test.com"}`)
			},
			body:         "", // filled dynamically below won't work; use setup to get ARN
			wantCode:     http.StatusOK,
			wantContains: []string{"describe-test.com"},
		},
		{
			name:   "DescribeCertificate_DNS_CNAME_records",
			target: "DescribeCertificate",
			setup: func(t *testing.T, h *acm.Handler) {
				t.Helper()
				postACMJSON(t, h, "RequestCertificate",
					`{"DomainName":"cname-test.com","ValidationMethod":"DNS"}`)
			},
			body:         "",
			wantCode:     http.StatusOK,
			wantContains: []string{"CNAME", "acm-validations.aws", "cname-test.com"},
		},
		{
			name:   "ListCertificates",
			target: "ListCertificates",
			setup: func(t *testing.T, h *acm.Handler) {
				t.Helper()
				postACMJSON(t, h, "RequestCertificate", `{"DomainName":"list1.com"}`)
				postACMJSON(t, h, "RequestCertificate", `{"DomainName":"list2.com"}`)
			},
			body:         `{}`,
			wantCode:     http.StatusOK,
			wantContains: []string{"list1.com", "list2.com"},
		},
		{
			name:   "DeleteCertificate",
			target: "DeleteCertificate",
			setup: func(t *testing.T, h *acm.Handler) {
				t.Helper()
				postACMJSON(t, h, "RequestCertificate", `{"DomainName":"delete-test.com"}`)
			},
			body:     "",
			wantCode: http.StatusOK,
		},
		{
			name:         "DeleteCertificate_NotFound",
			target:       "DeleteCertificate",
			body:         `{"CertificateArn":"arn:aws:acm:us-east-1:000000000000:certificate/nonexistent"}`,
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"ResourceNotFoundException"},
		},
		{
			name:   "AddTagsToCertificate",
			target: "AddTagsToCertificate",
			setup: func(t *testing.T, h *acm.Handler) {
				t.Helper()
				postACMJSON(t, h, "RequestCertificate", `{"DomainName":"tag-t1.com"}`)
			},
			// body is empty: test runner will extract the ARN from the cert list and
			// send {"CertificateArn": "<arn>"} — empty Tags is valid per AWS API.
			body:     "",
			wantCode: http.StatusOK,
		},
		{
			name:   "ListTagsForCertificate",
			target: "ListTagsForCertificate",
			setup: func(t *testing.T, h *acm.Handler) {
				t.Helper()
				postACMJSON(t, h, "RequestCertificate", `{"DomainName":"tag-t2.com"}`)
			},
			// body is empty: test runner injects {"CertificateArn": "<arn>"}.
			body:         "",
			wantCode:     http.StatusOK,
			wantContains: []string{"Tags"},
		},
		{
			name:   "RemoveTagsFromCertificate",
			target: "RemoveTagsFromCertificate",
			setup: func(t *testing.T, h *acm.Handler) {
				t.Helper()
				postACMJSON(t, h, "RequestCertificate", `{"DomainName":"tag-t3.com"}`)
			},
			// body is empty: test runner injects {"CertificateArn": "<arn>"}; empty Tags is valid.
			body:     "",
			wantCode: http.StatusOK,
		},
		{
			name:         "UnknownAction",
			target:       "BogusAction",
			body:         `{}`,
			wantCode:     http.StatusBadRequest,
			wantContains: []string{"InvalidAction"},
		},
		{
			name:       "MissingAction",
			body:       `{}`,
			omitTarget: true,
			wantCode:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			var rec *httptest.ResponseRecorder
			if tt.omitTarget {
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/x-amz-json-1.1")
				rec = httptest.NewRecorder()
				e := echo.New()
				c := e.NewContext(req, rec)
				err := h.Handler()(c)
				require.NoError(t, err)
			} else {
				body := tt.body
				if body == "" {
					// For tests that need to reuse an ARN: list certs and use first ARN
					listRec := postACMJSON(t, h, "ListCertificates", `{}`)
					var listResp struct {
						CertificateSummaryList []struct {
							CertificateArn string `json:"CertificateArn"`
						} `json:"CertificateSummaryList"`
					}
					require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))
					require.NotEmpty(t, listResp.CertificateSummaryList)
					b, _ := json.Marshal(
						map[string]string{"CertificateArn": listResp.CertificateSummaryList[0].CertificateArn},
					)
					body = string(b)
				}
				rec = postACMJSON(t, h, tt.target, body)
			}

			assert.Equal(t, tt.wantCode, rec.Code)
			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

// createTestAcmeEndpoint creates an ACME endpoint on h and returns its ARN.
func createTestAcmeEndpoint(t *testing.T, h *acm.Handler) string {
	t.Helper()

	rec := postACMJSON(t, h, "CreateAcmeEndpoint",
		`{"AuthorizationBehavior":"PRE_APPROVED",`+
			`"CertificateAuthority":{"PublicCertificateAuthority":{"AllowedKeyAlgorithms":["RSA_2048"]}}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var out struct {
		AcmeEndpointArn string `json:"AcmeEndpointArn"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.NotEmpty(t, out.AcmeEndpointArn)

	return out.AcmeEndpointArn
}

// TestACMHandler_AcmeEndpoints covers CreateAcmeEndpoint/DescribeAcmeEndpoint/
// ListAcmeEndpoints/UpdateAcmeEndpoint/DeleteAcmeEndpoint, field-diffed
// against aws-sdk-go-v2/service/acm's Create/Describe/List/Update/DeleteAcmeEndpoint
// Input/Output shapes (AcmeEndpointArn, AuthorizationBehavior,
// CertificateAuthority{PublicCertificateAuthority{AllowedKeyAlgorithms}},
// Contact, EndpointUrl, Status, CreatedAt/UpdatedAt).
func TestACMHandler_AcmeEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *acm.Handler)
		name string
	}{
		{
			name: "Create_Success",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				assert.Contains(t, epARN, "acme-endpoint/")
			},
		},
		{
			name: "Create_MissingCertificateAuthority_Validation",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				rec := postACMJSON(t, h, "CreateAcmeEndpoint", `{"AuthorizationBehavior":"PRE_APPROVED"}`)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")
			},
		},
		{
			name: "Create_BadAuthorizationBehavior_Validation",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				rec := postACMJSON(t, h, "CreateAcmeEndpoint",
					`{"AuthorizationBehavior":"BOGUS",`+
						`"CertificateAuthority":{"PublicCertificateAuthority":{}}}`)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")
			},
		},
		{
			name: "Describe_Success",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				body, _ := json.Marshal(map[string]string{"AcmeEndpointArn": epARN})
				rec := postACMJSON(t, h, "DescribeAcmeEndpoint", string(body))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				assert.Contains(t, rec.Body.String(), `"Status":"ACTIVE"`)
				assert.Contains(t, rec.Body.String(), `"AuthorizationBehavior":"PRE_APPROVED"`)
				assert.Contains(t, rec.Body.String(), "PublicCertificateAuthority")
			},
		},
		{
			name: "Describe_NotFound",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				body := `{"AcmeEndpointArn":"arn:aws:acm:us-east-1:000000000000:acme-endpoint/nope"}`
				rec := postACMJSON(t, h, "DescribeAcmeEndpoint", body)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ResourceNotFoundException")
			},
		},
		{
			name: "Describe_MalformedArn_Validation",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				rec := postACMJSON(t, h, "DescribeAcmeEndpoint", `{"AcmeEndpointArn":"not-an-arn"}`)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")
				assert.NotContains(t, rec.Body.String(), "InvalidArnException")
			},
		},
		{
			name: "List_ContainsCreated",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				rec := postACMJSON(t, h, "ListAcmeEndpoints", `{}`)
				require.Equal(t, http.StatusOK, rec.Code)
				assert.Contains(t, rec.Body.String(), epARN)
			},
		},
		{
			name: "Update_ChangesContact",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				updBody, _ := json.Marshal(map[string]string{"AcmeEndpointArn": epARN, "Contact": "REQUIRED"})
				updRec := postACMJSON(t, h, "UpdateAcmeEndpoint", string(updBody))
				require.Equal(t, http.StatusOK, updRec.Code, updRec.Body.String())

				descBody, _ := json.Marshal(map[string]string{"AcmeEndpointArn": epARN})
				descRec := postACMJSON(t, h, "DescribeAcmeEndpoint", string(descBody))
				assert.Contains(t, descRec.Body.String(), `"Contact":"REQUIRED"`)
			},
		},
		{
			name: "Delete_Success",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				delBody, _ := json.Marshal(map[string]string{"AcmeEndpointArn": epARN})
				delRec := postACMJSON(t, h, "DeleteAcmeEndpoint", string(delBody))
				require.Equal(t, http.StatusOK, delRec.Code)

				descRec := postACMJSON(t, h, "DescribeAcmeEndpoint", string(delBody))
				assert.Equal(t, http.StatusBadRequest, descRec.Code)
			},
		},
		{
			// DeleteAcmeEndpoint's deserializer declares no
			// ResourceNotFoundException, only ValidationException --
			// gopherstack-ftkd.
			name: "Delete_NotFound_ReturnsValidationException",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)

				delBody := `{"AcmeEndpointArn":"arn:aws:acm:us-east-1:000000000000:acme-endpoint/nope"}`
				delRec := postACMJSON(t, h, "DeleteAcmeEndpoint", delBody)
				assert.Equal(t, http.StatusBadRequest, delRec.Code)
				assert.Contains(t, delRec.Body.String(), "ValidationException")
				assert.NotContains(t, delRec.Body.String(), "ResourceNotFoundException")

				descBody, _ := json.Marshal(map[string]string{"AcmeEndpointArn": epARN})
				descRec := postACMJSON(t, h, "DescribeAcmeEndpoint", string(descBody))
				assert.Equal(t, http.StatusOK, descRec.Code, "unrelated endpoint must be untouched")
			},
		},
		{
			// DeleteAcmeEndpoint must cascade-delete every EAB and domain
			// validation owned by it (see acme_endpoints.go's
			// DeleteAcmeEndpoint doc) rather than leaving orphans behind.
			name: "Delete_CascadesToChildren",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)

				eabBody, _ := json.Marshal(map[string]string{
					"AcmeEndpointArn": epARN,
					"RoleArn":         "arn:aws:iam::000000000000:role/acme-role",
				})
				eabRec := postACMJSON(t, h, "CreateAcmeExternalAccountBinding", string(eabBody))
				require.Equal(t, http.StatusOK, eabRec.Code, eabRec.Body.String())

				var eabOut struct {
					ExternalAccountBinding struct {
						AcmeExternalAccountBindingArn string `json:"AcmeExternalAccountBindingArn"`
					} `json:"ExternalAccountBinding"`
				}
				require.NoError(t, json.Unmarshal(eabRec.Body.Bytes(), &eabOut))
				eabARN := eabOut.ExternalAccountBinding.AcmeExternalAccountBindingArn
				require.NotEmpty(t, eabARN)

				dvBody, _ := json.Marshal(map[string]any{
					"AcmeEndpointArn":      epARN,
					"DomainName":           "cascade.example.com",
					"PrevalidationOptions": map[string]any{"DnsPrevalidation": map[string]any{}},
				})
				dvRec := postACMJSON(t, h, "CreateAcmeDomainValidation", string(dvBody))
				require.Equal(t, http.StatusOK, dvRec.Code, dvRec.Body.String())

				var dvOut struct {
					AcmeDomainValidationArn string `json:"AcmeDomainValidationArn"`
				}
				require.NoError(t, json.Unmarshal(dvRec.Body.Bytes(), &dvOut))
				require.NotEmpty(t, dvOut.AcmeDomainValidationArn)

				delBody, _ := json.Marshal(map[string]string{"AcmeEndpointArn": epARN})
				delRec := postACMJSON(t, h, "DeleteAcmeEndpoint", string(delBody))
				require.Equal(t, http.StatusOK, delRec.Code)

				descEABBody, _ := json.Marshal(map[string]string{"AcmeExternalAccountBindingArn": eabARN})
				descEABRec := postACMJSON(t, h, "DescribeAcmeExternalAccountBinding", string(descEABBody))
				assert.Equal(t, http.StatusBadRequest, descEABRec.Code, "EAB must be cascade-deleted with its endpoint")

				descDVBody, _ := json.Marshal(
					map[string]string{"AcmeDomainValidationArn": dvOut.AcmeDomainValidationArn},
				)
				descDVRec := postACMJSON(t, h, "DescribeAcmeDomainValidation", string(descDVBody))
				assert.Equal(t, http.StatusBadRequest, descDVRec.Code,
					"domain validation must be cascade-deleted with its endpoint")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			tt.run(t, h)
		})
	}
}

// TestACMHandler_AcmeExternalAccountBindings covers Create/Describe/List/
// GetCredentials/Revoke/DeleteAcmeExternalAccountBinding, field-diffed
// against the real SDK's AcmeExternalAccountBinding shape (AcmeEndpointArn,
// AcmeExternalAccountBindingArn, RoleArn, CreatedAt/UpdatedAt/ExpiresAt/
// LastUsedAt/RevokedAt) and GetAcmeExternalAccountBindingCredentialsOutput
// (KeyId, MacKey).
func TestACMHandler_AcmeExternalAccountBindings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *acm.Handler)
		name string
	}{
		{
			name: "Create_Success",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				body, _ := json.Marshal(map[string]any{
					"AcmeEndpointArn": epARN,
					"RoleArn":         "arn:aws:iam::000000000000:role/acme-role",
					"Expiration":      map[string]any{"Type": "DAYS", "Value": 7},
				})
				rec := postACMJSON(t, h, "CreateAcmeExternalAccountBinding", string(body))
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				assert.Contains(t, rec.Body.String(), "acme-external-account-binding/")
				assert.Contains(t, rec.Body.String(), `"ExpiresAt"`)
			},
		},
		{
			name: "Create_BadRoleArn_Validation",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				body, _ := json.Marshal(map[string]string{"AcmeEndpointArn": epARN, "RoleArn": "not-a-role-arn"})
				rec := postACMJSON(t, h, "CreateAcmeExternalAccountBinding", string(body))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")
			},
		},
		{
			name: "Create_EndpointNotFound",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				body, _ := json.Marshal(map[string]string{
					"AcmeEndpointArn": "arn:aws:acm:us-east-1:000000000000:acme-endpoint/nope",
					"RoleArn":         "arn:aws:iam::000000000000:role/acme-role",
				})
				rec := postACMJSON(t, h, "CreateAcmeExternalAccountBinding", string(body))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ResourceNotFoundException")
			},
		},
		{
			name: "GetCredentials_ThenRevoke_ThenCredentialsRejected",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				createBody, _ := json.Marshal(map[string]string{
					"AcmeEndpointArn": epARN,
					"RoleArn":         "arn:aws:iam::000000000000:role/acme-role",
				})
				createRec := postACMJSON(t, h, "CreateAcmeExternalAccountBinding", string(createBody))
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut struct {
					ExternalAccountBinding struct {
						AcmeExternalAccountBindingArn string `json:"AcmeExternalAccountBindingArn"`
					} `json:"ExternalAccountBinding"`
				}
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
				eabARN := createOut.ExternalAccountBinding.AcmeExternalAccountBindingArn

				credBody, _ := json.Marshal(map[string]string{"AcmeExternalAccountBindingArn": eabARN})
				credRec := postACMJSON(t, h, "GetAcmeExternalAccountBindingCredentials", string(credBody))
				require.Equal(t, http.StatusOK, credRec.Code)

				var credOut struct {
					KeyID  string `json:"KeyId"`
					MacKey string `json:"MacKey"`
				}
				require.NoError(t, json.Unmarshal(credRec.Body.Bytes(), &credOut))
				assert.NotEmpty(t, credOut.KeyID)
				assert.NotEmpty(t, credOut.MacKey)

				revokeRec := postACMJSON(t, h, "RevokeAcmeExternalAccountBinding", string(credBody))
				require.Equal(t, http.StatusOK, revokeRec.Code)

				descBeforeRec := postACMJSON(t, h, "DescribeAcmeExternalAccountBinding", string(credBody))
				require.Equal(t, http.StatusOK, descBeforeRec.Code)

				// Revoking again must fail with ConflictException --
				// RevokeAcmeExternalAccountBinding's deserializer declares
				// ConflictException, not InvalidStateException, for an
				// already-revoked EAB (gopherstack-ftkd).
				revokeAgainRec := postACMJSON(t, h, "RevokeAcmeExternalAccountBinding", string(credBody))
				assert.Equal(t, http.StatusBadRequest, revokeAgainRec.Code)
				assert.Contains(t, revokeAgainRec.Body.String(), "ConflictException")
				assert.NotContains(t, revokeAgainRec.Body.String(), "InvalidStateException")

				// The rejected second revoke must not have mutated the EAB.
				descAfterRec := postACMJSON(t, h, "DescribeAcmeExternalAccountBinding", string(credBody))
				require.Equal(t, http.StatusOK, descAfterRec.Code)
				assert.JSONEq(t, descBeforeRec.Body.String(), descAfterRec.Body.String())

				// GetAcmeExternalAccountBindingCredentials declares neither
				// ConflictException nor InvalidStateException, only
				// ValidationException, for a revoked EAB (gopherstack-ftkd).
				credAfterRevokeRec := postACMJSON(t, h, "GetAcmeExternalAccountBindingCredentials", string(credBody))
				assert.Equal(t, http.StatusBadRequest, credAfterRevokeRec.Code,
					"credentials must not be issued for a revoked EAB")
				assert.Contains(t, credAfterRevokeRec.Body.String(), "ValidationException")
				assert.NotContains(t, credAfterRevokeRec.Body.String(), "InvalidStateException")
			},
		},
		{
			name: "List_ContainsCreated",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				createBody, _ := json.Marshal(map[string]string{
					"AcmeEndpointArn": epARN,
					"RoleArn":         "arn:aws:iam::000000000000:role/acme-role",
				})
				createRec := postACMJSON(t, h, "CreateAcmeExternalAccountBinding", string(createBody))
				require.Equal(t, http.StatusOK, createRec.Code)

				listBody, _ := json.Marshal(map[string]string{"AcmeEndpointArn": epARN})
				listRec := postACMJSON(t, h, "ListAcmeExternalAccountBindings", string(listBody))
				require.Equal(t, http.StatusOK, listRec.Code)
				assert.Contains(t, listRec.Body.String(), "acme-external-account-binding/")
			},
		},
		{
			name: "Delete_Success",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				createBody, _ := json.Marshal(map[string]string{
					"AcmeEndpointArn": epARN,
					"RoleArn":         "arn:aws:iam::000000000000:role/acme-role",
				})
				createRec := postACMJSON(t, h, "CreateAcmeExternalAccountBinding", string(createBody))
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut struct {
					ExternalAccountBinding struct {
						AcmeExternalAccountBindingArn string `json:"AcmeExternalAccountBindingArn"`
					} `json:"ExternalAccountBinding"`
				}
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

				delBody, _ := json.Marshal(map[string]string{
					"AcmeExternalAccountBindingArn": createOut.ExternalAccountBinding.AcmeExternalAccountBindingArn,
				})
				delRec := postACMJSON(t, h, "DeleteAcmeExternalAccountBinding", string(delBody))
				assert.Equal(t, http.StatusOK, delRec.Code)
			},
		},
		{
			// DeleteAcmeExternalAccountBinding's deserializer declares no
			// ResourceNotFoundException, only ValidationException --
			// gopherstack-ftkd.
			name: "Delete_NotFound_ReturnsValidationException",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				createBody, _ := json.Marshal(map[string]string{
					"AcmeEndpointArn": epARN,
					"RoleArn":         "arn:aws:iam::000000000000:role/acme-role",
				})
				createRec := postACMJSON(t, h, "CreateAcmeExternalAccountBinding", string(createBody))
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut struct {
					ExternalAccountBinding struct {
						AcmeExternalAccountBindingArn string `json:"AcmeExternalAccountBindingArn"`
					} `json:"ExternalAccountBinding"`
				}
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
				eabARN := createOut.ExternalAccountBinding.AcmeExternalAccountBindingArn

				delBody := `{"AcmeExternalAccountBindingArn":"` + epARN +
					`/acme-external-account-binding/nope"}`
				delRec := postACMJSON(t, h, "DeleteAcmeExternalAccountBinding", delBody)
				assert.Equal(t, http.StatusBadRequest, delRec.Code)
				assert.Contains(t, delRec.Body.String(), "ValidationException")
				assert.NotContains(t, delRec.Body.String(), "ResourceNotFoundException")

				descBody, _ := json.Marshal(map[string]string{"AcmeExternalAccountBindingArn": eabARN})
				descRec := postACMJSON(t, h, "DescribeAcmeExternalAccountBinding", string(descBody))
				assert.Equal(t, http.StatusOK, descRec.Code, "unrelated EAB must be untouched")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			tt.run(t, h)
		})
	}
}

// TestACMHandler_AcmeDomainValidations covers Create/Describe/List/Update/
// DeleteAcmeDomainValidation, field-diffed against the real SDK's
// AcmeDomainValidation shape (AcmeDomainValidationArn, AcmeEndpointArn,
// DomainName, PrevalidationType, PrevalidationDetails.DnsPrevalidation,
// Status). Status must always be VALIDATING -- gopherstack has no DNS
// resolver to actually check the synthesized ResourceRecord against, so it
// must never claim VALID (see acme_domain_validations.go's doc comment).
func TestACMHandler_AcmeDomainValidations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *acm.Handler)
		name string
	}{
		{
			name: "Create_Success_StatusIsValidating",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				body, _ := json.Marshal(map[string]any{
					"AcmeEndpointArn": epARN,
					"DomainName":      "dv.example.com",
					"PrevalidationOptions": map[string]any{
						"DnsPrevalidation": map[string]any{"HostedZoneId": "Z123456"},
					},
				})
				createRec := postACMJSON(t, h, "CreateAcmeDomainValidation", string(body))
				require.Equal(t, http.StatusOK, createRec.Code, createRec.Body.String())

				var createOut struct {
					AcmeDomainValidationArn string `json:"AcmeDomainValidationArn"`
				}
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

				descBody, _ := json.Marshal(
					map[string]string{"AcmeDomainValidationArn": createOut.AcmeDomainValidationArn},
				)
				descRec := postACMJSON(t, h, "DescribeAcmeDomainValidation", string(descBody))
				require.Equal(t, http.StatusOK, descRec.Code)
				assert.Contains(t, descRec.Body.String(), `"Status":"VALIDATING"`)
				assert.Contains(t, descRec.Body.String(), "DnsPrevalidation")
				assert.Contains(t, descRec.Body.String(), "ResourceRecord")
				assert.Contains(t, descRec.Body.String(), "dv.example.com")
			},
		},
		{
			name: "Create_MissingPrevalidationOptions_Validation",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				body, _ := json.Marshal(map[string]string{"AcmeEndpointArn": epARN, "DomainName": "dv2.example.com"})
				rec := postACMJSON(t, h, "CreateAcmeDomainValidation", string(body))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")
			},
		},
		{
			name: "List_ContainsCreated",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				body, _ := json.Marshal(map[string]any{
					"AcmeEndpointArn":      epARN,
					"DomainName":           "dv3.example.com",
					"PrevalidationOptions": map[string]any{"DnsPrevalidation": map[string]any{}},
				})
				createRec := postACMJSON(t, h, "CreateAcmeDomainValidation", string(body))
				require.Equal(t, http.StatusOK, createRec.Code)

				listBody, _ := json.Marshal(map[string]string{"AcmeEndpointArn": epARN})
				listRec := postACMJSON(t, h, "ListAcmeDomainValidations", string(listBody))
				require.Equal(t, http.StatusOK, listRec.Code)
				assert.Contains(t, listRec.Body.String(), "dv3.example.com")
			},
		},
		{
			name: "Update_RegeneratesResourceRecord",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				body, _ := json.Marshal(map[string]any{
					"AcmeEndpointArn":      epARN,
					"DomainName":           "dv4.example.com",
					"PrevalidationOptions": map[string]any{"DnsPrevalidation": map[string]any{}},
				})
				createRec := postACMJSON(t, h, "CreateAcmeDomainValidation", string(body))
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut struct {
					AcmeDomainValidationArn string `json:"AcmeDomainValidationArn"`
				}
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

				updBody, _ := json.Marshal(map[string]any{
					"AcmeDomainValidationArn": createOut.AcmeDomainValidationArn,
					"PrevalidationOptions": map[string]any{
						"DnsPrevalidation": map[string]any{"HostedZoneId": "Znew"},
					},
				})
				updRec := postACMJSON(t, h, "UpdateAcmeDomainValidation", string(updBody))
				require.Equal(t, http.StatusOK, updRec.Code, updRec.Body.String())

				descBody, _ := json.Marshal(
					map[string]string{"AcmeDomainValidationArn": createOut.AcmeDomainValidationArn},
				)
				descRec := postACMJSON(t, h, "DescribeAcmeDomainValidation", string(descBody))
				assert.Contains(t, descRec.Body.String(), `"HostedZoneId":"Znew"`)
			},
		},
		{
			name: "Delete_Success",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				body, _ := json.Marshal(map[string]any{
					"AcmeEndpointArn":      epARN,
					"DomainName":           "dv5.example.com",
					"PrevalidationOptions": map[string]any{"DnsPrevalidation": map[string]any{}},
				})
				createRec := postACMJSON(t, h, "CreateAcmeDomainValidation", string(body))
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut struct {
					AcmeDomainValidationArn string `json:"AcmeDomainValidationArn"`
				}
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

				delBody, _ := json.Marshal(
					map[string]string{"AcmeDomainValidationArn": createOut.AcmeDomainValidationArn},
				)
				delRec := postACMJSON(t, h, "DeleteAcmeDomainValidation", string(delBody))
				assert.Equal(t, http.StatusOK, delRec.Code)
			},
		},
		{
			// DeleteAcmeDomainValidation's deserializer declares no
			// ResourceNotFoundException, only ValidationException --
			// gopherstack-ftkd.
			name: "Delete_NotFound_ReturnsValidationException",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				body, _ := json.Marshal(map[string]any{
					"AcmeEndpointArn":      epARN,
					"DomainName":           "dv6.example.com",
					"PrevalidationOptions": map[string]any{"DnsPrevalidation": map[string]any{}},
				})
				createRec := postACMJSON(t, h, "CreateAcmeDomainValidation", string(body))
				require.Equal(t, http.StatusOK, createRec.Code)

				var createOut struct {
					AcmeDomainValidationArn string `json:"AcmeDomainValidationArn"`
				}
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))

				delBody := `{"AcmeDomainValidationArn":"` + epARN + `/acme-domain-validation/nope"}`
				delRec := postACMJSON(t, h, "DeleteAcmeDomainValidation", delBody)
				assert.Equal(t, http.StatusBadRequest, delRec.Code)
				assert.Contains(t, delRec.Body.String(), "ValidationException")
				assert.NotContains(t, delRec.Body.String(), "ResourceNotFoundException")

				descBody, _ := json.Marshal(
					map[string]string{"AcmeDomainValidationArn": createOut.AcmeDomainValidationArn},
				)
				descRec := postACMJSON(t, h, "DescribeAcmeDomainValidation", string(descBody))
				assert.Equal(t, http.StatusOK, descRec.Code, "unrelated domain validation must be untouched")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			tt.run(t, h)
		})
	}
}

// TestACMHandler_AcmeAccounts_HonestlyEmpty proves the deliberate scope
// decision documented on AcmeAccount (acme_accounts.go): real ACME accounts
// are created by an ACME client's own protocol call against the endpoint's
// EndpointUrl, which gopherstack does not implement, so
// Describe/List/RevokeAcmeAccount are wired against real (always empty)
// backend state and validate their AcmeEndpointArn FK for real, but never
// claim an account exists.
func TestACMHandler_AcmeAccounts_HonestlyEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *acm.Handler, epARN string)
		name string
	}{
		{
			name: "Describe_NeverFound",
			run: func(t *testing.T, h *acm.Handler, epARN string) {
				t.Helper()

				body, _ := json.Marshal(map[string]string{
					"AcmeEndpointArn": epARN, "AccountUrl": "https://example/acct/1",
				})
				rec := postACMJSON(t, h, "DescribeAcmeAccount", string(body))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ResourceNotFoundException")
			},
		},
		{
			name: "List_AlwaysEmpty",
			run: func(t *testing.T, h *acm.Handler, epARN string) {
				t.Helper()

				body, _ := json.Marshal(map[string]string{"AcmeEndpointArn": epARN})
				rec := postACMJSON(t, h, "ListAcmeAccounts", string(body))
				require.Equal(t, http.StatusOK, rec.Code)
				assert.Contains(t, rec.Body.String(), `"AcmeAccounts":[]`)
			},
		},
		{
			name: "Revoke_NeverFound",
			run: func(t *testing.T, h *acm.Handler, epARN string) {
				t.Helper()

				body, _ := json.Marshal(map[string]string{
					"AcmeEndpointArn": epARN, "AccountUrl": "https://example/acct/1",
				})
				rec := postACMJSON(t, h, "RevokeAcmeAccount", string(body))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ResourceNotFoundException")
			},
		},
		{
			name: "List_EndpointNotFound",
			run: func(t *testing.T, h *acm.Handler, _ string) {
				t.Helper()

				body := `{"AcmeEndpointArn":"arn:aws:acm:us-east-1:000000000000:acme-endpoint/nope"}`
				rec := postACMJSON(t, h, "ListAcmeAccounts", body)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ResourceNotFoundException")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			epARN := createTestAcmeEndpoint(t, h)
			tt.run(t, h, epARN)
		})
	}
}
