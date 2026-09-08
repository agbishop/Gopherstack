package quicksight

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// ---- Dashboards ----

func (b *InMemoryBackend) CreateDashboard(
	accountID, dashboardID, name, themeArn, versionDescription string,
	definition map[string]any,
	permissions []ResourcePermission,
	tags map[string]string,
) (*Dashboard, error) {
	if dashboardID == "" || name == "" {
		return nil, ErrValidation
	}

	b.mu.Lock("CreateDashboard")
	defer b.mu.Unlock()

	key := dashboardKey(accountID, dashboardID)
	if b.dashboards.Has(key) {
		return nil, ErrDashboardAlreadyExists
	}

	now := time.Now().UTC()
	d := &storedDashboard{
		CreatedTime:            now,
		LastUpdatedTime:        now,
		LastPublishedTime:      now,
		DashboardID:            dashboardID,
		Arn:                    arn.Build("quicksight", b.region, accountID, fmt.Sprintf("dashboard/%s", dashboardID)),
		Name:                   name,
		Status:                 statusCreationSuccessful,
		ThemeArn:               themeArn,
		VersionDescription:     versionDescription,
		VersionNumber:          1,
		PublishedVersionNumber: 1,
		Definition:             definition,
		Permissions:            clonePermissions(permissions),
	}
	b.dashboards.Put(d)

	if len(tags) > 0 {
		b.tags[d.Arn] = maps.Clone(tags)
	}

	return d.toDashboard(), nil
}

func (b *InMemoryBackend) DescribeDashboard(accountID, dashboardID string) (*Dashboard, error) {
	b.mu.RLock("DescribeDashboard")
	defer b.mu.RUnlock()

	d, ok := b.dashboards.Get(dashboardKey(accountID, dashboardID))
	if !ok {
		return nil, ErrDashboardNotFound
	}

	return d.toDashboard(), nil
}

func (b *InMemoryBackend) UpdateDashboard(
	accountID, dashboardID, name, themeArn, versionDescription string,
	definition map[string]any,
) (*Dashboard, error) {
	b.mu.Lock("UpdateDashboard")
	defer b.mu.Unlock()

	key := dashboardKey(accountID, dashboardID)
	d, ok := b.dashboards.Get(key)
	if !ok {
		return nil, ErrDashboardNotFound
	}

	if name != "" {
		d.Name = name
	}
	if definition != nil {
		d.Definition = definition
	}
	if themeArn != "" {
		d.ThemeArn = themeArn
	}
	if versionDescription != "" {
		d.VersionDescription = versionDescription
	}
	d.LastUpdatedTime = time.Now().UTC()
	d.VersionNumber++
	// UpdateDashboardOutput's field is named CreationStatus: it reports the
	// creation status of the new dashboard version this update just created.
	d.Status = statusCreationSuccessful

	return d.toDashboard(), nil
}

// DeleteDashboard deletes a dashboard, or (per api_op_DeleteDashboard.go's
// VersionNumber doc comment: "If the version number property is provided, only
// the specified version of the dashboard is deleted") just one of its versions
// when versionNumber is nonzero. This backend has no real per-version content
// history (see dashboardCurrentVersionToMap's doc comment), so a targeted
// version delete can't remove that version's Definition/ThemeArn/etc -- but it
// does record the version number as deleted (DeletedVersions) rather than
// fabricating removal or -- the bug gopherstack-86y fixed -- deleting the
// entire dashboard when the caller asked to delete only one version. A
// version must exist and not already be deleted (mirroring
// UpdateDashboardPublishedVersion's own [1, VersionNumber] check plus the
// DeletedVersions check) else ErrDashboardVersionNotFound, matching
// DeleteTemplate's real behavior of 404ing on a repeat delete of the same
// version.
func (b *InMemoryBackend) DeleteDashboard(accountID, dashboardID string, versionNumber int64) error {
	b.mu.Lock("DeleteDashboard")
	defer b.mu.Unlock()

	key := dashboardKey(accountID, dashboardID)
	d, ok := b.dashboards.Get(key)
	if !ok {
		return ErrDashboardNotFound
	}

	if versionNumber != 0 {
		if versionNumber < 1 || versionNumber > d.VersionNumber || d.DeletedVersions[versionNumber] {
			return ErrDashboardVersionNotFound
		}

		if d.DeletedVersions == nil {
			d.DeletedVersions = make(map[int64]bool)
		}
		d.DeletedVersions[versionNumber] = true

		return nil
	}

	delete(b.tags, d.Arn)
	b.dashboards.Delete(key)

	return nil
}

//nolint:dupl // list functions share structure but operate on different stored types
func (b *InMemoryBackend) ListDashboards(
	_ string,
	maxResults int32,
	nextToken string,
) ([]*Dashboard, string, error) {
	b.mu.RLock("ListDashboards")
	defer b.mu.RUnlock()

	all := b.dashboards.All()
	sort.Slice(all, func(i, j int) bool { return all[i].DashboardID < all[j].DashboardID })

	if maxResults <= 0 || maxResults > defaultMaxResults {
		maxResults = defaultMaxResults
	}

	start := 0
	if nextToken != "" {
		start = len(all)
		for i, d := range all {
			if d.DashboardID == nextToken {
				start = i

				break
			}
		}
	}

	end := start + int(maxResults)
	var next string
	if end < len(all) {
		next = all[end].DashboardID
	} else {
		end = len(all)
	}

	result := make([]*Dashboard, 0, end-start)
	for _, d := range all[start:end] {
		result = append(result, d.toDashboard())
	}

	return result, next, nil
}

