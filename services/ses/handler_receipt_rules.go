package ses

import (
	"encoding/xml"
	"net/url"
	"strconv"
	"strings"
)

func (h *Handler) handleCreateReceiptRule(vals url.Values, reqID string) (any, error) {
	ruleSetName := vals.Get("RuleSetName")
	after := vals.Get("After")

	enabled := vals.Get("Rule.Enabled") != boolFalse
	scanEnabled := vals.Get("Rule.ScanEnabled") != boolFalse

	rule := ReceiptRule{
		Name:        vals.Get("Rule.Name"),
		Enabled:     enabled,
		TLSPolicy:   vals.Get("Rule.TlsPolicy"),
		ScanEnabled: scanEnabled,
		Recipients:  parseSESMemberList(vals, "Rule.Recipients"),
		Actions:     parseReceiptActions(vals, "Rule.Actions"),
	}

	if err := h.Backend.CreateReceiptRule(ruleSetName, rule, after); err != nil {
		return nil, err
	}

	return &createReceiptRuleResponse{
		Xmlns:     sesXMLNS,
		RequestID: reqID,
	}, nil
}

func (h *Handler) handleCreateReceiptFilter(vals url.Values, reqID string) (any, error) {
	filter := ReceiptFilter{
		Name:   vals.Get("Filter.Name"),
		Policy: vals.Get("Filter.IpFilter.Policy"),
		CIDR:   vals.Get("Filter.IpFilter.Cidr"),
	}

	if err := h.Backend.CreateReceiptFilter(filter); err != nil {
		return nil, err
	}

	return &createReceiptFilterResponse{
		Xmlns:     sesXMLNS,
		RequestID: reqID,
	}, nil
}

type createReceiptRuleResult struct{}

type createReceiptRuleResponse struct {
	XMLName   xml.Name                `xml:"CreateReceiptRuleResponse"`
	Xmlns     string                  `xml:"xmlns,attr"`
	Result    createReceiptRuleResult `xml:"CreateReceiptRuleResult"`
	RequestID string                  `xml:"ResponseMetadata>RequestId"`
}

type createReceiptFilterResult struct{}

type createReceiptFilterResponse struct {
	XMLName   xml.Name                  `xml:"CreateReceiptFilterResponse"`
	Xmlns     string                    `xml:"xmlns,attr"`
	Result    createReceiptFilterResult `xml:"CreateReceiptFilterResult"`
	RequestID string                    `xml:"ResponseMetadata>RequestId"`
}

func (h *Handler) handleListReceiptFilters(reqID string) any {
	filters := h.Backend.ListReceiptFilters()
	members := make([]xmlReceiptFilter, 0, len(filters))
	for _, f := range filters {
		members = append(members, xmlReceiptFilter(f))
	}

	return &listReceiptFiltersResponse{
		Xmlns:     sesXMLNS,
		RequestID: reqID,
		Result: listReceiptFiltersResult{
			Filters: xmlReceiptFilterList{Members: members},
		},
	}
}

func (h *Handler) handleDeleteReceiptFilter(vals url.Values, reqID string) (any, error) {
	name := vals.Get("FilterName")
	if err := h.Backend.DeleteReceiptFilter(name); err != nil {
		return nil, err
	}

	return &deleteReceiptFilterResponse{Xmlns: sesXMLNS, RequestID: reqID}, nil
}

func (h *Handler) handleDeleteReceiptRule(vals url.Values, reqID string) (any, error) {
	ruleSetName := vals.Get("RuleSetName")
	ruleName := vals.Get("RuleName")
	if err := h.Backend.DeleteReceiptRule(ruleSetName, ruleName); err != nil {
		return nil, err
	}

	return &deleteReceiptRuleResponse{Xmlns: sesXMLNS, RequestID: reqID}, nil
}

