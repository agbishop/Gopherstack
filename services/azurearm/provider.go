package azurearm

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/aadauth"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// ErrNilAppContext is returned when Init is called with a nil AppContext.
var ErrNilAppContext = errors.New("azurearm: nil app context")

// ConfigProvider is a private interface to extract ARM configuration from
// the abstract AppContext Config, mirroring services/cosmosdb.ConfigProvider.
type ConfigProvider interface {
	GetAzureARMSettings() Settings
}

// StorageAccountsProvider is a private interface AppContext.Config may
// implement to supply a real StorageAccounts adapter (wired by cli.go's
// wireCrossServiceDependencies in a future milestone -- see interfaces.go).
// Absent (the common case for M7), NewStorageProvider falls back to the
// nil-safe noopStorageAccounts default.
type StorageAccountsProvider interface {
	GetAzureARMStorageAccounts() StorageAccounts
}

// Provider implements service.Provider for the ARM emulation.
//
// Like services/azureblob/azurequeue/azuretable/cosmosdb/azureservicebus,
// AzureARM does not register a RouteMatcher into the shared AWS single-port
// Router -- see handler.go's RouteMatcher doc comment. It is registered in
// cli.go's getMostRecentServiceProviders like every other provider; only its
// RouteMatcher (which always returns false) is inert.
type Provider struct{}

// Name returns the service provider name.
func (p *Provider) Name() string { return "AzureARM" }

// Init initializes the ARM backend, registry (with the Storage RP
// registered), AAD token issuer, and handler. The configured port
// (Settings.Port, default DefaultPort) is only recorded here; the actual TCP
// bind happens synchronously in Handler.StartWorker, so a port-in-use
// failure is returned to the caller directly instead of being discovered
// later from a background goroutine.
//
//nolint:ireturn,nolintlint // architecturally required to return interface
func (p *Provider) Init(ctx *service.AppContext) (service.Registerable, error) {
	if ctx == nil {
		return nil, ErrNilAppContext
	}

	settings := DefaultSettings()
	if cp, ok := ctx.Config.(ConfigProvider); ok {
		settings = cp.GetAzureARMSettings()
	}

	var dataPlane StorageAccounts
	if sp, ok := ctx.Config.(StorageAccountsProvider); ok {
		dataPlane = sp.GetAzureARMStorageAccounts()
	}

	backend := NewInMemoryBackend()
	registry := NewRegistry(backend)

	storageCfg := StorageEndpointConfig{
		BlobOverride:  settings.AdvertiseBlobEndpoint,
		QueueOverride: settings.AdvertiseQueueEndpoint,
		TableOverride: settings.AdvertiseTableEndpoint,
	}
	registry.Register(NewStorageProvider(storageCfg, dataPlane))

	issuer, err := aadauth.NewIssuer()
	if err != nil {
		return nil, err
	}

	handler := NewHandler(backend, registry, issuer, settings)

	return handler, nil
}
