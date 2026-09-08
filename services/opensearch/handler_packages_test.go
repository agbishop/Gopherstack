package opensearch_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	opensearchsdk "github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/opensearch"
)

// TestDescribePackages_PaginationOrderIsReproducible walks every package via
// NextToken-based pagination and asserts the concatenation of pages
// reproduces the full set exactly -- no drops, no duplicates. DescribePackages
// (with no PackageID filter) builds its page from b.packages.All(), an
// unspecified-order map walk (pkgs/store's Table.All doc), with no sort
// before pkgs/page.New's offset-based pagination -- so a second call backing
// a later page can observe a completely different order than the first,
// corrupting the walk even though PackageID (the table's own key) is unique.
func TestDescribePackages_PaginationOrderIsReproducible(t *testing.T) {
	t.Parallel()

	const numPackages = 60
	const pageSize = 7

	for iter := range 30 {
		_, h := newTestHandlerAndBackend()
		client := newTestOpenSearchClient(t, h)
		ctx := t.Context()

		want := make(map[string]bool, numPackages)

		for i := range numPackages {
			name := fmt.Sprintf("order-pkg-%03d", i)

			out, err := client.CreatePackage(ctx, &opensearchsdk.CreatePackageInput{
				PackageName: aws.String(name),
				PackageType: types.PackageTypeTxtDictionary,
				PackageSource: &types.PackageSource{
					S3BucketName: aws.String("pkg-bucket"),
					S3Key:        aws.String("pkg-key-" + name),
				},
			})
			require.NoErrorf(t, err, "iteration %d: setup create package %q", iter, name)
			want[aws.ToString(out.PackageDetails.PackageID)] = true
		}

		got := make(map[string]int, numPackages)

		var nextToken *string

		for page := range numPackages/pageSize + 5 {
			out, err := client.DescribePackages(ctx, &opensearchsdk.DescribePackagesInput{
				MaxResults: pageSize,
				NextToken:  nextToken,
			})
			require.NoErrorf(t, err, "iteration %d page %d", iter, page)

			for _, pd := range out.PackageDetailsList {
				got[aws.ToString(pd.PackageID)]++
			}

			if aws.ToString(out.NextToken) == "" {
				break
			}

			nextToken = out.NextToken
		}

		for id := range want {
			assert.Equalf(t, 1, got[id], "iteration %d: package %s expected exactly once, got %d", iter, id, got[id])
		}

		assert.Lenf(t, got, numPackages, "iteration %d: total distinct packages returned", iter)
	}
}

// TestOpenSearchHandler_DeletePackage_RejectedWhileAssociated locks real
// AWS's DeletePackage doc comment: "The package can't be associated with
// any OpenSearch Service domain".
func TestOpenSearchHandler_DeletePackage_RejectedWhileAssociated(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	createTestDomain(t, h, "delpkg-domain")

	pkgResp := doRequest(t, h, http.MethodPost, "/2021-01-01/packages",
		map[string]any{"PackageName": "delpkg-pkg", "PackageType": "TXT-DICTIONARY"})

	var pkgOut map[string]any
	require.NoError(t, json.NewDecoder(pkgResp.Body).Decode(&pkgOut))
	pkgResp.Body.Close()

	pkgDetails, ok := pkgOut["PackageDetails"].(map[string]any)
	require.True(t, ok)
	pkgID, ok := pkgDetails["PackageID"].(string)
	require.True(t, ok)

	assocResp := doRequest(t, h, http.MethodPost,
		"/2021-01-01/packages/associate/"+pkgID+"/delpkg-domain", nil)
	assocResp.Body.Close()
	require.Equal(t, http.StatusOK, assocResp.StatusCode)

	delResp := doRequest(t, h, http.MethodDelete, "/2021-01-01/packages/"+pkgID, nil)
	defer delResp.Body.Close()
	assert.Equal(t, http.StatusConflict, delResp.StatusCode)

	dissocResp := doRequest(t, h, http.MethodPost,
		"/2021-01-01/packages/dissociate/"+pkgID+"/delpkg-domain", nil)
	dissocResp.Body.Close()
	require.Equal(t, http.StatusOK, dissocResp.StatusCode)

	delResp2 := doRequest(t, h, http.MethodDelete, "/2021-01-01/packages/"+pkgID, nil)
	defer delResp2.Body.Close()
	assert.Equal(t, http.StatusOK, delResp2.StatusCode)
}

