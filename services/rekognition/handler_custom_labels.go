package rekognition

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

func (h *Handler) customLabelOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"DetectCustomLabels": service.WrapOp(h.handleDetectCustomLabels),
	}
}

type detectCustomLabelsReq struct {
	ProjectVersionArn string   `json:"ProjectVersionArn"`
	Image             imageRef `json:"Image"`
	MaxResults        int32    `json:"MaxResults"`
	MinConfidence     float32  `json:"MinConfidence"`
}

type customLabelEntry struct {
	Name       string  `json:"Name"`
	Confidence float32 `json:"Confidence"`
}

type detectCustomLabelsResp struct {
	CustomLabels []customLabelEntry `json:"CustomLabels"`
}

func (h *Handler) handleDetectCustomLabels(
	ctx context.Context, req *detectCustomLabelsReq,
) (*detectCustomLabelsResp, error) {
	if err := h.checkImageRef(ctx, req.Image); err != nil {
		return nil, err
	}

	return &detectCustomLabelsResp{CustomLabels: []customLabelEntry{}}, nil
}
