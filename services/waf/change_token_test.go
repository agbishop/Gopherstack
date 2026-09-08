package waf_test

// change_token_test.go covers WAF Classic change-token semantics:
//
//   - Issuance: GetChangeToken returns PROVISIONED tokens; a repeated call
//     returns the same outstanding token until it is consumed by a mutation,
//     after which the next call mints a new one.
//   - State machine: GetChangeTokenStatus transitions PROVISIONED -> INSYNC
//     once a token is consumed by a mutation; unknown tokens report INSYNC.
//   - Enforcement: Create/Update/Delete must reject a token that was never
//     returned by GetChangeToken, or that has already been consumed by an
//     earlier mutation (WAFStaleDataException), and a rejection must not
//     itself consume any other token.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

// --- Issuance ---

func TestWAF_ChangeToken_Basic(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	token := wafGetToken(t, h)
	assert.NotEmpty(t, token)
}

func TestWAF_ChangeToken_Unique(t *testing.T) {
	t.Parallel()

	// Real AWS: "If your application submits a GetChangeToken request and
	// then submits a second GetChangeToken request before submitting a
	// create, update, or delete request, the second GetChangeToken request
	// returns the same value as the first" (waf@v1.33.4
	// api_op_GetChangeToken.go:23-27). A repeated call before any mutation
	// must return the SAME outstanding token; only after that token is
	// consumed by a mutation does the next call mint a new one.
	h := newWAFHandler(t)

	t1 := wafGetToken(t, h)
	t2 := wafGetToken(t, h)
	assert.Equal(t, t1, t2, "a repeated GetChangeToken before any mutation must return the same token")

	rec := wafDo(t, h, "CreateRule", map[string]any{
		"ChangeToken": t1,
		"Name":        "unique-token-rule",
		"MetricName":  "uniqueTokenRule",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	t3 := wafGetToken(t, h)
	assert.NotEqual(t, t1, t3, "GetChangeToken must mint a new token once the outstanding one is consumed")
}

func TestWAF_GetChangeTokenStatus_Fresh(t *testing.T) {
	t.Parallel()

	// A freshly-issued token is PROVISIONED until consumed by a mutation.
	h := newWAFHandler(t)
	token := wafGetToken(t, h)

	rec := wafDo(t, h, "GetChangeTokenStatus", map[string]any{
		"ChangeToken": token,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "PROVISIONED", resp["ChangeTokenStatus"])
}

func TestWAF_GetChangeTokenStatus_AfterMutation_INSYNC(t *testing.T) {
	t.Parallel()

	// A token transitions to INSYNC after it is consumed by any mutation.
	h := newWAFHandler(t)
	token := wafGetToken(t, h)

	wafDo(t, h, "CreateWebACL", map[string]any{
		"ChangeToken":   token,
		"Name":          "acl-status-check",
		"MetricName":    "aclStatusCheck",
		"DefaultAction": map[string]any{"Type": "ALLOW"},
	})

	rec := wafDo(t, h, "GetChangeTokenStatus", map[string]any{
		"ChangeToken": token,
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "INSYNC", resp["ChangeTokenStatus"])
}

// --- State machine ---

func TestChangeTokenStatus_Lifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// setup runs, and fully consumes its own change token, BEFORE the
		// outer token is fetched -- needed by DeleteWebACL_transitions_to_INSYNC,
		// which must create its WebACL first so that creation does not steal
		// the outstanding token this test reserves for the delete.
		setup      func(t *testing.T, h *waf.Handler) string
		mutate     func(t *testing.T, h *waf.Handler, token, setupResult string)
		wantBefore string
		wantAfter  string
	}{
		{
			name:       "fresh_token_is_PROVISIONED",
			mutate:     nil,
			wantBefore: "PROVISIONED",
			wantAfter:  "PROVISIONED",
		},
		{
			name: "CreateWebACL_transitions_to_INSYNC",
			mutate: func(t *testing.T, h *waf.Handler, token, _ string) {
				t.Helper()
				rec := wafDo(t, h, "CreateWebACL", map[string]any{
					"ChangeToken":   token,
					"Name":          "acl-lifecycle",
					"MetricName":    "aclLifecycle",
					"DefaultAction": map[string]any{"Type": "ALLOW"},
				})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
			wantBefore: "PROVISIONED",
			wantAfter:  "INSYNC",
		},
		{
			name: "CreateRule_transitions_to_INSYNC",
			mutate: func(t *testing.T, h *waf.Handler, token, _ string) {
				t.Helper()
				rec := wafDo(t, h, "CreateRule", map[string]any{
					"ChangeToken": token,
					"Name":        "rule-lifecycle",
					"MetricName":  "ruleLifecycle",
				})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
			wantBefore: "PROVISIONED",
			wantAfter:  "INSYNC",
		},
		{
			name: "CreateIPSet_transitions_to_INSYNC",
			mutate: func(t *testing.T, h *waf.Handler, token, _ string) {
				t.Helper()
				rec := wafDo(t, h, "CreateIPSet", map[string]any{
					"ChangeToken": token,
					"Name":        "ip-lifecycle",
				})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
			wantBefore: "PROVISIONED",
			wantAfter:  "INSYNC",
		},
		{
			name: "DeleteWebACL_transitions_to_INSYNC",
			setup: func(t *testing.T, h *waf.Handler) string {
				t.Helper()

				return wafCreateWebACL(t, h, "delete-me")
			},
			mutate: func(t *testing.T, h *waf.Handler, token, aclID string) {
				t.Helper()
				rec := wafDo(t, h, "DeleteWebACL", map[string]any{
					"ChangeToken": token,
					"WebACLId":    aclID,
				})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
			wantBefore: "PROVISIONED",
			wantAfter:  "INSYNC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newWAFHandler(t)

			var setupResult string
			if tt.setup != nil {
				setupResult = tt.setup(t, h)
			}

			token := wafGetToken(t, h)

			// Check status before mutation
			rec := wafDo(t, h, "GetChangeTokenStatus", map[string]any{"ChangeToken": token})
			require.Equal(t, http.StatusOK, rec.Code)
			var before map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &before))
			assert.Equal(t, tt.wantBefore, before["ChangeTokenStatus"], "status before mutation")

			if tt.mutate != nil {
				tt.mutate(t, h, token, setupResult)
			}

			// Check status after mutation
			rec = wafDo(t, h, "GetChangeTokenStatus", map[string]any{"ChangeToken": token})
			require.Equal(t, http.StatusOK, rec.Code)
			var after map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &after))
			assert.Equal(t, tt.wantAfter, after["ChangeTokenStatus"], "status after mutation")
		})
	}
}

func TestChangeTokenStatus_UnknownReturnsINSYNC(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	rec := wafDo(t, h, "GetChangeTokenStatus", map[string]any{
		"ChangeToken": "00000000-0000-0000-0000-000000000000",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "INSYNC", resp["ChangeTokenStatus"], "unknown token should return INSYNC per AWS behavior")
}

// --- Enforcement ---

func TestChangeToken_NeverIssuedTokenRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		body   map[string]any
		name   string
		action string
	}{
		{
			name:   "CreateWebACL",
			action: "CreateWebACL",
			body: map[string]any{
				"ChangeToken":   "never-issued",
				"Name":          "acl",
				"MetricName":    "aclMetric",
				"DefaultAction": map[string]any{"Type": "ALLOW"},
			},
		},
		{
			name:   "CreateRule",
			action: "CreateRule",
			body: map[string]any{
				"ChangeToken": "never-issued",
				"Name":        "rule",
				"MetricName":  "ruleMetric",
			},
		},
		{
			name:   "CreateIPSet",
			action: "CreateIPSet",
			body:   map[string]any{"ChangeToken": "never-issued", "Name": "ipset"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWAFHandler(t)
			rec := wafDo(t, h, tc.action, tc.body)
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Equal(t, "WAFStaleDataException", errType(t, rec.Body.Bytes()))
		})
	}
}

