package rekognition

import (
	"context"
	"fmt"

	sdktypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

func (h *Handler) faceOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"IndexFaces":         service.WrapOp(h.handleIndexFaces),
		"DeleteFaces":        service.WrapOp(h.handleDeleteFaces),
		"ListFaces":          service.WrapOp(h.handleListFaces),
		"SearchFaces":        service.WrapOp(h.handleSearchFaces),
		"SearchFacesByImage": service.WrapOp(h.handleSearchFacesByImage),
		"CompareFaces":       service.WrapOp(h.handleCompareFaces),
		"DetectFaces":        service.WrapOp(h.handleDetectFaces),
		"StartFaceDetection": service.WrapOp(h.handleStartFaceDetection),
		"GetFaceDetection":   service.WrapOp(h.handleGetFaceDetection),
		"StartFaceSearch":    service.WrapOp(h.handleStartFaceSearch),
		"GetFaceSearch":      service.WrapOp(h.handleGetFaceSearch),
	}
}

// --- Face requests ---

type indexFacesReq struct {
	CollectionID    string `json:"CollectionId"`
	ExternalImageID string `json:"ExternalImageId"`
}

type faceRecord struct {
	Face struct {
		FaceID          string  `json:"FaceId"`
		ImageID         string  `json:"ImageId"`
		ExternalImageID string  `json:"ExternalImageId"`
		Confidence      float64 `json:"Confidence"`
	} `json:"Face"`
}

type indexFacesResp struct {
	FaceModelVersion string       `json:"FaceModelVersion"`
	FaceRecords      []faceRecord `json:"FaceRecords"`
}

func (h *Handler) handleIndexFaces(_ context.Context, req *indexFacesReq) (*indexFacesResp, error) {
	if req.CollectionID == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	faces, err := h.Backend.IndexFaces(req.CollectionID, req.ExternalImageID)
	if err != nil {
		return nil, err
	}

	records := make([]faceRecord, 0, len(faces))

	for _, f := range faces {
		var rec faceRecord
		rec.Face.FaceID = f.FaceID
		rec.Face.ImageID = f.ImageID
		rec.Face.ExternalImageID = f.ExternalImageID
		rec.Face.Confidence = f.Confidence
		records = append(records, rec)
	}

	return &indexFacesResp{
		FaceModelVersion: faceModelVersion,
		FaceRecords:      records,
	}, nil
}

type deleteFacesReq struct {
	CollectionID string   `json:"CollectionId"`
	FaceIDs      []string `json:"FaceIds"`
}

type deleteFacesResp struct {
	DeletedFaces []string `json:"DeletedFaces"`
}

func (h *Handler) handleDeleteFaces(_ context.Context, req *deleteFacesReq) (*deleteFacesResp, error) {
	if req.CollectionID == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	deleted, err := h.Backend.DeleteFaces(req.CollectionID, req.FaceIDs)
	if err != nil {
		return nil, err
	}

	if deleted == nil {
		deleted = []string{}
	}

	return &deleteFacesResp{DeletedFaces: deleted}, nil
}

type listFacesReq struct {
	CollectionID string   `json:"CollectionId"`
	NextToken    string   `json:"NextToken"`
	UserID       string   `json:"UserId"`
	FaceIDs      []string `json:"FaceIds"`
	MaxResults   int32    `json:"MaxResults"`
}

type faceEntry struct {
	FaceID          string  `json:"FaceId"`
	ImageID         string  `json:"ImageId"`
	ExternalImageID string  `json:"ExternalImageId"`
	Confidence      float64 `json:"Confidence"`
}

type listFacesResp struct {
	FaceModelVersion string      `json:"FaceModelVersion"`
	NextToken        string      `json:"NextToken,omitempty"`
	Faces            []faceEntry `json:"Faces"`
}

