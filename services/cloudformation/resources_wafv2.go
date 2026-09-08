package cloudformation

import (
	"context"
	"encoding/json"
	"fmt"
)

const wafScopeRegional = "REGIONAL"

// ---- WAFv2 WebACL ----

func (rc *ResourceCreator) createWAFv2WebACL(
	ctx context.Context,
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.WAFv2 == nil {
		return logicalID + "-stub", nil
	}

	name := strProp(props, "Name", params, physicalIDs)
	if name == "" {
		name = logicalID
	}

	scope := strProp(props, "Scope", params, physicalIDs)
	if scope == "" {
		scope = wafScopeRegional
	}

	acl, err := rc.backends.WAFv2.Backend.CreateWebACL(
		ctx,
		name, scope, "",
		json.RawMessage(`{"Allow":{}}`), nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil,
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("create WAFv2 WebACL %s: %w", name, err)
	}

	return acl.ID, nil
}

func (rc *ResourceCreator) deleteWAFv2WebACL(ctx context.Context, id string) error {
	if rc.backends.WAFv2 == nil {
		return nil
	}

	return rc.backends.WAFv2.Backend.DeleteWebACL(ctx, id, "")
}

// ---- WAFv2 IPSet ----

func (rc *ResourceCreator) createWAFv2IPSet(
	ctx context.Context,
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.WAFv2 == nil {
		return logicalID + "-stub", nil
	}

	name := strProp(props, "Name", params, physicalIDs)
	if name == "" {
		name = logicalID
	}

	scope := strProp(props, "Scope", params, physicalIDs)
	if scope == "" {
		scope = wafScopeRegional
	}

	ipVersion := strProp(props, "IPAddressVersion", params, physicalIDs)
	if ipVersion == "" {
		ipVersion = "IPV4"
	}

	ipset, err := rc.backends.WAFv2.Backend.CreateIPSet(ctx, name, scope, "", ipVersion, nil, nil)
	if err != nil {
		return "", fmt.Errorf("create WAFv2 IPSet %s: %w", name, err)
	}

	return ipset.ID, nil
}

func (rc *ResourceCreator) deleteWAFv2IPSet(ctx context.Context, id string) error {
	if rc.backends.WAFv2 == nil {
		return nil
	}

	return rc.backends.WAFv2.Backend.DeleteIPSet(ctx, id, "")
}

// ---- WAFv2 RuleGroup ----

func (rc *ResourceCreator) createWAFv2RuleGroup(
	ctx context.Context,
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.WAFv2 == nil {
		return logicalID + "-stub", nil
	}

	name := strProp(props, "Name", params, physicalIDs)
	if name == "" {
		name = logicalID
	}

	scope := strProp(props, "Scope", params, physicalIDs)
	if scope == "" {
		scope = wafScopeRegional
	}

	rg, err := rc.backends.WAFv2.Backend.CreateRuleGroup(ctx, name, scope, "", "", 0, nil, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("create WAFv2 RuleGroup %s: %w", name, err)
	}

	return rg.ID, nil
}

func (rc *ResourceCreator) deleteWAFv2RuleGroup(ctx context.Context, id string) error {
	if rc.backends.WAFv2 == nil {
		return nil
	}

	return rc.backends.WAFv2.Backend.DeleteRuleGroup(ctx, id, "")
}
