package timestreamwrite_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/timestreamwrite"
)

// writeTestBaseMillis returns a millisecond epoch a few minutes in the past.
// WriteRecords fixtures offset from it so records land inside a table's
// default 6-hour memory-store retention window (records.go
// recordOutsideRetention) regardless of how long the test run takes.
// writeTestBase is fixed for the whole test binary. Recomputing it per call let
// two writes of the "same" record straddle a millisecond boundary and stop
// being duplicates, which made the version-conflict tests flaky.
var writeTestBase = sync.OnceValue(func() int64 { //nolint:gochecknoglobals // one fixed clock for the whole test binary
	return time.Now().Add(-5 * time.Minute).UnixMilli()
})

func writeTestBaseMillis() int64 { return writeTestBase() }

// recentTimeMillis returns a "MILLISECONDS" Time fixture offset from a recent
// base -- inside the default memory-store retention window.
func recentTimeMillis(offset int64) string {
	return strconv.FormatInt(writeTestBaseMillis()+offset, 10)
}

// recentTimeSeconds returns a "SECONDS" Time fixture inside the default
// memory-store retention window.
func recentTimeSeconds() string {
	return strconv.FormatInt(writeTestBaseMillis()/1000, 10)
}

// recentTimeForUnit returns a Time fixture scaled to the given TimeUnit (as
// records.go's parseTimestreamTime interprets it), staying inside the default
// memory-store retention window regardless of unit.
func recentTimeForUnit(unit string) string {
	t := time.UnixMilli(writeTestBaseMillis())

	switch unit {
	case "SECONDS":
		return strconv.FormatInt(t.Unix(), 10)
	case "MICROSECONDS":
		return strconv.FormatInt(t.UnixMicro(), 10)
	case "NANOSECONDS":
		return strconv.FormatInt(t.UnixNano(), 10)
	default:
		return strconv.FormatInt(t.UnixMilli(), 10)
	}
}

func newTestHandler(t *testing.T) *timestreamwrite.Handler {
	t.Helper()

	return timestreamwrite.NewHandler(timestreamwrite.NewInMemoryBackend())
}

func doRequest(
	t *testing.T,
	h *timestreamwrite.Handler,
	target string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyBytes []byte

	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		require.NoError(t, err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Timestream_20181101."+target)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetRequest(req)

	err := h.Handler()(c)
	require.NoError(t, err)

	return rec
}

func TestHandler_Name(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Equal(t, "TimestreamWrite", h.Name())
}

func TestHandler_RouteMatcher(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	e := echo.New()

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{name: "matching target", target: "Timestream_20181101.CreateDatabase", want: true},
		{name: "non-matching target", target: "SageMaker.ListModels", want: false},
		{name: "empty target", target: "", want: false},
		{
			name:   "timestream query operation not matched",
			target: "Timestream_20181101.ListScheduledQueries",
			want:   false,
		},
		{name: "timestream write operation matched", target: "Timestream_20181101.WriteRecords", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			assert.Equal(t, tt.want, h.RouteMatcher()(c))
		})
	}
}

func TestHandler_UnknownAction(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "UnknownOperation", map[string]string{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_GetSupportedOperations(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	ops := h.GetSupportedOperations()

	assert.Contains(t, ops, "CreateDatabase")
	assert.Contains(t, ops, "DescribeDatabase")
	assert.Contains(t, ops, "WriteRecords")
	assert.Contains(t, ops, "DescribeEndpoints")
}

// TestHandler_SupportedOperationsCount verifies the dispatch table's operation
// count matches the real Timestream Write API surface this handler covers.
func TestHandler_SupportedOperationsCount(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	h := timestreamwrite.NewHandler(b)
	assert.Len(t, h.GetSupportedOperations(), 19)
}

// TestHandler_SupportedOperationsSorted verifies GetSupportedOperations is sorted.
func TestHandler_SupportedOperationsSorted(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	h := timestreamwrite.NewHandler(b)
	ops := h.GetSupportedOperations()

	require.NotEmpty(t, ops)

	for i := 1; i < len(ops); i++ {
		assert.LessOrEqual(t, ops[i-1], ops[i],
			"ops not sorted at index %d: %s > %s", i, ops[i-1], ops[i])
	}
}

// TestHandler_OpsLenExport verifies HandlerOpsLen matches GetSupportedOperations.
func TestHandler_OpsLenExport(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	h := timestreamwrite.NewHandler(b)
	assert.Equal(t, 19, timestreamwrite.HandlerOpsLen(h))
}

// TestHandler_Reset verifies that Reset clears all backend state.
func TestHandler_Reset(t *testing.T) {
	t.Parallel()

	b := timestreamwrite.NewInMemoryBackend()
	h := timestreamwrite.NewHandler(b)

	doRequest(t, h, "CreateDatabase", map[string]any{"DatabaseName": "reset-db"})
	assert.Equal(t, 1, timestreamwrite.DatabaseCount(b))

	h.Reset()
	assert.Equal(t, 0, timestreamwrite.DatabaseCount(b))
}

func TestHandler_ChaosInfo(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	assert.Equal(t, "timestreamwrite", h.ChaosServiceName())
	assert.Equal(t, h.GetSupportedOperations(), h.ChaosOperations())
	assert.NotEmpty(t, h.ChaosRegions())
}

func TestHandler_MatchPriority(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	assert.Positive(t, h.MatchPriority())
}

func TestHandler_ExtractResource(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	assert.Empty(t, h.ExtractResource(c))
}

func TestHandler_ExtractOperation(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	e := echo.New()

	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "valid target", target: "Timestream_20181101.CreateDatabase", want: "CreateDatabase"},
		{name: "empty target", target: "", want: "Unknown"},
		{name: "wrong prefix", target: "Something.Action", want: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Amz-Target", tt.target)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			assert.Equal(t, tt.want, h.ExtractOperation(c))
		})
	}
}
