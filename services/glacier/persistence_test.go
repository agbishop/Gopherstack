package glacier_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/glacier"
)

func TestGlacier_PersistenceSnapshotRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup  func(b *glacier.InMemoryBackend)
		verify func(t *testing.T, b *glacier.InMemoryBackend)
		name   string
	}{
		{
			name:  "empty_backend",
			setup: func(_ *glacier.InMemoryBackend) {},
			verify: func(t *testing.T, b *glacier.InMemoryBackend) {
				t.Helper()

				vaults := b.ListVaults("123", "us-east-1")
				assert.Empty(t, vaults)
			},
		},
		{
			name: "vault_with_archive_preserved",
			setup: func(b *glacier.InMemoryBackend) {
				_, err := b.CreateVault("123", "us-east-1", "my-vault")
				if err != nil {
					return
				}

				_, _ = b.UploadArchive("123", "us-east-1", "my-vault", "desc", "hash", 1024, []byte("data"))
			},
			verify: func(t *testing.T, b *glacier.InMemoryBackend) {
				t.Helper()

				vaults := b.ListVaults("123", "us-east-1")
				require.Len(t, vaults, 1)
				assert.Equal(t, "my-vault", vaults[0].VaultName)
				assert.Equal(t, int64(1), vaults[0].NumberOfArchives)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := glacier.NewInMemoryBackend()
			tt.setup(b)

			snap := b.Snapshot(t.Context())
			require.NotNil(t, snap)

			b2 := glacier.NewInMemoryBackend()
			err := b2.Restore(t.Context(), snap)
			require.NoError(t, err)

			tt.verify(t, b2)
		})
	}
}

