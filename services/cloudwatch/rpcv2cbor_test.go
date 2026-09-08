package cloudwatch_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/smithy-go/encoding/cbor"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudwatch"
)

const cborTestServicePath = "/service/GraniteServiceVersion20100801/operation/"

// fixedTS is a fixed test anchor point, computed at test-run time so it stays
// inside PutMetricData's write-time Timestamp acceptance window (two weeks
// past / two hours future) regardless of when the suite runs, expressed as a
// Unix timestamp (the CBOR wire encoding for a Timestamp tag).
//
//nolint:gochecknoglobals // test-only fixed reference point
var fixedTS = float64(time.Now().UTC().Add(-time.Hour).Unix())

// postCBOR sends a rpc-v2-cbor POST to the CloudWatch handler.
func postCBOR(t *testing.T, h *cloudwatch.Handler, op string, body cbor.Map) *httptest.ResponseRecorder {
	t.Helper()

	encoded := cbor.Encode(body)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, cborTestServicePath+op, bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/cbor")
	req.Header.Set("Smithy-Protocol", "rpc-v2-cbor")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.Handler()(c))

	return rec
}

func newCBORHandler() *cloudwatch.Handler {
	return cloudwatch.NewHandler(cloudwatch.NewInMemoryBackend())
}

// decodeCBORResponse decodes the CBOR response body into a cbor.Map.
func decodeCBORResponse(t *testing.T, rec *httptest.ResponseRecorder) cbor.Map {
	t.Helper()

	v, err := cbor.Decode(rec.Body.Bytes())
	require.NoError(t, err)

	m, ok := v.(cbor.Map)
	require.True(t, ok, "expected CBOR map response")

	return m
}

func TestCBOR_RouteMatcher(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "matches CBOR",
			path: cborTestServicePath + "PutMetricData",
			want: true,
		},
		{
			name: "rejects unknown op",
			path: cborTestServicePath + "UnknownOp",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newCBORHandler()
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			req.Header.Set("Content-Type", "application/cbor")
			assert.Equal(t, tt.want, h.RouteMatcher()(e.NewContext(req, httptest.NewRecorder())))
		})
	}
}

func TestCBOR_ExtractOperation(t *testing.T) {
	t.Parallel()

	h := newCBORHandler()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, cborTestServicePath+"PutMetricAlarm", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.Equal(t, "PutMetricAlarm", h.ExtractOperation(c))
}

