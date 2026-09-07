package acm

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// Constants shared by the ACME resource families (endpoints, external
// account bindings, accounts, domain validations). See acme_endpoints.go,
// acme_eab.go, acme_accounts.go, and acme_domain_validations.go.
const (
	acmeAuthorizationBehaviorPreApproved = "PRE_APPROVED"

	acmeContactRequired    = "REQUIRED"
	acmeContactNotRequired = "NOT_REQUIRED"

	acmeEndpointStatusActive = "ACTIVE"

	acmeAccountStatusValid       = "VALID"
	acmeAccountStatusDeactivated = "DEACTIVATED"
	acmeAccountStatusRevoked     = "REVOKED"

	// acmeDomainValidationStatusValidating is the only status gopherstack ever
	// assigns: it has no real DNS/HTTP resolver to check the prevalidation
	// resource record against, so claiming VALID (or INVALID) here would
	// fabricate a verification that never happened. See
	// acme_domain_validations.go.
	acmeDomainValidationStatusValidating = "VALIDATING"
	acmeDomainValidationStatusDeleting   = "DELETING"

	prevalidationTypeDNS = "DNS_PREVALIDATION"

	domainScopeEnabled  = "ENABLED"
	domainScopeDisabled = "DISABLED"

	timeTypeMinutes = "MINUTES"
	timeTypeHours   = "HOURS"
	timeTypeDays    = "DAYS"

	acmeResourceTypeEndpoint = "acme-endpoint"
	acmeResourceTypeEAB      = "acme-external-account-binding"
	acmeResourceTypeDomainV  = "acme-domain-validation"
	acmeResourceTypeCert     = "certificate"

	// maxTagsPerResource mirrors the 50-tag cap AWS documents for both
	// certificate tags (reservedTagPrefix's maxTagsPerCertificate in
	// handler_tags.go) and every new ACME resource's Tags/CertificateTags
	// input.
	maxTagsPerResource = 50
)

// acmeARNIDPart matches a single synthetic resource-id path segment (the
// same alphanumeric-plus-punctuation character class the real ACM ARN
// patterns document, e.g. `arn:aws[a-z-]*:acm:[a-z0-9-]+:[0-9]{12}:acme-
// endpoint/[a-zA-Z0-9-]+`, narrowed like certArnPattern to the ARN shapes
// gopherstack actually generates).
const acmeARNIDPart = `[\w+=,.@-]+`

// acmeEndpointArnPattern matches an ACME endpoint ARN:
// arn:<partition>:acm:<region>:<account>:acme-endpoint/<id>.
var acmeEndpointArnPattern = regexp.MustCompile(
	`^arn:[\w+=/,.@-]+:acm:[\w+=/,.@-]*:[0-9]+:acme-endpoint/` + acmeARNIDPart + `$`,
)

// acmeEABArnPattern matches an ACME external account binding ARN, nested
// under its owning endpoint:
// arn:<partition>:acm:<region>:<account>:acme-endpoint/<epID>/acme-external-account-binding/<id>.
var acmeEABArnPattern = regexp.MustCompile(
	`^arn:[\w+=/,.@-]+:acm:[\w+=/,.@-]*:[0-9]+:acme-endpoint/` + acmeARNIDPart +
		`/acme-external-account-binding/` + acmeARNIDPart + `$`,
)

// acmeDomainValidationArnPattern matches an ACME domain validation ARN,
// nested under its owning endpoint:
// arn:<partition>:acm:<region>:<account>:acme-endpoint/<epID>/acme-domain-validation/<id>.
var acmeDomainValidationArnPattern = regexp.MustCompile(
	`^arn:[\w+=/,.@-]+:acm:[\w+=/,.@-]*:[0-9]+:acme-endpoint/` + acmeARNIDPart +
		`/acme-domain-validation/` + acmeARNIDPart + `$`,
)

// genericAcmResourceArnPattern is the permissive shape ListTagsForResource/
// TagResource/UntagResource validate ResourceArn against on the real API --
// see the documented pattern
// `arn:[\w+=/,.@-]+:acm:[\w+=/,.@-]*:[0-9]+:[\w+=,.@-]+(/[\w+=,.@-]+)*`,
// which (unlike CertificateArn's dedicated InvalidArnException) is enforced
// as a plain ValidationException per those three ops' documented Errors
// sections -- see handler_resource_tags.go.
var genericAcmResourceArnPattern = regexp.MustCompile(
	`^arn:[\w+=/,.@-]+:acm:[\w+=/,.@-]*:[0-9]+:[\w+=,.@-]+(/[\w+=,.@-]+)*$`,
)

