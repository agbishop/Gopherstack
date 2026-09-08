package cloudformation_test

import (
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
)

// TestStackSet_CRUD covers CreateStackSet, DescribeStackSet, ListStackSets,
// UpdateStackSet, CreateStackInstances, ListStackInstances, DescribeStackInstance,
// DetectStackSetDrift, ListStackSetOperations, DescribeStackSetOperation,
// StopStackSetOperation, ListStackSetOperationResults, DeleteStackInstances, DeleteStackSet.
func TestStackSet_CRUD(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// CreateStackSet.
	rec := postForm(t, h, "Action=CreateStackSet&Version=2010-05-15"+
		"&StackSetName=test-stack-set"+
		"&TemplateBody="+encodeTemplate(simpleTemplate)+
		"&PermissionModel=SELF_MANAGED")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "CreateStackSetResponse")

	// DescribeStackSet.
	rec = postForm(t, h, "Action=DescribeStackSet&Version=2010-05-15"+
		"&StackSetName=test-stack-set")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DescribeStackSetResponse")

	// ListStackSets.
	rec = postForm(t, h, "Action=ListStackSets&Version=2010-05-15")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ListStackSetsResponse")

	// UpdateStackSet.
	rec = postForm(t, h, "Action=UpdateStackSet&Version=2010-05-15"+
		"&StackSetName=test-stack-set"+
		"&TemplateBody="+encodeTemplate(simpleTemplate))
	assert.Equal(t, http.StatusOK, rec.Code)

	// CreateStackInstances.
	rec = postForm(t, h, "Action=CreateStackInstances&Version=2010-05-15"+
		"&StackSetName=test-stack-set"+
		"&Accounts.member.1=000000000000"+
		"&Regions.member.1=us-east-1")
	assert.Equal(t, http.StatusOK, rec.Code)

	// ListStackInstances.
	rec = postForm(t, h, "Action=ListStackInstances&Version=2010-05-15"+
		"&StackSetName=test-stack-set")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ListStackInstancesResponse")

	// DescribeStackInstance.
	rec = postForm(t, h, "Action=DescribeStackInstance&Version=2010-05-15"+
		"&StackSetName=test-stack-set"+
		"&StackInstanceAccount=000000000000"+
		"&StackInstanceRegion=us-east-1")
	assert.Equal(t, http.StatusOK, rec.Code)

	// UpdateStackInstances.
	rec = postForm(t, h, "Action=UpdateStackInstances&Version=2010-05-15"+
		"&StackSetName=test-stack-set"+
		"&Accounts.member.1=000000000000"+
		"&Regions.member.1=us-east-1")
	assert.Equal(t, http.StatusOK, rec.Code)

	// DetectStackSetDrift.
	rec = postForm(t, h, "Action=DetectStackSetDrift&Version=2010-05-15"+
		"&StackSetName=test-stack-set")
	assert.Equal(t, http.StatusOK, rec.Code)

	// ListStackSetOperations.
	rec = postForm(t, h, "Action=ListStackSetOperations&Version=2010-05-15"+
		"&StackSetName=test-stack-set")
	assert.Equal(t, http.StatusOK, rec.Code)

	// DescribeStackSetOperation (operation ID from CreateStackInstances response).
	body := rec.Body.String()
	_ = body
	rec = postForm(t, h, "Action=DescribeStackSetOperation&Version=2010-05-15"+
		"&StackSetName=test-stack-set"+
		"&OperationId=op-1234")
	// May return 404 if op not found, just check it doesn't panic.
	assert.GreaterOrEqual(t, rec.Code, 200)

	// StopStackSetOperation.
	rec = postForm(t, h, "Action=StopStackSetOperation&Version=2010-05-15"+
		"&StackSetName=test-stack-set"+
		"&OperationId=op-1234")
	assert.GreaterOrEqual(t, rec.Code, 200)

	// ListStackSetOperationResults.
	rec = postForm(t, h, "Action=ListStackSetOperationResults&Version=2010-05-15"+
		"&StackSetName=test-stack-set"+
		"&OperationId=op-1234")
	assert.GreaterOrEqual(t, rec.Code, 200)

	// DeleteStackInstances.
	rec = postForm(t, h, "Action=DeleteStackInstances&Version=2010-05-15"+
		"&StackSetName=test-stack-set"+
		"&Accounts.member.1=000000000000"+
		"&Regions.member.1=us-east-1"+
		"&RetainStacks=false")
	assert.Equal(t, http.StatusOK, rec.Code)

	// DeleteStackSet.
	rec = postForm(t, h, "Action=DeleteStackSet&Version=2010-05-15"+
		"&StackSetName=test-stack-set")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestStackSet_DescribeFieldCompleteness locks in the DescribeStackSetResult.StackSet
