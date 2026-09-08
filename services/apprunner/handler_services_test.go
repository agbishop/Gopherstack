package apprunner_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apprunner"
)

func TestCreateService_ReturnsServiceArn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     any
		check    func(t *testing.T, body []byte)
		name     string
		wantCode int
	}{
		{
			name: "create returns ServiceArn",
			body: map[string]any{
				"ServiceName": "my-service",
				"SourceConfiguration": map[string]any{
					"ImageRepository": map[string]any{
						"ImageIdentifier":     "public.ecr.aws/nginx/nginx:latest",
						"ImageRepositoryType": "ECR_PUBLIC",
					},
				},
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				svc, ok := resp["Service"].(map[string]any)
				require.True(t, ok)
				assert.Contains(t, svc["ServiceArn"], "arn:aws:apprunner:us-east-1:000000000000:service/")
				assert.Equal(t, "RUNNING", svc["Status"])
				assert.NotEmpty(t, svc["ServiceUrl"])
				assert.NotEmpty(t, resp["OperationId"])
			},
		},
		{
			name:     "create missing ServiceName returns 400",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateService", tc.body)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestServiceCRUD(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Create
	svcArn := createTestService(t, h)
	assert.Equal(t, 1, apprunner.ServiceCount(h.Backend.(*apprunner.InMemoryBackend)))

	// Describe
	rec := doRequest(t, h, "DescribeService", map[string]any{"ServiceArn": svcArn})
	assert.Equal(t, http.StatusOK, rec.Code)
	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	svc := descResp["Service"].(map[string]any)
	assert.Equal(t, "test-service", svc["ServiceName"])
	assert.Equal(t, "RUNNING", svc["Status"])

	// List
	rec = doRequest(t, h, "ListServices", map[string]any{})
	assert.Equal(t, http.StatusOK, rec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	assert.Len(t, listResp["ServiceSummaryList"], 1)

	// Delete
	rec = doRequest(t, h, "DeleteService", map[string]any{"ServiceArn": svcArn})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, apprunner.ServiceCount(h.Backend.(*apprunner.InMemoryBackend)))

	// Describe deleted returns 400
	rec = doRequest(t, h, "DescribeService", map[string]any{"ServiceArn": svcArn})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestPauseResume(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	svcArn := createTestService(t, h)

	// Pause
	rec := doRequest(t, h, "PauseService", map[string]any{"ServiceArn": svcArn})
	assert.Equal(t, http.StatusOK, rec.Code)
	var pauseResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pauseResp))
	svc := pauseResp["Service"].(map[string]any)
	assert.Equal(t, "PAUSED", svc["Status"])

	// Resume
	rec = doRequest(t, h, "ResumeService", map[string]any{"ServiceArn": svcArn})
	assert.Equal(t, http.StatusOK, rec.Code)
	var resumeResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resumeResp))
	svc = resumeResp["Service"].(map[string]any)
	assert.Equal(t, "RUNNING", svc["Status"])
}

func TestStartDeployment(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	svcArn := createTestService(t, h)

	rec := doRequest(t, h, "StartDeployment", map[string]any{"ServiceArn": svcArn})
	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["OperationId"])
}

func TestUpdateService(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	svcArn := createTestService(t, h)

	rec := doRequest(t, h, "UpdateService", map[string]any{
		"ServiceArn": svcArn,
		"InstanceConfiguration": map[string]any{
			"Cpu":    "2 vCPU",
			"Memory": "4 GB",
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["OperationId"])
}

func TestListServices_Empty(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "ListServices", map[string]any{})
	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	services, ok := resp["ServiceSummaryList"].([]any)
	require.True(t, ok)
	assert.Empty(t, services)
}

func TestPauseService_StateGuard(t *testing.T) { //nolint:paralleltest // existing issue.
	tests := []struct {
		setup    func(t *testing.T, h *apprunner.Handler, arn string)
		name     string
		wantType string
		wantCode int
	}{
		{
			name:     "pause RUNNING service succeeds",
			setup:    func(_ *testing.T, _ *apprunner.Handler, _ string) {},
			wantCode: http.StatusOK,
		},
		{
			name: "pause PAUSED service returns InvalidStateException",
			setup: func(t *testing.T, h *apprunner.Handler, arn string) {
				t.Helper()
				rec := doRequest(t, h, "PauseService", map[string]any{"ServiceArn": arn})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			wantCode: http.StatusBadRequest,
			wantType: "InvalidStateException",
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			arn := createTestService(t, h)
			tc.setup(t, h, arn)

			rec := doRequest(t, h, "PauseService", map[string]any{"ServiceArn": arn})
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.wantType != "" {
				var body map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, tc.wantType, body["__type"])
			}
		})
	}
}

