package acm

import (
	"context"
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// AcmeDomainValidation authorizes an ACME endpoint to issue certificates for
// a domain name via DNS prevalidation. Owned by exactly one AcmeEndpoint
// (AcmeEndpointArn), cascade-deleted with it -- see DeleteAcmeEndpoint in
// acme_endpoints.go.
//
// Status is always VALIDATING: gopherstack has no real DNS resolver to check
// the synthesized ResourceRecord against, so it never claims VALID (a
// verification that never actually happened) or INVALID. This mirrors the
// task's explicit instruction not to fabricate a validated state -- see
// PARITY.md's gaps entry for this family.
type AcmeDomainValidation struct {
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	ResourceRecord    *ResourceRecord `json:"resourceRecord,omitempty"`
	ARN               string          `json:"arn"`
	Region            string          `json:"region"`
	AcmeEndpointArn   string          `json:"acmeEndpointArn"`
	DomainName        string          `json:"domainName"`
	Status            string          `json:"status"`
	PrevalidationType string          `json:"prevalidationType"`
	DomainScopeExact  string          `json:"domainScopeExact,omitempty"`
	DomainScopeSub    string          `json:"domainScopeSub,omitempty"`
	DomainScopeWild   string          `json:"domainScopeWild,omitempty"`
	HostedZoneID      string          `json:"hostedZoneId,omitempty"`
	IdempotencyToken  string          `json:"idempotencyToken,omitempty"`
}

func acmeDomainValidationKeyFn(v *AcmeDomainValidation) string         { return v.ARN }
func acmeDomainValidationRegionIndexFn(v *AcmeDomainValidation) string { return v.Region }
func acmeDomainValidationEndpointIndexFn(v *AcmeDomainValidation) string {
	return v.AcmeEndpointArn
}

func copyAcmeDomainValidation(v *AcmeDomainValidation) AcmeDomainValidation {
	cp := *v
	if v.ResourceRecord != nil {
		rr := *v.ResourceRecord
		cp.ResourceRecord = &rr
	}

	return cp
}

// DNSPrevalidationParams is the parsed PrevalidationOptions.DnsPrevalidation
// member -- the only PrevalidationOptions union member the real API defines.
type DNSPrevalidationParams struct {
	DomainScopeExact string
	DomainScopeSub   string
	DomainScopeWild  string
	HostedZoneID     string
}

// CreateAcmeDomainValidationParams holds the parsed CreateAcmeDomainValidation input.
type CreateAcmeDomainValidationParams struct {
	AcmeEndpointArn  string
	DomainName       string
	IdempotencyToken string
	DNSPrevalidation *DNSPrevalidationParams
	Tags             []svcTags.KV
}

func (p CreateAcmeDomainValidationParams) fingerprint() string {
	var dv string
	if p.DNSPrevalidation != nil {
		dv = p.DNSPrevalidation.DomainScopeExact + "," + p.DNSPrevalidation.DomainScopeSub + "," +
			p.DNSPrevalidation.DomainScopeWild + "," + p.DNSPrevalidation.HostedZoneID
	}

	return p.AcmeEndpointArn + "|" + p.DomainName + "|" + dv
}

func validateDomainScopeOption(v string) error {
	if v == "" || v == domainScopeEnabled || v == domainScopeDisabled {
		return nil
	}

	return fmt.Errorf("%w: invalid DomainScope option %q", ErrInvalidParameter, v)
}

// buildDNSPrevalidationRecord synthesizes the CNAME resource record an ACME
// domain-validation prevalidation would ask the caller to publish, using the
// same random-token construction buildDomainValidationOptions uses for
// certificate DNS validation (certificate_validation.go), with a distinct
// well-known suffix so the two never look identical on the wire.
func buildDNSPrevalidationRecord(domain string) (*ResourceRecord, error) {
	nameToken, err := randHex()
	if err != nil {
		return nil, err
	}

	valueToken, err := randHex()
	if err != nil {
		return nil, err
	}

	return &ResourceRecord{
		Name:  "_" + nameToken + "." + domain + ".",
		Type:  "CNAME",
		Value: "_" + valueToken + ".acme-dv-validations.aws.",
	}, nil
}

// validateDNSPrevalidation checks all three DomainScope option fields of a
// DNSPrevalidationParams.
func validateDNSPrevalidation(dns *DNSPrevalidationParams) error {
	if err := validateDomainScopeOption(dns.DomainScopeExact); err != nil {
		return err
	}

	if err := validateDomainScopeOption(dns.DomainScopeSub); err != nil {
		return err
	}

	return validateDomainScopeOption(dns.DomainScopeWild)
}

// validateCreateAcmeDomainValidationParams checks the
// CreateAcmeDomainValidation input, factored out of
// CreateAcmeDomainValidation to keep that method's cyclomatic complexity
// down.
func validateCreateAcmeDomainValidationParams(p CreateAcmeDomainValidationParams) error {
	if err := validateAcmeEndpointArn(p.AcmeEndpointArn); err != nil {
		return err
	}

	if p.AcmeEndpointArn == "" {
		return fmt.Errorf("%w: AcmeEndpointArn is required", ErrInvalidParameter)
	}

	if p.DomainName == "" {
		return fmt.Errorf("%w: DomainName is required", ErrInvalidParameter)
	}

	if err := validateDomainName(p.DomainName, ErrInvalidParameter); err != nil {
		return err
	}

	if p.DNSPrevalidation == nil {
		return fmt.Errorf(
			"%w: PrevalidationOptions must specify DnsPrevalidation (the only supported member)",
			ErrInvalidParameter,
		)
	}

	if err := validateDNSPrevalidation(p.DNSPrevalidation); err != nil {
		return err
	}

	if len(p.Tags) > maxTagsPerResource {
		return fmt.Errorf("%w: maximum of 50 Tags allowed", ErrInvalidParameter)
	}

	return nil
}

// CreateAcmeDomainValidation creates a new domain validation under an
// existing endpoint.
func (b *InMemoryBackend) CreateAcmeDomainValidation(
	ctx context.Context, p CreateAcmeDomainValidationParams,
) (*AcmeDomainValidation, error) {
	if err := validateCreateAcmeDomainValidationParams(p); err != nil {
		return nil, err
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateAcmeDomainValidation")
	defer b.mu.Unlock()

	ep, ok := b.endpoints.Get(p.AcmeEndpointArn)
	if !ok || ep.Region != region {
		return nil, fmt.Errorf("%w: ACME endpoint %s not found", ErrAcmeResourceNotFound, p.AcmeEndpointArn)
	}

	if existingARN, found, err := checkAcmeIdempotency(
		b.domainValidationIdempotencyStore(region), p.IdempotencyToken, p.fingerprint(),
	); err != nil {
		return nil, err
	} else if found {
		if dv, exists := b.domainValidations.Get(existingARN); exists {
			cp := copyAcmeDomainValidation(dv)

			return &cp, nil
		}
	}

	record, err := buildDNSPrevalidationRecord(p.DomainName)
	if err != nil {
		return nil, err
	}

	id := fmt.Sprintf("%x", time.Now().UnixNano())
	dvARN := arn.Build(
		"acm", region, b.accountID,
		acmeResourceTypeEndpoint+"/"+endpointIDFromArn(p.AcmeEndpointArn)+"/"+acmeResourceTypeDomainV+"/"+id,
	)
	now := time.Now().UTC()

	dv := &AcmeDomainValidation{
		ARN:               dvARN,
		Region:            region,
		AcmeEndpointArn:   p.AcmeEndpointArn,
		DomainName:        p.DomainName,
		Status:            acmeDomainValidationStatusValidating,
		PrevalidationType: prevalidationTypeDNS,
		DomainScopeExact:  p.DNSPrevalidation.DomainScopeExact,
		DomainScopeSub:    p.DNSPrevalidation.DomainScopeSub,
		DomainScopeWild:   p.DNSPrevalidation.DomainScopeWild,
		HostedZoneID:      p.DNSPrevalidation.HostedZoneID,
		ResourceRecord:    record,
		CreatedAt:         now,
		UpdatedAt:         now,
		IdempotencyToken:  p.IdempotencyToken,
	}
	b.domainValidations.Put(dv)

	if p.IdempotencyToken != "" {
		b.domainValidationIdempotencyStore(region)[p.IdempotencyToken] = acmeIdempotencyEntry{
			ARN: dvARN, Fingerprint: p.fingerprint(), CreatedAt: now,
		}
	}

	cp := copyAcmeDomainValidation(dv)

	return &cp, nil
}

// DescribeAcmeDomainValidation returns the domain validation with the given ARN.
func (b *InMemoryBackend) DescribeAcmeDomainValidation(
	ctx context.Context, dvARN string,
) (*AcmeDomainValidation, error) {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DescribeAcmeDomainValidation")
	defer b.mu.RUnlock()

	dv, ok := b.domainValidations.Get(dvARN)
	if !ok || dv.Region != region {
		return nil, fmt.Errorf("%w: ACME domain validation %s not found", ErrAcmeResourceNotFound, dvARN)
	}

	cp := copyAcmeDomainValidation(dv)

	return &cp, nil
}

// ListAcmeDomainValidations returns a paginated list of domain validations
// owned by epARN.
func (b *InMemoryBackend) ListAcmeDomainValidations(
	ctx context.Context, epARN, nextToken string, maxResults int,
) (page.Page[AcmeDomainValidation], error) {
	return listOwnedByEndpoint(
		ctx, b, "ListAcmeDomainValidations", epARN, nextToken, maxResults,
		b.domainValidationsByEndpoint, copyAcmeDomainValidation, func(v AcmeDomainValidation) string { return v.ARN },
	)
}

// UpdateAcmeDomainValidation replaces an existing domain validation's
// prevalidation options (the only field the real API allows updating),
// regenerating its DNS resource record. Status is left at VALIDATING for the
// same reason CreateAcmeDomainValidation never sets anything else.
func (b *InMemoryBackend) UpdateAcmeDomainValidation(
	ctx context.Context, dvARN string, dns *DNSPrevalidationParams,
) error {
	if dns != nil {
		if err := validateDNSPrevalidation(dns); err != nil {
			return err
		}
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("UpdateAcmeDomainValidation")
	defer b.mu.Unlock()

	dv, ok := b.domainValidations.Get(dvARN)
	if !ok || dv.Region != region {
		return fmt.Errorf("%w: ACME domain validation %s not found", ErrAcmeResourceNotFound, dvARN)
	}

	if dns != nil {
		record, err := buildDNSPrevalidationRecord(dv.DomainName)
		if err != nil {
			return err
		}

		dv.DomainScopeExact = dns.DomainScopeExact
		dv.DomainScopeSub = dns.DomainScopeSub
		dv.DomainScopeWild = dns.DomainScopeWild
		dv.HostedZoneID = dns.HostedZoneID
		dv.ResourceRecord = record
	}

	dv.UpdatedAt = time.Now().UTC()

	return nil
}

// DeleteAcmeDomainValidation removes the domain validation with the given
// ARN. Unlike Describe/List/Update, Delete's deserializer declares no
// ResourceNotFoundException -- a missing ARN is ErrInvalidParameter
// (ValidationException), not ErrAcmeResourceNotFound -- gopherstack-ftkd.
func (b *InMemoryBackend) DeleteAcmeDomainValidation(ctx context.Context, dvARN string) error {
	region := getRegion(ctx, b.region)

	b.mu.Lock("DeleteAcmeDomainValidation")
	defer b.mu.Unlock()

	dv, ok := b.domainValidations.Get(dvARN)
	if !ok || dv.Region != region {
		return fmt.Errorf("%w: ACME domain validation %s not found", ErrInvalidParameter, dvARN)
	}

	b.domainValidations.Delete(dvARN)

	for tok, entry := range b.domainValidationIdempotencyStore(region) {
		if entry.ARN == dvARN {
			delete(b.domainValidationIdempotency[region], tok)
		}
	}

	return nil
}

// DomainValidationExists reports whether a domain validation with the given
// ARN exists in the request region. Used by generic resource tagging
// (handler_resource_tags.go).
func (b *InMemoryBackend) DomainValidationExists(ctx context.Context, dvARN string) bool {
	region := getRegion(ctx, b.region)

	b.mu.RLock("DomainValidationExists")
	defer b.mu.RUnlock()

	dv, ok := b.domainValidations.Get(dvARN)

	return ok && dv.Region == region
}
