package ce

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/collections"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

type groupBySpec struct {
	Type string `json:"Type"`
	Key  string `json:"Key"`
}

type getCostAndUsageInput struct {
	Filter        *ceExpression     `json:"Filter"`
	TimePeriod    map[string]string `json:"TimePeriod"`
	Granularity   string            `json:"Granularity"`
	NextPageToken string            `json:"NextPageToken"`
	Metrics       []string          `json:"Metrics"`
	GroupBy       []groupBySpec     `json:"GroupBy"`
}

// getCostAndUsageOutput's GroupDefinitions field (the groups specified by the request's
// GroupBy, echoed back -- see aws-sdk-go-v2/service/costexplorer's GetCostAndUsageOutput)
// was previously missing entirely.
type getCostAndUsageOutput struct {
	NextPageToken            string         `json:"NextPageToken,omitempty"`
	ResultsByTime            []ResultByTime `json:"ResultsByTime"`
	GroupDefinitions         []groupBySpec  `json:"GroupDefinitions"`
	DimensionValueAttributes []any          `json:"DimensionValueAttributes"`
}

// resultByTimeKey returns the unique cursor key for a ResultByTime page --
// its bucket start date, unique because buildTimeBuckets never produces two
// buckets with the same start.
func resultByTimeKey(r ResultByTime) string {
	return r.TimePeriod[timePeriodKeyStart]
}

func (h *Handler) handleGetCostAndUsage(
	_ context.Context,
	in *getCostAndUsageInput,
) (*getCostAndUsageOutput, error) {
	if in.Granularity == "" {
		return nil, fmt.Errorf("%w: Granularity is required", ErrValidation)
	}

	// Granularity is a real Smithy enum (botocore ce/2017-10-25's Granularity
	// shape: DAILY/MONTHLY/HOURLY), not an arbitrary non-empty string --
	// buildTimeBuckets silently bucketed anything else as DAILY.
	switch in.Granularity {
	case granularityDaily, granularityMonthly, granularityHourly:
	default:
		return nil, fmt.Errorf("%w: Granularity must be one of DAILY, MONTHLY, HOURLY", ErrValidation)
	}

	// Real GetCostAndUsageInput requires TimePeriod (a DateInterval whose Start
	// and End members are themselves both required) and Metrics -- see
	// api_op_GetCostAndUsage.go and types.DateInterval. An earlier revision
	// silently defaulted a missing/partial TimePeriod to defaultStartDate/
	// defaultEndDate and never checked Metrics at all, so a request missing
	// either required member (which real AWS rejects with ValidationError)
	// instead got a permissive, silently-defaulted 200.
	if in.TimePeriod == nil {
		return nil, fmt.Errorf("%w: TimePeriod is required", ErrValidation)
	}

	start := in.TimePeriod["Start"]
	end := in.TimePeriod["End"]

	if start == "" {
		return nil, fmt.Errorf("%w: TimePeriod.Start is required", ErrValidation)
	}

	if end == "" {
		return nil, fmt.Errorf("%w: TimePeriod.End is required", ErrValidation)
	}

	// The wire model constrains Start/End to YearMonthDay
	// ((\d{4}-\d{2}-\d{2})(T\d{2}:\d{2}:\d{2}Z)?) -- buildTimeBuckets silently
	// returned zero buckets (a 200 with an empty result) for a value it
	// couldn't parse instead of rejecting it.
	if _, err := time.Parse("2006-01-02", start); err != nil {
		return nil, fmt.Errorf("%w: TimePeriod.Start must be a date in YYYY-MM-DD format", ErrValidation)
	}

	if _, err := time.Parse("2006-01-02", end); err != nil {
		return nil, fmt.Errorf("%w: TimePeriod.End must be a date in YYYY-MM-DD format", ErrValidation)
	}

	if len(in.Metrics) == 0 {
		return nil, fmt.Errorf("%w: Metrics is required", ErrValidation)
	}

	granularity := in.Granularity

	groupBy := make([]GroupBySpec, len(in.GroupBy))
	for i, g := range in.GroupBy {
		groupBy[i] = GroupBySpec(g)
	}

	results := h.Backend.GetCostAndUsageFiltered(
		start, end, granularity, in.Metrics, groupBy, in.Filter,
	)

	page, nextToken := paginateList(results, 0, in.NextPageToken, resultByTimeKey)

	return &getCostAndUsageOutput{
		ResultsByTime:            page,
		NextPageToken:            nextToken,
		GroupDefinitions:         in.GroupBy,
		DimensionValueAttributes: []any{},
	}, nil
}

