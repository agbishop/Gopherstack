package acm_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/acm"
)

// TestACMHandler_TagOps_CertExistenceValidation verifies that tag ops reject unknown ARNs.
func TestACMHandler_TagOps_CertExistenceValidation(t *testing.T) {
	t.Parallel()

	const fakeARN = "arn:aws:acm:us-east-1:000000000000:certificate/does-not-exist"

	tests := []struct {
		name   string
		action string
		body   string
	}{
		{
			name:   "AddTags_NotFound",
			action: "AddTagsToCertificate",
			body:   `{"CertificateArn":"` + fakeARN + `","Tags":[{"Key":"k","Value":"v"}]}`,
		},
		{
			name:   "ListTags_NotFound",
			action: "ListTagsForCertificate",
			body:   `{"CertificateArn":"` + fakeARN + `"}`,
		},
		{
			name:   "RemoveTags_NotFound",
			action: "RemoveTagsFromCertificate",
			body:   `{"CertificateArn":"` + fakeARN + `","Tags":[{"Key":"k"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newACMHandler()
			rec := postACMJSON(t, h, tt.action, tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "ResourceNotFoundException")
		})
	}
}

// TestACMHandler_RequestCertificate_Tags verifies that tags passed at request time are stored.
func TestACMHandler_RequestCertificate_Tags(t *testing.T) {
	t.Parallel()

	h := newACMHandler()

	body := `{"DomainName":"tagged.example.com","Tags":[{"Key":"env","Value":"test"},{"Key":"team","Value":"infra"}]}`
	rec := postACMJSON(t, h, "RequestCertificate", body)
	require.Equal(t, http.StatusOK, rec.Code)

	var out struct {
		CertificateArn string `json:"CertificateArn"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	// Verify tags are stored
	listBody, _ := json.Marshal(map[string]string{"CertificateArn": out.CertificateArn})
	listRec := postACMJSON(t, h, "ListTagsForCertificate", string(listBody))
	require.Equal(t, http.StatusOK, listRec.Code)

	var tagsOut struct {
		Tags []map[string]string `json:"Tags"`
	}
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &tagsOut))

	tagMap := make(map[string]string)
	for _, t2 := range tagsOut.Tags {
		tagMap[t2["Key"]] = t2["Value"]
	}

	assert.Equal(t, "test", tagMap["env"])
	assert.Equal(t, "infra", tagMap["team"])
}

