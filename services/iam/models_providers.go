package iam

import (
	"encoding/xml"
	"time"
)

// ---- SAML Provider types ----

// SAMLProvider represents an IAM SAML identity provider.
type SAMLProvider struct {
	CreateDate           time.Time `json:"CreateDate"`
	ValidUntil           time.Time `json:"ValidUntil"`
	Arn                  string    `json:"Arn,omitempty"`
	SAMLMetadataDocument string    `json:"SAMLMetadataDocument,omitempty"`
}

// SAMLProviderListEntryXML is the XML representation of a SAML provider in list responses.
type SAMLProviderListEntryXML struct {
	Arn        string `xml:"Arn"`
	ValidUntil string `xml:"ValidUntil,omitempty"`
	CreateDate string `xml:"CreateDate"`
}

// CreateSAMLProviderResult wraps the ARN of the created SAML provider.
type CreateSAMLProviderResult struct {
	SAMLProviderArn string `xml:"SAMLProviderArn"`
}

// CreateSAMLProviderResponse is the XML response for CreateSAMLProvider.
type CreateSAMLProviderResponse struct {
	XMLName                  xml.Name                 `xml:"CreateSAMLProviderResponse"`
	Xmlns                    string                   `xml:"xmlns,attr"`
	CreateSAMLProviderResult CreateSAMLProviderResult `xml:"CreateSAMLProviderResult"`
	ResponseMetadata         ResponseMetadata         `xml:"ResponseMetadata"`
}

// UpdateSAMLProviderResult wraps the ARN of the updated SAML provider.
type UpdateSAMLProviderResult struct {
	SAMLProviderArn string `xml:"SAMLProviderArn"`
}

// UpdateSAMLProviderResponse is the XML response for UpdateSAMLProvider.
type UpdateSAMLProviderResponse struct {
	XMLName                  xml.Name                 `xml:"UpdateSAMLProviderResponse"`
	Xmlns                    string                   `xml:"xmlns,attr"`
	UpdateSAMLProviderResult UpdateSAMLProviderResult `xml:"UpdateSAMLProviderResult"`
	ResponseMetadata         ResponseMetadata         `xml:"ResponseMetadata"`
}

