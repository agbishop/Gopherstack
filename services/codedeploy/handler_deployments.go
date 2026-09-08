package codedeploy

import (
	"context"
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
)

// s3LocationEntry is the wire format for an S3 deployment revision.
type s3LocationEntry struct {
	Bucket     string `json:"bucket,omitempty"`
	Key        string `json:"key,omitempty"`
	BundleType string `json:"bundleType,omitempty"`
	ETag       string `json:"eTag,omitempty"`
	Version    string `json:"version,omitempty"`
}

// gitHubLocationEntry is the wire format for a GitHub deployment revision.
type gitHubLocationEntry struct {
	Repository string `json:"repository,omitempty"`
	CommitID   string `json:"commitId,omitempty"`
}

// appSpecContentEntry is the wire format for an AppSpecContent revision.
type appSpecContentEntry struct {
	Content string `json:"content,omitempty"`
	Sha256  string `json:"sha256,omitempty"`
}

type revisionLocationInput struct {
	S3Location     *s3LocationEntry     `json:"s3Location,omitempty"`
	GitHubLocation *gitHubLocationEntry `json:"gitHubLocation,omitempty"`
	AppSpecContent *appSpecContentEntry `json:"appSpecContent,omitempty"`
	RevisionType   string               `json:"revisionType"`
}

// revisionFromWire converts a wire revisionLocationInput to a backend RevisionLocation.
func revisionFromWire(r *revisionLocationInput) *RevisionLocation {
	if r == nil {
		return nil
	}

	out := &RevisionLocation{RevisionType: r.RevisionType}

	if r.S3Location != nil {
		out.S3Location = &RevisionS3Location{
			Bucket:     r.S3Location.Bucket,
			Key:        r.S3Location.Key,
			BundleType: r.S3Location.BundleType,
			ETag:       r.S3Location.ETag,
			Version:    r.S3Location.Version,
		}
	}

	if r.GitHubLocation != nil {
		out.GitHubLocation = &RevisionGitHubLocation{
			Repository: r.GitHubLocation.Repository,
			CommitID:   r.GitHubLocation.CommitID,
		}
	}

	if r.AppSpecContent != nil {
		out.AppSpecContent = &RevisionAppSpecContent{
			Content: r.AppSpecContent.Content,
			Sha256:  r.AppSpecContent.Sha256,
		}
	}

	return out
}

// revisionToWire converts a backend RevisionLocation to the wire revisionLocationInput.
func revisionToWire(r *RevisionLocation) *revisionLocationInput {
	if r == nil {
		return nil
	}

	out := &revisionLocationInput{RevisionType: r.RevisionType}

	if r.S3Location != nil {
		out.S3Location = &s3LocationEntry{
			Bucket:     r.S3Location.Bucket,
			Key:        r.S3Location.Key,
			BundleType: r.S3Location.BundleType,
			ETag:       r.S3Location.ETag,
			Version:    r.S3Location.Version,
		}
	}

	if r.GitHubLocation != nil {
		out.GitHubLocation = &gitHubLocationEntry{
			Repository: r.GitHubLocation.Repository,
			CommitID:   r.GitHubLocation.CommitID,
		}
	}

	if r.AppSpecContent != nil {
		out.AppSpecContent = &appSpecContentEntry{
			Content: r.AppSpecContent.Content,
			Sha256:  r.AppSpecContent.Sha256,
		}
	}

	return out
}

type createDeploymentInput struct {
	Revision                      *revisionLocationInput `json:"revision"`
	ApplicationName               string                 `json:"applicationName"`
	DeploymentGroupName           string                 `json:"deploymentGroupName"`
	Description                   string                 `json:"description"`
	FileExistsBehavior            string                 `json:"fileExistsBehavior"`
	UpdateOutdatedInstancesOnly   bool                   `json:"updateOutdatedInstancesOnly"`
	IgnoreApplicationStopFailures bool                   `json:"ignoreApplicationStopFailures"`
}

type createDeploymentOutput struct {
	DeploymentID string `json:"deploymentId"`
}

func (h *Handler) handleCreateDeployment(
	_ context.Context,
	in *createDeploymentInput,
) (*createDeploymentOutput, error) {
	if in.ApplicationName == "" {
		return nil, fmt.Errorf("%w: applicationName is required", ErrApplicationNameRequired)
	}

	if in.DeploymentGroupName == "" {
		return nil, fmt.Errorf("%w: deploymentGroupName is required", ErrDeploymentGroupNameRequired)
	}

	opts := DeploymentOptions{
		Description:                   in.Description,
		FileExistsBehavior:            in.FileExistsBehavior,
		UpdateOutdatedInstancesOnly:   in.UpdateOutdatedInstancesOnly,
		IgnoreApplicationStopFailures: in.IgnoreApplicationStopFailures,
		Revision:                      revisionFromWire(in.Revision),
	}

	d, err := h.Backend.CreateDeployment(in.ApplicationName, in.DeploymentGroupName, opts)
	if err != nil {
		return nil, err
	}

	return &createDeploymentOutput{DeploymentID: d.DeploymentID}, nil
}