func TestChangeToken_AlreadyConsumedTokenRejected(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	token := wafGetToken(t, h)

	// Consume the token with a first, valid mutation.
	rec := wafDo(t, h, "CreateRule", map[string]any{
		"ChangeToken": token,
		"Name":        "first-rule",
		"MetricName":  "firstRuleMetric",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	statusRec := wafDo(t, h, "GetChangeTokenStatus", map[string]any{"ChangeToken": token})
	require.Equal(t, http.StatusOK, statusRec.Code)

	var statusResp map[string]any
	require.NoError(t, json.Unmarshal(statusRec.Body.Bytes(), &statusResp))
	require.Equal(t, "INSYNC", statusResp["ChangeTokenStatus"], "token must be consumed before reuse attempt")

	// Reusing the same, now-INSYNC token for a second mutation must fail.
	rec = wafDo(t, h, "CreateRule", map[string]any{
		"ChangeToken": token,
		"Name":        "second-rule",
		"MetricName":  "secondRuleMetric",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "WAFStaleDataException", errType(t, rec.Body.Bytes()))

	rec = wafDo(t, h, "ListRules", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	assert.Len(t, listResp["Rules"], 1, "the rejected second CreateRule must not have mutated state")
}

func TestChangeToken_FreshTokenAcceptedAfterRejection(t *testing.T) {
	t.Parallel()

	// A stale-token rejection must not consume any other token: the caller
	// can immediately retry with a freshly-issued one.
	h := newWAFHandler(t)

	rec := wafDo(t, h, "CreateRule", map[string]any{
		"ChangeToken": "bogus",
		"Name":        "rule",
		"MetricName":  "ruleMetric",
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)

	token := wafGetToken(t, h)
	rec = wafDo(t, h, "CreateRule", map[string]any{
		"ChangeToken": token,
		"Name":        "rule",
		"MetricName":  "ruleMetric",
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
