package acm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

type requestCertificateInput struct {
	Options                 *certificateOptionsInput           `json:"Options,omitempty"`
	DomainName              string                             `json:"DomainName"`
	ValidationMethod        string                             `json:"ValidationMethod"`
	CertificateAuthorityArn string                             `json:"CertificateAuthorityArn"`
	IdempotencyToken        string                             `json:"IdempotencyToken"`
	KeyAlgorithm            string                             `json:"KeyAlgorithm"`
	ManagedBy               string                             `json:"ManagedBy,omitempty"`
	SubjectAlternativeNames []string                           `json:"SubjectAlternativeNames"`
	Tags                    []map[string]string                `json:"Tags"`
	DomainValidationOptions []domainValidationOptionInputEntry `json:"DomainValidationOptions"`
}

// domainValidationOptionInputEntry is the wire shape of one
// RequestCertificate DomainValidationOptions input entry, letting the caller
// pick which domain ACM should send EMAIL validation messages to.
type domainValidationOptionInputEntry struct {
	DomainName       string `json:"DomainName"`
	ValidationDomain string `json:"ValidationDomain"`
}

type requestCertificateOutput struct {
	CertificateArn string `json:"CertificateArn"`
}

type domainValidationOption struct {
	ResourceRecord   *resourceRecord `json:"ResourceRecord,omitempty"`
	HTTPRedirect     *httpRedirect   `json:"HttpRedirect,omitempty"`
	DomainName       string          `json:"DomainName"`
	ValidationDomain string          `json:"ValidationDomain"`
	ValidationStatus string          `json:"ValidationStatus"`
	ValidationMethod string          `json:"ValidationMethod"`
	ValidationEmails []string        `json:"ValidationEmails,omitempty"`
}

type resourceRecord struct {
	Name  string `json:"Name"`
	Type  string `json:"Type"`
	Value string `json:"Value"`
}

// httpRedirect is the wire shape of types.HttpRedirect
// (aws-sdk-go-v2/service/acm/types/types.go:1316-1327, v1.43.4).
type httpRedirect struct {
	RedirectFrom string `json:"RedirectFrom,omitempty"`
	RedirectTo   string `json:"RedirectTo,omitempty"`
}

// renewalSummaryDetail is the wire format for the RenewalSummary field in DescribeCertificate.
type renewalSummaryDetail struct {
	RenewalStatus           string                   `json:"RenewalStatus"`
	RenewalStatusReason     string                   `json:"RenewalStatusReason,omitempty"`
	DomainValidationOptions []domainValidationOption `json:"DomainValidationOptions,omitempty"`
	// UpdatedAt is required (always present) on the real AWS wire.
	UpdatedAt int64 `json:"UpdatedAt"`
}

type certificateDetail struct {
	RevokedAt               *int64                `json:"RevokedAt,omitempty"`
	IssuedAt                *int64                `json:"IssuedAt,omitempty"`
	ImportedAt              *int64                `json:"ImportedAt,omitempty"`
	RenewalSummary          *renewalSummaryDetail `json:"RenewalSummary,omitempty"`
	CertificateArn          string                `json:"CertificateArn"`
	DomainName              string                `json:"DomainName"`
	Serial                  string                `json:"Serial,omitempty"`
	Subject                 string                `json:"Subject,omitempty"`
	Issuer                  string                `json:"Issuer,omitempty"`
	KeyAlgorithm            string                `json:"KeyAlgorithm,omitempty"`
	SignatureAlgorithm      string                `json:"SignatureAlgorithm,omitempty"`
	Status                  string                `json:"Status"`
	Type                    string                `json:"Type"`
	RevocationReason        string                `json:"RevocationReason,omitempty"`
	RenewalEligibility      string                `json:"RenewalEligibility,omitempty"`
	FailureReason           string                `json:"FailureReason,omitempty"`
	CertificateAuthorityArn string                `json:"CertificateAuthorityArn,omitempty"`
	ManagedBy               string                `json:"ManagedBy,omitempty"`
	Options                 *certificateOptions   `json:"Options,omitempty"`
	SubjectAlternativeNames []string              `json:"SubjectAlternativeNames,omitempty"`
	// DomainValidationOptions uses a concrete slice type here to satisfy the JSON
	// marshaller; the field name matches the AWS wire format.
	DomainValidationOptions []domainValidationOption `json:"DomainValidationOptions"`
	// InUseBy is always present (possibly empty) matching real AWS DescribeCertificate behavior.
	InUseBy          []string            `json:"InUseBy"`
	KeyUsage         []keyUsageDetail    `json:"KeyUsages,omitempty"`
	ExtendedKeyUsage []extKeyUsageDetail `json:"ExtendedKeyUsages,omitempty"`
	CreatedAt        int64               `json:"CreatedAt"`
	NotBefore        int64               `json:"NotBefore,omitempty"`
	NotAfter         int64               `json:"NotAfter,omitempty"`
}

