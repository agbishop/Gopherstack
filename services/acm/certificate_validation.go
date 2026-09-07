package acm

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// certArnPattern matches the ACM certificate ARN shape:
// arn:<partition>:acm:<region>:<account>:certificate/<id>
// This mirrors the pattern the real SDK validates client-side
// (arn:[\w+=/,.@-]+:acm:[\w+=/,.@-]*:[0-9]+:[\w+=,.@-]+(/[\w+=,.@-]+)*) but is
// narrowed to the "certificate/" resource type, since that is the only
// resource shape a CertificateArn field ever carries.
var certArnPattern = regexp.MustCompile(`^arn:[\w+=/,.@-]+:acm:[\w+=/,.@-]*:[0-9]+:certificate/[\w+=,.@-]+$`)

// validateCertArn checks that a non-empty CertificateArn matches the expected
// ACM ARN shape, returning ErrInvalidArn (InvalidArnException) if it does
// not. An empty ARN is intentionally not flagged here -- callers that
// require a non-empty CertificateArn already surface their own
// ValidationException with a clearer "CertificateArn is required" message,
// and read paths correctly fall through to ErrCertNotFound for "".
func validateCertArn(certARN string) error {
	if certARN == "" {
		return nil
	}

	if !certArnPattern.MatchString(certARN) {
		return fmt.Errorf("%w: %q is not a valid ACM certificate ARN", ErrInvalidArn, certARN)
	}

	return nil
}

// validateManagedBy checks a RequestCertificate ManagedBy input against real
// AWS's CertificateManagedBy enum, which currently defines a single value,
// "CLOUDFRONT" (aws-sdk-go-v2/service/acm/types.CertificateManagedByCloudfront).
// An empty value (the common, caller-managed case) is left alone. Validated
// before the certificate is created so a bad value never leaves an orphaned
// certificate behind, matching validateDomainValidationOptions' reasoning.
func validateManagedBy(managedBy string) error {
	if managedBy == "" || managedBy == certManagedByCloudfront {
		return nil
	}

	return fmt.Errorf("%w: ManagedBy must be %s", ErrRequestCertInvalidParameter, certManagedByCloudfront)
}

// isSameOrSuperdomain reports whether validationDomain is the same as, or a
// superdomain of, domain -- the constraint AWS enforces on
// DomainValidationOption.ValidationDomain (RequestCertificate input). A
// leading "*." wildcard on domain is stripped before comparison, matching
// AWS's own handling of wildcard certificate requests.
func isSameOrSuperdomain(domain, validationDomain string) bool {
	d := strings.TrimPrefix(domain, "*.")
	if validationDomain == d {
		return true
	}

	return strings.HasSuffix(d, "."+validationDomain)
}

// validateDomainValidationOptions checks a RequestCertificate
// DomainValidationOptions input against the domains actually being
// requested (domainName + sans). Each entry's DomainName must reference one
// of those domains, and its ValidationDomain must be the same as or a
// superdomain of that DomainName -- violations return
// ErrInvalidDomainValidationOptions (InvalidDomainValidationOptionsException),
// matching real AWS.
func validateDomainValidationOptions(
	allDomains []string, opts []domainValidationOptionOverride,
) (map[string]string, error) {
	if len(opts) == 0 {
		return nil, nil //nolint:nilnil // absent overrides map is a meaningful "use defaults" signal
	}

	domainSet := make(map[string]struct{}, len(allDomains))
	for _, d := range allDomains {
		domainSet[d] = struct{}{}
	}

	overrides := make(map[string]string, len(opts))

	for _, o := range opts {
		if _, ok := domainSet[o.DomainName]; !ok {
			return nil, fmt.Errorf(
				"%w: DomainName %q is not part of this certificate request",
				ErrInvalidDomainValidationOptions, o.DomainName,
			)
		}

		if o.ValidationDomain == "" || !isSameOrSuperdomain(o.DomainName, o.ValidationDomain) {
			return nil, fmt.Errorf(
				"%w: ValidationDomain %q for %q must be the domain itself or a superdomain",
				ErrInvalidDomainValidationOptions, o.ValidationDomain, o.DomainName,
			)
		}

		overrides[o.DomainName] = o.ValidationDomain
	}

	return overrides, nil
}

