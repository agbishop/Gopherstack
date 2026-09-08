package codeconnections_test

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codeconnections"
)

// TestListHostsExcludesTags verifies ListHosts output items carry the real
// Host field set but never a Tags member: aws-sdk-go-v2/service/
// codeconnections@v1.13.4's types.Host has no Tags field at all (confirmed
// against awsAwsjson10_deserializeDocumentHost's case switch) -- tags for a
// host are only ever returned via ListTagsForResource. A previous version of
// this test asserted Tags on ListHosts items as correct.
func TestListHostsExcludesTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tags     []map[string]string
		wantTags int
	}{
		{
			name: "tags_in_list_item",
			tags: []map[string]string{
				{"Key": "Env", "Value": "staging"},
				{"Key": "Owner", "Value": "ops"},
			},
			wantTags: 2,
		},
		{
			name:     "no_tags_list_item",
			tags:     nil,
			wantTags: 0,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			body := map[string]any{
				"Name":             "tagged-host-" + strconv.Itoa(i),
				"ProviderType":     "GitHubEnterpriseServer",
				"ProviderEndpoint": "https://ghe.example.com",
			}
			if tt.tags != nil {
				body["Tags"] = tt.tags
			}

			rec := doJSON(t, h, "CreateHost", body)
			require.Equal(t, http.StatusOK, rec.Code)
			hostArn := parseResp(t, rec)["HostArn"].(string)

			listRec := doJSON(t, h, "ListHosts", nil)
			require.Equal(t, http.StatusOK, listRec.Code)

			resp := parseResp(t, listRec)
			hosts, ok := resp["Hosts"].([]any)
			require.True(t, ok)
			require.Len(t, hosts, 1)

			hostMap, isMap := hosts[0].(map[string]any)
			require.True(t, isMap)

			_, hasTags := hostMap["Tags"]
			assert.False(t, hasTags, "ListHosts item must not include a Tags member")

			listTagsRec := doJSON(t, h, "ListTagsForResource", map[string]any{"ResourceArn": hostArn})
			require.Equal(t, http.StatusOK, listTagsRec.Code)
			tags, _ := parseResp(t, listTagsRec)["Tags"].([]any)
			assert.Len(t, tags, tt.wantTags)
		})
	}
}

// TestListHostsPagination verifies MaxResults and NextToken work for ListHosts.
func TestListHostsPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		hostCount  int
		maxResults int
		wantCount  int
		wantToken  bool
	}{
		{
			name:       "first_page",
			hostCount:  3,
			maxResults: 2,
			wantCount:  2,
			wantToken:  true,
		},
		{
			name:       "all_results",
			hostCount:  2,
			maxResults: 10,
			wantCount:  2,
			wantToken:  false,
		},
		{
			name:       "zero_max_defaults",
			hostCount:  2,
			maxResults: 0,
			wantCount:  2,
			wantToken:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			for i := range tt.hostCount {
				rec := doJSON(t, h, "CreateHost", map[string]any{
					"Name":             "phost-" + strconv.Itoa(i),
					"ProviderType":     "GitHubEnterpriseServer",
					"ProviderEndpoint": "https://ghe" + strconv.Itoa(i) + ".example.com",
				})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			body := map[string]any{}
			if tt.maxResults > 0 {
				body["MaxResults"] = tt.maxResults
			}

			rec := doJSON(t, h, "ListHosts", body)
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			hosts, ok := resp["Hosts"].([]any)
			require.True(t, ok)
			assert.Len(t, hosts, tt.wantCount)

			_, hasToken := resp["NextToken"]
			assert.Equal(t, tt.wantToken, hasToken)
		})
	}
}