// keyUsageDetail wraps a single AWS key usage string.
type keyUsageDetail struct {
	Name string `json:"Name"`
}

// extKeyUsageDetail wraps a single AWS extended key usage string.
type extKeyUsageDetail struct {
	Name string `json:"Name"`
}

type certificateOptions struct {
	CertificateTransparencyLoggingPreference string `json:"CertificateTransparencyLoggingPreference,omitempty"`
	Export                                   string `json:"Export,omitempty"`
}

type describeCertificateOutput struct {
	Certificate certificateDetail `json:"Certificate"`
}

// certificateSummary is the CertificateSummary shape returned by
// ListCertificates. Unlike certificateDetail (DescribeCertificate), real
// AWS's CertificateSummary.KeyUsages/ExtendedKeyUsages are plain string
// arrays (types.KeyUsageName/types.ExtendedKeyUsageName), NOT arrays of
// {"Name": "..."} objects -- that wrapped-object shape only exists on
// CertificateDetail.KeyUsages ([]types.KeyUsage). Using keyUsageDetail here
// previously broke every real SDK client's ListCertificates deserializer.
type certificateSummary struct {
	CreatedAt                            *int64   `json:"CreatedAt,omitempty"`
	IssuedAt                             *int64   `json:"IssuedAt,omitempty"`
	ImportedAt                           *int64   `json:"ImportedAt,omitempty"`
	NotBefore                            *int64   `json:"NotBefore,omitempty"`
	NotAfter                             *int64   `json:"NotAfter,omitempty"`
	RevokedAt                            *int64   `json:"RevokedAt,omitempty"`
	Exported                             *bool    `json:"Exported,omitempty"`
	InUse                                *bool    `json:"InUse,omitempty"`
	HasAdditionalSubjectAlternativeNames *bool    `json:"HasAdditionalSubjectAlternativeNames,omitempty"`
	CertificateArn                       string   `json:"CertificateArn"`
	DomainName                           string   `json:"DomainName"`
	Status                               string   `json:"Status,omitempty"`
	KeyAlgorithm                         string   `json:"KeyAlgorithm,omitempty"`
	RenewalEligibility                   string   `json:"RenewalEligibility,omitempty"`
	Type                                 string   `json:"Type,omitempty"`
	ExportOption                         string   `json:"ExportOption,omitempty"`
	ManagedBy                            string   `json:"ManagedBy,omitempty"`
	SubjectAlternativeNameSummaries      []string `json:"SubjectAlternativeNameSummaries,omitempty"`
	KeyUsages                            []string `json:"KeyUsages,omitempty"`
	ExtendedKeyUsages                    []string `json:"ExtendedKeyUsages,omitempty"`
}

// listCertificatesIncludes mirrors the AWS Filters shape for ListCertificates.
type listCertificatesIncludes struct {
	KeyTypes         []string `json:"KeyTypes,omitempty"`
	ExtendedKeyUsage []string `json:"ExtendedKeyUsage,omitempty"`
	KeyUsage         []string `json:"KeyUsage,omitempty"`
}

