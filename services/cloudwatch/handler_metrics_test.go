package cloudwatch_test

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

// ---------------------------------------------------------------------------
// Handler integration: PutMetricData StatisticSet form parsing (gap #2)
// ---------------------------------------------------------------------------

func TestHandler_PutMetricData_StatisticSet_FormParsed(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	ts := cloudwatch.RecentTestAnchor().Format(time.RFC3339)
	rec := postForm(
		t, h,
		"Action=PutMetricData"+
			"&Namespace=App"+
			"&MetricData.member.1.MetricName=Reqs"+
			"&MetricData.member.1.StatisticValues.SampleCount=5"+
			"&MetricData.member.1.StatisticValues.Sum=250"+
			"&MetricData.member.1.StatisticValues.Minimum=40"+
			"&MetricData.member.1.StatisticValues.Maximum=60"+
			"&MetricData.member.1.Timestamp="+ts,
	)
	assert.Equal(t, 200, rec.Code, "valid StatisticSet should return 200; body: %s", rec.Body.String())
}

// TestHandler_PutMetricData_ValueAndStatisticSet_Returns400 verifies real AWS
// behaviour: PutMetricDataOutput has no members besides the request ID (no
// per-datum "unprocessed" result exists for this operation), so an invalid
// datum fails the entire request with a 400 InvalidParameterCombination
// instead of a fabricated 200-with-partial-failure response.
func TestHandler_PutMetricData_ValueAndStatisticSet_Returns400(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	ts := cloudwatch.RecentTestAnchor().Format(time.RFC3339)
	rec := postForm(
		t, h,
		"Action=PutMetricData"+
			"&Namespace=App"+
			"&MetricData.member.1.MetricName=BadReq"+
			"&MetricData.member.1.Value=1.0"+
			"&MetricData.member.1.StatisticValues.SampleCount=5"+
			"&MetricData.member.1.StatisticValues.Sum=250"+
			"&MetricData.member.1.StatisticValues.Minimum=40"+
			"&MetricData.member.1.StatisticValues.Maximum=60"+
			"&MetricData.member.1.Timestamp="+ts,
	)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterCombination")
}

// TestHandler_PutMetricData_TimestampOutOfRange_Returns400 verifies real AWS
// behaviour (api_op_PutMetricData.go: "You can specify time stamps that are
// as much as two weeks before the current date, and as much as 2 hours after
// the current day and time.") -- a Timestamp outside that window must fail
// with 400 InvalidParameterValue, not fall through to a 500 as an unmapped
// error.
func TestHandler_PutMetricData_TimestampOutOfRange_Returns400(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	tooOld := time.Now().UTC().Add(-15 * 24 * time.Hour).Format(time.RFC3339)
	rec := postForm(
		t, h,
		"Action=PutMetricData"+
			"&Namespace=App"+
			"&MetricData.member.1.MetricName=Latency"+
			"&MetricData.member.1.Value=1.0"+
			"&MetricData.member.1.Timestamp="+tooOld,
	)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterValue")
}

func TestHandler_PutMetricData_InvalidStorageResolution_Returns400(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	ts := cloudwatch.RecentTestAnchor().Format(time.RFC3339)
	rec := postForm(
		t, h,
		"Action=PutMetricData"+
			"&Namespace=App"+
			"&MetricData.member.1.MetricName=BadRes"+
			"&MetricData.member.1.Value=1.0"+
			"&MetricData.member.1.StorageResolution=30"+
			"&MetricData.member.1.Timestamp="+ts,
	)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterValue")
}

// ---------------------------------------------------------------------------
// Handler integration: PutMetricData with dimensions (gap #1)
// ---------------------------------------------------------------------------

func TestHandler_PutMetricData_WithDimensions(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	ts := cloudwatch.RecentTestAnchor().Format(time.RFC3339)
	rec := postForm(
		t, h,
		"Action=PutMetricData"+
			"&Namespace=MyApp"+
			"&MetricData.member.1.MetricName=Errors"+
			"&MetricData.member.1.Value=5"+
			"&MetricData.member.1.Dimensions.member.1.Name=Service"+
			"&MetricData.member.1.Dimensions.member.1.Value=auth"+
			"&MetricData.member.1.Dimensions.member.2.Name=Region"+
			"&MetricData.member.1.Dimensions.member.2.Value=us-east-1"+
			"&MetricData.member.1.Timestamp="+ts,
	)
	assert.Equal(t, 200, rec.Code, "response: %s", rec.Body.String())
}

// ---------------------------------------------------------------------------
// Handler integration: ListMetrics dimension filter (gap #4)
// ---------------------------------------------------------------------------

