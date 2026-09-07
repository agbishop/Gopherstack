package awsconfig

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// describeConfigRulesPageSize bounds a single DescribeConfigRules page.
const describeConfigRulesPageSize = 100

// Operation name constants for config rule ops.
const (
	opDeleteConfigRule                          = "DeleteConfigRule"
	opDeleteEvaluationResults                   = "DeleteEvaluationResults"
	opDescribeAggregateComplianceByConfigRules  = "DescribeAggregateComplianceByConfigRules"
	opDescribeComplianceByConfigRule            = "DescribeComplianceByConfigRule"
	opDescribeComplianceByResource              = "DescribeComplianceByResource"
	opDescribeConfigRuleEvaluationStatus        = "DescribeConfigRuleEvaluationStatus"
	opDescribeConfigRules                       = "DescribeConfigRules"
	opGetAggregateComplianceDetailsByConfigRule = "GetAggregateComplianceDetailsByConfigRule"
	opGetAggregateConfigRuleComplianceSummary   = "GetAggregateConfigRuleComplianceSummary"
	opGetComplianceDetailsByConfigRule          = "GetComplianceDetailsByConfigRule"
	opGetComplianceDetailsByResource            = "GetComplianceDetailsByResource"
	opGetComplianceSummaryByConfigRule          = "GetComplianceSummaryByConfigRule"
	opGetComplianceSummaryByResourceType        = "GetComplianceSummaryByResourceType"
	opGetCustomRulePolicy                       = "GetCustomRulePolicy"
	opPutConfigRule                             = "PutConfigRule"
	opPutEvaluations                            = "PutEvaluations"
	opPutExternalEvaluation                     = "PutExternalEvaluation"
	opStartConfigRulesEvaluation                = "StartConfigRulesEvaluation"
)

// configRuleSupportedOps returns the operation names this family handles.
func configRuleSupportedOps() []string {
	return []string{
		opDescribeConfigRules,
		opGetComplianceDetailsByConfigRule,
		opDeleteConfigRule,
		opDeleteEvaluationResults,
		opPutConfigRule,
		opDescribeConfigRuleEvaluationStatus,
		opDescribeComplianceByConfigRule,
		opDescribeComplianceByResource,
		opGetComplianceSummaryByConfigRule,
		opGetComplianceSummaryByResourceType,
		opGetComplianceDetailsByResource,
		opGetAggregateComplianceDetailsByConfigRule,
		opGetAggregateConfigRuleComplianceSummary,
		opDescribeAggregateComplianceByConfigRules,
		opPutEvaluations,
		opPutExternalEvaluation,
		opStartConfigRulesEvaluation,
		opGetCustomRulePolicy,
	}
}

// describeConfigRulesFiltersInput mirrors types.DescribeConfigRulesFilters.
// Both fields are accepted but inert: gopherstack's ConfigRule has no
// EvaluationMode/RuleEvaluationVisibility concept (PutConfigRule doesn't
// model the real types.ConfigRule.EvaluationModes field either), so there is
// no per-rule state here to filter by -- a filtered request currently returns
// the same unfiltered set as an unfiltered one, rather than silently dropping
// the Filters object as an unknown JSON key.
type describeConfigRulesFiltersInput struct {
	EvaluationMode           string `json:"EvaluationMode,omitempty"`
	RuleEvaluationVisibility string `json:"RuleEvaluationVisibility,omitempty"`
}

type describeConfigRulesInput struct {
	Filters         *describeConfigRulesFiltersInput `json:"Filters,omitempty"`
	NextToken       string                           `json:"NextToken,omitempty"`
	ConfigRuleNames []string                         `json:"ConfigRuleNames,omitempty"`
}

type describeConfigRulesOutput struct {
	NextToken   string       `json:"NextToken,omitempty"`
	ConfigRules []ConfigRule `json:"ConfigRules"`
}

