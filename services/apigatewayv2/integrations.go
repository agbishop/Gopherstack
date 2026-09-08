package apigatewayv2

import (
	"fmt"
	"slices"
	"sort"
)

// integrationTimeoutMaxFor returns the maximum (and default) integration
// timeout in milliseconds for the given API protocol type: 30,000ms for HTTP
// APIs and 29,000ms for WebSocket APIs, matching real API Gateway v2 limits.
func integrationTimeoutMaxFor(protocolType string) int32 {
	if protocolType == protocolTypeHTTP {
		return integrationTimeoutMaxHTTP
	}

	return integrationTimeoutMaxWebSocket
}

// validateTimeoutInMillis returns ErrBadRequest if ms is outside
// [50, integrationTimeoutMaxFor(protocolType)].
func validateTimeoutInMillis(ms int32, protocolType string) error {
	maxMs := integrationTimeoutMaxFor(protocolType)
	if ms < integrationTimeoutMin || ms > maxMs {
		return fmt.Errorf(
			"%w: timeoutInMillis must be between %d and %d",
			ErrBadRequest, integrationTimeoutMin, maxMs,
		)
	}

	return nil
}

// validateConnectionType returns ErrBadRequest if connectionType is not one of
// the modeled enum values (INTERNET, VPC_LINK), or if VPC_LINK is specified
// without a connectionId (the VPC link to route the private integration
// through), matching real API Gateway v2 validation.
func validateConnectionType(connectionType, connectionID string) error {
	switch connectionType {
	case connectionTypeInternet:
		return nil
	case connectionTypeVpcLink:
		if connectionID == "" {
			return fmt.Errorf("%w: connectionId is required when connectionType is VPC_LINK", ErrBadRequest)
		}

		return nil
	default:
		return fmt.Errorf("%w: connectionType must be one of INTERNET, VPC_LINK", ErrBadRequest)
	}
}

// validateIntegrationTypeForProtocol returns ErrBadRequest if integrationType
// is AWS, HTTP, or MOCK on an HTTP API. Per api_op_CreateIntegration.go's
// IntegrationType doc comment, those three are each "Supported only for
// WebSocket APIs" -- only AWS_PROXY and HTTP_PROXY are valid on HTTP APIs.
func validateIntegrationTypeForProtocol(integrationType, protocolType string) error {
	if protocolType != protocolTypeHTTP {
		return nil
	}

	switch integrationType {
	case "AWS", integrationTypeHTTP, integrationTypeMock:
		return fmt.Errorf(
			"%w: integrationType %s is supported only for WebSocket APIs", ErrBadRequest, integrationType,
		)
	default:
		return nil
	}
}

// cloneIntegrationTLSConfig returns a deep copy of cfg, or nil if cfg is nil.
func cloneIntegrationTLSConfig(cfg *IntegrationTLSConfig) *IntegrationTLSConfig {
	if cfg == nil {
		return nil
	}

	cp := *cfg

	return &cp
}

// CreateIntegration creates a new integration for an API.
func (b *InMemoryBackend) CreateIntegration(apiID string, input CreateIntegrationInput) (*Integration, error) {
	b.mu.Lock("CreateIntegration")
	defer b.mu.Unlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return nil, ErrAPINotFound
	}

	integration, err := buildIntegration(apiID, api.ProtocolType, input)
	if err != nil {
		return nil, err
	}

	b.integrations.Put(integration)
	b.autoDeployLocked(apiID)

	cp := *integration

	return &cp, nil
}

