package eks_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/services/eks"
)

func TestEKS_ClusterUpdates(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)

	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "update-cluster"})

	// Update cluster config (logging). Real path is POST
	// /clusters/{name}/update-config, not a bare-path PUT.
	rec := doREST(t, h, http.MethodPost, "/clusters/update-cluster/update-config", map[string]any{
		"logging": map[string]any{
			"clusterLogging": []map[string]any{
				{"types": []string{"api", "audit"}, "enabled": true},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Update nonexistent cluster config
	rec = doREST(t, h, http.MethodPost, "/clusters/nonexistent/update-config", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Update cluster version. Real path is POST /clusters/{name}/updates
	// (shared with ListUpdates' GET), not "/update-version".
	rec = doREST(t, h, http.MethodPost, "/clusters/update-cluster/updates", map[string]any{
		"version": "1.33",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Update cluster version for nonexistent cluster
	rec = doREST(t, h, http.MethodPost, "/clusters/nonexistent/updates", map[string]any{
		"version": "1.33",
	})
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// List updates
	rec = doREST(t, h, http.MethodGet, "/clusters/update-cluster/updates", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// List updates for nonexistent cluster
	rec = doREST(t, h, http.MethodGet, "/clusters/nonexistent/updates", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Describe cluster versions
	rec = doREST(t, h, http.MethodGet, "/cluster-versions", nil)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestEncryptionConfig_ValidKMSArn(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "enc-cluster"})

	rec := doREST(t, h, http.MethodPost, "/clusters/enc-cluster/encryption-config/associate",
		map[string]any{
			"encryptionConfig": []map[string]any{
				{
					"provider":  map[string]string{"keyArn": "arn:aws:kms:us-east-1:123456789012:key/mrk-abc"},
					"resources": []string{"secrets"},
				},
			},
		})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestEncryptionConfig_InvalidKMSArn_Rejected(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "bad-enc"})

	rec := doREST(t, h, http.MethodPost, "/clusters/bad-enc/encryption-config/associate",
		map[string]any{
			"encryptionConfig": []map[string]any{
				{
					"provider":  map[string]string{"keyArn": "not-a-kms-arn"},
					"resources": []string{"secrets"},
				},
			},
		})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEncryptionConfig_Replace_Not_Append(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	first := []eks.EncryptionConfig{
		{Provider: map[string]string{"keyArn": "arn:aws:kms:us-east-1:123:key/k1"}, Resources: []string{"secrets"}},
	}
	result, err := b.AssociateEncryptionConfig("c1", first)
	require.NoError(t, err)
	require.Len(t, result, 1)

	second := []eks.EncryptionConfig{
		{Provider: map[string]string{"keyArn": "arn:aws:kms:us-east-1:123:key/k2"}, Resources: []string{"secrets"}},
	}
	result2, err := b.AssociateEncryptionConfig("c1", second)
	require.NoError(t, err)
	// Must replace, not append — exactly 1 entry.
	require.Len(t, result2, 1, "AssociateEncryptionConfig must replace the stored config, not append")
	assert.Equal(t, "arn:aws:kms:us-east-1:123:key/k2", result2[0].Provider["keyArn"])
}

func TestLogging_EnableAndDisable_PerType(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	// Enable api and audit.
	_, err = b.UpdateClusterConfig("c1", eks.ClusterConfigUpdate{LogEntries: []eks.ClusterLogEntry{
		{Types: []string{"api", "audit"}, Enabled: true},
	}})
	require.NoError(t, err)

	c, err := b.DescribeCluster("c1")
	require.NoError(t, err)
	require.Len(t, c.ClusterLogging, 1)
	assert.ElementsMatch(t, []string{"api", "audit"}, c.ClusterLogging[0].Types)
	assert.True(t, c.ClusterLogging[0].Enabled)

	// Disable api.
	_, err = b.UpdateClusterConfig("c1", eks.ClusterConfigUpdate{LogEntries: []eks.ClusterLogEntry{
		{Types: []string{"api"}, Enabled: false},
	}})
	require.NoError(t, err)

	c2, err := b.DescribeCluster("c1")
	require.NoError(t, err)
	require.Len(t, c2.ClusterLogging, 1)
	assert.Equal(t, []string{"audit"}, c2.ClusterLogging[0].Types)
}

func TestLogging_DisableAll_ClearsLogging(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.UpdateClusterConfig("c1", eks.ClusterConfigUpdate{LogEntries: []eks.ClusterLogEntry{
		{Types: []string{"api"}, Enabled: true},
	}})
	require.NoError(t, err)

	_, err = b.UpdateClusterConfig("c1", eks.ClusterConfigUpdate{LogEntries: []eks.ClusterLogEntry{
		{Types: []string{"api"}, Enabled: false},
	}})
	require.NoError(t, err)

	c, err := b.DescribeCluster("c1")
	require.NoError(t, err)
	assert.Empty(t, c.ClusterLogging, "all log types disabled should produce empty ClusterLogging")
}

func TestLogging_NilEntries_NoChange(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.UpdateClusterConfig("c1", eks.ClusterConfigUpdate{LogEntries: []eks.ClusterLogEntry{
		{Types: []string{"api"}, Enabled: true},
	}})
	require.NoError(t, err)

	// Nil entries must not change existing logging.
	_, err = b.UpdateClusterConfig("c1", eks.ClusterConfigUpdate{})
	require.NoError(t, err)

	c, err := b.DescribeCluster("c1")
	require.NoError(t, err)
	require.Len(t, c.ClusterLogging, 1, "nil log entries must preserve existing logging")
}

func TestLogging_HTTP_StructuredResponse(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})
	doREST(t, h, http.MethodPost, "/clusters/c1/update-config", map[string]any{
		"logging": map[string]any{
			"clusterLogging": []map[string]any{
				{"types": []string{"api", "audit"}, "enabled": true},
			},
		},
	})

	rec := doREST(t, h, http.MethodGet, "/clusters/c1", nil)
	cluster := parseResp(t, rec)["cluster"].(map[string]any)
	logging, ok := cluster["logging"].(map[string]any)
	require.True(t, ok, "logging should appear in describe response after update")
	entries, _ := logging["clusterLogging"].([]any)
	require.NotEmpty(t, entries)
	entry := entries[0].(map[string]any)
	assert.Equal(t, true, entry["enabled"])
}

func TestEncryptionConfig_NoKeyArn_Accepted(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("c1", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	// Provider without keyArn should not trigger KMS validation.
	cfg := []eks.EncryptionConfig{
		{Provider: map[string]string{"keyArn": ""}, Resources: []string{"secrets"}},
	}
	_, err = b.AssociateEncryptionConfig("c1", cfg)
	require.NoError(t, err)
}

func TestLogging_HTTP_Disabled_Types_Preserved(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "c1"})

	doREST(t, h, http.MethodPost, "/clusters/c1/update-config", map[string]any{
		"logging": map[string]any{
			"clusterLogging": []map[string]any{
				{"types": []string{"api", "audit", "authenticator"}, "enabled": true},
			},
		},
	})

	doREST(t, h, http.MethodPost, "/clusters/c1/update-config", map[string]any{
		"logging": map[string]any{
			"clusterLogging": []map[string]any{
				{"types": []string{"audit"}, "enabled": false},
			},
		},
	})

	rec := doREST(t, h, http.MethodGet, "/clusters/c1", nil)
	cluster := parseResp(t, rec)["cluster"].(map[string]any)
	logging := cluster["logging"].(map[string]any)
	entries := logging["clusterLogging"].([]any)
	entry := entries[0].(map[string]any)
	types := entry["types"].([]any)

	typeStrs := make([]string, len(types))
	for i, v := range types {
		typeStrs[i] = v.(string)
	}
	assert.ElementsMatch(t, []string{"api", "authenticator"}, typeStrs)
}

func TestEKS_AssociateEncryptionConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(t *testing.T, h *eks.Handler)
		body       map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "associate_encryption_config_success",
			setup: func(t *testing.T, h *eks.Handler) {
				t.Helper()
				doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "my-cluster"})
			},
			body: map[string]any{
				"encryptionConfig": []map[string]any{
					{
						"provider":  map[string]string{"keyArn": "arn:aws:kms:us-east-1:123:key/abc"},
						"resources": []string{"secrets"},
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "associate_encryption_config_cluster_not_found",
			body:       map[string]any{"encryptionConfig": []map[string]any{}},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestEKSHandler(t)
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doREST(t, h, http.MethodPost, "/clusters/my-cluster/encryption-config/associate", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				resp := parseResp(t, rec)
				update, ok := resp["update"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "AssociateEncryptionConfig", update["type"])
			}
		})
	}
}

func TestEncryptionConfig_Returned_By_DescribeCluster(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "enc-describe")

	// Associate encryption config.
	rec := doREST(t, h, http.MethodPost, "/clusters/enc-describe/encryption-config/associate",
		map[string]any{
			"encryptionConfig": []map[string]any{
				{
					"provider":  map[string]string{"keyArn": "arn:aws:kms:us-east-1:123456789012:key/mrk-abc123"},
					"resources": []string{"secrets"},
				},
			},
		})
	require.Equal(t, http.StatusOK, rec.Code)

	// DescribeCluster must now include encryptionConfig.
	rec2 := doREST(t, h, http.MethodGet, "/clusters/enc-describe", nil)
	require.Equal(t, http.StatusOK, rec2.Code)

	cluster := parseClusterResp(t, rec2)
	encCfg, ok := cluster["encryptionConfig"]
	require.True(t, ok, "DescribeCluster must return encryptionConfig after AssociateEncryptionConfig")
	encList, isList := encCfg.([]any)
	require.True(t, isList, "encryptionConfig must be an array")
	require.Len(t, encList, 1)

	entry := encList[0].(map[string]any)
	provider := entry["provider"].(map[string]any)
	assert.Equal(t, "arn:aws:kms:us-east-1:123456789012:key/mrk-abc123", provider["keyArn"])
}

