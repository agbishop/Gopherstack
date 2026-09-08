package apigatewayv2_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/apigatewayv2"
)

func TestInMemoryBackend_Integrations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		integrationType string
	}{
		{
			name:            "aws_proxy",
			integrationType: "AWS_PROXY",
		},
		{
			name:            "http_proxy",
			integrationType: "HTTP_PROXY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := apigatewayv2.NewInMemoryBackend()

			api, err := b.CreateAPI(
				context.Background(),
				apigatewayv2.CreateAPIInput{Name: "test", ProtocolType: "HTTP"},
			)
			require.NoError(t, err)

			integration, err := b.CreateIntegration(api.APIID, apigatewayv2.CreateIntegrationInput{
				IntegrationType: tt.integrationType,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.integrationType, integration.IntegrationType)
			assert.NotEmpty(t, integration.IntegrationID)

			got, err := b.GetIntegration(api.APIID, integration.IntegrationID)
			require.NoError(t, err)
			assert.Equal(t, integration.IntegrationID, got.IntegrationID)

			integrations, err := b.GetIntegrations(api.APIID)
			require.NoError(t, err)
			assert.Len(t, integrations, 1)

			err = b.DeleteIntegration(api.APIID, integration.IntegrationID)
			require.NoError(t, err)

			_, err = b.GetIntegration(api.APIID, integration.IntegrationID)
			require.ErrorIs(t, err, apigatewayv2.ErrIntegrationNotFound)
		})
	}
}

func TestInMemoryBackend_UpdateIntegration_AllFields(t *testing.T) {
	t.Parallel()

	b := apigatewayv2.NewInMemoryBackend()

	api, err := b.CreateAPI(context.Background(), apigatewayv2.CreateAPIInput{Name: "test", ProtocolType: "HTTP"})
	require.NoError(t, err)

	integration, err := b.CreateIntegration(api.APIID, apigatewayv2.CreateIntegrationInput{
		IntegrationType: "AWS_PROXY",
	})
	require.NoError(t, err)

	updated, err := b.UpdateIntegration(api.APIID, integration.IntegrationID, apigatewayv2.UpdateIntegrationInput{
		IntegrationType:      "HTTP_PROXY",
		IntegrationMethod:    "POST",
		IntegrationURI:       "https://example.com",
		Description:          "updated",
		PayloadFormatVersion: "2.0",
		ConnectionType:       "INTERNET",
		ConnectionID:         "conn-1",
		TimeoutInMillis:      5000,
	})
	require.NoError(t, err)
	assert.Equal(t, "HTTP_PROXY", updated.IntegrationType)
	assert.Equal(t, "POST", updated.IntegrationMethod)
	assert.Equal(t, int32(5000), updated.TimeoutInMillis)
}

func TestInMemoryBackend_CreateIntegration_ApiNotFound(t *testing.T) {
	t.Parallel()

	b := apigatewayv2.NewInMemoryBackend()

	_, err := b.CreateIntegration("bad-api", apigatewayv2.CreateIntegrationInput{IntegrationType: "AWS_PROXY"})
	require.ErrorIs(t, err, apigatewayv2.ErrAPINotFound)
}

// TestInMemoryBackend_CreateIntegration_TimeoutValidation proves the
// per-protocol timeout ceiling AWS documents for CreateIntegration: HTTP APIs
// allow up to 30,000ms (default 30,000ms when unset) while WebSocket APIs
// allow up to 29,000ms (default 29,000ms when unset). Before this fix the
// backend hardcoded a 29,000ms ceiling/default for both protocols, incorrectly
// rejecting valid HTTP API timeouts in [29001, 30000] and under-defaulting
// unset HTTP API timeouts to 29000 instead of 30000.
func TestInMemoryBackend_CreateIntegration_TimeoutValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		protocolType string
		timeoutMs    int32
		wantErr      bool
		wantTimeout  int32
	}{
		{name: "http_zero_defaults_to_30000", protocolType: "HTTP", timeoutMs: 0, wantErr: false, wantTimeout: 30000},
		{name: "http_min_boundary_50", protocolType: "HTTP", timeoutMs: 50, wantErr: false, wantTimeout: 50},
		{name: "http_max_boundary_30000", protocolType: "HTTP", timeoutMs: 30000, wantErr: false, wantTimeout: 30000},
		{name: "http_mid_range_5000", protocolType: "HTTP", timeoutMs: 5000, wantErr: false, wantTimeout: 5000},
		{name: "http_29001_now_valid", protocolType: "HTTP", timeoutMs: 29001, wantErr: false, wantTimeout: 29001},
		{name: "http_too_low_49", protocolType: "HTTP", timeoutMs: 49, wantErr: true},
		{name: "http_too_low_1", protocolType: "HTTP", timeoutMs: 1, wantErr: true},
		{name: "http_too_high_30001", protocolType: "HTTP", timeoutMs: 30001, wantErr: true},
		{name: "http_too_high_60000", protocolType: "HTTP", timeoutMs: 60000, wantErr: true},
		{
			name: "ws_zero_defaults_to_29000", protocolType: "WEBSOCKET",
			timeoutMs: 0, wantErr: false, wantTimeout: 29000,
		},
		{
			name: "ws_max_boundary_29000", protocolType: "WEBSOCKET",
			timeoutMs: 29000, wantErr: false, wantTimeout: 29000,
		},
		{name: "ws_too_high_29001", protocolType: "WEBSOCKET", timeoutMs: 29001, wantErr: true},
		{name: "ws_too_high_30000", protocolType: "WEBSOCKET", timeoutMs: 30000, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := apigatewayv2.NewInMemoryBackend()

			api, err := b.CreateAPI(context.Background(), apigatewayv2.CreateAPIInput{
				Name:         "api",
				ProtocolType: tt.protocolType,
			})
			require.NoError(t, err)

			integrationType := "HTTP_PROXY"
			if tt.protocolType == "WEBSOCKET" {
				integrationType = "AWS"
			}

			intg, err := b.CreateIntegration(api.APIID, apigatewayv2.CreateIntegrationInput{
				IntegrationType: integrationType,
				IntegrationURI:  "https://example.com",
				TimeoutInMillis: tt.timeoutMs,
			})

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, apigatewayv2.ErrBadRequest)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantTimeout, intg.TimeoutInMillis)
			}
		})
	}
}

