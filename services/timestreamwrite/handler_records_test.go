package timestreamwrite_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/timestreamwrite"
)

func TestHandler_WriteRecords(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "mydb"})
	require.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, "CreateTable", map[string]string{"DatabaseName": "mydb", "TableName": "tbl"})
	require.Equal(t, http.StatusOK, rec.Code)

	tests := []struct {
		body       any
		name       string
		wantStatus int
	}{
		{
			name: "success",
			body: map[string]any{
				"DatabaseName": "mydb",
				"TableName":    "tbl",
				"Records": []map[string]any{
					{
						"MeasureName":      "cpu_utilization",
						"MeasureValue":     "13.5",
						"MeasureValueType": "DOUBLE",
						"Time":             recentTimeMillis(0),
						"TimeUnit":         "MILLISECONDS",
					},
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing names",
			body:       map[string]string{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := doRequest(t, h, "WriteRecords", tt.body)
			assert.Equal(t, tt.wantStatus, result.Code)
		})
	}
}

// TestHandler_WriteRecords_RequiresMeasureName verifies that WriteRecords
// rejects records with an empty MeasureName. Real AWS returns
// ValidationException for records missing this required field; the emulator
// previously accepted them silently, masking misconfigured callers.
func TestHandler_WriteRecords_RequiresMeasureName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		commonAttributes map[string]any
		name             string
		records          []map[string]any
		wantCode         int
	}{
		{
			name: "record_without_measure_name_rejected",
			records: []map[string]any{
				{
					"MeasureValue":     "42.0",
					"MeasureValueType": "DOUBLE",
					"Time":             recentTimeMillis(0),
					"TimeUnit":         "MILLISECONDS",
				},
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name: "measure_name_from_common_attributes_accepted",
			commonAttributes: map[string]any{
				"MeasureName":      "cpu_usage",
				"MeasureValueType": "DOUBLE",
			},
			records: []map[string]any{
				{
					"MeasureValue": "85.5",
					"Time":         recentTimeMillis(0),
					"TimeUnit":     "MILLISECONDS",
				},
			},
			wantCode: http.StatusOK,
		},
		{
			name: "record_with_measure_name_accepted",
			records: []map[string]any{
				{
					"MeasureName":      "temperature",
					"MeasureValue":     "36.6",
					"MeasureValueType": "DOUBLE",
					"Time":             recentTimeMillis(0),
					"TimeUnit":         "MILLISECONDS",
				},
			},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)

			dbRec := doRequest(t, h, "CreateDatabase", map[string]any{"DatabaseName": "parity-db-" + tt.name})
			require.Equal(t, http.StatusOK, dbRec.Code)

			tblRec := doRequest(t, h, "CreateTable", map[string]any{
				"DatabaseName": "parity-db-" + tt.name,
				"TableName":    "parity-tbl",
			})
			require.Equal(t, http.StatusOK, tblRec.Code)

			body := map[string]any{
				"DatabaseName": "parity-db-" + tt.name,
				"TableName":    "parity-tbl",
				"Records":      tt.records,
			}

			if tt.commonAttributes != nil {
				body["CommonAttributes"] = tt.commonAttributes
			}

			rec := doRequest(t, h, "WriteRecords", body)
			assert.Equal(t, tt.wantCode, rec.Code, "WriteRecords status for case %q", tt.name)

			if tt.wantCode == http.StatusBadRequest {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))

				assert.NotEmpty(t, errResp["__type"], "error response must include __type")
			}
		})
	}
}

