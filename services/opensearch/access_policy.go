package opensearch

import (
	"encoding/json"
	"strings"

	"github.com/blackbirdworks/gopherstack/services/iam"
)

// Real OpenSearch domain access policies gate the document data-plane REST
// proxy using the HTTP-verb IAM actions AWS assigns to it.
const (
	actionESHttpGet    = "es:ESHttpGet"
	actionESHttpPut    = "es:ESHttpPut"
	actionESHttpPost   = "es:ESHttpPost"
	actionESHttpDelete = "es:ESHttpDelete"
)

// accessPolicyDenies reports whether a domain's resource-based AccessPolicies
// unambiguously denies a data-plane action: an explicit Deny statement matching
// the action and resource, or a policy with no Allow statement at all (grants
// nothing). It deliberately does not model Principal or evaluate every
// Condition -- that would require a full IAM policy evaluator, out of scope
// here (gopherstack-5hsd) -- so it only catches AWS's two unconditional cases.
func accessPolicyDenies(accessPolicies, action, resourceARN string) bool {
	if accessPolicies == "" {
		return false
	}

	var pd iam.PolicyDocument
	if err := json.Unmarshal([]byte(accessPolicies), &pd); err != nil {
		return false
	}

	hasAllow := false
	for _, stmt := range pd.Statement {
		if strings.EqualFold(stmt.Effect, "Allow") {
			hasAllow = true

			break
		}
	}

	if !hasAllow {
		return true
	}

	result := iam.EvaluatePolicies([]string{accessPolicies}, action, resourceARN, iam.ConditionContext{})

	return result == iam.EvalExplicitDeny
}