func TestInMemoryBackend_UpdateIntegration_TimeoutValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		protocolType string
		timeoutMs    int32
		wantErr      bool
	}{
		{name: "http_valid_50", protocolType: "HTTP", timeoutMs: 50, wantErr: false},
		{name: "http_valid_30000", protocolType: "HTTP", timeoutMs: 30000, wantErr: false},
		{name: "http_valid_1000", protocolType: "HTTP", timeoutMs: 1000, wantErr: false},
		{name: "http_zero_skips_update", protocolType: "HTTP", timeoutMs: 0, wantErr: false},
		{name: "http_too_low_49", protocolType: "HTTP", timeoutMs: 49, wantErr: true},
		{name: "http_too_high_30001", protocolType: "HTTP", timeoutMs: 30001, wantErr: true},
		{name: "ws_valid_29000", protocolType: "WEBSOCKET", timeoutMs: 29000, wantErr: false},
		{name: "ws_too_high_29001", protocolType: "WEBSOCKET", timeoutMs: 29001, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := apigatewayv2.NewInMemoryBackend()

			api, err := b.CreateAPI(context.Background(), apigatewayv2.CreateAPIInput{
				Name:         "api",
				ProtocolType: tt.protocolType,
			})
			require.NoError(t, err)

			integrationType := "HTTP_PROXY"
			if tt.protocolType == "WEBSOCKET" {
				integrationType = "AWS"
			}

			intg, err := b.CreateIntegration(api.APIID, apigatewayv2.CreateIntegrationInput{
				IntegrationType: integrationType,
				IntegrationURI:  "https://example.com",
			})
			require.NoError(t, err)

			_, err = b.UpdateIntegration(api.APIID, intg.IntegrationID, apigatewayv2.UpdateIntegrationInput{
				TimeoutInMillis: tt.timeoutMs,
			})

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, apigatewayv2.ErrBadRequest)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// Test_CreateIntegration_ConnectionType proves ConnectionType defaults to
// INTERNET when unset (matching the AWS-documented default), is validated
// against the modeled enum (INTERNET, VPC_LINK), and requires a connectionId
// when VPC_LINK is specified. Before this fix ConnectionType had no default
// and no validation, so GetIntegration returned an empty string instead of
// "INTERNET" for the common case of an internet-routed integration.
func Test_CreateIntegration_ConnectionType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		connectionType string
		connectionID   string
		wantConnType   string
		wantErr        bool
	}{
		{name: "unset_defaults_to_internet", connectionType: "", wantErr: false, wantConnType: "INTERNET"},
		{name: "explicit_internet", connectionType: "INTERNET", wantErr: false, wantConnType: "INTERNET"},
		{
			name: "vpc_link_with_connection_id", connectionType: "VPC_LINK", connectionID: "vpc-link-1",
			wantErr: false, wantConnType: "VPC_LINK",
		},
		{name: "vpc_link_missing_connection_id", connectionType: "VPC_LINK", wantErr: true},
		{name: "invalid_enum_value", connectionType: "BOGUS", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := apigatewayv2.NewInMemoryBackend()

			api, err := b.CreateAPI(context.Background(), apigatewayv2.CreateAPIInput{
				Name:         "api",
				ProtocolType: "HTTP",
			})
			require.NoError(t, err)

			intg, err := b.CreateIntegration(api.APIID, apigatewayv2.CreateIntegrationInput{
				IntegrationType: "HTTP_PROXY",
				IntegrationURI:  "https://example.com",
				ConnectionType:  tc.connectionType,
				ConnectionID:    tc.connectionID,
			})

			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, apigatewayv2.ErrBadRequest)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantConnType, intg.ConnectionType)

			got, err := b.GetIntegration(api.APIID, intg.IntegrationID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantConnType, got.ConnectionType)
		})
	}
}