// TestHandler_WriteRecords_DimensionNameRequired verifies that dimension
// Name/Value are required per the AWS API.
func TestHandler_WriteRecords_DimensionNameRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dimension  map[string]any
		name       string
		wantStatus int
	}{
		{
			name:       "empty_dimension_name_rejected",
			dimension:  map[string]any{"name": "", "value": "host-1"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty_dimension_value_rejected",
			dimension:  map[string]any{"name": "host", "value": ""},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid_dimension_accepted",
			dimension:  map[string]any{"name": "host", "value": "server-1"},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateDatabase", map[string]any{"DatabaseName": "dim-val-db"})
			doRequest(t, h, "CreateTable", map[string]any{
				"DatabaseName": "dim-val-db",
				"TableName":    "dim-val-tbl",
			})

			rec := doRequest(t, h, "WriteRecords", map[string]any{
				"DatabaseName": "dim-val-db",
				"TableName":    "dim-val-tbl",
				"Records": []map[string]any{
					{
						"MeasureName":      "cpu",
						"MeasureValue":     "42.0",
						"MeasureValueType": "DOUBLE",
						"Time":             recentTimeMillis(0),
						"TimeUnit":         "MILLISECONDS",
						"Dimensions":       []map[string]any{tt.dimension},
					},
				},
			})
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

// TestHandler_WriteRecords_MULTIConstraints verifies MULTI measure type
// constraints: MeasureValues required, MeasureValue must be empty.
func TestHandler_WriteRecords_MULTIConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		record     map[string]any
		name       string
		wantStatus int
	}{
		{
			name: "MULTI_without_MeasureValues_rejected",
			record: map[string]any{
				"MeasureName":      "multi-m",
				"MeasureValueType": "MULTI",
				"Time":             recentTimeMillis(0),
				"TimeUnit":         "MILLISECONDS",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "MULTI_with_MeasureValue_non_empty_rejected",
			record: map[string]any{
				"MeasureName":      "multi-m",
				"MeasureValue":     "should-be-empty",
				"MeasureValueType": "MULTI",
				"Time":             recentTimeMillis(0),
				"TimeUnit":         "MILLISECONDS",
				"MeasureValues": []map[string]any{
					{"name": "a", "value": "1", "type": "DOUBLE"},
				},
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "MULTI_with_MeasureValues_and_empty_MeasureValue_accepted",
			record: map[string]any{
				"MeasureName":      "multi-m",
				"MeasureValueType": "MULTI",
				"Time":             recentTimeMillis(0),
				"TimeUnit":         "MILLISECONDS",
				"MeasureValues": []map[string]any{
					{"name": "cpu", "value": "42.0", "type": "DOUBLE"},
					{"name": "mem", "value": "1024", "type": "BIGINT"},
				},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateDatabase", map[string]any{"DatabaseName": "multi-db"})
			doRequest(t, h, "CreateTable", map[string]any{
				"DatabaseName": "multi-db",
				"TableName":    "multi-tbl",
			})

			rec := doRequest(t, h, "WriteRecords", map[string]any{
				"DatabaseName": "multi-db",
				"TableName":    "multi-tbl",
				"Records":      []map[string]any{tt.record},
			})
			assert.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
		})
	}
}

// TestHandler_WriteRecords_InvalidMeasureValueType verifies that an unknown
// MeasureValueType is rejected with ValidationException.
func TestHandler_WriteRecords_InvalidMeasureValueType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		measureValueType string
		wantStatus       int
	}{
		{
			name:             "DOUBLE is valid",
			measureValueType: "DOUBLE",
			wantStatus:       http.StatusOK,
		},
		{
			name:             "BIGINT is valid",
			measureValueType: "BIGINT",
			wantStatus:       http.StatusOK,
		},
		{
			name:             "BOOLEAN is valid",
			measureValueType: "BOOLEAN",
			wantStatus:       http.StatusOK,
		},
		{
			name:             "VARCHAR is valid",
			measureValueType: "VARCHAR",
			wantStatus:       http.StatusOK,
		},
		{
			name:             "TIMESTAMP is valid",
			measureValueType: "TIMESTAMP",
			wantStatus:       http.StatusOK,
		},
		{
			name:             "unknown type rejected",
			measureValueType: "INVALID",
			wantStatus:       http.StatusBadRequest,
		},
		{
			name:             "lowercase rejected",
			measureValueType: "double",
			wantStatus:       http.StatusBadRequest,
		},
		{
			name:             "mixed case rejected",
			measureValueType: "Double",
			wantStatus:       http.StatusBadRequest,
		},
		{
			name:             "empty string passes (field optional)",
			measureValueType: "",
			wantStatus:       http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "mvt-db"})
			doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "mvt-db", "TableName": "mvt-tbl"})

			record := map[string]any{
				"MeasureName":  "cpu",
				"MeasureValue": "1.5",
				"Time":         recentTimeMillis(0),
				"TimeUnit":     "MILLISECONDS",
			}
			if tt.measureValueType != "" {
				record["MeasureValueType"] = tt.measureValueType
			}

			rec := doRequest(t, h, "WriteRecords", map[string]any{
				"DatabaseName": "mvt-db",
				"TableName":    "mvt-tbl",
				"Records":      []any{record},
			})
			assert.Equal(t, tt.wantStatus, rec.Code, "MeasureValueType=%q", tt.measureValueType)

			if tt.wantStatus == http.StatusBadRequest {
				var body map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, "ValidationException", body["__type"])
			}
		})
	}
}