func (h *Handler) handleDescribeConfigRules(
	_ context.Context,
	in *describeConfigRulesInput,
) (*describeConfigRulesOutput, error) {
	if err := page.ValidateToken(in.NextToken); err != nil {
		return nil, fmt.Errorf("%w: invalid NextToken", ErrInvalidNextToken)
	}

	all, err := h.Backend.DescribeConfigRules(in.ConfigRuleNames)
	if err != nil {
		return nil, err
	}

	p := page.New(all, in.NextToken, describeConfigRulesPageSize, describeConfigRulesPageSize)

	return &describeConfigRulesOutput{ConfigRules: p.Data, NextToken: p.Next}, nil
}

type getComplianceDetailsByConfigRuleInput struct {
	ConfigRuleName  string   `json:"ConfigRuleName"`
	NextToken       string   `json:"NextToken,omitempty"`
	ComplianceTypes []string `json:"ComplianceTypes,omitempty"`
}

type getComplianceDetailsByConfigRuleOutput struct {
	NextToken         string                     `json:"NextToken,omitempty"`
	EvaluationResults []DetailedEvaluationResult `json:"EvaluationResults"`
}

// handleGetComplianceDetailsByConfigRule returns the real per-resource compliance
// evaluation results recorded for a config rule.
func (h *Handler) handleGetComplianceDetailsByConfigRule(
	_ context.Context,
	in *getComplianceDetailsByConfigRuleInput,
) (*getComplianceDetailsByConfigRuleOutput, error) {
	results, err := h.Backend.GetComplianceDetailsByConfigRule(in.ConfigRuleName, in.ComplianceTypes)
	if err != nil {
		return nil, err
	}

	return &getComplianceDetailsByConfigRuleOutput{EvaluationResults: results}, nil
}

type deleteConfigRuleInput struct {
	ConfigRuleName string `json:"ConfigRuleName"`
}

type deleteConfigRuleOutput struct{}

func (h *Handler) handleDeleteConfigRule(
	_ context.Context,
	in *deleteConfigRuleInput,
) (*deleteConfigRuleOutput, error) {
	if err := h.Backend.DeleteConfigRule(in.ConfigRuleName); err != nil {
		return nil, err
	}

	return &deleteConfigRuleOutput{}, nil
}

type deleteEvaluationResultsInput struct {
	ConfigRuleName string `json:"ConfigRuleName"`
}

type deleteEvaluationResultsOutput struct{}

func (h *Handler) handleDeleteEvaluationResults(
	_ context.Context,
	in *deleteEvaluationResultsInput,
) (*deleteEvaluationResultsOutput, error) {
	if err := h.Backend.DeleteEvaluationResults(in.ConfigRuleName); err != nil {
		return nil, err
	}

	return &deleteEvaluationResultsOutput{}, nil
}

// PutConfigRule request/response types and handler.
type putConfigRuleSourceBody struct {
	Owner            string `json:"Owner"`
	SourceIdentifier string `json:"SourceIdentifier"`
}

type putConfigRuleScopeBody struct {
	ComplianceResourceID    string   `json:"ComplianceResourceId,omitempty"`
	TagKey                  string   `json:"TagKey,omitempty"`
	TagValue                string   `json:"TagValue,omitempty"`
	ComplianceResourceTypes []string `json:"ComplianceResourceTypes,omitempty"`
}

type putConfigRuleBody struct {
	Source                    *putConfigRuleSourceBody `json:"Source,omitempty"`
	Scope                     *putConfigRuleScopeBody  `json:"Scope,omitempty"`
	ConfigRuleName            string                   `json:"ConfigRuleName"`
	ConfigRuleState           string                   `json:"ConfigRuleState,omitempty"`
	Description               string                   `json:"Description,omitempty"`
	InputParameters           string                   `json:"InputParameters,omitempty"`
	MaximumExecutionFrequency string                   `json:"MaximumExecutionFrequency,omitempty"`
}

