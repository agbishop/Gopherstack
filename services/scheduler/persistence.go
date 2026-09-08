package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// snapshotTimeLayout is the time format used when persisting timestamps.
const snapshotTimeLayout = time.RFC3339Nano

// schedulerSnapshotVersion identifies the shape of [backendSnapshot]. It must be
// bumped whenever a change to a DTO type or backendSnapshot itself would make an
// older snapshot unsafe to decode as the current shape. Restore compares this
// against the persisted value and discards (rather than partially decodes) any
// mismatch -- see Restore below. This is the first version: earlier
// (pre-Phase-3.3) snapshots carried no version field at all, so they decode as 0
// and are discarded rather than misread, which is the safe behaviour across any
// snapshot-format change.
const schedulerSnapshotVersion = 1

// persistedSchedule is the serialisation-friendly representation of Schedule.
// It replaces the live *tags.Tags pointer with a plain map so JSON round-trips
// work without carrying Prometheus metric state, and stores timestamps as
// strings. It doubles as the store.Table DTO value type for the "schedules"
// snapshot table (see buildPersistenceDTORegistry); its Region field carries the
// live table's composite-key region through the round-trip.
type persistedSchedule struct {
	CreationDate               string             `json:"creationDate"`
	LastModificationDate       string             `json:"lastModificationDate"`
	StartDate                  string             `json:"startDate,omitempty"`
	EndDate                    string             `json:"endDate,omitempty"`
	Tags                       map[string]string  `json:"tags,omitempty"`
	Target                     Target             `json:"target"`
	Name                       string             `json:"name"`
	ARN                        string             `json:"arn"`
	ScheduleExpression         string             `json:"scheduleExpression"`
	ScheduleExpressionTimezone string             `json:"scheduleExpressionTimezone,omitempty"`
	Description                string             `json:"description,omitempty"`
	GroupName                  string             `json:"groupName"`
	State                      string             `json:"state"`
	ActionAfterCompletion      string             `json:"actionAfterCompletion,omitempty"`
	KmsKeyArn                  string             `json:"kmsKeyArn,omitempty"`
	AccountID                  string             `json:"accountID"`
	Region                     string             `json:"region"`
	FlexibleTimeWindow         FlexibleTimeWindow `json:"flexibleTimeWindow"`
}

// persistedScheduleKeyFn keys the schedules DTO table by ARN, which uniquely
// identifies a schedule (region + group + name are all encoded in it).
func persistedScheduleKeyFn(v *persistedSchedule) string { return v.ARN }

// persistedScheduleGroup is the serialisation-friendly representation of
// ScheduleGroup. Its region is carried in its ARN, so no separate field is
// needed; it doubles as the store.Table DTO value type for the "scheduleGroups"
// snapshot table.
type persistedScheduleGroup struct {
	CreationDate         string            `json:"creationDate"`
	LastModificationDate string            `json:"lastModificationDate"`
	Tags                 map[string]string `json:"tags,omitempty"`
	Name                 string            `json:"name"`
	ARN                  string            `json:"arn"`
	State                string            `json:"state"`
}

// persistedScheduleGroupKeyFn keys the scheduleGroups DTO table by ARN, which
// uniquely identifies a group (its region and name are both encoded in it).
func persistedScheduleGroupKeyFn(v *persistedScheduleGroup) string { return v.ARN }

// backendSnapshot is the top-level on-disk shape for the Scheduler backend.
//
// Tables holds one JSON-encoded array per DTO table produced by
// [store.Registry.SnapshotAll] -- "schedules" and "scheduleGroups", each wrapped
// in a plain-map DTO (persistedSchedule/persistedScheduleGroup) so the live
// *tags.Tags and time.Time fields round-trip cleanly. Version guards against
// decoding a snapshot from an incompatible (older or newer) build of this
// backend as though it were the current shape; see Restore.
type backendSnapshot struct {
	Tables    map[string]json.RawMessage `json:"tables"`
	AccountID string                     `json:"accountID"`
	Region    string                     `json:"region"`
	Version   int                        `json:"version"`
}

