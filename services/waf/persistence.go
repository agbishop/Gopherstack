package waf

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// wafSnapshotVersion identifies the shape of [backendSnapshot]. It must be
// bumped whenever a change to backendSnapshot (or a value type held by one of
// the registered tables) would make an older snapshot unsafe to decode as the
// current shape. Restore compares this against the persisted value and
// discards (ResetAll, not a partial decode) any mismatch -- see Restore. The
// pre-Phase-3.3 snapshot format had no version field at all, so an old
// snapshot decodes with Version == 0, which is guaranteed to mismatch
// wafSnapshotVersion and is discarded the same way any other incompatible
// snapshot is.
const wafSnapshotVersion = 1

// backendSnapshot is the top-level on-disk shape for the WAF Classic backend.
//
// Tables holds one JSON-encoded array per registered table name, produced by
// b.registry.SnapshotAll() -- every store.Table-backed resource field is a
// "clean" table (see store_setup.go's file doc comment), so no ephemeral
// DTO registry is needed here. Version guards against decoding a snapshot
// from an incompatible (older or newer) build of this backend as though it
// were the current shape; see Restore.
//
// ChangeTokens, RuleGroupRules, PermissionPolicies, and Tags are left as
// plain maps -- see store_setup.go's file doc comment for why they do not fit
// store.Table's keyed-by-identity-value shape.
type backendSnapshot struct {
	Tables                 map[string]json.RawMessage   `json:"tables"`
	ChangeTokens           map[string]string            `json:"changeTokens"`
	RuleGroupRules         map[string][]ActivatedRule   `json:"ruleGroupRules"`
	PermissionPolicies     map[string]string            `json:"permissionPolicies"`
	Tags                   map[string]map[string]string `json:"tags"`
	OutstandingChangeToken string                       `json:"outstandingChangeToken,omitempty"`
	Version                int                          `json:"version"`
}

func ensureNonNilMaps(s *backendSnapshot) {
	if s.ChangeTokens == nil {
		s.ChangeTokens = make(map[string]string)
	}

	if s.RuleGroupRules == nil {
		s.RuleGroupRules = make(map[string][]ActivatedRule)
	}

	if s.PermissionPolicies == nil {
		s.PermissionPolicies = make(map[string]string)
	}

	if s.Tags == nil {
		s.Tags = make(map[string]map[string]string)
	}
}

// Snapshot serializes backend state to JSON.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	tables, err := b.registry.SnapshotAll()
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "waf: snapshot table marshal failed", "error", err)

		return nil
	}

	return persistence.MarshalSnapshot(ctx, "waf", backendSnapshot{
		Version:                wafSnapshotVersion,
		Tables:                 tables,
		ChangeTokens:           b.changeTokens,
		RuleGroupRules:         b.ruleGroupRules,
		PermissionPolicies:     b.permissionPolicies,
		Tags:                   b.tags,
		OutstandingChangeToken: b.outstandingChangeToken,
	})
}

// Restore deserializes backend state from JSON.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var s backendSnapshot
	if err := persistence.UnmarshalSnapshot(ctx, "waf", data, &s); err != nil {
		return err
	}

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if s.Version != wafSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption.
		logger.Load(ctx).WarnContext(ctx,
			"waf: discarding incompatible snapshot version, starting empty",
			"gotVersion", s.Version, "wantVersion", wafSnapshotVersion)

		b.registry.ResetAll()
		b.changeTokens = make(map[string]string)
		b.outstandingChangeToken = ""
		b.ruleGroupRules = make(map[string][]ActivatedRule)
		b.permissionPolicies = make(map[string]string)
		b.tags = make(map[string]map[string]string)

		return nil
	}

	if err := b.registry.RestoreAll(s.Tables); err != nil {
		return fmt.Errorf("waf: restore snapshot tables: %w", err)
	}

	ensureNonNilMaps(&s)

	b.changeTokens = s.ChangeTokens
	b.ruleGroupRules = s.RuleGroupRules
	b.permissionPolicies = s.PermissionPolicies
	b.tags = s.Tags
	b.outstandingChangeToken = s.OutstandingChangeToken

	return nil
}
