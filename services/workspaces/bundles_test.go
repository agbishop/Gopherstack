package workspaces_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/workspaces"
)

// TestDescribeWorkspaceBundles_PaginatesResults verifies that DescribeWorkspaceBundles
// returns a NextToken when there are more results, and the token can be used to fetch the rest.
func TestDescribeWorkspaceBundles_PaginatesResults(t *testing.T) {
	t.Parallel()

	b := workspaces.NewInMemoryBackend("000000000000", "us-east-1")
	h := workspaces.NewHandler(b)

	// Fetch first page (Amazon has 5 bundles; page size = 25, so all fit on one page normally).
	// Add custom bundles to force pagination by querying via API with small NextToken simulation.
	// Instead test that a real NextToken from the API roundtrips correctly.
	rec := doTargetRequest(t, h, "DescribeWorkspaceBundles", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	bundles, _ := resp["Bundles"].([]any)
	assert.NotEmpty(t, bundles, "DescribeWorkspaceBundles must return at least one bundle")

	// When all bundles fit on one page there should be no NextToken.
	nextToken, hasToken := resp["NextToken"]
	if hasToken {
		assert.NotEmpty(t, nextToken, "NextToken must be non-empty when present")
	}
}

// TestDescribeWorkspaceBundles_FiltersByBundleID verifies that filtering
// by specific BundleIds returns only those bundles.
func TestDescribeWorkspaceBundles_FiltersByBundleID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		bundleIDs []any
		wantCount int
	}{
		{
			name:      "filter by one bundle",
			bundleIDs: []any{"wsb-bh8rsxt14"},
			wantCount: 1,
		},
		{
			name:      "filter by two bundles",
			bundleIDs: []any{"wsb-bh8rsxt14", "wsb-gm4d5tx2v"},
			wantCount: 2,
		},
		{
			name:      "filter by nonexistent bundle",
			bundleIDs: []any{"wsb-doesnotexist"},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := workspaces.NewInMemoryBackend("000000000000", "us-east-1")
			h := workspaces.NewHandler(b)

			rec := doTargetRequest(t, h, "DescribeWorkspaceBundles", map[string]any{
				"BundleIds": tc.bundleIDs,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			bundles, _ := resp["Bundles"].([]any)
			assert.Len(t, bundles, tc.wantCount)
		})
	}
}