// TestACMHandler_GenericResourceTags_RouteByArnType verifies TagResource/
// ListTagsForResource/UntagResource (field-diffed against the real SDK's
// Tag/ListTagsForResource/UntagResourceInput -- ResourceArn + Tags/TagKeys)
// resolve their ResourceArn to the correct resource type -- certificate,
// ACME endpoint, or ACME external account binding -- rather than assuming a
// certificate, and that a certificate's tags are the SAME underlying set
// AddTagsToCertificate/ListTagsForCertificate see (see
// resolveTaggableResourceArn's doc in handler_resource_tags.go).
func TestACMHandler_GenericResourceTags_RouteByArnType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *acm.Handler)
		name string
	}{
		{
			name: "CertificateArn_SharesStoreWithAddTagsToCertificate",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				reqRec := postACMJSON(t, h, "RequestCertificate", `{"DomainName":"shared-tags.example.com"}`)
				require.Equal(t, http.StatusOK, reqRec.Code)

				var reqOut struct {
					CertificateArn string `json:"CertificateArn"`
				}
				require.NoError(t, json.Unmarshal(reqRec.Body.Bytes(), &reqOut))

				// Tag via the certificate-specific op...
				addBody, _ := json.Marshal(map[string]any{
					"CertificateArn": reqOut.CertificateArn,
					"Tags":           []map[string]string{{"Key": "via", "Value": "AddTagsToCertificate"}},
				})
				addRec := postACMJSON(t, h, "AddTagsToCertificate", string(addBody))
				require.Equal(t, http.StatusOK, addRec.Code)

				// ...and read it back via the generic op.
				listBody, _ := json.Marshal(map[string]string{"ResourceArn": reqOut.CertificateArn})
				listRec := postACMJSON(t, h, "ListTagsForResource", string(listBody))
				require.Equal(t, http.StatusOK, listRec.Code)
				assert.Contains(t, listRec.Body.String(), "AddTagsToCertificate")

				// Tag via the generic op...
				tagBody, _ := json.Marshal(map[string]any{
					"ResourceArn": reqOut.CertificateArn,
					"Tags":        []map[string]string{{"Key": "via", "Value": "TagResource"}},
				})
				tagRec := postACMJSON(t, h, "TagResource", string(tagBody))
				require.Equal(t, http.StatusOK, tagRec.Code, tagRec.Body.String())

				// ...and read it back via the certificate-specific op.
				certListBody, _ := json.Marshal(map[string]string{"CertificateArn": reqOut.CertificateArn})
				certListRec := postACMJSON(t, h, "ListTagsForCertificate", string(certListBody))
				require.Equal(t, http.StatusOK, certListRec.Code)
				assert.Contains(t, certListRec.Body.String(), "TagResource")
			},
		},
		{
			name: "AcmeEndpointArn_TagAndUntag",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)

				tagBody, _ := json.Marshal(map[string]any{
					"ResourceArn": epARN,
					"Tags":        []map[string]string{{"Key": "env", "Value": "prod"}},
				})
				tagRec := postACMJSON(t, h, "TagResource", string(tagBody))
				require.Equal(t, http.StatusOK, tagRec.Code, tagRec.Body.String())

				listBody, _ := json.Marshal(map[string]string{"ResourceArn": epARN})
				listRec := postACMJSON(t, h, "ListTagsForResource", string(listBody))
				require.Equal(t, http.StatusOK, listRec.Code)
				assert.Contains(t, listRec.Body.String(), "prod")

				untagBody, _ := json.Marshal(map[string]any{"ResourceArn": epARN, "TagKeys": []string{"env"}})
				untagRec := postACMJSON(t, h, "UntagResource", string(untagBody))
				require.Equal(t, http.StatusOK, untagRec.Code)

				listAfterRec := postACMJSON(t, h, "ListTagsForResource", string(listBody))
				require.Equal(t, http.StatusOK, listAfterRec.Code)
				assert.NotContains(t, listAfterRec.Body.String(), "prod")
			},
		},
		{
			name: "UnknownResource_ResourceNotFound",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				body := `{"ResourceArn":"arn:aws:acm:us-east-1:000000000000:acme-endpoint/does-not-exist"}`
				rec := postACMJSON(t, h, "ListTagsForResource", body)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ResourceNotFoundException")
			},
		},
		{
			name: "MalformedArn_ValidationException",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				rec := postACMJSON(t, h, "ListTagsForResource", `{"ResourceArn":"not-an-arn"}`)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")
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

// TestACMHandler_NewStyleTagValidation_UsesValidationAndServiceQuota verifies
// that the ACME resource families (CreateAcmeEndpoint et al.) and the
// generic TagResource op reject bad tags with ValidationException/
// ServiceQuotaExceededException, not the legacy certificate-tag codes
// InvalidTagException/TooManyTagsException that AddTagsToCertificate/
// ImportCertificate/RequestCertificate declare -- gopherstack-ftkd.
func TestACMHandler_NewStyleTagValidation_UsesValidationAndServiceQuota(t *testing.T) {
	t.Parallel()

	tooManyTags := func() []map[string]string {
		tags := make([]map[string]string, 0, 51)
		for i := range 51 {
			tags = append(tags, map[string]string{"Key": fmt.Sprintf("k%d", i), "Value": "v"})
		}

		return tags
	}

	tests := []struct {
		run  func(t *testing.T, h *acm.Handler)
		name string
	}{
		{
			name: "CreateAcmeEndpoint_ReservedTagPrefix_ValidationException",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				body := `{"AuthorizationBehavior":"PRE_APPROVED",` +
					`"CertificateAuthority":{"PublicCertificateAuthority":{}},` +
					`"Tags":[{"Key":"aws:reserved","Value":"v"}]}`
				rec := postACMJSON(t, h, "CreateAcmeEndpoint", body)
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")
				assert.NotContains(t, rec.Body.String(), "InvalidTagException")
			},
		},
		{
			name: "CreateAcmeEndpoint_TooManyTags_ServiceQuotaExceededException",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				body, err := json.Marshal(map[string]any{
					"AuthorizationBehavior": "PRE_APPROVED",
					"CertificateAuthority":  map[string]any{"PublicCertificateAuthority": map[string]any{}},
					"Tags":                  tooManyTags(),
				})
				require.NoError(t, err)

				rec := postACMJSON(t, h, "CreateAcmeEndpoint", string(body))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ServiceQuotaExceededException")
				assert.NotContains(t, rec.Body.String(), "TooManyTagsException")
			},
		},
		{
			name: "TagResource_ReservedTagPrefix_ValidationException",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)
				body, err := json.Marshal(map[string]any{
					"ResourceArn": epARN,
					"Tags":        []map[string]string{{"Key": "aws:reserved", "Value": "v"}},
				})
				require.NoError(t, err)

				rec := postACMJSON(t, h, "TagResource", string(body))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")
				assert.NotContains(t, rec.Body.String(), "InvalidTagException")

				listBody, _ := json.Marshal(map[string]string{"ResourceArn": epARN})
				listRec := postACMJSON(t, h, "ListTagsForResource", string(listBody))
				require.Equal(t, http.StatusOK, listRec.Code)
				assert.JSONEq(t, `{"Tags":[]}`, listRec.Body.String(), "rejected tag must not be stored")
			},
		},
		{
			name: "TagResource_TooManyTags_ServiceQuotaExceededException",
			run: func(t *testing.T, h *acm.Handler) {
				t.Helper()

				epARN := createTestAcmeEndpoint(t, h)

				body, err := json.Marshal(map[string]any{"ResourceArn": epARN, "Tags": tooManyTags()})
				require.NoError(t, err)

				rec := postACMJSON(t, h, "TagResource", string(body))
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ServiceQuotaExceededException")
				assert.NotContains(t, rec.Body.String(), "TooManyTagsException")
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