func TestHandler_ListMetrics_DimensionFilter(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	ts := cloudwatch.RecentTestAnchor().Format(time.RFC3339)

	// Store two metrics with different dimensions.
	postForm(
		t, h,
		"Action=PutMetricData&Namespace=NS"+
			"&MetricData.member.1.MetricName=RPM&MetricData.member.1.Value=100"+
			"&MetricData.member.1.Dimensions.member.1.Name=Env&MetricData.member.1.Dimensions.member.1.Value=prod"+
			"&MetricData.member.1.Timestamp="+ts,
	)
	postForm(
		t, h,
		"Action=PutMetricData&Namespace=NS"+
			"&MetricData.member.1.MetricName=RPM&MetricData.member.1.Value=50"+
			"&MetricData.member.1.Dimensions.member.1.Name=Env&MetricData.member.1.Dimensions.member.1.Value=staging"+
			"&MetricData.member.1.Timestamp="+ts,
	)

	// Filter to prod only.
	rec := postForm(
		t, h,
		"Action=ListMetrics&Namespace=NS&MetricName=RPM"+
			"&Dimensions.member.1.Name=Env&Dimensions.member.1.Value=prod",
	)
	assert.Equal(t, 200, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "RPM")
}

// ---------------------------------------------------------------------------
// Handler integration: GetMetricStatistics with dimensions (gap #9)
// ---------------------------------------------------------------------------

func TestHandler_GetMetricStatistics_WithDimensions(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	anchor := cloudwatch.RecentTestAnchor()
	postForm(
		t, h,
		"Action=PutMetricData&Namespace=MyNS"+
			"&MetricData.member.1.MetricName=CPU"+
			"&MetricData.member.1.Value=75"+
			"&MetricData.member.1.Dimensions.member.1.Name=Host&MetricData.member.1.Dimensions.member.1.Value=web1"+
			"&MetricData.member.1.Timestamp="+anchor.Format(time.RFC3339),
	)

	rec := postForm(
		t, h,
		"Action=GetMetricStatistics"+
			"&Namespace=MyNS"+
			"&MetricName=CPU"+
			"&Dimensions.member.1.Name=Host&Dimensions.member.1.Value=web1"+
			"&StartTime="+anchor.Add(-time.Hour).Format(time.RFC3339)+
			"&EndTime="+anchor.Add(time.Hour).Format(time.RFC3339)+
			"&Period=60"+
			"&Statistics.member.1=Average",
	)
	assert.Equal(t, 200, rec.Code)
}

// ---------------------------------------------------------------------------
// Handler integration: ScanBy (gap #6)
// ---------------------------------------------------------------------------

func TestHandler_GetMetricData_ScanByDescending(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	base := cloudwatch.RecentTestAnchor()

	// Store a few data points.
	for i := 1; i <= 3; i++ {
		ts := base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339)
		postForm(
			t, h, "Action=PutMetricData&Namespace=NS"+
				"&MetricData.member.1.MetricName=Counter"+
				"&MetricData.member.1.Value=1"+
				"&MetricData.member.1.Timestamp="+ts,
		)
	}

	rec := postForm(
		t, h,
		"Action=GetMetricData"+
			"&MetricDataQueries.member.1.Id=m1"+
			"&MetricDataQueries.member.1.MetricStat.Metric.Namespace=NS"+
			"&MetricDataQueries.member.1.MetricStat.Metric.MetricName=Counter"+
			"&MetricDataQueries.member.1.MetricStat.Stat=Sum"+
			"&MetricDataQueries.member.1.MetricStat.Period=60"+
			"&MetricDataQueries.member.1.ReturnData=true"+
			"&StartTime="+base.Format(time.RFC3339)+
			"&EndTime="+base.Add(4*time.Minute).Format(time.RFC3339)+
			"&ScanBy=TimestampDescending",
	)
	assert.Equal(t, 200, rec.Code, "body: %s", rec.Body.String())
}

// ---------------------------------------------------------------------------
// Handler integration: StorageResolution field (gap #3)
// ---------------------------------------------------------------------------

func TestHandler_PutMetricData_StorageResolution1(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	ts := cloudwatch.RecentTestAnchor().Format(time.RFC3339)
	rec := postForm(
		t, h,
		"Action=PutMetricData&Namespace=NS"+
			"&MetricData.member.1.MetricName=Ticks"+
			"&MetricData.member.1.Value=1"+
			"&MetricData.member.1.StorageResolution=1"+
			"&MetricData.member.1.Timestamp="+ts,
	)
	assert.Equal(t, 200, rec.Code, "valid StorageResolution=1 should succeed")
}

func TestHandler_PutMetricData_StorageResolution60(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	ts := cloudwatch.RecentTestAnchor().Format(time.RFC3339)
	rec := postForm(
		t, h,
		"Action=PutMetricData&Namespace=NS"+
			"&MetricData.member.1.MetricName=Ticks"+
			"&MetricData.member.1.Value=1"+
			"&MetricData.member.1.StorageResolution=60"+
			"&MetricData.member.1.Timestamp="+ts,
	)
	assert.Equal(t, 200, rec.Code)
}

