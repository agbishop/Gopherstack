package dms_test

import (
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/dms"
)

func TestDeleteInstanceProfile(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	h.Backend.AddInstanceProfileInternal("del-ip")

	rec := doDMS(t, h, "DeleteInstanceProfile", map[string]any{
		"InstanceProfileIdentifier": "del-ip",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, h.Backend.InstanceProfileCount())

	rec2 := doDMS(t, h, "DeleteInstanceProfile", map[string]any{
		"InstanceProfileIdentifier": "del-ip",
	})
	assert.Equal(t, http.StatusNotFound, rec2.Code)
}

// TestDeleteInstanceProfile_RejectedWithMigrationProject locks real AWS's
// DeleteInstanceProfile doc comment: "All migration projects associated
// with the instance profile must be deleted or modified before you can
// delete the instance profile".
func TestDeleteInstanceProfile_RejectedWithMigrationProject(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	deps := migrationProjectDeps(t, h)

	createBody := map[string]any{"MigrationProjectName": "ip-guard-proj"}
	maps.Copy(createBody, deps)

	createRec := doDMS(t, h, "CreateMigrationProject", createBody)
	require.Equal(t, http.StatusOK, createRec.Code)

	rec := doDMS(t, h, "DeleteInstanceProfile", map[string]any{
		"InstanceProfileIdentifier": deps["InstanceProfileIdentifier"],
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, h.Backend.InstanceProfileCount())

	delProjRec := doDMS(t, h, "DeleteMigrationProject", map[string]any{
		"MigrationProjectIdentifier": "ip-guard-proj",
	})
	require.Equal(t, http.StatusOK, delProjRec.Code)

	rec2 := doDMS(t, h, "DeleteInstanceProfile", map[string]any{
		"InstanceProfileIdentifier": deps["InstanceProfileIdentifier"],
	})
	assert.Equal(t, http.StatusOK, rec2.Code)
}

func TestModifyInstanceProfile(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	h.Backend.AddInstanceProfileInternal("mod-ip")

	descRec := doDMS(t, h, "DescribeInstanceProfiles", map[string]any{})
	require.Equal(t, http.StatusOK, descRec.Code)
	profiles := parseJSON(t, descRec)["InstanceProfiles"].([]any)
	require.Len(t, profiles, 1)
	ipArn := profiles[0].(map[string]any)["InstanceProfileArn"].(string)

	rec := doDMS(t, h, "ModifyInstanceProfile", map[string]any{
		"InstanceProfileIdentifier": ipArn,
		"Description":               "updated description",
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec2 := doDMS(t, h, "ModifyInstanceProfile", map[string]any{
		"InstanceProfileIdentifier": "nonexistent",
	})
	assert.Equal(t, http.StatusNotFound, rec2.Code)
}

// TestModifyInstanceProfile_RejectedWithMigrationProject locks real AWS's
// ModifyInstanceProfile doc comment: "All migration projects associated
// with the instance profile must be deleted or modified before you can
// modify the instance profile" (databasemigrationservice@v1.66.4
// api_op_ModifyInstanceProfile.go:16-17).
func TestModifyInstanceProfile_RejectedWithMigrationProject(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	deps := migrationProjectDeps(t, h)

	createBody := map[string]any{"MigrationProjectName": "ip-modify-guard-proj"}
	maps.Copy(createBody, deps)

	createRec := doDMS(t, h, "CreateMigrationProject", createBody)
	require.Equal(t, http.StatusOK, createRec.Code)

	rec := doDMS(t, h, "ModifyInstanceProfile", map[string]any{
		"InstanceProfileIdentifier": deps["InstanceProfileIdentifier"],
		"Description":               "should be rejected",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	delProjRec := doDMS(t, h, "DeleteMigrationProject", map[string]any{
		"MigrationProjectIdentifier": "ip-modify-guard-proj",
	})
	require.Equal(t, http.StatusOK, delProjRec.Code)

	rec2 := doDMS(t, h, "ModifyInstanceProfile", map[string]any{
		"InstanceProfileIdentifier": deps["InstanceProfileIdentifier"],
		"Description":               "should now succeed",
	})
	assert.Equal(t, http.StatusOK, rec2.Code)
}

func TestHandler_CreateInstanceProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		run  func(t *testing.T, h *dms.Handler)
		name string
	}{
		{
			name: "create_success_named",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "CreateInstanceProfile", map[string]any{
					"InstanceProfileName":   "my-profile",
					"AvailabilityZone":      "us-east-1a",
					"NetworkType":           "IPV4",
					"PubliclyAccessible":    false,
					"Description":           "Test profile",
					"SubnetGroupIdentifier": "subnet-group-1",
					"Tags": []map[string]string{
						{"Key": "Env", "Value": "staging"},
					},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec)
				ip, ok := resp["InstanceProfile"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "my-profile", ip["InstanceProfileName"])
				assert.Equal(t, "us-east-1a", ip["AvailabilityZone"])
				assert.NotEmpty(t, ip["InstanceProfileArn"])
			},
		},
		{
			name: "create_success_no_name",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				rec := doDMS(t, h, "CreateInstanceProfile", map[string]any{
					"AvailabilityZone": "us-west-2a",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec)
				ip, ok := resp["InstanceProfile"].(map[string]any)
				require.True(t, ok)
				assert.NotEmpty(t, ip["InstanceProfileArn"])
			},
		},
		{
			name: "create_duplicate_conflict",
			run: func(t *testing.T, h *dms.Handler) {
				t.Helper()
				doDMS(t, h, "CreateInstanceProfile", map[string]any{
					"InstanceProfileName": "dup-profile",
				})
				rec := doDMS(t, h, "CreateInstanceProfile", map[string]any{
					"InstanceProfileName": "dup-profile",
				})
				assert.Equal(t, http.StatusConflict, rec.Code)
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

func TestNetworkTypeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		networkType string
		wantStatus  int
	}{
		{name: "empty_network_type", networkType: "", wantStatus: http.StatusOK},
		{name: "ipv4", networkType: "IPV4", wantStatus: http.StatusOK},
		{name: "ipv6", networkType: "IPV6", wantStatus: http.StatusOK},
		{name: "dual", networkType: "DUAL", wantStatus: http.StatusOK},
		{name: "invalid", networkType: "INVALID", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestDMSHandler()
			body := map[string]any{
				"InstanceProfileName": "prof-" + tt.networkType,
			}
			if tt.networkType != "" {
				body["NetworkType"] = tt.networkType
			}
			rec := doDMS(t, h, "CreateInstanceProfile", body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestInstanceProfileSeedHelper(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()
	h.Backend.AddInstanceProfileInternal("seed-profile")
	assert.Equal(t, 1, h.Backend.InstanceProfileCount())
}