// buildIntegration validates input and constructs a new Integration for apiID
// (of the given protocolType), applying the same AWS-realistic defaults
// CreateIntegration always has. It does not touch backend state -- callers
// (CreateIntegration, and CreateAPI's quick-create shortcut) store it and
// hold b.mu themselves.
func buildIntegration(apiID, protocolType string, input CreateIntegrationInput) (*Integration, error) {
	validTypes := map[string]bool{
		"AWS":                    true,
		integrationTypeHTTP:      true,
		integrationTypeMock:      true,
		IntegrationTypeAWSProxy:  true,
		integrationTypeHTTPProxy: true,
	}
	if !validTypes[input.IntegrationType] {
		return nil, fmt.Errorf(
			"%w: integrationType must be one of AWS, HTTP, MOCK, AWS_PROXY, HTTP_PROXY",
			ErrBadRequest,
		)
	}

	if err := validateIntegrationTypeForProtocol(input.IntegrationType, protocolType); err != nil {
		return nil, err
	}

	// Apply AWS-realistic defaults.
	payloadFmtVer := input.PayloadFormatVersion
	if payloadFmtVer == "" && input.IntegrationType == IntegrationTypeAWSProxy {
		payloadFmtVer = "1.0"
	}

	passthroughBehavior := input.PassthroughBehavior
	if passthroughBehavior == "" && input.IntegrationType == integrationTypeHTTPProxy {
		passthroughBehavior = "WHEN_NO_MATCH"
	}

	connectionType := input.ConnectionType
	if connectionType == "" {
		connectionType = connectionTypeInternet
	}

	if err := validateConnectionType(connectionType, input.ConnectionID); err != nil {
		return nil, err
	}

	timeoutMs := input.TimeoutInMillis
	if timeoutMs == 0 {
		timeoutMs = integrationTimeoutMaxFor(protocolType)
	} else if err := validateTimeoutInMillis(timeoutMs, protocolType); err != nil {
		return nil, err
	}

	return &Integration{
		IntegrationID:               randomID(),
		APIID:                       apiID,
		IntegrationType:             input.IntegrationType,
		IntegrationSubtype:          input.IntegrationSubtype,
		IntegrationMethod:           input.IntegrationMethod,
		IntegrationURI:              input.IntegrationURI,
		Description:                 input.Description,
		PayloadFormatVersion:        payloadFmtVer,
		ConnectionType:              connectionType,
		ConnectionID:                input.ConnectionID,
		CredentialsArn:              input.CredentialsArn,
		TimeoutInMillis:             timeoutMs,
		RequestParameters:           input.RequestParameters,
		RequestTemplates:            input.RequestTemplates,
		TemplateSelectionExpression: input.TemplateSelectionExpression,
		PassthroughBehavior:         passthroughBehavior,
		TLSConfig:                   cloneIntegrationTLSConfig(input.TLSConfig),
	}, nil
}

// GetIntegration retrieves an integration by ID.
func (b *InMemoryBackend) GetIntegration(apiID, integrationID string) (*Integration, error) {
	b.mu.RLock("GetIntegration")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	i, ok := b.integrations.Get(integrationKey(apiID, integrationID))
	if !ok {
		return nil, ErrIntegrationNotFound
	}

	cp := *i

	return &cp, nil
}

// GetIntegrations retrieves all integrations for an API.
func (b *InMemoryBackend) GetIntegrations(apiID string) ([]Integration, error) {
	b.mu.RLock("GetIntegrations")
	defer b.mu.RUnlock()

	if !b.apis.Has(apiID) {
		return nil, ErrAPINotFound
	}

	integrations := b.integrationsByAPI.Get(apiID)
	result := make([]Integration, 0, len(integrations))

	for _, i := range integrations {
		result = append(result, *i)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].IntegrationID < result[j].IntegrationID
	})

	return result, nil
}

// DeleteIntegration removes an integration from an API.
func (b *InMemoryBackend) DeleteIntegration(apiID, integrationID string) error {
	b.mu.Lock("DeleteIntegration")
	defer b.mu.Unlock()

	if !b.apis.Has(apiID) {
		return ErrAPINotFound
	}

	if !b.integrations.Delete(integrationKey(apiID, integrationID)) {
		return ErrIntegrationNotFound
	}

	for _, ir := range slices.Clone(b.integrationResponsesByIntegration.Get(integrationKey(apiID, integrationID))) {
		b.integrationResponses.Delete(integrationResponseKey(apiID, integrationID, ir.IntegrationResponseID))
	}

	b.autoDeployLocked(apiID)

	return nil
}

