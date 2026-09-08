package opsworks_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/opsworks"
)

// TestInstance verifies instance CRUD and lifecycle operations.
func TestInstance(t *testing.T) {
	t.Parallel()

	setup := func(h *opsworks.Handler) (string, string) {
		rec := doTarget(t, h, "CreateStack", map[string]any{
			"Name":                      "stack",
			"Region":                    "us-east-1",
			"DefaultInstanceProfileArn": "arn:aws:iam::000000000000:instance-profile/test",
			"ServiceRoleArn":            "arn:aws:iam::000000000000:role/test",
		})
		require.Equal(t, http.StatusOK, rec.Code)
		stackID := parseJSON(t, rec.Body.Bytes())["StackId"].(string)

		rec = doTarget(t, h, "CreateLayer", map[string]any{
			"StackId":   stackID,
			"Type":      "custom",
			"Name":      "layer",
			"Shortname": "l",
		})
		require.Equal(t, http.StatusOK, rec.Code)
		layerID := parseJSON(t, rec.Body.Bytes())["LayerId"].(string)

		return stackID, layerID
	}

	tests := []struct {
		check func(t *testing.T, h *opsworks.Handler, stackID, layerID string)
		name  string
	}{
		{
			name: "CreateInstance returns InstanceId",
			check: func(t *testing.T, h *opsworks.Handler, stackID, layerID string) {
				t.Helper()
				rec := doTarget(t, h, "CreateInstance", map[string]any{
					"StackId":      stackID,
					"LayerIds":     []string{layerID},
					"InstanceType": "t2.micro",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())
				assert.NotEmpty(t, resp["InstanceId"])
			},
		},
		{
			name: "DescribeInstances returns instance with Status, Arn, and LayerIds list",
			check: func(t *testing.T, h *opsworks.Handler, stackID, layerID string) {
				t.Helper()
				doTarget(t, h, "CreateInstance", map[string]any{
					"StackId":      stackID,
					"LayerIds":     []string{layerID},
					"InstanceType": "t2.micro",
				})
				rec := doTarget(t, h, "DescribeInstances", map[string]any{"StackId": stackID})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())
				instances, ok := resp["Instances"].([]any)
				require.True(t, ok)
				require.Len(t, instances, 1)
				inst := instances[0].(map[string]any)
				assert.NotEmpty(t, inst["InstanceId"])
				assert.NotEmpty(t, inst["Arn"])
				assert.NotEmpty(t, inst["Status"])
				assert.NotEmpty(t, inst["CreatedAt"])
				assert.Equal(t, "t2.micro", inst["InstanceType"])
				// The real types.Instance has a plural LayerIds []string
				// member, not a singular "LayerId" string -- a previous
				// pass emitted the latter, which a real SDK client's
				// Instance.LayerIds would never populate from.
				layerIDs, ok := inst["LayerIds"].([]any)
				require.True(t, ok, "LayerIds must be a JSON array")
				require.Len(t, layerIDs, 1)
				assert.Equal(t, layerID, layerIDs[0])
				assert.NotContains(t, inst, "LayerId")
			},
		},
		{
			name: "StartInstance transitions status",
			check: func(t *testing.T, h *opsworks.Handler, stackID, layerID string) {
				t.Helper()
				rec := doTarget(t, h, "CreateInstance", map[string]any{
					"StackId":      stackID,
					"LayerIds":     []string{layerID},
					"InstanceType": "t2.micro",
				})
				instanceID := parseJSON(t, rec.Body.Bytes())["InstanceId"].(string)

				rec = doTarget(t, h, "StartInstance", map[string]any{"InstanceId": instanceID})
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DescribeInstances", map[string]any{
					"InstanceIds": []string{instanceID},
				})
				resp := parseJSON(t, rec.Body.Bytes())
				inst := resp["Instances"].([]any)[0].(map[string]any)
				// StartInstance commits directly to the terminal "online" status
				// rather than parking the instance in a transient status that
				// nothing ever advances (that used to leave DescribeInstances
				// pollers spinning on "starting" forever).
				assert.Equal(t, "online", inst["Status"])
			},
		},
		{
			name: "StopInstance transitions status",
			check: func(t *testing.T, h *opsworks.Handler, stackID, layerID string) {
				t.Helper()
				rec := doTarget(t, h, "CreateInstance", map[string]any{
					"StackId":      stackID,
					"LayerIds":     []string{layerID},
					"InstanceType": "t2.micro",
				})
				instanceID := parseJSON(t, rec.Body.Bytes())["InstanceId"].(string)

				rec = doTarget(t, h, "StopInstance", map[string]any{"InstanceId": instanceID})
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DescribeInstances", map[string]any{
					"InstanceIds": []string{instanceID},
				})
				resp := parseJSON(t, rec.Body.Bytes())
				inst := resp["Instances"].([]any)[0].(map[string]any)
				// StopInstance commits directly to the terminal "stopped" status
				// rather than parking the instance in a transient status that
				// nothing ever advances.
				assert.Equal(t, "stopped", inst["Status"])
			},
		},
		{
			name: "RebootInstance transitions status back to online",
			check: func(t *testing.T, h *opsworks.Handler, stackID, layerID string) {
				t.Helper()
				rec := doTarget(t, h, "CreateInstance", map[string]any{
					"StackId":      stackID,
					"LayerIds":     []string{layerID},
					"InstanceType": "t2.micro",
				})
				instanceID := parseJSON(t, rec.Body.Bytes())["InstanceId"].(string)

				doTarget(t, h, "StartInstance", map[string]any{"InstanceId": instanceID})

				rec = doTarget(t, h, "RebootInstance", map[string]any{"InstanceId": instanceID})
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DescribeInstances", map[string]any{
					"InstanceIds": []string{instanceID},
				})
				resp := parseJSON(t, rec.Body.Bytes())
				inst := resp["Instances"].([]any)[0].(map[string]any)
				assert.Equal(t, "online", inst["Status"])
			},
		},
		{
			name: "StartInstance is idempotent when already online",
			check: func(t *testing.T, h *opsworks.Handler, stackID, layerID string) {
				t.Helper()
				rec := doTarget(t, h, "CreateInstance", map[string]any{
					"StackId":      stackID,
					"LayerIds":     []string{layerID},
					"InstanceType": "t2.micro",
				})
				instanceID := parseJSON(t, rec.Body.Bytes())["InstanceId"].(string)

				doTarget(t, h, "StartInstance", map[string]any{"InstanceId": instanceID})
				rec = doTarget(t, h, "StartInstance", map[string]any{"InstanceId": instanceID})
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DescribeInstances", map[string]any{
					"InstanceIds": []string{instanceID},
				})
				resp := parseJSON(t, rec.Body.Bytes())
				inst := resp["Instances"].([]any)[0].(map[string]any)
				assert.Equal(t, "online", inst["Status"])
			},
		},
		{
			name: "DeleteInstance succeeds on a stopped instance",
			check: func(t *testing.T, h *opsworks.Handler, stackID, layerID string) {
				t.Helper()
				rec := doTarget(t, h, "CreateInstance", map[string]any{
					"StackId":      stackID,
					"LayerIds":     []string{layerID},
					"InstanceType": "t2.micro",
				})
				instanceID := parseJSON(t, rec.Body.Bytes())["InstanceId"].(string)

				rec = doTarget(t, h, "DeleteInstance", map[string]any{"InstanceId": instanceID})
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DescribeInstances", map[string]any{"StackId": stackID})
				resp := parseJSON(t, rec.Body.Bytes())
				assert.Empty(t, resp["Instances"].([]any))
			},
		},
		{
			// api_op_DeleteInstance.go: "You must stop an instance before
			// you can delete it."
			name: "DeleteInstance on an online instance returns ValidationException",
			check: func(t *testing.T, h *opsworks.Handler, stackID, layerID string) {
				t.Helper()
				rec := doTarget(t, h, "CreateInstance", map[string]any{
					"StackId":      stackID,
					"LayerIds":     []string{layerID},
					"InstanceType": "t2.micro",
				})
				instanceID := parseJSON(t, rec.Body.Bytes())["InstanceId"].(string)

				doTarget(t, h, "StartInstance", map[string]any{"InstanceId": instanceID})

				rec = doTarget(t, h, "DeleteInstance", map[string]any{"InstanceId": instanceID})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")

				rec = doTarget(t, h, "DescribeInstances", map[string]any{"StackId": stackID})
				resp := parseJSON(t, rec.Body.Bytes())
				assert.Len(t, resp["Instances"].([]any), 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			stackID, layerID := setup(h)
			tt.check(t, h, stackID, layerID)
		})
	}
}

