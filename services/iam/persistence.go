package iam

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// iamSnapshotVersion identifies the shape of backendSnapshot's Tables blob
// (i.e. the set of resources registered on b.registry -- see
// registerAllTables in store_setup.go). It must be bumped whenever a change
// there would make an older snapshot unsafe to decode as the current shape.
// Restore compares this against the persisted value and discards (rather
// than attempts to partially decode) any mismatch -- see Restore below. This
// mirrors the services/ec2 (commit 12e611a4) and services/sqs (commit
// 0f09d77c) conversions.
const iamSnapshotVersion = 1

type backendSnapshot struct {
	GroupInlinePolicies        map[string]map[string]string     `json:"groupInlinePolicies,omitempty"`
	GroupPolicies              map[string][]string              `json:"groupPolicies,omitempty"`
	PasswordPolicy             *PasswordPolicy                  `json:"passwordPolicy,omitempty"`
	GroupMembers               map[string][]string              `json:"groupMembers,omitempty"`
	UserPolicies               map[string][]string              `json:"userPolicies,omitempty"`
	UserInlinePolicies         map[string]map[string]string     `json:"userInlinePolicies,omitempty"`
	RoleInlinePolicies         map[string]map[string]string     `json:"roleInlinePolicies,omitempty"`
	PolicyVersionCounters      map[string]int                   `json:"policyVersionCounters,omitempty"`
	RolePolicies               map[string][]string              `json:"rolePolicies,omitempty"`
	PolicyVersions             map[string][]StoredPolicyVersion `json:"policyVersions,omitempty"`
	RoleByARN                  map[string]string                `json:"roleByARN,omitempty"`
	PolicyByARN                map[string]string                `json:"policyByARN,omitempty"`
	Tables                     map[string]json.RawMessage       `json:"tables"`
	PolicyAttachments          map[string]policyAttachmentRefs  `json:"policyAttachments,omitempty"`
	DeletedV1Policies          map[string]bool                  `json:"deletedV1Policies,omitempty"`
	Comprehensive              *comprehensiveSnapshot           `json:"comprehensive,omitempty"`
	OutboundFederationEnabled  *bool                            `json:"outboundFederationEnabled,omitempty"`
	AccountID                  string                           `json:"accountID,omitempty"`
	CurrentPassword            string                           `json:"currentPassword,omitempty"`
	GlobalEndpointTokenVersion string                           `json:"globalEndpointTokenVersion,omitempty"`
	AccountAliases             []string                         `json:"accountAliases,omitempty"`
	CurrentPasswordHistory     []string                         `json:"currentPasswordHistory,omitempty"`
	Version                    int                              `json:"version"`
}

// Snapshot serialises the backend state to JSON.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	// comprehensiveBackend's fields are guarded by this same b.mu (see its doc
	// comment in store.go), so reading it here gives Snapshot() one consistent
	// point-in-time view across all backend state, not just the registry tables.
	comp := b.comp().snapshot()

	outboundFederationEnabled := b.outboundFederationEnabled

	tables, err := b.registry.SnapshotAll()
	if err != nil {
		// The registered tables are plain JSON-friendly structs, so a marshal
		// failure here would indicate a programming error rather than bad
		// input data. Log and skip the snapshot rather than panic, matching
		// the persistence.Persistable contract (nil is skipped by the Manager).
		logger.Load(ctx).WarnContext(ctx, "iam: snapshot table marshal failed", "error", err)

		return nil
	}

	snap := backendSnapshot{
		Version:                    iamSnapshotVersion,
		Tables:                     tables,
		Comprehensive:              &comp,
		UserPolicies:               b.userPolicies,
		RolePolicies:               b.rolePolicies,
		GroupPolicies:              b.groupPolicies,
		GroupMembers:               b.groupMembers,
		UserInlinePolicies:         b.userInlinePolicies,
		RoleInlinePolicies:         b.roleInlinePolicies,
		GroupInlinePolicies:        b.groupInlinePolicies,
		AccountAliases:             b.accountAliases,
		PolicyVersions:             b.policyVersions,
		PolicyVersionCounters:      b.policyVersionCounters,
		AccountID:                  b.accountID,
		PolicyByARN:                b.policyByARN,
		RoleByARN:                  b.roleByARN,
		PolicyAttachments:          b.policyAttachments,
		DeletedV1Policies:          b.deletedV1Policies,
		PasswordPolicy:             b.passwordPolicy,
		CurrentPassword:            b.currentPassword,
		CurrentPasswordHistory:     b.currentPasswordHistory,
		GlobalEndpointTokenVersion: b.globalEndpointTokenVersion,
		OutboundFederationEnabled:  &outboundFederationEnabled,
	}

	return persistence.MarshalSnapshot(ctx, "iam", snap)
}