// UpdateIntegration updates fields on an existing integration.
// applyIntegrationUpdate copies non-zero fields from input onto the
// integration. Split into two helpers (by field group) to stay under the
// cyclomatic complexity threshold rather than growing a single branch-heavy
// function.
func applyIntegrationUpdate(i *Integration, input UpdateIntegrationInput) {
	applyIntegrationIdentityUpdate(i, input)
	applyIntegrationBehaviorUpdate(i, input)
}

// applyIntegrationIdentityUpdate copies the "what/where" fields (type,
// subtype, method, URI, description, connection/credentials) from input onto
// the integration.
func applyIntegrationIdentityUpdate(i *Integration, input UpdateIntegrationInput) {
	if input.IntegrationType != "" {
		i.IntegrationType = input.IntegrationType
	}

	if input.IntegrationSubtype != "" {
		i.IntegrationSubtype = input.IntegrationSubtype
	}

	if input.IntegrationMethod != "" {
		i.IntegrationMethod = input.IntegrationMethod
	}

	if input.IntegrationURI != "" {
		i.IntegrationURI = input.IntegrationURI
	}

	if input.Description != "" {
		i.Description = input.Description
	}

	if input.ConnectionType != "" {
		i.ConnectionType = input.ConnectionType
	}

	if input.ConnectionID != "" {
		i.ConnectionID = input.ConnectionID
	}

	if input.CredentialsArn != "" {
		i.CredentialsArn = input.CredentialsArn
	}
}

// applyIntegrationBehaviorUpdate copies the request/response-shaping fields
// (payload version, timeout, templates, passthrough, TLS) from input onto
// the integration.
func applyIntegrationBehaviorUpdate(i *Integration, input UpdateIntegrationInput) {
	if input.PayloadFormatVersion != "" {
		i.PayloadFormatVersion = input.PayloadFormatVersion
	}

	if input.TimeoutInMillis != 0 {
		i.TimeoutInMillis = input.TimeoutInMillis
	}

	if input.RequestParameters != nil {
		i.RequestParameters = input.RequestParameters
	}

	if input.RequestTemplates != nil {
		i.RequestTemplates = input.RequestTemplates
	}

	if input.TemplateSelectionExpression != "" {
		i.TemplateSelectionExpression = input.TemplateSelectionExpression
	}

	if input.PassthroughBehavior != "" {
		i.PassthroughBehavior = input.PassthroughBehavior
	}

	if input.TLSConfig != nil {
		i.TLSConfig = cloneIntegrationTLSConfig(input.TLSConfig)
	}
}

// UpdateIntegration updates fields on an existing integration.
func (b *InMemoryBackend) UpdateIntegration(
	apiID, integrationID string,
	input UpdateIntegrationInput,
) (*Integration, error) {
	b.mu.Lock("UpdateIntegration")
	defer b.mu.Unlock()

	api, ok := b.apis.Get(apiID)
	if !ok {
		return nil, ErrAPINotFound
	}

	i, ok := b.integrations.Get(integrationKey(apiID, integrationID))
	if !ok {
		return nil, ErrIntegrationNotFound
	}

	if input.IntegrationType != "" {
		if err := validateIntegrationTypeForProtocol(input.IntegrationType, api.ProtocolType); err != nil {
			return nil, err
		}
	}

	if input.TimeoutInMillis != 0 {
		if err := validateTimeoutInMillis(input.TimeoutInMillis, api.ProtocolType); err != nil {
			return nil, err
		}
	}

	effectiveConnectionType := i.ConnectionType
	if input.ConnectionType != "" {
		effectiveConnectionType = input.ConnectionType
	}

	effectiveConnectionID := i.ConnectionID
	if input.ConnectionID != "" {
		effectiveConnectionID = input.ConnectionID
	}

	if input.ConnectionType != "" || input.ConnectionID != "" {
		if err := validateConnectionType(effectiveConnectionType, effectiveConnectionID); err != nil {
			return nil, err
		}
	}

	applyIntegrationUpdate(i, input)
	b.autoDeployLocked(apiID)

	cp := *i

	return &cp, nil
}
