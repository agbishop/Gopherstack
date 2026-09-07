package ses_test

import (
	"maps"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ses"
)

func TestReceiptActionToXML_AllTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		actionParams string
		wantContains string
	}{
		{
			name: "s3_action",
			actionParams: url.Values{
				"Rule.Actions.member.1.S3Action.BucketName":      {"my-bucket"},
				"Rule.Actions.member.1.S3Action.ObjectKeyPrefix": {"prefix/"},
			}.Encode(),
			wantContains: "my-bucket",
		},
		{
			name: "sns_action",
			actionParams: url.Values{
				"Rule.Actions.member.1.SNSAction.TopicArn": {"arn:aws:sns:us-east-1:123:topic"},
			}.Encode(),
			wantContains: "TopicArn",
		},
		{
			name: "lambda_action",
			actionParams: url.Values{
				"Rule.Actions.member.1.LambdaAction.FunctionArn": {"arn:aws:lambda:us-east-1:123:fn"},
			}.Encode(),
			wantContains: "FunctionArn",
		},
		{
			name: "sqs_action",
			actionParams: url.Values{
				"Rule.Actions.member.1.SqsAction.QueueArn": {"arn:aws:sqs:us-east-1:123:q"},
			}.Encode(),
			wantContains: "QueueArn",
		},
		{
			name: "add_header_action",
			actionParams: url.Values{
				"Rule.Actions.member.1.AddHeaderAction.HeaderName":  {"X-My-Header"},
				"Rule.Actions.member.1.AddHeaderAction.HeaderValue": {"val"},
			}.Encode(),
			wantContains: "X-My-Header",
		},
		{
			name: "bounce_action",
			actionParams: url.Values{
				"Rule.Actions.member.1.BounceAction.SmtpReplyCode": {"550"},
				"Rule.Actions.member.1.BounceAction.StatusCode":    {"5.1.1"},
				"Rule.Actions.member.1.BounceAction.Message":       {"rejected"},
				"Rule.Actions.member.1.BounceAction.Sender":        {"noreply@example.com"},
			}.Encode(),
			wantContains: "550",
		},
		{
			name: "stop_action",
			actionParams: url.Values{
				"Rule.Actions.member.1.StopAction.Scope": {"RuleSet"},
			}.Encode(),
			wantContains: "RuleSet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()

			// Create a rule set and a rule with the action.
			postForm(t, h, "Action=CreateReceiptRuleSet&Version=2010-12-01&RuleSetName=test-rs")

			createBody := url.Values{
				"Action":       {"CreateReceiptRule"},
				"Version":      {"2010-12-01"},
				"RuleSetName":  {"test-rs"},
				"Rule.Name":    {"rule1"},
				"Rule.Enabled": {"true"},
			}
			// Merge action params.
			parsed, err := url.ParseQuery(tt.actionParams)
			require.NoError(t, err)
			maps.Copy(createBody, parsed)
			postForm(t, h, createBody.Encode())

			// Now describe it — this exercises toXMLReceiptRule + receiptActionToXML.
			rec := postForm(t, h, "Action=DescribeReceiptRule&Version=2010-12-01&RuleSetName=test-rs&RuleName=rule1")
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_DescribeReceiptRule_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "ruleset_not_found",
			body:         "Action=DescribeReceiptRule&Version=2010-12-01&RuleSetName=missing&RuleName=r1",
			wantCode:     http.StatusBadRequest,
			wantContains: "RuleSetDoesNotExist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_UpdateReceiptRule_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *ses.Handler)
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "ruleset_not_found",
			body:         "Action=UpdateReceiptRule&Version=2010-12-01&RuleSetName=missing&Rule.Name=r1",
			wantCode:     http.StatusBadRequest,
			wantContains: "RuleSetDoesNotExist",
		},
		{
			name: "valid_update",
			setup: func(h *ses.Handler) {
				postForm(t, h, "Action=CreateReceiptRuleSet&Version=2010-12-01&RuleSetName=myrs")
				postForm(t, h, "Action=CreateReceiptRule&Version=2010-12-01&RuleSetName=myrs&Rule.Name=myrule")
			},
			body:         "Action=UpdateReceiptRule&Version=2010-12-01&RuleSetName=myrs&Rule.Name=myrule&Rule.Enabled=true",
			wantCode:     http.StatusOK,
			wantContains: "UpdateReceiptRuleResponse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_ReorderReceiptRuleSet_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "ruleset_not_found",
			body:         "Action=ReorderReceiptRuleSet&Version=2010-12-01&RuleSetName=missing&RuleNames.member.1=r1",
			wantCode:     http.StatusBadRequest,
			wantContains: "RuleSetDoesNotExist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestHandler_SetReceiptRulePosition_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(t *testing.T, h *ses.Handler)
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "ruleset_not_found",
			body:         "Action=SetReceiptRulePosition&Version=2010-12-01&RuleSetName=missing&RuleName=r1&After=r0",
			wantCode:     http.StatusBadRequest,
			wantContains: "RuleSetDoesNotExist",
		},
		{
			name: "rule_not_found",
			setup: func(t *testing.T, h *ses.Handler) {
				t.Helper()
				require.NoError(t, h.Backend.CreateReceiptRuleSet("rs1"))
			},
			body:         "Action=SetReceiptRulePosition&Version=2010-12-01&RuleSetName=rs1&RuleName=missing&After=r0",
			wantCode:     http.StatusBadRequest,
			wantContains: "RuleDoesNotExist",
		},
		{
			name: "after_rule_not_found",
			setup: func(t *testing.T, h *ses.Handler) {
				t.Helper()
				require.NoError(t, h.Backend.CreateReceiptRuleSet("rs1"))
				require.NoError(t, h.Backend.CreateReceiptRule("rs1", ses.ReceiptRule{Name: "r1"}, ""))
			},
			body:         "Action=SetReceiptRulePosition&Version=2010-12-01&RuleSetName=rs1&RuleName=r1&After=no-such-rule",
			wantCode:     http.StatusBadRequest,
			wantContains: "RuleDoesNotExist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}
			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

func TestCreateReceiptRule_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.CreateReceiptRuleSet("rs1"))

	rec := postForm(t, h, url.Values{
		"Action":                   {"CreateReceiptRule"},
		"Version":                  {"2010-12-01"},
		"RuleSetName":              {"rs1"},
		"Rule.Name":                {"rule1"},
		"Rule.Enabled":             {"true"},
		"Rule.ScanEnabled":         {"true"},
		"Rule.Recipients.member.1": {"user@example.com"},
		"Rule.TlsPolicy":           {"Require"},
		"Rule.Actions.member.1.SNSAction.TopicArn": {"arn:aws:sns:us-east-1:123:t"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rule, err := h.Backend.DescribeReceiptRule("rs1", "rule1")
	require.NoError(t, err)
	assert.Equal(t, "rule1", rule.Name)
	assert.True(t, rule.Enabled)
	assert.True(t, rule.ScanEnabled)
	assert.Equal(t, []string{"user@example.com"}, rule.Recipients)
	assert.Equal(t, "Require", rule.TLSPolicy)
	require.Len(t, rule.Actions, 1)
	assert.Equal(t, ses.ReceiptActionTypeSNS, rule.Actions[0].Type)
}

func TestUpdateReceiptRule_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.CreateReceiptRuleSet("rs1"))
	require.NoError(t, h.Backend.CreateReceiptRule("rs1", ses.ReceiptRule{Name: "r1", Enabled: false}, ""))

	rec := postForm(t, h, url.Values{
		"Action":       {"UpdateReceiptRule"},
		"Version":      {"2010-12-01"},
		"RuleSetName":  {"rs1"},
		"Rule.Name":    {"r1"},
		"Rule.Enabled": {"true"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rule, err := h.Backend.DescribeReceiptRule("rs1", "r1")
	require.NoError(t, err)
	assert.True(t, rule.Enabled)
}

func TestDescribeReceiptRule_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.CreateReceiptRuleSet("rs1"))
	require.NoError(t, h.Backend.CreateReceiptRule("rs1", ses.ReceiptRule{Name: "r1", Enabled: true}, ""))

	rec := postForm(t, h, url.Values{
		"Action":      {"DescribeReceiptRule"},
		"Version":     {"2010-12-01"},
		"RuleSetName": {"rs1"},
		"RuleName":    {"r1"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "DescribeReceiptRuleResponse")
}

func TestDeleteReceiptRule_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.CreateReceiptRuleSet("rs1"))
	require.NoError(t, h.Backend.CreateReceiptRule("rs1", ses.ReceiptRule{Name: "r1"}, ""))

	rec := postForm(t, h, url.Values{
		"Action":      {"DeleteReceiptRule"},
		"Version":     {"2010-12-01"},
		"RuleSetName": {"rs1"},
		"RuleName":    {"r1"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rs, err := h.Backend.DescribeReceiptRuleSet("rs1")
	require.NoError(t, err)
	assert.Empty(t, rs.Rules)
}

func TestReorderReceiptRuleSet_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.CreateReceiptRuleSet("rs1"))
	require.NoError(t, h.Backend.CreateReceiptRule("rs1", ses.ReceiptRule{Name: "r1"}, ""))
	require.NoError(t, h.Backend.CreateReceiptRule("rs1", ses.ReceiptRule{Name: "r2"}, ""))
	require.NoError(t, h.Backend.CreateReceiptRule("rs1", ses.ReceiptRule{Name: "r3"}, ""))

	rec := postForm(t, h, url.Values{
		"Action":             {"ReorderReceiptRuleSet"},
		"Version":            {"2010-12-01"},
		"RuleSetName":        {"rs1"},
		"RuleNames.member.1": {"r3"},
		"RuleNames.member.2": {"r1"},
		"RuleNames.member.3": {"r2"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)

	rs, err := h.Backend.DescribeReceiptRuleSet("rs1")
	require.NoError(t, err)
	assert.Equal(t, []string{"r3", "r1", "r2"}, ruleNames(rs.Rules))
}

// TestSetReceiptRulePosition_Handler proves the rule set ends up in the
// AWS-documented order after SetReceiptRulePosition, not merely that the
// request parses. The real wire field is After (SetReceiptRulePositionInput.After,
// api_op_SetReceiptRulePosition.go:51) -- there is no numeric position field;
// a handler reading a fabricated "Position" key would leave the After param
// unread and always move the rule to the front, which these assertions catch.
func TestSetReceiptRulePosition_Handler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ruleName  string
		after     string
		wantOrder []string
	}{
		{
			name:      "after_named_rule_inserts_immediately_following_it",
			ruleName:  "r3",
			after:     "r1",
			wantOrder: []string{"r2", "r1", "r3"},
		},
		{
			name:      "empty_after_moves_rule_to_front",
			ruleName:  "r1",
			after:     "",
			wantOrder: []string{"r1", "r3", "r2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			require.NoError(t, h.Backend.CreateReceiptRuleSet("rs1"))
			// each CreateReceiptRule call with after="" prepends, so starting order is [r3, r2, r1].
			require.NoError(t, h.Backend.CreateReceiptRule("rs1", ses.ReceiptRule{Name: "r1"}, ""))
			require.NoError(t, h.Backend.CreateReceiptRule("rs1", ses.ReceiptRule{Name: "r2"}, ""))
			require.NoError(t, h.Backend.CreateReceiptRule("rs1", ses.ReceiptRule{Name: "r3"}, ""))

			rec := postForm(t, h, url.Values{
				"Action":      {"SetReceiptRulePosition"},
				"Version":     {"2010-12-01"},
				"RuleSetName": {"rs1"},
				"RuleName":    {tt.ruleName},
				"After":       {tt.after},
			}.Encode())
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			rs, err := h.Backend.DescribeReceiptRuleSet("rs1")
			require.NoError(t, err)
			assert.Equal(t, tt.wantOrder, ruleNames(rs.Rules))
		})
	}
}

func TestReceiptRule_After_Parameter(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.CreateReceiptRuleSet("rs"))
	// empty after="" prepends; successive prepends give [r2, r1]
	require.NoError(t, b.CreateReceiptRule("rs", ses.ReceiptRule{Name: "r1"}, ""))
	require.NoError(t, b.CreateReceiptRule("rs", ses.ReceiptRule{Name: "r2"}, ""))
	// insert r3 after r1: [r2, r1, r3]
	require.NoError(t, b.CreateReceiptRule("rs", ses.ReceiptRule{Name: "r3"}, "r1"))

	rs, err := b.DescribeReceiptRuleSet("rs")
	require.NoError(t, err)
	names := ruleNames(rs.Rules)
	assert.Equal(t, "r2", names[0])
	assert.Equal(t, "r1", names[1])
	assert.Equal(t, "r3", names[2])
}

// ruleNames extracts rule names from a slice of ReceiptRule.
func ruleNames(rules []ses.ReceiptRule) []string {
	names := make([]string, len(rules))
	for i, r := range rules {
		names[i] = r.Name
	}

	return names
}

func TestCreateReceiptFilter_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, url.Values{
		"Action":                 {"CreateReceiptFilter"},
		"Version":                {"2010-12-01"},
		"Filter.Name":            {"my-filter"},
		"Filter.IpFilter.Policy": {"Allow"},
		"Filter.IpFilter.Cidr":   {"10.0.0.0/8"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, h.Backend.(*ses.InMemoryBackend).ReceiptFilterCount())
}

func TestListReceiptFilters_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	h.Backend.(*ses.InMemoryBackend).AddReceiptFilterInternal(ses.ReceiptFilter{
		Name:   "f1",
		Policy: "Allow",
		CIDR:   "10.0.0.0/8",
	})

	rec := postForm(t, h, "Action=ListReceiptFilters&Version=2010-12-01")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "f1")
}