// hasPrefixedKey reports whether any key in vals starts with prefix. Used by
// parseReceiptActions to detect which action type was submitted for a given
// Rule.Actions.member.N slot: detection must not key off a single "known"
// subfield (e.g. S3Action.BucketName), because a submission that names the
// action type but omits that field (a required member with an empty value)
// would otherwise look identical to "no action at this index" and be
// silently dropped instead of reaching validateReceiptAction.
func hasPrefixedKey(vals url.Values, prefix string) bool {
	for k := range vals {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}

	return false
}

// parseReceiptActions parses Rule.Actions.member.N.{ActionType}Action form values.
func parseReceiptActions(vals url.Values, prefix string) []ReceiptAction {
	var actions []ReceiptAction
	base := prefix + ".member."

	for i := 1; ; i++ {
		idx := base + strconv.Itoa(i)

		var action ReceiptAction

		switch {
		case hasPrefixedKey(vals, idx+".S3Action."):
			action = ReceiptAction{
				Type:         ReceiptActionTypeS3,
				S3BucketName: vals.Get(idx + ".S3Action.BucketName"),
				S3KeyPrefix:  vals.Get(idx + ".S3Action.ObjectKeyPrefix"),
				S3TopicARN:   vals.Get(idx + ".S3Action.TopicArn"),
			}
		case hasPrefixedKey(vals, idx+".SNSAction."):
			action = ReceiptAction{
				Type:        ReceiptActionTypeSNS,
				SNSTopicARN: vals.Get(idx + ".SNSAction.TopicArn"),
			}
		case hasPrefixedKey(vals, idx+".LambdaAction."):
			action = ReceiptAction{
				Type:              ReceiptActionTypeLambda,
				LambdaFunctionARN: vals.Get(idx + ".LambdaAction.FunctionArn"),
				LambdaTopicARN:    vals.Get(idx + ".LambdaAction.TopicArn"),
			}
		case hasPrefixedKey(vals, idx+".AddHeaderAction."):
			action = ReceiptAction{
				Type:        ReceiptActionTypeAddHeader,
				HeaderName:  vals.Get(idx + ".AddHeaderAction.HeaderName"),
				HeaderValue: vals.Get(idx + ".AddHeaderAction.HeaderValue"),
			}
		case hasPrefixedKey(vals, idx+".BounceAction."):
			action = ReceiptAction{
				Type:           ReceiptActionTypeBounce,
				SMTPReplyCode:  vals.Get(idx + ".BounceAction.SmtpReplyCode"),
				StatusCode:     vals.Get(idx + ".BounceAction.StatusCode"),
				Message:        vals.Get(idx + ".BounceAction.Message"),
				Sender:         vals.Get(idx + ".BounceAction.Sender"),
				BounceTopicARN: vals.Get(idx + ".BounceAction.TopicArn"),
			}
		case hasPrefixedKey(vals, idx+".StopAction."):
			action = ReceiptAction{
				Type:           ReceiptActionTypeStop,
				BounceTopicARN: vals.Get(idx + ".StopAction.TopicArn"),
			}
		}

		if action.Type == "" {
			break
		}

		actions = append(actions, action)
	}

	return actions
}

func toXMLReceiptRule(r ReceiptRule) xmlReceiptRule {
	recipients := make([]xmlMember, 0, len(r.Recipients))
	for _, rec := range r.Recipients {
		recipients = append(recipients, xmlMember{Value: rec})
	}

	actions := make([]xmlReceiptAction, 0, len(r.Actions))
	for _, a := range r.Actions {
		actions = append(actions, receiptActionToXML(a))
	}

	return xmlReceiptRule{
		Name:        r.Name,
		Enabled:     r.Enabled,
		TLSPolicy:   r.TLSPolicy,
		ScanEnabled: r.ScanEnabled,
		Recipients:  xmlMemberList{Members: recipients},
		Actions:     xmlReceiptActionList{Members: actions},
	}
}

