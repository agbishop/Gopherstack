package stepfunctions_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/stepfunctions"
)

func TestHandler_CreateStateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, ctx context.Context, h *stepfunctions.Handler, e *echo.Echo)
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "success returns ARN containing name",
			body:     makeSMBody("test-sm", validPassDef, "STANDARD"),
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Contains(t, resp["stateMachineArn"].(string), "test-sm")
			},
		},
		{
			name: "duplicate name returns conflict",
			setup: func(t *testing.T, ctx context.Context, h *stepfunctions.Handler, e *echo.Echo) {
				t.Helper()

				sfnPost(ctx, t, h, e, "CreateStateMachine",
					makeSMBody("dup", validPassDef, ""))
			},
			// Different definition → StateMachineAlreadyExists (same def would be idempotent).
			body:     makeSMBody("dup", `{"StartAt":"T","States":{"T":{"Type":"Succeed"}}}`, ""),
			wantCode: http.StatusConflict,
		},
		{
			name:     "invalid definition returns bad request",
			body:     makeSMBody("invalid-sm", "{}", "STANDARD"),
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)

			if tt.setup != nil {
				tt.setup(t, ctx, h, e)
			}

			rec := sfnPost(ctx, t, h, e, "CreateStateMachine", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.check != nil {
				tt.check(t, rec)
			}
		})
	}
}

func TestHandler_DeleteStateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, ctx context.Context, h *stepfunctions.Handler, e *echo.Echo) string
		bodyFn   func(setupArn string) string
		name     string
		body     string
		wantCode int
	}{
		{
			name: "success deletes existing state machine",
			setup: func(t *testing.T, ctx context.Context, h *stepfunctions.Handler, e *echo.Echo) string {
				t.Helper()

				return createSM(ctx, t, h, e, "del-sm")
			},
			bodyFn:   func(arn string) string { return `{"stateMachineArn":"` + arn + `"}` },
			wantCode: http.StatusOK,
		},
		{
			// AWS: DeleteStateMachine's own error switch models only InvalidArn
			// and ValidationException -- no StateMachineDoesNotExist -- so it is
			// idempotent on a missing state machine.
			name:     "not found is idempotent",
			body:     `{"stateMachineArn":"arn:aws:states:us-east-1:123:stateMachine:nonexistent"}`,
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)

			var setupArn string
			if tt.setup != nil {
				setupArn = tt.setup(t, ctx, h, e)
			}

			body := tt.body
			if tt.bodyFn != nil {
				body = tt.bodyFn(setupArn)
			}

			rec := sfnPost(ctx, t, h, e, "DeleteStateMachine", body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestHandler_StartExecution_StateMachineDeleting is the gopherstack-kx95
// wire-level regression test: while a state machine has a running
// execution, DeleteStateMachine must leave it observable as DELETING
// (checked at the backend level in state_machines_test.go), and a further
// StartExecution against it must emit the actual "StateMachineDeleting"
// __type over the wire, at the HTTP status this handler uses for its
// conflict-family errors -- not merely "some error occurred".
func TestHandler_StartExecution_StateMachineDeleting(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h, e := newSFNHandler(t)

	const waitDef = `{"StartAt":"W","States":{"W":{"Type":"Wait","Seconds":3600,"End":true}}}`

	rec := sfnPost(ctx, t, h, e, "CreateStateMachine", makeSMBody("deleting-wire-sm", waitDef, ""))
	require.Equal(t, http.StatusOK, rec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	smArn, ok := createResp["stateMachineArn"].(string)
	require.True(t, ok)

	execArn := startExec(ctx, t, h, e, smArn, "keep-alive")

	t.Cleanup(func() {
		sfnPost(ctx, t, h, e, "StopExecution", `{"executionArn":"`+execArn+`","error":"Test","cause":"cleanup"}`)
	})

	require.Eventually(t, func() bool {
		descRec := sfnPost(ctx, t, h, e, "DescribeExecution", `{"executionArn":"`+execArn+`"}`)
		if descRec.Code != http.StatusOK {
			return false
		}

		var desc map[string]any

		return json.Unmarshal(descRec.Body.Bytes(), &desc) == nil && desc["status"] == "RUNNING"
	}, 5*time.Second, 10*time.Millisecond)

	delRec := sfnPost(ctx, t, h, e, "DeleteStateMachine", `{"stateMachineArn":"`+smArn+`"}`)
	require.Equal(t, http.StatusOK, delRec.Code)

	descRec := sfnPost(ctx, t, h, e, "DescribeStateMachine", `{"stateMachineArn":"`+smArn+`"}`)
	require.Equal(t, http.StatusOK, descRec.Code, "DELETING state machine must still be describable")

	var desc map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &desc))
	assert.Equal(t, "DELETING", desc["status"])

	startRec := sfnPost(ctx, t, h, e, "StartExecution",
		`{"stateMachineArn":"`+smArn+`","name":"second","input":"{}"}`)
	assert.Equal(t, http.StatusConflict, startRec.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(startRec.Body.Bytes(), &errResp))
	assert.Equal(t, "StateMachineDeleting", errResp["__type"])
}

func TestHandler_ListStateMachines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		smNames   []string
		wantCode  int
		wantCount int
	}{
		{
			name:      "returns all created state machines",
			smNames:   []string{"sm-1", "sm-2"},
			wantCode:  http.StatusOK,
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)

			for _, smName := range tt.smNames {
				createSM(ctx, t, h, e, smName)
			}

			rec := sfnPost(ctx, t, h, e, "ListStateMachines", `{}`)
			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Len(t, resp["stateMachines"].([]any), tt.wantCount)
		})
	}
}

