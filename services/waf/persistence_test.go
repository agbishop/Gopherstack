package waf_test

// persistence_test.go covers the Phase 3.3 pkgs/store datalayer conversion:
// a Snapshot->Restore round trip across every store.Table-backed resource
// family (webACLs, rules, rateBasedRules, ipSets, byteMatchSets,
// sizeConstraintSets, sqlInjectionMatchSets, xssMatchSets, geoMatchSets,
// regexPatternSets, regexMatchSets, ruleGroups, loggingConfigs) plus every
// plain map left un-converted (changeTokens, ruleGroupRules,
// permissionPolicies, tags), and a snapshot-version-mismatch test.

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/waf"
)

func TestInMemoryBackend_RestoreInvalidData(t *testing.T) {
	t.Parallel()

	b := waf.NewInMemoryBackend("123456789012", "us-east-1")
	err := b.Restore(t.Context(), []byte("not-valid-json"))
	require.Error(t, err)
}

// TestInMemoryBackend_RestoreVersionMismatch verifies that a snapshot whose
// version doesn't match the current backend (including the pre-Phase-3.3
// format, which decodes with Version == 0) is discarded cleanly rather than
// partially decoded: the backend resets to empty state and Restore returns
// no error.
func TestInMemoryBackend_RestoreVersionMismatch(t *testing.T) {
	t.Parallel()

	b := waf.NewInMemoryBackend("123456789012", "us-east-1")
	_, err := b.CreateWebACL("seed-acl", "seedMetric", waf.WafAction{Type: "ALLOW"}, b.GetChangeToken(), nil)
	require.NoError(t, err)

	// A syntactically valid but version-less/mismatched snapshot.
	err = b.Restore(t.Context(), []byte(`{"version":999,"tables":{}}`))
	require.NoError(t, err)

	assert.Equal(t, 0, waf.WebACLCount(b))
}

func TestHandler_SnapshotRestoreDelegate(t *testing.T) {
	t.Parallel()

	h := waf.NewHandler(waf.NewInMemoryBackend("123456789012", "us-east-1"))
	_, err := h.Backend.CreateWebACL(
		"delegate-acl", "delegateMetric", waf.WafAction{Type: "ALLOW"}, h.Backend.GetChangeToken(), nil,
	)
	require.NoError(t, err)

	snap := h.Snapshot(t.Context())
	require.NotNil(t, snap)

	h2 := waf.NewHandler(waf.NewInMemoryBackend("123456789012", "us-east-1"))
	require.NoError(t, h2.Restore(t.Context(), snap))

	assert.Equal(t, 1, waf.WebACLCount(h2.Backend.(*waf.InMemoryBackend)))
}

