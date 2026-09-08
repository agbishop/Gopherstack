package mediatailor

import (
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

type storedPlaybackConfiguration struct {
	Tags                        map[string]string                      `json:"tags"`
	LogConfiguration            *PlaybackConfigurationLogConfiguration `json:"logConfiguration,omitempty"`
	Extra                       map[string]any                         `json:"extra,omitempty"`
	Name                        string                                 `json:"name"`
	AdDecisionServerURL         string                                 `json:"adDecisionServerUrl"`
	VideoContentSourceURL       string                                 `json:"videoContentSourceUrl"`
	PlaybackConfigurationARN    string                                 `json:"playbackConfigurationArn"`
	PlaybackEndpointPrefix      string                                 `json:"playbackEndpointPrefix"`
	SessionInitializationPrefix string                                 `json:"sessionInitializationPrefix"`
	HlsManifestEndpointPrefix   string                                 `json:"hlsManifestEndpointPrefix"`
}

func (s *storedPlaybackConfiguration) toPlaybackConfiguration() *PlaybackConfiguration {
	tags := make(map[string]string, len(s.Tags))
	maps.Copy(tags, s.Tags)

	return &PlaybackConfiguration{
		Tags:                        tags,
		LogConfiguration:            s.LogConfiguration,
		Extra:                       s.Extra,
		Name:                        s.Name,
		AdDecisionServerURL:         s.AdDecisionServerURL,
		VideoContentSourceURL:       s.VideoContentSourceURL,
		PlaybackConfigurationARN:    s.PlaybackConfigurationARN,
		PlaybackEndpointPrefix:      s.PlaybackEndpointPrefix,
		SessionInitializationPrefix: s.SessionInitializationPrefix,
		HlsManifestEndpointPrefix:   s.HlsManifestEndpointPrefix,
	}
}

func (s *storedPlaybackConfiguration) toSummary() *PlaybackConfigurationSummary {
	tags := make(map[string]string, len(s.Tags))
	maps.Copy(tags, s.Tags)

	return &PlaybackConfigurationSummary{
		Tags:                        tags,
		LogConfiguration:            s.LogConfiguration,
		Extra:                       s.Extra,
		Name:                        s.Name,
		AdDecisionServerURL:         s.AdDecisionServerURL,
		VideoContentSourceURL:       s.VideoContentSourceURL,
		PlaybackConfigurationARN:    s.PlaybackConfigurationARN,
		PlaybackEndpointPrefix:      s.PlaybackEndpointPrefix,
		SessionInitializationPrefix: s.SessionInitializationPrefix,
		HlsManifestEndpointPrefix:   s.HlsManifestEndpointPrefix,
	}
}

// --- PlaybackConfiguration operations ---

// PutPlaybackConfiguration creates or updates a playback configuration.
func (b *InMemoryBackend) PutPlaybackConfiguration(
	name, adDecisionServerURL, videoContentSourceURL string,
	tags map[string]string,
	extra map[string]any,
) (*PlaybackConfiguration, error) {
	// Name is the only required member of PutPlaybackConfigurationInput —
	// confirmed against aws-sdk-go-v2's validators.go and botocore's
	// service-2.json ("required": ["Name"]). AdDecisionServerUrl and
	// VideoContentSourceUrl are both optional on the wire; rejecting a
	// request that omits them is a false BadRequestException a real SDK
	// client would never trigger.
	if name == "" {
		return nil, fmt.Errorf("%w: Name required", ErrInvalidParameter)
	}

	cfgARN := b.playbackConfigARN(name)

	b.mu.Lock("PutPlaybackConfiguration")
	defer b.mu.Unlock()

	// A prior ConfigureLogsForPlaybackConfiguration call survives a re-Put of
	// the same configuration, matching real MediaTailor (logging config is
	// managed by its own operation, not reset by PutPlaybackConfiguration).
	var logCfg *PlaybackConfigurationLogConfiguration
	if existing, ok := b.playbackConfigurations.Get(name); ok {
		logCfg = existing.LogConfiguration
	}

	cfg := &storedPlaybackConfiguration{
		Tags:                     copyTags(tags),
		LogConfiguration:         logCfg,
		Extra:                    extra,
		Name:                     name,
		AdDecisionServerURL:      adDecisionServerURL,
		VideoContentSourceURL:    videoContentSourceURL,
		PlaybackConfigurationARN: cfgARN,
		PlaybackEndpointPrefix: fmt.Sprintf(
			"https://%s.mediatailor.%s.amazonaws.com/v1/master/%s/%s",
			b.accountID,
			b.region,
			b.accountID,
			name,
		),
		SessionInitializationPrefix: fmt.Sprintf(
			"https://%s.mediatailor.%s.amazonaws.com/v1/session/%s/%s",
			b.accountID,
			b.region,
			b.accountID,
			name,
		),
		HlsManifestEndpointPrefix: fmt.Sprintf(
			"https://%s.mediatailor.%s.amazonaws.com/v1/master/%s/%s/",
			b.accountID,
			b.region,
			b.accountID,
			name,
		),
	}

	b.playbackConfigurations.Put(cfg)
	b.tags[cfgARN] = copyTags(tags)

	return cfg.toPlaybackConfiguration(), nil
}

// GetPlaybackConfiguration returns a playback configuration by name.
func (b *InMemoryBackend) GetPlaybackConfiguration(name string) (*PlaybackConfiguration, error) {
	b.mu.RLock("GetPlaybackConfiguration")
	defer b.mu.RUnlock()

	cfg, ok := b.playbackConfigurations.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: playback configuration %s not found", ErrNotFound, name)
	}

	result := cfg.toPlaybackConfiguration()
	result.Tags = make(map[string]string)
	maps.Copy(result.Tags, b.tags[cfg.PlaybackConfigurationARN])

	return result, nil
}

