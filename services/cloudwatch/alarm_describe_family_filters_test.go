package cloudwatch_test

import (
	"encoding/xml"
	"net/http"
	"testing"

	"github.com/aws/smithy-go/encoding/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

func newAlarmWithActions(name string, alarmActions, okActions, insufficientActions []string) *cloudwatch.MetricAlarm {
	return &cloudwatch.MetricAlarm{
		AlarmName:               name,
		Namespace:               "NS",
		MetricName:              "M",
		ComparisonOperator:      "GreaterThanThreshold",
		Threshold:               50,
		EvaluationPeriods:       1,
		AlarmActions:            alarmActions,
		OKActions:               okActions,
		InsufficientDataActions: insufficientActions,
	}
}

// ---------------------------------------------------------------------------
// DescribeAlarms: ActionPrefix
// ---------------------------------------------------------------------------

func TestDescribeAlarms_ActionPrefix(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(newAlarmWithActions(
		"via-alarm-actions", []string{"arn:aws:sns:us-east-1:1:topic"}, nil, nil,
	)))
	require.NoError(t, b.PutMetricAlarm(newAlarmWithActions(
		"via-ok-actions", nil, []string{"arn:aws:sns:us-east-1:1:topic"}, nil,
	)))
	require.NoError(t, b.PutMetricAlarm(newAlarmWithActions(
		"via-insufficient-actions", nil, nil, []string{"arn:aws:sns:us-east-1:1:topic"},
	)))
	require.NoError(t, b.PutMetricAlarm(newAlarmWithActions(
		"non-matching", []string{"arn:aws:sns:us-east-1:1:other-topic-with-topic-inside"}, nil, nil,
	)))

	tests := []struct {
		name   string
		want   string
		prefix string
	}{
		{name: "alarm actions", prefix: "arn:aws:sns:us-east-1:1:topic", want: "via-alarm-actions"},
		{name: "ok actions", prefix: "arn:aws:sns:us-east-1:1:topic", want: "via-ok-actions"},
		{
			name:   "insufficient actions",
			prefix: "arn:aws:sns:us-east-1:1:topic",
			want:   "via-insufficient-actions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			alarms, _, _, err := b.DescribeAlarms(nil, nil, "", "", "", 0, tc.prefix, "", "")
			require.NoError(t, err)

			var names []string
			for _, a := range alarms.Data {
				names = append(names, a.AlarmName)
			}
			assert.Contains(t, names, tc.want)
		})
	}
}

func TestDescribeAlarms_ActionPrefix_NotSubstringMatch(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(newAlarmWithActions(
		"non-matching", []string{"arn:aws:sns:us-east-1:1:other-topic-with-topic-inside"}, nil, nil,
	)))

	alarms, _, _, err := b.DescribeAlarms(nil, nil, "", "", "", 0, "arn:aws:sns:us-east-1:1:topic", "", "")
	require.NoError(t, err)
	assert.Empty(t, alarms.Data, "ActionPrefix must be a prefix match, not a substring match")
}

func TestDescribeAlarms_ActionPrefix_EmptyReturnsEverything(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(newAlarmWithActions("a1", nil, nil, nil)))
	require.NoError(t, b.PutMetricAlarm(newAlarmWithActions("a2", []string{"arn:x"}, nil, nil)))

	alarms, _, _, err := b.DescribeAlarms(nil, nil, "", "", "", 0, "", "", "")
	require.NoError(t, err)
	assert.Len(t, alarms.Data, 2, "an unfiltered DescribeAlarms must still return everything")
}

// ---------------------------------------------------------------------------
// DescribeAlarms: ChildrenOfAlarmName
// ---------------------------------------------------------------------------

