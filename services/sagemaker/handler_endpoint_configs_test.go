package sagemaker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	sagemakersdk "github.com/aws/aws-sdk-go-v2/service/sagemaker"
	smtypes "github.com/aws/aws-sdk-go-v2/service/sagemaker/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sagemaker"
)

func TestHandler_CreateEndpointConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
		wantARN  bool
	}{
		{
			name: "success",
			body: map[string]any{
				"EndpointConfigName": "my-config",
				"ProductionVariants": []map[string]any{
					{
						"VariantName":          "AllTraffic",
						"ModelName":            "my-model",
						"InstanceType":         "ml.t2.medium",
						"InitialInstanceCount": 1,
					},
				},
			},
			wantCode: http.StatusOK,
			wantARN:  true,
		},
		{
			name:     "missing config name",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doSageMakerRequest(t, h, "CreateEndpointConfig", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantARN {
				var resp map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Contains(t, resp["EndpointConfigArn"], "arn:aws:sagemaker")
			}
		})
	}
}

func TestHandler_DescribeEndpointConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*testing.T, *sagemaker.Handler)
		body     map[string]any
		name     string
		wantName string
		wantCode int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *sagemaker.Handler) {
				t.Helper()

				_, err := h.Backend.CreateEndpointConfig(context.Background(), "my-config", nil, nil)
				require.NoError(t, err)
			},
			body:     map[string]any{"EndpointConfigName": "my-config"},
			wantCode: http.StatusOK,
			wantName: "my-config",
		},
		{
			name:     "not found",
			body:     map[string]any{"EndpointConfigName": "nonexistent"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doSageMakerRequest(t, h, "DescribeEndpointConfig", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantName != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantName, resp["EndpointConfigName"])
			}
		})
	}
}

func TestHandler_DeleteEndpointConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*testing.T, *sagemaker.Handler)
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "success",
			setup: func(t *testing.T, h *sagemaker.Handler) {
				t.Helper()

				_, err := h.Backend.CreateEndpointConfig(context.Background(), "to-delete", nil, nil)
				require.NoError(t, err)
			},
			body:     map[string]any{"EndpointConfigName": "to-delete"},
			wantCode: http.StatusOK,
		},
		{
			name:     "not found",
			body:     map[string]any{"EndpointConfigName": "nonexistent"},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "in use by endpoint",
			setup: func(t *testing.T, h *sagemaker.Handler) {
				t.Helper()

				_, err := h.Backend.CreateEndpointConfig(context.Background(), "in-use", nil, nil)
				require.NoError(t, err)
				_, err = h.Backend.CreateEndpoint(context.Background(), sagemaker.CreateEndpointOptions{
					Name:               "using-endpoint",
					EndpointConfigName: "in-use",
				})
				require.NoError(t, err)
			},
			body:     map[string]any{"EndpointConfigName": "in-use"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doSageMakerRequest(t, h, "DeleteEndpointConfig", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestHandler_DeleteEndpointConfig_AfterEndpointDeleted asserts that once the
// referencing endpoint is gone, DeleteEndpointConfig succeeds
// (api_op_DeleteEndpointConfig.go: "You must not delete an EndpointConfig in
// use by an endpoint that is live").
func TestHandler_DeleteEndpointConfig_AfterEndpointDeleted(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	_, err := h.Backend.CreateEndpointConfig(context.Background(), "ec-freed", nil, nil)
	require.NoError(t, err)
	_, err = h.Backend.CreateEndpoint(context.Background(), sagemaker.CreateEndpointOptions{
		Name:               "ep-freed",
		EndpointConfigName: "ec-freed",
	})
	require.NoError(t, err)

	require.NoError(t, h.Backend.DeleteEndpoint(context.Background(), "ep-freed"))

	rec := doSageMakerRequest(t, h, "DeleteEndpointConfig", map[string]any{"EndpointConfigName": "ec-freed"})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_ListEndpointConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*testing.T, *sagemaker.Handler)
		name       string
		wantCode   int
		wantLength int
	}{
		{
			name:       "empty list",
			wantCode:   http.StatusOK,
			wantLength: 0,
		},
		{
			name: "returns all configs",
			setup: func(t *testing.T, h *sagemaker.Handler) {
				t.Helper()

				_, err := h.Backend.CreateEndpointConfig(context.Background(), "config-a", nil, nil)
				require.NoError(t, err)

				_, err = h.Backend.CreateEndpointConfig(context.Background(), "config-b", nil, nil)
				require.NoError(t, err)
			},
			wantCode:   http.StatusOK,
			wantLength: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := doSageMakerRequest(t, h, "ListEndpointConfigs", map[string]any{})
			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			configs, ok := resp["EndpointConfigs"].([]any)
			require.True(t, ok)
			assert.Len(t, configs, tt.wantLength)
		})
	}
}

