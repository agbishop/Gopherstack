package rekognition

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

func (h *Handler) textDetectionOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"DetectText":         service.WrapOp(h.handleDetectText),
		"StartTextDetection": service.WrapOp(h.handleStartTextDetection),
		"GetTextDetection":   service.WrapOp(h.handleGetTextDetection),
	}
}

type detectTextReq struct {
	Filters *struct{} `json:"Filters"`
	Image   imageRef  `json:"Image"`
}

type textDetectionEntry struct {
	DetectedText string  `json:"DetectedText"`
	Type         string  `json:"Type"`
	Confidence   float64 `json:"Confidence"`
	Id           int32   `json:"Id"` //nolint:revive,staticcheck // existing issue.
}

type detectTextResp struct {
	TextModelVersion string               `json:"TextModelVersion"`
	TextDetections   []textDetectionEntry `json:"TextDetections"`
}

func (h *Handler) handleDetectText(ctx context.Context, req *detectTextReq) (*detectTextResp, error) {
	if err := h.checkImageRef(ctx, req.Image); err != nil {
		return nil, err
	}

	detections := plausibleTextDetections(req)

	return &detectTextResp{
		TextDetections:   detections,
		TextModelVersion: "3.1",
	}, nil
}

// plausibleTextDetections returns minimal text detection results derived from the image reference.
func plausibleTextDetections(req *detectTextReq) []textDetectionEntry {
	// Derive a plausible text value from the image S3 key when available.
	label := "SAMPLE TEXT"
	if req.Image.S3Object != nil && req.Image.S3Object.Name != "" {
		label = req.Image.S3Object.Name
	}

	const textConfidence = 97.2

	return []textDetectionEntry{
		{Id: 0, DetectedText: label, Type: "LINE", Confidence: textConfidence},
		{Id: 1, DetectedText: label, Type: "WORD", Confidence: textConfidence},
	}
}

// --- Async video jobs: text detection ---

type startTextDetectionReq struct {
	Video              videoRef `json:"Video"`
	ClientRequestToken string   `json:"ClientRequestToken"`
	JobTag             string   `json:"JobTag"`
}

func (h *Handler) handleStartTextDetection(
	ctx context.Context, req *startTextDetectionReq,
) (*startJobResp, error) {
	if err := h.checkVideoRef(ctx, req.Video); err != nil {
		return nil, err
	}

	bucket, name, version := videoRefS3(req.Video)

	jobID, err := h.Backend.StartAsyncJob(StartAsyncJobParams{
		JobType:        "text_detection",
		JobTag:         req.JobTag,
		VideoS3Bucket:  bucket,
		VideoS3Name:    name,
		VideoS3Version: version,
	})
	if err != nil {
		return nil, err
	}

	return &startJobResp{JobId: jobID}, nil
}

type getTextDetectionResp struct {
	getJobBaseResp
	TextDetections []struct{} `json:"TextDetections"`
}

func (h *Handler) handleGetTextDetection(
	_ context.Context, req *getJobReq,
) (*getTextDetectionResp, error) {
	base, _, err := h.getJobBase(req.JobId)
	if err != nil {
		return nil, err
	}

	return &getTextDetectionResp{
		getJobBaseResp: *base,
		TextDetections: []struct{}{},
	}, nil
}