type listCertificatesInput struct {
	Includes                  *listCertificatesIncludes `json:"Includes,omitempty"`
	NextToken                 string                    `json:"NextToken"`
	SortBy                    string                    `json:"SortBy,omitempty"`
	SortOrder                 string                    `json:"SortOrder,omitempty"`
	CertificateStatuses       []string                  `json:"CertificateStatuses,omitempty"`
	CertificateKeyPairOrigins []string                  `json:"CertificateKeyPairOrigins,omitempty"`
	MaxItems                  int                       `json:"MaxItems"`
}

type listCertificatesOutput struct {
	NextToken              string               `json:"NextToken,omitempty"`
	CertificateSummaryList []certificateSummary `json:"CertificateSummaryList"`
}

type deleteCertificateOutput struct{}

type describeCertificateInput struct {
	CertificateArn string `json:"CertificateArn"`
}

type deleteCertificateInput struct {
	CertificateArn string `json:"CertificateArn"`
}

// Certificate/PrivateKey/CertificateChain are []byte, not string: the real
// SDK base64-encodes these blob fields on the wire (acm@v1.43.4
// serializers.go:3284-3301, Base64EncodeBytes), and encoding/json auto-
// decodes a JSON string into []byte the same way. A string field would leave
// the payload still base64-encoded, breaking every real client's
// ImportCertificate.
type importCertificateInput struct {
	CertificateArn   string              `json:"CertificateArn"`
	Certificate      []byte              `json:"Certificate"`
	PrivateKey       []byte              `json:"PrivateKey"`
	CertificateChain []byte              `json:"CertificateChain"`
	Tags             []map[string]string `json:"Tags"`
}

type importCertificateOutput struct {
	CertificateArn string `json:"CertificateArn"`
}

type renewCertificateInput struct {
	CertificateArn string `json:"CertificateArn"`
}

type renewCertificateOutput struct{}

type exportCertificateInput struct {
	CertificateArn string `json:"CertificateArn"`
	Passphrase     string `json:"Passphrase"`
}

type exportCertificateOutput struct {
	Certificate      string `json:"Certificate"`
	CertificateChain string `json:"CertificateChain,omitempty"`
	PrivateKey       string `json:"PrivateKey"`
}

type getCertificateInput struct {
	CertificateArn string `json:"CertificateArn"`
}

type getCertificateOutput struct {
	Certificate      string `json:"Certificate"`
	CertificateChain string `json:"CertificateChain,omitempty"`
}

type resendValidationEmailInput struct {
	CertificateArn   string `json:"CertificateArn"`
	Domain           string `json:"Domain"`
	ValidationDomain string `json:"ValidationDomain"`
}

type resendValidationEmailOutput struct{}

type revokeCertificateInput struct {
	CertificateArn   string `json:"CertificateArn"`
	RevocationReason string `json:"RevocationReason"`
}

type revokeCertificateOutput struct{}

type certificateOptionsInput struct {
	CertificateTransparencyLoggingPreference string `json:"CertificateTransparencyLoggingPreference"`
	// Export is only meaningful on RequestCertificate; AWS ignores it on
	// UpdateCertificateOptions since Export is immutable after creation
	// ("You cannot update the value of Export after the certificate is
	// created.").
	Export string `json:"Export"`
}

type updateCertificateOptionsInput struct {
	CertificateArn string                  `json:"CertificateArn"`
	Options        certificateOptionsInput `json:"Options"`
}

type updateCertificateOptionsOutput struct{}

