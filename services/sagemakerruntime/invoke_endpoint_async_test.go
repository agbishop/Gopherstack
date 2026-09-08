package sagemakerruntime_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/sagemakerruntime"
)

// --- InvokeEndpointAsync tests ---

func TestHandler_InvokeEndpointAsync(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		headers         map[string]string
		wantInferenceID string
	}{
		{name: "generated_inference_id"},
		{
			name:            "client_inference_id_is_preserved",
			headers:         map[string]string{"X-Amzn-Sagemaker-Inference-Id": "request-42"},
			wantInferenceID: "request-42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequestWithHeaders(t, h, http.MethodPost, "/endpoints/my-endpoint/async-invocations",
				map[string]any{"data": "async input"}, tt.headers)

			assert.Equal(t, http.StatusAccepted, rec.Code)

			var out map[string]string
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.NotEmpty(t, out["InferenceId"])
			assert.NotContains(t, out, "OutputLocation")
			assert.NotContains(t, out, "FailureLocation")
			if tt.wantInferenceID != "" {
				assert.Equal(t, tt.wantInferenceID, out["InferenceId"])
			}
			assert.Contains(t, rec.Header().Get("X-Amzn-Sagemaker-Outputlocation"), out["InferenceId"])
			assert.Contains(t, rec.Header().Get("X-Amzn-Sagemaker-Failurelocation"), out["InferenceId"])
			assert.NotEqual(t, rec.Header().Get("X-Amzn-Sagemaker-Outputlocation"),
				rec.Header().Get("X-Amzn-Sagemaker-Failurelocation"))

			async := h.Backend.ListAsyncInvocations()
			require.Len(t, async, 1)
			assert.Equal(t, out["InferenceId"], async[0].InferenceID)
			assert.Equal(t, rec.Header().Get("X-Amzn-Sagemaker-Outputlocation"), async[0].OutputLocation)
			assert.Equal(t, rec.Header().Get("X-Amzn-Sagemaker-Failurelocation"), async[0].FailureLocation)
		})
	}
}

// TestAsyncInvocationOutputLocation verifies that a caller-supplied
// X-Amzn-Sagemaker-Outputlocation request header is used verbatim in the
// response header and in the stored async invocation record.
func TestAsyncInvocationOutputLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		headers            map[string]string
		wantOutputLocation string
		containsInferID    bool
	}{
		{
			name:            "generated_location_when_not_supplied",
			headers:         nil,
			containsInferID: true,
		},
		{
			name: "caller_supplied_location_used_verbatim",
			headers: map[string]string{
				"X-Amzn-Sagemaker-Outputlocation": "s3://my-bucket/results/",
			},
			wantOutputLocation: "s3://my-bucket/results/",
		},
		{
			name: "caller_supplied_location_with_explicit_inference_id",
			headers: map[string]string{
				"X-Amzn-Sagemaker-Outputlocation": "s3://my-bucket/out/",
				"X-Amzn-Sagemaker-Inference-Id":   "my-infer-42",
			},
			wantOutputLocation: "s3://my-bucket/out/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequestWithHeaders(
				t, h, http.MethodPost,
				"/endpoints/my-endpoint/async-invocations",
				map[string]any{"data": "payload"},
				tt.headers,
			)

			require.Equal(t, http.StatusAccepted, rec.Code)

			respLoc := rec.Header().Get("X-Amzn-Sagemaker-Outputlocation")
			require.NotEmpty(t, respLoc)

			async := h.Backend.ListAsyncInvocations()
			require.Len(t, async, 1)

			if tt.wantOutputLocation != "" {
				assert.Equal(t, tt.wantOutputLocation, respLoc,
					"response header must echo caller-supplied output location")
				assert.Equal(t, tt.wantOutputLocation, async[0].OutputLocation,
					"stored output location must equal caller-supplied value")
			}

			if tt.containsInferID {
				assert.Contains(t, respLoc, async[0].InferenceID,
					"generated location must embed the inference ID")
				assert.Equal(t, respLoc, async[0].OutputLocation,
					"response header and stored location must agree")
			}
		})
	}
}

// TestAsyncInvocationFailureLocation verifies that InvokeEndpointAsync
// always returns an X-Amzn-Sagemaker-Failurelocation response header (bound by
// the real SDK to InvokeEndpointAsyncOutput.FailureLocation), distinct from
// OutputLocation, regardless of whether the caller supplied an output
// location.
func TestAsyncInvocationFailureLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		headers map[string]string
		name    string
	}{
		{name: "generated_location_when_not_supplied", headers: nil},
		{
			name: "caller_supplied_output_location",
			headers: map[string]string{
				"X-Amzn-Sagemaker-Outputlocation": "s3://my-bucket/results/",
			},
		},
		{
			name: "caller_supplied_output_location_no_trailing_slash",
			headers: map[string]string{
				"X-Amzn-Sagemaker-Outputlocation": "s3://my-bucket/results/out.json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequestWithHeaders(
				t, h, http.MethodPost,
				"/endpoints/my-endpoint/async-invocations",
				map[string]any{"data": "payload"},
				tt.headers,
			)

			require.Equal(t, http.StatusAccepted, rec.Code)

			outputLoc := rec.Header().Get("X-Amzn-Sagemaker-Outputlocation")
			failureLoc := rec.Header().Get("X-Amzn-Sagemaker-Failurelocation")
			require.NotEmpty(t, outputLoc)
			require.NotEmpty(t, failureLoc, "AWS always returns a FailureLocation, even on success")
			assert.NotEqual(t, outputLoc, failureLoc)

			async := h.Backend.ListAsyncInvocations()
			require.Len(t, async, 1)
			assert.Equal(t, failureLoc, async[0].FailureLocation)
		})
	}
}