type putConfigRuleInput struct {
	ConfigRule putConfigRuleBody `json:"ConfigRule"`
	Tags       []Tag             `json:"Tags,omitempty"`
}

func (h *Handler) handlePutConfigRule(
	_ context.Context, in *putConfigRuleInput,
) (*emptyOutput, error) {
	rule := &ConfigRule{
		ConfigRuleName:            in.ConfigRule.ConfigRuleName,
		Description:               in.ConfigRule.Description,
		InputParameters:           in.ConfigRule.InputParameters,
		MaximumExecutionFrequency: in.ConfigRule.MaximumExecutionFrequency,
		ConfigRuleState:           in.ConfigRule.ConfigRuleState,
	}

	if in.ConfigRule.Source != nil {
		rule.Source = &ConfigRuleSource{
			Owner:            in.ConfigRule.Source.Owner,
			SourceIdentifier: in.ConfigRule.Source.SourceIdentifier,
		}
	}

	if in.ConfigRule.Scope != nil {
		rule.Scope = &ConfigRuleScope{
			ComplianceResourceTypes: in.ConfigRule.Scope.ComplianceResourceTypes,
			ComplianceResourceID:    in.ConfigRule.Scope.ComplianceResourceID,
			TagKey:                  in.ConfigRule.Scope.TagKey,
			TagValue:                in.ConfigRule.Scope.TagValue,
		}
	}

	if err := h.Backend.PutConfigRule(rule); err != nil {
		return nil, err
	}

	if len(in.Tags) > 0 {
		if err := h.Backend.TagResource(rule.ConfigRuleArn, in.Tags); err != nil {
			return nil, err
		}
	}

	return &emptyOutput{}, nil
}

// DescribeConfigRuleEvaluationStatus request/response types and handler.
type describeConfigRuleEvaluationStatusInput struct {
	ConfigRuleNames []string `json:"ConfigRuleNames"`
}
type describeConfigRuleEvaluationStatusOutput struct {
	ConfigRulesEvaluationStatus []ConfigRuleEvaluationStatus `json:"ConfigRulesEvaluationStatus"`
}

func (h *Handler) handleDescribeConfigRuleEvaluationStatus(
	_ context.Context, in *describeConfigRuleEvaluationStatusInput,
) (*describeConfigRuleEvaluationStatusOutput, error) {
	return &describeConfigRuleEvaluationStatusOutput{
		ConfigRulesEvaluationStatus: h.Backend.DescribeConfigRuleEvaluationStatus(in.ConfigRuleNames),
	}, nil
}

// DescribeComplianceByConfigRule request/response types and handler.
type describeComplianceByConfigRuleInput struct {
	ConfigRuleNames []string `json:"ConfigRuleNames"`
}
type describeComplianceByConfigRuleOutput struct {
	ComplianceByConfigRules []ComplianceByConfigRule `json:"ComplianceByConfigRules"`
}

func (h *Handler) handleDescribeComplianceByConfigRule(
	_ context.Context, in *describeComplianceByConfigRuleInput,
) (*describeComplianceByConfigRuleOutput, error) {
	return &describeComplianceByConfigRuleOutput{
		ComplianceByConfigRules: h.Backend.DescribeComplianceByConfigRule(in.ConfigRuleNames),
	}, nil
}

// DescribeComplianceByResource request/response types and handler.
type describeComplianceByResourceInput struct {
	ResourceType    string   `json:"ResourceType,omitempty"`
	ResourceID      string   `json:"ResourceId,omitempty"`
	NextToken       string   `json:"NextToken,omitempty"`
	ComplianceTypes []string `json:"ComplianceTypes,omitempty"`
}

type describeComplianceByResourceOutput struct {
	NextToken             string                 `json:"NextToken,omitempty"`
	ComplianceByResources []ComplianceByResource `json:"ComplianceByResources"`
}

