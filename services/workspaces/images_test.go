package workspaces_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	wssdk "github.com/aws/aws-sdk-go-v2/service/workspaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/workspaces"
)

// TestWorkspaceImageCRUD exercises image creation via Copy/Create/Import operations.
func TestWorkspaceImageCRUD(t *testing.T) { //nolint:paralleltest // existing issue.
	h, _ := newTestHandlerWithBackend(t)
	wsID := createWorkspace(t, h)

	tests := []struct {
		body  any
		check func(t *testing.T, body []byte)
		name  string
		op    string
	}{
		{
			// SourceRegion differs from the test backend's "us-east-1", so
			// SourceImageId is deliberately not validated here (see the
			// CopyWorkspaceImage doc comment) -- a made-up ID is fine.
			name: "CopyWorkspaceImage",
			op:   "CopyWorkspaceImage",
			body: map[string]any{
				"Name":          "copied-image",
				"SourceImageId": "wsi-source",
				"SourceRegion":  "us-west-2",
				"Description":   "A copied image",
			},
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var out map[string]string
				decodeJSON(t, body, &out)
				if out["ImageId"] == "" {
					t.Fatal("expected ImageId")
				}
			},
		},
		{
			name: "CreateWorkspaceImage",
			op:   "CreateWorkspaceImage",
			body: map[string]any{
				"Name":        "new-image",
				"Description": "from workspace",
				"WorkspaceId": wsID,
			},
			check: func(t *testing.T, body []byte) {
				t.Helper()
				// Created is a wire-format epoch-seconds number (not a
				// string), so decode into map[string]any rather than
				// map[string]string.
				var out map[string]any
				decodeJSON(t, body, &out)
				if out["ImageId"] == "" {
					t.Fatal("expected ImageId")
				}
				if out["State"] != "AVAILABLE" {
					t.Fatalf("expected AVAILABLE state, got %v", out["State"])
				}
				if created, ok := out["Created"].(float64); !ok || created <= 0 {
					t.Fatalf("expected positive numeric Created, got %v", out["Created"])
				}
			},
		},
		{
			name: "ImportWorkspaceImage",
			op:   "ImportWorkspaceImage",
			body: map[string]any{
				"Ec2ImageId":       "ami-12345678",
				"ImageName":        "imported",
				"ImageDescription": "ec2 import",
				"IngestionProcess": "BYOL_REGULAR",
			},
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var out map[string]string
				decodeJSON(t, body, &out)
				if out["ImageId"] == "" {
					t.Fatal("expected ImageId")
				}
			},
		},
		{
			name: "ImportCustomWorkspaceImage",
			op:   "ImportCustomWorkspaceImage",
			body: map[string]any{
				"ImageName":                      "custom-img",
				"ImageDescription":               "custom",
				"ComputeType":                    "BASE",
				"ImageSource":                    map[string]any{"Ec2ImageId": "ami-custom"},
				"InfrastructureConfigurationArn": "arn:aws:imagebuilder:us-east-1:000000000000:infrastructure-configuration/test",
				"OsVersion":                      "Windows_11",
				"Platform":                       "WINDOWS",
				"Protocol":                       "DCV",
			},
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var out map[string]string
				decodeJSON(t, body, &out)
				if out["ImageId"] == "" {
					t.Fatal("expected ImageId")
				}
			},
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doTargetRequest(t, h, tc.op, tc.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body)
			}

			tc.check(t, rec.Body.Bytes())
		})
	}
}

