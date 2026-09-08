package cloudfront_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cfsdk "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudfront"
)

// TestGetManagedCertificateDetails_NotFound verifies a 404 for a distribution tenant
// that does not exist (rather than the previous hardcoded always-SUCCESS stub).
func TestGetManagedCertificateDetails_NotFound(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	const prefix = "/2020-05-31/"

	// Real GetManagedCertificateDetails is GET /2020-05-31/managed-certificate/{Identifier}
	// (cloudfront@v1.67.4 serializers.go: awsRestxml_serializeOpGetManagedCertificateDetails's
	// SplitURI), not nested under distribution-tenant.
	rec := doXML(t, h, http.MethodGet, prefix+"managed-certificate/does-not-exist", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "EntityNotFound")
}

// TestGetManagedCertificateDetails_StableACrossCalls verifies the derived
// certificate ARN and validation tokens are stable across repeated Get calls for the same
// tenant (a real, cached backend value, not a fresh random result each time).
func TestGetManagedCertificateDetails_StableACrossCalls(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t)
	const prefix = "/2020-05-31/"

	createBody := `<CreateDistributionTenantRequest>` +
		`<DistributionId>dist-parity-001</DistributionId>` +
		`<Domain>parity-cert-test.example.com</Domain>` +
		`</CreateDistributionTenantRequest>`
	createRec := doXML(t, h, http.MethodPost, prefix+"distribution-tenant", []byte(createBody))
	require.Equal(t, http.StatusCreated, createRec.Code)
	tenantID := extractXMLID(t, createRec.Body.String())

	path := prefix + "managed-certificate/" + tenantID
	first := doXML(t, h, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, first.Code)
	require.Contains(t, first.Body.String(), "SUCCESS")

	second := doXML(t, h, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, second.Code)

	firstARN, _, _ := strings.Cut(strings.SplitN(first.Body.String(), "<CertificateArn>", 2)[1], "</CertificateArn>")
	secondARN, _, _ := strings.Cut(strings.SplitN(second.Body.String(), "<CertificateArn>", 2)[1], "</CertificateArn>")
	assert.Equal(t, firstARN, secondARN)
	assert.Contains(t, first.Body.String(), "parity-cert-test.example.com")
}

// TestCreateDistributionTenant_RequiresDistributionId checks that
// an empty DistributionId is rejected with 400 InvalidArgument.
func TestCreateDistributionTenant_RequiresDistributionId(t *testing.T) {
	t.Parallel()

	const prefix = "/2020-05-31/"

	tests := []struct {
		name     string
		body     string
		wantErr  string
		wantCode int
	}{
		{
			name: "missing_distribution_id",
			body: `<CreateDistributionTenantRequest>
				<Domain>tenant-test.com</Domain>
			</CreateDistributionTenantRequest>`,
			wantCode: http.StatusBadRequest,
			wantErr:  "InvalidArgument",
		},
		{
			name: "empty_distribution_id",
			body: `<CreateDistributionTenantRequest>
				<DistributionId></DistributionId>
				<Domain>tenant-test2.com</Domain>
			</CreateDistributionTenantRequest>`,
			wantCode: http.StatusBadRequest,
			wantErr:  "InvalidArgument",
		},
		{
			name: "valid_with_distribution_id",
			body: `<CreateDistributionTenantRequest>
				<DistributionId>dist-xyz</DistributionId>
				<Domain>tenant-valid.com</Domain>
			</CreateDistributionTenantRequest>`,
			wantCode: http.StatusCreated,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newCFHandler(t)
			rr := cfRequest(t, h, http.MethodPost, prefix+"distribution-tenant", tc.body)
			if rr.Code != tc.wantCode {
				t.Errorf("got %d want %d: %s", rr.Code, tc.wantCode, rr.Body.String())
			}
			if tc.wantErr != "" && !strings.Contains(rr.Body.String(), tc.wantErr) {
				t.Errorf("want %q in body, got: %s", tc.wantErr, rr.Body.String())
			}
		})
	}
}

