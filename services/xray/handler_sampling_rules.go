package xray

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

type samplingRateBoostView struct {
	MaxRate               float64 `json:"MaxRate"`
	CooldownWindowMinutes int32   `json:"CooldownWindowMinutes"`
}

type samplingRuleView struct {
	SamplingRateBoost *samplingRateBoostView `json:"SamplingRateBoost,omitempty"`
	Attributes        map[string]string      `json:"Attributes,omitempty"`
	RuleARN           string                 `json:"RuleARN"`
	RuleName          string                 `json:"RuleName"`
	ResourceARN       string                 `json:"ResourceARN"`
	ServiceName       string                 `json:"ServiceName"`
	ServiceType       string                 `json:"ServiceType"`
	Host              string                 `json:"Host"`
	HTTPMethod        string                 `json:"HTTPMethod"`
	URLPath           string                 `json:"URLPath"`
	FixedRate         float64                `json:"FixedRate"`
	Priority          int32                  `json:"Priority"`
	ReservoirSize     int32                  `json:"ReservoirSize"`
	Version           int                    `json:"Version"`
}

type samplingRuleRecord struct {
	SamplingRule samplingRuleView `json:"SamplingRule"`
	CreatedAt    float64          `json:"CreatedAt"`
	ModifiedAt   float64          `json:"ModifiedAt"`
}

func toSamplingRuleView(r *SamplingRule) samplingRuleView {
	v := samplingRuleView{
		RuleARN:       r.RuleARN,
		RuleName:      r.RuleName,
		ResourceARN:   r.ResourceARN,
		ServiceName:   r.ServiceName,
		ServiceType:   r.ServiceType,
		Host:          r.Host,
		HTTPMethod:    r.HTTPMethod,
		URLPath:       r.URLPath,
		FixedRate:     r.FixedRate,
		Priority:      r.Priority,
		ReservoirSize: r.ReservoirSize,
		Attributes:    r.Attributes,
		Version:       1,
	}

	if r.SamplingRateBoost != nil {
		v.SamplingRateBoost = &samplingRateBoostView{
			MaxRate:               r.SamplingRateBoost.MaxRate,
			CooldownWindowMinutes: r.SamplingRateBoost.CooldownWindowMinutes,
		}
	}

	return v
}

func toSamplingRuleRecord(r *SamplingRule) samplingRuleRecord {
	return samplingRuleRecord{
		SamplingRule: toSamplingRuleView(r),
		CreatedAt:    float64(r.CreatedAt.Unix()),
		ModifiedAt:   float64(r.ModifiedAt.Unix()),
	}
}

type samplingRuleInput struct {
	SamplingRateBoost *samplingRateBoostView `json:"SamplingRateBoost,omitempty"`
	Attributes        map[string]string      `json:"Attributes,omitempty"`
	RuleName          string                 `json:"RuleName"`
	ResourceARN       string                 `json:"ResourceARN"`
	ServiceName       string                 `json:"ServiceName"`
	ServiceType       string                 `json:"ServiceType"`
	Host              string                 `json:"Host"`
	HTTPMethod        string                 `json:"HTTPMethod"`
	URLPath           string                 `json:"URLPath"`
	FixedRate         float64                `json:"FixedRate"`
	Priority          int32                  `json:"Priority"`
	ReservoirSize     int32                  `json:"ReservoirSize"`
}

type createSamplingRuleInput struct {
	SamplingRule samplingRuleInput `json:"SamplingRule"`
}

func (h *Handler) handleCreateSamplingRule(_ context.Context, body []byte) ([]byte, error) {
	var in createSamplingRuleInput
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return nil, err
		}
	}

	if in.SamplingRule.RuleName == "" {
		return nil, fmt.Errorf("%w: RuleName is required", errInvalidRequest)
	}

	rule := SamplingRule{
		RuleName:      in.SamplingRule.RuleName,
		ResourceARN:   in.SamplingRule.ResourceARN,
		ServiceName:   in.SamplingRule.ServiceName,
		ServiceType:   in.SamplingRule.ServiceType,
		Host:          in.SamplingRule.Host,
		HTTPMethod:    in.SamplingRule.HTTPMethod,
		URLPath:       in.SamplingRule.URLPath,
		FixedRate:     in.SamplingRule.FixedRate,
		Priority:      in.SamplingRule.Priority,
		ReservoirSize: in.SamplingRule.ReservoirSize,
		Attributes:    in.SamplingRule.Attributes,
	}

	if in.SamplingRule.SamplingRateBoost != nil {
		rule.SamplingRateBoost = &SamplingRateBoost{
			MaxRate:               in.SamplingRule.SamplingRateBoost.MaxRate,
			CooldownWindowMinutes: in.SamplingRule.SamplingRateBoost.CooldownWindowMinutes,
		}
	}

	if err := ValidateSamplingRule(rule); err != nil {
		return nil, err
	}

	r, err := h.Backend.CreateSamplingRule(rule)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		keySamplingRuleRecord: toSamplingRuleRecord(r),
	})
}

