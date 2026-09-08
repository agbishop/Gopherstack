package cloudcontrol_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudcontrol"
)

func TestHandler_CreateResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		wantOp     string
		wantStatus int
	}{
		{
			name: "success with named identifier",
			body: map[string]any{
				"TypeName":     "AWS::Logs::LogGroup",
				"DesiredState": `{"LogGroupName":"my-log-group"}`,
			},
			wantStatus: http.StatusOK,
			wantOp:     "CREATE",
		},
		{
			name: "success without identifier generates uuid",
			body: map[string]any{
				"TypeName":     "AWS::S3::Bucket",
				"DesiredState": `{}`,
			},
			wantStatus: http.StatusOK,
			wantOp:     "CREATE",
		},
		{
			name: "missing TypeName returns 400",
			body: map[string]any{
				"DesiredState": `{}`,
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, "CreateResource", tt.body)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				pe, ok := out["ProgressEvent"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantOp, pe["Operation"])
				assert.Equal(t, "SUCCESS", pe["OperationStatus"])
				assert.NotEmpty(t, pe["RequestToken"])
			}
		})
	}
}

// TestHandler_CreateResource_DuplicateReturns400 verifies AlreadyExistsException maps to
// HTTP 400, per the real API reference -- not 409, which real CloudControl never returns.
func TestHandler_CreateResource_DuplicateReturns400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	body := map[string]any{
		"TypeName":     "AWS::Logs::LogGroup",
		"DesiredState": `{"LogGroupName":"duplicate-group"}`,
	}

	rec := doRequest(t, h, "CreateResource", body)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec2 := doRequest(t, h, "CreateResource", body)
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

// TestHandler_CreateResource_Duplicate_IsAlreadyExistsException verifies both the wire error
// code and the HTTP 400 status (not 409).
func TestHandler_CreateResource_Duplicate_IsAlreadyExistsException(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	body := map[string]any{
		"TypeName":     "AWS::Logs::LogGroup",
		"DesiredState": `{"LogGroupName":"dup"}`,
	}
	require.Equal(t, http.StatusOK, doRequest(t, h, "CreateResource", body).Code)

	rec := doRequest(t, h, "CreateResource", body)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "AlreadyExistsException", errType(t, rec.Body.Bytes()))
}
func TestHandler_CreateResource_ClientToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	body := map[string]any{
		"TypeName":     "AWS::Logs::LogGroup",
		"DesiredState": `{"LogGroupName":"client-token-test"}`,
		"ClientToken":  "unique-client-token-123",
	}

	rec1 := doRequest(t, h, "CreateResource", body)
	require.Equal(t, http.StatusOK, rec1.Code)

	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &out1))
	pe1 := out1["ProgressEvent"].(map[string]any)

	// Same clientToken must return the same RequestToken (idempotent).
	rec2 := doRequest(t, h, "CreateResource", body)
	require.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))
	pe2 := out2["ProgressEvent"].(map[string]any)

	assert.Equal(t, pe1["RequestToken"], pe2["RequestToken"])
}
func TestBackend_CreateResource_ClientToken_Idempotency(t *testing.T) {
	t.Parallel()

	b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")

	ev1, err := b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"idem-test"}`, "my-client-token")
	require.NoError(t, err)

	// Second call with the same clientToken must return the same RequestToken.
	ev2, err := b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"idem-test"}`, "my-client-token")
	require.NoError(t, err)

	assert.Equal(t, ev1.RequestToken, ev2.RequestToken, "idempotent calls must return the same request token")
	assert.Len(t, b.ListAllResources(), 1, "only one resource should be created")
}
func TestBackend_NewIdentifierKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desiredState   string
		name           string
		wantIdentifier string
	}{
		{
			name:           "TableName",
			desiredState:   `{"TableName":"my-table"}`,
			wantIdentifier: "my-table",
		},
		{
			name:           "RoleName",
			desiredState:   `{"RoleName":"my-role"}`,
			wantIdentifier: "my-role",
		},
		{
			name:           "ClusterName",
			desiredState:   `{"ClusterName":"my-cluster"}`,
			wantIdentifier: "my-cluster",
		},
		{
			name:           "StreamName",
			desiredState:   `{"StreamName":"my-stream"}`,
			wantIdentifier: "my-stream",
		},
		{
			name:           "DBInstanceIdentifier",
			desiredState:   `{"DBInstanceIdentifier":"my-db"}`,
			wantIdentifier: "my-db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
			ev, err := b.CreateResource("AWS::Some::Type", tt.desiredState, "")
			require.NoError(t, err)
			assert.Equal(t, tt.wantIdentifier, ev.Identifier)
		})
	}
}