type dimensionValue struct {
	Attributes map[string]string `json:"Attributes,omitempty"`
	Value      string            `json:"Value"`
}

type getDimensionValuesInput struct {
	Filter        *ceExpression      `json:"Filter"`
	TimePeriod    map[string]string  `json:"TimePeriod"`
	Dimension     string             `json:"Dimension"`
	SearchString  string             `json:"SearchString"`
	Context       string             `json:"Context"`
	NextPageToken string             `json:"NextPageToken"`
	SortBy        []ceSortDefinition `json:"SortBy"`
	MaxResults    int                `json:"MaxResults"`
}

type getDimensionValuesOutput struct {
	NextPageToken   string           `json:"NextPageToken,omitempty"`
	DimensionValues []dimensionValue `json:"DimensionValues"`
	ReturnSize      int              `json:"ReturnSize"`
	TotalSize       int              `json:"TotalSize"`
}

func (h *Handler) handleGetDimensionValues(
	_ context.Context,
	in *getDimensionValuesInput,
) (*getDimensionValuesOutput, error) {
	if in.Dimension == "" {
		return nil, fmt.Errorf("%w: Dimension is required", ErrValidation)
	}

	// Real GetDimensionValuesInput requires TimePeriod. This emulator's dimension
	// values are derived from the whole cost ledger rather than narrowed to
	// TimePeriod (real AWS does narrow by it; there is no per-entry-in-range
	// filtering here -- see gaps), so this is a presence check only, matching
	// GetCostAndUsage's required-field-gap fix.
	if in.TimePeriod == nil || in.TimePeriod[timePeriodKeyStart] == "" || in.TimePeriod[timePeriodKeyEnd] == "" {
		return nil, fmt.Errorf("%w: TimePeriod is required", ErrValidation)
	}

	// Context selects between COST_AND_USAGE/RESERVATIONS/SAVINGS_PLANS
	// dimension namespaces; this emulator's ledger models one flat dimension
	// space shared across all three, so Context is validated (an unrecognized
	// value real AWS rejects) but does not change which dimensions resolve.
	switch in.Context {
	case "", "COST_AND_USAGE", "RESERVATIONS", "SAVINGS_PLANS":
	default:
		return nil, fmt.Errorf("%w: Context must be one of COST_AND_USAGE, RESERVATIONS, SAVINGS_PLANS", ErrValidation)
	}

	var vals []string
	if in.Filter != nil && in.Filter.Dimensions != nil && in.Filter.Dimensions.Key != "" {
		vals = h.Backend.GetDimensionValuesFiltered(
			in.Dimension, in.Filter.Dimensions.Key, in.Filter.Dimensions.Values,
		)
	} else {
		vals = h.Backend.GetDimensionValues(in.Dimension)
	}

	if in.SearchString != "" {
		filtered := vals[:0]
		search := strings.ToLower(in.SearchString)

		for _, v := range vals {
			if strings.Contains(strings.ToLower(v), search) {
				filtered = append(filtered, v)
			}
		}

		vals = filtered
	}

	if len(in.SortBy) > 0 {
		vals = sortDimensionValuesByCost(h.Backend, in.Dimension, vals, in.SortBy[0])
	}

	totalSize := len(vals)

	// paginateOrdered, not paginateList: vals may already be in SortBy's
	// cost-based order (sortDimensionValuesByCost), which re-sorting by value
	// would discard.
	page, nextToken := paginateOrdered(vals, in.MaxResults, in.NextPageToken, func(v string) string { return v })

	items := make([]dimensionValue, 0, len(page))
	for _, v := range page {
		items = append(items, dimensionValue{Value: v})
	}

	return &getDimensionValuesOutput{
		DimensionValues: items,
		NextPageToken:   nextToken,
		ReturnSize:      len(items),
		TotalSize:       totalSize,
	}, nil
}