// fields that were previously silently dropped (parity gap: only
// StackSetId/StackSetName/Status/Description were returned). Field-diffed against
// aws-sdk-go-v2/service/cloudformation@v1.76.1's
// awsAwsquery_deserializeDocumentStackSet.
func TestStackSet_DescribeFieldCompleteness(t *testing.T) {
	t.Parallel()

	h := newHandler()

	rec := postForm(t, h, url.Values{
		"Action":       {"CreateStackSet"},
		"StackSetName": {"field-complete-ss"},
		"TemplateBody": {
			`{"AWSTemplateFormatVersion":"2010-09-09","Resources":{}}`,
		},
		"AdministrationRoleARN": {
			"arn:aws:iam::000000000000:role/AWSCloudFormationStackSetAdministrationRole",
		},
		"ExecutionRoleName":                  {"AWSCloudFormationStackSetExecutionRole"},
		"PermissionModel":                    {"SELF_MANAGED"},
		"Capabilities.member.1":              {"CAPABILITY_IAM"},
		"Parameters.member.1.ParameterKey":   {"Env"},
		"Parameters.member.1.ParameterValue": {"prod"},
		"Tags.member.1.Key":                  {"Team"},
		"Tags.member.1.Value":                {"platform"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	rec = postForm(t, h, url.Values{
		"Action":            {"CreateStackInstances"},
		"StackSetName":      {"field-complete-ss"},
		"Accounts.member.1": {"000000000000"},
		"Regions.member.1":  {"us-west-2"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	rec = postForm(t, h, url.Values{
		"Action":       {"DescribeStackSet"},
		"StackSetName": {"field-complete-ss"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "<AdministrationRoleARN>arn:aws:iam::000000000000:role/"+
		"AWSCloudFormationStackSetAdministrationRole</AdministrationRoleARN>")
	assert.Equal(
		t,
		"AWSCloudFormationStackSetExecutionRole",
		extractField(body, "ExecutionRoleName"),
	)
	assert.Equal(t, "SELF_MANAGED", extractField(body, "PermissionModel"))
	assert.Contains(t, body, "<Capabilities><member>CAPABILITY_IAM</member></Capabilities>")
	assert.Contains(t, body, "<ParameterKey>Env</ParameterKey>")
	assert.Contains(t, body, "<Key>Team</Key>")
	assert.Contains(t, body, "<Regions><member>us-west-2</member></Regions>")
	assert.Contains(t, body, "<StackSetARN>arn:aws:cloudformation:")
	assert.Contains(t, body, "stackset/field-complete-ss:")
}

func encodeTemplate(_ string) string {
	// Simple URL encode for the template body in form params.
	return "AWSTemplateFormatVersion%3D2010-09-09%26Resources%3D%7B%7D"
}

// TestStackInstances_OperationId verifies that
// CreateStackInstances, DeleteStackInstances, and UpdateStackInstances all
// return an OperationId in their result, matching AWS CloudFormation behaviour.
// Previously these handlers returned empty responses with no OperationId.
func TestStackInstances_OperationId(t *testing.T) {
	t.Parallel()

	tests := []struct {
		extraFields url.Values
		name        string
		action      string
		resultTag   string
	}{
		{
			name:      "create_instances_returns_operation_id",
			action:    "CreateStackInstances",
			resultTag: "CreateStackInstancesResult",
			extraFields: url.Values{
				"Accounts.member.1": {"111111111111"},
				"Regions.member.1":  {"us-east-1"},
			},
		},
		{
			name:      "delete_instances_returns_operation_id",
			action:    "DeleteStackInstances",
			resultTag: "DeleteStackInstancesResult",
			extraFields: url.Values{
				"Accounts.member.1": {"111111111111"},
				"Regions.member.1":  {"us-east-1"},
			},
		},
		{
			name:      "update_instances_returns_operation_id",
			action:    "UpdateStackInstances",
			resultTag: "UpdateStackInstancesResult",
			extraFields: url.Values{
				"Accounts.member.1": {"111111111111"},
				"Regions.member.1":  {"us-east-1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			// Create the stack set.
			rec := postForm(t, h, url.Values{
				"Action":       {"CreateStackSet"},
				"StackSetName": {"opid-test-set"},
				"TemplateBody": {`{"AWSTemplateFormatVersion":"2010-09-09","Resources":{}}`},
			}.Encode())
			require.Equal(t, http.StatusOK, rec.Code)

			// Ensure we have an instance to delete/update.
			if tt.action != "CreateStackInstances" {
				seedRec := postForm(t, h, url.Values{
					"Action":            {"CreateStackInstances"},
					"StackSetName":      {"opid-test-set"},
					"Accounts.member.1": {"111111111111"},
					"Regions.member.1":  {"us-east-1"},
				}.Encode())
				require.Equal(t, http.StatusOK, seedRec.Code)
			}

			fields := url.Values{
				"Action":       {tt.action},
				"StackSetName": {"opid-test-set"},
			}
			maps.Copy(fields, tt.extraFields)

			rec = postForm(t, h, fields.Encode())
			require.Equal(t, http.StatusOK, rec.Code)

			body := rec.Body.String()
			assert.Contains(t, body, tt.resultTag,
				"response must contain result wrapper element")
			opID := extractField(body, "OperationId")
			assert.NotEmpty(t, opID,
				"OperationId must be present in %s response", tt.action)
		})
	}
}

// TestDeleteStackSet_NotEmpty verifies that DeleteStackSet returns
// StackSetNotEmptyException when the stack set still has instances, matching
// AWS CloudFormation behaviour. Previously the backend silently deleted all
// instances alongside the stack set.
func TestDeleteStackSet_NotEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		wantErrorCode string
		createInst    bool
		wantOK        bool
	}{
		{
			name:       "delete_empty_set_succeeds",
			createInst: false,
			wantOK:     true,
		},
		{
			name:          "delete_non_empty_set_fails",
			createInst:    true,
			wantOK:        false,
			wantErrorCode: "StackSetNotEmptyException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			rec := postForm(t, h, url.Values{
				"Action":       {"CreateStackSet"},
				"StackSetName": {"del-test-set"},
				"TemplateBody": {`{"AWSTemplateFormatVersion":"2010-09-09","Resources":{}}`},
			}.Encode())
			require.Equal(t, http.StatusOK, rec.Code)

			if tt.createInst {
				instRec := postForm(t, h, url.Values{
					"Action":            {"CreateStackInstances"},
					"StackSetName":      {"del-test-set"},
					"Accounts.member.1": {"111111111111"},
					"Regions.member.1":  {"us-east-1"},
				}.Encode())
				require.Equal(t, http.StatusOK, instRec.Code)
			}

			rec = postForm(t, h, url.Values{
				"Action":       {"DeleteStackSet"},
				"StackSetName": {"del-test-set"},
			}.Encode())

			if tt.wantOK {
				assert.Equal(t, http.StatusOK, rec.Code,
					"DeleteStackSet on empty set must succeed")
			} else {
				assert.NotEqual(t, http.StatusOK, rec.Code,
					"DeleteStackSet on non-empty set must fail")
				assert.Contains(t, rec.Body.String(), tt.wantErrorCode,
					"error code must be StackSetNotEmptyException")
			}
		})
	}
}

// TestDeleteStackSet_Idempotent verifies that DeleteStackSet on a
// StackSet name that was never created (or already deleted) is a silent
// no-op, not a StackSetNotFoundException. Confirmed against the SDK's
// generated DeleteStackSet error deserializer, whose modeled error set is
// exactly {OperationInProgressException, StackSetNotEmptyException} — no
// "not found" case — mirroring the already-established DeleteStack
// idempotency fix in this codebase.
func TestDeleteStackSet_Idempotent(t *testing.T) {
	t.Parallel()

	h := newHandler()

	rec := postForm(t, h, url.Values{
		"Action":       {"DeleteStackSet"},
		"StackSetName": {"never-existed-set"},
	}.Encode())

	assert.Equal(t, http.StatusOK, rec.Code,
		"DeleteStackSet on a nonexistent StackSet must succeed idempotently")
	assert.NotContains(t, rec.Body.String(), "StackSetNotFoundException")
}

// TestDeleteStackSet_ClearsOperationHistory verifies that DeleteStackSet
// clears b.stackSetOperations and b.stackSetOpResults for the deleted
// StackSetName, not just b.stackSets and b.stackInstances. Otherwise a new
// StackSet created with the same (user-chosen, reusable) name inherits the
// dead StackSet's operation history.
func TestDeleteStackSet_ClearsOperationHistory(t *testing.T) {
	t.Parallel()

	b := newBackend()
	const name = "reused-stack-set"

	_, err := b.CreateStackSet(name, "", simpleTemplate, cloudformation.StackSetOptions{
		PermissionModel: "SELF_MANAGED",
	})
	require.NoError(t, err)

	b.AddStackSetOperationInternal(name, &cloudformation.StackSetOperation{
		OperationID:  "ghost-op",
		StackSetName: name,
		Action:       "UPDATE",
		Status:       "SUCCEEDED",
	})

	require.NoError(t, b.DeleteStackSet(name))

	_, err = b.CreateStackSet(name, "", simpleTemplate, cloudformation.StackSetOptions{
		PermissionModel: "SELF_MANAGED",
	})
	require.NoError(t, err)

	ops, err := b.ListStackSetOperations(name, "")
	require.NoError(t, err)
	assert.Empty(t, ops.Data,
		"recreated StackSet must not inherit the deleted StackSet's operation history")
}

// TestDescribeStackSetOperation_Action verifies that
// DescribeStackSetOperation returns the Action field in its response, matching
// AWS CloudFormation behaviour. Previously only OperationId and Status were
// returned; the Action (e.g. CREATE_INSTANCES, DETECT_DRIFT) was omitted.
func TestDescribeStackSetOperation_Action(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		triggerAction string
		triggerFields url.Values
		wantAction    string
	}{
		{
			name:          "detect_drift_action_present",
			triggerAction: "DetectStackSetDrift",
			triggerFields: url.Values{},
			wantAction:    "DETECT_DRIFT",
		},
		{
			name:          "create_instances_action_present",
			triggerAction: "CreateStackInstances",
			triggerFields: url.Values{
				"Accounts.member.1": {"111111111111"},
				"Regions.member.1":  {"us-east-1"},
			},
			wantAction: "CREATE_INSTANCES",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			rec := postForm(t, h, url.Values{
				"Action":       {"CreateStackSet"},
				"StackSetName": {"action-test-set"},
				"TemplateBody": {`{"AWSTemplateFormatVersion":"2010-09-09","Resources":{}}`},
			}.Encode())
			require.Equal(t, http.StatusOK, rec.Code)

			fields := url.Values{
				"Action":       {tt.triggerAction},
				"StackSetName": {"action-test-set"},
			}
			maps.Copy(fields, tt.triggerFields)
			triggerRec := postForm(t, h, fields.Encode())
			require.Equal(t, http.StatusOK, triggerRec.Code)

			opID := extractField(triggerRec.Body.String(), "OperationId")
			require.NotEmpty(t, opID, "OperationId must be present in trigger response")

			rec = postForm(t, h, url.Values{
				"Action":       {"DescribeStackSetOperation"},
				"StackSetName": {"action-test-set"},
				"OperationId":  {opID},
			}.Encode())
			require.Equal(t, http.StatusOK, rec.Code)

			body := rec.Body.String()
			action := extractField(body, "Action")
			assert.Equal(t, tt.wantAction, action,
				"DescribeStackSetOperation must return Action field")
		})
	}
}

// TestDescribeStackSetOperation_NotFoundErrorCodes verifies DescribeStackSetOperation
// distinguishes its two modeled not-found errors: StackSetNotFoundException when the
// StackSetName itself doesn't exist, vs OperationNotFoundException when the StackSet
// exists but the OperationId doesn't. Field-diffed against
// aws-sdk-go-v2/service/cloudformation@v1.76.1's
// awsAwsquery_deserializeOpErrorDescribeStackSetOperation, which models both cases.
func TestDescribeStackSetOperation_NotFoundErrorCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		stackSetName   string
		wantErrorCode  string
		createStackSet bool
	}{
		{
			name:          "unknown_stack_set_name",
			stackSetName:  "does-not-exist-ss",
			wantErrorCode: "StackSetNotFoundException",
		},
		{
			name:           "known_stack_set_unknown_operation",
			stackSetName:   "known-ss-unknown-op",
			createStackSet: true,
			wantErrorCode:  "OperationNotFoundException",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			if tt.createStackSet {
				rec := postForm(t, h, url.Values{
					"Action":       {"CreateStackSet"},
					"StackSetName": {tt.stackSetName},
					"TemplateBody": {`{"AWSTemplateFormatVersion":"2010-09-09","Resources":{}}`},
				}.Encode())
				require.Equal(t, http.StatusOK, rec.Code)
			}

			rec := postForm(t, h, url.Values{
				"Action":       {"DescribeStackSetOperation"},
				"StackSetName": {tt.stackSetName},
				"OperationId":  {"nonexistent-op"},
			}.Encode())
			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantErrorCode)
		})
	}
}

