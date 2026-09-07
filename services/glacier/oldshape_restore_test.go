package glacier_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stripOldShapeVaultFields simulates a snapshot taken before gopherstack-x8em/
// zpo5 added NumberOfArchivesAtLastInventory, SizeInBytesAtLastInventory and
// WriteSinceLastInventory: it deletes those three keys from every vault in
// the snapshot's "vaults" table, leaving everything else (including
// glacierSnapshotVersion) untouched.
func stripOldShapeVaultFields(t *testing.T, snap []byte) []byte {
	t.Helper()

	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(snap, &top))

	var tables map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["tables"], &tables))

	var vaults []map[string]any
	require.NoError(t, json.Unmarshal(tables["vaults"], &vaults))

	for _, v := range vaults {
		delete(v, "numberOfArchivesAtLastInventory")
		delete(v, "sizeInBytesAtLastInventory")
		delete(v, "writeSinceLastInventory")
	}

	vaultsRaw, err := json.Marshal(vaults)
	require.NoError(t, err)
	tables["vaults"] = vaultsRaw

	tablesRaw, err := json.Marshal(tables)
	require.NoError(t, err)
	top["tables"] = tablesRaw

	out, err := json.Marshal(top)
	require.NoError(t, err)

	return out
}

// TestRestoreOldShapeSnapshot_DeleteVaultGuard reproduces gopherstack-c8sa: a
// vault snapshotted before x8em/zpo5 added the three as-of-inventory fields
// restores with them zeroed, and DeleteVault's guard (v.NumberOfArchivesAt
// LastInventory > 0 || v.WriteSinceLastInventory) then treats a genuinely
// non-empty vault as deletable.
func TestRestoreOldShapeSnapshot_DeleteVaultGuard(t *testing.T) {
	t.Parallel()

	t.Run("non_empty_vault_old_shape_delete_refused", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler()
		createVault(t, h, "old-shape-vault")
		uploadArchiveData(t, h, "old-shape-vault", []byte("archive-bytes"))
		initiateJobWithBody(t, h, "old-shape-vault", `{"Type":"inventory-retrieval"}`)

		snap := h.Snapshot(t.Context())
		require.NotNil(t, snap)

		oldShape := stripOldShapeVaultFields(t, snap)

		h2 := newTestHandler()
		require.NoError(t, h2.Restore(t.Context(), oldShape))

		rec := doRequest(t, h2, http.MethodGet, "/"+testAccountID+"/vaults/old-shape-vault", "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		t.Logf("DescribeVault on restored old-shape vault: %s", rec.Body.String())

		var describeResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &describeResp))
		assert.NotEmpty(t, describeResp["LastInventoryDate"], "LastInventoryDate must survive restore")
		_, hasNumArchives := describeResp["NumberOfArchives"]
		assert.False(t, hasNumArchives,
			"NumberOfArchives must be omitted (unknown), not fabricated as zero, for a restored old-shape vault")
		_, hasSizeInBytes := describeResp["SizeInBytes"]
		assert.False(t, hasSizeInBytes,
			"SizeInBytes must be omitted (unknown), not fabricated as zero, for a restored old-shape vault")

		rec = doRequest(t, h2, http.MethodDelete, "/"+testAccountID+"/vaults/old-shape-vault", "")
		t.Logf("DeleteVault on restored old-shape non-empty vault: status=%d body=%s", rec.Code, rec.Body.String())
		assert.Equal(t, http.StatusConflict, rec.Code, "restored old-shape non-empty vault must not be deletable")

		rec = doRequest(t, h2, http.MethodGet, "/"+testAccountID+"/vaults/old-shape-vault", "")
		assert.Equal(t, http.StatusOK, rec.Code, "vault must still exist after refused delete")
	})

	t.Run("empty_vault_old_shape_delete_succeeds", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler()
		createVault(t, h, "old-shape-empty-vault")

		snap := h.Snapshot(t.Context())
		require.NotNil(t, snap)

		oldShape := stripOldShapeVaultFields(t, snap)

		h2 := newTestHandler()
		require.NoError(t, h2.Restore(t.Context(), oldShape))

		rec := doRequest(t, h2, http.MethodDelete, "/"+testAccountID+"/vaults/old-shape-empty-vault", "")
		assert.Equal(t, http.StatusNoContent, rec.Code,
			"restored old-shape genuinely empty vault must remain deletable")
	})

	t.Run("new_shape_vault_roundtrips_unchanged", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler()
		createVault(t, h, "new-shape-vault")
		uploadArchiveData(t, h, "new-shape-vault", []byte("archive-bytes"))
		initiateJobWithBody(t, h, "new-shape-vault", `{"Type":"inventory-retrieval"}`)

		snap := h.Snapshot(t.Context())
		require.NotNil(t, snap)

		h2 := newTestHandler()
		require.NoError(t, h2.Restore(t.Context(), snap))

		rec := doRequest(t, h2, http.MethodDelete, "/"+testAccountID+"/vaults/new-shape-vault", "")
		assert.Equal(t, http.StatusConflict, rec.Code, "new-shape restored non-empty vault must stay refused")
	})
}