func (h *Handler) jsonRequestCertificate(ctx context.Context, body []byte) (any, error) {
	var input requestCertificateInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrRequestCertInvalidParameter
	}
	certType := ""
	if input.CertificateAuthorityArn != "" {
		certType = certTypePrivate
	}
	opts, exportPref := "", ""
	if input.Options != nil {
		opts = input.Options.CertificateTransparencyLoggingPreference
		exportPref = input.Options.Export
	}

	// Validate DomainValidationOptions and ManagedBy before creating
	// anything -- a ValidationException must not leave a certificate behind.
	allDomains := append([]string{input.DomainName}, input.SubjectAlternativeNames...)

	overrides, dvoErr := validateDomainValidationOptions(allDomains, toOverrideEntries(input.DomainValidationOptions))
	if dvoErr != nil {
		return nil, dvoErr
	}

	if mbErr := validateManagedBy(input.ManagedBy); mbErr != nil {
		return nil, mbErr
	}

	cert, err := h.Backend.RequestCertificate(
		ctx,
		input.DomainName,
		certType,
		input.ValidationMethod,
		input.IdempotencyToken,
		input.KeyAlgorithm,
		input.CertificateAuthorityArn,
		opts,
		input.SubjectAlternativeNames,
	)
	if err != nil {
		return nil, err
	}

	if applyErr := h.Backend.ApplyDomainValidationOverrides(ctx, cert.ARN, overrides); applyErr != nil {
		return nil, applyErr
	}

	if setErr := h.Backend.SetExportPreference(ctx, cert.ARN, exportPref); setErr != nil {
		return nil, setErr
	}

	if setErr := h.Backend.SetManagedBy(ctx, cert.ARN, input.ManagedBy); setErr != nil {
		return nil, setErr
	}

	// If tags were provided, apply them immediately after creating the certificate.
	if len(input.Tags) > 0 {
		kvMap := make(map[string]string, len(input.Tags))

		for _, tag := range input.Tags {
			if k, ok := tag["Key"]; ok {
				kvMap[k] = tag["Value"]
			}
		}
		if setErr := h.setTags(cert.ARN, kvMap, ErrInvalidTag, ErrTooManyTags); setErr != nil {
			return nil, setErr
		}
	}

	return &requestCertificateOutput{CertificateArn: cert.ARN}, nil
}

// toOverrideEntries adapts the wire-shape DomainValidationOptions input to
// the internal domainValidationOptionOverride type used by
// validateDomainValidationOptions.
func toOverrideEntries(in []domainValidationOptionInputEntry) []domainValidationOptionOverride {
	if len(in) == 0 {
		return nil
	}

	out := make([]domainValidationOptionOverride, 0, len(in))
	for _, e := range in {
		out = append(out, domainValidationOptionOverride(e))
	}

	return out
}

// buildDomainValidationOptionList projects backend DomainValidationOption
// entries into the wire shape shared by DescribeCertificate's top-level
// DomainValidationOptions and its nested RenewalSummary.DomainValidationOptions.
func buildDomainValidationOptionList(dvos []DomainValidationOption) []domainValidationOption {
	dvoList := make([]domainValidationOption, 0, len(dvos))

	for _, dvo := range dvos {
		opt := domainValidationOption{
			DomainName:       dvo.DomainName,
			ValidationDomain: dvo.ValidationDomain,
			ValidationStatus: dvo.ValidationStatus,
			ValidationMethod: dvo.ValidationMethod,
		}
		if dvo.ResourceRecord != nil {
			opt.ResourceRecord = &resourceRecord{
				Name:  dvo.ResourceRecord.Name,
				Type:  dvo.ResourceRecord.Type,
				Value: dvo.ResourceRecord.Value,
			}
		}

		if dvo.HTTPRedirect != nil {
			opt.HTTPRedirect = &httpRedirect{
				RedirectFrom: dvo.HTTPRedirect.RedirectFrom,
				RedirectTo:   dvo.HTTPRedirect.RedirectTo,
			}
		}

		if len(dvo.ValidationEmails) > 0 {
			opt.ValidationEmails = dvo.ValidationEmails
		}

		dvoList = append(dvoList, opt)
	}

	return dvoList
}

