package codeconnections_test

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codeconnections"
)

// TestCreateConnection exercises the CreateConnection handler.
func TestCreateConnection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
		wantArn    bool
	}{
		{
			name: "success",
			body: map[string]any{
				"ConnectionName": "my-conn",
				"ProviderType":   "GitHub",
				"Tags":           []map[string]string{{"Key": "Env", "Value": "test"}},
			},
			wantStatus: http.StatusOK,
			wantArn:    true,
		},
		{
			name:       "missing_name",
			body:       map[string]any{"ProviderType": "GitHub"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty_body",
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			rec := doJSON(t, h, "CreateConnection", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantArn {
				resp := parseResp(t, rec)
				assert.NotEmpty(t, resp["ConnectionArn"])
			}
		})
	}
}

// TestGetConnection exercises the GetConnection handler.
func TestGetConnection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup       func(t *testing.T, h *codeconnections.Handler) string
		name        string
		wantName    string
		wantType    string
		wantStatus2 string
		wantStatus  int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *codeconnections.Handler) string {
				t.Helper()

				return createConn(t, h, "my-conn", "GitHub")
			},
			wantStatus:  http.StatusOK,
			wantName:    "my-conn",
			wantType:    "GitHub",
			wantStatus2: "AVAILABLE",
		},
		{
			name: "not_found",
			setup: func(_ *testing.T, _ *codeconnections.Handler) string {
				return "arn:aws:codeconnections:us-east-1:123:connection/nonexistent"
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			connArn := tt.setup(t, h)
			rec := doJSON(t, h, "GetConnection", map[string]any{"ConnectionArn": connArn})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				resp := parseResp(t, rec)
				conn, ok := resp["Connection"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantName, conn["ConnectionName"])
				assert.Equal(t, tt.wantType, conn["ProviderType"])
				assert.Equal(t, tt.wantStatus2, conn["ConnectionStatus"])
				assert.Equal(t, "123456789012", conn["OwnerAccountId"])
				assert.NotEmpty(t, conn["ConnectionArn"])
			}
		})
	}
}

// TestListConnections exercises the ListConnections handler.
func TestListConnections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *codeconnections.Handler)
		body       map[string]any
		name       string
		wantStatus int
		wantCount  int
	}{
		{
			name:       "empty_list",
			setup:      func(_ *testing.T, _ *codeconnections.Handler) {},
			body:       map[string]any{},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "multiple_connections",
			setup: func(t *testing.T, h *codeconnections.Handler) {
				t.Helper()
				createConn(t, h, "conn1", "GitHub")
				createConn(t, h, "conn2", "GitLab")
			},
			body:       map[string]any{},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name: "filtered_by_provider_type",
			setup: func(t *testing.T, h *codeconnections.Handler) {
				t.Helper()
				createConn(t, h, "conn1", "GitHub")
				createConn(t, h, "conn2", "GitLab")
			},
			body:       map[string]any{"ProviderTypeFilter": "GitHub"},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			tt.setup(t, h)

			rec := doJSON(t, h, "ListConnections", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			resp := parseResp(t, rec)
			conns, ok := resp["Connections"].([]any)
			require.True(t, ok)
			assert.Len(t, conns, tt.wantCount)
		})
	}
}

// TestDeleteConnection exercises the DeleteConnection handler.
func TestDeleteConnection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *codeconnections.Handler) string
		name       string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *codeconnections.Handler) string {
				t.Helper()

				return createConn(t, h, "my-conn", "GitHub")
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "not_found",
			setup: func(_ *testing.T, _ *codeconnections.Handler) string {
				return "arn:aws:codeconnections:us-east-1:123:connection/nonexistent"
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			connArn := tt.setup(t, h)
			rec := doJSON(t, h, "DeleteConnection", map[string]any{"ConnectionArn": connArn})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				getRec := doJSON(t, h, "GetConnection", map[string]any{"ConnectionArn": connArn})
				assert.Equal(t, http.StatusBadRequest, getRec.Code)
			}
		})
	}
}

