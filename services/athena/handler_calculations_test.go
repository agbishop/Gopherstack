package athena_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/athena"
)

func startCalc(t *testing.T, h *athena.Handler, sessionID string) string {
	t.Helper()

	rec := doRequest(t, h, "StartCalculationExecution",
		`{"SessionId":"`+sessionID+`","CodeBlock":"print(1)","Description":"hi"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	return jsonField(t, rec.Body.Bytes(), "CalculationExecutionId")
}

func TestHandler_StartCalculationExecution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		terminate  bool
		wantStatus int
	}{
		{
			name:       "no_session",
			body:       `{"CodeBlock":"x"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown_session",
			body:       `{"SessionId":"missing","CodeBlock":"x"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "terminated_session",
			terminate:  true,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := tt.body

			if tt.terminate {
				id := startSession(t, h)
				doRequest(t, h, "TerminateSession", `{"SessionId":"`+id+`"}`)
				body = `{"SessionId":"` + id + `","CodeBlock":"x"}`
			}

			rec := doRequest(t, h, "StartCalculationExecution", body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_GetCalculationExecution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		calcID     string
		wantField  string
		wantStatus int
	}{
		{
			name:       "success",
			wantStatus: http.StatusOK,
			wantField:  "SessionId",
		},
		{
			name:       "not_found",
			calcID:     "x",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			calcID := tt.calcID

			if calcID == "" {
				sid := startSession(t, h)
				calcID = startCalc(t, h, sid)
			}

			rec := doRequest(t, h, "GetCalculationExecution", `{"CalculationExecutionId":"`+calcID+`"}`)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantField != "" {
				assert.NotEmpty(t, jsonField(t, rec.Body.Bytes(), tt.wantField))
			}
		})
	}
}

func TestHandler_GetCalculationExecutionStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		calcID     string
		wantStatus int
	}{
		{
			name:       "success",
			wantStatus: http.StatusOK,
		},
		{
			name:       "not_found",
			calcID:     "x",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			calcID := tt.calcID

			if calcID == "" {
				sid := startSession(t, h)
				calcID = startCalc(t, h, sid)
			}

			rec := doRequest(t, h, "GetCalculationExecutionStatus", `{"CalculationExecutionId":"`+calcID+`"}`)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_GetCalculationExecutionCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		calcID     string
		wantCode   string
		wantStatus int
	}{
		{
			name:       "success",
			wantStatus: http.StatusOK,
			wantCode:   "print(1)",
		},
		{
			name:       "not_found",
			calcID:     "x",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			calcID := tt.calcID

			if calcID == "" {
				sid := startSession(t, h)
				calcID = startCalc(t, h, sid)
			}

			rec := doRequest(t, h, "GetCalculationExecutionCode", `{"CalculationExecutionId":"`+calcID+`"}`)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, jsonField(t, rec.Body.Bytes(), "CodeBlock"))
			}
		})
	}
}

// TestHandler_StopCalculationExecution_TerminalIsNoop guards AWS's documented
// StopCalculationExecution behavior: "A StopCalculationExecution call on a
// calculation that is already in a terminal state (for example, STOPPED,
// FAILED, or COMPLETED) succeeds but has no effect."
// (aws-sdk-go-v2/service/athena@v1.60.4 api_op_StopCalculationExecution.go).
// StartCalculationExecution always completes synchronously to COMPLETED, so
// every calculation is already terminal by the time a client can call Stop.
func TestHandler_StopCalculationExecution_TerminalIsNoop(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	sid := startSession(t, h)
	calcID := startCalc(t, h, sid)

	rec := doRequest(t, h, "StopCalculationExecution", `{"CalculationExecutionId":"`+calcID+`"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "COMPLETED", jsonField(t, rec.Body.Bytes(), "State"),
		"stopping a terminal calculation must have no effect on its state")

	rec = doRequest(t, h, "GetCalculationExecutionStatus", `{"CalculationExecutionId":"`+calcID+`"}`)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandler_StopCalculationExecution_NotFound(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, "StopCalculationExecution", `{"CalculationExecutionId":"x"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestBackend_StopCalculation_Cancellable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		state     string
		wantState string
		wantErr   bool
	}{
		{
			name:      "running_can_be_canceled",
			state:     "RUNNING",
			wantState: "CANCELED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := athena.NewInMemoryBackend("", "")
			sid, _, err := backend.StartSession("primary", "", "",
				athena.EngineConfiguration{}, athena.SessionConfiguration{},
				athena.MonitoringConfiguration{}, "")
			require.NoError(t, err)

			cid, _, err := backend.StartCalculationExecution(sid, "", "x")
			require.NoError(t, err)

			backend.SetCalculationState(cid, tt.state)
			got, err := backend.StopCalculationExecution(cid)

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantState, got)
		})
	}
}

func TestHandler_ListCalculationExecutions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		useSession bool
		wantStatus int
	}{
		{
			name:       "success",
			useSession: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "filter",
			useSession: true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid_state_filter",
			useSession: true,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "validation_no_session",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown_session",
			body:       `{"SessionId":"missing"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			body := tt.body

			if tt.useSession {
				sid := startSession(t, h)
				startCalc(t, h, sid)

				switch tt.name {
				case "filter":
					body = `{"SessionId":"` + sid + `","StateFilter":"FAILED"}`
				case "invalid_state_filter":
					body = `{"SessionId":"` + sid + `","StateFilter":"NEVER"}`
				default:
					body = `{"SessionId":"` + sid + `"}`
				}
			}

			rec := doRequest(t, h, "ListCalculationExecutions", body)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// --- Capacity tests ---