func TestHandler_DescribeStateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, ctx context.Context, h *stepfunctions.Handler, e *echo.Echo) string
		bodyFn   func(setupArn string) string
		check    func(t *testing.T, rec *httptest.ResponseRecorder)
		name     string
		body     string
		wantCode int
	}{
		{
			name: "success returns state machine details",
			setup: func(t *testing.T, ctx context.Context, h *stepfunctions.Handler, e *echo.Echo) string {
				t.Helper()

				rec := sfnPost(ctx, t, h, e, "CreateStateMachine",
					makeSMBody("desc-sm", validPassDef, "EXPRESS"))
				require.Equal(t, http.StatusOK, rec.Code)
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				return resp["stateMachineArn"].(string)
			},
			bodyFn:   func(arn string) string { return `{"stateMachineArn":"` + arn + `"}` },
			wantCode: http.StatusOK,
			check: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				var sm map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sm))
				assert.Equal(t, "EXPRESS", sm["type"])
			},
		},
		{
			name:     "not found returns 404",
			body:     `{"stateMachineArn":"arn:nonexistent"}`,
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)

			var setupArn string
			if tt.setup != nil {
				setupArn = tt.setup(t, ctx, h, e)
			}

			body := tt.body
			if tt.bodyFn != nil {
				body = tt.bodyFn(setupArn)
			}

			rec := sfnPost(ctx, t, h, e, "DescribeStateMachine", body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.check != nil {
				tt.check(t, rec)
			}
		})
	}
}

func TestStateMachineName_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		smName   string
		wantCode int
	}{
		{
			name:     "valid_alphanumeric",
			smName:   "my-state-machine",
			wantCode: http.StatusOK,
		},
		{
			name:     "valid_with_allowed_specials",
			smName:   "sfn_test.flow@prod",
			wantCode: http.StatusOK,
		},
		{
			name:     "valid_max_length",
			smName:   strings.Repeat("a", 80),
			wantCode: http.StatusOK,
		},
		{
			name:     "empty_name_rejected",
			smName:   "",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "too_long_name_rejected",
			smName:   strings.Repeat("a", 81),
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid_chars_rejected",
			smName:   "bad<name>here",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)

			body, err := json.Marshal(map[string]string{
				"name":       tt.smName,
				"definition": sfnPassDefinition,
				"roleArn":    "arn:aws:iam::123456789012:role/role",
			})
			require.NoError(t, err)

			rec := sfnPost(ctx, t, h, e, "CreateStateMachine", string(body))
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode == http.StatusBadRequest {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, "InvalidName", resp["__type"])
			}
		})
	}
}

