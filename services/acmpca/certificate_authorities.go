package acmpca

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// createCAOptions holds the optional CreateCertificateAuthority fields beyond
// the required caType/cfg (IdempotencyToken, KeyStorageSecurityStandard,
// RevocationConfiguration, UsageMode -- see aws-sdk-go-v2's
// CreateCertificateAuthorityInput). Zero value matches every pre-existing
// caller that omits opts entirely.
type createCAOptions struct {
	revocationConfiguration    *RevocationConfiguration
	idempotencyToken           string
	keyStorageSecurityStandard string
	usageMode                  string
}

// CreateCAOption customizes CreateCertificateAuthority. See WithCreateCA* below.
type CreateCAOption func(*createCAOptions)

// WithCreateCAIdempotencyToken deduplicates repeated CreateCertificateAuthority
// calls bearing the same token within a 5-minute window: the original CA's ARN
// is returned instead of creating a duplicate.
func WithCreateCAIdempotencyToken(token string) CreateCAOption {
	return func(o *createCAOptions) { o.idempotencyToken = token }
}

// WithCreateCAKeyStorageSecurityStandard sets KeyStorageSecurityStandard.
func WithCreateCAKeyStorageSecurityStandard(std string) CreateCAOption {
	return func(o *createCAOptions) { o.keyStorageSecurityStandard = std }
}

// WithCreateCAUsageMode sets UsageMode (GENERAL_PURPOSE or SHORT_LIVED_CERTIFICATE).
func WithCreateCAUsageMode(mode string) CreateCAOption {
	return func(o *createCAOptions) { o.usageMode = mode }
}

// WithCreateCARevocationConfiguration sets the CA's initial CRL/OCSP configuration.
func WithCreateCARevocationConfiguration(rc *RevocationConfiguration) CreateCAOption {
	return func(o *createCAOptions) { o.revocationConfiguration = rc }
}

// CreateCertificateAuthority creates a new Certificate Authority.
func (b *InMemoryBackend) CreateCertificateAuthority(
	ctx context.Context,
	caType string,
	cfg CertificateAuthorityConfiguration,
	opts ...CreateCAOption,
) (*CertificateAuthority, error) {
	caType, err := resolveCAType(caType)
	if err != nil {
		return nil, err
	}

	o, keyStorageSecurityStandard, usageMode, err := resolveCreateCAOptions(opts)
	if err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateCertificateAuthority")
	defer b.mu.Unlock()

	now := time.Now().UTC()

	if cached, ok := b.lookupIdempotentCA(region, o.idempotencyToken, now); ok {
		return cached, nil
	}

	ca, err := b.newCertificateAuthorityLocked(
		region, caType, cfg, keyStorageSecurityStandard, usageMode, o.revocationConfiguration, now,
	)
	if err != nil {
		return nil, err
	}

	b.rememberIdempotency(region, "CreateCertificateAuthority", o.idempotencyToken, ca.ARN, now)

	cp := copyCA(ca)

	return &cp, nil
}

// resolveCAType defaults an empty caType to ROOT and validates the result,
// matching CreateCertificateAuthorityInput.CertificateAuthorityType.
func resolveCAType(caType string) (string, error) {
	if caType == "" {
		caType = caTypePRoot
	}

	if caType != caTypePRoot && caType != caTypeSubordinate {
		return "", fmt.Errorf("%w: CertificateAuthorityType must be ROOT or SUBORDINATE", ErrInvalidArgs)
	}

	return caType, nil
}

// resolveCreateCAOptions applies opts and validates/defaults every optional
// CreateCertificateAuthority field, keeping that validation out of
// CreateCertificateAuthority's own cyclomatic complexity.
func resolveCreateCAOptions(opts []CreateCAOption) (createCAOptions, string, string, error) {
	var o createCAOptions
	for _, opt := range opts {
		opt(&o)
	}

	if err := validateRevocationConfiguration(o.revocationConfiguration); err != nil {
		return o, "", "", err
	}

	keyStorageSecurityStandard, err := resolveKeyStorageSecurityStandard(o.keyStorageSecurityStandard)
	if err != nil {
		return o, "", "", err
	}

	usageMode, err := resolveUsageMode(o.usageMode)
	if err != nil {
		return o, "", "", err
	}

	return o, keyStorageSecurityStandard, usageMode, nil
}

