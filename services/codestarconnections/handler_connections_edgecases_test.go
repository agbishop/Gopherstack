package codestarconnections_test

import (
	"context"
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codestarconnections"
)

func TestConnectionName_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		connName   string
		wantStatus int
	}{
		{
			name:       "valid simple name",
			connName:   "my-conn",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid name with dots",
			connName:   "my.conn.1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid name with underscores",
			connName:   "my_conn_1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid max length name",
			connName:   "abcdefghijklmnopqrstuvwxyz123456",
			wantStatus: http.StatusOK,
		},
		{
			name:       "name too long (33 chars)",
			connName:   "abcdefghijklmnopqrstuvwxyz1234567",
			wantStatus: http.StatusBadRequest,
		},
		{
			// Real ConnectionName shape (botocore codestar-connections/
			// 2019-12-01/service-2.json) has pattern "[\s\S]*" -- i.e. any
			// character is valid, not just [a-zA-Z0-9_.-]. A space is
			// accepted.
			name:       "name with space is valid",
			connName:   "my conn",
			wantStatus: http.StatusOK,
		},
		{
			name:       "name with slash is valid",
			connName:   "my/conn",
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty name from body missing ConnectionName",
			connName:   "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := map[string]any{"ProviderType": "GitHub"}
			if tt.connName != "" {
				body["ConnectionName"] = tt.connName
			}

			rec := doRequest(t, h, "CreateConnection", body)
			assert.Equal(t, tt.wantStatus, rec.Code, "body=%v", body)
		})
	}
}

