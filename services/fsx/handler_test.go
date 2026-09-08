package fsx_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/fsx"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func newTestHandler(t *testing.T) *fsx.Handler {
	t.Helper()
	backend := fsx.NewInMemoryBackend("000000000000", "us-east-1")

	return fsx.NewHandler(backend)
}

func doFSxRequest(t *testing.T, h *fsx.Handler, op string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte

	if body != nil {
		var marshalErr error

		bodyBytes, marshalErr = json.Marshal(body)
		require.NoError(t, marshalErr)
	}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSSimbaAPIService_v20180301."+op)
	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)
	handlerErr := h.Handler()(c)
	require.NoError(t, handlerErr)

	return rec
}

// fileSystemCreateBody returns a CreateFileSystem request body for fsType.
// Real AWS FSx requires a type-specific configuration block (with its own
// required members, e.g. ThroughputCapacity) for WINDOWS/ONTAP/OPENZFS; this
// mirrors the minimal valid request a real client would send so createFS
// stays usable as a shared fixture across every _test.go file in this
// package.
func fileSystemCreateBody(fsType string) map[string]any {
	body := map[string]any{"FileSystemType": fsType}

	switch fsType {
	case "WINDOWS":
		body["WindowsConfiguration"] = map[string]any{"ThroughputCapacity": 8}
	case "ONTAP":
		body["OntapConfiguration"] = map[string]any{"DeploymentType": "SINGLE_AZ_1", "ThroughputCapacity": 128}
	case "OPENZFS":
		body["OpenZFSConfiguration"] = map[string]any{"DeploymentType": "SINGLE_AZ_1", "ThroughputCapacity": 64}
	}

	return body
}

func createFS(t *testing.T, h *fsx.Handler, fsType string) string {
	t.Helper()
	rec := doFSxRequest(t, h, "CreateFileSystem", fileSystemCreateBody(fsType))
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	return out["FileSystem"].(map[string]any)["FileSystemId"].(string)
}

func createFSandBackup(t *testing.T, h *fsx.Handler, fsType string) string {
	t.Helper()
	fsID := createFS(t, h, fsType)
	rec := doFSxRequest(t, h, "CreateBackup", map[string]any{"FileSystemId": fsID})
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	return out["Backup"].(map[string]any)["BackupId"].(string)
}

// createVolume creates a volume of volType. Real CreateVolumeInput has no
// top-level FileSystemId or StorageVirtualMachineId at all (fsx@v1.68.4
// api_op_CreateVolume.go) -- the parent file system is only reachable via
// OntapConfiguration.StorageVirtualMachineId (ONTAP) or
// OpenZFSConfiguration.ParentVolumeId (OPENZFS), so this helper resolves (or
// creates) one of those anchors under fsID before calling CreateVolume, the
// same way fileSystemCreateBody mirrors CreateFileSystem's own per-type
// required config block. An empty fsID creates a fresh file system of volType.
func createVolume(t *testing.T, h *fsx.Handler, fsID, volType, name string) string { //nolint:unparam // existing issue.
	t.Helper()

	if fsID == "" {
		fsID = createFS(t, h, volType)
	}

	body := map[string]any{
		"VolumeType": volType,
		"Name":       name,
	}

	switch volType {
	case "ONTAP":
		svmID := createSVM(t, h, fsID, name+"-svm")
		body["OntapConfiguration"] = map[string]any{"StorageVirtualMachineId": svmID}
	case "OPENZFS":
		body["OpenZFSConfiguration"] = map[string]any{"ParentVolumeId": openZFSRootVolumeID(t, h, fsID)}
	}

	rec := doFSxRequest(t, h, "CreateVolume", body)
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	return out["Volume"].(map[string]any)["VolumeId"].(string)
}