func TestDeleteReceiptFilter_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	h.Backend.(*ses.InMemoryBackend).AddReceiptFilterInternal(ses.ReceiptFilter{
		Name:   "del-f",
		Policy: "Block",
		CIDR:   "192.168.0.0/16",
	})

	rec := postForm(t, h, url.Values{
		"Action":     {"DeleteReceiptFilter"},
		"Version":    {"2010-12-01"},
		"FilterName": {"del-f"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, h.Backend.(*ses.InMemoryBackend).ReceiptFilterCount())
}

// TestDeleteReceiptFilter_NotFound_IsIdempotent replaces the previous
// TestDeleteReceiptFilter_NotFound_Error, which asserted a 400 here. That was
// wrong: DeleteReceiptFilter's own deserializer (ses@v1.37.4 deserializers.go)
// declares no exception at all, and botocore's ses/2010-12-01 service-2.json
// has no "errors" key on this op whatsoever -- a missing filter is a no-op,
// matching the sibling DeleteReceiptRule/DeleteReceiptRuleSet precedent (see
// undeclared_delete_errors_test.go).
func TestDeleteReceiptFilter_NotFound_IsIdempotent(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, url.Values{
		"Action":     {"DeleteReceiptFilter"},
		"Version":    {"2010-12-01"},
		"FilterName": {"nonexistent"},
	}.Encode())
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestReceiptFilter_AllowAndBlock_Policies(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.CreateReceiptFilter(ses.ReceiptFilter{
		Name:   "allow-filter",
		Policy: ses.FilterPolicyAllow,
		CIDR:   "10.0.0.0/8",
	}))
	require.NoError(t, b.CreateReceiptFilter(ses.ReceiptFilter{
		Name:   "block-filter",
		Policy: ses.FilterPolicyBlock,
		CIDR:   "192.168.0.0/16",
	}))

	filters := b.ListReceiptFilters()
	require.Len(t, filters, 2)

	policies := map[string]string{}
	for _, f := range filters {
		policies[f.Name] = f.Policy
	}
	assert.Equal(t, ses.FilterPolicyAllow, policies["allow-filter"])
	assert.Equal(t, ses.FilterPolicyBlock, policies["block-filter"])
}