func (h *Handler) handleListFaces(_ context.Context, req *listFacesReq) (*listFacesResp, error) {
	if req.CollectionID == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	faces, nextToken, err := h.Backend.ListFaces(
		req.CollectionID, req.FaceIDs, req.UserID, req.MaxResults, req.NextToken,
	)
	if err != nil {
		return nil, err
	}

	entries := make([]faceEntry, 0, len(faces))

	for _, f := range faces {
		entries = append(entries, faceEntry{
			FaceID:          f.FaceID,
			ImageID:         f.ImageID,
			ExternalImageID: f.ExternalImageID,
			Confidence:      f.Confidence,
		})
	}

	return &listFacesResp{
		FaceModelVersion: faceModelVersion,
		Faces:            entries,
		NextToken:        nextToken,
	}, nil
}

type searchFacesReq struct {
	CollectionID string `json:"CollectionId"`
	FaceID       string `json:"FaceId"`
	MaxFaces     int32  `json:"MaxFaces"`
}

type faceMatchEntry struct {
	Face       faceEntry `json:"Face"`
	Similarity float64   `json:"Similarity"`
}

type searchFacesResp struct {
	FaceModelVersion string           `json:"FaceModelVersion"`
	SearchedFaceID   string           `json:"SearchedFaceId"`
	FaceMatches      []faceMatchEntry `json:"FaceMatches"`
}

func (h *Handler) handleSearchFaces(_ context.Context, req *searchFacesReq) (*searchFacesResp, error) {
	if req.CollectionID == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	if req.FaceID == "" {
		return nil, fmt.Errorf("%w: FaceId is required", ErrValidation)
	}

	matches, err := h.Backend.SearchFaces(req.CollectionID, req.FaceID, req.MaxFaces)
	if err != nil {
		return nil, err
	}

	entries := make([]faceMatchEntry, 0, len(matches))

	for _, m := range matches {
		entries = append(entries, faceMatchEntry{
			Similarity: m.Similarity,
			Face: faceEntry{
				FaceID:          m.Face.FaceID,
				ImageID:         m.Face.ImageID,
				ExternalImageID: m.Face.ExternalImageID,
				Confidence:      m.Face.Confidence,
			},
		})
	}

	return &searchFacesResp{
		FaceMatches:      entries,
		FaceModelVersion: faceModelVersion,
		SearchedFaceID:   req.FaceID,
	}, nil
}

type searchFacesByImageReq struct {
	CollectionID  string   `json:"CollectionId"`
	QualityFilter string   `json:"QualityFilter"`
	Image         imageRef `json:"Image"`
	MaxFaces      int32    `json:"MaxFaces"`
}

type searchFacesByImageResp struct {
	FaceModelVersion string           `json:"FaceModelVersion"`
	FaceMatches      []faceMatchEntry `json:"FaceMatches"`
}

// isValidQualityFilter derives its answer from types.QualityFilter.Values()
// so it cannot drift from the real enum; "" is also valid (field omitted).
func isValidQualityFilter(v string) bool {
	if v == "" {
		return true
	}

	for _, qf := range sdktypes.QualityFilter("").Values() {
		if string(qf) == v {
			return true
		}
	}

	return false
}

func (h *Handler) handleSearchFacesByImage(
	ctx context.Context,
	req *searchFacesByImageReq,
) (*searchFacesByImageResp, error) {
	if req.CollectionID == "" {
		return nil, fmt.Errorf("%w: CollectionId is required", ErrValidation)
	}

	if !isValidQualityFilter(req.QualityFilter) {
		return nil, fmt.Errorf("%w: QualityFilter value %q is not valid", ErrValidation, req.QualityFilter)
	}

	if err := h.checkImageRef(ctx, req.Image); err != nil {
		return nil, err
	}

	imageKey := imageRefKey(req.Image)
	matches, err := h.Backend.SearchFacesByImage(req.CollectionID, req.MaxFaces, imageKey)
	if err != nil {
		return nil, err
	}

	entries := make([]faceMatchEntry, 0, len(matches))

	for _, m := range matches {
		entries = append(entries, faceMatchEntry{
			Similarity: m.Similarity,
			Face: faceEntry{
				FaceID:          m.Face.FaceID,
				ImageID:         m.Face.ImageID,
				ExternalImageID: m.Face.ExternalImageID,
				Confidence:      m.Face.Confidence,
			},
		})
	}

	return &searchFacesByImageResp{
		FaceMatches:      entries,
		FaceModelVersion: faceModelVersion,
	}, nil
}

