package scheduler

import "context"

// Schedule group handlers.

type createScheduleGroupInput struct {
	Name        string        `json:"Name"`
	ClientToken string        `json:"ClientToken,omitempty"`
	Tags        []resourceTag `json:"Tags"`
}

type createScheduleGroupOutput struct {
	ScheduleGroupArn string `json:"ScheduleGroupArn"`
}

func (h *Handler) handleCreateScheduleGroup(
	ctx context.Context,
	in *createScheduleGroupInput,
) (*createScheduleGroupOutput, error) {
	tokenKey := clientTokenKey("group", "", in.Name, in.ClientToken)
	if arn, ok := h.lookupIdempotent(tokenKey); ok {
		return &createScheduleGroupOutput{ScheduleGroupArn: arn}, nil
	}

	g, err := h.Backend.CreateScheduleGroup(ctx, in.Name, tagsFromWire(in.Tags))
	if err != nil {
		return nil, err
	}

	h.storeIdempotent(tokenKey, g.ARN)

	return &createScheduleGroupOutput{ScheduleGroupArn: g.ARN}, nil
}

type scheduleGroupNameInput struct {
	Name string `json:"Name"`
}

type deleteScheduleGroupOutput struct{}

func (h *Handler) handleDeleteScheduleGroup(
	ctx context.Context,
	in *scheduleGroupNameInput,
) (*deleteScheduleGroupOutput, error) {
	if err := h.Backend.DeleteScheduleGroup(ctx, in.Name); err != nil {
		return nil, err
	}

	return &deleteScheduleGroupOutput{}, nil
}

// getScheduleGroupOutput mirrors real AWS's GetScheduleGroupOutput, which has no
// Tags or Description field (schedule group tags are only ever fetched via
// ListTagsForResource; GetScheduleGroupOutput carries no Description member at all).
type getScheduleGroupOutput struct {
	Arn                  string  `json:"Arn"`
	Name                 string  `json:"Name"`
	State                string  `json:"State"`
	CreationDate         float64 `json:"CreationDate"`
	LastModificationDate float64 `json:"LastModificationDate"`
}

func (h *Handler) handleGetScheduleGroup(
	ctx context.Context,
	in *scheduleGroupNameInput,
) (*getScheduleGroupOutput, error) {
	g, err := h.Backend.GetScheduleGroup(ctx, in.Name)
	if err != nil {
		return nil, err
	}

	return &getScheduleGroupOutput{
		Arn:                  g.ARN,
		CreationDate:         float64(g.CreationDate.Unix()),
		LastModificationDate: float64(g.LastModificationDate.Unix()),
		Name:                 g.Name,
		State:                g.State,
	}, nil
}

type listScheduleGroupsInput struct {
	NamePrefix string `json:"NamePrefix"`
	NextToken  string `json:"NextToken"`
	MaxResults string `json:"MaxResults"`
}

// scheduleGroupSummary mirrors real AWS's ScheduleGroupSummary, which has no Tags
// field (schedule group tags are only ever fetched via ListTagsForResource).
type scheduleGroupSummary struct {
	Arn                  string  `json:"Arn"`
	Name                 string  `json:"Name"`
	State                string  `json:"State"`
	CreationDate         float64 `json:"CreationDate"`
	LastModificationDate float64 `json:"LastModificationDate"`
}

type listScheduleGroupsOutput struct {
	NextToken      string                 `json:"NextToken,omitempty"`
	ScheduleGroups []scheduleGroupSummary `json:"ScheduleGroups"`
}

func (h *Handler) handleListScheduleGroups(
	ctx context.Context,
	in *listScheduleGroupsInput,
) (*listScheduleGroupsOutput, error) {
	maxResults := parseMaxResults(in.MaxResults)
	groups, nextToken := h.Backend.ListScheduleGroups(ctx, in.NamePrefix, in.NextToken, maxResults)
	items := make([]scheduleGroupSummary, 0, len(groups))

	for _, g := range groups {
		items = append(items, scheduleGroupSummary{
			Arn:                  g.ARN,
			CreationDate:         float64(g.CreationDate.Unix()),
			LastModificationDate: float64(g.LastModificationDate.Unix()),
			Name:                 g.Name,
			State:                g.State,
		})
	}

	return &listScheduleGroupsOutput{ScheduleGroups: items, NextToken: nextToken}, nil
}
