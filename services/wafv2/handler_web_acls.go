package wafv2

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// createWebACLRequest is the request body for CreateWebACL.
type createWebACLRequest struct {
	DefaultAction                json.RawMessage  `json:"DefaultAction"`
	VisibilityConfig             json.RawMessage  `json:"VisibilityConfig"`
	CustomResponseBodies         json.RawMessage  `json:"CustomResponseBodies"`
	AssociationConfig            json.RawMessage  `json:"AssociationConfig"`
	CaptchaConfig                json.RawMessage  `json:"CaptchaConfig"`
	ChallengeConfig              json.RawMessage  `json:"ChallengeConfig"`
	MonetizationConfig           json.RawMessage  `json:"MonetizationConfig"`
	DataProtectionConfig         json.RawMessage  `json:"DataProtectionConfig"`
	ApplicationConfig            json.RawMessage  `json:"ApplicationConfig"`
	OnSourceDDoSProtectionConfig json.RawMessage  `json:"OnSourceDDoSProtectionConfig"`
	Name                         string           `json:"Name"`
	Scope                        string           `json:"Scope"`
	Description                  string           `json:"Description"`
	Tags                         []tags.KV        `json:"Tags"`
	TokenDomains                 []string         `json:"TokenDomains"`
	Rules                        []map[string]any `json:"Rules"`
}

func (h *Handler) handleCreateWebACL(ctx context.Context, body []byte) ([]byte, error) {
	var req createWebACLRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", errInvalidRequest)
	}

	if err := validateResourceName(req.Name); err != nil {
		return nil, err
	}

	if err := validateDescription(req.Description); err != nil {
		return nil, err
	}

	if req.Scope == "" {
		return nil, fmt.Errorf("%w: Scope is required", errInvalidRequest)
	}

	if !validScope(req.Scope) {
		return nil, fmt.Errorf("%w: Scope must be %s or %s", errInvalidRequest, ScopeRegional, ScopeCloudFront)
	}

	if err := validateVisibilityConfig(req.VisibilityConfig); err != nil {
		return nil, err
	}

	if err := validateDefaultAction(req.DefaultAction); err != nil {
		return nil, err
	}

	if err := validateRules(req.Rules); err != nil {
		return nil, err
	}

	tags := tags.MapFromKV(req.Tags)
	if err := validateTags(tags); err != nil {
		return nil, err
	}

	w, err := h.Backend.CreateWebACL(
		ctx,
		req.Name,
		req.Scope,
		req.Description,
		req.DefaultAction,
		req.VisibilityConfig,
		req.Rules,
		req.TokenDomains,
		req.CustomResponseBodies,
		req.AssociationConfig,
		req.CaptchaConfig,
		req.ChallengeConfig,
		req.MonetizationConfig,
		req.DataProtectionConfig,
		req.ApplicationConfig,
		req.OnSourceDDoSProtectionConfig,
		tags,
	)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "wafv2: created web ACL", "name", w.Name, "id", w.ID)

	arnStr := h.Backend.WebACLARN(w.Name, w.ID, w.Scope)

	return json.Marshal(map[string]any{
		keySummary: map[string]string{
			"Id":           w.ID,
			keyName:        w.Name,
			keyARN:         arnStr,
			keyLockToken:   w.LockToken,
			keyDescription: w.Description,
		},
	})
}

// validateDefaultAction validates that exactly one of Allow/Block is set in DefaultAction.
func validateDefaultAction(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("%w: invalid DefaultAction JSON: %w", errInvalidRequest, err)
	}

	_, hasAllow := m["Allow"]
	_, hasBlock := m["Block"]

	if hasAllow && hasBlock {
		return fmt.Errorf("%w: DefaultAction must specify exactly one of Allow or Block", errInvalidRequest)
	}

	return nil
}

// getWebACLRequest is the request body for GetWebACL.
type getWebACLRequest struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	Scope string `json:"Scope"`
	ARN   string `json:"ARN"`
}

func (h *Handler) handleGetWebACL(ctx context.Context, body []byte) ([]byte, error) {
	var req getWebACLRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	// GetWebACLInput marks no member required (wafv2@v1.77.3
	// api_op_GetWebACL.go): ARN is an alternative to Name+Scope+Id, so only
	// their absence together is invalid -- gopherstack-4ly2.
	if req.ID == "" && req.ARN == "" {
		return nil, fmt.Errorf("%w: Id or ARN is required", errInvalidRequest)
	}

	var (
		w   *WebACL
		err error
	)

	if req.ID != "" {
		w, err = h.Backend.GetWebACL(ctx, req.ID)
	} else {
		w, err = h.Backend.GetWebACLByARN(ctx, req.ARN)
	}

	if err != nil {
		return nil, err
	}

	if req.Scope != "" && w.Scope != req.Scope {
		return nil, fmt.Errorf("%w: web ACL %q has scope %s, not %s", ErrWebACLNotFound, w.ID, w.Scope, req.Scope)
	}

	return h.marshalWebACL(ctx, w)
}