func TestDescribeAlarms_ChildrenOfAlarmName(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "child-metric", Namespace: "NS", MetricName: "M",
		ComparisonOperator: "GreaterThanThreshold", Threshold: 50, EvaluationPeriods: 1,
		AlarmDescription: "should not be returned",
	}))
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "child-composite", AlarmRule: `ALARM("child-metric")`,
		AlarmDescription: "should not be returned either",
	}))
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "parent",
		AlarmRule: `ALARM("child-metric") OR ALARM("child-composite")`,
	}))

	metrics, composites, logs, err := b.DescribeAlarms(nil, nil, "", "", "", 0, "", "parent", "")
	require.NoError(t, err)

	require.Len(t, metrics.Data, 1)
	assert.Equal(t, "child-metric", metrics.Data[0].AlarmName)
	assert.Empty(t, metrics.Data[0].Namespace, "children response must be abbreviated")
	assert.Empty(t, metrics.Data[0].AlarmDescription, "children response must be abbreviated")
	assert.NotEmpty(t, metrics.Data[0].StateValue)

	require.Len(t, composites.Data, 1)
	assert.Equal(t, "child-composite", composites.Data[0].AlarmName)
	assert.Empty(t, composites.Data[0].AlarmRule, "children response must be abbreviated")
	assert.Empty(t, composites.Data[0].AlarmDescription, "children response must be abbreviated")
	assert.NotEmpty(t, composites.Data[0].StateValue)

	assert.Empty(t, logs.Data, "ChildrenOfAlarmName never returns log alarms")

	for _, c := range composites.Data {
		assert.NotEqual(t, "parent", c.AlarmName, "the named composite alarm itself must not be returned")
	}
}

func TestDescribeAlarms_ChildrenOfAlarmName_NotComposite(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "solo", Namespace: "NS", MetricName: "M",
		ComparisonOperator: "GreaterThanThreshold", Threshold: 50, EvaluationPeriods: 1,
	}))

	metrics, composites, _, err := b.DescribeAlarms(nil, nil, "", "", "", 0, "", "solo", "")
	require.NoError(t, err)
	assert.Empty(t, metrics.Data)
	assert.Empty(t, composites.Data)
}

// ---------------------------------------------------------------------------
// DescribeAlarms: ParentsOfAlarmName
// ---------------------------------------------------------------------------

func TestDescribeAlarms_ParentsOfAlarmName(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "leaf", Namespace: "NS", MetricName: "M",
		ComparisonOperator: "GreaterThanThreshold", Threshold: 50, EvaluationPeriods: 1,
	}))
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "parent", AlarmRule: `ALARM("leaf")`,
		AlarmDescription: "should not be returned",
	}))
	require.NoError(t, b.PutCompositeAlarm(&cloudwatch.CompositeAlarm{
		AlarmName: "unrelated", AlarmRule: `ALARM("some-other-alarm")`,
	}))

	_, composites, _, err := b.DescribeAlarms(nil, nil, "", "", "", 0, "", "", "leaf")
	require.NoError(t, err)

	require.Len(t, composites.Data, 1)
	assert.Equal(t, "parent", composites.Data[0].AlarmName)
	assert.Empty(t, composites.Data[0].AlarmRule, "parents response must be abbreviated to Name+ARN")
	assert.Empty(t, composites.Data[0].StateValue, "parents response must be abbreviated to Name+ARN")
	assert.Empty(t, composites.Data[0].AlarmDescription, "parents response must be abbreviated to Name+ARN")
}

func TestDescribeAlarms_ParentsOfAlarmName_NoMatch(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	require.NoError(t, b.PutMetricAlarm(&cloudwatch.MetricAlarm{
		AlarmName: "orphan", Namespace: "NS", MetricName: "M",
		ComparisonOperator: "GreaterThanThreshold", Threshold: 50, EvaluationPeriods: 1,
	}))

	_, composites, _, err := b.DescribeAlarms(nil, nil, "", "", "", 0, "", "", "orphan")
	require.NoError(t, err)
	assert.Empty(t, composites.Data)
}

// ---------------------------------------------------------------------------
// DescribeAlarms: ChildrenOfAlarmName / ParentsOfAlarmName mutual exclusivity
// ---------------------------------------------------------------------------

