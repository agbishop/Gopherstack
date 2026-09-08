package cloudwatch

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/aws/smithy-go/encoding/cbor"
	"github.com/labstack/echo/v5"
)

func (h *Handler) cborPutInsightRule(input cbor.Map, c *echo.Context) error {
	return h.cborPutInsightRuleWithName(cborStr(input, "RuleName"), input, c)
}

func (h *Handler) cborPutInsightRuleWithName(
	ruleName string,
	input cbor.Map,
	c *echo.Context,
) error {
	if ruleName == "" {
		return h.cborError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"RuleName is required",
		)
	}

	definition := cborStr(input, "RuleDefinition")
	if err := validateInsightRuleDefinition(definition); err != nil {
		return h.cborError(c, http.StatusBadRequest, "InvalidParameterValueException", err.Error())
	}

	if err := h.Backend.PutInsightRule(&InsightRule{
		Name:       ruleName,
		Definition: definition,
		State:      cborStr(input, "RuleState"),
	}); err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	if rule, err := h.Backend.GetInsightRule(ruleName); err == nil {
		h.applyCreationTags(input, rule.Arn)
	}

	return writeCBOR(c, cbor.Map{})
}

func (h *Handler) cborDeleteInsightRules(input cbor.Map, c *echo.Context) error {
	ruleNames := cborStringSlice(input["RuleNames"])
	if len(ruleNames) == 0 {
		return h.cborError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"RuleNames is required",
		)
	}

	arns := h.insightRuleARNs(ruleNames)

	failures, err := h.Backend.DeleteInsightRules(ruleNames)
	if err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	for _, a := range arns {
		h.deleteResourceTags(a)
	}

	return writeCBOR(c, buildInsightRuleFailureCBOR(failures))
}

func (h *Handler) cborDescribeInsightRules(input cbor.Map, c *echo.Context) error {
	nextToken := cborStr(input, "NextToken")
	maxResults := int(cborInt32(input, "MaxResults"))

	p, err := h.Backend.DescribeInsightRules(nextToken, maxResults)
	if err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	rules := make(cbor.List, 0, len(p.Data))
	for _, r := range p.Data {
		entry := cbor.Map{
			keyName:       cbor.String(r.Name),
			keyState:      cbor.String(r.State),
			"Schema":      cbor.String(r.Schema),
			"Definition":  cbor.String(r.Definition),
			"ManagedRule": cbor.Bool(r.ManagedRule),
		}
		if r.Arn != "" {
			entry["RuleArn"] = cbor.String(r.Arn)
		}
		if !r.CreatedAt.IsZero() {
			entry["CreatedAt"] = cborFromTime(r.CreatedAt)
		}
		rules = append(rules, entry)
	}

	out := cbor.Map{
		"InsightRules": rules,
	}
	if p.Next != "" {
		out["NextToken"] = cbor.String(p.Next)
	}

	return writeCBOR(c, out)
}

func (h *Handler) cborDisableInsightRules(input cbor.Map, c *echo.Context) error {
	ruleNames := cborStringSlice(input["RuleNames"])
	if len(ruleNames) == 0 {
		return h.cborError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"RuleNames is required",
		)
	}

	failures, err := h.Backend.DisableInsightRules(ruleNames)
	if err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	return writeCBOR(c, buildInsightRuleFailureCBOR(failures))
}

func (h *Handler) cborEnableInsightRules(input cbor.Map, c *echo.Context) error {
	ruleNames := cborStringSlice(input["RuleNames"])
	if len(ruleNames) == 0 {
		return h.cborError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"RuleNames is required",
		)
	}

	failures, err := h.Backend.EnableInsightRules(ruleNames)
	if err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	return writeCBOR(c, buildInsightRuleFailureCBOR(failures))
}

