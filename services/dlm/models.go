package dlm

import (
	"maps"
	"slices"
	"strings"
	"time"
)

const (
	policyIDPrefix = "policy-"
	stateEnabled   = "ENABLED"
	stateDisabled  = "DISABLED"
)

// storedPolicy holds a lifecycle policy with all persisted fields.
// time.Time fields are first so their non-pointer prefix reduces GC pointer bytes.
type storedPolicy struct {
	DateCreated      time.Time         `json:"dateCreated"`
	DateModified     time.Time         `json:"dateModified"`
	Tags             map[string]string `json:"tags"`
	PolicyDetails    map[string]any    `json:"policyDetails,omitempty"`
	Description      string            `json:"description"`
	ExecutionRoleARN string            `json:"executionRoleArn"`
	PolicyArn        string            `json:"policyArn"`
	PolicyID         string            `json:"policyId"`
	State            string            `json:"state"`
	// StatusMessage is real AWS's description of why a policy is in ERROR
	// state. This backend's state machine never produces ERROR (only
	// ENABLED/DISABLED, see stateEnabled/stateDisabled), so this always
	// stays empty -- present for wire completeness (gopherstack-x009).
	StatusMessage string `json:"statusMessage,omitempty"`
}

func (p *storedPolicy) toPolicy() *Policy {
	tags := make(map[string]string)
	maps.Copy(tags, p.Tags)

	return &Policy{
		DateCreated:      p.DateCreated,
		DateModified:     p.DateModified,
		Description:      p.Description,
		ExecutionRoleARN: p.ExecutionRoleARN,
		PolicyArn:        p.PolicyArn,
		PolicyID:         p.PolicyID,
		State:            p.State,
		StatusMessage:    p.StatusMessage,
		Tags:             tags,
		PolicyDetails:    p.PolicyDetails,
		DefaultPolicy:    policyDetailsIsDefaultPolicy(p.PolicyDetails),
	}
}

func (p *storedPolicy) toSummary() *PolicySummary {
	tags := make(map[string]string)
	maps.Copy(tags, p.Tags)

	return &PolicySummary{
		PolicyID:      p.PolicyID,
		Description:   p.Description,
		State:         p.State,
		Tags:          tags,
		PolicyType:    policyDetailsPolicyType(p.PolicyDetails),
		DefaultPolicy: policyDetailsIsDefaultPolicy(p.PolicyDetails),
	}
}

// tagPair is a decoded "key=value" filter entry, or a {Key,Value} tag
// extracted from a stored PolicyDetails document.
type tagPair struct {
	Key   string
	Value string
}

// policyDetailsPolicyType returns the PolicyType carried in a stored
// PolicyDetails document, defaulting to EBS_SNAPSHOT_MANAGEMENT to match AWS
// behavior when PolicyType is unspecified.
func policyDetailsPolicyType(details map[string]any) string {
	if pt, ok := details["PolicyType"].(string); ok && pt != "" {
		return pt
	}

	return "EBS_SNAPSHOT_MANAGEMENT"
}

// policyDetailsIsDefaultPolicy reports whether details belongs to a default
// (as opposed to custom) policy, matching the real LifecyclePolicy/
// LifecyclePolicySummary top-level DefaultPolicy bool
// (aws-sdk-go-v2/service/dlm@v1.39.4/types/types.go:411,449). PolicyLanguage
// is a reliable signal: applyTo (see defaultPolicyFields) sets it to
// SIMPLIFIED precisely when, and only when, a policy was created via the
// top-level default-policy request fields.
func policyDetailsIsDefaultPolicy(details map[string]any) bool {
	pl, _ := details["PolicyLanguage"].(string)

	return strings.EqualFold(pl, "SIMPLIFIED")
}

// policyDetailsStringSlice reads a []string-shaped field out of a decoded
// PolicyDetails document (stored as map[string]any, so list elements arrive
// as []any of string).
func policyDetailsStringSlice(details map[string]any, key string) []string {
	raw, _ := details[key].([]any)
	out := make([]string, 0, len(raw))

	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}

	return out
}

// policyDetailsTagPairs reads a []{Key,Value}-shaped field (e.g. TargetTags,
// or a schedule's TagsToAdd) out of a decoded PolicyDetails/schedule
// document.
func policyDetailsTagPairs(doc map[string]any, key string) []tagPair {
	raw, _ := doc[key].([]any)
	out := make([]tagPair, 0, len(raw))

	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}

		k, _ := m["Key"].(string)
		val, _ := m["Value"].(string)
		out = append(out, tagPair{Key: k, Value: val})
	}

	return out
}

// policyDetailsScheduleTagsToAdd gathers the TagsToAdd of every schedule in a
// stored PolicyDetails document.
func policyDetailsScheduleTagsToAdd(details map[string]any) []tagPair {
	schedules, _ := details["Schedules"].([]any)

	var out []tagPair

	for _, s := range schedules {
		sm, ok := s.(map[string]any)
		if !ok {
			continue
		}

		out = append(out, policyDetailsTagPairs(sm, "TagsToAdd")...)
	}

	return out
}

