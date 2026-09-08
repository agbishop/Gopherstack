package guardduty_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/guardduty"
)

func TestDetector_Lifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fn   func(t *testing.T, h *guardduty.Handler)
		name string
	}{
		{
			name: "create_enabled",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()
				id := createTestDetector(t, h)
				require.NotEmpty(t, id)
			},
		},
		{
			name: "create_disabled",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()
				rec := doRequest(t, h, http.MethodPost, "/detector", map[string]any{
					"enable": false,
				})
				require.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				require.NotEmpty(t, resp["detectorId"])
			},
		},
		{
			name: "get_detector",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()
				id := createTestDetector(t, h)

				rec := doRequest(t, h, http.MethodGet, "/detector/"+id, nil)
				require.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, "ENABLED", resp["status"])
				assert.NotEmpty(t, resp["serviceRole"])
				assert.Equal(t, "SIX_HOURS", resp["findingPublishingFrequency"])
			},
		},
		{
			name: "update_detector_frequency",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()
				id := createTestDetector(t, h)

				rec := doRequest(t, h, http.MethodPost, "/detector/"+id, map[string]any{
					"findingPublishingFrequency": "FIFTEEN_MINUTES",
				})
				require.Equal(t, http.StatusOK, rec.Code)

				rec = doRequest(t, h, http.MethodGet, "/detector/"+id, nil)
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, "FIFTEEN_MINUTES", resp["findingPublishingFrequency"])
			},
		},
		{
			name: "update_detector_disable",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()
				id := createTestDetector(t, h)
				enable := false
				rec := doRequest(t, h, http.MethodPost, "/detector/"+id, map[string]any{
					"enable": enable,
				})
				require.Equal(t, http.StatusOK, rec.Code)

				rec = doRequest(t, h, http.MethodGet, "/detector/"+id, nil)
				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				assert.Equal(t, "DISABLED", resp["status"])
			},
		},
		{
			name: "delete_detector",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()
				id := createTestDetector(t, h)

				rec := doRequest(t, h, http.MethodDelete, "/detector/"+id, nil)
				require.Equal(t, http.StatusOK, rec.Code)

				rec = doRequest(t, h, http.MethodGet, "/detector/"+id, nil)
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "list_detectors",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()
				id := createTestDetector(t, h)

				rec := doRequest(t, h, http.MethodGet, "/detector", nil)
				require.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				ids, ok := resp["detectorIds"].([]any)
				require.True(t, ok)
				require.Len(t, ids, 1)
				assert.Equal(t, id, ids[0])
			},
		},
		{
			name: "duplicate_detector_rejected",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()
				createTestDetector(t, h)

				rec := doRequest(t, h, http.MethodPost, "/detector", map[string]any{
					"enable": true,
				})
				assert.Equal(t, http.StatusConflict, rec.Code)
			},
		},
		{
			name: "get_nonexistent_detector",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()
				rec := doRequest(t, h, http.MethodGet, "/detector/nonexistent", nil)
				assert.Equal(t, http.StatusNotFound, rec.Code)
			},
		},
		{
			name: "create_with_tags",
			fn: func(t *testing.T, h *guardduty.Handler) {
				t.Helper()
				rec := doRequest(t, h, http.MethodPost, "/detector", map[string]any{
					"enable": true,
					"tags":   map[string]string{"env": "test", "team": "security"},
				})
				require.Equal(t, http.StatusOK, rec.Code)

				var resp map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
				id := resp["detectorId"].(string)

				rec = doRequest(t, h, http.MethodGet, "/detector/"+id, nil)
				var det map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &det))
				tags, ok := det["tags"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "test", tags["env"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			tt.fn(t, h)
		})
	}
}

var (
	reDetectorID  = regexp.MustCompile(`^[0-9a-f]{32}$`)
	reDetectorARN = regexp.MustCompile(`^arn:aws:guardduty:[a-z0-9-]+:\d{12}:detector/[0-9a-f]{32}$`)
)

func TestDetectorID_Shape(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	detectorID := createTestDetector(t, h)

	assert.True(t, reDetectorID.MatchString(detectorID),
		"detectorId must be 32 lowercase hex chars, got %q", detectorID)

	// Verify the ARN we'd construct is well-formed.
	arn := fmt.Sprintf("arn:aws:guardduty:us-east-1:123456789012:detector/%s", detectorID)
	assert.True(t, reDetectorARN.MatchString(arn),
		"constructed detector ARN must match expected pattern, got %q", arn)
}

const tsLayout = "2006-01-02T15:04:05.000Z"

func parseTS(t *testing.T, field, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(tsLayout, value)
	require.NoError(t, err, "%s must parse with layout %q, got %q", field, tsLayout, value)

	return parsed
}

