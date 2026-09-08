package verifiedpermissions

import (
	"context"
	"fmt"
)

type staticPolicyDefinition struct {
	Statement   string `json:"statement"`
	Description string `json:"description,omitempty"`
}

type templateLinkedPolicyDefinition struct {
	Principal        *entityIdentifier `json:"principal,omitempty"`
	Resource         *entityIdentifier `json:"resource,omitempty"`
	PolicyTemplateID string            `json:"policyTemplateId"`
}

type entityIdentifier struct {
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
}

type policyDefinitionIn struct {
	Static         *staticPolicyDefinition         `json:"static,omitempty"`
	TemplateLinked *templateLinkedPolicyDefinition `json:"templateLinked,omitempty"`
}

// staticPolicyDefinitionOut / templateLinkedPolicyDefinitionOut /
// policyDefinitionOut mirror the real SDK's PolicyDefinitionDetail union
// (StaticPolicyDefinitionDetail / TemplateLinkedPolicyDefinitionDetail): the
// "full detail" shape used by GetPolicy and BatchGetPolicy, where the static
// variant includes the full Cedar statement text.
type staticPolicyDefinitionOut struct {
	Statement   string `json:"statement"`
	Description string `json:"description,omitempty"`
}

type templateLinkedPolicyDefinitionOut struct {
	Principal        *entityIdentifier `json:"principal,omitempty"`
	Resource         *entityIdentifier `json:"resource,omitempty"`
	PolicyTemplateID string            `json:"policyTemplateId"`
}

type policyDefinitionOut struct {
	Static         *staticPolicyDefinitionOut         `json:"static,omitempty"`
	TemplateLinked *templateLinkedPolicyDefinitionOut `json:"templateLinked,omitempty"`
}

// staticPolicyDefinitionItemOut / policyDefinitionItemOut mirror the real
// SDK's PolicyDefinitionItem union (StaticPolicyDefinitionItem /
// TemplateLinkedPolicyDefinitionItem): the lighter "item" shape used by
// ListPolicies, where the static variant does NOT echo the Cedar statement
// text (only Description) -- unlike the detail shape above.
type staticPolicyDefinitionItemOut struct {
	Description string `json:"description,omitempty"`
}

type policyDefinitionItemOut struct {
	Static         *staticPolicyDefinitionItemOut     `json:"static,omitempty"`
	TemplateLinked *templateLinkedPolicyDefinitionOut `json:"templateLinked,omitempty"`
}

type createPolicyInput struct {
	Definition    policyDefinitionIn `json:"definition"`
	PolicyStoreID string             `json:"policyStoreId"`
	ClientToken   string             `json:"clientToken,omitempty"`
}

// policyIDsOutput is shared by CreatePolicy/UpdatePolicy. Beyond the policy's
// identity, the real SDK's CreatePolicyOutput/UpdatePolicyOutput also echo
// effect/actions/principal/resource parsed from the policy's Cedar scope
// clause.
type policyIDsOutput struct {
	Principal       *entityIdentifier      `json:"principal,omitempty"`
	Resource        *entityIdentifier      `json:"resource,omitempty"`
	PolicyStoreID   string                 `json:"policyStoreId"`
	PolicyID        string                 `json:"policyId"`
	PolicyType      string                 `json:"policyType"`
	Effect          string                 `json:"effect,omitempty"`
	CreatedDate     string                 `json:"createdDate"`
	LastUpdatedDate string                 `json:"lastUpdatedDate"`
	Actions         []actionIdentifierJSON `json:"actions,omitempty"`
}

// policyEcho bundles the effect/actions/principal/resource fields the real
// SDK echoes at the top level of Create/Get/Update/List policy responses.
type policyEcho struct {
	Principal *entityIdentifier
	Resource  *entityIdentifier
	Effect    string
	Actions   []actionIdentifierJSON
}