func TestReceiptRule_S3Action(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.CreateReceiptRuleSet("rs"))

	rule := ses.ReceiptRule{
		Name:    "s3-rule",
		Enabled: true,
		Actions: []ses.ReceiptAction{
			{
				Type:         ses.ReceiptActionTypeS3,
				S3BucketName: "my-bucket",
				S3KeyPrefix:  "emails/",
			},
		},
	}
	require.NoError(t, b.CreateReceiptRule("rs", rule, ""))

	described, err := b.DescribeReceiptRule("rs", "s3-rule")
	require.NoError(t, err)
	require.Len(t, described.Actions, 1)
	assert.Equal(t, ses.ReceiptActionTypeS3, described.Actions[0].Type)
	assert.Equal(t, "my-bucket", described.Actions[0].S3BucketName)
}

func TestReceiptRule_LambdaAction(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.CreateReceiptRuleSet("rs"))

	rule := ses.ReceiptRule{
		Name:    "lambda-rule",
		Enabled: true,
		Actions: []ses.ReceiptAction{
			{
				Type:              ses.ReceiptActionTypeLambda,
				LambdaFunctionARN: "arn:aws:lambda:us-east-1:123:function:ProcessEmail",
			},
		},
	}
	require.NoError(t, b.CreateReceiptRule("rs", rule, ""))

	described, err := b.DescribeReceiptRule("rs", "lambda-rule")
	require.NoError(t, err)
	require.Len(t, described.Actions, 1)
	assert.Equal(t, ses.ReceiptActionTypeLambda, described.Actions[0].Type)
}

