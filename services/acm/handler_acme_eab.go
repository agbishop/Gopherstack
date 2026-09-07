package acm

import (
	"context"
	"encoding/json"
	"fmt"

	svcTags "github.com/blackbirdworks/gopherstack/pkgs/tags"
)

type acmeEABWire struct {
	ExpiresAt                     *int64 `json:"ExpiresAt,omitempty"`
	LastUsedAt                    *int64 `json:"LastUsedAt,omitempty"`
	RevokedAt                     *int64 `json:"RevokedAt,omitempty"`
	AcmeEndpointArn               string `json:"AcmeEndpointArn"`
	AcmeExternalAccountBindingArn string `json:"AcmeExternalAccountBindingArn"`
	RoleArn                       string `json:"RoleArn"`
	CreatedAt                     int64  `json:"CreatedAt"`
	UpdatedAt                     int64  `json:"UpdatedAt"`
}

func acmeEABToWire(eab *AcmeExternalAccountBinding) acmeEABWire {
	w := acmeEABWire{
		AcmeEndpointArn:               eab.AcmeEndpointArn,
		AcmeExternalAccountBindingArn: eab.ARN,
		RoleArn:                       eab.RoleArn,
		CreatedAt:                     eab.CreatedAt.Unix(),
		UpdatedAt:                     eab.UpdatedAt.Unix(),
	}
	if eab.ExpiresAt != nil {
		v := eab.ExpiresAt.Unix()
		w.ExpiresAt = &v
	}

	if eab.LastUsedAt != nil {
		v := eab.LastUsedAt.Unix()
		w.LastUsedAt = &v
	}

	if eab.RevokedAt != nil {
		v := eab.RevokedAt.Unix()
		w.RevokedAt = &v
	}

	return w
}

type expirationWire struct {
	Type  string `json:"Type"`
	Value int64  `json:"Value"`
}

type createAcmeEABInput struct {
	Expiration       *expirationWire `json:"Expiration"`
	AcmeEndpointArn  string          `json:"AcmeEndpointArn"`
	RoleArn          string          `json:"RoleArn"`
	IdempotencyToken string          `json:"IdempotencyToken"`
	Tags             []svcTags.KV    `json:"Tags"`
}

type createAcmeEABOutput struct {
	ExternalAccountBinding *acmeEABWire `json:"ExternalAccountBinding"`
}

func (h *Handler) jsonCreateAcmeExternalAccountBinding(ctx context.Context, body []byte) (any, error) {
	var input createAcmeEABInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	params := CreateAcmeEABParams{
		AcmeEndpointArn:  input.AcmeEndpointArn,
		RoleArn:          input.RoleArn,
		IdempotencyToken: input.IdempotencyToken,
		Tags:             input.Tags,
	}
	if input.Expiration != nil {
		params.ExpirationType = input.Expiration.Type
		params.ExpirationValue = input.Expiration.Value
	}

	eab, err := h.Backend.CreateAcmeExternalAccountBinding(ctx, params)
	if err != nil {
		return nil, err
	}

	if len(input.Tags) > 0 {
		kv := make(map[string]string, len(input.Tags))
		for _, t := range input.Tags {
			kv[t.Key] = t.Value
		}

		if tagErr := h.setTags(eab.ARN, kv, ErrInvalidParameter, ErrServiceQuotaExceeded); tagErr != nil {
			return nil, tagErr
		}
	}

	wire := acmeEABToWire(eab)

	return &createAcmeEABOutput{ExternalAccountBinding: &wire}, nil
}

type describeAcmeEABInput struct {
	AcmeExternalAccountBindingArn string `json:"AcmeExternalAccountBindingArn"`
}

type describeAcmeEABOutput struct {
	ExternalAccountBinding *acmeEABWire `json:"ExternalAccountBinding"`
}

func (h *Handler) jsonDescribeAcmeExternalAccountBinding(ctx context.Context, body []byte) (any, error) {
	var input describeAcmeEABInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeEABArn(input.AcmeExternalAccountBindingArn); err != nil {
		return nil, err
	}

	if input.AcmeExternalAccountBindingArn == "" {
		return nil, fmt.Errorf("%w: AcmeExternalAccountBindingArn is required", ErrInvalidParameter)
	}

	eab, err := h.Backend.DescribeAcmeExternalAccountBinding(ctx, input.AcmeExternalAccountBindingArn)
	if err != nil {
		return nil, err
	}

	wire := acmeEABToWire(eab)

	return &describeAcmeEABOutput{ExternalAccountBinding: &wire}, nil
}

