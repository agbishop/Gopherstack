package applicationautoscaling

import (
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// maxScalingPoliciesPerTarget is the real, non-adjustable AWS quota on the
// number of scaling policies (step + target tracking combined) per scalable
// target. Confirmed against the "Quotas for Application Auto Scaling" AWS
// documentation page.
const maxScalingPoliciesPerTarget = 50

// maxStepAdjustmentsPerPolicy is the real, adjustable-but-defaulted AWS quota
// on the number of step adjustments per step scaling policy. Confirmed
// against the "Quotas for Application Auto Scaling" AWS documentation page.
const maxStepAdjustmentsPerPolicy = 20

// The three real AWS PolicyType enum values (types.PolicyType in the
// vendored SDK).
const (
	policyTypeStepScaling           = "StepScaling"
	policyTypeTargetTrackingScaling = "TargetTrackingScaling"
	policyTypePredictiveScaling     = "PredictiveScaling"
)

// isValidPolicyType reports whether t is a recognised Application Auto Scaling
// PolicyType value.
func isValidPolicyType(t string) bool {
	switch t {
	case policyTypeStepScaling, policyTypeTargetTrackingScaling, policyTypePredictiveScaling:
		return true
	default:
		return false
	}
}

// cloneScalingPolicy returns a deep copy of p with config maps cloned.
func cloneScalingPolicy(p *ScalingPolicy) *ScalingPolicy {
	cp := *p
	cp.TargetTrackingConfig = maps.Clone(p.TargetTrackingConfig)
	cp.StepScalingConfig = maps.Clone(p.StepScalingConfig)
	cp.PredictiveScalingConfig = maps.Clone(p.PredictiveScalingConfig)
	cp.Alarms = append([]Alarm(nil), p.Alarms...)

	return &cp
}

// stepAdjustmentCount returns the number of entries in
// stepScalingConfig["StepAdjustments"], or 0 if absent/malformed. gopherstack
// stores StepScalingPolicyConfiguration as a passthrough map[string]any (see
// the package's general "don't over-validate sub-shapes" philosophy in
// PARITY.md), so this reads the wire field name directly rather than a typed
// struct.
func stepAdjustmentCount(stepScalingConfig map[string]any) int {
	raw, ok := stepScalingConfig["StepAdjustments"]
	if !ok {
		return 0
	}

	list, ok := raw.([]any)
	if !ok {
		return 0
	}

	return len(list)
}

// PutScalingPolicy upserts a scaling policy (update if policyName matches for resource, create otherwise).
func (b *InMemoryBackend) PutScalingPolicy(
	serviceNamespace, resourceID, scalableDimension, policyName, policyType string,
	targetTrackingConfig, stepScalingConfig, predictiveScalingConfig map[string]any,
) (*ScalingPolicy, error) {
	if serviceNamespace == "" {
		return nil, fmt.Errorf("%w: ServiceNamespace is required", ErrValidation)
	}

	if resourceID == "" {
		return nil, fmt.Errorf("%w: ResourceId is required", ErrValidation)
	}

	if scalableDimension == "" {
		return nil, fmt.Errorf("%w: ScalableDimension is required", ErrValidation)
	}

	if policyName == "" {
		return nil, fmt.Errorf("%w: PolicyName is required", ErrValidation)
	}

	// Validate PolicyType if provided; do not default yet — defaulting only
	// applies when creating a brand-new policy (see below).
	if policyType != "" && !isValidPolicyType(policyType) {
		return nil, fmt.Errorf(
			"%w: invalid PolicyType %q; must be one of StepScaling, TargetTrackingScaling, PredictiveScaling",
			ErrValidation,
			policyType,
		)
	}

	if stepAdjustmentCount(stepScalingConfig) > maxStepAdjustmentsPerPolicy {
		return nil, fmt.Errorf(
			"%w: too many step adjustments; maximum allowed is %d",
			ErrLimitExceeded,
			maxStepAdjustmentsPerPolicy,
		)
	}

	b.mu.Lock("PutScalingPolicy")
	defer b.mu.Unlock()

	// PutScalingPolicy's modeled error set includes ObjectNotFoundException:
	// real AWS requires the scalable target to already be registered (its
	// ObjectNotFoundException doc explicitly names "any operation that
	// depends on the existence of a scalable target").
	if !b.scalableTargetExists(serviceNamespace, resourceID, scalableDimension) {
		return nil, fmt.Errorf(
			"%w: scalable target %s not found",
			ErrNotFound,
			scalableTargetKey(serviceNamespace, resourceID, scalableDimension),
		)
	}

	now := time.Now().UTC()
	key := policyNameKey(serviceNamespace, resourceID, scalableDimension, policyName)

	if group := b.policiesByName.Get(key); len(group) > 0 {
		p := group[0]
		// Only update PolicyType when the caller explicitly provided one.
		if policyType != "" {
			p.PolicyType = policyType
		}

		p.TargetTrackingConfig = maps.Clone(targetTrackingConfig)
		p.StepScalingConfig = maps.Clone(stepScalingConfig)
		p.PredictiveScalingConfig = maps.Clone(predictiveScalingConfig)
		p.LastModifiedTime = now

		return cloneScalingPolicy(p), nil
	}

	// A brand-new policy counts against the per-target quota; updates to an
	// existing policy (handled above) do not.
	if b.policiesForTargetLocked(serviceNamespace, resourceID, scalableDimension) >= maxScalingPoliciesPerTarget {
		return nil, fmt.Errorf(
			"%w: maximum number of scaling policies (%d) per scalable target exceeded",
			ErrLimitExceeded,
			maxScalingPoliciesPerTarget,
		)
	}

	// Default PolicyType to StepScaling for new policies only -- this matches
	// real AWS/Terraform behavior (the aws_appautoscaling_policy resource
	// documents "StepScaling" as its default policy_type), not
	// TargetTrackingScaling.
	if policyType == "" {
		policyType = policyTypeStepScaling
	}

	// Real AWS policy ARNs separate the policyName segment from the
	// resource/namespace/resourceId segment with a colon, not a slash:
	// scalingPolicy:{uuid}:resource/{namespace}/{resourceId}:policyName/{name}.
	policyARN := arn.Build("autoscaling", b.region, b.accountID,
		fmt.Sprintf("scalingPolicy:%s:resource/%s/%s:policyName/%s",
			uuid.NewString(), serviceNamespace, resourceID, policyName))
	p := &ScalingPolicy{
		ServiceNamespace:        serviceNamespace,
		ResourceID:              resourceID,
		ScalableDimension:       scalableDimension,
		PolicyName:              policyName,
		PolicyType:              policyType,
		ARN:                     policyARN,
		TargetTrackingConfig:    maps.Clone(targetTrackingConfig),
		StepScalingConfig:       maps.Clone(stepScalingConfig),
		PredictiveScalingConfig: maps.Clone(predictiveScalingConfig),
		// Alarms is intentionally left nil (honest-empty). Real AWS creates
		// genuine backing CloudWatch alarms server-side for
		// TargetTrackingScaling/StepScaling policies, visible via
		// cloudwatch:DescribeAlarms. gopherstack's applicationautoscaling
		// backend has no reference to the cloudwatch service's backend --
		// the SetAppConfig/siblingServices pattern (see AppContext in
		// pkgs/service) would allow one from this service's own
		// provider.Init, but adopting it is a separate decision that has not
		// been taken -- so it cannot create a real alarm today. A
		// previous pass synthesized a stable-looking name + arn.Build ARN
		// here that resolved to nothing when queried against
		// cloudwatch:DescribeAlarms; that is exactly the kind of
		// invented-resource fabrication this project removes elsewhere, so
		// it was deleted. See PARITY.md gaps for the downgrade rationale.
		Alarms:           nil,
		CreationTime:     now,
		LastModifiedTime: now,
	}
	b.scalingPolicies.Put(p)
	cp := cloneScalingPolicy(p)

	return cp, nil
}

// policiesForTargetLocked counts the scaling policies registered against the
// given scalable target. Caller must hold the write lock.
func (b *InMemoryBackend) policiesForTargetLocked(serviceNamespace, resourceID, scalableDimension string) int {
	count := 0

	for _, p := range b.scalingPolicies.All() {
		if p.ServiceNamespace == serviceNamespace && p.ResourceID == resourceID &&
			p.ScalableDimension == scalableDimension {
			count++
		}
	}

	return count
}

// DeleteScalingPolicy removes a scaling policy by name.
func (b *InMemoryBackend) DeleteScalingPolicy(
	serviceNamespace, resourceID, scalableDimension, policyName string,
) error {
	if serviceNamespace == "" {
		return fmt.Errorf("%w: ServiceNamespace is required", ErrValidation)
	}

	if resourceID == "" {
		return fmt.Errorf("%w: ResourceId is required", ErrValidation)
	}

	if scalableDimension == "" {
		return fmt.Errorf("%w: ScalableDimension is required", ErrValidation)
	}

	if policyName == "" {
		return fmt.Errorf("%w: PolicyName is required", ErrValidation)
	}

	b.mu.Lock("DeleteScalingPolicy")
	defer b.mu.Unlock()

	key := policyNameKey(serviceNamespace, resourceID, scalableDimension, policyName)

	group := b.policiesByName.Get(key)
	if len(group) == 0 {
		return fmt.Errorf("%w: scaling policy %s not found", ErrNotFound, policyName)
	}

	b.scalingPolicies.Delete(group[0].ARN)

	return nil
}

// DescribeScalingPoliciesFilter carries optional filters for DescribeScalingPolicies.
type DescribeScalingPoliciesFilter struct {
	ServiceNamespace  string
	ResourceID        string
	ScalableDimension string
	NextToken         string
	PolicyNames       []string
	MaxResults        int32
}

// buildStringSet converts ss into an O(1) membership-test set. The returned set
// is nil when ss is empty, which callers can use as a "no filter" sentinel.
func buildStringSet(ss []string) map[string]bool {
	if len(ss) == 0 {
		return nil
	}

	out := make(map[string]bool, len(ss))
	for _, s := range ss {
		out[s] = true
	}

	return out
}

// policyMatchesFilter reports whether p satisfies filter f.
// nameSet is a pre-built O(1) lookup set derived from f.PolicyNames; it is
// passed in to avoid rebuilding it inside the inner loop. A nil set means
// "no filter on that dimension".
func policyMatchesFilter(p *ScalingPolicy, f DescribeScalingPoliciesFilter, nameSet map[string]bool) bool {
	if f.ServiceNamespace != "" && p.ServiceNamespace != f.ServiceNamespace {
		return false
	}

	if f.ResourceID != "" && p.ResourceID != f.ResourceID {
		return false
	}

	if f.ScalableDimension != "" && p.ScalableDimension != f.ScalableDimension {
		return false
	}

	if nameSet != nil && !nameSet[p.PolicyName] {
		return false
	}

	return true
}

// DescribeScalingPolicies lists scaling policies, optionally filtered, and
// returns the NextToken for the following page (empty on the last page).
// Returns ErrInvalidNextToken if f.NextToken fails to decode.
func (b *InMemoryBackend) DescribeScalingPolicies(f DescribeScalingPoliciesFilter) ([]*ScalingPolicy, string, error) {
	b.mu.RLock("DescribeScalingPolicies")
	defer b.mu.RUnlock()

	nameSet := buildStringSet(f.PolicyNames)

	list := make([]*ScalingPolicy, 0, b.scalingPolicies.Len())
	for _, p := range b.scalingPolicies.All() {
		if policyMatchesFilter(p, f, nameSet) {
			list = append(list, cloneScalingPolicy(p))
		}
	}

	return paginate(list, f.MaxResults, f.NextToken, func(p *ScalingPolicy) string {
		return p.ARN
	})
}