// TestGlacier_PersistenceFullStateRoundTrip exercises every piece of backend
// state -- vaults (with inline archives), jobs, multipart uploads and their
// parts, vault locks, provisioned capacity, the account data-retrieval
// policy, tags, notifications, and the access policy -- through a single
// Snapshot/Restore round trip. This is the Phase 3.3 pkgs/store conversion's
// full-state coverage: every store.Table (vaults, jobs, multipartUploads,
// vaultLocks) and every raw map left behind (multipartParts,
// provisionedCapacity, dataRetrievalPolicies) is represented.
func TestGlacier_PersistenceFullStateRoundTrip(t *testing.T) {
	t.Parallel()

	const (
		accountID = "111122223333"
		region    = "us-west-2"
		vaultName = "full-state-vault"
	)

	b := glacier.NewInMemoryBackend()
	glacier.SetRetrievalDelay(b, 0)

	_, err := b.CreateVault(accountID, region, vaultName)
	require.NoError(t, err)

	archive, err := b.UploadArchive(accountID, region, vaultName, "desc", "archive-hash", 2048, []byte("archive-bytes"))
	require.NoError(t, err)

	job, err := b.InitiateJob(accountID, region, vaultName, &glacier.ExportedInitiateJobRequest{
		Type:      "archive-retrieval",
		ArchiveID: archive.ArchiveID,
	})
	require.NoError(t, err)

	upload, err := b.InitiateMultipartUpload(accountID, region, vaultName, "mpu-desc", 1<<20)
	require.NoError(t, err)

	partData := make([]byte, 1<<20)
	partHash := glacier.ComputeTreeHash(partData)
	require.NoError(t,
		b.UploadMultipartPart(
			accountID, region, vaultName, upload.MultipartUploadID, "bytes 0-1048575/*", partHash, partData,
		),
	)

	require.NoError(t, b.SetVaultLock(accountID, region, vaultName, `{"Version":"2012-10-17"}`, "lock-id-1"))

	require.NoError(t, b.AddTagsToVault(accountID, region, vaultName, map[string]string{"env": "prod"}))
	require.NoError(t, b.SetVaultNotifications(accountID, region, vaultName, "arn:aws:sns:us-west-2:111122223333:topic",
		[]string{"ArchiveRetrievalCompleted"}))
	require.NoError(t, b.SetVaultAccessPolicy(accountID, region, vaultName, `{"Version":"2012-10-17"}`))

	b.SetDataRetrievalPolicy(accountID, []byte(`{"Rules":[]}`))

	_, err = b.PurchaseProvisionedCapacity(accountID)
	require.NoError(t, err)

	snap := b.Snapshot(t.Context())
	require.NotNil(t, snap)

	b2 := glacier.NewInMemoryBackend()
	require.NoError(t, b2.Restore(t.Context(), snap))

	// Vault + inline archive.
	v, err := b2.DescribeVault(accountID, region, vaultName)
	require.NoError(t, err)
	assert.Equal(t, int64(1), v.NumberOfArchives)
	assert.Equal(t, int64(2048), v.SizeInBytes)

	archives, err := b2.ListArchives(accountID, region, vaultName)
	require.NoError(t, err)
	require.Len(t, archives, 1)
	assert.Equal(t, archive.ArchiveID, archives[0].ArchiveID)

	// Job (store.Table, byVault index).
	gotJob, err := b2.DescribeJob(accountID, region, vaultName, job.JobID)
	require.NoError(t, err)
	assert.Equal(t, job.JobID, gotJob.JobID)

	jobs, err := b2.ListJobs(accountID, region, vaultName)
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	// Multipart upload + parts (upload is a store.Table; parts stay a raw map).
	uploads := b2.ListMultipartUploads(accountID, region, vaultName)
	require.Len(t, uploads, 1)
	assert.Equal(t, upload.MultipartUploadID, uploads[0].MultipartUploadID)

	parts, err := b2.ListParts(accountID, region, vaultName, upload.MultipartUploadID)
	require.NoError(t, err)
	require.Len(t, parts.Parts, 1)
	assert.Equal(t, partHash, parts.Parts[0].SHA256TreeHash)

	// Vault lock (store.Table).
	lock, err := b2.GetVaultLock(accountID, region, vaultName)
	require.NoError(t, err)
	assert.Equal(t, "lock-id-1", lock.LockID)
	assert.Equal(t, "InProgress", lock.State)

	// Tags, notifications, access policy (fields on the restored Vault).
	tags, err := b2.ListTagsForVault(accountID, region, vaultName)
	require.NoError(t, err)
	assert.Equal(t, "prod", tags["env"])

	snsTopic, events, err := b2.GetVaultNotifications(accountID, region, vaultName)
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:sns:us-west-2:111122223333:topic", snsTopic)
	assert.Equal(t, []string{"ArchiveRetrievalCompleted"}, events)

	policy, err := b2.GetVaultAccessPolicy(accountID, region, vaultName)
	require.NoError(t, err)
	assert.JSONEq(t, `{"Version":"2012-10-17"}`, policy)

	// Raw maps: data retrieval policy and provisioned capacity.
	assert.JSONEq(t, `{"Rules":[]}`, b2.GetDataRetrievalPolicy(accountID))
	assert.Len(t, b2.ListProvisionedCapacity(accountID), 1)

	// GetArchiveData is a pre-existing gap this conversion preserves
	// byte-for-byte: raw archive bytes were never part of any persisted map
	// before this conversion (only Archive metadata was), so they do not
	// survive a Snapshot/Restore round trip either.
	_, ok := b2.GetArchiveData(archive.ArchiveID)
	assert.False(t, ok)
}

// TestGlacier_PersistenceVersionGuard verifies that Restore discards an
// incompatible (missing/old) snapshot version by resetting to empty rather
// than erroring, and returns an error for genuinely malformed JSON.
func TestGlacier_PersistenceVersionGuard(t *testing.T) {
	t.Parallel()

	t.Run("incompatible_version_resets_to_empty", func(t *testing.T) {
		t.Parallel()

		b := glacier.NewInMemoryBackend()
		_, err := b.CreateVault("123", "us-east-1", "pre-existing")
		require.NoError(t, err)

		// A version-0 (pre-conversion-shaped) snapshot must be treated as
		// incompatible, not partially decoded.
		err = b.Restore(t.Context(), []byte(`{"version":0,"tables":{}}`))
		require.NoError(t, err)

		assert.Empty(t, b.ListVaults("123", "us-east-1"))
	})

	t.Run("malformed_json_errors", func(t *testing.T) {
		t.Parallel()

		b := glacier.NewInMemoryBackend()
		err := b.Restore(t.Context(), []byte(`{not valid json`))
		require.Error(t, err)
	})
}