func TestCreateStateMachine_InlineTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantTags map[string]string
		name     string
		tags     []map[string]string
	}{
		{
			name:     "no_inline_tags",
			tags:     nil,
			wantTags: map[string]string{},
		},
		{
			name: "single_inline_tag",
			tags: []map[string]string{
				{"key": "env", "value": "test"},
			},
			wantTags: map[string]string{"env": "test"},
		},
		{
			name: "multiple_inline_tags",
			tags: []map[string]string{
				{"key": "env", "value": "prod"},
				{"key": "owner", "value": "team"},
			},
			wantTags: map[string]string{"env": "prod", "owner": "team"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)

			reqBody := map[string]any{
				"name":       "tagged-sm-" + tt.name,
				"definition": sfnPassDefinition,
				"roleArn":    "arn:aws:iam::123456789012:role/role",
			}
			if tt.tags != nil {
				reqBody["tags"] = tt.tags
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			rec := sfnPost(ctx, t, h, e, "CreateStateMachine", string(body))
			require.Equal(t, http.StatusOK, rec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
			smARN, _ := createResp["stateMachineArn"].(string)
			require.NotEmpty(t, smARN)

			// Verify tags were applied.
			listBody, _ := json.Marshal(map[string]string{"resourceArn": smARN})
			listRec := sfnPost(ctx, t, h, e, "ListTagsForResource", string(listBody))
			require.Equal(t, http.StatusOK, listRec.Code)

			var tagsResp map[string]any
			require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &tagsResp))

			rawTags, _ := tagsResp["tags"].([]any)
			got := make(map[string]string, len(rawTags))
			for _, rt := range rawTags {
				m, _ := rt.(map[string]any)
				k, _ := m["key"].(string)
				v, _ := m["value"].(string)
				got[k] = v
			}
			assert.Equal(t, tt.wantTags, got)
		})
	}
}

// ─── Encryption Configuration ────────────────────────────────────────────────

func TestCreateStateMachine_EncryptionConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		encCfg   map[string]any
		name     string
		wantType string
	}{
		{
			name:     "no_encryption_config",
			encCfg:   nil,
			wantType: "",
		},
		{
			name: "customer_managed_key",
			encCfg: map[string]any{
				"type":     "CUSTOMER_MANAGED_KMS_KEY",
				"kmsKeyId": "arn:aws:kms:us-east-1:123456789012:key/test-key",
			},
			wantType: "CUSTOMER_MANAGED_KMS_KEY",
		},
		{
			name: "aws_owned_key",
			encCfg: map[string]any{
				"type": "AWS_OWNED_KEY",
			},
			wantType: "AWS_OWNED_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)

			reqBody := map[string]any{
				"name":       "enc-sm-" + tt.name,
				"definition": sfnPassDefinition,
				"roleArn":    "arn:aws:iam::123456789012:role/role",
			}
			if tt.encCfg != nil {
				reqBody["encryptionConfiguration"] = tt.encCfg
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			rec := sfnPost(ctx, t, h, e, "CreateStateMachine", string(body))
			require.Equal(t, http.StatusOK, rec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
			smARN, _ := createResp["stateMachineArn"].(string)
			require.NotEmpty(t, smARN)

			// Describe to verify encryption config is stored.
			descBody, _ := json.Marshal(map[string]string{"stateMachineArn": smARN})
			descRec := sfnPost(ctx, t, h, e, "DescribeStateMachine", string(descBody))
			require.Equal(t, http.StatusOK, descRec.Code)

			var descResp map[string]any
			require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))

			if tt.wantType == "" {
				// No encryption config present in response or nil.
				enc, ok := descResp["encryptionConfiguration"]
				assert.True(t, !ok || enc == nil, "expected no encryptionConfiguration, got %v", enc)
			} else {
				enc, ok := descResp["encryptionConfiguration"].(map[string]any)
				require.True(t, ok, "expected encryptionConfiguration in response")
				assert.Equal(t, tt.wantType, enc["type"])
			}
		})
	}
}

func TestUpdateStateMachine_EncryptionConfiguration(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h, e := newSFNHandler(t)
	smARN := createSM(ctx, t, h, e, "enc-update-sm")

	// Update with encryption config.
	body, err := json.Marshal(map[string]any{
		"stateMachineArn": smARN,
		"encryptionConfiguration": map[string]string{
			"type":     "CUSTOMER_MANAGED_KMS_KEY",
			"kmsKeyId": "arn:aws:kms:us-east-1:123456789012:key/my-key",
		},
	})
	require.NoError(t, err)

	rec := sfnPost(ctx, t, h, e, "UpdateStateMachine", string(body))
	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify via describe.
	descBody, _ := json.Marshal(map[string]string{"stateMachineArn": smARN})
	descRec := sfnPost(ctx, t, h, e, "DescribeStateMachine", string(descBody))
	require.Equal(t, http.StatusOK, descRec.Code)

	var descResp map[string]any
	require.NoError(t, json.Unmarshal(descRec.Body.Bytes(), &descResp))
	enc, ok := descResp["encryptionConfiguration"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "CUSTOMER_MANAGED_KMS_KEY", enc["type"])
}