func TestCBOR_DeleteAlarms(t *testing.T) {
	t.Parallel()

	h := newCBORHandler()
	postCBOR(t, h, "PutMetricAlarm", cbor.Map{
		"AlarmName":          cbor.String("to-delete"),
		"ComparisonOperator": cbor.String("GreaterThanThreshold"),
		"Threshold":          cbor.Float64(1.0),
		"EvaluationPeriods":  cbor.Uint(1),
		"Period":             cbor.Uint(60),
	})

	rec := postCBOR(t, h, "DeleteAlarms", cbor.Map{
		"AlarmNames": cbor.List{cbor.String("to-delete")},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	descRec := postCBOR(t, h, "DescribeAlarms", cbor.Map{
		"AlarmNames": cbor.List{cbor.String("to-delete")},
	})
	m := decodeCBORResponse(t, descRec)
	alarms := m["MetricAlarms"].(cbor.List)
	assert.Empty(t, alarms)
}

func TestCBOR_InvalidBody(t *testing.T) {
	t.Parallel()

	h := newCBORHandler()
	e := echo.New()
	req := httptest.NewRequest(
		http.MethodPost,
		cborTestServicePath+"PutMetricData",
		bytes.NewReader([]byte{0x00, 0xFF, 0xAA}),
	)
	req.Header.Set("Content-Type", "application/cbor")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.Handler()(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCBOR_EmptyBody(t *testing.T) {
	t.Parallel()

	h := newCBORHandler()
	e := echo.New()
	req := httptest.NewRequest(
		http.MethodPost,
		cborTestServicePath+"PutMetricData",
		bytes.NewReader(nil),
	)
	req.Header.Set("Content-Type", "application/cbor")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	require.NoError(t, h.Handler()(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCBOR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup            func(t *testing.T, h *cloudwatch.Handler)
		body             cbor.Map
		name             string
		op               string
		wantStringField  string
		wantStringValue  string
		wantListField    string
		wantErrorType    string
		wantCode         int
		wantListLen      int
		wantProtocol     bool
		wantListNotEmpty bool
		wantListEmpty    bool
	}{
		{
			name: "PutMetricData",
			op:   "PutMetricData",
			body: cbor.Map{
				"Namespace": cbor.String("TestNS"),
				"MetricData": cbor.List{
					cbor.Map{
						"MetricName": cbor.String("Latency"),
						"Value":      cbor.Float64(123.0),
						"Timestamp":  cbor.Tag{ID: 1, Value: cbor.Float64(fixedTS)},
					},
				},
			},
			wantCode:     http.StatusOK,
			wantProtocol: true,
		},
		{
			// Regression coverage for gopherstack-7fyf: the CBOR error body
			// must carry "__type" (not just the X-Amzn-Errortype header),
			// because rpc-v2-cbor's generated deserializer
			// (getProtocolErrorInfo) resolves the exception name from the
			// decoded CBOR payload, never from a header.
			name:          "PutMetricData/missing namespace",
			op:            "PutMetricData",
			body:          cbor.Map{},
			wantCode:      http.StatusBadRequest,
			wantProtocol:  true,
			wantErrorType: "InvalidParameterValue",
		},
		{
			name: "PutAndGetMetricStatistics",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				putRec := postCBOR(t, h, "PutMetricData", cbor.Map{
					"Namespace": cbor.String("StatNS"),
					"MetricData": cbor.List{
						cbor.Map{
							"MetricName": cbor.String("Requests"),
							"Value":      cbor.Float64(50.0),
							"Timestamp":  cbor.Tag{ID: 1, Value: cbor.Float64(fixedTS)},
						},
					},
				})
				require.Equal(t, http.StatusOK, putRec.Code)
			},
			op: "GetMetricStatistics",
			body: cbor.Map{
				"Namespace":  cbor.String("StatNS"),
				"MetricName": cbor.String("Requests"),
				"StartTime":  cbor.Tag{ID: 1, Value: cbor.Float64(fixedTS - 3600)},
				"EndTime":    cbor.Tag{ID: 1, Value: cbor.Float64(fixedTS + 60)},
				"Period":     cbor.Uint(3600),
				"Statistics": cbor.List{cbor.String("Sum")},
			},
			wantCode:         http.StatusOK,
			wantStringField:  "Label",
			wantStringValue:  "Requests",
			wantListField:    "Datapoints",
			wantListNotEmpty: true,
		},
		{
			name: "PutMetricAlarm",
			op:   "PutMetricAlarm",
			body: cbor.Map{
				"AlarmName":          cbor.String("test-alarm"),
				"Namespace":          cbor.String("TestNS"),
				"MetricName":         cbor.String("Errors"),
				"ComparisonOperator": cbor.String("GreaterThanThreshold"),
				"Statistic":          cbor.String("Sum"),
				"Threshold":          cbor.Float64(10.0),
				"EvaluationPeriods":  cbor.Uint(1),
				"Period":             cbor.Uint(60),
			},
			wantCode: http.StatusOK,
		},
		{
			name: "PutAlarmMuteRule",
			op:   "PutAlarmMuteRule",
			body: cbor.Map{
				"Name":        cbor.String("mute-cbor"),
				"Description": cbor.String("cbor mute"),
				"Rule": cbor.Map{
					"Schedule": cbor.Map{
						"Expression": cbor.String("cron(0 2 * * *)"),
						"Duration":   cbor.String("PT1H"),
					},
				},
				"MuteTargets": cbor.Map{
					"AlarmNames": cbor.List{cbor.String("alarm-a")},
				},
			},
			wantCode: http.StatusOK,
		},
		{
			name: "PutInsightRule",
			op:   "PutInsightRule",
			body: cbor.Map{
				"RuleName":       cbor.String("cbor-rule"),
				"RuleState":      cbor.String("ENABLED"),
				"RuleDefinition": cbor.String(validInsightRuleDefinition),
			},
			wantCode: http.StatusOK,
		},
		{
			name: "PutMetricStream",
			op:   "PutMetricStream",
			body: cbor.Map{
				"Name":         cbor.String("stream-cbor"),
				"FirehoseArn":  cbor.String("arn:aws:firehose:us-east-1:123456789012:deliverystream/main"),
				"RoleArn":      cbor.String("arn:aws:iam::123456789012:role/main"),
				"OutputFormat": cbor.String("json"),
			},
			wantCode: http.StatusOK,
		},
		{
			// Real CloudWatch has no UpdateAlarmMuteRule/UpdateInsightRule/
			// UpdateMetricStream operations -- confirm they are rejected as
			// unknown ops (InvalidAction), not silently routed anywhere.
			name:     "UpdateAlarmMuteRule/not a real op",
			op:       "UpdateAlarmMuteRule",
			body:     cbor.Map{"Name": cbor.String("missing-mute")},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "UpdateInsightRule/not a real op",
			op:       "UpdateInsightRule",
			body:     cbor.Map{"RuleName": cbor.String("missing-rule")},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "UpdateMetricStream/not a real op",
			op:       "UpdateMetricStream",
			body:     cbor.Map{"Name": cbor.String("missing-stream")},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "DescribeAlarms",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutMetricAlarm", cbor.Map{
					"AlarmName":          cbor.String("my-alarm"),
					"Namespace":          cbor.String("NS"),
					"MetricName":         cbor.String("M"),
					"ComparisonOperator": cbor.String("GreaterThanThreshold"),
					"Threshold":          cbor.Float64(5.0),
					"EvaluationPeriods":  cbor.Uint(1),
					"Period":             cbor.Uint(60),
				})
			},
			op: "DescribeAlarms",
			body: cbor.Map{
				"AlarmNames": cbor.List{cbor.String("my-alarm")},
			},
			wantCode:      http.StatusOK,
			wantListField: "MetricAlarms",
			wantListLen:   1,
		},
		{
			name: "ListMetrics",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutMetricData", cbor.Map{
					"Namespace": cbor.String("ListNS"),
					"MetricData": cbor.List{
						cbor.Map{
							"MetricName": cbor.String("CPU"),
							"Value":      cbor.Float64(80.0),
							"Timestamp":  cbor.Tag{ID: 1, Value: cbor.Float64(fixedTS)},
						},
					},
				})
			},
			op: "ListMetrics",
			body: cbor.Map{
				"Namespace": cbor.String("ListNS"),
			},
			wantCode:         http.StatusOK,
			wantListField:    "Metrics",
			wantListNotEmpty: true,
		},
		{
			name:          "UnknownOperation",
			op:            "NotAnOp",
			body:          cbor.Map{},
			wantCode:      http.StatusBadRequest,
			wantProtocol:  true,
			wantErrorType: "InvalidAction",
		},
		{
			name: "GetMetricData",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutMetricData", cbor.Map{
					"Namespace": cbor.String("MDataNS"),
					"MetricData": cbor.List{
						cbor.Map{
							"MetricName": cbor.String("Errors"),
							"Value":      cbor.Float64(42.0),
							"Timestamp":  cbor.Tag{ID: 1, Value: cbor.Float64(fixedTS)},
						},
					},
				})
			},
			op: "GetMetricData",
			body: cbor.Map{
				"StartTime": cbor.Tag{ID: 1, Value: cbor.Float64(fixedTS - 3600)},
				"EndTime":   cbor.Tag{ID: 1, Value: cbor.Float64(fixedTS + 60)},
				"MetricDataQueries": cbor.List{
					cbor.Map{
						"Id":    cbor.String("q1"),
						"Label": cbor.String("ErrorCount"),
						"MetricStat": cbor.Map{
							"Stat":   cbor.String("Sum"),
							"Period": cbor.Uint(3600),
							"Metric": cbor.Map{
								"Namespace":  cbor.String("MDataNS"),
								"MetricName": cbor.String("Errors"),
							},
						},
					},
				},
			},
			wantCode:         http.StatusOK,
			wantProtocol:     true,
			wantListField:    "MetricDataResults",
			wantListNotEmpty: true,
		},
		{
			name:          "GetMetricData/empty queries",
			op:            "GetMetricData",
			body:          cbor.Map{},
			wantCode:      http.StatusOK,
			wantListField: "MetricDataResults",
			wantListEmpty: true,
		},
		{
			name:          "PutMetricAlarm/missing name",
			op:            "PutMetricAlarm",
			body:          cbor.Map{},
			wantCode:      http.StatusBadRequest,
			wantProtocol:  true,
			wantErrorType: "InvalidParameterValue",
		},
		{
			name:          "DescribeAlarms/empty",
			op:            "DescribeAlarms",
			body:          cbor.Map{},
			wantCode:      http.StatusOK,
			wantListField: "MetricAlarms",
			wantListEmpty: true,
		},
		{
			name: "TagResource",
			op:   "TagResource",
			body: cbor.Map{
				"ResourceARN": cbor.String("arn:aws:cloudwatch:us-east-1:123456789:alarm:test"),
				"Tags": cbor.List{
					cbor.Map{
						"Key":   cbor.String("env"),
						"Value": cbor.String("prod"),
					},
				},
			},
			wantCode: http.StatusOK,
		},
		{
			name: "ListTagsForResource/empty",
			op:   "ListTagsForResource",
			body: cbor.Map{
				"ResourceARN": cbor.String("arn:aws:cloudwatch:us-east-1:123456789:alarm:none"),
			},
			wantCode:      http.StatusOK,
			wantListField: "Tags",
			wantListEmpty: true,
		},
		{
			name: "ListTagsForResource/with tags",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "TagResource", cbor.Map{
					"ResourceARN": cbor.String("arn:aws:cloudwatch:us-east-1:123456789:alarm:tagged"),
					"Tags": cbor.List{
						cbor.Map{
							"Key":   cbor.String("env"),
							"Value": cbor.String("prod"),
						},
					},
				})
			},
			op: "ListTagsForResource",
			body: cbor.Map{
				"ResourceARN": cbor.String("arn:aws:cloudwatch:us-east-1:123456789:alarm:tagged"),
			},
			wantCode:         http.StatusOK,
			wantListField:    "Tags",
			wantListNotEmpty: true,
			wantListLen:      1,
		},
		{
			name: "UntagResource",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "TagResource", cbor.Map{
					"ResourceARN": cbor.String("arn:aws:cloudwatch:us-east-1:123456789:alarm:untag"),
					"Tags": cbor.List{
						cbor.Map{
							"Key":   cbor.String("env"),
							"Value": cbor.String("prod"),
						},
					},
				})
			},
			op: "UntagResource",
			body: cbor.Map{
				"ResourceARN": cbor.String("arn:aws:cloudwatch:us-east-1:123456789:alarm:untag"),
				"TagKeys":     cbor.List{cbor.String("env")},
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newCBORHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postCBOR(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantProtocol {
				assert.Equal(t, "rpc-v2-cbor", rec.Header().Get("Smithy-Protocol"))
			}

			if tt.wantErrorType != "" {
				// The rpc-v2-cbor SDK deserializer resolves the exception
				// name from "__type" in the decoded CBOR body, not from the
				// X-Amzn-Errortype header -- both must carry it.
				assert.Equal(t, tt.wantErrorType, rec.Header().Get("X-Amzn-Errortype"))

				m := decodeCBORResponse(t, rec)
				typeVal, ok := m["__type"].(cbor.String)
				require.True(t, ok, "CBOR error body must include __type")
				assert.Equal(t, tt.wantErrorType, string(typeVal))
			}

			if tt.wantStringField != "" {
				m := decodeCBORResponse(t, rec)
				assert.Equal(t, tt.wantStringValue, string(m[tt.wantStringField].(cbor.String)))
			}

			if tt.wantListField != "" {
				m := decodeCBORResponse(t, rec)
				list, ok := m[tt.wantListField].(cbor.List)
				require.True(t, ok)

				if tt.wantListNotEmpty {
					assert.NotEmpty(t, list)
				}

				if tt.wantListEmpty {
					assert.Empty(t, list)
				}

				if tt.wantListLen > 0 {
					assert.Len(t, list, tt.wantListLen)
				}
			}
		})
	}
}

