package stepfunctions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/stepfunctions"
)

// ─── ARN Format ───────────────────────────────────────────────────────────────

func TestARN_Version(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackendWithConfig("123456789012", "us-east-1")
	sm, err := b.CreateStateMachine(
		context.Background(),
		"ver-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	v, err := b.PublishStateMachineVersion(sm.StateMachineArn, "v1", "")
	require.NoError(t, err)
	assert.Contains(
		t,
		v.StateMachineVersionArn,
		"arn:aws:states:us-east-1:123456789012:stateMachine:ver-sm:",
	)
}

// ─── StateMachine CRUD ────────────────────────────────────────────────────────

func TestVersion_PublishAndDescribe(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"ver-pub-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	v, err := b.PublishStateMachineVersion(sm.StateMachineArn, "initial release", "")
	require.NoError(t, err)
	assert.NotEmpty(t, v.StateMachineVersionArn)
	assert.Equal(t, sm.StateMachineArn, v.StateMachineArn)
	assert.Equal(t, "initial release", v.Description)

	got, err := b.DescribeStateMachineVersion(v.StateMachineVersionArn)
	require.NoError(t, err)
	assert.Equal(t, v.StateMachineVersionArn, got.StateMachineVersionArn)
}

func TestVersion_ListVersions(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"list-ver-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	for i := range 3 {
		_, err = b.PublishStateMachineVersion(sm.StateMachineArn, fmt.Sprintf("v%d", i+1), "")
		require.NoError(t, err)
	}

	versions, _, err := b.ListStateMachineVersions(sm.StateMachineArn, "", 100)
	require.NoError(t, err)
	assert.Len(t, versions, 3)
}

func TestVersion_Delete(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"del-ver-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	v, err := b.PublishStateMachineVersion(sm.StateMachineArn, "", "")
	require.NoError(t, err)

	require.NoError(t, b.DeleteStateMachineVersion(v.StateMachineVersionArn))

	_, err = b.DescribeStateMachineVersion(v.StateMachineVersionArn)
	require.Error(t, err)
	assert.ErrorIs(t, err, stepfunctions.ErrStateMachineVersionDoesNotExist)
}

// TestVersion_Delete_RejectedWhileReferencedByAlias locks real AWS's
// DeleteStateMachineVersion doc comment: "You can't delete a state machine
// version currently referenced by one or more aliases. Before you delete a
// version, you must either delete the aliases or update them....
func TestVersion_Delete_RejectedWhileReferencedByAlias(t *testing.T) {
	t.Parallel()

	b := stepfunctions.NewInMemoryBackend()
	sm, err := b.CreateStateMachine(
		context.Background(),
		"del-ver-aliased-sm",
		minimalDefinition,
		validRoleARN,
		"STANDARD",
	)
	require.NoError(t, err)

	v, err := b.PublishStateMachineVersion(sm.StateMachineArn, "", "")
	require.NoError(t, err)

	alias, err := b.CreateStateMachineAlias(sm.StateMachineArn, "live", "", []stepfunctions.AliasRoutingConfig{
		{StateMachineVersionArn: v.StateMachineVersionArn, Weight: 100},
	})
	require.NoError(t, err)

	err = b.DeleteStateMachineVersion(v.StateMachineVersionArn)
	require.ErrorIs(t, err, stepfunctions.ErrStateMachineVersionReferencedByAlias)

	_, err = b.DescribeStateMachineVersion(v.StateMachineVersionArn)
	require.NoError(t, err, "version must still exist after the rejected delete")

	require.NoError(t, b.DeleteStateMachineAlias(alias.StateMachineAliasArn))

	require.NoError(t, b.DeleteStateMachineVersion(v.StateMachineVersionArn))
}

// ─── Aliases ──────────────────────────────────────────────────────────────────

func TestHandler_ListStateMachineVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "success_empty_list",
			body:     "", // will be filled in using created SM ARN
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)
			smARN := createSM(ctx, t, h, e, "ver-sm-"+tt.name)

			body := tt.body
			if body == "" {
				b, err := json.Marshal(map[string]string{"stateMachineArn": smARN})
				require.NoError(t, err)
				body = string(b)
			}

			rec := sfnPost(ctx, t, h, e, "ListStateMachineVersions", body)
			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Contains(t, resp, "stateMachineVersions")
		})
	}
}

// ---- UpdateStateMachine ----

func TestStateMachineVersions_Lifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		publishN    int
		deleteFirst bool
	}{
		{
			name:     "publish_two_versions",
			publishN: 2,
		},
		{
			name:        "publish_and_delete_version",
			publishN:    1,
			deleteFirst: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newSFBackend()
			sm, err := b.CreateStateMachine(
				context.Background(),
				"ver-sm-"+tt.name,
				exprPassDef,
				"arn:role",
				"STANDARD",
			)
			require.NoError(t, err)

			var lastVersionARN string

			for i := range tt.publishN {
				v, pubErr := b.PublishStateMachineVersion(sm.StateMachineArn,
					"version "+string(rune('A'+i)), "")
				require.NoError(t, pubErr)
				require.NotEmpty(t, v.StateMachineVersionArn)
				assert.Contains(t, v.StateMachineVersionArn, sm.StateMachineArn)
				lastVersionARN = v.StateMachineVersionArn
			}

			// List versions.
			versions, _, err := b.ListStateMachineVersions(sm.StateMachineArn, "", 100)
			require.NoError(t, err)
			assert.Len(t, versions, tt.publishN)

			// Describe version.
			described, err := b.DescribeStateMachineVersion(lastVersionARN)
			require.NoError(t, err)
			assert.Equal(t, sm.StateMachineArn, described.StateMachineArn)
			assert.JSONEq(t, exprPassDef, described.Definition)

			if tt.deleteFirst {
				err = b.DeleteStateMachineVersion(lastVersionARN)
				require.NoError(t, err)

				versions, _, err = b.ListStateMachineVersions(sm.StateMachineArn, "", 100)
				require.NoError(t, err)
				assert.Empty(t, versions)

				_, err = b.DescribeStateMachineVersion(lastVersionARN)
				require.ErrorIs(t, err, stepfunctions.ErrStateMachineVersionDoesNotExist)
			}
		})
	}
}

// TestRefinement1_ListStateMachineVersions tests the ListStateMachineVersions handler.
func TestListStateMachineVersions(t *testing.T) {
	t.Parallel()

	h, e := newSFNHandler(t)

	// Create a state machine first so ListStateMachineVersions can find it.
	createBody, err := json.Marshal(map[string]string{
		"name":       "ver-sm",
		"definition": validPassDef,
		"roleArn":    "arn:aws:iam::000:role/r",
		"type":       "STANDARD",
	})
	require.NoError(t, err)

	createRec := sfnPost(t.Context(), t, h, e, "CreateStateMachine", string(createBody))
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	smARN := createResp["stateMachineArn"].(string)

	listBody, merr := json.Marshal(map[string]string{"stateMachineArn": smARN})
	require.NoError(t, merr)

	rec := sfnPost(t.Context(), t, h, e, "ListStateMachineVersions", string(listBody))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	versions, ok := resp["stateMachineVersions"].([]any)
	require.True(t, ok, "stateMachineVersions should be an array")
	assert.Empty(t, versions)
}
