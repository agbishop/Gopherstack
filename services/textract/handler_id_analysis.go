package textract

import (
	"context"
	"fmt"
)

// analyzeIDInput is the input for AnalyzeID.
type analyzeIDInput struct {
	DocumentPages []struct {
		S3Object struct {
			Bucket string `json:"Bucket"`
			Name   string `json:"Name"`
		} `json:"S3Object"`
		Bytes []byte `json:"Bytes"`
	} `json:"DocumentPages"`
}

// analyzeIDResponse is the response for AnalyzeID.
type analyzeIDResponse struct {
	AnalyzeIDModelVersion string             `json:"AnalyzeIDModelVersion"`
	IdentityDocuments     []IdentityDocument `json:"IdentityDocuments"`
	DocumentMetadata      struct {
		Pages int `json:"Pages"`
	} `json:"DocumentMetadata"`
}

func (h *Handler) handleAnalyzeID(
	ctx context.Context,
	in *analyzeIDInput,
) (*analyzeIDResponse, error) {
	if len(in.DocumentPages) == 0 {
		return nil, fmt.Errorf("%w: DocumentPages is required", errInvalidRequest)
	}

	uris := make([]string, 0, len(in.DocumentPages))
	for _, dp := range in.DocumentPages {
		if err := h.checkS3Object(ctx, dp.S3Object.Bucket, dp.S3Object.Name); err != nil {
			return nil, err
		}

		uris = append(uris, documentURI(dp.S3Object.Bucket, dp.S3Object.Name))
	}

	docs := h.Backend.AnalyzeID(ctx, uris)

	resp := &analyzeIDResponse{
		AnalyzeIDModelVersion: modelVersion10,
		IdentityDocuments:     docs,
	}
	resp.DocumentMetadata.Pages = len(in.DocumentPages)

	return resp, nil
}