func TestOpenSearchHandler_DissociatePackage(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	// Create domain.
	createTestDomain(t, h, "dissoc-domain")

	// Create package.
	pkgResp := doRequest(t, h, http.MethodPost, "/2021-01-01/packages",
		map[string]any{"PackageName": "my-pkg", "PackageType": "TXT-DICTIONARY"})

	var pkgOut map[string]any
	require.NoError(t, json.NewDecoder(pkgResp.Body).Decode(&pkgOut))
	pkgResp.Body.Close()

	pkgDetails, ok := pkgOut["PackageDetails"].(map[string]any)
	require.True(t, ok)
	pkgID, ok := pkgDetails["PackageID"].(string)
	require.True(t, ok)

	// Associate the package.
	assocResp := doRequest(t, h, http.MethodPost,
		"/2021-01-01/packages/associate/"+pkgID+"/dissoc-domain", nil)
	assocResp.Body.Close()
	require.Equal(t, http.StatusOK, assocResp.StatusCode)

	// Dissociate.
	dissocResp := doRequest(t, h, http.MethodPost,
		"/2021-01-01/packages/dissociate/"+pkgID+"/dissoc-domain", nil)
	defer dissocResp.Body.Close()

	assert.Equal(t, http.StatusOK, dissocResp.StatusCode)

	var out map[string]any
	require.NoError(t, json.NewDecoder(dissocResp.Body).Decode(&out))

	details2, ok := out["DomainPackageDetails"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, pkgID, details2["PackageID"])
	assert.Equal(t, "dissoc-domain", details2["DomainName"])
	assert.Equal(t, "DISSOCIATING", details2["DomainPackageStatus"])
}

func TestOpenSearchHandler_DissociatePackages(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	// Create domain.
	createTestDomain(t, h, "multi-dissoc-domain")

	// Create two packages and associate them.
	pkgIDs := make([]string, 0, 2)

	for _, name := range []string{"pkg-one", "pkg-two"} {
		pkgResp := doRequest(t, h, http.MethodPost, "/2021-01-01/packages",
			map[string]any{"PackageName": name, "PackageType": "TXT-DICTIONARY"})
		var pkgOut map[string]any
		require.NoError(t, json.NewDecoder(pkgResp.Body).Decode(&pkgOut))
		pkgResp.Body.Close()

		id := pkgOut["PackageDetails"].(map[string]any)["PackageID"].(string)
		pkgIDs = append(pkgIDs, id)

		assocResp := doRequest(t, h, http.MethodPost,
			"/2021-01-01/packages/associate/"+id+"/multi-dissoc-domain", nil)
		assocResp.Body.Close()
	}

	// Dissociate multiple.
	pkgList := make([]map[string]any, 0, len(pkgIDs))
	for _, id := range pkgIDs {
		pkgList = append(pkgList, map[string]any{"PackageID": id})
	}

	resp := doRequest(t, h, http.MethodPost, "/2021-01-01/packages/dissociateMultiple",
		map[string]any{
			"DomainName":  "multi-dissoc-domain",
			"PackageList": pkgList,
		})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))

	list, ok := out["DomainPackageDetailsList"].([]any)
	require.True(t, ok)
	assert.Len(t, list, 2)

	for _, item := range list {
		entry, ok2 := item.(map[string]any)
		require.True(t, ok2)
		assert.Equal(t, "DISSOCIATING", entry["DomainPackageStatus"])
	}
}

func TestListPackagesForDomain_ReturnsDomainPackageDetailsShape(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	createTestDomain(t, h, "list-pkg-domain")

	pkgResp := doRequest(t, h, http.MethodPost, "/2021-01-01/packages",
		map[string]any{"PackageName": "list-pkg", "PackageType": "TXT-DICTIONARY"})
	var pkgOut map[string]any
	require.NoError(t, json.NewDecoder(pkgResp.Body).Decode(&pkgOut))
	pkgResp.Body.Close()
	pkgID := pkgOut["PackageDetails"].(map[string]any)["PackageID"].(string)

	assocResp := doRequest(t, h, http.MethodPost,
		"/2021-01-01/packages/associate/"+pkgID+"/list-pkg-domain", nil)
	assocResp.Body.Close()

	// GET /domain/{name}/packages must return the DomainPackageDetailsList
	// shape (PackageID/DomainName/DomainPackageStatus/PackageName/PackageType),
	// not raw Package objects.
	lr := doRequest(t, h, http.MethodGet,
		"/2021-01-01/domain/list-pkg-domain/packages", nil)
	defer lr.Body.Close()
	require.Equal(t, http.StatusOK, lr.StatusCode)

	var lOut map[string]any
	require.NoError(t, json.NewDecoder(lr.Body).Decode(&lOut))
	list, ok := lOut["DomainPackageDetailsList"].([]any)
	require.True(t, ok)
	require.Len(t, list, 1)

	entry := list[0].(map[string]any)
	assert.Equal(t, pkgID, entry["PackageID"])
	assert.Equal(t, "list-pkg-domain", entry["DomainName"])
	assert.Equal(t, "ACTIVE", entry["DomainPackageStatus"])
	assert.Equal(t, "list-pkg", entry["PackageName"])
	assert.Equal(t, "TXT-DICTIONARY", entry["PackageType"])
}