func (h *Handler) handleDescribeComplianceByResource(
	_ context.Context, in *describeComplianceByResourceInput,
) (*describeComplianceByResourceOutput, error) {
	byResource := h.Backend.DescribeComplianceByResource(in.ResourceType, in.ResourceID, in.ComplianceTypes)

	return &describeComplianceByResourceOutput{ComplianceByResources: byResource}, nil
}

// GetComplianceDetailsByResource request/response types and handler.
type getComplianceDetailsByResourceInput struct {
	ResourceType    string   `json:"ResourceType"`
	ResourceID      string   `json:"ResourceId"`
	NextToken       string   `json:"NextToken,omitempty"`
	ComplianceTypes []string `json:"ComplianceTypes,omitempty"`
}
type getComplianceDetailsByResourceOutput struct {
	NextToken         string                     `json:"NextToken,omitempty"`
	EvaluationResults []DetailedEvaluationResult `json:"EvaluationResults"`
}

func (h *Handler) handleGetComplianceDetailsByResource(
	_ context.Context, in *getComplianceDetailsByResourceInput,
) (*getComplianceDetailsByResourceOutput, error) {
	return &getComplianceDetailsByResourceOutput{
		EvaluationResults: h.Backend.GetComplianceDetailsByResource(in.ResourceType, in.ResourceID, in.ComplianceTypes),
	}, nil
}

// GetComplianceSummaryByConfigRule request/response types and handler. Real
// GetComplianceSummaryByConfigRuleOutput wraps a single ComplianceSummary
// object under "ComplianceSummary" (confirmed at
// api_op_GetComplianceSummaryByConfigRule.go) -- this previously emitted an
// invented "ComplianceSummariesByConfigRule" list key that doesn't exist on
// the wire at all, so a real client's ComplianceSummary was always nil.
type getComplianceSummaryByConfigRuleOutput struct {
	ComplianceSummary ComplianceSummary `json:"ComplianceSummary"`
}

func (h *Handler) handleGetComplianceSummaryByConfigRule(
	_ context.Context, _ *emptyInput,
) (*getComplianceSummaryByConfigRuleOutput, error) {
	return &getComplianceSummaryByConfigRuleOutput{
		ComplianceSummary: h.Backend.GetComplianceSummaryByConfigRule(),
	}, nil
}

// GetComplianceSummaryByResourceType request/response types and handler.
type getComplianceSummaryByResourceTypeInput struct {
	ResourceTypes []string `json:"ResourceTypes"`
}

type getComplianceSummaryByResourceTypeOutput struct {
	ComplianceSummariesByResourceType []ComplianceSummaryByResourceType `json:"ComplianceSummariesByResourceType"`
}

func (h *Handler) handleGetComplianceSummaryByResourceType(
	_ context.Context, in *getComplianceSummaryByResourceTypeInput,
) (*getComplianceSummaryByResourceTypeOutput, error) {
	return &getComplianceSummaryByResourceTypeOutput{
		ComplianceSummariesByResourceType: h.Backend.GetComplianceSummaryByResourceType(in.ResourceTypes),
	}, nil
}

// GetAggregateComplianceDetailsByConfigRule request/response types and handler.
type getAggregateComplianceDetailsByConfigRuleInput struct {
	ConfigurationAggregatorName string `json:"ConfigurationAggregatorName"`
	ConfigRuleName              string `json:"ConfigRuleName"`
	AccountID                   string `json:"AccountId"`
	AwsRegion                   string `json:"AwsRegion"`
	NextToken                   string `json:"NextToken,omitempty"`
	ComplianceType              string `json:"ComplianceType,omitempty"`
	Limit                       int    `json:"Limit,omitempty"`
}

type getAggregateComplianceDetailsByConfigRuleOutput struct {
	AggregateEvaluationResults []AggregateEvaluationResult `json:"AggregateEvaluationResults"`
}

