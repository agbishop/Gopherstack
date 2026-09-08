package verifiedpermissions

import (
	"context"
	"fmt"
	"strings"
)

// cedarVersion is the Cedar language version gopherstack's cedar-go
// evaluation engine implements (see GetPolicyStoreOutput.CedarVersion /
// Amazon Verified Permissions' Cedar v4 FAQ). Always CEDAR_4: gopherstack
// has no legacy CEDAR_2 policy stores to distinguish.
const cedarVersion = "CEDAR_4"

type validationSettingsJSON struct {
	Mode string `json:"mode"`
}

type createPolicyStoreInput struct {
	Tags               map[string]string      `json:"tags"`
	Description        string                 `json:"description"`
	ValidationSettings validationSettingsJSON `json:"validationSettings"`
	DeletionProtection string                 `json:"deletionProtection,omitempty"`
	ClientToken        string                 `json:"clientToken,omitempty"`
}

// createPolicyStoreOutput mirrors the real SDK's CreatePolicyStoreOutput:
// unlike GetPolicyStoreOutput, it does NOT echo validationSettings.
type createPolicyStoreOutput struct {
	PolicyStoreID   string `json:"policyStoreId"`
	Arn             string `json:"arn"`
	CreatedDate     string `json:"createdDate"`
	LastUpdatedDate string `json:"lastUpdatedDate"`
}

func (h *Handler) handleCreatePolicyStore(
	_ context.Context,
	in *createPolicyStoreInput,
) (*createPolicyStoreOutput, error) {
	if in.ValidationSettings.Mode == "" {
		return nil, fmt.Errorf("%w: validationSettings.mode is required", errInvalidRequest)
	}

	if in.ValidationSettings.Mode != ValidationModeOff && in.ValidationSettings.Mode != ValidationModeStrict {
		return nil, fmt.Errorf(
			"%w: validationSettings.mode must be %q or %q",
			errInvalidRequest, ValidationModeOff, ValidationModeStrict,
		)
	}

	// AWS bounds PolicyStoreDescription at 150 characters.
	if len(in.Description) > maxPolicyStoreDescriptionLen {
		return nil, fmt.Errorf(
			"%w: description must be %d characters or fewer",
			errInvalidRequest, maxPolicyStoreDescriptionLen,
		)
	}

	ps, err := h.Backend.CreatePolicyStore(
		in.Description, in.Tags,
		in.ValidationSettings.Mode, in.DeletionProtection, in.ClientToken,
	)
	if err != nil {
		return nil, err
	}

	return &createPolicyStoreOutput{
		PolicyStoreID:   ps.PolicyStoreID,
		Arn:             ps.Arn,
		CreatedDate:     ps.CreatedDate.UTC().Format(timeFormat),
		LastUpdatedDate: ps.LastUpdated.UTC().Format(timeFormat),
	}, nil
}

type policyStoreIDInput struct {
	PolicyStoreID string `json:"policyStoreId"`
}

// policyStoreView mirrors the real SDK's PolicyStoreItem (ListPolicyStores):
// a leaner shape than GetPolicyStoreOutput -- no validationSettings,
// deletionProtection, or cedarVersion.
type policyStoreView struct {
	PolicyStoreID   string `json:"policyStoreId"`
	Arn             string `json:"arn"`
	Description     string `json:"description"`
	CreatedDate     string `json:"createdDate"`
	LastUpdatedDate string `json:"lastUpdatedDate"`
}

// getPolicyStoreOutput mirrors the real SDK's GetPolicyStoreOutput.
type getPolicyStoreOutput struct {
	Tags               map[string]string      `json:"tags,omitempty"`
	PolicyStoreID      string                 `json:"policyStoreId"`
	Arn                string                 `json:"arn"`
	Description        string                 `json:"description"`
	CreatedDate        string                 `json:"createdDate"`
	LastUpdatedDate    string                 `json:"lastUpdatedDate"`
	ValidationSettings validationSettingsJSON `json:"validationSettings"`
	CedarVersion       string                 `json:"cedarVersion,omitempty"`
	DeletionProtection string                 `json:"deletionProtection,omitempty"`
}

func (h *Handler) handleGetPolicyStore(_ context.Context, in *policyStoreIDInput) (*getPolicyStoreOutput, error) {
	if in.PolicyStoreID == "" {
		return nil, fmt.Errorf("%w: policyStoreId is required", errInvalidRequest)
	}

	resolvedID, err := h.resolvePolicyStoreID(in.PolicyStoreID)
	if err != nil {
		return nil, err
	}

	ps, err := h.Backend.GetPolicyStore(resolvedID)
	if err != nil {
		return nil, err
	}

	return &getPolicyStoreOutput{
		PolicyStoreID:      ps.PolicyStoreID,
		Arn:                ps.Arn,
		Description:        ps.Description,
		CreatedDate:        ps.CreatedDate.UTC().Format(timeFormat),
		LastUpdatedDate:    ps.LastUpdated.UTC().Format(timeFormat),
		ValidationSettings: validationSettingsJSON{Mode: ps.ValidationMode},
		CedarVersion:       cedarVersion,
		DeletionProtection: ps.DeletionProtection,
		Tags:               ps.Tags,
	}, nil
}