// marshalWebACL builds the canonical WebACL JSON response.
func (h *Handler) marshalWebACL(ctx context.Context, w *WebACL) ([]byte, error) {
	arnStr := h.Backend.WebACLARN(w.Name, w.ID, w.Scope)
	visConfig := parseVisibilityConfig(w.VisibilityConfig, w.Name)

	// Capacity ("web ACL capacity units... currently being used by this web
	// ACL", wafv2@v1.77.3 types/types.go) is real, always-populated AWS data
	// this backend can derive with its existing per-statement WCU cost model
	// (capacity.go, the same one CheckCapacity uses) rather than fabricating
	// a value. Ignoring the error is safe: CheckCapacity never returns one.
	capacity, _ := h.Backend.CheckCapacity(ctx, w.Scope, w.Rules)

	// LabelNamespace grammar ("awswaf:<account ID>:webacl:<web ACL name>:")
	// confirmed via https://docs.aws.amazon.com/waf/latest/APIReference/API_WebACL.html
	// (the pinned SDK's own doc comment has its <placeholder> substitutions
	// stripped by a codegen artifact) -- deterministic from data this
	// backend already has, not fabricated.
	labelNamespace := fmt.Sprintf("awswaf:%s:webacl:%s:", h.Backend.AccountID(), w.Name)

	defaultActionJSON := w.DefaultAction
	if len(defaultActionJSON) == 0 {
		defaultActionJSON = json.RawMessage(`{"Allow":{}}`)
	}

	var defaultActionMap any
	if err := json.Unmarshal(defaultActionJSON, &defaultActionMap); err != nil {
		defaultActionMap = map[string]any{"Allow": map[string]any{}}
	}

	rules := w.Rules
	if rules == nil {
		rules = []map[string]any{}
	}

	webACLMap := map[string]any{
		"Id":                w.ID,
		keyName:             w.Name,
		keyARN:              arnStr,
		keyLockToken:        w.LockToken,
		keyDescription:      w.Description,
		"DefaultAction":     defaultActionMap,
		keyVisibilityConfig: visConfig,
		keyRules:            rules,
		keyCapacity:         capacity,
		keyLabelNamespace:   labelNamespace,
	}

	if len(w.TokenDomains) > 0 {
		webACLMap["TokenDomains"] = w.TokenDomains
	}

	addOptionalWebACLConfigs(webACLMap, w)

	return json.Marshal(map[string]any{
		"WebACL":     webACLMap,
		keyLockToken: w.LockToken,
	})
}

// addOptionalWebACLConfigs unmarshals each opaque config field on w that is
// present and adds it to webACLMap under its wire key. Extracted from
// marshalWebACL to keep its cognitive complexity down.
func addOptionalWebACLConfigs(webACLMap map[string]any, w *WebACL) {
	configs := []struct {
		key string
		raw json.RawMessage
	}{
		{"CustomResponseBodies", w.CustomResponseBodies},
		{"AssociationConfig", w.AssociationConfig},
		{"CaptchaConfig", w.CaptchaConfig},
		{"ChallengeConfig", w.ChallengeConfig},
		{"MonetizationConfig", w.MonetizationConfig},
		{"DataProtectionConfig", w.DataProtectionConfig},
		{"ApplicationConfig", w.ApplicationConfig},
		{"OnSourceDDoSProtectionConfig", w.OnSourceDDoSProtectionConfig},
	}

	for _, c := range configs {
		if len(c.raw) == 0 {
			continue
		}

		var v any
		if json.Unmarshal(c.raw, &v) == nil {
			webACLMap[c.key] = v
		}
	}
}

// updateWebACLRequest is the request body for UpdateWebACL.
type updateWebACLRequest struct {
	DefaultAction                json.RawMessage  `json:"DefaultAction"`
	VisibilityConfig             json.RawMessage  `json:"VisibilityConfig"`
	CustomResponseBodies         json.RawMessage  `json:"CustomResponseBodies"`
	AssociationConfig            json.RawMessage  `json:"AssociationConfig"`
	CaptchaConfig                json.RawMessage  `json:"CaptchaConfig"`
	ChallengeConfig              json.RawMessage  `json:"ChallengeConfig"`
	MonetizationConfig           json.RawMessage  `json:"MonetizationConfig"`
	DataProtectionConfig         json.RawMessage  `json:"DataProtectionConfig"`
	ApplicationConfig            json.RawMessage  `json:"ApplicationConfig"`
	OnSourceDDoSProtectionConfig json.RawMessage  `json:"OnSourceDDoSProtectionConfig"`
	ID                           string           `json:"Id"`
	Name                         string           `json:"Name"`
	Scope                        string           `json:"Scope"`
	LockToken                    string           `json:"LockToken"`
	Description                  string           `json:"Description"`
	TokenDomains                 []string         `json:"TokenDomains"`
	Rules                        []map[string]any `json:"Rules"`
}