// TestHandler_WriteRecords_InvalidTimeUnit verifies that an unrecognised
// TimeUnit is rejected with ValidationException.
func TestHandler_WriteRecords_InvalidTimeUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		timeUnit   string
		wantStatus int
	}{
		{
			name:       "SECONDS is valid",
			timeUnit:   "SECONDS",
			wantStatus: http.StatusOK,
		},
		{
			name:       "MILLISECONDS is valid",
			timeUnit:   "MILLISECONDS",
			wantStatus: http.StatusOK,
		},
		{
			name:       "MICROSECONDS is valid",
			timeUnit:   "MICROSECONDS",
			wantStatus: http.StatusOK,
		},
		{
			name:       "NANOSECONDS is valid",
			timeUnit:   "NANOSECONDS",
			wantStatus: http.StatusOK,
		},
		{
			name:       "unknown unit rejected",
			timeUnit:   "HOURS",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "lowercase rejected",
			timeUnit:   "milliseconds",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty string passes (field optional)",
			timeUnit:   "",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "tu-db"})
			doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "tu-db", "TableName": "tu-tbl"})

			record := map[string]any{
				"MeasureName":      "metric",
				"MeasureValue":     "42",
				"MeasureValueType": "BIGINT",
				"Time":             recentTimeForUnit(tt.timeUnit),
			}
			if tt.timeUnit != "" {
				record["TimeUnit"] = tt.timeUnit
			}

			rec := doRequest(t, h, "WriteRecords", map[string]any{
				"DatabaseName": "tu-db",
				"TableName":    "tu-tbl",
				"Records":      []any{record},
			})
			assert.Equal(t, tt.wantStatus, rec.Code, "TimeUnit=%q", tt.timeUnit)

			if tt.wantStatus == http.StatusBadRequest {
				var body map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, "ValidationException", body["__type"])
			}
		})
	}
}

// TestHandler_WriteRecords_DimensionCountLimit verifies that the handler
// enforces the AWS limit of 128 dimensions per record.
func TestHandler_WriteRecords_DimensionCountLimit(t *testing.T) {
	t.Parallel()

	makeDimensions := func(n int) []map[string]string {
		dims := make([]map[string]string, n)
		for i := range dims {
			dims[i] = map[string]string{
				"Name":  "dim" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
				"Value": "v",
			}
		}

		return dims
	}

	tests := []struct {
		name       string
		dimCount   int
		wantStatus int
	}{
		{
			name:       "127 dimensions (below limit)",
			dimCount:   127,
			wantStatus: http.StatusOK,
		},
		{
			name:       "128 dimensions (at limit)",
			dimCount:   128,
			wantStatus: http.StatusOK,
		},
		{
			name:       "129 dimensions (exceeds limit)",
			dimCount:   129,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "200 dimensions (well over limit)",
			dimCount:   200,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "dim-db"})
			doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "dim-db", "TableName": "dim-tbl"})

			rec := doRequest(t, h, "WriteRecords", map[string]any{
				"DatabaseName": "dim-db",
				"TableName":    "dim-tbl",
				"Records": []any{
					map[string]any{
						"MeasureName":      "m",
						"MeasureValue":     "1",
						"MeasureValueType": "BIGINT",
						"Time":             recentTimeMillis(0),
						"TimeUnit":         "MILLISECONDS",
						"Dimensions":       makeDimensions(tt.dimCount),
					},
				},
			})
			assert.Equal(t, tt.wantStatus, rec.Code, "dimCount=%d", tt.dimCount)

			if tt.wantStatus == http.StatusBadRequest {
				var body map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, "ValidationException", body["__type"])
			}
		})
	}
}