func TestReceiptRule_MultipleActions(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.CreateReceiptRuleSet("rs"))

	rule := ses.ReceiptRule{
		Name:    "multi-action",
		Enabled: true,
		Actions: []ses.ReceiptAction{
			{Type: ses.ReceiptActionTypeSNS, SNSTopicARN: "arn:sns:topic"},
			{Type: ses.ReceiptActionTypeAddHeader, HeaderName: "X-Processed", HeaderValue: "true"},
			{Type: ses.ReceiptActionTypeS3, S3BucketName: "bucket"},
		},
	}
	require.NoError(t, b.CreateReceiptRule("rs", rule, ""))

	described, err := b.DescribeReceiptRule("rs", "multi-action")
	require.NoError(t, err)
	assert.Len(t, described.Actions, 3)
}

func TestReceiptRule_BounceAction(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.CreateReceiptRuleSet("rs"))

	rule := ses.ReceiptRule{
		Name:    "bounce-rule",
		Enabled: true,
		Actions: []ses.ReceiptAction{
			{
				Type:          ses.ReceiptActionTypeBounce,
				SMTPReplyCode: "550",
				StatusCode:    "5.1.1",
				Message:       "User unknown",
				Sender:        "mailer-daemon@example.com",
			},
		},
	}
	require.NoError(t, b.CreateReceiptRule("rs", rule, ""))

	described, err := b.DescribeReceiptRule("rs", "bounce-rule")
	require.NoError(t, err)
	require.Len(t, described.Actions, 1)
	assert.Equal(t, ses.ReceiptActionTypeBounce, described.Actions[0].Type)
	assert.Equal(t, "550", described.Actions[0].SMTPReplyCode)
}

