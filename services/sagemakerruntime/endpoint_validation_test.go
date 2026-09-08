package sagemakerruntime_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
	"github.com/blackbirdworks/gopherstack/services/sagemaker"
	"github.com/blackbirdworks/gopherstack/services/sagemakerruntime"
)

// --- EndpointName validation (services/sagemaker cross-reference) ---

// fakeEndpointLookup is a minimal in-test double for
// sagemakerruntime.EndpointLookup. It lets these tests drive
// validateEndpoint's branches (unknown name / not-InService / InService)
// without standing up a full services/sagemaker backend.
type fakeEndpointLookup struct {
	endpoints map[string]*sagemaker.Endpoint
}

func (f fakeEndpointLookup) DescribeEndpoint(_ context.Context, name string) (*sagemaker.Endpoint, error) {
	ep, ok := f.endpoints[name]
	if !ok {
		return nil, sagemaker.ErrEndpointNotFound
	}

	return ep, nil
}

// TestHandler_EndpointValidation verifies that all three operations reject
// an EndpointName that is unknown, or known but not yet InService, with the
// real AWS ValidationError, and accept an InService endpoint -- once an
// EndpointLookup has been wired via SetEndpointLookup.
func TestHandler_EndpointValidation(t *testing.T) {
	t.Parallel()

	lookup := fakeEndpointLookup{endpoints: map[string]*sagemaker.Endpoint{
		"creating-ep":   {EndpointName: "creating-ep", EndpointStatus: "Creating"},
		"in-service-ep": {EndpointName: "in-service-ep", EndpointStatus: "InService"},
		"failed-ep":     {EndpointName: "failed-ep", EndpointStatus: "Failed"},
	}}

	tests := []struct {
		name         string
		endpointName string
		wantStatus   int
	}{
		{name: "unknown_endpoint_rejected", endpointName: "no-such-endpoint", wantStatus: http.StatusBadRequest},
		{name: "creating_endpoint_rejected", endpointName: "creating-ep", wantStatus: http.StatusBadRequest},
		{name: "failed_endpoint_rejected", endpointName: "failed-ep", wantStatus: http.StatusBadRequest},
		{name: "in_service_endpoint_accepted", endpointName: "in-service-ep", wantStatus: http.StatusOK},
	}

	paths := []struct {
		suffix     string
		wantStatus int // overridden to http.StatusAccepted for async when the base case is OK
	}{
		{suffix: "/invocations"},
		{suffix: "/invocations-response-stream"},
		{suffix: "/async-invocations", wantStatus: http.StatusAccepted},
	}

	for _, tt := range tests {
		for _, p := range paths {
			t.Run(tt.name+"_"+p.suffix, func(t *testing.T) {
				t.Parallel()

				h := newTestHandler(t)
				h.Backend.SetEndpointLookup(lookup)

				want := tt.wantStatus
				if want == http.StatusOK && p.wantStatus != 0 {
					want = p.wantStatus
				}

				rec := doRequestWithHeaders(
					t, h, http.MethodPost, "/endpoints/"+tt.endpointName+p.suffix,
					map[string]any{"data": "x"}, nil,
				)
				require.Equal(t, want, rec.Code)

				if want == http.StatusBadRequest {
					var body map[string]string
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
					assert.Equal(t, "ValidationError", body["__type"])
					assert.Contains(t, body["message"], tt.endpointName)
				}
			})
		}
	}
}

// TestHandler_EndpointValidation_UnwiredIsNoop verifies that when no
// EndpointLookup has been wired (the default for a bare NewInMemoryBackend,
// e.g. every other test in this package), any EndpointName is accepted --
// preserving this service's pre-existing behaviour for standalone use.
func TestHandler_EndpointValidation_UnwiredIsNoop(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/endpoints/never-registered-anywhere/invocations")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// fakeSageMakerHandlerConfig implements the unexported
// sagemakerHandlerProvider interface structurally (Go interface
// satisfaction requires no import of the unexported type), letting this
// test drive Provider.Init's real cross-service wiring path.
type fakeSageMakerHandlerConfig struct {
	handler *sagemaker.Handler
}

func (f fakeSageMakerHandlerConfig) GetSageMakerHandler() service.Registerable { return f.handler }

// TestProvider_WiresSageMakerEndpointLookup verifies end-to-end that
// Provider.Init, given an AppContext.Config exposing GetSageMakerHandler(),
// wires the resulting backend to the real services/sagemaker endpoint
// registry: an endpoint that was never created is rejected, and one that
// has genuinely reached InService (via services/sagemaker's real
// CreateEndpointFSM lifecycle, not a test double) is accepted.
func TestProvider_WiresSageMakerEndpointLookup(t *testing.T) {
	t.Parallel()

	smBackend := sagemaker.NewInMemoryBackend("000000000000", "us-east-1")
	smHandler := sagemaker.NewHandler(smBackend)

	ctx := &service.AppContext{Config: fakeSageMakerHandlerConfig{handler: smHandler}}

	p := &sagemakerruntime.Provider{}
	reg, err := p.Init(ctx)
	require.NoError(t, err)

	h, ok := reg.(*sagemakerruntime.Handler)
	require.True(t, ok)

	// No endpoint was ever created on smBackend: rejected.
	rec := doRequest(t, h, http.MethodPost, "/endpoints/ghost-endpoint/invocations")
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Create a real endpoint and drive it to InService via the real FSM.
	_, err = smBackend.CreateEndpointConfig(t.Context(), "cfg", []sagemaker.ProductionVariant{{
		VariantName:          "AllTraffic",
		InstanceType:         "ml.m5.large",
		InitialInstanceCount: 1,
		InitialVariantWeight: 1,
	}}, nil)
	require.NoError(t, err)

	_, err = smBackend.CreateEndpointFSM(t.Context(), sagemaker.CreateEndpointOptions{
		Name:               "wired-endpoint",
		EndpointConfigName: "cfg",
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		ep, descErr := smBackend.DescribeEndpoint(t.Context(), "wired-endpoint")

		return descErr == nil && ep.EndpointStatus == "InService"
	}, 2*time.Second, 10*time.Millisecond, "endpoint must reach InService via the real FSM")

	rec = doRequest(t, h, http.MethodPost, "/endpoints/wired-endpoint/invocations")
	assert.Equal(t, http.StatusOK, rec.Code)
}
