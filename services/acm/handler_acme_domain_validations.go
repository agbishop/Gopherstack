package acm

import (
	"context"
	"encoding/json"
	"fmt"

	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

type domainScopeWire struct {
	ExactDomain string `json:"ExactDomain,omitempty"`
	Subdomains  string `json:"Subdomains,omitempty"`
	Wildcards   string `json:"Wildcards,omitempty"`
}

type dnsPrevalidationOptionsWire struct {
	DomainScope  *domainScopeWire `json:"DomainScope,omitempty"`
	HostedZoneID string           `json:"HostedZoneId,omitempty"`
}

type prevalidationOptionsWire struct {
	DNSPrevalidation *dnsPrevalidationOptionsWire `json:"DnsPrevalidation,omitempty"`
}

// dnsPrevalidationDetailsWire additionally carries the synthesized
// ResourceRecord, matching the real API's PrevalidationDetails (distinct
// from the input-only PrevalidationOptions shape, which has no
// ResourceRecord member).
type dnsPrevalidationDetailsWire struct {
	DomainScope    *domainScopeWire `json:"DomainScope,omitempty"`
	ResourceRecord *resourceRecord  `json:"ResourceRecord,omitempty"`
	HostedZoneID   string           `json:"HostedZoneId,omitempty"`
}

type prevalidationDetailsWire struct {
	DNSPrevalidation *dnsPrevalidationDetailsWire `json:"DnsPrevalidation,omitempty"`
}

type acmeDomainValidationWire struct {
	PrevalidationDetails    *prevalidationDetailsWire `json:"PrevalidationDetails,omitempty"`
	AcmeDomainValidationArn string                    `json:"AcmeDomainValidationArn"`
	AcmeEndpointArn         string                    `json:"AcmeEndpointArn"`
	DomainName              string                    `json:"DomainName"`
	PrevalidationType       string                    `json:"PrevalidationType"`
	Status                  string                    `json:"Status"`
	CreatedAt               int64                     `json:"CreatedAt"`
	UpdatedAt               int64                     `json:"UpdatedAt"`
}

func acmeDomainValidationToWire(dv *AcmeDomainValidation) acmeDomainValidationWire {
	var rr *resourceRecord
	if dv.ResourceRecord != nil {
		rr = &resourceRecord{Name: dv.ResourceRecord.Name, Type: dv.ResourceRecord.Type, Value: dv.ResourceRecord.Value}
	}

	return acmeDomainValidationWire{
		AcmeDomainValidationArn: dv.ARN,
		AcmeEndpointArn:         dv.AcmeEndpointArn,
		DomainName:              dv.DomainName,
		PrevalidationType:       dv.PrevalidationType,
		Status:                  dv.Status,
		CreatedAt:               dv.CreatedAt.Unix(),
		UpdatedAt:               dv.UpdatedAt.Unix(),
		PrevalidationDetails: &prevalidationDetailsWire{
			DNSPrevalidation: &dnsPrevalidationDetailsWire{
				DomainScope: &domainScopeWire{
					ExactDomain: dv.DomainScopeExact,
					Subdomains:  dv.DomainScopeSub,
					Wildcards:   dv.DomainScopeWild,
				},
				HostedZoneID:   dv.HostedZoneID,
				ResourceRecord: rr,
			},
		},
	}
}

// dnsPrevalidationFromWire converts the wire PrevalidationOptions.DnsPrevalidation
// member to the backend's DNSPrevalidationParams, or nil if absent.
func dnsPrevalidationFromWire(p *prevalidationOptionsWire) *DNSPrevalidationParams {
	if p == nil || p.DNSPrevalidation == nil {
		return nil
	}

	out := &DNSPrevalidationParams{HostedZoneID: p.DNSPrevalidation.HostedZoneID}
	if p.DNSPrevalidation.DomainScope != nil {
		out.DomainScopeExact = p.DNSPrevalidation.DomainScope.ExactDomain
		out.DomainScopeSub = p.DNSPrevalidation.DomainScope.Subdomains
		out.DomainScopeWild = p.DNSPrevalidation.DomainScope.Wildcards
	}

	return out
}

type createAcmeDomainValidationInput struct {
	PrevalidationOptions *prevalidationOptionsWire `json:"PrevalidationOptions"`
	AcmeEndpointArn      string                    `json:"AcmeEndpointArn"`
	DomainName           string                    `json:"DomainName"`
	IdempotencyToken     string                    `json:"IdempotencyToken"`
	Tags                 []svcTags.KV              `json:"Tags"`
}

type createAcmeDomainValidationOutput struct {
	AcmeDomainValidationArn string `json:"AcmeDomainValidationArn"`
}

func (h *Handler) jsonCreateAcmeDomainValidation(ctx context.Context, body []byte) (any, error) {
	var input createAcmeDomainValidationInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	dv, err := h.Backend.CreateAcmeDomainValidation(ctx, CreateAcmeDomainValidationParams{
		AcmeEndpointArn:  input.AcmeEndpointArn,
		DomainName:       input.DomainName,
		IdempotencyToken: input.IdempotencyToken,
		DNSPrevalidation: dnsPrevalidationFromWire(input.PrevalidationOptions),
		Tags:             input.Tags,
	})
	if err != nil {
		return nil, err
	}

	if len(input.Tags) > 0 {
		kv := make(map[string]string, len(input.Tags))
		for _, t := range input.Tags {
			kv[t.Key] = t.Value
		}

		if tagErr := h.setTags(dv.ARN, kv, ErrInvalidParameter, ErrServiceQuotaExceeded); tagErr != nil {
			return nil, tagErr
		}
	}

	return &createAcmeDomainValidationOutput{AcmeDomainValidationArn: dv.ARN}, nil
}

type describeAcmeDomainValidationInput struct {
	AcmeDomainValidationArn string `json:"AcmeDomainValidationArn"`
}

type describeAcmeDomainValidationOutput struct {
	AcmeDomainValidation *acmeDomainValidationWire `json:"AcmeDomainValidation"`
}

func (h *Handler) jsonDescribeAcmeDomainValidation(ctx context.Context, body []byte) (any, error) {
	var input describeAcmeDomainValidationInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeDomainValidationArn(input.AcmeDomainValidationArn); err != nil {
		return nil, err
	}

	if input.AcmeDomainValidationArn == "" {
		return nil, fmt.Errorf("%w: AcmeDomainValidationArn is required", ErrInvalidParameter)
	}

	dv, err := h.Backend.DescribeAcmeDomainValidation(ctx, input.AcmeDomainValidationArn)
	if err != nil {
		return nil, err
	}

	wire := acmeDomainValidationToWire(dv)

	return &describeAcmeDomainValidationOutput{AcmeDomainValidation: &wire}, nil
}

type deleteAcmeDomainValidationInput struct {
	AcmeDomainValidationArn string `json:"AcmeDomainValidationArn"`
}

type deleteAcmeDomainValidationOutput struct{}

func (h *Handler) jsonDeleteAcmeDomainValidation(ctx context.Context, body []byte) (any, error) {
	var input deleteAcmeDomainValidationInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeDomainValidationArn(input.AcmeDomainValidationArn); err != nil {
		return nil, err
	}

	if err := h.Backend.DeleteAcmeDomainValidation(ctx, input.AcmeDomainValidationArn); err != nil {
		return nil, err
	}

	h.cleanupTags(input.AcmeDomainValidationArn)

	return &deleteAcmeDomainValidationOutput{}, nil
}

type listAcmeDomainValidationsInput struct {
	AcmeEndpointArn string `json:"AcmeEndpointArn"`
	NextToken       string `json:"NextToken"`
	MaxResults      int32  `json:"MaxResults"`
}

type listAcmeDomainValidationsOutput struct {
	NextToken             string                     `json:"NextToken,omitempty"`
	AcmeDomainValidations []acmeDomainValidationWire `json:"AcmeDomainValidations"`
}

func (h *Handler) jsonListAcmeDomainValidations(ctx context.Context, body []byte) (any, error) {
	var input listAcmeDomainValidationsInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeEndpointArn(input.AcmeEndpointArn); err != nil {
		return nil, err
	}

	if input.AcmeEndpointArn == "" {
		return nil, fmt.Errorf("%w: AcmeEndpointArn is required", ErrInvalidParameter)
	}

	pg, err := h.Backend.ListAcmeDomainValidations(ctx, input.AcmeEndpointArn, input.NextToken, int(input.MaxResults))
	if err != nil {
		return nil, err
	}

	out := make([]acmeDomainValidationWire, 0, len(pg.Data))
	for i := range pg.Data {
		out = append(out, acmeDomainValidationToWire(&pg.Data[i]))
	}

	return &listAcmeDomainValidationsOutput{AcmeDomainValidations: out, NextToken: pg.Next}, nil
}

type updateAcmeDomainValidationInput struct {
	PrevalidationOptions    *prevalidationOptionsWire `json:"PrevalidationOptions"`
	AcmeDomainValidationArn string                    `json:"AcmeDomainValidationArn"`
}

type updateAcmeDomainValidationOutput struct{}

func (h *Handler) jsonUpdateAcmeDomainValidation(ctx context.Context, body []byte) (any, error) {
	var input updateAcmeDomainValidationInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeDomainValidationArn(input.AcmeDomainValidationArn); err != nil {
		return nil, err
	}

	if input.AcmeDomainValidationArn == "" {
		return nil, fmt.Errorf("%w: AcmeDomainValidationArn is required", ErrInvalidParameter)
	}

	if err := h.Backend.UpdateAcmeDomainValidation(
		ctx, input.AcmeDomainValidationArn, dnsPrevalidationFromWire(input.PrevalidationOptions),
	); err != nil {
		return nil, err
	}

	return &updateAcmeDomainValidationOutput{}, nil
}