// ---------------------------------------------------------------------------
// Additional handler tests for accuracy gaps
// ---------------------------------------------------------------------------

func TestHandler_GetMetricData_WithExpressions(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	anchor := cloudwatch.RecentTestAnchor()

	postForm(
		t, h,
		"Action=PutMetricData&Namespace=NS"+
			"&MetricData.member.1.MetricName=Hits"+
			"&MetricData.member.1.Value=10"+
			"&MetricData.member.1.Timestamp="+anchor.Add(30*time.Second).Format(time.RFC3339),
	)

	rec := postForm(
		t, h,
		"Action=GetMetricData"+
			"&MetricDataQueries.member.1.Id=m1"+
			"&MetricDataQueries.member.1.MetricStat.Metric.Namespace=NS"+
			"&MetricDataQueries.member.1.MetricStat.Metric.MetricName=Hits"+
			"&MetricDataQueries.member.1.MetricStat.Stat=Sum"+
			"&MetricDataQueries.member.1.MetricStat.Period=60"+
			"&MetricDataQueries.member.1.ReturnData=true"+
			"&MetricDataQueries.member.2.Id=e1"+
			"&MetricDataQueries.member.2.Expression=m1+*+2"+
			"&MetricDataQueries.member.2.ReturnData=true"+
			"&StartTime="+anchor.Format(time.RFC3339)+
			"&EndTime="+anchor.Add(2*time.Minute).Format(time.RFC3339),
	)
	assert.Equal(t, 200, rec.Code, "GetMetricData with expression: %s", rec.Body.String())
}

func TestHandler_PutMetricData_MultipleWithMixedDimensions(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	ts := cloudwatch.RecentTestAnchor().Format(time.RFC3339)
	rec := postForm(
		t, h,
		"Action=PutMetricData&Namespace=Multi"+
			"&MetricData.member.1.MetricName=Errors"+
			"&MetricData.member.1.Value=1"+
			"&MetricData.member.1.Dimensions.member.1.Name=Svc&MetricData.member.1.Dimensions.member.1.Value=auth"+
			"&MetricData.member.1.Timestamp="+ts+
			"&MetricData.member.2.MetricName=Errors"+
			"&MetricData.member.2.Value=2"+
			"&MetricData.member.2.Dimensions.member.1.Name=Svc&MetricData.member.2.Dimensions.member.1.Value=api"+
			"&MetricData.member.2.Timestamp="+ts+
			"&MetricData.member.3.MetricName=Errors"+
			"&MetricData.member.3.Value=3"+
			"&MetricData.member.3.Timestamp="+ts,
	)
	assert.Equal(t, 200, rec.Code, "three metrics with different dimension sets")
}

func TestHandler_GetMetricStatistics_NoDimensions(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	anchor := cloudwatch.RecentTestAnchor()

	postForm(
		t, h,
		"Action=PutMetricData&Namespace=NS"+
			"&MetricData.member.1.MetricName=Mem&MetricData.member.1.Value=512"+
			"&MetricData.member.1.Timestamp="+anchor.Add(30*time.Second).Format(time.RFC3339),
	)

	rec := postForm(
		t, h,
		"Action=GetMetricStatistics"+
			"&Namespace=NS&MetricName=Mem"+
			"&StartTime="+anchor.Format(time.RFC3339)+
			"&EndTime="+anchor.Add(2*time.Minute).Format(time.RFC3339)+
			"&Period=60&Statistics.member.1=Sum",
	)
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "GetMetricStatisticsResponse")
}

func TestHandler_ListMetrics_PartialDimFilter(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	ts := cloudwatch.RecentTestAnchor().Format(time.RFC3339)
	// Store metric with two dimensions.
	postForm(
		t, h,
		"Action=PutMetricData&Namespace=NS"+
			"&MetricData.member.1.MetricName=CPU"+
			"&MetricData.member.1.Value=50"+
			"&MetricData.member.1.Dimensions.member.1.Name=Env&MetricData.member.1.Dimensions.member.1.Value=prod"+
			"&MetricData.member.1.Dimensions.member.2.Name=Host&MetricData.member.1.Dimensions.member.2.Value=web1"+
			"&MetricData.member.1.Timestamp="+ts,
	)

	// Filter by just Env=prod (partial).
	rec := postForm(
		t, h,
		"Action=ListMetrics&Namespace=NS&MetricName=CPU"+
			"&Dimensions.member.1.Name=Env&Dimensions.member.1.Value=prod",
	)
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "CPU")
}

// ---------------------------------------------------------------------------
// ExtendedStatistics in GetMetricStatistics response
// ---------------------------------------------------------------------------