// TestUpdateDistributionTenant_RequiresIfMatch verifies that
// UpdateDistributionTenant returns 412 when If-Match is absent or stale.
func TestUpdateDistributionTenant_RequiresIfMatch(t *testing.T) {
	t.Parallel()

	const prefix = "/2020-05-31/"

	tests := []struct {
		name     string
		ifMatch  string
		wantErr  string
		wantCode int
	}{
		{
			name:     "missing_if_match",
			ifMatch:  "",
			wantCode: http.StatusPreconditionFailed,
			wantErr:  "PreconditionFailed",
		},
		{
			name:     "wrong_etag",
			ifMatch:  "wrong-etag",
			wantCode: http.StatusPreconditionFailed,
			wantErr:  "PreconditionFailed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newCFHandler(t)
			createRR := cfRequest(t, h, http.MethodPost, prefix+"distribution-tenant",
				`<CreateDistributionTenantRequest>`+
					`<DistributionId>dist-001</DistributionId>`+
					`<Domain>update-match-`+tc.name+`.com</Domain>`+
					`</CreateDistributionTenantRequest>`)
			if createRR.Code != http.StatusCreated {
				t.Fatalf("create got %d: %s", createRR.Code, createRR.Body.String())
			}
			tenantID := extractXMLID(t, createRR.Body.String())

			var headers map[string]string
			if tc.ifMatch != "" {
				headers = map[string]string{"If-Match": tc.ifMatch}
			}
			rr := cfRequestWithHeader(t, h, http.MethodPut, prefix+"distribution-tenant/"+tenantID, headers)
			if rr.Code != tc.wantCode {
				t.Errorf("got %d want %d: %s", rr.Code, tc.wantCode, rr.Body.String())
			}
			if tc.wantErr != "" && !strings.Contains(rr.Body.String(), tc.wantErr) {
				t.Errorf("want %q in body, got: %s", tc.wantErr, rr.Body.String())
			}
		})
	}
}

// TestDeleteDistributionTenant_RequiresIfMatch verifies that
// DeleteDistributionTenant returns 412 when If-Match is absent or stale.
func TestDeleteDistributionTenant_RequiresIfMatch(t *testing.T) {
	t.Parallel()

	const prefix = "/2020-05-31/"

	tests := []struct {
		name     string
		ifMatch  string
		wantErr  string
		wantCode int
	}{
		{
			name:     "missing_if_match",
			ifMatch:  "",
			wantCode: http.StatusPreconditionFailed,
			wantErr:  "PreconditionFailed",
		},
		{
			name:     "wrong_etag",
			ifMatch:  "bad-etag-value",
			wantCode: http.StatusPreconditionFailed,
			wantErr:  "PreconditionFailed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newCFHandler(t)
			createRR := cfRequest(t, h, http.MethodPost, prefix+"distribution-tenant",
				`<CreateDistributionTenantRequest>`+
					`<DistributionId>dist-001</DistributionId>`+
					`<Domain>delete-match-`+tc.name+`.com</Domain>`+
					`</CreateDistributionTenantRequest>`)
			if createRR.Code != http.StatusCreated {
				t.Fatalf("create got %d: %s", createRR.Code, createRR.Body.String())
			}
			tenantID := extractXMLID(t, createRR.Body.String())

			var headers map[string]string
			if tc.ifMatch != "" {
				headers = map[string]string{"If-Match": tc.ifMatch}
			}
			rr := cfRequestWithHeader(t, h, http.MethodDelete, prefix+"distribution-tenant/"+tenantID, headers)
			if rr.Code != tc.wantCode {
				t.Errorf("got %d want %d: %s", rr.Code, tc.wantCode, rr.Body.String())
			}
			if tc.wantErr != "" && !strings.Contains(rr.Body.String(), tc.wantErr) {
				t.Errorf("want %q in body, got: %s", tc.wantErr, rr.Body.String())
			}
		})
	}
}

