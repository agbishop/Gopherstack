package directoryservice_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/directoryservice"
)

func TestDirectoryService_Snapshots(t *testing.T) {
	t.Parallel()

	createDir := func(h *directoryservice.Handler) string {
		rec := doRequest(t, h, "CreateDirectory", map[string]any{
			"Name": "corp.example.com", "Password": "Admin1234!", "Size": "Small",
		})
		var resp map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)

		return resp["DirectoryId"].(string)
	}

	t.Run("CreateSnapshot returns SnapshotId", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		dirID := createDir(h)
		rec := doRequest(t, h, "CreateSnapshot", map[string]any{
			"DirectoryId": dirID,
			"Name":        "my-snapshot",
		})
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		snapID, ok := resp["SnapshotId"].(string)
		require.True(t, ok)
		assert.NotEmpty(t, snapID)
	})

	t.Run("CreateSnapshot unknown directory returns 400", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		rec := doRequest(t, h, "CreateSnapshot", map[string]any{"DirectoryId": "d-0000000000"})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("DeleteSnapshot removes snapshot", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		dirID := createDir(h)
		snapRec := doRequest(t, h, "CreateSnapshot", map[string]any{"DirectoryId": dirID})
		var snapResp map[string]any
		require.NoError(t, json.Unmarshal(snapRec.Body.Bytes(), &snapResp))
		snapID := snapResp["SnapshotId"].(string)

		rec := doRequest(t, h, "DeleteSnapshot", map[string]any{"SnapshotId": snapID})
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify it's gone.
		descRec := doRequest(t, h, "DescribeSnapshots", map[string]any{"SnapshotIds": []string{snapID}})
		assert.Equal(t, http.StatusOK, descRec.Code)
		var descResp map[string]any
		require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
		snaps := descResp["Snapshots"].([]any)
		assert.Empty(t, snaps)
	})

	t.Run("DeleteSnapshot unknown returns 400", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		rec := doRequest(t, h, "DeleteSnapshot", map[string]any{"SnapshotId": "s-0000000000"})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("DescribeSnapshots filters by directory", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		dirID1 := createDir(h)
		dirID2 := func() string {
			rec := doRequest(t, h, "CreateDirectory", map[string]any{
				"Name": "other.example.com", "Password": "Admin1234!", "Size": "Small",
			})
			var resp map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)

			return resp["DirectoryId"].(string)
		}()

		doRequest(t, h, "CreateSnapshot", map[string]any{"DirectoryId": dirID1})
		doRequest(t, h, "CreateSnapshot", map[string]any{"DirectoryId": dirID2})

		rec := doRequest(t, h, "DescribeSnapshots", map[string]any{"DirectoryId": dirID1})
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		snaps := resp["Snapshots"].([]any)
		assert.Len(t, snaps, 1)
	})

	t.Run("GetSnapshotLimits returns limits", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		dirID := createDir(h)
		doRequest(t, h, "CreateSnapshot", map[string]any{"DirectoryId": dirID})

		rec := doRequest(t, h, "GetSnapshotLimits", map[string]any{"DirectoryId": dirID})
		assert.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		limits := resp["SnapshotLimits"].(map[string]any)
		assert.EqualValues(t, 1, limits["ManualSnapshotsCurrentCount"])
	})

	t.Run("RestoreFromSnapshot succeeds on existing snapshot", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		dirID := createDir(h)
		snapRec := doRequest(t, h, "CreateSnapshot", map[string]any{"DirectoryId": dirID})
		var snapResp map[string]any
		require.NoError(t, json.Unmarshal(snapRec.Body.Bytes(), &snapResp))
		snapID := snapResp["SnapshotId"].(string)

		rec := doRequest(t, h, "RestoreFromSnapshot", map[string]any{"SnapshotId": snapID})
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("RestoreFromSnapshot unknown returns 400", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		rec := doRequest(t, h, "RestoreFromSnapshot", map[string]any{"SnapshotId": "s-0000000000"})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestRestoreFromSnapshot_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(h *directoryservice.Handler) string
		name     string
		wantCode int
	}{
		{
			name: "valid snapshot restores directory",
			setup: func(h *directoryservice.Handler) string {
				createRec := doRequest(t, h, "CreateDirectory", map[string]any{
					"Name": "corp.example.com", "Password": "Admin1234!", "Size": "Small",
				})
				var createResp map[string]any
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
				dirID := createResp["DirectoryId"].(string)
				snapRec := doRequest(t, h, "CreateSnapshot", map[string]any{"DirectoryId": dirID})
				var snapResp map[string]any
				require.NoError(t, json.Unmarshal(snapRec.Body.Bytes(), &snapResp))

				return snapResp["SnapshotId"].(string)
			},
			wantCode: http.StatusOK,
		},
		{
			name: "non-existent snapshot returns 400",
			setup: func(_ *directoryservice.Handler) string {
				return "s-0000000000"
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "orphaned snapshot (directory deleted) returns 400",
			setup: func(h *directoryservice.Handler) string {
				createRec := doRequest(t, h, "CreateDirectory", map[string]any{
					"Name": "corp.example.com", "Password": "Admin1234!", "Size": "Small",
				})
				var createResp map[string]any
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
				dirID := createResp["DirectoryId"].(string)
				snapRec := doRequest(t, h, "CreateSnapshot", map[string]any{"DirectoryId": dirID})
				var snapResp map[string]any
				require.NoError(t, json.Unmarshal(snapRec.Body.Bytes(), &snapResp))
				snapID := snapResp["SnapshotId"].(string)
				doRequest(t, h, "DeleteDirectory", map[string]any{"DirectoryId": dirID})

				return snapID
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			snapID := tt.setup(h)
			rec := doRequest(t, h, "RestoreFromSnapshot", map[string]any{"SnapshotId": snapID})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestCreateSnapshot_LimitEnforcement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantType string
		wantCode int
	}{
		{name: "5 snapshots succeeds", wantCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			dirID := mustCreateSimpleAD(t, h, "corp.example.com")

			for i := range 5 {
				rec := doRequest(t, h, "CreateSnapshot", map[string]any{
					"DirectoryId": dirID,
					"Name":        fmt.Sprintf("snap-%d", i),
				})
				assert.Equal(t, tt.wantCode, rec.Code, "snapshot %d", i)
			}
		})
	}

	t.Run("6th snapshot returns SnapshotLimitExceededException", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		dirID := mustCreateSimpleAD(t, h, "corp.example.com")

		for i := range 5 {
			rec := doRequest(t, h, "CreateSnapshot", map[string]any{
				"DirectoryId": dirID,
				"Name":        fmt.Sprintf("snap-%d", i),
			})
			require.Equal(t, http.StatusOK, rec.Code, "snapshot %d should succeed", i)
		}

		rec := doRequest(t, h, "CreateSnapshot", map[string]any{
			"DirectoryId": dirID,
			"Name":        "overflow",
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		body := respBody(t, rec)
		assert.Equal(t, "SnapshotLimitExceededException", body["__type"])
	})

	t.Run("snapshot limit is per directory", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		dir1 := mustCreateSimpleAD(t, h, "corp1.example.com")
		dir2 := mustCreateSimpleAD(t, h, "corp2.example.com")

		for i := range 5 {
			rec := doRequest(
				t,
				h,
				"CreateSnapshot",
				map[string]any{"DirectoryId": dir1, "Name": fmt.Sprintf("s%d", i)},
			)
			require.Equal(t, http.StatusOK, rec.Code)
		}
		// dir2 is unaffected by dir1's snapshots
		rec := doRequest(
			t,
			h,
			"CreateSnapshot",
			map[string]any{"DirectoryId": dir2, "Name": "first"},
		)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("deleting snapshot frees slot", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		dirID := mustCreateSimpleAD(t, h, "corp.example.com")

		snapIDs := make([]string, 0, 5)
		for i := range 5 {
			rec := doRequest(
				t,
				h,
				"CreateSnapshot",
				map[string]any{"DirectoryId": dirID, "Name": fmt.Sprintf("s%d", i)},
			)
			require.Equal(t, http.StatusOK, rec.Code)
			body := respBody(t, rec)
			snapIDs = append(snapIDs, body["SnapshotId"].(string))
		}

		// Delete one to free up a slot
		delRec := doRequest(t, h, "DeleteSnapshot", map[string]any{"SnapshotId": snapIDs[0]})
		require.Equal(t, http.StatusOK, delRec.Code)

		// Now a new one should succeed
		rec := doRequest(
			t,
			h,
			"CreateSnapshot",
			map[string]any{"DirectoryId": dirID, "Name": "new-snap"},
		)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

// --- Cascade delete ---

func TestDescribeSnapshots_Pagination(t *testing.T) {
	t.Parallel()

	t.Run("paginate through snapshots", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		dirID := mustCreateSimpleAD(t, h, "corp.example.com")

		for i := range 4 {
			rec := doRequest(
				t,
				h,
				"CreateSnapshot",
				map[string]any{"DirectoryId": dirID, "Name": fmt.Sprintf("s%d", i)},
			)
			require.Equal(t, http.StatusOK, rec.Code)
		}

		// Page 1: limit 2
		rec := doRequest(
			t,
			h,
			"DescribeSnapshots",
			map[string]any{"DirectoryId": dirID, "Limit": 2},
		)
		assert.Equal(t, http.StatusOK, rec.Code)
		body := respBody(t, rec)
		page1, _ := body["Snapshots"].([]any)
		assert.Len(t, page1, 2)
		nextToken, _ := body["NextToken"].(string)
		assert.NotEmpty(t, nextToken)

		// Page 2
		rec2 := doRequest(
			t,
			h,
			"DescribeSnapshots",
			map[string]any{"DirectoryId": dirID, "Limit": 2, "NextToken": nextToken},
		)
		assert.Equal(t, http.StatusOK, rec2.Code)
		body2 := respBody(t, rec2)
		page2, _ := body2["Snapshots"].([]any)
		assert.Len(t, page2, 2)
		_, hasMore := body2["NextToken"]
		assert.False(t, hasMore)

		// 4 distinct snapshots total
		seen := map[string]bool{}
		for _, page := range [][]any{page1, page2} {
			for _, s := range page {
				snap := s.(map[string]any)
				seen[snap["SnapshotId"].(string)] = true
			}
		}
		assert.Len(t, seen, 4)
	})
}

func TestRestoreFromSnapshot_SetsRestoringStage(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	dirID := mustCreateSimpleAD(t, h, "corp.example.com")

	snapRec := doRequest(t, h, "CreateSnapshot", map[string]any{"DirectoryId": dirID})
	require.Equal(t, http.StatusOK, snapRec.Code)
	snapBody := respBody(t, snapRec)
	snapID := snapBody["SnapshotId"].(string)

	doRequest(t, h, "RestoreFromSnapshot", map[string]any{"SnapshotId": snapID})

	rec := doRequest(t, h, "DescribeDirectories", map[string]any{"DirectoryIds": []string{dirID}})
	body := respBody(t, rec)
	dirs := body["DirectoryDescriptions"].([]any)
	d := dirs[0].(map[string]any)
	assert.Equal(t, "Restoring", d["Stage"])
}

// --- CreateAlias state transitions ---

// TestRestoreFromSnapshotTransitionsBackToActive verifies that after RestoreFromSnapshot
// the directory automatically transitions back to Active.
func TestRestoreFromSnapshotTransitionsBackToActive(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	backend := h.Backend.(*directoryservice.InMemoryBackend)
	dirID := mustCreateSimpleAD(t, h, "corp.example.com")
	require.True(t, directoryservice.WaitForDirectoryActive(backend, dirID, 2*time.Second))

	snapRec := doRequest(t, h, "CreateSnapshot", map[string]any{"DirectoryId": dirID})
	require.Equal(t, http.StatusOK, snapRec.Code)
	snapID := respBody(t, snapRec)["SnapshotId"].(string)

	doRequest(t, h, "RestoreFromSnapshot", map[string]any{"SnapshotId": snapID})

	// Immediately after the call the directory must be Restoring.
	assert.Equal(t, "Restoring", directoryservice.DirectoryStageForTest(backend, dirID))

	// Within 2s it must return to Active.
	ok := directoryservice.WaitForDirectoryActive(backend, dirID, 2*time.Second)
	assert.True(t, ok, "directory should return to Active after restore")
}

// TestCreateSnapshot_ADConnectorUnsupported verifies that CreateSnapshot
// rejects AD Connector directories: CreateSnapshot's doc comment states "You
// cannot take snapshots of AD Connector directories" (directoryservice@v1.41.4
// api_op_CreateSnapshot.go).
func TestCreateSnapshot_ADConnectorUnsupported(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	connectRec := doRequest(t, h, "ConnectDirectory", map[string]any{
		"Name":     "corp.example.com",
		"Password": "Admin1234!",
		"Size":     "Small",
		"ConnectSettings": map[string]any{
			"CustomerUserName": "Admin",
			"VpcId":            "vpc-12345678",
			"SubnetIds":        []string{"subnet-11111111", "subnet-22222222"},
		},
	})
	require.Equal(t, http.StatusOK, connectRec.Code)
	dirID := respBody(t, connectRec)["DirectoryId"].(string)

	rec := doRequest(t, h, "CreateSnapshot", map[string]any{"DirectoryId": dirID})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "ClientException", respBody(t, rec)["__type"])
}
