package stepfunctions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/stepfunctions"
)

// ─── ARN Format ───────────────────────────────────────────────────────────────

func createSFNStateMachineCov(
	ctx context.Context,
	t *testing.T,
	h *stepfunctions.Handler,
	e *echo.Echo,
	name string,
) string {
	t.Helper()

	rec := sfnPost(ctx, t, h, e, "CreateStateMachine", makeSMBody(name, validPassDef, "STANDARD"))
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	arn, _ := resp["stateMachineArn"].(string)
	require.NotEmpty(t, arn)

	return arn
}

func TestSFN_StateMachineAliases(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h, e := newSFNHandler(t)
	smARN := createSFNStateMachineCov(ctx, t, h, e, "alias-sm")

	// CreateStateMachineAlias
	rec := sfnPost(ctx, t, h, e, "CreateStateMachineAlias", fmt.Sprintf(`{
		"name": "my-alias",
		"routingConfiguration": [{"stateMachineVersionArn": "%s", "weight": 100}]
	}`, smARN))
	assert.Positive(t, rec.Code)

	// ListStateMachineAliases
	rec = sfnPost(ctx, t, h, e, "ListStateMachineAliases", fmt.Sprintf(`{"stateMachineArn": "%s"}`, smARN))
	assert.Positive(t, rec.Code)
}

func TestAlias_CreateAndDescribe(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"alias-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	v, err := b.PublishStateMachineVersion(sm.StateMachineArn, "", "")
	require.NoError(t, err)

	routing := []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: v.StateMachineVersionArn, Weight: 100},
	}
	alias, err := b.CreateStateMachineAlias(sm.StateMachineArn, "live", "prod alias", routing)
	require.NoError(t, err)
	assert.NotEmpty(t, alias.StateMachineAliasArn)
	assert.Equal(t, "live", alias.Name)

	got, err := b.DescribeStateMachineAlias(alias.StateMachineAliasArn)
	require.NoError(t, err)
	assert.Equal(t, "live", got.Name)
}

func TestAlias_ListAliases(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"list-alias-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	v, err := b.PublishStateMachineVersion(sm.StateMachineArn, "", "")
	require.NoError(t, err)

	routing := []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: v.StateMachineVersionArn, Weight: 100},
	}

	for _, name := range []string{"staging", "production"} {
		_, err = b.CreateStateMachineAlias(sm.StateMachineArn, name, "", routing)
		require.NoError(t, err)
	}

	aliases, _, err := b.ListStateMachineAliases(sm.StateMachineArn, "", 100)
	require.NoError(t, err)
	assert.Len(t, aliases, 2)
}

func TestAlias_Delete(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"del-alias-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	v, err := b.PublishStateMachineVersion(sm.StateMachineArn, "", "")
	require.NoError(t, err)

	routing := []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: v.StateMachineVersionArn, Weight: 100},
	}
	alias, err := b.CreateStateMachineAlias(sm.StateMachineArn, "del-alias", "", routing)
	require.NoError(t, err)

	require.NoError(t, b.DeleteStateMachineAlias(alias.StateMachineAliasArn))

	_, err = b.DescribeStateMachineAlias(alias.StateMachineAliasArn)
	require.Error(t, err)
	assert.ErrorIs(t, err, stepfunctions.ErrStateMachineAliasDoesNotExist)
}

// ─── Tags ─────────────────────────────────────────────────────────────────────

