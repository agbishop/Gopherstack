package codedeploy

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
)

// instanceSummaryEntry is the (deprecated but still-served) wire format for
// GetDeploymentInstance/BatchGetDeploymentInstances.
type instanceSummaryEntry struct {
	DeploymentID  string  `json:"deploymentId,omitempty"`
	InstanceID    string  `json:"instanceId,omitempty"`
	InstanceType  string  `json:"instanceType,omitempty"`
	Status        string  `json:"status,omitempty"`
	LastUpdatedAt float64 `json:"lastUpdatedAt,omitempty"`
}

// instanceTargetEntry, ecsTargetEntry, and lambdaTargetEntry are the wire
// formats for the three DeploymentTarget union members this backend can
// resolve (there is no CloudFormationTarget concept here, since this backend
// has no CloudFormation blue/green integration).
type instanceTargetEntry struct {
	DeploymentID  string  `json:"deploymentId,omitempty"`
	TargetID      string  `json:"targetId,omitempty"`
	TargetArn     string  `json:"targetArn,omitempty"`
	Status        string  `json:"status,omitempty"`
	InstanceLabel string  `json:"instanceLabel,omitempty"`
	LastUpdatedAt float64 `json:"lastUpdatedAt,omitempty"`
}

type ecsTargetEntry struct {
	DeploymentID  string  `json:"deploymentId,omitempty"`
	TargetID      string  `json:"targetId,omitempty"`
	TargetArn     string  `json:"targetArn,omitempty"`
	Status        string  `json:"status,omitempty"`
	LastUpdatedAt float64 `json:"lastUpdatedAt,omitempty"`
}

type lambdaTargetEntry struct {
	DeploymentID  string  `json:"deploymentId,omitempty"`
	TargetID      string  `json:"targetId,omitempty"`
	TargetArn     string  `json:"targetArn,omitempty"`
	Status        string  `json:"status,omitempty"`
	LastUpdatedAt float64 `json:"lastUpdatedAt,omitempty"`
}

// deploymentTargetEntry is the wire format for the DeploymentTarget union:
// exactly one of InstanceTarget/EcsTarget/LambdaTarget is populated, selected
// by DeploymentTargetType.
type deploymentTargetEntry struct {
	InstanceTarget       *instanceTargetEntry `json:"instanceTarget,omitempty"`
	EcsTarget            *ecsTargetEntry      `json:"ecsTarget,omitempty"`
	LambdaTarget         *lambdaTargetEntry   `json:"lambdaTarget,omitempty"`
	DeploymentTargetType string               `json:"deploymentTargetType,omitempty"`
}

// instanceSummaryFromRecord converts a computed DeploymentTargetRecord to the
// legacy InstanceSummary wire format used by GetDeploymentInstance and
// BatchGetDeploymentInstances.
func instanceSummaryFromRecord(t *DeploymentTargetRecord) instanceSummaryEntry {
	return instanceSummaryEntry{
		DeploymentID:  t.DeploymentID,
		InstanceID:    t.TargetID,
		InstanceType:  t.InstanceLabel,
		Status:        t.Status,
		LastUpdatedAt: awstime.Epoch(t.LastUpdatedAt),
	}
}

// targetEntryFromRecord converts a computed DeploymentTargetRecord to its
// DeploymentTarget union wire representation, selecting the member and
// deploymentTargetType enum value that match t.TargetType.
func targetEntryFromRecord(t *DeploymentTargetRecord) deploymentTargetEntry {
	switch t.TargetType {
	case "ecsTarget":
		return deploymentTargetEntry{
			DeploymentTargetType: "ECSTarget",
			EcsTarget: &ecsTargetEntry{
				DeploymentID:  t.DeploymentID,
				TargetID:      t.TargetID,
				TargetArn:     t.TargetArn,
				Status:        t.Status,
				LastUpdatedAt: awstime.Epoch(t.LastUpdatedAt),
			},
		}
	case "lambdaTarget":
		return deploymentTargetEntry{
			DeploymentTargetType: "LambdaTarget",
			LambdaTarget: &lambdaTargetEntry{
				DeploymentID:  t.DeploymentID,
				TargetID:      t.TargetID,
				TargetArn:     t.TargetArn,
				Status:        t.Status,
				LastUpdatedAt: awstime.Epoch(t.LastUpdatedAt),
			},
		}
	default: // instanceTarget
		return deploymentTargetEntry{
			DeploymentTargetType: "InstanceTarget",
			InstanceTarget: &instanceTargetEntry{
				DeploymentID:  t.DeploymentID,
				TargetID:      t.TargetID,
				TargetArn:     t.TargetArn,
				Status:        t.Status,
				InstanceLabel: t.InstanceLabel,
				LastUpdatedAt: awstime.Epoch(t.LastUpdatedAt),
			},
		}
	}
}

type batchGetDeploymentInstancesInput struct {
	DeploymentID string   `json:"deploymentId"`
	InstanceIDs  []string `json:"instanceIds"`
}

type batchGetDeploymentInstancesOutput struct {
	ErrorMessage     string                 `json:"errorMessage,omitempty"`
	InstancesSummary []instanceSummaryEntry `json:"instancesSummary"`
}