func TestCBOR_NewOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup             func(t *testing.T, h *cloudwatch.Handler)
		body              cbor.Map
		name              string
		op                string
		wantListField     string
		wantField         string
		wantFieldContains string
		wantCode          int
		wantListLen       int
		wantListNotEmpty  bool
		wantListEmpty     bool
	}{
		// PutCompositeAlarm
		{
			name: "PutCompositeAlarm/success",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutMetricAlarm", cbor.Map{
					"AlarmName":          cbor.String("child-cbor"),
					"Namespace":          cbor.String("NS"),
					"MetricName":         cbor.String("M"),
					"ComparisonOperator": cbor.String("GreaterThanThreshold"),
					"Threshold":          cbor.Float64(1.0),
					"EvaluationPeriods":  cbor.Uint(1),
					"Period":             cbor.Uint(60),
				})
			},
			op: "PutCompositeAlarm",
			body: cbor.Map{
				"AlarmName":    cbor.String("parent-cbor"),
				"AlarmRule":    cbor.String(`ALARM("child-cbor")`),
				"AlarmActions": cbor.List{cbor.String("arn:aws:sns:us-east-1:123:t1")},
			},
			wantCode: http.StatusOK,
		},
		{
			name: "PutCompositeAlarm/missing name",
			op:   "PutCompositeAlarm",
			body: cbor.Map{
				"AlarmRule": cbor.String(`ALARM("x")`),
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "PutCompositeAlarm/missing rule",
			op:   "PutCompositeAlarm",
			body: cbor.Map{
				"AlarmName": cbor.String("x"),
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "PutCompositeAlarm/actions_disabled",
			op:   "PutCompositeAlarm",
			body: cbor.Map{
				"AlarmName":      cbor.String("comp-disabled-cbor"),
				"AlarmRule":      cbor.String(`ALARM("x")`),
				"ActionsEnabled": cbor.Bool(false),
			},
			wantCode: http.StatusOK,
		},
		// DescribeAlarmsForMetric
		{
			name: "DescribeAlarmsForMetric/success",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutMetricAlarm", cbor.Map{
					"AlarmName":          cbor.String("cpu-cbor"),
					"Namespace":          cbor.String("AWS/EC2"),
					"MetricName":         cbor.String("CPUUtilization"),
					"ComparisonOperator": cbor.String("GreaterThanThreshold"),
					"Threshold":          cbor.Float64(80.0),
					"EvaluationPeriods":  cbor.Uint(1),
					"Period":             cbor.Uint(60),
				})
			},
			op: "DescribeAlarmsForMetric",
			body: cbor.Map{
				"Namespace":  cbor.String("AWS/EC2"),
				"MetricName": cbor.String("CPUUtilization"),
			},
			wantCode:         http.StatusOK,
			wantListField:    "MetricAlarms",
			wantListNotEmpty: true,
		},
		{
			name: "DescribeAlarmsForMetric/empty",
			op:   "DescribeAlarmsForMetric",
			body: cbor.Map{
				"Namespace":  cbor.String("AWS/EC2"),
				"MetricName": cbor.String("NotExist"),
			},
			wantCode:      http.StatusOK,
			wantListField: "MetricAlarms",
			wantListEmpty: true,
		},
		// DescribeAlarmHistory
		{
			name: "DescribeAlarmHistory/success",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutMetricAlarm", cbor.Map{
					"AlarmName":          cbor.String("hist-cbor"),
					"Namespace":          cbor.String("NS"),
					"MetricName":         cbor.String("M"),
					"ComparisonOperator": cbor.String("GreaterThanThreshold"),
					"Threshold":          cbor.Float64(1.0),
					"EvaluationPeriods":  cbor.Uint(1),
					"Period":             cbor.Uint(60),
				})
				postCBOR(t, h, "SetAlarmState", cbor.Map{
					"AlarmName":   cbor.String("hist-cbor"),
					"StateValue":  cbor.String("ALARM"),
					"StateReason": cbor.String("test"),
				})
			},
			op: "DescribeAlarmHistory",
			body: cbor.Map{
				"AlarmName": cbor.String("hist-cbor"),
			},
			wantCode:         http.StatusOK,
			wantListField:    "AlarmHistoryItems",
			wantListNotEmpty: true,
		},
		{
			name: "DescribeAlarmHistory/with_dates",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutMetricAlarm", cbor.Map{
					"AlarmName":          cbor.String("date-cbor"),
					"Namespace":          cbor.String("NS"),
					"MetricName":         cbor.String("M"),
					"ComparisonOperator": cbor.String("GreaterThanThreshold"),
					"Threshold":          cbor.Float64(1.0),
					"EvaluationPeriods":  cbor.Uint(1),
					"Period":             cbor.Uint(60),
				})
			},
			op: "DescribeAlarmHistory",
			body: cbor.Map{
				"AlarmName": cbor.String("date-cbor"),
				"StartDate": cbor.Tag{ID: 1, Value: cbor.Float64(fixedTS - 3600)},
				"EndDate":   cbor.Tag{ID: 1, Value: cbor.Float64(fixedTS + 3600)},
			},
			wantCode:      http.StatusOK,
			wantListField: "AlarmHistoryItems",
			wantListEmpty: true,
		},
		// SetAlarmState
		{
			name: "SetAlarmState/success",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutMetricAlarm", cbor.Map{
					"AlarmName":          cbor.String("state-cbor"),
					"Namespace":          cbor.String("NS"),
					"MetricName":         cbor.String("M"),
					"ComparisonOperator": cbor.String("GreaterThanThreshold"),
					"Threshold":          cbor.Float64(1.0),
					"EvaluationPeriods":  cbor.Uint(1),
					"Period":             cbor.Uint(60),
				})
			},
			op: "SetAlarmState",
			body: cbor.Map{
				"AlarmName":   cbor.String("state-cbor"),
				"StateValue":  cbor.String("ALARM"),
				"StateReason": cbor.String("manual"),
			},
			wantCode: http.StatusOK,
		},
		{
			name: "SetAlarmState/missing name",
			op:   "SetAlarmState",
			body: cbor.Map{
				"StateValue": cbor.String("ALARM"),
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "SetAlarmState/not found",
			op:   "SetAlarmState",
			body: cbor.Map{
				"AlarmName":  cbor.String("not-exist-cbor"),
				"StateValue": cbor.String("ALARM"),
			},
			wantCode: http.StatusBadRequest,
		},
		// EnableAlarmActions
		{
			name: "EnableAlarmActions/success",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutMetricAlarm", cbor.Map{
					"AlarmName":          cbor.String("enable-cbor"),
					"Namespace":          cbor.String("NS"),
					"MetricName":         cbor.String("M"),
					"ComparisonOperator": cbor.String("GreaterThanThreshold"),
					"Threshold":          cbor.Float64(1.0),
					"EvaluationPeriods":  cbor.Uint(1),
					"Period":             cbor.Uint(60),
				})
			},
			op: "EnableAlarmActions",
			body: cbor.Map{
				"AlarmNames": cbor.List{cbor.String("enable-cbor")},
			},
			wantCode: http.StatusOK,
		},
		// DisableAlarmActions
		{
			name: "DisableAlarmActions/success",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutMetricAlarm", cbor.Map{
					"AlarmName":          cbor.String("disable-cbor"),
					"Namespace":          cbor.String("NS"),
					"MetricName":         cbor.String("M"),
					"ComparisonOperator": cbor.String("GreaterThanThreshold"),
					"Threshold":          cbor.Float64(1.0),
					"EvaluationPeriods":  cbor.Uint(1),
					"Period":             cbor.Uint(60),
				})
			},
			op: "DisableAlarmActions",
			body: cbor.Map{
				"AlarmNames": cbor.List{cbor.String("disable-cbor")},
			},
			wantCode: http.StatusOK,
		},
		// PutLogAlarm
		{
			name:     "PutLogAlarm/success",
			op:       "PutLogAlarm",
			body:     validLogAlarmCBORBody("log-cbor-1"),
			wantCode: http.StatusOK,
		},
		{
			name:     "PutLogAlarm/missing name",
			op:       "PutLogAlarm",
			body:     cbor.Map{"ComparisonOperator": cbor.String("GreaterThanThreshold")},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "DescribeAlarms/log_alarm_excluded_by_default",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutLogAlarm", validLogAlarmCBORBody("log-cbor-hidden"))
			},
			op:            "DescribeAlarms",
			body:          cbor.Map{},
			wantCode:      http.StatusOK,
			wantListField: "LogAlarms",
			wantListEmpty: true,
		},
		{
			name: "DescribeAlarms/log_alarm_included_when_requested",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutLogAlarm", validLogAlarmCBORBody("log-cbor-shown"))
			},
			op: "DescribeAlarms",
			body: cbor.Map{
				"AlarmTypes": cbor.List{cbor.String("LogAlarm")},
			},
			wantCode:         http.StatusOK,
			wantListField:    "LogAlarms",
			wantListNotEmpty: true,
		},
		// GetDataset / AssociateDatasetKmsKey / DisassociateDatasetKmsKey
		{
			name:              "GetDataset/default",
			op:                "GetDataset",
			body:              cbor.Map{"DatasetIdentifier": cbor.String("default")},
			wantCode:          http.StatusOK,
			wantField:         "DatasetId",
			wantFieldContains: "default",
		},
		{
			name:     "GetDataset/unsupported identifier",
			op:       "GetDataset",
			body:     cbor.Map{"DatasetIdentifier": cbor.String("not-default")},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "AssociateDatasetKmsKey/success",
			op:   "AssociateDatasetKmsKey",
			body: cbor.Map{
				"DatasetIdentifier": cbor.String("default"),
				"KmsKeyArn": cbor.String(
					"arn:aws:kms:us-east-1:123456789012:key/1234abcd-12ab-34cd-56ef-1234567890ab",
				),
			},
			wantCode: http.StatusOK,
		},
		{
			name: "AssociateDatasetKmsKey/invalid key arn",
			op:   "AssociateDatasetKmsKey",
			body: cbor.Map{
				"DatasetIdentifier": cbor.String("default"),
				"KmsKeyArn":         cbor.String("not-an-arn"),
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "DisassociateDatasetKmsKey/no key associated",
			op:       "DisassociateDatasetKmsKey",
			body:     cbor.Map{"DatasetIdentifier": cbor.String("default")},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "DisassociateDatasetKmsKey/success",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "AssociateDatasetKmsKey", cbor.Map{
					"DatasetIdentifier": cbor.String("default"),
					"KmsKeyArn": cbor.String(
						"arn:aws:kms:us-east-1:123456789012:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					),
				})
			},
			op:       "DisassociateDatasetKmsKey",
			body:     cbor.Map{"DatasetIdentifier": cbor.String("default")},
			wantCode: http.StatusOK,
		},
		// GetOTelEnrichment / StartOTelEnrichment / StopOTelEnrichment
		{
			name:              "GetOTelEnrichment/default_stopped",
			op:                "GetOTelEnrichment",
			body:              cbor.Map{},
			wantCode:          http.StatusOK,
			wantField:         "Status",
			wantFieldContains: "Stopped",
		},
		{
			name:     "StartOTelEnrichment/success",
			op:       "StartOTelEnrichment",
			body:     cbor.Map{},
			wantCode: http.StatusOK,
		},
		{
			name: "GetOTelEnrichment/after_start_is_running",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "StartOTelEnrichment", cbor.Map{})
			},
			op:                "GetOTelEnrichment",
			body:              cbor.Map{},
			wantCode:          http.StatusOK,
			wantField:         "Status",
			wantFieldContains: "Running",
		},
		{
			name:     "StopOTelEnrichment/success",
			op:       "StopOTelEnrichment",
			body:     cbor.Map{},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newCBORHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postCBOR(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantListField != "" {
				m := decodeCBORResponse(t, rec)
				list, ok := m[tt.wantListField].(cbor.List)
				require.True(t, ok)

				if tt.wantListNotEmpty {
					assert.NotEmpty(t, list)
				}
				if tt.wantListEmpty {
					assert.Empty(t, list)
				}
				if tt.wantListLen > 0 {
					assert.Len(t, list, tt.wantListLen)
				}
			}

			if tt.wantField != "" {
				m := decodeCBORResponse(t, rec)
				v, ok := m[tt.wantField].(cbor.String)
				require.True(t, ok)
				assert.Contains(t, string(v), tt.wantFieldContains)
			}
		})
	}
}