// --- Face image analysis (stateless mock results) ---

type compareFacesReq struct {
	QualityFilter       string   `json:"QualityFilter"`
	SourceImage         imageRef `json:"SourceImage"`
	TargetImage         imageRef `json:"TargetImage"`
	SimilarityThreshold float64  `json:"SimilarityThreshold"`
}

type faceMatchResult struct {
	Similarity float64 `json:"Similarity"`
	Face       struct {
		BoundingBox struct {
			Height float32 `json:"Height"`
			Left   float32 `json:"Left"`
			Top    float32 `json:"Top"`
			Width  float32 `json:"Width"`
		} `json:"BoundingBox"`
		Confidence float64 `json:"Confidence"`
	} `json:"Face"`
}

type compareFacesResp struct {
	FaceMatches     []faceMatchResult `json:"FaceMatches"`
	UnmatchedFaces  []struct{}        `json:"UnmatchedFaces"`
	SourceImageFace struct {
		BoundingBox struct {
			Height float32 `json:"Height"`
			Left   float32 `json:"Left"`
			Top    float32 `json:"Top"`
			Width  float32 `json:"Width"`
		} `json:"BoundingBox"`
		Confidence float64 `json:"Confidence"`
	} `json:"SourceImageFace"`
}

// compareFacesDefaultThreshold mirrors CompareFacesInput.SimilarityThreshold's
// documented default (api_op_CompareFaces.go: "By default, only faces with a
// similarity score of greater than or equal to 80% are returned").
const compareFacesDefaultThreshold = 80.0

// compareFacesIdenticalSimilarity/compareFacesDistinctSimilarity are this
// stateless mock's synthetic similarity scores: identical image references
// score a perfect match, distinct ones a plausible but lower score, so
// SimilarityThreshold has an observable effect instead of being ignored.
const (
	compareFacesIdenticalSimilarity = 100.0
	compareFacesDistinctSimilarity  = 92.0
)

func (h *Handler) handleCompareFaces(ctx context.Context, req *compareFacesReq) (*compareFacesResp, error) {
	if !isValidQualityFilter(req.QualityFilter) {
		return nil, fmt.Errorf("%w: QualityFilter value %q is not valid", ErrValidation, req.QualityFilter)
	}

	if err := h.checkImageRef(ctx, req.SourceImage); err != nil {
		return nil, err
	}

	if err := h.checkImageRef(ctx, req.TargetImage); err != nil {
		return nil, err
	}

	resp := &compareFacesResp{}
	resp.SourceImageFace.Confidence = 99.9
	resp.SourceImageFace.BoundingBox.Height = 0.5
	resp.SourceImageFace.BoundingBox.Width = 0.3
	resp.FaceMatches = []faceMatchResult{}
	resp.UnmatchedFaces = []struct{}{}

	threshold := req.SimilarityThreshold
	if threshold <= 0 {
		threshold = compareFacesDefaultThreshold
	}

	similarity := compareFacesDistinctSimilarity
	if imageRefKey(req.SourceImage) == imageRefKey(req.TargetImage) {
		similarity = compareFacesIdenticalSimilarity
	}

	if similarity >= threshold {
		match := faceMatchResult{Similarity: similarity}
		match.Face.Confidence = 99.9
		match.Face.BoundingBox.Height = 0.5
		match.Face.BoundingBox.Width = 0.3
		resp.FaceMatches = append(resp.FaceMatches, match)
	}

	return resp, nil
}

