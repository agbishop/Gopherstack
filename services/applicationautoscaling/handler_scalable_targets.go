package applicationautoscaling

import (
	"context"
)

// --- Input/Output types ---

type suspendedStateInput struct {
	DynamicScalingInSuspended  bool `json:"DynamicScalingInSuspended"`
	DynamicScalingOutSuspended bool `json:"DynamicScalingOutSuspended"`
	ScheduledScalingSuspended  bool `json:"ScheduledScalingSuspended"`
}

type registerScalableTargetInput struct {
	SuspendedState    *suspendedStateInput `json:"SuspendedState,omitempty"`
	MinCapacity       *int32               `json:"MinCapacity,omitempty"`
	MaxCapacity       *int32               `json:"MaxCapacity,omitempty"`
	Tags              map[string]string    `json:"Tags,omitempty"`
	ServiceNamespace  string               `json:"ServiceNamespace"`
	ResourceID        string               `json:"ResourceId"`
	ScalableDimension string               `json:"ScalableDimension"`
	RoleARN           string               `json:"RoleARN,omitempty"`
}

type registerScalableTargetOutput struct {
	ScalableTargetARN string `json:"ScalableTargetARN"`
}

func (h *Handler) handleRegisterScalableTarget(
	_ context.Context,
	in *registerScalableTargetInput,
) (*registerScalableTargetOutput, error) {
	var ss *SuspendedState
	if in.SuspendedState != nil {
		ss = &SuspendedState{
			DynamicScalingInSuspended:  in.SuspendedState.DynamicScalingInSuspended,
			DynamicScalingOutSuspended: in.SuspendedState.DynamicScalingOutSuspended,
			ScheduledScalingSuspended:  in.SuspendedState.ScheduledScalingSuspended,
		}
	}

	t, err := h.Backend.RegisterScalableTarget(
		in.ServiceNamespace, in.ResourceID, in.ScalableDimension,
		in.MinCapacity, in.MaxCapacity,
		in.Tags, in.RoleARN, ss,
	)
	if err != nil {
		return nil, err
	}

	return &registerScalableTargetOutput{ScalableTargetARN: t.ARN}, nil
}

type deregisterScalableTargetInput struct {
	ServiceNamespace  string `json:"ServiceNamespace"`
	ResourceID        string `json:"ResourceId"`
	ScalableDimension string `json:"ScalableDimension"`
}

type deregisterScalableTargetOutput struct{}

func (h *Handler) handleDeregisterScalableTarget(
	_ context.Context,
	in *deregisterScalableTargetInput,
) (*deregisterScalableTargetOutput, error) {
	if err := h.Backend.DeregisterScalableTarget(in.ServiceNamespace, in.ResourceID, in.ScalableDimension); err != nil {
		return nil, err
	}

	return &deregisterScalableTargetOutput{}, nil
}

type describeScalableTargetsInput struct {
	ServiceNamespace  string   `json:"ServiceNamespace"`
	ScalableDimension string   `json:"ScalableDimension,omitempty"`
	NextToken         string   `json:"NextToken,omitempty"`
	ResourceIDs       []string `json:"ResourceIds,omitempty"`
	MaxResults        int32    `json:"MaxResults,omitempty"`
}

type suspendedStateSummary struct {
	DynamicScalingInSuspended  bool `json:"DynamicScalingInSuspended"`
	DynamicScalingOutSuspended bool `json:"DynamicScalingOutSuspended"`
	ScheduledScalingSuspended  bool `json:"ScheduledScalingSuspended"`
}

type scalableTargetSummary struct {
	SuspendedState    *suspendedStateSummary `json:"SuspendedState,omitempty"`
	Tags              map[string]string      `json:"Tags,omitempty"`
	CreationTime      *float64               `json:"CreationTime,omitempty"`
	LastModifiedTime  *float64               `json:"LastModifiedTime,omitempty"`
	PredictedCapacity *int32                 `json:"PredictedCapacity,omitempty"`
	ServiceNamespace  string                 `json:"ServiceNamespace"`
	ResourceID        string                 `json:"ResourceId"`
	ScalableDimension string                 `json:"ScalableDimension"`
	ScalableTargetARN string                 `json:"ScalableTargetARN"`
	RoleARN           string                 `json:"RoleARN,omitempty"`
	MinCapacity       int32                  `json:"MinCapacity"`
	MaxCapacity       int32                  `json:"MaxCapacity"`
}

type describeScalableTargetsOutput struct {
	NextToken       string                  `json:"NextToken,omitempty"`
	ScalableTargets []scalableTargetSummary `json:"ScalableTargets"`
}

func (h *Handler) handleDescribeScalableTargets(
	_ context.Context,
	in *describeScalableTargetsInput,
) (*describeScalableTargetsOutput, error) {
	targets, nextToken, err := h.Backend.DescribeScalableTargets(DescribeScalableTargetsFilter{
		ServiceNamespace:  in.ServiceNamespace,
		ResourceIDs:       in.ResourceIDs,
		ScalableDimension: in.ScalableDimension,
		MaxResults:        in.MaxResults,
		NextToken:         in.NextToken,
	})
	if err != nil {
		return nil, err
	}

	items := make([]scalableTargetSummary, 0, len(targets))
	for _, t := range targets {
		item := scalableTargetSummary{
			ServiceNamespace:  t.ServiceNamespace,
			ResourceID:        t.ResourceID,
			ScalableDimension: t.ScalableDimension,
			MinCapacity:       t.MinCapacity,
			MaxCapacity:       t.MaxCapacity,
			ScalableTargetARN: t.ARN,
			RoleARN:           t.RoleARN,
			Tags:              t.Tags,
			CreationTime:      epochSecondsPtr(t.CreationTime),
			LastModifiedTime:  epochSecondsPtr(t.LastModifiedTime),
			PredictedCapacity: t.PredictedCapacity,
		}
		if t.SuspendedState != nil {
			item.SuspendedState = &suspendedStateSummary{
				DynamicScalingInSuspended:  t.SuspendedState.DynamicScalingInSuspended,
				DynamicScalingOutSuspended: t.SuspendedState.DynamicScalingOutSuspended,
				ScheduledScalingSuspended:  t.SuspendedState.ScheduledScalingSuspended,
			}
		}

		items = append(items, item)
	}

	return &describeScalableTargetsOutput{ScalableTargets: items, NextToken: nextToken}, nil
}