func TestHandler_GetMetricStatistics_ExtendedStatistics_InResponse(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	anchor := cloudwatch.RecentTestAnchor()
	postForm(
		t, h,
		"Action=PutMetricData&Namespace=NS"+
			"&MetricData.member.1.MetricName=Latency&MetricData.member.1.Value=10"+
			"&MetricData.member.1.Timestamp="+anchor.Add(30*time.Second).Format(time.RFC3339)+
			"&MetricData.member.2.MetricName=Latency&MetricData.member.2.Value=50"+
			"&MetricData.member.2.Timestamp="+anchor.Add(31*time.Second).Format(time.RFC3339)+
			"&MetricData.member.3.MetricName=Latency&MetricData.member.3.Value=90"+
			"&MetricData.member.3.Timestamp="+anchor.Add(32*time.Second).Format(time.RFC3339),
	)

	rec := postForm(
		t, h,
		"Action=GetMetricStatistics&Namespace=NS&MetricName=Latency"+
			"&StartTime="+anchor.Format(time.RFC3339)+"&EndTime="+anchor.Add(2*time.Minute).Format(time.RFC3339)+"&Period=60"+
			"&ExtendedStatistics.member.1=p99&ExtendedStatistics.member.2=p50",
	)
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "GetMetricStatisticsResponse")
}

// ---------------------------------------------------------------------------
// MetricMath: handler integration for constant expressions
// ---------------------------------------------------------------------------

func TestHandler_GetMetricData_ConstantExpression(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	anchor := cloudwatch.RecentTestAnchor()
	postForm(
		t, h,
		"Action=PutMetricData&Namespace=NS"+
			"&MetricData.member.1.MetricName=Hits"+
			"&MetricData.member.1.Value=10"+
			"&MetricData.member.1.Timestamp="+anchor.Add(30*time.Second).Format(time.RFC3339),
	)

	rec := postForm(
		t, h,
		"Action=GetMetricData"+
			"&MetricDataQueries.member.1.Id=m1"+
			"&MetricDataQueries.member.1.MetricStat.Metric.Namespace=NS"+
			"&MetricDataQueries.member.1.MetricStat.Metric.MetricName=Hits"+
			"&MetricDataQueries.member.1.MetricStat.Stat=Sum"+
			"&MetricDataQueries.member.1.MetricStat.Period=60"+
			"&MetricDataQueries.member.1.ReturnData=false"+
			"&MetricDataQueries.member.2.Id=e1"+
			"&MetricDataQueries.member.2.Expression=m1+*+2"+
			"&MetricDataQueries.member.2.ReturnData=true"+
			"&StartTime="+anchor.Format(time.RFC3339)+"&EndTime="+anchor.Add(2*time.Minute).Format(time.RFC3339),
	)
	assert.Equal(t, 200, rec.Code)
}

func TestHandler_GetMetricData_AvgMetricsExpression(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	anchor := cloudwatch.RecentTestAnchor()
	ts := anchor.Add(30 * time.Second).Format(time.RFC3339)
	postForm(t, h, "Action=PutMetricData&Namespace=NS"+
		"&MetricData.member.1.MetricName=A&MetricData.member.1.Value=10&MetricData.member.1.Timestamp="+ts)
	postForm(t, h, "Action=PutMetricData&Namespace=NS"+
		"&MetricData.member.1.MetricName=B&MetricData.member.1.Value=30&MetricData.member.1.Timestamp="+ts)

	rec := postForm(
		t, h,
		"Action=GetMetricData"+
			"&MetricDataQueries.member.1.Id=m1&MetricDataQueries.member.1.MetricStat.Metric.Namespace=NS"+
			"&MetricDataQueries.member.1.MetricStat.Metric.MetricName=A"+
			"&MetricDataQueries.member.1.MetricStat.Stat=Sum&MetricDataQueries.member.1.MetricStat.Period=60"+
			"&MetricDataQueries.member.1.ReturnData=false"+
			"&MetricDataQueries.member.2.Id=m2&MetricDataQueries.member.2.MetricStat.Metric.Namespace=NS"+
			"&MetricDataQueries.member.2.MetricStat.Metric.MetricName=B"+
			"&MetricDataQueries.member.2.MetricStat.Stat=Sum&MetricDataQueries.member.2.MetricStat.Period=60"+
			"&MetricDataQueries.member.2.ReturnData=false"+
			"&MetricDataQueries.member.3.Id=avg&MetricDataQueries.member.3.Expression=AVG%28METRICS%28%29%29"+
			"&MetricDataQueries.member.3.ReturnData=true"+
			"&StartTime="+anchor.Format(time.RFC3339)+"&EndTime="+anchor.Add(2*time.Minute).Format(time.RFC3339),
	)
	assert.Equal(t, 200, rec.Code)
}

// ---------------------------------------------------------------------------
// HasStatisticSet: handler sets flag, validateMetricDatum enforces exclusion
// ---------------------------------------------------------------------------