// TestDistributionTenantCRUD tests create, get, update, list, delete for DistributionTenant.
func TestDistributionTenantCRUD(t *testing.T) {
	t.Parallel()
	h := newCFHandler(t)
	const prefix = "/2020-05-31/"

	// Create distribution tenant
	createBody := `<CreateDistributionTenantRequest>
		<DistributionId>dist-001</DistributionId>
		<Domain>example.com</Domain>
	</CreateDistributionTenantRequest>`
	createRR := cfRequest(t, h, http.MethodPost, prefix+"distribution-tenant", createBody)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 on create, got %d: %s", createRR.Code, createRR.Body.String())
	}
	resp := createRR.Body.String()
	if !strings.Contains(resp, "DistributionTenant") {
		t.Fatalf("expected DistributionTenant in response, got: %s", resp)
	}
	if !strings.Contains(resp, "example.com") {
		t.Fatalf("expected domain in response, got: %s", resp)
	}

	tenantID := extractXMLID(t, resp)
	if tenantID == "" {
		t.Fatal("expected non-empty tenant ID")
	}
	etag := createRR.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected ETag header from create")
	}

	// Get tenant by ID
	getResp := cfOK(t, h, http.MethodGet, prefix+"distribution-tenant/"+tenantID, "")
	if !strings.Contains(getResp, tenantID) {
		t.Errorf("get response missing tenant ID: %s", getResp)
	}
	if !strings.Contains(getResp, "example.com") {
		t.Errorf("get response missing domain: %s", getResp)
	}

	// List tenants is POST to the plural "distribution-tenants" path (cloudfront@v1.67.4
	// serializers.go: awsRestxml_serializeOpListDistributionTenants's SplitURI); the bare
	// singular GET is GetDistributionTenantByDomain instead.
	listResp := cfOK(t, h, http.MethodPost, prefix+"distribution-tenants", "")
	if !strings.Contains(listResp, "DistributionTenantList") {
		t.Errorf("expected DistributionTenantList, got: %s", listResp)
	}
	if !strings.Contains(listResp, tenantID) {
		t.Errorf("list missing created tenant: %s", listResp)
	}

	// Update tenant requires If-Match ETag.
	updateRR := cfRequestWithHeader(t, h, http.MethodPut, prefix+"distribution-tenant/"+tenantID,
		map[string]string{"If-Match": etag})
	if updateRR.Code != http.StatusOK {
		t.Errorf("expected 200 on update, got %d: %s", updateRR.Code, updateRR.Body.String())
	}
	if !strings.Contains(updateRR.Body.String(), "DistributionTenant") {
		t.Errorf("update response missing DistributionTenant: %s", updateRR.Body.String())
	}
	// Refresh ETag after update.
	etag = updateRR.Header().Get("ETag")

	// Delete tenant requires If-Match ETag.
	rr := cfRequestWithHeader(t, h, http.MethodDelete, prefix+"distribution-tenant/"+tenantID,
		map[string]string{"If-Match": etag})
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 on delete, got %d: %s", rr.Code, rr.Body.String())
	}

	// List should be empty after delete.
	listAfter := cfOK(t, h, http.MethodPost, prefix+"distribution-tenants", "")
	if strings.Contains(listAfter, tenantID) {
		t.Errorf("deleted tenant still in list: %s", listAfter)
	}
}

// TestDistributionTenantByDomain tests GetDistributionTenantByDomain.
func TestDistributionTenantByDomain(t *testing.T) {
	t.Parallel()
	h := newCFHandler(t)
	const prefix = "/2020-05-31/"

	// Create tenant
	createBody := `<CreateDistributionTenantRequest>
		<DistributionId>dist-001</DistributionId>
		<Domain>mysite.com</Domain>
	</CreateDistributionTenantRequest>`
	cfOK(t, h, http.MethodPost, prefix+"distribution-tenant", createBody)

	// Get by domain is the bare GET "distribution-tenant" (Domain travels as a
	// "?domain=" query value; cloudfront@v1.67.4 serializers.go:
	// awsRestxml_serializeOpGetDistributionTenantByDomain's SplitURI).
	resp := cfOK(t, h, http.MethodGet, prefix+"distribution-tenant?domain=mysite.com", "")
	if !strings.Contains(resp, "mysite.com") {
		t.Errorf("expected domain in response, got: %s", resp)
	}
}