// TestListHostsContinuation verifies two-page traversal for ListHosts.
func TestListHostsContinuation(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	for i := range 3 {
		rec := doJSON(t, h, "CreateHost", map[string]any{
			"Name":             "cont-host-" + strconv.Itoa(i),
			"ProviderType":     "GitHubEnterpriseServer",
			"ProviderEndpoint": "https://ghe" + strconv.Itoa(i) + ".example.com",
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec1 := doJSON(t, h, "ListHosts", map[string]any{"MaxResults": 2})
	require.Equal(t, http.StatusOK, rec1.Code)
	resp1 := parseResp(t, rec1)
	page1, ok := resp1["Hosts"].([]any)
	require.True(t, ok)
	assert.Len(t, page1, 2)

	nextToken, hasToken := resp1["NextToken"].(string)
	require.True(t, hasToken)
	require.NotEmpty(t, nextToken)

	rec2 := doJSON(t, h, "ListHosts", map[string]any{
		"MaxResults": 2,
		"NextToken":  nextToken,
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	resp2 := parseResp(t, rec2)
	page2, ok := resp2["Hosts"].([]any)
	require.True(t, ok)
	assert.Len(t, page2, 1)

	_, stillHasToken := resp2["NextToken"]
	assert.False(t, stillHasToken)

	names := make([]string, 0, 3)
	for _, item := range append(page1, page2...) {
		hMap := item.(map[string]any)
		names = append(names, hMap["Name"].(string))
	}
	assert.ElementsMatch(t, []string{"cont-host-0", "cont-host-1", "cont-host-2"}, names)
}

// TestUpdateHost exercises UpdateHost happy path and error cases.
func TestUpdateHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupHostArn func(t *testing.T, h *codeconnections.Handler) string
		name         string
		newEndpoint  string
		wantStatus   int
	}{
		{
			name: "success_updates_endpoint",
			setupHostArn: func(t *testing.T, h *codeconnections.Handler) string {
				t.Helper()

				return createHost(
					t,
					h,
					"updateable-host",
					"GitHubEnterpriseServer",
					"https://old.example.com",
				)
			},
			newEndpoint: "https://new.example.com",
			wantStatus:  http.StatusOK,
		},
		{
			name: "not_found",
			setupHostArn: func(_ *testing.T, _ *codeconnections.Handler) string {
				return "arn:aws:codeconnections:us-east-1:123:host/nonexistent"
			},
			newEndpoint: "https://new.example.com",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name: "missing_arn",
			setupHostArn: func(_ *testing.T, _ *codeconnections.Handler) string {
				return ""
			},
			newEndpoint: "https://new.example.com",
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			hostArn := tt.setupHostArn(t, h)

			rec := doJSON(t, h, "UpdateHost", map[string]any{
				"HostArn":          hostArn,
				"ProviderEndpoint": tt.newEndpoint,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				getRec := doJSON(t, h, "GetHost", map[string]any{"HostArn": hostArn})
				require.Equal(t, http.StatusOK, getRec.Code)
				resp := parseResp(t, getRec)
				assert.Equal(t, tt.newEndpoint, resp["ProviderEndpoint"])
			}
		})
	}
}

// TestBackendCreateHostNameNotUnique verifies duplicate host names succeed
// (see TestCreateHostNameNotUnique for why: CreateHost has no
// ResourceAlreadyExistsException in its real error list).
func TestBackendCreateHostNameNotUnique(t *testing.T) {
	t.Parallel()

	b := codeconnections.NewInMemoryBackend("123456789012", "us-east-1")

	host1, err := b.CreateHost(
		context.Background(),
		"unique-host-x",
		"GitHubEnterpriseServer",
		"https://a.example.com",
		nil,
		nil,
	)
	require.NoError(t, err, "first create should succeed")

	host2, err := b.CreateHost(
		context.Background(),
		"unique-host-x",
		"GitHubEnterpriseServer",
		"https://b.example.com",
		nil,
		nil,
	)
	require.NoError(t, err, "duplicate host name must succeed")
	assert.NotEqual(t, host1.HostArn, host2.HostArn)
}

// TestListHostsOrdersTiedNamesByArn verifies that when two hosts share a Name
// (allowed -- see TestBackendCreateHostNameNotUnique), ListHosts still returns
// a deterministic total order between them, broken by HostArn. Hosts are
// seeded via AddHostInternal in descending-ARN insertion order: without a
// secondary sort key, tied-Name hosts are left in insertion order (the
// backing index is insertion-ordered, not re-sorted for ties), which is the
// reverse of the expected ascending-ARN order this test asserts.
func TestListHostsOrdersTiedNamesByArn(t *testing.T) {
	t.Parallel()

	b := codeconnections.NewInMemoryBackend("123456789012", "us-east-1")
	ctx := context.Background()

	arns := []string{
		"arn:aws:codeconnections:us-east-1:123456789012:host/c-third",
		"arn:aws:codeconnections:us-east-1:123456789012:host/b-second",
		"arn:aws:codeconnections:us-east-1:123456789012:host/a-first",
	}

	for _, hostArn := range arns {
		b.AddHostInternal(ctx, &codeconnections.Host{
			HostArn:          hostArn,
			Name:             "dup-name",
			ProviderType:     "GitHub",
			ProviderEndpoint: "https://example.com",
			Status:           "AVAILABLE",
		})
	}

	hosts := b.ListHosts(ctx)
	require.Len(t, hosts, 3)

	gotArns := make([]string, 0, len(hosts))
	for _, h := range hosts {
		gotArns = append(gotArns, h.HostArn)
	}

	wantArns := append([]string(nil), arns...)
	sort.Strings(wantArns)

	assert.Equal(t, wantArns, gotArns)
}

// TestBackendHostsByNameDeleteRestores verifies delete releases the name for reuse.
func TestBackendHostsByNameDeleteRestores(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "name_reusable_after_delete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := codeconnections.NewInMemoryBackend("123456789012", "us-east-1")
			host, err := b.CreateHost(
				context.Background(),
				"recycled-host",
				"GitHubEnterpriseServer",
				"https://a.example.com",
				nil,
				nil,
			)
			require.NoError(t, err)

			err = b.DeleteHost(context.Background(), host.HostArn)
			require.NoError(t, err)

			_, err = b.CreateHost(
				context.Background(),
				"recycled-host",
				"GitHubEnterpriseServer",
				"https://b.example.com",
				nil,
				nil,
			)
			require.NoError(t, err, "name should be reusable after delete")
		})
	}
}

// TestBackendAddHostInternalThenCreateSameName verifies AddHostInternal does
// not block a later CreateHost call for the same Name -- CreateHost has no
// ResourceAlreadyExistsException in its real error list (see
// TestCreateHostNameNotUnique), so duplicate names must never be rejected.
func TestBackendAddHostInternalThenCreateSameName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "internal_add_does_not_block_duplicate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := codeconnections.NewInMemoryBackend("123456789012", "us-east-1")
			b.AddHostInternal(context.Background(), &codeconnections.Host{
				Name:             "seeded-host",
				HostArn:          "arn:aws:codeconnections:us-east-1:123:host/seeded",
				ProviderType:     "GitHubEnterpriseServer",
				ProviderEndpoint: "https://ghe.example.com",
				Status:           "AVAILABLE",
				Tags:             map[string]string{},
			})

			_, err := b.CreateHost(
				context.Background(),
				"seeded-host",
				"GitHubEnterpriseServer",
				"https://other.example.com",
				nil,
				nil,
			)
			require.NoError(t, err)
		})
	}
}