// sortDimensionValuesByCost orders dimension values by the total cost metric
// (real GetDimensionValues SortBy keys are cost/usage metrics such as
// BlendedCost/UnblendedCost) each value accounts for in the ledger, honoring
// SortOrder. Ties keep the existing (alphabetical) order.
func sortDimensionValuesByCost(
	backend *InMemoryBackend, dimension string, vals []string, sortBy ceSortDefinition,
) []string {
	ordered := make([]string, len(vals))
	copy(ordered, vals)

	costs := make(map[string]float64, len(ordered))
	for _, v := range ordered {
		costs[v] = backend.DimensionValueCost(dimension, v, sortBy.Key)
	}

	desc := sortDescending(sortBy.SortOrder)
	sort.SliceStable(ordered, func(i, j int) bool {
		if desc {
			return costs[ordered[i]] > costs[ordered[j]]
		}

		return costs[ordered[i]] < costs[ordered[j]]
	})

	return ordered
}

type getTagsInput struct {
	TimePeriod    map[string]string  `json:"TimePeriod"`
	TagKey        string             `json:"TagKey"`
	SearchString  string             `json:"SearchString"`
	Filter        *ceExpression      `json:"Filter"`
	NextPageToken string             `json:"NextPageToken"`
	SortBy        []ceSortDefinition `json:"SortBy"`
	MaxResults    int                `json:"MaxResults"`
}

type getTagsOutput struct {
	NextPageToken string   `json:"NextPageToken,omitempty"`
	Tags          []string `json:"Tags"`
	ReturnSize    int      `json:"ReturnSize"`
	TotalSize     int      `json:"TotalSize"`
}

func (h *Handler) handleGetTags(
	_ context.Context,
	in *getTagsInput,
) (*getTagsOutput, error) {
	// Real GetTagsInput requires TimePeriod. As with GetDimensionValues, this
	// emulator derives tag keys/values from the whole ledger rather than
	// narrowing by TimePeriod, so this is a presence check only.
	if in.TimePeriod == nil || in.TimePeriod[timePeriodKeyStart] == "" || in.TimePeriod[timePeriodKeyEnd] == "" {
		return nil, fmt.Errorf("%w: TimePeriod is required", ErrValidation)
	}

	var constraintKey string

	var constraintValues []string

	if in.Filter != nil && in.Filter.Tags != nil && in.Filter.Tags.Key != "" {
		constraintKey = in.Filter.Tags.Key
		constraintValues = in.Filter.Tags.Values
	}

	var tags []string
	if in.TagKey != "" {
		tags = h.Backend.GetTagValuesFiltered(in.TagKey, constraintKey, constraintValues)
	} else {
		tags = h.Backend.GetTagKeysFiltered(constraintKey, constraintValues)
	}

	if in.SearchString != "" {
		filtered := tags[:0]
		search := strings.ToLower(in.SearchString)

		for _, t := range tags {
			if strings.Contains(strings.ToLower(t), search) {
				filtered = append(filtered, t)
			}
		}

		tags = filtered
	}

	if len(in.SortBy) > 0 && in.TagKey != "" {
		tags = sortTagValuesByCost(h.Backend, in.TagKey, tags, in.SortBy[0])
	}

	if tags == nil {
		tags = []string{}
	}

	totalSize := len(tags)

	// paginateOrdered: tags may already be in SortBy's cost-based order
	// (sortTagValuesByCost).
	page, nextToken := paginateOrdered(tags, in.MaxResults, in.NextPageToken, func(v string) string { return v })

	return &getTagsOutput{
		Tags:          page,
		NextPageToken: nextToken,
		ReturnSize:    len(page),
		TotalSize:     totalSize,
	}, nil
}