// buildRenewalSummaryDetail projects a backend RenewalSummary into the wire
// shape used by DescribeCertificate's nested RenewalSummary field. Returns
// nil when rs is nil (RenewalSummary is only ever present on AMAZON_ISSUED
// certificates that have had RenewCertificate called).
func buildRenewalSummaryDetail(rs *RenewalSummary) *renewalSummaryDetail {
	if rs == nil {
		return nil
	}

	return &renewalSummaryDetail{
		RenewalStatus:           rs.RenewalStatus,
		RenewalStatusReason:     rs.RenewalStatusReason,
		DomainValidationOptions: buildDomainValidationOptionList(rs.DomainValidationOptions),
		UpdatedAt:               rs.UpdatedAt.Unix(),
	}
}

func (h *Handler) jsonDescribeCertificate(ctx context.Context, body []byte) (any, error) {
	var input describeCertificateInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}
	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}
	cert, err := h.Backend.DescribeCertificate(ctx, input.CertificateArn)
	if err != nil {
		return nil, err
	}

	dvoList := buildDomainValidationOptionList(cert.DomainValidationOptions)

	keyUsages := make([]keyUsageDetail, 0, len(cert.KeyUsage))
	for _, ku := range cert.KeyUsage {
		keyUsages = append(keyUsages, keyUsageDetail{Name: ku})
	}

	extKeyUsages := make([]extKeyUsageDetail, 0, len(cert.ExtendedKeyUsage))
	for _, eku := range cert.ExtendedKeyUsage {
		extKeyUsages = append(extKeyUsages, extKeyUsageDetail{Name: eku})
	}

	detail := certificateDetail{
		CertificateArn:          cert.ARN,
		DomainName:              cert.DomainName,
		Serial:                  cert.Serial,
		Subject:                 cert.Subject,
		Issuer:                  cert.Issuer,
		KeyAlgorithm:            cert.KeyAlgorithm,
		SignatureAlgorithm:      cert.SignatureAlgorithm,
		Status:                  cert.Status,
		Type:                    cert.Type,
		RenewalEligibility:      cert.RenewalEligibility,
		RevocationReason:        cert.RevocationReason,
		FailureReason:           cert.FailureReason,
		CertificateAuthorityArn: cert.CertificateAuthorityArn,
		ManagedBy:               cert.ManagedBy,
		CreatedAt:               cert.CreatedAt.Unix(),
		NotBefore:               cert.NotBefore.Unix(),
		NotAfter:                cert.NotAfter.Unix(),
		SubjectAlternativeNames: cert.SubjectAlternativeNames,
		DomainValidationOptions: dvoList,
		Options:                 describeCertOptions(cert),
		RevokedAt:               certTimeUnix(cert.RevokedAt),
		IssuedAt:                certTimeUnix(cert.IssuedAt),
		ImportedAt:              certTimeUnix(cert.ImportedAt),
		InUseBy:                 nonNilSlice(cert.InUseBy),
		KeyUsage:                keyUsages,
		ExtendedKeyUsage:        extKeyUsages,

		RenewalSummary: buildRenewalSummaryDetail(cert.RenewalSummary)}

	return &describeCertificateOutput{Certificate: detail}, nil
}

// jsonListCertificates handles the ListCertificates operation.
func (h *Handler) jsonListCertificates(ctx context.Context, body []byte) (any, error) {
	var input listCertificatesInput
	_ = json.Unmarshal(body, &input)

	params := ListCertificatesParams{
		NextToken:                 input.NextToken,
		MaxItems:                  input.MaxItems,
		StatusFilter:              input.CertificateStatuses,
		CertificateKeyPairOrigins: input.CertificateKeyPairOrigins,
		SortBy:                    input.SortBy,
		SortOrder:                 input.SortOrder,
	}

	if input.Includes != nil {
		params.KeyTypes = input.Includes.KeyTypes
		params.KeyUsage = input.Includes.KeyUsage
		params.ExtendedKeyUsage = input.Includes.ExtendedKeyUsage
	}

	p, err := h.Backend.ListCertificates(ctx, params)
	if err != nil {
		return nil, err
	}

	summaries := make([]certificateSummary, 0, len(p.Data))

	for _, c := range p.Data {
		summaries = append(summaries, buildCertificateSummary(&c))
	}

	return &listCertificatesOutput{CertificateSummaryList: summaries, NextToken: p.Next}, nil
}