func receiptActionToXML(a ReceiptAction) xmlReceiptAction {
	x := xmlReceiptAction{}

	switch a.Type {
	case ReceiptActionTypeS3:
		x.S3Action = &xmlS3Action{
			BucketName:      a.S3BucketName,
			ObjectKeyPrefix: a.S3KeyPrefix,
			TopicARN:        a.S3TopicARN,
		}
	case ReceiptActionTypeSNS:
		x.SNSAction = &xmlSNSAction{TopicARN: a.SNSTopicARN}
	case ReceiptActionTypeLambda:
		x.LambdaAction = &xmlLambdaAction{FunctionARN: a.LambdaFunctionARN, TopicARN: a.LambdaTopicARN}
	case ReceiptActionTypeAddHeader:
		x.AddHeaderAction = &xmlAddHeaderAction{HeaderName: a.HeaderName, HeaderValue: a.HeaderValue}
	case ReceiptActionTypeBounce:
		x.BounceAction = &xmlBounceAction{
			SMTPReplyCode: a.SMTPReplyCode,
			StatusCode:    a.StatusCode,
			Message:       a.Message,
			Sender:        a.Sender,
			TopicARN:      a.BounceTopicARN,
		}
	case ReceiptActionTypeStop:
		x.StopAction = &xmlStopAction{Scope: "RuleSet", TopicARN: a.BounceTopicARN}
	}

	return x
}

type xmlReceiptFilter struct {
	Name   string `xml:"Name"`
	Policy string `xml:"IpFilter>Policy"`
	CIDR   string `xml:"IpFilter>Cidr"`
}

type xmlReceiptFilterList struct {
	Members []xmlReceiptFilter `xml:"member"`
}

type listReceiptFiltersResult struct {
	Filters xmlReceiptFilterList `xml:"Filters"`
}

type listReceiptFiltersResponse struct {
	XMLName   xml.Name                 `xml:"ListReceiptFiltersResponse"`
	Xmlns     string                   `xml:"xmlns,attr"`
	RequestID string                   `xml:"ResponseMetadata>RequestId"`
	Result    listReceiptFiltersResult `xml:"ListReceiptFiltersResult"`
}

type deleteReceiptFilterResult struct{}

type deleteReceiptFilterResponse struct {
	XMLName   xml.Name                  `xml:"DeleteReceiptFilterResponse"`
	Xmlns     string                    `xml:"xmlns,attr"`
	Result    deleteReceiptFilterResult `xml:"DeleteReceiptFilterResult"`
	RequestID string                    `xml:"ResponseMetadata>RequestId"`
}

type xmlS3Action struct {
	BucketName      string `xml:"BucketName"`
	ObjectKeyPrefix string `xml:"ObjectKeyPrefix,omitempty"`
	TopicARN        string `xml:"TopicArn,omitempty"`
}

type xmlSNSAction struct {
	TopicARN string `xml:"TopicArn"`
}

type xmlLambdaAction struct {
	FunctionARN string `xml:"FunctionArn"`
	TopicARN    string `xml:"TopicArn,omitempty"`
}

type xmlAddHeaderAction struct {
	HeaderName  string `xml:"HeaderName"`
	HeaderValue string `xml:"HeaderValue"`
}

type xmlBounceAction struct {
	SMTPReplyCode string `xml:"SmtpReplyCode"`
	StatusCode    string `xml:"StatusCode,omitempty"`
	Message       string `xml:"Message"`
	Sender        string `xml:"Sender"`
	TopicARN      string `xml:"TopicArn,omitempty"`
}

type xmlStopAction struct {
	Scope    string `xml:"Scope"`
	TopicARN string `xml:"TopicArn,omitempty"`
}

type xmlReceiptAction struct {
	S3Action        *xmlS3Action        `xml:"S3Action,omitempty"`
	SNSAction       *xmlSNSAction       `xml:"SNSAction,omitempty"`
	LambdaAction    *xmlLambdaAction    `xml:"LambdaAction,omitempty"`
	AddHeaderAction *xmlAddHeaderAction `xml:"AddHeaderAction,omitempty"`
	BounceAction    *xmlBounceAction    `xml:"BounceAction,omitempty"`
	StopAction      *xmlStopAction      `xml:"StopAction,omitempty"`
}