// TestBackendListConnections exercises ListConnections filtering directly.
func TestBackendListConnections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, b *codeconnections.InMemoryBackend)
		name         string
		filter       string
		wantProvider string
		wantCount    int
	}{
		{
			name: "no_filter_returns_all",
			setup: func(t *testing.T, b *codeconnections.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateConnection(context.Background(), "c1", "GitHub", "", nil)
				require.NoError(t, err)
				_, err = b.CreateConnection(context.Background(), "c2", "GitLab", "", nil)
				require.NoError(t, err)
			},
			filter:    "",
			wantCount: 2,
		},
		{
			name: "filter_by_provider",
			setup: func(t *testing.T, b *codeconnections.InMemoryBackend) {
				t.Helper()
				_, err := b.CreateConnection(context.Background(), "c1", "GitHub", "", nil)
				require.NoError(t, err)
				_, err = b.CreateConnection(context.Background(), "c2", "GitLab", "", nil)
				require.NoError(t, err)
			},
			filter:       "GitHub",
			wantCount:    1,
			wantProvider: "GitHub",
		},
		{
			name:      "empty_backend",
			setup:     func(_ *testing.T, _ *codeconnections.InMemoryBackend) {},
			filter:    "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := codeconnections.NewInMemoryBackend("123456789012", "us-east-1")
			tt.setup(t, b)
			conns := b.ListConnections(context.Background(), tt.filter, "")
			assert.Len(t, conns, tt.wantCount)

			if tt.wantProvider != "" {
				for _, c := range conns {
					assert.Equal(t, tt.wantProvider, c.ProviderType)
				}
			}
		})
	}
}

// TestBackendNotFoundErrors exercises not-found error paths in backend methods.
func TestBackendNotFoundErrors(t *testing.T) {
	t.Parallel()

	const missingArn = "arn:aws:codeconnections:us-east-1:123:connection/missing"

	tests := []struct {
		call    func(b *codeconnections.InMemoryBackend) error
		name    string
		wantErr bool
	}{
		{
			name:    "GetConnection_not_found",
			wantErr: true,
			call: func(b *codeconnections.InMemoryBackend) error {
				_, err := b.GetConnection(context.Background(), missingArn)

				return err
			},
		},
		{
			name:    "DeleteConnection_not_found",
			wantErr: true,
			call: func(b *codeconnections.InMemoryBackend) error {
				return b.DeleteConnection(context.Background(), missingArn)
			},
		},
		{
			name:    "TagResource_not_found",
			wantErr: true,
			call: func(b *codeconnections.InMemoryBackend) error {
				return b.TagResource(context.Background(), missingArn, map[string]string{"k": "v"})
			},
		},
		{
			name:    "UntagResource_not_found",
			wantErr: true,
			call: func(b *codeconnections.InMemoryBackend) error {
				return b.UntagResource(context.Background(), missingArn, []string{"k"})
			},
		},
		{
			name:    "ListTagsForResource_not_found",
			wantErr: true,
			call: func(b *codeconnections.InMemoryBackend) error {
				_, err := b.ListTagsForResource(context.Background(), missingArn)

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := codeconnections.NewInMemoryBackend("123456789012", "us-east-1")
			err := tt.call(b)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestBackendCreateAndGet exercises happy-path create and get.
func TestBackendCreateAndGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		inputTags    map[string]string
		name         string
		connName     string
		providerType string
		wantStatus   string
	}{
		{
			name:         "github_connection",
			connName:     "my-conn",
			providerType: "GitHub",
			inputTags:    map[string]string{"Env": "prod"},
			wantStatus:   "AVAILABLE",
		},
		{
			name:         "gitlab_connection_no_tags",
			connName:     "gl-conn",
			providerType: "GitLab",
			wantStatus:   "AVAILABLE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := codeconnections.NewInMemoryBackend("123456789012", "us-east-1")
			conn, err := b.CreateConnection(
				context.Background(),
				tt.connName,
				tt.providerType,
				"",
				tt.inputTags,
			)
			require.NoError(t, err)
			assert.NotEmpty(t, conn.ConnectionArn)
			assert.Equal(t, tt.connName, conn.ConnectionName)
			assert.Equal(t, tt.providerType, conn.ProviderType)
			assert.Equal(t, tt.wantStatus, conn.Status)
			assert.Equal(t, "123456789012", conn.OwnerAccountID)
			assert.Contains(
				t,
				conn.ConnectionArn,
				"arn:aws:codeconnections:us-east-1:123456789012:connection/",
			)

			got, err := b.GetConnection(context.Background(), conn.ConnectionArn)
			require.NoError(t, err)
			assert.Equal(t, conn.ConnectionArn, got.ConnectionArn)
		})
	}
}

// TestListConnectionsPagination verifies NextToken/MaxResults pagination for ListConnections.
func TestListConnectionsPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		count      int
		maxResults int
		wantCount  int
		wantToken  bool
	}{
		{
			name:       "first_page_limited",
			count:      3,
			maxResults: 2,
			wantCount:  2,
			wantToken:  true,
		},
		{
			name:       "all_results_no_token",
			count:      2,
			maxResults: 10,
			wantCount:  2,
			wantToken:  false,
		},
		{
			name:       "zero_max_uses_default",
			count:      2,
			maxResults: 0,
			wantCount:  2,
			wantToken:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			for i := range tt.count {
				createConn(t, h, "conn-"+strconv.Itoa(i), "GitHub")
			}

			body := map[string]any{}
			if tt.maxResults > 0 {
				body["MaxResults"] = tt.maxResults
			}

			rec := doJSON(t, h, "ListConnections", body)
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			conns, ok := resp["Connections"].([]any)
			require.True(t, ok)
			assert.Len(t, conns, tt.wantCount)

			_, hasToken := resp["NextToken"]
			assert.Equal(t, tt.wantToken, hasToken)
		})
	}
}