// buildCertificateSummary projects a backend Certificate into the wire-shape
// CertificateSummary returned by ListCertificates.
func buildCertificateSummary(c *Certificate) certificateSummary {
	inUse := len(c.InUseBy) > 0
	// Real ACM caps SubjectAlternativeNameSummaries at the first 100 names
	// and sets this true when more exist; gopherstack never stores more than
	// the account's domain-per-certificate quota (well under 100), so this
	// is always false here -- correct given our data, not a stubbed field.
	hasMoreSANs := false

	// Exported: real AWS's doc comment for CertificateSummary.Exported used to
	// read "This value exists only when the certificate type is PRIVATE"
	// (aws-sdk-go-v2/service/acm@v1.37.21/types/types.go) -- that sentence is
	// gone in the currently-installed v1.43.0 (confirmed by reading
	// types.go directly), dropped when AWS added exportable public
	// certificates in 2025. Now that AMAZON_ISSUED certificates can be
	// genuinely exported too (see validateCertExportable,
	// certificates.go), gating this field to PRIVATE only would be stale --
	// a real client exporting a now-eligible AMAZON_ISSUED cert would never
	// see Exported flip to true. Set unconditionally, matching
	// certToSearchResult's (handler_search_certificates.go) SearchCertificates
	// projection, which was already correctly unconditional.
	exported := c.Exported

	summary := certificateSummary{
		CertificateArn:                       c.ARN,
		DomainName:                           c.DomainName,
		Status:                               c.Status,
		KeyAlgorithm:                         c.KeyAlgorithm,
		RenewalEligibility:                   c.RenewalEligibility,
		Type:                                 c.Type,
		ExportOption:                         c.ExportPref,
		ManagedBy:                            c.ManagedBy,
		CreatedAt:                            certTimeUnix(&c.CreatedAt),
		IssuedAt:                             certTimeUnix(c.IssuedAt),
		ImportedAt:                           certTimeUnix(c.ImportedAt),
		RevokedAt:                            certTimeUnix(c.RevokedAt),
		SubjectAlternativeNameSummaries:      c.SubjectAlternativeNames,
		InUse:                                &inUse,
		HasAdditionalSubjectAlternativeNames: &hasMoreSANs,
		Exported:                             &exported,
	}

	summary.KeyUsages = append(summary.KeyUsages, c.KeyUsage...)
	summary.ExtendedKeyUsages = append(summary.ExtendedKeyUsages, c.ExtendedKeyUsage...)

	if !c.NotBefore.IsZero() {
		ts := c.NotBefore.Unix()
		summary.NotBefore = &ts
	}

	if !c.NotAfter.IsZero() {
		ts := c.NotAfter.Unix()
		summary.NotAfter = &ts
	}

	return summary
}

func (h *Handler) jsonDeleteCertificate(ctx context.Context, body []byte) (any, error) {
	var input deleteCertificateInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}
	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}
	if err := h.Backend.DeleteCertificate(ctx, input.CertificateArn); err != nil {
		return nil, err
	}
	h.cleanupTags(input.CertificateArn)

	return &deleteCertificateOutput{}, nil
}

func (h *Handler) jsonImportCertificate(ctx context.Context, body []byte) (any, error) {
	var input importCertificateInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}
	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}
	cert, err := h.Backend.ImportCertificate(
		ctx,
		string(input.Certificate),
		string(input.PrivateKey),
		string(input.CertificateChain),
		input.CertificateArn,
	)
	if err != nil {
		return nil, err
	}

	if len(input.Tags) > 0 {
		kvMap := make(map[string]string, len(input.Tags))

		for _, tag := range input.Tags {
			if k, ok := tag["Key"]; ok {
				kvMap[k] = tag["Value"]
			}
		}
		if setErr := h.setTags(cert.ARN, kvMap, ErrInvalidTag, ErrTooManyTags); setErr != nil {
			return nil, setErr
		}
	}

	return &importCertificateOutput{CertificateArn: cert.ARN}, nil
}