// TestHandler_ProgressEvent_ResourceModel verifies that CreateResource and UpdateResource
// populate ProgressEvent.ResourceModel with the resource's current properties (a JSON
// string), matching the real ProgressEvent shape -- so callers can read the resource
// straight off the ProgressEvent without a follow-up GetResource call.
func TestHandler_ProgressEvent_ResourceModel(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	createRec := doRequest(t, h, "CreateResource", map[string]any{
		"TypeName":     "AWS::Logs::LogGroup",
		"DesiredState": `{"LogGroupName":"model-test","RetentionInDays":7}`,
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createOut))
	createPE := createOut["ProgressEvent"].(map[string]any)

	createModel, ok := createPE["ResourceModel"].(string)
	require.True(t, ok, "ResourceModel must be present as a string on CreateResource")

	var createParsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(createModel), &createParsed))
	assert.Equal(t, "model-test", createParsed["LogGroupName"])

	updateRec := doRequest(t, h, "UpdateResource", map[string]any{
		"TypeName":      "AWS::Logs::LogGroup",
		"Identifier":    "model-test",
		"PatchDocument": `[{"op":"replace","path":"/RetentionInDays","value":30}]`,
	})
	require.Equal(t, http.StatusOK, updateRec.Code)

	var updateOut map[string]any
	require.NoError(t, json.Unmarshal(updateRec.Body.Bytes(), &updateOut))
	updatePE := updateOut["ProgressEvent"].(map[string]any)

	updateModel, ok := updatePE["ResourceModel"].(string)
	require.True(t, ok, "ResourceModel must be present as a string on UpdateResource")

	var updateParsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(updateModel), &updateParsed))
	assert.InEpsilon(t, float64(30), updateParsed["RetentionInDays"], 0, "ResourceModel must reflect the patched value")
}
func TestHandler_GetResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*cloudcontrol.Handler)
		body       map[string]any
		name       string
		wantProps  string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"get-test"}`, "")
			},
			body: map[string]any{
				"TypeName":   "AWS::Logs::LogGroup",
				"Identifier": "get-test",
			},
			wantStatus: http.StatusOK,
			wantProps:  `{"LogGroupName":"get-test"}`,
		},
		{
			name:  "not found returns 400",
			setup: nil,
			body: map[string]any{
				"TypeName":   "AWS::Logs::LogGroup",
				"Identifier": "nonexistent",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "missing TypeName returns 400",
			setup: nil,
			body: map[string]any{
				"Identifier": "something",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "missing Identifier returns 400",
			setup: nil,
			body: map[string]any{
				"TypeName": "AWS::Logs::LogGroup",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "GetResource", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				rd, ok := out["ResourceDescription"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantProps, rd["Properties"])
			}
		})
	}
}

// TestHandler_GetResource_PropertiesIsJSONString verifies Properties is returned as a JSON string
// (not an object), matching the AWS Cloud Control API specification.
func TestHandler_GetResource_PropertiesIsJSONString(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	props := `{"BucketName":"my-bucket","VersioningConfiguration":{"Status":"Enabled"}}`

	rec := doRequest(t, h, "CreateResource", map[string]any{
		"TypeName":     "AWS::S3::Bucket",
		"DesiredState": props,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var createOut map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createOut))
	pe := createOut["ProgressEvent"].(map[string]any)
	identifier := pe["Identifier"].(string)

	rec = doRequest(t, h, "GetResource", map[string]any{
		"TypeName":   "AWS::S3::Bucket",
		"Identifier": identifier,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var getOut map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getOut))
	desc, ok := getOut["ResourceDescription"].(map[string]any)
	require.True(t, ok, "ResourceDescription must be present")

	// Properties MUST be a string (JSON-encoded), not an object.
	propertiesRaw, ok := desc["Properties"]
	require.True(t, ok, "Properties must be present")
	_, isString := propertiesRaw.(string)
	assert.True(t, isString, "Properties must be a JSON string, not %T", propertiesRaw)

	// The string must be valid JSON.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(propertiesRaw.(string)), &parsed))
	assert.Equal(t, "my-bucket", parsed["BucketName"])
}

// TestHandler_GetResource_NotFound_IsResourceNotFoundException verifies both the wire error
// code and the HTTP 400 status (not 404 -- real CloudControl returns 400 for every client
// fault, including ResourceNotFoundException).
func TestHandler_GetResource_NotFound_IsResourceNotFoundException(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "GetResource", map[string]any{
		"TypeName":   "AWS::Logs::LogGroup",
		"Identifier": "nonexistent",
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "ResourceNotFoundException", errType(t, rec.Body.Bytes()))
}
func TestHandler_ListResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*cloudcontrol.Handler)
		body       map[string]any
		name       string
		wantStatus int
		wantCount  int
	}{
		{
			name: "returns resources of type",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"list-test-1"}`, "")
				_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"list-test-2"}`, "")
				_, _ = h.Backend.CreateResource("AWS::S3::Bucket", `{"BucketName":"other-bucket"}`, "")
			},
			body:       map[string]any{"TypeName": "AWS::Logs::LogGroup"},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "empty list for unknown type",
			setup:      nil,
			body:       map[string]any{"TypeName": "AWS::Unknown::Resource"},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:       "missing TypeName returns 400",
			setup:      nil,
			body:       map[string]any{},
			wantStatus: http.StatusBadRequest,
		},
		// ResourceModel is a real ListResourcesInput field ("The resource model to use
		// to select the resources to return") that was previously accepted on the wire
		// but never applied as a filter (bd issue gopherstack-c9yf). These cases lock in
		// the fix: it now filters on matching Properties key/value pairs.
		{
			name: "ResourceModel filters to matching properties",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource(
					"AWS::Logs::LogGroup", `{"LogGroupName":"model-a","RetentionInDays":7}`, "",
				)
				_, _ = h.Backend.CreateResource(
					"AWS::Logs::LogGroup", `{"LogGroupName":"model-b","RetentionInDays":30}`, "",
				)
			},
			body: map[string]any{
				"TypeName":      "AWS::Logs::LogGroup",
				"ResourceModel": `{"RetentionInDays":7}`,
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
		{
			name: "ResourceModel matching nothing returns empty",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource(
					"AWS::Logs::LogGroup", `{"LogGroupName":"model-c","RetentionInDays":7}`, "",
				)
			},
			body: map[string]any{
				"TypeName":      "AWS::Logs::LogGroup",
				"ResourceModel": `{"RetentionInDays":999}`,
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "unparseable ResourceModel returns empty rather than error",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource(
					"AWS::Logs::LogGroup", `{"LogGroupName":"model-d"}`, "",
				)
			},
			body: map[string]any{
				"TypeName":      "AWS::Logs::LogGroup",
				"ResourceModel": `not-json`,
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "empty ResourceModel does not filter",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource(
					"AWS::Logs::LogGroup", `{"LogGroupName":"model-e","RetentionInDays":7}`, "",
				)
			},
			body: map[string]any{
				"TypeName": "AWS::Logs::LogGroup",
			},
			wantStatus: http.StatusOK,
			wantCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "ListResources", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				descs, ok := out["ResourceDescriptions"].([]any)
				require.True(t, ok)
				assert.Len(t, descs, tt.wantCount)
			}
		})
	}
}
func TestHandler_ListResources_Pagination(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	for i := range 5 {
		name := "paginated-" + strings.Repeat("x", i+1)
		_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"`+name+`"}`, "")
	}

	// First page: 3 items
	rec := doRequest(t, h, "ListResources", map[string]any{
		"TypeName":   "AWS::Logs::LogGroup",
		"MaxResults": 3,
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out1))

	descs1, ok := out1["ResourceDescriptions"].([]any)
	require.True(t, ok)
	assert.Len(t, descs1, 3)

	nextToken, ok := out1["NextToken"].(string)
	require.True(t, ok, "NextToken must be present after first page")
	require.NotEmpty(t, nextToken)

	// Second page: remaining 2 items
	rec2 := doRequest(t, h, "ListResources", map[string]any{
		"TypeName":   "AWS::Logs::LogGroup",
		"MaxResults": 3,
		"NextToken":  nextToken,
	})

	require.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))

	descs2, ok := out2["ResourceDescriptions"].([]any)
	require.True(t, ok)
	assert.Len(t, descs2, 2)

	_, hasMore := out2["NextToken"]
	assert.False(t, hasMore, "NextToken must be absent on last page")
}
func TestHandler_DeleteResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*cloudcontrol.Handler)
		body       map[string]any
		name       string
		wantOp     string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"delete-me"}`, "")
			},
			body: map[string]any{
				"TypeName":   "AWS::Logs::LogGroup",
				"Identifier": "delete-me",
			},
			wantStatus: http.StatusOK,
			wantOp:     "DELETE",
		},
		{
			name:  "not found returns 400",
			setup: nil,
			body: map[string]any{
				"TypeName":   "AWS::Logs::LogGroup",
				"Identifier": "nonexistent",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "missing TypeName returns 400",
			setup: nil,
			body: map[string]any{
				"Identifier": "something",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "missing Identifier returns 400",
			setup: nil,
			body: map[string]any{
				"TypeName": "AWS::Logs::LogGroup",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "DeleteResource", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				pe, ok := out["ProgressEvent"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantOp, pe["Operation"])
				assert.Equal(t, "SUCCESS", pe["OperationStatus"])
			}
		})
	}
}
func TestHandler_UpdateResource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup      func(*cloudcontrol.Handler)
		body       map[string]any
		name       string
		wantOp     string
		wantStatus int
	}{
		{
			name: "success",
			setup: func(h *cloudcontrol.Handler) {
				_, _ = h.Backend.CreateResource(
					"AWS::Logs::LogGroup",
					`{"LogGroupName":"update-me","RetentionInDays":7}`,
					"",
				)
			},
			body: map[string]any{
				"TypeName":      "AWS::Logs::LogGroup",
				"Identifier":    "update-me",
				"PatchDocument": `[{"op":"replace","path":"/RetentionInDays","value":30}]`,
			},
			wantStatus: http.StatusOK,
			wantOp:     "UPDATE",
		},
		{
			name:  "not found returns 400",
			setup: nil,
			body: map[string]any{
				"TypeName":      "AWS::Logs::LogGroup",
				"Identifier":    "nonexistent",
				"PatchDocument": `[]`,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "missing TypeName returns 400",
			setup: nil,
			body: map[string]any{
				"Identifier":    "something",
				"PatchDocument": `[]`,
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "missing Identifier returns 400",
			setup: nil,
			body: map[string]any{
				"TypeName":      "AWS::Logs::LogGroup",
				"PatchDocument": `[]`,
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := doRequest(t, h, "UpdateResource", tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var out map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
				pe, ok := out["ProgressEvent"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.wantOp, pe["Operation"])
				assert.Equal(t, "SUCCESS", pe["OperationStatus"])
			}
		})
	}
}

// TestHandler_UpdateResource_ClientToken verifies that UpdateResourceInput.ClientToken --
// a real field on the SDK's UpdateResourceInput, previously silently dropped by
// gopherstack's updateResourceInput -- provides the same idempotent-replay
// behavior as CreateResource's ClientToken: a repeated call with the same token
// returns the original ProgressEvent (same RequestToken) without re-applying the
// patch a second time.
func TestHandler_UpdateResource_ClientToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateResource(
		"AWS::Logs::LogGroup", `{"LogGroupName":"update-token-test","RetentionInDays":7}`, "",
	)

	body := map[string]any{
		"TypeName":      "AWS::Logs::LogGroup",
		"Identifier":    "update-token-test",
		"PatchDocument": `[{"op":"replace","path":"/RetentionInDays","value":30}]`,
		"ClientToken":   "update-client-token-1",
	}

	rec1 := doRequest(t, h, "UpdateResource", body)
	require.Equal(t, http.StatusOK, rec1.Code)

	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &out1))
	pe1 := out1["ProgressEvent"].(map[string]any)

	rec2 := doRequest(t, h, "UpdateResource", body)
	require.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))
	pe2 := out2["ProgressEvent"].(map[string]any)

	assert.Equal(t, pe1["RequestToken"], pe2["RequestToken"],
		"idempotent UpdateResource calls must return the same request token")
}

// TestBackend_UpdateResource_ClientToken_Idempotency is the backend-level
// counterpart of TestHandler_UpdateResource_ClientToken: it additionally
// verifies the patch was applied exactly once.
func TestBackend_UpdateResource_ClientToken_Idempotency(t *testing.T) {
	t.Parallel()

	b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"update-idem","Count":1}`, "")
	require.NoError(t, err)

	patch := `[{"op":"replace","path":"/Count","value":2}]`

	ev1, err := b.UpdateResource("AWS::Logs::LogGroup", "update-idem", patch, "my-update-token")
	require.NoError(t, err)

	ev2, err := b.UpdateResource("AWS::Logs::LogGroup", "update-idem", patch, "my-update-token")
	require.NoError(t, err)

	assert.Equal(t, ev1.RequestToken, ev2.RequestToken, "idempotent calls must return the same request token")

	r, err := b.GetResource("AWS::Logs::LogGroup", "update-idem")
	require.NoError(t, err)
	assert.JSONEq(t, `{"LogGroupName":"update-idem","Count":2}`, r.Properties,
		"patch must only be applied once")
}

