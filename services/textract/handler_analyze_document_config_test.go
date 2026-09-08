package textract_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/textract"
)

func analyzeDocumentBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"Document": map[string]any{
			"S3Object": map[string]any{"Bucket": "b", "Name": "doc.pdf"},
		},
		"FeatureTypes": []string{"TABLES"},
	}
	maps.Copy(body, extra)

	return body
}

// TestAnalyzeDocument_AdaptersConfig verifies AdaptersConfig.Adapters
// references are validated against real Adapter/AdapterVersion backend
// state, and that an unknown adapter surfaces InvalidParameterException --
// not ResourceNotFoundException, which AnalyzeDocument's real error
// surface has no case for at all.
func TestAnalyzeDocument_AdaptersConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		buildConfig func(t *testing.T, h *testHandlerT) map[string]any
		name        string
		wantType    string
		wantStatus  int
	}{
		{
			name: "known adapter and version succeeds",
			buildConfig: func(t *testing.T, h *testHandlerT) map[string]any {
				t.Helper()

				return map[string]any{
					"Adapters": []map[string]any{
						{"AdapterId": h.adapterID, "Version": h.adapterVersion},
					},
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "unknown adapter id gives InvalidParameterException",
			buildConfig: func(_ *testing.T, _ *testHandlerT) map[string]any {
				return map[string]any{
					"Adapters": []map[string]any{
						{"AdapterId": "no-such-adapter", "Version": "1"},
					},
				}
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "InvalidParameterException",
		},
		{
			name: "unknown version on a known adapter gives InvalidParameterException",
			buildConfig: func(t *testing.T, h *testHandlerT) map[string]any {
				t.Helper()

				return map[string]any{
					"Adapters": []map[string]any{
						{"AdapterId": h.adapterID, "Version": "no-such-version"},
					},
				}
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "InvalidParameterException",
		},
		{
			name: "empty adapters list gives InvalidParameterException",
			buildConfig: func(_ *testing.T, _ *testHandlerT) map[string]any {
				return map[string]any{"Adapters": []map[string]any{}}
			},
			wantStatus: http.StatusBadRequest,
			wantType:   "InvalidParameterException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			th := newTestHandlerT(t)

			rec := doTextractRequest(t, th.h, "AnalyzeDocument", analyzeDocumentBody(map[string]any{
				"AdaptersConfig": tt.buildConfig(t, th),
			}))
			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantType == "" {
				return
			}

			var errResp map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
			assert.Equal(t, tt.wantType, errResp["__type"])
		})
	}
}

// TestAnalyzeDocument_HumanLoopConfig verifies the two required
// HumanLoopConfig members are enforced, and that a valid config is accepted
// without HumanLoopActivationOutput ever being fabricated in the response.
func TestAnalyzeDocument_HumanLoopConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		config     map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "valid config succeeds",
			config: map[string]any{
				"FlowDefinitionArn": "arn:aws:sagemaker:us-east-1:123456789012:flow-definition/my-flow",
				"HumanLoopName":     "loop-1",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing FlowDefinitionArn is rejected",
			config:     map[string]any{"HumanLoopName": "loop-1"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing HumanLoopName is rejected",
			config: map[string]any{
				"FlowDefinitionArn": "arn:aws:sagemaker:us-east-1:123456789012:flow-definition/my-flow",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doTextractRequest(t, h, "AnalyzeDocument", analyzeDocumentBody(map[string]any{
				"HumanLoopConfig": tt.config,
			}))
			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus != http.StatusOK {
				return
			}

			var out struct {
				HumanLoopActivationOutput *struct{} `json:"HumanLoopActivationOutput"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Nil(
				t,
				out.HumanLoopActivationOutput,
				"gopherstack has no real condition-evaluation engine to decide activation, so it must never fabricate one",
			)
		})
	}
}

// testHandlerT bundles a handler with a real adapter+version already
// created, for AdaptersConfig success-path tests.
type testHandlerT struct {
	h              *textract.Handler
	adapterID      string
	adapterVersion string
}

func newTestHandlerT(t *testing.T) *testHandlerT {
	t.Helper()

	h := newTestHandler(t)

	createRec := doTextractRequest(t, h, "CreateAdapter", map[string]any{
		"AdapterName":  "analyze-doc-adapter",
		"FeatureTypes": []string{"FORMS"},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]string
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))

	versionRec := doTextractRequest(t, h, "CreateAdapterVersion", map[string]any{
		"AdapterId": createResp["AdapterId"],
		"DatasetConfig": map[string]any{
			"ManifestS3Object": map[string]any{
				"Bucket": "test-dataset-bucket",
				"Name":   "manifest.json",
			},
		},
		"OutputConfig": map[string]any{
			"S3Bucket": "test-output-bucket",
		},
	})
	require.Equal(t, http.StatusOK, versionRec.Code)

	var versionResp map[string]string
	require.NoError(t, json.Unmarshal(versionRec.Body.Bytes(), &versionResp))

	return &testHandlerT{
		h:              h,
		adapterID:      createResp["AdapterId"],
		adapterVersion: versionResp["AdapterVersion"],
	}
}