// DeletePlaybackConfiguration deletes a playback configuration, cascading to
// every prefetch schedule attached to it so no ghost rows remain in the
// prefetchSchedules table (or its byPlaybackConfig index) afterward.
// Idempotent: returns nil if the configuration does not exist.
func (b *InMemoryBackend) DeletePlaybackConfiguration(name string) error {
	b.mu.Lock("DeletePlaybackConfiguration")
	defer b.mu.Unlock()

	cfg, ok := b.playbackConfigurations.Get(name)
	if !ok {
		return nil
	}

	for _, ps := range slices.Clone(b.prefetchSchedulesByConfig.Get(name)) {
		b.prefetchSchedules.Delete(prefetchScheduleKey(ps.PlaybackConfigurationName, ps.Name))
	}

	delete(b.tags, cfg.PlaybackConfigurationARN)
	b.playbackConfigurations.Delete(name)

	return nil
}

// ListPlaybackConfigurations returns a paginated list of playback configurations.
func (b *InMemoryBackend) ListPlaybackConfigurations(
	maxResults int,
	nextToken string,
) ([]*PlaybackConfigurationSummary, string, error) {
	b.mu.RLock("ListPlaybackConfigurations")
	defer b.mu.RUnlock()

	all := b.playbackConfigurations.All()

	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	pg := page.New(all, nextToken, maxResults, defaultMaxResults)

	summaries := make([]*PlaybackConfigurationSummary, 0, len(pg.Data))
	for _, cfg := range pg.Data {
		s := cfg.toSummary()
		s.Tags = make(map[string]string)
		maps.Copy(s.Tags, b.tags[cfg.PlaybackConfigurationARN])
		summaries = append(summaries, s)
	}

	return summaries, pg.Next, nil
}

// ConfigureLogsForPlaybackConfiguration sets the logging configuration for a
// playback configuration and persists it so it is queryable back from
// Get/List/PutPlaybackConfiguration's LogConfiguration.
func (b *InMemoryBackend) ConfigureLogsForPlaybackConfiguration(
	playbackConfigName string,
	percentEnabled int,
	enabledLoggingStrategies []string,
	adsInteractionLog, manifestServiceInteractionLog map[string]any,
) (*PlaybackConfigurationLogConfiguration, error) {
	// "Valid values: 0 - 100" -- aws-sdk-go-v2/service/mediatailor@v1.63.4
	// api_op_ConfigureLogsForPlaybackConfiguration.go:39.
	if percentEnabled < 0 || percentEnabled > 100 {
		return nil, fmt.Errorf("%w: PercentEnabled must be between 0 and 100", ErrInvalidParameter)
	}

	for _, s := range enabledLoggingStrategies {
		switch s {
		case loggingStrategyVendedLogs, loggingStrategyLegacyCloudwatch:
		default:
			return nil, fmt.Errorf(
				"%w: EnabledLoggingStrategies must be %s or %s",
				ErrInvalidParameter, loggingStrategyVendedLogs, loggingStrategyLegacyCloudwatch,
			)
		}
	}

	b.mu.Lock("ConfigureLogsForPlaybackConfiguration")
	defer b.mu.Unlock()

	cfg, ok := b.playbackConfigurations.Get(playbackConfigName)
	if !ok {
		return nil, fmt.Errorf("%w: playback configuration %s not found", ErrNotFound, playbackConfigName)
	}

	strategies := make([]string, len(enabledLoggingStrategies))
	copy(strategies, enabledLoggingStrategies)

	logCfg := &PlaybackConfigurationLogConfiguration{
		PercentEnabled:                percentEnabled,
		EnabledLoggingStrategies:      strategies,
		AdsInteractionLog:             adsInteractionLog,
		ManifestServiceInteractionLog: manifestServiceInteractionLog,
	}
	cfg.LogConfiguration = logCfg

	return logCfg, nil
}
