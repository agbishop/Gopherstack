package shield

import (
	"encoding/json"
	"fmt"
)

// listAttacksTimeRange mirrors the AWS Shield TimeRange wire format for ListAttacks.
// AWS sends StartTime/EndTime as {"FromInclusive": float64} / {"ToExclusive": float64}.
type listAttacksTimeRange struct {
	FromInclusive *float64 `json:"FromInclusive,omitempty"`
	ToExclusive   *float64 `json:"ToExclusive,omitempty"`
}

// listAttacksRequest is the request body for ListAttacks.
type listAttacksRequest struct {
	StartTime    *listAttacksTimeRange `json:"StartTime,omitempty"`
	EndTime      *listAttacksTimeRange `json:"EndTime,omitempty"`
	NextToken    string                `json:"NextToken,omitempty"`
	ResourceARNs []string              `json:"ResourceArns"`
	MaxResults   int                   `json:"MaxResults,omitempty"`
}

func (h *Handler) handleListAttacks(body []byte) ([]byte, error) {
	var req listAttacksRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
		}
	}

	var startTime, endTime int64
	if req.StartTime != nil && req.StartTime.FromInclusive != nil {
		startTime = int64(*req.StartTime.FromInclusive)
	}

	if req.EndTime != nil && req.EndTime.ToExclusive != nil {
		endTime = int64(*req.EndTime.ToExclusive)
	}

	attacks := h.Backend.ListAttacks(req.ResourceARNs, startTime, endTime)

	maxResults := clampMaxResults(req.MaxResults, maxAttacksPerPage)

	start, err := decodeOffsetToken(req.NextToken)
	if err != nil {
		// ListAttacks's declared error catalog has no InvalidPaginationTokenException
		// (deserializers.go's deserializeOpErrorListAttacks), unlike ListProtections/
		// ListProtectionGroups/ListResourcesInProtectionGroup which do declare it -- classify via
		// errInvalidRequest (-> InvalidParameterException, which it does declare) instead of
		// chaining decodeOffsetToken's own errInvalidPaginationToken sentinel.
		return nil, fmt.Errorf("%w: invalid NextToken", errInvalidRequest)
	}

	var nextToken string

	if start < len(attacks) {
		end := start + maxResults
		if end < len(attacks) {
			nextToken = encodeOffsetToken(end)
			attacks = attacks[start:end]
		} else {
			attacks = attacks[start:]
		}
	} else {
		attacks = nil
	}

	items := make([]map[string]any, 0, len(attacks))

	for _, a := range attacks {
		vectors := make([]map[string]any, 0, len(a.AttackVectors))
		for _, v := range a.AttackVectors {
			vectors = append(vectors, map[string]any{"VectorType": v.VectorType})
		}

		items = append(items, map[string]any{
			keyAttackID:     a.AttackID,
			keyResourceArn:  a.ResourceARN,
			keyStartTime:    floatSeconds(a.StartTime),
			keyEndTime:      floatSeconds(a.EndTime),
			"AttackVectors": vectors,
		})
	}

	resp := map[string]any{"AttackSummaries": items}

	if nextToken != "" {
		resp["NextToken"] = nextToken
	}

	return json.Marshal(resp)
}

// simulateAttackRequest is the request body for the __SimulateAttack endpoint (gap 25).
type simulateAttackRequest struct {
	ResourceArn       string   `json:"ResourceArn"`
	AttackVectorTypes []string `json:"AttackVectorTypes,omitempty"`
}

// handleSimulateAttack creates a synthetic attack record via the chaos endpoint (gap 25/10).
func (h *Handler) handleSimulateAttack(body []byte) ([]byte, error) {
	var req simulateAttackRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ResourceArn == "" {
		return nil, fmt.Errorf("%w: ResourceArn is required", errInvalidRequest)
	}

	attack, err := h.Backend.SimulateAttack(req.ResourceArn, req.AttackVectorTypes)
	if err != nil {
		return nil, err
	}

	return json.Marshal(map[string]any{
		keyAttackID:    attack.AttackID,
		keyResourceArn: attack.ResourceARN,
		keyStartTime:   floatSeconds(attack.StartTime),
		keyEndTime:     floatSeconds(attack.EndTime),
	})
}

// describeAttackRequest is the request body for DescribeAttack.
type describeAttackRequest struct {
	AttackID string `json:"AttackId"`
}

func (h *Handler) handleDescribeAttack(body []byte) ([]byte, error) {
	var req describeAttackRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.AttackID == "" {
		return nil, fmt.Errorf("%w: AttackId is required", errInvalidRequest)
	}

	attack, err := h.Backend.DescribeAttack(req.AttackID)
	if err != nil {
		return nil, err
	}

	vectors := make([]map[string]any, 0, len(attack.AttackVectors))
	for _, v := range attack.AttackVectors {
		vectors = append(vectors, map[string]any{"VectorType": v.VectorType})
	}

	counters := make([]map[string]any, 0, len(attack.AttackCounters))
	for _, c := range attack.AttackCounters {
		counters = append(counters, map[string]any{
			"Name":    c.Name,
			keyMax:    c.Max,
			"Average": c.Average,
			"Sum":     c.Sum,
			"N":       c.N,
			"Unit":    c.Unit,
		})
	}

	mitigations := make([]map[string]any, 0, len(attack.Mitigations))
	for _, m := range attack.Mitigations {
		mitigations = append(mitigations, map[string]any{"MitigationName": m.MitigationName})
	}

	return json.Marshal(map[string]any{
		"Attack": map[string]any{
			keyAttackID:      attack.AttackID,
			keyResourceArn:   attack.ResourceARN,
			keyStartTime:     floatSeconds(attack.StartTime),
			keyEndTime:       floatSeconds(attack.EndTime),
			"AttackVectors":  vectors,
			"AttackCounters": counters,
			"Mitigations":    mitigations,
		},
	})
}

func (h *Handler) handleDescribeAttackStatistics() ([]byte, error) {
	stats := h.Backend.DescribeAttackStatistics()

	// AWS returns DataItems and TimeRange at the top level (no "AttackStatistics" wrapper).
	return json.Marshal(map[string]any{
		"TimeRange": map[string]any{
			"FromInclusive": float64(stats.TimeRange.FromInclusive),
			"ToExclusive":   float64(stats.TimeRange.ToExclusive),
		},
		"DataItems": stats.DataItems,
	})
}