// Test_Integration_TlsConfig proves the tlsConfig field (AWS-modeled for
// private integrations) round-trips through Create, Get, and Update. Before
// this fix the Integration model had no TLSConfig field at all, so any client
// setting it silently lost the value.
func Test_Integration_TlsConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		createTLS       *apigatewayv2.IntegrationTLSConfig
		updateTLS       *apigatewayv2.IntegrationTLSConfig
		wantAfterUpdate *apigatewayv2.IntegrationTLSConfig
		name            string
	}{
		{
			name:            "create_with_tls_no_update",
			createTLS:       &apigatewayv2.IntegrationTLSConfig{ServerNameToVerify: "backend.example.com"},
			wantAfterUpdate: &apigatewayv2.IntegrationTLSConfig{ServerNameToVerify: "backend.example.com"},
		},
		{
			name:      "create_without_tls_then_add_via_update",
			createTLS: nil,
			updateTLS: &apigatewayv2.IntegrationTLSConfig{ServerNameToVerify: "updated.example.com"},
			wantAfterUpdate: &apigatewayv2.IntegrationTLSConfig{
				ServerNameToVerify: "updated.example.com",
			},
		},
		{
			name:            "create_without_tls_stays_nil",
			createTLS:       nil,
			wantAfterUpdate: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := apigatewayv2.NewInMemoryBackend()

			api, err := b.CreateAPI(context.Background(), apigatewayv2.CreateAPIInput{
				Name:         "api",
				ProtocolType: "HTTP",
			})
			require.NoError(t, err)

			intg, err := b.CreateIntegration(api.APIID, apigatewayv2.CreateIntegrationInput{
				IntegrationType: "HTTP_PROXY",
				IntegrationURI:  "https://example.com",
				ConnectionType:  "VPC_LINK",
				ConnectionID:    "vpc-link-1",
				TLSConfig:       tc.createTLS,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.createTLS, intg.TLSConfig)

			if tc.updateTLS != nil {
				_, err = b.UpdateIntegration(api.APIID, intg.IntegrationID, apigatewayv2.UpdateIntegrationInput{
					TLSConfig: tc.updateTLS,
				})
				require.NoError(t, err)
			}

			got, err := b.GetIntegration(api.APIID, intg.IntegrationID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAfterUpdate, got.TLSConfig)

			// Mutating the caller's struct after Create/Update must not alias
			// backend state (deep-copy proof).
			if tc.createTLS != nil {
				tc.createTLS.ServerNameToVerify = "mutated"

				afterMutate, getErr := b.GetIntegration(api.APIID, intg.IntegrationID)
				require.NoError(t, getErr)
				assert.NotEqual(t, "mutated", func() string {
					if afterMutate.TLSConfig == nil {
						return ""
					}

					return afterMutate.TLSConfig.ServerNameToVerify
				}())
			}
		})
	}
}

// Test_Integration_CredentialsArn covers Integration.CredentialsArn, which
// the real AWS SDK carries on Integration/CreateIntegrationInput/
// UpdateIntegrationInput but was entirely absent from gopherstack's shapes,
// so a caller-supplied credentialsArn was silently dropped on decode and
// never returned.
func Test_Integration_CredentialsArn(t *testing.T) {
	t.Parallel()

	b := apigatewayv2.NewInMemoryBackend()

	api, err := b.CreateAPI(context.Background(), apigatewayv2.CreateAPIInput{Name: "api", ProtocolType: "WEBSOCKET"})
	require.NoError(t, err)

	const roleARN = "arn:aws:iam::123456789012:role/apigw-role"

	intg, err := b.CreateIntegration(api.APIID, apigatewayv2.CreateIntegrationInput{
		IntegrationType: "AWS",
		IntegrationURI:  "arn:aws:apigateway:us-east-1:s3:path/bucket/key",
		CredentialsArn:  roleARN,
	})
	require.NoError(t, err)
	assert.Equal(t, roleARN, intg.CredentialsArn)

	got, err := b.GetIntegration(api.APIID, intg.IntegrationID)
	require.NoError(t, err)
	assert.Equal(t, roleARN, got.CredentialsArn)

	const updatedARN = "arn:aws:iam::*:user/*"

	updated, err := b.UpdateIntegration(api.APIID, intg.IntegrationID, apigatewayv2.UpdateIntegrationInput{
		CredentialsArn: updatedARN,
	})
	require.NoError(t, err)
	assert.Equal(t, updatedARN, updated.CredentialsArn)
}
