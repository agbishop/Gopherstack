package medialive_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/medialive"
)

func TestOfferings_ListDescribe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check    func(t *testing.T, body []byte)
		name     string
		wantCode int
	}{
		{
			name:     "list returns seeded offerings",
			wantCode: http.StatusOK,
			check: func(t *testing.T, body []byte) {
				t.Helper()
				var resp map[string]any
				require.NoError(t, json.Unmarshal(body, &resp))
				offerings := resp["offerings"].([]any)
				assert.GreaterOrEqual(t, len(offerings), 3)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(t)
			rec := doRequest(t, h, http.MethodGet, "/prod/offerings", nil)
			assert.Equal(t, tc.wantCode, rec.Code)
			if tc.check != nil {
				tc.check(t, rec.Body.Bytes())
			}
		})
	}
}

func TestOfferings_Describe(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/prod/offerings/87654321", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "87654321", resp["offeringId"])
	assert.NotEmpty(t, resp["region"])
	_, hasName := resp["name"]
	assert.False(t, hasName, "real DescribeOfferingOutput has no name field")

	// Unknown offering
	rec = doRequest(t, h, http.MethodGet, "/prod/offerings/99999999", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestReservations_PurchaseListDescribeDeleteUpdate(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	// Purchase
	rec := doRequest(t, h, http.MethodPost, "/prod/offerings/87654321/purchase", map[string]any{
		"name":  "test-reservation",
		"count": 2.0,
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	var purchaseResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &purchaseResp))
	resv := purchaseResp["reservation"].(map[string]any)
	reservationID := resv["reservationId"].(string)
	assert.NotEmpty(t, reservationID)
	assert.Equal(t, "ACTIVE", resv["state"], "a term starting now hasn't ended yet")
	assert.InDelta(t, float64(2), resv["count"], 0.001)

	// Describe
	rec = doRequest(t, h, http.MethodGet, "/prod/reservations/"+reservationID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	// List
	rec = doRequest(t, h, http.MethodGet, "/prod/reservations", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	assert.Len(t, listResp["reservations"].([]any), 1)

	// Update name
	rec = doRequest(t, h, http.MethodPut, "/prod/reservations/"+reservationID, map[string]any{
		"name": "renamed-reservation",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	var updatedResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updatedResp))
	updatedResv := updatedResp["reservation"].(map[string]any)
	assert.Equal(t, "renamed-reservation", updatedResv["name"])

	// DeleteReservation requires the reservation to already be EXPIRED
	// (covered separately by TestReservations_DeleteRequiresExpired); force
	// the term into the past here so this round-trip can reach delete.
	medialive.ForceReservationEnd(h.Backend.(*medialive.InMemoryBackend), reservationID, "2000-01-01T00:00:00Z")
	rec = doRequest(t, h, http.MethodDelete, "/prod/reservations/"+reservationID, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = doRequest(t, h, http.MethodGet, "/prod/reservations/"+reservationID, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestReservations_RenewalSettings locks in a fix for a gap where
// gopherstack didn't track "renewalSettings" at all (a real field on
// DescribeReservationOutput/Reservation -- verified against
// aws-sdk-go-v2/service/medialive's Reservation/RenewalSettings types):
// PurchaseOffering/UpdateReservation silently discarded any renewalSettings
// a caller sent, and it was never echoed back.
func TestReservations_RenewalSettings(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	rec := doRequest(t, h, http.MethodPost, "/prod/offerings/87654321/purchase", map[string]any{
		"name":  "renewal-reservation",
		"count": 1.0,
		"renewalSettings": map[string]any{
			"automaticRenewal": "ENABLED",
			"renewalCount":     3.0,
		},
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	purchased := decodeBody(t, rec.Body.Bytes())["reservation"].(map[string]any)
	reservationID := purchased["reservationId"].(string)

	rs := purchased["renewalSettings"].(map[string]any)
	assert.Equal(t, "ENABLED", rs["automaticRenewal"])
	assert.InDelta(t, float64(3), rs["renewalCount"], 0)

	// Describe echoes the same renewalSettings back.
	rec = doRequest(t, h, http.MethodGet, "/prod/reservations/"+reservationID, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	described := decodeBody(t, rec.Body.Bytes())
	rs = described["renewalSettings"].(map[string]any)
	assert.Equal(t, "ENABLED", rs["automaticRenewal"])

	// Update with a new renewalSettings object overwrites it.
	rec = doRequest(t, h, http.MethodPut, "/prod/reservations/"+reservationID, map[string]any{
		"renewalSettings": map[string]any{"automaticRenewal": "DISABLED"},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	updated := decodeBody(t, rec.Body.Bytes())["reservation"].(map[string]any)
	rs = updated["renewalSettings"].(map[string]any)
	assert.Equal(t, "DISABLED", rs["automaticRenewal"])

	// A reservation purchased without renewalSettings omits the key
	// entirely, matching a real never-configured reservation.
	rec = doRequest(t, h, http.MethodPost, "/prod/offerings/87654321/purchase", map[string]any{
		"name": "no-renewal",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	noRenewal := decodeBody(t, rec.Body.Bytes())["reservation"].(map[string]any)
	_, hasRenewal := noRenewal["renewalSettings"]
	assert.False(t, hasRenewal)
}

// TestPurchaseOffering_DerivesTermFromDuration covers gopherstack-b668:
// PurchaseOffering used to fabricate a frozen Start=2024-01-01/End=2025-01-01
// on every purchase instead of deriving the term from the offering's
// Duration/DurationUnits (medialive/types/types.go's Offering -- the only
// declared OfferingDurationUnits value is MONTHS, types/enums.go). With no
// "start" in the request, Start must default to now and End must be exactly
// Start plus Duration months.
func TestPurchaseOffering_DerivesTermFromDuration(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	offRec := doRequest(t, h, http.MethodGet, "/prod/offerings/87654321", nil)
	require.Equal(t, http.StatusOK, offRec.Code)
	offering := decodeBody(t, offRec.Body.Bytes())
	duration := int(offering["duration"].(float64))
	require.Equal(t, "MONTHS", offering["durationUnits"])

	rec := doRequest(t, h, http.MethodPost, "/prod/offerings/87654321/purchase", map[string]any{
		"name": "term-test",
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	resv := decodeBody(t, rec.Body.Bytes())["reservation"].(map[string]any)

	start, err := time.Parse(time.RFC3339, resv["start"].(string))
	require.NoError(t, err)
	end, err := time.Parse(time.RFC3339, resv["end"].(string))
	require.NoError(t, err)

	assert.WithinDuration(t, time.Now().UTC(), start, time.Minute, "Start must default to now, not a fixed past date")
	assert.Equal(
		t, start.AddDate(0, duration, 0), end,
		"End must be Start plus the offering's Duration in DurationUnits, not a fixed date",
	)
}

// TestPurchaseOffering_HonorsExplicitStart covers gopherstack-b668:
// PurchaseOfferingInput.Start (api_op_PurchaseOffering.go: "Requested
// reservation start time ... If no value is given, the default is now")
// lets a caller pin the term start; the fabricated Start/End ignored it
// entirely. The chosen start is 7 days out from the current wall clock, not
// a fixed date -- api_op_PurchaseOffering.go also bounds Start to "the first
// day of the current month and one year from now" (gopherstack-f6dz), so a
// frozen date eventually falls outside that window and starts failing.
func TestPurchaseOffering_HonorsExplicitStart(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	offRec := doRequest(t, h, http.MethodGet, "/prod/offerings/87654321", nil)
	require.Equal(t, http.StatusOK, offRec.Code)
	offering := decodeBody(t, offRec.Body.Bytes())
	duration := int(offering["duration"].(float64))

	start := time.Now().UTC().AddDate(0, 0, 7).Truncate(time.Second)

	rec := doRequest(t, h, http.MethodPost, "/prod/offerings/87654321/purchase", map[string]any{
		"name":  "explicit-start-test",
		"start": start.Format(time.RFC3339),
	})
	require.Equal(t, http.StatusCreated, rec.Code)
	resv := decodeBody(t, rec.Body.Bytes())["reservation"].(map[string]any)

	assert.Equal(t, start.Format(time.RFC3339), resv["start"])
	assert.Equal(
		t, start.AddDate(0, duration, 0).Format(time.RFC3339), resv["end"],
		"12-month term derived from Duration/DurationUnits",
	)
}

// TestPurchaseOffering_StartWindow covers gopherstack-f6dz:
// api_op_PurchaseOffering.go documents Start as "The specified time must be
// between the first day of the current month and one year from now", but
// PurchaseOffering accepted any well-formed RFC3339 value regardless of
// window. Both bounds are read as inclusive and relative to the backend's
// clock (nowFunc), not wall-clock, so each case pins nowFunc via SetNow and
// derives its Start from that same fixed instant -- never a hardcoded date.
func TestPurchaseOffering_StartWindow(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC)
	lowerBound := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	upperBound := fixedNow.AddDate(1, 0, 0)

	tests := []struct {
		start      time.Time
		name       string
		wantAccept bool
	}{
		{name: "first instant of current month accepted", start: lowerBound, wantAccept: true},
		{name: "last instant of previous month rejected", start: lowerBound.Add(-time.Second), wantAccept: false},
		{name: "exactly one year from now accepted", start: upperBound, wantAccept: true},
		{name: "one second past one year from now rejected", start: upperBound.Add(time.Second), wantAccept: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler(t)
			medialive.SetNow(h.Backend.(*medialive.InMemoryBackend), func() time.Time { return fixedNow })

			rec := doRequest(t, h, http.MethodPost, "/prod/offerings/87654321/purchase", map[string]any{
				"name":  "start-window-test",
				"start": tc.start.Format(time.RFC3339),
			})

			if tc.wantAccept {
				assert.Equal(t, http.StatusCreated, rec.Code)

				return
			}

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "BadRequestException", rec.Header().Get("X-Amzn-Errortype"))
		})
	}
}

// TestReservations_DeleteRequiresExpired locks in a fix for
// gopherstack-1um: DeleteReservation had no state guard at all, so any
// ACTIVE (or CANCELED) reservation could be deleted -- real DeleteReservation
// requires State == EXPIRED first (api_op_DeleteReservation.go: "Delete an
// expired reservation.").
func TestReservations_DeleteRequiresExpired(t *testing.T) {
	t.Parallel()

	purchase := func(t *testing.T, h *medialive.Handler, name string) string {
		t.Helper()

		rec := doRequest(t, h, http.MethodPost, "/prod/offerings/87654321/purchase", map[string]any{
			"name": name,
		})
		require.Equal(t, http.StatusCreated, rec.Code)

		return decodeBody(t, rec.Body.Bytes())["reservation"].(map[string]any)["reservationId"].(string)
	}

	t.Run("still_within_term_is_rejected", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		id := purchase(t, h, "active-test")
		medialive.ForceReservationEnd(h.Backend.(*medialive.InMemoryBackend), id, "2999-01-01T00:00:00Z")

		rec := doRequest(t, h, http.MethodGet, "/prod/reservations/"+id, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "ACTIVE", decodeBody(t, rec.Body.Bytes())["state"])

		rec = doRequest(t, h, http.MethodDelete, "/prod/reservations/"+id, nil)
		assert.Equal(t, http.StatusConflict, rec.Code)

		rec = doRequest(t, h, http.MethodGet, "/prod/reservations/"+id, nil)
		assert.Equal(t, http.StatusOK, rec.Code, "reservation must survive the rejected delete")
	})

	t.Run("past_term_end_is_deletable", func(t *testing.T) {
		t.Parallel()

		h := newTestHandler(t)
		id := purchase(t, h, "expired-test")
		medialive.ForceReservationEnd(h.Backend.(*medialive.InMemoryBackend), id, "2000-01-01T00:00:00Z")

		rec := doRequest(t, h, http.MethodGet, "/prod/reservations/"+id, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "EXPIRED", decodeBody(t, rec.Body.Bytes())["state"])

		rec = doRequest(t, h, http.MethodDelete, "/prod/reservations/"+id, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "CANCELED", decodeBody(t, rec.Body.Bytes())["state"])

		rec = doRequest(t, h, http.MethodGet, "/prod/reservations/"+id, nil)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