func TestDescribeAlarms_FamilyFilters_MutuallyExclusive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		alarmNamePrefix     string
		stateValue          string
		actionPrefix        string
		childrenOfAlarmName string
		parentsOfAlarmName  string
		alarmNames          []string
	}{
		{name: "children and parents together", childrenOfAlarmName: "a", parentsOfAlarmName: "b"},
		{name: "children with AlarmNames", childrenOfAlarmName: "a", alarmNames: []string{"x"}},
		{name: "children with AlarmNamePrefix", childrenOfAlarmName: "a", alarmNamePrefix: "x"},
		{name: "children with StateValue", childrenOfAlarmName: "a", stateValue: "ALARM"},
		{name: "children with ActionPrefix", childrenOfAlarmName: "a", actionPrefix: "arn:x"},
		{name: "parents with AlarmNames", parentsOfAlarmName: "a", alarmNames: []string{"x"}},
		{name: "parents with StateValue", parentsOfAlarmName: "a", stateValue: "ALARM"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackend()
			_, _, _, err := b.DescribeAlarms(
				tc.alarmNames, nil, tc.alarmNamePrefix, tc.stateValue, "", 0,
				tc.actionPrefix, tc.childrenOfAlarmName, tc.parentsOfAlarmName,
			)
			require.ErrorIs(t, err, cloudwatch.ErrValidation)
		})
	}
}

// ---------------------------------------------------------------------------
// Handler: ActionPrefix, both protocols
// ---------------------------------------------------------------------------

func TestHandler_DescribeAlarms_ActionPrefix_BothProtocols(t *testing.T) {
	t.Parallel()

	h := cloudwatch.NewHandler(cloudwatch.NewInMemoryBackend())
	postForm(t, h, "Action=PutMetricAlarm&AlarmName=matching&Namespace=NS&MetricName=M"+
		"&ComparisonOperator=GreaterThanThreshold&Threshold=50&EvaluationPeriods=1"+
		"&AlarmActions.member.1=arn%3Aaws%3Asns%3Aus-east-1%3A1%3Atopic")
	postForm(t, h, "Action=PutMetricAlarm&AlarmName=not-matching&Namespace=NS&MetricName=M"+
		"&ComparisonOperator=GreaterThanThreshold&Threshold=50&EvaluationPeriods=1"+
		"&AlarmActions.member.1=arn%3Aaws%3Asns%3Aus-east-1%3A1%3Aother")

	xmlRec := postForm(t, h, "Action=DescribeAlarms&ActionPrefix=arn%3Aaws%3Asns%3Aus-east-1%3A1%3Atopic")
	require.Equal(t, http.StatusOK, xmlRec.Code)

	type alarm struct {
		AlarmName string `xml:"AlarmName"`
	}
	type resp struct {
		XMLName xml.Name `xml:"DescribeAlarmsResponse"`
		Alarms  []alarm  `xml:"DescribeAlarmsResult>MetricAlarms>member"`
	}
	var xmlOut resp
	require.NoError(t, xml.Unmarshal(xmlRec.Body.Bytes(), &xmlOut))
	require.Len(t, xmlOut.Alarms, 1)
	assert.Equal(t, "matching", xmlOut.Alarms[0].AlarmName)

	cborRec := postCBOR(t, h, "DescribeAlarms", cbor.Map{
		"ActionPrefix": cbor.String("arn:aws:sns:us-east-1:1:topic"),
	})
	require.Equal(t, http.StatusOK, cborRec.Code)
	cborOut := decodeCBORResponse(t, cborRec)
	cborAlarms, ok := cborOut["MetricAlarms"].(cbor.List)
	require.True(t, ok)
	require.Len(t, cborAlarms, 1)
	cborAlarm, ok := cborAlarms[0].(cbor.Map)
	require.True(t, ok)
	assert.Equal(t, cbor.String("matching"), cborAlarm["AlarmName"],
		"Query/XML and rpc-v2-cbor must apply ActionPrefix identically")
}

func TestHandler_DescribeAlarms_FamilyFilterExclusive_BothProtocols(t *testing.T) {
	t.Parallel()

	h := cloudwatch.NewHandler(cloudwatch.NewInMemoryBackend())

	xmlRec := postForm(t, h,
		"Action=DescribeAlarms&ChildrenOfAlarmName=a&AlarmNamePrefix=b")
	assert.Equal(t, http.StatusBadRequest, xmlRec.Code)

	cborRec := postCBOR(t, h, "DescribeAlarms", cbor.Map{
		"ChildrenOfAlarmName": cbor.String("a"),
		"AlarmNamePrefix":     cbor.String("b"),
	})
	assert.Equal(t, http.StatusBadRequest, cborRec.Code)
}
