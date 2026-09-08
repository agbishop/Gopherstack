package apigateway_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	apigatewaysdk "github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigateway"
)

// TestFlushStageCache_NotFound verifies that FlushStageCache returns 404 for unknown
// API or stage. Real AWS returns NotFoundException.
func TestFlushStageCache_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		apiID     string
		stageName string
		setup     bool
		wantCode  int
	}{
		{
			name:      "unknown API returns 404",
			apiID:     "nonexistent",
			stageName: "prod",
			setup:     false,
			wantCode:  http.StatusNotFound,
		},
		{
			name:      "unknown stage returns 404",
			stageName: "nonexistent-stage",
			setup:     true,
			wantCode:  http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newAPIGWHandler()
			apiID := tt.apiID
			if tt.setup {
				apiID = createParityAPI(t, h, "flush-test-api")
			}

			rec := restRequest(t, h, http.MethodDelete,
				fmt.Sprintf("/restapis/%s/stages/%s/cache/data", apiID, tt.stageName), "")

			assert.Equal(t, tt.wantCode, rec.Code,
				"FlushStageCache on non-existent resource must return 404; body: %s", rec.Body.String())
		})
	}
}

// TestFlushStageCache_ReachableViaRealClient is a regression test for
// gopherstack-0bq8: the router matched DELETE
// /restapis/{id}/stages/{name}/cache (5 path segments), but the real SDK
// sends DELETE /restapis/{id}/stages/{name}/cache/data (6 segments,
// apigateway@v1.42.4 serializers.go:
// awsRestjson1_serializeOpFlushStageCache's opPath). A real client's
// FlushStageCache call never matched any router case and fell through to
// the "Unknown" 404 sentinel. Drives the real aws-sdk-go-v2 client end to
// end and asserts the call succeeds instead of 404ing.
func TestFlushStageCache_ReachableViaRealClient(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()
	client := newTestAPIGatewayClient(t, h)

	api, err := client.CreateRestApi(t.Context(), &apigatewaysdk.CreateRestApiInput{
		Name: aws.String("flush-cache-real-client-api"),
	})
	require.NoError(t, err)

	depl, err := client.CreateDeployment(t.Context(), &apigatewaysdk.CreateDeploymentInput{
		RestApiId: api.Id,
	})
	require.NoError(t, err)

	_, err = client.CreateStage(t.Context(), &apigatewaysdk.CreateStageInput{
		RestApiId:    api.Id,
		DeploymentId: depl.Id,
		StageName:    aws.String("prod"),
	})
	require.NoError(t, err)

	_, err = client.FlushStageCache(t.Context(), &apigatewaysdk.FlushStageCacheInput{
		RestApiId: api.Id,
		StageName: aws.String("prod"),
	})
	require.NoError(t, err, "FlushStageCache must reach its own handler, not fall through to Unknown/404")
}

func TestStage_ClientCertificateId_Create(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "cert-api"})
	depl, _ := b.CreateDeployment(api.ID, "", "v1")

	cert, err := b.GenerateClientCertificate(apigateway.GenerateClientCertificateInput{
		Description: "test cert",
	})
	require.NoError(t, err)

	_, err = b.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
	})
	require.NoError(t, err)

	// Real CreateStageInput has no ClientCertificateId member (aws-sdk-go-v2
	// apigateway@v1.42.4 api_op_CreateStage.go) -- it's only settable via
	// UpdateStage's PATCH after creation.
	stage, err := b.UpdateStage(api.ID, "prod", apigateway.UpdateStageInput{
		ClientCertificateID: cert.ClientCertificateID,
	})
	require.NoError(t, err)
	assert.Equal(t, cert.ClientCertificateID, stage.ClientCertificateID)

	got, err := b.GetStage(api.ID, "prod")
	require.NoError(t, err)
	assert.Equal(t, cert.ClientCertificateID, got.ClientCertificateID)
}