// TestRegisterDeregisterInstance verifies instance registration lifecycle.
func TestRegisterDeregisterInstance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check func(t *testing.T, h *opsworks.Handler)
		name  string
	}{
		{
			name: "RegisterInstance returns InstanceId",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				rec := doTarget(t, h, "RegisterInstance", map[string]any{
					"StackId":  stackID,
					"Hostname": "my-server",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())
				assert.NotEmpty(t, resp["InstanceId"])
			},
		},
		{
			name: "DeregisterInstance removes instance",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				rec := doTarget(t, h, "RegisterInstance", map[string]any{
					"StackId":  stackID,
					"Hostname": "to-deregister",
				})
				instanceID := parseJSON(t, rec.Body.Bytes())["InstanceId"].(string)

				rec = doTarget(t, h, "DeregisterInstance", map[string]any{
					"InstanceId": instanceID,
				})
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DescribeInstances", map[string]any{
					"InstanceIds": []string{instanceID},
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "RegisterInstance on nonexistent stack returns 404",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				rec := doTarget(t, h, "RegisterInstance", map[string]any{
					"StackId":  "nonexistent",
					"Hostname": "host",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			// StackId is "This member is required" on the real
			// RegisterInstanceInput (confirmed against
			// aws-sdk-go-v2/service/opsworks@v1.31.0's
			// api_op_RegisterInstance.go); a missing one must be
			// ValidationException, not fall through to the stack-lookup's
			// ResourceNotFoundException.
			name: "RegisterInstance with missing StackId returns 400",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				rec := doTarget(t, h, "RegisterInstance", map[string]any{
					"Hostname": "host",
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			tt.check(t, h)
		})
	}
}

// TestAssignUnassignInstance verifies AssignInstance/UnassignInstance.
func TestAssignUnassignInstance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check func(t *testing.T, h *opsworks.Handler)
		name  string
	}{
		{
			// AssignInstance is for *registered* (on-premises) instances --
			// AWS's own docs say "You cannot use this action with instances
			// that were created with OpsWorks" -- so this uses
			// RegisterInstance rather than CreateInstance, unlike the rest
			// of this file's helpers.
			name: "AssignInstance assigns to layer",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				layerID := createTestLayer(t, h, stackID)

				rec := doTarget(t, h, "RegisterInstance", map[string]any{
					"StackId":  stackID,
					"Hostname": "on-prem-host",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				instanceID := parseJSON(t, rec.Body.Bytes())["InstanceId"].(string)

				rec = doTarget(t, h, "AssignInstance", map[string]any{
					"InstanceId": instanceID,
					"LayerIds":   []string{layerID},
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "AssignInstance with layer from a different stack returns 400",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				otherStackID := createTestStack(t, h)
				otherLayerID := createTestLayer(t, h, otherStackID)

				rec := doTarget(t, h, "RegisterInstance", map[string]any{
					"StackId":  stackID,
					"Hostname": "cross-stack-host",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				instanceID := parseJSON(t, rec.Body.Bytes())["InstanceId"].(string)

				rec = doTarget(t, h, "AssignInstance", map[string]any{
					"InstanceId": instanceID,
					"LayerIds":   []string{otherLayerID},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
			},
		},
		{
			// AssignInstance's doc comment (aws-sdk-go-v2/service/
			// opsworks@v1.31.0's api_op_AssignInstance.go) says "You cannot
			// use this action with instances that were created with
			// OpsWorks Stacks" -- i.e. CreateInstance'd instances, as
			// opposed to RegisterInstance'd ones.
			name: "AssignInstance rejects an instance created with OpsWorks",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				layerID := createTestLayer(t, h, stackID)
				instanceID := createTestInstance(t, h, stackID, layerID)
				otherLayerID := createTestLayer(t, h, stackID)

				rec := doTarget(t, h, "AssignInstance", map[string]any{
					"InstanceId": instanceID,
					"LayerIds":   []string{otherLayerID},
				})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")
			},
		},
		{
			name: "AssignInstance with nonexistent layer returns 404",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)

				rec := doTarget(t, h, "RegisterInstance", map[string]any{
					"StackId":  stackID,
					"Hostname": "no-layer-host",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				instanceID := parseJSON(t, rec.Body.Bytes())["InstanceId"].(string)

				rec = doTarget(t, h, "AssignInstance", map[string]any{
					"InstanceId": instanceID,
					"LayerIds":   []string{"nonexistent-layer"},
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "UnassignInstance returns OK",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				layerID := createTestLayer(t, h, stackID)
				instanceID := createTestInstance(t, h, stackID, layerID)

				rec := doTarget(t, h, "UnassignInstance", map[string]any{
					"InstanceId": instanceID,
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			tt.check(t, h)
		})
	}
}

// TestCreateInstanceHostnameUnique verifies hostnames are unique.
func TestCreateInstanceHostnameUnique(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	stackID := createTestStack(t, h)
	layerID := createTestLayer(t, h, stackID)

	seen := make(map[string]bool)
	for range 10 {
		rec := doTarget(t, h, "CreateInstance", map[string]any{
			"StackId":      stackID,
			"LayerIds":     []string{layerID},
			"InstanceType": "t2.micro",
		})
		require.Equal(t, http.StatusOK, rec.Code)
		instanceID := parseJSON(t, rec.Body.Bytes())["InstanceId"].(string)

		rec = doTarget(t, h, "DescribeInstances", map[string]any{
			"InstanceIds": []string{instanceID},
		})
		inst := parseJSON(t, rec.Body.Bytes())["Instances"].([]any)[0].(map[string]any)
		hostname := inst["Hostname"].(string)
		assert.False(t, seen[hostname], "duplicate hostname: %s", hostname)
		seen[hostname] = true
	}
}

// TestCreateInstanceValidation verifies CreateInstance rejects requests
// missing a required member. StackId, LayerIds (at least one), and
// InstanceType are all "This member is required" on the real
// CreateInstanceInput.
func TestCreateInstanceValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		buildBody func(stackID, layerID string) map[string]any
		name      string
	}{
		{
			name: "missing StackId",
			buildBody: func(_, layerID string) map[string]any {
				return map[string]any{"LayerIds": []string{layerID}, "InstanceType": "t2.micro"}
			},
		},
		{
			name: "missing LayerIds",
			buildBody: func(stackID, _ string) map[string]any {
				return map[string]any{"StackId": stackID, "InstanceType": "t2.micro"}
			},
		},
		{
			name: "missing InstanceType",
			buildBody: func(stackID, layerID string) map[string]any {
				return map[string]any{"StackId": stackID, "LayerIds": []string{layerID}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			stackID := createTestStack(t, h)
			layerID := createTestLayer(t, h, stackID)

			rec := doTarget(t, h, "CreateInstance", tt.buildBody(stackID, layerID))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "ValidationException")
		})
	}
}