// ─── Publish on CreateStateMachine / UpdateStateMachine ─────────────────────

func TestCreateStateMachine_WithPublish(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		publish     bool
		wantVersion bool
	}{
		{
			name:        "publish_false_no_version",
			publish:     false,
			wantVersion: false,
		},
		{
			name:        "publish_true_creates_version",
			publish:     true,
			wantVersion: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)

			body, err := json.Marshal(map[string]any{
				"name":       "pub-sm-" + tt.name,
				"definition": sfnPassDefinition,
				"roleArn":    "arn:aws:iam::123456789012:role/role",
				"publish":    tt.publish,
			})
			require.NoError(t, err)

			rec := sfnPost(ctx, t, h, e, "CreateStateMachine", string(body))
			require.Equal(t, http.StatusOK, rec.Code)

			var createResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
			smARN, _ := createResp["stateMachineArn"].(string)
			require.NotEmpty(t, smARN)

			// AWS: stateMachineVersionArn is only populated on the
			// CreateStateMachine response when publish=true ("If you do not
			// set the publish parameter to true, this field returns null").
			versionArn, hasVersionArn := createResp["stateMachineVersionArn"]
			if tt.wantVersion {
				require.True(t, hasVersionArn, "expected stateMachineVersionArn in response")
				assert.NotEmpty(t, versionArn)
			} else {
				assert.False(t, hasVersionArn, "expected no stateMachineVersionArn when publish=false")
			}

			// Check versions.
			versBody, _ := json.Marshal(map[string]string{"stateMachineArn": smARN})
			versRec := sfnPost(ctx, t, h, e, "ListStateMachineVersions", string(versBody))
			require.Equal(t, http.StatusOK, versRec.Code)

			var versResp map[string]any
			require.NoError(t, json.Unmarshal(versRec.Body.Bytes(), &versResp))
			versions, _ := versResp["stateMachineVersions"].([]any)

			if tt.wantVersion {
				assert.Len(t, versions, 1, "expected one version after publish=true")
			} else {
				assert.Empty(t, versions, "expected no versions when publish=false")
			}
		})
	}
}

func TestUpdateStateMachine_WithPublish(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h, e := newSFNHandler(t)
	smARN := createSM(ctx, t, h, e, "pub-update-sm")

	// Update with publish=true.
	newDef := `{"StartAt":"S","States":{"S":{"Type":"Succeed"}}}`
	body, err := json.Marshal(map[string]any{
		"stateMachineArn": smARN,
		"definition":      newDef,
		"publish":         true,
	})
	require.NoError(t, err)

	rec := sfnPost(ctx, t, h, e, "UpdateStateMachine", string(body))
	require.Equal(t, http.StatusOK, rec.Code)

	var updateResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updateResp))
	assert.NotEmpty(t, updateResp["revisionId"], "expected a non-empty revisionId after update")
	assert.NotEmpty(t, updateResp["stateMachineVersionArn"], "expected stateMachineVersionArn when publish=true")

	versBody, _ := json.Marshal(map[string]string{"stateMachineArn": smARN})
	versRec := sfnPost(ctx, t, h, e, "ListStateMachineVersions", string(versBody))
	require.Equal(t, http.StatusOK, versRec.Code)

	var versResp map[string]any
	require.NoError(t, json.Unmarshal(versRec.Body.Bytes(), &versResp))
	versions, _ := versResp["stateMachineVersions"].([]any)
	assert.Len(t, versions, 1, "expected one version after update with publish=true")
}

// TestUpdateStateMachine_NoPublish_NoVersionArn verifies that
// UpdateStateMachine's response omits stateMachineVersionArn when
// publish=false, but still returns a fresh revisionId on every update.
func TestUpdateStateMachine_NoPublish_NoVersionArn(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h, e := newSFNHandler(t)
	smARN := createSM(ctx, t, h, e, "no-pub-update-sm")

	newDef := `{"StartAt":"S","States":{"S":{"Type":"Succeed"}}}`
	body, err := json.Marshal(map[string]any{
		"stateMachineArn": smARN,
		"definition":      newDef,
	})
	require.NoError(t, err)

	rec := sfnPost(ctx, t, h, e, "UpdateStateMachine", string(body))
	require.Equal(t, http.StatusOK, rec.Code)

	var updateResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updateResp))
	assert.NotEmpty(t, updateResp["revisionId"])
	_, hasVersionArn := updateResp["stateMachineVersionArn"]
	assert.False(t, hasVersionArn, "expected no stateMachineVersionArn when publish=false")
}