func TestHandler_PutMetricData_StatisticSetOnly_Accepted(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	ts := cloudwatch.RecentTestAnchor().Format(time.RFC3339)
	rec := postForm(
		t, h,
		"Action=PutMetricData&Namespace=App"+
			"&MetricData.member.1.MetricName=Reqs"+
			"&MetricData.member.1.StatisticValues.SampleCount=5"+
			"&MetricData.member.1.StatisticValues.Sum=250"+
			"&MetricData.member.1.StatisticValues.Minimum=40"+
			"&MetricData.member.1.StatisticValues.Maximum=60"+
			"&MetricData.member.1.Timestamp="+ts,
	)
	assert.Equal(t, 200, rec.Code)
	// PutMetricDataOutput has no members: a 200 response body carries only ResponseMetadata.
	assert.NotContains(t, rec.Body.String(), "<ErrorCode>")
}

// TestHandler_PutMetricData_StatisticSetAndValue_Rejected verifies that an
// invalid datum fails the whole request with a real AWS error response, not a
// 200 with a fabricated UnprocessedMetricData list (PutMetricDataOutput has no
// such field — confirmed against aws-sdk-go-v2 cloudwatch types).
func TestHandler_PutMetricData_StatisticSetAndValue_Rejected(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	ts := cloudwatch.RecentTestAnchor().Format(time.RFC3339)
	rec := postForm(
		t, h,
		"Action=PutMetricData&Namespace=App"+
			"&MetricData.member.1.MetricName=Bad"+
			"&MetricData.member.1.Value=1.0"+
			"&MetricData.member.1.StatisticValues.SampleCount=5"+
			"&MetricData.member.1.StatisticValues.Sum=250"+
			"&MetricData.member.1.StatisticValues.Minimum=40"+
			"&MetricData.member.1.StatisticValues.Maximum=60"+
			"&MetricData.member.1.Timestamp="+ts,
	)
	assert.Equal(t, 400, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterCombination")
}

func TestHandler_PutMetricData_StatisticSet_StoredCorrectly(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	ts := cloudwatch.RecentTestAnchor()

	err := b.PutMetricData("NS", []cloudwatch.MetricDatum{
		{
			MetricName: "Reqs", HasStatisticSet: true,
			Count: 10, Sum: 500, Min: 20, Max: 80,
			Timestamp: ts,
		},
	})
	require.NoError(t, err)

	dps, err := b.GetMetricStatistics("NS", "Reqs", nil,
		ts.Add(-time.Minute), ts.Add(time.Minute), 60, []string{"Sum", "SampleCount"}, nil)
	require.NoError(t, err)
	require.Len(t, dps, 1)
	require.NotNil(t, dps[0].Sum)
	assert.InDelta(t, 500.0, *dps[0].Sum, 1e-9, "StatisticSet Sum should be stored")
	require.NotNil(t, dps[0].SampleCount)
	assert.InDelta(t, 10.0, *dps[0].SampleCount, 1e-9, "StatisticSet SampleCount should be stored")
}

// ---------------------------------------------------------------------------
// PutMetricData: Values/Counts array input
//
// Real CloudWatch lets a caller publish up to 150 unique values per datum via
// parallel Values/Counts arrays instead of a single Value or a StatisticSet
// (see aws-sdk-go-v2 cloudwatch/types.MetricDatum.Values doc comment). Prior to
// this test the form and rpc-v2-cbor parsers silently dropped this field
// entirely, so any client using it lost data with no error.
// ---------------------------------------------------------------------------

func TestHandler_PutMetricData_ValuesCountsArray_StoredCorrectly(t *testing.T) {
	t.Parallel()

	h := cloudwatch.NewHandler(cloudwatch.NewInMemoryBackend())
	ts := cloudwatch.RecentTestAnchor()
	rec := postForm(
		t, h,
		"Action=PutMetricData&Namespace=App"+
			"&MetricData.member.1.MetricName=Latency"+
			"&MetricData.member.1.Values.member.1=10"+
			"&MetricData.member.1.Values.member.2=20"+
			"&MetricData.member.1.Values.member.3=30"+
			"&MetricData.member.1.Counts.member.1=2"+
			"&MetricData.member.1.Counts.member.2=3"+
			"&MetricData.member.1.Counts.member.3=5"+
			"&MetricData.member.1.Timestamp="+ts.Format(time.RFC3339),
	)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	b := h.Backend.(*cloudwatch.InMemoryBackend)
	dps, err := b.GetMetricStatistics("App", "Latency", nil,
		ts.Add(-time.Minute), ts.Add(time.Minute), 60,
		[]string{"Sum", "SampleCount", "Minimum", "Maximum"}, nil)
	require.NoError(t, err)
	require.Len(t, dps, 1)

	// count = 2+3+5 = 10; sum = 10*2 + 20*3 + 30*5 = 230; min = 10; max = 30.
	require.NotNil(t, dps[0].SampleCount)
	assert.InDelta(t, 10.0, *dps[0].SampleCount, 1e-9)
	require.NotNil(t, dps[0].Sum)
	assert.InDelta(t, 230.0, *dps[0].Sum, 1e-9)
	require.NotNil(t, dps[0].Minimum)
	assert.InDelta(t, 10.0, *dps[0].Minimum, 1e-9)
	require.NotNil(t, dps[0].Maximum)
	assert.InDelta(t, 30.0, *dps[0].Maximum, 1e-9)
}

func TestHandler_PutMetricData_UnitParsed(t *testing.T) {
	t.Parallel()

	h := newCWHandler()
	anchor := cloudwatch.RecentTestAnchor()
	postForm(
		t, h,
		"Action=PutMetricData&Namespace=NS"+
			"&MetricData.member.1.MetricName=Mem"+
			"&MetricData.member.1.Value=1024"+
			"&MetricData.member.1.Unit=Bytes"+
			"&MetricData.member.1.Timestamp="+anchor.Format(time.RFC3339),
	)

	rec := postForm(
		t, h,
		"Action=GetMetricStatistics&Namespace=NS&MetricName=Mem"+
			"&StartTime="+anchor.Add(-24*time.Hour).Format(time.RFC3339)+
			"&EndTime="+anchor.Add(24*time.Hour).Format(time.RFC3339)+
			"&Period=86400&Statistics.member.1=Sum",
	)
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "Bytes")
}

