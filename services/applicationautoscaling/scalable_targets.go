package applicationautoscaling

import (
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// Per-account/per-region scalable-target quotas, keyed by ServiceNamespace.
// Source: AWS "Quotas for Application Auto Scaling" documentation. All are
// adjustable via Service Quotas, but these are the real documented defaults;
// gopherstack has no per-account quota-override concept (same rationale as
// maxStepAdjustmentsPerPolicy).
const (
	// maxScalableTargetsDynamoDB is the default quota for the dynamodb namespace.
	maxScalableTargetsDynamoDB = 5000
	// maxScalableTargetsECS is the default quota for the ecs namespace.
	maxScalableTargetsECS = 3000
	// maxScalableTargetsCassandra is the default quota for the cassandra
	// (Amazon Keyspaces) namespace.
	maxScalableTargetsCassandra = 1500
	// maxScalableTargetsDefault is the default quota for every other
	// ServiceNamespace (ec2, elasticmapreduce, appstream, comprehend, lambda,
	// sagemaker, custom-resource, neptune, kafka, workspaces, rds, ...).
	maxScalableTargetsDefault = 500
)

// maxScalableTargetsForNamespace returns the real, documented default
// per-account/per-region scalable-target quota for the given ServiceNamespace.
func maxScalableTargetsForNamespace(serviceNamespace string) int {
	switch serviceNamespace {
	case "dynamodb":
		return maxScalableTargetsDynamoDB
	case "ecs":
		return maxScalableTargetsECS
	case "cassandra":
		return maxScalableTargetsCassandra
	default:
		return maxScalableTargetsDefault
	}
}

// scalableTargetsForNamespaceLocked counts the scalable targets currently
// registered under the given ServiceNamespace. Caller must hold the lock.
func (b *InMemoryBackend) scalableTargetsForNamespaceLocked(serviceNamespace string) int {
	count := 0

	for _, t := range b.scalableTargets.All() {
		if t.ServiceNamespace == serviceNamespace {
			count++
		}
	}

	return count
}

// RegisterScalableTarget upserts a scalable target (creates or updates).
// minCapacity and maxCapacity are *int32, not int32: real AWS's
// RegisterScalableTargetInput models both as optional pointers, required only
// "when registering a new scalable target" (api_op_RegisterScalableTarget.go),
// and the operation doc states "Any parameters that you don't specify are not
// changed by this update request." A plain int32 could not distinguish
// "caller omitted MinCapacity" from "caller explicitly set MinCapacity to 0"
// (0 is a documented valid capacity for several namespaces), so omitting it on
// an update would silently reset capacity to 0 -- see updateExistingTarget.
func (b *InMemoryBackend) RegisterScalableTarget(
	serviceNamespace, resourceID, scalableDimension string,
	minCapacity, maxCapacity *int32,
	tags map[string]string,
	roleARN string,
	suspendedState *SuspendedState,
) (*ScalableTarget, error) {
	if serviceNamespace == "" {
		return nil, fmt.Errorf("%w: ServiceNamespace is required", ErrValidation)
	}

	if resourceID == "" {
		return nil, fmt.Errorf("%w: ResourceId is required", ErrValidation)
	}

	if scalableDimension == "" {
		return nil, fmt.Errorf("%w: ScalableDimension is required", ErrValidation)
	}

	// RegisterScalableTarget's modeled error set has LimitExceededException
	// but no TooManyTagsException (that's only modeled on TagResource -- see
	// ErrTooManyTags's doc comment), so an over-limit tag count here is
	// reported as LimitExceededException.
	if len(tags) > maxTagsPerResource {
		return nil, fmt.Errorf(
			"%w: too many tags; maximum allowed is %d",
			ErrLimitExceeded,
			maxTagsPerResource,
		)
	}

	b.mu.Lock("RegisterScalableTarget")
	defer b.mu.Unlock()

	key := scalableTargetKey(serviceNamespace, resourceID, scalableDimension)
	now := time.Now().UTC()

	if existing, ok := b.scalableTargets.Get(key); ok {
		return b.updateExistingTarget(existing, minCapacity, maxCapacity, tags, roleARN, suspendedState, now)
	}

	// MinCapacity/MaxCapacity are "required when registering a new scalable
	// target" (api_op_RegisterScalableTarget.go field docs); only optional on
	// an update to an existing target, handled above.
	if minCapacity == nil {
		return nil, fmt.Errorf("%w: MinCapacity is required when registering a new scalable target", ErrValidation)
	}

	if maxCapacity == nil {
		return nil, fmt.Errorf("%w: MaxCapacity is required when registering a new scalable target", ErrValidation)
	}

	if *minCapacity > *maxCapacity {
		return nil, fmt.Errorf(
			"%w: MinCapacity (%d) must not exceed MaxCapacity (%d)",
			ErrValidation,
			*minCapacity,
			*maxCapacity,
		)
	}

	limit := maxScalableTargetsForNamespace(serviceNamespace)
	if b.scalableTargetsForNamespaceLocked(serviceNamespace) >= limit {
		return nil, fmt.Errorf(
			"%w: scalable target quota exceeded for service namespace %s; maximum allowed is %d",
			ErrLimitExceeded,
			serviceNamespace,
			limit,
		)
	}

	t := &ScalableTarget{
		ServiceNamespace:  serviceNamespace,
		ResourceID:        resourceID,
		ScalableDimension: scalableDimension,
		MinCapacity:       *minCapacity,
		MaxCapacity:       *maxCapacity,
		ARN: arn.Build(
			"application-autoscaling",
			b.region,
			b.accountID,
			"scalable-target/"+uuid.NewString(),
		),
		RoleARN:          roleARN,
		AccountID:        b.accountID,
		Region:           b.region,
		Tags:             maps.Clone(tags),
		SuspendedState:   suspendedState,
		CreationTime:     now,
		LastModifiedTime: now,
	}
	if t.Tags == nil {
		t.Tags = make(map[string]string)
	}

	b.scalableTargets.Put(t)

	b.recordActivityLocked(
		serviceNamespace, resourceID, scalableDimension,
		"Setting min/max capacity",
		"target registered via RegisterScalableTarget",
		now,
	)

	cp := *t
	cp.Tags = maps.Clone(t.Tags)

	return &cp, nil
}

// recordActivityLocked appends a completed scaling activity. The caller must
// hold the write lock.
func (b *InMemoryBackend) recordActivityLocked(
	serviceNamespace, resourceID, scalableDimension, description, cause string,
	when time.Time,
) {
	b.scalingActivities = append(b.scalingActivities, &ScalingActivity{
		ActivityID:        uuid.NewString(),
		ServiceNamespace:  serviceNamespace,
		ResourceID:        resourceID,
		ScalableDimension: scalableDimension,
		Description:       description,
		Cause:             cause,
		StartTime:         when,
		EndTime:           when,
		StatusCode:        "Successful",
		StatusMessage:     "Successfully set desired capacity.",
	})
}

// updateExistingTarget updates an existing scalable target in-place.
// Caller must hold the write lock.
func (b *InMemoryBackend) updateExistingTarget(
	existing *ScalableTarget,
	minCapacity, maxCapacity *int32,
	tags map[string]string,
	roleARN string,
	suspendedState *SuspendedState,
	now time.Time,
) (*ScalableTarget, error) {
	newMin := existing.MinCapacity
	if minCapacity != nil {
		newMin = *minCapacity
	}

	newMax := existing.MaxCapacity
	if maxCapacity != nil {
		newMax = *maxCapacity
	}

	if newMin > newMax {
		return nil, fmt.Errorf(
			"%w: MinCapacity (%d) must not exceed MaxCapacity (%d)",
			ErrValidation,
			newMin,
			newMax,
		)
	}

	existing.MinCapacity = newMin
	existing.MaxCapacity = newMax
	existing.LastModifiedTime = now

	if roleARN != "" {
		existing.RoleARN = roleARN
	}

	if suspendedState != nil {
		existing.SuspendedState = suspendedState
	}

	if len(tags) > 0 {
		if existing.Tags == nil {
			existing.Tags = make(map[string]string)
		}

		if err := mergeTags(existing.Tags, tags, ErrLimitExceeded); err != nil {
			return nil, err
		}
	}

	b.recordActivityLocked(
		existing.ServiceNamespace, existing.ResourceID, existing.ScalableDimension,
		"Setting min/max capacity",
		"target updated via RegisterScalableTarget",
		now,
	)

	cp := *existing
	cp.Tags = maps.Clone(existing.Tags)

	return &cp, nil
}

// DeregisterScalableTarget removes a scalable target.
// DeregisterScalableTarget removes a scalable target and cascades the deletion
// to all scaling policies and scheduled actions that belong to the same
// resource (AWS behaviour).
func (b *InMemoryBackend) DeregisterScalableTarget(serviceNamespace, resourceID, scalableDimension string) error {
	if serviceNamespace == "" {
		return fmt.Errorf("%w: ServiceNamespace is required", ErrValidation)
	}

	if resourceID == "" {
		return fmt.Errorf("%w: ResourceId is required", ErrValidation)
	}

	if scalableDimension == "" {
		return fmt.Errorf("%w: ScalableDimension is required", ErrValidation)
	}

	b.mu.Lock("DeregisterScalableTarget")
	defer b.mu.Unlock()

	key := scalableTargetKey(serviceNamespace, resourceID, scalableDimension)

	if !b.scalableTargets.Has(key) {
		return fmt.Errorf("%w: scalable target %s not found", ErrNotFound, key)
	}

	b.scalableTargets.Delete(key)

	// Cascade: remove all scaling policies that belong to this target.
	// b.scalingPolicies.All() returns a fresh slice, so deleting by key while
	// ranging over it is safe (unlike ranging directly over the table's
	// internal map).
	for _, p := range b.scalingPolicies.All() {
		if p.ServiceNamespace == serviceNamespace && p.ResourceID == resourceID &&
			p.ScalableDimension == scalableDimension {
			b.scalingPolicies.Delete(p.ARN)
		}
	}

	// Cascade: remove all scheduled actions that belong to this target.
	for _, a := range b.scheduledActions.All() {
		if a.ServiceNamespace == serviceNamespace && a.ResourceID == resourceID &&
			a.ScalableDimension == scalableDimension {
			b.scheduledActions.Delete(a.ARN)
		}
	}

	return nil
}

// DescribeScalableTargetsFilter carries optional filters for DescribeScalableTargets.
type DescribeScalableTargetsFilter struct {
	ServiceNamespace  string
	ScalableDimension string
	// NextToken is the opaque pagination cursor returned by a prior call.
	NextToken   string
	ResourceIDs []string
	// MaxResults, when > 0, limits the number of returned items. Capped at maxDescribeResults.
	MaxResults int32
}

// DescribeScalableTargets lists scalable targets, optionally filtered, and
// returns the NextToken for the following page (empty on the last page).
// Returns ErrInvalidNextToken if f.NextToken fails to decode.
func (b *InMemoryBackend) DescribeScalableTargets(f DescribeScalableTargetsFilter) ([]*ScalableTarget, string, error) {
	b.mu.RLock("DescribeScalableTargets")
	defer b.mu.RUnlock()

	var idSet map[string]bool
	if len(f.ResourceIDs) > 0 {
		idSet = make(map[string]bool, len(f.ResourceIDs))
		for _, id := range f.ResourceIDs {
			idSet[id] = true
		}
	}

	list := make([]*ScalableTarget, 0, b.scalableTargets.Len())
	for _, t := range b.scalableTargets.All() {
		if f.ServiceNamespace != "" && t.ServiceNamespace != f.ServiceNamespace {
			continue
		}

		if idSet != nil && !idSet[t.ResourceID] {
			continue
		}

		if f.ScalableDimension != "" && t.ScalableDimension != f.ScalableDimension {
			continue
		}

		cp := *t
		cp.Tags = maps.Clone(t.Tags)

		list = append(list, &cp)
	}

	return paginate(list, f.MaxResults, f.NextToken, func(t *ScalableTarget) string {
		return t.ResourceID + "|" + t.ScalableDimension
	})
}

// scalableTargetExists reports whether a scalable target is registered for
// the given (serviceNamespace,resourceId,scalableDimension) key. Caller must
// hold at least a read lock.
func (b *InMemoryBackend) scalableTargetExists(serviceNamespace, resourceID, scalableDimension string) bool {
	return b.scalableTargets.Has(scalableTargetKey(serviceNamespace, resourceID, scalableDimension))
}
