package codeconnections_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/codeconnections"
)

// TestCreateRepositoryLink exercises the CreateRepositoryLink handler.
func TestCreateRepositoryLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantStatus int
		wantLink   bool
	}{
		{
			name: "success",
			body: map[string]any{
				"ConnectionArn":  "arn:aws:codeconnections:us-east-1:123:connection/abc",
				"OwnerId":        "my-org",
				"RepositoryName": "my-repo",
			},
			wantStatus: http.StatusOK,
			wantLink:   true,
		},
		{
			name:       "missing_connection_arn",
			body:       map[string]any{"OwnerId": "my-org", "RepositoryName": "my-repo"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_owner_id",
			body: map[string]any{
				"ConnectionArn":  "arn:aws:codeconnections:us-east-1:123:connection/abc",
				"RepositoryName": "my-repo",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_repository_name",
			body: map[string]any{
				"ConnectionArn": "arn:aws:codeconnections:us-east-1:123:connection/abc",
				"OwnerId":       "my-org",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			rec := doJSON(t, h, "CreateRepositoryLink", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantLink {
				resp := parseResp(t, rec)
				info, ok := resp["RepositoryLinkInfo"].(map[string]any)
				require.True(t, ok)
				assert.NotEmpty(t, info["RepositoryLinkId"])
				assert.NotEmpty(t, info["RepositoryLinkArn"])
				assert.Equal(t, "my-org", info["OwnerId"])
				assert.Equal(t, "my-repo", info["RepositoryName"])
			}
		})
	}
}

// TestGetRepositoryLink exercises the GetRepositoryLink handler.
func TestGetRepositoryLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupLinkID  func(t *testing.T, h *codeconnections.Handler) string
		name         string
		wantOwner    string
		wantRepoName string
		wantStatus   int
	}{
		{
			name: "success",
			setupLinkID: func(t *testing.T, h *codeconnections.Handler) string {
				t.Helper()
				connArn := createConn(t, h, "my-conn", "GitHub")

				return createRepositoryLink(t, h, connArn, "my-org", "my-repo")
			},
			wantStatus:   http.StatusOK,
			wantOwner:    "my-org",
			wantRepoName: "my-repo",
		},
		{
			name: "not_found",
			setupLinkID: func(_ *testing.T, _ *codeconnections.Handler) string {
				return "nonexistent-id"
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_id",
			setupLinkID: func(_ *testing.T, _ *codeconnections.Handler) string {
				return ""
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			linkID := tt.setupLinkID(t, h)
			rec := doJSON(t, h, "GetRepositoryLink", map[string]any{"RepositoryLinkId": linkID})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				resp := parseResp(t, rec)
				info, ok := resp["RepositoryLinkInfo"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantOwner, info["OwnerId"])
				assert.Equal(t, tt.wantRepoName, info["RepositoryName"])
			}
		})
	}
}

// TestDeleteRepositoryLink exercises the DeleteRepositoryLink handler.
func TestDeleteRepositoryLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupLinkID func(t *testing.T, h *codeconnections.Handler) string
		name        string
		wantStatus  int
	}{
		{
			name: "success",
			setupLinkID: func(t *testing.T, h *codeconnections.Handler) string {
				t.Helper()
				connArn := createConn(t, h, "my-conn", "GitHub")

				return createRepositoryLink(t, h, connArn, "my-org", "my-repo")
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "not_found",
			setupLinkID: func(_ *testing.T, _ *codeconnections.Handler) string {
				return "nonexistent-id"
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			linkID := tt.setupLinkID(t, h)
			rec := doJSON(t, h, "DeleteRepositoryLink", map[string]any{"RepositoryLinkId": linkID})
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				getRec := doJSON(
					t,
					h,
					"GetRepositoryLink",
					map[string]any{"RepositoryLinkId": linkID},
				)
				assert.Equal(t, http.StatusBadRequest, getRec.Code)
			}
		})
	}
}

// TestRepositoryLinkProviderTypeDerivedFromConnection verifies that provider type
// is inherited from the associated connection when creating a repository link.
func TestRepositoryLinkProviderTypeDerivedFromConnection(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	connArn := createConn(t, h, "my-conn", "GitHub")
	linkID := createRepositoryLink(t, h, connArn, "acme-corp", "acme-service")

	rec := doJSON(t, h, "GetRepositoryLink", map[string]any{"RepositoryLinkId": linkID})
	require.Equal(t, http.StatusOK, rec.Code)

	resp := parseResp(t, rec)
	info, ok := resp["RepositoryLinkInfo"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "GitHub", info["ProviderType"])
	assert.Equal(t, "acme-corp", info["OwnerId"])
}

