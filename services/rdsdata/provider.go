package rdsdata

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// ErrNilAppContext is returned by Init when a nil AppContext is passed.
var ErrNilAppContext = errors.New("nil AppContext passed to RDSData Provider.Init")

// Provider implements service.Provider for RDS Data.
type Provider struct{}

// Name returns the provider name.
func (p *Provider) Name() string { return "RDSData" }

// Init initializes the RDS Data service backend and handler.
//
//nolint:ireturn,nolintlint // architecturally required to return interface
func (p *Provider) Init(ctx *service.AppContext) (service.Registerable, error) {
	if ctx == nil {
		return nil, ErrNilAppContext
	}

	accountID, region := service.AccountRegionOrDefault(ctx)

	backend := NewInMemoryBackend(accountID, region)
	handler := NewHandler(backend)
	handler.WithJanitor(backend, 0, 0, 0, ctx.JanitorTimeout)

	return handler, nil
}
