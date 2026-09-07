package acm

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// RequestCertificate creates a new certificate for the given domain.
// When validationMethod is "DNS" or "EMAIL" the certificate starts in
// PENDING_VALIDATION and automatically transitions to ISSUED after a short delay.
// idempotencyToken, if non-empty, deduplicates the request — repeated calls with
// the same token return the previously created certificate ARN.
func (b *InMemoryBackend) RequestCertificate(
	ctx context.Context,
	domainName, certType, validationMethod, idempotencyToken, keyAlgorithm, caArn, optionsPref string,
	sans []string,
) (*Certificate, error) {
	if err := validateRequestCertInput(domainName, sans); err != nil {
		return nil, err
	}

	certBody, privateKey, certMeta, notBefore, notAfter, err := generateSelfSignedCert(domainName, sans, keyAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificate: %w", err)
	}

	if keyAlgorithm == "" {
		keyAlgorithm = keyAlgorithmEC
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("RequestCertificate")
	defer b.mu.Unlock()

	// Idempotency: return existing cert if same token was already used.
	existing, found, checkErr := b.checkIdempotency(
		region, idempotencyToken, domainName, validationMethod, keyAlgorithm, sans,
	)
	if checkErr != nil {
		return nil, checkErr
	} else if found {
		return existing, nil
	}

	id := fmt.Sprintf("%x", time.Now().UnixNano())
	certARN := arn.Build("acm", region, b.accountID, "certificate/"+id)

	if certType == "" {
		certType = "AMAZON_ISSUED"
	}

	renewalEligibility := renewalEligibilityEligible
	if certType == certTypeImported {
		renewalEligibility = renewalEligibilityIneligible
	}

	// DomainValidationOptions overrides (custom EMAIL ValidationDomain per
	// domain) are validated and applied by the caller via
	// ApplyDomainValidationOverrides after creation -- see jsonRequestCertificate
	// -- since they arrive as part of the same RequestCertificate wire
	// request but this positional signature is depended on by a large
	// number of existing call sites.
	status, dvoList, err := buildInitialDVOList(domainName, sans, validationMethod, nil)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var issuedAt *time.Time

	if status == statusIssued {
		issuedAt = &now
	}
	if optionsPref == "" {
		optionsPref = transparencyLoggingEnabled
	}

	// Real AWS ACM always includes the primary domain as the first SAN entry.
	allSANs := buildSANList(domainName, sans)

	cert := &Certificate{
		ARN:                                certARN,
		DomainName:                         domainName,
		Serial:                             certMeta.serial,
		Subject:                            certMeta.subject,
		Issuer:                             certMeta.issuer,
		SubjectCommonName:                  certMeta.subjectCommonName,
		IssuerCommonName:                   certMeta.issuerCommonName,
		KeyAlgorithm:                       keyAlgorithm,
		SignatureAlgorithm:                 certMeta.signatureAlgorithm,
		Status:                             status,
		Type:                               certType,
		RenewalEligibility:                 renewalEligibility,
		ValidationMethod:                   validationMethod,
		IdempotencyToken:                   idempotencyToken,
		SubjectAlternativeNames:            allSANs,
		DomainValidationOptions:            dvoList,
		CertificateBody:                    certBody,
		PrivateKey:                         privateKey,
		CreatedAt:                          now,
		IssuedAt:                           issuedAt,
		NotBefore:                          notBefore,
		NotAfter:                           notAfter,
		KeyUsage:                           []string{keyUsageDigitalSignature},
		ExtendedKeyUsage:                   []string{extKeyUsageServerAuth},
		CertificateTransparencyLoggingPref: optionsPref,
		CertificateAuthorityArn:            caArn,
	}
	cert.region = region
	b.certs.Put(cert)
	b.recordNewCert(region, certARN, idempotencyToken, status, now)

	cp := copyCert(cert)

	return &cp, nil
}

// ApplyDomainValidationOverrides applies a RequestCertificate
// DomainValidationOptions input (already validated by
// validateDomainValidationOptions) to a just-created certificate, overriding
// the ValidationDomain -- and, for EMAIL validation, the well-known
// validation email addresses derived from it -- for each named domain. It is
// a no-op when overrides is empty.
func (b *InMemoryBackend) ApplyDomainValidationOverrides(
	ctx context.Context, certARN string, overrides map[string]string,
) error {
	if len(overrides) == 0 {
		return nil
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("ApplyDomainValidationOverrides")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	for i, dvo := range cert.DomainValidationOptions {
		vd, found := overrides[dvo.DomainName]
		if !found {
			continue
		}

		cert.DomainValidationOptions[i].ValidationDomain = vd

		if dvo.ValidationMethod != validationMethodEMAIL {
			continue
		}

		rootDomain := strings.TrimPrefix(vd, "*.")
		cert.DomainValidationOptions[i].ValidationEmails = []string{
			"admin@" + rootDomain,
			"administrator@" + rootDomain,
			"hostmaster@" + rootDomain,
			"postmaster@" + rootDomain,
			"webmaster@" + rootDomain,
		}
	}

	return nil
}

// SetExportPreference records a just-created certificate's RequestCertificate
// Options.Export choice (ENABLED/DISABLED). Real AWS treats Export as
// immutable once the certificate exists, so this is only ever called once,
// immediately after RequestCertificate, from jsonRequestCertificate. A blank
// pref is a no-op (the Certificate's zero value already reads as DISABLED).
func (b *InMemoryBackend) SetExportPreference(ctx context.Context, certARN, exportPref string) error {
	if exportPref == "" {
		return nil
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("SetExportPreference")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	cert.ExportPref = exportPref

	return nil
}

// SetManagedBy records a just-created certificate's RequestCertificate
// ManagedBy choice. The value must already be validated (see
// validateManagedBy, called before the certificate is created so an invalid
// value never leaves an orphaned certificate behind -- same reasoning as
// DomainValidationOptions, see ApplyDomainValidationOverrides). Like
// SetExportPreference, this is only ever called once, immediately after
// RequestCertificate, from jsonRequestCertificate. A blank value is a no-op
// (the Certificate's zero value already reads as caller-managed).
func (b *InMemoryBackend) SetManagedBy(ctx context.Context, certARN, managedBy string) error {
	if managedBy == "" {
		return nil
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("SetManagedBy")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	cert.ManagedBy = managedBy

	return nil
}

// recordNewCert records the idempotency-token mapping for a newly created certificate and
// schedules its auto-validation timer when the certificate is pending validation.
// Callers must hold b.mu.
func (b *InMemoryBackend) recordNewCert(region, certARN, idempotencyToken, status string, now time.Time) {
	if idempotencyToken != "" {
		b.idempotencyStore(region)[idempotencyToken] = certIdempotencyEntry{
			ARN:       certARN,
			CreatedAt: now,
		}
	}

	if status == statusPendingValidation {
		t := time.AfterFunc(b.getAutoValidateDelayLocked(), func() { b.autoValidate(region, certARN) })
		b.timersStore(region)[certARN] = t
	}
}

func (b *InMemoryBackend) checkIdempotency(
	region, idempotencyToken, domainName, validationMethod, keyAlgorithm string,
	sans []string,
) (*Certificate, bool, error) {
	if idempotencyToken == "" {
		return nil, false, nil
	}

	entry, ok := b.idempotencyStore(region)[idempotencyToken]
	if !ok {
		return nil, false, nil
	}

	c, exists := b.certs.Get(regionKey(region, entry.ARN))
	if !exists {
		return nil, false, nil
	}

	if c.DomainName != domainName || c.ValidationMethod != validationMethod ||
		c.KeyAlgorithm != keyAlgorithm ||
		!slices.Equal(c.SubjectAlternativeNames, buildSANList(domainName, sans)) {
		return nil, false, fmt.Errorf(
			"%w: idempotency token already used with different parameters",
			ErrRequestCertInvalidParameter,
		)
	}

	cp := copyCert(c)

	return &cp, true, nil
}

// validateRequestCertInput validates the DomainName and all SANs for a RequestCertificate call.
func validateRequestCertInput(domainName string, sans []string) error {
	if domainName == "" {
		return fmt.Errorf("%w: DomainName is required", ErrRequestCertInvalidParameter)
	}

	// AWS's default account quota for domain names per certificate (1
	// primary + 9 SANs); exceeding it is a quota violation
	// (LimitExceededException), not a shape/value validation failure.
	const maxDomainsPerCertificate = 10
	if len(sans)+1 > maxDomainsPerCertificate {
		return fmt.Errorf(
			"%w: maximum of 10 domain names (1 primary + 9 SANs) allowed per certificate",
			ErrLimitExceeded,
		)
	}

	if err := validateDomainName(domainName, ErrRequestCertInvalidParameter); err != nil {
		return err
	}

	for _, san := range sans {
		if err := validateDomainName(san, ErrRequestCertInvalidParameter); err != nil {
			return fmt.Errorf("%w: invalid SAN %q: %w", ErrRequestCertInvalidParameter, san, err)
		}
	}

	return nil
}

// buildInitialDVOList constructs the initial DomainValidationOptions list and determines
// the certificate's initial status based on the validation method. overrides
// (DomainName -> caller-supplied ValidationDomain, from RequestCertificate's
// DomainValidationOptions input) may be nil.
func buildInitialDVOList(
	domainName string,
	sans []string,
	validationMethod string,
	overrides map[string]string,
) (string, []DomainValidationOption, error) {
	allDomains := append([]string{domainName}, sans...)
	status := statusIssued

	var (
		dvoList []DomainValidationOption
		err     error
	)

	switch validationMethod {
	case validationMethodDNS, validationMethodEMAIL, validationMethodHTTP:
		status = statusPendingValidation
		dvoList, err = buildDomainValidationOptions(allDomains, validationMethod, overrides)
	default:
		dvoList, err = buildDomainValidationOptions(allDomains, validationMethodDNS, overrides)
	}

	if err != nil {
		return "", nil, err
	}

	if status == statusIssued {
		for i := range dvoList {
			dvoList[i].ValidationStatus = validationStatusSuccess
		}
	}

	return status, dvoList, nil
}

// ImportCertificate stores a PEM-encoded certificate, private key, and optional
// certificate chain, returning the ARN of the newly created or updated entry.
// When certARNToUpdate is non-empty, the existing certificate is updated in-place
// (re-import), matching AWS behavior where CertificateArn may be passed to replace
// an existing imported certificate.
func (b *InMemoryBackend) ImportCertificate(
	ctx context.Context,
	certBody, privateKey, certChain, certARNToUpdate string,
) (*Certificate, error) {
	if certBody == "" {
		return nil, fmt.Errorf("%w: Certificate is required", ErrInvalidParameter)
	}

	if privateKey == "" {
		return nil, fmt.Errorf("%w: PrivateKey is required", ErrInvalidParameter)
	}

	domainName, meta, notBefore, notAfter, err := extractCertMetadataFull(certBody)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid certificate body: %w", ErrInvalidParameter, err)
	}

	now := time.Now().UTC()

	region := getRegion(ctx, b.region)

	b.mu.Lock("ImportCertificate")
	defer b.mu.Unlock()

	// Re-import: update existing certificate in-place.
	if certARNToUpdate != "" {
		existing, ok := b.certs.Get(regionKey(region, certARNToUpdate))
		if !ok {
			return nil, fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARNToUpdate)
		}

		existing.CertificateBody = certBody
		existing.CertificateChain = certChain
		existing.PrivateKey = privateKey
		existing.DomainName = domainName
		existing.NotBefore = notBefore
		existing.NotAfter = notAfter
		existing.Serial = meta.serial
		existing.Subject = meta.subject
		existing.Issuer = meta.issuer
		existing.SubjectCommonName = meta.subjectCommonName
		existing.IssuerCommonName = meta.issuerCommonName
		existing.SignatureAlgorithm = meta.signatureAlgorithm
		existing.KeyAlgorithm = meta.keyAlgorithm
		existing.ImportedAt = &now
		existing.Status = statusIssued
		existing.KeyUsage = meta.keyUsage
		existing.ExtendedKeyUsage = meta.extKeyUsage

		cp := copyCert(existing)

		return &cp, nil
	}

	id := fmt.Sprintf("%x", time.Now().UnixNano())
	certARN := arn.Build("acm", region, b.accountID, "certificate/"+id)

	cert := &Certificate{
		ARN:                                certARN,
		DomainName:                         domainName,
		Serial:                             meta.serial,
		Subject:                            meta.subject,
		Issuer:                             meta.issuer,
		SubjectCommonName:                  meta.subjectCommonName,
		IssuerCommonName:                   meta.issuerCommonName,
		KeyAlgorithm:                       meta.keyAlgorithm,
		SignatureAlgorithm:                 meta.signatureAlgorithm,
		Status:                             statusIssued,
		Type:                               certTypeImported,
		RenewalEligibility:                 renewalEligibilityIneligible,
		CertificateBody:                    certBody,
		CertificateChain:                   certChain,
		PrivateKey:                         privateKey,
		CreatedAt:                          now,
		ImportedAt:                         &now,
		NotBefore:                          notBefore,
		NotAfter:                           notAfter,
		KeyUsage:                           meta.keyUsage,
		ExtendedKeyUsage:                   meta.extKeyUsage,
		CertificateTransparencyLoggingPref: transparencyLoggingEnabled,
		region:                             region,
	}
	b.certs.Put(cert)

	cp := copyCert(cert)

	return &cp, nil
}

// RenewCertificate regenerates the certificate material for an AMAZON_ISSUED certificate,
// extending its validity by one year. Returns ErrNotEligible for IMPORTED certificates,
// as AWS ACM does not support renewing imported certificates.
func (b *InMemoryBackend) RenewCertificate(ctx context.Context, certARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("RenewCertificate")
	defer b.mu.Unlock()

	c, exists := b.certs.Get(regionKey(region, certARN))
	if !exists {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if c.Type == certTypeImported {
		return fmt.Errorf("%w: only AMAZON_ISSUED certificates can be renewed", ErrNotEligible)
	}

	if c.CertificateAuthorityArn != "" {
		return fmt.Errorf("%w: PRIVATE certificates cannot be renewed through this API", ErrNotEligible)
	}

	domainName := c.DomainName
	sans := c.SubjectAlternativeNames
	validationMethod := c.ValidationMethod

	certBody, privateKey, meta, notBefore, notAfter, err := generateSelfSignedCert(domainName, sans, c.KeyAlgorithm)
	if err != nil {
		return fmt.Errorf("failed to generate self-signed certificate: %w", err)
	}

	status, dvoList, err := buildInitialDVOList(domainName, sans, validationMethod, nil)
	if err != nil {
		return fmt.Errorf("failed to build domain validation options: %w", err)
	}

	c.CertificateBody = certBody
	c.PrivateKey = privateKey
	c.Serial = meta.serial
	c.Subject = meta.subject
	c.Issuer = meta.issuer
	c.SubjectCommonName = meta.subjectCommonName
	c.IssuerCommonName = meta.issuerCommonName
	c.SignatureAlgorithm = meta.signatureAlgorithm
	c.NotBefore = notBefore
	c.NotAfter = notAfter

	// Mark the certificate as eligible for renewal and set the renewal summary.
	c.RenewalEligibility = renewalEligibilityEligible
	c.RenewalSummary = &RenewalSummary{
		UpdatedAt:               time.Now().UTC(),
		RenewalStatus:           status,
		DomainValidationOptions: dvoList,
	}

	if status == statusPendingValidation {
		t := time.AfterFunc(b.getAutoValidateDelayLocked(), func() { b.autoValidateRenewal(region, certARN) })
		// We can share the timer map, because normal validation is done
		// if a renewal is happening (a cert must be issued to be renewed).
		// Wait, if there's an existing timer, stop it first.
		timers := b.timersStore(region)
		if oldT, ok := timers[certARN]; ok {
			oldT.Stop()
		}
		timers[certARN] = t
	}

	return nil
}

// fakeCertChain is a fake PEM certificate chain (intermediate + root) returned by
// ExportCertificate when the stored certificate has no associated chain.
// This simulates the AWS ACM behavior of always returning a chain for exported certs.
const fakeCertChain = "-----BEGIN CERTIFICATE-----\n" +
	"MIIBpDCCAUqgAwIBAgIUFakeIntermediateCA0001AgAgAAgICAgICAgICIwCgYIKoZIzj0E\n" +
	"AwIwETEPMA0GA1UEAxMGZmFrZUNBMB4XDTIwMDEwMTAwMDAwMFoXDTMwMDEwMTAw\n" +
	"MDAwMFowETEPMA0GA1UEAxMGZmFrZUNBMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcD\n" +
	"QgAEfakeIntermediatePublicKeyDataHereForTestingPurposesOnlyNotRealCA\n" +
	"o0IwQDAdBgNVHQ4EFgQUFakeIntermediateCAKeyId001234MA8GA1UdEwEB/wQFMAMB\n" +
	"Af8wDgYDVR0PAQH/BAQDAgGGMAoGCCqGSM49BAMCA0gAMEUCIFakeIntermediateSig\n" +
	"nature001ForTestPurposesAgICAgICAgICAgICAgAgICAgICAgICAgICAgIA\n" +
	"-----END CERTIFICATE-----\n" +
	"-----BEGIN CERTIFICATE-----\n" +
	"MIIBmDCCAT6gAwIBAgIUFakeRootCA0001AgAgAAgICAgICAgICIwCgYIKoZIzj0E\n" +
	"AwIwDzENMAsGA1UEAxMEcm9vdDAeFw0yMDAxMDEwMDAwMDBaFw0zMDAxMDEwMDAw\n" +
	"MDBaMA8xDTALBgNVBAMTBHJvb3QwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAARf\n" +
	"akeRootPublicKeyDataHereForTestingPurposesOnlyNotARealRootCertificate\n" +
	"o0IwQDAdBgNVHQ4EFgQUFakeRootCAKeyId00123456789012345678901234567890\n" +
	"MA8GA1UdEwEB/wQFMAMBAf8wDgYDVR0PAQH/BAQDAgGGMAoGCCqGSM49BAMCA0AA\n" +
	"MEYCIQCFakeRootSignature001ForTestingPurposesNotARealSignatureAtAllXX\n" +
	"AiEAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICA\n" +
	"-----END CERTIFICATE-----\n"

// validateCertExportable enforces ACM's export-eligibility rules, confirmed
// against the live AWS API reference this pass (parity-5,
// docs.aws.amazon.com/acm/latest/APIReference/API_ExportCertificate.html and
// API_CertificateOptions.html):
//
//   - IMPORTED and PRIVATE certificates have always been exportable -- the
//     caller supplied or generated the private key themselves, ACM never
//     withholds it.
//   - AMAZON_ISSUED (public) certificates are gated by
//     CertificateOptions.Export: "You can opt in to allow the export of your
//     certificates by specifying ENABLED" (API_CertificateOptions.html).
//     ExportCertificateInput's own doc additionally notes "ACM public
//     certificates created prior to June 17, 2025 cannot be exported" -- not
//     modeled as a separate check here because every certificate this
//     backend creates is timestamped with the real current time, which is
//     always after that cutoff, so the date condition can never affect a
//     gopherstack-issued certificate; modeling it would mean adding a branch
//     only reachable by fabricating an artificially old CreatedAt.
//   - A still-pending AMAZON_ISSUED request keeps the pre-existing
//     RequestInProgressException ("The certificate request is in process and
//     the certificate in your account has not yet been issued" -- the exact
//     documented meaning of that error, confirmed against the op's Errors
//     section, which lists no distinct "not exportable" error at all).
//   - An issued-but-not-opted-in AMAZON_ISSUED certificate returns
//     ValidationException ("The supplied input failed to satisfy
//     constraints of an AWS service") -- the best (and only plausible) fit
//     among the op's five documented errors
//     (InvalidArnException/RequestInProgressException/
//     ResourceNotFoundException/ThrottlingException/ValidationException),
//     not RequestInProgressException, whose documented meaning is
//     specifically "not yet issued" and would misrepresent an already-issued
//     certificate that is simply ineligible.
func validateCertExportable(cert *Certificate) error {
	if cert.Type == certTypeImported || cert.Type == certTypePrivate {
		return nil
	}

	if cert.Status == statusPendingValidation || cert.Status == statusValidationTimedOut ||
		cert.Status == statusFailed {
		return fmt.Errorf("%w: certificate %s is in state %s", ErrRequestInProgress, cert.ARN, cert.Status)
	}

	if cert.ExportPref != certificateExportEnabled {
		return fmt.Errorf(
			"%w: AMAZON_ISSUED certificate %s is not exportable "+
				"(RequestCertificate's Options.Export must be ENABLED)",
			ErrInvalidParameter, cert.ARN,
		)
	}

	return nil
}

// ExportCertificate returns the PEM certificate body, chain, and private key for
// an eligible certificate -- IMPORTED/PRIVATE unconditionally, AMAZON_ISSUED only
// when opted in via Options.Export=ENABLED (see validateCertExportable).
// When the stored certificate has no associated chain, a fake chain (intermediate + root)
// is returned in PEM format to simulate AWS ACM behaviour.
// If passphrase is non-nil and non-empty, the private key is returned encrypted using AES-256.
func (b *InMemoryBackend) ExportCertificate(
	ctx context.Context, certARN string, passphrase []byte,
) (*Certificate, error) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("ExportCertificate")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return nil, fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if err := validateCertExportable(cert); err != nil {
		return nil, err
	}

	cert.Exported = true

	cp := copyCert(cert)

	// Always return a certificate chain; use a fake chain when none was supplied.
	if cp.CertificateChain == "" {
		cp.CertificateChain = fakeCertChain
	}

	if len(passphrase) > 0 {
		encKey, encErr := encryptPrivateKeyPEM(cp.PrivateKey, passphrase)
		if encErr != nil {
			return nil, fmt.Errorf("export: %w", encErr)
		}

		cp.PrivateKey = encKey
	}

	return &cp, nil
}

// GetCertificate returns the PEM certificate body and chain for any certificate.
func (b *InMemoryBackend) GetCertificate(ctx context.Context, certARN string) (string, string, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("GetCertificate")
	defer b.mu.RUnlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return "", "", fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if cert.Status == statusPendingValidation || cert.Status == statusValidationTimedOut ||
		cert.Status == statusFailed {
		return "", "", fmt.Errorf("%w: certificate %s is in state %s", ErrRequestInProgress, certARN, cert.Status)
	}

	return cert.CertificateBody, cert.CertificateChain, nil
}

// DescribeCertificate returns the certificate with the given ARN.
func (b *InMemoryBackend) DescribeCertificate(ctx context.Context, arn string) (*Certificate, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeCertificate")
	defer b.mu.RUnlock()

	cert, exists := b.certs.Get(regionKey(region, arn))
	if !exists {
		return nil, fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, arn)
	}

	cp := copyCert(cert)

	return &cp, nil
}

// ListCertificatesParams holds all filter and sorting options for ListCertificates.
type ListCertificatesParams struct {
	NextToken                 string
	SortBy                    string
	SortOrder                 string
	StatusFilter              []string
	KeyTypes                  []string
	KeyUsage                  []string
	ExtendedKeyUsage          []string
	CertificateKeyPairOrigins []string
	MaxItems                  int
}

// certKeyPairOrigin derives a certificate's CertificateKeyPairOrigin.
// gopherstack never creates Certificate records through the ACME workflow
// (acme_accounts.go et al. model ACME resources separately), so ACME is
// never produced here -- only the two origins RequestCertificate/
// ImportCertificate can actually generate.
func certKeyPairOrigin(c *Certificate) string {
	if c.Type == certTypeImported {
		return "CUSTOMER_PROVIDED"
	}

	return "AWS_MANAGED"
}

// listCertFilters holds compiled filter sets for ListCertificates.
type listCertFilters struct {
	statusSet        map[string]struct{}
	keyTypeSet       map[string]struct{}
	keyUsageSet      map[string]struct{}
	extKeyUsageSet   map[string]struct{}
	keyPairOriginSet map[string]struct{}
}

// buildListCertFilters compiles the filter sets from ListCertificatesParams.
func buildListCertFilters(p ListCertificatesParams) listCertFilters {
	f := listCertFilters{
		statusSet:        make(map[string]struct{}, len(p.StatusFilter)),
		keyTypeSet:       make(map[string]struct{}, len(p.KeyTypes)),
		keyUsageSet:      make(map[string]struct{}, len(p.KeyUsage)),
		extKeyUsageSet:   make(map[string]struct{}, len(p.ExtendedKeyUsage)),
		keyPairOriginSet: make(map[string]struct{}, len(p.CertificateKeyPairOrigins)),
	}

	for _, s := range p.StatusFilter {
		f.statusSet[s] = struct{}{}
	}

	for _, k := range p.KeyTypes {
		f.keyTypeSet[k] = struct{}{}
	}

	for _, ku := range p.KeyUsage {
		f.keyUsageSet[ku] = struct{}{}
	}

	for _, eku := range p.ExtendedKeyUsage {
		f.extKeyUsageSet[eku] = struct{}{}
	}

	for _, o := range p.CertificateKeyPairOrigins {
		f.keyPairOriginSet[o] = struct{}{}
	}

	return f
}

// matches returns true if the certificate satisfies all filters.
func (f listCertFilters) matches(c *Certificate) bool {
	if len(f.statusSet) > 0 {
		if _, ok := f.statusSet[c.Status]; !ok {
			return false
		}
	}

	if len(f.keyTypeSet) > 0 {
		if _, ok := f.keyTypeSet[c.KeyAlgorithm]; !ok {
			return false
		}
	}

	if len(f.keyUsageSet) > 0 && !matchesAny(c.KeyUsage, f.keyUsageSet) {
		return false
	}

	if len(f.extKeyUsageSet) > 0 && !matchesAny(c.ExtendedKeyUsage, f.extKeyUsageSet) {
		return false
	}

	if len(f.keyPairOriginSet) > 0 {
		if _, ok := f.keyPairOriginSet[certKeyPairOrigin(c)]; !ok {
			return false
		}
	}

	return true
}

// ListCertificates returns a paginated list of certificates, with optional
// filtering and sorting.
func (b *InMemoryBackend) ListCertificates(
	ctx context.Context, p ListCertificatesParams,
) (page.Page[Certificate], error) {
	if err := page.ValidateToken(p.NextToken); err != nil {
		return page.Page[Certificate]{}, fmt.Errorf("%w: invalid NextToken", ErrInvalidParameter)
	}

	if err := validateListCertificatesParams(p); err != nil {
		return page.Page[Certificate]{}, err
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("ListCertificates")
	defer b.mu.RUnlock()

	filters := buildListCertFilters(p)
	regionCerts := b.certsByRegion.Get(region)
	certs := make([]Certificate, 0, len(regionCerts))

	for _, c := range regionCerts {
		if filters.matches(c) {
			certs = append(certs, copyCert(c))
		}
	}

	descending := strings.EqualFold(p.SortOrder, "DESCENDING")

	switch strings.ToUpper(p.SortBy) {
	case listCertSortByCreatedAt:
		sort.Slice(certs, func(i, j int) bool {
			if descending {
				return certs[i].CreatedAt.After(certs[j].CreatedAt)
			}

			return certs[i].CreatedAt.Before(certs[j].CreatedAt)
		})
	default:
		// Default: sort by ARN for stable, deterministic ordering.
		sort.Slice(certs, func(i, j int) bool {
			if descending {
				return certs[i].ARN > certs[j].ARN
			}

			return certs[i].ARN < certs[j].ARN
		})
	}

	return page.New(certs, p.NextToken, p.MaxItems, acmDefaultMaxItems), nil
}

const acmDefaultMaxItems = 100

// matchesAny returns true if any element of values is in the set.
func matchesAny(values []string, set map[string]struct{}) bool {
	for _, v := range values {
		if _, ok := set[v]; ok {
			return true
		}
	}

	return false
}

// CertExists reports whether a certificate with the given ARN exists in the backend.
// This is used by the handler to validate tag operations.
func (b *InMemoryBackend) CertExists(ctx context.Context, certARN string) bool {
	region := getRegion(ctx, b.region)

	b.mu.RLock("CertExists")
	defer b.mu.RUnlock()

	return b.certs.Has(regionKey(region, certARN))
}

// AddInUseBy records that a resource ARN is using the certificate. It is a no-op
// if the certificate does not exist or the ARN is already present.
func (b *InMemoryBackend) AddInUseBy(ctx context.Context, certARN, resourceARN string) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("AddInUseBy")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return
	}

	if slices.Contains(cert.InUseBy, resourceARN) {
		return
	}

	cert.InUseBy = append(cert.InUseBy, resourceARN)
}

