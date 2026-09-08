package cloudformation_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
)

// TestDeleteWAFv2RuleGroup_RemovesRuleGroup confirms DeleteStack's teardown of an
// AWS::WAFv2::RuleGroup resource actually deletes it, instead of leaving it as a ghost row.
// deleteWAFv2RuleGroup previously hardcoded `return nil` and never called the WAFv2 backend at
// all, unlike its WebACL/IPSet siblings in the same file which both call a real Delete method.
func TestDeleteWAFv2RuleGroup_RemovesRuleGroup(t *testing.T) {
	t.Parallel()

	backends := newMoreTypesServiceBackends(t)
	rc := cloudformation.NewResourceCreator(backends)

	props := map[string]any{
		"Name":     "unit-cfn-rule-group",
		"Scope":    "REGIONAL",
		"Capacity": float64(100),
	}

	physID, err := rc.Create(t.Context(), "MyRuleGroup", "AWS::WAFv2::RuleGroup", props, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, physID)

	_, err = backends.WAFv2.Backend.GetRuleGroup(t.Context(), physID)
	require.NoError(t, err, "precondition: rule group must exist after create")

	err = rc.Delete(t.Context(), "AWS::WAFv2::RuleGroup", physID, props)
	require.NoError(t, err)

	_, err = backends.WAFv2.Backend.GetRuleGroup(t.Context(), physID)
	assert.Error(t, err, "rule group must not survive DeleteStack as a ghost row")
}
