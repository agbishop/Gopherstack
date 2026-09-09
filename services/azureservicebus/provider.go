package azureservicebus

import (
	"errors"

	"github.com/blackbirdworks/gopherstack/pkgs/service"
)

// ErrNilAppContext is returned when Init is called with a nil AppContext.
var ErrNilAppContext = errors.New("azureservicebus: nil app context")

// ConfigProvider is a private interface to extract AzureServiceBus
// configuration from the abstract AppContext Config, mirroring
// services/azurequeue.ConfigProvider.
type ConfigProvider interface {
	GetAzureServiceBusSettings() Settings
}

// Provider implements service.Provider for the Azure Service Bus service.
//
// Like services/azureblob/azurequeue/azuretable, AzureServiceBus does not
// register a RouteMatcher into the shared AWS single-port Router: its path
// shape (/<queue-or-topic>[/subscriptions/<name>][/messages[/...]]) has no
// service-identifying header the way AWS's X-Amz-Target does, and multiplexing
// it onto a shared port risks exactly the collision the router avoids by
// construction for AWS services (see AZURE.md section 4). Instead the
// returned Handler implements service.BackgroundWorker and stands up its own
// dedicated *echo.Echo/*http.Server, listening on a fixed port (DefaultPort,
// 10003). It is registered in cli.go's getMostRecentServiceProviders like
// every other provider; only its RouteMatcher (which always returns false)
// is inert.
type Provider struct{}

// Name returns the service provider name.
func (p *Provider) Name() string { return "AzureServiceBus" }

// Init initializes the AzureServiceBus service backend and handler. The
// configured port (Settings.Port, default DefaultPort) is only recorded
// here; the actual TCP bind happens synchronously in Handler.StartWorker, so
// a port-in-use failure is returned to the caller directly instead of being
// discovered later from a background goroutine.
//
//nolint:ireturn,nolintlint // architecturally required to return interface
func (p *Provider) Init(ctx *service.AppContext) (service.Registerable, error) {
	if ctx == nil {
		return nil, ErrNilAppContext
	}

	settings := DefaultSettings()
	if cp, ok := ctx.Config.(ConfigProvider); ok {
		settings = cp.GetAzureServiceBusSettings()
	}

	backend := NewInMemoryBackend()
	handler := NewHandler(backend).WithJanitor(0)
	handler.Port = settings.Port

	if settings.ValidateSAS {
		handler.WithSASValidation(DefaultKeyValue)
	}

	return handler, nil
}