func TestCloudWatchHandler_GetMetricData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name: "with_metric_data_queries",
			body: "Action=GetMetricData" +
				"&StartTime=2000-01-01T00:00:00Z" +
				"&EndTime=2099-01-01T00:00:00Z" +
				"&MetricDataQueries.member.1.Id=q1" +
				"&MetricDataQueries.member.1.Label=TestLabel" +
				"&MetricDataQueries.member.1.MetricStat.Metric.Namespace=AWS/EC2" +
				"&MetricDataQueries.member.1.MetricStat.Metric.MetricName=CPUUtilization" +
				"&MetricDataQueries.member.1.MetricStat.Stat=Average" +
				"&MetricDataQueries.member.1.MetricStat.Period=60",
			wantCode: http.StatusOK,
		},
		{
			name: "invalid_start_and_end_time_uses_defaults",
			body: "Action=GetMetricData" +
				"&StartTime=invalid" +
				"&EndTime=invalid" +
				"&MetricDataQueries.member.1.Id=q1" +
				"&MetricDataQueries.member.1.MetricStat.Metric.Namespace=NS" +
				"&MetricDataQueries.member.1.MetricStat.Metric.MetricName=M" +
				"&MetricDataQueries.member.1.MetricStat.Stat=Sum" +
				"&MetricDataQueries.member.1.MetricStat.Period=60",
			wantCode: http.StatusOK,
		},
		{
			name: "with_query_label",
			body: "Action=GetMetricData" +
				"&StartTime=2000-01-01T00:00:00Z" +
				"&EndTime=2099-01-01T00:00:00Z" +
				"&MetricDataQueries.member.1.Id=q1" +
				"&MetricDataQueries.member.1.Label=" +
				"&MetricDataQueries.member.1.MetricStat.Metric.Namespace=NS" +
				"&MetricDataQueries.member.1.MetricStat.Metric.MetricName=M" +
				"&MetricDataQueries.member.1.MetricStat.Stat=Average" +
				"&MetricDataQueries.member.1.MetricStat.Period=60",
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ts := cwServer(t)
			// Pre-populate some metric data
			cwPost(t, ts,
				"Action=PutMetricData&Namespace=AWS/EC2"+
					"&MetricData.member.1.MetricName=CPUUtilization"+
					"&MetricData.member.1.Value=42").Body.Close()

			resp := cwPost(t, ts, tt.body)
			defer resp.Body.Close()
			assert.Equal(t, tt.wantCode, resp.StatusCode)
		})
	}
}

// ---------------------------------------------------------------------------
// Accuracy audit handler tests — gaps from issue #1686
// ---------------------------------------------------------------------------