func (h *Handler) handleBatchGetDeploymentInstances(
	_ context.Context,
	in *batchGetDeploymentInstancesInput,
) (*batchGetDeploymentInstancesOutput, error) {
	if in.DeploymentID == "" {
		return nil, fmt.Errorf("%w: deploymentId is required", ErrDeploymentIDRequired)
	}

	items, err := h.Backend.BatchGetDeploymentInstances(in.DeploymentID, in.InstanceIDs)
	if err != nil {
		return nil, err
	}

	summaries := make([]instanceSummaryEntry, 0, len(items))
	for _, item := range items {
		summaries = append(summaries, instanceSummaryFromRecord(item))
	}

	return &batchGetDeploymentInstancesOutput{InstancesSummary: summaries}, nil
}

type batchGetDeploymentTargetsInput struct {
	DeploymentID string   `json:"deploymentId"`
	TargetIDs    []string `json:"targetIds"`
}

type batchGetDeploymentTargetsOutput struct {
	DeploymentTargets []deploymentTargetEntry `json:"deploymentTargets"`
}

func (h *Handler) handleBatchGetDeploymentTargets(
	_ context.Context,
	in *batchGetDeploymentTargetsInput,
) (*batchGetDeploymentTargetsOutput, error) {
	if in.DeploymentID == "" {
		return nil, fmt.Errorf("%w: deploymentId is required", ErrDeploymentIDRequired)
	}

	items, err := h.Backend.BatchGetDeploymentTargets(in.DeploymentID, in.TargetIDs)
	if err != nil {
		return nil, err
	}

	targets := make([]deploymentTargetEntry, 0, len(items))
	for _, item := range items {
		targets = append(targets, targetEntryFromRecord(item))
	}

	return &batchGetDeploymentTargetsOutput{DeploymentTargets: targets}, nil
}

type getDeploymentInstanceInput struct {
	DeploymentID string `json:"deploymentId"`
	InstanceID   string `json:"instanceId"`
}

type getDeploymentInstanceOutput struct {
	InstanceSummary instanceSummaryEntry `json:"instanceSummary"`
}

func (h *Handler) handleGetDeploymentInstance(
	_ context.Context,
	in *getDeploymentInstanceInput,
) (*getDeploymentInstanceOutput, error) {
	if in.DeploymentID == "" {
		return nil, fmt.Errorf("%w: deploymentId is required", ErrDeploymentIDRequired)
	}

	if in.InstanceID == "" {
		return nil, fmt.Errorf("%w: instanceId is required", ErrInstanceIDRequired)
	}

	t, err := h.Backend.GetDeploymentInstance(in.DeploymentID, in.InstanceID)
	if err != nil {
		return nil, err
	}

	return &getDeploymentInstanceOutput{InstanceSummary: instanceSummaryFromRecord(t)}, nil
}

type getDeploymentTargetInput struct {
	DeploymentID string `json:"deploymentId"`
	TargetID     string `json:"targetId"`
}

type getDeploymentTargetOutput struct {
	DeploymentTarget deploymentTargetEntry `json:"deploymentTarget"`
}

func (h *Handler) handleGetDeploymentTarget(
	_ context.Context,
	in *getDeploymentTargetInput,
) (*getDeploymentTargetOutput, error) {
	if in.DeploymentID == "" {
		return nil, fmt.Errorf("%w: deploymentId is required", ErrDeploymentIDRequired)
	}

	if in.TargetID == "" {
		return nil, fmt.Errorf("%w: targetId is required", ErrDeploymentTargetIDRequired)
	}

	t, err := h.Backend.GetDeploymentTarget(in.DeploymentID, in.TargetID)
	if err != nil {
		return nil, err
	}

	return &getDeploymentTargetOutput{DeploymentTarget: targetEntryFromRecord(t)}, nil
}

type listDeploymentInstancesInput struct {
	DeploymentID         string   `json:"deploymentId"`
	InstanceStatusFilter []string `json:"instanceStatusFilter"`
	InstanceTypeFilter   []string `json:"instanceTypeFilter"`
}

type listDeploymentInstancesOutput struct {
	InstancesList []string `json:"instancesList"`
}

func (h *Handler) handleListDeploymentInstances(
	_ context.Context,
	in *listDeploymentInstancesInput,
) (*listDeploymentInstancesOutput, error) {
	if in.DeploymentID == "" {
		return nil, fmt.Errorf("%w: deploymentId is required", ErrDeploymentIDRequired)
	}

	ids, err := h.Backend.ListDeploymentInstances(in.DeploymentID, InstanceListFilter{
		StatusFilter: in.InstanceStatusFilter,
		TypeFilter:   in.InstanceTypeFilter,
	})
	if err != nil {
		return nil, err
	}

	return &listDeploymentInstancesOutput{InstancesList: ids}, nil
}

type listDeploymentTargetsInput struct {
	TargetFilters map[string][]string `json:"targetFilters"`
	DeploymentID  string              `json:"deploymentId"`
}

type listDeploymentTargetsOutput struct {
	TargetIDs []string `json:"targetIds"`
}

func (h *Handler) handleListDeploymentTargets(
	_ context.Context,
	in *listDeploymentTargetsInput,
) (*listDeploymentTargetsOutput, error) {
	if in.DeploymentID == "" {
		return nil, fmt.Errorf("%w: deploymentId is required", ErrDeploymentIDRequired)
	}

	ids, err := h.Backend.ListDeploymentTargets(in.DeploymentID, TargetListFilter{
		TargetStatus:        in.TargetFilters["TargetStatus"],
		ServerInstanceLabel: in.TargetFilters["ServerInstanceLabel"],
	})
	if err != nil {
		return nil, err
	}

	return &listDeploymentTargetsOutput{TargetIDs: ids}, nil
}