// TestVaultLocks_Persistence verifies snapshot/restore preserves vault locks.
func TestVaultLocks_Persistence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		vaultName string
		lockID    string
	}{
		{name: "vault_lock_survives_roundtrip", vaultName: "locked-vault", lockID: "my-lock-id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := glacier.NewInMemoryBackend()
			_, err := b.CreateVault(testAccountID, testRegion, tt.vaultName)
			require.NoError(t, err)

			err = b.SetVaultLock(testAccountID, testRegion, tt.vaultName, "{}", tt.lockID)
			require.NoError(t, err)

			snap := b.Snapshot(t.Context())
			require.NotNil(t, snap)

			b2 := glacier.NewInMemoryBackend()
			err = b2.Restore(t.Context(), snap)
			require.NoError(t, err)

			lock, err := b2.GetVaultLock(testAccountID, testRegion, tt.vaultName)
			require.NoError(t, err)
			assert.Equal(t, tt.lockID, lock.LockID)
			assert.Equal(t, "InProgress", lock.State)
		})
	}
}

// TestPersistenceRoundTrip verifies full snapshot/restore preserves state.
func TestPersistenceRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		vaultName    string
		archiveDesc  string
		wantVaults   int
		wantArchives int
	}{
		{name: "full_state_roundtrip", vaultName: "myv", archiveDesc: "my-archive", wantVaults: 1, wantArchives: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := glacier.NewInMemoryBackend()
			_, err := b.CreateVault(testAccountID, testRegion, tt.vaultName)
			require.NoError(t, err)

			_, err = b.UploadArchive(
				testAccountID,
				testRegion,
				tt.vaultName,
				tt.archiveDesc,
				"chk",
				512,
				[]byte("data"),
			)
			require.NoError(t, err)

			snap := b.Snapshot(t.Context())
			require.NotNil(t, snap)

			b2 := glacier.NewInMemoryBackend()
			err = b2.Restore(t.Context(), snap)
			require.NoError(t, err)

			assert.Equal(t, tt.wantVaults, glacier.VaultCount(b2))
			assert.Equal(t, tt.wantArchives, glacier.ArchiveCount(b2))
		})
	}
}

// TestPersistenceEmpty verifies an empty snapshot round-trips cleanly.
func TestPersistenceEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "empty_snapshot_restores_cleanly"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := glacier.NewInMemoryBackend()
			snap := b.Snapshot(t.Context())
			require.NotNil(t, snap, tt.name)

			b2 := glacier.NewInMemoryBackend()
			err := b2.Restore(t.Context(), snap)
			require.NoError(t, err)

			assert.Equal(t, 0, glacier.VaultCount(b2))
		})
	}
}