// sortTagValuesByCost orders tag values by the total cost metric attributed
// to that tag value in the ledger, honoring SortOrder. Only applies when
// listing values for a specific TagKey -- sorting tag *keys* (TagKey unset)
// by a cost metric has no well-defined per-key total to use, so that case is
// left in its existing (alphabetical) order rather than fabricating one.
func sortTagValuesByCost(
	backend *InMemoryBackend, tagKey string, vals []string, sortBy ceSortDefinition,
) []string {
	ordered := make([]string, len(vals))
	copy(ordered, vals)

	costs := make(map[string]float64, len(ordered))
	for _, v := range ordered {
		costs[v] = backend.TagValueCost(tagKey, v, sortBy.Key)
	}

	desc := sortDescending(sortBy.SortOrder)
	sort.SliceStable(ordered, func(i, j int) bool {
		if desc {
			return costs[ordered[i]] > costs[ordered[j]]
		}

		return costs[ordered[i]] < costs[ordered[j]]
	})

	return ordered
}

type getCostForecastInput struct {
	Filter                  *ceExpression     `json:"Filter"`
	TimePeriod              map[string]string `json:"TimePeriod"`
	Granularity             string            `json:"Granularity"`
	Metric                  string            `json:"Metric"`
	PredictionIntervalLevel int               `json:"PredictionIntervalLevel"`
}

// getCostForecastOutput.Total is field-diffed against real AWS CE's
// GetCostForecastOutput: the member is *types.MetricValue (Amount/Unit), not
// a ForecastResult (MeanValue/PredictionIntervalLowerBound/
// PredictionIntervalUpperBound/TimePeriod) -- that shape belongs to each
// entry of ForecastResultsByTime, not to Total. A prior revision used the
// ForecastResult shape for Total too, so a real client's typed
// Total.Amount/.Unit were always nil regardless of the computed forecast.
type getCostForecastOutput struct {
	Total                 *MetricValue     `json:"Total,omitempty"`
	ForecastResultsByTime []ForecastResult `json:"ForecastResultsByTime"`
}

func (h *Handler) handleGetCostForecast(
	_ context.Context,
	in *getCostForecastInput,
) (*getCostForecastOutput, error) {
	start, end := defaultForecastStart, defaultForecastEnd
	if in.TimePeriod != nil {
		if s := in.TimePeriod["Start"]; s != "" {
			start = s
		}
		if e := in.TimePeriod["End"]; e != "" {
			end = e
		}
	}

	granularity := in.Granularity
	if granularity == "" {
		granularity = defaultGranularity
	}

	level := in.PredictionIntervalLevel
	if level == 0 {
		level = 80
	}

	buckets, totalMean, _, _ := h.Backend.GetForecastByTime(
		start,
		end,
		granularity,
		in.Metric,
		level,
		serviceDimensionFilter(in.Filter),
	)

	return &getCostForecastOutput{
		Total: &MetricValue{
			Amount: fmt.Sprintf("%.4f", totalMean),
			Unit:   metricUnit(in.Metric),
		},
		ForecastResultsByTime: buckets,
	}, nil
}

type getUsageForecastInput struct {
	Filter                  *ceExpression     `json:"Filter"`
	TimePeriod              map[string]string `json:"TimePeriod"`
	Granularity             string            `json:"Granularity"`
	Metric                  string            `json:"Metric"`
	PredictionIntervalLevel int               `json:"PredictionIntervalLevel"`
}

// getUsageForecastOutput.Total has the same real shape as
// getCostForecastOutput.Total (*types.MetricValue, not ForecastResult) --
// see that type's doc comment.
type getUsageForecastOutput struct {
	Total                 *MetricValue     `json:"Total,omitempty"`
	ForecastResultsByTime []ForecastResult `json:"ForecastResultsByTime"`
}

func (h *Handler) handleGetUsageForecast(
	_ context.Context,
	in *getUsageForecastInput,
) (*getUsageForecastOutput, error) {
	start, end := defaultForecastStart, defaultForecastEnd
	if in.TimePeriod != nil {
		if s := in.TimePeriod["Start"]; s != "" {
			start = s
		}
		if e := in.TimePeriod["End"]; e != "" {
			end = e
		}
	}

	granularity := in.Granularity
	if granularity == "" {
		granularity = defaultGranularity
	}

	level := in.PredictionIntervalLevel
	if level == 0 {
		level = 80
	}

	buckets, totalMean, _, _ := h.Backend.GetForecastByTime(
		start,
		end,
		granularity,
		in.Metric,
		level,
		serviceDimensionFilter(in.Filter),
	)

	return &getUsageForecastOutput{
		Total: &MetricValue{
			Amount: fmt.Sprintf("%.4f", totalMean),
			Unit:   metricUnit(in.Metric),
		},
		ForecastResultsByTime: buckets,
	}, nil
}