func TestStage_ClientCertificateId_Update(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "cert-upd-api"})
	depl, _ := b.CreateDeployment(api.ID, "", "v1")

	stage, err := b.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "dev",
		DeploymentID: depl.ID,
	})
	require.NoError(t, err)
	assert.Empty(t, stage.ClientCertificateID)

	cert, _ := b.GenerateClientCertificate(apigateway.GenerateClientCertificateInput{})
	updated, err := b.UpdateStage(api.ID, "dev", apigateway.UpdateStageInput{
		ClientCertificateID: cert.ClientCertificateID,
	})
	require.NoError(t, err)
	assert.Equal(t, cert.ClientCertificateID, updated.ClientCertificateID)
}

func TestHandlerStage_ClientCertificateId(t *testing.T) {
	t.Parallel()

	h := newAPIGWHandler()

	rec := restRequest(t, h, http.MethodPost, "/clientcertificates", `{"description":"test cert"}`)
	require.True(t, rec.Code >= 200 && rec.Code < 300)

	var certResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&certResp))
	certID := certResp["clientCertificateId"].(string)
	require.NotEmpty(t, certID)

	rec = restRequest(t, h, http.MethodPost, "/restapis", `{"name":"cert-stage-api"}`)
	require.True(t, rec.Code >= 200 && rec.Code < 300)

	var apiResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&apiResp))
	apiID := apiResp["id"].(string)

	rec = restRequest(t, h, http.MethodPost, "/restapis/"+apiID+"/deployments",
		`{"stageName":"prod","description":"v1"}`)
	require.True(t, rec.Code >= 200 && rec.Code < 300)

	rec = restRequest(t, h, http.MethodPatch, "/restapis/"+apiID+"/stages/prod",
		`{"clientCertificateId":"`+certID+`"}`)
	require.True(t, rec.Code >= 200 && rec.Code < 300)

	var stageResp map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&stageResp))
	assert.Equal(t, certID, stageResp["clientCertificateId"])
}

func TestStage_CanarySettings(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "canary-api"})
	// Create deployment without auto-staging to allow explicit CreateStage below.
	depl, _ := b.CreateDeployment(api.ID, "", "v1")

	canary := &apigateway.CanarySettings{
		PercentTraffic: 10.0,
		DeploymentID:   depl.ID,
		UseStageCache:  true,
	}
	stage, err := b.CreateStage(apigateway.CreateStageInput{
		RestAPIID:      api.ID,
		StageName:      "prod",
		DeploymentID:   depl.ID,
		CanarySettings: canary,
	})
	require.NoError(t, err)
	require.NotNil(t, stage.CanarySettings)
	assert.InDelta(t, 10.0, stage.CanarySettings.PercentTraffic, 0.001)
}

func TestStage_AccessLogSettings(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "log-api"})
	depl, _ := b.CreateDeployment(api.ID, "", "v1")

	_, err := b.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
	})
	require.NoError(t, err)

	// Real CreateStageInput has no AccessLogSettings member (aws-sdk-go-v2
	// apigateway@v1.42.4 api_op_CreateStage.go) -- it's only settable via
	// UpdateStage's PATCH after creation.
	stage, err := b.UpdateStage(api.ID, "prod", apigateway.UpdateStageInput{
		AccessLogSettings: &apigateway.AccessLogSettings{
			DestinationARN: "arn:aws:logs:us-east-1:123456789012:log-group:my-api",
			Format:         "$context.requestId",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, stage.AccessLogSettings)
	assert.Equal(t, "arn:aws:logs:us-east-1:123456789012:log-group:my-api", stage.AccessLogSettings.DestinationARN)
}

func TestStage_TracingEnabled(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "trace-api"})
	depl, _ := b.CreateDeployment(api.ID, "", "v1")

	stage, err := b.CreateStage(apigateway.CreateStageInput{
		RestAPIID:      api.ID,
		StageName:      "prod",
		DeploymentID:   depl.ID,
		TracingEnabled: true,
	})
	require.NoError(t, err)
	assert.True(t, stage.TracingEnabled)

	updated, err := b.UpdateStage(api.ID, "prod", apigateway.UpdateStageInput{
		TracingEnabled: new(bool),
	})
	require.NoError(t, err)
	assert.False(t, updated.TracingEnabled)
}