type getDeploymentInput struct {
	DeploymentID string `json:"deploymentId"`
}

// deploymentOverview holds a summary of instance counts for a deployment.
type deploymentOverview struct {
	Pending    int64 `json:"Pending"`
	InProgress int64 `json:"InProgress"`
	Succeeded  int64 `json:"Succeeded"`
	Failed     int64 `json:"Failed"`
	Skipped    int64 `json:"Skipped"`
	Ready      int64 `json:"Ready"`
}

type deploymentInfo struct {
	DeploymentOverview            *deploymentOverview    `json:"deploymentOverview,omitempty"`
	Revision                      *revisionLocationInput `json:"revision,omitempty"`
	CompleteTime                  *float64               `json:"completeTime,omitempty"`
	DeploymentID                  string                 `json:"deploymentId"`
	ApplicationName               string                 `json:"applicationName"`
	DeploymentGroupName           string                 `json:"deploymentGroupName"`
	DeploymentConfigName          string                 `json:"deploymentConfigName,omitempty"`
	Status                        string                 `json:"status"`
	Creator                       string                 `json:"creator"`
	Description                   string                 `json:"description,omitempty"`
	FileExistsBehavior            string                 `json:"fileExistsBehavior,omitempty"`
	CreateTime                    float64                `json:"createTime"`
	UpdateOutdatedInstancesOnly   bool                   `json:"updateOutdatedInstancesOnly,omitempty"`
	IgnoreApplicationStopFailures bool                   `json:"ignoreApplicationStopFailures,omitempty"`
}

type getDeploymentOutput struct {
	DeploymentInfo deploymentInfo `json:"deploymentInfo"`
}

func (h *Handler) handleGetDeployment(
	_ context.Context,
	in *getDeploymentInput,
) (*getDeploymentOutput, error) {
	if in.DeploymentID == "" {
		return nil, fmt.Errorf("%w: deploymentId is required", ErrDeploymentIDRequired)
	}

	d, err := h.Backend.GetDeployment(in.DeploymentID)
	if err != nil {
		return nil, err
	}

	info := deploymentInfo{
		DeploymentID:                  d.DeploymentID,
		ApplicationName:               d.ApplicationName,
		DeploymentGroupName:           d.DeploymentGroupName,
		DeploymentConfigName:          d.DeploymentConfigName,
		Status:                        d.Status,
		Creator:                       d.Creator,
		CreateTime:                    awstime.Epoch(d.CreateTime),
		Description:                   d.Description,
		FileExistsBehavior:            d.FileExistsBehavior,
		UpdateOutdatedInstancesOnly:   d.UpdateOutdatedInstancesOnly,
		IgnoreApplicationStopFailures: d.IgnoreApplicationStopFailures,
		Revision:                      revisionToWire(d.Revision),
		DeploymentOverview:            deploymentOverviewForStatus(d.Status),
	}

	if d.CompleteTime != nil {
		ct := awstime.Epoch(*d.CompleteTime)
		info.CompleteTime = &ct
	}

	return &getDeploymentOutput{DeploymentInfo: info}, nil
}

// timeRangeEntry is the wire format for a create-time range filter. The AWS
// json-1.1 protocol serializes Timestamp shapes as epoch-seconds JSON numbers
// (see smithytime.FormatEpochSeconds in the real SDK's serializers), not
// epoch milliseconds, so these must be float64 to preserve sub-second
// precision the same way awstime.Epoch does on the response side.
type timeRangeEntry struct {
	Start *float64 `json:"start,omitempty"`
	End   *float64 `json:"end,omitempty"`
}

// epochSecondsToTime converts a wire-format epoch-seconds value (as produced
// by smithytime.FormatEpochSeconds / awstime.Epoch) back into a time.Time.
func epochSecondsToTime(sec float64) time.Time {
	return time.Unix(0, int64(sec*float64(time.Second))).UTC()
}

type listDeploymentsInput struct {
	CreateTimeRange     *timeRangeEntry `json:"createTimeRange"`
	ApplicationName     string          `json:"applicationName"`
	DeploymentGroupName string          `json:"deploymentGroupName"`
	ExternalID          string          `json:"externalId"`
	IncludeOnlyStatuses []string        `json:"includeOnlyStatuses"`
}

type listDeploymentsOutput struct {
	Deployments []string `json:"deployments"`
}

func (h *Handler) handleListDeployments(
	_ context.Context,
	in *listDeploymentsInput,
) (*listDeploymentsOutput, error) {
	filter := DeploymentFilter{
		ApplicationName:     in.ApplicationName,
		DeploymentGroupName: in.DeploymentGroupName,
		ExternalID:          in.ExternalID,
		Statuses:            in.IncludeOnlyStatuses,
	}

	if in.CreateTimeRange != nil {
		if in.CreateTimeRange.Start != nil {
			t := epochSecondsToTime(*in.CreateTimeRange.Start)
			filter.CreateTimeStart = &t
		}
		if in.CreateTimeRange.End != nil {
			t := epochSecondsToTime(*in.CreateTimeRange.End)
			filter.CreateTimeEnd = &t
		}
	}

	return &listDeploymentsOutput{
		Deployments: h.Backend.ListDeployments(filter),
	}, nil
}