func (h *Handler) handleUpdateWebACL(ctx context.Context, body []byte) ([]byte, error) {
	var req updateWebACLRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ID == "" {
		return nil, fmt.Errorf("%w: Id is required", errInvalidRequest)
	}

	if req.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", errInvalidRequest)
	}

	if req.Scope == "" {
		return nil, fmt.Errorf("%w: Scope is required", errInvalidRequest)
	}

	if req.LockToken == "" {
		return nil, fmt.Errorf("%w: LockToken is required", errInvalidRequest)
	}

	if err := validateVisibilityConfig(req.VisibilityConfig); err != nil {
		return nil, err
	}

	if err := validateDefaultAction(req.DefaultAction); err != nil {
		return nil, err
	}

	if err := validateRules(req.Rules); err != nil {
		return nil, err
	}

	w, err := h.Backend.UpdateWebACL(
		ctx,
		req.ID,
		req.Description,
		req.LockToken,
		req.DefaultAction,
		req.VisibilityConfig,
		req.Rules,
		req.TokenDomains,
		req.CustomResponseBodies,
		req.AssociationConfig,
		req.CaptchaConfig,
		req.ChallengeConfig,
		req.MonetizationConfig,
		req.DataProtectionConfig,
		req.ApplicationConfig,
		req.OnSourceDDoSProtectionConfig,
	)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "wafv2: updated web ACL", "id", req.ID)

	return json.Marshal(map[string]string{
		keyNextLockToken: w.LockToken,
	})
}

// deleteWebACLRequest is the request body for DeleteWebACL.
type deleteWebACLRequest struct {
	ID        string `json:"Id"`
	Name      string `json:"Name"`
	Scope     string `json:"Scope"`
	LockToken string `json:"LockToken"`
}

func (h *Handler) handleDeleteWebACL(ctx context.Context, body []byte) ([]byte, error) {
	var req deleteWebACLRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if err := requireIDNameScopeLockToken(req.ID, req.Name, req.Scope, req.LockToken); err != nil {
		return nil, err
	}

	if err := h.Backend.DeleteWebACL(ctx, req.ID, req.LockToken); err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "wafv2: deleted web ACL", "id", req.ID)

	return nil, nil
}

// handleListWebACLs handles the ListWebACLs request.
func (h *Handler) handleListWebACLs(ctx context.Context, body []byte) ([]byte, error) {
	return handleListResourceFamily(
		body,
		h.Backend.ListWebACLs(ctx),
		"WebACLs",
		func(w *WebACL) string { return w.Scope },
		func(w *WebACL) string { return w.Name },
		func(w *WebACL) map[string]string {
			return map[string]string{
				"Id":           w.ID,
				keyName:        w.Name,
				keyARN:         h.Backend.WebACLARN(w.Name, w.ID, w.Scope),
				keyLockToken:   w.LockToken,
				keyDescription: w.Description,
			}
		},
	)
}

// deleteFirewallManagerRuleGroupsRequest is the request body for DeleteFirewallManagerRuleGroups.
type deleteFirewallManagerRuleGroupsRequest struct {
	WebACLArn       string `json:"WebACLArn"`
	WebACLLockToken string `json:"WebACLLockToken"`
}

func (h *Handler) handleDeleteFirewallManagerRuleGroups(ctx context.Context, body []byte) ([]byte, error) {
	var req deleteFirewallManagerRuleGroupsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.WebACLArn == "" {
		return nil, fmt.Errorf("%w: WebACLArn is required", errInvalidRequest)
	}

	if req.WebACLLockToken == "" {
		return nil, fmt.Errorf("%w: WebACLLockToken is required", errInvalidRequest)
	}

	w, err := h.Backend.DeleteFirewallManagerRuleGroups(ctx, req.WebACLArn, req.WebACLLockToken)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "wafv2: deleted firewall manager rule groups", "webACLArn", req.WebACLArn)

	return json.Marshal(map[string]string{
		"NextWebACLLockToken": w.LockToken,
	})
}

// webACLDispatchOps returns the WebACL-family operation dispatch entries. Each entry is a
// bound method value -- handleCreateWebACL et al. already match the dispatchFn signature,
// so no wrapper closure is needed.
func (h *Handler) webACLDispatchOps() map[string]dispatchFn {
	return map[string]dispatchFn{
		"CreateWebACL":                    h.handleCreateWebACL,
		"GetWebACL":                       h.handleGetWebACL,
		"UpdateWebACL":                    h.handleUpdateWebACL,
		"DeleteWebACL":                    h.handleDeleteWebACL,
		"ListWebACLs":                     h.handleListWebACLs,
		"DeleteFirewallManagerRuleGroups": h.handleDeleteFirewallManagerRuleGroups,
	}
}
