package directoryservice

import (
	"context"
	"sort"
	"time"
)

// EnableLDAPS enables LDAPS for a directory.
func (b *InMemoryBackend) EnableLDAPS(ctx context.Context, directoryID, ldapsType string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("EnableLDAPS")
	defer b.mu.Unlock()

	if _, ok := b.directoryGet(region, directoryID); !ok {
		return ErrDirectoryNotFoundDDNE
	}

	now := time.Now().UTC()
	if existing, ok := b.ldapsSettingGet(region, directoryID, ldapsType); ok {
		if existing.State == "Enabled" { //nolint:goconst // existing issue.
			return ErrInvalidLDAPSStatus
		}
		existing.State = "Enabled"
		existing.LastUpdatedDateTime = now
	} else {
		b.ldapsSettingPut(&storedLDAPSSetting{
			region:                    region,
			DirectoryID:               directoryID,
			LDAPSType:                 ldapsType,
			State:                     "Enabled",
			LastUpdatedDateTime:       now,
			CertificateExpiryDateTime: now.Add(365 * 24 * time.Hour),
		})
	}

	return nil
}

// DisableLDAPS disables LDAPS for a directory.
func (b *InMemoryBackend) DisableLDAPS(ctx context.Context, directoryID, ldapsType string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DisableLDAPS")
	defer b.mu.Unlock()

	if _, ok := b.directoryGet(region, directoryID); !ok {
		return ErrDirectoryNotFoundDDNE
	}

	setting, ok := b.ldapsSettingGet(region, directoryID, ldapsType)
	if !ok || setting.State == "Disabled" { //nolint:goconst // existing issue.
		return ErrInvalidLDAPSStatus
	}

	setting.State = "Disabled"
	setting.LastUpdatedDateTime = time.Now().UTC()

	return nil
}

// DescribeLDAPSSettings returns LDAPS settings for a directory.
func (b *InMemoryBackend) DescribeLDAPSSettings(
	ctx context.Context,
	directoryID, ldapsType string,
	limit int32, //nolint:revive // existing issue.
	nextToken string, //nolint:revive // existing issue.
) ([]LDAPSSetting, string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeLDAPSSettings")
	defer b.mu.RUnlock()

	if _, ok := b.directoryGet(region, directoryID); !ok {
		return nil, "", ErrDirectoryNotFoundDDNE
	}

	var result []LDAPSSetting
	for _, s := range b.ldapsSettingsInRegion(region) {
		if s.DirectoryID != directoryID {
			continue
		}
		if ldapsType != "" && s.LDAPSType != ldapsType {
			continue
		}
		result = append(result, LDAPSSetting{
			DirectoryID:               s.DirectoryID,
			LDAPSType:                 s.LDAPSType,
			CertificateID:             s.CertificateID,
			State:                     s.State,
			LastUpdatedDateTime:       s.LastUpdatedDateTime,
			CertificateExpiryDateTime: s.CertificateExpiryDateTime,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LDAPSType < result[j].LDAPSType })

	return result, "", nil
}
