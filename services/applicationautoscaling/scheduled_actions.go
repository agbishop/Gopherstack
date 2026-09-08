package applicationautoscaling

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// maxScheduledActionsPerTarget is the real, non-adjustable AWS quota on the
// number of scheduled actions per scalable target. Confirmed against the
// "Quotas for Application Auto Scaling" AWS documentation page.
const maxScheduledActionsPerTarget = 200

// PutScheduledAction upserts a scheduled action.
func (b *InMemoryBackend) PutScheduledAction(
	serviceNamespace, resourceID, scalableDimension, scheduledActionName, schedule, timezone string,
	startTime, endTime *time.Time,
	scalableTargetAction *ScalableTargetAction,
) (*ScheduledAction, error) {
	if serviceNamespace == "" {
		return nil, fmt.Errorf("%w: ServiceNamespace is required", ErrValidation)
	}

	if resourceID == "" {
		return nil, fmt.Errorf("%w: ResourceId is required", ErrValidation)
	}

	if scalableDimension == "" {
		return nil, fmt.Errorf("%w: ScalableDimension is required", ErrValidation)
	}

	if scheduledActionName == "" {
		return nil, fmt.Errorf("%w: ScheduledActionName is required", ErrValidation)
	}

	b.mu.Lock("PutScheduledAction")
	defer b.mu.Unlock()

	// PutScheduledAction's modeled error set includes ObjectNotFoundException:
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
	key := actionNameKey(serviceNamespace, resourceID, scalableDimension, scheduledActionName)

	if group := b.actionsByName.Get(key); len(group) > 0 {
		a := group[0]
		if schedule != "" {
			a.Schedule = schedule
		}

		a.LastModifiedTime = now
		if scalableTargetAction != nil {
			a.ScalableTargetAction = scalableTargetAction
		}

		// "To update a scheduled action, specify the parameters that you
		// want to change. If you don't specify start and end times, the old
		// values are deleted." (api_op_PutScheduledAction.go doc) -- unlike
		// Schedule/Timezone/ScalableTargetAction, start/end are always
		// overwritten with whatever the caller sent, nil included.
		a.StartTime = startTime
		a.EndTime = endTime

		if timezone != "" {
			a.Timezone = timezone
		}

		cp := *a

		return &cp, nil
	}

	// Schedule is not marked "This member is required" on
	// PutScheduledActionInput -- only ResourceId/ScalableDimension/
	// ScheduledActionName/ServiceNamespace are -- and the operation doc's
	// "specify the parameters that you want to change" confirms it's
	// optional on update (handled above). It has no meaning for a brand-new
	// action, though, so it's required here.
	if schedule == "" {
		return nil, fmt.Errorf("%w: Schedule is required", ErrValidation)
	}

	// A brand-new scheduled action counts against the per-target quota;
	// updates to an existing one (handled above) do not.
	actionCount := b.scheduledActionsForTargetLocked(serviceNamespace, resourceID, scalableDimension)
	if actionCount >= maxScheduledActionsPerTarget {
		return nil, fmt.Errorf(
			"%w: maximum number of scheduled actions (%d) per scalable target exceeded",
			ErrLimitExceeded,
			maxScheduledActionsPerTarget,
		)
	}

	// Real AWS scheduled-action ARNs separate the scheduledActionName segment
	// from the resource/namespace/resourceId segment with a colon, not a
	// slash: scheduledAction:{uuid}:resource/{namespace}/{resourceId}:scheduledActionName/{name}.
	actionARN := arn.Build("autoscaling", b.region, b.accountID,
		fmt.Sprintf("scheduledAction:%s:resource/%s/%s:scheduledActionName/%s",
			uuid.NewString(), serviceNamespace, resourceID, scheduledActionName))
	a := &ScheduledAction{
		ServiceNamespace:     serviceNamespace,
		ResourceID:           resourceID,
		ScalableDimension:    scalableDimension,
		ScheduledActionName:  scheduledActionName,
		Schedule:             schedule,
		ScalableTargetAction: scalableTargetAction,
		StartTime:            startTime,
		EndTime:              endTime,
		Timezone:             timezone,
		ARN:                  actionARN,
		CreationTime:         now,
		LastModifiedTime:     now,
	}
	b.scheduledActions.Put(a)
	cp := *a

	return &cp, nil
}

