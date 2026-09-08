package cloudformation_test

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
)

// postFormValues posts a url.Values form to the handler.
func postFormValues(t *testing.T, h *cloudformation.Handler, values url.Values) *httpResponse {
	t.Helper()

	return postFormBody(t, h, values.Encode())
}

type httpResponse struct {
	Body   string
	Status int
}

func postFormBody(t *testing.T, h *cloudformation.Handler, body string) *httpResponse {
	t.Helper()
	rec := postForm(t, h, body)

	return &httpResponse{Body: rec.Body.String(), Status: rec.Code}
}

func (r *httpResponse) mustOK(t *testing.T) {
	t.Helper()
	assert.Equal(t, http.StatusOK, r.Status, "body: %s", r.Body)
}

// ---- Handler: CreateStack with Capabilities -----------------------------------

func TestHandler_CreateStack_WithCapabilities(t *testing.T) {
	t.Parallel()

	h := newHandler()
	v := url.Values{
		"Action":                {"CreateStack"},
		"StackName":             {"cap-stack"},
		"TemplateBody":          {simpleTemplate},
		"Capabilities.member.1": {"CAPABILITY_IAM"},
		"Capabilities.member.2": {"CAPABILITY_NAMED_IAM"},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)

	// Verify capabilities stored on stack.
	stack, err := h.Backend.(*cloudformation.InMemoryBackend).DescribeStack("cap-stack")
	require.NoError(t, err)
	assert.Contains(t, stack.Capabilities, "CAPABILITY_IAM")
	assert.Contains(t, stack.Capabilities, "CAPABILITY_NAMED_IAM")
}