// templateLinkedRef builds the principal/resource EntityIdentifiers bound on
// a TEMPLATE_LINKED policy (nil when a slot isn't bound). Unlike a STATIC
// policy's principal/resource (parsed from its Cedar scope clause), a
// template-linked policy's principal/resource are always the concrete values
// substituted for the template's ?principal/?resource placeholders.
func templateLinkedRef(p *Policy) (*entityIdentifier, *entityIdentifier) {
	var principal, resource *entityIdentifier

	if p.PrincipalEntityType != "" {
		principal = &entityIdentifier{EntityType: p.PrincipalEntityType, EntityID: p.PrincipalEntityID}
	}

	if p.ResourceEntityType != "" {
		resource = &entityIdentifier{EntityType: p.ResourceEntityType, EntityID: p.ResourceEntityID}
	}

	return principal, resource
}

// policyEchoFields computes the effect/actions/principal/resource fields the
// real SDK echoes at the top level of Create/Get/Update/List policy
// responses. Effect and Actions always come from the policy's effective
// Cedar scope clause (STATIC: the policy's own statement; TEMPLATE_LINKED:
// the referenced template's statement). Principal/Resource come from that
// same scope clause for STATIC policies, but from the policy's own bound
// principal/resource for TEMPLATE_LINKED policies (whose scope clause holds
// a slot, not a concrete entity).
func (h *Handler) policyEchoFields(p *Policy) policyEcho {
	var echo policyEcho

	scope := h.Backend.PolicyScope(p)
	if scope != nil {
		echo.Effect = scope.Effect
		echo.Actions = scope.Actions
	}

	switch p.PolicyType {
	case policyTypeStatic:
		if scope != nil {
			echo.Principal = scope.Principal
			echo.Resource = scope.Resource
		}
	case policyTypeTemplateLinked:
		echo.Principal, echo.Resource = templateLinkedRef(p)
	}

	return echo
}

// policyDefinitionDetail builds the full-detail Definition union (statement
// text included for STATIC) used by GetPolicy/BatchGetPolicy.
func policyDefinitionDetail(p *Policy) policyDefinitionOut {
	var def policyDefinitionOut

	switch p.PolicyType {
	case policyTypeStatic:
		def.Static = &staticPolicyDefinitionOut{Statement: p.Statement, Description: p.Description}
	case policyTypeTemplateLinked:
		principal, resource := templateLinkedRef(p)
		def.TemplateLinked = &templateLinkedPolicyDefinitionOut{
			PolicyTemplateID: p.PolicyTemplateID,
			Principal:        principal,
			Resource:         resource,
		}
	}

	return def
}

// policyDefinitionItem builds the lighter item Definition union (no
// statement text for STATIC) used by ListPolicies.
func policyDefinitionItem(p *Policy) policyDefinitionItemOut {
	var def policyDefinitionItemOut

	switch p.PolicyType {
	case policyTypeStatic:
		def.Static = &staticPolicyDefinitionItemOut{Description: p.Description}
	case policyTypeTemplateLinked:
		principal, resource := templateLinkedRef(p)
		def.TemplateLinked = &templateLinkedPolicyDefinitionOut{
			PolicyTemplateID: p.PolicyTemplateID,
			Principal:        principal,
			Resource:         resource,
		}
	}

	return def
}