// lookupIdempotentCA returns a copy of the CA previously created for
// (region, token) within its idempotency window, if any. Must be called with
// b.mu.Lock held.
func (b *InMemoryBackend) lookupIdempotentCA(region, token string, now time.Time) (*CertificateAuthority, bool) {
	cachedARN, found := b.idempotentResourceARN(region, "CreateCertificateAuthority", token, now)
	if !found {
		return nil, false
	}

	cached, ok := b.caGet(region, cachedARN)
	if !ok {
		return nil, false
	}

	cp := copyCA(cached)

	return &cp, true
}

// newCertificateAuthorityLocked generates the CA's key/CSR, stores it, and
// (for ROOT) self-signs and activates it. Must be called with b.mu.Lock held.
func (b *InMemoryBackend) newCertificateAuthorityLocked(
	region, caType string,
	cfg CertificateAuthorityConfiguration,
	keyStorageSecurityStandard, usageMode string,
	revocationConfiguration *RevocationConfiguration,
	now time.Time,
) (*CertificateAuthority, error) {
	id, err := newRandomID()
	if err != nil {
		return nil, err
	}

	caARN := arn.Build("acm-pca", region, b.accountID, caResourceIDPrefix+id)

	if cfg.KeyAlgorithm == "" {
		cfg.KeyAlgorithm = defaultKeyAlgorithm
	}

	if cfg.SigningAlgorithm == "" {
		cfg.SigningAlgorithm = defaultSignAlgorithm
	}

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}

	csrPEM, err := generateCSR(privKey, cfg.Subject)
	if err != nil {
		return nil, fmt.Errorf("generate CSR: %w", err)
	}

	ca := &CertificateAuthority{
		ARN:                               caARN,
		OwnerAccount:                      b.accountID,
		Type:                              caType,
		Status:                            caStatusCreating,
		CertificateAuthorityConfiguration: cfg,
		CSR:                               csrPEM,
		CreatedAt:                         now,
		LastStateChangeAt:                 now,
		KeyStorageSecurityStandard:        keyStorageSecurityStandard,
		UsageMode:                         usageMode,
		RevocationConfiguration:           revocationConfiguration,
		privKey:                           privKey,
		region:                            region,
	}

	b.caPut(ca)

	// For ROOT CAs we auto-sign and activate to make Terraform apply succeed without
	// requiring a multi-step workflow.
	if caType == caTypePRoot {
		if activateErr := b.selfSignAndActivate(ca, now); activateErr != nil {
			return nil, activateErr
		}
	} else {
		// SUBORDINATE CAs immediately transition to PENDING_CERTIFICATE, matching
		// AWS behaviour (CREATING is only a transient internal state).
		ca.Status = caStatusPendingCertificate
	}

	return ca, nil
}

// resolveKeyStorageSecurityStandard validates std against
// types.KeyStorageSecurityStandard's known values, defaulting to
// FIPS_140_2_LEVEL_3_OR_HIGHER (the real API's documented default) when empty.
func resolveKeyStorageSecurityStandard(std string) (string, error) {
	if std == "" {
		return keyStorageStandardFips3, nil
	}

	switch std {
	case keyStorageStandardFips2, keyStorageStandardFips3, keyStorageStandardCCPC1:
		return std, nil
	default:
		return "", fmt.Errorf("%w: unsupported KeyStorageSecurityStandard %q", ErrInvalidArgs, std)
	}
}