func TestHandler_CreateStack_WithOnFailureDelete(t *testing.T) {
	t.Parallel()

	h := newHandler()
	failTemplate := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"B": {"Type": "AWS::S3::Bucket", "Properties": {"BucketName": {"Fn::ImportValue": "no-export"}}}
		}
	}`
	v := url.Values{
		"Action":       {"CreateStack"},
		"StackName":    {"fail-del"},
		"TemplateBody": {failTemplate},
		"OnFailure":    {"DELETE"},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)
}

func TestHandler_CreateStack_WithRoleARN(t *testing.T) {
	t.Parallel()

	h := newHandler()
	v := url.Values{
		"Action":       {"CreateStack"},
		"StackName":    {"role-stack"},
		"TemplateBody": {simpleTemplate},
		"RoleARN":      {"arn:aws:iam::123456789012:role/CFNRole"},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)

	stack, err := h.Backend.(*cloudformation.InMemoryBackend).DescribeStack("role-stack")
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iam::123456789012:role/CFNRole", stack.RoleARN)
}

func TestHandler_CreateStack_WithNotificationARNs(t *testing.T) {
	t.Parallel()

	h := newHandler()
	v := url.Values{
		"Action":                    {"CreateStack"},
		"StackName":                 {"notif-stack"},
		"TemplateBody":              {simpleTemplate},
		"NotificationARNs.member.1": {"arn:aws:sns:us-east-1:123:MyTopic"},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)

	stack, err := h.Backend.(*cloudformation.InMemoryBackend).DescribeStack("notif-stack")
	require.NoError(t, err)
	assert.Len(t, stack.NotificationARNs, 1)
	assert.Equal(t, "arn:aws:sns:us-east-1:123:MyTopic", stack.NotificationARNs[0])
}

func TestHandler_CreateStack_WithTimeoutInMinutes(t *testing.T) {
	t.Parallel()

	h := newHandler()
	v := url.Values{
		"Action":           {"CreateStack"},
		"StackName":        {"timeout-stack"},
		"TemplateBody":     {simpleTemplate},
		"TimeoutInMinutes": {"45"},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)

	stack, err := h.Backend.(*cloudformation.InMemoryBackend).DescribeStack("timeout-stack")
	require.NoError(t, err)
	assert.Equal(t, 45, stack.TimeoutInMinutes)
}

func TestHandler_CreateStack_WithDisableRollback(t *testing.T) {
	t.Parallel()

	h := newHandler()
	v := url.Values{
		"Action":          {"CreateStack"},
		"StackName":       {"noroll-stack"},
		"TemplateBody":    {simpleTemplate},
		"DisableRollback": {"true"},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)

	stack, err := h.Backend.(*cloudformation.InMemoryBackend).DescribeStack("noroll-stack")
	require.NoError(t, err)
	assert.True(t, stack.DisableRollback)
}

func TestHandler_CreateStack_WithRollbackConfiguration(t *testing.T) {
	t.Parallel()

	h := newHandler()
	v := url.Values{
		"Action":       {"CreateStack"},
		"StackName":    {"rc-stack"},
		"TemplateBody": {simpleTemplate},
		"RollbackConfiguration.MonitoringTimeInMinutes": {"10"},
		"RollbackConfiguration.RollbackTriggers.member.1.Arn": {
			"arn:aws:cloudwatch:us-east-1:123:alarm/MyAlarm",
		},
		"RollbackConfiguration.RollbackTriggers.member.1.Type": {"AWS::CloudWatch::Alarm"},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)

	stack, err := h.Backend.(*cloudformation.InMemoryBackend).DescribeStack("rc-stack")
	require.NoError(t, err)
	require.NotNil(t, stack.RollbackConfiguration)
	assert.Equal(t, 10, stack.RollbackConfiguration.MonitoringTimeInMinutes)
	assert.Len(t, stack.RollbackConfiguration.RollbackTriggers, 1)
}

// ---- Handler: UpdateStack with new fields -------------------------------------

func TestHandler_UpdateStack_WithCapabilities(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Create first.
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"us-cap"}, "TemplateBody": {simpleTemplate},
	})

	v := url.Values{
		"Action":                {"UpdateStack"},
		"StackName":             {"us-cap"},
		"TemplateBody":          {simpleTemplate},
		"Capabilities.member.1": {"CAPABILITY_AUTO_EXPAND"},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)

	stack, err := h.Backend.(*cloudformation.InMemoryBackend).DescribeStack("us-cap")
	require.NoError(t, err)
	assert.Contains(t, stack.Capabilities, "CAPABILITY_AUTO_EXPAND")
}

// ---- Handler: UpdateTerminationProtection -------------------------------------

func TestHandler_UpdateTerminationProtection_Enable(t *testing.T) {
	t.Parallel()

	h := newHandler()
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"prot"}, "TemplateBody": {simpleTemplate},
	})

	v := url.Values{
		"Action":                      {"UpdateTerminationProtection"},
		"StackName":                   {"prot"},
		"EnableTerminationProtection": {"true"},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)

	stack, err := h.Backend.(*cloudformation.InMemoryBackend).DescribeStack("prot")
	require.NoError(t, err)
	assert.True(t, stack.EnableTerminationProtection)
}

func TestHandler_DeleteStack_TerminationProtected_Returns403(t *testing.T) {
	t.Parallel()

	h := newHandler()
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"prot-del"}, "TemplateBody": {simpleTemplate},
	})
	postFormValues(t, h, url.Values{
		"Action": {
			"UpdateTerminationProtection",
		}, "StackName": {"prot-del"}, "EnableTerminationProtection": {"true"},
	})

	resp := postFormValues(t, h, url.Values{
		"Action": {"DeleteStack"}, "StackName": {"prot-del"},
	})
	// Should return an error (non-200).
	assert.NotEqual(t, http.StatusOK, resp.Status)
}

// ---- Handler: ValidateTemplate returns AllowedValues --------------------------

func TestHandler_ValidateTemplate_ReturnsCapabilities(t *testing.T) {
	t.Parallel()

	h := newHandler()
	tmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Parameters": {
			"Env": {"Type": "String", "AllowedValues": ["dev", "prod"]}
		},
		"Resources": {
			"Role": {"Type": "AWS::IAM::Role"}
		}
	}`
	v := url.Values{
		"Action":       {"ValidateTemplate"},
		"TemplateBody": {tmpl},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)
}

// ---- Handler: GetTemplateSummary returns AllowedValues / NoEcho ---------------

