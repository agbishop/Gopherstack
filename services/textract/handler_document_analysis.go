package textract

import (
	"context"
	"fmt"
)

// documentInput is the input for synchronous document operations.
type documentInput struct {
	QueriesConfig   *QueriesConfig   `json:"QueriesConfig"`
	AdaptersConfig  *AdaptersConfig  `json:"AdaptersConfig"`
	HumanLoopConfig *HumanLoopConfig `json:"HumanLoopConfig"`
	Document        struct {
		S3Object struct {
			Bucket string `json:"Bucket"`
			Name   string `json:"Name"`
		} `json:"S3Object"`
		Bytes []byte `json:"Bytes"`
	} `json:"Document"`
	FeatureTypes []string `json:"FeatureTypes"`
}

// analyzeDocumentResponse is the response for AnalyzeDocument.
type analyzeDocumentResponse struct {
	HumanLoopActivationOutput   *HumanLoopActivationOutput `json:"HumanLoopActivationOutput,omitempty"`
	AnalyzeDocumentModelVersion string                     `json:"AnalyzeDocumentModelVersion"`
	Blocks                      []Block                    `json:"Blocks"`
	DocumentMetadata            struct {
		Pages int `json:"Pages"`
	} `json:"DocumentMetadata"`
}

func (h *Handler) handleAnalyzeDocument(
	ctx context.Context,
	in *documentInput,
) (*analyzeDocumentResponse, error) {
	if err := validateAnalyzeDocumentFeatureTypes(in.FeatureTypes); err != nil {
		return nil, err
	}

	if err := validateQueriesConfig(in.FeatureTypes, in.QueriesConfig); err != nil {
		return nil, err
	}

	if err := validateHumanLoopConfig(in.HumanLoopConfig); err != nil {
		return nil, err
	}

	if err := h.checkS3Object(ctx, in.Document.S3Object.Bucket, in.Document.S3Object.Name); err != nil {
		return nil, err
	}

	uri := documentURI(in.Document.S3Object.Bucket, in.Document.S3Object.Name)

	var blocks []Block

	if b, ok := h.Backend.(*InMemoryBackend); ok {
		if err := b.ValidateAdaptersConfig(ctx, in.AdaptersConfig); err != nil {
			return nil, err
		}

		blocks = b.AnalyzeDocumentWithFeatures(ctx, uri, in.FeatureTypes, in.QueriesConfig)
	} else {
		blocks = h.Backend.AnalyzeDocument(ctx, uri)
	}

	resp := &analyzeDocumentResponse{
		Blocks:                      blocks,
		AnalyzeDocumentModelVersion: modelVersion10,
	}
	// Pages=1 for now; should reflect actual page count for multi-page PDFs.
	resp.DocumentMetadata.Pages = 1

	return resp, nil
}

func (h *Handler) handleStartDocumentAnalysis(
	ctx context.Context,
	in *asyncInput,
) (*startJobResponse, error) {
	if err := validateAnalyzeDocumentFeatureTypes(in.FeatureTypes); err != nil {
		return nil, err
	}

	if err := validateQueriesConfig(in.FeatureTypes, in.QueriesConfig); err != nil {
		return nil, err
	}

	uri, err := h.resolveDocumentLocation(ctx, in.DocumentLocation.S3Object.Bucket, in.DocumentLocation.S3Object.Name)
	if err != nil {
		return nil, err
	}

	var job *DocumentJob

	if b, ok := h.Backend.(*InMemoryBackend); ok {
		if err = b.ValidateAdaptersConfig(ctx, in.AdaptersConfig); err != nil {
			return nil, err
		}

		job, err = b.StartDocumentAnalysisWithOptions(
			ctx,
			uri,
			in.FeatureTypes,
			in.QueriesConfig,
			in.OutputConfig,
			in.JobTag,
			in.ClientRequestToken,
		)
	} else {
		job, err = h.Backend.StartDocumentAnalysis(ctx, uri)
	}

	if err != nil {
		return nil, err
	}

	return &startJobResponse{JobID: job.JobID}, nil
}

// getDocumentAnalysisResponse is the response for GetDocumentAnalysis.
type getDocumentAnalysisResponse struct {
	JobStatus                   string         `json:"JobStatus"`
	NextToken                   string         `json:"NextToken,omitempty"`
	StatusMessage               string         `json:"StatusMessage,omitempty"`
	AnalyzeDocumentModelVersion string         `json:"AnalyzeDocumentModelVersion"`
	Blocks                      []Block        `json:"Blocks"`
	Warnings                    []WarningBlock `json:"Warnings,omitempty"`
	DocumentMetadata            struct {
		Pages int `json:"Pages"`
	} `json:"DocumentMetadata"`
}

func (h *Handler) handleGetDocumentAnalysis(
	ctx context.Context,
	in *getJobInput,
) (*getDocumentAnalysisResponse, error) {
	if in.JobID == "" {
		return nil, fmt.Errorf("%w: JobID is required", errInvalidRequest)
	}

	job, err := h.Backend.GetDocumentAnalysis(ctx, in.JobID)
	if err != nil {
		return nil, err
	}

	blocks, nextToken := PaginateBlocks(job.Blocks, in.MaxResults, in.NextToken)

	resp := &getDocumentAnalysisResponse{
		JobStatus:                   job.JobStatus,
		Blocks:                      blocks,
		NextToken:                   nextToken,
		StatusMessage:               job.StatusMessage,
		Warnings:                    job.Warnings,
		AnalyzeDocumentModelVersion: modelVersion10,
	}
	resp.DocumentMetadata.Pages = 1

	return resp, nil
}