type listPolicyStoresInput struct {
	NextToken  string `json:"nextToken,omitempty"`
	MaxResults int    `json:"maxResults,omitempty"`
}

type listPolicyStoresOutput struct {
	NextToken    string            `json:"nextToken,omitempty"`
	PolicyStores []policyStoreView `json:"policyStores"`
}

func (h *Handler) handleListPolicyStores(
	_ context.Context,
	in *listPolicyStoresInput,
) (*listPolicyStoresOutput, error) {
	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = defaultListPageSize
	}

	stores, nextToken := h.Backend.ListPolicyStores(in.NextToken, maxResults)
	items := make([]policyStoreView, 0, len(stores))

	for i := range stores {
		ps := &stores[i]
		items = append(items, policyStoreView{
			PolicyStoreID:   ps.PolicyStoreID,
			Arn:             ps.Arn,
			Description:     ps.Description,
			CreatedDate:     ps.CreatedDate.UTC().Format(timeFormat),
			LastUpdatedDate: ps.LastUpdated.UTC().Format(timeFormat),
		})
	}

	return &listPolicyStoresOutput{PolicyStores: items, NextToken: nextToken}, nil
}

type updatePolicyStoreInput struct {
	PolicyStoreID      string                  `json:"policyStoreId"`
	Description        string                  `json:"description"`
	ValidationSettings *validationSettingsJSON `json:"validationSettings,omitempty"`
	DeletionProtection string                  `json:"deletionProtection,omitempty"`
}

// updatePolicyStoreOutput mirrors the real SDK's UpdatePolicyStoreOutput:
// unlike CreatePolicyStoreOutput's sibling shape, it requires CreatedDate
// too (since the store already existed), and -- like CreatePolicyStoreOutput
// -- does NOT echo validationSettings.
type updatePolicyStoreOutput struct {
	PolicyStoreID   string `json:"policyStoreId"`
	Arn             string `json:"arn"`
	CreatedDate     string `json:"createdDate"`
	LastUpdatedDate string `json:"lastUpdatedDate"`
}

func (h *Handler) handleUpdatePolicyStore(
	_ context.Context,
	in *updatePolicyStoreInput,
) (*updatePolicyStoreOutput, error) {
	if in.PolicyStoreID == "" {
		return nil, fmt.Errorf("%w: policyStoreId is required", errInvalidRequest)
	}

	resolvedID, err := h.resolvePolicyStoreID(in.PolicyStoreID)
	if err != nil {
		return nil, err
	}

	var validationMode string

	if in.ValidationSettings != nil {
		validationMode = in.ValidationSettings.Mode
	}

	ps, err := h.Backend.UpdatePolicyStore(resolvedID, in.Description, validationMode, in.DeletionProtection)
	if err != nil {
		return nil, err
	}

	return &updatePolicyStoreOutput{
		PolicyStoreID:   ps.PolicyStoreID,
		Arn:             ps.Arn,
		CreatedDate:     ps.CreatedDate.UTC().Format(timeFormat),
		LastUpdatedDate: ps.LastUpdated.UTC().Format(timeFormat),
	}, nil
}

// handleDeletePolicyStore does not resolve policyStoreId through
// resolvePolicyStoreID: the real SDK's DeletePolicyStoreInput.PolicyStoreId
// doc is explicit that this operation is the exception to the usual
// ID-or-alias rule -- "the alias name cannot be used. Only the ID can be
// used." An alias-shaped value is rejected outright, distinct from
// DeletePolicyStore's own idempotent-on-missing-ID behavior.
func (h *Handler) handleDeletePolicyStore(_ context.Context, in *policyStoreIDInput) (*struct{}, error) {
	if in.PolicyStoreID == "" {
		return nil, fmt.Errorf("%w: policyStoreId is required", errInvalidRequest)
	}

	if strings.HasPrefix(in.PolicyStoreID, policyStoreAliasPrefix) {
		return nil, fmt.Errorf("%w: policyStoreId must be a policy store ID, not an alias name", errInvalidRequest)
	}

	if err := h.Backend.DeletePolicyStore(in.PolicyStoreID); err != nil {
		return nil, err
	}

	return &struct{}{}, nil
}
