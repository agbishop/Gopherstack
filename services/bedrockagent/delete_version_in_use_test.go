package bedrockagent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/bedrockagent"
)

// TestDeleteAgentVersion_BlockedWhileAliasReferencesIt guards
// api_op_DeleteAgentVersion.go's documented precondition: "By default, this
// value is false and deletion is stopped if the resource is in use. If you
// set it to true, the resource will be deleted even if the resource is in
// use." An agent version is in use when an alias's routingConfiguration
// still points at it.
func TestDeleteAgentVersion_BlockedWhileAliasReferencesIt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cases := []struct {
		name    string
		skip    bool
		wantErr bool
	}{
		{name: "blocked by default", skip: false, wantErr: true},
		{name: "skip flag deletes anyway", skip: true, wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := bedrockagent.NewTestBackend("us-east-1", "123456789012")

			agent, err := b.CreateAgent(ctx, bedrockagent.AgentConfig{
				AgentName:       "in-use-agent",
				FoundationModel: "anthropic.claude-v2",
				RoleARN:         "arn:aws:iam::123456789012:role/BedrockRole",
			})
			require.NoError(t, err)

			alias, err := b.CreateAgentAlias(ctx, agent.AgentID, bedrockagent.AliasConfig{
				AliasName: "in-use-alias",
			})
			require.NoError(t, err)
			version := alias.RoutingConfiguration[0].AgentVersion

			err = b.DeleteAgentVersion(ctx, agent.AgentID, version, tc.skip)

			if !tc.wantErr {
				require.NoError(t, err)
				_, getErr := b.GetAgentVersion(ctx, agent.AgentID, version)
				assert.ErrorIs(t, getErr, bedrockagent.ErrNotFound)

				return
			}

			require.Error(t, err)
			require.ErrorIs(t, err, bedrockagent.ErrResourceInUse)

			_, getErr := b.GetAgentVersion(ctx, agent.AgentID, version)
			assert.NoError(t, getErr, "version must survive a blocked delete")
		})
	}
}

// TestDeleteFlowVersion_BlockedWhileAliasReferencesIt mirrors the agent
// version case for api_op_DeleteFlowVersion.go's identical
// skipResourceInUseCheck precondition, checked against a flow alias's
// routingConfiguration.
func TestDeleteFlowVersion_BlockedWhileAliasReferencesIt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cases := []struct {
		name    string
		skip    bool
		wantErr bool
	}{
		{name: "blocked by default", skip: false, wantErr: true},
		{name: "skip flag deletes anyway", skip: true, wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := bedrockagent.NewTestBackend("us-east-1", "123456789012")

			flow, err := b.CreateFlow(ctx, bedrockagent.FlowConfig{
				Name:    "in-use-flow",
				RoleARN: "arn:aws:iam::123456789012:role/FlowRole",
			})
			require.NoError(t, err)

			fv, err := b.CreateFlowVersion(ctx, flow.FlowID, "")
			require.NoError(t, err)

			_, err = b.CreateFlowAlias(ctx, flow.FlowID, bedrockagent.FlowAliasConfig{
				Name:                 "in-use-alias",
				RoutingConfiguration: []bedrockagent.FlowAliasRouting{{FlowVersion: fv.Version}},
			})
			require.NoError(t, err)

			err = b.DeleteFlowVersion(ctx, flow.FlowID, fv.Version, tc.skip)

			if !tc.wantErr {
				require.NoError(t, err)
				_, getErr := b.GetFlowVersion(ctx, flow.FlowID, fv.Version)
				assert.ErrorIs(t, getErr, bedrockagent.ErrNotFound)

				return
			}

			require.Error(t, err)
			require.ErrorIs(t, err, bedrockagent.ErrResourceInUse)

			_, getErr := b.GetFlowVersion(ctx, flow.FlowID, fv.Version)
			assert.NoError(t, getErr, "version must survive a blocked delete")
		})
	}
}

// TestHandleDeleteAgentVersion_ConflictOverHTTP confirms the HTTP layer
// parses the skipResourceInUseCheck query parameter (api_op_DeleteAgentVersion
// serializes it as a query bool, serializers.go:~1657) and surfaces a real
// 409 ConflictException rather than a 500 -- handleErr must map
// awserr.ErrConflict, which was previously unhandled and would have fallen
// through to InternalServerException.
func TestHandleDeleteAgentVersion_ConflictOverHTTP(t *testing.T) {
	t.Parallel()

	h, e := setupHandler(t)

	createRec := doRequest(t, h, e, http.MethodPut, "/agents", map[string]any{
		"agentName":            "http-in-use-agent",
		"foundationModel":      "anthropic.claude-v2",
		"agentResourceRoleArn": "arn:aws:iam::123456789012:role/BedrockRole",
	})
	require.Equal(t, http.StatusOK, createRec.Code, createRec.Body.String())

	var created map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))
	agentWrap, _ := created["agent"].(map[string]any)
	agentID, _ := agentWrap["agentId"].(string)
	require.NotEmpty(t, agentID)

	aliasRec := doRequest(t, h, e, http.MethodPut, "/agents/"+agentID+"/agentaliases", map[string]any{
		"agentAliasName": "http-in-use-alias",
	})
	require.Equal(t, http.StatusOK, aliasRec.Code, aliasRec.Body.String())

	var aliasBody map[string]any
	require.NoError(t, json.Unmarshal(aliasRec.Body.Bytes(), &aliasBody))
	aliasWrap, _ := aliasBody["agentAlias"].(map[string]any)
	routing, _ := aliasWrap["routingConfiguration"].([]any)
	require.NotEmpty(t, routing)
	firstRoute, _ := routing[0].(map[string]any)
	version, _ := firstRoute["agentVersion"].(string)
	require.NotEmpty(t, version)

	blockedRec := doRequest(
		t, h, e, http.MethodDelete, "/agents/"+agentID+"/agentversions/"+version, nil,
	)
	require.Equal(t, http.StatusConflict, blockedRec.Code, blockedRec.Body.String())
	assert.Equal(t, "ConflictException", blockedRec.Header().Get("X-Amzn-Errortype"))

	skipRec := doRequest(
		t, h, e, http.MethodDelete,
		"/agents/"+agentID+"/agentversions/"+version+"?skipResourceInUseCheck=true", nil,
	)
	require.Equal(t, http.StatusOK, skipRec.Code, skipRec.Body.String())
}