// TestWorkspaceImageDescribeAndPermissions exercises describe/permission/update
// flows for a previously created image.
func TestWorkspaceImageDescribeAndPermissions(
	t *testing.T,
) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)

	// Create an image. CopyWorkspaceImage validates SourceImageId against
	// b.images when SourceRegion matches this backend's own region, so use
	// one actually created rather than a made-up ID.
	rec := doTargetRequest(t, h, "CopyWorkspaceImage", map[string]any{
		"Name":          "perm-test",
		"SourceImageId": createImage(t, h),
		"SourceRegion":  "us-east-1",
	})
	var createOut map[string]string
	decodeJSON(t, rec.Body.Bytes(), &createOut)
	imageID := createOut["ImageId"]

	// Describe images
	rec2 := doTargetRequest(t, h, "DescribeWorkspaceImages", map[string]any{
		"ImageIds": []string{imageID},
	})
	if rec2.Code != http.StatusOK {
		t.Fatalf("describe images: expected 200, got %d", rec2.Code)
	}

	var descOut struct {
		Images []map[string]any `json:"Images"`
	}
	decodeJSON(t, rec2.Body.Bytes(), &descOut)

	if len(descOut.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(descOut.Images))
	}

	// Update permission
	rec3 := doTargetRequest(t, h, "UpdateWorkspaceImagePermission", map[string]any{
		"ImageId":         imageID,
		"SharedAccountId": "999988887777",
		"AllowCopyImage":  true,
	})
	if rec3.Code != http.StatusOK {
		t.Fatalf("update permission: expected 200, got %d", rec3.Code)
	}

	// Describe permissions
	rec4 := doTargetRequest(t, h, "DescribeWorkspaceImagePermissions", map[string]any{
		"ImageId": imageID,
	})
	if rec4.Code != http.StatusOK {
		t.Fatalf("describe perms: expected 200, got %d", rec4.Code)
	}

	var permsOut struct {
		ImageId          string           `json:"ImageId"` //nolint:revive,staticcheck // existing issue.
		ImagePermissions []map[string]any `json:"ImagePermissions"`
	}
	decodeJSON(t, rec4.Body.Bytes(), &permsOut)

	if len(permsOut.ImagePermissions) != 1 {
		t.Fatalf("expected 1 permission, got %d", len(permsOut.ImagePermissions))
	}

	// DescribeCustomWorkspaceImageImport
	rec5 := doTargetRequest(t, h, "DescribeCustomWorkspaceImageImport", map[string]any{
		"ImageId": imageID,
	})
	if rec5.Code != http.StatusOK {
		t.Fatalf("describe custom import: expected 200, got %d", rec5.Code)
	}

	// CreateUpdatedWorkspaceImage
	rec6 := doTargetRequest(t, h, "CreateUpdatedWorkspaceImage", map[string]any{
		"SourceImageId": imageID,
		"Name":          "updated",
		"Description":   "updated version",
	})
	if rec6.Code != http.StatusOK {
		t.Fatalf("create updated image: expected 200, got %d", rec6.Code)
	}

	// Delete image
	rec7 := doTargetRequest(t, h, "DeleteWorkspaceImage", map[string]any{
		"ImageId": imageID,
	})
	if rec7.Code != http.StatusOK {
		t.Fatalf("delete image: expected 200, got %d", rec7.Code)
	}
}

func createWorkspaceImageReq(workspaceID string) map[string]any {
	return map[string]any{
		"Name":        "validation-test",
		"Description": "test",
		"WorkspaceId": workspaceID,
	}
}

// TestDeleteWorkspaceImage_InUse verifies DeleteWorkspaceImage rejects an
// image still referenced by a custom bundle's ImageId: "To delete an image,
// you must first delete any bundles that are associated with the image"
// (api_op_DeleteWorkspaceImage.go doc comment).
func TestDeleteWorkspaceImage_InUse(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)
	imageID := createImage(t, h)

	rec := doTargetRequest(t, h, "CreateWorkspaceBundle", createBundleReq(imageID))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

	recDelete := doTargetRequest(t, h, "DeleteWorkspaceImage", map[string]any{
		"ImageId": imageID,
	})
	assert.Equal(t, http.StatusBadRequest, recDelete.Code,
		"deleting an image referenced by a bundle must fail: body: %s", recDelete.Body)
}