// TestInMemoryBackend_SnapshotRestore_FullState exercises a Snapshot->Restore
// round trip across every resource family the Phase 3.3 pkgs/store
// conversion touched, plus every plain map left un-converted (changeTokens,
// ruleGroupRules, permissionPolicies, tags).
func TestInMemoryBackend_SnapshotRestore_FullState(t *testing.T) {
	t.Parallel()

	original := waf.NewInMemoryBackend("123456789012", "us-east-1")

	// staleToken exercises the changeTokens map's INSYNC-persistence path
	// (Snapshot/Restore) in isolation from the resource-creation calls
	// below: InMemoryBackend now validates that a ChangeToken passed to a
	// mutation was issued by GetChangeToken and is still PROVISIONED, so
	// the token used to create/update resources must stay unconsumed --
	// these direct backend calls bypass Handler.dispatch, which is the only
	// thing that ever calls MarkChangeTokenUsed, so a single fresh token
	// remains valid for reuse across every call below.
	staleToken := original.GetChangeToken()
	original.MarkChangeTokenUsed(staleToken)

	token := original.GetChangeToken()
	_ = original.GetChangeToken() // same outstanding token, still PROVISIONED, until consumed below

	acl, err := original.CreateWebACL(
		"acl1", "aclMetric", waf.WafAction{Type: "ALLOW"}, token, map[string]string{"env": "prod"},
	)
	require.NoError(t, err)

	rule, err := original.CreateRule("rule1", "ruleMetric", token, nil)
	require.NoError(t, err)

	err = original.UpdateWebACL(acl.WebACLId, token, nil, []waf.WebACLUpdate{
		{
			Action: "INSERT",
			ActivatedRule: waf.ActivatedRule{
				RuleId:   rule.RuleId,
				Priority: 1,
				Action:   &waf.WafAction{Type: "BLOCK"},
			},
		},
	})
	require.NoError(t, err)

	rateBasedRule, err := original.CreateRateBasedRule(
		"rbr1", "rbrMetric", "IP", 2000, token, map[string]string{"k": "v"},
	)
	require.NoError(t, err)

	ipSet, err := original.CreateIPSet("ipset1", token, nil)
	require.NoError(t, err)
	require.NoError(t, original.UpdateIPSet(ipSet.IPSetId, token, []waf.IPSetUpdate{
		{Action: "INSERT", IPSetDescriptor: waf.IPSetDescriptor{Type: "IPV4", Value: "10.0.0.0/8"}},
	}))

	byteMatchSet, err := original.CreateByteMatchSet("bms1", token)
	require.NoError(t, err)
	require.NoError(t, original.UpdateByteMatchSet(byteMatchSet.ByteMatchSetId, token, []waf.ByteMatchSetUpdate{
		{
			Action: "INSERT",
			ByteMatchTuple: waf.ByteMatchTuple{
				FieldToMatch:         waf.FieldToMatch{Type: "URI"},
				PositionalConstraint: "CONTAINS",
				TargetString:         "admin",
				TextTransformation:   "NONE",
			},
		},
	}))

	sizeConstraintSet, err := original.CreateSizeConstraintSet("scs1", token)
	require.NoError(t, err)
	require.NoError(t, original.UpdateSizeConstraintSet(
		sizeConstraintSet.SizeConstraintSetId, token, []waf.SizeConstraintSetUpdate{
			{
				Action: "INSERT",
				SizeConstraint: waf.SizeConstraint{
					FieldToMatch:       waf.FieldToMatch{Type: "BODY"},
					ComparisonOperator: "GT",
					TextTransformation: "NONE",
					Size:               8192,
				},
			},
		},
	))

	sqlSet, err := original.CreateSqlInjectionMatchSet("sims1", token)
	require.NoError(t, err)
	require.NoError(t, original.UpdateSqlInjectionMatchSet(
		sqlSet.SqlInjectionMatchSetId, token, []waf.SqlInjectionMatchSetUpdate{
			{
				Action: "INSERT",
				SqlInjectionMatchTuple: waf.SqlInjectionMatchTuple{
					FieldToMatch:       waf.FieldToMatch{Type: "QUERY_STRING"},
					TextTransformation: "URL_DECODE",
				},
			},
		},
	))

	xssSet, err := original.CreateXssMatchSet("xms1", token)
	require.NoError(t, err)
	require.NoError(t, original.UpdateXssMatchSet(xssSet.XssMatchSetId, token, []waf.XssMatchSetUpdate{
		{
			Action: "INSERT",
			XssMatchTuple: waf.XssMatchTuple{
				FieldToMatch:       waf.FieldToMatch{Type: "QUERY_STRING"},
				TextTransformation: "URL_DECODE",
			},
		},
	}))

	geoSet, err := original.CreateGeoMatchSet("gms1", token)
	require.NoError(t, err)
	require.NoError(t, original.UpdateGeoMatchSet(geoSet.GeoMatchSetId, token, []waf.GeoMatchSetUpdate{
		{Action: "INSERT", GeoMatchConstraint: waf.GeoMatchConstraint{Type: "Country", Value: "US"}},
	}))

	regexPatternSet, err := original.CreateRegexPatternSet("rps1", token)
	require.NoError(t, err)
	require.NoError(t, original.UpdateRegexPatternSet(
		regexPatternSet.RegexPatternSetId, token, []waf.RegexPatternSetUpdate{
			{Action: "INSERT", RegexPatternString: "^admin.*"},
		},
	))

	regexMatchSet, err := original.CreateRegexMatchSet("rms1", token)
	require.NoError(t, err)
	require.NoError(t, original.UpdateRegexMatchSet(regexMatchSet.RegexMatchSetId, token, []waf.RegexMatchSetUpdate{
		{
			Action: "INSERT",
			RegexMatchTuple: waf.RegexMatchTuple{
				FieldToMatch:       waf.FieldToMatch{Type: "URI"},
				TextTransformation: "NONE",
				RegexPatternSetId:  regexPatternSet.RegexPatternSetId,
			},
		},
	}))

	ruleGroup, err := original.CreateRuleGroup("rg1", "rgMetric", token, nil)
	require.NoError(t, err)
	require.NoError(t, original.UpdateRuleGroup(ruleGroup.RuleGroupId, token, []waf.ActivatedRuleUpdate{
		{
			Action: "INSERT",
			ActivatedRule: waf.ActivatedRule{
				RuleId:   rule.RuleId,
				Priority: 1,
				Type:     "REGULAR",
			},
		},
	}))

	loggingConfig, err := original.PutLoggingConfiguration(waf.LoggingConfiguration{
		ResourceArn:           acl.WebACLArn,
		LogDestinationConfigs: []string{"arn:aws:firehose:us-east-1:123456789012:deliverystream/waf-logs"},
	})
	require.NoError(t, err)

	require.NoError(t, original.PutPermissionPolicy(acl.WebACLArn, `{"Version":"2012-10-17"}`))

	snap := original.Snapshot(t.Context())
	require.NotNil(t, snap)

	fresh := waf.NewInMemoryBackend("999999999999", "eu-west-1")
	require.NoError(t, fresh.Restore(t.Context(), snap))

	assert.Equal(t, "INSYNC", fresh.GetChangeTokenStatus(staleToken))
	assert.Equal(t, "PROVISIONED", fresh.GetChangeTokenStatus(token))

	gotACL, err := fresh.GetWebACL(acl.WebACLId)
	require.NoError(t, err)
	require.Len(t, gotACL.Rules, 1)
	assert.Equal(t, rule.RuleId, gotACL.Rules[0].RuleId)

	gotRule, err := fresh.GetRule(rule.RuleId)
	require.NoError(t, err)
	assert.Equal(t, "rule1", gotRule.Name)

	gotRBR, err := fresh.GetRateBasedRule(rateBasedRule.RuleId)
	require.NoError(t, err)
	assert.Equal(t, int64(2000), gotRBR.RateLimit)

	gotIPSet, err := fresh.GetIPSet(ipSet.IPSetId)
	require.NoError(t, err)
	require.Len(t, gotIPSet.IPSetDescriptors, 1)
	assert.Equal(t, "10.0.0.0/8", gotIPSet.IPSetDescriptors[0].Value)

	gotBMS, err := fresh.GetByteMatchSet(byteMatchSet.ByteMatchSetId)
	require.NoError(t, err)
	require.Len(t, gotBMS.ByteMatchTuples, 1)
	assert.Equal(t, "admin", gotBMS.ByteMatchTuples[0].TargetString)

	gotSCS, err := fresh.GetSizeConstraintSet(sizeConstraintSet.SizeConstraintSetId)
	require.NoError(t, err)
	require.Len(t, gotSCS.SizeConstraints, 1)
	assert.Equal(t, int64(8192), gotSCS.SizeConstraints[0].Size)

	gotSQL, err := fresh.GetSqlInjectionMatchSet(sqlSet.SqlInjectionMatchSetId)
	require.NoError(t, err)
	require.Len(t, gotSQL.SqlInjectionMatchTuples, 1)

	gotXSS, err := fresh.GetXssMatchSet(xssSet.XssMatchSetId)
	require.NoError(t, err)
	require.Len(t, gotXSS.XssMatchTuples, 1)

	gotGeo, err := fresh.GetGeoMatchSet(geoSet.GeoMatchSetId)
	require.NoError(t, err)
	require.Len(t, gotGeo.GeoMatchConstraints, 1)
	assert.Equal(t, "US", gotGeo.GeoMatchConstraints[0].Value)

	gotRPS, err := fresh.GetRegexPatternSet(regexPatternSet.RegexPatternSetId)
	require.NoError(t, err)
	assert.Contains(t, gotRPS.RegexPatternStrings, "^admin.*")

	gotRMS, err := fresh.GetRegexMatchSet(regexMatchSet.RegexMatchSetId)
	require.NoError(t, err)
	require.Len(t, gotRMS.RegexMatchTuples, 1)
	assert.Equal(t, regexPatternSet.RegexPatternSetId, gotRMS.RegexMatchTuples[0].RegexPatternSetId)

	gotRuleGroup, err := fresh.GetRuleGroup(ruleGroup.RuleGroupId)
	require.NoError(t, err)
	assert.Equal(t, "rg1", gotRuleGroup.Name)

	activatedRules, err := fresh.ListActivatedRulesInRuleGroup(ruleGroup.RuleGroupId)
	require.NoError(t, err)
	require.Len(t, activatedRules, 1)
	assert.Equal(t, rule.RuleId, activatedRules[0].RuleId)

	gotLoggingConfig, err := fresh.GetLoggingConfiguration(acl.WebACLArn)
	require.NoError(t, err)
	assert.Equal(t, loggingConfig.LogDestinationConfigs, gotLoggingConfig.LogDestinationConfigs)

	gotPolicy, err := fresh.GetPermissionPolicy(acl.WebACLArn)
	require.NoError(t, err)
	assert.JSONEq(t, `{"Version":"2012-10-17"}`, gotPolicy)

	tags, err := fresh.ListTagsForResource(acl.WebACLArn)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, waf.Tag{Key: "env", Value: "prod"}, tags[0])

	assert.Equal(t, waf.WebACLCount(original), waf.WebACLCount(fresh))
	assert.Equal(t, waf.RuleCount(original), waf.RuleCount(fresh))
	assert.Equal(t, waf.RateBasedRuleCount(original), waf.RateBasedRuleCount(fresh))
	assert.Equal(t, waf.IPSetCount(original), waf.IPSetCount(fresh))
	assert.Equal(t, waf.ByteMatchSetCount(original), waf.ByteMatchSetCount(fresh))
	assert.Equal(t, waf.SizeConstraintSetCount(original), waf.SizeConstraintSetCount(fresh))
	assert.Equal(t, waf.SqlInjectionMatchSetCount(original), waf.SqlInjectionMatchSetCount(fresh))
	assert.Equal(t, waf.XssMatchSetCount(original), waf.XssMatchSetCount(fresh))
	assert.Equal(t, waf.GeoMatchSetCount(original), waf.GeoMatchSetCount(fresh))
	assert.Equal(t, waf.RegexPatternSetCount(original), waf.RegexPatternSetCount(fresh))
	assert.Equal(t, waf.RegexMatchSetCount(original), waf.RegexMatchSetCount(fresh))
	assert.Equal(t, waf.RuleGroupCount(original), waf.RuleGroupCount(fresh))
}

