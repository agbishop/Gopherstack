package apprunner_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionCRUD(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:     "create returns ARN",
			action:   "CreateConnection",
			body:     map[string]any{"ConnectionName": "my-conn", "ProviderType": "GITHUB"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				conn := resp["Connection"].(map[string]any)
				assert.Contains(t, conn["ConnectionArn"], "connection/my-conn/")
				assert.Equal(t, "AVAILABLE", conn["Status"])
				assert.Equal(t, "GITHUB", conn["ProviderType"])
			},
		},
		{
			name:     "create missing name returns 400",
			action:   "CreateConnection",
			body:     map[string]any{"ProviderType": "GITHUB"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "create missing provider type returns 400",
			action:   "CreateConnection",
			body:     map[string]any{"ConnectionName": "x"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, tc.action, tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestConnectionDeleteList(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateConnection", map[string]any{"ConnectionName": "conn1", "ProviderType": "GITHUB"})
	require.Equal(t, http.StatusOK, rec.Code)
	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	connArn := createResp["Connection"].(map[string]any)["ConnectionArn"].(string)

	doRequest(t, h, "CreateConnection", map[string]any{"ConnectionName": "conn2", "ProviderType": "BITBUCKET"})

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:     "list returns 2 connections",
			action:   "ListConnections",
			body:     map[string]any{},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				list := resp["ConnectionSummaryList"].([]any)
				assert.Len(t, list, 2)
			},
		},
		{
			name:     "list with name filter returns 1",
			action:   "ListConnections",
			body:     map[string]any{"ConnectionName": "conn1"},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				list := resp["ConnectionSummaryList"].([]any)
				assert.Len(t, list, 1)
			},
		},
		{
			name:     "delete returns connection",
			action:   "DeleteConnection",
			body:     map[string]any{"ConnectionArn": connArn},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				conn := resp["Connection"].(map[string]any)
				assert.Equal(t, "conn1", conn["ConnectionName"])
			},
		},
		{
			name:   "delete unknown ARN returns 400",
			action: "DeleteConnection",
			body: map[string]any{
				"ConnectionArn": "arn:aws:apprunner:us-east-1:000000000000:connection/notexist/abc",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "delete missing ARN returns 400",
			action:   "DeleteConnection",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			rec := doRequest(t, h, tc.action, tc.body) //nolint:govet // existing issue.
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

// TestDeleteConnection_RejectsWhenServiceUsesIt verifies DeleteConnection
// fails while a service's CodeRepository authentication still references the
// connection (api_op_DeleteConnection.go: "You must first ensure that there
// are no running App Runner services that use this connection. If there are
// any, the DeleteConnection action fails."), and succeeds once that service
// no longer references it.
func TestDeleteConnection_RejectsWhenServiceUsesIt(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateConnection", map[string]any{"ConnectionName": "gh-conn", "ProviderType": "GITHUB"})
	require.Equal(t, http.StatusOK, rec.Code)
	var connResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &connResp))
	connArn := connResp["Connection"].(map[string]any)["ConnectionArn"].(string)

	rec = doRequest(t, h, "CreateService", map[string]any{
		"ServiceName": "code-svc",
		"SourceConfiguration": map[string]any{
			"AuthenticationConfiguration": map[string]any{"ConnectionArn": connArn},
			"CodeRepository": map[string]any{
				"RepositoryUrl":     "https://github.com/example/repo",
				"SourceCodeVersion": map[string]any{"Type": "BRANCH", "Value": "main"},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var svcResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &svcResp))
	svcArn := svcResp["Service"].(map[string]any)["ServiceArn"].(string)

	rec = doRequest(t, h, "DeleteConnection", map[string]any{"ConnectionArn": connArn})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "connection still referenced by code-svc must not be deletable")

	rec = doRequest(t, h, "DeleteService", map[string]any{"ServiceArn": svcArn})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DeleteConnection", map[string]any{"ConnectionArn": connArn})
	assert.Equal(t, http.StatusOK, rec.Code, "connection must be deletable once no service references it")
}