// TestDistributionTenantInvalidation tests create/list invalidations for a tenant.
func TestDistributionTenantInvalidation(t *testing.T) {
	t.Parallel()
	h := newCFHandler(t)
	const prefix = "/2020-05-31/"

	// Create tenant
	createBody := `<CreateDistributionTenantRequest>
		<DistributionId>dist-001</DistributionId>
		<Domain>inv-test.com</Domain>
	</CreateDistributionTenantRequest>`
	tenantResp := cfOK(t, h, http.MethodPost, prefix+"distribution-tenant", createBody)
	tenantID := extractXMLID(t, tenantResp)
	if tenantID == "" {
		t.Fatal("no tenant ID in response")
	}

	// Create invalidation
	invBody := `<InvalidationBatch>` +
		`<CallerReference>ref1</CallerReference>` +
		`<Paths><Quantity>1</Quantity><Items><Path>/*</Path></Items></Paths>` +
		`</InvalidationBatch>`
	invResp := cfOK(t, h, http.MethodPost, prefix+"distribution-tenant/"+tenantID+"/invalidation", invBody)
	if !strings.Contains(invResp, "Invalidation") {
		t.Fatalf("expected Invalidation in response, got: %s", invResp)
	}
	invID := extractXMLID(t, invResp)
	if invID == "" {
		t.Fatal("no invalidation ID in response")
	}

	// List invalidations
	listResp := cfOK(t, h, http.MethodGet, prefix+"distribution-tenant/"+tenantID+"/invalidation", "")
	if !strings.Contains(listResp, "InvalidationList") {
		t.Errorf("expected InvalidationList, got: %s", listResp)
	}

	// Get specific invalidation
	getResp := cfOK(t, h, http.MethodGet, prefix+"distribution-tenant/"+tenantID+"/invalidation/"+invID, "")
	if !strings.Contains(getResp, invID) {
		t.Errorf("expected invalidation ID in response, got: %s", getResp)
	}
}

// TestGetManagedCertificateDetails tests the managed cert details endpoint.
func TestGetManagedCertificateDetails(t *testing.T) {
	t.Parallel()
	h := newCFHandler(t)
	const prefix = "/2020-05-31/"

	// Create tenant first
	createBody := `<CreateDistributionTenantRequest>
		<DistributionId>dist-001</DistributionId>
		<Domain>cert-test.com</Domain>
	</CreateDistributionTenantRequest>`
	tenantResp := cfOK(t, h, http.MethodPost, prefix+"distribution-tenant", createBody)
	tenantID := extractXMLID(t, tenantResp)

	// Get managed certificate details
	resp := cfOK(t, h, http.MethodGet, prefix+"managed-certificate/"+tenantID, "")
	if !strings.Contains(resp, "ManagedCertificateDetails") {
		t.Errorf("expected ManagedCertificateDetails, got: %s", resp)
	}
	if !strings.Contains(resp, "SUCCESS") {
		t.Errorf("expected SUCCESS status, got: %s", resp)
	}
}

// TestVerifyDNSConfiguration tests the DNS verification endpoint.
func TestVerifyDNSConfiguration(t *testing.T) {
	t.Parallel()
	h := newCFHandler(t)
	const prefix = "/2020-05-31/"

	resp := cfOK(t, h, http.MethodPost, prefix+"verify-dns-configuration", "")
	if !strings.Contains(resp, "VerifyDnsConfigurationResponse") {
		t.Errorf("expected VerifyDnsConfigurationResponse, got: %s", resp)
	}
	if !strings.Contains(resp, "PASSED") {
		t.Errorf("expected PASSED, got: %s", resp)
	}
}

// ---------------------------------------------------------------------------
// Table-driven tests for formerly-stubbed operations with real state
// ---------------------------------------------------------------------------

func doTenantReq(
	t *testing.T,
	h *cloudfront.Handler,
	method, path string,
) *httptest.ResponseRecorder {
	t.Helper()

	return cfRequest(t, h, method, path, "")
}

