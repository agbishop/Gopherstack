package codestarconnections_test

import (
	"net/http"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codestarconnections"
)

func TestCreateHost_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "valid host",
			body: map[string]any{
				"Name":             "my-ghe-host",
				"ProviderType":     "GitHubEnterpriseServer",
				"ProviderEndpoint": "https://ghe.example.com",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing provider endpoint",
			body: map[string]any{
				"Name":         "no-ep-host",
				"ProviderType": "GitHubEnterpriseServer",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// Real HostName shape (botocore codestar-connections/2019-12-01/
			// service-2.json) has max 64, NOT the 32-char ConnectionName
			// limit -- 33 chars is a valid host name.
			name: "name at connection-name length is still valid for a host",
			body: map[string]any{
				"Name":             repeatStr("h", 33),
				"ProviderType":     "GitHubEnterpriseServer",
				"ProviderEndpoint": "https://x.com",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "name too long (65 chars, over the real 64-char HostName max)",
			body: map[string]any{
				"Name":             repeatStr("h", 65),
				"ProviderType":     "GitHubEnterpriseServer",
				"ProviderEndpoint": "https://x.com",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// Real HostName shape has pattern ".*" -- any character is
			// valid, not just [a-zA-Z0-9_.-].
			name: "name with special chars is valid",
			body: map[string]any{
				"Name":             "my host!",
				"ProviderType":     "GitHubEnterpriseServer",
				"ProviderEndpoint": "https://x.com",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "invalid provider type for host",
			body: map[string]any{
				"Name":             "bad-pt-host",
				"ProviderType":     "NOTVALID",
				"ProviderEndpoint": "https://x.com",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateHost", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestDeleteHost_ResourceInUse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantErrType string
		wantStatus  int
		setupConn   bool
	}{
		{
			name:       "delete host without connections succeeds",
			setupConn:  false,
			wantStatus: http.StatusOK,
		},
		{
			name:        "delete host with active connection fails",
			setupConn:   true,
			wantStatus:  http.StatusBadRequest,
			wantErrType: "ResourceUnavailableException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			hostArn := createCSCHost(t, h, "deletable-host", "GitHubEnterpriseServer", "https://ghe.example.com")

			if tt.setupConn {
				rec := doRequest(t, h, "CreateConnection", map[string]any{
					"ConnectionName": "uses-host-conn",
					"ProviderType":   "GitHubEnterpriseServer",
					"HostArn":        hostArn,
				})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doRequest(t, h, "DeleteHost", map[string]any{"HostArn": hostArn})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantErrType != "" {
				resp := parseResp(t, rec)
				assert.Equal(t, tt.wantErrType, resp["__type"])
			}
		})
	}
}

func TestDeleteHost_AfterDeletingConnections(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	hostArn := createCSCHost(t, h, "detach-host", "GitHubEnterpriseServer", "https://ghe.example.com")
	connArn := createCSCConn(t, h, "host-conn", "GitHubEnterpriseServer")

	// Update connection to reference the host (simulate via backend).
	h.Backend.AddConnectionInternal(&codestarconnections.Connection{
		ConnectionName:   "host-ref-conn",
		ConnectionArn:    "arn:aws:codestar-connections:us-east-1:000000000000:connection/hostref",
		ConnectionStatus: codestarconnections.ConnectionStatusAvailable,
		OwnerAccountID:   "000000000000",
		ProviderType:     "GitHubEnterpriseServer",
		HostArn:          hostArn,
	})

	// Can't delete host while connection references it.
	rec1 := doRequest(t, h, "DeleteHost", map[string]any{"HostArn": hostArn})
	assert.Equal(t, http.StatusBadRequest, rec1.Code)

	// Delete the referencing connection, then the host becomes deletable.
	rec2 := doRequest(t, h, "DeleteConnection", map[string]any{
		"ConnectionArn": "arn:aws:codestar-connections:us-east-1:000000000000:connection/hostref",
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	// Also delete the other connection we created (it doesn't reference the host).
	rec3 := doRequest(t, h, "DeleteConnection", map[string]any{"ConnectionArn": connArn})
	require.Equal(t, http.StatusOK, rec3.Code)

	rec4 := doRequest(t, h, "DeleteHost", map[string]any{"HostArn": hostArn})
	assert.Equal(t, http.StatusOK, rec4.Code)
}

func TestListHosts_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 4 {
		createCSCHost(t, h, "pag-host-"+string(rune('a'+i)), "GitHubEnterpriseServer",
			"https://ghe"+string(rune('a'+i))+".example.com")
	}

	rec1 := doRequest(t, h, "ListHosts", map[string]any{"MaxResults": 2})
	require.Equal(t, http.StatusOK, rec1.Code)
	resp1 := parseResp(t, rec1)
	hosts1, ok := resp1["Hosts"].([]any)
	require.True(t, ok)
	assert.Len(t, hosts1, 2)

	nextToken, hasNext := resp1["NextToken"].(string)
	assert.True(t, hasNext && nextToken != "")

	rec2 := doRequest(t, h, "ListHosts", map[string]any{
		"MaxResults": 2,
		"NextToken":  nextToken,
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	resp2 := parseResp(t, rec2)
	hosts2, ok := resp2["Hosts"].([]any)
	require.True(t, ok)
	assert.Len(t, hosts2, 2)
	assert.Empty(t, resp2["NextToken"])
}

func TestUpdateHost_ProviderEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		newEndpoint string
		wantOK      bool
	}{
		{
			name:        "update endpoint succeeds",
			newEndpoint: "https://new-ghe.example.com",
			wantOK:      true,
		},
		{
			name:        "empty endpoint is no-op",
			newEndpoint: "",
			wantOK:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			hostArn := createCSCHost(t, h, "upd-ep-host", "GitHubEnterpriseServer", "https://old.example.com")

			rec := doRequest(t, h, "UpdateHost", map[string]any{
				"HostArn":          hostArn,
				"ProviderEndpoint": tt.newEndpoint,
			})
			assert.Equal(t, http.StatusOK, rec.Code)

			// Verify the update by getting the host.
			if tt.newEndpoint != "" {
				getRec := doRequest(t, h, "GetHost", map[string]any{"HostArn": hostArn})
				require.Equal(t, http.StatusOK, getRec.Code)
				resp := parseResp(t, getRec)
				assert.Equal(t, tt.newEndpoint, resp["ProviderEndpoint"])
			}
		})
	}
}

func TestListHosts_EmptyIsArray(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "ListHosts", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	hosts, ok := resp["Hosts"].([]any)
	require.True(t, ok, "Hosts should be an array, not null")
	assert.Empty(t, hosts)
}

func TestCreateHost_TagsRoundtrip(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateHost", map[string]any{
		"Name":             "tagged-host",
		"ProviderType":     "GitHubEnterpriseServer",
		"ProviderEndpoint": "https://ghe.example.com",
		"Tags": []map[string]string{
			{"Key": "cost-center", "Value": "engineering"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	tags, ok := resp["Tags"].([]any)
	require.True(t, ok, "CreateHost must echo Tags in response (real CreateHostOutput.Tags)")
	require.Len(t, tags, 1)
	assert.Equal(t, "cost-center", tags[0].(map[string]any)["Key"])

	hostArn := resp["HostArn"].(string)
	recTags := doRequest(t, h, "ListTagsForResource", map[string]any{"ResourceArn": hostArn})
	require.Equal(t, http.StatusOK, recTags.Code)

	tagsResp := parseResp(t, recTags)
	listTags := tagsResp["Tags"].([]any)
	require.Len(t, listTags, 1)
	assert.Equal(t, "cost-center", listTags[0].(map[string]any)["Key"])
}

func TestHost_StatusAvailableOnCreate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	hostArn := createCSCHost(t, h, "status-avail-host", "GitHubEnterpriseServer", "https://ghe.example.com")

	rec := doRequest(t, h, "GetHost", map[string]any{"HostArn": hostArn})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	assert.Equal(t, "PENDING", resp["Status"])
}

func TestCreateHost_AllProviderTypes(t *testing.T) {
	t.Parallel()

	// All provider types should be accepted for hosts.
	types := []string{"Bitbucket", "GitHub", "GitHubEnterpriseServer", "GitLab", "GitLabSelfManaged"}

	for i, pt := range types {
		t.Run(pt, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateHost", map[string]any{
				"Name":             "host-" + pt + string(rune('0'+i)),
				"ProviderType":     pt,
				"ProviderEndpoint": "https://x.com",
			})
			assert.Equal(t, http.StatusOK, rec.Code, "provider type %q should be accepted", pt)
		})
	}
}

// TestGetHost_OmitsStatusMessage verifies GetHost's response never includes
// a StatusMessage field, while ListHosts' per-item shape does. Confirmed
// against aws-sdk-go-v2's generated GetHostOutput struct and its
// deserializer (awsAwsjson10_deserializeOpDocumentGetHostOutput): unlike
// types.Host (used by ListHosts), GetHostOutput has no StatusMessage member
// at all -- this is a genuine real-API asymmetry, not a gopherstack gap.
func TestGetHost_OmitsStatusMessage(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	const hostArn = "arn:aws:codestar-connections:us-east-1:000000000000:host/status-msg-host/abc"

	h.Backend.AddHostInternal(&codestarconnections.Host{
		HostArn:          hostArn,
		Name:             "status-msg-host",
		ProviderType:     "GitHubEnterpriseServer",
		ProviderEndpoint: "https://ghe.example.com",
		Status:           "VPC_CONFIG_FAILED",
		StatusMessage:    "VPC configuration failed: subnet unreachable",
	})

	getRec := doRequest(t, h, "GetHost", map[string]any{"HostArn": hostArn})
	require.Equal(t, http.StatusOK, getRec.Code)

	getResp := parseResp(t, getRec)
	_, hasStatusMessage := getResp["StatusMessage"]
	assert.False(t, hasStatusMessage, "GetHost must not include StatusMessage in response")

	listRec := doRequest(t, h, "ListHosts", map[string]any{})
	require.Equal(t, http.StatusOK, listRec.Code)

	listResp := parseResp(t, listRec)
	hosts, ok := listResp["Hosts"].([]any)
	require.True(t, ok)
	require.Len(t, hosts, 1)
	assert.Equal(t, "VPC configuration failed: subnet unreachable", hosts[0].(map[string]any)["StatusMessage"])
}

// TestGetHost_IncludesHostArn verifies GetHost does NOT include HostArn (real AWS omits it).
// HostArn is only present in ListHosts items.
func TestGetHost_IncludesHostArn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		hostName     string
		providerType string
		endpoint     string
	}{
		{
			name:         "host_arn_not_in_get_response",
			hostName:     "my-ghe-host",
			providerType: "GitHubEnterpriseServer",
			endpoint:     "https://ghe.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			hostArn := createCSCHost(t, h, tt.hostName, tt.providerType, tt.endpoint)

			rec := doRequest(t, h, "GetHost", map[string]any{"HostArn": hostArn})
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			_, hasHostArn := resp["HostArn"]
			assert.False(t, hasHostArn, "GetHost must not include HostArn in response")
			assert.Equal(t, tt.hostName, resp["Name"])
			assert.Equal(t, tt.endpoint, resp["ProviderEndpoint"])
			assert.Equal(t, tt.providerType, resp["ProviderType"])
		})
	}
}

