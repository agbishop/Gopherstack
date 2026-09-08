package textract

import (
	"context"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// createAdapterInput is the input for CreateAdapter.
type createAdapterInput struct {
	Tags               map[string]string `json:"Tags"`
	AdapterName        string            `json:"AdapterName"`
	AutoUpdate         string            `json:"AutoUpdate"`
	Description        string            `json:"Description"`
	ClientRequestToken string            `json:"ClientRequestToken"`
	FeatureTypes       []string          `json:"FeatureTypes"`
}

// createAdapterResponse is the response for CreateAdapter.
type createAdapterResponse struct {
	AdapterID string `json:"AdapterId"`
}

func (h *Handler) handleCreateAdapter(
	ctx context.Context,
	in *createAdapterInput,
) (*createAdapterResponse, error) {
	if in.AdapterName == "" {
		return nil, fmt.Errorf("%w: AdapterName is required", errInvalidRequest)
	}

	var adapter *Adapter
	var err error

	if b, ok := h.Backend.(*InMemoryBackend); ok {
		adapter, err = b.CreateAdapterWithToken(
			ctx,
			in.AdapterName, in.Description, in.AutoUpdate,
			in.FeatureTypes, in.Tags, in.ClientRequestToken,
		)
	} else {
		adapter, err = h.Backend.CreateAdapter(
			ctx, in.AdapterName, in.Description, in.AutoUpdate, in.FeatureTypes, in.Tags,
		)
	}

	if err != nil {
		return nil, err
	}

	return &createAdapterResponse{AdapterID: adapter.AdapterID}, nil
}

// getAdapterInput is the input for GetAdapter.
type getAdapterInput struct {
	AdapterID string `json:"AdapterId"`
}

// getAdapterResponse is the response for GetAdapter.
type getAdapterResponse struct {
	Tags         map[string]string `json:"Tags"`
	AdapterID    string            `json:"AdapterId"`
	AdapterName  string            `json:"AdapterName"`
	AutoUpdate   string            `json:"AutoUpdate"`
	Description  string            `json:"Description"`
	FeatureTypes []string          `json:"FeatureTypes"`
	CreationTime float64           `json:"CreationTime"`
}

func (h *Handler) handleGetAdapter(
	ctx context.Context,
	in *getAdapterInput,
) (*getAdapterResponse, error) {
	if in.AdapterID == "" {
		return nil, fmt.Errorf("%w: AdapterId is required", errInvalidRequest)
	}

	adapter, err := h.Backend.GetAdapter(ctx, in.AdapterID)
	if err != nil {
		return nil, err
	}

	return &getAdapterResponse{
		AdapterID:    adapter.AdapterID,
		AdapterName:  adapter.AdapterName,
		AutoUpdate:   adapter.AutoUpdate,
		CreationTime: awstime.Epoch(adapter.CreationTime),
		Description:  adapter.Description,
		FeatureTypes: adapter.FeatureTypes,
		Tags:         adapter.Tags,
	}, nil
}

// updateAdapterInput is the input for UpdateAdapter. AdapterName and
// Description are *string: the real UpdateAdapterInput sends both only when
// present (serializers.go's `!= nil` guards), so an omitted field must leave
// the existing value unchanged while an explicit "" clears it.
type updateAdapterInput struct {
	AdapterName *string `json:"AdapterName"`
	Description *string `json:"Description"`
	AdapterID   string  `json:"AdapterId"`
	AutoUpdate  string  `json:"AutoUpdate"`
}

// updateAdapterResponse is the response for UpdateAdapter. Real AWS's
// UpdateAdapterOutput has no Tags member (unlike GetAdapterOutput) --
// gopherstack previously invented one.
type updateAdapterResponse struct {
	AdapterID    string   `json:"AdapterId"`
	AdapterName  string   `json:"AdapterName"`
	AutoUpdate   string   `json:"AutoUpdate"`
	Description  string   `json:"Description"`
	FeatureTypes []string `json:"FeatureTypes"`
	CreationTime float64  `json:"CreationTime"`
}

func (h *Handler) handleUpdateAdapter(
	ctx context.Context,
	in *updateAdapterInput,
) (*updateAdapterResponse, error) {
	if in.AdapterID == "" {
		return nil, fmt.Errorf("%w: AdapterId is required", errInvalidRequest)
	}

	adapter, err := h.Backend.UpdateAdapter(ctx, in.AdapterID, in.AdapterName, in.Description, in.AutoUpdate)
	if err != nil {
		return nil, err
	}

	return &updateAdapterResponse{
		AdapterID:    adapter.AdapterID,
		AdapterName:  adapter.AdapterName,
		AutoUpdate:   adapter.AutoUpdate,
		CreationTime: awstime.Epoch(adapter.CreationTime),
		Description:  adapter.Description,
		FeatureTypes: adapter.FeatureTypes,
	}, nil
}

// listAdaptersDefaultPageSize is used when ListAdaptersInput.MaxResults is
// unset or non-positive.
const listAdaptersDefaultPageSize = 1000

// listAdaptersInput is the input for ListAdapters. AfterCreationTime /
// BeforeCreationTime are epoch-seconds (JSON numbers), matching the
// awsjson1.1 unixTimestamp wire format -- see pkgs/awstime's package doc.
type listAdaptersInput struct {
	NextToken          string  `json:"NextToken"`
	AfterCreationTime  float64 `json:"AfterCreationTime"`
	BeforeCreationTime float64 `json:"BeforeCreationTime"`
	MaxResults         int     `json:"MaxResults"`
}

// listAdaptersResponse is the response for ListAdapters.
type listAdaptersResponse struct {
	NextToken string           `json:"NextToken,omitempty"`
	Adapters  []adapterSummary `json:"Adapters"`
}

type adapterSummary struct {
	AdapterID    string   `json:"AdapterId"`
	AdapterName  string   `json:"AdapterName"`
	FeatureTypes []string `json:"FeatureTypes"`
	CreationTime float64  `json:"CreationTime"`
}

func (h *Handler) handleListAdapters(
	ctx context.Context,
	in *listAdaptersInput,
) (*listAdaptersResponse, error) {
	adapters := h.Backend.ListAdapters(ctx)

	filtered := make([]Adapter, 0, len(adapters))

	for _, a := range adapters {
		if in.AfterCreationTime > 0 && awstime.Epoch(a.CreationTime) <= in.AfterCreationTime {
			continue
		}

		if in.BeforeCreationTime > 0 && awstime.Epoch(a.CreationTime) >= in.BeforeCreationTime {
			continue
		}

		filtered = append(filtered, a)
	}

	pg := page.New(filtered, in.NextToken, in.MaxResults, listAdaptersDefaultPageSize)

	summaries := make([]adapterSummary, 0, len(pg.Data))
	for _, a := range pg.Data {
		summaries = append(summaries, adapterSummary{
			AdapterID:    a.AdapterID,
			AdapterName:  a.AdapterName,
			CreationTime: awstime.Epoch(a.CreationTime),
			FeatureTypes: a.FeatureTypes,
		})
	}

	return &listAdaptersResponse{Adapters: summaries, NextToken: pg.Next}, nil
}

// deleteAdapterInput is the input for DeleteAdapter.
type deleteAdapterInput struct {
	AdapterID string `json:"AdapterId"`
}

func (h *Handler) handleDeleteAdapter(
	ctx context.Context,
	in *deleteAdapterInput,
) (*emptyResponse, error) {
	if in.AdapterID == "" {
		return nil, fmt.Errorf("%w: AdapterId is required", errInvalidRequest)
	}

	if err := h.Backend.DeleteAdapter(ctx, in.AdapterID); err != nil {
		return nil, err
	}

	return &emptyResponse{}, nil
}
