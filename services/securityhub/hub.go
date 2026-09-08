package securityhub

import (
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

func (b *InMemoryBackend) hubARN() string {
	return arn.Build("securityhub", b.region, b.accountID, "hub/default")
}

func (b *InMemoryBackend) EnableHub(enableDefaultStandards bool, tags map[string]string) error {
	b.mu.Lock("EnableHub")
	defer b.mu.Unlock()

	if b.hubEnabled {
		return ErrHubAlreadyExists
	}

	b.hubEnabled = true
	now := time.Now().UTC().Format(time.RFC3339)
	b.hub = &Hub{
		HubArn:                  b.hubARN(),
		SubscribedAt:            now,
		AutoEnableControls:      true,
		AutoEnableStandards:     "DEFAULT",
		ControlFindingGenerator: "SECURITY_CONTROL",
	}

	if len(tags) > 0 {
		b.tags[b.hub.HubArn] = tags
	}

	if enableDefaultStandards {
		for i, std := range knownStandards {
			if std.EnabledByDefault {
				b.standardsSeq++
				subArn := fmt.Sprintf(
					"arn:aws:securityhub:%s:%s:subscription/%s/v/1.0.0",
					b.region,
					b.accountID,
					fmt.Sprintf("default-%d", i),
				)
				b.standardsSubscriptions.Put(&StandardsSubscription{
					StandardsSubscriptionArn: subArn,
					StandardsArn:             std.StandardsArn,
					StandardsInput:           map[string]string{},
					StandardsStatus:          statusReady,
				})
			}
		}
	}

	return nil
}

// DisableHub disables Security Hub. AWS documents that this is refused
// while the account is currently the Security Hub administrator for other
// member accounts (api_op_DisableSecurityHub.go) -- CreateMembers is this
// backend's only path to that relationship (Organizations delegated-admin
// never creates Member records, see organizations.go), so an account with
// at least one non-Removed member is administering it.
func (b *InMemoryBackend) DisableHub() error {
	b.mu.Lock("DisableHub")
	defer b.mu.Unlock()

	if !b.hubEnabled {
		return ErrHubNotEnabled
	}

	for _, m := range b.members.Snapshot() {
		if m.MemberStatus != "Removed" {
			return ErrHubIsAdministrator
		}
	}

	b.hubEnabled = false
	b.hub = nil

	return nil
}

func (b *InMemoryBackend) DescribeHub() (*Hub, error) {
	b.mu.RLock("DescribeHub")
	defer b.mu.RUnlock()

	if !b.hubEnabled || b.hub == nil {
		return nil, ErrHubNotEnabled
	}

	cp := *b.hub

	return &cp, nil
}

func (b *InMemoryBackend) UpdateHubConfiguration(
	autoEnableControls *bool,
	autoEnableStandards *string,
	controlFindingGenerator *string,
) error {
	b.mu.Lock("UpdateHubConfiguration")
	defer b.mu.Unlock()

	if !b.hubEnabled || b.hub == nil {
		return ErrHubNotEnabled
	}

	if autoEnableControls != nil {
		b.hub.AutoEnableControls = *autoEnableControls
	}

	if autoEnableStandards != nil {
		b.hub.AutoEnableStandards = *autoEnableStandards
	}

	if controlFindingGenerator != nil {
		b.hub.ControlFindingGenerator = *controlFindingGenerator
	}

	return nil
}

func (b *InMemoryBackend) hubV2ARN() string {
	return arn.Build("securityhub", b.region, b.accountID, "hub-v2/default")
}

func (b *InMemoryBackend) EnableSecurityHubV2(tags map[string]string) error {
	b.mu.Lock("EnableSecurityHubV2")
	defer b.mu.Unlock()

	if b.hubV2Enabled {
		return ErrHubAlreadyExists
	}

	now := time.Now().UTC().Format(time.RFC3339)
	b.hubV2Enabled = true
	b.hubV2 = &HubV2{
		HubV2Arn:  b.hubV2ARN(),
		CreatedAt: now,
		UpdatedAt: now,
		Features:  map[string]*HubV2Feature{},
	}

	if len(tags) > 0 {
		b.tags[b.hubV2.HubV2Arn] = tags
	}

	return nil
}

func (b *InMemoryBackend) DisableSecurityHubV2() error {
	b.mu.Lock("DisableSecurityHubV2")
	defer b.mu.Unlock()

	if !b.hubV2Enabled {
		return ErrHubNotEnabled
	}

	b.hubV2Enabled = false
	b.hubV2 = nil

	return nil
}

func (b *InMemoryBackend) DescribeSecurityHubV2() (*HubV2, error) {
	b.mu.RLock("DescribeSecurityHubV2")
	defer b.mu.RUnlock()

	if !b.hubV2Enabled || b.hubV2 == nil {
		return nil, ErrHubNotEnabled
	}

	return b.hubV2.clone(), nil
}

// clone deep-copies h, including Features: a shallow "cp := *h" only copies
// the map header, leaving cp.Features aliased to the live map that
// Enable/DisableSecurityHubFeatureV2 write into under lock -- see
// TestSecurityHubV2FeatureDescribeRace.
func (h *HubV2) clone() *HubV2 {
	cp := *h
	cp.Features = make(map[string]*HubV2Feature, len(h.Features))

	for name, f := range h.Features {
		fc := *f
		cp.Features[name] = &fc
	}

	return &cp
}

const (
	featureStatusEnabled  = "ENABLED"
	featureStatusDisabled = "DISABLED"
)

// EnableSecurityHubFeatureV2 enables an opt-in Security Hub V2 feature (e.g.
// NETWORK_SCANNING) for the account/region. Per the real API's documented
// behavior that the service must be enabled before a feature can be enabled,
// this requires SecurityHub V2 itself to already be enabled -- there is no
// standalone feature-enablement state independent of the V2 hub. The
// operation is idempotent: re-enabling an already-ENABLED feature is a
// silent no-op (no UpdatedAt bump), per the real API's documented "no
// changes are made" guarantee.
func (b *InMemoryBackend) EnableSecurityHubFeatureV2(featureName string) error {
	b.mu.Lock("EnableSecurityHubFeatureV2")
	defer b.mu.Unlock()

	if !b.hubV2Enabled || b.hubV2 == nil {
		return ErrHubNotEnabled
	}

	if b.hubV2.Features == nil {
		b.hubV2.Features = map[string]*HubV2Feature{}
	}

	if f, ok := b.hubV2.Features[featureName]; ok && f.FeatureStatus == featureStatusEnabled {
		return nil
	}

	b.hubV2.Features[featureName] = &HubV2Feature{
		FeatureStatus: featureStatusEnabled,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	return nil
}

// DisableSecurityHubFeatureV2 disables an opt-in Security Hub V2 feature.
// Mirrors EnableSecurityHubFeatureV2's gating and idempotency: requires
// SecurityHub V2 to be enabled, and disabling an already-DISABLED (or never
// enabled) feature is a no-op success rather than an error, per the real
// API's documented "no changes are made" guarantee.
func (b *InMemoryBackend) DisableSecurityHubFeatureV2(featureName string) error {
	b.mu.Lock("DisableSecurityHubFeatureV2")
	defer b.mu.Unlock()

	if !b.hubV2Enabled || b.hubV2 == nil {
		return ErrHubNotEnabled
	}

	if b.hubV2.Features == nil {
		b.hubV2.Features = map[string]*HubV2Feature{}
	}

	// A feature that was never enabled (absent from the map) is implicitly
	// already disabled -- leave it absent rather than fabricating a DISABLED
	// entry, matching "if the feature is already disabled, no changes are made."
	if f, ok := b.hubV2.Features[featureName]; ok && f.FeatureStatus == featureStatusEnabled {
		b.hubV2.Features[featureName] = &HubV2Feature{
			FeatureStatus: featureStatusDisabled,
			UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
	}

	return nil
}