// TestBackend_UpdateResource_NestedPatchPaths verifies that PatchDocument's Path is
// resolved as a real RFC 6901 JSON Pointer -- navigating into nested objects and
// array elements -- rather than treated as a literal top-level map key. The real
// UpdateResourceInput.PatchDocument is "a JSON document listing the patch operations"
// that "adheres to the RFC 6902 ... standard" (api_op_UpdateResource.go), whose Path
// members are routinely multi-segment for real resource shapes (e.g. Environment
// variables, Tags arrays).
func TestBackend_UpdateResource_NestedPatchPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initial    string
		patch      string
		wantResult string
	}{
		{
			name:       "replace nested object field",
			initial:    `{"Id":"r1","Nested":{"Field":"old"}}`,
			patch:      `[{"op":"replace","path":"/Nested/Field","value":"new"}]`,
			wantResult: `{"Id":"r1","Nested":{"Field":"new"}}`,
		},
		{
			name:       "add nested object field",
			initial:    `{"Id":"r1","Nested":{}}`,
			patch:      `[{"op":"add","path":"/Nested/Field","value":"created"}]`,
			wantResult: `{"Id":"r1","Nested":{"Field":"created"}}`,
		},
		{
			name:       "remove nested object field",
			initial:    `{"Id":"r1","Nested":{"Field":"gone","Keep":1}}`,
			patch:      `[{"op":"remove","path":"/Nested/Field"}]`,
			wantResult: `{"Id":"r1","Nested":{"Keep":1}}`,
		},
		{
			name:       "replace array element by index",
			initial:    `{"Id":"r1","Tags":[{"Key":"a","Value":"old"}]}`,
			patch:      `[{"op":"replace","path":"/Tags/0/Value","value":"new"}]`,
			wantResult: `{"Id":"r1","Tags":[{"Key":"a","Value":"new"}]}`,
		},
		{
			name:       "add element at array end via dash",
			initial:    `{"Id":"r1","Tags":["a"]}`,
			patch:      `[{"op":"add","path":"/Tags/-","value":"b"}]`,
			wantResult: `{"Id":"r1","Tags":["a","b"]}`,
		},
		{
			name:       "add element at array index shifts remainder",
			initial:    `{"Id":"r1","Tags":["a","c"]}`,
			patch:      `[{"op":"add","path":"/Tags/1","value":"b"}]`,
			wantResult: `{"Id":"r1","Tags":["a","b","c"]}`,
		},
		{
			name:       "remove element from array",
			initial:    `{"Id":"r1","Tags":["a","b","c"]}`,
			patch:      `[{"op":"remove","path":"/Tags/1"}]`,
			wantResult: `{"Id":"r1","Tags":["a","c"]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreateResource("AWS::Test::Resource", tt.initial, "")
			require.NoError(t, err)

			_, err = b.UpdateResource("AWS::Test::Resource", "r1", tt.patch, "")
			require.NoError(t, err)

			r, err := b.GetResource("AWS::Test::Resource", "r1")
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantResult, r.Properties)
		})
	}
}

// TestBackend_UpdateResource_MoveOp verifies RFC 6902 4.4 "move": the value at
// From is removed, then added at Path. gopherstack-j6lv.
func TestBackend_UpdateResource_MoveOp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initial    string
		patch      string
		wantResult string
	}{
		{
			name:       "move top-level field",
			initial:    `{"Id":"r1","Source":"v"}`,
			patch:      `[{"op":"move","from":"/Source","path":"/Dest"}]`,
			wantResult: `{"Id":"r1","Dest":"v"}`,
		},
		{
			name:       "move into nested object",
			initial:    `{"Id":"r1","Source":"v","Nested":{}}`,
			patch:      `[{"op":"move","from":"/Source","path":"/Nested/Dest"}]`,
			wantResult: `{"Id":"r1","Nested":{"Dest":"v"}}`,
		},
		{
			name:       "move array element shifts index",
			initial:    `{"Id":"r1","Tags":["a","b","c"]}`,
			patch:      `[{"op":"move","from":"/Tags/0","path":"/Tags/2"}]`,
			wantResult: `{"Id":"r1","Tags":["b","c","a"]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreateResource("AWS::Test::Resource", tt.initial, "")
			require.NoError(t, err)

			_, err = b.UpdateResource("AWS::Test::Resource", "r1", tt.patch, "")
			require.NoError(t, err)

			r, err := b.GetResource("AWS::Test::Resource", "r1")
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantResult, r.Properties)
		})
	}
}