// resolveUsageMode validates mode against types.CertificateAuthorityUsageMode's
// known values, defaulting to GENERAL_PURPOSE (the real API's documented default)
// when empty.
func resolveUsageMode(mode string) (string, error) {
	if mode == "" {
		return usageModeGeneralPurpose, nil
	}

	switch mode {
	case usageModeGeneralPurpose, usageModeShortLivedCertificate:
		return mode, nil
	default:
		return "", fmt.Errorf("%w: unsupported UsageMode %q", ErrInvalidArgs, mode)
	}
}

// validateRevocationConfiguration enforces the documented CreateCertificateAuthority/
// UpdateCertificateAuthority constraints on rc (nil is always valid -- CRL/OCSP left
// unconfigured): a configuration disabling CRLs or OCSP must set only Enabled=false,
// and an *enabled* CRL configuration must specify the S3 bucket to write to.
func validateRevocationConfiguration(rc *RevocationConfiguration) error {
	if rc == nil {
		return nil
	}

	if err := validateCrlConfiguration(rc.CrlConfiguration); err != nil {
		return err
	}

	return validateOcspConfiguration(rc.OcspConfiguration)
}

// crlDisabledExtraFieldsSet reports whether crl sets any field beyond Enabled,
// which is invalid once Enabled is false (see validateCrlConfiguration).
func crlDisabledExtraFieldsSet(crl *CrlConfiguration) bool {
	return crl.ExpirationInDays != 0 || crl.CustomCname != "" || crl.CustomPath != "" ||
		crl.S3BucketName != "" || crl.S3ObjectACL != "" || crl.CrlType != "" || crl.OmitExtension
}

func validateCrlConfiguration(crl *CrlConfiguration) error {
	if crl == nil {
		return nil
	}

	switch {
	case !crl.Enabled && crlDisabledExtraFieldsSet(crl):
		return fmt.Errorf("%w: CrlConfiguration with Enabled=false must not set any other field", ErrInvalidArgs)
	case crl.Enabled && crl.S3BucketName == "":
		return fmt.Errorf("%w: CrlConfiguration.S3BucketName is required when Enabled=true", ErrInvalidArgs)
	}

	if crl.CrlType != "" && crl.CrlType != crlTypeComplete && crl.CrlType != crlTypePartitioned {
		return fmt.Errorf("%w: unsupported CrlType %q", ErrInvalidArgs, crl.CrlType)
	}

	if crl.S3ObjectACL != "" && crl.S3ObjectACL != s3ObjectACLPublicRead && crl.S3ObjectACL != s3ObjectACLBucketOwner {
		return fmt.Errorf("%w: unsupported S3ObjectAcl %q", ErrInvalidArgs, crl.S3ObjectACL)
	}

	return nil
}

func validateOcspConfiguration(ocsp *OcspConfiguration) error {
	if ocsp != nil && !ocsp.Enabled && ocsp.OcspCustomCname != "" {
		return fmt.Errorf("%w: OcspConfiguration with Enabled=false must not set OcspCustomCname", ErrInvalidArgs)
	}

	return nil
}

// selfSignAndActivate generates a self-signed certificate for the CA and sets it to ACTIVE.
// Must be called with the write lock held.
func (b *InMemoryBackend) selfSignAndActivate(ca *CertificateAuthority, now time.Time) error {
	certPEM, serial, err := selfSignCA(ca, now)
	if err != nil {
		return fmt.Errorf("self-sign CA: %w", err)
	}

	ca.CertificateBody = certPEM
	ca.Serial = serial
	ca.Status = caStatusActive
	ca.NotBefore = now
	ca.NotAfter = now.Add(10 * 365 * 24 * time.Hour)
	ca.LastStateChangeAt = now

	return nil
}

// verifyCertificateAuthorityActive checks that the CA exists and is not DELETED.
func (b *InMemoryBackend) verifyCertificateAuthorityActive(ctx context.Context, caARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.RLock("verifyCertificateAuthorityActive")
	defer b.mu.RUnlock()

	ca, ok := b.caGet(region, caARN)
	if !ok {
		return fmt.Errorf("%w: CA %s not found", ErrCANotFound, caARN)
	}

	if ca.Status == caStatusDeleted {
		return fmt.Errorf("%w: CA is DELETED", ErrInvalidState)
	}

	return nil
}

