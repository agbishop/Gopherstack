package iam_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/blackbirdworks/gopherstack/services/iam"
)

// TestEvaluatePolicies_MutatedDocumentReEvaluates is gopherstack-ugfu's
// decisive correctness test: parsedPolicyCache is keyed on the policy
// document's own text, so a document that changes (PutUserPolicy,
// CreatePolicyVersion, SetDefaultPolicyVersion, attach/detach all replace
// the stored string) is a different key, not a stale hit on the old one.
// Evaluate once against the original document, mutate it exactly the way a
// policy update would, then evaluate the SAME action/resource/ctx again and
// confirm the decision changed. A cache keyed wrongly (e.g. on
// action+resource alone, or ignoring the document text) would still return
// the first decision here.
func TestEvaluatePolicies_MutatedDocumentReEvaluates(t *testing.T) {
	t.Parallel()

	original := `{"Version":"2012-10-17","Statement":[
		{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}
	]}`

	got := iam.EvaluatePolicies([]string{original}, "s3:GetObject", "arn:aws:s3:::bucket/key", iam.ConditionContext{})
	assert.Equal(t, iam.EvalAllow, got, "original document should allow s3:GetObject")

	mutated := `{"Version":"2012-10-17","Statement":[
		{"Effect":"Deny","Action":"s3:GetObject","Resource":"*"}
	]}`

	got = iam.EvaluatePolicies([]string{mutated}, "s3:GetObject", "arn:aws:s3:::bucket/key", iam.ConditionContext{})
	assert.Equal(t, iam.EvalExplicitDeny, got,
		"mutated document must be re-parsed, not served from a stale cache entry")

	// Re-evaluating the original text again (e.g. a rollback to a prior
	// policy version) must still resolve to its own decision -- the cache
	// entry for the original text was never evicted or corrupted by the
	// mutated document's entry under a different key.
	got = iam.EvaluatePolicies([]string{original}, "s3:GetObject", "arn:aws:s3:::bucket/key", iam.ConditionContext{})
	assert.Equal(
		t,
		iam.EvalAllow,
		got,
		"original document's cache entry must be unaffected by evaluating a mutated document",
	)
}

// TestEvaluatePolicies_ConcurrentEvaluationIsRaceFree exercises the cache
// from many goroutines evaluating both a shared document and distinct
// per-goroutine documents concurrently, for `go test -race`.
func TestEvaluatePolicies_ConcurrentEvaluationIsRaceFree(t *testing.T) {
	t.Parallel()

	shared := `{"Version":"2012-10-17","Statement":[
		{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}
	]}`

	const goroutines = 32

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()

			own := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"ec2:Describe*","Resource":"*"}]}`

			for range 50 {
				iam.EvaluatePolicies([]string{shared, own}, "s3:GetObject", "*", iam.ConditionContext{})
				iam.EvaluatePolicies([]string{own}, "ec2:DescribeInstances", "*", iam.ConditionContext{})
			}
		}()
	}

	wg.Wait()
}
