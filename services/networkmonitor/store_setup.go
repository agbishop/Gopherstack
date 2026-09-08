package networkmonitor

// Code in this file supports Phase 3.3 of the datalayer refactor: monitors,
// previously nested by region (map[string]map[string]*Monitor), is replaced
// by a single flat *store.Table[Monitor] keyed by the composite "region|name"
// string (see regionKey in backend.go), with a companion *store.Index
// grouping entries by region for ListMonitors' per-region scan -- the
// region-qualified-table pattern services/emr and services/codeartifact use.
//
// Monitor gained a new Region field (exported, no json:"-") purely to carry
// this composite-key component through JSON round-tripping. Unlike EMR's
// hidden `region` fields, this makes monitors a "clean" direct-registered
// table (services/sesv2, services/dax pattern): no DTO wrapper is needed
// since Region serializes and restores automatically as part of Monitor's
// own JSON shape. Monitor is never marshaled directly to the wire (handler.go
// always builds a dedicated response DTO -- createMonitorResponse,
// getMonitorResponse, probeWireBody, ...), so this addition is invisible to
// the AWS API.
//
// Probes are NOT converted: they were never a map (map[string]*Probe) to
// begin with, only a []*Probe slice nested inside Monitor, so there is
// nothing here for store.Table to replace.
import (
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

func monitorTableKeyFn(v *Monitor) string       { return regionKey(v.Region, v.MonitorName) }
func monitorRegionIndexKeyFn(v *Monitor) string { return v.Region }

// registerAllTables registers monitors on b.registry exactly once. It must be
// called during construction only (immediately after b.registry is created
// -- see NewInMemoryBackend), never on every Reset() -- store.Register panics
// on a duplicate name, so runtime resets go through b.registry.ResetAll()
// instead (see InMemoryBackend.Reset in backend.go).
func registerAllTables(b *InMemoryBackend) {
	b.monitors = store.Register(b.registry, "monitors", store.New(monitorTableKeyFn))
	b.monitorsByRegion = b.monitors.AddIndex("byRegion", monitorRegionIndexKeyFn)
}