type getApproximateUsageRecordsInput struct {
	ApproximationDimension string   `json:"ApproximationDimension"`
	Granularity            string   `json:"Granularity"`
	Services               []string `json:"Services"`
}

type getApproximateUsageRecordsOutput struct {
	LookbackPeriod map[string]string `json:"LookbackPeriod,omitempty"`
	// Services and TotalRecords are wire-typed as JSON numbers in real AWS CE
	// (NonNegativeLong), not strings -- see aws-sdk-go-v2/service/costexplorer's
	// GetApproximateUsageRecordsOutput (Services map[string]int64, TotalRecords int64).
	Services     map[string]int64 `json:"Services"`
	TotalRecords int64            `json:"TotalRecords"`
}

func (h *Handler) handleGetApproximateUsageRecords(
	_ context.Context,
	in *getApproximateUsageRecordsInput,
) (*getApproximateUsageRecordsOutput, error) {
	if in.ApproximationDimension == "" {
		return nil, fmt.Errorf("%w: ApproximationDimension is required", ErrValidation)
	}

	if in.Granularity == "" {
		return nil, fmt.Errorf("%w: Granularity is required", ErrValidation)
	}

	start, end, perService, total := h.Backend.GetApproximateUsageRecords(in.Services)

	return &getApproximateUsageRecordsOutput{
		LookbackPeriod: map[string]string{timePeriodKeyStart: start, timePeriodKeyEnd: end},
		Services:       perService,
		TotalRecords:   total,
	}, nil
}

// getCostAndUsageComparisonsInput's field names/types are field-diffed against real AWS
// CE's GetCostAndUsageComparisonsInput: the request field is BaselineTimePeriod (not
// "BaseTimePeriod"), there is no Granularity member on this op, and the metric member is
// the singular, required MetricForComparison string (not a "Metrics" array).
type getCostAndUsageComparisonsInput struct {
	Filter               *ceExpression     `json:"Filter"`
	BaselineTimePeriod   map[string]string `json:"BaselineTimePeriod"`
	ComparisonTimePeriod map[string]string `json:"ComparisonTimePeriod"`
	MetricForComparison  string            `json:"MetricForComparison"`
	NextPageToken        string            `json:"NextPageToken"`
	GroupBy              []groupBySpec     `json:"GroupBy"`
	MaxResults           int               `json:"MaxResults"`
}

// comparisonMetricValue mirrors aws-sdk-go-v2/service/costexplorer/types'
// ComparisonMetricValue exactly.
type comparisonMetricValue struct {
	BaselineTimePeriodAmount   string `json:"BaselineTimePeriodAmount,omitempty"`
	ComparisonTimePeriodAmount string `json:"ComparisonTimePeriodAmount,omitempty"`
	Difference                 string `json:"Difference,omitempty"`
}

// costAndUsageComparison mirrors aws-sdk-go-v2/service/costexplorer/types'
// CostAndUsageComparison (CostAndUsageSelector -- the Expression identifying
// which group this entry represents, set only when GroupBy narrowed the
// comparison to a single dimension value; Metrics -- a map of metric name to
// comparison value).
type costAndUsageComparison struct {
	CostAndUsageSelector *ceExpression                    `json:"CostAndUsageSelector,omitempty"`
	Metrics              map[string]comparisonMetricValue `json:"Metrics,omitempty"`
}

// getCostAndUsageComparisonsOutput's field names/types are field-diffed against real AWS
// CE's GetCostAndUsageComparisonsOutput: CostAndUsageComparisons (not the previously
// invented "CostAndUsages"), and TotalCostAndUsage is a map keyed by metric name (not an
// array).
type getCostAndUsageComparisonsOutput struct {
	TotalCostAndUsage       map[string]comparisonMetricValue `json:"TotalCostAndUsage"`
	NextPageToken           string                           `json:"NextPageToken,omitempty"`
	CostAndUsageComparisons []costAndUsageComparison         `json:"CostAndUsageComparisons"`
}