//nolint:nestif // definition union type dispatch
func (h *Handler) handleCreatePolicy(_ context.Context, in *createPolicyInput) (*policyIDsOutput, error) {
	if in.PolicyStoreID == "" {
		return nil, fmt.Errorf("%w: policyStoreId is required", errInvalidRequest)
	}

	resolvedID, err := h.resolvePolicyStoreID(in.PolicyStoreID)
	if err != nil {
		return nil, err
	}

	if (in.Definition.Static == nil) == (in.Definition.TemplateLinked == nil) {
		return nil, fmt.Errorf("%w: definition must contain exactly one of static or templateLinked", errInvalidRequest)
	}

	params := CreatePolicyParams{ClientToken: in.ClientToken}

	if in.Definition.Static != nil {
		if in.Definition.Static.Statement == "" {
			return nil, fmt.Errorf("%w: definition.static.statement is required", errInvalidRequest)
		}

		params.PolicyType = policyTypeStatic
		params.Statement = in.Definition.Static.Statement
		params.Description = in.Definition.Static.Description
	} else {
		tl := in.Definition.TemplateLinked
		if tl.PolicyTemplateID == "" {
			return nil, fmt.Errorf("%w: definition.templateLinked.policyTemplateId is required", errInvalidRequest)
		}

		params.PolicyType = policyTypeTemplateLinked
		params.PolicyTemplateID = tl.PolicyTemplateID

		if tl.Principal != nil {
			params.PrincipalEntityType = tl.Principal.EntityType
			params.PrincipalEntityID = tl.Principal.EntityID
		}

		if tl.Resource != nil {
			params.ResourceEntityType = tl.Resource.EntityType
			params.ResourceEntityID = tl.Resource.EntityID
		}
	}

	p, err := h.Backend.CreatePolicy(resolvedID, params)
	if err != nil {
		return nil, err
	}

	echo := h.policyEchoFields(p)

	return &policyIDsOutput{
		PolicyStoreID:   p.PolicyStoreID,
		PolicyID:        p.PolicyID,
		PolicyType:      p.PolicyType,
		Effect:          echo.Effect,
		Actions:         echo.Actions,
		Principal:       echo.Principal,
		Resource:        echo.Resource,
		CreatedDate:     p.CreatedDate.UTC().Format(timeFormat),
		LastUpdatedDate: p.LastUpdated.UTC().Format(timeFormat),
	}, nil
}

type policyInput struct {
	PolicyStoreID string `json:"policyStoreId"`
	PolicyID      string `json:"policyId"`
}

// policyView mirrors the real SDK's GetPolicyOutput shape.
type policyView struct {
	Definition      policyDefinitionOut    `json:"definition"`
	Principal       *entityIdentifier      `json:"principal,omitempty"`
	Resource        *entityIdentifier      `json:"resource,omitempty"`
	PolicyStoreID   string                 `json:"policyStoreId"`
	PolicyID        string                 `json:"policyId"`
	PolicyType      string                 `json:"policyType"`
	Effect          string                 `json:"effect,omitempty"`
	CreatedDate     string                 `json:"createdDate"`
	LastUpdatedDate string                 `json:"lastUpdatedDate"`
	Actions         []actionIdentifierJSON `json:"actions,omitempty"`
}

// policyListItemView mirrors the real SDK's PolicyItem shape used by
// ListPolicies -- same top-level fields as policyView, but a lighter
// Definition (no Cedar statement text for STATIC policies).
type policyListItemView struct {
	Definition      policyDefinitionItemOut `json:"definition"`
	Principal       *entityIdentifier       `json:"principal,omitempty"`
	Resource        *entityIdentifier       `json:"resource,omitempty"`
	PolicyStoreID   string                  `json:"policyStoreId"`
	PolicyID        string                  `json:"policyId"`
	PolicyType      string                  `json:"policyType"`
	Effect          string                  `json:"effect,omitempty"`
	CreatedDate     string                  `json:"createdDate"`
	LastUpdatedDate string                  `json:"lastUpdatedDate"`
	Actions         []actionIdentifierJSON  `json:"actions,omitempty"`
}

func (h *Handler) policyToView(p *Policy) policyView {
	echo := h.policyEchoFields(p)

	return policyView{
		PolicyStoreID:   p.PolicyStoreID,
		PolicyID:        p.PolicyID,
		PolicyType:      p.PolicyType,
		Definition:      policyDefinitionDetail(p),
		Effect:          echo.Effect,
		Actions:         echo.Actions,
		Principal:       echo.Principal,
		Resource:        echo.Resource,
		CreatedDate:     p.CreatedDate.UTC().Format(timeFormat),
		LastUpdatedDate: p.LastUpdated.UTC().Format(timeFormat),
	}
}

