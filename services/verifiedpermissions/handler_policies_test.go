package verifiedpermissions_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/verifiedpermissions"
)

func TestVPHandler_PolicyCRUD(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantCode int
	}{
		{
			name:     "full CRUD lifecycle",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)

			// Create policy store
			rec := doVPRequest(
				t,
				h,
				"CreatePolicyStore",
				map[string]any{"description": "test", "validationSettings": map[string]any{"mode": "OFF"}},
			)
			require.Equal(t, http.StatusOK, rec.Code)

			var storeResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &storeResp))
			storeID := storeResp["policyStoreId"].(string)

			// Create policy
			rec = doVPRequest(t, h, "CreatePolicy", map[string]any{
				"policyStoreId": storeID,
				"definition": map[string]any{
					"static": map[string]any{
						"statement": "permit(principal, action, resource);",
					},
				},
			})
			require.Equal(t, tt.wantCode, rec.Code)

			var policyResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &policyResp))
			policyID := policyResp["policyId"].(string)
			assert.NotEmpty(t, policyID)

			// Get policy
			rec = doVPRequest(t, h, "GetPolicy", map[string]any{
				"policyStoreId": storeID,
				"policyId":      policyID,
			})
			assert.Equal(t, http.StatusOK, rec.Code)

			// List policies
			rec = doVPRequest(t, h, "ListPolicies", map[string]any{
				"policyStoreId": storeID,
			})
			assert.Equal(t, http.StatusOK, rec.Code)

			// Update policy
			rec = doVPRequest(t, h, "UpdatePolicy", map[string]any{
				"policyStoreId": storeID,
				"policyId":      policyID,
				"definition": map[string]any{
					"static": map[string]any{
						"statement": "forbid(principal, action, resource);",
					},
				},
			})
			assert.Equal(t, http.StatusOK, rec.Code)

			// Delete policy
			rec = doVPRequest(t, h, "DeletePolicy", map[string]any{
				"policyStoreId": storeID,
				"policyId":      policyID,
			})
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestVPHandler_PolicyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body     map[string]any
		name     string
		action   string
		wantCode int
	}{
		{
			name:     "create policy missing store id",
			action:   "CreatePolicy",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "create policy missing definition",
			action: "CreatePolicy",
			body: map[string]any{
				"policyStoreId": "store-1",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "create policy both static and template linked",
			action: "CreatePolicy",
			body: map[string]any{
				"policyStoreId": "store-1",
				"definition": map[string]any{
					"static":         map[string]any{"statement": "permit(principal, action, resource);"},
					"templateLinked": map[string]any{"policyTemplateId": "tpl-1"},
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "create policy static empty statement",
			action: "CreatePolicy",
			body: map[string]any{
				"policyStoreId": "store-1",
				"definition": map[string]any{
					"static": map[string]any{"statement": ""},
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "create policy template linked empty policy template id",
			action: "CreatePolicy",
			body: map[string]any{
				"policyStoreId": "store-1",
				"definition": map[string]any{
					"templateLinked": map[string]any{"policyTemplateId": ""},
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "get policy missing store id",
			action:   "GetPolicy",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "get policy missing policy id",
			action:   "GetPolicy",
			body:     map[string]any{"policyStoreId": "store-1"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "list policies missing store id",
			action:   "ListPolicies",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "update policy missing store id",
			action:   "UpdatePolicy",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "update policy missing policy id",
			action:   "UpdatePolicy",
			body:     map[string]any{"policyStoreId": "store-1"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "update policy template linked definition rejected",
			action: "UpdatePolicy",
			body: map[string]any{
				"policyStoreId": "store-1",
				"policyId":      "policy-1",
				"definition": map[string]any{
					"templateLinked": map[string]any{"policyTemplateId": "tpl-1"},
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "update policy empty static statement",
			action: "UpdatePolicy",
			body: map[string]any{
				"policyStoreId": "store-1",
				"policyId":      "policy-1",
				"definition": map[string]any{
					"static": map[string]any{"statement": ""},
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "update policy missing definition",
			action: "UpdatePolicy",
			body: map[string]any{
				"policyStoreId": "store-1",
				"policyId":      "policy-1",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "delete policy missing store id",
			action:   "DeletePolicy",
			body:     map[string]any{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "delete policy missing policy id",
			action:   "DeletePolicy",
			body:     map[string]any{"policyStoreId": "store-1"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			rec := doVPRequest(t, h, tt.action, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
		})
	}
}

func TestVPHandler_GetPolicy_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)

	rec := doVPRequest(
		t,
		h,
		"CreatePolicyStore",
		map[string]any{"description": "test", "validationSettings": map[string]any{"mode": "OFF"}},
	)
	var storeResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &storeResp))
	storeID := storeResp["policyStoreId"].(string)

	rec = doVPRequest(t, h, "GetPolicy", map[string]any{
		"policyStoreId": storeID,
		"policyId":      "nonexistent-policy",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestVPHandler_DeletePolicy_IdempotentOnMissingPolicy is the regression
// test for gopherstack-990: DeletePolicy is documented idempotent for a
// missing policy ("the request response returns a successful HTTP 200
// status code"), so deleting a nonexistent policy in an existing store must
// succeed, not 400.
func TestVPHandler_DeletePolicy_IdempotentOnMissingPolicy(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)

	rec := doVPRequest(
		t,
		h,
		"CreatePolicyStore",
		map[string]any{"description": "test", "validationSettings": map[string]any{"mode": "OFF"}},
	)
	var storeResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &storeResp))
	storeID := storeResp["policyStoreId"].(string)

	rec = doVPRequest(t, h, "DeletePolicy", map[string]any{
		"policyStoreId": storeID,
		"policyId":      "nonexistent-policy",
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestVPHandler_DeletePolicy_NotFound proves DeletePolicy still errors when
// the policy STORE doesn't exist -- ResourceNotFoundException remains in
// DeletePolicy's modelled error set, unlike DeletePolicyStore's.
func TestVPHandler_DeletePolicy_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)

	rec := doVPRequest(t, h, "DeletePolicy", map[string]any{
		"policyStoreId": "nonexistent-store",
		"policyId":      "nonexistent-policy",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestVPHandler_CreatePolicy_TemplateLinked(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)

	// Create store
	rec := doVPRequest(
		t,
		h,
		"CreatePolicyStore",
		map[string]any{"description": "test", "validationSettings": map[string]any{"mode": "OFF"}},
	)
	var storeResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &storeResp))
	storeID := storeResp["policyStoreId"].(string)

	// Create a policy template first.
	tmplRec := doVPRequest(t, h, "CreatePolicyTemplate", map[string]any{
		"policyStoreId": storeID,
		"statement":     "permit(principal == ?principal, action, resource);",
		"description":   "test template",
	})
	require.Equal(t, http.StatusOK, tmplRec.Code)
	var tmplResp map[string]any
	require.NoError(t, json.Unmarshal(tmplRec.Body.Bytes(), &tmplResp))
	templateID := tmplResp["policyTemplateId"].(string)

	// Create template-linked policy
	rec = doVPRequest(t, h, "CreatePolicy", map[string]any{
		"policyStoreId": storeID,
		"definition": map[string]any{
			"templateLinked": map[string]any{
				"policyTemplateId": templateID,
				"principal":        map[string]any{"entityType": "User", "entityId": "alice"},
				"resource":         map[string]any{"entityType": "Document", "entityId": "doc1"},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var policyResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &policyResp))
	assert.Equal(t, "TEMPLATE_LINKED", policyResp["policyType"])
}

// TestVPHandler_UpdatePolicy_TemplateLinkedRejected is the regression test
// for gopherstack-990. Real AWS's UpdatePolicyDefinition union has only a
// "static" member (types.UpdatePolicyDefinitionMemberStatic) -- there is no
// templateLinked variant -- and the op doc is explicit: "You can directly
// update only static policies. To change a template-linked policy, you must
// update the template instead, using UpdatePolicyTemplate." Before the fix,
// gopherstack's UpdatePolicy accepted a templateLinked definition and let a
// caller silently rebind a template-linked policy's principal/resource.
func TestVPHandler_UpdatePolicy_TemplateLinkedRejected(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeID := createTestPolicyStore(t, h)

	tmplRec := doVPRequest(t, h, "CreatePolicyTemplate", map[string]any{
		"policyStoreId": storeID,
		"statement":     "permit(principal == ?principal, action, resource);",
	})
	require.Equal(t, http.StatusOK, tmplRec.Code)
	var tmplResp map[string]any
	require.NoError(t, json.Unmarshal(tmplRec.Body.Bytes(), &tmplResp))
	templateID := tmplResp["policyTemplateId"].(string)

	createRec := doVPRequest(t, h, "CreatePolicy", map[string]any{
		"policyStoreId": storeID,
		"definition": map[string]any{
			"templateLinked": map[string]any{
				"policyTemplateId": templateID,
				"principal":        map[string]any{"entityType": "User", "entityId": "alice"},
				"resource":         map[string]any{"entityType": "Document", "entityId": "doc1"},
			},
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)
	var createResp map[string]any
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &createResp))
	policyID := createResp["policyId"].(string)

	t.Run("templateLinked definition rejected outright", func(t *testing.T) {
		t.Parallel()

		rec := doVPRequest(t, h, "UpdatePolicy", map[string]any{
			"policyStoreId": storeID,
			"policyId":      policyID,
			"definition": map[string]any{
				"templateLinked": map[string]any{
					"policyTemplateId": templateID,
					"principal":        map[string]any{"entityType": "User", "entityId": "bob"},
					"resource":         map[string]any{"entityType": "Document", "entityId": "doc1"},
				},
			},
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("static definition against a template-linked policy rejected", func(t *testing.T) {
		t.Parallel()

		rec := doVPRequest(t, h, "UpdatePolicy", map[string]any{
			"policyStoreId": storeID,
			"policyId":      policyID,
			"definition": map[string]any{
				"static": map[string]any{"statement": "permit(principal, action, resource);"},
			},
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("templateLinked definition against a static policy rejected", func(t *testing.T) {
		t.Parallel()

		staticRec := doVPRequest(t, h, "CreatePolicy", map[string]any{
			"policyStoreId": storeID,
			"definition": map[string]any{
				"static": map[string]any{"statement": "permit(principal, action, resource);"},
			},
		})
		require.Equal(t, http.StatusOK, staticRec.Code)
		var staticResp map[string]any
		require.NoError(t, json.Unmarshal(staticRec.Body.Bytes(), &staticResp))
		staticPolicyID := staticResp["policyId"].(string)

		rec := doVPRequest(t, h, "UpdatePolicy", map[string]any{
			"policyStoreId": storeID,
			"policyId":      staticPolicyID,
			"definition": map[string]any{
				"templateLinked": map[string]any{
					"policyTemplateId": templateID,
					"principal":        map[string]any{"entityType": "User", "entityId": "bob"},
				},
			},
		})
		assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
	})

	// Prove the rebinding never actually happened.
	getRec := doVPRequest(t, h, "GetPolicy", map[string]any{"policyStoreId": storeID, "policyId": policyID})
	require.Equal(t, http.StatusOK, getRec.Code)
	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	def := getResp["definition"].(map[string]any)["templateLinked"].(map[string]any)
	assert.Equal(t, "alice", def["principal"].(map[string]any)["entityId"])
}

func TestVPHandler_BatchGetPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(*testing.T, *verifiedpermissions.Handler) map[string]any
		check    func(*testing.T, map[string]any)
		name     string
		wantCode int
	}{
		{
			name: "batch get existing policies",
			setup: func(t *testing.T, h *verifiedpermissions.Handler) map[string]any {
				t.Helper()

				storeID := createTestPolicyStore(t, h)

				rec := doVPRequest(t, h, "CreatePolicy", map[string]any{
					"policyStoreId": storeID,
					"definition": map[string]any{
						"static": map[string]any{"statement": "permit(principal, action, resource);"},
					},
				})
				require.Equal(t, http.StatusOK, rec.Code)

				var pResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pResp))
				policyID := pResp["policyId"].(string)

				return map[string]any{
					"requests": []any{
						map[string]any{"policyStoreId": storeID, "policyId": policyID},
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, resp map[string]any) {
				t.Helper()

				results := resp["results"].([]any)
				assert.Len(t, results, 1)
				assert.Empty(t, resp["errors"])
			},
		},
		{
			name: "batch get with missing policy",
			setup: func(t *testing.T, h *verifiedpermissions.Handler) map[string]any {
				t.Helper()

				storeID := createTestPolicyStore(t, h)

				return map[string]any{
					"requests": []any{
						map[string]any{"policyStoreId": storeID, "policyId": "nonexistent-policy"},
					},
				}
			},
			wantCode: http.StatusOK,
			check: func(t *testing.T, resp map[string]any) {
				t.Helper()

				errors := resp["errors"].([]any)
				assert.Len(t, errors, 1)
			},
		},
		{
			name: "batch get missing required fields",
			setup: func(_ *testing.T, _ *verifiedpermissions.Handler) map[string]any {
				return map[string]any{
					"requests": []any{
						map[string]any{"policyStoreId": "", "policyId": ""},
					},
				}
			},
			wantCode: http.StatusBadRequest,
			check:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestVPHandler(t)
			body := tt.setup(t, h)

			rec := doVPRequest(t, h, "BatchGetPolicy", body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.check != nil {
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				tt.check(t, resp)
			}
		})
	}
}

func TestVPHandler_BatchGetPolicy_EmptyRequests(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)

	rec := doVPRequest(t, h, "BatchGetPolicy", map[string]any{
		"requests": []any{},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp["results"])
	assert.Empty(t, resp["errors"])
}

func TestVPHandler_ListPolicies_FilterByPrincipalIdentifier(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeID := createTestPolicyStore(t, h)

	ptRec := doVPRequest(t, h, "CreatePolicyTemplate", map[string]any{
		"policyStoreId": storeID,
		"statement":     "permit(principal == ?principal, action, resource);",
	})
	require.Equal(t, http.StatusOK, ptRec.Code)

	var ptResp map[string]any
	require.NoError(t, json.Unmarshal(ptRec.Body.Bytes(), &ptResp))
	templateID := ptResp["policyTemplateId"].(string)

	linkedRec := doVPRequest(t, h, "CreatePolicy", map[string]any{
		"policyStoreId": storeID,
		"definition": map[string]any{
			"templateLinked": map[string]any{
				"policyTemplateId": templateID,
				"principal":        map[string]any{"entityType": "MyCorp::User", "entityId": "alice"},
			},
		},
	})
	require.Equal(t, http.StatusOK, linkedRec.Code)

	staticRec := doVPRequest(t, h, "CreatePolicy", map[string]any{
		"policyStoreId": storeID,
		"definition": map[string]any{
			"static": map[string]any{"statement": "permit(principal, action, resource);"},
		},
	})
	require.Equal(t, http.StatusOK, staticRec.Code)

	// Filter for policies scoped to a specific principal identifier.
	filteredRec := doVPRequest(t, h, "ListPolicies", map[string]any{
		"policyStoreId": storeID,
		"filter": map[string]any{
			"principal": map[string]any{
				"identifier": map[string]any{"entityType": "MyCorp::User", "entityId": "alice"},
			},
		},
	})
	require.Equal(t, http.StatusOK, filteredRec.Code, "body: %s", filteredRec.Body.String())

	var filteredResp map[string]any
	require.NoError(t, json.Unmarshal(filteredRec.Body.Bytes(), &filteredResp))
	policies := filteredResp["policies"].([]any)
	require.Len(t, policies, 1)

	// Filter for policies with no principal scope at all (unspecified).
	unspecifiedRec := doVPRequest(t, h, "ListPolicies", map[string]any{
		"policyStoreId": storeID,
		"filter": map[string]any{
			"principal": map[string]any{"unspecified": true},
		},
	})
	require.Equal(t, http.StatusOK, unspecifiedRec.Code, "body: %s", unspecifiedRec.Body.String())

	var unspecifiedResp map[string]any
	require.NoError(t, json.Unmarshal(unspecifiedRec.Body.Bytes(), &unspecifiedResp))
	unspecifiedPolicies := unspecifiedResp["policies"].([]any)
	require.Len(t, unspecifiedPolicies, 1)
}

// TestVPHandler_DeletePolicyStore_DeletionProtection_ConflictException
// verifies the wire error type is "ConflictException" (matching the real
// SDK's exception name), not the previously-hardcoded "ResourceConflictException".

func TestVPHandler_BatchGetPolicy_IncludesDefinition(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)

	// Create a policy store.
	storeRec := doVPRequest(t, h, "CreatePolicyStore", map[string]any{
		"validationSettings": map[string]any{"mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, storeRec.Code)

	var storeResp map[string]any
	require.NoError(t, json.Unmarshal(storeRec.Body.Bytes(), &storeResp))

	policyStoreID, _ := storeResp["policyStoreId"].(string)
	require.NotEmpty(t, policyStoreID)

	// Create a static policy.
	staticRec := doVPRequest(t, h, "CreatePolicy", map[string]any{
		"policyStoreId": policyStoreID,
		"definition": map[string]any{
			"static": map[string]any{
				"statement":   `permit(principal, action, resource);`,
				"description": "parity static policy",
			},
		},
	})
	require.Equal(t, http.StatusOK, staticRec.Code)

	var staticResp map[string]any
	require.NoError(t, json.Unmarshal(staticRec.Body.Bytes(), &staticResp))

	staticPolicyID, _ := staticResp["policyId"].(string)
	require.NotEmpty(t, staticPolicyID)

	// Create a policy template then a template-linked policy.
	tmplRec := doVPRequest(t, h, "CreatePolicyTemplate", map[string]any{
		"policyStoreId": policyStoreID,
		"statement":     `permit(principal == ?principal, action, resource == ?resource);`,
		"description":   "parity template",
	})
	require.Equal(t, http.StatusOK, tmplRec.Code)

	var tmplResp map[string]any
	require.NoError(t, json.Unmarshal(tmplRec.Body.Bytes(), &tmplResp))

	templateID, _ := tmplResp["policyTemplateId"].(string)
	require.NotEmpty(t, templateID)

	tlRec := doVPRequest(t, h, "CreatePolicy", map[string]any{
		"policyStoreId": policyStoreID,
		"definition": map[string]any{
			"templateLinked": map[string]any{
				"policyTemplateId": templateID,
				"principal": map[string]any{
					"entityType": "User",
					"entityId":   "alice",
				},
				"resource": map[string]any{
					"entityType": "Document",
					"entityId":   "doc1",
				},
			},
		},
	})
	require.Equal(t, http.StatusOK, tlRec.Code)

	var tlResp map[string]any
	require.NoError(t, json.Unmarshal(tlRec.Body.Bytes(), &tlResp))

	tlPolicyID, _ := tlResp["policyId"].(string)
	require.NotEmpty(t, tlPolicyID)

	tests := []struct {
		name       string
		policyID   string
		wantDefKey string
		wantField  string
		wantValue  string
	}{
		{
			name:       "static_definition_present",
			policyID:   staticPolicyID,
			wantDefKey: "static",
			wantField:  "description",
			wantValue:  "parity static policy",
		},
		{
			name:       "template_linked_definition_present",
			policyID:   tlPolicyID,
			wantDefKey: "templateLinked",
			wantField:  "policyTemplateId",
			wantValue:  templateID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			batchRec := doVPRequest(t, h, "BatchGetPolicy", map[string]any{
				"requests": []map[string]any{
					{
						"policyStoreId": policyStoreID,
						"policyId":      tt.policyID,
					},
				},
			})
			require.Equal(t, http.StatusOK, batchRec.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(batchRec.Body.Bytes(), &resp))

			results, ok := resp["results"].([]any)
			require.True(t, ok, "results must be an array")
			require.Len(t, results, 1, "expected exactly one result")

			item, ok := results[0].(map[string]any)
			require.True(t, ok)

			def, ok := item["definition"].(map[string]any)
			require.True(t, ok, "definition must be present in batch result item")

			defSubObj, ok := def[tt.wantDefKey].(map[string]any)
			require.True(t, ok, "definition.%s must be present", tt.wantDefKey)

			assert.Equal(t, tt.wantValue, defSubObj[tt.wantField],
				"definition.%s.%s must match", tt.wantDefKey, tt.wantField)
		})
	}
}

func TestVPHandler_ListPolicies_PaginationOpaqueTokens(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)

	storeRec := doVPRequest(t, h, "CreatePolicyStore", map[string]any{
		"validationSettings": map[string]any{"mode": "OFF"},
	})
	require.Equal(t, http.StatusOK, storeRec.Code)
	var storeResp map[string]any
	require.NoError(t, json.Unmarshal(storeRec.Body.Bytes(), &storeResp))
	policyStoreID := storeResp["policyStoreId"].(string)

	// Create 3 policies.
	for i := range 3 {
		rec := doVPRequest(t, h, "CreatePolicy", map[string]any{
			"policyStoreId": policyStoreID,
			"definition": map[string]any{
				"static": map[string]any{
					"statement":   `permit(principal, action, resource) when { true };`,
					"description": fmt.Sprintf("policy-%d", i),
				},
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)
	}

	// Page 1: maxResults=2.
	page1Rec := doVPRequest(t, h, "ListPolicies", map[string]any{
		"policyStoreId": policyStoreID,
		"maxResults":    2,
	})
	require.Equal(t, http.StatusOK, page1Rec.Code)
	var page1 map[string]any
	require.NoError(t, json.Unmarshal(page1Rec.Body.Bytes(), &page1))

	nextToken, _ := page1["nextToken"].(string)
	require.NotEmpty(t, nextToken, "nextToken must be present when more results exist")

	// Token must be opaque base64 (not a raw UUID).
	_, err := base64.StdEncoding.DecodeString(nextToken)
	require.NoError(t, err, "nextToken should be valid base64")
	assert.NotContains(t, nextToken, "-", "nextToken should be opaque (no raw UUID dashes)")

	// Page 2 using the token.
	page2Rec := doVPRequest(t, h, "ListPolicies", map[string]any{
		"policyStoreId": policyStoreID,
		"maxResults":    2,
		"nextToken":     nextToken,
	})
	require.Equal(t, http.StatusOK, page2Rec.Code, "page2 body: %s", page2Rec.Body.String())
	var page2 map[string]any
	require.NoError(t, json.Unmarshal(page2Rec.Body.Bytes(), &page2))

	page2Items, _ := page2["policies"].([]any)
	assert.Len(t, page2Items, 1, "page2 should have the remaining 1 policy")
	_, hasMore := page2["nextToken"]
	assert.False(t, hasMore, "no nextToken on last page")
}

// TestVPHandler_CreatePolicy_StaticScopeEcho locks in a wire-shape gap fix:
// the real SDK's CreatePolicyOutput echoes effect/actions/principal/resource
// parsed from a STATIC policy's Cedar scope clause (e.g. "forbid(principal
// == User::"alice", action == Action::"view", resource);"), which gopherstack
// previously never populated at all for STATIC policies.
func TestVPHandler_CreatePolicy_StaticScopeEcho(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeID := createTestPolicyStore(t, h)

	rec := doVPRequest(t, h, "CreatePolicy", map[string]any{
		"policyStoreId": storeID,
		"definition": map[string]any{
			"static": map[string]any{
				"statement": `forbid(principal == User::"alice", action == Action::"view", resource == Document::"doc1");`,
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Equal(t, "Forbid", resp["effect"])

	principal, _ := resp["principal"].(map[string]any)
	require.NotNil(t, principal, "principal should be echoed for an == scope clause")
	assert.Equal(t, "User", principal["entityType"])
	assert.Equal(t, "alice", principal["entityId"])

	resource, _ := resp["resource"].(map[string]any)
	require.NotNil(t, resource, "resource should be echoed for an == scope clause")
	assert.Equal(t, "Document", resource["entityType"])
	assert.Equal(t, "doc1", resource["entityId"])

	actions, _ := resp["actions"].([]any)
	require.Len(t, actions, 1)
	action, _ := actions[0].(map[string]any)
	assert.Equal(t, "Action", action["actionType"])
	assert.Equal(t, "view", action["actionId"])
}

// TestVPHandler_CreatePolicy_UnconstrainedScopeOmitsEchoFields verifies that
// an unconstrained ("All") scope clause -- permit(principal, action,
// resource) -- omits principal/resource/actions entirely, matching AWS's
// documented "isn't included in the response when [it] isn't present in the
// policy content" behavior, rather than echoing empty/zero-value objects.
func TestVPHandler_CreatePolicy_UnconstrainedScopeOmitsEchoFields(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeID := createTestPolicyStore(t, h)

	rec := doVPRequest(t, h, "CreatePolicy", map[string]any{
		"policyStoreId": storeID,
		"definition": map[string]any{
			"static": map[string]any{
				"statement": `permit(principal, action, resource);`,
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	assert.Equal(t, "Permit", resp["effect"])
	assert.NotContains(t, resp, "principal")
	assert.NotContains(t, resp, "resource")
	assert.NotContains(t, resp, "actions")
}

// TestVPHandler_GetPolicy_TemplateLinkedScopeEcho verifies a TEMPLATE_LINKED
// policy's echoed effect/actions come from the referenced template's Cedar
// statement (with ?principal/?resource substituted), while principal/
// resource come from the policy's own bound entities -- not from re-parsing
// the (slot-containing) template scope clause.
func TestVPHandler_GetPolicy_TemplateLinkedScopeEcho(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeID := createTestPolicyStore(t, h)

	tmplRec := doVPRequest(t, h, "CreatePolicyTemplate", map[string]any{
		"policyStoreId": storeID,
		"statement":     `forbid(principal == ?principal, action == Action::"delete", resource == ?resource);`,
	})
	require.Equal(t, http.StatusOK, tmplRec.Code)
	var tmplResp map[string]any
	require.NoError(t, json.Unmarshal(tmplRec.Body.Bytes(), &tmplResp))
	templateID := tmplResp["policyTemplateId"].(string)

	rec := doVPRequest(t, h, "CreatePolicy", map[string]any{
		"policyStoreId": storeID,
		"definition": map[string]any{
			"templateLinked": map[string]any{
				"policyTemplateId": templateID,
				"principal":        map[string]any{"entityType": "User", "entityId": "bob"},
				"resource":         map[string]any{"entityType": "Document", "entityId": "doc2"},
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var createResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &createResp))
	policyID := createResp["policyId"].(string)

	assert.Equal(t, "Forbid", createResp["effect"])
	actions, _ := createResp["actions"].([]any)
	require.Len(t, actions, 1)
	action, _ := actions[0].(map[string]any)
	assert.Equal(t, "delete", action["actionId"])

	getRec := doVPRequest(t, h, "GetPolicy", map[string]any{
		"policyStoreId": storeID,
		"policyId":      policyID,
	})
	require.Equal(t, http.StatusOK, getRec.Code)
	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))

	principal, _ := getResp["principal"].(map[string]any)
	require.NotNil(t, principal)
	assert.Equal(t, "bob", principal["entityId"])
}

// TestVPHandler_ListPolicies_StaticItemOmitsStatement locks in a wire-shape
// bug fix: the real SDK's StaticPolicyDefinitionItem (used by ListPolicies)
// carries only a description, NOT the full Cedar statement text -- unlike
// GetPolicy's StaticPolicyDefinitionDetail, which does include it.
// gopherstack previously echoed the full statement in ListPolicies items too.
func TestVPHandler_ListPolicies_StaticItemOmitsStatement(t *testing.T) {
	t.Parallel()

	h := newTestVPHandler(t)
	storeID := createTestPolicyStore(t, h)

	createRec := doVPRequest(t, h, "CreatePolicy", map[string]any{
		"policyStoreId": storeID,
		"definition": map[string]any{
			"static": map[string]any{
				"statement":   `permit(principal, action, resource);`,
				"description": "my static policy",
			},
		},
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	listRec := doVPRequest(t, h, "ListPolicies", map[string]any{"policyStoreId": storeID})
	require.Equal(t, http.StatusOK, listRec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listResp))

	items, _ := listResp["policies"].([]any)
	require.Len(t, items, 1)
	item, _ := items[0].(map[string]any)
	def, _ := item["definition"].(map[string]any)
	static, _ := def["static"].(map[string]any)
	require.NotNil(t, static)
	assert.Equal(t, "my static policy", static["description"])
	assert.NotContains(t, static, "statement",
		"ListPolicies' static definition item must not echo the Cedar statement text")

	// GetPolicy's detail shape, by contrast, DOES include the statement.
	policyID, _ := item["policyId"].(string)
	getRec := doVPRequest(t, h, "GetPolicy", map[string]any{"policyStoreId": storeID, "policyId": policyID})
	require.Equal(t, http.StatusOK, getRec.Code)
	var getResp map[string]any
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &getResp))
	getStatic, _ := getResp["definition"].(map[string]any)["static"].(map[string]any)
	assert.Equal(t, `permit(principal, action, resource);`, getStatic["statement"])
}
