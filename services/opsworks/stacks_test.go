package opsworks_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/opsworks"
)

// TestStack verifies stack CRUD operations return correct fields.
func TestStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body      any
		setup     func(h *opsworks.Handler) string
		check     func(t *testing.T, body []byte, setupID string)
		name      string
		operation string
		wantCode  int
	}{
		{
			name:      "CreateStack returns StackId",
			operation: "CreateStack",
			body: map[string]any{
				"Name":                      "my-stack",
				"Region":                    "us-east-1",
				"DefaultInstanceProfileArn": "arn:aws:iam::000000000000:instance-profile/test",
				"ServiceRoleArn":            "arn:aws:iam::000000000000:role/test",
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte, _ string) {
				t.Helper()
				resp := parseJSON(t, body)
				stackID, ok := resp["StackId"].(string)
				assert.True(t, ok, "StackId should be a string")
				assert.NotEmpty(t, stackID)
			},
		},
		{
			name:      "DescribeStacks returns stack with CreatedAt",
			operation: "DescribeStacks",
			body:      map[string]any{},
			setup: func(h *opsworks.Handler) string {
				rec := doTarget(t, h, "CreateStack", map[string]any{
					"Name":                      "my-stack",
					"Region":                    "us-east-1",
					"DefaultInstanceProfileArn": "arn:aws:iam::000000000000:instance-profile/test",
					"ServiceRoleArn":            "arn:aws:iam::000000000000:role/test",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())

				return resp["StackId"].(string)
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte, _ string) {
				t.Helper()
				resp := parseJSON(t, body)
				stacks, ok := resp["Stacks"].([]any)
				require.True(t, ok)
				require.Len(t, stacks, 1)
				stack := stacks[0].(map[string]any)
				assert.NotEmpty(t, stack["StackId"])
				assert.NotEmpty(t, stack["Name"])
				assert.NotEmpty(t, stack["CreatedAt"])
				assert.NotEmpty(t, stack["Arn"])
				// The real types.Stack has no Status member; a previous
				// pass invented one and put it on the wire.
				assert.NotContains(t, stack, "Status")
			},
		},
		{
			name:      "DescribeStacks by ID returns correct stack",
			operation: "DescribeStacks",
			setup: func(h *opsworks.Handler) string {
				rec := doTarget(t, h, "CreateStack", map[string]any{
					"Name":                      "targeted-stack",
					"Region":                    "us-east-1",
					"DefaultInstanceProfileArn": "arn:aws:iam::000000000000:instance-profile/test",
					"ServiceRoleArn":            "arn:aws:iam::000000000000:role/test",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())

				return resp["StackId"].(string)
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte, setupID string) {
				t.Helper()
				resp := parseJSON(t, body)
				stacks := resp["Stacks"].([]any)
				require.Len(t, stacks, 1)
				stack := stacks[0].(map[string]any)
				assert.Equal(t, setupID, stack["StackId"])
				assert.Equal(t, "targeted-stack", stack["Name"])
			},
		},
		{
			name:      "UpdateStack modifies name",
			operation: "UpdateStack",
			setup: func(h *opsworks.Handler) string {
				rec := doTarget(t, h, "CreateStack", map[string]any{
					"Name":                      "old-name",
					"Region":                    "us-east-1",
					"DefaultInstanceProfileArn": "arn:aws:iam::000000000000:instance-profile/test",
					"ServiceRoleArn":            "arn:aws:iam::000000000000:role/test",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())

				return resp["StackId"].(string)
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte, _ string) {
				t.Helper()
				resp := parseJSON(t, body)
				assert.Empty(t, resp)
			},
		},
		{
			name:      "DeleteStack removes stack",
			operation: "DeleteStack",
			setup: func(h *opsworks.Handler) string {
				rec := doTarget(t, h, "CreateStack", map[string]any{
					"Name":                      "to-delete",
					"Region":                    "us-east-1",
					"DefaultInstanceProfileArn": "arn:aws:iam::000000000000:instance-profile/test",
					"ServiceRoleArn":            "arn:aws:iam::000000000000:role/test",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())

				return resp["StackId"].(string)
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte, _ string) {
				t.Helper()
				resp := parseJSON(t, body)
				assert.Empty(t, resp)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			var setupID string
			if tt.setup != nil {
				setupID = tt.setup(h)
			}

			body := tt.body
			if body == nil && setupID != "" {
				switch tt.operation {
				case "DescribeStacks":
					body = map[string]any{"StackIds": []string{setupID}}
				case "UpdateStack":
					body = map[string]any{"StackId": setupID, "Name": "new-name"}
				case "DeleteStack":
					body = map[string]any{"StackId": setupID}
				}
			}

			rec := doTarget(t, h, tt.operation, body)
			require.Equal(t, tt.wantCode, rec.Code)

			if tt.check != nil {
				tt.check(t, rec.Body.Bytes(), setupID)
			}
		})
	}
}

// TestCreateStackValidation verifies CreateStack rejects requests missing a
// required member. Name, Region, DefaultInstanceProfileArn, and
// ServiceRoleArn are all "This member is required" on the real
// CreateStackInput (confirmed against
// aws-sdk-go-v2/service/opsworks@v1.31.0's api_op_CreateStack.go).
func TestCreateStackValidation(t *testing.T) {
	t.Parallel()

	full := map[string]any{
		"Name":                      "n",
		"Region":                    "us-east-1",
		"DefaultInstanceProfileArn": "arn:aws:iam::000000000000:instance-profile/test",
		"ServiceRoleArn":            "arn:aws:iam::000000000000:role/test",
	}

	tests := []struct {
		name    string
		missing string
	}{
		{name: "missing Name", missing: "Name"},
		{name: "missing Region", missing: "Region"},
		{name: "missing DefaultInstanceProfileArn", missing: "DefaultInstanceProfileArn"},
		{name: "missing ServiceRoleArn", missing: "ServiceRoleArn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := make(map[string]any, len(full))
			for k, v := range full {
				if k != tt.missing {
					body[k] = v
				}
			}

			h := newTestHandler(t)
			rec := doTarget(t, h, "CreateStack", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "ValidationException")
		})
	}
}

// TestCreateStackOptionalParams verifies CreateStack accepts and echoes
// back VpcId, Attributes, ConfigurationManager, and ChefConfiguration --
// all real optional CreateStackInput members (confirmed against
// aws-sdk-go-v2/service/opsworks@v1.31.0's api_op_CreateStack.go /
// types.go) that a previous pass's Handler never decoded at all.
func TestCreateStackOptionalParams(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doTarget(t, h, "CreateStack", map[string]any{
		"Name":                      "opt-stack",
		"Region":                    "us-east-1",
		"DefaultInstanceProfileArn": "arn:aws:iam::000000000000:instance-profile/test",
		"ServiceRoleArn":            "arn:aws:iam::000000000000:role/test",
		"VpcId":                     "vpc-abc123",
		"Attributes":                map[string]any{"Color": "blue"},
		"ConfigurationManager":      map[string]any{"Name": "Chef", "Version": "12"},
		"ChefConfiguration":         map[string]any{"ManageBerkshelf": true, "BerkshelfVersion": "5.1"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	stackID := parseJSON(t, rec.Body.Bytes())["StackId"].(string)

	rec = doTarget(t, h, "DescribeStacks", map[string]any{"StackIds": []string{stackID}})
	require.Equal(t, http.StatusOK, rec.Code)
	stack := parseJSON(t, rec.Body.Bytes())["Stacks"].([]any)[0].(map[string]any)

	assert.Equal(t, "vpc-abc123", stack["VpcId"])
	assert.Equal(t, map[string]any{"Color": "blue"}, stack["Attributes"])
	assert.Equal(t, map[string]any{"Name": "Chef", "Version": "12"}, stack["ConfigurationManager"])
	assert.Equal(t, map[string]any{"ManageBerkshelf": true, "BerkshelfVersion": "5.1"}, stack["ChefConfiguration"])
}

// TestCloneStack verifies CloneStack creates an independent copy.
func TestCloneStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check func(t *testing.T, h *opsworks.Handler)
		name  string
	}{
		{
			name: "CloneStack returns new StackId",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				rec := doTarget(t, h, "CloneStack", map[string]any{
					"SourceStackId": stackID,
					"Name":          "cloned-stack",
					"Region":        "us-west-2",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())
				cloneID, ok := resp["StackId"].(string)
				require.True(t, ok)
				assert.NotEmpty(t, cloneID)
				assert.NotEqual(t, stackID, cloneID)
			},
		},
		{
			name: "CloneStack of nonexistent stack returns 404",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				rec := doTarget(t, h, "CloneStack", map[string]any{
					"SourceStackId": "nonexistent",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "cloned stack visible via DescribeStacks",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				rec := doTarget(t, h, "CloneStack", map[string]any{
					"SourceStackId": stackID,
					"Name":          "clone2",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				cloneID := parseJSON(t, rec.Body.Bytes())["StackId"].(string)

				rec = doTarget(t, h, "DescribeStacks", map[string]any{
					"StackIds": []string{cloneID},
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())
				stacks := resp["Stacks"].([]any)
				require.Len(t, stacks, 1)
				assert.Equal(t, "clone2", stacks[0].(map[string]any)["Name"])
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

// TestStartStopStack verifies StartStack and StopStack.
func TestStartStopStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check func(t *testing.T, h *opsworks.Handler)
		name  string
	}{
		{
			name: "StartStack returns empty response",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				rec := doTarget(t, h, "StartStack", map[string]any{"StackId": stackID})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "StopStack returns empty response",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				rec := doTarget(t, h, "StopStack", map[string]any{"StackId": stackID})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name: "StartStack on nonexistent stack returns 404",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				rec := doTarget(t, h, "StartStack", map[string]any{"StackId": "no-such-stack"})
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			// StartStack must commit its instances to the terminal "online"
			// status. Previously it parked them in "starting" with nothing to
			// ever advance them, so DescribeInstances pollers waiting for
			// "online" spun forever.
			name: "StartStack transitions stopped instances to online",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				layerID := createTestLayer(t, h, stackID)
				instanceID := createTestInstance(t, h, stackID, layerID)

				rec := doTarget(t, h, "StartStack", map[string]any{"StackId": stackID})
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
			// StopStack must commit its instances to the terminal "stopped"
			// status rather than leaving them stuck in "stopping".
			name: "StopStack transitions online instances to stopped",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				layerID := createTestLayer(t, h, stackID)
				instanceID := createTestInstance(t, h, stackID, layerID)
				doTarget(t, h, "StartStack", map[string]any{"StackId": stackID})

				rec := doTarget(t, h, "StopStack", map[string]any{"StackId": stackID})
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DescribeInstances", map[string]any{
					"InstanceIds": []string{instanceID},
				})
				resp := parseJSON(t, rec.Body.Bytes())
				inst := resp["Instances"].([]any)[0].(map[string]any)
				assert.Equal(t, "stopped", inst["Status"])
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

// TestGetHostnameSuggestion verifies hostname suggestions.
func TestGetHostnameSuggestion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check func(t *testing.T, h *opsworks.Handler)
		name  string
	}{
		{
			// Real GetHostnameSuggestionInput has only a LayerId member --
			// no StackId -- so a real SDK client never sends StackId.
			name: "returns non-empty hostname keyed by layer",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				layerID := createTestLayer(t, h, stackID)
				rec := doTarget(t, h, "GetHostnameSuggestion", map[string]any{
					"LayerId": layerID,
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())
				assert.NotEmpty(t, resp["Hostname"])
				assert.Equal(t, layerID, resp["LayerId"])
			},
		},
		{
			name: "returns 404 for nonexistent layer",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				rec := doTarget(t, h, "GetHostnameSuggestion", map[string]any{
					"LayerId": "nonexistent",
				})
				assert.Equal(t, http.StatusNotFound, rec.Code)
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

// TestDescribeStackSummary verifies DescribeStackSummary returns counts.
func TestDescribeStackSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check func(t *testing.T, h *opsworks.Handler)
		name  string
	}{
		{
			name: "returns summary with instance counts",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				layerID := createTestLayer(t, h, stackID)
				createTestInstance(t, h, stackID, layerID)

				rec := doTarget(t, h, "DescribeStackSummary", map[string]any{"StackId": stackID})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())
				summary, ok := resp["StackSummary"].(map[string]any)
				require.True(t, ok)
				assert.NotEmpty(t, summary["StackId"])
				assert.NotEmpty(t, summary["Name"])
				counts := summary["InstancesCount"].(map[string]any)
				// A freshly created instance is "stopped" (see
				// CreateInstance). InstancesCount's field set mirrors the
				// real types.InstancesCount exactly -- no "Total" or
				// "Starting" field exists on the real API.
				assert.InEpsilon(t, float64(1), counts["Stopped"], 0.001)
				assert.NotContains(t, counts, "Total")
				assert.NotContains(t, counts, "Starting")
				assert.Contains(t, counts, "Assigning")
				assert.Contains(t, counts, "Unassigning")
			},
		},
		{
			name: "nonexistent stack returns 404",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				rec := doTarget(t, h, "DescribeStackSummary", map[string]any{"StackId": "none"})
				assert.Equal(t, http.StatusNotFound, rec.Code)
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

// TestDescribeStackProvisioningParameters verifies provisioning params.
func TestDescribeStackProvisioningParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check func(t *testing.T, h *opsworks.Handler)
		name  string
	}{
		{
			name: "returns agent installer URL",
			check: func(t *testing.T, h *opsworks.Handler) {
				t.Helper()
				stackID := createTestStack(t, h)
				rec := doTarget(t, h, "DescribeStackProvisioningParameters", map[string]any{
					"StackId": stackID,
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())
				assert.NotEmpty(t, resp["AgentInstallerUrl"])
				// The real DescribeStackProvisioningParametersOutput has
				// only AgentInstallerUrl and Parameters members; a
				// previous pass invented a StackArn member and put it on
				// the wire.
				assert.NotContains(t, resp, "StackArn")
				// AgentInstallerUrl is a dedicated top-level field, never
				// also a member inside Parameters -- a previous version of
				// this handler duplicated it under a fabricated key inside
				// Parameters, which no real response ever carries there.
				params, ok := resp["Parameters"].(map[string]any)
				require.True(t, ok)
				assert.NotContains(t, params, "AgentInstallerUrl")
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

// TestDeleteStackCascade verifies DeleteStack's delete precondition
// (api_op_DeleteStack.go: "You must first delete all instances, layers, and
// apps or deregister registered instances.") -- refused with
// ValidationException while any of the three exist, and succeeding once
// they are all gone.
func TestDeleteStackCascade(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check func(t *testing.T, h *opsworks.Handler, b *opsworks.InMemoryBackend)
		name  string
	}{
		{
			name: "DeleteStack with a layer, instance, and app returns ValidationException",
			check: func(t *testing.T, h *opsworks.Handler, b *opsworks.InMemoryBackend) {
				t.Helper()
				stackID := createTestStack(t, h)
				layerID := createTestLayer(t, h, stackID)
				createTestInstance(t, h, stackID, layerID)

				rec := doTarget(t, h, "CreateApp", map[string]any{
					"StackId": stackID,
					"Name":    "occupied-app",
					"Type":    "other",
				})
				require.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DeleteStack", map[string]any{"StackId": stackID})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")

				assert.Equal(t, 1, opsworks.StackCount(b))
				assert.Equal(t, 1, opsworks.LayerCount(b))
				assert.Equal(t, 1, opsworks.InstanceCount(b))
				assert.Equal(t, 1, opsworks.AppCount(b))
			},
		},
		{
			name: "DeleteStack with only a layer returns ValidationException",
			check: func(t *testing.T, h *opsworks.Handler, b *opsworks.InMemoryBackend) {
				t.Helper()
				stackID := createTestStack(t, h)
				createTestLayer(t, h, stackID)

				rec := doTarget(t, h, "DeleteStack", map[string]any{"StackId": stackID})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")

				assert.Equal(t, 1, opsworks.StackCount(b))
				assert.Equal(t, 1, opsworks.LayerCount(b))
			},
		},
		{
			// RegisterInstance (unlike CreateInstance) takes no LayerId, so
			// this is a genuinely layer-free instance -- confirmed against
			// its backend signature (stackID, hostname only).
			name: "DeleteStack with only an instance returns ValidationException",
			check: func(t *testing.T, h *opsworks.Handler, b *opsworks.InMemoryBackend) {
				t.Helper()
				stackID := createTestStack(t, h)

				rec := doTarget(t, h, "RegisterInstance", map[string]any{
					"StackId":  stackID,
					"Hostname": "registered-only",
				})
				require.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DeleteStack", map[string]any{"StackId": stackID})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")

				assert.Equal(t, 1, opsworks.StackCount(b))
				assert.Equal(t, 0, opsworks.LayerCount(b))
				assert.Equal(t, 1, opsworks.InstanceCount(b))
			},
		},
		{
			name: "DeleteStack with only an app returns ValidationException",
			check: func(t *testing.T, h *opsworks.Handler, b *opsworks.InMemoryBackend) {
				t.Helper()
				stackID := createTestStack(t, h)

				rec := doTarget(t, h, "CreateApp", map[string]any{
					"StackId": stackID,
					"Name":    "app-only",
					"Type":    "other",
				})
				require.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DeleteStack", map[string]any{"StackId": stackID})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")

				assert.Equal(t, 1, opsworks.StackCount(b))
				assert.Equal(t, 0, opsworks.LayerCount(b))
				assert.Equal(t, 0, opsworks.InstanceCount(b))
				assert.Equal(t, 1, opsworks.AppCount(b))
			},
		},
		{
			name: "DeleteStack succeeds once the layer, instance, and app are removed",
			check: func(t *testing.T, h *opsworks.Handler, b *opsworks.InMemoryBackend) {
				t.Helper()
				stackID := createTestStack(t, h)
				layerID := createTestLayer(t, h, stackID)
				instanceID := createTestInstance(t, h, stackID, layerID)

				rec := doTarget(t, h, "CreateApp", map[string]any{
					"StackId": stackID,
					"Name":    "removable-app",
					"Type":    "other",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				appID := parseJSON(t, rec.Body.Bytes())["AppId"].(string)

				rec = doTarget(t, h, "DeleteInstance", map[string]any{"InstanceId": instanceID})
				require.Equal(t, http.StatusOK, rec.Code)
				rec = doTarget(t, h, "DeleteLayer", map[string]any{"LayerId": layerID})
				require.Equal(t, http.StatusOK, rec.Code)
				rec = doTarget(t, h, "DeleteApp", map[string]any{"AppId": appID})
				require.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DeleteStack", map[string]any{"StackId": stackID})
				require.Equal(t, http.StatusOK, rec.Code)

				assert.Equal(t, 0, opsworks.StackCount(b))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := opsworks.NewInMemoryBackend("000000000000", "us-east-1")
			h := opsworks.NewHandler(b)
			tt.check(t, h, b)
		})
	}
}