// TestAsyncInvocationInferenceIDPreserved verifies that a caller-supplied
// X-Amzn-Sagemaker-Inference-Id is used verbatim as the stored inference ID.
func TestAsyncInvocationInferenceIDPreserved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		headers         map[string]string
		wantInferenceID string
		wantGenerated   bool
	}{
		{
			name:          "generated_when_not_supplied",
			headers:       nil,
			wantGenerated: true,
		},
		{
			name: "supplied_id_preserved",
			headers: map[string]string{
				"X-Amzn-Sagemaker-Inference-Id": "caller-id-99",
			},
			wantInferenceID: "caller-id-99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequestWithHeaders(
				t, h, http.MethodPost, "/endpoints/ep/async-invocations",
				map[string]any{"data": "payload"}, tt.headers,
			)

			require.Equal(t, http.StatusAccepted, rec.Code)

			async := h.Backend.ListAsyncInvocations()
			require.Len(t, async, 1)

			if tt.wantInferenceID != "" {
				assert.Equal(t, tt.wantInferenceID, async[0].InferenceID)
			}

			if tt.wantGenerated {
				assert.NotEmpty(t, async[0].InferenceID)
				assert.True(t, strings.HasPrefix(async[0].InferenceID, "gopherstack-"),
					"generated ID should have gopherstack- prefix")
			}
		})
	}
}

// TestBackendRecordAsyncWithSuppliedLocation verifies that an explicit
// outputLocation is stored instead of the generated fake S3 path.
func TestBackendRecordAsyncWithSuppliedLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		outputLocation     string
		wantContainsS3Mock bool
	}{
		{
			name:               "no_location_generates_fake",
			outputLocation:     "",
			wantContainsS3Mock: true,
		},
		{
			name:           "supplied_location_stored_verbatim",
			outputLocation: "s3://real-bucket/path/output/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sagemakerruntime.NewInMemoryBackend("000000000000", "us-east-1")
			inv := b.RecordAsyncInvocation("ep", "infer-1", "payload", tt.outputLocation)

			if tt.wantContainsS3Mock {
				assert.Contains(t, inv.OutputLocation, "sagemaker-runtime-mock")
			} else {
				assert.Equal(t, tt.outputLocation, inv.OutputLocation)
			}
		})
	}
}

// TestBackendRecordAsyncFailureLocationDerivation verifies that
// RecordAsyncInvocation derives a distinct FailureLocation for every shape of
// OutputLocation (generated, caller-supplied with/without a trailing slash,
// caller-supplied ending in "/output"), matching the real AWS convention of
// InvokeEndpointAsync always returning both an OutputLocation and a
// FailureLocation.
func TestBackendRecordAsyncFailureLocationDerivation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		outputLocation string
		wantFailureLoc string
	}{
		{
			name:           "generated_output_location_ends_in_output",
			outputLocation: "",
			wantFailureLoc: "s3://sagemaker-runtime-mock/ep/infer-1/failure",
		},
		{
			name:           "supplied_location_ending_in_output",
			outputLocation: "s3://real-bucket/path/output",
			wantFailureLoc: "s3://real-bucket/path/failure",
		},
		{
			name:           "supplied_location_with_trailing_slash",
			outputLocation: "s3://my-bucket/results/",
			wantFailureLoc: "s3://my-bucket/results/failure",
		},
		{
			name:           "supplied_location_opaque_key",
			outputLocation: "s3://my-bucket/results/out.json",
			wantFailureLoc: "s3://my-bucket/results/out.json-failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := sagemakerruntime.NewInMemoryBackend("000000000000", "us-east-1")
			inv := b.RecordAsyncInvocation("ep", "infer-1", "payload", tt.outputLocation)

			assert.Equal(t, tt.wantFailureLoc, inv.FailureLocation)
			assert.NotEqual(t, inv.OutputLocation, inv.FailureLocation)
		})
	}
}

// TestAsyncInvocation_BodyInputLocationMutualExclusion verifies the real
// AWS constraint documented on InvokeEndpointAsyncInput.Body ("Body and
// InputLocation are mutually exclusive. Provide exactly one of them.") --
// unenforceable client-side (only EndpointName is a required member per
// validators.go's validateOpInvokeEndpointAsyncInput), so it must be
// enforced server-side. Neither field, or both, must be rejected with
// ValidationError; exactly one of them must succeed.
func TestAsyncInvocation_BodyInputLocationMutualExclusion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		headers    map[string]string
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "neither_body_nor_input_location_rejected",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "both_body_and_input_location_rejected",
			body: "payload",
			headers: map[string]string{
				"X-Amzn-Sagemaker-Inputlocation": "s3://bucket/key",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "body_only_accepted",
			body:       "payload",
			wantStatus: http.StatusAccepted,
		},
		{
			name: "input_location_only_accepted",
			headers: map[string]string{
				"X-Amzn-Sagemaker-Inputlocation": "s3://bucket/key",
			},
			wantStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var body any
			if tt.body != "" {
				body = tt.body
			}

			h := newTestHandler(t)
			rec := doRequestWithHeaders(
				t, h, http.MethodPost, "/endpoints/my-endpoint/async-invocations",
				body, tt.headers,
			)

			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusBadRequest {
				var out map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				assert.Equal(t, "ValidationError", out["__type"])
				assert.Contains(t, out["message"], "mutually exclusive")
			}
		})
	}
}
