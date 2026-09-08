package rekognition

import (
	"context"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

func (h *Handler) labelOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"DetectLabels":        service.WrapOp(h.handleDetectLabels),
		"StartLabelDetection": service.WrapOp(h.handleStartLabelDetection),
		"GetLabelDetection":   service.WrapOp(h.handleGetLabelDetection),
	}
}

type detectLabelsReq struct {
	Image         imageRef `json:"Image"`
	MaxLabels     int32    `json:"MaxLabels"`
	MinConfidence float64  `json:"MinConfidence"`
}

type labelEntry struct {
	Name       string  `json:"Name"`
	Confidence float64 `json:"Confidence"`
}

type detectLabelsResp struct {
	OrientationCorrection string       `json:"OrientationCorrection"`
	Labels                []labelEntry `json:"Labels"`
}

func (h *Handler) handleDetectLabels(ctx context.Context, req *detectLabelsReq) (*detectLabelsResp, error) {
	if err := h.checkImageRef(ctx, req.Image); err != nil {
		return nil, err
	}

	labels := plausibleLabels(resolveMinConfidence(req.MinConfidence), req.MaxLabels)

	return &detectLabelsResp{
		Labels:                labels,
		OrientationCorrection: orientationRotate0,
	}, nil
}

// Synthetic confidence scores for generic scene labels, in descending order.
const (
	confPerson     = 95.5
	confHuman      = 94.8
	confOutdoor    = 87.3
	confNature     = 73.2
	confSky        = 68.9
	confVegetation = 62.1
	confAnimal     = 55.4

	// defaultMinConfidence is DetectLabelsInput.MinConfidence's documented
	// default: "you can specify MinConfidence to control the confidence
	// threshold for the labels returned. The default is 55%.".
	defaultMinConfidence = 55.0
)

// resolveMinConfidence returns the MinConfidence to apply when the client omitted it (zero value).
func resolveMinConfidence(v float64) float64 {
	if v <= 0 {
		return defaultMinConfidence
	}

	return v
}

// plausibleLabels returns a set of generic scene labels above the confidence threshold.
func plausibleLabels(minConfidence float64, maxLabels int32) []labelEntry {
	all := []labelEntry{
		{Name: "Person", Confidence: confPerson},
		{Name: "Human", Confidence: confHuman},
		{Name: "Outdoor", Confidence: confOutdoor},
		{Name: "Nature", Confidence: confNature},
		{Name: "Sky", Confidence: confSky},
		{Name: "Vegetation", Confidence: confVegetation},
		{Name: "Animal", Confidence: confAnimal},
	}
	out := make([]labelEntry, 0, len(all))
	for _, l := range all {
		if l.Confidence >= minConfidence {
			out = append(out, l)
		}
		if maxLabels > 0 && len(out) >= int(maxLabels) {
			break
		}
	}

	return out
}

// --- Async video jobs: label detection ---

type startLabelDetectionReq struct {
	Video              videoRef `json:"Video"`
	ClientRequestToken string   `json:"ClientRequestToken"`
	JobTag             string   `json:"JobTag"`
	MinConfidence      float32  `json:"MinConfidence"`
}

func (h *Handler) handleStartLabelDetection(
	ctx context.Context, req *startLabelDetectionReq,
) (*startJobResp, error) {
	if err := h.checkVideoRef(ctx, req.Video); err != nil {
		return nil, err
	}

	bucket, name, version := videoRefS3(req.Video)

	jobID, err := h.Backend.StartAsyncJob(StartAsyncJobParams{
		JobType:        "label_detection",
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

type getLabelDetectionReq struct {
	JobId       string `json:"JobId"` //nolint:revive,staticcheck // existing issue.
	NextToken   string `json:"NextToken"`
	AggregateBy string `json:"AggregateBy"`
	SortBy      string `json:"SortBy"`
	MaxResults  int32  `json:"MaxResults"`
}

type getLabelDetectionResp struct {
	getJobBaseResp
	GetRequestMetadata *getRequestMetadataWire `json:"GetRequestMetadata,omitempty"`
	Labels             []struct{}              `json:"Labels"`
}

// labelDetectionSortByDefault is GetLabelDetectionInput.SortBy's documented
// default ("The default sort is by TIMESTAMP.", api_op_GetLabelDetection.go).
const labelDetectionSortByDefault = "TIMESTAMP"

func (h *Handler) handleGetLabelDetection(
	_ context.Context, req *getLabelDetectionReq,
) (*getLabelDetectionResp, error) {
	base, _, err := h.getJobBase(req.JobId)
	if err != nil {
		return nil, err
	}

	return &getLabelDetectionResp{
		getJobBaseResp: *base,
		Labels:         []struct{}{},
		GetRequestMetadata: &getRequestMetadataWire{
			AggregateBy: req.AggregateBy,
			SortBy:      orDefault(req.SortBy, labelDetectionSortByDefault),
		},
	}, nil
}
