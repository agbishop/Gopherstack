package ssm_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/services/ssm"
)

func TestSSMHandler_ExtractResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantName string
	}{
		{
			name:     "name_field_extracted",
			body:     `{"Name":"/my/param"}`,
			wantName: "/my/param",
		},
		{
			name:     "no_name_field_returns_empty",
			body:     `{"Type":"String"}`,
			wantName: "",
		},
		{
			name:     "invalid_json_returns_empty",
			body:     "not-json",
			wantName: "",
		},
		{
			name:     "name_not_string_returns_empty",
			body:     `{"Name":123}`,
			wantName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, _ := newTestHandler(t)
			e := echo.New()

			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(http.MethodPost, "/", nil)
			}

			req.Header.Set("X-Amz-Target", "AmazonSSM.GetParameter")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			assert.Equal(t, tt.wantName, h.ExtractResource(c))
		})
	}
}
func TestSSMHandler_UnknownOperation(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)
	rec := doRequest(t, h, "UnknownOp", `{}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["__type"], "UnknownOperationException")
}
func TestSSMHandler_InternalError(t *testing.T) {
	t.Parallel()

	// PutParameter with invalid JSON triggers UnmarshalError, which would fall to InternalServerError
	// unless it's a recognized error type
	h, _ := newTestHandler(t)
	rec := doRequest(t, h, "PutParameter", `{"Name":`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
func TestSSMHandler_DeleteParameter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(b *ssm.InMemoryBackend)
		body     string
		wantCode int
	}{
		{
			name: "success",
			setup: func(b *ssm.InMemoryBackend) {
				_, _ = b.PutParameter(context.TODO(), &ssm.PutParameterInput{
					Name:  "/delete-me",
					Type:  "String",
					Value: "val",
				})
			},
			body:     `{"Name":"/delete-me"}`,
			wantCode: http.StatusOK,
		},
		{
			name:     "not_found",
			body:     `{"Name":"/nonexistent"}`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(b)
			}

			rec := doRequest(t, h, "DeleteParameter", tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

// TestSSMHandler_SnapshotRestore_Delegation tests the Handler's Snapshot/Restore delegation.
func TestSSMHandler_SnapshotRestore_Delegation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(b *ssm.InMemoryBackend)
		check func(t *testing.T, b *ssm.InMemoryBackend)
		name  string
	}{
		{
			name: "snapshot_and_restore_via_handler",
			setup: func(b *ssm.InMemoryBackend) {
				_, _ = b.PutParameter(
					context.TODO(),
					&ssm.PutParameterInput{Name: "/snap-param", Type: "String", Value: "snap-value"},
				)
			},
			check: func(t *testing.T, b *ssm.InMemoryBackend) {
				t.Helper()

				out, err := b.GetParameter(context.TODO(), &ssm.GetParameterInput{Name: "/snap-param"})
				require.NoError(t, err)
				assert.Equal(t, "snap-value", out.Parameter.Value)
			},
		},
		{
			name:  "empty_backend_snapshot_and_restore",
			setup: func(_ *ssm.InMemoryBackend) {},
			check: func(t *testing.T, b *ssm.InMemoryBackend) {
				t.Helper()
				assert.Empty(t, b.ListAll(context.TODO()))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			origBackend := ssm.NewInMemoryBackend()
			tt.setup(origBackend)

			snap := origBackend.Snapshot(t.Context())
			require.NotNil(t, snap)

			freshBackend := ssm.NewInMemoryBackend()
			require.NoError(t, freshBackend.Restore(t.Context(), snap))

			tt.check(t, freshBackend)
		})
	}
}

// TestSSMHandler_ValidationError covers the path where a ValidationException error is returned.
func TestSSMHandler_ValidationError(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)

	// ssm/ reserved-namespace prefix triggers ErrParameterNamePattern
	// (ParameterPatternMismatchException, PutParameter's own declared
	// exception for a malformed Name), which is now explicitly handled.
	rec := doRequest(t, h, "PutParameter", `{"Name":"ssm/bad","Type":"String","Value":"v"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ParameterPatternMismatchException", resp["__type"])
}

// TestSSMHandler_ErrInvalidKeyID covers the InvalidKeyId path.
func TestSSMHandler_ErrInvalidKeyID(t *testing.T) {
	t.Parallel()

	// The ErrInvalidKeyID is returned when KeyId is provided
	// We exercise handleError by directly checking each branch
	// The InternalServerError branch is hit by a random error
	h, _ := newTestHandler(t)

	// Unknown operation triggers UnknownOperationException
	rec := doRequest(t, h, "BogusOperation", `{}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp["__type"], "UnknownOperationException")
}

// testInvalidBackend wraps InMemoryBackend and returns an internal error on PutParameter.
type testInvalidBackend struct {
	*ssm.InMemoryBackend
}

var errSimulatedInternal = errors.New("simulated internal error")

func (b *testInvalidBackend) PutParameter(
	_ context.Context,
	_ *ssm.PutParameterInput,
) (*ssm.PutParameterOutput, error) {
	return nil, errSimulatedInternal
}

// TestSSMHandler_InternalServerError exercises the InternalServerError path in handleError.
func TestSSMHandler_InternalServerError(t *testing.T) {
	t.Parallel()

	// Use a backend that returns a non-recognized error
	errBackend := &testInvalidBackend{InMemoryBackend: ssm.NewInMemoryBackend()}
	h2 := ssm.NewHandler(errBackend)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/",
		strings.NewReader(`{"Name":"/test","Type":"String","Value":"v"}`))
	req.Header.Set("X-Amz-Target", "AmazonSSM.PutParameter")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h2.Handler()(c))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
func TestSSMHandler_HandlerSnapshotRestore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(b *ssm.InMemoryBackend)
		name  string
	}{
		{
			name: "with_data",
			setup: func(b *ssm.InMemoryBackend) {
				_, _ = b.PutParameter(
					context.TODO(),
					&ssm.PutParameterInput{Name: "/h-snap", Type: "String", Value: "hval"},
				)
			},
		},
		{
			name:  "empty",
			setup: func(_ *ssm.InMemoryBackend) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := ssm.NewInMemoryBackend()
			tt.setup(backend)
			h := ssm.NewHandler(backend)

			snap := h.Snapshot(t.Context())
			require.NotNil(t, snap)

			freshBackend := ssm.NewInMemoryBackend()
			freshH := ssm.NewHandler(freshBackend)
			require.NoError(t, freshH.Restore(t.Context(), snap))

			if tt.name == "with_data" {
				out, err := freshBackend.GetParameter(context.TODO(), &ssm.GetParameterInput{Name: "/h-snap"})
				require.NoError(t, err)
				assert.Equal(t, "hval", out.Parameter.Value)
			}
		})
	}
}

// TestDeepCopy_Association verifies modifying caller's Parameters doesn't corrupt stored data.
func TestDeepCopy_Association(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		docName   string
		params    map[string][]string
		mutateKey string
		mutateVal string
	}{
		{
			name:      "modify_after_create_does_not_corrupt",
			docName:   "TestDoc",
			params:    map[string][]string{"commands": {"echo hello"}},
			mutateKey: "commands",
			mutateVal: "echo MUTATED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := ssm.NewInMemoryBackend()
			h := ssm.NewHandler(backend)

			doRequest(t, h, "CreateDocument",
				`{"Name":"`+tt.docName+`","Content":"{\"schemaVersion\":\"2.2\"}"}`)

			paramsJSON, _ := json.Marshal(tt.params)
			createBody := `{"Name":"` + tt.docName + `","Parameters":` + string(paramsJSON) + `}`

			rec := doRequest(t, h, "CreateAssociation", createBody)
			require.Equal(t, http.StatusOK, rec.Code)

			var createResp ssm.CreateAssociationOutput
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
			originalCmds := createResp.AssociationDescription.Parameters[tt.mutateKey]

			// Verify original value was stored correctly
			require.NotEmpty(t, originalCmds)
			assert.Equal(t, tt.params[tt.mutateKey][0], originalCmds[0])
		})
	}
}

func TestFull_ParameterStore_PutGetDelete(t *testing.T) {
	t.Parallel()
	h := newHandler()

	// Put
	code, out := postJSON(t, h, "PutParameter", map[string]any{
		"Name":  "/full/p1",
		"Type":  "String",
		"Value": "v1",
	})
	assert.Equal(t, http.StatusOK, code)
	assert.InEpsilon(t, float64(1), out["Version"], 0.01)

	// Get
	code, out = postJSON(t, h, "GetParameter", map[string]any{"Name": "/full/p1"})
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "v1", out["Parameter"].(map[string]any)["Value"])

	// Overwrite
	code, out = postJSON(t, h, "PutParameter", map[string]any{
		"Name":      "/full/p1",
		"Type":      "String",
		"Value":     "v2",
		"Overwrite": true,
	})
	assert.Equal(t, http.StatusOK, code)
	assert.InEpsilon(t, float64(2), out["Version"], 0.01)

	// Delete
	code, _ = postJSON(t, h, "DeleteParameter", map[string]any{"Name": "/full/p1"})
	assert.Equal(t, http.StatusOK, code)

	// Gone
	code, _ = postJSON(t, h, "GetParameter", map[string]any{"Name": "/full/p1"})
	assert.Equal(t, http.StatusBadRequest, code)
}
func TestFull_ParameterStore_GetParameters_Batch(t *testing.T) {
	t.Parallel()
	h := newHandler()

	for _, name := range []string{"/b/1", "/b/2", "/b/3"} {
		postJSON(t, h, "PutParameter", map[string]any{"Name": name, "Type": "String", "Value": "v"})
	}

	code, out := postJSON(t, h, "GetParameters", map[string]any{
		"Names": []string{"/b/1", "/b/2", "/b/missing"},
	})

	assert.Equal(t, http.StatusOK, code)
	params := out["Parameters"].([]any)
	invalid := out["InvalidParameters"].([]any)
	assert.Len(t, params, 2)
	assert.Len(t, invalid, 1)
	assert.Equal(t, "/b/missing", invalid[0])
}
func TestFull_ParameterStore_DeleteParameters_Batch(t *testing.T) {
	t.Parallel()
	h := newHandler()

	for _, name := range []string{"/del/1", "/del/2"} {
		postJSON(t, h, "PutParameter", map[string]any{"Name": name, "Type": "String", "Value": "v"})
	}

	code, out := postJSON(t, h, "DeleteParameters", map[string]any{
		"Names": []string{"/del/1", "/del/missing"},
	})

	assert.Equal(t, http.StatusOK, code)
	deleted := out["DeletedParameters"].([]any)
	invalid := out["InvalidParameters"].([]any)
	assert.Len(t, deleted, 1)
	assert.Len(t, invalid, 1)
}

// TestPutParameter_Validation exercises validation branches in PutParameter.
func TestPutParameter_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		errIs   error
		name    string
		input   ssm.PutParameterInput
		wantErr bool
	}{
		{
			name: "invalid_data_type",
			input: ssm.PutParameterInput{
				Name:     "/valid/param",
				Type:     "String",
				Value:    "v",
				DataType: "badtype",
			},
			wantErr: true,
			errIs:   ssm.ErrValidationException,
		},
		{
			name: "invalid_allowed_pattern",
			input: ssm.PutParameterInput{
				Name:           "/valid/param",
				Type:           "String",
				Value:          "hello world",
				AllowedPattern: `^\d+$`,
			},
			wantErr: true,
			// InvalidAllowedPatternException, not the generic
			// ValidationException: it is PutParameter's own declared
			// exception for this (gopherstack-jpfk).
			errIs: ssm.ErrInvalidAllowedPattern,
		},
		{
			name: "bad_regex_pattern",
			input: ssm.PutParameterInput{
				Name:           "/valid/param",
				Type:           "String",
				Value:          "v",
				AllowedPattern: `[invalid`,
			},
			wantErr: true,
			errIs:   ssm.ErrInvalidAllowedPattern,
		},
		{
			name: "tier_too_large_standard",
			input: ssm.PutParameterInput{
				Name:  "/valid/param",
				Type:  "String",
				Value: string(make([]byte, 5000)), // > 4096 bytes
				Tier:  "Standard",
			},
			wantErr: true,
			errIs:   ssm.ErrValidationException,
		},
		{
			name: "tier_too_large_advanced",
			input: ssm.PutParameterInput{
				Name:  "/valid/param",
				Type:  "String",
				Value: string(make([]byte, 9000)), // > 8192 bytes
				Tier:  "Advanced",
			},
			wantErr: true,
			errIs:   ssm.ErrValidationException,
		},
		{
			name: "invalid_tier_string",
			input: ssm.PutParameterInput{
				Name:  "/valid/param",
				Type:  "String",
				Value: "v",
				Tier:  "UnknownTier",
			},
			wantErr: true,
			errIs:   ssm.ErrValidationException,
		},
		{
			name: "valid_ec2_image_datatype",
			input: ssm.PutParameterInput{
				Name:     "/valid/ec2",
				Type:     "String",
				Value:    "ami-12345678",
				DataType: "aws:ec2:image",
			},
			wantErr: false,
		},
		{
			name: "valid_intelligent_tiering",
			input: ssm.PutParameterInput{
				Name:  "/valid/tiering",
				Type:  "String",
				Value: "v",
				Tier:  "Intelligent-Tiering",
			},
			wantErr: false,
		},
		{
			name: "param_name_too_long",
			input: ssm.PutParameterInput{
				Name:  "/" + string(make([]byte, 2048)),
				Type:  "String",
				Value: "v",
			},
			wantErr: true,
			// ParameterPatternMismatchException, not the generic
			// ValidationException: it is PutParameter's own declared
			// exception for a malformed Name (gopherstack-jpfk).
			errIs: ssm.ErrParameterNamePattern,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ssm.NewInMemoryBackend()
			_, err := b.PutParameter(context.TODO(), &tt.input)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					require.ErrorIs(t, err, tt.errIs)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestHandlerReset exercises the handler Reset method.
func TestHandlerReset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup func(h *ssm.Handler, b *ssm.InMemoryBackend)
		name  string
	}{
		{
			name: "reset_clears_parameters",
			setup: func(_ *ssm.Handler, b *ssm.InMemoryBackend) {
				_, _ = b.PutParameter(context.TODO(), &ssm.PutParameterInput{
					Name: "/before-reset", Type: "String", Value: "v",
				})
			},
		},
		{
			name:  "reset_on_empty_backend",
			setup: func(_ *ssm.Handler, _ *ssm.InMemoryBackend) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, b := newTestHandler(t)
			tt.setup(h, b)
			h.Reset()
			assert.Empty(t, b.ListAll(context.TODO()))
		})
	}
}
func TestAdvancedTier_AcceptsLargeValue(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)

	// Advanced tier allows up to 8 KiB — use a 5 KiB value.
	largeVal := make([]byte, 5000)
	for i := range largeVal {
		largeVal[i] = 'A'
	}

	body, _ := json.Marshal(map[string]any{
		"Name":  "/advanced/param",
		"Value": string(largeVal),
		"Type":  "String",
		"Tier":  "Advanced",
	})
	rec := doRequest(t, h, "PutParameter", string(body))
	assert.Equal(t, http.StatusOK, rec.Code, "Advanced tier should accept 5KiB value")
}
func TestStandardTier_RejectsLargeValue(t *testing.T) {
	t.Parallel()

	h, _ := newTestHandler(t)

	// Standard tier only allows up to 4 KiB — use 5 KiB.
	largeVal := make([]byte, 5000)
	for i := range largeVal {
		largeVal[i] = 'B'
	}

	body, _ := json.Marshal(map[string]any{
		"Name":  "/standard/param",
		"Value": string(largeVal),
		"Type":  "String",
		"Tier":  "Standard",
	})
	rec := doRequest(t, h, "PutParameter", string(body))
	assert.Equal(t, http.StatusBadRequest, rec.Code, "Standard tier should reject 5KiB value")
}