func TestResumeService_StateGuard(t *testing.T) { //nolint:paralleltest // existing issue.
	tests := []struct {
		setup    func(t *testing.T, h *apprunner.Handler, arn string)
		name     string
		wantType string
		wantCode int
	}{
		{
			name: "resume PAUSED service succeeds",
			setup: func(t *testing.T, h *apprunner.Handler, arn string) {
				t.Helper()
				rec := doRequest(t, h, "PauseService", map[string]any{"ServiceArn": arn})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			wantCode: http.StatusOK,
		},
		{
			name:     "resume RUNNING service returns InvalidStateException",
			setup:    func(_ *testing.T, _ *apprunner.Handler, _ string) {},
			wantCode: http.StatusBadRequest,
			wantType: "InvalidStateException",
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			arn := createTestService(t, h)
			tc.setup(t, h, arn)

			rec := doRequest(t, h, "ResumeService", map[string]any{"ServiceArn": arn})
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.wantType != "" {
				var body map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, tc.wantType, body["__type"])
			}
		})
	}
}

func TestUpdateService_StateGuard(t *testing.T) { //nolint:paralleltest // existing issue.
	tests := []struct {
		setup    func(t *testing.T, h *apprunner.Handler, arn string)
		name     string
		wantType string
		wantCode int
	}{
		{
			name:     "update RUNNING service succeeds",
			setup:    func(_ *testing.T, _ *apprunner.Handler, _ string) {},
			wantCode: http.StatusOK,
		},
		{
			name: "update PAUSED service returns InvalidStateException",
			setup: func(t *testing.T, h *apprunner.Handler, arn string) {
				t.Helper()
				rec := doRequest(t, h, "PauseService", map[string]any{"ServiceArn": arn})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			wantCode: http.StatusBadRequest,
			wantType: "InvalidStateException",
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			arn := createTestService(t, h)
			tc.setup(t, h, arn)

			rec := doRequest(t, h, "UpdateService", map[string]any{
				"ServiceArn":            arn,
				"InstanceConfiguration": map[string]any{"Cpu": "2 vCPU", "Memory": "4 GB"},
			})
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.wantType != "" {
				var body map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, tc.wantType, body["__type"])
			}
		})
	}
}

func TestStartDeployment_StateGuard(t *testing.T) { //nolint:paralleltest // existing issue.
	tests := []struct {
		setup    func(t *testing.T, h *apprunner.Handler, arn string)
		name     string
		wantType string
		wantCode int
	}{
		{
			name:     "deploy RUNNING service succeeds",
			setup:    func(_ *testing.T, _ *apprunner.Handler, _ string) {},
			wantCode: http.StatusOK,
		},
		{
			// StartDeployment's own deserializeOpError switch in the
			// vendored SDK types only InternalServiceErrorException,
			// InvalidRequestException, and ResourceNotFoundException --
			// unlike UpdateService/PauseService/ResumeService it has no
			// InvalidStateException case, so a non-running service must
			// report InvalidRequestException instead.
			name: "deploy PAUSED service returns InvalidRequestException",
			setup: func(t *testing.T, h *apprunner.Handler, arn string) {
				t.Helper()
				rec := doRequest(t, h, "PauseService", map[string]any{"ServiceArn": arn})
				require.Equal(t, http.StatusOK, rec.Code)
			},
			wantCode: http.StatusBadRequest,
			wantType: "InvalidRequestException",
		},
	}

	for _, tc := range tests { //nolint:paralleltest // existing issue.
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t)
			arn := createTestService(t, h)
			tc.setup(t, h, arn)

			rec := doRequest(t, h, "StartDeployment", map[string]any{"ServiceArn": arn})
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.wantType != "" {
				var body map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, tc.wantType, body["__type"])
			}
		})
	}
}