func (h *Handler) jsonRenewCertificate(ctx context.Context, body []byte) (any, error) {
	var input renewCertificateInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}
	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}
	if err := h.Backend.RenewCertificate(ctx, input.CertificateArn); err != nil {
		return nil, err
	}

	return &renewCertificateOutput{}, nil
}

func (h *Handler) jsonExportCertificate(ctx context.Context, body []byte) (any, error) {
	var input exportCertificateInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}

	if input.Passphrase == "" {
		return nil, fmt.Errorf("%w: Passphrase is required for ExportCertificate", ErrInvalidParameter)
	}

	passphrase, decErr := base64.StdEncoding.DecodeString(input.Passphrase)
	if decErr != nil {
		// Try raw (unpadded) base64.
		passphrase, decErr = base64.RawStdEncoding.DecodeString(input.Passphrase)
		if decErr != nil {
			return nil, fmt.Errorf("%w: Passphrase must be base64-encoded", ErrInvalidParameter)
		}
	}

	cert, err := h.Backend.ExportCertificate(ctx, input.CertificateArn, passphrase)
	if err != nil {
		return nil, err
	}

	return &exportCertificateOutput{
		Certificate:      cert.CertificateBody,
		CertificateChain: cert.CertificateChain,
		PrivateKey:       cert.PrivateKey,
	}, nil
}

func (h *Handler) jsonGetCertificate(ctx context.Context, body []byte) (any, error) {
	var input getCertificateInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}
	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}
	certBody, certChain, err := h.Backend.GetCertificate(ctx, input.CertificateArn)
	if err != nil {
		return nil, err
	}

	return &getCertificateOutput{
		Certificate:      certBody,
		CertificateChain: certChain,
	}, nil
}

func (h *Handler) jsonResendValidationEmail(ctx context.Context, body []byte) (any, error) {
	var input resendValidationEmailInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}

	err := h.Backend.ResendValidationEmail(ctx, input.CertificateArn, input.Domain, input.ValidationDomain)
	if err != nil {
		return nil, err
	}

	return &resendValidationEmailOutput{}, nil
}

func (h *Handler) jsonRevokeCertificate(ctx context.Context, body []byte) (any, error) {
	var input revokeCertificateInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}

	if err := h.Backend.RevokeCertificate(ctx, input.CertificateArn, input.RevocationReason); err != nil {
		return nil, err
	}

	return &revokeCertificateOutput{}, nil
}

func (h *Handler) jsonUpdateCertificateOptions(ctx context.Context, body []byte) (any, error) {
	var input updateCertificateOptionsInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateCertArn(input.CertificateArn); err != nil {
		return nil, err
	}

	if err := h.Backend.UpdateCertificateOptions(
		ctx,
		input.CertificateArn,
		input.Options.CertificateTransparencyLoggingPreference,
	); err != nil {
		return nil, err
	}

	return &updateCertificateOptionsOutput{}, nil
}

// describeCertOptions builds the Options response field if the cert has a transparency preference set.
func describeCertOptions(cert *Certificate) *certificateOptions {
	if cert.CertificateTransparencyLoggingPref == "" && cert.ExportPref == "" {
		return nil
	}

	return &certificateOptions{
		CertificateTransparencyLoggingPreference: cert.CertificateTransparencyLoggingPref,
		Export:                                   cert.ExportPref,
	}
}

// nonNilSlice returns s if non-nil, otherwise an empty slice.
// Used to ensure JSON marshals as [] instead of null.
func nonNilSlice(s []string) []string {
	if s == nil {
		return []string{}
	}

	return s
}

// certTimeUnix returns the Unix timestamp of a [time.Time] pointer, or nil if nil.
func certTimeUnix(t *time.Time) *int64 {
	if t == nil {
		return nil
	}

	ts := t.Unix()

	return &ts
}
