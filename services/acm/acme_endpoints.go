package acm

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// validPublicKeyAlgorithms are the AWS-defined AllowedKeyAlgorithms values
// for a PublicCertificateAuthority (types.PublicKeyAlgorithm in the real
// SDK).
//
//nolint:gochecknoglobals // read-only enum value list initialized once at startup
var validPublicKeyAlgorithms = []string{keyAlgorithmRSA2048, keyAlgorithmEC, keyAlgorithmECSecp384r1}

// AcmeEndpoint is a managed ACME server (RFC 8555) endpoint. It is the root
// of the ACME resource family: external account bindings (acme_eab.go),
// ACME accounts (acme_accounts.go), and domain validations
// (acme_domain_validations.go) all belong to exactly one endpoint and are
// cascade-deleted with it -- see DeleteAcmeEndpoint below.
type AcmeEndpoint struct {
	CreatedAt             time.Time    `json:"createdAt"`
	UpdatedAt             time.Time    `json:"updatedAt"`
	ARN                   string       `json:"arn"`
	Region                string       `json:"region"`
	AuthorizationBehavior string       `json:"authorizationBehavior"`
	Contact               string       `json:"contact,omitempty"`
	EndpointURL           string       `json:"endpointUrl"`
	Status                string       `json:"status"`
	FailureReason         string       `json:"failureReason,omitempty"`
	IdempotencyToken      string       `json:"idempotencyToken,omitempty"`
	AllowedKeyAlgorithms  []string     `json:"allowedKeyAlgorithms,omitempty"`
	CertificateTags       []svcTags.KV `json:"certificateTags,omitempty"`
}

func acmeEndpointKeyFn(v *AcmeEndpoint) string         { return v.ARN }
func acmeEndpointRegionIndexFn(v *AcmeEndpoint) string { return v.Region }

func copyAcmeEndpoint(v *AcmeEndpoint) AcmeEndpoint {
	cp := *v
	if len(v.AllowedKeyAlgorithms) > 0 {
		cp.AllowedKeyAlgorithms = append([]string(nil), v.AllowedKeyAlgorithms...)
	}

	if len(v.CertificateTags) > 0 {
		cp.CertificateTags = append([]svcTags.KV(nil), v.CertificateTags...)
	}

	return cp
}

// validateAllowedKeyAlgorithms checks every entry against the AWS-defined
// PublicKeyAlgorithm enum.
func validateAllowedKeyAlgorithms(algs []string) error {
	for _, a := range algs {
		if !slices.Contains(validPublicKeyAlgorithms, a) {
			return fmt.Errorf("%w: %q is not a valid key algorithm", ErrInvalidParameter, a)
		}
	}

	return nil
}

// CreateAcmeEndpointParams holds the parsed CreateAcmeEndpoint input.
type CreateAcmeEndpointParams struct {
	AuthorizationBehavior string
	Contact               string
	IdempotencyToken      string
	AllowedKeyAlgorithms  []string
	CertificateTags       []svcTags.KV
}

func (p CreateAcmeEndpointParams) fingerprint() string {
	return p.AuthorizationBehavior + "|" + p.Contact + "|" + strings.Join(p.AllowedKeyAlgorithms, ",")
}

