package azurearm

// metadataAPIVersion is the ONLY api-version the metadata endpoint is served
// under. Real ARM's metadata endpoint is versioned 2022-09-01 specifically
// (confirmed against go-azure-sdk's metadata client) -- not "1.0", which most
// public docs/Azure Stack examples show and which the real client does not
// request. See AZURE.md section 10.1/10.8.
const metadataAPIVersion = "2022-09-01"

// EnvironmentDescriptor is one entry of the GET /metadata/endpoints response
// array. Field set and semantics verified against
// hashicorp/go-azure-sdk's environments.FromEndpoint, which hard-fails on a
// document missing Name, ResourceManagerEndpoint, or
// ResourceIdentifiers.MicrosoftGraphResourceID (AZURE.md section 10.8) --
// every one of those three, plus every other field FromEndpoint reads, is
// populated below.
type EnvironmentDescriptor struct {
	Name                    string                 `json:"name"`
	Portal                  string                 `json:"portal"`
	Authentication          EnvironmentAuth        `json:"authentication"`
	Media                   string                 `json:"media,omitempty"`
	Graph                   string                 `json:"graph"`
	GraphAudience           string                 `json:"graphAudience"`
	Gallery                 string                 `json:"gallery"`
	ResourceManager         string                 `json:"resourceManager"`
	ResourceManagerEndpoint string                 `json:"resourceManagerEndpoint"`
	ActiveDirectoryDataLake string                 `json:"activeDirectoryDataLake,omitempty"`
	SQLManagement           string                 `json:"sqlManagement,omitempty"`
	Batch                   string                 `json:"batch,omitempty"`
	Suffixes                EnvironmentSuffixes    `json:"suffixes"`
	ResourceIdentifiers     EnvironmentResourceIDs `json:"resourceIdentifiers"`
}

// EnvironmentAuth is EnvironmentDescriptor's "authentication" field.
type EnvironmentAuth struct {
	LoginEndpoint string   `json:"loginEndpoint"`
	Audiences     []string `json:"audiences"`
	Tenant        string   `json:"tenant"`
}

// EnvironmentSuffixes is EnvironmentDescriptor's "suffixes" field.
type EnvironmentSuffixes struct {
	Storage            string `json:"storage"`
	KeyVaultDNS        string `json:"keyVaultDns"`
	SQLServerHostname  string `json:"sqlServerHostname"`
	ACRLoginServer     string `json:"acrLoginServer"`
	AzureDatalakeStore string `json:"azureDataLakeStoreFileSystem,omitempty"`
}

// EnvironmentResourceIDs is EnvironmentDescriptor's "resourceIdentifiers"
// field -- FromEndpoint hard-fails without MicrosoftGraphResourceID.
type EnvironmentResourceIDs struct {
	MicrosoftGraphResourceID string `json:"microsoftGraphResourceId"`
}

// BuildMetadataEndpoints builds the GET /metadata/endpoints response body:
// an array with a single environment descriptor for settings.Environment,
// with every URL pointing back at baseURL (the ARM listener's own
// scheme://host:port, e.g. "https://host:10006").
func BuildMetadataEndpoints(baseURL string, settings Settings) []EnvironmentDescriptor {
	return []EnvironmentDescriptor{
		{
			Name:   settings.Environment,
			Portal: baseURL + "/portal",
			Authentication: EnvironmentAuth{
				LoginEndpoint: baseURL + "/",
				Audiences:     []string{baseURL + "/", "https://management.core.windows.net/"},
				Tenant:        settings.TenantID,
			},
			Graph:                   baseURL + "/graph",
			GraphAudience:           baseURL + "/graph",
			Gallery:                 baseURL + "/gallery",
			ResourceManager:         baseURL + "/",
			ResourceManagerEndpoint: baseURL + "/",
			Suffixes: EnvironmentSuffixes{
				Storage:           "localhost",
				KeyVaultDNS:       ".vault." + hostnameOnly(baseURL),
				SQLServerHostname: ".database." + hostnameOnly(baseURL),
				ACRLoginServer:    ".azurecr." + hostnameOnly(baseURL),
			},
			ResourceIdentifiers: EnvironmentResourceIDs{
				MicrosoftGraphResourceID: baseURL + "/graph",
			},
		},
	}
}

// hostnameOnly strips scheme and port from baseURL, returning just the
// hostname, for building plausible-looking DNS suffixes.
func hostnameOnly(baseURL string) string {
	s := baseURL

	for _, prefix := range []string{"https://", "http://"} {
		if len(s) > len(prefix) && s[:len(prefix)] == prefix {
			s = s[len(prefix):]

			break
		}
	}

	for i := range s {
		if s[i] == ':' || s[i] == '/' {
			return s[:i]
		}
	}

	return s
}