// validLogAlarmCBORBody builds a PutLogAlarm CBOR request body that satisfies
// backend validation, for tests that only care about the alarm existing.
func validLogAlarmCBORBody(alarmName string) cbor.Map {
	return cbor.Map{
		"AlarmName":              cbor.String(alarmName),
		"ComparisonOperator":     cbor.String("GreaterThanThreshold"),
		"Threshold":              cbor.Float64(1.0),
		"QueryResultsToAlarm":    cbor.Uint(1),
		"QueryResultsToEvaluate": cbor.Uint(1),
		"ScheduledQueryConfiguration": cbor.Map{
			"AggregationExpression": cbor.String("count(*)"),
			"QueryString":           cbor.String("fields @message"),
			"ScheduledQueryRoleARN": cbor.String("arn:aws:iam::123456789012:role/cw-log-alarm"),
			"ScheduleConfiguration": cbor.Map{
				"ScheduleExpression": cbor.String("rate(5 minutes)"),
				"StartTimeOffset":    cbor.Uint(300),
			},
		},
	}
}

func TestCBOR_Dashboards(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup    func(t *testing.T, h *cloudwatch.Handler)
		body     cbor.Map
		wantBody func(t *testing.T, m cbor.Map)
		name     string
		op       string
		wantCode int
	}{
		{
			name: "PutDashboard/success",
			op:   "PutDashboard",
			body: cbor.Map{
				"DashboardName": cbor.String("test-dash"),
				"DashboardBody": cbor.String(`{"widgets":[]}`),
			},
			wantCode: http.StatusOK,
		},
		{
			name:     "PutDashboard/missing_name",
			op:       "PutDashboard",
			body:     cbor.Map{"DashboardBody": cbor.String(`{}`)},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "GetDashboard/success",
			op:   "GetDashboard",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutDashboard", cbor.Map{
					"DashboardName": cbor.String("my-dash"),
					"DashboardBody": cbor.String(`{"widgets":[]}`),
				})
			},
			body:     cbor.Map{"DashboardName": cbor.String("my-dash")},
			wantCode: http.StatusOK,
			wantBody: func(t *testing.T, m cbor.Map) {
				t.Helper()
				assert.Equal(t, cbor.String("my-dash"), m["DashboardName"])
				bodyVal, ok := m["DashboardBody"].(cbor.String)
				require.True(t, ok)
				assert.JSONEq(t, `{"widgets":[]}`, string(bodyVal))
			},
		},
		{
			name:     "GetDashboard/not_found",
			op:       "GetDashboard",
			body:     cbor.Map{"DashboardName": cbor.String("no-such-dash")},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "GetDashboard/missing_name",
			op:       "GetDashboard",
			body:     cbor.Map{},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "ListDashboards/success",
			op:   "ListDashboards",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutDashboard", cbor.Map{
					"DashboardName": cbor.String("list-dash-1"),
					"DashboardBody": cbor.String(`{}`),
				})
			},
			body:     cbor.Map{},
			wantCode: http.StatusOK,
			wantBody: func(t *testing.T, m cbor.Map) {
				t.Helper()
				entries, ok := m["DashboardEntries"].(cbor.List)
				require.True(t, ok)
				assert.NotEmpty(t, entries)
			},
		},
		{
			name: "DeleteDashboards/success",
			op:   "DeleteDashboards",
			setup: func(t *testing.T, h *cloudwatch.Handler) {
				t.Helper()
				postCBOR(t, h, "PutDashboard", cbor.Map{
					"DashboardName": cbor.String("del-dash"),
					"DashboardBody": cbor.String(`{}`),
				})
			},
			body:     cbor.Map{"DashboardNames": cbor.List{cbor.String("del-dash")}},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newCBORHandler()
			if tt.setup != nil {
				tt.setup(t, h)
			}

			rec := postCBOR(t, h, tt.op, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			if tt.wantBody != nil {
				m := decodeCBORResponse(t, rec)
				tt.wantBody(t, m)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Accuracy audit CBOR tests — gaps from issue #1686
// ---------------------------------------------------------------------------

func TestCBOR_PutMetricData_WithDimensions(t *testing.T) {
	t.Parallel()

	h := newCBORHandler()

	dims := cbor.List{
		cbor.Map{"Name": cbor.String("InstanceId"), "Value": cbor.String("i-abc")},
	}
	body := cbor.Map{
		"Namespace": cbor.String("AWS/EC2"),
		"MetricData": cbor.List{
			cbor.Map{
				"MetricName": cbor.String("CPUUtilization"),
				"Value":      cbor.Float64(42.5),
				"Dimensions": dims,
			},
		},
	}
	rec := postCBOR(t, h, "PutMetricData", body)
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify metric was stored with dimension via ListMetrics.
	listBody := cbor.Map{
		"Namespace":  cbor.String("AWS/EC2"),
		"MetricName": cbor.String("CPUUtilization"),
	}
	listRec := postCBOR(t, h, "ListMetrics", listBody)
	require.Equal(t, http.StatusOK, listRec.Code)

	resp := decodeCBORResponse(t, listRec)
	metrics, ok := resp["Metrics"].(cbor.List)
	require.True(t, ok)
	require.Len(t, metrics, 1)

	m := metrics[0].(cbor.Map)
	dimList, ok := m["Dimensions"].(cbor.List)
	require.True(t, ok)
	require.Len(t, dimList, 1)
	dimMap := dimList[0].(cbor.Map)
	assert.Equal(t, "i-abc", string(dimMap["Value"].(cbor.String)))
}

func TestCBOR_PutMetricData_WithStatisticValues(t *testing.T) {
	t.Parallel()

	h := newCBORHandler()

	ss := cbor.Map{
		"SampleCount": cbor.Float64(10),
		"Sum":         cbor.Float64(250),
		"Minimum":     cbor.Float64(20),
		"Maximum":     cbor.Float64(35),
	}
	body := cbor.Map{
		"Namespace": cbor.String("App"),
		"MetricData": cbor.List{
			cbor.Map{
				"MetricName":      cbor.String("Latency"),
				"StatisticValues": ss,
			},
		},
	}
	rec := postCBOR(t, h, "PutMetricData", body)
	require.Equal(t, http.StatusOK, rec.Code)

	statsBody := cbor.Map{
		"Namespace":  cbor.String("App"),
		"MetricName": cbor.String("Latency"),
		"StartTime":  cbor.Tag{ID: 1, Value: cbor.Float64(0)},
		"EndTime":    cbor.Tag{ID: 1, Value: cbor.Float64(float64(time.Now().UTC().Add(24 * time.Hour).Unix()))},
		"Period":     cbor.Uint(3600),
		"Statistics": cbor.List{
			cbor.String("Sum"), cbor.String("SampleCount"),
		},
	}
	statsRec := postCBOR(t, h, "GetMetricStatistics", statsBody)
	require.Equal(t, http.StatusOK, statsRec.Code)

	resp := decodeCBORResponse(t, statsRec)
	dps, ok := resp["Datapoints"].(cbor.List)
	require.True(t, ok)
	require.Len(t, dps, 1)
	dp := dps[0].(cbor.Map)
	assert.InDelta(t, 250.0, float64(dp["Sum"].(cbor.Float64)), 1e-9)
	assert.InDelta(t, 10.0, float64(dp["SampleCount"].(cbor.Float64)), 1e-9)
}

// TestCBOR_PutMetricData_TimestampOutOfRange verifies real AWS behaviour
// (api_op_PutMetricData.go: "You can specify time stamps that are as much as
// two weeks before the current date, and as much as 2 hours after the
// current day and time.") -- a Timestamp outside that window must fail with
// 400 InvalidParameterValueException, not fall through to a 500 as an
// unmapped error.
func TestCBOR_PutMetricData_TimestampOutOfRange(t *testing.T) {
	t.Parallel()

	h := newCBORHandler()
	tooOld := float64(time.Now().UTC().Add(-15 * 24 * time.Hour).Unix())
	body := cbor.Map{
		"Namespace": cbor.String("TestNS"),
		"MetricData": cbor.List{
			cbor.Map{
				"MetricName": cbor.String("Latency"),
				"Value":      cbor.Float64(123.0),
				"Timestamp":  cbor.Tag{ID: 1, Value: cbor.Float64(tooOld)},
			},
		},
	}
	rec := postCBOR(t, h, "PutMetricData", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "InvalidParameterValueException", rec.Header().Get("X-Amzn-Errortype"))
}

func TestCBOR_GetMetricStatistics_WithDimensions(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	h := cloudwatch.NewHandler(b)

	dims := []cloudwatch.Dimension{{Name: "Host", Value: "h1"}}
	_ = b.PutMetricData("App", []cloudwatch.MetricDatum{
		{
			MetricName: "Load", Value: 80, Count: 1, Sum: 80, Min: 80, Max: 80,
			Timestamp: time.Now().UTC(), Dimensions: dims,
		},
	})

	statsBody := cbor.Map{
		"Namespace":  cbor.String("App"),
		"MetricName": cbor.String("Load"),
		"Dimensions": cbor.List{
			cbor.Map{"Name": cbor.String("Host"), "Value": cbor.String("h1")},
		},
		"StartTime":  cbor.Tag{ID: 1, Value: cbor.Float64(float64(time.Now().UTC().Add(-time.Hour).Unix()))},
		"EndTime":    cbor.Tag{ID: 1, Value: cbor.Float64(float64(time.Now().UTC().Add(time.Hour).Unix()))},
		"Period":     cbor.Uint(3600),
		"Statistics": cbor.List{cbor.String("Sum")},
	}
	rec := postCBOR(t, h, "GetMetricStatistics", statsBody)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeCBORResponse(t, rec)
	dps, ok := resp["Datapoints"].(cbor.List)
	require.True(t, ok)
	require.Len(t, dps, 1)
	dp := dps[0].(cbor.Map)
	assert.InDelta(t, 80.0, float64(dp["Sum"].(cbor.Float64)), 1e-9)
}

func TestCBOR_GetMetricData_WithDimensions(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	h := cloudwatch.NewHandler(b)

	dimsA := []cloudwatch.Dimension{{Name: "Shard", Value: "a"}}
	ts := time.Now().UTC().Add(-30 * time.Second)
	_ = b.PutMetricData("NS", []cloudwatch.MetricDatum{
		{MetricName: "Ops", Value: 5, Count: 1, Sum: 5, Min: 5, Max: 5, Timestamp: ts, Dimensions: dimsA},
		{MetricName: "Ops", Value: 50, Count: 1, Sum: 50, Min: 50, Max: 50, Timestamp: ts},
	})

	queryBody := cbor.Map{
		"StartTime": cbor.Tag{ID: 1, Value: cbor.Float64(float64(ts.Add(-time.Minute).Unix()))},
		"EndTime":   cbor.Tag{ID: 1, Value: cbor.Float64(float64(ts.Add(time.Minute).Unix()))},
		"MetricDataQueries": cbor.List{
			cbor.Map{
				"Id": cbor.String("m1"),
				"MetricStat": cbor.Map{
					"Metric": cbor.Map{
						"Namespace":  cbor.String("NS"),
						"MetricName": cbor.String("Ops"),
						"Dimensions": cbor.List{
							cbor.Map{"Name": cbor.String("Shard"), "Value": cbor.String("a")},
						},
					},
					"Stat":   cbor.String("Sum"),
					"Period": cbor.Uint(60),
				},
			},
		},
	}
	rec := postCBOR(t, h, "GetMetricData", queryBody)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeCBORResponse(t, rec)
	results, ok := resp["MetricDataResults"].(cbor.List)
	require.True(t, ok)
	require.Len(t, results, 1)
	result := results[0].(cbor.Map)
	vals, ok := result["Values"].(cbor.List)
	require.True(t, ok)
	require.Len(t, vals, 1)
	assert.InDelta(t, 5.0, float64(vals[0].(cbor.Float64)), 1e-9)
}

// TestCBOR_PutMetricStream_WithFilters covers IncludeFilters over rpc-v2-cbor.
// IncludeFilters and ExcludeFilters are mutually exclusive in real CloudWatch
// ("You cannot include ExcludeFilters and IncludeFilters in the same
// operation" -- PutMetricStreamInput doc comment), so only one is set here;
// TestCBOR_PutMetricStream_WithExcludeFilters covers the other case.
func TestCBOR_PutMetricStream_WithFilters(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	h := cloudwatch.NewHandler(b)

	body := cbor.Map{
		"Name":         cbor.String("test-stream"),
		"FirehoseArn":  cbor.String("arn:aws:firehose:us-east-1:123:deliverystream/s"),
		"RoleArn":      cbor.String("arn:aws:iam::123:role/r"),
		"OutputFormat": cbor.String("json"),
		"IncludeFilters": cbor.List{
			cbor.Map{
				"Namespace":   cbor.String("AWS/EC2"),
				"MetricNames": cbor.List{cbor.String("CPUUtilization")},
			},
		},
	}
	rec := postCBOR(t, h, "PutMetricStream", body)
	require.Equal(t, http.StatusOK, rec.Code)

	stream, err := b.GetMetricStream("test-stream")
	require.NoError(t, err)
	require.Len(t, stream.IncludeFilters, 1)
	assert.Equal(t, "AWS/EC2", stream.IncludeFilters[0].Namespace)
}

// TestCBOR_PutMetricStream_WithExcludeFilters covers ExcludeFilters over
// rpc-v2-cbor as a standalone (non-Include) case.
func TestCBOR_PutMetricStream_WithExcludeFilters(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	h := cloudwatch.NewHandler(b)

	body := cbor.Map{
		"Name":         cbor.String("test-stream-excl"),
		"FirehoseArn":  cbor.String("arn:aws:firehose:us-east-1:123:deliverystream/s"),
		"RoleArn":      cbor.String("arn:aws:iam::123:role/r"),
		"OutputFormat": cbor.String("json"),
		"ExcludeFilters": cbor.List{
			cbor.Map{"Namespace": cbor.String("AWS/Lambda")},
		},
	}
	rec := postCBOR(t, h, "PutMetricStream", body)
	require.Equal(t, http.StatusOK, rec.Code)

	stream, err := b.GetMetricStream("test-stream-excl")
	require.NoError(t, err)
	require.Len(t, stream.ExcludeFilters, 1)
	assert.Equal(t, "AWS/Lambda", stream.ExcludeFilters[0].Namespace)
}

func TestCBOR_ListMetrics_WithDimensions(t *testing.T) {
	t.Parallel()

	b := cloudwatch.NewInMemoryBackend()
	h := cloudwatch.NewHandler(b)

	ts := time.Now().UTC()
	for _, env := range []string{"prod", "staging"} {
		_ = b.PutMetricData("App", []cloudwatch.MetricDatum{
			{
				MetricName: "RPM", Value: 1, Count: 1, Sum: 1, Min: 1, Max: 1,
				Timestamp:  ts,
				Dimensions: []cloudwatch.Dimension{{Name: "Env", Value: env}},
			},
		})
	}

	// Filter by Env=prod.
	body := cbor.Map{
		"Namespace":  cbor.String("App"),
		"MetricName": cbor.String("RPM"),
		"Dimensions": cbor.List{
			cbor.Map{"Name": cbor.String("Env"), "Value": cbor.String("prod")},
		},
	}
	rec := postCBOR(t, h, "ListMetrics", body)
	require.Equal(t, http.StatusOK, rec.Code)

	resp := decodeCBORResponse(t, rec)
	metrics, ok := resp["Metrics"].(cbor.List)
	require.True(t, ok)
	require.Len(t, metrics, 1)
}

// ---------------------------------------------------------------------------
// Fix 4: buildCompositeAlarmCBOR missing StateTransitionedTimestamp
// ---------------------------------------------------------------------------

func TestCBOR_DescribeAlarms_CompositeAlarm_StateTransitionedTimestamp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stateValue string
	}{
		{name: "ALARM state", stateValue: "ALARM"},
		{name: "OK state", stateValue: "OK"},
		{name: "INSUFFICIENT_DATA state", stateValue: "INSUFFICIENT_DATA"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newCBORHandler()
			postCBOR(t, h, "PutCompositeAlarm", cbor.Map{
				"AlarmName": cbor.String("comp-cbor"),
				"AlarmRule": cbor.String("FALSE"),
			})
			postCBOR(t, h, "SetAlarmState", cbor.Map{
				"AlarmName":   cbor.String("comp-cbor"),
				"StateValue":  cbor.String(tc.stateValue),
				"StateReason": cbor.String("manual"),
			})

			// AlarmTypes must be explicit: DescribeAlarms defaults to metric
			// alarms only when AlarmTypes is omitted (bd gopherstack-yvb7), so
			// a composite alarm is invisible here without it, even though
			// AlarmNames names it directly.
			rec := postCBOR(t, h, "DescribeAlarms", cbor.Map{
				"AlarmNames": cbor.List{cbor.String("comp-cbor")},
				"AlarmTypes": cbor.List{cbor.String("CompositeAlarm")},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			resp := decodeCBORResponse(t, rec)
			compList, ok := resp["CompositeAlarms"].(cbor.List)
			require.True(t, ok, "CompositeAlarms must be a list")
			require.Len(t, compList, 1)

			alarmMap, ok := compList[0].(cbor.Map)
			require.True(t, ok)

			_, hasTimestamp := alarmMap["StateTransitionedTimestamp"]
			assert.True(t, hasTimestamp,
				"CBOR DescribeAlarms must include StateTransitionedTimestamp for composite alarms")
		})
	}
}

// ---------------------------------------------------------------------------
// Fix 5: cborPutMetricAlarm drops Dimensions
// ---------------------------------------------------------------------------

func TestCBOR_PutMetricAlarm_Dimensions_Stored(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dims cbor.List
		want []cloudwatch.Dimension
	}{
		{
			name: "single dimension stored",
			dims: cbor.List{
				cbor.Map{"Name": cbor.String("Env"), "Value": cbor.String("prod")},
			},
			want: []cloudwatch.Dimension{{Name: "Env", Value: "prod"}},
		},
		{
			name: "multiple dimensions stored",
			dims: cbor.List{
				cbor.Map{"Name": cbor.String("Env"), "Value": cbor.String("prod")},
				cbor.Map{"Name": cbor.String("Region"), "Value": cbor.String("us-east-1")},
			},
			want: []cloudwatch.Dimension{
				{Name: "Env", Value: "prod"},
				{Name: "Region", Value: "us-east-1"},
			},
		},
		{
			name: "no dimensions",
			dims: nil,
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := cloudwatch.NewInMemoryBackend()
			h := cloudwatch.NewHandler(b)

			body := cbor.Map{
				"AlarmName":          cbor.String("dim-alarm"),
				"Namespace":          cbor.String("NS"),
				"MetricName":         cbor.String("M"),
				"ComparisonOperator": cbor.String("GreaterThanThreshold"),
				"Threshold":          cbor.Float64(1.0),
				"EvaluationPeriods":  cbor.Uint(1),
				"Period":             cbor.Uint(60),
			}
			if tc.dims != nil {
				body["Dimensions"] = tc.dims
			}

			rec := postCBOR(t, h, "PutMetricAlarm", body)
			require.Equal(t, http.StatusOK, rec.Code)

			page, _, _, err := b.DescribeAlarms([]string{"dim-alarm"}, nil, "", "", "", 0, "", "", "")
			require.NoError(t, err)
			require.Len(t, page.Data, 1)

			got := page.Data[0].Dimensions
			if len(tc.want) == 0 {
				assert.Empty(t, got, "no dimensions should be stored")
			} else {
				require.Len(t, got, len(tc.want))
				for _, wantDim := range tc.want {
					found := false
					for _, gotDim := range got {
						if gotDim.Name == wantDim.Name && gotDim.Value == wantDim.Value {
							found = true

							break
						}
					}
					assert.True(t, found, "dimension %s=%s must be stored", wantDim.Name, wantDim.Value)
				}
			}
		})
	}
}
