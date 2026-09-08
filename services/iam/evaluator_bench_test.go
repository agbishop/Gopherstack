package iam_test

import (
	"fmt"
	"testing"

	"github.com/blackbirdworks/gopherstack/services/iam"
)

// benchmarkPolicyDocs builds n policy documents, each with 5 statements,
// modeling a principal with several attached managed/inline policies.
func benchmarkPolicyDocs(n int) []string {
	docs := make([]string, n)
	for i := range n {
		docs[i] = fmt.Sprintf(`{"Version":"2012-10-17","Statement":[
			{"Effect":"Allow","Action":["s3:GetObject","s3:PutObject","s3:ListBucket"],"Resource":"arn:aws:s3:::bucket-%d/*"},
			{"Effect":"Allow","Action":"dynamodb:*","Resource":"arn:aws:dynamodb:us-east-1:000000000000:table/table-%d"},
			{"Effect":"Deny","Action":"s3:DeleteObject","Resource":"*","Condition":{"StringEquals":{"aws:username":"blocked"}}},
			{"Effect":"Allow","Action":"ec2:Describe*","Resource":"*"},
			{"Effect":"Allow","Action":"lambda:InvokeFunction","Resource":"arn:aws:lambda:us-east-1:000000000000:function:fn-%d"}
		]}`, i, i, i)
	}

	return docs
}

// BenchmarkEvaluatePolicies_RealisticPrincipal models a single
// EvaluatePolicies call for a principal with 5 attached policies of 5
// statements each -- gopherstack-ugfu's claim is that every one of these
// documents is json.Unmarshal'd fresh on every enforced request, and
// enforceIAMPolicy (middleware.go) calls EvaluatePolicies up to 3 times per
// request (identity, boundary, resource-based).
func BenchmarkEvaluatePolicies_RealisticPrincipal(b *testing.B) {
	docs := benchmarkPolicyDocs(5)
	ctx := iam.ConditionContext{Username: "alice"}

	b.ReportAllocs()

	for b.Loop() {
		iam.EvaluatePolicies(docs, "s3:GetObject", "arn:aws:s3:::bucket-2/key", ctx)
	}
}
