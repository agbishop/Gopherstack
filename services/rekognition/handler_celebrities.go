package rekognition

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

func (h *Handler) celebrityOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"RecognizeCelebrities":      service.WrapOp(h.handleRecognizeCelebrities),
		"GetCelebrityInfo":          service.WrapOp(h.handleGetCelebrityInfo),
		"StartCelebrityRecognition": service.WrapOp(h.handleStartCelebrityRecognition),
		"GetCelebrityRecognition":   service.WrapOp(h.handleGetCelebrityRecognition),
	}
}

type recognizeCelebritiesReq struct {
	Image imageRef `json:"Image"`
}

type celebrityEntry struct {
	Id              string   `json:"Id"` //nolint:revive,staticcheck // existing issue.
	Name            string   `json:"Name"`
	Urls            []string `json:"Urls"`
	MatchConfidence float32  `json:"MatchConfidence"`
}

type recognizeCelebritiesResp struct {
	OrientationCorrection string           `json:"OrientationCorrection"`
	CelebrityFaces        []celebrityEntry `json:"CelebrityFaces"`
	UnrecognizedFaces     []struct{}       `json:"UnrecognizedFaces"`
}

func (h *Handler) handleRecognizeCelebrities(
	ctx context.Context, req *recognizeCelebritiesReq,
) (*recognizeCelebritiesResp, error) {
	if err := h.checkImageRef(ctx, req.Image); err != nil {
		return nil, err
	}

	return &recognizeCelebritiesResp{
		CelebrityFaces:        []celebrityEntry{},
		UnrecognizedFaces:     []struct{}{},
		OrientationCorrection: orientationRotate0,
	}, nil
}

type getCelebrityInfoReq struct {
	Id string `json:"Id"` //nolint:revive,staticcheck // existing issue.
}

type knownGender struct {
	Type string `json:"Type"`
}

type getCelebrityInfoResp struct {
	KnownGender *knownGender `json:"KnownGender"`
	Name        string       `json:"Name"`
	Urls        []string     `json:"Urls"`
}

func (h *Handler) handleGetCelebrityInfo(
	_ context.Context, req *getCelebrityInfoReq,
) (*getCelebrityInfoResp, error) {
	if req.Id == "" {
		return nil, fmt.Errorf("%w: Id is required", ErrValidation)
	}

	// Mock celebrity info — return a plausible response for any ID.
	return &getCelebrityInfoResp{
		Name:        "Celebrity " + req.Id,
		Urls:        []string{},
		KnownGender: &knownGender{Type: "Unknown"},
	}, nil
}

// --- Async video jobs: celebrity recognition ---

type startCelebrityRecognitionReq struct {
	Video              videoRef `json:"Video"`
	ClientRequestToken string   `json:"ClientRequestToken"`
	JobTag             string   `json:"JobTag"`
}

func (h *Handler) handleStartCelebrityRecognition(
	ctx context.Context, req *startCelebrityRecognitionReq,
) (*startJobResp, error) {
	if err := h.checkVideoRef(ctx, req.Video); err != nil {
		return nil, err
	}

	bucket, name, version := videoRefS3(req.Video)

	jobID, err := h.Backend.StartAsyncJob(StartAsyncJobParams{
		JobType:        "celebrity_recognition",
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

type getCelebrityRecognitionResp struct {
	getJobBaseResp
	Celebrities []struct{} `json:"Celebrities"`
}

func (h *Handler) handleGetCelebrityRecognition(
	_ context.Context, req *getJobReq,
) (*getCelebrityRecognitionResp, error) {
	base, _, err := h.getJobBase(req.JobId)
	if err != nil {
		return nil, err
	}

	return &getCelebrityRecognitionResp{
		getJobBaseResp: *base,
		Celebrities:    []struct{}{},
	}, nil
}