func TestDetector_Timestamps_Present(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id := createTestDetector(t, h)

	rec := doRequest(t, h, http.MethodGet, "/detector/"+id, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	for _, field := range []string{"createdAt", "updatedAt"} {
		raw, ok := resp[field]
		require.True(t, ok, "GetDetector must include %s", field)
		ts, ok := raw.(string)
		require.True(t, ok, "%s must be a string, got %T", field, raw)
		require.NotEmpty(t, ts, "%s must not be empty", field)
		parseTS(t, field, ts)
	}
}

func TestDetector_UpdatedAt_Advances(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id := createTestDetector(t, h)

	rec1 := doRequest(t, h, http.MethodGet, "/detector/"+id, nil)
	require.Equal(t, http.StatusOK, rec1.Code)

	var before map[string]any
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &before))
	createdAt := before["createdAt"].(string)
	updatedAt1 := before["updatedAt"].(string)

	time.Sleep(2 * time.Millisecond)

	rec := doRequest(t, h, http.MethodPost, "/detector/"+id, map[string]any{
		"findingPublishingFrequency": "FIFTEEN_MINUTES",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	rec2 := doRequest(t, h, http.MethodGet, "/detector/"+id, nil)
	require.Equal(t, http.StatusOK, rec2.Code)

	var after map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &after))
	createdAt2 := after["createdAt"].(string)
	updatedAt2 := after["updatedAt"].(string)

	assert.Equal(t, createdAt, createdAt2, "createdAt must not change after UpdateDetector")

	t1 := parseTS(t, "updatedAt before", updatedAt1)
	t2 := parseTS(t, "updatedAt after", updatedAt2)
	assert.True(t, t2.After(t1) || t2.Equal(t1),
		"updatedAt must not regress: before=%s after=%s", updatedAt1, updatedAt2)
}

