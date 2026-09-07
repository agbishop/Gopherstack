package azureservicebus

// DefaultPort is Azure Service Bus's fixed, gopherstack-chosen TCP port. This
// follows the same pattern as services/azureblob's DefaultPort (10000),
// services/azurequeue's DefaultPort (10001), and services/azuretable's
// DefaultPort (10002): pick one default and try to bind exactly that, rather
// than drawing from cli.go's shared --port-range-start/--port-range-end
// PortAlloc pool. Unlike those three, there is no Azurite Service Bus
// emulator to mirror -- Azurite doesn't implement Service Bus -- so 10003 is
// simply the next available slot after Table's 10002 in gopherstack's own
// numbering convention. See AZURE.md section 9 (M5) for the full rationale.
const DefaultPort = 10003

// Settings holds service-level configuration for the Azure Service Bus
// backend. Fields are picked up by the Kong CLI parser when this struct is
// embedded in the root CLI command (see cli.go's CLI.AzureServiceBus field),
// mirroring services/azurequeue's and services/azuretable's Settings pattern.
type Settings struct {
	// Port is the fixed TCP port for the dedicated Service Bus listener. See
	// handler.go's StartWorker for what happens when it's unavailable (fails
	// fast; no fallback pool, matching services/azureblob/azurequeue/azuretable).
	Port int `json:"port" env:"AZURE_SERVICEBUS_PORT" default:"10003" name:"port" help:"Fixed TCP port for the dedicated Azure Service Bus listener; startup fails if it's unavailable (no fallback pool)."` //nolint:lll // config struct tags are intentionally verbose
	// ValidateSAS opts the handler into cryptographic verification of SAS
	// (SharedAccessSignature) tokens against DevKeyValue (or a caller-supplied
	// key via WithSASValidation), mirroring services/s3's
	// --validate-sigv4/PresignSecret opt-in pattern and Blob/Queue/Table's
	// WithSharedKeyValidation. Off by default: SAS tokens are always
	// structurally parsed (key name + resource scope extracted) but not
	// cryptographically checked unless this is set.
	ValidateSAS bool `json:"validateSAS" env:"AZURE_SERVICEBUS_VALIDATE_SAS" default:"false" name:"validate-sas" help:"Cryptographically validate Service Bus SAS token signatures (opt-in). Signed requests whose signature does not match the configured key are rejected."` //nolint:lll // config struct tags are intentionally verbose
}

// DefaultSettings returns the default Settings. Used when no ConfigProvider
// is available at init time (e.g. tests constructing a Provider directly).
func DefaultSettings() Settings {
	return Settings{Port: DefaultPort, ValidateSAS: false}
}