// DescribeCertificateAuthority returns the CA with the given ARN.
func (b *InMemoryBackend) DescribeCertificateAuthority(
	ctx context.Context, caARN string,
) (*CertificateAuthority, error) {
	if err := validateRequiredParameter(caARN, "CertificateAuthorityArn", ErrInvalidArn); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeCertificateAuthority")
	defer b.mu.RUnlock()

	ca, ok := b.caGet(region, caARN)
	if !ok {
		return nil, fmt.Errorf("%w: CA %s not found", ErrCANotFound, caARN)
	}

	cp := copyCA(ca)

	return &cp, nil
}

// ListCertificateAuthorities returns a paginated list of CAs sorted by ARN.
// resourceOwner mirrors ListCertificateAuthoritiesInput.ResourceOwner: SELF
// (or empty, the real API's default) lists CAs owned by this account;
// OTHER_ACCOUNTS always returns an empty page, since gopherstack does not
// model cross-account CA sharing (no CA here is ever owned by another
// account -- see PARITY.md).
func (b *InMemoryBackend) ListCertificateAuthorities(
	ctx context.Context, nextToken string, maxItems int, resourceOwner string,
) (page.Page[CertificateAuthority], error) {
	switch resourceOwner {
	case "", resourceOwnerSelf:
		// proceed below
	case resourceOwnerOtherAccounts:
		return page.Page[CertificateAuthority]{Data: []CertificateAuthority{}}, nil
	default:
		// ListCertificateAuthorities's own error model declares only
		// InvalidNextTokenException -- not InvalidArgsException, and no
		// other declared code fits a bad ResourceOwner value. No correct
		// code exists to send here; left rather than substituted
		// (gopherstack-6flj/uox6 error-envelope sweep).
		return page.Page[CertificateAuthority]{}, fmt.Errorf(
			"%w: unsupported ResourceOwner %q", ErrInvalidArgs, resourceOwner,
		)
	}

	region := getRegion(ctx, b.region)

	var cas []CertificateAuthority
	func() {
		b.mu.RLock("ListCertificateAuthorities")
		defer b.mu.RUnlock()

		casInRegion := b.casInRegion(region)
		cas = make([]CertificateAuthority, 0, len(casInRegion))
		for _, ca := range casInRegion {
			cas = append(cas, copyCA(ca))
		}
	}()

	sort.Slice(cas, func(i, j int) bool { return cas[i].ARN < cas[j].ARN })

	// api_op_ListCertificateAuthorities.go: "Although the maximum value is
	// 1000, the action only returns a maximum of 100 items." -- the page size
	// never exceeds defaultMaxItems (100) even when the caller requests more.
	if maxItems <= 0 || maxItems > defaultMaxItems {
		maxItems = defaultMaxItems
	}

	return page.New(cas, nextToken, maxItems, defaultMaxItems), nil
}

