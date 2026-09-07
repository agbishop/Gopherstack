package workmail

import (
	"fmt"
	"net"
	"slices"
	"sort"
	"strings"
	"time"
)

// --- Access Control Rules ---

// PutAccessControlRule creates or updates an access control rule.
func (b *InMemoryBackend) PutAccessControlRule(
	orgID string, params PutAccessControlRuleParams,
) (*AccessControlRule, error) {
	b.mu.Lock("PutAccessControlRule")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return nil, fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}

	now := time.Now().UTC()
	existing, _ := b.accessRules.Get(orgKey(orgID, params.Name))

	rule := &AccessControlRule{
		DateCreated:             now,
		DateModified:            now,
		Name:                    params.Name,
		Effect:                  params.Effect,
		Description:             params.Description,
		IPRanges:                params.IPRanges,
		NotIPRanges:             params.NotIPRanges,
		Actions:                 params.Actions,
		NotActions:              params.NotActions,
		UserIDs:                 params.UserIDs,
		NotUserIDs:              params.NotUserIDs,
		ImpersonationRoleIDs:    params.ImpersonationRoleIDs,
		NotImpersonationRoleIDs: params.NotImpersonationRoleIDs,
		orgID:                   orgID,
	}
	if existing != nil {
		rule.DateCreated = existing.DateCreated
	}

	b.accessRules.Put(rule)

	return rule, nil
}

// DeleteAccessControlRule removes an access control rule.
func (b *InMemoryBackend) DeleteAccessControlRule(orgID, name string) error {
	b.mu.Lock("DeleteAccessControlRule")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrOrganizationNotFound, orgID)
	}
	// Real DeleteAccessControlRule doc (aws-sdk-go-v2 api_op_DeleteAccessControlRule.go):
	// "Deleting already deleted and non-existing rules does not produce an
	// error. In those cases, the service sends back an HTTP 200 response
	// with an empty HTTP body." -- deleting a missing rule is a no-op, not
	// EntityNotFoundException (which this op's model doesn't even declare).
	b.accessRules.Delete(orgKey(orgID, name))

	return nil
}

// GetAccessControlEffect evaluates access control rules.
func (b *InMemoryBackend) GetAccessControlEffect(
	orgID, ipAddr, action, userID, impersonationRoleID string,
) (string, []string, error) {
	b.mu.RLock("GetAccessControlEffect")
	defer b.mu.RUnlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return "", nil, fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}

	byOrg := b.accessRulesByOrg.Get(orgID)
	rules := make([]*AccessControlRule, 0, len(byOrg))
	rules = append(rules, byOrg...)
	// AWS evaluates rules in creation order; sort by DateCreated for determinism
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].DateCreated.Before(rules[j].DateCreated)
	})

	for _, rule := range rules {
		if !ruleMatchesRequest(rule, ipAddr, action, userID, impersonationRoleID) {
			continue
		}

		return rule.Effect, []string{rule.Name}, nil
	}

	return effectAllow, []string{}, nil
}

// ruleMatchesRequest returns true when ALL non-empty condition lists match.
// Split into matchesIPAndAction/matchesUserAndImpersonation to stay under
// the per-function cyclomatic-complexity budget.
func ruleMatchesRequest(rule *AccessControlRule, ipAddr, action, userID, impersonationRoleID string) bool {
	return matchesIPAndAction(rule, ipAddr, action) && matchesUserAndImpersonation(rule, userID, impersonationRoleID)
}

// matchesIPAndAction checks the IpRanges/NotIpRanges and Actions/NotActions
// condition pairs.
func matchesIPAndAction(rule *AccessControlRule, ipAddr, action string) bool {
	if len(rule.IPRanges) > 0 && !matchesCIDRList(ipAddr, rule.IPRanges) {
		return false
	}
	if len(rule.NotIPRanges) > 0 && matchesCIDRList(ipAddr, rule.NotIPRanges) {
		return false
	}
	if len(rule.Actions) > 0 && !slices.Contains(rule.Actions, action) {
		return false
	}
	if len(rule.NotActions) > 0 && slices.Contains(rule.NotActions, action) {
		return false
	}

	return true
}

// matchesUserAndImpersonation checks the UserIds/NotUserIds and
// ImpersonationRoleIds/NotImpersonationRoleIds condition pairs.
func matchesUserAndImpersonation(rule *AccessControlRule, userID, impersonationRoleID string) bool {
	if len(rule.UserIDs) > 0 && !slices.Contains(rule.UserIDs, userID) {
		return false
	}
	if len(rule.NotUserIDs) > 0 && slices.Contains(rule.NotUserIDs, userID) {
		return false
	}
	if len(rule.ImpersonationRoleIDs) > 0 && !slices.Contains(rule.ImpersonationRoleIDs, impersonationRoleID) {
		return false
	}
	if len(rule.NotImpersonationRoleIDs) > 0 && slices.Contains(rule.NotImpersonationRoleIDs, impersonationRoleID) {
		return false
	}

	return true
}

func matchesCIDRList(ipAddr string, cidrs []string) bool {
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return false
	}
	for _, cidr := range cidrs {
		if !strings.Contains(cidr, "/") {
			cidr += "/32"
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// ListAccessControlRules returns all access control rules.
func (b *InMemoryBackend) ListAccessControlRules(orgID string) ([]*AccessControlRule, error) {
	b.mu.RLock("ListAccessControlRules")
	defer b.mu.RUnlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return nil, fmt.Errorf("%w: organization %q not found", ErrOrganizationNotFound, orgID)
	}

	byOrg := b.accessRulesByOrg.Get(orgID)
	rules := make([]*AccessControlRule, 0, len(byOrg))
	rules = append(rules, byOrg...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })

	return rules, nil
}