func TestListDomainsForPackage_ReturnsDomainPackageDetailsShape(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	createTestDomain(t, h, "domain-for-pkg")

	pkgResp := doRequest(t, h, http.MethodPost, "/2021-01-01/packages",
		map[string]any{"PackageName": "shared-pkg", "PackageType": "TXT-DICTIONARY"})
	var pkgOut map[string]any
	require.NoError(t, json.NewDecoder(pkgResp.Body).Decode(&pkgOut))
	pkgResp.Body.Close()
	pkgID := pkgOut["PackageDetails"].(map[string]any)["PackageID"].(string)

	assocResp := doRequest(t, h, http.MethodPost,
		"/2021-01-01/packages/associate/"+pkgID+"/domain-for-pkg", nil)
	assocResp.Body.Close()

	// GET /packages/{id}/domains must return the DomainPackageDetailsList
	// shape, not bare domain-name strings.
	lr := doRequest(t, h, http.MethodGet, "/2021-01-01/packages/"+pkgID+"/domains", nil)
	defer lr.Body.Close()
	require.Equal(t, http.StatusOK, lr.StatusCode)

	var lOut map[string]any
	require.NoError(t, json.NewDecoder(lr.Body).Decode(&lOut))
	list, ok := lOut["DomainPackageDetailsList"].([]any)
	require.True(t, ok)
	require.Len(t, list, 1)

	entry := list[0].(map[string]any)
	assert.Equal(t, pkgID, entry["PackageID"])
	assert.Equal(t, "domain-for-pkg", entry["DomainName"])
	assert.Equal(t, "ACTIVE", entry["DomainPackageStatus"])
	assert.Equal(t, "shared-pkg", entry["PackageName"])
}

func TestAssociatePackages_EmptyList(t *testing.T) {
	t.Parallel()

	b := opensearch.NewInMemoryBackend(testAccountID, testRegion)
	b.AddDomainInternal("my-domain", "")

	_, err := b.AssociatePackages("my-domain", []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, opensearch.ErrInvalidParameter)
}

func TestHTTPAssociatePackages_BadJSON(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	resp := doRequest(t, h, http.MethodPost, "/2021-01-01/packages/associateMultiple", nil)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAssociatePackage_PackageValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(b *opensearch.InMemoryBackend)
		packageID  string
		domainName string
		wantErr    bool
	}{
		{
			name: "success_when_both_exist",
			setup: func(b *opensearch.InMemoryBackend) {
				b.AddDomainInternal("assoc-domain", "")
				b.AddPackageInternal("pkg-valid", "my-pkg", "TXT-DICTIONARY")
			},
			packageID:  "pkg-valid",
			domainName: "assoc-domain",
			wantErr:    false,
		},
		{
			name: "error_package_not_found",
			setup: func(b *opensearch.InMemoryBackend) {
				b.AddDomainInternal("assoc-domain-np", "")
			},
			packageID:  "pkg-missing",
			domainName: "assoc-domain-np",
			wantErr:    true,
		},
		{
			name: "error_domain_not_found",
			setup: func(b *opensearch.InMemoryBackend) {
				b.AddPackageInternal("pkg-nodomain", "my-pkg", "TXT-DICTIONARY")
			},
			packageID:  "pkg-nodomain",
			domainName: "missing-domain",
			wantErr:    true,
		},
		{
			name:       "error_both_not_found",
			setup:      func(_ *opensearch.InMemoryBackend) {},
			packageID:  "pkg-none",
			domainName: "dom-none",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := opensearch.NewInMemoryBackend("123456789012", "us-east-1")
			tt.setup(b)

			_, err := b.AssociatePackage(tt.packageID, tt.domainName)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAssociatePackage_HTTPValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T, h *opensearch.Handler)
		packageID string
		domain    string
		wantCode  int
	}{
		{
			name: "success_package_exists",
			setup: func(t *testing.T, h *opensearch.Handler) {
				t.Helper()
				r := doRequest(t, h, http.MethodPost, "/2021-01-01/opensearch/domain",
					map[string]any{"DomainName": "pkg-dom"})
				r.Body.Close()
				h.Backend.(*opensearch.InMemoryBackend).AddPackageInternal("pkg-real", "real-pkg", "TXT-DICTIONARY")
			},
			packageID: "pkg-real",
			domain:    "pkg-dom",
			wantCode:  http.StatusOK,
		},
		{
			name: "error_package_not_found",
			setup: func(t *testing.T, h *opensearch.Handler) {
				t.Helper()
				r := doRequest(t, h, http.MethodPost, "/2021-01-01/opensearch/domain",
					map[string]any{"DomainName": "pkg-dom2"})
				r.Body.Close()
			},
			packageID: "pkg-ghost",
			domain:    "pkg-dom2",
			wantCode:  http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			tt.setup(t, h)

			path := "/2021-01-01/packages/associate/" + tt.packageID + "/" + tt.domain
			resp := doRequest(t, h, http.MethodPost, path, nil)
			defer resp.Body.Close()

			assert.Equal(t, tt.wantCode, resp.StatusCode)
		})
	}
}

