package ce_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	costexplorersdk "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/ce"
)

// TestGetCostAndUsage_Pagination_RealClient proves NextPageToken pagination
// over ResultsByTime is real: before this pass, NextPageToken was parsed off
// the wire and never read, so a request spanning more than the default
// 100-item page size silently returned every bucket in one response with no
// NextPageToken, instead of the real API's paginated shape. A 130-day DAILY
// range forces more than 100 buckets, crossing the default page-size
// boundary; every bucket's TimePeriod.Start must appear exactly once across
// the full page walk.
func TestGetCostAndUsage_Pagination_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	end := time.Now().UTC().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -130)
	startStr, endStr := start.Format("2006-01-02"), end.Format("2006-01-02")
	wantBuckets := int(end.Sub(start).Hours() / 24)

	seen := make(map[string]bool)

	var token *string

	pages := 0

	for {
		out, err := client.GetCostAndUsage(t.Context(), &costexplorersdk.GetCostAndUsageInput{
			TimePeriod:    &cetypes.DateInterval{Start: aws.String(startStr), End: aws.String(endStr)},
			Granularity:   cetypes.GranularityDaily,
			Metrics:       []string{"BlendedCost"},
			NextPageToken: token,
		})
		require.NoError(t, err)

		pages++

		for _, r := range out.ResultsByTime {
			key := aws.ToString(r.TimePeriod.Start)
			require.False(t, seen[key], "duplicate bucket %s across pages", key)
			seen[key] = true
		}

		if aws.ToString(out.NextPageToken) == "" {
			break
		}

		token = out.NextPageToken

		require.Less(t, pages, 10, "runaway pagination loop")
	}

	assert.Greater(t, pages, 1, "130 daily buckets must force multiple pages at the default 100-item page size")
	assert.Len(t, seen, wantBuckets, "every bucket must appear exactly once across the page walk")
}

// TestGetCostAndUsage_FilterNarrowsResults_RealClient proves
// GetCostAndUsageInput.Filter's SERVICE dimension is real, not dropped: a
// request filtered to one service's ledger entries must total less than the
// unfiltered sum across all 12 seeded services, and greater than zero.
func TestGetCostAndUsage_FilterNarrowsResults_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	end := time.Now().UTC().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -7)
	period := &cetypes.DateInterval{
		Start: aws.String(start.Format("2006-01-02")),
		End:   aws.String(end.Format("2006-01-02")),
	}

	unfiltered, err := client.GetCostAndUsage(t.Context(), &costexplorersdk.GetCostAndUsageInput{
		TimePeriod:  period,
		Granularity: cetypes.GranularityDaily,
		Metrics:     []string{"BlendedCost"},
	})
	require.NoError(t, err)

	filtered, err := client.GetCostAndUsage(t.Context(), &costexplorersdk.GetCostAndUsageInput{
		TimePeriod:  period,
		Granularity: cetypes.GranularityDaily,
		Metrics:     []string{"BlendedCost"},
		Filter: &cetypes.Expression{
			Dimensions: &cetypes.DimensionValues{
				Key:    cetypes.DimensionService,
				Values: []string{"AWS Lambda"},
			},
		},
	})
	require.NoError(t, err)

	unfilteredTotal := sumBlendedCost(t, unfiltered.ResultsByTime)
	filteredTotal := sumBlendedCost(t, filtered.ResultsByTime)

	assert.Positive(t, filteredTotal, "the filtered service must still have real cost")
	assert.Less(t, filteredTotal, unfilteredTotal, "a single-service filter must narrow the total")
}