// TestCreateWorkspaceImage_WorkspaceIDValidation verifies CreateWorkspaceImage
// rejects a WorkspaceId that doesn't reference a real workspace and accepts
// one that does -- ResourceNotFoundException is in this operation's real
// error list (aws-sdk-go-v2/service/workspaces@v1.73.1 deserializers.go's
// awsAwsjson11_deserializeOpErrorCreateWorkspaceImage).
func TestCreateWorkspaceImage_WorkspaceIDValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		workspaceID func(t *testing.T, h *workspaces.Handler) string
		name        string
		wantCode    int
	}{
		{
			name: "missing workspace rejects",
			workspaceID: func(t *testing.T, _ *workspaces.Handler) string {
				t.Helper()

				return "ws-doesnotexist"
			},
			wantCode: http.StatusNotFound,
		},
		{
			name: "valid workspace succeeds",
			workspaceID: func(t *testing.T, h *workspaces.Handler) string {
				t.Helper()

				return createWorkspace(t, h)
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandlerWithBackend(t)

			rec := doTargetRequest(
				t, h, "CreateWorkspaceImage", createWorkspaceImageReq(tc.workspaceID(t, h)),
			)

			require.Equal(t, tc.wantCode, rec.Code, rec.Body.String())
		})
	}
}

// TestCreateWorkspaceImage_UnknownWorkspace_ConsumesNoState verifies a
// rejected CreateWorkspaceImage call leaves nothing behind: no image
// appears in DescribeWorkspaceImages, and the shared ID counter (store.go's
// nextID) isn't advanced, proving the workspace-existence check runs before
// createImageLocked's nextID call, not after.
func TestCreateWorkspaceImage_UnknownWorkspace_ConsumesNoState(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)
	wsID := createWorkspace(t, h)

	rec1 := doTargetRequest(t, h, "CreateWorkspaceImage", createWorkspaceImageReq(wsID))
	require.Equal(t, http.StatusOK, rec1.Code, rec1.Body.String())

	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &out1))
	firstID, _ := out1["ImageId"].(string)
	require.NotEmpty(t, firstID)

	describeRec := doTargetRequest(t, h, "DescribeWorkspaceImages", map[string]any{})
	require.Equal(t, http.StatusOK, describeRec.Code)

	var before struct {
		Images []map[string]any `json:"Images"`
	}
	require.NoError(t, json.Unmarshal(describeRec.Body.Bytes(), &before))

	rejectedRec := doTargetRequest(
		t, h, "CreateWorkspaceImage", createWorkspaceImageReq("ws-doesnotexist"),
	)
	require.Equal(t, http.StatusNotFound, rejectedRec.Code)

	describeRec2 := doTargetRequest(t, h, "DescribeWorkspaceImages", map[string]any{})
	require.Equal(t, http.StatusOK, describeRec2.Code)

	var after struct {
		Images []map[string]any `json:"Images"`
	}
	require.NoError(t, json.Unmarshal(describeRec2.Body.Bytes(), &after))

	assert.Len(t, after.Images, len(before.Images), "rejected create must not add an image")

	rec2 := doTargetRequest(t, h, "CreateWorkspaceImage", createWorkspaceImageReq(wsID))
	require.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))
	secondID, _ := out2["ImageId"].(string)
	require.NotEmpty(t, secondID)

	assert.Equal(
		t,
		idCounterSuffix(t, firstID, "wsi-")+1,
		idCounterSuffix(t, secondID, "wsi-"),
		"rejected create must not consume an ID from the shared counter",
	)
}

func createCopyImageReq(sourceImageID, sourceRegion string) map[string]any {
	return map[string]any{
		"Name":          "validation-test",
		"SourceImageId": sourceImageID,
		"SourceRegion":  sourceRegion,
	}
}

