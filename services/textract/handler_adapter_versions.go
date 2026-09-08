package textract

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// createAdapterVersionInput is the input for CreateAdapterVersion.
type createAdapterVersionInput struct {
	Tags               map[string]string `json:"Tags"`
	DatasetConfig      *DatasetConfig    `json:"DatasetConfig"`
	OutputConfig       *OutputConfig     `json:"OutputConfig"`
	AdapterID          string            `json:"AdapterId"`
	ClientRequestToken string            `json:"ClientRequestToken"`
	//nolint:revive,staticcheck // KMSKeyId: AWS SDK field name convention
	KMSKeyId string `json:"KMSKeyId"`
}

// createAdapterVersionResponse is the response for CreateAdapterVersion.
type createAdapterVersionResponse struct {
	AdapterID      string `json:"AdapterId"`
	AdapterVersion string `json:"AdapterVersion"`
}

func (h *Handler) handleCreateAdapterVersion(
	ctx context.Context,
	in *createAdapterVersionInput,
) (*createAdapterVersionResponse, error) {
	if in.AdapterID == "" {
		return nil, fmt.Errorf("%w: AdapterId is required", errInvalidRequest)
	}

	// DatasetConfig and OutputConfig (with OutputConfig.S3Bucket) are both
	// "This member is required" on CreateAdapterVersionInput
	// (api_op_CreateAdapterVersion.go), enforced client-side by the real
	// SDK's validateOpCreateAdapterVersionInput/validateOutputConfig.
	if in.DatasetConfig == nil {
		return nil, fmt.Errorf("%w: DatasetConfig is required", errInvalidRequest)
	}

	if in.OutputConfig == nil || in.OutputConfig.S3Bucket == "" {
		return nil, fmt.Errorf("%w: OutputConfig.S3Bucket is required", errInvalidRequest)
	}

	if m := in.DatasetConfig.ManifestS3Object; m != nil {
		if err := h.checkS3Object(ctx, m.Bucket, m.Name); err != nil {
			return nil, err
		}
	}

	var av *AdapterVersion
	var err error

	if b, ok := h.Backend.(*InMemoryBackend); ok {
		av, err = b.CreateAdapterVersionWithOptions(
			ctx,
			in.AdapterID, in.Tags,
			in.DatasetConfig, in.OutputConfig,
			in.KMSKeyId, in.ClientRequestToken,
		)
	} else {
		av, err = h.Backend.CreateAdapterVersion(ctx, in.AdapterID, in.Tags)
	}

	if err != nil {
		return nil, err
	}

	return &createAdapterVersionResponse{
		AdapterID:      av.AdapterID,
		AdapterVersion: av.AdapterVersion,
	}, nil
}

// getAdapterVersionInput is the input for GetAdapterVersion.
type getAdapterVersionInput struct {
	AdapterID      string `json:"AdapterId"`
	AdapterVersion string `json:"AdapterVersion"`
}

// getAdapterVersionResponse is the response for GetAdapterVersion.
type getAdapterVersionResponse struct {
	Tags           map[string]string `json:"Tags"`
	DatasetConfig  *DatasetConfig    `json:"DatasetConfig,omitempty"`
	OutputConfig   *OutputConfig     `json:"OutputConfig,omitempty"`
	AdapterID      string            `json:"AdapterId"`
	AdapterVersion string            `json:"AdapterVersion"`
	Status         string            `json:"Status"`
	StatusMessage  string            `json:"StatusMessage"`
	//nolint:revive,staticcheck // KMSKeyId: AWS SDK field name convention
	KMSKeyId          string                           `json:"KMSKeyId,omitempty"`
	EvaluationMetrics []AdapterVersionEvaluationMetric `json:"EvaluationMetrics,omitempty"`
	FeatureTypes      []string                         `json:"FeatureTypes"`
	CreationTime      float64                          `json:"CreationTime"`
}

func (h *Handler) handleGetAdapterVersion(
	ctx context.Context,
	in *getAdapterVersionInput,
) (*getAdapterVersionResponse, error) {
	if in.AdapterID == "" {
		return nil, fmt.Errorf("%w: AdapterId is required", errInvalidRequest)
	}

	if in.AdapterVersion == "" {
		return nil, fmt.Errorf("%w: AdapterVersion is required", errInvalidRequest)
	}

	av, err := h.Backend.GetAdapterVersion(ctx, in.AdapterID, in.AdapterVersion)
	if err != nil {
		return nil, err
	}

	return &getAdapterVersionResponse{
		AdapterID:         av.AdapterID,
		AdapterVersion:    av.AdapterVersion,
		CreationTime:      awstime.Epoch(av.CreationTime),
		FeatureTypes:      av.FeatureTypes,
		Status:            av.Status,
		StatusMessage:     av.StatusMessage,
		Tags:              av.Tags,
		DatasetConfig:     av.DatasetConfig,
		OutputConfig:      av.OutputConfig,
		KMSKeyId:          av.KMSKeyId,
		EvaluationMetrics: av.EvaluationMetrics,
	}, nil
}

