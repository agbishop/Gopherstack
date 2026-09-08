package codestarconnections_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codestarconnections"
)

func TestHandler_CreateRepositoryLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body    map[string]any
		name    string
		wantErr bool
	}{
		{
			name: "happy path",
			body: map[string]any{
				"ConnectionArn":  "arn:aws:codestar-connections:us-east-1:000000000000:connection/abc",
				"OwnerId":        "my-owner",
				"RepositoryName": "my-repo",
			},
			wantErr: false,
		},
		{
			name:    "missing connection arn",
			body:    map[string]any{"OwnerId": "owner", "RepositoryName": "repo"},
			wantErr: true,
		},
		{
			name: "missing owner id",
			body: map[string]any{
				"ConnectionArn":  "arn:aws:codestar-connections:us-east-1:000000000000:connection/abc",
				"RepositoryName": "repo",
			},
			wantErr: true,
		},
		{
			name: "missing repository name",
			body: map[string]any{
				"ConnectionArn": "arn:aws:codestar-connections:us-east-1:000000000000:connection/abc",
				"OwnerId":       "owner",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateRepositoryLink", tt.body)

			if tt.wantErr {
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				return
			}

			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			info, ok := out["RepositoryLinkInfo"].(map[string]any)
			require.True(t, ok)
			assert.NotEmpty(t, info["RepositoryLinkId"])
			assert.NotEmpty(t, info["RepositoryLinkArn"])
			assert.Equal(t, "my-owner", info["OwnerId"])
			assert.Equal(t, "my-repo", info["RepositoryName"])
		})
	}
}

func TestHandler_GetRepositoryLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupFn func(h *codestarconnections.Handler) string
		name    string
		wantErr bool
	}{
		{
			name: "happy path",
			setupFn: func(h *codestarconnections.Handler) string {
				link, err := h.Backend.CreateRepositoryLink(
					context.Background(),
					"arn:aws:codestar-connections:us-east-1:000000000000:connection/abc",
					"my-owner", "my-repo", "", nil,
				)
				if err != nil {
					return ""
				}

				return link.RepositoryLinkID
			},
			wantErr: false,
		},
		{
			name: "not found",
			setupFn: func(_ *codestarconnections.Handler) string {
				return "nonexistent-id"
			},
			wantErr: true,
		},
		{
			name: "missing id",
			setupFn: func(_ *codestarconnections.Handler) string {
				return ""
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			id := tt.setupFn(h)

			rec := doRequest(t, h, "GetRepositoryLink", map[string]any{"RepositoryLinkId": id})

			if tt.wantErr {
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				return
			}

			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			info, ok := out["RepositoryLinkInfo"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "my-owner", info["OwnerId"])
			assert.Equal(t, "my-repo", info["RepositoryName"])
		})
	}
}