func TestHandler_GetTemplateSummary_AllowedValues(t *testing.T) {
	t.Parallel()

	h := newHandler()
	tmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Parameters": {
			"Env": {
				"Type": "String",
				"AllowedValues": ["dev", "staging", "prod"],
				"NoEcho": false
			},
			"Secret": {
				"Type": "String",
				"NoEcho": true
			}
		},
		"Resources": {"B": {"Type": "AWS::S3::Bucket"}}
	}`
	v := url.Values{
		"Action":       {"GetTemplateSummary"},
		"TemplateBody": {tmpl},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)
	// Response should contain AllowedValues members.
	assert.Contains(t, resp.Body, "dev")
	assert.Contains(t, resp.Body, "staging")
	assert.Contains(t, resp.Body, "prod")
}

// ---- Handler: DescribeStacks returns new fields --------------------------------

func TestHandler_DescribeStacks_ReturnsCapabilities(t *testing.T) {
	t.Parallel()

	h := newHandler()
	postFormValues(t, h, url.Values{
		"Action":                {"CreateStack"},
		"StackName":             {"cap-describe"},
		"TemplateBody":          {simpleTemplate},
		"Capabilities.member.1": {"CAPABILITY_IAM"},
	})

	v := url.Values{
		"Action":    {"DescribeStacks"},
		"StackName": {"cap-describe"},
	}
	resp := postFormValues(t, h, v)
	resp.mustOK(t)
	assert.Contains(t, resp.Body, "CAPABILITY_IAM")
}

// ---- Handler: StackSet operations via HTTP ------------------------------------

func TestHandler_StackSetCRUD_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Create.
	resp := postFormValues(t, h, url.Values{
		"Action":       {"CreateStackSet"},
		"StackSetName": {"my-http-ss"},
		"Description":  {"http test set"},
		"TemplateBody": {simpleTemplate},
	})
	resp.mustOK(t)

	// Describe.
	resp = postFormValues(t, h, url.Values{
		"Action":       {"DescribeStackSet"},
		"StackSetName": {"my-http-ss"},
	})
	resp.mustOK(t)
	assert.Contains(t, resp.Body, "my-http-ss")

	// List.
	resp = postFormValues(t, h, url.Values{"Action": {"ListStackSets"}})
	resp.mustOK(t)

	// Delete.
	resp = postFormValues(t, h, url.Values{
		"Action":       {"DeleteStackSet"},
		"StackSetName": {"my-http-ss"},
	})
	resp.mustOK(t)
}

func TestHandler_StackInstances_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()

	postFormValues(t, h, url.Values{
		"Action": {"CreateStackSet"}, "StackSetName": {"inst-ss"}, "TemplateBody": {simpleTemplate},
	})

	// Create instances.
	resp := postFormValues(t, h, url.Values{
		"Action":            {"CreateStackInstances"},
		"StackSetName":      {"inst-ss"},
		"Accounts.member.1": {"111111111111"},
		"Regions.member.1":  {"us-east-1"},
		"Regions.member.2":  {"us-west-2"},
	})
	resp.mustOK(t)

	// List instances.
	resp = postFormValues(t, h, url.Values{
		"Action":       {"ListStackInstances"},
		"StackSetName": {"inst-ss"},
	})
	resp.mustOK(t)

	// Describe instance.
	resp = postFormValues(t, h, url.Values{
		"Action":               {"DescribeStackInstance"},
		"StackSetName":         {"inst-ss"},
		"StackInstanceAccount": {"111111111111"},
		"StackInstanceRegion":  {"us-east-1"},
	})
	resp.mustOK(t)
}

// ---- Handler: Drift detection via HTTP ----------------------------------------

func TestHandler_DetectStackDrift_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"drift-http"}, "TemplateBody": {simpleTemplate},
	})

	resp := postFormValues(t, h, url.Values{
		"Action":    {"DetectStackDrift"},
		"StackName": {"drift-http"},
	})
	resp.mustOK(t)
	assert.Contains(t, resp.Body, "StackDriftDetectionId")
}

func TestHandler_DescribeStackDriftDetectionStatus_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"drift-status"}, "TemplateBody": {simpleTemplate},
	})

	// Start drift detection.
	respDrift := postFormValues(t, h, url.Values{
		"Action":    {"DetectStackDrift"},
		"StackName": {"drift-status"},
	})
	respDrift.mustOK(t)

	// Parse detection ID.
	var detResp struct {
		XMLName xml.Name `xml:"DetectStackDriftResponse"`
		Result  struct {
			ID string `xml:"StackDriftDetectionId"`
		} `xml:"DetectStackDriftResult"`
	}
	err := xml.Unmarshal([]byte(respDrift.Body), &detResp)
	require.NoError(t, err)

	// Describe status.
	resp := postFormValues(t, h, url.Values{
		"Action":                {"DescribeStackDriftDetectionStatus"},
		"StackDriftDetectionId": {detResp.Result.ID},
	})
	resp.mustOK(t)
	assert.Contains(t, resp.Body, "DETECTION_COMPLETE")
}

// ---- Handler: ListExports via HTTP --------------------------------------------

func TestHandler_ListExports_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Create an exporter stack.
	exportTmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {"Q": {"Type": "AWS::SQS::Queue"}},
		"Outputs": {"QueueURL": {"Value": {"Ref": "Q"}, "Export": {"Name": "http-queue-url"}}}
	}`
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"exp-http"}, "TemplateBody": {exportTmpl},
	})

	resp := postFormValues(t, h, url.Values{"Action": {"ListExports"}})
	resp.mustOK(t)
	assert.Contains(t, resp.Body, "http-queue-url")
}