func (h *Handler) handleGetAggregateComplianceDetailsByConfigRule(
	_ context.Context, in *getAggregateComplianceDetailsByConfigRuleInput,
) (*getAggregateComplianceDetailsByConfigRuleOutput, error) {
	var complianceTypes []string
	if in.ComplianceType != "" {
		complianceTypes = []string{in.ComplianceType}
	}

	results, err := h.Backend.GetAggregateComplianceDetailsByConfigRule(
		in.ConfigurationAggregatorName, in.ConfigRuleName, in.AccountID, in.AwsRegion, complianceTypes,
	)
	if err != nil {
		return nil, err
	}

	return &getAggregateComplianceDetailsByConfigRuleOutput{AggregateEvaluationResults: results}, nil
}

// GetAggregateConfigRuleComplianceSummary request/response types and
// handler. Real GetAggregateConfigRuleComplianceSummaryOutput echoes the
// request's GroupByKey ("the key passed into the request object" per
// api_op_GetAggregateConfigRuleComplianceSummary.go) -- this was never
// emitted at all.
type getAggregateConfigRuleComplianceSummaryInput struct {
	ConfigurationAggregatorName string `json:"ConfigurationAggregatorName"`
	GroupByKey                  string `json:"GroupByKey,omitempty"`
}
type getAggregateConfigRuleComplianceSummaryOutput struct {
	GroupByKey                string                     `json:"GroupByKey,omitempty"`
	AggregateComplianceCounts []AggregateComplianceCount `json:"AggregateComplianceCounts"`
}

func (h *Handler) handleGetAggregateConfigRuleComplianceSummary(
	_ context.Context, in *getAggregateConfigRuleComplianceSummaryInput,
) (*getAggregateConfigRuleComplianceSummaryOutput, error) {
	counts, err := h.Backend.GetAggregateConfigRuleComplianceSummary(in.ConfigurationAggregatorName, in.GroupByKey)
	if err != nil {
		return nil, err
	}

	return &getAggregateConfigRuleComplianceSummaryOutput{
		GroupByKey:                in.GroupByKey,
		AggregateComplianceCounts: counts,
	}, nil
}

// DescribeAggregateComplianceByConfigRules request/response types and handler.
type describeAggregateComplianceByConfigRulesOutput struct {
	AggregateComplianceByConfigRules []any `json:"AggregateComplianceByConfigRules"`
}

func (h *Handler) handleDescribeAggregateComplianceByConfigRules(
	_ context.Context, _ *emptyInput,
) (*describeAggregateComplianceByConfigRulesOutput, error) {
	return &describeAggregateComplianceByConfigRulesOutput{
		AggregateComplianceByConfigRules: h.Backend.DescribeAggregateComplianceByConfigRules(),
	}, nil
}

// evaluationBody accepts the AWS-shaped evaluation payload. AWS uses
// ComplianceResourceType/ComplianceResourceId; ConfigRuleName is carried
// alongside so the result can be associated with its rule.
type evaluationBody struct {
	ConfigRuleName         string `json:"ConfigRuleName,omitempty"`
	ComplianceResourceType string `json:"ComplianceResourceType,omitempty"`
	ComplianceResourceID   string `json:"ComplianceResourceId,omitempty"`
	ComplianceType         string `json:"ComplianceType"`
	Annotation             string `json:"Annotation,omitempty"`
}

func (e evaluationBody) toResult(fallbackRule string) EvaluationResult {
	rule := e.ConfigRuleName
	if rule == "" {
		rule = fallbackRule
	}

	return EvaluationResult{
		ConfigRuleName: rule,
		ComplianceType: e.ComplianceType,
		ResourceType:   e.ComplianceResourceType,
		ResourceID:     e.ComplianceResourceID,
		Annotation:     e.Annotation,
	}
}

// PutEvaluations request/response types and handler.
type putEvaluationsInput struct {
	ConfigRuleName string           `json:"ConfigRuleName,omitempty"`
	ResultToken    string           `json:"ResultToken,omitempty"`
	Evaluations    []evaluationBody `json:"Evaluations"`
}

