package directoryservice

import (
	"context"
)

// EnableRadius enables RADIUS for a directory.
func (b *InMemoryBackend) EnableRadius(ctx context.Context, directoryID string, settings RadiusSettingsInput) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("EnableRadius")
	defer b.mu.Unlock()

	if _, ok := b.directoryGet(region, directoryID); !ok {
		return ErrDirectoryNotFound
	}
	if _, exists := b.radiusSettingsGet(region, directoryID); exists {
		return ErrRadiusAlreadyEnabled
	}

	servers := make([]string, len(settings.RadiusServers))
	copy(servers, settings.RadiusServers)
	b.radiusSettingsPut(&storedRadiusSettings{
		region:                 region,
		DirectoryID:            directoryID,
		AuthenticationProtocol: settings.AuthenticationProtocol,
		DisplayLabel:           settings.DisplayLabel,
		RadiusServers:          servers,
		SharedSecret:           settings.SharedSecret,
		RadiusPort:             settings.RadiusPort,
		RadiusRetries:          settings.RadiusRetries,
		RadiusTimeout:          settings.RadiusTimeout,
		UseSameUsername:        settings.UseSameUsername,
	})

	return nil
}

// DisableRadius disables RADIUS for a directory.
func (b *InMemoryBackend) DisableRadius(ctx context.Context, directoryID string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DisableRadius")
	defer b.mu.Unlock()

	if _, ok := b.directoryGet(region, directoryID); !ok {
		return ErrDirectoryNotFound
	}

	b.radiusSettingsDelete(region, directoryID)

	return nil
}

// UpdateRadius updates RADIUS settings for a directory.
func (b *InMemoryBackend) UpdateRadius(ctx context.Context, directoryID string, settings RadiusSettingsInput) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateRadius")
	defer b.mu.Unlock()

	if _, ok := b.directoryGet(region, directoryID); !ok {
		return ErrDirectoryNotFound
	}

	servers := make([]string, len(settings.RadiusServers))
	copy(servers, settings.RadiusServers)
	existing, ok := b.radiusSettingsGet(region, directoryID)
	if !ok {
		existing = &storedRadiusSettings{region: region}
	}
	existing.DirectoryID = directoryID
	existing.AuthenticationProtocol = settings.AuthenticationProtocol
	existing.DisplayLabel = settings.DisplayLabel
	existing.RadiusServers = servers
	existing.SharedSecret = settings.SharedSecret
	existing.RadiusPort = settings.RadiusPort
	existing.RadiusRetries = settings.RadiusRetries
	existing.RadiusTimeout = settings.RadiusTimeout
	existing.UseSameUsername = settings.UseSameUsername
	b.radiusSettingsPut(existing)

	return nil
}
