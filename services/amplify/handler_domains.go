package amplify

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
)

// JSON response key used by the domain association handlers.
const keyDomainAssociation = "domainAssociation"

// domainCertificateSettingsIn mirrors aws-sdk-go-v2/service/amplify/
// types.CertificateSettings, the nested wire shape of
// CreateDomainAssociationInput/UpdateDomainAssociationInput's
// "certificateSettings" member.
type domainCertificateSettingsIn struct {
	CertificateType      string `json:"type"`
	CustomCertificateARN string `json:"customCertificateArn"`
}

func (c *domainCertificateSettingsIn) toBackend() *domainCertificateSettings {
	if c == nil {
		return nil
	}

	return &domainCertificateSettings{
		CertificateType:      c.CertificateType,
		CustomCertificateARN: c.CustomCertificateARN,
	}
}

// handleDomainAssociations handles POST/GET /apps/{appId}/domains.
func (h *Handler) handleDomainAssociations(ctx context.Context, c *echo.Context, appID string) error {
	switch c.Request().Method {
	case http.MethodPost:
		return h.createDomainAssociation(ctx, c, appID)
	case http.MethodGet:
		return h.listDomainAssociations(ctx, c, appID)
	default:
		return amplifyErrorJSON(c, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleDomainAssociationName handles GET/DELETE/POST /apps/{appId}/domains/{domainName}.
func (h *Handler) handleDomainAssociationName(
	ctx context.Context,
	c *echo.Context,
	appID, domainName string,
) error {
	switch c.Request().Method {
	case http.MethodGet:
		return h.getDomainAssociation(ctx, c, appID, domainName)
	case http.MethodDelete:
		return h.deleteDomainAssociation(ctx, c, appID, domainName)
	case http.MethodPost:
		return h.updateDomainAssociation(ctx, c, appID, domainName)
	default:
		return amplifyErrorJSON(c, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// createDomainAssociation handles POST /apps/{appId}/domains.
func (h *Handler) createDomainAssociation(ctx context.Context, c *echo.Context, appID string) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return amplifyErrorJSON(c, http.StatusInternalServerError, err.Error())
	}

	var input struct {
		CertificateSettings           *domainCertificateSettingsIn `json:"certificateSettings"`
		DomainName                    string                       `json:"domainName"`
		AutoSubDomainIAMRole          string                       `json:"autoSubDomainIAMRole"`
		SubDomainSettings             []SubDomainSetting           `json:"subDomainSettings"`
		AutoSubDomainCreationPatterns []string                     `json:"autoSubDomainCreationPatterns"`
		EnableAutoSubDomain           bool                         `json:"enableAutoSubDomain"`
	}

	if jsonErr := json.Unmarshal(body, &input); jsonErr != nil {
		return amplifyErrorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	domain, createErr := h.Backend.CreateDomainAssociation(
		appID, input.DomainName, input.SubDomainSettings, input.EnableAutoSubDomain,
		input.AutoSubDomainCreationPatterns, input.AutoSubDomainIAMRole,
		input.CertificateSettings.toBackend(),
	)
	if createErr != nil {
		return h.handleBackendError(ctx, c, "CreateDomainAssociation", createErr)
	}

	return c.JSON(http.StatusCreated, map[string]any{keyDomainAssociation: toDomainAssociationView(domain)})
}

// listDomainAssociations handles GET /apps/{appId}/domains.
func (h *Handler) listDomainAssociations(ctx context.Context, c *echo.Context, appID string) error {
	q := c.Request().URL.Query()
	nextToken := q.Get("nextToken")

	maxResults := 0
	if s := q.Get("maxResults"); s != "" {
		if n, convErr := strconv.Atoi(s); convErr == nil && n > 0 {
			maxResults = n
		}
	}

	domains, outToken, err := h.Backend.ListDomainAssociations(appID, nextToken, maxResults)
	if err != nil {
		return h.handleBackendError(ctx, c, opListDomainAssociations, err)
	}

	resp := map[string]any{"domainAssociations": toDomainAssociationViews(domains)}
	if outToken != "" {
		resp["nextToken"] = outToken
	}

	return c.JSON(http.StatusOK, resp)
}

// getDomainAssociation handles GET /apps/{appId}/domains/{domainName}.
func (h *Handler) getDomainAssociation(
	ctx context.Context,
	c *echo.Context,
	appID, domainName string,
) error {
	domain, err := h.Backend.GetDomainAssociation(appID, domainName)
	if err != nil {
		return h.handleBackendError(ctx, c, "GetDomainAssociation", err)
	}

	return c.JSON(http.StatusOK, map[string]any{keyDomainAssociation: toDomainAssociationView(domain)})
}

// deleteDomainAssociation handles DELETE /apps/{appId}/domains/{domainName}.
func (h *Handler) deleteDomainAssociation(
	ctx context.Context,
	c *echo.Context,
	appID, domainName string,
) error {
	domain, err := h.Backend.DeleteDomainAssociation(appID, domainName)
	if err != nil {
		return h.handleBackendError(ctx, c, "DeleteDomainAssociation", err)
	}

	return c.JSON(http.StatusOK, map[string]any{keyDomainAssociation: toDomainAssociationView(domain)})
}

// updateDomainAssociation handles POST /apps/{appId}/domains/{domainName}.
func (h *Handler) updateDomainAssociation(
	ctx context.Context,
	c *echo.Context,
	appID, domainName string,
) error {
	body, err := httputils.ReadBody(c.Request())
	if err != nil {
		return amplifyErrorJSON(c, http.StatusInternalServerError, err.Error())
	}

	var input struct {
		CertificateSettings           *domainCertificateSettingsIn `json:"certificateSettings"`
		AutoSubDomainIAMRole          *string                      `json:"autoSubDomainIAMRole"`
		EnableAutoSubDomain           *bool                        `json:"enableAutoSubDomain"`
		SubDomainSettings             []SubDomainSetting           `json:"subDomainSettings"`
		AutoSubDomainCreationPatterns []string                     `json:"autoSubDomainCreationPatterns"`
	}

	if jsonErr := json.Unmarshal(body, &input); jsonErr != nil {
		return amplifyErrorJSON(c, http.StatusBadRequest, "invalid request body")
	}

	domain, updateErr := h.Backend.UpdateDomainAssociation(
		appID, domainName, input.SubDomainSettings, input.EnableAutoSubDomain,
		input.AutoSubDomainCreationPatterns, input.AutoSubDomainIAMRole,
		input.CertificateSettings.toBackend(),
	)
	if updateErr != nil {
		return h.handleBackendError(ctx, c, "UpdateDomainAssociation", updateErr)
	}

	return c.JSON(http.StatusOK, map[string]any{keyDomainAssociation: toDomainAssociationView(domain)})
}

type subDomainSettingView struct {
	Prefix     string `json:"prefix"`
	BranchName string `json:"branchName"`
}

type subDomainView struct {
	SubDomainSetting subDomainSettingView `json:"subDomainSetting"`
	DNSRecord        string               `json:"dnsRecord,omitempty"`
	Verified         bool                 `json:"verified"`
}

// domainCertificateView mirrors aws-sdk-go-v2/service/amplify/types.Certificate
// on the response side.
type domainCertificateView struct {
	CertificateType                  string `json:"type"`
	CertificateVerificationDNSRecord string `json:"certificateVerificationDNSRecord,omitempty"`
	CustomCertificateARN             string `json:"customCertificateArn,omitempty"`
}

type domainAssociationView struct {
	DomainName                       string                 `json:"domainName"`
	ARN                              string                 `json:"domainAssociationArn"`
	DomainStatus                     string                 `json:"domainStatus"`
	StatusReason                     string                 `json:"statusReason"`
	CertificateVerificationDNSRecord string                 `json:"certificateVerificationDNSRecord,omitempty"`
	AutoSubDomainIAMRole             string                 `json:"autoSubDomainIAMRole,omitempty"`
	Certificate                      *domainCertificateView `json:"certificate,omitempty"`
	SubDomains                       []subDomainView        `json:"subDomains"`
	AutoSubDomainCreationPatterns    []string               `json:"autoSubDomainCreationPatterns,omitempty"`
	EnableAutoSubDomain              bool                   `json:"enableAutoSubDomain"`
}

func toDomainAssociationView(d *DomainAssociation) domainAssociationView {
	subs := make([]subDomainView, len(d.SubDomains))
	for i, sd := range d.SubDomains {
		subs[i] = subDomainView{
			SubDomainSetting: subDomainSettingView{
				Prefix:     sd.SubDomainSetting.Prefix,
				BranchName: sd.SubDomainSetting.BranchName,
			},
			DNSRecord: sd.DNSRecord,
			Verified:  sd.Verified,
		}
	}

	var cert *domainCertificateView
	if d.CertificateType != "" {
		cert = &domainCertificateView{
			CertificateType:                  d.CertificateType,
			CertificateVerificationDNSRecord: d.CertificateVerificationDNSRecord,
			CustomCertificateARN:             d.CertificateCustomArn,
		}
	}

	return domainAssociationView{
		SubDomains:                       subs,
		DomainName:                       d.DomainName,
		ARN:                              d.ARN,
		DomainStatus:                     string(d.DomainStatus),
		StatusReason:                     d.StatusReason,
		CertificateVerificationDNSRecord: d.CertificateVerificationDNSRecord,
		AutoSubDomainCreationPatterns:    d.AutoSubDomainCreationPatterns,
		AutoSubDomainIAMRole:             d.AutoSubDomainIAMRole,
		Certificate:                      cert,
		EnableAutoSubDomain:              d.EnableAutoSubDomain,
	}
}

func toDomainAssociationViews(ds []*DomainAssociation) []domainAssociationView {
	views := make([]domainAssociationView, len(ds))
	for i, d := range ds {
		views[i] = toDomainAssociationView(d)
	}

	return views
}
