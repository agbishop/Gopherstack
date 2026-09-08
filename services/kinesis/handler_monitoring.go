package kinesis

import (
	"context"
	"encoding/json"
	"net/http"
)

type jsonEnhancedMonitoringReq struct {
	StreamName        string   `json:"StreamName"`
	StreamARN         string   `json:"StreamARN"`
	ShardLevelMetrics []string `json:"ShardLevelMetrics"`
}

type jsonEnhancedMonitoringResp struct {
	StreamName               string   `json:"StreamName"`
	StreamARN                string   `json:"StreamARN"`
	CurrentShardLevelMetrics []string `json:"CurrentShardLevelMetrics"`
	DesiredShardLevelMetrics []string `json:"DesiredShardLevelMetrics"`
}

func (h *Handler) handleEnableEnhancedMonitoring(
	ctx context.Context,
	_ *http.Request,
	body []byte,
) (any, error) {
	var req jsonEnhancedMonitoringReq
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, ErrInvalidArgument
	}

	streamName, ctx := resolveStreamNameAndRegion(ctx, req.StreamName, req.StreamARN, h.defaultRegion())

	out, err := h.Backend.EnableEnhancedMonitoring(ctx, &EnableEnhancedMonitoringInput{
		StreamName:        streamName,
		ShardLevelMetrics: req.ShardLevelMetrics,
	})
	if err != nil {
		return nil, err
	}

	return jsonEnhancedMonitoringResp{
		StreamName:               out.StreamName,
		StreamARN:                out.StreamARN,
		CurrentShardLevelMetrics: out.CurrentShardLevelMetrics,
		DesiredShardLevelMetrics: out.DesiredShardLevelMetrics,
	}, nil
}

func (h *Handler) handleDisableEnhancedMonitoring(
	ctx context.Context,
	_ *http.Request,
	body []byte,
) (any, error) {
	var req jsonEnhancedMonitoringReq
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, ErrInvalidArgument
	}

	streamName, ctx := resolveStreamNameAndRegion(ctx, req.StreamName, req.StreamARN, h.defaultRegion())

	out, err := h.Backend.DisableEnhancedMonitoring(ctx, &DisableEnhancedMonitoringInput{
		StreamName:        streamName,
		ShardLevelMetrics: req.ShardLevelMetrics,
	})
	if err != nil {
		return nil, err
	}

	return jsonEnhancedMonitoringResp{
		StreamName:               out.StreamName,
		StreamARN:                out.StreamARN,
		CurrentShardLevelMetrics: out.CurrentShardLevelMetrics,
		DesiredShardLevelMetrics: out.DesiredShardLevelMetrics,
	}, nil
}
