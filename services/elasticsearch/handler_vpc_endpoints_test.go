package elasticsearch_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/elasticsearch"
)

func TestElasticsearchHandler_CreateVpcEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body         map[string]any
		name         string
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			body: map[string]any{
				"DomainArn": "arn:aws:es:us-east-1:123456789012:domain/my-domain",
				"VpcOptions": map[string]any{
					"SecurityGroupIds": []string{"sg-12345"},
					"SubnetIds":        []string{"subnet-12345"},
				},
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"VpcEndpointId", "VpcEndpointOwner", "ACTIVE", "subnet-12345"},
		},
		{
			name:     "no_domain_arn",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid_json",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			if tt.name == "invalid_json" {
				req := httptest.NewRequest(http.MethodPost, "/2015-01-01/es/vpcEndpoints",
					strings.NewReader("not-json"))
				req.Header.Set("Content-Type", "application/json")
				rw := httptest.NewRecorder()
				h.ServeHTTP(rw, req)
				assert.Equal(t, tt.wantCode, rw.Code)

				return
			}

			resp := doRequest(t, h, http.MethodPost, "/2015-01-01/es/vpcEndpoints", tt.body)
			defer resp.Body.Close()

			assert.Equal(t, tt.wantCode, resp.StatusCode)

			if len(tt.wantContains) > 0 {
				bodyBytes, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				for _, s := range tt.wantContains {
					assert.Contains(t, string(bodyBytes), s)
				}
			}
		})
	}
}

func TestElasticsearchHandler_AuthorizeVpcEndpointAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *elasticsearch.Handler)
		name         string
		domainName   string
		account      string
		wantContains []string
		wantCode     int
	}{
		{
			name:       "success",
			domainName: "my-domain",
			account:    "111111111111",
			setup: func(t *testing.T, h *elasticsearch.Handler) {
				t.Helper()
				r := doRequest(t, h, http.MethodPost, "/2015-01-01/es/domain", map[string]any{
					"DomainName": "my-domain",
				})
				r.Body.Close()
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"AuthorizedPrincipal", "111111111111", "AWS_ACCOUNT"},
		},
		{
			name:       "domain_not_found",
			domainName: "nonexistent",
			account:    "111111111111",
			wantCode:   http.StatusNotFound,
		},
		{
			name:       "no_account",
			domainName: "my-domain",
			setup: func(t *testing.T, h *elasticsearch.Handler) {
				t.Helper()
				r := doRequest(t, h, http.MethodPost, "/2015-01-01/es/domain", map[string]any{
					"DomainName": "my-domain",
				})
				r.Body.Close()
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			if tt.setup != nil {
				tt.setup(t, h)
			}

			body := map[string]any{}
			if tt.account != "" {
				body["Account"] = tt.account
			}

			resp := doRequest(t, h, http.MethodPost,
				"/2015-01-01/es/domain/"+tt.domainName+"/authorizeVpcEndpointAccess", body)
			defer resp.Body.Close()

			assert.Equal(t, tt.wantCode, resp.StatusCode)

			if len(tt.wantContains) > 0 {
				bodyBytes, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				for _, s := range tt.wantContains {
					assert.Contains(t, string(bodyBytes), s)
				}
			}
		})
	}
}