// openZFSRootVolumeID returns the RootVolumeId AWS auto-creates for an
// OPENZFS file system, the anchor a real client must supply as
// OpenZFSConfiguration.ParentVolumeId when creating a child volume.
func openZFSRootVolumeID(t *testing.T, h *fsx.Handler, fsID string) string {
	t.Helper()
	rec := doFSxRequest(t, h, "DescribeFileSystems", map[string]any{"FileSystemIds": []string{fsID}})
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	fileSystems, ok := out["FileSystems"].([]any)
	require.True(t, ok)
	require.Len(t, fileSystems, 1)
	fs, ok := fileSystems[0].(map[string]any)
	require.True(t, ok)
	openZFS, ok := fs["OpenZFSConfiguration"].(map[string]any)
	require.True(t, ok, "OpenZFSConfiguration must be present on an OPENZFS file system, got %v", fs)

	return openZFS["RootVolumeId"].(string)
}

func createSVM(t *testing.T, h *fsx.Handler, fsID, name string) string {
	t.Helper()
	rec := doFSxRequest(t, h, "CreateStorageVirtualMachine", map[string]any{
		"FileSystemId": fsID,
		"Name":         name,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	return out["StorageVirtualMachine"].(map[string]any)["StorageVirtualMachineId"].(string)
}

func createFileCache(t *testing.T, h *fsx.Handler, cacheType string) string {
	t.Helper()
	rec := doFSxRequest(t, h, "CreateFileCache", map[string]any{
		"FileCacheType":        cacheType,
		"FileCacheTypeVersion": "2.12",
		"SubnetIds":            []string{"subnet-1"},
		"StorageCapacity":      1200,
		"LustreConfiguration":  fileCacheLustreConfigBody(),
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	return out["FileCache"].(map[string]any)["FileCacheId"].(string)
}

// fileCacheLustreConfigBody builds a minimal valid CreateFileCache
// LustreConfiguration block (fsx@v1.68.4 types/types.go:574 --
// DeploymentType/MetadataConfiguration/PerUnitStorageThroughput required).
func fileCacheLustreConfigBody() map[string]any {
	return map[string]any{
		"DeploymentType":           "CACHE_1",
		"MetadataConfiguration":    map[string]any{"StorageCapacity": 2400},
		"PerUnitStorageThroughput": 1000,
	}
}

// decodeField requires a 200 response and returns the named top-level field
// decoded as a JSON object.
func decodeField(t *testing.T, rec *httptest.ResponseRecorder, field string) map[string]any {
	t.Helper()

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	obj, ok := out[field].(map[string]any)
	require.True(t, ok, "response must contain object field %q, got %v", field, out)

	return obj
}

// ---------------------------------------------------------------------------
// Core dispatch tests
// ---------------------------------------------------------------------------

func TestFSx_UnknownOperation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doFSxRequest(t, h, "CreateVolume", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// Test_CreationTime_IsEpochSecondsNumber verifies that every FSx resource's
// CreationTime field serializes as a JSON number (epoch seconds), matching
// the real aws-sdk-go-v2/service/fsx deserializer, which hard-fails
// ("expected CreationTime to be a JSON Number, got string instead") on an
// RFC3339 string. Previously DataRepositoryAssociation, DataRepositoryTask,
// FileCache, Snapshot, StorageVirtualMachine, Volume, and S3AccessPoint used
// a bare time.Time field, which Go's encoding/json renders as an RFC3339
// string.
func Test_CreationTime_IsEpochSecondsNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		create func(t *testing.T, h *fsx.Handler) map[string]any
		name   string
		field  string
	}{
		{
			name:  "FileSystem",
			field: "FileSystem",
			create: func(t *testing.T, h *fsx.Handler) map[string]any {
				t.Helper()

				return decodeField(t, doFSxRequest(t, h, "CreateFileSystem",
					map[string]any{"FileSystemType": "LUSTRE"}), "FileSystem")
			},
		},
		{
			name:  "Backup",
			field: "Backup",
			create: func(t *testing.T, h *fsx.Handler) map[string]any {
				t.Helper()
				fsID := createFS(t, h, "LUSTRE")

				return decodeField(t, doFSxRequest(t, h, "CreateBackup",
					map[string]any{"FileSystemId": fsID}), "Backup")
			},
		},
		{
			name:  "DataRepositoryAssociation",
			field: "Association",
			create: func(t *testing.T, h *fsx.Handler) map[string]any {
				t.Helper()
				fsID := createFS(t, h, "LUSTRE")

				return decodeField(t, doFSxRequest(t, h, "CreateDataRepositoryAssociation", map[string]any{
					"FileSystemId":       fsID,
					"FileSystemPath":     "/data",
					"DataRepositoryPath": "s3://bucket/prefix",
				}), "Association")
			},
		},
		{
			name:  "DataRepositoryTask",
			field: "DataRepositoryTask",
			create: func(t *testing.T, h *fsx.Handler) map[string]any {
				t.Helper()
				fsID := createFS(t, h, "LUSTRE")

				return decodeField(t, doFSxRequest(t, h, "CreateDataRepositoryTask", map[string]any{
					"FileSystemId": fsID,
					"Type":         "EXPORT_TO_REPOSITORY",
					"Report":       map[string]any{"Enabled": false},
				}), "DataRepositoryTask")
			},
		},
		{
			name:  "FileCache",
			field: "FileCache",
			create: func(t *testing.T, h *fsx.Handler) map[string]any {
				t.Helper()

				return decodeField(t, doFSxRequest(t, h, "CreateFileCache",
					map[string]any{
						"FileCacheType":        "LUSTRE",
						"FileCacheTypeVersion": "2.12",
						"SubnetIds":            []string{"subnet-1"},
						"StorageCapacity":      1200,
						"LustreConfiguration":  fileCacheLustreConfigBody(),
					}), "FileCache")
			},
		},
		{
			name:  "Snapshot",
			field: "Snapshot",
			create: func(t *testing.T, h *fsx.Handler) map[string]any {
				t.Helper()
				volID := createVolume(t, h, "", "ONTAP", "vol1")

				return decodeField(t, doFSxRequest(t, h, "CreateSnapshot", map[string]any{
					"VolumeId": volID,
					"Name":     "snap1",
				}), "Snapshot")
			},
		},
		{
			name:  "StorageVirtualMachine",
			field: "StorageVirtualMachine",
			create: func(t *testing.T, h *fsx.Handler) map[string]any {
				t.Helper()
				fsID := createFS(t, h, "ONTAP")

				return decodeField(t, doFSxRequest(t, h, "CreateStorageVirtualMachine", map[string]any{
					"FileSystemId": fsID,
					"Name":         "svm1",
				}), "StorageVirtualMachine")
			},
		},
		{
			name:  "Volume",
			field: "Volume",
			create: func(t *testing.T, h *fsx.Handler) map[string]any {
				t.Helper()
				fsID := createFS(t, h, "ONTAP")
				svmID := createSVM(t, h, fsID, "svm2")

				return decodeField(t, doFSxRequest(t, h, "CreateVolume", map[string]any{
					"VolumeType":         "ONTAP",
					"Name":               "vol2",
					"OntapConfiguration": map[string]any{"StorageVirtualMachineId": svmID},
				}), "Volume")
			},
		},
		{
			name:  "S3AccessPointAttachment",
			field: "S3AccessPointAttachment",
			create: func(t *testing.T, h *fsx.Handler) map[string]any {
				t.Helper()
				volID := createVolume(t, h, "", "ONTAP", "ap-vol")

				return decodeField(t, doFSxRequest(t, h, "CreateAndAttachS3AccessPoint", map[string]any{
					"Name": "ap1",
					"Type": "ONTAP",
					"OntapConfiguration": map[string]any{
						"VolumeId": volID,
					},
				}), "S3AccessPointAttachment")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			resource := tt.create(t, h)

			raw, ok := resource["CreationTime"]
			require.True(t, ok, "%s response must include CreationTime", tt.field)

			switch v := raw.(type) {
			case float64:
				assert.Positive(t, v, "CreationTime must be a positive epoch-seconds number")
			default:
				t.Fatalf("%s.CreationTime must decode as a JSON number (epoch seconds), got %T: %v",
					tt.field, raw, raw)
			}
		})
	}
}