type deleteAcmeEABInput struct {
	AcmeExternalAccountBindingArn string `json:"AcmeExternalAccountBindingArn"`
}

type deleteAcmeEABOutput struct{}

func (h *Handler) jsonDeleteAcmeExternalAccountBinding(ctx context.Context, body []byte) (any, error) {
	var input deleteAcmeEABInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeEABArn(input.AcmeExternalAccountBindingArn); err != nil {
		return nil, err
	}

	if err := h.Backend.DeleteAcmeExternalAccountBinding(ctx, input.AcmeExternalAccountBindingArn); err != nil {
		return nil, err
	}

	h.cleanupTags(input.AcmeExternalAccountBindingArn)

	return &deleteAcmeEABOutput{}, nil
}

type listAcmeEABsInput struct {
	AcmeEndpointArn string `json:"AcmeEndpointArn"`
	NextToken       string `json:"NextToken"`
	MaxResults      int32  `json:"MaxResults"`
}

type listAcmeEABsOutput struct {
	NextToken               string        `json:"NextToken,omitempty"`
	ExternalAccountBindings []acmeEABWire `json:"ExternalAccountBindings"`
}

func (h *Handler) jsonListAcmeExternalAccountBindings(ctx context.Context, body []byte) (any, error) {
	var input listAcmeEABsInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeEndpointArn(input.AcmeEndpointArn); err != nil {
		return nil, err
	}

	if input.AcmeEndpointArn == "" {
		return nil, fmt.Errorf("%w: AcmeEndpointArn is required", ErrInvalidParameter)
	}

	pg, err := h.Backend.ListAcmeExternalAccountBindings(
		ctx, input.AcmeEndpointArn, input.NextToken, int(input.MaxResults),
	)
	if err != nil {
		return nil, err
	}

	out := make([]acmeEABWire, 0, len(pg.Data))
	for i := range pg.Data {
		out = append(out, acmeEABToWire(&pg.Data[i]))
	}

	return &listAcmeEABsOutput{ExternalAccountBindings: out, NextToken: pg.Next}, nil
}

type getAcmeEABCredentialsInput struct {
	AcmeExternalAccountBindingArn string `json:"AcmeExternalAccountBindingArn"`
}

type getAcmeEABCredentialsOutput struct {
	KeyID  string `json:"KeyId"`
	MacKey string `json:"MacKey"`
}

func (h *Handler) jsonGetAcmeExternalAccountBindingCredentials(ctx context.Context, body []byte) (any, error) {
	var input getAcmeEABCredentialsInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeEABArn(input.AcmeExternalAccountBindingArn); err != nil {
		return nil, err
	}

	if input.AcmeExternalAccountBindingArn == "" {
		return nil, fmt.Errorf("%w: AcmeExternalAccountBindingArn is required", ErrInvalidParameter)
	}

	keyID, macKey, err := h.Backend.GetAcmeExternalAccountBindingCredentials(ctx, input.AcmeExternalAccountBindingArn)
	if err != nil {
		return nil, err
	}

	return &getAcmeEABCredentialsOutput{KeyID: keyID, MacKey: macKey}, nil
}

type revokeAcmeEABInput struct {
	AcmeExternalAccountBindingArn string `json:"AcmeExternalAccountBindingArn"`
}

type revokeAcmeEABOutput struct{}

func (h *Handler) jsonRevokeAcmeExternalAccountBinding(ctx context.Context, body []byte) (any, error) {
	var input revokeAcmeEABInput
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, ErrInvalidParameter
	}

	if err := validateAcmeEABArn(input.AcmeExternalAccountBindingArn); err != nil {
		return nil, err
	}

	if input.AcmeExternalAccountBindingArn == "" {
		return nil, fmt.Errorf("%w: AcmeExternalAccountBindingArn is required", ErrInvalidParameter)
	}

	if err := h.Backend.RevokeAcmeExternalAccountBinding(ctx, input.AcmeExternalAccountBindingArn); err != nil {
		return nil, err
	}

	return &revokeAcmeEABOutput{}, nil
}
