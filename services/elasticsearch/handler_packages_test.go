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

func TestElasticsearchHandler_CreatePackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *elasticsearch.Handler)
		name         string
		packageName  string
		packageType  string
		wantContains []string
		omitSource   bool
		wantCode     int
	}{
		{
			name:         "success",
			packageName:  "my-dict",
			packageType:  "TXT-DICTIONARY",
			wantCode:     http.StatusOK,
			wantContains: []string{"PackageID", "my-dict", "TXT-DICTIONARY", "AVAILABLE"},
		},
		{
			name:        "no_name",
			packageType: "TXT-DICTIONARY",
			wantCode:    http.StatusBadRequest,
		},
		{
			name:        "duplicate_name",
			packageName: "dup-pkg",
			packageType: "TXT-DICTIONARY",
			setup: func(t *testing.T, h *elasticsearch.Handler) {
				t.Helper()
				r := doRequest(t, h, http.MethodPost, "/2015-01-01/packages", map[string]any{
					"PackageName":   "dup-pkg",
					"PackageType":   "TXT-DICTIONARY",
					"PackageSource": map[string]any{"S3BucketName": "b", "S3Key": "k"},
				})
				r.Body.Close()
			},
			wantCode: http.StatusConflict,
		},
		{
			name:     "invalid_json",
			wantCode: http.StatusBadRequest,
		},
		{
			name:        "missing_package_source",
			packageName: "no-source-pkg",
			packageType: "TXT-DICTIONARY",
			omitSource:  true,
			wantCode:    http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			if tt.setup != nil {
				tt.setup(t, h)
			}

			if tt.name == "invalid_json" {
				req := httptest.NewRequest(http.MethodPost, "/2015-01-01/packages", strings.NewReader("not-json"))
				req.Header.Set("Content-Type", "application/json")
				rw := httptest.NewRecorder()
				h.ServeHTTP(rw, req)
				assert.Equal(t, tt.wantCode, rw.Code)

				return
			}

			body := map[string]any{}
			if tt.packageName != "" {
				body["PackageName"] = tt.packageName
			}

			if tt.packageType != "" {
				body["PackageType"] = tt.packageType
			}

			if !tt.omitSource {
				body["PackageSource"] = map[string]any{"S3BucketName": "test-bucket", "S3Key": "test-key"}
			}

			resp := doRequest(t, h, http.MethodPost, "/2015-01-01/packages", body)
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

func TestElasticsearchHandler_AssociatePackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setup        func(t *testing.T, h *elasticsearch.Handler) (packageID, domainName string)
		wantContains []string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *elasticsearch.Handler) (string, string) {
				t.Helper()

				pkgID := createTestPackage(t, h, "assoc-dict")

				domResp := doRequest(t, h, http.MethodPost, "/2015-01-01/es/domain", map[string]any{
					"DomainName": "assoc-domain",
				})
				domResp.Body.Close()

				return pkgID, "assoc-domain"
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"DomainPackageDetails", "ACTIVE"},
		},
		{
			name: "package_not_found",
			setup: func(t *testing.T, h *elasticsearch.Handler) (string, string) {
				t.Helper()

				domResp := doRequest(t, h, http.MethodPost, "/2015-01-01/es/domain", map[string]any{
					"DomainName": "another-domain",
				})
				domResp.Body.Close()

				return "nonexistent-pkg", "another-domain"
			},
			wantCode: http.StatusNotFound,
		},
		{
			name: "duplicate_association_conflict",
			setup: func(t *testing.T, h *elasticsearch.Handler) (string, string) {
				t.Helper()

				pkgID := createTestPackage(t, h, "dup-dict")

				domResp := doRequest(t, h, http.MethodPost, "/2015-01-01/es/domain", map[string]any{
					"DomainName": "dup-domain",
				})
				domResp.Body.Close()

				// First association succeeds; the test body issues the duplicate.
				firstResp := doRequest(t, h, http.MethodPost,
					"/2015-01-01/packages/associate/"+pkgID+"/dup-domain", nil)
				firstResp.Body.Close()
				require.Equal(t, http.StatusOK, firstResp.StatusCode)

				return pkgID, "dup-domain"
			},
			wantCode:     http.StatusConflict,
			wantContains: []string{"ConflictException"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			packageID, domainName := tt.setup(t, h)

			resp := doRequest(t, h, http.MethodPost,
				"/2015-01-01/packages/associate/"+packageID+"/"+domainName, nil)
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

// TestElasticsearchHandler_AssociatePackage_DuplicateRejected covers the same
// duplicate-association scenario across a table of domains, verifying a
// first association always succeeds and a same-package second association is
// rejected while a different-package second association is allowed.
func TestElasticsearchHandler_AssociatePackage_DuplicateRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		domainName string
		wantCode   int
	}{
		{
			name:       "first_association_succeeds",
			domainName: "parity-dom-first",
			wantCode:   http.StatusOK,
		},
		{
			name:       "second_association_rejected",
			domainName: "parity-dom-dup",
			wantCode:   http.StatusConflict,
		},
		{
			name:       "different_package_allowed",
			domainName: "parity-dom-diff",
			wantCode:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			createDomainAndGetARN(t, h, tt.domainName)

			switch tt.name {
			case "first_association_succeeds":
				pkgID := createTestPackage(t, h, "pkg-first")
				resp := doRequest(
					t, h, http.MethodPost, "/2015-01-01/packages/associate/"+pkgID+"/"+tt.domainName, nil,
				)
				resp.Body.Close()
				assert.Equal(t, tt.wantCode, resp.StatusCode)

			case "second_association_rejected":
				pkgID := createTestPackage(t, h, "pkg-dup")
				// First association must succeed.
				first := doRequest(
					t, h, http.MethodPost, "/2015-01-01/packages/associate/"+pkgID+"/"+tt.domainName, nil,
				)
				first.Body.Close()
				require.Equal(t, http.StatusOK, first.StatusCode)
				// Second association of the same package to the same domain must fail.
				second := doRequest(
					t, h, http.MethodPost, "/2015-01-01/packages/associate/"+pkgID+"/"+tt.domainName, nil,
				)
				second.Body.Close()
				assert.Equal(t, tt.wantCode, second.StatusCode)

			case "different_package_allowed":
				pkgA := createTestPackage(t, h, "pkg-a")
				pkgB := createTestPackage(t, h, "pkg-b")
				respA := doRequest(
					t, h, http.MethodPost, "/2015-01-01/packages/associate/"+pkgA+"/"+tt.domainName, nil,
				)
				respA.Body.Close()
				require.Equal(t, http.StatusOK, respA.StatusCode)
				respB := doRequest(
					t, h, http.MethodPost, "/2015-01-01/packages/associate/"+pkgB+"/"+tt.domainName, nil,
				)
				respB.Body.Close()
				assert.Equal(t, tt.wantCode, respB.StatusCode)
			}
		})
	}
}