// TestUpdateStateMachine_VersionDescriptionRequiresPublish verifies AWS's
// documented ValidationException: versionDescription may only be set when
// publish=true.
func TestUpdateStateMachine_VersionDescriptionRequiresPublish(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h, e := newSFNHandler(t)
	smARN := createSM(ctx, t, h, e, "verdesc-update-sm")

	body, err := json.Marshal(map[string]any{
		"stateMachineArn":    smARN,
		"definition":         `{"StartAt":"S","States":{"S":{"Type":"Succeed"}}}`,
		"versionDescription": "should be rejected",
		"publish":            false,
	})
	require.NoError(t, err)

	rec := sfnPost(ctx, t, h, e, "UpdateStateMachine", string(body))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "ValidationException")
}

// ─── RedriveExecution with Count ─────────────────────────────────────────────

func TestRoleARN_InvalidPrefix(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h, e := newSFNHandler(t)

	body := makeSMBody("role-bad-sm", validPassDef, "")
	body = strings.Replace(body, `"arn:role"`, `"not-an-arn"`, 1)

	rec := sfnPost(ctx, t, h, e, "CreateStateMachine", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRoleARN_WithWhitespace(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h, e := newSFNHandler(t)

	body := makeSMBody("role-ws-sm", validPassDef, "")
	body = strings.Replace(body, `"arn:role"`, `"arn:aws:iam:: 123:role/sfn"`, 1)

	rec := sfnPost(ctx, t, h, e, "CreateStateMachine", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ─── InputPayload size validation ─────────────────────────────────────────────

func TestCreateStateMachine_ResponseContainsARNAndDate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h, e := newSFNHandler(t)

	rec := sfnPost(ctx, t, h, e, "CreateStateMachine", makeSMBody("resp-sm", validPassDef, ""))
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	assert.NotEmpty(t, out["stateMachineArn"])
	assert.Greater(t, out["creationDate"].(float64), float64(0))
}

// ─── StartExecution response fields ──────────────────────────────────────────

func TestCreateStateMachine_Idempotent_ViaHandler(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h, e := newSFNHandler(t)

	body, err := json.Marshal(map[string]any{
		"name":       "idem-handler-sm",
		"definition": sfnPassDefinition,
		"roleArn":    "arn:aws:iam::123456789012:role/role",
	})
	require.NoError(t, err)

	rec1 := sfnPost(ctx, t, h, e, "CreateStateMachine", string(body))
	require.Equal(t, http.StatusOK, rec1.Code)
	var resp1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &resp1))

	rec2 := sfnPost(ctx, t, h, e, "CreateStateMachine", string(body))
	require.Equal(t, http.StatusOK, rec2.Code)
	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))

	assert.Equal(t, resp1["stateMachineArn"], resp2["stateMachineArn"])
}

// TestParity_CreateStateMachineRequiresRoleArn verifies that CreateStateMachine
// rejects requests with a missing or empty roleArn.
// Real AWS returns ValidationException for this case; the emulator previously
// accepted an empty roleArn, silently creating a state machine without a role.
func TestCreateStateMachineRequiresRoleArn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantType string
		wantCode int
	}{
		{
			name: "absent_roleArn_rejected",
			body: map[string]any{
				"name":       "my-sm",
				"definition": validPassDef,
			},
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name: "empty_roleArn_rejected",
			body: map[string]any{
				"name":       "my-sm",
				"definition": validPassDef,
				"roleArn":    "",
			},
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name: "valid_roleArn_accepted",
			body: map[string]any{
				"name":       "my-sm",
				"definition": validPassDef,
				"roleArn":    "arn:aws:iam::123456789012:role/MyRole",
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e := newSFNHandler(t)
			ctx := context.Background()

			body, err := json.Marshal(tt.body)
			require.NoError(t, err)

			rec := sfnPost(ctx, t, h, e, "CreateStateMachine", string(body))
			assert.Equal(t, tt.wantCode, rec.Code,
				"CreateStateMachine status for case %q", tt.name)

			if tt.wantType != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantType, resp["__type"],
					"error type for case %q", tt.name)
			}
		})
	}
}