// TestWAF_SnapshotRestore_HandlerLevel exercises Snapshot/Restore through the
// Handler (dispatch-level), covering the resource families introduced first.
func TestWAF_SnapshotRestore_HandlerLevel(t *testing.T) {
	t.Parallel()

	h := newWAFHandler(t)
	wafCreateWebACL(t, h, "snap-acl")
	wafCreateRule(t, h, "snap-rule")
	wafCreateIPSet(t, h, "snap-ipset")

	snap := h.Snapshot(t.Context())
	require.NotEmpty(t, snap)

	h2 := newWAFHandler(t)
	require.NoError(t, h2.Restore(t.Context(), snap))

	assert.Equal(t, 1, waf.WebACLCount(h2.Backend.(*waf.InMemoryBackend)))
	assert.Equal(t, 1, waf.RuleCount(h2.Backend.(*waf.InMemoryBackend)))
	assert.Equal(t, 1, waf.IPSetCount(h2.Backend.(*waf.InMemoryBackend)))
}

// TestSnapshotRestore_ExtendedResourceFamilies exercises Snapshot/Restore
// through the Handler for the resource families not covered by
// TestWAF_SnapshotRestore_HandlerLevel (RateBasedRule, RegexPatternSet,
// RegexMatchSet, RuleGroup, LoggingConfiguration, PermissionPolicy).
func TestSnapshotRestore_ExtendedResourceFamilies(t *testing.T) {
	t.Parallel()

	t.Run("snapshot and restore preserves extended resource state", func(t *testing.T) {
		t.Parallel()

		h := newWAFHandler(t)

		// Create one of each batch-2 resource type
		wafCreateRateBasedRule(t, h, "SnapshotRule")
		wafCreateRegexPatternSet(t, h, "SnapshotPatterns")
		wafCreateRegexMatchSet(t, h, "SnapshotMatch")
		rgID := wafCreateRuleGroup(t, h, "SnapshotGroup")
		rgARN := "arn:aws:waf::123456789012:rulegroup/" + rgID
		aclID := wafCreateWebACL(t, h, "SnapshotACL")
		aclARN := "arn:aws:waf::123456789012:webacl/" + aclID

		rec := wafDo(t, h, "PutLoggingConfiguration", map[string]any{
			"LoggingConfiguration": map[string]any{
				"ResourceArn":           aclARN,
				"LogDestinationConfigs": []string{"arn:aws:firehose:us-east-1:123:deliverystream/logs"},
			},
		})
		require.Equal(t, http.StatusOK, rec.Code)

		rec = wafDo(t, h, "PutPermissionPolicy", map[string]any{
			"ResourceArn": rgARN,
			"Policy":      `{"Version":"2012-10-17"}`,
		})
		require.Equal(t, http.StatusOK, rec.Code)

		// Snapshot
		snap := h.Snapshot(t.Context())

		// Restore into new handler
		h2 := newWAFHandler(t)
		require.NoError(t, h2.Restore(t.Context(), snap))

		b2 := h2.Backend.(*waf.InMemoryBackend)
		assert.Equal(t, 1, waf.RateBasedRuleCount(b2))
		assert.Equal(t, 1, waf.RegexPatternSetCount(b2))
		assert.Equal(t, 1, waf.RegexMatchSetCount(b2))
		assert.Equal(t, 1, waf.RuleGroupCount(b2))

		// Verify logging config restored
		rec = wafDo(t, h2, "GetLoggingConfiguration", map[string]any{"ResourceArn": aclARN})
		require.Equal(t, http.StatusOK, rec.Code)

		// Verify permission policy restored
		rec = wafDo(t, h2, "GetPermissionPolicy", map[string]any{"ResourceArn": rgARN})
		require.Equal(t, http.StatusOK, rec.Code)
	})
}
