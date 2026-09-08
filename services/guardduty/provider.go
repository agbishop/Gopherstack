package guardduty

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// ErrNilAppContext is returned when Init is called with a nil AppContext.
var ErrNilAppContext = errors.New("guardduty: nil app context")

// Provider implements service.Provider for AWS GuardDuty.
type Provider struct{}

// Name returns the provider name.
func (p *Provider) Name() string { return "GuardDuty" }

// Init initializes the GuardDuty service backend and handler.
//
//nolint:ireturn,nolintlint // architecturally required to return interface
func (p *Provider) Init(ctx *service.AppContext) (service.Registerable, error) {
	if ctx == nil {
		return nil, ErrNilAppContext
	}

	accountID, region := service.AccountRegionOrDefault(ctx)

	backend := NewInMemoryBackend(accountID, region)
	backend.SetAppConfig(ctx.Config)
	handler := NewHandler(backend)

	return handler, nil
}