// restoreSnapshotLocked applies snap to backend state under b.mu. It returns
// versionMismatch=true if snap's version doesn't match iamSnapshotVersion, in
// which case the backend registry has been reset to empty and no other
// fields from snap were applied.
func (b *InMemoryBackend) restoreSnapshotLocked(ctx context.Context, snap *backendSnapshot) (bool, error) {
	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != iamSnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption. Mirrors the services/ec2 and services/sqs conversions.
		logger.Load(ctx).WarnContext(ctx,
			"iam: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", iamSnapshotVersion)

		b.registry.ResetAll()

		return true, nil
	}

	if err := b.registry.RestoreAll(snap.Tables); err != nil {
		return false, fmt.Errorf("iam: restore snapshot tables: %w", err)
	}

	// userAccessKeys is a secondary index (username -> []AccessKeyID) over
	// b.accessKeys; it is not itself persisted (see store_setup.go), so it is
	// always rebuilt fresh from the restored accessKeys table.
	b.userAccessKeys = make(map[string][]string)
	for _, ak := range b.accessKeys.All() {
		b.userAccessKeys[ak.UserName] = append(b.userAccessKeys[ak.UserName], ak.AccessKeyID)
	}

	b.userPolicies = snap.UserPolicies
	b.rolePolicies = snap.RolePolicies
	b.groupPolicies = snap.GroupPolicies
	b.groupMembers = snap.GroupMembers
	b.userInlinePolicies = snap.UserInlinePolicies
	b.roleInlinePolicies = snap.RoleInlinePolicies
	b.groupInlinePolicies = snap.GroupInlinePolicies
	b.accountAliases = snap.AccountAliases
	b.policyVersions = snap.PolicyVersions
	b.policyVersionCounters = snap.PolicyVersionCounters
	b.accountID = snap.AccountID
	b.rebuildIndexesLocked()

	if snap.PolicyByARN != nil {
		b.policyByARN = snap.PolicyByARN
	} else {
		b.policyByARN = make(map[string]string)
	}
	if snap.RoleByARN != nil {
		b.roleByARN = snap.RoleByARN
	} else {
		b.roleByARN = make(map[string]string)
	}
	if snap.PolicyAttachments != nil {
		b.policyAttachments = snap.PolicyAttachments
	} else {
		b.policyAttachments = make(map[string]policyAttachmentRefs)
	}
	if snap.DeletedV1Policies != nil {
		b.deletedV1Policies = snap.DeletedV1Policies
	} else {
		b.deletedV1Policies = make(map[string]bool)
	}
	b.passwordPolicy = snap.PasswordPolicy
	b.currentPassword = snap.CurrentPassword
	b.currentPasswordHistory = snap.CurrentPasswordHistory

	if snap.GlobalEndpointTokenVersion != "" {
		b.globalEndpointTokenVersion = snap.GlobalEndpointTokenVersion
	} else {
		// Pre-existing snapshot from before this field was added, or a
		// snapshot taken while still at the default: keep the same default
		// NewInMemoryBackendWithConfig uses, not the Go zero value.
		b.globalEndpointTokenVersion = globalEndpointTokenVersionV1
	}

	if snap.OutboundFederationEnabled != nil {
		b.outboundFederationEnabled = *snap.OutboundFederationEnabled
	} else {
		// Pre-existing snapshot from before this field was added: keep the
		// same default NewInMemoryBackendWithConfig uses (enabled), not the
		// Go zero value (false/disabled) -- see the field's doc comment in
		// backendSnapshot above.
		b.outboundFederationEnabled = true
	}

	// comprehensiveBackend's fields are guarded by this same b.mu, so restoring
	// them here keeps the whole backend's Restore atomic (see the matching note
	// in Snapshot).
	if snap.Comprehensive != nil {
		b.comp().restore(*snap.Comprehensive)
	} else {
		b.comp().restore(comprehensiveSnapshot{})
	}

	return false, nil
}

// Restore loads backend state from a JSON snapshot.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "iam", data, &snap); err != nil {
		return err
	}

	normalizeSnapshot(&snap)

	_, restoreErr := b.restoreSnapshotLocked(ctx, &snap)

	return restoreErr
}

// normalizeSnapshot ensures all map fields in snap are non-nil so callers
// can assign them directly without nil-pointer risk.
func normalizeSnapshot(snap *backendSnapshot) {
	normalizeSnapshotPolicies(snap)
	normalizeSnapshotNewOps(snap)
}