// TestListConnectionsOrdersTiedNamesByArn verifies that when two connections
// share a ConnectionName (allowed -- CreateConnection has no
// ResourceAlreadyExistsException for a duplicate name, see connections.go),
// ListConnections still returns a deterministic total order between them,
// broken by ConnectionArn. Connections are seeded via AddConnectionInternal
// in descending-ARN insertion order: without a secondary sort key, tied-name
// connections are left in insertion order, the reverse of the ascending-ARN
// order this test asserts.
func TestListConnectionsOrdersTiedNamesByArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	arns := []string{
		"arn:aws:codeconnections:us-east-1:123456789012:connection/c-third",
		"arn:aws:codeconnections:us-east-1:123456789012:connection/b-second",
		"arn:aws:codeconnections:us-east-1:123456789012:connection/a-first",
	}

	for _, connArn := range arns {
		h.Backend.AddConnectionInternal(context.Background(), &codeconnections.Connection{
			ConnectionArn:  connArn,
			ConnectionName: "dup-name",
			ProviderType:   "GitHub",
			Status:         "AVAILABLE",
			OwnerAccountID: "123456789012",
		})
	}

	rec := doJSON(t, h, "ListConnections", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	conns, ok := resp["Connections"].([]any)
	require.True(t, ok)
	require.Len(t, conns, 3)

	gotArns := make([]string, 0, len(conns))
	for _, c := range conns {
		item, isMap := c.(map[string]any)
		require.True(t, isMap)
		gotArns = append(gotArns, item["ConnectionArn"].(string))
	}

	wantArns := append([]string(nil), arns...)
	sort.Strings(wantArns)

	assert.Equal(t, wantArns, gotArns)
}

