package wafv2

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// createIPSetRequest is the request body for CreateIPSet.
type createIPSetRequest struct {
	Name             string    `json:"Name"`
	Scope            string    `json:"Scope"`
	Description      string    `json:"Description"`
	IPAddressVersion string    `json:"IPAddressVersion"`
	Addresses        []string  `json:"Addresses"`
	Tags             []tags.KV `json:"Tags"`
}

func (h *Handler) handleCreateIPSet(ctx context.Context, body []byte) ([]byte, error) {
	var req createIPSetRequest
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

	if req.IPAddressVersion == "" {
		req.IPAddressVersion = IPVersionIPv4
	}

	if req.IPAddressVersion != IPVersionIPv4 && req.IPAddressVersion != IPVersionIPv6 {
		return nil, fmt.Errorf("%w: IPAddressVersion must be %s or %s", errInvalidRequest, IPVersionIPv4, IPVersionIPv6)
	}

	if err := validateCIDRs(req.Addresses, req.IPAddressVersion); err != nil {
		return nil, err
	}

	tags := tags.MapFromKV(req.Tags)
	if err := validateTags(tags); err != nil {
		return nil, err
	}

	s, err := h.Backend.CreateIPSet(
		ctx,
		req.Name,
		req.Scope,
		req.Description,
		req.IPAddressVersion,
		req.Addresses,
		tags,
	)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "wafv2: created IP set", "name", s.Name, "id", s.ID)

	arnStr := h.Backend.IPSetARN(s.Name, s.ID, s.Scope)

	return json.Marshal(map[string]any{
		keySummary: map[string]string{
			"Id":           s.ID,
			keyName:        s.Name,
			keyARN:         arnStr,
			keyLockToken:   s.LockToken,
			keyDescription: s.Description,
		},
	})
}

// getIPSetRequest is the request body for GetIPSet.
type getIPSetRequest struct {
	ID    string `json:"Id"`
	Name  string `json:"Name"`
	Scope string `json:"Scope"`
}

func (h *Handler) handleGetIPSet(ctx context.Context, body []byte) ([]byte, error) {
	var req getIPSetRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if req.ID == "" {
		return nil, fmt.Errorf("%w: Id is required", errInvalidRequest)
	}

	if req.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", errInvalidRequest)
	}

	s, err := h.Backend.GetIPSet(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	if req.Scope != "" && s.Scope != req.Scope {
		return nil, fmt.Errorf("%w: IP set %q has scope %s, not %s", ErrIPSetNotFound, req.ID, s.Scope, req.Scope)
	}

	arnStr := h.Backend.IPSetARN(s.Name, s.ID, s.Scope)

	return json.Marshal(map[string]any{
		"IPSet": map[string]any{
			"Id":                s.ID,
			keyName:             s.Name,
			keyARN:              arnStr,
			keyLockToken:        s.LockToken,
			keyDescription:      s.Description,
			keyIPAddressVersion: s.IPAddressVersion,
			keyAddresses:        s.Addresses,
		},
		keyLockToken: s.LockToken,
	})
}

// updateIPSetRequest is the request body for UpdateIPSet.
type updateIPSetRequest struct {
	ID          string   `json:"Id"`
	Name        string   `json:"Name"`
	Scope       string   `json:"Scope"`
	LockToken   string   `json:"LockToken"`
	Description string   `json:"Description"`
	Addresses   []string `json:"Addresses"`
}

func (h *Handler) handleUpdateIPSet(ctx context.Context, body []byte) ([]byte, error) {
	var req updateIPSetRequest
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

	// Validate CIDRs against stored IP version — fetch first.
	existing, err := h.Backend.GetIPSet(ctx, req.ID)
	if err != nil {
		return nil, err
	}

	if req.Addresses != nil {
		if cidrErr := validateCIDRs(req.Addresses, existing.IPAddressVersion); cidrErr != nil {
			return nil, cidrErr
		}
	}

	s, err := h.Backend.UpdateIPSet(ctx, req.ID, req.Description, req.LockToken, req.Addresses)
	if err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "wafv2: updated IP set", "id", req.ID)

	return json.Marshal(map[string]string{
		keyNextLockToken: s.LockToken,
	})
}

// deleteIPSetRequest is the request body for DeleteIPSet.
type deleteIPSetRequest struct {
	ID        string `json:"Id"`
	Name      string `json:"Name"`
	Scope     string `json:"Scope"`
	LockToken string `json:"LockToken"`
}

func (h *Handler) handleDeleteIPSet(ctx context.Context, body []byte) ([]byte, error) {
	var req deleteIPSetRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("%w: %w", errInvalidRequest, err)
	}

	if err := requireIDNameScopeLockToken(req.ID, req.Name, req.Scope, req.LockToken); err != nil {
		return nil, err
	}

	if err := h.Backend.DeleteIPSet(ctx, req.ID, req.LockToken); err != nil {
		return nil, err
	}

	log := logger.Load(ctx)
	log.InfoContext(ctx, "wafv2: deleted IP set", "id", req.ID)

	return nil, nil
}

func (h *Handler) handleListIPSets(ctx context.Context, body []byte) ([]byte, error) {
	return handleListResourceFamily(
		body,
		h.Backend.ListIPSets(ctx),
		"IPSets",
		func(s *IPSet) string { return s.Scope },
		func(s *IPSet) string { return s.Name },
		func(s *IPSet) map[string]string {
			return map[string]string{
				"Id":           s.ID,
				keyName:        s.Name,
				keyARN:         h.Backend.IPSetARN(s.Name, s.ID, s.Scope),
				keyLockToken:   s.LockToken,
				keyDescription: s.Description,
			}
		},
	)
}

// ipSetDispatchOps returns the IPSet-family operation dispatch entries. Each entry is a
// bound method value -- handleCreateIPSet et al. already match the dispatchFn signature,
// so no wrapper closure is needed.
func (h *Handler) ipSetDispatchOps() map[string]dispatchFn {
	return map[string]dispatchFn{
		"CreateIPSet": h.handleCreateIPSet,
		"GetIPSet":    h.handleGetIPSet,
		"UpdateIPSet": h.handleUpdateIPSet,
		"DeleteIPSet": h.handleDeleteIPSet,
		"ListIPSets":  h.handleListIPSets,
	}
}
