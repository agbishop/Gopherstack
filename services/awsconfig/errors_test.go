package awsconfig_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/awsconfig"
)

func TestAWSConfigBackend_DeleteErrors_SpecificTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantErr error
		setup   func(t *testing.T, b *awsconfig.InMemoryBackend)
		del     func(b *awsconfig.InMemoryBackend) error
		name    string
	}{
		{
			name:    "delete_delivery_channel_not_found",
			del:     func(b *awsconfig.InMemoryBackend) error { return b.DeleteDeliveryChannel("x") },
			wantErr: awsconfig.ErrNoSuchDeliveryChannel,
		},
		{
			name:    "delete_config_rule_not_found",
			del:     func(b *awsconfig.InMemoryBackend) error { return b.DeleteConfigRule("x") },
			wantErr: awsconfig.ErrNoSuchConfigRule,
		},
		{
			name:    "delete_aggregator_not_found",
			del:     func(b *awsconfig.InMemoryBackend) error { return b.DeleteConfigurationAggregator("x") },
			wantErr: awsconfig.ErrNoSuchAggregator,
		},
		{
			name:    "delete_conformance_pack_not_found",
			del:     func(b *awsconfig.InMemoryBackend) error { return b.DeleteConformancePack("x") },
			wantErr: awsconfig.ErrNoSuchConformancePack,
		},
		{
			name:    "delete_org_config_rule_not_found",
			del:     func(b *awsconfig.InMemoryBackend) error { return b.DeleteOrganizationConfigRule("x") },
			wantErr: awsconfig.ErrNoSuchOrganizationConfigRule,
		},
		{
			name:    "delete_org_conformance_pack_not_found",
			del:     func(b *awsconfig.InMemoryBackend) error { return b.DeleteOrganizationConformancePack("x") },
			wantErr: awsconfig.ErrNoSuchOrganizationConformancePack,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := awsconfig.NewInMemoryBackend()
			if tt.setup != nil {
				tt.setup(t, b)
			}

			err := tt.del(b)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestAWSConfigHandler_UndeclaredValidationCodes locks in that a required-field
// check on an op whose declared error model has no ValidationException emits the
// code the op actually declares instead. Before the fix, every one of these
// backends raised the shared ErrValidation sentinel (aggregators.go, config_rules.go,
// conformance_packs.go, remediation.go, retention.go, handler_config_rules.go),
// wiring "ValidationException" onto ops none of which declare it (configservice@v1.68.4
// deserializers.go): DeleteAggregationAuthorization/DeletePendingAggregationRequest/
// PutConfigurationAggregator/PutConfigRule/PutConformancePack/StartRemediationExecution/
// PutRetentionConfiguration declare only InvalidParameterValueException, and
// DescribeConfigRules declares InvalidNextTokenException specifically for a bad token.
func TestAWSConfigHandler_UndeclaredValidationCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body         any
		name         string
		operation    string
		wantContains string
	}{
		{
			name:         "delete_aggregation_authorization_missing_account",
			operation:    "DeleteAggregationAuthorization",
			body:         map[string]any{},
			wantContains: "InvalidParameterValueException",
		},
		{
			name:         "put_configuration_aggregator_missing_name",
			operation:    "PutConfigurationAggregator",
			body:         map[string]any{},
			wantContains: "InvalidParameterValueException",
		},
		{
			name:         "delete_pending_aggregation_request_missing_account",
			operation:    "DeletePendingAggregationRequest",
			body:         map[string]any{},
			wantContains: "InvalidParameterValueException",
		},
		{
			name:         "put_config_rule_missing_name",
			operation:    "PutConfigRule",
			body:         map[string]any{"ConfigRule": map[string]any{}},
			wantContains: "InvalidParameterValueException",
		},
		{
			name:         "put_conformance_pack_missing_name",
			operation:    "PutConformancePack",
			body:         map[string]any{},
			wantContains: "InvalidParameterValueException",
		},
		{
			name:         "start_remediation_execution_missing_rule_name",
			operation:    "StartRemediationExecution",
			body:         map[string]any{},
			wantContains: "InvalidParameterValueException",
		},
		{
			name:         "put_retention_configuration_missing_name",
			operation:    "PutRetentionConfiguration",
			body:         map[string]any{},
			wantContains: "InvalidParameterValueException",
		},
		{
			name:         "describe_config_rules_invalid_next_token",
			operation:    "DescribeConfigRules",
			body:         map[string]any{"NextToken": "not-base64!!"},
			wantContains: "InvalidNextTokenException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			rec := doAWSConfigRequest(t, h, tt.operation, tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
			assert.NotContains(t, rec.Body.String(), "\"__type\":\"ValidationException\"")
		})
	}
}