// deploymentOverviewForStatus returns a synthetic DeploymentOverview based on deployment status.
func deploymentOverviewForStatus(status string) *deploymentOverview {
	switch status {
	case statusSucceeded:
		return &deploymentOverview{Succeeded: 1}
	case "Failed":
		return &deploymentOverview{Failed: 1}
	case statusStopped:
		return &deploymentOverview{Skipped: 1}
	default:
		return &deploymentOverview{InProgress: 1}
	}
}

type stopDeploymentInput struct {
	DeploymentID        string `json:"deploymentId"`
	AutoRollbackEnabled bool   `json:"autoRollbackEnabled"`
}

type stopDeploymentOutput struct {
	Status        string `json:"status"`
	StatusMessage string `json:"statusMessage,omitempty"`
}

func (h *Handler) handleStopDeployment(
	_ context.Context,
	in *stopDeploymentInput,
) (*stopDeploymentOutput, error) {
	if in.DeploymentID == "" {
		return nil, fmt.Errorf("%w: deploymentId is required", ErrDeploymentIDRequired)
	}

	if err := h.Backend.StopDeployment(in.DeploymentID); err != nil {
		return nil, err
	}

	return &stopDeploymentOutput{Status: stopStatusSucceeded, StatusMessage: stopStatusSucceededMessage}, nil
}

type skipWaitTimeInput struct {
	DeploymentID string `json:"deploymentId"`
}

type skipWaitTimeOutput struct{}

func (h *Handler) handleSkipWaitTimeForInstanceTermination(
	_ context.Context,
	in *skipWaitTimeInput,
) (*skipWaitTimeOutput, error) {
	if in.DeploymentID == "" {
		return nil, fmt.Errorf("%w: deploymentId is required", ErrDeploymentIDRequired)
	}

	if _, err := h.Backend.GetDeployment(in.DeploymentID); err != nil {
		return nil, err
	}

	return &skipWaitTimeOutput{}, nil
}

type continueDeploymentInput struct {
	DeploymentID       string `json:"deploymentId"`
	DeploymentWaitType string `json:"deploymentWaitType"`
}

type continueDeploymentOutput struct{}

func (h *Handler) handleContinueDeployment(
	_ context.Context,
	in *continueDeploymentInput,
) (*continueDeploymentOutput, error) {
	if in.DeploymentID == "" {
		return nil, fmt.Errorf("%w: deploymentId is required", ErrDeploymentIDRequired)
	}

	if in.DeploymentWaitType != "" &&
		in.DeploymentWaitType != waitTypeReadyWait &&
		in.DeploymentWaitType != waitTypeTerminationWait {
		return nil, fmt.Errorf(
			"%w: deploymentWaitType must be READY_WAIT or TERMINATION_WAIT", ErrInvalidDeploymentWaitType,
		)
	}

	if err := h.Backend.ContinueDeployment(in.DeploymentID); err != nil {
		return nil, err
	}

	return &continueDeploymentOutput{}, nil
}

type batchGetDeploymentsInput struct {
	DeploymentIDs []string `json:"deploymentIds"`
}

type batchGetDeploymentsOutput struct {
	DeploymentsInfo []deploymentInfo `json:"deploymentsInfo"`
}

func (h *Handler) handleBatchGetDeployments(
	_ context.Context,
	in *batchGetDeploymentsInput,
) (*batchGetDeploymentsOutput, error) {
	if len(in.DeploymentIDs) == 0 {
		return nil, fmt.Errorf("%w: deploymentIds is required", ErrDeploymentIDRequired)
	}

	deployments := h.Backend.BatchGetDeployments(in.DeploymentIDs)

	infos := make([]deploymentInfo, 0, len(deployments))
	for _, d := range deployments {
		info := deploymentInfo{
			DeploymentID:                  d.DeploymentID,
			ApplicationName:               d.ApplicationName,
			DeploymentGroupName:           d.DeploymentGroupName,
			DeploymentConfigName:          d.DeploymentConfigName,
			Status:                        d.Status,
			Creator:                       d.Creator,
			CreateTime:                    awstime.Epoch(d.CreateTime),
			Description:                   d.Description,
			FileExistsBehavior:            d.FileExistsBehavior,
			UpdateOutdatedInstancesOnly:   d.UpdateOutdatedInstancesOnly,
			IgnoreApplicationStopFailures: d.IgnoreApplicationStopFailures,
			Revision:                      revisionToWire(d.Revision),
			DeploymentOverview:            deploymentOverviewForStatus(d.Status),
		}

		if d.CompleteTime != nil {
			ct := awstime.Epoch(*d.CompleteTime)
			info.CompleteTime = &ct
		}

		infos = append(infos, info)
	}

	return &batchGetDeploymentsOutput{DeploymentsInfo: infos}, nil
}
