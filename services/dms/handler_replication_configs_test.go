package dms_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplicationConfigLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()

	createRec := doDMS(t, h, "CreateReplicationConfig", map[string]any{
		"ReplicationConfigIdentifier": "rc-1",
		"ReplicationType":             "full-load",
		"SourceEndpointArn":           "arn:src",
		"TargetEndpointArn":           "arn:tgt",
		"TableMappings":               "{}",
		"ComputeConfig":               map[string]any{},
	})
	require.Equal(t, http.StatusOK, createRec.Code)
	rcArn := parseJSON(t, createRec)["ReplicationConfig"].(map[string]any)["ReplicationConfigArn"].(string)

	// Duplicate.
	dupRec := doDMS(t, h, "CreateReplicationConfig", map[string]any{
		"ReplicationConfigIdentifier": "rc-1",
		"ReplicationType":             "cdc",
		"SourceEndpointArn":           "arn:src",
		"TargetEndpointArn":           "arn:tgt",
		"TableMappings":               "{}",
		"ComputeConfig":               map[string]any{},
	})
	assert.Equal(t, http.StatusConflict, dupRec.Code)

	// Describe.
	descRec := doDMS(t, h, "DescribeReplicationConfigs", map[string]any{})
	assert.Equal(t, http.StatusOK, descRec.Code)

	// Modify by ARN.
	modRec := doDMS(t, h, "ModifyReplicationConfig", map[string]any{
		"ReplicationConfigArn": rcArn,
		"ReplicationType":      "cdc",
	})
	assert.Equal(t, http.StatusOK, modRec.Code)

	// Modify not found.
	notFoundRec := doDMS(t, h, "ModifyReplicationConfig", map[string]any{
		"ReplicationConfigArn": "nonexistent",
	})
	assert.Equal(t, http.StatusNotFound, notFoundRec.Code)

	// Delete by ARN.
	delRec := doDMS(t, h, "DeleteReplicationConfig", map[string]any{
		"ReplicationConfigArn": rcArn,
	})
	assert.Equal(t, http.StatusOK, delRec.Code)

	delRec2 := doDMS(t, h, "DeleteReplicationConfig", map[string]any{
		"ReplicationConfigArn": rcArn,
	})
	assert.Equal(t, http.StatusNotFound, delRec2.Code)
}

// TestDeleteReplicationConfig_RejectedWhileRunning locks real AWS's
// DeleteReplicationConfig doc comment: "You can't delete the configuration
// for an DMS Serverless replication that is ongoing. You can delete the
// configuration when the replication is in a non-RUNNING and non-STARTING
// state".
func TestDeleteReplicationConfig_RejectedWhileRunning(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()

	createRec := doDMS(t, h, "CreateReplicationConfig", map[string]any{
		"ReplicationConfigIdentifier": "rc-running",
		"ReplicationType":             "full-load",
		"SourceEndpointArn":           "arn:src",
		"TargetEndpointArn":           "arn:tgt",
		"TableMappings":               "{}",
		"ComputeConfig":               map[string]any{},
	})
	require.Equal(t, http.StatusOK, createRec.Code)
	rcArn := parseJSON(t, createRec)["ReplicationConfig"].(map[string]any)["ReplicationConfigArn"].(string)

	startRec := doDMS(t, h, "StartReplication", map[string]any{
		"ReplicationConfigArn": rcArn,
		"StartReplicationType": "start-replication",
	})
	require.Equal(t, http.StatusOK, startRec.Code)

	delRec := doDMS(t, h, "DeleteReplicationConfig", map[string]any{
		"ReplicationConfigArn": rcArn,
	})
	assert.Equal(t, http.StatusBadRequest, delRec.Code)

	stopRec := doDMS(t, h, "StopReplication", map[string]any{
		"ReplicationConfigArn": rcArn,
	})
	require.Equal(t, http.StatusOK, stopRec.Code)

	delRec2 := doDMS(t, h, "DeleteReplicationConfig", map[string]any{
		"ReplicationConfigArn": rcArn,
	})
	assert.Equal(t, http.StatusOK, delRec2.Code)
}