type getSamplingRulesInput struct {
	NextToken string `json:"NextToken"`
}

func (h *Handler) handleGetSamplingRules(_ context.Context, body []byte) ([]byte, error) {
	var in getSamplingRulesInput
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return nil, err
		}
	}

	rules := h.Backend.GetSamplingRules()
	records := make([]samplingRuleRecord, 0, len(rules))

	for i := range rules {
		records = append(records, toSamplingRuleRecord(&rules[i]))
	}

	pg := page.New(records, in.NextToken, 0, defaultSamplingRulesPageSize)

	return json.Marshal(map[string]any{
		"SamplingRuleRecords": pg.Data,
		keyNextToken:          pg.Next,
	})
}

// samplingRuleUpdateInput uses json.RawMessage so we can detect which fields
// were explicitly provided (even zero values like FixedRate=0). RuleName and RuleARN
// are both accepted (specify a rule by either, but not both, per the real
// SamplingRuleUpdate shape).
type samplingRuleUpdateInput struct {
	ResourceARN       *string                `json:"ResourceARN"`
	ServiceName       *string                `json:"ServiceName"`
	ServiceType       *string                `json:"ServiceType"`
	Host              *string                `json:"Host"`
	HTTPMethod        *string                `json:"HTTPMethod"`
	URLPath           *string                `json:"URLPath"`
	FixedRate         *float64               `json:"FixedRate"`
	Priority          *int32                 `json:"Priority"`
	ReservoirSize     *int32                 `json:"ReservoirSize"`
	SamplingRateBoost *samplingRateBoostView `json:"SamplingRateBoost,omitempty"`
	Attributes        map[string]string      `json:"Attributes,omitempty"`
	RuleName          string                 `json:"RuleName"`
	RuleARN           string                 `json:"RuleARN"`
}

type updateSamplingRuleInput struct {
	SamplingRuleUpdate samplingRuleUpdateInput `json:"SamplingRuleUpdate"`
}

func (h *Handler) handleUpdateSamplingRule(_ context.Context, body []byte) ([]byte, error) {
	var in updateSamplingRuleInput
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return nil, err
		}
	}

	if in.SamplingRuleUpdate.RuleName == "" && in.SamplingRuleUpdate.RuleARN == "" {
		return nil, fmt.Errorf("%w: RuleName or RuleARN is required", errInvalidRequest)
	}

	updates := SamplingRuleUpdate{
		ResourceARN:   in.SamplingRuleUpdate.ResourceARN,
		ServiceName:   in.SamplingRuleUpdate.ServiceName,
		ServiceType:   in.SamplingRuleUpdate.ServiceType,
		Host:          in.SamplingRuleUpdate.Host,
		HTTPMethod:    in.SamplingRuleUpdate.HTTPMethod,
		URLPath:       in.SamplingRuleUpdate.URLPath,
		FixedRate:     in.SamplingRuleUpdate.FixedRate,
		Priority:      in.SamplingRuleUpdate.Priority,
		ReservoirSize: in.SamplingRuleUpdate.ReservoirSize,
		Attributes:    in.SamplingRuleUpdate.Attributes,
	}

	if in.SamplingRuleUpdate.SamplingRateBoost != nil {
		updates.SamplingRateBoost = &SamplingRateBoost{
			MaxRate:               in.SamplingRuleUpdate.SamplingRateBoost.MaxRate,
			CooldownWindowMinutes: in.SamplingRuleUpdate.SamplingRateBoost.CooldownWindowMinutes,
		}
	}

	if err := ValidateSamplingRuleUpdate(updates); err != nil {
		return nil, err
	}

	r, err := h.Backend.UpdateSamplingRuleWithPointers(
		in.SamplingRuleUpdate.RuleName, in.SamplingRuleUpdate.RuleARN, updates,
	)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		keySamplingRuleRecord: toSamplingRuleRecord(r),
	})
}

type deleteSamplingRuleInput struct {
	RuleName string `json:"RuleName"`
	RuleARN  string `json:"RuleARN"`
}

func (h *Handler) handleDeleteSamplingRule(_ context.Context, body []byte) ([]byte, error) {
	var in deleteSamplingRuleInput
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return nil, err
		}
	}

	if in.RuleName == "" && in.RuleARN == "" {
		return nil, fmt.Errorf("%w: RuleName or RuleARN is required", errInvalidRequest)
	}

	r, err := h.Backend.DeleteSamplingRule(in.RuleName, in.RuleARN)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		keySamplingRuleRecord: toSamplingRuleRecord(r),
	})
}
