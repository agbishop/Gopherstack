package azurearm

// StorageAccounts is the narrow, consumer-defined seam the Microsoft.Storage
// resource provider (rp_storage.go) delegates account lifecycle events
// through, mirroring the wireCrossServiceDependencies adapter pattern
// cli_adapters.go already uses ~60 times for AWS (see AZURE.md section
// 10.6). It is declared here even though M7's Storage RP does not need it to
// serve Blob/Queue/Table traffic today (those backends are already
// account-name-agnostic -- AZURE.md section 10.4) -- so that M10's
// per-account namespacing (RegisterAccount/DeleteAccount keying each data
// plane's top-level map by account name) is additive: rp_storage.go already
// calls these hooks, they're just no-ops until M10 wires a real adapter in
// cli_adapters.go.
//
// ServiceBusEntities/CosmosResources (M8/M9's equivalents) are deliberately
// NOT declared here yet -- AZURE.md's own scope note prefers keeping this
// milestone's interface surface to what it actually uses, over speculatively
// declaring shapes for resource providers this milestone doesn't implement.
type StorageAccounts interface {
	// RegisterAccount is called when the Storage RP creates a new storage
	// account. A nil-safe no-op default (noopStorageAccounts) is used when no
	// real adapter is wired, so services/azurearm works standalone in unit
	// tests and degrades gracefully if the data plane is disabled.
	RegisterAccount(name string) error
	// DeleteAccount is called when the Storage RP deletes a storage account.
	DeleteAccount(name string) error
}

// noopStorageAccounts is the nil-safe default StorageAccounts implementation.
type noopStorageAccounts struct{}

func (noopStorageAccounts) RegisterAccount(string) error { return nil }
func (noopStorageAccounts) DeleteAccount(string) error   { return nil }

var _ StorageAccounts = noopStorageAccounts{}