// TestGetManagedCertificateDetails_TableDriven validates cert details with real tenant state.
func TestGetManagedCertificateDetails_TableDriven(t *testing.T) {
	t.Parallel()

	const prefix = "/2020-05-31/"

	makeTenant := func(h *cloudfront.Handler, domain string) string {
		const tenantTemplate = `<CreateDistributionTenantRequest>` +
			`<DistributionId>d-001</DistributionId>` +
			`<Domain>%s</Domain>` +
			`</CreateDistributionTenantRequest>`
		body := fmt.Sprintf(tenantTemplate, domain)
		resp := cfOK(t, h, http.MethodPost, prefix+"distribution-tenant", body)

		return extractXMLID(t, resp)
	}

	tests := []struct {
		setup    func(h *cloudfront.Handler) string
		name     string
		wantBody []string
		wantCode int
	}{
		{
			name: "existing_tenant_returns_success_cert",
			setup: func(h *cloudfront.Handler) string {
				return makeTenant(h, "example.com")
			},
			wantCode: http.StatusOK,
			wantBody: []string{"ManagedCertificateDetails", "SUCCESS", "example.com"},
		},
		{
			name: "non_existent_tenant_returns_404",
			setup: func(_ *cloudfront.Handler) string {
				return "no-such-tenant"
			},
			wantCode: http.StatusNotFound,
			wantBody: []string{"EntityNotFound"},
		},
		{
			name: "tenant_domain_appears_in_validation_tokens",
			setup: func(h *cloudfront.Handler) string {
				return makeTenant(h, "my-service.example.com")
			},
			wantCode: http.StatusOK,
			wantBody: []string{"my-service.example.com", "ValidationTokenDetails"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := cloudfront.NewHandler(newTestBackend(t))
			tenantID := tt.setup(h)
			certPath := prefix + "managed-certificate/" + tenantID
			rec := doTenantReq(t, h, http.MethodGet, certPath)

			assert.Equal(t, tt.wantCode, rec.Code)
			for _, want := range tt.wantBody {
				assert.Contains(t, rec.Body.String(), want)
			}
		})
	}
}

// TestListDomainConflicts_TableDriven validates domain conflict detection with real state,
// including the required DomainControlValidationResource scoping (gopherstack-3izo): real AWS
// excludes that resource from its own conflict list, and rejects the request outright when
// either required member (Domain, DomainControlValidationResource) is missing or when the
// referenced resource does not exist.
func TestListDomainConflicts_TableDriven(t *testing.T) {
	t.Parallel()

	const prefix = "/2020-05-31/"

	tests := []struct {
		setup    func(b *cloudfront.InMemoryBackend) (domain, distID, tenantID string)
		name     string
		bodyOvr  string // overrides the built body when non-empty; used for malformed-request cases
		wantBody []string
		wantNot  []string
		wantCode int
	}{
		{
			name: "no_conflicts_returns_empty_list",
			setup: func(b *cloudfront.InMemoryBackend) (string, string, string) {
				dist, err := b.CreateDistribution("ref-scope", "test", true, nil)
				require.NoError(t, err)

				return "nonexistent.example.com", dist.ID, ""
			},
			wantCode: http.StatusOK,
			wantBody: []string{"DomainConflictList", "<DomainConflicts></DomainConflicts>"},
		},
		{
			name: "conflict_via_distribution_alias",
			setup: func(b *cloudfront.InMemoryBackend) (string, string, string) {
				dist, err := b.CreateDistribution("ref-1", "test", true, nil)
				require.NoError(t, err)
				err = b.AssociateAlias(dist.ID, "conflict.example.com")
				require.NoError(t, err)

				scope, err := b.CreateDistribution("ref-1-scope", "test", true, nil)
				require.NoError(t, err)

				return "conflict.example.com", scope.ID, ""
			},
			wantCode: http.StatusOK,
			wantBody: []string{"DomainConflictList", "conflict.example.com"},
			wantNot:  []string{"<DomainConflicts></DomainConflicts>"},
		},
		{
			name: "conflict_via_distribution_tenant_domain",
			setup: func(b *cloudfront.InMemoryBackend) (string, string, string) {
				dist, err := b.CreateDistribution("ref-2", "test", true, nil)
				require.NoError(t, err)
				_, err = b.CreateDistributionTenant(
					dist.ID, "tenant-domain-tenant", []string{"tenant-domain.example.com"}, nil,
				)
				require.NoError(t, err)

				scope, err := b.CreateDistribution("ref-2-scope", "test", true, nil)
				require.NoError(t, err)

				return "tenant-domain.example.com", scope.ID, ""
			},
			wantCode: http.StatusOK,
			wantBody: []string{"DomainConflictList", "tenant-domain.example.com"},
			wantNot:  []string{"<DomainConflicts></DomainConflicts>"},
		},
		{
			name: "scoping_to_the_claiming_distribution_excludes_it",
			setup: func(b *cloudfront.InMemoryBackend) (string, string, string) {
				dist, err := b.CreateDistribution("ref-3", "test", true, nil)
				require.NoError(t, err)
				err = b.AssociateAlias(dist.ID, "self-scoped.example.com")
				require.NoError(t, err)

				return "self-scoped.example.com", dist.ID, ""
			},
			wantCode: http.StatusOK,
			wantBody: []string{"DomainConflictList", "<DomainConflicts></DomainConflicts>"},
		},
		{
			name: "scoping_to_the_claiming_tenant_excludes_it",
			setup: func(b *cloudfront.InMemoryBackend) (string, string, string) {
				dist, err := b.CreateDistribution("ref-4", "test", true, nil)
				require.NoError(t, err)
				tenant, err := b.CreateDistributionTenant(
					dist.ID, "self-scoped-tenant", []string{"self-scoped-tenant.example.com"}, nil,
				)
				require.NoError(t, err)

				return "self-scoped-tenant.example.com", "", tenant.ID
			},
			wantCode: http.StatusOK,
			wantBody: []string{"DomainConflictList", "<DomainConflicts></DomainConflicts>"},
		},
		{
			name: "missing_domain_rejected",
			bodyOvr: `<ListDomainConflictsRequest><DomainControlValidationResource>` +
				`<DistributionId>d-x</DistributionId></DomainControlValidationResource>` +
				`</ListDomainConflictsRequest>`,
			wantCode: http.StatusBadRequest,
			wantBody: []string{"InvalidArgument"},
		},
		{
			name:     "missing_validation_resource_rejected",
			bodyOvr:  `<ListDomainConflictsRequest><Domain>whatever.example.com</Domain></ListDomainConflictsRequest>`,
			wantCode: http.StatusBadRequest,
			wantBody: []string{"InvalidArgument"},
		},
		{
			name: "both_distribution_and_tenant_id_rejected",
			bodyOvr: `<ListDomainConflictsRequest><Domain>whatever.example.com</Domain>` +
				`<DomainControlValidationResource><DistributionId>d-x</DistributionId>` +
				`<DistributionTenantId>dt-x</DistributionTenantId></DomainControlValidationResource>` +
				`</ListDomainConflictsRequest>`,
			wantCode: http.StatusBadRequest,
			wantBody: []string{"InvalidArgument"},
		},
		{
			name:     "unknown_validation_resource_not_found",
			bodyOvr:  listDomainConflictsBody("whatever.example.com", "does-not-exist", ""),
			wantCode: http.StatusNotFound,
			wantBody: []string{"EntityNotFound"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newTestBackend(t)

			body := tt.bodyOvr
			if tt.setup != nil {
				domain, distID, tenantID := tt.setup(b)
				body = listDomainConflictsBody(domain, distID, tenantID)
			}

			h := cloudfront.NewHandler(b)

			rec := cfRequest(t, h, http.MethodPost, prefix+"domain-conflicts", body)
			assert.Equal(t, tt.wantCode, rec.Code, rec.Body.String())
			for _, want := range tt.wantBody {
				assert.Contains(t, rec.Body.String(), want)
			}
			for _, notWant := range tt.wantNot {
				assert.NotContains(t, rec.Body.String(), notWant)
			}
		})
	}
}