// TestBackend_UpdateResource_MoveOp_FromIsPrefixOfPath_Rejected verifies RFC
// 6902 4.4's "a location cannot be moved into one of its children" rule, and
// that a rejected move leaves the resource entirely unchanged. gopherstack-j6lv.
func TestBackend_UpdateResource_MoveOp_FromIsPrefixOfPath_Rejected(t *testing.T) {
	t.Parallel()

	b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
	initial := `{"Id":"r1","Parent":{"Child":"v"}}`
	_, err := b.CreateResource("AWS::Test::Resource", initial, "")
	require.NoError(t, err)

	patch := `[{"op":"move","from":"/Parent","path":"/Parent/Child2"}]`
	_, err = b.UpdateResource("AWS::Test::Resource", "r1", patch, "")
	require.ErrorIs(t, err, cloudcontrol.ErrValidation)

	r, err := b.GetResource("AWS::Test::Resource", "r1")
	require.NoError(t, err)
	assert.JSONEq(t, initial, r.Properties, "a rejected move must not mutate the resource")
}

// TestBackend_UpdateResource_CopyOp verifies RFC 6902 4.5 "copy": the value
// at From is added at Path, and From is left untouched. gopherstack-j6lv.
func TestBackend_UpdateResource_CopyOp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initial    string
		patch      string
		wantResult string
	}{
		{
			name:       "copy top-level field",
			initial:    `{"Id":"r1","Source":"v"}`,
			patch:      `[{"op":"copy","from":"/Source","path":"/Dest"}]`,
			wantResult: `{"Id":"r1","Source":"v","Dest":"v"}`,
		},
		{
			name:       "copy nested object",
			initial:    `{"Id":"r1","Source":{"A":1}}`,
			patch:      `[{"op":"copy","from":"/Source","path":"/Dest"}]`,
			wantResult: `{"Id":"r1","Source":{"A":1},"Dest":{"A":1}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreateResource("AWS::Test::Resource", tt.initial, "")
			require.NoError(t, err)

			_, err = b.UpdateResource("AWS::Test::Resource", "r1", tt.patch, "")
			require.NoError(t, err)

			r, err := b.GetResource("AWS::Test::Resource", "r1")
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantResult, r.Properties)
		})
	}
}

// TestBackend_UpdateResource_CopyOp_DeepCopyIndependence verifies that "copy"
// performs a DEEP copy, not just a fresh outer map: within a SINGLE patch
// document, a copy immediately followed by a replace that mutates a NESTED
// path under the copy's source must not also mutate the previously copied
// destination.
//
// Both the single-patch-document requirement and the nesting depth (Source
// is two levels deep, {"Inner":{"A":1}}) are load-bearing, not incidental:
//   - Properties is a JSON string, so every UpdateResource call starts from a
//     fresh json.Unmarshal of it -- any aliasing from a copy is already gone
//     by the time a SECOND UpdateResource call runs a mutation, so a
//     two-call version of this test cannot observe aliasing at all.
//   - A shallow copy that only allocates a fresh OUTER map (copying inner
//     values by reference) already looks independent one level deep, since
//     a scalar value like {"A":1}'s "A" is a Go value type -- copying the
//     reference to it is indistinguishable from copying the value itself.
//     The shared reference only becomes observable by replacing through a
//     nested map (/Source/Inner/A) that only a genuinely recursive deep copy
//     would have cloned. gopherstack-j6lv.
func TestBackend_UpdateResource_CopyOp_DeepCopyIndependence(t *testing.T) {
	t.Parallel()

	b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateResource("AWS::Test::Resource", `{"Id":"r1","Source":{"Inner":{"A":1}}}`, "")
	require.NoError(t, err)

	patch := `[
		{"op":"copy","from":"/Source","path":"/Dest"},
		{"op":"replace","path":"/Source/Inner/A","value":2}
	]`
	_, err = b.UpdateResource("AWS::Test::Resource", "r1", patch, "")
	require.NoError(t, err)

	r, err := b.GetResource("AWS::Test::Resource", "r1")
	require.NoError(t, err)

	wantResult := `{"Id":"r1","Source":{"Inner":{"A":2}},"Dest":{"Inner":{"A":1}}}`
	wantMsg := "a later op in the SAME patch mutating a nested path under the source " +
		"must not affect the copied destination"
	assert.JSONEq(t, wantResult, r.Properties, wantMsg)
}

// TestBackend_UpdateResource_TestOp_Passes verifies RFC 6902 4.6 "test"
// succeeds by JSON structural equality (not Go ==): a JSON number 1 sent by
// the caller must match a stored value that separately round-tripped as 1.0.
// A passing test lets the rest of the patch apply. gopherstack-j6lv.
func TestBackend_UpdateResource_TestOp_Passes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial string
		patch   string
	}{
		{
			name:    "matching scalar",
			initial: `{"Id":"r1","Count":1}`,
			patch:   `[{"op":"test","path":"/Count","value":1},{"op":"replace","path":"/Count","value":2}]`,
		},
		{
			name:    "integer matches float value",
			initial: `{"Id":"r1","Count":1}`,
			patch:   `[{"op":"test","path":"/Count","value":1.0},{"op":"replace","path":"/Count","value":2}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreateResource("AWS::Test::Resource", tt.initial, "")
			require.NoError(t, err)

			_, err = b.UpdateResource("AWS::Test::Resource", "r1", tt.patch, "")
			require.NoError(t, err)

			r, err := b.GetResource("AWS::Test::Resource", "r1")
			require.NoError(t, err)
			assert.JSONEq(t, `{"Id":"r1","Count":2}`, r.Properties)
		})
	}
}