// TestHandler_WriteRecords_InvalidMeasureValueTypeViaCommonAttributes
// verifies that an invalid MeasureValueType in CommonAttributes is caught
// when it propagates to a record that does not override it.
func TestHandler_WriteRecords_InvalidMeasureValueTypeViaCommonAttributes(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "ca-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "ca-db", "TableName": "ca-tbl"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "ca-db",
		"TableName":    "ca-tbl",
		"CommonAttributes": map[string]any{
			"MeasureValueType": "NOT_A_TYPE",
			"TimeUnit":         "MILLISECONDS",
		},
		"Records": []any{
			map[string]any{
				"MeasureName":  "cpu",
				"MeasureValue": "1.0",
				"Time":         recentTimeMillis(0),
			},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ValidationException", body["__type"])
}

// TestHandler_WriteRecords_InvalidTimeUnitViaCommonAttributes verifies that
// an invalid TimeUnit in CommonAttributes is caught when it propagates to a
// record.
func TestHandler_WriteRecords_InvalidTimeUnitViaCommonAttributes(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "catu-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "catu-db", "TableName": "catu-tbl"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "catu-db",
		"TableName":    "catu-tbl",
		"CommonAttributes": map[string]any{
			"MeasureValueType": "DOUBLE",
			"TimeUnit":         "DAYS",
		},
		"Records": []any{
			map[string]any{
				"MeasureName":  "load",
				"MeasureValue": "0.5",
				"Time":         recentTimeMillis(0),
			},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ValidationException", body["__type"])
}

// TestHandler_WriteRecords_RecordOverridesCommonInvalidType verifies that a
// record with a valid MeasureValueType overrides an invalid one in
// CommonAttributes, and the request succeeds.
func TestHandler_WriteRecords_RecordOverridesCommonInvalidType(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "ovr-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "ovr-db", "TableName": "ovr-tbl"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "ovr-db",
		"TableName":    "ovr-tbl",
		"CommonAttributes": map[string]any{
			"TimeUnit": "MILLISECONDS",
		},
		"Records": []any{
			map[string]any{
				"MeasureName":      "cpu",
				"MeasureValue":     "1.0",
				"MeasureValueType": "DOUBLE",
				"Time":             recentTimeMillis(0),
			},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestHandler_WriteRecords_MultiMeasureValueTypeAccepted verifies that MULTI
// is a valid MeasureValueType and is accepted by the handler.
func TestHandler_WriteRecords_MultiMeasureValueTypeAccepted(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "multi-db2"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "multi-db2", "TableName": "multi-tbl2"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "multi-db2",
		"TableName":    "multi-tbl2",
		"Records": []any{
			map[string]any{
				"MeasureName":      "metrics",
				"MeasureValueType": "MULTI",
				"Time":             recentTimeMillis(0),
				"TimeUnit":         "MILLISECONDS",
				"MeasureValues": []map[string]string{
					{"Name": "cpu", "Value": "0.5", "Type": "DOUBLE"},
				},
			},
		},
	})
	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestHandler_WriteRecords_MultiMeasure verifies that MULTI-type records with
// MeasureValues are accepted and stored correctly.
func TestHandler_WriteRecords_MultiMeasure(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "multi-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "multi-db", "TableName": "multi-tbl"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "multi-db",
		"TableName":    "multi-tbl",
		"Records": []map[string]any{
			{
				"MeasureName":      "metrics",
				"MeasureValueType": "MULTI",
				"Time":             recentTimeMillis(0),
				"TimeUnit":         "MILLISECONDS",
				"MeasureValues": []map[string]any{
					{"Name": "cpu", "Value": "45.3", "Type": "DOUBLE"},
					{"Name": "mem", "Value": "8192", "Type": "BIGINT"},
					{"Name": "disk_ok", "Value": "true", "Type": "BOOLEAN"},
				},
			},
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	ingested := out["RecordsIngested"].(map[string]any)
	assert.Equal(t, 1, int(ingested["Total"].(float64)))
}

// TestHandler_WriteRecords_CommonAttributes verifies that CommonAttributes
// dimensions are merged into each record per the AWS specification.
func TestHandler_WriteRecords_CommonAttributes(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "ca-db2"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "ca-db2", "TableName": "ca-tbl2"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "ca-db2",
		"TableName":    "ca-tbl2",
		"CommonAttributes": map[string]any{
			"Dimensions": []map[string]any{
				{"Name": "region", "Value": "us-east-1"},
				{"Name": "env", "Value": "prod"},
			},
			"Time":     recentTimeMillis(0),
			"TimeUnit": "MILLISECONDS",
		},
		"Records": []map[string]any{
			{
				"MeasureName":      "cpu",
				"MeasureValue":     "65.4",
				"MeasureValueType": "DOUBLE",
				// No Dimensions or Time — should come from CommonAttributes
			},
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	ingested := out["RecordsIngested"].(map[string]any)
	assert.Equal(t, 1, int(ingested["Total"].(float64)))
}

// TestHandler_WriteRecords_CommonAttributesMerge verifies a record's own
// dimensions are unioned with CommonAttributes' dimensions when the two sets
// don't share a name.
func TestHandler_WriteRecords_CommonAttributesMerge(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "cam-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "cam-db", "TableName": "cam-tbl"})

	// Two unique records: first with no extra dimension, second with a
	// record-specific dimension that doesn't overlap CommonAttributes'.
	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "cam-db",
		"TableName":    "cam-tbl",
		"CommonAttributes": map[string]any{
			"Time":     recentTimeMillis(0),
			"TimeUnit": "MILLISECONDS",
			"Dimensions": []map[string]any{
				{"Name": "env", "Value": "prod"},
			},
		},
		"Records": []map[string]any{
			{
				"MeasureName":      "r1",
				"MeasureValue":     "1",
				"MeasureValueType": "DOUBLE",
			},
			{
				"MeasureName":      "r2",
				"MeasureValue":     "2",
				"MeasureValueType": "DOUBLE",
				"Dimensions": []map[string]any{
					{"Name": "zone", "Value": "us-east-1a"}, // unions with common's "env"
				},
			},
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	ingested := out["RecordsIngested"].(map[string]any)
	assert.Equal(t, 2, int(ingested["Total"].(float64)))
}

// TestHandler_WriteRecords_CommonAttributesDimensionOverlapRejected verifies
// that a record whose Dimensions share a name with CommonAttributes'
// Dimensions is rejected with ValidationException, per
// WriteRecordsInput.CommonAttributes' doc comment (api_op_WriteRecords.go,
// timestreamwrite@v1.38.4): "Dimensions may not overlap, or a
// ValidationException will be thrown".
func TestHandler_WriteRecords_CommonAttributesDimensionOverlapRejected(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "cao-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "cao-db", "TableName": "cao-tbl"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "cao-db",
		"TableName":    "cao-tbl",
		"CommonAttributes": map[string]any{
			"Time":     recentTimeMillis(0),
			"TimeUnit": "MILLISECONDS",
			"Dimensions": []map[string]any{
				{"Name": "env", "Value": "prod"},
			},
		},
		"Records": []map[string]any{
			{
				"MeasureName":      "r1",
				"MeasureValue":     "1",
				"MeasureValueType": "DOUBLE",
				"Dimensions": []map[string]any{
					{"Name": "env", "Value": "staging"}, // overlaps common's "env"
				},
			},
		},
	})

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var errBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	assert.Equal(t, "ValidationException", errBody["__type"])
}

// TestHandler_WriteRecords_DimensionValueType verifies DimensionValueType is
// accepted via the HTTP handler.
func TestHandler_WriteRecords_DimensionValueType(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "dvth-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "dvth-db", "TableName": "dvth-tbl"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "dvth-db",
		"TableName":    "dvth-tbl",
		"Records": []map[string]any{
			{
				"MeasureName":      "latency",
				"MeasureValue":     "23",
				"MeasureValueType": "BIGINT",
				"Time":             recentTimeMillis(0),
				"TimeUnit":         "MILLISECONDS",
				"Dimensions": []map[string]any{
					{"Name": "endpoint", "Value": "/api/health", "DimensionValueType": "VARCHAR"},
				},
			},
		},
	})

	assert.Equal(t, http.StatusOK, rec.Code)
}

// TestHandler_WriteRecords_RejectedRecordsException verifies that the handler
// returns 400 with __type=RejectedRecordsException on version conflict.
func TestHandler_WriteRecords_RejectedRecordsException(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "rr-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "rr-db", "TableName": "rr-tbl"})

	writeBody := map[string]any{
		"DatabaseName": "rr-db",
		"TableName":    "rr-tbl",
		"Records": []map[string]any{
			{
				"MeasureName":      "errors",
				"MeasureValue":     "5",
				"MeasureValueType": "BIGINT",
				"Time":             recentTimeMillis(0),
				"TimeUnit":         "MILLISECONDS",
				"Version":          int64(5),
			},
		},
	}

	// First write succeeds.
	rec1 := doRequest(t, h, "WriteRecords", writeBody)
	require.Equal(t, http.StatusOK, rec1.Code)

	// Same record, lower version — should fail with RejectedRecordsException.
	writeBody["Records"] = []map[string]any{
		{
			"MeasureName":      "errors",
			"MeasureValue":     "5",
			"MeasureValueType": "BIGINT",
			"Time":             recentTimeMillis(0),
			"TimeUnit":         "MILLISECONDS",
			"Version":          int64(3),
		},
	}

	rec2 := doRequest(t, h, "WriteRecords", writeBody)
	assert.Equal(t, http.StatusBadRequest, rec2.Code)

	var errBody map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &errBody))
	assert.Equal(t, "RejectedRecordsException", errBody["__type"])

	rejected, ok := errBody["RejectedRecords"].([]any)
	require.True(t, ok, "RejectedRecords should be present in error response")
	assert.Len(t, rejected, 1)
}