// TestAssociateDistributionTenantWebACL covers the AssociateDistributionTenantWebACL
// operation. Regression coverage for gopherstack-4ara: the request bodies below use the
// real AssociateDistributionTenantWebACLRequest>WebACLArn shape (cloudfront@v1.67.4
// serializers.go: awsRestxml_serializeOpDocumentAssociateDistributionTenantWebACLInput),
// not the previous invented WebACLAssociation>WebACLId shape this test used to send --
// that invented body happened to match the pre-fix handler's (also wrong) expectation,
// so this test passed even though every real client's request 400'd MalformedXML.
func TestAssociateDistributionTenantWebACL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check         func(*testing.T, *httptest.ResponseRecorder)
		name          string
		tenantID      string
		body          []byte
		wantStatus    int
		useRealTenant bool
	}{
		{
			name: "associate_tenant_web_acl_success",
			body: []byte(
				`<AssociateDistributionTenantWebACLRequest>` +
					`<WebACLArn>arn:aws:wafv2:us-east-1:123:global/webacl/tenant/abc</WebACLArn>` +
					`</AssociateDistributionTenantWebACLRequest>`,
			),
			wantStatus:    http.StatusOK,
			useRealTenant: true,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				assert.NotEmpty(t, rec.Header().Get("ETag"))
				assert.Contains(t, rec.Body.String(), "arn:aws:wafv2:us-east-1:123:global/webacl/tenant/abc")
			},
		},
		{
			name:     "associate_tenant_web_acl_empty_tenant",
			tenantID: "",
			body: []byte(
				`<AssociateDistributionTenantWebACLRequest>` +
					`<WebACLArn>some-acl</WebACLArn>` +
					`</AssociateDistributionTenantWebACLRequest>`,
			),
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()
				assert.Contains(t, rec.Body.String(), "InvalidArgument")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			tenantID := tt.tenantID
			if tt.useRealTenant {
				tenantID = createTestTenant(t, h, "dist-assoc-tenant-webacl", "assoc-tenant-webacl.example.com")
			}

			var path string
			if tenantID == "" {
				// POST to the tenant endpoint with empty tenant ID segment
				path = "/2020-05-31/distribution-tenant//associate-web-acl"
			} else {
				path = "/2020-05-31/distribution-tenant/" + tenantID + "/associate-web-acl"
			}

			rec := doXML(t, h, http.MethodPut, path, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
			tt.check(t, rec)
		})
	}
}