func TestProviderType_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		providerType string
		wantStatus   int
	}{
		{name: "GitHub", providerType: "GitHub", wantStatus: http.StatusOK},
		{name: "Bitbucket", providerType: "Bitbucket", wantStatus: http.StatusOK},
		{name: "GitLab", providerType: "GitLab", wantStatus: http.StatusOK},
		{name: "GitHubEnterpriseServer", providerType: "GitHubEnterpriseServer", wantStatus: http.StatusOK},
		{name: "GitLabSelfManaged", providerType: "GitLabSelfManaged", wantStatus: http.StatusOK},
		{name: "empty provider type is allowed", providerType: "", wantStatus: http.StatusOK},
		{name: "invalid provider type", providerType: "Subversion", wantStatus: http.StatusBadRequest},
		{name: "case sensitive invalid", providerType: "github", wantStatus: http.StatusBadRequest},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := map[string]any{
				"ConnectionName": "conn-" + tt.providerType + "-" + string(rune('0'+i)),
				"ProviderType":   tt.providerType,
			}
			if tt.providerType == "" {
				body["ConnectionName"] = "conn-empty-pt"
			}

			rec := doRequest(t, h, "CreateConnection", body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestListConnections_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 5 {
		createCSCConn(t, h, "pag-conn-"+string(rune('a'+i)), "GitHub")
	}

	// First page: MaxResults=3.
	rec1 := doRequest(t, h, "ListConnections", map[string]any{"MaxResults": 3})
	require.Equal(t, http.StatusOK, rec1.Code)
	resp1 := parseResp(t, rec1)
	conns1, ok := resp1["Connections"].([]any)
	require.True(t, ok)
	assert.Len(t, conns1, 3)
	nextToken, hasNext := resp1["NextToken"].(string)
	assert.True(t, hasNext && nextToken != "", "expected NextToken for first page")

	// Second page.
	rec2 := doRequest(t, h, "ListConnections", map[string]any{
		"MaxResults": 3,
		"NextToken":  nextToken,
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	resp2 := parseResp(t, rec2)
	conns2, ok := resp2["Connections"].([]any)
	require.True(t, ok)
	assert.Len(t, conns2, 2)
	assert.Empty(t, resp2["NextToken"], "no NextToken on last page")

	// Collect all names and verify they're the same set.
	allNames := make(map[string]bool)
	for _, c := range conns1 {
		cm := c.(map[string]any)
		allNames[cm["ConnectionName"].(string)] = true
	}

	for _, c := range conns2 {
		cm := c.(map[string]any)
		allNames[cm["ConnectionName"].(string)] = true
	}

	assert.Len(t, allNames, 5)
}

func TestConnection_HostArnOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	connArn := createCSCConn(t, h, "no-host-conn", "GitHub")

	rec := doRequest(t, h, "GetConnection", map[string]any{"ConnectionArn": connArn})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	conn := resp["Connection"].(map[string]any)
	_, hasHostArn := conn["HostArn"]
	assert.False(t, hasHostArn, "HostArn should be omitted when empty")
}

func TestConnection_HostArnIncludedWhenSet(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	// CreateConnection now validates HostArn against a real, previously-created
	// host (ResourceNotFoundException for an unknown one), so the host must
	// exist first -- see TestCreateConnection_HostArnNotFound.
	realHostArn := createCSCHost(t, h, "myhost", "GitHubEnterpriseServer", "https://ghe.example.com")

	rec := doRequest(t, h, "CreateConnection", map[string]any{
		"ConnectionName": "has-host-conn",
		"ProviderType":   "GitHubEnterpriseServer",
		"HostArn":        realHostArn,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResp(t, rec)
	connArn := resp["ConnectionArn"].(string)

	getRec := doRequest(t, h, "GetConnection", map[string]any{"ConnectionArn": connArn})
	require.Equal(t, http.StatusOK, getRec.Code)

	getResp := parseResp(t, getRec)
	conn := getResp["Connection"].(map[string]any)
	assert.Equal(t, realHostArn, conn["HostArn"])
}

func TestListConnections_EmptyIsArray(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "ListConnections", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	conns, ok := resp["Connections"].([]any)
	require.True(t, ok, "Connections should be an array, not null")
	assert.Empty(t, conns)
}

func TestCreateConnection_TagsRoundtrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateConnection", map[string]any{
		"ConnectionName": "tag-rt-conn",
		"ProviderType":   "GitHub",
		"Tags": []map[string]string{
			{"Key": "env", "Value": "prod"},
			{"Key": "team", "Value": "platform"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	tags, ok := resp["Tags"].([]any)
	require.True(t, ok, "CreateConnection must echo Tags in response (real CreateConnectionOutput.Tags)")
	require.Len(t, tags, 2)

	tag0 := tags[0].(map[string]any)
	tag1 := tags[1].(map[string]any)
	assert.Equal(t, "env", tag0["Key"])
	assert.Equal(t, "team", tag1["Key"])

	arn := resp["ConnectionArn"].(string)
	recTags := doRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": arn})
	require.Equal(t, http.StatusOK, recTags.Code)

	tagsResp := parseResp(t, recTags)
	listTags := tagsResp["Tags"].([]any)
	require.Len(t, listTags, 2)
}

func TestConnection_StatusAvailableOnCreate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	connArn := createCSCConn(t, h, "status-avail-conn", "GitHub")

	rec := doRequest(t, h, "GetConnection", map[string]any{"ConnectionArn": connArn})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	conn := resp["Connection"].(map[string]any)
	assert.Equal(t, "AVAILABLE", conn["ConnectionStatus"])
}

func TestPagination_NoNextTokenWhenFits(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 3 {
		createCSCConn(t, h, "fit-conn-"+string(rune('a'+i)), "GitHub")
	}

	// MaxResults=10 should show all 3 with no NextToken.
	rec := doRequest(t, h, "ListConnections", map[string]any{"MaxResults": 10})
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResp(t, rec)
	conns, ok := resp["Connections"].([]any)
	require.True(t, ok)
	assert.Len(t, conns, 3)
	assert.Empty(t, resp["NextToken"])
}

// TestCreateConnection_WithHostArn verifies HostArn is accepted.
func TestCreateConnection_WithHostArn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantArn  bool
		wantOK   bool
		withReal bool
	}{
		{
			name: "with_host_arn",
			body: map[string]any{
				"ConnectionName": "ghe-conn",
				"ProviderType":   "GitHubEnterpriseServer",
			},
			withReal: true,
			wantArn:  true,
			wantOK:   true,
		},
		{
			name: "without_host_arn",
			body: map[string]any{
				"ConnectionName": "gh-conn",
				"ProviderType":   "GitHub",
			},
			wantArn: true,
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.withReal {
				// CreateConnection now validates HostArn against a real,
				// previously-created host (ResourceNotFoundException for an
				// unknown one), so the host must exist first.
				tt.body["HostArn"] = createCSCHost(
					t, h, "ghe-conn-host", "GitHubEnterpriseServer", "https://ghe.example.com",
				)
			}

			rec := doRequest(t, h, "CreateConnection", tt.body)

			if tt.wantOK {
				require.Equal(t, http.StatusOK, rec.Code)
			}

			if tt.wantArn {
				resp := parseResp(t, rec)
				assert.NotEmpty(t, resp["ConnectionArn"])
			}
		})
	}
}

// TestCreateConnection_HostArnNotFound verifies that CreateConnection rejects
// a HostArn that does not reference an existing host. Real CreateConnection's
// error list is [LimitExceededException, ResourceNotFoundException,
// ResourceUnavailableException] (botocore codestar-connections/2019-12-01/
// service-2.json) -- a bad HostArn maps to ResourceNotFoundException, the
// same real type GetHost/DeleteHost use for a missing host.
func TestCreateConnection_HostArnNotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateConnection", map[string]any{
		"ConnectionName": "bad-host-conn",
		"ProviderType":   "GitHubEnterpriseServer",
		"HostArn":        "arn:aws:codestar-connections:us-east-1:000000000000:host/nonexistent/abc",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	resp := parseResp(t, rec)
	assert.Equal(t, "ResourceNotFoundException", resp["__type"])
}

