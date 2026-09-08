package workmail

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// --- Mobile Device Access Rules ---

// CreateMobileDeviceAccessRule creates a mobile device access rule.
func (b *InMemoryBackend) CreateMobileDeviceAccessRule(
	orgID, name, effect, description string,
	deviceModels, notDeviceModels, deviceTypes, notDeviceTypes,
	deviceOperatingSystems, notDeviceOperatingSystems, deviceUserAgents, notDeviceUserAgents []string,
) (*MobileDeviceAccessRule, error) {
	b.mu.Lock("CreateMobileDeviceAccessRule")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return nil, fmt.Errorf("%w: organization %q not found", ErrOrganizationNotFound, orgID)
	}
	now := time.Now()
	rule := &MobileDeviceAccessRule{
		DateCreated:               now,
		DateModified:              now,
		RuleID:                    newID(),
		Name:                      name,
		Effect:                    effect,
		Description:               description,
		DeviceModels:              deviceModels,
		NotDeviceModels:           notDeviceModels,
		DeviceTypes:               deviceTypes,
		NotDeviceTypes:            notDeviceTypes,
		DeviceOperatingSystems:    deviceOperatingSystems,
		NotDeviceOperatingSystems: notDeviceOperatingSystems,
		DeviceUserAgents:          deviceUserAgents,
		NotDeviceUserAgents:       notDeviceUserAgents,
		orgID:                     orgID,
	}
	b.mobileDeviceRules.Put(rule)

	return rule, nil
}

// DeleteMobileDeviceAccessRule deletes a mobile device access rule.
func (b *InMemoryBackend) DeleteMobileDeviceAccessRule(orgID, ruleID string) error {
	b.mu.Lock("DeleteMobileDeviceAccessRule")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrOrganizationNotFound, orgID)
	}
	// Real DeleteMobileDeviceAccessRule doc (aws-sdk-go-v2
	// api_op_DeleteMobileDeviceAccessRule.go): "Deleting already deleted and
	// non-existing rules does not produce an error. In those cases, the
	// service sends back an HTTP 200 response with an empty HTTP body." --
	// this op's model doesn't even declare EntityNotFoundException.
	b.mobileDeviceRules.Delete(orgKey(orgID, ruleID))

	return nil
}

// UpdateMobileDeviceAccessRule updates a mobile device access rule.
func (b *InMemoryBackend) UpdateMobileDeviceAccessRule(
	orgID, ruleID, name, effect, description string,
	deviceModels, notDeviceModels, deviceTypes, notDeviceTypes,
	deviceOperatingSystems, notDeviceOperatingSystems, deviceUserAgents, notDeviceUserAgents []string,
) error {
	b.mu.Lock("UpdateMobileDeviceAccessRule")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}
	rule, ok := b.mobileDeviceRules.Get(orgKey(orgID, ruleID))
	if !ok {
		return fmt.Errorf("%w: mobile device access rule %q not found", ErrNotFound, ruleID)
	}
	rule.DateModified = time.Now()
	rule.Name = name
	rule.Effect = effect
	rule.Description = description
	rule.DeviceModels = deviceModels
	rule.NotDeviceModels = notDeviceModels
	rule.DeviceTypes = deviceTypes
	rule.NotDeviceTypes = notDeviceTypes
	rule.DeviceOperatingSystems = deviceOperatingSystems
	rule.NotDeviceOperatingSystems = notDeviceOperatingSystems
	rule.DeviceUserAgents = deviceUserAgents
	rule.NotDeviceUserAgents = notDeviceUserAgents

	return nil
}

// ListMobileDeviceAccessRules lists all mobile device access rules for an org.
func (b *InMemoryBackend) ListMobileDeviceAccessRules(
	orgID string,
) ([]*MobileDeviceAccessRule, error) {
	b.mu.RLock("ListMobileDeviceAccessRules")
	defer b.mu.RUnlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return nil, fmt.Errorf("%w: organization %q not found", ErrOrganizationNotFound, orgID)
	}
	byOrg := b.mobileDeviceRulesByOrg.Get(orgID)
	rules := make([]*MobileDeviceAccessRule, 0, len(byOrg))
	rules = append(rules, byOrg...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].RuleID < rules[j].RuleID })

	return rules, nil
}

func matchesFilter(value string, allow, deny []string) bool {
	if len(allow) > 0 {
		found := false
		for _, v := range allow {
			if strings.EqualFold(v, value) {
				found = true

				break
			}
		}
		if !found {
			return false
		}
	}
	for _, v := range deny {
		if strings.EqualFold(v, value) {
			return false
		}
	}

	return true
}