// buildPersistenceDTORegistry constructs the ephemeral DTO registry used by both
// Snapshot and Restore. It is built fresh on every call (rather than reusing a
// live-table registry) because the DTO value types (persistedSchedule/
// persistedScheduleGroup) differ from the live table value types
// (Schedule/ScheduleGroup) -- see the store_setup.go doc for why the live tables
// are persistence-dirty and unregistered.
func buildPersistenceDTORegistry() (
	*store.Registry,
	*store.Table[persistedSchedule],
	*store.Table[persistedScheduleGroup],
) {
	dtoReg := store.NewRegistry()
	scheduleDTOs := store.Register(dtoReg, "schedules", store.New(persistedScheduleKeyFn))
	groupDTOs := store.Register(dtoReg, "scheduleGroups", store.New(persistedScheduleGroupKeyFn))

	return dtoReg, scheduleDTOs, groupDTOs
}

// parseSnapshotTime parses a timestamp stored by Snapshot.
// Returns [time.Time]{} on failure so the caller proceeds with a sensible default.
func parseSnapshotTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}

	t, err := time.Parse(snapshotTimeLayout, s)
	if err != nil {
		return time.Time{}
	}

	return t
}

// toPersistedSchedule builds the plain-map DTO for a live schedule.
func toPersistedSchedule(s *Schedule) *persistedSchedule {
	var tagMap map[string]string
	if s.Tags != nil {
		tagMap = s.Tags.Clone()
	}

	ps := &persistedSchedule{
		Name:                       s.Name,
		ARN:                        s.ARN,
		GroupName:                  s.GroupName,
		ScheduleExpression:         s.ScheduleExpression,
		ScheduleExpressionTimezone: s.ScheduleExpressionTimezone,
		Description:                s.Description,
		Target:                     s.Target,
		State:                      s.State,
		FlexibleTimeWindow:         s.FlexibleTimeWindow,
		ActionAfterCompletion:      s.ActionAfterCompletion,
		KmsKeyArn:                  s.KmsKeyArn,
		AccountID:                  s.AccountID,
		Region:                     s.Region,
		CreationDate:               s.CreationDate.Format(snapshotTimeLayout),
		LastModificationDate:       s.LastModificationDate.Format(snapshotTimeLayout),
		Tags:                       tagMap,
	}

	if s.StartDate != nil {
		ps.StartDate = s.StartDate.Format(snapshotTimeLayout)
	}

	if s.EndDate != nil {
		ps.EndDate = s.EndDate.Format(snapshotTimeLayout)
	}

	return ps
}

// toPersistedScheduleGroup builds the plain-map DTO for a live schedule group.
func toPersistedScheduleGroup(g *ScheduleGroup) *persistedScheduleGroup {
	var tagMap map[string]string
	if g.Tags != nil {
		tagMap = g.Tags.Clone()
	}

	return &persistedScheduleGroup{
		Name:                 g.Name,
		ARN:                  g.ARN,
		State:                g.State,
		CreationDate:         g.CreationDate.Format(snapshotTimeLayout),
		LastModificationDate: g.LastModificationDate.Format(snapshotTimeLayout),
		Tags:                 tagMap,
	}
}

// Snapshot serialises the backend state to JSON.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	dtoReg, scheduleDTOs, groupDTOs := buildPersistenceDTORegistry()

	for _, s := range b.schedules.Snapshot() {
		scheduleDTOs.Put(toPersistedSchedule(s))
	}

	for _, g := range b.scheduleGroups.Snapshot() {
		groupDTOs.Put(toPersistedScheduleGroup(g))
	}

	tables, err := dtoReg.SnapshotAll()
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "scheduler: failed to snapshot state", "error", err)

		return nil
	}

	snap := backendSnapshot{
		Version:   schedulerSnapshotVersion,
		Tables:    tables,
		AccountID: b.accountID,
		Region:    b.region,
	}

	return persistence.MarshalSnapshot(ctx, "scheduler", &snap)
}

// Restore loads backend state from a JSON snapshot.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "scheduler", data, &snap); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	b.closeAllTagMetrics()

	if snap.Version != schedulerSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption.
		logger.Load(ctx).WarnContext(ctx,
			"scheduler: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", schedulerSnapshotVersion)

		b.schedules.Reset()
		b.scheduleGroups.Reset()
		b.accountID = snap.AccountID
		b.region = snap.Region
		b.seedDefaultGroup(b.region)

		return nil
	}

	if err := b.restoreResourceTables(snap.Tables); err != nil {
		return err
	}

	b.accountID = snap.AccountID
	b.region = snap.Region

	// Ensure the default group always exists after restore in the default region.
	if !b.scheduleGroups.Has(regionKey(b.region, defaultGroupName)) {
		b.seedDefaultGroup(b.region)
	}

	return nil
}