func TestEncryptionConfig_Absent_Before_Associate(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "enc-absent")

	rec := doREST(t, h, http.MethodGet, "/clusters/enc-absent", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	cluster := parseClusterResp(t, rec)
	// Before AssociateEncryptionConfig, encryptionConfig should be absent or empty.
	if encCfg, ok := cluster["encryptionConfig"]; ok {
		encList, _ := encCfg.([]any)
		assert.Empty(t, encList, "encryptionConfig must be empty before association")
	}
}

func TestEncryptionConfig_Replace_Reflected_In_Describe(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "enc-replace")

	doREST(t, h, http.MethodPost, "/clusters/enc-replace/encryption-config/associate",
		map[string]any{
			"encryptionConfig": []map[string]any{
				{
					"provider":  map[string]string{"keyArn": "arn:aws:kms:us-east-1:123:key/k1"},
					"resources": []string{"secrets"},
				},
			},
		})

	doREST(t, h, http.MethodPost, "/clusters/enc-replace/encryption-config/associate",
		map[string]any{
			"encryptionConfig": []map[string]any{
				{
					"provider":  map[string]string{"keyArn": "arn:aws:kms:us-east-1:123:key/k2"},
					"resources": []string{"secrets"},
				},
			},
		})

	rec := doREST(t, h, http.MethodGet, "/clusters/enc-replace", nil)
	cluster := parseClusterResp(t, rec)
	encList := cluster["encryptionConfig"].([]any)
	require.Len(t, encList, 1, "replace must produce exactly one config")

	entry := encList[0].(map[string]any)
	provider := entry["provider"].(map[string]any)
	assert.Equal(t, "arn:aws:kms:us-east-1:123:key/k2", provider["keyArn"], "second config must overwrite first")
}