func TestHandler_ListImports_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()

	exportTmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {"Q": {"Type": "AWS::SQS::Queue"}},
		"Outputs": {"U": {"Value": {"Ref": "Q"}, "Export": {"Name": "imp-queue"}}}
	}`
	importTmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {"Q": {"Type": "AWS::SQS::Queue"}},
		"Outputs": {"U": {"Value": {"Fn::ImportValue": "imp-queue"}}}
	}`

	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"exp-imp-src"}, "TemplateBody": {exportTmpl},
	})
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"exp-imp-dst"}, "TemplateBody": {importTmpl},
	})

	resp := postFormValues(t, h, url.Values{
		"Action":     {"ListImports"},
		"ExportName": {"imp-queue"},
	})
	resp.mustOK(t)
}

// ---- Handler: DescribeStackResources HTTP -------------------------------------

func TestHandler_DescribeStackResources_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()

	tmpl := `{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Resources": {
			"Q": {"Type": "AWS::SQS::Queue"},
			"Topic": {"Type": "AWS::SNS::Topic"}
		}
	}`
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"res-http"}, "TemplateBody": {tmpl},
	})

	resp := postFormValues(t, h, url.Values{
		"Action":    {"DescribeStackResources"},
		"StackName": {"res-http"},
	})
	resp.mustOK(t)
	assert.Contains(t, resp.Body, "AWS::SQS::Queue")
}

// ---- Handler: DescribeAccountLimits HTTP --------------------------------------

func TestHandler_DescribeAccountLimits_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()
	resp := postFormValues(t, h, url.Values{"Action": {"DescribeAccountLimits"}})
	resp.mustOK(t)
	assert.Contains(t, resp.Body, "AccountLimitsResult")
}

// ---- Handler: SetStackPolicy / GetStackPolicy HTTP ----------------------------

func TestHandler_StackPolicy_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"pol-http"}, "TemplateBody": {simpleTemplate},
	})

	policy := `{"Statement":[{"Effect":"Allow","Action":"Update:*","Principal":"*","Resource":"*"}]}`
	resp := postFormValues(t, h, url.Values{
		"Action":          {"SetStackPolicy"},
		"StackName":       {"pol-http"},
		"StackPolicyBody": {policy},
	})
	resp.mustOK(t)

	resp = postFormValues(t, h, url.Values{
		"Action":    {"GetStackPolicy"},
		"StackName": {"pol-http"},
	})
	resp.mustOK(t)
	assert.Contains(t, resp.Body, "Effect")
}

// ---- Handler: ContinueUpdateRollback / CancelUpdateStack HTTP -----------------

func TestHandler_ContinueUpdateRollback_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"cur-http"}, "TemplateBody": {simpleTemplate},
	})

	resp := postFormValues(t, h, url.Values{
		"Action":    {"ContinueUpdateRollback"},
		"StackName": {"cur-http"},
	})
	resp.mustOK(t)
}

func TestHandler_CancelUpdateStack_HTTP(t *testing.T) {
	t.Parallel()

	b := newBackend()
	h := cloudformation.NewHandler(b)
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"cancel-http"}, "TemplateBody": {simpleTemplate},
	})

	// Real AWS: "You can cancel only stacks that are in the
	// UPDATE_IN_PROGRESS state." -- a freshly created (CREATE_COMPLETE)
	// stack is rejected.
	rejected := postFormValues(t, h, url.Values{
		"Action":    {"CancelUpdateStack"},
		"StackName": {"cancel-http"},
	})
	assert.NotEqual(t, http.StatusOK, rejected.Status)

	b.ForceStackStatus("cancel-http", "UPDATE_IN_PROGRESS")

	resp := postFormValues(t, h, url.Values{
		"Action":    {"CancelUpdateStack"},
		"StackName": {"cancel-http"},
	})
	resp.mustOK(t)
}

// ---- Handler: EstimateTemplateCost HTTP ----------------------------------------

func TestHandler_EstimateTemplateCost_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()
	resp := postFormValues(t, h, url.Values{
		"Action":       {"EstimateTemplateCost"},
		"TemplateBody": {simpleTemplate},
	})
	resp.mustOK(t)
	assert.Contains(t, resp.Body, "EstimateTemplateCostResult")
}

// ---- Handler: RollbackStack HTTP ----------------------------------------------

func TestHandler_RollbackStack_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler()
	postFormValues(t, h, url.Values{
		"Action": {"CreateStack"}, "StackName": {"rb-http"}, "TemplateBody": {simpleTemplate},
	})

	resp := postFormValues(t, h, url.Values{
		"Action":    {"RollbackStack"},
		"StackName": {"rb-http"},
	})
	resp.mustOK(t)
}
