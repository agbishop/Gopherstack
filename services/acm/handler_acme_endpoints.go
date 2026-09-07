package acm

import (
	"context"
	"encoding/json"
	"fmt"

	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// publicCertificateAuthorityWire is the only CertificateAuthority union
// member the real API defines (types.CertificateAuthorityMemberPublicCertificateAuthority).
type publicCertificateAuthorityWire struct {
	AllowedKeyAlgorithms []string `json:"AllowedKeyAlgorithms,omitempty"`
}

type certificateAuthorityWire struct {
	PublicCertificateAuthority *publicCertificateAuthorityWire `json:"PublicCertificateAuthority,omitempty"`
}

type acmeEndpointWire struct {
	CertificateAuthority  *certificateAuthorityWire `json:"CertificateAuthority,omitempty"`
	AcmeEndpointArn       string                    `json:"AcmeEndpointArn"`
	AuthorizationBehavior string                    `json:"AuthorizationBehavior"`
	Contact               string                    `json:"Contact,omitempty"`
	EndpointURL           string                    `json:"EndpointUrl"`
	Status                string                    `json:"Status"`
	FailureReason         string                    `json:"FailureReason,omitempty"`
	CertificateTags       []svcTags.KV              `json:"CertificateTags,omitempty"`
	CreatedAt             int64                     `json:"CreatedAt"`
	UpdatedAt             int64                     `json:"UpdatedAt"`
}

func acmeEndpointToWire(ep *AcmeEndpoint) acmeEndpointWire {
	return acmeEndpointWire{
		AcmeEndpointArn:       ep.ARN,
		AuthorizationBehavior: ep.AuthorizationBehavior,
		CertificateAuthority: &certificateAuthorityWire{
			PublicCertificateAuthority: &publicCertificateAuthorityWire{
				AllowedKeyAlgorithms: ep.AllowedKeyAlgorithms,
			},
		},
		CertificateTags: ep.CertificateTags,
		Contact:         ep.Contact,
		EndpointURL:     ep.EndpointURL,
		Status:          ep.Status,
		FailureReason:   ep.FailureReason,
		CreatedAt:       ep.CreatedAt.Unix(),
		UpdatedAt:       ep.UpdatedAt.Unix(),
	}
}

type createAcmeEndpointInput struct {
	CertificateAuthority  *certificateAuthorityWire `json:"CertificateAuthority"`
	AuthorizationBehavior string                    `json:"AuthorizationBehavior"`
	Contact               string                    `json:"Contact"`
	IdempotencyToken      string                    `json:"IdempotencyToken"`
	CertificateTags       []svcTags.KV              `json:"CertificateTags"`
	Tags                  []svcTags.KV              `json:"Tags"`
}

type createAcmeEndpointOutput struct {
	AcmeEndpointArn string `json:"AcmeEndpointArn"`
}

func (h *Handler) jsonCreateAcmeEndpoint(ctx context.Context, body []byte) (any, error) {
	var input createAcmeEndpointInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if input.CertificateAuthority == nil || input.CertificateAuthority.PublicCertificateAuthority == nil {
		return nil, fmt.Errorf(
			"%w: CertificateAuthority must specify PublicCertificateAuthority (the only supported member)",
			ErrInvalidParameter,
		)
	}

	ep, err := h.Backend.CreateAcmeEndpoint(ctx, CreateAcmeEndpointParams{
		AuthorizationBehavior: input.AuthorizationBehavior,
		Contact:               input.Contact,
		IdempotencyToken:      input.IdempotencyToken,
		AllowedKeyAlgorithms:  input.CertificateAuthority.PublicCertificateAuthority.AllowedKeyAlgorithms,
		CertificateTags:       input.CertificateTags,
	})
	if err != nil {
		return nil, err
	}

	if len(input.Tags) > 0 {
		kv := make(map[string]string, len(input.Tags))
		for _, t := range input.Tags {
			kv[t.Key] = t.Value
		}

		if tagErr := h.setTags(ep.ARN, kv, ErrInvalidParameter, ErrServiceQuotaExceeded); tagErr != nil {
			return nil, tagErr
		}
	}

	return &createAcmeEndpointOutput{AcmeEndpointArn: ep.ARN}, nil
}

type describeAcmeEndpointInput struct {
	AcmeEndpointArn string `json:"AcmeEndpointArn"`
}

type describeAcmeEndpointOutput struct {
	AcmeEndpoint *acmeEndpointWire `json:"AcmeEndpoint"`
}

func (h *Handler) jsonDescribeAcmeEndpoint(ctx context.Context, body []byte) (any, error) {
	var input describeAcmeEndpointInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeEndpointArn(input.AcmeEndpointArn); err != nil {
		return nil, err
	}

	if input.AcmeEndpointArn == "" {
		return nil, fmt.Errorf("%w: AcmeEndpointArn is required", ErrInvalidParameter)
	}

	ep, err := h.Backend.DescribeAcmeEndpoint(ctx, input.AcmeEndpointArn)
	if err != nil {
		return nil, err
	}

	wire := acmeEndpointToWire(ep)

	return &describeAcmeEndpointOutput{AcmeEndpoint: &wire}, nil
}

type deleteAcmeEndpointInput struct {
	AcmeEndpointArn string `json:"AcmeEndpointArn"`
}

type deleteAcmeEndpointOutput struct{}

func (h *Handler) jsonDeleteAcmeEndpoint(ctx context.Context, body []byte) (any, error) {
	var input deleteAcmeEndpointInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeEndpointArn(input.AcmeEndpointArn); err != nil {
		return nil, err
	}

	if err := h.Backend.DeleteAcmeEndpoint(ctx, input.AcmeEndpointArn); err != nil {
		return nil, err
	}

	h.cleanupTags(input.AcmeEndpointArn)

	return &deleteAcmeEndpointOutput{}, nil
}

type listAcmeEndpointsInput struct {
	NextToken  string `json:"NextToken"`
	MaxResults int32  `json:"MaxResults"`
}

type listAcmeEndpointsOutput struct {
	NextToken     string             `json:"NextToken,omitempty"`
	AcmeEndpoints []acmeEndpointWire `json:"AcmeEndpoints"`
}

func (h *Handler) jsonListAcmeEndpoints(ctx context.Context, body []byte) (any, error) {
	var input listAcmeEndpointsInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	pg, err := h.Backend.ListAcmeEndpoints(ctx, input.NextToken, int(input.MaxResults))
	if err != nil {
		return nil, err
	}

	out := make([]acmeEndpointWire, 0, len(pg.Data))
	for i := range pg.Data {
		out = append(out, acmeEndpointToWire(&pg.Data[i]))
	}

	return &listAcmeEndpointsOutput{AcmeEndpoints: out, NextToken: pg.Next}, nil
}

type updateAcmeEndpointInput struct {
	CertificateAuthority  *certificateAuthorityWire `json:"CertificateAuthority"`
	AcmeEndpointArn       string                    `json:"AcmeEndpointArn"`
	AuthorizationBehavior string                    `json:"AuthorizationBehavior"`
	Contact               string                    `json:"Contact"`
}

type updateAcmeEndpointOutput struct{}

func (h *Handler) jsonUpdateAcmeEndpoint(ctx context.Context, body []byte) (any, error) {
	var input updateAcmeEndpointInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeEndpointArn(input.AcmeEndpointArn); err != nil {
		return nil, err
	}

	if input.AcmeEndpointArn == "" {
		return nil, fmt.Errorf("%w: AcmeEndpointArn is required", ErrInvalidParameter)
	}

	params := UpdateAcmeEndpointParams{}
	if input.AuthorizationBehavior != "" {
		params.AuthorizationBehavior = &input.AuthorizationBehavior
	}

	if input.Contact != "" {
		params.Contact = &input.Contact
	}

	if input.CertificateAuthority != nil && input.CertificateAuthority.PublicCertificateAuthority != nil {
		algs := input.CertificateAuthority.PublicCertificateAuthority.AllowedKeyAlgorithms
		params.AllowedKeyAlgorithms = &algs
	}

	if err := h.Backend.UpdateAcmeEndpoint(ctx, input.AcmeEndpointArn, params); err != nil {
		return nil, err
	}

	return &updateAcmeEndpointOutput{}, nil
}
