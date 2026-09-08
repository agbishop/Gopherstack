package verifiedpermissions_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/verifiedpermissions"
)

func TestVPHandler_CreatePolicyStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		wantKey  string
		wantCode int
	}{
		{
			name: "create with description",
			body: map[string]any{
				"description":        "My test store",
				"validationSettings": map[string]any{"mode": "OFF"},
			},
			wantCode: http.StatusOK,
			wantKey:  "policyStoreId",
		},
		{
			name:     "create without description",
			body:     map[string]any{"validationSettings": map[string]any{"mode": "OFF"}},
			wantCode: http.StatusOK,
			wantKey:  "policyStoreId",
		},
		{
			name:     "create without validationSettings",
			body:     map[string]any{"description": "no validation settings"},
			wantCode: http.StatusBadRequest,
			wantKey:  "__type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			rec := doVPRequest(t, h, "CreatePolicyStore", tt.body)

			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Contains(t, resp, tt.wantKey)
		})
	}
}

func TestVPHandler_GetPolicyStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*testing.T, *verifiedpermissions.Handler) string
		name     string
		wantCode int
	}{
		{
			name: "get existing store",
			setup: func(t *testing.T, h *verifiedpermissions.Handler) string {
				t.Helper()

				rec := doVPRequest(
					t,
					h,
					"CreatePolicyStore",
					map[string]any{"description": "test", "validationSettings": map[string]any{"mode": "OFF"}},
				)
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				return resp["policyStoreId"].(string)
			},
			wantCode: http.StatusOK,
		},
		{
			name: "get non-existent store",
			setup: func(_ *testing.T, _ *verifiedpermissions.Handler) string {
				return "nonexistent-id"
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			id := tt.setup(t, h)

			rec := doVPRequest(t, h, "GetPolicyStore", map[string]any{"policyStoreId": id})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestVPHandler_ListPolicyStores(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		numStores int
		wantCode  int
	}{
		{
			name:      "list empty",
			numStores: 0,
			wantCode:  http.StatusOK,
		},
		{
			name:      "list with stores",
			numStores: 2,
			wantCode:  http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)

			for range tt.numStores {
				doVPRequest(
					t,
					h,
					"CreatePolicyStore",
					map[string]any{"description": "test", "validationSettings": map[string]any{"mode": "OFF"}},
				)
			}

			rec := doVPRequest(t, h, "ListPolicyStores", map[string]any{})
			assert.Equal(t, tt.wantCode, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			stores := resp["policyStores"].([]any)
			assert.Len(t, stores, tt.numStores)
		})
	}
}

func TestVPHandler_DeletePolicyStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*testing.T, *verifiedpermissions.Handler) string
		name     string
		wantCode int
	}{
		{
			name: "delete existing",
			setup: func(t *testing.T, h *verifiedpermissions.Handler) string {
				t.Helper()

				rec := doVPRequest(
					t,
					h,
					"CreatePolicyStore",
					map[string]any{"description": "test", "validationSettings": map[string]any{"mode": "OFF"}},
				)
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				return resp["policyStoreId"].(string)
			},
			wantCode: http.StatusOK,
		},
		{
			// DeletePolicyStore is documented idempotent: deleting a
			// nonexistent store still returns HTTP 200.
			name: "delete non-existent",
			setup: func(_ *testing.T, _ *verifiedpermissions.Handler) string {
				return "nonexistent-id"
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			id := tt.setup(t, h)

			rec := doVPRequest(t, h, "DeletePolicyStore", map[string]any{"policyStoreId": id})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestVPHandler_UpdatePolicyStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*testing.T, *verifiedpermissions.Handler) string
		name     string
		wantCode int
	}{
		{
			name: "update existing store",
			setup: func(t *testing.T, h *verifiedpermissions.Handler) string {
				t.Helper()

				rec := doVPRequest(
					t,
					h,
					"CreatePolicyStore",
					map[string]any{"description": "original", "validationSettings": map[string]any{"mode": "OFF"}},
				)
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

				return resp["policyStoreId"].(string)
			},
			wantCode: http.StatusOK,
		},
		{
			name: "update non-existent store",
			setup: func(_ *testing.T, _ *verifiedpermissions.Handler) string {
				return "nonexistent-id"
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing policyStoreId",
			setup: func(_ *testing.T, _ *verifiedpermissions.Handler) string {
				return ""
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			id := tt.setup(t, h)

			rec := doVPRequest(t, h, "UpdatePolicyStore", map[string]any{
				"policyStoreId": id,
				"description":   "updated",
			})
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestVPHandler_GetPolicyStore_MissingID(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	rec := doVPRequest(t, h, "GetPolicyStore", map[string]any{"policyStoreId": ""})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestVPHandler_DeletePolicyStore_MissingID(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	rec := doVPRequest(t, h, "DeletePolicyStore", map[string]any{"policyStoreId": ""})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestVPHandler_UpdatePolicyStore_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	rec := doVPRequest(t, h, "UpdatePolicyStore", map[string]any{
		"policyStoreId": "nonexistent-id",
		"description":   "updated",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestVPHandler_CreatePolicyStore_WithTags(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	rec := doVPRequest(t, h, "CreatePolicyStore", map[string]any{
		"description":        "tagged store",
		"tags":               map[string]any{"env": "prod", "team": "platform"},
		"validationSettings": map[string]any{"mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["policyStoreId"])
	assert.NotEmpty(t, resp["arn"])
}

// createTestPolicyStore is a helper to create a policy store and return its ID.

func TestVPHandler_DeletePolicyStore_CascadesIdentitySources(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeID := createTestPolicyStore(t, h)

	// Create identity source
	rec := doVPRequest(t, h, "CreateIdentitySource", map[string]any{
		"policyStoreId":       storeID,
		"principalEntityType": "MyCorp::User",
		"configuration": map[string]any{
			"cognitoUserPoolConfiguration": map[string]any{
				"userPoolArn": "arn:aws:cognito-idp:us-east-1:123456789012:userpool/us-east-1_test",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	// Delete the policy store
	rec = doVPRequest(t, h, "DeletePolicyStore", map[string]any{"policyStoreId": storeID})
	require.Equal(t, http.StatusOK, rec.Code)

	// Try to list identity sources - policy store should be gone
	rec = doVPRequest(t, h, "ListIdentitySources", map[string]any{"policyStoreId": storeID})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestVPHandler_DeletePolicyStore_DeletionProtection_InvalidStateException is
// the regression test for gopherstack-990: real AWS's InvalidStateException
// doc comment (types/errors.go) names this exact condition -- "The policy
// store can't be deleted because deletion protection is enabled" -- and
// ConflictException isn't even in DeletePolicyStore's modelled error set.
func TestVPHandler_DeletePolicyStore_DeletionProtection_InvalidStateException(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)

	createRec := doVPRequest(t, h, "CreatePolicyStore", map[string]any{
		"validationSettings": map[string]any{"mode": "OFF"},
		"deletionProtection": "ENABLED",
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	storeID := createResp["policyStoreId"].(string)

	rec := doVPRequest(t, h, "DeletePolicyStore", map[string]any{"policyStoreId": storeID})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "InvalidStateException", resp["__type"])
}

// TestVPHandler_TagResource_TooManyTags verifies exceeding the 50-tag limit
// via TagResource surfaces the real SDK's TooManyTagsException wire type.

func TestVPHandler_CreatePolicyStore_DescriptionBound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		descLen  int
		wantCode int
	}{
		{name: "at_bound_ok", descLen: 150, wantCode: http.StatusOK},
		{name: "over_bound_rejected", descLen: 151, wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			rec := doVPRequest(t, h, "CreatePolicyStore", map[string]any{
				"validationSettings": map[string]any{"mode": "OFF"},
				"description":        strings.Repeat("d", tt.descLen),
			})

			assert.Equal(t, tt.wantCode, rec.Code, "body: %s", rec.Body.String())
		})
	}
}

// TestVPHandler_CreatePolicyStore_WireShape locks in a wire-shape bug fix:
// the real SDK's CreatePolicyStoreOutput has no validationSettings field at
// all (only arn/createdDate/lastUpdatedDate/policyStoreId) -- gopherstack
// previously echoed the input validationSettings back, a field the real
// client-side deserializer never expects here.
func TestVPHandler_CreatePolicyStore_WireShape(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	rec := doVPRequest(t, h, "CreatePolicyStore", map[string]any{
		"validationSettings": map[string]any{"mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.NotContains(t, resp, "validationSettings")
	assert.NotEmpty(t, resp["policyStoreId"])
	assert.NotEmpty(t, resp["arn"])
	assert.NotEmpty(t, resp["createdDate"])
	assert.NotEmpty(t, resp["lastUpdatedDate"])
}

// TestVPHandler_GetPolicyStore_CedarVersion verifies GetPolicyStore always
// populates the optional cedarVersion field (Amazon Verified Permissions'
// Cedar v4 FAQ) -- gopherstack's cedar-go evaluation engine implements
// Cedar 4, so every policy store reports CEDAR_4.
func TestVPHandler_GetPolicyStore_CedarVersion(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeID := createTestPolicyStore(t, h)

	rec := doVPRequest(t, h, "GetPolicyStore", map[string]any{"policyStoreId": storeID})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "CEDAR_4", resp["cedarVersion"])
}

// TestVPHandler_ListPolicyStores_WireShape locks in a wire-shape bug fix:
// the real SDK's PolicyStoreItem (ListPolicyStores) is a leaner shape than
// GetPolicyStoreOutput -- no validationSettings or deletionProtection --
// gopherstack previously echoed both, fields the real item type doesn't
// declare at all.
func TestVPHandler_ListPolicyStores_WireShape(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	createTestPolicyStore(t, h)

	rec := doVPRequest(t, h, "ListPolicyStores", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	items, _ := resp["policyStores"].([]any)
	require.NotEmpty(t, items)
	item, _ := items[0].(map[string]any)
	assert.NotContains(t, item, "validationSettings")
	assert.NotContains(t, item, "deletionProtection")
	assert.NotContains(t, item, "cedarVersion")
}

// TestVPHandler_UpdatePolicyStore_WireShape locks in a wire-shape bug fix:
// the real SDK's UpdatePolicyStoreOutput requires createdDate (the store
// already existed) and, like CreatePolicyStoreOutput, has no
// validationSettings field -- gopherstack previously omitted createdDate
// (a required field) and echoed validationSettings (an invented one).
func TestVPHandler_UpdatePolicyStore_WireShape(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeID := createTestPolicyStore(t, h)

	rec := doVPRequest(t, h, "UpdatePolicyStore", map[string]any{
		"policyStoreId": storeID,
		"description":   "updated",
	})
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.NotContains(t, resp, "validationSettings")
	assert.NotEmpty(t, resp["createdDate"], "createdDate is a required UpdatePolicyStoreOutput field")
	assert.NotEmpty(t, resp["lastUpdatedDate"])
	assert.Equal(t, storeID, resp["policyStoreId"])
}

// TestVPHandler_CreatePolicyStoreAlias exercises CreatePolicyStoreAlias's
// validation and referential-integrity rules through the real router path:
// aliasName/policyStoreId are required, aliasName must carry the real SDK's
// mandatory "policy-store-alias/" prefix, and the target policy store must
// actually exist (ResourceNotFoundException otherwise -- aliases are a
// referential-integrity feature, so this check matters more than the CRUD).
func TestVPHandler_CreatePolicyStoreAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     func(storeID string) map[string]any
		name     string
		wantType string
		wantCode int
	}{
		{
			name: "create alias for existing store",
			body: func(storeID string) map[string]any {
				return map[string]any{"aliasName": "policy-store-alias/example", "policyStoreId": storeID}
			},
			wantCode: http.StatusOK,
		},
		{
			name: "missing aliasName",
			body: func(storeID string) map[string]any {
				return map[string]any{"policyStoreId": storeID}
			},
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name: "missing policyStoreId",
			body: func(string) map[string]any {
				return map[string]any{"aliasName": "policy-store-alias/example"}
			},
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name: "aliasName missing required prefix",
			body: func(storeID string) map[string]any {
				return map[string]any{"aliasName": "example", "policyStoreId": storeID}
			},
			wantCode: http.StatusBadRequest,
			wantType: "ValidationException",
		},
		{
			name: "target policy store does not exist",
			body: func(string) map[string]any {
				return map[string]any{"aliasName": "policy-store-alias/orphan", "policyStoreId": "nonexistent-id"}
			},
			wantCode: http.StatusBadRequest,
			wantType: "ResourceNotFoundException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			storeID := createTestPolicyStore(t, h)

			rec := doVPRequest(t, h, "CreatePolicyStoreAlias", tt.body(storeID))
			assert.Equal(t, tt.wantCode, rec.Code, "body: %s", rec.Body.String())

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			if tt.wantType != "" {
				assert.Equal(t, tt.wantType, resp["__type"])

				return
			}

			assert.Equal(t, storeID, resp["policyStoreId"])
			assert.NotEmpty(t, resp["aliasArn"])
			assert.NotEmpty(t, resp["createdAt"])
			// Alias ARNs are region-populated, unlike policy store ARNs
			// (arnNoRegion) -- verified against the real SDK's own
			// CreatePolicyStoreAlias example response.
			assert.Contains(t, resp["aliasArn"], "us-east-1")
		})
	}
}

// TestVPHandler_CreatePolicyStoreAlias_IdempotentAndConflict verifies the
// real SDK's documented idempotency (same aliasName+policyStoreId replays
// the existing alias, no duplicate created) and uniqueness (a different
// policyStoreId for the same aliasName is a ConflictException -- alias names
// are unique within account/region).
func TestVPHandler_CreatePolicyStoreAlias_IdempotentAndConflict(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeA := createTestPolicyStore(t, h)
	storeB := createTestPolicyStore(t, h)

	body := map[string]any{"aliasName": "policy-store-alias/dup", "policyStoreId": storeA}

	rec1 := doVPRequest(t, h, "CreatePolicyStoreAlias", body)
	require.Equal(t, http.StatusOK, rec1.Code)

	var resp1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &resp1))

	rec2 := doVPRequest(t, h, "CreatePolicyStoreAlias", body)
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))
	assert.Equal(t, resp1["createdAt"], resp2["createdAt"], "same (aliasName, policyStoreId) replays, no duplicate")

	rec3 := doVPRequest(t, h, "CreatePolicyStoreAlias", map[string]any{
		"aliasName": "policy-store-alias/dup", "policyStoreId": storeB,
	})
	require.Equal(t, http.StatusBadRequest, rec3.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &errResp))
	assert.Equal(t, "ConflictException", errResp["__type"])
}

func TestVPHandler_GetPolicyStoreAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, h *verifiedpermissions.Handler) string
		name     string
		wantCode int
	}{
		{
			name: "existing alias",
			setup: func(t *testing.T, h *verifiedpermissions.Handler) string {
				t.Helper()

				storeID := createTestPolicyStore(t, h)
				rec := doVPRequest(t, h, "CreatePolicyStoreAlias", map[string]any{
					"aliasName": "policy-store-alias/found", "policyStoreId": storeID,
				})
				require.Equal(t, http.StatusOK, rec.Code)

				return "policy-store-alias/found"
			},
			wantCode: http.StatusOK,
		},
		{
			name: "nonexistent alias",
			setup: func(*testing.T, *verifiedpermissions.Handler) string {
				return "policy-store-alias/missing"
			},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			aliasName := tt.setup(t, h)

			rec := doVPRequest(t, h, "GetPolicyStoreAlias", map[string]any{"aliasName": aliasName})
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantCode != http.StatusOK {
				return
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, "Active", resp["state"])
			assert.Equal(t, aliasName, resp["aliasName"])
		})
	}
}

func TestVPHandler_ListPolicyStoreAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		filterSelf bool
		wantCount  int
	}{
		{name: "list all", filterSelf: false, wantCount: 2},
		{name: "list filtered to one store", filterSelf: true, wantCount: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			storeA := createTestPolicyStore(t, h)
			storeB := createTestPolicyStore(t, h)

			doVPRequest(t, h, "CreatePolicyStoreAlias", map[string]any{
				"aliasName": "policy-store-alias/a1", "policyStoreId": storeA,
			})
			doVPRequest(t, h, "CreatePolicyStoreAlias", map[string]any{
				"aliasName": "policy-store-alias/b1", "policyStoreId": storeB,
			})

			body := map[string]any{}
			if tt.filterSelf {
				body["filter"] = map[string]any{"policyStoreId": storeA}
			}

			rec := doVPRequest(t, h, "ListPolicyStoreAliases", body)
			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			items, _ := resp["policyStoreAliases"].([]any)
			assert.Len(t, items, tt.wantCount)
		})
	}
}

// TestVPHandler_DeletePolicyStoreAlias exercises the real SDK's documented
// DeletionMode semantics: default/SoftDelete transitions the alias to
// PendingDeletion (still visible via Get) rather than removing it, while
// HardDelete removes it immediately.
func TestVPHandler_DeletePolicyStoreAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		deletionMode string
		wantState    string // "" means the alias must be gone entirely (Get -> 400)
	}{
		{name: "default (SoftDelete) enters PendingDeletion", deletionMode: "", wantState: "PendingDeletion"},
		{name: "explicit SoftDelete enters PendingDeletion", deletionMode: "SoftDelete", wantState: "PendingDeletion"},
		{name: "HardDelete removes immediately", deletionMode: "HardDelete", wantState: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			storeID := createTestPolicyStore(t, h)
			aliasName := "policy-store-alias/todelete"

			rec := doVPRequest(t, h, "CreatePolicyStoreAlias", map[string]any{
				"aliasName": aliasName, "policyStoreId": storeID,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			delBody := map[string]any{"aliasName": aliasName}
			if tt.deletionMode != "" {
				delBody["deletionMode"] = tt.deletionMode
			}

			rec = doVPRequest(t, h, "DeletePolicyStoreAlias", delBody)
			require.Equal(t, http.StatusOK, rec.Code)

			rec = doVPRequest(t, h, "GetPolicyStoreAlias", map[string]any{"aliasName": aliasName})

			if tt.wantState == "" {
				assert.Equal(t, http.StatusBadRequest, rec.Code)

				return
			}

			require.Equal(t, http.StatusOK, rec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantState, resp["state"])
		})
	}
}

func TestVPHandler_DeletePolicyStoreAlias_NonexistentIsIdempotent(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	rec := doVPRequest(t, h, "DeletePolicyStoreAlias", map[string]any{"aliasName": "policy-store-alias/never-existed"})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestVPHandler_DeletePolicyStore_CascadesAliases proves DeletePolicyStore
// cascade-deletes every alias pointing at the deleted store -- gopherstack's
// documented choice for a case the real API's docs are silent on (see
// InMemoryBackend.DeletePolicyStore). Without this, an alias would survive
// its target's deletion as a dangling row, resolvable (via
// ResolvePolicyStoreAlias) to a policy store ID that no longer exists -- the
// same ghost-row bug class fixed elsewhere in this campaign (e.g. emr
// sessions surviving cluster termination).
func TestVPHandler_DeletePolicyStore_CascadesAliases(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeID := createTestPolicyStore(t, h)
	aliasName := "policy-store-alias/cascaded"

	rec := doVPRequest(t, h, "CreatePolicyStoreAlias", map[string]any{
		"aliasName": aliasName, "policyStoreId": storeID,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doVPRequest(t, h, "DeletePolicyStore", map[string]any{"policyStoreId": storeID})
	require.Equal(t, http.StatusOK, rec.Code)

	// Neither GetPolicyStoreAlias...
	rec = doVPRequest(t, h, "GetPolicyStoreAlias", map[string]any{"aliasName": aliasName})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "alias must not survive its policy store's deletion")

	// ...nor ListPolicyStoreAliases should still report it: no dangling row.
	rec = doVPRequest(t, h, "ListPolicyStoreAliases", map[string]any{})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	items, _ := resp["policyStoreAliases"].([]any)
	assert.Empty(t, items, "dangling alias row survived policy store delete")

	// The alias name must be free to reuse for a brand new policy store.
	newStoreID := createTestPolicyStore(t, h)
	rec = doVPRequest(t, h, "CreatePolicyStoreAlias", map[string]any{
		"aliasName": aliasName, "policyStoreId": newStoreID,
	})
	assert.Equal(t, http.StatusOK, rec.Code, "alias name should be free for reuse after cascade delete")
}

// TestVPHandler_ResolvePolicyStoreID_AcceptsAlias verifies the real API's
// documented behavior that nearly every policyStoreId field accepts either
// the literal ID or an alias name (prefixed "policy-store-alias/") --
// GetPolicyStore/UpdatePolicyStore's doc: "To specify a policy store, use
// its ID or alias name" -- while the two documented exceptions
// (CreatePolicyStoreAlias.policyStoreId and DeletePolicyStore.policyStoreId)
// require the literal ID only ("the alias name cannot be used").
func TestVPHandler_ResolvePolicyStoreID_AcceptsAlias(t *testing.T) {
	t.Parallel()

	tests := []struct {
		extraBody  map[string]any
		name       string
		target     string
		wantCode   int
		checkStore bool
	}{
		{name: "GetPolicyStore accepts alias", target: "GetPolicyStore", wantCode: http.StatusOK, checkStore: true},
		{
			name:       "UpdatePolicyStore accepts alias",
			target:     "UpdatePolicyStore",
			extraBody:  map[string]any{"description": "via alias"},
			wantCode:   http.StatusOK,
			checkStore: true,
		},
		{
			name:      "CreatePolicyStoreAlias rejects alias as its own target",
			target:    "CreatePolicyStoreAlias",
			extraBody: map[string]any{"aliasName": "policy-store-alias/via-alias-target"},
			wantCode:  http.StatusBadRequest,
		},
		{name: "DeletePolicyStore rejects alias", target: "DeletePolicyStore", wantCode: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			storeID := createTestPolicyStore(t, h)
			aliasName := "policy-store-alias/resolvable"

			rec := doVPRequest(t, h, "CreatePolicyStoreAlias", map[string]any{
				"aliasName": aliasName, "policyStoreId": storeID,
			})
			require.Equal(t, http.StatusOK, rec.Code)

			body := map[string]any{"policyStoreId": aliasName}
			maps.Copy(body, tt.extraBody)

			rec = doVPRequest(t, h, tt.target, body)
			assert.Equal(t, tt.wantCode, rec.Code, "body: %s", rec.Body.String())

			if !tt.checkStore {
				return
			}

			var resp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			assert.Equal(t, storeID, resp["policyStoreId"], "response echoes the real ID, not the alias")
		})
	}
}

// TestVPHandler_ResolvePolicyStoreID_PendingDeletionAliasFails verifies the
// real SDK's documented behavior: once an alias enters PendingDeletion, an
// API call that uses it in a policyStoreId field fails with a
// ResourceNotFound exception.
func TestVPHandler_ResolvePolicyStoreID_PendingDeletionAliasFails(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeID := createTestPolicyStore(t, h)
	aliasName := "policy-store-alias/pending"

	rec := doVPRequest(t, h, "CreatePolicyStoreAlias", map[string]any{
		"aliasName": aliasName, "policyStoreId": storeID,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doVPRequest(t, h, "DeletePolicyStoreAlias", map[string]any{"aliasName": aliasName})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doVPRequest(t, h, "GetPolicyStore", map[string]any{"policyStoreId": aliasName})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "a PendingDeletion alias must not resolve")
}

// TestVPHandler_CreatePolicyStore_ClientTokenIdempotency verifies
// CreatePolicyStore's ClientToken idempotency semantics documented on
// CreatePolicyStoreInput.ClientToken: a retry with the same token and the
// same parameters replays the original policy store (no duplicate created);
// a retry with the same token but different parameters fails with
// ConflictException.
func TestVPHandler_CreatePolicyStore_ClientTokenIdempotency(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)

	body := func(description string) map[string]any {
		return map[string]any{
			"validationSettings": map[string]any{"mode": "OFF"},
			"description":        description,
			"clientToken":        "fixed-token",
		}
	}

	rec1 := doVPRequest(t, h, "CreatePolicyStore", body("first"))
	require.Equal(t, http.StatusOK, rec1.Code)
	var resp1 map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &resp1))

	// Same token, same parameters: replays the original policy store.
	rec2 := doVPRequest(t, h, "CreatePolicyStore", body("first"))
	require.Equal(t, http.StatusOK, rec2.Code)
	var resp2 map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))
	assert.Equal(t, resp1["policyStoreId"], resp2["policyStoreId"])
	assert.Equal(t, resp1["createdDate"], resp2["createdDate"])

	// Same token, different parameters: ConflictException.
	rec3 := doVPRequest(t, h, "CreatePolicyStore", body("different"))
	assert.Equal(t, http.StatusBadRequest, rec3.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(rec3.Body.Bytes(), &errResp))
	assert.Equal(t, "ConflictException", errResp["__type"])

	// A different token creates a genuinely new policy store.
	rec4 := doVPRequest(t, h, "CreatePolicyStore", map[string]any{
		"validationSettings": map[string]any{"mode": "OFF"},
		"description":        "first",
		"clientToken":        "another-token",
	})
	require.Equal(t, http.StatusOK, rec4.Code)
	var resp4 map[string]any
	require.NoError(t, json.Unmarshal(rec4.Body.Bytes(), &resp4))
	assert.NotEqual(t, resp1["policyStoreId"], resp4["policyStoreId"])
}