// TestBackend_UpdateResource_TestOp_Fails verifies RFC 6902 4.6 "test"
// rejects a mismatched value or an unresolvable path with ErrValidation
// (InvalidRequestException on the wire -- CloudControl's UpdateResource
// declares no more specific client error for a failed patch operation; see
// PARITY.md for the full reasoning). gopherstack-j6lv.
func TestBackend_UpdateResource_TestOp_Fails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial string
		patch   string
	}{
		{
			name:    "value mismatch",
			initial: `{"Id":"r1","Count":1}`,
			patch:   `[{"op":"test","path":"/Count","value":2}]`,
		},
		{
			name:    "path missing",
			initial: `{"Id":"r1"}`,
			patch:   `[{"op":"test","path":"/Missing","value":"x"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
			_, err := b.CreateResource("AWS::Test::Resource", tt.initial, "")
			require.NoError(t, err)

			_, err = b.UpdateResource("AWS::Test::Resource", "r1", tt.patch, "")
			require.ErrorIs(t, err, cloudcontrol.ErrValidation)
		})
	}
}

// TestBackend_UpdateResource_TestOp_Fails_AbortsWholePatch verifies RFC 6902's
// atomic-patch semantics: a "test" failure partway through a patch discards
// EVERY op in that patch, including ops earlier in the same document that had
// already mutated the working document before the failure. gopherstack-j6lv.
func TestBackend_UpdateResource_TestOp_Fails_AbortsWholePatch(t *testing.T) {
	t.Parallel()

	b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
	initial := `{"Id":"r1","Count":1}`
	_, err := b.CreateResource("AWS::Test::Resource", initial, "")
	require.NoError(t, err)

	patch := `[{"op":"replace","path":"/Count","value":99},{"op":"test","path":"/Count","value":999}]`
	_, err = b.UpdateResource("AWS::Test::Resource", "r1", patch, "")
	require.ErrorIs(t, err, cloudcontrol.ErrValidation)

	r, err := b.GetResource("AWS::Test::Resource", "r1")
	require.NoError(t, err)
	assert.JSONEq(t, initial, r.Properties,
		"a later op's failure must discard an earlier op's mutation in the same patch")
}

// TestHandler_DeleteResource_ClientToken verifies that DeleteResourceInput.ClientToken --
// a real field on the SDK's DeleteResourceInput, previously silently dropped by
// gopherstack's deleteResourceInput -- provides idempotent-replay behavior: a
// repeated delete with the same token returns the original ProgressEvent instead
// of ResourceNotFoundException for the now-already-deleted resource.
func TestHandler_DeleteResource_ClientToken(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"delete-token-test"}`, "")

	body := map[string]any{
		"TypeName":    "AWS::Logs::LogGroup",
		"Identifier":  "delete-token-test",
		"ClientToken": "delete-client-token-1",
	}

	rec1 := doRequest(t, h, "DeleteResource", body)
	require.Equal(t, http.StatusOK, rec1.Code)

	var out1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &out1))
	pe1 := out1["ProgressEvent"].(map[string]any)

	// Without idempotency this second call would 400 with ResourceNotFoundException,
	// since the resource is already gone.
	rec2 := doRequest(t, h, "DeleteResource", body)
	require.Equal(t, http.StatusOK, rec2.Code)

	var out2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out2))
	pe2 := out2["ProgressEvent"].(map[string]any)

	assert.Equal(t, pe1["RequestToken"], pe2["RequestToken"],
		"idempotent DeleteResource calls must return the same request token")
}