// TestListHosts_Sorted verifies hosts are returned sorted by name.
func TestListHosts_Sorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		hostNames []string
		wantOrder []string
	}{
		{
			name:      "sorted_alpha",
			hostNames: []string{"zulu-host", "alpha-host", "mike-host"},
			wantOrder: []string{"alpha-host", "mike-host", "zulu-host"},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			for j, n := range tt.hostNames {
				createCSCHost(t, h, n, "GitHubEnterpriseServer",
					"https://ghe"+strconv.Itoa(i)+strconv.Itoa(j)+".example.com")
			}

			rec := doRequest(t, h, "ListHosts", map[string]any{})
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			hosts, ok := resp["Hosts"].([]any)
			require.True(t, ok)
			require.Len(t, hosts, len(tt.wantOrder))

			for k, wantName := range tt.wantOrder {
				hostMap, isMap := hosts[k].(map[string]any)
				require.True(t, isMap)
				assert.Equal(t, wantName, hostMap["Name"])
			}
		})
	}
}

// TestListHosts_OrdersTiedNamesByArn verifies that when two hosts share a
// Name (allowed -- CreateHost has no ResourceAlreadyExistsException for a
// duplicate name, see errors.go), ListHosts still returns a deterministic
// total order between them, broken by HostArn. Hosts are seeded via
// AddHostInternal in descending-ARN order: without a secondary sort key,
// tied-name hosts are left in insertion order, the reverse of the ascending
// order this test asserts.
func TestListHosts_OrdersTiedNamesByArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	arns := []string{
		"arn:aws:codestar-connections:us-east-1:000000000000:host/c-third",
		"arn:aws:codestar-connections:us-east-1:000000000000:host/b-second",
		"arn:aws:codestar-connections:us-east-1:000000000000:host/a-first",
	}

	for _, hostArn := range arns {
		h.Backend.AddHostInternal(&codestarconnections.Host{
			HostArn:          hostArn,
			Name:             "dup-name",
			ProviderType:     "GitHubEnterpriseServer",
			ProviderEndpoint: "https://ghe.example.com",
			Status:           "AVAILABLE",
		})
	}

	rec := doRequest(t, h, "ListHosts", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	hosts, ok := resp["Hosts"].([]any)
	require.True(t, ok)
	require.Len(t, hosts, 3)

	gotArns := make([]string, 0, len(hosts))
	for _, hh := range hosts {
		item, isMap := hh.(map[string]any)
		require.True(t, isMap)
		gotArns = append(gotArns, item["HostArn"].(string))
	}

	wantArns := append([]string(nil), arns...)
	sort.Strings(wantArns)

	assert.Equal(t, wantArns, gotArns)
}
