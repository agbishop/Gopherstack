package acmpca

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// hexBase is the base used to parse/format a certificate serial number
// (hex.EncodeToString on the way in, big.Int decimal on the way out for the
// certificate ARN -- see IssueCertificate).
const hexBase = 16

// issueCertOptions holds the optional IssueCertificate fields beyond the
// required caARN/csrPEM/validityDays (IdempotencyToken, TemplateArn,
// APIPassthrough, ValidityNotBefore -- see aws-sdk-go-v2's IssueCertificateInput).
// Zero value matches every pre-existing caller that omits opts entirely.
type issueCertOptions struct {
	validityNotBefore time.Time
	apiPassthrough    *APIPassthrough
	idempotencyToken  string
	templateArn       string
}

// IssueCertOption customizes IssueCertificate. See WithIssueCert* below.
type IssueCertOption func(*issueCertOptions)

// WithIssueCertIdempotencyToken deduplicates repeated IssueCertificate calls
// bearing the same token within a 5-minute window: the original certificate's
// ARN is returned instead of issuing a duplicate.
func WithIssueCertIdempotencyToken(token string) IssueCertOption {
	return func(o *issueCertOptions) { o.idempotencyToken = token }
}

// WithIssueCertTemplateArn selects a certificate template. Only its
// APIPassthrough/APICSRPassthrough gating behavior is modeled (see
// templateAllowsAPIPassthrough): a non-passthrough template's APIPassthrough
// input is ignored, matching the real API's own documented behavior. Per-template
// default X.509 extension profiles are not modeled -- see PARITY.md.
func WithIssueCertTemplateArn(templateArn string) IssueCertOption {
	return func(o *issueCertOptions) { o.templateArn = templateArn }
}

// WithIssueCertAPIPassthrough applies custom subject/extension overrides,
// honored only when the request's TemplateArn selects an APIPassthrough/
// APICSRPassthrough template variant (see templateAllowsAPIPassthrough).
func WithIssueCertAPIPassthrough(ap *APIPassthrough) IssueCertOption {
	return func(o *issueCertOptions) { o.apiPassthrough = ap }
}

// WithIssueCertValidityNotBefore overrides the certificate's "Not Before" date
// (default: issuance time).
func WithIssueCertValidityNotBefore(notBefore time.Time) IssueCertOption {
	return func(o *issueCertOptions) { o.validityNotBefore = notBefore }
}

// templateAllowsAPIPassthrough reports whether templateArn selects an
// APIPassthrough or APICSRPassthrough template variant, per
// IssueCertificateInput.APIPassthrough's doc comment: "An APIPassthrough or
// APICSRPassthrough template variant must be selected, or else this parameter
// is ignored." An empty templateArn defaults to EndEntityCertificate/V1, which
// is not a passthrough variant.
func templateAllowsAPIPassthrough(templateArn string) bool {
	return strings.Contains(templateArn, "APIPassthrough") || strings.Contains(templateArn, "APICSRPassthrough")
}

// IssueCertificate issues a new certificate signed by the given CA.
func (b *InMemoryBackend) IssueCertificate(
	ctx context.Context, caARN, csrPEM string, validityDays int, opts ...IssueCertOption,
) (*IssuedCertificate, error) {
	if err := validateIssueCertificateInput(caARN, csrPEM); err != nil {
		return nil, err
	}

	o := resolveIssueCertOptions(opts)

	region := getRegion(ctx, b.region)

	b.mu.Lock("IssueCertificate")
	defer b.mu.Unlock()

	now := time.Now().UTC()

	if cached, ok := b.lookupIdempotentCert(region, o.idempotencyToken, now); ok {
		return cached, nil
	}

	cert, err := b.signAndStoreCertificateLocked(region, caARN, csrPEM, validityDays, o, now)
	if err != nil {
		return nil, err
	}

	b.rememberIdempotency(region, "IssueCertificate", o.idempotencyToken, cert.ARN, now)

	cp := *cert

	return &cp, nil
}

func validateIssueCertificateInput(caARN, csrPEM string) error {
	if err := validateRequiredParameter(caARN, "CertificateAuthorityArn", ErrInvalidArn); err != nil {
		return err
	}

	return validateRequiredParameter(csrPEM, "Csr", ErrMalformedCSR)
}

// resolveIssueCertOptions applies opts and enforces the real API's
// documented APIPassthrough-gating rule: it is silently ignored unless the
// template selected is an APIPassthrough/APICSRPassthrough variant.
func resolveIssueCertOptions(opts []IssueCertOption) issueCertOptions {
	var o issueCertOptions
	for _, opt := range opts {
		opt(&o)
	}

	if o.apiPassthrough != nil && !templateAllowsAPIPassthrough(o.templateArn) {
		o.apiPassthrough = nil
	}

	return o
}

// lookupIdempotentCert returns a copy of the certificate previously issued for
// (region, token) within its idempotency window, if any. Must be called with
// b.mu.Lock held.
func (b *InMemoryBackend) lookupIdempotentCert(region, token string, now time.Time) (*IssuedCertificate, bool) {
	cachedARN, found := b.idempotentResourceARN(region, "IssueCertificate", token, now)
	if !found {
		return nil, false
	}

	cached, ok := b.certGet(region, cachedARN)
	if !ok {
		return nil, false
	}

	cp := *cached

	return &cp, true
}