// TestElasticsearchHandler_VpcEndpoint_CRUD drives CreateVpcEndpoint,
// DescribeVpcEndpoints (POST), and DeleteVpcEndpoint through the HTTP handler.
func TestElasticsearchHandler_VpcEndpoint_CRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	domainARN := createDomainAndGetARN(t, h, "vpcdomain")

	// CreateVpcEndpoint
	resp := doRequest(t, h, http.MethodPost, "/2015-01-01/es/vpcEndpoints", map[string]any{
		"DomainArn":  domainARN,
		"VpcOptions": map[string]any{"SubnetIds": []string{"subnet-abc"}},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body := readJSONBody(t, resp)
	epID, _ := body["VpcEndpoint"].(map[string]any)["VpcEndpointId"].(string)
	require.NotEmpty(t, epID)

	// DescribeVpcEndpoints
	resp = doRequest(t, h, http.MethodPost, "/2015-01-01/es/vpcEndpoints/describe", map[string]any{
		"VpcEndpointIds": []string{epID},
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// DeleteVpcEndpoint
	resp = doRequest(t, h, http.MethodDelete, "/2015-01-01/es/vpcEndpoints/"+epID, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestElasticsearchHandler_VpcEndpoints_Lifecycle drives the full VPC
// endpoint access/create/update/list/revoke/delete lifecycle through the HTTP
// handler.
func TestElasticsearchHandler_VpcEndpoints_Lifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	domain := "vpc-state-domain"
	domainARN := createDomainAndGetARN(t, h, domain)

	resp := doRequest(t, h, http.MethodPost,
		"/2015-01-01/es/domain/"+domain+"/authorizeVpcEndpointAccess", map[string]any{"Account": "222222222222"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = doRequest(t, h, http.MethodGet, "/2015-01-01/es/domain/"+domain+"/listVpcEndpointAccess", nil)
	require.Len(t, readJSONBody(t, resp)["AuthorizedPrincipalList"], 1)

	resp = doRequest(t, h, http.MethodPost, "/2015-01-01/es/vpcEndpoints", map[string]any{
		"DomainArn": domainARN, "VpcOptions": map[string]any{"SubnetIds": []string{"subnet-a"}},
	})
	endpointID := readJSONBody(t, resp)["VpcEndpoint"].(map[string]any)["VpcEndpointId"].(string)

	resp = doRequest(t, h, http.MethodPost, "/2015-01-01/es/vpcEndpoints/update", map[string]any{
		"VpcEndpointId": endpointID, "VpcOptions": map[string]any{"SubnetIds": []string{"subnet-b"}},
	})
	assert.Equal(
		t,
		"subnet-b",
		readJSONBody(t, resp)["VpcEndpoint"].(map[string]any)["VpcOptions"].(map[string]any)["SubnetIds"].([]any)[0],
	)

	resp = doRequest(t, h, http.MethodGet, "/2015-01-01/es/domain/"+domain+"/vpcEndpoints", nil)
	require.Len(t, readJSONBody(t, resp)["VpcEndpointSummaryList"], 1)

	resp = doRequest(t, h, http.MethodPost,
		"/2015-01-01/es/domain/"+domain+"/revokeVpcEndpointAccess", map[string]any{"Account": "222222222222"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = doRequest(t, h, http.MethodGet, "/2015-01-01/es/domain/"+domain+"/listVpcEndpointAccess", nil)
	assert.Empty(t, readJSONBody(t, resp)["AuthorizedPrincipalList"])

	resp = doRequest(t, h, http.MethodDelete, "/2015-01-01/es/vpcEndpoints/"+endpointID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

// TestElasticsearchHandler_VpcEndpointSummary_NoEndpointOrVpcOptionsLeak
// asserts the raw JSON body of ListVpcEndpoints/ListVpcEndpointsForDomain
// items and DeleteVpcEndpoint's response have neither "Endpoint" nor
// "VpcOptions" -- types.VpcEndpointSummary (elasticsearchservice@v1.45.4
// types/types.go:1911, deserializer at deserializers.go:15436) has only
// DomainArn/Status/VpcEndpointId/VpcEndpointOwner. An SDK client silently
// drops the unrecognized keys, so only a raw-body assertion catches the
// leak.
func TestElasticsearchHandler_VpcEndpointSummary_NoEndpointOrVpcOptionsLeak(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	domain := "vpc-summary-leak-domain"
	domainARN := createDomainAndGetARN(t, h, domain)

	resp := doRequest(t, h, http.MethodPost, "/2015-01-01/es/vpcEndpoints", map[string]any{
		"DomainArn": domainARN, "VpcOptions": map[string]any{"SubnetIds": []string{"subnet-a"}},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	endpointID, _ := readJSONBody(t, resp)["VpcEndpoint"].(map[string]any)["VpcEndpointId"].(string)
	require.NotEmpty(t, endpointID)

	assertNoLeak := func(t *testing.T, item map[string]any) {
		t.Helper()
		_, hasEndpoint := item["Endpoint"]
		assert.False(
			t,
			hasEndpoint,
			"VpcEndpointSummary leaked Endpoint; real types.VpcEndpointSummary has no such member",
		)
		_, hasVpcOptions := item["VpcOptions"]
		assert.False(
			t,
			hasVpcOptions,
			"VpcEndpointSummary leaked VpcOptions; real types.VpcEndpointSummary has no such member",
		)
		assert.Contains(t, item, "VpcEndpointId", "VpcEndpointSummary must still emit VpcEndpointId")
		assert.Contains(t, item, "Status", "VpcEndpointSummary must still emit Status")
	}

	resp = doRequest(t, h, http.MethodGet, "/2015-01-01/es/vpcEndpoints", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	listBody := readJSONBody(t, resp)
	listItems, _ := listBody["VpcEndpointSummaryList"].([]any)
	require.Len(t, listItems, 1)
	assertNoLeak(t, listItems[0].(map[string]any))

	resp = doRequest(t, h, http.MethodGet, "/2015-01-01/es/domain/"+domain+"/vpcEndpoints", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	listForDomainBody := readJSONBody(t, resp)
	listForDomainItems, _ := listForDomainBody["VpcEndpointSummaryList"].([]any)
	require.Len(t, listForDomainItems, 1)
	assertNoLeak(t, listForDomainItems[0].(map[string]any))

	resp = doRequest(t, h, http.MethodDelete, "/2015-01-01/es/vpcEndpoints/"+endpointID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	deleteBody := readJSONBody(t, resp)
	deleteSummary, _ := deleteBody["VpcEndpointSummary"].(map[string]any)
	require.NotEmpty(t, deleteSummary)
	assertNoLeak(t, deleteSummary)
}

// TestElasticsearchHandler_VpcEndpointStatusActive verifies VPC endpoints are
// created with ACTIVE status.
func TestElasticsearchHandler_VpcEndpointStatusActive(t *testing.T) {
	t.Parallel()

	b := elasticsearch.NewInMemoryBackend("123456789012", "us-east-1")

	ep, err := b.CreateVpcEndpoint(
		context.Background(), "arn:aws:es:us-east-1:123456789012:domain/test",
		elasticsearch.VPCOptions{SubnetIDs: []string{"subnet-1"}},
	)
	require.NoError(t, err)
	assert.Equal(t, "ACTIVE", ep.Status)
}

// TestElasticsearchHandler_VpcOptionsDeepCopy verifies CreateVpcEndpoint
// deep-copies VpcOptions.
func TestElasticsearchHandler_VpcOptionsDeepCopy(t *testing.T) {
	t.Parallel()

	b := elasticsearch.NewInMemoryBackend("123456789012", "us-east-1")
	opts := elasticsearch.VPCOptions{SubnetIDs: []string{"subnet-1"}}

	ep, err := b.CreateVpcEndpoint(context.Background(), "arn:aws:es:us-east-1:123456789012:domain/test", opts)
	require.NoError(t, err)

	// Mutating the original opts must not affect the returned endpoint.
	opts.SubnetIDs[0] = "subnet-mutated"
	assert.Equal(t, "subnet-1", ep.VpcOptions.SubnetIDs[0])
}

// TestElasticsearchHandler_VpcEndpointValidation verifies empty DomainARN
// returns ErrValidation.
func TestElasticsearchHandler_VpcEndpointValidation(t *testing.T) {
	t.Parallel()

	b := elasticsearch.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.CreateVpcEndpoint(context.Background(), "", elasticsearch.VPCOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, elasticsearch.ErrValidation)
}

// TestElasticsearchHandler_AuthorizeVpcEndpointAccessValidation verifies
// empty account returns ErrValidation.
func TestElasticsearchHandler_AuthorizeVpcEndpointAccessValidation(t *testing.T) {
	t.Parallel()

	b := elasticsearch.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.CreateDomain(
		context.Background(), elasticsearch.CreateDomainInput{Name: "vpc-auth-dom"},
	)
	require.NoError(t, err)

	err = b.AuthorizeVpcEndpointAccess(context.Background(), "vpc-auth-dom", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, elasticsearch.ErrValidation)
}

// TestElasticsearchHandler_DeleteDomain_ClearsVpcAccess verifies that
// DeleteDomain clears the domain's VPC endpoint access authorizations.
// Otherwise a new domain created with the same (user-chosen, reusable) name
// inherits the deleted domain's authorized-account list -- an
// access-control artefact, not merely a leak.
func TestElasticsearchHandler_DeleteDomain_ClearsVpcAccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := elasticsearch.NewInMemoryBackend("123456789012", "us-east-1")

	_, err := b.CreateDomain(ctx, elasticsearch.CreateDomainInput{Name: "reused-domain"})
	require.NoError(t, err)

	err = b.AuthorizeVpcEndpointAccess(ctx, "reused-domain", "999988887777")
	require.NoError(t, err)

	access, err := b.ListVpcEndpointAccess(ctx, "reused-domain")
	require.NoError(t, err)
	require.NotEmpty(t, access)

	_, err = b.DeleteDomain(ctx, "reused-domain")
	require.NoError(t, err)

	_, err = b.CreateDomain(ctx, elasticsearch.CreateDomainInput{Name: "reused-domain"})
	require.NoError(t, err)

	access, err = b.ListVpcEndpointAccess(ctx, "reused-domain")
	require.NoError(t, err)
	assert.Empty(t, access,
		"recreated domain must not inherit the deleted domain's VPC endpoint access authorizations")
}
