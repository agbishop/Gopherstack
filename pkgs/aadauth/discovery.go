package aadauth

// OpenIDConfiguration is the document served from
// /{tenant}/v2.0/.well-known/openid-configuration -- azidentity/go-azure-sdk
// fetch this during AAD instance/authority discovery before requesting a
// token (AZURE.md section 10.1).
type OpenIDConfiguration struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	JWKSURI               string   `json:"jwks_uri"`
	ResponseTypesSupp     []string `json:"response_types_supported"`
	SubjectTypesSupp      []string `json:"subject_types_supported"`
	IDTokenSigningAlgs    []string `json:"id_token_signing_alg_values_supported"`
	TokenEndpointAuthMeth []string `json:"token_endpoint_auth_methods_supported"`
}

// BuildOpenIDConfiguration builds the OpenIDConfiguration document for
// tenant, rooted at baseURL (e.g. "https://host:10006").
func BuildOpenIDConfiguration(baseURL, tenant string) OpenIDConfiguration {
	return OpenIDConfiguration{
		Issuer:                baseURL + "/" + tenant + "/v2.0",
		AuthorizationEndpoint: baseURL + "/" + tenant + "/oauth2/v2.0/authorize",
		TokenEndpoint:         baseURL + "/" + tenant + "/oauth2/v2.0/token",
		JWKSURI:               baseURL + "/" + tenant + "/discovery/v2.0/keys",
		ResponseTypesSupp:     []string{"code", "id_token", "code id_token", "token"},
		SubjectTypesSupp:      []string{"pairwise"},
		IDTokenSigningAlgs:    []string{"RS256"},
		TokenEndpointAuthMeth: []string{"client_secret_post", "client_secret_basic"},
	}
}

// InstanceDiscoveryMetadata is one entry of InstanceDiscoveryResponse's
// "metadata" array, describing the aliases for a single AAD cloud instance.
type InstanceDiscoveryMetadata struct {
	PreferredNetwork string   `json:"preferred_network"`
	PreferredCache   string   `json:"preferred_cache"`
	Aliases          []string `json:"aliases"`
}

// InstanceDiscoveryResponse is the document served from
// /common/discovery/instance?api-version=1.1.
type InstanceDiscoveryResponse struct {
	TenantDiscoveryEndpoint string                      `json:"tenant_discovery_endpoint"`
	Metadata                []InstanceDiscoveryMetadata `json:"metadata"`
}

// BuildInstanceDiscoveryResponse builds the InstanceDiscoveryResponse
// document for host (e.g. "host:10006", no scheme) and tenant.
func BuildInstanceDiscoveryResponse(baseURL, tenant string) InstanceDiscoveryResponse {
	host := hostFromBaseURL(baseURL)

	return InstanceDiscoveryResponse{
		TenantDiscoveryEndpoint: baseURL + "/" + tenant + "/v2.0/.well-known/openid-configuration",
		Metadata: []InstanceDiscoveryMetadata{
			{
				PreferredNetwork: host,
				PreferredCache:   host,
				Aliases:          []string{host},
			},
		},
	}
}

// hostFromBaseURL strips a leading "https://" or "http://" from baseURL,
// returning the bare host:port.
func hostFromBaseURL(baseURL string) string {
	for _, prefix := range []string{"https://", "http://"} {
		if len(baseURL) > len(prefix) && baseURL[:len(prefix)] == prefix {
			return baseURL[len(prefix):]
		}
	}

	return baseURL
}
