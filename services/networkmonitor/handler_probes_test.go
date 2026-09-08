package networkmonitor_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerProbeLifecycle(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rr := doNMRequest(
		t,
		h,
		http.MethodPost,
		"/monitors",
		map[string]any{"monitorName": "probe-mon"},
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("create monitor: status %d", rr.Code)
	}

	rr = doNMRequest(t, h, http.MethodPost, "/monitors/probe-mon/probes", map[string]any{
		"probe": map[string]any{
			"destination": "10.0.0.2",
			"protocol":    "ICMP",
			"sourceArn":   "arn:aws:ec2:us-east-1:000000000000:subnet/subnet-abc",
		},
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("create probe: status %d — body: %s", rr.Code, rr.Body.String())
	}

	var probeResp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &probeResp); err != nil {
		t.Fatalf("unmarshal probe: %v", err)
	}

	probeID, _ := probeResp["probeId"].(string)
	if probeID == "" {
		t.Fatal("expected non-empty probeId")
	}

	rr = doNMRequest(t, h, http.MethodGet, "/monitors/probe-mon/probes/"+probeID, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get probe: status %d", rr.Code)
	}

	rr = doNMRequest(t, h, http.MethodPatch, "/monitors/probe-mon/probes/"+probeID, map[string]any{
		"destination": "10.0.0.3",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("update probe: status %d — %s", rr.Code, rr.Body.String())
	}

	rr = doNMRequest(t, h, http.MethodDelete, "/monitors/probe-mon/probes/"+probeID, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete probe: status %d", rr.Code)
	}

	rr = doNMRequest(t, h, http.MethodGet, "/monitors/probe-mon/probes/"+probeID, nil)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", rr.Code)
	}
}

// TestHandlerProbeTimestampsEpochFloat verifies that GetProbe returns
// createdAt/modifiedAt as JSON numbers (epoch seconds), not RFC3339 strings.
// Real AWS networkmonitor wire format uses Iso8601Timestamp = JSON Number.
func TestHandlerProbeTimestampsEpochFloat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		protocol    string
		destination string
	}{
		{name: "icmp_probe", protocol: "ICMP", destination: "10.0.0.1"},
		{name: "icmp_probe2", protocol: "ICMP", destination: "10.0.0.2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createMonitorP(t, h, "ts-mon")
			probeID := createProbeP(t, h, "ts-mon", tt.destination, tt.protocol)

			rec := doNMRequest(t, h, http.MethodGet, "/monitors/ts-mon/probes/"+probeID, nil)
			require.Equal(t, http.StatusOK, rec.Code)

			var raw map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

			createdAtRaw, hasCreatedAt := raw["createdAt"]
			assert.True(t, hasCreatedAt, "createdAt must be present in GetProbe response")

			if hasCreatedAt {
				var asFloat float64
				require.NoError(t, json.Unmarshal(createdAtRaw, &asFloat),
					"createdAt must be a JSON number (epoch seconds), got: %s", string(createdAtRaw))
				assert.Greater(t, asFloat, float64(0), "createdAt epoch value must be positive")
			}
		})
	}
}

// TestHandlerProbeAddressFamilyAutoDetect verifies that addressFamily is IPV4
// for IPv4 destinations and IPV6 for IPv6 destinations (colon-containing
// addresses).
func TestHandlerProbeAddressFamilyAutoDetect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		destination string
		wantFamily  string
	}{
		{name: "ipv4", destination: "10.0.0.1", wantFamily: "IPV4"},
		{name: "ipv6", destination: "2001:db8::1", wantFamily: "IPV6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			createMonitorP(t, h, "af-mon")
			createProbeP(t, h, "af-mon", tt.destination, "ICMP")

			rec := doNMRequest(t, h, http.MethodPost, "/monitors/af-mon/probes", map[string]any{
				"probe": map[string]any{
					"destination": tt.destination,
					"protocol":    "ICMP",
					"sourceArn":   "arn:aws:ec2:us-east-1:000000000000:subnet/subnet-xyz",
				},
			})
			require.Equal(t, http.StatusOK, rec.Code)

			var out map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
			assert.Equal(t, tt.wantFamily, out["addressFamily"], "addressFamily for %s", tt.destination)
		})
	}
}

// TestHandlerUpdateProbeStatePending pins PARITY.md's claim that PENDING is
// reachable via UpdateProbe (real AWS accepts a client-settable State field).
func TestHandlerUpdateProbeStatePending(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	createMonitorP(t, h, "pending-mon")
	probeID := createProbeP(t, h, "pending-mon", "10.0.0.1", "ICMP")

	rec := doNMRequest(t, h, http.MethodPatch, "/monitors/pending-mon/probes/"+probeID, map[string]any{
		"state": "PENDING",
	})
	require.Equal(t, http.StatusOK, rec.Code, "update probe: %s", rec.Body.String())

	rec = doNMRequest(t, h, http.MethodGet, "/monitors/pending-mon/probes/"+probeID, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "PENDING", out["state"], "probe state after UpdateProbe with state=PENDING")
}
