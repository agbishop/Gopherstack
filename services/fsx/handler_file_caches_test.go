package fsx_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/fsx"
)

// TestFSx_CreateFileCache_RequiresLustreConfiguration verifies the real
// MissingFileCacheConfiguration exception (fsx@v1.68.4 types/errors.go:
// "A cache configuration is required for this operation."): CreateFileCache
// requires a LustreConfiguration block with DeploymentType,
// MetadataConfiguration.StorageCapacity, and PerUnitStorageThroughput --
// FileCacheType is always LUSTRE, so this is CreateFileCache's only
// per-type configuration block. An absent block returns
// MissingFileCacheConfiguration; a present-but-incomplete block returns
// BadRequest, matching CreateFileSystem's established per-type-config-block
// pattern (applyWindowsConfig et al., file_systems.go).
func TestFSx_CreateFileCache_RequiresLustreConfiguration(t *testing.T) {
	t.Parallel()

	baseBody := func() map[string]any {
		return map[string]any{
			"FileCacheType":        "LUSTRE",
			"FileCacheTypeVersion": "2.12",
			"SubnetIds":            []string{"subnet-1"},
			"StorageCapacity":      1200,
		}
	}

	tests := []struct {
		lustreConfig map[string]any
		name         string
		wantType     string
	}{
		{
			name:         "missing LustreConfiguration entirely",
			lustreConfig: nil,
			wantType:     "MissingFileCacheConfiguration",
		},
		{
			name: "missing DeploymentType",
			lustreConfig: map[string]any{
				"MetadataConfiguration":    map[string]any{"StorageCapacity": 2400},
				"PerUnitStorageThroughput": 1000,
			},
			wantType: "BadRequest",
		},
		{
			name: "missing MetadataConfiguration",
			lustreConfig: map[string]any{
				"DeploymentType":           "CACHE_1",
				"PerUnitStorageThroughput": 1000,
			},
			wantType: "BadRequest",
		},
		{
			name: "missing PerUnitStorageThroughput",
			lustreConfig: map[string]any{
				"DeploymentType":        "CACHE_1",
				"MetadataConfiguration": map[string]any{"StorageCapacity": 2400},
			},
			wantType: "BadRequest",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			body := baseBody()
			if tc.lustreConfig != nil {
				body["LustreConfiguration"] = tc.lustreConfig
			}

			rec := doFSxRequest(t, h, "CreateFileCache", body)
			require.Equal(t, http.StatusBadRequest, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, tc.wantType, out["__type"])
		})
	}

	t.Run("valid LustreConfiguration round-trips", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)

		body := baseBody()
		body["LustreConfiguration"] = map[string]any{
			"DeploymentType":             "CACHE_1",
			"MetadataConfiguration":      map[string]any{"StorageCapacity": 2400},
			"PerUnitStorageThroughput":   1000,
			"WeeklyMaintenanceStartTime": "1:05:00",
		}

		rec := doFSxRequest(t, h, "CreateFileCache", body)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		var out map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		fc := out["FileCache"].(map[string]any)
		lc, ok := fc["LustreConfiguration"].(map[string]any)
		require.True(t, ok, "response must have a LustreConfiguration block")
		assert.Equal(t, "CACHE_1", lc["DeploymentType"])
		assert.InDelta(t, float64(1000), lc["PerUnitStorageThroughput"], 0.0001)
		assert.Equal(t, "1:05:00", lc["WeeklyMaintenanceStartTime"])
		mc, ok := lc["MetadataConfiguration"].(map[string]any)
		require.True(t, ok, "response must have a MetadataConfiguration block")
		assert.InDelta(t, float64(2400), mc["StorageCapacity"], 0.0001)
	})
}

func TestFSx_FileCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cacheType   string
		typeVersion string
		subnetIDs   []string
		capacity    int
		wantCode    int
		wantErr     bool
	}{
		{
			name:        "create LUSTRE cache",
			cacheType:   "LUSTRE",
			typeVersion: "2.12",
			subnetIDs:   []string{"subnet-1"},
			capacity:    1200,
			wantCode:    http.StatusOK,
		},
		{
			name:     "missing cache type returns 400",
			wantCode: http.StatusBadRequest,
			wantErr:  true,
		},
		{
			name:      "missing FileCacheTypeVersion returns 400",
			cacheType: "LUSTRE",
			subnetIDs: []string{"subnet-1"},
			capacity:  1200,
			wantCode:  http.StatusBadRequest,
			wantErr:   true,
		},
		{
			name:        "missing SubnetIds returns 400",
			cacheType:   "LUSTRE",
			typeVersion: "2.12",
			capacity:    1200,
			wantCode:    http.StatusBadRequest,
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)

			body := map[string]any{"StorageCapacity": tc.capacity}
			if tc.cacheType != "" {
				body["FileCacheType"] = tc.cacheType
			}
			if tc.typeVersion != "" {
				body["FileCacheTypeVersion"] = tc.typeVersion
			}
			if tc.subnetIDs != nil {
				body["SubnetIds"] = tc.subnetIDs
			}
			if tc.wantCode == http.StatusOK {
				body["LustreConfiguration"] = map[string]any{
					"DeploymentType":           "CACHE_1",
					"MetadataConfiguration":    map[string]any{"StorageCapacity": 2400},
					"PerUnitStorageThroughput": 1000,
				}
			}

			rec := doFSxRequest(t, h, "CreateFileCache", body)
			require.Equal(t, tc.wantCode, rec.Code)

			if !tc.wantErr {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				c := out["FileCache"].(map[string]any)
				assert.Contains(t, c["FileCacheId"].(string), "fc-")
				assert.Equal(t, "AVAILABLE", c["Lifecycle"])
				assert.InDelta(t, float64(tc.capacity), c["StorageCapacity"], 0.0001)
				assert.Equal(t, tc.typeVersion, c["FileCacheTypeVersion"],
					"FileCacheTypeVersion must be echoed back on CreateFileCache's response")
				assert.ElementsMatch(t, tc.subnetIDs, c["SubnetIds"])
			}
		})
	}
}

func TestFSx_FileCacheLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("describe/update/delete cycle", func(t *testing.T) {
		t.Parallel()
		h := newTestHandler(t)
		b := fsx.GetBackend(h)
		fcID := createFileCache(t, h, "LUSTRE")

		assert.Equal(t, 1, fsx.FileCacheCount(b))

		// describe by id
		rec := doFSxRequest(t, h, "DescribeFileCaches", map[string]any{
			"FileCacheIds": []string{fcID},
		})
		require.Equal(t, http.StatusOK, rec.Code)
		var dr map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dr))
		assert.Len(t, dr["FileCaches"].([]any), 1)

		// update
		rec2 := doFSxRequest(t, h, "UpdateFileCache", map[string]any{
			"FileCacheId":        fcID,
			"StorageCapacityGiB": 2400,
		})
		require.Equal(t, http.StatusOK, rec2.Code)
		var ur map[string]any
		require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &ur))
		assert.InDelta(t, float64(2400), ur["FileCache"].(map[string]any)["StorageCapacity"], 0.0001)

		// delete
		rec3 := doFSxRequest(t, h, "DeleteFileCache", map[string]any{"FileCacheId": fcID})
		require.Equal(t, http.StatusOK, rec3.Code)
		var del map[string]any
		require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &del))
		assert.Equal(t, fcID, del["FileCacheId"])
		assert.Equal(t, "DELETING", del["Lifecycle"])
		assert.Equal(t, 0, fsx.FileCacheCount(b))
	})
}
