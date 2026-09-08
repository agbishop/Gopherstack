package appstream_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/appstream"
)

// TestAppStream_Images covers Image CRUD, copy, permissions.
func TestAppStream_Images(t *testing.T) {
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
			name:     "CreateImportedImage returns image",
			action:   "CreateImportedImage",
			body:     map[string]any{"Name": "my-img"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				img := resp["Image"].(map[string]any)
				assert.Equal(t, "my-img", img["Name"])
				assert.Equal(t, "AVAILABLE", img["State"])
			},
		},
		{
			name:   "DescribeImages lists all",
			action: "DescribeImages",
			setup: func(h *appstream.Handler) {
				createImage(t, h, "img-a")
				createImage(t, h, "img-b")
			},
			body:     map[string]any{},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				imgs := resp["Images"].([]any)
				assert.Len(t, imgs, 2)
			},
		},
		{
			name:   "CopyImage creates copy",
			action: "CopyImage",
			setup: func(h *appstream.Handler) {
				createImage(t, h, "src-img")
			},
			body: map[string]any{
				"SourceImageName":      "src-img",
				"DestinationImageName": "dst-img",
				"DestinationRegion":    "us-west-2",
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				assert.Equal(t, "dst-img", resp["DestinationImageName"])
			},
		},
		{
			name:   "CreateUpdatedImage creates new version",
			action: "CreateUpdatedImage",
			setup: func(h *appstream.Handler) {
				createImage(t, h, "base-img")
			},
			body: map[string]any{
				"ExistingImageName": "base-img",
				"NewImageName":      "updated-img",
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "DeleteImage removes image",
			action: "DeleteImage",
			setup: func(h *appstream.Handler) {
				createImage(t, h, "rm-img")
			},
			body:     map[string]any{"Name": "rm-img"},
			wantCode: http.StatusOK,
		},
		{
			name:   "UpdateImagePermissions sets perms",
			action: "UpdateImagePermissions",
			setup: func(h *appstream.Handler) {
				createImage(t, h, "perm-img")
			},
			body: map[string]any{
				"Name":            "perm-img",
				"SharedAccountId": "111111111111",
				"ImagePermissions": map[string]any{
					"AllowFleet": true, "AllowImageBuilder": false,
				},
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "DescribeImagePermissions returns perms",
			action: "DescribeImagePermissions",
			setup: func(h *appstream.Handler) {
				createImage(t, h, "desc-perm-img")
				rec := doRequest(t, h, "UpdateImagePermissions", map[string]any{
					"Name": "desc-perm-img", "SharedAccountId": "222222222222",
					"ImagePermissions": map[string]any{"AllowFleet": true},
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{"Name": "desc-perm-img"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				perms := resp["SharedImagePermissionsList"].([]any)
				assert.Len(t, perms, 1)
			},
		},
		{
			name:   "DeleteImagePermissions removes perms",
			action: "DeleteImagePermissions",
			setup: func(h *appstream.Handler) {
				createImage(t, h, "del-perm-img")
				rec := doRequest(t, h, "UpdateImagePermissions", map[string]any{
					"Name": "del-perm-img", "SharedAccountId": "333333333333",
					"ImagePermissions": map[string]any{"AllowFleet": true},
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{"Name": "del-perm-img", "SharedAccountId": "333333333333"},
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

// TestAppStream_ImageBuilders covers ImageBuilder CRUD, Start/Stop, software associations.
func TestAppStream_ImageBuilders(t *testing.T) {
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
			name:   "CreateImageBuilder returns builder in STOPPED state",
			action: "CreateImageBuilder",
			body: map[string]any{
				"Name":         "my-ib",
				"InstanceType": "stream.standard.medium",
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				ib := resp["ImageBuilder"].(map[string]any)
				assert.Equal(t, "STOPPED", ib["State"])
			},
		},
		{
			name:   "StartImageBuilder transitions to RUNNING",
			action: "StartImageBuilder",
			setup: func(h *appstream.Handler) {
				createImageBuilder(t, h, "start-ib")
			},
			body:     map[string]any{"Name": "start-ib"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				ib := resp["ImageBuilder"].(map[string]any)
				assert.Equal(t, "RUNNING", ib["State"])
				// Real StartImageBuilderOutput carries only ImageBuilder --
				// no StreamingURL (that's a separate CreateImageBuilderStreamingURL call).
				assert.NotContains(t, resp, "StreamingURL")
			},
		},
		{
			name:   "StopImageBuilder transitions to STOPPED",
			action: "StopImageBuilder",
			setup: func(h *appstream.Handler) {
				createImageBuilder(t, h, "stop-ib")
				rec := doRequest(t, h, "StartImageBuilder", map[string]any{"Name": "stop-ib"})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{"Name": "stop-ib"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				ib := resp["ImageBuilder"].(map[string]any)
				assert.Equal(t, "STOPPED", ib["State"])
			},
		},
		{
			name:   "StopImageBuilder on already-stopped builder succeeds (idempotent)",
			action: "StopImageBuilder",
			setup: func(h *appstream.Handler) {
				createImageBuilder(t, h, "already-stopped-ib")
			},
			body:     map[string]any{"Name": "already-stopped-ib"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				ib := resp["ImageBuilder"].(map[string]any)
				assert.Equal(t, "STOPPED", ib["State"])
			},
		},
		{
			name:   "CreateImageBuilderStreamingURL returns URL and Expires",
			action: "CreateImageBuilderStreamingURL",
			setup: func(h *appstream.Handler) {
				createImageBuilder(t, h, "url-ib")
			},
			body:     map[string]any{"Name": "url-ib"},
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
			name:   "AssociateSoftwareToImageBuilder adds software",
			action: "AssociateSoftwareToImageBuilder",
			setup: func(h *appstream.Handler) {
				createImageBuilder(t, h, "sw-ib")
			},
			body: map[string]any{
				"ImageBuilderName": "sw-ib",
				"SoftwareNames":    []string{"pkg-a", "pkg-b"},
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "DescribeSoftwareAssociations lists software",
			action: "DescribeSoftwareAssociations",
			setup: func(h *appstream.Handler) {
				createImageBuilder(t, h, "list-sw-ib")
				rec := doRequest(t, h, "AssociateSoftwareToImageBuilder", map[string]any{
					"ImageBuilderName": "list-sw-ib",
					"SoftwareNames":    []string{"pkg-x"},
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{"AssociatedResource": "list-sw-ib"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				assert.Equal(t, "list-sw-ib", resp["AssociatedResource"])
				assocs := resp["SoftwareAssociations"].([]any)
				assert.Len(t, assocs, 1)
			},
		},
		{
			name:   "DescribeSoftwareAssociations for image resource returns 200",
			action: "DescribeSoftwareAssociations",
			setup: func(h *appstream.Handler) {
				createImage(t, h, "sw-assoc-image")
			},
			body:     map[string]any{"AssociatedResource": "sw-assoc-image"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				assert.Equal(t, "sw-assoc-image", resp["AssociatedResource"])
				assocs := resp["SoftwareAssociations"].([]any)
				assert.Empty(t, assocs)
			},
		},
		{
			name:   "DisassociateSoftwareFromImageBuilder removes software",
			action: "DisassociateSoftwareFromImageBuilder",
			setup: func(h *appstream.Handler) {
				createImageBuilder(t, h, "dis-sw-ib")
				rec := doRequest(t, h, "AssociateSoftwareToImageBuilder", map[string]any{
					"ImageBuilderName": "dis-sw-ib",
					"SoftwareNames":    []string{"pkg-z"},
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body: map[string]any{
				"ImageBuilderName": "dis-sw-ib",
				"SoftwareNames":    []string{"pkg-z"},
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "StartSoftwareDeploymentToImageBuilder succeeds",
			action: "StartSoftwareDeploymentToImageBuilder",
			setup: func(h *appstream.Handler) {
				createImageBuilder(t, h, "deploy-ib")
			},
			body:     map[string]any{"ImageBuilderName": "deploy-ib"},
			wantCode: http.StatusOK,
		},
		{
			name:   "DeleteImageBuilder removes builder",
			action: "DeleteImageBuilder",
			setup: func(h *appstream.Handler) {
				createImageBuilder(t, h, "del-ib")
			},
			body:     map[string]any{"Name": "del-ib"},
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

// TestAppStream_ExportImageTasks covers export task lifecycle.
func TestAppStream_ExportImageTasks(t *testing.T) {
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
			name:   "CreateExportImageTask returns ExportImageTask",
			action: "CreateExportImageTask",
			setup: func(h *appstream.Handler) {
				createImage(t, h, "exp-img")
			},
			body: map[string]any{
				"ImageName":  "exp-img",
				"AmiName":    "exported-ami",
				"IamRoleArn": "arn:aws:iam::000000000000:role/export-role",
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				task := resp["ExportImageTask"].(map[string]any)
				assert.NotEmpty(t, task["TaskId"])
				assert.Equal(t, "exported-ami", task["AmiName"])
				assert.Equal(t, "COMPLETED", task["State"])
				assert.NotEmpty(t, task["ImageArn"])
				assert.NotEmpty(t, task["AmiId"])
			},
		},
		{
			name:   "ListExportImageTasks returns all",
			action: "ListExportImageTasks",
			setup: func(h *appstream.Handler) {
				createImage(t, h, "lst-exp-img")
				rec := doRequest(t, h, "CreateExportImageTask", map[string]any{
					"ImageName":  "lst-exp-img",
					"AmiName":    "exported-ami",
					"IamRoleArn": "arn:aws:iam::000000000000:role/export-role",
				})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			body:     map[string]any{},
			wantCode: http.StatusOK,
			check: func(t *testing.T, respBody []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(respBody, &resp))
				tasks := resp["ExportImageTasks"].([]any)
				assert.Len(t, tasks, 1)
			},
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

// TestAppStream_GetExportImageTask covers the round-trip for GetExportImageTask.
func TestAppStream_GetExportImageTask(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createImage(t, h, "rt-img")

	rec := doRequest(t, h, "CreateExportImageTask", map[string]any{
		"ImageName": "rt-img", "AmiName": "exported-ami", "IamRoleArn": "arn:aws:iam::000000000000:role/export-role",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	task := createResp["ExportImageTask"].(map[string]any)
	taskID := task["TaskId"].(string)

	rec2 := doRequest(t, h, "GetExportImageTask", map[string]any{"TaskId": taskID})
	require.Equal(t, http.StatusOK, rec2.Code)

	var getResp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &getResp))
	gotTask := getResp["ExportImageTask"].(map[string]any)
	assert.Equal(t, taskID, gotTask["TaskId"])
}

// TestAppStream_ImageBuilderARNFormat verifies image builder ARN format.
func TestAppStream_ImageBuilderARNFormat(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateImageBuilder", map[string]any{
		"Name":         "arn-builder",
		"InstanceType": "stream.standard.medium",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	builder := resp["ImageBuilder"].(map[string]any)
	assert.Regexp(t, `^arn:aws:appstream:[a-z0-9-]+:\d+:image-builder/arn-builder$`, builder["Arn"])
}

// TestAppStream_DeleteImage_InUseByFleet proves DeleteImage rejects an image
// still referenced by a fleet's ImageName or ImageArn. Regression for
// gopherstack-65w: DeleteImage's own doc comment states "You cannot delete
// an image when it is in use" (api_op_DeleteImage.go), and the op models
// ResourceInUseException, but the backend deleted unconditionally.
func TestAppStream_DeleteImage_InUseByFleet(t *testing.T) {
	t.Parallel()

	t.Run("referenced by ImageName", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		createImage(t, h, "inuse-img")
		doRequest(t, h, "CreateFleet", map[string]any{
			"Name":         "img-user-fleet",
			"InstanceType": "stream.standard.medium",
			"ImageName":    "inuse-img",
		})

		rec := doRequest(t, h, "DeleteImage", map[string]any{"Name": "inuse-img"})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("not referenced deletes fine", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		createImage(t, h, "free-img")

		rec := doRequest(t, h, "DeleteImage", map[string]any{"Name": "free-img"})
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