// metricTotalForPeriod sums metric across the cost ledger for [start, end),
// narrowed to serviceFilter (GetCostAndUsageComparisonsInput.Filter's SERVICE
// dimension, when present), by reusing the same DAILY-bucketed aggregation
// GetCostAndUsage uses, so comparisons are derived from real ledger state
// rather than a hardcoded literal.
func metricTotalForPeriod(h *Handler, start, end, metric string, serviceFilter []string) float64 {
	var total float64

	for _, r := range h.Backend.GetCostAndUsage(start, end, "DAILY", []string{metric}, nil, serviceFilter) {
		if mv, ok := r.Total[metric]; ok {
			if v, err := strconv.ParseFloat(mv.Amount, 64); err == nil {
				total += v
			}
		}
	}

	return total
}

// groupedMetricTotalsForPeriod sums metric across the ledger for [start, end),
// narrowed by serviceFilter and grouped by the single dimension groupKey
// (the same DIMENSION set extractGroupKeys models: SERVICE/REGION/USAGE_TYPE/
// LINKED_ACCOUNT). Gives GetCostAndUsageComparisonsInput.GroupBy a real,
// per-group breakdown instead of always collapsing to one aggregate entry.
func groupedMetricTotalsForPeriod(
	h *Handler, start, end, metric, groupKey string, serviceFilter []string,
) map[string]float64 {
	totals := make(map[string]float64)

	groupBy := []GroupBySpec{{Type: "DIMENSION", Key: groupKey}}
	for _, r := range h.Backend.GetCostAndUsage(start, end, "DAILY", []string{metric}, groupBy, serviceFilter) {
		for _, g := range r.Groups {
			if len(g.Keys) == 0 {
				continue
			}

			if mv, ok := g.Metrics[metric]; ok {
				if v, err := strconv.ParseFloat(mv.Amount, 64); err == nil {
					totals[g.Keys[0]] += v
				}
			}
		}
	}

	return totals
}

func comparisonMetricEntry(baseline, comparison float64, metric string) map[string]comparisonMetricValue {
	return map[string]comparisonMetricValue{
		metric: {
			BaselineTimePeriodAmount:   fmt.Sprintf("%.4f", baseline),
			ComparisonTimePeriodAmount: fmt.Sprintf("%.4f", comparison),
			Difference:                 fmt.Sprintf("%.4f", comparison-baseline),
		},
	}
}

// costAndUsageComparisonKey returns the pagination cursor key for a
// costAndUsageComparison: the single group value its CostAndUsageSelector
// narrows to (unique per group, since it comes from collections.SortedKeys),
// or "" for the single ungrouped aggregate entry.
func costAndUsageComparisonKey(c costAndUsageComparison) string {
	if c.CostAndUsageSelector == nil || c.CostAndUsageSelector.Dimensions == nil ||
		len(c.CostAndUsageSelector.Dimensions.Values) == 0 {
		return ""
	}

	return c.CostAndUsageSelector.Dimensions.Values[0]
}

