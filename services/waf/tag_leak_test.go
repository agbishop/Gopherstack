package waf_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/services/waf"
)

// wafAccountID matches the account ID newWAFHandler wires into every test
// backend, needed here to reproduce the ARNs the backend's own
// ipSetARN/rateBasedRuleARN/ruleGroupARN/ruleARN helpers build internally.
const wafAccountID = "123456789012"

// TestWAF_Delete_ClearsTags verifies every one of the twelve WAF resource
// delete paths (WebACL, Rule, RateBasedRule, IPSet, RuleGroup, and the seven
// match-set families: ByteMatchSet, SizeConstraintSet, SqlInjectionMatchSet,
// XssMatchSet, GeoMatchSet, RegexPatternSet, RegexMatchSet) clears its entry
// in the tags map. ListTagsForResource has no existence check against the
// resource itself, so a leaked entry is directly observable: tagging a
// resource, deleting it, then listing tags for the same ARN would otherwise
// still return the stale tags, and the tags map is persisted verbatim in
// Snapshot(), so the leak also grows the snapshot without bound.
func TestWAF_Delete_ClearsTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		create func(t *testing.T, h *waf.Handler) (id, resourceARN string)
		delete func(t *testing.T, h *waf.Handler, token, id string)
		name   string
	}{
		{
			name: "ip set",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateIPSet(t, h, "tag-leak-ipset")

				return id, arn.Build("waf", "", wafAccountID, "ipset/"+id)
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteIPSet", map[string]any{"ChangeToken": token, "IPSetId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
		{
			name: "rate based rule",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateRateBasedRule(t, h, "tag-leak-rbr")

				return id, arn.Build("waf", "", wafAccountID, "ratebasedrule/"+id)
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteRateBasedRule", map[string]any{"ChangeToken": token, "RuleId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
		{
			name: "rule group",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateRuleGroup(t, h, "tag-leak-rg")

				return id, arn.Build("waf", "", wafAccountID, "rulegroup/"+id)
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteRuleGroup", map[string]any{"ChangeToken": token, "RuleGroupId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
		{
			name: "rule",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateRule(t, h, "tag-leak-rule")

				return id, arn.Build("waf", "", wafAccountID, "rule/"+id)
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteRule", map[string]any{"ChangeToken": token, "RuleId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
		{
			name: "byte match set",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateByteMatchSet(t, h, "tag-leak-bms")

				return id, arn.Build("waf", "", wafAccountID, "bytematchset/"+id)
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteByteMatchSet", map[string]any{"ChangeToken": token, "ByteMatchSetId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
		{
			name: "size constraint set",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateSizeConstraintSet(t, h, "tag-leak-scs")

				return id, arn.Build("waf", "", wafAccountID, "sizeconstraintset/"+id)
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteSizeConstraintSet",
					map[string]any{"ChangeToken": token, "SizeConstraintSetId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
		{
			name: "sql injection match set",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateSQLInjectionMatchSet(t, h, "tag-leak-sims")

				return id, arn.Build("waf", "", wafAccountID, "sqlinjectionmatchset/"+id)
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteSqlInjectionMatchSet",
					map[string]any{"ChangeToken": token, "SqlInjectionMatchSetId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
		{
			name: "xss match set",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateXSSMatchSet(t, h, "tag-leak-xms")

				return id, arn.Build("waf", "", wafAccountID, "xssmatchset/"+id)
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteXssMatchSet", map[string]any{"ChangeToken": token, "XssMatchSetId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
		{
			name: "geo match set",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateGeoMatchSet(t, h, "tag-leak-gms")

				return id, arn.Build("waf", "", wafAccountID, "geomatchset/"+id)
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteGeoMatchSet", map[string]any{"ChangeToken": token, "GeoMatchSetId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
		{
			name: "regex pattern set",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateRegexPatternSet(t, h, "tag-leak-rps")

				return id, arn.Build("waf", "", wafAccountID, "regexpatternset/"+id)
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteRegexPatternSet",
					map[string]any{"ChangeToken": token, "RegexPatternSetId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
		{
			name: "regex match set",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateRegexMatchSet(t, h, "tag-leak-rms")

				return id, arn.Build("waf", "", wafAccountID, "regexmatchset/"+id)
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteRegexMatchSet", map[string]any{"ChangeToken": token, "RegexMatchSetId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
		{
			name: "web acl",
			create: func(t *testing.T, h *waf.Handler) (string, string) {
				t.Helper()

				id := wafCreateWebACL(t, h, "tag-leak-acl")

				rec := wafDo(t, h, "GetWebACL", map[string]any{"WebACLId": id})
				require.Equal(t, http.StatusOK, rec.Code)
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				resourceARN, ok := resp["WebACL"].(map[string]any)["WebACLArn"].(string)
				require.True(t, ok)

				return id, resourceARN
			},
			delete: func(t *testing.T, h *waf.Handler, token, id string) {
				t.Helper()

				rec := wafDo(t, h, "DeleteWebACL", map[string]any{"ChangeToken": token, "WebACLId": id})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newWAFHandler(t)
			id, resourceARN := tc.create(t, h)
			_, otherARN := tc.create(t, h)

			for _, a := range []string{resourceARN, otherARN} {
				rec := wafDo(t, h, "TagResource", map[string]any{
					"ResourceARN": a,
					"Tags":        []map[string]any{{"Key": "k", "Value": "v"}},
				})
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			}

			rec := wafDo(t, h, "ListTagsForResource", map[string]any{"ResourceARN": resourceARN})
			require.Equal(t, http.StatusOK, rec.Code)
			var listResp map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
			require.NotEmpty(t, listResp["TagInfoForResource"].(map[string]any)["TagList"])

			token := wafGetToken(t, h)
			tc.delete(t, h, token, id)

			rec = wafDo(t, h, "ListTagsForResource", map[string]any{"ResourceARN": resourceARN})
			require.Equal(t, http.StatusOK, rec.Code)
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
			assert.Empty(t, listResp["TagInfoForResource"].(map[string]any)["TagList"])

			// Deleting one resource must not disturb another's tags.
			rec = wafDo(t, h, "ListTagsForResource", map[string]any{"ResourceARN": otherARN})
			require.Equal(t, http.StatusOK, rec.Code)
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
			assert.NotEmpty(t, listResp["TagInfoForResource"].(map[string]any)["TagList"])
		})
	}
}
