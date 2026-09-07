package acm_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestACMHandler_RequestCertificate_InvalidParameterException verifies that RequestCertificate's
// own bad-input paths return InvalidParameterException, the code its real deserializer actually
// declares (deserializers.go:3346-3400+, aws-sdk-go-v2/service/acm@v1.43.4), not ValidationException
// -- which that op's deserializer does not recognize at all. gopherstack-bzyl.
func TestACMHandler_RequestCertificate_InvalidParameterException(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "empty_domain_name", body: `{"DomainName":""}`},
		{name: "domain_name_too_long", body: `{"DomainName":"` + tooLongDomain() + `"}`},
		{name: "domain_name_empty_label", body: `{"DomainName":"foo..example.com"}`},
		{name: "invalid_san", body: `{"DomainName":"ok.example.com","SubjectAlternativeNames":["foo..bad.com"]}`},
		{name: "invalid_managed_by", body: `{"DomainName":"ok2.example.com","ManagedBy":"BOGUS"}`},
		{name: "malformed_json_body", body: `{"DomainName":`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			rec := postACMJSON(t, h, "RequestCertificate", tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp struct {
				Type string `json:"__type"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(t, "InvalidParameterException", errResp.Type)
		})
	}
}

// TestACMHandler_RequestCertificate_IdempotencyMismatch_InvalidParameterException verifies that
// reusing an IdempotencyToken with different request parameters returns InvalidParameterException,
// not ValidationException -- same rationale as the table above. gopherstack-bzyl.
func TestACMHandler_RequestCertificate_IdempotencyMismatch_InvalidParameterException(t *testing.T) {
	t.Parallel()

	h := newACMHandler()

	first := postACMJSON(t, h, "RequestCertificate",
		`{"DomainName":"idem.example.com","IdempotencyToken":"tok-1"}`)
	require.Equal(t, http.StatusOK, first.Code)

	second := postACMJSON(t, h, "RequestCertificate",
		`{"DomainName":"other.example.com","IdempotencyToken":"tok-1"}`)
	assert.Equal(t, http.StatusBadRequest, second.Code)

	var errResp struct {
		Type string `json:"__type"`
	}
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &errResp))
	assert.Equal(t, "InvalidParameterException", errResp.Type)
}

// TestACMHandler_CreateAcmeDomainValidation_InvalidDomainName_StillValidationException guards the
// other side of the validateDomainName split: CreateAcmeDomainValidation's real deserializer DOES
// declare ValidationException, so its own bad-domain-shape path must keep returning it even after
// validateDomainName gained a caller-specific invalidErr parameter. gopherstack-bzyl.
func TestACMHandler_CreateAcmeDomainValidation_InvalidDomainName_StillValidationException(t *testing.T) {
	t.Parallel()

	h := newACMHandler()
	epARN := createTestAcmeEndpoint(t, h)

	body, err := json.Marshal(map[string]any{
		"AcmeEndpointArn":      epARN,
		"DomainName":           "foo..bad.com",
		"PrevalidationOptions": map[string]any{"DnsPrevalidation": map[string]any{}},
	})
	require.NoError(t, err)

	rec := postACMJSON(t, h, "CreateAcmeDomainValidation", string(body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp struct {
		Type string `json:"__type"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, "ValidationException", errResp.Type)
}

func tooLongDomain() string {
	label := strings.Repeat("a", 60)

	return strings.Repeat(label+".", 5) + "com"
}