func TestAssociatePackages_ValidatesPackageExistence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		packageID  string
		domainName string
		wantCode   int
	}{
		{
			name:       "unknown package returns error",
			packageID:  "F-does-not-exist",
			domainName: "my-domain",
			wantCode:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			resp := doRequest(t, h, http.MethodPost, "/2021-01-01/opensearch/domain", map[string]any{
				"DomainName": tt.domainName,
			})
			resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			resp = doRequest(t, h, http.MethodPost, "/2021-01-01/packages/associateMultiple", map[string]any{
				"DomainName":  tt.domainName,
				"PackageList": []map[string]any{{"PackageID": tt.packageID}},
			})
			defer resp.Body.Close()

			assert.Equal(t, tt.wantCode, resp.StatusCode)
		})
	}
}

func TestOpenSearchHandler_AssociatePackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *opensearch.Handler)
		name         string
		packageID    string
		domainName   string
		wantContains []string
		wantCode     int
	}{
		{
			name:       "success",
			packageID:  "pkg-001",
			domainName: "my-domain",
			setup: func(t *testing.T, h *opensearch.Handler) {
				t.Helper()
				r := doRequest(t, h, http.MethodPost, "/2021-01-01/opensearch/domain",
					map[string]any{"DomainName": "my-domain"})
				r.Body.Close()
				h.Backend.(*opensearch.InMemoryBackend).AddPackageInternal("pkg-001", "my-pkg", "TXT-DICTIONARY")
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"my-domain", "ACTIVE"},
		},
		{
			name:       "domain_not_found",
			packageID:  "pkg-002",
			domainName: "nonexistent",
			wantCode:   http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			path := "/2021-01-01/packages/associate/" + tt.packageID + "/" + tt.domainName
			resp := doRequest(t, h, http.MethodPost, path, nil)
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

func TestOpenSearchHandler_AssociatePackages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *opensearch.Handler)
		name         string
		domainName   string
		packageIDs   []string
		wantContains []string
		wantCode     int
	}{
		{
			name:       "success",
			domainName: "my-domain",
			packageIDs: []string{"pkg-001", "pkg-002"},
			setup: func(t *testing.T, h *opensearch.Handler) {
				t.Helper()
				r := doRequest(t, h, http.MethodPost, "/2021-01-01/opensearch/domain",
					map[string]any{"DomainName": "my-domain"})
				r.Body.Close()
				b := h.Backend.(*opensearch.InMemoryBackend)
				b.AddPackageInternal("pkg-001", "pkg-one", "TXT-DICTIONARY")
				b.AddPackageInternal("pkg-002", "pkg-two", "TXT-DICTIONARY")
			},
			wantCode:     http.StatusOK,
			wantContains: []string{"DomainPackageDetailsList"},
		},
		{
			name:       "domain_not_found",
			domainName: "nonexistent",
			packageIDs: []string{"pkg-001"},
			wantCode:   http.StatusNotFound,
		},
		{
			name:       "invalid_json",
			domainName: "",
			packageIDs: nil,
			wantCode:   http.StatusBadRequest,
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
				req := httptest.NewRequest(
					http.MethodPost,
					"/2021-01-01/packages/associateMultiple",
					strings.NewReader("bad-json"),
				)
				req.Header.Set("Content-Type", "application/json")
				rw := httptest.NewRecorder()
				h.ServeHTTP(rw, req)
				resp := rw.Result()
				defer resp.Body.Close()
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

				return
			}

			pkgList := make([]map[string]string, 0, len(tt.packageIDs))
			for _, id := range tt.packageIDs {
				pkgList = append(pkgList, map[string]string{"PackageID": id})
			}

			body := map[string]any{
				"DomainName":  tt.domainName,
				"PackageList": pkgList,
			}
			resp := doRequest(t, h, http.MethodPost, "/2021-01-01/packages/associateMultiple", body)
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
