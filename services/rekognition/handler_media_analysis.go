package rekognition

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

func (h *Handler) mediaAnalysisOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"StartPersonTracking":   service.WrapOp(h.handleStartPersonTracking),
		"GetPersonTracking":     service.WrapOp(h.handleGetPersonTracking),
		"StartSegmentDetection": service.WrapOp(h.handleStartSegmentDetection),
		"GetSegmentDetection":   service.WrapOp(h.handleGetSegmentDetection),
		"StartMediaAnalysisJob": service.WrapOp(h.handleStartMediaAnalysisJob),
		"GetMediaAnalysisJob":   service.WrapOp(h.handleGetMediaAnalysisJob),
		"ListMediaAnalysisJobs": service.WrapOp(h.handleListMediaAnalysisJobs),
	}
}

// --- Async video jobs: person tracking / segment detection ---

type startPersonTrackingReq struct {
	Video              videoRef `json:"Video"`
	ClientRequestToken string   `json:"ClientRequestToken"`
	JobTag             string   `json:"JobTag"`
}

func (h *Handler) handleStartPersonTracking(
	ctx context.Context, req *startPersonTrackingReq,
) (*startJobResp, error) {
	if err := h.checkVideoRef(ctx, req.Video); err != nil {
		return nil, err
	}

	bucket, name, version := videoRefS3(req.Video)

	jobID, err := h.Backend.StartAsyncJob(StartAsyncJobParams{
		JobType:        "person_tracking",
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

type getPersonTrackingResp struct {
	getJobBaseResp
	Persons []struct{} `json:"Persons"`
}

func (h *Handler) handleGetPersonTracking(
	_ context.Context, req *getJobReq,
) (*getPersonTrackingResp, error) {
	base, _, err := h.getJobBase(req.JobId)
	if err != nil {
		return nil, err
	}

	return &getPersonTrackingResp{
		getJobBaseResp: *base,
		Persons:        []struct{}{},
	}, nil
}

type startSegmentDetectionReq struct {
	Video              videoRef `json:"Video"`
	ClientRequestToken string   `json:"ClientRequestToken"`
	JobTag             string   `json:"JobTag"`
	SegmentTypes       []string `json:"SegmentTypes"`
}

func (h *Handler) handleStartSegmentDetection(
	ctx context.Context, req *startSegmentDetectionReq,
) (*startJobResp, error) {
	if err := h.checkVideoRef(ctx, req.Video); err != nil {
		return nil, err
	}

	bucket, name, version := videoRefS3(req.Video)

	jobID, err := h.Backend.StartAsyncJob(StartAsyncJobParams{
		JobType:        "segment_detection",
		JobTag:         req.JobTag,
		VideoS3Bucket:  bucket,
		VideoS3Name:    name,
		VideoS3Version: version,
		SegmentTypes:   req.SegmentTypes,
	})
	if err != nil {
		return nil, err
	}

	return &startJobResp{JobId: jobID}, nil
}

// segmentTypeInfoWire mirrors types.SegmentTypeInfo (types.go): Type is the
// segment type requested via StartSegmentDetection's SegmentTypes, echoed
// back verbatim. ModelVersion is deliberately omitted -- it names the
// internal Rekognition segment-detection model build and this backend has no
// legitimate source for that string (fabricating one would violate the
// no-invented-data rule).
type segmentTypeInfoWire struct {
	Type string `json:"Type"`
}

type getSegmentDetectionResp struct {
	getJobBaseResp
	Segments             []struct{}            `json:"Segments"`
	SelectedSegmentTypes []segmentTypeInfoWire `json:"SelectedSegmentTypes"`
}

func (h *Handler) handleGetSegmentDetection(
	_ context.Context, req *getJobReq,
) (*getSegmentDetectionResp, error) {
	base, job, err := h.getJobBase(req.JobId)
	if err != nil {
		return nil, err
	}

	selected := make([]segmentTypeInfoWire, 0, len(job.SegmentTypes))
	for _, t := range job.SegmentTypes {
		selected = append(selected, segmentTypeInfoWire{Type: t})
	}

	return &getSegmentDetectionResp{
		getJobBaseResp:       *base,
		Segments:             []struct{}{},
		SelectedSegmentTypes: selected,
	}, nil
}

// =============================================================================
// MediaAnalysis Jobs
// =============================================================================

// mediaAnalysisInputWire mirrors types.MediaAnalysisInput (types.go:1588).
type mediaAnalysisInputWire struct {
	S3Object *s3RefWire `json:"S3Object"`
}

// mediaAnalysisDetectModerationLabelsConfigWire mirrors
// types.MediaAnalysisDetectModerationLabelsConfig (types.go:1573).
type mediaAnalysisDetectModerationLabelsConfigWire struct {
	MinConfidence  *float32 `json:"MinConfidence,omitempty"`
	ProjectVersion string   `json:"ProjectVersion,omitempty"`
}

// mediaAnalysisOperationsConfigWire mirrors types.MediaAnalysisOperationsConfig
// (types.go:1701).
type mediaAnalysisOperationsConfigWire struct {
	DetectModerationLabels *mediaAnalysisDetectModerationLabelsConfigWire `json:"DetectModerationLabels,omitempty"`
}

// mediaAnalysisOutputConfigWire mirrors types.MediaAnalysisOutputConfig
// (types.go:1710).
type mediaAnalysisOutputConfigWire struct {
	S3Bucket    string `json:"S3Bucket"`
	S3KeyPrefix string `json:"S3KeyPrefix,omitempty"`
}

type startMediaAnalysisJobReq struct {
	Input              *mediaAnalysisInputWire            `json:"Input"`
	OperationsConfig   *mediaAnalysisOperationsConfigWire `json:"OperationsConfig"`
	OutputConfig       *mediaAnalysisOutputConfigWire     `json:"OutputConfig"`
	JobName            string                             `json:"JobName"`
	ClientRequestToken string                             `json:"ClientRequestToken"`
}

type startMediaAnalysisJobResp struct {
	JobId string `json:"JobId"` //nolint:revive,staticcheck // existing issue.
}

func (h *Handler) handleStartMediaAnalysisJob(
	ctx context.Context, req *startMediaAnalysisJobReq,
) (*startMediaAnalysisJobResp, error) {
	// OperationsConfig/Input/OutputConfig are required StartMediaAnalysisJobInput
	// members, Input.S3Object and OutputConfig.S3Bucket required nested members
	// (verified against validateOpStartMediaAnalysisJobInput, validators.go).
	if req.OperationsConfig == nil {
		return nil, fmt.Errorf("%w: OperationsConfig is required", ErrValidation)
	}

	if req.Input == nil {
		return nil, fmt.Errorf("%w: Input is required", ErrValidation)
	}

	if req.Input.S3Object == nil {
		return nil, fmt.Errorf("%w: Input.S3Object is required", ErrValidation)
	}

	if req.OutputConfig == nil {
		return nil, fmt.Errorf("%w: OutputConfig is required", ErrValidation)
	}

	if req.OutputConfig.S3Bucket == "" {
		return nil, fmt.Errorf("%w: OutputConfig.S3Bucket is required", ErrValidation)
	}

	if err := h.checkS3Object(ctx, req.Input.S3Object.Bucket, req.Input.S3Object.Name); err != nil {
		return nil, err
	}

	jobName := req.JobName
	if jobName == "" {
		jobName = "media-analysis-job"
	}

	params := StartMediaAnalysisJobParams{
		InputS3Bucket:           req.Input.S3Object.Bucket,
		InputS3Name:             req.Input.S3Object.Name,
		InputS3Version:          req.Input.S3Object.Version,
		OutputConfigS3Bucket:    req.OutputConfig.S3Bucket,
		OutputConfigS3KeyPrefix: req.OutputConfig.S3KeyPrefix,
	}

	if dml := req.OperationsConfig.DetectModerationLabels; dml != nil {
		params.HasDetectModerationLabels = true
		params.DetectModerationLabelsMinConfidence = dml.MinConfidence
		params.DetectModerationLabelsProjectVersion = dml.ProjectVersion
	}

	jobID, err := h.Backend.StartMediaAnalysisJob(jobName, params)
	if err != nil {
		return nil, err
	}

	return &startMediaAnalysisJobResp{JobId: jobID}, nil
}

type getMediaAnalysisJobReq struct {
	JobId string `json:"JobId"` //nolint:revive,staticcheck // existing issue.
}

// mediaAnalysisInputFromDomain renders job's Input as the wire shape.
// Input is a required StartMediaAnalysisJobInput member, so every stored job
// has one.
func mediaAnalysisInputFromDomain(job *MediaAnalysisJob) *mediaAnalysisInputWire {
	return &mediaAnalysisInputWire{
		S3Object: &s3RefWire{
			Bucket:  job.InputS3Bucket,
			Name:    job.InputS3Name,
			Version: job.InputS3Version,
		},
	}
}

// mediaAnalysisOperationsConfigFromDomain renders job's OperationsConfig as
// the wire shape. OperationsConfig is a required StartMediaAnalysisJobInput
// member, so every stored job has one, though its only known sub-operation
// (DetectModerationLabels) is itself optional.
func mediaAnalysisOperationsConfigFromDomain(job *MediaAnalysisJob) *mediaAnalysisOperationsConfigWire {
	cfg := &mediaAnalysisOperationsConfigWire{}
	if job.HasDetectModerationLabels {
		cfg.DetectModerationLabels = &mediaAnalysisDetectModerationLabelsConfigWire{
			MinConfidence:  job.DetectModerationLabelsMinConfidence,
			ProjectVersion: job.DetectModerationLabelsProjectVersion,
		}
	}

	return cfg
}

// mediaAnalysisOutputConfigFromDomain renders job's OutputConfig as the wire
// shape. OutputConfig is a required StartMediaAnalysisJobInput member, so
// every stored job has one.
func mediaAnalysisOutputConfigFromDomain(job *MediaAnalysisJob) *mediaAnalysisOutputConfigWire {
	return &mediaAnalysisOutputConfigWire{
		S3Bucket:    job.OutputConfigS3Bucket,
		S3KeyPrefix: job.OutputConfigS3KeyPrefix,
	}
}

// mediaAnalysisJobDescription mirrors types.MediaAnalysisJobDescription
// (types.go:1606), shared by GetMediaAnalysisJobOutput (flattened onto the
// response root, no httpPayload member) and each entry of
// ListMediaAnalysisJobsOutput.MediaAnalysisJobs. ManifestSummary/Results/
// FailureDetails/CompletionTimestamp/KmsKeyId are optional response members
// left absent: this backend runs every job to SUCCEEDED synchronously with no
// manifest/model-inference pipeline behind it, so there is no genuine result
// to report rather than fabricate one.
type mediaAnalysisJobDescription struct {
	Input             *mediaAnalysisInputWire            `json:"Input"`
	OperationsConfig  *mediaAnalysisOperationsConfigWire `json:"OperationsConfig"`
	OutputConfig      *mediaAnalysisOutputConfigWire     `json:"OutputConfig"`
	JobId             string                             `json:"JobId"` //nolint:revive,staticcheck // existing issue.
	JobName           string                             `json:"JobName,omitempty"`
	Status            string                             `json:"Status"`
	CreationTimestamp float64                            `json:"CreationTimestamp"`
}

func mediaAnalysisJobDescriptionFromDomain(job *MediaAnalysisJob) mediaAnalysisJobDescription {
	return mediaAnalysisJobDescription{
		JobId:             job.JobID,
		JobName:           job.JobName,
		Status:            job.Status,
		CreationTimestamp: epochSeconds(job.CreationTimestamp),
		Input:             mediaAnalysisInputFromDomain(job),
		OperationsConfig:  mediaAnalysisOperationsConfigFromDomain(job),
		OutputConfig:      mediaAnalysisOutputConfigFromDomain(job),
	}
}

func (h *Handler) handleGetMediaAnalysisJob(
	_ context.Context, req *getMediaAnalysisJobReq,
) (*mediaAnalysisJobDescription, error) {
	if req.JobId == "" {
		return nil, fmt.Errorf("%w: JobId is required", ErrValidation)
	}

	job, err := h.Backend.GetMediaAnalysisJob(req.JobId)
	if err != nil {
		return nil, err
	}

	desc := mediaAnalysisJobDescriptionFromDomain(job)

	return &desc, nil
}

type listMediaAnalysisJobsReq struct {
	NextToken  string `json:"NextToken"`
	MaxResults int32  `json:"MaxResults"`
}

type listMediaAnalysisJobsResp struct {
	NextToken         string                        `json:"NextToken,omitempty"`
	MediaAnalysisJobs []mediaAnalysisJobDescription `json:"MediaAnalysisJobs"`
}

func (h *Handler) handleListMediaAnalysisJobs(
	_ context.Context, req *listMediaAnalysisJobsReq,
) (*listMediaAnalysisJobsResp, error) {
	jobs, nextToken, err := h.Backend.ListMediaAnalysisJobs(req.MaxResults, req.NextToken)
	if err != nil {
		return nil, err
	}

	descriptions := make([]mediaAnalysisJobDescription, 0, len(jobs))
	for _, j := range jobs {
		descriptions = append(descriptions, mediaAnalysisJobDescriptionFromDomain(j))
	}

	return &listMediaAnalysisJobsResp{
		MediaAnalysisJobs: descriptions,
		NextToken:         nextToken,
	}, nil
}