// TestBackend_DeleteResource_ClientToken_Idempotency is the backend-level
// counterpart of TestHandler_DeleteResource_ClientToken.
func TestBackend_DeleteResource_ClientToken_Idempotency(t *testing.T) {
	t.Parallel()

	b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"delete-idem"}`, "")
	require.NoError(t, err)

	ev1, err := b.DeleteResource("AWS::Logs::LogGroup", "delete-idem", "my-delete-token")
	require.NoError(t, err)

	ev2, err := b.DeleteResource("AWS::Logs::LogGroup", "delete-idem", "my-delete-token")
	require.NoError(t, err)

	assert.Equal(t, ev1.RequestToken, ev2.RequestToken, "idempotent calls must return the same request token")
}

// TestBackend_ClientToken_ReuseAcrossDifferentRequest_ReturnsConflict covers the gap flagged
// in bd issue gopherstack-c9yf: reusing the same ClientToken across a genuinely different
// request (different TypeName/Identifier/DesiredState/PatchDocument) must return
// ClientTokenConflictException, not silently replay the first request's cached result nor
// silently process the second request as if the token were new.
func TestBackend_ClientToken_ReuseAcrossDifferentRequest_ReturnsConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		conflicting func(b *cloudcontrol.InMemoryBackend) error
		name        string
	}{
		{
			name: "CreateResource same token different DesiredState",
			conflicting: func(b *cloudcontrol.InMemoryBackend) error {
				_, err := b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"a"}`, "shared-token")
				require.NoError(t, err)

				_, err = b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"b"}`, "shared-token")

				return err
			},
		},
		{
			name: "CreateResource same token different TypeName",
			conflicting: func(b *cloudcontrol.InMemoryBackend) error {
				_, err := b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"c"}`, "shared-token-2")
				require.NoError(t, err)

				_, err = b.CreateResource("AWS::S3::Bucket", `{"BucketName":"c"}`, "shared-token-2")

				return err
			},
		},
		{
			name: "UpdateResource same token different PatchDocument",
			conflicting: func(b *cloudcontrol.InMemoryBackend) error {
				_, err := b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"u","Count":1}`, "")
				require.NoError(t, err)

				_, err = b.UpdateResource(
					"AWS::Logs::LogGroup", "u", `[{"op":"replace","path":"/Count","value":2}]`, "update-token",
				)
				require.NoError(t, err)

				_, err = b.UpdateResource(
					"AWS::Logs::LogGroup", "u", `[{"op":"replace","path":"/Count","value":3}]`, "update-token",
				)

				return err
			},
		},
		{
			name: "DeleteResource same token different Identifier",
			conflicting: func(b *cloudcontrol.InMemoryBackend) error {
				_, err := b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"d1"}`, "")
				require.NoError(t, err)
				_, err = b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"d2"}`, "")
				require.NoError(t, err)

				_, err = b.DeleteResource("AWS::Logs::LogGroup", "d1", "delete-token")
				require.NoError(t, err)

				_, err = b.DeleteResource("AWS::Logs::LogGroup", "d2", "delete-token")

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
			err := tt.conflicting(b)
			require.ErrorIs(t, err, cloudcontrol.ErrClientTokenConflict)
		})
	}
}

// TestHandler_ClientTokenConflict_WireShape verifies ClientTokenConflictException is
// returned on the wire with HTTP 400 (a client fault per the real SDK's
// types.ClientTokenConflictException.ErrorFault()), through the real router path.
func TestHandler_ClientTokenConflict_WireShape(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec1 := doRequest(t, h, "CreateResource", map[string]any{
		"TypeName":     "AWS::Logs::LogGroup",
		"DesiredState": `{"LogGroupName":"wire-a"}`,
		"ClientToken":  "wire-shared-token",
	})
	require.Equal(t, http.StatusOK, rec1.Code)

	rec2 := doRequest(t, h, "CreateResource", map[string]any{
		"TypeName":     "AWS::Logs::LogGroup",
		"DesiredState": `{"LogGroupName":"wire-b"}`,
		"ClientToken":  "wire-shared-token",
	})
	require.Equal(t, http.StatusBadRequest, rec2.Code)
	assert.Equal(t, "ClientTokenConflictException", errType(t, rec2.Body.Bytes()))
}

func TestInMemoryBackend_ListAllResources(t *testing.T) {
	t.Parallel()

	b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
	_, _ = b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"all-1"}`, "")
	_, _ = b.CreateResource("AWS::S3::Bucket", `{"BucketName":"all-2"}`, "")

	all := b.ListAllResources()
	assert.Len(t, all, 2)
}
func TestBackend_ListAllResources_SortedOutput(t *testing.T) {
	t.Parallel()

	b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
	_, _ = b.CreateResource("AWS::S3::Bucket", `{"BucketName":"z-bucket"}`, "")
	_, _ = b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"a-group"}`, "")
	_, _ = b.CreateResource("AWS::S3::Bucket", `{"BucketName":"a-bucket"}`, "")

	all := b.ListAllResources()
	require.Len(t, all, 3)

	// Sorted by TypeName then Identifier.
	assert.Equal(t, "AWS::Logs::LogGroup", all[0].TypeName)
	assert.Equal(t, "AWS::S3::Bucket", all[1].TypeName)
	assert.Equal(t, "a-bucket", all[1].Identifier)
	assert.Equal(t, "z-bucket", all[2].Identifier)
}

// TestBackend_ListAllResources_ReturnsCopiesNotLiveState locks in a fix: List(All)Resources
// previously handed back the *Resource pointers live inside the backend's
// store.Table (store.Table.All/Range perform no copying themselves -- see
// pkgs/store's package doc -- so that responsibility falls on the backend).
// Every other accessor (GetResource, the ProgressEvent returned by
// Create/Update/DeleteResource) already returns a defensive copy; a caller
// mutating a *Resource obtained from ListAllResources must never be able to
// corrupt backend state without holding the lock.
func TestBackend_ListAllResources_ReturnsCopiesNotLiveState(t *testing.T) {
	t.Parallel()

	b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"copy-test"}`, "")
	require.NoError(t, err)

	all := b.ListAllResources()
	require.Len(t, all, 1)

	all[0].Properties = `{"tampered":true}`

	r, err := b.GetResource("AWS::Logs::LogGroup", "copy-test")
	require.NoError(t, err)
	assert.JSONEq(t, `{"LogGroupName":"copy-test"}`, r.Properties,
		"mutating a ListAllResources result must not affect backend state")
}