// DeleteCertificateAuthority marks the CA as DELETED.
func (b *InMemoryBackend) DeleteCertificateAuthority(
	ctx context.Context, caARN string, permanentDeletionDays int32,
) error {
	if permanentDeletionDays != 0 &&
		(permanentDeletionDays < permanentDeletionMinDays || permanentDeletionDays > permanentDeletionMaxDays) {
		// DeleteCertificateAuthority's own error model declares
		// ConcurrentModification, InvalidArn, InvalidState, ResourceNotFound
		// -- not InvalidArgsException, and no other declared code fits a
		// day-count range check either (InvalidArnException's doc is
		// specifically about ARNs). No ValidationException exists anywhere
		// in this SDK module. No correct code exists to send here; left
		// rather than substituted (gopherstack-6flj/uox6 sweep).
		return fmt.Errorf(
			"%w: PermanentDeletionTimeInDays must be between %d and %d",
			ErrInvalidArgs,
			permanentDeletionMinDays,
			permanentDeletionMaxDays,
		)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteCertificateAuthority")
	defer b.mu.Unlock()

	ca, ok := b.caGet(region, caARN)
	if !ok {
		return fmt.Errorf("%w: CA %s not found", ErrCANotFound, caARN)
	}

	switch ca.Status {
	case caStatusDisabled, caStatusCreating, caStatusPendingCertificate:
		// allowed
	default:
		return fmt.Errorf(
			"%w: CA must be in DISABLED, CREATING, or PENDING_CERTIFICATE state (current: %s)",
			ErrInvalidState,
			ca.Status,
		)
	}

	// AWS defaults PermanentDeletionTimeInDays to 30 when unset; DescribeCertificateAuthority
	// reports the remaining restoration window via RestorableUntil.
	days := permanentDeletionDays
	if days == 0 {
		days = defaultPermanentDeletionDays
	}

	now := time.Now().UTC()
	ca.RestorableUntil = now.AddDate(0, 0, int(days))
	ca.Status = caStatusDeleted
	ca.LastStateChangeAt = now

	return nil
}

// updateCAOptions holds the optional UpdateCertificateAuthority fields beyond
// the required caARN/status (RevocationConfiguration -- see aws-sdk-go-v2's
// UpdateCertificateAuthorityInput). Zero value matches every pre-existing
// caller that omits opts entirely.
type updateCAOptions struct {
	revocationConfiguration *RevocationConfiguration
	revocationConfigSet     bool
}

// UpdateCAOption customizes UpdateCertificateAuthority. See WithUpdateCA* below.
type UpdateCAOption func(*updateCAOptions)

// WithUpdateCARevocationConfiguration replaces the CA's CRL/OCSP configuration.
// Per the real API, omitting this option entirely (the zero-value default)
// leaves the CA's existing RevocationConfiguration unchanged; passing rc (even
// nil, meaning "clear it") always overwrites it.
func WithUpdateCARevocationConfiguration(rc *RevocationConfiguration) UpdateCAOption {
	return func(o *updateCAOptions) { o.revocationConfiguration = rc; o.revocationConfigSet = true }
}

// UpdateCertificateAuthority updates the CA status and/or revocation configuration.
func (b *InMemoryBackend) UpdateCertificateAuthority(
	ctx context.Context, caARN, status string, opts ...UpdateCAOption,
) error {
	if err := validateRequiredParameter(caARN, "CertificateAuthorityArn", ErrInvalidArn); err != nil {
		return err
	}

	if status != "" && status != caStatusActive && status != caStatusDisabled {
		return fmt.Errorf("%w: status must be ACTIVE or DISABLED", ErrInvalidArgs)
	}

	var o updateCAOptions
	for _, opt := range opts {
		opt(&o)
	}

	if o.revocationConfigSet {
		if err := validateRevocationConfiguration(o.revocationConfiguration); err != nil {
			return err
		}
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateCertificateAuthority")
	defer b.mu.Unlock()

	ca, ok := b.caGet(region, caARN)
	if !ok {
		return fmt.Errorf("%w: CA %s not found", ErrCANotFound, caARN)
	}

	// api_op_UpdateCertificateAuthority.go: "Your private CA must be in the
	// ACTIVE or DISABLED state before you can update it." Applies to every
	// update (status transition or RevocationConfiguration change alone), not
	// just a status change -- otherwise a CREATING/PENDING_CERTIFICATE/DELETED
	// CA could be flipped straight to ACTIVE, bypassing
	// ImportCertificateAuthorityCertificate/RestoreCertificateAuthority.
	if ca.Status != caStatusActive && ca.Status != caStatusDisabled {
		return fmt.Errorf(
			"%w: CA %s must be in ACTIVE or DISABLED state to be updated (current: %s)",
			ErrInvalidState, caARN, ca.Status,
		)
	}

	if status != "" {
		ca.Status = status
		ca.LastStateChangeAt = time.Now().UTC()
	}

	if o.revocationConfigSet {
		ca.RevocationConfiguration = o.revocationConfiguration
	}

	return nil
}

// GetCertificateAuthorityCsr returns the CSR PEM for the given CA.
func (b *InMemoryBackend) GetCertificateAuthorityCsr(ctx context.Context, caARN string) (string, error) {
	if err := validateRequiredParameter(caARN, "CertificateAuthorityArn", ErrInvalidArn); err != nil {
		return "", err
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("GetCertificateAuthorityCsr")
	defer b.mu.RUnlock()

	ca, ok := b.caGet(region, caARN)
	if !ok {
		return "", fmt.Errorf("%w: CA %s not found", ErrCANotFound, caARN)
	}

	return ca.CSR, nil
}

// ImportCertificateAuthorityCertificate imports a signed certificate for the CA, activating it.
// It parses the certificate to extract NotBefore/NotAfter and stores the optional chain.
func (b *InMemoryBackend) ImportCertificateAuthorityCertificate(
	ctx context.Context, caARN, certPEM, chainPEM string,
) error {
	if err := validateRequiredParameter(caARN, "CertificateAuthorityArn", ErrInvalidArn); err != nil {
		return err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("ImportCertificateAuthorityCertificate")
	defer b.mu.Unlock()

	ca, ok := b.caGet(region, caARN)
	if !ok {
		return fmt.Errorf("%w: CA %s not found", ErrCANotFound, caARN)
	}

	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return fmt.Errorf("%w: failed to decode certificate PEM for CA %s", ErrMalformedCertificate, caARN)
	}

	parsedCert, parseErr := x509.ParseCertificate(block.Bytes)
	if parseErr != nil {
		return fmt.Errorf("%w: failed to parse certificate for CA %s: %w", ErrMalformedCertificate, caARN, parseErr)
	}

	ca.NotBefore = parsedCert.NotBefore
	ca.NotAfter = parsedCert.NotAfter
	ca.Serial = hex.EncodeToString(parsedCert.SerialNumber.Bytes())

	ca.CertificateBody = certPEM
	ca.CertificateChain = chainPEM
	ca.Status = caStatusActive
	ca.LastStateChangeAt = time.Now().UTC()

	return nil
}

// GetCertificateAuthorityCertificate returns the certificate body and chain PEM for the given CA.
func (b *InMemoryBackend) GetCertificateAuthorityCertificate(
	ctx context.Context, caARN string,
) (string, string, error) {
	if err := validateRequiredParameter(caARN, "CertificateAuthorityArn", ErrInvalidArn); err != nil {
		return "", "", err
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("GetCertificateAuthorityCertificate")
	defer b.mu.RUnlock()

	ca, ok := b.caGet(region, caARN)
	if !ok {
		return "", "", fmt.Errorf("%w: CA %s not found", ErrCANotFound, caARN)
	}

	if ca.CertificateBody == "" {
		return "", "", fmt.Errorf("%w: CA %s has no certificate imported", ErrCANotFound, caARN)
	}

	return ca.CertificateBody, ca.CertificateChain, nil
}

// RestoreCertificateAuthority restores a deleted CA into the DISABLED state.
func (b *InMemoryBackend) RestoreCertificateAuthority(ctx context.Context, caARN string) error {
	if err := validateRequiredParameter(caARN, "CertificateAuthorityArn", ErrInvalidArn); err != nil {
		return err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("RestoreCertificateAuthority")
	defer b.mu.Unlock()

	ca, ok := b.caGet(region, caARN)
	if !ok {
		return fmt.Errorf("%w: CA %s not found", ErrCANotFound, caARN)
	}

	if ca.Status != caStatusDeleted {
		return fmt.Errorf("%w: CA %s is not DELETED", ErrInvalidState, caARN)
	}

	ca.Status = caStatusDisabled
	ca.RestorableUntil = time.Time{}
	ca.LastStateChangeAt = time.Now().UTC()

	return nil
}

// copyCA returns a shallow copy of the CertificateAuthority, excluding the private key.
func copyCA(ca *CertificateAuthority) CertificateAuthority {
	cp := *ca
	cp.privKey = nil

	return cp
}