// validateDomainName checks that the given domain name satisfies AWS ACM constraints.
// AWS rejects domain names longer than 253 characters, empty labels, labels exceeding 63
// characters, and labels that are purely numeric (which would be IP addresses).
// invalidErr lets each caller supply the code its own op actually declares:
// RequestCertificate's real deserializer has no ValidationException, only
// InvalidParameterException (ErrRequestCertInvalidParameter), while
// CreateAcmeDomainValidation's does (ErrInvalidParameter) -- gopherstack-bzyl.
func validateDomainName(name string, invalidErr error) error {
	if len(name) > maxDomainLength {
		return fmt.Errorf("%w: domain %q exceeds maximum length of %d", invalidErr, name, maxDomainLength)
	}

	// Strip leading wildcard component (*.example.com → example.com for label checks)
	checkName := strings.TrimPrefix(name, "*.")

	for label := range strings.SplitSeq(checkName, ".") {
		if label == "" {
			return fmt.Errorf("%w: domain %q contains an empty label", invalidErr, name)
		}

		if len(label) > maxDomainLabelLength {
			return fmt.Errorf("%w: domain label %q in %q exceeds %d characters",
				invalidErr, label, name, maxDomainLabelLength)
		}
	}

	return nil
}

// domainValidationOptionOverride is the parsed form of one RequestCertificate
// DomainValidationOptions input entry (see requestCertificateInput in
// handler_certificates.go), naming the ValidationDomain AWS should use for a
// given DomainName's EMAIL validation.
type domainValidationOptionOverride struct {
	DomainName       string
	ValidationDomain string
}

// buildDomainValidationOptions creates DomainValidationOption entries with
// synthetic CNAME records for DNS validation, or synthetic email addresses
// for EMAIL validation. overrides maps DomainName -> caller-supplied
// ValidationDomain (from RequestCertificate's DomainValidationOptions input,
// already validated by validateDomainValidationOptions); a domain absent
// from overrides defaults to using itself as its own ValidationDomain.
func buildDomainValidationOptions(
	domains []string, validationMethod string, overrides map[string]string,
) ([]DomainValidationOption, error) {
	opts := make([]DomainValidationOption, 0, len(domains))
	seen := make(map[string]bool, len(domains))

	for _, d := range domains {
		if seen[d] {
			continue
		}
		seen[d] = true

		status := validationStatusSuccess
		if validationMethod == validationMethodDNS || validationMethod == validationMethodEMAIL ||
			validationMethod == validationMethodHTTP {
			status = statusPendingValidation
		}

		validationDomain := d
		if vd, ok := overrides[d]; ok {
			validationDomain = vd
		}

		opt := DomainValidationOption{
			DomainName:       d,
			ValidationDomain: validationDomain,
			ValidationStatus: status,
			ValidationMethod: validationMethod,
		}

		switch validationMethod {
		case validationMethodDNS:
			nameToken, err := randHex()
			if err != nil {
				return nil, err
			}

			valueToken, err := randHex()
			if err != nil {
				return nil, err
			}

			opt.ResourceRecord = &ResourceRecord{
				Name:  "_" + nameToken + "." + d + ".",
				Type:  "CNAME",
				Value: "_" + valueToken + ".acm-validations.aws.",
			}

		case validationMethodEMAIL:
			// AWS sends validation emails to well-known addresses at the
			// validation domain root (the DomainName's superdomain when a
			// ValidationDomain override was supplied, otherwise itself).
			rootDomain := strings.TrimPrefix(validationDomain, "*.")

			opt.ValidationEmails = []string{
				"admin@" + rootDomain,
				"administrator@" + rootDomain,
				"hostmaster@" + rootDomain,
				"postmaster@" + rootDomain,
				"webmaster@" + rootDomain,
			}

		case validationMethodHTTP:
			token, err := randHex()
			if err != nil {
				return nil, err
			}

			opt.HTTPRedirect = &HTTPRedirect{
				RedirectFrom: "http://" + d + "/.well-known/pki-validation/" + token + ".txt",
				RedirectTo:   "https://acm-validations.aws/" + token,
			}
		}

		opts = append(opts, opt)
	}

	return opts, nil
}

