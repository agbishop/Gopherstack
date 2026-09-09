package azurearm

import (
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
)

// InMemoryBackend owns the ARM resource graph: one subscription's resource
// groups, its generic resource-provider-registration state, and the flat
// map of every resource not owned by a dedicated ResourceProvider's own
// internal state (e.g. rp_storage.go keeps its own storage-account map --
// see registry.go for how the two interact). This is ARM-side metadata
// ONLY, never a second copy of any data-plane service's state (AZURE.md
// section 10.9).
type InMemoryBackend struct {
	mu *lockmetrics.RWMutex

	// resourceGroups is keyed by strings.ToLower(name) -- resource group
	// names are matched case-insensitively (AZURE.md section 10.1).
	resourceGroups map[string]*ResourceGroup

	// resources is keyed by ResourceID.storeKey() (the lower-cased ARM ID)
	// and holds every generic (non-Storage-RP) resource.
	resources map[string]*Resource

	// registeredProviders[subscriptionID][namespace] tracks whether a
	// resource provider namespace has been explicitly registered via POST
	// .../providers/{ns}/register. Every namespace known to the registry is
	// considered "NotRegistered" until this is set, matching real ARM's
	// registrationState semantics closely enough for azurerm's
	// resource_provider_registrations="none" mode to be unnecessary (though
	// still supported, since register/list both work regardless).
	registeredProviders map[string]map[string]bool
}

// NewInMemoryBackend creates an empty InMemoryBackend.
func NewInMemoryBackend() *InMemoryBackend {
	return &InMemoryBackend{
		mu:                  lockmetrics.New("azurearm.backend"),
		resourceGroups:      make(map[string]*ResourceGroup),
		resources:           make(map[string]*Resource),
		registeredProviders: make(map[string]map[string]bool),
	}
}

// Reset clears all in-memory ARM state. Used by the POST /_gopherstack/reset
// endpoint. Never touches any data-plane service's own state (AZURE.md
// section 10.9).
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.resourceGroups = make(map[string]*ResourceGroup)
	b.resources = make(map[string]*Resource)
	b.registeredProviders = make(map[string]map[string]bool)
}

func resourceGroupKey(name string) string { return strings.ToLower(name) }
