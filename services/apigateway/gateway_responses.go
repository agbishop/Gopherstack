package apigateway

import (
	"fmt"
)

// GatewayResponseKey generates a storage key for gateway responses.
func gatewayResponseKey(restAPIID, responseType string) string {
	return restAPIID + "#" + responseType
}

// GetGatewayResponse retrieves a gateway response by type.
func (b *InMemoryBackend) GetGatewayResponse(restAPIID, responseType string) (*GatewayResponse, error) {
	b.mu.RLock("GetGatewayResponse")
	defer b.mu.RUnlock()

	key := gatewayResponseKey(restAPIID, responseType)
	gr, ok := b.gatewayResponses.Get(key)
	if !ok {
		// Return default response (AWS returns default responses even when not explicitly set).
		return &GatewayResponse{
			RestAPIID:       restAPIID,
			ResponseType:    responseType,
			DefaultResponse: true,
			StatusCode:      gatewayResponseDefaultStatus(responseType),
		}, nil
	}

	cp := *gr

	return &cp, nil
}

// gatewayResponseDefaultStatus returns the default HTTP status for a gateway response type.
func gatewayResponseDefaultStatus(responseType string) string {
	switch responseType {
	case "UNAUTHORIZED", "ACCESS_DENIED":
		return "401"
	case "RESOURCE_NOT_FOUND":
		return "404"
	case "THROTTLED", "QUOTA_EXCEEDED":
		return "429"
	case "BAD_REQUEST_BODY", "BAD_REQUEST_PARAMETERS":
		return "400"
	case "REQUEST_TOO_LARGE":
		return "413"
	case "AUTHORIZER_FAILURE", "AUTHORIZER_CONFIGURATION_ERROR":
		return "500"
	case "DEFAULT_4XX":
		return "400"
	case "DEFAULT_5XX":
		return "500"
	default:
		return "500"
	}
}

// GetGatewayResponses retrieves all gateway responses for a REST API.
func (b *InMemoryBackend) GetGatewayResponses(restAPIID string) ([]GatewayResponse, error) {
	b.mu.RLock("GetGatewayResponses")
	defer b.mu.RUnlock()

	if !b.restApis.Has(restAPIID) {
		return nil, fmt.Errorf("%w: REST API %s not found", ErrRestAPINotFound, restAPIID)
	}

	defaultTypes := []string{
		"UNAUTHORIZED", "ACCESS_DENIED", "RESOURCE_NOT_FOUND",
		"THROTTLED", "QUOTA_EXCEEDED", "BAD_REQUEST_BODY",
		"BAD_REQUEST_PARAMETERS", "REQUEST_TOO_LARGE",
		"AUTHORIZER_FAILURE", "AUTHORIZER_CONFIGURATION_ERROR",
		"DEFAULT_4XX", "DEFAULT_5XX",
	}

	result := make([]GatewayResponse, 0, len(defaultTypes))

	for _, rt := range defaultTypes {
		key := gatewayResponseKey(restAPIID, rt)
		if gr, ok := b.gatewayResponses.Get(key); ok {
			result = append(result, *gr)
		} else {
			result = append(result, GatewayResponse{
				RestAPIID:       restAPIID,
				ResponseType:    rt,
				DefaultResponse: true,
				StatusCode:      gatewayResponseDefaultStatus(rt),
			})
		}
	}

	return result, nil
}

// PutGatewayResponse creates or updates a gateway response.
func (b *InMemoryBackend) PutGatewayResponse(input PutGatewayResponseInput) (*GatewayResponse, error) {
	b.mu.Lock("PutGatewayResponse")
	defer b.mu.Unlock()

	if !b.restApis.Has(input.RestAPIID) {
		return nil, fmt.Errorf("%w: REST API %s not found", ErrRestAPINotFound, input.RestAPIID)
	}

	gr := &GatewayResponse{
		RestAPIID:          input.RestAPIID,
		ResponseType:       input.ResponseType,
		StatusCode:         input.StatusCode,
		ResponseParameters: input.ResponseParameters,
		ResponseTemplates:  input.ResponseTemplates,
		DefaultResponse:    false,
	}

	if gr.StatusCode == "" {
		gr.StatusCode = gatewayResponseDefaultStatus(input.ResponseType)
	}

	b.gatewayResponses.Put(gr)

	cp := *gr

	return &cp, nil
}

// UpdateGatewayResponse applies a partial (PATCH) update to a gateway response,
// merging only the fields present in input with the existing response (or with
// AWS's implicit default response for responseType, if none has been customized
// yet). Unlike PutGatewayResponse — which is a full wholesale replace used by the
// real PUT operation — UpdateGatewayResponse must not clobber
// ResponseParameters/ResponseTemplates/StatusCode that weren't part of this PATCH
// document, matching AWS's PATCH-operation semantics for this resource.
func (b *InMemoryBackend) UpdateGatewayResponse(input PutGatewayResponseInput) (*GatewayResponse, error) {
	b.mu.Lock("UpdateGatewayResponse")
	defer b.mu.Unlock()

	if !b.restApis.Has(input.RestAPIID) {
		return nil, fmt.Errorf("%w: REST API %s not found", ErrRestAPINotFound, input.RestAPIID)
	}

	key := gatewayResponseKey(input.RestAPIID, input.ResponseType)

	existing, ok := b.gatewayResponses.Get(key)
	if !ok {
		existing = &GatewayResponse{
			RestAPIID:       input.RestAPIID,
			ResponseType:    input.ResponseType,
			StatusCode:      gatewayResponseDefaultStatus(input.ResponseType),
			DefaultResponse: true,
		}
	}

	cp := *existing
	if input.StatusCode != "" {
		cp.StatusCode = input.StatusCode
	}
	if input.ResponseParameters != nil {
		cp.ResponseParameters = input.ResponseParameters
	}
	if input.ResponseTemplates != nil {
		cp.ResponseTemplates = input.ResponseTemplates
	}
	cp.DefaultResponse = false
	b.gatewayResponses.Put(&cp)

	return &cp, nil
}

// deleteGatewayResponsesForAPILocked removes every custom gateway response
// scoped to restAPIID. Callers must hold b.mu.
func (b *InMemoryBackend) deleteGatewayResponsesForAPILocked(restAPIID string) {
	for _, gr := range b.gatewayResponses.All() {
		if gr.RestAPIID == restAPIID {
			b.gatewayResponses.Delete(gatewayResponseKey(restAPIID, gr.ResponseType))
		}
	}
}

// DeleteGatewayResponse removes a custom gateway response, reverting to default.
func (b *InMemoryBackend) DeleteGatewayResponse(restAPIID, responseType string) error {
	b.mu.Lock("DeleteGatewayResponse")
	defer b.mu.Unlock()

	if !b.restApis.Has(restAPIID) {
		return fmt.Errorf("%w: REST API %s not found", ErrRestAPINotFound, restAPIID)
	}

	key := gatewayResponseKey(restAPIID, responseType)
	b.gatewayResponses.Delete(key)

	return nil
}