// validCertificateStatuses, validListCertKeyAlgorithms, validKeyUsageNames,
// and validExtendedKeyUsageNames are ListCertificates' CertificateStatuses/
// Includes filter enums (aws-sdk-go-v2/service/acm/types: CertificateStatus
// enums.go:199-224, KeyAlgorithm enums.go:416-442, KeyUsageName
// enums.go:445-476, ExtendedKeyUsageName enums.go:328-363, v1.43.4).
//
//nolint:gochecknoglobals // read-only enum value sets initialized once at startup
var (
	validCertificateStatuses = map[string]struct{}{
		statusPendingValidation:  {},
		statusIssued:             {},
		statusInactive:           {},
		statusExpired:            {},
		statusValidationTimedOut: {},
		statusRevoked:            {},
		statusFailed:             {},
	}
	validListCertKeyAlgorithms = map[string]struct{}{
		keyAlgorithmRSA1024: {}, keyAlgorithmRSA2048: {}, keyAlgorithmRSA3072: {}, keyAlgorithmRSA4096: {},
		keyAlgorithmEC: {}, keyAlgorithmECSecp384r1: {}, keyAlgorithmECSecp521r1: {},
	}
	validKeyUsageNames = map[string]struct{}{
		"DIGITAL_SIGNATURE": {}, "NON_REPUDIATION": {}, "KEY_ENCIPHERMENT": {},
		"DATA_ENCIPHERMENT": {}, "KEY_AGREEMENT": {}, "CERTIFICATE_SIGNING": {},
		"CRL_SIGNING": {}, "ENCIPHER_ONLY": {}, "DECIPHER_ONLY": {}, "ANY": {}, "CUSTOM": {},
	}
	validExtendedKeyUsageNames = map[string]struct{}{
		"TLS_WEB_SERVER_AUTHENTICATION": {}, "TLS_WEB_CLIENT_AUTHENTICATION": {},
		"CODE_SIGNING": {}, "EMAIL_PROTECTION": {}, "TIME_STAMPING": {}, "OCSP_SIGNING": {},
		"IPSEC_END_SYSTEM": {}, "IPSEC_TUNNEL": {}, "IPSEC_USER": {}, "ANY": {},
		"NONE": {}, "CUSTOM": {},
	}
)

// validateListCertificatesParams rejects CertificateStatuses/Includes/SortBy/
// SortOrder values outside their real enums with ErrInvalidArgs
// (InvalidArgsException) -- the only invalid-input error ListCertificates'
// deserializer recognizes (see ErrInvalidArgs). Real ListCertificates'
// SortBy enum (types.SortBy, enums.go:692-697) has exactly one value,
// CREATED_AT.
func validateListCertificatesParams(p ListCertificatesParams) error {
	for _, s := range p.StatusFilter {
		if _, ok := validCertificateStatuses[s]; !ok {
			return fmt.Errorf("%w: invalid CertificateStatuses value %q", ErrInvalidArgs, s)
		}
	}

	for _, k := range p.KeyTypes {
		if _, ok := validListCertKeyAlgorithms[k]; !ok {
			return fmt.Errorf("%w: invalid Includes.KeyTypes value %q", ErrInvalidArgs, k)
		}
	}

	for _, ku := range p.KeyUsage {
		if _, ok := validKeyUsageNames[ku]; !ok {
			return fmt.Errorf("%w: invalid Includes.KeyUsage value %q", ErrInvalidArgs, ku)
		}
	}

	for _, eku := range p.ExtendedKeyUsage {
		if _, ok := validExtendedKeyUsageNames[eku]; !ok {
			return fmt.Errorf("%w: invalid Includes.ExtendedKeyUsage value %q", ErrInvalidArgs, eku)
		}
	}

	if p.SortBy != "" && p.SortBy != listCertSortByCreatedAt {
		return fmt.Errorf("%w: invalid SortBy value %q", ErrInvalidArgs, p.SortBy)
	}

	if p.SortOrder != "" && p.SortOrder != "ASCENDING" && p.SortOrder != "DESCENDING" {
		return fmt.Errorf("%w: invalid SortOrder value %q", ErrInvalidArgs, p.SortOrder)
	}

	return nil
}

// randHex returns a random lowercase hex string of length validationTokenLen
// characters. Every call site (certificate DNS validation tokens here and
// ACME domain-validation prevalidation tokens in acme_domain_validations.go)
// wants the same length, so it is a constant rather than a parameter.
func randHex() (string, error) {
	const n = validationTokenLen

	b := make([]byte, (n+randByteDivisor-1)/randByteDivisor)
	if _, err := cryptorand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand read failed: %w", err)
	}

	return hex.EncodeToString(b)[:n], nil
}