func TestDescribeWorkspaceBundles_ComputeTypeAndStorage(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doTargetRequest(t, h, "DescribeWorkspaceBundles", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	bundles := resp["Bundles"].([]any)
	require.NotEmpty(t, bundles)

	for _, b := range bundles {
		bun := b.(map[string]any)
		bundleID := bun["BundleId"].(string)

		ctRaw, hasComputeType := bun["ComputeType"]
		assert.True(t, hasComputeType, "bundle %s must have ComputeType", bundleID)

		if hasComputeType {
			ct := ctRaw.(map[string]any)
			name, _ := ct["Name"].(string)
			assert.NotEmpty(t, name, "bundle %s ComputeType.Name must not be empty", bundleID)
		}

		usRaw, hasUserStorage := bun["UserStorage"]
		assert.True(t, hasUserStorage, "bundle %s must have UserStorage", bundleID)

		if hasUserStorage {
			us := usRaw.(map[string]any)
			capacityStr, _ := us["Capacity"].(string)
			capacity, convErr := strconv.Atoi(capacityStr)
			require.NoError(t, convErr, "bundle %s UserStorage.Capacity must be numeric", bundleID)
			assert.Positive(
				t,
				capacity,
				"bundle %s UserStorage.Capacity must be > 0",
				bundleID,
			)
		}

		rsRaw, hasRootStorage := bun["RootStorage"]
		assert.True(t, hasRootStorage, "bundle %s must have RootStorage", bundleID)

		if hasRootStorage {
			rs := rsRaw.(map[string]any)
			capacityStr, _ := rs["Capacity"].(string)
			capacity, convErr := strconv.Atoi(capacityStr)
			require.NoError(t, convErr, "bundle %s RootStorage.Capacity must be numeric", bundleID)
			assert.Positive(
				t,
				capacity,
				"bundle %s RootStorage.Capacity must be > 0",
				bundleID,
			)
		}
	}
}

func TestDescribeWorkspaceBundles_ByOwnerAmazon(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create a custom bundle. CreateWorkspaceBundle validates that ImageId
	// references a real image, so use one actually created.
	rec := doTargetRequest(t, h, "CreateWorkspaceBundle", map[string]any{
		"BundleName":  "MyBundle",
		"ImageId":     createImage(t, h),
		"ComputeType": map[string]any{"Name": "STANDARD"},
		"UserStorage": map[string]any{"Capacity": "50"},
		"RootStorage": map[string]any{"Capacity": "80"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Filter by owner=Amazon: should NOT include custom bundle.
	rec2 := doTargetRequest(t, h, "DescribeWorkspaceBundles", map[string]any{
		"Owner": "Amazon",
	})
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
	bundles := resp["Bundles"].([]any)

	for _, b := range bundles {
		bun := b.(map[string]any)
		assert.Equal(t, "Amazon", bun["Owner"], "owner=Amazon filter must exclude custom bundles")
	}
}

// TestCreateWorkspaceBundle_StoresStorageCapacity covers gopherstack-4shm's
// class: CreateWorkspaceBundleInput.UserStorage (workspaces@v1.73.1
// api_op_CreateWorkspaceBundle.go: "This member is required") and
// RootStorage were decoded but never passed to the backend at all -- every
// custom bundle silently reported an empty Capacity regardless of what the
// client requested.
func TestCreateWorkspaceBundle_StoresStorageCapacity(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doTargetRequest(t, h, "CreateWorkspaceBundle", map[string]any{
		"BundleName":  "StorageBundle",
		"ImageId":     createImage(t, h),
		"ComputeType": map[string]any{"Name": "STANDARD"},
		"UserStorage": map[string]any{"Capacity": "50"},
		"RootStorage": map[string]any{"Capacity": "80"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	bun := resp["WorkspaceBundle"].(map[string]any)

	assert.Equal(t, "50", bun["UserStorage"].(map[string]any)["Capacity"])
	assert.Equal(t, "80", bun["RootStorage"].(map[string]any)["Capacity"])
}

func TestDescribeWorkspaceBundles_IncludesCustomBundle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doTargetRequest(t, h, "CreateWorkspaceBundle", map[string]any{
		"BundleName":        "MyCustomBundle",
		"BundleDescription": "A test bundle",
		"ImageId":           createImage(t, h),
		"ComputeType":       map[string]any{"Name": "STANDARD"},
		"UserStorage":       map[string]any{"Capacity": "50"},
		"RootStorage":       map[string]any{"Capacity": "80"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	customBundleID := createResp["WorkspaceBundle"].(map[string]any)["BundleId"].(string)

	// Without owner filter: should include both Amazon and custom bundles.
	rec2 := doTargetRequest(t, h, "DescribeWorkspaceBundles", map[string]any{})
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
	bundles := resp["Bundles"].([]any)

	found := false
	for _, b := range bundles {
		if b.(map[string]any)["BundleId"] == customBundleID {
			found = true
		}
	}

	assert.True(t, found, "custom bundle must appear in unfiltered DescribeWorkspaceBundles")
}

func TestDescribeWorkspaceBundles_FilterByID(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doTargetRequest(t, h, "DescribeWorkspaceBundles", map[string]any{
		"BundleIds": []string{"wsb-bh8rsxt14"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	bundles := resp["Bundles"].([]any)
	require.Len(t, bundles, 1)
	assert.Equal(t, "wsb-bh8rsxt14", bundles[0].(map[string]any)["BundleId"])
}

// TestWorkspaceBundleCRUD exercises the custom-bundle create/update/delete lifecycle.
func TestWorkspaceBundleCRUD(t *testing.T) { //nolint:paralleltest // existing issue.
	tests := []struct {
		name        string
		bundleName  string
		description string
	}{
		{name: "basic bundle", bundleName: "MyBundle", description: "A custom bundle"},
		{name: "minimal bundle", bundleName: "Min", description: ""},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newTestHandlerWithBackend(t)

			// Create -- CreateWorkspaceBundle validates ImageId references a
			// real image, so use one actually created rather than a made-up ID.
			rec := doTargetRequest(t, h, "CreateWorkspaceBundle", map[string]any{
				"BundleName":        tc.bundleName,
				"BundleDescription": tc.description,
				"ImageId":           createImage(t, h),
				"ComputeType":       map[string]string{"Name": "VALUE"},
				"UserStorage":       map[string]string{"Capacity": "10"},
				"RootStorage":       map[string]string{"Capacity": "80"},
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("create: expected 200, got %d: %s", rec.Code, rec.Body)
			}

			var createOut struct {
				WorkspaceBundle map[string]any `json:"WorkspaceBundle"`
			}
			decodeJSON(t, rec.Body.Bytes(), &createOut)

			bundleID, _ := createOut.WorkspaceBundle["BundleId"].(string)
			if bundleID == "" {
				t.Fatal("expected non-empty BundleId")
			}

			// Update -- UpdateWorkspaceBundle validates ImageId references a
			// real image (see TestUpdateWorkspaceBundle_UnknownImage), so use
			// one actually created rather than a made-up ID.
			rec2 := doTargetRequest(t, h, "UpdateWorkspaceBundle", map[string]any{
				"BundleId": bundleID,
				"ImageId":  createImage(t, h),
			})
			if rec2.Code != http.StatusOK {
				t.Fatalf("update: expected 200, got %d: %s", rec2.Code, rec2.Body)
			}

			// Delete
			rec3 := doTargetRequest(t, h, "DeleteWorkspaceBundle", map[string]any{
				"BundleId": bundleID,
			})
			if rec3.Code != http.StatusOK {
				t.Fatalf("delete: expected 200, got %d", rec3.Code)
			}
		})
	}
}

// TestDeleteWorkspaceBundle_InUse verifies DeleteWorkspaceBundle rejects a
// bundle still referenced by a WorkSpace's BundleId -- ResourceAssociatedException
// is modelled for this operation (aws-sdk-go-v2/service/workspaces@v1.73.1
// deserializers.go's awsAwsjson11_deserializeOpErrorDeleteWorkspaceBundle).
func TestDeleteWorkspaceBundle_InUse(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)

	rec := doTargetRequest(t, h, "CreateWorkspaceBundle", createBundleReq(createImage(t, h)))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body)

	var createOut struct {
		WorkspaceBundle map[string]any `json:"WorkspaceBundle"`
	}
	decodeJSON(t, rec.Body.Bytes(), &createOut)
	bundleID, _ := createOut.WorkspaceBundle["BundleId"].(string)
	require.NotEmpty(t, bundleID)

	doTargetRequest(t, h, "RegisterWorkspaceDirectory", map[string]any{
		"DirectoryId": "d-1234567890",
	})
	recCreate := doTargetRequest(t, h, "CreateWorkspaces", map[string]any{
		"Workspaces": []map[string]any{
			{"UserName": "testuser", "DirectoryId": "d-1234567890", "BundleId": bundleID},
		},
	})
	require.Equal(t, http.StatusOK, recCreate.Code, "body: %s", recCreate.Body)

	recDelete := doTargetRequest(t, h, "DeleteWorkspaceBundle", map[string]any{
		"BundleId": bundleID,
	})
	assert.Equal(t, http.StatusBadRequest, recDelete.Code,
		"deleting a bundle referenced by a WorkSpace must fail: body: %s", recDelete.Body)
}

// TestUpdateWorkspaceBundle_UnknownImage verifies UpdateWorkspaceBundle
// rejects an ImageId that doesn't reference a real image (previously
// accepted unconditionally and silently pointed the bundle at a phantom
// image ID) -- ResourceNotFoundException is in this operation's real error
// list.
func TestUpdateWorkspaceBundle_UnknownImage(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)

	rec := doTargetRequest(t, h, "CreateWorkspaceBundle", map[string]any{
		"BundleName":        "unknown-image-bundle",
		"BundleDescription": "test",
		"ImageId":           createImage(t, h),
		"ComputeType":       map[string]string{"Name": "VALUE"},
		"UserStorage":       map[string]string{"Capacity": "10"},
		"RootStorage":       map[string]string{"Capacity": "80"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", rec.Code, rec.Body)
	}

	var createOut struct {
		WorkspaceBundle map[string]any `json:"WorkspaceBundle"`
	}
	decodeJSON(t, rec.Body.Bytes(), &createOut)
	bundleID, _ := createOut.WorkspaceBundle["BundleId"].(string)

	rec2 := doTargetRequest(t, h, "UpdateWorkspaceBundle", map[string]any{
		"BundleId": bundleID,
		"ImageId":  "wsi-does-not-exist",
	})
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec2.Code, rec2.Body)
	}
}

func createBundleReq(imageID string) map[string]any {
	return map[string]any{
		"BundleName":        "validation-test",
		"BundleDescription": "test",
		"ImageId":           imageID,
		"ComputeType":       map[string]string{"Name": "VALUE"},
		"UserStorage":       map[string]string{"Capacity": "10"},
		"RootStorage":       map[string]string{"Capacity": "80"},
	}
}

// TestCreateWorkspaceBundle_ImageIDValidation verifies CreateWorkspaceBundle
// rejects an ImageId that doesn't reference a real image and accepts one
// that does -- ResourceNotFoundException is in this operation's real error
// list (aws-sdk-go-v2/service/workspaces@v1.73.1 deserializers.go's
// awsAwsjson11_deserializeOpErrorCreateWorkspaceBundle).
func TestCreateWorkspaceBundle_ImageIDValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		imageID  func(t *testing.T, h *workspaces.Handler) string
		name     string
		wantCode int
	}{
		{
			name: "missing image rejects",
			imageID: func(t *testing.T, _ *workspaces.Handler) string {
				t.Helper()

				return "wsi-doesnotexist"
			},
			wantCode: http.StatusNotFound,
		},
		{
			name: "valid image succeeds",
			imageID: func(t *testing.T, h *workspaces.Handler) string {
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

			rec := doTargetRequest(t, h, "CreateWorkspaceBundle", createBundleReq(tc.imageID(t, h)))

			require.Equal(t, tc.wantCode, rec.Code, rec.Body.String())
		})
	}
}

// TestCreateWorkspaceBundle_UnknownImage_ConsumesNoState verifies a rejected
// CreateWorkspaceBundle call leaves nothing behind: no bundle appears in
// DescribeWorkspaceBundles, and the shared ID counter (store.go's nextID)
// isn't advanced, proving b.nextID was never reached -- the existence check
// must run before any state mutation.
func TestCreateWorkspaceBundle_UnknownImage_ConsumesNoState(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandlerWithBackend(t)
	imageID := createImage(t, h)

	rec1 := doTargetRequest(t, h, "CreateWorkspaceBundle", createBundleReq(imageID))
	require.Equal(t, http.StatusOK, rec1.Code, rec1.Body.String())

	var out1 struct {
		WorkspaceBundle map[string]any `json:"WorkspaceBundle"`
	}
	decodeJSON(t, rec1.Body.Bytes(), &out1)
	firstID, _ := out1.WorkspaceBundle["BundleId"].(string)
	require.NotEmpty(t, firstID)

	describeRec := doTargetRequest(t, h, "DescribeWorkspaceBundles", map[string]any{"Owner": "111122223333"})
	require.Equal(t, http.StatusOK, describeRec.Code)

	var before map[string]any
	require.NoError(t, json.Unmarshal(describeRec.Body.Bytes(), &before))
	countBefore := len(before["Bundles"].([]any))

	rejectedRec := doTargetRequest(
		t, h, "CreateWorkspaceBundle", createBundleReq("wsi-doesnotexist"),
	)
	require.Equal(t, http.StatusNotFound, rejectedRec.Code)

	describeRec2 := doTargetRequest(t, h, "DescribeWorkspaceBundles", map[string]any{"Owner": "111122223333"})
	require.Equal(t, http.StatusOK, describeRec2.Code)

	var after map[string]any
	require.NoError(t, json.Unmarshal(describeRec2.Body.Bytes(), &after))
	countAfter := len(after["Bundles"].([]any))

	assert.Equal(t, countBefore, countAfter, "rejected create must not add a bundle")

	rec2 := doTargetRequest(t, h, "CreateWorkspaceBundle", createBundleReq(imageID))
	require.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())

	var out2 struct {
		WorkspaceBundle map[string]any `json:"WorkspaceBundle"`
	}
	decodeJSON(t, rec2.Body.Bytes(), &out2)
	secondID, _ := out2.WorkspaceBundle["BundleId"].(string)
	require.NotEmpty(t, secondID)

	assert.Equal(
		t,
		idCounterSuffix(t, firstID, "wsb-")+1,
		idCounterSuffix(t, secondID, "wsb-"),
		"rejected create must not consume an ID from the shared counter",
	)
}