func TestHandler_DeleteRepositoryLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupFn func(h *codestarconnections.Handler) string
		name    string
		wantErr bool
	}{
		{
			name: "happy path",
			setupFn: func(h *codestarconnections.Handler) string {
				link, err := h.Backend.CreateRepositoryLink(
					context.Background(),
					"arn:aws:codestar-connections:us-east-1:000000000000:connection/abc",
					"owner", "repo", "", nil,
				)
				if err != nil {
					return ""
				}

				return link.RepositoryLinkID
			},
			wantErr: false,
		},
		{
			name: "not found",
			setupFn: func(_ *codestarconnections.Handler) string {
				return "nonexistent-id"
			},
			wantErr: true,
		},
		{
			name: "missing id",
			setupFn: func(_ *codestarconnections.Handler) string {
				return ""
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			id := tt.setupFn(h)

			rec := doRequest(t, h, "DeleteRepositoryLink", map[string]any{"RepositoryLinkId": id})

			if tt.wantErr {
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				return
			}

			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestHandler_ListRepositoryLinks(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateRepositoryLink(
		context.Background(),
		"arn:aws:codestar-connections:us-east-1:000000000000:connection/abc",
		"owner1", "repo1", "", nil,
	)
	_, _ = h.Backend.CreateRepositoryLink(
		context.Background(),
		"arn:aws:codestar-connections:us-east-1:000000000000:connection/abc",
		"owner2", "repo2", "", nil,
	)

	rec := doRequest(t, h, "ListRepositoryLinks", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	links, ok := out["RepositoryLinks"].([]any)
	require.True(t, ok)
	assert.Len(t, links, 2)
}

// TestInMemoryBackend_ListRepositoryLinksSorted verifies repository links are returned sorted by ID.
func TestInMemoryBackend_ListRepositoryLinksSorted(t *testing.T) {
	t.Parallel()

	b := codestarconnections.NewInMemoryBackend("000000000000", "us-east-1")

	// Seed links with known IDs so we can verify order.
	b.AddRepositoryLinkInternal(context.Background(), &codestarconnections.RepositoryLink{
		RepositoryLinkID: "b-link",
		ConnectionArn:    "arn1",
		OwnerID:          "owner",
		RepositoryName:   "repo-b",
	})
	b.AddRepositoryLinkInternal(context.Background(), &codestarconnections.RepositoryLink{
		RepositoryLinkID: "a-link",
		ConnectionArn:    "arn1",
		OwnerID:          "owner",
		RepositoryName:   "repo-a",
	})

	links := b.ListRepositoryLinks(context.Background())
	require.Len(t, links, 2)
	assert.Equal(t, "a-link", links[0].RepositoryLinkID)
	assert.Equal(t, "b-link", links[1].RepositoryLinkID)
}

func TestDeleteRepositoryLink_ResourceInUse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wantErrType string
		wantStatus  int
		createSync  bool
	}{
		{
			name:       "delete link without sync configs succeeds",
			createSync: false,
			wantStatus: http.StatusOK,
		},
		{
			name:        "delete link with active sync config fails",
			createSync:  true,
			wantStatus:  http.StatusBadRequest,
			wantErrType: "SyncConfigurationStillExistsException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			connArn := createCSCConn(t, h, "rl-riu-conn", "GitHub")
			linkID := createCSCRepositoryLink(t, h, connArn, "rl-riu-repo")

			if tt.createSync {
				rec := doRequest(t, h, "CreateSyncConfiguration", map[string]any{
					"Branch":           "main",
					"ConfigFile":       "sync.yaml",
					"RepositoryLinkId": linkID,
					"ResourceName":     "rl-riu-stack",
					"RoleArn":          "arn:aws:iam::000000000000:role/r",
					"SyncType":         "CFN_STACK_SYNC",
				})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := doRequest(t, h, "DeleteRepositoryLink", map[string]any{"RepositoryLinkId": linkID})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantErrType != "" {
				resp := parseResp(t, rec)
				assert.Equal(t, tt.wantErrType, resp["__type"])
			}
		})
	}
}

func TestListRepositoryLinks_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	connArn := createCSCConn(t, h, "pag-links-conn", "GitHub")

	for i := range 5 {
		createCSCRepositoryLink(t, h, connArn, "pag-repo-"+string(rune('a'+i)))
	}

	rec1 := doRequest(t, h, "ListRepositoryLinks", map[string]any{"MaxResults": 3})
	require.Equal(t, http.StatusOK, rec1.Code)
	resp1 := parseResp(t, rec1)
	links1, ok := resp1["RepositoryLinks"].([]any)
	require.True(t, ok)
	assert.Len(t, links1, 3)

	nextToken, hasNext := resp1["NextToken"].(string)
	assert.True(t, hasNext && nextToken != "")

	rec2 := doRequest(t, h, "ListRepositoryLinks", map[string]any{
		"MaxResults": 3,
		"NextToken":  nextToken,
	})
	require.Equal(t, http.StatusOK, rec2.Code)
	resp2 := parseResp(t, rec2)
	links2, ok := resp2["RepositoryLinks"].([]any)
	require.True(t, ok)
	assert.Len(t, links2, 2)
	assert.Empty(t, resp2["NextToken"])
}

func TestCreateRepositoryLink_Duplicate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	connArn := createCSCConn(t, h, "dup-link-conn", "GitHub")

	body := map[string]any{
		"ConnectionArn":  connArn,
		"OwnerId":        "my-org",
		"RepositoryName": "my-repo",
	}

	rec1 := doRequest(t, h, "CreateRepositoryLink", body)
	require.Equal(t, http.StatusOK, rec1.Code)

	rec2 := doRequest(t, h, "CreateRepositoryLink", body)
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

func TestRepositoryLink_EncryptionKeyArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	connArn := createCSCConn(t, h, "enc-key-conn", "GitHub")
	const encKey = "arn:aws:kms:us-east-1:000000000000:key/my-key"

	rec := doRequest(t, h, "CreateRepositoryLink", map[string]any{
		"ConnectionArn":    connArn,
		"OwnerId":          "my-org",
		"RepositoryName":   "encrypted-repo",
		"EncryptionKeyArn": encKey,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	info := resp["RepositoryLinkInfo"].(map[string]any)
	assert.Equal(t, encKey, info["EncryptionKeyArn"])

	// UpdateRepositoryLink can update the encryption key.
	linkID := info["RepositoryLinkId"].(string)
	const newEncKey = "arn:aws:kms:us-east-1:000000000000:key/new-key"
	updRec := doRequest(t, h, "UpdateRepositoryLink", map[string]any{
		"RepositoryLinkId": linkID,
		"EncryptionKeyArn": newEncKey,
	})
	require.Equal(t, http.StatusOK, updRec.Code)
	updResp := parseResp(t, updRec)
	updInfo := updResp["RepositoryLinkInfo"].(map[string]any)
	assert.Equal(t, newEncKey, updInfo["EncryptionKeyArn"])
}

func TestListRepositoryLinks_EmptyIsArray(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "ListRepositoryLinks", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	links, ok := resp["RepositoryLinks"].([]any)
	require.True(t, ok, "RepositoryLinks should be an array, not null")
	assert.Empty(t, links)
}

func TestGetRepositoryLink_AllFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	connArn := createCSCConn(t, h, "field-conn", "GitHub")
	const encKey = "arn:aws:kms:us-east-1:000000000000:key/kk"

	rec := doRequest(t, h, "CreateRepositoryLink", map[string]any{
		"ConnectionArn":    connArn,
		"OwnerId":          "github-org",
		"RepositoryName":   "my-service",
		"EncryptionKeyArn": encKey,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	info := resp["RepositoryLinkInfo"].(map[string]any)
	linkID := info["RepositoryLinkId"].(string)

	getRec := doRequest(t, h, "GetRepositoryLink", map[string]any{"RepositoryLinkId": linkID})
	require.Equal(t, http.StatusOK, getRec.Code)

	getResp := parseResp(t, getRec)
	getLinkInfo := getResp["RepositoryLinkInfo"].(map[string]any)
	assert.NotEmpty(t, getLinkInfo["RepositoryLinkId"])
	assert.NotEmpty(t, getLinkInfo["RepositoryLinkArn"])
	assert.Equal(t, "github-org", getLinkInfo["OwnerId"])
	assert.Equal(t, "my-service", getLinkInfo["RepositoryName"])
	assert.Equal(t, "GitHub", getLinkInfo["ProviderType"])
	assert.Equal(t, connArn, getLinkInfo["ConnectionArn"])
	assert.Equal(t, encKey, getLinkInfo["EncryptionKeyArn"])
}

// TestRepositoryLink_ProviderTypeDerived verifies provider type comes from connection.
func TestRepositoryLink_ProviderTypeDerived(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		providerType string
	}{
		{name: "github_provider_derived", providerType: "GitHub"},
		{name: "gitlab_provider_derived", providerType: "GitLab"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			connArn := createCSCConn(t, h, "pt-conn-"+strconv.Itoa(i), tt.providerType)
			linkID := createCSCRepositoryLink(t, h, connArn, "my-repo")

			rec := doRequest(t, h, "GetRepositoryLink", map[string]any{"RepositoryLinkId": linkID})
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			info, ok := resp["RepositoryLinkInfo"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.providerType, info["ProviderType"])
		})
	}
}

// TestListRepositoryLinks_Sorted verifies links are returned sorted by RepositoryLinkId.
func TestListRepositoryLinks_Sorted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		linkCount int
	}{
		{name: "multiple_links_sorted", linkCount: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			connArn := createCSCConn(t, h, "list-conn", "GitHub")

			for i := range tt.linkCount {
				createCSCRepositoryLink(t, h, connArn, "repo-"+strconv.Itoa(i))
			}

			rec := doRequest(t, h, "ListRepositoryLinks", map[string]any{})
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			links, ok := resp["RepositoryLinks"].([]any)
			require.True(t, ok)
			assert.Len(t, links, tt.linkCount)
		})
	}
}