// TestCopyWorkspaceImage_SourceImageIDValidation verifies CopyWorkspaceImage
// rejects a SourceImageId that doesn't reference a real image when
// SourceRegion is empty or matches this backend's own region ("us-east-1"
// in tests), and that it deliberately does NOT reject an unknown
// SourceImageId when SourceRegion names a different region -- a real
// cross-region source image legitimately lives in a different backend
// instance this one cannot see (see the CopyWorkspaceImage doc comment).
// ResourceNotFoundException is in this operation's real error list
// (aws-sdk-go-v2/service/workspaces@v1.73.1 deserializers.go's
// awsAwsjson11_deserializeOpErrorCopyWorkspaceImage).
func TestCopyWorkspaceImage_SourceImageIDValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sourceImageID func(t *testing.T, h *workspaces.Handler) string
		name          string
		sourceRegion  string
		wantCode      int
	}{
		{
			name: "missing image same region rejects",
			sourceImageID: func(t *testing.T, _ *workspaces.Handler) string {
				t.Helper()

				return "wsi-doesnotexist"
			},
			sourceRegion: "us-east-1",
			wantCode:     http.StatusNotFound,
		},
		{
			name: "missing image empty region rejects",
			sourceImageID: func(t *testing.T, _ *workspaces.Handler) string {
				t.Helper()

				return "wsi-doesnotexist"
			},
			sourceRegion: "",
			wantCode:     http.StatusNotFound,
		},
		{
			name: "valid image same region succeeds",
			sourceImageID: func(t *testing.T, h *workspaces.Handler) string {
				t.Helper()

				return createImage(t, h)
			},
			sourceRegion: "us-east-1",
			wantCode:     http.StatusOK,
		},
		{
			name: "missing image cross region succeeds",
			sourceImageID: func(t *testing.T, _ *workspaces.Handler) string {
				t.Helper()

				return "wsi-doesnotexist"
			},
			sourceRegion: "us-west-2",
			wantCode:     http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandlerWithBackend(t)

			rec := doTargetRequest(
				t, h, "CopyWorkspaceImage",
				createCopyImageReq(tc.sourceImageID(t, h), tc.sourceRegion),
			)

			require.Equal(t, tc.wantCode, rec.Code, rec.Body.String())
		})
	}
}

// TestCopyWorkspaceImage_UnknownSourceImage_ConsumesNoState verifies a
// same-region CopyWorkspaceImage call rejected for an unknown SourceImageId
// leaves nothing behind: no image appears in DescribeWorkspaceImages, and
// the shared ID counter (store.go's nextID) isn't advanced, proving the
// existence check runs before createImageLocked's nextID call, not after.
func TestCopyWorkspaceImage_UnknownSourceImage_ConsumesNoState(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)
	sourceID := createImage(t, h)

	rec1 := doTargetRequest(t, h, "CopyWorkspaceImage", createCopyImageReq(sourceID, "us-east-1"))
	require.Equal(t, http.StatusOK, rec1.Code, rec1.Body.String())

	var out1 map[string]string
	decodeJSON(t, rec1.Body.Bytes(), &out1)
	firstID := out1["ImageId"]
	require.NotEmpty(t, firstID)

	describeRec := doTargetRequest(t, h, "DescribeWorkspaceImages", map[string]any{})
	require.Equal(t, http.StatusOK, describeRec.Code)

	var before struct {
		Images []map[string]any `json:"Images"`
	}
	require.NoError(t, json.Unmarshal(describeRec.Body.Bytes(), &before))

	rejectedRec := doTargetRequest(
		t, h, "CopyWorkspaceImage", createCopyImageReq("wsi-doesnotexist", "us-east-1"),
	)
	require.Equal(t, http.StatusNotFound, rejectedRec.Code)

	describeRec2 := doTargetRequest(t, h, "DescribeWorkspaceImages", map[string]any{})
	require.Equal(t, http.StatusOK, describeRec2.Code)

	var after struct {
		Images []map[string]any `json:"Images"`
	}
	require.NoError(t, json.Unmarshal(describeRec2.Body.Bytes(), &after))

	assert.Len(t, after.Images, len(before.Images), "rejected copy must not add an image")

	rec2 := doTargetRequest(t, h, "CopyWorkspaceImage", createCopyImageReq(sourceID, "us-east-1"))
	require.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())

	var out2 map[string]string
	decodeJSON(t, rec2.Body.Bytes(), &out2)
	secondID := out2["ImageId"]
	require.NotEmpty(t, secondID)

	assert.Equal(
		t,
		idCounterSuffix(t, firstID, "wsi-")+1,
		idCounterSuffix(t, secondID, "wsi-"),
		"rejected copy must not consume an ID from the shared counter",
	)
}

