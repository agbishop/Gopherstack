package medialive

import (
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// --- Offering operations ---

// ListOfferings returns the seeded offering catalog.
func (b *InMemoryBackend) ListOfferings(
	maxResults int,
	nextToken string,
) ([]*Offering, string, error) {
	b.mu.RLock("ListOfferings")
	defer b.mu.RUnlock()
	pg := page.New(b.offerings, nextToken, maxResults, defaultMaxResults)
	result := make([]*Offering, len(pg.Data))
	copy(result, pg.Data)

	return result, pg.Next, nil
}

// DescribeOffering returns a single offering by ID.
func (b *InMemoryBackend) DescribeOffering(offeringID string) (*Offering, error) {
	b.mu.RLock("DescribeOffering")
	defer b.mu.RUnlock()
	for _, o := range b.offerings {
		if o.OfferingID == offeringID {
			cp := *o

			return &cp, nil
		}
	}

	return nil, fmt.Errorf("%w: offering %s not found", ErrNotFound, offeringID)
}

// --- Reservation operations ---

// PurchaseOffering creates a Reservation from an Offering. start is the
// caller's requested term start (PurchaseOfferingInput.Start, ISO-8601); an
// empty string means "now", matching the SDK doc ("If no value is given,
// the default is now"). A non-empty start must fall within the SDK's
// documented window -- "between the first day of the current month and one
// year from now" -- both ends read as inclusive and both relative to the
// request time (b.now()), not any fixed date.
func (b *InMemoryBackend) PurchaseOffering(
	offeringID, name, start string,
	count int32,
	renewalSettings RenewalSettings,
	tags map[string]string,
) (*Reservation, error) {
	b.mu.Lock("PurchaseOffering")
	defer b.mu.Unlock()
	var off *Offering
	for _, o := range b.offerings {
		if o.OfferingID == offeringID {
			cp := *o
			off = &cp

			break
		}
	}
	if off == nil {
		return nil, fmt.Errorf("%w: offering %s not found", ErrNotFound, offeringID)
	}
	if count <= 0 {
		count = 1
	}
	now := b.now()
	startTime := now
	if start != "" {
		parsed, err := time.Parse(time.RFC3339, start)
		if err != nil {
			return nil, fmt.Errorf("%w: start %q is not RFC3339", ErrInvalidParameter, start)
		}
		startTime = parsed.UTC()
		lower := firstOfMonthUTC(now)
		upper := now.AddDate(1, 0, 0)
		if startTime.Before(lower) || startTime.After(upper) {
			return nil, fmt.Errorf(
				"%w: start %q must be between the first day of the current month (%s) and one year from now (%s)",
				ErrInvalidParameter, start, lower.Format(time.RFC3339), upper.Format(time.RFC3339),
			)
		}
	}
	endTime := addOfferingTerm(startTime, off.Duration, off.DurationUnits)
	id := newID()
	r := &storedReservation{
		Tags:                  copyTags(tags),
		ResourceSpecification: off.ResourceSpecification,
		RenewalSettings:       renewalSettings,
		Arn:                   b.reservationARN(id),
		ReservationID:         id,
		Name:                  name,
		OfferingID:            off.OfferingID,
		OfferingDescription:   off.OfferingDescription,
		OfferingType:          off.OfferingType,
		CurrencyCode:          off.CurrencyCode,
		FixedPrice:            off.FixedPrice,
		UsagePrice:            off.UsagePrice,
		Duration:              off.Duration,
		DurationUnits:         off.DurationUnits,
		Start:                 startTime.Format(time.RFC3339),
		End:                   endTime.Format(time.RFC3339),
		Region:                b.region,
		State:                 "ACTIVE",
		Count:                 count,
	}
	b.reservations.Put(r)

	return r.toReservation(), nil
}

// firstOfMonthUTC returns 00:00:00 UTC on the first day of t's UTC month.
func firstOfMonthUTC(t time.Time) time.Time {
	y, m, _ := t.Date()

	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

// addOfferingTerm adds a lease term to t. OfferingDurationUnits
// (medialive/types/enums.go) declares exactly one value, MONTHS, so no
// other unit is handled.
func addOfferingTerm(t time.Time, duration int32, units string) time.Time {
	if units != offeringDurationMonths {
		return t
	}

	return t.AddDate(0, int(duration), 0)
}

// ReservationFilter mirrors the ResourceSpecification-backed
// ListReservationsInput query filters this backend can honestly answer
// (codec/maximumBitrate/maximumFramerate/resolution/resourceType/
// specialFeature/videoQuality -- api_op_ListReservations.go, all bound as
// httpQuery). ChannelClass is deliberately excluded: neither Offering nor
// storedReservation tracks it anywhere in this backend, so it stays a
// disclosed structural gap rather than a fabricated match. An empty field
// means "no constraint on that attribute".
type ReservationFilter struct {
	Codec            string
	MaximumBitrate   string
	MaximumFramerate string
	Resolution       string
	ResourceType     string
	SpecialFeature   string
	VideoQuality     string
}

func (f ReservationFilter) matches(spec OfferingResourceSpecification) bool {
	return (f.Codec == "" || f.Codec == spec.Codec) &&
		(f.MaximumBitrate == "" || f.MaximumBitrate == spec.MaximumBitrate) &&
		(f.MaximumFramerate == "" || f.MaximumFramerate == spec.MaximumFramerate) &&
		(f.Resolution == "" || f.Resolution == spec.Resolution) &&
		(f.ResourceType == "" || f.ResourceType == spec.ResourceType) &&
		(f.SpecialFeature == "" || f.SpecialFeature == spec.SpecialFeature) &&
		(f.VideoQuality == "" || f.VideoQuality == spec.VideoQuality)
}

// ListReservations returns reservations matching filter.
func (b *InMemoryBackend) ListReservations(
	maxResults int,
	nextToken string,
	filter ReservationFilter,
) ([]*Reservation, string, error) {
	b.mu.RLock("ListReservations")
	defer b.mu.RUnlock()
	all := b.reservations.All()

	matched := make([]*storedReservation, 0, len(all))

	for _, r := range all {
		if filter.matches(r.ResourceSpecification) {
			matched = append(matched, r)
		}
	}

	all = matched
	sort.Slice(all, func(i, j int) bool { return all[i].ReservationID < all[j].ReservationID })
	pg := page.New(all, nextToken, maxResults, defaultMaxResults)
	result := make([]*Reservation, 0, len(pg.Data))
	for _, r := range pg.Data {
		result = append(result, r.toReservation())
	}

	return result, pg.Next, nil
}

// DescribeReservation returns a single reservation.
func (b *InMemoryBackend) DescribeReservation(reservationID string) (*Reservation, error) {
	b.mu.RLock("DescribeReservation")
	defer b.mu.RUnlock()
	r, ok := b.reservations.Get(reservationID)
	if !ok {
		return nil, fmt.Errorf("%w: reservation %s not found", ErrNotFound, reservationID)
	}

	return r.toReservation(), nil
}

// effectiveState derives ACTIVE vs EXPIRED from the reservation term. There is
// no expiry ticker, so a stored State alone would leave EXPIRED unreachable and
// DeleteReservation permanently refusing.
func (r *storedReservation) effectiveState() string {
	if r.State != "ACTIVE" {
		return r.State
	}

	end, err := time.Parse(time.RFC3339, r.End)
	if err == nil && time.Now().After(end) {
		return "EXPIRED"
	}

	return r.State
}

// DeleteReservation cancels a reservation. api_op_DeleteReservation.go
// describes the op as deleting an expired reservation.
func (b *InMemoryBackend) DeleteReservation(reservationID string) (*Reservation, error) {
	b.mu.Lock("DeleteReservation")
	defer b.mu.Unlock()
	r, ok := b.reservations.Get(reservationID)
	if !ok {
		return nil, fmt.Errorf("%w: reservation %s not found", ErrNotFound, reservationID)
	}
	if r.effectiveState() != "EXPIRED" {
		return nil, fmt.Errorf("%w: reservation must be expired before deleting", ErrConflict)
	}
	r.State = "CANCELED"
	out := r.toReservation()
	b.reservations.Delete(reservationID)
	delete(b.tags, r.Arn)

	return out, nil
}

// UpdateReservation updates a reservation's name and, optionally, its
// renewal settings.
func (b *InMemoryBackend) UpdateReservation(
	reservationID, name string,
	renewalSettings RenewalSettings,
	hasRenewalSettings bool,
) (*Reservation, error) {
	b.mu.Lock("UpdateReservation")
	defer b.mu.Unlock()
	r, ok := b.reservations.Get(reservationID)
	if !ok {
		return nil, fmt.Errorf("%w: reservation %s not found", ErrNotFound, reservationID)
	}
	if name != "" {
		r.Name = name
	}
	if hasRenewalSettings {
		r.RenewalSettings = renewalSettings
	}

	return r.toReservation(), nil
}
