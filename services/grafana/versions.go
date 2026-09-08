package grafana

import (
	"slices"
	"strconv"
	"strings"
)

// defaultGrafanaVersion returns the latest version this emulator supports,
// used by CreateWorkspace when GrafanaVersion is omitted (matching
// CreateWorkspaceInput.GrafanaVersion's doc comment: "If not specified,
// defaults to the latest version").
func defaultGrafanaVersion() string {
	return grafanaVersions[len(grafanaVersions)-1]
}

// isValidGrafanaVersion reports whether v is one of the versions this
// emulator supports creating a workspace with.
func isValidGrafanaVersion(v string) bool {
	return slices.Contains(grafanaVersions, v)
}

// isUpgradeVersion reports whether target is a real upgrade from current --
// i.e. it appears later in the ascending grafanaVersions list. Matches
// UpdateWorkspaceConfigurationInput.GrafanaVersion's doc comment: "Can only
// be used to upgrade ... not downgrade".
func isUpgradeVersion(current, target string) bool {
	curIdx := slices.Index(grafanaVersions, current)
	tgtIdx := slices.Index(grafanaVersions, target)

	if curIdx < 0 || tgtIdx < 0 {
		return false
	}

	return tgtIdx > curIdx
}

// upgradeVersionsFor returns the versions a workspace currently on
// current may upgrade to (every version strictly after it in the ascending
// list), matching ListVersionsInput.WorkspaceId's doc comment.
func upgradeVersionsFor(current string) []string {
	curIdx := slices.Index(grafanaVersions, current)
	if curIdx < 0 {
		return nil
	}

	return slices.Clone(grafanaVersions[curIdx+1:])
}

// minServiceAccountGrafanaMajor is the minimum Grafana major version that
// supports workspace service accounts: "You can only create service accounts
// for workspaces that are compatible with Grafana version 9 and above"
// (CreateWorkspaceServiceAccount's doc comment; CreateWorkspaceServiceAccountToken
// repeats the same rule as "Service accounts are only available for
// workspaces that are compatible with Grafana version 9 and above").
const minServiceAccountGrafanaMajor = 9

// supportsServiceAccounts reports whether a workspace on the given Grafana
// version is compatible with service accounts, per
// minServiceAccountGrafanaMajor. version is one of grafanaVersions'
// "<major>.<minor>" strings.
func supportsServiceAccounts(version string) bool {
	major, _, ok := strings.Cut(version, ".")
	if !ok {
		return false
	}

	n, err := strconv.Atoi(major)
	if err != nil {
		return false
	}

	return n >= minServiceAccountGrafanaMajor
}

// ListVersions returns the Grafana versions creatable (workspaceID == "") or,
// when workspaceID is non-empty, the versions that specific workspace may
// upgrade to.
func (b *InMemoryBackend) ListVersions(workspaceID string) ([]string, error) {
	b.mu.RLock("ListVersions")
	defer b.mu.RUnlock()

	if workspaceID == "" {
		return slices.Clone(grafanaVersions), nil
	}

	w, ok := b.workspaces.Get(workspaceID)
	if !ok {
		return nil, notFoundError("workspace", workspaceID)
	}

	return upgradeVersionsFor(w.GrafanaVersion), nil
}