// TestCreateStateMachine_NameValidation asserts InvalidName for malformed names.
func TestCreateStateMachine_NameValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		smName   string
		wantType string
		wantCode int
	}{
		{name: "valid_name", smName: "my-state-machine", wantCode: http.StatusOK},
		{name: "empty_name", smName: "", wantCode: http.StatusBadRequest, wantType: "InvalidName"},
		{
			name:     "too_long_81_chars",
			smName:   strings.Repeat("a", 81),
			wantCode: http.StatusBadRequest,
			wantType: "InvalidName",
		},
		{name: "invalid_chars", smName: "bad<name>!", wantCode: http.StatusBadRequest, wantType: "InvalidName"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, e := newSFNHandler(t)
			ctx := t.Context()

			body, err := json.Marshal(map[string]any{
				"name":       tt.smName,
				"definition": validPassDef,
				"roleArn":    "arn:aws:iam::123456789012:role/role",
			})
			require.NoError(t, err)

			rec := sfnPost(ctx, t, h, e, "CreateStateMachine", string(body))
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantType != "" {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, tt.wantType, resp["__type"])
			}
		})
	}
}

// TestUpdateStateMachine_ArnValidation asserts ValidationException for missing/empty ARN.
func TestUpdateStateMachine_ArnValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		arn      string
		wantType string
		wantCode int
	}{
		{
			name:     "empty_arn_returns_ValidationException",
			arn:      "",
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name:     "nonexistent_arn_returns_StateMachineDoesNotExist",
			arn:      "arn:aws:states:us-east-1:123456789012:stateMachine:no-such",
			wantCode: http.StatusNotFound,
			wantType: "StateMachineDoesNotExist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newSFNHandler(t)
			e := echo.New()
			ctx := t.Context()

			body, err := json.Marshal(map[string]any{
				"stateMachineArn": tt.arn,
				"definition":      validPassDef,
			})
			require.NoError(t, err)

			rec := sfnPost(ctx, t, h, e, "UpdateStateMachine", string(body))
			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantType, resp["__type"])
		})
	}
}

func TestHandler_StateMachineActions_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		action   string
		wantCode int
	}{
		// JSON unmarshal errors fall through to InternalServerError
		{
			name:     "CreateStateMachine_invalid_json",
			action:   "CreateStateMachine",
			wantCode: http.StatusInternalServerError,
		},
		{
			name:     "DeleteStateMachine_invalid_json",
			action:   "DeleteStateMachine",
			wantCode: http.StatusInternalServerError,
		},
		{name: "ListStateMachines_invalid_json", action: "ListStateMachines", wantCode: http.StatusInternalServerError},
		{
			name:     "DescribeStateMachine_invalid_json",
			action:   "DescribeStateMachine",
			wantCode: http.StatusInternalServerError,
		},
		{
			name:     "ListTagsForResource_invalid_json",
			action:   "ListTagsForResource",
			wantCode: http.StatusInternalServerError,
		},
		{name: "TagResource_invalid_json", action: "TagResource", wantCode: http.StatusInternalServerError},
		{name: "UntagResource_invalid_json", action: "UntagResource", wantCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)

			rec := sfnPost(ctx, t, h, e, tt.action, `{invalid`)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// ---- executionActions: invalid JSON for each operation ----

func TestHandler_UpdateStateMachine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		roleOnly bool
		wantCode int
	}{
		{
			name:     "update_with_valid_definition",
			wantCode: http.StatusOK,
		},
		{
			name:     "update_roleArn_only",
			roleOnly: true,
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			h, e := newSFNHandler(t)
			smARN := createSM(ctx, t, h, e, "update-sm")

			var body []byte
			if tt.roleOnly {
				var err error
				body, err = json.Marshal(map[string]any{
					"stateMachineArn": smARN,
					"roleArn":         "arn:aws:iam::123456789012:role/new-role",
				})
				require.NoError(t, err)
			} else {
				var err error
				body, err = json.Marshal(map[string]any{
					"stateMachineArn": smARN,
					"definition":      sfnPassDefinition,
				})
				require.NoError(t, err)
			}

			rec := sfnPost(ctx, t, h, e, "UpdateStateMachine", string(body))
			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Contains(t, resp, "updateDate")
		})
	}
}

// ---- ListExecutions with status filter ----