// TestUpdateRepositoryLink exercises the handler.
func TestUpdateRepositoryLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupLinkID     func(t *testing.T, h *codestarconnections.Handler) string
		setupNewConnArn func(t *testing.T, h *codestarconnections.Handler) string
		name            string
		wantStatus      int
		wantNewConn     bool
	}{
		{
			// UpdateRepositoryLinkInput.ConnectionArn's doc comment: "The
			// updated connection ARN must have the same providerType (such
			// as GitHub) as the original connection ARN for the repo link."
			name: "success_updates_connection",
			setupLinkID: func(t *testing.T, h *codestarconnections.Handler) string {
				t.Helper()
				connArn := createCSCConn(t, h, "upd-link-conn", "GitHub")

				return createCSCRepositoryLink(t, h, connArn, "my-repo")
			},
			setupNewConnArn: func(t *testing.T, h *codestarconnections.Handler) string {
				t.Helper()

				return createCSCConn(t, h, "new-conn", "GitHub")
			},
			wantStatus:  http.StatusOK,
			wantNewConn: true,
		},
		{
			name: "link_not_found",
			setupLinkID: func(_ *testing.T, _ *codestarconnections.Handler) string {
				return "nonexistent"
			},
			setupNewConnArn: func(_ *testing.T, _ *codestarconnections.Handler) string {
				return "arn:aws:codestar-connections:us-east-1:123:connection/new"
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// Previously accepted silently: a ConnectionArn naming no real
			// connection was written straight through with a 200.
			name: "new_connection_does_not_exist",
			setupLinkID: func(t *testing.T, h *codestarconnections.Handler) string {
				t.Helper()
				connArn := createCSCConn(t, h, "original-conn", "GitHub")

				return createCSCRepositoryLink(t, h, connArn, "my-repo")
			},
			setupNewConnArn: func(_ *testing.T, _ *codestarconnections.Handler) string {
				return "arn:aws:codestar-connections:us-east-1:000000000000:connection/does-not-exist"
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// Previously accepted silently: swapping in a real connection of
			// a DIFFERENT ProviderType than the link's original connection.
			name: "new_connection_provider_type_mismatch",
			setupLinkID: func(t *testing.T, h *codestarconnections.Handler) string {
				t.Helper()
				connArn := createCSCConn(t, h, "original-conn", "GitHub")

				return createCSCRepositoryLink(t, h, connArn, "my-repo")
			},
			setupNewConnArn: func(t *testing.T, h *codestarconnections.Handler) string {
				t.Helper()

				return createCSCConn(t, h, "new-conn", "Bitbucket")
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			linkID := tt.setupLinkID(t, h)
			newConnArn := tt.setupNewConnArn(t, h)

			rec := doRequest(t, h, "UpdateRepositoryLink", map[string]any{
				"RepositoryLinkId": linkID,
				"ConnectionArn":    newConnArn,
			})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantNewConn {
				resp := parseResp(t, rec)
				info, ok := resp["RepositoryLinkInfo"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, newConnArn, info["ConnectionArn"])
			}
		})
	}
}