func createUpdatedImageReq(sourceImageID string) map[string]any {
	return map[string]any{
		"SourceImageId": sourceImageID,
		"Name":          "validation-test",
		"Description":   "test",
	}
}

// TestCreateUpdatedWorkspaceImage_SourceImageIDValidation verifies
// CreateUpdatedWorkspaceImage rejects a SourceImageId that doesn't
// reference a real image and accepts one that does --
// ResourceNotFoundException is in this operation's real error list
// (aws-sdk-go-v2/service/workspaces@v1.73.1 deserializers.go's
// awsAwsjson11_deserializeOpErrorCreateUpdatedWorkspaceImage).
func TestCreateUpdatedWorkspaceImage_SourceImageIDValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sourceImageID func(t *testing.T, h *workspaces.Handler) string
		name          string
		wantCode      int
	}{
		{
			name: "missing image rejects",
			sourceImageID: func(t *testing.T, _ *workspaces.Handler) string {
				t.Helper()

				return "wsi-doesnotexist"
			},
			wantCode: http.StatusNotFound,
		},
		{
			name: "valid image succeeds",
			sourceImageID: func(t *testing.T, h *workspaces.Handler) string {
				t.Helper()

				return createImage(t, h)
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandlerWithBackend(t)

			rec := doTargetRequest(
				t, h, "CreateUpdatedWorkspaceImage", createUpdatedImageReq(tc.sourceImageID(t, h)),
			)

			require.Equal(t, tc.wantCode, rec.Code, rec.Body.String())
		})
	}
}

// TestCreateUpdatedWorkspaceImage_UnknownSourceImage_ConsumesNoState
// verifies a rejected CreateUpdatedWorkspaceImage call leaves nothing
// behind: no image appears in DescribeWorkspaceImages, and the shared ID
// counter (store.go's nextID) isn't advanced, proving the existence check
// runs before createImageLocked's nextID call, not after.
func TestCreateUpdatedWorkspaceImage_UnknownSourceImage_ConsumesNoState(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)
	sourceID := createImage(t, h)

	rec1 := doTargetRequest(t, h, "CreateUpdatedWorkspaceImage", createUpdatedImageReq(sourceID))
	require.Equal(t, http.StatusOK, rec1.Code, rec1.Body.String())

	var out1 map[string]string
	decodeJSON(t, rec1.Body.Bytes(), &out1)
	firstID := out1["ImageId"]
	require.NotEmpty(t, firstID)

	describeRec := doTargetRequest(t, h, "DescribeWorkspaceImages", map[string]any{})
	require.Equal(t, http.StatusOK, describeRec.Code)

	var before struct {
		Images []map[string]any `json:"Images"`
	}
	require.NoError(t, json.Unmarshal(describeRec.Body.Bytes(), &before))

	rejectedRec := doTargetRequest(
		t, h, "CreateUpdatedWorkspaceImage", createUpdatedImageReq("wsi-doesnotexist"),
	)
	require.Equal(t, http.StatusNotFound, rejectedRec.Code)

	describeRec2 := doTargetRequest(t, h, "DescribeWorkspaceImages", map[string]any{})
	require.Equal(t, http.StatusOK, describeRec2.Code)

	var after struct {
		Images []map[string]any `json:"Images"`
	}
	require.NoError(t, json.Unmarshal(describeRec2.Body.Bytes(), &after))

	assert.Len(t, after.Images, len(before.Images), "rejected create must not add an image")

	rec2 := doTargetRequest(t, h, "CreateUpdatedWorkspaceImage", createUpdatedImageReq(sourceID))
	require.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())

	var out2 map[string]string
	decodeJSON(t, rec2.Body.Bytes(), &out2)
	secondID := out2["ImageId"]
	require.NotEmpty(t, secondID)

	assert.Equal(
		t,
		idCounterSuffix(t, firstID, "wsi-")+1,
		idCounterSuffix(t, secondID, "wsi-"),
		"rejected create must not consume an ID from the shared counter",
	)
}

