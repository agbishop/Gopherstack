package iot_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/iot"
)

func TestBackend_TopicRuleLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   *iot.CreateTopicRuleInput
		name    string
		wantErr bool
	}{
		{
			name: "create_rule",
			input: &iot.CreateTopicRuleInput{
				RuleName: "TemperatureRule",
				TopicRulePayload: &iot.TopicRulePayload{
					SQL:     "SELECT * FROM 'sensor/temperature' WHERE temperature > 50",
					Actions: []iot.RuleAction{{SQS: &iot.SQSAction{QueueURL: "http://localhost/queue"}}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()

			err := b.CreateTopicRule(tt.input)
			require.NoError(t, err)

			r, getErr := b.GetTopicRule(tt.input.RuleName)
			require.NoError(t, getErr)
			assert.Equal(t, tt.input.RuleName, r.RuleName)
			assert.Equal(t, tt.input.TopicRulePayload.SQL, r.SQL)
			assert.True(t, r.Enabled)

			rules := b.ListTopicRules()
			assert.Len(t, rules, 1)

			delErr := b.DeleteTopicRule(tt.input.RuleName)
			require.NoError(t, delErr)

			_, getErr2 := b.GetTopicRule(tt.input.RuleName)
			require.ErrorIs(t, getErr2, iot.ErrRuleNotFound)
		})
	}
}

func TestBackend_GetTopicRule_NotFound(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	_, err := b.GetTopicRule("missing")
	require.ErrorIs(t, err, iot.ErrRuleNotFound)
}

func TestBackend_PolicyLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input *iot.CreatePolicyInput
		name  string
	}{
		{
			name: "create_policy",
			input: &iot.CreatePolicyInput{
				PolicyName:     "AllowAll",
				PolicyDocument: `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"iot:*","Resource":"*"}]}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := iot.NewInMemoryBackend()

			out, err := b.CreatePolicy(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.input.PolicyName, out.PolicyName)
			assert.NotEmpty(t, out.PolicyARN)
			assert.Equal(t, tt.input.PolicyDocument, out.PolicyDocument)

			attachErr := b.AttachPrincipalPolicy(&iot.AttachPrincipalPolicyInput{
				PolicyName: tt.input.PolicyName,
				Principal:  "arn:aws:iot:us-east-1:000000000000:cert/abc123",
			})
			require.NoError(t, attachErr)
		})
	}
}

func TestBackend_DescribeEndpoint(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackendWithConfig("123456789012", "eu-west-1")

	tests := []struct {
		name         string
		endpointType string
	}{
		{name: "data_ats", endpointType: "iot:Data-ATS"},
		{name: "data", endpointType: "iot:Data"},
		{name: "empty", endpointType: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, err := b.DescribeEndpoint(tt.endpointType)
			require.NoError(t, err)
			assert.NotEmpty(t, out.EndpointAddress)
		})
	}
}

func TestBackend_GetRules(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()

	_ = b.CreateTopicRule(&iot.CreateTopicRuleInput{
		RuleName:         "RuleA",
		TopicRulePayload: &iot.TopicRulePayload{SQL: "SELECT * FROM 'a/#'"},
	})
	_ = b.CreateTopicRule(&iot.CreateTopicRuleInput{
		RuleName:         "RuleB",
		TopicRulePayload: &iot.TopicRulePayload{SQL: "SELECT * FROM 'b/#'"},
	})

	rules := b.GetRules()
	assert.Len(t, rules, 2)
}

func TestBackend_SetRuleDispatcher(t *testing.T) {
	t.Parallel()

	b := iot.NewInMemoryBackend()
	assert.Nil(t, b.GetDispatcher())

	d := &mockDispatcher{}
	b.SetRuleDispatcher(d)
	assert.Equal(t, d, b.GetDispatcher())
}

// mockDispatcher is a test implementation of RuleDispatcher.
type mockDispatcher struct{}

func (m *mockDispatcher) SendToSQS(_, _ string) error { return nil }

func (m *mockDispatcher) InvokeLambda(_ context.Context, _ string, _ []byte) error {
	return nil
}

// TestReset verifies that Reset clears all backend state.
func TestReset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		populate func(b *iot.InMemoryBackend)
		name     string
	}{
		{name: "empty_backend"},
		{
			name: "populated_backend",
			populate: func(b *iot.InMemoryBackend) {
				b.AddThingInternal(iot.Thing{ThingName: "t1"})
				b.AddPolicyInternal(iot.Policy{PolicyName: "p1"})
				b.AddRuleInternal(iot.TopicRule{RuleName: "r1"})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()
			if tt.populate != nil {
				tt.populate(b)
			}

			b.Reset()

			assert.Equal(t, 0, b.ThingCount())
			assert.Equal(t, 0, b.PolicyCount())
			assert.Equal(t, 0, b.RuleCount())
		})
	}
}

// TestReset_ShadowsCleared verifies that Reset() clears device shadow state.
// Shadow lookups are gated on the owning thing existing, so the test re-creates
// a thing under the same name after Reset to prove a stale shadow doesn't
// silently reattach to it.
func TestReset_ShadowsCleared(t *testing.T) {
	t.Parallel()

	b := newRefBackend()

	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "t1"})
	require.NoError(t, err)

	_, err = b.UpdateThingShadow("t1", "", map[string]any{"reported": map[string]any{"on": true}})
	require.NoError(t, err)

	shadow, err := b.GetThingShadow("t1", "")
	require.NoError(t, err)
	require.NotNil(t, shadow)

	b.Reset()

	_, err = b.CreateThing(&iot.CreateThingInput{ThingName: "t1"})
	require.NoError(t, err)

	_, err = b.GetThingShadow("t1", "")
	assert.ErrorIs(t, err, iot.ErrShadowNotFound,
		"shadow must not survive Reset, even after the owning thing is re-created")
}

// TestMultipleResetCycle verifies Reset can be called multiple times.
func TestMultipleResetCycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cycles int
	}{
		{name: "single_reset", cycles: 1},
		{name: "triple_reset", cycles: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()
			b.AddThingInternal(iot.Thing{ThingName: "t1"})

			for range tt.cycles {
				b.Reset()
			}

			assert.Equal(t, 0, b.ThingCount())
		})
	}
}

// TestHandlerReset verifies Handler.Reset delegates to the backend.
func TestHandlerReset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "reset_clears_backend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newRefHandler()
			b.AddThingInternal(iot.Thing{ThingName: "thing-before-reset"})
			assert.Equal(t, 1, b.ThingCount())

			h.Reset()

			assert.Equal(t, 0, b.ThingCount())
		})
	}
}

// TestGetSupportedOperations_AllOps verifies the supported operation list.
func TestGetSupportedOperations_AllOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantOp  string
		wantMin int
	}{
		{name: "has_create_thing", wantOp: "CreateThing"},
		{name: "has_attach_policy", wantOp: "AttachPolicy"},
		{name: "has_cancel_audit_task", wantOp: "CancelAuditTask"},
		{name: "has_at_least_19_ops", wantMin: 19},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newRefHandler()
			ops := h.GetSupportedOperations()

			if tt.wantOp != "" {
				assert.Contains(t, ops, tt.wantOp)
			}

			if tt.wantMin > 0 {
				assert.GreaterOrEqual(t, len(ops), tt.wantMin)
			}
		})
	}
}

// TestHandlerOpsLen verifies the HandlerOpsLen export helper.
func TestHandlerOpsLen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		wantMin int
	}{
		{name: "ops_len_at_least_19", wantMin: 19},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newRefHandler()
			assert.GreaterOrEqual(t, iot.HandlerOpsLen(h), tt.wantMin)
		})
	}
}

// TestSeedHelpers verifies AddThingInternal / AddPolicyInternal / AddRuleInternal.
func TestSeedHelpers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		thingName    string
		policyName   string
		ruleName     string
		wantThings   int
		wantPolicies int
		wantRules    int
	}{
		{
			name:      "seed_one_of_each",
			thingName: "t1", policyName: "p1", ruleName: "r1",
			wantThings: 1, wantPolicies: 1, wantRules: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()
			b.AddThingInternal(iot.Thing{ThingName: tt.thingName})
			b.AddPolicyInternal(iot.Policy{PolicyName: tt.policyName})
			b.AddRuleInternal(iot.TopicRule{RuleName: tt.ruleName})

			assert.Equal(t, tt.wantThings, b.ThingCount())
			assert.Equal(t, tt.wantPolicies, b.PolicyCount())
			assert.Equal(t, tt.wantRules, b.RuleCount())
		})
	}
}

// TestSeedHelper_ARN verifies seed helpers generate ARNs automatically.
func TestSeedHelper_ARN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "thing_arn_auto_generated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()
			b.AddThingInternal(iot.Thing{ThingName: "auto-arn-thing"})

			thing, err := b.DescribeThing("auto-arn-thing")
			require.NoError(t, err)
			assert.Contains(t, thing.ARN, "arn:aws:iot:")
			assert.Contains(t, thing.ARN, "auto-arn-thing")
		})
	}
}

// TestNonNilActions verifies TopicRule.Actions is never nil.
func TestNonNilActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "no_actions_gives_empty_slice"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()

			err := b.CreateTopicRule(&iot.CreateTopicRuleInput{
				RuleName:         "NoActionsRule",
				TopicRulePayload: &iot.TopicRulePayload{SQL: "SELECT temperature FROM 'devices/#'"},
			})
			require.NoError(t, err)

			rule, err := b.GetTopicRule("NoActionsRule")
			require.NoError(t, err)
			assert.NotNil(t, rule.Actions)
		})
	}
}

// TestCertTransferCount verifies that TransferCertificate populates the
// pending-transfer count and AcceptCertificateTransfer/
// RejectCertificateTransfer/CancelCertificateTransfer consume it. Regression
// test for gopherstack-ep0r: AcceptCertificateTransfer previously wrote an
// entry into the pending-transfer map on *any* certificate ID (even unknown
// ones) instead of validating and consuming an existing pending transfer.
func TestCertTransferCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		numCerts int
		want     int
	}{
		{name: "two_certs", numCerts: 2, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()

			certIDs := make([]string, 0, tt.numCerts)
			for range tt.numCerts {
				cert, err := b.RegisterCertificate(&iot.RegisterCertificateInput{Status: "ACTIVE"})
				require.NoError(t, err)
				require.NoError(t, b.TransferCertificate(cert.CertificateID, "999999999999", ""))
				certIDs = append(certIDs, cert.CertificateID)
			}

			assert.Equal(t, tt.want, b.CertTransferCount())

			// Accepting a pending transfer consumes it.
			require.NoError(t, b.AcceptCertificateTransfer(&iot.AcceptCertificateTransferInput{
				CertificateID: certIDs[0],
				SetAsActive:   true,
			}))
			assert.Equal(t, tt.want-1, b.CertTransferCount())

			// Accepting an unknown certificate ID fails and does not add an entry.
			err := b.AcceptCertificateTransfer(&iot.AcceptCertificateTransferInput{CertificateID: "does-not-exist"})
			require.Error(t, err)
			assert.Equal(t, tt.want-1, b.CertTransferCount())
		})
	}
}

// TestChaosMetadata verifies chaos-related metadata methods.
func TestChaosMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "chaos_service_name_is_iot"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newRefHandler()
			assert.Equal(t, "iot", h.ChaosServiceName())
			assert.NotEmpty(t, h.ChaosOperations())
			assert.NotEmpty(t, h.ChaosRegions())
		})
	}
}

// TestMQTTPort verifies the MQTT port is returned.
func TestMQTTPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantPort int
	}{
		{name: "default_mqtt_port", wantPort: 1883},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newRefBackend()
			assert.Equal(t, tt.wantPort, b.MQTTPort())
		})
	}
}

func TestGetSupportedOperations_IncludesCoreOps(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	ops := h.GetSupportedOperations()

	expected := []string{
		"GetPolicy", "DeletePolicy", "ListPolicies",
		"DisableTopicRule", "EnableTopicRule", "ReplaceTopicRule",
		"UpdateThing", "ListThings", "ListTopicRules", "ListThingPrincipals",
	}

	for _, exp := range expected {
		assert.Contains(t, ops, exp, "missing operation: %s", exp)
	}
}

func TestReset_MultipleTimesIsIdempotent(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	_, err := b.CreateThing(&iot.CreateThingInput{ThingName: "multi-reset-thing"})
	require.NoError(t, err)

	b.Reset()
	b.Reset()
	b.Reset()

	assert.Empty(t, b.ListThings())
}

func TestDescribeEndpoint_ReturnsAddress(t *testing.T) {
	t.Parallel()

	_, b := newR3Handler()
	out, err := b.DescribeEndpoint("")
	require.NoError(t, err)
	assert.NotEmpty(t, out.EndpointAddress)
}