func TestHandler_Tags_EndpointConfig(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	ec, err := h.Backend.CreateEndpointConfig(context.Background(), "tagged-config", nil, nil)
	require.NoError(t, err)

	// Add tags to endpoint config.
	rec := doSageMakerRequest(t, h, "AddTags", map[string]any{
		"ResourceArn": ec.EndpointConfigARN,
		"Tags":        []map[string]string{{"Key": "Env", "Value": "test"}},
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	// List tags for endpoint config.
	rec = doSageMakerRequest(t, h, "ListTags", map[string]any{
		"ResourceArn": ec.EndpointConfigARN,
	})
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	tags, ok := resp["Tags"].([]any)
	require.True(t, ok)
	require.Len(t, tags, 1)

	// Delete tags from endpoint config.
	rec = doSageMakerRequest(t, h, "DeleteTags", map[string]any{
		"ResourceArn": ec.EndpointConfigARN,
		"TagKeys":     []string{"Env"},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_ListEndpointConfigsPagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		count         int
		wantNextToken bool
	}{
		{
			name:          "single_page",
			count:         3,
			wantNextToken: false,
		},
		{
			name:          "multi_page",
			count:         105, // exceeds sagemakerDefaultPageSize=100
			wantNextToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			for i := range tt.count {
				_, err := h.Backend.CreateEndpointConfig(context.Background(),
					fmt.Sprintf("cfg-%04d", i),
					nil,
					nil,
				)
				require.NoError(t, err)
			}

			rec := doSageMakerRequest(t, h, "ListEndpointConfigs", map[string]any{})
			assert.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			configs, configsOK := resp["EndpointConfigs"].([]any)
			require.True(t, configsOK)

			if tt.wantNextToken {
				assert.Len(t, configs, 100)
				nextToken, tokenOK := resp["NextToken"].(string)
				require.True(t, tokenOK, "NextToken should be present")
				assert.NotEmpty(t, nextToken)

				// Second page.
				rec2 := doSageMakerRequest(
					t,
					h,
					"ListEndpointConfigs",
					map[string]any{"NextToken": nextToken},
				)
				assert.Equal(t, http.StatusOK, rec2.Code)

				var resp2 map[string]any
				require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))

				configs2, configs2OK := resp2["EndpointConfigs"].([]any)
				require.True(t, configs2OK)
				assert.Len(t, configs2, tt.count-100)
				assert.Empty(t, resp2["NextToken"])
			} else {
				assert.Len(t, configs, tt.count)
				assert.Empty(t, resp["NextToken"])
			}
		})
	}
}

// TestCreateEndpointConfig_RequiresProductionVariants verifies that
// CreateEndpointConfig rejects requests with an empty ProductionVariants list.
// Real AWS returns ValidationException for this case; the emulator previously
// accepted empty variants and created a corrupt endpoint config.
func TestCreateEndpointConfig_RequiresProductionVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name: "empty_variants_rejected",
			body: map[string]any{
				"EndpointConfigName": "bad-config",
				"ProductionVariants": []map[string]any{},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "null_variants_rejected",
			body: map[string]any{
				"EndpointConfigName": "null-config",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "single_variant_accepted",
			body: map[string]any{
				"EndpointConfigName": "good-config",
				"ProductionVariants": []map[string]any{
					{
						"VariantName":          "AllTraffic",
						"ModelName":            "my-model",
						"InstanceType":         "ml.t2.medium",
						"InitialInstanceCount": 1,
					},
				},
			},
			wantCode: http.StatusOK,
		},
		{
			name: "multiple_variants_accepted",
			body: map[string]any{
				"EndpointConfigName": "multi-config",
				"ProductionVariants": []map[string]any{
					{
						"VariantName":          "Variant1",
						"ModelName":            "model-a",
						"InstanceType":         "ml.t2.medium",
						"InitialInstanceCount": 1,
					},
					{
						"VariantName":          "Variant2",
						"ModelName":            "model-b",
						"InstanceType":         "ml.t2.large",
						"InitialInstanceCount": 2,
					},
				},
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doSageMakerRequest(t, h, "CreateEndpointConfig", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code, "body=%v", tt.body)

			if tt.wantCode == http.StatusBadRequest {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
				msg, _ := errResp["message"].(string)
				assert.Contains(t, msg, "ProductionVariant",
					"error message must mention ProductionVariant")
			}
		})
	}
}