// signAndStoreCertificateLocked validates the issuing CA, signs csrPEM, builds
// the certificate's ARN (embedding its own serial number in decimal -- see
// IssueCertificateOutput's doc comment example), and stores the result. Must
// be called with b.mu.Lock held.
func (b *InMemoryBackend) signAndStoreCertificateLocked(
	region, caARN, csrPEM string, validityDays int, o issueCertOptions, now time.Time,
) (*IssuedCertificate, error) {
	ca, ok := b.caGet(region, caARN)
	if !ok {
		return nil, fmt.Errorf("%w: CA %s not found", ErrCANotFound, caARN)
	}

	if ca.Status != caStatusActive {
		return nil, fmt.Errorf("%w: CA %s is not ACTIVE", ErrInvalidState, caARN)
	}

	if validityDays <= 0 {
		validityDays = 365
	}

	if ca.UsageMode == usageModeShortLivedCertificate && validityDays > shortLivedCertMaxValidityDays {
		return nil, fmt.Errorf(
			"%w: CA %s has UsageMode SHORT_LIVED_CERTIFICATE, which limits certificate validity to %d days",
			ErrInvalidArgs, caARN, shortLivedCertMaxValidityDays,
		)
	}

	certPEM, serial, err := signCSR(ca, csrPEM, validityDays, o.validityNotBefore, o.apiPassthrough)
	if err != nil {
		return nil, fmt.Errorf("sign CSR: %w", err)
	}

	// gopherstack previously appended an unrelated random ID as the certificate
	// ARN's final path segment instead of the serial number -- a wire-shape bug
	// found while diffing this pass (see PARITY.md).
	serialInt, ok := new(big.Int).SetString(serial, hexBase)
	if !ok {
		return nil, fmt.Errorf("%w: could not parse issued certificate serial %q", ErrInvalidArgs, serial)
	}

	certARN := arn.Build("acm-pca", region, b.accountID,
		caResourceIDPrefix+extractCAID(caARN)+"/"+certResourceIDPrefix+serialInt.String())

	notBefore := o.validityNotBefore
	if notBefore.IsZero() {
		notBefore = now
	}

	cert := &IssuedCertificate{
		ARN:       certARN,
		CAARN:     caARN,
		Status:    certStatusActive,
		Serial:    serial,
		CertBody:  certPEM,
		IssuedAt:  now,
		NotBefore: notBefore,
		NotAfter:  notBefore.Add(time.Duration(validityDays) * 24 * time.Hour),
		region:    region,
	}

	b.certPut(cert)
	b.certsByCASerialStore(region)[caARN+"#"+serial] = certARN

	return cert, nil
}

// GetCertificate returns the certificate for the given CA and certificate ARN.
// It validates that the certificate belongs to the specified CA.
func (b *InMemoryBackend) GetCertificate(ctx context.Context, caARN, certARN string) (*IssuedCertificate, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("GetCertificate")
	defer b.mu.RUnlock()

	cert, ok := b.certGet(region, certARN)
	if !ok {
		return nil, fmt.Errorf("%w: certificate %s not found", ErrCertNotFound, certARN)
	}

	if cert.CAARN != caARN {
		return nil, fmt.Errorf("%w: certificate %s does not belong to CA %s", ErrCANotFound, certARN, caARN)
	}

	cp := *cert

	return &cp, nil
}

// RevokeCertificate revokes the given certificate using the O(1) serial index.
func (b *InMemoryBackend) RevokeCertificate(ctx context.Context, caARN, serial, revocationReason string) error {
	if revocationReason != "" {
		switch revocationReason {
		case revocationReasonUnspecified, revocationReasonKeyCompromise, revocationReasonCACompromise,
			revocationReasonAffiliation, revocationReasonSuperseded, revocationReasonCessation,
			revocationReasonPrivWithdrawn, revocationReasonAACompromise:
			// valid
		default:
			return fmt.Errorf("%w: invalid RevocationReason %q", ErrInvalidRequest, revocationReason)
		}
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("RevokeCertificate")
	defer b.mu.Unlock()

	ca, ok := b.caGet(region, caARN)
	if !ok {
		return fmt.Errorf("%w: CA %s not found", ErrCANotFound, caARN)
	}

	if ca.Status == caStatusDeleted {
		return fmt.Errorf("%w: CA %s is DELETED", ErrInvalidState, caARN)
	}

	certARN, ok := b.certsByCASerialStore(region)[caARN+"#"+serial]
	if !ok {
		return fmt.Errorf("%w: certificate with serial %s not found", ErrCertNotFound, serial)
	}

	cert, ok := b.certGet(region, certARN)
	if !ok {
		return fmt.Errorf("%w: certificate with serial %s not found", ErrCertNotFound, serial)
	}

	// RevokeCertificate's own deserializeOpError models RequestAlreadyProcessedException
	// ("Your request has already been completed") -- no other op in this service does,
	// evidence a repeat revocation must be rejected rather than silently re-applied.
	if cert.Status == certStatusRevoked {
		return fmt.Errorf("%w: certificate with serial %s is already revoked", ErrRequestAlreadyProcessed, serial)
	}

	cert.Status = certStatusRevoked
	now := time.Now().UTC()
	cert.RevokedAt = &now
	cert.RevocationReason = revocationReason

	return nil
}

// ListCertificates returns a paginated list of certificates issued by the given CA.
func (b *InMemoryBackend) ListCertificates(
	ctx context.Context,
	caARN string,
	nextToken string,
	maxItems int,
) page.Page[IssuedCertificate] {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListCertificates")
	defer b.mu.RUnlock()

	certsForCA := b.certsForCA(region, caARN)
	certs := make([]IssuedCertificate, 0, len(certsForCA))
	for _, c := range certsForCA {
		certs = append(certs, *c)
	}

	sort.Slice(certs, func(i, j int) bool { return certs[i].ARN < certs[j].ARN })

	return page.New(certs, nextToken, maxItems, defaultMaxItems)
}