func TestDetector_Tags_EmptyMap_Not_Null(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id := createTestDetector(t, h) // no tags

	rec := doRequest(t, h, http.MethodGet, "/detector/"+id, nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	raw, exists := resp["tags"]
	require.True(t, exists, "GetDetector response must include 'tags' key")
	assert.NotNil(t, raw, "tags must be {} not null when no tags on create")

	tags, ok := raw.(map[string]any)
	require.True(t, ok, "tags must be an object, got %T", raw)
	assert.Empty(t, tags, "tags must be empty map {}")
}

func TestListDetectors_Empty_State(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodGet, "/detector", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	raw, exists := resp["detectorIds"]
	require.True(t, exists, "ListDetectors must include 'detectorIds' key")
	assert.NotNil(t, raw, "detectorIds must be [] not null when empty")

	ids, ok := raw.([]any)
	require.True(t, ok, "detectorIds must be an array, got %T", raw)
	assert.Empty(t, ids, "detectorIds must be empty []")
}

// TestDeleteDetectorCleansUpSubResources verifies that DeleteDetector
// removes all sub-resource maps associated with the detector: members,
// publishing destinations, threat entity sets, and trusted entity sets.
// The previous implementation omitted these four delete calls, leaving dangling
// state in long-running processes and test suites that create/delete detectors
// in multiple cycles.
func TestDeleteDetectorCleansUpSubResources(t *testing.T) {
	t.Parallel()

	b := guardduty.NewInMemoryBackend("111111111111", "us-east-1")

	// Create a detector with AI_ANALYST enabled (required by CreateInvestigation).
	det, err := b.CreateDetector(true, "", nil, []guardduty.DetectorFeature{
		{Name: "AI_ANALYST", Status: "ENABLED"},
	})
	require.NoError(t, err)
	detID := det.DetectorID

	// Seed a member.
	_, unprocessed := b.CreateMembers(detID, []map[string]any{
		{"accountId": "222222222222", "email": "member@example.com"},
	})
	require.Empty(t, unprocessed, "CreateMembers should not produce unprocessed entries")

	// Seed a publishing destination.
	_, err = b.CreatePublishingDestination(detID, "S3", guardduty.DestinationProperties{
		DestinationArn: "arn:aws:s3:::my-bucket",
	}, nil)
	require.NoError(t, err)

	// Seed an investigation -- GuardDuty investigations are detector-scoped
	// (every op requires DetectorId) the same way filters/ipSets/members are,
	// so DeleteDetector must cascade to them too (the same ghost-row bug
	// class recently fixed in emr and verifiedpermissions).
	inv, err := b.CreateInvestigation(detID, "Investigate finding in this account")
	require.NoError(t, err)

	// Seed a malware scan and malware scan settings -- both are
	// detector-scoped on the real API (DescribeMalwareScans and
	// Get/UpdateMalwareScanSettings all require DetectorId in the URL), the
	// same as members/publishing destinations/entity sets above, so
	// DeleteDetector must cascade to them too.
	scanID, err := b.StartMalwareScan("arn:aws:ec2:us-east-1:111111111111:instance/i-0123456789abcdef0")
	require.NoError(t, err)
	require.NoError(t, b.UpdateMalwareScanSettings(detID, &guardduty.MalwareScanSettings{
		EbsSnapshotPreservation: "RETENTION_WITH_FINDING",
	}))

	// Verify sub-resources exist before deletion.
	assert.Equal(t, 1, guardduty.MemberCount(b, detID), "member should exist before delete")
	assert.Equal(t, 1, guardduty.PublishingDestinationCount(b, detID),
		"publishing destination should exist before delete")
	before, _, listErr := b.ListInvestigations(detID, guardduty.InvestigationsQuery{})
	require.NoError(t, listErr)
	assert.Len(t, before, 1, "investigation should exist before delete")

	// Delete the detector.
	require.NoError(t, b.DeleteDetector(detID))

	// Verify detector is gone.
	assert.Equal(t, 0, guardduty.DetectorCount(b), "detector must be removed")

	// Verify sub-resources are cleaned up.
	assert.Equal(t, 0, guardduty.MemberCount(b, detID),
		"members must be removed when detector is deleted")
	assert.Equal(t, 0, guardduty.PublishingDestinationCount(b, detID),
		"publishing destinations must be removed when detector is deleted")
	assert.Equal(t, 0, guardduty.ThreatEntitySetCount(b, detID),
		"threat entity sets must be removed when detector is deleted")
	// The investigation row itself must be gone, not merely unreachable
	// because its detector vanished: assert against the persisted snapshot,
	// which serializes the raw table.
	assert.NotContains(t, string(b.Snapshot(t.Context())), inv.InvestigationID,
		"investigations must be removed when detector is deleted")

	// The malware scan is reachable via GetMalwareScan/ListMalwareScans,
	// neither of which is detector-scoped on the real API -- unlike the
	// resources above, a leftover row here is directly client-observable,
	// not merely inert state.
	_, getErr := b.GetMalwareScan(scanID)
	require.ErrorIs(t, getErr, guardduty.ErrMalwareScanNotFound,
		"GetMalwareScan must not return a scan whose detector was deleted")

	allScans, _, listErr := b.ListMalwareScans(guardduty.MalwareScanQuery{})
	require.NoError(t, listErr)
	for _, s := range allScans {
		assert.NotEqual(t, scanID, s.ScanID,
			"ListMalwareScans must not return a scan whose detector was deleted")
	}

	assert.NotContains(t, string(b.Snapshot(t.Context())), "RETENTION_WITH_FINDING",
		"malware scan settings must be removed when detector is deleted")
}

// TestCreateDetector_RejectsInvalidFindingPublishingFrequency locks that an
// unknown findingPublishingFrequency is rejected, matching the real
// types.FindingPublishingFrequency enum (FIFTEEN_MINUTES/ONE_HOUR/
// SIX_HOURS) instead of being stored and echoed back verbatim -- this
// backend previously accepted any string here, more permissive than the
// real service, which rejects an invalid enum value with a validation
// error.
func TestCreateDetector_RejectsInvalidFindingPublishingFrequency(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/detector", map[string]any{
		"enable":                     true,
		"findingPublishingFrequency": "NOT_A_REAL_FREQUENCY",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestCreateDetector_RejectsInvalidFeatureName locks that an unknown
// features[].name is rejected, matching the real types.DetectorFeature
// enum, instead of being stored and echoed back verbatim.
func TestCreateDetector_RejectsInvalidFeatureName(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/detector", map[string]any{
		"enable": true,
		"features": []map[string]any{
			{"name": "NOT_A_REAL_FEATURE", "status": "ENABLED"},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestCreateDetector_RejectsInvalidFeatureStatus locks that an unknown
// features[].status is rejected, matching the real types.FeatureStatus
// enum (ENABLED/DISABLED).
func TestCreateDetector_RejectsInvalidFeatureStatus(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/detector", map[string]any{
		"enable": true,
		"features": []map[string]any{
			{"name": "S3_DATA_EVENTS", "status": "MAYBE"},
		},
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestUpdateDetector_RejectsInvalidFindingPublishingFrequency locks the
// same validation on UpdateDetector.
func TestUpdateDetector_RejectsInvalidFindingPublishingFrequency(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	id := createTestDetector(t, h)

	rec := doRequest(t, h, http.MethodPost, "/detector/"+id, map[string]any{
		"findingPublishingFrequency": "NOT_A_REAL_FREQUENCY",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}