func (h *Handler) cborGetInsightRuleReport(input cbor.Map, c *echo.Context) error {
	ruleName := cborStr(input, "RuleName")
	if ruleName == "" {
		return h.cborError(
			c,
			http.StatusBadRequest,
			"InvalidParameterValue",
			"RuleName is required",
		)
	}
	if _, err := h.Backend.GetInsightRule(ruleName); err != nil {
		return h.cborError(c, http.StatusBadRequest, "ResourceNotFoundException", err.Error())
	}

	maxContributors := int(cborInt32(input, "MaxContributorCount"))
	if maxContributors <= 0 {
		maxContributors = 10
	}
	orderBy := cborStr(input, "OrderBy")

	startTime := time.Now().UTC().Add(-time.Hour)
	if _, ok := input["StartTime"]; ok {
		startTime = cborTime(input, "StartTime")
	}
	endTime := time.Now().UTC()
	if _, ok := input["EndTime"]; ok {
		endTime = cborTime(input, "EndTime")
	}

	var contributors []InsightRuleContributor
	if bk, ok := h.Backend.(*InMemoryBackend); ok {
		var innerErr error
		func() {
			bk.mu.RLock("GetInsightRuleReport")
			defer bk.mu.RUnlock()
			contributors, innerErr = bk.GetInsightRuleContributors(
				ruleName,
				startTime,
				endTime,
				maxContributors,
				orderBy,
			)
		}()
		if innerErr != nil {
			return h.cborError(
				c,
				http.StatusBadRequest,
				"ResourceNotFoundException",
				innerErr.Error(),
			)
		}
	}

	contribList := make(cbor.List, 0, len(contributors))
	var aggregateValue float64
	for _, contrib := range contributors {
		keys := make(cbor.List, 0, len(contrib.Keys))
		for _, k := range contrib.Keys {
			keys = append(keys, cbor.String(k))
		}
		contribList = append(contribList, cbor.Map{
			"Keys":                      keys,
			"ApproximateAggregateValue": cbor.Float64(contrib.Sum),
			// Datapoints is required (types.InsightRuleContributor,
			// cloudwatch@v1.66.3 schemas/schemas.go:1085) but this backend has
			// no per-timestamp breakdown to offer (aggregateContributorRecord
			// only accumulates a single range-wide sum) -- emitted honestly
			// empty rather than fabricated, matching the top-level
			// MetricDatapoints field's same documented limitation. NOT
			// provable via a real aws-sdk-go-v2 client round trip: smithy-go's
			// rpc-v2-cbor deserializer collapses a present-but-zero-length
			// list to a nil Go slice identically to an absent key (confirmed
			// for both this field and MuteTargets.AlarmNames), so this key's
			// presence is unobservable from the client side. Fixed for wire
			// correctness anyway; not counted as a proven bug.
			"Datapoints": cbor.List{},
		})
		aggregateValue += contrib.Sum
	}

	// KeyLabels: extract key expressions from the rule definition (best-effort).
	rule, _ := h.Backend.GetInsightRule(ruleName)
	keyLabels := extractInsightRuleKeyLabels(rule)

	return writeCBOR(c, cbor.Map{
		"KeyLabels":              keyLabels,
		"AggregationStatistic":   cbor.String("Sum"),
		"AggregateValue":         cbor.Float64(aggregateValue),
		"ApproximateUniqueCount": cbor.Uint(uint64(len(contributors))),
		"Contributors":           contribList,
		"MetricDatapoints":       cbor.List{},
	})
}

// extractInsightRuleKeyLabels returns a cbor.List of key-label strings from the
// Contribution.Keys array in the rule definition JSON. Returns an empty list if
// the rule is nil or the definition cannot be parsed.
func extractInsightRuleKeyLabels(rule *InsightRule) cbor.List {
	if rule == nil || rule.Definition == "" {
		return cbor.List{}
	}
	var def struct {
		Contribution struct {
			Keys []string `json:"Keys"`
		} `json:"Contribution"`
	}
	if err := json.Unmarshal([]byte(rule.Definition), &def); err != nil {
		return cbor.List{}
	}
	labels := make(cbor.List, 0, len(def.Contribution.Keys))
	for _, k := range def.Contribution.Keys {
		labels = append(labels, cbor.String(k))
	}

	return labels
}