func TestModifyReplicationConfig_UpdatesReplicationType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		lookupByArn bool
	}{
		{name: "lookup_by_identifier", lookupByArn: false},
		{name: "lookup_by_arn", lookupByArn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestDMSHandler()

			createRec := doDMS(t, h, "CreateReplicationConfig", map[string]any{
				"ReplicationConfigIdentifier": "rc-modify",
				"ReplicationType":             "full-load",
				"SourceEndpointArn":           "arn:aws:dms:us-east-1:123:endpoint:src",
				"TargetEndpointArn":           "arn:aws:dms:us-east-1:123:endpoint:tgt",
				"TableMappings":               "{}",
				"ComputeConfig":               map[string]any{},
			})
			require.Equal(t, http.StatusOK, createRec.Code)
			rc := parseJSON(t, createRec)["ReplicationConfig"].(map[string]any)
			rcIdentifier := rc["ReplicationConfigIdentifier"].(string)
			rcArn := rc["ReplicationConfigArn"].(string)

			lookupKey := rcIdentifier
			if tt.lookupByArn {
				lookupKey = rcArn
			}

			modRec := doDMS(t, h, "ModifyReplicationConfig", map[string]any{
				"ReplicationConfigArn": lookupKey,
				"ReplicationType":      "cdc",
			})
			require.Equal(t, http.StatusOK, modRec.Code)

			updated := parseJSON(t, modRec)["ReplicationConfig"].(map[string]any)
			assert.Equal(t, "cdc", updated["ReplicationType"],
				"ModifyReplicationConfig must persist the updated ReplicationType")
		})
	}
}

// TestCreateReplicationConfig_ComputeConfigAndTableMappingsRoundTrip proves
// gopherstack-4ggy's fix: ComputeConfig and TableMappings are both required
// CreateReplicationConfigInput members (api_op_CreateReplicationConfig.go:30-81)
// that the pre-fix handler never read at all. This drives real field values
// (not empty placeholders) through Create and checks they come back on the
// response, matching real AWS's CreateReplicationConfigOutput.ReplicationConfig
// (types.go:3820) which echoes ComputeConfig verbatim.
func TestCreateReplicationConfig_ComputeConfigAndTableMappingsRoundTrip(t *testing.T) {
	t.Parallel()

	h := newTestDMSHandler()

	createRec := doDMS(t, h, "CreateReplicationConfig", map[string]any{
		"ReplicationConfigIdentifier": "rc-roundtrip",
		"ReplicationType":             "full-load",
		"SourceEndpointArn":           "arn:src",
		"TargetEndpointArn":           "arn:tgt",
		"TableMappings":               `{"rules":[{"rule-id":"1"}]}`,
		"ComputeConfig": map[string]any{
			"MaxCapacityUnits": 8,
			"MinCapacityUnits": 1,
			"MultiAZ":          true,
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	rc := parseJSON(t, createRec)["ReplicationConfig"].(map[string]any)
	assert.JSONEq(t, `{"rules":[{"rule-id":"1"}]}`, rc["TableMappings"].(string))

	cc, ok := rc["ComputeConfig"].(map[string]any)
	require.True(t, ok, "ComputeConfig must be echoed back on CreateReplicationConfig's response")
	assert.InDelta(t, float64(8), cc["MaxCapacityUnits"], 0)
	assert.InDelta(t, float64(1), cc["MinCapacityUnits"], 0)
	assert.Equal(t, true, cc["MultiAZ"])

	// DescribeReplicationConfigs must echo the same fields back too.
	descRec := doDMS(t, h, "DescribeReplicationConfigs", map[string]any{})
	require.Equal(t, http.StatusOK, descRec.Code)

	described := parseJSON(t, descRec)["ReplicationConfigs"].([]any)[0].(map[string]any)
	assert.JSONEq(t, `{"rules":[{"rule-id":"1"}]}`, described["TableMappings"].(string))
	assert.NotNil(t, described["ComputeConfig"])
}

func TestCreateReplicationConfig_MissingRequiredFields_ReturnsError(t *testing.T) {
	t.Parallel()

	base := func() map[string]any {
		return map[string]any{
			"ReplicationConfigIdentifier": "rc-missing",
			"ReplicationType":             "full-load",
			"SourceEndpointArn":           "arn:src",
			"TargetEndpointArn":           "arn:tgt",
			"TableMappings":               "{}",
			"ComputeConfig":               map[string]any{},
		}
	}

	tests := map[string]func(body map[string]any){
		"missing compute config": func(body map[string]any) { delete(body, "ComputeConfig") },
		"missing table mappings": func(body map[string]any) { delete(body, "TableMappings") },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newTestDMSHandler()
			body := base()
			mutate(body)

			rec := doDMS(t, h, "CreateReplicationConfig", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}
