package azurearm

// DefaultPort is the fixed TCP port for the dedicated ARM listener. It sits
// inside --port-range-start/--port-range-end's default range (10000-10100)
// and leaves Key Vault (10004, M11) and App Configuration (10005, M12)
// untouched -- see AZURE.md section 10.7.
const DefaultPort = 10006

// DefaultTenantID, DefaultSubscriptionID, and DefaultClientID are all-zeros
// GUIDs, matching LocalStack's own Azure emulator convention: valid GUIDs
// (azurerm parses subscription_id/tenant_id as GUIDs) that are obviously not
// real Azure identifiers. See AZURE.md section 10.5.
const DefaultTenantID = "00000000-0000-0000-0000-000000000000"

// DefaultSubscriptionID is the fixed dev subscription ID.
const DefaultSubscriptionID = "00000000-0000-0000-0000-000000000000"

// DefaultClientID is the fixed dev client (application) ID.
const DefaultClientID = "00000000-0000-0000-0000-000000000000"

// DefaultClientSecret is the fixed dev client secret accepted by the token
// endpoint when validation is off (the default).
const DefaultClientSecret = "gopherstack"

// DefaultEnvironmentName is the environment name advertised by the metadata
// document; must equal the Terraform provider's own `environment` setting.
const DefaultEnvironmentName = "gopherstack"

// DefaultLocation is the default Azure "location" value used when a request
// doesn't specify one.
const DefaultLocation = "local"

// Default advertised data-plane ports for the storage endpoints ARM's
// Microsoft.Storage RP advertises in primaryEndpoints (AZURE.md section
// 10.4). These intentionally duplicate services/azureblob.DefaultPort /
// services/azurequeue.DefaultPort / services/azuretable.DefaultPort as
// literal constants rather than importing those packages: services/azurearm
// has no other reason to depend on the data-plane packages for Storage (no
// delegation is needed, see rp_storage.go), and importing three packages
// just for one int constant each would be a needless coupling for a
// documented, stable set of port numbers.
const (
	DefaultBlobEndpointPort  = 10000
	DefaultQueueEndpointPort = 10001
	DefaultTableEndpointPort = 10002
)

// Settings holds service-level configuration for the ARM emulation backend.
// Fields are picked up by the Kong CLI parser when embedded in the root CLI
// command (see cli.go's CLI.AzureARM field), mirroring services/cosmosdb's
// Settings pattern.
type Settings struct {
	TenantID       string `json:"tenantId"       env:"AZURE_ARM_TENANT_ID"       default:"00000000-0000-0000-0000-000000000000" name:"tenant-id"       help:"Fixed dev AAD tenant ID advertised by the metadata/token endpoints."`                                             //nolint:lll // config struct tags are intentionally verbose
	SubscriptionID string `json:"subscriptionId" env:"AZURE_ARM_SUBSCRIPTION_ID" default:"00000000-0000-0000-0000-000000000000" name:"subscription-id" help:"Fixed dev Azure subscription ID."`                                                                                //nolint:lll // config struct tags are intentionally verbose
	ClientID       string `json:"clientId"       env:"AZURE_ARM_CLIENT_ID"       default:"00000000-0000-0000-0000-000000000000" name:"client-id"       help:"Fixed dev AAD application (client) ID."`                                                                          //nolint:lll // config struct tags are intentionally verbose
	ClientSecret   string `json:"clientSecret"   env:"AZURE_ARM_CLIENT_SECRET"   default:"gopherstack"                          name:"client-secret"   help:"Fixed dev client secret; any value is accepted unless --azure-arm-validate-tokens is set."`                       //nolint:lll // config struct tags are intentionally verbose
	Environment    string `json:"environment"    env:"AZURE_ARM_ENVIRONMENT"     default:"gopherstack"                          name:"environment"      help:"Environment name advertised by the metadata document; must match the Terraform provider's environment setting."` //nolint:lll // config struct tags are intentionally verbose
	Location       string `json:"location"       env:"AZURE_ARM_LOCATION"        default:"local"                                name:"location"         help:"Default Azure location value used when a request doesn't specify one."`                                          //nolint:lll // config struct tags are intentionally verbose

	// Port is the fixed TCP port for the dedicated ARM listener. See
	// handler.go's StartWorker for what happens when it's unavailable
	// (fails fast; no fallback pool).
	Port int `json:"port" env:"AZURE_ARM_PORT" default:"10006" name:"port" help:"Fixed TCP port for the dedicated ARM listener; startup fails if it's unavailable (no fallback pool)."` //nolint:lll // config struct tags are intentionally verbose

	// ValidateTokens opts into cryptographic signature+audience+expiry
	// verification of the Authorization bearer token (opt-in, off by
	// default -- mirrors --cosmosdb-validate-auth / --azure-servicebus-validate-sas).
	ValidateTokens bool `json:"validateTokens" env:"AZURE_ARM_VALIDATE_TOKENS" default:"false" name:"validate-tokens" help:"Cryptographically validate ARM bearer tokens (opt-in); by default any token, or none, is accepted."` //nolint:lll // config struct tags are intentionally verbose

	// AdvertiseBlobEndpoint/AdvertiseQueueEndpoint/AdvertiseTableEndpoint
	// override the storage-account primaryEndpoints ARM advertises (e.g. for
	// a remapped/reverse-proxied deployment). Empty (the default) means
	// derive scheme://<request Host's hostname>:<DefaultXEndpointPort> per
	// request, per AZURE.md section 10.4.
	AdvertiseBlobEndpoint  string `json:"advertiseBlobEndpoint"  env:"AZURE_ARM_ADVERTISE_BLOB_ENDPOINT"  name:"advertise-blob-endpoint"  help:"Override the blob endpoint advertised for ARM-created storage accounts. Default: derived from the request's Host and the Blob service's configured port."`   //nolint:lll // config struct tags are intentionally verbose
	AdvertiseQueueEndpoint string `json:"advertiseQueueEndpoint" env:"AZURE_ARM_ADVERTISE_QUEUE_ENDPOINT" name:"advertise-queue-endpoint" help:"Override the queue endpoint advertised for ARM-created storage accounts. Default: derived from the request's Host and the Queue service's configured port."` //nolint:lll // config struct tags are intentionally verbose
	AdvertiseTableEndpoint string `json:"advertiseTableEndpoint" env:"AZURE_ARM_ADVERTISE_TABLE_ENDPOINT" name:"advertise-table-endpoint" help:"Override the table endpoint advertised for ARM-created storage accounts. Default: derived from the request's Host and the Table service's configured port."` //nolint:lll // config struct tags are intentionally verbose
}

// DefaultSettings returns the default Settings. Used when no ConfigProvider
// is available at init time (e.g. tests constructing a Provider directly).
func DefaultSettings() Settings {
	return Settings{
		TenantID:       DefaultTenantID,
		SubscriptionID: DefaultSubscriptionID,
		ClientID:       DefaultClientID,
		ClientSecret:   DefaultClientSecret,
		Environment:    DefaultEnvironmentName,
		Location:       DefaultLocation,
		Port:           DefaultPort,
		ValidateTokens: false,
	}
}