// scheduledActionsForTargetLocked counts the scheduled actions registered
// against the given scalable target. Caller must hold the write lock.
func (b *InMemoryBackend) scheduledActionsForTargetLocked(serviceNamespace, resourceID, scalableDimension string) int {
	count := 0

	for _, a := range b.scheduledActions.All() {
		if a.ServiceNamespace == serviceNamespace && a.ResourceID == resourceID &&
			a.ScalableDimension == scalableDimension {
			count++
		}
	}

	return count
}

// DeleteScheduledAction removes a scheduled action.
func (b *InMemoryBackend) DeleteScheduledAction(
	serviceNamespace, resourceID, scalableDimension, scheduledActionName string,
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

	if scheduledActionName == "" {
		return fmt.Errorf("%w: ScheduledActionName is required", ErrValidation)
	}

	b.mu.Lock("DeleteScheduledAction")
	defer b.mu.Unlock()

	key := actionNameKey(serviceNamespace, resourceID, scalableDimension, scheduledActionName)

	group := b.actionsByName.Get(key)
	if len(group) == 0 {
		return fmt.Errorf("%w: scheduled action %s not found", ErrNotFound, scheduledActionName)
	}

	b.scheduledActions.Delete(group[0].ARN)

	return nil
}

// DescribeScheduledActionsFilter carries optional filters for DescribeScheduledActions.
type DescribeScheduledActionsFilter struct {
	// ServiceNamespace limits results to this namespace when non-empty.
	ServiceNamespace string
	// ResourceID limits results to this resource when non-empty.
	ResourceID string
	// ScalableDimension limits results to this dimension when non-empty.
	ScalableDimension string
	// NextToken is the opaque pagination cursor returned by a prior call.
	NextToken string
	// ScheduledActionNames, when non-empty, limits results to the named actions.
	ScheduledActionNames []string
	// MaxResults, when > 0, limits the number of returned items.
	MaxResults int32
}

// DescribeScheduledActions lists scheduled actions, optionally filtered, and
// returns the NextToken for the following page (empty on the last page).
// Returns ErrInvalidNextToken if f.NextToken fails to decode.
func (b *InMemoryBackend) DescribeScheduledActions(
	f DescribeScheduledActionsFilter,
) ([]*ScheduledAction, string, error) {
	b.mu.RLock("DescribeScheduledActions")
	defer b.mu.RUnlock()

	var nameSet map[string]bool
	if len(f.ScheduledActionNames) > 0 {
		nameSet = make(map[string]bool, len(f.ScheduledActionNames))
		for _, n := range f.ScheduledActionNames {
			nameSet[n] = true
		}
	}

	list := make([]*ScheduledAction, 0, b.scheduledActions.Len())
	for _, a := range b.scheduledActions.All() {
		if f.ServiceNamespace != "" && a.ServiceNamespace != f.ServiceNamespace {
			continue
		}

		if f.ResourceID != "" && a.ResourceID != f.ResourceID {
			continue
		}

		if f.ScalableDimension != "" && a.ScalableDimension != f.ScalableDimension {
			continue
		}

		if nameSet != nil && !nameSet[a.ScheduledActionName] {
			continue
		}

		cp := *a
		list = append(list, &cp)
	}

	return paginate(list, f.MaxResults, f.NextToken, func(a *ScheduledAction) string {
		return a.ServiceNamespace + "|" + a.ResourceID + "|" + a.ScalableDimension + "|" + a.ScheduledActionName
	})
}