func (b *InMemoryBackend) ListDashboardVersions(
	accountID, dashboardID string,
	maxResults int32,
	nextToken string,
) ([]*DashboardVersion, string, error) {
	b.mu.RLock("ListDashboardVersions")
	defer b.mu.RUnlock()

	d, ok := b.dashboards.Get(dashboardKey(accountID, dashboardID))
	if !ok {
		return nil, "", ErrDashboardNotFound
	}

	if maxResults <= 0 || maxResults > defaultMaxResults {
		maxResults = defaultMaxResults
	}

	start := 0
	if nextToken != "" {
		if off, err := decodePageToken(nextToken); err == nil {
			start = off
		}
	}

	total := int(d.VersionNumber)
	// A token issued before this dashboard's version count decreased (not
	// currently reachable, since versions are append-only, but the encoder
	// makes no such promise) can name an offset past the current end --
	// clamp instead of letting the loop below run backwards or panic.
	if start > total {
		start = total
	}

	end := start + int(maxResults)

	var next string
	if end < total {
		next = encodePageToken(end)
	} else {
		end = total
	}

	versions := make([]*DashboardVersion, 0, end-start)
	for i := start + 1; i <= end; i++ {
		if d.DeletedVersions[int64(i)] {
			continue
		}

		versions = append(versions, &DashboardVersion{
			CreatedTime:   d.CreatedTime,
			Arn:           fmt.Sprintf("%s/version/%d", d.Arn, i),
			Status:        statusCreationSuccessful,
			VersionNumber: int64(i),
		})
	}

	return versions, next, nil
}

// SearchDashboards searches dashboards by name (filter Name ==
// filterDashboardName); any other filter Name is an ownership-related filter
// that this in-memory backend doesn't track and is treated as a pass-through
// match.
//
//nolint:dupl // search functions share structure but operate on different stored types
func (b *InMemoryBackend) SearchDashboards(
	_ string,
	filters []SearchFilter,
	maxResults int32,
	nextToken string,
) ([]*Dashboard, string, error) {
	b.mu.RLock("SearchDashboards")
	defer b.mu.RUnlock()

	var filtered []*storedDashboard
	for _, d := range b.dashboards.All() {
		if matchesAllNameFilters(d.Name, filters, filterDashboardName) {
			filtered = append(filtered, d)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].DashboardID < filtered[j].DashboardID })

	if maxResults <= 0 || maxResults > defaultMaxResults {
		maxResults = defaultMaxResults
	}

	start := 0
	if nextToken != "" {
		for i, d := range filtered {
			if d.DashboardID == nextToken {
				start = i

				break
			}
		}
	}

	end := start + int(maxResults)
	var next string
	if end < len(filtered) {
		next = filtered[end].DashboardID
	} else {
		end = len(filtered)
	}

	result := make([]*Dashboard, 0, end-start)
	for _, d := range filtered[start:end] {
		result = append(result, d.toDashboard())
	}

	return result, next, nil
}

// UpdateDashboardPublishedVersion flips which stored version of a dashboard is
// the published one. versionNumber must name a version that actually exists
// and hasn't been deleted (i.e. be in [1, VersionNumber] and absent from
// DeletedVersions, matching the versions ListDashboardVersions synthesizes),
// else ErrDashboardVersionNotFound.
func (b *InMemoryBackend) UpdateDashboardPublishedVersion(
	accountID, dashboardID string,
	versionNumber int64,
) (*Dashboard, error) {
	b.mu.Lock("UpdateDashboardPublishedVersion")
	defer b.mu.Unlock()

	d, ok := b.dashboards.Get(dashboardKey(accountID, dashboardID))
	if !ok {
		return nil, ErrDashboardNotFound
	}

	if versionNumber < 1 || versionNumber > d.VersionNumber || d.DeletedVersions[versionNumber] {
		return nil, ErrDashboardVersionNotFound
	}

	d.PublishedVersionNumber = versionNumber
	now := time.Now().UTC()
	d.LastUpdatedTime = now
	d.LastPublishedTime = now

	return d.toDashboard(), nil
}

// UpdateDashboardLinks replaces the set of analysis ARNs linked to a dashboard.
func (b *InMemoryBackend) UpdateDashboardLinks(
	accountID, dashboardID string,
	linkEntities []string,
) (*Dashboard, error) {
	b.mu.Lock("UpdateDashboardLinks")
	defer b.mu.Unlock()

	d, ok := b.dashboards.Get(dashboardKey(accountID, dashboardID))
	if !ok {
		return nil, ErrDashboardNotFound
	}

	d.LinkEntities = linkEntities
	d.LastUpdatedTime = time.Now().UTC()

	return d.toDashboard(), nil
}

// ---- Dashboard permissions ----

func (b *InMemoryBackend) DescribeDashboardPermissions(
	accountID, dashboardID string,
) (*Dashboard, []ResourcePermission, error) {
	b.mu.RLock("DescribeDashboardPermissions")
	defer b.mu.RUnlock()

	d, ok := b.dashboards.Get(dashboardKey(accountID, dashboardID))
	if !ok {
		return nil, nil, ErrDashboardNotFound
	}

	return d.toDashboard(), clonePermissions(d.Permissions), nil
}

func (b *InMemoryBackend) UpdateDashboardPermissions(
	accountID, dashboardID string,
	grant, revoke []ResourcePermission,
) (*Dashboard, []ResourcePermission, error) {
	b.mu.Lock("UpdateDashboardPermissions")
	defer b.mu.Unlock()

	d, ok := b.dashboards.Get(dashboardKey(accountID, dashboardID))
	if !ok {
		return nil, nil, ErrDashboardNotFound
	}

	d.Permissions = applyGrantRevoke(d.Permissions, grant, revoke)
	d.LastUpdatedTime = time.Now().UTC()

	return d.toDashboard(), clonePermissions(d.Permissions), nil
}