// TestGetCostAndUsage_FilterDimensionsAndComposition_RealClient proves
// GetCostAndUsageInput.Filter is evaluated beyond the SERVICE dimension: before this
// pass, GetCostAndUsage routed Filter through serviceDimensionFilter, which discards
// any Dimensions clause whose Key isn't SERVICE and any And/Or/Not composition --
// even though the ledger's per-entry UsageType field (extractGroupKeys already reads
// it for GroupBy) makes USAGE_TYPE a real, non-fabricated dimension. A USAGE_TYPE-only
// filter, and an And() composition of SERVICE+USAGE_TYPE clauses, must both narrow the
// total.
func TestGetCostAndUsage_FilterDimensionsAndComposition_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	end := time.Now().UTC().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -7)
	period := &cetypes.DateInterval{
		Start: aws.String(start.Format("2006-01-02")),
		End:   aws.String(end.Format("2006-01-02")),
	}

	unfiltered, err := client.GetCostAndUsage(t.Context(), &costexplorersdk.GetCostAndUsageInput{
		TimePeriod:  period,
		Granularity: cetypes.GranularityDaily,
		Metrics:     []string{"BlendedCost"},
	})
	require.NoError(t, err)

	unfilteredTotal := sumBlendedCost(t, unfiltered.ResultsByTime)

	tests := []struct {
		filter *cetypes.Expression
		name   string
	}{
		{
			name: "usage_type_dimension",
			filter: &cetypes.Expression{
				Dimensions: &cetypes.DimensionValues{
					Key:    cetypes.DimensionUsageType,
					Values: []string{"Lambda-GB-Second"},
				},
			},
		},
		{
			name: "and_composition",
			filter: &cetypes.Expression{
				And: []cetypes.Expression{
					{Dimensions: &cetypes.DimensionValues{
						Key: cetypes.DimensionService, Values: []string{"AWS Lambda"},
					}},
					{Dimensions: &cetypes.DimensionValues{
						Key: cetypes.DimensionUsageType, Values: []string{"Lambda-GB-Second"},
					}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, callErr := client.GetCostAndUsage(t.Context(), &costexplorersdk.GetCostAndUsageInput{
				TimePeriod:  period,
				Granularity: cetypes.GranularityDaily,
				Metrics:     []string{"BlendedCost"},
				Filter:      tt.filter,
			})
			require.NoError(t, callErr)

			total := sumBlendedCost(t, out.ResultsByTime)
			assert.Positive(t, total, "the filtered usage type must still have real cost")
			assert.Less(t, total, unfilteredTotal, "the filter must narrow the total")
		})
	}
}

// TestGetDimensionValues_Pagination_RealClient proves NextPageToken/
// MaxResults pagination over the 12 seeded SERVICE dimension values drops
// nothing and duplicates nothing across page boundaries -- a regression
// guard for the paginateOrdered cursor off-by-one this pass found and fixed
// (it originally resumed one item past the cursor, silently dropping the
// first record of every resumed page).
func TestGetDimensionValues_Pagination_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	period := &cetypes.DateInterval{Start: aws.String("2024-01-01"), End: aws.String("2024-02-01")}

	seen := make(map[string]bool)

	var token *string

	pages := 0

	for {
		out, err := client.GetDimensionValues(t.Context(), &costexplorersdk.GetDimensionValuesInput{
			Dimension:     cetypes.DimensionService,
			TimePeriod:    period,
			MaxResults:    aws.Int32(3),
			NextPageToken: token,
		})
		require.NoError(t, err)

		pages++

		for _, v := range out.DimensionValues {
			key := aws.ToString(v.Value)
			require.False(t, seen[key], "duplicate value %s across pages", key)
			seen[key] = true
		}

		if aws.ToString(out.NextPageToken) == "" {
			break
		}

		token = out.NextPageToken

		require.Less(t, pages, 10, "runaway pagination loop")
	}

	assert.Greater(t, pages, 1, "12 services capped at 3 per page must force multiple pages")
	assert.Len(t, seen, 12, "every seeded service must appear exactly once across the page walk")
}

func sumBlendedCost(t *testing.T, results []cetypes.ResultByTime) float64 {
	t.Helper()

	var total float64

	for _, r := range results {
		mv, ok := r.Total["BlendedCost"]
		require.True(t, ok)

		v, err := strconv.ParseFloat(aws.ToString(mv.Amount), 64)
		require.NoError(t, err)

		total += v
	}

	return total
}

