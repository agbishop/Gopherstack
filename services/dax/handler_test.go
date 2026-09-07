package dax_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/dax"
)

// newTestHandler creates a handler backed by a fresh in-memory backend.
func newTestHandler() *dax.Handler {
	return dax.NewHandler(dax.NewInMemoryBackend("123456789012", "us-east-1"))
}

// daxRequest sends a JSON request to the handler with the given target and body.
func daxRequest(t *testing.T, h *dax.Handler, target string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer

	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonDAXV3."+target)

	rec := httptest.NewRecorder()

	e := echo.New()
	c := e.NewContext(req, rec)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func validClusterBody(name string) map[string]any {
	return map[string]any{
		"ClusterName":       name,
		"NodeType":          "dax.r5.large",
		"IamRoleArn":        "arn:aws:iam::123456789012:role/DAXRole",
		"ReplicationFactor": 1,
	}
}

// ---- Error mapping ----

func TestHandlerErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		setup     func(t *testing.T, h *dax.Handler)
		body      map[string]any
		wantCode  string
	}{
		{
			name:      "ClusterAlreadyExistsFault",
			operation: "CreateCluster",
			setup: func(t *testing.T, h *dax.Handler) {
				t.Helper()
				daxRequest(t, h, "CreateCluster", validClusterBody("dup"))
			},
			body:     validClusterBody("dup"),
			wantCode: "ClusterAlreadyExistsFault",
		},
		{
			name:      "ClusterNotFoundFault on delete",
			operation: "DeleteCluster",
			setup:     func(_ *testing.T, _ *dax.Handler) {},
			body:      map[string]any{"ClusterName": "no-such"},
			wantCode:  "ClusterNotFoundFault",
		},
		{
			name:      "ParameterGroupNotFoundFault",
			operation: "DescribeParameters",
			setup:     func(_ *testing.T, _ *dax.Handler) {},
			body:      map[string]any{"ParameterGroupName": "missing"},
			wantCode:  "ParameterGroupNotFoundFault",
		},
		{
			name:      "SubnetGroupNotFoundFault",
			operation: "DescribeSubnetGroups",
			setup:     func(_ *testing.T, _ *dax.Handler) {},
			body:      map[string]any{"SubnetGroupNames": []string{"missing"}},
			wantCode:  "SubnetGroupNotFoundFault",
		},
		{
			name:      "InvalidParameterValueException for bad node type",
			operation: "CreateCluster",
			setup:     func(_ *testing.T, _ *dax.Handler) {},
			body: map[string]any{
				"ClusterName":       "x",
				"NodeType":          "bad.type",
				"IamRoleArn":        "arn:aws:iam::123456789012:role/r",
				"ReplicationFactor": 1,
			},
			wantCode: "InvalidParameterValueException",
		},
		{
			name:      "SubnetGroupAlreadyExistsFault",
			operation: "CreateSubnetGroup",
			setup: func(t *testing.T, h *dax.Handler) {
				t.Helper()
				daxRequest(t, h, "CreateSubnetGroup", map[string]any{
					"SubnetGroupName": "dup-sg",
					"SubnetIds":       []string{"subnet-11111111"},
				})
			},
			body: map[string]any{
				"SubnetGroupName": "dup-sg",
				"SubnetIds":       []string{"subnet-11111111"},
			},
			wantCode: "SubnetGroupAlreadyExistsFault",
		},
		{
			name:      "ParameterGroupAlreadyExistsFault",
			operation: "CreateParameterGroup",
			setup: func(t *testing.T, h *dax.Handler) {
				t.Helper()
				daxRequest(t, h, "CreateParameterGroup", map[string]any{"ParameterGroupName": "dup-pg"})
			},
			body:     map[string]any{"ParameterGroupName": "dup-pg"},
			wantCode: "ParameterGroupAlreadyExistsFault",
		},
		{
			name:      "InvalidAction for unknown operation",
			operation: "UnknownAction",
			setup:     func(_ *testing.T, _ *dax.Handler) {},
			body:      map[string]any{},
			wantCode:  "InvalidAction",
		},
		{
			// UpdateSubnetGroup's declared error set has no
			// InvalidParameterValueException case; SubnetGroupNotFoundFault is the
			// code it actually types for a missing/empty SubnetGroupName.
			name:      "SubnetGroupNotFoundFault on UpdateSubnetGroup with empty name",
			operation: "UpdateSubnetGroup",
			setup:     func(_ *testing.T, _ *dax.Handler) {},
			body:      map[string]any{"SubnetGroupName": ""},
			wantCode:  "SubnetGroupNotFoundFault",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler()
			tt.setup(t, h)

			rec := daxRequest(t, h, tt.operation, tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var errResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(t, tt.wantCode, errResp["__type"])
		})
	}
}

// ---- CreateSubnetGroup: SubnetGroupName has no format constraint ----

func TestHandlerCreateSubnetGroupNameNotFormatValidated(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	rec := daxRequest(t, h, "CreateSubnetGroup", map[string]any{
		"SubnetGroupName": "1sg--bad",
		"SubnetIds":       []string{"subnet-11111111"},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	sg := resp["SubnetGroup"].(map[string]any)
	assert.Equal(t, "1sg--bad", sg["SubnetGroupName"])
}

// ---- Reset / GetSupportedOperations ----

func TestHandlerReset(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	daxRequest(t, h, "CreateCluster", validClusterBody("r-cluster"))

	h.Reset()

	rec := daxRequest(t, h, "DescribeClusters", map[string]any{})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	clusters := resp["Clusters"].([]any)
	assert.Empty(t, clusters)
}

func TestHandlerGetSupportedOperations(t *testing.T) {
	t.Parallel()
	h := newTestHandler()
	ops := h.GetSupportedOperations()

	expected := []string{
		"CreateCluster",
		"DescribeClusters",
		"UpdateCluster",
		"DeleteCluster",
		"IncreaseReplicationFactor",
		"DecreaseReplicationFactor",
		"RebootNode",
		"TagResource",
		"ListTags",
		"CreateParameterGroup",
		"DescribeParameterGroups",
		"UpdateParameterGroup",
		"DeleteParameterGroup",
		"DescribeParameters",
		"DescribeDefaultParameters",
		"CreateSubnetGroup",
		"DescribeSubnetGroups",
		"UpdateSubnetGroup",
		"DeleteSubnetGroup",
		"DescribeEvents",
	}

	for _, op := range expected {
		assert.Contains(t, ops, op)
	}

	// "ResetParameterGroup" is deliberately NOT advertised: it is not a real DAX
	// SDK operation (no such action exists in the real API) — see its comment in
	// handler.go.
	assert.NotContains(t, ops, "ResetParameterGroup")
}