// DeleteSAMLProviderResponse is the XML response for DeleteSAMLProvider.
type DeleteSAMLProviderResponse struct {
	XMLName          xml.Name         `xml:"DeleteSAMLProviderResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

// GetSAMLProviderResult contains the SAML provider details.
type GetSAMLProviderResult struct {
	SAMLMetadataDocument string `xml:"SAMLMetadataDocument"`
	ValidUntil           string `xml:"ValidUntil,omitempty"`
	CreateDate           string `xml:"CreateDate"`
}

// GetSAMLProviderResponse is the XML response for GetSAMLProvider.
type GetSAMLProviderResponse struct {
	XMLName               xml.Name              `xml:"GetSAMLProviderResponse"`
	Xmlns                 string                `xml:"xmlns,attr"`
	GetSAMLProviderResult GetSAMLProviderResult `xml:"GetSAMLProviderResult"`
	ResponseMetadata      ResponseMetadata      `xml:"ResponseMetadata"`
}

// ListSAMLProvidersResult contains the list of SAML providers.
type ListSAMLProvidersResult struct {
	SAMLProviderList []SAMLProviderListEntryXML `xml:"SAMLProviderList>member"`
}

// ListSAMLProvidersResponse is the XML response for ListSAMLProviders.
type ListSAMLProvidersResponse struct {
	XMLName                 xml.Name                `xml:"ListSAMLProvidersResponse"`
	Xmlns                   string                  `xml:"xmlns,attr"`
	ResponseMetadata        ResponseMetadata        `xml:"ResponseMetadata"`
	ListSAMLProvidersResult ListSAMLProvidersResult `xml:"ListSAMLProvidersResult"`
}

// ---- OIDC Provider types ----

// OIDCProvider represents an IAM OpenID Connect identity provider.
type OIDCProvider struct {
	CreateDate     time.Time `json:"CreateDate"`
	Arn            string    `json:"Arn,omitempty"`
	URL            string    `json:"Url,omitempty"`
	ClientIDList   []string  `json:"ClientIDList,omitempty"`
	ThumbprintList []string  `json:"ThumbprintList,omitempty"`
}

// OIDCProviderListEntryXML is the XML representation of an OIDC provider in list responses.
type OIDCProviderListEntryXML struct {
	Arn string `xml:"Arn"`
}

// CreateOpenIDConnectProviderResult wraps the ARN of the created OIDC provider.
type CreateOpenIDConnectProviderResult struct {
	OpenIDConnectProviderArn string `xml:"OpenIDConnectProviderArn"`
}

// CreateOpenIDConnectProviderResponse is the XML response for CreateOpenIDConnectProvider.
type CreateOpenIDConnectProviderResponse struct {
	XMLName                           xml.Name                          `xml:"CreateOpenIDConnectProviderResponse"`
	Xmlns                             string                            `xml:"xmlns,attr"`
	CreateOpenIDConnectProviderResult CreateOpenIDConnectProviderResult `xml:"CreateOpenIDConnectProviderResult"`
	ResponseMetadata                  ResponseMetadata                  `xml:"ResponseMetadata"`
}

// DeleteOpenIDConnectProviderResponse is the XML response for DeleteOpenIDConnectProvider.
type DeleteOpenIDConnectProviderResponse struct {
	XMLName          xml.Name         `xml:"DeleteOpenIDConnectProviderResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

// GetOpenIDConnectProviderResult contains OIDC provider details.
type GetOpenIDConnectProviderResult struct {
	URL            string   `xml:"Url"`
	CreateDate     string   `xml:"CreateDate"`
	ClientIDList   []string `xml:"ClientIDList>member"`
	ThumbprintList []string `xml:"ThumbprintList>member"`
}

// GetOpenIDConnectProviderResponse is the XML response for GetOpenIDConnectProvider.
type GetOpenIDConnectProviderResponse struct {
	XMLName                        xml.Name                       `xml:"GetOpenIDConnectProviderResponse"`
	Xmlns                          string                         `xml:"xmlns,attr"`
	ResponseMetadata               ResponseMetadata               `xml:"ResponseMetadata"`
	GetOpenIDConnectProviderResult GetOpenIDConnectProviderResult `xml:"GetOpenIDConnectProviderResult"`
}

// ListOpenIDConnectProvidersResult contains the list of OIDC providers.
type ListOpenIDConnectProvidersResult struct {
	OpenIDConnectProviderList []OIDCProviderListEntryXML `xml:"OpenIDConnectProviderList>member"`
}

// ListOpenIDConnectProvidersResponse is the XML response for ListOpenIDConnectProviders.
type ListOpenIDConnectProvidersResponse struct {
	XMLName                          xml.Name                         `xml:"ListOpenIDConnectProvidersResponse"`
	Xmlns                            string                           `xml:"xmlns,attr"`
	ResponseMetadata                 ResponseMetadata                 `xml:"ResponseMetadata"`
	ListOpenIDConnectProvidersResult ListOpenIDConnectProvidersResult `xml:"ListOpenIDConnectProvidersResult"`
}

// UpdateOpenIDConnectProviderThumbprintResponse is the XML response for UpdateOpenIDConnectProviderThumbprint.
type UpdateOpenIDConnectProviderThumbprintResponse struct {
	XMLName          xml.Name         `xml:"UpdateOpenIDConnectProviderThumbprintResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

// AddClientIDToOpenIDConnectProviderResponse is the XML response for AddClientIDToOpenIDConnectProvider.
type AddClientIDToOpenIDConnectProviderResponse struct {
	XMLName          xml.Name         `xml:"AddClientIDToOpenIDConnectProviderResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

// RemoveClientIDFromOpenIDConnectProviderResponse is the XML response for RemoveClientIDFromOpenIDConnectProvider.
type RemoveClientIDFromOpenIDConnectProviderResponse struct {
	XMLName          xml.Name         `xml:"RemoveClientIDFromOpenIDConnectProviderResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

// ---- Login Profile types ----

// LoginProfile represents an IAM user login profile (console access).
type LoginProfile struct {
	CreateDate time.Time `json:"CreateDate"`
	UserName   string    `json:"UserName,omitempty"`
	Password   string    `json:"Password,omitempty"`
	// PasswordHistory holds previously-set passwords, most recent first, bounded
	// to the account password policy's PasswordReusePrevention. Index 0 is
	// always the current password.
	PasswordHistory       []string `json:"PasswordHistory,omitempty"`
	PasswordResetRequired bool     `json:"PasswordResetRequired,omitempty"`
}

// LoginProfileXML is the XML representation of a LoginProfile.
type LoginProfileXML struct {
	UserName              string `xml:"UserName"`
	CreateDate            string `xml:"CreateDate"`
	PasswordResetRequired bool   `xml:"PasswordResetRequired"`
}

// CreateLoginProfileResult wraps the created login profile.
type CreateLoginProfileResult struct {
	LoginProfile LoginProfileXML `xml:"LoginProfile"`
}

// CreateLoginProfileResponse is the XML response for CreateLoginProfile.
type CreateLoginProfileResponse struct {
	XMLName                  xml.Name                 `xml:"CreateLoginProfileResponse"`
	Xmlns                    string                   `xml:"xmlns,attr"`
	ResponseMetadata         ResponseMetadata         `xml:"ResponseMetadata"`
	CreateLoginProfileResult CreateLoginProfileResult `xml:"CreateLoginProfileResult"`
}

// UpdateLoginProfileResponse is the XML response for UpdateLoginProfile.
type UpdateLoginProfileResponse struct {
	XMLName          xml.Name         `xml:"UpdateLoginProfileResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

// DeleteLoginProfileResponse is the XML response for DeleteLoginProfile.
type DeleteLoginProfileResponse struct {
	XMLName          xml.Name         `xml:"DeleteLoginProfileResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

// GetLoginProfileResult wraps the login profile.
type GetLoginProfileResult struct {
	LoginProfile LoginProfileXML `xml:"LoginProfile"`
}

// GetLoginProfileResponse is the XML response for GetLoginProfile.
type GetLoginProfileResponse struct {
	XMLName               xml.Name              `xml:"GetLoginProfileResponse"`
	Xmlns                 string                `xml:"xmlns,attr"`
	ResponseMetadata      ResponseMetadata      `xml:"ResponseMetadata"`
	GetLoginProfileResult GetLoginProfileResult `xml:"GetLoginProfileResult"`
}

// ---- Miscellaneous types ----

// ServiceLastAccessedDetailXML is the XML representation of a single service last accessed entry.
type ServiceLastAccessedDetailXML struct {
	ServiceName                string `xml:"ServiceName"`
	ServiceNamespace           string `xml:"ServiceNamespace"`
	LastAuthenticated          string `xml:"LastAuthenticated,omitempty"`
	LastAuthenticatedArn       string `xml:"LastAuthenticatedArn,omitempty"`
	TotalAuthenticatedEntities int    `xml:"TotalAuthenticatedEntities"`
}

// GetServiceLastAccessedDetailsResult contains the job status and services list.
type GetServiceLastAccessedDetailsResult struct {
	JobStatus            string                         `xml:"JobStatus"`
	JobCreationDate      string                         `xml:"JobCreationDate"`
	JobCompletionDate    string                         `xml:"JobCompletionDate"`
	ServicesLastAccessed []ServiceLastAccessedDetailXML `xml:"ServicesLastAccessed>member"`
	IsTruncated          bool                           `xml:"IsTruncated"`
}

// GetServiceLastAccessedDetailsResponse is the XML response for GetServiceLastAccessedDetails.
type GetServiceLastAccessedDetailsResponse struct {
	XMLName                             xml.Name                            `xml:"GetServiceLastAccessedDetailsResponse"`
	Xmlns                               string                              `xml:"xmlns,attr"`
	ResponseMetadata                    ResponseMetadata                    `xml:"ResponseMetadata"`
	GetServiceLastAccessedDetailsResult GetServiceLastAccessedDetailsResult `xml:"GetServiceLastAccessedDetailsResult"`
}

// SetSecurityTokenServicePreferencesResponse is the XML response for SetSecurityTokenServicePreferences.
type SetSecurityTokenServicePreferencesResponse struct {
	XMLName          xml.Name         `xml:"SetSecurityTokenServicePreferencesResponse"`
	Xmlns            string           `xml:"xmlns,attr"`
	ResponseMetadata ResponseMetadata `xml:"ResponseMetadata"`
}

// GetAccountSummaryResult contains the account summary map.
type GetAccountSummaryResult struct {
	SummaryMap []AccountSummaryEntry `xml:"SummaryMap>entry"`
}

// AccountSummaryEntry represents a single key-value entry in the account summary map.
type AccountSummaryEntry struct {
	Key   string `xml:"key"`
	Value int    `xml:"value"`
}

// GetAccountSummaryResponse is the XML response for GetAccountSummary.
type GetAccountSummaryResponse struct {
	XMLName                 xml.Name                `xml:"GetAccountSummaryResponse"`
	Xmlns                   string                  `xml:"xmlns,attr"`
	ResponseMetadata        ResponseMetadata        `xml:"ResponseMetadata"`
	GetAccountSummaryResult GetAccountSummaryResult `xml:"GetAccountSummaryResult"`
}