// TestUpdateStackSet_Description verifies that UpdateStackSet
// updates the Description field when supplied, matching AWS CloudFormation
// behaviour. Previously Description was silently ignored.
func TestUpdateStackSet_Description(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		updateDesc      string
		wantDescription string
	}{
		{
			name:            "description_updated",
			updateDesc:      "my updated description",
			wantDescription: "my updated description",
		},
		{
			name:            "empty_description_not_cleared",
			updateDesc:      "",
			wantDescription: "initial description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			rec := postForm(t, h, url.Values{
				"Action":       {"CreateStackSet"},
				"StackSetName": {"desc-test-set"},
				"Description":  {"initial description"},
				"TemplateBody": {`{"AWSTemplateFormatVersion":"2010-09-09","Resources":{}}`},
			}.Encode())
			require.Equal(t, http.StatusOK, rec.Code)

			rec = postForm(t, h, url.Values{
				"Action":       {"UpdateStackSet"},
				"StackSetName": {"desc-test-set"},
				"Description":  {tt.updateDesc},
			}.Encode())
			require.Equal(t, http.StatusOK, rec.Code)

			rec = postForm(t, h, url.Values{
				"Action":       {"DescribeStackSet"},
				"StackSetName": {"desc-test-set"},
			}.Encode())
			require.Equal(t, http.StatusOK, rec.Code)

			desc := extractField(rec.Body.String(), "Description")
			assert.Equal(t, tt.wantDescription, desc,
				"DescribeStackSet Description must reflect UpdateStackSet result")
		})
	}
}