type xmlReceiptActionList struct {
	Members []xmlReceiptAction `xml:"member"`
}

type xmlReceiptRule struct {
	Name        string               `xml:"Name"`
	TLSPolicy   string               `xml:"TlsPolicy,omitempty"`
	Recipients  xmlMemberList        `xml:"Recipients"`
	Actions     xmlReceiptActionList `xml:"Actions"`
	Enabled     bool                 `xml:"Enabled"`
	ScanEnabled bool                 `xml:"ScanEnabled"`
}

type xmlReceiptRuleList struct {
	Members []xmlReceiptRule `xml:"member"`
}

type deleteReceiptRuleResult struct{}

type deleteReceiptRuleResponse struct {
	XMLName   xml.Name                `xml:"DeleteReceiptRuleResponse"`
	Xmlns     string                  `xml:"xmlns,attr"`
	Result    deleteReceiptRuleResult `xml:"DeleteReceiptRuleResult"`
	RequestID string                  `xml:"ResponseMetadata>RequestId"`
}

func (h *Handler) handleDescribeReceiptRule(vals url.Values, reqID string) (any, error) {
	rule, err := h.Backend.DescribeReceiptRule(vals.Get("RuleSetName"), vals.Get("RuleName"))
	if err != nil {
		return nil, err
	}

	return &describeReceiptRuleResponse{
		Xmlns:     sesXMLNS,
		RequestID: reqID,
		Result:    describeReceiptRuleResult{Rule: toXMLReceiptRule(rule)},
	}, nil
}

func (h *Handler) handleUpdateReceiptRule(vals url.Values, reqID string) (any, error) {
	ruleSetName := vals.Get("RuleSetName")
	enabled := vals.Get("Rule.Enabled") != boolFalse
	scanEnabled := vals.Get("Rule.ScanEnabled") != boolFalse

	rule := ReceiptRule{
		Name:        vals.Get("Rule.Name"),
		Enabled:     enabled,
		TLSPolicy:   vals.Get("Rule.TlsPolicy"),
		ScanEnabled: scanEnabled,
		Recipients:  parseSESMemberList(vals, "Rule.Recipients"),
		Actions:     parseReceiptActions(vals, "Rule.Actions"),
	}

	if err := h.Backend.UpdateReceiptRule(ruleSetName, rule); err != nil {
		return nil, err
	}

	return newEmptyResponseWithResult("UpdateReceiptRule", reqID), nil
}

func (h *Handler) handleReorderReceiptRuleSet(vals url.Values, reqID string) (any, error) {
	ruleSetName := vals.Get("RuleSetName")
	ruleNames := parseSESMemberList(vals, "RuleNames")

	if err := h.Backend.ReorderReceiptRuleSet(ruleSetName, ruleNames); err != nil {
		return nil, err
	}

	return newEmptyResponseWithResult("ReorderReceiptRuleSet", reqID), nil
}

func (h *Handler) handleSetReceiptRulePosition(vals url.Values, reqID string) (any, error) {
	ruleSetName := vals.Get("RuleSetName")
	ruleName := vals.Get("RuleName")
	after := vals.Get("After")

	if err := h.Backend.SetReceiptRulePosition(ruleSetName, ruleName, after); err != nil {
		return nil, err
	}

	return newEmptyResponseWithResult("SetReceiptRulePosition", reqID), nil
}

type describeReceiptRuleResult struct {
	Rule xmlReceiptRule `xml:"Rule"`
}

type describeReceiptRuleResponse struct {
	XMLName   xml.Name                  `xml:"DescribeReceiptRuleResponse"`
	Xmlns     string                    `xml:"xmlns,attr"`
	RequestID string                    `xml:"ResponseMetadata>RequestId"`
	Result    describeReceiptRuleResult `xml:"DescribeReceiptRuleResult"`
}