func TestStage_MethodSettings(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "ms-api"})
	depl, _ := b.CreateDeployment(api.ID, "", "v1")

	_, err := b.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: depl.ID,
	})
	require.NoError(t, err)

	settings := map[string]apigateway.MethodSetting{
		"GET /items": {
			LoggingLevel:     "INFO",
			MetricsEnabled:   true,
			DataTraceEnabled: false,
		},
	}

	// Real CreateStageInput has no MethodSettings member (aws-sdk-go-v2
	// apigateway@v1.42.4 api_op_CreateStage.go) -- it's only settable via
	// UpdateStage's PATCH after creation.
	stage, err := b.UpdateStage(api.ID, "prod", apigateway.UpdateStageInput{
		MethodSettings: settings,
	})
	require.NoError(t, err)
	require.Contains(t, stage.MethodSettings, "GET /items")
	assert.Equal(t, "INFO", stage.MethodSettings["GET /items"].LoggingLevel)
}

// TestUpdateStage tests UpdateStage.
func TestUpdateStage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		newDesc  string
		wantCode int
		useValid bool
	}{
		{
			name:     "update_description",
			newDesc:  "updated stage",
			wantCode: http.StatusOK,
			useValid: true,
		},
		{
			name:     "stage_not_found",
			wantCode: http.StatusNotFound,
			useValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, e := boostSetup()
			apiID := boostAPI(t, handler, e)
			boostDeployment(t, handler, e, apiID, "prod")

			lookupStage := "prod"
			if !tt.useValid {
				lookupStage = "notexist"
			}

			rec := postWithHandler(t, handler, e, "UpdateStage",
				fmt.Sprintf(`{"restApiId":%q,"stageName":%q,"description":%q}`,
					apiID, lookupStage, tt.newDesc))
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestBackend_DeleteStage_NotFound covers the "stage not found" error branch in DeleteStage.
func TestBackend_DeleteStage_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "stage_not_found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := apigateway.NewInMemoryBackend()
			api, err := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "api"})
			require.NoError(t, err)

			err = b.DeleteStage(api.ID, "nonexistent-stage")
			require.Error(t, err)
		})
	}
}

// TestHandler_RESTPath_Stages exercises the GET stages REST-path branches in
// parseAPIGWRESTPath that are not covered by existing tests.
func TestHandler_RESTPath_Stages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(b *apigateway.InMemoryBackend) string
		name     string
		method   string
		wantCode int
	}{
		{
			name:   "GET_stages_lists",
			method: http.MethodGet,
			setup: func(b *apigateway.InMemoryBackend) string {
				api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "api"})
				_, _ = b.CreateDeployment(api.ID, "staging", "")

				return fmt.Sprintf("/restapis/%s/stages", api.ID)
			},
			wantCode: http.StatusOK,
		},
		{
			name:   "GET_stage_by_name",
			method: http.MethodGet,
			setup: func(b *apigateway.InMemoryBackend) string {
				api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "api"})
				_, _ = b.CreateDeployment(api.ID, "prod", "")

				return fmt.Sprintf("/restapis/%s/stages/prod", api.ID)
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := apigateway.NewInMemoryBackend()
			h := apigateway.NewHandler(backend)

			path := tt.setup(backend)
			rec := restRequest(t, h, tt.method, path, "")
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestGetStages_SortWithMultipleItems ensures the sort closure in GetStages is exercised
// by creating two deployments with different stage names and then listing stages.
func TestGetStages_SortWithMultipleItems(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stageNames []string
	}{
		{
			name:       "two_stages_triggers_sort",
			stageNames: []string{"prod", "staging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := apigateway.NewInMemoryBackend()
			api, err := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "api"})
			require.NoError(t, err)

			for _, s := range tt.stageNames {
				_, err = b.CreateDeployment(api.ID, s, "")
				require.NoError(t, err)
			}

			stages, err := b.GetStages(api.ID)
			require.NoError(t, err)
			assert.Len(t, stages, len(tt.stageNames))
		})
	}
}