// parseTagPairs decodes "key=value" query filter strings (the wire format
// documented for the targetTags/tagsToAdd GetLifecyclePolicies query
// parameters) into tagPairs. Entries without an "=" are skipped.
func parseTagPairs(raw []string) []tagPair {
	out := make([]tagPair, 0, len(raw))

	for _, s := range raw {
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			continue
		}

		out = append(out, tagPair{Key: k, Value: v})
	}

	return out
}

// matchesDefaultPolicyType reports whether details satisfies the
// defaultPolicyType query filter (GetLifecyclePoliciesInput.DefaultPolicyType,
// aws-sdk-go-v2/service/dlm@v1.39.4/api_op_GetLifecyclePolicies.go:32-40):
// "VOLUME" only the EBS-snapshot default policy, "INSTANCE" only the
// EBS-backed-AMI default policy, "ALL" any default policy. An empty want
// imposes no restriction.
func matchesDefaultPolicyType(details map[string]any, want string) bool {
	if want == "" {
		return true
	}

	if !policyDetailsIsDefaultPolicy(details) {
		return false
	}

	if strings.EqualFold(want, "ALL") {
		return true
	}

	rt, _ := details["ResourceType"].(string)

	return strings.EqualFold(rt, want)
}

// matchesResourceTypes reports whether want is empty, or any entry in want
// case-insensitively matches an entry of details' ResourceTypes.
func matchesResourceTypes(details map[string]any, want []string) bool {
	if len(want) == 0 {
		return true
	}

	got := policyDetailsStringSlice(details, "ResourceTypes")
	for _, w := range want {
		for _, g := range got {
			if strings.EqualFold(g, w) {
				return true
			}
		}
	}

	return false
}

// matchesAnyTagPair reports whether want is empty, or any pair in want is
// present in have.
func matchesAnyTagPair(have, want []tagPair) bool {
	if len(want) == 0 {
		return true
	}

	for _, w := range want {
		if slices.Contains(have, w) {
			return true
		}
	}

	return false
}

// defaultPolicyFields holds the "[Default policies only]" top-level request
// members shared by CreateLifecyclePolicyInput and UpdateLifecyclePolicyInput
// (aws-sdk-go-v2/service/dlm@v1.39.4/api_op_CreateLifecyclePolicy.go:65-138,
// api_op_UpdateLifecyclePolicy.go:39-97). The real API has no separate
// storage for them: types.PolicyDetails documents identically-named members
// (types/types.go:512-648) for all of them except DefaultPolicy, whose
// request-side value (VOLUME/INSTANCE) is the same enum PolicyDetails uses
// for its ResourceType member (types/types.go:614-622) -- so these are
// folded into the stored PolicyDetails document under those keys.
// DefaultPolicy is Create-only: UpdateLifecyclePolicyInput has no such
// member, since the policy type can't change after creation.
type defaultPolicyFields struct {
	Exclusions             map[string]any
	CopyTags               *bool
	CreateInterval         *int32
	RetainInterval         *int32
	ExtendDeletion         *bool
	DefaultPolicy          string
	CrossRegionCopyTargets []any
}

// defaultPolicyOverrideCount is the number of PolicyDetails keys overrides
// can set: CopyTags, CreateInterval, RetainInterval, ExtendDeletion,
// CrossRegionCopyTargets, Exclusions.
const defaultPolicyOverrideCount = 6

func (f defaultPolicyFields) empty() bool {
	return f.DefaultPolicy == "" && f.CopyTags == nil && f.CreateInterval == nil &&
		f.RetainInterval == nil && f.ExtendDeletion == nil && f.CrossRegionCopyTargets == nil &&
		f.Exclusions == nil
}

// overrides renders f's non-DefaultPolicy members as PolicyDetails-keyed
// entries.
func (f defaultPolicyFields) overrides() map[string]any {
	out := make(map[string]any, defaultPolicyOverrideCount)

	if f.CopyTags != nil {
		out["CopyTags"] = *f.CopyTags
	}

	if f.CreateInterval != nil {
		out["CreateInterval"] = *f.CreateInterval
	}

	if f.RetainInterval != nil {
		out["RetainInterval"] = *f.RetainInterval
	}

	if f.ExtendDeletion != nil {
		out["ExtendDeletion"] = *f.ExtendDeletion
	}

	if f.CrossRegionCopyTargets != nil {
		out["CrossRegionCopyTargets"] = f.CrossRegionCopyTargets
	}

	if f.Exclusions != nil {
		out["Exclusions"] = f.Exclusions
	}

	return out
}

// applyTo returns details with f's fields merged in, always overwriting a
// same-named key already present -- callers that also send a conflicting
// nested PolicyDetails value get the explicit top-level value, matching the
// real API's documented (if unenforced client-side) "either top-level or
// PolicyDetails, not both" contract. Returns details unchanged, including a
// nil details, when f is empty.
func (f defaultPolicyFields) applyTo(details map[string]any) map[string]any {
	if f.empty() {
		return details
	}

	const resourceTypeAndPolicyLanguage = 2

	merged := make(map[string]any, len(details)+defaultPolicyOverrideCount+resourceTypeAndPolicyLanguage)
	maps.Copy(merged, details)

	if f.DefaultPolicy != "" {
		merged["ResourceType"] = f.DefaultPolicy
		merged["PolicyLanguage"] = "SIMPLIFIED"
	}

	maps.Copy(merged, f.overrides())

	return merged
}