func (h *Handler) policyToListItemView(p *Policy) policyListItemView {
	echo := h.policyEchoFields(p)

	return policyListItemView{
		PolicyStoreID:   p.PolicyStoreID,
		PolicyID:        p.PolicyID,
		PolicyType:      p.PolicyType,
		Definition:      policyDefinitionItem(p),
		Effect:          echo.Effect,
		Actions:         echo.Actions,
		Principal:       echo.Principal,
		Resource:        echo.Resource,
		CreatedDate:     p.CreatedDate.UTC().Format(timeFormat),
		LastUpdatedDate: p.LastUpdated.UTC().Format(timeFormat),
	}
}

func (h *Handler) handleGetPolicy(_ context.Context, in *policyInput) (*policyView, error) {
	if in.PolicyStoreID == "" {
		return nil, fmt.Errorf("%w: policyStoreId is required", errInvalidRequest)
	}

	if in.PolicyID == "" {
		return nil, fmt.Errorf("%w: policyId is required", errInvalidRequest)
	}

	resolvedID, err := h.resolvePolicyStoreID(in.PolicyStoreID)
	if err != nil {
		return nil, err
	}

	p, err := h.Backend.GetPolicy(resolvedID, in.PolicyID)
	if err != nil {
		return nil, err
	}

	v := h.policyToView(p)

	return &v, nil
}

// entityReferenceJSON mirrors the real SDK's EntityReference union: a
// PolicyFilter's principal/resource is NOT a flat entityIdentifier, it's
// wrapped as {"identifier": {entityType, entityId}} or {"unspecified": true}
// (unlike GetPolicy's top-level principal/resource, which ARE flat).
type entityReferenceJSON struct {
	Identifier  *entityIdentifier `json:"identifier,omitempty"`
	Unspecified *bool             `json:"unspecified,omitempty"`
}

type listPoliciesFilterJSON struct {
	Principal                 *entityReferenceJSON `json:"principal,omitempty"`
	Resource                  *entityReferenceJSON `json:"resource,omitempty"`
	PolicyType                string               `json:"policyType,omitempty"`
	PolicyTemplateIDForFilter string               `json:"policyTemplateId,omitempty"`
}

type listPoliciesInput struct {
	PolicyStoreID string                  `json:"policyStoreId"`
	Filter        *listPoliciesFilterJSON `json:"filter,omitempty"`
	NextToken     string                  `json:"nextToken,omitempty"`
	MaxResults    int                     `json:"maxResults,omitempty"`
}

type listPoliciesOutput struct {
	NextToken string               `json:"nextToken,omitempty"`
	Policies  []policyListItemView `json:"policies"`
}

// resolvedEntityReference holds the fields extracted from an EntityReference
// union (see entityReferenceJSON).
type resolvedEntityReference struct {
	entityType  string
	entityID    string
	unspecified bool
}

// resolveEntityReference extracts entityType/entityId/unspecified from an
// EntityReference union. unspecified is true only when the wire sent
// {"unspecified": true}.
func resolveEntityReference(ref *entityReferenceJSON) resolvedEntityReference {
	if ref == nil {
		return resolvedEntityReference{}
	}

	if ref.Identifier != nil {
		return resolvedEntityReference{entityType: ref.Identifier.EntityType, entityID: ref.Identifier.EntityID}
	}

	return resolvedEntityReference{unspecified: ref.Unspecified != nil && *ref.Unspecified}
}

// buildListPoliciesFilter converts the wire filter (nil-safe) into the
// backend's ListPoliciesFilter.
func buildListPoliciesFilter(in *listPoliciesFilterJSON) ListPoliciesFilter {
	if in == nil {
		return ListPoliciesFilter{}
	}

	filter := ListPoliciesFilter{
		PolicyType:       in.PolicyType,
		PolicyTemplateID: in.PolicyTemplateIDForFilter,
	}

	principal := resolveEntityReference(in.Principal)
	filter.PrincipalEntityType = principal.entityType
	filter.PrincipalEntityID = principal.entityID
	filter.PrincipalUnspecified = principal.unspecified

	resource := resolveEntityReference(in.Resource)
	filter.ResourceEntityType = resource.entityType
	filter.ResourceEntityID = resource.entityID
	filter.ResourceUnspecified = resource.unspecified

	return filter
}