// CreateAcmeEndpoint creates a new ACME endpoint. AuthorizationBehavior must
// be PRE_APPROVED (the only value the real API defines) and a
// PublicCertificateAuthority (the only CertificateAuthority union member
// AWS defines) must have been supplied -- both checked by the caller via
// jsonCreateAcmeEndpoint before AllowedKeyAlgorithms reaches here. Endpoints
// go ACTIVE synchronously: gopherstack has no async provisioning pipeline to
// model CREATING against, and a synchronous ACTIVE result never claims a
// real ACME client interaction (directory fetch, account registration, cert
// issuance) that did not happen.
func (b *InMemoryBackend) CreateAcmeEndpoint(
	ctx context.Context, p CreateAcmeEndpointParams,
) (*AcmeEndpoint, error) {
	if p.AuthorizationBehavior != acmeAuthorizationBehaviorPreApproved {
		return nil, fmt.Errorf("%w: AuthorizationBehavior must be PRE_APPROVED", ErrInvalidParameter)
	}

	if p.Contact != "" && p.Contact != acmeContactRequired && p.Contact != acmeContactNotRequired {
		return nil, fmt.Errorf("%w: invalid Contact value %q", ErrInvalidParameter, p.Contact)
	}

	if err := validateAllowedKeyAlgorithms(p.AllowedKeyAlgorithms); err != nil {
		return nil, err
	}

	if len(p.CertificateTags) > maxTagsPerResource {
		return nil, fmt.Errorf("%w: maximum of 50 CertificateTags allowed", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateAcmeEndpoint")
	defer b.mu.Unlock()

	if existingARN, found, err := checkAcmeIdempotency(
		b.endpointIdempotencyStore(region), p.IdempotencyToken, p.fingerprint(),
	); err != nil {
		return nil, err
	} else if found {
		if ep, ok := b.endpoints.Get(existingARN); ok {
			cp := copyAcmeEndpoint(ep)

			return &cp, nil
		}
	}

	id := fmt.Sprintf("%x", time.Now().UnixNano())
	epARN := arn.Build("acm", region, b.accountID, acmeResourceTypeEndpoint+"/"+id)
	now := time.Now().UTC()

	ep := &AcmeEndpoint{
		ARN:                   epARN,
		Region:                region,
		AuthorizationBehavior: p.AuthorizationBehavior,
		Contact:               p.Contact,
		AllowedKeyAlgorithms:  p.AllowedKeyAlgorithms,
		CertificateTags:       p.CertificateTags,
		EndpointURL:           "https://acm-acme." + region + ".amazonaws.com/directory/" + id,
		Status:                acmeEndpointStatusActive,
		CreatedAt:             now,
		UpdatedAt:             now,
		IdempotencyToken:      p.IdempotencyToken,
	}
	b.endpoints.Put(ep)

	if p.IdempotencyToken != "" {
		b.endpointIdempotencyStore(region)[p.IdempotencyToken] = acmeIdempotencyEntry{
			ARN: epARN, Fingerprint: p.fingerprint(), CreatedAt: now,
		}
	}

	cp := copyAcmeEndpoint(ep)

	return &cp, nil
}

// DescribeAcmeEndpoint returns the endpoint with the given ARN.
func (b *InMemoryBackend) DescribeAcmeEndpoint(ctx context.Context, epARN string) (*AcmeEndpoint, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeAcmeEndpoint")
	defer b.mu.RUnlock()

	ep, ok := b.endpoints.Get(epARN)
	if !ok || ep.Region != region {
		return nil, fmt.Errorf("%w: ACME endpoint %s not found", ErrAcmeResourceNotFound, epARN)
	}

	cp := copyAcmeEndpoint(ep)

	return &cp, nil
}

// EndpointExists reports whether an ACME endpoint with the given ARN exists
// in the request region. Used by sibling families (EAB, domain validation,
// generic resource tagging) to validate ownership FKs.
func (b *InMemoryBackend) EndpointExists(ctx context.Context, epARN string) bool {
	region := getRegion(ctx, b.region)

	b.mu.RLock("EndpointExists")
	defer b.mu.RUnlock()

	ep, ok := b.endpoints.Get(epARN)

	return ok && ep.Region == region
}

// ListAcmeEndpoints returns a paginated list of ACME endpoints in the
// request region, sorted by ARN for stable ordering.
func (b *InMemoryBackend) ListAcmeEndpoints(
	ctx context.Context, nextToken string, maxResults int,
) (page.Page[AcmeEndpoint], error) {
	if err := page.ValidateToken(nextToken); err != nil {
		return page.Page[AcmeEndpoint]{}, fmt.Errorf("%w: invalid NextToken", ErrInvalidParameter)
	}

	region := getRegion(ctx, b.region)

	b.mu.RLock("ListAcmeEndpoints")
	defer b.mu.RUnlock()

	regionEPs := b.endpointsByRegion.Get(region)
	eps := make([]AcmeEndpoint, 0, len(regionEPs))

	for _, ep := range regionEPs {
		eps = append(eps, copyAcmeEndpoint(ep))
	}

	slices.SortFunc(eps, func(a, b AcmeEndpoint) int { return strings.Compare(a.ARN, b.ARN) })

	return page.New(eps, nextToken, maxResults, acmDefaultMaxItems), nil
}

// UpdateAcmeEndpointParams holds the parsed UpdateAcmeEndpoint input. Every
// field is optional on the real wire; an empty/nil value means "leave
// unchanged" (see jsonUpdateAcmeEndpoint, which only sets a field here when
// the caller's JSON body actually included it).
type UpdateAcmeEndpointParams struct {
	AuthorizationBehavior *string
	Contact               *string
	AllowedKeyAlgorithms  *[]string
}

// UpdateAcmeEndpoint applies a partial update to an existing endpoint.
func (b *InMemoryBackend) UpdateAcmeEndpoint(
	ctx context.Context, epARN string, p UpdateAcmeEndpointParams,
) error {
	if p.AuthorizationBehavior != nil && *p.AuthorizationBehavior != acmeAuthorizationBehaviorPreApproved {
		return fmt.Errorf("%w: AuthorizationBehavior must be PRE_APPROVED", ErrInvalidParameter)
	}

	if p.Contact != nil && *p.Contact != acmeContactRequired && *p.Contact != acmeContactNotRequired {
		return fmt.Errorf("%w: invalid Contact value %q", ErrInvalidParameter, *p.Contact)
	}

	if p.AllowedKeyAlgorithms != nil {
		if err := validateAllowedKeyAlgorithms(*p.AllowedKeyAlgorithms); err != nil {
			return err
		}
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateAcmeEndpoint")
	defer b.mu.Unlock()

	ep, ok := b.endpoints.Get(epARN)
	if !ok || ep.Region != region {
		return fmt.Errorf("%w: ACME endpoint %s not found", ErrAcmeResourceNotFound, epARN)
	}

	if p.AuthorizationBehavior != nil {
		ep.AuthorizationBehavior = *p.AuthorizationBehavior
	}

	if p.Contact != nil {
		ep.Contact = *p.Contact
	}

	if p.AllowedKeyAlgorithms != nil {
		ep.AllowedKeyAlgorithms = *p.AllowedKeyAlgorithms
	}

	ep.UpdatedAt = time.Now().UTC()

	return nil
}

// DeleteAcmeEndpoint removes the endpoint and cascade-deletes every external
// account binding, ACME account, and domain validation that belongs to it --
// real AWS ACME endpoints own their EABs/accounts/domain-validations (their
// ARNs are literally nested under the endpoint's), so leaving them behind as
// orphans after the parent endpoint disappears would misrepresent gopherstack's
// own resource-ownership model, not just the real API's.
//
// Unlike Describe/List/Update, Delete's deserializer declares no
// ResourceNotFoundException -- a missing ARN is ErrInvalidParameter
// (ValidationException), not ErrAcmeResourceNotFound -- gopherstack-ftkd.
func (b *InMemoryBackend) DeleteAcmeEndpoint(ctx context.Context, epARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteAcmeEndpoint")
	defer b.mu.Unlock()

	ep, ok := b.endpoints.Get(epARN)
	if !ok || ep.Region != region {
		return fmt.Errorf("%w: ACME endpoint %s not found", ErrInvalidParameter, epARN)
	}

	deletedEABs := make(map[string]struct{})
	for _, eab := range b.eabsByEndpoint.Get(epARN) {
		b.eabs.Delete(eab.ARN)
		deletedEABs[eab.ARN] = struct{}{}
	}

	deletedDVs := make(map[string]struct{})
	for _, dv := range b.domainValidationsByEndpoint.Get(epARN) {
		b.domainValidations.Delete(dv.ARN)
		deletedDVs[dv.ARN] = struct{}{}
	}

	for _, acct := range b.acmeAccountsByEndpoint.Get(epARN) {
		b.acmeAccounts.Delete(acct.key())
	}

	b.endpoints.Delete(epARN)

	// Drop idempotency-token entries for the endpoint itself and every
	// EAB/domain-validation it just cascade-deleted, so those tokens do not
	// sit orphaned (pointing at ARNs that no longer exist) until the
	// janitor's next TTL sweep -- same reasoning as DeleteCertificate's own
	// idempotency-entry cleanup.
	for tok, entry := range b.endpointIdempotencyStore(region) {
		if entry.ARN == epARN {
			delete(b.endpointIdempotency[region], tok)
		}
	}

	for tok, entry := range b.eabIdempotencyStore(region) {
		if _, deleted := deletedEABs[entry.ARN]; deleted {
			delete(b.eabIdempotency[region], tok)
		}
	}

	for tok, entry := range b.domainValidationIdempotencyStore(region) {
		if _, deleted := deletedDVs[entry.ARN]; deleted {
			delete(b.domainValidationIdempotency[region], tok)
		}
	}

	return nil
}