// GetMobileDeviceAccessEffect evaluates rules for a simulated device.
func (b *InMemoryBackend) GetMobileDeviceAccessEffect(
	orgID, deviceType, deviceModel, deviceOS, deviceUserAgent string,
) (string, []*MobileDeviceMatchedRule, error) {
	b.mu.RLock("GetMobileDeviceAccessEffect")
	defer b.mu.RUnlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return "", nil, fmt.Errorf("%w: organization %q not found", ErrOrganizationNotFound, orgID)
	}
	byOrg := b.mobileDeviceRulesByOrg.Get(orgID)
	rules := make([]*MobileDeviceAccessRule, 0, len(byOrg))
	rules = append(rules, byOrg...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].RuleID < rules[j].RuleID })

	effect := "ALLOW"
	matched := []*MobileDeviceMatchedRule{}
	for _, rule := range rules {
		if !matchesFilter(deviceType, rule.DeviceTypes, rule.NotDeviceTypes) {
			continue
		}
		if !matchesFilter(deviceModel, rule.DeviceModels, rule.NotDeviceModels) {
			continue
		}
		if !matchesFilter(deviceOS, rule.DeviceOperatingSystems, rule.NotDeviceOperatingSystems) {
			continue
		}
		if !matchesFilter(deviceUserAgent, rule.DeviceUserAgents, rule.NotDeviceUserAgents) {
			continue
		}
		effect = rule.Effect
		matched = append(matched, &MobileDeviceMatchedRule{RuleID: rule.RuleID, Name: rule.Name})
	}

	return effect, matched, nil
}

// --- Mobile Device Access Overrides ---

func mobileOverrideKey(userID, deviceID string) string {
	return userID + ":" + strings.ToLower(deviceID)
}

// PutMobileDeviceAccessOverride creates or updates a per-user per-device override.
func (b *InMemoryBackend) PutMobileDeviceAccessOverride(
	orgID, userID, deviceID, effect, description string,
) error {
	b.mu.Lock("PutMobileDeviceAccessOverride")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}
	key := orgKey(orgID, mobileOverrideKey(userID, deviceID))
	now := time.Now()
	if existing, ok := b.mobileDeviceOverrides.Get(key); ok {
		existing.Effect = effect
		existing.Description = description
		existing.DateModified = now
	} else {
		b.mobileDeviceOverrides.Put(&MobileDeviceAccessOverride{
			DateCreated:  now,
			DateModified: now,
			UserID:       userID,
			DeviceID:     strings.ToLower(deviceID),
			Effect:       effect,
			Description:  description,
			orgID:        orgID,
		})
	}

	return nil
}

// DeleteMobileDeviceAccessOverride removes a per-user per-device override.
func (b *InMemoryBackend) DeleteMobileDeviceAccessOverride(orgID, userID, deviceID string) error {
	b.mu.Lock("DeleteMobileDeviceAccessOverride")
	defer b.mu.Unlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}
	key := orgKey(orgID, mobileOverrideKey(userID, deviceID))
	if !b.mobileDeviceOverrides.Delete(key) {
		// Unlike DeleteAccessControlRule/DeleteMobileDeviceAccessRule,
		// DeleteMobileDeviceAccessOverride's own error model
		// (awsAwsjson11_deserializeOpErrorDeleteMobileDeviceAccessOverride)
		// DOES declare EntityNotFoundException, even though its SDK doc
		// carries the same "does not produce an error" sentence as those
		// two -- the model and the doc disagree here. Emitting the declared
		// code is correct per the wire model; not changed to a no-op
		// (gopherstack-hp83 investigation).
		return fmt.Errorf("%w: mobile device access override not found", ErrNotFound)
	}

	return nil
}

// GetMobileDeviceAccessOverride retrieves a per-user per-device override.
func (b *InMemoryBackend) GetMobileDeviceAccessOverride(
	orgID, userID, deviceID string,
) (*MobileDeviceAccessOverride, error) {
	b.mu.RLock("GetMobileDeviceAccessOverride")
	defer b.mu.RUnlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return nil, fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}
	key := orgKey(orgID, mobileOverrideKey(userID, deviceID))
	ov, ok := b.mobileDeviceOverrides.Get(key)
	if !ok {
		return nil, fmt.Errorf("%w: mobile device access override not found", ErrNotFound)
	}

	return ov, nil
}

// ListMobileDeviceAccessOverrides lists overrides filtered by userID and/or deviceID.
func (b *InMemoryBackend) ListMobileDeviceAccessOverrides(
	orgID, userID, deviceID string, maxResults int32, nextToken string,
) ([]*MobileDeviceAccessOverride, string, error) {
	b.mu.RLock("ListMobileDeviceAccessOverrides")
	defer b.mu.RUnlock()

	if _, ok := b.organizations.Get(orgID); !ok {
		return nil, "", fmt.Errorf("%w: organization %q not found", ErrNotFound, orgID)
	}
	all := make([]*MobileDeviceAccessOverride, 0)
	for _, ov := range b.mobileDeviceOverridesByOrg.Get(orgID) {
		if userID != "" && ov.UserID != userID {
			continue
		}
		if deviceID != "" && !strings.EqualFold(ov.DeviceID, deviceID) {
			continue
		}
		all = append(all, ov)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].UserID+all[i].DeviceID < all[j].UserID+all[j].DeviceID
	})
	page, next := paginate(all, maxResults, nextToken)

	return page, next, nil
}