// listAdapterVersionsDefaultPageSize is used when
// ListAdapterVersionsInput.MaxResults is unset or non-positive.
const listAdapterVersionsDefaultPageSize = 1000

// listAdapterVersionsInput is the input for ListAdapterVersions.
// AdapterId is an optional filter, not a required identifier: real AWS's
// ListAdapterVersionsInput marks no member required, and omitting AdapterId
// lists versions across every adapter (see [Handler.handleListAdapterVersions]).
// AfterCreationTime / BeforeCreationTime are epoch-seconds (JSON numbers),
// matching the awsjson1.1 unixTimestamp wire format -- see pkgs/awstime's
// package doc.
type listAdapterVersionsInput struct {
	AdapterID          string  `json:"AdapterId"`
	NextToken          string  `json:"NextToken"`
	AfterCreationTime  float64 `json:"AfterCreationTime"`
	BeforeCreationTime float64 `json:"BeforeCreationTime"`
	MaxResults         int     `json:"MaxResults"`
}

// listAdapterVersionsResponse is the response for ListAdapterVersions. There
// is no top-level AdapterId in the real SDK's ListAdapterVersionsOutput --
// gopherstack previously invented one; each entry inside AdapterVersions
// carries its own AdapterId instead, matching types.AdapterVersionOverview.
type listAdapterVersionsResponse struct {
	NextToken       string                  `json:"NextToken,omitempty"`
	AdapterVersions []adapterVersionSummary `json:"AdapterVersions"`
}

type adapterVersionSummary struct {
	AdapterID      string   `json:"AdapterId"`
	AdapterVersion string   `json:"AdapterVersion"`
	Status         string   `json:"Status"`
	StatusMessage  string   `json:"StatusMessage,omitempty"`
	FeatureTypes   []string `json:"FeatureTypes"`
	CreationTime   float64  `json:"CreationTime"`
}

func (h *Handler) handleListAdapterVersions(
	ctx context.Context,
	in *listAdapterVersionsInput,
) (*listAdapterVersionsResponse, error) {
	versions, err := h.Backend.ListAdapterVersions(ctx, in.AdapterID)
	if err != nil {
		return nil, err
	}

	filtered := make([]AdapterVersion, 0, len(versions))

	for _, av := range versions {
		if in.AfterCreationTime > 0 && awstime.Epoch(av.CreationTime) <= in.AfterCreationTime {
			continue
		}

		if in.BeforeCreationTime > 0 && awstime.Epoch(av.CreationTime) >= in.BeforeCreationTime {
			continue
		}

		filtered = append(filtered, av)
	}

	pg := page.New(filtered, in.NextToken, in.MaxResults, listAdapterVersionsDefaultPageSize)

	summaries := make([]adapterVersionSummary, 0, len(pg.Data))
	for _, av := range pg.Data {
		summaries = append(summaries, adapterVersionSummary{
			AdapterID:      av.AdapterID,
			AdapterVersion: av.AdapterVersion,
			CreationTime:   awstime.Epoch(av.CreationTime),
			FeatureTypes:   av.FeatureTypes,
			Status:         av.Status,
			StatusMessage:  av.StatusMessage,
		})
	}

	return &listAdapterVersionsResponse{
		AdapterVersions: summaries,
		NextToken:       pg.Next,
	}, nil
}

// deleteAdapterVersionInput is the input for DeleteAdapterVersion.
type deleteAdapterVersionInput struct {
	AdapterID      string `json:"AdapterId"`
	AdapterVersion string `json:"AdapterVersion"`
}

func (h *Handler) handleDeleteAdapterVersion(
	ctx context.Context,
	in *deleteAdapterVersionInput,
) (*emptyResponse, error) {
	if in.AdapterID == "" {
		return nil, fmt.Errorf("%w: AdapterId is required", errInvalidRequest)
	}

	if in.AdapterVersion == "" {
		return nil, fmt.Errorf("%w: AdapterVersion is required", errInvalidRequest)
	}

	if err := h.Backend.DeleteAdapterVersion(ctx, in.AdapterID, in.AdapterVersion); err != nil {
		return nil, err
	}

	return &emptyResponse{}, nil
}
