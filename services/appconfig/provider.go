package appconfig

import (
	"github.com/blackbirdworks/gopherstack/pkgs/config"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// Provider implements service.Provider for the AppConfig service.
type Provider struct{}

// Name returns the service provider name.
func (p *Provider) Name() string { return "AppConfig" }

// Init initialises the AppConfig backend and handler.
//
//nolint:ireturn,nolintlint // architecturally required to return interface
func (p *Provider) Init(ctx *service.AppContext) (service.Registerable, error) {
	accountID := config.DefaultAccountID
	region := config.DefaultRegion

	if ctx != nil {
		if cp, ok := ctx.Config.(config.Provider); ok {
			cfg := cp.GetGlobalConfig()
			accountID = cfg.GetAccountID()
			region = cfg.GetRegion()
		}
	}

	backend := NewInMemoryBackend(accountID, region)
	if ctx != nil {
		backend.SetAppConfig(ctx.Config)
	}
	handler := NewHandler(backend)

	return handler, nil
}