func (h *Handler) handleListPolicies(_ context.Context, in *listPoliciesInput) (*listPoliciesOutput, error) {
	if in.PolicyStoreID == "" {
		return nil, fmt.Errorf("%w: policyStoreId is required", errInvalidRequest)
	}

	resolvedID, err := h.resolvePolicyStoreID(in.PolicyStoreID)
	if err != nil {
		return nil, err
	}

	filter := buildListPoliciesFilter(in.Filter)

	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = defaultListPageSize
	}

	policies, nextToken, err := h.Backend.ListPolicies(resolvedID, filter, in.NextToken, maxResults)
	if err != nil {
		return nil, err
	}

	items := make([]policyListItemView, 0, len(policies))

	for i := range policies {
		items = append(items, h.policyToListItemView(&policies[i]))
	}

	return &listPoliciesOutput{Policies: items, NextToken: nextToken}, nil
}

type updatePolicyInput struct {
	Definition    policyDefinitionIn `json:"definition"`
	PolicyStoreID string             `json:"policyStoreId"`
	PolicyID      string             `json:"policyId"`
}

func (h *Handler) handleUpdatePolicy(_ context.Context, in *updatePolicyInput) (*policyIDsOutput, error) {
	if in.PolicyStoreID == "" {
		return nil, fmt.Errorf("%w: policyStoreId is required", errInvalidRequest)
	}

	if in.PolicyID == "" {
		return nil, fmt.Errorf("%w: policyId is required", errInvalidRequest)
	}

	resolvedID, err := h.resolvePolicyStoreID(in.PolicyStoreID)
	if err != nil {
		return nil, err
	}

	// Real AWS's UpdatePolicyDefinition union has only a "static" member
	// (UpdatePolicyDefinitionMemberStatic) -- templateLinked isn't a valid
	// member here at all. The op doc is explicit: "You can directly update
	// only static policies. To change a template-linked policy, you must
	// update the template instead, using UpdatePolicyTemplate."
	if in.Definition.TemplateLinked != nil {
		return nil, fmt.Errorf(
			"%w: definition.templateLinked is not valid for UpdatePolicy; use UpdatePolicyTemplate instead",
			errInvalidRequest,
		)
	}

	if in.Definition.Static == nil {
		return nil, fmt.Errorf("%w: definition.static is required", errInvalidRequest)
	}

	if in.Definition.Static.Statement == "" {
		return nil, fmt.Errorf("%w: definition.static.statement is required", errInvalidRequest)
	}

	params := UpdatePolicyParams{
		Statement:   in.Definition.Static.Statement,
		Description: in.Definition.Static.Description,
	}

	p, err := h.Backend.UpdatePolicy(resolvedID, in.PolicyID, params)
	if err != nil {
		return nil, err
	}

	echo := h.policyEchoFields(p)

	return &policyIDsOutput{
		PolicyStoreID:   p.PolicyStoreID,
		PolicyID:        p.PolicyID,
		PolicyType:      p.PolicyType,
		Effect:          echo.Effect,
		Actions:         echo.Actions,
		Principal:       echo.Principal,
		Resource:        echo.Resource,
		CreatedDate:     p.CreatedDate.UTC().Format(timeFormat),
		LastUpdatedDate: p.LastUpdated.UTC().Format(timeFormat),
	}, nil
}