func (h *Handler) handleGetCostAndUsageComparisons(
	_ context.Context,
	in *getCostAndUsageComparisonsInput,
) (*getCostAndUsageComparisonsOutput, error) {
	if in.BaselineTimePeriod == nil {
		return nil, fmt.Errorf("%w: BaselineTimePeriod is required", ErrValidation)
	}

	if in.ComparisonTimePeriod == nil {
		return nil, fmt.Errorf("%w: ComparisonTimePeriod is required", ErrValidation)
	}

	if in.MetricForComparison == "" {
		return nil, fmt.Errorf("%w: MetricForComparison is required", ErrValidation)
	}

	baseStart, baseEnd := in.BaselineTimePeriod["Start"], in.BaselineTimePeriod["End"]
	cmpStart, cmpEnd := in.ComparisonTimePeriod["Start"], in.ComparisonTimePeriod["End"]
	serviceFilter := serviceDimensionFilter(in.Filter)

	var comparisons []costAndUsageComparison

	if len(in.GroupBy) > 0 {
		groupKey := in.GroupBy[0].Key
		baselineByGroup := groupedMetricTotalsForPeriod(
			h,
			baseStart,
			baseEnd,
			in.MetricForComparison,
			groupKey,
			serviceFilter,
		)
		comparisonByGroup := groupedMetricTotalsForPeriod(
			h,
			cmpStart,
			cmpEnd,
			in.MetricForComparison,
			groupKey,
			serviceFilter,
		)

		groupValues := make(map[string]struct{}, len(baselineByGroup)+len(comparisonByGroup))
		for k := range baselineByGroup {
			groupValues[k] = struct{}{}
		}

		for k := range comparisonByGroup {
			groupValues[k] = struct{}{}
		}

		for _, gv := range collections.SortedKeys(groupValues) {
			comparisons = append(comparisons, costAndUsageComparison{
				CostAndUsageSelector: &ceExpression{
					Dimensions: &ceDimensionValues{Key: groupKey, Values: []string{gv}},
				},
				Metrics: comparisonMetricEntry(baselineByGroup[gv], comparisonByGroup[gv], in.MetricForComparison),
			})
		}
	} else {
		baseline := metricTotalForPeriod(h, baseStart, baseEnd, in.MetricForComparison, serviceFilter)
		comparison := metricTotalForPeriod(h, cmpStart, cmpEnd, in.MetricForComparison, serviceFilter)
		metrics := comparisonMetricEntry(baseline, comparison, in.MetricForComparison)
		comparisons = []costAndUsageComparison{{Metrics: metrics}}
	}

	page, nextToken := paginateList(comparisons, in.MaxResults, in.NextPageToken, costAndUsageComparisonKey)

	totalBaseline := metricTotalForPeriod(h, baseStart, baseEnd, in.MetricForComparison, serviceFilter)
	totalComparison := metricTotalForPeriod(h, cmpStart, cmpEnd, in.MetricForComparison, serviceFilter)

	return &getCostAndUsageComparisonsOutput{
		CostAndUsageComparisons: page,
		NextPageToken:           nextToken,
		TotalCostAndUsage:       comparisonMetricEntry(totalBaseline, totalComparison, in.MetricForComparison),
	}, nil
}

type getCostAndUsageWithResourcesInput struct {
	Filter      any               `json:"Filter"`
	TimePeriod  map[string]string `json:"TimePeriod"`
	Granularity string            `json:"Granularity"`
	Metrics     []string          `json:"Metrics"`
	GroupBy     []groupBySpec     `json:"GroupBy"`
}

// getCostAndUsageWithResourcesOutput's GroupDefinitions field was previously missing
// entirely -- see aws-sdk-go-v2/service/costexplorer's GetCostAndUsageWithResourcesOutput.
// ResultsByTime is legitimately always empty: real AWS resource-level cost data is keyed
// by individual resource ID (e.g. a specific EC2 instance ARN), and this emulator's
// synthetic cost ledger (seedCostLedger) only models service+date granularity, not
// per-resource entries, so there is no state to derive a non-empty result from.
type getCostAndUsageWithResourcesOutput struct {
	NextPageToken            string        `json:"NextPageToken,omitempty"`
	ResultsByTime            []any         `json:"ResultsByTime"`
	GroupDefinitions         []groupBySpec `json:"GroupDefinitions"`
	DimensionValueAttributes []any         `json:"DimensionValueAttributes"`
}

func (h *Handler) handleGetCostAndUsageWithResources(
	_ context.Context,
	in *getCostAndUsageWithResourcesInput,
) (*getCostAndUsageWithResourcesOutput, error) {
	if in.Filter == nil {
		return nil, fmt.Errorf("%w: Filter is required", ErrValidation)
	}

	if in.Granularity == "" {
		return nil, fmt.Errorf("%w: Granularity is required", ErrValidation)
	}

	// Real GetCostAndUsageWithResourcesInput requires TimePeriod and Metrics
	// (see api_op_GetCostAndUsageWithResources.go), same required-field gap
	// this pass closed on GetCostAndUsage. ResultsByTime stays legitimately
	// empty regardless (see the output type's doc comment above) -- this is
	// validation-only, not a behavior change to the empty result.
	if in.TimePeriod == nil || in.TimePeriod[timePeriodKeyStart] == "" || in.TimePeriod[timePeriodKeyEnd] == "" {
		return nil, fmt.Errorf("%w: TimePeriod is required", ErrValidation)
	}

	if len(in.Metrics) == 0 {
		return nil, fmt.Errorf("%w: Metrics is required", ErrValidation)
	}

	return &getCostAndUsageWithResourcesOutput{
		ResultsByTime:            []any{},
		GroupDefinitions:         in.GroupBy,
		DimensionValueAttributes: []any{},
	}, nil
}