// TestCC_RepositoryLink_ListUpdate exercises ListRepositoryLinks and UpdateRepositoryLink.
func TestCC_RepositoryLink_ListUpdate(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	connArn := createConn(t, h, "test-conn", "GitHub")
	linkID := createRepositoryLink(t, h, connArn, "my-owner", "my-repo")

	// ListRepositoryLinks
	rec := doJSON(t, h, "ListRepositoryLinks", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	resp := parseResp(t, rec)
	links, _ := resp["RepositoryLinks"].([]any)
	assert.NotEmpty(t, links)

	// UpdateRepositoryLink
	rec = doJSON(t, h, "UpdateRepositoryLink", map[string]any{
		"RepositoryLinkId": linkID,
		"ConnectionArn":    connArn,
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestRepositoryLinkCreate exercises CreateRepositoryLink field shapes and validation.
func TestRepositoryLinkCreate(t *testing.T) {
	t.Parallel()

	h := newHandlerFixedAccount(t)

	rec := doJSON(t, h, "CreateConnection", map[string]any{
		"ConnectionName": "rl-conn",
		"ProviderType":   "GitHub",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	connArn := parseResp(t, rec)["ConnectionArn"].(string)

	tests := []struct {
		body     map[string]any
		check    func(t *testing.T, m map[string]any)
		name     string
		wantCode int
	}{
		{
			name: "creates repository link",
			body: map[string]any{
				"ConnectionArn":  connArn,
				"OwnerId":        "my-org",
				"RepositoryName": "my-repo",
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, m map[string]any) {
				t.Helper()

				info, _ := m["RepositoryLinkInfo"].(map[string]any)
				require.NotNil(t, info)
				assert.NotEmpty(t, info["RepositoryLinkId"])
				assert.NotEmpty(t, info["RepositoryLinkArn"])
				assert.Equal(t, connArn, info["ConnectionArn"])
				assert.Equal(t, "my-org", info["OwnerId"])
				assert.Equal(t, "my-repo", info["RepositoryName"])
			},
		},
		{
			name:     "missing ConnectionArn returns error",
			body:     map[string]any{"OwnerId": "org", "RepositoryName": "repo"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing OwnerId returns error",
			body:     map[string]any{"ConnectionArn": connArn, "RepositoryName": "repo"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing RepositoryName returns error",
			body:     map[string]any{"ConnectionArn": connArn, "OwnerId": "org"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := doJSON(t, h, "CreateRepositoryLink", tt.body)
			assert.Equal(t, tt.wantCode, result.Code)

			if tt.check != nil {
				tt.check(t, parseResp(t, result))
			}
		})
	}
}

// TestRepositoryLinkGetDelete exercises the CreateRepositoryLink/GetRepositoryLink/
// DeleteRepositoryLink lifecycle.
func TestRepositoryLinkGetDelete(t *testing.T) {
	t.Parallel()

	h := newHandlerFixedAccount(t)

	rec := doJSON(t, h, "CreateConnection", map[string]any{
		"ConnectionName": "rl-getdel-conn",
		"ProviderType":   "GitHub",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	connArn := parseResp(t, rec)["ConnectionArn"].(string)

	rec = doJSON(t, h, "CreateRepositoryLink", map[string]any{
		"ConnectionArn":  connArn,
		"OwnerId":        "my-org",
		"RepositoryName": "my-repo",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	info := parseResp(t, rec)["RepositoryLinkInfo"].(map[string]any)
	rlID := info["RepositoryLinkId"].(string)

	rec = doJSON(t, h, "GetRepositoryLink", map[string]any{"RepositoryLinkId": rlID})
	assert.Equal(t, http.StatusOK, rec.Code)

	getInfo := parseResp(t, rec)["RepositoryLinkInfo"].(map[string]any)
	assert.Equal(t, rlID, getInfo["RepositoryLinkId"])

	rec = doJSON(t, h, "DeleteRepositoryLink", map[string]any{"RepositoryLinkId": rlID})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doJSON(t, h, "GetRepositoryLink", map[string]any{"RepositoryLinkId": rlID})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestUpdateRepositoryLink exercises UpdateRepositoryLink operations.
func TestUpdateRepositoryLink(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setupLinkID     func(t *testing.T, h *codeconnections.Handler) string
		setupNewConnArn func(t *testing.T, h *codeconnections.Handler) string
		name            string
		wantStatus      int
		wantNewConn     bool
	}{
		{
			// The updated ConnectionArn must resolve to a real connection of
			// the SAME ProviderType as the link's existing connection --
			// UpdateRepositoryLinkInput.ConnectionArn's doc comment
			// (api_op_UpdateRepositoryLink.go): "The updated connection ARN
			// must have the same providerType (such as GitHub) as the
			// original connection ARN for the repo link."
			name: "success_updates_connection",
			setupLinkID: func(t *testing.T, h *codeconnections.Handler) string {
				t.Helper()
				connArn := createConn(t, h, "original-conn", "GitHub")

				return createRepositoryLink(t, h, connArn, "my-org", "my-repo")
			},
			setupNewConnArn: func(t *testing.T, h *codeconnections.Handler) string {
				t.Helper()

				return createConn(t, h, "new-conn", "GitHub")
			},
			wantStatus:  http.StatusOK,
			wantNewConn: true,
		},
		{
			name: "not_found",
			setupLinkID: func(_ *testing.T, _ *codeconnections.Handler) string {
				return "nonexistent-link-id"
			},
			setupNewConnArn: func(_ *testing.T, _ *codeconnections.Handler) string {
				return "arn:aws:codeconnections:us-east-1:123:connection/new"
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_repository_link_id",
			setupLinkID: func(_ *testing.T, _ *codeconnections.Handler) string {
				return ""
			},
			setupNewConnArn: func(_ *testing.T, _ *codeconnections.Handler) string {
				return "arn:aws:codeconnections:us-east-1:123:connection/new"
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// Previously accepted silently: a ConnectionArn naming no real
			// connection was written straight through with a 200.
			name: "new_connection_does_not_exist",
			setupLinkID: func(t *testing.T, h *codeconnections.Handler) string {
				t.Helper()
				connArn := createConn(t, h, "original-conn", "GitHub")

				return createRepositoryLink(t, h, connArn, "my-org", "my-repo")
			},
			setupNewConnArn: func(_ *testing.T, _ *codeconnections.Handler) string {
				return "arn:aws:codeconnections:us-east-1:123456789012:connection/does-not-exist"
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			// Previously accepted silently: swapping in a real connection of
			// a DIFFERENT ProviderType than the link's original connection.
			name: "new_connection_provider_type_mismatch",
			setupLinkID: func(t *testing.T, h *codeconnections.Handler) string {
				t.Helper()
				connArn := createConn(t, h, "original-conn", "GitHub")

				return createRepositoryLink(t, h, connArn, "my-org", "my-repo")
			},
			setupNewConnArn: func(t *testing.T, h *codeconnections.Handler) string {
				t.Helper()

				return createConn(t, h, "new-conn", "Bitbucket")
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			linkID := tt.setupLinkID(t, h)
			newConnArn := tt.setupNewConnArn(t, h)

			rec := doJSON(t, h, "UpdateRepositoryLink", map[string]any{
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

// TestListRepositoryLinksPagination verifies MaxResults/NextToken for ListRepositoryLinks.
func TestListRepositoryLinksPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		linkCount  int
		maxResults int
		wantCount  int
		wantToken  bool
	}{
		{
			name:       "first_page",
			linkCount:  3,
			maxResults: 2,
			wantCount:  2,
			wantToken:  true,
		},
		{
			name:       "all_results",
			linkCount:  2,
			maxResults: 10,
			wantCount:  2,
			wantToken:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			connArn := createConn(t, h, "link-conn", "GitHub")

			for i := range tt.linkCount {
				rec := doJSON(t, h, "CreateRepositoryLink", map[string]any{
					"ConnectionArn":  connArn,
					"OwnerId":        "my-org",
					"RepositoryName": "repo-" + strconv.Itoa(i),
				})
				require.Equal(t, http.StatusOK, rec.Code)
			}

			body := map[string]any{}
			if tt.maxResults > 0 {
				body["MaxResults"] = tt.maxResults
			}

			rec := doJSON(t, h, "ListRepositoryLinks", body)
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			links, ok := resp["RepositoryLinks"].([]any)
			require.True(t, ok)
			assert.Len(t, links, tt.wantCount)

			_, hasToken := resp["NextToken"]
			assert.Equal(t, tt.wantToken, hasToken)
		})
	}
}

// TestRepositoryLinkNoTagsFieldInListItem verifies that ListRepositoryLinks
// (and CreateRepositoryLink/GetRepositoryLink) items have NO "Tags" field:
// the real RepositoryLinkInfo wire type (aws-sdk-go-v2/service/
// codeconnections@v1.13.4 types.RepositoryLinkInfo) has no Tags member at
// all -- tags for a repository link are retrievable only via
// ListTagsForResource, exercised here too so the tags themselves are not
// lost, just moved to the right operation.
func TestRepositoryLinkNoTagsFieldInListItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tags      []map[string]string
		wantCount int
	}{
		{
			name:      "tags_retrievable_via_list_tags_for_resource",
			tags:      []map[string]string{{"Key": "owner", "Value": "ops"}},
			wantCount: 1,
		},
		{
			name:      "no_tags",
			tags:      nil,
			wantCount: 0,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			connArn := createConn(t, h, "list-tag-conn-"+strconv.Itoa(i), "GitHub")

			body := map[string]any{
				"ConnectionArn":  connArn,
				"OwnerId":        "my-org",
				"RepositoryName": "my-repo",
			}
			if tt.tags != nil {
				body["Tags"] = tt.tags
			}

			rec := doJSON(t, h, "CreateRepositoryLink", body)
			require.Equal(t, http.StatusOK, rec.Code)

			createInfo := parseResp(t, rec)["RepositoryLinkInfo"].(map[string]any)
			_, hasTagsOnCreate := createInfo["Tags"]
			assert.False(t, hasTagsOnCreate, "RepositoryLinkInfo must not carry a Tags field")

			linkArn, _ := createInfo["RepositoryLinkArn"].(string)
			require.NotEmpty(t, linkArn)

			rec = doJSON(t, h, "ListRepositoryLinks", nil)
			require.Equal(t, http.StatusOK, rec.Code)

			resp := parseResp(t, rec)
			links, _ := resp["RepositoryLinks"].([]any)
			require.Len(t, links, 1)

			linkMap := links[0].(map[string]any)
			_, hasTags := linkMap["Tags"]
			assert.False(t, hasTags, "RepositoryLinkInfo list item must not carry a Tags field")

			// Tags are still real state, retrievable via ListTagsForResource.
			tagRec := doJSON(t, h, "ListTagsForResource", map[string]any{"ResourceArn": linkArn})
			require.Equal(t, http.StatusOK, tagRec.Code)
			gotTags, _ := parseResp(t, tagRec)["Tags"].([]any)
			assert.Len(t, gotTags, tt.wantCount)
		})
	}
}

// TestDeleteRepositoryLink_InUse verifies that a repository link cannot be deleted
// while a sync configuration still references it. Real DeleteRepositoryLink
// documents SyncConfigurationStillExistsException for this case.
func TestDeleteRepositoryLink_InUse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr    error
		name       string
		attachSync bool
	}{
		{
			name:       "link_with_active_sync_config_rejected",
			attachSync: true,
			wantErr:    codeconnections.ErrSyncConfigStillExists,
		},
		{name: "link_with_no_sync_configs_deletes", attachSync: false, wantErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			b := newTestBackend()

			conn, err := b.CreateConnection(ctx, "my-conn", "GitHub", "", nil)
			require.NoError(t, err)

			link, err := b.CreateRepositoryLink(ctx, conn.ConnectionArn, "my-org", "my-repo", "", nil)
			require.NoError(t, err)

			if tt.attachSync {
				_, syncErr := b.CreateSyncConfiguration(
					ctx, "main", "sync.yaml", link.RepositoryLinkID, "my-stack",
					"arn:aws:iam::123456789012:role/r", "CFN_STACK_SYNC", "", "", "",
				)
				require.NoError(t, syncErr)
			}

			err = b.DeleteRepositoryLink(ctx, link.RepositoryLinkID)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateRepositoryLink_Duplicate verifies that creating a repository link for the
// same connection+owner+repo combination twice is rejected. Real CreateRepositoryLink
// registers a dedicated ResourceAlreadyExistsException.
func TestCreateRepositoryLink_Duplicate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	b := newTestBackend()

	conn, err := b.CreateConnection(ctx, "my-conn", "GitHub", "", nil)
	require.NoError(t, err)

	_, err = b.CreateRepositoryLink(ctx, conn.ConnectionArn, "my-org", "my-repo", "", nil)
	require.NoError(t, err)

	_, err = b.CreateRepositoryLink(ctx, conn.ConnectionArn, "my-org", "my-repo", "", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, codeconnections.ErrAlreadyExists)
}
