package codepipeline

import (
	"context"
	"fmt"
)

type acknowledgeThirdPartyJobInput struct {
	ClientToken string `json:"clientToken"`
	JobID       string `json:"jobId"`
	Nonce       string `json:"nonce"`
}

type acknowledgeThirdPartyJobOutput struct {
	Status string `json:"status"`
}

func (h *Handler) handleAcknowledgeThirdPartyJob(
	ctx context.Context,
	in *acknowledgeThirdPartyJobInput,
) (*acknowledgeThirdPartyJobOutput, error) {
	if in.JobID == "" {
		return nil, fmt.Errorf("%w: jobId is required", errInvalidRequest)
	}

	if in.Nonce == "" {
		return nil, fmt.Errorf("%w: nonce is required", errInvalidRequest)
	}

	if in.ClientToken == "" {
		return nil, fmt.Errorf("%w: clientToken is required", errInvalidRequest)
	}

	status, err := h.Backend.AcknowledgeThirdPartyJob(ctx, in.JobID, in.Nonce, in.ClientToken)
	if err != nil {
		return nil, err
	}

	return &acknowledgeThirdPartyJobOutput{Status: status}, nil
}

type pollForThirdPartyJobsInput struct {
	ActionTypeID struct {
		Category string `json:"category"`
		Owner    string `json:"owner"`
		Provider string `json:"provider"`
		Version  string `json:"version"`
	} `json:"actionTypeId"`
	MaxBatchSize int32 `json:"maxBatchSize"`
}

type pollForThirdPartyJobsOutput struct {
	Jobs []map[string]any `json:"jobs"`
}

func (h *Handler) handlePollForThirdPartyJobs(
	ctx context.Context,
	in *pollForThirdPartyJobsInput,
) (*pollForThirdPartyJobsOutput, error) {
	jobs, err := h.Backend.PollForThirdPartyJobs(
		ctx, in.ActionTypeID.Category, in.ActionTypeID.Provider, in.ActionTypeID.Version,
	)
	if err != nil {
		return nil, err
	}

	limit := in.MaxBatchSize
	if limit <= 0 || limit > maxJobsPerPoll {
		limit = maxJobsPerPoll
	}
	if int(limit) < len(jobs) {
		jobs = jobs[:limit]
	}

	// AWS's real ThirdPartyJob (PollForThirdPartyJobsOutput.jobs) is {ClientId,
	// JobId} -- unlike the plain Job type, it does NOT carry Nonce: a
	// third-party worker only learns the nonce later, from
	// GetThirdPartyJobDetails, gated behind the clientToken/ClientId pairing.
	// Leaking Nonce here (as this handler previously did, under the wrong key
	// "id" instead of "jobId") both used the wrong wire key and exposed data
	// real AWS deliberately withholds at this step.
	items := make([]map[string]any, len(jobs))
	for i, j := range jobs {
		items[i] = map[string]any{"jobId": j.ID, "clientId": j.ClientID}
	}

	return &pollForThirdPartyJobsOutput{Jobs: items}, nil
}

type getThirdPartyJobDetailsInput struct {
	JobID       string `json:"jobId"`
	ClientToken string `json:"clientToken"`
}

type getThirdPartyJobDetailsOutput struct {
	JobDetails map[string]any `json:"jobDetails"`
}

func (h *Handler) handleGetThirdPartyJobDetails(
	ctx context.Context,
	in *getThirdPartyJobDetailsInput,
) (*getThirdPartyJobDetailsOutput, error) {
	if in.JobID == "" {
		return nil, fmt.Errorf("%w: jobId is required", errInvalidRequest)
	}

	if in.ClientToken == "" {
		return nil, fmt.Errorf("%w: clientToken is required", errInvalidRequest)
	}

	job, err := h.Backend.GetThirdPartyJobDetails(ctx, in.JobID, in.ClientToken)
	if err != nil {
		return nil, err
	}

	return &getThirdPartyJobDetailsOutput{
		JobDetails: map[string]any{
			keyJobID: job.ID,
			keyNonce: job.Nonce,
			"data":   jobDataResponse{ActionTypeID: job.ActionTypeID},
		},
	}, nil
}

type putThirdPartyJobSuccessResultInput struct {
	JobID       string `json:"jobId"`
	ClientToken string `json:"clientToken"`
}

func (h *Handler) handlePutThirdPartyJobSuccessResult(
	ctx context.Context,
	in *putThirdPartyJobSuccessResultInput,
) (*emptyOut, error) {
	if in.JobID == "" {
		return nil, fmt.Errorf("%w: jobId is required", errInvalidRequest)
	}

	if in.ClientToken == "" {
		return nil, fmt.Errorf("%w: clientToken is required", errInvalidRequest)
	}

	return &emptyOut{}, h.Backend.PutThirdPartyJobSuccessResult(ctx, in.JobID, in.ClientToken)
}

type putThirdPartyJobFailureResultInput struct {
	JobID          string `json:"jobId"`
	ClientToken    string `json:"clientToken"`
	FailureDetails struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"failureDetails"`
}

func (h *Handler) handlePutThirdPartyJobFailureResult(
	ctx context.Context,
	in *putThirdPartyJobFailureResultInput,
) (*emptyOut, error) {
	if in.JobID == "" {
		return nil, fmt.Errorf("%w: jobId is required", errInvalidRequest)
	}

	if in.ClientToken == "" {
		return nil, fmt.Errorf("%w: clientToken is required", errInvalidRequest)
	}

	return &emptyOut{}, h.Backend.PutThirdPartyJobFailureResult(
		ctx,
		in.JobID,
		in.ClientToken,
		in.FailureDetails.Message,
		in.FailureDetails.Type,
	)
}