// getCostComparisonDriversInput's metric member is field-diffed against real AWS CE's
// GetCostComparisonDriversInput: the field is the singular, required MetricForComparison
// string (same shape as GetCostAndUsageComparisons), not "Metric" -- the previous name
// matched no real member, so a real client's MetricForComparison was silently dropped and
// the required-field check below never fired for a request that omitted the (wrong) old
// name. Real AWS also carries GroupBy/MaxResults on this input and CostComparisonDrivers
// is always empty (see handler doc below, no per-line-item attribution state exists to
// derive drivers from) -- both are left off this struct rather than declared-and-ignored,
// matching Filter's existing documented-inert precedent (see gaps).
type getCostComparisonDriversInput struct {
	BaselineTimePeriod   map[string]string `json:"BaselineTimePeriod"`
	ComparisonTimePeriod map[string]string `json:"ComparisonTimePeriod"`
	Filter               *ceExpression     `json:"Filter"`
	MetricForComparison  string            `json:"MetricForComparison"`
	NextPageToken        string            `json:"NextPageToken"`
}

type getCostComparisonDriversOutput struct {
	NextPageToken         string `json:"NextPageToken,omitempty"`
	CostComparisonDrivers []any  `json:"CostComparisonDrivers"`
}

// handleGetCostComparisonDrivers always returns zero drivers: computing cost comparison
// drivers requires per-line-item cost-change attribution analysis this emulator's
// service+date-granularity synthetic ledger has no state to derive (same documented gap
// as GetCostAndUsageWithResources.ResultsByTime). NextPageToken is threaded through
// paginateList for a genuinely empty list (always yields an empty page and no next
// token, the correct terminal-page shape) rather than being echoed back unconditionally.
func (h *Handler) handleGetCostComparisonDrivers(
	_ context.Context,
	in *getCostComparisonDriversInput,
) (*getCostComparisonDriversOutput, error) {
	if in.BaselineTimePeriod == nil {
		return nil, fmt.Errorf("%w: BaselineTimePeriod is required", ErrValidation)
	}

	if in.ComparisonTimePeriod == nil {
		return nil, fmt.Errorf("%w: ComparisonTimePeriod is required", ErrValidation)
	}

	if in.MetricForComparison == "" {
		return nil, fmt.Errorf("%w: MetricForComparison is required", ErrValidation)
	}

	page, nextToken := paginateList([]any{}, 0, in.NextPageToken, func(any) string { return "" })

	return &getCostComparisonDriversOutput{
		CostComparisonDrivers: page,
		NextPageToken:         nextToken,
	}, nil
}

// buildCostUsageOps returns the cost-and-usage-family op dispatch entries.
func (h *Handler) buildCostUsageOps() map[string]service.JSONOpFunc {
	return map[string]service.JSONOpFunc{
		"GetCostAndUsage": service.WrapOp(
			h.handleGetCostAndUsage,
		),
		"GetCostForecast": service.WrapOp(
			h.handleGetCostForecast,
		),
		"GetUsageForecast": service.WrapOp(
			h.handleGetUsageForecast,
		),
		"GetDimensionValues": service.WrapOp(
			h.handleGetDimensionValues,
		),
		"GetTags": service.WrapOp(h.handleGetTags),
		"GetApproximateUsageRecords": service.WrapOp(
			h.handleGetApproximateUsageRecords,
		),
		"GetCostAndUsageComparisons": service.WrapOp(
			h.handleGetCostAndUsageComparisons,
		),
		"GetCostAndUsageWithResources": service.WrapOp(
			h.handleGetCostAndUsageWithResources,
		),
		"GetCostComparisonDrivers": service.WrapOp(
			h.handleGetCostComparisonDrivers,
		),
	}
}