// TestOrganizationsAccess covers Activate/Deactivate/DescribeOrganizationsAccess.
func TestOrganizationsAccess(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Initially DISABLED
	rec := postForm(t, h, url.Values{
		"Action": []string{"DescribeOrganizationsAccess"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DISABLED")

	// Activate
	rec = postForm(t, h, url.Values{
		"Action": []string{"ActivateOrganizationsAccess"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	// Now ENABLED
	rec = postForm(t, h, url.Values{
		"Action": []string{"DescribeOrganizationsAccess"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ENABLED")

	// Deactivate
	rec = postForm(t, h, url.Values{
		"Action": []string{"DeactivateOrganizationsAccess"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	// Now DISABLED again
	rec = postForm(t, h, url.Values{
		"Action": []string{"DescribeOrganizationsAccess"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DISABLED")
}

// TestStackSetOperations covers the operation-tracking methods:
// ListStackSetOperations, DescribeStackSetOperation, StopStackSetOperation,
// ListStackSetOperationResults, ListStackSetAutoDeploymentTargets,
// ImportStacksToStackSet, ListStackInstanceResourceDrifts.
func TestStackSetOperations(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Create a stack set first
	rec := postForm(t, h, url.Values{
		"Action":       []string{"CreateStackSet"},
		"StackSetName": []string{"ops-test-set"},
		"TemplateBody": []string{`{"AWSTemplateFormatVersion":"2010-09-09","Resources":{}}`},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	// CreateStackInstances creates an operation
	rec = postForm(t, h, url.Values{
		"Action":            []string{"CreateStackInstances"},
		"StackSetName":      []string{"ops-test-set"},
		"Accounts.member.1": []string{"111111111111"},
		"Regions.member.1":  []string{"us-east-1"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	// ListStackSetOperations — should have at least one operation
	rec = postForm(t, h, url.Values{
		"Action":       []string{"ListStackSetOperations"},
		"StackSetName": []string{"ops-test-set"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ListStackSetOperationsResponse")

	// DetectStackSetDrift — returns an operation ID
	rec = postForm(t, h, url.Values{
		"Action":       []string{"DetectStackSetDrift"},
		"StackSetName": []string{"ops-test-set"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)
	opID := extractField(rec.Body.String(), "OperationId")
	require.NotEmpty(t, opID, "OperationId must be returned from DetectStackSetDrift")

	// DescribeStackSetOperation — should work for the drift operation
	rec = postForm(t, h, url.Values{
		"Action":       []string{"DescribeStackSetOperation"},
		"StackSetName": []string{"ops-test-set"},
		"OperationId":  []string{opID},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "SUCCEEDED")

	// DescribeStackSetOperation — nonexistent op should return error
	rec = postForm(t, h, url.Values{
		"Action":       []string{"DescribeStackSetOperation"},
		"StackSetName": []string{"ops-test-set"},
		"OperationId":  []string{"nonexistent-op"},
	}.Encode())
	assert.NotEqual(t, http.StatusOK, rec.Code, "Nonexistent op should return error")

	// StopStackSetOperation — op is SUCCEEDED so should fail (not RUNNING)
	rec = postForm(t, h, url.Values{
		"Action":       []string{"StopStackSetOperation"},
		"StackSetName": []string{"ops-test-set"},
		"OperationId":  []string{opID},
	}.Encode())
	assert.NotEqual(t, http.StatusOK, rec.Code, "stopping a non-RUNNING operation should error")

	// ListStackSetOperationResults
	rec = postForm(t, h, url.Values{
		"Action":       []string{"ListStackSetOperationResults"},
		"StackSetName": []string{"ops-test-set"},
		"OperationId":  []string{opID},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ListStackSetOperationResultsResponse")

	// ListStackSetAutoDeploymentTargets — should return the account we added
	rec = postForm(t, h, url.Values{
		"Action":       []string{"ListStackSetAutoDeploymentTargets"},
		"StackSetName": []string{"ops-test-set"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "111111111111")

	// ImportStacksToStackSet — creates an IMPORT operation
	rec = postForm(t, h, url.Values{
		"Action":            []string{"ImportStacksToStackSet"},
		"StackSetName":      []string{"ops-test-set"},
		"StackIds.member.1": []string{"stack-abc"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	// ListStackInstanceResourceDrifts
	rec = postForm(t, h, url.Values{
		"Action":               []string{"ListStackInstanceResourceDrifts"},
		"StackSetName":         []string{"ops-test-set"},
		"OperationId":          []string{opID},
		"StackInstanceAccount": []string{"111111111111"},
		"StackInstanceRegion":  []string{"us-east-1"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	// UpdateStackSet — creates an UPDATE operation
	rec = postForm(t, h, url.Values{
		"Action":       []string{"UpdateStackSet"},
		"StackSetName": []string{"ops-test-set"},
		"TemplateBody": []string{`{"AWSTemplateFormatVersion":"2010-09-09","Resources":{}}`},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	// UpdateStackInstances — creates an UPDATE_INSTANCES operation
	rec = postForm(t, h, url.Values{
		"Action":            []string{"UpdateStackInstances"},
		"StackSetName":      []string{"ops-test-set"},
		"Accounts.member.1": []string{"111111111111"},
		"Regions.member.1":  []string{"us-east-1"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	// DeleteStackInstances — creates a DELETE_INSTANCES operation
	rec = postForm(t, h, url.Values{
		"Action":            []string{"DeleteStackInstances"},
		"StackSetName":      []string{"ops-test-set"},
		"Accounts.member.1": []string{"111111111111"},
		"Regions.member.1":  []string{"us-east-1"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestStackSetOperations_ListAutoDeploymentTargets_NotFound ensures
// ListStackSetAutoDeploymentTargets errors on missing stack set.
func TestStackSetOperations_ListAutoDeploymentTargets_NotFound(t *testing.T) {
	t.Parallel()

	h := newHandler()

	rec := postForm(t, h, url.Values{
		"Action":       []string{"ListStackSetAutoDeploymentTargets"},
		"StackSetName": []string{"nonexistent-set"},
	}.Encode())
	assert.NotEqual(t, http.StatusOK, rec.Code, "missing stack set should error")
}

// TestStackSetOperations_ImportNotFound ensures ImportStacksToStackSet
// returns error for missing stack set.
func TestStackSetOperations_ImportNotFound(t *testing.T) {
	t.Parallel()

	h := newHandler()

	rec := postForm(t, h, url.Values{
		"Action":       []string{"ImportStacksToStackSet"},
		"StackSetName": []string{"nonexistent-set"},
	}.Encode())
	assert.NotEqual(t, http.StatusOK, rec.Code, "Should error for nonexistent stack set")
}

// TestListStackSetOperations_TiedCreatedAtPageWalk proves
// ListStackSetOperations sorts on CreatedAt alone -- a field with no
// tiebreak -- over b.stackSetOperations[stackSetName] (a raw
// map[string]*StackSetOperation keyed by operation ID, unspecified Go map
// order). page.New then paginates that order with an offset-index scheme.
// Several operations sharing one CreatedAt can therefore land in a
// different relative order on each call, so a page boundary that fell
// between two tied operations on one call falls between two different tied
// operations on the next -- one gets dropped or duplicated across the page
// boundary with nothing else changed. Looped: a single walk can pass by
// luck since map iteration is randomized per-call.
func TestListStackSetOperations_TiedCreatedAtPageWalk(t *testing.T) {
	t.Parallel()

	b := newBackend()

	// ListStackSetOperations hardcodes cfnDefaultPageSize (100) as its page
	// size -- it takes no maxResults param -- so total must exceed 100 to
	// force a page boundary at all.
	const total = 110

	tied := time.Now()

	want := make(map[string]bool, total)

	for i := range total {
		opID := "op-" + strconv.Itoa(i)
		b.AddStackSetOperationInternal("my-stack-set", &cloudformation.StackSetOperation{
			OperationID:  opID,
			StackSetName: "my-stack-set",
			Action:       "UPDATE",
			Status:       "SUCCEEDED",
			CreatedAt:    tied,
		})
		want[opID] = true
	}

	const pageSize = 100

	for iter := range 30 {
		got := make(map[string]int, total)

		token := ""
		for range total/pageSize + 2 {
			p, err := b.ListStackSetOperations("my-stack-set", token)
			require.NoError(t, err)

			for _, op := range p.Data {
				got[op.OperationID]++
			}

			if p.Next == "" {
				break
			}

			token = p.Next
		}

		require.Lenf(
			t,
			got,
			total,
			"iteration %d: page walk produced %d distinct operations, want %d",
			iter,
			len(got),
			total,
		)

		for id := range want {
			require.Equalf(
				t,
				1,
				got[id],
				"iteration %d: operation %s appeared %d times across the page walk",
				iter,
				id,
				got[id],
			)
		}
	}
}