// TestServiceOutputInstanceConfiguration verifies DescribeService returns
// InstanceConfiguration with Cpu and Memory populated from backend storage.
func TestServiceOutputInstanceConfiguration(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateService", map[string]any{
		"ServiceName": "ic-svc",
		"InstanceConfiguration": map[string]any{
			"Cpu":    "2 vCPU",
			"Memory": "4 GB",
		},
		"SourceConfiguration": map[string]any{
			"ImageRepository": map[string]any{
				"ImageIdentifier":     "public.ecr.aws/nginx/nginx:latest",
				"ImageRepositoryType": "ECR_PUBLIC",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	svcArn := createResp["Service"].(map[string]any)["ServiceArn"].(string)

	rec = doRequest(t, h, "DescribeService", map[string]any{"ServiceArn": svcArn})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	svc := descResp["Service"].(map[string]any)

	ic, ok := svc["InstanceConfiguration"].(map[string]any)
	require.True(t, ok, "DescribeService must include InstanceConfiguration")
	assert.Equal(t, "2 vCPU", ic["Cpu"])
	assert.Equal(t, "4 GB", ic["Memory"])
}

// TestServiceOutputSourceConfiguration verifies DescribeService returns
// SourceConfiguration with ImageRepository.ImageIdentifier populated.
func TestServiceOutputSourceConfiguration(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateService", map[string]any{
		"ServiceName": "sc-svc",
		"SourceConfiguration": map[string]any{
			"ImageRepository": map[string]any{
				"ImageIdentifier":     "public.ecr.aws/nginx/nginx:latest",
				"ImageRepositoryType": "ECR_PUBLIC",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	svcArn := createResp["Service"].(map[string]any)["ServiceArn"].(string)

	rec = doRequest(t, h, "DescribeService", map[string]any{"ServiceArn": svcArn})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	svc := descResp["Service"].(map[string]any)

	sc, ok := svc["SourceConfiguration"].(map[string]any)
	require.True(t, ok, "DescribeService must include SourceConfiguration")

	ir, ok := sc["ImageRepository"].(map[string]any)
	require.True(t, ok, "SourceConfiguration must include ImageRepository")
	assert.Equal(t, "public.ecr.aws/nginx/nginx:latest", ir["ImageIdentifier"])
}

// TestDefaultInstanceConfigurationPresent verifies that services created
// without explicit InstanceConfiguration still have Cpu and Memory in the
// DescribeService response (defaults applied by backend).
func TestDefaultInstanceConfigurationPresent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateService", map[string]any{
		"ServiceName": "default-ic-svc",
		"SourceConfiguration": map[string]any{
			"ImageRepository": map[string]any{
				"ImageIdentifier":     "public.ecr.aws/nginx/nginx:latest",
				"ImageRepositoryType": "ECR_PUBLIC",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	svcArn := createResp["Service"].(map[string]any)["ServiceArn"].(string)

	rec = doRequest(t, h, "DescribeService", map[string]any{"ServiceArn": svcArn})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	svc := descResp["Service"].(map[string]any)

	ic, ok := svc["InstanceConfiguration"].(map[string]any)
	require.True(t, ok, "DescribeService must include InstanceConfiguration even with defaults")
	assert.NotEmpty(t, ic["Cpu"], "default Cpu must be non-empty")
	assert.NotEmpty(t, ic["Memory"], "default Memory must be non-empty")
}

// TestCreateService_InstanceRoleArn verifies InstanceConfiguration.InstanceRoleArn
// round-trips through CreateService/DescribeService.
func TestCreateService_InstanceRoleArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateService", map[string]any{
		"ServiceName": "role-svc",
		"InstanceConfiguration": map[string]any{
			"InstanceRoleArn": "arn:aws:iam::000000000000:role/my-role",
		},
		"SourceConfiguration": map[string]any{
			"ImageRepository": map[string]any{
				"ImageIdentifier":     "public.ecr.aws/nginx/nginx:latest",
				"ImageRepositoryType": "ECR_PUBLIC",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ic := resp["Service"].(map[string]any)["InstanceConfiguration"].(map[string]any)
	assert.Equal(t, "arn:aws:iam::000000000000:role/my-role", ic["InstanceRoleArn"])
}

// TestCreateService_EncryptionConfiguration verifies KmsKey round-trips and
// EncryptionConfiguration is omitted when no key was provided (App Runner
// only returns it for customer-provided keys).
func TestCreateService_EncryptionConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("kms key round trips", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		rec := doRequest(t, h, "CreateService", map[string]any{
			"ServiceName": "enc-svc",
			"SourceConfiguration": map[string]any{
				"ImageRepository": map[string]any{"ImageIdentifier": "img", "ImageRepositoryType": "ECR_PUBLIC"},
			},
			"EncryptionConfiguration": map[string]any{"KmsKey": "arn:aws:kms:us-east-1:000000000000:key/abc"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		enc, ok := resp["Service"].(map[string]any)["EncryptionConfiguration"].(map[string]any)
		require.True(t, ok, "EncryptionConfiguration must be present when a KmsKey was provided")
		assert.Equal(t, "arn:aws:kms:us-east-1:000000000000:key/abc", enc["KmsKey"])
	})

	t.Run("omitted when not provided", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		svcArn := createTestService(t, h)

		rec := doRequest(t, h, "DescribeService", map[string]any{"ServiceArn": svcArn})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		_, present := resp["Service"].(map[string]any)["EncryptionConfiguration"]
		assert.False(t, present, "EncryptionConfiguration must be absent when no KmsKey was provided")
	})
}

// TestCreateService_AutoScalingConfigurationAssociation verifies that
// CreateService threads AutoScalingConfigurationArn into real association
// state: the referenced configuration's HasAssociatedService flips true,
// ListServicesForAutoScalingConfiguration reflects the service, and
// DeleteService clears the association again.
func TestCreateService_AutoScalingConfigurationAssociation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateAutoScalingConfiguration", map[string]any{
		"AutoScalingConfigurationName": "custom-asg",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var asgResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &asgResp))
	asgArn := asgResp["AutoScalingConfiguration"].(map[string]any)["AutoScalingConfigurationArn"].(string)

	rec = doRequest(t, h, "CreateService", map[string]any{
		"ServiceName": "asg-svc",
		"SourceConfiguration": map[string]any{
			"ImageRepository": map[string]any{"ImageIdentifier": "img", "ImageRepositoryType": "ECR_PUBLIC"},
		},
		"AutoScalingConfigurationArn": asgArn,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	svc := createResp["Service"].(map[string]any)
	svcArn := svc["ServiceArn"].(string)
	summary := svc["AutoScalingConfigurationSummary"].(map[string]any)
	assert.Equal(t, asgArn, summary["AutoScalingConfigurationArn"])
	assert.Equal(t, true, summary["HasAssociatedService"])

	rec = doRequest(t, h, "ListServicesForAutoScalingConfiguration", map[string]any{
		"AutoScalingConfigurationArn": asgArn,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	arns := listResp["ServiceArnList"].([]any)
	require.Len(t, arns, 1)
	assert.Equal(t, svcArn, arns[0])

	rec = doRequest(t, h, "DeleteService", map[string]any{"ServiceArn": svcArn})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DescribeAutoScalingConfiguration", map[string]any{"AutoScalingConfigurationArn": asgArn})
	require.Equal(t, http.StatusOK, rec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &descResp))
	cfg := descResp["AutoScalingConfiguration"].(map[string]any)
	assert.Equal(t, false, cfg["HasAssociatedService"])

	rec = doRequest(t, h, "ListServicesForAutoScalingConfiguration", map[string]any{
		"AutoScalingConfigurationArn": asgArn,
	})
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	assert.Empty(t, listResp["ServiceArnList"])
}

// TestCreateService_DefaultAutoScalingConfiguration verifies that omitting
// AutoScalingConfigurationArn associates the account's always-present
// default configuration, matching CreateServiceInput's documented behavior.
func TestCreateService_DefaultAutoScalingConfiguration(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	svcArn := createTestService(t, h)

	rec := doRequest(t, h, "DescribeService", map[string]any{"ServiceArn": svcArn})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	summary := resp["Service"].(map[string]any)["AutoScalingConfigurationSummary"].(map[string]any)
	assert.Equal(t, "DefaultConfiguration", summary["AutoScalingConfigurationName"])
	assert.Equal(t, true, summary["IsDefault"])
}

// TestCreateService_UnknownAutoScalingConfigurationArn verifies an
// unresolvable AutoScalingConfigurationArn is rejected as
// InvalidRequestException -- CreateService's documented error set has no
// ResourceNotFoundException.
func TestCreateService_UnknownAutoScalingConfigurationArn(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateService", map[string]any{
		"ServiceName": "bad-asg-svc",
		"SourceConfiguration": map[string]any{
			"ImageRepository": map[string]any{"ImageIdentifier": "img", "ImageRepositoryType": "ECR_PUBLIC"},
		},
		"AutoScalingConfigurationArn": "arn:aws:apprunner:us-east-1:000000000000:autoscalingconfiguration/notexist/1/abc",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "InvalidRequestException", body["__type"])
}

// TestCreateService_SourceConfigurationValidation verifies SourceConfiguration
// must specify exactly one of ImageRepository/CodeRepository, matching
// types.SourceConfiguration's documented "either...but not both" contract.
func TestCreateService_SourceConfigurationValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		source map[string]any
		name   string
	}{
		{name: "missing SourceConfiguration entirely"},
		{
			name: "both ImageRepository and CodeRepository set",
			source: map[string]any{
				"ImageRepository": map[string]any{"ImageIdentifier": "img", "ImageRepositoryType": "ECR_PUBLIC"},
				"CodeRepository": map[string]any{
					"RepositoryUrl":     "https://github.com/example/repo",
					"SourceCodeVersion": map[string]any{"Type": "BRANCH", "Value": "main"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := map[string]any{"ServiceName": "svc"}
			if tc.source != nil {
				body["SourceConfiguration"] = tc.source
			}

			rec := doRequest(t, h, "CreateService", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// TestCreateService_NetworkConfiguration verifies NetworkConfiguration
// defaults (DEFAULT egress, publicly accessible, IPv4) apply when omitted,
// a VPC egress configuration round-trips when it references a real VPC
// connector, and an unresolvable VpcConnectorArn is rejected.
func TestCreateService_NetworkConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("defaults when omitted", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		svcArn := createTestService(t, h)

		rec := doRequest(t, h, "DescribeService", map[string]any{"ServiceArn": svcArn})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		nc := resp["Service"].(map[string]any)["NetworkConfiguration"].(map[string]any)
		assert.Equal(t, "IPV4", nc["IpAddressType"])
		assert.Equal(t, "DEFAULT", nc["EgressConfiguration"].(map[string]any)["EgressType"])
		assert.Equal(t, true, nc["IngressConfiguration"].(map[string]any)["IsPubliclyAccessible"])
	})

	t.Run("VPC egress with valid connector round trips", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := doRequest(t, h, "CreateVpcConnector", map[string]any{
			"VpcConnectorName": "vc1",
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
				"EgressConfiguration":  map[string]any{"EgressType": "VPC", "VpcConnectorArn": vcArn},
				"IngressConfiguration": map[string]any{"IsPubliclyAccessible": false},
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var createResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
		nc := createResp["Service"].(map[string]any)["NetworkConfiguration"].(map[string]any)
		assert.Equal(t, "VPC", nc["EgressConfiguration"].(map[string]any)["EgressType"])
		assert.Equal(t, vcArn, nc["EgressConfiguration"].(map[string]any)["VpcConnectorArn"])
		assert.Equal(t, false, nc["IngressConfiguration"].(map[string]any)["IsPubliclyAccessible"])
	})

	t.Run("VPC egress with unknown connector returns 400", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := doRequest(t, h, "CreateService", map[string]any{
			"ServiceName": "bad-vpc-svc",
			"SourceConfiguration": map[string]any{
				"ImageRepository": map[string]any{"ImageIdentifier": "img", "ImageRepositoryType": "ECR_PUBLIC"},
			},
			"NetworkConfiguration": map[string]any{
				"EgressConfiguration": map[string]any{
					"EgressType":      "VPC",
					"VpcConnectorArn": "arn:aws:apprunner:us-east-1:000000000000:vpcconnector/notexist/1/abc",
				},
			},
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// TestCreateService_HealthCheckConfiguration verifies App Runner's documented
// defaults apply when HealthCheckConfiguration is omitted, and that a custom
// configuration round-trips.
func TestCreateService_HealthCheckConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("defaults when omitted", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		svcArn := createTestService(t, h)

		rec := doRequest(t, h, "DescribeService", map[string]any{"ServiceArn": svcArn})
		require.Equal(t, http.StatusOK, rec.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		hc := resp["Service"].(map[string]any)["HealthCheckConfiguration"].(map[string]any)
		assert.Equal(t, "TCP", hc["Protocol"])
		assert.Equal(t, "/", hc["Path"])
		assert.InDelta(t, float64(5), hc["Interval"], 0.0001)
		assert.InDelta(t, float64(2), hc["Timeout"], 0.0001)
		assert.InDelta(t, float64(1), hc["HealthyThreshold"], 0.0001)
		assert.InDelta(t, float64(5), hc["UnhealthyThreshold"], 0.0001)
	})

	t.Run("custom values round trip", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := doRequest(t, h, "CreateService", map[string]any{
			"ServiceName": "hc-svc",
			"SourceConfiguration": map[string]any{
				"ImageRepository": map[string]any{"ImageIdentifier": "img", "ImageRepositoryType": "ECR_PUBLIC"},
			},
			"HealthCheckConfiguration": map[string]any{
				"Protocol":           "HTTP",
				"Path":               "/healthz",
				"Interval":           10,
				"Timeout":            5,
				"HealthyThreshold":   3,
				"UnhealthyThreshold": 3,
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var createResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
		hc := createResp["Service"].(map[string]any)["HealthCheckConfiguration"].(map[string]any)
		assert.Equal(t, "HTTP", hc["Protocol"])
		assert.Equal(t, "/healthz", hc["Path"])
		assert.InDelta(t, float64(10), hc["Interval"], 0.0001)
	})
}

// TestCreateService_ObservabilityConfiguration verifies
// ServiceObservabilityConfiguration round-trips when it references a real
// observability configuration, and an unresolvable ARN is rejected.
func TestCreateService_ObservabilityConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("enabled with valid arn round trips", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := doRequest(t, h, "CreateObservabilityConfiguration", map[string]any{
			"ObservabilityConfigurationName": "obs1",
			"TraceConfiguration":             map[string]any{"Vendor": "AWSXRAY"},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var obsResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &obsResp))
		obsArn := obsResp["ObservabilityConfiguration"].(map[string]any)["ObservabilityConfigurationArn"].(string)

		rec = doRequest(t, h, "CreateService", map[string]any{
			"ServiceName": "obs-svc",
			"SourceConfiguration": map[string]any{
				"ImageRepository": map[string]any{"ImageIdentifier": "img", "ImageRepositoryType": "ECR_PUBLIC"},
			},
			"ObservabilityConfiguration": map[string]any{
				"ObservabilityEnabled":          true,
				"ObservabilityConfigurationArn": obsArn,
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var createResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
		oc := createResp["Service"].(map[string]any)["ObservabilityConfiguration"].(map[string]any)
		assert.Equal(t, true, oc["ObservabilityEnabled"])
		assert.Equal(t, obsArn, oc["ObservabilityConfigurationArn"])
	})

	t.Run("enabled with unknown arn returns 400", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := doRequest(t, h, "CreateService", map[string]any{
			"ServiceName": "bad-obs-svc",
			"SourceConfiguration": map[string]any{
				"ImageRepository": map[string]any{"ImageIdentifier": "img", "ImageRepositoryType": "ECR_PUBLIC"},
			},
			"ObservabilityConfiguration": map[string]any{
				"ObservabilityEnabled": true,
				"ObservabilityConfigurationArn": "arn:aws:apprunner:us-east-1:000000000000:" +
					"observabilityconfiguration/notexist/1/abc",
			},
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// TestCreateService_CodeRepository verifies SourceConfiguration.CodeRepository
// (RepositoryUrl, SourceCodeVersion, CodeConfiguration, and
// AuthenticationConfiguration.ConnectionArn) round-trips, and that an
// AuthenticationConfiguration.ConnectionArn referencing an unknown connection
// is rejected.
func TestCreateService_CodeRepository(t *testing.T) {
	t.Parallel()

	t.Run("round trips with a real connection", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := doRequest(t, h, "CreateConnection", map[string]any{
			"ConnectionName": "gh-conn",
			"ProviderType":   "GITHUB",
		})
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
					"CodeConfiguration": map[string]any{
						"ConfigurationSource": "API",
						"CodeConfigurationValues": map[string]any{
							"Runtime":      "PYTHON_3",
							"StartCommand": "python app.py",
							"Port":         "8000",
						},
					},
				},
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		var createResp map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
		sc := createResp["Service"].(map[string]any)["SourceConfiguration"].(map[string]any)
		cr := sc["CodeRepository"].(map[string]any)
		assert.Equal(t, "https://github.com/example/repo", cr["RepositoryUrl"])
		assert.Equal(t, "main", cr["SourceCodeVersion"].(map[string]any)["Value"])
		values := cr["CodeConfiguration"].(map[string]any)["CodeConfigurationValues"].(map[string]any)
		assert.Equal(t, "PYTHON_3", values["Runtime"])
		auth := sc["AuthenticationConfiguration"].(map[string]any)
		assert.Equal(t, connArn, auth["ConnectionArn"])
	})

	t.Run("unknown connection arn returns 400", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)

		rec := doRequest(t, h, "CreateService", map[string]any{
			"ServiceName": "bad-conn-svc",
			"SourceConfiguration": map[string]any{
				"AuthenticationConfiguration": map[string]any{
					"ConnectionArn": "arn:aws:apprunner:us-east-1:000000000000:connection/notexist/abc",
				},
				"CodeRepository": map[string]any{
					"RepositoryUrl":     "https://github.com/example/repo",
					"SourceCodeVersion": map[string]any{"Type": "BRANCH", "Value": "main"},
				},
			},
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// TestUpdateService_CannotSwitchSourceType verifies App Runner's documented
// restriction that a service can't switch between an image and a code
// source ("you must provide the same structure member... that you
// originally included when you created the service").
func TestUpdateService_CannotSwitchSourceType(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	svcArn := createTestService(t, h) // image-based, see createTestService.

	rec := doRequest(t, h, "UpdateService", map[string]any{
		"ServiceArn": svcArn,
		"SourceConfiguration": map[string]any{
			"CodeRepository": map[string]any{
				"RepositoryUrl":     "https://github.com/example/repo",
				"SourceCodeVersion": map[string]any{"Type": "BRANCH", "Value": "main"},
			},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "InvalidRequestException", body["__type"])
}

// TestListOperations_UpdatedAtPresent verifies OperationSummary.UpdatedAt is
// populated (the real API field this backend previously omitted).
func TestListOperations_UpdatedAtPresent(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	svcArn := createTestService(t, h)

	rec := doRequest(t, h, "ListOperations", map[string]any{"ServiceArn": svcArn})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ops := resp["OperationSummaryList"].([]any)
	require.NotEmpty(t, ops)

	op := ops[0].(map[string]any)
	assert.NotEmpty(t, op["UpdatedAt"])
	assert.InDelta(t, op["StartedAt"], op["UpdatedAt"], 1)
}

// TestDeleteService_RejectsWhenActiveVpcIngressConnectionExists verifies
// DeleteService fails while a VPC ingress connection still references the
// service (api_op_DeleteService.go: "Make sure that you don't have any
// active VPCIngressConnections associated with the service you want to
// delete."), and succeeds once that VPC ingress connection is deleted.
func TestDeleteService_RejectsWhenActiveVpcIngressConnectionExists(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	svcArn := createTestService(t, h)

	rec := doRequest(t, h, "CreateVpcIngressConnection", map[string]any{
		"VpcIngressConnectionName": "vic1",
		"ServiceArn":               svcArn,
		"IngressVpcConfiguration": map[string]any{
			"VpcId":         "vpc-1",
			"VpcEndpointId": "vpce-1",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var vicResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &vicResp))
	vicArn := vicResp["VpcIngressConnection"].(map[string]any)["VpcIngressConnectionArn"].(string)

	rec = doRequest(t, h, "DeleteService", map[string]any{"ServiceArn": svcArn})
	assert.Equal(
		t, http.StatusBadRequest, rec.Code, "service with an active VPC ingress connection must not be deletable",
	)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "InvalidStateException", body["__type"])

	rec = doRequest(t, h, "DeleteVpcIngressConnection", map[string]any{"VpcIngressConnectionArn": vicArn})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "DeleteService", map[string]any{"ServiceArn": svcArn})
	assert.Equal(t, http.StatusOK, rec.Code, "service must be deletable once its VPC ingress connection is gone")
}