// TestAWSConfigHandler_ErrorTypes_NoFittingCode locks in the landmine sites: an op
// whose declared error model has neither ValidationException nor any other
// validation-shaped code for a required-field check. These stay on the generic
// ErrValidation sentinel (documented as such at each call site) since no declared
// alternative fits -- this test exists so a future swap-to-a-wrong-code regression
// fails loudly rather than silently, not to claim the current code is correct AWS
// parity.
func TestAWSConfigHandler_ErrorTypes_NoFittingCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body      any
		name      string
		operation string
	}{
		{
			name:      "delete_configuration_aggregator_missing_name",
			operation: "DeleteConfigurationAggregator",
			body:      map[string]any{},
		},
		{name: "delete_config_rule_missing_name", operation: "DeleteConfigRule", body: map[string]any{}},
		{
			name:      "delete_evaluation_results_missing_name",
			operation: "DeleteEvaluationResults",
			body:      map[string]any{},
		},
		{
			name:      "start_configuration_recorder_missing_name",
			operation: "StartConfigurationRecorder",
			body:      map[string]any{},
		},
		{
			name:      "stop_configuration_recorder_missing_name",
			operation: "StopConfigurationRecorder",
			body:      map[string]any{},
		},
		{
			name:      "delete_configuration_recorder_missing_name",
			operation: "DeleteConfigurationRecorder",
			body:      map[string]any{},
		},
		{
			name:      "delete_conformance_pack_missing_name",
			operation: "DeleteConformancePack",
			body:      map[string]any{},
		},
		{
			name:      "put_delivery_channel_missing_bucket",
			operation: "PutDeliveryChannel",
			body:      map[string]any{"DeliveryChannel": map[string]any{"name": "ch1"}},
		},
		{
			name:      "delete_delivery_channel_missing_name",
			operation: "DeleteDeliveryChannel",
			body:      map[string]any{},
		},
		{
			name:      "delete_organization_config_rule_missing_name",
			operation: "DeleteOrganizationConfigRule",
			body:      map[string]any{},
		},
		{
			name:      "delete_organization_conformance_pack_missing_name",
			operation: "DeleteOrganizationConformancePack",
			body:      map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			rec := doAWSConfigRequest(t, h, tt.operation, tt.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "ValidationException")
		})
	}
}

func TestAWSConfigHandler_ErrorTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body         any
		name         string
		operation    string
		wantContains string
		wantCode     int
	}{
		{
			name:         "delete_configuration_recorder_not_found_type",
			operation:    "DeleteConfigurationRecorder",
			body:         map[string]any{"ConfigurationRecorderName": "nonexistent"},
			wantCode:     http.StatusNotFound,
			wantContains: "NoSuchConfigurationRecorderException",
		},
		{
			name:         "delete_delivery_channel_not_found_type",
			operation:    "DeleteDeliveryChannel",
			body:         map[string]any{"DeliveryChannelName": "nonexistent"},
			wantCode:     http.StatusNotFound,
			wantContains: "NoSuchDeliveryChannelException",
		},
		{
			name:         "delete_config_rule_not_found_type",
			operation:    "DeleteConfigRule",
			body:         map[string]any{"ConfigRuleName": "nonexistent"},
			wantCode:     http.StatusNotFound,
			wantContains: "NoSuchConfigRuleException",
		},
		{
			name:         "delete_aggregator_not_found_type",
			operation:    "DeleteConfigurationAggregator",
			body:         map[string]any{"ConfigurationAggregatorName": "nonexistent"},
			wantCode:     http.StatusNotFound,
			wantContains: "NoSuchConfigurationAggregatorException",
		},
		{
			name:         "delete_conformance_pack_not_found_type",
			operation:    "DeleteConformancePack",
			body:         map[string]any{"ConformancePackName": "nonexistent"},
			wantCode:     http.StatusNotFound,
			wantContains: "NoSuchConformancePackException",
		},
		{
			name:         "delete_org_config_rule_not_found_type",
			operation:    "DeleteOrganizationConfigRule",
			body:         map[string]any{"OrganizationConfigRuleName": "nonexistent"},
			wantCode:     http.StatusNotFound,
			wantContains: "NoSuchOrganizationConfigRuleException",
		},
		{
			name:         "delete_org_conformance_pack_not_found_type",
			operation:    "DeleteOrganizationConformancePack",
			body:         map[string]any{"OrganizationConformancePackName": "nonexistent"},
			wantCode:     http.StatusNotFound,
			wantContains: "NoSuchOrganizationConformancePackException",
		},
		{
			name:         "start_recorder_no_delivery_channel_400",
			operation:    "StartConfigurationRecorder",
			body:         map[string]any{"ConfigurationRecorderName": "default"},
			wantCode:     http.StatusBadRequest,
			wantContains: "NoAvailableDeliveryChannelException",
		},
		{
			name:         "get_connector_not_found_type",
			operation:    "GetConnector",
			body:         map[string]any{"Arn": "arn:aws:config:us-east-1:000000000000:connector/nonexistent"},
			wantCode:     http.StatusBadRequest,
			wantContains: "ResourceNotFoundException",
		},
		{
			name:         "delete_connector_not_found_type",
			operation:    "DeleteConnector",
			body:         map[string]any{"Arn": "arn:aws:config:us-east-1:000000000000:connector/nonexistent"},
			wantCode:     http.StatusBadRequest,
			wantContains: "ResourceNotFoundException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestAWSConfigHandler(t)
			if tt.operation == "StartConfigurationRecorder" {
				require.NoError(t, h.Backend.PutConfigurationRecorder("default", "arn:aws:iam::000:role/r", nil))
			}

			rec := doAWSConfigRequest(t, h, tt.operation, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}
