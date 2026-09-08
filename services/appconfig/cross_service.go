package appconfig

import (
	"github.com/blackbirdworks/gopherstack/pkgs/service"

	appconfigdatabackend "github.com/blackbirdworks/gopherstack/services/appconfigdata"
)

// siblingServices is the subset of *CLI's method set this backend needs: the
// AppConfigData backend, so DeleteEnvironment/DeleteConfigurationProfile's
// DeletionProtectionCheck can ask whether the resource was actually read via
// GetLatestConfiguration recently, instead of the check always behaving like
// BYPASS (gopherstack-z4v1). Matched structurally against *CLI (no import of
// the top-level package, which would cycle) -- same pattern as
// services/grafana's cross_service.go. GetAppConfigDataHandler is the same
// accessor cli.go's wireAppConfigDeployments already calls for the
// appconfig -> appconfigdata publish bridge (bd gopherstack-uiyi).
type siblingServices interface {
	GetAppConfigDataHandler() service.Registerable
}

// SetAppConfig records the service.AppContext.Config value Provider.Init
// received, so this backend can resolve the AppConfigData backend on demand
// -- see services/grafana/cross_service.go's SetAppConfig doc comment for
// why this must be lazy rather than resolved at construction time.
func (b *InMemoryBackend) SetAppConfig(cfg any) {
	b.appConfig = cfg
}

func (b *InMemoryBackend) siblings() (siblingServices, bool) {
	s, ok := b.appConfig.(siblingServices)

	return s, ok
}

// appConfigDataBackend returns the emulator's AppConfigData backend, if wired.
func (b *InMemoryBackend) appConfigDataBackend() (appconfigdatabackend.StorageBackend, bool) {
	s, ok := b.siblings()
	if !ok {
		return nil, false
	}

	h, ok := s.GetAppConfigDataHandler().(*appconfigdatabackend.Handler)
	if !ok || h == nil || h.Backend == nil {
		return nil, false
	}

	return h.Backend, true
}