// TestHandler_CreateEndpointConfig_ExplainerAndMetricsConfig_RealClient
// proves CreateEndpointConfigInput.ExplainerConfig/MetricsConfig (both real,
// optional fields on api_op_CreateEndpointConfig.go previously entirely
// absent from decode) survive the real aws-sdk-go-v2 client's own
// serializer, round-tripping through DescribeEndpointConfig.
func TestHandler_CreateEndpointConfig_ExplainerAndMetricsConfig_RealClient(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	client := newTestSageMakerClient(t, h)

	_, err := client.CreateEndpointConfig(t.Context(), &sagemakersdk.CreateEndpointConfigInput{
		EndpointConfigName: aws.String("explainer-config"),
		ProductionVariants: []smtypes.ProductionVariant{
			{VariantName: aws.String("v1"), ModelName: aws.String("m1")},
		},
		ExplainerConfig: &smtypes.ExplainerConfig{
			ClarifyExplainerConfig: &smtypes.ClarifyExplainerConfig{
				ShapConfig: &smtypes.ClarifyShapConfig{
					ShapBaselineConfig: &smtypes.ClarifyShapBaselineConfig{
						ShapBaseline: aws.String("0,0,0"),
					},
				},
			},
		},
		MetricsConfig: &smtypes.MetricsConfig{
			EnableDetailedObservability: aws.Bool(true),
		},
	})
	require.NoError(t, err)

	out, err := client.DescribeEndpointConfig(t.Context(), &sagemakersdk.DescribeEndpointConfigInput{
		EndpointConfigName: aws.String("explainer-config"),
	})
	require.NoError(t, err)

	require.NotNil(t, out.ExplainerConfig, "DescribeEndpointConfig must return ExplainerConfig")
	require.NotNil(t, out.ExplainerConfig.ClarifyExplainerConfig)
	require.NotNil(t, out.ExplainerConfig.ClarifyExplainerConfig.ShapConfig)
	assert.Equal(t, "0,0,0",
		aws.ToString(out.ExplainerConfig.ClarifyExplainerConfig.ShapConfig.ShapBaselineConfig.ShapBaseline))

	require.NotNil(t, out.MetricsConfig, "DescribeEndpointConfig must return MetricsConfig")
	assert.True(t, aws.ToBool(out.MetricsConfig.EnableDetailedObservability))
}

// TestHandler_ListEndpointConfigs_FilterSort verifies
// ListEndpointConfigsInput's NameContains/SortBy/SortOrder
// (api_op_ListEndpointConfigs.go, previously entirely dropped except
// NextToken) are honored.
func TestHandler_ListEndpointConfigs_FilterSort(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for _, name := range []string{"alpha-config", "beta-config", "other"} {
		doSageMakerRequest(t, h, "CreateEndpointConfig", map[string]any{
			"EndpointConfigName": name,
			"ProductionVariants": []map[string]any{
				{"VariantName": "v1", "ModelName": "m1"},
			},
		})
	}

	rec := doSageMakerRequest(t, h, "ListEndpointConfigs", map[string]any{
		"NameContains": "-config",
		"SortBy":       "Name",
		"SortOrder":    "Ascending",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	list, ok := resp["EndpointConfigs"].([]any)
	require.True(t, ok)
	require.Len(t, list, 2)

	first, _ := list[0].(map[string]any)
	second, _ := list[1].(map[string]any)
	assert.Equal(t, "alpha-config", first["EndpointConfigName"])
	assert.Equal(t, "beta-config", second["EndpointConfigName"])
}

func TestDeleteEndpointConfig_NotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantCode int
	}{
		{
			name:     "delete non-existent config returns 400",
			body:     map[string]any{"EndpointConfigName": "does-not-exist"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doSageMakerRequest(t, h, "DeleteEndpointConfig", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestHandler_ListEndpointConfigs_CreationTimeAfterInclusive asserts
// CreationTimeAfter is an INCLUSIVE bound -- ListEndpointConfigsInput's own
// doc: "a creation time greater than or equal to the specified time" -- not
// the family's default strict bound, so a config filtered by a
// CreationTimeAfter EQUAL to its own CreationTime must still be returned.
// CreationTime is seeded to an exact whole second -- see
// TestHandler_ListModels_CreationTimeAfterInclusive for why a wire-level
// round trip can't reliably hit this boundary.
func TestHandler_ListEndpointConfigs_CreationTimeAfterInclusive(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	doSageMakerRequest(t, h, "CreateEndpointConfig", map[string]any{
		"EndpointConfigName": "boundary-config",
		"ProductionVariants": []map[string]any{
			{
				"VariantName":          "AllTraffic",
				"ModelName":            "my-model",
				"InstanceType":         "ml.t2.medium",
				"InitialInstanceCount": 1,
			},
		},
	})

	boundary := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	sagemaker.SeedEndpointConfigCreationTime(h.Backend, "us-east-1", "boundary-config", boundary)

	rec := doSageMakerRequest(
		t, h, "ListEndpointConfigs", map[string]any{"CreationTimeAfter": float64(boundary.Unix())},
	)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	configs, ok := resp["EndpointConfigs"].([]any)
	require.True(t, ok)
	require.Len(t, configs, 1)

	c, ok := configs[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "boundary-config", c["EndpointConfigName"])
}