// TestAssociateDistributionTenantWebACL_RealClient drives AssociateDistributionTenantWebACL
// through the real aws-sdk-go-v2 CloudFront client (gopherstack-4ara), the only check that
// cannot be fooled by a hand-typed body encoding the same wrong assumption as the handler.
// Fails against the pre-fix root/field shape (confirmed by hand-reverting).
func TestAssociateDistributionTenantWebACL_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	const prefix = "/2020-05-31/"

	createBody := `<CreateDistributionTenantRequest>` +
		`<DistributionId>dist-4ara-001</DistributionId>` +
		`<Domain>gopherstack-4ara.example.com</Domain>` +
		`</CreateDistributionTenantRequest>`
	createRec := doXML(t, h, http.MethodPost, prefix+"distribution-tenant", []byte(createBody))
	require.Equal(t, http.StatusCreated, createRec.Code, createRec.Body.String())
	tenantID := extractXMLID(t, createRec.Body.String())

	client := newTestCloudFrontClient(t, h)
	const webACLArn = "arn:aws:wafv2:us-east-1:123456789012:global/webacl/real-client-acl/abc123"

	_, err := client.AssociateDistributionTenantWebACL(t.Context(), &cfsdk.AssociateDistributionTenantWebACLInput{
		Id:        aws.String(tenantID),
		WebACLArn: aws.String(webACLArn),
	})
	require.NoError(t, err)

	getRec := doXML(t, h, http.MethodGet, prefix+"distribution-tenant/"+tenantID, nil)
	require.Equal(t, http.StatusOK, getRec.Code)
	assert.Contains(t, getRec.Body.String(), webACLArn)
}

// TestListDistributionTenants_Domains_RealClient drives ListDistributionTenants through the
// real aws-sdk-go-v2 CloudFront client and asserts DistributionTenantSummary.Domains is
// populated. The real deserializer (awsRestxml_deserializeDocumentDistributionTenantSummary,
// case "Domains") reads a <Domains><member><Domain>.../><Status>.../</member></Domains> list;
// a flat Domain field on tenantSummaryXML (the pre-fix shape, confirmed by hand-reverting)
// decodes to an always-empty Domains slice with the right item COUNT but blank content, even
// though the singular GetDistributionTenant path already emitted the list correctly.
func TestListDistributionTenants_Domains_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	const prefix = "/2020-05-31/"

	createBody := `<CreateDistributionTenantRequest>` +
		`<DistributionId>dist-domains-001</DistributionId>` +
		`<Domain>list-domains.example.com</Domain>` +
		`</CreateDistributionTenantRequest>`
	createRec := doXML(t, h, http.MethodPost, prefix+"distribution-tenant", []byte(createBody))
	require.Equal(t, http.StatusCreated, createRec.Code, createRec.Body.String())

	client := newTestCloudFrontClient(t, h)

	listed, err := client.ListDistributionTenants(t.Context(), &cfsdk.ListDistributionTenantsInput{})
	require.NoError(t, err)
	require.Len(t, listed.DistributionTenantList, 1)
	require.Len(t, listed.DistributionTenantList[0].Domains, 1)
	assert.Equal(t, "list-domains.example.com", aws.ToString(listed.DistributionTenantList[0].Domains[0].Domain))
}
