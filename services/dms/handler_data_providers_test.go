package dms_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/dms"
)

func TestDeleteDataProvider(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	h.Backend.AddDataProviderInternal("del-dp", "mysql")

	rec := doDMS(t, h, "DeleteDataProvider", map[string]any{
		"DataProviderIdentifier": "del-dp",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, h.Backend.DataProviderCount())
}

// TestDeleteDataProvider_RejectedWithMigrationProject locks real AWS's
// DeleteDataProvider doc comment: "All migration projects associated with
// the data provider must be deleted or modified before you can delete the
// data provider".
func TestDeleteDataProvider_RejectedWithMigrationProject(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	deps := migrationProjectDeps(t, h)

	createBody := map[string]any{"MigrationProjectName": "dp-guard-proj"}
	maps.Copy(createBody, deps)

	createRec := doDMS(t, h, "CreateMigrationProject", createBody)
	require.Equal(t, http.StatusOK, createRec.Code)

	srcDescriptor := deps["SourceDataProviderDescriptors"].([]map[string]any)[0]
	srcProviderID := srcDescriptor["DataProviderIdentifier"].(string)

	rec := doDMS(t, h, "DeleteDataProvider", map[string]any{
		"DataProviderIdentifier": srcProviderID,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 2, h.Backend.DataProviderCount())

	delProjRec := doDMS(t, h, "DeleteMigrationProject", map[string]any{
		"MigrationProjectIdentifier": "dp-guard-proj",
	})
	require.Equal(t, http.StatusOK, delProjRec.Code)

	rec2 := doDMS(t, h, "DeleteDataProvider", map[string]any{
		"DataProviderIdentifier": srcProviderID,
	})
	assert.Equal(t, http.StatusOK, rec2.Code)
}

func TestModifyDataProvider(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	h.Backend.AddDataProviderInternal("mod-dp", "mysql")

	rec := doDMS(t, h, "ModifyDataProvider", map[string]any{
		"DataProviderIdentifier": "mod-dp",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec2 := doDMS(t, h, "ModifyDataProvider", map[string]any{
		"DataProviderIdentifier": "nonexistent",
	})
	assert.Equal(t, http.StatusNotFound, rec2.Code)
}

// TestModifyDataProvider_RejectedWithMigrationProject locks real AWS's
// ModifyDataProvider doc comment: "You must remove the data provider from
// all migration projects before you can modify it" (databasemigrationservice
// @v1.66.4 api_op_ModifyDataProvider.go:16-17).
func TestModifyDataProvider_RejectedWithMigrationProject(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	deps := migrationProjectDeps(t, h)

	createBody := map[string]any{"MigrationProjectName": "dp-modify-guard-proj"}
	maps.Copy(createBody, deps)

	createRec := doDMS(t, h, "CreateMigrationProject", createBody)
	require.Equal(t, http.StatusOK, createRec.Code)

	srcDescriptor := deps["SourceDataProviderDescriptors"].([]map[string]any)[0]
	srcProviderID := srcDescriptor["DataProviderIdentifier"].(string)

	rec := doDMS(t, h, "ModifyDataProvider", map[string]any{
		"DataProviderIdentifier": srcProviderID,
		"Description":            "should be rejected",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	delProjRec := doDMS(t, h, "DeleteMigrationProject", map[string]any{
		"MigrationProjectIdentifier": "dp-modify-guard-proj",
	})
	require.Equal(t, http.StatusOK, delProjRec.Code)

	rec2 := doDMS(t, h, "ModifyDataProvider", map[string]any{
		"DataProviderIdentifier": srcProviderID,
		"Description":            "should now succeed",
	})
	assert.Equal(t, http.StatusOK, rec2.Code)
}

func TestDeleteDataProvider_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	rec := doDMS(t, h, "DeleteDataProvider", map[string]any{
		"DataProviderIdentifier": "arn:aws:dms:us-east-1:123:data-provider:nonexistent",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ResourceNotFoundFault", body["__type"])
}

func TestHandler_CreateDataProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *dms.Handler)
		name string
	}{
		{
			name: "create_success",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "CreateDataProvider", map[string]any{
					"DataProviderName": "my-provider",
					"Engine":           "mysql",
					"Description":      "My MySQL provider",
					"Tags": []map[string]string{
						{"Key": "Team", "Value": "infra"},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec)
				dp, ok := resp["DataProvider"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "my-provider", dp["DataProviderName"])
				assert.Equal(t, "mysql", dp["Engine"])
				assert.Equal(t, "My MySQL provider", dp["Description"])
				assert.NotEmpty(t, dp["DataProviderArn"])
			},
		},
		{
			name: "create_duplicate_conflict",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				doDMS(t, h, "CreateDataProvider", map[string]any{
					"DataProviderName": "dup-provider",
					"Engine":           "postgres",
				})
				rec := doDMS(t, h, "CreateDataProvider", map[string]any{
					"DataProviderName": "dup-provider",
					"Engine":           "postgres",
				})
				assert.Equal(t, http.StatusConflict, rec.Code)
			},
		},
		{
			name: "missing_name",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "CreateDataProvider", map[string]any{
					"Engine": "mysql",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			name: "missing_engine",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "CreateDataProvider", map[string]any{
					"DataProviderName": "no-engine",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestDMSHandler()
			tt.run(t, h)
		})
	}
}
