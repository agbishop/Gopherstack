package directoryservice

import (
	"context"
	"sort"
	"time"
)

// EnableClientAuthentication enables client authentication.
func (b *InMemoryBackend) EnableClientAuthentication(ctx context.Context, directoryID, authType string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("EnableClientAuthentication")
	defer b.mu.Unlock()

	if _, ok := b.directoryGet(region, directoryID); !ok {
		return ErrDirectoryNotFoundDDNE
	}

	now := time.Now().UTC()
	if existing, ok := b.clientAuthSettingGet(region, directoryID, authType); ok {
		if existing.Status == "Enabled" { //nolint:goconst // existing issue.
			return ErrInvalidClientAuthStatus
		}
		existing.Status = "Enabled"
		existing.LastUpdatedDateTime = now
	} else {
		b.clientAuthSettingPut(&storedClientAuthSetting{
			region:              region,
			DirectoryID:         directoryID,
			AuthType:            authType,
			Status:              "Enabled",
			LastUpdatedDateTime: now,
		})
	}

	return nil
}

// DisableClientAuthentication disables client authentication.
func (b *InMemoryBackend) DisableClientAuthentication(ctx context.Context, directoryID, authType string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DisableClientAuthentication")
	defer b.mu.Unlock()

	if _, ok := b.directoryGet(region, directoryID); !ok {
		return ErrDirectoryNotFoundDDNE
	}

	now := time.Now().UTC()
	existing, ok := b.clientAuthSettingGet(region, directoryID, authType)
	if !ok || existing.Status == "Disabled" { //nolint:goconst // existing issue.
		return ErrInvalidClientAuthStatus
	}

	existing.Status = "Disabled"
	existing.LastUpdatedDateTime = now

	return nil
}

// DescribeClientAuthenticationSettings returns client auth settings.
func (b *InMemoryBackend) DescribeClientAuthenticationSettings(
	ctx context.Context,
	directoryID, authType string,
	limit int32,
	nextToken string,
) ([]ClientAuthInfo, string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeClientAuthenticationSettings")
	defer b.mu.RUnlock()

	if _, ok := b.directoryGet(region, directoryID); !ok {
		return nil, "", ErrDirectoryNotFoundDDNE
	}

	var all []ClientAuthInfo
	for _, s := range b.clientAuthSettingsInRegion(region) {
		if s.DirectoryID != directoryID {
			continue
		}
		if authType != "" && s.AuthType != authType {
			continue
		}
		all = append(all, ClientAuthInfo{
			DirectoryID:         s.DirectoryID,
			AuthType:            s.AuthType,
			Status:              s.Status,
			LastUpdatedDateTime: s.LastUpdatedDateTime,
		})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].AuthType < all[j].AuthType })

	start := 0
	if nextToken != "" {
		for i, s := range all {
			if s.AuthType == nextToken {
				start = i

				break
			}
		}
	}

	pageSize := int(limit)
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 1000
	}

	end := min(start+pageSize, len(all))
	result := all[start:end]

	var outToken string
	if end < len(all) {
		outToken = all[end].AuthType
	}

	return result, outToken, nil
}
