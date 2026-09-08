package apprunner_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVpcConnectorCRUD(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:   "create returns ARN",
			action: "CreateVpcConnector",
			body: map[string]any{
				"VpcConnectorName": "my-vpc",
				"Subnets":          []string{"subnet-aaa"},
				"SecurityGroups":   []string{"sg-bbb"},
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				vc := resp["VpcConnector"].(map[string]any)
				assert.Contains(t, vc["VpcConnectorArn"], "vpcconnector/my-vpc/1/")
				assert.Equal(t, "ACTIVE", vc["Status"])
				assert.InDelta(t, float64(1), vc["VpcConnectorRevision"], 0.0001)
				subnets := vc["Subnets"].([]any)
				assert.Len(t, subnets, 1)
			},
		},
		{
			name:     "create missing name returns 400",
			action:   "CreateVpcConnector",
			body:     map[string]any{"Subnets": []string{"subnet-aaa"}},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "create missing subnets returns 400",
			action:   "CreateVpcConnector",
			body:     map[string]any{"VpcConnectorName": "x"},
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

func TestVpcConnectorDescribeDeleteList(t *testing.T) { //nolint:paralleltest // existing issue.
	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateVpcConnector", map[string]any{
		"VpcConnectorName": "vc1",
		"Subnets":          []string{"subnet-111"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	vcArn := createResp["VpcConnector"].(map[string]any)["VpcConnectorArn"].(string)

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		action   string
		wantCode int
	}{
		{
			name:     "describe returns connector",
			action:   "DescribeVpcConnector",
			body:     map[string]any{"VpcConnectorArn": vcArn},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				vc := resp["VpcConnector"].(map[string]any)
				assert.Equal(t, "vc1", vc["VpcConnectorName"])
			},
		},
		{
			name:     "describe missing ARN returns 400",
			action:   "DescribeVpcConnector",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "describe unknown ARN returns 400",
			action: "DescribeVpcConnector",
			body: map[string]any{
				"VpcConnectorArn": "arn:aws:apprunner:us-east-1:000000000000:vpcconnector/notexist/1/abc",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "list returns 1",
			action:   "ListVpcConnectors",
			body:     map[string]any{},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				list := resp["VpcConnectors"].([]any)
				assert.Len(t, list, 1)
			},
		},
		{
			name:     "delete returns connector",
			action:   "DeleteVpcConnector",
			body:     map[string]any{"VpcConnectorArn": vcArn},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				vc := resp["VpcConnector"].(map[string]any)
				assert.Equal(t, "vc1", vc["VpcConnectorName"])
			},
		},
		{
			name:   "delete unknown ARN returns 400",
			action: "DeleteVpcConnector",
			body: map[string]any{
				"VpcConnectorArn": "arn:aws:apprunner:us-east-1:000000000000:vpcconnector/notexist/1/abc",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "delete missing ARN returns 400",
			action:   "DeleteVpcConnector",
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

// TestDeleteVpcConnector_RejectsWhenServiceUsesIt verifies DeleteVpcConnector
// fails while a service's egress network configuration still references the
// connector (api_op_DeleteVpcConnector.go: "You can't delete a connector
// that's used by one or more App Runner services."), and succeeds once that
// service no longer references it.
func TestDeleteVpcConnector_RejectsWhenServiceUsesIt(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateVpcConnector", map[string]any{
		"VpcConnectorName": "vc-in-use",
		"Subnets":          []string{"subnet-1"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var vcResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &vcResp))
	vcArn := vcResp["VpcConnector"].(map[string]any)["VpcConnectorArn"].(string)

	rec = doRequest(t, h, "CreateService", map[string]any{
		"ServiceName": "vpc-svc",
		"SourceConfiguration": map[string]any{
			"ImageRepository": map[string]any{"ImageIdentifier": "img", "ImageRepositoryType": "ECR_PUBLIC"},
		},
		"NetworkConfiguration": map[string]any{
			"EgressConfiguration": map[string]any{"EgressType": "VPC", "VpcConnectorArn": vcArn},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var svcResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &svcResp))
	svcArn := svcResp["Service"].(map[string]any)["ServiceArn"].(string)

	rec = doRequest(t, h, "DeleteVpcConnector", map[string]any{"VpcConnectorArn": vcArn})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "connector still referenced by vpc-svc must not be deletable")

	rec = doRequest(t, h, "DeleteService", map[string]any{"ServiceArn": svcArn})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DeleteVpcConnector", map[string]any{"VpcConnectorArn": vcArn})
	assert.Equal(t, http.StatusOK, rec.Code, "connector must be deletable once no service references it")
}