func (h *Handler) handleDeletePolicy(_ context.Context, in *policyInput) (*struct{}, error) {
	if in.PolicyStoreID == "" {
		return nil, fmt.Errorf("%w: policyStoreId is required", errInvalidRequest)
	}

	if in.PolicyID == "" {
		return nil, fmt.Errorf("%w: policyId is required", errInvalidRequest)
	}

	resolvedID, err := h.resolvePolicyStoreID(in.PolicyStoreID)
	if err != nil {
		return nil, err
	}

	if err = h.Backend.DeletePolicy(resolvedID, in.PolicyID); err != nil {
		return nil, err
	}

	return &struct{}{}, nil
}

type batchGetPolicyRequest struct {
	Requests []struct {
		PolicyStoreID string `json:"policyStoreId"`
		PolicyID      string `json:"policyId"`
	} `json:"requests"`
}

// batchGetPolicyItemOut mirrors the real SDK's BatchGetPolicyOutputItem:
// unlike GetPolicy/ListPolicies, it carries no top-level
// effect/actions/principal/resource fields.
type batchGetPolicyItemOut struct {
	Definition      policyDefinitionOut `json:"definition"`
	PolicyStoreID   string              `json:"policyStoreId"`
	PolicyID        string              `json:"policyId"`
	PolicyType      string              `json:"policyType"`
	CreatedDate     string              `json:"createdDate"`
	LastUpdatedDate string              `json:"lastUpdatedDate"`
}

type batchGetPolicyHandlerOutput struct {
	Results []batchGetPolicyItemOut   `json:"results"`
	Errors  []batchGetPolicyErrorItem `json:"errors"`
}

func (h *Handler) handleBatchGetPolicy(
	_ context.Context,
	in *batchGetPolicyRequest,
) (*batchGetPolicyHandlerOutput, error) {
	items := make([]BatchGetPolicyItem, 0, len(in.Requests))
	unresolved := make([]batchGetPolicyErrorItem, 0)

	for _, r := range in.Requests {
		if r.PolicyStoreID == "" || r.PolicyID == "" {
			return nil, fmt.Errorf("%w: each request requires policyStoreId and policyId", errInvalidRequest)
		}

		// Each batch item's policyStoreId may itself be an alias (same
		// resolution rule as every other policyStoreId field -- see
		// resolvePolicyStoreID). An unresolvable alias fails just that one
		// item, not the whole batch, mirroring how the backend already
		// reports a not-found policy store as a per-item error.
		// resolvePolicyStoreID only errors when r.PolicyStoreID carried the
		// alias prefix and resolution failed, so this is always an alias
		// miss -- the real SDK's BatchGetPolicyErrorCode enum has a
		// dedicated POLICY_STORE_ALIAS_NOT_FOUND value for exactly that,
		// distinct from a bare-ID miss (POLICY_STORE_NOT_FOUND), which the
		// backend's own item loop already reports correctly.
		resolvedID, err := h.resolvePolicyStoreID(r.PolicyStoreID)
		if err != nil {
			unresolved = append(unresolved, batchGetPolicyErrorItem{
				PolicyStoreID: r.PolicyStoreID,
				PolicyID:      r.PolicyID,
				Code:          "POLICY_STORE_ALIAS_NOT_FOUND",
				Message:       err.Error(),
			})

			continue
		}

		items = append(items, BatchGetPolicyItem{
			PolicyStoreID: resolvedID,
			PolicyID:      r.PolicyID,
		})
	}

	result := h.Backend.BatchGetPolicy(items)
	result.Errors = append(unresolved, result.Errors...)

	out := make([]batchGetPolicyItemOut, 0, len(result.Results))

	for i := range result.Results {
		p := &result.Results[i]
		out = append(out, batchGetPolicyItemOut{
			Definition:      policyDefinitionDetail(p),
			PolicyStoreID:   p.PolicyStoreID,
			PolicyID:        p.PolicyID,
			PolicyType:      p.PolicyType,
			CreatedDate:     p.CreatedDate.UTC().Format(timeFormat),
			LastUpdatedDate: p.LastUpdated.UTC().Format(timeFormat),
		})
	}

	return &batchGetPolicyHandlerOutput{
		Results: out,
		Errors:  result.Errors,
	}, nil
}