func TestUpdate_Params_Array_Present_On_Version_Update(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "upd-params-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/upd-params-cluster/update-config", map[string]any{
		"version": "1.32",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	upd, ok := m["update"].(map[string]any)
	require.True(t, ok, "response must include 'update' object")
	_, hasParams := upd["params"]
	assert.True(t, hasParams, "update must include 'params' field")
	_, hasErrors := upd["errors"]
	assert.True(t, hasErrors, "update must include 'errors' field")
}

func TestUpdate_Params_Array_Present_On_ClusterVersion_Update(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "upd-cv-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/upd-cv-cluster/upgrade-policy", map[string]any{
		"version": "1.33",
	})
	// The endpoint that updates cluster version.
	_ = rec

	// Use backend directly for Update struct params verification.
	upd, err := b.UpdateClusterVersion("upd-cv-cluster", "1.33")
	require.NoError(t, err)
	require.NotEmpty(t, upd.Params, "UpdateClusterVersion must populate Params")
	assert.Equal(t, "Version", upd.Params[0].Type)
	assert.Equal(t, "1.33", upd.Params[0].Value)
}

func TestUpdate_Errors_Empty_Array_On_Success(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "upd-err-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/upd-err-cluster/update-config", map[string]any{
		"logging": map[string]any{
			"clusterLogging": []map[string]any{
				{"types": []string{"api"}, "enabled": true},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	m := parseResp(t, rec)
	upd := m["update"].(map[string]any)
	errorsRaw := upd["errors"]
	errsList, isList := errorsRaw.([]any)
	require.True(t, isList, "errors must be a JSON array")
	assert.Empty(t, errsList, "successful update must return empty errors array")
}

func TestUpdateClusterConfig_VpcEndpoint_Public_To_Private(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateCluster(t, b, "vpc-upd-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/vpc-upd-cluster/update-config", map[string]any{
		"resourcesVpcConfig": map[string]any{
			"endpointPublicAccess":  false,
			"endpointPrivateAccess": true,
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify DescribeCluster reflects the change.
	rec2 := doREST(t, h, http.MethodGet, "/clusters/vpc-upd-cluster", nil)
	cluster := parseClusterResp(t, rec2)
	vpc := cluster["resourcesVpcConfig"].(map[string]any)
	assert.Equal(t, false, vpc["endpointPublicAccess"])
	assert.Equal(t, true, vpc["endpointPrivateAccess"])
}

func TestUpdateClusterConfig_VpcEndpoint_PublicAccessCidrs(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateCluster(t, b, "vpc-cidr-cluster")

	rec := doREST(t, h, http.MethodPost, "/clusters/vpc-cidr-cluster/update-config", map[string]any{
		"resourcesVpcConfig": map[string]any{
			"publicAccessCidrs": []string{"10.0.0.0/8", "192.168.1.0/24"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/vpc-cidr-cluster", nil)
	cluster := parseClusterResp(t, rec2)
	vpc := cluster["resourcesVpcConfig"].(map[string]any)
	cidrs, _ := vpc["publicAccessCidrs"].([]any)
	assert.Len(t, cidrs, 2)
}

func TestUpdateClusterConfig_VpcEndpoint_Params_Populated(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateCluster(t, b, "vpc-params-cluster")

	pub := true
	priv := false
	upd, err := b.UpdateClusterVpcEndpoint("vpc-params-cluster", eks.VpcEndpointUpdate{
		EndpointPublicAccess:  &pub,
		EndpointPrivateAccess: &priv,
	})
	require.NoError(t, err)
	require.Len(t, upd.Params, 2, "VPC endpoint update must populate both params")

	types := make([]string, len(upd.Params))
	for i, p := range upd.Params {
		types[i] = p.Type
	}
	assert.Contains(t, types, "EndpointPublicAccess")
	assert.Contains(t, types, "EndpointPrivateAccess")
}

func TestLogging_AllFiveTypes_Accepted(t *testing.T) {
	t.Parallel()

	logTypes := []string{"api", "audit", "authenticator", "controllerManager", "scheduler"}
	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "logging-5types")

	entries := make([]eks.ClusterLogEntry, 0, len(logTypes))
	for _, lt := range logTypes {
		entries = append(entries, eks.ClusterLogEntry{Types: []string{lt}, Enabled: true})
	}

	_, err := b.UpdateClusterConfig("logging-5types", eks.ClusterConfigUpdate{LogEntries: entries})
	require.NoError(t, err)

	c, err := b.DescribeCluster("logging-5types")
	require.NoError(t, err)

	enabledTypes := make(map[string]bool)
	for _, e := range c.ClusterLogging {
		if e.Enabled {
			for _, t := range e.Types {
				enabledTypes[t] = true
			}
		}
	}

	for _, lt := range logTypes {
		assert.True(t, enabledTypes[lt], "log type %q must be enabled", lt)
	}
}

func TestLogging_Disable_Single_Type(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "logging-disable")

	// Enable all.
	_, err := b.UpdateClusterConfig("logging-disable", eks.ClusterConfigUpdate{LogEntries: []eks.ClusterLogEntry{
		{Types: []string{"api", "audit", "authenticator"}, Enabled: true},
	}})
	require.NoError(t, err)

	// Disable just "audit".
	_, err = b.UpdateClusterConfig("logging-disable", eks.ClusterConfigUpdate{LogEntries: []eks.ClusterLogEntry{
		{Types: []string{"audit"}, Enabled: false},
	}})
	require.NoError(t, err)

	c, err := b.DescribeCluster("logging-disable")
	require.NoError(t, err)

	enabled := make(map[string]bool)
	for _, e := range c.ClusterLogging {
		if e.Enabled {
			for _, t := range e.Types {
				enabled[t] = true
			}
		}
	}
	assert.True(t, enabled["api"])
	assert.True(t, enabled["authenticator"])
	assert.False(t, enabled["audit"], "audit must be disabled")
}

func TestLogging_HTTP_AllTypes_In_Response(t *testing.T) {
	t.Parallel()

	h, b := newHandlerAndBackend(t)
	mustCreateClusterNoVpc(t, b, "logging-http-all")

	rec := doREST(t, h, http.MethodPost, "/clusters/logging-http-all/update-config", map[string]any{
		"logging": map[string]any{
			"clusterLogging": []map[string]any{
				{"types": []string{"api", "audit", "authenticator", "controllerManager", "scheduler"}, "enabled": true},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doREST(t, h, http.MethodGet, "/clusters/logging-http-all", nil)
	cluster := parseClusterResp(t, rec2)
	raw, _ := json.Marshal(cluster)
	body := string(raw)

	for _, lt := range []string{"api", "audit", "authenticator", "controllerManager", "scheduler"} {
		assert.Contains(t, body, lt, "log type %q must appear in cluster response", lt)
	}
}

func TestKMS_Resources_Field_RoundTrip(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "kms-resources")

	cfg := []eks.EncryptionConfig{
		{
			Provider:  map[string]string{"keyArn": "arn:aws:kms:us-east-1:123456789012:key/mrk-xyz"},
			Resources: []string{"secrets"},
		},
	}
	result, err := b.AssociateEncryptionConfig("kms-resources", cfg)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, []string{"secrets"}, result[0].Resources)
}

func TestKMS_MultiResource_Accepted(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "kms-multi")

	cfg := []eks.EncryptionConfig{
		{
			Provider:  map[string]string{"keyArn": "arn:aws:kms:us-east-1:123:key/k1"},
			Resources: []string{"secrets", "configmaps"},
		},
	}
	result, err := b.AssociateEncryptionConfig("kms-multi", cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"secrets", "configmaps"}, result[0].Resources)
}

func TestUpdateClusterVpcEndpoint_NoVpc_Creates_Default(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	// Cluster with no VPC config.
	mustCreateClusterNoVpc(t, b, "novpc-endpoint-cluster")

	pub := true
	upd, err := b.UpdateClusterVpcEndpoint("novpc-endpoint-cluster", eks.VpcEndpointUpdate{
		EndpointPublicAccess: &pub,
	})
	require.NoError(t, err)
	assert.Equal(t, "Successful", upd.Status)

	c, err := b.DescribeCluster("novpc-endpoint-cluster")
	require.NoError(t, err)
	require.NotNil(t, c.VpcConfig, "VpcConfig must be created if it was nil")
	assert.True(t, c.VpcConfig.EndpointPublicAccess)
}

func TestUpdateClusterVpcEndpoint_NonExistent_Cluster(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	priv := true
	_, err := b.UpdateClusterVpcEndpoint("nonexistent", eks.VpcEndpointUpdate{
		EndpointPrivateAccess: &priv,
	})
	assert.Error(t, err)
}

func TestUpdateClusterConfig_Status_InProgress(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "upd-inprog-cluster")

	upd, err := b.UpdateClusterConfig("upd-inprog-cluster", eks.ClusterConfigUpdate{LogEntries: []eks.ClusterLogEntry{
		{Types: []string{"api"}, Enabled: true},
	}})
	require.NoError(t, err)
	assert.Equal(t, "InProgress", upd.Status)

	// The immediate status is right, but a machine that never leaves InProgress
	// would pass the assertion above forever -- confirm it actually reaches
	// Successful too.
	require.Eventually(t, func() bool {
		got, descErr := b.DescribeUpdate("upd-inprog-cluster", upd.ID)

		return descErr == nil && got.Status == "Successful"
	}, 2*time.Second, 10*time.Millisecond)
}

func TestDescribeUpdate_Status_Successful(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "desc-upd-cluster")

	created, err := b.UpdateClusterVersion("desc-upd-cluster", "1.30")
	require.NoError(t, err)

	upd, err := b.DescribeUpdate("desc-upd-cluster", created.ID)
	require.NoError(t, err)
	assert.Equal(t, "InProgress", upd.Status)

	require.Eventually(t, func() bool {
		got, descErr := b.DescribeUpdate("desc-upd-cluster", created.ID)

		return descErr == nil && got.Status == "Successful"
	}, 2*time.Second, 10*time.Millisecond)
}

func TestDescribeUpdate_NotFound(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "desc-upd-404")

	_, err := b.DescribeUpdate("desc-upd-404", "nonexistent-update-id")
	require.Error(t, err)
	require.ErrorIs(t, err, eks.ErrNotFound)
}

func TestListUpdates_ReturnsStoredIDs(t *testing.T) {
	t.Parallel()

	b := newBackend(t)
	mustCreateClusterNoVpc(t, b, "list-upd-cluster")
	mustCreateNodegroup(t, b, "list-upd-cluster")

	ids, err := b.ListUpdates("list-upd-cluster")
	require.NoError(t, err)
	assert.Empty(t, ids, "no updates yet")

	u1, err := b.UpdateClusterVersion("list-upd-cluster", "1.30")
	require.NoError(t, err)

	u2, err := b.UpdateNodegroupVersion("list-upd-cluster", "ng1", "1.30")
	require.NoError(t, err)

	ids, err = b.ListUpdates("list-upd-cluster")
	require.NoError(t, err)
	assert.Len(t, ids, 2)
	assert.Contains(t, ids, u1.ID)
	assert.Contains(t, ids, u2.ID)
}

func TestUpdateClusterConfig_AccessConfig(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("upd-ac", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.UpdateClusterConfig("upd-ac", eks.ClusterConfigUpdate{
		AccessConfig: &eks.AccessConfig{AuthenticationMode: "API_AND_CONFIG_MAP"},
	})
	require.NoError(t, err)

	c, err := b.DescribeCluster("upd-ac")
	require.NoError(t, err)
	require.NotNil(t, c.AccessConfig)
	assert.Equal(t, "API_AND_CONFIG_MAP", c.AccessConfig.AuthenticationMode)
}

func TestUpdateClusterConfig_SubnetIDs(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("upd-subnets", "1.32", "", &eks.VpcConfig{
		SubnetIDs: []string{"subnet-old"},
	}, nil, nil)
	require.NoError(t, err)

	_, err = b.UpdateClusterConfig("upd-subnets", eks.ClusterConfigUpdate{
		SubnetIDs: []string{"subnet-new-1", "subnet-new-2"},
	})
	require.NoError(t, err)

	c, err := b.DescribeCluster("upd-subnets")
	require.NoError(t, err)
	require.NotNil(t, c.VpcConfig)
	assert.Equal(t, []string{"subnet-new-1", "subnet-new-2"}, c.VpcConfig.SubnetIDs)
}

func TestUpdateClusterConfig_SubnetIDs_Via_Handler(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "upd-sub-cluster",
		"resourcesVpcConfig": map[string]any{
			"subnetIds": []string{"subnet-orig"},
		},
	})

	rec := doREST(t, h, http.MethodPost, "/clusters/upd-sub-cluster/update-config", map[string]any{
		"resourcesVpcConfig": map[string]any{
			"subnetIds": []string{"subnet-a", "subnet-b"},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	desc := doREST(t, h, http.MethodGet, "/clusters/upd-sub-cluster", nil)
	cluster := parseResp(t, desc)["cluster"].(map[string]any)
	vpc := cluster["resourcesVpcConfig"].(map[string]any)
	subs, _ := vpc["subnetIds"].([]any)
	require.Len(t, subs, 2)
	assert.Equal(t, "subnet-a", subs[0])
}

func TestUpdateClusterConfig_ComputeConfig(t *testing.T) {
	t.Parallel()

	b := eks.NewInMemoryBackend(t.Context(), "123456789012", config.DefaultRegion)
	_, err := b.CreateCluster("upd-compute", "1.32", "", nil, nil, nil)
	require.NoError(t, err)

	_, err = b.UpdateClusterConfig("upd-compute", eks.ClusterConfigUpdate{
		ComputeConfig: &eks.ComputeConfig{Enabled: true, NodeRoleARN: "arn:aws:iam::123:role/node"},
	})
	require.NoError(t, err)

	c, err := b.DescribeCluster("upd-compute")
	require.NoError(t, err)
	require.NotNil(t, c.ComputeConfig)
	assert.True(t, c.ComputeConfig.Enabled)
	assert.Equal(t, "arn:aws:iam::123:role/node", c.ComputeConfig.NodeRoleARN)
}

func TestUpdateClusterConfig_Handler_AccessConfig(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "upd-ac-handler"})

	rec := doREST(t, h, http.MethodPost, "/clusters/upd-ac-handler/update-config", map[string]any{
		"accessConfig": map[string]any{
			"authenticationMode": "API",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	desc := doREST(t, h, http.MethodGet, "/clusters/upd-ac-handler", nil)
	cluster := parseResp(t, desc)["cluster"].(map[string]any)
	ac, ok := cluster["accessConfig"].(map[string]any)
	require.True(t, ok, "accessConfig must be present after update")
	assert.Equal(t, "API", ac["authenticationMode"])
}

// TestUpdateClusterConfig_AccessConfig_DoesNotClobberBootstrapPermissions
// guards against a field-clobbering bug: types.UpdateAccessConfigRequest
// (eks@v1.90.4 types/types.go:2727) has only AuthenticationMode --
// BootstrapClusterCreatorAdminPermissions cannot be sent on this op by any
// real client. UpdateClusterConfig must leave a cluster's existing bootstrap
// flag untouched when only authenticationMode is being updated.
func TestUpdateClusterConfig_AccessConfig_DoesNotClobberBootstrapPermissions(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{
		"name": "upd-ac-no-clobber",
		"accessConfig": map[string]any{
			"authenticationMode":                      "CONFIG_MAP",
			"bootstrapClusterCreatorAdminPermissions": true,
		},
	})

	rec := doREST(t, h, http.MethodPost, "/clusters/upd-ac-no-clobber/update-config", map[string]any{
		"accessConfig": map[string]any{
			"authenticationMode": "API",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	desc := doREST(t, h, http.MethodGet, "/clusters/upd-ac-no-clobber", nil)
	cluster := parseResp(t, desc)["cluster"].(map[string]any)
	ac, ok := cluster["accessConfig"].(map[string]any)
	require.True(t, ok, "accessConfig must be present after update")
	assert.Equal(t, "API", ac["authenticationMode"])
	assert.Equal(t, true, ac["bootstrapClusterCreatorAdminPermissions"],
		"UpdateClusterConfig must not clobber this field -- it is not part of this op's wire shape")
}

func TestUpdateClusterConfig_Handler_ComputeConfig(t *testing.T) {
	t.Parallel()

	h := newTestEKSHandler(t)
	doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "upd-cc-handler"})

	rec := doREST(t, h, http.MethodPost, "/clusters/upd-cc-handler/update-config", map[string]any{
		"computeConfig": map[string]any{
			"enabled":     true,
			"nodeRoleArn": "arn:aws:iam::123:role/node",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	desc := doREST(t, h, http.MethodGet, "/clusters/upd-cc-handler", nil)
	cluster := parseResp(t, desc)["cluster"].(map[string]any)
	cc, ok := cluster["computeConfig"].(map[string]any)
	require.True(t, ok, "computeConfig must be present after update")
	assert.Equal(t, true, cc["enabled"])
}

func TestUpdateClusterVersionReturnsInProgress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "version_update_is_inprogress"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := newBackend(t)
			_, err := b.CreateCluster(
				"cl", "1.31", "arn:aws:iam::123456789012:role/role", nil, nil, nil,
			)
			require.NoError(t, err)

			u, err := b.UpdateClusterVersion("cl", "1.32")
			require.NoError(t, err)
			assert.Equal(t, "InProgress", u.Status, tc.name)
		})
	}
}

// TestEKS_CancelUpdate covers CancelUpdate: success on an InProgress
// VersionRollback update, and the InvalidRequestException/ResourceNotFound
// rejection paths. Real EKS only supports cancellation for VersionRollback
// updates (Kubernetes version rollback on EKS Auto Mode clusters) that are
// still InProgress -- verified against aws-sdk-go-v2/service/eks's
// CancelUpdate doc comment. Since no public op creates a VersionRollback
// update, tests seed one directly via the exported StoreUpdate helper.
func TestEKS_CancelUpdate(t *testing.T) {
	t.Parallel()

	t.Run("cancels_inprogress_version_rollback", func(t *testing.T) {
		t.Parallel()

		h := newTestEKSHandler(t)
		doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "cancel-cluster"})

		h.Backend.StoreUpdate(&eks.Update{
			ID:          "rollback-1",
			ClusterName: "cancel-cluster",
			Status:      "InProgress",
			Type:        "VersionRollback",
		})

		rec := doREST(t, h, http.MethodPost, "/clusters/cancel-cluster/updates/rollback-1/cancel-update", nil)
		require.Equal(t, http.StatusOK, rec.Code)

		resp := parseResp(t, rec)
		upd, ok := resp["update"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Cancelled", upd["status"])

		cancellation, ok := upd["cancellation"].(map[string]any)
		require.True(t, ok, "cancellation field must be present after a successful cancel")
		assert.Equal(t, "Successful", cancellation["status"])
	})

	t.Run("rejects_non_rollback_update_type", func(t *testing.T) {
		t.Parallel()

		h := newTestEKSHandler(t)
		doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "cancel-cluster2"})

		u, err := h.Backend.UpdateClusterVersion("cancel-cluster2", "1.33")
		require.NoError(t, err)

		rec := doREST(t, h, http.MethodPost, "/clusters/cancel-cluster2/updates/"+u.ID+"/cancel-update", nil)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("rejects_already_completed_update", func(t *testing.T) {
		t.Parallel()

		h := newTestEKSHandler(t)
		doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "cancel-cluster3"})

		h.Backend.StoreUpdate(&eks.Update{
			ID:          "rollback-done",
			ClusterName: "cancel-cluster3",
			Status:      "Successful",
			Type:        "VersionRollback",
		})

		rec := doREST(t, h, http.MethodPost, "/clusters/cancel-cluster3/updates/rollback-done/cancel-update", nil)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("update_not_found", func(t *testing.T) {
		t.Parallel()

		h := newTestEKSHandler(t)
		doREST(t, h, http.MethodPost, "/clusters", map[string]any{"name": "cancel-cluster4"})

		rec := doREST(t, h, http.MethodPost, "/clusters/cancel-cluster4/updates/nonexistent/cancel-update", nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("cluster_not_found", func(t *testing.T) {
		t.Parallel()

		h := newTestEKSHandler(t)

		rec := doREST(t, h, http.MethodPost, "/clusters/nonexistent/updates/upd1/cancel-update", nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("backend_direct_call", func(t *testing.T) {
		t.Parallel()

		b := newBackend(t)
		mustCreateClusterNoVpc(t, b, "cancel-direct")

		b.StoreUpdate(&eks.Update{
			ID: "u1", ClusterName: "cancel-direct", Status: "InProgress", Type: "VersionRollback",
		})

		u, err := b.CancelUpdate("cancel-direct", "u1")
		require.NoError(t, err)
		assert.Equal(t, "Cancelled", u.Status)
		require.NotNil(t, u.Cancellation)
		assert.Equal(t, "Successful", u.Cancellation.Status)

		// Cancelling again fails: no longer InProgress.
		_, err = b.CancelUpdate("cancel-direct", "u1")
		assert.ErrorIs(t, err, eks.ErrInvalidRequest)
	})
}
