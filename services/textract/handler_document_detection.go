package textract

import (
	"context"
	"fmt"
)

// detectDocumentTextResponse is the response for DetectDocumentText.
type detectDocumentTextResponse struct {
	DetectDocumentTextModelVersion string  `json:"DetectDocumentTextModelVersion"`
	Blocks                         []Block `json:"Blocks"`
	DocumentMetadata               struct {
		Pages int `json:"Pages"`
	} `json:"DocumentMetadata"`
}

func (h *Handler) handleDetectDocumentText(
	ctx context.Context,
	in *documentInput,
) (*detectDocumentTextResponse, error) {
	if err := h.checkS3Object(ctx, in.Document.S3Object.Bucket, in.Document.S3Object.Name); err != nil {
		return nil, err
	}

	uri := documentURI(in.Document.S3Object.Bucket, in.Document.S3Object.Name)
	blocks := h.Backend.DetectDocumentText(ctx, uri)

	resp := &detectDocumentTextResponse{
		Blocks:                         blocks,
		DetectDocumentTextModelVersion: modelVersion10,
	}
	// Pages=1 for now; should reflect actual page count for multi-page PDFs.
	resp.DocumentMetadata.Pages = 1

	return resp, nil
}

func (h *Handler) handleStartDocumentTextDetection(
	ctx context.Context,
	in *asyncInput,
) (*startJobResponse, error) {
	uri, err := h.resolveDocumentLocation(ctx, in.DocumentLocation.S3Object.Bucket, in.DocumentLocation.S3Object.Name)
	if err != nil {
		return nil, err
	}

	var job *DocumentJob

	if b, ok := h.Backend.(*InMemoryBackend); ok {
		job, err = b.StartDocumentTextDetectionWithOptions(
			ctx,
			uri,
			in.OutputConfig,
			in.NotificationChannel,
			in.JobTag,
			in.ClientRequestToken,
		)
	} else {
		job, err = h.Backend.StartDocumentTextDetection(ctx, uri)
	}

	if err != nil {
		return nil, err
	}

	return &startJobResponse{JobID: job.JobID}, nil
}

// getDocumentTextDetectionResponse is the response for GetDocumentTextDetection.
type getDocumentTextDetectionResponse struct {
	JobStatus                      string         `json:"JobStatus"`
	NextToken                      string         `json:"NextToken,omitempty"`
	StatusMessage                  string         `json:"StatusMessage,omitempty"`
	DetectDocumentTextModelVersion string         `json:"DetectDocumentTextModelVersion"`
	Blocks                         []Block        `json:"Blocks"`
	Warnings                       []WarningBlock `json:"Warnings,omitempty"`
	DocumentMetadata               struct {
		Pages int `json:"Pages"`
	} `json:"DocumentMetadata"`
}

func (h *Handler) handleGetDocumentTextDetection(
	ctx context.Context,
	in *getJobInput,
) (*getDocumentTextDetectionResponse, error) {
	if in.JobID == "" {
		return nil, fmt.Errorf("%w: JobID is required", errInvalidRequest)
	}

	job, err := h.Backend.GetDocumentTextDetection(ctx, in.JobID)
	if err != nil {
		return nil, err
	}

	blocks, nextToken := PaginateBlocks(job.Blocks, in.MaxResults, in.NextToken)

	resp := &getDocumentTextDetectionResponse{
		JobStatus:                      job.JobStatus,
		Blocks:                         blocks,
		NextToken:                      nextToken,
		StatusMessage:                  job.StatusMessage,
		Warnings:                       job.Warnings,
		DetectDocumentTextModelVersion: modelVersion10,
	}
	resp.DocumentMetadata.Pages = 1

	return resp, nil
}