// TestHandler_WriteRecords_MaxRecordsLimit verifies that the handler enforces
// the AWS limit of 100 records per WriteRecords request.
func TestHandler_WriteRecords_MaxRecordsLimit(t *testing.T) {
	t.Parallel()

	makeRecords := func(n int) []map[string]any {
		records := make([]map[string]any, n)
		for i := range records {
			records[i] = map[string]any{
				"MeasureName":      fmt.Sprintf("metric-%d", i),
				"MeasureValue":     "1.0",
				"MeasureValueType": "DOUBLE",
				"Time":             recentTimeMillis(int64(i) * 1000),
				"TimeUnit":         "MILLISECONDS",
			}
		}

		return records
	}

	tests := []struct {
		name       string
		count      int
		wantStatus int
	}{
		{
			name:       "exactly 100 records (at limit)",
			count:      100,
			wantStatus: http.StatusOK,
		},
		{
			name:       "99 records (below limit)",
			count:      99,
			wantStatus: http.StatusOK,
		},
		{
			name:       "101 records (exceeds limit)",
			count:      101,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "200 records (well over limit)",
			count:      200,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Each sub-test gets its own handler to avoid record conflicts.
			h := newTestHandler(t)
			doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "limit-db"})
			doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "limit-db", "TableName": "limit-tbl"})

			rec := doRequest(t, h, "WriteRecords", map[string]any{
				"DatabaseName": "limit-db",
				"TableName":    "limit-tbl",
				"Records":      makeRecords(tt.count),
			})
			assert.Equal(t, tt.wantStatus, rec.Code, "count=%d", tt.count)

			if tt.wantStatus == http.StatusBadRequest {
				var body map[string]string
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, "ValidationException", body["__type"])
			}
		})
	}
}