func TestDescribeReceiptRule_NotFound_Error(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.CreateReceiptRuleSet("rs"))
	_, err := b.DescribeReceiptRule("rs", "nonexistent")
	assert.Error(t, err)
}

func TestUpdateReceiptRule_NotFound_Error(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.CreateReceiptRuleSet("rs"))
	assert.Error(t, b.UpdateReceiptRule("rs", ses.ReceiptRule{Name: "nonexistent"}))
}

func TestReorderReceiptRuleSet_WrongCount_Error(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.CreateReceiptRuleSet("rs"))
	require.NoError(t, b.CreateReceiptRule("rs", ses.ReceiptRule{Name: "r1"}, ""))
	require.NoError(t, b.CreateReceiptRule("rs", ses.ReceiptRule{Name: "r2"}, ""))
	assert.Error(t, b.ReorderReceiptRuleSet("rs", []string{"r1"}))
}

func TestReceiptRule_Actions_CreateDescribe(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.CreateReceiptRuleSet("rs"))

	rule := ses.ReceiptRule{
		Name:    "rule1",
		Enabled: true,
		Actions: []ses.ReceiptAction{
			{Type: ses.ReceiptActionTypeSNS, SNSTopicARN: "arn:aws:sns:us-east-1:123:topic"},
			{Type: ses.ReceiptActionTypeAddHeader, HeaderName: "X-Custom", HeaderValue: "val"},
		},
	}
	require.NoError(t, b.CreateReceiptRule("rs", rule, ""))

	described, err := b.DescribeReceiptRule("rs", "rule1")
	require.NoError(t, err)
	require.Len(t, described.Actions, 2)
	assert.Equal(t, ses.ReceiptActionTypeSNS, described.Actions[0].Type)
	assert.Equal(t, "arn:aws:sns:us-east-1:123:topic", described.Actions[0].SNSTopicARN)
	assert.Equal(t, ses.ReceiptActionTypeAddHeader, described.Actions[1].Type)
	assert.Equal(t, "X-Custom", described.Actions[1].HeaderName)
}