type detectFacesReq struct {
	Image      imageRef `json:"Image"`
	Attributes []string `json:"Attributes"`
}

// isValidFaceAttribute derives its answer from types.Attribute.Values() so it
// cannot drift from the real enum.
func isValidFaceAttribute(v string) bool {
	for _, a := range sdktypes.Attribute("").Values() {
		if string(a) == v {
			return true
		}
	}

	return false
}

type faceDetailEntry struct {
	BoundingBox struct {
		Height float32 `json:"Height"`
		Left   float32 `json:"Left"`
		Top    float32 `json:"Top"`
		Width  float32 `json:"Width"`
	} `json:"BoundingBox"`
	Confidence float64 `json:"Confidence"`
}

type detectFacesResp struct {
	OrientationCorrection string            `json:"OrientationCorrection"`
	FaceDetails           []faceDetailEntry `json:"FaceDetails"`
}

func (h *Handler) handleDetectFaces(ctx context.Context, req *detectFacesReq) (*detectFacesResp, error) {
	if err := h.checkImageRef(ctx, req.Image); err != nil {
		return nil, err
	}

	for _, a := range req.Attributes {
		if !isValidFaceAttribute(a) {
			return nil, fmt.Errorf("%w: Attributes value %q is not valid", ErrValidation, a)
		}
	}

	return &detectFacesResp{
		FaceDetails:           []faceDetailEntry{},
		OrientationCorrection: orientationRotate0,
	}, nil
}

// --- Async video jobs: face detection / face search ---

type startFaceDetectionReq struct {
	Video              videoRef `json:"Video"`
	ClientRequestToken string   `json:"ClientRequestToken"`
	JobTag             string   `json:"JobTag"`
	FaceAttributes     string   `json:"FaceAttributes"`
}

func (h *Handler) handleStartFaceDetection(
	ctx context.Context, req *startFaceDetectionReq,
) (*startJobResp, error) {
	if err := h.checkVideoRef(ctx, req.Video); err != nil {
		return nil, err
	}

	bucket, name, version := videoRefS3(req.Video)

	jobID, err := h.Backend.StartAsyncJob(StartAsyncJobParams{
		JobType:        "face_detection",
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

type getFaceDetectionResp struct {
	getJobBaseResp
	Faces []struct{} `json:"Faces"`
}

func (h *Handler) handleGetFaceDetection(
	_ context.Context, req *getJobReq,
) (*getFaceDetectionResp, error) {
	base, _, err := h.getJobBase(req.JobId)
	if err != nil {
		return nil, err
	}

	return &getFaceDetectionResp{
		getJobBaseResp: *base,
		Faces:          []struct{}{},
	}, nil
}

type startFaceSearchReq struct {
	Video              videoRef `json:"Video"`
	CollectionId       string   `json:"CollectionId"` //nolint:revive,staticcheck // existing issue.
	ClientRequestToken string   `json:"ClientRequestToken"`
	JobTag             string   `json:"JobTag"`
	FaceMatchThreshold float32  `json:"FaceMatchThreshold"`
}

func (h *Handler) handleStartFaceSearch(
	ctx context.Context, req *startFaceSearchReq,
) (*startJobResp, error) {
	if err := h.checkVideoRef(ctx, req.Video); err != nil {
		return nil, err
	}

	bucket, name, version := videoRefS3(req.Video)

	jobID, err := h.Backend.StartAsyncJob(StartAsyncJobParams{
		JobType:        "face_search",
		CollectionID:   req.CollectionId,
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

type getFaceSearchResp struct {
	getJobBaseResp
	Persons []struct{} `json:"Persons"`
}

func (h *Handler) handleGetFaceSearch(
	_ context.Context, req *getJobReq,
) (*getFaceSearchResp, error) {
	base, _, err := h.getJobBase(req.JobId)
	if err != nil {
		return nil, err
	}

	return &getFaceSearchResp{
		getJobBaseResp: *base,
		Persons:        []struct{}{},
	}, nil
}