func (h *Handler) cborListManagedInsightRules(input cbor.Map, c *echo.Context) error {
	resourceARN := cborStr(input, "ResourceARN")
	nextToken := cborStr(input, "NextToken")
	maxResults := int(cborInt32(input, "MaxResults"))

	p, err := h.Backend.ListManagedInsightRules(resourceARN, nextToken, maxResults)
	if err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	rules := make(cbor.List, 0, len(p.Data))
	for _, rule := range p.Data {
		// rule.Definition holds the managed rule's TemplateName (set by
		// PutManagedInsightRules); rule.Name is the RuleName, which belongs
		// under the nested RuleState below, not here.
		entry := cbor.Map{
			"TemplateName": cbor.String(rule.Definition),
		}
		if rule.Arn != "" {
			entry["ResourceARN"] = cbor.String(rule.Arn)
		}
		ruleState := cbor.Map{
			"RuleName": cbor.String(rule.Name),
		}
		if rule.State != "" {
			ruleState[keyState] = cbor.String(rule.State)
		}
		entry["RuleState"] = ruleState
		rules = append(rules, entry)
	}

	out := cbor.Map{
		"ManagedRules": rules,
	}
	if p.Next != "" {
		out["NextToken"] = cbor.String(p.Next)
	}

	return writeCBOR(c, out)
}

func (h *Handler) cborPutManagedInsightRules(input cbor.Map, c *echo.Context) error {
	rulesRaw, ok := input["ManagedRules"]
	if !ok {
		return writeCBOR(c, buildInsightRuleFailureCBOR(nil))
	}

	rulesList, isList := rulesRaw.(cbor.List)
	if !isList {
		return writeCBOR(c, buildInsightRuleFailureCBOR(nil))
	}

	var failures []InsightRuleFailure
	for _, ruleRaw := range rulesList {
		if rule, isMap := ruleRaw.(cbor.Map); isMap {
			if fail := h.putOneManagedInsightRule(rule); fail != nil {
				failures = append(failures, *fail)
			}
		}
	}

	return writeCBOR(c, buildInsightRuleFailureCBOR(failures))
}

// putOneManagedInsightRule creates a single managed insight rule from one
// ManagedRules list entry, returning the resulting failure (or nil on
// success). A real client never sends RuleName (ManagedRule has no such
// member, only ResourceARN/TemplateName/Tags); a name is synthesized when
// one isn't present so internal callers/tests can still pass an explicit
// name.
func (h *Handler) putOneManagedInsightRule(rule cbor.Map) *InsightRuleFailure {
	resourceARN := cborStr(rule, "ResourceARN")
	templateName := cborStr(rule, "TemplateName")
	ruleName := cborStr(rule, "RuleName")
	if ruleName == "" {
		ruleName = managedInsightRuleName(resourceARN, templateName)
	}
	if ruleName == "" {
		return nil
	}

	if err := h.Backend.PutInsightRule(&InsightRule{
		Name:        ruleName,
		State:       insightRuleStateEnabled,
		Definition:  templateName,
		Arn:         resourceARN,
		ManagedRule: true,
	}); err != nil {
		return &InsightRuleFailure{
			RuleName:           ruleName,
			FailureCode:        errCodeInternalFailure,
			FailureDescription: err.Error(),
		}
	}

	return nil
}

// buildInsightRuleFailureCBOR builds a CBOR map for insight rule failure responses.
func buildInsightRuleFailureCBOR(failures []InsightRuleFailure) cbor.Map {
	failList := make(cbor.List, 0, len(failures))
	for _, f := range failures {
		failList = append(failList, cbor.Map{
			// Real member name is FailureResource, not RuleName
			// (cloudwatch@v1.66.3 schemas/schemas.go:3271, PartialFailure).
			"FailureResource":    cbor.String(f.RuleName),
			"FailureCode":        cbor.String(f.FailureCode),
			"FailureDescription": cbor.String(f.FailureDescription),
		})
	}

	return cbor.Map{
		"Failures": failList,
	}
}