func TestCreateStateMachineAlias_RoutingWeightsMustSum100(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		routing []stepfunctions.AliasRoutingConfig
		wantErr bool
	}{
		{
			name: "single_entry_weight_100_ok",
			routing: []stepfunctions.AliasRoutingConfig{
				{StateMachineVersionArn: "v1-arn", Weight: 100},
			},
			wantErr: false,
		},
		{
			name: "two_entries_sum_100_ok",
			routing: []stepfunctions.AliasRoutingConfig{
				{StateMachineVersionArn: "v1-arn", Weight: 80},
				{StateMachineVersionArn: "v2-arn", Weight: 20},
			},
			wantErr: false,
		},
		{
			name: "single_entry_weight_50_error",
			routing: []stepfunctions.AliasRoutingConfig{
				{StateMachineVersionArn: "v1-arn", Weight: 50},
			},
			wantErr: true,
		},
		{
			name: "two_entries_sum_not_100_error",
			routing: []stepfunctions.AliasRoutingConfig{
				{StateMachineVersionArn: "v1-arn", Weight: 60},
				{StateMachineVersionArn: "v2-arn", Weight: 60},
			},
			wantErr: true,
		},
		{
			name:    "empty_routing_error",
			routing: []stepfunctions.AliasRoutingConfig{},
			wantErr: true,
		},
		{
			name: "three_entries_error",
			routing: []stepfunctions.AliasRoutingConfig{
				{StateMachineVersionArn: "v1-arn", Weight: 50},
				{StateMachineVersionArn: "v2-arn", Weight: 30},
				{StateMachineVersionArn: "v3-arn", Weight: 20},
			},
			wantErr: true,
		},
		{
			name: "weight_over_100_error",
			routing: []stepfunctions.AliasRoutingConfig{
				{StateMachineVersionArn: "v1-arn", Weight: 101},
			},
			wantErr: true,
		},
		{
			name: "negative_weight_error",
			routing: []stepfunctions.AliasRoutingConfig{
				{StateMachineVersionArn: "v1-arn", Weight: -1},
				{StateMachineVersionArn: "v2-arn", Weight: 101},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := stepfunctions.NewInMemoryBackend()
			sm, err := b.CreateStateMachine(
				context.Background(),
				"alias-weight-sm-"+tt.name[:min(len(tt.name), 20)],
				minimalDefinition,
				validRoleARN,
				"STANDARD",
			)
			require.NoError(t, err)

			_, err = b.CreateStateMachineAlias(
				sm.StateMachineArn,
				"my-alias-"+tt.name[:min(len(tt.name), 10)],
				"",
				tt.routing,
			)
			if tt.wantErr {
				require.ErrorIs(t, err, stepfunctions.ErrInvalidRoutingConfiguration)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpdateStateMachineAlias_RoutingWeightsMustSum100(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"alias-update-weight-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	v, err := b.PublishStateMachineVersion(sm.StateMachineArn, "v1", "")
	require.NoError(t, err)

	alias, err := b.CreateStateMachineAlias(sm.StateMachineArn, "stable", "", []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: v.StateMachineVersionArn, Weight: 100},
	})
	require.NoError(t, err)

	// Update with weights that don't sum to 100 → error.
	_, err = b.UpdateStateMachineAlias(alias.StateMachineAliasArn, "", []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: v.StateMachineVersionArn, Weight: 50},
	})
	require.ErrorIs(t, err, stepfunctions.ErrInvalidRoutingConfiguration)

	// Update with valid weights → succeeds.
	_, err = b.UpdateStateMachineAlias(alias.StateMachineAliasArn, "", []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: v.StateMachineVersionArn, Weight: 100},
	})
	require.NoError(t, err)
}

func TestCreateStateMachineAlias_RoutingViaHandler(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h, e := newSFNHandler(t)
	smARN := createSM(ctx, t, h, e, "alias-routing-handler-sm")

	// Create alias with weights not summing to 100 → 400.
	body, err := json.Marshal(map[string]any{
		"stateMachineArn": smARN,
		"name":            "bad-alias",
		"routingConfiguration": []map[string]any{
			{"stateMachineVersionArn": "arn:fake:version:1", "weight": 50},
		},
	})
	require.NoError(t, err)

	rec := sfnPost(ctx, t, h, e, "CreateStateMachineAlias", string(body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	// CHANGED (orphan-code sweep, gopherstack-yatn): this assertion previously
	// checked for "InvalidRoutingConfiguration", which is not a real sfn SDK
	// error type -- CreateStateMachineAlias's own deserializeOpError switch
	// (sfn@v1.45.4 deserializers.go) declares ValidationException, not this.
	// The old assertion just confirmed the bug, not correct behavior; see
	// TestCreateStateMachineAlias_InvalidRoutingConfig_RealClient for the
	// real-client proof.
	assert.Equal(t, "ValidationException", resp["__type"])
}

// ─── Tag Validation ───────────────────────────────────────────────────────────