// TestDescribeWorkspaceImagePermissions_Pagination proves the op pages
// through every shared-account permission exactly once instead of returning
// them all on a single page with no cursor.
func TestDescribeWorkspaceImagePermissions_Pagination(t *testing.T) {
	t.Parallel()

	client := newTestHandlerAndClient(t)
	ctx := t.Context()

	copyOut, err := client.CopyWorkspaceImage(ctx, &wssdk.CopyWorkspaceImageInput{
		Name:          aws.String("copied-image"),
		SourceImageId: aws.String("wsi-source"),
		SourceRegion:  aws.String("us-west-2"),
	})
	require.NoError(t, err)
	imageID := copyOut.ImageId

	sharedAccounts := []string{"111111111111", "222222222222", "333333333333"}
	for _, acct := range sharedAccounts {
		_, updateErr := client.UpdateWorkspaceImagePermission(ctx, &wssdk.UpdateWorkspaceImagePermissionInput{
			ImageId:         imageID,
			SharedAccountId: aws.String(acct),
			AllowCopyImage:  aws.Bool(true),
		})
		require.NoError(t, updateErr)
	}

	page1, err := client.DescribeWorkspaceImagePermissions(ctx, &wssdk.DescribeWorkspaceImagePermissionsInput{
		ImageId:    imageID,
		MaxResults: aws.Int32(2),
	})
	require.NoError(t, err)
	require.Len(t, page1.ImagePermissions, 2)
	require.NotNil(t, page1.NextToken, "first page must return a cursor when more permissions remain")

	page2, err := client.DescribeWorkspaceImagePermissions(ctx, &wssdk.DescribeWorkspaceImagePermissionsInput{
		ImageId:    imageID,
		MaxResults: aws.Int32(2),
		NextToken:  page1.NextToken,
	})
	require.NoError(t, err)
	require.Len(t, page2.ImagePermissions, 1)
	require.Empty(t, aws.ToString(page2.NextToken))

	seen := map[string]bool{}
	for _, p := range page1.ImagePermissions {
		seen[aws.ToString(p.SharedAccountId)] = true
	}

	for _, p := range page2.ImagePermissions {
		acct := aws.ToString(p.SharedAccountId)
		require.False(t, seen[acct], "account %s returned on both pages", acct)
		seen[acct] = true
	}

	require.Len(t, seen, len(sharedAccounts))
	for _, acct := range sharedAccounts {
		require.True(t, seen[acct])
	}
}

// TestDescribeWorkspaceImages_Pagination proves the op pages through every
// image exactly once instead of returning them all on a single page with no
// cursor.
func TestDescribeWorkspaceImages_Pagination(t *testing.T) {
	t.Parallel()

	client := newTestHandlerAndClient(t)
	ctx := t.Context()

	names := []string{"image-a", "image-b", "image-c"}
	for _, n := range names {
		_, err := client.CopyWorkspaceImage(ctx, &wssdk.CopyWorkspaceImageInput{
			Name:          aws.String(n),
			SourceImageId: aws.String("wsi-source"),
			SourceRegion:  aws.String("us-west-2"),
		})
		require.NoError(t, err)
	}

	page1, err := client.DescribeWorkspaceImages(ctx, &wssdk.DescribeWorkspaceImagesInput{
		MaxResults: aws.Int32(2),
	})
	require.NoError(t, err)
	require.Len(t, page1.Images, 2)
	require.NotNil(t, page1.NextToken, "first page must return a cursor when more images remain")

	page2, err := client.DescribeWorkspaceImages(ctx, &wssdk.DescribeWorkspaceImagesInput{
		MaxResults: aws.Int32(2),
		NextToken:  page1.NextToken,
	})
	require.NoError(t, err)
	require.Len(t, page2.Images, 1)
	require.Empty(t, aws.ToString(page2.NextToken))

	seen := map[string]bool{}
	for _, img := range page1.Images {
		seen[aws.ToString(img.ImageId)] = true
	}

	for _, img := range page2.Images {
		id := aws.ToString(img.ImageId)
		require.False(t, seen[id], "image %s returned on both pages", id)
		seen[id] = true
	}

	require.Len(t, seen, len(names))
}