func TestCloudWatchHandler_PutMetricData_WithStatisticSet(t *testing.T) {
	t.Parallel()

	h := newCWHandler()

	// Put a StatisticSet datum.
	body := strings.Join([]string{
		"Action=PutMetricData",
		"Namespace=App%2FMetrics",
		"MetricData.member.1.MetricName=Latency",
		"MetricData.member.1.Unit=Milliseconds",
		"MetricData.member.1.StatisticValues.SampleCount=10",
		"MetricData.member.1.StatisticValues.Sum=250",
		"MetricData.member.1.StatisticValues.Minimum=20",
		"MetricData.member.1.StatisticValues.Maximum=35",
	}, "&")
	rec := postForm(t, h, body)
	require.Equal(t, http.StatusOK, rec.Code)

	// GetMetricStatistics should reflect the pre-aggregated values.
	statsBody := strings.Join([]string{
		"Action=GetMetricStatistics",
		"Namespace=App%2FMetrics",
		"MetricName=Latency",
		"StartTime=2000-01-01T00%3A00%3A00Z",
		"EndTime=2100-01-01T00%3A00%3A00Z",
		"Period=600",
		"Statistics.member.1=Sum",
		"Statistics.member.2=SampleCount",
		"Statistics.member.3=Minimum",
		"Statistics.member.4=Maximum",
	}, "&")
	statsRec := postForm(t, h, statsBody)
	require.Equal(t, http.StatusOK, statsRec.Code)

	type dp struct {
		Sum         *float64 `xml:"Sum"`
		SampleCount *float64 `xml:"SampleCount"`
		Minimum     *float64 `xml:"Minimum"`
		Maximum     *float64 `xml:"Maximum"`
	}
	type resp struct {
		Datapoints []dp `xml:"GetMetricStatisticsResult>Datapoints>member"`
	}
	var out resp
	require.NoError(t, xml.Unmarshal(statsRec.Body.Bytes(), &out))
	require.Len(t, out.Datapoints, 1)
	require.NotNil(t, out.Datapoints[0].Sum)
	assert.InDelta(t, 250.0, *out.Datapoints[0].Sum, 1e-9)
	require.NotNil(t, out.Datapoints[0].SampleCount)
	assert.InDelta(t, 10.0, *out.Datapoints[0].SampleCount, 1e-9)
}

func TestCloudWatchHandler_PutMetricData_WithDimensions(t *testing.T) {
	t.Parallel()

	h := newCWHandler()

	body := strings.Join([]string{
		"Action=PutMetricData",
		"Namespace=AWS%2FEC2",
		"MetricData.member.1.MetricName=CPUUtilization",
		"MetricData.member.1.Value=75",
		"MetricData.member.1.Dimensions.member.1.Name=InstanceId",
		"MetricData.member.1.Dimensions.member.1.Value=i-123",
	}, "&")
	rec := postForm(t, h, body)
	require.Equal(t, http.StatusOK, rec.Code)

	listBody := "Action=ListMetrics&Namespace=AWS%2FEC2&MetricName=CPUUtilization"
	listRec := postForm(t, h, listBody)
	require.Equal(t, http.StatusOK, listRec.Code)

	type dim struct {
		Name  string `xml:"Name"`
		Value string `xml:"Value"`
	}
	type metric struct {
		MetricName string `xml:"MetricName"`
		Dimensions []dim  `xml:"Dimensions>member"`
	}
	type resp struct {
		Metrics []metric `xml:"ListMetricsResult>Metrics>member"`
	}
	var out resp
	require.NoError(t, xml.Unmarshal(listRec.Body.Bytes(), &out))
	require.Len(t, out.Metrics, 1)
	require.Len(t, out.Metrics[0].Dimensions, 1)
	assert.Equal(t, "InstanceId", out.Metrics[0].Dimensions[0].Name)
	assert.Equal(t, "i-123", out.Metrics[0].Dimensions[0].Value)
}

func TestCloudWatchHandler_GetMetricStatistics_WithDimensions(t *testing.T) {
	t.Parallel()

	h := newCWHandler()

	// Put metrics for two different instance IDs.
	for _, id := range []string{"i-001", "i-002"} {
		body := strings.Join([]string{
			"Action=PutMetricData",
			"Namespace=AWS%2FEC2",
			"MetricData.member.1.MetricName=NetworkIn",
			"MetricData.member.1.Value=100",
			"MetricData.member.1.Dimensions.member.1.Name=InstanceId",
			"MetricData.member.1.Dimensions.member.1.Value=" + id,
		}, "&")
		postForm(t, h, body)
	}

	// Query with the dimension filter for i-001 only.
	statsBody := strings.Join([]string{
		"Action=GetMetricStatistics",
		"Namespace=AWS%2FEC2",
		"MetricName=NetworkIn",
		"Dimensions.member.1.Name=InstanceId",
		"Dimensions.member.1.Value=i-001",
		"StartTime=2000-01-01T00%3A00%3A00Z",
		"EndTime=2100-01-01T00%3A00%3A00Z",
		"Period=600",
		"Statistics.member.1=Sum",
	}, "&")
	rec := postForm(t, h, statsBody)
	require.Equal(t, http.StatusOK, rec.Code)

	type dp struct {
		Sum *float64 `xml:"Sum"`
	}
	type resp struct {
		Datapoints []dp `xml:"GetMetricStatisticsResult>Datapoints>member"`
	}
	var out resp
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.Datapoints, 1)
	assert.InDelta(t, 100.0, *out.Datapoints[0].Sum, 1e-9)
}

