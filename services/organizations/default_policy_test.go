package organizations_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultFullAWSAccessPolicy_ViaHandler verifies that every organization
// is created with the AWS-managed FullAWSAccess SCP, matching real AWS
// Organizations (gopherstack-hg4i).
func TestDefaultFullAWSAccessPolicy_ViaHandler(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithOrg(t)

	rec := doRequest(t, h, "DescribePolicy", map[string]any{"PolicyId": "p-FullAWSAccess"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp describePolicyResp
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	assert.Equal(t, "p-FullAWSAccess", resp.Policy.PolicySummary.ID)
	assert.Equal(t, "FullAWSAccess", resp.Policy.PolicySummary.Name)
	assert.Equal(t, "SERVICE_CONTROL_POLICY", resp.Policy.PolicySummary.Type)
	assert.True(t, resp.Policy.PolicySummary.AwsManaged, "FullAWSAccess must be AwsManaged")
}

// TestDefaultFullAWSAccessPolicy_ARN_ViaHandler verifies the seeded
// FullAWSAccess SCP carries the AWS-owned ARN authority ("aws", not the
// caller's account ID), matching the PolicyArn pattern in botocore's
// organizations model (api-2.json), which allows only two shapes: the
// customer-owned "::<account>:policy/<org>/<type>/<id>" and the AWS-owned
// "::aws:policy/<type>/<id>" -- FullAWSAccess is AWS-owned (gopherstack-3hov).
func TestDefaultFullAWSAccessPolicy_ARN_ViaHandler(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithOrg(t)

	rec := doRequest(t, h, "DescribePolicy", map[string]any{"PolicyId": "p-FullAWSAccess"})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp describePolicyResp
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	assert.Equal(t,
		"arn:aws:organizations::aws:policy/SERVICE_CONTROL_POLICY/p-FullAWSAccess",
		resp.Policy.PolicySummary.ARN)
}

// TestDefaultFullAWSAccessPolicy_AttachedToRoot_ViaHandler verifies the
// default policy is attached to root, like real AWS.
func TestDefaultFullAWSAccessPolicy_AttachedToRoot_ViaHandler(t *testing.T) {
	t.Parallel()

	h, rootID := newHandlerWithOrg(t)

	rec := doRequest(t, h, "ListPoliciesForTarget", map[string]any{
		"TargetId": rootID,
		"Filter":   "SERVICE_CONTROL_POLICY",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var resp listPoliciesForTargetResp
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	found := false

	for _, p := range resp.Policies {
		if p.ID == "p-FullAWSAccess" {
			found = true
		}
	}

	assert.True(t, found, "FullAWSAccess must be attached to root")
}

// TestDeleteFullAWSAccessPolicy_Rejected_ViaHandler verifies that deleting
// the AWS-managed default policy is refused, and that the policy survives
// the attempt (not merely that an error came back).
func TestDeleteFullAWSAccessPolicy_Rejected_ViaHandler(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithOrg(t)

	rec := doRequest(t, h, "DeletePolicy", map[string]any{"PolicyId": "p-FullAWSAccess"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "AccessDeniedException", errResp["__type"])

	// The survivor check: DescribePolicy must still find it afterward.
	describeRec := doRequest(t, h, "DescribePolicy", map[string]any{"PolicyId": "p-FullAWSAccess"})
	assert.Equal(t, http.StatusOK, describeRec.Code)
}

// TestUpdateFullAWSAccessPolicy_Rejected_ViaHandler verifies that editing
// the AWS-managed default policy is refused, and that its content survives
// the attempt unchanged.
func TestUpdateFullAWSAccessPolicy_Rejected_ViaHandler(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithOrg(t)

	rec := doRequest(t, h, "UpdatePolicy", map[string]any{
		"PolicyId":    "p-FullAWSAccess",
		"Name":        "hijacked",
		"Description": "hijacked",
		"Content":     `{"Version":"2012-10-17","Statement":[]}`,
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
	assert.Equal(t, "AccessDeniedException", errResp["__type"])

	describeRec := doRequest(t, h, "DescribePolicy", map[string]any{"PolicyId": "p-FullAWSAccess"})
	require.Equal(t, http.StatusOK, describeRec.Code)

	var resp describePolicyResp
	require.NoError(t, json.NewDecoder(describeRec.Body).Decode(&resp))
	assert.Equal(t, "FullAWSAccess", resp.Policy.PolicySummary.Name, "name must not be hijacked")
	assert.JSONEq(t,
		`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}`,
		resp.Policy.Content,
		"content must not be hijacked")
}

// TestUserPolicy_AwsManagedFalse_StillDeletable_ViaHandler is the
// too-broad-guard tripwire: a user-created policy must remain AwsManaged
// false and remain deletable, so an over-eager "refuse to delete any
// policy" guard would fail this test even though it would pass the
// FullAWSAccess-specific tests above.
func TestUserPolicy_AwsManagedFalse_StillDeletable_ViaHandler(t *testing.T) {
	t.Parallel()

	h, _ := newHandlerWithOrg(t)

	createRec := doRequest(t, h, "CreatePolicy", map[string]any{
		"Name":        "custom-scp",
		"Description": "",
		"Content":     `{"Version":"2012-10-17","Statement":[]}`,
		"Type":        "SERVICE_CONTROL_POLICY",
	})
	require.Equal(t, http.StatusOK, createRec.Code)

	var createResp createPolicyResp
	require.NoError(t, json.NewDecoder(createRec.Body).Decode(&createResp))
	assert.False(t, createResp.Policy.PolicySummary.AwsManaged, "user-created policy must not be AwsManaged")

	policyID := createResp.Policy.PolicySummary.ID

	deleteRec := doRequest(t, h, "DeletePolicy", map[string]any{"PolicyId": policyID})
	assert.Equal(t, http.StatusOK, deleteRec.Code, "user-created policy must remain deletable")
}

// The following response shapes mirror handler_policies.go's unexported wire
// types (policyObject/policySummaryObject) for test-side decoding.

type policySummaryResp struct {
	ID         string `json:"Id"`
	ARN        string `json:"Arn"`
	Name       string `json:"Name"`
	Type       string `json:"Type"`
	AwsManaged bool   `json:"AwsManaged"`
}

type policyResp struct {
	Content       string            `json:"Content"`
	PolicySummary policySummaryResp `json:"PolicySummary"`
}

type describePolicyResp struct {
	Policy policyResp `json:"Policy"`
}

type createPolicyResp struct {
	Policy policyResp `json:"Policy"`
}

type listPoliciesForTargetResp struct {
	Policies []policySummaryResp `json:"Policies"`
}