func TestReceiptRule_Actions_Handler(t *testing.T) {
	t.Parallel()

	h := newHandler()
	require.NoError(t, h.Backend.CreateReceiptRuleSet("rs"))

	body := url.Values{
		"Action":       {"CreateReceiptRule"},
		"Version":      {"2010-12-01"},
		"RuleSetName":  {"rs"},
		"Rule.Name":    {"r1"},
		"Rule.Enabled": {"true"},
		"Rule.Actions.member.1.SNSAction.TopicArn": {"arn:aws:sns:us-east-1:123:t"},
	}.Encode()

	rec := postForm(t, h, body)
	assert.Equal(t, http.StatusOK, rec.Code)

	described, err := h.Backend.DescribeReceiptRule("rs", "r1")
	require.NoError(t, err)
	require.Len(t, described.Actions, 1)
	assert.Equal(t, ses.ReceiptActionTypeSNS, described.Actions[0].Type)
}

func TestListReceiptFilters_EmptyState(t *testing.T) {
	t.Parallel()

	h := newHandler()
	rec := postForm(t, h, url.Values{
		"Action":  {"ListReceiptFilters"},
		"Version": {"2010-12-01"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "ListReceiptFiltersResponse")
	assert.Contains(t, body, "Filters",
		"Filters element must be present even when empty")
}

// TestSESNewOps_CreateReceiptRule covers the CreateReceiptRule handler.
func TestCreateReceiptRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *ses.Handler)
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name: "success",
			body: url.Values{
				"Action":       {"CreateReceiptRule"},
				"Version":      {"2010-12-01"},
				"RuleSetName":  {"rules"},
				"Rule.Name":    {"my-rule"},
				"Rule.Enabled": {"true"},
			}.Encode(),
			setup: func(h *ses.Handler) {
				require.NoError(t, h.Backend.CreateReceiptRuleSet("rules"))
			},
			wantCode:     http.StatusOK,
			wantContains: "CreateReceiptRuleResponse",
		},
		{
			name:         "rule_set_not_found",
			body:         "Action=CreateReceiptRule&Version=2010-12-01&RuleSetName=missing&Rule.Name=r1",
			wantCode:     http.StatusBadRequest,
			wantContains: "RuleSetDoesNotExist",
		},
		{
			name: "duplicate_rule_returns_error",
			body: "Action=CreateReceiptRule&Version=2010-12-01&RuleSetName=rules&Rule.Name=existing",
			setup: func(h *ses.Handler) {
				require.NoError(t, h.Backend.CreateReceiptRuleSet("rules"))
				require.NoError(t, h.Backend.CreateReceiptRule("rules", ses.ReceiptRule{Name: "existing"}, ""))
			},
			wantCode:     http.StatusBadRequest,
			wantContains: "AlreadyExists",
		},
		{
			name: "empty_rule_name",
			body: "Action=CreateReceiptRule&Version=2010-12-01&RuleSetName=rules&Rule.Name=",
			setup: func(h *ses.Handler) {
				require.NoError(t, h.Backend.CreateReceiptRuleSet("rules"))
			},
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

// TestSESNewOps_CreateReceiptFilter covers the CreateReceiptFilter handler.
func TestCreateReceiptFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(h *ses.Handler)
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name: "success_allow",
			body: url.Values{
				"Action":                 {"CreateReceiptFilter"},
				"Version":                {"2010-12-01"},
				"Filter.Name":            {"allow-filter"},
				"Filter.IpFilter.Policy": {"Allow"},
				"Filter.IpFilter.Cidr":   {"10.0.0.0/8"},
			}.Encode(),
			wantCode:     http.StatusOK,
			wantContains: "CreateReceiptFilterResponse",
		},
		{
			name: "duplicate_filter_returns_error",
			body: url.Values{
				"Action":                 {"CreateReceiptFilter"},
				"Version":                {"2010-12-01"},
				"Filter.Name":            {"existing"},
				"Filter.IpFilter.Policy": {"Block"},
				"Filter.IpFilter.Cidr":   {"192.168.0.0/16"},
			}.Encode(),
			setup: func(h *ses.Handler) {
				require.NoError(
					t,
					h.Backend.CreateReceiptFilter(
						ses.ReceiptFilter{Name: "existing", Policy: "Block", CIDR: "0.0.0.0/0"},
					),
				)
			},
			wantCode:     http.StatusBadRequest,
			wantContains: "AlreadyExists",
		},
		{
			name: "empty_filter_name",
			body: url.Values{
				"Action":                 {"CreateReceiptFilter"},
				"Version":                {"2010-12-01"},
				"Filter.Name":            {""},
				"Filter.IpFilter.Policy": {"Allow"},
				"Filter.IpFilter.Cidr":   {"0.0.0.0/0"},
			}.Encode(),
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			if tt.setup != nil {
				tt.setup(h)
			}

			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

// TestHandler_ListReceiptFilters tests the ListReceiptFilters handler.
func TestHandler_ListReceiptFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(b *ses.InMemoryBackend)
		name         string
		wantContains string
		wantCode     int
	}{
		{
			name:         "empty_returns_empty_list",
			wantCode:     http.StatusOK,
			wantContains: "ListReceiptFiltersResponse",
		},
		{
			name: "with_filters",
			setup: func(b *ses.InMemoryBackend) {
				b.AddReceiptFilterInternal(ses.ReceiptFilter{Name: "filter1", Policy: "Allow", CIDR: "10.0.0.0/8"})
				b.AddReceiptFilterInternal(ses.ReceiptFilter{Name: "filter2", Policy: "Block", CIDR: "192.168.0.0/16"})
			},
			wantCode:     http.StatusOK,
			wantContains: "filter1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			if tt.setup != nil {
				tt.setup(h.Backend.(*ses.InMemoryBackend))
			}

			body := "Action=ListReceiptFilters&Version=2010-12-01"
			rec := postForm(t, h, body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

// TestHandler_DeleteReceiptFilter tests the DeleteReceiptFilter handler.
func TestHandler_DeleteReceiptFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(b *ses.InMemoryBackend)
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(b *ses.InMemoryBackend) {
				b.AddReceiptFilterInternal(ses.ReceiptFilter{Name: "my-filter", Policy: "Allow", CIDR: "10.0.0.0/8"})
			},
			body:         "Action=DeleteReceiptFilter&Version=2010-12-01&FilterName=my-filter",
			wantCode:     http.StatusOK,
			wantContains: "DeleteReceiptFilterResponse",
		},
		{
			// FilterDoesNotExist does not exist in this SDK; a missing
			// filter is idempotent (see TestDeleteReceiptFilter_NotFound_IsIdempotent).
			name:         "filter_not_found_is_idempotent",
			body:         "Action=DeleteReceiptFilter&Version=2010-12-01&FilterName=nonexistent",
			wantCode:     http.StatusOK,
			wantContains: "DeleteReceiptFilterResponse",
		},
		{
			name:         "empty_filter_name",
			body:         "Action=DeleteReceiptFilter&Version=2010-12-01&FilterName=",
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			if tt.setup != nil {
				tt.setup(h.Backend.(*ses.InMemoryBackend))
			}

			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

// TestHandler_DeleteReceiptRule tests the DeleteReceiptRule handler.
func TestHandler_DeleteReceiptRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(b *ses.InMemoryBackend)
		name         string
		body         string
		wantContains string
		wantCode     int
	}{
		{
			name: "success",
			setup: func(b *ses.InMemoryBackend) {
				b.AddReceiptRuleSetInternal(ses.ReceiptRuleSet{
					Name:      "my-set",
					CreatedAt: time.Now(),
					Rules:     []ses.ReceiptRule{{Name: "rule1"}},
				})
			},
			body:         "Action=DeleteReceiptRule&Version=2010-12-01&RuleSetName=my-set&RuleName=rule1",
			wantCode:     http.StatusOK,
			wantContains: "DeleteReceiptRuleResponse",
		},
		{
			name:         "rule_set_not_found",
			body:         "Action=DeleteReceiptRule&Version=2010-12-01&RuleSetName=missing&RuleName=rule1",
			wantCode:     http.StatusBadRequest,
			wantContains: "RuleSetDoesNotExist",
		},
		{
			// Idempotent: DeleteReceiptRule's own deserializer (ses@v1.37.4
			// deserializers.go) declares only RuleSetDoesNotExist, not RuleDoesNotExist.
			name: "rule_not_found_is_idempotent",
			setup: func(b *ses.InMemoryBackend) {
				b.AddReceiptRuleSetInternal(ses.ReceiptRuleSet{Name: "my-set", CreatedAt: time.Now()})
			},
			body:         "Action=DeleteReceiptRule&Version=2010-12-01&RuleSetName=my-set&RuleName=missing",
			wantCode:     http.StatusOK,
			wantContains: "DeleteReceiptRuleResponse",
		},
		{
			name:         "empty_rule_set_name",
			body:         "Action=DeleteReceiptRule&Version=2010-12-01&RuleSetName=&RuleName=rule1",
			wantCode:     http.StatusBadRequest,
			wantContains: "InvalidParameterValue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler()
			if tt.setup != nil {
				tt.setup(h.Backend.(*ses.InMemoryBackend))
			}

			rec := postForm(t, h, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantContains)
		})
	}
}

