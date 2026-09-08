package appstream_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appstream"
)

// TestAppStream_AppBlocks covers AppBlock CRUD.
func TestAppStream_AppBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(h *appstream.Handler)
		check    func(t *testing.T, body []byte)
		body     any
		name     string
		action   string
		wantCode int
	}{
		{
			name:     "CreateAppBlock returns app block with ARN",
			action:   "CreateAppBlock",
			body:     map[string]any{"Name": "my-block", "Description": "test"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				ab := resp["AppBlock"].(map[string]any)
				assert.Equal(t, "my-block", ab["Name"])
				assert.Contains(t, ab["Arn"], ":app-block/my-block")
				assert.Equal(t, "INACTIVE", ab["State"])
			},
		},
		{
			name:   "CreateAppBlock duplicate returns error",
			action: "CreateAppBlock",
			setup: func(h *appstream.Handler) {
				createAppBlock(t, h, "dup-block")
			},
			body:     map[string]any{"Name": "dup-block"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "DescribeAppBlocks returns all blocks",
			action: "DescribeAppBlocks",
			setup: func(h *appstream.Handler) {
				createAppBlock(t, h, "blk-a")
				createAppBlock(t, h, "blk-b")
			},
			body:     map[string]any{},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				blocks := resp["AppBlocks"].([]any)
				assert.Len(t, blocks, 2)
			},
		},
		{
			name:   "DeleteAppBlock removes block",
			action: "DeleteAppBlock",
			setup: func(h *appstream.Handler) {
				createAppBlock(t, h, "del-block")
			},
			body:     map[string]any{"Name": "del-block"},
			wantCode: http.StatusOK,
		},
		{
			name:     "DeleteAppBlock unknown returns error",
			action:   "DeleteAppBlock",
			body:     map[string]any{"Name": "no-such"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tc.setup != nil {
				tc.setup(h)
			}
			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

// TestAppStream_AppBlockBuilders covers AppBlockBuilder CRUD + Start/Stop.
func TestAppStream_AppBlockBuilders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(h *appstream.Handler)
		check    func(t *testing.T, body []byte)
		body     any
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "CreateAppBlockBuilder returns builder",
			action: "CreateAppBlockBuilder",
			body: map[string]any{
				"Name":         "my-builder",
				"InstanceType": "stream.standard.medium",
				"Platform":     "WINDOWS_SERVER_2019",
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				bb := resp["AppBlockBuilder"].(map[string]any)
				assert.Equal(t, "my-builder", bb["Name"])
				assert.Equal(t, "STOPPED", bb["State"])
			},
		},
		{
			name:     "CreateAppBlockBuilder missing InstanceType returns error",
			action:   "CreateAppBlockBuilder",
			body:     map[string]any{"Name": "no-instance"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "StartAppBlockBuilder transitions to RUNNING",
			action: "StartAppBlockBuilder",
			setup: func(h *appstream.Handler) {
				createAppBlockBuilder(t, h, "start-builder")
			},
			body:     map[string]any{"Name": "start-builder"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				bb := resp["AppBlockBuilder"].(map[string]any)
				assert.Equal(t, "RUNNING", bb["State"])
			},
		},
		{
			name:   "StopAppBlockBuilder transitions to STOPPED",
			action: "StopAppBlockBuilder",
			setup: func(h *appstream.Handler) {
				createAppBlockBuilder(t, h, "stop-builder")
				rec := doRequest(t, h, "StartAppBlockBuilder", map[string]any{"Name": "stop-builder"})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{"Name": "stop-builder"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				bb := resp["AppBlockBuilder"].(map[string]any)
				assert.Equal(t, "STOPPED", bb["State"])
			},
		},
		{
			name:   "StopAppBlockBuilder on already-stopped builder succeeds (idempotent)",
			action: "StopAppBlockBuilder",
			setup: func(h *appstream.Handler) {
				createAppBlockBuilder(t, h, "already-stopped-builder")
			},
			body:     map[string]any{"Name": "already-stopped-builder"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				bb := resp["AppBlockBuilder"].(map[string]any)
				assert.Equal(t, "STOPPED", bb["State"])
			},
		},
		{
			name:   "UpdateAppBlockBuilder changes description",
			action: "UpdateAppBlockBuilder",
			setup: func(h *appstream.Handler) {
				createAppBlockBuilder(t, h, "upd-builder")
			},
			body:     map[string]any{"Name": "upd-builder", "Description": "updated"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				bb := resp["AppBlockBuilder"].(map[string]any)
				assert.Equal(t, "updated", bb["Description"])
			},
		},
		{
			name:   "CreateAppBlockBuilderStreamingURL returns URL and Expires",
			action: "CreateAppBlockBuilderStreamingURL",
			setup: func(h *appstream.Handler) {
				createAppBlockBuilder(t, h, "url-builder")
			},
			body:     map[string]any{"AppBlockBuilderName": "url-builder"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				assert.NotEmpty(t, resp["StreamingURL"])
				assert.NotEmpty(t, resp["Expires"])
			},
		},
		{
			name:   "AssociateAppBlockBuilderAppBlock creates association",
			action: "AssociateAppBlockBuilderAppBlock",
			setup: func(h *appstream.Handler) {
				createAppBlockBuilder(t, h, "assoc-builder")
				createAppBlock(t, h, "assoc-block")
			},
			body: map[string]any{
				"AppBlockBuilderName": "assoc-builder",
				"AppBlockArn":         "assoc-block",
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "DescribeAppBlockBuilderAppBlockAssociations lists association",
			action: "DescribeAppBlockBuilderAppBlockAssociations",
			setup: func(h *appstream.Handler) {
				createAppBlockBuilder(t, h, "list-builder")
				createAppBlock(t, h, "list-block")
				rec := doRequest(t, h, "AssociateAppBlockBuilderAppBlock", map[string]any{
					"AppBlockBuilderName": "list-builder",
					"AppBlockArn":         "list-block",
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{"AppBlockBuilderName": "list-builder"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				assocs := resp["AppBlockBuilderAppBlockAssociations"].([]any)
				assert.Len(t, assocs, 1)
			},
		},
		{
			name:   "DeleteAppBlockBuilder removes builder",
			action: "DeleteAppBlockBuilder",
			setup: func(h *appstream.Handler) {
				createAppBlockBuilder(t, h, "del-builder")
			},
			body:     map[string]any{"Name": "del-builder"},
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			if tc.setup != nil {
				tc.setup(h)
			}
			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

// TestAppStream_AppBlockARNFormat verifies app block ARN format.
func TestAppStream_AppBlockARNFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateAppBlock", map[string]any{"Name": "arn-appblock"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ab := resp["AppBlock"].(map[string]any)
	assert.Contains(t, ab["Arn"], "arn:aws:appstream:")
	assert.Contains(t, ab["Arn"], "app-block/arn-appblock")
}

// TestAppStream_AppBlockBuilderStreamingURL verifies CreateAppBlockBuilderStreamingURL.
func TestAppStream_AppBlockBuilderStreamingURL(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateAppBlockBuilder", map[string]any{
		"Name":         "url-builder",
		"InstanceType": "stream.standard.medium",
	})

	rec := doRequest(t, h, "CreateAppBlockBuilderStreamingURL", map[string]any{
		"AppBlockBuilderName": "url-builder",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["StreamingURL"])
	assert.NotEmpty(t, resp["Expires"])
}

// TestAppStream_DeleteAppBlock_InUseByApplication proves DeleteAppBlock
// rejects an app block still referenced by an application's AppBlockArn.
// Regression for gopherstack-65w: DeleteAppBlock models ResourceInUseException
// but the backend deleted unconditionally, regardless of whether an
// application still pointed at it via the required AppBlockArn member
// (api_op_CreateApplication.go: "This member is required").
func TestAppStream_DeleteAppBlock_InUseByApplication(t *testing.T) {
	t.Parallel()

	t.Run("referenced by application", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		abRec := doRequest(t, h, "CreateAppBlock", map[string]any{"Name": "inuse-block"})
		require.Equal(t, http.StatusOK, abRec.Code)

		var abResp map[string]any
		require.NoError(t, json.Unmarshal(abRec.Body.Bytes(), &abResp))
		ab := abResp["AppBlock"].(map[string]any)

		appRec := doRequest(t, h, "CreateApplication", map[string]any{
			"Name":        "block-user-app",
			"LaunchPath":  "/app/block-user-app",
			"AppBlockArn": ab["Arn"],
			"IconS3Location": map[string]any{
				"S3Bucket": "icon-bucket",
				"S3Key":    "icons/block-user-app.png",
			},
			"InstanceFamilies": []string{"GENERAL_PURPOSE"},
		})
		require.Equal(t, http.StatusOK, appRec.Code)

		delRec := doRequest(t, h, "DeleteAppBlock", map[string]any{"Name": "inuse-block"})
		assert.Equal(t, http.StatusBadRequest, delRec.Code)
	})

	t.Run("not referenced deletes fine", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		createAppBlock(t, h, "free-block")

		rec := doRequest(t, h, "DeleteAppBlock", map[string]any{"Name": "free-block"})
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