func (h *Handler) handlePutEvaluations(
	_ context.Context, in *putEvaluationsInput,
) (*emptyOutput, error) {
	results := make([]EvaluationResult, 0, len(in.Evaluations))
	for _, e := range in.Evaluations {
		results = append(results, e.toResult(in.ConfigRuleName))
	}

	return &emptyOutput{}, h.Backend.PutEvaluations(results)
}

// PutExternalEvaluation request/response types and handler.
type putExternalEvaluationInput struct {
	ConfigRuleName     string         `json:"ConfigRuleName"`
	ExternalEvaluation evaluationBody `json:"ExternalEvaluation"`
}

func (h *Handler) handlePutExternalEvaluation(
	_ context.Context, in *putExternalEvaluationInput,
) (*emptyOutput, error) {
	return &emptyOutput{}, h.Backend.PutExternalEvaluation(in.ExternalEvaluation.toResult(in.ConfigRuleName))
}

// StartConfigRulesEvaluation request/response types and handler.
func (h *Handler) handleStartConfigRulesEvaluation(
	_ context.Context, _ *emptyInput,
) (*emptyOutput, error) {
	return &emptyOutput{}, h.Backend.StartConfigRulesEvaluation()
}

// GetCustomRulePolicy request/response types and handler.
type getCustomRulePolicyInput struct {
	ConfigRuleName string `json:"ConfigRuleName"`
}
type getCustomRulePolicyOutput struct {
	PolicyText string `json:"PolicyText"`
}

func (h *Handler) handleGetCustomRulePolicy(
	_ context.Context, in *getCustomRulePolicyInput,
) (*getCustomRulePolicyOutput, error) {
	return &getCustomRulePolicyOutput{
		PolicyText: h.Backend.GetCustomRulePolicy(in.ConfigRuleName),
	}, nil
}

// buildConfigRuleDispatch returns dispatch entries for config rule ops.
func (h *Handler) buildConfigRuleDispatch() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		opDescribeConfigRules:              service.WrapOp(h.handleDescribeConfigRules),
		opGetComplianceDetailsByConfigRule: service.WrapOp(h.handleGetComplianceDetailsByConfigRule),
		opDeleteConfigRule:                 service.WrapOp(h.handleDeleteConfigRule),
		opDeleteEvaluationResults:          service.WrapOp(h.handleDeleteEvaluationResults),
		opPutConfigRule:                    service.WrapOp(h.handlePutConfigRule),
		opDescribeConfigRuleEvaluationStatus: service.WrapOp(
			h.handleDescribeConfigRuleEvaluationStatus,
		),
		opDescribeComplianceByConfigRule: service.WrapOp(h.handleDescribeComplianceByConfigRule),
		opDescribeComplianceByResource:   service.WrapOp(h.handleDescribeComplianceByResource),
		opGetComplianceSummaryByConfigRule: service.WrapOp(
			h.handleGetComplianceSummaryByConfigRule,
		),
		opGetComplianceSummaryByResourceType: service.WrapOp(
			h.handleGetComplianceSummaryByResourceType,
		),
		opGetComplianceDetailsByResource: service.WrapOp(h.handleGetComplianceDetailsByResource),
		opGetAggregateComplianceDetailsByConfigRule: service.WrapOp(
			h.handleGetAggregateComplianceDetailsByConfigRule,
		),
		opGetAggregateConfigRuleComplianceSummary: service.WrapOp(
			h.handleGetAggregateConfigRuleComplianceSummary,
		),
		opDescribeAggregateComplianceByConfigRules: service.WrapOp(
			h.handleDescribeAggregateComplianceByConfigRules,
		),
		opPutEvaluations:             service.WrapOp(h.handlePutEvaluations),
		opPutExternalEvaluation:      service.WrapOp(h.handlePutExternalEvaluation),
		opStartConfigRulesEvaluation: service.WrapOp(h.handleStartConfigRulesEvaluation),
		opGetCustomRulePolicy:        service.WrapOp(h.handleGetCustomRulePolicy),
	}
}