// TestBackend_CreateReceiptFilter_PolicyValidation tests Policy validation.
func TestBackend_CreateReceiptFilter_PolicyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		filter  ses.ReceiptFilter
		wantErr bool
	}{
		{
			name:    "allow_policy",
			filter:  ses.ReceiptFilter{Name: "f1", Policy: "Allow", CIDR: "10.0.0.0/8"},
			wantErr: false,
		},
		{
			name:    "block_policy",
			filter:  ses.ReceiptFilter{Name: "f2", Policy: "Block", CIDR: "10.0.0.0/8"},
			wantErr: false,
		},
		{
			name:    "empty_policy_is_allowed",
			filter:  ses.ReceiptFilter{Name: "f3", Policy: "", CIDR: "10.0.0.0/8"},
			wantErr: false,
		},
		{
			name:    "invalid_policy",
			filter:  ses.ReceiptFilter{Name: "f4", Policy: "Invalid", CIDR: "10.0.0.0/8"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ses.NewInMemoryBackend()
			err := b.CreateReceiptFilter(tt.filter)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestBackend_CreateReceiptRule_TLSPolicyValidation tests TLSPolicy validation.
func TestBackend_CreateReceiptRule_TLSPolicyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tlsPolicy string
		wantErr   bool
	}{
		{name: "optional", tlsPolicy: "Optional", wantErr: false},
		{name: "require", tlsPolicy: "Require", wantErr: false},
		{name: "empty_is_allowed", tlsPolicy: "", wantErr: false},
		{name: "invalid", tlsPolicy: "invalid-val", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := ses.NewInMemoryBackend()
			require.NoError(t, b.CreateReceiptRuleSet("rs"))
			err := b.CreateReceiptRule("rs", ses.ReceiptRule{Name: "r1", TLSPolicy: tt.tlsPolicy}, "")
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestBackend_CreateReceiptRule_PrependWhenAfterEmpty tests that rules are prepended when after="".
func TestBackend_CreateReceiptRule_PrependWhenAfterEmpty(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	require.NoError(t, b.CreateReceiptRuleSet("rs"))
	require.NoError(t, b.CreateReceiptRule("rs", ses.ReceiptRule{Name: "first"}, ""))
	require.NoError(t, b.CreateReceiptRule("rs", ses.ReceiptRule{Name: "second"}, ""))

	rs, err := b.DescribeReceiptRuleSet("rs")
	require.NoError(t, err)
	require.Len(t, rs.Rules, 2)
	assert.Equal(t, "second", rs.Rules[0].Name)
	assert.Equal(t, "first", rs.Rules[1].Name)
}

// TestBackend_ListReceiptFilters_SortedOrder tests that filters are returned sorted.
func TestBackend_ListReceiptFilters_SortedOrder(t *testing.T) {
	t.Parallel()

	b := ses.NewInMemoryBackend()
	b.AddReceiptFilterInternal(ses.ReceiptFilter{Name: "zzz-filter"})
	b.AddReceiptFilterInternal(ses.ReceiptFilter{Name: "aaa-filter"})
	b.AddReceiptFilterInternal(ses.ReceiptFilter{Name: "mmm-filter"})

	filters := b.ListReceiptFilters()
	require.Len(t, filters, 3)
	assert.Equal(t, "aaa-filter", filters[0].Name)
	assert.Equal(t, "mmm-filter", filters[1].Name)
	assert.Equal(t, "zzz-filter", filters[2].Name)
}