// TestBackend_ListResources_ReturnsCopiesNotLiveState is the ListResources
// (per-TypeName, paginated) counterpart of
// TestBackend_ListAllResources_ReturnsCopiesNotLiveState.
func TestBackend_ListResources_ReturnsCopiesNotLiveState(t *testing.T) {
	t.Parallel()

	b := cloudcontrol.NewInMemoryBackend("000000000000", "us-east-1")
	_, err := b.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"copy-test-2"}`, "")
	require.NoError(t, err)

	page, _ := b.ListResources("AWS::Logs::LogGroup", 0, "", "")
	require.Len(t, page, 1)

	page[0].Properties = `{"tampered":true}`

	r, err := b.GetResource("AWS::Logs::LogGroup", "copy-test-2")
	require.NoError(t, err)
	assert.JSONEq(t, `{"LogGroupName":"copy-test-2"}`, r.Properties,
		"mutating a ListResources result must not affect backend state")
}
func TestHandler_TypeName_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body       map[string]any
		name       string
		action     string
		wantStatus int
	}{
		{
			name:       "create with invalid type name",
			action:     "CreateResource",
			body:       map[string]any{"TypeName": "InvalidTypeName", "DesiredState": `{}`},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create with empty type name",
			action:     "CreateResource",
			body:       map[string]any{"TypeName": "", "DesiredState": `{}`},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "get with invalid type name",
			action:     "GetResource",
			body:       map[string]any{"TypeName": "NoDoubleColons", "Identifier": "x"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "delete with invalid type name",
			action:     "DeleteResource",
			body:       map[string]any{"TypeName": "Bad", "Identifier": "x"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "update with invalid type name",
			action:     "UpdateResource",
			body:       map[string]any{"TypeName": "Bad", "Identifier": "x", "PatchDocument": "[]"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "list with invalid type name",
			action:     "ListResources",
			body:       map[string]any{"TypeName": "NoColons"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			rec := doRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_MissingRequiredField_IsInvalidRequestException verifies that CloudControl's
// generic input-validation error is InvalidRequestException, not ValidationException --
// CloudControl's error model has no ValidationException shape at all. See
// https://docs.aws.amazon.com/cloudcontrolapi/latest/APIReference/API_CreateResource.html#API_CreateResource_Errors
func TestHandler_MissingRequiredField_IsInvalidRequestException(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateResource", map[string]any{"DesiredState": `{}`})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "InvalidRequestException", errType(t, rec.Body.Bytes()))
}

// TestHandler_CreateResource_MissingDesiredState_Is400 verifies that DesiredState --
// "This member is required" on the real SDK's CreateResourceInput -- is enforced.
// A prior gopherstack pass validated TypeName but silently accepted a missing/empty
// DesiredState, creating a resource with empty Properties instead of rejecting the
// request the way real AWS would reject a missing required member.
func TestHandler_CreateResource_MissingDesiredState_Is400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "CreateResource", map[string]any{"TypeName": "AWS::Logs::LogGroup"})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "InvalidRequestException", errType(t, rec.Body.Bytes()))
	assert.Empty(t, h.Backend.ListAllResources(), "no resource must be created")
}

// TestHandler_UpdateResource_MissingPatchDocument_Is400 verifies that PatchDocument --
// "This member is required" on the real SDK's UpdateResourceInput -- is enforced.
// A prior gopherstack pass validated TypeName/Identifier but silently accepted a
// missing/empty PatchDocument, which applyPatch then silently no-oped on instead of
// the request being rejected the way real AWS would reject a missing required member.
func TestHandler_UpdateResource_MissingPatchDocument_Is400(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	_, _ = h.Backend.CreateResource("AWS::Logs::LogGroup", `{"LogGroupName":"no-patch-doc"}`, "")

	rec := doRequest(t, h, "UpdateResource", map[string]any{
		"TypeName":   "AWS::Logs::LogGroup",
		"Identifier": "no-patch-doc",
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "InvalidRequestException", errType(t, rec.Body.Bytes()))
}

// TestHandler_ProgressEvent_RequiredFields verifies the ProgressEvent shape on create/delete/update.
func TestHandler_ProgressEvent_RequiredFields(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	tests := []struct {
		setup     func() map[string]any
		name      string
		operation string
	}{
		{
			name:      "create_returns_progress_event",
			operation: "CreateResource",
			setup: func() map[string]any {
				return map[string]any{
					"TypeName":     "AWS::Logs::LogGroup",
					"DesiredState": `{"LogGroupName":"parity-grp"}`,
				}
			},
		},
		{
			name:      "delete_returns_progress_event",
			operation: "DeleteResource",
			setup: func() map[string]any {
				createRec := doRequest(t, h, "CreateResource", map[string]any{
					"TypeName":     "AWS::Logs::LogGroup",
					"DesiredState": `{"LogGroupName":"del-grp"}`,
				})
				require.Equal(t, http.StatusOK, createRec.Code)
				var out map[string]any
				require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &out))
				pe := out["ProgressEvent"].(map[string]any)

				return map[string]any{
					"TypeName":   "AWS::Logs::LogGroup",
					"Identifier": pe["Identifier"],
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := doRequest(t, h, tt.operation, tt.setup())
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			pe, ok := out["ProgressEvent"].(map[string]any)
			require.True(t, ok, "ProgressEvent must be present")

			// All required ProgressEvent fields.
			assert.NotEmpty(t, pe["TypeName"], "TypeName required")
			assert.NotEmpty(t, pe["RequestToken"], "RequestToken required")
			assert.NotEmpty(t, pe["Operation"], "Operation required")
			assert.NotEmpty(t, pe["OperationStatus"], "OperationStatus required")
			assert.NotEmpty(t, pe["EventTime"], "EventTime required")
		})
	}
}