// restoreResourceTables rebuilds the schedules and scheduleGroups store.Table
// values from snap's per-table JSON, factored out of Restore to keep Restore's
// own cognitive complexity low. Callers must hold b.mu.Lock.
func (b *InMemoryBackend) restoreResourceTables(tables map[string]json.RawMessage) error {
	dtoReg, scheduleDTOs, groupDTOs := buildPersistenceDTORegistry()

	if err := dtoReg.RestoreAll(tables); err != nil {
		return fmt.Errorf("scheduler: restore snapshot tables: %w", err)
	}

	schedules := make([]*Schedule, 0, scheduleDTOs.Len())
	for _, ps := range scheduleDTOs.All() {
		schedules = append(schedules, scheduleFromPersisted(ps))
	}
	b.schedules.Restore(schedules)

	groups := make([]*ScheduleGroup, 0, groupDTOs.Len())
	for _, pg := range groupDTOs.All() {
		groups = append(groups, scheduleGroupFromPersisted(pg.Name, pg))
	}
	b.scheduleGroups.Restore(groups)

	return nil
}

// closeAllTagMetrics releases the Prometheus metrics held by all live schedules
// and schedule groups across every region. Callers must hold b.mu.
func (b *InMemoryBackend) closeAllTagMetrics() {
	for _, s := range b.schedules.All() {
		if s.Tags != nil {
			s.Tags.Close()
		}
	}

	for _, g := range b.scheduleGroups.All() {
		if g.Tags != nil {
			g.Tags.Close()
		}
	}
}

// scheduleFromPersisted rebuilds a live Schedule from its persisted representation.
func scheduleFromPersisted(ps *persistedSchedule) *Schedule {
	groupName := ps.GroupName
	if groupName == "" {
		groupName = defaultGroupName
	}

	s := &Schedule{
		Name:                       ps.Name,
		ARN:                        ps.ARN,
		GroupName:                  groupName,
		ScheduleExpression:         ps.ScheduleExpression,
		ScheduleExpressionTimezone: ps.ScheduleExpressionTimezone,
		Description:                ps.Description,
		Target:                     ps.Target,
		State:                      ps.State,
		FlexibleTimeWindow:         ps.FlexibleTimeWindow,
		ActionAfterCompletion:      ps.ActionAfterCompletion,
		KmsKeyArn:                  ps.KmsKeyArn,
		AccountID:                  ps.AccountID,
		Region:                     ps.Region,
		CreationDate:               parseSnapshotTime(ps.CreationDate),
		LastModificationDate:       parseSnapshotTime(ps.LastModificationDate),
		Tags: tags.FromMap(
			"scheduler.schedule."+groupName+"."+ps.Name+".tags",
			ps.Tags,
		),
	}

	if ps.StartDate != "" {
		t := parseSnapshotTime(ps.StartDate)
		s.StartDate = &t
	}

	if ps.EndDate != "" {
		t := parseSnapshotTime(ps.EndDate)
		s.EndDate = &t
	}

	return s
}

// scheduleGroupFromPersisted rebuilds a live ScheduleGroup from its persisted representation.
func scheduleGroupFromPersisted(name string, pg *persistedScheduleGroup) *ScheduleGroup {
	return &ScheduleGroup{
		Name:                 pg.Name,
		ARN:                  pg.ARN,
		State:                pg.State,
		CreationDate:         parseSnapshotTime(pg.CreationDate),
		LastModificationDate: parseSnapshotTime(pg.LastModificationDate),
		Tags:                 tags.FromMap("scheduler.schedulegroup."+name+".tags", pg.Tags),
	}
}

// Snapshot implements persistence.Persistable by delegating to the backend.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	return h.Backend.Snapshot(ctx)
}

// Restore implements persistence.Persistable by delegating to the backend.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	return h.Backend.Restore(ctx, data)
}

// Reset implements service.Resettable by delegating to the backend.
func (h *Handler) Reset() {
	h.Backend.Reset()
	h.idempotency.Clear()
}