// normalizeSnapshotPolicies initialises policy maps in snap to non-nil empty maps.
func normalizeSnapshotPolicies(snap *backendSnapshot) {
	if snap.UserPolicies == nil {
		snap.UserPolicies = make(map[string][]string)
	}

	if snap.RolePolicies == nil {
		snap.RolePolicies = make(map[string][]string)
	}

	if snap.GroupPolicies == nil {
		snap.GroupPolicies = make(map[string][]string)
	}

	if snap.GroupMembers == nil {
		snap.GroupMembers = make(map[string][]string)
	}

	if snap.UserInlinePolicies == nil {
		snap.UserInlinePolicies = make(map[string]map[string]string)
	}

	if snap.RoleInlinePolicies == nil {
		snap.RoleInlinePolicies = make(map[string]map[string]string)
	}

	if snap.GroupInlinePolicies == nil {
		snap.GroupInlinePolicies = make(map[string]map[string]string)
	}
}

// normalizeSnapshotNewOps initialises new-ops maps in snap to non-nil empty values.
func normalizeSnapshotNewOps(snap *backendSnapshot) {
	if snap.PolicyVersions == nil {
		snap.PolicyVersions = make(map[string][]StoredPolicyVersion)
	}

	if snap.PolicyVersionCounters == nil {
		snap.PolicyVersionCounters = rebuildVersionCounters(snap.PolicyVersions)
	}
}

// rebuildVersionCounters derives a monotonic-counter map from stored policy versions.
// It is used when restoring snapshots that pre-date the counter field, ensuring that
// newly created versions do not collide with existing IDs.
func rebuildVersionCounters(versions map[string][]StoredPolicyVersion) map[string]int {
	counters := make(map[string]int, len(versions))

	for policyArn, pvs := range versions {
		counters[policyArn] = maxVersionNumber(pvs)
	}

	return counters
}

// maxVersionNumber returns max(1, highest vN suffix) across pvs.
// The result is stored as the counter so that counter++ yields the next unused ID.
func maxVersionNumber(pvs []StoredPolicyVersion) int {
	maxNum := 1 // v1 is always implicit

	for _, v := range pvs {
		if n := parseVersionNum(v.VersionID); n > maxNum {
			maxNum = n
		}
	}

	// counter = maxNum - 1 so counter++ produces maxNum+1 as the next version number.
	return maxNum - 1
}

// decimalBase is the numeric base used when parsing version ID suffixes.
const decimalBase = 10

// parseVersionNum extracts the integer suffix from a "vN" version ID string.
// Returns 0 if the ID does not match the "v<digits>" pattern.
func parseVersionNum(id string) int {
	if len(id) < 2 || id[0] != 'v' {
		return 0
	}

	n := 0

	for _, ch := range id[1:] {
		if ch < '0' || ch > '9' {
			return 0
		}

		n = n*decimalBase + int(ch-'0')
	}

	return n
}

// handlerSnapshot wraps the backend snapshot together with Handler-level state
// (resource tags for instance profiles, MFA devices, SAML/OIDC providers, and
// server certificates — see tagsSnapshot/restoreTags) so both round-trip
// through persistence together.
type handlerSnapshot struct {
	Tags    map[string]map[string]string `json:"tags,omitempty"`
	Backend json.RawMessage              `json:"backend,omitempty"`
}

// Snapshot implements persistence.Persistable. It combines the backend's own
// snapshot with Handler-level tag state.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	type snapshotter interface {
		Snapshot(ctx context.Context) []byte
	}

	snap := handlerSnapshot{Tags: h.tagsSnapshot()}

	if s, ok := h.Backend.(snapshotter); ok {
		if b := s.Snapshot(ctx); len(b) > 0 {
			snap.Backend = json.RawMessage(b)
		}
	}

	return persistence.MarshalSnapshot(ctx, "iam", snap)
}

// Restore implements persistence.Persistable. It accepts both the current
// wrapped format and the legacy format (where data was the raw backend
// snapshot with no Handler-level wrapper), so older persisted snapshots still
// load correctly.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	type restorer interface {
		Restore(context.Context, []byte) error
	}

	var snap handlerSnapshot
	if err := persistence.UnmarshalSnapshot(ctx, "iam", data, &snap); err != nil {
		return err
	}

	backendData := []byte(snap.Backend)
	if len(backendData) == 0 {
		// Legacy snapshot: the whole blob is the backend snapshot and has no
		// "backend"/"tags" wrapper fields (unmarshal above silently ignored
		// them since backendSnapshot's JSON shape doesn't collide with ours).
		backendData = data
	}

	if r, ok := h.Backend.(restorer); ok {
		if err := r.Restore(ctx, backendData); err != nil {
			return err
		}
	}

	h.restoreTags(snap.Tags)

	return nil
}