// TestListConnectionsContinuation verifies two-page traversal using NextToken.
func TestListConnectionsContinuation(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	for i := range 3 {
		createConn(t, h, "conn-"+strconv.Itoa(i), "GitHub")
	}

	// First page: 2 of 3.
	rec1 := doJSON(t, h, "ListConnections", map[string]any{"MaxResults": 2})
	require.Equal(t, http.StatusOK, rec1.Code)
	resp1 := parseResp(t, rec1)
	page1, ok := resp1["Connections"].([]any)
	require.True(t, ok)
	assert.Len(t, page1, 2)

	nextToken, hasToken := resp1["NextToken"].(string)
	require.True(t, hasToken, "expected NextToken in first page response")
	require.NotEmpty(t, nextToken)

	// Second page: remaining 1.
	rec2 := doJSON(t, h, "ListConnections", map[string]any{
		"MaxResults": 2,
		"NextToken":  nextToken,
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	resp2 := parseResp(t, rec2)
	page2, ok := resp2["Connections"].([]any)
	require.True(t, ok)
	assert.Len(t, page2, 1)

	_, stillHasToken := resp2["NextToken"]
	assert.False(t, stillHasToken, "last page should have no NextToken")

	// Collectively all connection names present.
	names := make([]string, 0, 3)
	for _, item := range append(page1, page2...) {
		conn := item.(map[string]any)
		names = append(names, conn["ConnectionName"].(string))
	}
	assert.ElementsMatch(t, []string{"conn-0", "conn-1", "conn-2"}, names)
}

// TestConnectionArnFormat verifies CreateConnection's ConnectionArn has the expected shape.
func TestConnectionArnFormat(t *testing.T) {
	t.Parallel()

	h := newHandlerFixedAccount(t)
	rec := doJSON(t, h, "CreateConnection", map[string]any{
		"ConnectionName": "arn-test",
		"ProviderType":   "GitHub",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	connArn, _ := m["ConnectionArn"].(string)
	assert.True(
		t,
		strings.HasPrefix(connArn, ccFixedArnPrefix+"connection/"),
		"ConnectionArn should start with %s, got %s",
		ccFixedArnPrefix+"connection/",
		connArn,
	)
}

// TestConnectionCreate exercises CreateConnection with tags and duplicate/validation error cases.
func TestConnectionCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		check    func(t *testing.T, m map[string]any)
		name     string
		wantCode int
	}{
		{
			name: "creates with GitHub provider",
			body: map[string]any{
				"ConnectionName": "gh-conn",
				"ProviderType":   "GitHub",
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, m map[string]any) {
				t.Helper()

				assert.NotEmpty(t, m["ConnectionArn"])
			},
		},
		{
			name: "creates with tags",
			body: map[string]any{
				"ConnectionName": "tagged-conn",
				"ProviderType":   "Bitbucket",
				"Tags": []map[string]any{
					{"Key": "env", "Value": "prod"},
				},
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, m map[string]any) {
				t.Helper()

				tags, _ := m["Tags"].([]any)
				assert.Len(t, tags, 1)
			},
		},
		{
			// CreateConnection has no ResourceAlreadyExistsException in its
			// real error list (see TestConnectionNameNotUnique), so a
			// second create with the same name must also succeed.
			name:     "duplicate name also succeeds",
			body:     map[string]any{"ConnectionName": "dup-conn", "ProviderType": "GitHub"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, m map[string]any) {
				t.Helper()

				assert.NotEmpty(t, m["ConnectionArn"])
			},
		},
		{
			name:     "missing provider type returns error",
			body:     map[string]any{"ConnectionName": "no-provider"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerFixedAccount(t)

			if tt.name == "duplicate name also succeeds" {
				rec := doJSON(t, h, "CreateConnection", tt.body)
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doJSON(t, h, "CreateConnection", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.check != nil {
				m := parseResp(t, rec)
				tt.check(t, m)
			}
		})
	}
}

// TestConnectionGet exercises GetConnection field shapes and not-found handling.
func TestConnectionGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check    func(t *testing.T, m map[string]any)
		name     string
		wantCode int
		preload  bool
	}{
		{
			name:     "returns connection fields",
			wantCode: http.StatusOK,
			preload:  true,
			check: func(t *testing.T, m map[string]any) {
				t.Helper()

				conn, _ := m["Connection"].(map[string]any)
				require.NotNil(t, conn)
				assert.Equal(t, "get-test", conn["ConnectionName"])
				assert.NotEmpty(t, conn["ConnectionArn"])
				assert.NotEmpty(t, conn["ProviderType"])
				assert.NotEmpty(t, conn["ConnectionStatus"])
				assert.NotEmpty(t, conn["OwnerAccountId"])
			},
		},
		{
			name:     "not found returns error",
			wantCode: http.StatusBadRequest,
			preload:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerFixedAccount(t)

			connArn := "arn:aws:codeconnections:us-east-1:000000000000:connection/nonexistent"
			if tt.preload {
				rec := doJSON(t, h, "CreateConnection", map[string]any{
					"ConnectionName": "get-test",
					"ProviderType":   "GitHub",
				})
				require.Equal(t, http.StatusOK, rec.Code)

				connArn = parseResp(t, rec)["ConnectionArn"].(string)
			}

			rec := doJSON(t, h, "GetConnection", map[string]any{"ConnectionArn": connArn})
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.check != nil {
				tt.check(t, parseResp(t, rec))
			}
		})
	}
}

// TestConnectionList exercises ListConnections' empty and populated cases.
func TestConnectionList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check    func(t *testing.T, m map[string]any)
		name     string
		wantCode int
		count    int
	}{
		{
			name:     "empty list returns Connections array",
			wantCode: http.StatusOK,
			count:    0,
			check: func(t *testing.T, m map[string]any) {
				t.Helper()

				list, _ := m["Connections"].([]any)
				assert.Empty(t, list)
			},
		},
		{
			name:     "returns all created connections",
			wantCode: http.StatusOK,
			count:    3,
			check: func(t *testing.T, m map[string]any) {
				t.Helper()

				list, _ := m["Connections"].([]any)
				assert.Len(t, list, 3)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerFixedAccount(t)

			for i := range tt.count {
				rec := doJSON(t, h, "CreateConnection", map[string]any{
					"ConnectionName": "list-conn-" + string(rune('a'+i)),
					"ProviderType":   "GitHub",
				})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doJSON(t, h, "ListConnections", map[string]any{})
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.check != nil {
				tt.check(t, parseResp(t, rec))
			}
		})
	}
}