// TestElasticsearchHandler_Packages_CRUD drives CreatePackage, DescribePackages
// (POST), and DeletePackage through the HTTP handler.
func TestElasticsearchHandler_Packages_CRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	pkgID := createTestPackage(t, h, "test-pkg")
	assert.NotEmpty(t, pkgID)

	// DescribePackages (POST)
	resp := doRequest(t, h, http.MethodPost, "/2015-01-01/packages/describe", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// DeletePackage
	resp = doRequest(t, h, http.MethodDelete, "/2015-01-01/packages/"+pkgID, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestElasticsearchHandler_Packages_AssociateDissociate drives the full
// associate/list/history/update/dissociate lifecycle through the HTTP handler.
func TestElasticsearchHandler_Packages_AssociateDissociate(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	domain := createTestDomainName(t, h, "assoc-dom")
	pkgID := createTestPackage(t, h, "assoc-pkg")

	// AssociatePackage
	resp := doRequest(t, h, http.MethodPost,
		"/2015-01-01/packages/associate/"+pkgID+"/"+domain, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// ListPackagesForDomain
	resp = doRequest(t, h, http.MethodGet,
		"/2015-01-01/domain/"+domain+"/packages", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// ListDomainsForPackage
	resp = doRequest(t, h, http.MethodGet,
		"/2015-01-01/packages/"+pkgID+"/domains", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// GetPackageVersionHistory
	resp = doRequest(t, h, http.MethodGet,
		"/2015-01-01/packages/"+pkgID+"/history", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// UpdatePackage
	resp = doRequest(t, h, http.MethodPost, "/2015-01-01/packages/update", map[string]any{
		"PackageID":          pkgID,
		"PackageDescription": "updated",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// DissociatePackage
	resp = doRequest(t, h, http.MethodPost,
		"/2015-01-01/packages/dissociate/"+pkgID+"/"+domain, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestElasticsearchHandler_PackageTypeValidation verifies CreatePackage
// rejects unknown PackageType values.
func TestElasticsearchHandler_PackageTypeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		packageType string
		wantStatus  int
	}{
		{name: "txt-dictionary", packageType: "TXT-DICTIONARY", wantStatus: http.StatusOK},
		// ZIP-PLUGIN is a valid OpenSearch Service package type but NOT valid
		// for the legacy elasticsearchservice API this backend emulates --
		// types.PackageType's only enum value is TXT-DICTIONARY.
		{name: "zip-plugin", packageType: "ZIP-PLUGIN", wantStatus: http.StatusBadRequest},
		{name: "invalid-type", packageType: "INVALID-TYPE", wantStatus: http.StatusBadRequest},
		{name: "empty-type", packageType: "", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			resp := doRequest(t, h, http.MethodPost, "/2015-01-01/packages", map[string]any{
				"PackageName":        "pkg-" + tt.name,
				"PackageType":        tt.packageType,
				"PackageDescription": "test package",
				"PackageSource":      map[string]any{"S3BucketName": "b", "S3Key": "k"},
			})
			defer resp.Body.Close()
			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

// TestElasticsearchHandler_PackageTypeBackend verifies the backend directly
// rejects invalid package types.
func TestElasticsearchHandler_PackageTypeBackend(t *testing.T) {
	t.Parallel()

	b := elasticsearch.NewInMemoryBackend("123456789012", "us-east-1")
	src := elasticsearch.PackageSource{S3BucketName: "b", S3Key: "k"}

	_, err := b.CreatePackage(context.Background(), "my-pkg", "UNKNOWN", "desc", src)
	require.ErrorIs(t, err, elasticsearch.ErrValidation)

	_, err = b.CreatePackage(context.Background(), "my-pkg2", "TXT-DICTIONARY", "desc", src)
	require.NoError(t, err)
}

// TestElasticsearchHandler_PackageSourceRequired verifies a missing
// PackageSource (S3BucketName/S3Key) returns ErrValidation, matching real
// AWS's required CreatePackageInput.PackageSource member.
func TestElasticsearchHandler_PackageSourceRequired(t *testing.T) {
	t.Parallel()

	b := elasticsearch.NewInMemoryBackend("123456789012", "us-east-1")

	_, err := b.CreatePackage(context.Background(), "my-pkg", "TXT-DICTIONARY", "desc", elasticsearch.PackageSource{})
	require.ErrorIs(t, err, elasticsearch.ErrValidation)

	_, err = b.CreatePackage(context.Background(), "my-pkg2", "TXT-DICTIONARY", "desc",
		elasticsearch.PackageSource{S3BucketName: "bucket"})
	require.ErrorIs(t, err, elasticsearch.ErrValidation)
}

// TestElasticsearchHandler_PackageValidation verifies empty package name
// returns ErrValidation.
func TestElasticsearchHandler_PackageValidation(t *testing.T) {
	t.Parallel()

	b := elasticsearch.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.CreatePackage(context.Background(), "", "TXT-DICTIONARY", "", elasticsearch.PackageSource{
		S3BucketName: "b", S3Key: "k",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, elasticsearch.ErrValidation)
}

// TestElasticsearchHandler_Packages_Lifecycle drives the full package
// associate/update/describe/list/dissociate/delete lifecycle, checking
// response bodies at each step (a superset of
// TestElasticsearchHandler_Packages_AssociateDissociate's route coverage).
func TestElasticsearchHandler_Packages_Lifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	domain := createTestDomainName(t, h, "pkg-state-domain")
	packageID := createTestPackage(t, h, "state-package")

	resp := doRequest(t, h, http.MethodPost, "/2015-01-01/packages/associate/"+packageID+"/"+domain, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = doRequest(t, h, http.MethodPost, "/2015-01-01/packages/update", map[string]any{
		"PackageID": packageID, "PackageDescription": "changed",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "changed", readJSONBody(t, resp)["PackageDetails"].(map[string]any)["PackageDescription"])

	resp = doRequest(t, h, http.MethodPost, "/2015-01-01/packages/describe", nil)
	require.Len(t, readJSONBody(t, resp)["PackageDetailsList"], 1)

	resp = doRequest(t, h, http.MethodGet, "/2015-01-01/packages/"+packageID+"/domains", nil)
	require.Len(t, readJSONBody(t, resp)["DomainPackageDetailsList"], 1)

	resp = doRequest(t, h, http.MethodGet, "/2015-01-01/domain/"+domain+"/packages", nil)
	require.Len(t, readJSONBody(t, resp)["DomainPackageDetailsList"], 1)

	resp = doRequest(t, h, http.MethodPost, "/2015-01-01/packages/dissociate/"+packageID+"/"+domain, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = doRequest(t, h, http.MethodDelete, "/2015-01-01/packages/"+packageID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = doRequest(t, h, http.MethodPost, "/2015-01-01/packages/describe", nil)
	assert.Empty(t, readJSONBody(t, resp)["PackageDetailsList"])
}

// TestElasticsearchHandler_Package_Timestamps verifies CreatedAt/LastUpdatedAt
// are set on CreatePackage and that LastUpdatedAt advances (never regresses)
// on UpdatePackage, matching types.PackageDetails. ErrorDetails is never
// present: this backend has no COPYING/COPY_FAILED state machine.
func TestElasticsearchHandler_Package_Timestamps(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	resp := doRequest(t, h, http.MethodPost, "/2015-01-01/packages", map[string]any{
		"PackageName":   "timestamped-pkg",
		"PackageType":   "TXT-DICTIONARY",
		"PackageSource": map[string]any{"S3BucketName": "b", "S3Key": "k"},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	created := readJSONBody(t, resp)["PackageDetails"].(map[string]any)
	packageID := created["PackageID"].(string)
	createdAt, ok := created["CreatedAt"].(float64)
	require.True(t, ok, "CreatedAt must be a JSON number")
	require.Positive(t, createdAt)
	lastUpdatedAt, ok := created["LastUpdatedAt"].(float64)
	require.True(t, ok, "LastUpdatedAt must be a JSON number")
	assert.InEpsilon(t, createdAt, lastUpdatedAt, 0)
	assert.NotContains(t, created, "ErrorDetails")

	resp = doRequest(t, h, http.MethodPost, "/2015-01-01/packages/update", map[string]any{
		"PackageID": packageID, "PackageDescription": "changed",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	updated := readJSONBody(t, resp)["PackageDetails"].(map[string]any)
	assert.InEpsilon(t, createdAt, updated["CreatedAt"], 0)
	assert.GreaterOrEqual(t, updated["LastUpdatedAt"], lastUpdatedAt)
}

// TestElasticsearchHandler_ListPackagesForDomain_UnknownDomain verifies
// ListPackagesForDomain rejects a domain name that does not exist with
// ResourceNotFoundException (404), matching real AWS:
// ListPackagesForDomainOutput's deserializer (elasticsearchservice@v1.45.4
// deserializers.go, awsRestjson1_deserializeOpErrorListPackagesForDomain)
// declares ResourceNotFoundException among its modelled errors. Before the
// fix the backend never checked domain existence at all and always returned
// an empty (or ghost-populated) list with 200 OK.
func TestElasticsearchHandler_ListPackagesForDomain_UnknownDomain(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	resp := doRequest(t, h, http.MethodGet, "/2015-01-01/domain/no-such-domain/packages", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestElasticsearchHandler_DeleteDomain_ClearsPackageAssociations verifies
// that deleting a domain removes it from every package's association list
// (packageAssociationsStore), instead of leaving a ghost row that
// ListDomainsForPackage would keep reporting forever. Both directions are
// checked: ListDomainsForPackage must no longer list the deleted domain, and
// ListPackagesForDomain against the now-gone domain name must 404 rather
// than read the stale association.
func TestElasticsearchHandler_DeleteDomain_ClearsPackageAssociations(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	domain := createTestDomainName(t, h, "ghost-assoc-dom")
	pkgID := createTestPackage(t, h, "ghost-assoc-pkg")

	assocResp := doRequest(t, h, http.MethodPost,
		"/2015-01-01/packages/associate/"+pkgID+"/"+domain, nil)
	assocResp.Body.Close()
	require.Equal(t, http.StatusOK, assocResp.StatusCode)

	delResp := doRequest(t, h, http.MethodDelete, "/2015-01-01/es/domain/"+domain, nil)
	delResp.Body.Close()
	require.Equal(t, http.StatusOK, delResp.StatusCode)

	domainsResp := doRequest(t, h, http.MethodGet, "/2015-01-01/packages/"+pkgID+"/domains", nil)
	defer domainsResp.Body.Close()
	require.Equal(t, http.StatusOK, domainsResp.StatusCode)

	domainsOut := readJSONBody(t, domainsResp)
	details, ok := domainsOut["DomainPackageDetailsList"].([]any)
	require.True(t, ok)
	assert.Empty(t, details, "deleted domain must not remain in the package's association list")
}