// RemoveInUseBy removes a resource ARN from the certificate's InUseBy list. It is a no-op
// if the certificate does not exist or the ARN is not present.
func (b *InMemoryBackend) RemoveInUseBy(ctx context.Context, certARN, resourceARN string) {
	region := getRegion(ctx, b.region)

	b.mu.Lock("RemoveInUseBy")
	defer b.mu.Unlock()

	cert, ok := b.certs.Get(regionKey(region, certARN))
	if !ok {
		return
	}

	filtered := cert.InUseBy[:0]

	for _, existing := range cert.InUseBy {
		if existing != resourceARN {
			filtered = append(filtered, existing)
		}
	}

	cert.InUseBy = filtered
}

// DeleteCertificate removes the certificate with the given ARN.
func (b *InMemoryBackend) DeleteCertificate(ctx context.Context, certARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteCertificate")
	defer b.mu.Unlock()

	key := regionKey(region, certARN)

	cert, exists := b.certs.Get(key)
	if !exists {
		return fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if len(cert.InUseBy) > 0 {
		return fmt.Errorf("%w: certificate %s is in use", ErrResourceInUse, certARN)
	}

	timers := b.timersStore(region)
	if t, ok := timers[certARN]; ok {
		t.Stop()
		delete(timers, certARN)
	}

	b.certs.Delete(key)

	// Drop any idempotency-token entries that pointed at this cert so the
	// map cannot grow unbounded for long-running backends.
	idempotency := b.idempotencyStore(region)
	for tok, entry := range idempotency {
		if entry.ARN == certARN {
			delete(idempotency, tok)
		}
	}

	return nil
}
