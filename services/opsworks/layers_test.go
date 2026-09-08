package opsworks_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/opsworks"
)

// TestLayer verifies layer CRUD operations return correct fields.
func TestLayer(t *testing.T) {
	t.Parallel()

	createStack := func(h *opsworks.Handler) string {
		rec := doTarget(t, h, "CreateStack", map[string]any{
			"Name":                      "stack",
			"Region":                    "us-east-1",
			"DefaultInstanceProfileArn": "arn:aws:iam::000000000000:instance-profile/test",
			"ServiceRoleArn":            "arn:aws:iam::000000000000:role/test",
		})
		require.Equal(t, http.StatusOK, rec.Code)
		resp := parseJSON(t, rec.Body.Bytes())

		return resp["StackId"].(string)
	}

	tests := []struct {
		check     func(t *testing.T, h *opsworks.Handler, stackID string)
		name      string
		operation string
	}{
		{
			name:      "CreateLayer returns LayerId",
			operation: "CreateLayer",
			check: func(t *testing.T, h *opsworks.Handler, stackID string) {
				t.Helper()
				rec := doTarget(t, h, "CreateLayer", map[string]any{
					"StackId":   stackID,
					"Type":      "custom",
					"Name":      "my-layer",
					"Shortname": "ml",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())
				layerID, ok := resp["LayerId"].(string)
				assert.True(t, ok)
				assert.NotEmpty(t, layerID)
			},
		},
		{
			name:      "DescribeLayers returns layer with CreatedAt and Arn",
			operation: "DescribeLayers",
			check: func(t *testing.T, h *opsworks.Handler, stackID string) {
				t.Helper()
				doTarget(t, h, "CreateLayer", map[string]any{
					"StackId":   stackID,
					"Type":      "custom",
					"Name":      "my-layer",
					"Shortname": "ml",
				})
				rec := doTarget(t, h, "DescribeLayers", map[string]any{"StackId": stackID})
				require.Equal(t, http.StatusOK, rec.Code)
				resp := parseJSON(t, rec.Body.Bytes())
				layers, ok := resp["Layers"].([]any)
				require.True(t, ok)
				require.Len(t, layers, 1)
				layer := layers[0].(map[string]any)
				assert.NotEmpty(t, layer["LayerId"])
				assert.NotEmpty(t, layer["Arn"])
				assert.NotEmpty(t, layer["CreatedAt"])
				assert.Equal(t, "custom", layer["Type"])
				assert.Equal(t, "my-layer", layer["Name"])
			},
		},
		{
			name:      "UpdateLayer modifies name",
			operation: "UpdateLayer",
			check: func(t *testing.T, h *opsworks.Handler, stackID string) {
				t.Helper()
				rec := doTarget(t, h, "CreateLayer", map[string]any{
					"StackId":   stackID,
					"Type":      "custom",
					"Name":      "old-name",
					"Shortname": "on",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				layerID := parseJSON(t, rec.Body.Bytes())["LayerId"].(string)

				rec = doTarget(t, h, "UpdateLayer", map[string]any{
					"LayerId": layerID,
					"Name":    "new-name",
				})
				assert.Equal(t, http.StatusOK, rec.Code)
			},
		},
		{
			name:      "DeleteLayer removes layer",
			operation: "DeleteLayer",
			check: func(t *testing.T, h *opsworks.Handler, stackID string) {
				t.Helper()
				rec := doTarget(t, h, "CreateLayer", map[string]any{
					"StackId":   stackID,
					"Type":      "custom",
					"Name":      "to-delete",
					"Shortname": "td",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				layerID := parseJSON(t, rec.Body.Bytes())["LayerId"].(string)

				rec = doTarget(t, h, "DeleteLayer", map[string]any{"LayerId": layerID})
				assert.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DescribeLayers", map[string]any{"StackId": stackID})
				resp := parseJSON(t, rec.Body.Bytes())
				assert.Empty(t, resp["Layers"].([]any))
			},
		},
		{
			// api_op_DeleteLayer.go: "You must first stop and then delete
			// all associated instances or unassign registered instances."
			name:      "DeleteLayer with an associated instance returns ValidationException",
			operation: "DeleteLayer",
			check: func(t *testing.T, h *opsworks.Handler, stackID string) {
				t.Helper()
				rec := doTarget(t, h, "CreateLayer", map[string]any{
					"StackId":   stackID,
					"Type":      "custom",
					"Name":      "occupied",
					"Shortname": "oc",
				})
				require.Equal(t, http.StatusOK, rec.Code)
				layerID := parseJSON(t, rec.Body.Bytes())["LayerId"].(string)

				rec = doTarget(t, h, "CreateInstance", map[string]any{
					"StackId":      stackID,
					"LayerIds":     []string{layerID},
					"InstanceType": "t2.micro",
				})
				require.Equal(t, http.StatusOK, rec.Code)

				rec = doTarget(t, h, "DeleteLayer", map[string]any{"LayerId": layerID})
				assert.Equal(t, http.StatusBadRequest, rec.Code)
				assert.Contains(t, rec.Body.String(), "ValidationException")

				rec = doTarget(t, h, "DescribeLayers", map[string]any{"StackId": stackID})
				resp := parseJSON(t, rec.Body.Bytes())
				assert.Len(t, resp["Layers"].([]any), 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			stackID := createStack(h)
			tt.check(t, h, stackID)
		})
	}
}

// TestCreateLayerValidation verifies CreateLayer rejects requests missing a
// required member or using a Type outside the real LayerType enum. Name,
// Shortname, StackId, and Type are all "This member is required" on the
// real CreateLayerInput.
func TestCreateLayerValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		buildBody func(stackID string) map[string]any
		name      string
	}{
		{
			name: "missing Name",
			buildBody: func(stackID string) map[string]any {
				return map[string]any{"StackId": stackID, "Type": "custom", "Shortname": "sn"}
			},
		},
		{
			name: "missing Shortname",
			buildBody: func(stackID string) map[string]any {
				return map[string]any{"StackId": stackID, "Type": "custom", "Name": "n"}
			},
		},
		{
			name: "missing StackId",
			buildBody: func(_ string) map[string]any {
				return map[string]any{"Type": "custom", "Name": "n", "Shortname": "sn"}
			},
		},
		{
			name: "Type outside the LayerType enum",
			buildBody: func(stackID string) map[string]any {
				return map[string]any{"StackId": stackID, "Type": "not-a-real-type", "Name": "n", "Shortname": "sn"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doTarget(t, h, "CreateStack", map[string]any{
				"Name":                      "stack",
				"Region":                    "us-east-1",
				"DefaultInstanceProfileArn": "arn:aws:iam::000000000000:instance-profile/test",
				"ServiceRoleArn":            "arn:aws:iam::000000000000:role/test",
			})
			require.Equal(t, http.StatusOK, rec.Code)
			stackID := parseJSON(t, rec.Body.Bytes())["StackId"].(string)

			rec = doTarget(t, h, "CreateLayer", tt.buildBody(stackID))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "ValidationException")
		})
	}
}