// validateAcmeEndpointArn checks that a non-empty AcmeEndpointArn matches the
// real ACM ARN shape (empty is left to the caller's own required-field
// check). Unlike validateCertArn's CertificateArn, none of the ACME-family
// ops (Create/Delete/Describe/List/Revoke/UpdateAcme*) declare
// InvalidArnException -- their deserializers recognize only
// ValidationException for a malformed ARN, so a bad shape here must return
// ErrInvalidParameter, not ErrInvalidArn -- gopherstack-ftkd.
func validateAcmeEndpointArn(v string) error {
	if v == "" {
		return nil
	}

	if !acmeEndpointArnPattern.MatchString(v) {
		return fmt.Errorf("%w: %q is not a valid ACME endpoint ARN", ErrInvalidParameter, v)
	}

	return nil
}

// validateAcmeEABArn: see validateAcmeEndpointArn's doc -- same
// ValidationException-only declared model, gopherstack-ftkd.
func validateAcmeEABArn(v string) error {
	if v == "" {
		return nil
	}

	if !acmeEABArnPattern.MatchString(v) {
		return fmt.Errorf("%w: %q is not a valid ACME external account binding ARN", ErrInvalidParameter, v)
	}

	return nil
}

// validateAcmeDomainValidationArn: see validateAcmeEndpointArn's doc -- same
// ValidationException-only declared model, gopherstack-ftkd.
func validateAcmeDomainValidationArn(v string) error {
	if v == "" {
		return nil
	}

	if !acmeDomainValidationArnPattern.MatchString(v) {
		return fmt.Errorf("%w: %q is not a valid ACME domain validation ARN", ErrInvalidParameter, v)
	}

	return nil
}

// acmeIdempotencyEntry records the ARN and an identity fingerprint (a
// caller-supplied-field summary distinguishing "same request replayed" from
// "same token, different request") for a Create* call made with a given
// IdempotencyToken. Shared shape for the endpoint/EAB/domain-validation
// families -- see checkAcmeIdempotency below.
type acmeIdempotencyEntry struct {
	CreatedAt   time.Time `json:"createdAt"`
	ARN         string    `json:"arn"`
	Fingerprint string    `json:"fingerprint"`
}

// checkAcmeIdempotency looks up token in store (a region-scoped
// map[token]acmeIdempotencyEntry) and reports whether the call is a
// dedupe-able replay. A token reused with a different fingerprint returns
// ErrConflict (ConflictException), matching the real API's documented
// "You are trying to update a resource or configuration that is already
// being created or updated" behavior for a mismatched retry.
func checkAcmeIdempotency(
	store map[string]acmeIdempotencyEntry, token, fingerprint string,
) (string, bool, error) {
	if token == "" {
		return "", false, nil
	}

	entry, ok := store[token]
	if !ok {
		return "", false, nil
	}

	if entry.Fingerprint != fingerprint {
		return "", false, fmt.Errorf(
			"%w: IdempotencyToken %q was already used with a different request", ErrConflict, token,
		)
	}

	return entry.ARN, true, nil
}

// listOwnedByEndpoint is the shared "list resources owned by an ACME
// endpoint" implementation for ListAcmeExternalAccountBindings
// (acme_eab.go) and ListAcmeDomainValidations (acme_domain_validations.go):
// validate NextToken, validate the endpoint exists in-region, copy every
// entry idx groups under epARN, sort by ARN, and paginate. Factored out as a
// package-level generic function (Go does not allow type parameters on
// methods) since the two call sites were otherwise identical but for their
// value type.
func listOwnedByEndpoint[V any](
	ctx context.Context, b *InMemoryBackend, lockLabel, epARN, nextToken string, maxResults int,
	idx *store.Index[V], copyFn func(*V) V, arnOf func(V) string,
) (page.Page[V], error) {
	if err := page.ValidateToken(nextToken); err != nil {
		return page.Page[V]{}, fmt.Errorf("%w: invalid NextToken", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock(lockLabel)
	defer b.mu.RUnlock()

	ep, ok := b.endpoints.Get(epARN)
	if !ok || ep.Region != region {
		return page.Page[V]{}, fmt.Errorf("%w: ACME endpoint %s not found", ErrAcmeResourceNotFound, epARN)
	}

	owned := idx.Get(epARN)
	out := make([]V, 0, len(owned))

	for _, v := range owned {
		out = append(out, copyFn(v))
	}

	slices.SortFunc(out, func(a, c V) int { return strings.Compare(arnOf(a), arnOf(c)) })

	return page.New(out, nextToken, maxResults, acmDefaultMaxItems), nil
}