func TestCloudWatchHandler_ListMetrics_DimensionFilter(t *testing.T) {
	t.Parallel()

	h := newCWHandler()

	for _, env := range []string{"prod", "staging"} {
		body := strings.Join([]string{
			"Action=PutMetricData",
			"Namespace=App",
			"MetricData.member.1.MetricName=RPM",
			"MetricData.member.1.Value=50",
			"MetricData.member.1.Dimensions.member.1.Name=Env",
			"MetricData.member.1.Dimensions.member.1.Value=" + env,
		}, "&")
		postForm(t, h, body)
	}

	// Filter to prod only.
	listBody := strings.Join([]string{
		"Action=ListMetrics",
		"Namespace=App",
		"Dimensions.member.1.Name=Env",
		"Dimensions.member.1.Value=prod",
	}, "&")
	rec := postForm(t, h, listBody)
	require.Equal(t, http.StatusOK, rec.Code)

	type metric struct {
		MetricName string `xml:"MetricName"`
	}
	type resp struct {
		Metrics []metric `xml:"ListMetricsResult>Metrics>member"`
	}
	var out resp
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))
	assert.Len(t, out.Metrics, 1)
}

func TestCloudWatchHandler_GetMetricData_ScanByDescending(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	h := cloudwatch.NewHandler(b)

	// Put data via backend to avoid timestamp URL-encoding issues.
	base := cloudwatch.RecentTestAnchor()
	for i := 1; i <= 3; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		err := b.PutMetricData("NS", []cloudwatch.MetricDatum{
			{
				MetricName: "Counter", Value: float64(i), Count: 1,
				Sum: float64(i), Min: float64(i), Max: float64(i), Timestamp: ts,
			},
		})
		require.NoError(t, err)
	}

	queryBody := strings.Join([]string{
		"Action=GetMetricData",
		"StartTime=" + url.QueryEscape(base.Format(time.RFC3339)),
		"EndTime=" + url.QueryEscape(base.Add(10*time.Minute).Format(time.RFC3339)),
		"ScanBy=TimestampDescending",
		"MetricDataQueries.member.1.Id=m1",
		"MetricDataQueries.member.1.MetricStat.Metric.Namespace=NS",
		"MetricDataQueries.member.1.MetricStat.Metric.MetricName=Counter",
		"MetricDataQueries.member.1.MetricStat.Stat=Sum",
		"MetricDataQueries.member.1.MetricStat.Period=60",
	}, "&")
	rec := postForm(t, h, queryBody)
	require.Equal(t, http.StatusOK, rec.Code)

	// Decode against the real wire shape (cloudwatch@v1.66.3 schemas.go:
	// GetMetricDataOutput_MetricDataResults = AddMember("MetricDataResults",
	// _MetricDataResults); _MetricDataResults.AddMember("member", ...)) --
	// GetMetricDataResult>MetricDataResults>member, not >member directly. A
	// prior handler bug (a resultEntry.XMLName override silently winning over
	// the parent field's own tag, dropping the <MetricDataResults> level) made
	// this test's old two-segment decode struct match the bug's own wrong
	// output instead of AWS's real one.
	type result struct {
		ID         string   `xml:"Id"`
		Timestamps []string `xml:"Timestamps>member"`
	}
	type resp struct {
		Results []result `xml:"GetMetricDataResult>MetricDataResults>member"`
	}
	var out resp
	require.NoError(t, xml.Unmarshal(rec.Body.Bytes(), &out))
	require.Len(t, out.Results, 1)
	if len(out.Results[0].Timestamps) >= 2 {
		// Descending: later timestamps first.
		assert.Greater(t, out.Results[0].Timestamps[0], out.Results[0].Timestamps[1])
	}
}

// TestCloudWatchHandler_PutMetricData_RejectedOnCap verifies that once a
// namespace is at its distinct-time-series cap, submitting one more new series
// fails the whole PutMetricData call with a real AWS-shaped error (400
// LimitExceeded) rather than a 200 with a fabricated per-datum "unprocessed"
// entry — PutMetricDataOutput carries no such field.
func TestCloudWatchHandler_PutMetricData_RejectedOnCap(t *testing.T) {
	t.Parallel()

	h := cloudwatch.NewHandler(cloudwatch.NewInMemoryBackendWithConfig("123456789012", "us-east-1"))

	// Fill up the namespace to the cap first.
	b := h.Backend.(*cloudwatch.InMemoryBackend)
	ts := cloudwatch.RecentTestAnchor()
	for i := range cloudwatch.CwMaxMetricNamesPerNamespace {
		_ = b.PutMetricData("FullNS", []cloudwatch.MetricDatum{
			{
				MetricName: strings.Repeat("x", 1) + strings.Repeat("y", i%10) + strings.Repeat("z", i/10),
				Value:      1, Count: 1, Sum: 1, Min: 1, Max: 1, Timestamp: ts,
			},
		})
	}

	body := "Action=PutMetricData&Namespace=FullNS&MetricData.member.1.MetricName=Overflow&MetricData.member.1.Value=1"
	rec := postForm(t, h, body)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "LimitExceeded")

	// The overflow metric must not have been stored.
	metric, err := b.ListMetrics("FullNS", "Overflow", nil, "", "", 0)
	require.NoError(t, err)
	assert.Empty(t, metric.Data)
}