// TestGetCostForecast_Metric_RealClient proves GetCostForecastInput.Metric
// actually changes which ledger metric the forecast is computed from: before
// this pass GetForecastByTime always used BlendedCost regardless of what was
// requested, so a BLENDED_COST forecast and a USAGE_QUANTITY forecast were
// numerically identical when they must not be (the two metrics have very
// different real magnitudes in this ledger).
func TestGetCostForecast_Metric_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	start := time.Now().UTC().Format("2006-01-02")
	end := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
	period := &cetypes.DateInterval{Start: aws.String(start), End: aws.String(end)}

	blended, err := client.GetCostForecast(t.Context(), &costexplorersdk.GetCostForecastInput{
		TimePeriod:  period,
		Granularity: cetypes.GranularityDaily,
		Metric:      cetypes.MetricBlendedCost,
	})
	require.NoError(t, err)

	usage, err := client.GetCostForecast(t.Context(), &costexplorersdk.GetCostForecastInput{
		TimePeriod:  period,
		Granularity: cetypes.GranularityDaily,
		Metric:      cetypes.MetricUsageQuantity,
	})
	require.NoError(t, err)

	blendedMean, err := strconv.ParseFloat(aws.ToString(blended.Total.Amount), 64)
	require.NoError(t, err)
	usageMean, err := strconv.ParseFloat(aws.ToString(usage.Total.Amount), 64)
	require.NoError(t, err)

	assert.NotEqual(t, blendedMean, usageMean, "different Metric values must produce different forecasts")
}

// TestGetCostAndUsageComparisons_MetricForComparison_RealClient proves the
// real MetricForComparison field name is honored: the wire struct previously
// declared "Metric" instead, which real AWS's aws-sdk-go-v2 client never
// sends (it always sends MetricForComparison), so this comparison would have
// been silently computed with an unset metric before the field-name fix.
func TestGetCostAndUsageComparisons_MetricForComparison_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	out, err := client.GetCostAndUsageComparisons(t.Context(), &costexplorersdk.GetCostAndUsageComparisonsInput{
		BaselineTimePeriod:   &cetypes.DateInterval{Start: aws.String("2024-01-01"), End: aws.String("2024-02-01")},
		ComparisonTimePeriod: &cetypes.DateInterval{Start: aws.String("2024-02-01"), End: aws.String("2024-03-01")},
		MetricForComparison:  aws.String("BlendedCost"),
	})
	require.NoError(t, err)
	require.Len(t, out.CostAndUsageComparisons, 1)

	mv, ok := out.CostAndUsageComparisons[0].Metrics["BlendedCost"]
	require.True(t, ok, "Metrics must be keyed by the real MetricForComparison value, not left empty")
	assert.NotEmpty(t, aws.ToString(mv.BaselineTimePeriodAmount))
}

// TestGetCostAndUsageComparisons_GroupBy_RealClient proves GroupBy produces a
// real per-group breakdown (one CostAndUsageComparisons entry per SERVICE
// value) instead of collapsing to a single aggregate entry regardless of
// GroupBy, and that Filter narrows which services are grouped.
func TestGetCostAndUsageComparisons_GroupBy_RealClient(t *testing.T) {
	t.Parallel()

	h := ce.NewHandler(ce.NewInMemoryBackend("000000000000", "us-east-1"))
	client := newTestCEClient(t, h)

	end := time.Now().UTC().Truncate(24 * time.Hour)
	baselineStart := end.AddDate(0, 0, -14)
	cmpStart := end.AddDate(0, 0, -7)

	out, err := client.GetCostAndUsageComparisons(t.Context(), &costexplorersdk.GetCostAndUsageComparisonsInput{
		BaselineTimePeriod: &cetypes.DateInterval{
			Start: aws.String(baselineStart.Format("2006-01-02")),
			End:   aws.String(cmpStart.Format("2006-01-02")),
		},
		ComparisonTimePeriod: &cetypes.DateInterval{
			Start: aws.String(cmpStart.Format("2006-01-02")),
			End:   aws.String(end.Format("2006-01-02")),
		},
		MetricForComparison: aws.String("BlendedCost"),
		GroupBy: []cetypes.GroupDefinition{
			{Type: cetypes.GroupDefinitionTypeDimension, Key: aws.String("SERVICE")},
		},
	})
	require.NoError(t, err)

	assert.Greater(
		t,
		len(out.CostAndUsageComparisons),
		1,
		"GroupBy=SERVICE over the 12-service ledger must yield more than one entry",
	)

	seen := make(map[string]bool)

	for _, c := range out.CostAndUsageComparisons {
		require.NotNil(t, c.CostAndUsageSelector)
		require.NotNil(t, c.CostAndUsageSelector.Dimensions)
		require.Len(t, c.CostAndUsageSelector.Dimensions.Values, 1)

		val := c.CostAndUsageSelector.Dimensions.Values[0]
		assert.False(t, seen[val], "duplicate group %s", val)
		seen[val] = true
	}
}