func TestPersistenceRoundTrip_DataRetrievalPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy string
	}{
		{
			name:   "drp_survives_snapshot_restore",
			policy: `{"Policy":{"Rules":[{"Strategy":"BytesPerHour","BytesPerHour":1048576}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			rec := doRequest(
				t,
				h,
				http.MethodPut,
				"/"+testAccountID+"/policies/data-retrieval",
				tt.policy,
			)
			require.Equal(t, http.StatusNoContent, rec.Code)

			snap := h.Snapshot(t.Context())
			require.NotNil(t, snap)

			h2 := newTestHandler()
			require.NoError(t, h2.Restore(t.Context(), snap))

			rec = doRequest(t, h2, http.MethodGet, "/"+testAccountID+"/policies/data-retrieval", "")
			require.Equal(t, http.StatusOK, rec.Code)

			var got map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			assert.NotEmpty(t, got)
		})
	}
}

func TestPersistenceRoundTrip_MultipartAndCapacity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "snapshot_restore_preserves_multipart_and_capacity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			// Create vault and initiate multipart upload
			rec := doRequest(t, h, http.MethodPut, "/-/vaults/persist-vault", "")
			require.Equal(t, http.StatusCreated, rec.Code)

			rec = doRequestWithHeaders(
				t,
				h,
				http.MethodPost,
				"/-/vaults/persist-vault/multipart-uploads",
				"",
				map[string]string{"X-Amz-Part-Size": "1048576"},
			)
			require.Equal(t, http.StatusCreated, rec.Code)

			// Purchase provisioned capacity
			rec = doRequest(t, h, http.MethodPost, "/-/provisioned-capacity", "")
			require.Equal(t, http.StatusCreated, rec.Code)

			// Snapshot and restore
			snap := h.Snapshot(t.Context())
			require.NotEmpty(t, snap)

			h2 := newTestHandler()
			require.NoError(t, h2.Restore(t.Context(), snap))

			// Verify multipart uploads are restored
			rec = doRequest(t, h2, http.MethodGet, "/-/vaults/persist-vault/multipart-uploads", "")
			require.Equal(t, http.StatusOK, rec.Code)

			var uploadsResp struct {
				UploadsList []any `json:"UploadsList"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &uploadsResp))
			assert.Len(t, uploadsResp.UploadsList, 1)

			// Verify provisioned capacity is restored
			rec = doRequest(t, h2, http.MethodGet, "/-/provisioned-capacity", "")
			require.Equal(t, http.StatusOK, rec.Code)

			var capsResp struct {
				ProvisionedCapacityList []any `json:"ProvisionedCapacityList"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &capsResp))
			assert.Len(t, capsResp.ProvisionedCapacityList, 1)
		})
	}
}

// TestPersistenceRoundTrip_SelectAndRangeInventoryJobs verifies that a Select job's
// SelectParameters/OutputLocation/JobOutputPath and an InventoryRetrieval job's range
// InventoryRetrievalParameters survive a Snapshot/Restore round trip -- both are new
// Job fields (see models.go) that must be JSON round-trippable through
// store.Registry.SnapshotAll/RestoreAll exactly like every pre-existing Job field.
func TestPersistenceRoundTrip_SelectAndRangeInventoryJobs(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	createVault(t, h, "persist-select-vault")
	archiveID := uploadArchiveData(t, h, "persist-select-vault", []byte(selectTestArchive))

	selectJobID := initiateJobWithBody(t, h, "persist-select-vault",
		basicSelectBody(archiveID, "SELECT * FROM archive WHERE _3 > 28"))

	invJobID := initiateJobWithBody(t, h, "persist-select-vault",
		`{"Type":"inventory-retrieval","Format":"CSV","InventoryRetrievalParameters":`+
			`{"StartDate":"2020-01-01T00:00:00Z","EndDate":"2020-02-01T00:00:00Z","Limit":"50","Marker":"a1"}}`)

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	bk2 := glacier.NewInMemoryBackend()
	h2 := glacier.NewHandler(bk2)
	h2.AccountID = testAccountID
	h2.DefaultRegion = testRegion
	require.NoError(t, h2.Restore(t.Context(), snap))

	// Select job: SelectParameters/OutputLocation/JobOutputPath restored.
	rec := doRequest(t, h2, http.MethodGet,
		"/"+testAccountID+"/vaults/persist-select-vault/jobs/"+selectJobID, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var selectResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &selectResp))
	assert.Equal(t, "Select", selectResp["Action"])
	assert.NotEmpty(t, selectResp["JobOutputPath"])

	sp, ok := selectResp["SelectParameters"].(map[string]any)
	require.True(t, ok, "SelectParameters must survive restore: %#v", selectResp)
	assert.Equal(t, "SELECT * FROM archive WHERE _3 > 28", sp["Expression"])

	ol, ok := selectResp["OutputLocation"].(map[string]any)
	require.True(t, ok, "OutputLocation must survive restore: %#v", selectResp)
	s3loc, ok := ol["S3"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "results-bucket", s3loc["BucketName"])

	// NOTE: GetJobOutput on the restored select job is NOT re-verified here: raw
	// archive bytes (b.archiveData) are a pre-existing, documented persistence gap
	// (see TestGlacier_PersistenceFullStateRoundTrip) -- they never survive a
	// Snapshot/Restore round trip, only Archive metadata does, so executing the
	// query post-restore would always 404 regardless of whether SelectParameters
	// itself round-tripped correctly (which is what this test actually verifies).

	// InventoryRetrieval job: range InventoryRetrievalParameters restored.
	rec = doRequest(t, h2, http.MethodGet,
		"/"+testAccountID+"/vaults/persist-select-vault/jobs/"+invJobID, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var invResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &invResp))

	invParams, ok := invResp["InventoryRetrievalParameters"].(map[string]any)
	require.True(t, ok, "InventoryRetrievalParameters must survive restore: %#v", invResp)
	assert.Equal(t, "2020-01-01T00:00:00Z", invParams["StartDate"])
	assert.Equal(t, "2020-02-01T00:00:00Z", invParams["EndDate"])
	assert.Equal(t, "CSV", invParams["Format"])
	assert.Equal(t, "50", invParams["Limit"])
	assert.Equal(t, "a1", invParams["Marker"])
}

// ----------------------------------------
// GetSupportedOperations includes new ops
// ----------------------------------------