func TestBackend_Stage_ClientCertificateId(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, _ := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "cert-stage-api"})

	cert, err := b.GenerateClientCertificate(apigateway.GenerateClientCertificateInput{
		Description: "test cert",
	})
	require.NoError(t, err)

	depl, _ := b.CreateDeployment(api.ID, "", "v1")

	// Real CreateStageInput has no ClientCertificateId member (aws-sdk-go-v2
	// apigateway@v1.42.4 api_op_CreateStage.go) -- it's only settable via
	// UpdateStage's PATCH after creation.
	tests := []struct {
		check    func(t *testing.T, stage *apigateway.Stage)
		name     string
		stage    string
		withCert bool
	}{
		{
			name:     "with_cert",
			stage:    "prod",
			withCert: true,
			check: func(t *testing.T, stage *apigateway.Stage) {
				t.Helper()
				assert.Equal(t, cert.ClientCertificateID, stage.ClientCertificateID)
			},
		},
		{
			name:  "without_cert",
			stage: "dev",
			check: func(t *testing.T, stage *apigateway.Stage) {
				t.Helper()
				assert.Empty(t, stage.ClientCertificateID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stage, createErr := b.CreateStage(apigateway.CreateStageInput{
				RestAPIID:    api.ID,
				StageName:    tt.stage,
				DeploymentID: depl.ID,
			})
			require.NoError(t, createErr)

			if tt.withCert {
				stage, createErr = b.UpdateStage(api.ID, tt.stage, apigateway.UpdateStageInput{
					ClientCertificateID: cert.ClientCertificateID,
				})
				require.NoError(t, createErr)
			}

			tt.check(t, stage)

			got, getErr := b.GetStage(api.ID, tt.stage)
			require.NoError(t, getErr)
			tt.check(t, got)
		})
	}
}

// TestStage_DocumentationVersion exercises CreateStage/UpdateStage's
// DocumentationVersion field (types.Stage.DocumentationVersion in the SDK),
// absent from gopherstack's Stage struct until this sweep.
func TestStage_DocumentationVersion(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, err := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "docversion-api"})
	require.NoError(t, err)
	depl, err := b.CreateDeployment(api.ID, "", "v1")
	require.NoError(t, err)

	stage, err := b.CreateStage(apigateway.CreateStageInput{
		RestAPIID:            api.ID,
		StageName:            "prod",
		DeploymentID:         depl.ID,
		DocumentationVersion: "1.0.0",
	})
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", stage.DocumentationVersion)

	updated, err := b.UpdateStage(api.ID, "prod", apigateway.UpdateStageInput{
		DocumentationVersion: "2.0.0",
	})
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", updated.DocumentationVersion)
}

// TestCreateStage_RejectsNonexistentDeploymentID pins the existing guard
// (stages.go) that CreateStage rejects a deploymentId with no matching
// Deployment, matching real AWS's NotFoundException for a bogus deploymentId.
func TestCreateStage_RejectsNonexistentDeploymentID(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, err := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "stage-bogus-deploy-api"})
	require.NoError(t, err)

	_, err = b.CreateStage(apigateway.CreateStageInput{
		RestAPIID:    api.ID,
		StageName:    "prod",
		DeploymentID: "does-not-exist",
	})
	require.Error(t, err, "CreateStage must reject a deploymentId that names no real deployment")
	require.ErrorIs(t, err, apigateway.ErrDeploymentNotFound)
}

// TestUpdateStage_RejectsNonexistentDeploymentID verifies that UpdateStage
// rejects repointing a stage's deploymentId at a Deployment that doesn't
// exist, mirroring CreateStage's existing guard.
func TestUpdateStage_RejectsNonexistentDeploymentID(t *testing.T) {
	t.Parallel()

	b := apigateway.NewInMemoryBackend()
	api, err := b.CreateRestAPI(apigateway.CreateRestAPIInput{Name: "update-stage-bogus-deploy-api"})
	require.NoError(t, err)
	_, err = b.CreateDeployment(api.ID, "prod", "v1")
	require.NoError(t, err)

	_, err = b.UpdateStage(api.ID, "prod", apigateway.UpdateStageInput{
		DeploymentID: "does-not-exist",
	})
	require.Error(t, err, "UpdateStage must reject repointing deploymentId at a nonexistent deployment")
	require.ErrorIs(t, err, apigateway.ErrDeploymentNotFound)

	// A real deployment ID must still be accepted.
	depl2, err := b.CreateDeployment(api.ID, "", "v2")
	require.NoError(t, err)
	updated, err := b.UpdateStage(api.ID, "prod", apigateway.UpdateStageInput{
		DeploymentID: depl2.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, depl2.ID, updated.DeploymentID)
}