// TestConnectionDelete exercises the full CreateConnection/DeleteConnection/GetConnection lifecycle.
func TestConnectionDelete(t *testing.T) {
	t.Parallel()

	h := newHandlerFixedAccount(t)

	rec := doJSON(t, h, "CreateConnection", map[string]any{
		"ConnectionName": "del-conn",
		"ProviderType":   "GitHub",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	connArn := parseResp(t, rec)["ConnectionArn"].(string)

	rec = doJSON(t, h, "DeleteConnection", map[string]any{"ConnectionArn": connArn})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doJSON(t, h, "GetConnection", map[string]any{"ConnectionArn": connArn})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestErrorPaths exercises not-found and validation error responses across Connection and Host ops.
func TestErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body        map[string]any
		wantErrType string
		name        string
		action      string
		wantCode    int
	}{
		{
			name:        "GetConnection not found",
			action:      "GetConnection",
			body:        map[string]any{"ConnectionArn": ccFixedArnPrefix + "connection/nonexistent"},
			wantCode:    http.StatusBadRequest,
			wantErrType: "ResourceNotFoundException",
		},
		{
			name:        "GetHost not found",
			action:      "GetHost",
			body:        map[string]any{"HostArn": ccFixedArnPrefix + "host/nonexistent"},
			wantCode:    http.StatusBadRequest,
			wantErrType: "ResourceNotFoundException",
		},
		{
			name:        "DeleteConnection not found",
			action:      "DeleteConnection",
			body:        map[string]any{"ConnectionArn": ccFixedArnPrefix + "connection/nonexistent"},
			wantCode:    http.StatusBadRequest,
			wantErrType: "ResourceNotFoundException",
		},
		{
			name:        "CreateConnection missing provider type",
			action:      "CreateConnection",
			body:        map[string]any{"ConnectionName": "bad-conn"},
			wantCode:    http.StatusBadRequest,
			wantErrType: "InvalidInputException",
		},
		{
			// GetHost declares ResourceNotFoundException, not
			// InvalidInputException; an absent HostArn (empty string) hits
			// the backend's own lookup-miss path (gopherstack-uox6
			// error-envelope sweep).
			name:        "GetHost missing HostArn",
			action:      "GetHost",
			body:        map[string]any{},
			wantCode:    http.StatusBadRequest,
			wantErrType: "ResourceNotFoundException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerFixedAccount(t)
			rec := doJSON(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			m := parseResp(t, rec)
			assert.Equal(t, tt.wantErrType, m["__type"], "error envelope __type mismatch")
			assert.NotEmpty(t, m["message"], "error envelope message should not be empty")
		})
	}
}

// TestValidProviderTypes_AzureDevOps verifies AzureDevOps is accepted as a real
// CodeConnections provider type (types.ProviderTypeAzureDevOps in
// aws-sdk-go-v2/service/codeconnections/types/enums.go) -- unlike the older
// CodeStarConnections service predating it.
func TestValidProviderTypes_AzureDevOps(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	b := newTestBackend()

	conn, err := b.CreateConnection(ctx, "azdo-conn", "AzureDevOps", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "AzureDevOps", conn.ProviderType)
}