// TestGetConnection_Fields verifies all expected fields are returned.
func TestGetConnection_Fields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		connName      string
		providerType  string
		wantStatus    string
		wantOwnerAcct string
	}{
		{
			name:          "github_connection_fields",
			connName:      "my-gh-conn",
			providerType:  "GitHub",
			wantStatus:    "AVAILABLE",
			wantOwnerAcct: "000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			connArn := createCSCConn(t, h, tt.connName, tt.providerType)

			rec := doRequest(t, h, "GetConnection", map[string]any{"ConnectionArn": connArn})
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			conn, ok := resp["Connection"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.connName, conn["ConnectionName"])
			assert.Equal(t, tt.providerType, conn["ProviderType"])
			assert.Equal(t, tt.wantStatus, conn["ConnectionStatus"])
			assert.Equal(t, tt.wantOwnerAcct, conn["OwnerAccountId"])
			assert.Equal(t, connArn, conn["ConnectionArn"])
		})
	}
}

// TestListConnections_Sorted verifies connections are returned sorted by name.
func TestListConnections_Sorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		connNames []string
		wantOrder []string
	}{
		{
			name:      "sorted_alpha",
			connNames: []string{"zebra-conn", "alpha-conn", "mango-conn"},
			wantOrder: []string{"alpha-conn", "mango-conn", "zebra-conn"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			for _, n := range tt.connNames {
				createCSCConn(t, h, n, "GitHub")
			}

			rec := doRequest(t, h, "ListConnections", map[string]any{})
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			conns, ok := resp["Connections"].([]any)
			require.True(t, ok)
			require.Len(t, conns, len(tt.wantOrder))

			for i, wantName := range tt.wantOrder {
				connMap, isMap := conns[i].(map[string]any)
				require.True(t, isMap)
				assert.Equal(t, wantName, connMap["ConnectionName"])
			}
		})
	}
}

// TestListConnections_OrdersTiedNamesByArn verifies that when two connections
// share a ConnectionName (allowed -- CreateConnection has no
// ResourceAlreadyExistsException for a duplicate name, see errors.go),
// ListConnections still returns a deterministic total order between them,
// broken by ConnectionArn. Connections are seeded via AddConnectionInternal
// in descending-ARN insertion order: without a secondary sort key, tied-name
// connections are left in insertion order, the reverse of the ascending-ARN
// order this test asserts.
func TestListConnections_OrdersTiedNamesByArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	arns := []string{
		"arn:aws:codestar-connections:us-east-1:000000000000:connection/c-third",
		"arn:aws:codestar-connections:us-east-1:000000000000:connection/b-second",
		"arn:aws:codestar-connections:us-east-1:000000000000:connection/a-first",
	}

	for _, connArn := range arns {
		h.Backend.AddConnectionInternal(&codestarconnections.Connection{
			ConnectionArn:    connArn,
			ConnectionName:   "dup-name",
			ProviderType:     "GitHub",
			ConnectionStatus: "AVAILABLE",
			OwnerAccountID:   "000000000000",
		})
	}

	rec := doRequest(t, h, "ListConnections", map[string]any{})
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

// TestListConnections_HostArnFilter verifies filtering by HostArn.
func TestListConnections_HostArnFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		applyFilter bool
		wantCount   int
	}{
		{name: "no_filter_returns_all", applyFilter: false, wantCount: 2},
		{name: "host_filter_returns_one", applyFilter: true, wantCount: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := codestarconnections.NewInMemoryBackend("000000000000", "us-east-1")
			h := codestarconnections.NewHandler(b)

			// CreateConnection now validates HostArn against a real,
			// previously-created host, so the host must exist first.
			host, err := b.CreateHost(
				context.Background(), "myghe", "GitHubEnterpriseServer", "https://ghe.example.com", nil, nil,
			)
			require.NoError(t, err)

			hostArn := host.HostArn

			_, err = b.CreateConnection(context.Background(), "ghe-conn", "GitHubEnterpriseServer", hostArn, nil)
			require.NoError(t, err)

			_, err = b.CreateConnection(context.Background(), "gh-conn", "GitHub", "", nil)
			require.NoError(t, err)

			body := map[string]any{}
			if tt.applyFilter {
				body["HostArnFilter"] = hostArn
			}

			rec := doRequest(t, h, "ListConnections", body)
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			conns, ok := resp["Connections"].([]any)
			require.True(t, ok)
			assert.Len(t, conns, tt.wantCount)
		})
	}
}