// TestHandler_WriteRecords_EmptyBatch verifies that writing zero records is
// accepted and returns a zero ingestion count.
func TestHandler_WriteRecords_EmptyBatch(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "empty-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "empty-db", "TableName": "empty-tbl"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "empty-db",
		"TableName":    "empty-tbl",
		"Records":      []map[string]any{},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	ingested := out["RecordsIngested"].(map[string]any)
	assert.Equal(t, 0, int(ingested["Total"].(float64)))
	assert.Equal(t, 0, int(ingested["MemoryStore"].(float64)))
}

// TestHandler_WriteRecords_MagneticStoreInResponse verifies that the
// MagneticStore count is propagated through the HTTP layer.
func TestHandler_WriteRecords_MagneticStoreInResponse(t *testing.T) {
	t.Parallel()

	h := timestreamwrite.NewHandler(timestreamwrite.NewInMemoryBackend())
	b := h.Backend

	_, err := b.CreateDatabase("mag-http-db", "", nil)
	require.NoError(t, err)

	_, err = b.CreateTable("mag-http-db", "mag-http-tbl", nil, &timestreamwrite.CreateTableInput{
		RetentionProperties: &timestreamwrite.RetentionProperties{
			MemoryStoreRetentionPeriodInHours:  1,
			MagneticStoreRetentionPeriodInDays: 365,
		},
		MagneticStoreWriteProperties: &timestreamwrite.MagneticStoreWriteProperties{
			EnableMagneticStoreWrites: true,
		},
	})
	require.NoError(t, err)

	oldTS := strconv.FormatInt(time.Now().UTC().Add(-5*time.Hour).UnixMilli(), 10)

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "mag-http-db",
		"TableName":    "mag-http-tbl",
		"Records": []map[string]any{
			{
				"MeasureName":      "latency",
				"MeasureValue":     "42",
				"MeasureValueType": "BIGINT",
				"Time":             oldTS,
				"TimeUnit":         "MILLISECONDS",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	ingested := out["RecordsIngested"].(map[string]any)
	assert.Equal(t, 1, int(ingested["Total"].(float64)))
	assert.Equal(t, 0, int(ingested["MemoryStore"].(float64)))
	assert.Equal(t, 1, int(ingested["MagneticStore"].(float64)),
		"old record should appear in MagneticStore count")
}

// TestHandler_WriteRecords_ReturnsIngestedTotal verifies records-ingested count.
func TestHandler_WriteRecords_ReturnsIngestedTotal(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]any{"DatabaseName": "wr-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "wr-db", "TableName": "wr-tbl"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "wr-db",
		"TableName":    "wr-tbl",
		"Records": []map[string]any{
			{
				"MeasureName": "cpu", "MeasureValue": "45", "MeasureValueType": "DOUBLE",
				"Time": recentTimeMillis(0), "TimeUnit": "MILLISECONDS",
			},
			{
				"MeasureName": "mem", "MeasureValue": "80", "MeasureValueType": "DOUBLE",
				"Time": recentTimeMillis(0), "TimeUnit": "MILLISECONDS",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	ingested := out["RecordsIngested"].(map[string]any)
	assert.Equal(t, 2, int(ingested["Total"].(float64)))
}

// TestHandler_WriteRecords_ReturnsMemoryStoreCount verifies that WriteRecords
// returns both Total and MemoryStore ingested counts.
func TestHandler_WriteRecords_ReturnsMemoryStoreCount(t *testing.T) {
	t.Parallel()

	h := timestreamwrite.NewHandler(timestreamwrite.NewInMemoryBackend())
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "wrc-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "wrc-db", "TableName": "wrc-tbl"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "wrc-db",
		"TableName":    "wrc-tbl",
		"Records": []map[string]any{
			{"MeasureName": "m1", "MeasureValue": "1", "Time": recentTimeMillis(0), "TimeUnit": "MILLISECONDS"},
			{"MeasureName": "m2", "MeasureValue": "2", "Time": recentTimeMillis(1), "TimeUnit": "MILLISECONDS"},
			{"MeasureName": "m3", "MeasureValue": "3", "Time": recentTimeMillis(2), "TimeUnit": "MILLISECONDS"},
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	ingested := out["RecordsIngested"].(map[string]any)
	assert.Equal(t, int(3), int(ingested["Total"].(float64)))
	assert.Equal(t, int(3), int(ingested["MemoryStore"].(float64)))
}

// TestHandler_WriteRecords_InvalidTable verifies that writing to a
// non-existent table returns an error.
func TestHandler_WriteRecords_InvalidTable(t *testing.T) {
	t.Parallel()

	h := timestreamwrite.NewHandler(timestreamwrite.NewInMemoryBackend())
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "wri-db"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "wri-db",
		"TableName":    "nonexistent",
		"Records":      []map[string]any{{"MeasureName": "m", "MeasureValue": "1"}},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_WriteRecords_InvalidDatabase verifies that writing to a
// non-existent database returns an error.
func TestHandler_WriteRecords_InvalidDatabase(t *testing.T) {
	t.Parallel()

	h := timestreamwrite.NewHandler(timestreamwrite.NewInMemoryBackend())
	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "no-db",
		"TableName":    "no-tbl",
		"Records":      []map[string]any{{"MeasureName": "m", "MeasureValue": "1"}},
	})

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestHandler_WriteRecords_ResponseIsParsableJSON verifies the RecordsIngested
// JSON can be parsed and contains the expected field names.
func TestHandler_WriteRecords_ResponseIsParsableJSON(t *testing.T) {
	t.Parallel()

	h := timestreamwrite.NewHandler(timestreamwrite.NewInMemoryBackend())
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "pj-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "pj-db", "TableName": "pj-tbl"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "pj-db",
		"TableName":    "pj-tbl",
		"Records": []map[string]any{
			{"MeasureName": "cpu", "MeasureValue": "99.9"},
		},
	})

	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, `"Total"`)
	assert.Contains(t, body, `"MemoryStore"`)
	// Should NOT contain old field name.
	assert.NotContains(t, body, `"MemStore"`)
}

// TestHandler_WriteRecords_ResponseFieldNames verifies that the WriteRecords
// HTTP response body contains the expected RecordsIngested structure with all
// three count fields.
func TestHandler_WriteRecords_ResponseFieldNames(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	doRequest(t, h, "CreateDatabase", map[string]string{"DatabaseName": "fields-db"})
	doRequest(t, h, "CreateTable", map[string]any{"DatabaseName": "fields-db", "TableName": "fields-tbl"})

	rec := doRequest(t, h, "WriteRecords", map[string]any{
		"DatabaseName": "fields-db",
		"TableName":    "fields-tbl",
		"Records": []map[string]any{
			{
				"MeasureName":      "cpu",
				"MeasureValue":     "45.0",
				"MeasureValueType": "DOUBLE",
				"Time":             recentTimeMillis(0),
				"TimeUnit":         "MILLISECONDS",
			},
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, `"Total"`, "response should contain Total field")
	assert.Contains(t, body, `"MemoryStore"`, "response should contain MemoryStore field")
	assert.Contains(t, body, `"MagneticStore"`, "response should contain MagneticStore field")
	assert.Contains(t, body, `"RecordsIngested"`, "response should contain RecordsIngested wrapper")
}
